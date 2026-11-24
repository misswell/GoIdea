package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mayoi/internal/openidea"
	"mayoi/internal/plugin"
)

// pluginFixture writes one installed plugin under the data folder of path,
// and returns its folder.
func pluginFixture(t *testing.T, settingsPath, name string) string {
	t.Helper()
	t.Cleanup(func() {
		unregisterAction(pluginActionPrefix + name + ".run")
	})
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
			"views":    []map[string]any{{"id": name + ".side", "title": "Demo View", "location": "sidebar"}},
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
	// The contributed command is a palette action under OpenIDEA's
	// plugin.<pluginId>.<commandId> address, and the plugin's switch takes it
	// out and puts it back. The old GoIDEA address remains callable for
	// existing keymaps.
	id := "plugin.demo.run"
	legacyID := pluginActionPrefix + "demo.run"
	if _, ok := Command(id); !ok {
		t.Fatalf("the plugin command is not in the palette: %v", actionList())
	}
	if _, ok := Command(legacyID); !ok {
		t.Fatalf("the legacy plugin command alias is not callable: %v", actionList())
	}
	if got := commandTitle(id); got != "Demo: Run" {
		t.Fatalf("the action title is %q", got)
	}
	m.setPluginEnabled("demo", false)
	if _, ok := Command(id); ok {
		t.Fatal("a disabled plugin's command stayed in the palette")
	}
	if _, ok := Command(legacyID); ok {
		t.Fatal("a disabled plugin's legacy alias stayed callable")
	}
	m.setPluginEnabled("demo", true)
	if _, ok := Command(id); !ok {
		t.Fatal("re-enabling the plugin did not bring its command back")
	}
	if _, ok := Command(legacyID); !ok {
		t.Fatal("re-enabling the plugin did not restore its legacy alias")
	}
}

