package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestIDEOZoomUsesOpenIDEAStopsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	var rendered float32
	tt := ui.NewTester(func(c *ui.Context) {
		m.applyIDEOZoom(c)
		rendered = c.Zoom()
	}, 1280, 800)
	tt.Frame()
	if rendered != 1 {
		t.Fatalf("default IDE zoom = %v, want 1", rendered)
	}
	if !m.RunCommand("view.zoomIn") || m.ideZoomPercent() != 110 {
		t.Fatalf("zoom in: percent=%d", m.ideZoomPercent())
	}
	tt.Frame()
	if rendered != 1.1 {
		t.Fatalf("zoomed frame = %v, want 1.1", rendered)
	}
	if !m.RunCommand("view.zoomIn") || m.ideZoomPercent() != 125 {
		t.Fatalf("second zoom in: percent=%d", m.ideZoomPercent())
	}
	if !m.RunCommand("view.zoomOut") || m.ideZoomPercent() != 110 {
		t.Fatalf("zoom out: percent=%d", m.ideZoomPercent())
	}
	if !m.RunCommand("view.resetZoom") || m.ideZoomPercent() != 100 {
		t.Fatalf("reset zoom: percent=%d", m.ideZoomPercent())
	}
	if got := LoadPreferences(path).Appearance.IDEZoom; got != 100 {
		t.Fatalf("stored IDE zoom = %d, want 100", got)
	}
}

func TestAppearanceToggleActionsChangeTheNativeWorkbench(t *testing.T) {
	m := &Model{}
	m.prefs.MainToolbar = &toolbarLayout{Right: []string{toolbarActionPrefix + "view.services"}}
	tt := workbenchTester(t, m)
	if !tt.HasText("Services") {
		t.Fatalf("the toolbar action is missing before hiding: %q", tt.Texts())
	}
	if !m.ActionToggled("view.toggleToolbar") || !m.ActionToggled("view.toggleStatusBar") ||
		!m.ActionToggled("view.toggleToolWindowBars") {
		t.Fatal("an appearance toggle does not report its visible default")
	}

	if !m.RunCommand("view.toggleToolbar") {
		t.Fatal("view.toggleToolbar is not registered")
	}
	tt.Frame()
	if tt.HasText("Services") || m.ActionToggled("view.toggleToolbar") {
		t.Fatalf("the toolbar remained visible: %q", tt.Texts())
	}

	if !m.RunCommand("view.toggleStatusBar") {
		t.Fatal("view.toggleStatusBar is not registered")
	}
	tt.Frame()
	if tt.HasText("GoIDEA backend not connected") || m.ActionToggled("view.toggleStatusBar") {
		t.Fatalf("the status bar remained visible: %q", tt.Texts())
	}

	project, ok := tt.Find("Project")
	if !ok || project.X < oiStripeWidth {
		t.Fatalf("the Project island is not right of the stripe before hiding: %+v, %v", project, ok)
	}
	if !m.RunCommand("view.toggleToolWindowBars") {
		t.Fatal("view.toggleToolWindowBars is not registered")
	}
	tt.Frame()
	if project, ok = tt.Find("Project"); !ok || project.X >= oiStripeWidth {
		t.Fatalf("the stripe still occupies space after hiding: %+v, %v", project, ok)
	}
}

func TestViewAppearanceAndThemeActionsChangeOffscreenChrome(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: settings}}
	tt := workbenchTester(t, m)

	if !m.RunAction("view.toggleTheme") ||
		!m.RunCommand("view.zoomIn") ||
		!m.RunCommand("view.toggleToolbar") ||
		!m.RunCommand("view.toggleToolWindowBars") ||
		!m.RunCommand("view.toggleStatusBar") {
		t.Fatal("an OpenIDEA view action did not run")
	}
	tt.Frame()
	if m.prefs.Theme != "light" || m.ideZoomPercent() != 110 {
		t.Fatalf("theme/zoom = %q/%d", m.prefs.Theme, m.ideZoomPercent())
	}
	if tt.HasText("Services") || tt.HasText("GoIDEA backend not connected") {
		t.Fatalf("hidden chrome still rendered: %q", tt.Texts())
	}
	if project, ok := tt.Find("Project"); !ok || project.X >= oiStripeWidth {
		t.Fatalf("hidden tool-window bars kept their stripe: %+v, %v", project, ok)
	}
	stored := LoadPreferences(settings)
	if stored.Theme != "light" || stored.Appearance.IDEZoom != 110 ||
		!stored.Appearance.HideMainToolbar ||
		!stored.Appearance.HideToolWindowBars ||
		!stored.Appearance.HideStatusBar {
		t.Fatalf("stored view appearance = %+v theme=%q", stored.Appearance, stored.Theme)
	}
}
