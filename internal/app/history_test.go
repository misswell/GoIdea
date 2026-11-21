package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/textdiff"
)

func TestRevisionRangeUsesTheOldestAndNewestSnapshotOfOneFile(t *testing.T) {
	entries := []historyEntry{
		{ID: "new", Path: "/project/a.go", Ms: 3, Label: "Saved"},
		{ID: "middle", Path: "/project/a.go", Ms: 2, Label: "Saved"},
		{ID: "old", Path: "/project/a.go", Ms: 1, Label: "Before refactoring"},
	}
	older, newer := revisionRange(entries, map[int]bool{0: true, 1: true, 2: true})
	if older == nil || newer == nil || older.ID != "old" || newer.ID != "new" {
		t.Fatalf("revision range = %#v, %#v", older, newer)
	}
	other := append([]historyEntry(nil), entries...)
	other[1].Path = "/project/b.go"
	if older, newer := revisionRange(other, map[int]bool{0: true, 1: true}); older != nil || newer != nil {
		t.Fatal("snapshots of different files were compared")
	}
}

func TestHistoryPreviewPlansWorktreeDeletedAndLabelDiffs(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	file := filepath.Join(root, "A.java")
	h := &historyState{
		path: file,
		entries: []historyEntry{
			{ID: "old", Path: file, Ms: 10, Label: "Saved"},
			{ID: "label", Ms: 20, Label: "Before Git checkout", Kind: "label"},
		},
		sel: 0,
	}
	plan := m.planHistoryPreview(h)
	if plan.leftID != "old" || !plan.current || plan.deleted {
		t.Fatalf("snapshot plan = %+v", plan)
	}
	h.deleted = []string{file}
	plan = m.planHistoryPreview(h)
	if !plan.deleted {
		t.Fatalf("deleted snapshot plan = %+v", plan)
	}
	h.sel = 1
	plan = m.planHistoryPreview(h)
	if plan.leftID != "old" || !plan.current || !plan.deleted {
		t.Fatalf("label diff plan = %+v", plan)
	}
	h.folder = true
	plan = m.planHistoryPreview(h)
	if plan.leftID != "" || plan.message != "Label 'Before Git checkout' · 1 file recorded before it" {
		t.Fatalf("folder label plan = %+v", plan)
	}
}

func TestHistoryPreviewLoadsTwoRevisionContents(t *testing.T) {
	root := t.TempDir()
	done := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			select {
			case done <- struct{}{}:
			default:
			}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "history_content" {
				return json.RawMessage(`null`), nil
			}
			id := args.(map[string]any)["id"].(string)
			text := "old\n"
			if id == "new" {
				text = "new\n"
			}
			return json.Marshal(text)
		},
	}
	m.setRoot(root)
	path := filepath.Join(root, "A.java")
	if err := os.WriteFile(path, []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &historyState{
		open: true, path: path, sel: 0, anchor: 0,
		entries: []historyEntry{
			{ID: "new", Path: path, Ms: 20, Label: "Saved"},
			{ID: "old", Path: path, Ms: 10, Label: "Saved"},
		},
		extra: map[int]bool{1: true},
	}
	m.history = h
	m.refreshHistoryPreview(h)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("history content did not load")
	}
	if h.preview.err != "" || h.preview.loading {
		t.Fatalf("history preview = %+v", h.preview)
	}
	if h.preview.leftLabel == "" || h.preview.rightLabel == "" || len(h.preview.rows) == 0 {
		t.Fatalf("history preview has no labels or rows: %+v", h.preview)
	}
	foundChange := false
	for _, row := range h.preview.rows {
		if row.Kind == textdiff.Delete || row.Kind == textdiff.Add {
			foundChange = true
			break
		}
	}
	if !foundChange {
		t.Fatalf("history diff has no changed rows: %+v", h.preview.rows)
	}
}

func TestHistoryLoaderMergesFileRevisionsWithDirectoryLabels(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "A.java")
	if err := os.WriteFile(path, []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			select {
			case done <- struct{}{}:
			default:
			}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "history_list":
				return json.Marshal([]historyEntry{{
					ID: "snapshot", Path: path, Ms: 10, Label: "Saved", Kind: "snapshot",
				}})
			case "history_list_dir":
				return json.Marshal(directoryHistory{Entries: []historyEntry{
					{ID: "snapshot", Path: path, Ms: 10, Label: "Saved", Kind: "snapshot"},
					{ID: "", Path: root, Ms: 20, Label: "Before Git checkout", Kind: "label"},
				}})
			default:
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot(root)
	h := &historyState{open: true, path: path}
	m.history = h
	m.loadHistory(h)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("history did not load")
	}
	if len(h.entries) != 2 {
		t.Fatalf("merged history = %+v", h.entries)
	}
	if h.entries[0].Kind != "label" || h.entries[1].ID != "snapshot" {
		t.Fatalf("history was not merged and sorted newest first: %+v", h.entries)
	}
}

func TestHistoryDialogRendersRevisionListAndPreview(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "A.java")
	m := &Model{}
	m.setRoot(root)
	h := &historyState{
		open: true, path: path, sel: 0, anchor: 0,
		entries: []historyEntry{
			{ID: "new", Path: path, Ms: 20, Label: "Saved", Size: 42},
			{ID: "old", Path: path, Ms: 10, Label: "Before refactoring", Size: 30},
		},
		preview: historyPreview{
			leftLabel: "older", rightLabel: "Current", added: 1, gone: 1,
			rows: []textdiff.Row{
				{Kind: textdiff.Hunk},
				{Kind: textdiff.Delete, Old: 1, New: 1, Text: "old"},
				{Kind: textdiff.Add, Old: 2, New: 2, Text: "new"},
			},
		},
	}
	m.history = h
	tester := ui.NewTester(func(c *ui.Context) {
		m.historyDialog(c, c.Theme())
	}, 1100, 720)
	tester.Frame()
	for _, text := range []string{
		"Local History · A.java", "Revisions", "Saved",
		"Before refactoring", "older", "Current", "+1 −1", "Restore This Version",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Local History is missing %q: %v", text, tester.Texts())
		}
	}
}
