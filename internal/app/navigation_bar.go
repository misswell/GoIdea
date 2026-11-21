package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// navigationCrumb is one project › directory › file › symbol segment of the
// workbench navigation bar.
type navigationCrumb struct {
	Label  string
	Title  string
	Path   string
	Dir    bool
	Symbol *openidea.OutlineItem
}

// navigationBarState is the Jump to Navigation Bar popup's snapshot. The
// segments stay fixed while it is open, as IDEA's popup does, and the child
// list can descend through folders without changing the editor underneath.
type navigationBarState struct {
	crumbs   []navigationCrumb
	index    int
	listOpen bool
	sel      int
	children []navigationCrumb
	listErr  string

	// segmentReveal and childReveal ask the next frame to scroll the chosen
	// segment or child into view. They clear after that frame so a later user
	// wheel is not immediately undone.
	segmentReveal bool
	childReveal   bool
	scroll        ui.ScrollState
	childScroll   ui.ScrollState
	focus         ui.Handle
}

// navigationChild is one row of the popup's child list.
type navigationChild = navigationCrumb

func (m *Model) navigationBarPosition() string {
	switch m.prefs.Appearance.NavigationBar {
	case "top", "hidden":
		return m.prefs.Appearance.NavigationBar
	default:
		return "statusBar"
	}
}

// SetNavigationBarPosition applies View › Appearance › Navigation Bar.
func (m *Model) SetNavigationBarPosition(position string) {
	switch position {
	case "top", "statusBar", "hidden":
	default:
		return
	}
	if m.navigationBarPosition() == position {
		return
	}
	m.prefs.Appearance.NavigationBar = position
	m.savePreferences()
	m.invalidate()
}

// ShowNavigationBar is Navigate › Jump to Navigation Bar. The normal top or
// status placement stays in place; this floating instance gives the keyboard
// path access to the same project, directory and symbol segments.
func (m *Model) ShowNavigationBar() {
	crumbs := m.navigationCrumbs()
	if len(crumbs) == 0 {
		return
	}
	m.navigation = navigationBarState{
		crumbs:        append([]navigationCrumb(nil), crumbs...),
		index:         navigationFileIndex(crumbs),
		segmentReveal: true,
	}
	m.navigationOpen = true
	m.invalidate()
}

func (m *Model) closeNavigationBar() {
	if !m.navigationOpen {
		return
	}
	m.navigationOpen = false
	m.navigation.listOpen = false
	m.navigation.children = nil
	m.navigation.listErr = ""
	m.invalidate()
}

func (m *Model) navigationBarPopup(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	w, _ := c.Size()
	width := min(float32(920), w-40)
	ui.Overlay(c, func() {
		box := ui.Column(c.Key("navigation:jump")).Role(ui.RolePopup).Label("Navigation Bar").
			Focusable().AutoFocus().Bind(&m.navigation.focus).
			Absolute().Left((w - width) / 2).Top(48).
			Width(width).Padding(6).Background(oi.popup).
			BorderWidth(1).BorderColor(oi.borderStrong).Radius(8)
		box.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Navigation Bar").FontSize(12).Bold().TextColor(oi.text).
					SingleLine().Grow(1)
				close := ui.ButtonBase(c.Key("navigation:close")).Size(22, 22).
					Radius(4).Justify(ui.Center).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if close.Hovered() {
					close.Background(oi.toolbarHover)
				}
				close.Children(func() { oiIconEl(c, "general/close", 14, oi, oi.textMuted) })
				if close.Clicked() {
					m.closeNavigationBar()
				}
			})
			m.renderNavigationCrumbs(c, p, oi, m.navigation.crumbs, true)
			m.renderNavigationChildren(c, p, oi)
		})
		switch {
		case box.PressedOutside():
			m.closeNavigationBar()
		case box.OverlayShortcut(0, ui.KeyEscape):
			m.escapeNavigationBar()
		case box.OverlayShortcut(0, ui.KeyLeft):
			m.stepNavigationSegment(-1)
		case box.OverlayShortcut(0, ui.KeyRight):
			m.stepNavigationSegment(1)
		case box.OverlayShortcut(0, ui.KeyDown):
			m.stepNavigationList(1)
		case box.OverlayShortcut(0, ui.KeyUp):
			m.stepNavigationList(-1)
		case box.OverlayShortcut(0, ui.KeyEnter):
			m.enterNavigationBar()
		}
	})
}

// navigationFileIndex starts the keyboard on the path's file segment, which
// is where IDEA starts its floating navigation bar.
func navigationFileIndex(crumbs []navigationCrumb) int {
	for i, crumb := range crumbs {
		if !crumb.Dir && crumb.Symbol == nil {
			return i
		}
	}
	return 0
}

