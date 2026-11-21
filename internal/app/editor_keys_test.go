package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestIntellijEditorBindingsMatchOpenIDEA(t *testing.T) {
	want := []editor.Binding{
		{Mods: ui.Cmd, Key: ui.KeySlash, Command: editor.ToggleComment},
		{Mods: ui.Cmd | ui.Alt, Key: ui.KeySlash, Command: editor.ToggleBlockComment},
		{Mods: ui.Ctrl | ui.Shift, Key: ui.KeySlash, Command: editor.ToggleBlockComment},
		{Mods: ui.Cmd, Key: ui.KeyD, Command: editor.DuplicateSelection},
		{Mods: ui.Cmd, Key: ui.KeyBackspace, Command: editor.DeleteLine},
		{Mods: ui.Alt | ui.Shift, Key: ui.KeyUp, Command: editor.MoveLinesUp},
		{Mods: ui.Alt | ui.Shift, Key: ui.KeyDown, Command: editor.MoveLinesDown},
		{Mods: ui.Cmd | ui.Shift, Key: ui.KeyUp, Command: editor.MoveLinesUp},
		{Mods: ui.Cmd | ui.Shift, Key: ui.KeyDown, Command: editor.MoveLinesDown},
		{Mods: ui.Ctrl | ui.Shift, Key: ui.KeyJ, Command: editor.JoinLines},
		{Mods: ui.Cmd, Key: ui.KeyF, Command: editor.Find},
		{Mods: ui.Cmd, Key: ui.KeyR, Command: editor.FindReplace},
		{Mods: ui.Cmd, Key: ui.KeyG, Command: editor.FindNext},
		{Mods: ui.Cmd | ui.Shift, Key: ui.KeyG, Command: editor.FindPrevious},
	}
	got := intellijEditorBindings()
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing binding %+v", w)
		}
	}
}

// editorTab is a tab over text with its caret at offset, set up as newTab
// sets up a document's editor.
func editorTab(text string, offset int) *Tab {
	st := &editor.State{Buffer: editor.NewBuffer(text), Bindings: intellijEditorBindings()}
	st.Select(offset, offset)
	return &Tab{state: st}
}

