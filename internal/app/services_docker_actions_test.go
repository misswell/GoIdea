package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestDockerActionRoutesAndRefreshes(t *testing.T) {
	posts := make(chan func(), 10)
	calls := make(chan string, 10)
	release := make(chan struct{})
	var args map[string]any
	m := &Model{postHook: func(f func()) { posts <- f }, invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command == "docker_prune" {
			args = input.(map[string]any)
			calls <- command
			<-release
			return nil, errors.New("prune failed")
		}
		calls <- command
		return json.RawMessage(`{}`), nil
	}}
	connection := openidea.DockerConnection{ID: "one", Endpoint: openidea.DockerEndpoint{Kind: "default"}}
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.runDockerPrune(connection, "images")
	if got := <-calls; got != "docker_prune" {
		t.Fatal(got)
	}
	channel := m.services.docker.operation.channel
	m.acceptChunk(channel, json.RawMessage(`"deleted"`))
	if got := m.services.docker.operation.output; len(got) != 1 || got[0] != "deleted" {
		t.Fatalf("output = %v", got)
	}
	if args["target"] != "images" || args["request"] == uint32(0) {
		t.Fatalf("args = %#v", args)
	}
	close(release)
	select {
	case f := <-posts:
		f()
	case <-time.After(time.Second):
		t.Fatal("operation did not finish")
	}
	if got := m.services.docker.operation.err; got != "prune failed" {
		t.Fatal(got)
	}
	select {
	case got := <-calls:
		if got != "docker_snapshot" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot not refreshed")
	}
}

func TestDockerChunkRoutingAndBounds(t *testing.T) {
	m := &Model{}
	m.services.docker.operation = &dockerOperation{channel: 3}
	stream := &dockerLogStream{channel: 4}
	events := &dockerEventStream{channel: 5}
	m.services.docker.logs = map[string]*dockerLogStream{"one": stream}
	m.services.docker.events = map[string]*dockerEventStream{"conn": events}
	m.acceptChunk(4, json.RawMessage(`{"stderr":false,"text":"hello"}`))
	m.acceptChunk(3, json.RawMessage(`"operation"`))
	m.acceptChunk(5, json.RawMessage(`{"object":"container","action":"start","id":"abc"}`))
	m.acceptChunk(6, json.RawMessage(`"ignored"`))
	if len(stream.lines) != 1 || stream.lines[0].Text != "hello" ||
		len(m.services.docker.operation.output) != 1 || len(events.events) != 1 {
		t.Fatal("chunks routed incorrectly")
	}
	lines := dockerAppendBounded(nil, strings.Repeat("x\n", dockerMaxLogLines+5))
	if len(lines) != dockerMaxLogLines {
		t.Fatalf("line count = %d", len(lines))
	}
	if got := dockerAppendBounded(nil, strings.Repeat("x", 20000)); len(got[0]) != 16384 {
		t.Fatal("line not bounded")
	}
	longLine := strings.Repeat("x", 16384)
	bounded := dockerAppendBounded(nil, strings.Repeat(longLine+"\n", 200))
	total := 0
	for _, line := range bounded {
		total += len(line) + 1
	}
	if total > dockerMaxBytes || len(bounded) >= 200 {
		t.Fatalf("byte bound = %d bytes across %d lines", total, len(bounded))
	}
	for i := 0; i < 120; i++ {
		m.acceptDockerChunk(5, json.RawMessage(`{"object":"container","action":"event","id":"abc"}`))
	}
	if len(events.events) != 100 {
		t.Fatalf("event queue = %d entries", len(events.events))
	}
	stream.channel = 0
	if m.acceptDockerChunk(4, json.RawMessage(`{"text":"late"}`)) {
		t.Fatal("a completed Docker log channel still accepted output")
	}
}

