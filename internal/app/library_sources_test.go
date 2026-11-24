package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

const testLibraryClass = "jdt://contents/com.acme/A.class?=demo"

func newLibrarySourceTestModel(t *testing.T, hook func(context.Context, string, any) (json.RawMessage, error)) *Model {
	t.Helper()
	doc := docbridge.Virtual(testLibraryClass, "class A {}\n")
	m := &Model{
		ctx:        context.Background(),
		invokeHook: hook,
		postHook:   func(fn func()) { fn() },
		root:       "/project",
		tabs: []*Tab{{
			doc:   doc,
			state: &editor.State{Buffer: doc.Buffer(), ReadOnly: true},
		}},
	}
	m.run.trusted = true
	return m
}

func waitLibrarySource(t *testing.T, state *librarySourceState, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("library source state did not settle: %+v", state)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLibrarySourceActionsMatchOpenIDEAContract(t *testing.T) {
	for _, id := range []string{
		"editor.downloadSources",
		"editor.chooseSources",
		"editor.cancelSourceDownload",
	} {
		action, ok := actionGet(id)
		if !ok || action == nil {
			t.Fatalf("%s is not registered", id)
		}
	}
	if !isLibraryClassPath(testLibraryClass) {
		t.Fatal("jdt class path was not recognized")
	}
	for _, path := range []string{"/src/A.java", "jdt://contents/A.java?=demo", "jar:file:/lib.jar!/A" + ".txt"} {
		if isLibraryClassPath(path) {
			t.Fatalf("%q was treated as a class", path)
		}
	}
	if !isLibraryClassPath("jar:file:/lib.jar!/com/acme/A.class") {
		t.Fatal("archive class path was not recognized")
	}
}

func TestLibrarySourcesBannerRendersOpenIDEAStates(t *testing.T) {
	render := func(state *librarySourceState) *ui.Tester {
		t.Helper()
		m := newLibrarySourceTestModel(t, func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage("null"), nil
		})
		state.root = m.Root()
		state.checked = true
		m.ensureLibrarySources()
		m.librarySources[testLibraryClass] = state
		return ui.NewTester(func(c *ui.Context) {
			m.librarySourcesBanner(c, c.Theme(), m.tab())
		}, 1100, 120)
	}

	ready := render(&librarySourceState{canDownload: true})
	for _, text := range []string{
		"Decompiled .class file: sources are not available.",
		"Download Sources",
		"Choose Sources…",
	} {
		if !ready.HasText(text) {
			t.Fatalf("ready banner is missing %q: %q", text, ready.Texts())
		}
	}

	busy := render(&librarySourceState{busy: true})
	if !busy.HasText("Loading library sources…") || !busy.HasText("Cancel") {
		t.Fatalf("busy banner = %q", busy.Texts())
	}

	missing := render(&librarySourceState{message: "Sources not found"})
	if !missing.HasText("Sources not found") {
		t.Fatalf("missing banner = %q", missing.Texts())
	}
}

