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

func TestDebugSessionsKeepIndependentStateAndOutput(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.debug = DebugPane{
		active:       11,
		session:      11,
		config:       "first.Application",
		tabName:      "First",
		view:         debugViewConsole,
		status:       "Running…",
		output:       []outputLine{{kind: "stdout", text: "first only"}},
		stopLine:     -1,
		sessions:     map[uint32]*debugSessionState{},
		sessionOrder: []uint32{11, 22},
	}
	m.debug.sessions[22] = &debugSessionState{
		session:  22,
		config:   "second.Application",
		tabName:  "Second",
		view:     debugViewVariables,
		status:   "Paused",
		paused:   true,
		output:   []outputLine{{kind: "stdout", text: "second only"}},
		stopLine: 7,
	}

	m.selectDebugSession(22)
	if m.debug.active != 22 || m.debug.tabName != "Second" ||
		m.debug.text() != "second only" || !m.debug.paused {
		t.Fatalf("second session was not restored: %+v", m.debug)
	}
	m.selectDebugSession(11)
	if m.debug.active != 11 || m.debug.tabName != "First" ||
		m.debug.text() != "first only" || m.debug.paused {
		t.Fatalf("first session was not restored: %+v", m.debug)
	}

	before := m.debug.text()
	m.acceptDebugEvent(22, json.RawMessage(`{"kind":"output","category":"stdout","output":"background\n"}`))
	if m.debug.text() != before {
		t.Fatalf("background output leaked into the active console: %q", m.debug.text())
	}
	if got := m.debug.sessions[22]; got == nil || len(got.output) != 2 ||
		got.output[1].text != "background" {
		t.Fatalf("background session output = %+v", got)
	}
}

func TestDebugTerminationKeepsItsOwnContentTab(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.debug = DebugPane{
		active:       31,
		session:      31,
		config:       "demo.Application",
		tabName:      "Demo",
		status:       "Running…",
		stopLine:     -1,
		sessions:     map[uint32]*debugSessionState{},
		sessionOrder: []uint32{31},
	}
	m.acceptDebugEvent(31, json.RawMessage(`{"kind":"exited","code":0}`))
	if m.debug.active != 31 || m.debug.session != 0 || !m.debug.ended {
		t.Fatalf("terminated tab = %+v", m.debug)
	}
	summaries := m.debug.debugSessionSummaries()
	if len(summaries) != 1 || summaries[0].id != 31 || summaries[0].live {
		t.Fatalf("terminated summaries = %+v", summaries)
	}
}

func TestDebugTerminationReleasesHeapRequestAndChannel(t *testing.T) {
	for _, tc := range []struct {
		name       string
		active     uint32
		owner      uint32
		background bool
	}{
		{name: "active", active: 11, owner: 11},
		{name: "background", active: 21, owner: 22, background: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const (
				request = uint32(901)
				channel = uint32(502)
			)
			cancelled := make(chan uint32, 1)
			m := &Model{ctx: context.Background()}
			m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
				if command != "cancel_request" {
					return nil, fmt.Errorf("unexpected command %s", command)
				}
				cancelled <- input.(map[string]any)["id"].(uint32)
				return json.RawMessage(`null`), nil
			}
			m.debug = DebugPane{
				active:       tc.active,
				session:      tc.active,
				tabName:      "First",
				stopLine:     -1,
				sessions:     map[uint32]*debugSessionState{},
				sessionOrder: []uint32{tc.active},
				heapChannels: map[uint32]uint32{channel: tc.owner},
			}
			if tc.background {
				m.debug.sessions[tc.owner] = &debugSessionState{
					session:  tc.owner,
					tabName:  "Second",
					stopLine: -1,
					heap: debugHeapState{
						busy: true, request: request, channel: channel, progress: "Analyzing…",
					},
				}
				m.debug.sessionOrder = append(m.debug.sessionOrder, tc.owner)
			} else {
				m.debug.heap = debugHeapState{
					busy: true, request: request, channel: channel, progress: "Analyzing…",
				}
			}

			m.acceptDebugEvent(tc.owner, json.RawMessage(`{"kind":"terminated"}`))

			state := m.debugSessionState(tc.owner)
			if state == nil || state.heap.busy || state.heap.request != 0 ||
				state.heap.channel != 0 || state.heap.progress != "" {
				t.Fatalf("heap state after termination = %+v", state)
			}
			if len(m.debug.heapChannels) != 0 {
				t.Fatalf("heap channels survived termination: %+v", m.debug.heapChannels)
			}
			if m.acceptDebugHeapProgress(channel, json.RawMessage(`{"type":"analyzing"}`)) {
				t.Fatal("a late heap progress chunk revived the terminated session")
			}
			select {
			case got := <-cancelled:
				if got != request {
					t.Fatalf("cancel_request id = %d, want %d", got, request)
				}
			case <-time.After(time.Second):
				t.Fatal("termination did not cancel the in-flight heap request")
			}
		})
	}
}

