package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func setTestDebugCapabilities(m *Model, session uint32, caps openidea.DebugCapabilities) {
	if m.debug.sessions == nil {
		m.debug.sessions = map[uint32]*debugSessionState{}
	}
	m.debug.active = session
	m.debug.session = session
	m.debug.caps = caps
	m.debug.capsKnown = true
	m.debug.rememberDebugSession()
}

func TestDebugSetValueUsesSelectedContainerAndFrame(t *testing.T) {
	gotArgs := make(chan map[string]any, 1)
	applied := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_set_variable" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			args := input.(map[string]any)
			gotArgs <- args
			return json.RawMessage(`{"name":"count","value":"9","typeName":"int","variablesReference":0,"evaluateName":"count"}`), nil
		},
	}
	name := "count"
	m.debug.paused = true
	m.debug.session, m.debug.frameID, m.debug.varContainer = 7, 31, 12
	m.debug.vars = []openidea.Variable{{
		Name: "count", Value: "3", VariablesReference: 0, EvaluateName: &name,
	}}
	setTestDebugCapabilities(m, 7, openidea.DebugCapabilities{SetValue: true})
	m.OpenDebugSetValue()
	m.debug.setValue.value = "9"
	m.commitDebugSetValue()
	select {
	case args := <-gotArgs:
		if args["session"] != uint32(7) || args["container"] != int64(12) ||
			args["frameId"] != int64(31) || args["name"] != "count" ||
			args["evaluateName"] != &name || args["value"] != "9" {
			t.Fatalf("Set Value arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Set Value did not reach the debugger")
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("Set Value did not apply its reply")
	}
	if m.debug.setValue.open || m.debug.vars[0].Value != "9" {
		t.Fatalf("Set Value result was not applied: %+v", m.debug.setValue)
	}
}

