package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestColumnSelectionExtendsVisibleColumnsLikeOpenIDEA(t *testing.T) {
	text := "abcd\nef\nijkl"
	state := &editor.State{Buffer: editor.NewBuffer(text), LineHeight: editorLineHeight(13)}
	state.SetSelections(editor.Selection{Anchor: 1, Head: 1})
	tab := &Tab{state: state}
	m := &Model{tabs: []*Tab{tab}, active: 0}

	m.ToggleColumnSelection()
	if !tab.columnMode || !m.columnSelectionEnabled() {
		t.Fatal("Column Selection Mode did not turn on")
	}
	if !m.extendColumnSelection(tab, 1, 0) {
		t.Fatal("Shift+Down did not extend the column selection")
	}
	if !m.extendColumnSelection(tab, 1, 0) {
		t.Fatal("second Shift+Down did not extend the column selection")
	}
	if !m.extendColumnSelection(tab, 0, 1) {
		t.Fatal("Shift+Right did not extend the column selection")
	}
	if !m.extendColumnSelection(tab, 0, 1) {
		t.Fatal("second Shift+Right did not extend the column selection")
	}
	selections := state.Selections()
	if len(selections) != 3 {
		t.Fatalf("column selections = %+v, want three rows", selections)
	}
	want := []editor.Selection{
		{Anchor: 1, Head: 3},
		{Anchor: len("abcd\n") + 1, Head: len("abcd\nef")},
		{Anchor: len("abcd\nef\n") + 1, Head: len("abcd\nef\n") + 3},
	}
	for i := range want {
		if selections[i] != want[i] {
			t.Fatalf("row %d selection = %+v, want %+v", i, selections[i], want[i])
		}
	}

	if !m.extendColumnSelection(tab, -1, 0) {
		t.Fatal("Shift+Up did not shrink the column selection")
	}
	selections = state.Selections()
	if len(selections) != 2 || selections[0] != want[0] || selections[1] != want[1] {
		t.Fatalf("Shift+Up selection = %+v, want %+v", selections, want[:2])
	}
}

func TestColumnSelectionStepsWideCharactersAndTabsInVisualColumns(t *testing.T) {
	text := "张三四\nabcdef"
	state := &editor.State{Buffer: editor.NewBuffer(text), LineHeight: editorLineHeight(13)}
	state.SetSelections(editor.Selection{Anchor: 0, Head: 0})
	tab := &Tab{state: state, columnMode: true}
	m := &Model{tabs: []*Tab{tab}, active: 0}

	if !m.extendColumnSelection(tab, 0, 1) || !m.extendColumnSelection(tab, 1, 0) {
		t.Fatal("wide-character column selection did not extend")
	}
	wide := state.Selections()
	if got, want := wide, []editor.Selection{
		{Anchor: 0, Head: len("张")},
		{Anchor: len("张三四\n"), Head: len("张三四\nab")},
	}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("wide-character selection = %+v, want %+v", got, want)
	}

	state = &editor.State{Buffer: editor.NewBuffer("\tab\n    cd"), LineHeight: editorLineHeight(13)}
	state.SetSelections(editor.Selection{Anchor: 1, Head: 1})
	tab = &Tab{state: state, columnMode: true}
	m = &Model{tabs: []*Tab{tab}, active: 0}
	if !m.extendColumnSelection(tab, 1, 0) || !m.extendColumnSelection(tab, 0, 1) {
		t.Fatal("tab column selection did not extend")
	}
	tabs := state.Selections()
	if got, want := tabs, []editor.Selection{
		{Anchor: 1, Head: 2},
		{Anchor: len("\tab\n    "), Head: len("\tab\n    c")},
	}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("tab selection = %+v, want %+v", got, want)
	}
}

func TestColumnSelectionMouseDragBecomesRectangularAcrossRows(t *testing.T) {
	text := "alpha\nbravo\ncharlie\n"
	state := &editor.State{Buffer: editor.NewBuffer(text), LineHeight: editorLineHeight(13)}
	tab := &Tab{state: state, columnMode: true}
	m := &Model{tabs: []*Tab{tab}, active: 0}
	state.SetSelections(editor.Selection{
		Anchor: 1,
		Head:   len("alpha\nbravo\nch"),
	})

	m.finishColumnPointerSelection(tab)
	selections := state.Selections()
	if len(selections) != 3 {
		t.Fatalf("mouse column drag selections = %+v, want three rows", selections)
	}
	want := []editor.Selection{
		{Anchor: 1, Head: 2},
		{Anchor: len("alpha\n") + 1, Head: len("alpha\nbr")},
		{Anchor: len("alpha\nbravo\n") + 1, Head: len("alpha\nbravo\nch")},
	}
	for i := range want {
		if selections[i] != want[i] {
			t.Fatalf("mouse row %d = %+v, want %+v", i, selections[i], want[i])
		}
	}
}

func TestColumnSelectionInputClaimsOnlyShiftArrows(t *testing.T) {
	state := &editor.State{Buffer: editor.NewBuffer("abc\ndef\n"), LineHeight: editorLineHeight(13)}
	state.SetSelections(editor.Selection{Anchor: 0, Head: 0})
	tab := &Tab{state: state, columnMode: true}
	m := &Model{tabs: []*Tab{tab}, active: 0}

	if !m.columnInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Mods: ui.Shift, Key: ui.KeyDown}) {
		t.Fatal("Shift+Down was not claimed")
	}
	if m.columnInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Mods: 0, Key: ui.KeyDown}) {
		t.Fatal("plain Down was claimed by column mode")
	}
}
