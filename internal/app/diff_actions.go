package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
	"mayoi/internal/textdiff"
)

func init() {
	register := func(id, title, category, keywords string, run func(*Model, any)) {
		_ = registerAction(&Action{
			ID: id, Title: title, Category: category, Keywords: keywords, Builtin: true,
			When:    func(m *Model) bool { return m.Root() != "" },
			KeyWhen: func(m *Model) bool { return m.diff != nil },
			Run:     func(m *Model) { run(m, nil) },
			RunWithArg: func(m *Model, arg any) {
				run(m, arg)
			},
		})
	}

	register("diff.nextDifference", "Next Difference", "Diff",
		"next change hunk", func(m *Model, _ any) { m.StepDiffChange(1) })
	register("diff.prevDifference", "Previous Difference", "Diff",
		"previous change hunk", func(m *Model, _ any) { m.StepDiffChange(-1) })
	register("diff.nextFile", "Compare Next File", "Diff",
		"next compared file", func(m *Model, _ any) { m.StepDiffFile(1) })
	register("diff.prevFile", "Compare Previous File", "Diff",
		"previous compared file", func(m *Model, _ any) { m.StepDiffFile(-1) })
	register("diff.applyLeftSide", "Accept Left Side", "Diff",
		"apply left change right", func(m *Model, _ any) { m.ApplyDiffSide(true) })
	register("diff.applyRightSide", "Accept Right Side", "Diff",
		"apply right change left", func(m *Model, _ any) { m.ApplyDiffSide(false) })
	register("diff.acceptLeftAll", "Accept Entire Left Side", "Diff",
		"replace right with left whole file all", func(m *Model, _ any) { m.ApplyWholeDiffSide(true) })
	register("diff.acceptRightAll", "Accept Entire Right Side", "Diff",
		"replace left with right whole file all", func(m *Model, _ any) { m.ApplyWholeDiffSide(false) })
	register("diff.applyResult", "Apply Diff Result", "Diff",
		"apply compare result working copy", func(m *Model, _ any) { m.ApplyDiffResult() })
	register("diff.saveResult", "Save Diff Result", "Diff",
		"save compare result working copy files", func(m *Model, _ any) { m.SaveDiffResult() })
	register("diff.jumpToSource", "Jump to Source", "Diff",
		"open source line", func(m *Model, _ any) { m.JumpDiffToSource() })
	register("diff.showSettings", "Diff Settings", "Diff",
		"ignore whitespace side by side unified", func(m *Model, _ any) { m.ToggleDiffSettings() })
	register("diff.compareFiles", "Compare Files", "Diff",
		"compare two files", func(m *Model, arg any) { m.CompareFiles(arg) })
	register("diff.compareWith", "Compare With…", "Diff",
		"compare files folders", func(m *Model, arg any) { m.ComparePathWith(arg) })
	register("diff.compareDirectories", "Compare Directories", "Diff",
		"compare folders", func(m *Model, arg any) { m.CompareDirectoriesArg(arg) })
	register("diff.compareWithClipboard", "Compare with Clipboard", "Diff",
		"diff clipboard", func(m *Model, arg any) { m.ComparePathWithClipboard(arg) })
	register("diff.compareTexts", "Compare Texts", "Diff",
		"diff text parameter hidden", func(m *Model, arg any) { m.CompareTextsArg(arg) })
	register("vcs.openDiff", "Open Diff", "Diff",
		"diff data", func(m *Model, arg any) { m.OpenDiffArg(arg) })
	register("vcs.showDiffInNewWindow", "Show Diff in New Window", "Git",
		"diff window", func(m *Model, arg any) {
			m.showDiffInAuxiliaryWindow(arg)
		})
	register("vcs.nextChange", "Next Change", "Git",
		"next diff change", func(m *Model, _ any) {
			m.StepEditorOrDiffChange(1)
		})
	register("vcs.prevChange", "Previous Change", "Git",
		"previous diff change", func(m *Model, _ any) {
			m.StepEditorOrDiffChange(-1)
		})

	if builtinShortcuts == nil {
		builtinShortcuts = map[string]string{}
	}
	builtinShortcuts["diff.nextDifference"] = "F7"
	builtinShortcuts["diff.prevDifference"] = "Shift+F7"
	builtinShortcuts["diff.jumpToSource"] = "F4 Cmd+Down"
	builtinShortcuts["diff.applyLeftSide"] = "Ctrl+Cmd+Right"
	builtinShortcuts["diff.applyRightSide"] = "Ctrl+Cmd+Left"
	builtinShortcuts["diff.nextFile"] = "Ctrl+Shift+Right"
	builtinShortcuts["diff.prevFile"] = "Ctrl+Shift+Left"
	builtinShortcuts["diff.showSettings"] = "Cmd+Shift+D"
}

// setDiffRows recomputes the rendered rows, counts and navigation changes for
// the two complete sides.
func (d *diffView) setDiffRows(left, right string) {
	options := d.options.normalized()
	context := diffContext
	if options.Collapse == nil {
		options.Collapse = boolPref(true)
	}
	if !*options.Collapse {
		context = 1_000_000
	}
	d.options = options
	d.rows, d.added, d.gone = diffRowsWithOptions(left, right, context, options)
	d.changes = diffChangesWithOptions(left, right, options)
	if len(d.changes) == 0 {
		d.selected = 0
	} else if d.selected < 0 {
		d.selected = 0
	} else if d.selected >= len(d.changes) {
		d.selected = len(d.changes) - 1
	}
}

// diffChanges converts textdiff's no-context rows into the edits Apply Left
// Side and Apply Right Side use.
func diffChanges(left, right string) []diffChange {
	return diffChangesWithOptions(left, right, DiffPrefs{})
}

func diffChangesWithOptions(left, right string, options DiffPrefs) []diffChange {
	rows := textdiff.CompareWithOptions(left, right, 0, diffComparisonOptions(options.normalized()))
	var out []diffChange
	current := diffChange{oldStart: -1, newStart: -1}
	oldPosition, newPosition := 0, 0
	flush := func() {
		if current.oldStart < 0 && current.newStart < 0 {
			return
		}
		if current.oldStart < 0 {
			current.oldStart = current.oldEnd
		}
		if current.newStart < 0 {
			current.newStart = current.newEnd
		}
		out = append(out, current)
		current = diffChange{oldStart: -1, newStart: -1}
	}
	for _, row := range rows {
		switch row.Kind {
		case textdiff.Hunk:
			flush()
			oldPosition = max(0, row.Old)
			newPosition = max(0, row.New-1)
			continue
		case textdiff.Context:
			flush()
			oldPosition = row.Old
			newPosition = row.New
		case textdiff.Delete:
			if current.oldStart < 0 {
				current.oldStart = row.Old - 1
			}
			if current.newStart < 0 {
				current.newStart = newPosition
			}
			if row.Old > 0 {
				current.oldEnd = row.Old
			}
			oldPosition = current.oldEnd
		case textdiff.Add:
			if current.oldStart < 0 {
				current.oldStart = oldPosition
			}
			if current.newStart < 0 {
				current.newStart = row.New - 1
			}
			if row.New > 0 {
				current.newEnd = row.New
			}
			newPosition = current.newEnd
		}
	}
	flush()
	return out
}