func TestRunEditorCommandActsOnTheActiveEditor(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		offset int
		cmd    editor.Command
		want   string
	}{
		{"deleteLine", "a\nb\nc\n", 2, editor.DeleteLine, "a\nc\n"},
		{"duplicateLine", "a\nb\nc\n", 2, editor.DuplicateSelection, "a\nb\nb\nc\n"},
		{"moveLineUp", "a\nb\n", 2, editor.MoveLinesUp, "b\na\n"},
		{"moveLineDown", "a\nb\n", 0, editor.MoveLinesDown, "b\na\n"},
	}
	for _, c := range cases {
		m := &Model{tabs: []*Tab{editorTab(c.text, c.offset)}}
		m.runEditorCommand(c.cmd)
		if got := m.tabs[0].state.Buffer.String(); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRunEditorCommandWithoutATabIsQuiet(t *testing.T) {
	m := &Model{}
	m.runEditorCommand(editor.DeleteLine)
	if len(m.tabs) != 0 {
		t.Fatalf("command made %d tabs", len(m.tabs))
	}
}

func TestBasicEditorActionsRunThroughTheRegistry(t *testing.T) {
	m := &Model{
		tabs:   []*Tab{editorTab("abc", 0)},
		active: 0,
	}
	m.tabs[0].state.Buffer.Set("changed")
	if !m.ActionEnabled("edit.undo") {
		t.Fatal("Undo is disabled with an undo step")
	}
	if !m.RunCommand("edit.undo") {
		t.Fatal("edit.undo is unavailable")
	}
	if got := m.tabs[0].state.Buffer.String(); got != "abc" {
		t.Fatalf("edit.undo = %q", got)
	}
	if m.ActionEnabled("edit.undo") || !m.ActionEnabled("edit.redo") {
		t.Fatal("Undo/Redo enabled state did not follow the buffer history")
	}
	if !m.RunCommand("edit.redo") {
		t.Fatal("edit.redo is unavailable")
	}
	if got := m.tabs[0].state.Buffer.String(); got != "changed" {
		t.Fatalf("edit.redo = %q", got)
	}

	if !m.RunCommand("edit.selectAll") {
		t.Fatal("edit.selectAll is unavailable")
	}
	if got := m.tabs[0].state.SelectedText(); got != "changed" {
		t.Fatalf("selectAll = %q", got)
	}
	m.tabs[0].state.Select(1, 2)
	if !m.RunCommand("edit.cut") {
		t.Fatal("edit.cut is unavailable")
	}
	if got := m.tabs[0].state.Buffer.String(); got != "canged" {
		t.Fatalf("cut = %q", got)
	}
	if !m.RunCommand("edit.paste") {
		t.Fatal("edit.paste is unavailable")
	}
	if !m.RunCommand("edit.copy") {
		t.Fatal("edit.copy is unavailable")
	}
}

func TestEditorActionsRespectEditorAvailability(t *testing.T) {
	m := &Model{}
	for _, id := range []string{
		"edit.undo", "edit.redo", "edit.cut", "edit.copy", "edit.paste",
		"edit.selectAll", "editor.toggleSoftWrap", "editor.increaseFontSize",
		"editor.decreaseFontSize", "editor.resetFontSize", "find.inFile",
		"find.replaceInFile", "find.next", "find.previous",
	} {
		if m.ActionEnabled(id) {
			t.Errorf("%s is enabled without an editor", id)
		}
	}
}

func TestFileFindActionsUseTheNativeFindBar(t *testing.T) {
	m := &Model{
		tabs:   []*Tab{editorTab("one two one", 0)},
		active: 0,
	}
	if !m.RunCommand("find.inFile") {
		t.Fatal("find.inFile is unavailable")
	}
	count, current := m.tabs[0].state.FindMatches()
	if count != 2 {
		t.Fatalf("find.inFile found %d matches, want 2", count)
	}
	if !m.RunCommand("find.next") {
		t.Fatal("find.next is unavailable")
	}
	_, next := m.tabs[0].state.FindMatches()
	if !m.RunCommand("find.next") {
		t.Fatal("find.next is unavailable on the selected match")
	}
	_, next = m.tabs[0].state.FindMatches()
	if next == current {
		t.Fatalf("find.next stayed on match %d", current)
	}
	if !m.RunCommand("find.previous") {
		t.Fatal("find.previous is unavailable")
	}
	_, previous := m.tabs[0].state.FindMatches()
	if previous != current {
		t.Fatalf("find.previous selected %d, want %d", previous, current)
	}
	if !m.RunCommand("find.replaceInFile") {
		t.Fatal("find.replaceInFile is unavailable")
	}
}

func TestFileFindActionsKeepTheirOpenIDEAIdentity(t *testing.T) {
	want := map[string]struct {
		title    string
		bindings []string
	}{
		"find.inFile":        {"Find", []string{"Cmd+F"}},
		"find.replaceInFile": {"Replace", []string{"Cmd+R"}},
		"find.next":          {"Find Next Occurrence", []string{"Cmd+G"}},
		"find.previous":      {"Find Previous Occurrence", []string{"Cmd+Shift+G"}},
	}
	m := &Model{}
	for id, wantAction := range want {
		a, ok := actionGet(id)
		if !ok {
			t.Errorf("registry has no %s", id)
			continue
		}
		if a.Title != wantAction.title {
			t.Errorf("%s title = %q, want %q", id, a.Title, wantAction.title)
		}
		if got := m.KeymapBindings(id); len(got) != len(wantAction.bindings) ||
			got[0] != wantAction.bindings[0] {
			t.Errorf("%s bindings = %v, want %v", id, got, wantAction.bindings)
		}
	}
	if got := canonicalActionID("find.inFile"); got != "find.inFile" {
		t.Fatalf("find.inFile aliases to %q", got)
	}
	if got := canonicalActionID("view.find"); got != "view.find" {
		t.Fatalf("view.find aliases to %q", got)
	}
}

func TestSoftWrapActionTogglesTheActiveTabAndState(t *testing.T) {
	m := &Model{
		tabs:   []*Tab{editorTab("long line", 0)},
		active: 0,
	}
	if m.ActionToggled("editor.toggleSoftWrap") {
		t.Fatal("soft wrap starts on")
	}
	if !m.RunCommand("editor.toggleSoftWrap") {
		t.Fatal("editor.toggleSoftWrap is unavailable")
	}
	if !m.tabs[0].state.SoftWrap || !m.ActionToggled("editor.toggleSoftWrap") {
		t.Fatal("soft wrap did not turn on")
	}
	if !m.RunCommand("editor.toggleSoftWrap") || m.tabs[0].state.SoftWrap {
		t.Fatal("soft wrap did not turn back off")
	}
}

func TestRenderedDocumentationCommentFallback(t *testing.T) {
	text := "class A {\n/** Return **value**.\n * `null` is allowed.\n */\nObject get() { return null; }\n}"
	offset := strings.Index(text, "value")
	raw, ok := documentationCommentAt(text, offset)
	if !ok {
		t.Fatal("block documentation comment was not found")
	}
	clean := cleanDocumentationComment(raw)
	if !strings.Contains(clean, "Return **value**.") ||
		!strings.Contains(clean, "`null` is allowed.") {
		t.Fatalf("cleaned Javadoc = %q", clean)
	}

	lineComment := "// Build the thing.\n// Runs `make`.\nfunc Build() {}"
	raw, ok = documentationCommentAt(lineComment, strings.Index(lineComment, "make"))
	if !ok || cleanDocumentationComment(raw) != "Build the thing.\nRuns `make`." {
		t.Fatalf("line documentation = %q, %v", raw, ok)
	}
}

func TestRenderedDocumentationToggleShowsAndClosesCommentPreview(t *testing.T) {
	text := "/** Show this as prose. */\nclass A {}"
	tab := editorTab(text, strings.Index(text, "prose"))
	m := &Model{tabs: []*Tab{tab}, active: 0}
	m.usagePopupAnchor = ui.Rect{X: 20, Y: 30, W: 1, H: 16}
	if !m.RunCommand("editor.toggleRenderedDocs") {
		t.Fatal("Render Documentation Comments is unavailable")
	}
	if m.documentation.popup == nil ||
		len(m.documentation.popup.entries) != 1 ||
		len(m.documentation.popup.entries[0].blocks) == 0 {
		t.Fatalf("documentation preview = %+v", m.documentation)
	}
	if tab.renderedDocsOverride == nil || !*tab.renderedDocsOverride {
		t.Fatal("the tab did not turn rendering on")
	}
	if !m.RunCommand("editor.toggleRenderedDocs") || m.documentation.popup != nil {
		t.Fatalf("rendering did not turn off: %+v", m.documentation)
	}
}

func TestEditorFontActionsUseSharedRuntimeZoom(t *testing.T) {
	main := editorTab("main", 0)
	split := editorTab("split", 0)
	m := &Model{
		tabs:   []*Tab{main},
		active: 0,
		split:  &splitGroup{tabs: []*Tab{split}},
		prefs:  Preferences{FontSize: 13},
	}
	if !m.RunCommand("editor.increaseFontSize") {
		t.Fatal("editor.increaseFontSize is unavailable")
	}
	if m.editorFontZoom != 1 || main.state.Font.Size != 14 || split.state.Font.Size != 14 {
		t.Fatalf("increase = zoom %d, fonts %v/%v", m.editorFontZoom, main.state.Font.Size, split.state.Font.Size)
	}
	if m.prefs.FontSize != 13 {
		t.Fatalf("font zoom changed the configured size to %v", m.prefs.FontSize)
	}
	for range 100 {
		m.RunCommand("editor.increaseFontSize")
	}
	if m.editorFontZoom != maxEditorFontZoom || main.state.Font.Size != 13+maxEditorFontZoom {
		t.Fatalf("maximum zoom = %d, size %v", m.editorFontZoom, main.state.Font.Size)
	}
	if !m.RunCommand("editor.decreaseFontSize") || m.editorFontZoom != maxEditorFontZoom-1 {
		t.Fatalf("decrease = %d", m.editorFontZoom)
	}
	if !m.RunCommand("editor.resetFontSize") {
		t.Fatal("editor.resetFontSize is unavailable")
	}
	if m.editorFontZoom != 0 || main.state.Font.Size != 13 || split.state.Font.Size != 13 {
		t.Fatalf("reset = zoom %d, fonts %v/%v", m.editorFontZoom, main.state.Font.Size, split.state.Font.Size)
	}
}

func TestEditorTabActionsHaveOpenIDEAIdentityAndBindings(t *testing.T) {
	want := map[string]string{
		"edit.undo":               "Undo",
		"edit.redo":               "Redo",
		"edit.cut":                "Cut",
		"edit.copy":               "Copy",
		"edit.paste":              "Paste",
		"edit.selectAll":          "Select All",
		"editor.nextTab":          "Select Next Tab",
		"editor.prevTab":          "Select Previous Tab",
		"editor.toggleSoftWrap":   "Soft-Wrap",
		"editor.resetFontSize":    "Reset Font Size in All Editors",
		"editor.increaseFontSize": "Increase Font Size",
		"editor.decreaseFontSize": "Decrease Font Size",
	}
	for id, title := range want {
		a, ok := actionGet(id)
		if !ok {
			t.Errorf("registry has no %s", id)
			continue
		}
		if a.Title != title {
			t.Errorf("%s title = %q, want %q", id, a.Title, title)
		}
	}
	m := &Model{}
	if got := m.KeymapBindings("editor.nextTab"); len(got) != 2 ||
		got[0] != "Ctrl+Right" || got[1] != "Cmd+Shift+]" {
		t.Fatalf("next tab bindings = %v", got)
	}
	if got := m.KeymapBindings("editor.prevTab"); len(got) != 2 ||
		got[0] != "Ctrl+Left" || got[1] != "Cmd+Shift+[" {
		t.Fatalf("previous tab bindings = %v", got)
	}
}

func TestCodeLineActionsRunThroughTheRegistry(t *testing.T) {
	m := &Model{tabs: []*Tab{editorTab("a\nb\nc\n", 2)}}
	if !m.RunCommand("code.deleteLine") {
		t.Fatal("registry has no code.deleteLine")
	}
	if got := m.tabs[0].state.Buffer.String(); got != "a\nc\n" {
		t.Errorf("code.deleteLine = %q, want %q", got, "a\nc\n")
	}
}

func TestCodeLineActionsAreInTheRegistry(t *testing.T) {
	ids := map[string]bool{}
	for _, c := range commands() {
		ids[c.id] = true
	}
	for _, id := range []string{
		"code.commentLine", "code.commentBlock", "code.duplicateLine",
		"code.deleteLine", "code.moveLineUp", "code.moveLineDown",
		"code.moveStatementUp", "code.moveStatementDown", "code.joinLines",
	} {
		if !ids[id] {
			t.Errorf("registry has no %q", id)
		}
	}
}

func TestCodeMenuListsTheLineActions(t *testing.T) {
	var code menuEntry
	for _, g := range mainMenuGroups() {
		if g.label == "Code" {
			code = g
		}
	}
	want := []string{
		"code.commentLine", "code.commentBlock", "code.duplicateLine",
		"code.deleteLine", "code.moveLineUp", "code.moveLineDown",
		"code.moveStatementUp", "code.moveStatementDown", "code.joinLines",
	}
	found := map[string]bool{}
	for _, e := range code.sub {
		found[e.command] = true
	}
	for _, id := range want {
		if !found[id] {
			t.Errorf("Code menu has no %q", id)
		}
	}
}
