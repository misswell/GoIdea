package app

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func remoteSSHSessionID(arg any) string {
	switch value := arg.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		if id, ok := value["id"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// StartRemoteSSHSession implements Tools › Start SSH Session. With no id it
// shows the configuration chooser, as the pinned frontend does.
func (m *Model) StartRemoteSSHSession(arg any) {
	state := &m.remote
	state.sshSessionErr = ""
	id := remoteSSHSessionID(arg)
	if !state.configsLoaded || state.configsLoading {
		state.sshSessionOpen = true
		state.sshSessionWaiting = true
		state.sshSessionPendingID = id
		m.loadRemoteChoices()
		m.invalidate()
		return
	}
	if id != "" {
		m.startLoadedRemoteSSHSession(id)
		return
	}
	state.sshSessionOpen = true
	m.invalidate()
}

func (m *Model) startLoadedRemoteSSHSession(id string) {
	state := &m.remote
	if id == "" {
		state.sshSessionOpen = true
		m.invalidate()
		return
	}
	index := remoteSSHConfigIndex(state.sshConfigs, id)
	if index < 0 {
		state.sshSessionOpen = true
		state.sshSessionErr = "SSH configuration not found."
		m.invalidate()
		return
	}
	m.startRemoteSSHSessionOn(state.sshConfigs[index])
}

func (m *Model) startSelectedRemoteSSHSession(label string) {
	state := &m.remote
	for i, choice := range remoteSSHConfigurationChoices(state.sshConfigs) {
		if choice == label {
			m.startRemoteSSHSessionOn(state.sshConfigs[i])
			return
		}
	}
	state.sshSessionErr = "SSH configuration not found."
	m.invalidate()
}

func (m *Model) startRemoteSSHSessionOn(config openidea.SSHConfiguration) {
	state := &m.remote
	if strings.TrimSpace(config.Host) == "" {
		state.sshSessionOpen = true
		state.sshSessionErr = "The SSH configuration has no host."
		m.invalidate()
		return
	}
	if m.Root() == "" {
		state.sshSessionOpen = true
		state.sshSessionErr = "Open a project before starting an SSH session."
		m.invalidate()
		return
	}
	if m.mgr == nil {
		state.sshSessionOpen = true
		state.sshSessionErr = "The GoIDEA backend is not connected."
		m.invalidate()
		return
	}
	state.sshSessionOpen = false
	state.sshSessionPendingID = ""
	state.sshSessionWaiting = false
	if !m.term.visible {
		m.autoSaveOnSwitch()
		m.term.visible = true
		m.occupyBottom("term")
	}
	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = config.Label()
	}
	if name == "" {
		name = config.ID
	}
	m.status = "Starting SSH session " + name + "…"
	if !remoteConnected(m) {
		m.startBackendRemoteSSHSession(config, name)
		return
	}
	m.spawnTermProcess(m.Root(), m.Root(), m.term.active, "ssh", remoteOpenSSHArgs(config), name)
}

// startBackendRemoteSSHSession uses OpenIDEA's built-in client, so an SSH
// configuration can use its saved password, key and trusted host directly.
// A remote project has no local Manager transport available, so its terminal
// still opens this machine's OpenSSH with the selected configuration.
func (m *Model) startBackendRemoteSSHSession(config openidea.SSHConfiguration, name string) {
	id := strings.TrimSpace(config.ID)
	if id == "" {
		m.term.err = "Starting the SSH session failed: the SSH configuration has no id."
		m.invalidate()
		return
	}
	root := m.Root()
	m.spawnTerminalRequest(root, root, m.term.active, name, func(ctx context.Context, channel uint32) (json.RawMessage, error) {
		return m.invoke(ctx, "ssh_open_terminal", map[string]any{
			"id": id, "cols": 100, "rows": 30,
			"onOutput": map[string]any{"$channel": channel},
		})
	})
}

// remoteOpenSSHArgs matches ide-ssh's openssh_args for the terminal client.
func remoteOpenSSHArgs(config openidea.SSHConfiguration) []string {
	args := make([]string, 0, 10)
	if config.Port != nil && *config.Port > 0 {
		args = append(args, "-p", strconv.Itoa(*config.Port))
	}
	if config.Auth == "keyPair" {
		if key := strings.TrimSpace(config.KeyFile); key != "" {
			args = append(args, "-i", expandUserPath(key), "-o", "IdentitiesOnly=yes")
		}
	}
	if config.Auth == "password" {
		args = append(args, "-o", "PreferredAuthentications=password,keyboard-interactive")
	}
	host := strings.TrimSpace(config.Host)
	if user := strings.TrimSpace(config.User); user != "" {
		host = user + "@" + host
	}
	return append(args, "--", host)
}

func (m *Model) remoteSSHSessionDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	if !state.sshSessionOpen {
		return
	}
	open := state.sshSessionOpen
	dialog := nativeModal(c.Key("ssh-session-dialog"), &open, func() {
		ui.Column(c).Width(560).Padding(18).Gap(9).Children(func() {
			ui.Text(c, "Start SSH Session").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			if state.sshSessionWaiting {
				loadingState(c, p, "Loading SSH configurations…")
			} else if len(state.sshConfigs) == 0 {
				ui.Text(c, "No SSH configurations. Add one to start a terminal session.").
					FontSize(12).TextColor(p.TextMuted)
			} else {
				for _, config := range state.sshConfigs {
					config := config
					label := strings.TrimSpace(config.Name)
					if label == "" {
						label = config.Label()
					}
					if label == "" {
						label = config.ID
					}
					ui.Button(c, label).Disabled(strings.TrimSpace(config.Host) == "").
						OnClick(func() { m.startRemoteSSHSessionOn(config) })
				}
			}
			if state.sshSessionErr != "" {
				errorState(c, p, state.sshSessionErr)
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Edit SSH Configurations…").OnClick(func() {
					state.sshSessionOpen = false
					m.openRemoteSSHSettings()
				})
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() { state.sshSessionOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.sshSessionOpen = false
		state.sshSessionPendingID = ""
		state.sshSessionWaiting = false
	}
}
