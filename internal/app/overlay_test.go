package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestOverlaysAreRegisteredOnce(t *testing.T) {
	want := []string{"rename", "symbols", "intentions", "quickFiles", "branches",
		"bookmarksPopup", "commandPalette", "searchEverywhere", "runAnything", "diff",
		"settings", "jdk.download", "newProject", "invalidateCaches", "scratchChooser",
		"commit", "database"}
	for _, id := range want {
		if _, ok := overlays.byID[id]; !ok {
			t.Fatalf("%s is not registered", id)
		}
	}
	seen := map[string]bool{}
	for _, o := range overlays.list {
		if seen[o.ID] {
			t.Fatalf("%s registered twice", o.ID)
		}
		seen[o.ID] = true
	}
}

func TestOverlayOpenFollowsBusinessState(t *testing.T) {
	m := &Model{}
	if m.overlayOpen() {
		t.Fatal("a fresh model has an overlay up")
	}
	m.palette = &commandPicker{open: true}
	if !m.overlayOpen() {
		t.Fatal("the palette is not seen")
	}
	if m.modalOverlayOpen() {
		t.Fatal("a picker is not a modal sheet")
	}
	m.palette = nil
	m.settingsOpen = true
	if !m.overlayOpen() || !m.modalOverlayOpen() {
		t.Fatal("Settings is not a modal overlay")
	}
	m.settingsOpen = false
	m.invalidateDlg = &invalidatePane{}
	if !m.modalOverlayOpen() {
		t.Fatal("Invalidate Caches is not a modal overlay")
	}
	m.invalidateDlg = nil
}

func TestNewProjectDrawsItsSheet(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) { fn() }
	m.NewProject()
	if !m.newProject.open {
		t.Fatal("New Project did not open")
	}
	if !m.modalOverlayOpen() {
		t.Fatal("the New Project sheet is not a modal overlay")
	}
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()
	// "Location:" belongs to the sheet alone; the wizard used to set its state
	// without a registered overlay, so the workbench was all that was drawn.
	if _, ok := tt.Find("Location:"); !ok {
		t.Fatalf("the New Project sheet was not drawn: %v", tt.Texts())
	}
	if _, ok := tt.Find("Create Git repository"); !ok {
		t.Fatal("the sheet's Git option was not drawn")
	}
	m.CloseNewProject()
	if m.overlayOpen() {
		t.Fatal("closing New Project left an overlay up")
	}
}

func TestOverlayOrderIsStable(t *testing.T) {
	order := overlayOrder()
	if len(order) < 10 {
		t.Fatalf("only %d overlays", len(order))
	}
	for i := 1; i < len(order); i++ {
		prev, cur := order[i-1], order[i]
		if cur.Layer < prev.Layer {
			t.Fatalf("%s (layer %d) draws under %s (layer %d)", cur.ID, cur.Layer, prev.ID, prev.Layer)
		}
		if cur.Layer == prev.Layer && cur.Priority < prev.Priority {
			t.Fatalf("%s (priority %d) draws under %s (priority %d)", cur.ID, cur.Priority, prev.ID, prev.Priority)
		}
	}
	// Settings draws above the palette, as it did in the old View list.
	if !indexBefore(order, "commandPalette", "settings") {
		t.Fatal("the settings sheet is under the palette")
	}
}

func indexBefore(list []*Overlay, a, b string) bool {
	ia, ib := -1, -1
	for i, o := range list {
		if o.ID == a {
			ia = i
		}
		if o.ID == b {
			ib = i
		}
	}
	return ia >= 0 && ib >= 0 && ia < ib
}

func TestOverlayUnregister(t *testing.T) {
	registerOverlay(&Overlay{ID: "test.overlay", Layer: overlayLayerPopup, Priority: 1,
		Open:   func(m *Model) bool { return true },
		Render: func(m *Model, c *ui.Context, p *ui.Theme) {}})
	if !unregisterOverlay("test.overlay") {
		t.Fatal("unregister reported nothing removed")
	}
	if unregisterOverlay("test.overlay") {
		t.Fatal("the overlay was removed twice")
	}
}

func TestOverlayDefaultActionUsesTheTopmostModal(t *testing.T) {
	defer unregisterOverlay("test.default.first")
	defer unregisterOverlay("test.default.second")
	m := &Model{}
	registerTestOverlay("test.default.first", true, func(*Model) bool { return true },
		func(*Model) func() { return func() {} })
	registerTestOverlay("test.default.second", true, func(*Model) bool { return true },
		func(*Model) func() { return nil })
	if action := m.overlayDefaultAction(); action != nil {
		t.Fatal("a disabled topmost default fell through to the dialog below")
	}
	unregisterOverlay("test.default.second")
	if action := m.overlayDefaultAction(); action == nil {
		t.Fatal("the visible modal has no default action")
	}
}

func TestModalOverlayRoutesUnclaimedEnterToItsDefault(t *testing.T) {
	defer unregisterOverlay("test.default.enter")
	entered := false
	registerTestOverlay("test.default.enter", true, func(*Model) bool { return true },
		func(*Model) func() {
			return func() { entered = true }
		})
	m := &Model{}
	tester := ui.NewTester(func(c *ui.Context) { m.shortcuts(c) }, 800, 600)
	tester.Frame()
	tester.Key(0, ui.KeyEnter)
	tester.Frame()
	if !entered {
		t.Fatal("modal overlay did not run its default action on Enter")
	}
}

func registerTestOverlay(id string, modal bool, open func(*Model) bool, action func(*Model) func()) *Overlay {
	o := &Overlay{ID: id, Layer: overlayLayerDialog, Priority: 1000, Modal: modal,
		Open: open, Render: func(*Model, *ui.Context, *ui.Theme) {}, Default: action}
	registerOverlay(o)
	return o
}

func TestModalOrderPreservesOverlayLayers(t *testing.T) {
	defer unregisterOverlay("test.modal.popup")
	defer unregisterOverlay("test.modal.dialog")
	registerTestOverlay("test.modal.popup", false, func(*Model) bool { return true }, nil)
	registerTestOverlay("test.modal.dialog", true, func(*Model) bool { return true },
		func(*Model) func() { return func() {} })
	m := &Model{}
	open := m.openOverlays()
	if len(open) < 2 {
		t.Fatalf("open overlays = %d", len(open))
	}
	var popup, dialog int
	for i, o := range open {
		switch o.ID {
		case "test.modal.popup":
			popup = i
		case "test.modal.dialog":
			dialog = i
		}
	}
	if popup >= dialog {
		t.Fatalf("popup index %d, dialog index %d", popup, dialog)
	}
}

func TestOverlayCloseHandsTheKeyboardBack(t *testing.T) {
	m := &Model{}
	m.overlayFocusSettle([]string{"rename"}, true)
	if m.focusApply {
		t.Fatal("the editor focused while the dialog was up")
	}
	m.overlayFocusSettle(nil, false)
	if !m.focusApply {
		t.Fatal("closing the last overlay did not restore the editor")
	}
	m.focusApply = false
	m.overlayIDs = []string{"a"}
	m.overlayFocusSettle([]string{"b"}, true)
	if m.focusApply {
		t.Fatal("a different dialog took the keyboard to the editor")
	}
	m.overlayIDs = []string{"a"}
	m.overlayFocusSettle([]string{"a"}, false)
	if m.focusApply {
		t.Fatal("an open overlay restored the editor")
	}
}
