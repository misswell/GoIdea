package app

// The workbench shell, ported from OpenIDEA's Islands layout: a 40px title
// bar with the project, VCS, run and search widgets, a tool window stripe
// beside a 240px tool window island, the editor island with its tabs, the
// bottom islands and a 34px status bar on the frame. Geometry and colours are
// the tokens in third_party/openidea/src/styles/tokens.css.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	oiTitleBarHeight    float32 = 40
	oiStatusBarHeight   float32 = 34
	oiStripeWidth       float32 = 40 // 30px button + 4px padding + the island gap
	oiStripeButton      float32 = 30
	oiStripeGap         float32 = 10 // title bar height minus the button
	oiStripeNamedBar    float32 = 66
	oiStripeNamedButton float32 = 56
	oiStripeNamedHeight float32 = 44
	oiIslandGap         float32 = 6
	oiIslandRadius      float32 = 10
	oiLeftWidth         float32 = 240
	// rightWidthDefault and its stored range are the right area's width, as
	// OpenIDEA's sizes.right.
	rightWidthDefault     float32 = 300
	rightWidthStoredMin           = 200
	rightWidthStoredMax           = 600
	oiHeaderHeight        float32 = 40
	oiNavigationBarHeight float32 = 26
	oiTabBarHeight        float32 = 40
	oiTabHeight           float32 = 28
	oiTabColumnMin        float32 = 140 // OpenIDEA's vertical strip is 140..260 wide
	oiTabColumnMax        float32 = 260
	oiTabColumnTab        float32 = 24
)

// titleBar is OpenIDEA's compact header: the project widget, the VCS widget,
// then the run widget, the search and terminal controls and the gear.
func (m *Model) titleBar(c *ui.Context, p *ui.Theme) {
	if m.prefs.Appearance.HideMainToolbar {
		m.compactTitleBar(c, p)
		return
	}
	oi := oiPaletteOf(p)
	tk := oiTokensFor(m, p)
	room := c.TitleBar()
	height := max(tk.TitleBarHeight, room.Height)
	titleLeft := room.Left + 10
	titleRight := room.Right + 8
	left, right := resolveToolbar(m.prefs.MainToolbar)
	bar := ui.Row(c).Height(height).Shrink(0).Gap(4).AlignItems(ui.Center).
		Padding(0, titleRight, 0, titleLeft).DragWindow()
	bar.ContextMenu(func(menu *ui.Menu) { m.toolbarCustomizeItems(menu) })
	bar.Children(func() {
		for _, id := range left {
			m.toolbarEntry(c, p, oi, id)
		}
		ui.Spacer(c)
		for _, id := range right {
			m.toolbarEntry(c, p, oi, id)
		}
		m.gearButton(c, p, oi)
	})
}

// compactTitleBar is IDEA's 30px compact header used when View › Appearance ›
// Toolbar is off. It keeps the active document title and the way back to the
// toolbar, instead of removing the title bar entirely.
func (m *Model) compactTitleBar(c *ui.Context, p *ui.Theme) {
	title := m.projectLabel()
	if t := m.tab(); t != nil && t.kind == "" && t.doc != nil && t.doc.Path() != "" {
		title = filepath.Base(t.doc.Path())
		if m.prefs.Appearance.FullPathInTitle {
			title = t.doc.Path()
		}
		if project := m.projectLabel(); project != "" {
			title += " – " + project
		}
	}
	room := c.TitleBar()
	bar := ui.Row(c).Height(max(float32(30), room.Height)).Shrink(0).
		AlignItems(ui.Center).Justify(ui.Center).
		Padding(0, max(float32(72), room.Right+8), 0, max(float32(72), room.Left+10)).
		DragWindow()
	bar.ContextMenu(func(menu *ui.Menu) { m.toolbarCustomizeItems(menu) })
	bar.Children(func() {
		ui.Text(c, title).FontSize(13).TextColor(p.Text).SingleLine().Ellipsis("…")
	})
}

// toolbarEntry draws one title bar entry of the saved layout. A widget this
// build has no contribution for is skipped, and an action entry draws the
// action's title, so the customization can add any registered action.
func (m *Model) toolbarEntry(c *ui.Context, p *ui.Theme, oi oiColors, id string) {
	if w := titleWidgetFor(id); w != nil {
		if w.Visible != nil && !w.Visible(m) {
			return
		}
		w.Render(m, c, p, oi)
		return
	}
	if isToolbarAction(id) {
		m.actionTitleTool(c, p, oi, toolbarAction(id))
	}
}

// actionTitleTool is an action the customization added that has no icon of its
// own: a compact text button running the same registry action as every other
// entrance. An action whose context is closed is not drawn, so the toolbar
// never shows a dead control.
func (m *Model) actionTitleTool(c *ui.Context, p *ui.Theme, oi oiColors, id string) {
	title := commandTitle(id)
	if title == "" || !m.ActionEnabled(id) {
		return
	}
	tk := oiTokensFor(m, p)
	b := ui.ButtonBase(c.Key("title:"+id)).Height(tk.ButtonHeight).Padding(0, tk.SpaceL).
		Radius(tk.RadiusM).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip(title)
	b.Children(func() {
		ui.Text(c, title).FontSize(tk.FontSizeM).TextColor(oi.text).SingleLine().Ellipsis("…")
	})
	if b.Clicked() {
		m.RunAction(id)
	}
}

// projectWidget is the avatar, the project name and the chevron, with
// OpenIDEA's project menu.
func (m *Model) projectWidget(c *ui.Context, p *ui.Theme, oi oiColors) {
	name := m.projectLabel()
	tk := oiTokensFor(m, p)
	w := ui.ButtonBase(c.Key("title:project")).Height(tk.StripeButton).Padding(0, tk.SpaceL).Gap(tk.SpaceM).
		Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Projects")
	if w.Hovered() {
		w.Background(oi.toolbarHover)
	}
	if root := m.Root(); root != "" {
		w.Tooltip(root)
	}
	w.Children(func() {
		size := tk.StripeButton - tk.SpaceM
		avatar := ui.Row(c).Size(size, size).Shrink(0).Radius(4).
			Justify(ui.Center).AlignItems(ui.Center)
		initialColor := ui.Hex("#ffffff")
		if m.prefs.projectColorsInToolbar() {
			start, end := projectColors(name)
			avatar.LinearGradient(ui.LinearGradient{From: start, To: end, Angle: 135})
		} else {
			avatar.Background(oi.elev)
			initialColor = oi.text
		}
		avatar.Children(func() {
			ui.Text(c, projectInitials(name)).FontSize(11).Bold().
				TextColor(initialColor).LetterSpacing(0.02).SingleLine()
		})
		ui.Text(c, name).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
		oiIconEl(c, "general/chevronDown", 12, oi, oi.textMuted)
	})
	if w.Clicked() {
		m.openProjectPopup(w.Bounds())
	}
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
		oiIconEl(c, "general/vcs", 16, oi, oi.icon)
		ui.Text(c, label).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
		oiIconEl(c, "general/chevronDown", 14, oi, oi.textSubtle)
	})
	if w.Clicked() {
		m.ToggleBranches()
	}
}

// vcsFallbackWidget is "Version Control ▾", the title bar's Git menu while the
// project has no repository, as OpenIDEA shows while its VCS widget is absent.
func (m *Model) vcsFallbackWidget(c *ui.Context, p *ui.Theme, oi oiColors) {
	w := ui.ButtonBase(c.Key("title:vcs")).Height(30).Padding(0, 8).Gap(4).
		Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if w.Hovered() {
		w.Background(oi.toolbarHover)
	}
	w.Tooltip("Version Control")
	w.Children(func() {
		ui.Text(c, "Version Control").FontSize(13).TextColor(oi.textStrong).SingleLine()
		oiIconEl(c, "general/chevronDown", 12, oi, oi.textSubtle)
	})
	w.Menu(func(menu *ui.Menu) {
		if menu.Item("Clone…").Chosen() {
			m.showWelcome()
			m.openCloneDialog()
		}
		if menu.Item("Show Git Log").Chosen() {
			m.RunCommand("vcs.log")
		}
	})
}

// titleTool is one 30px icon control on the right of the title bar.
func (m *Model) titleTool(c *ui.Context, p *ui.Theme, oi oiColors, key, tip, icon string, onClick func()) {
	tk := oiTokensFor(m, p)
	b := ui.ButtonBase(c.Key("title:"+key)).Size(tk.StripeButton, tk.StripeButton).
		Radius(8).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip(tip)
	b.Children(func() { oiIconEl(c, icon, 20, oi, oi.icon) })
	if b.Clicked() {
		onClick()
	}
}

// gearButton is IDEA's toolbar gear: the quick switches and settings.
func (m *Model) gearButton(c *ui.Context, p *ui.Theme, oi oiColors) {
	tk := oiTokensFor(m, p)
	b := ui.ButtonBase(c.Key("title:gear")).Size(tk.StripeButton, tk.StripeButton).
		Radius(8).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip("IDE and Project Settings")
	b.Children(func() { oiIconEl(c, "general/settings", 20, oi, oi.icon) })
	b.Menu(func(menu *ui.Menu) { addMenuEntries(menu, m, gearGroup(m)) })
}

// addMenuEntries fills a MyGo menu with entries of the shared menu model, so
// the gear, the project widget and the menu bar run the same actions.
func addMenuEntries(menu *ui.Menu, m *Model, entries []menuEntry) {
	for _, e := range entries {
		if e.dynamic != nil {
			addMenuEntries(menu, m, e.dynamic(m))
			continue
		}
		switch {
		case e.sep:
			menu.Separator()
		case len(e.sub) > 0:
			menu.Submenu(e.label, func(sub *ui.Menu) { addMenuEntries(sub, m, e.sub) })
		default:
			it := menu.Item(e.label)
			if e.check != nil {
				it.Checked(e.check(m))
			}
			if it.Chosen() {
				switch {
				case e.run != nil:
					e.run(m)
				case e.command != "":
					m.RunCommand(e.command)
				}
				if m.welcomeGearOpen {
					m.welcomeGearOpen = false
				}
			}
		}
	}
}

