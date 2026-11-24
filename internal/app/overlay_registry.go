package app

// Registered overlays: each entry wraps the field and dialog a feature already
// has, so the manager only changes where it is drawn and how the workbench
// asks whether one is up. Priorities follow the old drawing order, which keeps
// nested sheets stacked the way they were.

import (
	"github.com/egoist/mygo/ui"
)

func init() {
	reg := func(id string, layer, priority int, modal bool, open func(m *Model) bool,
		render func(m *Model, c *ui.Context, p *ui.Theme)) {
		registerOverlay(&Overlay{ID: id, Layer: layer, Priority: priority,
			Modal: modal, Open: open, Render: render})
	}
	// Popups and pickers, in the order the View drew them.
	reg("documentation.popup", overlayLayerPopup, 35, false,
		func(m *Model) bool { return m.documentation.popup != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.documentationPopup(c, p) })
	reg("workbench.backgroundTasks", overlayLayerPopup, 42, false,
		func(m *Model) bool { return m.tasksOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.backgroundTasksPopup(c, p) })
	reg("editor.navigationBar", overlayLayerPopup, 43, false,
		func(m *Model) bool { return m.navigationOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.navigationBarPopup(c, p) })
	reg("editor.quickDefinition", overlayLayerPopup, 44, false,
		func(m *Model) bool { return m.quickDefinition != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.quickDefinitionPopup(c, p) })
	reg("code.usagesPopup", overlayLayerPopup, 45, false,
		func(m *Model) bool { return m.usagePopup != nil && m.usagePopup.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.usagesPopup(c, p) })
	reg("editor.copyPathReference", overlayLayerPopup, 46, false,
		func(m *Model) bool { return m.copyPathPopup != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.copyPathReferencePopup(c, p) })
	reg("editor.pasteImports", overlayLayerDialog, 48, true,
		func(m *Model) bool { t := m.tab(); return t != nil && t.autoImport.paste.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.autoImportPasteDialog(c, p) })
	reg("editor.pasteHistory", overlayLayerDialog, 47, true,
		func(m *Model) bool { return m.editorExtras.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.editorPasteHistoryDialog(c, p) })
	reg("welcome.recentProjects", overlayLayerDialog, 36, false,
		func(m *Model) bool { return m.recentProjectsPopup != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.recentProjectsDialog(c, p) })
	reg("coverage.manageReports", overlayLayerDialog, 258, true,
		func(m *Model) bool { return m.coverage.manage },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.coverageManageDialog(c, p) })
	reg("rename", overlayLayerPopup, 10, false,
		func(m *Model) bool { return m.rename != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.renameDialog(c, p) })
	reg("editor.safeDelete", overlayLayerDialog, 15, true,
		func(m *Model) bool {
			return m.safeDelete != nil &&
				m.safeDelete.phase != editorSafeDeleteResolving
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.safeDeleteDialog(c, p) })
	reg("symbols", overlayLayerPopup, 20, false,
		func(m *Model) bool { return m.symbols != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.symbolsDialog(c, p) })
	reg("implementation", overlayLayerPopup, 25, true,
		func(m *Model) bool { return m.impl.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.implementationDialog(c, p) })
	reg("plugins.markerChooser", overlayLayerPopup, 26, true,
		func(m *Model) bool {
			return m.plugins != nil && m.plugins.visual.markerChooser.Open
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.pluginMarkerDialog(c, p) })
	reg("plugins.intellijThemes", overlayLayerDialog, 28, true,
		func(m *Model) bool { return m.intellijThemes != nil && m.intellijThemes.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.intellijThemesDialog(c, p) })
	reg("plugins.consent", overlayLayerDialog, 30, true,
		func(m *Model) bool { return m.intellijConsent != nil && m.intellijConsent.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.intellijConsentDialog(c, p) })
	// The database window's own sheets open on top of it: its sheet draws at
	// 210, so these sit above that rather than under it.
	reg("dbTable", overlayLayerDialog, 215, true,
		func(m *Model) bool { return m.dbTable != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbTableDialog(c, p) })
	reg("dbHistory", overlayLayerDialog, 216, true,
		func(m *Model) bool { return m.dbHist != nil && m.dbHist.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbHistoryDialog(c, p) })
	reg("dbPlan", overlayLayerDialog, 217, true,
		func(m *Model) bool { return m.dbPlan != nil && m.dbPlan.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbPlanDialog(c, p) })
	reg("database.source", overlayLayerDialog, 218, true,
		func(m *Model) bool { return m.db != nil && m.db.sourceForm != nil && m.db.sourceForm.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbSourceFormDialog(c, p) })
	reg("database.actionPrompt", overlayLayerDialog, 219, true,
		func(m *Model) bool { return m.db != nil && m.db.actionPrompt != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbActionPromptDialog(c, p) })
	reg("database.valueEditor", overlayLayerDialog, 220, true,
		func(m *Model) bool { return m.dbTable != nil && m.dbTable.valueOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbValueEditorDialog(c, p) })
	reg("database.tableEditor", overlayLayerDialog, 221, true,
		func(m *Model) bool { return m.db != nil && m.db.tableForm != nil && m.db.tableForm.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbTableFormDialog(c, p) })
	reg("database.importData", overlayLayerDialog, 222, true,
		func(m *Model) bool { return m.db != nil && m.db.importForm != nil && m.db.importForm.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbImportFormDialog(c, p) })
	reg("database.connectRemedy", overlayLayerDialog, 223, true,
		func(m *Model) bool { return m.db != nil && m.db.connectRemedy != nil && m.db.connectRemedy.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbConnectRemedyDialog(c, p) })
	reg("database.exportData", overlayLayerDialog, 225, true,
		func(m *Model) bool {
			return m.db != nil && m.db.exportDialog != nil && m.db.exportDialog.open
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbExportDialog(c, p) })
	reg("database.new", overlayLayerPopup, 224, true,
		func(m *Model) bool { return m.db != nil && m.db.newMenu != nil && m.db.newMenu.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbNewMenuPopup(c, p) })
	reg("termShell", overlayLayerPopup, 58, true,
		func(m *Model) bool { return m.termShell != nil && m.termShell.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.termShellDialog(c, p) })
	reg("term.rename", overlayLayerPopup, 59, false,
		func(m *Model) bool { return m.term.rename != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.termRenameDialog(c, p) })
	reg("intentions", overlayLayerPopup, 30, false,
		func(m *Model) bool { return m.intents != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.intentionsDialog(c, p) })
	reg("quickFiles", overlayLayerPopup, 40, false,
		func(m *Model) bool { return m.files != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.filesDialog(c, p) })
	reg("editor.gotoLine", overlayLayerDialog, 171, true,
		func(m *Model) bool { return m.gotoLine != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.gotoLineDialog(c, p) })
	reg("search.recent", overlayLayerDialog, 172, true,
		func(m *Model) bool { return m.recentNav.popup != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.recentNavigationDialog(c, p) })
	reg("find.scopes", overlayLayerDialog, 173, true,
		func(m *Model) bool { return m.search.scopesOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.searchScopesDialog(c, p) })
	reg("navigate.fileStructure", overlayLayerPopup, 174, false,
		func(m *Model) bool { return m.structure.popup },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.fileStructureDialog(c, p) })
	reg("run.stackTraceDialog", overlayLayerDialog, 190, true,
		func(m *Model) bool { return m.stackTrace != nil && m.stackTrace.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.stackTraceDialog(c, p) })
	reg("tools.externalToolPrompt", overlayLayerDialog, 191, true,
		func(m *Model) bool { return m.externalToolPrompt != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.externalToolPromptDialog(c, p) })
	reg("vim.command", overlayLayerDialog, 192, true,
		func(m *Model) bool { return m.vim.command != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.vimCommandDialog(c, p) })
	reg("branches", overlayLayerPopup, 50, false,
		func(m *Model) bool { return m.branch.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.branchesDialog(c, p) })
	reg("vcs.operations", overlayLayerDialog, 55, true,
		func(m *Model) bool { return m.vcsOps.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.vcsOperationDialog(c, p) })
	reg("vcs.quickList", overlayLayerDialog, 56, true,
		func(m *Model) bool { return m.vcsQuick.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.vcsQuickListDialog(c, p) })
	reg("vcs.more", overlayLayerDialog, 57, true,
		func(m *Model) bool { return m.vcsMore.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.vcsMoreDialog(c, p) })
	reg("vcs.fileConfirm", overlayLayerDialog, 58, true,
		func(m *Model) bool { return m.vcsFileConfirm != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.vcsFileConfirmationDialog(c, p) })
	reg("rollback", overlayLayerDialog, 60, false,
		func(m *Model) bool { return m.rollback != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.rollbackDialog(c, p) })
	reg("bookmarksPopup", overlayLayerPopup, 70, false,
		func(m *Model) bool { return m.bookmarksView != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.bookmarksDialog(c, p) })
	reg("bookmarkMnemonics", overlayLayerPopup, 80, false,
		func(m *Model) bool { return m.mnemonics != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.mnemonicDialog(c, p) })
	reg("bookmarkNewList", overlayLayerDialog, 90, false,
		func(m *Model) bool { return m.newList != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.newListDialog(c, p) })
	reg("bookmarkDeleteList", overlayLayerDialog, 100, false,
		func(m *Model) bool { return m.deleteList != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.deleteListDialog(c, p) })
	reg("bookmarkMove", overlayLayerDialog, 110, false,
		func(m *Model) bool { return m.move != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.moveBookmarkDialog(c, p) })
	reg("bookmarkDescription", overlayLayerDialog, 120, false,
		func(m *Model) bool { return m.editDesc != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.bookmarkDescriptionDialog(c, p) })
	reg("project.newPopup", overlayLayerPopup, 128, false,
		func(m *Model) bool { return m.projectNew != nil && m.projectNew.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.projectNewDialog(c, p) })
	reg("templates.newFile", overlayLayerDialog, 129, true,
		func(m *Model) bool { return m.treeOp != nil && m.treeOp.projectTemplate != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.treeOpDialog(c, p) })
	reg("templates.chooser", overlayLayerPopup, 129, false,
		func(m *Model) bool {
			return m.templateChooser != nil && m.templateChooser.kind != "updateTag"
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.templateChooserDialog(c, p) })
	reg("templates.emmetUpdateTag", overlayLayerPopup, 129, false,
		func(m *Model) bool {
			return m.templateChooser != nil && m.templateChooser.kind == "updateTag"
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.templateChooserDialog(c, p) })
	reg("treeOp", overlayLayerDialog, 130, false,
		func(m *Model) bool {
			return m.treeOp != nil && m.treeOp.projectTemplate == nil
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.treeOpDialog(c, p) })
	reg("project.fileRefactor", overlayLayerDialog, 131, true,
		func(m *Model) bool { return m.fileRefactor != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.fileRefactorDialog(c, p) })
	reg("project.refactoringPreview", overlayLayerDialog, 132, true,
		func(m *Model) bool { return m.fileRefactorPreview != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.fileRefactorPreviewDialog(c, p) })
	reg("history", overlayLayerDialog, 140, false,
		func(m *Model) bool { return m.history != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.historyDialog(c, p) })
	reg("dirCompare", overlayLayerDialog, 150, false,
		func(m *Model) bool { return m.dirCompare != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dirCompareDialog(c, p) })
	reg("commandPalette", overlayLayerDialog, 160, false,
		func(m *Model) bool { return m.palette != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.commandsDialog(c, p) })
	reg("searchEverywhere", overlayLayerDialog, 170, false,
		func(m *Model) bool { return m.se != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.searchEverywhereDialog(c, p) })
	reg("runAnything", overlayLayerDialog, 180, false,
		func(m *Model) bool { return m.runAnything != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.runAnythingDialog(c, p) })
	reg("services.addService", overlayLayerDialog, 182, true,
		func(m *Model) bool {
			return m.services.addChooser != nil && m.services.addChooser.open ||
				m.services.importing != nil && m.services.importing.open
		},
		func(m *Model, c *ui.Context, p *ui.Theme) {
			if m.services.addChooser != nil && m.services.addChooser.open {
				m.serviceAddChooserDialog(c, p)
			} else {
				m.serviceImportDialog(c, p)
			}
		})
	reg("run.serviceImport", overlayLayerDialog, 182, true,
		func(m *Model) bool {
			return m.services.addChooser != nil && m.services.addChooser.open ||
				m.services.importing != nil && m.services.importing.open
		},
		func(m *Model, c *ui.Context, p *ui.Theme) {
			if m.services.addChooser != nil && m.services.addChooser.open {
				m.serviceAddChooserDialog(c, p)
			} else {
				m.serviceImportDialog(c, p)
			}
		})
	reg("services.dockerSettingsForm", overlayLayerDialog, 183, true,
		func(m *Model) bool {
			return m.services.docker.settingsUI.formOpen &&
				m.services.docker.settingsUI.editing != "" &&
				!m.remote.sshSettingsOpen
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dockerSettingsFormDialog(c, p) })
	reg("services.dockerDialog", overlayLayerDialog, 184, true,
		func(m *Model) bool { return m.services.docker.dialog != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dockerDialogView(c, p) })
	reg("graphql.environment", overlayLayerDialog, 184, true,
		func(m *Model) bool { return m.graphql.environmentDialog != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.graphqlEnvironmentDialog(c, p) })
	reg("graphql.error", overlayLayerDialog, 184, true,
		func(m *Model) bool { return m.graphql.errorDialog != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.graphqlErrorDialog(c, p) })
	reg("graphql.introspectionConfirmation", overlayLayerDialog, 184, true,
		func(m *Model) bool { return m.graphql.introspectionAsk != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.graphqlIntrospectionDialog(c, p) })
	reg("toolwindow.layouts", overlayLayerDialog, 181, true,
		func(m *Model) bool { return m.layoutDlg != nil && m.layoutDlg.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.toolWindowLayoutDialog(c, p) })
	reg("agents.newTask", overlayLayerDialog, 185, true,
		func(m *Model) bool { return m.agents.newOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentsNewTaskDialog(c, p) })
	reg("agents.rename", overlayLayerDialog, 186, true,
		func(m *Model) bool { return m.agents.renameOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentsRenameDialog(c, p) })
	reg("agents.group", overlayLayerDialog, 187, true,
		func(m *Model) bool { return m.agents.groupOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentsGroupDialog(c, p) })
	reg("agents.removeGroup", overlayLayerDialog, 188, true,
		func(m *Model) bool { return m.agents.removeGroupOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentsRemoveGroupDialog(c, p) })
	reg("agents.delete", overlayLayerDialog, 188, true,
		func(m *Model) bool { return m.agents.deleteOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentsDeleteDialog(c, p) })
	reg("agents.changes", overlayLayerDialog, 189, true,
		func(m *Model) bool { return m.agents.changesOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentsChangesDialog(c, p) })
	reg("agents.externalSessions", overlayLayerDialog, 190, true,
		func(m *Model) bool { return m.agents.externalOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentsExternalSessionsDialog(c, p) })
	reg("agents.registryInstallConfirm", overlayLayerDialog, 191, true,
		func(m *Model) bool { return m.agents.registry.InstallConfirmID != "" },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentRegistryInstallConfirmDialog(c, p) })
	reg("agents.registryRemoveConfirm", overlayLayerDialog, 192, true,
		func(m *Model) bool { return m.agents.registry.RemoveOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentRegistryRemoveConfirmDialog(c, p) })
	reg("agents.custom", overlayLayerDialog, 193, true,
		func(m *Model) bool { return m.agents.registry.CustomOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.agentCustomDialog(c, p) })
	reg("merge", overlayLayerDialog, 185, true,
		func(m *Model) bool { return m.merge != nil && m.merge.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.mergeDialog(c, p) })
	reg("diff", overlayLayerDialog, 190, false,
		func(m *Model) bool { return m.diff != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.diffDialog(c, p) })
	reg("annotate", overlayLayerDialog, 200, false,
		func(m *Model) bool { return m.annotate != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.annotateDialog(c, p) })
	reg("database", overlayLayerDialog, 210, true,
		func(m *Model) bool { return m.db != nil && m.db.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.dbDialog(c, p) })
	reg("commit", overlayLayerDialog, 220, false,
		func(m *Model) bool {
			return m.commit != nil && !m.vcsCommitNonModal()
		},
		func(m *Model, c *ui.Context, p *ui.Theme) { m.commitDialog(c, p) })
	reg("settings", overlayLayerDialog, 230, true,
		func(m *Model) bool { return m.settingsOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.settingsDialog(c, p) })
	reg("settings.projectStructure", overlayLayerDialog, 230, true,
		func(m *Model) bool { return m.projectStructure.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.projectStructureDialog(c, p) })
	reg("jdk.download", overlayLayerDialog, 233, true,
		func(m *Model) bool { return m.jdkDialog.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.jdkDownloadDialog(c, p) })
	reg("app.about", overlayLayerDialog, 231, true,
		func(m *Model) bool { return m.aboutOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.aboutDialog(c, p) })
	reg("help.keymapReference", overlayLayerDialog, 232, true,
		func(m *Model) bool { return m.keymapReference.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.keymapReferenceDialog(c, p) })
	reg("app.workspace", overlayLayerDialog, 234, true,
		func(m *Model) bool { return m.workspaceDialog != nil && m.workspaceDialog.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.drawWorkspaceDialog(c, p) })
	// File › New Project… and the Welcome screen's button: a full sheet over
	// the workbench, above Settings so it cannot be hidden underneath it.
	reg("newProject", overlayLayerDialog, 235, true,
		func(m *Model) bool { return m.newProject.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.newProjectDialog(c, p) })
	reg("toolbarCustomize", overlayLayerDialog, 240, true,
		func(m *Model) bool { return m.toolbarCustomize },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.toolbarCustomizeDialog(c, p) })
	reg("invalidateCaches", overlayLayerDialog, 260, true,
		func(m *Model) bool { return m.invalidateDlg != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.invalidateCachesDialog(c, p) })
	reg("runConfigs", overlayLayerDialog, 252, true,
		func(m *Model) bool { return m.runCfgDlg != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.runConfigsDialog(c, p) })
	reg("buildTools.dependencyAnalyzer", overlayLayerDialog, 252, true,
		func(m *Model) bool { return m.maven.analyzer != nil && m.maven.analyzer.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.buildDependencyAnalyzerDialog(c, p) })
	reg("buildTools.addMavenDependency", overlayLayerDialog, 253, true,
		func(m *Model) bool { return m.maven.addDependency != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.addMavenDependencyDialog(c, p) })
	reg("attach", overlayLayerDialog, 254, true,
		func(m *Model) bool { return m.attach != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.attachDialog(c, p) })
	reg("runChooser", overlayLayerPopup, 253, false,
		func(m *Model) bool { return m.chooser != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.runChooserDialog(c, p) })
	reg("rerunConfirm", overlayLayerDialog, 255, true,
		func(m *Model) bool { return m.rerunAsk != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.rerunConfirmDialog(c, p) })
	reg("run.terminate", overlayLayerDialog, 255, true,
		func(m *Model) bool { return m.run.terminateAsk != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.runTerminateConfirmDialog(c, p) })
	reg("debug.evaluate", overlayLayerDialog, 256, true,
		func(m *Model) bool { return m.debug.evaluate.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.debugEvaluateDialog(c, p) })
	reg("debug.setValue", overlayLayerDialog, 256, true,
		func(m *Model) bool { return m.debug.setValue.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.debugSetValueDialog(c, p) })
	reg("debug.smartStep", overlayLayerDialog, 257, true,
		func(m *Model) bool { return m.debug.smartStep.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.debugSmartStepDialog(c, p) })
	reg("debug.consoleHistory", overlayLayerDialog, 258, true,
		func(m *Model) bool { return m.debug.console.historyOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.debugConsoleHistoryDialog(c, p) })
	reg("debug.breakpoints", overlayLayerDialog, 259, true,
		func(m *Model) bool { return m.debug.breakpointsOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.debugBreakpointsDialog(c, p) })
	reg("debug.breakpointEdit", overlayLayerDialog, 260, true,
		func(m *Model) bool { return m.debug.breakpointEdit.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.debugBreakpointEditorDialog(c, p) })
	// The user's answer to an MCP client is above every project sheet: the
	// client is waiting on it and it names the client, tool and project.
	reg("mcp.confirm", overlayLayerDialog, 290, true,
		func(m *Model) bool { return len(m.mcp.confirms) > 0 },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.mcpConfirmDialog(c, p) })
	reg("macros.edit", overlayLayerDialog, 279, true,
		func(m *Model) bool { return m.macros.editOpen },
		func(m *Model, c *ui.Context, _ *ui.Theme) { m.macroEditDialog(c) })
	reg("macros.name", overlayLayerDialog, 280, true,
		func(m *Model) bool { return m.macros.nameOpen },
		func(m *Model, c *ui.Context, _ *ui.Theme) { m.macroNameDialog(c) })
	reg("macros.play", overlayLayerDialog, 281, true,
		func(m *Model) bool { return m.macros.playOpen },
		func(m *Model, c *ui.Context, _ *ui.Theme) { m.macroPlayDialog(c) })
	reg("macros.rename", overlayLayerDialog, 282, true,
		func(m *Model) bool { return m.macros.renameOpen },
		func(m *Model, c *ui.Context, _ *ui.Theme) { m.macroRenameDialog(c) })
	reg("macros.delete", overlayLayerDialog, 283, true,
		func(m *Model) bool { return m.macros.deleteOpen },
		func(m *Model, c *ui.Context, _ *ui.Theme) { m.macroDeleteDialog(c) })
	reg("scratchChooser", overlayLayerDialog, 270, false,
		func(m *Model) bool { return m.scratch != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.scratchChooserDialog(c, p) })
	// The Trust Project question, shown for a project the user has not
	// allowed to build and run.
	reg("project.trustDialog", overlayLayerDialog, 265, true,
		func(m *Model) bool { return m.trustAsk },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.trustDialog(c, p) })
	reg("remote.connect", overlayLayerDialog, 266, true,
		func(m *Model) bool { return m.remote.connectOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.remoteConnectDialog(c, p) })
	reg("remote.disconnect", overlayLayerDialog, 267, true,
		func(m *Model) bool { return m.remote.disconnectOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.remoteDisconnectDialog(c, p) })
	reg("remote.notice", overlayLayerDialog, 268, true,
		func(m *Model) bool { return m.remote.noticeOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.remoteNoticeDialog(c, p) })
	reg("remote.secret", overlayLayerDialog, 269, true,
		func(m *Model) bool { return m.remote.secretOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.remoteSecretDialog(c, p) })
	reg("remote.hostTrust", overlayLayerDialog, 270, true,
		func(m *Model) bool { return m.remote.trustOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.remoteTrustDialog(c, p) })
	reg("remote.sshConfigurations", overlayLayerDialog, 271, true,
		func(m *Model) bool { return m.remote.sshSettingsOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.remoteSSHSettingsDialog(c, p) })
	reg("ssh.session", overlayLayerDialog, 272, true,
		func(m *Model) bool { return m.remote.sshSessionOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.remoteSSHSessionDialog(c, p) })
	reg("deployment.serverPicker", overlayLayerDialog, 273, true,
		func(m *Model) bool { return m.remote.deployment.pickerOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.deploymentPickerDialog(c, p) })
	reg("deployment.settings", overlayLayerDialog, 274, true,
		func(m *Model) bool { return m.remote.deployment.editorOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.deploymentSettingsDialog(c, p) })
	reg("deployment.auth", overlayLayerDialog, 275, true,
		func(m *Model) bool { return m.remote.deployment.authOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.deploymentAuthDialog(c, p) })
	reg("deployment.sync", overlayLayerDialog, 276, true,
		func(m *Model) bool { return m.remote.deployment.syncOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.deploymentSyncDialog(c, p) })
	reg("runTargets.settings", overlayLayerDialog, 277, true,
		func(m *Model) bool { return m.remote.runTargets.editorOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.runTargetSettingsDialog(c, p) })
	reg("runTargets.auth", overlayLayerDialog, 278, true,
		func(m *Model) bool { return m.remote.runTargets.remedyOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.runTargetAuthDialog(c, p) })
}
