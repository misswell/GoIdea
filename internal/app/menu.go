package app

import "github.com/egoist/mygo"

// menuEntry is one item of GoIDEA's main menu, following OpenIDEA's menu.rs:
// the label is its English wording, and the command the id of the registry
// action the menu, the palette and the shortcuts all run. A role is a
// platform item such as Undo, and sub holds a submenu.
type menuEntry struct {
	label   string
	command string
	role    mygo.MenuRole
	accel   string
	sep     bool
	check   func(*Model) bool
	sub     []menuEntry
}

// action is a menu item that runs a registry command.
func action(label, command, accel string) menuEntry {
	return menuEntry{label: label, command: command, accel: accel}
}

// actionChecked is a menu item that runs a registry command and shows a
// check mark while on reports true.
func actionChecked(label, command string, on func(*Model) bool) menuEntry {
	return menuEntry{label: label, command: command, check: on}
}

// submenu groups items under a titled item.
func submenu(label string, items ...menuEntry) menuEntry {
	return menuEntry{label: label, sub: items}
}

// platform is an item whose behavior, label and shortcut come from its role.
func platform(role mygo.MenuRole) menuEntry {
	return menuEntry{role: role}
}

// rule is a separator between groups of items.
func rule() menuEntry { return menuEntry{sep: true} }

