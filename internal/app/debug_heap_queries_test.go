package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func debugHeapQueryTestModel(
	t *testing.T,
	invoke func(context.Context, string, any) (json.RawMessage, error),
) (*Model, <-chan struct{}) {
	t.Helper()
	applied := make(chan struct{}, 16)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: invoke,
	}
	m.debug = DebugPane{
		active: 7, session: 7, tabName: "Example", stopLine: -1,
		sessions: map[uint32]*debugSessionState{}, sessionOrder: []uint32{7},
	}
	state := &debugSessionState{session: 7, tabName: "Example", stopLine: -1}
	debugHeapAdoptSnapshot(&state.heap, debugHeapSnapshot{
		ID: 20, Label: "Current", Summary: debugHeapSummary{ReachableBytes: 4096},
	})
	debugHeapAdoptSnapshot(&state.heap, debugHeapSnapshot{ID: 10, Label: "Base"})
	state.heap.selected = 20
	state.heap.syncSelectedSnapshot()
	m.debug.sessions[7] = state
	m.debug.heap = state.heap
	return m, applied
}

func waitDebugHeapApplied(t *testing.T, applied <-chan struct{}) {
	t.Helper()
	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("heap query did not publish its result")
	}
}

func TestDebugHeapObjectsPathAndCompareUsePinnedRPCs(t *testing.T) {
	type call struct {
		command string
		args    map[string]any
	}
	objectCall := make(chan call, 1)
	pathCall := make(chan call, 1)
	compareCall := make(chan call, 1)
	m, applied := debugHeapQueryTestModel(t, func(_ context.Context, command string, input any) (json.RawMessage, error) {
		args := input.(map[string]any)
		switch command {
		case "debug_heap_dominators":
			objectCall <- call{command, args}
			return json.RawMessage(`{
				"rows":[{
					"object":42,"className":"demo.Node","address":"0x2a",
					"shallow":24,"retained":2048,"hasChildren":true,"preview":"root node"
				}],
				"total":1
			}`), nil
		case "debug_heap_path":
			pathCall <- call{command, args}
			return json.RawMessage(`{
				"steps":[{
					"object":{
						"object":42,"className":"demo.Node","address":"0x2a",
						"shallow":24,"retained":2048,"hasChildren":true,"preview":null
					},
					"via":{"kind":"field","name":"root"}
				}],
				"omitted":0,"omittedAfter":0
			}`), nil
		case "debug_heap_compare":
			compareCall <- call{command, args}
			return json.RawMessage(`[{
				"name":"demo.Node","baseCount":1,"count":2,
				"baseShallow":24,"shallow":48,
				"baseRetained":1024,"retained":2048,"ambiguous":false
			}]`), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	})

	m.debugHeapShowClassObjects(3, "demo.Node")
	select {
	case got := <-objectCall:
		if got.args["snapshot"] != uint64(20) || got.args["parent"] != nil ||
			got.args["classIndex"] != uint32(3) || got.args["offset"] != uint32(0) ||
			got.args["limit"] != debugHeapPageSize {
			t.Fatalf("dominator arguments = %#v", got.args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dominator query did not reach the backend")
	}
	waitDebugHeapApplied(t, applied)
	if len(m.debug.heap.objects) != 1 || m.debug.heap.objects[0].Object != 42 ||
		m.debug.heap.objects[0].ClassName != "demo.Node" {
		t.Fatalf("dominator page = %+v", m.debug.heap.objects)
	}

	m.loadDebugHeapPath(m.debug.heap.objects[0])
	select {
	case got := <-pathCall:
		if got.args["snapshot"] != uint64(20) || got.args["object"] != uint32(42) {
			t.Fatalf("path arguments = %#v", got.args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("path query did not reach the backend")
	}
	waitDebugHeapApplied(t, applied)
	if m.debug.heap.path == nil || len(m.debug.heap.path.Steps) != 1 ||
		m.debug.heap.path.Steps[0].Object.Object != 42 {
		t.Fatalf("root path = %+v", m.debug.heap.path)
	}

	m.loadDebugHeapCompare(10)
	select {
	case got := <-compareCall:
		if got.args["base"] != uint64(10) || got.args["current"] != uint64(20) {
			t.Fatalf("compare arguments = %#v", got.args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("compare query did not reach the backend")
	}
	waitDebugHeapApplied(t, applied)
	if len(m.debug.heap.compare) != 1 || m.debug.heap.compare[0].Retained != 2048 {
		t.Fatalf("comparison = %+v", m.debug.heap.compare)
	}
}

func TestDebugHeapKeepsSnapshotsPerOwnerAndBoundsThem(t *testing.T) {
	var heap debugHeapState
	for id := uint64(1); id <= 10; id++ {
		debugHeapAdoptSnapshot(&heap, debugHeapSnapshot{ID: id, Label: fmt.Sprint(id)})
	}
	if len(heap.snapshots) != debugHeapMaxSnapshots || heap.selected != 10 ||
		heap.info == nil || heap.info.ID != 10 {
		t.Fatalf("bounded snapshots = %+v", heap.snapshots)
	}
	if heap.snapshots[0].info.ID != 3 {
		t.Fatalf("oldest retained snapshot = %d, want 3", heap.snapshots[0].info.ID)
	}
}

func TestDebugHeapQueryReplyDropsAfterSnapshotClose(t *testing.T) {
	started := make(chan struct{})
	respond := make(chan struct{})
	callbacks := make(chan func(), 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { callbacks <- fn },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "debug_heap_dominators":
				close(started)
				<-respond
				return json.RawMessage(`{
					"rows":[{"object":99,"className":"stale.Type","address":"0x63",
					"shallow":1,"retained":2,"hasChildren":false,"preview":null}],
					"total":1
				}`), nil
			case "debug_heap_close":
				return json.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.debug = DebugPane{
		active: 7, session: 7, stopLine: -1,
		sessions: map[uint32]*debugSessionState{}, sessionOrder: []uint32{7},
	}
	state := &debugSessionState{session: 7, stopLine: -1}
	debugHeapAdoptSnapshot(&state.heap, debugHeapSnapshot{ID: 20, Label: "Current"})
	m.debug.sessions[7] = state
	m.debug.heap = state.heap

	m.loadDebugHeapObjects(7, 20, 0, 0, false, 0, "Dominators")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dominator query did not start")
	}
	m.closeDebugHeap()
	close(respond)
	select {
	case callback := <-callbacks:
		callback()
	case <-time.After(time.Second):
		t.Fatal("stale dominator reply did not complete")
	}
	if m.debug.heap.info != nil || len(m.debug.heap.objects) != 0 {
		t.Fatalf("stale query survived snapshot close: %+v", m.debug.heap)
	}
}

func TestDebugHeapRendersNativeClassAndDominatorViews(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.debug = DebugPane{
		active: 7, session: 7, tabName: "Example", stopLine: -1,
		sessions: map[uint32]*debugSessionState{}, sessionOrder: []uint32{7},
	}
	state := &debugSessionState{session: 7, tabName: "Example", stopLine: -1}
	debugHeapAdoptSnapshot(&state.heap, debugHeapSnapshot{
		ID: 20, Label: "Current heap",
		Summary: debugHeapSummary{Objects: 2, ReachableBytes: 4096, Classes: 1},
	})
	state.heap.selectedSnapshot().classes = []debugHeapClass{{
		ClassIndex: 3, Name: "demo.Node", Count: 2, Shallow: 48, Retained: 2048,
	}}
	state.heap.syncSelectedSnapshot()
	m.debug.sessions[7] = state
	m.debug.heap = state.heap
	setTestDebugCapabilities(m, 7, openidea.DebugCapabilities{JVMHeap: true})
	m.debug.heap = state.heap

	tester := ui.NewTester(func(c *ui.Context) {
		m.debugHeap(c, c.Theme(), openideaColors(true))
	}, 900, 560)
	tester.Frame()
	for _, label := range []string{"Classes", "Dominators", "demo.Node", "Current heap"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("native heap view did not render %q: %q", label, tester.Texts())
		}
	}
}
