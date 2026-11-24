package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
)

func nativeTestDefaults(def nativeSettingsPageSpec) map[string]any {
	values := map[string]any{}
	for _, section := range def.sections {
		for _, field := range section.fields {
			values[field.key] = nativeSettingsDefault(field)
		}
	}
	return values
}

func TestNativeEditorSettingsLoadStageAndSaveDottedKeys(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_get_global":
			return json.RawMessage(`{
				"editor":{
					"completion":{"matchCase":false,"autoPopup":false,"parameterInfoDelay":750},
					"inlineCompletion":{"disabledLanguages":["go","rust"]},
					"smartKeys":{"camelHumps":true}
				},
				"appearance":{"locale":"zh"},
				"proxy":{"mode":"manual","host":"proxy.example","port":3128}
			}`), nil
		case "settings_set":
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string), key: request["key"].(string), value: request["value"].(string),
			})
			mu.Unlock()
			return json.RawMessage(`null`), nil
		}
		return json.RawMessage(`null`), nil
	}

	m.loadNativeSettingsPage(settingsPageCodeCompletion)
	waitSettingsBackendTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPageCodeCompletion]
		return state.loaded && !state.loading
	})
	if nativeSettingsBool(m, settingsPageCodeCompletion, "editor.completion.matchCase") {
		t.Fatal("loaded matchCase = true, want false")
	}
	if nativeSettingsNumberValue(m, settingsPageCodeCompletion, "editor.completion.parameterInfoDelay") != 750 {
		t.Fatalf("loaded parameter delay = %v", nativeSettingsNumberValue(m, settingsPageCodeCompletion, "editor.completion.parameterInfoDelay"))
	}

	m.setNativeSettingsValue(settingsPageCodeCompletion, "editor.completion.autoPopup", true)
	if !m.settingsDirtyPage(settingsPageCodeCompletion) {
		t.Fatal("completion edit was not staged")
	}
	writesBefore := m.pendingNativeSettingsWrites()
	if len(writesBefore) != 1 || writesBefore[0].key != "editor.completion.autoPopup" {
		t.Fatalf("pending native writes = %+v, want only autoPopup", writesBefore)
	}

	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool {
		return !m.nativeSettings.saving && !m.settingsDirtyPage(settingsPageCodeCompletion)
	})
	if !m.completionAutoPopup() {
		t.Fatal("applied completion auto-popup did not reach the runtime")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 1 {
		t.Fatalf("settings writes = %+v, want one dotted key", writes)
	}
	if writes[0].scope != "global" || writes[0].key != "editor.completion.autoPopup" || writes[0].value != "true" {
		t.Fatalf("settings write = %+v", writes[0])
	}
}

func TestNativeGlobalSettingsUseGlobalScopeAndPreserveLists(t *testing.T) {
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	m.postHook = func(fn func()) { fn() }
	m.backendFormatOnSaveKnown = true
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_get_global":
			return json.RawMessage(`{
				"appearance":{"locale":"zh"},
				"proxy":{"mode":"manual","kind":"socks","host":"127.0.0.1","port":1080,"noProxy":"*.example.com"}
			}`), nil
		case "settings_set":
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string), key: request["key"].(string), value: request["value"].(string),
			})
			mu.Unlock()
			return json.RawMessage(`null`), nil
		}
		return json.RawMessage(`null`), nil
	}

	m.loadNativeSettingsPage(settingsPageHTTPProxy)
	waitSettingsBackendTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPageHTTPProxy]
		return state.loaded && !state.loading
	})
	if got := m.nativeSettings.pages[settingsPageHTTPProxy].scope; got != "global" {
		t.Fatalf("proxy scope = %q, want global", got)
	}
	if got := nativeSettingsString(m, settingsPageHTTPProxy, "proxy.kind"); got != "socks" {
		t.Fatalf("proxy kind = %q, want socks", got)
	}
	m.setNativeSettingsValue(settingsPageHTTPProxy, "proxy.noProxy", "localhost,*.example.com")
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool {
		return !m.nativeSettings.saving && !m.settingsDirtyPage(settingsPageHTTPProxy)
	})
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 1 || writes[0].scope != "global" || writes[0].key != "proxy.noProxy" {
		t.Fatalf("proxy writes = %+v", writes)
	}
}

