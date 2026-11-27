package app

import (
	"context"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

type dockerSettingsUI struct {
	open, formOpen, saving, existing bool
	sshLoading                       bool
	sshSettingsOpened                bool
	sshRequest                       string
	formToken                        string
	editing                          string
	id                               string
	name                             string
	kind                             string
	path                             string
	host                             string
	tls                              string
	context                          string
	sshConfig                        string
	address                          string
	username                         string
	password                         string
	err                              string
	sshErr                           string
	sshConfigs                       []openidea.SSHConfiguration
}

func (m *Model) openDockerSettings() {
	m.services.docker.settingsUI = dockerSettingsUI{open: true}
	m.loadDockerServices()
	m.invalidate()
}

func (m *Model) editDockerConnection(connection *openidea.DockerConnection) {
	u := &m.services.docker.settingsUI
	settingsOpen := u.open
	*u = dockerSettingsUI{
		open: settingsOpen, formOpen: true,
		editing: "connection", formToken: adapter.UID(),
		name: "Docker", kind: "socket", path: "/var/run/docker.sock",
	}
	if connection != nil {
		u.existing = true
		u.id, u.name, u.kind = connection.ID, connection.Name, connection.Endpoint.Kind
		u.path, u.host, u.context = connection.Endpoint.Path, connection.Endpoint.Host, connection.Endpoint.Name
		if connection.Endpoint.TLSCertDir != nil {
			u.tls = *connection.Endpoint.TLSCertDir
		}
	}
	m.loadDockerSSHConfigurations(false)
	m.invalidate()
}

func (m *Model) loadDockerSSHConfigurations(force bool) {
	u := &m.services.docker.settingsUI
	if u.sshLoading || (!force && (len(u.sshConfigs) > 0 || u.sshErr != "")) {
		return
	}
	if m.invokeHook == nil && m.mgr == nil {
		u.sshLoading = false
		u.sshErr = "The Docker backend is unavailable."
		return
	}
	request := adapter.UID()
	u.sshRequest = request
	u.sshLoading = true
	u.sshErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "settings_get_global", nil)
		var configs []openidea.SSHConfiguration
		if err == nil {
			configs, err = openidea.DecodeSSHConfigurations(raw)
		}
		m.post(func() {
			if !u.formOpen || u.editing != "connection" || u.sshRequest != request {
				return
			}
			u.sshLoading = false
			if err != nil {
				u.sshErr = err.Error()
			} else {
				u.sshConfigs = configs
			}
			m.invalidate()
		})
	}()
}

func dockerSSHConfigURL(config openidea.SSHConfiguration) string {
	host := strings.TrimSpace(config.Host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	if user := strings.TrimSpace(config.User); user != "" {
		host = url.User(user).String() + "@" + host
	}
	if config.Port != nil && *config.Port > 0 {
		host += ":" + strconv.Itoa(*config.Port)
	}
	return "ssh://" + host
}

func dockerSSHConfigForURL(raw string, configs []openidea.SSHConfiguration) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "ssh") ||
		parsed.Hostname() == "" || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return ""
	}
	portText := parsed.Port()
	var port int
	if portText != "" {
		port, err = strconv.Atoi(portText)
		if err != nil || port <= 0 || port > 65535 {
			return ""
		}
	}
	user := ""
	if parsed.User != nil {
		user = parsed.User.Username()
	}
	for _, config := range configs {
		if config.Auth != "openSsh" ||
			!strings.EqualFold(strings.TrimSpace(config.Host), parsed.Hostname()) {
			continue
		}
		if user != "" && config.User != user {
			continue
		}
		if portText != "" && (config.Port == nil || *config.Port != port) {
			continue
		}
		return config.ID
	}
	return ""
}

func (m *Model) editDockerRegistry(registry *openidea.DockerRegistry) {
	u := &m.services.docker.settingsUI
	settingsOpen := u.open
	*u = dockerSettingsUI{
		open: settingsOpen, formOpen: true,
		editing: "registry", formToken: adapter.UID(), id: adapter.UID(),
		name: "Docker Registry", address: "docker.io",
	}
	if registry != nil {
		u.existing = true
		u.id, u.name, u.address, u.username = registry.ID, registry.Name, registry.Address, registry.Username
	}
	m.invalidate()
}