// stepNavigationSegment clamps left and right movement at the ends.
func stepNavigationSegment(index, count, delta int) int {
	if count <= 0 {
		return 0
	}
	return min(max(0, index+delta), count-1)
}

func (m *Model) stepNavigationSegment(delta int) {
	if !m.navigationOpen || len(m.navigation.crumbs) == 0 {
		return
	}
	next := stepNavigationSegment(m.navigation.index, len(m.navigation.crumbs), delta)
	if next == m.navigation.index {
		return
	}
	m.navigation.index = next
	m.navigation.segmentReveal = true
	if m.navigation.listOpen {
		m.openNavigationChildren(next)
	}
	m.invalidate()
}

func (m *Model) escapeNavigationBar() {
	if m.navigation.listOpen {
		m.navigation.listOpen = false
		m.navigation.children = nil
		m.navigation.listErr = ""
		m.invalidate()
		return
	}
	m.closeNavigationBar()
}

func (m *Model) enterNavigationBar() {
	if m.navigation.listOpen {
		m.chooseNavigationChild()
		return
	}
	m.openNavigationChildren(m.navigation.index)
}

func (m *Model) stepNavigationList(delta int) {
	if !m.navigationOpen {
		return
	}
	if !m.navigation.listOpen {
		if delta > 0 {
			m.openNavigationChildren(m.navigation.index)
		}
		return
	}
	if len(m.navigation.children) == 0 {
		return
	}
	next := min(max(0, m.navigation.sel+delta), len(m.navigation.children)-1)
	if next == m.navigation.sel {
		return
	}
	m.navigation.sel = next
	m.navigation.childReveal = true
	m.invalidate()
}

func (m *Model) openNavigationChildren(index int) {
	if index < 0 || index >= len(m.navigation.crumbs) {
		return
	}
	crumb := m.navigation.crumbs[index]
	children, err := m.navigationChildren(crumb)
	m.navigation.listOpen = true
	m.navigation.sel = 0
	m.navigation.children = children
	m.navigation.listErr = ""
	m.navigation.childScroll.Y = 0
	if err != nil {
		m.navigation.listErr = err.Error()
	}
	if index+1 < len(m.navigation.crumbs) {
		next := m.navigation.crumbs[index+1]
		for i, child := range children {
			if sameNavigationCrumb(child, next) {
				m.navigation.sel = i
				break
			}
		}
	}
	m.navigation.childReveal = true
	m.invalidate()
}

func sameNavigationCrumb(a, b navigationCrumb) bool {
	return realPath(a.Path) == realPath(b.Path) && a.Label == b.Label
}

func (m *Model) navigationChildren(crumb navigationCrumb) ([]navigationChild, error) {
	switch {
	case crumb.Dir:
		return navigationDirectoryChildren(crumb.Path)
	case crumb.Symbol != nil:
		return navigationSymbolChildren(*crumb.Symbol), nil
	default:
		t := m.tab()
		if t == nil || t.doc == nil || realPath(t.doc.Path()) != realPath(crumb.Path) {
			return nil, nil
		}
		return navigationSymbolChildren(openidea.OutlineItem{Children: t.outline}), nil
	}
}

// navigationDirectoryChildren lists a folder's visible immediate children,
// directories first, matching the navigation bar's folder popup.
func navigationDirectoryChildren(path string) ([]navigationChild, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	children := make([]navigationChild, 0, min(len(entries), 200))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if len(children) == 200 {
			break
		}
		childPath := filepath.Join(path, entry.Name())
		children = append(children, navigationChild{
			Label: entry.Name(), Title: childPath, Path: childPath, Dir: entry.IsDir(),
		})
	}
	return children, nil
}

func navigationSymbolChildren(item openidea.OutlineItem) []navigationChild {
	children := make([]navigationChild, 0, len(item.Children))
	for i := range item.Children {
		child := item.Children[i]
		children = append(children, navigationChild{
			Label: child.Name, Title: symbolTooltip(child), Symbol: &child,
		})
	}
	return children
}

func (m *Model) chooseNavigationChild() {
	if !m.navigationOpen || !m.navigation.listOpen ||
		m.navigation.sel < 0 || m.navigation.sel >= len(m.navigation.children) {
		return
	}
	child := m.navigation.children[m.navigation.sel]
	switch {
	case child.Dir:
		end := m.navigation.index + 1
		crumbs := append([]navigationCrumb(nil), m.navigation.crumbs[:end]...)
		m.navigation.crumbs = append(crumbs, child)
		m.navigation.index = len(m.navigation.crumbs) - 1
		m.navigation.segmentReveal = true
		m.openNavigationChildren(m.navigation.index)
	case child.Symbol != nil:
		t := m.tab()
		m.closeNavigationBar()
		if t != nil && t.doc != nil {
			m.goToOutlineItem(t, *child.Symbol)
		}
	default:
		path := child.Path
		m.closeNavigationBar()
		m.openFile(path)
	}
}

