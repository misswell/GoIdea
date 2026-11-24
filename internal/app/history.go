package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/textdiff"
)

// historyEntry is one Local History row: a stored revision or a label.
type historyEntry struct {
	ID    string  `json:"id"`
	Path  string  `json:"path"`
	Ms    float64 `json:"timestampMs"`
	Label string  `json:"label"`
	Size  uint64  `json:"size"`
	Kind  string  `json:"kind"`
}

// directoryHistory is what history_list_dir returns: snapshots under a path,
// labels, and files whose history outlived them on disk.
type directoryHistory struct {
	Entries []historyEntry `json:"entries"`
	Deleted []string       `json:"deleted"`
}

// historyState is the Local History sheet of one file or folder.
type historyState struct {
	open    bool
	path    string // the file or folder whose history is shown
	loading bool
	err     string
	entries []historyEntry
	deleted []string
	sel     int
	folder  bool
	// extra holds the additional rows selected with Cmd/Ctrl or Shift. IDEA
	// compares the oldest and newest snapshots when they belong to one file.
	extra  map[int]bool
	anchor int
	// preview is the revision diff OpenIDEA shows beside the timeline.
	preview    historyPreview
	previewSeq int
	// confirmRestore arms the selected snapshot's restore for a second click.
	confirmRestore bool
	busy           bool
	// labeling asks for the name of a new history point.
	labeling bool
	label    string
}

type historyPreview struct {
	loading    bool
	err        string
	message    string
	leftLabel  string
	rightLabel string
	rows       []textdiff.Row
	added      int
	gone       int
}

// ShowHistory opens Local History for path, as Local History › Show History
// does.
func (m *Model) ShowHistory(path string) {
	if path == "" || m.Root() == "" {
		return
	}
	h := &historyState{open: true, path: realPath(path), loading: true}
	m.history = h
	m.invalidate()
	m.loadHistory(h)
}

// ShowHistorySelected shows the history of the project view's selection, or of
// the project root when nothing is selected.
func (m *Model) ShowHistorySelected() {
	path := m.Root()
	if m.tree != nil {
		if sel := m.tree.Selected(); sel != "" {
			path = sel
		}
	}
	m.ShowHistory(path)
}

// OpenHistoryLabel asks for the label of a new history point, as Local History
// › Put Label… does.
func (m *Model) OpenHistoryLabel() {
	m.history = &historyState{open: true, path: m.Root(), labeling: true}
	m.invalidate()
}

// closeHistory closes the sheet.
func (m *Model) closeHistory() {
	if m.history == nil {
		return
	}
	m.history = nil
	m.invalidate()
}

// loadHistory rereads the sheet's entries from the backend.
func (m *Model) loadHistory(h *historyState) {
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		var direct []historyEntry
		if info, statErr := os.Stat(h.path); statErr == nil && !info.IsDir() {
			raw, listErr := m.invoke(ctx, "history_list", map[string]any{"path": h.path})
			if listErr == nil {
				listErr = json.Unmarshal(raw, &direct)
			}
			if listErr != nil {
				direct = nil
			}
		}
		raw, err := m.invoke(ctx, "history_list_dir", map[string]any{"path": h.path})
		var dir directoryHistory
		if err == nil {
			err = json.Unmarshal(raw, &dir)
		}
		m.post(func() {
			if m.history != h {
				return
			}
			h.loading = false
			if err != nil {
				h.err = "Could not read the local history: " + err.Error()
				m.invalidate()
				return
			}
			h.entries, h.deleted = mergeHistoryEntries(dir.Entries, direct), dir.Deleted
			sortHistoryNewestFirst(h.entries)
			h.folder = len(h.deleted) > 0
			for _, e := range h.entries {
				if e.Kind != "label" && filepath.Clean(e.Path) != filepath.Clean(h.path) {
					h.folder = true
					break
				}
			}
			h.extra = map[int]bool{}
			h.anchor = 0
			if h.sel >= len(h.entries) {
				h.sel = 0
			}
			m.refreshHistoryPreview(h)
			m.invalidate()
		})
	}()
}