func dockerSettingsEndpoint(u dockerSettingsUI) (openidea.DockerEndpoint, bool) {
	ep := openidea.DockerEndpoint{Kind: u.kind}
	switch u.kind {
	case "default":
	case "socket":
		ep.Path = strings.TrimSpace(u.path)
		if ep.Path == "" {
			return ep, false
		}
	case "tcp":
		ep.Host = strings.TrimSpace(u.host)
		if ep.Host == "" {
			return ep, false
		}
		if tls := strings.TrimSpace(u.tls); tls != "" {
			ep.TLSCertDir = &tls
		}
	case "context":
		ep.Name = strings.TrimSpace(u.context)
		if ep.Name == "" || strings.HasPrefix(ep.Name, "-") {
			return ep, false
		}
	default:
		return ep, false
	}
	return ep, true
}

func dockerEndpointKindLabel(kind string) string {
	switch kind {
	case "default":
		return "Docker CLI default (DOCKER_HOST / current context)"
	case "socket":
		return "Unix socket"
	case "tcp":
		return "TCP / SSH host"
	case "context":
		return "Docker context"
	default:
		return kind
	}
}

func dockerSettingsFormTitle(u dockerSettingsUI) string {
	if u.editing == "registry" {
		if u.existing {
			return "Edit Docker Registry"
		}
		return "New Docker Registry"
	}
	if u.existing {
		return "Edit Docker Connection"
	}
	return "New Docker Connection"
}

func dockerSettingsFormValid(u dockerSettingsUI) bool {
	if strings.TrimSpace(u.name) == "" {
		return false
	}
	if u.editing == "registry" {
		return strings.TrimSpace(u.address) != ""
	}
	if u.editing != "connection" {
		return false
	}
	_, ok := dockerSettingsEndpoint(u)
	return ok
}

func (m *Model) saveDockerSettingsForm(
	settings openidea.DockerSettings,
	registryID, password string,
	updatePassword bool,
) {
	d := &m.services.docker
	u := &d.settingsUI
	if u.saving {
		return
	}
	editingConnectionID := ""
	if u.editing == "connection" {
		editingConnectionID = u.id
	}
	epoch := m.dockerEpoch()
	u.saving, u.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "docker_save_settings", map[string]any{"settings": settings})
		var passwordErr error
		if err == nil && updatePassword {
			var passwordValue any
			if password != "" {
				passwordValue = password
			}
			_, passwordErr = m.invoke(ctx, "docker_set_registry_password",
				map[string]any{"id": registryID, "password": passwordValue})
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			u.saving = false
			if err != nil {
				u.err = err.Error()
				m.invalidate()
				return
			}
			d.settings = settings
			if editingConnectionID != "" {
				m.stopDockerConnection(editingConnectionID)
			}
			if passwordErr != nil {
				u.id = registryID
				u.err = "Settings saved, but the registry password could not be saved."
				m.invalidate()
				return
			}
			u.password = ""
			u.editing = ""
			u.formOpen = false
			u.sshSettingsOpened = false
			for _, connection := range settings.Connections {
				m.refreshDockerSnapshot(connection)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) submitDockerSettings() {
	d := &m.services.docker
	u := &d.settingsUI
	if u.saving || !dockerSettingsFormValid(*u) {
		return
	}
	name := strings.TrimSpace(u.name)
	if name == "" {
		u.err = "Name is required."
		return
	}
	settings := openidea.DockerSettings{Connections: append([]openidea.DockerConnection{}, d.settings.Connections...), Registries: append([]openidea.DockerRegistry{}, d.settings.Registries...)}
	id := u.id
	if id == "" {
		id = adapter.UID()
	}
	switch u.editing {
	case "connection":
		endpoint, ok := dockerSettingsEndpoint(*u)
		if !ok {
			u.err = "Enter a valid Docker endpoint."
			return
		}
		item := openidea.DockerConnection{ID: id, Name: name, Endpoint: endpoint}
		found := false
		for i := range settings.Connections {
			if settings.Connections[i].ID == id {
				settings.Connections[i] = item
				found = true
				break
			}
		}
		if !found {
			settings.Connections = append(settings.Connections, item)
		}
	case "registry":
		address := strings.TrimSpace(u.address)
		if address == "" {
			u.err = "Registry address is required."
			return
		}
		item := openidea.DockerRegistry{ID: id, Name: name, Address: address, Username: strings.TrimSpace(u.username)}
		found := false
		for i := range settings.Registries {
			if settings.Registries[i].ID == id {
				settings.Registries[i] = item
				found = true
				break
			}
		}
		if !found {
			settings.Registries = append(settings.Registries, item)
		}
	default:
		return
	}
	password := ""
	updatePassword := false
	if u.editing == "registry" {
		password = u.password
		updatePassword = password != "" || !u.existing
	}
	m.saveDockerSettingsForm(settings, id, password, updatePassword)
}

func dockerSettingsField(c *ui.Context, p *ui.Theme, label string, value *string) {
	oi := oiPaletteOf(p)
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label, settingsLabelMaxWidth)
		ui.TextInput(c, value).Grow(1).MaxWidth(settingsPathMaxWidth)
	})
}

