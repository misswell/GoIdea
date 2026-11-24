package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

func TestLoadDevContainerForwardPortsMatchesOpenIDEA(t *testing.T) {
	config := filepath.Join(t.TempDir(), "devcontainer.json")
	raw := `{
		// OpenIDEA accepts JSON with comments and a trailing comma.
		"forwardPorts": [
			8000, "5173", "localhost:3000", "127.0.0.1:8000",
			"db:5432", "selenium:4444", "selenium:4444",
			0, 70000, "db:0", "db:x", "-db:1", "d b:1", null,
		],
	}`
	if err := os.WriteFile(config, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	ports, err := loadDevContainerForwardPorts(config)
	if err != nil {
		t.Fatal(err)
	}
	var own []uint16
	var hosts []string
	for _, port := range ports {
		if port.Host == nil {
			own = append(own, port.Port)
		} else {
			hosts = append(hosts, *port.Host+":"+strconv.Itoa(int(port.Port)))
		}
	}
	if want := []uint16{8000, 5173, 3000}; !reflect.DeepEqual(own, want) {
		t.Fatalf("container ports = %v, want %v", own, want)
	}
	if want := []string{"db:5432", "selenium:4444"}; !reflect.DeepEqual(hosts, want) {
		t.Fatalf("service ports = %v, want %v", hosts, want)
	}
}

func TestDockerDaemonURLIsLocalMatchesOpenIDEA(t *testing.T) {
	for _, test := range []struct {
		url   string
		local bool
	}{
		{"/var/run/docker.sock", true},
		{"unix:///Users/me/.docker/run/docker.sock", true},
		{"npipe:////./pipe/docker_engine", true},
		{"tcp://localhost:2375", true},
		{"tcp://127.0.0.1:2375", true},
		{"https://[::1]:2376", true},
		{"ssh://me@build.example.test", false},
		{"tcp://build.example.test:2375", false},
	} {
		if got := dockerDaemonURLLocal(test.url); got != test.local {
			t.Errorf("dockerDaemonURLLocal(%q) = %v, want %v", test.url, got, test.local)
		}
	}
}

func TestRemoteServerDownloadsUseThePinnedForkRelease(t *testing.T) {
	got := remoteServerReleaseBaseURL("0.1.54")
	want := "https://github.com/misswell/openidea/releases/download/v0.1.54"
	if got != want {
		t.Fatalf("remote server base URL = %q, want %q", got, want)
	}
}

func TestRemoteBackendUsesThePinnedServerVersion(t *testing.T) {
	m := &Model{}
	backend := m.newRemoteBackend(
		openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "test"},
		openidea.SSHConfiguration{},
	)
	if backend.serverVersion != "0.1.54" {
		t.Fatalf("remote backend version = %q, want 0.1.54", backend.serverVersion)
	}
}

func TestRemotePortsPersistManualForwardsButNotGeneratedContainerPorts(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetDevContainer},
	}
	configuredPort := freeTCPPort(t)
	manualPort := freeTCPPort(t)
	if _, err := backend.startConfiguredForward(configuredPort, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.startForward(manualPort, nil); err != nil {
		t.Fatal(err)
	}
	defer backend.closeForwards()
	m := &Model{
		cfg: Config{SessionPath: sessionPath},
		remote: remoteDevelopmentState{
			backend: backend,
			session: &openidea.RemoteSession{
				Target: openidea.RemoteTarget{
					Kind:   openidea.RemoteTargetDevContainer,
					Folder: "/work/app",
					Config: ".devcontainer/devcontainer.json",
				},
				Root: "/workspaces/app",
			},
			ports: backend.listForwards(),
		},
	}
	m.saveRemotePorts()
	stored := loadRemotePortStore(m.remotePortStorePath())[remotePortStoreKey(m.remote.session)]
	if want := []uint16{manualPort}; !reflect.DeepEqual(stored, want) {
		t.Fatalf("stored remote ports = %v, want only manual forward %v", stored, want)
	}
}

