package app

import (
	"strings"
	"testing"
)

func TestTerminalSampleTUIFrameRepaintsAlternateScreen(t *testing.T) {
	var session termSession
	session.terminalCols, session.terminalRows = 80, 24
	session.write(sampleTUIStart + sampleTUIFrame(0, 80, 24))
	if !session.alternate {
		t.Fatal("sample TUI did not enter the alternate screen")
	}
	if len(session.lines) != 24 {
		t.Fatalf("alternate screen rows = %d", len(session.lines))
	}
	if title := session.terminalTitle; title != "⠋ Thinking - sample" {
		t.Fatalf("terminal title = %q", title)
	}
	first := append([]string(nil), session.lines...)
	session.write(sampleTUIFrame(1, 80, 24))
	var changed []int
	for row := range first {
		if first[row] != session.lines[row] {
			changed = append(changed, row)
		}
	}
	if len(changed) != 1 || changed[0] != 17 {
		t.Fatalf("changed rows = %v", changed)
	}
	if !strings.Contains(session.lines[17], "⠙ Thinking") {
		t.Fatalf("spinner row = %q", session.lines[17])
	}
}

func TestTerminalAlternateScreenRestoresPrimaryOutput(t *testing.T) {
	var session termSession
	session.write("shell output\r\n")
	primary := session.text()
	session.write(sampleTUIStart + sampleTUIFrame(0, 80, 24) + sampleTUIEnd)
	if session.alternate {
		t.Fatal("alternate screen remained active")
	}
	if got := session.text(); got != primary {
		t.Fatalf("restored scrollback = %q, want %q", got, primary)
	}
}

func TestTerminalEscapeSequenceMayArriveAcrossChunks(t *testing.T) {
	var session termSession
	session.terminalCols, session.terminalRows = 12, 4
	session.write("\x1b[?104")
	session.write("9h\x1b[2;3Habc")
	if !session.alternate {
		t.Fatal("split alternate-screen sequence was not applied")
	}
	if got := strings.TrimRight(session.lines[1], " "); got != "  abc" {
		t.Fatalf("cursor-positioned row = %q", got)
	}
}

func TestTerminalGridUsesCellColumnsForWideText(t *testing.T) {
	var session termSession
	session.terminalCols, session.terminalRows = 8, 3
	session.write("\x1b[?1049h你A")
	if !session.alternate {
		t.Fatal("alternate screen was not entered")
	}
	if session.screenCol != 3 {
		t.Fatalf("cursor column = %d, want 3 after 你 and A", session.screenCol)
	}
	text := strings.TrimRight(session.lines[0], " ")
	if text != "你A" {
		t.Fatalf("wide row = %q", text)
	}
	if got := terminalTextCellWidth(text); got != 3 {
		t.Fatalf("display columns = %d, want 3", got)
	}

	session.write("\x1b[1;2Hx")
	text = strings.TrimRight(session.lines[0], " ")
	if text != " xA" {
		t.Fatalf("row after overwriting a wide cell = %q, want %q", text, " xA")
	}
	if strings.Contains(text, "你") {
		t.Fatalf("overwritten wide cell survived: %q", text)
	}
}

func TestTerminalResizePreservesWideCellsAndClampsTheCursor(t *testing.T) {
	var session termSession
	session.terminalCols, session.terminalRows = 10, 4
	session.write("\x1b[?1049h你界\x1b[3;8H")
	session.applyTerminalSize(6, 2)
	if session.terminalCols != 6 || session.terminalRows != 2 {
		t.Fatalf("size = %dx%d", session.terminalCols, session.terminalRows)
	}
	if len(session.screenCells) != 2 || len(session.screenCells[0]) != 6 {
		t.Fatalf("resized grid = %d rows x %d columns",
			len(session.screenCells), len(session.screenCells[0]))
	}
	if got := strings.TrimRight(session.lines[0], " "); got != "你界" {
		t.Fatalf("wide row after resize = %q", got)
	}
	if session.screenRow >= session.terminalRows || session.screenCol >= session.terminalCols {
		t.Fatalf("cursor = %d,%d outside %dx%d",
			session.screenRow, session.screenCol, session.terminalCols, session.terminalRows)
	}
}

func TestTerminalInsertDeleteAndEraseKeepWideCellsWhole(t *testing.T) {
	var session termSession
	session.terminalCols, session.terminalRows = 8, 3
	session.write("\x1b[?1049hA你B")

	// Insert from the trailing half of 你: the whole glyph moves right.
	session.setAlternateCursor(0, 2)
	session.write("\x1b[1@")
	assertTerminalWideCells(t, &session)
	if got := strings.TrimRight(session.lines[0], " "); got != "A 你B" {
		t.Fatalf("row after insert = %q, want %q", got, "A 你B")
	}

	// Delete from the trailing half: the whole glyph leaves with the edit.
	session.setAlternateCursor(0, 3)
	session.write("\x1b[1P")
	assertTerminalWideCells(t, &session)
	if got := strings.TrimRight(session.lines[0], " "); got != "A B" {
		t.Fatalf("row after delete = %q, want %q", got, "A B")
	}

	// Erasing one cell inside a wide glyph erases both of its columns.
	session.write("\x1b[1;1H\x1b[2K你B")
	session.setAlternateCursor(0, 1)
	session.write("\x1b[1X")
	assertTerminalWideCells(t, &session)
	if strings.Contains(session.lines[0], "你") {
		t.Fatalf("erased wide glyph survived: %q", session.lines[0])
	}
}

