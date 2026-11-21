package app

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type mergeSide string

const (
	mergeLeft  mergeSide = "left"
	mergeRight mergeSide = "right"
)

type mergeChangeKind string

const (
	mergeChangedLeft    mergeChangeKind = "left"
	mergeChangedRight   mergeChangeKind = "right"
	mergeChangedBoth    mergeChangeKind = "both"
	mergeChangeConflict mergeChangeKind = "conflict"
)

type mergeDecision uint8

const (
	mergeDecisionNone mergeDecision = iota
	mergeDecisionAccept
	mergeDecisionIgnore
)

// mergeChange is one stretch of the base changed by one or both sides. The
// result offsets follow edits to the result editor so later commands continue
// to target the same fragment.
type mergeChange struct {
	baseFrom   int
	baseTo     int
	leftFrom   int
	leftTo     int
	rightFrom  int
	rightTo    int
	resultFrom int
	resultTo   int

	left  string
	base  string
	right string
	auto  string
	kind  mergeChangeKind

	leftDecision  mergeDecision
	rightDecision mergeDecision
	acceptOrder   []mergeSide
	manual        bool
}

func (c mergeChange) conflict() bool { return c.kind == mergeChangeConflict }

func (c mergeChange) changesSide(side mergeSide) bool {
	return c.kind == mergeChangedBoth || c.kind == mergeChangeConflict || c.kind == mergeChangeKind(side)
}

func (c mergeChange) resolved() bool {
	if c.manual {
		return true
	}
	if c.conflict() {
		if len(c.acceptOrder) == 2 {
			return true
		}
		for _, decision := range []mergeDecision{c.leftDecision, c.rightDecision} {
			if decision == mergeDecisionNone {
				return false
			}
		}
		return true
	}
	return c.leftDecision != mergeDecisionNone || c.rightDecision != mergeDecisionNone
}

func (c mergeChange) accepted(side mergeSide) bool {
	if side == mergeLeft {
		return c.leftDecision == mergeDecisionAccept
	}
	return c.rightDecision == mergeDecisionAccept
}

// mergeEditorModel is the native equivalent of OpenIDEA's MergeConflictModel.
// It keeps complete versions for the three panes and a result text the user
// can edit directly.
type mergeEditorModel struct {
	path   string
	left   string
	base   string
	right  string
	result string

	leftMissing  bool
	rightMissing bool
	baseMissing  bool
	eol          string

	changes  []mergeChange
	selected int
	compare  string
	auto     bool
}

type mergeDelta struct {
	fromBase int
	toBase   int
	fromSide int
	toSide   int
}

type mergeCluster struct {
	left  [2]int
	base  [2]int
	right [2]int
}

func newMergeEditorModel(path, left, base, right string) *mergeEditorModel {
	normalizedLeft, leftEOL := normalizeMergeText(left)
	normalizedBase, baseEOL := normalizeMergeText(base)
	normalizedRight, rightEOL := normalizeMergeText(right)
	eol := "\n"
	for _, candidate := range []string{baseEOL, leftEOL, rightEOL} {
		if candidate == "\r\n" {
			eol = candidate
			break
		}
	}
	model := &mergeEditorModel{
		path: path, left: normalizedLeft, base: normalizedBase,
		right: normalizedRight, result: normalizedBase, eol: eol,
	}
	model.changes = mergeTextChanges(normalizedLeft, normalizedBase, normalizedRight)
	for i := range model.changes {
		model.changes[i].resultFrom = model.changes[i].baseFrom
		model.changes[i].resultTo = model.changes[i].baseTo
	}
	return model
}

func normalizeMergeText(text string) (string, string) {
	if strings.Contains(text, "\r\n") {
		return strings.ReplaceAll(text, "\r\n", "\n"), "\r\n"
	}
	if strings.Contains(text, "\r") {
		return strings.ReplaceAll(text, "\r", "\n"), "\r"
	}
	return text, "\n"
}

