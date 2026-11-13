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
				h.err = "读取本地历史失败：" + err.Error()
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
				h.err = "恢复失败：" + err.Error()
				m.invalidate()
				return
			}
			h.err = ""
			m.refreshTree(filepath.Dir(e.Path))
			m.status = "已恢复 " + filepath.Base(e.Path) + " 的 " + historyTime(e.Ms) + " 版本"
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
		h.err = "标签不能为空"
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
				h.err = "标记失败：" + err.Error()
				m.invalidate()
				return
			}
			m.status = "已标记版本 " + label
			m.closeHistory()
		})
	}()
}

// historyTime is a snapshot's local time, to the minute.
func historyTime(ms float64) string {
	return time.UnixMilli(int64(ms)).Format("2006-01-02 15:04")
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
			title := "本地历史 · " + filepath.Base(h.path)
			if h.loading {
				title = "本地历史 · 正在读取…"
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
				ui.Text(c, "标签记下当前项目所有文件的状态。").FontSize(10.5).TextColor(p.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "取消").OnClick(func() { m.closeHistory() })
					ui.Button(c, "保存").Disabled(h.busy).OnClick(func() { m.ConfirmHistoryLabel() })
				})
				return
			}
			switch {
			case h.loading:
				ui.Text(c, "正在读取历史…").FontSize(11.5).TextColor(p.TextMuted)
			case len(h.entries) == 0:
				ui.Text(c, "还没有历史记录").FontSize(11.5).TextColor(p.TextMuted)
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
				ui.Button(c, "标记版本…").OnClick(func() {
					h.labeling = true
					h.label = ""
					h.err = ""
					h.confirmRestore = false
					m.invalidate()
				})
				ui.Spacer(c)
				if len(h.entries) > 0 && h.entries[h.sel].Kind != "label" {
					label := "恢复此版本"
					if h.confirmRestore {
						label = "确认恢复"
					}
					ui.Button(c, label).Disabled(h.busy).OnClick(func() { m.RestoreHistoryEntry() })
				}
				ui.Button(c, "关闭").OnClick(func() { m.closeHistory() })
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeHistory()
	}
}
