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

// logLimit is how many commits the Git log asks for at once, as OpenIDEA's
// first window does; the list is read-only, so a fixed window is enough.
const logLimit = 200

// LogPane is the Git tool window's Log tab: the commit list, the selected
// commit's message, branches and signature, and the files that commit changed.
type LogPane struct {
	visible   bool
	loading   bool
	loaded    bool
	commits   []openidea.Commit
	selected  int
	detailFor string
	details   openidea.LogDetails
	changes   []openidea.RevisionChange
	err       string
	detailErr string
	detailing bool
}

func (m *Model) resetLog() { m.log = LogPane{} }

// ShowLog opens the Git log panel and loads the commit list.
func (m *Model) ShowLog() {
	if m.Root() == "" {
		return
	}
	m.log.visible = true
	m.invalidate()
	m.RefreshLog()
}

// CloseLog hides the Git log panel; the next open reloads it.
func (m *Model) CloseLog() {
	m.log.visible = false
	m.invalidate()
}

// RefreshLog reloads the commit list in the background.
func (m *Model) RefreshLog() {
	root := m.Root()
	if root == "" || m.log.loading {
		return
	}
	m.log.loading = true
	m.log.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_log", map[string]any{
			"repo": nil,
			"query": map[string]any{
				"branch": nil, "path": nil, "author": nil, "text": nil,
				"skip": 0, "limit": logLimit,
			},
		})
		var commits []openidea.Commit
		if err == nil {
			err = json.Unmarshal(raw, &commits)
		}
		m.post(func() {
			m.log.loading = false
			if m.Root() != root {
				return
			}
			m.log.loaded = true
			if err != nil {
				m.log.err = vcsErrorText(err)
				m.log.commits = nil
				m.invalidate()
				return
			}
			m.log.commits = commits
			if len(commits) == 0 {
				m.log.selected = 0
				m.log.detailFor = ""
				m.log.details = openidea.LogDetails{}
				m.log.changes = nil
			} else if m.log.selected >= len(commits) {
				m.log.selected = 0
			}
			m.invalidate()
			if len(commits) > 0 {
				m.selectCommit(m.log.selected)
			}
		})
	}()
}

// selectCommit loads the selected commit's details and changed files.
func (m *Model) selectCommit(i int) {
	root := m.Root()
	if root == "" || i < 0 || i >= len(m.log.commits) {
		return
	}
	m.log.selected = i
	id := m.log.commits[i].ID
	m.log.detailFor = id
	m.log.details = openidea.LogDetails{}
	m.log.changes = nil
	m.log.detailErr = ""
	m.log.detailing = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		detailsRaw, detailsErr := m.mgr.Invoke(ctx, Window, "vcs_log_details", map[string]any{
			"repo": nil, "revision": id,
		})
		var details openidea.LogDetails
		if detailsErr == nil {
			detailsErr = json.Unmarshal(detailsRaw, &details)
		}
		changesRaw, changesErr := m.mgr.Invoke(ctx, Window, "vcs_commits_changes", map[string]any{
			"repo": nil, "commits": []string{id},
		})
		var changes []openidea.RevisionChange
		if changesErr == nil {
			changesErr = json.Unmarshal(changesRaw, &changes)
		}
		m.post(func() {
			if m.Root() != root || m.log.detailFor != id {
				return
			}
			m.log.detailing = false
			if detailsErr != nil {
				m.log.detailErr = "读取提交详情失败：" + detailsErr.Error()
			} else {
				m.log.details = details
			}
			if changesErr != nil && m.log.detailErr == "" {
				m.log.detailErr = "读取提交文件失败：" + changesErr.Error()
			}
			m.log.changes = changes
			m.invalidate()
		})
	}()
}

// SmokeLog reports the Git log through the backend: the offscreen check of the
// log panel, its details and the files of its first commit.
func (m *Model) SmokeLog(ctx context.Context) (map[string]any, error) {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_log", map[string]any{
		"repo": nil,
		"query": map[string]any{
			"branch": nil, "path": nil, "author": nil, "text": nil,
			"skip": 0, "limit": 10,
		},
	})
	if err != nil {
		return nil, err
	}
	var commits []openidea.Commit
	if err := json.Unmarshal(raw, &commits); err != nil {
		return nil, err
	}
	out := map[string]any{"count": len(commits)}
	if len(commits) == 0 {
		return out, fmt.Errorf("vcs_log returned no commits")
	}
	first := commits[0]
	out["first"] = first.ShortID
	out["summary"] = first.Summary
	detailsRaw, err := m.mgr.Invoke(ctx, Window, "vcs_log_details", map[string]any{
		"repo": nil, "revision": first.ID,
	})
	if err != nil {
		return out, err
	}
	var details openidea.LogDetails
	if err := json.Unmarshal(detailsRaw, &details); err != nil {
		return out, err
	}
	out["message"] = details.Message
	out["branches"] = len(details.ContainingBranches)
	changesRaw, err := m.mgr.Invoke(ctx, Window, "vcs_commits_changes", map[string]any{
		"repo": nil, "commits": []string{first.ID},
	})
	if err != nil {
		return out, err
	}
	var changes []openidea.RevisionChange
	if err := json.Unmarshal(changesRaw, &changes); err != nil {
		return out, err
	}
	out["changes"] = len(changes)
	names := make([]string, 0, len(changes))
	for _, c := range changes {
		names = append(names, c.Path)
	}
	out["changedFiles"] = names
	return out, nil
}