func mergeTextChanges(left, base, right string) []mergeChange {
	leftLines := mergeSplitLines(left)
	baseLines := mergeSplitLines(base)
	rightLines := mergeSplitLines(right)
	clusters := mergeClusters(leftLines, baseLines, rightLines)
	leftOffsets := mergeLineOffsets(leftLines)
	baseOffsets := mergeLineOffsets(baseLines)
	rightOffsets := mergeLineOffsets(rightLines)
	changes := make([]mergeChange, 0, len(clusters))
	for _, cluster := range clusters {
		leftText := mergeSlice(left, leftOffsets, cluster.left)
		baseText := mergeSlice(base, baseOffsets, cluster.base)
		rightText := mergeSlice(right, rightOffsets, cluster.right)
		kind, changed := classifyMergeText(leftText, baseText, rightText)
		if !changed {
			continue
		}
		auto := ""
		if kind == mergeChangeConflict && baseText != "" && leftText != "" && rightText != "" {
			auto = tryResolveMergeText(leftText, baseText, rightText)
		}
		changes = append(changes, mergeChange{
			baseFrom: baseOffsets[cluster.base[0]], baseTo: baseOffsets[cluster.base[1]],
			leftFrom: leftOffsets[cluster.left[0]], leftTo: leftOffsets[cluster.left[1]],
			rightFrom: rightOffsets[cluster.right[0]], rightTo: rightOffsets[cluster.right[1]],
			left: leftText, base: baseText, right: rightText, auto: auto, kind: kind,
		})
	}
	return changes
}

func mergeSplitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := make([]string, 0, strings.Count(text, "\n")+1)
	for len(text) > 0 {
		if newline := strings.IndexByte(text, '\n'); newline >= 0 {
			lines = append(lines, text[:newline+1])
			text = text[newline+1:]
			continue
		}
		lines = append(lines, text)
		break
	}
	return lines
}

func mergeLineOffsets(lines []string) []int {
	offsets := make([]int, len(lines)+1)
	for i, line := range lines {
		offsets[i+1] = offsets[i] + len(line)
	}
	return offsets
}

func mergeSlice(text string, offsets []int, span [2]int) string {
	return text[offsets[span[0]]:offsets[span[1]]]
}

func mergeClusters(left, base, right []string) []mergeCluster {
	leftChanges := mergeLineDeltas(base, left)
	rightChanges := mergeLineDeltas(base, right)
	var clusters []mergeCluster
	i, j, shiftLeft, shiftRight := 0, 0, 0, 0
	for i < len(leftChanges) || j < len(rightChanges) {
		useLeft := i < len(leftChanges) &&
			(j >= len(rightChanges) || leftChanges[i].fromBase <= rightChanges[j].fromBase)
		var first mergeDelta
		if useLeft {
			first = leftChanges[i]
		} else {
			first = rightChanges[j]
		}
		start, end := first.fromBase, first.toBase
		growLeft, growRight := 0, 0
		if useLeft {
			growLeft = mergeDeltaGrowth(first)
			i++
		} else {
			growRight = mergeDeltaGrowth(first)
			j++
		}
		progressed := true
		for progressed {
			progressed = false
			for i < len(leftChanges) && leftChanges[i].fromBase <= end {
				change := leftChanges[i]
				end = max(end, change.toBase)
				growLeft += mergeDeltaGrowth(change)
				i++
				progressed = true
			}
			for j < len(rightChanges) && rightChanges[j].fromBase <= end {
				change := rightChanges[j]
				end = max(end, change.toBase)
				growRight += mergeDeltaGrowth(change)
				j++
				progressed = true
			}
		}
		clusters = append(clusters, mergeCluster{
			left:  [2]int{start + shiftLeft, end + shiftLeft + growLeft},
			base:  [2]int{start, end},
			right: [2]int{start + shiftRight, end + shiftRight + growRight},
		})
		shiftLeft += growLeft
		shiftRight += growRight
	}
	return clusters
}

func mergeDeltaGrowth(change mergeDelta) int {
	return change.toSide - change.fromSide - (change.toBase - change.fromBase)
}

// mergeLineDeltas returns the stretches in which side differs from base. A
// replacement contributes one delta, not separate delete and insert entries.
func mergeLineDeltas(base, side []string) []mergeDelta {
	var deltas []mergeDelta
	width := len(side) + 1
	lcs := make([]int32, (len(base)+1)*width)
	for i := len(base) - 1; i >= 0; i-- {
		for j := len(side) - 1; j >= 0; j-- {
			if base[i] == side[j] {
				lcs[i*width+j] = lcs[(i+1)*width+j+1] + 1
			} else if lcs[(i+1)*width+j] >= lcs[i*width+j+1] {
				lcs[i*width+j] = lcs[(i+1)*width+j]
			} else {
				lcs[i*width+j] = lcs[i*width+j+1]
			}
		}
	}
	i, j := 0, 0
	for i < len(base) || j < len(side) {
		if i < len(base) && j < len(side) && base[i] == side[j] {
			i++
			j++
			continue
		}
		fromBase, fromSide := i, j
		for i < len(base) || j < len(side) {
			if i < len(base) && j < len(side) && base[i] == side[j] {
				break
			}
			if j >= len(side) ||
				i < len(base) && lcs[(i+1)*width+j] >= lcs[i*width+j+1] {
				i++
			} else {
				j++
			}
		}
		deltas = append(deltas, mergeDelta{fromBase: fromBase, toBase: i, fromSide: fromSide, toSide: j})
	}
	return deltas
}

