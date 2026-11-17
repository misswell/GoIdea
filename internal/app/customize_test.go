package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAccentReachesThemeAndIslands(t *testing.T) {
	m := &Model{prefs: Preferences{Theme: "dark"}}
	m.SetAccentPref("green")
	if m.prefs.Accent != "green" {
		t.Fatalf("accent = %q", m.prefs.Accent)
	}
	want, _ := accentColorOf("green", true)
	if m.theme == nil || m.theme.Accent != want {
		t.Fatalf("theme accent = %+v, want %+v", m.theme, want)
	}
	if got := oiPaletteOf(m.theme).accent; got != want {
		t.Fatalf("islands accent = %+v, want %+v", got, want)
	}
	// Blue is the stored default and clears the override.
	m.SetAccentPref("")
	if m.prefs.Accent != "" {
		t.Fatalf("accent = %q", m.prefs.Accent)
	}
	if m.theme.Accent != uiBlueAccent() {
		t.Fatalf("the default accent did not come back")
	}
}

func uiBlueAccent() ui.Color { return openideaColors(true).accent }

func TestAccentRejectsUnknownIDs(t *testing.T) {
	m := &Model{}
	m.SetAccentPref("magenta")
	if m.prefs.Accent != "" {
		t.Fatalf("an unknown accent was stored: %q", m.prefs.Accent)
	}
}

func TestAccentAndStartupPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.SetAccentPref("purple")
	m.SetStartupPref("reopen")
	got := LoadPreferences(path)
	if got.Accent != "purple" || got.Startup != "reopen" {
		t.Fatalf("stored = %+v", got)
	}
	m.SetStartupPref("welcome")
	got = LoadPreferences(path)
	if got.Startup != "welcome" {
		t.Fatalf("startup = %q", got.Startup)
	}
}

func TestStartupIsWelcomeUnlessReopenChosen(t *testing.T) {
	m := &Model{}
	if m.startupReopensProject() {
		t.Fatal("an untouched start reopens the last project")
	}
	m.SetStartupPref("reopen")
	if !m.startupReopensProject() {
		t.Fatal("the reopen choice is not read")
	}
	m.SetStartupPref("welcome")
	if m.startupReopensProject() {
		t.Fatal("the Welcome choice still reopens the last project")
	}
	m.SetStartupPref("nonsense")
	if m.prefs.Startup != "welcome" {
		t.Fatalf("an unknown choice was stored: %q", m.prefs.Startup)
	}
}

func TestKeymapPresetLabel(t *testing.T) {
	if keymapPreset() != keymapPresetTitle {
		t.Fatalf("preset = %q", keymapPreset())
	}
}
