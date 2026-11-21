package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

type fileActionBackend struct {
	info openidea.DocInfo
}

func (b *fileActionBackend) DocOpen(context.Context, string, string) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *fileActionBackend) DocEdit(context.Context, string, uint32, uint32, position.ChangeSet) (uint32, error) {
	return 0, errors.New("unexpected edit")
}

func (b *fileActionBackend) DocSnapshot(context.Context, string, uint32) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *fileActionBackend) DocSave(context.Context, string, uint32) error {
	return nil
}

func (b *fileActionBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func fileActionDocument(t *testing.T, readOnly bool) *docbridge.Document {
	t.Helper()
	backend := &fileActionBackend{info: openidea.DocInfo{
		ID:       7,
		Path:     "/project/src/Main.java",
		Text:     "class Main {}\n",
		Version:  3,
		ReadOnly: readOnly,
	}}
	doc, err := docbridge.Open(context.Background(), backend, "w1", backend.info.Path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestToggleReadOnlyRequiresARealFileDocument(t *testing.T) {
	m := &Model{}
	if m.ActionEnabled("file.toggleReadOnly") {
		t.Fatal("enabled without a tab")
	}

	virtual := docbridge.Virtual("What's New", "notes")
	m.tabs = []*Tab{{doc: virtual, state: &editor.State{Buffer: virtual.Buffer()}}}
	m.active = 0
	if m.ActionEnabled("file.toggleReadOnly") {
		t.Fatal("enabled for a virtual document")
	}

	doc := fileActionDocument(t, false)
	m.tabs[0] = &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}, kind: tabKindWhatsNew}
	if m.ActionEnabled("file.toggleReadOnly") {
		t.Fatal("enabled for an internal page")
	}

	m.tabs[0].kind = ""
	if !m.ActionEnabled("file.toggleReadOnly") {
		t.Fatal("disabled for a real file document")
	}
}

func TestSetFileWritableUpdatesEveryTabSharingTheDocument(t *testing.T) {
	doc := fileActionDocument(t, false)
	main := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	split := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	finished := make(chan struct{}, 1)
	calls := make(chan string, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{main},
		split: &splitGroup{
			tabs: []*Tab{split},
		},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			input := args.(map[string]any)
			if input["path"] != doc.Path() || input["writable"] != true {
				t.Errorf("%s args = %#v", command, input)
			}
			calls <- command
			return json.RawMessage(`true`), nil
		},
	}
	m.setFileWritable(doc, true)
	select {
	case command := <-calls:
		if command != "fs_set_writable" {
			t.Fatalf("command = %q", command)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fs_set_writable was not called")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("read-only result was not applied")
	}
	if !doc.ReadOnly() || !main.state.ReadOnly || !split.state.ReadOnly {
		t.Fatalf("read-only = doc %v, tabs %v/%v", doc.ReadOnly(), main.state.ReadOnly, split.state.ReadOnly)
	}
	if m.status != "Main.java is read-only" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestSetFileWritableFailureLeavesTheDocumentEditable(t *testing.T) {
	doc := fileActionDocument(t, false)
	state := &editor.State{Buffer: doc.Buffer()}
	finished := make(chan struct{}, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{{doc: doc, state: state}},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return nil, errors.New("permission denied")
		},
	}
	m.setFileWritable(doc, true)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("read-only failure was not reported")
	}
	if doc.ReadOnly() || state.ReadOnly {
		t.Fatal("failed RPC changed the document state")
	}
	if !strings.Contains(m.status, "Cannot make Main.java writable: permission denied") {
		t.Fatalf("status = %q", m.status)
	}
}
