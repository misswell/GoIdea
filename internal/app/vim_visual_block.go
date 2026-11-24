package app

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type vimBlockLineRange struct {
	line       int
	start, end int
}

func (m *Model) vimStartVisualBlock(t *Tab, st *vimTabState) {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return
	}
	st.mode = vimVisualBlock
	st.visualAnchor = t.state.Cursor()
	st.visualHead = st.visualAnchor
	m.vimUpdateVisualBlock(t, st)
}

func (m *Model) vimVisualBlockCommand(t *Tab, st *vimTabState, token string) bool {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return true
	}
	switch token {
	case "h", "<Left>":
		m.vimVisualBlockMove(t, st, "h", 1)
	case "l", "<Right>":
		m.vimVisualBlockMove(t, st, "l", 1)
	case "j", "<Down>":
		m.vimVisualBlockMove(t, st, "j", 1)
	case "k", "<Up>":
		m.vimVisualBlockMove(t, st, "k", 1)
	case "w", "W", "b", "B", "e", "E", "$", "^", "0", "G":
		m.vimVisualBlockMove(t, st, token, 1)
	case "v":
		if st.mode == vimVisualBlock {
			t.state.Select(st.visualHead, st.visualHead)
			st.mode = vimNormal
		} else {
			st.mode = vimVisualBlock
			m.vimUpdateVisualBlock(t, st)
		}
	case "V":
		st.mode = vimVisualLine
		m.vimUpdateVisual(t, st)
	case "<Ctrl+V>":
		if st.mode == vimVisualBlock {
			t.state.Select(st.visualHead, st.visualHead)
			st.mode = vimNormal
		}
	case "o":
		st.visualAnchor, st.visualHead = st.visualHead, st.visualAnchor
		m.vimUpdateVisualBlock(t, st)
	case "I":
		m.vimStartBlockInsert(t, st, false)
	case "A":
		m.vimStartBlockInsert(t, st, true)
	case "d", "x":
		tabWidth := vimTabWidth(t)
		_, _, firstCol, lastCol := vimBlockColumnBounds(
			t.state.Buffer, st.visualAnchor, st.visualHead, tabWidth)
		ranges := vimVisualBlockRanges(t.state.Buffer, st.visualAnchor, st.visualHead, tabWidth)
		m.vimYankBlock(t, st, t.state.Buffer, ranges, lastCol-firstCol, vimRegisterDelete)
		target := m.vimDeleteBlockRanges(t, ranges)
		t.state.Select(target, target)
		st.mode = vimNormal
	case "y":
		tabWidth := vimTabWidth(t)
		_, _, firstCol, lastCol := vimBlockColumnBounds(
			t.state.Buffer, st.visualAnchor, st.visualHead, tabWidth)
		ranges := vimVisualBlockRanges(t.state.Buffer, st.visualAnchor, st.visualHead, tabWidth)
		m.vimYankBlock(t, st, t.state.Buffer, ranges, lastCol-firstCol, vimRegisterYank)
		pos := min(st.visualAnchor, st.visualHead)
		t.state.Select(pos, pos)
		st.mode = vimNormal
	case "c":
		tabWidth := vimTabWidth(t)
		_, _, firstCol, lastCol := vimBlockColumnBounds(
			t.state.Buffer, st.visualAnchor, st.visualHead, tabWidth)
		ranges := vimVisualBlockRanges(t.state.Buffer, st.visualAnchor, st.visualHead, tabWidth)
		m.vimYankBlock(t, st, t.state.Buffer, ranges, lastCol-firstCol, vimRegisterDelete)
		m.vimDeleteBlockRanges(t, ranges)
		firstLine, lastLine, firstCol, _ := vimBlockColumnBounds(
			t.state.Buffer, st.visualAnchor, st.visualHead, tabWidth)
		m.vimSetBlockInsertCarets(t, firstLine, lastLine, firstCol, tabWidth)
		st.mode = vimInsert
		st.blockInsert = true
	default:
		if len(token) == 1 {
			return true
		}
		return false
	}
	return true
}

func (m *Model) vimVisualBlockMove(t *Tab, st *vimTabState, token string, count int) {
	b := t.state.Buffer
	tabWidth := vimTabWidth(t)
	line := b.Pos(st.visualHead).Line
	text := b.Line(line)
	headInLine := b.Pos(st.visualHead).Col
	switch token {
	case "h":
		offset := vimDisplayStep(text, headInLine, -count, tabWidth)
		st.visualHead = b.Offset(editor.Pos{Line: line, Col: offset})
		m.vimUpdateVisualBlock(t, st)
		return
	case "l":
		offset := vimDisplayStep(text, headInLine, count, tabWidth)
		st.visualHead = b.Offset(editor.Pos{Line: line, Col: offset})
		m.vimUpdateVisualBlock(t, st)
		return
	case "j", "k":
		targetLine := line + count
		if token == "k" {
			targetLine = line - count
		}
		targetLine = max(0, min(b.LineCount()-1, targetLine))
		column, _ := vimDisplayCellSpan(text, headInLine, tabWidth)
		offset := vimDisplayByteOffset(b.Line(targetLine), column, tabWidth)
		st.visualHead = b.Offset(editor.Pos{Line: targetLine, Col: offset})
		m.vimUpdateVisualBlock(t, st)
		return
	}
	if target, ok := vimMotionOffsetAt(t, token, count, st.visualHead); ok {
		st.visualHead = target
		m.vimUpdateVisualBlock(t, st)
	}
}

