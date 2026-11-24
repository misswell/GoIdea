package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func inlineCompletionTestModel(
	t *testing.T,
	hook func(context.Context, string, any) (json.RawMessage, error),
) (*Model, *Tab, *serialMainThread) {
	t.Helper()
	doc := docbridge.Virtual("/Demo.java", "pri")
	state := &editor.State{
		Buffer:     doc.Buffer(),
		LineHeight: editorLineHeight(13),
	}
	state.SetSelections(editor.Selection{Anchor: 3, Head: 3})
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

func TestInlineCompletionRequestsExactPositionAndConvertsUTF16Ranges(t *testing.T) {
	type call struct {
		command string
		input   map[string]any
	}
	calls := make(chan call, 1)
	m, tab, main := inlineCompletionTestModel(t, func(_ context.Context, command string, input any) (json.RawMessage, error) {
		calls <- call{command: command, input: input.(map[string]any)}
		return json.RawMessage(`[{"insertText":"print(","range":{"start":{"line":0,"col":0},"end":{"line":0,"col":3}}}]`), nil
	})

	main.do(func() { m.requestInlineCompletion(tab, false) })
	request := <-calls
	if request.command != "lsp_inline_completion" {
		t.Fatalf("command = %q", request.command)
	}
	if request.input["doc"] != tab.doc.ID() || request.input["automatic"] != false {
		t.Fatalf("request input = %#v", request.input)
	}
	pos, ok := request.input["pos"].(map[string]any)
	if !ok || pos["line"] != uint32(0) || pos["col"] != uint32(3) {
		t.Fatalf("position = %#v", request.input["pos"])
	}
	waitForSerialMainThread(t, main, time.Second, func() bool { return len(tab.inline.choices) == 1 })
	if got := tab.inline.choices[0]; got.start != 0 || got.end != 3 || got.text != "print(" {
		t.Fatalf("converted choice = %+v", got)
	}
}

func TestInlineCompletionConvertsUTF16RangesToBufferBytes(t *testing.T) {
	doc := docbridge.Virtual("/Demo.java", "😀x")
	state := &editor.State{Buffer: doc.Buffer(), LineHeight: editorLineHeight(13)}
	state.SetSelections(editor.Selection{Anchor: len("😀x"), Head: len("😀x")})
	tab := &Tab{doc: doc, state: state}
	choices := inlineChoicesFor(tab, len("😀x"), []openidea.InlineCompletionItem{{
		InsertText: "xyz",
		Range: &openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 0, Col: 2},
			End:   openidea.OutlinePos{Line: 0, Col: 3},
		},
	}})
	if len(choices) != 1 {
		t.Fatalf("choices = %+v", choices)
	}
	if choices[0].start != len("😀") || choices[0].end != len("😀x") {
		t.Fatalf("choice range = %d..%d, want %d..%d",
			choices[0].start, choices[0].end, len("😀"), len("😀x"))
	}
}

func TestInlineCompletionDropsALateReplyAfterCaretMoves(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	m, tab, main := inlineCompletionTestModel(t, func(context.Context, string, any) (json.RawMessage, error) {
		close(started)
		<-release
		return json.RawMessage(`[{"insertText":"print(","range":{"start":{"line":0,"col":0},"end":{"line":0,"col":3}}}]`), nil
	})
	main.do(func() { m.requestInlineCompletion(tab, false) })
	<-started
	main.do(func() {
		tab.state.SetSelections(editor.Selection{Anchor: 1, Head: 1})
	})
	close(release)
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return tab.inline.request == 0
	})
	main.do(func() {
		if len(tab.inline.choices) != 0 {
			t.Fatalf("stale reply installed %+v", tab.inline.choices)
		}
	})
}