func TestDockerOperationOnlyOffersCancellationForStreamingCommands(t *testing.T) {
	render := func(streaming bool) []string {
		m := &Model{}
		m.services.docker.operation = &dockerOperation{
			title: "Docker operation", request: 7, streaming: streaming,
		}
		tester := ui.NewTester(func(c *ui.Context) {
			m.dockerServiceProviderBanner(c, c.Theme())
		}, 700, 100)
		tester.Frame()
		return tester.Texts()
	}
	if texts := render(false); slices.Contains(texts, "Cancel") {
		t.Fatalf("non-streaming operation offered Cancel: %v", texts)
	}
	if texts := render(true); !slices.Contains(texts, "Cancel") {
		t.Fatalf("streaming operation lacks Cancel: %v", texts)
	}
}

func TestDockerComposeArguments(t *testing.T) {
	calls := make(chan map[string]any, 1)
	finished := make(chan struct{}, 2)
	m := &Model{postHook: func(fn func()) {
		fn()
		finished <- struct{}{}
	}, invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command == "docker_compose_op" {
			calls <- input.(map[string]any)
		}
		return nil, nil
	}}
	m.services.docker.settings.Connections = []openidea.DockerConnection{{ID: "conn"}}
	project, dir := "project", "/project"
	snapshot := openidea.DockerSnapshot{Containers: []openidea.DockerContainer{{ComposeProject: &project, ComposeWorkingDir: &dir, ComposeFiles: []string{"compose.yml"}}}}
	for _, op := range []string{"start", "stop", "restart", "down"} {
		m.runDockerComposeOp(serviceTreeNode{dockerConn: "conn", dockerID: project}, snapshot, op)
		select {
		case args := <-calls:
			if args["project"] != project || args["op"] != op ||
				*args["workingDir"].(*string) != dir || args["files"].([]string)[0] != "compose.yml" {
				t.Fatalf("%s args = %#v", op, args)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s compose operation was not invoked", op)
		}
		// The operation callback starts a snapshot refresh on another
		// goroutine. Wait for both posts before mutating the model again.
		<-finished
		<-finished
	}
}

func TestDockerContainerOperationUsesProgressTitle(t *testing.T) {
	posts := make(chan func(), 1)
	m := &Model{
		postHook: func(fn func()) { posts <- fn },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "docker_snapshot" {
				return nil, nil
			}
			if command != "docker_container_op" {
				t.Fatalf("command = %s", command)
			}
			args := input.(map[string]any)
			if args["op"] != "start" {
				t.Fatalf("args = %#v", args)
			}
			return nil, nil
		},
	}
	connection := openidea.DockerConnection{
		ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.runDockerContainerOpIDs(connection, []string{"container-id"}, "start")
	select {
	case fn := <-posts:
		fn()
	case <-time.After(time.Second):
		t.Fatal("container operation did not finish")
	}
	if got := m.services.docker.operation.title; got != "Starting container…" {
		t.Fatalf("title = %q", got)
	}
}

func TestDockerRegistryLoginErrorRedactsPassword(t *testing.T) {
	posts := make(chan func(), 1)
	password := "registry-secret"
	m := &Model{
		postHook: func(fn func()) { posts <- fn },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "docker_registry_login" {
				t.Fatalf("command = %s", command)
			}
			if got := input.(map[string]any)["password"]; got != password {
				t.Fatalf("password argument = %#v", got)
			}
			return nil, errors.New("login rejected registry-secret")
		},
	}
	m.services.docker.settings.Connections = []openidea.DockerConnection{{
		ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}}
	m.testDockerRegistryLoginWithPassword(
		openidea.DockerRegistry{ID: "registry", Name: "Registry", Address: "registry.example"},
		&password,
	)
	select {
	case fn := <-posts:
		fn()
	case <-time.After(time.Second):
		t.Fatal("registry login did not finish")
	}
	login := m.services.docker.logins["registry"]
	if login == nil {
		t.Fatal("login state missing")
	}
	if strings.Contains(login.err, password) || !strings.Contains(login.err, "[redacted]") {
		t.Fatalf("unsafe login error = %q", login.err)
	}
}

