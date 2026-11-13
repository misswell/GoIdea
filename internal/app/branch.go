package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// BranchPane is the title bar's branch widget and its Branches popup: the
// current head `vcs_branch_info` reports and the branches `vcs_branches` lists.
type BranchPane struct {
	loaded     bool
	head       *string
	headCommit *string
	upstream   *string
	ahead      int
	behind     int
	operation  *string

	open     bool
	query    string
	branches []openidea.Branch
	sel      int
	loading  bool
	listing  bool
	busy     bool
	err      string
}

func (m *Model) resetBranch() { m.branch = BranchPane{} }

// RefreshBranch reloads the current branch in the background.
func (m *Model) RefreshBranch() {
	root := m.Root()
	if root == "" || m.branch.loading {
		return
	}
	m.branch.loading = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_branch_info", map[string]any{"repo": nil})
		var info openidea.BranchInfo
		if err == nil {
			err = json.Unmarshal(raw, &info)
		}
		m.post(func() {
			m.branch.loading = false
			if m.Root() != root {
				return
			}
			m.branch.loaded = true
			if err != nil {
				// A project without Git simply has no widget.
				if branchErrorText(err) == "" {
					m.branch.head = nil
					m.branch.headCommit = nil
					m.branch.upstream = nil
					m.branch.ahead, m.branch.behind = 0, 0
					m.branch.operation = nil
				} else {
					m.branch.err = branchErrorText(err)
				}
				m.invalidate()
				return
			}
			m.branch.err = ""
			m.branch.head = info.Head
			m.branch.headCommit = info.HeadCommit
			m.branch.upstream = info.Upstream
			m.branch.ahead, m.branch.behind = info.Ahead, info.Behind
			m.branch.operation = info.Operation
			m.invalidate()
		})
	}()
}

// branchLabel is the title bar text: the branch or, on a detached head, its
// commit, an operation in progress, and how far it is from its upstream.
func (m *Model) branchLabel() string {
	b := &m.branch
	head := ""
	if b.head != nil {
		head = *b.head
	} else if b.headCommit != nil && len(*b.headCommit) >= 8 {
		head = (*b.headCommit)[:8]
	}
	label := branchOperationTitle(b.operation, head)
	if label == "" {
		label = head
	}
	if b.behind > 0 {
		label += fmt.Sprintf(" ↓%d", b.behind)
	}
	if b.ahead > 0 {
		label += fmt.Sprintf(" ↑%d", b.ahead)
	}
	return label
}

// branchOperationTitle words a stopped merge, rebase or cherry-pick the way
// IDEA's widget does: the operation with the branch it is on.
func branchOperationTitle(op *string, head string) string {
	if op == nil {
		return ""
	}
	verb := ""
	switch *op {
	case "rebase":
		verb = "正在变基"
	case "merge":
		verb = "正在合并"
	case "cherry-pick":
		verb = "正在拣选"
	case "revert":
		verb = "正在还原"
	case "am":
		verb = "正在应用补丁"
	default:
		return ""
	}
	if head == "" {
		return verb
	}
	return verb + " " + head
}

// ToggleBranches opens the Branches popup, or closes it when it is open.
func (m *Model) ToggleBranches() {
	if m.Root() == "" {
		return
	}
	if m.branch.open {
		m.closeBranches()
		return
	}
	m.branch.open = true
	m.branch.query = ""
	m.branch.err = ""
	m.branch.sel = 0
	m.branch.listing = true
	m.invalidate()
	m.RefreshBranch()
	m.loadBranches()
}

// closeBranches drops the popup.
func (m *Model) closeBranches() {
	if !m.branch.open {
		return
	}
	m.branch.open = false
	m.branch.query = ""
	m.invalidate()
}

