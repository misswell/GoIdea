package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func pasteHistoryTestTab(t *testing.T, text string) (*Model, *Tab) {
	t.Helper()
	previousClipboardWrite := pasteHistoryClipboardWrite
	pasteHistoryClipboardWrite = func(string) {}
	t.Cleanup(func() { pasteHistoryClipboardWrite = previousClipboardWrite })

	doc := docbridge.Virtual("/demo.properties", text)
	doc.SetReadOnly(false)
	state := &editor.State{
		Buffer:     doc.Buffer(),
		LineHeight: editorLineHeight(13),
	}
	tab := &Tab{doc: doc, state: state}
	return &Model{tabs: []*Tab{tab}, active: 0}, tab
}

func TestPasteHistoryDeduplicatesCapsAndFilters(t *testing.T) {
	m, _ := pasteHistoryTestTab(t, "")
	for i := 0; i < 35; i++ {
		m.rememberEditorClipboard(fmt.Sprintf("entry-%02d", i), false)
	}
	if got := len(m.editorExtras.history); got != maxEditorClipboardEntries {
		t.Fatalf("history length = %d, want %d", got, maxEditorClipboardEntries)
	}
	if m.editorExtras.history[0].text != "entry-34" {
		t.Fatalf("newest history entry = %q", m.editorExtras.history[0].text)
	}
	m.rememberEditorClipboard("entry-10", false)
	if got := m.editorExtras.history[0].text; got != "entry-10" {
		t.Fatalf("duplicate did not move to the top: %q", got)
	}
	if got := len(m.editorExtras.history); got != maxEditorClipboardEntries {
		t.Fatalf("history length after duplicate = %d", got)
	}
	m.filterPasteHistory("ENTRY-3")
	entries := m.pasteHistoryVisibleEntries()
	if len(entries) == 0 || entries[0].text != "entry-34" {
		t.Fatalf("filtered entries = %+v", entries)
	}
	if !containsClipboardEntry(entries, "entry-30") {
		t.Fatalf("filter did not match case-insensitively: %+v", entries)
	}
}

func TestPasteHistoryCopiesSelectionsAndLinewiseCarets(t *testing.T) {
	m, tab := pasteHistoryTestTab(t, "alpha\nbeta\ngamma\n")
	tab.state.SetSelections(
		editor.Selection{Anchor: 0, Head: 5},
		editor.Selection{Anchor: len("alpha\n"), Head: len("alpha\nbeta")},
	)
	text, linewise := copiedEditorText(tab)
	if text != "alpha\nbeta" || linewise {
		t.Fatalf("copied selections = %q, linewise %v", text, linewise)
	}
	tab.state.SetSelections(
		editor.Selection{Anchor: 2, Head: 2},
		editor.Selection{Anchor: len("alpha\n") + 2, Head: len("alpha\n") + 2},
	)
	text, linewise = copiedEditorText(tab)
	if text != "alpha\nbeta" || !linewise {
		t.Fatalf("copied caret lines = %q, linewise %v", text, linewise)
	}
	m.rememberEditorClipboard("first", false)
	m.rememberEditorClipboard("second", true)
	if m.editorExtras.history[0].text != "second" || m.editorExtras.history[0].linewise != true {
		t.Fatalf("history newest = %+v", m.editorExtras.history[0])
	}
}

func TestPasteHistoryDeletesAndMergesSelectedRows(t *testing.T) {
	m, tab := pasteHistoryTestTab(t, "start\n")
	m.rememberEditorClipboard("one", false)
	m.rememberEditorClipboard("two", false)
	m.rememberEditorClipboard("three", false)
	m.OpenPasteFromHistory()
	m.editorExtras.chosen.Clear()
	m.editorExtras.chosen.Add("three")
	m.editorExtras.chosen.Add("one")
	tab.state.SetSelections(editor.Selection{Anchor: len("start\n"), Head: len("start\n")})
	m.PasteFromHistory(nil)
	if got := tab.state.Buffer.String(); got != "start\nthree\none" {
		t.Fatalf("merged paste = %q", got)
	}
	if got := m.editorExtras.history[0].text; got != "three\none" {
		t.Fatalf("merged paste was not recorded first: %q", got)
	}

	m.rememberEditorClipboard("remove-me", false)
	m.OpenPasteFromHistory()
	m.filterPasteHistory("remove")
	m.editorExtras.chosen.Clear()
	m.editorExtras.chosen.Add("remove-me")
	m.deleteSelectedPasteHistory()
	for _, entry := range m.editorExtras.history {
		if entry.text == "remove-me" {
			t.Fatal("deleted history entry is still present")
		}
	}
}

func TestPasteHistoryLinewiseAndMultiCaretPaste(t *testing.T) {
	m, tab := pasteHistoryTestTab(t, "a\nb\n")
	tab.state.SetSelections(
		editor.Selection{Anchor: 0, Head: 0},
		editor.Selection{Anchor: len("a\n"), Head: len("a\n")},
	)
	m.pasteEditorHistoryText(tab, "x", true)
	if got := tab.state.Buffer.String(); got != "x\na\nx\nb\n" {
		t.Fatalf("linewise insert = %q", got)
	}

	m, tab = pasteHistoryTestTab(t, "one\ntwo\n")
	tab.state.SetSelections(
		editor.Selection{Anchor: 0, Head: 0},
		editor.Selection{Anchor: len("one\n"), Head: len("one\n")},
	)
	m.pasteEditorHistoryText(tab, "A\nB", false)
	if got := tab.state.Buffer.String(); got != "Aone\nBtwo\n" {
		t.Fatalf("multi-caret insert = %q", got)
	}
}

func TestPasteHistoryDialogRendersNativeChoices(t *testing.T) {
	m, _ := pasteHistoryTestTab(t, "")
	m.rememberEditorClipboard("hello\nworld", false)
	m.rememberEditorClipboard("single", true)
	m.OpenPasteFromHistory()
	tester := ui.NewTester(func(c *ui.Context) {
		m.editorPasteHistoryDialog(c, c.Theme())
	}, 900, 700)
	tester.Frame()
	for _, want := range []string{"Choose Content to Paste", "Clipboard history", "hello", "single", "Delete", "Paste", "Cancel"} {
		if !tester.HasText(want) {
			t.Fatalf("%q missing from paste history dialog: %q", want, tester.Texts())
		}
	}
}

func containsClipboardEntry(entries []editorClipboardEntry, text string) bool {
	for _, entry := range entries {
		if strings.EqualFold(entry.text, text) {
			return true
		}
	}
	return false
}
