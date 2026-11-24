package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestRemoteActionsAppearInCommandPalette(t *testing.T) {
	want := []string{
		"remote.connect",
		"remote.disconnect",
		"remote.reloadEnvironment",
		"remote.reopenInContainer",
		"remote.showToolWindow",
		"ssh.startSession",
		"ssh.configurations",
		"web.openInBrowser",
		"web.togglePreview",
	}
	seen := map[string]bool{}
	for _, cmd := range commands() {
		seen[cmd.id] = true
	}
	for _, id := range want {
		if _, ok := actionGet(id); !ok {
			t.Errorf("missing registered action %s", id)
		}
		if !seen[id] {
			t.Errorf("action %s is absent from the command palette", id)
		}
	}
}

func TestRemoteToolWindowAndDialogsAreRegistered(t *testing.T) {
	spec := dockSpec(remoteDevelopmentWindow)
	if spec == nil {
		t.Fatal("Remote Development tool window is not registered")
	}
	if spec.title != "Remote Development" || spec.anchor != dockBottom {
		t.Fatalf("Remote Development window = %q at %v, want bottom", spec.title, spec.anchor)
	}
	for _, id := range []string{
		"remote.connect", "remote.disconnect", "remote.notice", "ssh.session",
	} {
		if overlay := overlays.byID[id]; overlay == nil {
			t.Errorf("overlay %s is not registered", id)
		}
	}
}

func TestRemoteToolWindowShowsHostAndPortStates(t *testing.T) {
	m := &Model{}
	tester := ui.NewTester(func(c *ui.Context) {
		m.remoteDevelopmentPanel(c, c.Theme())
	}, 960, 420)
	tester.Frame()
	for _, want := range []string{
		"Remote Development",
		"Remote Host",
		"Not connected",
		"GoIDEA is using its local backend.",
		"Ports",
		"No remote session",
		"Connect to an SSH host or dev container to forward ports.",
	} {
		if !tester.HasText(want) {
			t.Errorf("Remote Development window lacks %q: %q", want, tester.Texts())
		}
	}

	m.remote.session = &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
		Root:   "/srv/app",
		Host:   "dev@example.test:22",
	}
	tester = ui.NewTester(func(c *ui.Context) {
		m.remoteDevelopmentPanel(c, c.Theme())
	}, 960, 420)
	tester.Frame()
	for _, want := range []string{
		"SSH: dev-box", "0 forwarded", "Forward Port", "No forwarded ports",
		"Forward a port of the remote host to reach it from this machine.",
	} {
		if !tester.HasText(want) {
			t.Errorf("connected Remote Development window lacks %q: %q", want, tester.Texts())
		}
	}

	m.remote.session.Target = openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: "Ubuntu-24.04"}
	tester = ui.NewTester(func(c *ui.Context) {
		m.remoteDevelopmentPanel(c, c.Theme())
	}, 960, 420)
	tester.Frame()
	if !tester.HasText("Forwarding ports is not supported in a WSL project yet.") {
		t.Fatalf("WSL port state missing: %q", tester.Texts())
	}
}

func TestRemoteEnvironmentReloadSuccess(t *testing.T) {
	devcontainer := "/project/.devcontainer/devcontainer.json"
	calls := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "env_reload" {
				t.Errorf("RPC command = %q, want env_reload", command)
			}
			if args != nil {
				t.Errorf("env_reload args = %#v, want nil", args)
			}
			calls <- struct{}{}
			return json.RawMessage(`{"changed":["PATH","JAVA_HOME"],"devcontainer":"/project/.devcontainer/devcontainer.json","shellRead":true}`), nil
		},
	}
	m.setRoot(t.TempDir())
	m.postHook = func(fn func()) {
		fn()
		done <- struct{}{}
	}

	if !m.RunAction("remote.reloadEnvironment") {
		t.Fatal("reload environment action could not run in an open project")
	}
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("env_reload was not called")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("environment reload did not finish")
	}

	state := &m.remote
	if state.envBusy {
		t.Fatal("environment reload stayed busy after success")
	}
	if !state.envLoaded || state.envErr != "" {
		t.Fatalf("environment result = loaded %v, error %q", state.envLoaded, state.envErr)
	}
	if len(state.envChanged) != 2 || state.envChanged[0] != "PATH" || state.envChanged[1] != "JAVA_HOME" {
		t.Fatalf("changed variables = %v", state.envChanged)
	}
	if state.envDevcontainer == nil || *state.envDevcontainer != devcontainer {
		t.Fatalf("devcontainer = %v, want %q", state.envDevcontainer, devcontainer)
	}
	if !state.envShellRead {
		t.Fatal("shellRead was not decoded")
	}
	summary := m.remoteEnvironmentSummary()
	for _, want := range []string{"PATH", "JAVA_HOME", devcontainer, "login shell was read"} {
		if !strings.Contains(summary, want) {
			t.Errorf("environment summary lacks %q: %q", want, summary)
		}
	}
}

