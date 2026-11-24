package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	terminalClipboardRead  = mygo.Clipboard.ReadText
	terminalClipboardWrite = mygo.Clipboard.WriteText
)

// terminalKeySequence maps a MyGo input event to the bytes an xterm-compatible
// PTY expects. Printable text arrives as InputText; key-down events cover the
// control and navigation keys that do not type text.
func terminalKeySequence(ev ui.InputEvent, optionAsMeta bool) (string, bool) {
	switch ev.Kind {
	case ui.InputText:
		if ev.Text == "" {
			return "", false
		}
		if ev.Mods&ui.Super != 0 {
			return "", false
		}
		if optionAsMeta && ev.Mods&ui.Alt != 0 {
			return "\x1b" + ev.Text, true
		}
		return ev.Text, true
	case ui.InputKeyDown:
	default:
		return "", false
	}

	if ev.Mods&ui.Super != 0 {
		// Command shortcuts belong to GoIDEA until a command explicitly
		// forwards them to the terminal.
		return "", false
	}
	key := ev.Key
	if key == ui.KeyUnknown {
		key = ev.PhysicalKey
	}
	if key == ui.KeyUnknown {
		return "", false
	}
	mods := ev.Mods &^ ui.Super
	if sequence, ok := terminalSpecialKeySequence(key, mods); ok {
		if mods&ui.Alt != 0 && !strings.HasPrefix(sequence, "\x1b") {
			sequence = "\x1b" + sequence
		}
		return sequence, true
	}
	if mods&ui.Ctrl != 0 {
		if control, ok := terminalControlByte(key); ok {
			if mods&ui.Alt != 0 {
				return "\x1b" + string(control), true
			}
			return string(control), true
		}
	}
	// A printable key without Command reports its text through InputText.
	return "", false
}

func (m *Model) handleTerminalInput(
	group int,
	session *termSession,
	ev ui.InputEvent,
	cellWidth, cellHeight float32,
) bool {
	if session == nil {
		return false
	}
	if ev.Kind == ui.InputPointerDown {
		m.ActivateTermGroup(group)
	}
	if sequence, ok := terminalMouseSequence(
		session, ev, cellWidth, cellHeight, m.terminalMouseReporting(),
	); ok {
		return m.queueTerminalInput(session, sequence)
	}
	if ev.Kind == ui.InputPointerDown && ev.Button == 2 &&
		m.terminalPasteOnMiddleClick() && session.id != 0 {
		return m.pasteTerminalClipboard(session)
	}
	if ev.Kind == ui.InputCommand {
		switch ev.Text {
		case "paste":
			return m.pasteTerminalClipboard(session)
		case "delete":
			return m.queueTerminalInput(session, "\x7f")
		}
		return false
	}
	key := ev.Key
	if key == ui.KeyUnknown {
		key = ev.PhysicalKey
	}
	if ev.Kind == ui.InputKeyUp && session.optionMetaKey == key {
		session.optionMetaKey = 0
		return true
	}
	if ev.Kind == ui.InputText && session.optionMetaKey != 0 {
		// The key-down already carried the Option-modified key as ESC + key.
		session.optionMetaKey = 0
		return true
	}
	if ev.Kind == ui.InputKeyDown && session.optionMetaKey != 0 {
		session.optionMetaKey = 0
	}
	if ev.Kind == ui.InputKeyDown && ev.Mods&ui.Alt != 0 &&
		ev.Mods&ui.Super == 0 && m.terminalOptionAsMeta() {
		text, ok := terminalPrintableKey(key, ev.Mods&ui.Shift != 0)
		if ok {
			session.optionMetaKey = key
			return m.queueTerminalInput(session, "\x1b"+text)
		}
	}
	if ev.Kind == ui.InputKeyDown &&
		key == ui.KeyV && ev.Mods&ui.Ctrl != 0 && ev.Mods&ui.Shift != 0 {
		return m.pasteTerminalClipboard(session)
	}
	if ev.Kind == ui.InputKeyDown && key == ui.KeyEscape && ev.Mods == 0 &&
		m.terminalEscapeToEditor() {
		m.FocusEditor()
		return true
	}
	if sequence, ok := terminalKeySequence(ev, m.terminalOptionAsMeta()); ok {
		return m.queueTerminalInput(session, sequence)
	}
	return false
}

func (m *Model) pasteTerminalClipboard(session *termSession) bool {
	if session == nil || session.id == 0 {
		return false
	}
	text := terminalClipboardRead()
	if text == "" {
		return false
	}
	return m.queueTerminalInput(session, terminalPasteText(text, session.bracketedPaste))
}

// reportTerminalFocus sends FocusIn/FocusOut when the child enabled xterm
// focus reporting (CSI ? 1004 h). Repeated frames with unchanged focus do
// not produce duplicate reports, and mode changes re-send the current state.
func (m *Model) reportTerminalFocus(session *termSession, focused bool) {
	if session == nil || !session.focusReporting {
		return
	}
	if session.focusKnown && session.focused == focused {
		return
	}
	session.focusKnown, session.focused = true, focused
	sequence := "\x1b[O"
	if focused {
		sequence = "\x1b[I"
	}
	m.queueTerminalInput(session, sequence)
}