func (m *Model) browseDockerTLSDirectory(u *dockerSettingsUI) {
	parent := m.win
	current := strings.TrimSpace(u.tls)
	token := u.formToken
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       "Certificates Folder",
			ButtonLabel: "Choose",
			DefaultPath: current,
			Directory:   true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			if !u.formOpen || u.editing != "connection" || u.formToken != token {
				return
			}
			u.tls = filepath.Clean(paths[0])
			m.invalidate()
		})
	}()
}

func (m *Model) dockerTLSDirectoryField(
	c *ui.Context,
	p *ui.Theme,
	u *dockerSettingsUI,
) {
	oi := oiPaletteOf(p)
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Certificates folder (TLS, optional)", settingsLabelMaxWidth)
		ui.TextInput(c, &u.tls).Grow(1).MaxWidth(settingsPathMaxWidth).
			Placeholder("contains ca.pem, cert.pem and key.pem")
		if ui.Button(c, "Browse…").Clicked() {
			m.browseDockerTLSDirectory(u)
		}
	})
}

func dockerSSHConfigByID(
	configs []openidea.SSHConfiguration,
	id string,
) (openidea.SSHConfiguration, bool) {
	for _, config := range configs {
		if config.ID == id {
			return config, true
		}
	}
	return openidea.SSHConfiguration{}, false
}

func dockerSSHConfigLabel(config openidea.SSHConfiguration) string {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = config.Label()
	}
	if config.Auth != "openSsh" {
		name += " (does not use OpenSSH config)"
	}
	return name
}

