package app

// The workbench shell, ported from OpenIDEA's Islands layout: a 40px title
// bar with the project, VCS, run and search widgets, a tool window stripe
// beside a 240px tool window island, the editor island with its tabs, the
// bottom islands and a 34px status bar on the frame. Geometry and colours are
// the tokens in third_party/openidea/src/styles/tokens.css.

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

const (
	oiTitleBarHeight  float32 = 40
	oiStatusBarHeight float32 = 34
	oiStripeWidth     float32 = 34 // 30px button + 4px outer padding
	oiStripeButton    float32 = 30
	oiStripeGap       float32 = 10 // title bar height minus the button
	oiIslandGap       float32 = 6
	oiIslandRadius    float32 = 10
	oiLeftWidth       float32 = 240
	oiHeaderHeight    float32 = 40
	oiTabBarHeight    float32 = 40
	oiTabHeight       float32 = 28
)

// titleBar is OpenIDEA's compact header: the project widget, the VCS widget,
// then the search, run, debug, terminal controls and the gear.
func (m *Model) titleBar(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(oiTitleBarHeight).Shrink(0).Gap(4).AlignItems(ui.Center).
		Padding(0, 8, 0, 10).Children(func() {
		m.projectWidget(c, p, oi)
		if label := m.branchLabel(); label != "" {
			m.branchWidget(c, p, oi, label)
		}
		ui.Spacer(c)
		m.titleTool(c, p, oi, "search", "Search Everywhere", iconWelcomeSearch, func() {
			m.OpenCommands()
		})
		runOn := m.run.running || m.run.awaiting
		m.titleTool(c, p, oi, "run", "Run", iconRun, func() {
			if runOn {
				m.StopRun()
			} else {
				m.Run()
			}
		})
		debugTip := "Debug"
		if m.debug.session != 0 && !m.debug.ended {
			debugTip = "Show Debug Window"
		}
		m.titleTool(c, p, oi, "debug", debugTip, iconDebug, func() {
			switch {
			case m.debug.session == 0 || m.debug.ended:
				m.Debug()
			case m.debug.paused:
				m.DebugContinue()
			default:
				m.ShowDebug()
			}
		})
		m.titleTool(c, p, oi, "term", "Terminal", iconTerminal, func() { m.ShowTerm() })
		m.gearButton(c, p, oi)
	})
}

// projectWidget is the avatar, the project name and the chevron, with
// OpenIDEA's project menu.
func (m *Model) projectWidget(c *ui.Context, p *ui.Theme, oi oiColors) {
	name := "OpenIDEA"
	if root := m.Root(); root != "" {
		name = filepath.Base(root)
	}
	w := ui.ButtonBase(c.Key("title:project")).Height(30).Padding(0, 8).Gap(6).
		Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if w.Hovered() {
		w.Background(oi.toolbarHover)
	}
	if root := m.Root(); root != "" {
		w.Tooltip(root)
	}
	w.Children(func() {
		start, end := projectColors(name)
		ui.Row(c).Size(20, 20).Shrink(0).Radius(4).
			LinearGradient(ui.LinearGradient{From: start, To: end, Angle: 135}).
			Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Text(c, projectInitials(name)).FontSize(11).Bold().
				TextColor(ui.Hex("#ffffff")).LetterSpacing(0.02).SingleLine()
		})
		ui.Text(c, name).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
		ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(oi.textMuted)
	})
	w.Menu(func(menu *ui.Menu) {
		if menu.Item("Open…").Chosen() {
			m.chooseProject()
		}
		if menu.Item("Recent Projects").Chosen() {
			m.showWelcome()
		}
	})
}

// branchWidget is VCS's branch widget: the branch glyph, the branch name and
// a chevron, opening the branches popup.
func (m *Model) branchWidget(c *ui.Context, p *ui.Theme, oi oiColors, label string) {
	w := ui.ButtonBase(c.Key("title:branch")).Height(30).Padding(0, 8).Gap(4).
		Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if w.Hovered() {
		w.Background(oi.toolbarHover)
	}
	w.Tooltip("Git branches")
	w.Children(func() {
		ui.Icon(c, iconWelcomeBranch).Size(16, 16).TextColor(oi.textMuted)
		ui.Text(c, label).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
		ui.Icon(c, iconChevronDown).Size(14, 14).TextColor(oi.textSubtle)
	})
	if w.Clicked() {
		m.ToggleBranches()
	}
}

