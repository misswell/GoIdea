package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func languageServerFixture() []openidea.ToolInfo {
	path := "/tools/jdtls"
	version := "1.44.0"
	initOptions := `{"bundles":[]}`
	install := "managed"
	return []openidea.ToolInfo{
		{
			Name: "jdtls", DisplayName: "Eclipse JDT Language Server",
			Kind: "languageServer", Command: "jdtls", Languages: []string{"java"},
			Status: "installed", Path: &path, Version: &version,
			CanInstall: true, CanUninstall: true, InstallMethod: &install,
			DefaultArgs:                  []string{"-data", "{workspace}"},
			DefaultInitializationOptions: &initOptions,
			Alternatives:                 []string{"jdtls"},
		},
		{
			Name: "java-debug", DisplayName: "Java Debug",
			Kind: "debugger", Command: "java-debug", Languages: []string{"java"},
			Status: "missing", CanInstall: true,
		},
		{
			Name: "gopls", DisplayName: "Go Language Server",
			Kind: "languageServer", Command: "gopls", Languages: []string{"go"},
			Status: "missing", CanInstall: true,
			Alternatives: []string{"gopls"},
		},
	}
}

func waitForLanguageTest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for language server work")
	}
}

func TestLanguageToolRowsMapBackendFieldsAndOrderFilteredTools(t *testing.T) {
	rows := languageToolRows(languageServerFixture())
	if len(rows) != 3 {
		t.Fatalf("language tool rows = %+v", rows)
	}
	jdtls := rows[0]
	if jdtls.Name != "jdtls" || jdtls.Display != "Eclipse JDT Language Server" ||
		jdtls.Kind != "languageServer" || jdtls.Command != "jdtls" ||
		jdtls.Path != "/tools/jdtls" || jdtls.Version != "1.44.0" ||
		len(jdtls.DefaultArgs) != 2 || jdtls.DefaultArgs[1] != "{workspace}" ||
		jdtls.DefaultInit != `{"bundles":[]}` || len(jdtls.Alternatives) != 1 {
		t.Fatalf("mapped jdtls row = %+v", jdtls)
	}

	shown := filterLanguageTools(rows, "", languageToolFilterAll)
	if shown[0].Name != "jdtls" || shown[1].Name != "java-debug" || shown[2].Name != "gopls" {
		t.Fatalf("all tools should put ready tools first: %+v", shown)
	}
	if got := filterLanguageTools(rows, "", languageToolFilterAvailable); len(got) != 1 || got[0].Name != "jdtls" {
		t.Fatalf("available tools = %+v", got)
	}
	if got := filterLanguageTools(rows, "debug", languageToolFilterMissing); len(got) != 1 || got[0].Name != "java-debug" {
		t.Fatalf("missing debugger search = %+v", got)
	}
	if got := filterLanguageTools(rows, "JAVA", languageToolFilterAll); len(got) != 2 {
		t.Fatalf("case-insensitive language search = %+v", got)
	}
	if got := languageToolsMissingCount(rows); got != 2 {
		t.Fatalf("missing count = %d", got)
	}
}

func TestLanguageServersGroupFilterAndPageRegistration(t *testing.T) {
	if _, ok := settingsPageByID(settingsPageLanguageServers); !ok {
		t.Fatal("Language Servers settings page is not registered")
	}
	m := &Model{}
	if !m.ActionEnabled("tools.languageServers") || hiddenActionIDs["tools.languageServers"] {
		t.Fatal("tools.languageServers is not a visible registered action")
	}
}

func languageServersTester(m *Model) *ui.Tester {
	tester := ui.NewTester(func(c *ui.Context) {
		p := c.Theme()
		m.settingsLanguageServersBody(c, p, oiPaletteOf(p))
	}, 900, 760)
	tester.Frame()
	return tester
}

func TestLanguageServersPageRendersAvailableMissingAndSelectedTool(t *testing.T) {
	rows := languageToolRows(languageServerFixture())
	m := &Model{
		root:             "/project",
		languageTools:    rows,
		toolsLoaded:      true,
		languageToolPick: "jdtls",
		languageToolSettings: languageToolSettingsState{
			loaded: true, scope: "project",
			applied: map[string]languageServerConfig{
				"jdtls": {Args: []string{"-data", "custom"}},
			},
			draft: map[string]languageServerConfig{
				"jdtls": {Args: []string{"-data", "custom"}},
			},
			pathBase:  map[string]string{},
			pathDraft: map[string]string{},
			languages: []openidea.LanguageInfo{{ID: "java", Name: "Java"}},
		},
	}
	tester := languageServersTester(m)
	for _, text := range []string{
		"Global", "This project",
		"Saved to this project's GoIDEA settings; overrides the global value.",
		"Available", "Missing (2)", "All",
		"Eclipse JDT Language Server", "Java Debug", "Reinstall", "Uninstall",
		"Use Eclipse JDT Language Server for Java files", "Executable", "Path",
		"Startup", "Arguments", "Environment variables", "Initialization options",
		"Workspace Configuration", "Settings", "Restore Defaults",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Language Servers page is missing %q: %v", text, tester.Texts())
		}
	}

	if err := tester.Click("Missing (2)"); err != nil {
		t.Fatal(err)
	}
	if m.languageToolFilter != languageToolFilterMissing {
		t.Fatalf("missing filter = %q", m.languageToolFilter)
	}
	if m.languageToolPick != "java-debug" {
		t.Fatalf("selection after filtering = %q", m.languageToolPick)
	}
}

