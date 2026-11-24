package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestDockerSettingsEndpointValidation(t *testing.T) {
	tests := []struct {
		ui    dockerSettingsUI
		valid bool
		want  openidea.DockerEndpoint
	}{
		{dockerSettingsUI{kind: "default"}, true, openidea.DockerEndpoint{Kind: "default"}},
		{dockerSettingsUI{kind: "socket", path: " /tmp/docker.sock "}, true, openidea.DockerEndpoint{Kind: "socket", Path: "/tmp/docker.sock"}},
		{dockerSettingsUI{kind: "socket"}, false, openidea.DockerEndpoint{}},
		{dockerSettingsUI{kind: "tcp", host: " ssh://docker "}, true, openidea.DockerEndpoint{Kind: "tcp", Host: "ssh://docker"}},
		{dockerSettingsUI{kind: "context", context: "-bad"}, false, openidea.DockerEndpoint{}},
		{dockerSettingsUI{kind: "context", context: " production "}, true, openidea.DockerEndpoint{Kind: "context", Name: "production"}},
	}
	for _, tt := range tests {
		got, ok := dockerSettingsEndpoint(tt.ui)
		if ok != tt.valid || (ok && got != tt.want) {
			t.Errorf("endpoint(%+v) = %+v, %v; want %+v, %v", tt.ui, got, ok, tt.want, tt.valid)
		}
	}
}

func TestDockerSSHConfigURLAndSelection(t *testing.T) {
	port := 2222
	configs := []openidea.SSHConfiguration{
		{
			ID: "open", Name: "Build", Host: "build-host", User: "me",
			Port: &port, Auth: "openSsh",
		},
		{
			ID: "password", Name: "Password", Host: "password-host",
			User: "deploy", Auth: "password",
		},
	}
	if got := dockerSSHConfigURL(configs[0]); got != "ssh://me@build-host:2222" {
		t.Fatalf("OpenSSH URL = %q", got)
	}
	tests := []struct {
		url  string
		want string
	}{
		{"ssh://me@build-host:2222", "open"},
		{"SSH://me@BUILD-HOST:2222", "open"},
		{"ssh://build-host", "open"},
		{"ssh://build-host:2222", "open"},
		{"ssh://other@build-host:2222", ""},
		{"ssh://me@build-host:2200", ""},
		{"tcp://build-host:2376", ""},
		{"ssh://build-host/path", ""},
		{"ssh://password-host", ""},
	}
	for _, tt := range tests {
		if got := dockerSSHConfigForURL(tt.url, configs); got != tt.want {
			t.Errorf("dockerSSHConfigForURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
	if got := dockerSSHConfigLabel(configs[1]); !strings.Contains(got, "does not use OpenSSH config") {
		t.Fatalf("unsupported label = %q", got)
	}
}

func TestDockerSSHConfigurationsLoad(t *testing.T) {
	posts := make(chan func(), 1)
	m := &Model{
		postHook: func(fn func()) { posts <- fn },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "settings_get_global" {
				t.Fatalf("command = %s", command)
			}
			return json.RawMessage(`{"ssh":{"configs":[
				{"id":"build","name":"Build","host":"build.example.com","user":"deploy","auth":"openSsh"}
			]}}`), nil
		},
	}
	m.services.docker.settingsUI = dockerSettingsUI{
		formOpen: true, editing: "connection", kind: "tcp",
	}
	m.loadDockerSSHConfigurations(false)
	select {
	case fn := <-posts:
		fn()
	case <-time.After(time.Second):
		t.Fatal("SSH configurations did not load")
	}
	u := m.services.docker.settingsUI
	if u.sshLoading || u.sshErr != "" || len(u.sshConfigs) != 1 ||
		u.sshConfigs[0].ID != "build" || u.sshConfigs[0].Auth != "openSsh" {
		t.Fatalf("SSH configuration state = %+v", u)
	}
}

func TestDockerSettingsRendersSSHConfigPicker(t *testing.T) {
	m := &Model{}
	port := 2222
	m.services.docker.settingsUI = dockerSettingsUI{
		formOpen: true, editing: "connection", name: "Build Docker", kind: "tcp",
		host: "ssh://deploy@build.example.com:2222",
		sshConfigs: []openidea.SSHConfiguration{{
			ID: "build", Name: "Build host", Host: "build.example.com",
			User: "deploy", Port: &port, Auth: "openSsh",
		}},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.dockerSettingsFormDialog(c, c.Theme())
	}, 900, 700)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	for _, text := range []string{
		"New Docker Connection", "SSH configuration", "Build host", "Edit SSH Configurations",
		"Engine API URL", "Certificates folder (TLS, optional)", "Browse…", "OK", "Cancel",
	} {
		if !tt.HasText(text) {
			t.Fatalf("Docker Connection settings is missing %q: %v", text, tt.Texts())
		}
	}
}

func TestDockerSettingsSaveFailureKeepsDraft(t *testing.T) {
	posts := make(chan func(), 1)
	m := &Model{postHook: func(fn func()) { posts <- fn }, invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "docker_save_settings" {
			t.Errorf("unexpected command %s", command)
		}
		payload := args.(map[string]any)["settings"].(openidea.DockerSettings)
		if len(payload.Registries) != 1 || payload.Registries[0].Address != "example.com" {
			t.Errorf("unexpected settings: %+v", payload)
		}
		return nil, errors.New("save failed")
	}}
	m.services.docker.settingsUI = dockerSettingsUI{open: true, editing: "registry", name: "Example", address: "example.com"}
	m.submitDockerSettings()
	select {
	case fn := <-posts:
		fn()
	case <-time.After(time.Second):
		t.Fatal("save did not finish")
	}
	u := m.services.docker.settingsUI
	if u.saving || u.err != "save failed" || u.editing != "registry" || u.address != "example.com" {
		t.Fatalf("draft lost after error: %+v", u)
	}
	if len(m.services.docker.settings.Registries) != 0 {
		t.Fatal("failed save changed committed settings")
	}
}

