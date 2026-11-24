package app

import "path/filepath"

// parityWindowCommands adds OpenIDEA's project-window tab actions to the
// native window groups. A GoIDEA window group is the equivalent surface for an
// OpenIDEA grouped project window: selecting, closing, cycling and reordering
// all happen over that group's strip.
func parityWindowCommands() []appCommand {
	return []appCommand{
		{"window.detachTab", "Move Tab to New Window", "Window", "", "window project tab detach", func(m *Model) { m.detachProjectTab(nil) }},
		{"window.mergeAllWindows", "Merge All Project Windows", "Window", "", "window project tabs merge", func(m *Model) { m.MergeAllWindowGroups() }},
		{"window.closeTab", "Close Project Tab", "Window", "", "window project tab close", func(m *Model) { m.CloseWindowTab(nil) }},
		{"window.closeOtherTabs", "Close Other Project Tabs", "Window", "", "window project tabs close others", func(m *Model) { m.closeProjectTabs("others", nil) }},
		{"window.closeTabsToLeft", "Close Project Tabs to the Left", "Window", "", "window project tabs close left", func(m *Model) { m.closeProjectTabs("left", nil) }},
		{"window.closeTabsToRight", "Close Project Tabs to the Right", "Window", "", "window project tabs close right", func(m *Model) { m.closeProjectTabs("right", nil) }},
		{"window.nextTab", "Next Project Tab", "Window", "Ctrl+Alt+PageDown", "window project next tab cycle", func(m *Model) { m.cycleProjectTab(1) }},
		{"window.previousTab", "Previous Project Tab", "Window", "Ctrl+Alt+PageUp", "window project previous tab cycle", func(m *Model) { m.cycleProjectTab(-1) }},
		{"window.reorderTabs", "Reorder Project Tabs", "Window", "", "window project reorder tabs", func(m *Model) { m.ReorderWindowTabs(nil) }},
		{"window.selectTab", "Select Project Tab", "Window", "", "window project select tab", func(m *Model) { m.SelectWindowTab("") }},
		{"lightEdit.openProject", "Open Project in Current Directory", "File", "", "light edit file folder project open", func(m *Model) { m.OpenLightEditProject() }},
		{"toolwindow.focusEditor", "Focus Editor", "Window", "Escape", "tool window editor focus", func(m *Model) { m.FocusEditor() }},
		{"usages.rerun", "Rerun Find Usages", "Find", "Cmd+R", "find usages rerun search", func(m *Model) { m.RerunUsages() }},
	}
}

// CloseWindowTab asks the owning project window to run its close guards.
func (m *Model) CloseWindowTab(arg any) { m.closeProjectTabs("one", arg) }

func (m *Model) closeProjectTabs(side string, arg any) {
	host, ok := m.workspaceHost.(projectTabHost)
	if !ok {
		return
	}
	windows := m.workspaceWindows()
	id, _ := arg.(string)
	for i, w := range windows {
		if w.ID != id && (arg != nil || !w.Selected) {
			continue
		}
		keep := w.ID
		var ids []string
		for j, target := range windows {
			switch side {
			case "one":
				if j != i {
					continue
				}
			case "others":
				if j == i {
					continue
				}
			case "left":
				if j >= i {
					continue
				}
			case "right":
				if j <= i {
					continue
				}
			default:
				return
			}
			ids = append(ids, target.ID)
		}
		if side == "one" {
			keep = ""
		}
		host.CloseWorkspaceWindows(m, keep, ids)
		return
	}
}

func (m *Model) SelectWindowTab(id string) {
	for _, w := range m.workspaceWindows() {
		if w.ID == id {
			m.focusWorkspaceWindow(id)
			return
		}
	}
}

func (m *Model) cycleProjectTab(delta int) {
	windows := m.workspaceWindows()
	for i, w := range windows {
		if w.Selected {
			m.focusWorkspaceWindow(windows[(i+delta+len(windows))%len(windows)].ID)
			return
		}
	}
}

