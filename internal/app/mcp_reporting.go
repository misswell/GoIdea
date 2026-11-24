package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"mayoi/internal/openidea"
)

const (
	mcpEditorReportDelay   = 300 * time.Millisecond
	mcpProblemsReportDelay = time.Second
	mcpSelectedTextLimit   = 65536
)

// mcpReportsState mirrors the two window snapshots OpenIDEA reports to its
// built-in MCP server. All fields are accessed on the native main thread.
type mcpReportsState struct {
	ready         bool
	windowFocused bool
	epoch         uint64

	editorTimer      *time.Timer
	editorGeneration uint64
	editorObserved   string
	pathsSent        string
	editorSent       string
	pathsInFlight    bool
	editorInFlight   bool
	editorDelay      time.Duration

	problemsTimer      *time.Timer
	problemsGeneration uint64
	problemsReset      bool
	problemsSent       map[string]string
	problemsInFlight   bool
	problemsDelay      time.Duration
}

type mcpFocusedEditor struct {
	Path         string                `json:"path"`
	Selection    openidea.OutlineRange `json:"selection"`
	SelectedText string                `json:"selectedText"`
}

type mcpFileProblems struct {
	Path  string                `json:"path"`
	Items []openidea.Diagnostic `json:"items"`
}

func (s *mcpReportsState) begin(projectsRoot string) {
	s.stopTimers()
	s.epoch++
	s.ready = projectsRoot != ""
	s.editorObserved = ""
	s.pathsSent = ""
	s.editorSent = ""
	s.pathsInFlight = false
	s.editorInFlight = false
	s.problemsReset = true
	s.problemsSent = map[string]string{}
	s.problemsInFlight = false
}

func (s *mcpReportsState) stop() {
	s.stopTimers()
	s.epoch++
	s.ready = false
	s.editorObserved = ""
	s.pathsSent = ""
	s.editorSent = ""
	s.pathsInFlight = false
	s.editorInFlight = false
	s.problemsReset = true
	s.problemsSent = nil
	s.problemsInFlight = false
}

func (s *mcpReportsState) stopTimers() {
	if s.editorTimer != nil {
		s.editorTimer.Stop()
		s.editorTimer = nil
	}
	if s.problemsTimer != nil {
		s.problemsTimer.Stop()
		s.problemsTimer = nil
	}
}

func (s *mcpReportsState) editorWait() time.Duration {
	if s.editorDelay > 0 {
		return s.editorDelay
	}
	return mcpEditorReportDelay
}

func (s *mcpReportsState) problemsWait() time.Duration {
	if s.problemsDelay > 0 {
		return s.problemsDelay
	}
	return mcpProblemsReportDelay
}

// mcpStartReports starts the initial snapshots only after mcp_ui_ready has
// succeeded, matching OpenIDEA's ideEventsReady().then chain.
func (m *Model) mcpStartReports() {
	m.mcpReports.begin(m.Root())
	if !m.mcpReports.ready {
		return
	}
	m.mcpScheduleEditorReport()
	m.mcpScheduleProblemsReport()
}

func (m *Model) mcpStopReports() {
	m.mcpReports.stop()
}

func (m *Model) mcpSetWindowFocused(focused bool) {
	m.mcpReports.windowFocused = focused
	if m.mcpReports.ready {
		m.mcpFlushEditorReports()
	}
}

// mcpObserveEditorState is called once per native frame. MyGo redraws for
// tab, caret and selection changes, so this observes them without adding
// reporting calls to every editing command.
func (m *Model) mcpObserveEditorState() {
	if !m.mcpReports.ready || m.Root() == "" {
		return
	}
	paths, focused := m.mcpEditorSnapshot()
	key := mcpEditorObservationKey(m.Root(), paths, focused, m.mcpReports.windowFocused)
	if key == m.mcpReports.editorObserved {
		return
	}
	m.mcpReports.editorObserved = key
	m.mcpScheduleEditorReport()
}

