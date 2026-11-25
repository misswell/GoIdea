package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/linediff"
	"mayoi/internal/openidea"
	"mayoi/internal/textdiff"
)

// diffContext is how many unchanged lines the compare view keeps around a
// change, as IDEA's default does.
const diffContext = 3

const diffEditorHeight float32 = 510

// diffView is the editor's compare-with-HEAD window: the loaded unified diff
// and where it came from.
type diffView struct {
	open  bool
	path  string
	rows  []textdiff.Row
	added int
	gone  int
	// leftText and rightText are the complete sides. Keeping them lets the
	// compare view navigate hunks and apply either side without re-reading a
	// file or guessing from the context-trimmed rows.
	leftText  string
	rightText string
	leftPath  string
	rightPath string
	// Editable comparisons keep the working texts in native MyGo editor
	// states. Their edits stay here until Apply Result or Save Result, so a
	// dismissed comparison cannot silently overwrite an open document.
	leftEditor           *editor.State
	rightEditor          *editor.State
	leftOriginal         string
	rightOriginal        string
	leftEditable         bool
	rightEditable        bool
	leftEditorVersion    uint64
	rightEditorVersion   uint64
	leftDocumentVersion  uint64
	rightDocumentVersion uint64
	leftVersionKnown     bool
	rightVersionKnown    bool
	activeSide           editor.DiffSide
	dirty                bool
	applying             bool
	editorScrollReady    bool
	editorScrollSyncing  bool
	lastEditorLeftY      float64
	lastEditorRightY     float64
	changes              []diffChange
	selected             int
	// leftLabel and rightLabel name an agent review's two versions instead of
	// the plain HEAD/current labels of the Git compare.
	leftLabel  string
	rightLabel string
	// agentChain is the files of one review, so Previous and Next stay inside
	// the same task and commit group.
	agentTask  string
	agentChain []agentDiffTarget
	agentIndex int
	// other is the right-hand file of a two-file compare; empty means the
	// compare is against HEAD.
	other string
	// fileChain carries Compare Directories files for Previous/Next File.
	fileChain []diffFileTarget
	fileIndex int
	// inline marks a compare whose right side is text rather than a file, as
	// Compare with Clipboard; label names that side.
	inline bool
	label  string
	// options are the shared Diff Settings choices shown by the toolbar.
	options      DiffPrefs
	settingsOpen bool
	// Each layout keeps its own scroll positions. Split panes can scroll
	// independently, or transfer the user's movement to the other side when
	// Synchronize Scrolling is on.
	unifiedScroll ui.ScrollState
	leftScroll    ui.ScrollState
	rightScroll   ui.ScrollState
	scrollReady   bool
	lastUnifiedX  float32
	lastUnifiedY  float32
	lastLeftX     float32
	lastLeftY     float32
	lastRightX    float32
	lastRightY    float32

	loading bool
	err     string
	message string
}

// diffChange is one non-overlapping edit in the full texts. Starts and ends
// are zero-based and end is exclusive.
type diffChange struct {
	oldStart int
	oldEnd   int
	newStart int
	newEnd   int
}

// diffFileTarget is one file of a multi-file compare such as Compare
// Directories. Previous/Next File moves through this list.
type diffFileTarget struct {
	left  string
	right string
	label string
}

// diffPaneRow is one row of one side. source is its index in diffView.rows,
// and present is false for the empty spacer an aligned pane keeps opposite a
// line the other document has by itself.
type diffPaneRow struct {
	source  int
	row     textdiff.Row
	present bool
}

// diffRowsForPane removes the other side's rows for unaligned panes, or keeps
// them as empty spacers for aligned panes.
func diffRowsForPane(rows []textdiff.Row, left, align bool) []diffPaneRow {
	out := make([]diffPaneRow, 0, len(rows))
	for i, row := range rows {
		present := row.Kind != textdiff.Add
		if !left {
			present = row.Kind != textdiff.Delete
		}
		if present || align {
			out = append(out, diffPaneRow{source: i, row: row, present: present})
		}
	}
	return out
}

// diffRows compares old against cur and counts the added and removed lines.
func diffRows(old, cur string) ([]textdiff.Row, int, int) {
	return diffRowsWithOptions(old, cur, diffContext, DiffPrefs{})
}

func diffRowsWithOptions(old, cur string, context int, options DiffPrefs) ([]textdiff.Row, int, int) {
	options = options.normalized()
	rows := textdiff.CompareWithOptions(old, cur, context, diffComparisonOptions(options))
	added, gone := 0, 0
	for _, r := range rows {
		switch r.Kind {
		case textdiff.Add:
			added++
		case textdiff.Delete:
			gone++
		}
	}
	return rows, added, gone
}

func diffComparisonOptions(options DiffPrefs) textdiff.Options {
	return textdiff.Options{
		TrimWhitespace:        options.Ignore == "trim",
		IgnoreWhitespace:      options.Ignore == "whitespace",
		IgnoreWhitespaceLines: options.Ignore == "whitespaceAndLines",
	}
}

func (m *Model) diffPrefs() DiffPrefs {
	options := m.prefs.VCS.Diff.normalized()
	if options.Collapse == nil {
		options.Collapse = boolPref(true)
	}
	if options.SyncScroll == nil {
		options.SyncScroll = boolPref(true)
	}
	return options
}

// diffUsesSoftWrap resolves Diff Settings' Follow editor choice against the
// same file-pattern setting the text editors use.
func (m *Model) diffUsesSoftWrap(d *diffView, options DiffPrefs) bool {
	switch options.SoftWrap {
	case "on":
		return true
	case "off":
		return false
	default:
		return d != nil && d.path != "" && m.prefs.Editor.WrapsFile(d.path)
	}
}

