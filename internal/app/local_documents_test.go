package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

type recordingDocBackend struct {
	mu    sync.Mutex
	opens []string
}

func (b *recordingDocBackend) DocOpen(_ context.Context, _ string, path string) (openidea.DocInfo, error) {
	b.mu.Lock()
	b.opens = append(b.opens, path)
	b.mu.Unlock()
	return openidea.DocInfo{
		ID: 7, Path: path, Text: "remote\n", Version: 1,
		Encoding: "UTF-8", LineEnding: "LF", URI: "file://" + path,
	}, nil
}

func (b *recordingDocBackend) DocEdit(context.Context, string, uint32, uint32, position.ChangeSet) (uint32, error) {
	return 0, errors.New("unexpected remote edit")
}

func (b *recordingDocBackend) DocSnapshot(context.Context, string, uint32) (openidea.DocInfo, error) {
	return openidea.DocInfo{}, errors.New("unexpected remote snapshot")
}

func (b *recordingDocBackend) DocSave(context.Context, string, uint32) error {
	return errors.New("unexpected remote save")
}

func (b *recordingDocBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func (b *recordingDocBackend) opened() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.opens...)
}

func TestHybridDocsRoutesProjectAndStandaloneFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	projectFile := filepath.Join(root, "src", "Main.java")
	if err := os.MkdirAll(filepath.Dir(projectFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectFile, []byte("class Main {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	standalone := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(standalone, []byte("standalone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := &recordingDocBackend{}
	docs := newHybridDocs(remote, func() string { return root })

	remoteInfo, err := docs.DocOpen(context.Background(), Window, projectFile)
	if err != nil {
		t.Fatal(err)
	}
	if docs.source(remoteInfo.ID) != docSourceRemote {
		t.Fatal("a project file did not use OpenIDEA")
	}

	localInfo, err := docs.DocOpen(context.Background(), Window, standalone)
	if err != nil {
		t.Fatal(err)
	}
	if docs.source(localInfo.ID) != docSourceLocal {
		t.Fatal("a standalone file did not use the local LightEdit backend")
	}
	if localInfo.Text != "standalone\n" {
		t.Fatalf("standalone text = %q", localInfo.Text)
	}

	want := []string{projectFile}
	if got := remote.opened(); !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenIDEA opens = %v, want %v", got, want)
	}
}

func TestLocalDocsPreserveBOMLineEndingsModeAndUTF16Offsets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unicode.txt")
	original := append([]byte{0xef, 0xbb, 0xbf}, []byte("α😀\r\nbeta\r\n")...)
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	backend := newLocalDocsBackend()
	info, err := backend.DocOpen(context.Background(), Window, path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Text != "α😀\nbeta\n" || !info.HasBOM || info.LineEnding != "CRLF" {
		t.Fatalf("decoded info = %+v", info)
	}

	// α is one UTF-16 unit and the emoji is two, so [1,3) replaces the emoji.
	version, err := backend.DocEdit(context.Background(), Window, info.ID, info.Version, position.ChangeSet{
		Changes: []position.TextChange{{From: 1, To: 3, Insert: "X"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.DocSnapshot(context.Background(), Window, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if version != snapshot.Version || snapshot.Text != "αX\nbeta\n" {
		t.Fatalf("edited snapshot = %+v, version %d", snapshot, version)
	}
	if err := backend.DocSave(context.Background(), Window, info.ID); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{0xef, 0xbb, 0xbf}, []byte("αX\r\nbeta\r\n")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("saved bytes = %q, want %q", got, want)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := stat.Mode().Perm(); got != 0o640 {
		t.Fatalf("saved mode = %o, want 640", got)
	}
}

func TestLocalDocsRejectEditsAndSavesToReadOnlyFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.txt")
	if err := os.WriteFile(path, []byte("locked\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	backend := newLocalDocsBackend()
	info, err := backend.DocOpen(context.Background(), Window, path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ReadOnly {
		t.Fatal("a read-only file opened as writable")
	}
	if _, err := backend.DocEdit(context.Background(), Window, info.ID, info.Version, position.ChangeSet{
		Changes: []position.TextChange{{From: 0, To: 6, Insert: "open"}},
	}); err == nil {
		t.Fatal("editing a read-only file succeeded")
	}
	if err := backend.DocSave(context.Background(), Window, info.ID); err == nil {
		t.Fatal("saving a read-only file succeeded")
	}
}

func TestStandaloneFileShowsWorkbenchAndClosingItReturnsToWelcome(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "standalone.go")
	if err := os.WriteFile(path, []byte("package standalone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		SettingsPath: filepath.Join(dir, "settings.json"),
		SessionPath:  filepath.Join(dir, "session.json"),
	})
	t.Cleanup(m.off)
	opened := make(chan struct{})
	var openedOnce sync.Once
	m.postHook = func(fn func()) {
		fn()
		openedOnce.Do(func() { close(opened) })
	}
	m.openFile(path)
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("standalone file did not finish opening")
	}
	if m.Root() != "" || len(m.tabs) != 1 || m.showingWelcome {
		t.Fatalf("root=%q tabs=%d welcome=%v", m.Root(), len(m.tabs), m.showingWelcome)
	}

	tester := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1280, 800)
	if !tester.HasText("standalone.go") {
		t.Fatalf("the standalone editor is not visible: %v", tester.Texts())
	}
	if tester.HasText("An open-source IDE inspired by IntelliJ IDEA") {
		t.Fatal("the Welcome screen covered the standalone editor")
	}

	m.closeTab(0)
	tester.Frame()
	if !m.showingWelcome || len(m.tabs) != 0 {
		t.Fatalf("after closing: welcome=%v tabs=%d", m.showingWelcome, len(m.tabs))
	}
	if !tester.HasText("An open-source IDE inspired by IntelliJ IDEA") {
		t.Fatalf("closing the last standalone file did not return to Welcome: %v", tester.Texts())
	}
}
