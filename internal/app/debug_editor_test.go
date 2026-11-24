package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func debugEditorTestTab(text string) *Tab {
	doc := docbridge.Virtual("/src/Example.java", text)
	return &Tab{
		doc:   doc,
		state: &editor.State{Buffer: doc.Buffer()},
	}
}

func TestDebugExpressionAtFindsDottedMemberAndSelection(t *testing.T) {
	text := "int result = order.items[index];\n"
	tab := debugEditorTestTab(text)
	off := strings.Index(text, "items") + 2
	got, ok := debugExpressionAt(tab, off)
	if !ok || got.Text != "order.items" || got.Start != strings.Index(text, "order") ||
		got.End != strings.Index(text, "order")+len("order.items") {
		t.Fatalf("member expression = %#v, %v", got, ok)
	}

	start := strings.Index(text, "index")
	tab.state.Select(start, start+len("index"))
	got, ok = debugExpressionAt(tab, start+1)
	if !ok || got.Text != "index" || got.Start != start || got.End != start+len("index") {
		t.Fatalf("selected expression = %#v, %v", got, ok)
	}
}

func TestDebugExpressionAtRejectsNumbersAndMultilineSelection(t *testing.T) {
	text := "return 123;\nnext();\n"
	tab := debugEditorTestTab(text)
	if got, ok := debugExpressionAt(tab, strings.Index(text, "123")+1); ok {
		t.Fatalf("numeric literal evaluated as %#v", got)
	}
	tab.state.Select(0, strings.Index(text, "next")+2)
	if got, ok := debugExpressionAt(tab, 1); ok {
		t.Fatalf("multiline selection evaluated as %#v", got)
	}
}

func TestDebugInlineInlaysShowUniqueLocalsAtLineEnds(t *testing.T) {
	text := "int total = count;\nlog(count, total, count, missing);\nafter();\n"
	tab := debugEditorTestTab(text)
	vars := []openidea.Variable{
		{Name: "count", Value: "42"},
		{Name: "total", Value: "40"},
		{Name: "missing", Value: "null"},
	}
	got := debugInlineInlays(tab, true, tab.doc.Path(), 1, vars)
	if len(got) != 2 {
		t.Fatalf("inlays = %#v, want values on the two visible source lines", got)
	}
	if got[0].Offset != len("int total = count;") ||
		got[0].Text != "    total: 40, count: 42" {
		t.Fatalf("first inlay = %#v", got[0])
	}
	if got[1].Offset != len("int total = count;\nlog(count, total, count, missing);") ||
		!strings.Contains(got[1].Text, "count: 42, total: 40, missing: null") {
		t.Fatalf("second inlay = %#v", got[1])
	}
}

func TestDebugInlineInlaysRespectPausePathAndSettingsInputs(t *testing.T) {
	tab := debugEditorTestTab("int count = 1;\n")
	vars := []openidea.Variable{{Name: "count", Value: "1"}}
	for name, got := range map[string][]editor.Inlay{
		"not paused": debugInlineInlays(tab, false, tab.doc.Path(), 0, vars),
		"other file": debugInlineInlays(tab, true, "/src/Other.java", 0, vars),
		"no vars":    debugInlineInlays(tab, true, tab.doc.Path(), 0, nil),
	} {
		if len(got) != 0 {
			t.Fatalf("%s inlays = %#v", name, got)
		}
	}
}

func TestRefreshDebugInlaysUsesTheSelectedPausedFrame(t *testing.T) {
	tab := debugEditorTestTab("int count = 1;\nreturn count;\n")
	m := &Model{}
	m.debug.paused = true
	m.debug.stopFile = tab.doc.Path()
	m.debug.stopLine = 1
	m.debug.vars = []openidea.Variable{{Name: "count", Value: "42"}}
	m.refreshDebugInlays(tab)
	if len(tab.inlay.debug) != 2 {
		t.Fatalf("debug inlays = %#v", tab.inlay.debug)
	}
	m.prefs.Debugger.NoInlineValues = true
	m.refreshDebugInlays(tab)
	if len(tab.inlay.debug) != 0 {
		t.Fatalf("disabled debug inlays = %#v", tab.inlay.debug)
	}
}

func TestDebugRunToCursorTargetsLeadingTextArea(t *testing.T) {
	text := "    int value = 1;\n\n  call();\n"
	tab := debugEditorTestTab(text)
	if line, ok := debugRunToCursorLine(tab, strings.Index(text, "int")); !ok || line != 0 {
		t.Fatalf("line start = %d, %v", line, ok)
	}
	if _, ok := debugRunToCursorLine(tab, strings.Index(text, "=")); ok {
		t.Fatal("Run to Cursor appeared over ordinary code")
	}
	blank := strings.Index(text, "\n\n") + 1
	if _, ok := debugRunToCursorLine(tab, blank); ok {
		t.Fatal("Run to Cursor appeared over a blank line")
	}
	call := strings.Index(text, "call")
	if line, ok := debugRunToCursorLine(tab, call); !ok || line != 2 {
		t.Fatalf("second line start = %d, %v", line, ok)
	}
}

