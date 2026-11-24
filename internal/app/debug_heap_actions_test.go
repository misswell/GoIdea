package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDebugOpenHeapDumpAnalyzesTheChosenFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	posts := make(chan func(), 8)
	requests := make(chan map[string]any, 1)
	m := &Model{
		ctx: ctx,
		postHook: func(fn func()) {
			posts <- fn
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_heap_open" {
				return nil, fmt.Errorf("command = %s", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(
				`{"status":"notEnoughDiskSpace","needed":104857600,"available":1048576}`,
			), nil
		},
	}
	m.debug.heap.busy = true
	m.debug.heap.request = 10
	m.debug.heap.channel = 11
	m.registerDebugHeapChannel(0, 11)

	const path = "/tmp/example.hprof"
	m.analyzeDebugHeapDump(0, path, 10, 11)

	for !strings.Contains(m.debug.heap.err, "Not enough disk space") {
		select {
		case fn := <-posts:
			fn()
		case <-ctx.Done():
			t.Fatalf("heap analysis did not finish: %q", m.debug.heap.err)
		}
	}

	select {
	case args := <-requests:
		if args["path"] != path || args["request"] != uint32(10) {
			t.Fatalf("debug_heap_open arguments = %#v", args)
		}
		progress, _ := args["onProgress"].(map[string]any)
		if progress["$channel"] != uint32(11) {
			t.Fatalf("onProgress arguments = %#v", args["onProgress"])
		}
	case <-ctx.Done():
		t.Fatal("debug_heap_open did not reach the backend")
	}
	if m.debug.heap.busy {
		t.Fatal("heap open left the pane busy")
	}
	if _, ok := m.debug.heapChannels[11]; ok {
		t.Fatal("heap open left its progress channel registered")
	}
}

func TestDebugSaveHeapDumpUsesTheOpenSnapshot(t *testing.T) {
	requests := make(chan map[string]any, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "debug_heap_save" {
				return nil, fmt.Errorf("command = %s", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(`null`), nil
		},
	}
	m.debug.heap.info = &debugHeapSnapshot{ID: 77}
	const path = "/tmp/saved-heap.hprof"
	if err := m.saveDebugHeapDump(0, 77, path); err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-requests:
		if args["snapshot"] != uint64(77) || args["path"] != path {
			t.Fatalf("debug_heap_save arguments = %#v", args)
		}
	default:
		t.Fatal("debug_heap_save did not reach the backend")
	}
}
