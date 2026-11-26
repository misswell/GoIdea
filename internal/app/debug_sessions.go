package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// debugSessionState is one Debug content tab. The selected tab is exposed by
// DebugPane's existing fields, so every action and view keeps operating on one
// session at a time while the other sessions retain independent state.
type debugSessionState struct {
	session  uint32
	parent   uint32
	adapter  string
	config   string
	tabName  string
	view     debugView
	console  debugConsoleView
	memory   debugMemoryView
	stopping bool

	caps        openidea.DebugCapabilities
	capsKnown   bool
	capsRequest uint64
	jvmBackend  openidea.JvmBackendState

	stopGeneration uint64
	threadRequest  uint64
	frameRequest   uint64

	lastID   string
	lastRaw  json.RawMessage
	lastKind string

	pluginExit    func(code *int32, stopped bool)
	pluginStopped bool

	launchCwd           string
	hotSwapNeedsRestart bool

	paused     bool
	pausing    bool
	ended      bool
	stopFile   string
	stopLine   int
	stopThread int64
	reason     string

	threads      []openidea.ThreadInfo
	frames       []openidea.StackFrame
	selFrame     int
	scopes       []openidea.Scope
	vars         []openidea.Variable
	frameID      int64
	varContainer int64
	selVar       int

	output    []outputLine
	truncated bool
	err       string
	status    string
	watches   []debugWatch
	evalText  string

	evaluate  debugEvaluateState
	setValue  debugSetValueState
	smartStep debugSmartStepState
	heap      debugHeapState
	evalBusy  bool
}

func cloneOutputLines(lines []outputLine) []outputLine {
	return append([]outputLine(nil), lines...)
}

func cloneThreads(threads []openidea.ThreadInfo) []openidea.ThreadInfo {
	return append([]openidea.ThreadInfo(nil), threads...)
}

func cloneFrames(frames []openidea.StackFrame) []openidea.StackFrame {
	out := make([]openidea.StackFrame, len(frames))
	copy(out, frames)
	for i := range out {
		if frames[i].Path != nil {
			path := *frames[i].Path
			out[i].Path = &path
		}
	}
	return out
}

func cloneWatches(watches []debugWatch) []debugWatch {
	return append([]debugWatch(nil), watches...)
}

// captureDebugSession copies the active tab without breaking the existing
// DebugPane field layout that actions, tests and smoke helpers already use.
func (d *DebugPane) captureDebugSession() debugSessionState {
	return debugSessionState{
		session:             d.session,
		parent:              d.parent,
		adapter:             d.adapter,
		config:              d.config,
		tabName:             d.tabName,
		view:                d.view,
		console:             d.console,
		memory:              d.memory,
		stopping:            d.stopping,
		caps:                d.caps,
		capsKnown:           d.capsKnown,
		capsRequest:         d.capsRequest,
		jvmBackend:          d.jvmBackend,
		stopGeneration:      d.stopGeneration,
		threadRequest:       d.threadRequest,
		frameRequest:        d.frameRequest,
		lastID:              d.lastID,
		lastRaw:             append(json.RawMessage(nil), d.lastRaw...),
		lastKind:            d.lastKind,
		pluginExit:          d.pluginExit,
		pluginStopped:       d.pluginStopped,
		launchCwd:           d.launchCwd,
		hotSwapNeedsRestart: d.hotSwapNeedsRestart,
		paused:              d.paused,
		pausing:             d.pausing,
		ended:               d.ended,
		stopFile:            d.stopFile,
		stopLine:            d.stopLine,
		stopThread:          d.stopThread,
		reason:              d.reason,
		threads:             cloneThreads(d.threads),
		frames:              cloneFrames(d.frames),
		selFrame:            d.selFrame,
		scopes:              append([]openidea.Scope(nil), d.scopes...),
		vars:                append([]openidea.Variable(nil), d.vars...),
		frameID:             d.frameID,
		varContainer:        d.varContainer,
		selVar:              d.selVar,
		output:              cloneOutputLines(d.output),
		truncated:           d.truncated,
		err:                 d.err,
		status:              d.status,
		watches:             cloneWatches(d.watches),
		evalText:            d.evalText,
		evaluate:            d.evaluate,
		setValue:            d.setValue,
		smartStep:           d.smartStep,
		heap:                d.heap,
		evalBusy:            d.evalBusy,
	}
}