func TestConcurrentDebugSessionsAreAllowed(t *testing.T) {
	m := &Model{run: RunPane{trusted: true}}
	m.debug.session = 44
	m.debug.active = 44
	if !m.canStartDebugWithWindow("/project", false) {
		t.Fatal("a live session still blocked a second debug launch")
	}
	m.debug.starting = true
	if m.canStartDebugWithWindow("/project", false) {
		t.Fatal("two preparations overlap in the native debug panel")
	}
}

func TestDebugSessionTabsRenderEveryNativeContent(t *testing.T) {
	m := &Model{
		ctx: context.Background(),
		debug: DebugPane{
			visible:      true,
			active:       51,
			session:      51,
			config:       "first.Application",
			tabName:      "First",
			status:       "Running…",
			stopLine:     -1,
			sessions:     map[uint32]*debugSessionState{},
			sessionOrder: []uint32{51, 52},
		},
	}
	m.debug.sessions[52] = &debugSessionState{
		session:  52,
		config:   "second.Application",
		tabName:  "Second",
		status:   "Paused",
		paused:   true,
		stopLine: -1,
	}
	tester := ui.NewTester(func(c *ui.Context) { m.debugPanel(c, c.Theme()) }, 1000, 520)
	tester.Frame()
	for _, label := range []string{"First", "Second"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("debug tab %q did not render: %q", label, tester.Texts())
		}
	}
}

func TestBackgroundBuildCompleteOffersHotSwapToItsSession(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.debug = DebugPane{
		active:       51,
		session:      51,
		tabName:      "First",
		stopLine:     -1,
		sessions:     map[uint32]*debugSessionState{},
		sessionOrder: []uint32{51, 52},
	}
	m.debug.sessions[52] = &debugSessionState{
		session:   52,
		tabName:   "Second",
		stopLine:  -1,
		capsKnown: true,
		caps:      openidea.DebugCapabilities{HotCodeReplace: true},
	}

	m.acceptDebugEvent(52, json.RawMessage(`{"kind":"hotCodeReplace","changeType":"BUILD_COMPLETE"}`))

	if len(m.notify.items) != 1 {
		t.Fatalf("background BUILD_COMPLETE did not offer HotSwap: %+v", m.notify.items)
	}
	item := m.notify.items[0]
	if item.key != "debug.hotswap" || len(item.actions) != 1 ||
		item.actions[0].label != "Reload Changed Classes" {
		t.Fatalf("background HotSwap notification = %+v", item)
	}
}

func TestClosingBackgroundDebugSessionStopsItBeforeRemoval(t *testing.T) {
	stopped := make(chan uint32, 1)
	removed := make(chan struct{}, 1)
	m := &Model{ctx: context.Background()}
	m.postHook = func(fn func()) {
		fn()
		select {
		case removed <- struct{}{}:
		default:
		}
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "debug_stop" {
			t.Fatalf("unexpected command %q", command)
		}
		stopped <- args.(map[string]any)["session"].(uint32)
		return []byte("null"), nil
	}
	m.debug = DebugPane{
		active:       61,
		session:      61,
		tabName:      "Keep",
		stopLine:     -1,
		sessions:     map[uint32]*debugSessionState{},
		sessionOrder: []uint32{61, 62},
	}
	m.debug.sessions[62] = &debugSessionState{
		session:  62,
		tabName:  "Close me",
		status:   "Running…",
		stopLine: -1,
	}

	m.closeDebugSessionTab(62)
	select {
	case id := <-stopped:
		if id != 62 {
			t.Fatalf("debug_stop session = %d, want 62", id)
		}
	case <-time.After(time.Second):
		t.Fatal("closing a background session did not stop its backend")
	}
	select {
	case <-removed:
	case <-time.After(time.Second):
		t.Fatal("the stopped background session was not removed")
	}
	if m.debug.sessions[62] != nil || m.debug.active != 61 ||
		m.debug.tabName != "Keep" {
		t.Fatalf("background close changed the active tab: %+v", m.debug)
	}
}
