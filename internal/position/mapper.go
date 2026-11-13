package position

import "sync"

// Mapper converts between UTF-8 byte offsets and UTF-16 code-unit offsets for
// one evolving text. Edits update the line index incrementally; conversions do
// not scan the file.
//
// The zero value is not usable: build one with NewMapper. Methods are safe from
// any goroutine.
type Mapper struct {
	mu  sync.RWMutex
	idx *lineIndex
}

// NewMapper builds a mapper for text.
func NewMapper(text string) *Mapper {
	return &Mapper{idx: newLineIndex(text)}
}

// ApplyEdit updates the index for an edit replacing bytes start through oldEnd
// of oldText with the bytes start through newEnd of newText. The old and new
// texts must be the versions on either side of that single edit.
func (m *Mapper) ApplyEdit(oldText, newText Text, start, oldEnd, newEnd int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	start = clamp(start, 0, oldText.Len())
	oldEnd = clamp(oldEnd, start, oldText.Len())
	newEnd = clamp(newEnd, start, newText.Len())
	m.idx.applyEdit(oldText, newText, start, oldEnd, newEnd)
}

// LineCount returns the number of lines the index holds; an empty text has one.
func (m *Mapper) LineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.idx.lines
}

// ByteOffsetToUTF16Offset returns the UTF-16 offset of a UTF-8 byte offset.
func (m *Mapper) ByteOffsetToUTF16Offset(t Text, off int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byteToUTF16(t, off)
}

func (m *Mapper) byteToUTF16(t Text, off int) int {
	off = clamp(off, 0, t.Len())
	line, start := m.idx.lineAtByte(int64(off))
	lineStartUnits := m.idx.lineStartUnit(line)
	return int(lineStartUnits) + utf16Len(t.Slice(int(start), off))
}

// UTF16OffsetToByteOffset returns the UTF-8 byte offset of a UTF-16 offset. A
// position inside a surrogate pair snaps to the start of the rune.
func (m *Mapper) UTF16OffsetToByteOffset(t Text, u int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.utf16ToByte(t, u)
}

func (m *Mapper) utf16ToByte(t Text, u int) int {
	u = clamp(u, 0, int(m.idx.totalUnits))
	line, _ := m.idx.lineAtUnit(int64(u))
	lineStartBytes := int(m.idx.lineStartByte(line))
	lineStartUnits := int(m.idx.lineStartUnit(line))
	bytes, _ := m.idx.sizes(line)
	lineEnd := clamp(lineStartBytes+int(bytes), 0, t.Len())
	return lineStartBytes + byteAtUTF16Col(t.Slice(lineStartBytes, lineEnd), u-lineStartUnits)
}

// BytePositionToUTF16Position converts a line and byte column to a protocol
// position. The column is clamped to the line's content, before its line break.
func (m *Mapper) BytePositionToUTF16Position(t Text, line, colBytes int) Position {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bytePosToUTF16(t, line, colBytes)
}

func (m *Mapper) bytePosToUTF16(t Text, line, colBytes int) Position {
	line = clamp(line, 0, m.idx.lines-1)
	start, contentEnd := m.lineContent(t, line)
	colBytes = clamp(colBytes, 0, contentEnd-start)
	return Position{Line: uint32(line), Character: uint32(utf16Len(t.Slice(start, start+colBytes)))}
}

// UTF16PositionToBytePosition converts a protocol position to a line and byte
// column, clamping the line and its column to the text.
func (m *Mapper) UTF16PositionToBytePosition(t Text, p Position) (line, colBytes int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.utf16PosToByte(t, p)
}

func (m *Mapper) utf16PosToByte(t Text, p Position) (int, int) {
	line := clamp(int(p.Line), 0, m.idx.lines-1)
	start, contentEnd := m.lineContent(t, line)
	contentUnits := utf16Len(t.Slice(start, contentEnd))
	col := clamp(int(p.Character), 0, contentUnits)
	return line, byteAtUTF16Col(t.Slice(start, contentEnd), col)
}

// RangeToByteOffsets converts a UTF-16 range to byte offsets.
func (m *Mapper) RangeToByteOffsets(t Text, r Range) (start, end int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	startLine, startCol := m.utf16PosToByte(t, r.Start)
	endLine, endCol := m.utf16PosToByte(t, r.End)
	start = int(m.idx.lineStartByte(startLine)) + startCol
	end = int(m.idx.lineStartByte(endLine)) + endCol
	if end < start {
		start, end = end, start
	}
	return start, end
}

// ByteRangeToUTF16Range converts byte offsets to a UTF-16 range.
func (m *Mapper) ByteRangeToUTF16Range(t Text, start, end int) Range {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byteRangeToUTF16(t, start, end)
}

func (m *Mapper) byteRangeToUTF16(t Text, start, end int) Range {
	if end < start {
		start, end = end, start
	}
	startLine, _ := m.idx.lineAtByte(int64(clamp(start, 0, t.Len())))
	endLine, _ := m.idx.lineAtByte(int64(clamp(end, 0, t.Len())))
	return Range{
		Start: m.bytePosToUTF16(t, startLine, start-int(m.idx.lineStartByte(startLine))),
		End:   m.bytePosToUTF16(t, endLine, end-int(m.idx.lineStartByte(endLine))),
	}
}

// ChangeSetToByteEdits converts a protocol change set, whose offsets are UTF-16
// into t before the changes, to byte ranges of t.
func (m *Mapper) ChangeSetToByteEdits(t Text, cs ChangeSet) []ByteChange {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ByteChange, 0, len(cs.Changes))
	for _, c := range cs.Changes {
		from := m.utf16ToByte(t, int(c.From))
		to := m.utf16ToByte(t, int(c.To))
		if to < from {
			from, to = to, from
		}
		out = append(out, ByteChange{Start: from, End: to, Text: c.Insert})
	}
	return out
}

// TextEditsToByteEdits converts LSP text edits to byte ranges of t.
func (m *Mapper) TextEditsToByteEdits(t Text, edits []TextEdit) []ByteChange {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ByteChange, 0, len(edits))
	for _, e := range edits {
		start, end := 0, 0
		_, startCol := m.utf16PosToByte(t, e.Range.Start)
		startLine := clamp(int(e.Range.Start.Line), 0, m.idx.lines-1)
		start = int(m.idx.lineStartByte(startLine)) + startCol
		endLine, endCol := m.utf16PosToByte(t, e.Range.End)
		end = int(m.idx.lineStartByte(endLine)) + endCol
		if end < start {
			start, end = end, start
		}
		out = append(out, ByteChange{Start: start, End: end, Text: e.NewText})
	}
	return out
}

// lineContent returns the byte range of a line without its line break.
func (m *Mapper) lineContent(t Text, line int) (start, end int) {
	line = clamp(line, 0, m.idx.lines-1)
	bytes, _ := m.idx.sizes(line)
	start = int(m.idx.lineStartByte(line))
	end = clamp(start+int(bytes), 0, t.Len())
	if end > start && t.Slice(end-1, end) == "\n" {
		end--
		if end > start && t.Slice(end-1, end) == "\r" {
			end--
		}
	}
	return start, end
}

// ByteChange is a replacement of bytes Start through End with Text, in the
// coordinates of the text it was converted against.
type ByteChange struct {
	Start, End int
	Text       string
}
