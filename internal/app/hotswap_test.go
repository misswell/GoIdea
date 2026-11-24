package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func hotSwapTestModel(root string, sessions ...[2]uint32) *Model {
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.debug = DebugPane{
		active: sessions[0][0], session: sessions[0][0],
		tabName: "First", stopLine: -1, adapter: "java-debug",
		capsKnown: true, caps: openidea.DebugCapabilities{HotCodeReplace: true},
		launchCwd: root,
		sessions:  map[uint32]*debugSessionState{}, sessionOrder: []uint32{},
	}
	for i, ids := range sessions {
		state := &debugSessionState{
			session: ids[0], tabName: "Debug", stopLine: -1,
			capsKnown: true, caps: openidea.DebugCapabilities{HotCodeReplace: true},
			adapter: "java-debug", launchCwd: root,
		}
		if i == 0 {
			state.tabName = "First"
		} else {
			state.tabName = "Second"
		}
		m.debug.sessions[ids[0]] = state
		m.debug.ensureDebugSessionOrder(ids[0])
	}
	m.debug.rememberDebugSession()
	return m
}

func TestHotSwapReloadsEveryCapturedJavaSession(t *testing.T) {
	root := t.TempDir()
	m := hotSwapTestModel(root, [2]uint32{11, 0}, [2]uint32{22, 0})
	var mu sync.Mutex
	var modules []string
	var sessions []uint32
	finished := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		if !m.debug.reloading && m.debug.hotSwapStage != "called" {
			select {
			case finished <- struct{}{}:
			default:
			}
		}
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "java_build_workspace":
			mu.Lock()
			modules = append(modules, input.(map[string]any)["module"].(string))
			mu.Unlock()
			return json.RawMessage(`"succeeded"`), nil
		case "debug_redefine_classes":
			session := input.(map[string]any)["session"].(uint32)
			mu.Lock()
			sessions = append(sessions, session)
			mu.Unlock()
			return json.RawMessage(`{"changedClasses":["demo.Example"]}`), nil
		default:
			return nil, nil
		}
	}
	m.HotSwapReload()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("HotSwap did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(modules) != 1 || modules[0] != root {
		t.Fatalf("HotSwap modules = %#v", modules)
	}
	if len(sessions) != 2 || sessions[0]+sessions[1] != 33 {
		t.Fatalf("HotSwap sessions = %#v", sessions)
	}
}

func TestHotSwapForConfigOnlyBuildsAndReloadsThatConfiguration(t *testing.T) {
	root := t.TempDir()
	m := hotSwapTestModel(root, [2]uint32{51, 0}, [2]uint32{52, 0})
	m.debug.sessions[51].lastID = "boot"
	m.debug.sessions[52].lastID = "service"
	var mu sync.Mutex
	var modules []string
	var sessions []uint32
	finished := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		if !m.debug.reloading && m.debug.hotSwapStage != "called" {
			select {
			case finished <- struct{}{}:
			default:
			}
		}
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "java_build_workspace":
			mu.Lock()
			modules = append(modules, input.(map[string]any)["module"].(string))
			mu.Unlock()
			return json.RawMessage(`"succeeded"`), nil
		case "debug_redefine_classes":
			mu.Lock()
			sessions = append(sessions, input.(map[string]any)["session"].(uint32))
			mu.Unlock()
			return json.RawMessage(`{"changedClasses":["demo.Service"]}`), nil
		}
		return nil, nil
	}
	if !m.HotSwapReloadForConfig("service") {
		t.Fatal("scoped HotSwap reported failure")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("scoped HotSwap did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(modules) != 1 || modules[0] != root {
		t.Fatalf("scoped modules = %#v", modules)
	}
	if len(sessions) != 1 || sessions[0] != 52 {
		t.Fatalf("scoped sessions = %#v", sessions)
	}
}

func TestHotSwapForUnknownConfigReportsNoSession(t *testing.T) {
	m := hotSwapTestModel(t.TempDir(), [2]uint32{61, 0})
	m.postHook = func(fn func()) { fn() }
	m.debug.sessions[61].lastID = "boot"
	if m.HotSwapReloadForConfig("missing") {
		t.Fatal("unknown configuration reported a successful HotSwap")
	}
	if m.debug.err == "" {
		t.Fatal("unknown configuration did not report why HotSwap was unavailable")
	}
}