func TestRemoteEnvironmentReloadErrorClearsBusyState(t *testing.T) {
	done := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return nil, errors.New("socket closed")
		},
	}
	m.setRoot(t.TempDir())
	m.postHook = func(fn func()) {
		fn()
		done <- struct{}{}
	}
	if !m.RunAction("remote.reloadEnvironment") {
		t.Fatal("reload environment action could not run in an open project")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("failed environment reload did not finish")
	}
	if m.remote.envBusy {
		t.Fatal("environment reload stayed busy after an error")
	}
	if !strings.Contains(m.remote.envErr, "socket closed") || !strings.Contains(m.remote.envErr, "Could not reload") {
		t.Fatalf("environment error = %q", m.remote.envErr)
	}
}

func TestRemoteConnectDialogUsesSSHAndWSLConfigurations(t *testing.T) {
	m := &Model{}
	m.remote.connectOpen = true
	m.remote.configsLoaded = true
	m.remote.connectTarget = openidea.RemoteTargetSSH
	m.remote.connectPath = "/srv/app"
	m.remote.sshConfigs = []openidea.SSHConfiguration{{
		ID: "dev-box", Name: "Development box", Host: "dev.example.test",
		Auth: "openSsh",
	}}
	m.remote.sshChoice = "Development box"
	m.remote.distros = []string{"Ubuntu-24.04"}
	m.remote.distro = "Ubuntu-24.04"
	if !m.RunAction("remote.connect") {
		t.Fatal("Remote Development action did not run")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.remoteConnectDialog(c, c.Theme())
	}, 760, 520)
	tester.Frame()
	for _, want := range []string{
		"Remote Development", "Connect through", "SSH",
		"SSH configuration", "Development box", "Project directory",
		"Open", "Cancel",
	} {
		if !tester.HasText(want) {
			t.Errorf("connect dialog lacks %q: %q", want, tester.Texts())
		}
	}

	m.remote.connectTarget = openidea.RemoteTargetWSL
	tester = ui.NewTester(func(c *ui.Context) {
		m.remoteConnectDialog(c, c.Theme())
	}, 760, 520)
	tester.Frame()
	for _, want := range []string{"WSL", "WSL distribution", "Ubuntu-24.04", "Project directory"} {
		if !tester.HasText(want) {
			t.Errorf("WSL connect dialog lacks %q: %q", want, tester.Texts())
		}
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".devcontainer", "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	posted := make(chan func(), 16)
	m = &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			posted <- fn
		},
	}
	m.setRoot(root)
	if !m.RunAction("remote.reopenInContainer") {
		t.Fatal("Reopen in Container action did not run")
	}
	deadline := time.After(2 * time.Second)
	for m.remote.connectErr == "" {
		select {
		case fn := <-posted:
			fn()
		case <-deadline:
			t.Fatal("dev-container connection attempt did not finish")
		}
	}
	if m.remote.pendingTarget.Kind != openidea.RemoteTargetDevContainer ||
		m.remote.pendingTarget.Folder != root {
		t.Fatalf("dev-container target = %+v", m.remote.pendingTarget)
	}
	if !strings.Contains(m.remote.connectErr, "backend manager is unavailable") {
		t.Fatalf("dev-container attempt error = %q", m.remote.connectErr)
	}
}

