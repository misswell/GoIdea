package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestNewEmptyFileDefersEncodingAndBOMToBackendCreateFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	var command string
	var args map[string]any
	finished := make(chan struct{})
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			close(finished)
		},
		invokeHook: func(_ context.Context, got string, input any) (json.RawMessage, error) {
			command = got
			args, _ = input.(map[string]any)
			return nil, errors.New("stop")
		},
	}
	m.setRoot(root)
	m.treeOp = &treeOpState{
		kind: treeOpNewFile, path: root, dir: true, name: "notes.txt", open: true,
	}
	m.ConfirmTreeOp()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("empty file creation did not finish")
	}
	if command != "fs_create_file" {
		t.Fatalf("command = %q, want fs_create_file", command)
	}
	if want := map[string]any{"path": target}; !reflect.DeepEqual(args, want) {
		t.Fatalf("fs_create_file arguments = %#v, want %#v", args, want)
	}
}

func TestProjectTreeOpenIDEAActionsAreRegistered(t *testing.T) {
	ids := []string{
		"file.new",
		"file.newProject",
		"project.newFile",
		"project.newFolder",
		"project.newInThisDirectory",
		"project.copy",
		"project.cut",
		"project.paste",
		"project.copyElement",
		"project.rename",
		"project.delete",
		"project.copyAbsolutePath",
		"project.copyRelativePath",
		"project.copyFileName",
		"project.copySourceRootPath",
		"project.move",
		"project.undoMove",
		"project.optimizeImports",
		"navigate.showFilePath",
	}
	for _, id := range ids {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA project action %q", id)
		}
	}
}

func TestProjectTreeNewAndClipboardUseTheSelection(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "src", "Main.java")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("class Main {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{tree: &Tree{sel: file}}
	m.setRoot(root)

	m.NewFileAtProjectSelection()
	if m.projectNew == nil || m.projectNew.dir != filepath.Dir(file) || m.treeOp != nil {
		t.Fatalf("New popup = %+v, prompt = %+v", m.projectNew, m.treeOp)
	}
	m.CloseProjectNew()
	m.NewDirectoryAtProjectSelection()
	if m.treeOp == nil || m.treeOp.kind != treeOpNewDir ||
		m.treeOp.path != filepath.Dir(file) {
		t.Fatalf("New Directory prompt = %+v", m.treeOp)
	}
	m.CloseTreeOp()

	m.CopySelectedTreeEntry()
	if m.treeClip == nil || m.treeClip.path != file || m.treeClip.cut {
		t.Fatalf("Copy = %+v", m.treeClip)
	}
	m.CutSelectedTreeEntry()
	if m.treeClip == nil || m.treeClip.path != file || !m.treeClip.cut {
		t.Fatalf("Cut = %+v", m.treeClip)
	}
}

func TestReferenceAwareTreeMoveUsesOpenIDEARPC(t *testing.T) {
	var mu sync.Mutex
	var commands []string
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			mu.Lock()
			commands = append(commands, command)
			mu.Unlock()
			switch command {
			case "refactor_file_move_edits":
				return json.RawMessage(`{"edit":{"files":[],"preconditions":[]}}`), nil
			case "refactor_file_move_text_edits":
				return json.RawMessage(`{"files":[],"preconditions":[]}`), nil
			case "refactor_move_files":
				return json.RawMessage(`7`), nil
			default:
				t.Fatalf("unexpected command %q: %#v", command, input)
				return nil, nil
			}
		},
	}
	done := make(chan error, 1)
	m.applyTreeMove("/p/Old.java", "/p/src/New.java", "Move Old.java", func(err error) {
		done <- err
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("move did not finish")
	}
	if len(commands) != 3 || commands[2] != "refactor_move_files" {
		t.Fatalf("commands = %v", commands)
	}
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
	if m.treeLastMove == nil || m.treeLastMove.ID != 7 ||
		m.treeLastMove.From != "/p/Old.java" || m.treeLastMove.To != "/p/src/New.java" {
		t.Fatalf("undo record = %+v", m.treeLastMove)
	}
}

