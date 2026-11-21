package app

import (
	"reflect"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// caretClickState bridges MyGo's rectangular Alt+Shift press into OpenIDEA's
// Add or Remove Caret gesture. MyGo applies the press as a one-line column
// selection before releasing it; observing the resulting public selection on
// pointer-up lets the native UI restore the old carets and add or remove the
// clicked one without guessing pixel-to-offset geometry.
type caretClickState struct {
	before []editor.Selection
	moved  bool
}

func (m *Model) editorCaretInput(t *Tab, ev ui.InputEvent) bool {
	if t == nil || t.state == nil {
		return false
	}
	switch ev.Kind {
	case ui.InputScroll:
		zoomModifier := ev.Mods&ui.Cmd != 0 || ev.Mods&ui.Ctrl != 0
		if !m.prefs.General.WheelZoom || !zoomModifier || ev.DY == 0 {
			return false
		}
		if ev.DY < 0 {
			m.stepEditorFontZoom(1)
		} else {
			m.stepEditorFontZoom(-1)
		}
		return true
	case ui.InputKeyDown:
		if m.prefs.General.CaretScroll == "center" && caretMovementKey(ev.Key) {
			t.centerCaret = true
			m.invalidate()
		}
	case ui.InputPointerDown:
		t.caret = nil
		if ev.Button == 0 && ev.Clicks == 1 &&
			ev.Mods&ui.Alt != 0 && ev.Mods&ui.Shift != 0 {
			t.caret = &caretClickState{before: t.state.Selections()}
		}
	case ui.InputPointerMove:
		if t.caret != nil {
			t.caret.moved = true
		}
	case ui.InputPointerUp:
		click := t.caret
		t.caret = nil
		if click == nil || click.moved || len(click.before) == 0 {
			return false
		}
		after := t.state.Selections()
		if len(after) != 1 {
			return false
		}
		target := after[0]
		merged := append([]editor.Selection(nil), click.before...)
		removed := false
		for i, selection := range merged {
			if reflect.DeepEqual(selection, target) {
				merged = append(merged[:i], merged[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			merged = append(merged, target)
		}
		if len(merged) == 0 {
			return false
		}
		t.state.SetSelections(merged...)
		m.invalidate()
		// Let the editor clear its own gesture on pointer-up. It keeps the
		// selections SetSelections installed.
		return false
	}
	return false
}

// caretMovementKey reports keys that move the editor caret rather than edit
// text. Center-caret scrolling applies to those moves only.
func caretMovementKey(key ui.Key) bool {
	switch key {
	case ui.KeyUp, ui.KeyDown, ui.KeyLeft, ui.KeyRight,
		ui.KeyPageUp, ui.KeyPageDown, ui.KeyHome, ui.KeyEnd:
		return true
	default:
		return false
	}
}

// applyDeferredCaretCenter runs after the editor has handled the preceding
// key and its view has been laid out. Centering in the input callback would
// act on the old caret position.
func (m *Model) applyDeferredCaretCenter(t *Tab) {
	if t == nil || t.state == nil || !t.centerCaret {
		return
	}
	t.centerCaret = false
	if m.prefs.General.CaretScroll != "center" {
		return
	}
	if t.state.Run(editor.ScrollCursorCenter) {
		m.invalidate()
	}
}

// takeEditorDroppedFiles asks MyGo for a file drop only when the setting
// permits it. Not querying is important: an element that never asks does not
// claim the window's drop.
func (m *Model) takeEditorDroppedFiles(view ui.Element) []string {
	if !m.prefs.General.dragAndDrop() {
		return nil
	}
	return view.DroppedFiles()
}
