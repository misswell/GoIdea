package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
)

type languageReloadTestCalls struct {
	mu         sync.Mutex
	writes     []capturedSettingsWrite
	runConfigs int
	libraries  int
}

func newLanguageReloadTestModel(t *testing.T) (*Model, *languageReloadTestCalls) {
	t.Helper()
	root := t.TempDir()
	calls := &languageReloadTestCalls{}
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.run.trusted = true
	m.postHook = func(fn func()) { fn() }
	m.backendFormatOnSaveKnown = true
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		calls.mu.Lock()
		defer calls.mu.Unlock()
		switch command {
		case "settings_set":
			request := args.(map[string]any)
			calls.writes = append(calls.writes, capturedSettingsWrite{
				scope: request["scope"].(string),
				key:   request["key"].(string),
				value: request["value"].(string),
			})
			return json.RawMessage(`null`), nil
		case "run_configs":
			calls.runConfigs++
			return json.RawMessage(`[]`), nil
		case "sdk_project_libraries":
			calls.libraries++
			return json.RawMessage(`[]`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	return m, calls
}

func (c *languageReloadTestCalls) countRunConfigs() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runConfigs
}

func (c *languageReloadTestCalls) countLibraries() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.libraries
}

func (c *languageReloadTestCalls) lastSettingsWrite() capturedSettingsWrite {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.writes) == 0 {
		return capturedSettingsWrite{}
	}
	return c.writes[len(c.writes)-1]
}

func TestPythonInterpreterSaveReloadsRunConfigsAndLibraries(t *testing.T) {
	m, calls := newLanguageReloadTestModel(t)
	def, ok := nativeSettingsDefinition(settingsPagePythonInterpret)
	if !ok {
		t.Fatal("Python interpreter page is not defined")
	}
	applied := nativeTestDefaults(def)
	draft := nativeSettingsCloneValues(applied)
	draft["languages.python.interpreter"] = "/project/.venv/bin/python"
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPagePythonInterpret: {
			loaded: true, scope: "mixed", applied: applied, draft: draft,
		},
	}

	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool {
		return calls.countRunConfigs() == 1 && calls.countLibraries() == 1
	})
	if m.nativeSettings.saving {
		t.Fatal("native settings were still saving after the reloads started")
	}
	write := calls.lastSettingsWrite()
	if write.scope != "project" || write.key != "languages.python.interpreter" ||
		write.value != `"/project/.venv/bin/python"` {
		t.Fatalf("settings write = %+v", write)
	}
}

func TestJavaSettingsSaveReloadsRunConfigsAndLibraries(t *testing.T) {
	m, calls := newLanguageReloadTestModel(t)
	m.javaSettings = javaSettingsState{
		loaded:  true,
		scope:   "project",
		applied: JavaSettings{JDK: "/old/jdk"},
		draft:   JavaSettings{JDK: "/new/jdk"},
	}

	m.saveJavaSettingsThenLanguageServers(false)
	waitSettingsBackendTest(t, func() bool {
		return !m.javaSettings.saving &&
			calls.countRunConfigs() == 1 && calls.countLibraries() == 1
	})
	write := calls.lastSettingsWrite()
	if write.scope != "project" || write.key != "languages.java.jdk" || write.value != `"/new/jdk"` {
		t.Fatalf("settings write = %+v", write)
	}
}
