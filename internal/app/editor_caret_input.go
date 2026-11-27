package app

import (
	"reflect"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const caretCloneChordTimeout = 300 * time.Millisecond

type caretCloneChordState struct {
	previousMods ui.Modifiers
	tappedAt     time.Time
	armed        bool
	armedAt      time.Time
	direction    int
	recentAt     time.Time
}

// caretClickState bridges MyGo's rectangular Alt+Shift press into OpenIDEA's
// Add or Remove Caret gesture. MyGo applies the press as a one-line column
// selection before releasing it; observing the resulting public selection on
// pointer-up lets the native UI restore the old carets and add or remove the
// clicked one without guessing pixel-to-offset geometry.
type caretClickState struct {
	before []editor.Selection
	moved  bool
}

// cloneCaretWithOppositeCancellation preserves OpenIDEA's Add or Remove Caret
// gesture across a two-modifier chord. MyGo's add-cursor stack already removes
// the newest caret when the next command goes the other way.
func (m *Model) cloneCaretWithOppositeCancellation(
	t *Tab,
	direction int,
	now time.Time,
) bool {
	if direction == 0 {
		return false
	}
	command := editor.AddCursorAbove
	if direction > 0 {
		command = editor.AddCursorBelow
	}
	if !t.state.Run(command) {
		return false
	}
	state := &m.caretCloneChord
	state.direction = direction
	state.armedAt = now
	state.recentAt = state.armedAt
	m.invalidate()
	return true
}

func (m *Model) editorCaretInput(t *Tab, ev ui.InputEvent) bool {
	if t == nil || t.state == nil {
		return false
	}
	if ev.Kind == ui.InputKeyDown && m.caretCloneChordConsumes(t, ev) {
		return true
	}
	if m.overwriteEditorInput(t, ev) {
		return true
	}
	m.scheduleParameterInfoForInput(t, ev)
	if autoImportInputMayEdit(ev) {
		m.markAutoImportDirty(t)
	}
	if ev.Kind == ui.InputPointerDown || ev.Kind == ui.InputKeyDown {
		m.cancelDebugSelectionHover(t)
	}
	m.recordMacroEditorInput(ev)
	if m.vimInput(t, ev) {
		return true
	}
	if m.completionInput(t, ev) {
		return true
	}
	if m.tabTemplateInput(t, ev) {
		return true
	}
	if m.columnInput(t, ev) {
		return true
	}
	if m.inlineCompletionInput(t, ev) {
		return true
	}
	if m.smartKeyInput(t, ev) {
		return true
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
		if ev.Mods&(ui.Cmd|ui.Ctrl) != 0 && ev.Key == ui.KeyV {
			if m.pasteMarkdownImage(t) {
				return true
			}
			if m.pasteHTTPRequest(t) {
				return true
			}
		}
		if ev.Mods&(ui.Cmd|ui.Ctrl) != 0 && (ev.Key == ui.KeyC || ev.Key == ui.KeyX) {
			// Keyboard copy/cut may be handled by the app command rather than
			// this editor callback. Recording here preserves the pre-cut text
			// for either dispatch order; the history deduplicates a second call.
			m.rememberActiveEditorClipboard()
			if ev.Key == ui.KeyC &&
				m.nativeRuntimeBool(settingsPageRichTextCopy, "editor.general.richTextCopy") &&
				m.copyEditorRichText(t) {
				return true
			}
		}
		if ev.Key == ui.KeyEnter && (ev.Mods == ui.Cmd || ev.Mods == ui.Ctrl) &&
			t.doc != nil && isHTTPRequestFile(t.doc.Path()) {
			m.RunHTTPRequestAtCaret()
			return true
		}
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
		if t.caret == nil {
			m.scheduleDebugSelectionHover(t)
		}
		m.finishColumnPointerSelection(t)
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
	case ui.InputKeyUp:
		m.scheduleDebugSelectionHover(t)
	}
	return false
}

func (m *Model) observeCaretCloneChord(c *ui.Context) bool {
	if !m.activeEditorFocused() {
		return false
	}
	mods := c.Modifiers()
	now := c.Now()
	state := &m.caretCloneChord
	if state.armed {
		if (state.direction != 0 && now.Sub(state.recentAt) <= caretCloneChordTimeout) ||
			now.Sub(state.armedAt) <= caretCloneChordTimeout {
			state.previousMods = mods
			return false
		}
		*state = caretCloneChordState{}
	}
	modifier := ui.Modifiers(0)
	switch mods {
	case ui.Alt:
		modifier = ui.Alt
	case ui.Ctrl:
		modifier = ui.Ctrl
	}
	if modifier != 0 {
		if state.previousMods == modifier && !state.tappedAt.IsZero() &&
			now.Sub(state.tappedAt) >= 0 && now.Sub(state.tappedAt) <= caretCloneChordTimeout {
			state.armed = true
			state.armedAt = now
			state.tappedAt = time.Time{}
			state.direction = 0
			state.recentAt = time.Time{}
			m.invalidate()
			return true
		}
		state.previousMods = modifier
		state.tappedAt = now
		return false
	}
	if !state.tappedAt.IsZero() && now.Sub(state.tappedAt) <= caretCloneChordTimeout {
		return false
	}
	state.tappedAt = time.Time{}
	state.previousMods = mods
	return false
}

func (m *Model) caretCloneChordConsumes(t *Tab, ev ui.InputEvent) bool {
	state := &m.caretCloneChord
	if !state.armed || ev.Repeat {
		return false
	}
	if ev.Time.Sub(state.armedAt) < 0 || ev.Time.Sub(state.armedAt) > caretCloneChordTimeout {
		*state = caretCloneChordState{}
		return false
	}
	direction := 0
	modifier := ""
	switch ev.Key {
	case ui.KeyUp:
		direction = -1
		modifier = "Up"
	case ui.KeyDown:
		direction = 1
		modifier = "Down"
	default:
		return false
	}
	modPrefix := ""
	switch ev.Mods {
	case ui.Alt:
		modPrefix = "Alt"
	case ui.Ctrl:
		modPrefix = "Ctrl"
	default:
		*state = caretCloneChordState{}
		return false
	}
	binding := modPrefix + " " + modPrefix + "+" + modifier
	id := "editor.cloneCaretAbove"
	if direction > 0 {
		id = "editor.cloneCaretBelow"
	}
	if m.activeEditorFocused() && m.tab() == t &&
		m.keymapHasBinding(id, binding) {
		return m.cloneCaretWithOppositeCancellation(t, direction, ev.Time)
	}
	*state = caretCloneChordState{}
	return false
}

// caretCloneChordShortcut claims the direction key in the frame's shortcut
// pass. Waiting for the editor's OnInput is too late because the generic
// keymap can consume Alt+Down as Shrink Selection or Move Statement Down.
func (m *Model) caretCloneChordShortcut(c *ui.Context) bool {
	if c == nil {
		return false
	}
	state := &m.caretCloneChord
	if !state.armed {
		return false
	}
	now := c.Now()
	if now.Sub(state.armedAt) < 0 || now.Sub(state.armedAt) > caretCloneChordTimeout {
		*state = caretCloneChordState{}
		return false
	}
	t := m.tab()
	if t == nil || !m.activeEditorFocused() {
		*state = caretCloneChordState{}
		return false
	}
	for _, gesture := range []struct {
		mods      ui.Modifiers
		prefix    string
		key       ui.Key
		direction int
		action    string
	}{
		{ui.Alt, "Alt", ui.KeyUp, -1, "editor.cloneCaretAbove"},
		{ui.Alt, "Alt", ui.KeyDown, 1, "editor.cloneCaretBelow"},
		{ui.Ctrl, "Ctrl", ui.KeyUp, -1, "editor.cloneCaretAbove"},
		{ui.Ctrl, "Ctrl", ui.KeyDown, 1, "editor.cloneCaretBelow"},
	} {
		if !c.Shortcut(gesture.mods, gesture.key) {
			continue
		}
		binding := gesture.prefix + " " + gesture.prefix
		if gesture.key == ui.KeyUp {
			binding += "+Up"
		} else {
			binding += "+Down"
		}
		if m.keymapHasBinding(gesture.action, binding) {
			return m.cloneCaretWithOppositeCancellation(t, gesture.direction, now)
		}
	}
	return false
}

func autoImportInputMayEdit(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputText, ui.InputCompose, ui.InputCommand:
		return ev.Text != ""
	case ui.InputKeyDown:
		if ev.Mods&(ui.Cmd|ui.Ctrl) != 0 {
			switch ev.Key {
			case ui.KeyV, ui.KeyX, ui.KeyZ:
				return true
			}
		}
		return ev.Mods == 0 &&
			(ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete || ev.Key == ui.KeyEnter)
	default:
		return false
	}
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
	dropped := view.DroppedFiles()
	if m.dropMarkdownImages(m.tab(), dropped) {
		return nil
	}
	return dropped
}
