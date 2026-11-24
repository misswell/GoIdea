package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNativeToAsciiActionStagesOnEncodingPage(t *testing.T) {
	root := t.TempDir()
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "settings_get" {
			t.Fatalf("command = %q, want settings_get", command)
		}
		return json.RawMessage(`{"editor":{"encodings":{"nativeToAscii":false}}}`), nil
	}
	m.loadNativeSettingsPage(settingsPageFileEncodings)
	waitSettingsBackendTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPageFileEncodings]
		return state.loaded && !state.loading
	})

	staged := map[string]any{
		"pageId": settingsPageFileEncodings,
		"scope":  "project",
		"value":  true,
	}
	if !m.RunActionWithArg(nativeToAsciiActionID, staged) {
		action, ok := actionGet(nativeToAsciiActionID)
		t.Fatalf("native-to-ascii action did not run: registered=%v whenNil=%v enabledNil=%v",
			ok, ok && action.When == nil, ok && action.Enabled == nil)
	}
	if !nativeSettingsBool(m, settingsPageFileEncodings, nativeToAsciiSettingID) {
		t.Fatal("native-to-ascii action did not stage true")
	}
	if !m.settingsDirtyPage(settingsPageFileEncodings) {
		t.Fatal("native-to-ascii action did not dirty the Encoding page")
	}
	if !m.ActionToggled(nativeToAsciiActionID) {
		t.Fatal("native-to-ascii action does not report its staged state")
	}
}

func TestNativeToAsciiActionWritesProjectOrGlobalScope(t *testing.T) {
	for _, test := range []struct {
		name  string
		root  string
		scope string
	}{
		{name: "project", root: t.TempDir(), scope: "project"},
		{name: "global", scope: "global"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var writes []capturedSettingsWrite
			m := &Model{ctx: context.Background()}
			if test.root != "" {
				m.setRoot(test.root)
			}
			m.postHook = func(fn func()) { fn() }
			m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
				switch command {
				case "settings_get", "settings_get_global":
					return json.RawMessage(`{"editor":{"encodings":{"nativeToAscii":false}}}`), nil
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
				}
				t.Fatalf("unexpected command %q", command)
				return nil, nil
			}

			if !m.RunAction(nativeToAsciiActionID) {
				t.Fatal("native-to-ascii action is unavailable")
			}
			waitSettingsBackendTest(t, func() bool {
				mu.Lock()
				defer mu.Unlock()
				return len(writes) == 1
			})
			mu.Lock()
			defer mu.Unlock()
			if writes[0].scope != test.scope ||
				writes[0].key != nativeToAsciiSettingID ||
				writes[0].value != "true" {
				t.Fatalf("settings_set = %+v", writes[0])
			}
		})
	}
}

func TestFileEncodingsPageLoadsAllOpenIDEAValues(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.backendFormatOnSaveKnown = true
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_get":
			payload := map[string]any{
				"editor": map[string]any{
					"encodings": map[string]any{
						"global":        "gbk",
						"project":       "utf-16le",
						"properties":    "windows-1252",
						"nativeToAscii": true,
					},
				},
			}
			setNestedTestValue(payload, utf8BOMSettingID, "always")
			raw, err := json.Marshal(payload)
			return raw, err
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
		}
		t.Fatalf("unexpected command %q", command)
		return nil, nil
	}
	m.loadNativeSettingsPage(settingsPageFileEncodings)
	waitSettingsBackendTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPageFileEncodings]
		return state.loaded && !state.loading
	})

	for key, want := range map[string]string{
		"editor.encodings.global":     "gbk",
		"editor.encodings.project":    "utf-16le",
		"editor.encodings.properties": "windows-1252",
		utf8BOMSettingID:              "always",
	} {
		if got := nativeSettingsString(m, settingsPageFileEncodings, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if !nativeSettingsBool(m, settingsPageFileEncodings, nativeToAsciiSettingID) {
		t.Fatal("nativeToAscii did not load true")
	}

	m.setNativeSettingsValue(settingsPageFileEncodings, utf8BOMSettingID, "windows")
	before := m.pendingNativeSettingsWrites()
	if len(before) != 1 || before[0].key != utf8BOMSettingID || before[0].value != "windows" {
		t.Fatalf("pending BOM write = %+v, want exact encoded key and Windows policy", before)
	}
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool {
		return !m.nativeSettings.saving
	})
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 1 || writes[0].scope != "project" ||
		writes[0].key != utf8BOMSettingID || writes[0].value != `"windows"` {
		t.Fatalf("settings_set = %+v, want exact encoded key in project scope", writes)
	}
}

func setNestedTestValue(root map[string]any, dotted string, value any) {
	parts := strings.Split(dotted, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}