func TestInlineCompletionAcceptWordCycleAndCommandRunOnce(t *testing.T) {
	var commandCalls atomic.Int32
	m, tab, main := inlineCompletionTestModel(t, func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "lsp_execute_command" {
			return nil, nil
		}
		commandCalls.Add(1)
		return json.RawMessage(`null`), nil
	})
	tab.state.Buffer.Edit(editor.Edit{Start: 0, End: 3, Text: "pri"})
	tab.state.SetSelections(editor.Selection{Anchor: 3, Head: 3})
	tab.inline.choices = []inlineCompletionChoice{
		{text: "print(", start: 0, end: 3, command: json.RawMessage(`{"title":"insert"}`)},
		{text: "println(", start: 0, end: 3},
	}
	tab.inline.caret = 3
	tab.inline.version = tab.state.Buffer.Version()
	main.do(func() { m.showInlineChoice(tab) })

	main.do(func() { m.CycleInlineCompletion(1) })
	if tab.inline.index != 1 {
		t.Fatalf("cycle index = %d, want 1", tab.inline.index)
	}
	main.do(func() { m.CycleInlineCompletion(-1) })
	if tab.inline.index != 0 {
		t.Fatalf("cycle back index = %d, want 0", tab.inline.index)
	}
	main.do(func() { m.AcceptInlineCompletionWord() })
	if got := tab.state.Buffer.String(); !strings.HasPrefix(got, "print") {
		t.Fatalf("accept word produced %q", got)
	}
	if !tab.inlineCompletionVisible() {
		t.Fatal("accept word dismissed the remaining suggestion too early")
	}
	main.do(func() { m.AcceptInlineCompletion() })
	if got := tab.state.Buffer.String(); got != "print(" {
		t.Fatalf("accept all produced %q", got)
	}
	if tab.inlineCompletionVisible() {
		t.Fatal("accept all left a suggestion visible")
	}
	deadline := time.Now().Add(time.Second)
	for commandCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(30 * time.Millisecond)
	if got := commandCalls.Load(); got != 1 {
		t.Fatalf("inline command ran %d times, want once", got)
	}
	main.do(func() {
		if tab.inline.choices != nil {
			t.Fatalf("inline state survived acceptance: %+v", tab.inline)
		}
	})
}

func TestInlineCompletionInputClaimsOnlyVisibleActions(t *testing.T) {
	m, tab, main := inlineCompletionTestModel(t, func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`[]`), nil
	})
	main.do(func() {
		if m.inlineCompletionInput(tab, uiInputKey(uiEscape)) {
			t.Fatal("Escape was claimed without a suggestion")
		}
		tab.inline.choices = []inlineCompletionChoice{{text: "print(", start: 0, end: 3}}
		tab.inline.caret, tab.inline.version = 3, tab.state.Buffer.Version()
		if !m.inlineCompletionInput(tab, uiInputKey(uiTab)) {
			t.Fatal("Tab was not claimed while a suggestion was visible")
		}
		if tab.inlineCompletionVisible() {
			t.Fatal("Tab did not accept the suggestion")
		}
	})
}

func TestInlineCompletionUsesDefaultsAndLoadedLanguageSettings(t *testing.T) {
	m, tab, _ := inlineCompletionTestModel(t, func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`[]`), nil
	})
	if !m.inlineCompletionAllowed(tab) {
		t.Fatal("inline completion default = false, want true")
	}

	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageInlineCompletion: {
			loaded: true,
			applied: map[string]any{
				"editor.inlineCompletion.enabled":           false,
				"editor.inlineCompletion.onlyOnPause":       true,
				"editor.inlineCompletion.pauseMs":           float64(900),
				"editor.inlineCompletion.disabledLanguages": []string{},
			},
			draft: map[string]any{
				"editor.inlineCompletion.enabled":           false,
				"editor.inlineCompletion.onlyOnPause":       true,
				"editor.inlineCompletion.pauseMs":           float64(900),
				"editor.inlineCompletion.disabledLanguages": []string{},
			},
		},
	}
	if m.inlineCompletionAllowed(tab) {
		t.Fatal("inline completion ignored the loaded disabled value")
	}
	setNativeAppliedTestValue(m, settingsPageInlineCompletion, "editor.inlineCompletion.enabled", true)
	setNativeAppliedTestValue(
		m,
		settingsPageInlineCompletion,
		"editor.inlineCompletion.disabledLanguages",
		[]string{"Java"},
	)
	if m.inlineCompletionAllowed(tab) {
		t.Fatal("inline completion ignored the disabled Java language")
	}
	if got := m.inlineCompletionSettingNumber("editor.inlineCompletion.pauseMs"); got != 900 {
		t.Fatalf("inline completion pause = %d, want 900", got)
	}
}

// These aliases keep the keyboard assertions above readable without hiding
// the real MyGo key names.
var (
	uiEscape = ui.KeyEscape
	uiTab    = ui.KeyTab
)

func uiInputKey(key ui.Key) ui.InputEvent {
	return ui.InputEvent{Kind: ui.InputKeyDown, Key: key}
}
