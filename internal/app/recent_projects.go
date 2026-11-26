package app

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// recentProjectsPopup is File › Open Recent's compact 560 DIP chooser. It
// mirrors OpenIDEA's RecentProjectsPopup: the current project is hidden, the
// search field owns focus, arrows move through the compact 40 DIP rows, and
// Enter or a click opens the selected project.
type recentProjectsPopup struct {
	query  string
	index  int
	scroll ui.ScrollState
}

func (m *Model) showRecentProjectsPopup() {
	m.recentProjectsPopup = &recentProjectsPopup{}
	m.invalidate()
}

func (m *Model) closeRecentProjectsPopup() {
	m.recentProjectsPopup = nil
	m.invalidate()
}

func (m *Model) recentProjectsMatches() []recentMatch {
	if m.recentProjectsPopup == nil {
		return nil
	}
	return m.filteredRecentsFor(m.recentProjectsPopup.query, m.Root())
}

func (m *Model) stepRecentProjects(delta int) {
	p := m.recentProjectsPopup
	if p == nil {
		return
	}
	count := len(m.recentProjectsMatches())
	if count == 0 {
		p.index = 0
		m.invalidate()
		return
	}
	p.index = (p.index + delta) % count
	if p.index < 0 {
		p.index += count
	}
	top := float32(p.index) * 40
	p.scroll.Y = min(max(p.scroll.Y, top+40-320), top)
	m.invalidate()
}

func (m *Model) chooseRecentProjectsPopup() {
	p := m.recentProjectsPopup
	if m == nil || p == nil {
		return
	}
	list := m.recentProjectsMatches()
	if len(list) == 0 {
		return
	}
	p.index = min(max(p.index, 0), len(list)-1)
	path := list[p.index].project.Path
	m.closeRecentProjectsPopup()
	m.OpenRecentProject(path)
}

func (m *Model) removeSelectedRecentProject() {
	p := m.recentProjectsPopup
	if p == nil {
		return
	}
	list := m.recentProjectsMatches()
	if len(list) == 0 {
		return
	}
	p.index = min(max(p.index, 0), len(list)-1)
	m.askRemoveRecent(list[p.index].project.Path)
}

func (m *Model) recentProjectsDialog(c *ui.Context, theme *ui.Theme) {
	p := m.recentProjectsPopup
	if p == nil {
		return
	}
	oi := oiPaletteOf(theme)
	open := true
	ui.DialogBase(c.Key("recent-projects"), &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.12)).Justify(ui.Start).Padding(40, 0).
			AlignItems(ui.Center)
		panel.Width(560).MaxHeight(512).Padding(0).Radius(10).
			Background(oi.elev).Border(1, oi.border).
			Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3))
		panel.Children(func() {
			ui.Row(c).Height(40).Shrink(0).Padding(0, 14).
				BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
				AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Recent Projects").FontSize(13).Bold().
					TextColor(oi.text).SingleLine()
			})
			ui.Row(c).Padding(8, 10).Shrink(0).AlignItems(ui.Center).Children(func() {
				oiIconEl(c, "general/search", 14, oi, oi.textSubtle).MarginEnd(6)
				in := ui.TextInput(c, &p.query).Grow(1).AutoFocus().
					Placeholder("Search projects")
				in.OnChange(func() {
					p.index = 0
					p.scroll.Y = 0
					m.invalidate()
				})
				in.OnShortcut(0, ui.KeyUp, func() { m.stepRecentProjects(-1) })
				in.OnShortcut(0, ui.KeyDown, func() { m.stepRecentProjects(1) })
				in.OnShortcut(0, ui.KeyPageUp, func() { m.stepRecentProjects(-8) })
				in.OnShortcut(0, ui.KeyPageDown, func() { m.stepRecentProjects(8) })
				in.OnShortcut(0, ui.KeyDelete, m.removeSelectedRecentProject)
				in.OnShortcut(ui.Cmd, ui.KeyBackspace, m.removeSelectedRecentProject)
				if in.Submitted() {
					m.chooseRecentProjectsPopup()
				}
			})
			ui.Scroll(c).TrackScroll(&p.scroll).MaxHeight(400).Children(func() {
				m.recentProjectsRows(c, oi, p)
			})
		})
	})
	if !open {
		m.closeRecentProjectsPopup()
	}
}

func (m *Model) recentProjectsRows(c *ui.Context, oi oiColors, p *recentProjectsPopup) {
	list := m.recentProjectsMatches()
	if len(list) == 0 {
		text := "No recent projects"
		if p.query != "" {
			text = "No matching projects"
		}
		ui.Text(c, text).Padding(12, 14).FontSize(12).TextColor(oi.textSubtle)
		return
	}
	p.index = min(max(p.index, 0), len(list)-1)
	for i, match := range list {
		i, match := i, match
		project := match.project
		name := project.Name
		if name == "" {
			name = filepath.Base(project.Path)
		}
		row := ui.Row(c.Key("recent-project:"+project.Path)).Height(40).
			Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(6)
		switch {
		case i == p.index:
			row.Background(oi.selected)
		case row.Hovered():
			row.Background(oi.hover)
		}
		moreClicked := false
		row.Children(func() {
			recentProjectTile(c, name)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					m.welcomeName(c, oi, name, match.nameIdx)
					if branch := m.branchOf(project.Path); branch != "" {
						ui.Spacer(c)
						oiIconEl(c, "general/vcs", 11, oi, oi.textSubtle)
						ui.Text(c, branch).FontSize(11).TextColor(oi.textSubtle).SingleLine()
					}
				})
				ui.Text(c, shortenHome(project.Path)).FontSize(11).
					TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
			})
			more := ui.ButtonBase(c.Key("recent-project-more:"+project.Path)).
				Size(24, 24).Radius(6).AlignSelf(ui.Center).
				Justify(ui.Center).Cursor(ui.CursorPointer)
			if i == p.index || row.Hovered() {
				if more.Hovered() {
					more.Background(oi.hoverTint)
				}
				more.Children(func() {
					oiIconEl(c, "general/moreVertical", 14, oi, oi.icon)
				})
				more.Menu(func(menu *ui.Menu) {
					m.welcomeRecentMenu(menu, project.Path, m.closeRecentProjectsPopup)
				})
				if more.Clicked() {
					moreClicked = true
				}
			}
		})
		row.OnClick(func() {
			if !moreClicked {
				p.index = i
				m.chooseRecentProjectsPopup()
			}
		})
		if row.DoubleClicked() {
			p.index = i
			m.chooseRecentProjectsPopup()
		}
		row.ContextMenu(func(menu *ui.Menu) {
			p.index = i
			m.welcomeRecentMenu(menu, project.Path, m.closeRecentProjectsPopup)
		})
	}
}
