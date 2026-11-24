package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func menuItem(t *testing.T, items []treeMenuItem, label string) treeMenuItem {
	t.Helper()
	for _, it := range items {
		if it.Label == label {
			return it
		}
	}
	t.Fatalf("menu has no %q: %v", label, treeMenuLabels(items))
	return treeMenuItem{}
}

func TestTreeMenuFollowsTheSelection(t *testing.T) {
	root := treeTarget{path: "/p", dir: true, root: true}
	items := treeMenuItems(root, false)
	if got := menuItem(t, items, "New"); len(got.Items) != 2 ||
		got.Items[0].Label != "File…" || got.Items[1].Label != "Directory…" {
		t.Fatalf("New submenu = %+v", got.Items)
	}
	if menuItem(t, items, "Cut").Enabled {
		t.Fatal("the project root can be cut")
	}
	if menuItem(t, items, "Paste").Enabled {
		t.Fatal("paste is enabled with an empty clipboard")
	}
	refactor := menuItem(t, items, "Refactor")
	if menuItem(t, refactor.Items, "Rename…").Enabled ||
		menuItem(t, refactor.Items, "Move Directory…").Enabled ||
		menuItem(t, refactor.Items, "Copy Directory…").Enabled ||
		menuItem(t, items, "Delete…").Enabled {
		t.Fatal("the project root can be renamed or deleted")
	}
	if menuItem(t, items, "Jump to Source").Enabled {
		t.Fatal("a folder can jump to source")
	}
	openIn := menuItem(t, items, "Open In")
	if !menuItem(t, openIn.Items, "Terminal").Enabled {
		t.Fatal("a folder cannot open a terminal")
	}
	if menuItem(t, items, "Send to Agent").Enabled {
		t.Fatal("the project root can be sent to an Agent")
	}

	file := treeTarget{path: "/p/A.java"}
	items = treeMenuItems(file, true)
	if !menuItem(t, items, "Cut").Enabled || !menuItem(t, items, "Paste").Enabled {
		t.Fatal("a file or a filled clipboard is disabled")
	}
	if !menuItem(t, items, "Jump to Source").Enabled {
		t.Fatal("a file cannot jump to source")
	}
	refactor = menuItem(t, items, "Refactor")
	if !menuItem(t, refactor.Items, "Copy File…").Enabled {
		t.Fatal("a file cannot open the Copy dialog")
	}
	if !menuItem(t, items, "Send to Agent").Enabled {
		t.Fatal("a project file cannot be sent to an Agent")
	}

	html := treeTarget{path: "/p/web/index.html"}
	items = treeMenuItems(html, false)
	browser := menuItem(t, menuItem(t, items, "Open In").Items, "Browser")
	if !menuItem(t, browser.Items, "Default Browser").Enabled ||
		!menuItem(t, browser.Items, "Built-in Preview").Enabled {
		t.Fatalf("HTML Open In›Browser = %+v", browser.Items)
	}
}

func TestTreeMenuUsesTheMatchingComparisonAction(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "A.java")
	second := filepath.Join(root, "B.java")
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{left, right} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := []treeTarget{{path: first}, {path: second}}
	items := treeMenuItems(files[0], false, files...)
	if got := menuItem(t, items, "Compare Files"); !got.Enabled {
		t.Fatalf("Compare Files is disabled: %+v", items)
	}
	labels := strings.Join(treeMenuLabels(items), "|")
	if strings.Contains(labels, "Compare With…") || strings.Contains(labels, "Compare with Clipboard") {
		t.Fatalf("two-file menu also shows single-file comparison actions: %v", treeMenuLabels(items))
	}

	directories := []treeTarget{{path: left, dir: true}, {path: right, dir: true}}
	items = treeMenuItems(directories[0], false, directories...)
	if got := menuItem(t, items, "Compare Directories…"); !got.Enabled {
		t.Fatalf("Compare Directories is disabled: %+v", items)
	}

	items = treeMenuItems(treeTarget{path: first}, false)
	if got := menuItem(t, items, "Compare With…"); !got.Enabled {
		t.Fatalf("single-file comparison is disabled: %+v", items)
	}
	if got := menuItem(t, items, "Compare with Clipboard"); !got.Enabled {
		t.Fatalf("single-file clipboard comparison is disabled: %+v", items)
	}

	mixed := []treeTarget{{path: first}, {path: right, dir: true}}
	items = treeMenuItems(mixed[0], false, mixed...)
	labels = strings.Join(treeMenuLabels(items), "|")
	if strings.Contains(labels, "Compare Files") ||
		strings.Contains(labels, "Compare Directories") ||
		strings.Contains(labels, "Compare With…") ||
		strings.Contains(labels, "Compare with Clipboard") {
		t.Fatalf("mixed selection received a comparison action: %v", labels)
	}
}