// loadBranches lists the repository's branches for the popup.
func (m *Model) loadBranches() {
	root := m.Root()
	if root == "" || !m.branch.listing {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_branches", map[string]any{"repo": nil})
		var branches []openidea.Branch
		if err == nil {
			err = json.Unmarshal(raw, &branches)
		}
		m.post(func() {
			if m.Root() != root || !m.branch.listing {
				return
			}
			m.branch.listing = false
			if err != nil {
				if branchErrorText(err) != "" {
					m.branch.err = branchErrorText(err)
				}
				m.branch.branches = nil
				m.invalidate()
				return
			}
			// Local branches first, as IDEA groups them.
			sort.SliceStable(branches, func(i, j int) bool {
				if branches[i].Remote != branches[j].Remote {
					return !branches[i].Remote
				}
				return branches[i].Name < branches[j].Name
			})
			m.branch.branches = branches
			m.branch.sel = m.currentBranchRow()
			m.invalidate()
		})
	}()
}

func (m *Model) currentBranchRow() int {
	rows := m.branchRows()
	for i, b := range rows {
		if b.Current {
			return i
		}
	}
	return 0
}

// branchRows is the list the popup shows: the branches matching the typed
// filter, in their loaded order.
func (m *Model) branchRows() []openidea.Branch {
	query := strings.ToLower(strings.TrimSpace(m.branch.query))
	if query == "" {
		return m.branch.branches
	}
	out := make([]openidea.Branch, 0, len(m.branch.branches))
	for _, b := range m.branch.branches {
		if strings.Contains(strings.ToLower(b.Name), query) {
			out = append(out, b)
		}
	}
	return out
}

// stepBranch moves the chosen row by delta, wrapping as IDEA's popup does.
func (m *Model) stepBranch(delta int) {
	rows := m.branchRows()
	if len(rows) == 0 {
		return
	}
	sel := m.branch.sel + delta
	if sel < 0 {
		sel = len(rows) - 1
	} else if sel >= len(rows) {
		sel = 0
	}
	m.branch.sel = sel
	m.invalidate()
}

// CheckoutSelectedBranch checks out the row Enter picks.
func (m *Model) CheckoutSelectedBranch() {
	rows := m.branchRows()
	if len(rows) == 0 {
		m.invalidate()
		return
	}
	m.checkoutBranch(rows[clampSymbolSel(m.branch.sel, len(rows))].Name)
}

// checkoutBranch switches the repository to name and refreshes what shows it.
// The branch already checked out is a no-op that closes the popup.
func (m *Model) checkoutBranch(name string) {
	if name == "" || m.Root() == "" {
		return
	}
	if m.branch.head != nil && name == *m.branch.head {
		m.closeBranches()
		return
	}
	if m.branch.busy {
		return
	}
	m.branch.busy = true
	m.branch.err = ""
	m.status = "正在切换到 " + name + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_checkout", map[string]any{
			"repo": nil, "branch": name, "create": false, "options": nil,
		})
		m.post(func() {
			m.branch.busy = false
			if m.Root() == "" {
				return
			}
			if err != nil {
				m.branch.err = "切换分支失败：" + err.Error()
				m.invalidate()
				return
			}
			m.closeBranches()
			m.status = "已切换到 " + name
			m.RefreshBranch()
			if m.vcs.loaded || m.vcs.loading {
				m.RefreshVCS()
			}
			if m.log.visible {
				m.RefreshLog()
			}
			m.invalidate()
		})
	}()
}

// SmokeBranch exercises the widget's commands against the backend: the current
// head, the branch list, and a checkout that comes back to where it started.
func (m *Model) SmokeBranch(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	info, err := m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.Head != nil {
		out["head"] = *info.Head
	}
	branches, err := m.branchList(ctx)
	if err != nil {
		return out, err
	}
	out["branches"] = len(branches)
	for _, b := range branches {
		if b.Current {
			out["current"] = b.Name
		}
	}
	if info.Head == nil {
		return out, nil
	}
	const temp = "goidea-smoke"
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_create_branch", map[string]any{
		"repo": nil, "name": temp, "start": nil, "checkout": true, "overwrite": nil,
	}); err != nil {
		return out, err
	}
	if now, err := m.branchInfo(ctx); err == nil && now.Head != nil {
		out["checkedOut"] = *now.Head
	}
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_checkout", map[string]any{
		"repo": nil, "branch": *info.Head, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	if _, err := m.mgr.Invoke(ctx, Window, "vcs_delete_branch", map[string]any{
		"repo": nil, "branch": temp,
	}); err != nil {
		return out, err
	}
	back, err := m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	out["restored"] = back.Head != nil && *back.Head == *info.Head
	return out, nil
}

