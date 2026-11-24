package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func completionSettingsModel(values map[string]any) *Model {
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageCodeCompletion: {
			loaded:  true,
			applied: values,
			draft:   values,
		},
	}
	return m
}

func TestCompletionAutoPopupUsesTheStoredSetting(t *testing.T) {
	if !(&Model{}).completionAutoPopup() {
		t.Fatal("completion auto-popup default = false, want true")
	}
	m := completionSettingsModel(map[string]any{"editor.completion.autoPopup": false})
	if m.completionAutoPopup() {
		t.Fatal("completion auto-popup ignored the loaded false value")
	}
}

func TestCompletionSettingsFilterCaseAndDocumentation(t *testing.T) {
	m := completionSettingsModel(map[string]any{
		"editor.completion.matchCase":         true,
		"editor.completion.caseSensitivity":   "first",
		"editor.completion.showDocumentation": false,
	})
	items := m.applyCompletionSettings("U", []editor.Completion{
		{Label: "User", Documentation: "user docs"},
		{Label: "URL", Documentation: "url docs"},
		{Label: "unused", Documentation: "unused docs"},
	})
	if len(items) != 2 || items[0].Label != "User" || items[1].Label != "URL" {
		t.Fatalf("first-letter matching = %+v", items)
	}
	if items[0].Documentation != "user docs" || items[1].Documentation != "url docs" {
		t.Fatalf("documentation was discarded instead of kept for Quick Documentation: %+v", items)
	}

	m = completionSettingsModel(map[string]any{
		"editor.completion.matchCase":       true,
		"editor.completion.caseSensitivity": "all",
	})
	items = m.applyCompletionSettings("UE", []editor.Completion{
		{Label: "UserEntry"},
		{Label: "userEntry"},
		{Label: "UserGuide"},
	})
	if len(items) != 1 || items[0].Label != "UserEntry" {
		t.Fatalf("all-letter case matching = %+v", items)
	}
}

func TestCompletionSettingsSortAlphabetically(t *testing.T) {
	m := completionSettingsModel(map[string]any{
		"editor.completion.sortAlphabetically": true,
	})
	items := m.applyCompletionSettings("", []editor.Completion{
		{Label: "zeta"},
		{Label: "Beta"},
		{Label: "alpha"},
	})
	got := []string{items[0].Label, items[1].Label, items[2].Label}
	want := []string{"alpha", "Beta", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("alphabetical order = %v, want %v", got, want)
		}
	}
}
