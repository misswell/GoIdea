package app

import "github.com/egoist/mygo"

// menuEntry is one item of GoIDEA's main menu, following OpenIDEA's menu.rs:
// the label is its English wording, and the command the id of the registry
// action the menu, the palette and the shortcuts all run. A role is a
// platform item such as Undo, and sub holds a submenu.
type menuEntry struct {
	label   string
	command string
	arg     any
	role    mygo.MenuRole
	accel   string
	sep     bool
	check   func(*Model) bool
	dynamic func(*Model) []menuEntry
	// run is the entry's action when it is not a registry command, as the
	// toolbar checkboxes and the theme and keymap picks are.
	run func(*Model)
	sub []menuEntry
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
		action("TODO", "view.todo", ""),
		action("Commit", "view.commit", "CmdOrCtrl+0"),
		action("Notifications", "view.notifications", ""),
		action("Git", "vcs.show", "CmdOrCtrl+9"),
		action("Terminal", "view.terminal", "Alt+F12"),
		action("Database", "view.database", ""),
		action("Services", "view.services", ""),
		action("Agents", "agents.showToolWindow", ""),
		action("Remote Development", "remote.showToolWindow", ""),
		action("Documentation", "toolwindow.documentation", ""),
		action("Coverage", "toolwindow.coverage", ""),
		action("Move Tool Window to Left", "view.moveToolToLeft", ""),
		action("Move Tool Window to Left Bottom", "view.moveToolToLeftSplit", ""),
		action("Move Tool Window to Right", "view.moveToolToRight", ""),
		action("Move Tool Window to Right Bottom", "view.moveToolToRightSplit", ""),
		action("Move Tool Window to Bottom", "view.moveToolToBottom", ""),
		actionChecked("Pin Tool Window", "view.pinToolWindow", func(m *Model) bool { return m.dockPinned(m.tool) }),
		submenu("View Mode",
			actionChecked("Dock Pinned", "view.viewModePinned", func(m *Model) bool { return m.toolViewMode(m.tool) == "dockPinned" }),
			actionChecked("Dock Unpinned", "view.viewModeUnpinned", func(m *Model) bool { return m.toolViewMode(m.tool) == "dockUnpinned" }),
			action("Undock", "view.viewModeUndock", ""),
		),
		rule(),
		action("Hide All Tool Windows", "view.hideAll", "CmdOrCtrl+Shift+F12"),
	)
	appearance := submenu("Appearance",
		action("Enter Distraction Free Mode", "editor.toggleZen", ""),
		action("Toggle Full Screen", "window.toggleFullScreen", "CmdOrCtrl+Ctrl+F"),
		actionChecked("Compact Mode", "view.toggleCompactMode", func(m *Model) bool {
			return m.prefs.Appearance.CompactMode
		}),
		rule(),
		action("Zoom In IDE", "view.zoomIn", "Ctrl+Alt+="),
		action("Zoom Out IDE", "view.zoomOut", "Ctrl+Alt+-"),
		action("Reset IDE Zoom", "view.resetZoom", "Ctrl+Alt+0"),
		rule(),
		action("Toggle Light/Dark Theme", "view.toggleTheme", ""),
		action("Customize Main Toolbar…", "toolbar.customize", ""),
		rule(),
		actionChecked("Toolbar", "view.toggleToolbar", func(m *Model) bool {
			return !m.prefs.Appearance.HideMainToolbar
		}),
		actionChecked("Tool Window Bars", "view.toggleToolWindowBars", func(m *Model) bool {
			return !m.prefs.Appearance.HideToolWindowBars
		}),
		submenu("Navigation Bar",
			actionChecked("Top", "view.navigationBarTop", func(m *Model) bool {
				return m.navigationBarPosition() == "top"
			}),
			actionChecked("Status Bar", "view.navigationBarStatusBar", func(m *Model) bool {
				return m.navigationBarPosition() == "statusBar"
			}),
			actionChecked("Don't Show", "view.navigationBarHidden", func(m *Model) bool {
				return m.navigationBarPosition() == "hidden"
			}),
		),
		actionChecked("Status Bar", "view.toggleStatusBar", func(m *Model) bool {
			return !m.prefs.Appearance.HideStatusBar
		}),
		menuEntry{label: "Status Bar Widgets", dynamic: func(m *Model) []menuEntry {
			rows := m.statusWidgetRows()
			out := make([]menuEntry, 0, len(rows))
			for _, item := range rows {
				item := item
				out = append(out, menuEntry{
					label:   item.Title,
					command: "view.toggleStatusBarWidget",
					arg:     item.ID,
					check: func(m *Model) bool {
						return m.statusItemEnabled(item)
					},
				})
			}
			return out
		}},
	)
	return []menuEntry{
		submenu("File",
			action("New Project…", "file.newProject", ""),
			action("New Workspace…", "workspace.create", ""),
			action("Open Workspace…", "workspace.open", ""),
			action("Manage Workspace…", "workspace.manage", ""),
			action("New…", "file.new", ""),
			action("New Scratch File…", "file.newScratchFile", "CmdOrCtrl+Shift+N"),
			action("Open…", "file.openProject", "CmdOrCtrl+Shift+Alt+O"),
			action("Attach Project…", "project.attach", ""),
			action("Open File…", "file.openFile", ""),
			submenu("Remote Development",
				action("Remote Development…", "remote.connect", ""),
				action("Reload Environment", "remote.reloadEnvironment", ""),
				action("Reopen in Container", "remote.reopenInContainer", ""),
				rule(),
				action("Disconnect from Remote Host", "remote.disconnect", ""),
			),
			submenu("Open Recent",
				action("Recent Projects…", "welcome.recentProjects", ""),
				action("Manage Projects…", "welcome.manageProjects", ""),
			),
			rule(),
			action("Close Project", "project.close", ""),
			rule(),
			action("Save", "file.save", "CmdOrCtrl+S"),
			action("Close", "file.close", "CmdOrCtrl+W"),
			rule(),
			action("Reload All from Disk", "file.reloadAllFromDisk", ""),
			submenu("File Properties",
				action("Make File Read-only / Writable", "file.toggleReadOnly", ""),
			),
			rule(),
			action("Settings…", "file.settings", "CmdOrCtrl+,"),
			rule(),
			action("Invalidate Caches…", "file.invalidateCaches", ""),
			action("Restart IDE", "app.restart", ""),
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
			action("Copy Path", "editor.copyPath", ""),
			action("Copy Reference", "editor.copyReference", "CmdOrCtrl+Alt+Shift+C"),
			rule(),
			submenu("Find",
				action("Find…", "find.inFile", "CmdOrCtrl+F"),
				action("Replace…", "find.replaceInFile", "CmdOrCtrl+R"),
				rule(),
				action("Find in Files…", "find.inPath", "CmdOrCtrl+Shift+F"),
				action("Replace in Files…", "find.replaceInPath", "CmdOrCtrl+Shift+R"),
				action("Rerun Search", "find.rerun", "CmdOrCtrl+R"),
				action("Edit Scopes…", "search.editScopes", ""),
				action("Find Usages", "code.usages", "Alt+F7"),
				action("Show Usages", "code.showUsages", "Alt+Cmd+F7"),
			),
			rule(),
			bookmarks(),
		),
		submenu("View",
			toolWindows,
			appearance,
			submenu("Active Editor",
				actionChecked("Soft-Wrap", "editor.toggleSoftWrap", func(m *Model) bool {
					return m.activeEditorSoftWrapped()
				}),
				actionChecked("Render Documentation Comments", "editor.toggleRenderedDocs", func(m *Model) bool {
					return m.activeEditorRenderedDocs()
				}),
			),
			rule(),
			action("Quick Definition", "code.quickDefinition", "Alt+Space Cmd+Y"),
			action("Quick Type Definition", "code.quickTypeDefinition", ""),
			rule(),
			action("Recent Locations", "navigate.recentLocations", "CmdOrCtrl+Shift+E"),
			action("Recent Files", "navigate.recentFiles", "CmdOrCtrl+E"),
			action("Recent Changes", "localHistory.recentChanges", ""),
			rule(),
			action("Compare with Clipboard", "diff.compareWithClipboard", ""),
			rule(),
			action("Quick Switch Scheme…", "view.quickSwitchScheme", "Ctrl+`"),
			rule(),
			action("Reset Font Size in All Editors", "editor.resetFontSize", ""),
			rule(),
			action("Notifications", "view.notifications", ""),
		),
		submenu("Navigate",
			action("Search Everywhere", "navigate.searchEverywhere", "Shift Shift"),
			action("Find Action…", "navigate.findAction", "CmdOrCtrl+Shift+A"),
			action("Class…", "navigate.gotoClass", "CmdOrCtrl+O"),
			action("File…", "navigate.gotoFile", "CmdOrCtrl+Shift+O"),
			action("Symbol…", "navigate.gotoSymbol", "CmdOrCtrl+Alt+O"),
			action("Go to Line:Column…", "navigate.gotoLine", "CmdOrCtrl+L"),
			action("Jump to Navigation Bar", "navigate.navigationBar", "Cmd+Up"),
			action("Back", "navigate.back", "CmdOrCtrl+["),
			action("Forward", "navigate.forward", "CmdOrCtrl+]"),
			action("Last Edit Location", "navigate.lastEditLocation", "CmdOrCtrl+Shift+Backspace"),
			action("Next Edit Location", "navigate.nextEditLocation", ""),
			rule(),
			action("Recent Files", "navigate.recentFiles", "CmdOrCtrl+E"),
			action("Recent Locations", "navigate.recentLocations", "CmdOrCtrl+Shift+E"),
			action("Switcher", "navigate.switcher", "Ctrl+Tab"),
			action("File Structure", "navigate.fileStructure", "CmdOrCtrl+F12"),
			rule(),
			action("Call Hierarchy", "navigate.callHierarchy", "Ctrl+Alt+H"),
			action("Type Hierarchy", "navigate.typeHierarchy", "Ctrl+H"),
			action("Go to Implementation(s)", "navigate.implementation", "Cmd+Alt+B"),
			action("Go to Type Declaration", "navigate.typeDeclaration", "Cmd+Shift+B"),
			rule(),
			action("Select In Project View", "navigate.selectInProjectView", "Alt+F1"),
			rule(),
			action("Find in Files…", "find.inPath", "CmdOrCtrl+Shift+F"),
			action("Replace in Files…", "find.replaceInPath", "CmdOrCtrl+Shift+R"),
			action("Find in Folder…", "project.findInFolder", ""),
			action("Find Usages", "code.usages", "Alt+F7"),
			rule(),
			action("Next Highlighted Error", "problems.next", "CmdOrCtrl+Alt+Down"),
			action("Previous Highlighted Error", "problems.prev", "CmdOrCtrl+Alt+Up"),
		),
		submenu("Code",
			action("Quick Documentation", "code.quickDocumentation", "F1"),
			action("Analyze Stack Trace or Thread Dump…", "analyze.stackTrace", ""),
			action("Show Context Actions", "code.intentions", "Alt+Enter"),
			action("Go to Symbol…", "navigate.gotoSymbol", "CmdOrCtrl+Alt+O"),
			action("Find Usages", "code.usages", "Alt+F7"),
			action("Rename…", "code.rename", "Shift+F6"),
			action("Reformat Code", "code.format", "Alt+CmdOrCtrl+L"),
			rule(),
			action("Comment with Line Comment", "code.commentLine", ""),
			action("Comment with Block Comment", "code.commentBlock", ""),
			rule(),
			action("Duplicate Line or Selection", "code.duplicateLine", ""),
			action("Delete Line", "code.deleteLine", ""),
			action("Move Line Up", "code.moveLineUp", ""),
			action("Move Line Down", "code.moveLineDown", ""),
			action("Move Statement Up", "code.moveStatementUp", ""),
			action("Move Statement Down", "code.moveStatementDown", ""),
			action("Join Lines", "code.joinLines", ""),
			rule(),
			bookmarks(),
			rule(),
			action("Undo Refactor", "code.undoRefactor", "CmdOrCtrl+Alt+Z"),
			action("Redo Refactor", "code.redoRefactor", "CmdOrCtrl+Alt+Shift+Z"),
		),
		submenu("Refactor",
			action("Rename…", "code.rename", "Shift+F6"),
			action("Safe Delete…", "refactor.safeDelete", "Cmd+Delete"),
			rule(),
			action("Undo Refactor", "code.undoRefactor", "CmdOrCtrl+Alt+Z"),
			action("Redo Refactor", "code.redoRefactor", "CmdOrCtrl+Alt+Shift+Z"),
		),
		submenu("Build",
			action("Build Project", "build.buildProject", "CmdOrCtrl+F9"),
			action("Build Module", "build.buildModule", "CmdOrCtrl+Shift+F9"),
			action("Compile File", "build.compileFile", ""),
			action("Rebuild Project", "build.rebuildProject", ""),
			action("Build Artifacts", "build.buildArtifacts", ""),
			rule(),
			action("Stop Build", "build.stop", ""),
			rule(),
			action("Show Build Window", "view.build", ""),
		),
		submenu("Run",
			action("Run", "run.start", "F5"),
			action("Run Context Configuration", "run.runContext", "Ctrl+Shift+R"),
			action("Run Context Configuration with Coverage", "run.coverageContext", ""),
			action("Run with Coverage", "run.coverage", ""),
			action("Debug", "run.debug", "F9"),
			action("Debug Context Configuration", "run.debugContext", "Ctrl+Shift+D"),
			action("Rerun Debug", "debug.rerun", ""),
			action("Stop", "run.stop", "Shift+F5"),
			action("Stop Debug Session", "debug.stop", ""),
			action("Reload Changed Classes", "debug.reloadChangedClasses", ""),
			action("Cancel Hot Swap Build", "debug.cancelHotSwap", ""),
			submenu("Coverage",
				action("Import a Coverage Report…", "coverage.importReport", ""),
				action("Manage Coverage Reports…", "coverage.manageReports", ""),
				action("Show Coverage", "coverage.show", ""),
				action("Hide Coverage", "coverage.hide", ""),
			),
			rule(),
			action("Select Run/Debug Configuration…", "run.chooseConfig", ""),
			action("Reload Run Configurations", "run.reloadConfigs", ""),
			rule(),
			action("Toggle Line Breakpoint", "run.breakpoint", "CmdOrCtrl+F8"),
			action("Step Over", "run.stepOver", "F8"),
			action("Step Into", "run.stepInto", "F7"),
			action("Step Out", "run.stepOut", "Shift+F8"),
			action("Smart Step Into", "debug.smartStepInto", ""),
			action("Force Step Over", "debug.forceStepOver", ""),
			action("Force Step Into", "debug.forceStepInto", ""),
			action("Run to Cursor", "run.runToCursor", "Alt+F9"),
			action("Force Run to Cursor", "run.forceRunToCursor", "Cmd+Alt+F9"),
			action("Resume Program", "run.continue", ""),
			action("Evaluate Expression…", "debug.evaluate", ""),
			action("Quick Evaluate Expression", "debug.quickEvaluate", ""),
			action("Set Value…", "debug.setValue", ""),
			action("Show Execution Point", "debug.showExecutionPoint", ""),
			action("Jump to Source", "debug.jumpToSource", ""),
			action("Reset Frame", "debug.resetFrame", ""),
			action("Browse Console History", "debug.browseConsoleHistory", ""),
			action("View Breakpoints…", "debug.viewBreakpoints", ""),
			action("Toggle Temporary Breakpoint", "debug.toggleTemporaryBreakpoint", ""),
			action("Toggle Method Breakpoint", "debug.toggleMethodBreakpoint", ""),
			action("Toggle Field Watchpoint", "debug.toggleFieldWatchpoint", ""),
			action("Capture Heap Snapshot", "debug.heap.capture", ""),
			action("Open Heap Dump…", "debug.heap.open", ""),
			action("Save Heap Dump As…", "debug.heap.saveDump", ""),
			action("Mute Breakpoints", "debug.muteBreakpoints", ""),
			action("Remove All Breakpoints", "debug.removeAllBreakpoints", ""),
			action("Pause Program", "run.pause", ""),
			rule(),
			action("Next Run Configuration", "run.configNext", ""),
			action("Previous Run Configuration", "run.configPrev", ""),
			rule(),
			action("Trust Project", "project.trust", ""),
		),
		submenu("Git",
			action("VCS Operations…", "vcs.operations", "Ctrl+V"),
			action("Commit…", "vcs.commit", "CmdOrCtrl+K"),
			action("Update Project…", "vcs.update", ""),
			action("Push…", "vcs.push", ""),
			action("Fetch", "vcs.fetch", ""),
			action("Pull…", "vcs.pull", ""),
			action("Manage Remotes…", "vcs.manageRemotes", ""),
			rule(),
			action("Merge…", "vcs.merge", ""),
			action("Rebase…", "vcs.rebase", ""),
			action("New Branch…", "vcs.newBranch", ""),
			action("New Tag…", "vcs.newTag", ""),
			rule(),
			action("Stash Changes…", "vcs.stash", ""),
			action("Unstash Changes…", "vcs.unstash", ""),
			action("Reset HEAD…", "vcs.resetHead", ""),
			rule(),
			submenu("Resolve in Progress",
				action("Continue", "vcs.rebase.continue", ""),
				action("Skip Commit", "vcs.rebase.skip", ""),
				action("Abort Operation", "vcs.rebase.abort", ""),
			),
			rule(),
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
			rule(),
			action("Install 'goidea' Command", "app.installCli", ""),
			action("Uninstall 'goidea' Command", "app.uninstallCli", ""),
			rule(),
			action("New Agent Task…", "agents.newTask", ""),
			action("Agents", "agents.showToolWindow", ""),
			action("Agent Sessions…", "agents.externalSessions", ""),
			action("Find in Conversation", "agents.findInConversation", "CmdOrCtrl+F"),
			action("Close Agent Task", "agents.closeTask", ""),
			action("Close and New Agent Task", "agents.closeAndNewTask", ""),
			rule(),
			action("Refresh TODO Items", "todo.refresh", ""),
			action("Show TODO Items in Current File", "todo.showCurrentFile", ""),
		),
		submenu("Window",
			submenu("Tool Window Layouts",
				action("Save Current Layout as…", "layout.save", ""),
				action("Restore Default Layout", "layout.restoreDefault", "Shift+F12"),
				rule(),
				menuEntry{dynamic: toolWindowLayoutMenuItems},
				rule(),
				action("Rename Layout…", "layout.rename", ""),
				action("Delete Layout…", "layout.delete", ""),
			),
			rule(),
			action("Maximize Tool Window", "view.maximize", "CmdOrCtrl+Shift+'"),
			action("Hide Active Tool Window", "view.hideActive", "Shift+Escape"),
			action("Jump to Last Tool Window", "view.jumpLast", "F12"),
			rule(),
			submenu("Editor Tabs",
				action("Split Right", "editor.splitRight", "CmdOrCtrl+\\"),
				action("Split Down", "editor.splitDown", ""),
				rule(),
				action("Split and Move Right", "editor.splitAndMoveRight", ""),
				action("Split and Move Down", "editor.splitAndMoveDown", ""),
				action("Move to Opposite Group", "editor.moveToOppositeGroup", ""),
				action("Open in Opposite Group", "editor.openInOppositeGroup", ""),
				action("Change Splitter Orientation", "editor.changeSplitOrientation", ""),
				action("Unsplit", "editor.unsplit", ""),
				action("Unsplit All", "editor.unsplitAll", ""),
				rule(),
				action("Go to Next Splitter", "editor.nextSplitter", "Alt+Tab"),
				action("Go to Previous Splitter", "editor.previousSplitter", "Alt+Shift+Tab"),
				rule(),
				action("Configure Editor Tabs…", "editor.configureTabs", ""),
			),
			rule(),
			action("Stretch to Top", "view.resizeUp", "Ctrl+Alt+Up"),
			action("Stretch to Bottom", "view.resizeDown", "Ctrl+Alt+Down"),
			action("Stretch to Left", "view.resizeLeft", "Ctrl+Alt+Left"),
			action("Stretch to Right", "view.resizeRight", "Ctrl+Alt+Right"),
			rule(),
			action("Move to Left Top", "view.moveToolToLeft", ""),
			action("Move to Left Bottom", "view.moveToolToLeftSplit", ""),
			action("Move to Right Top", "view.moveToolToRight", ""),
			action("Move to Right Bottom", "view.moveToolToRightSplit", ""),
			action("Move to Bottom Left", "view.movePrimary", ""),
			action("Move to Bottom Right", "view.moveSplit", ""),
			rule(),
			platform(mygo.RoleWindow),
		),
		// Help, as OpenIDEA's menu.rs builds it on macOS: Find Action, What's
		// New and the log; its Check for Updates sits in the gear menu there,
		// and stays here as well since GoIDEA has no custom app menu.
		submenu("Help",
			action("Find Action…", "navigate.findAction", "CmdOrCtrl+Shift+A"),
			action("Keymap Reference", "help.keymapReference", ""),
			action("What's New", "app.whatsNew", ""),
			action("Show Log in Finder", "help.showLog", ""),
			rule(),
			action("Check for Updates…", "app.checkForUpdates", ""),
			action("About GoIDEA", "app.about", ""),
		),
	}
}