func (m *Model) branchInfo(ctx context.Context) (openidea.BranchInfo, error) {
	var info openidea.BranchInfo
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_branch_info", map[string]any{"repo": nil})
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(raw, &info)
	return info, err
}

func (m *Model) branchList(ctx context.Context) ([]openidea.Branch, error) {
	var branches []openidea.Branch
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_branches", map[string]any{"repo": nil})
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &branches)
	return branches, err
}

// branchErrorText turns the backend's refusal for a project without Git into
// "" (no widget, no error) and other failures into the sentence the popup
// shows.
func branchErrorText(err error) string {
	var oe *openidea.Error
	if errors.As(err, &oe) && oe.Kind == "unavailable" && strings.Contains(oe.Message, "not a git repository") {
		return ""
	}
	return "读取分支失败：" + err.Error()
}

// branchesDialog draws the Branches popup: a filter over the branches, the
// current one marked, and Enter or a click checking one out.
func (m *Model) branchesDialog(c *ui.Context, p *ui.Theme) {
	if !m.branch.open {
		return
	}
	open := m.branch.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, "分支").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &m.branch.query).AutoFocus()
			in.OnChange(func() {
				m.branch.sel = 0
				m.invalidate()
			})
			in.OnShortcut(0, ui.KeyDown, func() { m.stepBranch(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepBranch(-1) })
			if in.Submitted() {
				m.CheckoutSelectedBranch()
			}
			rows := m.branchRows()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				text := "输入名称筛选，Enter 切换分支"
				switch {
				case m.branch.busy:
					text = "正在切换…"
				case m.branch.err != "":
					text = ""
				case m.branch.listing:
					text = "正在读取分支…"
				case len(rows) == 0:
					text = "没有匹配的分支"
				default:
					text = fmt.Sprintf("%d 个分支", len(rows))
				}
				if text != "" {
					ui.Text(c, text).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
				}
			})
			if m.branch.err != "" {
				ui.Text(c, m.branch.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if len(rows) == 0 {
				return
			}
			ui.Scroll(c).Height(330).Children(func() {
				ui.Column(c).Children(func() {
					for i, b := range rows {
						i, b := i, b
						r := ui.Row(c.Key(fmt.Sprintf("branch:%s", b.Name))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == m.branch.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							color := p.Text
							if b.Remote {
								color = p.TextMuted
							}
							ui.Text(c, b.Name).FontSize(12).TextColor(color).SingleLine().Ellipsis("…")
							if b.Current {
								ui.Text(c, "当前").FontSize(10.5).TextColor(p.Accent).SingleLine()
							}
							if b.Upstream != nil && !b.Remote {
								ui.Text(c, "跟踪 "+*b.Upstream).FontSize(10.5).TextColor(p.TextMuted).
									SingleLine().Ellipsis("…")
							}
							if b.Behind > 0 {
								ui.Text(c, fmt.Sprintf("↓%d", b.Behind)).FontSize(10.5).
									TextColor(p.TextMuted).SingleLine()
							}
							if b.Ahead > 0 {
								ui.Text(c, fmt.Sprintf("↑%d", b.Ahead)).FontSize(10.5).
									TextColor(p.TextMuted).SingleLine()
							}
						})
						if r.Clicked() {
							m.checkoutBranch(b.Name)
						}
					}
				})
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeBranches()
	}
}