func (d *DebugPane) restoreDebugSession(state debugSessionState) {
	state.lastRaw = append(json.RawMessage(nil), state.lastRaw...)
	state.threads = cloneThreads(state.threads)
	state.frames = cloneFrames(state.frames)
	state.scopes = append([]openidea.Scope(nil), state.scopes...)
	state.vars = append([]openidea.Variable(nil), state.vars...)
	state.output = cloneOutputLines(state.output)
	state.watches = cloneWatches(state.watches)

	d.active = state.session
	d.session = state.session
	d.parent = state.parent
	d.adapter = state.adapter
	d.config = state.config
	d.tabName = state.tabName
	d.view = state.view
	d.console = state.console
	d.memory = state.memory
	d.stopping = state.stopping
	d.caps = state.caps
	d.capsKnown = state.capsKnown
	d.capsRequest = state.capsRequest
	d.jvmBackend = state.jvmBackend
	d.stopGeneration = state.stopGeneration
	d.threadRequest = state.threadRequest
	d.frameRequest = state.frameRequest
	d.lastID = state.lastID
	d.lastRaw = state.lastRaw
	d.lastKind = state.lastKind
	d.pluginExit = state.pluginExit
	d.pluginStopped = state.pluginStopped
	d.launchCwd = state.launchCwd
	d.hotSwapNeedsRestart = state.hotSwapNeedsRestart
	d.paused = state.paused
	d.pausing = state.pausing
	d.ended = state.ended
	d.stopFile = state.stopFile
	d.stopLine = state.stopLine
	d.stopThread = state.stopThread
	d.reason = state.reason
	d.threads = state.threads
	d.frames = state.frames
	d.selFrame = state.selFrame
	d.scopes = state.scopes
	d.vars = state.vars
	d.frameID = state.frameID
	d.varContainer = state.varContainer
	d.selVar = state.selVar
	d.output = state.output
	d.truncated = state.truncated
	d.err = state.err
	d.status = state.status
	d.watches = state.watches
	d.evalText = state.evalText
	d.evaluate = state.evaluate
	d.setValue = state.setValue
	d.smartStep = state.smartStep
	d.heap = state.heap
	d.evalBusy = state.evalBusy
}

func (d *DebugPane) rememberDebugSession() {
	if d.active == 0 && d.session != 0 {
		d.active = d.session
	}
	if d.active == 0 {
		return
	}
	state := d.captureDebugSession()
	if d.sessions == nil {
		d.sessions = map[uint32]*debugSessionState{}
	}
	d.sessions[d.active] = &state
	d.ensureDebugSessionOrder(d.active)
}

func (d *DebugPane) ensureDebugSessionOrder(id uint32) {
	if id == 0 || slices.Contains(d.sessionOrder, id) {
		return
	}
	d.sessionOrder = append(d.sessionOrder, id)
}

func (d *DebugPane) adoptDebugSession(id uint32) {
	d.active = id
	d.ensureDebugSessionOrder(id)
	if d.sessions == nil {
		d.sessions = map[uint32]*debugSessionState{}
	}
	delete(d.sessions, id)
}

func (m *Model) debugSessionState(id uint32) *debugSessionState {
	if id == 0 {
		return nil
	}
	if m.debug.active == id {
		m.debug.rememberDebugSession()
	}
	return m.debug.sessions[id]
}

// updateDebugSession applies a reply to its owning content tab. Switching tabs
// while an RPC is in flight cannot move the result into whichever tab happens
// to be selected when the reply arrives.
func (m *Model) updateDebugSession(id uint32, update func(*debugSessionState)) bool {
	if id == 0 {
		return false
	}
	tab := id
	if m.debug.active == tab {
		m.debug.rememberDebugSession()
	}
	state := m.debug.sessions[tab]
	if state == nil {
		return false
	}
	update(state)
	if m.debug.active == tab {
		m.debug.restoreDebugSession(*state)
		m.debug.active = tab
	}
	return true
}

func (m *Model) debugSessionOwns(id uint32) bool {
	if id == 0 || m.debug.retiredSessions[id] {
		return false
	}
	if m.debug.active == id && (m.debug.session == id || m.debug.starting) {
		return true
	}
	_, ok := m.debug.sessions[id]
	return ok
}

func (m *Model) debugHasLiveSessions() bool {
	if m.debug.session != 0 && !m.debug.ended {
		return true
	}
	for _, state := range m.debug.sessions {
		if state != nil && state.session != 0 && !state.ended {
			return true
		}
	}
	return false
}