func (d *diffView) hasEditablePanes() bool {
	return d != nil && (d.leftEditable || d.rightEditable)
}

// resetDiffEditors starts both panes from the newly loaded comparison. It is
// called whenever a file, chain entry, or text pair replaces the old texts.
func (d *diffView) resetDiffEditors() {
	d.leftEditor = nil
	d.rightEditor = nil
	d.leftOriginal = d.leftText
	d.rightOriginal = d.rightText
	d.leftEditorVersion = 0
	d.rightEditorVersion = 0
	d.dirty = false
	d.editorScrollReady = false
	d.editorScrollSyncing = false
	d.lastEditorLeftY = 0
	d.lastEditorRightY = 0
	d.activeSide = editor.DiffNew
}

func (m *Model) newDiffEditor(path, text string, readOnly bool) *editor.State {
	fontSize := m.editorFontSize()
	font := m.prefs.Font
	state := &editor.State{
		Buffer:     editor.NewBuffer(text),
		LineHeight: font.lineHeightPixels(fontSize) / fontSize,
		Bindings:   m.diffEditorBindings(),
		ReadOnly:   readOnly,
		NoFolding:  true,
		Label:      filepath.Base(path),
	}
	state.Font = ui.Font{
		Family:   font.fontFamily(),
		Size:     fontSize,
		Weight:   font.weight(),
		Features: font.features(),
	}
	if path != "" {
		state.Buffer.SetLanguage(editor.LanguageFor(path))
	}
	return state
}

// diffEditorBindings keeps the normal keymap conflict resolution while
// treating Find as available in a Diff pane. Find actions normally require an
// active tab, but Diff can be shown without one.
func (m *Model) diffEditorBindings() []editor.Binding {
	candidates := editorBindingCandidatesFor(m.keymapPresetID(), m.keymapUserBindings())
	byBinding := map[string][]keymapDispatchCandidate{}
	for _, action := range m.keymapEffectiveActions() {
		for _, binding := range action.Bindings {
			strokes, ok := keymapInputStrokes(binding)
			if !ok {
				continue
			}
			normalized := keymapNormalize(binding)
			byBinding[normalized] = append(byBinding[normalized], keymapDispatchCandidate{
				ActionID: action.ID,
				Binding:  normalized,
				Strokes:  strokes,
			})
		}
	}
	bindings := make([]editor.Binding, 0, len(candidates))
	for _, candidate := range candidates {
		winner, ok := m.diffKeymapWinningCandidate(byBinding[candidate.Binding])
		if !ok || canonicalActionID(winner.ActionID) != canonicalActionID(candidate.ActionID) {
			continue
		}
		bindings = append(bindings, candidate.Entry)
	}
	return bindings
}

func (m *Model) diffKeymapWinningCandidate(
	candidates []keymapDispatchCandidate,
) (keymapDispatchCandidate, bool) {
	fallback := make([]keymapDispatchCandidate, 0, len(candidates))
	for i := len(candidates) - 1; i >= 0; i-- {
		candidate := candidates[i]
		action, ok := actionGet(candidate.ActionID)
		if !ok {
			continue
		}
		find := isDiffFindEditorAction(action.ID)
		if !find && !action.usable(m) {
			continue
		}
		if find && action.Enabled != nil && !action.Enabled(m) {
			continue
		}
		if action.KeyWhen == nil && !find {
			fallback = append(fallback, candidate)
			continue
		}
		if find || action.KeyWhen(m) {
			return candidate, true
		}
	}
	if len(fallback) > 0 {
		return fallback[0], true
	}
	return keymapDispatchCandidate{}, false
}

func isDiffFindEditorAction(id string) bool {
	switch canonicalActionID(id) {
	case "find.inFile", "find.replaceInFile", "find.next", "find.previous":
		return true
	default:
		return false
	}
}

// ensureDiffEditors creates the native editor states after loading or on the
// first frame. Read-only comparison panes keep selection, Find, and copy
// behavior, matching the native editors used by the main workbench.
func (m *Model) ensureDiffEditors(d *diffView) {
	if d == nil {
		return
	}
	if d.leftEditor == nil {
		d.leftEditor = m.newDiffEditor(d.leftPath, d.leftText, !d.leftEditable)
		d.leftEditor.Label = "Compare left"
		d.leftEditorVersion = d.leftEditor.Buffer.Version()
		d.leftEditor.ContextMenu = func(menu *ui.Menu) {
			m.diffPaneContextMenu(menu, true)
		}
		d.leftEditor.OnInput = func(ui.InputEvent) bool {
			d.activeSide = editor.DiffOld
			return false
		}
	}
	if d.rightEditor == nil {
		d.rightEditor = m.newDiffEditor(d.rightPath, d.rightText, !d.rightEditable)
		d.rightEditor.Label = "Compare right"
		d.rightEditorVersion = d.rightEditor.Buffer.Version()
		d.rightEditor.ContextMenu = func(menu *ui.Menu) {
			m.diffPaneContextMenu(menu, false)
		}
		d.rightEditor.OnInput = func(ui.InputEvent) bool {
			d.activeSide = editor.DiffNew
			return false
		}
	}
	d.leftEditor.ReadOnly = !d.leftEditable || d.applying
	d.rightEditor.ReadOnly = !d.rightEditable || d.applying
}

