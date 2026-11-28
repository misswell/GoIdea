package app

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

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
	state.sshSessionResult = ""
	state.sshSessionRemedy = ""
	state.sshSessionFingerprint = ""
	state.sshSessionSecret = ""
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
	if m.mgr == nil && m.invokeHook == nil {
		state.sshSessionOpen = true
		state.sshSessionErr = "The GoIDEA backend is not connected."
		m.invalidate()
		return
	}
	state.sshSessionOpen = true
	state.sshSessionBusy = false
	state.sshSessionPendingID = ""
	state.sshSessionWaiting = false
	state.sshSessionErr = ""
	state.sshSessionResult = ""
	state.sshSessionRemedy = ""
	state.sshSessionFingerprint = ""
	state.sshSessionConfig = config
	if !remoteConnected(m) {
		m.testRemoteSSHSession()
		return
	}
	m.prepareRemoteSSHSessionTerminal()
	name := remoteSSHConfigurationName(config)
	state.sshSessionOpen = false
	m.status = "Starting SSH session " + name + "…"
	m.spawnTermProcess(m.Root(), m.Root(), m.term.active, "ssh", remoteOpenSSHArgs(config), name)
}

func remoteSSHConfigurationName(config openidea.SSHConfiguration) string {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = config.Label()
	}
	if name == "" {
		name = config.ID
	}
	return name
}

func (m *Model) prepareRemoteSSHSessionTerminal() {
	if !m.term.visible {
		m.autoSaveOnSwitch()
		m.term.visible = true
		m.occupyBottom("term")
	}
}

func (m *Model) testRemoteSSHSession() {
	state := &m.remote
	if state.sshSessionBusy {
		return
	}
	config := state.sshSessionConfig
	if strings.TrimSpace(config.Host) == "" {
		state.sshSessionErr = "The SSH configuration has no host."
		m.invalidate()
		return
	}
	state.sshSessionRequest++
	request := state.sshSessionRequest
	generation := state.generation
	secret := state.sshSessionSecret
	name := remoteSSHConfigurationName(config)
	state.sshSessionBusy = true
	state.sshSessionErr = ""
	state.sshSessionResult = ""
	state.sshSessionRemedy = ""
	state.sshSessionFingerprint = ""
	m.status = "Connecting to SSH configuration " + name + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		message, err := m.testRemoteSSHConnection(ctx, config, secret)
		if err == nil && secret != "" {
			err = m.setRemoteSSHSecret(ctx, config.ID, secret, config.SaveSecret)
		}
		m.post(func() {
			if state.generation != generation || state.sshSessionRequest != request {
				return
			}
			state.sshSessionBusy = false
			if err != nil {
				state.sshSessionErr = "Could not start the SSH session: " + err.Error()
				state.sshSessionRemedy = remoteSSHRemedyForError(err, config.Auth)
				state.sshSessionFingerprint = sshHostKeyFingerprint(err.Error())
				if state.sshSessionRemedy == remoteSSHRemedyTrustHost &&
					state.sshSessionFingerprint == "" {
					state.sshSessionErr += " The host key fingerprint is unavailable; try again."
				}
				m.status = state.sshSessionErr
				m.invalidate()
				return
			}
			state.sshSessionResult = message
			state.sshSessionSecret = ""
			state.sshSessionOpen = false
			m.prepareRemoteSSHSessionTerminal()
			m.startBackendRemoteSSHSession(config, name)
		})
	}()
}

