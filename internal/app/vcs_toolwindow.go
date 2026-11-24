package app

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// vcsContentTabs is the native content strip of the Git tool window. Its
// selection is kept per repository, so inspecting the log in one repository
// does not move another repository back to Local Changes.
func (m *Model) vcsContentTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	type contentTab struct {
		tab   vcsContentTab
		label string
		count int
	}
	tabs := []contentTab{
		{vcsContentChanges, "Local Changes", 0},
		{vcsContentBranches, "Branches", 0},
	}
	if m.vcsGitBool("vcs.git.combineStashAndShelf") {
		tabs = append(tabs, contentTab{
			vcsContentStash, "Stashes and Shelves", m.vcsShelfCount() + m.vcsStashCount(),
		})
	} else {
		tabs = append(tabs,
			contentTab{vcsContentShelf, "Shelf", m.vcsShelfCount()},
			contentTab{vcsContentStash, "Stash", m.vcsStashCount()},
		)
	}
	tabs = append(tabs,
		contentTab{vcsContentLog, "Log", 0},
		contentTab{vcsContentWorktrees, "Worktrees", 0},
	)
	active := m.currentVCSContent()
	combineStashAndShelf := m.vcsGitBool("vcs.git.combineStashAndShelf")
	if combineStashAndShelf && active == vcsContentShelf {
		active = vcsContentStash
	}
	ui.Row(c).Height(30).Shrink(0).Padding(0, 6).Gap(2).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		for _, item := range tabs {
			item := item
			row := ui.Row(c.Key("vcs:content:"+item.label)).Height(26).Padding(0, 9).
				Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			switch {
			case item.tab == active:
				row.Background(oi.stripeSelected)
			case row.Hovered():
				row.Background(oi.hoverTint)
			}
			row.Children(func() {
				ui.Text(c, item.label).FontSize(12).
					TextColor(boolColor(item.tab == active, oi.textStrong, oi.textSubtle))
				if item.count > 0 {
					ui.Text(c, fmt.Sprint(item.count)).FontSize(10).TextColor(oi.textSubtle)
				}
			})
			if row.Clicked() {
				m.SelectVCSTab(item.tab)
			}
		}
		ui.Spacer(c)
		if m.vcs.loading {
			ui.Text(c, "Refreshing…").FontSize(11).TextColor(oi.textSubtle)
		}
	})
}

func (m *Model) vcsToolWindowContent(c *ui.Context, p *ui.Theme, oi oiColors) {
	switch m.currentVCSContent() {
	case vcsContentBranches:
		m.vcsBranchesContent(c, p, oi)
	case vcsContentShelf:
		if m.vcsGitBool("vcs.git.combineStashAndShelf") {
			m.vcsCombinedStashAndShelfContent(c, p, oi)
		} else {
			m.vcsShelfContent(c, p, oi)
		}
	case vcsContentStash:
		if m.vcsGitBool("vcs.git.combineStashAndShelf") {
			m.vcsCombinedStashAndShelfContent(c, p, oi)
		} else {
			m.vcsStashContent(c, p, oi)
		}
	case vcsContentLog:
		m.vcsLogContent(c, p)
	case vcsContentWorktrees:
		m.vcsWorktreeContent(c, p, oi)
	default:
		m.vcsChangesContent(c, p)
	}
}

func (m *Model) vcsCombinedStashAndShelfContent(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			settingsSectionTitle(c, oi, "Stashes")
			m.vcsStashContent(c, p, oi)
		})
		ui.Row(c).Height(1).Shrink(0).Background(p.Border)
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			settingsSectionTitle(c, oi, "Shelves")
			m.vcsShelfContent(c, p, oi)
		})
	})
}

func (m *Model) vcsChangesContent(c *ui.Context, p *ui.Theme) {
	m.vcsToolbar(c, p)
	if m.commit != nil && m.vcsCommitNonModal() {
		m.vcsInlineCommit(c, p)
	}
	ui.Row(c).Height(34).Shrink(0).Padding(6, 8).Children(func() {
		if m.vcs.changeQueries == nil {
			m.vcs.changeQueries = map[string]string{}
		}
		root := vcsRepositoryKey(m.vcs.current)
		query := m.vcs.changeQueries[root]
		input := ui.TextInput(c.Key("vcs:changes:filter:"+root), &query)
		input.Grow(1).Placeholder("Filter changed files")
		input.OnChange(func() {
			m.vcs.changeQueries[root] = query
			m.invalidate()
		})
	})
	scroll := ui.ScrollBoth(c).Key("vcs:changes:scroll").Grow(1).MinHeight(0).
		TrackScroll(&m.treeScroll)
	scroll.Children(func() {
		if m.vcs.loading && !m.vcs.loaded {
			ui.Text(c, "Reading Git status…").Padding(8).FontSize(12).TextColor(p.TextMuted)
			return
		}
		m.vcsRepositoryRows(c, p)
	})
}

func (m *Model) vcsInlineCommit(c *ui.Context, p *ui.Theme) {
	s := m.commit
	if s == nil {
		return
	}
	ui.Column(c).Shrink(0).Padding(8).Gap(6).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "Commit Changes").FontSize(12).Bold().TextColor(p.Text)
		m.commitMessageEditor(c, p, s, true)
		ui.Row(c).Gap(12).Children(func() {
			m.commitOptions(c, s)
		})
		m.commitFooter(c, p, s, len(m.commitFiles(s)) == 0)
	})
}

