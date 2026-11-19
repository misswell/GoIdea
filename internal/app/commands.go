package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// appCommand is one entry of the command palette. keywords widen what the
// query matches, so Go to File is found by "quick" and Show Git Changes by "git".
type appCommand struct {
	id       string
	title    string
	group    string
	shortcut string
	keywords string
	run      func(*Model)
}

// builtinCommands is the built-in contribution of the action registry: the
// commands this build ships. They are registered into the registry at start,
// which is what every entry point reads.
func builtinCommands() []appCommand {
	return []appCommand{
		{"project.new", "New Project…", "File", "Cmd+N", "new project create", func(m *Model) { m.NewProject() }},
		{"file.newScratchFile", "New Scratch File…", "File", "Cmd+Shift+N", "scratch temporary buffer new file language", func(m *Model) { m.OpenScratchChooser() }},
		{"project.open", "Open…", "File", "Cmd+O", "open project folder", func(m *Model) { m.ChooseProject() }},
		{"project.close", "Close Project", "File", "", "close project welcome", func(m *Model) { m.CloseProject() }},
		{"welcome.manageProjects", "Manage Projects…", "File", "", "manage projects welcome recent", func(m *Model) { m.showWelcome() }},
		{"navigate.searchEverywhere", "Search Everywhere", "Navigate", "Shift Shift", "search everywhere find class file symbol action text", func(m *Model) { m.OpenSearchEverywhere() }},
		{"navigate.gotoLine", "Go to Line:Column…", "Navigate", "Cmd+L", "go to line column caret", func(m *Model) { m.OpenGotoLine() }},
		{"navigate.recentFiles", "Recent Files", "Navigate", "Cmd+E", "recently opened edited files switcher", func(m *Model) { m.openRecentNavigation(false) }},
		{"navigate.recentLocations", "Recent Locations", "Navigate", "Cmd+Shift+E", "recently changed locations", func(m *Model) { m.openRecentNavigation(true) }},
		{"analyze.stackTrace", "Analyze Stack Trace or Thread Dump…", "Code", "", "exception console source line thread dump", func(m *Model) { m.OpenStackTrace() }},
		{"app.commandPalette", "Find Action…", "Navigate", "Cmd+Shift+A", "find action command palette search", func(m *Model) { m.OpenCommands() }},
		{"runAnything.show", "Run Anything…", "Run", "", "command shell execute npm mvn gradle goal", func(m *Model) { m.OpenRunAnything() }},
		{"plugins.settings", "Plugins…", "File", "", "plugins settings marketplace", func(m *Model) { m.openPlugins() }},
		{"file.quickOpen", "Go to File…", "File", "Cmd+P", "quick open file goto recent", func(m *Model) { m.OpenQuickFiles() }},
		{"file.save", "Save", "File", "Cmd+S", "save write", func(m *Model) { m.SaveActive() }},
		{"file.close", "Close Tab", "File", "Cmd+W", "close tab", func(m *Model) { m.closeTab(m.active) }},
		{"file.reloadAllFromDisk", "Reload All from Disk", "File", "", "reload all from disk synchronize refresh external", func(m *Model) { m.ReloadAllFromDisk() }},
		{"editor.pinTab", "Pin/Unpin Tab", "File", "", "pin unpin tab", func(m *Model) { m.PinActiveTab() }},
		{"editor.closeOthers", "Close Other Tabs", "File", "", "close other tabs", func(m *Model) { m.CloseOtherTabs(nil) }},
		{"editor.closeAll", "Close All Tabs", "File", "", "close all tabs", func(m *Model) { m.CloseAllTabs(false) }},
		{"editor.closeAllPinned", "Close Unpinned Tabs", "File", "", "close all but pinned tabs", func(m *Model) { m.CloseAllTabs(true) }},
		{"editor.reopenClosedTab", "Reopen Closed Tab", "File", "", "reopen closed tab history restore", func(m *Model) { m.ReopenClosedTab() }},
		{"editor.splitRight", "Split Right", "View", "Cmd+\\", "split editor right window group pane", func(m *Model) { m.SplitEditorRight() }},
		{"editor.splitDown", "Split Down", "View", "", "split editor down window group pane stacked", func(m *Model) { m.SplitEditorDown() }},
		{"editor.splitAndMoveRight", "Split and Move Right", "View", "", "split and move right tab window group", func(m *Model) { m.SplitAndMoveRight() }},
		{"editor.splitAndMoveDown", "Split and Move Down", "View", "", "split and move down tab window group", func(m *Model) { m.SplitAndMoveDown() }},
		{"editor.moveToOppositeGroup", "Move to Opposite Group", "View", "", "move tab opposite editor group pane", func(m *Model) { m.MoveToOppositeGroup() }},
		{"editor.openInOppositeGroup", "Open in Opposite Group", "View", "", "open tab opposite editor group pane duplicate", func(m *Model) { m.OpenInOppositeGroup() }},
		{"editor.changeSplitOrientation", "Change Splitter Orientation", "View", "", "change splitter orientation horizontal vertical", func(m *Model) { m.ChangeSplitterOrientation() }},
		{"editor.unsplit", "Unsplit", "View", "", "unsplit close editor group merge panes", func(m *Model) { m.Unsplit() }},
		{"editor.unsplitAll", "Unsplit All", "View", "", "unsplit all close other editor groups merge", func(m *Model) { m.UnsplitAll() }},
		{"editor.nextSplitter", "Go to Next Splitter", "View", "Alt+Tab", "go to next splitter editor group pane", func(m *Model) { m.GoToNextSplitter() }},
		{"editor.previousSplitter", "Go to Previous Splitter", "View", "Alt+Shift+Tab", "go to previous splitter editor group pane", func(m *Model) { m.GoToPreviousSplitter() }},
		{"editor.configureTabs", "Configure Editor Tabs…", "File", "", "configure editor tabs settings", func(m *Model) { m.OpenSettingsPage(settingsPageEditorTabs) }},
		{"view.notifications", "Notifications", "View", "", "notifications balloon history alerts unread", func(m *Model) { m.ToggleNotificationsWindow() }},
		{"view.maven", "Maven", "View", "", "maven tool window modules reactor", func(m *Model) { m.ToggleMaven() }},
		{"view.services", "Services", "View", "", "services build maven gradle run tools", func(m *Model) { m.ToggleServices() }},
		{"view.moveToolToRight", "Move Tool Window to Right", "View", "", "dock undock move tool window right", func(m *Model) { m.MoveToolToRight() }},
		{"view.moveToolToBottom", "Move Tool Window to Bottom", "View", "", "dock undock move tool window bottom", func(m *Model) { m.MoveToolToBottom() }},
		{"view.moveToolToLeft", "Move Tool Window to Left", "View", "", "dock move tool window left top", func(m *Model) { m.MoveToolToLeft() }},
		{"view.moveToolToLeftSplit", "Move Tool Window to Left Bottom", "View", "", "dock move tool window left bottom split", func(m *Model) { m.MoveToolToLeftSplit() }},
		{"view.moveToolToRightSplit", "Move Tool Window to Right Bottom", "View", "", "dock move tool window right bottom split", func(m *Model) { m.MoveToolToRightSplit() }},
		{"view.moveToolToBottomSplit", "Move Tool Window to Bottom Right", "View", "", "dock move tool window bottom right split", func(m *Model) { m.MoveToolToBottomSplit() }},
		{"view.pinToolWindow", "Pin Tool Window", "View", "", "dock pin tool window keep slot", func(m *Model) { m.ToggleToolWindowPin() }},
		{"view.viewModePinned", "View Mode: Dock Pinned", "View", "", "dock pinned view mode", func(m *Model) { m.SetToolViewMode(m.tool, "dockPinned") }},
		{"view.viewModeUnpinned", "View Mode: Dock Unpinned", "View", "", "dock unpinned hide focus view mode", func(m *Model) { m.SetToolViewMode(m.tool, "dockUnpinned") }},
		{"view.viewModeUndock", "View Mode: Undock", "View", "", "undock floating window view mode", func(m *Model) { m.SetToolViewMode(m.tool, "undock") }},
		{"notifications.clear", "Clear All Notifications", "View", "", "clear all notifications history", func(m *Model) { m.ClearNotifications() }},
		{"vcs.show", "Show Git Changes", "Git", "Cmd+9", "git vcs changes status", func(m *Model) { m.ShowVCS() }},
		{"vcs.log", "Show Git Log", "Git", "", "git vcs log history commits", func(m *Model) { m.ShowLog() }},
		{"vcs.branches", "Branches…", "Git", "", "git vcs branches checkout", func(m *Model) { m.ToggleBranches() }},
		{"vcs.commit", "Commit…", "Git", "Cmd+K", "git commit message stage", func(m *Model) { m.OpenCommit() }},
		{"vcs.diff", "Compare with HEAD", "Git", "Cmd+Shift+D", "git diff compare head", func(m *Model) { m.CompareActive() }},
		{"search.project", "Find in Files", "Search", "Cmd+Shift+F", "search find in project", func(m *Model) { m.ShowSearch() }},
		{"view.find", "Find", "Search", "Cmd+3", "find search tool window", func(m *Model) { m.ShowSearch() }},
		{"project.findInFolder", "Find in Folder…", "Search", "", "find in folder search directory scope selected", func(m *Model) { m.FindInFolderSelected() }},
		{"view.project", "Show Project", "View", "Cmd+1", "project tree files sidebar", func(m *Model) { m.ShowProject() }},
		{"project.expandAll", "Expand All", "Navigate", "Cmd+=", "expand all project tree folders", func(m *Model) { m.ExpandProjectAll() }},
		{"project.collapseAll", "Collapse All", "Navigate", "Cmd+-", "collapse all project tree folders", func(m *Model) { m.CollapseProjectAll() }},
		{"project.refresh", "Reload Project Tree from Disk", "File", "", "reload project tree disk synchronize refresh all", func(m *Model) { m.ReloadProjectTree() }},
		{"project.reloadFromDisk", "Reload from Disk", "File", "Cmd+Alt+Y", "reload selected folder from disk synchronize refresh", func(m *Model) { m.ReloadSelectedFromDisk() }},
		{"project.showHistory", "Show Local History", "File", "", "local history revisions snapshots restore", func(m *Model) { m.ShowHistorySelected() }},
		{"project.putLabel", "Put Label…", "File", "", "local history put label version", func(m *Model) { m.OpenHistoryLabel() }},
		{"project.compareDirectories", "Compare Directories…", "File", "", "compare directories diff folders", func(m *Model) { m.CompareDirectoriesSelected() }},
		{"navigate.selectInProjectView", "Select in Project View", "Navigate", "Alt+F1", "select in project view reveal active file locate", func(m *Model) { m.SelectInProjectView() }},
		{"navigate.callHierarchy", "Call Hierarchy", "Navigate", "Ctrl+Alt+H", "call hierarchy callers callees", func(m *Model) { m.OpenCallHierarchy() }},
		{"navigate.typeHierarchy", "Type Hierarchy", "Navigate", "Ctrl+H", "type hierarchy supertypes subtypes class", func(m *Model) { m.OpenTypeHierarchy() }},
		{"navigate.implementation", "Go to Implementation(s)", "Navigate", "Cmd+Alt+B", "implementation implementations overrides goto", func(m *Model) { m.GoToImplementation() }},
		{"navigate.typeDeclaration", "Go to Type Declaration", "Navigate", "Cmd+Shift+B", "type declaration class type goto", func(m *Model) { m.GoToTypeDeclaration() }},
		{"view.terminal", "Show Terminal", "View", "Alt+F12", "terminal shell", func(m *Model) { m.ShowTerm() }},
		{"term.new", "New Terminal Tab", "Terminal", "Cmd+T", "terminal new tab shell", func(m *Model) { m.OpenTermTab() }},
		{"term.newWithShell", "New Terminal Tab (Choose Shell)…", "Terminal", "", "terminal new tab choose shell list", func(m *Model) { m.OpenTermShellPicker() }},
		{"term.splitRight", "Split Terminal Right", "Terminal", "", "terminal split right pane group", func(m *Model) { m.SplitTermRight() }},
		{"term.splitDown", "Split Terminal Down", "Terminal", "", "terminal split down pane group", func(m *Model) { m.SplitTermDown() }},
		{"term.unsplit", "Unsplit Terminal", "Terminal", "", "terminal unsplit merge pane group", func(m *Model) { m.UnsplitTerm() }},
		{"term.close", "Close Current Terminal Tab", "Terminal", "", "terminal close tab session", func(m *Model) { m.CloseActiveTermTab() }},
		{"term.clear", "Clear Terminal", "Terminal", "", "terminal clear buffer", func(m *Model) { m.ClearTerm() }},
		{"term.find", "Find in Terminal", "Terminal", "Cmd+F", "terminal find search scrollback", func(m *Model) { m.ShowTermFind() }},
		{"view.run", "Show Run Window", "View", "Cmd+4", "run output console", func(m *Model) { m.ShowRun() }},
		{"view.debug", "Show Debug Window", "View", "Cmd+5", "debug variables frames", func(m *Model) { m.ShowDebug() }},
		{"view.problems", "Show Problems", "View", "Cmd+6", "problems diagnostics errors warnings inspections", func(m *Model) { m.ShowProblems() }},
		{"view.structure", "Show Structure", "View", "Cmd+7", "structure outline symbols members file", func(m *Model) { m.ShowStructure() }},
		{"view.todo", "Show TODO", "View", "", "todo fixme hack xxx comments patterns", func(m *Model) { m.ShowTodo() }},
		{"todo.refresh", "Refresh TODO Items", "Tools", "", "todo refresh rescan patterns", func(m *Model) { m.RunTodo() }},
		{"todo.showCurrentFile", "Show TODO Items in Current File", "Tools", "", "todo current file", func(m *Model) { m.ShowTodoCurrentFile() }},
		{"view.hideActive", "Hide Active Tool Window", "View", "Shift+Escape", "hide tool window active", func(m *Model) { m.HideActiveToolWindow() }},
		{"view.hideAll", "Hide All Tool Windows", "View", "Cmd+Shift+F12", "hide all tool windows restore", func(m *Model) { m.HideAllToolWindows() }},
		{"view.jumpLast", "Jump to Last Tool Window", "View", "F12", "jump last tool window", func(m *Model) { m.JumpToLastToolWindow() }},
		{"view.maximize", "Maximize Tool Window", "View", "Cmd+Shift+'", "maximize tool window restore size", func(m *Model) { m.ToggleMaximizeToolWindow() }},
		{"view.resetLayout", "Restore Default Layout", "View", "", "restore default layout reset tool windows split", func(m *Model) { m.ResetToolWindowLayout() }},
		{"view.resizeUp", "Stretch Tool Window Up", "View", "Ctrl+Alt+Up", "resize stretch tool window taller", func(m *Model) { m.ResizeBottomToolWindow("up") }},
		{"view.resizeDown", "Stretch Tool Window Down", "View", "Ctrl+Alt+Down", "resize stretch tool window shorter", func(m *Model) { m.ResizeBottomToolWindow("down") }},
		{"view.resizeLeft", "Stretch Tool Window Left", "View", "Ctrl+Alt+Left", "resize stretch tool window left", func(m *Model) { m.ResizeBottomToolWindow("left") }},
		{"view.resizeRight", "Stretch Tool Window Right", "View", "Ctrl+Alt+Right", "resize stretch tool window right", func(m *Model) { m.ResizeBottomToolWindow("right") }},
		{"view.moveSplit", "Move to Bottom Right", "View", "", "move tool window split bottom right", func(m *Model) { m.MoveActiveToolWindowToSplit() }},
		{"view.movePrimary", "Move to Bottom", "View", "", "move tool window primary bottom", func(m *Model) { m.MoveActiveToolWindowToBottom() }},
		{"view.commit", "Commit Window", "View", "Cmd+0", "commit tool window git", func(m *Model) { m.OpenCommit() }},
		{"problems.next", "Next Problem", "View", "Cmd+Alt+Down", "problems next occurrence navigate", func(m *Model) { m.StepProblem(1) }},
		{"problems.prev", "Previous Problem", "View", "Cmd+Alt+Up", "problems previous occurrence navigate", func(m *Model) { m.StepProblem(-1) }},
		{"view.database", "Database…", "View", "", "database sql sqlite postgres mysql query", func(m *Model) { m.OpenDatabase() }},
		{"code.intentions", "Show Context Actions", "Code", "Alt+Enter", "intention action quickfix", func(m *Model) { m.ShowIntentions() }},
		{"code.symbols", "Go to Symbol…", "Code", "Cmd+Shift+O", "go to symbol navigate", func(m *Model) { m.OpenSymbols() }},
		{"code.usages", "Find Usages", "Code", "Alt+F7", "find usages references", func(m *Model) { m.FindUsages() }},
		{"code.showUsages", "Show Usages", "Code", "", "show usages results window", func(m *Model) { m.showUsages() }},
		{"bookmarks.toggle", "Toggle Bookmark", "Navigate", "F3", "bookmark toggle line mark", func(m *Model) { m.ToggleBookmark() }},
		{"bookmarks.show", "Show Bookmarks", "Navigate", "Cmd+F3", "bookmarks show popup list", func(m *Model) { m.ShowBookmarks() }},
		{"bookmarks.next", "Next Bookmark", "Navigate", "", "next bookmark navigate", func(m *Model) { m.NextBookmark() }},
		{"bookmarks.prev", "Previous Bookmark", "Navigate", "", "previous bookmark navigate", func(m *Model) { m.PrevBookmark() }},
		{"bookmarks.mnemonic", "Add Mnemonic Bookmark…", "Navigate", "Alt+F3", "bookmark mnemonic key", func(m *Model) { m.OpenBookmarkMnemonics() }},
		{"bookmarks.window", "Bookmarks Tool Window", "Navigate", "Cmd+2", "bookmarks tool window", func(m *Model) { m.ToggleBookmarksWindow() }},
		{"bookmarks.file", "Toggle File Bookmark", "Navigate", "", "file bookmark", func(m *Model) { m.ToggleFileBookmark() }},
		{"bookmarks.newList", "New Bookmark List…", "Navigate", "", "new bookmark list", func(m *Model) { m.OpenNewBookmarkList() }},
		{"code.rename", "Rename Symbol", "Refactor", "Shift+F6", "rename symbol refactor", func(m *Model) { m.RenameActive() }},
		{"code.undoRefactor", "Undo Refactoring", "Refactor", "Cmd+Alt+Z", "undo refactor", func(m *Model) { m.UndoRefactor() }},
		{"code.redoRefactor", "Redo Refactoring", "Refactor", "Cmd+Alt+Shift+Z", "redo refactor", func(m *Model) { m.RedoRefactor() }},
		{"code.format", "Reformat Code", "Code", "Cmd+Alt+L", "format code", func(m *Model) { m.FormatActive() }},
		{"code.commentLine", "Comment with Line Comment", "Code", "Cmd+/", "comment line toggle", func(m *Model) { m.runEditorCommand(editor.ToggleComment) }},
		{"code.commentBlock", "Comment with Block Comment", "Code", "Cmd+Alt+/", "comment block toggle", func(m *Model) { m.runEditorCommand(editor.ToggleBlockComment) }},
		{"code.duplicateLine", "Duplicate Line or Selection", "Code", "Cmd+D", "duplicate line selection", func(m *Model) { m.runEditorCommand(editor.DuplicateSelection) }},
		{"code.deleteLine", "Delete Line", "Code", "Cmd+Backspace", "delete line", func(m *Model) { m.runEditorCommand(editor.DeleteLine) }},
		{"code.moveLineUp", "Move Line Up", "Code", "Alt+Shift+Up", "move line up", func(m *Model) { m.runEditorCommand(editor.MoveLinesUp) }},
		{"code.moveLineDown", "Move Line Down", "Code", "Alt+Shift+Down", "move line down", func(m *Model) { m.runEditorCommand(editor.MoveLinesDown) }},
		{"code.moveStatementUp", "Move Statement Up", "Code", "Cmd+Shift+Up", "move statement up", func(m *Model) { m.runEditorCommand(editor.MoveLinesUp) }},
		{"code.moveStatementDown", "Move Statement Down", "Code", "Cmd+Shift+Down", "move statement down", func(m *Model) { m.runEditorCommand(editor.MoveLinesDown) }},
		{"code.joinLines", "Join Lines", "Code", "Ctrl+Shift+J", "join lines", func(m *Model) { m.runEditorCommand(editor.JoinLines) }},
		{"run.start", "Run", "Run", "F5", "run main class", func(m *Model) { m.Run() }},
		{"run.rerun", "Rerun", "Run", "Cmd+R", "rerun restart process", func(m *Model) { m.Rerun() }},
		{"run.rerunFailed", "Rerun Failed Tests", "Run", "", "rerun failed tests", func(m *Model) { m.RerunFailedTests() }},
		{"debug.reloadChangedClasses", "Reload Changed Classes", "Run", "", "hotswap hot swap reload changed classes hot code replace", func(m *Model) { m.HotSwapReload() }},
		{"run.chooseConfig", "Choose Run/Debug Configuration…", "Run", "Ctrl+Alt+R", "choose run configuration select popup", func(m *Model) { m.OpenRunChooser("run") }},
		{"run.chooseDebugConfig", "Choose Debug Configuration…", "Run", "Ctrl+Alt+D", "choose debug configuration popup", func(m *Model) { m.OpenRunChooser("debug") }},
		{"run.editConfigs", "Edit Configurations…", "Run", "Ctrl+Alt+Shift+R", "edit run debug configurations dialog", func(m *Model) { m.OpenRunConfigs() }},
		{"run.debug", "Debug", "Run", "F9", "debug selected configuration", func(m *Model) { m.Debug() }},
		{"run.debugContext", "Debug Current File", "Run", "Ctrl+Shift+D", "debug context active file", func(m *Model) { m.DebugContext() }},
		{"run.stop", "Stop", "Run", "Shift+F5", "stop terminate", func(m *Model) { m.StopRun(); m.StopDebug() }},
		{"debug.attach", "Attach to Process…", "Run", "Alt+Shift+F5", "attach to process jdwp java", func(m *Model) { m.OpenAttach() }},
		{"run.breakpoint", "Toggle Breakpoint", "Run", "Cmd+F8", "toggle breakpoint", func(m *Model) { m.ToggleBreakpoint() }},
		{"run.stepOver", "Step Over", "Run", "F8", "step over", func(m *Model) { m.StepOver() }},
		{"run.stepInto", "Step Into", "Run", "F7", "step into", func(m *Model) { m.StepInto() }},
		{"run.stepOut", "Step Out", "Run", "Shift+F8", "step out", func(m *Model) { m.StepOut() }},
		{"run.runToCursor", "Run to Cursor", "Run", "Alt+F9", "run to cursor continue", func(m *Model) { m.RunToCursor(false) }},
		{"run.forceRunToCursor", "Force Run to Cursor", "Run", "Cmd+Alt+F9", "force run to cursor ignore breakpoints", func(m *Model) { m.ForceRunToCursor() }},
		{"run.continue", "Resume Program", "Run", "", "continue resume", func(m *Model) { m.DebugContinue() }},
		{"run.pause", "Pause Program", "Run", "", "pause suspend", func(m *Model) { m.DebugPause() }},
		{"run.trust", "Trust Project", "Run", "", "trust project", func(m *Model) { m.TrustProject() }},
		{"run.reloadConfigs", "Reload Run Configurations", "Run", "", "reload run configurations", func(m *Model) { m.ReloadRunConfigs() }},
		{"run.configNext", "Next Run Configuration", "Run", "", "next run configuration", func(m *Model) { m.CycleRunConfig(1) }},
		{"run.configPrev", "Previous Run Configuration", "Run", "", "previous run configuration", func(m *Model) { m.CycleRunConfig(-1) }},
		{"build.buildProject", "Build Project", "Build", "Cmd+F9", "build project compile make mvn gradle cmake", func(m *Model) { m.BuildProject() }},
		{"build.buildModule", "Build Module", "Build", "Cmd+Shift+F9", "build module compile file", func(m *Model) { m.BuildModule() }},
		{"build.compileFile", "Compile File", "Build", "", "javac single file recompile", func(m *Model) { m.CompileFile() }},
		{"build.rebuildProject", "Rebuild Project", "Build", "", "rebuild clean build", func(m *Model) { m.RebuildProject() }},
		{"build.buildArtifacts", "Build Artifacts", "Build", "", "artifacts package assemble jar release", func(m *Model) { m.BuildArtifacts() }},
		{"build.stop", "Stop Build", "Build", "", "stop cancel build", func(m *Model) { m.StopBuild() }},
		{"view.build", "Show Build Window", "View", "", "build output console", func(m *Model) { m.ShowBuild() }},
		{"java.tools", "Install Java Language Services and Debugger", "Run", "", "java jdtls language server debugger install toolchain", func(m *Model) { m.InstallJavaToolchain() }},
		{"file.settings", "Settings…", "File", "Cmd+,", "settings preferences theme font appearance", func(m *Model) { m.OpenSettings() }},
		{"file.invalidateCaches", "Invalidate Caches…", "File", "", "invalidate caches clear rebuild index restart repair", func(m *Model) { m.OpenInvalidateCaches() }},
		{"app.restart", "Restart IDE", "File", "", "restart relaunch restart ide", func(m *Model) { m.RestartIDE() }},
		{"view.toggleTheme", "Toggle Dark/Light Theme", "View", "", "theme dark light appearance toggle", func(m *Model) { m.ToggleTheme() }},
		{"app.themeLight", "Light", "View", "", "theme light appearance", func(m *Model) { m.SetThemePref("light") }},
		{"app.themeDark", "Dark", "View", "", "theme dark appearance", func(m *Model) { m.SetThemePref("dark") }},
		{"app.themeSystem", "Sync with OS", "View", "", "theme system appearance", func(m *Model) { m.SetThemePref("system") }},
		{"editor.toggleZen", "Toggle Distraction Free Mode", "View", "", "distraction free mode zen presentation", func(m *Model) { m.ToggleZen() }},
		{"window.toggleFullScreen", "Toggle Full Screen", "View", "Cmd+Ctrl+F", "fullscreen window display", func(m *Model) { m.ToggleFullScreen() }},
		{"settings.appearance", "Appearance…", "View", "", "settings appearance theme font", func(m *Model) { m.OpenSettingsPage("appearance") }},
		{"settings.keymap", "Keymap…", "View", "", "settings keymap shortcuts bindings", func(m *Model) { m.OpenSettingsPage("keymap") }},
		{"toolbar.customize", "Customize Main Toolbar…", "View", "", "toolbar widgets title bar customize", func(m *Model) { m.OpenToolbarCustomize() }},
		{"toolbar.reset", "Restore Default Toolbar", "View", "", "toolbar default widgets restore", func(m *Model) { m.RestoreDefaultToolbar() }},
		{"app.checkForUpdates", "Check for Updates…", "Help", "", "update upgrade version release", func(m *Model) { m.CheckForUpdates() }},
		{"app.whatsNew", "What's New", "Help", "", "whats new release notes changelog update notes", func(m *Model) { m.OpenCurrentWhatsNew() }},
	}
}

