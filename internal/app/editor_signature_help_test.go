package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestParameterInfoRequestsUTF16PositionAndShowsActiveSignature(t *testing.T) {
	type request struct {
		command string
		input   map[string]any
	}
	requests := make(chan request, 1)
	m, tab, main := usageHighlightTestModel(t, "😀call(", len("😀call("),
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			requests <- request{command: command, input: input.(map[string]any)}
			return json.RawMessage(`{
				"signatures":[
					{"label":"first()","documentation":null,"parameters":[]},
					{
						"label":"second(String name, int age)",
						"documentation":{"kind":"markdown","value":"Use the name value."},
						"parameters":[[14,26],[28,31]]
					}
				],
				"activeSignature":1,
				"activeParameter":1
			}`), nil
		})

	main.do(func() {
		if !m.RunCommand("code.parameterInfo") {
			t.Fatal("code.parameterInfo is unavailable")
		}
	})
	got := <-requests
	if got.command != "lsp_signature_help" {
		t.Fatalf("command = %q", got.command)
	}
	if got.input["doc"] != tab.doc.ID() {
		t.Fatalf("document = %#v, want %d", got.input["doc"], tab.doc.ID())
	}
	pos, ok := got.input["pos"].(map[string]any)
	if !ok || pos["line"] != uint32(0) || pos["col"] != uint32(7) {
		t.Fatalf("position = %#v", got.input["pos"])
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.status == "Parameter info"
	})
}

func TestParameterInfoTextUsesActiveSignatureParameterAndDocs(t *testing.T) {
	help := &openidea.SignatureHelp{
		Signatures: []openidea.Signature{
			{Label: "first()"},
			{
				Label: "second(String name, int age)",
				Documentation: &openidea.Markup{
					Kind:  "markdown",
					Value: "Use the `name` value.",
				},
				Parameters: [][2]uint32{{14, 27}, {29, 32}},
			},
		},
		ActiveSignature: 1,
		ActiveParameter: 1,
	}
	text, ok := parameterInfoText(help)
	if !ok {
		t.Fatal("parameterInfoText rejected a valid reply")
	}
	if !strings.Contains(text, "second(String name, int age)") ||
		!strings.Contains(text, "Parameter 2 of 2") ||
		!strings.Contains(text, "Use the name value.") {
		t.Fatalf("parameter info = %q", text)
	}
	help.ActiveSignature = 99
	help.ActiveParameter = 99
	text, ok = parameterInfoText(help)
	if !ok || !strings.HasPrefix(text, "first()") || strings.Contains(text, "Parameter ") {
		t.Fatalf("fallback parameter info = %q, ok=%v", text, ok)
	}
}

func TestParameterInfoCurrentRejectsACaretMove(t *testing.T) {
	m, tab, _ := usageHighlightTestModel(t, "call()", 5, nil)
	tab.signatureGen = 3
	version := tab.state.Buffer.Version()
	if !m.parameterInfoCurrent(tab, 3, tab.doc.ID(), version, 5) {
		t.Fatal("matching parameter-info request was rejected")
	}
	tab.state.SetSelections(editor.Selection{Anchor: 4, Head: 4})
	if m.parameterInfoCurrent(tab, 3, tab.doc.ID(), version, 5) {
		t.Fatal("caret move did not reject the stale parameter-info reply")
	}
}

func TestParameterInfoWithoutSignaturesReportsNone(t *testing.T) {
	m, _, main := usageHighlightTestModel(t, "call()", 5,
		func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`null`), nil
		})
	main.do(func() { m.ShowParameterInfo() })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.status == "No parameter information found"
	})
}

func TestParameterInfoSettingsScheduleAutomaticHintsAfterCallTriggers(t *testing.T) {
	for _, trigger := range []ui.InputEvent{
		{Kind: ui.InputKeyDown, Key: ui.Key9, Mods: ui.Shift},
		{Kind: ui.InputText, Text: ","},
	} {
		inserted, _ := parameterInfoTrigger(trigger)
		requests := make(chan struct{}, 1)
		m, tab, main := usageHighlightTestModel(t, "call", len("call"),
			func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command == "lsp_signature_help" {
					requests <- struct{}{}
				}
				return json.RawMessage(`null`), nil
			})
		tab.editorFocused = true
		m.nativeSettings.pages = map[string]nativeSettingsPageState{
			settingsPageCodeCompletion: {
				loaded: true,
				applied: map[string]any{
					"editor.completion.parameterInfo":      true,
					"editor.completion.parameterInfoDelay": float64(10),
				},
			},
		}
		main.do(func() {
			m.scheduleParameterInfoForInput(tab, trigger)
			tab.state.Insert(inserted)
		})
		select {
		case <-requests:
		case <-time.After(time.Second):
			t.Fatal("automatic parameter info did not query the backend")
		}
	}
}

func TestParameterInfoTriggerMapping(t *testing.T) {
	for _, tc := range []struct {
		event ui.InputEvent
		want  string
	}{
		{event: ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.Key9, Mods: ui.Shift}, want: "("},
		{event: ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyComma}, want: ","},
		{event: ui.InputEvent{Kind: ui.InputText, Text: "("}, want: "("},
		{event: ui.InputEvent{Kind: ui.InputText, Text: ","}, want: ","},
	} {
		got, ok := parameterInfoTrigger(tc.event)
		if !ok || got != tc.want {
			t.Fatalf("parameterInfoTrigger(%+v) = %q, %v", tc.event, got, ok)
		}
	}
	if _, ok := parameterInfoTrigger(ui.InputEvent{Kind: ui.InputText, Text: "x"}); ok {
		t.Fatal("ordinary text opened automatic parameter info")
	}
}
