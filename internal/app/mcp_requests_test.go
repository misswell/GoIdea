package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type mcpResponseCall struct {
	ID    uint32  `json:"id"`
	Allow bool    `json:"allow"`
	Error *string `json:"error"`
}

func decodeMCPResponseCall(input any) (mcpResponseCall, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return mcpResponseCall{}, err
	}
	var call mcpResponseCall
	err = json.Unmarshal(raw, &call)
	return call, err
}

func waitMCPResponse(t *testing.T, responses <-chan mcpResponseCall) mcpResponseCall {
	t.Helper()
	select {
	case response := <-responses:
		return response
	case <-time.After(2 * time.Second):
		t.Fatal("MCP response was not sent")
		return mcpResponseCall{}
	}
}

func assertNoMCPResponse(t *testing.T, responses <-chan mcpResponseCall) {
	t.Helper()
	select {
	case response := <-responses:
		t.Fatalf("unexpected additional MCP response: %+v", response)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMCPConfirmQueueAnswersAllowDenyAndExpiry(t *testing.T) {
	main := &serialMainThread{}
	responses := make(chan mcpResponseCall, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "mcp_respond" {
				response, err := decodeMCPResponseCall(input)
				if err != nil {
					return nil, err
				}
				responses <- response
			}
			return json.RawMessage("null"), nil
		},
	}
	confirm := openidea.McpUIRequest{
		Type: "confirm", Client: "Claude Code", Tool: "execute_run_configuration",
		Subject: "Tests", Project: "/project", Detail: "npm test",
	}
	m.acceptMCPRequest(openidea.McpRequestEvent{ID: 1, Request: confirm})
	confirm.Subject = "Build"
	m.acceptMCPRequest(openidea.McpRequestEvent{ID: 2, Request: confirm})
	main.do(func() {
		if len(m.mcp.confirms) != 2 {
			t.Fatalf("initial confirmation queue = %+v", m.mcp.confirms)
		}
	})

	main.do(func() { m.answerMCPConfirm(true) })
	response := waitMCPResponse(t, responses)
	if response.ID != 1 || !response.Allow || response.Error != nil {
		t.Fatalf("allow response = %+v", response)
	}
	main.do(func() {
		if len(m.mcp.confirms) != 1 || m.mcp.confirms[0].id != 2 {
			t.Fatalf("queue after allow = %+v", m.mcp.confirms)
		}
	})

	main.do(func() { m.answerMCPConfirm(false) })
	response = waitMCPResponse(t, responses)
	if response.ID != 2 || response.Allow || response.Error != nil {
		t.Fatalf("deny response = %+v", response)
	}

	m.acceptMCPRequest(openidea.McpRequestEvent{ID: 3, Request: confirm})
	main.do(func() { m.dropMCPRequest(3) })
	main.do(func() {
		if len(m.mcp.confirms) != 0 {
			t.Fatalf("expired confirmation remains queued: %+v", m.mcp.confirms)
		}
	})
}

func TestMCPConfirmDialogEnterAndEscapeDeny(t *testing.T) {
	main := &serialMainThread{}
	responses := make(chan mcpResponseCall, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "mcp_respond" {
				response, err := decodeMCPResponseCall(input)
				if err != nil {
					return nil, err
				}
				responses <- response
			}
			return json.RawMessage("null"), nil
		},
	}
	confirm := openidea.McpUIRequest{
		Type: "confirm", Client: "Claude Code", Tool: "execute_run_configuration",
		Subject: "Tests", Project: "/project", Detail: "npm test",
	}
	m.acceptMCPRequest(openidea.McpRequestEvent{ID: 11, Request: confirm})
	confirm.Subject = "Build"
	m.acceptMCPRequest(openidea.McpRequestEvent{ID: 12, Request: confirm})

	tester := ui.NewTester(func(c *ui.Context) {
		m.mcpConfirmDialog(c, c.Theme())
	}, 700, 420)
	tester.Frame()
	for _, text := range []string{
		"Claude Code wants to run Tests",
		"Project: /project",
		"Tool: execute_run_configuration",
		"npm test",
		"1 more waiting",
	} {
		if !tester.HasText(text) {
			t.Fatalf("confirmation is missing %q: %v", text, tester.Texts())
		}
	}
	tester.Key(0, ui.KeyEnter)
	response := waitMCPResponse(t, responses)
	if response.ID != 11 || response.Allow {
		t.Fatalf("Enter response = %+v", response)
	}
	tester.Frame()
	if !tester.HasText("Claude Code wants to run Build") {
		t.Fatalf("next confirmation did not appear: %v", tester.Texts())
	}
	tester.Key(0, ui.KeyEscape)
	response = waitMCPResponse(t, responses)
	if response.ID != 12 || response.Allow {
		t.Fatalf("Escape response = %+v", response)
	}
}

