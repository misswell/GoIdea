package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestOverlaysAreRegisteredOnce(t *testing.T) {
	want := []string{"rename", "symbols", "intentions", "quickFiles", "branches",
		"bookmarksPopup", "commandPalette", "searchEverywhere", "runAnything", "diff",
		"settings", "newProject", "invalidateCaches", "scratchChooser", "commit", "database"}
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
