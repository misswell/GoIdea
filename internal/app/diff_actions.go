package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

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
			if path := firstActionPath(arg); path != "" {
				m.ShowDiff(path)
				return
			}
			if t := m.tab(); t != nil && t.doc != nil {
				m.ShowDiff(t.doc.Path())
			}
		})
	register("vcs.nextChange", "Next Change", "Git",
		"next diff change", func(m *Model, _ any) {
			if m.diff == nil {
				m.status = "Open a diff or editor with changes first"
				m.invalidate()
				return
			}
			m.StepDiffChange(1)
		})
	register("vcs.prevChange", "Previous Change", "Git",
		"previous diff change", func(m *Model, _ any) {
			if m.diff == nil {
				m.status = "Open a diff or editor with changes first"
				m.invalidate()
				return
			}
			m.StepDiffChange(-1)
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
			continue
		case textdiff.Context:
			flush()
		case textdiff.Delete:
			if current.oldStart < 0 {
				current.oldStart = row.Old - 1
			}
			if row.Old > 0 {
				current.oldEnd = row.Old
			}
		case textdiff.Add:
			if current.newStart < 0 {
				current.newStart = row.New - 1
			}
			if row.New > 0 {
				current.newEnd = row.New
			}
		}
	}
	flush()
	return out
}

// StepDiffChange moves the selected edit, wrapping at either end as the
// IntelliJ diff navigator does.
func (m *Model) StepDiffChange(delta int) {
	d := m.diff
	if d == nil || len(d.changes) == 0 {
		m.status = "There are no differences"
		m.invalidate()
		return
	}
	d.selected = (d.selected + delta) % len(d.changes)
	if d.selected < 0 {
		d.selected += len(d.changes)
	}
	m.status = fmt.Sprintf("Change %d of %d", d.selected+1, len(d.changes))
	m.invalidate()
}

// StepDiffFile moves through a Compare Directories or agent review chain.
func (m *Model) StepDiffFile(delta int) {
	d := m.diff
	if d == nil {
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

// ApplyDiffSide applies the selected edit from the left to the right, or from
// the right to the left, exactly as the diff toolbar actions do.
func (m *Model) ApplyDiffSide(fromLeft bool) {
	d := m.diff
	if d == nil || len(d.changes) == 0 || d.selected < 0 || d.selected >= len(d.changes) {
		m.status = "There is no selected change to apply"
		m.invalidate()
		return
	}
	change := d.changes[d.selected]
	previousLeft, previousRight := d.leftText, d.rightText
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
	d.leftText, d.rightText = left, right
	d.setDiffRows(left, right)
	if fromLeft {
		m.syncDiffDocument(d, false, previousRight, right)
	} else {
		m.syncDiffDocument(d, true, previousLeft, left)
	}
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

// syncDiffDocument mirrors an accepted in-memory side into its open editor.
// The snapshot guard prevents an old compare window from replacing newer user
// edits silently.
func (m *Model) syncDiffDocument(d *diffView, left bool, expected, next string) {
	path := d.rightPath
	if left {
		path = d.leftPath
	}
	if path == "" {
		return
	}
	m.openFileThen(path, func(t *Tab) {
		if m.diff != d || t == nil || t.doc == nil {
			return
		}
		if current := t.doc.Text(); current != expected {
			if left {
				d.leftText = current
			} else {
				d.rightText = current
			}
			d.setDiffRows(d.leftText, d.rightText)
			m.status = "The file changed while comparing; select the change again"
			m.invalidate()
			return
		}
		if t.doc.Text() == next {
			return
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		go func() {
			err := t.doc.ApplyEdits(ctx, editor.Edit{
				Start: 0, End: len(t.doc.Text()), Text: next,
			})
			m.post(func() {
				if m.diff != d {
					return
				}
				if err != nil {
					m.status = "Could not apply the compared change: " + err.Error()
				} else {
					m.status = "Applied the compared change"
				}
				m.invalidate()
			})
		}()
	})
}

// JumpDiffToSource opens the right side, or the left when the right is an
// in-memory result, at the selected edit.
func (m *Model) JumpDiffToSource() {
	d := m.diff
	if d == nil {
		return
	}
	path, line := d.rightPath, 0
	if path == "" {
		path = d.leftPath
	}
	if len(d.changes) > 0 && d.selected >= 0 && d.selected < len(d.changes) {
		change := d.changes[d.selected]
		if d.rightPath != "" {
			line = change.newStart
		} else {
			line = change.oldStart
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
	title, _ := data["title"].(string)
	d := &diffView{
		open: true, path: path, inline: true, leftLabel: left.label, rightLabel: right.label,
		leftText: left.text, rightText: right.text,
		options: m.diffPrefs(),
	}
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
	d := &diffView{
		open: true, path: path, inline: true,
		leftPath: left.path, rightPath: right.path,
		leftLabel: left.label, rightLabel: right.label,
		leftText: left.text, rightText: right.text, loading: true,
		options: m.diffPrefs(),
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
		var err error
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
		if err == nil {
			rightText = read(right)
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
			d.setDiffRows(leftText, rightText)
			m.invalidate()
		})
	}()
}

type diffInputSideValue struct {
	text  string
	label string
	path  string
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
		leftLabel: filepath.Base(target.left), rightLabel: filepath.Base(target.right),
		fileChain: append([]diffFileTarget(nil), chain...), fileIndex: index,
		loading: true, options: m.diffPrefs(),
	}
	m.diff = d
	m.invalidate()
	go func() {
		left, leftErr := os.ReadFile(target.left)
		right, rightErr := os.ReadFile(target.right)
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
			d.leftText, d.rightText = string(left), string(right)
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

func diffToolbar(c *ui.Context, m *Model, p *ui.Theme) {
	d := m.diff
	if d == nil {
		return
	}
	options := m.diffPrefs()
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Button(c, "‹ Change").Disabled(len(d.changes) == 0).
				OnClick(func() { m.StepDiffChange(-1) })
			ui.Button(c, "Change ›").Disabled(len(d.changes) == 0).
				OnClick(func() { m.StepDiffChange(1) })
			ui.Button(c, "Accept Left").Disabled(len(d.changes) == 0).
				OnClick(func() { m.ApplyDiffSide(true) })
			ui.Button(c, "Accept Right").Disabled(len(d.changes) == 0).
				OnClick(func() { m.ApplyDiffSide(false) })
			ui.Button(c, "Jump to Source").Disabled(d.rightPath == "" && d.leftPath == "").
				OnClick(m.JumpDiffToSource)
			ui.Spacer(c)
			ui.Text(c, diffSelectionSummary(d)).FontSize(11).TextColor(p.TextMuted).SingleLine()
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