func TestHotSwapAfterExternalBuildDoesNotCompileAgain(t *testing.T) {
	root := t.TempDir()
	m := hotSwapTestModel(root, [2]uint32{21, 0})
	redefined := make(chan uint32, 1)
	finished := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		if !m.debug.reloading && m.debug.hotSwapStage != "reloading" {
			select {
			case finished <- struct{}{}:
			default:
			}
		}
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "java_build_workspace":
			t.Fatal("an external BUILD_COMPLETE was compiled a second time")
			return nil, nil
		case "debug_redefine_classes":
			redefined <- input.(map[string]any)["session"].(uint32)
			return json.RawMessage(`{"changedClasses":["demo.Example"]}`), nil
		default:
			return nil, nil
		}
	}
	m.HotSwapReloadAfterBuild()
	select {
	case id := <-redefined:
		if id != 21 {
			t.Fatalf("redefined session = %d, want 21", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("external build HotSwap did not reload the session")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("external build HotSwap did not finish")
	}
}

func TestCancelledHotSwapMarksEverySessionForRestart(t *testing.T) {
	root := t.TempDir()
	m := hotSwapTestModel(root, [2]uint32{31, 0}, [2]uint32{32, 0})
	buildStarted := make(chan uint32, 1)
	cancelled := make(chan struct{})
	finished := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		if !m.debug.reloading && m.debug.hotSwapStage != "called" {
			select {
			case finished <- struct{}{}:
			default:
			}
		}
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "java_build_workspace":
			buildStarted <- input.(map[string]any)["request"].(uint32)
			<-cancelled
			return json.RawMessage(`"cancelled"`), nil
		case "cancel_request":
			select {
			case <-cancelled:
			default:
				close(cancelled)
			}
			return json.RawMessage(`null`), nil
		default:
			return nil, nil
		}
	}
	m.HotSwapReload()
	select {
	case request := <-buildStarted:
		if request == 0 {
			t.Fatal("HotSwap build request is zero")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HotSwap build did not start")
	}
	m.CancelHotSwap()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled HotSwap did not finish")
	}
	for _, id := range []uint32{31, 32} {
		if state := m.debug.sessions[id]; state == nil || !state.hotSwapNeedsRestart {
			t.Fatalf("session %d was not marked for restart: %+v", id, state)
		}
	}
}

func TestCancelledScopedHotSwapMarksOnlyItsSession(t *testing.T) {
	root := t.TempDir()
	main := &serialMainThread{}
	m := hotSwapTestModel(root, [2]uint32{33, 0}, [2]uint32{34, 0})
	m.postHook = main.post
	m.debug.sessions[33].lastID = "boot"
	m.debug.sessions[34].lastID = "service"
	buildStarted := make(chan struct{}, 1)
	cancelled := make(chan struct{})
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "java_build_workspace":
			buildStarted <- struct{}{}
			<-cancelled
			return json.RawMessage(`"cancelled"`), nil
		case "cancel_request":
			select {
			case <-cancelled:
			default:
				close(cancelled)
			}
			return json.RawMessage(`null`), nil
		}
		return json.RawMessage(`null`), nil
	}
	result := make(chan bool, 1)
	go func() {
		result <- m.HotSwapReloadForConfig("service")
	}()
	select {
	case <-buildStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("scoped HotSwap build did not start")
	}
	main.do(m.CancelHotSwap)
	select {
	case success := <-result:
		if success {
			t.Fatal("cancelled scoped HotSwap reported success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled scoped HotSwap did not finish")
	}
	main.do(func() {
		if state := m.debug.sessions[33]; state == nil || state.hotSwapNeedsRestart {
			t.Fatalf("unselected session was marked for restart: %+v", state)
		}
		if state := m.debug.sessions[34]; state == nil || !state.hotSwapNeedsRestart {
			t.Fatalf("selected session was not marked for restart: %+v", state)
		}
	})
}

func TestCancelHotSwapIsIdempotentAfterSignalClosed(t *testing.T) {
	cancelled := make(chan struct{})
	close(cancelled)
	called := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			called <- struct{}{}
			return json.RawMessage(`null`), nil
		},
	}
	m.debug.reloading = true
	m.debug.hotSwapRequest = 91
	m.debug.hotSwapCancel = cancelled

	m.CancelHotSwap()
	select {
	case <-called:
		t.Fatal("a closed Hot Swap cancel signal issued a second request")
	default:
	}
	if m.debug.hotSwapCancel != nil {
		t.Fatal("closed Hot Swap cancel signal was retained")
	}
}

func TestHotSwapBuildCompleteIsQuietDuringAndAfterReload(t *testing.T) {
	root := t.TempDir()
	m := hotSwapTestModel(root, [2]uint32{41, 0})

	m.debug.reloading = true
	m.offerHotSwapAfterBuild(41)
	if len(m.notify.items) != 0 {
		t.Fatalf("HotSwap prompted during its own build: %+v", m.notify.items)
	}

	m.debug.reloading = false
	m.debug.hotSwapQuietUntil = time.Now().Add(time.Second)
	m.offerHotSwapAfterBuild(41)
	if len(m.notify.items) != 0 {
		t.Fatalf("HotSwap prompted immediately after its own build: %+v", m.notify.items)
	}

	m.debug.hotSwapQuietUntil = time.Now().Add(-time.Second)
	m.offerHotSwapAfterBuild(41)
	if len(m.notify.items) != 1 {
		t.Fatalf("HotSwap did not offer an external build: %+v", m.notify.items)
	}
}