func TestTreeCompareFilesDispatchesBothSelectedPaths(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "A.java")
	second := filepath.Join(root, "B.java")
	if err := os.WriteFile(first, []byte("class A {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("class B {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{}, 1)
	m := &Model{
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		tree: &Tree{
			sel:  first,
			sels: map[string]bool{first: true, second: true},
			rows: []*treeNode{{path: first}, {path: second}},
		},
	}
	m.setRoot(root)
	m.runTreeAct(nil, treeMenuItem{Act: treeActCompareFiles}, treeTarget{path: first})
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("Compare Files did not finish reading both selections")
	}
	if m.diff == nil || m.diff.leftPath != realPath(first) || m.diff.rightPath != realPath(second) {
		t.Fatalf("Compare Files dispatch = %+v", m.diff)
	}
}

func TestTreeCompareWithDispatchesSingleDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "left")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	m.setRoot(root)
	m.runTreeAct(nil, treeMenuItem{Act: treeActCompareWith}, treeTarget{path: dir, dir: true})
	if m.dirCompare == nil || !m.dirCompare.open || !m.dirCompare.asking ||
		m.dirCompare.left != realPath(dir) {
		t.Fatalf("Compare With dispatch = %+v", m.dirCompare)
	}
}

func TestTreeMenuLabelsFlattenSubmenus(t *testing.T) {
	labels := treeMenuLabels(treeMenuItems(treeTarget{path: "/p", dir: true, root: true}, false))
	joined := strings.Join(labels, "|")
	for _, want := range []string{
		"New/File…/Directory…",
		"Copy Path/Reference/Absolute Path/File Name/Path From Content Root",
		"Refactor/Rename…/Move Directory…/Copy Directory…",
		"Delete…",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("labels %q miss %q", joined, want)
		}
	}
}

func TestTreeGitSubmenuNeedsARepository(t *testing.T) {
	labels := treeMenuLabels(treeMenuItems(treeTarget{path: "/p", dir: true, root: true}, false))
	if strings.Contains(strings.Join(labels, "|"), "Git") {
		t.Fatalf("the Git submenu shows without a Git repository: %v", labels)
	}
}

func TestTreeGitSubmenuFollowsTheSelection(t *testing.T) {
	root := treeTarget{path: "/p", dir: true, root: true, git: true}
	items := menuItem(t, treeMenuItems(root, false), "Git").Items
	if !menuItem(t, items, "Commit…").Enabled {
		t.Fatal("a folder cannot commit")
	}
	ignore := menuItem(t, items, "Add to .gitignore")
	for _, dest := range []string{".gitignore", ".git/info/exclude"} {
		if menuItem(t, ignore.Items, dest).Enabled {
			t.Fatalf("the project root can be added to %s", dest)
		}
	}
	if menuItem(t, items, "Annotate with Git Blame").Enabled {
		t.Fatal("a folder can be annotated")
	}
	if menuItem(t, items, "Add").Enabled {
		t.Fatal("a folder can be added to VCS")
	}
	if !menuItem(t, items, "Branches…").Enabled {
		t.Fatal("the branches popup is disabled")
	}
	for _, label := range []string{
		"Compare with Branch or Revision…", "Checkout Tag or Revision…",
		"Worktrees…", "New Worktree…", "Manage Remotes…", "Resolve Conflicts…",
	} {
		if !menuItem(t, items, label).Enabled {
			t.Fatalf("%s is disabled for a repository", label)
		}
	}

	file := treeTarget{path: "/p/src/A.java", git: true, kind: "modified", rollback: true}
	items = menuItem(t, treeMenuItems(file, false), "Git").Items
	if !menuItem(t, items, "Commit File…").Enabled || strings.Contains(strings.Join(treeMenuLabels(items), "|"), "Commit…") {
		t.Fatal("a file commits itself alone")
	}
	if menuItem(t, items, "Add").Enabled {
		t.Fatal("a modified file can be added to VCS")
	}
	if !menuItem(t, items, "Annotate with Git Blame").Enabled || !menuItem(t, items, "Show Diff").Enabled {
		t.Fatal("a file cannot be annotated or diffed")
	}
	if !menuItem(t, items, "Rollback…").Enabled {
		t.Fatal("a working-tree change cannot be rolled back")
	}
	if !menuItem(t, items, "Compare with Revision…").Enabled ||
		!menuItem(t, items, "Show Current Revision").Enabled ||
		!menuItem(t, items, "Show History for Selection").Enabled {
		t.Fatal("a file is missing its revision and selection history actions")
	}
	untracked := treeTarget{path: "/p/src/New.java", git: true, kind: "untracked"}
	items = menuItem(t, treeMenuItems(untracked, false), "Git").Items
	if !menuItem(t, items, "Add").Enabled {
		t.Fatal("an unversioned file cannot be added")
	}
	if menuItem(t, items, "Rollback…").Enabled {
		t.Fatal("an unversioned file has no working-tree change to roll back")
	}
}

