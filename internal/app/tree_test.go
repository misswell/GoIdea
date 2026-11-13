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
	for _, dir := range []string{"src/main/java/com/example", ".idea", "target", "build"} {
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
	for _, hidden := range []string{".idea", "target", "build"} {
		if p := filepath.Join(root, hidden); hasRow(tr, p) {
			t.Errorf("%s shows in the tree", hidden)
		}
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
}
