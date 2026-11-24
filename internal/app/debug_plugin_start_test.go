package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func newDebugPluginTestModel(
	t *testing.T,
	debugStart func(*Model, uint32),
	debugStop func(*Model, uint32),
) *Model {
	t.Helper()
	root := t.TempDir()
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	m.setRoot(root)
	m.run.trusted = true
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "plugin_prepare_debug_run":
			return json.Marshal(map[string]any{
				"id": "plugin:demo:debug", "name": "Plugin Debug",
				"kind": "application", "command": "java", "module": "app",
				"args": []string{"-cp", "${classpath}", "demo.Application"},
			})
		case "java_resolve_launch":
			return json.Marshal(openidea.JavaLaunchInfo{
				Build: strPtr("succeeded"), ClassPaths: []string{root + "/app/target/classes"},
				PathListSeparator: ":", JavaExec: strPtr("/jdk/bin/java"), ProjectRoot: &root,
			})
		case "lsp_servers":
			return json.RawMessage(`[]`), nil
		case "java_debug_server":
			return json.RawMessage(`5005`), nil
		case "debug_start":
			if debugStart != nil {
				debugStart(m, 55)
			}
			return json.RawMessage(`55`), nil
		case "debug_stop":
			id := input.(map[string]any)["session"].(uint32)
			if debugStop != nil {
				debugStop(m, id)
			}
			return json.RawMessage(`null`), nil
		case "debug_capabilities":
			return json.RawMessage(`{}`), nil
		case "debug_set_breakpoints":
			return json.RawMessage(`[]`), nil
		}
		return json.RawMessage(`null`), nil
	}
	return m
}

func pluginDebugArg(
	callback func(code *int32, stopped bool),
) map[string]any {
	return map[string]any{
		"config": map[string]any{
			"id": "plugin:demo:debug", "name": "Plugin Debug",
			"kind": "application", "command": "java", "module": "app",
			"args": []string{"-cp", "${classpath}", "demo.Application"},
		},
		"onExit": callback,
	}
}

