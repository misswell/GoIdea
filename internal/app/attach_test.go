package app

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func intPtr(value int) *int { return &value }

func TestAttachRootHonorsAnAbsoluteModule(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "services", "gateway")
	if got, want := attachRoot(module, root), filepath.Clean(module); got != want {
		t.Fatalf("attachRoot(%q, %q) = %q, want %q", module, root, got, want)
	}
	if got, want := attachRoot("services/gateway", root), filepath.Join(root, "services", "gateway"); got != want {
		t.Fatalf("relative attachRoot = %q, want %q", got, want)
	}
}

func TestAttachPythonUsesDebuggerPageOptions(t *testing.T) {
	m := &Model{
		ctx: context.Background(),
		prefs: Preferences{Debugger: DebuggerPrefs{
			NoPythonAttachSubprocess: true,
			PythonGevent:             true,
		}},
	}
	launch, err := m.attachLaunchConfig(context.Background(), openidea.AttachableProcess{
		PID: 21, Name: "python3", Kind: "python", Adapter: "debugpy",
		Port: intPtr(5678),
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	extra := decodeLaunchExtra(t, launch)
	if extra["subProcess"] != false || extra["gevent"] != true {
		t.Fatalf("Python debugger options = %#v", extra)
	}
}

func decodeLaunchExtra(t *testing.T, launch openidea.DebugLaunchConfig) map[string]any {
	t.Helper()
	if launch.ExtraJSON == nil {
		t.Fatal("debug launch has no extraJson")
	}
	var extra map[string]any
	if err := stdjson.Unmarshal([]byte(*launch.ExtraJSON), &extra); err != nil {
		t.Fatalf("extraJson is invalid: %v", err)
	}
	return extra
}

func TestAttachLaunchConfigsMatchPinnedAdapters(t *testing.T) {
	root := t.TempDir()
	m := &Model{ctx: context.Background()}
	tests := []struct {
		name    string
		process openidea.AttachableProcess
		adapter string
		extra   map[string]any
	}{
		{
			name: "node inspector",
			process: openidea.AttachableProcess{
				PID: 11, Name: "node", Kind: "node", Adapter: "js-debug",
				Host: stringPtr("127.0.0.1"), Port: intPtr(9229),
			},
			adapter: "js-debug",
			extra: map[string]any{
				"type": "pwa-node", "address": "127.0.0.1", "port": float64(9229),
				"continueOnAttach": true,
			},
		},
		{
			name: "node process",
			process: openidea.AttachableProcess{
				PID: 12, Name: "node", Kind: "node", Adapter: "js-debug",
			},
			adapter: "js-debug",
			extra:   map[string]any{"processId": "12", "continueOnAttach": true},
		},
		{
			name: "debugpy listener",
			process: openidea.AttachableProcess{
				PID: 21, Name: "python3", Kind: "python", Adapter: "debugpy",
				Port: intPtr(5678),
			},
			adapter: "debugpy",
			extra: map[string]any{
				"debugServer": float64(5678), "justMyCode": true, "subProcess": true,
			},
		},
		{
			name: "debugpy process",
			process: openidea.AttachableProcess{
				PID: 22, Name: "python3", Kind: "python", Adapter: "debugpy",
			},
			adapter: "debugpy",
			extra: map[string]any{
				"processId": float64(22), "justMyCode": true, "subProcess": true,
			},
		},
		{
			name: "delve process",
			process: openidea.AttachableProcess{
				PID: 31, Name: "app", Kind: "go", Adapter: "delve",
			},
			adapter: "delve",
			extra:   map[string]any{"mode": "local", "processId": float64(31)},
		},
		{
			name: "native process",
			process: openidea.AttachableProcess{
				PID: 41, Name: "app", Kind: "native", Adapter: "codelldb",
			},
			adapter: "codelldb",
			extra:   map[string]any{"pid": float64(41)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			launch, err := m.attachLaunchConfig(context.Background(), test.process, root)
			if err != nil {
				t.Fatal(err)
			}
			if launch.Adapter != test.adapter || launch.Request != "attach" ||
				launch.Cwd != filepath.Clean(root) || launch.Program != nil {
				t.Fatalf("launch = %+v", launch)
			}
			extra := decodeLaunchExtra(t, launch)
			for key, want := range test.extra {
				if fmt.Sprint(extra[key]) != fmt.Sprint(want) {
					t.Fatalf("extra[%q] = %#v, want %#v", key, extra[key], want)
				}
			}
		})
	}
}

func TestJavaAttachUsesPinnedDebugServerPayload(t *testing.T) {
	root := t.TempDir()
	calls := make(chan map[string]any, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (stdjson.RawMessage, error) {
			if command != "java_debug_server" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			calls <- input.(map[string]any)
			return stdjson.RawMessage(`4711`), nil
		},
	}
	launch, err := m.attachLaunchConfig(context.Background(), openidea.AttachableProcess{
		PID: 51, Name: "java", Kind: "java", Adapter: "java-debug",
		Host: stringPtr("remote.example"), Port: intPtr(5005),
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-calls:
		if args["module"] != "" {
			t.Fatalf("java_debug_server arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("java_debug_server was not called")
	}
	extra := decodeLaunchExtra(t, launch)
	if launch.Adapter != "java-debug" || launch.Request != "attach" ||
		extra["debugServer"] != float64(4711) ||
		extra["hostName"] != "remote.example" ||
		extra["port"] != float64(5005) || extra["timeout"] != float64(30000) {
		t.Fatalf("Java attach launch = %+v, extra = %#v", launch, extra)
	}
}

func TestAttachKeepsConcurrentDebugSession(t *testing.T) {
	root := t.TempDir()
	request := make(chan openidea.DebugLaunchConfig, 1)
	capabilities := make(chan uint32, 1)
	posted := make(chan struct{}, 8)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (stdjson.RawMessage, error) {
			switch command {
			case "debug_start":
				request <- input.(map[string]any)["config"].(openidea.DebugLaunchConfig)
				return stdjson.RawMessage(`22`), nil
			case "debug_capabilities":
				capabilities <- input.(map[string]any)["session"].(uint32)
				return stdjson.RawMessage(`{}`), nil
			case "debug_set_function_breakpoints", "debug_set_data_breakpoints":
				return stdjson.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	m.run.trusted = true
	m.debug = DebugPane{
		active: 11, session: 11, tabName: "Live", status: "Running…",
		stopLine: -1, sessions: map[uint32]*debugSessionState{},
		sessionOrder: []uint32{11},
	}
	m.debug.rememberDebugSession()

	m.AttachToProcess(openidea.AttachableProcess{
		PID: 61, Name: "node", Kind: "node", Adapter: "js-debug",
		Port: intPtr(9229),
	})
	select {
	case launch := <-request:
		if launch.Adapter != "js-debug" || launch.Request != "attach" {
			t.Fatalf("attach launch = %+v", launch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("attach did not call debug_start")
	}
	select {
	case session := <-capabilities:
		if session != 22 {
			t.Fatalf("capability session = %d", session)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("attach did not refresh its capabilities")
	}
	deadline := time.After(2 * time.Second)
	for m.debug.active != 22 || m.debug.starting {
		select {
		case <-posted:
		case <-deadline:
			t.Fatalf("attach did not adopt session: %+v", m.debug)
		}
	}
	if m.debug.sessions[11] == nil || m.debug.sessions[11].session != 11 {
		t.Fatalf("live session was removed while attaching: %+v", m.debug.sessions)
	}
	if m.debug.session != 22 || m.debug.tabName != "node" {
		t.Fatalf("attached tab = %+v", m.debug)
	}
}

func TestRemoteAttachPayloadsMatchPinnedOpenIDEA(t *testing.T) {
	root := t.TempDir()
	m := &Model{ctx: context.Background()}
	tests := []struct {
		kind    string
		params  map[string]any
		adapter string
		request string
		check   map[string]any
	}{
		{
			kind: "go-remote", params: map[string]any{
				"host": "devbox", "port": "2345", "remoteRoot": "/src/app",
			},
			adapter: "delve", request: "attach",
			check: map[string]any{"debugServer": "devbox:2345", "mode": "remote"},
		},
		{
			kind: "python-remote", params: map[string]any{
				"host": "::1", "port": "5679", "remoteRoot": "/app",
			},
			adapter: "debugpy", request: "attach",
			check: map[string]any{
				"debugServer": "[::1]:5679", "justMyCode": true, "subProcess": true,
			},
		},
		{
			kind: "node-remote", params: map[string]any{
				"host": "127.0.0.1", "port": "9230",
			},
			adapter: "js-debug", request: "attach",
			check: map[string]any{"type": "pwa-node", "address": "127.0.0.1", "port": float64(9230)},
		},
		{
			kind: "php-remote", params: map[string]any{
				"port": "9004", "remoteRoot": "/var/www",
			},
			adapter: "php-debug", request: "launch",
			check: map[string]any{"port": float64(9004)},
		},
		{
			kind: "godot-attach", params: map[string]any{
				"host": "localhost", "port": "6007",
			},
			adapter: "godot", request: "attach",
			check: map[string]any{"debugServer": "localhost:6007", "project": root},
		},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			config := &openidea.RunConfig{
				Name: test.kind, Kind: test.kind, Params: test.params,
			}
			launch, err := m.remoteAttachLaunch(config, root)
			if err != nil {
				t.Fatal(err)
			}
			if launch.Adapter != test.adapter || launch.Request != test.request || launch.Cwd != root {
				t.Fatalf("launch = %+v", launch)
			}
			extra := decodeLaunchExtra(t, launch)
			for key, want := range test.check {
				if fmt.Sprint(extra[key]) != fmt.Sprint(want) {
					t.Fatalf("extra[%q] = %#v, want %#v", key, extra[key], want)
				}
			}
		})
	}
}

func TestRemoteJvmListenRelaysToLoopbackAttach(t *testing.T) {
	root := t.TempDir()
	listenArgs := make(chan map[string]any, 1)
	serverCalls := make(chan struct{}, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (stdjson.RawMessage, error) {
			switch command {
			case "debug_listen_for_jvm":
				listenArgs <- input.(map[string]any)
				return stdjson.RawMessage(`49111`), nil
			case "java_debug_server":
				serverCalls <- struct{}{}
				return stdjson.RawMessage(`4711`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	config := &openidea.RunConfig{
		Name: "Remote JVM", Kind: "remote-jvm",
		Params: map[string]any{
			"mode": "listen", "host": "0.0.0.0", "port": "5005",
		},
	}
	launch, err := m.remoteJvmLaunch(context.Background(), config, root)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-listenArgs:
		if args["request"] == uint32(0) || args["host"] != "0.0.0.0" ||
			args["port"] != 5005 {
			t.Fatalf("listen arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("remote JVM listen did not call the backend")
	}
	select {
	case <-serverCalls:
	case <-time.After(time.Second):
		t.Fatal("remote JVM listen did not start java-debug")
	}
	extra := decodeLaunchExtra(t, launch)
	if extra["hostName"] != "127.0.0.1" || extra["port"] != float64(49111) ||
		extra["debugServer"] != float64(4711) {
		t.Fatalf("remote JVM launch = %+v, extra = %#v", launch, extra)
	}
}