func TestRemoteDisconnectClosesTheRemoteSession(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	if !m.RunAction("remote.disconnect") {
		t.Fatal("Disconnect action did not run")
	}
	if !m.remote.disconnectOpen {
		t.Fatal("Disconnect did not open a confirmation")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.remoteDisconnectDialog(c, c.Theme())
	}, 760, 420)
	tester.Frame()
	for _, want := range []string{
		"Disconnect from Remote Host?", "No remote session is connected.",
		"Disconnect", "Cancel",
	} {
		if !tester.HasText(want) {
			t.Errorf("disconnect confirmation lacks %q: %q", want, tester.Texts())
		}
	}

	done := make(chan struct{}, 1)
	m = &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			done <- struct{}{}
		},
	}
	m.remote.session = &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
		Root:   "/srv/app", Host: "dev@example.test:22",
	}
	m.setRoot("/srv/app")
	m.showingWelcome = false
	m.confirmRemoteDisconnect()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect did not finish")
	}
	if m.Root() != "" || m.remote.session != nil || !m.showingWelcome {
		t.Fatalf("disconnected state: root=%q session=%v welcome=%v",
			m.Root(), m.remote.session, m.showingWelcome)
	}
}

func TestRemoteMenusExposeWindowAndActions(t *testing.T) {
	var file, tools, view menuEntry
	for _, group := range mainMenuGroups() {
		switch group.label {
		case "File":
			file = group
		case "Tools":
			tools = group
		case "View":
			view = group
		}
	}
	if file.label == "" || tools.label == "" || view.label == "" {
		t.Fatal("main menu is missing File, Tools or View")
	}

	var fileRemote menuEntry
	for _, item := range file.sub {
		if item.label == "Remote Development" {
			fileRemote = item
			break
		}
	}
	if len(fileRemote.sub) == 0 {
		t.Fatal("File has no Remote Development submenu")
	}
	fileCommands := map[string]bool{}
	for _, item := range fileRemote.sub {
		fileCommands[item.command] = true
	}
	for _, id := range []string{
		"remote.connect", "remote.reloadEnvironment", "remote.reopenInContainer", "remote.disconnect",
	} {
		if !fileCommands[id] {
			t.Errorf("File › Remote Development lacks %s", id)
		}
	}

	var toolWindows menuEntry
	for _, item := range view.sub {
		if item.label == "Tool Windows" {
			toolWindows = item
			break
		}
	}
	found := false
	for _, item := range toolWindows.sub {
		if item.label == "Remote Development" && item.command == "remote.showToolWindow" {
			found = true
		}
	}
	if !found {
		t.Fatal("View › Tool Windows has no Remote Development entry")
	}

	sshCommands := map[string]bool{}
	for _, item := range tools.sub {
		if strings.HasPrefix(item.command, "ssh.") {
			sshCommands[item.command] = true
		}
	}
	for _, id := range []string{"ssh.startSession", "ssh.configurations"} {
		if !sshCommands[id] {
			t.Errorf("Tools menu lacks %s", id)
		}
	}
}