// titleTool is one 30px icon control on the right of the title bar.
func (m *Model) titleTool(c *ui.Context, p *ui.Theme, oi oiColors, key, tip string, icon *ui.SVG, onClick func()) {
	b := ui.ButtonBase(c.Key("title:"+key)).Size(oiStripeButton, oiStripeButton).
		Radius(8).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip(tip)
	b.Children(func() {
		ui.Icon(c, icon).Size(20, 20).TextColor(oi.textSecondary)
	})
	if b.Clicked() {
		onClick()
	}
}

// gearButton is IDEA's toolbar gear: the quick switches and settings.
func (m *Model) gearButton(c *ui.Context, p *ui.Theme, oi oiColors) {
	b := ui.ButtonBase(c.Key("title:gear")).Size(oiStripeButton, oiStripeButton).
		Radius(8).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip("IDE and Project Settings")
	b.Children(func() {
		ui.Icon(c, iconWelcomeGear).Size(20, 20).TextColor(oi.textSecondary)
	})
	b.Menu(func(menu *ui.Menu) {
		if menu.Item("Command Palette…").Chosen() {
			m.OpenCommands()
		}
		menu.Separator()
		if menu.Item("Settings…").Chosen() {
			m.OpenSettings()
		}
	})
}

// workbench is the Islands frame: title bar, the stripe and islands of the
// work area, an optional bottom area across their width, and the status bar.
func (m *Model) workbench(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	bottom := m.bottomTool()
	ui.Column(c).Fill().Background(oi.frame).Children(func() {
		m.titleBar(c, p)
		if m.maximized && bottom != "" {
			// A maximized tool window takes the workbench, as OpenIDEA
			// hides the editor island over it.
			m.bottomSlot(c, p)
		} else {
			ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).Gap(oiIslandGap).
				AlignItems(ui.Stretch).Padding(0, 6, 0, 6).Children(func() {
				m.toolStripe(c, p, oi)
				m.leftSide(c, p, oi)
				m.editorIsland(c, p)
			})
			if bottom != "" {
				ui.Row(c).Shrink(0).Padding(6, 6, 0, 6).
					Height(float32(m.bottomSize()) + oiIslandGap).Children(func() {
					m.bottomSlot(c, p)
				})
			}
		}
		m.statusBar(c, p)
	})
}

// showWelcome returns to the Welcome screen without unloading the project,
// which the title bar's project menu and the Welcome actions share.
func (m *Model) showWelcome() {
	m.showingWelcome = true
	m.welcomePage = ""
	m.invalidate()
}

// toolStripe is the left stripe of tool window buttons, icons only, 40px
// wide, its buttons repeating at the title bar's rhythm. Structure and
// Bookmarks belong to the side's split area, as OpenIDEA registers them
// below the Project window.
func (m *Model) toolStripe(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Width(oiStripeWidth).Shrink(0).Padding(5, 0, 6, 4).
		Gap(oiStripeGap).Children(func() {
		m.stripeButton(c, p, oi, "project", iconWelcomeFolder, "Project", m.sideMode == "" && !m.leftHidden, func() {
			m.modeClick("")
		})
		m.stripeButton(c, p, oi, "vcs", iconWelcomeBranch, "Commit", m.sideMode == "vcs" && !m.leftHidden, func() {
			m.modeClick("vcs")
		})
		m.stripeButton(c, p, oi, "structure", iconStructure, "Structure", m.leftSplit == "structure", func() {
			m.splitClick("structure")
		})
		m.stripeButton(c, p, oi, "bookmarks", iconBookmark, "Bookmarks", m.leftSplit == "bookmarks", func() {
			m.splitClick("bookmarks")
		})
		ui.Spacer(c)
		m.stripeButton(c, p, oi, "commands", iconWelcomeSearch, "Search Everywhere", false, func() {
			m.OpenCommands()
		})
	})
}

