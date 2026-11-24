package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestMCPServerSettingsLoadStageAndSaveGlobally(t *testing.T) {
	var mu sync.Mutex
	var writes []settingsBackendWrite
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "settings_get_global":
				return json.RawMessage(`{
					"mcpServer":{
						"enabled":true,
						"runWithoutConfirmation":true,
						"agentTabLimit":4,
						"agentArchiveLimit":12
					}
				}`), nil
			case "settings_set":
				args := input.(map[string]any)
				mu.Lock()
				writes = append(writes, settingsBackendWrite{
					scope: args["scope"].(string),
					key:   args["key"].(string),
					value: args["value"],
				})
				mu.Unlock()
				return json.RawMessage(`null`), nil
			default:
				t.Fatalf("command = %q", command)
				return nil, nil
			}
		},
	}
	m.setRoot(t.TempDir())
	m.loadNativeSettingsPage(settingsPageAgentMCPServer)
	waitSettingsBackendTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPageAgentMCPServer]
		return state.loaded && !state.loading
	})
	if !nativeSettingsBool(m, settingsPageAgentMCPServer, "mcpServer.enabled") ||
		!nativeSettingsBool(m, settingsPageAgentMCPServer, "mcpServer.runWithoutConfirmation") {
		t.Fatal("MCP server booleans were not loaded")
	}
	m.setNativeSettingsValue(settingsPageAgentMCPServer, "mcpServer.enabled", false)
	m.setNativeSettingsValue(settingsPageAgentMCPServer, "mcpServer.runWithoutConfirmation", false)
	staged := m.pendingNativeSettingsWrites()
	if len(staged) != 2 {
		t.Fatalf("MCP settings writes = %+v", staged)
	}
	for _, write := range staged {
		if write.scope != "global" {
			t.Fatalf("MCP setting %s scope = %q, want global", write.key, write.scope)
		}
	}
	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.saveBackendCatalogThenJava(false)
	waitSettingsBackendTest(t, func() bool {
		return !m.nativeSettings.saving && !m.settingsDirtyPage(settingsPageAgentMCPServer)
	})
	if m.nativeRuntimeBool(settingsPageAgentMCPServer, "mcpServer.enabled") ||
		m.nativeRuntimeBool(settingsPageAgentMCPServer, "mcpServer.runWithoutConfirmation") {
		t.Fatal("applied MCP server settings did not reach the runtime")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 2 {
		t.Fatalf("saved MCP settings = %+v", writes)
	}
	for _, write := range writes {
		if write.scope != "global" ||
			(write.key != "mcpServer.enabled" && write.key != "mcpServer.runWithoutConfirmation") ||
			write.value != "false" {
			t.Fatalf("saved MCP setting = %+v", write)
		}
	}
}

func TestMCPServerStatusLoadsAndClientConfigurations(t *testing.T) {
	port := uint16(1234)
	want := openidea.McpServerStatus{
		Running:      true,
		Port:         &port,
		Project:      "/p",
		URL:          "http://127.0.0.1:1234/mcp?project=/p",
		Token:        "secret",
		StdioCommand: []string{"/Applications/GoIDEA.app/Contents/MacOS/goidea", "--mcp-stdio", "--project", "/p"},
	}
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "mcp_server_status" {
				t.Fatalf("command = %q, want mcp_server_status", command)
			}
			return json.Marshal(want)
		},
	}
	m.setRoot("/p")
	m.loadMCPServerStatus(true)
	waitSettingsBackendTest(t, func() bool {
		return m.mcpServer.loaded && !m.mcpServer.loading
	})
	if m.mcpServer.err != "" || !m.mcpServer.status.Running ||
		m.mcpServer.status.Port == nil || *m.mcpServer.status.Port != port {
		t.Fatalf("MCP server status = %+v, error = %q", m.mcpServer.status, m.mcpServer.err)
	}

	httpConfig := mcpHTTPClientConfiguration(want)
	var httpJSON struct {
		Servers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(httpConfig), &httpJSON); err != nil {
		t.Fatal(err)
	}
	httpEntry := httpJSON.Servers["openidea"]
	if httpEntry.Type != "http" || httpEntry.URL != want.URL ||
		httpEntry.Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("HTTP config = %+v", httpEntry)
	}

	stdioConfig := mcpStdioClientConfiguration(want)
	if strings.Contains(stdioConfig, "secret") {
		t.Fatalf("stdio config contains the HTTP token: %s", stdioConfig)
	}
	var stdioJSON struct {
		Servers map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(stdioConfig), &stdioJSON); err != nil {
		t.Fatal(err)
	}
	stdioEntry := stdioJSON.Servers["openidea"]
	if stdioEntry.Type != "stdio" || stdioEntry.Command != want.StdioCommand[0] ||
		len(stdioEntry.Args) != len(want.StdioCommand)-1 {
		t.Fatalf("stdio config = %+v", stdioEntry)
	}
	if mcpHTTPClientConfiguration(openidea.McpServerStatus{}) != "" ||
		mcpStdioClientConfiguration(openidea.McpServerStatus{}) != "" {
		t.Fatal("incomplete MCP status produced a client configuration")
	}
}

func TestMCPServerPageRendersEndpointAndClientActions(t *testing.T) {
	port := uint16(1234)
	m := &Model{}
	m.setRoot("/p")
	values := nativeTestDefaults(mustNativeSettingsDefinition(t, settingsPageAgentMCPServer))
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageAgentMCPServer: {
			loaded:  true,
			session: true,
			root:    "/p",
			scope:   "global",
			applied: nativeSettingsValuesCopy(values),
			draft:   nativeSettingsValuesCopy(values),
		},
	}
	m.mcpServer = mcpServerSettingsState{
		session: true,
		loaded:  true,
		root:    "/p",
		status: openidea.McpServerStatus{
			Running:      true,
			Port:         &port,
			Project:      "/p",
			URL:          "http://127.0.0.1:1234/mcp?project=/p",
			Token:        "secret",
			StdioCommand: []string{"/Applications/GoIDEA.app/Contents/MacOS/goidea", "--mcp-stdio", "--project", "/p"},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsNativeBody(c, oiPaletteOf(c.Theme()), settingsPageAgentMCPServer)
	}, 900, 700)
	tester.Frame()
	for _, text := range []string{
		"Enable MCP server",
		"Run tools without confirmation (brave mode)",
		"Endpoint",
		"Running on port 1234",
		"Client configuration",
		"Copy HTTP Config",
		"Copy Stdio Config",
	} {
		if !tester.HasText(text) {
			t.Fatalf("IDE MCP Server page lacks %q: %q", text, tester.Texts())
		}
	}
}

func mustNativeSettingsDefinition(t *testing.T, page string) nativeSettingsPageSpec {
	t.Helper()
	def, ok := nativeSettingsDefinition(page)
	if !ok {
		t.Fatalf("missing native settings definition %q", page)
	}
	return def
}