func TestUnderTreePathKeepsSiblingsApart(t *testing.T) {
	base := filepath.Join("/p", "src")
	if !underTreePath(base, base) {
		t.Fatal("a path is not under itself")
	}
	if !underTreePath(filepath.Join(base, "A.java"), base) {
		t.Fatal("a file inside the folder is not under it")
	}
	if underTreePath(filepath.Join("/p", "src2"), base) {
		t.Fatal("a sibling folder counts as inside")
	}
}

func TestTreeSelectionAfterDeleteMatchesOpenIDEA(t *testing.T) {
	rows := []*treeNode{
		{path: "/p/a"},
		{path: "/p/folder", dir: true},
		{path: "/p/folder/child"},
		{path: "/p/c"},
	}
	if got := treeSelectionAfterDelete(rows, []string{"/p/folder"}); got != "/p/c" {
		t.Fatalf("next selection = %q", got)
	}
	if got := treeSelectionAfterDelete(rows[:3], []string{"/p/folder"}); got != "/p/a" {
		t.Fatalf("previous selection = %q", got)
	}
}

func TestTreeDeleteUsesSelectionAndCreatesUndoRecord(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "First.java")
	second := filepath.Join(root, "Second.java")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{}, 1)
	var deleted []string
	var m *Model
	m = &Model{
		ctx: context.Background(),
		tree: &Tree{
			sel: first,
			sels: map[string]bool{
				first: true, second: true,
			},
			rows: []*treeNode{{path: first}, {path: second}},
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "fs_delete":
				args, _ := input.(map[string]any)
				deleted = append(deleted, args["path"].(string))
				return json.RawMessage(`null`), nil
			case "vcs_status":
				return json.RawMessage(`[]`), nil
			default:
				t.Fatalf("unexpected RPC %s", command)
			}
			return nil, nil
		},
		postHook: func(fn func()) {
			fn()
			if m.treeLastMove != nil {
				done <- struct{}{}
			}
		},
	}
	m.setRoot(root)
	m.AskTreeDeletePaths([]string{first, second})
	if m.treeOp == nil || len(m.treeOp.paths) != 2 {
		t.Fatalf("delete prompt = %+v", m.treeOp)
	}
	m.treeOp.safeDelete = false
	m.ConfirmTreeOp()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("delete did not finish")
	}
	if !reflect.DeepEqual(deleted, []string{first, second}) {
		t.Fatalf("deleted = %v", deleted)
	}
	if m.treeLastMove == nil || m.treeLastMove.Kind != "delete" ||
		!reflect.DeepEqual(m.treeLastMove.DeletePaths, []string{first, second}) {
		t.Fatalf("undo record = %+v", m.treeLastMove)
	}
}

