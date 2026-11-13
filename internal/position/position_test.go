package position

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func naiveUTF16(s string) int { return utf16Len(s) }

func naiveUTF16At(s string, byteOff int) int {
	byteOff = clamp(byteOff, 0, len(s))
	return naiveUTF16(s[:byteOff])
}

func naiveByteAt(s string, units int) int {
	if units <= 0 {
		return 0
	}
	n, i := 0, 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		u := 1
		if r > 0xFFFF {
			u = 2
		}
		if n+u > units {
			return i
		}
		n += u
		i += size
	}
	return len(s)
}

func TestKnownOffsets(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		ticks []struct{ byteOff, unit int } // rune boundaries
	}{
		{"empty", "", []struct{ byteOff, unit int }{{0, 0}}},
		{"ascii", "abc", []struct{ byteOff, unit int }{{0, 0}, {1, 1}, {2, 2}, {3, 3}}},
		{"cjk", "中文", []struct{ byteOff, unit int }{{0, 0}, {3, 1}, {6, 2}}},
		{"emoji", "a😀b", []struct{ byteOff, unit int }{{0, 0}, {1, 1}, {5, 3}, {6, 4}}},
		{"combining", "e\u0301x", []struct{ byteOff, unit int }{{0, 0}, {1, 1}, {3, 2}, {4, 3}}},
		{"tab", "\t\n", []struct{ byteOff, unit int }{{0, 0}, {1, 1}, {2, 2}}},
		{"crlf", "a\r\nb", []struct{ byteOff, unit int }{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}}},
		{"lf-no-final-newline", "a\nb", []struct{ byteOff, unit int }{{0, 0}, {1, 1}, {2, 2}, {3, 3}}},
		{"trailing-newline", "a\n", []struct{ byteOff, unit int }{{0, 0}, {1, 1}, {2, 2}}},
		{"astral-lines", "𝄞\n😀", []struct{ byteOff, unit int }{{0, 0}, {4, 2}, {5, 3}, {9, 5}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMapper(tc.text)
			text := StringText(tc.text)
			for _, tick := range tc.ticks {
				if got := m.ByteOffsetToUTF16Offset(text, tick.byteOff); got != tick.unit {
					t.Errorf("ByteOffsetToUTF16Offset(%d) = %d, want %d", tick.byteOff, got, tick.unit)
				}
				if got := m.UTF16OffsetToByteOffset(text, tick.unit); got != tick.byteOff {
					t.Errorf("UTF16OffsetToByteOffset(%d) = %d, want %d", tick.unit, got, tick.byteOff)
				}
			}
			if !m.idx.verify(text) {
				t.Fatalf("index does not match text")
			}
		})
	}
}

func TestPositionsAndRanges(t *testing.T) {
	text := "中文😀\nsecond\r\nthird"
	m := NewMapper(text)
	s := StringText(text)

	line, col := m.UTF16PositionToBytePosition(s, Position{Line: 0, Character: 3})
	if line != 0 || col != 6 {
		t.Fatalf("position (0,3) = line %d col %d, want 0,6", line, col)
	}
	if got := m.BytePositionToUTF16Position(s, 0, 6); got != (Position{Line: 0, Character: 2}) {
		t.Fatalf("byte position (0,6) = %+v", got)
	}
	// Column 4 is inside the surrogate pair of the emoji: snaps to its start.
	line, col = m.UTF16PositionToBytePosition(s, Position{Line: 0, Character: 3})
	if line != 0 || col != 6 {
		t.Fatalf("mid-surrogate position = line %d col %d, want 0,6", line, col)
	}
	// CRLF line: content ends before \r\n.
	if got := m.BytePositionToUTF16Position(s, 1, 6); got != (Position{Line: 1, Character: 6}) {
		t.Fatalf("crlf line end = %+v", got)
	}
	if got := m.BytePositionToUTF16Position(s, 1, 99); got != (Position{Line: 1, Character: 6}) {
		t.Fatalf("column clamp = %+v", got)
	}

	r := Range{Start: Position{Line: 0, Character: 2}, End: Position{Line: 2, Character: 2}}
	start, end := m.RangeToByteOffsets(s, r)
	if want := (Range{Start: Position{Line: 0, Character: 2}, End: Position{Line: 2, Character: 2}}); m.ByteRangeToUTF16Range(s, start, end) != want {
		t.Fatalf("range round trip = %+v", m.ByteRangeToUTF16Range(s, start, end))
	}
	if got := s.Slice(start, end); got != "😀\nsecond\r\nth" {
		t.Fatalf("range bytes = %q", got)
	}
}