func (m *Model) renderNavigationChildren(c *ui.Context, p *ui.Theme, oi oiColors) {
	if !m.navigation.listOpen {
		return
	}
	ui.Row(c).Shrink(0).Height(1).Margin(4, 0, 0, 0).Background(oi.border)
	height := min(float32(240), float32(len(m.navigation.children))*24+8)
	ui.Scroll(c.Key("navigation:children")).TrackScroll(&m.navigation.childScroll).
		Height(max(32, height)).Shrink(0).Children(func() {
		ui.Column(c).Padding(4, 0).Children(func() {
			if m.navigation.listErr != "" {
				ui.Text(c, "Cannot read this folder: "+m.navigation.listErr).Padding(6, 8).
					FontSize(12).TextColor(oi.warning).SingleLine().Ellipsis("…")
				return
			}
			if len(m.navigation.children) == 0 {
				ui.Text(c, "Nothing here").Padding(6, 8).FontSize(12).
					TextColor(oi.textMuted).SingleLine()
				return
			}
			for i, child := range m.navigation.children {
				child := child
				selected := i == m.navigation.sel
				row := ui.Row(c.Key(fmt.Sprintf("navigation:child:%d:%s:%s",
					i, child.Path, child.Label))).Height(24).Padding(0, 7).
					Gap(7).Radius(4).AlignItems(ui.Center)
				if selected {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.toolbarHover)
				}
				if row.Hovered() && !selected {
					m.navigation.sel = i
					m.navigation.childReveal = true
					m.invalidate()
				}
				row.Tooltip(child.Title)
				row.OnClick(func() {
					m.navigation.sel = i
					m.chooseNavigationChild()
				})
				row.Children(func() {
					tone := oi.textMuted
					if selected {
						tone = oi.text
					}
					switch {
					case child.Dir:
						oiIconEl(c, "nodes/folder", 14, oi, tone)
					case child.Symbol != nil:
						asset := symbolIcon(strings.ToLower(child.Symbol.Kind))
						if asset == "" {
							asset = "nodes/method"
						}
						oiIconEl(c, asset, 14, oi, tone)
					default:
						fileIconEl(c, oi, child.Label, 14, tone)
					}
					ui.Text(c, child.Label).FontSize(12).TextColor(tone).
						SingleLine().Ellipsis("…")
				})
				if selected && m.navigation.childReveal {
					row.ScrollIntoView()
				}
			}
		})
	})
	m.navigation.childReveal = false
}

func (m *Model) navigationCrumbs() []navigationCrumb {
	t := m.tab()
	if t == nil || t.kind != "" || t.doc == nil || t.doc.Path() == "" {
		return nil
	}
	path := realPath(t.doc.Path())
	target := filepath.Dir(path)
	root := realPath(m.Root())
	var out []navigationCrumb

	inProject := root != "" && target != root && !strings.HasPrefix(target, root+string(filepath.Separator))
	if root != "" && !inProject {
		out = append(out, navigationCrumb{
			Label: m.projectLabel(), Title: root, Path: root, Dir: true,
		})
		rel, err := filepath.Rel(root, target)
		if err == nil && rel != "." {
			dirs := strings.Split(filepath.ToSlash(rel), "/")
			shown := dirs
			if len(dirs) > 4 {
				shown = append(append([]string{dirs[0]}, "…"), dirs[len(dirs)-2:]...)
			}
			for i, label := range shown {
				if label == "…" {
					out = append(out, navigationCrumb{
						Label: label, Title: strings.Join(dirs, "/"), Path: target,
					})
					continue
				}
				consumed := i
				if len(dirs) > 4 && i >= 2 {
					consumed = len(dirs) - 2 + i - 2
				}
				at := root
				for _, part := range dirs[:consumed+1] {
					at = filepath.Join(at, part)
				}
				out = append(out, navigationCrumb{Label: label, Title: at, Path: at, Dir: true})
			}
		}
	} else {
		out = append(out, navigationCrumb{
			Label: filepath.Base(target), Title: target, Path: target, Dir: true,
		})
	}

	name := filepath.Base(path)
	out = append(out, navigationCrumb{Label: name, Title: path, Path: path})
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem == "" {
		stem = name
	}
	pos := t.state.CursorPos()
	chain := outlinePath(t.outline, pos.Line)
	for i, item := range chain {
		if i == 0 && item.Name == stem {
			continue
		}
		out = append(out, navigationCrumb{Label: item.Name, Title: symbolTooltip(item), Symbol: &item})
	}
	return out
}

func symbolTooltip(item openidea.OutlineItem) string {
	if item.Detail != nil && *item.Detail != "" {
		return *item.Detail
	}
	return item.Name
}