func TestLanguageServersPageShowsInstallAlternativesAndNewServerModal(t *testing.T) {
	rows := languageToolRows([]openidea.ToolInfo{
		{Name: "basedpyright", DisplayName: "BasedPyright", Kind: "languageServer",
			Command: "basedpyright-langserver", Languages: []string{"python"}, Status: "missing",
			CanInstall: true, Alternatives: []string{"basedpyright", "pyright"}},
		{Name: "pyright", DisplayName: "Pyright", Kind: "languageServer",
			Command: "pyright-langserver", Languages: []string{"python"}, Status: "missing",
			CanInstall: true, Alternatives: []string{"basedpyright", "pyright"}},
	})
	m := &Model{
		languageTools: rows, toolsLoaded: true, languageToolPick: "basedpyright",
		languageToolSettings: languageToolSettingsState{
			loaded: true, scope: "global", draft: map[string]languageServerConfig{},
			applied: map[string]languageServerConfig{}, pathBase: map[string]string{},
			pathDraft: map[string]string{},
			languages: []openidea.LanguageInfo{{ID: "python", Name: "Python"}},
		},
	}
	tester := languageServersTester(m)
	for _, text := range []string{"Install", "Language server", "Automatic", "BasedPyright", "Pyright"} {
		if !tester.HasText(text) {
			t.Fatalf("alternative picker is missing %q: %v", text, tester.Texts())
		}
	}

	m.languageToolSettings.addOpen = true
	tester = languageServersTester(m)
	for _, text := range []string{"New Language Server", "Command", "Name", "Python", "OK", "Cancel"} {
		if !tester.HasText(text) {
			t.Fatalf("new-server modal is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestLanguageServerSettingsLoadScopeAndDraftLifecycle(t *testing.T) {
	done := make(chan struct{}, 1)
	signal := func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}
	var commands []string
	m := &Model{
		ctx: context.Background(), root: "/project",
		cfg:      Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		postHook: func(fn func()) { fn(); signal() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			commands = append(commands, command)
			switch command {
			case "settings_get":
				return json.RawMessage(`{
					"languageServers":{"custom":{"command":"custom-ls","languages":["go"]}},
					"tools":{"jdtls":{"path":"/tools/jdtls","other":true},"pa.cmd":{"path":"/tools/dotted","keep":"value"}}
				}`), nil
			case "languages":
				return json.RawMessage(`[{"id":"go","name":"Go"},{"id":"python","name":"Python"}]`), nil
			default:
				t.Fatalf("unexpected RPC %q", command)
				return nil, nil
			}
		},
	}
	m.OpenSettingsPage(settingsPageLanguageServers)
	waitForLanguageTest(t, done)
	state := &m.languageToolSettings
	if !m.settingsOpen || m.settingsPage != settingsPageLanguageServers || !state.loaded || state.loading {
		t.Fatalf("settings did not load: open=%v page=%q state=%+v", m.settingsOpen, m.settingsPage, state)
	}
	if state.scope != "project" || len(commands) != 2 || commands[0] != "settings_get" || commands[1] != "languages" {
		t.Fatalf("load scope/commands = %q %v", state.scope, commands)
	}
	if state.pathBase["jdtls"] != "/tools/jdtls" || state.pathBase["pa.cmd"] != "/tools/dotted" {
		t.Fatalf("path overrides = %+v", state.pathBase)
	}
	if !strings.Contains(string(state.toolsRaw["jdtls"]), `"other":true`) {
		t.Fatalf("unrelated tool fields were not retained: %s", state.toolsRaw["jdtls"])
	}
	if len(state.languages) != 2 || state.languages[0].Name != "Go" {
		t.Fatalf("languages = %+v", state.languages)
	}
	if m.settingsDirtyPage(settingsPageLanguageServers) || m.settingsPending() {
		t.Fatal("freshly loaded language settings are dirty")
	}

	m.stageLanguageToolArgs("custom", `--stdio "quoted value"`)
	if !m.settingsDirtyPage(settingsPageLanguageServers) || !m.settingsPending() {
		t.Fatal("staged language setting is not dirty")
	}
	m.closeSettings()
	if m.settingsOpen {
		t.Fatal("settings did not close")
	}
	if languageToolSettingsDirty(state) {
		t.Fatal("Cancel did not discard language server drafts")
	}
	if state.session || state.scopeChosen || state.scope != "" || state.scopes != nil {
		t.Fatalf("Cancel did not end the Settings session: %+v", state)
	}

	m.OpenSettingsPage(settingsPageLanguageServers)
	waitForLanguageTest(t, done)
	if !state.session || !state.loaded || state.scope != "project" {
		t.Fatalf("reopened Settings reused stale state: %+v", state)
	}
	if len(commands) != 4 || commands[2] != "settings_get" || commands[3] != "languages" {
		t.Fatalf("reopened Settings did not reload both layers: %v", commands)
	}
}

func TestLanguageServerSettingsDefaultToGlobalWithoutProject(t *testing.T) {
	done := make(chan struct{}, 1)
	var commands []string
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			done <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			commands = append(commands, command)
			if command == "languages" {
				return json.RawMessage(`[]`), nil
			}
			return json.RawMessage(`{"languageServers":{},"tools":{}}`), nil
		},
	}
	m.OpenSettingsPage(settingsPageLanguageServers)
	waitForLanguageTest(t, done)
	state := &m.languageToolSettings
	if state.scope != "global" || !state.scopeChosen || !state.session {
		t.Fatalf("default scope without a project = %+v", state)
	}
	if len(commands) != 2 || commands[0] != "settings_get_global" || commands[1] != "languages" {
		t.Fatalf("default global load commands = %v", commands)
	}
}

