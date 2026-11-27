package app

import (
	"path/filepath"
	"testing"
)

func TestQuickSwitchSchemeHasOpenIDEAGroups(t *testing.T) {
	entries := quickSwitchMenuEntries(&Model{})
	var labels []string
	for _, entry := range entries {
		labels = append(labels, entry.label)
	}
	want := []string{"Editor Color Scheme", "Keymap", "View Mode", "Theme"}
	if len(labels) != len(want) {
		t.Fatalf("Quick Switch groups = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("group %d = %q, want %q", i, labels[i], want[i])
		}
	}
	if got := entries[0].sub[0].label; got != "Islands Light" {
		t.Fatalf("color scheme = %q", got)
	}
	if got := entries[0].sub[0].command; got != "editor.colorScheme.light" {
		t.Fatalf("color scheme command = %q", got)
	}
	if got := entries[1].sub[0].label; got != "macOS" {
		t.Fatalf("keymap = %q", got)
	}
	if got := entries[2].sub[0].command; got != "editor.toggleZen" {
		t.Fatalf("view mode command = %q", got)
	}
	if got := entries[3].sub[1].command; got != "app.themeDark" {
		t.Fatalf("theme command = %q", got)
	}
}

func TestQuickSwitchWithoutWindowDoesNotPanic(t *testing.T) {
	m := &Model{}
	m.QuickSwitchScheme()
}

func TestQuickSwitchColorSchemesUseTheNativeRuntime(t *testing.T) {
	m := &Model{
		cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		prefs: Preferences{EditorScheme: EditorSchemePrefs{
			CustomSchemes: map[string]CustomColorScheme{
				"custom-harbor": {Name: "Harbor & Moss", Parent: "dark"},
			},
		}},
	}
	entries := quickSwitchMenuEntries(m)
	schemes := entries[0].sub
	if len(schemes) != len(knownColorSchemeChoices())+1 {
		t.Fatalf("Quick Switch schemes = %+v", schemes)
	}
	var harbor *menuEntry
	for i := range schemes {
		if schemes[i].label == "Harbor & Moss" {
			harbor = &schemes[i]
			break
		}
	}
	if harbor == nil || harbor.command != "editor.colorScheme.custom-harbor" {
		t.Fatalf("custom Quick Switch scheme = %+v", harbor)
	}
	if !m.RunCommandWithArg(harbor.command, nil) {
		t.Fatal("Quick Switch custom scheme did not run")
	}
	if m.prefs.EditorScheme.ColorScheme != "custom-harbor" {
		t.Fatalf("Quick Switch selected %q", m.prefs.EditorScheme.ColorScheme)
	}
	entries = quickSwitchMenuEntries(m)
	for _, scheme := range entries[0].sub {
		if scheme.command == harbor.command && (scheme.check == nil || !scheme.check(m)) {
			t.Fatalf("selected custom Quick Switch scheme is not checked: %+v", scheme)
		}
	}
}
