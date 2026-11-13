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

// commitState is the Commit dialog: the message, the inclusion of the changed
// files and the commit options. The changed files themselves stay in m.vcs, so
// a refresh while the dialog is open keeps the list current.
type commitState struct {
	message  string
	amend    bool
	signOff  bool
	runHooks bool
	// checks overrides the inclusion of a file (absolute path → included);
	// without an entry a file uses its initial check, as OpenIDEA does.
	checks map[string]bool
	// only keeps a single file in the commit, as the tab menu's Commit File…
	// asks for.
	only string
	busy bool
	err  string
}

// commitPlan is what vcs_commit gets: files committed with their working-tree
// content, staged files kept out, and the files that end up in the commit.
type commitPlan struct {
	stage    []string
	exclude  []string
	included []string
	conflict []string
}

// OpenCommit shows the Commit dialog for the project's changes, saving the
// documents first so the commit is made from what is on disk.
func (m *Model) OpenCommit() {
	if m.Root() == "" || m.commit != nil {
		return
	}
	m.commit = &commitState{runHooks: true, checks: map[string]bool{}}
	m.RefreshVCS()
	m.invalidate()
}

// OpenCommitFile shows the Commit dialog for one file, as the tab menu's
// Commit File… does: every other change stays out of the commit.
func (m *Model) OpenCommitFile(path string) {
	if m.Root() == "" || m.commit != nil || path == "" {
		return
	}
	m.commit = &commitState{runHooks: true, checks: map[string]bool{}, only: realPath(path)}
	m.RefreshVCS()
	m.invalidate()
}

// commitIncluded is a file's inclusion: the user's override, or the initial
// check OpenIDEA uses — changes are included, unversioned files and conflicts
// are opt-in, and a partially staged file commits only what is staged. A
// single-file commit includes only that file, even when it is new.
func (m *Model) commitIncluded(c *commitState, f openidea.FileStatus) bool {
	if c.only != "" {
		if f.Path != c.only {
			return false
		}
		kind := effectiveVCSKind(f)
		return kind != "ignored" && kind != "" && kind != "conflicted"
	}
	if on, ok := c.checks[f.Path]; ok {
		return on
	}
	kind := effectiveVCSKind(f)
	if kind == "ignored" || kind == "" {
		return false
	}
	if kind == "untracked" || kind == "conflicted" {
		return false
	}
	return true
}

// partiallyStaged reports whether a file has both an index entry and further
// working-tree changes, whose initial inclusion covers only the index side.
func partiallyStaged(f openidea.FileStatus) bool {
	if f.Staged == nil || f.Unstaged == nil {
		return false
	}
	switch *f.Unstaged {
	case "untracked", "conflicted":
		return false
	}
	return *f.Staged != "conflicted"
}

// stagedOnly reports whether an included file is committed as it is staged.
func stagedOnly(c *commitState, f openidea.FileStatus) bool {
	if _, ok := c.checks[f.Path]; ok {
		return false
	}
	return partiallyStaged(f)
}

// planCommit derives the vcs_commit options from the inclusion, mirroring
// OpenIDEA's planCommit: included unstaged files are staged, excluded staged
// files stay staged, and a staged rename's source is left out with it.
func (m *Model) planCommit(c *commitState) commitPlan {
	var plan commitPlan
	for _, f := range m.vcs.files {
		kind := effectiveVCSKind(f)
		if kind == "ignored" || kind == "" {
			continue
		}
		if !m.commitIncluded(c, f) {
			if f.Staged != nil && *f.Staged != "conflicted" {
				plan.exclude = append(plan.exclude, f.Path)
				if from := renamedFromPath(f); from != "" {
					plan.exclude = append(plan.exclude, from)
				}
			}
			continue
		}
		if kind == "conflicted" {
			plan.conflict = append(plan.conflict, f.RelPath)
			continue
		}
		if f.Unstaged != nil && !stagedOnly(c, f) {
			plan.stage = append(plan.stage, f.Path)
		}
		plan.included = append(plan.included, f.Path)
	}
	return plan
}

