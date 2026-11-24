package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestEditorShortcutEventsReachRegisteredActions(t *testing.T) {
	tests := []struct {
		name     string
		mods     ui.Modifiers
		key      ui.Key
		command  string
		response string
	}{
		{
			name: "highlight usages", mods: ui.Cmd | ui.Shift, key: ui.KeyF7,
			command: "lsp_document_highlights", response: `[]`,
		},
		{
			name: "go to declaration", mods: ui.Cmd, key: ui.KeyB,
			command: "lsp_goto", response: `[]`,
		},
		{
			name: "generate", mods: ui.Cmd, key: ui.KeyN,
			command: "lsp_code_action_query", response: `{"actions":[]}`,
		},
		{
			name: "implement methods", mods: ui.Ctrl, key: ui.KeyI,
			command: "lsp_code_action_query", response: `{"actions":[]}`,
		},
		{
			name: "override methods", mods: ui.Ctrl, key: ui.KeyO,
			command: "lsp_code_action_query", response: `{"actions":[]}`,
		},
		{
			name: "refactor this", mods: ui.Ctrl, key: ui.KeyT,
			command: "lsp_code_action_query", response: `{"actions":[]}`,
		},
		{
			name: "introduce variable", mods: ui.Cmd | ui.Alt, key: ui.KeyV,
			command: "lsp_code_action_query", response: `{"actions":[]}`,
		},
		{
			name: "change signature", mods: ui.Cmd, key: ui.KeyF6,
			command: "lsp_code_action_query", response: `{"actions":[]}`,
		},
		{
			name: "quick fix", mods: ui.Alt | ui.Shift, key: ui.KeyEnter,
			command: "lsp_code_action_query", response: `{"actions":[]}`,
		},
		{
			name: "parameter info", mods: ui.Cmd, key: ui.KeyP,
			command: "lsp_signature_help", response: `null`,
		},
		{
			name: "expression type info", mods: ui.Ctrl | ui.Shift, key: ui.KeyP,
			command: "lsp_hover", response: `null`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan string, 1)
			path := "/Demo.java"
			text := "int value = 1;"
			backend := &fileActionBackend{info: openidea.DocInfo{
				ID: 7, Path: path, Text: text, Version: 1, Language: "java",
			}}
			doc, err := docbridge.Open(context.Background(), backend, Window, path)
			if err != nil {
				t.Fatal(err)
			}
			tab := &Tab{doc: doc, state: &editor.State{
				Buffer: doc.Buffer(), LineHeight: editorLineHeight(13),
			}}
			tab.state.SetSelections(editor.Selection{Anchor: 4, Head: 4})
			main := &serialMainThread{}
			m := &Model{
				ctx: context.Background(), postHook: main.post,
				tabs: []*Tab{tab}, active: 0,
			}
			m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				requests <- command
				return json.RawMessage(test.response), nil
			}
			tab.doc.SetReadOnly(false)
			tab.state.ReadOnly = false
			if test.name == "quick fix" {
				tab.state.SetDiagnostics(tab.state.Buffer.Version(), []editor.Diagnostic{{
					Start: 0, End: len("int"), Severity: editor.SeverityError,
					Message: "broken type",
				}})
			}

			tester := ui.NewTester(func(c *ui.Context) {
				m.shortcuts(c)
			}, 800, 600)
			main.do(func() {
				tester.Key(test.mods, test.key)
			})
			select {
			case command := <-requests:
				if command != test.command {
					t.Fatalf("shortcut sent %q, want %q", command, test.command)
				}
			case <-time.After(time.Second):
				t.Fatalf("shortcut did not reach %s", test.command)
			}
		})
	}
}

func TestServicesShortcutEventTogglesTheNativeWindow(t *testing.T) {
	m := &Model{}
	tester := ui.NewTester(func(c *ui.Context) {
		m.shortcuts(c)
	}, 800, 600)

	tester.Key(ui.Cmd, ui.Key8)
	if !m.dockWindowShown("services") {
		t.Fatal("Cmd+8 did not open Services")
	}
	tester.Key(ui.Cmd, ui.Key8)
	if m.dockWindowShown("services") {
		t.Fatal("second Cmd+8 did not close Services")
	}
}
