package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// keymapRecorderState is the small native modal that captures one key chord
// for the Keymap page. Nothing reaches prefs until Settings itself is applied.
type keymapRecorderState struct {
	open       bool
	actionID   string
	binding    string
	conflicts  []string
	captureErr string
}

// settingsRecordKeymap opens the shortcut recorder for one action.
func (m *Model) settingsRecordKeymap(actionID string) {
	actionID = canonicalActionID(strings.TrimSpace(actionID))
	if actionID == "" {
		return
	}
	m.keymapRecorder = keymapRecorderState{open: true, actionID: actionID}
	m.invalidate()
}

// settingsRecordKeymapInput captures key-down events before the UI handles
// them. Modifier-only presses are intentionally left in place while the
// dialog waits for the non-modifier key.
func (m *Model) settingsRecordKeymapInput(ev ui.InputEvent) bool {
	recorder := &m.keymapRecorder
	if !recorder.open || ev.Kind != ui.InputKeyDown {
		return false
	}
	if ev.Key == ui.KeyEscape && ev.Mods == 0 {
		m.settingsCloseKeymapRecorder()
		return true
	}
	if ev.Repeat {
		return true
	}
	binding, ok := keymapBindingFromInput(ev)
	if !ok {
		return true
	}
	recorder.binding = binding
	recorder.captureErr = ""
	recorder.conflicts = keymapConflictsForBinding(
		m.settingsDraft.keymapPreset,
		m.settingsDraft.keymap,
		recorder.actionID,
		binding,
	)
	m.invalidate()
	return true
}

// settingsCommitRecordedKeymap stages the captured binding. A conflict is a
// warning, not a reason to discard the user's deliberate choice.
func (m *Model) settingsCommitRecordedKeymap() {
	recorder := &m.keymapRecorder
	binding := keymapNormalize(recorder.binding)
	if !recorder.open || binding == "" {
		recorder.captureErr = "Press a key combination first."
		m.invalidate()
		return
	}
	overrides := cloneKeymap(m.settingsDraft.keymap)
	if overrides == nil {
		overrides = map[string][]string{}
	}
	overrides[recorder.actionID] = []string{binding}
	m.settingsDraft.keymap = overrides
	m.settingsCloseKeymapRecorder()
}

// settingsCloseKeymapRecorder dismisses the recorder without changing the
// staged keymap.
func (m *Model) settingsCloseKeymapRecorder() {
	m.keymapRecorder = keymapRecorderState{}
	m.invalidate()
}

// keymapConflictsForBinding lists actions already using binding in overrides.
// The action being edited is omitted because its old binding is about to be
// replaced.
func keymapConflictsForBinding(
	presetID string,
	overrides map[string][]string,
	actionID, binding string,
) []string {
	binding = keymapNormalize(binding)
	actionID = canonicalActionID(actionID)
	if binding == "" {
		return nil
	}
	seen := map[string]bool{}
	var conflicts []string
	for _, action := range actionList() {
		id := canonicalActionID(action.ID)
		if id == actionID || seen[id] {
			continue
		}
		for _, current := range keymapBindingsIn(presetID, overrides, id) {
			if current != binding {
				continue
			}
			seen[id] = true
			conflicts = append(conflicts, id)
			break
		}
	}
	return conflicts
}

// keymapBindingFromInput formats a captured event in the same normalized
// notation as the stored keymap.
func keymapBindingFromInput(ev ui.InputEvent) (string, bool) {
	key := ev.Key
	if key == ui.KeyUnknown {
		key = ev.PhysicalKey
	}
	name, ok := keymapKeyName(key)
	if !ok {
		return "", false
	}
	var parts []string
	ctrlAdded := false
	if ev.Mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
		ctrlAdded = true
	}
	if ev.Mods&ui.Cmd != 0 && (!ctrlAdded || ui.Cmd != ui.Ctrl) {
		parts = append(parts, "Cmd")
	}
	if ev.Mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if ev.Mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	parts = append(parts, name)
	return keymapNormalize(strings.Join(parts, "+")), true
}

