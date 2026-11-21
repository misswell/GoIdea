package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func testPos(line, col uint32) workspacePosition {
	return workspacePosition{Line: line, Col: col}
}

func testRange(line, start, end uint32) workspaceRange {
	return workspaceRange{Start: testPos(line, start), End: testPos(line, end)}
}

func TestMergeFileMoveEditsMatchesOpenIDEA(t *testing.T) {
	versionOne, versionTwo := int32(1), int32(2)
	lsp := workspaceEdit{Files: []workspaceFileEdit{
		{
			Kind: "edit", Path: "/p/A.java", Version: &versionOne,
			Edits: []workspaceTextEdit{{Range: testRange(0, 0, 4), NewText: "New"}},
		},
		{
			Kind: "edit", Path: "/p/A.java", Version: &versionTwo,
			Edits: []workspaceTextEdit{{Range: testRange(1, 0, 4), NewText: "Second"}},
		},
		{
			Kind: "edit", Path: "/p/B.java",
			Edits: []workspaceTextEdit{{Range: testRange(2, 3, 6), NewText: "B"}},
		},
	}}
	text := emptyWorkspaceEdit()
	text.Files = []workspaceFileEdit{{
		Kind: "edit", Path: "/p/A.java",
		Edits: []workspaceTextEdit{
			{Range: testRange(0, 1, 3), NewText: "duplicate"},
			{Range: testRange(3, 0, 1), NewText: "text"},
		},
	}}
	text.Preconditions = []workspacePrecondition{{Path: "/p/A.java", Hash: "guard"}}

	got := mergeFileMoveEdits(lsp, text)
	if len(got.Files) != 3 {
		t.Fatalf("files = %+v, want guard plus two edits", got.Files)
	}
	guard := got.Files[0]
	if guard.Path != "/p/A.java" || guard.Version == nil || *guard.Version != 1 ||
		len(guard.Edits) != 0 {
		t.Fatalf("version guard = %+v", guard)
	}
	var aEdits []workspaceTextEdit
	for _, file := range got.Files[1:] {
		if file.Path == "/p/A.java" {
			aEdits = file.Edits
		}
	}
	if len(aEdits) != 3 || aEdits[2].NewText != "text" {
		t.Fatalf("A.java edits = %+v, overlapping text hit should be removed", aEdits)
	}
	if len(got.Preconditions) != 1 || got.Preconditions[0].Hash != "guard" {
		t.Fatalf("preconditions = %+v", got.Preconditions)
	}
}

func TestFileRefactorPreviewIncludesOnlyOtherFiles(t *testing.T) {
	edit := emptyWorkspaceEdit()
	edit.Files = []workspaceFileEdit{
		{Kind: "edit", Path: "/p/src/Foo.java", Edits: []workspaceTextEdit{{NewText: "ignored"}}},
		{Kind: "edit", Path: "/p/src/Bar.java", Edits: []workspaceTextEdit{{NewText: "changed"}}},
		{Kind: "delete", Path: "/p/obsolete"},
	}
	if !fileRefactorTouchesOtherFiles(edit, []treeFileMove{{
		From: "/p/src/Foo.java", To: "/p/lib/Foo.java",
	}}) {
		t.Fatal("a reference in Bar.java must open Refactoring Preview")
	}
	onlyMoved := emptyWorkspaceEdit()
	onlyMoved.Files = edit.Files[:1]
	if fileRefactorTouchesOtherFiles(onlyMoved, []treeFileMove{{
		From: "/p/src/Foo.java", To: "/p/lib/Foo.java",
	}}) {
		t.Fatal("edits inside the moved file alone do not need a preview")
	}
}

func TestMovedFilePathHandlesDirectories(t *testing.T) {
	moves := []treeFileMove{{From: "/p/src", To: "/p/module/src"}}
	if got := movedFilePath("/p/src/com/App.java", moves); got != "/p/module/src/com/App.java" {
		t.Fatalf("moved path = %q", got)
	}
	if got := movedFilePath("/p/test/AppTest.java", moves); got != "/p/test/AppTest.java" {
		t.Fatalf("unrelated path = %q", got)
	}
}