func TestMCPFailedAnswerReturnsToFrontOfQueue(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command == "mcp_respond" {
				return nil, errors.New("backend is offline")
			}
			return json.RawMessage("null"), nil
		},
	}
	m.acceptMCPRequest(openidea.McpRequestEvent{
		ID: 21,
		Request: openidea.McpUIRequest{
			Type: "confirm", Client: "Claude Code", Tool: "write_file",
			Subject: "main.go", Project: "/project",
		},
	})
	main.do(func() { m.answerMCPConfirm(true) })
	waitForSerialMainThread(t, main, 2*time.Second, func() bool {
		return len(m.mcp.confirms) == 1 && m.mcp.confirms[0].errText != ""
	})
	main.do(func() {
		response := m.mcp.confirms[0]
		if response.id != 21 || response.errText != "backend is offline" {
			t.Fatalf("retried confirmation = %+v", response)
		}
		if m.mcp.sending[21] {
			t.Fatal("failed response remains marked as sending")
		}
	})
}

func TestMCPClosedRequestDoesNotReturnAfterFailedAnswer(t *testing.T) {
	main := &serialMainThread{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command == "mcp_respond" {
				started <- struct{}{}
				<-release
				return nil, errors.New("expired")
			}
			return json.RawMessage("null"), nil
		},
	}
	m.acceptMCPRequest(openidea.McpRequestEvent{
		ID: 31,
		Request: openidea.McpUIRequest{
			Type: "confirm", Client: "Claude Code", Tool: "write_file",
			Subject: "main.go", Project: "/project",
		},
	})
	main.do(func() { m.answerMCPConfirm(true) })
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("MCP answer did not reach the backend")
	}
	main.do(func() { m.dropMCPRequest(31) })
	close(release)
	waitForSerialMainThread(t, main, 2*time.Second, func() bool {
		return !m.mcp.sending[31]
	})
	main.do(func() {
		if len(m.mcp.confirms) != 0 {
			t.Fatalf("expired response was requeued: %+v", m.mcp.confirms)
		}
	})
}

func TestMCPOpenFileUsesZeroBasedCoordinatesAndReportsFailures(t *testing.T) {
	main := &serialMainThread{}
	responses := make(chan mcpResponseCall, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		busy:     true,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "mcp_respond" {
				response, err := decodeMCPResponseCall(input)
				if err != nil {
					return nil, err
				}
				responses <- response
			}
			return json.RawMessage("null"), nil
		},
	}
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	line, column := 2, 4
	if err := m.openMCPFile(41, openidea.McpUIRequest{
		Type: "openFile", Path: path, Line: &line, Column: &column,
	}); err != nil {
		t.Fatal(err)
	}
	if m.reveal == nil || m.reveal.line != 1 || m.reveal.col != 3 ||
		m.reveal.path != realPath(path) {
		t.Fatalf("MCP reveal = %+v", m.reveal)
	}
	if len(m.openQueue) != 1 || m.openQueue[0].path != realPath(path) {
		t.Fatalf("queued MCP open = %+v", m.openQueue)
	}

	m.busy = false
	m.acceptMCPRequest(openidea.McpRequestEvent{
		ID: 42,
		Request: openidea.McpUIRequest{
			Type: "openFile", Path: t.TempDir(),
		},
	})
	response := waitMCPResponse(t, responses)
	if response.ID != 42 || response.Allow || response.Error == nil {
		t.Fatalf("directory failure response = %+v", response)
	}
}

