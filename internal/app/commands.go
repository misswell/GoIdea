package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
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
	return append([]appCommand{
		{"app.about", "About GoIDEA", "Help", "", "about version platform build", func(m *Model) { m.OpenAbout() }},
		{"app.quit", "Quit GoIDEA", "File", "Cmd+Q", "quit exit close application", func(m *Model) { m.Quit() }},
		{"app.installCli", "Install 'goidea' Command", "Tools", "", "cli shell terminal path command line launcher", func(m *Model) { m.InstallCLI() }},
		{"app.uninstallCli", "Uninstall 'goidea' Command", "Tools", "", "cli shell terminal path command line launcher", func(m *Model) { m.UninstallCLI() }},
		{"help.keymapReference", "Keyboard Reference", "Help", "", "shortcuts keyboard keymap cheat sheet", func(m *Model) { m.OpenKeymapReference() }},
		{"help.showLog", "Show Log in Finder", "Help", "", "log logs diagnose open reveal", func(m *Model) { m.ShowAppLog() }},
		{"file.newProject", "New Project…", "File", "", "new project create", func(m *Model) { m.NewProject() }},
		{"file.new", "New…", "File", "Cmd+N", "new file project view create", func(m *Model) { m.NewFileAtProjectSelection() }},
		{"file.newScratchFile", "New Scratch File…", "File", "Cmd+Shift+N", "scratch temporary buffer new file language", func(m *Model) { m.OpenScratchChooser() }},
		{"file.openProject", "Open…", "File", "Cmd+Shift+Alt+O", "open project folder", func(m *Model) { m.ChooseProject() }},
		{"file.openFile", "Open File…", "File", "", "file light edit open document", func(m *Model) { m.OpenFile() }},
		{"file.openPaths", "Open Paths", "File", "", "open files folders paths drop", func(*Model) {}},
		{"file.openProjectPath", "Open Project Path", "File", "", "open project folder path recent", func(*Model) {}},
		{"file.openRecentProject", "Open Recent Project", "File", "", "open recent project folder", func(*Model) {}},
		{"file.openRecent", "Open Recent…", "File", "", "recent projects welcome", func(m *Model) { m.showRecentProjectsPopup() }},
		{"welcome.recentProjects", "Recent Projects…", "File", "", "recent projects open folder", func(m *Model) { m.showRecentProjectsPopup() }},
		{"file.projectStructure", "Project Structure…", "File", "Cmd+;", "project structure sdk jdk modules libraries facets artifacts sources dependencies", func(m *Model) { m.OpenProjectStructure("") }},
		{"project.attach", "Attach Project…", "File", "", "attach project workspace linked folder", func(m *Model) { m.AttachProject() }},
		{"project.detach", "Detach Project", "File", "", "detach remove project attachment workspace", func(*Model) {}},
		{"workspace.create", "New Workspace…", "File", "", "new workspace linked projects", func(m *Model) { m.OpenNewWorkspace() }},
		{"workspace.open", "Open Workspace…", "File", "", "open workspace linked projects", func(m *Model) { m.OpenWorkspace() }},
		{"workspace.manage", "Manage Workspace…", "File", "", "manage workspace linked projects rename remove", func(m *Model) { m.OpenManageWorkspace() }},
		{"workspace.removeProject", "Remove from Workspace", "File", "", "remove project workspace linked folder", func(*Model) {}},
		{"project.close", "Close Project", "File", "", "close project welcome", func(m *Model) { m.CloseProject() }},
		{"welcome.manageProjects", "Manage Projects…", "File", "", "manage projects welcome recent", func(m *Model) { m.showWelcome() }},
		{"navigate.searchEverywhere", "Search Everywhere", "Navigate", "Shift Shift", "search everywhere find class file symbol action text", func(m *Model) { m.OpenSearchEverywhere() }},
		{"navigate.gotoClass", "Go to Class…", "Navigate", "Cmd+O", "class type", func(m *Model) { m.openSearchEverywhereAt(seTabClasses) }},
		{"navigate.gotoFile", "Go to File…", "Navigate", "Cmd+Shift+O", "quick open file", func(m *Model) { m.openSearchEverywhereAt(seTabFiles) }},
		{"navigate.gotoSymbol", "Go to Symbol…", "Navigate", "Cmd+Alt+O", "function method field", func(m *Model) { m.openSearchEverywhereAt(seTabSymbols) }},
		{"navigate.findAction", "Find Action…", "Help", "Cmd+Shift+A", "command palette search", func(m *Model) { m.openSearchEverywhereAt(seTabActions) }},
		{"navigate.gotoLine", "Go to Line:Column…", "Navigate", "Cmd+L", "go to line column caret", func(m *Model) { m.OpenGotoLine() }},
		{"navigate.back", "Back", "Navigate", "Cmd+[ Cmd+Alt+Left", "previous navigation history location", func(m *Model) { m.NavigateBack() }},
		{"navigate.forward", "Forward", "Navigate", "Cmd+] Cmd+Alt+Right", "next navigation history location", func(m *Model) { m.NavigateForward() }},
		{"navigate.lastEditLocation", "Last Edit Location", "Navigate", "Cmd+Shift+Backspace", "previous edit changed location history", func(m *Model) { m.LastEditLocation() }},
		{"navigate.nextEditLocation", "Next Edit Location", "Navigate", "", "next edit changed location history", func(m *Model) { m.NextEditLocation() }},
		{"navigate.recentFiles", "Recent Files", "Navigate", "Cmd+E", "recently opened edited files switcher", func(m *Model) { m.openRecentNavigation(false) }},
		{"navigate.recentLocations", "Recent Locations", "Navigate", "Cmd+Shift+E", "recently changed locations", func(m *Model) { m.openRecentNavigation(true) }},
		{"analyze.stackTrace", "Analyze Stack Trace or Thread Dump…", "Code", "", "exception console source line thread dump", func(m *Model) { m.OpenStackTrace() }},
		{"code.quickDocumentation", "Quick Documentation", "Code", "F1 Ctrl+J", "javadoc hover symbol help", func(m *Model) { m.QuickDocumentation() }},
		{"code.quickDefinition", "Quick Definition", "Code", "Alt+Space Cmd+Y", "declaration source peek definition", func(m *Model) { m.QuickDefinition("definition") }},
		{"code.quickTypeDefinition", "Quick Type Definition", "Code", "", "type declaration source peek definition", func(m *Model) { m.QuickDefinition("typeDefinition") }},
		{"toolwindow.documentation", "Documentation", "View", "", "documentation javadoc window", func(m *Model) { m.DockToggle(documentationWindow) }},
		{"toolwindow.coverage", "Coverage", "View", "", "test coverage lines branches", func(m *Model) { m.DockToggle("coverage") }},
		{"agents.showToolWindow", "Agents", "View", "", "agent tasks claude codex gemini acp", func(m *Model) { m.OpenAgentsWindow() }},
		{"agents.newTask", "New Agent Task", "Agents", "", "agent new task claude codex gemini prompt", func(m *Model) { m.NewAgentTaskOpen() }},
		{"agents.findInConversation", "Find in Conversation", "Agents", "Cmd+F", "agent conversation search find", func(m *Model) { m.FindAgentConversation() }},
		{"agents.closeTask", "Close Agent Task", "Agents", "", "agent close conversation task", func(m *Model) { m.CloseAgentTask() }},
		{"agents.closeAndNewTask", "Close and New Agent Task", "Agents", "", "agent close new task", func(m *Model) { m.CloseAndNewAgentTask() }},
		{"agents.sendToAgent", "Send to Agent", "Agents", "", "agent send selection file context", func(m *Model) { m.SendEditorToAgent() }},
		{"agents.externalSessions", "Agent Sessions…", "Agents", "", "agent history external resume import session", func(m *Model) { m.OpenAgentExternalSessions() }},
		{"agents.addAgent", "More Agents…", "Agents", "", "agent acp registry install update coding agent", func(m *Model) { m.OpenSettingsPage(settingsPageAgentRegistry) }},
		{"agents.mcpServers", "External MCP Servers…", "Agents", "", "mcp model context protocol servers tools agent", func(m *Model) { m.OpenSettingsPage(settingsPageAgentMCP) }},
		{"agents.openSampleConversations", "Open Sample Conversations", "Agents", "", "agent sample performance benchmark conversation", func(m *Model) { m.OpenAgentSampleConversations(2, 150) }},
		{"http.newRequest", "New HTTP Request", "Tools", "", "new http request rest scratch curl", func(m *Model) { m.NewHTTPRequest("") }},
		{"http.runRequest", "Run HTTP Request at Caret", "Tools", "Cmd+Enter", "send execute request http rest", func(m *Model) { m.RunHTTPRequestAtCaret() }},
		{"http.copyAsCurl", "Convert to cURL and Copy", "Tools", "", "copy curl http request export", func(m *Model) { m.CopyHTTPRequestAsCurl() }},
		{"http.openEndpoint", "Open in HTTP Client", "Tools", "", "spring endpoint route open generate http", func(m *Model) {}},
		{"http.addPublicEnvironment", "Add Environment to Public File", "Tools", "", "http client environment env public", func(m *Model) { m.OpenHTTPEnvironmentFile(false) }},
		{"http.addPrivateEnvironment", "Add Environment to Private File", "Tools", "", "http client environment env secrets private", func(m *Model) { m.OpenHTTPEnvironmentFile(true) }},
		{"http.runAll", "Run All HTTP Requests in File", "Tools", "", "run all requests send http rest", func(m *Model) { m.RunAllHTTPRequests() }},
		{"http.generateSpringRequest", "Generate Request in HTTP Client", "Tools", "", "spring request mapping rest endpoint http client", func(m *Model) { m.GenerateSpringRequest() }},
		{"http.generateSpringControllerRequests", "Generate Requests for Controller", "Tools", "", "spring controller request mappings rest endpoints http client", func(m *Model) { m.GenerateSpringControllerRequests() }},
		{"http.showToolWindow", "HTTP Client", "Tools", "", "http responses history environment rest", func(m *Model) { m.showHTTPClient() }},
		{"graphql.showToolWindow", "GraphQL", "Tools", "", "schema endpoints graphql-config", func(m *Model) { m.ShowGraphQL() }},
		{"view.profiler", "Profiler", "View", "", "profile flame graph call tree method list cpu memory", func(m *Model) { m.DockShow("profiler") }},
		{"view.logcat", "Logcat", "Tools", "", "android adb device log stream filter pid tag level", func(m *Model) { m.OpenLogcat() }},
		{"graphql.addConfiguration", "Add Schema Configuration", "Tools", "", "graphql graphql-config graphqlrc schema", func(m *Model) { m.AddGraphQLConfiguration() }},
		{"graphql.editConfiguration", "Open Configuration File", "Tools", "", "graphql graphql-config graphqlrc settings", func(m *Model) { m.OpenGraphQLConfiguration() }},
		{"graphql.introspect", "Rerun Introspection Query", "Tools", "", "graphql introspection schema endpoint download", func(m *Model) {
			if tab := m.tab(); tab != nil && tab.doc != nil {
				m.RerunGraphQLIntrospection(tab.doc.Path())
			}
		}},
		{"graphql.restartDiscovery", "Restart Schema Discovery", "Tools", "", "graphql schema refresh reload", func(m *Model) { m.RestartGraphQLDiscovery() }},
		{"graphql.runQuery", "Execute GraphQL", "Run", "Cmd+Enter", "graphql run execute query endpoint", func(m *Model) { m.RunGraphQLAtCaret() }},
		{"run.coverage", "Run with Coverage", "Run", "", "test code coverage", func(m *Model) { m.RunWithCoverage() }},
		{"run.profile", "Profile", "Run", "", "profiler flame graph cpu", func(m *Model) {
			m.RunWithProfiler(openidea.ProfileCPU)
		}},
		{"run.profileAllocations", "Profile Allocations", "Run", "", "allocation memory async-profiler", func(m *Model) {
			m.RunWithProfiler(openidea.ProfileAllocations)
		}},
		{"run.profileLiveObjects", "Profile Live Objects", "Run", "", "allocation live objects memory leak", func(m *Model) {
			m.RunWithProfiler(openidea.ProfileLiveObjects)
		}},
		{"run.profileContext", "Profile Context Configuration", "Run", "", "profile active file context", func(m *Model) {
			m.RunContextConfigurationWithProfiler()
		}},
		{"coverage.importReport", "Import a Coverage Report…", "Run", "", "coverage report lcov jacoco cobertura", func(m *Model) { m.ImportCoverageReport() }},
		{"coverage.manageReports", "Manage Coverage Reports…", "Run", "", "coverage reports suites", func(m *Model) { m.ManageCoverageReports() }},
		{"coverage.show", "Show Coverage", "Run", "", "coverage gutter", func(m *Model) { m.SetCoverageVisible(true) }},
		{"coverage.hide", "Hide Coverage", "Run", "", "coverage gutter", func(m *Model) { m.SetCoverageVisible(false) }},
		{"app.commandPalette", "Command Palette…", "Navigate", "Cmd+Shift+P", "find action command palette search", func(m *Model) { m.OpenCommands() }},
		{"runAnything.show", "Run Anything…", "Run", "", "command shell execute npm mvn gradle goal", func(m *Model) { m.OpenRunAnything() }},
		{"plugins.settings", "Plugins…", "File", "", "plugins settings marketplace", func(m *Model) { m.openPlugins() }},
		{"file.quickOpen", "Go to File…", "File", "Cmd+P", "quick open file goto recent", func(m *Model) { m.OpenQuickFiles() }},
		{"file.save", "Save", "File", "Cmd+S", "save write", func(m *Model) { m.SaveActive() }},
		{"file.close", "Close Tab", "File", "Cmd+W", "close tab", func(m *Model) { m.closeTab(m.active) }},
		{"edit.undo", "Undo", "Edit", "Cmd+Z", "undo history", func(m *Model) {
			if m.projectTreeFocus {
				m.UndoLastProjectFileOperation()
				return
			}
			if m.undoNotebook() {
				return
			}
			m.runEditorCommand(editor.Undo)
		}},
		{"edit.redo", "Redo", "Edit", "Cmd+Shift+Z", "redo history", func(m *Model) {
			if m.redoNotebook() {
				return
			}
			m.runEditorCommand(editor.Redo)
		}},
		{"edit.cut", "Cut", "Edit", "Cmd+X", "cut selection clipboard", func(m *Model) {
			if m.projectTreeFocus {
				m.CutSelectedTreeEntry()
				return
			}
			m.rememberActiveEditorClipboard()
			m.runEditorCommand(editor.Cut)
		}},
		{"edit.copy", "Copy", "Edit", "Cmd+C", "copy selection clipboard", func(m *Model) {
			if m.projectTreeFocus {
				m.CopySelectedTreeEntry()
				return
			}
			m.rememberActiveEditorClipboard()
			m.runEditorCommand(editor.Copy)
		}},
		{"edit.paste", "Paste", "Edit", "Cmd+V", "paste clipboard", func(m *Model) {
			if m.projectTreeFocus {
				m.PasteIntoTreeSelection()
				return
			}
			m.runEditorCommand(editor.Paste)
		}},
		{"edit.selectAll", "Select All", "Edit", "Cmd+A", "select all editor", func(m *Model) {
			if m.projectTreeFocus && m.tree != nil {
				m.tree.SelectAll()
				return
			}
			m.runEditorCommand(editor.SelectAll)
		}},
		{"editor.copyPath", "Absolute Path", "Edit", "", "copy path clipboard editor", func(m *Model) { m.CopyActiveEditorPath() }},
		{"editor.copyPathReference", "Copy Path/Reference…", "Edit", "Cmd+Shift+C", "copy absolute relative source line reference chooser", func(m *Model) { m.OpenCopyPathReferencePopup() }},
		{"editor.copyPathWithLineNumber", "Copy Path with Line Number", "Edit", "", "copy relative path line number", func(m *Model) { m.CopyActiveEditorPathWithLineNumber() }},
		{"editor.copyReference", "Copy Reference", "Edit", "Cmd+Alt+Shift+C", "fully qualified name fqn copy path", func(m *Model) { m.CopyActiveEditorReference() }},
		{"macro.startRecording", "Start Macro Recording", "Edit", "", "macro record recording start", func(m *Model) { m.startMacroRecording() }},
		{"macro.stopRecording", "Stop Macro Recording", "Edit", "", "macro record recording stop", func(m *Model) { m.stopMacroRecording() }},
		{"macro.playBack", "Play Back Last Macro", "Edit", "", "macro replay playback last", func(m *Model) { m.playLastMacro() }},
		{"macro.playSaved", "Play Saved Macros…", "Edit", "", "macro replay playback saved choose", func(m *Model) { m.openPlaySavedMacros() }},
		{"macro.edit", "Edit Macros…", "Edit", "", "macro rename delete saved", func(m *Model) { m.openEditMacros() }},
		{"editor.addOrRemoveCaret", "Add or Remove Caret", "Editor Actions", "Alt+Shift+Button1", "multiple cursor caret pointer", func(m *Model) {}},
		{"code.pasteFromHistory", "Paste from History…", "Edit", "Cmd+Shift+V", "clipboard history paste multiple", func(m *Model) { m.OpenPasteFromHistory() }},
		{"editor.toggleColumnSelection", "Column Selection Mode", "Edit", "Cmd+Shift+8", "block rectangular column selection", func(m *Model) { m.ToggleColumnSelection() }},
		{"editor.toggleSemanticHighlighting", "Semantic Highlighting", "Editor", "", "rainbow variables identifiers color scheme", func(m *Model) { m.toggleSemanticHighlighting() }},
		{"editor.toggleRainbowBrackets", "Rainbow Brackets", "Editor", "", "bracket pair colorization rainbow parentheses color scheme", func(m *Model) { m.toggleRainbowBrackets() }},
		{"editor.colorScheme.import", "Import Scheme", "Editor", "", "icls xml color scheme import", func(m *Model) { m.ImportColorScheme(nil) }},
		{"editor.colorScheme.export", "Export Scheme", "Editor", "", "icls color scheme export", func(m *Model) { m.ExportColorScheme(nil) }},
		{"editor.colorScheme.delete", "Delete Scheme", "Editor", "", "icls custom color scheme delete", func(m *Model) { m.DeleteColorScheme(nil) }},
		{"editor.inlineCompletion.trigger", "Call Inline Completion", "Code", "Alt+\\", "ghost text inline suggestion", func(m *Model) { m.TriggerInlineCompletion() }},
		{"find.inFile", "Find", "Find", "Cmd+F", "find search editor current file", func(m *Model) { m.runEditorCommand(editor.Find) }},
		{"find.replaceInFile", "Replace", "Find", "Cmd+R", "replace editor current file", func(m *Model) { m.runEditorCommand(editor.FindReplace) }},
		{"find.next", "Find Next Occurrence", "Find", "Cmd+G", "find next search occurrence", func(m *Model) { m.runEditorCommand(editor.FindNext) }},
		{"find.previous", "Find Previous Occurrence", "Find", "Cmd+Shift+G", "find previous search occurrence", func(m *Model) { m.runEditorCommand(editor.FindPrevious) }},
		{"file.reloadAllFromDisk", "Reload All from Disk", "File", "", "reload all from disk synchronize refresh external", func(m *Model) { m.ReloadAllFromDisk() }},
		{"file.toggleReadOnly", "Toggle Read-only Attribute", "File", "", "make file writable read-only lock", func(m *Model) { m.ToggleActiveFileReadOnly() }},
		{"editor.pinTab", "Pin/Unpin Tab", "File", "", "pin unpin tab", func(m *Model) { m.PinActiveTab() }},
		{"editor.closeOthers", "Close Other Tabs", "File", "", "close other tabs", func(m *Model) { m.CloseOtherTabs(nil) }},
		{"editor.closeAll", "Close All Tabs", "File", "", "close all tabs", func(m *Model) { m.CloseAllTabs(false) }},
		{"editor.closeAllPinned", "Close Unpinned Tabs", "File", "", "close all but pinned tabs", func(m *Model) { m.CloseAllTabs(true) }},
		{"editor.reopenClosedTab", "Reopen Closed Tab", "File", "", "reopen closed tab history restore", func(m *Model) { m.ReopenClosedTab() }},
		{"editor.nextTab", "Select Next Tab", "Window", "Ctrl+Right Cmd+Shift+]", "next tab cycle editor", func(m *Model) { m.cycleEditorTab(1) }},
		{"editor.prevTab", "Select Previous Tab", "Window", "Ctrl+Left Cmd+Shift+[", "previous tab cycle editor", func(m *Model) { m.cycleEditorTab(-1) }},
		{"editor.toggleSoftWrap", "Soft-Wrap", "View", "", "word wrap line wrap", func(m *Model) { m.toggleActiveEditorSoftWrap() }},
		{"editor.toggleRenderedDocs", "Render Documentation Comments", "View", "", "javadoc kdoc jsdoc rustdoc comments reader mode", func(m *Model) { m.toggleActiveEditorRenderedDocs() }},
		{"editor.resetFontSize", "Reset Font Size in All Editors", "View", "", "zoom font size reset", func(m *Model) { m.resetEditorFontZoom() }},
		{"editor.increaseFontSize", "Increase Font Size", "View", "Alt+Shift+.", "zoom font size increase", func(m *Model) { m.stepEditorFontZoom(1) }},
		{"editor.decreaseFontSize", "Decrease Font Size", "View", "Alt+Shift+,", "zoom font size decrease", func(m *Model) { m.stepEditorFontZoom(-1) }},
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
		{"editor.inlayHintsSettings", "Configure Code Vision…", "View", "", "code vision inlay hints usages author settings", func(m *Model) { m.OpenSettingsPage(settingsPageInlayHints) }},
		{"view.notifications", "Notifications", "View", "", "notifications balloon history alerts unread", func(m *Model) { m.ToggleNotificationsWindow() }},
		{"view.maven", "Maven", "View", "", "maven tool window modules reactor", func(m *Model) { m.ToggleMaven() }},
		{"view.gradle", "Gradle", "View", "", "gradle tool window projects tasks subprojects", func(m *Model) { m.ToggleGradle() }},
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
		{"vcs.newBranch", "New Branch…", "Git", "", "git vcs branch create checkout", func(m *Model) { m.OpenNewGitBranch() }},
		{"vcs.newTag", "New Tag…", "Git", "", "git vcs tag create revision", func(m *Model) { m.OpenNewGitTag() }},
		{"vcs.operations", "VCS Operations…", "Git", "Ctrl+V", "git quick list branch commit fetch pull push merge rebase stash reset", func(m *Model) { m.OpenVCSOperations() }},
		{"vcs.merge", "Merge…", "Git", "", "git vcs merge branch tag", func(m *Model) { m.OpenGitMerge() }},
		{"vcs.rebase", "Rebase…", "Git", "", "git vcs rebase branch onto", func(m *Model) { m.OpenGitRebase() }},
		{"vcs.fetch", "Fetch", "Git", "", "git fetch remote download", func(m *Model) { m.GitFetch() }},
		{"vcs.pull", "Pull…", "Git", "", "git pull update project upstream", func(m *Model) { m.OpenGitPull() }},
		{"vcs.update", "Update Project…", "Git", "", "git update project pull fetch", func(m *Model) { m.OpenGitPull() }},
		{"vcs.manageRemotes", "Manage Remotes…", "Git", "", "git remote origin url add edit remove rename", func(m *Model) { m.OpenGitRemotes() }},
		{"vcs.push", "Push…", "Git", "", "git push remote upstream", func(m *Model) { m.OpenGitPush() }},
		{"vcs.stash", "Stash Changes…", "Git", "", "git stash shelve local changes", func(m *Model) { m.OpenGitStash() }},
		{"vcs.unstash", "Unstash Changes…", "Git", "", "git unstash apply pop shelf", func(m *Model) { m.OpenGitUnstash() }},
		{"vcs.resetHead", "Reset HEAD…", "Git", "", "git reset current branch revision", func(m *Model) { m.OpenGitReset() }},
		{"vcs.rebase.continue", "Continue Rebase", "Git", "", "git rebase continue", func(m *Model) { m.ContinueGitOperation() }},
		{"vcs.rebase.abort", "Abort Rebase", "Git", "", "git rebase abort", func(m *Model) { m.AbortGitOperation() }},
		{"vcs.rebase.skip", "Skip Commit", "Git", "", "git rebase skip", func(m *Model) { m.SkipGitOperation() }},
		{"vcs.merge.abort", "Abort Merge", "Git", "", "git merge abort", func(m *Model) { m.AbortGitOperation() }},
		{"vcs.cherryPick.continue", "Continue Cherry-Pick", "Git", "", "git cherry pick continue", func(m *Model) { m.ContinueGitOperation() }},
		{"vcs.cherryPick.abort", "Abort Cherry-Pick", "Git", "", "git cherry pick abort", func(m *Model) { m.AbortGitOperation() }},
		{"vcs.revert.continue", "Continue Revert", "Git", "", "git revert continue", func(m *Model) { m.ContinueGitOperation() }},
		{"vcs.revert.abort", "Abort Revert", "Git", "", "git revert abort", func(m *Model) { m.AbortGitOperation() }},
		{"vcs.commit", "Commit…", "Git", "Cmd+K", "git commit message stage", func(m *Model) { m.OpenCommit() }},
		{"vcs.log.cherryPick", "Cherry-Pick", "Git", "", "git log commit cherry pick apply", func(m *Model) { m.CherryPickSelectedCommit() }},
		{"vcs.log.revert", "Revert Commit", "Git", "", "git log commit revert inverse", func(m *Model) { m.RevertSelectedCommit() }},
		{"vcs.diff", "Compare with HEAD", "Git", "Cmd+Shift+D", "git diff compare head", func(m *Model) { m.CompareActive() }},
		{"search.project", "Find in Files", "Search", "Cmd+Shift+F", "search find in project", func(m *Model) { m.ShowSearch() }},
		{"view.find", "Find", "Search", "Cmd+3", "find search tool window", func(m *Model) { m.ShowSearch() }},
		{"project.findInFolder", "Find in Folder…", "Search", "", "find in folder search directory scope selected", func(m *Model) { m.FindInFolderSelected() }},
		{"view.project", "Show Project", "View", "Cmd+1", "project tree files sidebar", func(m *Model) { m.ShowProject() }},
		{"project.expandAll", "Expand All", "Navigate", "Cmd+=", "expand all project tree folders", func(m *Model) { m.ExpandProjectAll() }},
		{"project.collapseAll", "Collapse All", "Navigate", "Cmd+-", "collapse all project tree folders", func(m *Model) { m.CollapseProjectAll() }},
		{"project.newFile", "New File", "File", "", "create new file project view", func(m *Model) { m.NewFileAtProjectSelection() }},
		{"project.newFolder", "New Directory", "File", "", "create new folder directory project view", func(m *Model) { m.NewDirectoryAtProjectSelection() }},
		{"project.newInThisDirectory", "New in This Directory…", "File", "Cmd+Alt+N", "create file same place current directory", func(m *Model) { m.NewInThisDirectory() }},
		{"project.copy", "Copy", "Project View", "", "copy file folder project view", func(m *Model) { m.CopySelectedTreeEntry() }},
		{"project.cut", "Cut", "Project View", "", "cut move file folder project view", func(m *Model) { m.CutSelectedTreeEntry() }},
		{"project.paste", "Paste", "Project View", "", "paste copy move file folder project view", func(m *Model) { m.PasteIntoTreeSelection() }},
		{"project.copyElement", "Copy…", "Project View", "F5", "copy duplicate clone file folder project view", func(m *Model) { m.CopySelectedTreeElement() }},
		{"project.copyAbsolutePath", "Copy Absolute Path", "Project View", "", "path clipboard absolute", func(m *Model) { m.CopySelectedTreePath("absolute") }},
		{"project.copyRelativePath", "Copy Path From Content Root", "Project View", "", "path clipboard relative content root", func(m *Model) { m.CopySelectedTreePath("relative") }},
		{"project.copyFileName", "Copy File Name", "Project View", "", "path clipboard name file", func(m *Model) { m.CopySelectedTreePath("name") }},
		{"project.copySourceRootPath", "Copy Path From Source Root", "Project View", "", "path clipboard source root", func(m *Model) { m.CopySelectedTreePath("source") }},
		{"project.copyPath", "Copy Path", "Project View", "", "path clipboard copy selected", func(m *Model) { m.CopySelectedTreePath("absolute") }},
		{"project.move", "Move File…", "Project View", "F6", "move rename file folder refactor", func(m *Model) { m.MoveSelectedTreeEntry() }},
		{"project.undoMove", "Undo Move / Rename", "Project View", "", "undo move rename refactor", func(m *Model) { m.UndoLastTreeMove() }},
		{"project.rename", "Rename File…", "Project View", "Shift+F6", "rename file folder refactor", func(m *Model) { m.RenameSelectedTreeEntry() }},
		{"project.delete", "Delete…", "Project View", "Cmd+Backspace", "delete remove trash selected file folder project view", func(m *Model) { m.DeleteSelectedTreeEntry() }},
		{"project.refresh", "Reload Project Tree from Disk", "File", "", "reload project tree disk synchronize refresh all", func(m *Model) { m.ReloadProjectTree() }},
		{"project.reloadFromDisk", "Reload from Disk", "File", "Cmd+Alt+Y", "reload selected folder from disk synchronize refresh", func(m *Model) { m.ReloadSelectedFromDisk() }},
		{"project.reveal", "Reveal in Project View", "File", "", "reveal select active file project tree", func(m *Model) { m.SelectInProjectView() }},
		{"project.open", "Jump to Source", "Project View", "F4", "open selected project tree file source", func(m *Model) { m.OpenSelectedTreeEntry() }},
		{"project.revealInFinder", "Reveal in Finder", "File", "", "reveal selected finder explorer folder", func(m *Model) { m.RevealSelectedInFinder() }},
		{"project.renameInline", "Rename…", "File", "", "rename selected tree file folder", func(m *Model) { m.RenameSelectedTreeEntry() }},
		{"project.compareWith", "Compare With…", "File", "", "compare selected file", func(m *Model) { m.CompareSelectedTreeFile() }},
		{"project.findUsages", "Find Usages of File", "File", "", "find usages selected file", func(m *Model) { m.FindUsagesOfSelectedTreeFile() }},
		{"project.reformatCode", "Reformat File", "File", "", "reformat selected source file", func(m *Model) { m.FormatSelectedTreeFile() }},
		{"code.reformatFile", "Reformat File", "Code", "", "reformat selected source file", func(m *Model) { m.FormatSelectedTreeFile() }},
		{"project.optimizeImports", "Optimize Imports in File", "Project View", "", "organize imports selected source file", func(m *Model) { m.OptimizeImportsOfSelectedTreeFile() }},
		{"code.optimizeImportsFile", "Optimize Imports in File", "Code", "", "organize imports selected source file", func(m *Model) { m.OptimizeImportsOfSelectedTreeFile() }},
		{"project.openInTerminal", "Open in Terminal", "Tools", "", "terminal selected folder file", func(m *Model) { m.OpenSelectedTreeInTerminal() }},
		{"project.trust", "Trust Project", "Project", "", "trust allow build run restricted mode", func(m *Model) { m.TrustProject() }},
		{"project.previewRestricted", "Preview in Restricted Mode", "Project", "", "preview restricted untrusted safe", func(m *Model) { m.PreviewProjectRestricted() }},
		{"project.dontOpen", "Don't Open", "Project", "", "reject close untrusted project", func(m *Model) { m.DontOpenProject() }},
		{"project.revokeTrust", "Revoke Project Trust", "Project", "", "revoke trust restricted mode", func(m *Model) { m.RevokeProjectTrust() }},
		{"project.removeTrustedLocation", "Remove Trusted Location…", "File", "", "remove trusted location trust", func(m *Model) { m.RemoveTrustedLocation() }},
		{"project.showHistory", "Show Local History", "File", "", "local history revisions snapshots restore", func(m *Model) { m.ShowHistorySelected() }},
		{"project.putLabel", "Put Label…", "File", "", "local history put label version", func(m *Model) { m.OpenHistoryLabel() }},
		{"navigate.selectInProjectView", "Select in Project View", "Navigate", "Alt+F1", "select in project view reveal active file locate", func(m *Model) { m.SelectInProjectView() }},
		{"navigate.showFilePath", "File Path", "Navigate", "Cmd+Alt+F12", "file path popup copy reveal finder", func(m *Model) { m.ShowSelectedFilePath() }},
		{"navigate.navigationBar", "Jump to Navigation Bar", "Navigate", "Cmd+Up Alt+Home", "breadcrumbs file path project directory symbol", func(m *Model) { m.ShowNavigationBar() }},
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
		{"layout.save", "Save Current Layout as…", "Window", "", "tool window layout store save current", func(m *Model) { m.OpenSaveCurrentLayout() }},
		{"layout.restore", "Restore Tool Window Layout", "Window", "", "tool window layout restore saved apply", func(m *Model) { m.OpenRestoreToolWindowLayout(nil) }},
		{"layout.restoreDefault", "Restore Default Layout", "Window", "Shift+F12", "tool window layout reset default sizes placement view mode", func(m *Model) { m.ResetToolWindowLayout() }},
		{"layout.rename", "Rename Tool Window Layout…", "Window", "", "tool window layout rename saved", func(m *Model) { m.OpenRenameToolWindowLayout() }},
		{"layout.delete", "Delete Tool Window Layout…", "Window", "", "tool window layout delete remove saved", func(m *Model) { m.OpenDeleteToolWindowLayout() }},
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
		{"code.showUsages", "Show Usages", "Code", "Cmd+Alt+F7", "show usages popup references", func(m *Model) { m.ShowUsagesPopup() }},
		{"code.showUsageResults", "Open Usage Results in Find Window", "Code", "", "show already found usages results window", func(m *Model) { m.OpenUsageResultsInFindWindow() }},
		{"code.optimizeImports", "Optimize Imports", "Code", "Cmd+Alt+O", "organize imports", func(m *Model) { m.OptimizeImports() }},
		{"code.fixEslintProblems", "Fix ESLint Problems", "Code", "", "eslint lint fix all", func(m *Model) { m.FixESLintProblems() }},
		{"bookmarks.toggle", "Toggle Bookmark", "Navigate", "F3", "bookmark toggle line mark", func(m *Model) { m.ToggleBookmark() }},
		{"bookmarks.show", "Show Bookmarks", "Navigate", "Cmd+F3", "bookmarks show popup list", func(m *Model) { m.ShowBookmarks() }},
		{"bookmarks.next", "Next Bookmark", "Navigate", "", "next bookmark navigate", func(m *Model) { m.NextBookmark() }},
		{"bookmarks.prev", "Previous Bookmark", "Navigate", "", "previous bookmark navigate", func(m *Model) { m.PrevBookmark() }},
		{"bookmarks.mnemonic", "Add Mnemonic Bookmark…", "Navigate", "Alt+F3", "bookmark mnemonic key", func(m *Model) { m.OpenBookmarkMnemonics() }},
		{"bookmarks.window", "Bookmarks Tool Window", "Navigate", "Cmd+2", "bookmarks tool window", func(m *Model) { m.ToggleBookmarksWindow() }},
		{"bookmarks.file", "Toggle File Bookmark", "Navigate", "", "file bookmark", func(m *Model) { m.ToggleFileBookmark() }},
		{"bookmarks.newList", "New Bookmark List…", "Navigate", "", "new bookmark list", func(m *Model) { m.OpenNewBookmarkList() }},
		{"code.rename", "Rename Symbol", "Refactor", "Shift+F6", "rename symbol refactor", func(m *Model) { m.RenameActive() }},
		{"refactor.safeDelete", "Safe Delete…", "Refactor", "Cmd+Delete", "safe delete remove declaration usages", func(m *Model) { m.SafeDeleteActive() }},
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
		{"run.runContext", "Run Context Configuration", "Run", "Ctrl+Shift+R", "run file test main context active", func(m *Model) { m.RunContextConfiguration() }},
		{"run.coverageContext", "Run Context Configuration with Coverage", "Run", "", "coverage run file test context active", func(m *Model) { m.RunContextConfigurationWithCoverage() }},
		{"run.rerun", "Rerun", "Run", "Cmd+R", "rerun restart process", func(m *Model) { m.Rerun() }},
		{"run.rerunFailed", "Rerun Failed Tests", "Run", "", "rerun failed tests", func(m *Model) { m.RerunFailedTests() }},
		{"debug.rerun", "Rerun Debug", "Run", "", "rerun debug session restart", func(m *Model) { m.DebugRerun() }},
		{"debug.reloadChangedClasses", "Reload Changed Classes", "Run", "", "hotswap hot swap reload changed classes hot code replace", func(m *Model) { m.HotSwapReload() }},
		{"debug.cancelHotSwap", "Cancel Hot Swap Build", "Run", "", "hotswap stop cancel compile build", func(m *Model) { m.CancelHotSwap() }},
		{"debug.hotReload", "Hot Reload", "Run", "", "flutter dart reload sources application", func(m *Model) { m.DebugHotReload() }},
		{"debug.hotRestart", "Hot Restart", "Run", "", "flutter dart restart application discard state", func(m *Model) { m.DebugHotRestart() }},
		{"debug.stop", "Stop Debug Session", "Run", "", "debugger stop terminate", func(m *Model) { m.StopDebug() }},
		{"debug.evaluate", "Evaluate Expression…", "Run", "", "evaluate expression watch debug", func(m *Model) { m.OpenDebugEvaluate() }},
		{"debug.quickEvaluate", "Quick Evaluate Expression", "Run", "", "quick evaluate selection word debug", func(m *Model) { m.QuickEvaluate() }},
		{"debug.setValue", "Set Value…", "Run", "", "set variable value edit debug", func(m *Model) { m.OpenDebugSetValue() }},
		{"debug.jumpToSource", "Jump to Source", "Run", "", "debug frame variable source declaration", func(m *Model) { m.DebugJumpToSource() }},
		{"debug.resetFrame", "Reset Frame", "Run", "", "drop frame restart frame debug", func(m *Model) { m.DebugResetFrame() }},
		{"debug.smartStepInto", "Smart Step Into", "Run", "", "choose call step into debug", func(m *Model) { m.DebugSmartStepInto() }},
		{"debug.browseConsoleHistory", "Browse Console History", "Run", "", "debug console repl history expression", func(m *Model) { m.DebugBrowseConsoleHistory() }},
		{"debug.showExecutionPoint", "Show Execution Point", "Run", "", "debug execution point current frame", func(m *Model) { m.ShowExecutionPoint() }},
		{"run.chooseConfig", "Choose Run/Debug Configuration…", "Run", "Ctrl+Alt+R", "choose run configuration select popup", func(m *Model) { m.OpenRunChooser("run") }},
		{"run.chooseDebugConfig", "Choose Debug Configuration…", "Run", "Ctrl+Alt+D", "choose debug configuration popup", func(m *Model) { m.OpenRunChooser("debug") }},
		{"run.editConfigs", "Edit Configurations…", "Run", "Ctrl+Alt+Shift+R", "edit run debug configurations dialog", func(m *Model) { m.OpenRunConfigs() }},
		{"run.debug", "Debug", "Run", "F9", "debug selected configuration", func(m *Model) { m.Debug() }},
		{"run.debugContext", "Debug Context Configuration", "Run", "Ctrl+Shift+D", "debug context active file", func(m *Model) { m.DebugContext() }},
		{"run.stop", "Stop", "Run", "Shift+F5", "stop terminate", func(m *Model) { m.StopRun(); m.StopDebug() }},
		{"debug.attach", "Attach to Process…", "Run", "Alt+Shift+F5", "attach to process jdwp java", func(m *Model) { m.OpenAttach() }},
		{"run.breakpoint", "Toggle Breakpoint", "Run", "Cmd+F8", "toggle breakpoint", func(m *Model) { m.ToggleBreakpoint() }},
		{"run.stepOver", "Step Over", "Run", "F8", "step over", func(m *Model) { m.StepOver() }},
		{"run.stepInto", "Step Into", "Run", "F7", "step into", func(m *Model) { m.StepInto() }},
		{"run.stepOut", "Step Out", "Run", "Shift+F8", "step out", func(m *Model) { m.StepOut() }},
		{"debug.forceStepOver", "Force Step Over", "Run", "", "force step over ignore breakpoints", func(m *Model) { m.ForceStepOver() }},
		{"debug.forceStepInto", "Force Step Into", "Run", "", "force step into ignore filters", func(m *Model) { m.ForceStepInto() }},
		{"run.runToCursor", "Run to Cursor", "Run", "Alt+F9", "run to cursor continue", func(m *Model) { m.RunToCursor(false) }},
		{"run.forceRunToCursor", "Force Run to Cursor", "Run", "Cmd+Alt+F9", "force run to cursor ignore breakpoints", func(m *Model) { m.ForceRunToCursor() }},
		{"debug.removeAllBreakpoints", "Remove All Breakpoints", "Run", "", "remove clear all breakpoints", func(m *Model) { m.RemoveAllBreakpoints() }},
		{"debug.muteBreakpoints", "Mute Breakpoints", "Run", "", "mute disable breakpoints temporarily", func(m *Model) { m.ToggleBreakpointsMuted() }},
		{"debug.toggleTemporaryBreakpoint", "Toggle Temporary Breakpoint", "Run", "", "temporary breakpoint remove after hit", func(m *Model) { m.ToggleTemporaryBreakpoint() }},
		{"debug.toggleMethodBreakpoint", "Toggle Method Breakpoint", "Run", "", "method breakpoint function debug", func(m *Model) { m.ToggleMethodBreakpoint() }},
		{"debug.toggleFieldWatchpoint", "Toggle Field Watchpoint", "Run", "", "field watchpoint data breakpoint debug", func(m *Model) { m.ToggleFieldWatchpoint() }},
		{"debug.viewBreakpoints", "View Breakpoints…", "Run", "", "breakpoints methods fields watchpoints", func(m *Model) { m.OpenDebugBreakpoints() }},
		{"debug.memory.refresh", "Refresh Memory View", "Run", "", "jvm memory classes refresh", func(m *Model) { m.refreshDebugMemory() }},
		{"debug.memory.showInstances", "Show Instances", "Run", "", "jvm memory instances objects", func(m *Model) { m.DebugShowMemoryInstances() }},
		{"debug.memory.showReferrers", "Show Referrers", "Run", "", "jvm memory referrers incoming references", func(m *Model) { m.DebugShowMemoryReferrers() }},
		{"debug.memory.closeInstances", "Close Instances", "Run", "", "jvm memory close instances", func(m *Model) { m.DebugCloseMemoryInstances() }},
		{"debug.memory.jumpToSource", "Jump to Source", "Run", "", "jvm memory class source", func(m *Model) { m.DebugMemoryJumpToSource("") }},
		{"debug.heap.capture", "Capture Heap Snapshot", "Run", "", "heap memory dump hprof snapshot", func(m *Model) { m.DebugCaptureHeap() }},
		{"debug.heap.open", "Open Heap Dump…", "Run", "", "heap memory dump hprof open", func(m *Model) { m.DebugOpenHeapDump() }},
		{"debug.heap.saveDump", "Save Heap Dump As…", "Run", "", "heap memory dump hprof save export", func(m *Model) { m.DebugSaveHeapDump() }},
		{"run.continue", "Resume Program", "Run", "", "continue resume", func(m *Model) { m.DebugContinue() }},
		{"run.pause", "Pause Program", "Run", "", "pause suspend", func(m *Model) { m.DebugPause() }},
		{"run.reloadConfigs", "Reload Run Configurations", "Run", "", "reload run configurations", func(m *Model) { m.ReloadRunConfigs() }},
		{"run.configNext", "Next Run Configuration", "Run", "", "next run configuration", func(m *Model) { m.CycleRunConfig(1) }},
		{"run.configPrev", "Previous Run Configuration", "Run", "", "previous run configuration", func(m *Model) { m.CycleRunConfig(-1) }},
		{"build.buildProject", "Build Project", "Build", "Cmd+F9", "build project compile make mvn gradle cmake", func(m *Model) { m.BuildProject() }},
		{"build.buildModule", "Build Module", "Build", "Cmd+Shift+F9", "build module compile file", func(m *Model) { m.BuildModule() }},
		{"build.compileFile", "Compile File", "Build", "", "javac single file recompile", func(m *Model) { m.CompileFile() }},
		{"build.rebuildProject", "Rebuild Project", "Build", "", "rebuild clean build", func(m *Model) { m.RebuildProject() }},
		{"build.buildArtifacts", "Build Artifacts", "Build", "", "artifacts package assemble jar release", func(m *Model) { m.BuildArtifacts() }},
		{"build.stop", "Stop Build", "Build", "", "stop cancel build", func(m *Model) { m.StopBuild() }},
		{"maven.reload", "Reload All Maven Projects", "Build", "", "maven reload sync projects", func(m *Model) { m.ReloadMaven() }},
		{"maven.cancelReload", "Stop Reloading Maven Projects", "Build", "", "maven cancel stop reload sync", func(m *Model) { m.CancelBuildToolReload("maven") }},
		{"maven.expandAll", "Expand All", "Build", "", "maven expand all dependencies modules", func(m *Model) { m.ExpandMavenAll() }},
		{"maven.collapseAll", "Collapse All", "Build", "", "maven collapse all dependencies modules", func(m *Model) { m.CollapseMavenAll() }},
		{"maven.toggleOffline", "Toggle Offline Mode", "Build", "", "maven offline mode", func(m *Model) { m.ToggleMavenOffline() }},
		{"maven.toggleSkipTests", "Toggle 'Skip Tests' Mode", "Build", "", "maven skip tests mode", func(m *Model) { m.ToggleMavenSkipTests() }},
		{"maven.analyzeDependencies", "Analyze Dependencies…", "Build", "", "maven dependency analysis", func(m *Model) { m.AnalyzeBuildDependencies() }},
		{"maven.settings", "Maven Settings…", "Build", "", "maven settings", func(m *Model) { m.OpenSettingsPage(settingsPageMaven) }},
		{"gradle.reload", "Reload All Gradle Projects", "Build", "", "gradle reload sync projects", func(m *Model) { m.ReloadGradle() }},
		{"gradle.cancelReload", "Stop Reloading Gradle Projects", "Build", "", "gradle cancel stop reload sync", func(m *Model) { m.CancelBuildToolReload("gradle") }},
		{"gradle.expandAll", "Expand All", "Build", "", "gradle expand all dependencies", func(m *Model) { m.ExpandGradleAll() }},
		{"gradle.collapseAll", "Collapse All", "Build", "", "gradle collapse all dependencies", func(m *Model) { m.CollapseGradleAll() }},
		{"gradle.toggleOffline", "Toggle Offline Mode", "Build", "", "gradle offline mode", func(m *Model) { m.ToggleGradleOffline() }},
		{"gradle.toggleSkipTests", "Toggle 'Skip Tests' Mode", "Build", "", "gradle skip tests mode", func(m *Model) { m.ToggleGradleSkipTests() }},
		{"gradle.analyzeDependencies", "Analyze Dependencies…", "Build", "", "gradle dependency analysis", func(m *Model) { m.AnalyzeBuildDependencies() }},
		{"view.build", "Show Build Window", "View", "", "build output console", func(m *Model) { m.ShowBuild() }},
		{"java.tools", "Install Java Language Services and Debugger", "Run", "", "java jdtls language server debugger install toolchain", func(m *Model) { m.InstallJavaToolchain() }},
		{"file.settings", "Settings…", "File", "Cmd+,", "settings preferences theme font appearance", func(m *Model) { m.OpenSettings() }},
		{"file.invalidateCaches", "Invalidate Caches…", "File", "", "invalidate caches clear rebuild index restart repair", func(m *Model) { m.OpenInvalidateCaches() }},
		{"app.restart", "Restart IDE", "File", "", "restart relaunch restart ide", func(m *Model) { m.RestartIDE() }},
		{"view.toggleTheme", "Toggle Dark/Light Theme", "View", "", "theme dark light appearance toggle", func(m *Model) { m.ToggleTheme() }},
		{"app.themeLight", "Light", "View", "", "theme light appearance", func(m *Model) { m.SetThemePref("light") }},
		{"app.themeDark", "Dark", "View", "", "theme dark appearance", func(m *Model) { m.SetThemePref("dark") }},
		{"app.themeSystem", "Sync with OS", "View", "", "theme system appearance", func(m *Model) { m.SetThemePref("system") }},
		{"view.zoomIn", "Zoom In IDE", "View", "Ctrl+Alt+=", "zoom scale bigger accessibility interface", func(m *Model) { m.StepIDEOZoom(1) }},
		{"view.zoomOut", "Zoom Out IDE", "View", "Ctrl+Alt+-", "zoom scale smaller accessibility interface", func(m *Model) { m.StepIDEOZoom(-1) }},
		{"view.resetZoom", "Reset IDE Zoom", "View", "Ctrl+Alt+0", "zoom scale 100 accessibility interface", func(m *Model) { m.ResetIDEOZoom() }},
		{"view.toggleCompactMode", "Compact Mode", "View", "", "appearance density smaller compact toolbars rows tabs", func(m *Model) { m.ToggleCompactMode() }},
		{"view.toggleToolbar", "Toolbar", "View", "", "appearance main toolbar header hide show", func(m *Model) { m.ToggleMainToolbar() }},
		{"view.toggleToolWindowBars", "Tool Window Bars", "View", "", "appearance stripes tool window buttons hide show", func(m *Model) { m.ToggleToolWindowBars() }},
		{"view.toggleStatusBar", "Status Bar", "View", "", "appearance hide show", func(m *Model) { m.ToggleStatusBar() }},
		{"view.navigationBarTop", "Top", "View", "", "appearance navigation bar breadcrumbs above editor project path", func(m *Model) { m.SetNavigationBarPosition("top") }},
		{"view.navigationBarStatusBar", "Status Bar", "View", "", "appearance navigation bar breadcrumbs footer project path", func(m *Model) { m.SetNavigationBarPosition("statusBar") }},
		{"view.navigationBarHidden", "Don't Show", "View", "", "appearance navigation bar breadcrumbs hide file path", func(m *Model) { m.SetNavigationBarPosition("hidden") }},
		{"view.backgroundTasks", "Background Tasks", "View", "", "progress running processes downloads cancel", func(m *Model) { m.ShowBackgroundTasks() }},
		{"workbench.stopBackgroundProcesses", "Stop Background Processes…", "Window", "Cmd+Shift+F2", "cancel task background tasks progress", func(m *Model) { m.StopBackgroundProcesses() }},
		{"view.toggleStatusBarWidget", "Status Bar Widget", "View", "", "appearance footer widget hide show", func(m *Model) {}},
		{"toolwindow.closeContent", "Close Tool Window Tab", "Window", "Cmd+W Ctrl+Shift+F4", "close selected tool window content tab", func(m *Model) { m.CloseToolWindowContent() }},
		{"editor.toggleZen", "Toggle Distraction Free Mode", "View", "", "distraction free mode zen presentation", func(m *Model) { m.ToggleZen() }},
		{"window.toggleFullScreen", "Toggle Full Screen", "View", "Cmd+Ctrl+F", "fullscreen window display", func(m *Model) { m.ToggleFullScreen() }},
		{"view.quickSwitchScheme", "Quick Switch Scheme…", "View", "Ctrl+`", "theme color scheme keymap view mode", func(m *Model) { m.QuickSwitchScheme() }},
		{"settings.appearance", "Appearance…", "View", "", "settings appearance theme font", func(m *Model) { m.OpenSettingsPage("appearance") }},
		{"settings.keymap", "Keymap…", "View", "", "settings keymap shortcuts bindings", func(m *Model) { m.OpenSettingsPage("keymap") }},
		{"toolbar.customize", "Customize Main Toolbar…", "View", "", "toolbar widgets title bar customize", func(m *Model) { m.OpenToolbarCustomize() }},
		{"toolbar.reset", "Restore Default Toolbar", "View", "", "toolbar default widgets restore", func(m *Model) { m.RestoreDefaultToolbar() }},
		{"app.checkForUpdates", "Check for Updates…", "Help", "", "update upgrade version release", func(m *Model) { m.CheckForUpdates() }},
		{"app.whatsNew", "What's New", "Help", "", "whats new release notes changelog update notes", func(m *Model) { m.OpenCurrentWhatsNew() }},
	}, append(append(append(projectTreeCommands(), parityWindowCommands()...),
		parityToolCommands()...), append(append(downloadJDKCommands(), parityRunCommands()...),
		servicesCommands()...)...)...)
}

// Command returns the registry's command with id, the one entry the menu, the
// title bar and the palette run a feature through.
// commands is every action the palette offers, in registration order: the
// registry's built-ins and anything registered since.
func commands() []appCommand {
	list := actionList()
	out := make([]appCommand, 0, len(list))
	for _, a := range list {
		if hiddenActionIDs[a.ID] {
			continue
		}
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
		run: func(m *Model) {
			m.RunActionWithArg(a.ID, nil)
		},
	}
}

// RunCommand runs the registry command with id and reports whether the
// registry knew it.
func (m *Model) RunCommand(id string) bool {
	return m.RunCommandWithArg(id, nil)
}

// RunCommandWithArg runs a registry command and records it in Recent Actions.
func (m *Model) RunCommandWithArg(id string, arg any) bool {
	if !m.RunActionWithArg(id, arg) {
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
	dlg := nativeModal(c, &open, func() {
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