func TestDebugRunToCursorHoverHonorsSettingAndRunsAtTheHoveredLine(t *testing.T) {
	text := "    first();\n  second();\n"
	tab := debugEditorTestTab(text)
	commands := make(chan map[string]any, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_run_to" {
				t.Fatalf("hover used %q, want debug_run_to", command)
			}
			commands <- input.(map[string]any)
			return json.RawMessage(`null`), nil
		},
	}
	m.debug.paused = true
	m.debug.session = 7
	m.debug.stopThread = 11
	m.debug.frameID = 31
	m.debug.stopFile = tab.doc.Path()
	m.debug.stopLine = 0
	lineStart := strings.Index(text, "second")

	if !m.showDebugRunToCursorHover(tab, lineStart) {
		t.Fatal("paused line did not offer Run to Cursor")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, tab.state).Fill()
	}, 900, 360)
	tester.Frame()
	if !tester.HasText("Run to Cursor") {
		t.Fatalf("Run to Cursor hover = %q", tester.Texts())
	}
	if err := tester.Click("Run to Cursor"); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-commands:
		if input["session"] != uint32(7) || input["threadId"] != int64(11) ||
			input["path"] != tab.doc.Path() || input["line"] != 1 || input["force"] != false {
			t.Fatalf("debug_run_to = %#v", input)
		}
	case <-time.After(time.Second):
		t.Fatal("Run to Cursor hover did not invoke the backend")
	}

	m.prefs.Debugger.NoRunToCursorInlay = true
	m.debug.paused = true
	if m.showDebugRunToCursorHover(tab, lineStart) {
		t.Fatal("disabled Run to Cursor inlay still showed")
	}
}

func TestDebugExecutionLineHoverResumesTheProgram(t *testing.T) {
	text := "  breakpoint();\n"
	tab := debugEditorTestTab(text)
	commands := make(chan map[string]any, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_resume" {
				t.Fatalf("execution hover used %q, want debug_resume", command)
			}
			commands <- input.(map[string]any)
			return json.RawMessage(`null`), nil
		},
	}
	m.debug.paused = true
	m.debug.session = 7
	m.debug.stopThread = 11
	m.debug.stopFile = tab.doc.Path()
	m.debug.stopLine = 0

	if !m.showDebugRunToCursorHover(tab, strings.Index(text, "breakpoint")) {
		t.Fatal("execution line did not offer Resume Program")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, tab.state).Fill()
	}, 900, 360)
	tester.Frame()
	if !tester.HasText("Resume Program") {
		t.Fatalf("Resume Program hover = %q", tester.Texts())
	}
	if err := tester.Click("Resume Program"); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-commands:
		if input["session"] != uint32(7) || input["threadId"] != int64(11) {
			t.Fatalf("debug_resume = %#v", input)
		}
	case <-time.After(time.Second):
		t.Fatal("Resume Program hover did not invoke the backend")
	}
}

func TestDebugSelectionExpressionTrimsOneSettledRange(t *testing.T) {
	text := "  order.items  \nnext\n"
	tab := debugEditorTestTab(text)
	start := strings.Index(text, "  order")
	end := strings.Index(text, "\n")
	tab.state.Select(start, end)
	expr, ok := debugSelectionExpressionAt(tab)
	if !ok || expr.Text != "order.items" ||
		expr.Start != strings.Index(text, "order") ||
		expr.End != strings.Index(text, "order")+len("order.items") {
		t.Fatalf("selection expression = %#v, %v", expr, ok)
	}
	tab.state.Select(0, strings.Index(text, "next")+2)
	if expr, ok := debugSelectionExpressionAt(tab); ok {
		t.Fatalf("multiline selection evaluated as %#v", expr)
	}
}

func TestDebugSelectionHoverWaitsThenEvaluates(t *testing.T) {
	text := "value = order.items;\n"
	tab := debugEditorTestTab(text)
	start, end := strings.Index(text, "order.items"), strings.Index(text, "order.items")+len("order.items")
	tab.state.Select(start, end)
	request := make(chan map[string]any, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
		postHook: func(fn func()) {
			fn()
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_evaluate_in" {
				return nil, context.Canceled
			}
			request <- input.(map[string]any)
			return json.RawMessage(`{"name":"order.items","value":"[1, 2]"}`), nil
		},
	}
	m.debug.paused, m.debug.session, m.debug.frameID, m.debug.stopGeneration = true, 7, 31, 4
	m.scheduleDebugSelectionHover(tab)
	select {
	case args := <-request:
		if args["expression"] != "order.items" || args["context"] != "hover" ||
			args["session"] != uint32(7) || args["frameId"] != int64(31) {
			t.Fatalf("selection evaluate arguments = %#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the settled selection was not evaluated")
	}
}