func TestLanguageServerSettingsScopeSwitchLoadsLayerAndRestoresDraft(t *testing.T) {
	done := make(chan struct{}, 1)
	signal := func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}
	m := &Model{
		ctx: context.Background(), root: "/project",
		postHook: func(fn func()) { fn(); signal() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "settings_get":
				return json.RawMessage(`{
					"languageServers":{"custom":{"command":"project-ls","languages":["go"]}},
					"tools":{}
				}`), nil
			case "settings_get_global":
				return json.RawMessage(`{
					"languageServers":{"custom":{"command":"global-ls","languages":["python"]}},
					"tools":{}
				}`), nil
			case "languages":
				return json.RawMessage(`[{"id":"go","name":"Go"},{"id":"python","name":"Python"}]`), nil
			default:
				t.Fatalf("unexpected RPC %q", command)
				return nil, nil
			}
		},
	}
	m.OpenSettingsPage(settingsPageLanguageServers)
	waitForLanguageTest(t, done)
	m.stageLanguageToolArgs("custom", "--project")
	tester := languageServersTester(m)
	if err := tester.Click("Global"); err != nil {
		t.Fatal(err)
	}
	waitForLanguageTest(t, done)
	state := &m.languageToolSettings
	if state.scope != "global" || state.draft["custom"].Command == nil ||
		*state.draft["custom"].Command != "global-ls" {
		t.Fatalf("global scope did not load its own layer: %+v", state)
	}
	m.stageLanguageToolArgs("custom", "--global")
	m.SetLanguageServerScope("project")
	if state.scope != "project" || state.draft["custom"].Command == nil ||
		*state.draft["custom"].Command != "project-ls" ||
		formatLanguageToolArgs(state.draft["custom"].Args) != "--project" {
		t.Fatalf("project draft was not restored: %+v", state.draft["custom"])
	}
}

func TestLanguageServerSettingsSaveWritesEveryDirtyScope(t *testing.T) {
	done := make(chan struct{}, 1)
	var writes []map[string]any
	m := &Model{
		ctx: context.Background(), root: "/project",
		postHook: func(fn func()) { fn(); done <- struct{}{} },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Fatalf("unexpected RPC %q", command)
			}
			args, _ := input.(map[string]any)
			writes = append(writes, args)
			return json.RawMessage(`{"ok":true}`), nil
		},
	}
	project := &languageServerScopeState{
		loaded: true,
		applied: map[string]languageServerConfig{
			"custom": {Command: stringRef("project-ls"), Languages: []string{"go"}},
		},
		draft: map[string]languageServerConfig{
			"custom": {Command: stringRef("project-ls"), Languages: []string{"go"}, Args: []string{"--project"}},
		},
		pathBase:  map[string]string{},
		pathDraft: map[string]string{},
		toolsRaw:  map[string]json.RawMessage{},
		editors:   map[string]*languageToolEditorState{},
	}
	global := &languageServerScopeState{
		loaded: true,
		applied: map[string]languageServerConfig{
			"custom": {Command: stringRef("global-ls"), Languages: []string{"python"}},
		},
		draft: map[string]languageServerConfig{
			"custom": {Command: stringRef("global-ls"), Languages: []string{"python"}, Args: []string{"--global"}},
		},
		pathBase:  map[string]string{},
		pathDraft: map[string]string{},
		toolsRaw:  map[string]json.RawMessage{},
		editors:   map[string]*languageToolEditorState{},
	}
	for _, scope := range []string{"project", "global"} {
		cached := project
		if scope == "global" {
			cached = global
		}
		for name, entry := range cached.draft {
			cached.editors[name] = languageToolEditorFrom(entry)
		}
	}
	m.languageToolSettings.session = true
	m.languageToolSettings.scope = "project"
	m.languageToolSettings.scopeChosen = true
	m.languageToolSettings.scopes = map[string]*languageServerScopeState{
		"project": project,
		"global":  global,
	}
	m.restoreLanguageServerScope(project)
	if !m.settingsDirtyPage(settingsPageLanguageServers) {
		t.Fatal("drafts in both scopes did not mark the page dirty")
	}

	m.saveLanguageServerSettings(false)
	waitForLanguageTest(t, done)
	if len(writes) != 2 {
		t.Fatalf("settings_set calls = %#v", writes)
	}
	got := map[string]string{}
	for _, write := range writes {
		scope, _ := write["scope"].(string)
		value, _ := write["value"].(string)
		got[scope] = value
	}
	if got["project"] != `["--project"]` || got["global"] != `["--global"]` {
		t.Fatalf("saved scope values = %#v", got)
	}
	state := &m.languageToolSettings
	if state.saving || languageToolSettingsDirty(state) {
		t.Fatalf("cross-scope save did not settle: %+v", state)
	}
	if formatLanguageToolArgs(state.scopes["global"].applied["custom"].Args) != "--global" {
		t.Fatalf("global applied value = %#v", state.scopes["global"].applied)
	}
}

