package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestAltShiftClickAddsRemovesAndEditsOpenIDEACarets(t *testing.T) {
	state := &editor.State{
		Buffer:     editor.NewBuffer("alpha\nbeta\ngamma\n"),
		LineHeight: editorLineHeight(13),
	}
	tab := &Tab{state: state}
	m := &Model{}
	state.OnInput = func(ev ui.InputEvent) bool { return m.editorCaretInput(tab, ev) }
	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, state).Fill().AutoFocus()
	}, 600, 400)

	firstY := float32(state.LineTop(0) + float64(state.LineHeight)/2)
	secondY := float32(state.LineTop(1) + float64(state.LineHeight)/2)
	tester.ClickAt(100, firstY)
	before := state.Selections()
	if len(before) != 1 || !before[0].Empty() {
		t.Fatalf("first click selections = %+v", before)
	}

	tester.ClickAtWith(ui.Alt|ui.Shift, 150, secondY)
	added := state.Selections()
	if len(added) != 2 {
		t.Fatalf("Alt+Shift click selections = %+v, want two carets", added)
	}

	tester.Type("X")
	text := state.Buffer.String()
	if strings.Count(text, "X") != 2 {
		t.Fatalf("typing at two carets produced %q", text)
	}

	tester.Advance(time.Second)
	tester.ClickAtWith(ui.Alt|ui.Shift, 150, secondY)
	remaining := state.Selections()
	if len(remaining) != 1 {
		t.Fatalf("second Alt+Shift click selections = %+v, want one caret", remaining)
	}
}

func TestKeyboardSelectionSettlesIntoDebugValueHover(t *testing.T) {
	text := "order.items\n"
	tab := debugEditorTestTab(text)
	tab.state.Select(
		strings.Index(text, "order.items"),
		strings.Index(text, "order.items")+len("order.items"),
	)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
	}
	m.debug.paused = true
	m.debug.session = 7
	m.debug.frameID = 31

	if m.editorCaretInput(tab, ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyRight}) {
		t.Fatal("keyboard selection release was consumed")
	}
	if tab.debugSelectTimer == nil {
		t.Fatal("keyboard selection release did not settle into a value hover")
	}
	m.cancelDebugSelectionHover(tab)
}

func TestKeyboardSelectionMovementCancelsTheOldValueHover(t *testing.T) {
	text := "first + second\n"
	tab := debugEditorTestTab(text)
	tab.state.Select(0, len("first"))
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
	}
	m.debug.paused = true
	m.debug.session = 7
	m.debug.frameID = 31
	m.scheduleDebugSelectionHover(tab)
	generation := tab.debugSelectGen

	m.editorCaretInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyRight})
	if tab.debugSelectTimer != nil || tab.debugSelectGen <= generation {
		t.Fatal("moving the keyboard selection kept the previous value hover")
	}
}