// workbench is the Islands frame: title bar, the stripe and islands of the
// work area, an optional bottom area across their width, and the status bar.
func (m *Model) workbench(c *ui.Context, p *ui.Theme) {
	if m.zen {
		m.zenWorkbench(c, p)
		return
	}
	oi := oiPaletteOf(p)
	bottom := m.bottomTool()
	ui.Column(c).Fill().Background(oi.frame).Children(func() {
		m.titleBar(c, p)
		m.workspaceWindowTabs(c, p)
		m.renderTopNavigationBar(c, p, oi)
		if m.maximized && bottom != "" {
			// A maximized tool window takes the workbench, as OpenIDEA
			// hides the editor island over it.
			m.bottomSlot(c, p)
		} else {
			ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).
				AlignItems(ui.Stretch).Padding(0, 6, 0, 6).Children(func() {
				if !m.prefs.Appearance.HideToolWindowBars {
					m.toolStripe(c, p, oi)
				}
				if m.prefs.Appearance.WidescreenToolWindows {
					m.leftSide(c, p, oi)
					if m.leftShown() {
						m.leftWidthHandle(c, oi)
					} else {
						ui.Box(c).Width(oiIslandGap).Shrink(0)
					}
				}
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
						if !m.prefs.Appearance.WidescreenToolWindows {
							m.leftSide(c, p, oi)
							if m.leftShown() {
								m.leftWidthHandle(c, oi)
							} else {
								ui.Box(c).Width(oiIslandGap).Shrink(0)
							}
						}
						m.editorIsland(c, p)
						if m.rightShown() {
							m.rightWidthHandle(c, oi)
							if !m.prefs.Appearance.WidescreenToolWindows {
								m.rightSide(c, p, oi)
							}
						}
					})
					if bottom != "" {
						m.bottomSizeHandle(c)
						m.bottomSlot(c, p)
					}
				})
				if m.prefs.Appearance.WidescreenToolWindows {
					if m.rightShown() {
						m.rightSide(c, p, oi)
					} else {
						ui.Box(c).Width(oiIslandGap).Shrink(0)
					}
				}
				if !m.prefs.Appearance.HideToolWindowBars {
					m.rightStripe(c, p, oi)
				}
			})
		}
		m.undockedToolWindows(c, p, oi)
		m.balloons(c, p)
		if !m.prefs.Appearance.HideStatusBar {
			m.statusBar(c, p)
		}
	})
}

// undockedToolWindows draws View Mode › Undock windows over the editor.
// Their slots and content stay intact, but they take no track from the side
// that owns the slot, and their open size is the side's stored size.
func (m *Model) undockedToolWindows(c *ui.Context, p *ui.Theme, oi oiColors) {
	ids := m.undockedWindows()
	if len(ids) == 0 {
		return
	}
	frameWidth, height := c.Size()
	frameTop := c.TitleBar().Height
	if frameTop <= 0 {
		frameTop = oiTitleBarHeight
	}
	frameHeight := max(float32(0), height-frameTop)
	ui.Overlay(c, func() {
		for _, id := range ids {
			slot, ok := m.dockSlotOf(id)
			if !ok {
				continue
			}
			spec := dockSpec(id)
			if spec == nil || spec.render == nil {
				continue
			}
			panel := ui.Column(c).Key("undocked:window:" + id).Role(ui.RoleDialog).
				Label(spec.title).Background(m.toolWindowBackground(p)).
				Radius(oiIslandRadius).Clip().MinWidth(0).MinHeight(0)
			if panel.DoubleClicked() {
				m.toggleUndockedWindowMaximized(id)
			}
			dockWidth, dockHeight := m.undockedWindowSize(slot, frameWidth, frameHeight)
			m.placeUndockedWindow(panel, slot, frameTop, frameHeight, dockWidth, dockHeight)
			if m.consumeToolWindowFocus(id) {
				panel.Focusable().AutoFocus()
			}
			panel.Children(func() {
				m.dockWindowContent(c, p, oi, id)
			})
		}
	})
}

func (m *Model) toolWindowFrameHeader(c *ui.Context, p *ui.Theme, oi oiColors, id string) {
	spec := dockSpec(id)
	if spec == nil {
		return
	}
	header := ui.Row(c).Key("toolwindow:frame-header:"+id).
		Height(oiTokensFor(m, p).HeaderHeight).Shrink(0).Padding(0, 6, 0, 8).
		Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border)
	if header.DoubleClicked() {
		m.toggleUndockedWindowMaximized(id)
	}
	header.Children(func() {
		ui.Text(c, spec.title).FontSize(13).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
		ui.Spacer(c)
		m.toolWindowCornerIcon(c, p, oi, id)
	})
}

// placeUndockedWindow pins an undocked window to its side's inner edge as
// OpenIDEA's .undocked rule does. The primary and split areas divide a side
// by position, and maximized mode fills the whole workbench.
func (m *Model) placeUndockedWindow(
	panel ui.Element,
	slot dockSlot,
	frameTop, frameHeight, width, height float32,
) {
	if m.undockedWindowMaximized(slot) {
		panel.Absolute().Left(0).Right(0).Top(frameTop).Bottom(0)
		return
	}
	switch slot {
	case dockLeft:
		panel.Absolute().Left(0).Top(frameTop)
	case dockLeftSplit:
		panel.Absolute().Left(0).Top(frameTop + frameHeight/2)
	case dockRight:
		panel.Absolute().Right(0).Top(frameTop)
	case dockRightSplit:
		panel.Absolute().Right(0).Top(frameTop + frameHeight/2)
	case dockBottom:
		panel.Absolute().Left(0).Bottom(0).WidthPercent(100)
	case dockBottomSplit:
		panel.Absolute().Right(0).Bottom(0).WidthPercent(50)
	}
	if width > 0 {
		panel.Width(width).Height(height)
	} else {
		panel.Height(height)
	}
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
	tk := oiTokensFor(m, p)
	stripeWidth := tk.StripeWidth
	gap := tk.StripeGap
	if m.prefs.Appearance.ShowToolWindowNames {
		stripeWidth = oiStripeNamedBar
		gap = 4
	}
	ui.Column(c).Width(stripeWidth).Shrink(0).Padding(5, oiIslandGap, 6, 4).
		Gap(gap).Children(func() {
		m.stripeButton(c, p, oi, "project", "toolwindows/project", "Project", m.sideMode == "" && !m.leftHidden, func() {
			m.modeClick("")
		})
		m.stripeButton(c, p, oi, "vcs", "toolwindows/commit", "Commit", m.sideMode == "vcs" && !m.leftHidden, func() {
			m.modeClick("vcs")
		})
		m.stripeButton(c, p, oi, "structure", "toolwindows/structure", "Structure", m.dockWindowShown("structure"), func() {
			m.splitClick("structure")
		})
		m.stripeButton(c, p, oi, "bookmarks", "toolwindows/bookmarks", "Bookmarks", m.dockWindowShown("bookmarks"), func() {
			m.splitClick("bookmarks")
		})
		// Plugin views have a stripe button of their own, under the built-in
		// ones, as a Mayoi sidebar view does.
		for _, id := range toolWindowOrder {
			if !strings.HasPrefix(id, pluginViewPrefix) {
				continue
			}
			spec := dockSpec(id)
			if spec == nil || spec.anchor != dockLeft || !m.toolWindowStripeVisible(id) {
				continue
			}
			id, title := id, spec.title
			m.stripeButton(c, p, oi, id, pluginWindowIcon(spec), title, m.dockWindowShown(id), func() {
				m.DockToggle(id)
			})
		}
		m.stripeMoreButton(c, p, oi)
		ui.Spacer(c)
		for _, id := range m.toolWindowsForStripe(dockBottom) {
			spec := dockSpec(id)
			id := id
			m.stripeButton(c, p, oi, id, pluginWindowIcon(spec), spec.title,
				m.dockWindowShown(id), func() { m.DockToggle(id) })
		}
		m.stripeButton(c, p, oi, "commands", "general/search", "Search Everywhere", false, func() {
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

func (m *Model) stripeButton(c *ui.Context, p *ui.Theme, oi oiColors, key, icon, tip string, active bool, onClick func()) {
	tk := oiTokensFor(m, p)
	width, height := tk.StripeButton, tk.StripeButton
	showName := m.prefs.Appearance.ShowToolWindowNames
	if showName {
		width, height = oiStripeNamedButton, oiStripeNamedHeight
	}
	b := ui.ButtonBase(c.Key("stripe:"+key)).Size(width, height).
		Radius(6).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	switch {
	case active:
		b.Background(oi.stripeSelected)
	case b.Hovered():
		b.Background(oi.stripeHover)
	}
	b.Tooltip(tip)
	tone := oi.icon
	if active {
		tone = oi.text
	}
	b.Children(func() {
		ui.Column(c).AlignItems(ui.Center).Justify(ui.Center).Gap(3).Children(func() {
			ui.Row(c).Size(20, 20).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				oiIconEl(c, icon, 20, oi, tone)
				count := m.toolWindowBadge(key)
				if key == "vcs" && m.prefs.toolWindowBadgesVisible() {
					count = m.vcsBadge()
				}
				m.stripeBadge(c, oi, count)
			})
			if showName {
				ui.Text(c, tip).FontSize(9).TextColor(oi.textMuted).
					Width(oiStripeNamedButton - 4).TextAlign(ui.Center).SingleLine().Ellipsis("…")
			}
		})
	})
	if b.Clicked() {
		onClick()
	}
	if dockSpec(key) != nil {
		b.ContextMenu(func(menu *ui.Menu) { m.stripeButtonMenu(menu, key) })
	}
	// A registered window's button drags: the edges of the workbench dock it,
	// as OpenIDEA's drag and drop does.
	if dockSpec(key) != nil {
		if _, _, held := b.Dragged(); held || m.dockDragArmed {
			w, h := c.Size()
			lx, ly, _ := b.PointerPosition()
			r := b.Bounds()
			m.dockDragGhost(key, r.X+lx, r.Y+ly, w, h, held)
		}
	}
}

// stripeBadge draws OpenIDEA's 13px overhanging count pill.
func (m *Model) stripeBadge(c *ui.Context, oi oiColors, count int) {
	if count <= 0 || !m.prefs.toolWindowBadgesVisible() {
		return
	}
	label := fmt.Sprintf("%d", count)
	if count > 99 {
		label = "99+"
	}
	ui.Row(c).Absolute().Left(10).Top(-5).MinWidth(13).Height(13).
		Padding(0, 3).Radius(7).Background(oi.badge).Justify(ui.Center).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(9).Bold().TextColor(oi.text).SingleLine()
	})
}

// stripeMoreButton is OpenIDEA's "…" button for registered windows whose
// onUse stripe button has not been added yet.
func (m *Model) stripeMoreButton(c *ui.Context, p *ui.Theme, oi oiColors) {
	ids := m.toolWindowsWithoutStripeButton()
	if len(ids) == 0 {
		return
	}
	tk := oiTokensFor(m, p)
	width := tk.StripeButton
	if m.prefs.Appearance.ShowToolWindowNames {
		width = oiStripeNamedButton
	}
	b := ui.ButtonBase(c.Key("stripe:more")).Size(width, tk.StripeButton).
		Radius(6).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.stripeHover)
	}
	b.Tooltip("More Tool Windows")
	b.Children(func() { oiIconEl(c, "general/moreHorizontal", 20, oi, oi.icon) })
	b.Menu(func(menu *ui.Menu) {
		for _, id := range ids {
			spec := dockSpec(id)
			if spec == nil {
				continue
			}
			if menu.Item(spec.title).Chosen() {
				m.DockShow(id)
			}
		}
	})
}