func TestLanguageServerSettingsIgnoreLateLoadFromOldScope(t *testing.T) {
	done := make(chan struct{}, 3)
	globalStarted := make(chan struct{})
	releaseGlobal := make(chan struct{})
	m := &Model{
		ctx: context.Background(), root: "/project",
		postHook: func(fn func()) {
			fn()
			select {
			case done <- struct{}{}:
			default:
			}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "settings_get_global":
				close(globalStarted)
				<-releaseGlobal
				return json.RawMessage(`{
					"languageServers":{"custom":{"command":"old-global","languages":["go"]}},
					"tools":{}
				}`), nil
			case "settings_get":
				return json.RawMessage(`{
					"languageServers":{"custom":{"command":"new-project","languages":["go"]}},
					"tools":{}
				}`), nil
			case "languages":
				return json.RawMessage(`[]`), nil
			default:
				t.Fatalf("unexpected RPC %q", command)
				return nil, nil
			}
		},
	}
	m.languageToolSettings.session = true
	m.languageToolSettings.scopeChosen = true
	m.loadLanguageServerScope("global")
	<-globalStarted
	m.loadLanguageServerScope("project")
	waitForLanguageTest(t, done)
	close(releaseGlobal)
	waitForLanguageTest(t, done)
	state := &m.languageToolSettings
	if state.scope != "project" || state.draft["custom"].Command == nil ||
		*state.draft["custom"].Command != "new-project" {
		t.Fatalf("late global reply overwrote the current scope: %+v", state)
	}
	if state.scopes["global"] != nil {
		t.Fatalf("late global reply was cached: %+v", state.scopes["global"])
	}
}

func TestLanguageServerSettingsValidateErrorsInInactiveScope(t *testing.T) {
	done := make(chan struct{}, 1)
	invoked := false
	m := &Model{
		ctx: context.Background(), root: "/project",
		postHook: func(fn func()) { fn(); done <- struct{}{} },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			invoked = true
			t.Fatalf("settings_set ran despite a staged validation error: %q", command)
			return nil, nil
		},
	}
	project := &languageServerScopeState{
		loaded: true,
		applied: map[string]languageServerConfig{
			"custom": {Command: stringRef("custom-ls"), Languages: []string{"go"}},
		},
		draft: map[string]languageServerConfig{
			"custom": {Command: stringRef("changed-ls"), Languages: []string{"go"}},
		},
		pathBase:  map[string]string{},
		pathDraft: map[string]string{},
		toolsRaw:  map[string]json.RawMessage{},
		editors: map[string]*languageToolEditorState{
			"custom": {Errors: map[string]string{"args": "Expected valid arguments"}},
		},
	}
	global := &languageServerScopeState{
		loaded:    true,
		applied:   map[string]languageServerConfig{},
		draft:     map[string]languageServerConfig{},
		pathBase:  map[string]string{},
		pathDraft: map[string]string{},
		toolsRaw:  map[string]json.RawMessage{},
		editors:   map[string]*languageToolEditorState{},
	}
	m.languageToolSettings.scope = "global"
	m.languageToolSettings.scopes = map[string]*languageServerScopeState{
		"project": project,
		"global":  global,
	}
	m.restoreLanguageServerScope(global)
	m.settingsOpen = true
	m.saveLanguageServerSettings(false)
	if invoked || m.languageToolSettings.saving {
		t.Fatal("save started despite the inactive scope's validation error")
	}
	if !strings.Contains(m.settingsErr, "Expected valid arguments") {
		t.Fatalf("settings error = %q", m.settingsErr)
	}
}