func terminalSpecialKeySequence(key ui.Key, mods ui.Modifiers) (string, bool) {
	modifier := terminalModifierParam(mods)
	modifiedCSI := func(final byte) string {
		if modifier <= 1 {
			return string([]byte{'\x1b', '[', final})
		}
		return fmt.Sprintf("\x1b[1;%d%c", modifier, final)
	}
	switch key {
	case ui.KeyEnter:
		return "\r", true
	case ui.KeyTab:
		if mods&ui.Shift != 0 {
			return "\x1b[Z", true
		}
		return "\t", true
	case ui.KeyEscape:
		return "\x1b", true
	case ui.KeyBackspace:
		return "\x7f", true
	case ui.KeyUp:
		return modifiedCSI('A'), true
	case ui.KeyDown:
		return modifiedCSI('B'), true
	case ui.KeyRight:
		return modifiedCSI('C'), true
	case ui.KeyLeft:
		return modifiedCSI('D'), true
	case ui.KeyHome:
		return modifiedCSI('H'), true
	case ui.KeyEnd:
		return modifiedCSI('F'), true
	case ui.KeyInsert:
		return terminalModifiedTilde(modifier, 2), true
	case ui.KeyDelete:
		return terminalModifiedTilde(modifier, 3), true
	case ui.KeyPageUp:
		return terminalModifiedTilde(modifier, 5), true
	case ui.KeyPageDown:
		return terminalModifiedTilde(modifier, 6), true
	case ui.KeyF1:
		return terminalFunctionKey(modifier, 'P', 11), true
	case ui.KeyF2:
		return terminalFunctionKey(modifier, 'Q', 12), true
	case ui.KeyF3:
		return terminalFunctionKey(modifier, 'R', 13), true
	case ui.KeyF4:
		return terminalFunctionKey(modifier, 'S', 14), true
	case ui.KeyF5:
		return terminalModifiedTilde(modifier, 15), true
	case ui.KeyF6:
		return terminalModifiedTilde(modifier, 17), true
	case ui.KeyF7:
		return terminalModifiedTilde(modifier, 18), true
	case ui.KeyF8:
		return terminalModifiedTilde(modifier, 19), true
	case ui.KeyF9:
		return terminalModifiedTilde(modifier, 20), true
	case ui.KeyF10:
		return terminalModifiedTilde(modifier, 21), true
	case ui.KeyF11:
		return terminalModifiedTilde(modifier, 23), true
	case ui.KeyF12:
		return terminalModifiedTilde(modifier, 24), true
	default:
		return "", false
	}
}

func terminalModifierParam(mods ui.Modifiers) int {
	value := 1
	if mods&ui.Shift != 0 {
		value++
	}
	if mods&ui.Alt != 0 {
		value += 2
	}
	if mods&ui.Ctrl != 0 {
		value += 4
	}
	return value
}

func terminalModifiedTilde(modifier, code int) string {
	if modifier <= 1 {
		return fmt.Sprintf("\x1b[%d~", code)
	}
	return fmt.Sprintf("\x1b[%d;%d~", code, modifier)
}

func terminalFunctionKey(modifier int, final byte, code int) string {
	if modifier <= 1 {
		return string([]byte{'\x1b', 'O', final})
	}
	return terminalModifiedTilde(modifier, code)
}

func terminalControlByte(key ui.Key) (byte, bool) {
	if key >= ui.KeyA && key <= ui.KeyZ {
		return byte(key-ui.KeyA) + 1, true
	}
	switch key {
	case ui.KeySpace:
		return 0, true
	case ui.KeyBracketLeft:
		return 0x1b, true
	case ui.KeyBackslash:
		return 0x1c, true
	case ui.KeyBracketRight:
		return 0x1d, true
	case ui.KeyBackquote:
		return 0x1e, true
	case ui.KeyMinus:
		return 0x1f, true
	case ui.KeySlash:
		return 0x1f, true
	default:
		return 0, false
	}
}

