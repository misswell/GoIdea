package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestToolWindowStripeNamesFollowTheAppearanceSetting(t *testing.T) {
	m := &Model{}
	render := func() *ui.Tester {
		tt := ui.NewTester(func(c *ui.Context) {
			m.toolStripe(c, c.Theme(), oiPaletteOf(c.Theme()))
		}, 100, 620)
		tt.Frame()
		return tt
	}

	iconsOnly := render()
	if iconsOnly.HasText("Project") || iconsOnly.HasText("Commit") {
		t.Fatalf("icons-only stripe rendered names: %v", iconsOnly.Texts())
	}

	m.prefs.Appearance.ShowToolWindowNames = true
	named := render()
	for _, name := range []string{"Project", "Commit", "Structure", "Bookmarks"} {
		if !named.HasText(name) {
			t.Fatalf("%q is not under its stripe icon: %v", name, named.Texts())
		}
	}
	if r, ok := named.Find("Project"); !ok || r.X < 0 || r.X+r.W > oiStripeNamedBar {
		t.Fatalf("named stripe label bounds = %+v, %v", r, ok)
	}
}

func TestRightToolWindowStripeNamesAndContextMenuFollowTheSetting(t *testing.T) {
	m := &Model{cfg: Config{SettingsPath: t.TempDir() + "/settings.json"}}
	renderStripe := func() *ui.Tester {
		tt := ui.NewTester(func(c *ui.Context) {
			m.rightStripe(c, c.Theme(), oiPaletteOf(c.Theme()))
		}, int(oiStripeNamedBar), 620)
		tt.Frame()
		return tt
	}

	iconsOnly := renderStripe()
	if iconsOnly.HasText("Hierarchy") || iconsOnly.HasText("Notifications") {
		t.Fatalf("icons-only right stripe rendered names: %v", iconsOnly.Texts())
	}

	m.prefs.Appearance.ShowToolWindowNames = true
	named := renderStripe()
	for _, name := range []string{"Hierarchy", "Notifications"} {
		if !named.HasText(name) {
			t.Fatalf("%q is not under its right stripe icon: %v", name, named.Texts())
		}
		if r, ok := named.Find(name); !ok || r.X < 0 || r.X+r.W > oiStripeNamedBar {
			t.Fatalf("named right stripe label %q bounds = %+v, %v", name, r, ok)
		}
	}

	if err := named.RightClick("Hierarchy"); err != nil {
		t.Fatal(err)
	}
	if err := named.ChooseMenuItem("Show Tool Window Names"); err != nil {
		t.Fatal(err)
	}
	if m.prefs.Appearance.ShowToolWindowNames {
		t.Fatal("the right stripe context menu did not hide tool window names")
	}
	if stored := LoadPreferences(m.cfg.SettingsPath); stored.Appearance.ShowToolWindowNames {
		t.Fatal("the right stripe name setting was not persisted")
	}
}
