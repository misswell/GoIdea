package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClassifyRefactorActionMatchesOpenIDEACatalog(t *testing.T) {
	tests := []struct {
		title string
		kind  string
		want  string
	}{
		{"Extract method", "refactor.extract.method", "extractMethod"},
		{"Introduce field", "refactor.extract.field", "extractField"},
		{"Extract to constant in enclosing scope", "refactor", "extractVariable"},
		{"Introduce parameter", "refactor.extract", "extractParameter"},
		{"Inline variable", "refactor", "inline"},
		{"Move to module", "refactor.move", "move"},
		{"Reorder parameters", "refactor.rewrite", "changeSignature"},
		{"Extract interface", "refactor.extract.interface", ""},
		{"Convert to arrow function", "refactor.rewrite.arrow", ""},
		{"Import String", "quickfix.import", ""},
	}
	for _, test := range tests {
		got := classifyRefactorAction(intentAction{title: test.title, kind: test.kind})
		if got != test.want {
			t.Errorf("%q (%s) classifies as %q, want %q",
				test.title, test.kind, got, test.want)
		}
	}
}

func TestRefactorThisQueriesOnlyRefactoringsAndShowsMatchingActions(t *testing.T) {
	type request struct {
		command string
		input   map[string]any
	}
	requests := make(chan request, 1)
	m, tab, main := usageHighlightTestModel(t, "int value = 1;", 0,
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			requests <- request{command: command, input: input.(map[string]any)}
			return json.RawMessage(`{"actions":[
				{"title":"Introduce variable","kind":"refactor.extract.variable"},
				{"title":"Inline variable","kind":"refactor.inline"},
				{"title":"Convert to arrow function","kind":"refactor.rewrite.arrow"},
				{"title":"Import String","kind":"quickfix.import"}
			]}`), nil
		})
	tab.doc.SetReadOnly(false)
	tab.state.ReadOnly = false
	main.do(func() { m.RunCommand("refactor.thisMenu") })
	got := <-requests
	if got.command != "lsp_code_action_query" {
		t.Fatalf("command = %q", got.command)
	}
	if !reflect.DeepEqual(got.input["only"], []string{"refactor"}) {
		t.Fatalf("only = %#v", got.input["only"])
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.intents != nil && !m.intents.busy
	})
	main.do(func() {
		if m.intents.title != "Refactor This" || len(m.intents.items) != 4 {
			t.Fatalf("refactor chooser = title %q items %+v", m.intents.title, m.intents.items)
		}
		if got := []string{
			m.intents.items[0].title,
			m.intents.items[1].title,
			m.intents.items[2].title,
			m.intents.items[3].title,
		}; !reflect.DeepEqual(got, []string{
			"Rename…",
			"Safe Delete…",
			"Introduce variable",
			"Inline variable",
		}) {
			t.Fatalf("Refactor This order = %v", got)
		}
	})
}

func TestRefactorThisRunsAClientSideRowThroughTheChooser(t *testing.T) {
	m, _, main := usageHighlightTestModel(t, "value", 1,
		func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`{"actions":[]}`), nil
		})
	main.do(func() { m.RunCommand("refactor.thisMenu") })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.intents != nil && !m.intents.busy
	})

	var ran bool
	main.do(func() {
		m.intents.items[0].run = func(*Model) { ran = true }
		m.applySelectedIntent()
		if !ran || m.intents != nil {
			t.Fatalf("client row ran=%v intents=%+v", ran, m.intents)
		}
	})
}

func TestRefactorMenuActionAutoAppliesItsMatchingCodeAction(t *testing.T) {
	tests := []struct {
		id       string
		response string
		applied  string
	}{
		{
			id: "refactor.extractVariable",
			response: `{"actions":[
				{"title":"Extract method","kind":"refactor.extract.method"},
				{"title":"Introduce variable","kind":"refactor.extract.variable"}
			]}`,
			applied: "Introduce variable",
		},
		{
			id: "refactor.inline",
			response: `{"actions":[
				{"title":"Move to module","kind":"refactor.move"},
				{"title":"Inline variable","kind":"refactor.inline"}
			]}`,
			applied: "Inline variable",
		},
		{
			id: "refactor.changeSignature",
			response: `{"actions":[
				{"title":"Extract function","kind":"refactor.extract.method"},
				{"title":"Change signature","kind":"refactor.rewrite.signature"}
			]}`,
			applied: "Change signature",
		},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			applied := make(chan string, 1)
			m, _, main := usageHighlightTestModel(t, "int value = 1;", 0,
				func(_ context.Context, command string, input any) (json.RawMessage, error) {
					switch command {
					case "lsp_code_action_query":
						return json.RawMessage(test.response), nil
					case "lsp_apply_code_action":
						action := input.(map[string]any)["action"].(json.RawMessage)
						applied <- string(action)
						return json.RawMessage(`{}`), nil
					default:
						return nil, nil
					}
				})
			main.do(func() { m.RunCommand(test.id) })
			select {
			case action := <-applied:
				if !strings.Contains(action, test.applied) {
					t.Fatalf("applied action = %s", action)
				}
			case <-time.After(time.Second):
				t.Fatal("matching refactoring was not applied")
			}
			waitForSerialMainThread(t, main, time.Second, func() bool {
				return m.status == "Applied: "+test.applied
			})
		})
	}
}

func TestRefactorEmptyMessageExplainsTheRequiredSelection(t *testing.T) {
	if got := refactorEmptyText("extractVariable", false); !strings.Contains(got, "inside an expression") {
		t.Fatalf("caret reason = %q", got)
	}
	if got := refactorEmptyText("extractMethod", true); !strings.Contains(got, "Selected block") {
		t.Fatalf("selection reason = %q", got)
	}
}
