package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalKeySequencesReachThePTY(t *testing.T) {
	tests := []struct {
		event ui.InputEvent
		want  string
	}{
		{ui.InputEvent{Kind: ui.InputText, Text: "日本"}, "日本"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}, "\r"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyTab}, "\t"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyTab, Mods: ui.Shift}, "\x1b[Z"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEscape}, "\x1b"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyBackspace}, "\x7f"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp}, "\x1b[A"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyLeft, Mods: ui.Ctrl}, "\x1b[1;5D"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyHome, Mods: ui.Alt}, "\x1b[1;3H"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyDelete}, "\x1b[3~"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF1}, "\x1bOP"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF5}, "\x1b[15~"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyC, Mods: ui.Ctrl}, "\x03"},
		{ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeySpace, Mods: ui.Ctrl}, "\x00"},
	}
	for _, test := range tests {
		got, ok := terminalKeySequence(test.event, false)
		if !ok || got != test.want {
			t.Errorf("%+v = %q, %v; want %q", test.event, got, ok, test.want)
		}
	}
}

func TestTerminalOptionAsMetaPrefixesTypedText(t *testing.T) {
	event := ui.InputEvent{Kind: ui.InputText, Text: "x", Mods: ui.Alt}
	if got, ok := terminalKeySequence(event, false); !ok || got != "x" {
		t.Fatalf("Option off = %q, %v", got, ok)
	}
	if got, ok := terminalKeySequence(event, true); !ok || got != "\x1bx" {
		t.Fatalf("Option as Meta = %q, %v", got, ok)
	}
}

func TestTerminalCommandShortcutsStayInTheIDE(t *testing.T) {
	event := ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyC, Mods: ui.Super}
	if sequence, ok := terminalKeySequence(event, false); ok || sequence != "" {
		t.Fatalf("Command+C was sent to the PTY: %q, %v", sequence, ok)
	}
}

func TestTerminalBracketedPaste(t *testing.T) {
	if got := terminalPasteText("one\ntwo\r\nthree", true); got !=
		"\x1b[200~one\rtwo\rthree\x1b[201~" {
		t.Fatalf("bracketed paste = %q", got)
	}
	if got := terminalPasteText("one\ntwo", false); got != "one\rtwo" {
		t.Fatalf("plain paste = %q", got)
	}
}

func TestTerminalSGRMouseReporting(t *testing.T) {
	session := &termSession{terminalCols: 80, terminalRows: 24, mouseMode: 1000, mouseSGR: true}
	down := ui.InputEvent{
		Kind: ui.InputPointerDown, Button: 0, X: 21, Y: 25, Mods: ui.Ctrl,
	}
	got, ok := terminalMouseSequence(session, down, 10, 20, true)
	if !ok || got != "\x1b[<16;3;2M" {
		t.Fatalf("mouse down = %q, %v", got, ok)
	}
	up := down
	up.Kind = ui.InputPointerUp
	got, ok = terminalMouseSequence(session, up, 10, 20, true)
	if !ok || got != "\x1b[<16;3;2m" {
		t.Fatalf("mouse up = %q, %v", got, ok)
	}
	if sequence, ok := terminalMouseSequence(session, down, 10, 20, false); ok {
		t.Fatalf("mouse reporting ran while the setting was off: %q", sequence)
	}
}

func TestTerminalSGRPixelMouseReportingUsesPointerPixels(t *testing.T) {
	session := &termSession{
		terminalCols: 80, terminalRows: 24,
		mouseMode: 1003, mousePixels: true,
	}
	event := ui.InputEvent{
		Kind: ui.InputPointerDown, Button: 0, X: 21, Y: 25, Mods: ui.Ctrl,
	}
	got, ok := terminalMouseSequence(session, event, 10, 20, true)
	if !ok || got != "\x1b[<16;22;26M" {
		t.Fatalf("pixel mouse down = %q, %v", got, ok)
	}
	event.Kind, event.Mods = ui.InputPointerUp, 0
	got, ok = terminalMouseSequence(session, event, 10, 20, true)
	if !ok || got != "\x1b[<0;22;26m" {
		t.Fatalf("pixel mouse up = %q, %v", got, ok)
	}
}