// stripeButtonMenu is the context menu of a tool-window stripe button.
func (m *Model) stripeButtonMenu(menu *ui.Menu, id string) {
	if menu.Item("Hide").Chosen() {
		m.HideToolWindowFromStripe(id)
	}
	menu.Separator()
	current, shown := m.dockSlotOf(id)
	menu.Submenu("Move To", func(sub *ui.Menu) {
		for slot := dockSlot(0); slot < dockSlotCount; slot++ {
			item := sub.Item(dockSlotLabel(slot))
			if shown && current == slot {
				item.Checked(true)
			}
			if item.Chosen() {
				m.DockMove(id, slot)
			}
		}
	})
	menu.Separator()
	item := menu.Item("Show Tool Window Names")
	item.Checked(m.prefs.Appearance.ShowToolWindowNames)
	if item.Chosen() {
		m.prefs.Appearance.ShowToolWindowNames = !m.prefs.Appearance.ShowToolWindowNames
		m.savePreferences()
		m.invalidate()
	}
}

// leftShown reports whether the left side draws: a window in either of its
// areas.
func (m *Model) leftShown() bool {
	return m.remainingDockOccupant(dockLeft) != "" || !m.leftHidden ||
		m.dockTrackShown(dockLeftSplit)
}

// leftSide is the left side's two areas: the primary island and, below it,
// the split island, with the gap between them the pointer drags. A lone
// island takes the whole side, as OpenIDEA's SidePanel does, and hiding both
// closes the side.
func (m *Model) leftSide(c *ui.Context, p *ui.Theme, oi oiColors) {
	if !m.leftShown() {
		return
	}
	splitID := m.remainingDockOccupant(dockLeftSplit)
	both := !m.leftHidden && splitID != ""
	primaryGrow, splitGrow := float32(1), float32(1)
	if both {
		primaryGrow = m.leftShare()
		splitGrow = 1 - primaryGrow
	}
	side := ui.Column(c).Key("left:side").Width(m.leftSideWidth()).Shrink(0).
		MinHeight(0).AlignItems(ui.Stretch)
	// The room the islands shared as the last frame laid it out, which the
	// handle moves the line within.
	room := side.Bounds().H - oiIslandGap
	side.Children(func() {
		if !m.leftHidden {
			m.leftIsland(c, p, oi, primaryGrow)
		}
		if both {
			m.leftSplitHandle(c, oi, room)
		}
		if splitID != "" {
			if spec := dockSpec(splitID); spec != nil && spec.renderGrow != nil {
				spec.renderGrow(m, c, p, oi, splitGrow)
			}
		}
	})
}

// The left side's sizes, as OpenIDEA's sizes.left and its handles: a stored
// width outside 80..4000 is ignored, a drag keeps 140 DIPs for the side and
// 200 for the editor, and half of 140 closes the side as IDEA does.
const (
	leftWidthStoredMin = 80
	leftWidthStoredMax = 4000
	leftWidthDragMin   = 140
	editorMinWidth     = 200
)

// leftSideWidth is the width the left side draws at: the width a resize left,
// or the width the gap's drag has put it at.
func (m *Model) leftSideWidth() float32 {
	if m.leftWidthDragging {
		if m.leftDragCollapse {
			return 0
		}
		w := m.leftDragWidth
		if w < leftWidthDragMin {
			w = leftWidthDragMin
		}
		if w > m.leftDragMax {
			w = m.leftDragMax
		}
		return w
	}
	if m.leftWidth < leftWidthStoredMin {
		return oiLeftWidth
	}
	return m.leftWidth
}

// leftWidthHandle is the gap on the side's inner edge, between it and the
// editor, which the pointer drags to resize the side, as OpenIDEA's GapHandle
// does. A drag past half the side's smallest width closes the side on release;
// anything else is kept then.
func (m *Model) leftWidthHandle(c *ui.Context, oi oiColors) {
	h := ui.Row(c).Key("left:width-handle").Width(oiIslandGap).Shrink(0).
		Cursor(ui.CursorResizeColumn)
	dx, _, held := h.Dragged()
	if !held {
		if m.leftWidthDragging {
			w := m.leftSideWidth()
			m.leftWidthDragging = false
			if m.leftDragCollapse {
				m.leftDragCollapse = false
				m.hideLeftSide()
			} else {
				// OpenIDEA stores and draws whole DIPs.
				m.leftWidth = float32(int(w + 0.5))
				m.rememberLayout()
			}
		}
		return
	}
	if !m.leftWidthDragging {
		// The base is the width drawn now, read before the drag takes over
		// from it.
		base := m.leftSideWidth()
		m.leftWidthDragging = true
		m.leftDragWidth = base
		m.leftDragMax = base
		if spare := m.editorWidth - editorMinWidth; spare > 0 {
			m.leftDragMax += spare
		}
		m.leftDragCollapse = false
	}
	m.leftDragWidth += dx
	m.leftDragCollapse = m.leftDragWidth < leftWidthDragMin/2
	m.invalidate()
}

// rightSideWidth is the right area's width in DIPs, as drawn: a drag in
// progress, the stored width, or OpenIDEA's default.
func (m *Model) rightSideWidth() float32 {
	if m.rightWidthDragging {
		return m.rightDragWidth
	}
	if m.rightWidth < rightWidthStoredMin {
		return rightWidthDefault
	}
	return m.rightWidth
}

// rightWidthHandle drags the gap between the editor and the right area. The
// pointer's left widens the right area, mirroring the left side's handle.
func (m *Model) rightWidthHandle(c *ui.Context, oi oiColors) {
	h := ui.Row(c).Key("right:width-handle").Width(oiIslandGap).Shrink(0).
		Cursor(ui.CursorResizeColumn)
	dx, _, held := h.Dragged()
	if !held {
		if m.rightWidthDragging {
			m.rightWidthDragging = false
			m.rightWidth = float32(int(m.rightDragWidth + 0.5))
			m.rememberLayout()
		}
		return
	}
	if !m.rightWidthDragging {
		m.rightWidthDragging = true
		m.rightDragWidth = m.rightSideWidth()
	}
	m.rightDragWidth = max(rightWidthStoredMin, min(m.rightDragWidth-dx, rightWidthStoredMax))
	m.invalidate()
}

// rightSide is the right side's two areas: the primary slot's window and,
// below it, the split slot's, with the gap between them the pointer drags. A
// lone window takes the whole side, as the left side does.
func (m *Model) rightSide(c *ui.Context, p *ui.Theme, oi oiColors) {
	top := m.remainingDockOccupant(dockRight)
	bottom := m.remainingDockOccupant(dockRightSplit)
	if top == "" && !m.rightHidden && dockSpec(m.rightTool) != nil {
		// A session or a test can still name the right window the old way.
		top = m.rightTool
	}
	if bottom == "" && !m.rightHidden && dockSpec(m.rightSplitTool) != nil {
		bottom = m.rightSplitTool
	}
	if m.undockedWindow(top) {
		top = ""
	}
	if m.undockedWindow(bottom) {
		bottom = ""
	}
	if top == "" && bottom == "" {
		return
	}
	both := top != "" && bottom != ""
	primaryGrow, splitGrow := float32(1), float32(1)
	if both {
		primaryGrow = m.rightShare()
		splitGrow = 1 - primaryGrow
	}
	side := ui.Column(c).Key("right:side").Width(m.rightSideWidth()).Shrink(0).
		MinHeight(0).AlignItems(ui.Stretch)
	room := side.Bounds().H - oiIslandGap
	side.Children(func() {
		if top != "" {
			m.rightIsland(c, p, oi, top, primaryGrow)
		}
		if both {
			m.rightSplitHandle(c, oi, room)
		}
		if bottom != "" {
			m.rightIsland(c, p, oi, bottom, splitGrow)
		}
	})
}

// rightIsland draws one registered window in the right side, at its share of
// the side's room.
func (m *Model) rightIsland(c *ui.Context, p *ui.Theme, oi oiColors, id string, grow float32) {
	spec := dockSpec(id)
	if spec == nil || spec.render == nil {
		return
	}
	island := ui.Column(c).Key("right:island:" + id).Grow(grow).MinHeight(0).
		MinWidth(0).Background(m.toolWindowBackground(p)).Radius(oiIslandRadius).
		Clip().AlignItems(ui.Stretch)
	if m.consumeToolWindowFocus(id) {
		island.Focusable().AutoFocus()
	}
	island.Children(func() {
		m.dockWindowContent(c, p, oi, id)
	})
}

// rightShare is the share of the right side the primary slot takes; a ratio
// no drag reaches keeps the default divide.
func (m *Model) rightShare() float32 {
	if m.rightRatio < leftRatioMin || m.rightRatio > leftRatioMax {
		return leftRatioDefault
	}
	return m.rightRatio
}

