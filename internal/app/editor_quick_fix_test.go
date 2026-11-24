package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

func quickFixTestModel(
	t *testing.T,
	text string,
	caret int,
	hook func(context.Context, string, any) (json.RawMessage, error),
) (*Model, *Tab, *serialMainThread) {
	t.Helper()
	m, tab, main := usageHighlightTestModel(t, text, caret, hook)
	tab.doc.SetReadOnly(false)
	return m, tab, main
}

func TestApplyQuickFixChoosesPreferredActionAndSendsExactRequests(t *testing.T) {
	type request struct {
		command string
		input   map[string]any
	}
	requests := make(chan request, 2)
	m, tab, main := quickFixTestModel(t, "broken name", 1,
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			requests <- request{command: command, input: input.(map[string]any)}
			switch command {
			case "lsp_code_action_query":
				return json.RawMessage(`{"actions":[
					{"title":"First","kind":"quickfix.first","isPreferred":false},
					{"title":"Disabled","kind":"quickfix.disabled","isPreferred":true,"disabledReason":"not here"},
					{"title":"Preferred","kind":"quickfix.preferred","isPreferred":true,"edit":"keep me"}
				]}`), nil
			case "lsp_apply_code_action":
				return json.RawMessage(`{}`), nil
			default:
				return nil, nil
			}
		})
	tab.state.SetDiagnostics(tab.state.Buffer.Version(), []editor.Diagnostic{
		{
			Start: 0, End: 6, Severity: editor.SeverityError,
			Message: "cannot resolve",
		},
		{
			Start: 7, End: 11, Severity: editor.SeverityWarning,
			Message: "unused",
		},
	})

	main.do(func() {
		if !m.RunCommand("code.applyQuickFix") {
			t.Fatal("code.applyQuickFix is unavailable")
		}
	})
	query := <-requests
	if query.command != "lsp_code_action_query" {
		t.Fatalf("first command = %q", query.command)
	}
	if query.input["doc"] != tab.doc.ID() {
		t.Fatalf("document = %#v, want %d", query.input["doc"], tab.doc.ID())
	}
	rng, ok := query.input["range"].(map[string]any)
	if !ok {
		t.Fatalf("range = %#v", query.input["range"])
	}
	start, _ := rng["start"].(map[string]any)
	end, _ := rng["end"].(map[string]any)
	if start["line"] != uint32(0) || start["col"] != uint32(0) ||
		end["line"] != uint32(0) || end["col"] != uint32(6) {
		t.Fatalf("range = %#v", rng)
	}
	if !reflect.DeepEqual(query.input["only"], []string{"quickfix"}) {
		t.Fatalf("only = %#v", query.input["only"])
	}
	if _, ok := query.input["request"].(uint32); !ok {
		t.Fatalf("request = %#v", query.input["request"])
	}

	apply := <-requests
	if apply.command != "lsp_apply_code_action" {
		t.Fatalf("second command = %q", apply.command)
	}
	if apply.input["doc"] != tab.doc.ID() {
		t.Fatalf("apply document = %#v", apply.input["doc"])
	}
	action, ok := apply.input["action"].(json.RawMessage)
	if !ok {
		t.Fatalf("action = %#v", apply.input["action"])
	}
	text := string(action)
	if !strings.Contains(text, `"title":"Preferred"`) ||
		!strings.Contains(text, `"edit":"keep me"`) {
		t.Fatalf("applied raw action = %s", text)
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.status == "Applied quick fix: Preferred"
	})
}

func TestApplyQuickFixFallsBackToTheNextProblemOnTheLine(t *testing.T) {
	var queries int
	applied := make(chan string, 1)
	m, tab, main := quickFixTestModel(t, "one two", 1,
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "lsp_code_action_query":
				queries++
				if queries == 1 {
					return json.RawMessage(`{"actions":[]}`), nil
				}
				rng := input.(map[string]any)["range"].(map[string]any)
				start := rng["start"].(map[string]any)
				if start["col"] != uint32(4) {
					t.Errorf("second query start = %#v, want col 4", start)
				}
				return json.RawMessage(
					`{"actions":[{"title":"Second problem fix","kind":"quickfix.second"}]}`,
				), nil
			case "lsp_apply_code_action":
				action := input.(map[string]any)["action"].(json.RawMessage)
				applied <- string(action)
				return json.RawMessage(`{}`), nil
			default:
				return nil, nil
			}
		})
	tab.state.SetDiagnostics(tab.state.Buffer.Version(), []editor.Diagnostic{
		{
			Start: 0, End: 3, Severity: editor.SeverityError,
			Message: "first",
		},
		{
			Start: 4, End: 7, Severity: editor.SeverityWarning,
			Message: "second",
		},
	})

	main.do(func() { m.RunCommand("code.applyQuickFix") })
	select {
	case action := <-applied:
		if !strings.Contains(action, "Second problem fix") {
			t.Fatalf("applied action = %s", action)
		}
	case <-time.After(time.Second):
		t.Fatal("second problem's quick fix was not applied")
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.status == "Applied quick fix: Second problem fix"
	})
}

func TestApplyQuickFixWithoutProblemsIsQuietlyUnavailable(t *testing.T) {
	m, _, main := quickFixTestModel(t, "clean", 1, nil)
	main.do(func() {
		if !m.RunCommand("code.applyQuickFix") {
			t.Fatal("code.applyQuickFix is unavailable")
		}
		if m.status != "No quick fixes available" {
			t.Fatalf("status = %q", m.status)
		}
	})
}

func TestQuickFixRequestCurrentRejectsCaretAndBufferChanges(t *testing.T) {
	m, tab, _ := quickFixTestModel(t, "old", 0, nil)
	tab.quickFixGen = 7
	version := tab.state.Buffer.Version()
	if !m.quickFixRequestCurrent(tab, 7, tab.doc.ID(), version, 0) {
		t.Fatal("matching quick-fix request was rejected")
	}
	tab.state.SetSelections(editor.Selection{Anchor: 1, Head: 1})
	if m.quickFixRequestCurrent(tab, 7, tab.doc.ID(), version, 0) {
		t.Fatal("caret change did not reject the stale quick-fix request")
	}
	tab.state.SetSelections(editor.Selection{Anchor: 0, Head: 0})
	tab.state.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "x"})
	if m.quickFixRequestCurrent(tab, 7, tab.doc.ID(), version, 0) {
		t.Fatal("buffer change did not reject the stale quick-fix request")
	}
}
