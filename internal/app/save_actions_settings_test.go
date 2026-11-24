package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func TestActionsOnSaveRunOptimizeImportsThenCleanup(t *testing.T) {
	doc := docbridge.Virtual("/Demo.java", "package demo;\n")
	tab := &Tab{
		doc:   doc,
		state: &editor.State{Buffer: doc.Buffer()},
	}
	var calls []string
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args := input.(map[string]any)
			switch command {
			case "lsp_code_action_query":
				only := args["only"].([]string)
				calls = append(calls, "query:"+only[0])
				return json.RawMessage(`{"actions":[{"title":"Action","kind":"` + only[0] + `"}]}`), nil
			case "lsp_apply_code_action":
				var action struct {
					Kind string `json:"kind"`
				}
				if err := json.Unmarshal(args["action"].(json.RawMessage), &action); err != nil {
					t.Fatal(err)
				}
				calls = append(calls, "apply:"+action.Kind)
				doc.Buffer().Edit(editor.Edit{Start: 0, End: 0, Text: " "})
				return json.RawMessage(`null`), nil
			default:
				t.Fatalf("command = %q", command)
				return nil, nil
			}
		},
	}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageActionsOnSave: {
			loaded: true,
			applied: map[string]any{
				"actionsOnSave.optimizeImports": true,
				"actionsOnSave.cleanup":         true,
			},
			draft: map[string]any{
				"actionsOnSave.optimizeImports": true,
				"actionsOnSave.cleanup":         true,
			},
		},
	}

	if err := m.applyActionsOnSave(context.Background(), tab); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"query:source.organizeImports",
		"apply:source.organizeImports",
		"query:source.fixAll",
		"apply:source.fixAll",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("actions on save = %v, want %v", calls, want)
	}
	if tab.formatting {
		t.Fatal("actions on save left the tab marked as formatting")
	}
}

func TestActionsOnSaveDoNothingWhenDisabled(t *testing.T) {
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("disabled actions on save contacted the backend")
			return nil, nil
		},
	}
	if err := m.applyActionsOnSave(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
