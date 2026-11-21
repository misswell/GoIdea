package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

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
	requests := make(chan map[string]any, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_step_in_targets" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(`[{"id":44,"label":"service.call()","line":9,"column":4,"endColumn":16}]`), nil
		},
	}
	m.debug.paused = true
	m.debug.session, m.debug.stopThread = 5, 6
	m.debug.frames = []openidea.StackFrame{{ID: 88, Name: "run"}}
	m.DebugSmartStepInto()
	select {
	case args := <-requests:
		if args["session"] != uint32(5) || args["frameId"] != int64(88) {
			t.Fatalf("Smart Step arguments = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Smart Step did not ask the adapter for targets")
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
	m.debug.session = 14
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

func TestDebuggerOpenIDEACommandsAreRegistered(t *testing.T) {
	ids := []string{
		"debug.browseConsoleHistory",
		"debug.cancelHotSwap",
		"debug.heap.capture",
		"debug.heap.open",
		"debug.heap.saveDump",
		"debug.hotRestart",
		"debug.jumpToSource",
		"debug.memory.closeInstances",
		"debug.memory.jumpToSource",
		"debug.memory.refresh",
		"debug.memory.showInstances",
		"debug.memory.showReferrers",
		"debug.pluginStart",
		"debug.resetFrame",
		"debug.setValue",
		"debug.smartStepInto",
		"debug.stop",
		"debug.toggleFieldWatchpoint",
		"debug.toggleMethodBreakpoint",
		"debug.toggleTemporaryBreakpoint",
		"debug.viewBreakpoints",
	}
	for _, id := range ids {
		if _, ok := Command(id); !ok {
			t.Errorf("%s is not registered", id)
		}
	}
}