func TestNativeSettingsPagesRenderEveryDeclaredControl(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.settingsDraft = m.settingsDraftFromPrefs()
	m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	for _, def := range nativeSettingsDefinitions() {
		values := nativeTestDefaults(def)
		m.nativeSettings.pages[def.page.id] = nativeSettingsPageState{
			loaded: true, session: true, scope: nativeSettingsPageScope(m, def),
			root: m.Root(), applied: nativeSettingsValuesCopy(values), draft: nativeSettingsValuesCopy(values),
		}
	}

	for _, def := range nativeSettingsDefinitions() {
		def := def
		t.Run(def.page.id, func(t *testing.T) {
			body := func(c *ui.Context) {
				m.settingsPage = def.page.id
				if def.page.section == "gutterIcons" {
					m.nativeGutterBody(c, oiPaletteOf(c.Theme()))
					return
				}
				m.settingsNativeBody(c, oiPaletteOf(c.Theme()), def.page.id)
			}
			tester := ui.NewTester(body, 1000, 2200)
			tester.Frame()
			if def.page.section == "pythonOverview" {
				if !tester.HasText("Python interpreters, dependencies and docstring formatting") {
					t.Fatalf("Python overview lacks its description: %q", tester.Texts())
				}
				return
			}
			for _, section := range def.sections {
				for _, field := range section.fields {
					if field.key == "editor.gutterIconsDisabled" {
						continue
					}
					if !tester.HasText(field.label) {
						t.Fatalf("%s page lacks %q: %q", def.page.id, field.label, tester.Texts())
					}
				}
			}
		})
	}
}

func nativeSettingsValuesCopy(values map[string]any) map[string]any {
	return nativeSettingsCloneValues(values)
}

func TestNativeSettingsPageIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, page := range nativeSettingsPages() {
		if page.id == "" || seen[page.id] {
			t.Fatalf("native page id %q is empty or duplicated", page.id)
		}
		seen[page.id] = true
		if _, ok := settingsPageByID(page.id); !ok {
			t.Fatalf("native page %q is not registered", page.id)
		}
		if nativeSettingsPageLoadable(page.id) && page.title == "" {
			t.Fatalf("native page %q has no title", page.id)
		}
	}
}

func TestEditorCodeStyleAndColorSchemePagesRoundTripOpenIDEAStructures(t *testing.T) {
	for _, page := range []string{
		settingsPageCodeStyle,
		settingsPageCodeStyleOther,
		settingsPageColorScheme,
		settingsPageColorSchemeGen,
		settingsPageColorSchemeLang,
		settingsPageColorSchemeDebug,
		settingsPageColorSchemeCons,
		settingsPageCMake,
	} {
		def, ok := nativeSettingsDefinition(page)
		if !ok {
			t.Fatalf("settings page %q is not native", page)
		}
		if def.page.title == "" || len(def.sections) == 0 {
			t.Fatalf("settings page %q has no title or controls", page)
		}
	}

	root := t.TempDir()
	m := &Model{ctx: context.Background(), cfg: Config{SettingsPath: filepath.Join(root, "settings.json")}}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_get":
			return json.RawMessage(`{
				"editor":{"codeStyle":{"languages":{"java":{"tabSize":2}}}},
				"cmake":{"profiles":[{"name":"Debug","buildType":"Debug","cmakeArgs":["-DONE=1"],"env":{"MODE":"debug"}}]}
			}`), nil
		}
		return json.RawMessage(`null`), nil
	}

	for _, page := range []string{settingsPageCodeStyle, settingsPageCMake} {
		m.loadNativeSettingsPage(page)
		waitSettingsBackendTest(t, func() bool {
			state := m.nativeSettings.pages[page]
			return state.loaded && !state.loading
		})
	}
	languageStyles, ok := nativeSettingsValue(m, settingsPageCodeStyle, "editor.codeStyle.languages").(map[string]any)
	if !ok || languageStyles["java"] == nil {
		t.Fatalf("code style languages = %#v, want loaded object", languageStyles)
	}
	profiles, ok := nativeSettingsValue(m, settingsPageCMake, "cmake.profiles").([]any)
	if !ok || len(profiles) != 1 {
		t.Fatalf("CMake profiles = %#v, want one profile", profiles)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsNativeBody(c, oiPaletteOf(c.Theme()), settingsPageCMake)
	}, 1000, 1200)
	tester.Frame()
	if !tester.HasText("CMake profiles") || !tester.HasText("Add") || !tester.HasText("Build type") {
		t.Fatalf("CMake page lacks native profile controls: %q", tester.Texts())
	}
	if err := tester.Click("Add"); err != nil {
		t.Fatalf("click CMake Add: %v", err)
	}
	tester.Frame()
	profiles, _ = nativeSettingsValue(m, settingsPageCMake, "cmake.profiles").([]any)
	if len(profiles) != 2 {
		t.Fatalf("CMake Add produced %d profiles, want 2", len(profiles))
	}

	edited := []any{map[string]any{
		"name": "Release", "buildType": "Release", "generator": "Ninja",
		"cmakeArgs": []any{"-DRELEASE=ON"}, "buildDir": "build-release",
		"buildArgs": []any{"-j", "8"}, "env": map[string]any{"MODE": "release"},
	}}
	m.setNativeSettingsValue(settingsPageCMake, "cmake.profiles", edited)
	writes := m.pendingNativeSettingsWrites()
	if len(writes) != 1 || writes[0].key != "cmake.profiles" || writes[0].scope != "project" {
		t.Fatalf("pending CMake writes = %+v", writes)
	}
	if got, ok := writes[0].value.([]any); !ok || len(got) != 1 {
		t.Fatalf("pending CMake value = %#v, want object array", writes[0].value)
	}
}