// submitCommit saves the documents and commits the planned files with
// vcs_commit; the backend copies the index, stages the included files and
// keeps the excluded ones staged.
func (m *Model) submitCommit() {
	c := m.commit
	if c == nil || c.busy {
		return
	}
	message := strings.TrimSpace(c.message)
	switch {
	case message == "" && !c.amend:
		c.err = "请输入提交信息"
		m.invalidate()
		return
	case len(m.vcs.files) == 0:
		c.err = "没有可提交的更改"
		m.invalidate()
		return
	}
	plan := m.planCommit(c)
	if len(plan.conflict) > 0 {
		c.err = "请先解决冲突：" + strings.Join(plan.conflict, "、")
		m.invalidate()
		return
	}
	if len(plan.included) == 0 && !c.amend {
		c.err = "没有选择要提交的更改"
		m.invalidate()
		return
	}
	c.busy = true
	c.err = ""
	m.status = "正在提交…"
	m.invalidate()
	tabs := append([]*Tab(nil), m.tabs...)
	options := openidea.CommitOptions{
		Amend: c.amend, SignOff: c.signOff, RunHooks: c.runHooks,
		Stage: plan.stage, Exclude: plan.exclude,
	}
	p := m.prefs.SaveFiles
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 3*time.Minute)
		defer cancel()
		if err := saveAll(ctx, tabs, p); err != nil {
			m.finishCommit(c, "", err)
			return
		}
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_commit", map[string]any{
			"repo": nil, "message": message, "options": options,
		})
		var id string
		if err == nil {
			err = json.Unmarshal(raw, &id)
		}
		m.finishCommit(c, id, err)
	}()
}

func (m *Model) finishCommit(c *commitState, id string, err error) {
	m.post(func() {
		if c != m.commit {
			return
		}
		c.busy = false
		if err != nil {
			c.err = commitErrorText(err)
			m.status = "提交失败"
			m.invalidate()
			return
		}
		m.commit = nil
		summary := firstLine(c.message)
		if summary == "" {
			summary = "修正上次提交"
		}
		m.status = fmt.Sprintf("已提交 %s：%s", shortID(id), summary)
		m.RefreshVCS()
		m.invalidate()
	})
}

// SmokeCommit commits the project's changed files with a message and verifies
// the status afterwards; it is the offscreen check of the commit path.
func (m *Model) SmokeCommit(ctx context.Context) (map[string]any, error) {
	status := func() ([]openidea.FileStatus, error) {
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
		if err != nil {
			return nil, err
		}
		var files []openidea.FileStatus
		if err := json.Unmarshal(raw, &files); err != nil {
			return nil, err
		}
		return files, nil
	}
	before, err := status()
	if err != nil {
		return nil, err
	}
	m.vcs.files = before
	c := &commitState{message: "e2e: commit the change", runHooks: true, checks: map[string]bool{}}
	plan := m.planCommit(c)
	if len(plan.included) == 0 {
		return map[string]any{"included": 0}, nil
	}
	options := openidea.CommitOptions{
		RunHooks: c.runHooks, Stage: plan.stage, Exclude: plan.exclude,
	}
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_commit", map[string]any{
		"repo": nil, "message": c.message, "options": options,
	})
	if err != nil {
		return map[string]any{"included": len(plan.included)}, err
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, err
	}
	after, err := status()
	if err != nil {
		return nil, err
	}
	remaining := make([]string, 0, len(after))
	for _, f := range after {
		remaining = append(remaining, f.RelPath)
	}
	cut := len(id)
	if cut > 12 {
		cut = 12
	}
	return map[string]any{
		"id": id, "shortId": id[:cut], "message": c.message,
		"staged": len(plan.stage), "excluded": len(plan.exclude),
		"before": len(before), "after": len(after), "remaining": remaining,
	}, nil
}