func TestRemotePortsRestoreManualForwardsOnReconnect(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	target := openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "build-box"}
	session := &openidea.RemoteSession{Target: target, Root: "/srv/app"}
	port := freeTCPPort(t)
	storePath := filepath.Join(filepath.Dir(sessionPath), "remote-ports.json")
	if err := saveRemotePortStore(storePath, map[string][]uint16{
		remotePortStoreKey(session): {port},
	}); err != nil {
		t.Fatal(err)
	}
	backend := &remoteBackend{target: target, runner: defaultRemoteRunner}
	m := &Model{
		cfg: Config{SessionPath: sessionPath},
		remote: remoteDevelopmentState{
			backend: backend,
			session: session,
		},
	}
	m.restoreRemotePorts()
	t.Cleanup(backend.closeForwards)
	forwards := backend.listForwards()
	if len(forwards) != 1 || forwards[0].RemotePort != port {
		t.Fatalf("restored forwards = %+v, want remote port %d", forwards, port)
	}
	if len(m.remote.ports) != 1 || m.remote.ports[0].RemotePort != port {
		t.Fatalf("tool-window ports = %+v", m.remote.ports)
	}
}

func TestRemotePortStopClosesListenerAndRemovesPersistence(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	target := openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "build-box"}
	session := &openidea.RemoteSession{Target: target, Root: "/srv/app"}
	port := freeTCPPort(t)
	backend := &remoteBackend{target: target, runner: defaultRemoteRunner}
	forwarded, err := backend.startForward(port, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{
		cfg: Config{SessionPath: sessionPath},
		remote: remoteDevelopmentState{
			backend: backend,
			session: session,
			ports:   backend.listForwards(),
		},
	}
	m.saveRemotePorts()
	key := remotePortStoreKey(session)
	if stored := loadRemotePortStore(m.remotePortStorePath())[key]; len(stored) != 1 ||
		stored[0] != port {
		t.Fatalf("stored remote ports before stop = %v, want [%d]", stored, port)
	}

	m.stopRemoteForward(port, nil)
	if forwards := backend.listForwards(); len(forwards) != 0 {
		t.Fatalf("stopped forward still listed: %+v", forwards)
	}
	if stored := loadRemotePortStore(m.remotePortStorePath())[key]; len(stored) != 0 {
		t.Fatalf("stopped forward remained persisted: %v", stored)
	}
	listener, err := net.Listen(
		"tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(int(forwarded.LocalPort))),
	)
	if err != nil {
		t.Fatalf("stopped forward left local port %d open: %v", forwarded.LocalPort, err)
	}
	_ = listener.Close()
}

func TestRemotePortsRestoreFailureIsVisible(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	target := openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "build-box"}
	session := &openidea.RemoteSession{Target: target, Root: "/srv/app"}
	port := freeTCPPort(t)
	storePath := filepath.Join(filepath.Dir(sessionPath), "remote-ports.json")
	if err := saveRemotePortStore(storePath, map[string][]uint16{
		remotePortStoreKey(session): {port},
	}); err != nil {
		t.Fatal(err)
	}
	backend := &remoteBackend{target: target, runner: defaultRemoteRunner}
	backend.forwarder = newRemotePortForwarder(backend)
	backend.forwarder.Close()
	m := &Model{
		cfg: Config{SessionPath: sessionPath},
		remote: remoteDevelopmentState{
			backend: backend,
			session: session,
		},
	}
	m.restoreRemotePorts()
	if !strings.Contains(m.remote.portsErr, strconv.Itoa(int(port))) ||
		!strings.Contains(m.remote.portsErr, "remote connection is closed") {
		t.Fatalf("restore failure = %q", m.remote.portsErr)
	}
}

