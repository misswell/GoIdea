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
	oiStripeWidth     float32 = 40 // 30px button + 4px padding + the island gap
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
// then the run widget, the search and terminal controls and the gear.
func (m *Model) titleBar(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	left, right := resolveToolbar(m.prefs.MainToolbar)
	bar := ui.Row(c).Height(oiTitleBarHeight).Shrink(0).Gap(4).AlignItems(ui.Center).
		Padding(0, 8, 0, 10)
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

// toolbarEntry draws one title bar entry of the saved layout. An entry this
// build has no widget for is skipped, as a contribution that is not
// registered is.
func (m *Model) toolbarEntry(c *ui.Context, p *ui.Theme, oi oiColors, id string) {
	switch id {
	case toolbarProject:
		m.projectWidget(c, p, oi)
	case toolbarVCS:
		if label := m.branchLabel(); label != "" {
			m.branchWidget(c, p, oi, label)
		} else {
			m.vcsFallbackWidget(c, p, oi)
		}
	case toolbarRun:
		m.runWidget(c, p, oi)
	case toolbarSearch:
		m.titleTool(c, p, oi, "search", "Search Everywhere", iconWelcomeSearch, func() {
			m.RunCommand(toolbarAction(toolbarSearch))
		})
	case toolbarTerminal:
		m.titleTool(c, p, oi, "term", "Terminal", iconTerminal, func() {
			m.RunCommand(toolbarAction(toolbarTerminal))
		})
	}
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
			m.RunCommand("project.open")
		}
		if menu.Item("Manage Projects…").Chosen() {
			m.RunCommand("welcome.manageProjects")
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
		ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(oi.textSubtle)
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
	b.Menu(func(menu *ui.Menu) { addMenuEntries(menu, m, gearGroup(m)) })
}

// addMenuEntries fills a MyGo menu with entries of the shared menu model, so
// the gear, the project widget and the menu bar run the same actions.
func addMenuEntries(menu *ui.Menu, m *Model, entries []menuEntry) {
	for _, e := range entries {
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
		if m.maximized && bottom != "" {
			// A maximized tool window takes the workbench, as OpenIDEA
			// hides the editor island over it.
			m.bottomSlot(c, p)
		} else {
			ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).
				AlignItems(ui.Stretch).Padding(0, 6, 0, 6).Children(func() {
				m.toolStripe(c, p, oi)
				m.leftSide(c, p, oi)
				// The gap beside the side takes the pointer, as OpenIDEA's
				// GapHandle does; the editor keeps the island gap from the
				// stripe when the side is closed.
				if m.leftShown() {
					m.leftWidthHandle(c, oi)
				} else {
					ui.Box(c).Width(oiIslandGap).Shrink(0)
				}
				m.editorIsland(c, p)
			})
			if bottom != "" {
				ui.Column(c).Shrink(0).Padding(0, 6, 0, 6).
					Height(float32(m.bottomDrawHeight()) + oiIslandGap).Children(func() {
					// The gap above the bottom island takes the pointer, as
					// OpenIDEA's GapHandle does.
					m.bottomSizeHandle(c)
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
	ui.Column(c).Width(oiStripeWidth).Shrink(0).Padding(5, oiIslandGap, 6, 4).
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

// leftShown reports whether the left side draws: a window in either of its
// areas.
func (m *Model) leftShown() bool {
	return !m.leftHidden || m.leftSplit != ""
}

// leftSide is the left side's two areas: the primary island and, below it,
// the split island, with the gap between them the pointer drags. A lone
// island takes the whole side, as OpenIDEA's SidePanel does, and hiding both
// closes the side.
func (m *Model) leftSide(c *ui.Context, p *ui.Theme, oi oiColors) {
	if !m.leftShown() {
		return
	}
	both := !m.leftHidden && m.leftSplit != ""
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
		if m.leftSplit != "" {
			m.leftSplitIsland(c, p, oi, splitGrow)
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
	title := "Project"
	if m.sideMode == "vcs" {
		title = "Commit"
		if n := len(m.vcs.files); n > 0 {
			title = fmt.Sprintf("Commit · %d", n)
		}
	}
	ui.Column(c).Grow(grow).MinHeight(0).Background(oi.island).
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
// one: Structure or Bookmarks, with a hide action in its header. grow is its
// share of the side, which the drag of the gap between the islands moves.
func (m *Model) leftSplitIsland(c *ui.Context, p *ui.Theme, oi oiColors, grow float32) {
	title := "Structure"
	if m.leftSplit == "bookmarks" {
		title = "Bookmarks"
	}
	ui.Column(c).Grow(grow).MinHeight(0).Background(oi.island).
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
	island := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(oi.island).
		Radius(oiIslandRadius)
	// The width it drew at last frame caps a drag of the side beside it, and
	// the height caps a drag of the bottom area below it.
	m.editorWidth = island.Bounds().W
	m.editorHeight = island.Bounds().H
	island.Children(func() {
		if !m.tabPrefs.AtBottom() {
			m.editorTabs(c, p, oi)
		}
		if m.banner != "" {
			m.bannerRow(c, p)
		}
		m.editorArea(c, p)
		if m.tabPrefs.AtBottom() {
			m.editorTabs(c, p, oi)
		}
	})
}

// editorTabs is OpenIDEA's tab bar: 40px high on the island, 28px tabs with
// the selected one tinted and underlined in the accent. Editor Tabs decides
// where it sits, whether pinned tabs get their own row, what each tab shows
// and where its close button is.
func (m *Model) editorTabs(c *ui.Context, p *ui.Theme, oi oiColors) {
	if len(m.tabs) == 0 || m.tabPrefs.Hidden() {
		return
	}
	pinned := 0
	for _, t := range m.tabs {
		if t.pinned {
			pinned++
		}
	}
	if m.tabPrefs.PinnedSeparateRow && pinned > 0 && pinned < len(m.tabs) {
		ui.Column(c).Shrink(0).Children(func() {
			m.tabStrip(c, p, oi, m.tabs[:pinned], 0)
			m.tabStrip(c, p, oi, m.tabs[pinned:], pinned)
		})
		return
	}
	m.tabStrip(c, p, oi, m.tabs, 0)
}

// tabStrip is the tab bar with the island's bottom border; offset is the
// row's first tab in the strip. Show tabs in decides how tabs that don't fit
// go: one row scrolls sideways or squeezes them, multiple rows wrap.
func (m *Model) tabStrip(c *ui.Context, p *ui.Theme, oi oiColors, tabs []*Tab, offset int) {
	reveal := m.tabsShown != m.active
	m.tabsShown = m.active
	strip := ui.Row(c).Shrink(0).Padding(0, 4).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border)
	if m.tabPrefs.MultipleRows {
		strip.MinHeight(oiTabBarHeight)
	} else {
		strip.Height(oiTabBarHeight)
	}
	strip.Children(func() {
		switch {
		case m.tabPrefs.MultipleRows:
			ui.Row(c).Grow(1).MinWidth(0).Wrap().Gap(2).Padding(3, 0).
				AlignItems(ui.Center).AlignContent(ui.Center).Children(func() {
				m.tabButtons(c, p, oi, tabs, offset, reveal)
			})
		case m.tabPrefs.SqueezeTabs:
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				m.tabButtons(c, p, oi, tabs, offset, reveal)
			})
		default:
			ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Gap(2).
				AlignItems(ui.Center).TrackScroll(&m.tabsScroll).Children(func() {
				m.tabButtons(c, p, oi, tabs, offset, reveal)
			})
		}
	})
}

// tabButtons draws tabs in a row, offset being the first tab's index in the
// strip. A squeezed strip lets the labels ellipsize down to the tab's fixed
// parts, and a change of active tab scrolls the new one into view.
func (m *Model) tabButtons(c *ui.Context, p *ui.Theme, oi oiColors, tabs []*Tab, offset int, reveal bool) {
	squeeze := m.tabPrefs.SqueezeTabs
	for j, t := range tabs {
		i, t := offset+j, t
		tab := ui.Row(c.Key(fmt.Sprintf("tab:%d", i))).Height(oiTabHeight).
			Padding(0, 10).Gap(6).Radius(6).AlignItems(ui.Center)
		if i == m.active {
			if reveal {
				tab.ScrollIntoView()
			}
			tab.Background(oi.tabSelected).
				BorderWidth(0, 0, 2, 0).BorderColor(oi.accent)
		} else if tab.Hovered() {
			tab.Background(oi.toolbarHover)
		}
		if !m.tabPrefs.HideFullPathOnHover {
			if path := tabPath(t); path != "" {
				tab.Tooltip(path)
			}
		}
		name, dir := m.tabLabel(t)
		tab.Children(func() {
			closeButton := func() {
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
			}
			if !m.tabPrefs.NoCloseButton && m.tabPrefs.CloseButtonLeft {
				closeButton()
			}
			if !m.tabPrefs.HideFileIcon {
				ui.Icon(c, iconFile).Size(14, 14).TextColor(oi.textMuted)
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
			if dir != "" {
				dirText := ui.Text(c, dir).FontSize(13).TextColor(oi.textFaint).
					PaddingStart(4).SingleLine()
				if squeeze {
					dirText.MinWidth(0).Ellipsis("…")
				}
			}
			if t.pinned {
				ui.Icon(c, iconPin).Size(11, 11).TextColor(oi.textMuted)
			}
			if !m.tabPrefs.NoCloseButton && !m.tabPrefs.CloseButtonLeft {
				closeButton()
			}
		})
		tab.ContextMenu(func(menu *ui.Menu) { m.tabMenu(menu, t) })
		if tab.Clicked() {
			m.activate(i)
		}
	}
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
