package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestVimSearchDirectionsRepeatCountAndWrap(t *testing.T) {
	text := "one two\nthree two\nlast\n"
	m, tab, state := newVimTestModel(t, text)
	first := strings.Index(text, "two")
	second := strings.LastIndex(text, "two")

	if err := m.vimExecuteSearch(tab, state, "/", "two"); err != nil {
		t.Fatal(err)
	}
	if got := tab.state.Cursor(); got != first {
		t.Fatalf("forward search cursor = %d, want %d", got, first)
	}
	if err := m.vimRepeatSearch(tab, state, 1, false); err != nil {
		t.Fatal(err)
	}
	if got := tab.state.Cursor(); got != second {
		t.Fatalf("n cursor = %d, want %d", got, second)
	}
	if err := m.vimRepeatSearch(tab, state, 1, true); err != nil {
		t.Fatal(err)
	}
	if got := tab.state.Cursor(); got != first {
		t.Fatalf("N cursor = %d, want %d", got, first)
	}
	if err := m.vimRepeatSearch(tab, state, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := m.vimRepeatSearch(tab, state, 1, false); err != nil {
		t.Fatal(err)
	}
	if got := tab.state.Cursor(); got != first {
		t.Fatalf("wrapped search cursor = %d, want %d", got, first)
	}

	tab.state.Select(len(text), len(text))
	if err := m.vimExecuteSearch(tab, state, "?", "two"); err != nil {
		t.Fatal(err)
	}
	if got := tab.state.Cursor(); got != second {
		t.Fatalf("backward search cursor = %d, want %d", got, second)
	}
	feedVim(t, m, tab, state, "n")
	if got := tab.state.Cursor(); got != first {
		t.Fatalf("backward repeat cursor = %d, want %d", got, first)
	}
}

func TestVimSearchModeCommandAndErrors(t *testing.T) {
	text := "Alpha\nalpha\n"
	m, tab, state := newVimTestModel(t, text)
	feedVim(t, m, tab, state, "/")
	if m.vim.command == nil || m.vim.command.prompt != "/" {
		t.Fatalf("search command = %+v", m.vim.command)
	}
	m.vim.command.input = "["
	m.submitVimCommand()
	if m.vim.command == nil || !strings.Contains(m.vim.command.err, "E486") {
		t.Fatalf("invalid search error = %+v", m.vim.command)
	}
	m.vim.command.input = "missing"
	m.submitVimCommand()
	if m.vim.command == nil || !strings.Contains(m.vim.command.err, "Pattern not found") {
		t.Fatalf("missing search error = %+v", m.vim.command)
	}
	m.vim.command.input = "Alpha"
	m.submitVimCommand()
	if m.vim.command != nil {
		t.Fatalf("successful search kept command = %+v", m.vim.command)
	}

	m.tabs = nil
	if err := m.vimExecuteSearch(tab, state, "/", "Alpha"); err == nil {
		t.Fatal("stale tab search did not fail")
	}
	if err := m.vimRepeatSearch(tab, state, 1, false); err == nil {
		t.Fatal("stale tab repeat search did not fail")
	}
}

func TestVimSubstituteRangesFlagsDelimitersCapturesAndUndo(t *testing.T) {
	text := "a1 a2\nb1 b2\nc1 c2\n"
	m, tab, state := newVimTestModel(t, text)
	if err := m.runVimExCommand(tab, "1,2s/([ab])([0-9])/\\2\\1/g"); err != nil {
		t.Fatal(err)
	}
	want := "1a 2a\n1b 2b\nc1 c2\n"
	if got := tab.state.Buffer.String(); got != want {
		t.Fatalf("range/g substitute = %q, want %q", got, want)
	}
	if !tab.state.Buffer.CanUndo() {
		t.Fatal("substitute did not make an undo step")
	}
	tab.state.Run(editor.Undo)
	if got := tab.state.Buffer.String(); got != text {
		t.Fatalf("substitute undo = %q", got)
	}
	tab.state.Run(editor.Redo)
	if got := tab.state.Buffer.String(); got != want {
		t.Fatalf("substitute redo = %q", got)
	}

	if err := m.runVimExCommand(tab, "%s#C#X#i"); err != nil {
		t.Fatal(err)
	}
	if got := tab.state.Buffer.String(); got != "1a 2a\n1b 2b\nX1 c2\n" {
		t.Fatalf("case-insensitive substitute = %q", got)
	}
	if err := m.runVimExCommand(tab, "1s|1a|&!|"); err != nil {
		t.Fatal(err)
	}
	if got := tab.state.Buffer.String(); !strings.HasPrefix(got, "1a! 2a") {
		t.Fatalf("whole-match substitute = %q", got)
	}
	before := tab.state.Buffer.String()
	if err := m.runVimExCommand(tab, "1s/not-there/replacement/"); err == nil ||
		!strings.Contains(err.Error(), "Pattern not found") {
		t.Fatalf("missing substitute error = %v", err)
	}
	if got := tab.state.Buffer.String(); got != before {
		t.Fatalf("failed substitute changed text: %q", got)
	}
	if err := m.runVimExCommand(tab, "1s/[bad/replacement/"); err == nil {
		t.Fatal("bad substitute pattern did not fail")
	}
	if err := m.runVimExCommand(tab, "9s/a/b/"); err == nil {
		t.Fatal("out-of-range substitute did not fail")
	}

	m.tabs = nil
	if handled, err := m.vimExecuteSubstitute(tab, "1s/a/b/"); !handled || err == nil {
		t.Fatalf("stale substitute = handled=%v err=%v", handled, err)
	}
	_ = state
}