func TestDebugSmartStepUsesBackendTargets(t *testing.T) {
	requests := make(chan map[string]any, 2)
	applied := make(chan struct{}, 2)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args := input.(map[string]any)
			args["command"] = command
			requests <- args
			switch command {
			case "debug_step_in_targets":
				return json.RawMessage(`[
					{"id":44,"label":"service.call()","line":9,"column":4,"endColumn":16},
					{"id":45,"label":"repository.load()","line":9,"column":18,"endColumn":31}
				]`), nil
			case "debug_step_in_target":
				return json.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.debug.paused = true
	m.debug.session, m.debug.stopThread = 5, 6
	m.debug.frameID = 88
	m.debug.frames = []openidea.StackFrame{{ID: 88, Name: "run"}}
	setTestDebugCapabilities(m, 5, openidea.DebugCapabilities{StepInTargets: true})
	m.debug.paused = true
	m.debug.stopThread = 6
	m.debug.frameID = 88
	m.debug.frames = []openidea.StackFrame{{ID: 88, Name: "run"}}
	m.DebugSmartStepInto()
	select {
	case args := <-requests:
		if args["command"] != "debug_step_in_targets" ||
			args["session"] != uint32(5) || args["frameId"] != int64(88) {
			t.Fatalf("Smart Step arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Smart Step did not ask the adapter for targets")
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("Smart Step targets were not applied")
	}
	m.stepIntoDebugTarget(44)
	select {
	case args := <-requests:
		if args["command"] != "debug_step_in_target" ||
			args["session"] != uint32(5) || args["threadId"] != int64(6) ||
			args["targetId"] != int64(44) {
			t.Fatalf("Smart Step target arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Smart Step did not enter the selected target")
	}
}

func TestWaitForDebugSessionStopWaitsForMainThreadState(t *testing.T) {
	callbacks := make(chan func(), 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { callbacks <- fn },
	}
	m.debug.session = 17
	m.debug.stopping = true
	result := make(chan bool, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { result <- m.waitForDebugSessionStop(ctx, 17) }()

	select {
	case callback := <-callbacks:
		callback()
	case <-time.After(time.Second):
		t.Fatal("stop-state query was not posted to the main thread")
	}
	select {
	case got := <-result:
		t.Fatalf("wait returned before the main-thread state changed: %v", got)
	default:
	}
	m.debug.session = 0
	m.debug.ended = true
	select {
	case callback := <-callbacks:
		callback()
	case <-time.After(time.Second):
		t.Fatal("second stop-state query was not posted")
	}
	select {
	case got := <-result:
		if !got {
			t.Fatal("wait did not observe the stopped session")
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not consume the main-thread reply")
	}
}

func TestWaitForDebugSessionStopRejectsFailedStop(t *testing.T) {
	m := &Model{ctx: context.Background(), postHook: func(fn func()) { fn() }}
	m.debug.session = 23
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if m.waitForDebugSessionStop(ctx, 23) {
		t.Fatal("a still-live session with no stop in progress was treated as stopped")
	}
}

func TestDebugInternalStepTargetRejectsZero(t *testing.T) {
	called := false
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
			called = true
			return nil, nil
		},
	}
	m.stepIntoDebugTarget(0)
	if called {
		t.Fatal("a zero Smart Step target reached the backend")
	}
}

func TestDebugResetFrameHonorsCapabilityAndUsesSelectedFrame(t *testing.T) {
	requests := make(chan map[string]any, 1)
	posted := make(chan struct{}, 2)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_restart_frame" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(`null`), nil
		},
	}
	m.debug.paused = true
	m.debug.session = 7
	m.debug.frames = []openidea.StackFrame{
		{ID: 30, Name: "caller"}, {ID: 20, Name: "selected"}, {ID: 10, Name: "main"},
	}
	m.debug.selFrame = 1
	m.debug.capsKnown = false
	m.DebugResetFrame()
	if got := m.debug.err; got != "Reading the debugger capabilities" {
		t.Fatalf("unknown capability reason = %q", got)
	}
	m.debug.capsKnown = true
	m.debug.caps.RestartFrame = false
	m.DebugResetFrame()
	if got := m.debug.err; got != "This debugger does not support Reset Frame" {
		t.Fatalf("unsupported capability reason = %q", got)
	}
	m.debug.caps.RestartFrame = true
	if reason := m.debugResetFrameDisabledReasonAt(1); reason != "" {
		t.Fatalf("selected frame disabled reason = %q", reason)
	}
	if reason := m.debugResetFrameDisabledReasonAt(2); reason != "The bottom stack frame cannot be reset" {
		t.Fatalf("bottom frame disabled reason = %q", reason)
	}
	m.DebugResetFrame()
	select {
	case args := <-requests:
		if args["session"] != uint32(7) || args["frameId"] != int64(20) {
			t.Fatalf("Reset Frame arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Reset Frame did not call the backend")
	}
	select {
	case <-posted:
	case <-time.After(time.Second):
		t.Fatal("Reset Frame result was not applied")
	}
	if m.debug.status != "Frame reset" || m.debug.paused {
		t.Fatalf("Reset Frame result = %+v", m.debug)
	}
}

func TestDebugTemporaryBreakpointIsRemovedAtItsStop(t *testing.T) {
	pushed := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command == "debug_set_breakpoints" {
				pushed <- struct{}{}
			}
			return json.RawMessage(`[]`), nil
		},
	}
	path := "/project/src/Example.java"
	m.debug.bps = map[string][]int{path: {12}}
	m.debug.temporaryBps = map[string][]int{path: {12}}
	m.removeTemporaryBreakpoint(path, 12)
	if containsInt(m.debug.bps[path], 12) || containsInt(m.debug.temporaryBps[path], 12) {
		t.Fatalf("temporary breakpoint survived its stop: %+v", m.debug)
	}
	select {
	case <-pushed:
	case <-time.After(time.Second):
		t.Fatal("removed temporary breakpoint was not pushed to the adapter")
	}
}

func TestJavaSpecialBreakpointParsers(t *testing.T) {
	source := `package demo.sample;

public class Example {
    private int count;

    public String greet(String name) {
        return "Hello " + name;
    }
}
`
	if got := javaDebugMethodAt(source, 5); got != "demo.sample.Example#greet" {
		t.Fatalf("method = %q", got)
	}
	if got := javaDebugFieldAt(source, 3); got != "demo.sample.Example#count" {
		t.Fatalf("field = %q", got)
	}
}

