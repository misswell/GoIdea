package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func readOnlyDiffForFind() *diffView {
	d := &diffView{
		open: true, path: "/project/Main.java",
		leftPath: "/project/Main.java", rightPath: "/project/Main.java",
		leftLabel: "Revision", rightLabel: "Revision",
		leftText: "left needle\nkept\n", rightText: "right needle\nkept\n",
		options: DiffPrefs{Mode: "split"},
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	return d
}

func TestReadOnlyDiffPanesUseNativeFind(t *testing.T) {
	m := &Model{}
	d := readOnlyDiffForFind()
	m.ensureDiffEditors(d)
	m.diff = d
	for label, state := range map[string]*editor.State{
		"left": d.leftEditor, "right": d.rightEditor,
	} {
		if state == nil || !state.ReadOnly {
			t.Fatalf("%s revision pane is not a read-only native editor", label)
		}
		state.Find("needle")
		if count, current := state.FindMatches(); count != 1 || current != 0 {
			t.Fatalf("%s find matches = %d current %d", label, count, current)
		}
	}
}

func TestRevisionDiffFindReplaceShowsSearchOnly(t *testing.T) {
	m := &Model{}
	d := readOnlyDiffForFind()
	m.ensureDiffEditors(d)
	m.diff = d
	var leftBar, rightBar *editor.FindBar
	d.leftEditor.RenderFindBar = func(c *ui.Context, bar ui.Element, find *editor.FindBar) {
		leftBar = find
		editor.DefaultFindBar(c, bar, find)
	}
	d.rightEditor.RenderFindBar = func(c *ui.Context, bar ui.Element, find *editor.FindBar) {
		rightBar = find
		editor.DefaultFindBar(c, bar, find)
	}
	d.leftEditor.Run(editor.FindReplace)
	d.rightEditor.Run(editor.FindReplace)
	tester := ui.NewTester(func(c *ui.Context) {
		m.renderDiffRows(c, c.Theme(), d.options.normalized())
	}, 1000, int(diffEditorHeight))
	tester.Frame()
	if leftBar == nil || rightBar == nil {
		t.Fatal("revision find bars did not render")
	}
	for label, bar := range map[string]*editor.FindBar{"left": leftBar, "right": rightBar} {
		if bar.CanReplace() || bar.Replacing() {
			t.Fatalf("%s read-only find bar can replace", label)
		}
	}
}

func TestEditableDiffFindReplaceIsUndoable(t *testing.T) {
	m := &Model{}
	d := &diffView{
		open: true, path: "/project/Main.java",
		leftPath: "/project/Main.java", rightPath: "/project/Main.java",
		leftText: "revision\n", rightText: "working needle\n",
		rightEditable: true, options: DiffPrefs{Mode: "split"},
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.ensureDiffEditors(d)
	m.diff = d
	var bar *editor.FindBar
	d.rightEditor.RenderFindBar = func(c *ui.Context, element ui.Element, find *editor.FindBar) {
		bar = find
		editor.DefaultFindBar(c, element, find)
	}
	d.rightEditor.Run(editor.FindReplace)
	tester := ui.NewTester(func(c *ui.Context) {
		m.renderDiffRows(c, c.Theme(), d.options.normalized())
	}, 1000, int(diffEditorHeight))
	tester.Frame()
	if bar == nil || !bar.CanReplace() || !bar.Replacing() {
		t.Fatal("working-copy find bar is not replaceable")
	}
	*bar.Query() = "needle"
	*bar.Replacement() = "result"
	bar.ReplaceAll()
	d.refreshDiffEditorText()
	if got, want := d.rightText, "working result\n"; got != want || !d.dirty {
		t.Fatalf("replacement = %q dirty %v, want %q dirty", got, d.dirty, want)
	}

	if !d.rightEditor.Run(editor.Undo) {
		t.Fatal("Diff replace was not undoable")
	}
	d.refreshDiffEditorText()
	if got, want := d.rightText, "working needle\n"; got != want || d.dirty {
		t.Fatalf("undo = %q dirty %v, want %q clean", got, d.dirty, want)
	}
}

func TestDiffFindUsesReboundKeysAndEscapeCloses(t *testing.T) {
	m := &Model{prefs: Preferences{
		KeymapPreset: "intellij-macos",
		Keymap: map[string][]string{
			"find.inFile":        {"Cmd+Shift+J"},
			"find.replaceInFile": {"Cmd+Shift+R"},
		},
	}}
	d := &diffView{
		open: true, path: "/project/Main.java",
		leftPath: "/project/Main.java", rightPath: "/project/Main.java",
		leftText: "revision\n", rightText: "working needle\n",
		rightEditable: true, options: DiffPrefs{Mode: "split"},
	}
	d.resetDiffEditors()
	d.setDiffRows(d.leftText, d.rightText)
	m.ensureDiffEditors(d)
	m.diff = d
	findBinding, ok := diffBindingFor(d.rightEditor.Bindings, editor.Find)
	if !ok || findBinding.Mods != ui.Cmd|ui.Shift || findBinding.Key != ui.KeyJ {
		t.Fatalf("rebound Find binding = %+v, found %v", findBinding, ok)
	}
	replaceBinding, ok := diffBindingFor(d.rightEditor.Bindings, editor.FindReplace)
	if !ok || replaceBinding.Mods != ui.Cmd|ui.Shift || replaceBinding.Key != ui.KeyR {
		t.Fatalf("rebound Replace binding = %+v, found %v", replaceBinding, ok)
	}

	var bar *editor.FindBar
	d.rightEditor.RenderFindBar = func(c *ui.Context, element ui.Element, find *editor.FindBar) {
		bar = find
		editor.DefaultFindBar(c, element, find)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.renderDiffRows(c, c.Theme(), d.options.normalized())
	}, 1000, int(diffEditorHeight))
	tester.Frame()
	if err := tester.Click("Compare right"); err != nil {
		t.Fatal(err)
	}
	tester.Key(ui.Cmd|ui.Shift, ui.KeyJ)
	tester.Frame()
	d.rightEditor.Find("needle")
	tester.Frame()
	if count, _ := d.rightEditor.FindMatches(); count != 1 {
		t.Fatalf("rebound Find found %d matches", count)
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if count, _ := d.rightEditor.FindMatches(); count != 0 {
		t.Fatalf("Escape left the Find bar open with %d matches", count)
	}

	tester.Key(ui.Cmd|ui.Shift, ui.KeyR)
	tester.Frame()
	if bar == nil || !bar.CanReplace() || !bar.Replacing() {
		t.Fatal("rebound Replace did not open the replaceable bar")
	}
}

func diffBindingFor(bindings []editor.Binding, command editor.Command) (editor.Binding, bool) {
	for _, binding := range bindings {
		if binding.Command == command {
			return binding, true
		}
	}
	return editor.Binding{}, false
}