func (m *Model) dockerSSHConfigPicker(c *ui.Context, p *ui.Theme, u *dockerSettingsUI) {
	oi := oiPaletteOf(p)
	if u.sshSettingsOpened && !m.remote.sshSettingsOpen {
		u.sshSettingsOpened = false
		if len(m.remote.sshConfigs) > 0 {
			u.sshConfigs = append([]openidea.SSHConfiguration(nil), m.remote.sshConfigs...)
		}
		m.loadDockerSSHConfigurations(true)
	}
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "SSH configuration", settingsLabelMaxWidth)
		if u.sshLoading {
			ui.Text(c, "Loading SSH configurations…").FontSize(12).TextColor(p.TextMuted).Grow(1)
			return
		}
		if u.sshErr != "" {
			ui.Text(c, "Cannot read SSH configurations: "+u.sshErr).
				FontSize(11).TextColor(p.Danger).Grow(1)
			if ui.Button(c, "Retry").Clicked() {
				m.loadDockerSSHConfigurations(true)
			}
			return
		}
		fromURL := dockerSSHConfigForURL(u.host, u.sshConfigs)
		shown := fromURL
		if shown == "" {
			shown = u.sshConfig
		}
		label := "None — type the Engine API URL below"
		if config, ok := dockerSSHConfigByID(u.sshConfigs, shown); ok {
			label = dockerSSHConfigLabel(config)
		}
		button := ui.Button(c, label).Grow(1).MaxWidth(settingsControlMaxWidth)
		button.Menu(func(menu *ui.Menu) {
			if menu.Item("None — type the Engine API URL below").Checked(shown == "").Chosen() {
				u.sshConfig = ""
			}
			for _, config := range u.sshConfigs {
				config := config
				if menu.Item(dockerSSHConfigLabel(config)).Checked(shown == config.ID).Chosen() {
					u.sshConfig = config.ID
					if config.Auth == "openSsh" && strings.TrimSpace(config.Host) != "" {
						u.host = dockerSSHConfigURL(config)
					}
				}
			}
		})
		edit := ui.Button(c, "…")
		edit.Label("Edit SSH Configurations")
		if edit.Clicked() {
			m.remote.sshConfigs = append([]openidea.SSHConfiguration(nil), u.sshConfigs...)
			m.openRemoteSSHSettings()
			u.sshSettingsOpened = true
			m.invalidate()
		}
	})
	fromURL := dockerSSHConfigForURL(u.host, u.sshConfigs)
	if fromURL == "" {
		if config, ok := dockerSSHConfigByID(u.sshConfigs, u.sshConfig); ok && config.Auth != "openSsh" {
			ui.Text(c, "Docker over SSH uses the system ssh. Choose a configuration with OpenSSH config authentication.").
				FontSize(11).TextColor(p.Danger)
		}
	}
}

func (m *Model) dockerSettingsView(c *ui.Context, p *ui.Theme) {
	d := &m.services.docker
	u := &d.settingsUI
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Docker Settings").FontSize(15).Bold().TextColor(p.Text).Grow(1)
				if ui.Button(c, "Close").Clicked() && !u.saving {
					u.open = false
				}
			})
			if d.loading {
				ui.Text(c, "Loading Docker settings…").FontSize(12).TextColor(p.TextMuted)
				return
			}
			if errText := d.errors["settings"]; errText != "" {
				ui.Text(c, "Cannot read Docker settings: "+errText).FontSize(12).TextColor(p.Danger)
				if ui.Button(c, "Retry").Clicked() {
					d.loaded = false
					delete(d.errors, "settings")
					m.loadDockerServices()
				}
				return
			}
			ui.Text(c, "Connections").FontSize(13).Bold().TextColor(p.Text)
			for i := range d.settings.Connections {
				item := d.settings.Connections[i]
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, item.Name+" — "+dockerEndpointText(item.Endpoint)).FontSize(12).TextColor(p.Text).Grow(1)
					if ui.Button(c, "Edit").Clicked() {
						m.editDockerConnection(&item)
					}
					if ui.Button(c, "Remove…").Clicked() {
						m.removeDockerConnection(item)
					}
				})
			}
			if ui.Button(c, "Add Connection…").Clicked() {
				m.editDockerConnection(nil)
			}
			ui.Text(c, "Registries").FontSize(13).Bold().TextColor(p.Text)
			for i := range d.settings.Registries {
				item := d.settings.Registries[i]
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, item.Name+" — "+item.Address).FontSize(12).TextColor(p.Text).Grow(1)
					if ui.Button(c, "Edit").Clicked() {
						m.editDockerRegistry(&item)
					}
					test := ui.Button(c, "Test Login")
					test.Disabled(item.Username == "")
					if test.Clicked() {
						m.testDockerRegistryLogin(item)
					}
					if ui.Button(c, "Remove…").Clicked() {
						m.removeDockerRegistry(item)
					}
				})
			}
			if ui.Button(c, "Add Registry…").Clicked() {
				m.editDockerRegistry(nil)
			}
		})
	})
}

