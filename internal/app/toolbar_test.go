package app

import (
	"path/filepath"
	"testing"
)

func TestToolbarResolveAddsWidgetsRegisteredSince(t *testing.T) {
	saved := &toolbarLayout{Left: []string{toolbarProject}, Hidden: []string{toolbarVCS}}
	left, right := resolveToolbar(saved)
	wantLeft := []string{toolbarProject}
	if len(left) != len(wantLeft) || left[0] != wantLeft[0] {
		t.Fatalf("left = %v, want %v (the hidden VCS widget stays out)", left, wantLeft)
	}
	wantRight := []string{toolbarRun, toolbarSearch, toolbarTerminal}
	if len(right) != len(wantRight) {
		t.Fatalf("right = %v, want %v", right, wantRight)
	}
	for i, id := range wantRight {
		if right[i] != id {
			t.Fatalf("right = %v, want %v", right, wantRight)
		}
	}
}

func TestToolbarToggleHidesWidgetAndShowsItAgain(t *testing.T) {
	layout := editableToolbar(nil)

	layout = toggleToolbarEntry(layout, toolbarVCS)
	if toolbarShowsEntry(layout, toolbarVCS) {
		t.Fatal("VCS widget still shown after toggling it off")
	}
	if !contains(layout.Hidden, toolbarVCS) {
		t.Fatalf("hidden = %v, want the removed widget", layout.Hidden)
	}

	layout = toggleToolbarEntry(layout, toolbarVCS)
	if !toolbarShowsEntry(layout, toolbarVCS) {
		t.Fatal("VCS widget not shown after toggling it back")
	}
	if contains(layout.Hidden, toolbarVCS) {
		t.Fatalf("hidden = %v, want the widget forgotten", layout.Hidden)
	}
}

func TestToolbarToggleDoesNotRememberHiddenActions(t *testing.T) {
	layout := toggleToolbarEntry(editableToolbar(nil), toolbarTerminal)
	if toolbarShowsEntry(layout, toolbarTerminal) {
		t.Fatal("terminal still shown after toggling it off")
	}
	if len(layout.Hidden) != 0 {
		t.Fatalf("hidden = %v, want actions not remembered as hidden", layout.Hidden)
	}
}

func toolbarShowsEntry(layout toolbarLayout, id string) bool {
	for _, x := range append(append([]string{}, layout.Left...), layout.Right...) {
		if x == id {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestToolbarLayoutPersistsInPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}

	m.toggleToolbarWidget(toolbarRun)
	raw := m.prefs.MainToolbar
	if raw == nil || toolbarShowsEntry(editableToolbar(raw), toolbarRun) {
		t.Fatal("the Run widget should be off and saved")
	}
	stored := LoadPreferences(path)
	if stored.MainToolbar == nil || toolbarShowsEntry(editableToolbar(stored.MainToolbar), toolbarRun) {
		t.Fatal("the saved Run widget removal did not read back")
	}

	m.RestoreDefaultToolbar()
	if m.prefs.MainToolbar != nil {
		t.Fatalf("MainToolbar = %+v, want nil after restore", m.prefs.MainToolbar)
	}
	if stored = LoadPreferences(path); stored.MainToolbar != nil {
		t.Fatalf("stored MainToolbar = %+v, want nil after restore", stored.MainToolbar)
	}
}

func TestTitleBarHidesARemovedWidget(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())

	layout := toggleToolbarEntry(editableToolbar(nil), toolbarRun)
	m.prefs.MainToolbar = &layout
	tt := workbenchTester(t, m)
	if tt.HasText("Select Configuration") {
		t.Fatal("the Run widget still draws after it was removed")
	}

	m.RestoreDefaultToolbar()
	tt = workbenchTester(t, m)
	if !tt.HasText("Select Configuration") {
		t.Fatal("the Run widget is missing after Restore Default Toolbar")
	}
}
