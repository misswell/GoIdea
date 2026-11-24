package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

type pluginExitResult struct {
	code    *int32
	stopped bool
}

func TestPluginStartSettlesImmediateExitAndStartFailureOnce(t *testing.T) {
	t.Run("immediate exit", func(t *testing.T) {
		exited := make(chan pluginExitResult, 1)
		main := &serialMainThread{}
		var m *Model
		m = &Model{
			ctx:      context.Background(),
			postHook: main.post,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command != "run_start" {
					return nil, nil
				}
				code := int32(0)
				m.post(func() {
					m.acceptExit(openidea.RunExitedEvent{Run: 17, Code: &code})
				})
				return json.Marshal(openidea.RunInfo{
					ID: 17, ConfigID: "plugin:demo:run", Name: "Plugin Run",
				})
			},
		}
		m.setRoot(t.TempDir())
		m.run.trusted = true
		callback := func(code *int32, stopped bool) {
			exited <- pluginExitResult{code: code, stopped: stopped}
		}
		if !m.RunPluginStart(map[string]any{
			"config": map[string]any{
				"id": "plugin:demo:run", "name": "Plugin Run",
				"kind": "shell", "command": "printf done",
			},
			"onExit": callback,
		}) {
			t.Fatalf("plugin run did not start: %s", m.status)
		}
		select {
		case result := <-exited:
			if result.code == nil || *result.code != 0 || result.stopped {
				t.Fatalf("immediate exit = %+v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("immediate plugin exit was not reported")
		}
		code := int32(9)
		main.do(func() {
			m.acceptExit(openidea.RunExitedEvent{Run: 17, Code: &code})
		})
		select {
		case duplicate := <-exited:
			t.Fatalf("duplicate exit callback = %+v", duplicate)
		case <-time.After(30 * time.Millisecond):
		}
	})

	t.Run("start failure", func(t *testing.T) {
		exited := make(chan pluginExitResult, 1)
		main := &serialMainThread{}
		m := &Model{
			ctx:      context.Background(),
			postHook: main.post,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command == "run_start" {
					return nil, errors.New("backend refused launch")
				}
				return nil, nil
			},
		}
		m.setRoot(t.TempDir())
		m.run.trusted = true
		m.RunPluginStart(map[string]any{
			"config": map[string]any{
				"id": "plugin:demo:run", "name": "Plugin Run",
				"kind": "shell", "command": "false",
			},
			"onExit": func(code *int32, stopped bool) {
				exited <- pluginExitResult{code: code, stopped: stopped}
			},
		})
		select {
		case result := <-exited:
			if result.code != nil || result.stopped {
				t.Fatalf("start failure callback = %+v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("start failure did not settle onExit")
		}
	})
}

func TestPluginStartReportsStoppedAndDelegatesDebug(t *testing.T) {
	t.Run("stopped", func(t *testing.T) {
		exited := make(chan pluginExitResult, 1)
		main := &serialMainThread{}
		m := &Model{
			ctx:      context.Background(),
			postHook: main.post,
		}
		m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "run_start":
				return json.Marshal(openidea.RunInfo{
					ID: 23, ConfigID: "plugin:demo:run", Name: "Plugin Run", Running: true,
				})
			case "run_stop":
				id := input.(map[string]any)["id"].(uint32)
				code := int32(0)
				m.post(func() {
					m.acceptExit(openidea.RunExitedEvent{Run: id, Code: &code})
				})
				return json.RawMessage(`null`), nil
			}
			return nil, nil
		}
		m.setRoot(t.TempDir())
		m.run.trusted = true
		m.RunPluginStart(map[string]any{
			"config": map[string]any{
				"id": "plugin:demo:run", "name": "Plugin Run",
				"kind": "shell", "command": "sleep 1",
			},
			"onExit": func(code *int32, stopped bool) {
				exited <- pluginExitResult{code: code, stopped: stopped}
			},
		})
		waitForSerialMainThread(t, main, time.Second, func() bool {
			return m.run.running && m.run.runID == 23
		})
		main.do(func() { m.StopRun() })
		select {
		case result := <-exited:
			if !result.stopped {
				t.Fatalf("Stop callback = %+v, want stopped", result)
			}
		case <-time.After(time.Second):
			t.Fatal("Stop did not settle the plugin callback")
		}
	})

	t.Run("debug delegation", func(t *testing.T) {
		prepared := make(chan struct{}, 1)
		exited := make(chan pluginExitResult, 1)
		main := &serialMainThread{}
		m := &Model{
			ctx:      context.Background(),
			postHook: main.post,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command == "plugin_prepare_debug_run" {
					prepared <- struct{}{}
					return nil, errors.New("preparation failed")
				}
				return nil, nil
			},
		}
		m.setRoot(t.TempDir())
		m.run.trusted = true
		m.RunPluginStart(map[string]any{
			"config": map[string]any{
				"id": "plugin:demo:debug", "name": "Plugin Debug",
				"kind": "application", "command": "java",
			},
			"debug": true,
			"onExit": func(code *int32, stopped bool) {
				exited <- pluginExitResult{code: code, stopped: stopped}
			},
		})
		select {
		case <-prepared:
		case <-time.After(time.Second):
			t.Fatal("run.pluginStart did not delegate to debug.pluginStart")
		}
		select {
		case result := <-exited:
			if result.code != nil || result.stopped {
				t.Fatalf("preparation failure callback = %+v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("debug preparation failure did not settle onExit")
		}
	})
}

func TestPluginStartActionsAreHiddenProjectOnlyDebugCategory(t *testing.T) {
	for _, id := range []string{"run.pluginStart", "debug.pluginStart"} {
		action, ok := actionGet(id)
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		if !hiddenActionIDs[id] {
			t.Fatalf("%s must be hidden", id)
		}
		if action.When(&Model{}) {
			t.Fatalf("%s must require an open project", id)
		}
	}
	debugAction, _ := actionGet("debug.pluginStart")
	if debugAction.Category != "Debug" {
		t.Fatalf("debug.pluginStart category = %q", debugAction.Category)
	}
}
