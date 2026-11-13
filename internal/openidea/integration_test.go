package openidea_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

// These tests drive a real openidea-server. Point GOIDEA_SERVER at the binary
// to run them; they are skipped otherwise, so ordinary runs need no backend.
//
//	GOIDEA_SERVER=/path/to/openidea-server CGO_ENABLED=0 go test ./internal/openidea -run Integration -v
func launchForTest(t *testing.T) *openidea.Manager {
	t.Helper()
	server := os.Getenv("GOIDEA_SERVER")
	if server == "" {
		t.Skip("GOIDEA_SERVER is not set")
	}
	home := t.TempDir()
	m := openidea.NewManager(openidea.LaunchConfig{
		ServerPath:     server,
		Home:           home,
		Log:            testWriter{t},
		StartupTimeout: 60 * time.Second,
	})
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Logf("close manager: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	return m
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("server: %s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func writeFixture(t *testing.T, dir string) (path, text string) {
	t.Helper()
	text = "package demo;\n\nclass 世界 {\n\tString s = \"😀\";\n}\n"
	path = filepath.Join(dir, "src", "demo", "World.java")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, text
}

// openProject opens dir in window and waits for the backend to accept it.
func openProject(t *testing.T, m *openidea.Manager, window, dir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := m.Invoke(ctx, window, "open_project", map[string]any{"path": dir}); err != nil {
		t.Fatalf("open_project: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		raw, err := m.Invoke(ctx, window, "project_current", nil)
		if err == nil && strings.TrimSpace(string(raw)) != "null" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("project never became current (last error %v, reply %s)", err, raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestIntegrationDocumentSync(t *testing.T) {
	m := launchForTest(t)
	dir := t.TempDir()
	path, text := writeFixture(t, dir)
	const window = "goidea-it"

	info := m.Server().Info()
	if info.Protocol != 2 {
		t.Fatalf("server speaks protocol %d", info.Protocol)
	}
	for _, cmd := range []string{"doc_open", "doc_edit", "doc_snapshot", "doc_save", "doc_close", "open_project"} {
		if !m.Server().HasCommand(cmd) {
			t.Fatalf("server does not offer %s", cmd)
		}
	}
	openProject(t, m, window, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	docs := openidea.Docs{M: m, Window: window}
	doc, err := docbridge.Open(ctx, docs, window, path)
	if err != nil {
		t.Fatalf("open document: %v", err)
	}
	defer doc.Close(ctx)
	if doc.Text() != text {
		t.Fatalf("text:\n%q\nwant\n%q", doc.Text(), text)
	}
	if doc.Info().Language != "java" {
		t.Logf("language %q", doc.Info().Language)
	}

	// Edit through the bridge: the change set is UTF-16, the insert has CJK and
	// an astral emoji, and the buffer and backend must agree.
	at := strings.Index(doc.Text(), "\tString")
	if at < 0 {
		t.Fatal("fixture")
	}
	insert := "\tint 数字 = 1; // 😀\n"
	if err := doc.ApplyEdits(ctx, editor.Edit{Start: at, End: at, Text: insert}); err != nil {
		t.Fatalf("apply edit: %v", err)
	}
	want := text[:at] + insert + text[at:]
	if got := doc.Text(); got != want {
		t.Fatalf("buffer:\n%q\nwant\n%q", got, want)
	}

	snap, err := docs.DocSnapshot(ctx, window, doc.ID())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Text != want {
		t.Fatalf("backend:\n%q\nwant\n%q", snap.Text, want)
	}
	if snap.Version != doc.Version() {
		t.Fatalf("backend version %d, bridge %d", snap.Version, doc.Version())
	}
	if !mapperMatches(snap.Text, doc.Mapper()) {
		t.Fatal("bridge line index disagrees with a fresh mapper")
	}

	if err := doc.Save(ctx); err != nil {
		t.Fatalf("save: %v", err)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != want {
		t.Fatalf("disk:\n%q\nwant\n%q", disk, want)
	}
	snap, err = docs.DocSnapshot(ctx, window, doc.ID())
	if err != nil {
		t.Fatalf("snapshot after save: %v", err)
	}
	if snap.Dirty {
		t.Fatal("document still dirty after save")
	}

	// Undo goes through the buffer and back out as a change set.
	if ok, err := doc.Undo(ctx); !ok || err != nil {
		t.Fatalf("undo: %v %v", ok, err)
	}
	snap, err = docs.DocSnapshot(ctx, window, doc.ID())
	if err != nil {
		t.Fatalf("snapshot after undo: %v", err)
	}
	if snap.Text != text {
		t.Fatalf("undo did not reach the backend:\n%q\nwant\n%q", snap.Text, text)
	}
}

func TestIntegrationRestartAndCleanup(t *testing.T) {
	m := launchForTest(t)
	dir := t.TempDir()
	path, text := writeFixture(t, dir)
	const window = "goidea-it"

	openProject(t, m, window, dir)
	m.OnRestart(func(ctx context.Context) error {
		if _, err := m.Invoke(ctx, window, "open_project", map[string]any{"path": dir}); err != nil {
			return err
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	docs := openidea.Docs{M: m, Window: window}
	reg := docbridge.NewRegistry(docs, window)
	go reg.Run(ctx, m.Notifications())

	doc, err := reg.Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	at := strings.Index(doc.Text(), "}\n")
	if err := doc.ApplyEdits(ctx, editor.Edit{Start: at, End: at, Text: "// unsaved\n"}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	unsaved := doc.Text()

	srv := m.Server()
	pid := srv.PID()
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill server: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if st := m.State(); st.Running && st.Restarts >= 1 && m.Server() != nil && m.Server() != srv {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not come back: %+v", m.State())
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The project re-opens through OnRestart; reopen the documents on top of it
	// and prove the unsaved buffer survived the crash.
	var reopenErr error
	for attempt := 0; attempt < 120; attempt++ {
		if reopenErr = reg.ReopenAll(ctx); reopenErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if reopenErr != nil {
		t.Fatalf("reopen: %v", reopenErr)
	}
	if got := doc.Text(); got != unsaved {
		t.Fatalf("unsaved text lost:\n%q\nwant\n%q", got, unsaved)
	}
	snap, err := docs.DocSnapshot(ctx, window, doc.ID())
	if err != nil {
		t.Fatalf("snapshot after reopen: %v", err)
	}
	if snap.Text != unsaved {
		t.Fatalf("backend:\n%q\nwant\n%q", snap.Text, unsaved)
	}
	if err := doc.Save(ctx); err != nil {
		t.Fatalf("save after reopen: %v", err)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != unsaved {
		t.Fatalf("disk:\n%q\nwant\n%q", disk, unsaved)
	}
	if !strings.Contains(string(disk), "// unsaved\n") {
		t.Fatal("fixture")
	}

	// Closing the manager must not leave the restarted server running.
	newPID := m.Server().PID()
	if err := m.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	waitGone(t, newPID)
	waitGone(t, pid)
	if !strings.Contains(text, "class 世界") {
		t.Fatal("fixture")
	}
}

func mapperMatches(text string, got *position.Mapper) bool {
	want := position.NewMapper(text)
	if got.LineCount() != want.LineCount() {
		return false
	}
	t := position.StringText(text)
	for off := 0; off <= len(text); off++ {
		if got.ByteOffsetToUTF16Offset(t, off) != want.ByteOffsetToUTF16Offset(t, off) {
			return false
		}
	}
	return true
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		// Signal 0 only checks for existence; the server runs as this user.
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still alive", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
