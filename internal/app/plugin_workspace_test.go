package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

type pluginWorkspaceBackend struct {
	info      openidea.DocInfo
	saveCount atomic.Int32
}

func (b *pluginWorkspaceBackend) DocOpen(context.Context, string, string) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *pluginWorkspaceBackend) DocEdit(context.Context, string, uint32, uint32, position.ChangeSet) (uint32, error) {
	return b.info.Version + 1, nil
}

func (b *pluginWorkspaceBackend) DocSnapshot(context.Context, string, uint32) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *pluginWorkspaceBackend) DocSave(context.Context, string, uint32) error {
	b.saveCount.Add(1)
	return nil
}

func (b *pluginWorkspaceBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func pluginWorkspaceModel(
	t *testing.T,
	root string,
	backend *pluginWorkspaceBackend,
) (*Model, *docbridge.Registry, *docbridge.Document) {
	t.Helper()
	reg := docbridge.NewRegistry(backend, Window)
	doc, err := reg.Open(context.Background(), backend.info.Path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{
		postHook: func(fn func()) { fn() },
		reg:      reg,
	}
	m.setRoot(root)
	return m, reg, doc
}

func TestPluginWorkspaceRefusesDirtyWritesAndSymlinkEscapes(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "App.java")
	if err := os.WriteFile(file, []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &pluginWorkspaceBackend{info: openidea.DocInfo{
		ID: 1, Path: file, Text: "unsaved", Version: 1, Dirty: true, Language: "java",
	}}
	m, _, doc := pluginWorkspaceModel(t, root, backend)
	if err := doc.ApplyEdits(context.Background(), editor.Edit{Start: 0, End: 0, Text: "unsaved"}); err != nil {
		t.Fatal(err)
	}
	h := &goideaPluginHost{m: m}
	if _, err := h.Call(nil, "workspace.writeFile", json.RawMessage(
		`{"path":"App.java","text":"plugin"}`,
	)); err == nil || !strings.Contains(err.Error(), "unsaved changes") {
		t.Fatalf("dirty write error = %v", err)
	}
	if value, err := os.ReadFile(file); err != nil || string(value) != "disk" {
		t.Fatalf("dirty write changed disk: %q, %v", value, err)
	}
	if _, err := h.Call(nil, "workspace.writeFile", json.RawMessage(
		`{"path":"App.java","text":"`+strings.Repeat("x", int(pluginWorkspaceFileLimit)+1)+`"}`,
	)); err == nil || !strings.Contains(err.Error(), "limited") {
		t.Fatalf("oversize write error = %v", err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "secret-link")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if _, err := h.Call(nil, "workspace.readFile", json.RawMessage(
		`{"path":"secret-link"}`,
	)); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("symlink escape error = %v", err)
	}
}

func TestPluginWorkspaceWritesCleanFilesAtomically(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	backend := &pluginWorkspaceBackend{info: openidea.DocInfo{
		ID: 2, Path: file, Text: "old", Version: 1,
	}}
	m, _, _ := pluginWorkspaceModel(t, root, backend)
	h := &goideaPluginHost{m: m}
	if _, err := h.Call(nil, "workspace.writeFile", json.RawMessage(
		`{"path":"nested/new.txt","text":"new"}`,
	)); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(filepath.Join(root, "nested", "new.txt"))
	if err != nil || string(value) != "new" {
		t.Fatalf("new file = %q, %v", value, err)
	}
	if _, err := h.Call(nil, "workspace.writeFile", json.RawMessage(
		`{"path":"notes.txt","text":"replaced"}`,
	)); err != nil {
		t.Fatal(err)
	}
	value, err = os.ReadFile(file)
	if err != nil || string(value) != "replaced" {
		t.Fatalf("replaced file = %q, %v", value, err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("replacement mode = %o, want 640", info.Mode().Perm())
	}
}

func TestPluginWorkspaceFindFilesUsesBoundedBackendRPC(t *testing.T) {
	var command string
	var input map[string]any
	m := &Model{
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, cmd string, raw any) (json.RawMessage, error) {
			command = cmd
			input = raw.(map[string]any)
			return json.RawMessage(`["src/App.java","src/Util.java"]`), nil
		},
	}
	m.setRoot("/srv/project")
	h := &goideaPluginHost{m: m}
	value, err := h.Call(nil, "workspace.findFiles", json.RawMessage(
		`{"glob":"*.java","limit":2000}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	paths := value.([]string)
	if command != "plugin_workspace_find_files" || input["root"] != "/srv/project" ||
		input["glob"] != "*.java" || input["limit"] != 2000 {
		t.Fatalf("find command = %q, input = %#v", command, input)
	}
	if request, ok := input["request"].(uint32); !ok || request == 0 {
		t.Fatalf("find request = %#v", input["request"])
	}
	if strings.Join(paths, ",") != "src/App.java,src/Util.java" {
		t.Fatalf("find paths = %#v", paths)
	}
}

func TestPluginWorkspaceDocumentsExposeLiveBuffers(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "Unicode.java")
	backend := &pluginWorkspaceBackend{info: openidea.DocInfo{
		ID: 7, Path: file, Text: "a😀b\nsecond", Version: 3, Language: "java",
	}}
	m, reg, doc := pluginWorkspaceModel(t, root, backend)
	defer reg.Close(context.Background(), file)
	if err := doc.ApplyEdits(context.Background(), editor.Edit{Start: 0, End: 0, Text: "// live\n"}); err != nil {
		t.Fatal(err)
	}
	h := &goideaPluginHost{m: m}

	docs := h.pluginDocuments()
	if len(docs) != 1 || docs[0]["id"] != uint32(7) || docs[0]["language"] != "java" ||
		docs[0]["dirty"] != true || docs[0]["lineCount"] != 3 {
		t.Fatalf("documents = %#v", docs)
	}
	got, err := h.pluginDocumentText(7, &pluginTextRange{
		Start: pluginTextPosition{Line: 1},
		End:   pluginTextPosition{Line: 1, Character: 3},
	})
	if err != nil || got != "a😀" {
		t.Fatalf("document text = %q, %v", got, err)
	}
	line, err := h.pluginDocumentLine(7, 2)
	if err != nil || line != "second" {
		t.Fatalf("document line = %q, %v", line, err)
	}
	if err := h.pluginSaveDocument(7); err != nil {
		t.Fatal(err)
	}
	if got := backend.saveCount.Load(); got != 1 {
		t.Fatalf("save calls = %d", got)
	}
}

func TestPluginWorkspaceRemoteWriteUsesRootScopedRPC(t *testing.T) {
	var command string
	var input map[string]any
	m := &Model{
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, cmd string, raw any) (json.RawMessage, error) {
			command = cmd
			input = raw.(map[string]any)
			return json.RawMessage(`null`), nil
		},
		remote: remoteDevelopmentState{session: &openidea.RemoteSession{
			Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev"},
			Root:   "/srv/project",
		}},
	}
	m.setRoot("/srv/project")
	h := &goideaPluginHost{m: m}
	if err := h.pluginWriteWorkspaceFile("src/App.java", "class App {}"); err != nil {
		t.Fatal(err)
	}
	if command != "plugin_workspace_write_text" || input["root"] != "/srv/project" ||
		input["path"] != "/srv/project/src/App.java" || input["text"] != "class App {}" {
		t.Fatalf("remote write = %q, %#v", command, input)
	}
	if _, ok := input["request"].(uint32); !ok {
		t.Fatalf("remote write request = %#v", input["request"])
	}
}

func TestPluginOpenFileUsesOneBasedCoordinates(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "App.java")
	backend := &pluginWorkspaceBackend{info: openidea.DocInfo{
		ID: 8, Path: file, Text: "one\ntwo", Version: 1, Language: "java",
	}}
	m, _, _ := pluginWorkspaceModel(t, root, backend)
	h := &goideaPluginHost{m: m}
	if _, err := h.Call(nil, "workspace.open", json.RawMessage(
		`{"path":"App.java","line":2,"column":2}`,
	)); err != nil {
		t.Fatal(err)
	}
	deadline := 0
	for m.tab() == nil && deadline < 100 {
		time.Sleep(10 * time.Millisecond)
		deadline++
	}
	if m.tab() == nil {
		t.Fatal("workspace.open did not create a tab")
	}
	if got := m.tab().state.CursorPos(); got != (editor.Pos{Line: 1, Col: 1}) {
		t.Fatalf("workspace.open cursor = %+v", got)
	}
}