func TestProjectTreeCopyUsesFinderStyleUniqueNames(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "Foo.kt"),
		filepath.Join(root, "Foo copy.kt"),
	} {
		if err := os.WriteFile(path, []byte("class Foo"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src copy"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	if got := m.uniqueTreeCopyName(root, "Foo.kt", false); got != "Foo copy 2.kt" {
		t.Fatalf("file copy name = %q", got)
	}
	if got := m.uniqueTreeCopyName(root, "src", true); got != "src copy 2" {
		t.Fatalf("directory copy name = %q", got)
	}
	if got := m.uniqueTreeCopyName(root, ".gitignore", false); got != ".gitignore" {
		t.Fatalf("dotfile copy name = %q", got)
	}
}

func TestProjectTreeDropPlanMatchesOpenIDEAMoveAndCopyRules(t *testing.T) {
	root := "/p"
	source := filepath.Join(root, "src", "Main.java")
	inPlace := filepath.Join(root, "src")
	other := filepath.Join(root, "test")
	if got := treeDropPlan([]string{source}, inPlace, false); len(got) != 0 {
		t.Fatalf("move to the current parent = %v", got)
	}
	if got := treeDropPlan([]string{source}, inPlace, true); !equalStrings(got, []string{source}) {
		t.Fatalf("copy to the current parent = %v", got)
	}
	if got := treeDropPlan([]string{source}, other, false); !equalStrings(got, []string{source}) {
		t.Fatalf("move to another folder = %v", got)
	}
	folder := filepath.Join(root, "src")
	if got := treeDropPlan([]string{folder}, filepath.Join(folder, "child"), false); len(got) != 0 {
		t.Fatalf("move into the dragged folder = %v", got)
	}
}

func TestProjectTreeDropOpensTheMatchingRefactorDialog(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "Main.java")
	target := filepath.Join(root, "src")
	if err := os.WriteFile(source, []byte("class Main {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}

	m := &Model{tree: &Tree{sel: source, sels: map[string]bool{source: true}}}
	m.setRoot(root)
	m.treeDrag = &treeDragState{
		active: true, paths: []string{source}, targetPath: target,
	}
	m.finishTreeDrag()
	if m.fileRefactor == nil || m.fileRefactor.kind != "move" ||
		!equalStrings(m.fileRefactor.paths, []string{source}) {
		t.Fatalf("drop move dialog = %+v", m.fileRefactor)
	}

	m.fileRefactor = nil
	m.treeDrag = &treeDragState{
		active: true, copy: true, paths: []string{source}, targetPath: target,
	}
	m.finishTreeDrag()
	if m.fileRefactor == nil || m.fileRefactor.kind != "copy" ||
		!equalStrings(m.fileRefactor.paths, []string{source}) {
		t.Fatalf("Alt-drop copy dialog = %+v", m.fileRefactor)
	}
}

func TestRunTreeFileCopiesKeepsGoingAfterAFailure(t *testing.T) {
	var mu sync.Mutex
	var calls []treeFileCopy
	done := make(chan treeCopyOutcome, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "fs_copy" {
				t.Errorf("unexpected command %s", command)
			}
			args := input.(map[string]any)
			copy := treeFileCopy{
				From: args["from"].(string),
				To:   args["to"].(string),
			}
			mu.Lock()
			calls = append(calls, copy)
			mu.Unlock()
			if filepath.Base(copy.From) == "bad.txt" {
				return nil, context.DeadlineExceeded
			}
			return json.RawMessage(`null`), nil
		},
	}
	copies := []treeFileCopy{
		{From: "/src/a.txt", To: "/dst/a.txt"},
		{From: "/src/bad.txt", To: "/dst/bad.txt"},
		{From: "/src/c.txt", To: "/dst/c.txt"},
	}
	m.runTreeFileCopies(copies, "/dst", false, func(outcome treeCopyOutcome) {
		done <- outcome
	})
	select {
	case outcome := <-done:
		if len(outcome.Copied) != 2 || len(outcome.Failures) != 1 {
			t.Fatalf("copy outcome = %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("copy sequence did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("copied %d of 3 files: %+v", len(calls), calls)
	}
	for index := range copies {
		if calls[index] != copies[index] {
			t.Fatalf("copy %d = %+v, want %+v", index, calls[index], copies[index])
		}
	}
}

func TestExternalFilesDropCopiesIntoTheTargetFolder(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	source := filepath.Join(external, "Dropped.java")
	target := filepath.Join(root, "src")
	if err := os.WriteFile(source, []byte("class Dropped {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}

	done := make(chan treeFileCopy, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "fs_copy" {
				t.Errorf("unexpected command %s", command)
			}
			args := input.(map[string]any)
			done <- treeFileCopy{
				From: args["from"].(string),
				To:   args["to"].(string),
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(root)
	m.pasteExternalTreeFiles([]string{source}, target)
	select {
	case copy := <-done:
		if copy.From != source || copy.To != filepath.Join(target, "Dropped.java") {
			t.Fatalf("external copy = %+v", copy)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("external drop did not copy")
	}
}

func TestProjectTreeFocusRoutesStandardEditCommands(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "A.java")
	second := filepath.Join(root, "B.java")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &Model{
		projectTreeFocus: true,
		tree: &Tree{
			sel:  first,
			sels: map[string]bool{first: true},
			rows: []*treeNode{
				{path: root, dir: true},
				{path: first},
				{path: second},
			},
		},
	}
	m.setRoot(root)
	if !m.RunCommand("edit.copy") || m.treeClip == nil ||
		!equalStrings(m.treeClip.paths, []string{first}) || m.treeClip.cut {
		t.Fatalf("project-tree Copy = %+v", m.treeClip)
	}
	if !m.RunCommand("edit.selectAll") ||
		!equalStrings(m.tree.SelectedPaths(), []string{root, first, second}) {
		t.Fatalf("project-tree Select All = %v", m.tree.SelectedPaths())
	}
	m.tree.sels = map[string]bool{first: true, second: true}
	m.tree.sel = first
	if !m.RunCommand("edit.cut") || m.treeClip == nil || !m.treeClip.cut ||
		!equalStrings(m.treeClip.paths, []string{first, second}) {
		t.Fatalf("project-tree Cut = %+v", m.treeClip)
	}

	m.tabs = []*Tab{{}}
	m.active = 0
	m.FocusEditor()
	if m.projectTreeFocus {
		t.Fatal("focus entering the editor did not clear the Project View focus")
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
