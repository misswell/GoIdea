package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestDockerOpenIDEAActionsAreRegisteredAndTargetAware(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.services.root = m.Root()
	ids := []string{
		"docker.setComposeFileOverride", "docker.refresh", "docker.start", "docker.stop",
		"docker.restart", "docker.remove", "docker.showDashboard", "docker.showLog",
		"docker.openLogInEditor", "docker.showProcesses", "docker.showLayers",
		"docker.inspect", "docker.toggleStoppedContainers", "docker.toggleUntaggedImages",
		"docker.execShell", "docker.pullImage", "docker.prune", "docker.composeUp",
	}
	for _, id := range ids {
		if _, ok := actionGet(id); !ok {
			t.Fatalf("%s is missing from the action registry", id)
		}
	}
	if !hiddenActionIDs["docker.setComposeFileOverride"] {
		t.Fatal("the integration-only Compose override action is visible in Find Action")
	}

	project := "demo"
	connection := openidea.DockerConnection{ID: "docker-one"}
	m.services.docker.loaded = true
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.services.docker.snapshots = map[string]openidea.DockerSnapshot{}
	m.services.docker.snapshots[connection.ID] = openidea.DockerSnapshot{
		Availability: openidea.DockerAvailability{Kind: "ready"},
		Containers: []openidea.DockerContainer{
			{ID: "stopped", Name: "worker", State: "exited"},
			{ID: "running", Name: "web", State: "running"},
			{
				ID: "compose-one", Name: "demo-api-1", State: "exited",
				ComposeProject: &project, ComposeService: stringPointerForDockerTest("api"),
			},
		},
	}

	m.selectOnlyServiceKey(dockerChildKey(connection.ID, "container:stopped"))
	stoppedEnabled := map[string]bool{}
	for _, id := range []string{
		"docker.start", "docker.stop", "docker.restart", "docker.showProcesses",
		"docker.execShell", "docker.showLog", "docker.openLogInEditor",
		"docker.inspect", "docker.remove",
	} {
		stoppedEnabled[id] = m.ActionEnabled(id)
	}
	if !stoppedEnabled["docker.start"] || stoppedEnabled["docker.stop"] ||
		!stoppedEnabled["docker.restart"] || stoppedEnabled["docker.showProcesses"] ||
		stoppedEnabled["docker.execShell"] {
		t.Fatalf("stopped container action availability is wrong: %+v", stoppedEnabled)
	}
	if !stoppedEnabled["docker.showLog"] || !stoppedEnabled["docker.openLogInEditor"] ||
		!stoppedEnabled["docker.inspect"] || !stoppedEnabled["docker.remove"] {
		t.Fatalf("stopped container detail actions are unavailable: enabled=%+v node=%+v selected=%q", stoppedEnabled, m.selectedDockerTarget(), m.services.selected)
	}
	if m.ActionEnabled("docker.showLayers") {
		t.Fatal("container exposed the image Layers action")
	}

	m.selectOnlyServiceKey(dockerChildKey(connection.ID, "container:running"))
	if m.ActionEnabled("docker.start") || !m.ActionEnabled("docker.stop") ||
		!m.ActionEnabled("docker.restart") || !m.ActionEnabled("docker.showProcesses") ||
		!m.ActionEnabled("docker.execShell") {
		t.Fatal("running container action availability is wrong")
	}

	m.selectOnlyServiceKey(dockerChildKey(connection.ID, "compose:"+project))
	if !m.ActionEnabled("docker.start") || m.ActionEnabled("docker.stop") ||
		!m.ActionEnabled("docker.restart") || !m.ActionEnabled("docker.remove") {
		t.Fatal("Compose action availability is wrong")
	}
}