// mainMenuGroups returns the menu bar of OpenIDEA's menu.rs, kept to the
// features GoIDEA carries: the same top-level order and wording, with the
// items the registry has an action for.
func mainMenuGroups() []menuEntry {
	bookmarks := func() menuEntry {
		return submenu("Bookmarks",
			action("Toggle Bookmark", "bookmarks.toggle", "F3"),
			action("Toggle Bookmark with Mnemonic", "bookmarks.mnemonic", "Alt+F3"),
			rule(),
			action("Show All Bookmarks…", "bookmarks.show", "CmdOrCtrl+F3"),
			action("Next Bookmark", "bookmarks.next", ""),
			action("Previous Bookmark", "bookmarks.prev", ""),
			rule(),
			action("Bookmarks Tool Window", "bookmarks.window", "CmdOrCtrl+2"),
			action("Toggle File Bookmark", "bookmarks.file", ""),
			action("New Bookmark List…", "bookmarks.newList", ""),
		)
	}
	toolWindows := submenu("Tool Windows",
		action("Project", "view.project", "CmdOrCtrl+1"),
		action("Bookmarks", "bookmarks.window", "CmdOrCtrl+2"),
		action("Find", "view.find", "CmdOrCtrl+3"),
		action("Run", "view.run", "CmdOrCtrl+4"),
		action("Debug", "view.debug", "CmdOrCtrl+5"),
		action("Problems", "view.problems", "CmdOrCtrl+6"),
		action("Structure", "view.structure", "CmdOrCtrl+7"),
		action("Commit", "view.commit", "CmdOrCtrl+0"),
		action("Git", "vcs.show", "CmdOrCtrl+9"),
		action("Terminal", "view.terminal", "Alt+F12"),
		action("Database", "view.database", ""),
		rule(),
		action("Hide All Tool Windows", "view.hideAll", "CmdOrCtrl+Shift+F12"),
	)
	appearance := submenu("Appearance",
		actionChecked("Light", "app.themeLight", func(m *Model) bool { return m.prefs.Theme == "light" }),
		actionChecked("Dark", "app.themeDark", func(m *Model) bool { return m.prefs.Theme == "dark" }),
		actionChecked("Use System Settings", "app.themeSystem", func(m *Model) bool { return m.prefs.Theme == "" || m.prefs.Theme == "system" }),
		rule(),
		action("Toggle Light/Dark Theme", "view.toggleTheme", ""),
		action("Reset Layout", "view.resetLayout", ""),
	)
	return []menuEntry{
		submenu("File",
			action("New Project…", "project.new", ""),
			action("Open Project…", "project.open", "CmdOrCtrl+O"),
			action("Open File…", "file.quickOpen", "CmdOrCtrl+P"),
			rule(),
			action("Save", "file.save", "CmdOrCtrl+S"),
			action("Close", "file.close", "CmdOrCtrl+W"),
			rule(),
			action("Manage Projects…", "welcome.manageProjects", ""),
			rule(),
			action("Settings…", "app.settings", "CmdOrCtrl+,"),
		),
		submenu("Edit",
			platform(mygo.RoleUndo),
			platform(mygo.RoleRedo),
			rule(),
			platform(mygo.RoleCut),
			platform(mygo.RoleCopy),
			platform(mygo.RolePaste),
			platform(mygo.RoleSelectAll),
			rule(),
			submenu("Find",
				action("Find in Files…", "search.project", "CmdOrCtrl+Shift+F"),
				action("Find Usages", "code.usages", "Alt+F7"),
				action("Show Usages", "code.showUsages", ""),
			),
			rule(),
			bookmarks(),
		),
		submenu("View",
			toolWindows,
			appearance,
		),
		submenu("Navigate",
			action("Search Everywhere", "app.commandPalette", "CmdOrCtrl+Shift+A"),
			action("File…", "file.quickOpen", "CmdOrCtrl+P"),
			action("Symbol…", "code.symbols", "CmdOrCtrl+Shift+O"),
			rule(),
			action("Select In Project View", "navigate.selectInProjectView", "Alt+F1"),
			rule(),
			action("Find in Files…", "search.project", "CmdOrCtrl+Shift+F"),
			action("Find in Folder…", "project.findInFolder", ""),
			action("Find Usages", "code.usages", "Alt+F7"),
			rule(),
			action("Next Highlighted Error", "problems.next", "CmdOrCtrl+Alt+Down"),
			action("Previous Highlighted Error", "problems.prev", "CmdOrCtrl+Alt+Up"),
		),
		submenu("Code",
			action("Show Context Actions", "code.intentions", "Alt+Enter"),
			action("Go to Symbol…", "code.symbols", "CmdOrCtrl+Shift+O"),
			action("Find Usages", "code.usages", "Alt+F7"),
			action("Rename…", "code.rename", "Shift+F6"),
			action("Reformat Code", "code.format", "Alt+CmdOrCtrl+L"),
			rule(),
			bookmarks(),
			rule(),
			action("Undo Refactor", "code.undoRefactor", "CmdOrCtrl+Alt+Z"),
			action("Redo Refactor", "code.redoRefactor", "CmdOrCtrl+Alt+Shift+Z"),
		),
		submenu("Refactor",
			action("Rename…", "code.rename", "Shift+F6"),
			rule(),
			action("Undo Refactor", "code.undoRefactor", "CmdOrCtrl+Alt+Z"),
			action("Redo Refactor", "code.redoRefactor", "CmdOrCtrl+Alt+Shift+Z"),
		),
		submenu("Build",
			action("Build Project", "build.buildProject", "CmdOrCtrl+F9"),
			action("Build Module", "build.buildModule", "CmdOrCtrl+Shift+F9"),
			action("Rebuild Project", "build.rebuildProject", ""),
			action("Build Artifacts", "build.buildArtifacts", ""),
			rule(),
			action("Stop Build", "build.stop", ""),
			rule(),
			action("Show Build Window", "view.build", ""),
		),
		submenu("Run",
			action("Run", "run.start", "F5"),
			action("Debug", "run.debug", "F9"),
			action("Stop", "run.stop", "Shift+F5"),
			rule(),
			action("Select Run/Debug Configuration…", "run.chooseConfig", ""),
			action("Reload Run Configurations", "run.reloadConfigs", ""),
			rule(),
			action("Toggle Line Breakpoint", "run.breakpoint", "CmdOrCtrl+F8"),
			action("Step Over", "run.stepOver", "F8"),
			action("Step Into", "run.stepInto", "F7"),
			action("Step Out", "run.stepOut", "Shift+F8"),
			action("Resume Program", "run.continue", ""),
			action("Pause Program", "run.pause", ""),
			rule(),
			action("Next Run Configuration", "run.configNext", ""),
			action("Previous Run Configuration", "run.configPrev", ""),
			rule(),
			action("Trust Project", "run.trust", ""),
		),
		submenu("Git",
			action("Commit…", "vcs.commit", "CmdOrCtrl+K"),
			action("Show Git Changes", "vcs.show", "CmdOrCtrl+9"),
			action("Show Git Log", "vcs.log", ""),
			action("Compare with HEAD", "vcs.diff", "CmdOrCtrl+Shift+D"),
			rule(),
			action("Branches…", "vcs.branches", ""),
			action("Commit Window", "view.commit", "CmdOrCtrl+0"),
		),
		submenu("Tools",
			action("Database", "view.database", ""),
			action("Terminal", "view.terminal", "Alt+F12"),
		),
		submenu("Window",
			action("Maximize Tool Window", "view.maximize", "CmdOrCtrl+Shift+'"),
			action("Hide Active Tool Window", "view.hideActive", "Shift+Escape"),
			action("Jump to Last Tool Window", "view.jumpLast", "F12"),
			rule(),
			action("Stretch to Top", "view.resizeUp", "Ctrl+Alt+Up"),
			action("Stretch to Bottom", "view.resizeDown", "Ctrl+Alt+Down"),
			action("Stretch to Left", "view.resizeLeft", "Ctrl+Alt+Left"),
			action("Stretch to Right", "view.resizeRight", "Ctrl+Alt+Right"),
			rule(),
			action("Move to Bottom Right", "view.moveSplit", ""),
			action("Move to Bottom", "view.movePrimary", ""),
			rule(),
			platform(mygo.RoleWindow),
		),
	}
}