// SessionSharesProcess reports whether a live debug tab still refers to the
// same started process. A Gradle or Maven fork can expose several JDWP ports,
// so ending one tab must not stop the Run process that feeds the others.
func (d *DebugPane) SessionSharesProcess(runID uint32) bool {
	if runID == 0 {
		return false
	}
	if d.session != 0 && !d.ended && d.parent == runID {
		return true
	}
	for _, state := range d.sessions {
		if state != nil && state.session != 0 && !state.ended && state.parent == runID {
			return true
		}
	}
	return false
}

func (m *Model) markDebugSessionRetired(id uint32) {
	if id == 0 {
		return
	}
	if m.debug.retiredSessions == nil {
		m.debug.retiredSessions = map[uint32]bool{}
	}
	if !m.debug.retiredSessions[id] {
		m.debug.retiredOrder = append(m.debug.retiredOrder, id)
	}
	m.debug.retiredSessions[id] = true
	for len(m.debug.retiredOrder) > 4096 {
		old := m.debug.retiredOrder[0]
		m.debug.retiredOrder = m.debug.retiredOrder[1:]
		delete(m.debug.retiredSessions, old)
	}
}

func (d *DebugPane) resetActiveDebugFields() {
	d.session = 0
	d.parent = 0
	d.adapter = ""
	d.ended = false
	d.paused = false
	d.pausing = false
	d.stopping = false
	d.stopThread = 0
	d.frameID = 0
	d.stopLine = -1
	d.stopFile = ""
	d.reason = ""
	d.frames = nil
	d.scopes = nil
	d.vars = nil
	d.threads = nil
	d.varContainer = 0
	d.selFrame = 0
	d.selVar = 0
	d.setValue = debugSetValueState{}
	d.smartStep = debugSmartStepState{}
	d.err = ""
	d.status = ""
	d.truncated = false
	d.evalBusy = false
	d.caps = openidea.DebugCapabilities{}
	d.capsKnown = false
	d.capsRequest = 0
	d.jvmBackend = openidea.JvmBackendState{}
	d.stopGeneration = 0
	d.threadRequest = 0
	d.frameRequest = 0
	d.hotSwapNeedsRestart = false
}

type debugSessionSummary struct {
	id     uint32
	name   string
	status string
	live   bool
	paused bool
	ended  bool
}

func (d *DebugPane) debugSessionSummaries() []debugSessionSummary {
	if d.active == 0 && d.session != 0 {
		d.active = d.session
	}
	out := make([]debugSessionSummary, 0, len(d.sessionOrder)+1)
	for _, id := range d.sessionOrder {
		state := d.sessions[id]
		if id == d.active {
			captured := d.captureDebugSession()
			state = &captured
		}
		if state == nil {
			continue
		}
		name := state.tabName
		if name == "" {
			name = state.config
		}
		if name == "" {
			name = "Debug"
		}
		live := state.session != 0 && !state.ended
		out = append(out, debugSessionSummary{
			id: id, name: name, status: state.status,
			live: live, paused: live && state.paused, ended: state.ended,
		})
	}
	if d.active == 0 && (d.starting || d.config != "" || d.tabName != "") {
		name := d.tabName
		if name == "" {
			name = d.config
		}
		if name == "" {
			name = "Debug"
		}
		out = append(out, debugSessionSummary{
			name: name + " …", status: d.status, live: d.starting, ended: d.ended,
		})
	}
	return out
}

func (m *Model) selectDebugSession(id uint32) {
	if id == 0 || id == m.debug.active || m.debug.starting {
		return
	}
	m.debug.rememberDebugSession()
	state := m.debug.sessions[id]
	if state == nil {
		return
	}
	m.debug.restoreDebugSession(*state)
	m.debug.active = id
	if m.debug.memory.loading {
		m.debug.memory.loading = false
	}
	if m.debug.evalBusy {
		m.debug.evalBusy = false
	}
	if m.debug.paused && len(m.debug.frames) == 0 && m.debug.stopThread != 0 {
		m.fetchStop(id, m.debug.stopThread, m.debug.stopGeneration)
	}
	m.refreshDecorations()
	m.invalidate()
}

