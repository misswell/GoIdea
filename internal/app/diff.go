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
	// other is the right-hand file of a two-file compare; empty means the
	// compare is against HEAD.
	other string

	loading bool
	err     string
	message string
}

// ShowDiff opens the compare view for a file against its committed version.
func (m *Model) ShowDiff(path string) {
	if path == "" || m.Root() == "" {
		return
	}
	path = realPath(path)
	// The buffer, not the saved file, is what the user is looking at.
	cur, fromTab := m.bufferText(path)
	m.diff = &diffView{open: true, path: path, loading: true}
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
		var rows []textdiff.Row
		var added, gone int
		if err == nil {
			rows = textdiff.Compare(old, cur, diffContext)
			for _, r := range rows {
				switch r.Kind {
				case textdiff.Add:
					added++
				case textdiff.Delete:
					gone++
				}
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
			d.added, d.gone = added, gone
			if added == 0 && gone == 0 {
				d.message = "与 HEAD 相同"
				m.invalidate()
				return
			}
			d.rows = rows
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
	m.diff = &diffView{open: true, path: left, other: right, loading: true}
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
		var rows []textdiff.Row
		var added, gone int
		if err == nil {
			rows = textdiff.Compare(old, cur, diffContext)
			for _, r := range rows {
				switch r.Kind {
				case textdiff.Add:
					added++
				case textdiff.Delete:
					gone++
				}
			}
		}
		m.post(func() {
			d := m.diff
			if d == nil || d.path != left || d.other != right {
				return
			}
			d.loading = false
			if err != nil {
				d.err = "读取文件失败：" + err.Error()
				m.invalidate()
				return
			}
			d.added, d.gone = added, gone
			if added == 0 && gone == 0 {
				d.message = "两个文件相同"
				m.invalidate()
				return
			}
			d.rows = rows
			m.invalidate()
		})
	}()
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
		return "当前项目不是 Git 仓库"
	}
	return "读取差异失败：" + err.Error()
}

// diffDialog draws the compare view: the file's diff against HEAD.
func (m *Model) diffDialog(c *ui.Context, p *ui.Theme) {
	d := m.diff
	if d == nil {
		return
	}
	open := d.open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(920).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				title := filepath.Base(d.path) + " · 与 HEAD 比较"
				if d.other != "" {
					title = filepath.Base(d.path) + " ↔ " + filepath.Base(d.other)
				}
				if d.loading {
					title = filepath.Base(d.path) + " · 正在比较…"
				}
				ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				if !d.loading && d.err == "" && d.message == "" {
					ui.Text(c, fmt.Sprintf("+%d  −%d", d.added, d.gone)).
						FontSize(11).TextColor(p.TextMuted).SingleLine()
				}
			})
			if rel, err := filepath.Rel(m.Root(), d.path); err == nil && !strings.HasPrefix(rel, "..") {
				ui.Text(c, filepath.ToSlash(rel)).FontSize(10.5).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			if d.other != "" {
				ui.Text(c, filepath.ToSlash(d.other)).FontSize(10.5).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			}
			switch {
			case d.err != "":
				ui.Text(c, d.err).FontSize(11.5).TextColor(p.Danger).SingleLine().Ellipsis("…")
			case d.message != "":
				ui.Text(c, d.message).FontSize(11.5).TextColor(p.TextMuted).SingleLine()
			}
			if len(d.rows) > 0 {
				addTint := p.Success.Mix(p.Background, 0.14)
				delTint := p.Danger.Mix(p.Background, 0.14)
				hunkTint := p.Accent.Mix(p.Background, 0.08)
				ui.Scroll(c).Height(480).Children(func() {
					ui.Column(c).Children(func() {
						for i, r := range d.rows {
							i, r := i, r
							row := ui.Row(c.Key(fmt.Sprintf("diff:%d", i))).Height(20).
								Padding(0, 8).AlignItems(ui.Center)
							switch r.Kind {
							case textdiff.Add:
								row.Background(addTint)
							case textdiff.Delete:
								row.Background(delTint)
							case textdiff.Hunk:
								row.Background(hunkTint)
							}
							row.Children(func() {
								switch r.Kind {
								case textdiff.Hunk:
									ui.Text(c, r.Text).FontSize(11).TextColor(p.Accent).
										SingleLine().Ellipsis("…")
								default:
									ui.Row(c).Width(34).Shrink(0).Children(func() {
										ui.Text(c, lineNumber(r.Old)).FontSize(10.5).
											TextColor(p.TextMuted).SingleLine()
									})
									ui.Row(c).Width(34).Shrink(0).Children(func() {
										ui.Text(c, lineNumber(r.New)).FontSize(10.5).
											TextColor(p.TextMuted).SingleLine()
									})
									ui.Row(c).Width(16).Shrink(0).Justify(ui.Center).Children(func() {
										ui.Text(c, diffSign(r.Kind)).FontSize(11).
											TextColor(diffColor(r.Kind, p)).SingleLine()
									})
									ui.Text(c, r.Text).FontSize(11.5).TextColor(diffColor(r.Kind, p)).
										SingleLine().Ellipsis("…")
								}
							})
						}
					})
				})
			}
		})
	})
	if dlg.Dismissed() || !open {
		m.closeDiff()
	}
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
		return out, errors.New("没有打开项目")
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
	for i := range files {
		if files[i].Unstaged != nil {
			target = &files[i]
			break
		}
	}
	if target == nil && len(files) > 0 {
		target = &files[0]
	}
	if target == nil {
		return out, errors.New("没有未提交的更改可比较")
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
			out["error"] = "比较超时"
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	mygo.RunOnMain(func() { m.closeDiff() })
	return out, nil
}