func TestDebugHeapCaptureAdoptsSnapshotAndClasses(t *testing.T) {
	var mu sync.Mutex
	var captured map[string]any
	classesLoaded := make(chan struct{})
	applied := make(chan struct{}, 4)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "debug_heap_capture":
				mu.Lock()
				captured = input.(map[string]any)
				mu.Unlock()
				return json.RawMessage(`{"status":"ready","snapshot":{"id":9,"label":"Example heap","summary":{"objects":2,"reachableBytes":2048,"classes":1}}}`), nil
			case "debug_heap_classes":
				close(classesLoaded)
				return json.RawMessage(`[{"classIndex":3,"name":"demo.Example","count":2,"shallow":64,"retained":2048}]`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	setTestDebugCapabilities(m, 14, openidea.DebugCapabilities{JVMHeap: true})
	m.DebugCaptureHeap()
	select {
	case <-classesLoaded:
	case <-time.After(time.Second):
		t.Fatal("heap class table was not read")
	}
	deadline := time.After(time.Second)
	for len(m.debug.heap.classes) == 0 {
		select {
		case <-applied:
		case <-deadline:
			t.Fatal("heap class table reply was not applied")
		}
	}
	mu.Lock()
	args := captured
	mu.Unlock()
	if args == nil || args["session"] != uint32(14) || args["request"] == uint32(0) {
		t.Fatalf("heap capture arguments = %#v", args)
	}
	progress, ok := args["onProgress"].(map[string]any)
	if !ok || progress["$channel"] == uint32(0) {
		t.Fatalf("heap progress channel = %#v", args["onProgress"])
	}
	if m.debug.heap.info == nil || m.debug.heap.info.ID != 9 ||
		len(m.debug.heap.classes) != 1 || m.debug.heap.classes[0].Name != "demo.Example" {
		t.Fatalf("heap result = %+v", m.debug.heap)
	}
}

func TestDebugHeapProgressRendersPhaseAndFraction(t *testing.T) {
	m := &Model{}
	m.debug.heap.channel = 8
	m.acceptDebugHeapProgress(8, json.RawMessage(`{"type":"analyzing","phase":"dominators","done":2,"total":4}`))
	if m.debug.heap.progress != "Heap dominators · 50%" {
		t.Fatalf("heap progress = %q", m.debug.heap.progress)
	}
}

func TestDebugPluginStartPreparesConfiguration(t *testing.T) {
	requests := make(chan map[string]any, 1)
	posted := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "plugin_prepare_debug_run" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(`{
				"id":"external:demo:run",
				"name":"Plugin run",
				"kind":"external",
				"command":"java",
				"args":["-cp",".","demo.Main"],
				"debugAdapter":"java-debug",
				"params":{"mainClass":"demo.Main"}
			}`), nil
		},
	}
	m.setRoot(t.TempDir())
	m.run.trusted = false
	m.DebugPluginStart(map[string]any{
		"config": map[string]any{
			"id": "external:demo:run", "name": "Plugin run", "kind": "external",
		},
	})
	select {
	case args := <-requests:
		raw, ok := args["config"].(json.RawMessage)
		if !ok || !strings.Contains(string(raw), `"external:demo:run"`) {
			t.Fatalf("plugin debug config = %#v", args["config"])
		}
	case <-time.After(time.Second):
		t.Fatal("plugin debug configuration was not prepared")
	}
	select {
	case <-posted:
	case <-time.After(time.Second):
		t.Fatal("prepared plugin debug configuration was not applied")
	}
	if !strings.Contains(m.debug.err, "not trusted") || m.debug.starting {
		t.Fatalf("untrusted plugin launch did not stop at the trust gate: %+v", m.debug)
	}
}

