package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mayoi/internal/openidea"
)

func phpRuntimeTestModel(t *testing.T, root string) *Model {
	t.Helper()
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	def, ok := nativeSettingsDefinition(settingsPagePHP)
	if !ok {
		t.Fatal("PHP settings page is missing")
	}
	values := nativeTestDefaults(def)
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPagePHP: {
			loaded:  true,
			applied: nativeSettingsCloneValues(values),
			draft:   nativeSettingsCloneValues(values),
		},
	}
	return m
}

func TestPHPRuntimeStatusConsumesEffectiveXdebugAndLanguageLevel(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.php")
	m := phpRuntimeTestModel(t, root)
	m.setNativeSettingsValue(settingsPagePHP, "languages.php.interpreter", "/configured/php")
	var mu sync.Mutex
	var commands []string
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()
		switch command {
		case "sdk_php_effective":
			return json.RawMessage(`"/configured/php"`), nil
		case "sdk_php_xdebug":
			request := args.(map[string]any)
			if request["php"] != "/configured/php" || request["cwd"] != root {
				t.Fatalf("xdebug request = %#v", request)
			}
			return json.RawMessage(`{
				"php":"/configured/php","phpVersion":"8.3.7","xdebug":"3.3.2"
			}`), nil
		case "sdk_php_language_level":
			request := args.(map[string]any)
			if request["path"] != target {
				t.Fatalf("language-level request = %#v", request)
			}
			return json.RawMessage(`{
				"effective":{"version":"8.2","source":"platform"},
				"auto":{"version":"8.2","source":"platform"},
				"member":"service/composer.json"
			}`), nil
		default:
			t.Fatalf("unexpected RPC %q", command)
			return nil, nil
		}
	}

	m.ensurePHPRuntimeStatus(target, false)
	waitSettingsBackendTest(t, func() bool {
		status := m.languageSDKs.php
		return status.loaded && !status.loading
	})
	status := m.languageSDKs.php
	if status.effective == nil || *status.effective != "/configured/php" {
		t.Fatalf("effective PHP = %#v", status.effective)
	}
	if status.xdebug == nil || status.xdebug.Xdebug == nil || *status.xdebug.Xdebug != "3.3.2" {
		t.Fatalf("Xdebug = %+v", status.xdebug)
	}
	if status.levels.Effective == nil || status.levels.Effective.Source != "platform" {
		t.Fatalf("language levels = %+v", status.levels)
	}
	if label := phpRuntimeExecutableLabel(status); !strings.Contains(label, "Xdebug 3.3.2") {
		t.Fatalf("executable label = %q", label)
	}
	if label := phpLanguageLevelLabel(status.levels); !strings.Contains(label, "Auto") ||
		!strings.Contains(label, "composer.json") {
		t.Fatalf("language label = %q", label)
	}

	m.ensurePHPRuntimeStatus(target, false)
	mu.Lock()
	defer mu.Unlock()
	if len(commands) != 3 {
		t.Fatalf("successful status was probed again: %q", commands)
	}
}

func TestPHPRuntimeStatusFallsBackToLocatedPHPTool(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.php")
	m := phpRuntimeTestModel(t, root)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "sdk_php_effective":
			return json.RawMessage(`null`), nil
		case "sdk_locate_tools":
			request := args.(map[string]any)
			if request["near"] != root {
				t.Fatalf("locate request = %#v", request)
			}
			return json.RawMessage(`{
				"tools":[{"name":"php","path":"/usr/local/bin/php","version":"8.3.7"}],
				"compileDir":"/tmp"
			}`), nil
		case "sdk_php_xdebug":
			request := args.(map[string]any)
			if request["php"] != "/usr/local/bin/php" {
				t.Fatalf("xdebug request = %#v", request)
			}
			return json.RawMessage(`{
				"php":"/usr/local/bin/php","phpVersion":"8.3.7","xdebug":null
			}`), nil
		case "sdk_php_language_level":
			return json.RawMessage(`{
				"effective":null,
				"auto":{"version":"8.3","source":"require"},
				"member":null
			}`), nil
		default:
			t.Fatalf("unexpected RPC %q", command)
			return nil, nil
		}
	}

	m.ensurePHPRuntimeStatus(target, false)
	waitSettingsBackendTest(t, func() bool {
		status := m.languageSDKs.php
		return status.loaded && !status.loading
	})
	status := m.languageSDKs.php
	if status.located == nil || status.located.Path == nil ||
		*status.located.Path != "/usr/local/bin/php" {
		t.Fatalf("located PHP = %+v", status.located)
	}
	if label := phpRuntimeExecutableLabel(status); !strings.Contains(label, "Xdebug not installed") {
		t.Fatalf("executable label = %q", label)
	}
	if label := phpLanguageLevelLabel(status.levels); !strings.Contains(label, "Auto selects") {
		t.Fatalf("language label = %q", label)
	}
}

func TestPHPRuntimeStatusIgnoresAProjectSwitch(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	posted := make(chan struct{}, 1)
	m := phpRuntimeTestModel(t, firstRoot)
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "sdk_php_effective":
			close(started)
			<-release
			return json.RawMessage(`"/old/php"`), nil
		case "sdk_php_xdebug":
			return json.RawMessage(`{
				"php":"/old/php","phpVersion":"8.3.0","xdebug":null
			}`), nil
		case "sdk_php_language_level":
			return json.RawMessage(`{"effective":null,"auto":null,"member":null}`), nil
		default:
			t.Errorf("unexpected RPC %q", command)
			return nil, nil
		}
	}
	m.ensurePHPRuntimeStatus(filepath.Join(firstRoot, "main.php"), false)
	<-started
	m.setRoot(secondRoot)
	close(release)
	<-posted
	if status := m.languageSDKs.php; status.effective != nil {
		t.Fatalf("stale PHP status was applied: %+v", status)
	}
}

func TestPHPLanguageLevelLabelsExplicitSettings(t *testing.T) {
	level := &openidea.PhpLanguageLevel{Version: "8.1", Source: "settings"}
	if got := phpLanguageLevelOrigin(level); got != "PHP 8.1, set in Settings" {
		t.Fatalf("origin = %q", got)
	}
	if got := phpShortVersion(level); got != "8.1" {
		t.Fatalf("short version = %q", got)
	}
	levels := openidea.PhpLanguageLevels{
		Effective: level,
		Auto:      &openidea.PhpLanguageLevel{Version: "8.2", Source: "platform"},
	}
	if got := phpLanguageLevelLabel(levels); strings.Contains(got, "Auto") {
		t.Fatalf("explicit label claimed Auto: %q", got)
	}
}
