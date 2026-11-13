// Package textdiff computes the line diff the editor's compare view shows.
package textdiff

import "strings"

// Kind is what a diff row is.
type Kind int

const (
	// Context is an unchanged line, kept around changes.
	Context Kind = iota
	// Add is a line only the new text has.
	Add
	// Delete is a line only the old text has.
	Delete
	// Hunk marks the start of a changed region.
	Hunk
)

// Row is one line of a unified diff. Old and New are 1-based line numbers, 0
// where that side has no line.
type Row struct {
	Kind Kind
	Old  int
	New  int
	Text string
}

// SplitLines splits text into lines, without the empty tail a final newline
// would add.
func SplitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if last := len(lines) - 1; lines[last] == "" {
		lines = lines[:last]
	}
	return lines
}

// Compare diffs oldText against newText and returns the unified rows with
// context unchanged lines around each change.
func Compare(oldText, newText string, context int) []Row {
	if context < 0 {
		context = 0
	}
	oldLines, newLines := SplitLines(oldText), SplitLines(newText)
	ops := lineOps(oldLines, newLines)
	rows := make([]Row, 0, len(ops))
	for _, op := range ops {
		rows = append(rows, Row{
			Kind: op.kind,
			Old:  op.old + 1,
			New:  op.new + 1,
			Text: op.text,
		})
	}
	return hunks(rows, context)
}

// op is one line of the line-by-line comparison, in 0-based indices where the
// side that has no line keeps its previous position.
type op struct {
	kind Kind
	old  int
	new  int
	text string
}

// lineOps walks both texts with a longest-common-subsequence table. Equal
// prefixes and suffixes are trimmed first, and a pair too large to tabulate is
// reported as one replaced block, which is still a correct diff.
func lineOps(a, b []string) []op {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix &&
		a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	out := make([]op, 0, len(a)+len(b))
	for i := 0; i < prefix; i++ {
		out = append(out, op{kind: Context, old: i, new: i, text: a[i]})
	}
	ma, mb := a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]
	mid := middleOps(ma, mb)
	for i, o := range mid {
		o.old += prefix
		o.new += prefix
		mid[i] = o
	}
	out = append(out, mid...)
	for i := suffix; i > 0; i-- {
		oi, ni := len(a)-i, len(b)-i
		out = append(out, op{kind: Context, old: oi, new: ni, text: a[oi]})
	}
	return out
}

// middleOps diffs two texts whose first and last lines already matched.
func middleOps(a, b []string) []op {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return replaced(a, b, 0, 0)
	}
	// The table is (n+1)*(m+1) int32s; beyond this the replaced block keeps
	// memory and pauses bounded.
	if int64(n+1)*int64(m+1) > 4_000_000 {
		return replaced(a, b, 0, 0)
	}
	width := m + 1
	lcs := make([]int32, (n+1)*width)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i*width+j] = lcs[(i+1)*width+j+1] + 1
			} else if down, right := lcs[(i+1)*width+j], lcs[i*width+j+1]; down >= right {
				lcs[i*width+j] = down
			} else {
				lcs[i*width+j] = right
			}
		}
	}
	out := make([]op, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, op{kind: Context, old: i, new: j, text: a[i]})
			i, j = i+1, j+1
		case lcs[(i+1)*width+j] >= lcs[i*width+j+1]:
			out = append(out, op{kind: Delete, old: i, text: a[i]})
			i++
		default:
			out = append(out, op{kind: Add, new: j, text: b[j]})
			j++
		}
	}
	out = append(out, replaced(a[i:], b[j:], i, j)...)
	return out
}

// replaced reports every line of a as deleted and every line of b as added,
// starting at the given indices.
func replaced(a, b []string, i, j int) []op {
	out := make([]op, 0, len(a)+len(b))
	for k, line := range a {
		out = append(out, op{kind: Delete, old: i + k, text: line})
	}
	for k, line := range b {
		out = append(out, op{kind: Add, new: j + k, text: line})
	}
	return out
}

// hunks drops context lines farther than context from any change and puts a
// @@ header before each region it keeps.
func hunks(rows []Row, context int) []Row {
	if len(rows) == 0 {
		return nil
	}
	keep := make([]bool, len(rows))
	for i, r := range rows {
		if r.Kind == Context {
			continue
		}
		from, to := max(0, i-context), min(len(rows)-1, i+context)
		for k := from; k <= to; k++ {
			keep[k] = true
		}
	}
	out := make([]Row, 0, len(rows))
	changed := false
	prevOld, prevNew := 0, 0
	advance := func(r Row) {
		if r.Old > 0 {
			prevOld = r.Old
		}
		if r.New > 0 {
			prevNew = r.New
		}
	}
	for i := 0; i < len(rows); {
		if !keep[i] {
			advance(rows[i])
			i++
			continue
		}
		j := i
		for j+1 < len(rows) && keep[j+1] {
			j++
		}
		out = append(out, hunkHeader(rows[i:j+1], prevOld, prevNew))
		out = append(out, rows[i:j+1]...)
		for _, r := range rows[i : j+1] {
			advance(r)
		}
		changed = true
		i = j + 1
	}
	if !changed {
		return nil
	}
	return out
}

// hunkHeader describes a kept region the way a unified diff writes it. A side
// with no line in the region is anchored at the last line before it.
func hunkHeader(region []Row, prevOld, prevNew int) Row {
	oldStart, newStart, oldCount, newCount := 0, 0, 0, 0
	for _, r := range region {
		if r.Kind != Add {
			if oldStart == 0 {
				oldStart = r.Old
			}
			oldCount++
		}
		if r.Kind != Delete {
			if newStart == 0 {
				newStart = r.New
			}
			newCount++
		}
	}
	if oldCount == 0 {
		oldStart = prevOld
	}
	if newCount == 0 {
		newStart = prevNew
	}
	text := "@@ -" + span(oldStart, oldCount) + " +" + span(newStart, newCount) + " @@"
	return Row{Kind: Hunk, Old: oldStart, New: newStart, Text: text}
}

func span(start, count int) string {
	if count == 0 {
		return itoa(start) + ",0"
	}
	if count == 1 {
		return itoa(start)
	}
	return itoa(start) + "," + itoa(count)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [8]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