func TestTreeSafeDeleteFindsTopLevelSymbolUsagesAndViewsThem(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "Greeter.java")
	caller := filepath.Join(root, "Caller.java")
	for _, path := range []string{file, caller} {
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var m *Model
	closed := false
	detected := make(chan struct{}, 1)
	m = &Model{
		ctx:  context.Background(),
		tree: &Tree{sel: file, sels: map[string]bool{file: true}},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "doc_open":
				closed = false
				return json.Marshal(openidea.DocInfo{ID: 7, Path: file, Language: "java"})
			case "doc_outline":
				return json.Marshal([]openidea.OutlineItem{{
					Name: "pkg", Kind: "package",
					Children: []openidea.OutlineItem{{
						Name: "Greeter", Kind: "class",
						SelectionRange: openidea.OutlineRange{
							Start: openidea.OutlinePos{Line: 2, Col: 6},
							End:   openidea.OutlinePos{Line: 2, Col: 13},
						},
					}},
				}})
			case "lsp_references":
				if closed {
					t.Error("lsp_references used a closed document")
				}
				args, _ := input.(map[string]any)
				if args["includeDeclaration"] != false ||
					args["projectWide"] != false || args["libraries"] != false {
					t.Errorf("reference flags = %#v", args)
				}
				return json.Marshal(openidea.References{Locations: []openidea.Location{{
					Path: caller,
					Range: openidea.OutlineRange{
						Start: openidea.OutlinePos{Line: 8, Col: 3},
						End:   openidea.OutlinePos{Line: 8, Col: 10},
					},
				}}})
			case "doc_close":
				closed = true
				return json.RawMessage(`null`), nil
			case "fs_delete":
				t.Error("safe delete deleted before the user chose Delete Anyway")
				return json.RawMessage(`null`), nil
			default:
				t.Errorf("unexpected RPC %s", command)
				return json.RawMessage(`null`), nil
			}
		},
		postHook: func(fn func()) {
			fn()
			if m.treeOp != nil && m.treeOp.safeRun != nil &&
				m.treeOp.safeRun.phase == treeSafeDeleteDetected {
				select {
				case detected <- struct{}{}:
				default:
				}
			}
		},
	}
	m.setRoot(root)
	m.AskTreeDeletePaths([]string{file})
	if m.treeOp == nil || !m.treeOp.safeSearchable || !m.treeOp.safeDelete {
		t.Fatalf("safe delete prompt = %+v", m.treeOp)
	}
	m.ConfirmTreeOp()
	select {
	case <-detected:
	case <-time.After(2 * time.Second):
		t.Fatal("safe delete did not detect usages")
	}
	if got := m.treeOp.safeRun.locations; len(got) != 1 || got[0].Path != caller {
		t.Fatalf("detected usages = %+v", got)
	}
	m.treeSafeDeleteViewUsages(m.treeOp)
	if m.treeOp != nil {
		t.Fatal("View Usages left the Delete dialog open")
	}
	if len(m.usages.locs) != 1 || m.usages.locs[0].Path != caller || !m.usages.done {
		t.Fatalf("Find Usages result = %+v", m.usages)
	}
	if !closed {
		t.Fatal("safe-delete document was not closed")
	}
}

func TestTreeSafeDeleteIsHiddenAboveTwentyFiles(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 0, maxSafeDeleteFiles+1)
	for i := 0; i <= maxSafeDeleteFiles; i++ {
		path := filepath.Join(root, "File"+string(rune('A'+i))+".java")
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	m := &Model{}
	m.setRoot(root)
	m.AskTreeDeletePaths(paths)
	if m.treeOp == nil {
		t.Fatal("delete prompt did not open")
	}
	if m.treeOp.safeSearchable {
		t.Fatal("safe delete is offered above OpenIDEA's 20-file limit")
	}
}

func TestTreeSafeDeleteOffDeletesWithoutOpeningDocuments(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Plain.java")
	if err := os.WriteFile(path, []byte("class Plain {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	var m *Model
	m = &Model{
		ctx:   context.Background(),
		prefs: Preferences{ProjectTree: ProjectTreePrefs{SafeDelete: boolPointer(false)}},
		tree:  &Tree{sel: path, sels: map[string]bool{path: true}},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "fs_delete":
				return json.RawMessage(`null`), nil
			case "vcs_status":
				return json.RawMessage(`[]`), nil
			default:
				t.Errorf("safe delete off called %s", command)
			}
			return nil, nil
		},
		postHook: func(fn func()) {
			fn()
			if m.treeLastMove != nil {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		},
	}
	m.setRoot(root)
	m.AskTreeDeletePaths([]string{path})
	if m.treeOp == nil || m.treeOp.safeDelete {
		t.Fatalf("safe delete should be off: %+v", m.treeOp)
	}
	m.ConfirmTreeOp()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("plain delete did not finish")
	}
}

