package openidea

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestParityRPCWireRequests(t *testing.T) {
	conn, peer := newConnPair(t, Options{})
	m := NewManager(LaunchConfig{})
	m.srv = &Server{conn: conn}
	ctx := context.Background()
	endpoint := DockerEndpoint{Kind: "local"}
	tests := []struct {
		name   string
		args   map[string]any
		result any
		call   func() error
	}{
		{"docker_events", map[string]any{"endpoint": endpoint, "request": uint32(8), "onEvent": streamChannel(9)}, nil, func() error { return m.DockerEvents(ctx, "w", endpoint, 8, 9) }},
		{"docker_logs", map[string]any{"endpoint": endpoint, "id": "id", "tail": uint32(10), "follow": true, "timestamps": false, "request": uint32(8), "onLine": streamChannel(9)}, nil, func() error { return m.DockerLogs(ctx, "w", endpoint, "id", 10, true, false, 8, 9) }},
		{"docker_pull", map[string]any{"endpoint": endpoint, "reference": "a:b", "request": uint32(8), "onOutput": streamChannel(9)}, nil, func() error { return m.DockerPull(ctx, "w", endpoint, "a:b", 8, 9) }},
		{"docker_prune", map[string]any{"endpoint": endpoint, "target": "images", "request": uint32(8), "onOutput": streamChannel(9)}, nil, func() error { return m.DockerPrune(ctx, "w", endpoint, "images", 8, 9) }},
		{"docker_compose_op", map[string]any{"endpoint": endpoint, "project": "p", "files": []string{}, "workingDir": nil, "op": "stop", "request": uint32(8), "onOutput": streamChannel(9)}, nil, func() error { return m.DockerComposeOp(ctx, "w", endpoint, "p", nil, nil, "stop", 8, 9) }},
		{"plugin_service_log", map[string]any{"pluginId": "p", "view": "v", "key": "k", "tab": "t", "on": []string{}, "request": uint32(8), "onLine": streamChannel(9)}, float64(5), func() error {
			c, e := m.PluginServiceLog(ctx, "w", "p", "v", "k", "t", 8, 9, []string{})
			if e == nil && (c == nil || *c != 5) {
				t.Errorf("exit code %v", c)
			}
			return e
		}},
		{"plugin_terminal_session_spawn", map[string]any{"operation": "op", "cols": uint32(80), "rows": uint32(24), "onOutput": streamChannel(9)}, map[string]any{"id": 3}, func() error { _, e := m.PluginTerminalSessionSpawn(ctx, "w", "op", 80, 24, 9); return e }},
		{"lsp_code_actions", map[string]any{"doc": uint32(2), "range": OutlineRange{}, "only": []string{}}, []any{}, func() error { _, e := m.LSPCodeActions(ctx, "w", 2, OutlineRange{}, nil); return e }},
		{"replace_preview", map[string]any{"query": map[string]any{"pattern": "x"}, "replacement": "y", "preserveCase": true, "path": "/p", "only": []SearchPosition{{Line: 1, Col: 2}}}, map[string]any{"before": "x", "after": "y", "edits": []any{}}, func() error {
			_, e := m.ReplacePreview(ctx, "w", map[string]any{"pattern": "x"}, "y", true, "/p", []SearchPosition{{Line: 1, Col: 2}})
			return e
		}},
		{"vcs_rebase", map[string]any{"repo": nil, "onto": "main"}, nil, func() error { return m.VCSRebase(ctx, "w", nil, "main") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- tt.call() }()
			req := peer.read(t)
			var got struct {
				Window string          `json:"window"`
				Cmd    string          `json:"cmd"`
				Args   json.RawMessage `json:"args"`
			}
			if err := json.Unmarshal(req.Params, &got); err != nil {
				t.Fatal(err)
			}
			want, _ := json.Marshal(tt.args)
			var a, b any
			_ = json.Unmarshal(got.Args, &a)
			_ = json.Unmarshal(want, &b)
			if got.Window != "w" || got.Cmd != tt.name || !reflect.DeepEqual(a, b) {
				t.Errorf("got %s %s %s, want %s", got.Window, got.Cmd, got.Args, want)
			}
			peer.send(t, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": tt.result})
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDecodeIDEEvent(t *testing.T) {
	kind, value, err := DecodeIDEEvent(json.RawMessage(`{"type":"fsChanged","created":["a"],"ignoreRules":["b"]}`))
	if err != nil || kind != "fsChanged" || !reflect.DeepEqual(value.(*FsChangedEvent).IgnoreRules, []string{"b"}) {
		t.Fatalf("event: %s %#v %v", kind, value, err)
	}
	kind, value, err = DecodeIDEEvent(json.RawMessage(`{"type":"askpass","id":4294967297,"prompt":"key","kind":"password","remember":true}`))
	if err != nil || kind != "askpass" || value.(*AskpassEvent).ID != 4294967297 {
		t.Fatalf("event: %s %#v %v", kind, value, err)
	}
	kind, value, err = DecodeIDEEvent(json.RawMessage(`{"type":"future","extra":42}`))
	if err != nil || kind != "future" || string(value.(json.RawMessage)) != `{"type":"future","extra":42}` {
		t.Fatalf("unknown: %s %#v %v", kind, value, err)
	}
	for _, raw := range []string{`{`, `{"type":"task","id":"bad"}`, `{}`} {
		if _, _, err := DecodeIDEEvent(json.RawMessage(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestParityStreamAndUnsupportedCommand(t *testing.T) {
	conn, peer := newConnPair(t, Options{})
	m := NewManager(LaunchConfig{})
	m.srv = &Server{conn: conn}
	done := make(chan error, 1)
	go func() { done <- m.DockerEvents(context.Background(), "w", DockerEndpoint{Kind: "local"}, 8, 9) }()
	req := peer.read(t)
	peer.send(t, map[string]any{"jsonrpc": "2.0", "method": "channel", "params": map[string]any{"id": 9, "data": map[string]any{"object": "container", "action": "start", "id": "abc", "name": nil}}})
	select {
	case n := <-conn.Notifications():
		var msg ChannelMessage
		if err := json.Unmarshal(n.Params, &msg); err != nil || n.Method != "channel" || msg.ID != 9 {
			t.Fatalf("notification %v, %v", n, err)
		}
		var event DockerEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil || event.Action != "start" {
			t.Fatalf("event %v, %v", event, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no stream notification")
	}
	peer.send(t, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "unknown command"}})
	var rpcErr *Error
	if err := <-done; !errors.As(err, &rpcErr) || rpcErr.Code != CodeMethodNotFound {
		t.Fatalf("expected unsupported error, got %v", err)
	}
}
