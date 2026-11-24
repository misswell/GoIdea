package app

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func TestRunTargetSelectionMapsNilEmptyAndIDTargets(t *testing.T) {
	remote := "build-box"
	for _, tt := range []struct {
		name   string
		target *string
		choice string
	}{
		{name: "project default", target: nil, choice: runTargetDefaultChoice},
		{name: "local machine", target: new(string), choice: runTargetLocalChoice},
		{name: "remote target", target: &remote, choice: remote},
	} {
		t.Run(tt.name, func(t *testing.T) {
			choice := runTargetChoiceForConfig(tt.target)
			if choice != tt.choice {
				t.Fatalf("choice = %q, want %q", choice, tt.choice)
			}
			got := runTargetChoicePointer(choice)
			if !reflect.DeepEqual(got, tt.target) {
				t.Fatalf("round trip = %#v, want %#v", got, tt.target)
			}
		})
	}
}

func TestRunTargetSettingsStayDeepCopiedAndParseContainerLists(t *testing.T) {
	settings := openidea.RunTargetSettings{
		DefaultTarget: "web",
		Targets: []openidea.RunTarget{{
			Kind:         openidea.RunTargetCompose,
			ID:           "web",
			Name:         "Web",
			ComposeFiles: []string{"compose.yaml"},
			Ports:        []string{"8000:8000"},
		}},
	}
	cloned := cloneRunTargetSettings(settings)
	cloned.Targets[0].ComposeFiles[0] = "changed.yaml"
	cloned.Targets[0].Ports[0] = "9000:9000"
	if settings.Targets[0].ComposeFiles[0] != "compose.yaml" ||
		settings.Targets[0].Ports[0] != "8000:8000" {
		t.Fatal("editing the draft changed the loaded settings")
	}
	got := parseRunTargetList("compose.yml, compose.override.yml; 8000:8000\n5678:5678")
	want := []string{"compose.yml", "compose.override.yml", "8000:8000", "5678:5678"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("list = %#v, want %#v", got, want)
	}
}

func TestRunConfigTargetPreservesRawObject(t *testing.T) {
	cfg, err := openidea.DecodeRunConfig(runConfigJSON(t, map[string]any{
		"id": "python:app", "name": "App", "kind": "python", "command": "python",
		"args": []string{"app.py"}, "unmodelled": map[string]any{"keep": true},
	}))
	if err != nil {
		t.Fatal(err)
	}
	d := &runConfigDlg{items: []*openidea.RunConfig{cfg}, sel: 0}
	d.loadForm()
	if d.runOn != runTargetDefaultChoice {
		t.Fatalf("initial Run on = %q", d.runOn)
	}
	d.runOn = "build-box"
	d.applyForm()
	if got := d.selected().Target; got == nil || *got != "build-box" {
		t.Fatalf("target = %#v", got)
	}
	obj, err := adapter.ConfigObject(d.selected())
	if err != nil {
		t.Fatal(err)
	}
	if obj["target"] != "build-box" {
		t.Fatalf("saved target = %#v", obj["target"])
	}
	if !reflect.DeepEqual(obj["unmodelled"], map[string]any{"keep": true}) {
		t.Fatalf("unmodelled field was lost: %#v", obj["unmodelled"])
	}

	d.runOn = runTargetLocalChoice
	d.applyForm()
	if got := d.selected().Target; got == nil || *got != "" {
		t.Fatalf("local target = %#v, want explicit empty string", got)
	}
	d.runOn = runTargetDefaultChoice
	d.applyForm()
	if got := d.selected().Target; got != nil {
		t.Fatalf("default target = %#v, want nil", got)
	}
}