func TestMCPProjectReadyAndClosedUpdateTheBackend(t *testing.T) {
	main := &serialMainThread{}
	type readyCall struct {
		ready bool
	}
	calls := make(chan readyCall, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "mcp_ui_ready" {
				var args struct {
					Ready bool `json:"ready"`
				}
				raw, _ := json.Marshal(input)
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, err
				}
				calls <- readyCall{ready: args.Ready}
			}
			return json.RawMessage("null"), nil
		},
	}
	main.do(func() { m.mcpProjectReady() })
	select {
	case call := <-calls:
		if !call.ready {
			t.Fatal("project ready sent ready=false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("project ready did not reach the backend")
	}
	main.do(func() { m.mcpProjectClosed() })
	select {
	case call := <-calls:
		if call.ready {
			t.Fatal("project close sent ready=true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("project close did not reach the backend")
	}
}

func TestMCPAgentRunAnswersExactlyOnceAfterStart(t *testing.T) {
	for _, test := range []struct {
		name    string
		start   func() (json.RawMessage, error)
		allowed bool
	}{
		{
			name: "started",
			start: func() (json.RawMessage, error) {
				return json.Marshal(openidea.RunInfo{
					ID: 51, ConfigID: "mcp-agent:_agent_test", Name: "_agent_test", Running: true,
				})
			},
			allowed: true,
		},
		{
			name: "failed",
			start: func() (json.RawMessage, error) {
				return nil, errors.New("cannot start")
			},
			allowed: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			main := &serialMainThread{}
			responses := make(chan mcpResponseCall, 4)
			m := &Model{
				ctx:      context.Background(),
				postHook: main.post,
				invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
					switch command {
					case "run_start":
						return test.start()
					case "mcp_respond":
						response, err := decodeMCPResponseCall(input)
						if err != nil {
							return nil, err
						}
						responses <- response
					}
					return json.RawMessage("null"), nil
				},
			}
			m.setRoot(t.TempDir())
			m.run.trusted = true
			m.acceptMCPRequest(openidea.McpRequestEvent{
				ID: 52,
				Request: openidea.McpUIRequest{
					Type: "agentRun", Name: "_agent_test", Command: "npm test",
					Cwd: m.Root(), Env: map[string]string{},
				},
			})
			response := waitMCPResponse(t, responses)
			if response.ID != 52 || response.Allow != test.allowed {
				t.Fatalf("agent response = %+v", response)
			}
			if !test.allowed && response.Error == nil {
				t.Fatal("failed agent start omitted its error")
			}
			assertNoMCPResponse(t, responses)
		})
	}
}

func TestMCPAgentRunClosedNotifiesOnlyForUserClosedTabs(t *testing.T) {
	main := &serialMainThread{}
	closed := make(chan string, 8)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "mcp_agent_run_closed" {
				var args struct {
					Name string `json:"name"`
				}
				raw, _ := json.Marshal(input)
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, err
				}
				closed <- args.Name
			}
			return json.RawMessage("null"), nil
		},
	}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageAgentMCPServer: {
			loaded: true,
			applied: map[string]any{
				"mcpServer.agentTabLimit":     float64(1),
				"mcpServer.agentArchiveLimit": float64(100),
			},
		},
	}
	m.run.agentRuns = []*agentRunSession{
		{id: 1, request: agentRunRequest{Name: "_agent_one"}, ended: time.Unix(1, 0)},
	}
	m.closeAgentRun(1)
	select {
	case name := <-closed:
		if name != "_agent_one" {
			t.Fatalf("direct close notified %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("direct close did not notify the backend")
	}

	m.run.agentRuns = []*agentRunSession{
		{id: 2, request: agentRunRequest{Name: "_agent_visible"}, ended: time.Unix(3, 0)},
		{id: 3, request: agentRunRequest{Name: "_agent_archived"}, ended: time.Unix(2, 0)},
	}
	m.closeAgentArchive()
	select {
	case name := <-closed:
		if name != "_agent_archived" {
			t.Fatalf("archive close notified %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("archive close did not notify the backend")
	}

	m.nativeSettings.pages[settingsPageAgentMCPServer] = nativeSettingsPageState{
		loaded: true,
		applied: map[string]any{
			"mcpServer.agentTabLimit":     float64(1),
			"mcpServer.agentArchiveLimit": float64(1),
		},
	}
	m.run.agentRuns = []*agentRunSession{
		{id: 4, request: agentRunRequest{Name: "_agent_new"}, ended: time.Unix(5, 0)},
		{id: 5, request: agentRunRequest{Name: "_agent_drop"}, ended: time.Unix(4, 0)},
		{id: 6, request: agentRunRequest{Name: "_agent_drop_older"}, ended: time.Unix(3, 0)},
	}
	m.trimAgentRuns()
	select {
	case name := <-closed:
		t.Fatalf("archive trim notified the backend for %q", name)
	case <-time.After(50 * time.Millisecond):
	}
}
