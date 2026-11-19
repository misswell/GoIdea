package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (m *Model) recentNavigationShortcuts(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyE) {
		m.RunCommand("navigate.recentFiles")
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyE) {
		m.RunCommand("navigate.recentLocations")
	}
}

func (m *Model) stepRecentNavigation(step int) {
	p := m.recentNav.popup
	if count := p.count(); count > 0 {
		p.index = ((p.index+step)%count + count) % count
		if !p.onTools {
			height := float32(24)
			if p.locations {
				height = 84
			}
			top := float32(p.index) * height
			p.scroll.Y = min(max(p.scroll.Y, top+height-336), top)
		}
		m.invalidate()
	}
}

func (m *Model) recentNavigationDialog(c *ui.Context, theme *ui.Theme) {
	p := m.recentNav.popup
	if p == nil {
		return
	}
	oi := oiPaletteOf(theme)
	title, filter, toggle := "Recent Files", "Filter recent files", "Show edited only"
	if p.locations {
		title, filter, toggle = "Recent Locations", "Type to filter by file name or code", "Show changed only"
		if p.onlyChanged {
			title = "Recently Changed Locations"
		}
	} else if p.onlyChanged {
		title = "Recently Edited Files"
	}
	open := true
	dialog := ui.Modal(c.Key("recent-navigation"), &open, func() {
		ui.Column(c).Width(720).Children(func() {
			ui.Row(c).Padding(8, 12).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(13).Bold().SingleLine()
				in := ui.TextInput(c, &p.query).Grow(1).AutoFocus().Placeholder(filter)
				in.OnShortcut(ui.Cmd, ui.KeyE, func() { m.RunCommand("navigate.recentFiles") })
				in.OnShortcut(ui.Cmd|ui.Shift, ui.KeyE, func() { m.RunCommand("navigate.recentLocations") })
				in.OnChange(func() { m.refreshRecentNavigation(true) })
				in.OnShortcut(0, ui.KeyUp, func() { m.stepRecentNavigation(-1) })
				in.OnShortcut(0, ui.KeyDown, func() { m.stepRecentNavigation(1) })
				in.OnShortcut(0, ui.KeyPageUp, func() { m.stepRecentNavigation(-5) })
				in.OnShortcut(0, ui.KeyPageDown, func() { m.stepRecentNavigation(5) })
				if p.locations {
					in.OnShortcut(0, ui.KeyDelete, m.removeRecentPlace)
					if p.query == "" {
						in.OnShortcut(0, ui.KeyBackspace, m.removeRecentPlace)
					}
				} else {
					in.OnShortcut(0, ui.KeyTab, func() {
						if (p.onTools && len(p.files) > 0) || (!p.onTools && len(p.tools) > 0) {
							p.onTools, p.index = !p.onTools, 0
							p.scroll.Y = 0
							m.invalidate()
						}
					})
				}
				if in.Submitted() {
					m.chooseRecentNavigation()
				}
				if ui.Checkbox(c, &p.onlyChanged, toggle).Changed() {
					m.refreshRecentNavigation(true)
				}
			})
			if p.locations {
				m.recentLocationRows(c, p, oi)
			} else {
				m.recentFileRows(c, p, oi)
			}
		})
	})
	dialog.Padding(0)
	if !open {
		m.closeRecentNavigation()
	}
}

func recentNavigationRow(c *ui.Context, id string, height float32, oi oiColors, selected bool) ui.Element {
	row := ui.Row(c.Key(id)).Height(height).Padding(0, 10).Gap(8).AlignItems(ui.Center)
	if selected {
		row.Background(oi.selected)
	} else if row.Hovered() {
		row.Background(oi.hover)
	}
	return row
}