func TestJVMClasspathResolutionPreservesTheRunTarget(t *testing.T) {
	raw := json.RawMessage(`{
		"id":"spring:app","name":"App","kind":"spring-boot","command":"mvn",
		"args":["spring-boot:run"],"target":"remote-build","unmodelled":{"keep":true}
	}`)
	resolved, err := resolvedRunConfig(raw, "/remote/jdk/bin/java", []string{
		"-cp", "/remote/project/target/classes:/remote/libs/lib.jar", "example.App",
	})
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(resolved, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["target"] != "remote-build" {
		t.Fatalf("resolved target = %#v", obj["target"])
	}
	if !reflect.DeepEqual(obj["unmodelled"], map[string]any{"keep": true}) {
		t.Fatalf("resolved unmodelled fields = %#v", obj["unmodelled"])
	}
	if got := obj["command"]; got != "/remote/jdk/bin/java" {
		t.Fatalf("resolved command = %#v", got)
	}
}

func TestRunTargetSettingsDialogDrawsSSHAndContainerFields(t *testing.T) {
	m := &Model{}
	state := &m.remote.runTargets
	state.loaded = true
	state.editorOpen = true
	state.pluginsLoaded = true
	state.dockerLoaded = true
	state.draft = openidea.RunTargetSettings{
		DefaultTarget: "web",
		Targets: []openidea.RunTarget{
			{
				Kind: openidea.RunTargetSSH, ID: "build", Name: "Build",
				SSHConfig: "ssh", ProjectRoot: "~/app", SyncBeforeRun: true,
			},
			{
				Kind: openidea.RunTargetCompose, ID: "web", Name: "Web",
				DockerConnection: "local", ComposeFiles: []string{"compose.yaml"},
				Service: "web", Ports: []string{"8000:8000"},
			},
		},
	}
	state.selectedTarget = 1
	state.resetItemText()
	state.plugins = []openidea.RegisteredRunTarget{{
		ID: "plugin:run", PluginID: "example", PluginName: "Example",
		Target: openidea.PluginRunTargetSpec{ID: "run", Name: "Plugin Target"},
	}}
	state.dockerConnections = []openidea.DockerConnection{{
		ID: "local", Name: "Local Docker",
	}}

	tester := ui.NewTester(func(c *ui.Context) {
		m.runTargetSettingsDialog(c, c.Theme())
	}, 1100, 800)
	tester.Frame()
	for _, want := range []string{
		"Run Targets", "Default run target", "Plugin Target", "Read-only",
		"Docker connection", "Compose files", "Service", "Published ports",
		"Run options", "Python interpreter", "Node.js interpreter",
		"Java executable", "Save", "Cancel",
	} {
		if !tester.HasText(want) {
			t.Errorf("Run Targets dialog lacks %q: %q", want, tester.Texts())
		}
	}
}

func TestSaveRunTargetsWritesCanonicalSettingsKeys(t *testing.T) {
	var (
		mu   sync.Mutex
		keys []string
		done = make(chan struct{})
		once sync.Once
	)
	model := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	model.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_set":
			payload := args.(map[string]any)
			mu.Lock()
			keys = append(keys, payload["key"].(string))
			mu.Unlock()
			return json.RawMessage(`{}`), nil
		case "settings_get":
			once.Do(func() { close(done) })
			return json.RawMessage(`{
				"runTargets":[{"kind":"compose","id":"web","name":"Web",
					"dockerConnection":"local","composeFiles":["compose.yml"],
					"service":"web","ports":["8000:8000"]}],
				"run":{"defaultTarget":"web"}
			}`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	state := &model.remote.runTargets
	state.loaded = true
	state.settings = openidea.RunTargetSettings{}
	state.draft = openidea.RunTargetSettings{
		DefaultTarget: "web",
		Targets: []openidea.RunTarget{{
			Kind: openidea.RunTargetCompose, ID: "web", Name: "Web",
			DockerConnection: "local", ComposeFiles: []string{"compose.yml"},
			Service: "web", Ports: []string{"8000:8000"},
		}},
	}
	state.resetItemText()
	state.editorOpen = true
	model.saveRunTargetSettings()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("settings were not reloaded after saving")
	}
	mu.Lock()
	gotKeys := append([]string(nil), keys...)
	mu.Unlock()
	wantKeys := []string{"runTargets", "run.defaultTarget"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("settings_set keys = %#v, want %#v", gotKeys, wantKeys)
	}
	for i := 0; i < 100 && (state.editorBusy || state.editorOpen); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if state.editorOpen || !state.loaded || state.settings.DefaultTarget != "web" {
		t.Fatalf("saved state = %+v", state)
	}
}

func TestRunTargetSSHRecoveryUsesTheDisplayedHostKeyFingerprint(t *testing.T) {
	const (
		message     = "rpc failed: Unknown SSH host key for build:22 (ED25519 SHA256:abcDEF_123-xyz)"
		fingerprint = "ED25519 SHA256:abcDEF_123-xyz"
	)
	if got := sshHostKeyFingerprint(message); got != fingerprint {
		t.Fatalf("fingerprint = %q, want %q", got, fingerprint)
	}
	if got := sshHostKeyFingerprint("Unknown SSH host key without a fingerprint"); got != "" {
		t.Fatalf("missing fingerprint parsed as %q", got)
	}

	calls := make(chan map[string]any, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "ssh_trust_host" {
				t.Errorf("command = %q, want ssh_trust_host", command)
			}
			calls <- args.(map[string]any)
			return json.RawMessage(`null`), nil
		},
	}
	retried := make(chan struct{}, 1)
	if !m.offerRunTargetAuth(message, "build", func() { retried <- struct{}{} }) {
		t.Fatal("host key error did not open recovery")
	}
	state := &m.remote.runTargets
	if !state.remedyTrust || state.remedyConfigID != "build" ||
		state.remedyFingerprint != fingerprint {
		t.Fatalf("recovery state = %+v", state)
	}
	m.retryRunTargetAuth()
	select {
	case args := <-calls:
		if args["id"] != "build" || args["fingerprint"] != fingerprint {
			t.Fatalf("ssh_trust_host args = %#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ssh_trust_host was not called")
	}
	select {
	case <-retried:
	case <-time.After(2 * time.Second):
		t.Fatal("run was not retried after trusting the key")
	}
	if state.remedyOpen || state.remedyFingerprint != "" {
		t.Fatalf("recovery did not close: %+v", state)
	}
}