// StepDiffChange moves the selected edit, wrapping at either end as the
// IntelliJ diff navigator does.
func (m *Model) StepDiffChange(delta int) {
	d := m.diff
	if d == nil {
		return
	}
	d.refreshDiffEditorText()
	if len(d.changes) == 0 {
		m.status = "There are no differences"
		m.invalidate()
		return
	}
	d.selected = (d.selected + delta) % len(d.changes)
	if d.selected < 0 {
		d.selected += len(d.changes)
	}
	m.revealDiffChange(d, d.changes[d.selected])
	m.status = fmt.Sprintf("Change %d of %d", d.selected+1, len(d.changes))
	m.invalidate()
}

// editorChangePositions returns the first line of each separate changed hunk
// in a normal editor, using the backend's Git line hunks.
func editorChangePositions(hunks []openidea.LineHunk) []int {
	var positions []int
	for _, hunk := range hunks {
		line := int(hunk.Start)
		if len(positions) == 0 || positions[len(positions)-1] != line {
			positions = append(positions, line)
		}
	}
	return positions
}

// StepEditorOrDiffChange prefers the focused diff navigator, then navigates
// the active editor's Git changes. The backend supplies hunks against HEAD so
// this works without an open GoIDEA diff view.
func (m *Model) StepEditorOrDiffChange(delta int) {
	if m.diff != nil {
		m.StepDiffChange(delta)
		return
	}
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil {
		m.status = "Open a file or diff with changes first"
		m.invalidate()
		return
	}
	path := t.doc.Path()
	text := t.state.Buffer.Snapshot().String()
	caretLine := t.state.Buffer.Snapshot().LineOf(t.state.Cursor())
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_file_at", map[string]any{
			"path": path, "rev": "HEAD",
		})
		var head *string
		if err == nil {
			err = json.Unmarshal(raw, &head)
		}
		var hunks []openidea.LineHunk
		if err == nil && head != nil && *head != text {
			raw, err = m.invoke(ctx, "vcs_diff_hunks", map[string]any{
				"oldText": *head, "newText": text,
			})
		}
		if err == nil {
			err = json.Unmarshal(raw, &hunks)
		}
		m.post(func() {
			if m.Root() != root || m.diff != nil {
				return
			}
			current := m.tab()
			if current == nil || current.doc == nil || current.doc.Path() != path ||
				current.state.Buffer.Snapshot().String() != text {
				return
			}
			if err != nil {
				m.status = "Could not read editor changes: " + err.Error()
				m.invalidate()
				return
			}
			positions := editorChangePositions(hunks)
			if len(positions) == 0 {
				m.status = "There are no differences from HEAD"
				m.invalidate()
				return
			}
			target := positions[0]
			switch {
			case delta > 0:
				for _, line := range positions {
					if line > caretLine {
						target = line
						break
					}
				}
			case delta < 0:
				target = positions[len(positions)-1]
				for i := len(positions) - 1; i >= 0; i-- {
					if positions[i] < caretLine {
						target = positions[i]
						break
					}
				}
			}
			snapshot := current.state.Buffer.Snapshot()
			line := max(0, min(target, snapshot.LineCount()-1))
			current.state.GoTo(editor.Pos{Line: line, Col: 0})
			current.state.ScrollTo(line)
			m.status = fmt.Sprintf(
				"Change %d of %d", indexOfLine(positions, target)+1, len(positions),
			)
			m.invalidate()
		})
	}()
}

func indexOfLine(lines []int, target int) int {
	for i, line := range lines {
		if line == target {
			return i
		}
	}
	return 0
}

func (m *Model) revealDiffChange(d *diffView, change diffChange) {
	if d == nil {
		return
	}
	m.ensureDiffEditors(d)
	state := d.editorFor(d.activeSide == editor.DiffOld)
	if state == nil {
		return
	}
	start, end := change.newStart, change.newEnd
	if d.activeSide == editor.DiffOld {
		start, end = change.oldStart, change.oldEnd
	}
	if start < 0 {
		return
	}
	snapshot := state.Buffer.Snapshot()
	from := snapshot.LineStart(min(start, snapshot.LineCount()-1))
	to := snapshot.Len()
	if end < snapshot.LineCount() {
		to = snapshot.LineStart(max(start, end))
	}
	state.Select(from, to)
	state.ScrollTo(start)
}

// StepDiffFile moves through a Compare Directories or agent review chain.
func (m *Model) StepDiffFile(delta int) {
	d := m.diff
	if d == nil {
		return
	}
	d.refreshDiffEditorText()
	if d.dirty {
		m.status = "Apply, save, or discard the current result before changing files"
		m.invalidate()
		return
	}
	if len(d.agentChain) > 1 {
		next := d.agentIndex + delta
		if next >= 0 && next < len(d.agentChain) {
			m.loadAgentDiff(d.agentTask, d.agentChain, next)
		}
		return
	}
	if len(d.fileChain) > 1 {
		next := d.fileIndex + delta
		if next >= 0 && next < len(d.fileChain) {
			m.loadDiffFile(d.fileChain, next)
		}
		return
	}
	m.status = "This comparison contains one file"
	m.invalidate()
}

func diffChangeContainsLine(change diffChange, line int, left bool) bool {
	start, end := change.newStart, change.newEnd
	if left {
		start, end = change.oldStart, change.oldEnd
	}
	if start < 0 || line < start {
		return false
	}
	return line < end || line == start && start == end
}

func (d *diffView) selectChangeAtCursor() (diffChange, bool) {
	if d == nil {
		return diffChange{}, false
	}
	side := d.rightEditor
	left := d.activeSide == editor.DiffOld
	if left {
		side = d.leftEditor
	}
	if side != nil {
		line := side.CursorPos().Line
		for i, change := range d.changes {
			if diffChangeContainsLine(change, line, left) {
				d.selected = i
				return change, true
			}
		}
	}
	if len(d.changes) == 0 || d.selected < 0 || d.selected >= len(d.changes) {
		return diffChange{}, false
	}
	return d.changes[d.selected], true
}

