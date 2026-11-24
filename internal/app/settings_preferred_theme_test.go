package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPreferredThemesResolveEachSystemAppearance(t *testing.T) {
	p := Preferences{
		PreferredLightTheme: "dark",
		PreferredDarkTheme:  "light",
	}
	if theme := p.systemThemeWithAccent(false); theme == nil || !theme.Dark {
		t.Fatalf("light OS mode did not use the preferred dark theme: %+v", theme)
	}
	if theme := p.systemThemeWithAccent(true); theme == nil || theme.Dark {
		t.Fatalf("dark OS mode did not use the preferred light theme: %+v", theme)
	}

	defaults := Preferences{PreferredLightTheme: "obsolete"}
	if got := defaults.preferredThemeName(false); got != "light" {
		t.Fatalf("invalid light preference = %q, want light", got)
	}
	if got := defaults.preferredThemeName(true); got != "dark" {
		t.Fatalf("invalid dark preference = %q, want dark", got)
	}
}

func TestPreferredThemesStageApplyAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg:        Config{SettingsPath: path},
		systemDark: true,
		prefs:      Preferences{Theme: ""},
	}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageAppearance)

	m.settingsSetPreferredTheme(false, "dark")
	m.settingsSetPreferredTheme(true, "light")
	if !m.settingsDirtyPage(settingsPageAppearance) {
		t.Fatal("preferred themes did not dirty Appearance")
	}
	if m.prefs.PreferredLightTheme != "" || m.prefs.PreferredDarkTheme != "" {
		t.Fatalf("staged preferences leaked into the store: %+v", m.prefs)
	}

	m.settingsPreview()
	if m.theme == nil || m.theme.Dark {
		t.Fatalf("dark OS mode did not preview the preferred light theme: %+v", m.theme)
	}

	m.applySettings()
	stored := LoadPreferences(path)
	if stored.PreferredLightTheme != "dark" ||
		stored.PreferredDarkTheme != "light" {
		t.Fatalf("stored preferred themes = %+v", stored)
	}

	m.settingsSelectPage(settingsPageAppearance)
	m.settingsResetPage()
	if m.settingsDraft.preferredLightTheme != "light" ||
		m.settingsDraft.preferredDarkTheme != "dark" {
		t.Fatalf("reset preferred themes = %+v", m.settingsDraft)
	}
}

func TestPreferredThemesPopoverRendersNativeControls(t *testing.T) {
	m := &Model{ctx: context.Background(), systemDark: false}
	m.postHook = func(fn func()) { fn() }
	m.OpenSettingsPage(settingsPageAppearance)
	m.settingsThemePrefs = true

	tester := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tester.Frame()
	for _, label := range []string{
		"Preferred theme for light OS mode:",
		"Preferred theme for dark OS mode:",
	} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("preferred themes popover did not render %q: %q", label, tester.Texts())
		}
	}
}
