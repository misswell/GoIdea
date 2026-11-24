package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func newPluginEditorFixture(t *testing.T, text string) (*Model, *goideaPluginHost, *Tab) {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "Unicode.java")
	backend := &pluginWorkspaceBackend{info: openidea.DocInfo{
		ID: 41, Path: file, Text: text, Version: 7, Language: "java",
	}}
	m, reg, doc := pluginWorkspaceModel(t, root, backend)
	t.Cleanup(func() { _ = reg.Close(context.Background(), file) })
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m.tabs = []*Tab{tab}
	m.active = 0
	return m, &goideaPluginHost{m: m}, tab
}

func TestPluginEditorReportsActiveAndVisibleUTF16Selections(t *testing.T) {
	_, host, tab := newPluginEditorFixture(t, "第一😀\nsecond")
	anchor := len("第一😀")
	head := len("第")
	tab.state.SetSelections(editor.Selection{Anchor: anchor, Head: head})

	active := host.pluginActiveEditor()
	if active == nil || active.ID != 41 || active.Document["name"] != "Unicode.java" ||
		active.Document["lineCount"] != 2 {
		t.Fatalf("active editor = %#v", active)
	}
	if len(active.Selections) != 1 ||
		active.Selections[0].Anchor != (pluginTextPosition{Line: 0, Character: 4}) ||
		active.Selections[0].Active != (pluginTextPosition{Line: 0, Character: 1}) {
		t.Fatalf("active selections = %#v", active.Selections)
	}
	visible := host.pluginVisibleEditors()
	if len(visible) != 1 || visible[0].ID != active.ID {
		t.Fatalf("visible editors = %#v", visible)
	}
}

func TestPluginEditorEditUsesUTF16AndVersionedBuffer(t *testing.T) {
	_, host, tab := newPluginEditorFixture(t, "a😀b\n中文")
	version := tab.doc.Buffer().Snapshot().Version()
	changed, err := host.pluginEdit(41, []pluginTextEdit{{
		Range: pluginTextRange{
			Start: pluginTextPosition{Line: 0, Character: 3},
			End:   pluginTextPosition{Line: 0, Character: 4},
		},
		Text: "X",
	}}, version, []pluginSelection{{
		Anchor: pluginTextPosition{Line: 1, Character: 1},
		Active: pluginTextPosition{Line: 1, Character: 2},
	}})
	if err != nil || !changed {
		t.Fatalf("editor edit = %v, %v", changed, err)
	}
	if got := tab.doc.Text(); got != "a😀X\n中文" {
		t.Fatalf("edited text = %q", got)
	}
	selection := tab.state.Selection()
	if got := tab.doc.UTF16Pos(selection.Anchor); got != (openidea.OutlinePos{Line: 1, Col: 1}) {
		t.Fatalf("selection anchor = %+v", got)
	}
	if got := tab.doc.UTF16Pos(selection.Head); got != (openidea.OutlinePos{Line: 1, Col: 2}) {
		t.Fatalf("selection head = %+v", got)
	}
}

func TestPluginEditorEditRejectsAStaleVersion(t *testing.T) {
	_, host, tab := newPluginEditorFixture(t, "seed")
	if err := tab.doc.ApplyEdits(context.Background(), editor.Edit{
		Start: 0, End: len("seed"), Text: "original",
	}); err != nil {
		t.Fatal(err)
	}
	stale := tab.doc.Buffer().Snapshot().Version()
	if err := tab.doc.ApplyEdits(context.Background(), editor.Edit{
		Start: 0, End: len("original"), Text: "user edit",
	}); err != nil {
		t.Fatal(err)
	}
	changed, err := host.pluginEdit(41, []pluginTextEdit{{
		Range: pluginTextRange{
			Start: pluginTextPosition{},
			End:   pluginTextPosition{Character: len("user edit")},
		},
		Text: "plugin edit",
	}}, stale, nil)
	if err != nil || changed {
		t.Fatalf("stale edit = %v, %v", changed, err)
	}
	if got := tab.doc.Text(); got != "user edit" {
		t.Fatalf("stale edit changed text to %q", got)
	}
}

func TestPluginEditorInsertHandlesMultipleSelections(t *testing.T) {
	_, host, tab := newPluginEditorFixture(t, "one\ntwo")
	tab.state.SetSelections(
		editor.Selection{Anchor: 0, Head: 1},
		editor.Selection{Anchor: len("one\n"), Head: len("one\n")},
	)
	changed, err := host.pluginInsert(41, "X")
	if err != nil || !changed {
		t.Fatalf("editor insert = %v, %v", changed, err)
	}
	if got := tab.doc.Text(); got != "Xne\nXtwo" {
		t.Fatalf("inserted text = %q", got)
	}
}

func TestPluginEditorAPISelectsRevealsAndValidatesEditorID(t *testing.T) {
	m, host, tab := newPluginEditorFixture(t, "one\ntwo")
	m.active = -1
	got, err := host.Call(nil, "editor.active", nil)
	if err != nil || got != (*pluginEditorSnapshot)(nil) {
		t.Fatalf("inactive editor = %#v, %v", got, err)
	}
	m.active = 0
	if _, err := host.Call(nil, "editor.select", json.RawMessage(
		`{"editor":41,"selections":[{"anchor":{"line":0,"character":1},"active":{"line":1,"character":2}}]}`,
	)); err != nil {
		t.Fatal(err)
	}
	selection := tab.state.Selection()
	if selection.Anchor != len("o") || selection.Head != len("one\ntw") {
		t.Fatalf("selected offsets = %+v", selection)
	}
	m.active = -1
	if _, err := host.Call(nil, "editor.reveal", json.RawMessage(`{"editor":41,"line":1}`)); err != nil {
		t.Fatal(err)
	}
	if m.active != 0 || tab.state.CursorPos() != (editor.Pos{Line: 1}) {
		t.Fatalf("reveal active = %d, cursor = %+v", m.active, tab.state.CursorPos())
	}
	if _, err := host.Call(nil, "editor.reveal", json.RawMessage(`{"editor":999,"line":0}`)); err == nil {
		t.Fatal("a closed editor id was accepted")
	}
}