// ApplyDiffSide applies the change at the active caret, or the current
// navigator selection when the caret is between changes, from the left to the
// right or from the right to the left. The result remains staged in the
// compare view until Apply Result or Save Result.
func (m *Model) ApplyDiffSide(fromLeft bool) {
	d := m.diff
	if d == nil || d.applying {
		return
	}
	d.refreshDiffEditorText()
	if len(d.changes) == 0 {
		m.status = "There is no selected change to apply"
		m.invalidate()
		return
	}
	targetEditable := d.rightEditable
	if !fromLeft {
		targetEditable = d.leftEditable
	}
	if !targetEditable {
		m.status = "This side is read-only"
		m.invalidate()
		return
	}
	m.ensureDiffEditors(d)
	change, ok := d.selectChangeAtCursor()
	if !ok {
		m.status = "There is no selected change to apply"
		m.invalidate()
		return
	}
	leftLines, leftNewline := diffTextLines(d.leftText)
	rightLines, rightNewline := diffTextLines(d.rightText)
	left, right := d.leftText, d.rightText
	if fromLeft {
		if change.oldStart < 0 || change.oldEnd < change.oldStart ||
			change.newStart < 0 || change.newEnd < change.newStart ||
			change.newEnd > len(rightLines) || change.oldEnd > len(leftLines) {
			m.status = "The selected change no longer applies"
			m.invalidate()
			return
		}
		replacement := append([]string(nil), leftLines[change.oldStart:change.oldEnd]...)
		rightLines = replaceDiffLines(rightLines, change.newStart, change.newEnd, replacement)
		right = joinDiffText(rightLines, rightNewline)
	} else {
		if change.newStart < 0 || change.newEnd < change.newStart ||
			change.oldStart < 0 || change.oldEnd < change.oldStart ||
			change.oldEnd > len(leftLines) || change.newEnd > len(rightLines) {
			m.status = "The selected change no longer applies"
			m.invalidate()
			return
		}
		replacement := append([]string(nil), rightLines[change.newStart:change.newEnd]...)
		leftLines = replaceDiffLines(leftLines, change.oldStart, change.oldEnd, replacement)
		left = joinDiffText(leftLines, leftNewline)
	}
	d.message = ""
	d.replaceDiffSide(true, left)
	d.replaceDiffSide(false, right)
	d.setDiffRows(left, right)
	d.dirty = d.leftText != d.leftOriginal || d.rightText != d.rightOriginal
	m.status = "Accepted the change. Use Save Result to write it to disk."
	m.invalidate()
}

// ApplyWholeDiffSide replaces the editable opposite pane with the whole source
// side. This is the Compare Files counterpart of a merge editor's Accept Yours
// or Accept Theirs and remains part of the unsaved compare result.
func (m *Model) ApplyWholeDiffSide(fromLeft bool) {
	d := m.diff
	if d == nil || d.applying {
		return
	}
	d.refreshDiffEditorText()
	targetEditable := d.rightEditable
	sourceText := d.leftText
	left := false
	if !fromLeft {
		targetEditable = d.leftEditable
		sourceText = d.rightText
		left = true
	}
	if !targetEditable {
		m.status = "This side is read-only"
		m.invalidate()
		return
	}
	m.ensureDiffEditors(d)
	d.replaceDiffSide(left, sourceText)
	if left {
		d.leftText = sourceText
	} else {
		d.rightText = sourceText
	}
	d.message = ""
	d.setDiffRows(d.leftText, d.rightText)
	d.dirty = d.leftText != d.leftOriginal || d.rightText != d.rightOriginal
	m.status = "Accepted the entire side. Save Result writes the working copy."
	m.invalidate()
}

// RejectDiffChange copies the opposite side over the active side's selected
// change. In a two-file comparison the active side is the result being
// edited, so this is the mirror of Accept for that pane.
func (m *Model) RejectDiffChange() {
	d := m.diff
	if d == nil || d.applying {
		return
	}
	switch {
	case d.activeSide == editor.DiffOld && d.leftEditable:
		m.ApplyDiffSide(false)
	case d.rightEditable:
		m.ApplyDiffSide(true)
	default:
		m.status = "This comparison has no editable side"
		m.invalidate()
	}
}

// RejectWholeDiffSide restores the active editable side from the opposite
// side. It stages the result in the compare view until Apply Result or Save
// Result, exactly like accepting a whole side.
func (m *Model) RejectWholeDiffSide() {
	d := m.diff
	if d == nil || d.applying {
		return
	}
	switch {
	case d.activeSide == editor.DiffOld && d.leftEditable:
		m.ApplyWholeDiffSide(false)
	case d.rightEditable:
		m.ApplyWholeDiffSide(true)
	default:
		m.status = "This comparison has no editable side"
		m.invalidate()
	}
}

// DiscardDiffResult drops edits made inside the compare panes and returns to
// the two texts shown when the comparison opened. It never touches a working
// copy document.
func (m *Model) DiscardDiffResult() {
	d := m.diff
	if d == nil || d.applying {
		return
	}
	d.refreshDiffEditorText()
	d.replaceDiffSide(true, d.leftOriginal)
	d.replaceDiffSide(false, d.rightOriginal)
	d.leftText, d.rightText = d.leftOriginal, d.rightOriginal
	d.dirty = false
	d.err = ""
	d.message = ""
	d.setDiffRows(d.leftText, d.rightText)
	m.status = "Discarded the diff result"
	m.invalidate()
}

func replaceDiffLines(lines []string, start, end int, replacement []string) []string {
	out := make([]string, 0, len(lines)-(end-start)+len(replacement))
	out = append(out, lines[:start]...)
	out = append(out, replacement...)
	out = append(out, lines[end:]...)
	return out
}

func diffTextLines(text string) ([]string, bool) {
	lines := textdiff.SplitLines(text)
	return lines, strings.HasSuffix(text, "\n")
}

func joinDiffText(lines []string, newline bool) string {
	text := strings.Join(lines, "\n")
	if newline && len(lines) > 0 {
		text += "\n"
	}
	return text
}

// JumpDiffToSource opens the right side, or the left when the right is an
// in-memory result, at the selected edit.
func (m *Model) JumpDiffToSource() {
	d := m.diff
	if d == nil {
		return
	}
	d.refreshDiffEditorText()
	if d.dirty {
		m.status = "Apply, save, or discard the comparison result before jumping to source"
		m.invalidate()
		return
	}
	path, line := d.rightPath, 0
	fromLeft := d.activeSide == editor.DiffOld && d.leftPath != ""
	if path == "" && d.leftPath != "" {
		fromLeft = true
	}
	if fromLeft {
		path = d.leftPath
	}
	if len(d.changes) > 0 && d.selected >= 0 && d.selected < len(d.changes) {
		change := d.changes[d.selected]
		if fromLeft {
			line = change.oldStart
		} else {
			line = change.newStart
		}
	}
	if path == "" {
		m.status = "This side has no source file"
		m.invalidate()
		return
	}
	m.closeDiff()
	m.revealAt(path, max(0, line), 0)
}

// ToggleDiffSettings opens the native form of OpenIDEA's diff toolbar menu.
func (m *Model) ToggleDiffSettings() {
	d := m.diff
	if d == nil {
		return
	}
	d.settingsOpen = !d.settingsOpen
	m.invalidate()
}