// refreshDiffEditorText reads buffer changes once per version, then rebuilds
// the navigation data. It returns true when either document changed.
func (d *diffView) refreshDiffEditorText() bool {
	if d == nil || d.leftEditor == nil || d.rightEditor == nil {
		return false
	}
	leftVersion := d.leftEditor.Buffer.Version()
	rightVersion := d.rightEditor.Buffer.Version()
	if leftVersion == d.leftEditorVersion && rightVersion == d.rightEditorVersion {
		return false
	}
	d.leftEditorVersion = leftVersion
	d.rightEditorVersion = rightVersion
	d.leftText = d.leftEditor.Buffer.String()
	d.rightText = d.rightEditor.Buffer.String()
	d.dirty = d.leftText != d.leftOriginal || d.rightText != d.rightOriginal
	d.setDiffRows(d.leftText, d.rightText)
	return true
}

func (d *diffView) replaceDiffSide(left bool, text string) {
	editorState := d.rightEditor
	if left {
		editorState = d.leftEditor
	}
	if editorState == nil {
		if left {
			d.leftText = text
		} else {
			d.rightText = text
		}
		d.dirty = d.leftText != d.leftOriginal || d.rightText != d.rightOriginal
		return
	}
	if editorState.Buffer.String() != text {
		editorState.Buffer.Edit(editor.Edit{
			Start: 0,
			End:   editorState.Buffer.Len(),
			Text:  text,
		})
	}
	if left {
		d.leftText = editorState.Buffer.String()
		d.leftEditorVersion = editorState.Buffer.Version()
	} else {
		d.rightText = editorState.Buffer.String()
		d.rightEditorVersion = editorState.Buffer.Version()
	}
	d.dirty = d.leftText != d.leftOriginal || d.rightText != d.rightOriginal
}

func (d *diffView) editorFor(left bool) *editor.State {
	if left {
		return d.leftEditor
	}
	return d.rightEditor
}

func diffLineOffsets(lines []string) []int {
	out := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		out[i] = offset
		offset += len(line) + 1
	}
	return out
}

// diffEditorTextBackgrounds highlights the words that changed within paired
// lines of each editable side. Unpaired added or deleted lines keep their
// whole-line tint, as IDEA does.
func diffEditorTextBackgrounds(changes []diffChange, left bool, leftText, rightText string, theme *ui.Theme, options DiffPrefs) []editor.TextBackground {
	if options.Highlight != "words" {
		return nil
	}
	leftLines := textdiff.SplitLines(leftText)
	rightLines := textdiff.SplitLines(rightText)
	leftOffsets := diffLineOffsets(leftLines)
	rightOffsets := diffLineOffsets(rightLines)
	out := make([]editor.TextBackground, 0, len(changes))
	for _, change := range changes {
		if change.oldStart < 0 || change.newStart < 0 ||
			change.oldStart > len(leftLines) || change.newStart > len(rightLines) {
			continue
		}
		oldEnd := min(change.oldEnd, len(leftLines))
		newEnd := min(change.newEnd, len(rightLines))
		paired := min(oldEnd-change.oldStart, newEnd-change.newStart)
		for i := 0; i < paired; i++ {
			oldLine := leftLines[change.oldStart+i]
			newLine := rightLines[change.newStart+i]
			oldSpans, newSpans := linediff.Words(oldLine, newLine)
			spans, base := newSpans, rightOffsets[change.newStart+i]
			color := theme.Background.Mix(theme.Success, 0.28)
			if left {
				spans, base = oldSpans, leftOffsets[change.oldStart+i]
				color = theme.Background.Mix(theme.Danger, 0.28)
			}
			for _, span := range spans {
				if span.End <= span.Start {
					continue
				}
				out = append(out, editor.TextBackground{
					Start: base + span.Start,
					End:   base + span.End,
					Color: color,
				})
			}
		}
	}
	return out
}

// editorLineBackgrounds maps diff changes onto the full editable buffers.
// Unlike the custom renderer these panes show the complete files, so edits and
// selection behave like the main editor.
func diffEditorLineBackgrounds(changes []diffChange, left bool, theme *ui.Theme, options DiffPrefs) []editor.LineBackground {
	if options.Highlight != "lines" && options.Highlight != "words" {
		return nil
	}
	out := make([]editor.LineBackground, 0, len(changes))
	for _, change := range changes {
		start, end := change.newStart, change.newEnd
		color := theme.Background.Mix(theme.Success, 0.14)
		if left {
			start, end = change.oldStart, change.oldEnd
			color = theme.Background.Mix(theme.Danger, 0.14)
		}
		if start < 0 {
			continue
		}
		if end <= start {
			end = start + 1
		}
		out = append(out, editor.LineBackground{
			First: start,
			Last:  end - 1,
			Color: color,
		})
	}
	return out
}

func (d *diffView) applyEditorOptions(softWrap bool, theme *ui.Theme, options DiffPrefs) {
	if d == nil || d.leftEditor == nil || d.rightEditor == nil {
		return
	}
	d.leftEditor.SoftWrap = softWrap
	d.rightEditor.SoftWrap = softWrap
	d.leftEditor.ReadOnly = !d.leftEditable || d.applying
	d.rightEditor.ReadOnly = !d.rightEditable || d.applying
	d.leftEditor.LineBackgrounds = diffEditorLineBackgrounds(d.changes, true, theme, options)
	d.rightEditor.LineBackgrounds = diffEditorLineBackgrounds(d.changes, false, theme, options)
	d.leftEditor.TextBackgrounds = diffEditorTextBackgrounds(
		d.changes, true, d.leftText, d.rightText, theme, options,
	)
	d.rightEditor.TextBackgrounds = diffEditorTextBackgrounds(
		d.changes, false, d.leftText, d.rightText, theme, options,
	)
	d.leftEditor.DiffBase = d.rightEditor.Buffer
	d.rightEditor.DiffBase = d.leftEditor.Buffer
}