func TestLanguageServerSettingsSaveWritesFieldLevelChangesInScope(t *testing.T) {
	for _, scope := range []string{"project", "global"} {
		t.Run(scope, func(t *testing.T) {
			done := make(chan struct{}, 1)
			signal := func() {
				select {
				case done <- struct{}{}:
				default:
				}
			}
			var gotCommand string
			var gotArgs map[string]any
			m := &Model{
				ctx: context.Background(), root: "/project",
				postHook: func(fn func()) { fn(); signal() },
				invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
					gotCommand = command
					gotArgs, _ = input.(map[string]any)
					return json.RawMessage(`{"ok":true}`), nil
				},
			}
			state := &m.languageToolSettings
			state.loaded = true
			state.scope = scope
			state.applied = map[string]languageServerConfig{
				"custom": {Command: stringRef("custom-ls"), Args: []string{"--old"}, Languages: []string{"go"}},
			}
			state.draft = map[string]languageServerConfig{
				"custom": {Command: stringRef("custom-ls"), Args: []string{"--new"}, Languages: []string{"go"}},
			}
			state.pathBase = map[string]string{}
			state.pathDraft = map[string]string{}
			state.toolsRaw = map[string]json.RawMessage{}
			state.editors = map[string]*languageToolEditorState{
				"custom": languageToolEditorFrom(state.draft["custom"]),
			}

			m.saveLanguageServerSettings(false)
			waitForLanguageTest(t, done)
			if gotCommand != "settings_set" {
				t.Fatalf("save command = %q", gotCommand)
			}
			if gotArgs["scope"] != scope || gotArgs["key"] != "languageServers.custom.args" ||
				gotArgs["value"] != `["--new"]` {
				t.Fatalf("settings_set args = %#v", gotArgs)
			}
			if state.saving || languageToolSettingsDirty(state) || state.err != "" {
				t.Fatalf("save did not settle: %+v", state)
			}
		})
	}
}

func TestLanguageServerSettingWritesUseJSONNullForRemovals(t *testing.T) {
	applied := map[string]languageServerConfig{
		"builtin": {Enabled: boolRef(false), Args: []string{"--old"}},
		"custom":  {Command: stringRef("custom-ls"), Languages: []string{"go"}},
	}
	draft := map[string]languageServerConfig{
		"builtin": {Args: nil},
	}
	values := map[string]any{}
	for _, write := range languageServerSettingWrites(applied, draft) {
		values[write.key] = write.value
	}
	if value, ok := values["languageServers.custom"]; !ok || value != nil {
		t.Fatalf("custom server removal = %#v", values)
	}
	for _, key := range []string{
		"languageServers.builtin.enabled",
		"languageServers.builtin.args",
	} {
		if value, ok := values[key]; !ok || value != nil {
			t.Fatalf("%s removal = %#v", key, values)
		}
	}
}

func TestLanguageToolPathWritesRemoveDefaultsAndPreserveDottedToolFields(t *testing.T) {
	if writes := languageToolPathWrites(
		map[string]string{"plain": "/old"},
		map[string]string{},
		nil,
	); len(writes) != 1 || writes[0].key != "tools.plain.path" || writes[0].value != nil {
		t.Fatalf("plain path removal = %+v", writes)
	}

	toolsRaw := map[string]json.RawMessage{
		"dotted.ls": json.RawMessage(`{"path":"/old","env":{"A":"B"},"args":["--x"]}`),
		"other":     json.RawMessage(`{"path":"/keep","enabled":false}`),
	}
	writes := languageToolPathWrites(
		map[string]string{"dotted.ls": "/old"},
		map[string]string{"dotted.ls": "/new"},
		toolsRaw,
	)
	if len(writes) != 1 || writes[0].key != "tools" {
		t.Fatalf("dotted path writes = %+v", writes)
	}
	next, ok := writes[0].value.(map[string]json.RawMessage)
	if !ok {
		t.Fatalf("tools value type = %T", writes[0].value)
	}
	var dotted map[string]any
	if err := json.Unmarshal(next["dotted.ls"], &dotted); err != nil {
		t.Fatal(err)
	}
	if dotted["path"] != "/new" || dotted["args"] == nil || dotted["env"] == nil {
		t.Fatalf("dotted tool fields changed: %#v", dotted)
	}
	if string(next["other"]) != string(toolsRaw["other"]) {
		t.Fatalf("unrelated tool changed: %s", next["other"])
	}
}

