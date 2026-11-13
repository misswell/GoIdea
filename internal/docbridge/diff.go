package docbridge

import (
	"unicode/utf8"
)

// Hunk is one changed region of a diff: bytes [OldStart, OldEnd) of the old
// text became bytes [NewStart, NewEnd) of the new one.
type Hunk struct {
	OldStart, OldEnd int
	NewStart, NewEnd int
}

func (h Hunk) oldLen() int { return h.OldEnd - h.OldStart }
func (h Hunk) newLen() int { return h.NewEnd - h.NewStart }

// maxDiffEdit bounds Myers' edit distance; a larger change (say a file
// reformatted end to end) falls back to one hunk over the changed span.
const maxDiffEdit = 2048

// diffTexts returns the changed regions between two texts, ordered and
// non-overlapping, in the coordinates of each. It strips the common prefix and
// suffix, then diffs lines with Myers inside the remainder and trims each hunk
// back to the bytes that differ.
func diffTexts(oldText, newText textReader) []Hunk {
	n, m := oldText.Len(), newText.Len()
	if n == 0 && m == 0 {
		return nil
	}
	p := commonPrefix(oldText, newText)
	// The prefix is found by bytes; back it up to a rune boundary so a shared
	// lead byte of two different runes never splits them.
	for p > 0 && !runeStartAt(oldText, p) {
		p--
	}
	s := commonSuffix(oldText, newText, p)
	// Likewise, the suffix must begin at a rune boundary.
	for s > 0 && !runeStartAt(oldText, n-s) {
		s--
	}
	oldMid := oldText.Slice(p, n-s)
	newMid := newText.Slice(p, m-s)
	if oldMid == "" && newMid == "" {
		return nil
	}
	if oldMid == newMid {
		return nil
	}

	a, aOff := splitLines(oldMid)
	b, bOff := splitLines(newMid)
	lineHunks, ok := myersHunks(a, b, maxDiffEdit)
	if !ok {
		h := Hunk{OldStart: 0, OldEnd: len(oldMid), NewStart: 0, NewEnd: len(newMid)}
		if h, ok = trimHunk(oldMid, newMid, h); ok {
			return []Hunk{shiftHunk(h, p)}
		}
		return nil
	}
	out := make([]Hunk, 0, len(lineHunks))
	for _, lh := range lineHunks {
		h := Hunk{
			OldStart: aOff[lh.aStart],
			OldEnd:   aOff[lh.aEnd],
			NewStart: bOff[lh.bStart],
			NewEnd:   bOff[lh.bEnd],
		}
		var keep bool
		if h, keep = trimHunk(oldMid, newMid, h); keep {
			out = append(out, shiftHunk(h, p))
		}
	}
	return out
}

func shiftHunk(h Hunk, by int) Hunk {
	h.OldStart += by
	h.OldEnd += by
	h.NewStart += by
	h.NewEnd += by
	return h
}

// textReader is the read side of a text for diffing; *position.Mapper's Text
// interface satisfies it.
type textReader interface {
	Len() int
	Slice(start, end int) string
}

const diffChunk = 1 << 16

// runeStartAt reports whether off is a rune boundary of t.
func runeStartAt(t textReader, off int) bool {
	if off <= 0 || off >= t.Len() {
		return true
	}
	return utf8.RuneStart(t.Slice(off, off+1)[0])
}

func commonPrefix(a, b textReader) int {
	n := min(a.Len(), b.Len())
	i := 0
	for i < n {
		size := min(diffChunk, n-i)
		sa := a.Slice(i, i+size)
		sb := b.Slice(i, i+size)
		if sa != sb {
			for j := 0; j < size; j++ {
				if sa[j] != sb[j] {
					return i + j
				}
			}
		}
		i += size
	}
	return n
}

// commonSuffix returns the bytes equal at the ends of a and b, at most
// min(len)-prefix of them.
func commonSuffix(a, b textReader, prefix int) int {
	na, nb := a.Len(), b.Len()
	n := min(na, nb) - prefix
	i := 0
	for i < n {
		size := min(diffChunk, n-i)
		sa := a.Slice(na-i-size, na-i)
		sb := b.Slice(nb-i-size, nb-i)
		if sa != sb {
			for j := size - 1; j >= 0; j-- {
				if sa[j] != sb[j] {
					return i + (size - 1 - j)
				}
			}
		}
		i += size
	}
	return n
}

// splitLines splits s into lines that keep their newline, with the start offset
// of each line and the end offset of the last.
func splitLines(s string) (lines []string, offs []int) {
	offs = append(offs, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
			offs = append(offs, start)
		}
	}
	lines = append(lines, s[start:])
	offs = append(offs, len(s))
	return lines, offs
}

type lineHunk struct{ aStart, aEnd, bStart, bEnd int }

