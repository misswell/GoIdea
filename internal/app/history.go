package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
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
	// confirmRestore arms the selected snapshot's restore for a second click.
	confirmRestore bool
	busy           bool
	// labeling asks for the name of a new history point.
	labeling bool
	label    string
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
		raw, err := m.mgr.Invoke(ctx, Window, "history_list_dir", map[string]any{"path": h.path})
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
			h.entries, h.deleted = dir.Entries, dir.Deleted
			if h.sel >= len(h.entries) {
				h.sel = 0
			}
			m.invalidate()
		})
	}()
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
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "history_restore", map[string]any{
			"path": e.Path, "id": e.ID,
		})
		m.post(func() {
			if m.history != h {
				return
			}
			h.busy = false
			if err != nil {
				h.err = "Restoring failed: " + err.Error()
				m.invalidate()
				return
			}
			h.err = ""
			m.refreshTree(filepath.Dir(e.Path))
			m.status = "Restored " + filepath.Base(e.Path) + " to the " + historyTime(e.Ms) + " version"
			m.invalidate()
			m.loadHistory(h)
		})
	}()
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
		_, err := m.mgr.Invoke(ctx, Window, "history_put_label", map[string]any{"label": label})
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

// historyTime is a snapshot's local time, to the minute.
func historyTime(ms float64) string {
	return time.UnixMilli(int64(ms)).Format("2006-01-02 15:04")
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
			if _, err := m.mgr.Invoke(ctx, Window, "history_restore", map[string]any{"path": e.Path, "id": e.ID}); err != nil {
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
		ui.Column(c).Width(680).Gap(8).Children(func() {
			title := "Local History · " + filepath.Base(h.path)
			if h.loading {
				title = "Local History · Reading…"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if rel, err := filepath.Rel(m.Root(), h.path); err == nil && !strings.HasPrefix(rel, "..") {
				ui.Text(c, filepath.ToSlash(rel)).FontSize(10.5).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			if h.err != "" {
				ui.Text(c, h.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if h.labeling {
				in := ui.TextInput(c, &h.label).AutoFocus()
				in.OnChange(func() { h.err = "" })
				if in.Submitted() {
					m.ConfirmHistoryLabel()
				}
				ui.Text(c, "The label records the state of every file in the project.").FontSize(10.5).TextColor(p.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "Cancel").OnClick(func() { m.closeHistory() })
					ui.Button(c, "Save").Disabled(h.busy).OnClick(func() { m.ConfirmHistoryLabel() })
				})
				return
			}
			switch {
			case h.loading:
				ui.Text(c, "Reading the history…").FontSize(11.5).TextColor(p.TextMuted)
			case len(h.entries) == 0:
				ui.Text(c, "No history yet").FontSize(11.5).TextColor(p.TextMuted)
			default:
				ui.Scroll(c).Height(380).Children(func() {
					ui.Column(c).Children(func() {
						for i, e := range h.entries {
							i, e := i, e
							row := ui.Row(c.Key(fmt.Sprintf("history:%d", i))).Height(24).
								Padding(0, 8).Gap(8).AlignItems(ui.Center)
							if i == h.sel {
								row.Background(p.Selection)
							} else if row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Row(c).Width(118).Shrink(0).Children(func() {
									ui.Text(c, historyTime(e.Ms)).FontSize(10.5).
										TextColor(p.TextMuted).SingleLine()
								})
								ui.Text(c, e.Label).FontSize(11.5).TextColor(p.Text).
									SingleLine().Ellipsis("…")
								if e.Kind != "label" {
									if rel, err := filepath.Rel(m.Root(), e.Path); err == nil {
										ui.Spacer(c)
										ui.Text(c, filepath.ToSlash(rel)).FontSize(10.5).
											TextColor(p.TextMuted).SingleLine().Ellipsis("…")
										ui.Text(c, historySize(e.Size)).FontSize(10.5).
											TextColor(p.TextMuted).SingleLine()
									}
								}
							})
							if row.Clicked() {
								h.sel = i
								h.confirmRestore = false
								m.invalidate()
							}
						}
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