func (m *Model) recentFileRows(c *ui.Context, p *recentNavPopup, oi oiColors) {
	ui.Row(c).Height(336).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(196).BorderWidth(0, 1, 0, 0).BorderColor(oi.border).Children(func() {
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				for i, spec := range p.tools {
					row := recentNavigationRow(c, spec.id, 24, oi, p.onTools && p.index == i)
					row.OnClick(func() {
						p.onTools, p.index = true, i
						m.invalidate()
					})
					if row.DoubleClicked() {
						p.onTools, p.index = true, i
						m.chooseRecentNavigation()
					}
					row.Children(func() {
						oiIconEl(c, spec.icon, 14, oi, oi.textMuted)
						ui.Text(c, spec.title).FontSize(13).SingleLine()
					})
				}
			})
			ui.Button(c, "Recent Locations").OnClick(func() { m.openRecentNavigation(true) })
		})
		ui.Scroll(c).TrackScroll(&p.scroll).Grow(1).MinWidth(0).Children(func() {
			if len(p.files) == 0 {
				label := "No recent files"
				switch {
				case p.loading:
					label = "Loading recent files…"
				case p.onlyChanged:
					label = "No edited files"
				case p.query != "":
					label = "No matching files"
				}
				ui.Text(c, label).Padding(10).FontSize(13).TextColor(oi.textMuted)
			}
			for i, path := range p.files {
				row := recentNavigationRow(c, path, 24, oi, !p.onTools && p.index == i)
				row.OnClick(func() {
					p.onTools, p.index = false, i
					m.invalidate()
				})
				if row.DoubleClicked() {
					p.onTools, p.index = false, i
					m.chooseRecentNavigation()
				}
				row.Children(func() {
					name := filepath.Base(path)
					fileIconEl(c, oi, name, 14, oi.textMuted)
					ui.Text(c, name).FontSize(13).SingleLine()
					if dir := filepath.Dir(projectRelative(m.Root(), path)); dir != "." {
						ui.Text(c, dir).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
					}
				})
			}
		})
	})
	footer := m.Root()
	if !p.onTools && p.index < len(p.files) {
		footer = p.files[p.index]
	}
	if p.err != "" {
		footer = p.err
	}
	ui.Text(c, footer).Padding(6, 12).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
}

func (m *Model) recentLocationRows(c *ui.Context, p *recentNavPopup, oi oiColors) {
	ui.Scroll(c).TrackScroll(&p.scroll).Height(336).Children(func() {
		if len(p.places) == 0 {
			text := "No recent locations"
			if p.onlyChanged {
				text = "No changed locations"
			}
			ui.Text(c, text).Padding(10).FontSize(13).TextColor(oi.textMuted)
		}
		for i, place := range p.places {
			key := fmt.Sprintf("%s:%d", place.path, place.pos.Line)
			row := recentNavigationRow(c, key, 84, oi, p.index == i)
			row.OnClick(func() {
				p.index = i
				m.invalidate()
			})
			if row.DoubleClicked() {
				p.index = i
				m.chooseRecentNavigation()
			}
			row.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						name := filepath.Base(place.path)
						fileIconEl(c, oi, name, 14, oi.textMuted)
						ui.Text(c, name).FontSize(13).SingleLine()
						rel := projectRelative(m.Root(), place.path)
						ui.Text(c, fmt.Sprintf("%s:%d", rel, place.pos.Line+1)).
							Grow(1).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
						if place.changed {
							ui.Text(c, "changed").FontSize(12).TextColor(oi.textMuted)
						}
						ui.Text(c, recentPlaceTime(place.at, p.now)).FontSize(12).TextColor(oi.textMuted)
					})
					for j, text := range place.lines {
						line := place.firstLine + j
						code := ui.Text(c, fmt.Sprintf("%4d  %s", line+1, strings.TrimRight(text, "\r\n"))).
							Font(oiFontMono).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
						if line == place.pos.Line {
							code.TextColor(oi.text)
						}
					}
				})
			})
		}
	})
}

func recentPlaceTime(at, now time.Time) string {
	age := now.Sub(at)
	switch {
	case age < time.Minute:
		return "Just now"
	case age < time.Hour:
		minutes := int(age.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	default:
		return at.Format("Jan 2 15:04")
	}
}