func TestRestoreLanguageServerDefaultsRemovesPathAndConfiguration(t *testing.T) {
	m := &Model{
		languageTools: languageToolRows(languageServerFixture()),
		languageToolSettings: languageToolSettingsState{
			loaded: true,
			draft: map[string]languageServerConfig{
				"jdtls": {Args: []string{"--custom"}},
			},
			pathDraft: map[string]string{"jdtls": "/custom/jdtls"},
		},
	}
	m.restoreLanguageServerDefaults("jdtls")
	if len(m.languageToolSettings.draft) != 0 || len(m.languageToolSettings.pathDraft) != 0 {
		t.Fatalf("restore defaults left %+v / %+v", m.languageToolSettings.draft, m.languageToolSettings.pathDraft)
	}
}

func TestToolInfoDecodesLanguageServerParityFields(t *testing.T) {
	var info openidea.ToolInfo
	raw := json.RawMessage(`{
		"name":"jdtls","displayName":"JDT","kind":"languageServer","command":"jdtls",
		"languages":["java"],"status":"installed","canInstall":true,
		"defaultArgs":["-data","{workspace}"],
		"defaultInitializationOptions":"{\"bundles\":[]}",
		"alternatives":["jdtls","java-ls"],
		"customError":"restart required"
	}`)
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if len(info.DefaultArgs) != 2 || info.DefaultInitializationOptions == nil ||
		*info.DefaultInitializationOptions != `{"bundles":[]}` ||
		len(info.Alternatives) != 2 || info.CustomError == nil ||
		*info.CustomError != "restart required" {
		t.Fatalf("decoded ToolInfo = %+v", info)
	}
}

func TestToolsInstallActionsMatchOpenIDEAState(t *testing.T) {
	m := &Model{}
	for _, id := range []string{"tools.languageServers", "tools.install", "tools.promptInstall"} {
		if !m.ActionEnabled(id) {
			t.Fatalf("%s is disabled without a project", id)
		}
	}
	if !hiddenActionIDs["tools.install"] || !hiddenActionIDs["tools.promptInstall"] {
		t.Fatal("tools.install and tools.promptInstall must stay hidden from the palette")
	}
}

func TestPromptInstallLanguageToolOffersActionsWithoutInstalling(t *testing.T) {
	m := &Model{
		languageTools: languageToolRows(languageServerFixture()),
		toolsLoaded:   true,
	}
	m.PromptInstallLanguageTool("java-debug", true, false)
	if len(m.notify.items) != 1 {
		t.Fatalf("install prompt count = %d", len(m.notify.items))
	}
	prompt := m.notify.items[0]
	if prompt.key != "lsp-missing-java-debug" || !prompt.sticky ||
		prompt.title != "Java Debug is not installed" ||
		!strings.Contains(prompt.body, "Code insight for java needs java-debug") ||
		len(prompt.actions) != 2 ||
		prompt.actions[0].label != "Install" ||
		prompt.actions[1].label != "Language Servers…" {
		t.Fatalf("install prompt = %+v", prompt)
	}
	if m.toolsBusy != "" {
		t.Fatalf("prompt started an installer immediately: %q", m.toolsBusy)
	}
}

func TestPromptInstallLanguageToolSuppressesFallbacksAndServedTools(t *testing.T) {
	m := &Model{
		languageTools: []javaTool{
			{Name: "basedpyright", Display: "BasedPyright", Kind: "languageServer",
				Languages: []string{"Python"}, Status: "missing", CanInstall: true,
				Alternatives: []string{"basedpyright", "pyright"}},
			{Name: "pyright", Display: "Pyright", Kind: "languageServer",
				Languages: []string{"Python"}, Status: "missing", CanInstall: true,
				Alternatives: []string{"basedpyright", "pyright"}},
			{Name: "typescript-language-server", Display: "TypeScript Language Server",
				Kind: "languageServer", Languages: []string{"TypeScript"}, Status: "onPath"},
			{Name: "vtsls", Display: "vtsls", Kind: "languageServer",
				Languages: []string{"TypeScript"}, Status: "missing", CanInstall: true,
				Alternatives: []string{"vtsls"}},
		},
		toolsLoaded: true,
	}
	m.PromptInstallLanguageTool("basedpyright", true, false)
	if len(m.notify.items) != 1 {
		t.Fatalf("preferred Python tool did not prompt: %+v", m.notify.items)
	}
	m.notify = notifier{}
	m.PromptInstallLanguageTool("pyright", true, false)
	if len(m.notify.items) != 0 {
		t.Fatalf("Python fallback prompted after its preferred tool: %+v", m.notify.items)
	}
	m.PromptInstallLanguageTool("vtsls", true, false)
	if len(m.notify.items) != 0 {
		t.Fatalf("vtsls prompted while typescript-language-server already serves it: %+v", m.notify.items)
	}
}

