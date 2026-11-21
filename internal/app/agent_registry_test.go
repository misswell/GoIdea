package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAgentRegistryRowsCombineProvidersAndRegistry(t *testing.T) {
	providers := []agentProvider{
		{ID: "codex", Name: "Codex", Installed: true, Source: agentSource{Kind: "builtin"}},
		{ID: "gemini", Name: "Gemini", Source: agentSource{Kind: "builtin"}},
		{ID: "mine", Name: "My Agent", CommandOverridden: true, Source: agentSource{Kind: "custom"}},
	}
	version := "1.0"
	registry := []registryAgent{
		{ID: "gemini", Name: "Gemini", Version: version, Distribution: stringPointer("npx")},
		{ID: "cursor", Name: "Cursor", Version: "2.0", Installed: stringPointer("2.0")},
	}

	installed, available := agentRegistryRows(providers, registry, "")
	var installedIDs, availableIDs []string
	for _, row := range installed {
		installedIDs = append(installedIDs, agentRegistryRowID(row))
	}
	for _, row := range available {
		availableIDs = append(availableIDs, agentRegistryRowID(row))
	}
	if !reflect.DeepEqual(installedIDs, []string{"codex", "mine", "cursor"}) {
		t.Fatalf("installed = %v", installedIDs)
	}
	if !reflect.DeepEqual(availableIDs, []string{"gemini"}) {
		t.Fatalf("available = %v", availableIDs)
	}
	if got := agentRegistryRowStatus(installed[1]); got != "Agent file" {
		t.Fatalf("custom status = %q", got)
	}

	installed, available = agentRegistryRows(providers, registry, "cursor")
	if len(installed) != 1 || agentRegistryRowID(installed[0]) != "cursor" ||
		len(available) != 0 {
		t.Fatalf("filtered rows = %+v, %+v", installed, available)
	}
}

func TestApplyAgentProviderSettingsCallsCommandAndEnvironment(t *testing.T) {
	tests := []struct {
		name           string
		provider       agentProvider
		command        string
		wantCommand    any
		wantCommandRPC bool
	}{
		{
			name: "sets executable and env",
			provider: agentProvider{
				ID: "codex", Name: "Codex", CommandEditable: true,
				DefaultProgram: stringPointer("codex-acp"),
				Env:            map[string]string{"OLD": "1"},
			},
			command:        "/opt/bin/codex-acp",
			wantCommand:    "/opt/bin/codex-acp",
			wantCommandRPC: true,
		},
		{
			name: "clears override",
			provider: agentProvider{
				ID: "codex", Name: "Codex", CommandEditable: true,
				CommandOverridden: true, Program: "/old/codex-acp",
			},
			command:        "",
			wantCommand:    nil,
			wantCommandRPC: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := make(chan recordedAgentCall, 8)
			m := &Model{
				ctx:      context.Background(),
				postHook: func(fn func()) { fn() },
				invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
					args, _ := input.(map[string]any)
					calls <- recordedAgentCall{command: command, args: args}
					switch command {
					case "agent_providers":
						return json.RawMessage(`[]`), nil
					case "agent_registry_list":
						return json.RawMessage(`{"agents":[],"warning":null}`), nil
					default:
						return json.RawMessage(`null`), nil
					}
				},
				agents: AgentsPane{
					generation: 1,
					providers: []agentProvider{
						test.provider,
					},
					registry: agentRegistryState{
						CommandDrafts: map[string]string{"codex": test.command},
						EnvDrafts:     map[string]string{"codex": "TOKEN=abc; MODE=test"},
					},
				},
			}

			m.applyAgentProviderSettings("codex")
			commandCall := waitRecordedAgentCall(t, calls, "agent_set_command")
			if commandCall.args["id"] != "codex" ||
				!reflect.DeepEqual(commandCall.args["command"], test.wantCommand) {
				t.Fatalf("command arguments = %#v", commandCall.args)
			}
			envCall := waitRecordedAgentCall(t, calls, "agent_set_env")
			wantEnv := map[string]string{"MODE": "test", "TOKEN": "abc"}
			if !reflect.DeepEqual(envCall.args["env"], wantEnv) {
				t.Fatalf("env arguments = %#v", envCall.args)
			}
			waitAgentState(t, "provider settings", func() bool {
				return m.agents.registry.ProviderBusyID == ""
			})
		})
	}
}