func TestServicesAddServiceActionOpensTheNativeChooser(t *testing.T) {
	m := newServicesTestModel(t)
	if !m.RunAction("services.addService") {
		t.Fatal("services.addService did not run")
	}
	if m.services.addChooser == nil || !m.services.addChooser.open {
		t.Fatal("the Add Service action did not open its native chooser")
	}
	if !m.dockWindowShown("services") {
		t.Fatal("the Add Service action did not reveal the Services window")
	}
	ids := map[string]bool{}
	for _, overlay := range m.openOverlays() {
		ids[overlay.ID] = true
	}
	if !ids["services.addService"] || !ids["run.serviceImport"] {
		t.Fatalf("service import overlays opened as %+v", ids)
	}
}

func TestDockerStartUsesTheOpenIDEAContainerOperation(t *testing.T) {
	type call struct {
		command string
		args    map[string]any
	}
	calls := make(chan call, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- call{command: command, args: args}
			return json.RawMessage(`{"availability":{"kind":"ready"}}`), nil
		},
	}
	m.setRoot(t.TempDir())
	m.services.root = m.Root()
	connection := openidea.DockerConnection{
		ID: "docker-one", Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}
	m.services.docker.loaded = true
	m.services.docker.settings.Connections = []openidea.DockerConnection{connection}
	m.services.docker.snapshots = map[string]openidea.DockerSnapshot{}
	m.services.docker.snapshots[connection.ID] = openidea.DockerSnapshot{
		Availability: openidea.DockerAvailability{Kind: "ready"},
		Containers: []openidea.DockerContainer{{
			ID: "container-id", Name: "worker", State: "exited",
		}},
	}
	m.selectOnlyServiceKey(dockerChildKey(connection.ID, "container:container-id"))
	if !m.RunAction("docker.start") {
		t.Fatal("docker.start did not run")
	}
	select {
	case got := <-calls:
		if got.command != "docker_container_op" || got.args["op"] != "start" {
			t.Fatalf("call = %#v", got)
		}
		ids, _ := got.args["ids"].([]string)
		if len(ids) != 1 || ids[0] != "container-id" {
			t.Fatalf("ids = %#v", got.args["ids"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("docker_container_op was not invoked")
	}
}

func TestDockerComposeOverrideControlsDetectionAndPersists(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	path := filepath.Join(root, "deploy", "stack.yml")
	if !m.isComposeFile(path, "services:\n  api:\n    image: test\n") {
		t.Fatal("Compose content was not detected")
	}
	m.RunActionWithArg("docker.setComposeFileOverride", map[string]any{
		"path": path, "status": "excluded",
	})
	if m.isComposeFile(path, "services:\n  api:\n    image: test\n") {
		t.Fatal("excluded YAML was still treated as Compose")
	}
	stored := m.prefs.Services.Projects[servicesProjectKey(root)]
	if stored.ComposeOverrides[filepath.Clean(path)] != "excluded" {
		t.Fatalf("stored overrides = %#v", stored.ComposeOverrides)
	}

	m.RunActionWithArg("docker.setComposeFileOverride", map[string]any{
		"path": path, "status": "kept",
	})
	if !m.isComposeFile(path, "name: arbitrary\n") {
		t.Fatal("kept YAML was not treated as Compose")
	}
}

func TestDockerComposeTargetAtCaretUsesTheServiceBlock(t *testing.T) {
	text := "name: demo\nservices:\n  web:\n    image: web\n  worker:\n    image: worker\nvolumes:\n  data: {}\n"
	if target, ok := dockerComposeTargetAtLine(text, 3); !ok ||
		target.kind != "composeService" || target.service != "web" {
		t.Fatalf("web target = %+v, %v", target, ok)
	}
	if target, ok := dockerComposeTargetAtLine(text, 4); !ok ||
		target.kind != "composeService" || target.service != "worker" {
		t.Fatalf("worker target = %+v, %v", target, ok)
	}
	if target, ok := dockerComposeTargetAtLine(text, 7); !ok ||
		target.kind != "composeAll" {
		t.Fatalf("outside target = %+v, %v", target, ok)
	}
}

func stringPointerForDockerTest(value string) *string {
	return &value
}