// logPanel draws the Git log: commits on the left, the selected commit on the
// right, as OpenIDEA's Git tool window splits Log and its details.
func (m *Model) logPanel(c *ui.Context, p *ui.Theme) {
	if !m.log.visible {
		return
	}
	l := &m.log
	ui.Column(c).Height(260).Shrink(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(30).Padding(0, 8).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "Git 日志").FontSize(12).Bold().TextColor(p.Text)
			ui.Text(c, "提交历史").FontSize(12).TextColor(p.TextMuted)
			ui.Spacer(c)
			if l.loading {
				ui.Text(c, "正在读取…").FontSize(11).TextColor(p.TextMuted)
			} else if l.loaded {
				ui.Text(c, fmt.Sprintf("最近 %d 个提交", len(l.commits))).
					FontSize(11).TextColor(p.TextMuted)
			}
			ui.Button(c, "刷新").OnClick(func() { m.RefreshLog() })
			ui.Button(c, "关闭").OnClick(func() { m.CloseLog() })
		})
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(420).Children(func() {
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Padding(4, 0).Children(func() { m.logRows(c, p) })
				})
			})
			ui.Column(c).Grow(1).MinWidth(0).Padding(8, 10).Children(func() {
				m.logDetails(c, p)
			})
		})
	})
}

func (m *Model) logRows(c *ui.Context, p *ui.Theme) {
	l := &m.log
	if l.err != "" {
		ui.Row(c).Padding(8, 6).Children(func() {
			ui.Text(c, l.err).FontSize(11.5).TextColor(p.Warning)
		})
		return
	}
	if !l.loaded && l.loading {
		ui.Text(c, "正在读取提交历史…").FontSize(11.5).TextColor(p.TextMuted)
		return
	}
	if len(l.commits) == 0 {
		ui.Text(c, "没有提交记录").FontSize(11.5).TextColor(p.TextMuted)
		return
	}
	for i, cm := range l.commits {
		i, cm := i, cm
		r := ui.Row(c.Key(fmt.Sprintf("log:%d", i))).Height(24).Padding(0, 8).Gap(8).
			AlignItems(ui.Center)
		if i == l.selected {
			r.Background(p.SurfaceHover)
		} else if r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			ui.Text(c, cm.ShortID).FontSize(10.5).TextColor(p.Accent).SingleLine()
			if len(cm.Refs) > 0 {
				ui.Text(c, strings.Join(cm.Refs, ", ")).FontSize(10).TextColor(p.Success).
					SingleLine().Ellipsis("…")
			}
			ui.Text(c, cm.Summary).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			ui.Text(c, cm.Author).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			ui.Text(c, logTime(cm.TimeMs)).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		})
		if r.Clicked() && i != l.selected {
			m.selectCommit(i)
		}
	}
}

func (m *Model) logDetails(c *ui.Context, p *ui.Theme) {
	l := &m.log
	if l.selected < 0 || l.selected >= len(l.commits) {
		ui.Text(c, "选择一个提交查看详情").FontSize(11.5).TextColor(p.TextMuted)
		return
	}
	cm := l.commits[l.selected]
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, cm.Summary).FontSize(12.5).Bold().TextColor(p.Text)
		ui.Text(c, cm.ShortID+" · "+cm.Author).FontSize(11).TextColor(p.TextMuted)
		if l.detailing {
			ui.Text(c, "正在读取提交详情…").FontSize(11).TextColor(p.TextMuted)
		}
		if l.detailErr != "" {
			ui.Text(c, l.detailErr).FontSize(11.5).TextColor(p.Warning)
		}
		message := l.details.Message
		if message == "" {
			message = cm.Message
		}
		if message != "" {
			ui.Scroll(c).Height(90).Children(func() {
				ui.Text(c, message).FontSize(11.5).TextColor(p.Text)
			})
		}
		if len(l.details.ContainingBranches) > 0 {
			ui.Text(c, "所在分支："+strings.Join(l.details.ContainingBranches, ", ")).
				FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
		if l.details.Signature != "" {
			ui.Text(c, "签名："+l.details.Signature).FontSize(11).TextColor(p.TextMuted)
		}
		if len(l.changes) == 0 {
			return
		}
		ui.Text(c, fmt.Sprintf("%d 个文件", len(l.changes))).FontSize(11).TextColor(p.TextMuted)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				for i, ch := range l.changes {
					i, ch := i, ch
					letter, color := logChangeBadge(ch.Kind, p)
					r := ui.Row(c.Key(fmt.Sprintf("logfile:%d", i))).Height(22).Gap(6).
						AlignItems(ui.Center)
					r.Children(func() {
						ui.Text(c, letter).FontSize(11).Bold().TextColor(color)
						ui.Text(c, filepath.Base(ch.Path)).FontSize(11.5).TextColor(p.Text).
							SingleLine().Ellipsis("…")
						ui.Spacer(c)
					})
				}
			})
		})
	})
}

// logChangeBadge is the letter and colour of a changed file in a commit.
func logChangeBadge(kind string, p *ui.Theme) (string, ui.Color) {
	switch kind {
	case "added":
		return "A", p.Success
	case "deleted":
		return "D", p.Danger
	case "renamed":
		return "R", p.Success
	case "conflicted":
		return "!", p.Danger
	}
	return "M", p.Accent
}

// logTime is a commit's time as the log list shows it: the date, or the time
// of day for today's commits, as Git log's default does.
func logTime(ms float64) string {
	if ms <= 0 {
		return ""
	}
	t := time.UnixMilli(int64(ms))
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("01-02")
	}
	return t.Format("2006-01-02")
}
