package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestEditorActionGapIDsAreRegisteredAndRouted(t *testing.T) {
	ids := []string{
		"find.popup.nextResult",
		"find.popup.previousResult",
		"find.popup.openResult",
		"find.popup.recentSearches",
		"navigate.customRegion",
		"navigate.gotoTest",
		"navigate.nextMethod",
		"navigate.previousMethod",
		"navigate.relatedSymbol",
		"navigate.superMethod",
		"refactor.pullMembersUp",
		"refactor.pushMembersDown",
		"code.autoIndentLines",
		"code.completeStatement",
		"code.cyclicExpandWord",
		"code.cyclicExpandWordBackward",
		"code.moveElementLeft",
		"code.moveElementRight",
		"code.unwrap",
		"code.reformatDialog",
		"editor.contextInfo",
		"editor.toggleInsertState",
		"code.moveStatementUp",
		"code.moveStatementDown",
		"code.expandToLevel1",
		"code.expandToLevel2",
		"code.expandToLevel3",
		"code.expandToLevel4",
		"code.expandToLevel5",
		"code.expandAllToLevel1",
		"code.expandAllToLevel2",
		"code.expandAllToLevel3",
		"code.expandAllToLevel4",
		"code.expandAllToLevel5",
	}
	for _, id := range ids {
		if action, ok := actionGet(id); !ok || action.Builtin == false {
			t.Errorf("%s is not a registered built-in action", id)
		}
	}
}

func TestAutoIndentPreservesSelectionsAndUsesOneUndoStep(t *testing.T) {
	m, tab := actionGapEditorModel(
		"/project/Demo.java",
		"class Demo {\nvoid run() {\nint value = 1; // keep\n}\n}\n",
		3,
	)
	tab.state.TabSize = 4
	tab.state.Indent = editor.IndentSpaces
	start := strings.Index(tab.state.Buffer.String(), "void run")
	end := strings.Index(tab.state.Buffer.String(), "int value") + len("int value")
	tab.state.SetSelections(editor.Selection{Anchor: end, Head: start})

	if !m.RunCommand("code.autoIndentLines") {
		t.Fatal("code.autoIndentLines is unavailable")
	}
	got := tab.state.Buffer.String()
	if !strings.Contains(got, "    void run() {\n        int value = 1; // keep\n") {
		t.Fatalf("auto-indented text = %q", got)
	}
	selection := tab.state.Selection()
	if selection.Anchor <= selection.Head {
		t.Fatalf("selection direction was not preserved: %+v", selection)
	}
	if !tab.state.Buffer.Undo() {
		t.Fatal("auto-indent did not create an undo step")
	}
	if !strings.Contains(tab.state.Buffer.String(), "void run") {
		t.Fatal("undo did not restore the text")
	}
}

func TestCompleteStatementClosesBracketsAndKeepsComment(t *testing.T) {
	m, tab := actionGapEditorModel(
		"/project/Demo.java",
		"class Demo {\n  void run() {\n    call(value // note\n  }\n}\n",
		len("class Demo {\n  void run() {\n    call(value"),
	)
	tab.state.TabSize = 4
	tab.state.Indent = editor.IndentSpaces
	if !m.RunCommand("code.completeStatement") {
		t.Fatal("code.completeStatement is unavailable")
	}
	if got := tab.state.Buffer.String(); !strings.Contains(got, "    call(value); // note\n") {
		t.Fatalf("completed statement = %q", got)
	}
}

func TestMoveElementSwapsArgumentsWithoutMovingStrings(t *testing.T) {
	m, tab := actionGapEditorModel(
		"/project/Demo.java",
		`call(alpha, "not, a separator", [one, two], target)`,
		strings.Index(`call(alpha, "not, a separator", [one, two], target)`, "target"),
	)
	if !m.RunCommand("code.moveElementLeft") {
		t.Fatal("code.moveElementLeft is unavailable")
	}
	want := `call(alpha, "not, a separator", target, [one, two])`
	if got := tab.state.Buffer.String(); got != want {
		t.Fatalf("move element = %q, want %q", got, want)
	}
	if tab.state.Cursor() != strings.Index(want, "target") {
		t.Fatalf("caret = %d, want on moved target", tab.state.Cursor())
	}
}