// gearGroup is the gear menu of OpenIDEA's TitleBar.tsx gearMenuItems: Run
// Anything, Settings and Plugins, then the Theme, Keymap and View Mode quick
// switches, then the toolbar customization and the update check.
func gearGroup(m *Model) []menuEntry {
	return []menuEntry{
		action("Run Anything…", "runAnything.show", ""),
		rule(),
		action("Settings…", "file.settings", "CmdOrCtrl+,"),
		action("Plugins…", "plugins.settings", ""),
		rule(),
		submenu("Theme",
			actionChecked("Light", "app.themeLight", func(m *Model) bool { return m.prefs.Theme == "light" }),
			actionChecked("Dark", "app.themeDark", func(m *Model) bool { return m.prefs.Theme == "dark" }),
			actionChecked("Use System Settings", "app.themeSystem", func(m *Model) bool { return m.prefs.Theme == "" || m.prefs.Theme == "system" }),
			rule(),
			action("Appearance…", "settings.appearance", ""),
		),
		submenu("Keymap",
			menuEntry{label: keymapPresetTitle, check: func(*Model) bool { return true }},
			rule(),
			action("Configure Keymap…", "settings.keymap", ""),
		),
		submenu("View Mode",
			menuEntry{label: zenTitle(m), command: "editor.toggleZen"},
			action("Toggle Full Screen", "window.toggleFullScreen", "CmdOrCtrl+Ctrl+F"),
		),
		rule(),
		submenu("Customize Main Toolbar", m.customizeToolbarEntries()...),
		rule(),
		action("Check for Updates…", "app.checkForUpdates", ""),
	}
}