func terminalPrintableKey(key ui.Key, shift bool) (string, bool) {
	if key >= ui.KeyA && key <= ui.KeyZ {
		r := rune('a' + key - ui.KeyA)
		if shift {
			r -= 'a' - 'A'
		}
		return string(r), true
	}
	if key >= ui.Key0 && key <= ui.Key9 {
		text := string(rune('0' + key - ui.Key0))
		if shift {
			shifted := ")!@#$%^&*("
			offset := int(key - ui.Key0)
			text = shifted[offset : offset+1]
		}
		return text, true
	}
	pairs := map[ui.Key][2]string{
		ui.KeySpace:        {" ", " "},
		ui.KeyMinus:        {"-", "_"},
		ui.KeyEqual:        {"=", "+"},
		ui.KeyComma:        {",", "<"},
		ui.KeyPeriod:       {".", ">"},
		ui.KeySlash:        {"/", "?"},
		ui.KeySemicolon:    {";", ":"},
		ui.KeyQuote:        {"'", `"`},
		ui.KeyBracketLeft:  {"[", "{"},
		ui.KeyBracketRight: {"]", "}"},
		ui.KeyBackslash:    {`\`, "|"},
		ui.KeyBackquote:    {"`", "~"},
	}
	pair, ok := pairs[key]
	if !ok {
		return "", false
	}
	if shift {
		return pair[1], true
	}
	return pair[0], true
}

func terminalPasteText(text string, bracketed bool) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "\r")
	if bracketed {
		return "\x1b[200~" + text + "\x1b[201~"
	}
	return text
}

// terminalMouseSequence encodes a pointer event for the reporting mode the
// child selected. Callers first check the user setting and consume the event
// only when ok is true, leaving ordinary terminal selection intact otherwise.
func terminalMouseSequence(
	session *termSession,
	ev ui.InputEvent,
	cellWidth, cellHeight float32,
	reporting bool,
) (string, bool) {
	if !reporting || session == nil || session.mouseMode == 0 {
		return "", false
	}
	pixelSGR := session.mousePixels
	if !pixelSGR && (cellWidth <= 0 || cellHeight <= 0 ||
		session.terminalCols <= 0 || session.terminalRows <= 0) {
		return "", false
	}
	switch ev.Kind {
	case ui.InputPointerDown, ui.InputPointerUp, ui.InputPointerMove, ui.InputScroll:
	default:
		return "", false
	}

	col, row := 0, 0
	if pixelSGR {
		col = int(math.Floor(float64(max(ev.X, 0))) + 1)
		row = int(math.Floor(float64(max(ev.Y, 0))) + 1)
	} else {
		col, row = terminalMouseCell(ev.X, ev.Y, cellWidth, cellHeight,
			session.terminalCols, session.terminalRows)
	}
	modifiers := terminalMouseModifiers(ev.Mods)
	sgr := session.mouseSGR || pixelSGR
	if session.mouseMode == 9 {
		// xterm's X10 protocol reports presses only and drops modifiers.
		if ev.Kind != ui.InputPointerDown {
			return "", false
		}
		modifiers = 0
	}
	if ev.Kind == ui.InputScroll {
		button := 64
		if ev.DX > 0 {
			button = 67
		} else if ev.DX < 0 {
			button = 66
		} else if ev.DY < 0 {
			button = 64
		} else if ev.DY > 0 {
			button = 65
		} else {
			return "", false
		}
		sequence := terminalMouseReport(sgr, button+modifiers, col, row, true)
		return sequence, sequence != ""
	}

	moving := ev.Kind == ui.InputPointerMove
	if moving {
		supported := session.mouseMode == 1003 ||
			session.mouseMode == 1002 && ev.Button >= 0
		if !supported {
			return "", false
		}
	}
	button := terminalMouseButton(ev.Button)
	if ev.Kind == ui.InputPointerUp {
		if !sgr {
			// The legacy protocol cannot identify which button was released.
			button = 3
		}
	} else if moving {
		button += 32
	}
	sequence := terminalMouseReport(
		sgr,
		button+modifiers,
		col,
		row,
		ev.Kind != ui.InputPointerUp,
	)
	return sequence, sequence != ""
}

func terminalMouseCell(
	x, y, cellWidth, cellHeight float32,
	cols, rows int,
) (int, int) {
	col := int(math.Floor(float64(max(x, 0)/cellWidth))) + 1
	row := int(math.Floor(float64(max(y, 0)/cellHeight))) + 1
	return min(max(col, 1), cols), min(max(row, 1), rows)
}

func terminalMouseButton(button int) int {
	switch button {
	case 2:
		return 1
	case 1:
		return 2
	case 0:
		return 0
	default:
		return 3
	}
}

func terminalMouseModifiers(mods ui.Modifiers) int {
	value := 0
	if mods&ui.Shift != 0 {
		value += 4
	}
	if mods&ui.Alt != 0 {
		value += 8
	}
	if mods&ui.Ctrl != 0 {
		value += 16
	}
	return value
}

func terminalMouseReport(sgr bool, button, col, row int, press bool) string {
	if sgr {
		final := 'm'
		if press {
			final = 'M'
		}
		return fmt.Sprintf("\x1b[<%d;%d;%d%c", button, col, row, final)
	}
	// xterm's default encoding stores each value in one byte after adding 32.
	// Coordinates or button codes outside that range cannot be represented.
	if button < 0 || button+32 > 255 || col < 0 || col+32 > 255 || row < 0 || row+32 > 255 {
		return ""
	}
	return string([]byte{
		'\x1b', '[', 'M',
		byte(32 + button),
		byte(32 + col),
		byte(32 + row),
	})
}