// effectiveVCSKind is the change kind a status row shows, working-tree side
// first, as OpenIDEA's effectiveKind derives it.
func effectiveVCSKind(f openidea.FileStatus) string {
	if f.Staged != nil && *f.Staged == "conflicted" || f.Unstaged != nil && *f.Unstaged == "conflicted" {
		return "conflicted"
	}
	if f.Unstaged != nil {
		if *f.Unstaged == "untracked" {
			if f.Staged != nil {
				return *f.Staged
			}
			return "untracked"
		}
		if *f.Unstaged == "ignored" {
			return "ignored"
		}
	}
	if f.Staged != nil && *f.Staged == "added" {
		if f.Unstaged != nil && *f.Unstaged == "deleted" {
			return ""
		}
		return "added"
	}
	if f.Unstaged != nil && *f.Unstaged == "deleted" || f.Staged != nil && *f.Staged == "deleted" {
		return "deleted"
	}
	if f.Staged != nil && *f.Staged == "renamed" {
		return "renamed"
	}
	if f.Staged != nil || f.Unstaged != nil {
		return "modified"
	}
	return ""
}

// renamedFromPath is the absolute path a staged rename came from, which the
// commit leaves out together with the new path.
func renamedFromPath(f openidea.FileStatus) string {
	if f.RenamedFrom == nil || *f.RenamedFrom == "" {
		return ""
	}
	path := strings.TrimSuffix(f.Path, "/")
	rel := strings.TrimSuffix(f.RelPath, "/")
	if !strings.HasSuffix(path, rel) {
		return ""
	}
	return strings.TrimSuffix(path, rel) + *f.RenamedFrom
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	return text
}

// commitErrorText turns the refusals a commit can hit into the dialog's words.
func commitErrorText(err error) string {
	if openidea.IsOutOfSync(err) {
		return "文件已在磁盘上改动，请刷新后重试"
	}
	return "提交失败：" + err.Error()
}

// commitDialog draws the Commit dialog over the window.
func (m *Model) commitDialog(c *ui.Context, p *ui.Theme) {
	s := m.commit
	if s == nil {
		return
	}
	open := true
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(8).Children(func() {
			ui.Text(c, "提交更改").FontSize(13).Bold().TextColor(p.Text)
			ui.TextArea(c, &s.message).Height(84)
			ui.Text(c, "提交信息，例如：修复登录超时").FontSize(11).TextColor(p.TextMuted)
			ui.Text(c, fmt.Sprintf("已选择 %d 个文件", len(m.planCommit(s).included))).
				FontSize(11).TextColor(p.TextMuted)
			if len(m.vcs.files) == 0 {
				ui.Text(c, "没有可提交的更改").FontSize(11.5).TextColor(p.TextMuted)
			} else {
				ui.Scroll(c).Height(220).Children(func() {
					for i, f := range m.vcs.files {
						i, f := i, f
						if effectiveVCSKind(f) == "ignored" {
							continue
						}
						on := m.commitIncluded(s, f)
						row := ui.Row(c.Key(fmt.Sprintf("commit:%d", i))).Gap(6).AlignItems(ui.Center)
						row.Children(func() {
							ui.Checkbox(c, &on, "")
							_, word, _ := vcsBadge(f)
							ui.Text(c, filepath.Base(f.RelPath)).FontSize(12).TextColor(p.Text).
								SingleLine().Ellipsis("…")
							ui.Spacer(c)
							ui.Text(c, word).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
						})
						if on != m.commitIncluded(s, f) {
							s.checks[f.Path] = on
							m.invalidate()
						}
					}
				})
			}
			ui.Row(c).Gap(12).Children(func() {
				ui.Checkbox(c, &s.amend, "修正上次提交")
				ui.Checkbox(c, &s.signOff, "签名")
				ui.Checkbox(c, &s.runHooks, "运行 Git 钩子")
			})
			if s.err != "" {
				ui.Text(c, s.err).FontSize(11).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if s.busy {
					ui.Text(c, "提交中…").FontSize(11.5).TextColor(p.TextMuted)
				}
				if s.amend {
					ui.Button(c, "修正提交").Disabled(s.busy).OnClick(m.submitCommit)
				} else {
					ui.Button(c, "提交").Disabled(s.busy || len(m.vcs.files) == 0).OnClick(m.submitCommit)
				}
				ui.Button(c, "取消").OnClick(func() {
					m.commit = nil
					m.invalidate()
				})
			})
		})
	})
	if d.Dismissed() || !open {
		if !s.busy {
			m.commit = nil
			m.invalidate()
		}
	}
}