func classifyMergeText(left, base, right string) (mergeChangeKind, bool) {
	if base == "" {
		switch {
		case left == "" && right == "":
			return "", false
		case left == "":
			return mergeChangedRight, true
		case right == "":
			return mergeChangedLeft, true
		case left == right:
			return mergeChangedBoth, true
		default:
			return mergeChangeConflict, true
		}
	}
	if left == "" && right == "" {
		return mergeChangedBoth, true
	}
	leftSame, rightSame := left == base, right == base
	switch {
	case leftSame && rightSame:
		return "", false
	case leftSame:
		return mergeChangedRight, true
	case rightSame:
		return mergeChangedLeft, true
	case left == right:
		return mergeChangedBoth, true
	default:
		return mergeChangeConflict, true
	}
}

var mergeWordPattern = regexp.MustCompile(`[\p{L}\p{M}\p{N}_]+|\r?\n|[^\S\r\n]+|[\s\S]`)
var mergeHorizontalSpace = regexp.MustCompile(`[^\S\r\n]+`)

func tryResolveMergeText(left, base, right string) string {
	if merged, ok := mergeWords(left, base, right, false); ok {
		return merged
	}
	if merged, ok := mergeWords(left, base, right, true); ok {
		return merged
	}
	return ""
}

func mergeWords(left, base, right string, ignoreBlankWidth bool) (string, bool) {
	squash := func(token string) string {
		if ignoreBlankWidth {
			return mergeHorizontalSpace.ReplaceAllString(token, " ")
		}
		return token
	}
	leftTokens := mergeWordPattern.FindAllString(left, -1)
	baseTokens := mergeWordPattern.FindAllString(base, -1)
	rightTokens := mergeWordPattern.FindAllString(right, -1)
	clusters := mergeClustersWithKey(leftTokens, baseTokens, rightTokens, squash)
	joined := func(tokens []string, from, to int) string {
		return strings.Join(tokens[from:to], "")
	}
	var out strings.Builder
	l, b, r := 0, 0, 0
	appendGap := func(leftTo, baseTo, rightTo int) {
		leftGap := joined(leftTokens, l, leftTo)
		baseGap := joined(baseTokens, b, baseTo)
		rightGap := joined(rightTokens, r, rightTo)
		switch {
		case squash(leftGap) != squash(baseGap):
			out.WriteString(leftGap)
		case squash(rightGap) != squash(baseGap):
			out.WriteString(rightGap)
		default:
			out.WriteString(baseGap)
		}
	}
	for _, cluster := range clusters {
		appendGap(cluster.left[0], cluster.base[0], cluster.right[0])
		leftPart := joined(leftTokens, cluster.left[0], cluster.left[1])
		basePart := joined(baseTokens, cluster.base[0], cluster.base[1])
		rightPart := joined(rightTokens, cluster.right[0], cluster.right[1])
		kind, changed := classifyMergeTextWithKey(leftPart, basePart, rightPart, squash)
		if kind == mergeChangeConflict {
			return "", false
		}
		if changed {
			if kind == mergeChangedRight {
				out.WriteString(rightPart)
			} else {
				out.WriteString(leftPart)
			}
		}
		l, b, r = cluster.left[1], cluster.base[1], cluster.right[1]
	}
	appendGap(len(leftTokens), len(baseTokens), len(rightTokens))
	return out.String(), true
}