func diffEditorAnchorLine(state *editor.State, y float64) int {
	if state == nil {
		return 0
	}
	lines := state.Buffer.LineCount()
	if lines <= 1 {
		return 0
	}
	low, high := 0, lines
	for low < high {
		mid := low + (high-low)/2
		if state.LineTop(mid) <= y {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return max(0, min(low-1, lines-1))
}

func mapDiffScrollLine(line int, leftToRight bool, changes []diffChange, targetLines int) int {
	if targetLines <= 0 {
		return 0
	}
	shift := 0
	for _, change := range changes {
		sourceStart, sourceEnd := change.oldStart, change.oldEnd
		targetStart, targetEnd := change.newStart, change.newEnd
		if !leftToRight {
			sourceStart, sourceEnd = change.newStart, change.newEnd
			targetStart, targetEnd = change.oldStart, change.oldEnd
		}
		if sourceStart < 0 || targetStart < 0 {
			continue
		}
		if line < sourceStart {
			break
		}
		sourceLength := max(0, sourceEnd-sourceStart)
		targetLength := max(0, targetEnd-targetStart)
		if line < sourceEnd || sourceLength == 0 && line == sourceStart {
			offset := min(max(0, line-sourceStart), max(0, targetLength-1))
			return min(targetLines-1, targetStart+offset)
		}
		shift += targetLength - sourceLength
	}
	return min(targetLines-1, max(0, line+shift))
}

func (d *diffView) syncEditorScroll(options DiffPrefs) {
	if d == nil || d.leftEditor == nil || d.rightEditor == nil ||
		options.Mode == "unified" || !prefBool(options.SyncScroll, true) {
		if d != nil && d.leftEditor != nil && d.rightEditor != nil {
			d.lastEditorLeftY = d.leftEditor.ScrollY()
			d.lastEditorRightY = d.rightEditor.ScrollY()
			d.editorScrollReady = true
			d.editorScrollSyncing = false
		}
		return
	}
	leftY, rightY := d.leftEditor.ScrollY(), d.rightEditor.ScrollY()
	if !d.editorScrollReady {
		d.lastEditorLeftY, d.lastEditorRightY = leftY, rightY
		d.editorScrollReady = true
		return
	}
	if d.editorScrollSyncing {
		d.lastEditorLeftY, d.lastEditorRightY = leftY, rightY
		d.editorScrollSyncing = false
		return
	}
	const epsilon = 0.5
	leftMoved := leftY < d.lastEditorLeftY-epsilon || leftY > d.lastEditorLeftY+epsilon
	rightMoved := rightY < d.lastEditorRightY-epsilon || rightY > d.lastEditorRightY+epsilon
	switch {
	case leftMoved && !rightMoved:
		line := diffEditorAnchorLine(d.leftEditor, leftY)
		target := mapDiffScrollLine(line, true, d.changes, d.rightEditor.Buffer.LineCount())
		d.rightEditor.SetScrollY(d.rightEditor.LineTop(target))
		d.editorScrollSyncing = true
	case rightMoved && !leftMoved:
		line := diffEditorAnchorLine(d.rightEditor, rightY)
		target := mapDiffScrollLine(line, false, d.changes, d.leftEditor.Buffer.LineCount())
		d.leftEditor.SetScrollY(d.leftEditor.LineTop(target))
		d.editorScrollSyncing = true
	}
	d.lastEditorLeftY, d.lastEditorRightY = leftY, rightY
}

// syncDiffScroll transfers a user scroll from the pane that moved to the
// other one. The second ScrollState is also the source when the user moves the
// right pane. Max values come from the previous layout, so the transfer keeps
// the same relative place in panes with different content heights.
func (d *diffView) syncDiffScroll() {
	options := d.options.normalized()
	if !d.scrollReady {
		d.lastUnifiedX, d.lastUnifiedY = d.unifiedScroll.X, d.unifiedScroll.Y
		d.lastLeftX, d.lastLeftY = d.leftScroll.X, d.leftScroll.Y
		d.lastRightX, d.lastRightY = d.rightScroll.X, d.rightScroll.Y
		d.scrollReady = true
		return
	}
	if options.Mode == "unified" || !prefBool(options.SyncScroll, true) {
		d.lastUnifiedX, d.lastUnifiedY = d.unifiedScroll.X, d.unifiedScroll.Y
		d.lastLeftX, d.lastLeftY = d.leftScroll.X, d.leftScroll.Y
		d.lastRightX, d.lastRightY = d.rightScroll.X, d.rightScroll.Y
		return
	}
	leftMoved := d.leftScroll.X != d.lastLeftX || d.leftScroll.Y != d.lastLeftY
	rightMoved := d.rightScroll.X != d.lastRightX || d.rightScroll.Y != d.lastRightY
	switch {
	case leftMoved && !rightMoved:
		d.rightScroll.X = transferDiffOffset(d.leftScroll.X, d.leftScroll.MaxX, d.rightScroll.MaxX)
		d.rightScroll.Y = transferDiffOffset(d.leftScroll.Y, d.leftScroll.MaxY, d.rightScroll.MaxY)
	case rightMoved && !leftMoved:
		d.leftScroll.X = transferDiffOffset(d.rightScroll.X, d.rightScroll.MaxX, d.leftScroll.MaxX)
		d.leftScroll.Y = transferDiffOffset(d.rightScroll.Y, d.rightScroll.MaxY, d.leftScroll.MaxY)
	}
	d.lastUnifiedX, d.lastUnifiedY = d.unifiedScroll.X, d.unifiedScroll.Y
	d.lastLeftX, d.lastLeftY = d.leftScroll.X, d.leftScroll.Y
	d.lastRightX, d.lastRightY = d.rightScroll.X, d.rightScroll.Y
}

func transferDiffOffset(value, fromMax, toMax float32) float32 {
	if toMax <= 0 {
		return 0
	}
	if fromMax <= 0 {
		if value < 0 {
			return 0
		}
		if value > toMax {
			return toMax
		}
		return value
	}
	next := value / fromMax * toMax
	if next < 0 {
		return 0
	}
	if next > toMax {
		return toMax
	}
	return next
}

// ShowDiff opens the compare view for a file against its committed version.
func (m *Model) ShowDiff(path string) {
	if path == "" || m.Root() == "" {
		return
	}
	path = realPath(path)
	// The buffer, not the saved file, is what the user is looking at.
	cur, curVersion, fromTab := m.bufferTextVersion(path)
	m.diff = &diffView{
		open: true, path: path, rightPath: path, loading: true,
		rightLabel: m.diffPathLabel(path), rightDocumentVersion: curVersion,
		rightVersionKnown: fromTab,
		options:           m.diffPrefs(), rightEditable: true,
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_file_at", map[string]any{"path": path})
		var base *string
		if err == nil {
			err = json.Unmarshal(raw, &base)
		}
		old := ""
		if base != nil {
			old = *base
		}
		if err == nil && !fromTab {
			var data []byte
			if data, err = os.ReadFile(path); err == nil {
				cur = string(data)
			}
		}
		m.post(func() {
			d := m.diff
			if d == nil || d.path != path {
				return
			}
			d.loading = false
			if err != nil {
				d.err = diffErrorText(err)
				m.invalidate()
				return
			}
			d.err = ""
			d.leftText, d.rightText = old, cur
			d.resetDiffEditors()
			d.setDiffRows(old, cur)
			if d.added == 0 && d.gone == 0 {
				d.message = "Identical to HEAD"
				m.invalidate()
				return
			}
			m.invalidate()
		})
	}()
}

// ShowFileDiff opens the compare view on two files, a missing side counting as
// empty text.
func (m *Model) ShowFileDiff(left, right string) {
	if left == "" && right == "" {
		return
	}
	left, right = realPath(left), realPath(right)
	leftBuffered, leftVersion, leftOpen := m.bufferTextVersion(left)
	rightBuffered, rightVersion, rightOpen := m.bufferTextVersion(right)
	d := &diffView{
		open: true, path: right, other: left, leftPath: left, rightPath: right,
		leftLabel: m.diffPathLabel(left), rightLabel: m.diffPathLabel(right), loading: true,
		leftDocumentVersion: leftVersion, rightDocumentVersion: rightVersion,
		leftVersionKnown: leftOpen, rightVersionKnown: rightOpen,
		options: m.diffPrefs(), leftEditable: true, rightEditable: true,
	}
	m.diff = d
	m.invalidate()
	go func() {
		var err error
		read := func(p string) string {
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				if !os.IsNotExist(rerr) {
					err = rerr
				}
				return ""
			}
			return string(data)
		}
		old, cur := "", ""
		if leftOpen {
			old = leftBuffered
		} else {
			old = read(left)
		}
		if err == nil {
			if rightOpen {
				cur = rightBuffered
			} else {
				cur = read(right)
			}
		}
		if err != nil {
			m.post(func() {
				if m.diff != d {
					return
				}
				d.loading = false
				d.err = "Reading the file failed: " + err.Error()
				m.invalidate()
			})
			return
		}
		m.finishCompare(d, old, cur, "The two files are identical")
	}()
}

// CompareTabWithFile opens the compare view between t's text and another file,
// as OpenIDEA's Compare With… does: the chosen file on the left, the tab on
// the right, unsaved text included.
func (m *Model) CompareTabWithFile(t *Tab, picked string) {
	if t == nil || t.doc == nil || picked == "" {
		return
	}
	path := t.doc.Path()
	picked = realPath(picked)
	if picked == path {
		return
	}
	d := &diffView{
		open: true, path: path, other: picked, leftPath: picked, rightPath: path,
		leftLabel: m.diffPathLabel(picked), rightLabel: m.diffPathLabel(path), loading: true,
		rightDocumentVersion: t.doc.Buffer().Version(), rightVersionKnown: true,
		options: m.diffPrefs(), leftEditable: true, rightEditable: true,
	}
	m.diff = d
	m.invalidate()
	cur := t.doc.Text()
	pickedText, pickedVersion, pickedOpen := m.bufferTextVersion(picked)
	d.leftDocumentVersion = pickedVersion
	d.leftVersionKnown = pickedOpen
	go func() {
		left := pickedText
		var err error
		if !pickedOpen {
			var data []byte
			data, err = os.ReadFile(picked)
			left = string(data)
		}
		if err != nil {
			m.post(func() {
				if m.diff != d {
					return
				}
				d.loading = false
				d.err = "Reading the file failed: " + err.Error()
				m.invalidate()
			})
			return
		}
		m.finishCompare(d, left, cur, "The two files are identical")
	}()
}

// PickCompareFile asks which file to compare t against, as OpenIDEA's
// Compare With… does, and opens the two-file compare.
func (m *Model) PickCompareFile(t *Tab) {
	if t == nil || t.doc == nil {
		return
	}
	parent := m.win
	start := filepath.Dir(t.doc.Path())
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Compare With", ButtonLabel: "Compare",
			DefaultPath: start,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() { m.CompareTabWithFile(t, paths[0]) })
	}()
}