// rightSplitHandle is the gap between the right side's two windows. Dragging
// it moves their shared line, as OpenIDEA's SplitHandle does, and the ratio is
// kept once the pointer lets go.
func (m *Model) rightSplitHandle(c *ui.Context, oi oiColors, room float32) {
	h := ui.Row(c).Key("right:split-handle").Height(oiIslandGap).Shrink(0).
		Cursor(ui.CursorResizeRow)
	_, dy, held := h.Dragged()
	if !held {
		if m.rightDragging {
			m.rightDragging = false
			m.rememberLayout()
		}
		return
	}
	if !m.rightDragging {
		m.rightDragging = true
		m.rightDragTotal = room
	}
	if dy == 0 || m.rightDragTotal <= 0 {
		return
	}
	lo, hi := leftRatioRange(m.rightDragTotal)
	ratio := m.rightShare() + dy/m.rightDragTotal
	if ratio < lo {
		ratio = lo
	}
	if ratio > hi {
		ratio = hi
	}
	if ratio != m.rightRatio {
		m.rightRatio = ratio
		m.invalidate()
	}
}

// rightStripe is the right edge's stripe of tool window buttons, mirroring the
// left one. It follows the same icon/name width and the same context menu for
// every registered window.
func (m *Model) rightStripe(c *ui.Context, p *ui.Theme, oi oiColors) {
	tk := oiTokensFor(m, p)
	stripeWidth := tk.StripeWidth
	gap := tk.StripeGap
	if m.prefs.Appearance.ShowToolWindowNames {
		stripeWidth = oiStripeNamedBar
		gap = 4
	}
	ui.Column(c).Width(stripeWidth).Shrink(0).Padding(5, oiIslandGap, 6, 4).
		Gap(gap).Children(func() {
		for _, id := range m.toolWindowsForStripe(dockRight) {
			spec := dockSpec(id)
			id := id
			m.stripeButton(c, p, oi, id, pluginWindowIcon(spec), spec.title,
				m.dockWindowShown(id), func() { m.DockToggle(id) })
		}
		ui.Spacer(c)
		for _, id := range m.toolWindowsForStripe(dockBottomSplit) {
			spec := dockSpec(id)
			id := id
			m.stripeButton(c, p, oi, id, pluginWindowIcon(spec), spec.title,
				m.dockWindowShown(id), func() { m.DockToggle(id) })
		}
	})
}

// pluginWindowIcon is a window's OpenIDEA drawing; contributed views without
// one draw the plugin glyph, as upstream's generic tool window does.
func pluginWindowIcon(spec *toolWindowSpec) string {
	if spec != nil && spec.icon != "" {
		return spec.icon
	}
	return "nodes/plugin"
}

// hideLeftSide closes both of the left side's areas, which a drag of its gap
// past half the smallest width does.
func (m *Model) hideLeftSide() {
	m.leftHidden = true
	m.leftSplit = ""
	m.structure.visible = false
	m.rememberLayout()
	m.invalidate()
}

// The bottom side's sizes, as OpenIDEA's sizes.bottom and its GapHandle: a drag
// keeps 90 DIPs for the side and 60 for the editor, and half of 90 closes the
// side on release.
const (
	bottomDragMin   = 90
	editorMinHeight = 60
)

// bottomDragHeight is the height a running drag of the bottom gap asks for,
// clamped to the side's smallest height and the room the editor keeps.
func (m *Model) bottomDragHeight() float32 {
	h := m.bottomDragSize
	if h < bottomDragMin {
		h = bottomDragMin
	}
	if h > m.bottomDragMax {
		h = m.bottomDragMax
	}
	return h
}

// bottomDrawHeight is the height the bottom side draws at: the height it was
// left with, or the height a drag of its gap has put it at.
func (m *Model) bottomDrawHeight() int {
	if !m.bottomDragging {
		return m.bottomSize()
	}
	if m.bottomDragCollapse {
		return 0
	}
	// OpenIDEA stores and draws whole DIPs.
	return int(m.bottomDragHeight() + 0.5)
}

// bottomSizeHandle is the gap above the bottom island, which the pointer drags
// to resize it, as OpenIDEA's GapHandle does. A drag past half the side's
// smallest height closes every bottom window on release; anything else is kept
// then.
func (m *Model) bottomSizeHandle(c *ui.Context) {
	h := ui.Row(c).Key("bottom:size-handle").Height(oiIslandGap).Shrink(0).
		Cursor(ui.CursorResizeRow)
	_, dy, held := h.Dragged()
	if !held {
		if m.bottomDragging {
			size := m.bottomDragHeight()
			m.bottomDragging = false
			if m.bottomDragCollapse {
				m.bottomDragCollapse = false
				m.hideBottomWindows()
			} else if int(size+0.5) != m.bottomSize() {
				m.bottomHeight = int(size + 0.5)
				m.rememberLayout()
			}
		}
		return
	}
	if !m.bottomDragging {
		// The base is the height drawn now, read before the drag takes over
		// from it.
		base := float32(m.bottomSize())
		m.bottomDragging = true
		m.bottomDragSize = base
		m.bottomDragMax = base
		if spare := m.editorHeight - editorMinHeight; spare > 0 {
			m.bottomDragMax += spare
		}
		m.bottomDragCollapse = false
	}
	// Dragging up gives the side more room, as OpenIDEA's sign of -1 for the
	// bottom anchor does.
	m.bottomDragSize -= dy
	m.bottomDragCollapse = m.bottomDragSize < bottomDragMin/2
	m.invalidate()
}

// leftRatioMin and leftRatioMax bound the left side's split, as OpenIDEA's
// MIN_SPLIT and MAX_SPLIT do, and leftSplitMin is the room the smaller area
// keeps when the side has it, SPLIT_MIN stacked.
const (
	leftRatioMin     = 0.1
	leftRatioMax     = 0.9
	leftRatioDefault = 0.5
	leftSplitMin     = 60
)

// leftShare is the share of the left side's room the primary area takes; a
// ratio no drag reaches keeps the default divide.
func (m *Model) leftShare() float32 {
	if m.leftRatio < leftRatioMin || m.leftRatio > leftRatioMax {
		return leftRatioDefault
	}
	return m.leftRatio
}

// leftRatioRange is how far a drag may push the split at the room the two
// areas share, keeping leftSplitMin on the smaller one as OpenIDEA clamps
// the pointer's position.
func leftRatioRange(room float32) (lo, hi float32) {
	lo = leftRatioMin
	if room > 0 {
		if floor := float32(leftSplitMin) / room; floor > lo {
			lo = floor
		}
		if lo > leftRatioDefault {
			lo = leftRatioDefault
		}
	}
	return lo, 1 - lo
}

// leftSplitHandle is the gap between the left side's two islands. Dragging it
// moves their shared line, as OpenIDEA's SplitHandle does, and the ratio is
// kept once the pointer lets go.
func (m *Model) leftSplitHandle(c *ui.Context, oi oiColors, room float32) {
	h := ui.Row(c).Key("left:split-handle").Height(oiIslandGap).Shrink(0).
		Cursor(ui.CursorResizeRow)
	_, dy, held := h.Dragged()
	if !held {
		if m.leftDragging {
			m.leftDragging = false
			m.rememberLayout()
		}
		return
	}
	if !m.leftDragging {
		m.leftDragging = true
		m.leftDragTotal = room
	}
	if dy == 0 || m.leftDragTotal <= 0 {
		return
	}
	lo, hi := leftRatioRange(m.leftDragTotal)
	ratio := m.leftShare() + dy/m.leftDragTotal
	if ratio < lo {
		ratio = lo
	}
	if ratio > hi {
		ratio = hi
	}
	if ratio != m.leftRatio {
		m.leftRatio = ratio
		m.invalidate()
	}
}

// leftIsland is the tool window showing in the left primary area: its header
// with the actions of the window, then its body. grow is its share of the
// side, which the drag of the gap between the islands moves.
func (m *Model) leftIsland(c *ui.Context, p *ui.Theme, oi oiColors, grow float32) {
	tk := oiTokensFor(m, p)
	if id := m.remainingDockOccupant(dockLeft); id != "" {
		if spec := dockSpec(id); spec != nil && spec.render != nil {
			background := oi.island
			if m.prefs.Appearance.DifferentToolWindowBackground {
				background = oi.elev
			}
			ui.Column(c).Grow(grow).MinHeight(0).Background(background).
				Radius(oiIslandRadius).Children(func() {
				spec.render(m, c, p, oi)
			})
			return
		}
	}
	title := "Project"
	if m.sideMode == "vcs" {
		title = "Commit"
		if n := len(m.currentVCSFiles()); n > 0 {
			title = fmt.Sprintf("Commit · %d", n)
		}
	}
	if m.sideMode != "vcs" {
		m.syncProjectTreeState()
	}
	background := oi.island
	if m.prefs.Appearance.DifferentToolWindowBackground {
		background = oi.elev
	}
	ui.Column(c).Grow(grow).MinHeight(0).Background(background).
		Radius(oiIslandRadius).Children(func() {
		ui.Row(c).Height(tk.HeaderHeight).Shrink(0).Padding(0, 6, 0, 8).
			Gap(6).AlignItems(ui.Center).Children(func() {
			if m.sideMode == "vcs" {
				ui.Text(c, title).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			} else if id := m.remainingDockOccupant(dockLeft); id != "" {
				ui.Text(c, dockSpec(id).title).FontSize(13).TextColor(oi.textStrong).
					SingleLine().Ellipsis("…")
			} else {
				m.projectTreeTitle(c, p, oi)
			}
			ui.Spacer(c)
			if m.sideMode != "vcs" && m.tree != nil {
				if !m.prefs.ProjectTree.AutoscrollFromSource {
					m.headerIcon(c, p, oi, "tree-locate", "Select Opened File", "general/locate", func() {
						m.followActiveInProjectView(true)
						m.invalidate()
					})
				}
				m.headerIcon(c, p, oi, "tree-expand", "Expand All", "general/expandAll", func() {
					m.ExpandProjectAll()
				})
				m.headerIcon(c, p, oi, "tree-collapse", "Collapse All", "general/collapseAll", func() {
					m.CollapseProjectAll()
				})
				m.projectViewOptionsButton(c, p, oi)
			}
			if id := m.remainingDockOccupant(dockLeft); id != "" {
				m.toolWindowCornerIcon(c, p, oi, id)
			}
		})
		if m.sideMode == "vcs" {
			m.vcsToolWindow(c, p, oi)
			return
		}
		// The rows scroll both ways, as OpenIDEA's tree does: a name that runs
		// past the island keeps its whole text, and the tree shows the rest
		// when it is scrolled sideways.
		scroll := ui.ScrollBoth(c).Key("tree:scroll").Grow(1).MinHeight(0).
			TrackScroll(&m.treeScroll)
		scroll.Children(func() {
			if m.tree == nil {
				return
			}
			rows := ui.Column(c).Padding(0, 6).
				// At least as wide as the island, so a narrow tree still paints
				// its selection and hover across the whole window.
				MinWidth(m.leftSideWidth())
			rows.Children(func() {
				m.beginTreeDragFrame()
				for _, row := range m.tree.Rows() {
					m.treeRow(c, p, row)
				}
				m.endTreeDragFrame()
			})
		})
	})
}