func TestTerminalScrollingPreservesWholeRowsWithWideCells(t *testing.T) {
	var session termSession
	session.terminalCols, session.terminalRows = 8, 3
	session.write("\x1b[?1049h你界\r\nＡＢ\r\nAB")
	session.scrollAlternate(1)
	assertTerminalWideCells(t, &session)
	if got := strings.TrimRight(session.lines[0], " "); got != "ＡＢ" {
		t.Fatalf("first row after scroll = %q, want %q", got, "ＡＢ")
	}
	if got := strings.TrimRight(session.lines[1], " "); got != "AB" {
		t.Fatalf("second row after scroll = %q, want %q", got, "AB")
	}
	if got := strings.TrimRight(session.lines[2], " "); got != "" {
		t.Fatalf("blank row after scroll = %q", got)
	}
}

func assertTerminalWideCells(t *testing.T, session *termSession) {
	t.Helper()
	for row, cells := range session.screenCells {
		if len(cells) != session.terminalCols {
			t.Fatalf("row %d has %d cells, want %d",
				row, len(cells), session.terminalCols)
		}
		for col := 0; col < len(cells); col++ {
			cell := cells[col]
			if cell.Continuation {
				if col == 0 || cells[col-1].Width != 2 {
					t.Fatalf("orphan continuation cell at row %d column %d", row, col)
				}
				continue
			}
			if cell.Width == 2 &&
				(col+1 >= len(cells) || !cells[col+1].Continuation) {
				t.Fatalf("wide cell without continuation at row %d column %d", row, col)
			}
		}
	}
}

func TestTerminalOSC8HyperlinksSurviveChunks(t *testing.T) {
	var session termSession
	session.terminalCols, session.terminalRows = 20, 3
	session.write("\x1b[?1049h\x1b]8;id=docs;https://example.com/a")
	session.write("\x1b\\link")
	session.write("\x1b]8;;\x1b\\ plain")
	if len(session.hyperlinks) == 0 {
		t.Fatal("OSC 8 link did not produce a span")
	}
	link := session.hyperlinks[0]
	if link.URI != "https://example.com/a" || link.Row != 0 ||
		link.Start != 0 || link.End != 4 {
		t.Fatalf("hyperlink span = %+v", link)
	}
	if got := strings.TrimRight(session.lines[0], " "); got != "link plain" {
		t.Fatalf("hyperlink row = %q", got)
	}
}

func TestTerminalPrivateModesTrackCursorMousePasteAndFocus(t *testing.T) {
	var session termSession
	session.write("\x1b[?25l\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[?1016h\x1b[?2004h\x1b[?1004h")
	if !session.cursorHidden || session.mouseMode != 1002 ||
		!session.mouseSGR || !session.mousePixels || !session.bracketedPaste || !session.focusReporting {
		t.Fatalf("modes = cursorHidden=%v mouse=%d sgr=%v pixels=%v paste=%v focus=%v",
			session.cursorHidden, session.mouseMode, session.mouseSGR,
			session.mousePixels, session.bracketedPaste, session.focusReporting)
	}
	// xterm selects one tracking protocol at a time, and resetting any
	// tracking mode disables the active protocol.
	session.write("\x1b[?25h\x1b[?1000l\x1b[?1006l\x1b[?1016l\x1b[?2004l\x1b[?1004l")
	if session.cursorHidden || session.mouseMode != 0 ||
		session.mouseSGR || session.mousePixels || session.bracketedPaste || session.focusReporting {
		t.Fatalf("modes after reset = %+v", session)
	}
}

func TestTerminalGlobalModesSurviveAlternateScreenSwitches(t *testing.T) {
	var session termSession
	session.write("\x1b[?25l\x1b[?1002h\x1b[?1006h\x1b[?1016h\x1b[?2004h\x1b[?1004h\x1b[?1049h")
	if !session.alternate || !session.cursorHidden || session.mouseMode != 1002 ||
		!session.mouseSGR || !session.mousePixels || !session.bracketedPaste || !session.focusReporting {
		t.Fatalf("global modes were lost entering alternate screen: %+v", session)
	}
	session.write("\x1b[?1049l")
	if session.alternate || !session.cursorHidden || session.mouseMode != 1002 ||
		!session.mouseSGR || !session.mousePixels || !session.bracketedPaste || !session.focusReporting {
		t.Fatalf("global modes were lost leaving alternate screen: %+v", session)
	}
}

func TestTerminalSampleTUIActionParsesFixtureArguments(t *testing.T) {
	action, ok := actionGet("terminal.sampleTui")
	if !ok || action.RunWithArg == nil {
		t.Fatal("terminal.sampleTui is not registered with its argument path")
	}
	if got := terminalSampleInt(float64(12), 40); got != 12 {
		t.Fatalf("frames = %d", got)
	}
	if got := terminalSampleInt("30", 10); got != 30 {
		t.Fatalf("fps = %d", got)
	}
}
