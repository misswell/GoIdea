package app

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/textdiff"
)

type overviewMarkKind uint8

const (
	overviewModified overviewMarkKind = iota
	overviewInserted
	overviewDeleted
	overviewConflict
)

// overviewMark is one change on a document's full-height overview lane. The
// byte range is the exact change in the document. The line range is exclusive
// at its end and drives the mark's vertical position, including points for
// changes that add or remove all lines on one side.
type overviewMark struct {
	startByte int
	endByte   int
	startLine int
	endLine   int
	kind      overviewMarkKind
	priority  int
}

func overviewPriority(kind overviewMarkKind) int {
	switch kind {
	case overviewDeleted:
		return 1
	case overviewConflict:
		return 2
	default:
		return 0
	}
}

func overviewLineOffsets(text string) []int {
	lines := textdiff.SplitLines(text)
	offsets := make([]int, len(lines)+1)
	offset := 0
	for i, line := range lines {
		offsets[i] = offset
		offset += len(line)
		if offset < len(text) && text[offset] == '\n' {
			offset++
		}
	}
	offsets[len(lines)] = len(text)
	return offsets
}

func overviewMarkForLines(
	text string,
	startLine, endLine int,
	kind overviewMarkKind,
) overviewMark {
	offsets := overviewLineOffsets(text)
	lineCount := len(offsets) - 1
	startLine = min(max(startLine, 0), lineCount)
	endLine = min(max(endLine, startLine), lineCount)
	return overviewMark{
		startByte: offsets[startLine],
		endByte:   offsets[endLine],
		startLine: startLine,
		endLine:   endLine,
		kind:      kind,
		priority:  overviewPriority(kind),
	}
}

func overviewMarkForBytes(
	text string,
	startByte, endByte int,
	kind overviewMarkKind,
) overviewMark {
	startByte = min(max(startByte, 0), len(text))
	endByte = min(max(endByte, startByte), len(text))
	startLine := strings.Count(text[:startByte], "\n")
	endLine := startLine
	if endByte > startByte {
		endLine = strings.Count(text[:endByte-1], "\n") + 1
	}
	return overviewMark{
		startByte: startByte,
		endByte:   endByte,
		startLine: startLine,
		endLine:   endLine,
		kind:      kind,
		priority:  overviewPriority(kind),
	}
}

func diffOverviewKind(change diffChange) overviewMarkKind {
	oldLines := change.oldEnd - change.oldStart
	newLines := change.newEnd - change.newStart
	switch {
	case oldLines > 0 && newLines > 0:
		return overviewModified
	case oldLines > 0:
		return overviewDeleted
	default:
		return overviewInserted
	}
}

// diffOverviewMarks returns a side pane's real changed ranges. Unified diffs
// use the editor's new side for positions, with inserted ranges filling their
// lines and deleted ranges as points on the line where the old text sat.
func diffOverviewMarks(d *diffView, left, unified bool) []overviewMark {
	if d == nil {
		return nil
	}
	if unified {
		if d.leftText == "" && d.rightText == "" {
			return nil
		}
		marks := make([]overviewMark, 0, len(d.changes)*2)
		for _, change := range d.changes {
			if change.newEnd > change.newStart {
				marks = append(marks, overviewMarkForLines(
					d.rightText, change.newStart, change.newEnd, overviewInserted,
				))
			}
			if change.oldEnd > change.oldStart {
				marks = append(marks, overviewMarkForLines(
					d.rightText, change.newStart, change.newStart, overviewDeleted,
				))
			}
		}
		return marks
	}
	text := d.rightText
	if left {
		text = d.leftText
	}
	if text == "" {
		return nil
	}
	marks := make([]overviewMark, 0, len(d.changes))
	for _, change := range d.changes {
		start, end := change.newStart, change.newEnd
		if left {
			start, end = change.oldStart, change.oldEnd
		}
		marks = append(marks, overviewMarkForLines(
			text, start, end, diffOverviewKind(change),
		))
	}
	return marks
}

func mergeOverviewKind(change mergeChange) overviewMarkKind {
	if change.kind == mergeChangeConflict {
		return overviewConflict
	}
	if change.base == "" {
		return overviewInserted
	}
	changed := change.left
	if change.kind == mergeChangedRight {
		changed = change.right
	}
	if changed == "" {
		return overviewDeleted
	}
	return overviewModified
}

