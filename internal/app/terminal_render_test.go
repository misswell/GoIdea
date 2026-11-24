package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func terminalViewTester(m *Model) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		m.termPaneBody(c, c.Theme(), 0)
	}, 620, 420)
}

func TestTerminalViewRoutesTypedKeysAndOptionMetaToThePTY(t *testing.T) {
	m := terminalSettingsModel(map[string]any{
		"terminal.optionAsMeta":   true,
		"terminal.escapeToEditor": false,
	})
	m.term.visible = true
	session := m.term.add(0)
	session.id, session.channel = 7, 11

	tester := terminalViewTester(m)
	tester.Type("go")
	tester.TypeKey(ui.Alt, ui.KeyX, "x")
	tester.Key(0, ui.KeyEnter)

	if got := string(session.writeQueue); got != "go\x1bx\r" {
		t.Fatalf("PTY input = %q, want %q", got, "go\x1bx\r")
	}
}

func TestTerminalViewIgnoresKeysWithoutALiveShell(t *testing.T) {
	m := terminalSettingsModel(map[string]any{})
	m.term.visible = true
	session := m.term.add(0)
	session.channel = 11

	tester := terminalViewTester(m)
	tester.Type("ignored")
	tester.Key(0, ui.KeyEnter)

	if len(session.writeQueue) != 0 {
		t.Fatalf("keys queued without a live PTY: %q", session.writeQueue)
	}
}

func TestTerminalEscapeToEditorLeavesKeysForFullScreenProgramsWhenOff(t *testing.T) {
	escape := ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEscape}
	m := terminalSettingsModel(map[string]any{"terminal.escapeToEditor": true})
	m.tabs = []*Tab{{}}
	session := &termSession{id: 7, channel: 11}
	if !m.handleTerminalInput(0, session, escape, 8, 16) {
		t.Fatal("Escape was not consumed")
	}
	if !m.focusApply || len(session.writeQueue) != 0 {
		t.Fatalf("Escape to editor = focusApply=%v queue=%q", m.focusApply, session.writeQueue)
	}

	m = terminalSettingsModel(map[string]any{"terminal.escapeToEditor": false})
	session = &termSession{id: 7, channel: 11}
	if !m.handleTerminalInput(0, session, escape, 8, 16) {
		t.Fatal("Escape was not sent to the PTY")
	}
	if got := string(session.writeQueue); got != "\x1b" {
		t.Fatalf("PTY Escape = %q", got)
	}
}

func TestTerminalResizeUsesMeasuredCells(t *testing.T) {
	metrics := terminalMetrics{
		font:      ui.Font{Family: "monospace", Size: 13},
		cellWidth: 8, cellHeight: 16,
	}
	cols, rows := terminalGridSize(620, 420, metrics)
	if cols != 75 || rows != 25 {
		t.Fatalf("grid = %dx%d, want 75x25", cols, rows)
	}

	m := terminalSettingsModel(map[string]any{})
	m.term.visible = true
	session := m.term.add(0)
	session.id, session.channel = 7, 11
	if !m.reconcileTerminalViewport(session, 620, 420, metrics) {
		t.Fatal("viewport size did not update")
	}
	if session.desiredCols != 75 || session.desiredRows != 25 ||
		session.resizeGen != 1 {
		t.Fatalf("resize state = %dx%d generation=%d",
			session.desiredCols, session.desiredRows, session.resizeGen)
	}
}