func TestDockerCloseCancelsOperationAndStreams(t *testing.T) {
	calls := make(chan string, 3)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			calls <- command
			return nil, nil
		},
	}
	cancelled := 0
	m.services.docker.watching = true
	m.services.docker.operation = &dockerOperation{
		request: 11, connectionID: "conn", cancel: func() { cancelled++ },
	}
	m.services.docker.events = map[string]*dockerEventStream{
		"conn": {request: 12, cancel: func() { cancelled++ }},
	}
	m.services.docker.logs = map[string]*dockerLogStream{
		"log": {request: 13, loading: true, cancel: func() { cancelled++ }},
	}
	m.closeDockerServices()
	if !m.services.docker.operation.stopped || len(m.services.docker.events) != 0 || len(m.services.docker.logs) != 0 {
		t.Fatalf("close left Docker state: %+v", m.services.docker)
	}
	if cancelled != 3 {
		t.Fatalf("cancel funcs called %d times", cancelled)
	}
	for i := 0; i < 3; i++ {
		select {
		case got := <-calls:
			if got != "cancel_request" {
				t.Fatalf("command = %s", got)
			}
		case <-time.After(time.Second):
			t.Fatal("cancel_request was not sent")
		}
	}
}

func TestDockerRegistryEndpointSelectionAndDraftPassword(t *testing.T) {
	m := &Model{}
	m.services.docker.settings.Connections = []openidea.DockerConnection{
		{ID: "one", Name: "One", Endpoint: openidea.DockerEndpoint{Kind: "socket", Path: "/one.sock"}},
		{ID: "two", Name: "Two", Endpoint: openidea.DockerEndpoint{Kind: "context", Name: "two"}},
	}
	if connection, _ := m.dockerRegistryEndpoint("registry"); connection.ID != "one" {
		t.Fatalf("initial endpoint = %+v", connection)
	}
	m.services.docker.logins = map[string]*dockerRegistryLoginState{
		"registry": {connectionID: "", connectionSelected: true},
	}
	if connection, _ := m.dockerRegistryEndpoint("registry"); connection.ID != "default" ||
		connection.Name != "Docker CLI default" {
		t.Fatalf("explicit default endpoint = %+v", connection)
	}

	calls := make(chan map[string]any, 1)
	posts := make(chan func(), 1)
	m.postHook = func(fn func()) { posts <- fn }
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		if command != "docker_registry_login" {
			t.Fatalf("command = %s", command)
		}
		calls <- input.(map[string]any)
		return json.RawMessage(`"Login Succeeded"`), nil
	}
	password := "draft-secret"
	m.testDockerRegistryLoginWithPassword(openidea.DockerRegistry{ID: "registry", Name: "Registry"}, &password)
	args := <-calls
	if args["password"] != password || args["endpoint"].(openidea.DockerEndpoint).Kind != "default" {
		t.Fatalf("login args = %#v", args)
	}
	(<-posts)()
}

func TestDockerDetailPullAndForceRemoveRoutes(t *testing.T) {
	type call struct {
		command string
		args    map[string]any
	}
	calls := make(chan call, 20)
	posts := make(chan func(), 20)
	m := &Model{
		postHook: func(fn func()) { posts <- fn },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- call{command: command, args: args}
			switch command {
			case "docker_inspect":
				return json.RawMessage(`"inspect json"`), nil
			case "docker_image_history":
				return json.RawMessage(`[]`), nil
			default:
				return json.RawMessage(`{}`), nil
			}
		},
	}
	connection := openidea.DockerConnection{
		ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	running := openidea.DockerContainer{ID: "container", State: "running"}
	m.services.docker.snapshots = map[string]openidea.DockerSnapshot{
		"conn": {Containers: []openidea.DockerContainer{running}},
	}
	runDetail := func(load func()) call {
		load()
		got := <-calls
		(<-posts)()
		return got
	}
	wantCommands := map[string]bool{
		"docker_inspect": true, "docker_container_stats": true,
		"docker_container_top": true, "docker_image_history": true,
	}
	for _, load := range []func(){
		func() { m.loadDockerInspect(connection, "container", "container") },
		func() { m.loadDockerStats(connection, "container") },
		func() { m.loadDockerProcesses(connection, "container") },
		func() { m.loadDockerHistory(connection, "image") },
	} {
		got := runDetail(load)
		if !wantCommands[got.command] {
			t.Fatalf("unexpected command = %s", got.command)
		}
		delete(wantCommands, got.command)
		if got.args["endpoint"].(openidea.DockerEndpoint) != connection.Endpoint {
			t.Fatalf("%s endpoint = %#v", got.command, got.args["endpoint"])
		}
	}

	m.services.docker.dialog = &dockerDialog{
		kind: "pull", connectionID: "conn", value: "postgres:16",
	}
	m.submitDockerDialog()
	pull := <-calls
	if pull.command != "docker_pull" || pull.args["reference"] != "postgres:16" ||
		pull.args["request"] == uint32(0) {
		t.Fatalf("pull call = %#v", pull)
	}
	(<-posts)()
	if snapshot := <-calls; snapshot.command != "docker_snapshot" {
		t.Fatalf("post-pull command = %s", snapshot.command)
	}
	(<-posts)()
	m.services.docker.snapshots["conn"] = openidea.DockerSnapshot{
		Containers: []openidea.DockerContainer{running},
	}

	m.services.docker.dialog = &dockerDialog{
		kind: "remove", connectionID: "conn", object: "container",
		ids: []string{"container"}, force: false,
	}
	m.submitDockerDialog()
	remove := <-calls
	if remove.command != "docker_remove" || remove.args["force"] != true ||
		remove.args["object"] != "container" {
		t.Fatalf("force remove call = %#v", remove)
	}
}