func (m *Model) setDiffOptions(next DiffPrefs) {
	d := m.diff
	if d == nil {
		return
	}
	d.refreshDiffEditorText()
	next = next.normalized()
	if next.Collapse == nil {
		next.Collapse = boolPref(true)
	}
	if next.SyncScroll == nil {
		next.SyncScroll = boolPref(true)
	}
	d.options = next
	m.prefs.VCS.Diff = next
	m.savePreferences()
	d.setDiffRows(d.leftText, d.rightText)
	if d.added == 0 && d.gone == 0 && d.leftText != d.rightText {
		d.message = "No differences under the current ignore settings"
	} else if d.added > 0 || d.gone > 0 {
		d.message = ""
	}
	m.invalidate()
}

// RequestCloseDiff protects staged edits made directly in the native compare
// panes. Applying transfers the result to each open document as an undoable,
// unsaved edit. Saving additionally writes those documents through the normal
// save pipeline.
func (m *Model) RequestCloseDiff() {
	d := m.diff
	if d == nil {
		return
	}
	d.refreshDiffEditorText()
	if !d.dirty {
		m.closeDiffNow()
		return
	}
	if d.applying {
		m.status = "Finishing the diff result…"
		m.invalidate()
		return
	}
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Unsaved Diff Result",
			Message:       "Save the editable compare result before closing?",
			Buttons:       []string{"Save & Close", "Apply & Close", "Discard", "Cancel"},
			DefaultButton: 0, CancelButton: 3,
		})
		if err != nil {
			return
		}
		m.post(func() {
			if m.diff != d {
				return
			}
			switch result.Button {
			case 0:
				m.applyDiffResult(true, true)
			case 1:
				m.applyDiffResult(false, true)
			case 2:
				m.closeDiffNow()
			}
		})
	}()
}

type diffResultFile struct {
	left         bool
	path         string
	before       string
	after        string
	version      uint64
	versionKnown bool
}

func (d *diffView) pendingResultFiles() []diffResultFile {
	if d == nil {
		return nil
	}
	d.refreshDiffEditorText()
	var out []diffResultFile
	if d.leftEditable && d.leftPath != "" && d.leftText != d.leftOriginal {
		out = append(out, diffResultFile{
			left: true, path: d.leftPath, before: d.leftOriginal, after: d.leftText,
			version: d.leftDocumentVersion, versionKnown: d.leftVersionKnown,
		})
	}
	if d.rightEditable && d.rightPath != "" && d.rightText != d.rightOriginal {
		out = append(out, diffResultFile{
			path: d.rightPath, before: d.rightOriginal, after: d.rightText,
			version: d.rightDocumentVersion, versionKnown: d.rightVersionKnown,
		})
	}
	return out
}

// ApplyDiffResult transfers the changed editable sides to their working-copy
// documents without saving. The tabs become dirty and keep the standard close
// and quit protection.
func (m *Model) ApplyDiffResult() {
	m.applyDiffResult(false, false)
}

// SaveDiffResult transfers and saves the changed editable sides.
func (m *Model) SaveDiffResult() {
	m.applyDiffResult(true, false)
}

func (m *Model) applyDiffResult(save, closeAfter bool) {
	d := m.diff
	if d == nil || d.applying {
		return
	}
	m.ensureDiffEditors(d)
	d.refreshDiffEditorText()
	files := d.pendingResultFiles()
	if len(files) == 0 {
		if closeAfter {
			m.closeDiffNow()
		} else {
			m.status = "There is no diff result to apply"
			m.invalidate()
		}
		return
	}
	d.applying = true
	d.err = ""
	d.message = ""
	m.status = "Applying the diff result…"
	m.invalidate()

	var applyNext func(int, error)
	applyNext = func(index int, firstErr error) {
		if firstErr != nil || index >= len(files) {
			m.post(func() {
				if m.diff != d {
					return
				}
				d.applying = false
				d.refreshDiffEditorText()
				d.dirty = d.leftText != d.leftOriginal || d.rightText != d.rightOriginal
				if firstErr != nil {
					d.err = "Could not apply the diff result: " + firstErr.Error()
					m.invalidate()
					return
				}
				m.applyActive()
				m.invalidate()
				if closeAfter {
					if d.dirty {
						m.status = "The comparison still has an unsaved in-memory side"
						return
					}
					m.closeDiffNow()
					return
				}
				if save {
					m.status = "Saved the diff result"
					if m.vcs.loaded {
						m.RefreshVCS()
					}
				} else {
					m.status = "Applied the diff result; the working-copy tab is unsaved"
				}
			})
			return
		}
		target := files[index]
		m.openWith(target.path, openSpec{
			at: -1,
			done: func(tab *Tab) {
				if m.diff != d || tab == nil || tab.doc == nil {
					applyNext(index+1, errors.New("the comparison closed"))
					return
				}
				version := tab.doc.Buffer().Version()
				if tab.doc.Text() != target.before ||
					target.versionKnown && version != target.version {
					applyNext(index+1, fmt.Errorf("%s changed while it was being compared",
						filepath.Base(target.path)))
					return
				}
				ctx := m.ctx
				if ctx == nil {
					ctx = context.Background()
				}
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				go func() {
					defer cancel()
					err := tab.doc.ApplyEditsIfVersion(ctx, version, editor.Edit{
						Start: 0, End: len(tab.doc.Text()), Text: target.after,
					})
					if err == nil && save {
						err = cleanForSave(ctx, tab.doc, m.prefs.SaveFiles)
						if err == nil {
							err = tab.doc.Sync(ctx)
						}
						if err == nil {
							err = tab.doc.Save(ctx)
						}
					}
					m.post(func() {
						if err != nil {
							applyNext(index+1, err)
							return
						}
						if save {
							tab.synced = tab.doc.Buffer().Version()
							tab.noteDisk()
						}
						if target.left {
							d.leftOriginal = target.after
							d.leftDocumentVersion = tab.doc.Buffer().Version()
							d.leftVersionKnown = true
						} else {
							d.rightOriginal = target.after
							d.rightDocumentVersion = tab.doc.Buffer().Version()
							d.rightVersionKnown = true
						}
						d.dirty = d.leftText != d.leftOriginal || d.rightText != d.rightOriginal
						applyNext(index+1, nil)
					})
				}()
			},
			failed: func(_ string, err error) {
				applyNext(index+1, err)
			},
		})
	}
	applyNext(0, nil)
}

// CompareFiles opens Compare Files with two explicit paths or the platform's
// multiple-file picker.
func (m *Model) CompareFiles(arg any) {
	paths := actionPaths(arg)
	if len(paths) == 2 {
		m.ShowFileDiff(paths[0], paths[1])
		return
	}
	parent := m.win
	start := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Compare Files", ButtonLabel: "Compare",
			DefaultPath: start, Multiple: true,
		})
		if err != nil || len(paths) != 2 {
			return
		}
		m.post(func() { m.ShowFileDiff(paths[0], paths[1]) })
	}()
}

