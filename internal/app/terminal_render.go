package app

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const (
	terminalPadX            = float32(10)
	terminalPadY            = float32(8)
	terminalCursorBlink     = 1060 * time.Millisecond
	terminalCursorLineWidth = float32(1.5)
)

type terminalMetrics struct {
	font       ui.Font
	cellWidth  float32
	cellHeight float32
	spacing    float32
}

func (m *Model) terminalMetrics() terminalMetrics {
	font := m.terminalTextStyle()
	spacing := m.terminalColumnSpacing()
	cellWidth := font.Size * 0.62
	if glyphs := ui.Shape("M", font); len(glyphs) > 0 {
		cellWidth = glyphs[0].Advance
	}
	cellWidth = max(1, cellWidth+spacing)
	lineHeight := font.LineHeight
	if lineHeight <= 0 {
		lineHeight = 1
	}
	cellHeight := max(1, font.Size*lineHeight)
	font.LineHeight = 0
	return terminalMetrics{
		font: font, cellWidth: cellWidth, cellHeight: cellHeight, spacing: spacing,
	}
}

func terminalGridSize(width, height float32, metrics terminalMetrics) (int, int) {
	if width <= 0 || height <= 0 || metrics.cellWidth <= 0 || metrics.cellHeight <= 0 {
		return 0, 0
	}
	cols := int(math.Floor(float64(max(width-2*terminalPadX, 0) / metrics.cellWidth)))
	rows := int(math.Floor(float64(max(height-2*terminalPadY, 0) / metrics.cellHeight)))
	return max(cols, 2), max(rows, 1)
}

func (m *Model) reconcileTerminalViewport(
	session *termSession,
	width, height float32,
	metrics terminalMetrics,
) bool {
	if session == nil {
		return false
	}
	cols, rows := terminalGridSize(width, height, metrics)
	if cols == 0 || rows == 0 {
		return false
	}
	if session.desiredCols == cols && session.desiredRows == rows {
		return false
	}
	session.desiredCols, session.desiredRows = cols, rows
	session.resizeGen++
	if session.id == 0 || m.mgr == nil {
		session.applyTerminalSize(cols, rows)
		session.ptyCols, session.ptyRows = cols, rows
		m.term.cols, m.term.rows = cols, rows
		return true
	}
	m.requestTerminalResize(session)
	return true
}

func terminalLineSpans(
	text string,
	links []terminalHyperlink,
	row int,
	highlightLinks bool,
	textColor, linkColor, cursorColor, cursorTextColor ui.Color,
	spacing float32,
	font ui.Font,
	cursorCol int,
	cursorShape string,
	cursorVisible bool,
) []ui.Span {
	if cursorVisible && cursorShape == "bar" {
		offset := terminalDisplayByteOffset(text, cursorCol)
		if offset < len(text) {
			_, size := utf8.DecodeRuneInString(text[offset:])
			if size > 0 {
				text = text[:offset] + "▏" + text[offset+size:]
			}
		}
	}
	boundaries := map[int]struct{}{0: {}, len(text): {}}
	for _, link := range links {
		if link.Row != row {
			continue
		}
		boundaries[terminalDisplayByteOffset(text, link.Start)] = struct{}{}
		boundaries[terminalDisplayByteOffset(text, link.End)] = struct{}{}
	}
	cursorStart, cursorEnd := cursorCol, cursorCol+1
	if cursorVisible {
		cursorStart = terminalDisplayByteOffset(text, cursorCol)
		cursorEnd = cursorStart
		if cursorStart < len(text) {
			_, size := utf8.DecodeRuneInString(text[cursorStart:])
			cursorEnd += size
		}
		if cursorEnd == cursorStart && cursorEnd < len(text) {
			cursorEnd++
		}
		boundaries[cursorStart] = struct{}{}
		boundaries[cursorEnd] = struct{}{}
	}
	offsets := make([]int, 0, len(boundaries))
	for offset := range boundaries {
		offsets = append(offsets, offset)
	}
	for i := 1; i < len(offsets); i++ {
		for j := i; j > 0 && offsets[j] < offsets[j-1]; j-- {
			offsets[j], offsets[j-1] = offsets[j-1], offsets[j]
		}
	}

	spans := make([]ui.Span, 0, max(len(offsets)-1, 1))
	if len(offsets) < 2 {
		offsets = []int{0, len(text)}
	}
	for i := 0; i+1 < len(offsets); i++ {
		start, end := offsets[i], offsets[i+1]
		if end <= start {
			continue
		}
		column := terminalDisplayColumnAt(text, start)
		span := ui.Span{
			Text: text[start:end], Font: font.Family, Size: font.Size,
			Weight: font.Weight, Italic: font.Italic, Color: textColor,
			LetterSpacing: spacing, Features: font.Features, Lang: font.Lang,
		}
		if highlightLinks {
			for _, link := range links {
				if link.Row == row && column >= link.Start && column < link.End {
					span.Color = linkColor
					span.Underline = true
					span.DecorationColor = linkColor
					break
				}
			}
		}
		cursorHere := cursorVisible && start >= cursorStart && start < cursorEnd
		if cursorHere {
			switch cursorShape {
			case "underline":
				span.Underline = true
				span.DecorationColor = cursorColor
				span.DecorationThickness = terminalCursorLineWidth
			default:
				span.Background = cursorColor
				span.Color = cursorTextColor
			}
		}
		spans = append(spans, span)
	}
	if len(spans) == 0 && cursorVisible && cursorCol == 0 {
		spans = append(spans, ui.Span{
			Text: " ", Font: font.Family, Size: font.Size,
			Background: cursorColor, Color: cursorTextColor,
			LetterSpacing: spacing,
		})
	}
	return spans
}

func terminalCopyText(parts []ui.SelectedText) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		start := max(0, min(part.Start, len([]rune(part.Text))))
		end := max(start, min(part.End, len([]rune(part.Text))))
		runes := []rune(part.Text)
		lines = append(lines, strings.TrimRight(string(runes[start:end]), " "))
	}
	return strings.Join(lines, "\n")
}

func terminalCopyData(parts []ui.SelectedText) transfer.Data {
	return transfer.TextData(terminalCopyText(parts))
}

func (m *Model) updateTerminalCopyOnSelection(
	session *termSession,
	parts []ui.SelectedText,
) {
	if session == nil {
		return
	}
	if !m.terminalCopyOnSelection() {
		session.copiedSelection = ""
		return
	}
	text := terminalCopyText(parts)
	if text == "" {
		session.copiedSelection = ""
		return
	}
	if text == session.copiedSelection {
		return
	}
	session.copiedSelection = text
	terminalClipboardWrite(text)
}