func TestDebugHotApplicationActionsUseTheirDartCommands(t *testing.T) {
	requests := make(chan map[string]any, 2)
	finished := make(chan string, 2)
	var m *Model
	m = &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			finished <- m.debug.status
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args := input.(map[string]any)
			args["command"] = command
			requests <- args
			return json.RawMessage(`null`), nil
		},
	}
	m.debug.session, m.debug.adapter = 41, "java-debug"
	m.DebugHotReload()
	if !strings.Contains(m.debug.err, "Dart or Flutter") {
		t.Fatalf("non-Dart Hot Reload error = %q", m.debug.err)
	}
	select {
	case args := <-requests:
		t.Fatalf("non-Dart Hot Reload reached the backend: %#v", args)
	default:
	}

	m.debug.adapter = "dart"
	m.debug.err = ""
	m.DebugHotReload()
	select {
	case args := <-requests:
		if args["command"] != "debug_hot_reload" || args["session"] != uint32(41) {
			t.Fatalf("Hot Reload arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Hot Reload did not reach the backend")
	}
	select {
	case status := <-finished:
		if status != "Application reloaded" {
			t.Fatalf("Hot Reload status = %q", status)
		}
	case <-time.After(time.Second):
		t.Fatal("Hot Reload did not finish")
	}

	m.DebugHotRestart()
	select {
	case args := <-requests:
		if args["command"] != "debug_hot_restart" || args["session"] != uint32(41) {
			t.Fatalf("Hot Restart arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Hot Restart did not reach the backend")
	}
	select {
	case status := <-finished:
		if status != "Application restarted" {
			t.Fatalf("Hot Restart status = %q", status)
		}
	case <-time.After(time.Second):
		t.Fatal("Hot Restart did not finish")
	}
}

func TestDebugEvaluateDialogUsesTheSelectedFrame(t *testing.T) {
	requests := make(chan map[string]any, 1)
	applied := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_evaluate" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(`{"name":"count + 1","value":"5","typeName":"int","variablesReference":0}`), nil
		},
	}
	m.debug.paused = true
	m.debug.frameID = 73
	setTestDebugCapabilities(m, 19, openidea.DebugCapabilities{})
	m.OpenDebugEvaluate()
	m.debug.evaluate.expression = "count + 1"
	m.commitDebugEvaluate()

	select {
	case args := <-requests:
		if args["session"] != uint32(19) || args["frameId"] != int64(73) ||
			args["expression"] != "count + 1" {
			t.Fatalf("Evaluate arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Evaluate did not reach the backend")
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("Evaluate did not apply its result")
	}
	if !m.debug.evaluate.hasResult || m.debug.evaluate.result.Value != "5" ||
		m.debug.evaluate.busy {
		t.Fatalf("Evaluate result = %+v", m.debug.evaluate)
	}
}

func TestDebuggerExecutionActionsSendPinnedPayloads(t *testing.T) {
	tests := []struct {
		name    string
		command string
		paused  bool
		invoke  func(*Model)
		args    map[string]any
	}{
		{
			name: "resume", command: "debug_resume", paused: true,
			invoke: func(m *Model) { m.DebugContinue() },
			args:   map[string]any{"session": uint32(7), "threadId": int64(11)},
		},
		{
			name: "pause", command: "debug_pause",
			invoke: func(m *Model) { m.DebugPause() },
			args:   map[string]any{"session": uint32(7), "threadId": int64(11)},
		},
		{
			name: "step over", command: "debug_step", paused: true,
			invoke: func(m *Model) { m.StepOver() },
			args:   map[string]any{"session": uint32(7), "threadId": int64(11), "kind": "over"},
		},
		{
			name: "step into", command: "debug_step", paused: true,
			invoke: func(m *Model) { m.StepInto() },
			args:   map[string]any{"session": uint32(7), "threadId": int64(11), "kind": "into"},
		},
		{
			name: "step out", command: "debug_step", paused: true,
			invoke: func(m *Model) { m.StepOut() },
			args:   map[string]any{"session": uint32(7), "threadId": int64(11), "kind": "out"},
		},
		{
			name: "force step over", command: "debug_force_step_over", paused: true,
			invoke: func(m *Model) { m.ForceStepOver() },
			args:   map[string]any{"session": uint32(7), "threadId": int64(11)},
		},
		{
			name: "force step into", command: "debug_step", paused: true,
			invoke: func(m *Model) { m.ForceStepInto() },
			args:   map[string]any{"session": uint32(7), "threadId": int64(11), "kind": "into"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			m := &Model{
				ctx:      context.Background(),
				postHook: func(fn func()) { fn() },
				invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
					if command != test.command {
						return nil, fmt.Errorf("command = %s, want %s", command, test.command)
					}
					requests <- input.(map[string]any)
					return json.RawMessage(`null`), nil
				},
			}
			m.debug.paused = test.paused
			m.debug.stopThread = 11
			setTestDebugCapabilities(m, 7, openidea.DebugCapabilities{})
			m.debug.stopThread = 11
			m.debug.paused = test.paused
			test.invoke(m)
			select {
			case args := <-requests:
				for key, want := range test.args {
					if args[key] != want {
						t.Fatalf("%s = %#v, want %#v", key, args[key], want)
					}
				}
			case <-time.After(time.Second):
				t.Fatalf("%s did not reach the backend", test.name)
			}
		})
	}
}