// ComparePathWith is Diff › Compare With… for a project-view selection, an
// explicit argument or the active editor.
func (m *Model) ComparePathWith(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		if target, ok := m.selectedTreeEntry(); ok {
			path = target.path
		} else if t := m.tab(); t != nil && t.doc != nil {
			path = t.doc.Path()
		}
	}
	if path == "" {
		m.status = "Open or select a file to compare"
		m.invalidate()
		return
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		m.OpenDirCompare(path)
		return
	}
	parent := m.win
	start := filepath.Dir(path)
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Compare With", ButtonLabel: "Compare",
			DefaultPath: start,
		})
		if err != nil || len(paths) == 0 || realPath(paths[0]) == realPath(path) {
			return
		}
		m.post(func() {
			m.openFileThen(path, func(t *Tab) { m.CompareTabWithFile(t, paths[0]) })
		})
	}()
}

// ComparePathWithClipboard mirrors OpenIDEA's Clipboard ↔ file orientation.
func (m *Model) ComparePathWithClipboard(arg any) {
	path := firstActionPath(arg)
	if path == "" {
		if t := m.tab(); t != nil && t.doc != nil {
			path = t.doc.Path()
		}
	}
	if path == "" {
		m.status = "Open or select a file to compare with the clipboard"
		m.invalidate()
		return
	}
	m.openFileThen(path, func(t *Tab) { m.CompareTabWithClipboard(t) })
}

// CompareTextsArg ports the parameterised diff.compareTexts action.
func (m *Model) CompareTextsArg(arg any) {
	data, ok := arg.(map[string]any)
	if !ok {
		m.status = "Compare Texts needs path, left and right"
		m.invalidate()
		return
	}
	path, _ := data["path"].(string)
	left, leftOK := diffTextSide(data["left"])
	right, rightOK := diffTextSide(data["right"])
	if path == "" || !leftOK || !rightOK {
		m.status = "Compare Texts needs path, left and right"
		m.invalidate()
		return
	}
	if m.diff != nil && m.diff.surface == diffSurfaceNewTab {
		m.closeDiff()
	}
	title, _ := data["title"].(string)
	d := &diffView{
		open: true, path: path, inline: true, leftLabel: left.label, rightLabel: right.label,
		leftText: left.text, rightText: right.text,
		options: m.diffPrefs(),
	}
	d.resetDiffEditors()
	d.setDiffRows(left.text, right.text)
	if line, ok := actionInt(data["line"]); ok {
		d.selected = diffChangeAtLine(d.changes, line)
	}
	if title != "" {
		d.label = title
	}
	m.diff = d
	m.invalidate()
}

type diffTextSideValue struct {
	text  string
	label string
}

func diffTextSide(value any) (diffTextSideValue, bool) {
	side, ok := value.(map[string]any)
	if !ok {
		return diffTextSideValue{}, false
	}
	text, ok := side["text"].(string)
	if !ok {
		return diffTextSideValue{}, false
	}
	label, _ := side["label"].(string)
	return diffTextSideValue{text: text, label: label}, true
}

func diffChangeAtLine(changes []diffChange, line int) int {
	if line < 0 {
		return 0
	}
	for i, change := range changes {
		if line >= change.newStart && line <= change.newEnd {
			return i
		}
	}
	return 0
}

// OpenDiffArg accepts the serialisable shape of OpenIDEA's DiffData.
func (m *Model) OpenDiffArg(arg any) {
	data, ok := arg.(map[string]any)
	if !ok {
		return
	}
	path, _ := data["path"].(string)
	left, leftOK := diffInputSide(data["left"])
	right, rightOK := diffInputSide(data["right"])
	if path == "" || !leftOK || !rightOK {
		return
	}
	leftBuffered, leftVersion, leftOpen := m.bufferTextVersion(left.path)
	rightBuffered, rightVersion, rightOpen := m.bufferTextVersion(right.path)
	primaryPath := right.path
	if primaryPath == "" {
		primaryPath = left.path
	}
	if primaryPath == "" {
		primaryPath = path
	}
	d := &diffView{
		open: true, path: primaryPath, inline: true,
		leftPath: left.path, rightPath: right.path,
		leftLabel: left.label, rightLabel: right.label,
		leftText: left.text, rightText: right.text, loading: true,
		leftDocumentVersion: leftVersion, rightDocumentVersion: rightVersion,
		leftVersionKnown: leftOpen, rightVersionKnown: rightOpen,
		options:      m.diffPrefs(),
		leftEditable: left.path != "", rightEditable: right.path != "",
	}
	if left.path == "" {
		d.leftEditable = false
	}
	if right.path == "" {
		d.rightEditable = false
	}
	if left.path != "" {
		d.leftText = ""
	}
	if right.path != "" {
		d.rightText = ""
	}
	m.diff = d
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), time.Minute)
		defer cancel()
		var err error
		readRevision := func(side diffInputSideValue) string {
			sidePath := side.path
			if sidePath == "" {
				sidePath = primaryPath
			}
			raw, readErr := m.invoke(ctx, "vcs_file_at", map[string]any{
				"path": sidePath, "rev": side.rev,
			})
			if readErr != nil {
				err = readErr
				return ""
			}
			var text *string
			if readErr = json.Unmarshal(raw, &text); readErr != nil {
				err = readErr
				return ""
			}
			if text == nil {
				return ""
			}
			return *text
		}
		read := func(side diffInputSideValue) string {
			if side.path == "" {
				return side.text
			}
			data, rerr := os.ReadFile(side.path)
			if rerr != nil {
				err = rerr
				return ""
			}
			return string(data)
		}
		leftText, rightText := read(left), ""
		if left.rev != "" {
			leftText = readRevision(left)
		} else if left.path != "" && leftOpen {
			leftText = leftBuffered
		}
		if err == nil {
			if right.rev != "" {
				rightText = readRevision(right)
			} else if right.path != "" && rightOpen {
				rightText = rightBuffered
			} else {
				rightText = read(right)
			}
		}
		m.post(func() {
			if m.diff != d {
				return
			}
			d.loading = false
			if err != nil {
				d.err = "Reading the compared file failed: " + err.Error()
				m.invalidate()
				return
			}
			d.leftText, d.rightText = leftText, rightText
			d.resetDiffEditors()
			d.setDiffRows(leftText, rightText)
			m.invalidate()
		})
	}()
}

type diffInputSideValue struct {
	text  string
	label string
	path  string
	rev   string
}

