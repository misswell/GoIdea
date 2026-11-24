package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestDeploymentActionsAndNativeSurfacesAreRegistered(t *testing.T) {
	for _, id := range []string{
		"deployment.uploadToDefault",
		"deployment.uploadTo",
		"deployment.downloadFromDefault",
		"deployment.downloadFrom",
		"deployment.deleteFromServer",
		"deployment.syncWith",
		"deployment.syncWithDefault",
		"deployment.openInBrowser",
		"deployment.toggleAutoUpload",
		"deployment.browseRemoteHost",
		"deployment.configure",
		"deployment.downloadRemoteInto",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("deployment action %s is not registered", id)
		}
	}
	spec := dockSpec(deploymentRemoteHostWindow)
	if spec == nil || spec.title != "Remote Host" || spec.anchor != dockRight {
		t.Fatalf("Remote Host tool window = %+v", spec)
	}
	for _, id := range []string{
		"deployment.serverPicker",
		"deployment.settings",
		"deployment.auth",
		"deployment.sync",
	} {
		if overlays.byID[id] == nil {
			t.Errorf("deployment overlay %s is not registered", id)
		}
	}
}

func deploymentAutoUploadModel(t *testing.T, mode openidea.AutoUpload) (*Model, string, string, chan map[string]any) {
	t.Helper()
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	uploads := make(chan map[string]any, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command == "deploy_upload" {
				uploads <- args.(map[string]any)
			}
			return json.RawMessage(`{"files":1,"skipped":0,"errors":[]}`), nil
		},
	}
	m.setRoot(root)
	state := &m.remote.deployment
	state.loaded = true
	state.settings = openidea.DeploymentSettings{
		AutoUpload:    mode,
		DefaultServer: "local",
		Servers: []openidea.DeploymentServer{{
			Kind: openidea.DeploymentLocal, ID: "local", Name: "Local",
			RootPath: filepath.Join(root, "deployed"),
			Mappings: []openidea.DeploymentMapping{{}},
		}},
	}
	return m, first, second, uploads
}