func TestTreeFileUsagesFallsBackWhenTheServerCannotSearch(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "Unsupported.java")
	if err := os.WriteFile(file, []byte("class Unsupported {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "doc_open":
				return json.Marshal(openidea.DocInfo{ID: 3, Path: file})
			case "doc_outline":
				return json.Marshal([]openidea.OutlineItem{{
					Name: "Unsupported", Kind: "class",
					SelectionRange: openidea.OutlineRange{
						Start: openidea.OutlinePos{Line: 0, Col: 6},
					},
				}})
			case "lsp_references":
				return nil, &openidea.Error{Kind: "unavailable", Message: "no server"}
			case "doc_close":
				return json.RawMessage(`null`), nil
			default:
				return nil, nil
			}
		},
	}
	locs, searched, err := m.fileUsagesForSafeDelete(
		context.Background(), []string{file}, &treeSafeDeleteRun{})
	if err != nil {
		t.Fatal(err)
	}
	if searched || len(locs) != 0 {
		t.Fatalf("unsupported usage search = %v, %v", searched, locs)
	}
}

func TestTreeDeleteUndoRestoresFilesAndFoldersFromLocalHistory(t *testing.T) {
	file := "/p/Plain.java"
	dir := "/p/assets"
	child := "/p/assets/logo.png"
	var commands []string
	var restored []string
	var created []string
	done := make(chan struct{}, 1)
	var m *Model
	m = &Model{
		ctx: context.Background(),
		treeLastMove: &treeMoveRecord{
			Kind:        "delete",
			DeletePaths: []string{file, dir},
			DeleteDirs:  []string{dir},
		},
		tree: &Tree{},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			commands = append(commands, command)
			args := input.(map[string]any)
			switch command {
			case "history_list_dir":
				switch args["path"] {
				case file:
					return json.Marshal(directoryHistory{
						Deleted: []string{file},
						Entries: []historyEntry{{ID: "file-1", Path: file, Kind: "snapshot"}},
					})
				case dir:
					return json.Marshal(directoryHistory{
						Deleted: []string{child},
						Entries: []historyEntry{{ID: "child-1", Path: child, Kind: "snapshot"}},
					})
				}
			case "history_restore":
				restored = append(restored, args["path"].(string))
				return json.RawMessage(`null`), nil
			case "fs_create_dir":
				created = append(created, args["path"].(string))
				return json.RawMessage(`null`), nil
			}
			t.Fatalf("unexpected command %s with %#v", command, input)
			return nil, nil
		},
		postHook: func(fn func()) {
			fn()
			if m.treeLastMove == nil {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		},
	}
	m.undoLastTreeDelete()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("delete undo did not finish")
	}
	if !equalStrings(restored, []string{file, child}) {
		t.Fatalf("restored = %v", restored)
	}
	if !equalStrings(created, []string{dir}) {
		t.Fatalf("created directories = %v", created)
	}
	if !equalStrings(commands, []string{
		"history_list_dir", "history_restore", "history_list_dir", "fs_create_dir", "history_restore",
	}) {
		t.Fatalf("commands = %v", commands)
	}
}

func TestTreeMoveUndoRevertsEveryMovedFile(t *testing.T) {
	firstFrom, firstTo := "/p/A.java", "/p/src/A.java"
	secondFrom, secondTo := "/p/B.java", "/p/src/B.java"
	done := make(chan struct{}, 1)
	var request uint32
	var m *Model
	m = &Model{
		ctx: context.Background(),
		treeLastMove: &treeMoveRecord{
			ID: 41, Kind: "move",
			Moves: []treeFileMove{
				{From: firstFrom, To: firstTo},
				{From: secondFrom, To: secondTo},
			},
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "refactor_undo_file_move" {
				t.Fatalf("unexpected command %s", command)
			}
			request = input.(map[string]any)["id"].(uint32)
			return json.Marshal(map[string]any{"moves": []treeFileMove{
				{From: firstTo, To: firstFrom},
				{From: secondTo, To: secondFrom},
			}})
		},
		postHook: func(fn func()) {
			fn()
			if m.treeLastMove == nil {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		},
	}
	m.UndoLastTreeMove()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("multi-file move undo did not finish")
	}
	if request != 41 {
		t.Fatalf("undo request = %d", request)
	}
	if m.status != "Undid move of 2 items" {
		t.Fatalf("status = %q", m.status)
	}
}