func diffInputSide(value any) (diffInputSideValue, bool) {
	side, ok := value.(map[string]any)
	if !ok {
		return diffInputSideValue{}, false
	}
	kind, _ := side["type"].(string)
	label, _ := side["label"].(string)
	switch kind {
	case "text":
		text, ok := side["text"].(string)
		return diffInputSideValue{text: text, label: label}, ok
	case "rev":
		rev, ok := side["rev"].(string)
		path, _ := side["path"].(string)
		return diffInputSideValue{label: label, path: path, rev: rev}, ok && rev != ""
	case "worktree", "file":
		path, ok := side["path"].(string)
		if !ok {
			return diffInputSideValue{}, false
		}
		return diffInputSideValue{path: path, label: label}, true
	}
	return diffInputSideValue{}, false
}

// CompareDirectoriesArg ports the two-path argument and falls back to the
// project-view selection, as diff.compareDirectories does.
func (m *Model) CompareDirectoriesArg(arg any) {
	paths := actionPaths(arg)
	if len(paths) != 2 {
		m.CompareDirectoriesSelected()
		return
	}
	leftInfo, leftErr := os.Stat(paths[0])
	rightInfo, rightErr := os.Stat(paths[1])
	if leftErr != nil || rightErr != nil || !leftInfo.IsDir() || !rightInfo.IsDir() {
		m.status = "Select two directories to compare"
		m.invalidate()
		return
	}
	m.dirCompare = &dirCompareState{
		open: true, left: paths[0], other: paths[1],
	}
	m.ConfirmDirCompare()
}

func actionPaths(arg any) []string {
	switch value := arg.(type) {
	case string:
		if value != "" {
			return []string{realPath(value)}
		}
	case []string:
		out := make([]string, 0, len(value))
		for _, path := range value {
			if path != "" {
				out = append(out, realPath(path))
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if path, ok := item.(string); ok && path != "" {
				out = append(out, realPath(path))
			}
		}
		return out
	case map[string]any:
		if paths, ok := value["paths"].([]string); ok {
			return actionPaths(paths)
		}
		if raw, ok := value["paths"].([]any); ok {
			return actionPaths(raw)
		}
		if path, ok := value["path"].(string); ok && path != "" {
			return []string{realPath(path)}
		}
	}
	return nil
}

func firstActionPath(arg any) string {
	paths := actionPaths(arg)
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func actionInt(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), true
	case json.Number:
		parsed, err := strconv.Atoi(string(number))
		return parsed, err == nil
	}
	return 0, false
}

// loadDiffFile switches a Compare Directories chain to another file.
func (m *Model) loadDiffFile(chain []diffFileTarget, index int) {
	if index < 0 || index >= len(chain) {
		return
	}
	target := chain[index]
	d := &diffView{
		open: true, path: target.right, other: target.left,
		leftPath: target.left, rightPath: target.right,
		leftLabel: m.diffPathLabel(target.left), rightLabel: m.diffPathLabel(target.right),
		fileChain: append([]diffFileTarget(nil), chain...), fileIndex: index,
		loading: true, options: m.diffPrefs(),
		leftEditable: true, rightEditable: true,
	}
	if target.textLoaded {
		d.loading = false
		d.leftPath, d.rightPath = "", ""
		d.leftText, d.rightText = target.leftText, target.rightText
		d.leftLabel, d.rightLabel = target.leftLabel, target.rightLabel
		d.leftEditable, d.rightEditable = target.editable, target.editable
		m.diff = d
		d.resetDiffEditors()
		d.setDiffRows(d.leftText, d.rightText)
		m.invalidate()
		return
	}
	m.diff = d
	m.invalidate()
	leftText, leftVersion, leftOpen := m.bufferTextVersion(target.left)
	rightText, rightVersion, rightOpen := m.bufferTextVersion(target.right)
	go func() {
		var leftErr, rightErr error
		if !leftOpen {
			var data []byte
			data, leftErr = os.ReadFile(target.left)
			leftText = string(data)
		}
		if !rightOpen {
			var data []byte
			data, rightErr = os.ReadFile(target.right)
			rightText = string(data)
		}
		m.post(func() {
			if m.diff != d {
				return
			}
			d.loading = false
			if leftErr != nil || rightErr != nil {
				d.err = "Reading the compared file failed"
				m.invalidate()
				return
			}
			d.leftText, d.rightText = leftText, rightText
			d.leftDocumentVersion, d.rightDocumentVersion = leftVersion, rightVersion
			d.leftVersionKnown, d.rightVersionKnown = leftOpen, rightOpen
			d.resetDiffEditors()
			d.setDiffRows(d.leftText, d.rightText)
			m.invalidate()
		})
	}()
}

// diffBanner names the two complete sides in the toolbar.
func diffBanner(d *diffView) (string, string) {
	left, right := d.leftLabel, d.rightLabel
	if left == "" {
		left = "HEAD"
	}
	if right == "" {
		if d.inline {
			right = "Working tree"
		} else {
			right = "Working tree"
		}
	}
	return left, right
}

// diffRowSelected reports whether a rendered row belongs to the selected
// change. Hunk headers are selected when their following edit is.
func diffRowSelected(d *diffView, index int) bool {
	if len(d.changes) == 0 || d.selected < 0 || d.selected >= len(d.changes) {
		return false
	}
	change := d.changes[d.selected]
	start, end := -1, -1
	for i, row := range d.rows {
		if row.Kind == textdiff.Hunk {
			continue
		}
		inChange := row.Kind == textdiff.Delete &&
			row.Old > change.oldStart && row.Old <= change.oldEnd
		inChange = inChange || row.Kind == textdiff.Add &&
			row.New > change.newStart && row.New <= change.newEnd
		if inChange && start < 0 {
			start = i
		}
		if inChange {
			end = i
		}
	}
	if index >= start && index <= end {
		return true
	}
	if index > 0 && d.rows[index-1].Kind == textdiff.Hunk && index-1 >= start-1 && start >= 0 {
		return true
	}
	return false
}

func (m *Model) diffSelectRow(index int) {
	d := m.diff
	if d == nil || index < 0 || index >= len(d.rows) || len(d.changes) == 0 {
		return
	}
	row := d.rows[index]
	for i, change := range d.changes {
		if row.Kind == textdiff.Delete && row.Old > change.oldStart && row.Old <= change.oldEnd ||
			row.Kind == textdiff.Add && row.New > change.newStart && row.New <= change.newEnd {
			d.selected = i
			m.invalidate()
			return
		}
	}
}

func actionPathOrDefault(m *Model, arg any) string {
	if path := firstActionPath(arg); path != "" {
		return path
	}
	if t := m.tab(); t != nil && t.doc != nil {
		return t.doc.Path()
	}
	return ""
}

func diffActionLabel(d *diffView) string {
	if d.label != "" {
		return d.label
	}
	if d.leftLabel != "" || d.rightLabel != "" {
		return d.leftLabel + " ↔ " + d.rightLabel
	}
	return ""
}

func diffLineText(text string, line int) string {
	lines := textdiff.SplitLines(text)
	if line < 0 || line >= len(lines) {
		return ""
	}
	return lines[line]
}

