package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestExpressionTypeInfoRequestsUTF16AndShowsHoverType(t *testing.T) {
	type request struct {
		command string
		input   map[string]any
	}
	requests := make(chan request, 1)
	m, tab, main := usageHighlightTestModel(t, "😀value", len("😀"),
		func(_ context.Context, command string, input any) (json.RawMessage, error) {
			requests <- request{command: command, input: input.(map[string]any)}
			data, err := json.Marshal(map[string]any{
				"contents": map[string]any{
					"kind":  "markdown",
					"value": "```typescript\nconst value: string = \"x\"\n```",
				},
				"range": map[string]any{
					"start": map[string]any{"line": 0, "col": 2},
					"end":   map[string]any{"line": 0, "col": 7},
				},
			})
			return json.RawMessage(data), err
		})

	main.do(func() {
		if !m.RunCommand("code.expressionTypeInfo") {
			t.Fatal("code.expressionTypeInfo is unavailable")
		}
	})
	got := <-requests
	if got.command != "lsp_hover" {
		t.Fatalf("command = %q", got.command)
	}
	if got.input["doc"] != tab.doc.ID() {
		t.Fatalf("document = %#v, want %d", got.input["doc"], tab.doc.ID())
	}
	pos, ok := got.input["pos"].(map[string]any)
	if !ok || pos["line"] != uint32(0) || pos["col"] != uint32(2) {
		t.Fatalf("position = %#v", got.input["pos"])
	}
	if _, ok := got.input["request"].(uint32); !ok {
		t.Fatalf("request = %#v", got.input["request"])
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.status == "Expression type info"
	})
}

func TestHoverTypeInfoPrefersARealCodeBlockOverAPath(t *testing.T) {
	hover := &openidea.Hover{Contents: openidea.Markup{
		Kind: "markdown",
		Value: "```text\ncrate::module::Item\n```\n\n" +
			"```rust\nlet item: Vec<String> = values\n```",
	}}
	text, ok := hoverTypeInfo(hover)
	if !ok || text != "Vec<String>" {
		t.Fatalf("type info = %q, ok=%v", text, ok)
	}

	hover.Contents.Value = "`(property) item: Map<String, Integer>`"
	text, ok = hoverTypeInfo(hover)
	if !ok || text != "Map<String, Integer>" {
		t.Fatalf("inline type info = %q, ok=%v", text, ok)
	}
}

func TestTypeInfoCurrentRejectsABufferChange(t *testing.T) {
	m, tab, _ := usageHighlightTestModel(t, "value", 0, nil)
	tab.typeInfoGen = 4
	version := tab.state.Buffer.Version()
	if !m.typeInfoRequestCurrent(tab, 4, tab.doc.ID(), version, 0) {
		t.Fatal("matching type-info request was rejected")
	}
	tab.state.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "x"})
	if m.typeInfoRequestCurrent(tab, 4, tab.doc.ID(), version, 0) {
		t.Fatal("buffer change did not reject the stale type-info reply")
	}
}