// keymapKeyName maps MyGo's key codes to the names keymapNormalize stores.
// Letters and digits use their printable spelling so settings files remain
// readable.
func keymapKeyName(key ui.Key) (string, bool) {
	switch key {
	case ui.KeyEnter:
		return "Enter", true
	case ui.KeyEscape:
		return "Escape", true
	case ui.KeyBackspace:
		return "Backspace", true
	case ui.KeyTab:
		return "Tab", true
	case ui.KeySpace:
		return "Space", true
	case ui.KeyDelete:
		return "Delete", true
	case ui.KeyInsert:
		return "Insert", true
	case ui.KeyHome:
		return "Home", true
	case ui.KeyEnd:
		return "End", true
	case ui.KeyPageUp:
		return "PageUp", true
	case ui.KeyPageDown:
		return "PageDown", true
	case ui.KeyLeft:
		return "Left", true
	case ui.KeyRight:
		return "Right", true
	case ui.KeyUp:
		return "Up", true
	case ui.KeyDown:
		return "Down", true
	case ui.KeyF1:
		return "F1", true
	case ui.KeyF2:
		return "F2", true
	case ui.KeyF3:
		return "F3", true
	case ui.KeyF4:
		return "F4", true
	case ui.KeyF5:
		return "F5", true
	case ui.KeyF6:
		return "F6", true
	case ui.KeyF7:
		return "F7", true
	case ui.KeyF8:
		return "F8", true
	case ui.KeyF9:
		return "F9", true
	case ui.KeyF10:
		return "F10", true
	case ui.KeyF11:
		return "F11", true
	case ui.KeyF12:
		return "F12", true
	case ui.KeyMinus:
		return "-", true
	case ui.KeyEqual:
		return "=", true
	case ui.KeyComma:
		return ",", true
	case ui.KeyPeriod:
		return ".", true
	case ui.KeySlash:
		return "/", true
	case ui.KeySemicolon:
		return ";", true
	case ui.KeyQuote:
		return "'", true
	case ui.KeyBracketLeft:
		return "[", true
	case ui.KeyBracketRight:
		return "]", true
	case ui.KeyBackslash:
		return "\\", true
	case ui.KeyBackquote:
		return "`", true
	case ui.Key0:
		return "0", true
	case ui.Key1:
		return "1", true
	case ui.Key2:
		return "2", true
	case ui.Key3:
		return "3", true
	case ui.Key4:
		return "4", true
	case ui.Key5:
		return "5", true
	case ui.Key6:
		return "6", true
	case ui.Key7:
		return "7", true
	case ui.Key8:
		return "8", true
	case ui.Key9:
		return "9", true
	case ui.KeyA:
		return "A", true
	case ui.KeyB:
		return "B", true
	case ui.KeyC:
		return "C", true
	case ui.KeyD:
		return "D", true
	case ui.KeyE:
		return "E", true
	case ui.KeyF:
		return "F", true
	case ui.KeyG:
		return "G", true
	case ui.KeyH:
		return "H", true
	case ui.KeyI:
		return "I", true
	case ui.KeyJ:
		return "J", true
	case ui.KeyK:
		return "K", true
	case ui.KeyL:
		return "L", true
	case ui.KeyM:
		return "M", true
	case ui.KeyN:
		return "N", true
	case ui.KeyO:
		return "O", true
	case ui.KeyP:
		return "P", true
	case ui.KeyQ:
		return "Q", true
	case ui.KeyR:
		return "R", true
	case ui.KeyS:
		return "S", true
	case ui.KeyT:
		return "T", true
	case ui.KeyU:
		return "U", true
	case ui.KeyV:
		return "V", true
	case ui.KeyW:
		return "W", true
	case ui.KeyX:
		return "X", true
	case ui.KeyY:
		return "Y", true
	case ui.KeyZ:
		return "Z", true
	default:
		return "", false
	}
}

// settingsKeymapRecorderDialog is the native shortcut-capture sheet. It is
// built after the Settings modal so it owns keyboard focus until Save or
// Cancel.
func (m *Model) settingsKeymapRecorderDialog(c *ui.Context, p *ui.Theme) {
	if !m.keymapRecorder.open || !m.settingsOpen {
		return
	}
	oi := oiPaletteOf(p)
	open := m.keymapRecorder.open
	dlg := ui.Modal(c.Key("settings:keymap-recorder"), &open, func() {
		ui.Column(c).Width(460).Padding(18).Gap(12).
			Background(oi.elev).Border(1, oi.border).Radius(6).
			Children(func() {
				ui.Text(c, "Set Keyboard Shortcut").FontSize(14).Bold().
					TextColor(oi.textStrong).SingleLine()
				ui.Text(c, m.keymapRecorderActionTitle()).FontSize(12).
					TextColor(oi.text).SingleLine().Ellipsis("…")
				capture := ui.Box(c.Key("settings:keymap-capture")).Height(64).
					Padding(12).Background(oi.frame).Border(1, oi.borderStrong).Radius(5).
					Focusable().AutoFocus().HandleInput(m.settingsRecordKeymapInput)
				capture.Children(func() {
					label := m.keymapRecorder.binding
					if label == "" {
						label = "Press a key combination"
					}
					ui.Text(c, label).FontSize(14).Bold().TextColor(oi.textStrong).
						SingleLine().Ellipsis("…")
				})
				if len(m.keymapRecorder.conflicts) > 0 {
					ui.Text(c, "Also used by "+strings.Join(m.keymapRecorder.conflicts, ", ")).
						FontSize(12).TextColor(oi.warning).SingleLine().Ellipsis("…")
				}
				if m.keymapRecorder.captureErr != "" {
					ui.Text(c, m.keymapRecorder.captureErr).FontSize(12).
						TextColor(oi.danger).SingleLine().Ellipsis("…")
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					ui.Button(c, "Cancel").OnClick(m.settingsCloseKeymapRecorder)
					ui.Button(c, "Save").Disabled(m.keymapRecorder.binding == "").
						OnClick(m.settingsCommitRecordedKeymap)
				})
			})
	})
	if dlg.Dismissed() || !open {
		m.settingsCloseKeymapRecorder()
	}
}

func (m *Model) keymapRecorderActionTitle() string {
	id := m.keymapRecorder.actionID
	for _, cmd := range commands() {
		if canonicalActionID(cmd.id) == id {
			return cmd.title
		}
	}
	return fmt.Sprintf("Action %s", id)
}
