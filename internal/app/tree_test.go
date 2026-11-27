package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mainQueue stands for the window's main thread: the workspace hands it the
// functions that read directories and report changes, and the test runs them
// in turn, as the app does between frames.
type mainQueue chan func()

func (q mainQueue) post(fn func()) { q <- fn }

func (q mainQueue) drain(t *testing.T, what string, until func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !until() {
		select {
		case fn := <-q:
			fn()
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func hasRow(tr *Tree, path string) bool {
	for _, row := range tr.Rows() {
		if row.path == path {
			return true
		}
	}
	return false
}

// TestTreeLoadsOnDemandFromTheWorkspace checks the tree GoIDEA shows: the root
// opens at once, a folder reads when it opens, and neither tool folders nor
// the project's build output show.
func TestTreeLoadsOnDemandFromTheWorkspace(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		"src/main/java/com/example", ".idea", ".openidea", ".vscode", ".git", "target", "build",
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	app := filepath.Join(root, "src/main/java/com/example/App.java")
	if err := os.WriteFile(app, []byte("public class App {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "target/Generated.java"), []byte("class Generated {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	q := make(mainQueue, 128)
	tr := NewTree(root, q.post, func() {})
	defer tr.Close()

	src := filepath.Join(root, "src")
	q.drain(t, "the root's children", func() bool { return hasRow(tr, src) })
	// Explicit build exclusions stay shaded. Project tool folders are ordinary
	// rows in IDEA and do not inherit that background.
	for _, excluded := range []string{"target", "build"} {
		p := filepath.Join(root, excluded)
		var row *treeNode
		for _, r := range tr.Rows() {
			if r.path == p {
				row = r
			}
		}
		if row == nil || !row.excluded {
			t.Errorf("%s is not drawn as an excluded row: %v", excluded, row)
		}
	}
	for _, ordinary := range []string{".idea", ".openidea", ".vscode"} {
		p := filepath.Join(root, ordinary)
		var row *treeNode
		for _, r := range tr.Rows() {
			if r.path == p {
				row = r
			}
		}
		if row == nil || row.excluded {
			t.Errorf("%s is not an ordinary project row: %v", ordinary, row)
		}
	}
	if p := filepath.Join(root, ".git"); hasRow(tr, p) {
		t.Errorf(".git shows in the tree")
	}

	tr.Toggle(src)
	q.drain(t, "the src folder", func() bool { return hasRow(tr, filepath.Join(src, "main")) })

	tr.reveal(app)
	q.drain(t, "the revealed file", func() bool { return hasRow(tr, app) })
	if !tr.open[filepath.Join(root, "src/main/java")] {
		t.Fatal("reveal left a folder closed")
	}

	tr.Toggle(src)
	if hasRow(tr, filepath.Join(src, "main")) {
		t.Fatal("a closed folder still shows its children")
	}
}

// TestTreeFollowsTheDisk checks that a file made outside the app shows up
// without reopening the project: the tree watches the folder.
func TestTreeFollowsTheDisk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Old.java"), []byte("class Old {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	q := make(mainQueue, 128)
	tr := NewTree(root, q.post, func() {})
	defer tr.Close()

	old := filepath.Join(root, "Old.java")
	removedPath := ""
	tr.SetRemovedHandler(func(paths []string) {
		for _, path := range paths {
			if path == old {
				removedPath = path
			}
		}
	})
	q.drain(t, "the first file", func() bool { return hasRow(tr, old) })

	made := filepath.Join(root, "Made.java")
	if err := os.WriteFile(made, []byte("class Made {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	q.drain(t, "the file made on disk", func() bool { return hasRow(tr, made) })

	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	q.drain(t, "the removed file", func() bool { return !hasRow(tr, old) })
	if removedPath != old {
		t.Fatalf("removed handler received %q, want %q", removedPath, old)
	}
}

// TestTreeScrollsSidewaysPastTheIsland: a row whose name runs past the Project
// window keeps its whole name, and the tree scrolls sideways to show it, as
// OpenIDEA's tree does.
func TestTreeScrollsSidewaysPastTheIsland(t *testing.T) {
	root := t.TempDir()
	long := filepath.Join(root, "AnotherExtremelyLongClassNameThatRunsPastTheIsland.java")
	m := &Model{}
	m.setRoot(root)
	m.tree = &Tree{open: map[string]bool{root: true}}
	m.tree.root = &treeNode{path: root, name: filepath.Base(root), dir: true, expanded: true}
	m.tree.rows = []*treeNode{
		m.tree.root,
		{path: long, name: filepath.Base(long), depth: 1},
	}
	defer m.tree.Close()
	m.tree.Select(long)
	tt := workbenchTester(t, m)
	if m.treeScroll.MaxX <= 0 {
		t.Fatalf("a row wider than the island left nothing to scroll sideways: %+v", m.treeScroll)
	}
	// The pointer over the tree scrolls it sideways, which brings the end of
	// the name in.
	tt.Move(100, 160)
	tt.Scroll(100, 160, 60, 0)
	tt.Frame()
	if m.treeScroll.X <= 0 {
		t.Fatalf("the tree did not scroll sideways: %+v", m.treeScroll)
	}
}

// TestTreeCompactsMiddlePackages checks the way OpenIDEA shows an opened
// package chain: com/example/app draws as one row, com.example.app, whose path
// is the deepest package; a package with two children keeps its own row.
func TestTreeCompactsMiddlePackages(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "src/main/java/com/example/app")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(deep, "Main.java")
	if err := os.WriteFile(main, []byte("package com.example.app;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"src/main/java/org/one", "src/main/java/org/two"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	q := make(mainQueue, 128)
	tr := NewTree(root, q.post, func() {})
	defer tr.Close()

	// reveal is what opening a file does: it opens the folders down to the file.
	tr.reveal(main)
	q.drain(t, "the revealed file", func() bool { return hasRow(tr, main) })

	var joined *treeNode
	for _, row := range tr.Rows() {
		if row.dir && row.name == "com.example.app" {
			joined = row
		}
		if row.dir && (row.name == "com" || row.name == "example") {
			t.Fatalf("%q still has its own row at %s", row.name, row.path)
		}
	}
	if joined == nil {
		t.Fatalf("com.example.app was not compacted: %v", tr.Rows())
	}
	if joined.path != deep {
		t.Fatalf("the compacted row points at %s, want the deepest package %s", joined.path, deep)
	}
	// A package with two children is not a middle package: it keeps its row.
	java := filepath.Join(root, "src/main/java")
	if !hasRow(tr, filepath.Join(java, "org")) {
		t.Fatalf("org has no row: %v", tr.Rows())
	}
}

func TestTreeSelectionMatchesOpenIDEAModifierClicks(t *testing.T) {
	rows := []*treeNode{
		{path: "/p/a", depth: 1},
		{path: "/p/b", depth: 1},
		{path: "/p/c", depth: 1},
		{path: "/p/d", depth: 1},
		{path: "/p/e", depth: 1},
	}
	tr := &Tree{rows: rows, sels: map[string]bool{}}
	tr.Select(rows[0].path)
	tr.clickSelection(rows[1].path, false, true)
	if got := tr.SelectedPaths(); !equalStrings(got, []string{"/p/a", "/p/b"}) {
		t.Fatalf("toggle selection = %v", got)
	}
	tr.clickSelection(rows[3].path, false, false)
	if got := tr.SelectedPaths(); !equalStrings(got, []string{"/p/d"}) {
		t.Fatalf("plain click selection = %v", got)
	}
	tr.clickSelection(rows[0].path, false, false)
	tr.clickSelection(rows[2].path, true, false)
	if got := tr.SelectedPaths(); !equalStrings(got, []string{"/p/a", "/p/b", "/p/c"}) {
		t.Fatalf("range selection = %v", got)
	}
	tr.clickSelection(rows[1].path, false, true)
	if got := tr.SelectedPaths(); !equalStrings(got, []string{"/p/a", "/p/c"}) {
		t.Fatalf("toggle off selection = %v", got)
	}
	tr.clickSelection(rows[0].path, false, true)
	tr.clickSelection(rows[2].path, false, true)
	if got := tr.SelectedPaths(); len(got) != 0 {
		t.Fatalf("empty selection = %v", got)
	}
}
