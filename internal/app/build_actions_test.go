package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestBuildActionsPassOpenIDEATargetsActivationAndCMake(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		arg        any
		command    string
		kind       string
		file       string
		cmake      string
		wantShown  bool
		hasKind    bool
		hasCMake   bool
		hasFileKey bool
	}{
		{
			name: "project hidden cmake", action: "build.buildProject",
			arg:     map[string]any{"activate": false, "cmake": "cmake:debug:app:build"},
			command: "build_run", kind: "build", cmake: "cmake:debug:app:build",
			hasKind: true, hasCMake: true, hasFileKey: true,
		},
		{
			name: "module path", action: "build.buildModule",
			arg: "/work/demo/core", command: "build_run", kind: "build",
			file: "/work/demo/core", wantShown: true, hasKind: true, hasFileKey: true,
		},
		{
			name: "module options", action: "build.buildModule",
			arg:        map[string]any{"target": "/work/demo/app", "activate": false, "wait": true},
			command:    "build_run",
			kind:       "build",
			file:       "/work/demo/app",
			hasKind:    true,
			hasFileKey: true,
		},
		{
			name: "artifacts hidden", action: "build.buildArtifacts",
			arg:     map[string]any{"activate": false, "wait": true},
			command: "build_run", kind: "artifacts", hasKind: true, hasFileKey: true,
		},
		{
			name: "artifacts target", action: "build.buildArtifacts",
			arg: "/work/demo/core", command: "build_run", kind: "artifacts",
			file: "/work/demo/core", wantShown: true, hasKind: true, hasFileKey: true,
		},
		{
			name: "compile target", action: "build.compileFile",
			arg:     map[string]any{"target": "/work/demo/src/App.java", "activate": false},
			command: "build_compile_file", file: "/work/demo/src/App.java", hasFileKey: true,
		},
		{
			name: "rebuild activates", action: "build.rebuildProject",
			arg:     map[string]any{"activate": false, "cmake": "cmake:release:app:build"},
			command: "build_run", kind: "rebuild", cmake: "cmake:release:app:build",
			wantShown: true, hasKind: true, hasCMake: true, hasFileKey: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			type call struct {
				command string
				args    map[string]any
			}
			calls := make(chan call, 1)
			m := &Model{
				ctx: context.Background(),
				invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
					calls <- call{command: command, args: args.(map[string]any)}
					return json.RawMessage(`{"success":true,"messages":[]}`), nil
				},
				postHook: func(fn func()) { fn() },
			}
			m.setRoot("/work/demo")
			m.run.trusted = true

			if !m.RunActionWithArg(test.action, test.arg) {
				t.Fatalf("%s was unavailable", test.action)
			}
			var got call
			select {
			case got = <-calls:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s did not call the backend", test.action)
			}
			if got.command != test.command {
				t.Fatalf("command = %q, want %q", got.command, test.command)
			}
			if test.hasKind && got.args["kind"] != test.kind {
				t.Fatalf("kind = %#v, want %q", got.args["kind"], test.kind)
			}
			if test.hasFileKey {
				file, _ := got.args["file"].(*string)
				if test.file == "" && file != nil {
					t.Fatalf("file = %q, want nil", *file)
				}
				if test.file != "" && (file == nil || *file != test.file) {
					t.Fatalf("file = %#v, want %q", got.args["file"], test.file)
				}
			}
			if test.hasCMake {
				if got.args["cmake"] != test.cmake {
					t.Fatalf("cmake = %#v, want %q", got.args["cmake"], test.cmake)
				}
			}
			if m.build.visible != test.wantShown {
				t.Fatalf("Build visible = %t, want %t", m.build.visible, test.wantShown)
			}
		})
	}
}

func TestBuildActionsAreGatedByBuildState(t *testing.T) {
	m := &Model{}
	m.setRoot("/work/demo")
	if !m.ActionEnabled("build.buildProject") {
		t.Fatal("Build Project was disabled for an idle project")
	}
	m.build.busy = true
	if m.ActionEnabled("build.buildProject") || !m.ActionEnabled("build.stop") {
		t.Fatal("build actions did not follow the running state")
	}
}