func (m *Model) vcsLogContent(c *ui.Context, p *ui.Theme) {
	l := &m.log
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Button(c, "Open Git Log").Size(88, 24).OnClick(m.ShowLog)
		ui.Button(c, "Refresh").Size(58, 24).Disabled(l.loading).OnClick(m.RefreshLog)
		ui.Button(c, "Branches").Size(68, 24).
			OnClick(func() { m.SelectVCSTab(vcsContentBranches) })
	})
	if l.err != "" {
		ui.Text(c, l.err).Padding(8).FontSize(12).TextColor(p.Warning)
		return
	}
	if l.loading && !l.loaded {
		ui.Text(c, "Reading the commit history…").Padding(8).FontSize(12).
			TextColor(p.TextMuted)
		return
	}
	if len(l.commits) == 0 {
		ui.Text(c, "No commits").Padding(8).FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Height(210).Shrink(0).Children(func() {
		ui.Column(c).Padding(2, 0).Children(func() {
			for i, commit := range l.commits {
				i, commit := i, commit
				row := ui.Row(c.Key(fmt.Sprintf("vcs-log:%d", i))).Height(26).
					Padding(0, 8).Gap(6).AlignItems(ui.Center)
				if i == l.selected {
					row.Background(p.Selection)
				} else if row.Hovered() {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					if len(l.commitRepos) > 0 {
						ui.Text(c, m.vcsRepositoryLabelForRoot(m.logCommitRepository(i))).
							FontSize(10).TextColor(p.TextMuted).SingleLine().
							Ellipsis("…")
					}
					ui.Text(c, commit.Summary).FontSize(12).TextColor(p.Text).
						SingleLine().Ellipsis("…").Grow(1)
					ui.Text(c, commit.ShortID).FontSize(11).Font("monospace").
						TextColor(p.TextMuted)
				})
				row.ContextMenu(func(menu *ui.Menu) {
					m.gitLogCommitMenu(menu, m.logCommitTarget(i))
				})
				if row.Clicked() && i != l.selected {
					m.selectCommit(i)
				}
			}
		})
	})
	ui.Row(c).Height(1).Background(p.Border)
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 6).Children(func() {
		m.logDetails(c, p)
	})
}

func (m *Model) vcsWorktreeContent(c *ui.Context, p *ui.Theme, oi oiColors) {
	root := m.vcs.current
	state := m.vcsWorktreeState(root)
	canOpen := !state.loading && state.selected >= 0 &&
		state.selected < len(state.worktrees)
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(3).AlignItems(ui.Center).
		BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Button(c, "Open").Size(50, 24).Disabled(!canOpen).OnClick(m.openSelectedVCSWorktree)
		ui.Button(c, "New…").Size(48, 24).Disabled(state.loading).OnClick(m.OpenNewVCSWorktree)
		ui.Button(c, "Manage…").Size(66, 24).Disabled(state.loading).OnClick(m.OpenVCSWorktrees)
		ui.Button(c, "Refresh").Size(58, 24).Disabled(state.loading).
			OnClick(func() { m.refreshVCSWorktrees(root) })
	})
	if state.err != "" {
		ui.Text(c, state.err).Padding(8).FontSize(12).TextColor(p.Danger)
		return
	}
	if state.loading && !state.loaded {
		ui.Text(c, "Loading worktrees…").Padding(8).FontSize(12).TextColor(p.TextMuted)
		return
	}
	if len(state.worktrees) == 0 {
		ui.Text(c, "No worktrees").Padding(8).FontSize(12).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(2, 0).Children(func() {
			for i, worktree := range state.worktrees {
				i, worktree := i, worktree
				row := ui.Row(c.Key("vcs-toolwindow-worktree:"+worktree.Path)).Height(42).
					Padding(0, 8).Gap(7).AlignItems(ui.Center)
				if i == state.selected {
					row.Background(p.Selection)
				} else if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				row.Children(func() {
					oiIconEl(c, "general/vcs", 14, oi, oi.icon)
					ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
						label := filepath.Base(worktree.Path)
						if worktree.Branch != nil && *worktree.Branch != "" {
							label = *worktree.Branch
						} else if worktree.Head != nil && *worktree.Head != "" {
							label = shortID(*worktree.Head)
						}
						ui.Text(c, label).FontSize(12).TextColor(oi.textStrong).
							SingleLine().Ellipsis("…")
						ui.Text(c, worktree.Path).FontSize(10).TextColor(oi.textSubtle).
							SingleLine().Ellipsis("…")
					})
					if worktree.Main {
						ui.Text(c, "main").FontSize(10).TextColor(oi.textSubtle)
					}
				})
				if row.Clicked() {
					m.selectVCSWorktree(i)
				}
				if row.DoubleClicked() {
					m.selectVCSWorktree(i)
					m.openSelectedVCSWorktree()
				}
			}
		})
	})
}

func boolColor(selected bool, on, off ui.Color) ui.Color {
	if selected {
		return on
	}
	return off
}