func (m *Model) vimUpdateVisualBlock(t *Tab, st *vimTabState) {
	b := t.state.Buffer
	ranges := vimVisualBlockRanges(b, st.visualAnchor, st.visualHead, vimTabWidth(t))
	headLine := b.Pos(st.visualHead).Line
	sels := make([]editor.Selection, 0, len(ranges))
	var head editor.Selection
	for _, block := range ranges {
		sel := editor.Selection{Anchor: block.start, Head: block.end}
		if block.line == headLine {
			head = sel
			continue
		}
		sels = append(sels, sel)
	}
	if len(ranges) == 0 {
		sels = append(sels, editor.Selection{Anchor: st.visualHead, Head: st.visualHead})
	} else {
		sels = append(sels, head)
	}
	t.state.SetSelections(sels...)
}

func vimVisualBlockRanges(b *editor.Buffer, anchor, head, tabWidth int) []vimBlockLineRange {
	if b == nil {
		return nil
	}
	firstLine, lastLine, firstCol, lastCol := vimBlockColumnBounds(b, anchor, head, tabWidth)
	ranges := make([]vimBlockLineRange, 0, lastLine-firstLine+1)
	for line := firstLine; line <= lastLine; line++ {
		text := b.Line(line)
		lineStart := b.Offset(editor.Pos{Line: line})
		start, end := vimBlockLineSpan(text, firstCol, lastCol, tabWidth)
		ranges = append(ranges, vimBlockLineRange{line: line, start: lineStart + start, end: lineStart + end})
	}
	return ranges
}

func (m *Model) vimStartBlockInsert(t *Tab, st *vimTabState, appendAfter bool) {
	b := t.state.Buffer
	tabWidth := vimTabWidth(t)
	ranges := vimVisualBlockRanges(b, st.visualAnchor, st.visualHead, tabWidth)
	if len(ranges) == 0 {
		return
	}
	firstLine, lastLine, firstCol, lastCol := vimBlockColumnBounds(
		b, st.visualAnchor, st.visualHead, tabWidth)
	col := firstCol
	if appendAfter {
		col = lastCol
	}
	m.vimSetBlockInsertCarets(t, firstLine, lastLine, col, tabWidth)
	st.mode = vimInsert
	st.blockInsert = true
}

func vimBlockColumnBounds(b *editor.Buffer, anchor, head, tabWidth int) (int, int, int, int) {
	if b == nil || b.LineCount() == 0 {
		return 0, 0, 0, 0
	}
	anchor = max(0, min(anchor, b.Len()))
	head = max(0, min(head, b.Len()))
	a, z := b.Pos(anchor), b.Pos(head)
	firstLine, lastLine := min(a.Line, z.Line), max(a.Line, z.Line)
	aStart, aEnd := vimDisplayCellSpan(b.Line(a.Line), a.Col, tabWidth)
	zStart, zEnd := vimDisplayCellSpan(b.Line(z.Line), z.Col, tabWidth)
	return firstLine, lastLine, min(aStart, zStart), max(aEnd, zEnd)
}

func vimBlockLineSpan(text string, left, right, tabWidth int) (int, int) {
	if right <= left || text == "" {
		return len(text), len(text)
	}
	start, end := -1, -1
	column := 0
	for offset := 0; offset < len(text); {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if size <= 0 {
			break
		}
		runeWidth := vimRuneDisplayWidth(r, column, tabWidth)
		runeEnd := column + runeWidth
		selected := runeWidth > 0 && column < right && runeEnd > left
		if runeWidth == 0 && column >= left && column < right {
			selected = true
		}
		if selected {
			if start < 0 {
				start = offset
			}
			end = offset + size
		}
		column = runeEnd
		offset += size
	}
	if start < 0 {
		return len(text), len(text)
	}
	return start, end
}

func (m *Model) vimSetBlockInsertCarets(t *Tab, firstLine, lastLine, col, tabWidth int) {
	if t == nil || t.state == nil || t.state.Buffer == nil || lastLine < firstLine {
		return
	}
	b := t.state.Buffer
	sels := make([]editor.Selection, 0, lastLine-firstLine+1)
	for line := firstLine; line <= lastLine; line++ {
		text := b.Line(line)
		offset := b.Offset(editor.Pos{Line: line, Col: vimDisplayByteOffset(text, col, tabWidth)})
		sels = append(sels, editor.Selection{Anchor: offset, Head: offset})
	}
	if len(sels) == 0 {
		pos := t.state.Cursor()
		sels = append(sels, editor.Selection{Anchor: pos, Head: pos})
	}
	t.state.SetSelections(sels...)
}

func (m *Model) vimYankBlock(t *Tab, st *vimTabState, b *editor.Buffer, ranges []vimBlockLineRange, width int, write vimRegisterWrite) {
	parts := make([]string, 0, len(ranges))
	for _, block := range ranges {
		if block.end > block.start {
			parts = append(parts, b.Slice(block.start, block.end))
		}
	}
	if len(parts) == 0 {
		return
	}
	text := strings.Join(parts, "\n")
	m.vimStoreRegister(t, st, vimRegister{
		text:       text,
		block:      true,
		blockWidth: max(1, width),
		set:        true,
	}, write)
}

func (m *Model) vimDeleteBlockRanges(t *Tab, ranges []vimBlockLineRange) int {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return 0
	}
	b := t.state.Buffer
	edits := make([]editor.Edit, 0, len(ranges))
	for _, block := range ranges {
		if block.end > block.start {
			edits = append(edits, editor.Edit{Start: block.start, End: block.end})
		}
	}
	target := t.state.Cursor()
	if len(ranges) > 0 {
		target = ranges[0].start
	}
	if len(edits) > 0 {
		b.Edit(edits...)
		target = min(target, b.Len())
	}
	return target
}
