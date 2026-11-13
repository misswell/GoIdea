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

// dirCompareEntry is one relative path of fs_compare_directories and how the
// two folders differ there.
type dirCompareEntry struct {
	Path      string `json:"path"`
	Status    string `json:"status"` // leftOnly, rightOnly, different, equal
	LeftKind  string `json:"leftKind"`
	RightKind string `json:"rightKind"`
}

// dirCompareState is the Compare Directories sheet: the folder Compare
// Directories was invoked on, the other folder typed into it, and the rows
// the backend answered.
type dirCompareState struct {
	open      bool
	left      string
	other     string // the typed folder, absolute once confirmed
	asking    bool
	loading   bool
	err       string
	entries   []dirCompareEntry
	showEqual bool
}

// OpenDirCompare asks for the folder to compare dir with, as Compare
// Directories does.
func (m *Model) OpenDirCompare(dir string) {
	if dir == "" || m.Root() == "" {
		return
	}
	m.dirCompare = &dirCompareState{open: true, left: dir, asking: true}
	m.invalidate()
}

// CompareDirectoriesSelected compares the folder selected in the project view,
// or the project root, with a typed folder.
func (m *Model) CompareDirectoriesSelected() {
	dir := m.Root()
	if m.tree != nil {
		if sel := m.tree.Selected(); m.tree.IsDir(sel) {
			dir = sel
		}
	}
	m.OpenDirCompare(dir)
}

// ConfirmDirCompare runs the comparison with the typed folder.
func (m *Model) ConfirmDirCompare() {
	d := m.dirCompare
	if d == nil || d.loading {
		return
	}
	other := strings.TrimSpace(d.other)
	if other == "" {
		d.err = "先输入要比较的目录"
		m.invalidate()
		return
	}
	if !filepath.IsAbs(other) {
		other = filepath.Join(m.Root(), other)
	}
	other = filepath.Clean(other)
	if other == d.left {
		d.err = "选中和输入的目录相同"
		m.invalidate()
		return
	}
	d.other = other
	d.asking, d.loading, d.err = false, true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		ch := m.nextChannel()
		raw, err := m.mgr.Invoke(ctx, Window, "fs_compare_directories", map[string]any{
			"left": d.left, "right": other, "request": ch,
			"onProgress": map[string]any{"$channel": ch},
		})
		var entries []dirCompareEntry
		if err == nil {
			err = json.Unmarshal(raw, &entries)
		}
		m.post(func() {
			if m.dirCompare != d {
				return
			}
			d.loading = false
			if err != nil {
				d.err = "比较目录失败：" + err.Error()
				m.invalidate()
				return
			}
			d.entries = entries
			m.invalidate()
		})
	}()
}

// closeDirCompare closes the sheet.
func (m *Model) closeDirCompare() {
	if m.dirCompare == nil {
		return
	}
	m.dirCompare = nil
	m.invalidate()
}

// dirCompareStatus names a row's difference in the sheet's words.
func dirCompareStatus(status string) string {
	switch status {
	case "leftOnly":
		return "仅左侧"
	case "rightOnly":
		return "仅右侧"
	case "different":
		return "内容不同"
	}
	return "相同"
}

// dirCompareCounts counts the files each status found, folders excluded.
func dirCompareCounts(entries []dirCompareEntry) map[string]int {
	out := map[string]int{}
	for _, e := range entries {
		if e.LeftKind == "dir" || e.RightKind == "dir" {
			continue
		}
		out[e.Status]++
	}
	return out
}

// dirCompareDialog draws the Compare Directories sheet.
func (m *Model) dirCompareDialog(c *ui.Context, p *ui.Theme) {
	d := m.dirCompare
	if d == nil {
		return
	}
	open := d.open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(720).Gap(8).Children(func() {
			title := "比较目录 · " + filepath.Base(d.left)
			if d.loading {
				title = "比较目录 · 正在比较…"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, filepath.ToSlash(d.left)).FontSize(10.5).TextColor(p.TextMuted).
				SingleLine().Ellipsis("…")
			if d.other != "" {
				ui.Text(c, filepath.ToSlash(d.other)).FontSize(10.5).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			if d.err != "" {
				ui.Text(c, d.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if d.asking {
				in := ui.TextInput(c, &d.other).AutoFocus()
				in.OnChange(func() { d.err = "" })
				if in.Submitted() {
					m.ConfirmDirCompare()
				}
				ui.Text(c, "输入另一个目录的路径，相对路径从项目根开始。").
					FontSize(10.5).TextColor(p.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "取消").OnClick(func() { m.closeDirCompare() })
					ui.Button(c, "比较").OnClick(func() { m.ConfirmDirCompare() })
				})
				return
			}
			if d.loading {
				ui.Text(c, "正在读取两个目录…").FontSize(11.5).TextColor(p.TextMuted)
				ui.Button(c, "取消").OnClick(func() { m.closeDirCompare() })
				return
			}
			counts := dirCompareCounts(d.entries)
			ui.Text(c, fmt.Sprintf("%d 个文件不同，%d 个只在左侧，%d 个只在右侧，%d 个相同",
				counts["different"], counts["leftOnly"], counts["rightOnly"], counts["equal"])).
				FontSize(11).TextColor(p.TextMuted).SingleLine()
			shown := 0
			ui.Scroll(c).Height(400).Children(func() {
				ui.Column(c).Children(func() {
					for i, e := range d.entries {
						i, e := i, e
						if !d.showEqual && e.Status == "equal" {
							continue
						}
						shown++
						row := ui.Row(c.Key(fmt.Sprintf("dircmp:%d", i))).Height(22).
							Padding(0, 8).Gap(8).AlignItems(ui.Center)
						if row.Hovered() {
							row.Background(p.SurfaceHover)
						}
						row.Children(func() {
							ui.Row(c).Width(64).Shrink(0).Children(func() {
								ui.Text(c, dirCompareStatus(e.Status)).FontSize(10.5).
									TextColor(dirCompareTint(e.Status, p)).SingleLine()
							})
							ui.Text(c, e.Path).FontSize(11.5).TextColor(p.Text).
								SingleLine().Ellipsis("…")
						})
					}
					if shown == 0 {
						ui.Row(c).Padding(6, 8).Children(func() {
							ui.Text(c, "两个目录没有差异").FontSize(11.5).TextColor(p.TextMuted)
						})
					}
				})
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := "显示相同的文件"
				if d.showEqual {
					label = "隐藏相同的文件"
				}
				ui.Button(c, label).OnClick(func() {
					d.showEqual = !d.showEqual
					m.invalidate()
				})
				ui.Spacer(c)
				ui.Button(c, "另一个目录…").OnClick(func() {
					d.asking = true
					d.other = ""
					d.err = ""
					m.invalidate()
				})
				ui.Button(c, "关闭").OnClick(func() { m.closeDirCompare() })
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeDirCompare()
	}
}

func dirCompareTint(status string, p *ui.Theme) ui.Color {
	switch status {
	case "leftOnly":
		return p.Accent
	case "rightOnly":
		return p.Warning
	case "different":
		return p.Danger
	}
	return p.TextMuted
}