// CompareTabWithClipboard opens the compare view between t's text and the
// clipboard, as OpenIDEA's Compare with Clipboard does.
func (m *Model) CompareTabWithClipboard(t *Tab) {
	if t == nil || t.doc == nil {
		return
	}
	data, err := mygo.Clipboard.Read(transfer.Text)
	var raw []byte
	if err == nil {
		raw, err = data.Read(transfer.Text)
	}
	if err != nil {
		m.status = "The clipboard has no text"
		m.invalidate()
		return
	}
	m.compareTabWithClipboardText(t, string(raw))
}

func (m *Model) compareTabWithClipboardText(t *Tab, clipboard string) {
	if t == nil || t.doc == nil {
		return
	}
	var selection string
	if t.state != nil {
		selection = t.state.SelectedText()
	}
	if selection != "" {
		d := &diffView{
			open: true, path: t.doc.Path(), inline: true,
			leftLabel: "Clipboard", rightLabel: "Selection",
			options: m.diffPrefs(),
		}
		d.label = "Clipboard ↔ " + m.diffPathLabel(t.doc.Path()) + " (selection)"
		m.diff = d
		m.finishCompare(d, clipboard, selection, "The clipboard matches the selection")
		return
	}
	m.showTextCompare(
		t.doc.Path(), clipboard, "Clipboard", t.doc.Text(),
		t.doc.Buffer().Version(), true,
	)
}

