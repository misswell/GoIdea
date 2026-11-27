package app

import (
	"context"
	"path/filepath"
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
	uiFont, ok := tt.Find("UI font:")
	if !ok {
		t.Fatalf("no second form label in the sheet: %q", tt.Texts())
	}
	if theme.X != uiFont.X {
		t.Fatalf("form labels use %v and %v as their column origin", theme.X, uiFont.X)
	}
	if delta := theme.Y - tree.Y; delta < -24 || delta > 48 {
		t.Fatalf("the page content sits %.0f DIP away from the page tree: content at y=%.0f, tree at y=%.0f",
			delta, theme.Y, tree.Y)
	}
	if okBtn, ok := tt.Find("OK"); ok && okBtn.Y < theme.Y {
		t.Fatalf("the footer (y=%.0f) is above the page content (y=%.0f)", okBtn.Y, theme.Y)
	}
}

func TestSettingsSearchEscapeClearsBeforeClose(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) { fn() }
	m.OpenSettingsPage(settingsPageAppearance)
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()
	tt.Frame()
	tt.Type("font")
	tt.Frame()
	if m.settingsQuery == "" {
		t.Fatal("the Settings search field did not receive typed text")
	}
	tt.TypeKey(ui.Modifiers(0), ui.KeyEscape, "")
	if !m.settingsOpen {
		t.Fatal("Escape closed Settings instead of clearing its query")
	}
	if m.settingsQuery != "" {
		t.Fatalf("Escape left Settings query as %q", m.settingsQuery)
	}
	tt.Frame()
	tt.TypeKey(ui.Modifiers(0), ui.KeyEscape, "")
	if m.settingsOpen {
		t.Fatal("Escape did not close Settings after clearing an empty query")
	}
}

func TestSettingsBackdropClickDoesNotDismiss(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) { fn() }
	m.OpenSettingsPage(settingsPageAppearance)
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()

	tt.ClickAt(2, 2)
	if !m.settingsOpen {
		t.Fatal("clicking the Settings backdrop dismissed the dialog")
	}
}

func TestSettingsSectionsUseOpenIDEASeparation(t *testing.T) {
	m := &Model{}
	tt := ui.NewTester(func(c *ui.Context) {
		oi := oiPaletteOf(c.Theme())
		m.settingsSectionStarted = false
		m.settingsSectionTitle(c, oi, "First section")
		m.settingsSectionTitle(c, oi, "Second section")
	}, 600, 200)
	tt.Frame()

	first, ok := tt.Find("First section")
	if !ok {
		t.Fatal("first title did not render")
	}
	second, ok := tt.Find("Second section")
	if !ok {
		t.Fatal("second title did not render")
	}
	if got := second.Y - first.Y; got < settingsSectionTitleRow+settingsSectionGap-1 {
		t.Fatalf("second section starts after %v DIPs, want at least %v", got,
			settingsSectionTitleRow+settingsSectionGap)
	}
}

func TestEverySettingsPageRendersAtReferenceSize(t *testing.T) {
	var pages []settingsNode
	var walk func([]settingsNode)
	walk = func(nodes []settingsNode) {
		for _, node := range nodes {
			if node.page != nil {
				pages = append(pages, node)
			}
			walk(node.children)
		}
	}
	walk(settingsTree())
	if len(pages) < 70 {
		t.Fatalf("only %d Settings pages are registered", len(pages))
	}
	for _, node := range pages {
		node := node
		t.Run(node.key, func(t *testing.T) {
			m := &Model{
				ctx:      context.Background(),
				cfg:      Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
				postHook: func(fn func()) { fn() },
			}
			m.OpenSettingsPage(node.key)
			tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
			tt.Frame()
			if !tt.HasText("Settings") {
				t.Fatalf("page %q did not render inside the Settings sheet", node.key)
			}
		})
	}
}
