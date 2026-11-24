package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestPythonInterpreterPageLoadsMixedScopesAndSavesEachFieldInItsScope(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_get_global":
			return json.RawMessage(`{
				"languages":{"python":{
					"interpreters":["/opt/added/python"],
					"hiddenInterpreters":["/opt/hidden/python"],
					"hideOtherProjectInterpreters":false,
					"showInterpreterPaths":true
				}}
			}`), nil
		case "settings_get":
			return json.RawMessage(`{
				"languages":{"python":{"interpreter":"/project/.venv/bin/python"}}
			}`), nil
		case "settings_set":
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string),
				key:   request["key"].(string),
				value: request["value"].(string),
			})
			mu.Unlock()
			return json.RawMessage(`null`), nil
		default:
			t.Fatalf("unexpected command %q", command)
			return nil, nil
		}
	}

	m.loadNativeSettingsPage(settingsPagePythonInterpret)
	waitSettingsBackendTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPagePythonInterpret]
		return state.loaded && !state.loading
	})
	state := m.nativeSettings.pages[settingsPagePythonInterpret]
	if state.scope != "mixed" {
		t.Fatalf("scope = %q, want mixed", state.scope)
	}
	if got := nativeSettingsString(m, settingsPagePythonInterpret, "languages.python.interpreter"); got != "/project/.venv/bin/python" {
		t.Fatalf("project interpreter = %q", got)
	}
	if got := nativeSettingsListValue(m, settingsPagePythonInterpret, "languages.python.interpreters"); !reflect.DeepEqual(got, []string{"/opt/added/python"}) {
		t.Fatalf("added interpreters = %#v", got)
	}

	m.setNativeSettingsValue(settingsPagePythonInterpret, "languages.python.interpreter", "/project/.venv/bin/python3")
	m.setPythonInterpreterAdded([]string{"/opt/added/python", "/opt/another/python"})
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool {
		return !m.nativeSettings.saving && !m.settingsDirtyPage(settingsPagePythonInterpret)
	})

	mu.Lock()
	defer mu.Unlock()
	got := map[string]string{}
	for _, write := range writes {
		got[write.scope+"\x00"+write.key] = write.value
	}
	if got["project\x00languages.python.interpreter"] != `"/project/.venv/bin/python3"` {
		t.Fatalf("project writes = %#v", got)
	}
	if got["global\x00languages.python.interpreters"] != `["/opt/added/python","/opt/another/python"]` {
		t.Fatalf("global writes = %#v", got)
	}
}

func TestLanguageSDKDetectionIgnoresAProjectSwitch(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	m := &Model{ctx: context.Background()}
	m.setRoot(firstRoot)
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "sdk_detect" {
			t.Fatalf("unexpected command %q", command)
		}
		request := args.(map[string]any)
		if request["kind"] == openidea.SdkPackageManager {
			return json.RawMessage(`[]`), nil
		}
		startOnce.Do(func() { close(started) })
		<-release
		return json.RawMessage(`[{"path":"/old/node","name":"Old Node","version":"20.0.0","source":"path","home":null,"extra":null}]`), nil
	}
	m.ensureNativeLanguageSDKs(settingsPageNode, false)
	<-started
	m.setRoot(secondRoot)
	close(release)
	time.Sleep(20 * time.Millisecond)
	state := m.languageSDKs.kinds[openidea.SdkNode]
	if len(state.sdks) != 0 {
		t.Fatalf("stale SDK reply was applied: %+v", state.sdks)
	}
}