func TestDockerRegistryPasswordUpdateSemantics(t *testing.T) {
	t.Run("new blank password is forgotten", func(t *testing.T) {
		posts := make(chan func(), 1)
		calls := make(chan string, 2)
		passwords := make(chan any, 1)
		m := &Model{
			postHook: func(fn func()) { posts <- fn },
			invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
				calls <- command
				if command == "docker_set_registry_password" {
					passwords <- args.(map[string]any)["password"]
				}
				return []byte("null"), nil
			},
		}
		m.services.docker.settingsUI = dockerSettingsUI{
			open: true, editing: "registry", name: "Example",
			address: "example.com", username: "user",
		}
		m.submitDockerSettings()
		if got := <-calls; got != "docker_save_settings" {
			t.Fatalf("first command = %s", got)
		}
		if got := <-calls; got != "docker_set_registry_password" {
			t.Fatalf("second command = %s", got)
		}
		if got := <-passwords; got != nil {
			t.Fatalf("password = %#v, want JSON null", got)
		}
		(<-posts)()
	})

	t.Run("editing with blank password keeps it", func(t *testing.T) {
		posts := make(chan func(), 1)
		calls := make(chan string, 2)
		m := &Model{
			postHook: func(fn func()) { posts <- fn },
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				calls <- command
				return []byte("null"), nil
			},
		}
		m.services.docker.settingsUI = dockerSettingsUI{
			open: true, editing: "registry", existing: true, id: "registry",
			name: "Example", address: "example.com", username: "user",
		}
		m.submitDockerSettings()
		if got := <-calls; got != "docker_save_settings" {
			t.Fatalf("first command = %s", got)
		}
		(<-posts)()
		select {
		case got := <-calls:
			t.Fatalf("unexpected password command %s", got)
		default:
		}
	})
}

func TestDockerSettingsFormEscapeCancelsWithoutClosingServices(t *testing.T) {
	m := &Model{}
	m.services.visible = true
	m.services.docker.settingsUI = dockerSettingsUI{
		formOpen: true, editing: "connection", name: "Build Docker", kind: "socket",
		path: "/var/run/docker.sock",
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.dockerSettingsFormDialog(c, c.Theme())
	}, 900, 700)
	tester.Frame()
	if !m.overlayOpen() {
		t.Fatal("Docker settings form is not registered as a modal overlay")
	}

	tester.Key(0, ui.KeyEscape)
	u := m.services.docker.settingsUI
	if u.editing != "" || u.formOpen || !m.services.visible {
		t.Fatalf("Escape result: settings=%+v visible=%v", u, m.services.visible)
	}
}

func TestDockerSettingsFormEscapeIsIgnoredWhileSaving(t *testing.T) {
	m := &Model{}
	m.services.visible = true
	m.services.docker.settingsUI = dockerSettingsUI{
		formOpen: true, editing: "registry", saving: true,
		name: "Registry", address: "ghcr.io", username: "me",
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.dockerSettingsFormDialog(c, c.Theme())
	}, 900, 700)
	tester.Frame()

	tester.Key(0, ui.KeyEscape)
	u := m.services.docker.settingsUI
	if u.editing != "registry" || !u.formOpen || !u.saving {
		t.Fatalf("Escape changed a saving form: %+v", u)
	}
}

func TestDockerAddEditDialogDoesNotOpenTheSettingsPage(t *testing.T) {
	m := &Model{}
	m.editDockerConnection(nil)
	if !m.services.docker.settingsUI.formOpen || m.services.docker.settingsUI.open {
		t.Fatalf("connection dialog state = %+v", m.services.docker.settingsUI)
	}
	m.cancelDockerSettingsEditing()

	m.openDockerSettings()
	m.editDockerRegistry(nil)
	if !m.services.docker.settingsUI.formOpen || !m.services.docker.settingsUI.open {
		t.Fatalf("registry dialog did not preserve the settings page = %+v", m.services.docker.settingsUI)
	}
}