func mergeHistoryEntries(groups ...[]historyEntry) []historyEntry {
	seen := map[string]bool{}
	var out []historyEntry
	for _, entries := range groups {
		for _, entry := range entries {
			key := entry.Kind + "\x00" + entry.ID + "\x00" + entry.Path + "\x00" +
				fmt.Sprintf("%.0f", entry.Ms)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, entry)
		}
	}
	return out
}

func sortHistoryNewestFirst(entries []historyEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Ms > entries[j-1].Ms; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// RestoreHistoryEntry puts the selected snapshot back: the first call arms the
// action, the second restores, since the write replaces the file's text.
func (m *Model) RestoreHistoryEntry() {
	h := m.history
	if h == nil || h.busy || h.sel < 0 || h.sel >= len(h.entries) {
		return
	}
	e := h.entries[h.sel]
	if e.Kind == "label" || e.ID == "" {
		return
	}
	if !h.confirmRestore {
		h.confirmRestore = true
		m.invalidate()
		return
	}
	h.confirmRestore, h.busy = false, true
	m.invalidate()
	var restoredDoc historyRestoreDocument
	var beforeText string
	var beforeVersion uint64
	if m.reg != nil {
		restoredDoc = m.reg.Get(e.Path)
		if restoredDoc != nil {
			beforeText = restoredDoc.Text()
			beforeVersion = restoredDoc.Buffer().Version()
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "history_restore", map[string]any{
			"path": e.Path, "id": e.ID,
		})
		var restoredText string
		var syncErr error
		if err == nil && restoredDoc != nil {
			var raw json.RawMessage
			raw, syncErr = m.invoke(ctx, "fs_read_text", map[string]any{"path": e.Path})
			if syncErr == nil {
				syncErr = json.Unmarshal(raw, &restoredText)
			}
			if syncErr == nil {
				syncErr = replaceHistoryDocumentText(
					ctx, restoredDoc, beforeVersion, beforeText, restoredText,
				)
			}
		}
		m.post(func() {
			if err != nil {
				if m.history == h {
					h.busy = false
					h.err = "Restoring failed: " + err.Error()
					m.invalidate()
				}
				return
			}
			if syncErr == nil {
				for _, tab := range m.allTabs() {
					if restoredDoc != nil && tab.doc == restoredDoc {
						tab.seen = restoredDoc.Buffer().Version()
						tab.noteDisk()
					}
				}
			}
			m.refreshTree(filepath.Dir(e.Path))
			if syncErr == nil {
				m.status = "Restored " + filepath.Base(e.Path) + " to the " +
					historyTime(m.prefs.dateFormatPrefs(), m.systemRegion, e.Ms) + " version"
			} else {
				m.banner = "The restored file changed while open; the editor's current text was kept"
			}
			if m.history == h {
				h.busy = false
				if syncErr != nil {
					h.err = "The file was restored, but its open editor could not be refreshed: " + syncErr.Error()
				} else {
					h.err = ""
				}
				m.loadHistory(h)
			}
			m.invalidate()
		})
	}()
}

type historyRestoreDocument interface {
	Text() string
	Buffer() *editor.Buffer
	ApplyEditsIfVersion(context.Context, uint64, ...editor.Edit) error
}

// replaceHistoryDocumentText mirrors OpenIDEA's in-place history restore for
// an editor document owned by GoIDEA. The version guard keeps text typed while
// the backend restore was running from being silently replaced.
func replaceHistoryDocumentText(
	ctx context.Context,
	doc historyRestoreDocument,
	expectedVersion uint64,
	beforeText string,
	restoredText string,
) error {
	if doc == nil {
		return nil
	}
	return doc.ApplyEditsIfVersion(ctx, expectedVersion, editor.Edit{
		Start: 0, End: len(beforeText), Text: restoredText,
	})
}

// ConfirmHistoryLabel records a label for the whole project.
func (m *Model) ConfirmHistoryLabel() {
	h := m.history
	if h == nil || !h.labeling || h.busy {
		return
	}
	label := strings.TrimSpace(h.label)
	if label == "" {
		h.err = "The label cannot be empty"
		m.invalidate()
		return
	}
	h.busy = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "history_put_label", map[string]any{"label": label})
		m.post(func() {
			if m.history != h {
				return
			}
			h.busy = false
			if err != nil {
				h.err = "Putting the label failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Put label " + label
			m.closeHistory()
		})
	}()
}