// leftSplitIsland is the tool window in the left split area, below the primary
// one: Structure or Bookmarks, with a hide action in its header. grow is its
// share of the side, which the drag of the gap between the islands moves.
func (m *Model) leftSplitIsland(c *ui.Context, p *ui.Theme, oi oiColors, grow float32) {
	tk := oiTokensFor(m, p)
	title := "Structure"
	id := "structure"
	if m.leftSplit == "bookmarks" {
		title = "Bookmarks"
		id = "bookmarks"
	}
	island := ui.Column(c).Grow(grow).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius).Clip()
	if m.consumeToolWindowFocus(id) {
		island.Focusable().AutoFocus()
	}
	island.Children(func() {
		ui.Row(c).Height(tk.HeaderHeight).Shrink(0).Padding(0, 6, 0, 8).
			Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			m.toolWindowCornerIcon(c, p, oi, id)
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
func (m *Model) headerIcon(c *ui.Context, p *ui.Theme, oi oiColors, key, tip, icon string, onClick func()) {
	m.headerIconDisabled(c, p, oi, key, tip, icon, onClick)
}

func (m *Model) headerIconDisabled(c *ui.Context, p *ui.Theme, oi oiColors, key, tip, icon string, onClick func()) {
	b := ui.ButtonBase(c.Key("panel:"+key)).Size(20, 20).Radius(4).
		Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	b.Disabled(onClick == nil)
	if b.Hovered() || b.Focused() {
		b.Background(oi.toolbarHover)
		b.Opacity(1)
	} else {
		b.Opacity(0.42)
	}
	b.Tooltip(tip)
	b.Children(func() { oiIconEl(c, icon, 16, oi, oi.icon) })
	if onClick != nil && b.Clicked() {
		onClick()
	}
}

// toolWindowCornerIcon draws the same ⋮ and Hide controls as HeaderRow on a
// panel that already owns its title. It keeps panel headers visually alike
// without taking the content tab strip away from the feature.
func (m *Model) toolWindowCornerIcon(c *ui.Context, p *ui.Theme, oi oiColors, id string) {
	spec := dockSpec(id)
	if spec == nil {
		return
	}
	more := ui.ButtonBase(c.Key("toolwindow:frame-options:"+id)).Size(20, 20).
		Radius(4).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip("Options").Label("Options")
	if more.Hovered() || more.Focused() {
		more.Background(oi.toolbarHover)
	}
	more.Children(func() { oiIconEl(c, "general/moreVertical", 16, oi, oi.icon) })
	more.Menu(func(menu *ui.Menu) { m.toolWindowFrameOptionsMenu(menu, id) })
	hideTip := "Hide " + spec.title + " Tool Window"
	hide := ui.ButtonBase(c.Key("toolwindow:frame-hide:"+id)).Size(20, 20).
		Radius(4).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip(hideTip).Label(hideTip)
	if hide.Hovered() || hide.Focused() {
		hide.Background(oi.toolbarHover)
	}
	hide.Children(func() { oiIconEl(c, "general/hide", 16, oi, oi.icon) })
	if hide.Clicked() {
		m.DockHide(id)
	}
}

// toolWindowFrameOptionsMenu is HeaderRow's options menu for a window whose
// content owns its title: View Mode, Move To, Maximize / Restore and Hide.
func (m *Model) toolWindowFrameOptionsMenu(menu *ui.Menu, id string) {
	current := m.toolViewMode(id)
	menu.Submenu("View Mode", func(sub *ui.Menu) {
		for _, mode := range []struct {
			id, label string
		}{
			{"dockPinned", "Dock Pinned"},
			{"dockUnpinned", "Dock Unpinned"},
			{"undock", "Undock"},
		} {
			mode := mode
			item := sub.Item(mode.label).Checked(current == mode.id)
			if item.Chosen() {
				m.SetToolViewMode(id, mode.id)
			}
		}
	})
	menu.Submenu("Move To", func(sub *ui.Menu) {
		from, shown := m.dockSlotOf(id)
		for slot := dockSlot(0); slot < dockSlotCount; slot++ {
			slot := slot
			item := sub.Item(dockSlotLabel(slot)).Checked(shown && from == slot).
				Disabled(shown && from == slot)
			if item.Chosen() {
				m.DockMove(id, slot)
			}
		}
	})
	maximize := "Maximize"
	if m.maximized {
		maximize = "Restore Size"
	}
	if menu.Item(maximize).Chosen() {
		if m.undockedWindow(id) {
			m.toggleUndockedWindowMaximized(id)
		} else {
			m.toggleToolWindowMaximized()
		}
	}
	menu.Separator()
	if menu.Item("Hide").Chosen() {
		m.DockHide(id)
	}
}

// editorIsland is the editor surface: one window group, or two split side by
// side or stacked, each with its tab bar, breadcrumb chain and editor.
func (m *Model) editorIsland(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	island := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius)
	// The width it drew at last frame caps a drag of the side beside it, and
	// the height caps a drag of the bottom area below it.
	m.editorWidth = island.Bounds().W
	m.editorHeight = island.Bounds().H
	island.Children(func() {
		if m.split == nil {
			m.editorGroup(c, p, oi, m.focusedView())
			return
		}
		first, second := m.focusedView(), m.splitView()
		if m.splitFirst {
			first, second = second, first
		}
		if m.splitDown {
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				m.editorGroup(c, p, oi, first)
				ui.Row(c).Height(1).Shrink(0).Background(oi.border)
				m.editorGroup(c, p, oi, second)
			})
			return
		}
		ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			m.editorGroup(c, p, oi, first)
			ui.Column(c).Width(1).Shrink(0).Background(oi.border)
			m.editorGroup(c, p, oi, second)
		})
	})
}

// editorGroup is one window group: its tabs where Editor Tabs puts them and
// its editor, taking its half of the island.
func (m *Model) editorGroup(c *ui.Context, p *ui.Theme, oi oiColors, g groupView) {
	pane := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0)
	pane.Children(func() {
		if m.tabPrefs.Vertical() {
			ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				if m.tabPrefs.Placement != "right" {
					m.verticalTabs(c, p, oi, g)
				}
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					m.editorBody(c, p, g)
				})
				if m.tabPrefs.Placement == "right" {
					m.verticalTabs(c, p, oi, g)
				}
			})
			return
		}
		if !m.tabPrefs.AtBottom() {
			m.editorTabs(c, p, oi, g)
		}
		m.editorBody(c, p, g)
		if m.tabPrefs.AtBottom() {
			m.editorTabs(c, p, oi, g)
		}
	})
}

// editorBody is the banner over the editor, which takes the rest of the
// group. The banner is the project's, so only the focused group shows it.
func (m *Model) editorBody(c *ui.Context, p *ui.Theme, g groupView) {
	if m.banner != "" && g.focused {
		m.bannerRow(c, p)
	}
	if g.focused {
		m.composeDetectionBanner(c, p, g.tab())
	}
	m.librarySourcesBanner(c, p, g.tab())
	m.editorArea(c, p, g)
}

// editorTabs is OpenIDEA's tab bar: 40px high on the island, 28px tabs with
// the selected one tinted and underlined in the accent. Editor Tabs decides
// where it sits, whether pinned tabs get their own row, what each tab shows
// and where its close button is.
func (m *Model) editorTabs(c *ui.Context, p *ui.Theme, oi oiColors, g groupView) {
	if len(g.tabs) == 0 || m.tabPrefs.Hidden() {
		return
	}
	if g.focused {
		m.applyTabDrop()
	}
	pinned := 0
	for _, t := range g.tabs {
		if t.pinned {
			pinned++
		}
	}
	if m.tabPrefs.PinnedSeparateRow && pinned > 0 && pinned < len(g.tabs) {
		ui.Column(c).Shrink(0).Children(func() {
			m.tabStrip(c, p, oi, g, g.tabs[:pinned], 0, g.pinned)
			m.tabStrip(c, p, oi, g, g.tabs[pinned:], pinned, g.scroll)
		})
		return
	}
	m.tabStrip(c, p, oi, g, g.tabs, 0, g.scroll)
}

// tabStrip is the tab bar with the island's bottom border; offset is the
// row's first tab in the strip and scroll the row's sideways place. Show tabs
// in decides how tabs that don't fit go: one row scrolls sideways or squeezes
// them, multiple rows wrap.
func (m *Model) tabStrip(c *ui.Context, p *ui.Theme, oi oiColors, g groupView, tabs []*Tab, offset int, scroll *ui.ScrollState) {
	tk := oiTokensFor(m, p)
	reveal := *g.shown != g.active
	*g.shown = g.active
	strip := ui.Row(c).Shrink(0).Padding(0, 4).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border)
	if m.tabPrefs.MultipleRows {
		strip.MinHeight(tk.TabBarHeight)
	} else {
		strip.Height(tk.TabBarHeight)
	}
	strip.Children(func() {
		switch {
		case m.tabPrefs.MultipleRows:
			ui.Row(c).Grow(1).MinWidth(0).Wrap().Gap(2).Padding(3, 0).
				AlignItems(ui.Center).AlignContent(ui.Center).Children(func() {
				m.tabButtons(c, p, oi, g, tabs, offset, reveal, false)
			})
		case m.tabPrefs.SqueezeTabs:
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				m.tabButtons(c, p, oi, g, tabs, offset, reveal, false)
			})
		default:
			ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Children(func() {
				ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Gap(2).
					AlignItems(ui.Center).TrackScroll(scroll).Children(func() {
					m.tabButtons(c, p, oi, g, tabs, offset, reveal, false)
				})
				if scroll.MaxX > 0.5 {
					m.tabOverflow(c, p, oi, g, tabs, offset)
				}
			})
		}
	})
}