func diffSelectionSummary(d *diffView) string {
	if len(d.changes) == 0 || d.selected < 0 || d.selected >= len(d.changes) {
		return "No differences"
	}
	change := d.changes[d.selected]
	return fmt.Sprintf("%d of %d · lines %d–%d → %d–%d",
		d.selected+1, len(d.changes),
		change.oldStart+1, max(change.oldStart+1, change.oldEnd),
		change.newStart+1, max(change.newStart+1, change.newEnd))
}

func diffSelectionSide(d *diffView, left bool) string {
	if d == nil || len(d.changes) == 0 || d.selected < 0 || d.selected >= len(d.changes) {
		return ""
	}
	change := d.changes[d.selected]
	if left {
		lines := textdiff.SplitLines(d.leftText)
		if change.oldStart >= 0 && change.oldEnd <= len(lines) {
			return strings.Join(lines[change.oldStart:change.oldEnd], "\n")
		}
		return ""
	}
	lines := textdiff.SplitLines(d.rightText)
	if change.newStart >= 0 && change.newEnd <= len(lines) {
		return strings.Join(lines[change.newStart:change.newEnd], "\n")
	}
	return ""
}

func (m *Model) showDiffStatus(text string) {
	m.status = text
	m.invalidate()
}

func (m *Model) diffPaneContextMenu(menu *ui.Menu, left bool) {
	d := m.diff
	if d == nil || menu == nil {
		return
	}
	d.activeSide = editor.DiffNew
	if left {
		d.activeSide = editor.DiffOld
	}
	menu.Submenu("Accept Difference", func(menu *ui.Menu) {
		acceptLeft := menu.Item("From Left").Disabled(!d.rightEditable || len(d.changes) == 0)
		if acceptLeft.Chosen() {
			m.ApplyDiffSide(true)
		}
		acceptRight := menu.Item("From Right").Disabled(!d.leftEditable || len(d.changes) == 0)
		if acceptRight.Chosen() {
			m.ApplyDiffSide(false)
		}
	})
	menu.Submenu("Accept Entire Side", func(menu *ui.Menu) {
		useLeft := menu.Item("Use Left").Disabled(!d.rightEditable)
		if useLeft.Chosen() {
			m.ApplyWholeDiffSide(true)
		}
		useRight := menu.Item("Use Right").Disabled(!d.leftEditable)
		if useRight.Chosen() {
			m.ApplyWholeDiffSide(false)
		}
	})
	reject := menu.Item("Reject Change").Disabled(len(d.changes) == 0 ||
		!d.leftEditable && !d.rightEditable || d.applying)
	if reject.Chosen() {
		m.RejectDiffChange()
	}
	rejectSide := menu.Item("Reject Entire Side").Disabled(
		!d.leftEditable && !d.rightEditable || d.applying,
	)
	if rejectSide.Chosen() {
		m.RejectWholeDiffSide()
	}
	menu.Separator()
	apply := menu.Item("Apply Result").Disabled(!d.dirty || d.applying)
	if apply.Chosen() {
		m.ApplyDiffResult()
	}
	save := menu.Item("Save Result").Disabled(!d.dirty || d.applying)
	if save.Chosen() {
		m.SaveDiffResult()
	}
	discard := menu.Item("Discard Result").Disabled(!d.dirty || d.applying)
	if discard.Chosen() {
		m.DiscardDiffResult()
	}
	if left {
		menu.Separator()
		if menu.Item("Jump to Source").Chosen() {
			m.JumpDiffToSource()
		}
	}
}

func diffToolbar(c *ui.Context, m *Model, p *ui.Theme) {
	d := m.diff
	if d == nil {
		return
	}
	options := m.diffPrefs()
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Button(c, "‹ Change").Disabled(len(d.changes) == 0).
				OnClick(func() { m.StepDiffChange(-1) })
			ui.Button(c, "Change ›").Disabled(len(d.changes) == 0).
				OnClick(func() { m.StepDiffChange(1) })
			ui.Button(c, "Accept Left").Disabled(len(d.changes) == 0 || !d.rightEditable).
				OnClick(func() { m.ApplyDiffSide(true) })
			ui.Button(c, "Accept Right").Disabled(len(d.changes) == 0 || !d.leftEditable).
				OnClick(func() { m.ApplyDiffSide(false) })
			ui.Button(c, "Use Left for All").Disabled(!d.rightEditable).
				OnClick(func() { m.ApplyWholeDiffSide(true) })
			ui.Button(c, "Use Right for All").Disabled(!d.leftEditable).
				OnClick(func() { m.ApplyWholeDiffSide(false) })
			ui.Button(c, "Reject Change").Disabled(len(d.changes) == 0 ||
				!d.leftEditable && !d.rightEditable || d.applying).
				Tooltip("Restore the selected change from the opposite side").
				OnClick(m.RejectDiffChange)
			ui.Button(c, "Reject Entire Side").Disabled(
				!d.leftEditable && !d.rightEditable || d.applying,
			).Tooltip("Restore the active editable side from the opposite side").
				OnClick(m.RejectWholeDiffSide)
			ui.Button(c, "Apply Result").Disabled(!d.dirty || d.applying).
				OnClick(m.ApplyDiffResult)
			ui.Button(c, "Save Result").Disabled(!d.dirty || d.applying).
				OnClick(m.SaveDiffResult)
			ui.Button(c, "Discard Result").Disabled(!d.dirty || d.applying).
				Tooltip("Drop edits made inside the comparison without touching files").
				OnClick(m.DiscardDiffResult)
			ui.Button(c, "Jump to Source").Disabled(d.rightPath == "" && d.leftPath == "").
				OnClick(m.JumpDiffToSource)
			ui.Spacer(c)
			ui.Text(c, diffSelectionSummary(d)).FontSize(11).TextColor(p.TextMuted).SingleLine()
			if d.dirty {
				state := "Unsaved result"
				if d.applying {
					state = "Applying…"
				}
				ui.Text(c, state).FontSize(11).TextColor(p.Accent).SingleLine()
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			mode := diffModeLabel(options.Mode)
			if choice := ui.Select(c, &mode, []string{"Side by side", "Unified"}).Width(124); choice.Changed() {
				options.Mode = diffModeValue(mode)
				m.setDiffOptions(options)
			}
			ignore := diffIgnoreLabel(options.Ignore)
			if choice := ui.Select(c, &ignore, diffIgnoreLabels).Width(230); choice.Changed() {
				options.Ignore = diffIgnoreValue(ignore)
				m.setDiffOptions(options)
			}
			highlight := diffHighlightLabel(options.Highlight)
			if choice := ui.Select(c, &highlight, diffHighlightLabels).Width(155); choice.Changed() {
				options.Highlight = diffHighlightValue(highlight)
				m.setDiffOptions(options)
			}
			collapse := prefBool(options.Collapse, true)
			ui.Button(c, "Collapse: "+onOff(collapse)).OnClick(func() {
				options.Collapse = boolPref(!collapse)
				m.setDiffOptions(options)
			})
			syncScroll := prefBool(options.SyncScroll, true)
			ui.Button(c, "Sync Scroll: "+onOff(syncScroll)).OnClick(func() {
				options.SyncScroll = boolPref(!syncScroll)
				m.setDiffOptions(options)
			})
			ui.Button(c, "Diff Settings…").OnClick(m.ToggleDiffSettings)
		})
	})
}

