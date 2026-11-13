package app

import (
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