// tabMenuRow is a tab the Show Hidden Tabs menu lists: its place in the
// workbench strip and whether it is the tab on show.
type tabMenuRow struct {
	Tab    *Tab
	Index  int
	Active bool
}

// tabMenuRows numbers a strip's tabs for the menu above the chevron.
func tabMenuRows(tabs []*Tab, offset, active int) []tabMenuRow {
	rows := make([]tabMenuRow, 0, len(tabs))
	for j, t := range tabs {
		i := offset + j
		rows = append(rows, tabMenuRow{Tab: t, Index: i, Active: i == active})
	}
	return rows
}

// tabOverflow is OpenIDEA's chevron at the end of a strip whose tabs run past
// its width: Show Hidden Tabs opens a menu of every tab in the strip, the
// active one marked, and picking one shows it.
func (m *Model) tabOverflow(c *ui.Context, p *ui.Theme, oi oiColors, g groupView, tabs []*Tab, offset int) {
	key, rowKey := "tabsOverflow", "tabsOverflow:%d"
	if g.ref == groupSplit {
		key, rowKey = "splitTabsOverflow", "splitTabsOverflow:%d"
	}
	b := ui.ButtonBase(c.Key(key)).Size(22, 22).Radius(4).
		AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
		Tooltip("Show Hidden Tabs")
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Children(func() {
		oiIconEl(c, "general/chevronDown", 14, oi, oi.textMuted)
	})
	if b.Clicked() {
		*g.menuOpen = !*g.menuOpen
		m.invalidate()
	}
	open := *g.menuOpen
	ui.PopoverBase(c, b, &open, func(panel ui.Element) {
		panel.Width(300).Padding(4).Background(oi.popup).
			BorderWidth(1).BorderColor(oi.borderStrong).Radius(8)
		panel.Children(func() {
			for _, row := range tabMenuRows(tabs, offset, g.active) {
				row := row
				r := ui.Row(c.Key(fmt.Sprintf(rowKey, row.Index))).
					Height(26).Shrink(0).Padding(0, 8).Gap(8).Radius(4).
					AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if row.Active || r.Hovered() {
					r.Background(oi.hoverTint)
				}
				name, dir := m.tabLabelIn(g.tabs, row.Tab)
				r.Children(func() {
					fileIconEl(c, oi, name, 14, oi.textMuted)
					ui.Text(c, name).FontSize(13).TextColor(oi.text).SingleLine().
						Ellipsis("…")
					if dir != "" {
						ui.Text(c, dir).FontSize(13).TextColor(oi.textFaint).
							SingleLine().Ellipsis("…")
					}
				})
				if r.Clicked() {
					*g.menuOpen = false
					m.activateIn(g.ref, row.Index)
				}
			}
		})
	})
	if !open {
		*g.menuOpen = false
	}
}

// verticalTabs is OpenIDEA's strip on the editor's left or right: a scrolling
// column of full-width 24px tabs, as wide as its widest tab between 140 and
// 260, divided from the editor by a border on that side. Show tabs in does not
// apply: a vertical strip always scrolls.
func (m *Model) verticalTabs(c *ui.Context, p *ui.Theme, oi oiColors, g groupView) {
	if len(g.tabs) == 0 || m.tabPrefs.Hidden() {
		return
	}
	if g.focused {
		m.applyTabDrop()
	}
	reveal := *g.shown != g.active
	*g.shown = g.active
	strip := ui.Column(c).Shrink(0).MinWidth(oiTabColumnMin).MaxWidth(oiTabColumnMax).
		Padding(4).Gap(2).BorderColor(oi.border)
	if m.tabPrefs.Placement == "right" {
		strip.BorderWidth(0, 0, 0, 1)
	} else {
		strip.BorderWidth(0, 1, 0, 0)
	}
	pinned := 0
	for _, t := range g.tabs {
		if t.pinned {
			pinned++
		}
	}
	if m.tabPrefs.PinnedSeparateRow && pinned > 0 && pinned < len(g.tabs) {
		strip.Children(func() {
			ui.Scroll(c).Shrink(0).MinHeight(0).Gap(2).Children(func() {
				m.tabButtons(c, p, oi, g, g.tabs[:pinned], 0, reveal, true)
			})
			ui.Scroll(c).Grow(1).MinHeight(0).Gap(2).TrackScroll(g.side).
				Children(func() {
					m.tabButtons(c, p, oi, g, g.tabs[pinned:], pinned, reveal, true)
				})
		})
		return
	}
	strip.Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).Gap(2).TrackScroll(g.side).
			Children(func() {
				m.tabButtons(c, p, oi, g, g.tabs, 0, reveal, true)
			})
	})
}

// tabButtons draws tabs in a row, offset being the first tab's index in the
// strip. A squeezed strip lets the labels ellipsize down to the tab's fixed
// parts; a vertical strip always ellipsizes, its tabs full width and only 24
// high. A change of active tab scrolls the new one into view. A tab dragged
// along its row, as OpenIDEA's Tabs drags it, leaves its slot where it would
// drop while the others make room, and a ghost of it hangs from the pointer.
func (m *Model) tabButtons(c *ui.Context, p *ui.Theme, oi oiColors, g groupView, tabs []*Tab, offset int, reveal, vertical bool) {
	tk := oiTokensFor(m, p)
	squeeze := m.tabPrefs.SqueezeTabs || vertical
	from := -1
	if g.focused && m.tabDrag.Held {
		candidate := m.tabDrag.From - offset
		if candidate >= 0 && candidate < len(tabs) {
			from = candidate
		}
	}
	lifted := from >= 0 && from < len(tabs) && m.tabDrag.Lifted
	order := tabOrder(len(tabs), from, m.tabDrag.To-offset, lifted)
	var rects []ui.Rect
	var others []int
	for _, j := range order {
		i, t := offset+j, tabs[j]
		height := tk.TabHeight
		if vertical {
			height = oiTabColumnTab
		}
		key := fmt.Sprintf("tab:%d", i)
		if g.ref == groupSplit {
			key = fmt.Sprintf("splitTab:%d", i)
		}
		tab := ui.Row(c.Key(key)).Height(height).
			Padding(0, 10).Gap(6).Radius(6).AlignItems(ui.Center)
		if vertical {
			tab.MinWidth(0)
		}
		slot := lifted && j == from
		if slot {
			tab.Width(m.tabDrag.W).Background(oi.toolbarHover)
		} else if i == g.active {
			if reveal {
				tab.ScrollIntoView()
			}
			// The tab the group shows is tinted; the one an unfocused group
			// shows is muted, as OpenIDEA fades the pane that has no focus.
			if g.focused {
				tab.Background(oi.tabSelected).BorderWidth(1).BorderColor(oi.tabSelectedBorder)
			} else {
				tab.Background(oi.toolbarHover)
			}
		} else if tab.Hovered() {
			tab.Background(oi.toolbarHover)
		}
		if !slot {
			if !m.tabPrefs.HideFullPathOnHover {
				if path := tabPath(t); path != "" {
					tab.Tooltip(path)
				}
			}
			name, dir := m.tabLabelIn(g.tabs, t)
			tab.Children(func() {
				closeButton := func() {
					close := ui.Row(c).Size(20, 20).Radius(4).
						AlignItems(ui.Center).Justify(ui.Center)
					if close.Hovered() {
						close.Background(oi.hover)
					}
					close.Children(func() {
						oiIconEl(c, "general/close", 14, oi, oi.icon)
					})
					if close.Clicked() {
						m.closeTabThen(t, nil)
					}
				}
				if !m.tabPrefs.NoCloseButton && m.tabPrefs.CloseButtonLeft {
					closeButton()
				}
				if !m.tabPrefs.HideFileIcon {
					if t.kind == tabKindAgentSession || t.kind == tabKindAgentHome {
						oiIconEl(c, "toolwindows/agents", 14, oi, oi.textMuted)
					} else {
						fileIconEl(c, oi, name, 14, oi.textMuted)
					}
				}
				if t.dirty() {
					ui.Text(c, "●").FontSize(9).TextColor(oi.accent)
				}
				label := ui.Text(c, name).FontSize(13).TextColor(oi.text).SingleLine()
				if squeeze {
					label = label.MinWidth(0).Ellipsis("…")
				}
				if t.preview {
					label = label.Italic()
				}
				if t.kind == tabKindAgentSession {
					if task, ok := m.agents.tasks[t.agentTask]; ok {
						ui.Text(c, "●").FontSize(8).
							TextColor(agentStatusColor(p, task.Status))
					}
				}
				if dir != "" {
					dirText := ui.Text(c, dir).FontSize(13).TextColor(oi.textFaint).
						PaddingStart(4).SingleLine()
					if squeeze {
						dirText.MinWidth(0).Ellipsis("…")
					}
				}
				if t.pinned {
					oiIconEl(c, "general/pin", 11, oi, oi.icon)
				}
				if !m.tabPrefs.NoCloseButton && !m.tabPrefs.CloseButtonLeft {
					closeButton()
				}
			})
			tab.ContextMenu(func(menu *ui.Menu) { m.tabMenu(menu, t) })
			if tab.Clicked() {
				m.activateIn(g.ref, i)
			}
		}
		if b := tab.Bounds(); b.W > 0 {
			rects = append(rects, b)
			others = append(others, j)
		}
		if g.focused {
			m.tabDragPress(tab, i, offset, len(tabs))
		}
	}
	if !lifted {
		return
	}
	if len(others) > 0 {
		to := tabDropIndex(m.tabDrag.At.X, m.tabDrag.At.Y, rects)
		m.tabDrag.To = offset + len(tabs)
		if to < len(others) {
			m.tabDrag.To = offset + others[to]
		}
	}
	m.tabGhost(c, oi, tabs[from])
}

// tabDragState is the tab the pointer holds: where it would drop, the point
// and box the press took it from, and where the pointer is now. Held turns
// false once the pointer lets go; Lifted marks the tab that left the strip.
type tabDragState struct {
	Held   bool
	Lifted bool
	From   int // strip index of the tab pressed
	To     int // insertion index in its row the tab would drop at
	Offset int // first strip index in the row the drag belongs to
	Count  int // number of tabs in that row
	Press  ui.Point
	At     ui.Point // the pointer now, in window DIPs
	Grab   ui.Point // where in the tab the press took it
	W, H   float32
}

