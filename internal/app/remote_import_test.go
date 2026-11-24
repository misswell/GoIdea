package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

type remoteImportCall struct {
	command string
	path    string
	target  string
	from    string
	to      string
	data    []int
	mode    int
	request uint32
}

func newRemoteImportTestModel(t *testing.T, invoke func(context.Context, string, any) (json.RawMessage, error)) *Model {
	t.Helper()
	root := "/srv/app"
	target := "/srv/app/imports"
	m := &Model{
		ctx:        context.Background(),
		invokeHook: invoke,
		postHook:   func(fn func()) { fn() },
		tree: &Tree{
			root:          &treeNode{path: root, name: "app", dir: true},
			remoteLoader:  func(string, func([]openidea.RemoteDirectoryEntry, error)) {},
			remoteEntries: map[string][]openidea.RemoteDirectoryEntry{target: make([]openidea.RemoteDirectoryEntry, 0)},
			remoteLoading: map[string]bool{},
			remoteErrs:    map[string]string{},
			open:          map[string]bool{},
			sels:          map[string]bool{},
			collapsed:     map[string]bool{},
			outlines:      map[string][]openidea.OutlineItem{},
			virtualDirs:   map[string]bool{},
			options:       defaultTreeOptions(),
		},
	}
	m.remote.session = &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
		Root:   root,
		OS:     "linux",
	}
	m.setRoot(root)
	m.showingWelcome = false
	return m
}

func waitForRemoteImport(t *testing.T, m *Model) {
	t.Helper()
	done := m.remote.importDone
	if done == nil {
		t.Fatal("remote import has no completion signal")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("remote import did not finish")
	}
}

func remoteImportCallFromInput(command string, input any) remoteImportCall {
	args, _ := input.(map[string]any)
	call := remoteImportCall{command: command}
	if args == nil {
		return call
	}
	call.path, _ = args["path"].(string)
	call.target, _ = args["target"].(string)
	call.from, _ = args["from"].(string)
	call.to, _ = args["to"].(string)
	if data, ok := args["data"].([]int); ok {
		call.data = append([]int(nil), data...)
	}
	if mode, ok := args["mode"].(uint32); ok {
		call.mode = int(mode)
	}
	if request, ok := args["request"].(uint32); ok {
		call.request = request
	}
	return call
}

func TestRemoteImportCopiesLocalDirectoryThroughOpenIDEARPCs(t *testing.T) {
	external := t.TempDir()
	bundle := filepath.Join(external, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "nested", "b.bin"), []byte{0, 1, 255}, 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var calls []remoteImportCall
	m := newRemoteImportTestModel(t, func(_ context.Context, command string, input any) (json.RawMessage, error) {
		mu.Lock()
		calls = append(calls, remoteImportCallFromInput(command, input))
		mu.Unlock()
		return json.RawMessage(`null`), nil
	})

	m.pasteExternalTreeFiles([]string{bundle}, "/srv/app/imports")
	waitForRemoteImport(t, m)
	mu.Lock()
	got := append([]remoteImportCall(nil), calls...)
	mu.Unlock()
	wantCommands := []string{
		"fs_create_dir_exclusive",
		"fs_create_file",
		"fs_append_file_bytes",
		"fs_set_permissions",
		"fs_rename",
		"fs_create_dir_exclusive",
		"fs_create_file",
		"fs_append_file_bytes",
		"fs_set_permissions",
		"fs_rename",
		"fs_set_permissions",
		"fs_set_permissions",
	}
	if len(got) != len(wantCommands) {
		t.Fatalf("remote import calls = %+v, want %d calls", got, len(wantCommands))
	}
	for i, command := range wantCommands {
		if got[i].command != command {
			t.Fatalf("remote import call %d = %+v, want %s", i, got[i], command)
		}
	}
	if got[0].path != "/srv/app/imports/bundle" || got[5].path != "/srv/app/imports/bundle/nested" {
		t.Fatalf("folder calls = %+v", got)
	}
	if !reflect.DeepEqual(got[2].data, []int{'a', 'l', 'p', 'h', 'a'}) ||
		!reflect.DeepEqual(got[7].data, []int{0, 1, 255}) {
		t.Fatalf("streamed data = %v and %v", got[2].data, got[7].data)
	}
	if got[1].path != got[4].from || got[4].to != "/srv/app/imports/bundle/a.txt" ||
		got[6].path != got[9].from || got[9].to != "/srv/app/imports/bundle/nested/b.bin" {
		t.Fatalf("temporary publish calls = %+v", got)
	}
	if got[2].request == 0 || got[7].request != got[2].request {
		t.Fatalf("import request ids = %d and %d", got[2].request, got[7].request)
	}
	if got[10].path != "/srv/app/imports/bundle/nested" ||
		got[11].path != "/srv/app/imports/bundle" {
		t.Fatalf("folder permission calls = %+v", got[10:])
	}
}