// ShowTextCompare opens the compare view on a tab's text and a text that is
// not a file; label names the left-hand side and the tab names the right one.
func (m *Model) ShowTextCompare(path, left, leftLabel, right string) {
	m.showTextCompare(path, left, leftLabel, right, 0, false)
}

func (m *Model) showTextCompare(
	path, left, leftLabel, right string,
	rightVersion uint64, rightVersionKnown bool,
) {
	if path == "" {
		return
	}
	d := &diffView{
		open: true, path: path, rightPath: path, inline: true,
		leftLabel: leftLabel, rightLabel: m.diffPathLabel(path), loading: true,
		rightDocumentVersion: rightVersion, rightVersionKnown: rightVersionKnown,
		options: m.diffPrefs(), rightEditable: true,
	}
	m.diff = d
	m.invalidate()
	go func() {
		m.finishCompare(d, left, right, leftLabel+" is identical to the current file")
	}()
}

// finishCompare posts a computed comparison unless another one replaced it.
func (m *Model) finishCompare(d *diffView, left, right, same string) {
	m.post(func() {
		if m.diff != d {
			return
		}
		d.leftText, d.rightText = left, right
		d.resetDiffEditors()
		d.setDiffRows(left, right)
		d.loading = false
		if d.added == 0 && d.gone == 0 {
			d.message = same
			m.invalidate()
			return
		}
		m.invalidate()
	})
}

// bufferText returns the text the user sees for path: an open tab's buffer
// wins over the file on disk.
func (m *Model) bufferText(path string) (string, bool) {
	text, _, ok := m.bufferTextVersion(path)
	return text, ok
}

func (m *Model) bufferTextVersion(path string) (string, uint64, bool) {
	for _, t := range m.tabs {
		if t.doc.Path() == path {
			return t.doc.Text(), t.doc.Buffer().Version(), true
		}
	}
	return "", 0, false
}

func (m *Model) diffPathLabel(path string) string {
	if path == "" {
		return ""
	}
	return filepath.ToSlash(projectRelative(m.Root(), path))
}

// CompareActive compares the active file with its committed version.
func (m *Model) CompareActive() {
	if t := m.tab(); t != nil {
		m.ShowDiff(t.doc.Path())
	}
}

// closeDiff closes an unchanged compare after an internal navigation action.
// A staged result goes through the same save/apply/discard prompt as the
// modal's close control.
func (m *Model) closeDiff() {
	if m.diff == nil {
		return
	}
	m.diff.refreshDiffEditorText()
	if m.diff.dirty {
		m.RequestCloseDiff()
		return
	}
	m.closeDiffNow()
}

func (m *Model) closeDiffNow() {
	if m.diff == nil {
		return
	}
	m.diff = nil
	m.invalidate()
}

// diffErrorText turns the backend's refusal for a project without Git into
// the sentence the compare view shows.
func diffErrorText(err error) string {
	var oe *openidea.Error
	if errors.As(err, &oe) && oe.Kind == "unavailable" && strings.Contains(oe.Message, "not a git repository") {
		return "The current project is not a Git repository"
	}
	return "Could not read the diff: " + err.Error()
}