// tabDragPress follows a pressed tab, as OpenIDEA's Tabs does: once the
// pointer moves past its threshold the tab lifts out of the strip, and
// letting go drops it where the strip made room.
func (m *Model) tabDragPress(tab ui.Element, i, offset, count int) {
	dx, dy, pressed := tab.Dragged()
	if !pressed {
		if m.tabDrag.Held {
			m.tabDrag.Held = false
			m.invalidate()
		}
		return
	}
	if !m.tabDrag.Held {
		lx, ly, _ := tab.PointerPosition()
		b := tab.Bounds()
		m.tabDrag = tabDragState{
			Held: true, From: i, To: i, Offset: offset, Count: count,
			Press: ui.Point{X: b.X + lx, Y: b.Y + ly},
			At:    ui.Point{X: b.X + lx, Y: b.Y + ly},
			Grab:  ui.Point{X: lx, Y: ly},
			W:     b.W, H: b.H,
		}
		return
	}
	if m.tabDrag.From != i {
		return
	}
	m.tabDrag.At = ui.Point{X: m.tabDrag.Press.X + dx, Y: m.tabDrag.Press.Y + dy}
	if !m.tabDrag.Lifted {
		ax, ay := m.tabDrag.At.X-m.tabDrag.Press.X, m.tabDrag.At.Y-m.tabDrag.Press.Y
		m.tabDrag.Lifted = ax*ax+ay*ay > 16
	}
}

// applyTabDrop drops a lifted tab whose pointer let go where the strip made room
// for it. Strips call it before they take their rows, so the drop lands
// between frames and the rows draw in the new order.
func (m *Model) applyTabDrop() {
	d := m.tabDrag
	if d.Held || !d.Lifted {
		return
	}
	m.tabDrag = tabDragState{}
	if d.From >= 0 && d.From < len(m.tabs) {
		if edge, ok := editorDropEdge(m.editorBodyRects[int(groupFocused)], d.At); ok {
			m.moveTabToEditorEdge(m.tabs[d.From], edge)
			return
		}
		m.tabs, m.active = reorderTabsInRange(m.tabs, m.active, d.From, d.To, d.Offset, d.Count)
	}
}

// reorderTabsInRange keeps a drag within the strip row where it started. A
// vertical strip can therefore reorder its column and still drop on an editor
// edge to create a group, without crossing a pinned/unpinned divider.
func reorderTabsInRange(tabs []*Tab, active, from, to, offset, count int) ([]*Tab, int) {
	start := max(0, offset)
	end := min(len(tabs), start+count)
	to = max(start, min(to, end))
	if from < start || from >= end || count <= 0 || to == from || to == from+1 {
		return tabs, active
	}
	localActive := active - start
	ordered, localActive := reorderTabs(tabs[start:end], localActive, from-start, to-start)
	out := append([]*Tab(nil), tabs...)
	copy(out[start:end], ordered)
	if active >= start && active < end {
		active = start + localActive
	}
	return out, active
}

// editorDropEdge maps a lifted tab's release point to the edge of an editor
// group. Releasing in the middle leaves the tab in its strip.
func editorDropEdge(rect ui.Rect, point ui.Point) (string, bool) {
	if rect.W <= 0 || rect.H <= 0 ||
		point.X < rect.X || point.X >= rect.X+rect.W ||
		point.Y < rect.Y || point.Y >= rect.Y+rect.H {
		return "", false
	}
	dx := min(point.X-rect.X, rect.X+rect.W-point.X)
	dy := min(point.Y-rect.Y, rect.Y+rect.H-point.Y)
	band := min(rect.W, rect.H) * 0.22
	if min(dx, dy) > band {
		return "", false
	}
	switch {
	case dx < dy && point.X < rect.X+rect.W/2:
		return "left", true
	case dx < dy:
		return "right", true
	case point.Y < rect.Y+rect.H/2:
		return "top", true
	default:
		return "bottom", true
	}
}

// moveTabToEditorEdge moves a tab into the other group and places that group
// at the edge the pointer chose. A group must keep at least one tab behind.
func (m *Model) moveTabToEditorEdge(t *Tab, edge string) bool {
	from, ok := m.groupOf(t)
	if !ok || from != groupFocused || len(m.tabs) < 2 {
		return false
	}
	switch edge {
	case "left", "right", "top", "bottom":
	default:
		return false
	}
	if m.split == nil {
		m.split = &splitGroup{}
	}
	m.splitDown = edge == "top" || edge == "bottom"
	m.moveToGroup(t, groupSplit)
	// moveToGroup focuses the destination. Keep that focused pane at the edge
	// where the user dropped it, even when the other pane already existed.
	m.splitFirst = edge == "left" || edge == "top"
	m.status = "Moved tab to the " + edge + " editor group"
	m.saveSession()
	m.invalidate()
	return true
}

// tabOrder returns the local places 0..n-1 that draw in order, with the tab
// at from spliced to insertion index to once lifted, as previewOrder splices
// it: to past from counts the tab itself, so dropping right after it keeps
// the strip as it was.
func tabOrder(n, from, to int, lifted bool) []int {
	order := make([]int, 0, n)
	for j := 0; j < n; j++ {
		if lifted && j == from {
			continue
		}
		order = append(order, j)
	}
	if !lifted {
		return order
	}
	at := to
	if to > from {
		at--
	}
	at = max(0, min(at, len(order)))
	order = append(order, 0)
	copy(order[at+1:], order[at:])
	order[at] = from
	return order
}

// reorderTabs returns tabs with the one at from moved to insertion index to
// and where the active tab went, as tabOrder draws it.
func reorderTabs(tabs []*Tab, active, from, to int) ([]*Tab, int) {
	if from < 0 || from >= len(tabs) || to < 0 || to > len(tabs) || to == from || to == from+1 {
		return tabs, active
	}
	order := tabOrder(len(tabs), from, to, true)
	ordered := make([]*Tab, len(tabs))
	at := active
	for j, i := range order {
		ordered[j] = tabs[i]
		if i == active {
			at = j
		}
	}
	return ordered, at
}

// tabDropIndex is OpenIDEA's dropIndex: the insertion index for a drop at
// (x, y) among boxes laid out in reading order, possibly wrapped into rows.
// Above or below the tabs counts as their first or last row.
func tabDropIndex(x, y float32, rects []ui.Rect) int {
	first, last := rects[0], rects[len(rects)-1]
	py := max(first.Y, min(y, last.Y+last.H-1))
	for i, r := range rects {
		if py < r.Y || (py < r.Y+r.H && x < r.X+r.W/2) {
			return i
		}
	}
	return len(rects)
}

// tabGhost draws the tab the pointer lifted over the workbench, as OpenIDEA's
// drag ghost: the tab's face on the popup colour a little translucent, above
// the rest of the window.
func (m *Model) tabGhost(c *ui.Context, oi oiColors, t *Tab) {
	d := m.tabDrag
	ui.Overlay(c, func() {
		if edge, ok := editorDropEdge(m.editorBodyRects[int(groupFocused)], d.At); ok {
			r := tabDropPreviewRect(m.editorBodyRects[int(groupFocused)], edge)
			ui.Row(c.Key("tabSplitDrop")).Absolute().Left(r.X).Top(r.Y).
				Width(r.W).Height(r.H).Radius(6).Background(oi.accent.Alpha(0.16)).
				Border(1, oi.accent.Alpha(0.7))
		}
		g := ui.Row(c.Key("tabGhost")).Absolute().
			Left(d.At.X-d.Grab.X).Top(d.At.Y-d.Grab.Y).
			Width(d.W).Height(d.H).
			Padding(0, 10).Gap(6).Radius(6).AlignItems(ui.Center).
			Background(oi.popup).BorderWidth(1).BorderColor(oi.borderStrong).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.35)).Opacity(0.85)
		g.Children(func() {
			if !m.tabPrefs.HideFileIcon {
				name, _ := m.tabLabel(t)
				fileIconEl(c, oi, name, 14, oi.textMuted)
			}
			if t.dirty() {
				ui.Text(c, "●").FontSize(9).TextColor(oi.accent)
			}
			name, dir := m.tabLabel(t)
			label := ui.Text(c, name).FontSize(13).TextColor(oi.text).
				SingleLine().MinWidth(0).Ellipsis("…")
			if t.preview {
				label = label.Italic()
			}
			if dir != "" {
				ui.Text(c, dir).FontSize(13).TextColor(oi.textFaint).
					PaddingStart(4).SingleLine().MinWidth(0).Ellipsis("…")
			}
			if t.pinned {
				oiIconEl(c, "general/pin", 11, oi, oi.icon)
			}
			if !m.tabPrefs.NoCloseButton {
				oiIconEl(c, "general/close", 14, oi, oi.icon)
			}
		})
	})
}

// tabDropPreviewRect is the half of an editor group that a tab dropped on the
// given edge will occupy.
func tabDropPreviewRect(rect ui.Rect, edge string) ui.Rect {
	const gap = float32(4)
	switch edge {
	case "left":
		return ui.Rect{X: rect.X + gap, Y: rect.Y + gap, W: max(0, rect.W/2-gap*1.5), H: max(0, rect.H-gap*2)}
	case "right":
		return ui.Rect{X: rect.X + rect.W/2 + gap/2, Y: rect.Y + gap, W: max(0, rect.W/2-gap*1.5), H: max(0, rect.H-gap*2)}
	case "top":
		return ui.Rect{X: rect.X + gap, Y: rect.Y + gap, W: max(0, rect.W-gap*2), H: max(0, rect.H/2-gap*1.5)}
	case "bottom":
		return ui.Rect{X: rect.X + gap, Y: rect.Y + rect.H/2 + gap/2, W: max(0, rect.W-gap*2), H: max(0, rect.H/2-gap*1.5)}
	}
	return ui.Rect{}
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
			hint("search", "Search Everywhere", "", m.OpenSearchEverywhere)
			hint("goto", "Go to File", "⌘⇧O", func() { m.RunCommand("navigate.gotoFile") })
			hint("recent", "Recent Files", "⌘E", func() { m.RunCommand("navigate.recentFiles") })
			ui.Row(c).Padding(0, 0, 0, 0).Children(func() {
				ui.Text(c, "Drop files here to open them").FontSize(13).
					TextColor(oi.textFaint).SingleLine()
			})
		})
	})
}

