package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// pluginFixture writes one installed plugin under the data folder of path,
// and returns its folder.
func pluginFixture(t *testing.T, settingsPath, name string) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(settingsPath), "plugins", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"name":        name,
		"displayName": "Demo Plugin",
		"version":     "1.2.3",
		"description": "Adds things to the editor.",
		"author":      "Tester",
		"contributes": map[string]any{
			"commands": []map[string]any{{"id": name + ".run", "title": "Demo: Run"}},
			"languages": []map[string]any{{
				"id": "demo", "extensions": []string{".demo"},
			}},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPluginPageListsInstalledPlugins(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	pluginFixture(t, settingsPath, "demo")
	m := &Model{cfg: Config{SettingsPath: settingsPath}, welcomePage: "plugins"}
	tt := welcomeTester(t, m, true)
	for _, text := range []string{
		"Downloaded", "Demo Plugin", "Tester · 1.2.3 · Local",
		"Adds things to the editor.", "Demo: Run", "demo",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not on the plugins page: %q", text, tt.Texts())
		}
	}
}

func TestPluginEnablePersists(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	pluginFixture(t, settingsPath, "demo")
	m := &Model{cfg: Config{SettingsPath: settingsPath}, welcomePage: "plugins"}
	pg := m.pluginsPage()
	p := pg.reg.Plugin("demo")
	if p == nil {
		t.Fatal("the fixture plugin is not loaded")
	}
	if !m.pluginIsEnabled(p) {
		t.Fatal("a plugin with no setting should follow its manifest default")
	}
	m.setPluginEnabled("demo", false)
	if m.pluginIsEnabled(p) {
		t.Fatal("the plugin is still enabled after the checkbox turned it off")
	}
	saved := LoadPreferences(settingsPath)
	var s struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(saved.Plugins["demo"], &s); err != nil || s.Enabled == nil || *s.Enabled {
		t.Fatalf("plugins.demo = %s, %v, want enabled false", saved.Plugins["demo"], err)
	}
	tt := welcomeTester(t, m, true)
	if !tt.HasText("Disabled") {
		t.Fatalf("the off plugin does not say it is disabled: %q", tt.Texts())
	}
}

func TestPluginRemoveDeletesFolderAndSetting(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	dir := pluginFixture(t, settingsPath, "demo")
	m := &Model{cfg: Config{SettingsPath: settingsPath}, welcomePage: "plugins"}
	m.pluginsPage()
	m.setPluginEnabled("demo", false)
	m.removePlugin("demo")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the plugin folder is still there: %v", err)
	}
	if _, ok := LoadPreferences(settingsPath).Plugins["demo"]; ok {
		t.Fatal("the removed plugin kept its setting")
	}
	tt := welcomeTester(t, m, true)
	if !tt.HasText("No plugins installed.") {
		t.Fatalf("the page does not show the empty state: %q", tt.Texts())
	}
}
