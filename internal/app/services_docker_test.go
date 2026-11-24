package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func dockerTestSnapshot() openidea.DockerSnapshot {
	shop, api, db := "shop", "api", "db"
	return openidea.DockerSnapshot{
		Availability: openidea.DockerAvailability{Kind: "ready"},
		Containers: []openidea.DockerContainer{
			{ID: "web-id", Name: "web", Image: "nginx:1.27", State: "exited", Status: "Exited (0)"},
			{ID: "api-id", Name: "shop-api-1", Image: "app:latest", State: "running", Status: "Up", ComposeProject: &shop, ComposeService: &api},
			{ID: "db-id", Name: "shop-db-1", Image: "postgres:16", State: "exited", Status: "Exited (0)", ComposeProject: &shop, ComposeService: &db},
			{ID: "dev-id", Name: "dev", Image: "dev:latest", State: "running", Status: "Up", Labels: map[string]string{"devcontainer.local_folder": "/project"}},
		},
		Images: []openidea.DockerImage{
			{ID: "sha256:1111111111112222", Repository: "postgres", Tag: "16", Size: "431MB", CreatedSince: "7 weeks ago"},
			{ID: "sha256:1111111111112222", Repository: "postgres", Tag: "latest", Size: "431MB", CreatedSince: "7 weeks ago"},
			{ID: "sha256:aaaaaaaaaaaa0000", Repository: "<none>", Tag: "<none>", Size: "120MB", CreatedSince: "4 weeks ago"},
		},
	}
}

func dockerTestModel(t *testing.T, stopped, untagged bool) *Model {
	t.Helper()
	m := &Model{cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	m.prefs.Services.Docker = DockerFilterPrefs{
		ShowStoppedContainers: boolPointer(stopped),
		ShowUntaggedImages:    boolPointer(untagged),
	}
	connection := openidea.DockerConnection{
		ID: "conn", Name: "Docker", Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}
	m.services.docker.loaded = true
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.services.docker.snapshots = map[string]openidea.DockerSnapshot{"conn": dockerTestSnapshot()}
	return m
}

func dockerTreeChild(parent serviceTreeNode, kind, id string) *serviceTreeNode {
	for i := range parent.children {
		child := &parent.children[i]
		if child.nodeKind == kind && (id == "" || child.dockerID == id) {
			return child
		}
	}
	return nil
}

func TestDockerTreeFiltersComposeCountsAndImageKeys(t *testing.T) {
	m := dockerTestModel(t, false, false)
	nodes := m.dockerServiceNodes()
	if len(nodes) != 1 || nodes[0].nodeKind != "docker" {
		t.Fatalf("roots = %+v", nodes)
	}
	root := nodes[0]
	compose := dockerTreeChild(root, "docker-compose", "shop")
	if compose == nil || compose.detail != "1/2 running" || len(compose.children) != 1 ||
		compose.children[0].dockerID != "api-id" {
		t.Fatalf("compose = %+v", compose)
	}
	containers := dockerTreeChild(root, "docker-group", "containers")
	if containers == nil {
		t.Fatal("Containers group not found")
	}
	if child := dockerTreeChild(*containers, "docker-container", "web-id"); child != nil {
		t.Fatalf("stopped standalone container is visible: %+v", child)
	}
	dev := dockerTreeChild(root, "docker-group", "devContainers")
	if dev == nil || dev.detail != "1" || len(dev.children) != 1 || dev.children[0].dockerID != "dev-id" {
		t.Fatalf("dev containers = %+v", dev)
	}
	images := dockerTreeChild(root, "docker-group", "images")
	if images == nil || images.detail != "2" || len(images.children) != 2 {
		t.Fatalf("images = %+v", images)
	}
	if images.children[0].key == images.children[1].key {
		t.Fatalf("image tags share a tree key: %+v", images.children)
	}

	latest := dockerTreeChild(*images, "docker-image", "sha256:1111111111112222")
	for i := range images.children {
		if images.children[i].dockerRef == "postgres:latest" {
			latest = &images.children[i]
			break
		}
	}
	if latest == nil {
		t.Fatal("postgres:latest node not found")
	}
	image := dockerImageForNode(dockerTestSnapshot(), *latest)
	if image == nil || image.Tag != "latest" {
		t.Fatalf("resolved image = %+v", image)
	}

	m.setDockerFilter(boolPointer(true), boolPointer(true))
	if !m.dockerFilters().showStoppedContainers() || !m.dockerFilters().showUntaggedImages() {
		t.Fatal("Docker filters were not updated")
	}
	stored := LoadPreferences(m.cfg.SettingsPath)
	if !stored.Services.Docker.showStoppedContainers() || !stored.Services.Docker.showUntaggedImages() {
		t.Fatalf("Docker filters were not persisted: %+v", stored.Services.Docker)
	}
	nodes = m.dockerServiceNodes()
	containers = dockerTreeChild(nodes[0], "docker-group", "containers")
	if containers == nil || dockerTreeChild(*containers, "docker-container", "web-id") == nil {
		t.Fatal("stopped container did not return after enabling it")
	}
	images = dockerTreeChild(nodes[0], "docker-group", "images")
	if images == nil || images.detail != "3" || len(images.children) != 3 {
		t.Fatalf("unfiltered images = %+v", images)
	}
}

func TestDockerProviderActionsUseAllComposeContainers(t *testing.T) {
	m := dockerTestModel(t, false, true)
	compose := serviceTreeNode{dockerConn: "conn", dockerID: "shop", nodeKind: "docker-compose"}
	actions := m.dockerServiceProviderActions(compose)
	byID := make(map[string]serviceItemAction, len(actions))
	for _, action := range actions {
		byID[action.id] = action
	}
	if byID["start"].disabled {
		t.Fatal("Compose Start is disabled when hidden stopped services exist")
	}
	if byID["stop"].disabled {
		t.Fatal("Compose Stop is disabled while a service runs")
	}
	if byID["restart"].disabled || byID["delete"].disabled {
		t.Fatal("Compose Restart or Down is disabled")
	}

	running := serviceTreeNode{dockerConn: "conn", dockerID: "api-id", nodeKind: "docker-container"}
	actions = m.dockerServiceProviderActions(running)
	byID = make(map[string]serviceItemAction, len(actions))
	for _, action := range actions {
		byID[action.id] = action
	}
	if !byID["start"].disabled || byID["stop"].disabled || byID["restart"].disabled ||
		byID["terminal"].disabled || byID["delete"].disabled {
		t.Fatalf("running container actions = %+v", byID)
	}
}

func TestDockerProviderActivationMatchesOpenIDEA(t *testing.T) {
	m := dockerTestModel(t, true, true)
	m.ctx = context.Background()
	m.postHook = func(f func()) { f() }
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		return nil, nil
	}
	container := serviceTreeNode{
		dockerConn: "conn", dockerID: "api-id", nodeKind: "docker-container", live: true,
	}
	m.dockerServiceProviderActivate(container)
	if m.services.docker.detailTab != dockerDetailLog {
		t.Fatalf("container activation tab = %q", m.services.docker.detailTab)
	}

	m.services.docker.settings.Registries = []openidea.DockerRegistry{{
		ID: "registry", Name: "Registry", Address: "example.com",
	}}
	m.dockerServiceProviderActivate(serviceTreeNode{
		dockerID: "registry", nodeKind: "docker-registry",
	})
	if !m.services.docker.settingsUI.formOpen || m.services.docker.settingsUI.open ||
		m.services.docker.settingsUI.editing != "registry" ||
		m.services.docker.settingsUI.id != "registry" {
		t.Fatalf("registry activation = %+v", m.services.docker.settingsUI)
	}
}

