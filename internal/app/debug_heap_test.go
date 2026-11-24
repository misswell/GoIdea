package app

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestDebugHeapProgressRoutesByOwnerAndChecksOwnershipFirst(t *testing.T) {
	m := &Model{}
	m.debug.active = 2
	m.debug.session = 2
	m.debug.stopLine = -1
	m.debug.heap.channel = 202
	m.debug.sessions = map[uint32]*debugSessionState{
		1: {session: 1, stopLine: -1, heap: debugHeapState{channel: 101}},
		2: {session: 2, stopLine: -1, heap: debugHeapState{channel: 202}},
	}
	m.debug.heapChannels = map[uint32]uint32{101: 1, 202: 2}

	if m.acceptDebugHeapProgress(303, stdjson.RawMessage(`not-json`)) {
		t.Fatal("an unknown channel claimed a non-JSON output chunk")
	}
	if !m.acceptDebugHeapProgress(101, stdjson.RawMessage(
		`{"type":"dumping"}`,
	)) {
		t.Fatal("owned heap progress was not accepted")
	}
	if !m.acceptDebugHeapProgress(202, stdjson.RawMessage(
		`{"type":"analyzing","phase":"dominators","done":1,"total":2}`,
	)) {
		t.Fatal("second session's heap progress was not accepted")
	}
	if got := m.debug.sessions[1].heap.progress; got != "Dumping the JVM heap…" {
		t.Fatalf("first session heap progress = %q", got)
	}
	if got := m.debug.sessions[2].heap.progress; got != "Heap dominators · 50%" {
		t.Fatalf("second session heap progress = %q", got)
	}
}

func TestDebugHeapUnavailableExplainsSuspendedJdkAndAttachFailures(t *testing.T) {
	reason := debugHeapCaptureReason{Kind: "memory"}
	reason.Reason.Kind = "unsafeWhileSuspended"
	reason.Reason.JavaMajor = 8
	if got := debugHeapUnavailable(reason); !strings.Contains(got, "JDK 8") ||
		!strings.Contains(got, "Resume") {
		t.Fatalf("suspended JDK reason = %q", got)
	}

	reason.Reason.Kind = "attachedJvmNotFound"
	if got := debugHeapUnavailable(reason); !strings.Contains(got, "attach listener") {
		t.Fatalf("attach reason = %q", got)
	}
}

func TestDebugHeapCapturesStayWithTheirContentTabs(t *testing.T) {
	type captureCall struct {
		session uint32
		reply   chan stdjson.RawMessage
	}
	captures := make(chan captureCall, 2)
	classCalls := make(chan uint64, 2)
	posted := make(chan struct{}, 16)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(ctx context.Context, command string, input any) (stdjson.RawMessage, error) {
			switch command {
			case "debug_heap_capture":
				args := input.(map[string]any)
				reply := make(chan stdjson.RawMessage, 1)
				captures <- captureCall{session: args["session"].(uint32), reply: reply}
				select {
				case result := <-reply:
					return result, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			case "debug_heap_classes":
				classCalls <- input.(map[string]any)["snapshot"].(uint64)
				return stdjson.RawMessage(`[]`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.debug = DebugPane{
		active: 1, session: 1, tabName: "First", stopLine: -1,
		sessions: map[uint32]*debugSessionState{}, sessionOrder: []uint32{1, 2},
	}
	m.debug.sessions[2] = &debugSessionState{
		session: 2, tabName: "Second", stopLine: -1,
	}
	setTestDebugCapabilities(m, 1, openidea.DebugCapabilities{JVMHeap: true})
	m.updateDebugSession(2, func(state *debugSessionState) {
		state.caps = openidea.DebugCapabilities{JVMHeap: true}
		state.capsKnown = true
	})

	m.DebugCaptureHeap()
	first := <-captures
	m.selectDebugSession(2)
	m.DebugCaptureHeap()
	second := <-captures
	if first.session != 1 || second.session != 2 {
		t.Fatalf("capture sessions = %d, %d", first.session, second.session)
	}

	second.reply <- stdjson.RawMessage(
		`{"status":"ready","snapshot":{"id":22,"label":"Second heap","summary":{"objects":2,"classes":1}}}`,
	)
	select {
	case snapshot := <-classCalls:
		if snapshot != 22 {
			t.Fatalf("second session class snapshot = %d", snapshot)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second session heap classes were not requested")
	}
	for range 2 {
		select {
		case <-posted:
		case <-time.After(2 * time.Second):
			t.Fatal("second session heap reply did not finish")
		}
	}
	if state := m.debug.sessions[2]; state.heap.info == nil || state.heap.info.ID != 22 {
		t.Fatalf("second session heap = %+v", state.heap)
	}
	if state := m.debug.sessions[1]; state.heap.info != nil {
		t.Fatalf("second session result leaked into the first tab: %+v", state.heap)
	}

	first.reply <- stdjson.RawMessage(
		`{"status":"ready","snapshot":{"id":11,"label":"First heap","summary":{"objects":1,"classes":1}}}`,
	)
	select {
	case snapshot := <-classCalls:
		if snapshot != 11 {
			t.Fatalf("first session class snapshot = %d", snapshot)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first session heap classes were not requested")
	}
	for range 2 {
		select {
		case <-posted:
		case <-time.After(2 * time.Second):
			t.Fatal("first session heap reply did not finish")
		}
	}
	if state := m.debug.sessions[1]; state.heap.info == nil || state.heap.info.ID != 11 {
		t.Fatalf("first session heap = %+v", state.heap)
	}
}