func TestPluginLogsCanFindWrapAndClear(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	pluginFixture(t, settingsPath, "demo")
	m := &Model{cfg: Config{SettingsPath: settingsPath}, welcomePage: "plugins"}
	pg := m.pluginsPage()
	p := pg.reg.Plugin("demo")
	if p == nil {
		t.Fatal("the fixture plugin is not loaded")
	}
	for i := 0; i < 8; i++ {
		level := "info"
		message := "log entry " + fmt.Sprint(i)
		if i == 7 {
			level = "error"
			message = "needle"
		}
		pg.host.Logged(p, plugin.LogLine{Level: level, Message: message})
	}

	tt := welcomeTester(t, m, true)
	if tt.HasText("info: log entry 0") {
		t.Fatalf("the unwrapped log should show only the latest six lines: %q", tt.Texts())
	}
	if err := tt.Click("Soft-wrap"); err != nil {
		t.Fatal(err)
	}
	if !pg.logWrap || !tt.HasText("info: log entry 0") {
		t.Fatalf("Soft-wrap = %v, logs = %q", pg.logWrap, tt.Texts())
	}

	if err := tt.Click("Find in Logs"); err != nil {
		t.Fatal(err)
	}
	pg.logQuery = "needle"
	tt.Frame()
	if !tt.HasText("error: needle") || tt.HasText("info: log entry 7") {
		t.Fatalf("the native log query did not filter lines: %q", tt.Texts())
	}

	if err := tt.Click("Clear"); err != nil {
		t.Fatal(err)
	}
	if got := pg.host.lines(p.Name); len(got) != 0 {
		t.Fatalf("Clear left %d plugin log lines", len(got))
	}
	if !tt.HasText("No plugin log entries") {
		t.Fatalf("the cleared log has no empty state: %q", tt.Texts())
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
	m := &Model{postHook: func(fn func()) { fn() }}
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
	if got := stat.(map[string]any); got["isDir"] != false || got["size"] != int64(2) {
		t.Fatalf("workspace.stat = %v", got)
	}
	if _, err := h.Call(nil, "editor.frobnicate", nil); err == nil {
		t.Fatal("an unsupported API answered without an error")
	}
}

func TestPluginHostReadsAndStatsRemoteWorkspaceThroughOpenIDEA(t *testing.T) {
	root := "/srv/app"
	var commands []string
	m := &Model{
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			commands = append(commands, command)
			value := input.(map[string]any)["path"]
			switch command {
			case "plugin_workspace_read_text":
				if value != root+"/src/a.txt" {
					t.Fatalf("read path = %v", value)
				}
				return json.RawMessage(`"remote text"`), nil
			case "plugin_workspace_stat":
				if value == root+"/src" {
					return json.RawMessage(`{"isDir":true,"size":0,"mtime":1000}`), nil
				}
				return json.RawMessage(`{"isDir":false,"size":123,"mtime":2000}`), nil
			default:
				t.Fatalf("unexpected command %q", command)
				return nil, nil
			}
		},
		remote: remoteDevelopmentState{
			session: &openidea.RemoteSession{
				Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
				Root:   root,
				OS:     "linux",
			},
		},
	}
	m.setRoot(root)
	h := &goideaPluginHost{m: m}
	if value, err := h.Call(nil, "workspace.isRemote", nil); err != nil || value != true {
		t.Fatalf("workspace.isRemote = %v, %v", value, err)
	}
	if value, err := h.Call(nil, "workspace.readFile", json.RawMessage(`{"path":"src/a.txt"}`)); err != nil || value != "remote text" {
		t.Fatalf("workspace.readFile = %v, %v", value, err)
	}
	if _, err := h.Call(nil, "workspace.readFile", json.RawMessage(`{"path":"../outside"}`)); err == nil {
		t.Fatal("remote plugin read escaped the project")
	}
	fileStat, err := h.Call(nil, "workspace.stat", json.RawMessage(`{"path":"src/a.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := fileStat.(map[string]any); got["isDir"] != false || got["size"] != int64(123) {
		t.Fatalf("remote file stat = %+v", got)
	}
	dirStat, err := h.Call(nil, "workspace.stat", json.RawMessage(`{"path":"src"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirStat.(map[string]any); got["isDir"] != true || got["size"] != int64(0) {
		t.Fatalf("remote directory stat = %+v", got)
	}
	if got := strings.Join(commands, ","); got != "plugin_workspace_read_text,plugin_workspace_stat,plugin_workspace_stat" {
		t.Fatalf("remote plugin commands = %s", got)
	}
}

func TestPluginNotificationIsVerbatimAndMultiline(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	h := &goideaPluginHost{m: m}
	p := &plugin.Plugin{Manifest: plugin.Manifest{Name: "demo", DisplayName: "Demo Plugin"}}
	body := json.RawMessage(`{"message":"first line\nsecond line","kind":"warning"}`)
	v, err := h.Call(p, "window.notify", body)
	if err != nil || v != false {
		t.Fatalf("window.notify = %v, %v", v, err)
	}
	if len(m.notify.items) != 1 {
		t.Fatalf("notifications = %+v", m.notify.items)
	}
	note := m.notify.items[0]
	if note.title != "Demo Plugin" || note.body != "first line\nsecond line" ||
		note.level != "warning" || note.group != "plugins" || !note.verbatim ||
		!note.userInitiated {
		t.Fatalf("plugin notification = %+v", note)
	}
}

func TestPluginCommandBridgeForwardsItsArgument(t *testing.T) {
	var got any
	if err := registerAction(&Action{
		ID: "test.pluginArg", Title: "Plugin Arg",
		RunWithArg: func(_ *Model, arg any) { got = arg },
	}); err != nil {
		t.Fatal(err)
	}
	defer unregisterAction("test.pluginArg")

	m := &Model{postHook: func(fn func()) { fn() }}
	h := &goideaPluginHost{m: m}
	if _, err := h.Call(nil, "commands.run", json.RawMessage(
		`{"id":"test.pluginArg","arg":{"task":"open","turn":1}}`,
	)); err != nil {
		t.Fatal(err)
	}
	arg, ok := got.(map[string]any)
	if !ok || arg["task"] != "open" || arg["turn"] != float64(1) {
		t.Fatalf("commands.run argument = %#v", got)
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
		`{"type":"list","items":[{"label":"one","description":"d","onClick":{"$h":"h1"}}]},` +
		`{"type":"button","label":"Run demo","onClick":{"$h":"h2"}},` +
		`{"type":"input","value":"typed","placeholder":"Search plugins"},` +
		`{"type":"select","value":"two","options":["one","two"]},` +
		`{"type":"checkbox","label":"Enabled","checked":true},` +
		`{"type":"progress","value":0.42}]}}`
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
	if !tt.HasText("Run demo") {
		t.Fatalf("the view button did not draw: %q", tt.Texts())
	}
	for _, text := range []string{"Enabled", "42%"} {
		if !tt.HasText(text) {
			t.Fatalf("%q did not draw in the plugin view: %q", text, tt.Texts())
		}
	}
	m.DockHide(full)
	m.postHook = func(fn func()) { fn() }
	request := json.RawMessage(`{"id":"demo.side"}`)
	if _, err := pg.host.Call(p, "views.reveal", request); err != nil {
		t.Fatal(err)
	}
	if !m.dockWindowShown(full) {
		t.Fatal("views.reveal did not show the contributed tool window")
	}
	if _, err := pg.host.Call(p, "views.reveal", json.RawMessage(`{"id":"missing"}`)); err == nil {
		t.Fatal("views.reveal accepted a view the plugin does not contribute")
	}
	m.setPluginEnabled("demo", false)
	if dockSpec(full) != nil {
		t.Fatal("a disabled plugin kept its tool window")
	}
	if m.dockWindowShown(full) {
		t.Fatal("a disabled plugin's view is still open")
	}
}

func TestPluginViewFormStateIsKeptUntilThePluginRendersAgain(t *testing.T) {
	host := &goideaPluginHost{}
	key := pluginViewPrefix + "demo:side\x00/0"
	checkboxKey := pluginViewPrefix + "demo:side\x00/1"
	if got := host.viewTextValue(key, "initial"); got != "initial" {
		t.Fatalf("initial input = %q", got)
	}
	host.setViewTextValue(key, "edited")
	if got := host.viewTextValue(key, "ignored"); got != "edited" {
		t.Fatalf("edited input = %q", got)
	}
	if got := host.viewCheckedValue(checkboxKey, true); !got {
		t.Fatal("initial checkbox was false")
	}
	host.setViewCheckedValue(checkboxKey, false)
	if got := host.viewCheckedValue(checkboxKey, true); got {
		t.Fatal("checkbox edit was lost")
	}
	host.resetViewForm(pluginViewPrefix + "demo:side")
	if got := host.viewTextValue(key, "rendered again"); got != "rendered again" {
		t.Fatalf("input after render = %q", got)
	}
}

func TestPluginStatusItemIsANamedWidgetAndFollowsUpdates(t *testing.T) {
	m := &Model{}
	host := &goideaPluginHost{m: m, status: map[string]pluginStatusItem{}}
	p := &plugin.Plugin{Manifest: plugin.Manifest{Name: "demo", DisplayName: "Demo Plugin"}}
	full := "plugin.status:demo:build"
	defer unregisterStatusItem(full)

	host.setStatus(p, "build", &pluginStatusItem{text: "Building"})
	rows := m.statusWidgetRows()
	var found *StatusItem
	for _, row := range rows {
		if row.ID == full {
			found = row
			break
		}
	}
	if found == nil || found.Title != "demo: Building" {
		t.Fatalf("plugin status widget = %+v, rows %+v", found, rows)
	}
	host.setStatus(p, "build", &pluginStatusItem{text: "Ready", tooltip: "Build status"})
	if found.Title != "demo: Build status" {
		t.Fatalf("updated plugin status title = %q", found.Title)
	}
}
