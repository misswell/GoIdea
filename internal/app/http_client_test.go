package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestHTTPRequestAtLineMatchesOpenIDEA(t *testing.T) {
	name := "user"
	blocks := []openidea.HTTPRequestBlock{
		{Index: 0, Method: "GET", URL: "https://one.test", Line: 1, StartLine: 0, EndLine: 4},
		{Index: 1, Name: &name, Method: "POST", URL: "https://two.test", Line: 8, StartLine: 7, EndLine: 12},
	}
	tests := []struct {
		line int
		want uint32
	}{
		{0, 0},
		{4, 0},
		{6, 0},
		{8, 1},
		{12, 1},
		{20, 1},
	}
	for _, tt := range tests {
		got, ok := httpRequestAtLine(blocks, tt.line)
		if !ok || got.Index != tt.want {
			t.Fatalf("line %d = %#v, want request %d", tt.line, got, tt.want)
		}
	}
	if _, ok := httpRequestAtLine(nil, 0); ok {
		t.Fatal("an empty file selected a request")
	}
}

func TestHTTPEnvironmentChoicePersistsPerFile(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: settingsPath}}
	m.prefs.HTTP.Environments = map[string]string{
		"/old/request.http": "Local",
	}
	request := "/project/request.http"
	m.setHTTPEnvironment(request, "Development")
	if got := m.http.envByFile[request]; got != "Development" {
		t.Fatalf("in-memory environment = %q", got)
	}
	stored := LoadPreferences(settingsPath)
	if got := stored.HTTP.Environments[request]; got != "Development" {
		t.Fatalf("stored environment = %q", got)
	}
	if got := stored.HTTP.Environments["/old/request.http"]; got != "Local" {
		t.Fatalf("existing choice was lost: %q", got)
	}
	m.setHTTPEnvironment(request, "")
	if _, ok := LoadPreferences(settingsPath).HTTP.Environments[request]; ok {
		t.Fatal("No Environment left the choice in the settings file")
	}
	reloaded := &Model{
		cfg:   Config{SettingsPath: settingsPath},
		prefs: stored,
	}
	reloaded.resetHTTPClient()
	if got := reloaded.http.envByFile["/old/request.http"]; got != "Local" {
		t.Fatalf("reloaded environment = %q", got)
	}
}

func TestHTTPRequestAtCaretUsesTheRequestUnderTheCaret(t *testing.T) {
	text := "GET https://one.test/\n\n###\nPOST https://two.test/\n"
	doc := docbridge.Virtual("/project/requests.http", text)
	state := &editor.State{Buffer: doc.Buffer()}
	offset := strings.Index(text, "POST")
	state.Select(offset, offset)
	tab := &Tab{doc: doc, state: state}
	m := &Model{
		tabs:   []*Tab{tab},
		active: 0,
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "http_requests" {
				t.Fatalf("command = %q", command)
			}
			return json.Marshal([]openidea.HTTPRequestBlock{
				{Index: 0, Method: "GET", URL: "https://one.test/", Line: 0, StartLine: 0, EndLine: 0},
				{Index: 1, Method: "POST", URL: "https://two.test/", Line: 3, StartLine: 2, EndLine: 3},
			})
		},
	}
	m.postHook = func(fn func()) { fn() }
	got := make(chan uint32, 1)
	m.withHTTPRequestAtCaret(tab, func(block openidea.HTTPRequestBlock, _ string) {
		got <- block.Index
	})
	select {
	case index := <-got:
		if index != 1 {
			t.Fatalf("request index = %d, want 1", index)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request selection did not finish")
	}
}

func TestRunHTTPRequestSendsSelectedEnvironmentAndStoresTheResponse(t *testing.T) {
	path := "/project/requests.http"
	text := "GET https://example.test/\n"
	var executeArgs map[string]any
	m := &Model{
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "http_requests":
				return json.Marshal([]openidea.HTTPRequestBlock{{
					Index: 0, Method: "GET", URL: "https://example.test/", Line: 0, EndLine: 0,
				}})
			case "http_execute":
				executeArgs, _ = input.(map[string]any)
				return json.Marshal(openidea.HTTPExchange{
					Entry: openidea.HTTPHistoryEntry{
						ID: "response-1", Method: "GET", URL: "https://example.test/",
					},
					Request: openidea.HTTPPreparedRequest{
						Method: "GET", URL: "https://example.test/",
					},
				})
			default:
				t.Fatalf("unexpected command %q", command)
				return nil, nil
			}
		},
	}
	m.postHook = func(fn func()) { fn() }
	m.resetHTTPClient()
	m.http.envByFile[path] = "Development"
	m.http.environments[path] = []string{"Development", "Production"}
	if !m.runHTTPRequest(path, text, 0) {
		t.Fatal("request failed")
	}
	if got := executeArgs["env"]; got != "Development" {
		t.Fatalf("environment argument = %#v", got)
	}
	if len(m.http.history) != 1 || m.http.history[0].ID != "response-1" {
		t.Fatalf("history = %#v", m.http.history)
	}
	if m.http.running != nil {
		t.Fatal("the running request was not cleared")
	}
}

func TestHTTPModelDecodesCamelCasePayload(t *testing.T) {
	raw := []byte(`{
		"index": 2,
		"name": "status",
		"method": "GET",
		"url": "https://example.test/status",
		"line": 8,
		"startLine": 7,
		"endLine": 10
	}`)
	var block openidea.HTTPRequestBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatal(err)
	}
	if block.Index != 2 || block.Name == nil || *block.Name != "status" ||
		block.StartLine != 7 || block.EndLine != 10 {
		t.Fatalf("block = %#v", block)
	}
}