type dockerRPCCall struct {
	command string
	args    map[string]any
}

func TestDockerDetailTabsLoadOnlySelectedRPC(t *testing.T) {
	load := func(t *testing.T, state, tab string) []string {
		t.Helper()
		calls := make(chan dockerRPCCall, 8)
		m := &Model{
			ctx:      context.Background(),
			postHook: func(f func()) { f() },
			invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
				args, _ := input.(map[string]any)
				calls <- dockerRPCCall{command: command, args: args}
				switch command {
				case "docker_inspect":
					return json.RawMessage(`"{}"`), nil
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
		m.services.docker.snapshots = map[string]openidea.DockerSnapshot{
			"conn": {Containers: []openidea.DockerContainer{{
				ID: "container", Name: "container", State: state,
			}}},
		}
		node := serviceTreeNode{
			dockerConn: "conn", dockerID: "container",
			nodeKind: "docker-container", live: state == "running",
		}
		m.setDockerDetailTab(connection, node, tab)
		expected := map[string]int{
			dockerDetailDashboard: 2,
			dockerDetailLog:       1,
			dockerDetailProcesses: 1,
			dockerDetailInspect:   1,
		}[tab]
		if state != "running" && (tab == dockerDetailProcesses || tab == dockerDetailDashboard) {
			expected = 1
			if tab == dockerDetailProcesses {
				expected = 0
			}
		}
		commands := make([]string, 0, expected)
		for len(commands) < expected {
			select {
			case call := <-calls:
				commands = append(commands, call.command)
			case <-time.After(time.Second):
				t.Fatalf("tab %s produced %v, want %d RPCs", tab, commands, expected)
			}
		}
		select {
		case extra := <-calls:
			t.Fatalf("tab %s issued extra RPC %s", tab, extra.command)
		case <-time.After(30 * time.Millisecond):
		}
		sort.Strings(commands)
		return commands
	}

	for _, tt := range []struct {
		tab  string
		want []string
	}{
		{dockerDetailDashboard, []string{"docker_container_stats", "docker_inspect"}},
		{dockerDetailLog, []string{"docker_logs"}},
		{dockerDetailProcesses, []string{"docker_container_top"}},
		{dockerDetailInspect, []string{"docker_inspect"}},
	} {
		if got := load(t, "running", tt.tab); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s RPCs = %v, want %v", tt.tab, got, tt.want)
		}
	}
	if got := load(t, "exited", dockerDetailProcesses); len(got) != 0 {
		t.Fatalf("stopped Processes RPCs = %v", got)
	}
	if got := load(t, "exited", dockerDetailDashboard); !reflect.DeepEqual(got, []string{"docker_inspect"}) {
		t.Fatalf("stopped Dashboard RPCs = %v", got)
	}
}

func TestDockerInspectDashboardParsers(t *testing.T) {
	raw := `{
		"Id":"container",
		"Created":"2026-10-01T15:00:00Z",
		"State":{"Status":"running","Running":true,"Pid":4321,"Health":{"Status":"healthy"}},
		"Config":{
			"Hostname":"db",
			"User":"postgres",
			"Env":["PGDATA=/var/lib/postgresql/data","POSTGRES_DB=shop"],
			"Cmd":["postgres"],
			"Entrypoint":["docker-entrypoint.sh"],
			"WorkingDir":"/var/lib/postgresql",
			"Labels":{"com.docker.compose.project":"shop"},
			"ExposedPorts":{"5432/tcp":{}}
		},
		"HostConfig":{"RestartPolicy":{"Name":"unless-stopped"}},
		"Mounts":[{"Type":"volume","Name":"shop_pgdata","Destination":"/var/lib/postgresql/data","Mode":"z","RW":true}],
		"NetworkSettings":{
			"Ports":{"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"5432"}]},
			"Networks":{"shop_default":{"IPAddress":"172.18.0.2","Gateway":"172.18.0.1"}}
		}
	}`
	inspect := dockerContainerInspectFromText(raw)
	if inspect == nil || inspect.State.Pid != 4321 || inspect.State.Health == nil ||
		inspect.State.Health.Status != "healthy" || inspect.HostConfig.RestartPolicy.Name != "unless-stopped" {
		t.Fatalf("container inspect = %+v", inspect)
	}
	env := dockerEnvRows(inspect.Config.Env)
	if !reflect.DeepEqual(env, [][2]string{{"PGDATA", "/var/lib/postgresql/data"}, {"POSTGRES_DB", "shop"}}) {
		t.Fatalf("environment rows = %#v", env)
	}
	ports := dockerInspectPortMappings(inspect.NetworkSettings.Ports, "80/tcp")
	if len(ports) != 1 || ports[0].url != "http://127.0.0.1:5432" ||
		ports[0].label != "127.0.0.1:5432 -> 5432/tcp" {
		t.Fatalf("port mappings = %+v", ports)
	}
	if got := dockerExposedPorts(inspect.Config.ExposedPorts); !reflect.DeepEqual(got, []string{"5432/tcp"}) {
		t.Fatalf("exposed ports = %v", got)
	}
	network := dockerNetworkInspectFromText(`{
		"Id":"network-id",
		"Name":"shop_default",
		"IPAM":{"Config":[{"Subnet":"172.18.0.0/16","Gateway":"172.18.0.1"}]},
		"Containers":{"container":{"Name":"shop-db-1","IPv4Address":"172.18.0.2/16"}}
	}`)
	if network == nil || dockerNetworkIPAMText(network) != "172.18.0.0/16 · gw 172.18.0.1" ||
		dockerNetworkContainerAddress(network, "container") != "172.18.0.2/16" {
		t.Fatalf("network inspect = %+v", network)
	}
}

func TestDockerLogEditorReceivesBoundedStream(t *testing.T) {
	m := &Model{}
	tab := m.newDockerLogTab("docker-log:conn/container", "container (Log)", "")
	m.tabs = []*Tab{tab}
	stream := &dockerLogStream{channel: 91, editor: tab}
	m.services.docker.logs = map[string]*dockerLogStream{"log": stream}

	if !m.acceptDockerChunk(91, json.RawMessage(`{"stderr":true,"text":"first\nalpha"}`)) {
		t.Fatal("first log line was not routed")
	}
	if got := tab.state.Buffer.Snapshot().String(); got != "first\nalpha" {
		t.Fatalf("editor after first line = %q", got)
	}
	if len(stream.lines) != 2 || !stream.lines[0].Stderr || !stream.lines[1].Stderr {
		t.Fatalf("stderr lines = %+v", stream.lines)
	}
	language := tab.state.Buffer.Language()
	if language == nil || language.Highlighter == nil {
		t.Fatal("Docker log editor has no stderr highlighter")
	}
	tokens, final := language.Highlighter(tab.state.Buffer.Snapshot(), func() {}).
		Tokens(0, "first", nil)
	if !final || len(tokens) != 1 || tokens[0].Kind != editor.TokenInvalid {
		t.Fatalf("stderr tokens = %+v, final=%v", tokens, final)
	}
	theme := &ui.Theme{TextMuted: ui.RGB(100, 100, 100), Danger: ui.RGB(200, 20, 20)}
	spans := dockerLogSpans(stream.lines, theme)
	if len(spans) != 2 || spans[0].Color != theme.Danger || spans[1].Color != theme.Danger {
		t.Fatalf("stderr spans = %+v", spans)
	}

	stream.lines = dockerAppendLogBounded(stream.lines, openidea.DockerLogLine{
		Text: strings.Repeat("line\n", dockerMaxLogLines+10),
	})
	m.syncDockerLogEditor(stream)
	text := tab.state.Buffer.Snapshot().String()
	if strings.Count(text, "\n")+1 != dockerMaxLogLines {
		t.Fatalf("editor contains %d lines", strings.Count(text, "\n")+1)
	}
	if strings.HasPrefix(text, "first\n") || !strings.HasSuffix(text, "line") {
		t.Fatalf("editor did not retain the bounded tail")
	}
}

func TestDockerPullAndRemoveDialogsCancelWithEscape(t *testing.T) {
	for _, dialog := range []*dockerDialog{
		{
			kind: "pull", connectionID: "conn",
			title: "Pull Docker Image", value: "postgres:16", button: "Pull",
		},
		{
			kind: "remove", connectionID: "conn", object: "container",
			ids: []string{"container-id"}, title: "Delete Container", button: "Delete",
		},
	} {
		m := &Model{}
		m.services.docker.dialog = dialog
		tester := ui.NewTester(func(c *ui.Context) {
			m.dockerDialogView(c, c.Theme())
		}, 720, 520)
		tester.Frame()
		if !m.overlayOpen() {
			t.Fatalf("%s dialog is not registered as a modal overlay", dialog.kind)
		}
		tester.Key(0, ui.KeyEscape)
		if m.services.docker.dialog != nil {
			t.Fatalf("%s dialog stayed open after Escape", dialog.kind)
		}
	}
}

func TestDockerDashboardFallbackLoadsInspect(t *testing.T) {
	calls := make(chan string, 2)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			calls <- command
			return json.RawMessage(`"{}"`), nil
		},
	}
	connection := openidea.DockerConnection{
		ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.services.docker.snapshots = map[string]openidea.DockerSnapshot{
		"conn": {Images: []openidea.DockerImage{{ID: "image-id", Repository: "app", Tag: "latest"}}},
	}
	m.ensureDockerDashboardData(connection, serviceTreeNode{
		dockerConn: "conn", dockerID: "image-id", nodeKind: "docker-image",
	})
	select {
	case command := <-calls:
		if command != "docker_inspect" {
			t.Fatalf("dashboard command = %s", command)
		}
	case <-time.After(time.Second):
		t.Fatal("image dashboard did not load inspect data")
	}
}