// modeClick shows a window in the left primary area, or hides that area. The
// split area stays as it was, as OpenIDEA hides one area at a time.
func (m *Model) modeClick(mode string) {
	if m.sideMode == mode && !m.leftHidden {
		m.leftHidden = true
		m.rememberLayout()
		m.invalidate()
		return
	}
	m.leftHidden = false
	m.sideMode = mode
	switch mode {
	case "":
		m.structure.visible = false
	case "vcs":
		m.RefreshVCS()
		m.RefreshBranch()
	}
	m.rememberLayout()
	m.invalidate()
}

// splitClick shows a window in the left split area, or closes it. Showing one
// takes the area from the other, as each area shows a single window.
func (m *Model) splitClick(mode string) {
	if m.leftSplit == mode {
		m.leftSplit = ""
		if mode == "structure" {
			m.structure.visible = false
		}
		m.rememberLayout()
		m.invalidate()
		return
	}
	switch mode {
	case "structure":
		m.ShowStructure()
	case "bookmarks":
		m.ShowBookmarksWindow()
	}
}

func (m *Model) stripeButton(c *ui.Context, p *ui.Theme, oi oiColors, key string, icon *ui.SVG, tip string, active bool, onClick func()) {
	b := ui.ButtonBase(c.Key("stripe:"+key)).Size(oiStripeButton, oiStripeButton).
		Radius(6).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	switch {
	case active && !m.leftHidden:
		b.Background(oi.stripeSelected)
	case b.Hovered():
		b.Background(oi.stripeHover)
	}
	b.Tooltip(tip)
	tone := oi.textMuted
	if active && !m.leftHidden {
		tone = oi.text
	}
	b.Children(func() { ui.Icon(c, icon).Size(20, 20).TextColor(tone) })
	if b.Clicked() {
		onClick()
	}
}

// leftSide is the left side's two areas: the primary island and, below it,
// the split island. A lone island takes the whole side, as OpenIDEA's
// SidePanel does, and hiding both closes the side.
func (m *Model) leftSide(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.leftHidden && m.leftSplit == "" {
		return
	}
	ui.Column(c).Width(oiLeftWidth).Shrink(0).MinHeight(0).Gap(oiIslandGap).
		AlignItems(ui.Stretch).Children(func() {
		if !m.leftHidden {
			m.leftIsland(c, p, oi)
		}
		if m.leftSplit != "" {
			m.leftSplitIsland(c, p, oi)
		}
	})
}

// leftIsland is the tool window showing in the left primary area: its header
// with the actions of the window, then its body.
func (m *Model) leftIsland(c *ui.Context, p *ui.Theme, oi oiColors) {
	title := "Project"
	if m.sideMode == "vcs" {
		title = "Commit"
		if n := len(m.vcs.files); n > 0 {
			title = fmt.Sprintf("Commit · %d", n)
		}
	}
	ui.Column(c).Grow(1).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius).Children(func() {
		ui.Row(c).Height(oiHeaderHeight).Shrink(0).Padding(0, 6, 0, 8).
			Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			if m.sideMode == "vcs" {
				label := "Refresh"
				if m.vcs.loading {
					label = "Loading…"
				}
				ui.Button(c, label).OnClick(func() { m.RefreshVCS() })
			} else if m.tree != nil {
				m.headerIcon(c, p, oi, "tree-expand", "Expand All", iconExpandAll, func() {
					m.tree.ExpandAll(m.Root())
					m.invalidate()
				})
				m.headerIcon(c, p, oi, "tree-collapse", "Collapse All", iconCollapseAll, func() {
					m.tree.CollapseAll()
					m.invalidate()
				})
			}
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if m.sideMode == "vcs" {
				m.vcsRows(c, p)
				return
			}
			if m.tree == nil {
				return
			}
			for _, row := range m.tree.Rows() {
				m.treeRow(c, p, row)
			}
		})
	})
}

// leftSplitIsland is the tool window in the left split area, below the primary
// one: Structure or Bookmarks, with a hide action in its header.
func (m *Model) leftSplitIsland(c *ui.Context, p *ui.Theme, oi oiColors) {
	title := "Structure"
	if m.leftSplit == "bookmarks" {
		title = "Bookmarks"
	}
	ui.Column(c).Grow(1).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius).Children(func() {
		ui.Row(c).Height(oiHeaderHeight).Shrink(0).Padding(0, 6, 0, 8).
			Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			m.headerIcon(c, p, oi, "split-hide", "Hide", iconClose, func() {
				if m.leftSplit == "structure" {
					m.structure.visible = false
				}
				m.leftSplit = ""
				m.rememberLayout()
				m.invalidate()
			})
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if m.leftSplit == "bookmarks" {
				m.bookmarkWindowRows(c, p)
				return
			}
			m.structureRowsUI(c, p)
		})
	})
}