func (m *Model) mcpScheduleEditorReport() {
	if !m.mcpReports.ready {
		return
	}
	if m.mcpReports.editorTimer != nil {
		m.mcpReports.editorTimer.Stop()
	}
	m.mcpReports.editorGeneration++
	generation := m.mcpReports.editorGeneration
	epoch := m.mcpReports.epoch
	m.mcpReports.editorTimer = time.AfterFunc(m.mcpReports.editorWait(), func() {
		m.post(func() {
			if m.mcpReports.ready &&
				m.mcpReports.epoch == epoch &&
				m.mcpReports.editorGeneration == generation {
				m.mcpReports.editorTimer = nil
				m.mcpFlushEditorReports()
			}
		})
	})
}

func (m *Model) mcpFlushEditorReports() {
	if !m.mcpReports.ready || !mcpReportsHasBackend(m) || m.Root() == "" {
		return
	}
	paths, focused := m.mcpEditorSnapshot()
	pathsKey := mcpOpenEditorsKey(m.Root(), paths)
	windowFocused := m.mcpReports.windowFocused
	editorKey := mcpFocusedEditorKey(m.Root(), focused, windowFocused)
	epoch := m.mcpReports.epoch

	if pathsKey != m.mcpReports.pathsSent && !m.mcpReports.pathsInFlight {
		m.mcpReports.pathsInFlight = true
		sentKey := pathsKey
		go func() {
			ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
			defer cancel()
			_, err := m.invoke(ctx, "mcp_set_open_editors", map[string]any{"paths": paths})
			m.post(func() {
				if m.mcpReports.epoch != epoch || !m.mcpReports.ready {
					return
				}
				m.mcpReports.pathsInFlight = false
				if err != nil {
					m.mcpReports.pathsSent = ""
					m.mcpScheduleEditorReport()
					return
				}
				m.mcpReports.pathsSent = sentKey
				if m.mcpCurrentEditorKeysChanged(pathsKey, editorKey) {
					m.mcpScheduleEditorReport()
				}
			})
		}()
	}

	if editorKey != m.mcpReports.editorSent && !m.mcpReports.editorInFlight {
		m.mcpReports.editorInFlight = true
		sentKey := editorKey
		go func() {
			ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
			defer cancel()
			_, err := m.invoke(ctx, "mcp_set_focused_editor", map[string]any{
				"editor":  focused,
				"focused": windowFocused,
			})
			m.post(func() {
				if m.mcpReports.epoch != epoch || !m.mcpReports.ready {
					return
				}
				m.mcpReports.editorInFlight = false
				if err != nil {
					m.mcpReports.editorSent = ""
					m.mcpScheduleEditorReport()
					return
				}
				m.mcpReports.editorSent = sentKey
				if m.mcpCurrentEditorKeysChanged(pathsKey, editorKey) {
					m.mcpScheduleEditorReport()
				}
			})
		}()
	}
}

func (m *Model) mcpCurrentEditorKeysChanged(pathsKey, editorKey string) bool {
	paths, focused := m.mcpEditorSnapshot()
	return pathsKey != mcpOpenEditorsKey(m.Root(), paths) ||
		editorKey != mcpFocusedEditorKey(m.Root(), focused, m.mcpReports.windowFocused)
}

func (m *Model) mcpEditorSnapshot() ([]string, *mcpFocusedEditor) {
	var paths []string
	seen := map[string]bool{}
	for _, tab := range m.allTabs() {
		if !mcpTextEditorTab(tab) {
			continue
		}
		path := realPath(tab.doc.Path())
		if seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}

	tab := m.tab()
	if !mcpTextEditorTab(tab) {
		return paths, nil
	}
	selection := tab.state.Selection()
	start, end := selection.Anchor, selection.Head
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	buffer := tab.state.Buffer
	if end > buffer.Len() {
		end = buffer.Len()
	}
	if start > end {
		start = end
	}
	return paths, &mcpFocusedEditor{
		Path: realPath(tab.doc.Path()),
		Selection: openidea.OutlineRange{
			Start: tab.doc.UTF16Pos(start),
			End:   tab.doc.UTF16Pos(end),
		},
		SelectedText: mcpSelectedText(buffer.Slice(start, end)),
	}
}

