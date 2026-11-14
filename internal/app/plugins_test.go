package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mayoi/internal/plugin"
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
			"views":    []map[string]any{{"id": "demo.side", "title": "Demo View", "location": "sidebar"}},
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
	// The contributed command is a palette action, and the plugin's switch
	// takes it out and puts it back.
	id := pluginActionPrefix + "demo.run"
	if _, ok := Command(id); !ok {
		t.Fatalf("the plugin command is not in the palette: %v", actionList())
	}
	if got := commandTitle(id); got != "Demo: Run" {
		t.Fatalf("the action title is %q", got)
	}
	m.setPluginEnabled("demo", false)
	if _, ok := Command(id); ok {
		t.Fatal("a disabled plugin's command stayed in the palette")
	}
	m.setPluginEnabled("demo", true)
	if _, ok := Command(id); !ok {
		t.Fatal("re-enabling the plugin did not bring its command back")
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

func TestPluginHostServesTheWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	m.setRoot(root)
	h := &goideaPluginHost{m: m}
	if v, err := h.Call(nil, "workspace.root", nil); err != nil || v != root {
		t.Fatalf("workspace.root = %v, %v", v, err)
	}
	if v, err := h.Call(nil, "workspace.isRemote", nil); err != nil || v != false {
		t.Fatalf("workspace.isRemote = %v, %v", v, err)
	}
	v, err := h.Call(nil, "workspace.readFile", json.RawMessage(`{"path":"a.txt"}`))
	if err != nil || v != "hi" {
		t.Fatalf("workspace.readFile = %v, %v", v, err)
	}
	if _, err := h.Call(nil, "workspace.readFile", json.RawMessage(`{"path":"../escape.txt"}`)); err == nil {
		t.Fatal("a path outside the project was read")
	}
	stat, err := h.Call(nil, "workspace.stat", json.RawMessage(`{"path":"a.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := stat.(map[string]any)["type"]; got != "file" {
		t.Fatalf("workspace.stat type = %v", got)
	}
	if _, err := h.Call(nil, "editor.active", nil); err == nil {
		t.Fatal("an unsupported API answered without an error")
	}
}

func TestPluginStatusItemJoinsTheStatusBar(t *testing.T) {
	m := &Model{}
	h := &goideaPluginHost{m: m}
	p := &plugin.Plugin{Name: "demo"}
	set := func(item string) {
		t.Helper()
		if _, err := h.Call(p, "status.set", json.RawMessage(item)); err != nil {
			t.Fatal(err)
		}
	}
	const id = "plugin.status:demo:count"
	set(`{"id":"count","item":{"text":"3 files","tooltip":"Files","command":"view.problems"}}`)
	if indexOf(statusIDs(m.statusSide("right")), id) == -1 {
		t.Fatal("the plugin's status item is not on the bar")
	}
	r, ok := workbenchTester(t, m).Find("3 files")
	if !ok || r.Y < 900-oiStatusBarHeight {
		t.Fatalf("the plugin status is not in the status bar: %+v, %v", r, ok)
	}
	set(`{"id":"count","item":{"text":"4 files"}}`)
	if _, ok := workbenchTester(t, m).Find("4 files"); !ok {
		t.Fatal("updating the item did not redraw it")
	}
	set(`{"id":"count","item":null}`)
	if indexOf(statusIDs(m.statusSide("right")), id) != -1 {
		t.Fatal("hiding the item left it on the bar")
	}
	set(`{"id":"count","item":{"text":"5 files"}}`)
	h.clearStatus("demo")
	if indexOf(statusIDs(m.statusSide("right")), id) != -1 {
		t.Fatal("disabling the plugin left its status item")
	}
}

func TestPluginViewBecomesAToolWindow(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	pluginFixture(t, settingsPath, "demo")
	m := &Model{cfg: Config{SettingsPath: settingsPath}, welcomePage: "plugins"}
	pg := m.pluginsPage()
	full := pluginViewPrefix + "demo:demo.side"
	if dockSpec(full) == nil {
		t.Fatal("the contributed view is not a tool window")
	}
	p := pg.reg.Plugin("demo")
	tree := `{"id":"demo.side","tree":{"type":"stack","children":[` +
		`{"type":"text","text":"Plugin says hi"},` +
		`{"type":"list","items":[{"label":"one","description":"d"}]}]}}`
	if _, err := pg.host.Call(p, "views.render", json.RawMessage(tree)); err != nil {
		t.Fatal(err)
	}
	m.DockToggle(full)
	tt := workbenchTester(t, m)
	if !tt.HasText("Plugin says hi") {
		t.Fatalf("the plugin view is blank: %q", tt.Texts())
	}
	if !tt.HasText("• one") {
		t.Fatalf("the list item did not draw: %q", tt.Texts())
	}
	m.setPluginEnabled("demo", false)
	if dockSpec(full) != nil {
		t.Fatal("a disabled plugin kept its tool window")
	}
	if m.dockWindowShown(full) {
		t.Fatal("a disabled plugin's view is still open")
	}
}
