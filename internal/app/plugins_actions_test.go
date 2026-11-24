package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginActionsInstallFromDiskAndReload(t *testing.T) {
	for _, id := range []string{"plugins.installFromDisk", "plugins.updateAll", "plugins.reloadAll"} {
		if _, ok := actionGet(id); !ok {
			t.Fatalf("%s is not registered", id)
		}
	}

	sourceSettings := filepath.Join(t.TempDir(), "source-settings.json")
	source := pluginFixture(t, sourceSettings, "disk-demo")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg:         Config{SettingsPath: settingsPath},
		welcomePage: "plugins",
		postHook:    func(fn func()) { fn() },
	}
	if !m.RunActionWithArg("plugins.installFromDisk", map[string]string{"path": source}) {
		t.Fatal("Install Plugin from Disk did not run")
	}
	pg := m.pluginsPage()
	if pg.reg.Plugin("disk-demo") == nil {
		t.Fatalf("the linked plugin is not loaded: %q", pg.err)
	}
	target := filepath.Join(filepath.Dir(settingsPath), "plugins", "disk-demo")
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("installed plugin is not linked from disk: %v, %v", info, err)
	}

	manifestPath := filepath.Join(source, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["displayName"] = "Reloaded Plugin"
	data, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.RunAction("plugins.reloadAll") {
		t.Fatal("Reload Plugins did not run")
	}
	if got := pg.reg.Plugin("disk-demo").Title(); got != "Reloaded Plugin" {
		t.Fatalf("reloaded title = %q", got)
	}
	if !m.RunAction("plugins.updateAll") || m.status != "No Git-installed plugins to update" {
		t.Fatalf("Update All status = %q", m.status)
	}
}
