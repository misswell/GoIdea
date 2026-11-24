package app

import (
	"os"
	"path/filepath"
	"testing"

	"mayoi/internal/docbridge"
)

func TestSessionSnapshotKeepsPreviewTabsInBothGroups(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "Main.java")
	utilPath := filepath.Join(root, "Util.java")
	for _, path := range []string{mainPath, utilPath} {
		if err := os.WriteFile(path, []byte("class X {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := &Model{
		tabs: []*Tab{
			{doc: docbridge.Virtual(mainPath, "class Main {}\n"), preview: true},
			{doc: docbridge.Virtual(utilPath, "class Util {}\n")},
		},
		split: &splitGroup{tabs: []*Tab{
			{doc: docbridge.Virtual(mainPath, "class Main {}\n")},
			{doc: docbridge.Virtual(utilPath, "class Util {}\n"), preview: true},
		}},
	}
	m.root = root

	session, ok := m.sessionSnapshot()
	if !ok {
		t.Fatal("session snapshot was empty")
	}
	if len(session.PreviewFiles) != 1 || session.PreviewFiles[0] != mainPath {
		t.Fatalf("preview files = %v", session.PreviewFiles)
	}
	if len(session.SplitPreviewFiles) != 1 || session.SplitPreviewFiles[0] != utilPath {
		t.Fatalf("split preview files = %v", session.SplitPreviewFiles)
	}

	path := filepath.Join(root, "session.json")
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded := LoadSession(path)
	if len(loaded.PreviewFiles) != 1 || loaded.PreviewFiles[0] != mainPath ||
		len(loaded.SplitPreviewFiles) != 1 || loaded.SplitPreviewFiles[0] != utilPath {
		t.Fatalf("loaded preview state = %+v", loaded)
	}
}

func TestRestorePreviewFlagsPromotesPinnedTabs(t *testing.T) {
	preview := &Tab{doc: docbridge.Virtual("/project/Preview.java", "")}
	pinned := &Tab{doc: docbridge.Virtual("/project/Pinned.java", "")}
	regular := &Tab{doc: docbridge.Virtual("/project/Regular.java", "")}
	restorePreviewFlags(
		[]*Tab{preview, pinned, regular},
		map[string]bool{pinned.doc.Path(): true},
		map[string]bool{preview.doc.Path(): true, pinned.doc.Path(): true},
	)
	if !preview.preview || pinned.preview || regular.preview {
		t.Fatalf("preview flags = %v %v %v", preview.preview, pinned.preview, regular.preview)
	}
}