func TestDebugForceRunToCursorUsesTheCaretLine(t *testing.T) {
	requests := make(chan map[string]any, 1)
	m := gotoLineTestModel("class Example {\n  void run() {}\n}\n", 18)
	m.ctx = context.Background()
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command != "debug_run_to" {
			return nil, fmt.Errorf("command = %s", command)
		}
		requests <- input.(map[string]any)
		return json.RawMessage(`null`), nil
	}
	m.debug.paused = true
	m.debug.stopThread = 9
	setTestDebugCapabilities(m, 3, openidea.DebugCapabilities{})
	m.debug.stopThread = 9
	m.debug.paused = true
	m.ForceRunToCursor()
	select {
	case args := <-requests:
		if args["session"] != uint32(3) || args["threadId"] != int64(9) ||
			args["path"] != "/test.java" || args["line"] != 1 || args["force"] != true {
			t.Fatalf("Force Run to Cursor arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Force Run to Cursor did not reach the backend")
	}
}

func TestDebuggerSourceActionsRevealFramesAndExecutionPoints(t *testing.T) {
	t.Run("selected frame", func(t *testing.T) {
		m := gotoLineTestModel("class Example {\n  void run() {}\n}\n", 0)
		path := "/test.java"
		m.debug.paused = true
		m.debug.frames = []openidea.StackFrame{{
			ID: 3, Name: "run", Path: &path, Line: 1, Column: 4,
		}}
		m.DebugJumpToSource()
		if m.tab() == nil || m.tab().state.CursorPos().Line != 1 ||
			m.tab().state.CursorPos().Col != 4 {
			t.Fatalf("Jump to Source caret = %+v", m.tab().state.CursorPos())
		}
	})
	t.Run("execution point fallback", func(t *testing.T) {
		m := gotoLineTestModel("class Example {\n  void run() {}\n}\n", 0)
		path := "/test.java"
		m.debug.paused = true
		m.debug.frames = []openidea.StackFrame{{ID: 3, Name: "run"}}
		m.debug.stopFile, m.debug.stopLine = path, 2
		m.DebugJumpToSource()
		if m.tab() == nil || m.tab().state.CursorPos().Line != 2 {
			t.Fatalf("execution-point fallback caret = %+v", m.tab().state.CursorPos())
		}
		m.tab().state.GoTo(editor.Pos{})
		m.ShowExecutionPoint()
		if m.tab() == nil || m.tab().state.CursorPos().Line != 2 {
			t.Fatalf("Show Execution Point caret = %+v", m.tab().state.CursorPos())
		}
	})
}

