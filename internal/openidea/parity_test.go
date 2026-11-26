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
	tests := []struct {
		name  string
		raw   string
		check func(any) bool
	}{
		{"dbSourcesChanged", `{"type":"dbSourcesChanged"}`, func(v any) bool {
			_, ok := v.(*DBSourcesChangedEvent)
			return ok
		}},
		{"projectTrustChanged", `{"type":"projectTrustChanged","root":"/p","trusted":true}`, func(v any) bool {
			e, ok := v.(*ProjectTrustChangedEvent)
			return ok && e.Root == "/p" && e.Trusted
		}},
		{"fsChanged", `{"type":"fsChanged","created":["a"],"modified":["b"],"removed":["c"],"ignoreRules":["d"]}`, func(v any) bool {
			e, ok := v.(*FsChangedEvent)
			return ok && reflect.DeepEqual(e.Created, []string{"a"}) &&
				reflect.DeepEqual(e.Modified, []string{"b"}) &&
				reflect.DeepEqual(e.Removed, []string{"c"}) &&
				reflect.DeepEqual(e.IgnoreRules, []string{"d"})
		}},
		{"excludedDirsChanged", `{"type":"excludedDirsChanged","dirs":["/p/target"]}`, func(v any) bool {
			e, ok := v.(*ExcludedDirsChangedEvent)
			return ok && reflect.DeepEqual(e.Dirs, []string{"/p/target"})
		}},
		{"task", `{"type":"task","id":4294967295,"title":"Build","message":"Compiling","fraction":0.5,"done":false,"cancellable":true}`, func(v any) bool {
			e, ok := v.(*TaskEvent)
			return ok && e.ID == 4294967295 && e.Title == "Build" &&
				e.Message != nil && *e.Message == "Compiling" &&
				e.Fraction != nil && *e.Fraction == 0.5 && !e.Done && e.Cancellable
		}},
		{"indexState", `{"type":"indexState","indexing":true,"files":12,"symbols":34}`, func(v any) bool {
			e, ok := v.(*IndexStateEvent)
			return ok && e.Indexing && e.Files == 12 && e.Symbols == 34
		}},
		{"vcsChanged", `{"type":"vcsChanged","root":"/p"}`, func(v any) bool {
			e, ok := v.(*VCSChangedEvent)
			return ok && e.Root == "/p"
		}},
		{"lspRefresh", `{"type":"lspRefresh","server":7,"kind":"semanticTokens"}`, func(v any) bool {
			e, ok := v.(*LSPRefreshEvent)
			return ok && e.Server == 7 && e.Kind == "semanticTokens"
		}},
		{"notify", `{"type":"notify","level":"warning","title":"Index","body":"Paused"}`, func(v any) bool {
			e, ok := v.(*NotifyEvent)
			return ok && e.Level == "warning" && e.Title == "Index" &&
				e.Body != nil && *e.Body == "Paused"
		}},
		{"settingsChanged", `{"type":"settingsChanged","key":"editor.fontSize"}`, func(v any) bool {
			e, ok := v.(*SettingsChangedEvent)
			return ok && e.Key == "editor.fontSize"
		}},
		{"projectRootsChanged", `{"type":"projectRootsChanged","root":"/p"}`, func(v any) bool {
			e, ok := v.(*ProjectRootsChangedEvent)
			return ok && e.Root == "/p"
		}},
		{"askpass", `{"type":"askpass","id":4294967297,"prompt":"key","kind":"password","remember":true}`, func(v any) bool {
			e, ok := v.(*AskpassEvent)
			return ok && e.ID == 4294967297 && e.Prompt == "key" &&
				e.Kind == "password" && e.Remember
		}},
		{"askpassDone", `{"type":"askpassDone","id":4294967297}`, func(v any) bool {
			e, ok := v.(*AskpassDoneEvent)
			return ok && e.ID == 4294967297
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, value, err := DecodeIDEEvent(json.RawMessage(tt.raw))
			if err != nil || kind != tt.name || !tt.check(value) {
				t.Fatalf("event: %s %#v %v", kind, value, err)
			}
		})
	}
	kind, value, err := DecodeIDEEvent(json.RawMessage(`{"type":"future","extra":42}`))
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
