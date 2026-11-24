package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

type mcpReportCall struct {
	Command string
	Args    map[string]json.RawMessage
}

func decodeMCPReportCall(t *testing.T, command string, input any) mcpReportCall {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal %s arguments: %v", command, err)
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("decode %s arguments: %v", command, err)
	}
	return mcpReportCall{Command: command, Args: args}
}

func waitMCPReportCall(t *testing.T, calls <-chan mcpReportCall) mcpReportCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("MCP report did not reach the backend")
		return mcpReportCall{}
	}
}

func mcpTestEditorTab(path, text string, start, end int) *Tab {
	doc := docbridge.Virtual(path, text)
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(start, end)
	return &Tab{doc: doc, state: state}
}

func TestMCPReportsOpenAndFocusedEditorsWithUTF16Coordinates(t *testing.T) {
	main := &serialMainThread{}
	calls := make(chan mcpReportCall, 4)
	root := t.TempDir()
	first := root + "/src/One.java"
	second := root + "/src/Two.java"
	text := "a😀世界\nb"
	start := strings.Index(text, "😀")
	end := strings.Index(text, "\n")
	active := mcpTestEditorTab(first, text, start, end)
	other := mcpTestEditorTab(second, "class Two {}\n", 0, 0)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		tabs:     []*Tab{active, other},
		active:   0,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			calls <- decodeMCPReportCall(t, command, input)
			return json.RawMessage("null"), nil
		},
	}
	m.setRoot(root)
	m.mcpReports.ready = true
	m.mcpReports.windowFocused = true
	m.mcpReports.problemsSent = map[string]string{}

	main.do(func() { m.mcpFlushEditorReports() })
	var paths []string
	var focused mcpFocusedEditor
	var focusedFlag bool
	for range 2 {
		call := waitMCPReportCall(t, calls)
		switch call.Command {
		case "mcp_set_open_editors":
			if err := json.Unmarshal(call.Args["paths"], &paths); err != nil {
				t.Fatal(err)
			}
		case "mcp_set_focused_editor":
			if err := json.Unmarshal(call.Args["editor"], &focused); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(call.Args["focused"], &focusedFlag); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unexpected MCP report %q", call.Command)
		}
	}
	if got, want := paths, []string{realPath(first), realPath(second)}; len(got) != 2 ||
		got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("open editor paths = %v, want %v", got, want)
	}
	if focused.Path != realPath(first) || focused.SelectedText != "😀世界" ||
		focused.Selection.Start != (openidea.OutlinePos{Line: 0, Col: 1}) ||
		focused.Selection.End != (openidea.OutlinePos{Line: 0, Col: 5}) {
		t.Fatalf("focused editor = %+v", focused)
	}
	if !focusedFlag {
		t.Fatal("focused editor reported focused=false")
	}
}

func TestMCPSelectedTextCapsAtUTF16WithoutSplittingEmoji(t *testing.T) {
	text := strings.Repeat("😀", mcpSelectedTextLimit/2+10)
	got := mcpSelectedText(text)
	if utf16Length := len([]rune(got)) * 2; utf16Length != mcpSelectedTextLimit {
		t.Fatalf("selected text has %d UTF-16 units, want %d", utf16Length, mcpSelectedTextLimit)
	}
	if !json.Valid([]byte(`"` + got + `"`)) {
		t.Fatal("selected text ended in an invalid partial UTF-8 or surrogate")
	}
}

func TestMCPProjectReadyPrecedesInitialReports(t *testing.T) {
	main := &serialMainThread{}
	calls := make(chan mcpReportCall, 8)
	root := t.TempDir()
	tab := mcpTestEditorTab(root+"/src/Main.java", "class Main {}\n", 0, 0)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		tabs:     []*Tab{tab},
		active:   0,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			calls <- decodeMCPReportCall(t, command, input)
			return json.RawMessage("null"), nil
		},
	}
	m.setRoot(root)
	m.mcpReports.editorDelay = 5 * time.Millisecond
	m.mcpReports.problemsDelay = 5 * time.Millisecond

	main.do(func() { m.mcpProjectReady() })
	first := waitMCPReportCall(t, calls)
	if first.Command != "mcp_ui_ready" {
		t.Fatalf("first MCP call after project open = %q", first.Command)
	}
	var ready bool
	if err := json.Unmarshal(first.Args["ready"], &ready); err != nil || !ready {
		t.Fatalf("ready argument = %v, %v", first.Args["ready"], err)
	}
	seen := map[string]bool{}
	for len(seen) < 3 {
		call := waitMCPReportCall(t, calls)
		switch call.Command {
		case "mcp_set_open_editors", "mcp_set_focused_editor", "mcp_report_problems":
			seen[call.Command] = true
		default:
			t.Fatalf("unexpected initial MCP call %q", call.Command)
		}
	}
	main.do(func() { m.mcpProjectClosed() })
}