func TestDebugPluginStartReportsSuccessAndStopped(t *testing.T) {
	t.Run("success completion", func(t *testing.T) {
		result := make(chan pluginExitResult, 1)
		m := newDebugPluginTestModel(t,
			func(m *Model, id uint32) {
				m.post(func() {
					m.acceptDebugEvent(id, json.RawMessage(`{"kind":"exited","code":0}`))
				})
			},
			nil,
		)
		m.DebugPluginStart(pluginDebugArg(func(code *int32, stopped bool) {
			result <- pluginExitResult{code: code, stopped: stopped}
		}))
		select {
		case got := <-result:
			if got.code == nil || *got.code != 0 || got.stopped {
				t.Fatalf("success callback = %+v", got)
			}
		case <-time.After(2 * time.Second):
			var debugErr string
			m.post(func() { debugErr = m.debug.err })
			t.Fatalf("debug completion was not reported: %s", debugErr)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		result := make(chan pluginExitResult, 1)
		m := newDebugPluginTestModel(t, nil,
			func(m *Model, id uint32) {
				m.post(func() {
					m.acceptDebugEvent(id, json.RawMessage(`{"kind":"terminated"}`))
				})
			},
		)
		m.DebugPluginStart(pluginDebugArg(func(code *int32, stopped bool) {
			result <- pluginExitResult{code: code, stopped: stopped}
		}))
		waitForDynamicAction(t, 2*time.Second, func() bool {
			var ready bool
			m.post(func() {
				ready = m.debug.session == 55 && !m.debug.starting
			})
			return ready
		})
		m.post(func() { m.StopDebug() })
		select {
		case got := <-result:
			if !got.stopped {
				t.Fatalf("Stop callback = %+v", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Stop did not report the plugin debug completion")
		}
	})
}

func TestDebugPluginStartFailureAndStalePreparation(t *testing.T) {
	t.Run("preparation failure", func(t *testing.T) {
		result := make(chan pluginExitResult, 1)
		main := &serialMainThread{}
		m := &Model{
			ctx:      context.Background(),
			postHook: main.post,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command == "plugin_prepare_debug_run" {
					return nil, errors.New("plugin host rejected the configuration")
				}
				return nil, fmt.Errorf("unexpected %s", command)
			},
		}
		m.setRoot(t.TempDir())
		m.run.trusted = true
		m.DebugPluginStart(pluginDebugArg(func(code *int32, stopped bool) {
			result <- pluginExitResult{code: code, stopped: stopped}
		}))
		select {
		case got := <-result:
			if got.code != nil || got.stopped {
				t.Fatalf("preparation failure callback = %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("preparation failure did not settle onExit")
		}
	})

	t.Run("stale preparation", func(t *testing.T) {
		firstStarted := make(chan struct{}, 1)
		releaseFirst := make(chan struct{})
		firstResult := make(chan pluginExitResult, 1)
		secondResult := make(chan pluginExitResult, 1)
		main := &serialMainThread{}
		m := &Model{
			ctx:      context.Background(),
			postHook: main.post,
			invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
				if command != "plugin_prepare_debug_run" {
					return nil, fmt.Errorf("unexpected %s", command)
				}
				raw, _ := json.Marshal(input.(map[string]any)["config"])
				var cfg struct {
					ID string `json:"id"`
				}
				_ = json.Unmarshal(raw, &cfg)
				if cfg.ID == "first" {
					firstStarted <- struct{}{}
					<-releaseFirst
					return json.RawMessage(`{"id":"first","name":"First","kind":"application"}`), nil
				}
				return nil, errors.New("second preparation failed")
			},
		}
		m.setRoot(t.TempDir())
		m.run.trusted = true
		m.DebugPluginStart(map[string]any{
			"config": map[string]any{"id": "first", "name": "First", "kind": "application"},
			"onExit": func(code *int32, stopped bool) {
				firstResult <- pluginExitResult{code: code, stopped: stopped}
			},
		})
		select {
		case <-firstStarted:
		case <-time.After(time.Second):
			t.Fatal("first preparation did not start")
		}
		m.DebugPluginStart(map[string]any{
			"config": map[string]any{"id": "second", "name": "Second", "kind": "application"},
			"onExit": func(code *int32, stopped bool) {
				secondResult <- pluginExitResult{code: code, stopped: stopped}
			},
		})
		select {
		case got := <-secondResult:
			if got.code != nil || got.stopped {
				t.Fatalf("second preparation callback = %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("second preparation did not settle")
		}
		close(releaseFirst)
		select {
		case got := <-firstResult:
			if got.code != nil || !got.stopped {
				t.Fatalf("stale preparation callback = %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("stale preparation callback was not cancelled")
		}
		m.post(func() {
			if m.debug.starting || m.debug.session != 0 {
				t.Fatalf("stale preparation started a session: %+v", m.debug)
			}
		})
	})
}

func TestDebugPluginLateSessionEventDoesNotSettleNewerLaunch(t *testing.T) {
	firstResult := make(chan pluginExitResult, 1)
	secondResult := make(chan pluginExitResult, 1)
	main := &serialMainThread{}
	root := t.TempDir()
	nextSession := uint32(55)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	m.setRoot(root)
	m.run.trusted = true
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "plugin_prepare_debug_run":
			return input.(map[string]any)["config"].(json.RawMessage), nil
		case "java_resolve_launch":
			return json.Marshal(openidea.JavaLaunchInfo{
				Build: strPtr("succeeded"), ClassPaths: []string{root + "/app/target/classes"},
				PathListSeparator: ":", JavaExec: strPtr("/jdk/bin/java"), ProjectRoot: &root,
			})
		case "lsp_servers":
			return json.RawMessage(`[]`), nil
		case "java_debug_server":
			return json.RawMessage(`5005`), nil
		case "debug_start":
			id := nextSession
			nextSession++
			return json.RawMessage(fmt.Sprintf("%d", id)), nil
		case "debug_stop":
			id := input.(map[string]any)["session"].(uint32)
			m.post(func() {
				m.acceptDebugEvent(id, json.RawMessage(`{"kind":"terminated"}`))
			})
			return json.RawMessage(`null`), nil
		case "debug_capabilities":
			return json.RawMessage(`{}`), nil
		case "debug_set_breakpoints":
			return json.RawMessage(`[]`), nil
		}
		return json.RawMessage(`null`), nil
	}

	m.DebugPluginStart(pluginDebugArg(func(code *int32, stopped bool) {
		firstResult <- pluginExitResult{code: code, stopped: stopped}
	}))
	waitForDynamicAction(t, 2*time.Second, func() bool {
		var ready bool
		m.post(func() { ready = m.debug.session == 55 && !m.debug.starting })
		return ready
	})
	m.post(func() { m.StopDebug() })
	select {
	case got := <-firstResult:
		if !got.stopped {
			t.Fatalf("first callback = %+v, want stopped", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first plugin debug session did not stop")
	}

	m.DebugPluginStart(pluginDebugArg(func(code *int32, stopped bool) {
		secondResult <- pluginExitResult{code: code, stopped: stopped}
	}))
	waitForDynamicAction(t, 2*time.Second, func() bool {
		var ready bool
		m.post(func() { ready = m.debug.session == 56 && !m.debug.starting })
		return ready
	})
	m.post(func() {
		code := int32(9)
		m.acceptDebugEvent(55, json.RawMessage(
			fmt.Sprintf(`{"kind":"exited","code":%d}`, code),
		))
	})
	select {
	case got := <-secondResult:
		t.Fatalf("late old-session event settled the newer callback: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
	m.post(func() {
		m.acceptDebugEvent(56, json.RawMessage(`{"kind":"exited","code":0}`))
	})
	select {
	case got := <-secondResult:
		if got.code == nil || *got.code != 0 || got.stopped {
			t.Fatalf("second callback = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("newer plugin debug session did not settle")
	}
}
