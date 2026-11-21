package app

import "testing"

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
	if got := entries[0].sub[0].label; got != "Theme default" {
		t.Fatalf("color scheme = %q", got)
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