func TestCustomFoldingRegionsMatchSupportedMarkers(t *testing.T) {
	text := strings.Join([]string{
		"class Demo {",
		"// region Fields",
		"int value;",
		"// endregion",
		"#region Setup",
		"void run() {}",
		"#endregion",
		"<!-- <editor-fold desc=\"Unmatched\"> -->",
		"}",
	}, "\n")
	m, tab := actionGapEditorModel("/project/Demo.java", text, 0)
	if !m.RunCommand("navigate.customRegion") {
		t.Fatal("navigate.customRegion is unavailable")
	}
	if m.customRegions == nil || len(m.customRegions.regions) != 2 {
		t.Fatalf("custom regions = %+v", m.customRegions)
	}
	if m.customRegions.regions[0].name != "Fields" ||
		m.customRegions.regions[1].name != "Setup" {
		t.Fatalf("custom regions = %+v", m.customRegions.regions)
	}
	m.openCustomRegion(m.customRegions.regions[1])
	if got := tab.state.CursorPos().Line; got != 4 {
		t.Fatalf("region caret line = %d, want 4", got)
	}
}

func TestNextPreviousMethodUsesOutlineSelectionRange(t *testing.T) {
	m, tab := actionGapEditorModel(
		"/project/Demo.java",
		"class Demo {\n  void first() {}\n  void second() {}\n}\n",
		0,
	)
	tab.outline = []openidea.OutlineItem{
		{
			Name: "Demo", Kind: "class",
			Range:          actionGapOutlineRange(0, 0, 4, 1),
			SelectionRange: actionGapOutlineRange(0, 6, 0, 10),
			Children: []openidea.OutlineItem{
				{Name: "first", Kind: "method", Range: actionGapOutlineRange(1, 2, 1, 18), SelectionRange: actionGapOutlineRange(1, 7, 1, 12)},
				{Name: "second", Kind: "method", Range: actionGapOutlineRange(2, 2, 2, 19), SelectionRange: actionGapOutlineRange(2, 7, 2, 13)},
			},
		},
	}
	if !m.RunCommand("navigate.nextMethod") {
		t.Fatal("navigate.nextMethod is unavailable")
	}
	if got := tab.state.CursorPos().Line; got != 1 {
		t.Fatalf("next method line = %d, want 1", got)
	}
	if !m.RunCommand("navigate.nextMethod") {
		t.Fatal("navigate.nextMethod is unavailable")
	}
	if got := tab.state.CursorPos().Line; got != 2 {
		t.Fatalf("second next method line = %d, want 2", got)
	}
	if !m.RunCommand("navigate.previousMethod") {
		t.Fatal("navigate.previousMethod is unavailable")
	}
	if got := tab.state.CursorPos().Line; got != 1 {
		t.Fatalf("previous method line = %d, want 1", got)
	}
}

func TestFindPopupStepsAndOpensExistingResults(t *testing.T) {
	m, _ := actionGapEditorModel("/project/Demo.java", "first\nsecond\n", 0)
	m.search.visible = true
	m.search.done = true
	m.search.files = []openidea.FileMatches{{
		Path: m.tab().doc.Path(), RelPath: "Demo.java",
		Matches: []openidea.LineMatch{
			{Line: 0, Text: "first", Ranges: [][2]uint32{{0, 5}}},
			{Line: 1, Text: "second", Ranges: [][2]uint32{{0, 6}}},
		},
	}}
	if !m.RunCommand("find.popup.nextResult") {
		t.Fatal("find.popup.nextResult is unavailable")
	}
	if m.search.selectedFile != 0 || m.search.selectedMatch != 1 {
		t.Fatalf("selected result = %d:%d", m.search.selectedFile, m.search.selectedMatch)
	}
	if !m.RunCommand("find.popup.openResult") {
		t.Fatal("find.popup.openResult is unavailable")
	}
	if got := m.tab().state.CursorPos().Line; got != 1 {
		t.Fatalf("opened result line = %d, want 1", got)
	}
}

