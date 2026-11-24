package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func usageHighlightTestModel(
	t *testing.T,
	text string,
	caret int,
	hook func(context.Context, string, any) (json.RawMessage, error),
) (*Model, *Tab, *serialMainThread) {
	t.Helper()
	doc := docbridge.Virtual("/Demo.java", text)
	state := &editor.State{
		Buffer:     doc.Buffer(),
		LineHeight: editorLineHeight(13),
	}
	state.SetSelections(editor.Selection{Anchor: caret, Head: caret})
	tab := &Tab{doc: doc, state: state}
	main := &serialMainThread{}
	m := &Model{
		ctx:        context.Background(),
		postHook:   main.post,
		invokeHook: hook,
		tabs:       []*Tab{tab},
		active:     0,
	}
	return m, tab, main
}

func TestHighlightUsagesRequestsUTF16PositionAndConvertsRanges(t *testing.T) {
	type request struct {
		command string
		input   map[string]any
	}
	requests := make(chan request, 1)
	m, tab, main := usageHighlightTestModel(t, "😀foo foo", len("😀"),
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			requests <- request{command: command, input: input.(map[string]any)}
			return json.RawMessage(`[
				{"range":{"start":{"line":0,"col":2},"end":{"line":0,"col":5}},"kind":"read"},
				{"range":{"start":{"line":0,"col":6},"end":{"line":0,"col":9}},"kind":"write"}
			]`), nil
		})

	main.do(func() { m.ToggleHighlightUsages() })
	got := <-requests
	if got.command != "lsp_document_highlights" {
		t.Fatalf("command = %q", got.command)
	}
	if got.input["doc"] != tab.doc.ID() {
		t.Fatalf("document = %#v, want %d", got.input["doc"], tab.doc.ID())
	}
	pos, ok := got.input["pos"].(map[string]any)
	if !ok || pos["line"] != uint32(0) || pos["col"] != uint32(2) {
		t.Fatalf("position = %#v", got.input["pos"])
	}

	waitForSerialMainThread(t, main, time.Second, func() bool {
		return tab.usageHighlightSticky && !tab.usageHighlightBusy
	})
	main.do(func() {
		if len(tab.usageHighlights) != 2 {
			t.Fatalf("highlights = %+v", tab.usageHighlights)
		}
		first, second := tab.usageHighlights[0], tab.usageHighlights[1]
		if first.Start != len("😀") || first.End != len("😀foo") ||
			second.Start != len("😀foo ") || second.End != len("😀foo foo") {
			t.Fatalf("converted highlights = %+v", tab.usageHighlights)
		}
		if first.Kind != "read" || second.Kind != "write" {
			t.Fatalf("highlight kinds = %q, %q", first.Kind, second.Kind)
		}
		if got := len(tab.state.TextBackgrounds); got != 2 {
			t.Fatalf("backgrounds = %d, want 2", got)
		}
		if m.status != "2 usages of 'foo' found (press Cmd+Shift+F7 again to remove, Escape to clear)" {
			t.Fatalf("status = %q", m.status)
		}
	})
}

func TestHighlightUsagesDropsAStaleBufferReply(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	m, tab, main := usageHighlightTestModel(t, "foo", 0,
		func(context.Context, string, any) (json.RawMessage, error) {
			close(started)
			<-release
			return json.RawMessage(
				`[{"range":{"start":{"line":0,"col":0},"end":{"line":0,"col":3}},"kind":"read"}]`,
			), nil
		})

	main.do(func() { m.ToggleHighlightUsages() })
	<-started
	main.do(func() {
		tab.state.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "x"})
	})
	close(release)
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return !tab.usageHighlightBusy
	})
	main.do(func() {
		if tab.usageHighlightSticky || len(tab.usageHighlights) != 0 {
			t.Fatalf("stale reply installed %+v", tab.usageHighlights)
		}
	})
}

func TestHighlightUsagesToggleAndEscapeClearTheSession(t *testing.T) {
	var calls atomic.Int32
	m, tab, main := usageHighlightTestModel(t, "foo", 1,
		func(context.Context, string, any) (json.RawMessage, error) {
			calls.Add(1)
			return json.RawMessage(
				`[{"range":{"start":{"line":0,"col":0},"end":{"line":0,"col":3}},"kind":"read"}]`,
			), nil
		})

	main.do(func() { m.ToggleHighlightUsages() })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return tab.usageHighlightSticky && !tab.usageHighlightBusy
	})
	main.do(func() { m.ToggleHighlightUsages() })
	if calls.Load() != 1 || tab.usageHighlightSticky || len(tab.usageHighlights) != 0 {
		t.Fatalf("second toggle did not clear the session: calls=%d highlights=%+v",
			calls.Load(), tab.usageHighlights)
	}

	main.do(func() { m.ToggleHighlightUsages() })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return tab.usageHighlightSticky && !tab.usageHighlightBusy
	})
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
	main.do(func() {
		if !m.ClearStickyUsageHighlights() {
			t.Fatal("Escape did not claim the active session")
		}
		if tab.usageHighlightSticky || len(tab.usageHighlights) != 0 {
			t.Fatalf("Escape left %+v", tab.usageHighlights)
		}
	})
	if calls.Load() != 2 {
		t.Fatalf("Escape issued another request: calls = %d", calls.Load())
	}
}

func TestHighlightedUsageNavigationWraps(t *testing.T) {
	m, tab, _ := usageHighlightTestModel(t, "aa aa aa", 0, nil)
	tab.usageHighlights = []usageHighlight{
		{Start: 0, End: 2, Kind: "read"},
		{Start: 3, End: 5, Kind: "write"},
		{Start: 6, End: 8, Kind: "read"},
	}
	tab.usageHighlightVersion = tab.state.Buffer.Version()
	tab.usageHighlightSticky = true

	step := func(direction int) int {
		m.StepHighlightedUsage(direction)
		return tab.state.Cursor()
	}
	if got := step(1); got != 3 {
		t.Fatalf("next = %d, want 3", got)
	}
	if got := step(1); got != 6 {
		t.Fatalf("second next = %d, want 6", got)
	}
	if got := step(1); got != 0 {
		t.Fatalf("wrapped next = %d, want 0", got)
	}
	if got := step(-1); got != 6 {
		t.Fatalf("wrapped previous = %d, want 6", got)
	}
}

func TestNavigateDeclarationSendsExactLSPRequest(t *testing.T) {
	type request struct {
		command string
		input   map[string]any
	}
	requests := make(chan request, 1)
	m, tab, main := usageHighlightTestModel(t, "😀foo", len("😀"),
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			requests <- request{command: command, input: input.(map[string]any)}
			return json.RawMessage(`[]`), nil
		})

	main.do(func() {
		if !m.RunCommand("navigate.declaration") {
			t.Fatal("navigate.declaration is unavailable")
		}
	})
	got := <-requests
	if got.command != "lsp_goto" {
		t.Fatalf("command = %q", got.command)
	}
	if got.input["doc"] != tab.doc.ID() || got.input["kind"] != "declaration" {
		t.Fatalf("request = %#v", got.input)
	}
	pos, ok := got.input["pos"].(map[string]any)
	if !ok || pos["line"] != uint32(0) || pos["col"] != uint32(2) {
		t.Fatalf("position = %#v", got.input["pos"])
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.navDone
	})
	main.do(func() {
		if m.status != "No definition found" {
			t.Fatalf("status = %q", m.status)
		}
	})
}
