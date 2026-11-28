package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestNextOccurrenceShortcutKeepsWholeWordsAndWaitsBeforeWrapping(t *testing.T) {
	text := "词$ 词$Suffix 词$ 词$"
	start := strings.Index(text, " 词$ 词$") + 1
	tab := editorTab(text, start+len("词$"))
	tab.editorFocused = true
	m := &Model{tabs: []*Tab{tab}, prefs: Preferences{KeymapPreset: "intellij-macos"}}
	m.applyKeymapPreferences()
	view := ui.NewTester(func(c *ui.Context) {
		m.shortcuts(c)
		editor.View(c, tab.state).Fill().AutoFocus()
	}, 720, 320)
	view.Frame()
	press := func() {
		view.Key(ui.Ctrl, ui.KeyG)
		view.Frame()
	}
	press()
	if got := tab.state.Selection(); got.Start() != start || got.End() != start+len("词$") {
		t.Fatalf("selected word at its right edge: %+v", got)
	}
	press()
	if got := tab.state.Selection().Start(); got != strings.LastIndex(text, "词$") {
		t.Fatalf("next word offset = %d", got)
	}
	press()
	if got := len(tab.state.Selections()); got != 2 || !strings.Contains(m.status, "press again") {
		t.Fatalf("at end: %d selections, status %q", got, m.status)
	}
	press()
	if got := tab.state.Selection().Start(); got != 0 || len(tab.state.Selections()) != 3 {
		t.Fatalf("wrapped word offset = %d, selections = %+v", got, tab.state.Selections())
	}
	press()
	if got := tab.state.Selection().Start(); got != 0 || len(tab.state.Selections()) != 3 {
		t.Fatalf("selected hit changed the primary: %+v", tab.state.Selections())
	}
}

func TestSelectAllOccurrencesKeepsDirectionAndPrimary(t *testing.T) {
	tab := editorTab("Cat cat scats cat", 0)
	m := &Model{tabs: []*Tab{tab}}
	tab.state.Select(7, 4)
	if !m.RunCommand("code.selectAllOccurrences") {
		t.Fatal("Select All Occurrences is unavailable")
	}
	want := []editor.Selection{{Anchor: 7, Head: 4}, {Anchor: 12, Head: 9}, {Anchor: 17, Head: 14}}
	if got := tab.state.Selections(); !slices.Equal(got, want) || tab.state.Selection() != want[0] {
		t.Fatalf("all reversed occurrences = %+v, primary %+v", got, tab.state.Selection())
	}
	tab.state.Select(17, 17)
	m.RunCommand("code.selectAllOccurrences")
	want = []editor.Selection{{Anchor: 4, Head: 7}, {Anchor: 14, Head: 17}}
	if got := tab.state.Selections(); !slices.Equal(got, want) || tab.state.Selection() != want[1] {
		t.Fatalf("whole-word occurrences = %+v, primary %+v", got, tab.state.Selection())
	}
}