func TestDebugSelectionHoverCancellationDropsTheRequest(t *testing.T) {
	text := "first + second;\n"
	tab := debugEditorTestTab(text)
	tab.state.Select(0, len("first"))
	called := make(chan struct{}, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
		postHook: func(fn func()) {
			fn()
		},
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			called <- struct{}{}
			return nil, context.Canceled
		},
	}
	m.debug.paused, m.debug.session, m.debug.frameID = true, 7, 31
	m.scheduleDebugSelectionHover(tab)
	m.cancelDebugSelectionHover(tab)
	tab.state.Select(strings.Index(text, "second"), len(text)-2)
	select {
	case <-called:
		t.Fatal("a superseded selection was evaluated")
	case <-time.After(550 * time.Millisecond):
	}
}

func TestSortedDebugVariablesUsesNaturalNamesAndKeepsIndices(t *testing.T) {
	vars := []openidea.Variable{{Name: "item10"}, {Name: "item2"}, {Name: "Item1"}}
	got := sortedDebugVariables(vars, true)
	if got[0].Name != "Item1" || got[1].Name != "item2" || got[2].Name != "item10" {
		t.Fatalf("sorted names = %#v", got)
	}
	indices := []openidea.Variable{{Name: "10"}, {Name: "2"}}
	got = sortedDebugVariables(indices, true)
	if got[0].Name != "10" || got[1].Name != "2" {
		t.Fatalf("array order changed: %#v", got)
	}
}

func TestDebugValueAtUsesHoverContextAndCurrentFrame(t *testing.T) {
	request := make(chan map[string]any, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_evaluate_in" {
				t.Fatalf("command = %q", command)
			}
			request <- input.(map[string]any)
			return json.RawMessage(`{"name":"order.items","value":"[1, 2]","typeName":"List","variablesReference":8}`), nil
		},
	}
	m.debug.session, m.debug.frameID = 7, 31
	value, err := m.debugValueAt(context.Background(), "order.items")
	if err != nil {
		t.Fatal(err)
	}
	args := <-request
	if args["session"] != uint32(7) || args["frameId"] != int64(31) ||
		args["expression"] != "order.items" || args["context"] != "hover" {
		t.Fatalf("evaluate arguments = %#v", args)
	}
	if value.TypeName == nil || *value.TypeName != "List" ||
		debugValueHint("order.items", value) != "order.items = [1, 2]  {List}" {
		t.Fatalf("value hint = %q from %#v", debugValueHint("order.items", value), value)
	}
}

func TestDebuggerSettingsApplyToNativeRuntime(t *testing.T) {
	m := &Model{cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	pages := map[string]nativeSettingsPageState{
		settingsPageDebugger: {
			loaded: true,
			draft: map[string]any{
				"debugger.showOnBreakpoint":   false,
				"debugger.hideOnTermination":  true,
				"debugger.breakpointsAnyFile": true,
				"debugger.runToCursorInlay":   false,
			},
		},
		settingsPageDebuggerData: {
			loaded: true,
			draft: map[string]any{
				"debugger.inlineValues":            false,
				"debugger.sortValues":              true,
				"debugger.valueTooltip":            false,
				"debugger.valueTooltipOnSelection": false,
				"debugger.valueTooltipDelay":       float64(1200),
			},
		},
		settingsPageDebuggerStepping: {
			loaded: true,
			draft:  map[string]any{"debugger.skipLibraryCode": false},
		},
		settingsPagePythonDebugger: {
			loaded: true,
			draft: map[string]any{
				"debugger.python.attachSubprocess": false,
				"debugger.python.gevent":           true,
			},
		},
	}
	if !m.applyDebuggerRuntimeSettings(pages) {
		t.Fatal("debugger runtime settings did not change")
	}
	prefs := m.prefs.Debugger
	if prefs.ShowOnBreakpoint() || !prefs.HideOnTermination || !prefs.BreakpointsAnyFile ||
		prefs.RunToCursorInlay() || prefs.InlineValues() || !prefs.SortValues ||
		prefs.ValueTooltip() || prefs.ValueTooltipOnSelection() ||
		prefs.TooltipDelay() != 1200 || prefs.SkipLibraryCode() ||
		prefs.PythonAttachSubprocess() || !prefs.PythonGevent {
		t.Fatalf("runtime debugger preferences = %+v", prefs)
	}
	loaded := LoadPreferences(m.cfg.SettingsPath)
	if !debuggerPrefsEqual(loaded.Debugger, prefs) {
		t.Fatalf("stored debugger preferences = %+v, want %+v", loaded.Debugger, prefs)
	}
}

func TestDebuggerTooltipDelayPersistsExplicitZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	delay := 0
	prefs := Preferences{Debugger: DebuggerPrefs{ValueTooltipDelay: &delay}}
	if err := prefs.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded := LoadPreferences(path)
	if loaded.Debugger.ValueTooltipDelay == nil ||
		loaded.Debugger.TooltipDelay() != 0 {
		t.Fatalf("tooltip delay = %#v, want an explicit zero", loaded.Debugger.ValueTooltipDelay)
	}
}