func TestProjectSelectionDropsNestedEntriesAndPlansMultiCopy(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "one")
	secondDir := filepath.Join(root, "two")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(firstDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(firstDir, "App.java"),
		filepath.Join(firstDir, "nested", "Nested.java"),
		filepath.Join(secondDir, "App.java"),
	} {
		if err := os.WriteFile(path, []byte("class App {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &Model{}
	m.setRoot(root)
	top := m.topLevelTreePaths([]string{
		filepath.Join(firstDir, "App.java"),
		firstDir,
		filepath.Join(firstDir, "nested", "Nested.java"),
		filepath.Join(secondDir, "App.java"),
		firstDir,
	})
	if !equalStrings(top, []string{firstDir, filepath.Join(secondDir, "App.java")}) {
		t.Fatalf("top-level paths = %v", top)
	}
	d := &fileRefactorDialogState{
		kind: "copy", path: top[1], target: m.displayRefactorTarget(target),
		paths: []string{
			filepath.Join(firstDir, "App.java"),
			filepath.Join(secondDir, "App.java"),
		},
	}
	copies, err := m.copyDialogPlan(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 2 ||
		copies[0].To != filepath.Join(target, "App.java") ||
		copies[1].To != filepath.Join(target, "App copy.java") {
		t.Fatalf("copies = %+v", copies)
	}
}

func TestProjectMoveDialogPlansAllSelectedFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src")
	destination := filepath.Join(root, "lib")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(source, "First.java")
	second := filepath.Join(source, "Second.java")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &Model{}
	m.setRoot(root)
	m.OpenFileMoveDialogFor([]string{first, second}, destination, false)
	if m.fileRefactor == nil || len(m.fileRefactor.paths) != 2 {
		t.Fatalf("move dialog = %+v", m.fileRefactor)
	}
	moves, err := m.refactorDialogMoves(m.fileRefactor)
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 2 ||
		moves[0].To != filepath.Join(destination, "First.java") ||
		moves[1].To != filepath.Join(destination, "Second.java") {
		t.Fatalf("moves = %+v", moves)
	}
}

func TestProjectMoveDialogValidatesTargetAndUsesRefactoring(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src", "Foo.java")
	other := filepath.Join(root, "src", "Uses.java")
	destination := filepath.Join(root, "lib")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{source, other} {
		if err := os.WriteFile(path, []byte("package p;\nclass Foo {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	var commands []string
	moved := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		prefs: Preferences{ProjectTree: ProjectTreePrefs{
			MoveRefactor: FileRefactorPrefs{OpenInEditor: boolPointer(false)},
		}},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			mu.Lock()
			commands = append(commands, command)
			mu.Unlock()
			switch command {
			case "refactor_file_move_edits":
				return json.RawMessage(`{
					"edit":{"files":[{"kind":"edit","path":"` + filepath.ToSlash(other) + `",
					"edits":[{"range":{"start":{"line":0,"col":8},"end":{"line":0,"col":11}},
					"new_text":"Bar"}]}],"preconditions":[]},
					"servers":["java"],"errors":[]}`), nil
			case "refactor_file_move_text_edits":
				return json.RawMessage(`{"files":[],"preconditions":[]}`), nil
			case "refactor_move_files":
				return json.RawMessage(`19`), nil
			default:
				t.Fatalf("unexpected RPC %s", command)
				return nil, nil
			}
		},
	}
	m.postHook = func(fn func()) {
		fn()
		if m.treeLastMove != nil {
			select {
			case moved <- struct{}{}:
			default:
			}
		}
	}
	m.setRoot(root)
	m.OpenFileMoveDialog(source, destination, false)
	if m.fileRefactor == nil || m.fileRefactor.kind != "move" {
		t.Fatalf("move dialog = %+v", m.fileRefactor)
	}
	m.ConfirmFileRefactor()

	deadline := time.Now().Add(2 * time.Second)
	for m.fileRefactorPreview == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if m.fileRefactorPreview == nil {
		t.Fatal("move did not open Refactoring Preview")
	}
	if len(m.fileRefactorPreview.files) != 1 ||
		!strings.HasSuffix(m.fileRefactorPreview.files[0].Path, "Uses.java") {
		t.Fatalf("preview files = %+v", m.fileRefactorPreview.files)
	}
	m.ConfirmFileRefactorPreview()
	select {
	case <-moved:
	case <-time.After(2 * time.Second):
		t.Fatal("move did not apply")
	}
	if m.treeLastMove == nil || m.treeLastMove.ID != 19 ||
		m.treeLastMove.From != source ||
		m.treeLastMove.To != filepath.Join(destination, "Foo.java") {
		t.Fatalf("undo record = %+v", m.treeLastMove)
	}
	mu.Lock()
	defer mu.Unlock()
	seen := map[string]bool{}
	for _, command := range commands {
		seen[command] = true
	}
	for _, command := range []string{
		"refactor_file_move_edits", "refactor_file_move_text_edits", "refactor_move_files",
	} {
		if !seen[command] {
			t.Fatalf("commands = %v, missing %s", commands, command)
		}
	}
}

func TestRenameDialogRejectsNameConflicts(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "First.java")
	second := filepath.Join(root, "Second.java")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &Model{}
	m.setRoot(root)
	m.OpenFileRenameDialog(first)
	m.fileRefactor.name = "Second.java"
	if _, err := m.refactorDialogMove(m.fileRefactor); err == nil {
		t.Fatal("rename over an existing sibling must be rejected")
	}
	m.fileRefactor.name = "Renamed.java"
	move, err := m.refactorDialogMove(m.fileRefactor)
	if err != nil {
		t.Fatal(err)
	}
	if move.To != filepath.Join(root, "Renamed.java") {
		t.Fatalf("rename target = %q", move.To)
	}
}

func TestProjectFileRefactorOverlaysRenderNativeDialogs(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src", "Foo.java")
	destination := filepath.Join(root, "lib")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("class Foo {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Model{}
	m.setRoot(root)
	m.OpenFileMoveDialog(source, destination, false)
	dialog := overlays.byID["project.fileRefactor"]
	if dialog == nil || !dialog.Modal || !dialog.Open(m) {
		t.Fatalf("project.fileRefactor overlay = %+v", dialog)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		dialog.Render(m, c, c.Theme())
	}, 960, 720)
	tester.Frame()
	for _, want := range []string{
		"Move", "Move file 'Foo.java'", "To directory:", "Search for references",
		"Search in comments and strings", "Open moved files in editor", "Refactor",
	} {
		if !tester.HasText(want) {
			t.Errorf("Move dialog lacks %q: %q", want, tester.Texts())
		}
	}

	m.fileRefactor = nil
	m.fileRefactorPreview = &fileRefactorPreviewState{
		title: "Move Foo.java",
		files: []fileRefactorPreviewFile{{
			Path: filepath.Join(root, "Uses.java"), Relative: "src/Uses.java",
			Name: "Uses.java", Enabled: true,
			Edits: []fileRefactorPreviewEdit{{
				Line: 4, Pre: "new ", Old: "Foo", New: "Bar", Post: "();",
			}},
		}},
	}
	preview := overlays.byID["project.refactoringPreview"]
	if preview == nil || !preview.Modal || !preview.Open(m) {
		t.Fatalf("project.refactoringPreview overlay = %+v", preview)
	}
	tester = ui.NewTester(func(c *ui.Context) {
		preview.Render(m, c, c.Theme())
	}, 960, 720)
	tester.Frame()
	for _, want := range []string{
		"Refactoring Preview: Move Foo.java", "References to update: 1 occurrence in 1 file",
		"Uses.java", "src/Uses.java", "Foo", "Bar", "Do Refactor",
	} {
		if !tester.HasText(want) {
			t.Errorf("Refactoring Preview lacks %q: %q", want, tester.Texts())
		}
	}
}
