package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestVimVisualBlockDeleteYankUndoAndRedo(t *testing.T) {
	text := "abcd\nwxyz\n1234\n"
	m, tab, state := newVimTestModel(t, text)
	tab.state.Select(1, 1)
	feedVim(t, m, tab, state, "<Ctrl+V>", "j", "l")
	if state.mode != vimVisualBlock {
		t.Fatalf("mode = %v", state.mode)
	}
	if got := len(tab.state.Selections()); got != 2 {
		t.Fatalf("block selections = %d, want 2", got)
	}
	feedVim(t, m, tab, state, "d")
	if got := tab.state.Buffer.String(); got != "ad\nwz\n1234\n" {
		t.Fatalf("block delete = %q", got)
	}
	if state.mode != vimNormal || state.register.text != "bc\nxy" {
		t.Fatalf("block delete state = %+v register=%+v", state, state.register)
	}
	tab.state.Run(editor.Undo)
	if got := tab.state.Buffer.String(); got != text {
		t.Fatalf("block undo = %q", got)
	}
	tab.state.Run(editor.Redo)
	if got := tab.state.Buffer.String(); got != "ad\nwz\n1234\n" {
		t.Fatalf("block redo = %q", got)
	}

	m, tab, state = newVimTestModel(t, text)
	tab.state.Select(1, 1)
	feedVim(t, m, tab, state, "<Ctrl+V>", "j", "l", "y")
	if got := tab.state.Buffer.String(); got != text {
		t.Fatalf("block yank changed text = %q", got)
	}
	if state.register.text != "bc\nxy" || state.mode != vimNormal {
		t.Fatalf("block yank state = %+v register=%+v", state, state.register)
	}
}

func TestVimVisualBlockInsertAppendAndChange(t *testing.T) {
	text := "abcd\nwxyz\n1234\n"
	m, tab, state := newVimTestModel(t, text)
	tab.state.Select(1, 1)
	feedVim(t, m, tab, state, "<Ctrl+V>", "j", "I")
	if state.mode != vimInsert || len(tab.state.Selections()) != 2 {
		t.Fatalf("block I state=%+v selections=%v", state, tab.state.Selections())
	}
	tab.state.Insert("X")
	feedVim(t, m, tab, state, "<Esc>")
	if got := tab.state.Buffer.String(); got != "aXbcd\nwXxyz\n1234\n" {
		t.Fatalf("block I = %q", got)
	}
	if state.mode != vimNormal || len(tab.state.Selections()) != 1 {
		t.Fatalf("block I cleanup state=%+v selections=%v", state, tab.state.Selections())
	}

	m, tab, state = newVimTestModel(t, text)
	tab.state.Select(1, 1)
	feedVim(t, m, tab, state, "<Ctrl+V>", "l", "j", "A")
	tab.state.Insert("Y")
	feedVim(t, m, tab, state, "<Esc>")
	if got := tab.state.Buffer.String(); got != "abcYd\nwxyYz\n1234\n" {
		t.Fatalf("block A = %q", got)
	}

	m, tab, state = newVimTestModel(t, text)
	tab.state.Select(1, 1)
	feedVim(t, m, tab, state, "<Ctrl+V>", "j", "c")
	if state.mode != vimInsert || len(tab.state.Selections()) != 2 {
		t.Fatalf("block c state=%+v selections=%v", state, tab.state.Selections())
	}
	tab.state.Insert("Q")
	feedVim(t, m, tab, state, "<Esc>")
	if got := tab.state.Buffer.String(); got != "aQcd\nwQyz\n1234\n" {
		t.Fatalf("block c = %q", got)
	}
}

func TestVimVisualBlockRaggedCollapseAndStaleRecovery(t *testing.T) {
	text := "abcd\nx\n1234\n"
	m, tab, state := newVimTestModel(t, text)
	tab.state.Select(2, 2)
	feedVim(t, m, tab, state, "<Ctrl+V>", "j", "d")
	if got := tab.state.Buffer.String(); got != "ad\nx\n1234\n" {
		t.Fatalf("ragged block delete = %q", got)
	}
	if state.register.text != "bc" {
		t.Fatalf("ragged yank register = %q", state.register.text)
	}

	m, tab, state = newVimTestModel(t, text)
	tab.state.Select(1, 1)
	feedVim(t, m, tab, state, "<Ctrl+V>", "v")
	if state.mode != vimNormal || tab.state.Cursor() != 1 {
		t.Fatalf("collapsed block state=%+v cursor=%d", state, tab.state.Cursor())
	}

	tab.state.Buffer = nil
	if !m.vimVisualBlockCommand(tab, state, "d") {
		t.Fatal("visual block command did not consume a missing-buffer key")
	}
}

func TestVimVisualBlockPutUsesColumnCountAndGrowsTheBuffer(t *testing.T) {
	m, tab, state := newVimTestModel(t, "xx\nxx\n")
	state.register = vimRegister{
		text: "a1\nb2", block: true, blockWidth: 2, set: true,
	}
	tab.state.Select(0, 0)
	feedVim(t, m, tab, state, "2", "p")
	if got := tab.state.Buffer.String(); got != "xa1a1x\nxb2b2x\n" {
		t.Fatalf("block put with count = %q", got)
	}

	m, tab, state = newVimTestModel(t, "xx\n")
	state.register = vimRegister{
		text: "a1\nb2", block: true, blockWidth: 2, set: true,
	}
	tab.state.Select(1, 1)
	feedVim(t, m, tab, state, "P")
	if got := tab.state.Buffer.String(); got != "xa1x\n b2" {
		t.Fatalf("block put beyond EOF = %q", got)
	}
}
