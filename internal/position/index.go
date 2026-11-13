package position

import (
	"math/rand/v2"
	"strings"
)

// lineIndex holds the length of every line, in bytes and UTF-16 units, in an
// implicit treap keyed by line number. A line includes its trailing newline, so
// a text ending in "\n" has a last empty line, as editors show.
//
// Prefix sums in the nodes make line lookup a descent of the tree (O(log n) for
// the texts that occur), and an edit rebuilds only the lines it touched, so a
// keystroke in a million-line file costs the same as one in a small file.
type lineIndex struct {
	root       *lineNode
	totalBytes int64
	totalUnits int64
	lines      int
}

type lineNode struct {
	left, right  *lineNode
	prio         uint64
	bytes, units int64
	sumBytes     int64
	sumUnits     int64
	count        int
}

func (n *lineNode) pull() {
	n.count = 1
	n.sumBytes, n.sumUnits = n.bytes, n.units
	if n.left != nil {
		n.count += n.left.count
		n.sumBytes += n.left.sumBytes
		n.sumUnits += n.left.sumUnits
	}
	if n.right != nil {
		n.count += n.right.count
		n.sumBytes += n.right.sumBytes
		n.sumUnits += n.right.sumUnits
	}
}

func linesOf(n *lineNode) int {
	if n == nil {
		return 0
	}
	return n.count
}

func newLineIndex(text string) *lineIndex {
	root, count := buildLines(text)
	if root == nil {
		// Always one (empty) line: an empty document has a line 0.
		root = &lineNode{prio: rand.Uint64()}
		root.pull()
		count = 1
	}
	return &lineIndex{root: root, totalBytes: root.sumBytes, totalUnits: root.sumUnits, lines: count}
}

// buildLines makes a treap of the lines of text.
func buildLines(text string) (*lineNode, int) {
	var stack []*lineNode
	count := 0
	add := func(start, end int) {
		n := &lineNode{prio: rand.Uint64(), bytes: int64(end - start), units: int64(utf16Len(text[start:end]))}
		n.pull()
		var last *lineNode
		for len(stack) > 0 && stack[len(stack)-1].prio > n.prio {
			last = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}
		n.left = last
		if len(stack) > 0 {
			stack[len(stack)-1].right = n
		}
		stack = append(stack, n)
		count++
	}
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			add(start, i+1)
			start = i + 1
		}
	}
	add(start, len(text))
	root := stack[0]
	repull(root)
	return root, count
}

// repull recomputes every aggregate, bottom-up.
func repull(n *lineNode) {
	if n == nil {
		return
	}
	repull(n.left)
	repull(n.right)
	n.pull()
}

func mergeTree(a, b *lineNode) *lineNode {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.prio < b.prio {
		a.right = mergeTree(a.right, b)
		a.pull()
		return a
	}
	b.left = mergeTree(a, b.left)
	b.pull()
	return b
}

// splitTree splits n into its first k lines and the rest.
func splitTree(n *lineNode, k int) (*lineNode, *lineNode) {
	if n == nil {
		return nil, nil
	}
	lc := linesOf(n.left)
	if k <= lc {
		a, b := splitTree(n.left, k)
		n.left = b
		n.pull()
		return a, n
	}
	a, b := splitTree(n.right, k-lc-1)
	n.right = a
	n.pull()
	return n, b
}

// lineAtByte returns the line holding byte offset off and the byte its line starts at.
func (x *lineIndex) lineAtByte(off int64) (line int, start int64) {
	if off < 0 {
		off = 0
	}
	if off > x.totalBytes {
		off = x.totalBytes
	}
	return locateBytes(x.root, off, 0, 0)
}

func locateBytes(n *lineNode, off int64, line int, base int64) (int, int64) {
	if n == nil {
		return line, base
	}
	lc := linesOf(n.left)
	lb := int64(0)
	if n.left != nil {
		lb = n.left.sumBytes
	}
	if off < base+lb {
		return locateBytes(n.left, off, line, base)
	}
	lineStart := base + lb
	if off < lineStart+n.bytes || n.right == nil {
		return line + lc, lineStart
	}
	return locateBytes(n.right, off, line+lc+1, lineStart+n.bytes)
}

// lineAtUnit returns the line holding UTF-16 offset u and the unit its line starts at.
func (x *lineIndex) lineAtUnit(u int64) (line int, startUnits int64) {
	if u < 0 {
		u = 0
	}
	if u > x.totalUnits {
		u = x.totalUnits
	}
	return locateUnits(x.root, u, 0, 0)
}