func TestDeploymentAutoUploadQueuesOnlyExplicitSavesWhenConfigured(t *testing.T) {
	m, first, second, uploads := deploymentAutoUploadModel(t, openidea.AutoUploadExplicitSave)
	state := &m.remote.deployment
	state.busy = true
	m.deploymentAutoUploadAfterSave(&Tab{doc: docbridge.Virtual(first, "one")}, false)
	m.deploymentAutoUploadAfterSave(&Tab{doc: docbridge.Virtual(second, "two")}, true)
	if len(state.autoQueue) != 2 {
		t.Fatalf("queued uploads = %d, want 2 while the server is busy", len(state.autoQueue))
	}

	state.busy = false
	m.flushDeploymentAutoUploads()
	select {
	case args := <-uploads:
		paths, _ := args["paths"].([]string)
		if !reflect.DeepEqual(paths, []string{second}) {
			t.Fatalf("auto-upload paths = %#v, want only %q", paths, second)
		}
		if args["server"] != "local" {
			t.Fatalf("auto-upload server = %#v", args["server"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("explicit save was not uploaded")
	}
	select {
	case args := <-uploads:
		t.Fatalf("unexpected second upload: %#v", args)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDeploymentAutoUploadQueueIsBoundedAndCoalescesOnePath(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	state := &m.remote.deployment
	state.autoLoading = true
	for i := 0; i < 300; i++ {
		path := filepath.Join(m.Root(), "files", string(rune('a'+i%26)), string(rune('a'+i/26))+".txt")
		m.deploymentAutoUploadAfterSave(&Tab{doc: docbridge.Virtual(path, "")}, false)
	}
	if len(state.autoQueue) != 256 {
		t.Fatalf("queue = %d, want the 256-item bound", len(state.autoQueue))
	}
	first := filepath.Join(m.Root(), "same.txt")
	state.autoQueue = nil
	state.autoLoading = true
	m.deploymentAutoUploadAfterSave(&Tab{doc: docbridge.Virtual(first, "")}, false)
	m.deploymentAutoUploadAfterSave(&Tab{doc: docbridge.Virtual(first, "")}, true)
	if len(state.autoQueue) != 1 || !state.autoQueue[0].Explicit {
		t.Fatalf("coalesced queue = %#v", state.autoQueue)
	}
}

func TestDeploymentAutoUploadNeverDoesNotCallTheBackend(t *testing.T) {
	m, first, _, uploads := deploymentAutoUploadModel(t, openidea.AutoUploadNever)
	state := &m.remote.deployment
	state.busy = true
	m.deploymentAutoUploadAfterSave(&Tab{doc: docbridge.Virtual(first, "")}, true)
	state.busy = false
	m.flushDeploymentAutoUploads()
	if len(state.autoQueue) != 0 {
		t.Fatalf("never mode retained %#v", state.autoQueue)
	}
	select {
	case args := <-uploads:
		t.Fatalf("never mode uploaded %#v", args)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDeploymentTestConnectionUsesTheTaggedConnection(t *testing.T) {
	server := openidea.DeploymentServer{
		Kind: openidea.DeploymentFTPS, ID: "site", Name: "Site",
		FTP:      openidea.FTPConnection{Host: "files.example", User: "web", Passive: true},
		RootPath: "/public",
	}
	called := make(chan map[string]any, 1)
	posted := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "deploy_test_connection" {
				t.Errorf("command = %q, want deploy_test_connection", command)
			}
			called <- args.(map[string]any)
			return json.RawMessage(`"Connected"`), nil
		},
	}
	m.testDeploymentServer(server)
	select {
	case args := <-called:
		connection, ok := args["connection"].(openidea.FsConnection)
		if !ok || connection.Kind != openidea.FsConnectionFTP || !connection.Secure ||
			connection.FTP == nil || connection.FTP.Host != "files.example" {
			t.Fatalf("connection = %#v", args["connection"])
		}
		if args["path"] != "/public" {
			t.Fatalf("path = %#v", args["path"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("deploy_test_connection was not called")
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("deploy_test_connection completion was not posted")
	}
	if m.remote.deployment.testMsg != "Connected" {
		t.Fatalf("test message = %q", m.remote.deployment.testMsg)
	}
}

func TestDeploymentAutoUploadQueueIsMainThreadBoundedStorage(t *testing.T) {
	var mu sync.Mutex
	m := &Model{postHook: func(fn func()) {
		mu.Lock()
		defer mu.Unlock()
		fn()
	}}
	m.setRoot(t.TempDir())
	state := &m.remote.deployment
	state.autoLoading = true
	for i := 0; i < 4; i++ {
		path := filepath.Join(m.Root(), filepath.Base(t.Name())+string(rune('a'+i)))
		m.deploymentAutoUploadAfterSave(&Tab{doc: docbridge.Virtual(path, "")}, false)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(state.autoQueue) != 4 {
		t.Fatalf("queue = %#v", state.autoQueue)
	}
}

func TestDeploymentAutoUploadUsesConfiguredDefaultAndDrainsInOrder(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "one.txt"),
		filepath.Join(root, "two.txt"),
		filepath.Join(root, "three.txt"),
	}
	uploads := make(chan map[string]any, 3)
	m := &Model{
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command == "deploy_upload" {
				uploads <- args.(map[string]any)
			}
			return json.RawMessage(`{"files":1,"skipped":0,"errors":[]}`), nil
		},
	}
	m.setRoot(root)
	state := &m.remote.deployment
	state.loaded = true
	state.settings = openidea.DeploymentSettings{
		AutoUpload:    openidea.AutoUploadAlways,
		DefaultServer: "production",
		Servers: []openidea.DeploymentServer{
			{ID: "first", Name: "First", Kind: openidea.DeploymentLocal},
			{ID: "production", Name: "Production", Kind: openidea.DeploymentSFTP, SSHConfig: "deploy"},
		},
	}
	for _, file := range paths {
		state.autoQueue = append(state.autoQueue, deploymentAutoUpload{Path: file, Explicit: true})
	}
	m.flushDeploymentAutoUploads()

	for i, want := range paths {
		select {
		case args := <-uploads:
			got := args["paths"].([]string)
			if len(got) != 1 || got[0] != want || args["server"] != "production" {
				t.Fatalf("upload %d = %#v", i, args)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("upload %d did not run", i)
		}
	}
	for i := 0; i < 100; i++ {
		if len(state.autoQueue) == 0 && !state.busy && state.request == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("auto-upload did not become idle: queue=%d busy=%v request=%d",
		len(state.autoQueue), state.busy, state.request)
}

func TestDeploymentAutoUploadWaitsForAuthRetryAndThenContinues(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "one.txt"), filepath.Join(root, "two.txt")}
	var uploads atomic.Int32
	secrets := make(chan map[string]any, 1)
	m := &Model{
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "deploy_upload":
				if uploads.Add(1) == 1 {
					return nil, errors.New("password required")
				}
				return json.RawMessage(`{"files":1,"skipped":0,"errors":[]}`), nil
			case "ssh_set_secret":
				secrets <- args.(map[string]any)
				return json.RawMessage(`null`), nil
			default:
				t.Fatalf("unexpected command %q", command)
				return nil, nil
			}
		},
	}
	m.setRoot(root)
	state := &m.remote.deployment
	state.loaded = true
	state.settings = openidea.DeploymentSettings{
		AutoUpload:    openidea.AutoUploadAlways,
		DefaultServer: "site",
		Servers: []openidea.DeploymentServer{{
			ID: "site", Name: "Site", Kind: openidea.DeploymentSFTP, SSHConfig: "deploy",
		}},
	}
	for _, file := range paths {
		state.autoQueue = append(state.autoQueue, deploymentAutoUpload{Path: file, Explicit: true})
	}
	m.flushDeploymentAutoUploads()

	for i := 0; i < 100 && !state.authOpen; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !state.authOpen || state.authKey != "deploy" {
		t.Fatalf("auth prompt = %+v", state)
	}
	if len(state.autoQueue) != 1 {
		t.Fatalf("queue while auth waits = %#v", state.autoQueue)
	}
	state.authValue = "correct horse battery staple"
	m.retryDeploymentAuth()
	select {
	case args := <-secrets:
		if args["id"] != "deploy" || args["secret"] != "correct horse battery staple" {
			t.Fatalf("secret args = %#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SSH secret was not stored")
	}
	for i := 0; i < 200; i++ {
		if uploads.Load() == 3 && len(state.autoQueue) == 0 && !state.busy && !state.authOpen {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("auth retry did not finish: uploads=%d queue=%d busy=%v authOpen=%v",
		uploads.Load(), len(state.autoQueue), state.busy, state.authOpen)
}