// keymapPresetTitle is the one keymap preset GoIDEA carries, named as
// OpenIDEA's PRESET_TITLES names IntelliJ's macOS map.
const keymapPresetTitle = "macOS"

// zenTitle is Enter/Exit Distraction Free Mode, the title following the mode
// as OpenIDEA's dynamic action title does.
func zenTitle(m *Model) string {
	if m.zen {
		return "Exit Distraction Free Mode"
	}
	return "Enter Distraction Free Mode"
}

// customizeToolbarEntries is Customize Main Toolbar's body: a checkbox per
// entry, shown and hidden alike, then the dialog and Restore Default Toolbar.
func (m *Model) customizeToolbarEntries() []menuEntry {
	var entries []menuEntry
	for _, id := range m.toolbarEntries() {
		id := id
		entries = append(entries, menuEntry{
			label: toolbarTitle(id),
			check: func(mm *Model) bool { return mm.toolbarShows(id) },
			run:   func(mm *Model) { mm.toggleToolbarWidget(id) },
		})
	}
	return append(entries, rule(),
		menuEntry{label: "Customize Toolbar…", run: func(mm *Model) { mm.OpenToolbarCustomize() }},
		menuEntry{label: "Restore Default Toolbar", run: func(mm *Model) { mm.RestoreDefaultToolbar() }},
	)
}