func TestCreatePythonEnvironmentUsesRequestChannelAndMakesTheResultCurrent(t *testing.T) {
	root := t.TempDir()
	release := make(chan struct{})
	var mu sync.Mutex
	var request map[string]any
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	values := nativeTestDefaults(mustNativeSettingsDefinition(t, settingsPagePythonInterpret))
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPagePythonInterpret: {
			loaded: true, applied: nativeSettingsCloneValues(values), draft: nativeSettingsCloneValues(values),
		},
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "sdk_python_create_venv" {
			t.Fatalf("unexpected command %q", command)
		}
		mu.Lock()
		request = args.(map[string]any)
		mu.Unlock()
		<-release
		return json.RawMessage(`{
			"path":"/project/.venv/bin/python","name":"Python 3.13 (.venv)",
			"version":"3.13.2","source":"venv","home":"/project/.venv","extra":null
		}`), nil
	}

	m.createPythonEnvironment("venv", "/usr/bin/python3", filepath.Join(root, ".venv"))
	waitSettingsBackendTest(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return request != nil
	})
	mu.Lock()
	call := map[string]any{}
	for key, value := range request {
		call[key] = value
	}
	mu.Unlock()
	if call["tool"] != "venv" || call["base"] != "/usr/bin/python3" || call["location"] != filepath.Join(root, ".venv") {
		t.Fatalf("create request = %#v", call)
	}
	if call["request"].(uint32) == 0 {
		t.Fatal("request id is zero")
	}
	output := call["onOutput"].(map[string]any)
	channel := output["$channel"].(uint32)
	if channel == 0 {
		t.Fatal("output channel is zero")
	}
	if !m.acceptLanguageSDKChunk(channel, json.RawMessage(`"created pyvenv.cfg"`)) {
		t.Fatal("channel output was not accepted")
	}
	if got := m.languageSDKs.python.log; !reflect.DeepEqual(got, []string{"created pyvenv.cfg"}) {
		t.Fatalf("log = %#v", got)
	}
	close(release)
	waitSettingsBackendTest(t, func() bool { return !m.languageSDKs.python.busy })
	if got := nativeSettingsString(m, settingsPagePythonInterpret, "languages.python.interpreter"); got != "/project/.venv/bin/python" {
		t.Fatalf("current interpreter = %q", got)
	}
	if got := m.pythonInterpreterAdded(); !reflect.DeepEqual(got, []string{"/project/.venv/bin/python"}) {
		t.Fatalf("added interpreters = %#v", got)
	}
}

func TestBuildPythonInterpreterRowsKeepsCurrentVisibleAndDropsHiddenOthers(t *testing.T) {
	current := "/p/.venv/bin/python"
	root := "/p"
	rows := buildPythonInterpreterRows(
		[]openidea.Sdk{
			{Path: current, Name: "Python 3.13 (.venv)", Source: "venv"},
			{Path: "/p/other/.venv/bin/python", Name: "Python 3.12 (other)", Source: "venv"},
			{Path: "/usr/bin/python3", Name: "Python 3.12", Source: "system"},
			{Path: "/hidden/python", Name: "Hidden Python", Source: "path"},
		},
		nil,
		[]string{"/custom/python"},
		[]string{current, "/hidden/python"},
		current,
		root,
		pythonInterpreterProjects(root, nil),
		map[string]*openidea.Sdk{
			languageSDKPathKey(current): {Path: current, Name: "Python 3.13 (.venv)", Source: "venv"},
			"/custom/python":            {Path: "/custom/python", Name: "Custom Python", Source: "custom"},
		},
	)
	if len(rows) != 4 {
		t.Fatalf("rows = %+v", rows)
	}
	foundCurrent := false
	foundOtherVenv := false
	for _, row := range rows {
		if row.Current {
			foundCurrent = row.SDK.Path == current
		}
		if row.SDK.Path == "/p/other/.venv/bin/python" {
			foundOtherVenv = true
		}
		if row.SDK.Path == "/hidden/python" {
			t.Fatalf("hidden non-current interpreter was returned: %+v", row)
		}
	}
	if !foundCurrent {
		t.Fatalf("current interpreter was omitted: %+v", rows)
	}
	if !foundOtherVenv {
		t.Fatalf("unselected project virtualenv was omitted: %+v", rows)
	}
}

func TestNativeSDKChoicesKeepAnUnlistedCurrentPath(t *testing.T) {
	values, labels := nativeSDKChoices(
		[]openidea.Sdk{
			{Path: "/opt/node", Name: "Node", Version: stringPointer("22.1.0"), Source: "path"},
		},
		"/project/node",
		nativeSDKEmptyLabel(openidea.SdkNode),
		true,
	)
	if !reflect.DeepEqual(values, []string{"", "/opt/node", "/project/node"}) {
		t.Fatalf("values = %#v", values)
	}
	if labels[2] != "Current · /project/node" || labels[1] != "Node (22.1.0) · /opt/node" {
		t.Fatalf("labels = %#v", labels)
	}
}
