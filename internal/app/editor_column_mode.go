package app

import (
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type columnSelectionBlock struct {
	valid                    bool
	version                  uint64
	anchorLine, anchorColumn int
	headLine, headColumn     int
}

// ToggleColumnSelection matches Edit › Column Selection Mode. The mode keeps
// Shift+arrows rectangular; existing selections are retained until the first
// block extension.
func (m *Model) ToggleColumnSelection() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	t.columnMode = !t.columnMode
	t.columnBlock = columnSelectionBlock{}
	if t.columnMode {
		m.status = "Column selection mode on"
	} else {
		m.status = "Column selection mode off"
	}
	m.invalidate()
}

func (m *Model) columnSelectionEnabled() bool {
	t := m.tab()
	return t != nil && t.columnMode
}

// extendColumnSelection is the high-priority Shift+arrow handler while
// Column Selection Mode is on.
func (m *Model) extendColumnSelection(t *Tab, dLine, dColumn int) bool {
	if t == nil || t.state == nil || !t.columnMode || t.state.Buffer == nil {
		return false
	}
	m.clearInlineCompletionState(t)
	buffer := t.state.Buffer
	snapshot := buffer.Snapshot()
	if snapshot.LineCount() == 0 {
		return false
	}
	if !t.columnBlock.valid || t.columnBlock.version != buffer.Version() {
		selection := t.state.Selection()
		anchorLine, anchorColumn := columnPoint(buffer, selection.Anchor)
		headLine, headColumn := columnPoint(buffer, selection.Head)
		t.columnBlock = columnSelectionBlock{
			valid:        true,
			version:      buffer.Version(),
			anchorLine:   anchorLine,
			anchorColumn: anchorColumn,
			headLine:     headLine,
			headColumn:   headColumn,
		}
	}
	block := &t.columnBlock
	if dLine != 0 {
		block.headLine = max(0, min(block.headLine+dLine, snapshot.LineCount()-1))
	}
	if dColumn != 0 {
		block.headColumn = stepVisibleColumn(snapshot.Line(block.headLine), block.headColumn, dColumn)
	}
	step := 1
	if block.headLine < block.anchorLine {
		step = -1
	}
	var selections []editor.Selection
	for line := block.anchorLine; ; line += step {
		text := snapshot.Line(line)
		start := visibleColumnOffset(text, block.anchorColumn)
		end := visibleColumnOffset(text, block.headColumn)
		base := snapshot.LineStart(line)
		selections = append(selections, editor.Selection{
			Anchor: base + start,
			Head:   base + end,
		})
		if line == block.headLine {
			break
		}
	}
	if len(selections) == 0 {
		return false
	}
	t.state.SetSelections(selections...)
	m.invalidate()
	return true
}

// finishColumnPointerSelection turns a finished ordinary mouse drag into the
// same rectangular selection Column Selection Mode uses for Shift+arrows.
// MyGo has already resolved the pointer to document offsets by pointer-up.
func (m *Model) finishColumnPointerSelection(t *Tab) {
	if t == nil || t.state == nil || !t.columnMode || t.state.Buffer == nil || t.caret != nil {
		return
	}
	selection := t.state.Selection()
	if selection.Empty() {
		return
	}
	buffer := t.state.Buffer
	snapshot := buffer.Snapshot()
	anchorLine, anchorColumn := columnPoint(buffer, selection.Anchor)
	headLine, headColumn := columnPoint(buffer, selection.Head)
	if anchorLine == headLine && anchorColumn == headColumn {
		return
	}
	t.columnBlock = columnSelectionBlock{
		valid:        true,
		version:      buffer.Version(),
		anchorLine:   anchorLine,
		anchorColumn: anchorColumn,
		headLine:     headLine,
		headColumn:   headColumn,
	}
	step := 1
	if headLine < anchorLine {
		step = -1
	}
	selections := make([]editor.Selection, 0, abs(headLine-anchorLine)+1)
	for line := anchorLine; ; line += step {
		text := snapshot.Line(line)
		start := snapshot.LineStart(line) + visibleColumnOffset(text, anchorColumn)
		end := snapshot.LineStart(line) + visibleColumnOffset(text, headColumn)
		selections = append(selections, editor.Selection{Anchor: start, Head: end})
		if line == headLine {
			break
		}
	}
	t.state.SetSelections(selections...)
	m.invalidate()
}