// Command returns the registry's command with id, the one entry the menu, the
// title bar and the palette run a feature through.
// commands is every action the palette offers, in registration order: the
// registry's built-ins and anything registered since.
func commands() []appCommand {
	list := actionList()
	out := make([]appCommand, 0, len(list))
	for _, a := range list {
		out = append(out, commandOf(a))
	}
	return out
}

// Command looks an action up by id.
func Command(id string) (appCommand, bool) {
	a, ok := actionGet(id)
	if !ok {
		return appCommand{}, false
	}
	return commandOf(a), true
}

// commandOf adapts a registry action to the palette's row, with the shortcut
// the keymap gives it.
func commandOf(a *Action) appCommand {
	return appCommand{
		id:       a.ID,
		title:    a.Title,
		group:    a.Category,
		shortcut: keymapShortcutLabel(a.ID),
		keywords: a.Keywords,
		run:      a.Run,
	}
}

// RunCommand runs the registry command with id and reports whether the
// registry knew it.
func (m *Model) RunCommand(id string) bool {
	if !m.RunAction(id) {
		return false
	}
	m.rememberRecentAction(id)
	return true
}

// commandPicker is the open command palette: the query and the commands it
// still admits.
type commandPicker struct {
	open  bool
	query string
	items []appCommand
	sel   int
}