func TestTerminalHyperlinkAndCursorStyles(t *testing.T) {
	links := []terminalHyperlink{{Row: 0, Start: 0, End: 4, URI: "https://example.com"}}
	spans := terminalLineSpans(
		"link text", links, 0, true,
		ui.RGB(220, 220, 220), ui.RGB(80, 140, 255),
		ui.RGB(80, 140, 255), ui.RGB(10, 10, 10), 0,
		ui.Font{Family: "monospace", Size: 13}, 5, "block", true,
	)
	if len(spans) < 2 || !spans[0].Underline || spans[0].Color != ui.RGB(80, 140, 255) {
		t.Fatalf("hyperlink spans = %+v", spans)
	}
	var cursor ui.Span
	for _, span := range spans {
		if span.Background == ui.RGB(80, 140, 255) {
			cursor = span
			break
		}
	}
	if cursor.Text == "" || cursor.Color != ui.RGB(10, 10, 10) {
		t.Fatalf("block cursor span = %+v", cursor)
	}
	spans = terminalLineSpans(
		"link text", nil, 0, true,
		ui.RGB(220, 220, 220), ui.RGB(80, 140, 255),
		ui.RGB(80, 140, 255), ui.RGB(10, 10, 10), 0,
		ui.Font{Family: "monospace", Size: 13}, 5, "underline", true,
	)
	var underline ui.Span
	for _, span := range spans {
		if span.Text == "t" && span.Underline {
			underline = span
			break
		}
	}
	if underline.Text == "" || underline.DecorationColor != ui.RGB(80, 140, 255) {
		t.Fatalf("underline cursor spans = %+v", spans)
	}
	spans = terminalLineSpans(
		"link text", links, 0, false,
		ui.RGB(220, 220, 220), ui.RGB(80, 140, 255),
		ui.RGB(80, 140, 255), ui.RGB(10, 10, 10), 0,
		ui.Font{Family: "monospace", Size: 13}, -1, "block", false,
	)
	if spans[0].Underline || spans[0].Color != ui.RGB(220, 220, 220) {
		t.Fatalf("hyperlink styling ignored the disabled setting: %+v", spans)
	}
}

func TestTerminalCopyOnSelectionDeduplicatesAndTrimsRows(t *testing.T) {
	previousRead, previousWrite := terminalClipboardRead, terminalClipboardWrite
	t.Cleanup(func() {
		terminalClipboardRead, terminalClipboardWrite = previousRead, previousWrite
	})
	var writes []string
	terminalClipboardWrite = func(text string) { writes = append(writes, text) }

	m := terminalSettingsModel(map[string]any{"terminal.copyOnSelection": true})
	session := &termSession{}
	parts := []ui.SelectedText{
		{Text: "one   ", Start: 0, End: 6},
		{Text: "two", Start: 0, End: 3},
	}
	m.updateTerminalCopyOnSelection(session, parts)
	m.updateTerminalCopyOnSelection(session, parts)
	if len(writes) != 1 || writes[0] != "one\ntwo" {
		t.Fatalf("copy-on-selection writes = %#v", writes)
	}
	m.updateTerminalCopyOnSelection(session, nil)
	m.updateTerminalCopyOnSelection(session, parts)
	if len(writes) != 2 || writes[1] != "one\ntwo" {
		t.Fatalf("copy-on-selection did not copy a new selection: %#v", writes)
	}
}

func TestTerminalFocusReportingSendsOnlyFocusTransitions(t *testing.T) {
	m := terminalSettingsModel(map[string]any{})
	session := &termSession{id: 7, channel: 11, focusReporting: true}

	m.reportTerminalFocus(session, true)
	m.reportTerminalFocus(session, true)
	if got := string(session.writeQueue); got != "\x1b[I" {
		t.Fatalf("focus-in queue = %q", got)
	}

	session.writeQueue = nil
	m.reportTerminalFocus(session, false)
	m.reportTerminalFocus(session, false)
	if got := string(session.writeQueue); got != "\x1b[O" {
		t.Fatalf("focus-out queue = %q", got)
	}

	// Enabling the mode again re-sends the current focus state.
	session.writeQueue = nil
	session.focusKnown = false
	m.reportTerminalFocus(session, false)
	if got := string(session.writeQueue); got != "\x1b[O" {
		t.Fatalf("re-enabled focus queue = %q", got)
	}
}