// columnInput runs before MyGo's ordinary Shift+arrow selection.
func (m *Model) columnInput(t *Tab, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || t == nil || !t.columnMode || ev.Mods != ui.Shift {
		return false
	}
	switch ev.Key {
	case ui.KeyUp:
		return m.extendColumnSelection(t, -1, 0)
	case ui.KeyDown:
		return m.extendColumnSelection(t, 1, 0)
	case ui.KeyLeft:
		return m.extendColumnSelection(t, 0, -1)
	case ui.KeyRight:
		return m.extendColumnSelection(t, 0, 1)
	default:
		return false
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func columnPoint(buffer *editor.Buffer, offset int) (line, column int) {
	pos := buffer.Snapshot().Pos(offset)
	return pos.Line, visibleColumn(buffer.Snapshot().Line(pos.Line), pos.Col)
}

func stepVisibleColumn(text string, column, delta int) int {
	if delta == 0 {
		return column
	}
	width := visibleColumn(text, len(text))
	if delta > 0 && column >= width || delta < 0 && column > width {
		return max(0, column+delta)
	}
	if delta > 0 {
		return visibleColumn(text, visibleColumnOffsetAfter(text, column+1))
	}
	position := visibleColumnOffsetBefore(text, column)
	if visibleColumn(text, position) < column {
		return visibleColumn(text, position)
	}
	if position == 0 {
		return 0
	}
	return visibleColumn(text, visibleColumnOffsetBefore(text, column-1))
}

func visibleColumn(text string, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	byteOffset = min(byteOffset, len(text))
	column := 0
	for offset := 0; offset < byteOffset; {
		width, next := visibleColumnCluster(text, offset, column)
		column += width
		offset = next
	}
	return column
}

func visibleColumnOffset(text string, target int) int {
	return visibleColumnOffsetWithBias(text, target, columnNearest)
}

func visibleColumnOffsetBefore(text string, target int) int {
	return visibleColumnOffsetWithBias(text, target, columnBefore)
}

func visibleColumnOffsetAfter(text string, target int) int {
	return visibleColumnOffsetWithBias(text, target, columnAfter)
}

type columnBias uint8

const (
	columnNearest columnBias = iota
	columnBefore
	columnAfter
)

func visibleColumnOffsetWithBias(text string, target int, bias columnBias) int {
	if target <= 0 {
		return 0
	}
	column := 0
	for offset := 0; offset < len(text); {
		width, next := visibleColumnCluster(text, offset, column)
		if width > 0 && column+width > target {
			switch bias {
			case columnBefore:
				return offset
			case columnAfter:
				return next
			}
			if target-column < column+width-target {
				return offset
			}
			return next
		}
		column += width
		offset = next
		if column >= target {
			return offset
		}
	}
	return len(text)
}

func visibleColumnCluster(text string, offset, column int) (width, next int) {
	r, size := utf8.DecodeRuneInString(text[offset:])
	if size == 0 {
		return 0, offset
	}
	if r == '\t' {
		return 4 - column%4, offset + size
	}
	next = offset + size
	width = runeVisibleWidth(r, column)
	if isRegionalIndicator(r) {
		if following, followingSize := utf8.DecodeRuneInString(text[next:]); followingSize > 0 && isRegionalIndicator(following) {
			width = 2
			next += followingSize
		}
	}
	pictographic := isPictographicRune(r)
	for next < len(text) {
		following, followingSize := utf8.DecodeRuneInString(text[next:])
		switch {
		case isColumnExtend(following):
			if following == '\ufe0f' && pictographic {
				width = 2
			}
			next += followingSize
		case following == '\u200d':
			next += followingSize
			if next >= len(text) {
				return width, next
			}
			joined, joinedSize := utf8.DecodeRuneInString(text[next:])
			width = max(width, runeVisibleWidth(joined, 0))
			pictographic = pictographic || isPictographicRune(joined)
			next += joinedSize
		default:
			return width, next
		}
	}
	return width, next
}

func isColumnExtend(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		unicode.Is(unicode.Cf, r) || r >= 0x1f3fb && r <= 0x1f3ff ||
		r >= 0xfe00 && r <= 0xfe0f || r >= 0xe0100 && r <= 0xe01ef
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1f1e6 && r <= 0x1f1ff
}

func isPictographicRune(r rune) bool {
	return r == 0x00a9 || r == 0x00ae || r == 0x203c || r == 0x2049 ||
		r == 0x2122 || r == 0x2139 ||
		r >= 0x2194 && r <= 0x21ff ||
		r >= 0x2300 && r <= 0x27ff ||
		r >= 0x2934 && r <= 0x2935 ||
		r >= 0x2b05 && r <= 0x2b55 ||
		r == 0x3030 || r == 0x303d || r == 0x3297 || r == 0x3299 ||
		r >= 0x1f000 && r <= 0x1faff
}

func runeVisibleWidth(r rune, column int) int {
	switch {
	case r == '\t':
		return 4 - column%4
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r):
		return 0
	case isWideRune(r):
		return 2
	default:
		return 1
	}
}

func isWideRune(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		r >= 0x2e80 && r <= 0xa4cf && r != 0x303f ||
		r >= 0xac00 && r <= 0xd7a3 ||
		r >= 0xf900 && r <= 0xfaff ||
		r >= 0xfe10 && r <= 0xfe19 ||
		r >= 0xfe30 && r <= 0xfe6f ||
		r >= 0xff00 && r <= 0xff60 ||
		r >= 0xffe0 && r <= 0xffe6 ||
		r >= 0x1f000 && r <= 0x1faff ||
		r >= 0x20000 && r <= 0x3fffd)
}