func TestApplyAgentProviderSettingsOnlySendsChangedValues(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		env           string
		wantCommand   bool
		wantEnv       bool
		wantRPC       string
		unexpectedRPC string
	}{
		{
			name:          "command only",
			command:       "/opt/bin/codex-acp",
			env:           "TOKEN=abc",
			wantCommand:   true,
			wantRPC:       "agent_set_command",
			unexpectedRPC: "agent_set_env",
		},
		{
			name:          "environment only",
			command:       "",
			env:           "TOKEN=changed",
			wantEnv:       true,
			wantRPC:       "agent_set_env",
			unexpectedRPC: "agent_set_command",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := make(chan recordedAgentCall, 12)
			m := &Model{
				ctx:      context.Background(),
				postHook: func(fn func()) { fn() },
				invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
					args, _ := input.(map[string]any)
					calls <- recordedAgentCall{command: command, args: args}
					switch command {
					case "agent_providers":
						return json.RawMessage(`[]`), nil
					case "agent_registry_list":
						return json.RawMessage(`{"agents":[],"warning":null}`), nil
					default:
						return json.RawMessage(`null`), nil
					}
				},
				agents: AgentsPane{
					generation: 1,
					providers: []agentProvider{{
						ID: "codex", Name: "Codex", CommandEditable: true,
						DefaultProgram: stringPointer("codex-acp"),
						Env:            map[string]string{"TOKEN": "abc"},
					}},
					registry: agentRegistryState{
						CommandDrafts: map[string]string{"codex": test.command},
						EnvDrafts:     map[string]string{"codex": test.env},
					},
				},
			}

			m.applyAgentProviderSettings("codex")
			call := waitRecordedAgentCall(t, calls, test.wantRPC)
			if test.wantCommand && call.args["command"] != test.command {
				t.Fatalf("command arguments = %#v", call.args)
			}
			if test.wantEnv && !reflect.DeepEqual(call.args["env"], map[string]string{"TOKEN": "changed"}) {
				t.Fatalf("env arguments = %#v", call.args)
			}
			waitAgentState(t, "provider settings", func() bool {
				return m.agents.registry.ProviderBusyID == ""
			})
			for {
				select {
				case extra := <-calls:
					if extra.command == test.unexpectedRPC {
						t.Fatalf("unchanged setting called %s: %#v", extra.command, extra.args)
					}
				default:
					return
				}
			}
		})
	}
}