func TestDockerOperationBusyDisablesMutations(t *testing.T) {
	m := dockerTestModel(t, true, true)
	m.services.docker.operation = &dockerOperation{request: 7, connectionID: "conn"}
	actions := m.dockerServiceProviderActions(serviceTreeNode{
		dockerConn: "conn", dockerID: "api-id", nodeKind: "docker-container",
	})
	byID := make(map[string]serviceItemAction, len(actions))
	for _, action := range actions {
		byID[action.id] = action
	}
	for _, id := range []string{"start", "stop", "restart", "delete", "pull"} {
		if action := byID[id]; !action.disabled {
			t.Errorf("%s is enabled while another Docker operation runs", id)
		}
	}
	if byID["terminal"].disabled {
		t.Fatal("Create Terminal should remain available during a Docker operation")
	}
}

func TestDockerProjectResetRejectsStaleSnapshotCallback(t *testing.T) {
	posts := make(chan func(), 1)
	started := make(chan struct{})
	release := make(chan struct{})
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { posts <- fn },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "docker_snapshot" {
				return nil, nil
			}
			close(started)
			<-release
			return json.RawMessage(`{"availability":{"kind":"ready"}}`), nil
		},
	}
	connection := openidea.DockerConnection{ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"}}
	m.services.docker.loaded = true
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.refreshDockerSnapshot(connection)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("snapshot did not start")
	}

	m.resetServices()
	close(release)
	select {
	case callback := <-posts:
		callback()
	case <-time.After(time.Second):
		t.Fatal("stale snapshot callback did not finish")
	}
	if len(m.services.docker.snapshots) != 0 || m.services.docker.loaded {
		t.Fatalf("stale snapshot changed the new project: %+v", m.services.docker)
	}
}

func TestDockerBackendLossCancelsStreamsAndRestores(t *testing.T) {
	var cancelled atomic.Int32
	commands := make(chan string, 32)
	settingsLoaded := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			select {
			case settingsLoaded <- struct{}{}:
			default:
			}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			commands <- command
			switch command {
			case "docker_settings":
				return json.RawMessage(`{"connections":[],"registries":[]}`), nil
			default:
				return nil, nil
			}
		},
	}
	m.services.docker.watching = true
	m.services.docker.loaded = true
	m.services.docker.operation = &dockerOperation{
		request: 11, connectionID: "conn", cancel: func() { cancelled.Add(1) },
	}
	m.services.docker.events = map[string]*dockerEventStream{
		"conn": {request: 12, cancel: func() { cancelled.Add(1) }},
	}
	m.services.docker.logs = map[string]*dockerLogStream{
		"log": {request: 13, loading: true, cancel: func() { cancelled.Add(1) }},
	}
	m.services.docker.settingsUI = dockerSettingsUI{
		formOpen: true, editing: "registry", saving: true, sshLoading: true,
		name: "Registry", address: "ghcr.io",
	}

	m.dockerBackendLost(errors.New("connection closed"))
	if !m.services.docker.backendLost || m.services.docker.loaded ||
		len(m.services.docker.events) != 0 || len(m.services.docker.logs) != 0 {
		t.Fatalf("backend loss state = %+v", m.services.docker)
	}
	if u := m.services.docker.settingsUI; u.saving || u.sshLoading ||
		u.editing != "registry" || !u.formOpen || !strings.Contains(u.err, "connection closed") {
		t.Fatalf("backend loss left a stuck settings form: %+v", u)
	}
	if cancelled.Load() != 3 {
		t.Fatalf("cancel funcs called %d times, want 3", cancelled.Load())
	}

	m.dockerBackendRestored()
	if m.services.docker.backendLost {
		t.Fatal("backend restoration kept the lost flag")
	}
	deadline := time.After(time.Second)
	for {
		select {
		case command := <-commands:
			if command == "docker_settings" {
				goto settingsReloaded
			}
			if command != "cancel_request" {
				t.Fatalf("restore command = %s", command)
			}
		case <-deadline:
			t.Fatal("Docker settings were not reloaded after the backend restarted")
		}
	}