func (m *Model) trustRemoteSSHSessionHost() {
	state := &m.remote
	if state.sshSessionBusy || state.sshSessionRemedy != remoteSSHRemedyTrustHost {
		return
	}
	fingerprint := state.sshSessionFingerprint
	if strings.TrimSpace(fingerprint) == "" {
		state.sshSessionErr = "The SSH host key fingerprint is unavailable. Try the session again."
		m.invalidate()
		return
	}
	configID := strings.TrimSpace(state.sshSessionConfig.ID)
	if configID == "" {
		state.sshSessionErr = "The SSH configuration has no id."
		m.invalidate()
		return
	}
	state.sshSessionRequest++
	request := state.sshSessionRequest
	generation := state.generation
	state.sshSessionBusy = true
	state.sshSessionErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		err := m.trustRemoteSSHHost(ctx, configID, fingerprint)
		m.post(func() {
			if state.generation != generation || state.sshSessionRequest != request {
				return
			}
			state.sshSessionBusy = false
			if err != nil {
				state.sshSessionErr = "Could not trust the SSH host: " + err.Error()
				state.sshSessionRemedy = ""
				m.invalidate()
				return
			}
			state.sshSessionRemedy = ""
			state.sshSessionFingerprint = ""
			m.testRemoteSSHSession()
		})
	}()
}

func (m *Model) retryRemoteSSHSessionSecret() {
	state := &m.remote
	if state.sshSessionBusy || state.sshSessionRemedy != remoteSSHRemedySecret {
		return
	}
	if state.sshSessionSecret == "" {
		state.sshSessionErr = "Enter the password or key passphrase."
		m.invalidate()
		return
	}
	m.testRemoteSSHSession()
}

func (m *Model) cancelRemoteSSHSession() {
	state := &m.remote
	state.sshSessionRequest++
	state.sshSessionOpen = false
	state.sshSessionPendingID = ""
	state.sshSessionWaiting = false
	state.sshSessionBusy = false
	state.sshSessionErr = ""
	state.sshSessionResult = ""
	state.sshSessionSecret = ""
	state.sshSessionRemedy = ""
	state.sshSessionFingerprint = ""
	state.sshSessionConfig = openidea.SSHConfiguration{}
	m.status = "SSH session cancelled."
	m.invalidate()
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
			} else if state.configsErr != "" {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, state.configsErr).FontSize(12).TextColor(p.Danger).Grow(1)
					ui.Button(c, "Retry").Disabled(state.configsLoading).
						OnClick(m.retryRemoteChoices)
				})
			} else if len(state.sshConfigs) == 0 {
				emptyState(c, p, "No SSH configurations")
				ui.Text(c, "Add one to start a terminal session.").
					FontSize(12).TextColor(p.TextMuted)
			} else {
				for _, config := range state.sshConfigs {
					config := config
					ui.Button(c, remoteSSHConfigurationName(config)).
						Disabled(state.sshSessionBusy || strings.TrimSpace(config.Host) == "").
						OnClick(func() { m.startRemoteSSHSessionOn(config) })
				}
			}
			if state.sshSessionBusy {
				loadingState(c, p, "Connecting to the SSH host…")
			}
			if state.sshSessionErr != "" {
				errorState(c, p, state.sshSessionErr)
				switch state.sshSessionRemedy {
				case remoteSSHRemedyTrustHost:
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "The authenticity of the host cannot be established.").
							FontSize(11).TextColor(p.TextMuted).Grow(1)
						ui.Button(c, "Trust Host and Retry").
							Disabled(state.sshSessionBusy).
							OnClick(m.trustRemoteSSHSessionHost)
					})
				case remoteSSHRemedySecret:
					label := "Password"
					if state.sshSessionConfig.Auth != "password" {
						label = "Key passphrase"
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, label).FontSize(11).TextColor(p.TextMuted).Width(100)
						ui.TextInput(c, &state.sshSessionSecret).Password().Height(28).Grow(1)
						ui.Button(c, "Connect").Disabled(state.sshSessionBusy).
							OnClick(m.retryRemoteSSHSessionSecret)
					})
				}
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Edit SSH Configurations…").Disabled(state.sshSessionBusy).OnClick(func() {
					m.cancelRemoteSSHSession()
					state.sshSessionOpen = false
					m.openRemoteSSHSettings()
				})
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(m.cancelRemoteSSHSession)
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.cancelRemoteSSHSession()
	}
}
