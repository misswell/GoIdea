package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

const (
	defaultAgentRunTabLimit     = 1
	defaultAgentRunArchiveLimit = 999
	maxAgentRunTabLimit         = 1000
	maxAgentRunArchiveLimit     = 100_000
)

type agentRunSession struct {
	id          uint64
	request     agentRunRequest
	runID       uint32
	channel     uint32
	running     bool
	awaiting    bool
	stopped     bool
	stdinClosed bool
	pid         *int32
	exitCode    *int32
	started     time.Time
	ended       time.Time
	err         string

	output    []outputLine
	truncated bool
	dropped   int
}

func (r *agentRunSession) live() bool {
	return r != nil && (r.running || r.awaiting)
}

func (m *Model) agentRunSettingsPage() {
	if m == nil || m.mgr == nil && m.invokeHook == nil {
		return
	}
	state := m.nativeSettings.pages[settingsPageAgentMCPServer]
	if state.loaded || state.loading {
		return
	}
	m.loadNativeSettingsPage(settingsPageAgentMCPServer)
}

func (m *Model) agentRunLimits() (tabs, archive int) {
	tabs = defaultAgentRunTabLimit
	archive = defaultAgentRunArchiveLimit
	state, ok := m.nativeSettings.pages[settingsPageAgentMCPServer]
	if !ok || !state.loaded {
		return tabs, archive
	}
	tabs = nativeRunLimit(state.applied["mcpServer.agentTabLimit"], maxAgentRunTabLimit, tabs)
	archive = nativeRunLimit(state.applied["mcpServer.agentArchiveLimit"], maxAgentRunArchiveLimit, archive)
	return tabs, archive
}

func nativeRunLimit(value any, maximum, fallback int) int {
	var number int
	switch typed := value.(type) {
	case float64:
		number = int(math.Round(typed))
	case float32:
		number = int(math.Round(float64(typed)))
	case int:
		number = typed
	case int64:
		number = int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return fallback
		}
		number = int(parsed)
	default:
		return fallback
	}
	if number < 0 {
		return 0
	}
	if number > maximum {
		return maximum
	}
	return number
}

// finishedAgentRuns returns finished agent tabs newest first, as OpenIDEA's
// archivedAgentSessions does before splitting the newest visible tabs from the
// archive.
func (m *Model) finishedAgentRuns() []*agentRunSession {
	runs := make([]*agentRunSession, 0, len(m.run.agentRuns))
	for _, run := range m.run.agentRuns {
		if run != nil && !run.live() {
			runs = append(runs, run)
		}
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].ended.Equal(runs[j].ended) {
			return runs[i].id > runs[j].id
		}
		return runs[i].ended.After(runs[j].ended)
	})
	return runs
}

func (m *Model) archivedAgentRuns() []*agentRunSession {
	finished := m.finishedAgentRuns()
	tabs, _ := m.agentRunLimits()
	if len(finished) <= tabs {
		return nil
	}
	return finished[tabs:]
}

func (m *Model) visibleAgentRuns() []*agentRunSession {
	archived := m.archivedAgentRuns()
	hidden := make(map[uint64]bool, len(archived))
	for _, run := range archived {
		hidden[run.id] = true
	}
	out := make([]*agentRunSession, 0, len(m.run.agentRuns)-len(archived))
	for _, run := range m.run.agentRuns {
		if run != nil && !hidden[run.id] {
			out = append(out, run)
		}
	}
	return out
}

func (m *Model) activeAgentRun() *agentRunSession {
	for _, run := range m.run.agentRuns {
		if run != nil && run.id == m.run.agentActive {
			return run
		}
	}
	return nil
}

func (m *Model) agentRunByChannel(channel uint32) *agentRunSession {
	for _, run := range m.run.agentRuns {
		if run != nil && run.channel == channel {
			return run
		}
	}
	return nil
}

func (m *Model) agentRunByName(name string) *agentRunSession {
	for _, run := range m.run.agentRuns {
		if run != nil && run.request.Name == name {
			return run
		}
	}
	return nil
}

func (m *Model) agentRunByID(id uint64) *agentRunSession {
	for _, run := range m.run.agentRuns {
		if run != nil && run.id == id {
			return run
		}
	}
	return nil
}

func (m *Model) newAgentRunSession(request agentRunRequest) *agentRunSession {
	m.run.agentNextID++
	run := &agentRunSession{
		id:       m.run.agentNextID,
		request:  request,
		awaiting: true,
		started:  time.Now(),
	}
	m.run.agentRuns = append(m.run.agentRuns, run)
	m.run.agentActive = run.id
	return run
}

func (m *Model) beginAgentRun(run *agentRunSession) {
	run.running = false
	run.awaiting = true
	run.stopped = false
	run.stdinClosed = false
	run.runID = 0
	run.channel = m.nextChannel()
	run.pid = nil
	run.exitCode = nil
	run.started = time.Now()
	run.ended = time.Time{}
	run.err = ""
	run.output = nil
	run.truncated = false
	run.dropped = 0
	m.run.agentActive = run.id
}

func (m *Model) acceptAgentRunChunk(channel uint32, data json.RawMessage) bool {
	run := m.agentRunByChannel(channel)
	if run == nil {
		return m.runChildChannelRetired(channel)
	}
	var chunk openidea.OutputChunk
	if json.Unmarshal(data, &chunk) != nil {
		return true
	}
	run.append(chunk)
	m.invalidate()
	return true
}