func TestDebugQuickEvaluateUsesSelectionAndAddsAWatch(t *testing.T) {
	requests := make(chan map[string]any, 1)
	applied := make(chan struct{}, 1)
	m := gotoLineTestModel("count + 1", 0)
	m.tab().state.Select(0, 5)
	m.ctx = context.Background()
	m.postHook = func(fn func()) {
		fn()
		applied <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command != "debug_evaluate" {
			return nil, fmt.Errorf("command = %s", command)
		}
		requests <- input.(map[string]any)
		return json.RawMessage(`{"name":"count","value":"4","typeName":"int","variablesReference":0}`), nil
	}
	m.debug.paused = true
	m.debug.frameID = 61
	setTestDebugCapabilities(m, 8, openidea.DebugCapabilities{})
	m.debug.paused = true
	m.debug.frameID = 61
	m.QuickEvaluate()
	select {
	case args := <-requests:
		if args["session"] != uint32(8) || args["frameId"] != int64(61) ||
			args["expression"] != "count" {
			t.Fatalf("Quick Evaluate arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Quick Evaluate did not reach the backend")
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("Quick Evaluate did not apply its value")
	}
	if len(m.debug.watches) != 1 || m.debug.watches[0].Expr != "count" ||
		m.debug.watches[0].Value != "4" {
		t.Fatalf("Quick Evaluate watches = %+v", m.debug.watches)
	}
}

func TestDebugConsoleHistoryRecallsAnExpression(t *testing.T) {
	m := &Model{}
	m.debug.console.history = []string{"count + 1", `System.getProperty("java.version")`}
	m.DebugBrowseConsoleHistory()
	if !m.debug.console.historyOpen {
		t.Fatal("Browse Console History did not open its native dialog")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.renderOverlays(c, c.Theme())
	}, 900, 600)
	tester.Frame()
	if !tester.HasText("Debug Console History") || !tester.HasText("count + 1") {
		t.Fatalf("history dialog content = %q", tester.Texts())
	}
	if err := tester.Click("count + 1"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if m.debug.console.historyOpen || m.debug.console.expr != "count + 1" ||
		m.debug.view != debugViewConsole {
		t.Fatalf("history selection = %+v view=%v", m.debug.console, m.debug.view)
	}
}

func TestDebugCloseMemoryInstancesClearsThePaneWithoutALiveSession(t *testing.T) {
	m := &Model{}
	m.debug.memory.inspection = &debugMemoryInspection{
		session: 4, className: "com.example.Holder",
	}
	m.DebugCloseMemoryInstances()
	if m.debug.memory.inspection != nil {
		t.Fatalf("Close Instances left the pane open: %+v", m.debug.memory.inspection)
	}
}

func TestDebugBreakpointMuteAndRemoveAllKeepBackendStateConsistent(t *testing.T) {
	requests := make(chan []openidea.Breakpoint, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "debug_set_breakpoints":
				requests <- input.(map[string]any)["breakpoints"].([]openidea.Breakpoint)
			case "debug_set_function_breakpoints", "debug_set_data_breakpoints":
				requests <- []openidea.Breakpoint{}
			}
			return json.RawMessage(`[]`), nil
		},
	}
	path := "/project/src/Example.java"
	m.debug.bps = map[string][]int{path: {6}}
	m.debug.setBreakpointOptions(path, 6, debugBreakpointOptions{
		Enabled: true, Condition: "count > 0",
	})
	m.ToggleBreakpointsMuted()
	select {
	case list := <-requests:
		if len(list) != 1 || list[0].Enabled ||
			debugStringValue(list[0].Condition) != "count > 0" {
			t.Fatalf("muted backend breakpoint = %+v", list)
		}
	case <-time.After(time.Second):
		t.Fatal("Mute Breakpoints did not republish the file")
	}
	m.RemoveAllBreakpoints()
	if len(m.debug.bps) != 0 || len(m.debug.breakpointOptions) != 0 ||
		len(m.debug.methodBps) != 0 || len(m.debug.fieldBps) != 0 {
		t.Fatalf("Remove All Breakpoints left native state: %+v", m.debug)
	}
	for range 3 {
		select {
		case list := <-requests:
			if len(list) != 0 {
				t.Fatalf("Remove All Breakpoints retained %+v", list)
			}
		case <-time.After(time.Second):
			t.Fatal("Remove All Breakpoints did not clear every backend list")
		}
	}
}

