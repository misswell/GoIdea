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
		case "file.newScratchFile":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "file.openProjectPath":
			a.RunWithArg = func(m *Model, arg any) { m.openProjectPath(projectPathArg(arg)) }
		case "file.openRecentProject":
			a.RunWithArg = func(m *Model, arg any) { m.openProjectPath(projectPathArg(arg)) }
		case "build.compileFile":
			a.When = func(m *Model) bool {
				return m.Root() != "" && !m.showingWelcome
			}
			a.RunWithArg = func(m *Model, arg any) { m.runCompileFileAction(arg) }
		case "build.buildProject":
			a.When = buildActionAvailable
			a.RunWithArg = func(m *Model, arg any) { m.runBuildProjectAction(arg) }
		case "build.buildModule":
			a.When = buildActionAvailable
			a.RunWithArg = func(m *Model, arg any) { m.runBuildModuleAction(arg) }
		case "build.rebuildProject":
			a.When = buildActionAvailable
			a.RunWithArg = func(m *Model, arg any) { m.runRebuildProjectAction(arg) }
		case "build.buildArtifacts":
			a.When = buildActionAvailable
			a.RunWithArg = func(m *Model, arg any) { m.runBuildArtifactsAction(arg) }
		case "build.stop":
			a.When = func(m *Model) bool { return m.build.busy }
		case "maven.addProjects", "maven.importExternal":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "maven.reload", "maven.generateSources", "maven.executeGoal", "maven.settings":
			a.When = mavenBuildToolAvailable
		case "maven.cancelReload":
			a.When = mavenBuildToolAvailable
			hiddenActionIDs[c.id] = true
		case "maven.toggleOffline":
			a.When = mavenBuildToolAvailable
			a.Toggled = func(m *Model) bool { return m.maven.offline }
		case "maven.toggleSkipTests":
			a.When = mavenBuildToolAvailable
			a.Toggled = func(m *Model) bool { return m.maven.skipTests }
		case "maven.expandAll", "maven.collapseAll":
			a.When = mavenBuildToolAvailable
			hiddenActionIDs[c.id] = true
		case "maven.unlinkProjects":
			a.When = mavenBuildToolAvailable
			hiddenActionIDs[c.id] = true
		case "maven.analyzeDependencies", "gradle.analyzeDependencies":
			a.When = func(m *Model) bool {
				return mavenBuildToolAvailable(m) || gradleBuildToolAvailable(m)
			}
			a.RunWithArg = func(m *Model, arg any) { m.OpenBuildDependencyAnalyzer(arg) }
		case "gradle.linkProjects":
			a.When = gradleBuildToolAvailable
		case "gradle.reload", "gradle.executeTask":
			a.When = gradleBuildToolAvailable
			if c.id == "gradle.reload" {
				a.RunWithArg = func(m *Model, arg any) {
					if buildDir, ok := buildToolArgValue(arg, "buildDir"); ok {
						m.SyncGradleBuild(buildDir)
						return
					}
					m.ReloadGradle()
				}
			}
		case "gradle.cancelReload":
			a.When = gradleBuildToolAvailable
			hiddenActionIDs[c.id] = true
		case "gradle.toggleOffline":
			a.When = gradleBuildToolAvailable
			a.Toggled = func(m *Model) bool { return m.maven.gradleOffline }
		case "gradle.toggleSkipTests":
			a.When = gradleBuildToolAvailable
			a.Toggled = func(m *Model) bool { return m.maven.gradleSkipTests }
		case "gradle.expandAll", "gradle.collapseAll", "gradle.unlinkProjects":
			a.When = gradleBuildToolAvailable
			hiddenActionIDs[c.id] = true
		case "run.debug":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
			a.RunWithArg = func(m *Model, arg any) { m.DebugConfiguration(arg) }
		case "run.chooseConfig":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
			a.RunWithArg = func(m *Model, arg any) {
				if arg == "select" {
					m.OpenRunChooser("select")
					return
				}
				m.OpenRunChooser("run")
			}
		case "run.editConfigs":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
			a.RunWithArg = func(m *Model, arg any) { m.OpenRunConfigsFor(arg) }
		case "run.runContext", "run.debugContext":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
			a.RunWithArg = func(m *Model, arg any) {
				m.RunContextConfigurationFor(arg, c.id == "run.debugContext")
			}
		case "run.stop":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
			a.RunWithArg = func(m *Model, arg any) { m.StopProcess(arg) }
		case "edit.undo":
			a.When = func(m *Model) bool {
				return m.projectTreeFocus || m.activeEditorAvailable() || m.activeNotebook()
			}
			a.Enabled = func(m *Model) bool {
				if m.projectTreeFocus {
					return m.treeLastMove != nil
				}
				if m.activeNotebook() {
					return m.activeNotebookCanUndo()
				}
				return m.activeEditorCanUndo()
			}
		case "edit.redo":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() || m.activeNotebook() }
			a.Enabled = func(m *Model) bool {
				if m.activeNotebook() {
					return m.activeNotebookCanRedo()
				}
				return m.activeEditorCanRedo()
			}
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
		case "code.pasteFromHistory", "editor.inlineCompletion.trigger":
			a.When = func(m *Model) bool { return m.activeEditorEditable() }
			a.KeyWhen = func(m *Model) bool { return m.activeEditorEditable() }
		case "editor.toggleColumnSelection":
			a.When = func(m *Model) bool { return m.activeEditorAvailable() }
			a.KeyWhen = func(m *Model) bool { return m.activeEditorAvailable() }
			a.Toggled = func(m *Model) bool { return m.columnSelectionEnabled() }
		case "editor.toggleSemanticHighlighting":
			a.Toggled = func(m *Model) bool { return m.prefs.EditorScheme.SemanticHighlighting }
		case "editor.toggleRainbowBrackets":
			a.Toggled = func(m *Model) bool { return m.prefs.EditorScheme.RainbowBrackets }
		case "editor.colorScheme.import":
			a.RunWithArg = func(m *Model, arg any) { m.ImportColorScheme(arg) }
		case "editor.colorScheme.export":
			a.RunWithArg = func(m *Model, arg any) { m.ExportColorScheme(arg) }
		case "editor.colorScheme.delete":
			a.RunWithArg = func(m *Model, arg any) { m.DeleteColorScheme(arg) }
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
		case "debug.reloadChangedClasses":
			a.RunWithArg = func(m *Model, arg any) {
				if configID := hotSwapConfigIDArg(arg); configID != "" {
					m.HotSwapReloadForConfigAsync(configID)
					return
				}
				m.HotSwapReload()
			}
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
		case "http.runRequest", "http.copyAsCurl", "http.addPublicEnvironment",
			"http.addPrivateEnvironment", "http.runAll":
			a.When = func(m *Model) bool { return m.activeHTTPRequestEditor() }
			a.KeyWhen = func(m *Model) bool { return m.activeHTTPRequestEditor() }
		case "http.newRequest":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "http.showToolWindow":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "http.generateSpringRequest", "http.generateSpringControllerRequests":
			a.When = func(m *Model) bool { return m.activeSpringEditor() }
		case "graphql.showToolWindow", "graphql.addConfiguration", "graphql.editConfiguration",
			"graphql.restartDiscovery":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "graphql.runQuery", "graphql.introspect":
			a.When = func(m *Model) bool { return m.activeGraphQLEditor() }
			a.KeyWhen = func(m *Model) bool { return m.activeGraphQLEditor() }
			a.Enabled = func(m *Model) bool { return m.graphql.running == nil }
		case "view.profiler", "run.profile", "run.profileAllocations", "run.profileLiveObjects":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		case "run.profileContext":
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
		if _, editorCommand := keymapEditorCommands[c.id]; editorCommand &&
			a.KeyWhen == nil &&
			!strings.HasPrefix(c.id, "edit.") &&
			!strings.HasPrefix(c.id, "find.") {
			a.KeyWhen = func(m *Model) bool { return m.activeEditorFocused() }
		}
		if c.id == "layout.restore" {
			a.RunWithArg = func(m *Model, arg any) {
				m.OpenRestoreToolWindowLayout(arg)
			}
		}
		if c.id == "http.newRequest" {
			a.RunWithArg = func(m *Model, arg any) {
				dir, _ := arg.(string)
				m.NewHTTPRequest(dir)
			}
		}
		if c.id == "http.openEndpoint" {
			a.RunWithArg = func(m *Model, arg any) {
				m.OpenHTTPEndpoint(arg)
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
		serviceActionState(a)
		dockerActionState(a)
		macroActionState(a)
		coverageActionState(a)
		downloadJDKActionState(a)
		workspaceActionState(a)
		_ = registerAction(a)
	}
	registerFileTypeActions()
}

func mavenBuildToolAvailable(m *Model) bool {
	return m != nil && m.Root() != "" && !m.showingWelcome && m.maven.model.Maven != nil
}

func gradleBuildToolAvailable(m *Model) bool {
	return m != nil && m.Root() != "" && !m.showingWelcome && len(m.maven.model.Gradle) > 0
}
