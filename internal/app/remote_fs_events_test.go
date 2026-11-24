package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestChangedParentFoldersDeduplicatesCreatedModifiedAndRemoved(t *testing.T) {
	got := changedParentFolders(openidea.FsChangedEvent{
		Created:  []string{"/srv/app/src/New.java", "/srv/app/assets/icon.svg"},
		Modified: []string{"/srv/app/src/Main.java"},
		Removed:  []string{"/srv/app/src/Old.java"},
	})
	want := []string{"/srv/app/assets", "/srv/app/src"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refresh folders = %v, want %v", got, want)
	}
}

func TestAcceptFsChangedInvalidatesSpringRoutesForJavaKotlinAndScripts(t *testing.T) {
	for _, file := range []string{"/srv/app/src/Main.java", "/srv/app/src/Main.kt", "/srv/app/build.kts"} {
		if !acceptsSpringRouteChanges(openidea.FsChangedEvent{Modified: []string{file}}) {
			t.Errorf("%s did not invalidate Spring routes", file)
		}
	}
	if acceptsSpringRouteChanges(openidea.FsChangedEvent{Modified: []string{"/srv/app/README.md"}}) {
		t.Fatal("an unrelated file invalidated Spring routes")
	}
}

func TestAcceptFsChangedReloadsCleanRemoteEditor(t *testing.T) {
	root := "/srv/app"
	file := root + "/src/Main.java"
	doc, err := docbridge.Open(context.Background(), &remoteReloadTestBackend{path: file}, Window, file)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "fs_read_text" {
				t.Fatalf("command = %q, want fs_read_text", command)
			}
			if got := args.(map[string]any)["path"]; got != file {
				t.Fatalf("reload path = %v, want %s", got, file)
			}
			return json.RawMessage(`"class Main { /* fresh */ }"`), nil
		},
		postHook: func(fn func()) {
			fn()
			reloaded <- struct{}{}
		},
		remote: remoteDevelopmentState{
			session: &openidea.RemoteSession{
				Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH},
				Root:   root,
				OS:     "linux",
			},
		},
	}
	m.setRoot(root)
	tab := &Tab{doc: doc}
	if tab.unsaved() {
		t.Fatal("the clean remote editor fixture is already dirty")
	}
	m.tabs = []*Tab{tab}

	m.acceptFsChanged(openidea.FsChangedEvent{Modified: []string{file}})
	select {
	case <-reloaded:
	case <-time.After(2 * time.Second):
		t.Fatal("fsChanged did not reload the clean remote editor")
	}
	if got, want := doc.Text(), "class Main { /* fresh */ }"; got != want {
		t.Fatalf("reloaded text = %q, want %q", got, want)
	}
}

func TestAcceptFsChangedKeepsUnsavedRemoteEditor(t *testing.T) {
	root := "/srv/app"
	file := root + "/src/Main.java"
	backend := &remoteReloadTestBackend{path: file}
	doc, err := docbridge.Open(context.Background(), backend, Window, file)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.ApplyEdits(context.Background(), editor.Edit{
		Start: 0, End: len(doc.Text()), Text: "unsaved local edit",
	}); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("an unsaved editor was reloaded")
			return nil, nil
		},
		remote: remoteDevelopmentState{
			session: &openidea.RemoteSession{
				Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH},
				Root:   root,
				OS:     "linux",
			},
		},
	}
	m.setRoot(root)
	m.tabs = []*Tab{{doc: doc}}

	m.acceptFsChanged(openidea.FsChangedEvent{Modified: []string{file}})
	if got := doc.Text(); got != "unsaved local edit" {
		t.Fatalf("unsaved text changed to %q", got)
	}
	if m.banner == "" {
		t.Fatal("the unsaved external change did not show a banner")
	}
}