func TestRemoteSSHSessionChooserAndArgumentsMatchOpenIDEA(t *testing.T) {
	port := 2222
	config := openidea.SSHConfiguration{
		ID: "dev-box", Name: "Development box", Host: "dev.example.test",
		Port: &port, User: "dev", Auth: "keyPair", KeyFile: "~/.ssh/id_ed25519",
	}
	configs := []openidea.SSHConfiguration{config}
	args := remoteOpenSSHArgs(config)
	want := []string{
		"-p", "2222", "-i", expandUserPath("~/.ssh/id_ed25519"),
		"-o", "IdentitiesOnly=yes", "--", "dev@dev.example.test",
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("SSH args = %q, want %q", args, want)
	}

	m := &Model{}
	m.remote.configsLoaded = true
	m.remote.sshConfigs = configs
	m.StartRemoteSSHSession(nil)
	if !m.remote.sshSessionOpen {
		t.Fatal("SSH Session did not open the configuration chooser")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.remoteSSHSessionDialog(c, c.Theme())
	}, 640, 420)
	tester.Frame()
	for _, text := range []string{
		"Start SSH Session", "Development box", "Edit SSH Configurations…", "Cancel",
	} {
		if !tester.HasText(text) {
			t.Errorf("SSH Session chooser lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestBackendRemoteSSHTerminalUsesThePinnedCommandAndSavedCredentials(t *testing.T) {
	root := "/srv/app"
	var gotPayload map[string]any
	finished := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "ssh_open_terminal" {
				t.Fatalf("command = %q, want ssh_open_terminal", command)
			}
			gotPayload, _ = input.(map[string]any)
			return json.RawMessage(`{"id":42,"title":"Development box","pid":null}`), nil
		},
	}
	m.setRoot(root)
	m.term.visible = true
	m.startBackendRemoteSSHSession(openidea.SSHConfiguration{
		ID: "dev-box", Name: "Development box", Host: "dev.example.test",
	}, "Development box")
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("SSH terminal did not finish starting")
	}
	if tab := m.term.current(); tab == nil || tab.starting || tab.id != 42 || tab.shell != "Development box" {
		t.Fatalf("SSH terminal tab = %+v", tab)
	}
	if gotPayload["id"] != "dev-box" || gotPayload["cols"] != 100 || gotPayload["rows"] != 30 {
		t.Fatalf("ssh_open_terminal payload = %#v", gotPayload)
	}
	output, ok := gotPayload["onOutput"].(map[string]any)
	if !ok || output["$channel"] == nil {
		t.Fatalf("ssh_open_terminal output channel = %#v", gotPayload["onOutput"])
	}
	for _, secret := range []string{"secret", "password", "passphrase", "keyFile"} {
		if _, leaked := gotPayload[secret]; leaked {
			t.Fatalf("ssh_open_terminal payload leaked %q: %#v", secret, gotPayload)
		}
	}
}

func TestRemoteEnvironmentReloadDropsALateReplyAfterProjectSwitch(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	finished := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			started <- struct{}{}
			<-release
			return json.RawMessage(`{"changed":["PATH"],"shellRead":true}`), nil
		},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
	}
	m.setRoot("/srv/old")
	m.remote.session = &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
		Root:   "/srv/old",
	}
	m.ReloadRemoteEnvironment()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("env_reload did not start")
	}

	m.resetRemoteProjectState()
	m.resetProjectState("/srv/new")
	m.setRoot("/srv/new")
	close(release)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("late env_reload reply did not return")
	}

	if m.remote.envBusy || m.remote.envLoaded || m.remote.envErr != "" ||
		len(m.remote.envChanged) != 0 || m.remote.envShellRead {
		t.Fatalf("late env_reload changed the new project state: %+v", m.remote)
	}
}

func TestRemoteConnectionCompletionCannotReplaceANewerProject(t *testing.T) {
	m := &Model{}
	m.setRoot("/local/new")
	m.remote.generation = 8
	m.remote.busy = true
	backend := &remoteBackend{
		target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "old-host"},
	}
	port := freeTCPPort(t)
	forwarded, err := backend.startForward(port, nil)
	if err != nil {
		t.Fatal(err)
	}
	session := openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "old-host"},
		Root:   "/srv/old",
		Host:   "old.example.test",
	}
	m.finishRemoteConnection(
		7,
		backend,
		session,
		"/local/new",
		openidea.ProjectInfo{Root: "/srv/old"},
		nil,
	)
	if m.Root() != "/local/new" || m.remote.session != nil || m.remote.backend != nil {
		t.Fatalf("stale connection replaced the current project: root=%q session=%+v backend=%v",
			m.Root(), m.remote.session, m.remote.backend)
	}
	if forwards := backend.listForwards(); len(forwards) != 0 {
		t.Fatalf("stale connection kept forwarded ports: %+v", forwards)
	}
	listener, err := net.Listen(
		"tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(int(forwarded.LocalPort))),
	)
	if err != nil {
		t.Fatalf("stale connection left local port %d open: %v", forwarded.LocalPort, err)
	}
	_ = listener.Close()
}