// diffDialog draws the compare view: the file's diff against HEAD.
func (m *Model) diffDialog(c *ui.Context, p *ui.Theme) {
	d := m.diff
	if d == nil {
		return
	}
	open := d.open
	dlg := nativeModal(c, &open, func() {
		ui.Column(c).Width(1080).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, diffTitle(d)).FontSize(13).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				if len(d.agentChain) > 1 || len(d.fileChain) > 1 {
					index, count := d.agentIndex, len(d.agentChain)
					if len(d.fileChain) > count {
						index, count = d.fileIndex, len(d.fileChain)
					}
					ui.Button(c, "‹ Previous File").Disabled(index <= 0).OnClick(func() { m.StepDiffFile(-1) })
					ui.Text(c, fmt.Sprintf("%d of %d", index+1, count)).
						FontSize(11).TextColor(p.TextMuted).SingleLine()
					ui.Button(c, "Next File ›").Disabled(index >= count-1).OnClick(func() { m.StepDiffFile(1) })
				}
				if !d.loading && d.err == "" && d.message == "" {
					ui.Text(c, fmt.Sprintf("+%d  −%d", d.added, d.gone)).
						FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
			})
			if !d.loading && d.err == "" {
				diffToolbar(c, m, p)
				if d.settingsOpen {
					diffSettingsPanel(c, m, p)
				}
			}
			leftName, rightName := diffBanner(d)
			ui.Row(c).Gap(1).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Padding(5, 8).Background(p.Surface).
					BorderWidth(1).BorderColor(p.Border).Children(func() {
					ui.Text(c, leftName).FontSize(12).Bold().TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
				})
				ui.Column(c).Grow(1).MinWidth(0).Padding(5, 8).Background(p.Surface).
					BorderWidth(1).BorderColor(p.Border).Children(func() {
					ui.Text(c, rightName).FontSize(12).Bold().TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
				})
			})
			if d.path != "" {
				if rel, err := filepath.Rel(m.Root(), d.path); err == nil && !strings.HasPrefix(rel, "..") {
					ui.Text(c, filepath.ToSlash(rel)).FontSize(12).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
				}
			}
			switch {
			case d.err != "":
				ui.Text(c, d.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			case d.message != "":
				ui.Text(c, d.message).FontSize(12).TextColor(p.TextMuted).SingleLine()
			}
			if !d.loading && d.err == "" && (d.hasEditablePanes() || len(d.rows) > 0) {
				options := d.options.normalized()
				d.syncDiffScroll()
				m.renderDiffRows(c, p, options)
			}
		})
	})
	if dlg.Dismissed() || !open {
		m.RequestCloseDiff()
	}
}

// renderDiffRows draws the unified source, or two independent side-by-side
// sources. Aligned split panes keep empty spacers where the other side has an
// extra line. Unaligned panes omit those rows, so each side scrolls its own
// document as IDEA's diff does.
func (m *Model) renderDiffRows(c *ui.Context, p *ui.Theme, options DiffPrefs) {
	d := m.diff
	if d == nil {
		return
	}
	m.ensureDiffEditors(d)
	if d.leftEditor != nil && d.rightEditor != nil {
		m.renderEditableDiffRows(c, p, options)
		return
	}
	softWrap := m.diffUsesSoftWrap(d, options)
	if options.Mode == "unified" {
		scroll := ui.Scroll(c)
		if !softWrap {
			scroll = ui.ScrollBoth(c)
		}
		scroll.Height(510).TrackScroll(&d.unifiedScroll).Children(func() {
			ui.Column(c).Children(func() {
				for i, row := range d.rows {
					m.renderDiffRow(c, p, diffPaneRow{source: i, row: row, present: true},
						options, softWrap, false, false)
				}
			})
		})
		return
	}
	leftRows := diffRowsForPane(d.rows, true, options.AlignChanges)
	rightRows := diffRowsForPane(d.rows, false, options.AlignChanges)
	ui.Row(c).Gap(1).Children(func() {
		m.renderDiffPane(c, p, true, leftRows, &d.leftScroll, options, softWrap)
		m.renderDiffPane(c, p, false, rightRows, &d.rightScroll, options, softWrap)
	})
}

func (m *Model) renderEditableDiffRows(c *ui.Context, p *ui.Theme, options DiffPrefs) {
	d := m.diff
	if d == nil {
		return
	}
	d.refreshDiffEditorText()
	m.ensureDiffEditors(d)
	softWrap := m.diffUsesSoftWrap(d, options)
	d.applyEditorOptions(softWrap, p, options)
	if options.Mode == "unified" {
		ui.Row(c).Height(diffEditorHeight).MinWidth(0).Children(func() {
			view := editor.View(c, d.rightEditor).Grow(1).Height(diffEditorHeight).MinWidth(0)
			if view.FocusWithin() {
				d.activeSide = editor.DiffNew
			}
			overviewLane(c, p, "diff-overview-unified", "Diff overview unified",
				d.rightEditor, d.rightText, diffOverviewMarks(d, false, true), diffEditorHeight)
		})
		d.syncEditorScroll(options)
		return
	}
	sharedOverview := options.AlignChanges
	ui.Row(c).Gap(1).Height(diffEditorHeight).Children(func() {
		m.renderDiffEditorPane(c, p, d, true, !sharedOverview)
		ui.Box(c).Width(1).FillHeight().Background(p.Border)
		m.renderDiffEditorPane(c, p, d, false, true)
	})
	d.syncEditorScroll(options)
}

func (m *Model) renderDiffEditorPane(
	c *ui.Context,
	p *ui.Theme,
	d *diffView,
	left, showOverview bool,
) {
	ui.Row(c).Grow(1).Basis(0).Height(diffEditorHeight).MinWidth(0).Children(func() {
		state := d.rightEditor
		key, label := "diff-overview-right", "Diff overview right"
		if left {
			state = d.leftEditor
			key, label = "diff-overview-left", "Diff overview left"
		}
		view := editor.View(c, state).Grow(1).Height(diffEditorHeight).MinWidth(0)
		if view.FocusWithin() {
			if left {
				d.activeSide = editor.DiffOld
			} else {
				d.activeSide = editor.DiffNew
			}
		}
		if showOverview {
			text := d.rightText
			if left {
				text = d.leftText
			}
			overviewLane(c, p, key, label, state, text,
				diffOverviewMarks(d, left, false), diffEditorHeight)
		}
	})
}

func (m *Model) renderDiffPane(c *ui.Context, p *ui.Theme, left bool, rows []diffPaneRow,
	state *ui.ScrollState, options DiffPrefs, softWrap bool) {
	scroll := ui.Scroll(c)
	if !softWrap {
		scroll = ui.ScrollBoth(c)
	}
	scroll.Grow(1).Height(510).MinWidth(0).TrackScroll(state).Children(func() {
		ui.Column(c).Children(func() {
			for _, row := range rows {
				m.renderDiffRow(c, p, row, options, softWrap, true, left)
			}
		})
	})
}