func TestLanguageServerMissingAndWarningEventsOfferInstall(t *testing.T) {
	tools := languageServerFixture()
	tools = append(tools, openidea.ToolInfo{
		Name: "vue-language-server", DisplayName: "Vue Language Server",
		Kind: "languageServer", Command: "vue-language-server",
		Languages: []string{"Vue"}, Status: "missing", CanInstall: true,
		Alternatives: []string{"vue-language-server"},
	})
	m := &Model{languageTools: languageToolRows(tools), toolsLoaded: true}
	m.acceptLanguageServerEvent(openidea.LanguageServerEvent{
		Name: "java-debug", Status: "missing",
	})
	if len(m.notify.items) != 1 || m.notify.items[0].key != "lsp-missing-java-debug" {
		t.Fatalf("missing languageServer event did not prompt: %+v", m.notify.items)
	}
	m.acceptLanguageServerEvent(openidea.LanguageServerEvent{
		Name: "java-debug", Status: "missing",
	})
	if len(m.notify.items) != 1 {
		t.Fatalf("duplicate missing event prompted again: %+v", m.notify.items)
	}

	m.acceptLanguageServerWarning(openidea.LanguageServerWarningEvent{
		Name: "Vue", Message: "Vue needs the managed server for component support.",
		InstallTool: "vue-language-server",
	})
	if len(m.notify.items) != 2 {
		t.Fatalf("languageServerWarning did not post: %+v", m.notify.items)
	}
	warning := m.notify.items[0]
	if warning.key != "lsp-warning-Vue-vue-language-server" ||
		warning.title != "Vue" || len(warning.actions) != 2 ||
		warning.actions[0].label != "Install" ||
		warning.actions[1].label != "Language Servers…" {
		t.Fatalf("languageServerWarning prompt = %+v", warning)
	}
	m.acceptLanguageServerWarning(openidea.LanguageServerWarningEvent{
		Name: "Vue", Message: "again", InstallTool: "vue-language-server",
	})
	if len(m.notify.items) != 2 {
		t.Fatalf("duplicate languageServerWarning prompted again: %+v", m.notify.items)
	}
}

func TestInstallLanguageToolUsesBackendStreamAndRefreshesState(t *testing.T) {
	done := make(chan struct{})
	var commands []string
	toolLists := 0
	var m *Model
	m = &Model{
		ctx: context.Background(), root: "/project",
		settingsOpen: true, settingsPage: settingsPageLanguageServers,
		languageToolSettings: languageToolSettingsState{
			session: true, loaded: true, scope: "project",
			applied: map[string]languageServerConfig{},
			draft:   map[string]languageServerConfig{},
		},
		postHook: func(fn func()) {
			fn()
			if m.toolsBusy == "" {
				select {
				case <-done:
				default:
					close(done)
				}
			}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			commands = append(commands, command)
			switch command {
			case "tools_list":
				toolLists++
				if toolLists == 1 {
					return json.RawMessage(`[{
						"name":"gopls","displayName":"Go Language Server","kind":"languageServer",
						"command":"gopls","languages":["go"],"status":"missing","canInstall":true
					}]`), nil
				}
				return json.RawMessage(`[{
					"name":"gopls","displayName":"Go Language Server","kind":"languageServer",
					"command":"gopls","languages":["go"],"status":"installed",
					"path":"/tools/gopls","version":"1.2.3","canInstall":true
				}]`), nil
			case "tools_install":
				args, _ := input.(map[string]any)
				if args["name"] != "gopls" || args["request"] == uint32(0) {
					t.Fatalf("tools_install args = %#v", args)
				}
				return json.RawMessage(`{
					"name":"gopls","displayName":"Go Language Server","kind":"languageServer",
					"command":"gopls","languages":["go"],"status":"installed",
					"path":"/tools/gopls","version":"1.2.3","canInstall":true
				}`), nil
			case "lsp_restart_named":
				return json.RawMessage(`{}`), nil
			default:
				t.Fatalf("unexpected RPC %q", command)
				return nil, nil
			}
		},
	}
	m.InstallLanguageTool("gopls", false)
	waitForLanguageTest(t, done)
	if len(m.languageTools) != 1 || m.languageTools[0].Status != "installed" ||
		m.languageTools[0].Path != "/tools/gopls" {
		t.Fatalf("installed tool list = %+v", m.languageTools)
	}
	if m.toolsBusy != "" || m.toolsErr != "" ||
		m.status != "Go Language Server installed" {
		t.Fatalf("install did not finish: busy=%q err=%q status=%q",
			m.toolsBusy, m.toolsErr, m.status)
	}
	want := []string{"tools_list", "tools_install", "lsp_restart_named", "tools_list"}
	if len(commands) != len(want) {
		t.Fatalf("calls = %v", commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Fatalf("calls = %v", commands)
		}
	}
}