func TestRemoteImportRemovesOnlyTheFolderItCreatedAfterFailure(t *testing.T) {
	external := t.TempDir()
	bundle := filepath.Join(external, "bundle")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "bad.txt"), []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var calls []remoteImportCall
	cleaned := make(chan struct{}, 1)
	m := newRemoteImportTestModel(t, func(_ context.Context, command string, input any) (json.RawMessage, error) {
		mu.Lock()
		calls = append(calls, remoteImportCallFromInput(command, input))
		mu.Unlock()
		switch command {
		case "fs_append_file_bytes":
			return nil, errors.New("upload failed")
		case "fs_remove_tree":
			cleaned <- struct{}{}
		}
		return json.RawMessage(`null`), nil
	})

	m.pasteExternalTreeFiles([]string{bundle}, "/srv/app/imports")
	select {
	case <-cleaned:
	case <-time.After(2 * time.Second):
		t.Fatal("failed import did not remove its new folder")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 5 ||
		calls[0].command != "fs_create_dir_exclusive" ||
		calls[0].path != "/srv/app/imports/bundle" ||
		calls[1].command != "fs_create_file" ||
		calls[2].command != "fs_append_file_bytes" ||
		calls[3].command != "fs_remove_file" ||
		calls[3].path != calls[1].path ||
		calls[4].command != "fs_remove_tree" ||
		calls[4].path != "/srv/app/imports/bundle" {
		t.Fatalf("failed import calls = %+v", calls)
	}
}

func TestRemoteImportStreamsLargeFilesInBoundedChunks(t *testing.T) {
	source := filepath.Join(t.TempDir(), "large.bin")
	payload := bytes.Repeat([]byte{0x00, 0x7f, 0xff, 0x42}, remoteImportChunkSize)
	if err := os.WriteFile(source, payload, 0o640); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var chunks [][]int
	m := newRemoteImportTestModel(t, func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command == "fs_append_file_bytes" {
			call := remoteImportCallFromInput(command, input)
			if len(call.data) > remoteImportChunkSize {
				t.Errorf("chunk = %d bytes, limit %d", len(call.data), remoteImportChunkSize)
			}
			mu.Lock()
			chunks = append(chunks, call.data)
			mu.Unlock()
		}
		return json.RawMessage(`null`), nil
	})
	m.pasteExternalTreeFiles([]string{source}, "/srv/app/imports")
	waitForRemoteImport(t, m)

	mu.Lock()
	defer mu.Unlock()
	if len(chunks) < 2 {
		t.Fatalf("streamed %d chunks, want a multi-chunk upload", len(chunks))
	}
	var received []byte
	for _, chunk := range chunks {
		for _, value := range chunk {
			received = append(received, byte(value))
		}
	}
	if !bytes.Equal(received, payload) {
		t.Fatalf("streamed %d bytes, want %d", len(received), len(payload))
	}
}

func TestRemoteImportPreservesSymbolicLinksWithoutFollowingThem(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "links")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "target.txt"), []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(bundle, "alias.txt")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	var mu sync.Mutex
	var linkCall remoteImportCall
	m := newRemoteImportTestModel(t, func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command == "fs_create_symlink" {
			mu.Lock()
			linkCall = remoteImportCallFromInput(command, input)
			mu.Unlock()
		}
		return json.RawMessage(`null`), nil
	})
	m.pasteExternalTreeFiles([]string{bundle}, "/srv/app/imports")
	waitForRemoteImport(t, m)

	mu.Lock()
	defer mu.Unlock()
	if linkCall.path != "/srv/app/imports/links/alias.txt" || linkCall.target != "target.txt" {
		t.Fatalf("symbolic-link call = %+v", linkCall)
	}
}

func TestRemoteImportCancellationStopsStreamingAndCleansTemporary(t *testing.T) {
	source := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(source, bytes.Repeat([]byte("x"), remoteImportChunkSize*2), 0o644); err != nil {
		t.Fatal(err)
	}
	streaming := make(chan struct{}, 1)
	var mu sync.Mutex
	var removed []string
	m := newRemoteImportTestModel(t, func(ctx context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "fs_append_file_bytes":
			select {
			case streaming <- struct{}{}:
			default:
			}
			<-ctx.Done()
			return nil, ctx.Err()
		case "fs_remove_file":
			mu.Lock()
			removed = append(removed, remoteImportCallFromInput(command, input).path)
			mu.Unlock()
		}
		return json.RawMessage(`null`), nil
	})
	m.pasteExternalTreeFiles([]string{source}, "/srv/app/imports")
	select {
	case <-streaming:
	case <-time.After(2 * time.Second):
		t.Fatal("remote import did not start streaming")
	}
	importDone := m.remote.importDone
	if importDone == nil {
		t.Fatal("remote import has no completion signal")
	}
	m.CancelRemoteImport()
	waitForRemoteImport(t, m)
	select {
	case <-importDone:
	case <-time.After(2 * time.Second):
		t.Fatal("remote import completion signal stayed open after cleanup")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(removed) != 1 || !strings.HasPrefix(removed[0], "/srv/app/imports/.goid-import-") {
		t.Fatalf("temporary cleanup = %v", removed)
	}
}

func TestWaitRemoteImportDoneReturnsWhenCleanupStalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	if err := waitRemoteImportDone(ctx, done); err == nil {
		t.Fatal("waitRemoteImportDone returned before the import cleanup finished")
	}
	close(done)
	if err := waitRemoteImportDone(context.Background(), done); err != nil {
		t.Fatalf("waitRemoteImportDone after completion: %v", err)
	}
}
