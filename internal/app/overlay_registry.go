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
	reg("rename", overlayLayerPopup, 10, false,
		func(m *Model) bool { return m.rename != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.renameDialog(c, p) })
	reg("symbols", overlayLayerPopup, 20, false,
		func(m *Model) bool { return m.symbols != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.symbolsDialog(c, p) })
	reg("intentions", overlayLayerPopup, 30, false,
		func(m *Model) bool { return m.intents != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.intentionsDialog(c, p) })
	reg("quickFiles", overlayLayerPopup, 40, false,
		func(m *Model) bool { return m.files != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.filesDialog(c, p) })
	reg("branches", overlayLayerPopup, 50, false,
		func(m *Model) bool { return m.branch.open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.branchesDialog(c, p) })
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
	reg("toolbarCustomize", overlayLayerDialog, 240, true,
		func(m *Model) bool { return m.toolbarCustomize },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.toolbarCustomizeDialog(c, p) })
	reg("updates", overlayLayerDialog, 250, true,
		func(m *Model) bool { return m.update.Open },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.updateDialog(c, p) })
	reg("invalidateCaches", overlayLayerDialog, 260, true,
		func(m *Model) bool { return m.invalidateDlg != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.invalidateCachesDialog(c, p) })
	reg("scratchChooser", overlayLayerDialog, 270, false,
		func(m *Model) bool { return m.scratch != nil },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.scratchChooserDialog(c, p) })
	// The Trust Project question, shown for a project the user has not
	// allowed to build and run.
	reg("trust", overlayLayerDialog, 265, true,
		func(m *Model) bool { return m.trustAsk },
		func(m *Model, c *ui.Context, p *ui.Theme) { m.trustDialog(c, p) })
}