func TestInstallLanguageToolReportsNonInstallableTool(t *testing.T) {
	done := make(chan struct{})
	var m *Model
	m = &Model{
		ctx: context.Background(), root: "/project",
		settingsOpen: true, settingsPage: settingsPageLanguageServers,
		languageToolSettings: languageToolSettingsState{
			session: true, loaded: true, scope: "project",
			applied: map[string]languageServerConfig{},
			draft:   map[string]languageServerConfig{},
		},
		postHook: func(fn func()) {
			fn()
			if m.toolsBusy == "" {
				select {
				case <-done:
				default:
					close(done)
				}
			}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "tools_list" {
				t.Fatalf("unexpected RPC %q", command)
			}
			return json.RawMessage(`[{
				"name":"manual-ls","displayName":"Manual LS","kind":"languageServer",
				"command":"manual-ls","status":"missing","canInstall":false,
				"unavailableReason":"Install it with your package manager."
			}]`), nil
		},
	}
	m.settingsOpen = true
	m.InstallLanguageTool("manual-ls", false)
	waitForLanguageTest(t, done)
	if !strings.Contains(m.toolsErr, "package manager") ||
		m.languageToolPick != "manual-ls" || m.toolsBusy != "" {
		t.Fatalf("manual install result = busy=%q err=%q pick=%q",
			m.toolsBusy, m.toolsErr, m.languageToolPick)
	}
}

func TestCancelLanguageToolInstallSendsBackendCancellation(t *testing.T) {
	done := make(chan struct{})
	var got map[string]any
	m := &Model{
		ctx:       context.Background(),
		toolsBusy: "Installing gopls", toolsRequest: 77,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "cancel_request" {
				t.Fatalf("unexpected RPC %q", command)
			}
			got, _ = input.(map[string]any)
			close(done)
			return json.RawMessage(`{}`), nil
		},
	}
	m.CancelToolInstall()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for cancellation")
	}
	if got["id"] != uint32(77) || !m.toolsAbort.Load() ||
		m.toolsBusy != "Cancelling the installation…" {
		t.Fatalf("cancel state/value = %#v %+v", got, m)
	}
}

func TestLanguageToolBrowsedPathUpdatesNewBuiltinAndCustomServers(t *testing.T) {
	m := &Model{
		languageTools: languageToolRows(languageServerFixture()),
		languageToolSettings: languageToolSettingsState{
			loaded: true,
			draft: map[string]languageServerConfig{
				"custom": {Command: stringRef("custom-ls"), Languages: []string{"go"}},
			},
			pathDraft: map[string]string{},
		},
	}
	m.applyLanguageToolBrowsedPath("", "/Applications/Language Servers/custom-ls", true)
	if m.languageToolSettings.addCommand != `"/Applications/Language Servers/custom-ls"` {
		t.Fatalf("new-server command = %q", m.languageToolSettings.addCommand)
	}
	m.applyLanguageToolBrowsedPath("jdtls", "/opt/java/jdtls", false)
	if m.languageToolSettings.pathDraft["jdtls"] != "/opt/java/jdtls" {
		t.Fatalf("built-in path = %+v", m.languageToolSettings.pathDraft)
	}
	m.applyLanguageToolBrowsedPath("custom", "/opt/custom-ls", false)
	if got := m.languageToolSettings.draft["custom"].Command; got == nil || *got != "/opt/custom-ls" {
		t.Fatalf("custom command = %#v", got)
	}
}

func TestLanguageServerAddValidationAndAutomaticName(t *testing.T) {
	m := &Model{
		languageToolSettings: languageToolSettingsState{
			loaded: true,
			draft: map[string]languageServerConfig{
				"existing": {Command: stringRef("existing-ls"), Languages: []string{"go"}},
			},
		},
	}
	tests := []struct {
		name      string
		command   string
		explicit  string
		languages string
		want      string
	}{
		{"empty command", "", "", "go", "Enter the command"},
		{"unclosed quote", `"gopls --stdio`, "", "go", "Unclosed"},
		{"invalid name", "gopls", "../gopls", "go", "letters, digits"},
		{"existing name", "gopls", "existing", "go", "already exists"},
		{"missing language", "gopls", "", "", "Choose the languages"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m.languageToolSettings.addCommand = test.command
			m.languageToolSettings.addName = test.explicit
			m.languageToolSettings.addLanguages = test.languages
			if got := m.languageServerAddProblem(); !strings.Contains(got, test.want) {
				t.Fatalf("problem = %q, want %q", got, test.want)
			}
		})
	}

	m.languageToolSettings.addOpen = true
	m.languageToolSettings.addCommand = "/opt/lsp/clangd --stdio"
	m.languageToolSettings.addName = ""
	m.languageToolSettings.addLanguages = "c, cpp"
	m.addLanguageServer()
	entry, ok := m.languageToolSettings.draft["clangd"]
	if !ok || entry.Command == nil || *entry.Command != "/opt/lsp/clangd" ||
		formatLanguageToolArgs(entry.Args) != "--stdio" ||
		formatLanguageToolList(entry.Languages) != "c, cpp" ||
		m.languageToolSettings.addOpen {
		t.Fatalf("added server = %+v, open=%v", entry, m.languageToolSettings.addOpen)
	}
}