func (m *Model) acceptAgentRunExit(event openidea.RunExitedEvent) bool {
	if m.runChildRunRetired(event.Run) {
		return true
	}
	hasPendingStart := false
	for _, run := range m.run.agentRuns {
		if run != nil && run.awaiting && run.runID == 0 {
			hasPendingStart = true
		}
		if run == nil || run.runID == 0 || run.runID != event.Run || !run.live() {
			continue
		}
		m.finishAgentRun(run, event.Code)
		return true
	}
	if hasPendingStart && event.Run != 0 {
		if m.run.agentPendingExit == nil {
			m.run.agentPendingExit = map[uint32]*int32{}
		}
		m.run.agentPendingExit[event.Run] = event.Code
	}
	return false
}

func (m *Model) finishAgentRun(run *agentRunSession, code *int32) {
	run.awaiting = false
	run.running = false
	run.ended = time.Now()
	run.exitCode = cloneAgentExitCode(code)
	switch {
	case code == nil:
		run.err = "The agent command ended"
	case *code == 0:
		run.err = ""
	default:
		run.err = fmt.Sprintf("The agent command failed with exit code %d", *code)
	}
	if m.activeAgentRun() == run {
		m.status = "Agent " + run.request.Name + ": " + run.err
		if run.err == "" {
			m.status = "Agent " + run.request.Name + " finished"
		}
	}
	m.trimAgentRuns()
	m.invalidate()
}

func cloneAgentExitCode(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (r *agentRunSession) append(chunk openidea.OutputChunk) {
	text := strings.TrimSuffix(stripANSI(chunk.Text), "\n")
	if text == "" {
		return
	}
	kind := chunk.Kind
	if kind == "" {
		kind = "stdout"
	}
	for _, line := range strings.Split(text, "\n") {
		r.output = append(r.output, outputLine{kind: kind, text: line})
	}
	if over := len(r.output) - maxRunLines; over > 0 {
		r.output = append(r.output[:0], r.output[over:]...)
		r.truncated = true
		r.dropped += over
	}
}

func (m *Model) selectAgentRun(id uint64) {
	for _, run := range m.run.agentRuns {
		if run != nil && run.id == id {
			m.run.agentActive = id
			m.run.activeChild = 0
			if m.stackTrace != nil {
				m.stackTrace.active = -1
			}
			m.invalidate()
			return
		}
	}
}

func (m *Model) clearActiveAgentRun() bool {
	run := m.activeAgentRun()
	if run == nil {
		return false
	}
	run.output = nil
	run.truncated = false
	run.dropped = 0
	m.invalidate()
	return true
}

func (m *Model) rerunActiveAgentRun() bool {
	run := m.activeAgentRun()
	if run == nil {
		return false
	}
	if run.live() {
		m.status = run.request.Name + " is still running"
		m.invalidate()
		return true
	}
	return m.StartAgentRun(run.request)
}

func (m *Model) stopActiveAgentRun() bool {
	run := m.activeAgentRun()
	if run == nil || !run.live() {
		return false
	}
	run.stopped = true
	if run.runID == 0 {
		run.err = "The agent command has not started"
		m.invalidate()
		return true
	}
	id := run.runID
	run.err = "Stopping the agent command…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "run_stop", map[string]any{"id": id})
		m.post(func() {
			if err != nil {
				if current := m.agentRunByName(run.request.Name); current == run {
					run.err = "Stopping failed: " + err.Error()
				}
			}
			m.invalidate()
		})
	}()
	return true
}

func (m *Model) closeAgentRun(id uint64) {
	var closed *agentRunSession
	runs := m.run.agentRuns[:0]
	for _, run := range m.run.agentRuns {
		if run != nil && run.id == id {
			closed = run
			continue
		}
		runs = append(runs, run)
	}
	m.run.agentRuns = runs
	if closed == nil {
		return
	}
	m.retireRunChildChannel(closed.channel)
	m.retireRunChildRun(closed.runID)
	m.notifyMCPAgentClosed(closed.request.Name)
	if closed.live() && closed.runID != 0 {
		runID := closed.runID
		go func() {
			ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "run_stop", map[string]any{"id": runID})
		}()
	}
	if m.run.agentActive == id {
		m.run.agentActive = 0
		if visible := m.visibleAgentRuns(); len(visible) > 0 {
			m.run.agentActive = visible[len(visible)-1].id
		}
	}
	m.trimAgentRuns()
	m.invalidate()
}

func (m *Model) closeAgentArchive() {
	archive := m.archivedAgentRuns()
	if len(archive) == 0 {
		return
	}
	close := make(map[uint64]bool, len(archive))
	for _, run := range archive {
		close[run.id] = true
	}
	runs := m.run.agentRuns[:0]
	for _, run := range m.run.agentRuns {
		if run != nil && !close[run.id] {
			runs = append(runs, run)
		}
	}
	m.run.agentRuns = runs
	if close[m.run.agentActive] {
		m.run.agentActive = 0
	}
	for _, run := range archive {
		m.notifyMCPAgentClosed(run.request.Name)
	}
	m.status = fmt.Sprintf("Closed %d archived agent tabs", len(archive))
	m.invalidate()
}

func (m *Model) trimAgentRuns() {
	_, archiveLimit := m.agentRunLimits()
	archive := m.archivedAgentRuns()
	if len(archive) <= archiveLimit {
		return
	}
	remove := make(map[uint64]bool, len(archive)-archiveLimit)
	for _, run := range archive[archiveLimit:] {
		remove[run.id] = true
	}
	runs := m.run.agentRuns[:0]
	for _, run := range m.run.agentRuns {
		if run != nil && !remove[run.id] {
			runs = append(runs, run)
		}
	}
	m.run.agentRuns = runs
	if remove[m.run.agentActive] {
		m.run.agentActive = 0
		if visible := m.visibleAgentRuns(); len(visible) > 0 {
			m.run.agentActive = visible[len(visible)-1].id
		}
	}
}
