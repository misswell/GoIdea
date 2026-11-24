package app

import (
	"path/filepath"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestRunRuntimeSettingsFollowTheNativePage(t *testing.T) {
	m := &Model{
		cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		prefs: Preferences{Run: &RunPrefs{Projects: map[string]*RunProjectPrefs{
			"/project": {Selected: "application"},
		}}},
	}
	m.setRoot("/project")
	applied := map[string]any{
		"run.confirmRerun":         false,
		"run.confirmTerminate":     false,
		"run.showOnStart":          false,
		"run.terminationTimeoutMs": float64(0),
	}
	pages := map[string]nativeSettingsPageState{
		settingsPageRunDebug: {
			loaded:  true,
			scope:   "project",
			root:    "/project",
			applied: nativeSettingsCloneValues(applied),
			draft:   nativeSettingsCloneValues(applied),
		},
	}

	if !m.applyRunRuntimeSettings(pages) {
		t.Fatal("Run/Debug runtime settings did not change")
	}
	if m.confirmRerunEnabled() || m.confirmTerminateEnabled() || m.showRunOnStart() {
		t.Fatalf("native run preferences = %+v", m.prefs.Run)
	}
	if got := m.runTerminationTimeout(); got != 0 {
		t.Fatalf("termination timeout = %v, want an explicit zero", got)
	}
	if got := m.prefs.Run.Projects["/project"]; got == nil || got.Selected != "application" {
		t.Fatalf("project run preferences = %+v", got)
	}
	if m.prefs.Run.ConfirmRerun != nil || m.prefs.Run.ConfirmTerminate != nil ||
		m.prefs.Run.ShowOnStart != nil || m.prefs.Run.TerminationTimeout != nil {
		t.Fatalf("project settings leaked into the legacy globals: %+v", m.prefs.Run)
	}
	if m.applyRunRuntimeSettings(pages) {
		t.Fatal("reapplying the same project Run settings reported a change")
	}
}

func TestRunRuntimeSettingsLoadDefaultsWithoutRewritingPreferences(t *testing.T) {
	m := &Model{}
	pages := map[string]nativeSettingsPageState{
		settingsPageRunDebug: {
			loaded: true,
			applied: map[string]any{
				"run.confirmRerun":         true,
				"run.confirmTerminate":     true,
				"run.showOnStart":          true,
				"run.terminationTimeoutMs": float64(3000),
			},
			draft: map[string]any{
				"run.confirmRerun":         true,
				"run.confirmTerminate":     true,
				"run.showOnStart":          true,
				"run.terminationTimeoutMs": float64(3000),
			},
		},
	}
	if !m.applyRunRuntimeSettings(pages) {
		t.Fatal("loaded defaults were not installed in the Run runtime")
	}
	if m.prefs.Run != nil {
		t.Fatalf("unchanged defaults created Run preferences: %+v", m.prefs.Run)
	}
	if m.applyRunRuntimeSettings(pages) {
		t.Fatal("reapplying the same loaded defaults reported a change")
	}
}

func TestRunTerminationTimeoutUsesBackendLimit(t *testing.T) {
	timeout := 12_000
	m := &Model{prefs: Preferences{Run: &RunPrefs{TerminationTimeout: &timeout}}}
	if got := m.runTerminationTimeout(); got != 12*time.Second {
		t.Fatalf("termination timeout = %v, want 12s", got)
	}

	config := openidea.RunConfig{}
	if !m.showRunWindow(&config, true) {
		t.Fatal("default Run window visibility changed")
	}
}