// historyTime is a snapshot's local time, following Date Formats.
func historyTime(prefs DateFormatPrefs, region dateRegion, ms float64) string {
	return formatAppTimestamp(prefs, region, ms, time.Now())
}

// snapshotBefore is LocalHistory.tsx's snapshotsBefore for one file: the
// newest snapshot recorded before the label's timestamp.
func snapshotBefore(entries []historyEntry, at float64, path string) *historyEntry {
	var best *historyEntry
	for i := range entries {
		e := &entries[i]
		if e.Kind == "label" || e.Ms >= at || e.Path != path {
			continue
		}
		if best == nil || best.Ms < e.Ms {
			best = e
		}
	}
	return best
}

// revisionRange returns the oldest and newest snapshots selected by index.
// Entries are newest first, so the largest index is the older revision. As in
// OpenIDEA, a comparison is available only for snapshots of one file.
func revisionRange(entries []historyEntry, selected map[int]bool) (*historyEntry, *historyEntry) {
	var picked []int
	for i := range entries {
		if selected != nil && selected[i] {
			picked = append(picked, i)
		}
	}
	if len(picked) < 2 {
		return nil, nil
	}
	path := ""
	for _, i := range picked {
		if entries[i].Kind == "label" {
			return nil, nil
		}
		if path == "" {
			path = entries[i].Path
		} else if entries[i].Path != path {
			return nil, nil
		}
	}
	older, newer := picked[0], picked[0]
	for _, i := range picked[1:] {
		if i > older {
			older = i
		}
		if i < newer {
			newer = i
		}
	}
	return &entries[older], &entries[newer]
}

type historyPreviewPlan struct {
	leftID     string
	rightID    string
	leftLabel  string
	rightLabel string
	path       string
	current    bool
	deleted    bool
	message    string
}

func historyEntryLabel(prefs DateFormatPrefs, region dateRegion, entry historyEntry) string {
	return historyTime(prefs, region, entry.Ms) + " · " + entry.Label
}

// planHistoryPreview mirrors LocalHistory.tsx's detail choice: selected
// snapshots compare with each other, a snapshot compares with the worktree,
// and a label compares the last version before it with the worktree.
func (m *Model) planHistoryPreview(h *historyState) historyPreviewPlan {
	if h == nil || h.sel < 0 || h.sel >= len(h.entries) {
		return historyPreviewPlan{message: "Select a revision"}
	}
	selected := map[int]bool{h.sel: true}
	for i, on := range h.extra {
		if on {
			selected[i] = true
		}
	}
	if older, newer := revisionRange(h.entries, selected); older != nil {
		return historyPreviewPlan{
			leftID: older.ID, rightID: newer.ID,
			leftLabel:  historyEntryLabel(m.prefs.dateFormatPrefs(), m.systemRegion, *older),
			rightLabel: historyEntryLabel(m.prefs.dateFormatPrefs(), m.systemRegion, *newer),
			path:       newer.Path,
		}
	}
	if len(h.extra) > 0 {
		return historyPreviewPlan{message: "Select revisions of one file to compare them"}
	}

	current := h.entries[h.sel]
	if current.Kind != "label" {
		return historyPreviewPlan{
			leftID:    current.ID,
			leftLabel: historyEntryLabel(m.prefs.dateFormatPrefs(), m.systemRegion, current),
			path:      current.Path, current: true, deleted: historyPathDeleted(h, current.Path),
			rightLabel: "Current",
		}
	}

	at := current.Ms
	if h.folder {
		targets := snapshotsBefore(historyEntriesBefore(h.entries, at))
		message := "Label '" + current.Label + "'"
		if len(targets) > 0 {
			message += fmt.Sprintf(" · %s recorded before it", count(len(targets), "file", "files"))
		}
		return historyPreviewPlan{message: message}
	}
	if snap := snapshotBefore(h.entries, at, h.path); snap != nil {
		return historyPreviewPlan{
			leftID:    snap.ID,
			leftLabel: historyEntryLabel(m.prefs.dateFormatPrefs(), m.systemRegion, *snap),
			path:      h.path, current: true, deleted: historyPathDeleted(h, h.path),
			rightLabel: "Current",
		}
	}
	return historyPreviewPlan{message: "Nothing recorded before label '" + current.Label + "'"}
}

