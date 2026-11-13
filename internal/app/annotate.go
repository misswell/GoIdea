package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// annotateState is the Annotate sheet of one file: the commit that last
// touched each of its lines.
type annotateState struct {
	open    bool
	path    string
	loading bool
	err     string
	rows    []annotateRow
}

// annotateRow is a run of consecutive lines from the same commit, as the
// annotate view groups them.
type annotateRow struct {
	Commit  string
	Author  string
	TimeMs  float64
	Summary string
	From    int
	To      int
}

// annotateRows groups blame lines by commit, numbering from 1.
func annotateRows(lines []openidea.BlameLine) []annotateRow {
	out := make([]annotateRow, 0, len(lines))
	for i, l := range lines {
		n := i + 1
		if len(out) > 0 {
			last := &out[len(out)-1]
			if last.Commit == l.Commit && last.Author == l.Author &&
				last.Summary == l.Summary && last.TimeMs == l.TimeMs {
				last.To = n
				continue
			}
		}
		out = append(out, annotateRow{
			Commit: l.Commit, Author: l.Author, TimeMs: l.TimeMs,
			Summary: l.Summary, From: n, To: n,
		})
	}
	return out
}

// annotateLines is one row's line range, as "12" or "12–18".
func annotateLines(r annotateRow) string {
	if r.To > r.From {
		return fmt.Sprintf("%d–%d", r.From, r.To)
	}
	return fmt.Sprintf("%d", r.From)
}

// annotateTime is a blame line's local date.
func annotateTime(ms float64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(int64(ms)).Format("2006-01-02")
}

// ShowAnnotate opens Annotate for a file, as the Git menu's Annotate does: the
// blame of the text the user sees, with runs of the same commit grouped.
func (m *Model) ShowAnnotate(path string) {
	if path == "" || m.Root() == "" {
		return
	}
	path = realPath(path)
	a := &annotateState{open: true, path: path, loading: true}
	m.annotate = a
	m.invalidate()
	// The buffer, not the saved file, is what the user is annotating.
	text, _ := m.bufferText(path)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		var body *string
		if text != "" {
			body = &text
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_blame", map[string]any{
			"path": path, "text": body, "revision": nil,
		})
		var lines []openidea.BlameLine
		if err == nil {
			err = json.Unmarshal(raw, &lines)
		}
		m.post(func() {
			if m.annotate != a {
				return
			}
			a.loading = false
			if err != nil {
				a.err = diffErrorText(err)
				m.invalidate()
				return
			}
			a.rows = annotateRows(lines)
			m.invalidate()
		})
	}()
}

// closeAnnotate closes the Annotate sheet.
func (m *Model) closeAnnotate() {
	if m.annotate == nil {
		return
	}
	m.annotate = nil
	m.invalidate()
}

// annotateDialog draws the Annotate sheet.
func (m *Model) annotateDialog(c *ui.Context, p *ui.Theme) {
	a := m.annotate
	if a == nil {
		return
	}
	open := a.open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(820).Gap(8).Children(func() {
			title := "注释 · " + filepath.Base(a.path)
			if a.loading {
				title = "注释 · 正在读取…"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if rel, err := filepath.Rel(m.Root(), a.path); err == nil && !strings.HasPrefix(rel, "..") {
				ui.Text(c, filepath.ToSlash(rel)).FontSize(10.5).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			switch {
			case a.err != "":
				ui.Text(c, a.err).FontSize(11.5).TextColor(p.Danger).SingleLine().Ellipsis("…")
			case a.loading:
				ui.Text(c, "正在读取逐行提交…").FontSize(11.5).TextColor(p.TextMuted)
			case len(a.rows) == 0:
				ui.Text(c, "没有可注释的行").FontSize(11.5).TextColor(p.TextMuted)
			}
			if len(a.rows) > 0 {
				ui.Scroll(c).Height(460).Children(func() {
					ui.Column(c).Children(func() {
						for i, r := range a.rows {
							i, r := i, r
							row := ui.Row(c.Key(fmt.Sprintf("annotate:%d", i))).Height(22).
								Padding(0, 8).Gap(8).AlignItems(ui.Center)
							if row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Row(c).Width(56).Shrink(0).Justify(ui.End).Children(func() {
									ui.Text(c, annotateLines(r)).FontSize(10.5).
										TextColor(p.TextMuted).SingleLine()
								})
								ui.Row(c).Width(72).Shrink(0).Children(func() {
									short := r.Commit
									if len(short) > 8 {
										short = short[:8]
									}
									ui.Text(c, short).FontSize(10.5).TextColor(p.Accent).SingleLine()
								})
								ui.Row(c).Width(120).Shrink(0).Children(func() {
									ui.Text(c, r.Author).FontSize(10.5).TextColor(p.Text).
										SingleLine().Ellipsis("…")
								})
								ui.Row(c).Width(86).Shrink(0).Children(func() {
									ui.Text(c, annotateTime(r.TimeMs)).FontSize(10.5).
										TextColor(p.TextMuted).SingleLine()
								})
								ui.Text(c, r.Summary).FontSize(11.5).TextColor(p.Text).
									SingleLine().Ellipsis("…")
							})
						}
					})
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "关闭").OnClick(func() { m.closeAnnotate() })
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeAnnotate()
	}
}