func mcpTextEditorTab(tab *Tab) bool {
	return tab != nil && tab.doc != nil && tab.state != nil && tab.state.Buffer != nil &&
		tab.kind == "" && tab.viewer == nil
}

func mcpOpenEditorsKey(root string, paths []string) string {
	raw, _ := json.Marshal([]any{root, paths})
	return string(raw)
}

func mcpFocusedEditorKey(root string, focused *mcpFocusedEditor, windowFocused bool) string {
	raw, _ := json.Marshal([]any{root, focused, windowFocused})
	return string(raw)
}

func mcpEditorObservationKey(
	root string,
	paths []string,
	focused *mcpFocusedEditor,
	windowFocused bool,
) string {
	return mcpOpenEditorsKey(root, paths) + "\x00" +
		mcpFocusedEditorKey(root, focused, windowFocused)
}

func mcpSelectedText(text string) string {
	if len(text) == 0 {
		return ""
	}
	units := 0
	for i, r := range text {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > mcpSelectedTextLimit {
			return text[:i]
		}
		units += width
	}
	return text
}

// mcpDiagnosticsChanged follows OpenIDEA's one-second Problems reporter. The
// first snapshot resets the backend, later snapshots carry only changed or
// removed files.
func (m *Model) mcpDiagnosticsChanged() {
	if m.mcpReports.ready {
		m.mcpScheduleProblemsReport()
	}
}

func (m *Model) mcpScheduleProblemsReport() {
	if !m.mcpReports.ready {
		return
	}
	if m.mcpReports.problemsTimer != nil {
		m.mcpReports.problemsTimer.Stop()
	}
	m.mcpReports.problemsGeneration++
	generation := m.mcpReports.problemsGeneration
	epoch := m.mcpReports.epoch
	m.mcpReports.problemsTimer = time.AfterFunc(m.mcpReports.problemsWait(), func() {
		m.post(func() {
			if m.mcpReports.ready &&
				m.mcpReports.epoch == epoch &&
				m.mcpReports.problemsGeneration == generation {
				m.mcpReports.problemsTimer = nil
				m.mcpFlushProblemsReport()
			}
		})
	})
}

func (m *Model) mcpFlushProblemsReport() {
	if !m.mcpReports.ready || !mcpReportsHasBackend(m) || m.Root() == "" ||
		m.mcpReports.problemsInFlight {
		return
	}
	files, current := m.mcpProblemSnapshot()
	reset := m.mcpReports.problemsReset
	if !reset && len(files) == 0 {
		return
	}
	m.mcpReports.problemsInFlight = true
	epoch := m.mcpReports.epoch
	sentReset := reset
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "mcp_report_problems", map[string]any{
			"reset": sentReset,
			"files": files,
		})
		m.post(func() {
			if m.mcpReports.epoch != epoch || !m.mcpReports.ready {
				return
			}
			m.mcpReports.problemsInFlight = false
			if err != nil {
				m.mcpReports.problemsReset = true
				m.mcpReports.problemsSent = map[string]string{}
				m.mcpScheduleProblemsReport()
				return
			}
			m.mcpReports.problemsReset = false
			m.mcpReports.problemsSent = current
			if _, latest := m.mcpProblemSnapshot(); mcpProblemFingerprintsDiffer(current, latest) {
				m.mcpScheduleProblemsReport()
			}
		})
	}()
}