// OpenCommands opens the command palette over the window.
func (m *Model) OpenCommands() {
	if m.palette != nil {
		return
	}
	m.palette = &commandPicker{open: true}
	m.palette.refresh()
	m.invalidate()
}

func (m *Model) closeCommands() {
	if m.palette == nil {
		return
	}
	m.palette = nil
	m.invalidate()
}

// refresh filters the registry against the typed query and keeps the chosen
// row in range.
func (p *commandPicker) refresh() {
	all := commands()
	p.items = p.items[:0]
	for _, cmd := range all {
		if commandMatches(cmd, p.query) {
			p.items = append(p.items, cmd)
		}
	}
	p.sel = clampSymbolSel(p.sel, len(p.items))
}

// commandMatches reports whether the query is a subsequence of what the
// command is called or the words it is known by.
func commandMatches(cmd appCommand, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	q := []rune(query)
	hay := strings.ToLower(cmd.title + " " + cmd.group + " " + cmd.keywords)
	i := 0
	for _, r := range hay {
		if r == q[i] {
			i++
			if i == len(q) {
				return true
			}
		}
	}
	return false
}

// stepCommand moves the chosen row by delta, stopping at the ends.
func (m *Model) stepCommand(delta int) {
	p := m.palette
	if p == nil || len(p.items) == 0 {
		return
	}
	p.sel = clampSymbolSel(p.sel+delta, len(p.items))
	m.invalidate()
}

