package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func setNativeAppliedTestValue(m *Model, page, key string, value any) {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	state := m.nativeSettings.pages[page]
	if state.applied == nil {
		state.applied = map[string]any{}
	}
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	state.applied[key] = value
	state.draft[key] = value
	m.nativeSettings.pages[page] = state
}

func TestNativeRuntimeSettingsLoadBeforePageNavigation(t *testing.T) {
	root := t.TempDir()
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "settings_get_global":
				return json.RawMessage(`{
					"editor":{
						"completion":{"autoPopup":false,"matchCase":false},
						"inlineCompletion":{"enabled":false,"onlyOnPause":true,"pauseMs":800,"disabledLanguages":["go"]}
					},
					"mcpServer":{"enabled":true,"runWithoutConfirmation":true},
					"languages":{"markdown":{"previewFontSize":18}}
				}`), nil
			case "settings_get":
				return json.RawMessage(`{
					"terminal":{"tabName":"ci-terminal","startDirectory":"tools","env":{"CI":"1"}},
					"actionsOnSave":{"optimizeImports":true,"cleanup":true},
					"run":{"confirmRerun":false,"confirmTerminate":false,"showOnStart":false,"terminationTimeoutMs":900}
				}`), nil
			default:
				t.Fatalf("command = %q", command)
				return nil, nil
			}
		},
	}
	m.setRoot(root)

	if err := m.loadNativeRuntimeSettings(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, page := range []string{
		settingsPageCodeCompletion,
		settingsPageInlineCompletion,
		settingsPageMarkdown,
		settingsPageTerminal,
		settingsPageActionsOnSave,
		settingsPageAgentMCPServer,
	} {
		if state := m.nativeSettings.pages[page]; !state.loaded || state.loading || state.err != "" {
			t.Fatalf("%s runtime state = %+v", page, state)
		}
	}
	if m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.autoPopup") {
		t.Fatal("runtime completion auto-popup = true, want false")
	}
	if m.nativeRuntimeBool(settingsPageInlineCompletion, "editor.inlineCompletion.enabled") {
		t.Fatal("runtime inline completion = true, want false")
	}
	if !m.nativeRuntimeBool(settingsPageInlineCompletion, "editor.inlineCompletion.onlyOnPause") ||
		m.nativeRuntimeNumber(settingsPageInlineCompletion, "editor.inlineCompletion.pauseMs") != 800 {
		t.Fatal("runtime inline completion pause settings were not loaded")
	}
	if got := m.nativeRuntimeString(settingsPageTerminal, "terminal.tabName"); got != "ci-terminal" {
		t.Fatalf("runtime terminal tab = %q", got)
	}
	if got := m.nativeRuntimeStringMap(settingsPageTerminal, "terminal.env")["CI"]; got != "1" {
		t.Fatalf("runtime terminal environment CI = %q", got)
	}
	if got := m.markdownPreviewFontSize(); got != 18 {
		t.Fatalf("markdown preview size = %v", got)
	}
	if !m.nativeRuntimeBool(settingsPageActionsOnSave, "actionsOnSave.optimizeImports") ||
		!m.nativeRuntimeBool(settingsPageActionsOnSave, "actionsOnSave.cleanup") {
		t.Fatal("runtime actions on save were not loaded")
	}
	if !m.nativeRuntimeBool(settingsPageAgentMCPServer, "mcpServer.enabled") ||
		!m.nativeRuntimeBool(settingsPageAgentMCPServer, "mcpServer.runWithoutConfirmation") {
		t.Fatal("runtime MCP server settings were not loaded")
	}
	if m.confirmRerunEnabled() || m.confirmTerminateEnabled() || m.showRunOnStart() ||
		m.runTerminationTimeout() != 900*time.Millisecond {
		t.Fatal("runtime Run settings were not applied to their native lifecycle")
	}
}

func TestNativeRuntimeSettingsIgnoreUnappliedDrafts(t *testing.T) {
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageCodeCompletion: {
			loaded: true,
			applied: map[string]any{
				"editor.completion.autoPopup": true,
			},
			draft: map[string]any{
				"editor.completion.autoPopup": false,
			},
		},
	}
	if !m.completionAutoPopup() {
		t.Fatal("an unapplied completion draft changed runtime behavior")
	}
	m.nativeSettings.pages[settingsPageCodeCompletion] = nativeSettingsPageState{
		loaded: true,
		applied: map[string]any{
			"editor.completion.autoPopup": false,
		},
		draft: map[string]any{
			"editor.completion.autoPopup": false,
		},
	}
	if m.completionAutoPopup() {
		t.Fatal("an applied completion value did not change runtime behavior")
	}
}