func TestChangeSetConversion(t *testing.T) {
	base := "a😀中\nx"
	m := NewMapper(base)
	s := StringText(base)
	cs := ChangeSet{Changes: []TextChange{
		{From: 1, To: 3, Insert: "Z"},
		{From: 4, To: 5, Insert: "字"},
	}}
	edits := m.ChangeSetToByteEdits(s, cs)
	if len(edits) != 2 {
		t.Fatalf("got %d edits", len(edits))
	}
	if edits[0].Start != 1 || edits[0].End != 5 || edits[0].Text != "Z" {
		t.Fatalf("edit 0 = %+v", edits[0])
	}
	if edits[1].Start != 8 || edits[1].End != 9 || edits[1].Text != "字" {
		t.Fatalf("edit 1 = %+v", edits[1])
	}
}

func TestTextEditsConversion(t *testing.T) {
	base := "one\n中文two\n"
	m := NewMapper(base)
	s := StringText(base)
	edits := m.TextEditsToByteEdits(s, []TextEdit{
		{Range: Range{Start: Position{Line: 1, Character: 0}, End: Position{Line: 1, Character: 2}}, NewText: "EN"},
	})
	if len(edits) != 1 || edits[0].Start != 4 || edits[0].End != 10 || edits[0].Text != "EN" {
		t.Fatalf("edits = %+v", edits)
	}
}

func TestIncrementalEdits(t *testing.T) {
	alphabet := []string{"a", "b", "中", "😀", "𝄞", "e\u0301", "\t", "\n", "\r\n", "β", "1", " "}
	rng := rand.New(rand.NewPCG(1, 2))
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString(alphabet[rng.IntN(len(alphabet))])
	}
	text := b.String()
	m := NewMapper(text)
	if !m.idx.verify(StringText(text)) {
		t.Fatal("initial index mismatch")
	}
	for step := 0; step < 500; step++ {
		old := text
		// Choose rune-aligned offsets, as editors produce.
		offsets := []int{0}
		for i := 0; i < len(old); {
			_, size := utf8.DecodeRuneInString(old[i:])
			i += size
			offsets = append(offsets, i)
		}
		a := offsets[rng.IntN(len(offsets))]
		c := offsets[rng.IntN(len(offsets))]
		if c < a {
			a, c = c, a
		}
		var ins strings.Builder
		for i := 0; i < rng.IntN(4); i++ {
			ins.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		insert := ins.String()
		text = old[:a] + insert + old[c:]
		m.ApplyEdit(StringText(old), StringText(text), a, c, a+len(insert))

		if !m.idx.verify(StringText(text)) {
			t.Fatalf("step %d: index mismatch after replacing [%d,%d) with %q", step, a, c, insert)
		}
		for off := 0; off <= len(text); off++ {
			if got, want := m.ByteOffsetToUTF16Offset(StringText(text), off), naiveUTF16At(text, off); got != want {
				t.Fatalf("step %d: byte %d -> %d, want %d (text %q)", step, off, got, want, text)
			}
		}
		for u := 0; u <= naiveUTF16(text); u++ {
			if got, want := m.UTF16OffsetToByteOffset(StringText(text), u), naiveByteAt(text, u); got != want {
				t.Fatalf("step %d: unit %d -> %d, want %d (text %q)", step, u, got, want, text)
			}
		}
	}
}

func TestConcurrentConversions(t *testing.T) {
	text := strings.Repeat("line 中文 😀\n", 1000)
	m := NewMapper(text)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), 9))
			for j := 0; j < 2000; j++ {
				off := rng.IntN(len(text))
				u := m.ByteOffsetToUTF16Offset(StringText(text), off)
				if u < 0 {
					t.Errorf("negative offset")
				}
				m.BytePositionToUTF16Position(StringText(text), off%1000, off%20)
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkByteToUTF16OneMillionLines(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 1_000_000; i++ {
		fmt.Fprintf(&sb, "line %d with 中文 😀\n", i)
	}
	text := sb.String()
	idx := newLineIndex(text)
	b.Logf("lines=%d bytes=%d build ok=%v", idx.lines, idx.totalBytes, idx.verify(StringText(text)))
	b.ResetTimer()
	m := &Mapper{idx: idx}
	s := StringText(text)
	for i := 0; i < b.N; i++ {
		off := (i * 7919) % len(text)
		m.ByteOffsetToUTF16Offset(s, off)
	}
}

func BenchmarkEditMiddleOfMillionLines(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 1_000_000; i++ {
		fmt.Fprintf(&sb, "line %d with 中文 😀\n", i)
	}
	text := sb.String()
	idx := newLineIndex(text)
	off := len(text) / 2
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := text
		next := old[:off] + "x" + old[off:]
		idx.applyEdit(StringText(old), StringText(next), off, off, off+1)
		text = next
		off++
	}
}