func mergeClustersWithKey(left, base, right []string, key func(string) string) []mergeCluster {
	leftChanges := mergeLineDeltasWithKey(base, left, key)
	rightChanges := mergeLineDeltasWithKey(base, right, key)
	// The line algorithm works from its delta positions, so reuse it by
	// combining the two side maps here.
	var clusters []mergeCluster
	i, j, shiftLeft, shiftRight := 0, 0, 0, 0
	for i < len(leftChanges) || j < len(rightChanges) {
		useLeft := i < len(leftChanges) &&
			(j >= len(rightChanges) || leftChanges[i].fromBase <= rightChanges[j].fromBase)
		var first mergeDelta
		if useLeft {
			first = leftChanges[i]
		} else {
			first = rightChanges[j]
		}
		start, end := first.fromBase, first.toBase
		growLeft, growRight := 0, 0
		if useLeft {
			growLeft = mergeDeltaGrowth(first)
			i++
		} else {
			growRight = mergeDeltaGrowth(first)
			j++
		}
		progressed := true
		for progressed {
			progressed = false
			for i < len(leftChanges) && leftChanges[i].fromBase <= end {
				change := leftChanges[i]
				end = max(end, change.toBase)
				growLeft += mergeDeltaGrowth(change)
				i++
				progressed = true
			}
			for j < len(rightChanges) && rightChanges[j].fromBase <= end {
				change := rightChanges[j]
				end = max(end, change.toBase)
				growRight += mergeDeltaGrowth(change)
				j++
				progressed = true
			}
		}
		clusters = append(clusters, mergeCluster{
			left:  [2]int{start + shiftLeft, end + shiftLeft + growLeft},
			base:  [2]int{start, end},
			right: [2]int{start + shiftRight, end + shiftRight + growRight},
		})
		shiftLeft += growLeft
		shiftRight += growRight
	}
	return clusters
}

func mergeLineDeltasWithKey(base, side []string, key func(string) string) []mergeDelta {
	baseKeys := make([]string, len(base))
	sideKeys := make([]string, len(side))
	for i := range base {
		baseKeys[i] = key(base[i])
	}
	for i := range side {
		sideKeys[i] = key(side[i])
	}
	width := len(side) + 1
	lcs := make([]int32, (len(base)+1)*width)
	for i := len(base) - 1; i >= 0; i-- {
		for j := len(side) - 1; j >= 0; j-- {
			if baseKeys[i] == sideKeys[j] {
				lcs[i*width+j] = lcs[(i+1)*width+j+1] + 1
			} else if lcs[(i+1)*width+j] >= lcs[i*width+j+1] {
				lcs[i*width+j] = lcs[(i+1)*width+j]
			} else {
				lcs[i*width+j] = lcs[i*width+j+1]
			}
		}
	}
	var deltas []mergeDelta
	i, j := 0, 0
	for i < len(base) || j < len(side) {
		if i < len(base) && j < len(side) && baseKeys[i] == sideKeys[j] {
			i++
			j++
			continue
		}
		fromBase, fromSide := i, j
		for i < len(base) || j < len(side) {
			if i < len(base) && j < len(side) && baseKeys[i] == sideKeys[j] {
				break
			}
			if j >= len(side) ||
				i < len(base) && lcs[(i+1)*width+j] >= lcs[i*width+j+1] {
				i++
			} else {
				j++
			}
		}
		deltas = append(deltas, mergeDelta{fromBase: fromBase, toBase: i, fromSide: fromSide, toSide: j})
	}
	return deltas
}

func classifyMergeTextWithKey(left, base, right string, key func(string) string) (mergeChangeKind, bool) {
	if key(base) == "" {
		switch {
		case key(left) == "":
			return mergeChangedRight, true
		case key(right) == "":
			return mergeChangedLeft, true
		case key(left) == key(right):
			return mergeChangedBoth, true
		default:
			return mergeChangeConflict, true
		}
	}
	if key(left) == "" && key(right) == "" {
		return mergeChangedBoth, true
	}
	leftSame, rightSame := key(left) == key(base), key(right) == key(base)
	switch {
	case leftSame && rightSame:
		return "", false
	case leftSame:
		return mergeChangedRight, true
	case rightSame:
		return mergeChangedLeft, true
	case key(left) == key(right):
		return mergeChangedBoth, true
	default:
		return mergeChangeConflict, true
	}
}

