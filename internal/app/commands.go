package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// appCommand is one entry of the command palette. keywords widen what the
// query matches, so 快速打开 is found by "quick" and 显示 Git 更改 by "git".
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
		{"project.new", "新建项目…", "文件", "Cmd+N", "new project create", func(m *Model) { m.NewProject() }},
		{"file.newScratchFile", "New Scratch File…", "文件", "Cmd+Shift+N", "scratch temporary buffer new file language", func(m *Model) { m.OpenScratchChooser() }},
		{"project.open", "打开项目…", "文件", "Cmd+O", "open project folder", func(m *Model) { m.ChooseProject() }},
		{"welcome.manageProjects", "管理项目…", "文件", "", "manage projects welcome recent", func(m *Model) { m.showWelcome() }},
		{"navigate.searchEverywhere", "Search Everywhere", "导航", "Shift Shift", "search everywhere find class file symbol action text", func(m *Model) { m.OpenSearchEverywhere() }},
		{"app.commandPalette", "Find Action…", "导航", "Cmd+Shift+A", "find action command palette search", func(m *Model) { m.OpenCommands() }},
		{"runAnything.show", "Run Anything…", "运行", "", "command shell execute npm mvn gradle goal", func(m *Model) { m.OpenRunAnything() }},
		{"plugins.settings", "Plugins…", "文件", "", "plugins settings marketplace", func(m *Model) { m.openPlugins() }},
		{"file.quickOpen", "快速打开…", "文件", "Cmd+P", "quick open file goto recent", func(m *Model) { m.OpenQuickFiles() }},
		{"file.save", "保存", "文件", "Cmd+S", "save write", func(m *Model) { m.SaveActive() }},
		{"file.close", "关闭标签页", "文件", "Cmd+W", "close tab", func(m *Model) { m.closeTab(m.active) }},
		{"file.reloadAllFromDisk", "Reload All from Disk", "文件", "", "reload all from disk synchronize refresh external", func(m *Model) { m.ReloadAllFromDisk() }},
		{"editor.pinTab", "固定/取消固定标签页", "文件", "", "pin unpin tab", func(m *Model) { m.PinActiveTab() }},
		{"editor.closeOthers", "关闭其他标签页", "文件", "", "close other tabs", func(m *Model) { m.CloseOtherTabs(nil) }},
		{"editor.closeAll", "关闭所有标签页", "文件", "", "close all tabs", func(m *Model) { m.CloseAllTabs(false) }},
		{"editor.closeAllPinned", "关闭未固定的标签页", "文件", "", "close all but pinned tabs", func(m *Model) { m.CloseAllTabs(true) }},
		{"editor.reopenClosedTab", "Reopen Closed Tab", "文件", "", "reopen closed tab history restore", func(m *Model) { m.ReopenClosedTab() }},
		{"editor.splitRight", "向右拆分编辑器", "视图", "Cmd+\\", "split editor right window group pane", func(m *Model) { m.SplitEditorRight() }},
		{"editor.splitDown", "向下拆分编辑器", "视图", "", "split editor down window group pane stacked", func(m *Model) { m.SplitEditorDown() }},
		{"editor.splitAndMoveRight", "向右拆分并移动", "视图", "", "split and move right tab window group", func(m *Model) { m.SplitAndMoveRight() }},
		{"editor.splitAndMoveDown", "向下拆分并移动", "视图", "", "split and move down tab window group", func(m *Model) { m.SplitAndMoveDown() }},
		{"editor.moveToOppositeGroup", "移到另一组", "视图", "", "move tab opposite editor group pane", func(m *Model) { m.MoveToOppositeGroup() }},
		{"editor.openInOppositeGroup", "在另一组中打开", "视图", "", "open tab opposite editor group pane duplicate", func(m *Model) { m.OpenInOppositeGroup() }},
		{"editor.changeSplitOrientation", "切换拆分方向", "视图", "", "change splitter orientation horizontal vertical", func(m *Model) { m.ChangeSplitterOrientation() }},
		{"editor.unsplit", "合并编辑器组", "视图", "", "unsplit close editor group merge panes", func(m *Model) { m.Unsplit() }},
		{"editor.unsplitAll", "合并全部编辑器组", "视图", "", "unsplit all close other editor groups merge", func(m *Model) { m.UnsplitAll() }},
		{"editor.nextSplitter", "转到下一个编辑器组", "视图", "Alt+Tab", "go to next splitter editor group pane", func(m *Model) { m.GoToNextSplitter() }},
		{"editor.previousSplitter", "转到上一个编辑器组", "视图", "Alt+Shift+Tab", "go to previous splitter editor group pane", func(m *Model) { m.GoToPreviousSplitter() }},
		{"editor.configureTabs", "配置编辑器标签页…", "文件", "", "configure editor tabs settings", func(m *Model) { m.OpenSettingsPage(settingsPageEditorTabs) }},
		{"view.notifications", "Notifications", "视图", "", "notifications balloon history alerts unread", func(m *Model) { m.ToggleNotificationsWindow() }},
		{"view.maven", "Maven", "视图", "", "maven tool window modules reactor", func(m *Model) { m.ToggleMaven() }},
		{"view.services", "Services", "视图", "", "services build maven gradle run tools", func(m *Model) { m.ToggleServices() }},
		{"view.moveToolToRight", "Move Tool Window to Right", "视图", "", "dock undock move tool window right", func(m *Model) { m.MoveToolToRight() }},
		{"view.moveToolToBottom", "Move Tool Window to Bottom", "视图", "", "dock undock move tool window bottom", func(m *Model) { m.MoveToolToBottom() }},
		{"view.moveToolToLeft", "Move Tool Window to Left", "视图", "", "dock move tool window left top", func(m *Model) { m.MoveToolToLeft() }},
		{"view.moveToolToLeftSplit", "Move Tool Window to Left Bottom", "视图", "", "dock move tool window left bottom split", func(m *Model) { m.MoveToolToLeftSplit() }},
		{"view.moveToolToRightSplit", "Move Tool Window to Right Bottom", "视图", "", "dock move tool window right bottom split", func(m *Model) { m.MoveToolToRightSplit() }},
		{"view.moveToolToBottomSplit", "Move Tool Window to Bottom Right", "视图", "", "dock move tool window bottom right split", func(m *Model) { m.MoveToolToBottomSplit() }},
		{"view.pinToolWindow", "Pin Tool Window", "视图", "", "dock pin tool window keep slot", func(m *Model) { m.ToggleToolWindowPin() }},
		{"view.viewModePinned", "View Mode: Dock Pinned", "视图", "", "dock pinned view mode", func(m *Model) { m.SetToolViewMode(m.tool, "dockPinned") }},
		{"view.viewModeUnpinned", "View Mode: Dock Unpinned", "视图", "", "dock unpinned hide focus view mode", func(m *Model) { m.SetToolViewMode(m.tool, "dockUnpinned") }},
		{"view.viewModeUndock", "View Mode: Undock", "视图", "", "undock floating window view mode", func(m *Model) { m.SetToolViewMode(m.tool, "undock") }},
		{"notifications.clear", "Clear All Notifications", "视图", "", "clear all notifications history", func(m *Model) { m.ClearNotifications() }},
		{"vcs.show", "显示 Git 更改", "Git", "Cmd+9", "git vcs changes status", func(m *Model) { m.ShowVCS() }},
		{"vcs.log", "显示 Git 日志", "Git", "", "git vcs log history commits", func(m *Model) { m.ShowLog() }},
		{"vcs.branches", "分支…", "Git", "", "git vcs branches checkout", func(m *Model) { m.ToggleBranches() }},
		{"vcs.commit", "提交…", "Git", "Cmd+K", "git commit message stage", func(m *Model) { m.OpenCommit() }},
		{"vcs.diff", "与 HEAD 比较", "Git", "Cmd+Shift+D", "git diff compare head", func(m *Model) { m.CompareActive() }},
		{"search.project", "在项目中查找", "搜索", "Cmd+Shift+F", "search find in project", func(m *Model) { m.ShowSearch() }},
		{"view.find", "查找", "搜索", "Cmd+3", "find search tool window", func(m *Model) { m.ShowSearch() }},
		{"project.findInFolder", "在文件夹中查找…", "搜索", "", "find in folder search directory scope selected", func(m *Model) { m.FindInFolderSelected() }},
		{"view.project", "显示项目", "视图", "Cmd+1", "project tree files sidebar", func(m *Model) { m.ShowProject() }},
		{"project.expandAll", "展开全部", "导航", "Cmd+=", "expand all project tree folders", func(m *Model) { m.ExpandProjectAll() }},
		{"project.collapseAll", "折叠全部", "导航", "Cmd+-", "collapse all project tree folders", func(m *Model) { m.CollapseProjectAll() }},
		{"project.refresh", "从磁盘重新加载项目树", "文件", "", "reload project tree disk synchronize refresh all", func(m *Model) { m.ReloadProjectTree() }},
		{"project.reloadFromDisk", "从磁盘重新加载", "文件", "Cmd+Alt+Y", "reload selected folder from disk synchronize refresh", func(m *Model) { m.ReloadSelectedFromDisk() }},
		{"project.showHistory", "显示本地历史", "文件", "", "local history revisions snapshots restore", func(m *Model) { m.ShowHistorySelected() }},
		{"project.putLabel", "标记版本…", "文件", "", "local history put label version", func(m *Model) { m.OpenHistoryLabel() }},
		{"project.compareDirectories", "比较目录…", "文件", "", "compare directories diff folders", func(m *Model) { m.CompareDirectoriesSelected() }},
		{"navigate.selectInProjectView", "在项目视图中选择", "导航", "Alt+F1", "select in project view reveal active file locate", func(m *Model) { m.SelectInProjectView() }},
		{"navigate.callHierarchy", "Call Hierarchy", "导航", "Ctrl+Alt+H", "call hierarchy callers callees", func(m *Model) { m.OpenCallHierarchy() }},
		{"navigate.typeHierarchy", "Type Hierarchy", "导航", "Ctrl+H", "type hierarchy supertypes subtypes class", func(m *Model) { m.OpenTypeHierarchy() }},
		{"navigate.implementation", "转到实现", "导航", "Cmd+Alt+B", "implementation implementations overrides goto", func(m *Model) { m.GoToImplementation() }},
		{"view.terminal", "显示终端", "视图", "Alt+F12", "terminal shell", func(m *Model) { m.ShowTerm() }},
		{"term.new", "新建终端标签", "终端", "Cmd+T", "terminal new tab shell", func(m *Model) { m.OpenTermTab() }},
		{"term.splitRight", "向右拆分终端", "终端", "", "terminal split right pane group", func(m *Model) { m.SplitTermRight() }},
		{"term.splitDown", "向下拆分终端", "终端", "", "terminal split down pane group", func(m *Model) { m.SplitTermDown() }},
		{"term.unsplit", "合并终端拆分", "终端", "", "terminal unsplit merge pane group", func(m *Model) { m.UnsplitTerm() }},
		{"term.close", "关闭当前终端标签", "终端", "", "terminal close tab session", func(m *Model) { m.CloseActiveTermTab() }},
		{"term.clear", "清空终端输出", "终端", "", "terminal clear buffer", func(m *Model) { m.ClearTerm() }},
		{"term.find", "在终端中查找", "终端", "Cmd+F", "terminal find search scrollback", func(m *Model) { m.ShowTermFind() }},
		{"view.run", "显示运行窗口", "视图", "Cmd+4", "run output console", func(m *Model) { m.ShowRun() }},
		{"view.debug", "显示调试窗口", "视图", "Cmd+5", "debug variables frames", func(m *Model) { m.ShowDebug() }},
		{"view.problems", "显示问题", "视图", "Cmd+6", "problems diagnostics errors warnings inspections", func(m *Model) { m.ShowProblems() }},
		{"view.structure", "显示结构", "视图", "Cmd+7", "structure outline symbols members file", func(m *Model) { m.ShowStructure() }},
		{"view.todo", "显示 TODO", "视图", "", "todo fixme hack xxx comments patterns", func(m *Model) { m.ShowTodo() }},
		{"todo.refresh", "Refresh TODO Items", "工具", "", "todo refresh rescan patterns", func(m *Model) { m.RunTodo() }},
		{"todo.showCurrentFile", "Show TODO Items in Current File", "工具", "", "todo current file", func(m *Model) { m.ShowTodoCurrentFile() }},
		{"view.hideActive", "隐藏当前工具窗口", "视图", "Shift+Escape", "hide tool window active", func(m *Model) { m.HideActiveToolWindow() }},
		{"view.hideAll", "隐藏全部工具窗口", "视图", "Cmd+Shift+F12", "hide all tool windows restore", func(m *Model) { m.HideAllToolWindows() }},
		{"view.jumpLast", "跳转到上一个工具窗口", "视图", "F12", "jump last tool window", func(m *Model) { m.JumpToLastToolWindow() }},
		{"view.maximize", "最大化工具窗口", "视图", "Cmd+Shift+'", "maximize tool window restore size", func(m *Model) { m.ToggleMaximizeToolWindow() }},
		{"view.resetLayout", "恢复默认布局", "视图", "", "restore default layout reset tool windows split", func(m *Model) { m.ResetToolWindowLayout() }},
		{"view.resizeUp", "向上拉伸工具窗口", "视图", "Ctrl+Alt+Up", "resize stretch tool window taller", func(m *Model) { m.ResizeBottomToolWindow("up") }},
		{"view.resizeDown", "向下拉伸工具窗口", "视图", "Ctrl+Alt+Down", "resize stretch tool window shorter", func(m *Model) { m.ResizeBottomToolWindow("down") }},
		{"view.resizeLeft", "向左拉伸工具窗口", "视图", "Ctrl+Alt+Left", "resize stretch tool window left", func(m *Model) { m.ResizeBottomToolWindow("left") }},
		{"view.resizeRight", "向右拉伸工具窗口", "视图", "Ctrl+Alt+Right", "resize stretch tool window right", func(m *Model) { m.ResizeBottomToolWindow("right") }},
		{"view.moveSplit", "移动到底部右侧", "视图", "", "move tool window split bottom right", func(m *Model) { m.MoveActiveToolWindowToSplit() }},
		{"view.movePrimary", "移动到底部", "视图", "", "move tool window primary bottom", func(m *Model) { m.MoveActiveToolWindowToBottom() }},
		{"view.commit", "提交窗口", "视图", "Cmd+0", "commit tool window git", func(m *Model) { m.OpenCommit() }},
		{"problems.next", "下一个问题", "视图", "Cmd+Alt+Down", "problems next occurrence navigate", func(m *Model) { m.StepProblem(1) }},
		{"problems.prev", "上一个问题", "视图", "Cmd+Alt+Up", "problems previous occurrence navigate", func(m *Model) { m.StepProblem(-1) }},
		{"view.database", "数据库…", "视图", "", "database sql sqlite postgres mysql query", func(m *Model) { m.OpenDatabase() }},
		{"code.intentions", "意图操作…", "代码", "Alt+Enter", "intention action quickfix", func(m *Model) { m.ShowIntentions() }},
		{"code.symbols", "转到符号…", "代码", "Cmd+Shift+O", "go to symbol navigate", func(m *Model) { m.OpenSymbols() }},
		{"code.usages", "查找用法", "代码", "Alt+F7", "find usages references", func(m *Model) { m.FindUsages() }},
		{"code.showUsages", "显示用法", "代码", "", "show usages results window", func(m *Model) { m.showUsages() }},
		{"bookmarks.toggle", "切换书签", "导航", "F3", "bookmark toggle line mark", func(m *Model) { m.ToggleBookmark() }},
		{"bookmarks.show", "显示书签", "导航", "Cmd+F3", "bookmarks show popup list", func(m *Model) { m.ShowBookmarks() }},
		{"bookmarks.next", "下一个书签", "导航", "", "next bookmark navigate", func(m *Model) { m.NextBookmark() }},
		{"bookmarks.prev", "上一个书签", "导航", "", "previous bookmark navigate", func(m *Model) { m.PrevBookmark() }},
		{"bookmarks.mnemonic", "添加助记符书签…", "导航", "Alt+F3", "bookmark mnemonic key", func(m *Model) { m.OpenBookmarkMnemonics() }},
		{"bookmarks.window", "书签工具窗口", "导航", "Cmd+2", "bookmarks tool window", func(m *Model) { m.ToggleBookmarksWindow() }},
		{"bookmarks.file", "切换文件书签", "导航", "", "file bookmark", func(m *Model) { m.ToggleFileBookmark() }},
		{"bookmarks.newList", "新建书签列表…", "导航", "", "new bookmark list", func(m *Model) { m.OpenNewBookmarkList() }},
		{"code.rename", "重命名符号", "重构", "Shift+F6", "rename symbol refactor", func(m *Model) { m.RenameActive() }},
		{"code.undoRefactor", "撤销重构", "重构", "Cmd+Alt+Z", "undo refactor", func(m *Model) { m.UndoRefactor() }},
		{"code.redoRefactor", "重做重构", "重构", "Cmd+Alt+Shift+Z", "redo refactor", func(m *Model) { m.RedoRefactor() }},
		{"code.format", "格式化代码", "代码", "Cmd+Alt+L", "format code", func(m *Model) { m.FormatActive() }},
		{"code.commentLine", "注释行", "代码", "Cmd+/", "comment line toggle", func(m *Model) { m.runEditorCommand(editor.ToggleComment) }},
		{"code.commentBlock", "块注释", "代码", "Cmd+Alt+/", "comment block toggle", func(m *Model) { m.runEditorCommand(editor.ToggleBlockComment) }},
		{"code.duplicateLine", "复制行或选区", "代码", "Cmd+D", "duplicate line selection", func(m *Model) { m.runEditorCommand(editor.DuplicateSelection) }},
		{"code.deleteLine", "删除行", "代码", "Cmd+Backspace", "delete line", func(m *Model) { m.runEditorCommand(editor.DeleteLine) }},
		{"code.moveLineUp", "上移行", "代码", "Alt+Shift+Up", "move line up", func(m *Model) { m.runEditorCommand(editor.MoveLinesUp) }},
		{"code.moveLineDown", "下移行", "代码", "Alt+Shift+Down", "move line down", func(m *Model) { m.runEditorCommand(editor.MoveLinesDown) }},
		{"code.moveStatementUp", "上移语句", "代码", "Cmd+Shift+Up", "move statement up", func(m *Model) { m.runEditorCommand(editor.MoveLinesUp) }},
		{"code.moveStatementDown", "下移语句", "代码", "Cmd+Shift+Down", "move statement down", func(m *Model) { m.runEditorCommand(editor.MoveLinesDown) }},
		{"code.joinLines", "合并行", "代码", "Ctrl+Shift+J", "join lines", func(m *Model) { m.runEditorCommand(editor.JoinLines) }},
		{"run.start", "运行", "运行", "F5", "run main class", func(m *Model) { m.Run() }},
		{"run.debug", "调试", "运行", "F9", "debug session", func(m *Model) { m.Debug() }},
		{"run.stop", "停止", "运行", "Shift+F5", "stop terminate", func(m *Model) { m.StopRun(); m.StopDebug() }},
		{"run.breakpoint", "切换断点", "运行", "Cmd+F8", "toggle breakpoint", func(m *Model) { m.ToggleBreakpoint() }},
		{"run.stepOver", "步过", "运行", "F8", "step over", func(m *Model) { m.StepOver() }},
		{"run.stepInto", "步入", "运行", "F7", "step into", func(m *Model) { m.StepInto() }},
		{"run.stepOut", "步出", "运行", "Shift+F8", "step out", func(m *Model) { m.StepOut() }},
		{"run.continue", "继续", "运行", "", "continue resume", func(m *Model) { m.DebugContinue() }},
		{"run.pause", "暂停", "运行", "", "pause suspend", func(m *Model) { m.DebugPause() }},
		{"run.trust", "信任此项目并启用运行", "运行", "", "trust project", func(m *Model) { m.TrustProject() }},
		{"run.chooseConfig", "选择运行配置…", "运行", "", "select run debug configuration", func(m *Model) { m.ChooseRunConfig() }},
		{"run.reloadConfigs", "重新加载运行配置", "运行", "", "reload run configurations", func(m *Model) { m.ReloadRunConfigs() }},
		{"run.configNext", "下一个运行配置", "运行", "", "next run configuration", func(m *Model) { m.CycleRunConfig(1) }},
		{"run.configPrev", "上一个运行配置", "运行", "", "previous run configuration", func(m *Model) { m.CycleRunConfig(-1) }},
		{"build.buildProject", "构建项目", "构建", "Cmd+F9", "build project compile make mvn gradle cmake", func(m *Model) { m.BuildProject() }},
		{"build.buildModule", "构建模块", "构建", "Cmd+Shift+F9", "build module compile file", func(m *Model) { m.BuildModule() }},
		{"build.rebuildProject", "重新构建项目", "构建", "", "rebuild clean build", func(m *Model) { m.RebuildProject() }},
		{"build.buildArtifacts", "构建工件", "构建", "", "artifacts package assemble jar release", func(m *Model) { m.BuildArtifacts() }},
		{"build.stop", "停止构建", "构建", "", "stop cancel build", func(m *Model) { m.StopBuild() }},
		{"view.build", "显示构建窗口", "视图", "", "build output console", func(m *Model) { m.ShowBuild() }},
		{"java.tools", "安装 Java 语言服务与调试器", "运行", "", "java jdtls language server debugger install toolchain", func(m *Model) { m.InstallJavaToolchain() }},
		{"file.settings", "Settings…", "文件", "Cmd+,", "settings preferences theme font appearance", func(m *Model) { m.OpenSettings() }},
		{"file.invalidateCaches", "Invalidate Caches…", "文件", "", "invalidate caches clear rebuild index restart repair", func(m *Model) { m.OpenInvalidateCaches() }},
		{"app.restart", "Restart IDE", "文件", "", "restart relaunch restart ide", func(m *Model) { m.RestartIDE() }},
		{"view.toggleTheme", "切换深色/浅色主题", "视图", "", "theme dark light appearance toggle", func(m *Model) { m.ToggleTheme() }},
		{"app.themeLight", "浅色", "视图", "", "theme light appearance", func(m *Model) { m.SetThemePref("light") }},
		{"app.themeDark", "深色", "视图", "", "theme dark appearance", func(m *Model) { m.SetThemePref("dark") }},
		{"app.themeSystem", "跟随系统设置", "视图", "", "theme system appearance", func(m *Model) { m.SetThemePref("system") }},
		{"editor.toggleZen", "Toggle Distraction Free Mode", "视图", "", "distraction free mode zen presentation", func(m *Model) { m.ToggleZen() }},
		{"window.toggleFullScreen", "Toggle Full Screen", "视图", "Cmd+Ctrl+F", "fullscreen window display", func(m *Model) { m.ToggleFullScreen() }},
		{"settings.appearance", "Appearance…", "视图", "", "settings appearance theme font", func(m *Model) { m.OpenSettingsPage("appearance") }},
		{"settings.keymap", "Keymap…", "视图", "", "settings keymap shortcuts bindings", func(m *Model) { m.OpenSettingsPage("keymap") }},
		{"toolbar.customize", "Customize Main Toolbar…", "视图", "", "toolbar widgets title bar customize", func(m *Model) { m.OpenToolbarCustomize() }},
		{"toolbar.reset", "Restore Default Toolbar", "视图", "", "toolbar default widgets restore", func(m *Model) { m.RestoreDefaultToolbar() }},
		{"app.checkForUpdates", "Check for Updates…", "帮助", "", "update upgrade version release", func(m *Model) { m.CheckForUpdates() }},
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
			ui.Text(c, "命令").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &pal.query).AutoFocus()
			in.OnChange(func() { pal.refresh() })
			in.OnShortcut(0, ui.KeyDown, func() { m.stepCommand(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepCommand(-1) })
			if in.Submitted() {
				m.runSelectedCommand()
			}
			hint := "输入命令名，Enter 执行"
			switch {
			case strings.TrimSpace(pal.query) != "" && len(pal.items) == 0:
				hint = "没有匹配的命令"
			case strings.TrimSpace(pal.query) != "":
				hint = fmt.Sprintf("%d 个命令", len(pal.items))
			}
			ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
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
							ui.Text(c, cmd.group).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
							if cmd.shortcut != "" {
								ui.Text(c, cmd.shortcut).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
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
			out["error"] = "命令面板没有打开"
			return
		}
		out["all"] = len(m.palette.items)
		m.palette.query = "git"
		m.palette.refresh()
		out["git"] = len(m.palette.items)
		if len(m.palette.items) > 0 {
			out["first"] = m.palette.items[0].title
		}
		m.palette.query = "显示 Git 更改"
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