func TestMCPReportsProblemsResetChangesAndRemovedFiles(t *testing.T) {
	main := &serialMainThread{}
	calls := make(chan mcpReportCall, 4)
	root := t.TempDir()
	first := root + "/src/One.java"
	second := root + "/src/Two.java"
	warning := openidea.Diagnostic{
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 1, Col: 2},
			End:   openidea.OutlinePos{Line: 1, Col: 5},
		},
		Severity: "warning",
		Message:  "first warning",
	}
	errorItem := openidea.Diagnostic{
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 2, Col: 0},
			End:   openidea.OutlinePos{Line: 2, Col: 4},
		},
		Severity: "error",
		Message:  "second error",
	}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		diags: map[string]map[string][]openidea.Diagnostic{
			first:  {"java": {warning}},
			second: {"java": {errorItem}},
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			calls <- decodeMCPReportCall(t, command, input)
			return json.RawMessage("null"), nil
		},
	}
	m.setRoot(root)
	m.mcpReports.ready = true
	m.mcpReports.problemsReset = true
	m.mcpReports.problemsSent = map[string]string{}

	main.do(func() { m.mcpFlushProblemsReport() })
	call := waitMCPReportCall(t, calls)
	if call.Command != "mcp_report_problems" {
		t.Fatalf("first Problems report = %q", call.Command)
	}
	var reset bool
	var files []mcpFileProblems
	if err := json.Unmarshal(call.Args["reset"], &reset); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call.Args["files"], &files); err != nil {
		t.Fatal(err)
	}
	if !reset || len(files) != 2 || files[0].Path != first || files[1].Path != second {
		t.Fatalf("first Problems snapshot reset=%v files=%+v", reset, files)
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return !m.mcpReports.problemsReset && !m.mcpReports.problemsInFlight
	})

	main.do(func() {
		delete(m.diags, second)
		m.mcpFlushProblemsReport()
	})
	call = waitMCPReportCall(t, calls)
	if err := json.Unmarshal(call.Args["reset"], &reset); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call.Args["files"], &files); err != nil {
		t.Fatal(err)
	}
	if reset || len(files) != 1 || files[0].Path != second || len(files[0].Items) != 0 {
		t.Fatalf("removed Problems snapshot reset=%v files=%+v", reset, files)
	}
}

func TestMCPReportsIgnoreAStaleReplyAfterAProjectSwitch(t *testing.T) {
	main := &serialMainThread{}
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	newRequestFinished := make(chan struct{}, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		diags: map[string]map[string][]openidea.Diagnostic{
			"/old/src/A.java": {"java": {{
				Range: openidea.OutlineRange{}, Severity: "error", Message: "old",
			}}},
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "mcp_report_problems" {
				return json.RawMessage("null"), nil
			}
			raw, _ := json.Marshal(input)
			if !strings.Contains(string(raw), "/old/") {
				newRequestFinished <- struct{}{}
				return json.RawMessage("null"), nil
			}
			entered <- struct{}{}
			<-release
			return nil, errors.New("old project backend unavailable")
		},
	}
	m.setRoot(t.TempDir())
	m.mcpReports.ready = true
	m.mcpReports.problemsReset = true
	m.mcpReports.problemsSent = map[string]string{}
	m.mcpReports.problemsDelay = 10 * time.Millisecond

	main.do(func() { m.mcpFlushProblemsReport() })
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first Problems report did not start")
	}
	main.do(func() {
		m.mcpStopReports()
		m.setRoot(t.TempDir())
		m.diags = map[string]map[string][]openidea.Diagnostic{}
		m.mcpStartReports()
	})
	select {
	case <-newRequestFinished:
	case <-time.After(2 * time.Second):
		t.Fatal("new project's Problems report did not start")
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return !m.mcpReports.problemsReset && !m.mcpReports.problemsInFlight
	})
	close(release)
	time.Sleep(40 * time.Millisecond)
	main.do(func() {
		if m.mcpReports.problemsReset || len(m.mcpReports.problemsSent) != 0 {
			t.Fatalf("a stale reply changed the new project's reports: %+v", m.mcpReports)
		}
	})
	m.mcpStopReports()
}