func (m *Model) mcpProblemSnapshot() ([]mcpFileProblems, map[string]string) {
	paths := make([]string, 0, len(m.diags))
	for path := range m.diags {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	files := make([]mcpFileProblems, 0, len(paths))
	current := make(map[string]string, len(paths))
	for _, path := range paths {
		perSource := m.diags[path]
		sources := make([]string, 0, len(perSource))
		for source := range perSource {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		var items []openidea.Diagnostic
		for _, source := range sources {
			items = append(items, perSource[source]...)
		}
		if len(items) == 0 {
			continue
		}
		raw, _ := json.Marshal(items)
		fingerprint := string(raw)
		current[path] = fingerprint
		reported, ok := m.mcpReports.problemsSent[path]
		if m.mcpReports.problemsReset || !ok || reported != fingerprint {
			files = append(files, mcpFileProblems{Path: path, Items: items})
		}
	}
	if !m.mcpReports.problemsReset {
		removed := make([]string, 0, len(m.mcpReports.problemsSent))
		for path := range m.mcpReports.problemsSent {
			if _, ok := current[path]; !ok {
				removed = append(removed, path)
			}
		}
		sort.Strings(removed)
		for _, path := range removed {
			files = append(files, mcpFileProblems{Path: path, Items: []openidea.Diagnostic{}})
		}
	}
	return files, current
}

func mcpProblemFingerprintsDiffer(a, b map[string]string) bool {
	if len(a) != len(b) {
		return true
	}
	for path, fingerprint := range a {
		if b[path] != fingerprint {
			return true
		}
	}
	return false
}

func mcpReportsHasBackend(m *Model) bool {
	return m.mgr != nil || m.invokeHook != nil
}

// SmokeMCPReports proves the three window snapshots over a real
// openidea-server. The stdio MCP bridge's confirmation round trip is covered
// separately by the Rust runtime integration test.
func (m *Model) SmokeMCPReports(ctx context.Context) (map[string]any, error) {
	tab := m.tab()
	if tab == nil || tab.doc == nil || tab.state == nil {
		return nil, errors.New("MCP smoke needs an active editor")
	}
	if _, err := m.invoke(ctx, "project_trust_set", map[string]any{
		"path": m.Root(), "trusted": true, "folder": true,
	}); err != nil {
		return nil, fmt.Errorf("trust project for MCP smoke: %w", err)
	}
	m.post(func() {
		m.run.trusted = true
		m.clearTrustPrompt()
	})
	paths, focused := m.mcpEditorSnapshot()
	path := tab.doc.Path()
	if _, err := m.invoke(ctx, "mcp_set_open_editors", map[string]any{"paths": paths}); err != nil {
		return nil, fmt.Errorf("mcp_set_open_editors: %w", err)
	}
	if _, err := m.invoke(ctx, "mcp_set_focused_editor", map[string]any{
		"editor":  focused,
		"focused": true,
	}); err != nil {
		return nil, fmt.Errorf("mcp_set_focused_editor: %w", err)
	}
	diagnostic := openidea.Diagnostic{
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 0, Col: 0},
			End:   openidea.OutlinePos{Line: 0, Col: 1},
		},
		Severity: "warning",
		Message:  "GoIDEA MCP smoke",
	}
	if _, err := m.invoke(ctx, "mcp_report_problems", map[string]any{
		"reset": true,
		"files": []mcpFileProblems{{Path: path, Items: []openidea.Diagnostic{diagnostic}}},
	}); err != nil {
		return nil, fmt.Errorf("mcp_report_problems reset: %w", err)
	}
	if _, err := m.invoke(ctx, "mcp_report_problems", map[string]any{
		"reset": false,
		"files": []mcpFileProblems{{Path: path, Items: []openidea.Diagnostic{}}},
	}); err != nil {
		return nil, fmt.Errorf("mcp_report_problems removal: %w", err)
	}
	return map[string]any{
		"editors":  len(paths) > 0,
		"focused":  focused != nil && focused.Path != "",
		"problems": true,
		"removed":  true,
	}, nil
}