func (m *Model) cancelDockerSettingsEditing() {
	u := &m.services.docker.settingsUI
	u.formOpen = false
	u.editing = ""
	u.password = ""
	u.err = ""
	u.sshSettingsOpened = false
	m.invalidate()
}

func (m *Model) dockerSettingsFormDialog(c *ui.Context, p *ui.Theme) {
	u := &m.services.docker.settingsUI
	if !u.formOpen || u.editing == "" {
		return
	}
	open := true
	modal := nativeModal(c.Key("services:docker-settings-form"), &open, func() {
		ui.Column(c).Width(520).MaxWidth(720).Padding(18).Gap(9).Children(func() {
			ui.Text(c, dockerSettingsFormTitle(*u)).FontSize(15).Bold().TextColor(p.Text)
			dockerSettingsField(c, p, "Name", &u.name)
			if u.editing == "connection" {
				button := ui.Button(c, "Connect with: "+dockerEndpointKindLabel(u.kind))
				button.Menu(func(menu *ui.Menu) {
					for _, kind := range []string{"default", "socket", "tcp", "context"} {
						kind := kind
						if menu.Item(dockerEndpointKindLabel(kind)).
							Checked(u.kind == kind).Chosen() {
							u.kind = kind
						}
					}
				})
				switch u.kind {
				case "socket":
					dockerSettingsField(c, p, "Socket path or URL", &u.path)
				case "tcp":
					m.dockerSSHConfigPicker(c, p, u)
					dockerSettingsField(c, p, "Engine API URL", &u.host)
					m.dockerTLSDirectoryField(c, p, u)
				case "context":
					dockerSettingsField(c, p, "Context name", &u.context)
				}
			} else {
				dockerSettingsField(c, p, "Address", &u.address)
				dockerSettingsField(c, p, "Username", &u.username)
				oi := oiPaletteOf(p)
				ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
					settingsFormLabel(c, oi, "Password", settingsLabelMaxWidth)
					placeholder := "Password or access token"
					if u.existing {
						placeholder = "Unchanged"
					}
					ui.TextInput(c, &u.password).Grow(1).MaxWidth(settingsControlMaxWidth).
						Password().Placeholder(placeholder)
				})
				ui.Text(c, "Images from this registry are pulled with these credentials.").
					FontSize(11).TextColor(p.TextMuted)
			}
			if u.err != "" {
				ui.Text(c, u.err).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if u.editing == "registry" {
					login := m.services.docker.logins[u.id]
					test := ui.Button(c, "Test Connection")
					test.Disabled(u.saving || strings.TrimSpace(u.name) == "" ||
						strings.TrimSpace(u.address) == "" ||
						strings.TrimSpace(u.username) == "" ||
						(login != nil && login.loading))
					if test.Clicked() {
						m.testDockerRegistryLoginWithPassword(openidea.DockerRegistry{
							ID: u.id, Name: strings.TrimSpace(u.name),
							Address:  strings.TrimSpace(u.address),
							Username: strings.TrimSpace(u.username),
						}, &u.password)
					}
				}
				ui.Spacer(c)
				cancel := ui.Button(c, "Cancel")
				cancel.Disabled(u.saving)
				if cancel.Clicked() {
					m.cancelDockerSettingsEditing()
				}
				ok := ui.Button(c, "OK")
				ok.Disabled(u.saving || !dockerSettingsFormValid(*u))
				if ok.Clicked() {
					m.submitDockerSettings()
				}
			})
			if u.editing == "registry" {
				if login := m.services.docker.logins[u.id]; login != nil {
					if login.loading {
						ui.Text(c, "Testing registry login…").FontSize(12).TextColor(p.TextMuted)
					}
					if login.answer != "" {
						ui.Text(c, login.answer).FontSize(12).TextColor(p.Success)
					}
					if login.err != "" {
						ui.Text(c, login.err).FontSize(12).TextColor(p.Danger)
					}
				}
			}
		})
	})
	if u.saving {
		return
	}
	if !open || modal.Dismissed() {
		m.cancelDockerSettingsEditing()
	}
}