// gearGroup is the gear menu of OpenIDEA's TitleBar.tsx: settings, plugins,
// the theme and keymap quick switches, and the palette.
func gearGroup() []menuEntry {
	return []menuEntry{
		action("Settings…", "app.settings", "CmdOrCtrl+,"),
		action("Plugins…", "app.plugins", ""),
		rule(),
		submenu("Theme",
			actionChecked("Light", "app.themeLight", func(m *Model) bool { return m.prefs.Theme == "light" }),
			actionChecked("Dark", "app.themeDark", func(m *Model) bool { return m.prefs.Theme == "dark" }),
			actionChecked("Use System Settings", "app.themeSystem", func(m *Model) bool { return m.prefs.Theme == "" || m.prefs.Theme == "system" }),
		),
		rule(),
		action("Search Everywhere…", "app.commandPalette", "CmdOrCtrl+Shift+A"),
	}
}

// MainMenu is the application's menu bar, the one OpenIDEA builds from its
// menu model, with every item running a registry action.
func (m *Model) MainMenu() *mygo.Menu {
	return mygo.NewMenu(append([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}}, menuItems(m, mainMenuGroups())...))
}

// menuItems turns a group of entries into platform menu items.
func menuItems(m *Model, groups []menuEntry) []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(groups))
	for _, g := range groups {
		if g.sep {
			items = append(items, mygo.Separator())
			continue
		}
		it := &mygo.MenuItem{Label: g.label, Role: g.role, Accelerator: g.accel}
		if len(g.sub) > 0 {
			it.Submenu = menuItems(m, g.sub)
		}
		if g.command != "" {
			it.ID = "menu." + g.command
			id := g.command
			it.Click = func(*mygo.MenuItem, *mygo.Window) { m.RunCommand(id) }
			if g.check != nil && g.check(m) {
				it.Checked = true
			}
		}
		items = append(items, it)
	}
	return items
}
