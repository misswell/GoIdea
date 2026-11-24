package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func languageStatusTestModel(t *testing.T, root string) *Model {
	t.Helper()
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.run.loaded = true
	m.run.trusted = true
	m.postHook = func(fn func()) { fn() }
	m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	for _, page := range []string{settingsPagePythonInterpret, settingsPagePHP} {
		def, ok := nativeSettingsDefinition(page)
		if !ok {
			t.Fatalf("%s settings page is missing", page)
		}
		values := nativeTestDefaults(def)
		m.nativeSettings.pages[page] = nativeSettingsPageState{
			loaded:  true,
			applied: nativeSettingsCloneValues(values),
			draft:   nativeSettingsCloneValues(values),
		}
	}
	return m
}

func TestLanguageStatusActionsAreRegistered(t *testing.T) {
	for _, id := range []string{
		"python.interpreterPopup",
		"python.selectInterpreter",
		"python.addInterpreter",
		"python.interpreterSettings",
		"php.languageLevelPopup",
		"php.selectLanguageLevel",
	} {
		if actions.byID[id] == nil {
			t.Fatalf("action %q is not registered", id)
		}
	}
}

func TestPythonInterpreterStatusUsesEffectiveSDKAndWritesProjectSetting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".venv", "bin", "python")
	m := languageStatusTestModel(t, root)
	doc := docbridge.Virtual(filepath.Join(root, "main.py"), "print('ok')\n")
	tab := m.newTab(doc)
	m.tabs = []*Tab{tab}
	m.active = 0

	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "sdk_python_effective":
			return json.RawMessage(`"` + filepath.ToSlash(path) + `"`), nil
		case "sdk_detect":
			request := args.(map[string]any)
			if request["kind"] != openidea.SdkPython {
				t.Fatalf("sdk kind = %#v", request)
			}
			return json.RawMessage(`[{
				"path":"` + filepath.ToSlash(path) + `",
				"name":"Python 3.12 (.venv)",
				"source":"venv"
			}]`), nil
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
			t.Fatalf("unexpected RPC %q", command)
			return nil, nil
		}
	}

	tt := workbenchTester(t, m)
	waitSettingsBackendTest(t, func() bool {
		state := m.languageSDKs.status.python
		return state.loaded && !state.loading && state.effective != nil
	})
	tt.Frame()
	if !tt.HasText("Python 3.12 (.venv)") {
		t.Fatalf("status bar lacks the effective interpreter: %v", tt.Texts())
	}
	if err := tt.Click("Python 3.12 (.venv)"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	if !containsLanguageStatusMenu(menu, "Python 3.12 (.venv)") ||
		!containsLanguageStatusMenu(menu, "Add New Interpreter…") ||
		!containsLanguageStatusMenu(menu, "Interpreter Settings…") {
		t.Fatalf("interpreter menu = %v", menu)
	}
	if err := tt.ChooseMenuItem("Python 3.12 (.venv)"); err != nil {
		t.Fatal(err)
	}
	waitSettingsBackendTest(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(writes) == 1
	})
	mu.Lock()
	write := writes[0]
	mu.Unlock()
	if write.scope != "project" || write.key != "languages.python.interpreter" ||
		write.value != `"`+path+`"` {
		t.Fatalf("interpreter write = %+v", write)
	}
	if got := nativeSettingsString(
		m, settingsPagePythonInterpret, "languages.python.interpreter",
	); got != path {
		t.Fatalf("selected interpreter = %q", got)
	}
}

func TestPHPLanguageLevelMenuWritesProjectSettingAndHonoursMember(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.php")
	m := languageStatusTestModel(t, root)
	tab := m.newTab(docbridge.Virtual(target, "<?php\n"))
	m.tabs = []*Tab{tab}
	m.active = 0
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	effective := &openidea.PhpLanguageLevel{Version: "7.4.0", Source: "require"}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "sdk_php_effective":
			return json.RawMessage(`null`), nil
		case "sdk_locate_tools":
			return json.RawMessage(`{
				"tools":[{"name":"php","path":"/usr/local/bin/php","version":"8.3.7"}],
				"compileDir":"/tmp"
			}`), nil
		case "sdk_php_xdebug":
			return json.RawMessage(`{
				"php":"/usr/local/bin/php","phpVersion":"8.3.7","xdebug":null
			}`), nil
		case "sdk_php_language_level":
			mu.Lock()
			level := effective
			mu.Unlock()
			levels, err := json.Marshal(openidea.PhpLanguageLevels{
				Effective: level,
				Auto:      &openidea.PhpLanguageLevel{Version: "7.4.0", Source: "require"},
			})
			return levels, err
		case "settings_set":
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string),
				key:   request["key"].(string),
				value: request["value"].(string),
			})
			effective = &openidea.PhpLanguageLevel{Version: "8.1.0", Source: "settings"}
			mu.Unlock()
			return json.RawMessage(`null`), nil
		case "find_files":
			return json.RawMessage(`[]`), nil
		case "settings_get":
			return json.RawMessage(`{}`), nil
		default:
			t.Fatalf("unexpected RPC %q", command)
			return nil, nil
		}
	}

	tt := workbenchTester(t, m)
	waitSettingsBackendTest(t, func() bool {
		state := m.languageSDKs.php
		return state.loaded && !state.loading && state.levels.Effective != nil
	})
	tt.Frame()
	if !tt.HasText("PHP 7.4") {
		t.Fatalf("PHP status bar lacks its level: %v", tt.Texts())
	}
	if err := tt.Click("PHP 7.4"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	for _, want := range []string{"Auto (7.4)", "8.1", "5.6"} {
		if !containsLanguageStatusMenu(menu, want) {
			t.Fatalf("PHP menu %v lacks %q", menu, want)
		}
	}
	if err := tt.ChooseMenuItem("8.1"); err != nil {
		t.Fatal(err)
	}
	waitSettingsBackendTest(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(writes) == 1
	})
	mu.Lock()
	write := writes[0]
	mu.Unlock()
	if write.scope != "project" || write.key != "languages.php.languageLevel" ||
		write.value != `"8.1"` {
		t.Fatalf("language-level write = %+v", write)
	}

	m.languageSDKs.php.levels.Member = stringPointer(filepath.Join(root, "service"))
	if m.selectPHPLanguageLevel("8.2") {
		t.Fatal("a linked-project language level was changed from the root project")
	}
}

func TestLanguageStatusPopupFallsBackToSettings(t *testing.T) {
	root := t.TempDir()
	m := languageStatusTestModel(t, root)
	if !m.RunAction("python.interpreterPopup") {
		t.Fatal("python.interpreterPopup is not runnable")
	}
	if !m.settingsOpen || m.settingsPage != settingsPagePythonInterpret {
		t.Fatalf("Python settings did not open: open=%v page=%q", m.settingsOpen, m.settingsPage)
	}
}

func containsLanguageStatusMenu(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
