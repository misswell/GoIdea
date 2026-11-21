package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestRemoteActionsAppearInCommandPalette(t *testing.T) {
	want := []string{
		"remote.connect",
		"remote.disconnect",
		"remote.reloadEnvironment",
		"remote.reopenInContainer",
		"remote.showToolWindow",
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
	for _, id := range []string{"remote.connect", "remote.disconnect", "remote.notice"} {
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
		"No ports are forwarded.",
		"Port forwarding needs host RPCs",
	} {
		if !tester.HasText(want) {
			t.Errorf("Remote Development window lacks %q: %q", want, tester.Texts())
		}
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

func TestRemoteConnectAndReopenDialogsExplainBackendGap(t *testing.T) {
	m := &Model{}
	if !m.RunAction("remote.connect") {
		t.Fatal("Remote Development action did not run")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.remoteConnectDialog(c, c.Theme())
	}, 760, 520)
	tester.Frame()
	for _, want := range []string{
		"Remote Development", "SSH", "WSL", "Dev Container",
		"backend bundled with this build", "does not expose", "Connect", "Close",
	} {
		if !tester.HasText(want) {
			t.Errorf("connect dialog lacks %q: %q", want, tester.Texts())
		}
	}

	m.remote.connectOpen = false
	m.setRoot(t.TempDir())
	if !m.RunAction("remote.reopenInContainer") {
		t.Fatal("Reopen in Container action did not run")
	}
	tester = ui.NewTester(func(c *ui.Context) {
		m.remoteNoticeDialog(c, c.Theme())
	}, 760, 420)
	tester.Frame()
	for _, want := range []string{
		"Reopen in Container Unavailable", "backend bundled with this build",
		"did not restart, close, or reopen the project",
	} {
		if !tester.HasText(want) {
			t.Errorf("reopen notice lacks %q: %q", want, tester.Texts())
		}
	}
}

func TestRemoteDisconnectConfirmsWithoutClosingProject(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
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
		"Disconnect from Remote Host?", "Not connected", "no remote teardown RPC",
		"leave this project open", "Disconnect", "Cancel",
	} {
		if !tester.HasText(want) {
			t.Errorf("disconnect confirmation lacks %q: %q", want, tester.Texts())
		}
	}

	m.confirmRemoteDisconnect()
	if m.Root() != root {
		t.Fatalf("disconnect changed project root from %q to %q", root, m.Root())
	}
	if m.remote.disconnectOpen || !m.remote.noticeOpen {
		t.Fatalf("disconnect state = open %v, notice %v", m.remote.disconnectOpen, m.remote.noticeOpen)
	}
	if !strings.Contains(m.remote.noticeBody, "project remains open") ||
		!strings.Contains(m.remote.noticeBody, "No remote connection was changed") {
		t.Fatalf("disconnect notice = %q", m.remote.noticeBody)
	}
}

func TestRemoteMenusExposeWindowAndActions(t *testing.T) {
	var file, view menuEntry
	for _, group := range mainMenuGroups() {
		switch group.label {
		case "File":
			file = group
		case "View":
			view = group
		}
	}
	if file.label == "" || view.label == "" {
		t.Fatal("main menu is missing File or View")
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
}