func (m *Model) ReorderWindowTabs(arg any) {
	host, ok := m.workspaceHost.(projectTabHost)
	if !ok {
		return
	}
	var ids []string
	switch values := arg.(type) {
	case []string:
		ids = values
	case []any:
		for _, v := range values {
			id, ok := v.(string)
			if !ok {
				return
			}
			ids = append(ids, id)
		}
	default:
		return
	}
	host.ReorderWorkspaceWindows(m, ids)
}

func (m *Model) detachProjectTab(arg any) {
	if m.workspaceHost == nil {
		m.DetachWindowTab(arg)
	} else if host, ok := m.workspaceHost.(projectTabHost); ok {
		host.DetachWorkspaceWindow(m)
	}
}

func (m *Model) tabByPathInAnyGroup(path string) (*Tab, bool) {
	path = realPath(path)
	for _, t := range m.allTabs() {
		if tabPath(t) != "" && realPath(tabPath(t)) == path {
			return t, true
		}
	}
	return nil, false
}

// OpenLightEditProject opens the current standalone file's directory as the
// project and keeps the file open there.
func (m *Model) OpenLightEditProject() {
	if m.Root() != "" {
		return
	}
	t := m.tab()
	if t == nil || t.doc == nil || t.doc.Path() == "" {
		m.status = "Open a file before turning LightEdit into a project"
		m.invalidate()
		return
	}
	path := realPath(t.doc.Path())
	m.confirmSwitchWithFiles(filepath.Dir(path), []string{path})
}

// FocusEditor gives the active editor the keyboard after keyboard navigation
// through a tool window or the menu.
func (m *Model) FocusEditor() {
	if m.tab() == nil {
		return
	}
	m.projectTreeFocus = false
	m.focusApply = true
	m.hideUnpinnedWindows("")
	m.invalidate()
}

// Re-run and focus commands remain discoverable in OpenIDEA-like contexts,
// but are unavailable where they would have no effect.
func parityWindowActionState(a *Action) {
	switch a.ID {
	case "window.closeTab":
		a.RunWithArg = func(m *Model, arg any) { m.CloseWindowTab(arg) }
	case "window.closeOtherTabs", "window.closeTabsToLeft", "window.closeTabsToRight":
		side := map[string]string{"window.closeOtherTabs": "others", "window.closeTabsToLeft": "left", "window.closeTabsToRight": "right"}[a.ID]
		a.RunWithArg = func(m *Model, arg any) { m.closeProjectTabs(side, arg) }
	case "window.detachTab":
		a.RunWithArg = (*Model).detachProjectTab
	case "window.selectTab":
		a.RunWithArg = func(m *Model, arg any) {
			id, _ := arg.(string)
			m.SelectWindowTab(id)
		}
	case "window.reorderTabs":
		a.RunWithArg = func(m *Model, arg any) { m.ReorderWindowTabs(arg) }
	}
	switch a.ID {
	case "window.closeTab", "window.closeOtherTabs", "window.closeTabsToLeft",
		"window.closeTabsToRight", "window.nextTab", "window.previousTab",
		"window.reorderTabs", "window.selectTab":
		a.When = func(m *Model) bool { return len(m.workspaceWindows()) > 1 }
	case "window.detachTab":
		a.When = func(m *Model) bool { return len(m.workspaceWindows()) > 1 || m.workspaceHost == nil && m.tab() != nil }
	case "window.mergeAllWindows":
		a.When = func(m *Model) bool {
			return m.workspaceHost != nil && m.Root() != "" || m.workspaceHost == nil && m.tab() != nil
		}
	case "lightEdit.openProject":
		a.When = func(m *Model) bool {
			t := m.tab()
			return m.Root() == "" && t != nil && t.doc != nil
		}
	case "toolwindow.focusEditor":
		a.When = func(m *Model) bool { return m.tab() != nil }
	case "usages.rerun":
		a.When = func(m *Model) bool {
			return m.toolVisible("usages") && m.usages.path != ""
		}
		a.Enabled = func(m *Model) bool { return !m.usages.running }
	}
}
