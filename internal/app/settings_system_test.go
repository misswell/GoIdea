package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSystemPrefsUseOpenIDEADefaultsAndBounds(t *testing.T) {
	p := Preferences{}
	if !p.reopenLastProject() || !p.System.autoSave() || !p.System.syncOnFocus() ||
		!p.System.confirmExit() || p.System.openProjectIn() != "ask" ||
		p.System.idleSeconds() != 15 || p.System.localHistoryDays() != 5 ||
		p.System.localHistoryMaxMB() != 512 {
		t.Fatalf("System Settings defaults = %+v", p.System)
	}
	p.System = SystemPrefs{
		OpenProjectIn:     "attach",
		IdleSeconds:       9000,
		LocalHistoryDays:  999,
		LocalHistoryMaxMB: 1,
	}
	if p.System.openProjectIn() != "ask" ||
		p.System.idleSeconds() != 3600 ||
		p.System.localHistoryDays() != 365 ||
		p.System.localHistoryMaxMB() != 16 {
		t.Fatalf("normalized System Settings = %+v", p.System)
	}
}

func TestOpenProjectPolicyUsesThreeChoices(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
		want string
	}{
		{name: "ask", mode: "ask", want: "ask"},
		{name: "new", mode: "new", want: "new"},
		{name: "same", mode: "same", want: "same"},
		{name: "empty defaults to ask", mode: "", want: "ask"},
		{name: "unknown defaults to ask", mode: "attach", want: "ask"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := openProjectWindowPolicy(tc.mode); got != tc.want {
				t.Fatalf("policy = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSystemSettingsStageApplyAndWriteBackendHistoryLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	calls := make(chan map[string]any, 32)
	m := &Model{
		ctx: context.Background(),
		cfg: Config{SettingsPath: path},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command == "settings_set" {
				if values, ok := args.(map[string]any); ok {
					calls <- values
				}
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.OpenSettingsPage(settingsPageSystem)
	m.settingsDraft.reopenLast = false
	m.settingsDraft.openProjectIn = "same"
	m.settingsDraft.defaultProjectDirectory = t.TempDir()
	m.settingsDraft.autoSave = false
	m.settingsDraft.syncOnFocus = false
	m.settingsDraft.saveOnIdle = true
	m.settingsDraft.idleSeconds = 45
	m.settingsDraft.localHistoryDays = 9
	m.settingsDraft.localHistoryMaxMB = 1024
	m.settingsDraft.confirmExit = false
	m.settingsDraft.closeWithoutWelcome = true
	m.settingsDraft.closeTabsWithWindow = true

	if !m.settingsDirtyPage(settingsPageSystem) {
		t.Fatal("System Settings changes did not dirty the page")
	}
	m.applySettings()

	stored := LoadPreferences(path)
	if stored.reopenLastProject() ||
		stored.System.openProjectIn() != "same" ||
		stored.System.defaultProjectDirectory() != m.settingsDraft.defaultProjectDirectory ||
		stored.System.autoSave() ||
		stored.System.syncOnFocus() ||
		!stored.System.SaveOnIdle ||
		stored.System.idleSeconds() != 45 ||
		stored.System.localHistoryDays() != 9 ||
		stored.System.localHistoryMaxMB() != 1024 ||
		stored.System.confirmExit() ||
		!stored.System.CloseWithoutWelcome ||
		!stored.System.CloseTabsWithWindow {
		t.Fatalf("stored System Settings = %+v, startup = %q", stored.System, stored.Startup)
	}

	want := map[string]string{
		"files.localHistoryDays":  "9",
		"files.localHistoryMaxMb": "1024",
	}
	deadline := time.After(3 * time.Second)
	for len(want) > 0 {
		select {
		case call := <-calls:
			key, _ := call["key"].(string)
			if expected, ok := want[key]; ok {
				if call["scope"] != "global" || call["value"] != expected {
					t.Fatalf("%s settings_set = %#v", key, call)
				}
				delete(want, key)
			}
		case <-deadline:
			t.Fatalf("missing backend writes: %#v", want)
		}
	}
}

func TestLocalDataDirectoryBootstrapMigrationAndStaging(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "old")
	newDir := filepath.Join(root, "new")
	configPath := filepath.Join(root, "config", "local-data-dir.json")
	if err := os.MkdirAll(filepath.Join(oldDir, "openidea-server", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "openidea-server", "data", "index.bin"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{cfg: Config{
		DataDir: oldDir, DefaultDataDir: filepath.Join(root, "default"),
		SettingsPath:           filepath.Join(oldDir, "settings.json"),
		LocalDataDirConfigPath: configPath,
	}}
	m.OpenSettingsPage(settingsPageSystem)
	m.settingsDraft.localDataDir = newDir
	m.settingsDraft.localDataMove = true
	if !m.settingsDirtyPage(settingsPageSystem) {
		t.Fatal("a changed local data directory did not dirty System Settings")
	}
	if err := m.applyLocalDataDirectory(newDir, true); err != nil {
		t.Fatal(err)
	}
	if custom, err := LoadCustomLocalDataDir(configPath); err != nil || custom != newDir {
		t.Fatalf("stored custom root = %q, %v", custom, err)
	}
	for _, rel := range []string{"settings.json", filepath.Join("openidea-server", "data", "index.bin")} {
		if _, err := os.Stat(filepath.Join(newDir, rel)); err != nil {
			t.Fatalf("migrated %s: %v", rel, err)
		}
	}
	if !m.restartPending {
		t.Fatal("a changed data directory did not schedule a restart")
	}
}

func TestSettingsSystemRendersOpenIDEAOptions(t *testing.T) {
	m := &Model{
		ctx:      context.Background(),
		cfg:      Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		postHook: func(fn func()) { fn() },
	}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageSystem)

	tester := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tester.Frame()
	for _, label := range []string{
		"Reopen projects on startup",
		"Open project in:",
		"New window",
		"Current window",
		"Ask",
		"Default directory:",
		"Local data directory:",
		"Save files if the IDE is idle for",
		"Save files when switching to a different application or a built-in terminal",
		"Synchronize external changes when switching to the IDE window",
		"Keep local history for:",
		"Maximum local history size per project:",
		"Confirm before exiting the IDE",
		"Close the window instead of showing the Welcome screen",
		"Close all editor tabs when the window closes",
	} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("System Settings did not render %q", label)
		}
	}
}

func TestRunningWorkCountIncludesRunDebugServicesAndTerminals(t *testing.T) {
	m := &Model{}
	m.run.running = true
	m.debug.session = 7
	m.services.children = []*runChild{{running: true}, {running: false}}
	m.term.groups = []termGroup{{tabs: []termSession{{}, {}}}}
	if got := m.runningWorkCount(); got != 5 {
		t.Fatalf("running work = %d, want 5", got)
	}
}

func TestSessionWithoutEditorTabsKeepsProjectAndRecents(t *testing.T) {
	s := Session{
		Root:              "/project",
		Files:             []string{"/project/a.go", "/project/b.go"},
		Focus:             "/project/a.go",
		Pinned:            []string{"/project/a.go"},
		PreviewFiles:      []string{"/project/b.go"},
		SplitFiles:        []string{"/project/b.go"},
		SplitPreviewFiles: []string{"/project/b.go"},
		SplitActive:       1,
		SplitFirst:        true,
		RecentProjects:    []string{"/project"},
	}
	got := sessionWithoutEditorTabs(s)
	if got.Root != "/project" || len(got.RecentProjects) != 1 ||
		len(got.Files) != 0 || got.Focus != "" || len(got.Pinned) != 0 ||
		len(got.PreviewFiles) != 0 || len(got.SplitFiles) != 0 ||
		got.SplitActive != 0 || got.SplitFirst || len(got.SplitPreviewFiles) != 0 {
		t.Fatalf("session without editor tabs = %+v", got)
	}
}