// myersHunks diffs a and b by line with Myers' O(ND) algorithm, returning the
// changed line ranges; ok is false when the edit distance exceeds maxD.
func myersHunks(a, b []string, maxD int) ([]lineHunk, bool) {
	n, m := len(a), len(b)
	switch {
	case n == 0 && m == 0:
		return nil, true
	case n == 0:
		return []lineHunk{{0, 0, 0, m}}, true
	case m == 0:
		return []lineHunk{{0, n, 0, 0}}, true
	}
	ids := make(map[string]int, n+m)
	intern := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, s := range lines {
			id, ok := ids[s]
			if !ok {
				id = len(ids)
				ids[s] = id
			}
			out[i] = id
		}
		return out
	}
	ai, bi := intern(a), intern(b)

	maxEdits := n + m
	off := maxEdits + 1
	v := make([]int, 2*maxEdits+3)
	trace := make([][]int, 0, 32)
	endD := -1
	for d := 0; d <= maxEdits && endD < 0; d++ {
		if d > maxD {
			return nil, false
		}
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && ai[x] == bi[y] {
				x, y = x+1, y+1
			}
			v[off+k] = x
			if x >= n && y >= m {
				endD = d
				break
			}
		}
	}
	if endD < 0 {
		return nil, false
	}

	type step struct {
		del, ins bool
		ai, bi   int
	}
	steps := make([]step, 0, endD)
	x, y := n, m
	for d := endD; d > 0; d-- {
		vd := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && vd[off+k-1] < vd[off+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := vd[off+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x, y = x-1, y-1
		}
		if x == prevX {
			y--
			steps = append(steps, step{ins: true, ai: x, bi: y})
		} else {
			x--
			steps = append(steps, step{del: true, ai: x, bi: y})
		}
	}

	hunks := make([]lineHunk, 0, len(steps))
	for i := len(steps) - 1; i >= 0; i-- {
		st := steps[i]
		aS, aE := st.ai, st.ai
		if st.del {
			aE++
		}
		bS, bE := st.bi, st.bi
		if st.ins {
			bE++
		}
		if k := len(hunks); k > 0 && hunks[k-1].aEnd == aS && hunks[k-1].bEnd == bS {
			hunks[k-1].aEnd, hunks[k-1].bEnd = aE, bE
			continue
		}
		hunks = append(hunks, lineHunk{aS, aE, bS, bE})
	}
	return hunks, true
}

// trimHunk removes the equal runes at the ends of a hunk, keeping rune
// boundaries so offsets stay valid for UTF-16 conversion.
func trimHunk(oldMid, newMid string, h Hunk) (Hunk, bool) {
	os, ns, oe, ne := h.OldStart, h.NewStart, h.OldEnd, h.NewEnd
	for os < oe && ns < ne {
		ro, so := utf8.DecodeRuneInString(oldMid[os:oe])
		rn, sn := utf8.DecodeRuneInString(newMid[ns:ne])
		if ro != rn || so != sn {
			break
		}
		os += so
		ns += sn
	}
	for oe > os && ne > ns {
		ro, so := utf8.DecodeLastRuneInString(oldMid[os:oe])
		rn, sn := utf8.DecodeLastRuneInString(newMid[ns:ne])
		if ro != rn || so != sn {
			break
		}
		oe -= so
		ne -= sn
	}
	if os == oe && ns == ne {
		return Hunk{}, false
	}
	return Hunk{OldStart: os, OldEnd: oe, NewStart: ns, NewEnd: ne}, true
}

// conflicts reports whether a change of a and a change of b touch the same
// place, so merging them automatically would guess the user's intent.
func conflicts(a, b []Hunk) bool {
	for _, x := range a {
		for _, y := range b {
			if hunksConflict(x, y) {
				return true
			}
		}
	}
	return false
}

func hunksConflict(a, b Hunk) bool {
	aEmpty, bEmpty := a.OldStart == a.OldEnd, b.OldStart == b.OldEnd
	switch {
	case aEmpty && bEmpty:
		return a.OldStart == b.OldStart
	case aEmpty:
		return b.OldStart <= a.OldStart && a.OldStart < b.OldEnd
	case bEmpty:
		return a.OldStart <= b.OldStart && b.OldStart < a.OldEnd
	default:
		return a.OldStart < b.OldEnd && b.OldStart < a.OldEnd
	}
}

// concatText is the concatenation of two byte strings, read without copying.
type concatText struct {
	a, b string
}

func (t concatText) Len() int { return len(t.a) + len(t.b) }

func (t concatText) Slice(start, end int) string {
	start = clamp(start, 0, t.Len())
	end = clamp(end, 0, t.Len())
	if end < start {
		end = start
	}
	if end <= len(t.a) {
		return t.a[start:end]
	}
	if start >= len(t.a) {
		return t.b[start-len(t.a) : end-len(t.a)]
	}
	return t.a[start:] + t.b[:end-len(t.a)]
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