func locateUnits(n *lineNode, u int64, line int, base int64) (int, int64) {
	if n == nil {
		return line, base
	}
	lc := linesOf(n.left)
	lu := int64(0)
	if n.left != nil {
		lu = n.left.sumUnits
	}
	if u < base+lu {
		return locateUnits(n.left, u, line, base)
	}
	lineStart := base + lu
	if u < lineStart+n.units || n.right == nil {
		return line + lc, lineStart
	}
	return locateUnits(n.right, u, line+lc+1, lineStart+n.units)
}

// prefixSums returns the bytes and units of the first k lines.
func (x *lineIndex) prefixSums(k int) (bytes, units int64) {
	if k <= 0 {
		return 0, 0
	}
	if k >= x.lines {
		return x.totalBytes, x.totalUnits
	}
	return prefixSums(x.root, k)
}

func prefixSums(n *lineNode, k int) (int64, int64) {
	if n == nil || k <= 0 {
		return 0, 0
	}
	lc := linesOf(n.left)
	if k <= lc {
		return prefixSums(n.left, k)
	}
	var b, u int64
	if n.left != nil {
		b, u = n.left.sumBytes, n.left.sumUnits
	}
	b += n.bytes
	u += n.units
	if k == lc+1 {
		return b, u
	}
	rb, ru := prefixSums(n.right, k-lc-1)
	return b + rb, u + ru
}

// sizes returns the bytes and units of line i, including its newline.
func (x *lineIndex) sizes(i int) (bytes, units int64) {
	if i < 0 || i >= x.lines {
		return 0, 0
	}
	n := x.root
	line := 0
	for n != nil {
		lc := linesOf(n.left)
		switch {
		case i < line+lc:
			n = n.left
		case i == line+lc:
			return n.bytes, n.units
		default:
			line += lc + 1
			n = n.right
		}
	}
	return 0, 0
}

// applyEdit rebuilds the lines an edit touched. oldText is the text before the
// edit, start and oldEnd the replaced byte range in it, and newEnd the end of the
// replacement in newText.
func (x *lineIndex) applyEdit(oldText, newText Text, start, oldEnd, newEnd int) {
	startLine, startLineStart := x.lineAtByte(int64(start))
	endLine, _ := x.lineAtByte(int64(oldEnd))
	endLineBytes, _ := x.sizes(endLine)
	lineEnd := x.lineStartByte(endLine) + endLineBytes

	prefix := oldText.Slice(int(startLineStart), start)
	inserted := newText.Slice(start, newEnd)
	suffix := oldText.Slice(oldEnd, int(lineEnd))
	region := prefix + inserted + suffix
	x.replaceLines(startLine, endLine, region, endLine == x.lines-1)
}

func (x *lineIndex) lineStartByte(line int) int64 {
	b, _ := x.prefixSums(line)
	return b
}

func (x *lineIndex) lineStartUnit(line int) int64 {
	_, u := x.prefixSums(line)
	return u
}

// replaceLines replaces lines start through end (inclusive) with the lines of
// region. atEOF says the replaced lines end the text: a region ending in a
// newline then has its last, empty line, while in the middle of the text that
// empty line would be a phantom duplicate of the following line.
func (x *lineIndex) replaceLines(start, end int, region string, atEOF bool) {
	var old *lineNode
	a, bc := splitTree(x.root, start)
	old, c := splitTree(bc, end-start+1)
	repl, count := buildLines(region)
	if !atEOF && count > 0 && strings.HasSuffix(region, "\n") {
		repl, _ = splitTree(repl, count-1)
	}
	x.root = mergeTree(mergeTree(a, repl), c)
	if x.root == nil {
		x.root = &lineNode{prio: rand.Uint64()}
		x.root.pull()
	}
	x.totalBytes = x.root.sumBytes
	x.totalUnits = x.root.sumUnits
	x.lines = x.root.count
	_ = old
}

// verify walks the index against text, for tests and diagnostics.
func (x *lineIndex) verify(text Text) bool {
	if x.lines != newlineCount(text)+1 {
		return false
	}
	off := 0
	units := 0
	for line := 0; line < x.lines; line++ {
		b, u := x.sizes(line)
		if b < 0 || u < 0 || off+int(b) > text.Len() {
			return false
		}
		if u != int64(utf16Len(text.Slice(off, off+int(b)))) {
			return false
		}
		off += int(b)
		units += int(u)
	}
	return off == text.Len() && units == int(x.totalUnits) && off == int(x.totalBytes)
}

// newlineCount returns the '\n' bytes of text, read in chunks.
func newlineCount(text Text) int {
	const chunk = 1 << 16
	n := 0
	for i := 0; i < text.Len(); i += chunk {
		end := i + chunk
		if end > text.Len() {
			end = text.Len()
		}
		n += strings.Count(text.Slice(i, end), "\n")
	}
	return n
}