func TestDebugBreakpointToggleActionsPublishLineMethodFieldAndTemporary(t *testing.T) {
	type backendCall struct {
		command string
		input   map[string]any
	}
	requests := make(chan backendCall, 8)
	text := `package demo.sample;

public class Example {
    private int count;

    public String greet(String name) {
        return "Hello " + name;
    }
}
`
	m := gotoLineTestModel(text, 0)
	m.ctx = context.Background()
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		requests <- backendCall{command: command, input: input.(map[string]any)}
		return json.RawMessage(`[]`), nil
	}
	next := func() backendCall {
		t.Helper()
		select {
		case call := <-requests:
			return call
		case <-time.After(time.Second):
			t.Fatal("breakpoint action did not reach the backend")
			return backendCall{}
		}
	}

	m.tab().state.GoTo(editor.Pos{Line: 6})
	m.ToggleBreakpoint()
	call := next()
	if call.command != "debug_set_breakpoints" {
		t.Fatalf("line breakpoint command = %s", call.command)
	}
	lines := call.input["breakpoints"].([]openidea.Breakpoint)
	if len(lines) != 1 || lines[0].Line != 6 || !lines[0].Enabled {
		t.Fatalf("line breakpoint payload = %+v", lines)
	}

	m.tab().state.GoTo(editor.Pos{Line: 5})
	m.ToggleMethodBreakpoint()
	call = next()
	if call.command != "debug_set_function_breakpoints" {
		t.Fatalf("method breakpoint command = %s", call.command)
	}
	methods := call.input["breakpoints"].([]openidea.FunctionBreakpoint)
	if len(methods) != 1 || methods[0].Name != "demo.sample.Example#greet" ||
		!methods[0].Enabled {
		t.Fatalf("method breakpoint payload = %+v", methods)
	}
	call = next()
	if call.command != "debug_set_data_breakpoints" {
		t.Fatalf("method refresh data command = %s", call.command)
	}

	m.tab().state.GoTo(editor.Pos{Line: 3})
	m.ToggleFieldWatchpoint()
	call = next()
	if call.command != "debug_set_function_breakpoints" {
		t.Fatalf("field refresh function command = %s", call.command)
	}
	call = next()
	if call.command != "debug_set_data_breakpoints" {
		t.Fatalf("field watchpoint command = %s", call.command)
	}
	fields := call.input["breakpoints"].([]openidea.DataBreakpoint)
	if len(fields) != 1 || fields[0].DataID != "demo.sample.Example#count" ||
		!fields[0].Enabled {
		t.Fatalf("field watchpoint payload = %+v", fields)
	}

	m.tab().state.GoTo(editor.Pos{Line: 7})
	m.ToggleTemporaryBreakpoint()
	call = next()
	if call.command != "debug_set_breakpoints" {
		t.Fatalf("temporary breakpoint command = %s", call.command)
	}
	lines = call.input["breakpoints"].([]openidea.Breakpoint)
	if len(lines) != 2 || !containsInt(m.debug.temporaryBps["/test.java"], 7) {
		t.Fatalf("temporary breakpoint payload = %+v, temporary=%+v", lines, m.debug.temporaryBps)
	}
}