func (m *mergeEditorModel) setResultText(next string) {
	previous := m.result
	if previous == next {
		return
	}
	prefix := 0
	for prefix < len(previous) && prefix < len(next) && previous[prefix] == next[prefix] {
		prefix++
	}
	prefix = mergeUTF8Boundary(previous, prefix)
	suffix := 0
	for suffix < len(previous)-prefix && suffix < len(next)-prefix &&
		previous[len(previous)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}
	suffix = mergeUTF8Boundary(previous, len(previous)-suffix)
	suffix = len(previous) - suffix
	oldEnd := len(previous) - suffix
	newEnd := len(next) - suffix
	delta := (newEnd - prefix) - (oldEnd - prefix)
	for i := range m.changes {
		change := &m.changes[i]
		switch {
		case change.resultTo <= prefix:
		case change.resultFrom >= oldEnd:
			change.resultFrom += delta
			change.resultTo += delta
		default:
			change.manual = true
			change.leftDecision = mergeDecisionAccept
			change.rightDecision = mergeDecisionAccept
			change.resultFrom = min(change.resultFrom, newEnd)
			change.resultTo = max(change.resultFrom, newEnd)
		}
	}
	m.result = next
}

func mergeUTF8Boundary(text string, offset int) int {
	for offset > 0 && offset < len(text) && !utf8.RuneStart(text[offset]) {
		offset--
	}
	return offset
}

func (m *mergeEditorModel) replaceChange(index int, text string) {
	if index < 0 || index >= len(m.changes) {
		return
	}
	change := &m.changes[index]
	from, to := max(0, change.resultFrom), max(change.resultFrom, change.resultTo)
	if from > len(m.result) {
		from = len(m.result)
	}
	if to > len(m.result) {
		to = len(m.result)
	}
	delta := len(text) - (to - from)
	m.result = m.result[:from] + text + m.result[to:]
	change.resultTo = from + len(text)
	for i := range m.changes {
		if i == index || m.changes[i].resultFrom < to {
			continue
		}
		m.changes[i].resultFrom += delta
		m.changes[i].resultTo += delta
	}
}

func (m *mergeEditorModel) renderedChange(change mergeChange) string {
	if change.manual {
		from := min(max(0, change.resultFrom), len(m.result))
		to := min(max(from, change.resultTo), len(m.result))
		return m.result[from:to]
	}
	if !change.conflict() {
		if change.kind == mergeChangedLeft && change.leftDecision == mergeDecisionAccept {
			return change.left
		}
		if change.kind == mergeChangedRight && change.rightDecision == mergeDecisionAccept {
			return change.right
		}
		if change.kind == mergeChangedBoth &&
			(change.leftDecision == mergeDecisionAccept || change.rightDecision == mergeDecisionAccept) {
			return change.left
		}
		return change.base
	}
	if len(change.acceptOrder) > 0 {
		var accepted []string
		for _, side := range change.acceptOrder {
			if side == mergeLeft {
				accepted = append(accepted, change.left)
			} else {
				accepted = append(accepted, change.right)
			}
		}
		if len(accepted) == 2 {
			first, second := accepted[0], accepted[1]
			if first != "" && second != "" && !strings.HasSuffix(first, "\n") {
				first += "\n"
			}
			return first + second
		}
		return accepted[0]
	}
	return change.base
}

func (m *mergeEditorModel) acceptChange(index int, side mergeSide, resolve bool) bool {
	if index < 0 || index >= len(m.changes) {
		return false
	}
	change := &m.changes[index]
	if change.manual || change.accepted(side) || change.resolved() {
		return false
	}
	if change.conflict() {
		if side == mergeLeft {
			change.leftDecision = mergeDecisionAccept
		} else {
			change.rightDecision = mergeDecisionAccept
		}
		change.acceptOrder = append(change.acceptOrder, side)
		if resolve {
			if change.leftDecision == mergeDecisionNone {
				change.leftDecision = mergeDecisionIgnore
			}
			if change.rightDecision == mergeDecisionNone {
				change.rightDecision = mergeDecisionIgnore
			}
		}
	} else if change.changesSide(side) {
		if side == mergeLeft {
			change.leftDecision = mergeDecisionAccept
			change.rightDecision = mergeDecisionIgnore
		} else {
			change.rightDecision = mergeDecisionAccept
			change.leftDecision = mergeDecisionIgnore
		}
	} else {
		change.leftDecision = mergeDecisionIgnore
		change.rightDecision = mergeDecisionIgnore
	}
	m.replaceChange(index, m.renderedChange(*change))
	return true
}