// headerIcon is a 24px header action, which OpenIDEA shows faded until the
// window is under the pointer.
func (m *Model) headerIcon(c *ui.Context, p *ui.Theme, oi oiColors, key, tip string, icon *ui.SVG, onClick func()) {
	b := ui.ButtonBase(c.Key("panel:"+key)).Size(24, 24).Radius(4).
		Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip(tip)
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(oi.textMuted) })
	if b.Clicked() {
		onClick()
	}
}

// editorIsland is the editor surface: the tab bar on top, then the breadcrumb
// chain and the editor.
func (m *Model) editorIsland(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius).Children(func() {
		m.editorTabs(c, p, oi)
		if m.banner != "" {
			m.bannerRow(c, p)
		}
		m.editorArea(c, p)
	})
}

// editorTabs is OpenIDEA's tab bar: 40px high on the island, 28px tabs with
// the selected one tinted and underlined in the accent.
func (m *Model) editorTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	if len(m.tabs) == 0 {
		return
	}
	ui.Row(c).Height(oiTabBarHeight).Shrink(0).Padding(0, 4).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			for i, t := range m.tabs {
				i, t := i, t
				tab := ui.Row(c.Key(fmt.Sprintf("tab:%d", i))).Height(oiTabHeight).
					Padding(0, 10).Gap(6).Radius(6).AlignItems(ui.Center)
				if i == m.active {
					tab.Background(oi.tabSelected).
						BorderWidth(0, 0, 2, 0).BorderColor(oi.accent)
				} else if tab.Hovered() {
					tab.Background(oi.toolbarHover)
				}
				tab.Children(func() {
					name := filepath.Base(t.doc.Path())
					if t.dirty() {
						ui.Text(c, "●").FontSize(9).TextColor(oi.accent)
					}
					ui.Text(c, name).FontSize(13).TextColor(oi.text).SingleLine()
					if t.pinned {
						ui.Icon(c, iconPin).Size(11, 11).TextColor(oi.textMuted)
					}
					close := ui.Row(c).Size(20, 20).Radius(4).
						AlignItems(ui.Center).Justify(ui.Center)
					if close.Hovered() {
						close.Background(oi.hover)
					}
					close.Children(func() {
						ui.Icon(c, iconClose).Size(14, 14).TextColor(oi.textMuted)
					})
					if close.Clicked() {
						m.closeTab(i)
					}
				})
				tab.ContextMenu(func(menu *ui.Menu) { m.tabMenu(menu, t) })
				if tab.Clicked() {
					m.activate(i)
				}
			}
		})
		ui.Row(c).Grow(1)
	})
}

// emptyEditor is IDEA's empty editor: navigation hints on the left of their
// keys, then the drop hint, centred in the island.
func (m *Model) emptyEditor(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	hint := func(key, label, keys string, onClick func()) {
		ui.Row(c.Key("empty:" + key)).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Width(150).Justify(ui.End).Children(func() {
				b := ui.ButtonBase(c).Padding(2, 4).Radius(4).Cursor(ui.CursorPointer)
				tone := oi.textMuted
				if b.Hovered() {
					tone = oi.textSecondary
				}
				b.Children(func() {
					ui.Text(c, label).FontSize(13).TextColor(tone).SingleLine()
				})
				if b.Clicked() {
					onClick()
				}
			})
			ui.Text(c, keys).FontSize(13).TextColor(oi.link).SingleLine()
		})
	}
	ui.Column(c).Grow(1).FillHeight().AlignItems(ui.Center).Justify(ui.Center).
		Padding(24, 24).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			hint("search", "Search Everywhere", "⌘⇧P", func() { m.OpenCommands() })
			hint("goto", "Go to File", "⌘P", func() { m.OpenQuickFiles() })
			ui.Row(c).Padding(0, 0, 0, 0).Children(func() {
				ui.Text(c, "Drop files here to open them").FontSize(13).
					TextColor(oi.textFaint).SingleLine()
			})
		})
	})
}

