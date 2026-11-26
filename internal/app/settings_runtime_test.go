package app

import (
	"context"
	"encoding/json"
	"path/filepath"
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
		cfg:      Config{SettingsPath: filepath.Join(root, "preferences.json")},
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "settings_get_global":
				return json.RawMessage(`{
					"editor":{
						"fontFamily":"Iosevka",
						"fontSize":15,
						"lineHeight":24,
						"ligatures":true,
						"softWrap":true,
						"showWhitespace":true,
						"formatOnSave":true,
						"general":{"wheelZoom":true,"dragAndDrop":false},
						"tabs":{"tabLimit":20,"showExtension":false},
						"inlay":{"codeVisionAuthor":false},
						"completion":{"autoPopup":false,"matchCase":false},
						"inlineCompletion":{"enabled":false,"onlyOnPause":true,"pauseMs":800,"disabledLanguages":["go"]}
					},
					"appearance":{"theme":"dark","showStatusBar":false},
					"agents":{
						"sendWith":"modEnter",
						"whileRunning":"steer",
						"showContextUsage":false,
						"defaults":{"codex":{"model":"gpt-test"}}
					},
					"agentSecurity":{"allowedCommands":["go test *"],"blockedCommands":["rm -rf *"]},
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
	if got := m.prefs.Font.Family; got != "Iosevka" {
		t.Fatalf("runtime font family = %q", got)
	}
	if m.prefs.fontSize() != 15 || m.prefs.Font.LineHeight != 24 ||
		!m.prefs.Font.Ligatures || !m.prefs.Editor.SoftWrap ||
		!m.prefs.Editor.ShowWhitespace || !m.prefs.General.WheelZoom ||
		m.prefs.General.dragAndDrop() || !m.prefs.General.ReformatOnSave {
		t.Fatalf("runtime editor preferences = %+v", m.prefs)
	}
	if m.prefs.Tabs.limit() != 20 || !m.prefs.Tabs.HideExtension ||
		m.prefs.Inlay.CodeVisionAuthor() {
		t.Fatalf("runtime tabs or inlay preferences = %+v / %+v", m.prefs.Tabs, m.prefs.Inlay)
	}
	if m.prefs.Theme != "dark" || !m.prefs.Appearance.HideStatusBar {
		t.Fatalf("runtime appearance preferences = %+v", m.prefs.Appearance)
	}
	if m.agentPrefs().sendWith() != "modEnter" ||
		m.agentPrefs().whileRunning() != "steer" ||
		!m.agentPrefs().NoContextUsage ||
		m.agentDefaultChoices("codex")["model"] != "gpt-test" {
		t.Fatalf("runtime agent preferences = %+v", m.agentPrefs())
	}
	if got := m.agentSecurityPrefs().AllowedCommands; len(got) != 1 || got[0] != "go test *" {
		t.Fatalf("runtime agent security = %+v", m.agentSecurityPrefs())
	}
	stored := LoadPreferences(m.cfg.SettingsPath)
	if stored.Font.Family != "Iosevka" || stored.Tabs.limit() != 20 ||
		stored.Theme != "dark" || stored.agentPrefsForTest().sendWith() != "modEnter" {
		t.Fatalf("runtime settings were not persisted locally: %+v", stored)
	}
}

func (p Preferences) agentPrefsForTest() AgentPrefs {
	if p.Agents == nil {
		return (AgentPrefs{}).normalized()
	}
	return p.Agents.normalized()
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

func TestNativeRuntimeCatalogConsumesFullSettingsRoot(t *testing.T) {
	global := map[string]any{
		"editor": map[string]any{
			"fontFamily":  "Iosevka",
			"fontSize":    float64(15),
			"breadcrumbs": map[string]any{"show": true, "placement": "top"},
			"general":     map[string]any{"wheelZoom": true},
		},
		"agents": map[string]any{
			"sendWith": "modEnter",
			"defaults": map[string]any{"codex": map[string]any{"model": "gpt-test"}},
		},
	}
	project := map[string]any{
		"editor": map[string]any{
			"fontSize":    float64(17),
			"breadcrumbs": map[string]any{"placement": "bottom"},
		},
		"general": map[string]any{"confirmExit": false},
	}
	values := nativeSettingsPageValuesFromRuntimeRoot(
		mergeNativeSettingRoots(global, project),
		nativeRuntimeSettingsDefinitions()[3].sections,
		"global",
	)
	if got := values["editor.fontSize"]; got != float64(17) {
		t.Fatalf("project font size = %#v, want 17", got)
	}
	if got := values["editor.breadcrumbs.show"]; got != true {
		t.Fatalf("global breadcrumbs show = %#v, want true", got)
	}
	if got := values["editor.breadcrumbs.placement"]; got != "bottom" {
		t.Fatalf("project breadcrumb placement = %#v, want bottom", got)
	}
}

func TestNativeRuntimeCatalogMapsOnlyConsumedKeys(t *testing.T) {
	values := map[string]any{
		"agents.sendWith":                         "modEnter",
		"agents.thinking":                         "collapsed",
		"agents.showContextUsage":                 false,
		"appearance.theme":                        "dark",
		"editor.fontFamily":                       "Iosevka",
		"editor.fontSize":                         float64(15),
		"editor.general.wheelZoom":                true,
		"editor.tabs.tabLimit":                    float64(20),
		"editor.inlay.codeVisionUsages":           false,
		"files.trimTrailingWhitespace":            true,
		"files.ensureFinalNewline":                true,
		"general.powerSaveMode":                   true,
		"general.autoCheckUpdates":                false,
		"general.showWhatsNewAfterUpdate":         false,
		"general.updateSource":                    "github",
		"general.saveOnIdle":                      true,
		"general.idleSeconds":                     float64(45),
		"files.autoSave":                          false,
		"files.syncOnFocus":                       false,
		"terminal.tabName":                        "CI",
		"terminal.useEditorFont":                  true,
		"agentSecurity.allowedCommands":           []string{"go test *"},
		"agentSecurity.blockedCommands":           []string{"rm -rf *"},
		"editor.general.removeTrailingBlankLines": true,
	}
	values["general.confirmExit"] = false
	m := &Model{}
	defs := nativeRuntimeSettingsDefinitions()
	m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	for _, def := range defs {
		pageValues := map[string]any{}
		for _, section := range def.sections {
			for _, field := range section.fields {
				if value, ok := values[field.key]; ok {
					pageValues[field.key] = value
				}
			}
		}
		m.nativeSettings.pages[def.page.id] = nativeSettingsPageState{
			loaded: true, applied: pageValues, draft: pageValues,
		}
	}
	if !m.applyNativePreferenceRuntimeSettings(defs) {
		t.Fatal("native runtime mapping did not report a preference change")
	}
	if m.agentPrefs().sendWith() != "modEnter" ||
		m.agentPrefs().thinking() != "collapsed" ||
		!m.agentPrefs().NoContextUsage {
		t.Fatalf("agent runtime mapping = %+v", m.agentPrefs())
	}
	if m.prefs.Font.Family != "Iosevka" || m.prefs.fontSize() != 15 ||
		!m.prefs.General.WheelZoom ||
		m.prefs.Tabs.limit() != 20 ||
		m.prefs.Inlay.CodeVisionUsages() {
		t.Fatalf("editor runtime mapping = %+v / %+v", m.prefs, m.prefs.Tabs)
	}
	if !m.prefs.SaveFiles.TrimTrailing ||
		!m.prefs.SaveFiles.DropTrailingBlankLines ||
		!m.prefs.SaveFiles.EnsureFinalNewline {
		t.Fatalf("save runtime mapping = %+v", m.prefs.SaveFiles)
	}
	if m.autoCheckUpdates() || m.showWhatsNewAfterUpdate() ||
		m.prefs.Updates == nil || m.prefs.Updates.Source != "github" {
		t.Fatalf("update runtime mapping = %+v", m.prefs.Updates)
	}
	if m.prefs.System.autoSave() || m.prefs.System.syncOnFocus() ||
		!m.prefs.System.SaveOnIdle || m.prefs.System.idleSeconds() != 45 {
		t.Fatalf("system save runtime mapping = %+v", m.prefs.System)
	}
	if m.prefs.Theme != "dark" {
		t.Fatalf("appearance runtime mapping = %+v", m.prefs.Appearance)
	}
	if got := m.agentSecurityPrefs().AllowedCommands; len(got) != 1 || got[0] != "go test *" {
		t.Fatalf("security runtime mapping = %+v", got)
	}
	if m.nativeRuntimeBool("native.runtime.general", "general.confirmExit") {
		t.Fatal("native runtime reading did not alias the loaded key")
	}
	if m.nativeRuntimeString("native.runtime.terminal", "terminal.tabName") != "CI" {
		t.Fatal("native runtime terminal reading did not alias the loaded key")
	}
}

func TestNativeRuntimeCatalogKeysHaveOneOwner(t *testing.T) {
	owners := map[string]string{}
	for _, def := range nativeRuntimeSettingsDefinitions() {
		for _, section := range def.sections {
			for _, field := range section.fields {
				if previous, ok := owners[field.key]; ok {
					t.Fatalf("%q appears in %s and %s", field.key, previous, def.page.id)
				}
				owners[field.key] = def.page.id
			}
		}
	}
}

func TestNativeRuntimeCatalogUsesAppliedVisiblePageValues(t *testing.T) {
	m := &Model{}
	defs := nativeRuntimeSettingsDefinitions()
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		"native.runtime.editor": {
			loaded:  true,
			applied: map[string]any{"editor.fontSize": float64(13)},
		},
		settingsPageEditorFont: {
			loaded:  true,
			applied: map[string]any{"editor.fontSize": float64(19)},
		},
	}
	if !m.applyNativePreferenceRuntimeSettings(defs) {
		t.Fatal("native runtime mapping did not report a preference change")
	}
	if m.prefs.fontSize() != 19 {
		t.Fatalf("visible applied font size = %v, want 19", m.prefs.fontSize())
	}
}