// bottomTool names the window the bottom area shows, if any.
func (m *Model) bottomTool() string {
	if id := m.primaryTool(); id != "" && !m.undockedWindow(id) {
		return id
	}
	if id := m.splitTool(); id != "" && !m.undockedWindow(id) {
		return id
	}
	return ""
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
	tk := oiTokensFor(m, p)
	bar := ui.Row(c).Height(tk.StatusBarHeight).Shrink(0).Padding(0, 10, 0, 12).
		Gap(4).AlignItems(ui.Center)
	bar.ContextMenu(func(menu *ui.Menu) { m.statusBarMenu(menu) })
	bar.Children(func() {
		for _, it := range m.statusSide("left") {
			it.Render(m, c, p, oi)
		}
		ui.Spacer(c)
		m.taskStatus(c, p, oi)
		for _, it := range m.statusSide("right") {
			it.Render(m, c, p, oi)
		}
	})
}

// statusBarMenu is OpenIDEA's status bar context menu: every titled widget in
// bar order, including one currently hidden by its own choice.
func (m *Model) statusBarMenu(menu *ui.Menu) {
	for _, it := range m.statusWidgetRows() {
		id := it.ID
		if menu.Item(it.Title).Checked(m.statusItemEnabled(it)).Chosen() {
			m.SetStatusBarWidgetVisible(id, !m.statusItemEnabled(it))
		}
	}
}

// tabMenu is OpenIDEA's tab context menu: the close family, whose bulk closes
// leave pinned tabs alone, then Pin or Unpin and Keep Tab Open for the clicked
// tab, the file actions and the editor-tabs settings the menu opens.
func (m *Model) tabMenu(menu *ui.Menu, t *Tab) {
	g, ok := m.groupOf(t)
	if !ok {
		return
	}
	tabs := m.groupTabsAt(g)
	at := tabIndexIn(tabs, t)
	path := tabPath(t)
	if t.kind == "" {
		m.compileFileMenu(menu, path)
		if strings.EqualFold(filepath.Ext(path), ".go") {
			menu.Submenu("Go Generate", func(sub *ui.Menu) {
				if sub.Item("Go Generate File").Chosen() {
					m.RunCommandWithArg("go.generate.file", map[string]any{"path": path})
				}
				if sub.Item("Go Generate Package").Chosen() {
					m.RunCommandWithArg("go.generate.package", map[string]any{"path": path})
				}
			})
		}
	}
	if menu.Item("Close").Shortcut(ui.Cmd, ui.KeyW).Chosen() {
		m.closeTabThen(t, nil)
	}
	if menu.Item("Close Other Tabs").Disabled(len(tabs) < 2).Chosen() {
		m.CloseOtherTabs(t)
	}
	if menu.Item("Close Tabs to the Left").Disabled(at <= 0).Chosen() {
		m.CloseTabsToLeft(t)
	}
	if menu.Item("Close Tabs to the Right").Disabled(at < 0 || at >= len(tabs)-1).Chosen() {
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
	if t.kind == tabKindAgentSession {
		if menu.Item("Open Agent Task in Tool Window").Chosen() {
			m.dockAgentTask(t.agentTask)
		}
	} else if t.kind == tabKindAgentHome {
		if menu.Item("Open in Tool Window").Chosen() {
			m.useAgentToolWindow()
		}
	}
	if t.preview {
		if menu.Item("Keep Tab Open").Chosen() {
			m.promoteTab(t)
		}
	}
	menu.Separator()
	// The split entries act on the tab's own group, as OpenIDEA's tab menu
	// does; the opposite-group entries appear once the editor has two groups.
	// A tab in the unfocused group takes the keyboard first, so the commands
	// that act on the active tab act on this one.
	focusT := func() {
		if g != groupFocused {
			m.focusGroup(groupSplit)
		}
	}
	if menu.Item("Split and Move Right").Chosen() {
		focusT()
		m.SplitAndMoveRight()
	}
	if menu.Item("Split and Move Down").Chosen() {
		focusT()
		m.SplitAndMoveDown()
	}
	if menu.Item("Split Right").Shortcut(ui.Cmd, ui.KeyBackslash).Chosen() {
		focusT()
		m.SplitEditorRight()
	}
	if menu.Item("Split Down").Chosen() {
		focusT()
		m.SplitEditorDown()
	}
	if m.split != nil {
		if menu.Item("Move to Opposite Group").Chosen() {
			focusT()
			m.MoveToOppositeGroup()
		}
		if menu.Item("Open in Opposite Group").Chosen() {
			focusT()
			m.OpenInOppositeGroup()
		}
		if menu.Item("Change Splitter Orientation").Chosen() {
			m.ChangeSplitterOrientation()
		}
		if menu.Item("Unsplit").Chosen() {
			m.Unsplit()
		}
		if menu.Item("Unsplit All").Chosen() {
			m.UnsplitAll()
		}
	}
	menu.Separator()
	if path != "" && t.kind == "" {
		m.tabOpenInMenu(menu, path)
		menu.Separator()
		menu.Submenu("Local History", func(menu *ui.Menu) {
			if menu.Item("Show History").Chosen() {
				m.ShowHistory(path)
			}
			if menu.Item("Put Label…").Chosen() {
				m.OpenHistoryLabel()
			}
		})
		if m.Root() != "" && inProject(m.Root(), path) {
			menu.Submenu("Git", func(menu *ui.Menu) {
				m.tabGitMenu(menu, path)
			})
		}
		menu.Separator()
		if menu.Item("Compare With…").Chosen() {
			m.PickCompareFile(t)
		}
		if menu.Item("Compare with Clipboard").Chosen() {
			m.CompareTabWithClipboard(t)
		}
		menu.Separator()
		menu.Submenu("Copy Path/Reference…", func(menu *ui.Menu) {
			for _, f := range tabPathFlavours(m.Root(), path) {
				if menu.Item(f.label).Chosen() {
					m.copyEditorText(f.value)
				}
			}
		})
		if menu.Item("Rename File…").Chosen() {
			m.OpenFileRenameDialog(path)
		}
		m.fileTypeOverrideMenu(menu, path)
		menu.Separator()
	}
	if menu.Item("Configure Editor Tabs…").Chosen() {
		m.OpenSettingsPage(settingsPageEditorTabs)
	}
}

// tabOpenInMenu is OpenIDEA's Open In › for a tab's file: the project view,
// the file manager and a terminal at the file's folder.
func (m *Model) tabOpenInMenu(menu *ui.Menu, path string) {
	menu.Submenu("Open In", func(menu *ui.Menu) {
		if menu.Item("Project View").Chosen() {
			m.selectInProjectView(path)
		}
		if menu.Item(revealLabel()).Chosen() {
			revealPath(path)
		}
		if menu.Item("Terminal").Chosen() {
			m.OpenTerminalAt(filepath.Dir(path))
		}
		if isWebPath(path) {
			menu.Submenu("Browser", func(sub *ui.Menu) {
				if sub.Item("Default Browser").Chosen() {
					m.OpenWebInBrowser(path)
				}
				if isHTMLPath(path) &&
					sub.Item("Built-in Preview").Checked(m.remote.webPreviews[path]).Chosen() {
					m.ToggleWebPreview(path)
				}
			})
		}
	})
}

// tabGitMenu is OpenIDEA's Git › for a tab's file.
func (m *Model) tabGitMenu(menu *ui.Menu, path string) {
	f, changed := m.statusFor(path)
	kind := effectiveVCSKind(f)
	if menu.Item("Commit File…").Disabled(!changed || kind == "conflicted").Chosen() {
		m.OpenCommitFile(path)
	}
	if menu.Item("Add").Disabled(kind != "untracked").Chosen() {
		m.StageChange(f)
	}
	menu.Separator()
	annotate := "Annotate with Git Blame"
	if m.annotationsShown(path) {
		annotate = "Close Annotations"
	}
	if menu.Item(annotate).Chosen() {
		m.ShowAnnotate(path)
	}
	if menu.Item("Show Diff").Chosen() {
		m.ShowDiff(path)
	}
	if menu.Item("Compare with Branch or Revision…").Chosen() {
		m.OpenCompareWithRevision(map[string]any{"path": path})
	}
	if menu.Item("Compare with Revision…").Chosen() {
		m.OpenCompareWithRevision(map[string]any{"path": path})
	}
	if menu.Item("Show History").Chosen() {
		m.ShowHistory(path)
	}
	if menu.Item("Show History for Selection").Chosen() {
		m.ShowHistoryForSelection()
	}
	if menu.Item("Show Current Revision").Chosen() {
		m.ShowCurrentRevision(map[string]any{"path": path})
	}
	if menu.Item("Rollback…").Disabled(!changed || f.Unstaged == nil).Chosen() {
		m.AskRollback(f)
	}
	menu.Separator()
	if menu.Item("Branches…").Chosen() {
		m.ShowBranches()
	}
	if menu.Item("New Branch…").Chosen() {
		m.OpenNewGitBranch()
	}
	if menu.Item("New Tag…").Chosen() {
		m.OpenNewGitTag()
	}
	if menu.Item("Checkout Tag or Revision…").Chosen() {
		m.OpenCheckoutRevision("")
	}
	if menu.Item("Worktrees…").Chosen() {
		m.OpenVCSWorktrees()
	}
	if menu.Item("New Worktree…").Chosen() {
		m.OpenNewVCSWorktree()
	}
	if menu.Item("Manage Remotes…").Chosen() {
		m.OpenGitRemotes()
	}
	if menu.Item("Resolve Conflicts…").Chosen() {
		m.OpenResolveConflicts()
	}
	menu.Separator()
	if menu.Item("Pull…").Chosen() {
		m.OpenGitPull()
	}
	if menu.Item("Push…").Chosen() {
		m.OpenGitPush()
	}
	if menu.Item("Fetch").Chosen() {
		m.GitFetch()
	}
	if menu.Item("Merge…").Chosen() {
		m.OpenGitMerge()
	}
	if menu.Item("Rebase…").Chosen() {
		m.OpenGitRebase()
	}
	menu.Separator()
	if menu.Item("Stash Changes…").Chosen() {
		m.OpenGitStash()
	}
	if menu.Item("Unstash Changes…").Chosen() {
		m.OpenGitUnstash()
	}
	if menu.Item("Reset HEAD…").Chosen() {
		m.OpenGitReset()
	}
}
