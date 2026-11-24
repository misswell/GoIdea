package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestInlineColumnAndPasteHistoryActionsMatchOpenIDEA(t *testing.T) {
	for id, shortcut := range map[string]string{
		"editor.inlineCompletion.trigger":    "Alt+\\",
		"editor.inlineCompletion.accept":     "Tab",
		"editor.inlineCompletion.acceptWord": "Cmd+Right",
		"editor.inlineCompletion.dismiss":    "Escape",
		"editor.inlineCompletion.next":       "Alt+]",
		"editor.inlineCompletion.previous":   "Alt+[",
		"code.pasteFromHistory":              "Cmd+Shift+V",
		"editor.toggleColumnSelection":       "Cmd+Shift+8",
	} {
		if _, ok := actionGet(id); !ok {
			t.Fatalf("action %q is not registered", id)
		}
		if got := builtinShortcuts[id]; got != shortcut {
			t.Fatalf("%s shortcut = %q, want %q", id, got, shortcut)
		}
	}
	for _, id := range []string{
		"editor.inlineCompletion.accept",
		"editor.inlineCompletion.acceptWord",
		"editor.inlineCompletion.dismiss",
		"editor.inlineCompletion.next",
		"editor.inlineCompletion.previous",
		"editor.pasteHistory.paste",
		"editor.pasteHistory.remove",
		"editor.showHint",
	} {
		if !hiddenActionIDs[id] {
			t.Fatalf("%s is visible in the command palette", id)
		}
	}
	if overlay := overlays.byID["editor.pasteHistory"]; overlay == nil || !overlay.Modal {
		t.Fatalf("editor.pasteHistory overlay = %+v", overlay)
	}
}

func TestNativeKeymapActionsMatchOpenIDEA(t *testing.T) {
	wantShortcuts := map[string]string{
		"code.addCaretNextOccurrence": "Ctrl+G",
		"code.selectAllOccurrences":   "Ctrl+Cmd+G",
		"code.unselectOccurrence":     "Ctrl+Shift+G",
		"code.addCaretsToLineEnds":    "Alt+Shift+G",
		"code.extendSelection":        "Alt+Up",
		"code.shrinkSelection":        "Alt+Down",
		"code.fold":                   "Cmd+-",
		"code.unfold":                 "Cmd+=",
		"code.foldAll":                "Cmd+Shift+-",
		"code.unfoldAll":              "Cmd+Shift+=",
		"code.collapseRecursively":    "Cmd+Alt+-",
		"code.expandRecursively":      "Cmd+Alt+=",
		"code.foldSelection":          "Cmd+.",
		"code.collapseBlock":          "Cmd+Shift+.",
		"code.completion":             "Ctrl+Space",
		"code.startNewLine":           "Shift+Enter",
		"code.startNewLineBefore":     "Cmd+Alt+Enter",
		"code.splitLine":              "Cmd+Enter",
		"code.toggleCase":             "Cmd+Shift+U",
		"code.pasteAsPlainText":       "Cmd+Alt+Shift+V",
		"code.showErrorDescription":   "Cmd+F1",
		"code.applyQuickFix":          "Alt+Shift+Enter",
		"code.parameterInfo":          "Cmd+P",
		"code.expressionTypeInfo":     "Ctrl+Shift+P",
		"code.generate":               "Cmd+N",
		"code.implementMethods":       "Ctrl+I",
		"code.overrideMethods":        "Ctrl+O",
		"refactor.thisMenu":           "Ctrl+T",
		"refactor.changeSignature":    "Cmd+F6",
		"refactor.extractVariable":    "Cmd+Alt+V",
		"refactor.extractMethod":      "Cmd+Alt+M",
		"refactor.extractConstant":    "Cmd+Alt+C",
		"refactor.extractField":       "Cmd+Alt+F",
		"refactor.extractParameter":   "Cmd+Alt+P",
		"refactor.inline":             "Cmd+Alt+N",
		"refactor.move":               "F6",
		"editor.scrollToCenter":       "Ctrl+L",
		"editor.matchBrace":           "Ctrl+M",
		"navigate.declaration":        "Cmd+B",
	}
	for id, shortcut := range wantShortcuts {
		a, ok := actionGet(id)
		if !ok {
			t.Errorf("registry has no %s", id)
			continue
		}
		if a.ID != id {
			t.Errorf("%s canonical id = %s", id, a.ID)
		}
		if got := builtinShortcuts[id]; got != shortcut {
			t.Errorf("%s shortcut = %q, want %q", id, got, shortcut)
		}
	}

	for id, want := range map[string]string{
		"code.showIntentions": "code.intentions",
		"code.reformat":       "code.format",
		"refactor.rename":     "code.rename",
		"toolwindow.services": "view.services",
		"navigate.nextError":  "problems.next",
		"navigate.prevError":  "problems.prev",
	} {
		if got := canonicalActionID(id); got != want {
			t.Errorf("%s aliases to %q, want %q", id, got, want)
		}
		if _, ok := actionGet(id); !ok {
			t.Errorf("%s is not callable through the alias", id)
		}
	}
}