func TestLibrarySourceSupportAndDownloadPayload(t *testing.T) {
	var supportCalls atomic.Int32
	type requestCall struct {
		method string
		args   map[string]any
	}
	calls := make(chan requestCall, 2)
	m := newLibrarySourceTestModel(t, func(ctx context.Context, method string, args any) (json.RawMessage, error) {
		values, _ := args.(map[string]any)
		switch method {
		case "lsp_can_download_sources":
			supportCalls.Add(1)
			if values["uri"] != testLibraryClass {
				t.Errorf("can-download uri = %v", values["uri"])
			}
			return json.RawMessage("true"), nil
		case "lsp_library_sources":
			calls <- requestCall{method: method, args: values}
			return json.RawMessage("null"), nil
		default:
			return nil, errors.New("unexpected command " + method)
		}
	})

	state := m.ensureLibrarySourceSupport(testLibraryClass)
	waitLibrarySource(t, state, func() bool { return state.checked })
	if !state.canDownload || supportCalls.Load() != 1 {
		t.Fatalf("support = checked:%v download:%v calls:%d", state.checked, state.canDownload, supportCalls.Load())
	}
	if !m.ActionEnabled("editor.downloadSources") {
		t.Fatal("Download Sources is not enabled for the active class")
	}
	if !m.RunActionWithArg("editor.downloadSources", testLibraryClass) {
		t.Fatal("Download Sources action did not run")
	}
	select {
	case call := <-calls:
		for _, key := range []string{"request", "uri", "pos", "sources"} {
			if _, ok := call.args[key]; !ok {
				t.Fatalf("%s payload misses %q: %#v", call.method, key, call.args)
			}
		}
		sources, hasSources := call.args["sources"].(*string)
		if call.args["uri"] != testLibraryClass || !hasSources || sources != nil {
			t.Fatalf("library source payload = %#v", call.args)
		}
		pos, _ := call.args["pos"].(map[string]any)
		if pos["line"] != uint32(0) || pos["col"] != uint32(0) {
			t.Fatalf("library source position = %#v", call.args["pos"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lsp_library_sources was not called")
	}
	waitLibrarySource(t, state, func() bool { return !state.busy })
	if state.message != "Sources not found" {
		t.Fatalf("null location message = %q", state.message)
	}
}

func TestLibrarySourceChoosePayloadKeepsArchive(t *testing.T) {
	got := make(chan map[string]any, 1)
	m := newLibrarySourceTestModel(t, func(ctx context.Context, method string, args any) (json.RawMessage, error) {
		if method != "lsp_library_sources" {
			return nil, errors.New("unexpected command " + method)
		}
		got <- args.(map[string]any)
		return json.RawMessage("null"), nil
	})
	m.startLibrarySourceRequest(testLibraryClass, m.Root(), testLibraryPosition(), libraryPtr("/tmp/A-sources.jar"))
	select {
	case payload := <-got:
		sources, _ := payload["sources"].(*string)
		if sources == nil || *sources != "/tmp/A-sources.jar" {
			t.Fatalf("sources = %#v", payload["sources"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lsp_library_sources was not called")
	}
}

func TestLibrarySourceCancelSendsOpenIDEARequestID(t *testing.T) {
	cancelled := make(chan uint32, 1)
	requestSeen := make(chan uint32, 1)
	m := newLibrarySourceTestModel(t, func(ctx context.Context, method string, args any) (json.RawMessage, error) {
		values := args.(map[string]any)
		switch method {
		case "lsp_library_sources":
			request := uint32(values["request"].(uint32))
			requestSeen <- request
			<-ctx.Done()
			return nil, ctx.Err()
		case "cancel_request":
			cancelled <- uint32(values["request"].(uint32))
			return json.RawMessage("null"), nil
		default:
			return nil, errors.New("unexpected command " + method)
		}
	})
	if !m.RunActionWithArg("editor.downloadSources", testLibraryClass) {
		t.Fatal("Download Sources action did not run")
	}
	var request uint32
	select {
	case request = <-requestSeen:
	case <-time.After(3 * time.Second):
		t.Fatal("download request did not start")
	}
	if !m.RunActionWithArg("editor.cancelSourceDownload", testLibraryClass) {
		t.Fatal("Cancel action did not run")
	}
	select {
	case got := <-cancelled:
		if got != request {
			t.Fatalf("cancel_request id = %d, want %d", got, request)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel_request was not sent")
	}
}

func TestLibrarySourceProjectSwitchDropsLateReply(t *testing.T) {
	release := make(chan struct{})
	m := newLibrarySourceTestModel(t, func(ctx context.Context, method string, args any) (json.RawMessage, error) {
		<-release
		return json.RawMessage(`{"path":"/tmp/A.java","range":{"start":{"line":2,"col":3},"end":{"line":2,"col":4}}}`), nil
	})
	m.startLibrarySourceRequest(testLibraryClass, m.Root(), testLibraryPosition(), nil)
	m.root = "/another-project"
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		state := m.librarySources[testLibraryClass]
		if state != nil && !state.busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("late library source reply did not settle")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m.librarySources[testLibraryClass].message == "Sources not found" {
		t.Fatal("late reply from the old project changed the new project's state")
	}
}

func testLibraryPosition() openidea.OutlinePos {
	return openidea.OutlinePos{Line: 4, Col: 7}
}

func libraryPtr[T any](value T) *T { return &value }
