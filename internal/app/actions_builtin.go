package app

import "strings"

// The shipped command list registers itself into the action registry, so the
// built-ins go through the same lifecycle as a feature's or plugin's actions.

var builtinShortcuts = map[string]string{}
var builtinCategories = map[string]string{}

func init() {
	for _, c := range builtinCommands() {
		builtinShortcuts[c.id] = c.shortcut
		builtinCategories[c.id] = c.group
		// A malformed built-in id would be a programming error; the tests
		// check the whole list instead of failing the app at start.
		a := &Action{
			ID:       c.id,
			Title:    c.title,
			Category: c.group,
			Keywords: c.keywords,
			Run:      c.run,
			Builtin:  true,
		}
		switch c.id {
		case "file.openPaths":
			a.RunWithArg = func(m *Model, arg any) { m.openPaths(stringPaths(arg)) }
		case "file.openProjectPath":
			a.RunWithArg = func(m *Model, arg any) { m.openProjectPath(projectPathArg(arg)) }
		case "file.openRecentProject":
			a.RunWithArg = func(m *Model, arg any) { m.openProjectPath(projectPathArg(arg)) }
		case "build.compileFile":
			a.When = func(m *Model) bool {
				t := m.tab()
				return t != nil && t.kind == "" && t.doc != nil && m.canCompileFile(t.doc.Path())
			}
		case "edit.undo":
			a.When = func(m *Model) bool {
				return m.projectTreeFocus || m.activeEditorAvailable()
			}
			a.Enabled = func(m *Model) bool {
				if m.projectTreeFocus {
					return m.treeLastMove != nil
				}
				return m.activeEditorCanUndo()
			}
		case "edit.redo":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() }
			a.Enabled = func(m *Model) bool { return m.activeEditorCanRedo() }
		case "edit.copy", "edit.selectAll":
			a.When = func(m *Model) bool {
				return (m.projectTreeFocus && m.tree != nil) || m.activeEditorAvailable()
			}
		case "edit.cut", "edit.paste":
			a.When = func(m *Model) bool {
				return (m.projectTreeFocus && m.tree != nil) || m.activeEditorEditable()
			}
		case "editor.addOrRemoveCaret", "editor.copyPath", "editor.copyReference":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() }
		case "editor.nextTab", "editor.prevTab":
			a.When = func(m *Model) bool { return len(m.tabs) > 1 }
		case "editor.toggleSoftWrap":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() }
			a.Toggled = func(m *Model) bool { return m.activeEditorSoftWrapped() }
		case "editor.toggleRenderedDocs":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() }
			a.Toggled = func(m *Model) bool { return m.activeEditorRenderedDocs() }
		case "view.toggleToolbar":
			a.Toggled = func(m *Model) bool { return !m.prefs.Appearance.HideMainToolbar }
		case "view.toggleCompactMode":
			a.Toggled = func(m *Model) bool { return m.prefs.Appearance.CompactMode }
		case "view.toggleToolWindowBars":
			a.Toggled = func(m *Model) bool { return !m.prefs.Appearance.HideToolWindowBars }
		case "view.toggleStatusBar":
			a.Toggled = func(m *Model) bool { return !m.prefs.Appearance.HideStatusBar }
		case "view.navigationBarTop":
			a.Toggled = func(m *Model) bool { return m.navigationBarPosition() == "top" }
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "view.navigationBarStatusBar":
			a.Toggled = func(m *Model) bool { return m.navigationBarPosition() == "statusBar" }
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "view.navigationBarHidden":
			a.Toggled = func(m *Model) bool { return m.navigationBarPosition() == "hidden" }
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "view.toggleStatusBarWidget":
			a.RunWithArg = func(m *Model, arg any) {
				id, _ := arg.(string)
				m.toggleStatusBarWidget(id)
			}
		case "toolwindow.closeContent":
			a.When = func(m *Model) bool { return m.activeToolWindow() != "" }
		case "editor.resetFontSize", "editor.increaseFontSize", "editor.decreaseFontSize":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() }
		case "file.toggleReadOnly":
			a.When = func(m *Model) bool { return m.activeFileCanToggleReadOnly() }
		case "find.inFile", "find.replaceInFile", "find.next", "find.previous":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() }
			a.KeyWhen = func(m *Model) bool { return m.activeEditorAvailable() }
		case "navigate.back":
			a.Enabled = func(m *Model) bool { return m.recentNav.history.canGoBack() }
		case "navigate.forward":
			a.Enabled = func(m *Model) bool { return m.recentNav.history.canGoForward() }
		case "navigate.lastEditLocation":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
			a.Enabled = func(m *Model) bool { return len(m.recentNav.edits.entries) > 0 }
		case "navigate.nextEditLocation":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
			a.Enabled = func(m *Model) bool {
				return m.recentNav.edits.index < len(m.recentNav.edits.entries)-1
			}
		case "navigate.recentFiles", "navigate.recentLocations", "analyze.stackTrace",
			"navigate.gotoClass", "navigate.gotoFile", "navigate.gotoSymbol", "navigate.findAction",
			"code.quickDefinition", "code.quickTypeDefinition":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "navigate.navigationBar":
			a.When = func(m *Model) bool {
				return m.Root() != "" && !m.showingWelcome && m.tab() != nil
			}
		case "layout.save", "layout.restore", "layout.restoreDefault":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "layout.rename", "layout.delete":
			a.When = func(m *Model) bool {
				return m.Root() != "" && !m.showingWelcome && len(m.namedLayouts) > 0
			}
		}
		if c.id == "layout.restore" {
			a.RunWithArg = func(m *Model, arg any) {
				m.OpenRestoreToolWindowLayout(arg)
			}
		}
		if key, ok := strings.CutPrefix(c.id, "project.option."); ok {
			a.Toggled = func(m *Model) bool { return m.projectTreeOption(key) }
		}
		if sortBy, ok := strings.CutPrefix(c.id, "project.sortBy."); ok {
			a.Toggled = func(m *Model) bool {
				return m.prefs.ProjectTree.normalizedSort() == sortBy
			}
		}
		if mode, ok := strings.CutPrefix(c.id, "project.viewMode."); ok {
			a.Toggled = func(m *Model) bool {
				return m.prefs.ProjectTree.normalizedMode() == mode
			}
			a.When = func(m *Model) bool {
				if m.Root() == "" || m.showingWelcome {
					return false
				}
				return mode != "unity" || m.projectUnityVersion != ""
			}
		}
		parityWindowActionState(a)
		parityToolActionState(a)
		parityRunActionState(a)
		coverageActionState(a)
		downloadJDKActionState(a)
		workspaceActionState(a)
		_ = registerAction(a)
	}
}
