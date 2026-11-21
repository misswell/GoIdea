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
		d.err = "Enter the directory to compare first"
		m.invalidate()
		return
	}
	if !filepath.IsAbs(other) {
		other = filepath.Join(m.Root(), other)
	}
	other = filepath.Clean(other)
	if other == d.left {
		d.err = "The selected and entered directories are the same"
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
				d.err = "Comparing the directories failed: " + err.Error()
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
		return "Left only"
	case "rightOnly":
		return "Right only"
	case "different":
		return "Different"
	}
	return "Same"
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
			title := "Compare Directories · " + filepath.Base(d.left)
			if d.loading {
				title = "Compare Directories · Comparing…"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, filepath.ToSlash(d.left)).FontSize(12).TextColor(p.TextMuted).
				SingleLine().Ellipsis("…")
			if d.other != "" {
				ui.Text(c, filepath.ToSlash(d.other)).FontSize(12).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			if d.err != "" {
				ui.Text(c, d.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if d.asking {
				in := ui.TextInput(c, &d.other).AutoFocus()
				in.OnChange(func() { d.err = "" })
				if in.Submitted() {
					m.ConfirmDirCompare()
				}
				ui.Text(c, "Enter the other directory's path; relative paths start at the project root.").
					FontSize(12).TextColor(p.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "Cancel").OnClick(func() { m.closeDirCompare() })
					ui.Button(c, "Compare").OnClick(func() { m.ConfirmDirCompare() })
				})
				return
			}
			if d.loading {
				ui.Text(c, "Reading both directories…").FontSize(12).TextColor(p.TextMuted)
				ui.Button(c, "Cancel").OnClick(func() { m.closeDirCompare() })
				return
			}
			counts := dirCompareCounts(d.entries)
			ui.Text(c, fmt.Sprintf("%d differ, %d only on the left, %d only on the right, %d identical",
				counts["different"], counts["leftOnly"], counts["rightOnly"], counts["equal"])).
				FontSize(12).TextColor(p.TextMuted).SingleLine()
			shown := 0
			ui.Scroll(c).Height(400).Children(func() {
				ui.Column(c).Children(func() {
					for i, e := range d.entries {
						i, e := i, e
						if !d.showEqual && e.Status == "equal" {
							continue
						}
						shown++
						row := ui.Row(c.Key(fmt.Sprintf("dircmp:%d", i))).Height(24).
							Padding(0, 8).Gap(8).AlignItems(ui.Center)
						if row.Hovered() {
							row.Background(p.SurfaceHover)
						}
						row.Children(func() {
							ui.Row(c).Width(64).Shrink(0).Children(func() {
								ui.Text(c, dirCompareStatus(e.Status)).FontSize(12).
									TextColor(dirCompareTint(e.Status, p)).SingleLine()
							})
							ui.Text(c, e.Path).FontSize(13).TextColor(p.Text).
								SingleLine().Ellipsis("…")
						})
						if row.Clicked() && e.LeftKind != "dir" && e.RightKind != "dir" {
							rel := filepath.FromSlash(e.Path)
							var chain []diffFileTarget
							for _, entry := range d.entries {
								if entry.LeftKind == "dir" || entry.RightKind == "dir" {
									continue
								}
								entryRel := filepath.FromSlash(entry.Path)
								chain = append(chain, diffFileTarget{
									left:  filepath.Join(d.left, entryRel),
									right: filepath.Join(d.other, entryRel),
									label: entry.Path,
								})
							}
							index := 0
							for j := range chain {
								if filepath.ToSlash(filepath.Join(d.left, rel)) == filepath.ToSlash(chain[j].left) {
									index = j
									break
								}
							}
							m.closeDirCompare()
							if len(chain) > 1 {
								m.loadDiffFile(chain, index)
							} else {
								m.ShowFileDiff(filepath.Join(d.left, rel), filepath.Join(d.other, rel))
							}
						}
					}
					if shown == 0 {
						ui.Row(c).Padding(6, 8).Children(func() {
							ui.Text(c, "The two directories have no differences").FontSize(12).TextColor(p.TextMuted)
						})
					}
				})
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := "Show identical files"
				if d.showEqual {
					label = "Hide identical files"
				}
				ui.Button(c, label).OnClick(func() {
					d.showEqual = !d.showEqual
					m.invalidate()
				})
				ui.Spacer(c)
				ui.Button(c, "Another Directory…").OnClick(func() {
					d.asking = true
					d.other = ""
					d.err = ""
					m.invalidate()
				})
				ui.Button(c, "Close").OnClick(func() { m.closeDirCompare() })
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
