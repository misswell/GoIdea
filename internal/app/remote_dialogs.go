package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (m *Model) remoteSecretDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	if !state.secretOpen {
		return
	}
	open := state.secretOpen
	dialog := nativeModal(c.Key("remote-secret-dialog"), &open, func() {
		ui.Column(c).Width(520).Padding(18).Gap(10).Children(func() {
			ui.Text(c, state.secretTitle).FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "Connect to "+state.pendingSSH.Label()).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, state.secretLabel).FontSize(12).TextColor(p.Text)
			input := ui.TextInput(c, &state.secretValue).Height(28).Password().AutoFocus()
			input.OnSubmit(m.submitRemoteSecret)
			ui.Checkbox(c, &state.secretSave, "Save in Keychain")
			ui.Text(c, "A saved password or key passphrase is kept in macOS Keychain, never in settings.").
				FontSize(11).TextColor(p.TextMuted)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Connect").Disabled(strings.TrimSpace(state.secretValue) == "").
					OnClick(m.submitRemoteSecret)
				ui.Button(c, "Cancel").OnClick(func() { state.secretOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.secretOpen = false
		state.secretValue = ""
	}
}

func (m *Model) remoteTrustDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	if !state.trustOpen {
		return
	}
	open := state.trustOpen
	dialog := nativeModal(c.Key("remote-trust-dialog"), &open, func() {
		ui.Column(c).Width(600).Padding(18).Gap(10).Children(func() {
			ui.Text(c, "Unknown SSH Host Key").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "Host: "+state.trustHost).FontSize(12).TextColor(p.TextMuted).SingleLine()
			ui.Text(c, "Fingerprint: "+state.trustFingerprint).FontSize(12).
				TextColor(p.Text).SingleLine().Ellipsis("…")
			ui.Text(c, "Compare this fingerprint with the host's administrator or another trusted channel before connecting.").
				FontSize(12).TextColor(p.TextMuted)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Trust and Connect").OnClick(m.submitRemoteHostTrust)
				ui.Button(c, "Cancel").OnClick(func() { state.trustOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.trustOpen = false
	}
}

func (m *Model) remoteSSHSettingsDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	if !state.sshSettingsOpen {
		return
	}
	open := state.sshSettingsOpen
	dialog := nativeModal(c.Key("remote-ssh-settings-dialog"), &open, func() {
		ui.Column(c).Width(760).Height(540).Padding(18).Gap(10).Children(func() {
			ui.Text(c, "SSH Configurations").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "Connections are made from this machine. OpenSSH config can use ~/.ssh/config and its agent.").
				FontSize(12).TextColor(p.TextMuted)
			if state.sshSettingsPending {
				loadingState(c, p, "Loading SSH configurations…")
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Spacer(c)
					ui.Button(c, "Cancel").OnClick(func() {
						state.sshSettingsPending = false
						state.sshSettingsOpen = false
					})
				})
				return
			}
			if state.configsErr != "" {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, state.configsErr).FontSize(11).TextColor(p.Danger).Grow(1)
					ui.Button(c, "Retry").Disabled(state.configsLoading).
						OnClick(m.retryRemoteChoices)
				})
			}
			ui.Row(c).Grow(1).Gap(14).Children(func() {
				ui.Column(c).Width(230).Gap(4).Children(func() {
					if len(state.sshConfigs) == 0 {
						emptyState(c, p, "No SSH configurations")
					}
					for i, config := range state.sshConfigs {
						i, config := i, config
						label := strings.TrimSpace(config.Name)
						if label == "" {
							label = config.Label()
						}
						ui.Button(c, label).OnClick(func() { m.selectRemoteSSHConfig(i) })
					}
					ui.Spacer(c)
					ui.Button(c, "Add").Disabled(state.sshSettingsBusy || state.sshSettingsTesting).
						OnClick(m.addRemoteSSHConfiguration)
				})
				ui.Column(c).Grow(1).Gap(8).Children(func() {
					ui.Text(c, "Configuration").FontSize(12).Bold().TextColor(p.Text).SingleLine()
					remoteSettingsRow(c, p, "Name", func() {
						ui.TextInput(c, &state.sshName).Height(28).Grow(1)
					})
					remoteSettingsRow(c, p, "Host", func() {
						ui.TextInput(c, &state.sshHost).Height(28).Grow(1)
					})
					remoteSettingsRow(c, p, "Port", func() {
						ui.TextInput(c, &state.sshPort).Height(28).Width(120).
							Placeholder("22")
					})
					remoteSettingsRow(c, p, "User", func() {
						ui.TextInput(c, &state.sshUser).Height(28).Grow(1)
					})
					remoteSettingsRow(c, p, "Authentication", func() {
						labels := []string{"OpenSSH config", "Password", "Key pair"}
						selected := "OpenSSH config"
						switch state.sshAuth {
						case "password":
							selected = "Password"
						case "keyPair":
							selected = "Key pair"
						}
						if choice := ui.Select(c, &selected, labels).Grow(1); choice.Changed() {
							switch selected {
							case "Password":
								state.sshAuth = "password"
							case "Key pair":
								state.sshAuth = "keyPair"
							default:
								state.sshAuth = "openSsh"
							}
							m.resetRemoteSSHTest(false)
						}
					})
					if state.sshAuth == "keyPair" {
						remoteSettingsRow(c, p, "Private key", func() {
							ui.TextInput(c, &state.sshKeyFile).Height(28).Grow(1).
								Placeholder("~/.ssh/id_ed25519")
						})
					}
					if state.sshAuth != "openSsh" {
						secretLabel := "Password"
						saveLabel := "Save password"
						if state.sshAuth == "keyPair" {
							secretLabel = "Passphrase"
							saveLabel = "Save passphrase"
						}
						remoteSettingsRow(c, p, secretLabel, func() {
							ui.TextInput(c, &state.sshSecret).Password().Height(28).Grow(1).
								Placeholder("Leave empty to use a saved credential")
						})
						ui.Checkbox(c, &state.sshSaveSecret, saveLabel)
					} else {
						ui.Text(c, "Uses HostName, User, Port, IdentityFile and ProxyJump from ~/.ssh/config, and keys of the SSH agent.").
							FontSize(11).TextColor(p.TextMuted)
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Button(c, "Test Connection").
							Disabled(state.sshSettingsBusy || state.sshSettingsTesting ||
								strings.TrimSpace(state.sshHost) == "").
							OnClick(m.testRemoteSSHConfiguration)
						if state.sshSettingsTesting {
							loadingState(c, p, "Connecting…")
						} else if state.sshTestResult != "" && m.remoteSSHTestMatchesForm() {
							ui.Text(c, state.sshTestResult).FontSize(11).TextColor(p.Success).Grow(1)
						}
					})
					if state.sshTestErr != "" && m.remoteSSHTestMatchesForm() {
						errorState(c, p, state.sshTestErr)
						switch state.sshTestRemedy {
						case remoteSSHRemedyTrustHost:
							ui.Row(c).AlignItems(ui.Center).Children(func() {
								ui.Text(c, "The authenticity of the host cannot be established.").
									FontSize(11).TextColor(p.TextMuted).Grow(1)
								ui.Button(c, "Trust Host and Retry").
									Disabled(state.sshSettingsTesting).
									OnClick(m.trustRemoteSSHConfigurationHost)
							})
						case remoteSSHRemedySecret:
							ui.Button(c, "Connect").Disabled(state.sshSettingsTesting).
								OnClick(m.retryRemoteSSHConfigurationSecret)
						}
					}
					if state.sshSettingsErr != "" {
						errorState(c, p, state.sshSettingsErr)
					}
				})
			})
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				if !state.sshEditing && state.sshSelected >= 0 {
					ui.Button(c, "Delete").Disabled(state.sshSettingsBusy || state.sshSettingsTesting).
						OnClick(m.deleteRemoteSSHConfiguration)
				}
				ui.Spacer(c)
				ui.Button(c, "Cancel").Disabled(state.sshSettingsBusy || state.sshSettingsTesting).
					OnClick(func() {
						m.resetRemoteSSHTest(true)
						state.sshSettingsPending = false
						state.sshSettingsOpen = false
					})
				ui.Button(c, "Save").Disabled(state.sshSettingsBusy || state.sshSettingsTesting).
					OnClick(m.saveRemoteSSHConfiguration)
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.resetRemoteSSHTest(true)
		state.sshSettingsPending = false
		state.sshSettingsOpen = false
	}
}

func remoteSettingsRow(c *ui.Context, p *ui.Theme, label string, body func()) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.Text).Width(110)
		body()
	})
}

func remoteSSHConfigureSummary(state *remoteDevelopmentState) string {
	config := selectedRemoteSSHConfiguration(state)
	if config == nil {
		return "No SSH configurations"
	}
	return fmt.Sprintf("%s · %s", config.Name, config.Label())
}