// mergeOverviewMarks mirrors the merge model's live decisions. Side ranges
// disappear as each side is accepted or ignored. Result ranges remain only
// while unresolved and disappear immediately after a manual result edit.
func mergeOverviewMarks(model *mergeEditorModel, pane mergePane) []overviewMark {
	if model == nil {
		return nil
	}
	marks := make([]overviewMark, 0, len(model.changes))
	for _, change := range model.changes {
		var text string
		var start, end int
		switch pane {
		case mergePaneYours:
			if !change.changesSide(mergeLeft) || change.leftDecision != mergeDecisionNone {
				continue
			}
			text, start, end = model.left, change.leftFrom, change.leftTo
		case mergePaneTheirs:
			if !change.changesSide(mergeRight) || change.rightDecision != mergeDecisionNone {
				continue
			}
			text, start, end = model.right, change.rightFrom, change.rightTo
		default:
			if change.resolved() {
				continue
			}
			text, start, end = model.result, change.resultFrom, change.resultTo
		}
		marks = append(marks, overviewMarkForBytes(
			text, start, end, mergeOverviewKind(change),
		))
	}
	return marks
}

func sortedOverviewMarks(marks []overviewMark) []overviewMark {
	out := append([]overviewMark(nil), marks...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].priority < out[j].priority
	})
	return out
}

func overviewColor(kind overviewMarkKind, theme *ui.Theme) ui.Color {
	switch kind {
	case overviewInserted:
		return theme.Success
	case overviewDeleted:
		return theme.Danger
	case overviewConflict:
		return theme.Warning
	default:
		return theme.Accent
	}
}

func overviewLineY(
	state *editor.State,
	line, totalLines int,
	r ui.Rect,
) float32 {
	if state != nil && state.Buffer != nil {
		lastLine := max(1, state.Buffer.LineCount())
		totalPixels := state.LineTop(lastLine)
		if totalPixels > 0 {
			line = min(max(line, 0), lastLine)
			return r.Y + float32(state.LineTop(line)/totalPixels)*r.H
		}
	}
	totalLines = max(1, totalLines)
	line = min(max(line, 0), totalLines)
	return r.Y + float32(line)/float32(totalLines)*r.H
}

// paintOverviewLane draws marks in document order, with deletions above
// inserted ranges and conflicts above every other kind of overlap.
func paintOverviewLane(
	p *ui.Painter,
	r ui.Rect,
	state *editor.State,
	text string,
	marks []overviewMark,
	theme *ui.Theme,
) {
	if p == nil || r.W <= 0 || r.H <= 0 || theme == nil {
		return
	}
	trackWidth := min(2, r.W)
	p.Fill(ui.Rect{
		X: r.X + (r.W-trackWidth)/2,
		Y: r.Y,
		W: trackWidth,
		H: r.H,
	}, theme.Border.Mix(theme.Background, 0.55), 1)
	totalLines := len(overviewLineOffsets(text)) - 1
	for _, mark := range sortedOverviewMarks(marks) {
		top := overviewLineY(state, mark.startLine, totalLines, r)
		bottom := overviewLineY(state, mark.endLine, totalLines, r)
		if bottom < top+2 {
			bottom = top + 2
		}
		top = max(r.Y, min(top, r.Y+r.H))
		bottom = max(top, min(bottom, r.Y+r.H))
		if bottom <= top {
			continue
		}
		width := max(2, r.W-2)
		p.Fill(ui.Rect{
			X: r.X + (r.W-width)/2,
			Y: top,
			W: width,
			H: bottom - top,
		}, overviewColor(mark.kind, theme), 1)
	}
}

func overviewLane(
	c *ui.Context,
	theme *ui.Theme,
	key, label string,
	state *editor.State,
	text string,
	marks []overviewMark,
	height float32,
) ui.Element {
	return ui.Box(c.Key(key)).Width(9).Height(height).Background(theme.Surface).
		Label(label).Draw(func(p *ui.Painter, r ui.Rect) {
		paintOverviewLane(p, r, state, text, marks, theme)
	})
}