func (m *Model) closeDebugSessionTab(id uint32) {
	if id == 0 {
		m.debug.starting = false
		if m.debug.startCancel != nil {
			m.debug.startCancel()
			m.debug.startCancel = nil
		}
		m.debug.resetActiveDebugFields()
		m.debug.tabName = ""
		m.debug.config = ""
		m.debug.output = nil
		m.debug.invalidateViews()
		if len(m.debug.sessionOrder) > 0 {
			next := m.debug.sessionOrder[len(m.debug.sessionOrder)-1]
			if state := m.debug.sessions[next]; state != nil {
				m.debug.restoreDebugSession(*state)
				m.debug.active = next
			}
		}
		m.invalidate()
		return
	}
	if id != m.debug.active {
		state := m.debug.sessions[id]
		if state != nil && state.session != 0 && !state.ended {
			m.stopBackgroundDebugSession(id, state.session)
			return
		}
		m.removeBackgroundDebugSession(id)
		m.invalidate()
		return
	}
	if m.debug.session != 0 && !m.debug.ended || m.debug.starting {
		if m.debug.closeAfterStop == nil {
			m.debug.closeAfterStop = map[uint32]bool{}
		}
		if m.debug.active != 0 {
			m.debug.closeAfterStop[m.debug.active] = true
		}
		m.StopDebug()
		return
	}
	m.debug.rememberDebugSession()
	m.debugHeapCloseSessionSnapshots(id)
	delete(m.debug.sessions, id)
	index := slices.Index(m.debug.sessionOrder, id)
	m.debug.sessionOrder = slices.DeleteFunc(m.debug.sessionOrder, func(v uint32) bool {
		return v == id
	})
	if len(m.debug.sessionOrder) == 0 {
		visible, bps, muted, watches := m.debug.visible, m.debug.bps, m.debug.muted, m.debug.watches
		m.debug = DebugPane{
			visible: visible, bps: bps, muted: muted, watches: watches,
			stopLine: -1, sessions: map[uint32]*debugSessionState{},
			closeAfterStop: map[uint32]bool{},
		}
		m.refreshDecorations()
		m.invalidate()
		return
	}
	if index >= len(m.debug.sessionOrder) {
		index = len(m.debug.sessionOrder) - 1
	}
	next := m.debug.sessionOrder[max(index, 0)]
	state := m.debug.sessions[next]
	if state == nil {
		return
	}
	m.debug.restoreDebugSession(*state)
	m.debug.active = next
	m.refreshDecorations()
	m.invalidate()
}

func (m *Model) removeBackgroundDebugSession(id uint32) {
	m.debugHeapCloseSessionSnapshots(id)
	delete(m.debug.sessions, id)
	delete(m.debug.closeAfterStop, id)
	m.debug.sessionOrder = slices.DeleteFunc(m.debug.sessionOrder, func(v uint32) bool {
		return v == id
	})
}

func (m *Model) stopBackgroundDebugSession(id, session uint32) {
	if m.debug.closeAfterStop == nil {
		m.debug.closeAfterStop = map[uint32]bool{}
	}
	if m.mgr == nil && m.invokeHook == nil {
		delete(m.debug.closeAfterStop, id)
		return
	}
	m.debug.closeAfterStop[id] = true
	if state := m.debug.sessions[id]; state != nil {
		state.stopping = true
		state.status = "Stopping the debug session…"
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "debug_stop", map[string]any{"session": session})
		m.post(func() {
			if err != nil {
				delete(m.debug.closeAfterStop, id)
				if state := m.debug.sessions[id]; state != nil {
					state.err = "Stopping the debug session failed: " + err.Error()
					state.stopping = false
				}
				m.status = "Stopping the debug session failed: " + err.Error()
				m.invalidate()
				return
			}
			m.markDebugSessionRetired(session)
			m.endDebugHeapOperation(id)
			m.removeBackgroundDebugSession(id)
			m.invalidate()
		})
	}()
}

func (d *DebugPane) invalidateViews() {
	d.console = debugConsoleView{}
	d.memory = debugMemoryView{}
	d.heap = debugHeapState{}
}

