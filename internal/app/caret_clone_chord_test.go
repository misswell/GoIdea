package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestCaretCloneChordRoutesThroughWorkbenchAndCancelsInOppositeDirection(t *testing.T) {
	text := "alpha\nbeta\ngamma\n"
	tab := editorTab(text, 0)
	tab.editorFocused = true
	m := &Model{
		tabs:   []*Tab{tab},
		active: 0,
		prefs: Preferences{
			KeymapPreset: "intellij-macos",
			Keymap: map[string][]string{
				"editor.cloneCaretAbove": {"Alt Alt+Up"},
				"editor.cloneCaretBelow": {"Alt Alt+Down"},
			},
		},
	}
	tab.state.OnInput = func(ev ui.InputEvent) bool {
		return m.editorCaretInput(tab, ev)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.shortcuts(c)
		view := editor.View(c, tab.state).Fill().AutoFocus()
		tab.editorFocused = view.Focused() || view.FocusWithin()
	}, 800, 300)
	tt.SetFocused(true)
	tt.Frame()
	tab.editorFocused = true
	if !m.keymapHasBinding("editor.cloneCaretBelow", "Alt Alt+Down") {
		t.Fatalf("model bindings lack Alt Alt+Down: %v", m.KeymapBindings("editor.cloneCaretBelow"))
	}
	if !m.ActionEnabled("editor.cloneCaretBelow") || !m.activeEditorFocused() {
		t.Fatalf("clone action availability = %v focus=%v", m.ActionEnabled("editor.cloneCaretBelow"), m.activeEditorFocused())
	}

	for range 2 {
		tt.HoldModifiers(ui.Alt)
		tt.HoldModifiers(0)
	}
	if !m.caretCloneChord.armed {
		t.Fatalf("double Alt did not arm the caret chord: %+v", m.caretCloneChord)
	}
	if len(m.tabs) != 1 || m.tab() != tab {
		t.Fatalf("double Alt changed the editor before the direction key: active=%d tabs=%v", m.active, m.tabs)
	}
	tt.Key(ui.Alt, ui.KeyDown)
	tt.Frame()
	if got := selectionHeads(tab.state); len(got) != 2 ||
		got[0] != 0 || got[1] != strings.Index(text, "beta") {
		t.Fatalf("first double-Alt Down carets = %+v, focus=%v, chord=%+v, bindings=%v",
			got, tab.editorFocused, m.caretCloneChord,
			m.KeymapBindings("editor.cloneCaretBelow"))
	}
	tt.Key(ui.Alt, ui.KeyDown)
	tt.Frame()
	if got := selectionHeads(tab.state); len(got) != 3 {
		t.Fatalf("second double-Alt Down carets = %+v", got)
	}
	tt.Key(ui.Alt, ui.KeyUp)
	tt.Frame()
	if got := selectionHeads(tab.state); len(got) != 2 {
		t.Fatalf("opposite double-Alt Up did not cancel the newest caret: %+v", got)
	}
}

func TestCaretCloneChordExpiresAfterOpenIDEAStep(t *testing.T) {
	tab := editorTab("one\ntwo\nthree\n", 0)
	tab.editorFocused = true
	m := &Model{
		tabs:   []*Tab{tab},
		active: 0,
		prefs: Preferences{
			KeymapPreset: "intellij-macos",
			Keymap: map[string][]string{
				"editor.cloneCaretBelow": {"Alt Alt+Down"},
			},
		},
	}
	tab.state.OnInput = func(ev ui.InputEvent) bool {
		return m.editorCaretInput(tab, ev)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.shortcuts(c)
		view := editor.View(c, tab.state).Fill().AutoFocus()
		tab.editorFocused = view.Focused() || view.FocusWithin()
	}, 800, 300)
	tt.Frame()
	tab.editorFocused = true
	for range 2 {
		tt.HoldModifiers(ui.Alt)
		tt.HoldModifiers(0)
	}
	tt.Advance(301 * time.Millisecond)
	tt.Key(ui.Alt, ui.KeyDown)
	tt.Frame()
	if got := selectionHeads(tab.state); len(got) != 1 {
		t.Fatalf("an expired double-Alt chord added carets: %+v", got)
	}
}

func selectionHeads(state *editor.State) []int {
	selections := state.Selections()
	out := make([]int, len(selections))
	for i, selection := range selections {
		out[i] = selection.Head
	}
	return out
}