func diffSettingsPanel(c *ui.Context, m *Model, p *ui.Theme) {
	d := m.diff
	if d == nil {
		return
	}
	options := m.diffPrefs()
	ui.Column(c).Padding(8).Gap(8).Background(p.Surface).
		BorderWidth(1).BorderColor(p.Border).Children(func() {
		ui.Text(c, "Diff Settings").FontSize(12).Bold().TextColor(p.Text)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ignore := diffIgnoreLabel(options.Ignore)
			if choice := ui.Select(c, &ignore, diffIgnoreLabels).Width(270); choice.Changed() {
				options.Ignore = diffIgnoreValue(ignore)
				m.setDiffOptions(options)
			}
			highlight := diffHighlightLabel(options.Highlight)
			if choice := ui.Select(c, &highlight, diffHighlightLabels).Width(170); choice.Changed() {
				options.Highlight = diffHighlightValue(highlight)
				m.setDiffOptions(options)
			}
			softWrap := diffSoftWrapLabel(options.SoftWrap)
			if choice := ui.Select(c, &softWrap, diffSoftWrapLabels).Width(160); choice.Changed() {
				options.SoftWrap = diffSoftWrapValue(softWrap)
				m.setDiffOptions(options)
			}
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			collapse := prefBool(options.Collapse, true)
			ui.Button(c, "Collapse Unchanged: "+onOff(collapse)).OnClick(func() {
				options.Collapse = boolPref(!collapse)
				m.setDiffOptions(options)
			})
			syncScroll := prefBool(options.SyncScroll, true)
			ui.Button(c, "Synchronize Scrolling: "+onOff(syncScroll)).OnClick(func() {
				options.SyncScroll = boolPref(!syncScroll)
				m.setDiffOptions(options)
			})
			ui.Button(c, "Align Changes: "+onOff(options.AlignChanges)).OnClick(func() {
				options.AlignChanges = !options.AlignChanges
				m.setDiffOptions(options)
			})
			ui.Spacer(c)
			ui.Button(c, "Close").OnClick(m.ToggleDiffSettings)
		})
	})
}

var diffIgnoreLabels = []string{
	"Do not ignore",
	"Trim whitespaces",
	"Ignore whitespaces",
	"Ignore whitespaces and empty lines",
}

var diffHighlightLabels = []string{"Highlight words", "Highlight lines", "Do not highlight"}
var diffSoftWrapLabels = []string{"Soft-wrap: follow editor", "Soft-wrap: on", "Soft-wrap: off"}

func onOff(on bool) string {
	if on {
		return "On"
	}
	return "Off"
}

func diffModeLabel(value string) string {
	if value == "unified" {
		return "Unified"
	}
	return "Side by side"
}

func diffModeValue(label string) string {
	if label == "Unified" {
		return "unified"
	}
	return "split"
}

func diffIgnoreLabel(value string) string {
	switch value {
	case "trim":
		return "Trim whitespaces"
	case "whitespace":
		return "Ignore whitespaces"
	case "whitespaceAndLines":
		return "Ignore whitespaces and empty lines"
	default:
		return "Do not ignore"
	}
}

func diffIgnoreValue(label string) string {
	switch label {
	case "Trim whitespaces":
		return "trim"
	case "Ignore whitespaces":
		return "whitespace"
	case "Ignore whitespaces and empty lines":
		return "whitespaceAndLines"
	default:
		return "none"
	}
}

func diffHighlightLabel(value string) string {
	switch value {
	case "lines":
		return "Highlight lines"
	case "none":
		return "Do not highlight"
	default:
		return "Highlight words"
	}
}

func diffHighlightValue(label string) string {
	switch label {
	case "Highlight lines":
		return "lines"
	case "Do not highlight":
		return "none"
	default:
		return "words"
	}
}

func diffSoftWrapLabel(value string) string {
	switch value {
	case "on":
		return "Soft-wrap: on"
	case "off":
		return "Soft-wrap: off"
	default:
		return "Soft-wrap: follow editor"
	}
}

func diffSoftWrapValue(label string) string {
	switch label {
	case "Soft-wrap: on":
		return "on"
	case "Soft-wrap: off":
		return "off"
	default:
		return "follow"
	}
}

func diffUnifiedLine(c *ui.Context, p *ui.Theme, row textdiff.Row, highlight string, softWrap bool) {
	color := p.Text
	if highlight != "none" {
		switch row.Kind {
		case textdiff.Add:
			color = p.Success
		case textdiff.Delete:
			color = p.Danger
		}
	}
	layoutRow := ui.Row(c).Grow(1).MinWidth(0).MinHeight(22).Padding(0, 8).Gap(6).
		AlignItems(ui.Center)
	if !softWrap {
		layoutRow.Height(22)
	}
	layoutRow.Children(func() {
		ui.Text(c, diffSign(row.Kind)).Width(14).Font("monospace").FontSize(12).
			TextColor(color).SingleLine()
		ui.Text(c, fmt.Sprintf("%d:%d", row.Old, row.New)).Width(76).
			Font("monospace").FontSize(11).TextColor(p.TextMuted).SingleLine()
		text := ui.Text(c, row.Text).Font("monospace").FontSize(13).TextColor(color)
		if softWrap {
			text.Grow(1).MinWidth(0)
		} else {
			text.NoWrap()
		}
	})
}

func diffPaneLine(c *ui.Context, p *ui.Theme, line int, text string, present, changed, softWrap bool) {
	rowHeight := ui.Row(c).Grow(1).MinWidth(0).MinHeight(22).Padding(0, 8).Gap(6)
	if !softWrap {
		rowHeight.Height(22)
	}
	rowHeight.
		AlignItems(ui.Center).Children(func() {
		ui.Row(c).Width(38).Shrink(0).Justify(ui.End).Children(func() {
			ui.Text(c, lineNumber(line)).FontSize(12).Font("monospace").
				TextColor(p.TextMuted).SingleLine()
		})
		if !present {
			ui.Spacer(c)
			return
		}
		color := p.Text
		if changed {
			color = p.Accent
		}
		lineText := ui.Text(c, text).FontSize(13).Font("monospace").TextColor(color)
		if softWrap {
			lineText.Grow(1).MinWidth(0)
		} else {
			lineText.NoWrap()
		}
	})
}
