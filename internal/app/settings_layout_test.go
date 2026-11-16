package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSettingsSheetAlignsItsPanes guards the two-column layout: the page tree
// and the page host start at the same height, the search field sits above the
// tree, and the footer stays below both. ui.Row centers its children, so
// without an explicit stretch the short page host floated ~150px below the
// tree (and outside its own hit boxes), which is what made buttons land away
// from their drawings.
func TestSettingsSheetAlignsItsPanes(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) { fn() }
	m.OpenSettingsPage(settingsPageAppearance)
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()

	tree, ok := tt.Find("Appearance & Behavior")
	if !ok {
		t.Fatalf("no page tree in the sheet: %q", tt.Texts())
	}
	theme, ok := tt.Find("Theme:")
	if !ok {
		t.Fatalf("no page content in the sheet: %q", tt.Texts())
	}
	if delta := theme.Y - tree.Y; delta < -24 || delta > 48 {
		t.Fatalf("the page content sits %.0f DIP away from the page tree: content at y=%.0f, tree at y=%.0f",
			delta, theme.Y, tree.Y)
	}
	if search, ok := tt.Find("Search settings"); ok && search.Y+search.H > tree.Y {
		t.Fatalf("the search field (y=%.0f..%.0f) is not above the tree (y=%.0f)",
			search.Y, search.Y+search.H, tree.Y)
	}
	if okBtn, ok := tt.Find("OK"); ok && okBtn.Y < theme.Y {
		t.Fatalf("the footer (y=%.0f) is above the page content (y=%.0f)", okBtn.Y, theme.Y)
	}
}
