package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestVimNamedRegistersIndependentFromUnnamed(t *testing.T) {
	m, tab, state := newVimTestModel(t, "first\nsecond\n")
	feedVim(t, m, tab, state, "\"", "a", "d", "d")
	if reg, ok := m.vimNamedRegister(state, 'a'); !ok ||
		reg.text != "first\n" || !reg.linewise {
		t.Fatalf("named register a = %+v ok=%v", reg, ok)
	}
	if got := tab.state.Buffer.String(); got != "second\n" {
		t.Fatalf("named dd = %q", got)
	}

	feedVim(t, m, tab, state, "\"", "b", "y", "y")
	if reg, _ := m.vimNamedRegister(state, 'b'); reg.text != "second\n" {
		t.Fatalf("named register b = %+v", reg)
	}
	feedVim(t, m, tab, state, "\"", "a", "p")
	if got := tab.state.Buffer.String(); got != "second\nfirst\n" {
		t.Fatalf("named paste = %q", got)
	}
	if state.registerName != 0 {
		t.Fatalf("register prefix leaked: %q", state.registerName)
	}
}

func TestVimMacrosRecordPlayCountRepeatAndStop(t *testing.T) {
	m, tab, state := newVimTestModel(t, "one\ntwo\nthree\nfour\n")
	feedVim(t, m, tab, state, "q", "a", "d", "d", "q")
	if state.recording != 0 || len(m.vim.macros['a']) != 2 {
		t.Fatalf("recorded macro state=%+v macro=%v", state, m.vim.macros['a'])
	}
	if got := tab.state.Buffer.String(); got != "two\nthree\nfour\n" {
		t.Fatalf("macro recording changed text = %q", got)
	}
	feedVim(t, m, tab, state, "2", "@", "a")
	if got := tab.state.Buffer.String(); got != "four\n" {
		t.Fatalf("counted macro = %q", got)
	}
	tab.state.Run(editor.Undo)
	if got := tab.state.Buffer.String(); got != "two\nthree\nfour\n" {
		t.Fatalf("counted macro undo = %q", got)
	}
	tab.state.Select(0, 0)
	feedVim(t, m, tab, state, "@", "@")
	if got := tab.state.Buffer.String(); got != "three\nfour\n" {
		t.Fatalf("@@ macro = %q", got)
	}

	m, tab, state = newVimTestModel(t, "abc\ndef\n")
	feedVim(t, m, tab, state, "q", "b", "i")
	if m.vimInput(tab, ui.InputEvent{Kind: ui.InputText, Text: "X"}) {
		t.Fatal("insert text was consumed instead of reaching MyGo")
	}
	tab.state.Insert("X")
	feedVim(t, m, tab, state, "<Esc>", "q")
	if got := tab.state.Buffer.String(); got != "Xabc\ndef\n" {
		t.Fatalf("insert macro recording = %q", got)
	}
	feedVim(t, m, tab, state, "j", "0", "@", "b")
	if got := tab.state.Buffer.String(); got != "Xabc\nXdef\n" {
		t.Fatalf("insert macro playback = %q", got)
	}
}

func TestVimMacrosRecursiveGuardAndStaleTab(t *testing.T) {
	m, tab, state := newVimTestModel(t, "one\ntwo\n")
	m.vim.macros = map[rune][]string{'a': {"@", "a"}}
	feedVim(t, m, tab, state, "@", "a")
	if !strings.Contains(m.status, "Vim:") {
		t.Fatalf("recursive macro status = %q", m.status)
	}

	m.tabs = nil
	feedVim(t, m, tab, state, "@", "a")
	if !strings.Contains(m.status, "E158") {
		t.Fatalf("stale macro status = %q", m.status)
	}
}

func TestVimMacrosCaptureAndReplayCommandLineText(t *testing.T) {
	m, tab, state := newVimTestModel(t, "one two\none three\n")
	feedVim(t, m, tab, state, "q", "b", ":")
	if m.vim.command == nil {
		t.Fatal("command-line prompt did not open")
	}
	m.vim.command.input = "s/one/two/g"
	m.submitVimCommand()
	feedVim(t, m, tab, state, "q")
	if got := tab.state.Buffer.String(); got != "two two\none three\n" {
		t.Fatalf("first command-line macro run = %q", got)
	}

	tab.state.Run(editor.Undo)
	tab.state.Select(0, 0)
	feedVim(t, m, tab, state, "@", "b")
	if got := tab.state.Buffer.String(); got != "two two\none three\n" {
		t.Fatalf("replayed command-line macro = %q", got)
	}
	if got := strings.Join(m.vim.macros['b'], "|"); got != ":|s|/|o|n|e|/|t|w|o|/|g|<Enter>" {
		t.Fatalf("command-line macro tokens = %q", m.vim.macros['b'])
	}
}

func TestVimLetterMarksLineExactMissingAndClamp(t *testing.T) {
	text := "one\ntwo\nthree\n"
	m, tab, state := newVimTestModel(t, text)
	tab.state.Select(5, 5)
	feedVim(t, m, tab, state, "m", "a")
	tab.state.Select(len(text), len(text))
	feedVim(t, m, tab, state, "'", "a")
	if got := tab.state.Cursor(); got != 4 {
		t.Fatalf("line mark cursor = %d, want 4", got)
	}
	tab.state.Select(len(text), len(text))
	feedVim(t, m, tab, state, "`", "a")
	if got := tab.state.Cursor(); got != 5 {
		t.Fatalf("exact mark cursor = %d, want 5", got)
	}

	feedVim(t, m, tab, state, "'", "z")
	if !strings.Contains(m.status, "E20") {
		t.Fatalf("missing mark status = %q", m.status)
	}
	tab.state.Buffer.Edit(editor.Edit{Start: 0, End: len(text)})
	feedVim(t, m, tab, state, "`", "a")
	if got := tab.state.Cursor(); got < 0 || got > tab.state.Buffer.Len() {
		t.Fatalf("clamped mark cursor = %d", got)
	}
}