func (m *mergeEditorModel) ignoreChange(index int, side mergeSide) bool {
	if index < 0 || index >= len(m.changes) {
		return false
	}
	change := &m.changes[index]
	if change.manual || change.resolved() {
		return false
	}
	if change.conflict() {
		if side == mergeLeft {
			change.leftDecision = mergeDecisionIgnore
		} else {
			change.rightDecision = mergeDecisionIgnore
		}
	} else {
		change.leftDecision = mergeDecisionIgnore
		change.rightDecision = mergeDecisionIgnore
	}
	m.replaceChange(index, m.renderedChange(*change))
	return true
}

func (m *mergeEditorModel) ignoreWholeChange(index int) bool {
	if index < 0 || index >= len(m.changes) {
		return false
	}
	change := &m.changes[index]
	if change.manual || change.resolved() {
		return false
	}
	change.leftDecision = mergeDecisionIgnore
	change.rightDecision = mergeDecisionIgnore
	change.acceptOrder = nil
	m.replaceChange(index, m.renderedChange(*change))
	return true
}

func (m *mergeEditorModel) resolveAutomatically(index int) bool {
	if index < 0 || index >= len(m.changes) {
		return false
	}
	change := &m.changes[index]
	if change.manual || change.resolved() {
		return false
	}
	if change.conflict() {
		if change.auto == "" {
			return false
		}
		change.leftDecision = mergeDecisionAccept
		change.rightDecision = mergeDecisionAccept
		m.replaceChange(index, change.auto)
		return true
	}
	for _, side := range []mergeSide{mergeLeft, mergeRight} {
		if change.changesSide(side) {
			return m.acceptChange(index, side, true)
		}
	}
	return false
}

func (m *mergeEditorModel) applyNonConflicting(side mergeSide, all bool) int {
	resolved := 0
	for i := len(m.changes) - 1; i >= 0; i-- {
		change := m.changes[i]
		if change.conflict() || change.resolved() ||
			!all && !change.changesSide(side) {
			continue
		}
		target := side
		if all && !change.changesSide(target) {
			if change.changesSide(mergeLeft) {
				target = mergeLeft
			} else if change.changesSide(mergeRight) {
				target = mergeRight
			}
		}
		if m.acceptChange(i, target, true) {
			resolved++
		}
	}
	return resolved
}

func (m *mergeEditorModel) resolveSimple() int {
	resolved := 0
	for i := len(m.changes) - 1; i >= 0; i-- {
		if m.resolveAutomatically(i) {
			resolved++
		}
	}
	return resolved
}

func (m *mergeEditorModel) revertResolved() int {
	reverted := 0
	for i := range m.changes {
		if !m.changes[i].resolved() && !m.changes[i].manual {
			continue
		}
		m.changes[i].leftDecision = mergeDecisionNone
		m.changes[i].rightDecision = mergeDecisionNone
		m.changes[i].acceptOrder = nil
		m.changes[i].manual = false
		reverted++
	}
	m.result = m.base
	for i := range m.changes {
		m.changes[i].resultFrom = m.changes[i].baseFrom
		m.changes[i].resultTo = m.changes[i].baseTo
	}
	return reverted
}

func (m *mergeEditorModel) unresolved() []int {
	var indexes []int
	for i, change := range m.changes {
		if !change.resolved() {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func (m *mergeEditorModel) conflictCount() int {
	count := 0
	for _, change := range m.changes {
		if change.conflict() && !change.resolved() {
			count++
		}
	}
	return count
}

func (m *mergeEditorModel) canApplyNonConflicting(side mergeSide, all bool) bool {
	for _, change := range m.changes {
		if change.conflict() || change.resolved() || change.manual {
			continue
		}
		if all || change.changesSide(side) {
			return true
		}
	}
	return false
}

func (m *mergeEditorModel) canResolveSimple() bool {
	for _, change := range m.changes {
		if change.resolved() || change.manual {
			continue
		}
		if change.conflict() {
			if change.auto != "" {
				return true
			}
		} else {
			return true
		}
	}
	return false
}

func (m *mergeEditorModel) canRevert() bool {
	for _, change := range m.changes {
		if change.resolved() || change.manual {
			return true
		}
	}
	return false
}

func (m *mergeEditorModel) outputText() string {
	if m.eol == "\n" {
		return m.result
	}
	return strings.ReplaceAll(m.result, "\n", m.eol)
}

func mergeTextContainsBinary(text string) bool {
	return strings.IndexByte(text, 0) >= 0
}