func TestDebugBreakpointEditorPublishesConditionsAndLogMessage(t *testing.T) {
	requests := make(chan []openidea.Breakpoint, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_set_breakpoints" {
				return nil, nil
			}
			requests <- input.(map[string]any)["breakpoints"].([]openidea.Breakpoint)
			return json.RawMessage(`[]`), nil
		},
	}
	path := "/project/src/Example.java"
	m.debug.bps = map[string][]int{path: {4}}
	m.editDebugLineBreakpoint(path, 4)
	m.debug.breakpointEdit.condition = " count > 2 "
	m.debug.breakpointEdit.hitCondition = " >= 3 "
	m.debug.breakpointEdit.logMessage = " count={count} "
	m.saveDebugBreakpointEditor()

	select {
	case list := <-requests:
		if len(list) != 1 {
			t.Fatalf("published breakpoints = %+v", list)
		}
		bp := list[0]
		if bp.Line != 4 || !bp.Enabled ||
			debugStringValue(bp.Condition) != "count > 2" ||
			debugStringValue(bp.HitCondition) != ">= 3" ||
			debugStringValue(bp.LogMessage) != "count={count}" {
			t.Fatalf("conditioned breakpoint = %+v", bp)
		}
	case <-time.After(time.Second):
		t.Fatal("the edited breakpoint was not published")
	}
	options := m.debug.breakpointOptionsAt(path, 4)
	if options.Condition != "count > 2" || options.HitCondition != ">= 3" ||
		options.LogMessage != "count={count}" {
		t.Fatalf("stored breakpoint options = %+v", options)
	}
}

func TestDebugBreakpointDialogShowsEditableAdvancedState(t *testing.T) {
	m := &Model{}
	path := "/project/src/Example.java"
	m.debug.bps = map[string][]int{path: {4}}
	m.debug.setBreakpointOptions(path, 4, debugBreakpointOptions{
		Enabled: true, Condition: "count > 2", LogMessage: "count={count}",
	})
	m.debug.methodBps = []openidea.FunctionBreakpoint{{
		Name: "demo.Example#run", Enabled: false,
	}}
	m.debug.fieldBps = []openidea.DataBreakpoint{{
		DataID: "demo.Example#count", Enabled: true,
		HitCondition: debugStringPointer("3"),
	}}
	m.debug.breakpointsOpen = true
	tester := ui.NewTester(func(c *ui.Context) {
		m.renderOverlays(c, c.Theme())
	}, 1000, 700)
	tester.Frame()
	for _, text := range []string{"Breakpoints", "Conditional", "Log", "Method · demo.Example#run",
		"Disabled", "Field · demo.Example#count", "Hit count", "Edit"} {
		if !tester.HasText(text) {
			t.Fatalf("Breakpoints dialog is missing %q: %q", text, tester.Texts())
		}
	}
}

func debugStringPointer(value string) *string {
	return &value
}

func TestDebuggerOpenIDEACommandsAreRegistered(t *testing.T) {
	ids := []string{
		"debug.attachToProcess",
		"debug.browseConsoleHistory",
		"debug.cancelHotSwap",
		"debug.evaluate",
		"debug.forceRunToCursor",
		"debug.forceStepInto",
		"debug.forceStepOver",
		"debug.heap.capture",
		"debug.heap.open",
		"debug.heap.saveDump",
		"debug.hotReload",
		"debug.hotRestart",
		"debug.jumpToSource",
		"debug.memory.closeInstances",
		"debug.memory.jumpToSource",
		"debug.memory.refresh",
		"debug.memory.showInstances",
		"debug.memory.showReferrers",
		"debug.muteBreakpoints",
		"debug.pause",
		"debug.pluginStart",
		"debug.quickEvaluate",
		"debug.removeAllBreakpoints",
		"debug.rerun",
		"debug.resetFrame",
		"debug.resume",
		"debug.runToCursor",
		"debug.setValue",
		"debug.showExecutionPoint",
		"debug.showToolWindow",
		"debug.smartStepInto",
		"debug.startConfig",
		"debug.stepInto",
		"debug.stepOut",
		"debug.stepOver",
		"debug.stop",
		"debug.toggleFieldWatchpoint",
		"debug.toggleMethodBreakpoint",
		"debug.toggleTemporaryBreakpoint",
		"debug.toggleBreakpoint",
		"debug.viewBreakpoints",
	}
	for _, id := range ids {
		if _, ok := Command(id); !ok {
			t.Errorf("%s is not registered", id)
		}
	}
	if hotReload, ok := actionGet("debug.hotReload"); !ok || hotReload.ID != "debug.hotReload" {
		t.Fatalf("debug.hotReload resolved to %+v, want its Dart action", hotReload)
	}
}