// bottomTool names the window the bottom area shows, if any.
func (m *Model) bottomTool() string {
	if m.primaryTool() != "" {
		return m.primaryTool()
	}
	return m.splitTool()
}

// bottomIsland wraps the bottom tool windows: their own panels already draw
// a header and body, so the island adds OpenIDEA's surface and gap.
func (m *Model) bottomIsland(c *ui.Context, p *ui.Theme, oi oiColors, tool string) {
	height := float32(m.bottomSize())
	if m.maximized {
		height = 0
	}
	row := ui.Row(c).Shrink(0).Gap(oiIslandGap).Children(func() {
		m.bottomSlot(c, p)
	})
	if height > 0 {
		row.Height(height)
	} else {
		row.Grow(1)
	}
}

// statusBar is OpenIDEA's 34px status bar on the frame: plain text items, the
// caret and encoding on the right.
func (m *Model) statusBar(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(oiStatusBarHeight).Shrink(0).Padding(0, 10, 0, 12).
		Gap(4).AlignItems(ui.Center).Children(func() {
		if t := m.tab(); t != nil {
			ui.Text(c, shortenHome(t.doc.Path())).FontSize(13).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		} else if m.status != "" {
			ui.Text(c, m.status).FontSize(13).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		}
		ui.Spacer(c)
		if m.run.visible {
			text := ""
			tone := oi.textMuted
			switch {
			case m.run.running:
				text = "Running"
			case m.run.exitCode != nil && *m.run.exitCode == 0:
				text = "Done"
			case m.run.exitCode != nil:
				text = fmt.Sprintf("Exit code %d", *m.run.exitCode)
				tone = oi.warning
			}
			if text != "" {
				ui.Text(c, text).FontSize(13).TextColor(tone).SingleLine()
			}
		}
		m.problemsStatus(c, p)
		if t := m.tab(); t != nil {
			pos := t.state.CursorPos()
			ui.Text(c, fmt.Sprintf("Ln %d, Col %d", pos.Line+1, pos.Col+1)).
				FontSize(13).TextColor(oi.textMuted).SingleLine()
		}
		info := "UTF-8"
		if t := m.tab(); t != nil {
			if enc := t.doc.Info().Encoding; enc != "" {
				info = enc
			}
		}
		ui.Text(c, info).FontSize(13).TextColor(oi.textMuted).SingleLine()
		state := "openidea-server connected"
		if !m.BackendState().Running {
			state = "openidea-server offline"
		}
		ui.Text(c, state).FontSize(13).TextColor(oi.textMuted).SingleLine()
	})
}

// tabMenu is OpenIDEA's tab context menu: the close family, whose bulk closes
// leave pinned tabs alone, then Pin or Unpin Tab for the clicked tab.
func (m *Model) tabMenu(menu *ui.Menu, t *Tab) {
	at := m.tabIndex(t)
	if menu.Item("Close").Chosen() {
		m.closeTabThen(t, nil)
	}
	if menu.Item("Close Other Tabs").Disabled(len(m.tabs) < 2).Chosen() {
		m.CloseOtherTabs(t)
	}
	if menu.Item("Close Tabs to the Left").Disabled(at <= 0).Chosen() {
		m.CloseTabsToLeft(t)
	}
	if menu.Item("Close Tabs to the Right").Disabled(at < 0 || at >= len(m.tabs)-1).Chosen() {
		m.CloseTabsToRight(t)
	}
	if menu.Item("Close All Tabs").Chosen() {
		m.CloseAllTabs(false)
	}
	if menu.Item("Close Unmodified Tabs").Chosen() {
		m.CloseUnmodifiedTabs()
	}
	if m.anyPinned() {
		if menu.Item("Close All but Pinned").Chosen() {
			m.CloseAllTabs(true)
		}
	}
	if m.anyReadOnlyUnpinned() {
		if menu.Item("Close All Read-Only Tabs").Chosen() {
			m.CloseReadOnlyTabs()
		}
	}
	menu.Separator()
	label := "Pin Tab"
	if t.pinned {
		label = "Unpin Tab"
	}
	if menu.Item(label).Chosen() {
		m.setTabPinned(t, !t.pinned)
	}
}
