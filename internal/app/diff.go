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

	"mayoi/internal/openidea"
	"mayoi/internal/textdiff"
)

// diffContext is how many unchanged lines the compare view keeps around a
// change, as IDEA's default does.
const diffContext = 3

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
	changes   []diffChange
	selected  int
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
	cur, fromTab := m.bufferText(path)
	m.diff = &diffView{
		open: true, path: path, rightPath: path, loading: true,
		options: m.diffPrefs(),
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
	d := &diffView{
		open: true, path: right, other: left, leftPath: left, rightPath: right,
		leftLabel: filepath.Base(left), rightLabel: filepath.Base(right), loading: true,
		options: m.diffPrefs(),
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
		old, cur := read(left), ""
		if err == nil {
			cur = read(right)
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
		leftLabel: filepath.Base(picked), rightLabel: filepath.Base(path), loading: true,
		options: m.diffPrefs(),
	}
	m.diff = d
	m.invalidate()
	cur := t.doc.Text()
	go func() {
		data, err := os.ReadFile(picked)
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
		m.finishCompare(d, string(data), cur, "The two files are identical")
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
	m.ShowTextCompare(t.doc.Path(), string(raw), "Clipboard", t.doc.Text())
}

// ShowTextCompare opens the compare view on a tab's text and a text that is
// not a file; label names the left-hand side and the tab names the right one.
func (m *Model) ShowTextCompare(path, left, leftLabel, right string) {
	if path == "" {
		return
	}
	d := &diffView{
		open: true, path: path, rightPath: path, inline: true,
		leftLabel: leftLabel, rightLabel: filepath.Base(path), loading: true,
		options: m.diffPrefs(),
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
	for _, t := range m.tabs {
		if t.doc.Path() == path {
			return t.doc.Text(), true
		}
	}
	return "", false
}

// CompareActive compares the active file with its committed version.
func (m *Model) CompareActive() {
	if t := m.tab(); t != nil {
		m.ShowDiff(t.doc.Path())
	}
}

// closeDiff closes the compare view.
func (m *Model) closeDiff() {
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
	dlg := ui.Modal(c, &open, func() {
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
			if len(d.rows) > 0 {
				options := d.options.normalized()
				d.syncDiffScroll()
				m.renderDiffRows(c, p, options)
			}
		})
	})
	if dlg.Dismissed() || !open {
		m.closeDiff()
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
