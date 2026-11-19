package app

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func TestParseGotoLine(t *testing.T) {
	for _, tc := range []struct {
		input     string
		line, col int
		ok        bool
	}{
		{"1", 0, 0, true}, {" 12 : 3 ", 11, 2, true}, {"2,4", 1, 3, true},
		{"0001:002", 0, 1, true}, {"", 0, 0, false}, {"0", 0, 0, false},
		{"1:0", 0, 0, false}, {"1:", 0, 0, false}, {"-1", 0, 0, false},
		{"1 2", 0, 0, false}, {"1:2:3", 0, 0, false},
		{"9999999999999999999999999999", int(^uint32(0)) - 1, 0, true},
	} {
		line, col, ok := parseGotoLine(tc.input)
		if line != tc.line || col != tc.col || ok != tc.ok {
			t.Errorf("parseGotoLine(%q) = %d, %d, %v; want %d, %d, %v", tc.input, line, col, ok, tc.line, tc.col, tc.ok)
		}
	}
}

func gotoLineTestModel(text string, offset int) *Model {
	doc := docbridge.Virtual("/test.java", text)
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(offset, offset)
	return &Model{tabs: []*Tab{{doc: doc, state: state}}}
}

func TestGotoLineSubmissionAndCancel(t *testing.T) {
	m := gotoLineTestModel("a😀b\nsecond\nlast", 5)
	m.OpenGotoLine()
	if m.gotoLine == nil || m.gotoLine.query != "1:4" {
		t.Fatalf("initial dialog = %+v", m.gotoLine)
	}
	m.gotoLine.query = "2:3"
	m.closeGotoLine()
	if got := m.tab().state.Cursor(); got != 5 {
		t.Fatalf("cancel moved caret to %d", got)
	}
	m.OpenGotoLine()
	m.gotoLine.query = "1:4"
	m.submitGotoLine()
	if got := m.tab().state.Cursor(); got != 5 {
		t.Fatalf("UTF-16 column moved caret to %d, want 5", got)
	}
	m.OpenGotoLine()
	m.gotoLine.query = "999999999999999999999:999999999999999999"
	m.submitGotoLine()
	if got := m.tab().state.Cursor(); got != len("a😀b\nsecond\nlast") {
		t.Fatalf("clamped caret = %d", got)
	}
	m.OpenGotoLine()
	before := m.tab().state.Cursor()
	m.gotoLine.query = "0:1"
	m.submitGotoLine()
	if m.gotoLine == nil || m.tab().state.Cursor() != before {
		t.Fatal("invalid input should keep dialog and caret")
	}
}

func TestGotoLineWithoutEditor(t *testing.T) {
	m := &Model{}
	m.OpenGotoLine()
	if m.gotoLine == nil || m.gotoLine.tab != nil || m.gotoLine.query != "" {
		t.Fatalf("dialog = %+v", m.gotoLine)
	}
	m.gotoLine.query = "1"
	m.submitGotoLine()
	if m.gotoLine == nil {
		t.Fatal("submission without an editor closed dialog")
	}
}
