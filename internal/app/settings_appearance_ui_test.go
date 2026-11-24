package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsAppearanceUIChoicesStageApplyAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageAppearance)

	m.settingsDraft.accent = "purple"
	m.settingsDraft.ideZoom = 125
	m.settingsDraft.showMainToolbar = false
	m.settingsDraft.showToolWindowBars = false
	m.settingsDraft.showToolWindowNames = true
	m.settingsDraft.showStatusBar = false
	m.settingsDraft.compactMode = true
	m.settingsDraft.navigationBar = "top"

	if m.prefs.Accent != "" || m.prefs.Appearance.IDEZoom != 0 ||
		m.prefs.Appearance.HideMainToolbar ||
		m.prefs.Appearance.HideToolWindowBars ||
		m.prefs.Appearance.ShowToolWindowNames ||
		m.prefs.Appearance.HideStatusBar ||
		m.prefs.Appearance.CompactMode ||
		m.prefs.Appearance.NavigationBar != "" {
		t.Fatalf("staged appearance leaked into preferences: %+v", m.prefs.Appearance)
	}
	if !m.settingsDirtyPage(settingsPageAppearance) {
		t.Fatal("appearance choices did not dirty the page")
	}

	m.applySettings()
	stored := LoadPreferences(path)
	if stored.Accent != "purple" ||
		stored.Appearance.IDEZoom != 125 ||
		!stored.Appearance.HideMainToolbar ||
		!stored.Appearance.HideToolWindowBars ||
		!stored.Appearance.ShowToolWindowNames ||
		!stored.Appearance.HideStatusBar ||
		!stored.Appearance.CompactMode ||
		stored.Appearance.NavigationBar != "top" {
		t.Fatalf("stored appearance = %+v, accent = %q", stored.Appearance, stored.Accent)
	}

	m.settingsSelectPage(settingsPageAppearance)
	m.settingsResetPage()
	if m.settingsDraft.accent != "" ||
		m.settingsDraft.ideZoom != 100 ||
		!m.settingsDraft.showMainToolbar ||
		!m.settingsDraft.showToolWindowBars ||
		m.settingsDraft.showToolWindowNames ||
		!m.settingsDraft.showStatusBar ||
		m.settingsDraft.compactMode ||
		m.settingsDraft.navigationBar != "statusBar" {
		t.Fatalf("Reset appearance = %+v", m.settingsDraft)
	}
}

func TestSettingsAppearanceRendersUIChoices(t *testing.T) {
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	m.postHook = func(fn func()) { fn() }
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageAppearance)

	tester := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tester.Frame()
	for _, label := range []string{
		"Accent color:",
		"Zoom:",
		"Show main toolbar",
		"Show status bar",
		"Navigation bar:",
		"Compact Mode",
		"Show tool window bars",
		"Show tool window names",
		"Show tool window badges",
		"Use custom font",
		"UI font:",
		"Fallback font:",
		"Use contrast scrollbars",
		"Adjust colors for red-green vision deficiency",
		"Always show full path in window header",
		"Use project colors in main toolbar",
		"Different tool window background",
		"Drag-and-drop with Alt pressed only",
		"Use smaller indents in trees",
		"Display icons in menu items",
		"Open main menu with a lone Alt press",
		"Main menu:",
		"Antialiasing",
		"Widescreen tool window layout",
	} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("Appearance did not render %q", label)
		}
	}
}