func TestDockerDetailLogStopsWhenHiddenAndTracksContainerState(t *testing.T) {
	var cancelled atomic.Int32
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return nil, nil
		},
	}
	connection := openidea.DockerConnection{
		ID: "conn", Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.services.docker.snapshots = map[string]openidea.DockerSnapshot{
		"conn": {Containers: []openidea.DockerContainer{{ID: "container", State: "running"}}},
	}
	node := serviceTreeNode{
		key: "docker:conn:container:container", dockerConn: "conn",
		dockerID: "container", nodeKind: "docker-container", live: true,
	}
	old := &dockerLogStream{
		key: node.key, connection: "conn", container: "container",
		request: 17, channel: 18, loading: true, sourceLive: false,
		cancel: func() { cancelled.Add(1) },
	}
	m.services.docker.logs = map[string]*dockerLogStream{node.key: old}
	stream := m.ensureDockerDetailLog(connection, node, true)
	if stream == nil || stream == old || !stream.follow || !stream.sourceLive ||
		!old.stopped || cancelled.Load() != 1 {
		t.Fatalf("state transition did not replace the stream: old=%+v new=%+v", old, stream)
	}

	editor := &Tab{}
	editorStream := &dockerLogStream{key: "editor", editor: editor}
	m.services.docker.logs["editor"] = editorStream
	m.retainDockerDetailLog("")
	if _, ok := m.services.docker.logs[node.key]; ok {
		t.Fatal("hidden detail log stream remains active")
	}
	if m.services.docker.logs["editor"] != editorStream {
		t.Fatal("editor log stream was stopped with its detail view")
	}
}