// runSelectedCommand runs the chosen command and closes the palette first,
// since the command may open a dialog of its own.
func (m *Model) runSelectedCommand() {
	p := m.palette
	if p == nil || len(p.items) == 0 {
		m.invalidate()
		return
	}
	cmd := p.items[clampSymbolSel(p.sel, len(p.items))]
	m.closeCommands()
	cmd.run(m)
}

// commandsDialog draws the command palette.
func (m *Model) commandsDialog(c *ui.Context, p *ui.Theme) {
	pal := m.palette
	if pal == nil {
		return
	}
	open := pal.open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(600).Gap(8).Children(func() {
			ui.Text(c, "Commands").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &pal.query).AutoFocus()
			in.OnChange(func() { pal.refresh() })
			in.OnShortcut(0, ui.KeyDown, func() { m.stepCommand(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepCommand(-1) })
			if in.Submitted() {
				m.runSelectedCommand()
			}
			hint := "Type a command name; Enter runs it"
			switch {
			case strings.TrimSpace(pal.query) != "" && len(pal.items) == 0:
				hint = "No matching commands"
			case strings.TrimSpace(pal.query) != "":
				hint = fmt.Sprintf("%d commands", len(pal.items))
			}
			ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted).SingleLine()
			if len(pal.items) == 0 {
				return
			}
			ui.Scroll(c).Height(380).Children(func() {
				ui.Column(c).Children(func() {
					for i, cmd := range pal.items {
						i, cmd := i, cmd
						r := ui.Row(c.Key("cmd:"+cmd.id)).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == pal.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							ui.Text(c, cmd.title).FontSize(12).TextColor(p.Text).
								SingleLine().Ellipsis("…")
							ui.Spacer(c)
							ui.Text(c, cmd.group).FontSize(12).TextColor(p.TextMuted).SingleLine()
							if cmd.shortcut != "" {
								ui.Text(c, cmd.shortcut).FontSize(12).TextColor(p.TextMuted).SingleLine()
							}
						})
						if r.Clicked() {
							pal.sel = i
							m.runSelectedCommand()
						}
					}
				})
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeCommands()
	}
}

// SmokeCommands drives the command palette offscreen: listing, filtering and
// running a command through the same path a keystroke takes.
func (m *Model) SmokeCommands() map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		m.OpenCommands()
		if m.palette == nil {
			out["error"] = "The command palette did not open"
			return
		}
		out["all"] = len(m.palette.items)
		m.palette.query = "git"
		m.palette.refresh()
		out["git"] = len(m.palette.items)
		if len(m.palette.items) > 0 {
			out["first"] = m.palette.items[0].title
		}
		m.palette.query = "Show Git Changes"
		m.palette.refresh()
		m.runSelectedCommand()
		out["sideMode"] = m.sideMode
		out["closed"] = m.palette == nil
		m.OpenInvalidateCaches()
		out["invalidateOpen"] = m.invalidateDlg != nil && m.invalidateDlg.open
		m.invalidateDlg.languageServers = true
		out["invalidateStaged"] = m.invalidateDlg.languageServers
		m.closeInvalidateCaches()
		out["invalidateClosed"] = m.invalidateDlg == nil
	})
	return out
}