func TestRemoteConnectionCancelStopsTheInFlightAttempt(t *testing.T) {
	finished := make(chan struct{}, 1)
	m := &Model{
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
	}
	m.setRoot("/local/app")
	ctx, cancel := context.WithCancel(context.Background())
	run := &remoteConnectionRun{
		cancel:       cancel,
		done:         make(chan struct{}),
		previousRoot: "/local/app",
	}
	m.remote.connectOpen = true
	m.remote.busy = true
	m.remote.busyLabel = remoteStageLabel(openidea.RemoteStageConnecting)
	m.remote.progress = &openidea.RemoteProgress{Stage: openidea.RemoteStageConnecting}
	m.remote.connection = run
	generation := m.remote.generation

	tester := ui.NewTester(func(c *ui.Context) {
		m.remoteConnectDialog(c, c.Theme())
	}, 760, 520)
	tester.Frame()
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel did not cancel the connection context")
	}
	if m.remote.connectOpen {
		t.Fatal("Cancel left the connection dialog open")
	}
	if m.remote.generation == generation {
		t.Fatal("Cancel did not invalidate late connection callbacks")
	}
	close(run.done)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled connection did not finish")
	}
	if m.remote.busy || m.remote.connection != nil || m.remote.progress != nil {
		t.Fatalf("cancelled connection stayed active: %+v", m.remote)
	}
	if m.Root() != "/local/app" || m.status != "Remote connection cancelled." {
		t.Fatalf("cancelled connection changed the local project: root=%q status=%q",
			m.Root(), m.status)
	}
}

func TestRemoteBackendFailureIsVisibleInTheRemotePanel(t *testing.T) {
	m := &Model{}
	m.setRoot("/srv/app")
	m.showingWelcome = false
	m.remote.session = &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
		Root:   "/srv/app",
	}
	m.remoteBackendUnavailable(
		"Remote connection was cancelled, but the local backend could not start",
		errors.New("local server unavailable"),
	)

	tester := ui.NewTester(func(c *ui.Context) {
		m.remoteDevelopmentPanel(c, c.Theme())
	}, 960, 420)
	tester.Frame()
	for _, want := range []string{
		"Remote Development",
		"Remote connection was cancelled, but the local backend could not start",
		"local server unavailable",
	} {
		if !tester.HasText(want) {
			t.Errorf("remote failure panel lacks %q: %q", want, tester.Texts())
		}
	}
}

func TestRemoteDisconnectReportsFailureToRestoreTheLocalBackend(t *testing.T) {
	failingLauncher := func(context.Context, openidea.LaunchConfig) (openidea.Transport, error) {
		return nil, errors.New("local server unavailable")
	}
	manager := openidea.NewManager(openidea.LaunchConfig{
		Launcher: failingLauncher, StartupTimeout: time.Second,
	})
	t.Cleanup(func() { _ = manager.Close() })

	finished := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		mgr: manager,
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
	}
	m.setRoot("/srv/app")
	m.remote.backend = &remoteBackend{}
	m.remote.session = &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
		Root:   "/srv/app",
		Host:   "dev@example.test:22",
	}
	m.disconnectRemoteNow()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect failure did not reach the UI")
	}
	if m.Root() != "" || !m.showingWelcome || m.remote.session != nil ||
		m.remote.backend != nil || m.remote.busy {
		t.Fatalf("disconnect failure left inconsistent state: root=%q welcome=%v session=%v backend=%v busy=%v",
			m.Root(), m.showingWelcome, m.remote.session, m.remote.backend, m.remote.busy)
	}
	for _, want := range []string{"local backend could not start", "local server unavailable"} {
		if !strings.Contains(m.remote.connectErr, want) {
			t.Fatalf("disconnect failure = %q, want %q", m.remote.connectErr, want)
		}
	}
}