// MainMenu is the application's menu bar, the one OpenIDEA builds from its
// menu model, with every item running a registry action.
func (m *Model) MainMenu() *mygo.Menu {
	return mygo.NewMenu(append([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}}, menuItems(m, mainMenuGroups())...))
}

// toolWindowLayoutMenuItems is the dynamic middle of Window › Tool Window
// Layouts: every saved name restores its snapshot through layout.restore.
func toolWindowLayoutMenuItems(m *Model) []menuEntry {
	out := make([]menuEntry, 0, len(m.namedLayouts))
	for _, entry := range m.namedLayouts {
		entry := entry
		out = append(out, menuEntry{
			label:   entry.Name,
			command: "layout.restore",
			arg:     entry.Name,
			check:   func(mm *Model) bool { return mm.activeNamedLayout == entry.Name },
		})
	}
	return out
}

// menuItems turns a group of entries into platform menu items.
func menuItems(m *Model, groups []menuEntry) []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(groups))
	for _, g := range groups {
		if g.dynamic != nil {
			items = append(items, menuItems(m, g.dynamic(m))...)
			continue
		}
		if g.sep {
			items = append(items, mygo.Separator())
			continue
		}
		it := &mygo.MenuItem{Label: g.label, Role: g.role, Accelerator: g.accel}
		if len(g.sub) > 0 {
			it.Submenu = menuItems(m, g.sub)
		}
		if g.check != nil && g.check(m) {
			it.Checked = true
		}
		switch {
		case g.command != "":
			it.ID = "menu." + g.command
			id := g.command
			arg := g.arg
			it.Click = func(*mygo.MenuItem, *mygo.Window) { m.RunCommandWithArg(id, arg) }
		case g.run != nil:
			it.ID = "menu.custom"
			run := g.run
			it.Click = func(*mygo.MenuItem, *mygo.Window) { run(m) }
		}
		items = append(items, it)
	}
	return items
}