func TestTerminalMouseProtocolsMatchXtermTrackingRules(t *testing.T) {
	base := termSession{terminalCols: 300, terminalRows: 24, mouseSGR: true}
	down := ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Mods: ui.Ctrl}
	up := ui.InputEvent{Kind: ui.InputPointerUp, Button: 0}
	wheel := ui.InputEvent{Kind: ui.InputScroll, DY: -1}
	moveWithoutButton := ui.InputEvent{Kind: ui.InputPointerMove, Button: -1}
	moveWithButton := ui.InputEvent{Kind: ui.InputPointerMove, Button: 0}

	base.mouseMode = 9
	if got, ok := terminalMouseSequence(&base, down, 1, 1, true); !ok || got != "\x1b[<0;1;1M" {
		t.Fatalf("X10 press = %q, %v; want modifiers omitted", got, ok)
	}
	for name, event := range map[string]ui.InputEvent{"release": up, "wheel": wheel, "motion": moveWithButton} {
		if got, ok := terminalMouseSequence(&base, event, 1, 1, true); ok {
			t.Fatalf("X10 %s was reported: %q", name, got)
		}
	}

	base.mouseMode = 1000
	if got, ok := terminalMouseSequence(&base, moveWithButton, 1, 1, true); ok {
		t.Fatalf("VT200 reported motion: %q", got)
	}
	if got, ok := terminalMouseSequence(&base, up, 1, 1, true); !ok || got != "\x1b[<0;1;1m" {
		t.Fatalf("VT200 SGR release = %q, %v", got, ok)
	}

	base.mouseMode = 1002
	if got, ok := terminalMouseSequence(&base, moveWithoutButton, 1, 1, true); ok {
		t.Fatalf("drag protocol reported motion without a held button: %q", got)
	}
	if got, ok := terminalMouseSequence(&base, moveWithButton, 1, 1, true); !ok || got != "\x1b[<32;1;1M" {
		t.Fatalf("drag protocol motion = %q, %v", got, ok)
	}

	base.mouseMode = 1003
	if got, ok := terminalMouseSequence(&base, moveWithoutButton, 1, 1, true); !ok || got != "\x1b[<35;1;1M" {
		t.Fatalf("any-motion protocol = %q, %v", got, ok)
	}
}

func TestTerminalLegacyMouseReportsDropUnencodableCoordinates(t *testing.T) {
	session := &termSession{terminalCols: 300, terminalRows: 24, mouseMode: 1000}
	event := ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: 224, Y: 0}
	if got, ok := terminalMouseSequence(session, event, 1, 1, true); ok {
		t.Fatalf("unencodable legacy coordinate was clamped and reported: %q", got)
	}
}

func TestTerminalLegacyMouseReleaseUsesNoButtonCode(t *testing.T) {
	session := &termSession{terminalCols: 80, terminalRows: 24, mouseMode: 1000}
	event := ui.InputEvent{Kind: ui.InputPointerUp, Button: 0}
	got, ok := terminalMouseSequence(session, event, 1, 1, true)
	want := string([]byte{'\x1b', '[', 'M', 35, 33, 33})
	if !ok || got != want {
		t.Fatalf("legacy release = %q, %v; want no-button report %q", got, ok, want)
	}
}

func TestTerminalMouseMotionAndWheelModes(t *testing.T) {
	session := &termSession{terminalCols: 80, terminalRows: 24, mouseMode: 1003, mouseSGR: true}
	move := ui.InputEvent{Kind: ui.InputPointerMove, Button: -1, X: 9, Y: 9}
	if got, ok := terminalMouseSequence(session, move, 10, 10, true); !ok ||
		got != "\x1b[<35;1;1M" {
		t.Fatalf("all-motion report = %q, %v", got, ok)
	}
	wheel := ui.InputEvent{Kind: ui.InputScroll, DY: -1, X: 9, Y: 9}
	if got, ok := terminalMouseSequence(session, wheel, 10, 10, true); !ok ||
		got != "\x1b[<64;1;1M" {
		t.Fatalf("wheel report = %q, %v", got, ok)
	}
}