settingsReloaded:
	select {
	case <-settingsLoaded:
	case <-time.After(time.Second):
		t.Fatal("Docker settings callback did not finish after restoration")
	}
	if !m.services.docker.loaded {
		t.Fatal("Docker settings did not finish loading after restoration")
	}
}

func TestDockerLogAndEventStreamsFinishWithoutLeaking(t *testing.T) {
	t.Run("log success", func(t *testing.T) {
		finished := make(chan struct{}, 1)
		m := &Model{
			ctx:      context.Background(),
			postHook: func(fn func()) { fn(); finished <- struct{}{} },
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command == "docker_cancel" {
					t.Fatalf("unexpected command %s", command)
				}
				return nil, nil
			},
		}
		m.services.docker.settings.Connections = []openidea.DockerConnection{{
			ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"},
		}}
		m.startDockerLogsWithOptions(m.services.docker.settings.Connections[0],
			serviceTreeNode{key: "log", dockerConn: "conn", dockerID: "container"},
			false, false)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("log stream did not finish")
		}
		stream := m.services.docker.logs["log"]
		if stream == nil || stream.loading || stream.request != 0 || stream.err != "" {
			t.Fatalf("finished log stream = %+v", stream)
		}
	})

	t.Run("log error", func(t *testing.T) {
		finished := make(chan struct{}, 1)
		m := &Model{
			ctx:      context.Background(),
			postHook: func(fn func()) { fn(); finished <- struct{}{} },
			invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
				return nil, errors.New("log stream ended")
			},
		}
		m.services.docker.settings.Connections = []openidea.DockerConnection{{
			ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"},
		}}
		m.startDockerLogsWithOptions(m.services.docker.settings.Connections[0],
			serviceTreeNode{key: "log", dockerConn: "conn", dockerID: "container"},
			false, false)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("failed log stream did not finish")
		}
		stream := m.services.docker.logs["log"]
		if stream == nil || stream.loading || stream.err != "log stream ended" {
			t.Fatalf("failed log stream = %+v", stream)
		}
	})

	t.Run("events daemon stop", func(t *testing.T) {
		finished := make(chan struct{}, 1)
		m := &Model{
			ctx:      context.Background(),
			postHook: func(fn func()) { fn(); finished <- struct{}{} },
			invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
				return nil, errors.New("daemon stopped")
			},
		}
		m.services.docker.watching = true
		m.services.docker.settings.Connections = []openidea.DockerConnection{{
			ID: "conn", Name: "Docker", Endpoint: openidea.DockerEndpoint{Kind: "default"},
		}}
		m.startDockerEvents(m.services.docker.settings.Connections[0])
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("event stream did not finish")
		}
		if stream := m.services.docker.events["conn"]; stream != nil {
			t.Fatalf("stopped event stream remains: %+v", stream)
		}
		if got := m.services.providerErrors[dockerProviderID]; !strings.Contains(got, "daemon stopped") {
			t.Fatalf("provider error = %q", got)
		}
	})
}