func (m *Model) renderDiffRow(c *ui.Context, p *ui.Theme, item diffPaneRow,
	options DiffPrefs, softWrap, split, left bool) {
	d := m.diff
	if d == nil {
		return
	}
	row := ui.Row(c.Key(fmt.Sprintf("diff:%t:%d", options.Mode == "unified", item.source))).
		MinHeight(22).AlignItems(ui.Center)
	selected := diffRowSelected(d, item.source)
	if selected {
		row.Background(p.Background.Mix(p.Accent, 0.18))
	} else if options.Highlight != "none" && item.present {
		switch item.row.Kind {
		case textdiff.Add, textdiff.Delete:
			row.Background(diffRowTint(p, item.row.Kind))
		case textdiff.Hunk:
			row.Background(p.Background.Mix(p.Accent, 0.08))
		}
	}
	row.Children(func() {
		if item.row.Kind == textdiff.Hunk {
			ui.Row(c).Padding(0, 8).Children(func() {
				text := ui.Text(c, item.row.Text).FontSize(13).Font("monospace").TextColor(p.Accent)
				if softWrap {
					text.Grow(1).MinWidth(0)
				} else {
					text.NoWrap()
				}
			})
			return
		}
		if !split {
			diffUnifiedLine(c, p, item.row, options.Highlight, softWrap)
			return
		}
		line, changed := item.row.Old, item.row.Kind == textdiff.Delete
		if !left {
			line, changed = item.row.New, item.row.Kind == textdiff.Add
		}
		if !item.present {
			line = 0
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			text := ""
			if item.present {
				text = item.row.Text
			}
			diffPaneLine(c, p, line, text, item.present,
				changed && options.Highlight != "none", softWrap)
		})
	})
	if row.Clicked() {
		m.diffSelectRow(item.source)
	}
}

func diffRowTint(p *ui.Theme, kind textdiff.Kind) ui.Color {
	if kind == textdiff.Add {
		return p.Background.Mix(p.Success, 0.14)
	}
	return p.Background.Mix(p.Danger, 0.14)
}

// diffTitle names the compare view in its header.
func diffTitle(d *diffView) string {
	name := filepath.Base(d.path)
	switch {
	case d.loading:
		return name + " · Comparing…"
	case d.leftLabel != "" || d.rightLabel != "":
		left, right := d.leftLabel, d.rightLabel
		if left == "" {
			left = "Empty"
		}
		if right == "" {
			right = "Empty"
		}
		if d.inline || d.leftPath != "" || d.rightPath != "" {
			return left + " ↔ " + right
		}
		return name + " ↔ " + left + " / " + right
	case d.inline:
		return name + " ↔ " + d.label
	case d.other != "":
		return name + " ↔ " + filepath.Base(d.other)
	}
	return name + " · Compared with HEAD"
}

func diffSign(k textdiff.Kind) string {
	switch k {
	case textdiff.Add:
		return "+"
	case textdiff.Delete:
		return "−"
	}
	return " "
}

func diffColor(k textdiff.Kind, p *ui.Theme) ui.Color {
	switch k {
	case textdiff.Add:
		return p.Success
	case textdiff.Delete:
		return p.Danger
	}
	return p.Text
}

func lineNumber(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

// diffRowText is one row of the compare view as the smoke output records it.
func diffRowText(r textdiff.Row) string {
	return fmt.Sprintf("%s %d:%d %s", diffSign(r.Kind), r.Old, r.New, r.Text)
}

// SmokeDiff opens the compare view on a changed file and reports what it
// computed: the offscreen check of the diff view.
func (m *Model) SmokeDiff(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	if m.Root() == "" {
		return out, errors.New("No project is open")
	}
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
	if err != nil {
		return out, err
	}
	var files []openidea.FileStatus
	if err := json.Unmarshal(raw, &files); err != nil {
		return out, err
	}
	var target *openidea.FileStatus
	// An untracked file has no base to compare against (its diff view is
	// empty), so a tracked modification makes the better smoke target.
	for i := range files {
		if files[i].Unstaged != nil && *files[i].Unstaged != "untracked" {
			target = &files[i]
			break
		}
	}
	if target == nil {
		for i := range files {
			if files[i].Unstaged != nil {
				target = &files[i]
				break
			}
		}
	}
	if target == nil && len(files) > 0 {
		target = &files[0]
	}
	if target == nil {
		return out, errors.New("There are no uncommitted changes to compare")
	}
	path := target.Path
	out["path"] = path
	mygo.RunOnMain(func() { m.ShowDiff(path) })
	deadline := time.Now().Add(60 * time.Second)
	for {
		var loading bool
		var errText, message string
		var rows, added, gone int
		var first []string
		mygo.RunOnMain(func() {
			d := m.diff
			if d == nil {
				loading = true
				return
			}
			loading = d.loading
			errText, message = d.err, d.message
			rows, added, gone = len(d.rows), d.added, d.gone
			for i, r := range d.rows {
				if i >= 3 {
					break
				}
				first = append(first, diffRowText(r))
			}
		})
		if !loading {
			out["rows"] = rows
			out["added"] = added
			out["removed"] = gone
			if errText != "" {
				out["error"] = errText
			}
			if message != "" {
				out["message"] = message
			}
			if len(first) > 0 {
				out["first"] = first
			}
			break
		}
		if time.Now().After(deadline) {
			out["error"] = "Comparing timed out"
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	mygo.RunOnMain(func() { m.closeDiff() })
	return out, nil
}