func TestDevContainerRejectsADockerContextOnAnotherMachine(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, ".devcontainer.json"), []byte(`{"image":"ubuntu"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "build-box")
	runner := remoteProcessRunner{
		lookPath: func(name string) (string, error) { return name + "-fake", nil },
		command: func(ctx context.Context, program string, args ...string) *exec.Cmd {
			if program != "docker-fake" || strings.Join(args, " ") !=
				"context inspect --format {{.Endpoints.docker.Host}} build-box" {
				t.Fatalf("docker command = %s %v", program, args)
			}
			return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'ssh://me@build.example.test\\n'")
		},
		http: defaultRemoteRunner.http,
	}
	backend := &remoteBackend{
		target: openidea.RemoteTarget{
			Kind: openidea.RemoteTargetDevContainer, Folder: folder,
		},
		runner: runner,
	}
	err := backend.prepareContainer(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "not on this machine") {
		t.Fatalf("remote Docker context error = %v", err)
	}
}

func TestFindDevContainerConfigUsesPinnedLocations(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}
	preferred := filepath.Join(root, ".devcontainer", "devcontainer.json")
	fallback := filepath.Join(root, ".devcontainer.json")
	recursive := filepath.Join(root, ".devcontainer", "nested", "devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(recursive), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{preferred, fallback, recursive} {
		if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := findDevContainerConfig(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if found != preferred {
		t.Fatalf("config = %q, want %q", found, preferred)
	}
	if err := os.Remove(preferred); err != nil {
		t.Fatal(err)
	}
	found, err = findDevContainerConfig(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if found != fallback {
		t.Fatalf("fallback config = %q, want %q", found, fallback)
	}
}

func TestDevContainerCLIUsesTheBackendManagedToolStatus(t *testing.T) {
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "tools_list" || args != nil {
				t.Fatalf("command = %q, args = %#v", command, args)
			}
			return json.RawMessage(`[
				{"name":"devcontainer","status":"installed","path":"/managed/devcontainer","version":"0.91.0","canInstall":false}
			]`), nil
		},
	}
	status := m.loadDevContainerCLI(context.Background())
	if status.Status != "installed" || status.Path != "/managed/devcontainer" ||
		status.Version != "0.91.0" || status.Message != "Managed by GoIDEA" {
		t.Fatalf("managed CLI status = %+v", status)
	}

	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`[
			{"name":"devcontainer","status":"missing","canInstall":true}
		]`), nil
	}
	status = m.loadDevContainerCLI(context.Background())
	if status.Status != "missing" || status.Message != "Not installed · GoIDEA can install it" {
		t.Fatalf("missing CLI status = %+v", status)
	}
}

func TestRemoteSSHSettingsSaveUsesCanonicalKeyAndBackendValues(t *testing.T) {
	type call struct {
		key   string
		value string
	}
	calls := make(chan call, 1)
	done := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			done <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Fatalf("command = %q, want settings_set", command)
			}
			input := args.(map[string]any)
			calls <- call{key: input["key"].(string), value: input["value"].(string)}
			return json.RawMessage(`{
				"ssh": {"configs": [{
					"id": "box", "name": "Box", "host": "dev.example.test",
					"port": 2222, "user": "developer", "auth": "openSsh",
					"keyFile": "", "saveSecret": false
				}]}
			}`), nil
		},
	}
	m.remote.sshConfigs = []openidea.SSHConfiguration{{
		ID: "box", Name: "Old name", Host: "old.example.test", Auth: "openSsh",
	}}
	m.remote.sshSelected = 0
	m.remote.sshEditing = false
	m.remote.sshName = "Box"
	m.remote.sshHost = "dev.example.test"
	m.remote.sshPort = "2222"
	m.remote.sshUser = "developer"
	m.remote.sshAuth = "openSsh"
	m.saveRemoteSSHConfiguration()

	var got call
	select {
	case got = <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("settings_set was not called")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("saved SSH settings were not applied")
	}
	if got.key != "ssh.configs" {
		t.Fatalf("settings key = %q, want ssh.configs", got.key)
	}
	var saved []openidea.SSHConfiguration
	if err := json.Unmarshal([]byte(got.value), &saved); err != nil {
		t.Fatalf("settings value is not a configuration array: %v", err)
	}
	if len(saved) != 1 || saved[0].ID != "box" || saved[0].Port == nil || *saved[0].Port != 2222 {
		t.Fatalf("saved configurations = %+v", saved)
	}
	if m.remote.sshSettingsBusy || len(m.remote.sshConfigs) != 1 ||
		m.remote.sshConfigs[0].User != "developer" {
		t.Fatalf("canonical backend settings were not applied: %+v", m.remote)
	}
}

func TestRemoteTreeLoadsPosixFoldersWithoutLocalFiles(t *testing.T) {
	root := "/srv/example"
	loader := func(dir string, done func([]openidea.RemoteDirectoryEntry, error)) {
		switch dir {
		case root:
			done([]openidea.RemoteDirectoryEntry{
				{Name: "src", Path: "/srv/example/src", IsDir: true},
				{Name: "README.md", Path: "/srv/example/README.md"},
			}, nil)
		case "/srv/example/src":
			done([]openidea.RemoteDirectoryEntry{
				{Name: "Main.java", Path: "/srv/example/src/Main.java"},
			}, nil)
		default:
			done(nil, errors.New("unexpected folder "+dir))
		}
	}
	tree := NewRemoteTree(root, loader, func(fn func()) { fn() }, func() {})
	defer tree.Close()
	if !treeEntryVisible(tree, "/srv/example/src") {
		t.Fatalf("remote folder missing from rows: %+v", tree.Rows())
	}
	tree.Toggle("/srv/example/src")
	if !treeEntryVisible(tree, "/srv/example/src/Main.java") {
		t.Fatalf("remote child missing after expansion: %+v", tree.Rows())
	}
	if got := tree.IsDir("/srv/example/src"); !got {
		t.Fatal("remote source row is not a directory")
	}
}

func TestRemoteProjectPathsStayPosixAndCaseSensitive(t *testing.T) {
	root := "/srv/Example"
	tree := NewRemoteTree(root, func(string, func([]openidea.RemoteDirectoryEntry, error)) {},
		func(fn func()) { fn() }, func() {})
	defer tree.Close()
	m := &Model{
		remote: remoteDevelopmentState{
			session: &openidea.RemoteSession{
				Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH},
				Root:   root,
				OS:     "linux",
			},
		},
	}
	m.setRoot(root)
	m.tree = tree
	if !m.projectTreeUnderPath(root+"/src/Main.java", root+"/src") {
		t.Fatal("remote child is not under its remote folder")
	}
	if m.projectTreeUnderPath(root+"/SRC/Main.java", root+"/src") {
		t.Fatal("a Linux remote path was treated as case-insensitive")
	}
	got := m.topLevelTreePaths([]string{root + "/src", root + "/SRC"})
	if !reflect.DeepEqual(got, []string{root + "/src", root + "/SRC"}) {
		t.Fatalf("case-distinct remote paths were merged: %v", got)
	}
	dropped := m.projectTreeDropPlan(
		[]string{root + "/src"},
		root+"/src/generated",
		false,
	)
	if len(dropped) != 0 {
		t.Fatalf("folder was allowed to drop into itself: %v", dropped)
	}
}

func TestRemoteReloadReadsThroughTheBackendNotTheLocalFilesystem(t *testing.T) {
	root := "/srv/app"
	file := root + "/index.html"
	backend := &remoteReloadTestBackend{path: file}
	doc, err := docbridge.Open(context.Background(), backend, Window, file)
	if err != nil {
		t.Fatal(err)
	}
	tree := NewRemoteTree(root, func(string, func([]openidea.RemoteDirectoryEntry, error)) {},
		func(fn func()) { fn() }, func() {})
	defer tree.Close()
	finished := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "fs_read_text" {
				t.Fatalf("command = %q, want fs_read_text", command)
			}
			if got := args.(map[string]any)["path"]; got != file {
				t.Fatalf("remote reload path = %v, want %s", got, file)
			}
			return json.RawMessage(`"<html>reloaded</html>"`), nil
		},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
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
	m.tree = tree
	tab := &Tab{doc: doc}
	m.tabs = []*Tab{tab}
	m.reloadTab(tab)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("remote reload did not finish")
	}
	if got := doc.Text(); got != "<html>reloaded</html>" {
		t.Fatalf("remote reloaded text = %q", got)
	}
}

func TestWebOpenUsesRemoteURLAndForwardingHook(t *testing.T) {
	root := "/srv/app"
	file := root + "/index.html"
	opened := make(chan string, 1)
	finished := make(chan struct{}, 1)
	oldOpen := openRemoteWebPage
	openRemoteWebPage = func(url string) { opened <- url }
	defer func() { openRemoteWebPage = oldOpen }()
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: "Ubuntu"},
	}
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "web_url" {
				t.Fatalf("command = %q, want web_url", command)
			}
			if got := args.(map[string]any)["path"]; got != file {
				t.Fatalf("web_url path = %v, want %s", got, file)
			}
			return json.RawMessage(`"http://127.0.0.1:5173/"`), nil
		},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		remote: remoteDevelopmentState{
			session: &openidea.RemoteSession{
				Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: "Ubuntu"},
				Root:   root,
				Host:   "Ubuntu",
			},
			backend: backend,
		},
	}
	m.setRoot(root)
	m.OpenWebInBrowser(map[string]any{"path": file})
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("web open did not finish")
	}
	select {
	case url := <-opened:
		if url != "http://127.0.0.1:5173/" {
			t.Fatalf("opened URL = %q", url)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("browser was not opened")
	}
}

type webSaveTestBackend struct {
	next  uint32
	docs  map[string]openidea.DocInfo
	saved []string
}

func (b *webSaveTestBackend) DocOpen(
	_ context.Context,
	_,
	path string,
) (openidea.DocInfo, error) {
	if b.docs == nil {
		b.docs = map[string]openidea.DocInfo{}
	}
	b.next++
	info := openidea.DocInfo{ID: b.next, Path: path, Text: "old", Version: 1}
	b.docs[path] = info
	return info, nil
}

func (b *webSaveTestBackend) DocEdit(
	_ context.Context,
	_ string,
	id uint32,
	_ uint32,
	_ position.ChangeSet,
) (uint32, error) {
	for path, info := range b.docs {
		if info.ID == id {
			info.Version++
			b.docs[path] = info
			return info.Version, nil
		}
	}
	return 0, errors.New("unknown document")
}

func (b *webSaveTestBackend) DocSnapshot(
	_ context.Context,
	_ string,
	id uint32,
) (openidea.DocInfo, error) {
	for _, info := range b.docs {
		if info.ID == id {
			return info, nil
		}
	}
	return openidea.DocInfo{}, errors.New("unknown document")
}

func (b *webSaveTestBackend) DocSave(_ context.Context, _ string, id uint32) error {
	for path, info := range b.docs {
		if info.ID == id {
			b.saved = append(b.saved, path)
			return nil
		}
	}
	return errors.New("unknown document")
}

func (b *webSaveTestBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func TestWebPreviewSavesEveryDirtyTabAndUsesTheExternalBrowser(t *testing.T) {
	htmlPath := "/srv/app/index.html"
	scriptPath := "/srv/app/app.js"
	backend := &webSaveTestBackend{}
	htmlDoc, err := docbridge.Open(context.Background(), backend, Window, htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	scriptDoc, err := docbridge.Open(context.Background(), backend, Window, scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	htmlDoc.Buffer().Reset("<html><body>new</body></html>")
	scriptDoc.Buffer().Reset("console.log('new');")
	if !htmlDoc.ModifiedFromSaved() || !scriptDoc.ModifiedFromSaved() {
		t.Fatal("web preview fixture documents are not dirty")
	}

	opened := make(chan string, 1)
	finished := make(chan struct{}, 1)
	oldOpen := openRemoteWebPage
	openRemoteWebPage = func(url string) { opened <- url }
	defer func() { openRemoteWebPage = oldOpen }()
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "web_url" {
				t.Fatalf("command = %q, want web_url", command)
			}
			if got := args.(map[string]any)["path"]; got != htmlPath {
				t.Fatalf("web_url path = %v, want %s", got, htmlPath)
			}
			return json.RawMessage(`"http://127.0.0.1:5173/"`), nil
		},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		tabs:  []*Tab{{doc: htmlDoc}},
		split: &splitGroup{tabs: []*Tab{{doc: scriptDoc}}},
	}
	m.setRoot("/srv/app")
	if !m.RunActionWithArg("web.togglePreview", map[string]any{"path": htmlPath}) {
		t.Fatal("Built-in Preview action did not run for an HTML editor")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("web preview did not finish opening")
	}
	select {
	case url := <-opened:
		if url != "http://127.0.0.1:5173/" {
			t.Fatalf("opened URL = %q", url)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("system browser was not opened")
	}
	saved := map[string]bool{}
	for _, path := range backend.saved {
		saved[path] = true
	}
	if !saved[htmlPath] || !saved[scriptPath] {
		t.Fatalf("saved documents = %v, want both dirty tabs", backend.saved)
	}
	if !m.remote.webPreviews[htmlPath] || !webPreviewToggled(m) {
		t.Fatal("Built-in Preview did not record its on state")
	}

	if !m.RunActionWithArg("web.togglePreview", map[string]any{"path": htmlPath}) {
		t.Fatal("Built-in Preview could not be toggled off")
	}
	if m.remote.webPreviews[htmlPath] || webPreviewToggled(m) {
		t.Fatal("Built-in Preview stayed on")
	}
}

func treeEntryVisible(tree *Tree, path string) bool {
	for _, row := range tree.Rows() {
		if row.path == path {
			return true
		}
	}
	return false
}

type remoteReloadTestBackend struct {
	path string
}

func (b *remoteReloadTestBackend) DocOpen(
	_ context.Context,
	_,
	path string,
) (openidea.DocInfo, error) {
	b.path = path
	return openidea.DocInfo{ID: 1, Path: path, Text: "old", Version: 1}, nil
}

func (b *remoteReloadTestBackend) DocEdit(
	_ context.Context,
	_ string,
	_ uint32,
	_ uint32,
	_ position.ChangeSet,
) (uint32, error) {
	return 2, nil
}

func (b *remoteReloadTestBackend) DocSnapshot(
	context.Context,
	string,
	uint32,
) (openidea.DocInfo, error) {
	return openidea.DocInfo{ID: 1, Path: b.path, Text: "old", Version: 1}, nil
}

func (b *remoteReloadTestBackend) DocSave(context.Context, string, uint32) error {
	return nil
}

func (b *remoteReloadTestBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func TestRemoteConfiguredAndWebPortsStaySeparated(t *testing.T) {
	configuredPort := freeTCPPort(t)
	webPort := freeTCPPort(t)
	backend := &remoteBackend{
		target: openidea.RemoteTarget{
			Kind: openidea.RemoteTargetDevContainer, Folder: "/local/project",
		},
		runner: defaultRemoteRunner,
	}
	config := filepath.Join(t.TempDir(), "devcontainer.json")
	raw := `{"forwardPorts":[` + strconv.Itoa(int(configuredPort)) + `]}`
	if err := os.WriteFile(config, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	backend.devContainerConfig = config
	if err := backend.forwardConfiguredPorts(); err != nil {
		t.Fatal(err)
	}
	forwards := backend.listForwards()
	if len(forwards) != 1 || forwards[0].RemotePort != configuredPort {
		t.Fatalf("configured forwards = %+v", forwards)
	}
	webURL, err := backend.forwardWebURL("http://127.0.0.1:" + strconv.Itoa(int(webPort)) + "/preview")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(webURL, "http://127.0.0.1:") || !strings.HasSuffix(webURL, "/preview") {
		t.Fatalf("forwarded web URL = %q", webURL)
	}
	if got := backend.listForwards(); len(got) != 1 {
		t.Fatalf("web forwarding leaked into the Ports list: %+v", got)
	}
	backend.closeForwards()
}

func freeTCPPort(t *testing.T) uint16 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return uint16(port)
}

func TestOpenSSHUnknownHostStillOffersTrust(t *testing.T) {
	runner := remoteProcessRunner{
		lookPath: func(name string) (string, error) {
			return name + "-fake", nil
		},
		command: func(ctx context.Context, program string, _ ...string) *exec.Cmd {
			switch program {
			case "ssh":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'hostname actual.example.test\\nport 22\\n'")
			case "ssh-keyscan-fake":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'actual.example.test ssh-ed25519 AAAA\\n'")
			case "ssh-keygen-fake":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf '256 SHA256:test-fingerprint ED25519\\n'")
			default:
				return exec.CommandContext(ctx, "/usr/bin/false")
			}
		},
		http: defaultRemoteRunner.http,
	}
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "box"},
		ssh:    openidea.SSHConfiguration{ID: "box", Host: "box", Auth: "openSsh"},
		runner: runner,
	}
	err := backend.classifyError(
		context.Background(),
		errors.New("exit status 255"),
		"Host key verification failed.",
	)
	var trust *remoteHostTrustError
	if !errors.As(err, &trust) {
		t.Fatalf("OpenSSH auth error = %T %v, want host-trust prompt", err, err)
	}
	if trust.Fingerprint != "SHA256:test-fingerprint" {
		t.Fatalf("fingerprint = %q", trust.Fingerprint)
	}
}

func TestTrustHostUsesTheEffectiveOpenSSHHostAndPort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runner := remoteProcessRunner{
		lookPath: func(name string) (string, error) {
			return name + "-fake", nil
		},
		command: func(ctx context.Context, program string, _ ...string) *exec.Cmd {
			switch program {
			case "ssh":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'hostname actual.example.test\\nport 2222\\n'")
			case "ssh-keyscan-fake":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf '[actual.example.test]:2222 ssh-ed25519 AAAA\\n'")
			default:
				return exec.CommandContext(ctx, "/usr/bin/false")
			}
		},
		http: defaultRemoteRunner.http,
	}
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "box"},
		ssh:    openidea.SSHConfiguration{ID: "box", Host: "box", Auth: "openSsh"},
		runner: runner,
	}
	if err := backend.trustHost(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(raw)), "[actual.example.test]:2222 ssh-ed25519 AAAA"; got != want {
		t.Fatalf("known_hosts line = %q, want %q", got, want)
	}
}

func TestTrustHostRecordsOnlyTheKeyItShows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var scanned []string
	runner := remoteProcessRunner{
		lookPath: func(name string) (string, error) {
			return name + "-fake", nil
		},
		command: func(ctx context.Context, program string, args ...string) *exec.Cmd {
			switch program {
			case "ssh":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'hostname actual.example.test\\nport 2222\\n'")
			case "ssh-keyscan-fake":
				scanned = append([]string(nil), args...)
				return exec.CommandContext(ctx, "/bin/sh", "-c",
					"printf '[actual.example.test]:2222 ssh-ed25519 ED\\n[actual.example.test]:2222 ecdsa-sha2-nistp256 EC\\n[actual.example.test]:2222 ssh-rsa RSA\\n'")
			case "ssh-keygen-fake":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf '256 SHA256:shown ED25519\\n'")
			default:
				return exec.CommandContext(ctx, "/usr/bin/false")
			}
		},
		http: defaultRemoteRunner.http,
	}
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "box"},
		ssh:    openidea.SSHConfiguration{ID: "box", Host: "box", Auth: "openSsh"},
		runner: runner,
	}
	fingerprint, err := backend.hostKeyFingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != "SHA256:shown" {
		t.Fatalf("fingerprint = %q", fingerprint)
	}
	if err := backend.trustHost(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(scanned, " "), "ed25519") {
		t.Fatalf("ssh-keyscan arguments = %v", scanned)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 || !strings.HasSuffix(lines[0], " ssh-ed25519 ED") {
		t.Fatalf("known_hosts = %q", raw)
	}
}

func TestTrustHostMatchingRejectsAKeyThatChangedAfterThePrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scans := 0
	runner := remoteProcessRunner{
		lookPath: func(name string) (string, error) {
			return name + "-fake", nil
		},
		command: func(ctx context.Context, program string, _ ...string) *exec.Cmd {
			switch program {
			case "ssh":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'hostname actual.example.test\\nport 2222\\n'")
			case "ssh-keyscan-fake":
				scans++
				return exec.CommandContext(ctx, "/bin/sh", "-c",
					"printf '[actual.example.test]:2222 ssh-ed25519 REPLACEMENT\\n'")
			case "ssh-keygen-fake":
				return exec.CommandContext(ctx, "/bin/sh", "-c", "printf '256 SHA256:replacement ED25519\\n'")
			default:
				return exec.CommandContext(ctx, "/usr/bin/false")
			}
		},
		http: defaultRemoteRunner.http,
	}
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "box"},
		ssh:    openidea.SSHConfiguration{ID: "box", Host: "box", Auth: "openSsh"},
		runner: runner,
	}
	err := backend.trustHostMatching(context.Background(), "SHA256:shown")
	if err == nil || !strings.Contains(err.Error(), "changed while connecting") {
		t.Fatalf("trustHostMatching error = %v, want changed-key refusal", err)
	}
	if scans != 1 {
		t.Fatalf("ssh-keyscan ran %d times, want one scan", scans)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "known_hosts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("known_hosts exists after refusal: %v", err)
	}
}

func TestRemoteLocationRoundTrip(t *testing.T) {
	tests := []struct {
		target openidea.RemoteTarget
		root   string
	}{
		{
			target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev box"},
			root:   "/srv/My Project",
		},
		{
			target: openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: "Ubuntu-24.04"},
			root:   "/home/user/project",
		},
		{
			target: openidea.RemoteTarget{
				Kind: openidea.RemoteTargetDevContainer, Folder: "/Users/me/My Project",
				Config: ".devcontainer/devcontainer.json",
			},
			root: "/workspaces/My Project",
		},
	}
	for _, test := range tests {
		location := remoteProjectLocation(test.target, test.root)
		target, root, ok := parseRemoteProjectLocation(location)
		if !ok || target != test.target || root != test.root {
			t.Errorf("round trip %q = %+v %q %v", location, target, root, ok)
		}
	}
}

func TestRemoteLocationAcceptsWindowsWSLUNC(t *testing.T) {
	for _, location := range []string{
		`\\wsl$\Ubuntu-24.04\home\user\project`,
		`\\wsl.localhost\Ubuntu-24.04\home\user\project`,
	} {
		target, root, ok := parseRemoteProjectLocation(location)
		if !ok || target.Kind != openidea.RemoteTargetWSL ||
			target.Distro != "Ubuntu-24.04" || root != "/home/user/project" {
			t.Fatalf("parse %q = %+v %q %v", location, target, root, ok)
		}
	}
	if _, _, ok := parseRemoteProjectLocation(`\\server\share\project`); ok {
		t.Fatal("an ordinary Windows UNC share was treated as WSL")
	}
}

func TestRemoteChecksumAndPlatformValidation(t *testing.T) {
	sum := strings.Repeat("a", 64)
	text := strings.Repeat("b", 64) + "  other.gz\n" + sum + "  openidea-server-linux-x86_64.gz\n"
	if got := parseRemoteChecksum(text, "openidea-server-linux-x86_64.gz"); got != sum {
		t.Fatalf("checksum = %q, want %q", got, sum)
	}
	if !supportedRemotePlatform("linux", "aarch64") || supportedRemotePlatform("windows", "x86_64") {
		t.Fatal("remote platform support does not match the release assets")
	}
}