func TestBuildActionWithoutWaitRemainsNonblocking(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(ctx context.Context, _ string, _ any) (json.RawMessage, error) {
			started <- struct{}{}
			select {
			case <-release:
				return json.RawMessage(`{"success":true,"messages":[]}`), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/work/demo")
	m.run.trusted = true

	if !m.RunActionWithArg("build.buildProject", nil) {
		t.Fatal("Build Project was unavailable")
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("non-waiting build did not start")
	}
	if !m.build.busy {
		t.Fatal("ordinary build action returned after completion instead of starting asynchronously")
	}
	close(release)
	waitRunChain(t, 2*time.Second, func() bool {
		var busy bool
		m.post(func() { busy = m.build.busy })
		return !busy
	})
}

func TestHiddenBuildRevealsBuildWindowOnlyWhenItFails(t *testing.T) {
	for _, test := range []struct {
		name    string
		success bool
		want    bool
	}{
		{name: "success stays hidden", success: true},
		{name: "failure reveals output", success: false, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			finished := make(chan struct{}, 1)
			editor := &Tab{editorFocused: true}
			m := &Model{
				ctx:  context.Background(),
				tabs: []*Tab{editor},
				invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
					if test.success {
						return json.RawMessage(`{"success":true,"messages":[]}`), nil
					}
					return json.RawMessage(`{"success":false,"errors":1,"messages":[]}`), nil
				},
				postHook: func(fn func()) {
					fn()
					finished <- struct{}{}
				},
			}
			m.setRoot("/work/demo")
			m.run.trusted = true

			if !m.RunActionWithArg("build.buildProject", map[string]any{"activate": false}) {
				t.Fatal("hidden Build Project was unavailable")
			}
			select {
			case <-finished:
			case <-time.After(2 * time.Second):
				t.Fatal("hidden build did not finish")
			}
			if got := m.dockWindowShown("build"); got != test.want {
				t.Fatalf("Build window shown = %t, want %t", got, test.want)
			}
			if !editor.editorFocused {
				t.Fatal("revealing a failed build moved keyboard focus away from the editor")
			}
		})
	}
}

func TestBeforeLaunchBuildWaitHoldsMainUntilSuccess(t *testing.T) {
	hook := &runChainHook{
		runIn:    make(chan string, 1),
		buildIn:  make(chan string, 1),
		buildOut: make(chan openidea.BuildResult, 1),
	}
	m := newRunChainModel(t, hook, map[string]any{
		"id": "app", "name": "App", "kind": "shell", "command": "echo",
		"beforeLaunch": []map[string]string{{"kind": "buildModule", "target": "core"}},
	})
	app := m.run.configs[0]
	if !m.startRunWith(app.Raw(), app.Name) {
		t.Fatal("build-before-launch did not start")
	}
	select {
	case kind := <-hook.buildIn:
		if kind != "build" {
			t.Fatalf("build kind = %q, want build", kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("build did not start")
	}

	select {
	case id := <-hook.runIn:
		t.Fatalf("%s started while its build was still running", id)
	case <-time.After(100 * time.Millisecond):
	}

	hook.buildOut <- openidea.BuildResult{Title: "Build", Success: true}
	select {
	case id := <-hook.runIn:
		if id != "app" {
			t.Fatalf("started %q, want app", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("main configuration did not start after the build succeeded")
	}
}

func TestBeforeLaunchBuildWaitRejectsFailure(t *testing.T) {
	hook := &runChainHook{
		runIn:    make(chan string, 1),
		buildIn:  make(chan string, 1),
		buildOut: make(chan openidea.BuildResult, 1),
	}
	m := newRunChainModel(t, hook, map[string]any{
		"id": "app", "name": "App", "kind": "shell", "command": "echo",
		"beforeLaunch": []map[string]string{{"kind": "buildArtifacts"}},
	})
	app := m.run.configs[0]
	if !m.startRunWith(app.Raw(), app.Name) {
		t.Fatal("build-before-launch did not start")
	}
	select {
	case <-hook.buildIn:
	case <-time.After(2 * time.Second):
		t.Fatal("Build Artifacts did not start")
	}
	hook.buildOut <- openidea.BuildResult{Title: "Build", Errors: 1}

	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		return !m.run.preparingGroup && !m.run.busy && !m.run.awaiting
	})
	if !strings.Contains(m.run.err, "Build failed") {
		t.Fatalf("run error = %q, want the build failure", m.run.err)
	}
	select {
	case id := <-hook.runIn:
		t.Fatalf("main configuration %q started after a failed build", id)
	default:
	}
}

func TestBeforeLaunchBuildStopNeverStartsMain(t *testing.T) {
	hook := &runChainHook{
		runIn:    make(chan string, 1),
		buildIn:  make(chan string, 1),
		buildOut: make(chan openidea.BuildResult, 1),
		buildEnd: make(chan struct{}, 1),
	}
	m := newRunChainModel(t, hook, map[string]any{
		"id": "app", "name": "App", "kind": "shell", "command": "echo",
		"beforeLaunch": []map[string]string{{"kind": "build"}},
	})
	app := m.run.configs[0]
	if !m.startRunWith(app.Raw(), app.Name) {
		t.Fatal("build-before-launch did not start")
	}
	select {
	case <-hook.buildIn:
	case <-time.After(2 * time.Second):
		t.Fatal("Build Project did not start")
	}
	m.post(func() { m.StopBuild() })

	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		return !m.run.preparingGroup && !m.run.busy && !m.run.awaiting
	})
	select {
	case id := <-hook.runIn:
		t.Fatalf("main configuration %q started after the build was stopped", id)
	default:
	}
}
