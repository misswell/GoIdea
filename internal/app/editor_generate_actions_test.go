package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGenerateFiltersActionsAndShowsTheNativeChooserForMultipleMatches(t *testing.T) {
	m, _, main := usageHighlightTestModel(t, "class Demo {}", 0,
		func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "lsp_code_action_query" {
				t.Fatalf("unexpected command %q", command)
			}
			return json.RawMessage(`{"actions":[
				{"title":"Generate constructor","kind":"source.generate"},
				{"title":"Add getter","kind":"source.generate"},
				{"title":"Import String","kind":"quickfix.import"}
			]}`), nil
		})

	main.do(func() {
		if !m.RunCommand("code.generate") {
			t.Fatal("code.generate is unavailable")
		}
	})
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.intents != nil && !m.intents.busy
	})
	main.do(func() {
		if m.intents.title != "Generate" {
			t.Fatalf("title = %q", m.intents.title)
		}
		if len(m.intents.items) != 2 ||
			m.intents.items[0].title != "Generate constructor" ||
			m.intents.items[1].title != "Add getter" {
			t.Fatalf("items = %+v", m.intents.items)
		}
	})
}

func TestFilteredCodeActionsAutoApplyTheSingleMatch(t *testing.T) {
	tests := []struct {
		id       string
		response string
		applied  string
	}{
		{
			id: "code.generate",
			response: `{"actions":[
				{"title":"Implement interface","kind":"source.generate"},
				{"title":"Import String","kind":"quickfix.import"}
			]}`,
			applied: "Implement interface",
		},
		{
			id: "code.implementMethods",
			response: `{"actions":[
				{"title":"Override methods","kind":"source.generate"},
				{"title":"Implement missing members","kind":"source.generate"}
			]}`,
			applied: "Implement missing members",
		},
		{
			id: "code.overrideMethods",
			response: `{"actions":[
				{"title":"Implement missing members","kind":"source.generate"},
				{"title":"Override methods","kind":"source.generate"}
			]}`,
			applied: "Override methods",
		},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			applied := make(chan string, 1)
			m, _, main := usageHighlightTestModel(t, "class Demo {}", 0,
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
				if !json.Valid([]byte(action)) ||
					!strings.Contains(action, test.applied) {
					t.Fatalf("applied action = %s", action)
				}
			case <-time.After(time.Second):
				t.Fatal("single filtered action was not applied")
			}
			waitForSerialMainThread(t, main, time.Second, func() bool {
				return m.status == "Applied: "+test.applied
			})
		})
	}
}

func TestFilteredCodeActionsUseSpecificEmptyMessages(t *testing.T) {
	tests := map[string]string{
		"code.generate":         "Nothing to generate here",
		"code.implementMethods": "Nothing to implement",
		"code.overrideMethods":  "Nothing to override",
	}
	for id, want := range tests {
		t.Run(id, func(t *testing.T) {
			m, _, main := usageHighlightTestModel(t, "class Demo {}", 0,
				func(context.Context, string, any) (json.RawMessage, error) {
					return json.RawMessage(`{"actions":[]}`), nil
				})
			main.do(func() { m.RunCommand(id) })
			waitForSerialMainThread(t, main, time.Second, func() bool {
				return m.intents == nil && m.status == want
			})
		})
	}
}