func TestEditActionsRespectReadOnlyState(t *testing.T) {
	readOnly := editorTab("one", 0)
	readOnly.state.ReadOnly = true
	m := &Model{tabs: []*Tab{readOnly}}
	for _, id := range []string{
		"code.completion",
		"code.startNewLine",
		"code.startNewLineBefore",
		"code.splitLine",
		"code.sortLines",
		"code.reverseLines",
		"code.toggleCase",
		"code.pasteAsPlainText",
		"code.applyQuickFix",
	} {
		if m.ActionEnabled(id) {
			t.Errorf("%s is enabled in a read-only editor", id)
		}
	}
	for _, id := range []string{
		"code.addCaretNextOccurrence",
		"code.selectAllOccurrences",
		"code.unselectOccurrence",
		"code.addCaretsToLineEnds",
		"code.extendSelection",
		"code.shrinkSelection",
		"code.fold",
		"code.unfold",
		"editor.scrollToCenter",
		"editor.matchBrace",
		"navigate.declaration",
	} {
		if !m.ActionEnabled(id) && id != "navigate.declaration" {
			t.Errorf("%s is disabled in a read-only editor", id)
		}
	}
}

func TestAddCaretsToSelectedLineEndsMatchesOpenIDEA(t *testing.T) {
	tab := editorTab("ab\ncd\nef\ngh", 0)
	tab.state.SetSelections(editor.Selection{Anchor: 0, Head: 9})
	m := &Model{tabs: []*Tab{tab}}
	if !m.RunCommand("code.addCaretsToLineEnds") {
		t.Fatal("code.addCaretsToLineEnds is unavailable")
	}
	got := tab.state.Selections()
	wantHeads := []int{2, 5, 8}
	if len(got) != len(wantHeads) {
		t.Fatalf("carets = %+v", got)
	}
	for i, want := range wantHeads {
		if got[i].Anchor != want || got[i].Head != want {
			t.Errorf("caret %d = %+v, want %d", i, got[i], want)
		}
	}
}

func TestUnselectLatestOccurrenceMatchesOpenIDEA(t *testing.T) {
	tab := editorTab("a a a", 0)
	tab.state.SetSelections(
		editor.Selection{Anchor: 0, Head: 1},
		editor.Selection{Anchor: 2, Head: 3},
		editor.Selection{Anchor: 4, Head: 5},
	)
	m := &Model{tabs: []*Tab{tab}}
	if !m.RunCommand("code.unselectOccurrence") {
		t.Fatal("code.unselectOccurrence is unavailable")
	}
	got := tab.state.Selections()
	if len(got) != 2 || got[0].Start() != 0 || got[1].Start() != 2 {
		t.Fatalf("selections after unselect = %+v", got)
	}
	if !m.RunCommand("code.unselectOccurrence") {
		t.Fatal("second unselect is unavailable")
	}
	if got = tab.state.Selections(); len(got) != 1 || got[0].Start() != 0 || got[0].End() != 1 {
		t.Fatalf("selections after second unselect = %+v", got)
	}
	if !m.RunCommand("code.unselectOccurrence") {
		t.Fatal("single selection did not collapse")
	}
	if got = tab.state.Selections(); len(got) != 1 || !got[0].Empty() || got[0].Head != 1 {
		t.Fatalf("collapsed selection = %+v", got)
	}
}

func TestToggleCaseUsesOneUndoStepAndKeepsDirection(t *testing.T) {
	tab := editorTab("fooBar bazQux", 0)
	tab.state.SetSelections(
		editor.Selection{Anchor: 0, Head: 6},
		editor.Selection{Anchor: 12, Head: 7},
	)
	m := &Model{tabs: []*Tab{tab}}
	if !m.RunCommand("code.toggleCase") {
		t.Fatal("code.toggleCase is unavailable")
	}
	if got := tab.state.Buffer.String(); got != "foobar bazqux" {
		t.Fatalf("toggle case = %q", got)
	}
	selections := tab.state.Selections()
	if len(selections) != 2 || selections[1].Anchor <= selections[1].Head {
		t.Fatalf("selection direction was not preserved: %+v", selections)
	}
	if !tab.state.Buffer.Undo() {
		t.Fatal("toggle case did not record undo")
	}
	if got := tab.state.Buffer.String(); got != "fooBar bazQux" {
		t.Fatalf("undo after toggle = %q", got)
	}
}

func TestPasteAsPlainTextUsesAnExactInsertion(t *testing.T) {
	original := readEditorClipboardText
	readEditorClipboardText = func() string { return "x\ny\n" }
	t.Cleanup(func() { readEditorClipboardText = original })

	tab := editorTab("ab", 1)
	m := &Model{tabs: []*Tab{tab}}
	if !m.RunCommand("code.pasteAsPlainText") {
		t.Fatal("code.pasteAsPlainText is unavailable")
	}
	if got := tab.state.Buffer.String(); got != "ax\ny\nb" {
		t.Fatalf("plain paste = %q", got)
	}
}

func TestShowErrorDescriptionReportsNoProblemAndShowsTheProblem(t *testing.T) {
	tab := editorTab("broken", 2)
	var shown string
	tab.state.RenderHover = func(_ *ui.Context, _ ui.Element, card *editor.HoverCard) {
		shown = card.Text()
	}
	m := &Model{tabs: []*Tab{tab}}
	if !m.RunCommand("code.showErrorDescription") {
		t.Fatal("code.showErrorDescription is unavailable")
	}
	if m.status != "No error or warning under the caret" {
		t.Fatalf("missing diagnostic status = %q", m.status)
	}
	tab.state.SetDiagnostics(tab.state.Buffer.Version(), []editor.Diagnostic{{
		Start: 0, End: len("broken"), Severity: editor.SeverityError,
		Message: "cannot resolve symbol", Source: "java",
	}})
	if !m.RunCommand("code.showErrorDescription") {
		t.Fatal("code.showErrorDescription is unavailable with a diagnostic")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, tab.state).Fill()
	}, 800, 300)
	tester.Frame()
	if shown != "java: cannot resolve symbol" {
		t.Fatalf("error hover = %q", shown)
	}
}
