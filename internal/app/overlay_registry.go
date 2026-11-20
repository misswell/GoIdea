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
	reg("coverage.manageReports", overlayLayerDialog, 258, true,
		func(m *Model) bool { return m.coverage.manage },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.coverageManageDialog(c, p) })
	reg("rename", overlayLayerPopup, 10, false,
		func(m *Model) bool { return m.rename != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.renameDialog(c, p) })
	reg("symbols", overlayLayerPopup, 20, false,
		func(m *Model) bool { return m.symbols != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.symbolsDialog(c, p) })
	reg("implementation", overlayLayerPopup, 25, true,
		func(m *Model) bool { return m.impl.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.implementationDialog(c, p) })
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
	reg("termShell", overlayLayerPopup, 58, true,
		func(m *Model) bool { return m.termShell != nil && m.termShell.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.termShellDialog(c, p) })
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
	reg("run.stackTraceDialog", overlayLayerDialog, 190, true,
		func(m *Model) bool { return m.stackTrace != nil && m.stackTrace.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.stackTraceDialog(c, p) })
	reg("branches", overlayLayerPopup, 50, false,
		func(m *Model) bool { return m.branch.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.branchesDialog(c, p) })
	reg("vcs.operations", overlayLayerDialog, 55, true,
		func(m *Model) bool { return m.vcsOps.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.vcsOperationDialog(c, p) })
	reg("vcs.quickList", overlayLayerDialog, 56, true,
		func(m *Model) bool { return m.vcsQuick.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.vcsQuickListDialog(c, p) })
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
	reg("treeOp", overlayLayerDialog, 130, false,
		func(m *Model) bool { return m.treeOp != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.treeOpDialog(c, p) })
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
		func(m *Model) bool { return m.commit != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.commitDialog(c, p) })
	reg("settings", overlayLayerDialog, 230, true,
		func(m *Model) bool { return m.settingsOpen },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.settingsDialog(c, p) })
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
	reg("attach", overlayLayerDialog, 254, true,
		func(m *Model) bool { return m.attach != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.attachDialog(c, p) })
	reg("runChooser", overlayLayerPopup, 253, false,
		func(m *Model) bool { return m.chooser != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.runChooserDialog(c, p) })
	reg("rerunConfirm", overlayLayerDialog, 255, true,
		func(m *Model) bool { return m.rerunAsk != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.rerunConfirmDialog(c, p) })
	reg("scratchChooser", overlayLayerDialog, 270, false,
		func(m *Model) bool { return m.scratch != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.scratchChooserDialog(c, p) })
	// The Trust Project question, shown for a project the user has not
	// allowed to build and run.
	reg("trust", overlayLayerDialog, 265, true,
		func(m *Model) bool { return m.trustAsk },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.trustDialog(c, p) })
}