// acceptBackgroundDebugEvent updates a non-selected session's lifecycle and
// output. Frames and variables are loaded when its content tab is selected.
func (m *Model) acceptBackgroundDebugEvent(id uint32, event openidea.DebugEvent) bool {
	statePtr := m.debug.sessions[id]
	if statePtr == nil {
		return false
	}
	state := *statePtr
	if state.ended && event.Kind != "hotCodeReplace" {
		return true
	}
	switch event.Kind {
	case "started":
		state.ended = false
		state.adapter = event.Adapter
		if event.Parent != nil {
			state.parent = *event.Parent
		}
		if event.Name != "" {
			state.config = event.Name
			state.tabName = event.Name
		}
		state.status = "Debugging started"
	case "initialized":
		state.status = "Running…"
	case "stopped":
		state.stopGeneration++
		state.threadRequest++
		state.frameRequest++
		state.memory.invalidateInspection()
		state.paused = true
		state.ended = false
		state.stopThread = event.ThreadID
		state.reason = event.Reason
		state.frames = nil
		state.scopes = nil
		state.vars = nil
		state.frameID = 0
		state.varContainer = 0
		state.stopLine = -1
		state.stopFile = ""
		state.status = "Paused"
		if event.Description != "" {
			state.status = "Paused: " + event.Description
		}
	case "continued":
		state.stopGeneration++
		state.threadRequest++
		state.frameRequest++
		state.memory.invalidateInspection()
		state.paused = false
		state.setValue.open = false
		state.smartStep.open = false
		state.frames = nil
		state.scopes = nil
		state.vars = nil
		state.stopLine = -1
		state.stopFile = ""
		state.status = "Running…"
	case "output":
		text := strings.TrimSuffix(stripANSI(event.Output), "\n")
		if text != "" {
			kind := event.Category
			if kind == "" {
				kind = "console"
			}
			appendDebugOutput(&state.output, &state.truncated, text, kind)
		}
	case "terminated", "exited":
		parent := state.parent
		m.markDebugSessionRetired(id)
		request, cancelHeap := invalidateDebugHeapOperation(&state.heap)
		state.stopGeneration++
		state.threadRequest++
		state.frameRequest++
		state.memory.invalidateInspection()
		state.stopping = false
		state.ended = true
		state.paused = false
		state.setValue.open = false
		state.smartStep.open = false
		state.frames = nil
		state.scopes = nil
		state.vars = nil
		state.stopLine = -1
		state.session = 0
		state.parent = 0
		state.frameID = 0
		state.varContainer = 0
		state.status = "Debugging ended"
		if event.Kind == "exited" {
			state.status = "The program exited"
			if event.Code != nil {
				state.status = fmt.Sprintf("The program exited (code %d)", *event.Code)
			}
		}
		if state.pluginExit != nil {
			done, stopped := state.pluginExit, state.pluginStopped
			state.pluginExit = nil
			state.pluginStopped = false
			done(event.Code, stopped)
		}
		*statePtr = state
		m.releaseDebugHeapChannels(id)
		if cancelHeap {
			m.cancelDebugRequest(request)
		}
		if m.debug.closeAfterStop[id] {
			m.removeBackgroundDebugSession(id)
			m.invalidate()
			return true
		}
		if parent != 0 && !m.debug.SessionSharesProcess(parent) {
			m.debug.parent = 0
			m.stopAssociatedRun(parent)
		}
	case "hotCodeReplace":
		if event.ChangeType == "BUILD_COMPLETE" {
			m.offerHotSwapAfterBuild(id)
		} else if event.Message != "" {
			state.status = "Hot swap: " + event.Message
		}
	case "invalidated":
		if state.paused && debugInvalidatedVariables(event.Areas) {
			state.frameRequest++
			state.scopes = nil
			state.vars = nil
			state.varContainer = 0
		}
	case "jvmBackend":
		if event.JVMBackend != nil {
			state.jvmBackend = *event.JVMBackend
			switch state.jvmBackend.Kind {
			case "degraded", "closed":
				state.memory.invalidateInspection()
			}
		}
	}
	*statePtr = state
	if event.Kind == "capabilities" {
		m.refreshDebugCapabilities(id)
	}
	m.invalidate()
	return true
}

func debugInvalidatedVariables(areas []string) bool {
	if len(areas) == 0 {
		return true
	}
	for _, area := range areas {
		if area == "all" || area == "variables" {
			return true
		}
	}
	return false
}

func appendDebugOutput(lines *[]outputLine, truncated *bool, text, kind string) {
	for _, line := range strings.Split(text, "\n") {
		*lines = append(*lines, outputLine{kind: kind, text: line})
	}
	if over := len(*lines) - maxDebugLines; over > 0 {
		*lines = append((*lines)[:0], (*lines)[over:]...)
		*truncated = true
	}
}