// renderTopNavigationBar draws the explicit View › Appearance › Navigation
// Bar › Top placement between the title bar and the editor area.
func (m *Model) renderTopNavigationBar(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.navigationBarPosition() != "top" || m.tab() == nil {
		return
	}
	ui.Row(c).Height(oiNavigationBarHeight).Shrink(0).Padding(0, 10).
		Gap(4).AlignItems(ui.Center).Children(func() {
		m.renderNavigationBar(c, p, oi)
	})
}

// renderNavigationBar is OpenIDEA's new UI navigation bar: project › dirs ›
// file › symbols. Directory segments list their children; symbol segments jump
// to their declaration.
func (m *Model) renderNavigationBar(c *ui.Context, p *ui.Theme, oi oiColors) {
	m.renderNavigationCrumbs(c, p, oi, m.navigationCrumbs(), false)
}

func (m *Model) renderNavigationCrumbs(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	crumbs []navigationCrumb,
	interactive bool,
) {
	if len(crumbs) == 0 {
		return
	}
	scroll := ui.ScrollHorizontal(c.Key("navigation:bar")).Grow(1).MinWidth(0)
	if interactive {
		scroll.TrackScroll(&m.navigation.scroll)
	}
	scroll.Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			for i, crumb := range crumbs {
				crumb := crumb
				active := interactive && i == m.navigation.index
				if i > 0 {
					oiIconEl(c, "general/chevronRight", 12, oi, oi.textSubtle)
				}
				b := ui.ButtonBase(c.Key("navigation:"+crumb.Title+":"+crumb.Label)).
					Height(oiNavigationBarHeight-6).Padding(0, 4).Radius(4).
					Gap(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if active {
					b.Background(oi.selected)
				} else if b.Hovered() {
					b.Background(oi.toolbarHover)
				}
				b.Tooltip(crumb.Title)
				b.Children(func() {
					iconColor, textColor := oi.textMuted, oi.textMuted
					if active {
						textColor = oi.textStrong
					}
					switch {
					case crumb.Dir:
						oiIconEl(c, "nodes/folder", 14, oi, iconColor)
					case crumb.Symbol == nil:
						fileIconEl(c, oi, crumb.Label, 14, iconColor)
					default:
						asset := symbolIcon(strings.ToLower(crumb.Symbol.Kind))
						if asset == "" {
							asset = "nodes/method"
						}
						oiIconEl(c, asset, 14, oi, iconColor)
					}
					ui.Text(c, crumb.Label).FontSize(12).TextColor(textColor).SingleLine()
				})
				if interactive {
					if b.Clicked() {
						m.navigation.index = i
						m.navigation.segmentReveal = true
						m.openNavigationChildren(i)
					}
					if active && m.navigation.segmentReveal {
						b.ScrollIntoView()
					}
				} else {
					switch {
					case crumb.Dir:
						b.Menu(func(menu *ui.Menu) { m.navigationChildrenMenu(menu, crumb.Path) })
					case crumb.Symbol != nil:
						b.Menu(func(menu *ui.Menu) {
							t := m.tab()
							if t == nil {
								return
							}
							for _, item := range flattenOutlineItems(t.outline) {
								item := item
								if menu.Item(item.Name).Chosen() {
									m.goToOutlineItem(t, item)
								}
							}
						})
					default:
						if b.Clicked() {
							m.FocusEditor()
						}
					}
				}
			}
		})
	})
	if interactive {
		m.navigation.segmentReveal = false
	}
}

// navigationChildrenMenu lists a directory's immediate children, directories
// first, and nests folders so the popup remains the project-wide navigator.
func (m *Model) navigationChildrenMenu(menu *ui.Menu, path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		menu.Item("Cannot read directory").Disabled(true)
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	shown := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if shown == 200 {
			menu.Item("…").Disabled(true)
			break
		}
		shown++
		child := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			menu.Submenu(entry.Name(), func(sub *ui.Menu) {
				m.navigationChildrenMenu(sub, child)
			})
			continue
		}
		if menu.Item(entry.Name()).Chosen() {
			m.openFile(child)
		}
	}
	if shown == 0 {
		menu.Item("Empty directory").Disabled(true)
	}
}

func flattenOutlineItems(items []openidea.OutlineItem) []openidea.OutlineItem {
	var out []openidea.OutlineItem
	var walk func([]openidea.OutlineItem)
	walk = func(level []openidea.OutlineItem) {
		for _, item := range level {
			out = append(out, item)
			walk(item.Children)
		}
	}
	walk(items)
	return out
}

func (m *Model) goToOutlineItem(t *Tab, item openidea.OutlineItem) {
	line, col := t.doc.BytePos(
		int(item.SelectionRange.Start.Line), int(item.SelectionRange.Start.Col))
	t.state.GoTo(editor.Pos{Line: line, Col: col})
	m.focusApply = true
	m.invalidate()
}