func historyEntriesBefore(entries []historyEntry, at float64) []historyEntry {
	out := make([]historyEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind != "label" && entry.Ms < at {
			out = append(out, entry)
		}
	}
	return out
}

func snapshotsBefore(entries []historyEntry) map[string]historyEntry {
	out := map[string]historyEntry{}
	for _, entry := range entries {
		prev, exists := out[entry.Path]
		if !exists || prev.Ms < entry.Ms {
			out[entry.Path] = entry
		}
	}
	return out
}

func historyPathDeleted(h *historyState, path string) bool {
	if h == nil {
		return false
	}
	for _, deleted := range h.deleted {
		if filepath.Clean(deleted) == filepath.Clean(path) {
			return true
		}
	}
	return false
}

func (m *Model) refreshHistoryPreview(h *historyState) {
	if h == nil {
		return
	}
	h.previewSeq++
	seq := h.previewSeq
	plan := m.planHistoryPreview(h)
	preview := historyPreview{message: plan.message, leftLabel: plan.leftLabel, rightLabel: plan.rightLabel}
	preview.loading = plan.leftID != "" && (plan.rightID != "" || plan.current)
	h.preview = preview
	if !preview.loading {
		m.invalidate()
		return
	}

	currentText := ""
	if plan.current {
		currentText = m.historyCurrentText(plan.path)
		if plan.deleted {
			currentText = ""
			preview.rightLabel = "Deleted"
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
		defer cancel()
		readContent := func(id string) (string, error) {
			if id == "" {
				return "", nil
			}
			raw, err := m.invoke(ctx, "history_content", map[string]any{"id": id})
			if err != nil {
				return "", err
			}
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return "", err
			}
			return text, nil
		}
		leftText, err := readContent(plan.leftID)
		rightText := currentText
		if err == nil && plan.rightID != "" {
			rightText, err = readContent(plan.rightID)
		}
		m.post(func() {
			if m.history != h || h.previewSeq != seq {
				return
			}
			h.preview.loading = false
			if err != nil {
				h.preview.err = "Could not read the revision: " + err.Error()
				m.invalidate()
				return
			}
			rows, added, gone := diffRows(leftText, rightText)
			h.preview.rows, h.preview.added, h.preview.gone = rows, added, gone
			if added == 0 && gone == 0 {
				h.preview.message = "The selected revisions are identical"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) historyCurrentText(path string) string {
	if text, ok := m.bufferText(path); ok {
		return text
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// RestoreHistoryLabelTime restores every file of the label's directory to
// its state at the label, the way LocalHistory.tsx restores a label-time
// selection (snapshotsBefore + history_restore per file).
func (m *Model) RestoreHistoryLabelTime() {
	h := m.history
	if h == nil {
		return
	}
	entries := h.entries
	var label *historyEntry
	if h.sel >= 0 && h.sel < len(entries) && entries[h.sel].Kind == "label" {
		label = &entries[h.sel]
	}
	if label == nil {
		return
	}
	at := label.Ms
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		var restored, failed int
		for i := range entries {
			e := entries[i]
			if e.Kind == "label" || e.Ms >= at {
				continue
			}
			snap := snapshotBefore(entries, at, e.Path)
			if snap == nil || snap.ID != e.ID {
				continue
			}
			if _, err := m.invoke(ctx, "history_restore", map[string]any{"path": e.Path, "id": e.ID}); err != nil {
				failed++
			} else {
				restored++
			}
		}
		m.post(func() {
			if failed > 0 {
				m.notifyWarning("local-history.restore", "Local History", fmt.Sprintf("Restored %d files; %d failed", restored, failed))
			} else if restored > 0 {
				m.notifyInfo("local-history.restore", "Local History", fmt.Sprintf("Restored %d files to the label's moment", restored))
			} else {
				m.notifyInfo("local-history.restore", "Local History", "The label's moment has no file snapshots to restore")
			}
			if m.history != nil {
				m.loadHistory(m.history)
			}
			m.invalidate()
		})
	}()
}

// historySize is a snapshot's stored size.
func historySize(n uint64) string {
	if n >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%d B", n)
}

// historyDialog draws the Local History sheet.
func (m *Model) historyDialog(c *ui.Context, p *ui.Theme) {
	h := m.history
	if h == nil {
		return
	}
	open := h.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(980).Gap(8).Children(func() {
			title := "Local History · " + filepath.Base(h.path)
			if h.loading {
				title = "Local History · Reading…"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if rel, err := filepath.Rel(m.Root(), h.path); err == nil && !strings.HasPrefix(rel, "..") {
				ui.Text(c, filepath.ToSlash(rel)).FontSize(12).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			if h.err != "" {
				ui.Text(c, h.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if h.labeling {
				in := ui.TextInput(c, &h.label).AutoFocus()
				in.OnChange(func() { h.err = "" })
				if in.Submitted() {
					m.ConfirmHistoryLabel()
				}
				ui.Text(c, "The label records the state of every file in the project.").FontSize(12).TextColor(p.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "Cancel").OnClick(func() { m.closeHistory() })
					ui.Button(c, "Save").Disabled(h.busy).OnClick(func() { m.ConfirmHistoryLabel() })
				})
				return
			}
			switch {
			case h.loading:
				ui.Text(c, "Reading the history…").FontSize(12).TextColor(p.TextMuted)
			case len(h.entries) == 0:
				ui.Text(c, "No history yet").FontSize(12).TextColor(p.TextMuted)
			default:
				ui.Row(c).Height(410).Gap(12).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Width(390).Height(410).Gap(4).Children(func() {
						ui.Text(c, "Revisions").FontSize(12).Bold().TextColor(p.Text)
						ui.Scroll(c).Grow(1).Children(func() {
							ui.Column(c).Children(func() {
								for i, e := range h.entries {
									i, e := i, e
									height := float32(38)
									if h.folder {
										height = 52
									}
									row := ui.Row(c.Key(fmt.Sprintf("history:%d", i))).Height(height).
										Padding(0, 8).Gap(8).AlignItems(ui.Center)
									switch {
									case i == h.sel:
										row.Background(p.Selection)
									case h.extra[i]:
										row.Background(p.SurfaceHover)
									case row.Hovered():
										row.Background(p.SurfaceHover)
									}
									row.Children(func() {
										ui.Text(c, historyTime(m.prefs.dateFormatPrefs(), c.Region(), e.Ms)).Width(118).Shrink(0).FontSize(12).
											TextColor(p.TextMuted).SingleLine()
										ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
											label := e.Label
											if e.Kind == "label" {
												label = "◆ " + label
											}
											ui.Text(c, label).FontSize(13).TextColor(p.Text).
												SingleLine().Ellipsis("…")
											if e.Kind != "label" && h.folder {
												rel := e.Path
												if relative, err := filepath.Rel(m.Root(), e.Path); err == nil {
													rel = relative
												}
												pathText := ui.Text(c, filepath.ToSlash(rel)).FontSize(12).
													TextColor(p.TextMuted).SingleLine().Ellipsis("…")
												if historyPathDeleted(h, e.Path) {
													pathText.TextColor(p.Danger).Strikethrough()
												}
											}
										})
										if e.Kind != "label" {
											ui.Text(c, historySize(e.Size)).FontSize(12).
												TextColor(p.TextMuted).SingleLine()
										}
									})
									if row.Clicked() {
										m.selectHistoryRow(h, i, row.ClickModifiers())
									}
								}
							})
						})
					})
					ui.Column(c).Grow(1).MinWidth(0).Height(410).Gap(4).Children(func() {
						m.historyPreviewPane(c, p, h)
					})
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Put Label…").OnClick(func() {
					h.labeling = true
					h.label = ""
					h.err = ""
					h.confirmRestore = false
					m.invalidate()
				})
				ui.Spacer(c)
				if len(h.entries) > 0 && h.entries[h.sel].Kind == "label" {
					// The label-time restore of upstream LocalHistory.tsx:
					// every file of this folder goes back to its newest
					// snapshot before the label.
					ui.Button(c, "Restore to the Label's Moment").Disabled(h.busy).OnClick(func() { m.RestoreHistoryLabelTime() })
				}
				if len(h.entries) > 0 && h.entries[h.sel].Kind != "label" {
					label := "Restore This Version"
					if historyPathDeleted(h, h.entries[h.sel].Path) {
						label = "Restore"
					}
					if h.confirmRestore {
						label = "Confirm Restore"
					}
					ui.Button(c, label).Disabled(h.busy).OnClick(func() { m.RestoreHistoryEntry() })
				}
				ui.Button(c, "Close").OnClick(func() { m.closeHistory() })
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeHistory()
	}
}

func (m *Model) selectHistoryRow(h *historyState, index int, mods ui.Modifiers) {
	if h == nil || index < 0 || index >= len(h.entries) {
		return
	}
	if h.extra == nil {
		h.extra = map[int]bool{}
	}
	old := h.sel
	switch {
	case mods&ui.Shift != 0:
		from, to := h.anchor, index
		if from > to {
			from, to = to, from
		}
		h.extra = map[int]bool{}
		for i := from; i <= to; i++ {
			if i != index {
				h.extra[i] = true
			}
		}
	case mods&(ui.Cmd|ui.Ctrl) != 0:
		if old != index && old >= 0 && old < len(h.entries) {
			h.extra[old] = true
		}
		h.anchor = index
	default:
		h.extra = map[int]bool{}
		h.anchor = index
	}
	h.sel = index
	h.confirmRestore = false
	m.refreshHistoryPreview(h)
}

func (m *Model) historyPreviewPane(c *ui.Context, p *ui.Theme, h *historyState) {
	switch {
	case h.preview.loading:
		ui.Text(c, "Reading the revision…").FontSize(12).TextColor(p.TextMuted)
		return
	case h.preview.err != "":
		ui.Text(c, h.preview.err).FontSize(12).TextColor(p.Danger)
		return
	case h.preview.message != "":
		ui.Text(c, h.preview.message).FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, h.preview.leftLabel).FontSize(11).TextColor(p.TextMuted).
			SingleLine().Ellipsis("…")
		ui.Text(c, "→").FontSize(11).TextColor(p.TextMuted)
		ui.Text(c, h.preview.rightLabel).FontSize(11).TextColor(p.TextMuted).
			SingleLine().Ellipsis("…")
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("+%d −%d", h.preview.added, h.preview.gone)).
			FontSize(11).TextColor(p.TextMuted)
	})
	ui.Column(c).Grow(1).MinWidth(0).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				for i, row := range h.preview.rows {
					if row.Kind == textdiff.Hunk {
						ui.Text(c, "⋯").Height(18).FontSize(11).TextColor(p.TextMuted)
						continue
					}
					line := ui.Row(c.Key(fmt.Sprintf("history-preview:%d", i))).Height(18).
						Gap(8).Padding(0, 4).AlignItems(ui.Center)
					if row.Kind == textdiff.Add || row.Kind == textdiff.Delete {
						line.Background(diffRowTint(p, row.Kind))
					}
					line.Children(func() {
						oldLine, newLine := "", ""
						if row.Old > 0 {
							oldLine = fmt.Sprintf("%d", row.Old)
						}
						if row.New > 0 {
							newLine = fmt.Sprintf("%d", row.New)
						}
						ui.Text(c, oldLine).Width(38).Shrink(0).FontSize(11).
							TextColor(p.TextMuted).SingleLine()
						ui.Text(c, newLine).Width(38).Shrink(0).FontSize(11).
							TextColor(p.TextMuted).SingleLine()
						ui.Text(c, diffSign(row.Kind)).Width(12).Shrink(0).FontSize(11).
							TextColor(diffColor(row.Kind, p)).SingleLine()
						ui.Text(c, row.Text).FontSize(12).TextColor(p.Text).
							SingleLine().Ellipsis("…")
					})
				}
			})
		})
	})
}
