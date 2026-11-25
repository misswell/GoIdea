package openidea

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestPluginActionParityRPCWireRequests(t *testing.T) {
	conn, peer := newConnPair(t, Options{})
	m := NewManager(LaunchConfig{})
	m.srv = &Server{conn: conn}
	ctx := context.Background()

	scope := "project"
	request := uint32(41)
	editor := map[string]any{
		"path":     "/project/App.java",
		"line":     float64(2),
		"column":   float64(4),
		"isFolder": false,
	}
	paths := []string{"/project/App.java", "/project/Util.java"}
	chain := []string{"plugin.echo.before", "plugin.echo.run"}

	done := make(chan error, 1)
	go func() {
		done <- m.PluginRunCommandWithContext(
			ctx,
			"window-a",
			"echo",
			"run",
			"payload",
			PluginCommandContext{
				Editor:        editor,
				Paths:         paths,
				SettingsScope: &scope,
				CommandChain:  chain,
				Request:       &request,
			},
		)
	}()
	req := peer.read(t)
	got := decodePluginRPCRequest(t, req)
	want := map[string]any{
		"pluginId":      "echo",
		"commandId":     "run",
		"arg":           "payload",
		"editor":        editor,
		"paths":         []any{"/project/App.java", "/project/Util.java"},
		"settingsScope": scope,
		"commandChain":  []any{"plugin.echo.before", "plugin.echo.run"},
		"request":       float64(request),
	}
	if got.Window != "window-a" || got.Cmd != "plugin_run_command" || !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("plugin_run_command = %#v, want %#v", got, want)
	}
	peer.send(t, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	action := map[string]any{"kind": "save", "id": "toolbar.save"}
	done = make(chan error, 1)
	go func() {
		_, err := m.PluginContentTabAction(ctx, "window-a", "echo", action)
		done <- err
	}()
	req = peer.read(t)
	got = decodePluginRPCRequest(t, req)
	want = map[string]any{
		"pluginId": "echo",
		"action":   action,
	}
	if got.Window != "window-a" || got.Cmd != "plugin_content_tab_action" || !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("plugin_content_tab_action = %#v, want %#v", got, want)
	}
	peer.send(t, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPluginRunCommandDefaultsPathsToEmpty(t *testing.T) {
	conn, peer := newConnPair(t, Options{})
	m := NewManager(LaunchConfig{})
	m.srv = &Server{conn: conn}

	done := make(chan error, 1)
	go func() {
		done <- m.PluginRunCommand(context.Background(), "window-a", "echo", "run", "payload")
	}()
	req := peer.read(t)
	got := decodePluginRPCRequest(t, req)
	want := map[string]any{
		"pluginId":      "echo",
		"commandId":     "run",
		"arg":           "payload",
		"editor":        nil,
		"paths":         []any{},
		"settingsScope": nil,
		"commandChain":  nil,
		"request":       nil,
	}
	if !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("default plugin_run_command args = %#v, want %#v", got.Args, want)
	}
	peer.send(t, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type pluginRPCRequest struct {
	Window string
	Cmd    string
	Args   map[string]any
}

func decodePluginRPCRequest(t *testing.T, req message) pluginRPCRequest {
	t.Helper()
	var got pluginRPCRequest
	if err := json.Unmarshal(req.Params, &got); err != nil {
		t.Fatal(err)
	}
	return got
}