func TestApplyAgentProviderSettingsRejectsInvalidEnvironmentBeforeRPC(t *testing.T) {
	called := make(chan recordedAgentCall, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			called <- recordedAgentCall{command: command, args: input.(map[string]any)}
			return json.RawMessage(`null`), nil
		},
		agents: AgentsPane{
			generation: 1,
			providers: []agentProvider{{
				ID: "codex", Name: "Codex", CommandEditable: true,
			}},
			registry: agentRegistryState{
				CommandDrafts: map[string]string{"codex": "/opt/bin/codex-acp"},
				EnvDrafts:     map[string]string{"codex": "not-an-assignment"},
			},
		},
	}

	m.applyAgentProviderSettings("codex")
	if got := m.agents.registry.ProviderErr["codex"]; !strings.Contains(got, "Expected NAME=value") {
		t.Fatalf("provider error = %q", got)
	}
	select {
	case call := <-called:
		t.Fatalf("invalid environment still called %s", call.command)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestCustomAgentSaveAndACPImport(t *testing.T) {
	calls := make(chan recordedAgentCall, 10)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- recordedAgentCall{command: command, args: args}
			switch command {
			case "agent_custom_import":
				return json.RawMessage(`[
					{"name":"My Agent","id":"my-agent","saved":true},
					{"name":"Taken","id":"taken","saved":false,"reason":"taken"}
				]`), nil
			case "agent_providers":
				return json.RawMessage(`[]`), nil
			case "agent_registry_list":
				return json.RawMessage(`{"agents":[],"warning":null}`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
		agents: AgentsPane{generation: 1},
	}
	m.agents.registry.CustomOpen = true
	m.agents.registry.CustomName = "My Agent"
	m.agents.registry.CustomID = "my-agent"
	m.agents.registry.CustomCommand = "/opt/my-agent"
	m.agents.registry.CustomArgs = `--acp --label "My Agent"`
	m.agents.registry.CustomEnv = "TOKEN=abc; MODE=test"

	m.submitCustomAgent()
	save := waitRecordedAgentCall(t, calls, "agent_custom_save")
	input := save.args["input"].(map[string]any)
	if input["id"] != "my-agent" || input["name"] != "My Agent" ||
		input["command"] != "/opt/my-agent" {
		t.Fatalf("custom save = %#v", input)
	}
	if !reflect.DeepEqual(input["args"], []string{"--acp", "--label", "My Agent"}) {
		t.Fatalf("custom args = %#v", input["args"])
	}
	if !reflect.DeepEqual(input["env"], map[string]string{"MODE": "test", "TOKEN": "abc"}) {
		t.Fatalf("custom env = %#v", input["env"])
	}
	waitAgentState(t, "custom save", func() bool {
		return !m.agents.registry.CustomBusy && !m.agents.registry.CustomOpen
	})

	m.importAgentACPText(`{"agent_servers":{}}`)
	var importCall recordedAgentCall
	for {
		select {
		case importCall = <-calls:
			if importCall.command == "agent_providers" ||
				importCall.command == "agent_registry_list" {
				continue
			}
			if importCall.command != "agent_custom_import" {
				t.Fatalf("command = %q, want agent_custom_import", importCall.command)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("agent_custom_import was not called")
		}
		break
	}
	if importCall.args["text"] != `{"agent_servers":{}}` {
		t.Fatalf("import arguments = %#v", importCall.args)
	}
	waitAgentState(t, "ACP import notice", func() bool {
		return strings.Contains(m.agents.registry.Notice, "Imported My Agent from acp.json.")
	})
	if m.agents.registry.Notice !=
		"Imported My Agent from acp.json. Skipped Taken (taken)." {
		t.Fatalf("notice = %q", m.agents.registry.Notice)
	}
}

func TestCustomAgentSlugAndValidation(t *testing.T) {
	if got := agentSlugID("  My First Agent  "); got != "my-first-agent" {
		t.Fatalf("slug = %q", got)
	}
	if got := agentSlugID("Already__Fine-42"); got != "already-fine-42" {
		t.Fatalf("slug = %q", got)
	}
	if got := agentSlugID("中文 Agent"); got != "agent" {
		t.Fatalf("slug = %q", got)
	}

	called := make(chan recordedAgentCall, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			called <- recordedAgentCall{command: command, args: input.(map[string]any)}
			return json.RawMessage(`null`), nil
		},
	}
	for _, test := range []struct {
		label, name, id, command string
	}{
		{label: "Missing command", name: "Name", id: "valid-id"},
		{label: "Missing name", id: "valid-id", command: "/opt/agent"},
		{label: "Invalid id", name: "Name", id: "bad id", command: "/opt/agent"},
		{label: "Empty id", name: "Name", command: "/opt/agent"},
	} {
		m.agents.registry.CustomName = test.name
		m.agents.registry.CustomID = test.id
		m.agents.registry.CustomCommand = test.command
		m.submitCustomAgent()
	}
	select {
	case call := <-called:
		t.Fatalf("invalid custom agent still called %s", call.command)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestAgentRegistryNoChecksumNeedsConfirmation(t *testing.T) {
	calls := make(chan recordedAgentCall, 8)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- recordedAgentCall{command: command, args: args}
			switch command {
			case "agent_providers":
				return json.RawMessage(`[]`), nil
			case "agent_registry_list":
				return json.RawMessage(`{"agents":[],"warning":null}`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
		agents: AgentsPane{generation: 1},
	}
	agent := registryAgent{
		ID: "unsafe", Name: "Unsafe", Version: "1.0",
		Distribution: stringPointer("binary"), NoChecksum: true,
	}
	m.agents.registry.Agents = []registryAgent{agent}
	m.requestRegistryInstall(agent)
	if m.agents.registry.InstallConfirmID != "unsafe" {
		t.Fatalf("confirmation state = %+v", m.agents.registry)
	}
	select {
	case call := <-calls:
		t.Fatalf("installed before confirmation: %#v", call)
	case <-time.After(30 * time.Millisecond):
	}
	m.confirmRegistryInstall()
	call := waitRecordedAgentCall(t, calls, "agent_registry_install")
	if call.args["id"] != "unsafe" {
		t.Fatalf("install arguments = %#v", call.args)
	}
}

func TestAgentRegistryNoChecksumCancelDoesNotInstall(t *testing.T) {
	calls := make(chan recordedAgentCall, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			calls <- recordedAgentCall{command: command, args: input.(map[string]any)}
			return json.RawMessage(`null`), nil
		},
		agents: AgentsPane{generation: 1},
	}
	agent := registryAgent{
		ID: "unsafe", Name: "Unsafe", Version: "1.0",
		Distribution: stringPointer("binary"), NoChecksum: true,
	}
	m.agents.registry.Agents = []registryAgent{agent}
	m.requestRegistryInstall(agent)
	m.agents.registry.InstallConfirmID = ""
	m.confirmRegistryInstall()
	select {
	case call := <-calls:
		t.Fatalf("cancelled install called %s", call.command)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestAgentRegistrationPollingStopsAfterInstall(t *testing.T) {
	var listCalls atomic.Int32
	m := &Model{
		ctx:          context.Background(),
		postHook:     func(fn func()) { fn() },
		settingsOpen: true,
		settingsPage: settingsPageAgentRegistry,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "agent_registry_list" {
				t.Fatalf("unexpected command %q", command)
			}
			if listCalls.Add(1) == 1 {
				return json.RawMessage(`{"agents":[
					{"id":"installing","name":"Installing","version":"1","installing":true}
				],"warning":null}`), nil
			}
			return json.RawMessage(`{"agents":[
				{"id":"installing","name":"Installing","version":"1","installed":"1"}
			],"warning":null}`), nil
		},
		agents: AgentsPane{generation: 1},
	}

	m.LoadAgentRegistry(true)
	waitAgentState(t, "first registry load", func() bool {
		return m.agents.registry.Loaded && !m.agents.registry.Loading
	})
	if m.agents.registry.PollTimer == nil {
		t.Fatal("an installing agent did not schedule a poll")
	}
	waitAgentState(t, "installation registry poll", func() bool {
		st := &m.agents.registry
		return listCalls.Load() >= 2 && st.PollTimer == nil && !st.Loading &&
			len(st.Agents) == 1 && !st.Agents[0].Installing
	})
	if len(m.agents.registry.Agents) != 1 || m.agents.registry.Agents[0].Installing {
		t.Fatalf("registry did not refresh the completed install: %+v", m.agents.registry.Agents)
	}
	settled := listCalls.Load()
	time.Sleep(1800 * time.Millisecond)
	if got := listCalls.Load(); got != settled {
		t.Fatalf("polling continued after installation: %d -> %d calls", settled, got)
	}
}

func TestAgentRegistryDialogsShowDestructiveConfirmation(t *testing.T) {
	m := &Model{agents: AgentsPane{
		providers: []agentProvider{{
			ID: "mine", Name: "My Agent", Source: agentSource{Kind: "custom"},
		}},
		registry: agentRegistryState{
			RemoveOpen:   true,
			RemoveID:     "mine",
			RemoveCustom: true,
		},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentRegistryRemoveConfirmDialog(c, c.Theme())
	}, 720, 480)
	for _, want := range []string{
		"Remove My Agent",
		"delete its agent file",
		"Remove",
		"Cancel",
	} {
		if !tester.HasText(want) {
			t.Fatalf("remove dialog lacks %q: %q", want, tester.Texts())
		}
	}

	m.agents.registry.RemoveOpen = false
	m.agents.registry.CustomOpen = true
	tester = ui.NewTester(func(c *ui.Context) {
		m.agentCustomDialog(c, c.Theme())
	}, 720, 640)
	for _, want := range []string{
		"Add Custom Agent", "Name", "ID", "Command", "Arguments",
		"Environment", "Browse…", "Add", "Cancel",
	} {
		if !tester.HasText(want) {
			t.Fatalf("custom agent dialog lacks %q: %q", want, tester.Texts())
		}
	}
}

func TestAgentRegistryPageShowsProvidersAndLaunchSettings(t *testing.T) {
	m := &Model{agents: AgentsPane{
		providers: []agentProvider{{
			ID: "codex", Name: "Codex", Installed: true,
			Program: "codex-acp", CommandEditable: true,
			DefaultProgram: stringPointer("codex-acp"),
			Source:         agentSource{Kind: "builtin"},
			Config: []agentConfigOption{{
				ID: "model", Name: "Model", Category: stringPointer("model"),
				Current: stringPointer("balanced"),
				Values: []agentConfigValue{
					{Value: "fast", Name: "Fast"},
					{Value: "balanced", Name: "Balanced"},
				},
			}},
		}},
		registry: agentRegistryState{
			Loaded:   true,
			Selected: "codex",
			Agents: []registryAgent{{
				ID: "gemini", Name: "Gemini", Version: "1.0",
				Distribution: stringPointer("npx"),
			}},
		},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsAgentRegistryBody(c, oiPaletteOf(c.Theme()))
	}, 1100, 760)
	tester.Frame()
	for _, want := range []string{
		"Coding Agents", "Installed  1", "Available  1", "Codex", "Gemini",
		"Add Custom Agent…", "Import acp.json", "New Tasks", "Model",
		"Launch", "Executable", "Environment", "Apply",
	} {
		if !tester.HasText(want) {
			t.Fatalf("Coding Agents page lacks %q: %q", want, tester.Texts())
		}
	}
}

func TestAgentDefaultsDriveNewTasksAndPersist(t *testing.T) {
	calls := make(chan recordedAgentCall, 2)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			calls <- recordedAgentCall{command: command, args: input.(map[string]any)}
			return json.RawMessage(`null`), nil
		},
		agents: AgentsPane{
			newProvider: "codex",
			providers: []agentProvider{{
				ID: "codex", Name: "Codex",
				Config: []agentConfigOption{{
					ID: "model", Name: "Model", Category: stringPointer("model"),
					Current: stringPointer("fast"),
					Values: []agentConfigValue{
						{Value: "fast", Name: "Fast"},
						{Value: "balanced", Name: "Balanced"},
					},
				}},
			}},
		},
	}
	prefs := (AgentPrefs{}).normalized()
	prefs.BackendDefaults = `{"codex":{"model":"balanced"}}`
	m.prefs.Agents = &prefs
	m.prepareNewAgentConfig()
	if got := m.agents.newConfig["model"]; got != "balanced" {
		t.Fatalf("new task model = %q", got)
	}

	m.setAgentDefaultChoice("codex", "model", "fast")
	call := waitRecordedAgentCall(t, calls, "settings_set")
	if call.args["key"] != "agents.defaults" ||
		!strings.Contains(call.args["value"].(string), `"model":"fast"`) {
		t.Fatalf("settings call = %#v", call.args)
	}
	if got := m.agentDefaultChoices("codex")["model"]; got != "fast" {
		t.Fatalf("stored default = %q", got)
	}
}
