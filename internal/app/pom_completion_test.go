package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func pomCompletionAcceptanceModel(
	t *testing.T,
	text string,
	valueStart, valueEnd, caret int,
) (*Model, *Tab) {
	t.Helper()
	m, tab, _ := completionInputTestModel(
		"/project/pom.xml",
		text,
		text[:caret],
		map[string]any{"editor.completion.insertOnSpecialKeys": false},
	)
	item := editor.Completion{
		Label:  "9.8.7",
		Kind:   editor.CompletionConstant,
		Insert: "9.8.7",
	}
	tab.state.ShowCompletions(valueStart, []editor.Completion{item})
	tab.completion.start = valueStart
	tab.completion.selected = &item
	tab.completion.infos = map[string]completionInfo{
		completionTrackKey(item): {
			Replace: &completionReplacement{From: valueStart, To: valueEnd},
		},
	}
	return m, tab
}

func TestPOMCompletionReplacesTheWholeValueFromTheMiddle(t *testing.T) {
	text := `<project><version>1.2.3</version></project>`
	valueStart := len(`<project><version>`)
	valueEnd := valueStart + len("1.2.3")
	caret := valueStart + len("1.")

	t.Run("tab", func(t *testing.T) {
		m, tab := pomCompletionAcceptanceModel(t, text, valueStart, valueEnd, caret)
		if !m.tabTemplateInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyTab}) {
			t.Fatal("Tab did not accept the POM completion")
		}
		want := `<project><version>9.8.7</version></project>`
		if got := tab.doc.Text(); got != want {
			t.Fatalf("buffer = %q, want %q", got, want)
		}
		if got := tab.state.Cursor(); got != valueStart+len("9.8.7") {
			t.Fatalf("caret = %d", got)
		}
	})

	t.Run("enter", func(t *testing.T) {
		m, tab := pomCompletionAcceptanceModel(t, text, valueStart, valueEnd, caret)
		if !m.completionInput(tab, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}) {
			t.Fatal("Enter did not accept the POM completion")
		}
		want := `<project><version>9.8.7</version></project>`
		if got := tab.doc.Text(); got != want {
			t.Fatalf("buffer = %q, want %q", got, want)
		}
	})
}

func TestCompletionReplacementRejectsStaleOrOutOfBoundsSpans(t *testing.T) {
	m, tab, _ := completionInputTestModel(
		"/project/pom.xml", "<version>1.2.3</version>", "<version>1.",
		map[string]any{},
	)
	item := editor.Completion{Label: "2.0", Insert: "2.0"}
	if m.applyCompletionReplacement(tab, item, completionReplacement{From: -1, To: 2}) {
		t.Fatal("negative completion span was accepted")
	}
	if m.applyCompletionReplacement(tab, item, completionReplacement{From: 3, To: 500}) {
		t.Fatal("out-of-bounds completion span was accepted")
	}
	if got := tab.doc.Text(); got != "<version>1.2.3</version>" {
		t.Fatalf("invalid completion changed the buffer to %q", got)
	}
}