func TestCyclicExpandWordCyclesAndSingleUndoRestores(t *testing.T) {
	m, tab := actionGapEditorModel(
		"/project/Demo.java",
		"alpha alphabet already value alp",
		len("alpha alphabet already value alp"),
	)
	if !m.RunCommand("code.cyclicExpandWord") {
		t.Fatal("code.cyclicExpandWord is unavailable")
	}
	first := tab.state.Buffer.String()
	if !strings.HasSuffix(first, "alphabet") {
		t.Fatalf("first expansion = %q", first)
	}
	if !m.RunCommand("code.cyclicExpandWord") {
		t.Fatal("second cyclic expansion is unavailable")
	}
	second := tab.state.Buffer.String()
	if second == first {
		t.Fatal("cyclic expansion did not advance")
	}
}

func TestOverwriteInputNeverCrossesLineEnd(t *testing.T) {
	m, tab := actionGapEditorModel("/project/Demo.java", "abcd\nnext", 1)
	if !m.RunCommand("editor.toggleInsertState") {
		t.Fatal("editor.toggleInsertState is unavailable")
	}
	if !m.overwriteEditorInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "XY"}) {
		t.Fatal("overwrite input was not handled")
	}
	if got := tab.state.Buffer.String(); got != "aXYd\nnext" {
		t.Fatalf("overwrite result = %q", got)
	}
}

func TestStatementMoveUsesStructuralNeighbors(t *testing.T) {
	var calls []string
	m, tab, main := actionGapAsyncModel(
		t,
		"/project/Demo.java",
		"void run() {\n  first();\n  second();\n}\n",
		strings.Index("void run() {\n  first();\n  second();\n}\n", "first"),
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			calls = append(calls, command)
			if command != "syntax_structural_ranges" {
				t.Fatalf("unexpected RPC %q", command)
			}
			if input.(map[string]any)["doc"] == nil {
				t.Fatal("structural request used another document")
			}
			return json.RawMessage(`{
				"statement":{"kind":"expression_statement","range":{"start":{"line":1,"col":2},"end":{"line":1,"col":11}}},
				"prevStatement":{"kind":"expression_statement","range":{"start":{"line":0,"col":12},"end":{"line":0,"col":20}}},
				"nextStatement":{"kind":"expression_statement","range":{"start":{"line":2,"col":2},"end":{"line":2,"col":12}}}
			}`), nil
		},
	)
	main.do(func() { m.RunCommand("code.moveStatementDown") })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return strings.Contains(tab.state.Buffer.String(), "second();\n  first();")
	})
	if len(calls) != 1 || calls[0] != "syntax_structural_ranges" {
		t.Fatalf("RPC calls = %v", calls)
	}
}

func TestHierarchyRefactoringUsesJDTMembersAndApplyWorkspaceEdit(t *testing.T) {
	var calls []string
	m, _, main := actionGapAsyncModel(
		t,
		"/project/Demo.java",
		"class Dog extends Animal { String bark() { return \"\"; } }\n",
		28,
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			calls = append(calls, command)
			switch command {
			case "lsp_hierarchy_members":
				return json.RawMessage(`{
					"className":"Dog",
					"targets":[{"id":"animal","name":"Animal","qualifiedName":"zoo.Animal","interface":false}],
					"members":[{"id":"bark","label":"bark() : String","kind":"method","selected":true,"abstractable":true}]
				}`), nil
			case "lsp_hierarchy_refactor":
				input := input.(map[string]any)
				if input["kind"] != "pullUp" || input["target"] != "animal" {
					t.Fatalf("hierarchy request = %#v", input)
				}
				return json.RawMessage(`{
					"problems":[],
					"edit":{"files":[{"kind":"edit","path":"/project/Animal.java","edits":[]}]}
				}`), nil
			case "apply_workspace_edit":
				return json.RawMessage(`{}`), nil
			default:
				t.Fatalf("unexpected RPC %q", command)
				return nil, nil
			}
		},
	)
	main.do(func() { m.RunCommand("refactor.pullMembersUp") })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.hierarchyRefactor != nil && !m.hierarchyRefactor.busy
	})
	main.do(func() { m.confirmHierarchyRefactor() })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.hierarchyRefactor == nil
	})
	want := []string{"lsp_hierarchy_members", "lsp_hierarchy_refactor", "apply_workspace_edit"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("RPC calls = %v, want %v", calls, want)
	}
}

