package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestLoadPreferencesDefaults(t *testing.T) {
	if p := LoadPreferences(filepath.Join(t.TempDir(), "missing.json")); !reflect.DeepEqual(p, Preferences{}) {
		t.Fatalf("missing file = %+v, want defaults", p)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := LoadPreferences(bad); !reflect.DeepEqual(p, Preferences{}) {
		t.Fatalf("broken file = %+v, want defaults", p)
	}
}

func TestPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	want := Preferences{Theme: "dark", FontSize: 17}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := LoadPreferences(path); !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadPreferences = %+v, want %+v", got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		t.Fatalf("directory holds %v, want only settings.json", entries)
	}
}

func TestThemePreferenceFollowsChoice(t *testing.T) {
	if th := (Preferences{}).themePreference(); th != nil {
		t.Fatalf("unset theme = %+v, want system", th)
	}
	if th := (Preferences{Theme: "system"}).themePreference(); th != nil {
		t.Fatalf("system theme = %+v, want nil", th)
	}
	light := (Preferences{Theme: "light"}).themePreference()
	dark := (Preferences{Theme: "dark"}).themePreference()
	if light == nil || dark == nil {
		t.Fatalf("light = %v, dark = %v, want both set", light, dark)
	}
	if light.Dark || !dark.Dark || light.Background == dark.Background {
		t.Fatalf("themes do not differ: light=%+v dark=%+v", light, dark)
	}
}

func TestFontSizePrefDefaults(t *testing.T) {
	if got := (Preferences{}).fontSize(); got != defaultFontSize {
		t.Fatalf("unset font size = %v, want %v", got, defaultFontSize)
	}
	if got := (Preferences{FontSize: 16}).fontSize(); got != 16 {
		t.Fatalf("stored font size = %v, want 16", got)
	}
}

func TestStepFontClamps(t *testing.T) {
	tab := &Tab{state: &editor.State{}}
	m := &Model{tabs: []*Tab{tab}, prefs: Preferences{FontSize: maxFontSize}}
	m.applyPreferences()
	m.stepFont(5)
	if m.prefs.FontSize != maxFontSize {
		t.Fatalf("size above the range = %v, want %d", m.prefs.FontSize, maxFontSize)
	}
	if tab.state.Font.Size != maxFontSize {
		t.Fatalf("editor font = %v, want %d", tab.state.Font.Size, maxFontSize)
	}
	m.stepFont(-5)
	if m.prefs.FontSize != maxFontSize-5 {
		t.Fatalf("size after lower step = %v, want %d", m.prefs.FontSize, maxFontSize-5)
	}
	m.prefs.FontSize = minFontSize
	m.stepFont(-5)
	if m.prefs.FontSize != minFontSize {
		t.Fatalf("size below the range = %v, want %d", m.prefs.FontSize, minFontSize)
	}
	m.prefs.FontSize = 0
	m.stepFont(1)
	if m.prefs.FontSize != defaultFontSize+1 {
		t.Fatalf("size from the default = %v, want %d", m.prefs.FontSize, defaultFontSize+1)
	}
}

func TestApplyPreferencesPicksTheme(t *testing.T) {
	m := &Model{prefs: Preferences{Theme: "dark"}}
	m.applyPreferences()
	if m.theme == nil || !m.theme.Dark {
		t.Fatalf("theme = %+v, want dark", m.theme)
	}
	m.prefs.Theme = "system"
	m.applyPreferences()
	if m.theme != nil {
		t.Fatalf("theme = %+v, want system", m.theme)
	}
}
