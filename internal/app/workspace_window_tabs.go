package app

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// WorkspaceTitle is the project name shown in a native window's project tab.
func (m *Model) WorkspaceTitle() string {
	if m == nil {
		return ""
	}
	if label := m.projectLabel(); label != "" {
		return label
	}
	if t := m.tab(); t != nil && t.doc != nil {
		return t.doc.Path()
	}
	return "Welcome"
}

// WorkspaceDirty reports whether the window has unsaved editor text.
func (m *Model) WorkspaceDirty() bool {
	if m == nil {
		return false
	}
	if m.hasDirty() {
		return true
	}
	for _, t := range m.groupTabsAt(groupSplit) {
		if t.unsaved() {
			return true
		}
	}
	return false
}

// WorkspaceBranch is the branch shown for this project in the title-bar
// project switcher.
func (m *Model) WorkspaceBranch() string {
	if m == nil {
		return ""
	}
	return m.branchLabel()
}

// RefreshWorkspaceWindows redraws the project-tab strip after group order or
// selection changes in another native window.
func (m *Model) RefreshWorkspaceWindows() {
	if m != nil {
		m.invalidate()
	}
}

// workspaceWindowTabs draws OpenIDEA's project tabs for a native window group.
// MyGo disables AppKit window tabbing, so the selected project is the visible
// native window and selecting another swaps visibility while services remain
// alive in both models.
func (m *Model) workspaceWindowTabs(c *ui.Context, p *ui.Theme) {
	windows := m.workspaceWindows()
	if len(windows) < 2 {
		return
	}
	oi := oiPaletteOf(p)
	ui.Row(c.Key("workspace-window-tabs")).Role(ui.RoleTabList).Label("Project windows").Height(32).Shrink(0).
		Padding(0, 8).Gap(2).AlignItems(ui.Center).
		Background(oi.frame).Children(func() {
		for i, window := range windows {
			tab := ui.Row(c.Key("workspace-window:"+window.ID)).Height(27).
				Padding(0, 11).Gap(4).Radius(4).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Role(ui.RoleTab).Label(window.Title).Selected(window.Selected).Focusable()
			switch {
			case window.Selected:
				tab.Background(oi.tabSelected)
			case tab.Hovered():
				tab.Background(oi.toolbarHover)
			}
			tab.Tooltip(window.Title).HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown || ev.Mods != 0 {
					return false
				}
				var to int
				switch ev.Key {
				case ui.KeyHome:
					to = 0
				case ui.KeyEnd:
					to = len(windows) - 1
				case ui.KeyLeft:
					to = (i + len(windows) - 1) % len(windows)
				case ui.KeyRight:
					to = (i + 1) % len(windows)
				case ui.KeyEnter, ui.KeySpace:
					to = i
				default:
					return false
				}
				m.SelectWindowTab(windows[to].ID)
				return true
			})
			tab.Drag(projectWindowDrag(window.ID))
			dropped, moved := ui.Drop[projectWindowDrag](tab)
			if moved {
				if ids := projectWindowTabOrder(windows, string(dropped), i); ids != nil {
					m.RunActionWithArg("window.reorderTabs", ids)
				}
			}
			tab.ContextMenu(func(menu *ui.Menu) {
				if menu.Item("Move Tab to New Window").Disabled(!window.Selected).Chosen() {
					m.RunAction("window.detachTab")
				}
				menu.Separator()
				for _, action := range []struct {
					label, id string
					disabled  bool
				}{
					{"Close Tab", "window.closeTab", false},
					{"Close Other Tabs", "window.closeOtherTabs", false},
					{"Close Tabs to the Left", "window.closeTabsToLeft", i == 0},
					{"Close Tabs to the Right", "window.closeTabsToRight", i == len(windows)-1},
				} {
					if menu.Item(action.label).Disabled(action.disabled).Chosen() {
						m.RunActionWithArg(action.id, window.ID)
					}
				}
			})
			closed := false
			tab.Children(func() {
				title := window.Title
				text := oi.text
				if window.Selected {
					text = oi.textStrong
				}
				ui.Text(c, title).FontSize(12).TextColor(text).
					SingleLine().Ellipsis("…")
				close := ui.ButtonBase(c.Key("close-project:"+window.ID)).Size(19, 19).
					AlignItems(ui.Center).Justify(ui.Center).Radius(3).Label("Close " + title).Tooltip("Close " + title)
				if close.Hovered() {
					close.Background(oi.toolbarHover)
				}
				close.Children(func() {
					if window.Dirty && !close.Hovered() {
						ui.Text(c, "●").FontSize(9).TextColor(oi.accent)
					} else {
						oiIconEl(c, "general/close", 12, oi, oi.icon)
					}
				})
				if close.Clicked() {
					closed = true
					m.RunActionWithArg("window.closeTab", window.ID)
				}
			})
			if !closed && !moved && tab.Clicked() && !window.Selected {
				m.focusWorkspaceWindow(window.ID)
			}
		}
		ui.Spacer(c)
		detach := ui.ButtonBase(c.Key("detach-project-tab")).Size(24, 24).Label("Move Tab to New Window").
			Tooltip("Move Tab to New Window").AlignItems(ui.Center).Justify(ui.Center)
		if detach.Hovered() {
			detach.Background(oi.toolbarHover)
		}
		detach.Children(func() { oiIconEl(c, "general/externalLink", 14, oi, oi.icon) })
		if detach.Clicked() {
			m.RunAction("window.detachTab")
		}
	})
}

type projectWindowDrag string

func projectWindowTabOrder(windows []WorkspaceWindow, source string, to int) []string {
	ids := make([]string, len(windows))
	for i, w := range windows {
		ids[i] = w.ID
	}
	from := slices.Index(ids, source)
	if from < 0 || to < 0 || to >= len(ids) || from == to {
		return nil
	}
	ids = slices.Delete(ids, from, from+1)
	return slices.Insert(ids, to, source)
}