func TestFoldLevelCommandsRemainDistinct(t *testing.T) {
	text := "class Demo {\n  void run() {\n    if (x) {\n      while (ready) {\n        call();\n      }\n    }\n  }\n}\n"
	m, tab := actionGapEditorModel("/project/Demo.java", text, 0)
	tab.state.Run(editor.FoldAll)
	if !m.RunCommand("code.expandAllToLevel1") {
		t.Fatal("code.expandAllToLevel1 is unavailable")
	}
	if got := tab.state.Folded(); len(got) == 0 || got[0] == 0 {
		t.Fatalf("level 1 should keep the top region open and fold nested ones: %v", got)
	}
	levelOne := tab.state.Folded()
	if !m.RunCommand("code.expandAllToLevel2") {
		t.Fatal("code.expandAllToLevel2 is unavailable")
	}
	levelTwo := tab.state.Folded()
	if len(levelTwo) >= len(levelOne) {
		t.Fatalf("level 2 did not expose more regions: level 1=%v level 2=%v", levelOne, levelTwo)
	}
}

func actionGapEditorModel(path, text string, cursor int) (*Model, *Tab) {
	doc := docbridge.Virtual(path, text)
	doc.SetReadOnly(false)
	state := &editor.State{
		Buffer: doc.Buffer(), LineHeight: editorLineHeight(13),
		TabSize: 4, Indent: editor.IndentSpaces,
	}
	state.Select(cursor, cursor)
	tab := &Tab{doc: doc, state: state, editorFocused: true}
	m := &Model{ctx: context.Background(), tabs: []*Tab{tab}, active: 0}
	m.setRoot("/project")
	return m, tab
}

func actionGapOutlineRange(startLine, startCol, endLine, endCol uint32) openidea.OutlineRange {
	return openidea.OutlineRange{
		Start: openidea.OutlinePos{Line: startLine, Col: startCol},
		End:   openidea.OutlinePos{Line: endLine, Col: endCol},
	}
}

func actionGapAsyncModel(
	t *testing.T,
	path, text string,
	cursor int,
	hook func(context.Context, string, any) (json.RawMessage, error),
) (*Model, *Tab, *serialMainThread) {
	t.Helper()
	doc := docbridge.Virtual(path, text)
	doc.SetReadOnly(false)
	state := &editor.State{
		Buffer: doc.Buffer(), LineHeight: editorLineHeight(13),
		TabSize: 4, Indent: editor.IndentSpaces,
	}
	state.Select(cursor, cursor)
	tab := &Tab{doc: doc, state: state}
	main := &serialMainThread{}
	m := &Model{
		ctx: context.Background(), postHook: main.post, invokeHook: hook,
		tabs: []*Tab{tab}, active: 0,
	}
	tab.editorFocused = true
	m.setRoot("/project")
	return m, tab, main
}

func TestCompleteStatementUsesBackendMissingBracketEvidence(t *testing.T) {
	var calls []string
	m, tab, main := actionGapAsyncModel(
		t,
		"/project/Demo.java",
		"class Demo { void run() {\n  call(value\n",
		len("class Demo { void run() {\n  call(value"),
		func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			calls = append(calls, command)
			if command != "syntax_errors" {
				t.Fatalf("unexpected RPC %q", command)
			}
			return json.RawMessage(`{
				"errors":[{"kind":"missing","range":{"start":{"line":1,"col":12},"end":{"line":1,"col":12}},"expected":")"}],
				"truncated":false
			}`), nil
		},
	)
	main.do(func() { m.RunCommand("code.completeStatement") })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var got string
		main.do(func() { got = tab.state.Buffer.String() })
		if strings.Contains(got, "call(value);\n") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var got string
	main.do(func() { got = tab.state.Buffer.String() })
	if !strings.Contains(got, "call(value);\n") {
		t.Fatalf("completed statement = %q", got)
	}
	if got := strings.Join(calls, ","); got != "syntax_errors" {
		t.Fatalf("RPC calls = %q", got)
	}
}
