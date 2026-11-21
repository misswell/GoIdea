package app

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// parityWindowCommands adds OpenIDEA's project-window tab actions to the
// native window groups. A GoIDEA window group is the equivalent surface for an
// OpenIDEA grouped project window: selecting, closing, cycling and reordering
// all happen over that group's strip.
func parityWindowCommands() []appCommand {
	return []appCommand{
		{"window.closeTab", "Close Project Tab", "Window", "", "window project tab close", func(m *Model) { m.CloseWindowTab(nil) }},
		{"window.closeOtherTabs", "Close Other Project Tabs", "Window", "", "window project tabs close others", func(m *Model) { m.CloseOtherTabs(nil) }},
		{"window.closeTabsToLeft", "Close Project Tabs to the Left", "Window", "", "window project tabs close left", func(m *Model) { m.CloseTabsToLeft(m.tab()) }},
		{"window.closeTabsToRight", "Close Project Tabs to the Right", "Window", "", "window project tabs close right", func(m *Model) { m.CloseTabsToRight(m.tab()) }},
		{"window.nextTab", "Next Project Tab", "Window", "Ctrl+Alt+PageDown", "window project next tab cycle", func(m *Model) { m.cycleEditorTab(1) }},
		{"window.previousTab", "Previous Project Tab", "Window", "Ctrl+Alt+PageUp", "window project previous tab cycle", func(m *Model) { m.cycleEditorTab(-1) }},
		{"window.reorderTabs", "Reorder Project Tabs", "Window", "", "window project reorder tabs", func(m *Model) { m.ReorderWindowTabs(nil) }},
		{"window.selectTab", "Select Project Tab", "Window", "", "window project select tab", func(m *Model) { m.SelectWindowTab("") }},
		{"lightEdit.openProject", "Open Project in Current Directory", "File", "", "light edit file folder project open", func(m *Model) { m.OpenLightEditProject() }},
		{"toolwindow.focusEditor", "Focus Editor", "Window", "Escape", "tool window editor focus", func(m *Model) { m.FocusEditor() }},
		{"usages.rerun", "Rerun Find Usages", "Find", "Cmd+R", "find usages rerun search", func(m *Model) { m.RerunUsages() }},
	}
}

// CloseWindowTab closes the tab named by arg, or the active tab when arg is
// absent. It is the target of OpenIDEA's hidden window.closeTab action.
func (m *Model) CloseWindowTab(arg any) {
	path, ok := arg.(string)
	if !ok || strings.TrimSpace(path) == "" {
		if t := m.tab(); t != nil {
			m.closeTabThen(t, nil)
		}
		return
	}
	if t, found := m.tabByPathInAnyGroup(path); found {
		m.closeTabThen(t, nil)
	}
}

// SelectWindowTab activates the tab named by path.
func (m *Model) SelectWindowTab(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if t, ok := m.tabByPathInAnyGroup(path); ok {
		m.activateTab(t)
	}
}

// ReorderWindowTabs applies an ordered list of paths to the focused group's
// strip. Paths that do not belong to the strip are ignored, and tabs omitted
// from the argument keep their relative order at the end.
func (m *Model) ReorderWindowTabs(arg any) {
	paths := tabPathArgList(arg)
	if len(paths) == 0 {
		return
	}
	order := reorderTabsByPath(m.tabs, m.active, paths)
	if order.active == m.active && sameTabOrder(order.tabs, m.tabs) {
		return
	}
	m.tabs, m.active = order.tabs, order.active
	m.saveSession()
	m.invalidate()
}

// reorderTabsByPath returns the focused strip in the requested path order and
// where its active tab moved.
func reorderTabsByPath(tabs []*Tab, active int, paths []string) (out struct {
	tabs   []*Tab
	active int
}) {
	byPath := make(map[string][]*Tab)
	for _, t := range tabs {
		path := tabPath(t)
		if path != "" {
			path = realPath(path)
			byPath[path] = append(byPath[path], t)
		}
	}
	used := make(map[*Tab]bool, len(tabs))
	ordered := make([]*Tab, 0, len(tabs))
	for _, path := range paths {
		path = realPath(strings.TrimSpace(path))
		for len(byPath[path]) > 0 {
			t := byPath[path][0]
			byPath[path] = byPath[path][1:]
			if used[t] {
				continue
			}
			used[t] = true
			ordered = append(ordered, t)
		}
	}
	for _, t := range tabs {
		if !used[t] {
			used[t] = true
			ordered = append(ordered, t)
		}
	}
	out.tabs = ordered
	out.active = active
	if active >= 0 && active < len(tabs) {
		want := tabs[active]
		for i, t := range ordered {
			if t == want {
				out.active = i
				break
			}
		}
	}
	return out
}

func sameTabOrder(a, b []*Tab) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func tabPathArgList(arg any) []string {
	switch value := arg.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if path, ok := item.(string); ok && path != "" {
				out = append(out, path)
			}
		}
		return out
	}
	return nil
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
	case "window.selectTab":
		a.RunWithArg = func(m *Model, arg any) {
			path, _ := arg.(string)
			m.SelectWindowTab(path)
		}
	case "window.reorderTabs":
		a.RunWithArg = func(m *Model, arg any) { m.ReorderWindowTabs(arg) }
	}
	switch a.ID {
	case "window.closeTab", "window.closeOtherTabs", "window.closeTabsToLeft",
		"window.closeTabsToRight", "window.nextTab", "window.previousTab",
		"window.reorderTabs", "window.selectTab":
		a.When = func(m *Model) bool { return len(m.tabs) > 0 }
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

// Keep the UI dependency visible in this file: action context mirrors the
// same local menus and strips drawn with MyGo.
var _ = ui.Modifiers(0)
