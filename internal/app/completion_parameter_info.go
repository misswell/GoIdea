package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

func (m *Model) scheduleParameterInfoForInput(t *Tab, ev ui.InputEvent) {
	if t == nil || t.state == nil {
		return
	}
	trigger, ok := parameterInfoTrigger(ev)
	if !ok {
		if ev.Kind == ui.InputPointerDown || parameterInfoMovesCaret(ev) {
			t.signatureGen++
		}
		return
	}
	if !m.completionParameterInfo() {
		t.signatureGen++
		return
	}
	text := t.doc.Text()
	caret := t.state.Cursor()
	if caret < 0 || caret > len(text) {
		return
	}
	gen := t.signatureGen + 1
	t.signatureGen = gen
	docID := t.doc.ID()
	delay := m.completionParameterInfoDelay()
	time.AfterFunc(delay, func() {
		m.post(func() {
			if !m.tabOpen(t) || t.signatureGen != gen || t.doc.ID() != docID ||
				t.state == nil || !t.editorFocused {
				return
			}
			current := t.doc.Text()
			currentCaret := t.state.Cursor()
			triggerEnd := caret + 1
			if caret < 0 || triggerEnd > len(current) || current[caret:triggerEnd] != trigger ||
				currentCaret < triggerEnd || lineForOffset(current, currentCaret) != lineForOffset(current, caret) {
				return
			}
			m.ShowParameterInfoAt(t)
		})
	})
}

func parameterInfoMovesCaret(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	switch ev.Key {
	case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd,
		ui.KeyPageUp, ui.KeyPageDown, ui.KeyEscape:
		return true
	default:
		return false
	}
}

func parameterInfoTrigger(ev ui.InputEvent) (string, bool) {
	if ev.Kind == ui.InputText {
		if ev.Text == "(" || ev.Text == "," {
			return ev.Text, true
		}
		return "", false
	}
	if ev.Kind != ui.InputKeyDown {
		return "", false
	}
	key := ev.Key
	if key == ui.KeyUnknown {
		key = ev.PhysicalKey
	}
	if key == ui.KeyComma && ev.Mods == 0 {
		return ",", true
	}
	if key == ui.Key9 && ev.Mods == ui.Shift {
		return "(", true
	}
	return "", false
}
