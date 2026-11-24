package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type notebookTestCall struct {
	command string
	args    map[string]any
}

func notebookSnapshot(path string) openidea.NotebookSnapshot {
	count := uint32(1)
	return openidea.NotebookSnapshot{
		ID: "notebook-1", Path: path, Version: 1, Dirty: false, CanUndo: false, CanRedo: false,
		NBFormat: 4, NBFormatMinor: 5,
		Metadata: json.RawMessage(`{}`),
		Cells: []openidea.NotebookCell{
			{
				ID: "code", CellType: "code", Source: "print('hello')",
				Metadata: json.RawMessage(`{}`), ExecutionCount: &count,
				Outputs: []json.RawMessage{
					json.RawMessage(`{"output_type":"stream","name":"stdout","text":"hello\n"}`),
				},
				Attachments: json.RawMessage(`{}`),
			},
			{
				ID: "text", CellType: "markdown", Source: "# Notes\n\n- first",
				Metadata: json.RawMessage(`{}`), Attachments: json.RawMessage(`{}`),
			},
		},
	}
}

func TestNotebookOpenRoutesLoadsAndRendersNativeCells(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	snapshot := notebookSnapshot(path)
	var mu sync.Mutex
	var calls []notebookTestCall
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			mu.Lock()
			calls = append(calls, notebookTestCall{command: command, args: args})
			mu.Unlock()
			switch command {
			case "notebook_open":
				return json.Marshal(snapshot)
			case "sdk_detect":
				return json.Marshal([]openidea.Sdk{{Path: "/venv/python", Name: "Python 3.13"}})
			case "sdk_python_effective":
				return json.Marshal("/venv/python")
			case "notebook_cell_docs":
				return json.Marshal([]openidea.NotebookCellDoc{})
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	m.prefs.FontSize = 13

	m.openNotebook(path, openSpec{at: -1})
	waitFor(t, "the notebook to load", func() bool {
		session := m.notebooks.sessions[path]
		return session != nil && session.loaded
	})
	if len(m.tabs) != 1 || m.tabs[0].kind != tabKindNotebook ||
		m.tabs[0].pagePath != path || m.tabs[0].pageTitle != "demo.ipynb" {
		t.Fatalf("notebook tab = %+v", m.tabs)
	}
	if got := m.notebooks.sessions[path].python; got != "/venv/python" {
		t.Fatalf("python = %q", got)
	}
	mu.Lock()
	commands := make([]string, 0, len(calls))
	for _, call := range calls {
		commands = append(commands, call.command)
	}
	mu.Unlock()
	if !reflect.DeepEqual(commands, []string{"notebook_open", "sdk_detect", "sdk_python_effective", "notebook_cell_docs"}) {
		t.Fatalf("commands = %v", commands)
	}

	tester := ui.NewTester(func(c *ui.Context) {
		m.notebookPage(c, c.Theme(), m.tabs[0])
	}, 1000, 760)
	allText := strings.Join(tester.Texts(), "\n")
	for _, text := range []string{"Run Cell", "Run All", "Python 3.13", "Notebook cell source", "hello\n", "Notes", "first"} {
		if !strings.Contains(allText, text) {
			t.Fatalf("notebook view lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestNotebookEditsUseVersionAndPreservePendingSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	snapshot := notebookSnapshot(path)
	calls := make(chan notebookTestCall, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args := input.(map[string]any)
			if command == "notebook_cell_docs" {
				return json.Marshal([]openidea.NotebookCellDoc{})
			}
			calls <- notebookTestCall{command: command, args: args}
			snapshot.Version = 2
			snapshot.Dirty = true
			snapshot.Cells[0].Source = "print('new')"
			return json.Marshal(snapshot)
		},
	}
	m.setRoot(root)
	session := m.notebook(path)
	m.acceptNotebookSnapshot(session, snapshot)

	m.notebookCellChanged(session, "code", "print('new')")
	m.flushNotebookCell(session, "code")
	call := <-calls
	if call.command != "notebook_edit" || call.args["id"] != "notebook-1" ||
		call.args["version"] != uint32(1) {
		t.Fatalf("edit call = %+v", call)
	}
	edit, ok := call.args["edit"].(openidea.NotebookEdit)
	if !ok || edit.Type != "setSource" || edit.Cell != "code" || edit.Source != "print('new')" {
		t.Fatalf("edit = %#v", call.args["edit"])
	}
	waitFor(t, "the edit reply", func() bool { return !session.editBusy })
	if session.snapshot.Version != 2 || session.snapshot.Cells[0].Source != "print('new')" {
		t.Fatalf("snapshot = %+v", session.snapshot)
	}
}

func TestNotebookExecuteInterruptAndStreamingOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	snapshot := notebookSnapshot(path)
	calls := make(chan notebookTestCall, 4)
	releaseExecute := make(chan struct{})
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			if command == "notebook_cell_docs" {
				return json.Marshal([]openidea.NotebookCellDoc{})
			}
			calls <- notebookTestCall{command: command, args: args}
			switch command {
			case "notebook_execute":
				<-releaseExecute
				return json.Marshal(snapshot)
			case "notebook_interrupt", "cancel_request":
				return json.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	session := m.notebook(path)
	m.acceptNotebookSnapshot(session, snapshot)
	session.python = "/venv/python"
	m.executeNotebook(session, []string{"code"})
	call := <-calls
	if call.command != "notebook_execute" || call.args["python"] != "/venv/python" {
		t.Fatalf("execute call = %+v", call)
	}
	if got := call.args["cells"]; !reflect.DeepEqual(got, []string{"code"}) {
		t.Fatalf("cells = %#v", got)
	}
	channel, _ := call.args["onOutput"].(map[string]any)["$channel"].(uint32)
	if channel == 0 || call.args["request"] == uint32(0) {
		t.Fatalf("stream ids = %#v", call.args)
	}
	m.acceptNotebookChunk(channel, json.RawMessage(
		`{"id":"notebook-1","path":"`+strings.ReplaceAll(path, `\`, `\\`)+`","cell":"code","kind":"stream","content":{"text":"live\n"}}`,
	))
	if got := session.streaming["code"]; got != "live\n" {
		t.Fatalf("streaming output = %q", got)
	}
	close(releaseExecute)
	waitFor(t, "execution to finish", func() bool { return !session.running })
	m.interruptNotebook(session)
	call = <-calls
	if call.command != "notebook_interrupt" && call.command != "cancel_request" {
		t.Fatalf("interrupt call = %+v", call)
	}
	if call.command == "cancel_request" {
		call = <-calls
	}
	if call.command != "notebook_interrupt" {
		t.Fatalf("interrupt call = %+v", call)
	}
}

func TestNotebookDirtyCloseKeepsTabUntilConfirmation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	closed := make(chan notebookTestCall, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "notebook_cell_docs" {
				return json.Marshal([]openidea.NotebookCellDoc{})
			}
			closed <- notebookTestCall{command: command, args: input.(map[string]any)}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(root)
	session := m.notebook(path)
	m.acceptNotebookSnapshot(session, notebookSnapshot(path))
	tab := m.newNotebookTab(path, "demo.ipynb", false)
	tab.pageDirty = true
	m.tabs, m.active = []*Tab{tab}, 0

	m.requestCloseNotebook(tab)
	if !session.closeAsk || len(m.tabs) != 1 {
		t.Fatalf("dirty close = ask %v tabs %d", session.closeAsk, len(m.tabs))
	}
	m.confirmCloseNotebook(false)
	if len(m.tabs) != 0 {
		t.Fatalf("tabs after discard = %d", len(m.tabs))
	}
	call := <-closed
	if call.command != "notebook_close" || call.args["id"] != "notebook-1" {
		t.Fatalf("close call = %+v", call)
	}
}

func TestNotebookSaveAndCloseFlushesPendingSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	snapshot := notebookSnapshot(path)
	calls := make(chan notebookTestCall, 8)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			switch command {
			case "notebook_cell_docs":
				return json.Marshal([]openidea.NotebookCellDoc{})
			case "notebook_edit":
				calls <- notebookTestCall{command: command, args: args}
				snapshot.Version = 2
				snapshot.Dirty = true
				snapshot.Cells[0].Source = "print('new')"
				return json.Marshal(snapshot)
			case "notebook_save":
				calls <- notebookTestCall{command: command, args: args}
				snapshot.Version = 3
				snapshot.Dirty = false
				return json.Marshal(snapshot)
			case "notebook_close":
				calls <- notebookTestCall{command: command, args: args}
				return json.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	session := m.notebook(path)
	m.acceptNotebookSnapshot(session, snapshot)
	tab := m.newNotebookTab(path, "demo.ipynb", false)
	tab.pageDirty = true
	m.tabs, m.active = []*Tab{tab}, 0
	session.pending["code"] = "print('new')"

	m.confirmCloseNotebook(true)
	waitFor(t, "save and close to finish", func() bool { return len(m.tabs) == 0 })

	var commands []string
	for len(calls) > 0 {
		commands = append(commands, (<-calls).command)
	}
	if !reflect.DeepEqual(commands, []string{"notebook_edit", "notebook_save", "notebook_close"}) {
		t.Fatalf("save and close commands = %v", commands)
	}
	if len(session.pending) != 0 || session.closeAfter {
		t.Fatalf("save and close state: pending=%v closeAfter=%v", session.pending, session.closeAfter)
	}
}

func TestNotebookSaveAndCloseWaitsForRunningExecution(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	snapshot := notebookSnapshot(path)
	snapshot.Dirty = true
	releaseExecute := make(chan struct{})
	saved := make(chan struct{}, 1)
	closed := make(chan struct{}, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "notebook_execute":
				<-releaseExecute
				return json.Marshal(snapshot)
			case "notebook_save":
				snapshot.Dirty = false
				saved <- struct{}{}
				return json.Marshal(snapshot)
			case "notebook_close":
				closed <- struct{}{}
				return json.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	session := m.notebook(path)
	m.acceptNotebookSnapshot(session, snapshot)
	tab := m.newNotebookTab(path, "demo.ipynb", false)
	tab.pageDirty = true
	m.tabs, m.active = []*Tab{tab}, 0

	m.executeNotebook(session, []string{"code"})
	waitFor(t, "execution to start", func() bool { return session.running })
	m.confirmCloseNotebook(true)
	if !session.closeAfter || len(m.tabs) != 1 {
		t.Fatalf("running close state = closeAfter %v tabs %d", session.closeAfter, len(m.tabs))
	}

	close(releaseExecute)
	select {
	case <-saved:
	case <-time.After(2 * time.Second):
		t.Fatal("save did not resume after execution finished")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("notebook did not close after saving")
	}
	waitFor(t, "the saved notebook tab to close", func() bool { return len(m.tabs) == 0 })
}

func TestNotebookOutputParsingAndKernelEvent(t *testing.T) {
	text, isError := notebookPersistedOutput(json.RawMessage(
		`{"output_type":"error","ename":"ValueError","evalue":"bad","traceback":["line 1","line 2"]}`,
	))
	if text != "line 1\nline 2" || !isError {
		t.Fatalf("error output = %q %v", text, isError)
	}
	text, isError = notebookPersistedOutput(json.RawMessage(
		`{"output_type":"execute_result","data":{"text/plain":"42"}}`,
	))
	if text != "42" || isError {
		t.Fatalf("result output = %q %v", text, isError)
	}
	if got := notebookKernelOutputText(json.RawMessage(`{"name":"stdout","text":["a","b"]}`)); got != "ab" {
		t.Fatalf("kernel text = %q", got)
	}

	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	m := &Model{ctx: context.Background(), notebooks: notebookState{sessions: map[string]*notebookSession{}}}
	session := m.notebook(path)
	m.acceptNotebookSnapshot(session, notebookSnapshot(path))
	m.acceptNotebookKernelEvent(openidea.NotebookKernelEvent{
		ID: "notebook-1", Path: path, State: "dead", Message: "kernel exited",
	})
	if session.kernel != "dead" || session.error != "kernel exited" {
		t.Fatalf("kernel state = %q error %q", session.kernel, session.error)
	}
}

func TestNotebookCellDocsDriveLanguageServicesAndDiagnostics(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	snapshot := notebookSnapshot(path)
	snapshot.Cells[0].Source = "😀 name"
	var completionArgs map[string]any
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			switch command {
			case "notebook_cell_docs":
				return json.Marshal([]openidea.NotebookCellDoc{{
					Cell: "code", Doc: 41,
					URI: "vscode-notebook-cell:/demo.ipynb#code",
				}})
			case "lsp_completion":
				completionArgs = args
				return json.Marshal(openidea.CompletionList{Items: []openidea.CompletionItem{{
					Label: "name", InsertText: "name",
				}}})
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	session := m.notebook(path)
	m.acceptNotebookSnapshot(session, snapshot)
	waitFor(t, "cell documents", func() bool { return session.cellDocs["code"].Doc == 41 })

	cell := &session.snapshot.Cells[0]
	view := m.notebookCellEditorView(session, cell)
	m.requestNotebookCompletion(session, view, session.cellDocs["code"], editor.CompletionRequest{
		Offset: len("😀 na"), Prefix: "na", Version: view.state.Buffer.Version(),
	})
	waitFor(t, "completion reply", func() bool { return completionArgs != nil })
	if completionArgs["doc"] != uint32(41) {
		t.Fatalf("completion doc = %#v", completionArgs)
	}
	pos, _ := completionArgs["pos"].(map[string]any)
	if pos["line"] != uint32(0) || pos["col"] != uint32(5) {
		t.Fatalf("completion position = %#v", completionArgs["pos"])
	}

	m.applyDiagnostics(openidea.DiagnosticsEvent{
		Path: "vscode-notebook-cell:/demo.ipynb#code", Source: "pyright",
		Items: []openidea.Diagnostic{{
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: 0, Col: 3},
				End:   openidea.OutlinePos{Line: 0, Col: 7},
			},
			Severity: "warning", Message: "unknown name",
		}},
	})
	diagnostics := view.state.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Start != len("😀 ") || diagnostics[0].Severity != editor.SeverityWarning {
		t.Fatalf("cell diagnostics = %+v", diagnostics)
	}
}

func TestNotebookTabsAreSavedAndFocusedInTheSession(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "analysis.ipynb")
	if err := os.WriteFile(path, []byte(`{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Model{notebooks: notebookState{sessions: make(map[string]*notebookSession)}}
	m.setRoot(root)
	m.prefs.FontSize = 13
	tab := m.newNotebookTab(path, "analysis.ipynb", false)
	tab.pinned = true
	m.tabs, m.active = []*Tab{tab}, 0

	session, ok := m.sessionSnapshot()
	if !ok {
		t.Fatal("notebook-only session was not saved")
	}
	resolved := realPath(path)
	if !reflect.DeepEqual(session.Files, []string{resolved}) || session.Focus != resolved ||
		!reflect.DeepEqual(session.Pinned, []string{resolved}) {
		t.Fatalf("notebook session = %+v", session)
	}
}
