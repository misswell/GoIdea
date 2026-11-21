package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (m *Model) agentRegistryInstallConfirmDialog(c *ui.Context, p *ui.Theme) {
	st := &m.agents.registry
	if st.InstallConfirmID == "" {
		return
	}
	agent := m.registryAgentByID(st.InstallConfirmID)
	if agent == nil {
		st.InstallConfirmID = ""
		return
	}
	open := true
	dialog := ui.Modal(c.Key("agent-registry-install-confirm"), &open, func() {
		ui.Column(c).Width(500).Gap(9).Children(func() {
			ui.Text(c, "Install "+agent.Name).FontSize(14).Bold().TextColor(p.Text)
			ui.Text(c, agent.Name+" provides no checksum for its download, so the downloaded file cannot be verified. Install it anyway?").
				FontSize(12).TextColor(p.Text)
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Install").OnClick(m.confirmRegistryInstall)
				ui.Button(c, "Cancel").OnClick(func() {
					st.InstallConfirmID = ""
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		st.InstallConfirmID = ""
	}
}

func (m *Model) agentRegistryRemoveConfirmDialog(c *ui.Context, p *ui.Theme) {
	st := &m.agents.registry
	if !st.RemoveOpen {
		return
	}
	name := st.RemoveID
	if provider := agentProviderByID(m.agents.providers, st.RemoveID); provider != nil &&
		provider.Name != "" {
		name = provider.Name
	} else if registry := m.registryAgentByID(st.RemoveID); registry != nil &&
		registry.Name != "" {
		name = registry.Name
	}
	if name == "" {
		st.RemoveOpen = false
		st.RemoveID = ""
		st.RemoveCustom = false
		return
	}
	open := st.RemoveOpen
	dialog := ui.Modal(c.Key("agent-registry-remove-confirm"), &open, func() {
		ui.Column(c).Width(500).Gap(9).Children(func() {
			ui.Text(c, "Remove "+name).FontSize(14).Bold().TextColor(p.Text)
			message := "Remove " + name + " and delete its agent file? Its tasks stay, but can't continue without it."
			if !st.RemoveCustom {
				message = "Remove " + name + " from this computer? Its tasks stay, but can't continue until it is installed again."
			}
			ui.Text(c, message).FontSize(12).TextColor(p.Text)
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Remove").OnClick(m.confirmRemoveAgent)
				ui.Button(c, "Cancel").OnClick(func() {
					st.RemoveOpen = false
					st.RemoveID = ""
					st.RemoveCustom = false
				})
			})
		})
	})
	if dialog.Dismissed() || !open {
		st.RemoveOpen = false
		st.RemoveID = ""
		st.RemoveCustom = false
	}
}

func (m *Model) agentCustomDialog(c *ui.Context, p *ui.Theme) {
	st := &m.agents.registry
	if !st.CustomOpen {
		return
	}
	open := st.CustomOpen
	dialog := ui.Modal(c.Key("agent-custom-dialog"), &open, func() {
		ui.Column(c).Width(560).Gap(9).Children(func() {
			ui.Text(c, "Add Custom Agent").FontSize(14).Bold().TextColor(p.Text)
			if st.CustomErr != "" {
				ui.Text(c, st.CustomErr).FontSize(11).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Name").Width(112).FontSize(11).TextColor(p.TextMuted)
				input := ui.TextInput(c.Key("agent-custom-name"), &st.CustomName).
					Grow(1).Placeholder("My agent").Disabled(st.CustomBusy)
				if input.Changed() {
					m.updateCustomAgentName(st.CustomName)
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "ID").Width(112).FontSize(11).TextColor(p.TextMuted)
				input := ui.TextInput(c.Key("agent-custom-id"), &st.CustomID).
					Grow(1).Font(oiFontMono).Placeholder("my-agent").Disabled(st.CustomBusy)
				if input.Changed() {
					st.CustomIDTouched = true
				}
			})
			ui.Text(c, "Letters, digits, dashes and underscores only; names the agent file.").
				FontSize(10).TextColor(p.TextMuted)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Command").Width(112).FontSize(11).TextColor(p.TextMuted)
				ui.TextInput(c.Key("agent-custom-command"), &st.CustomCommand).
					Grow(1).Font(oiFontMono).Placeholder("/path/to/agent-acp").
					Disabled(st.CustomBusy)
				ui.Button(c, "Browse…").Disabled(st.CustomBusy).
					OnClick(m.chooseCustomAgentExecutable)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Arguments").Width(112).FontSize(11).TextColor(p.TextMuted)
				ui.TextInput(c.Key("agent-custom-args"), &st.CustomArgs).
					Grow(1).Font(oiFontMono).Placeholder("--acp").Disabled(st.CustomBusy)
			})
			ui.Text(c, "Separate arguments with spaces and quote arguments containing spaces.").
				FontSize(10).TextColor(p.TextMuted)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Environment").Width(112).FontSize(11).TextColor(p.TextMuted)
				ui.TextInput(c.Key("agent-custom-env"), &st.CustomEnv).
					Grow(1).Font(oiFontMono).Placeholder("NAME=value; OTHER=value").
					Disabled(st.CustomBusy)
			})
			ui.Row(c).Gap(8).Children(func() {
				canAdd := strings.TrimSpace(st.CustomName) != "" &&
					agentIDPattern.MatchString(strings.TrimSpace(st.CustomID)) &&
					strings.TrimSpace(st.CustomCommand) != ""
				ui.Button(c, "Add").Disabled(st.CustomBusy || !canAdd).
					OnClick(m.submitCustomAgent)
				ui.Button(c, "Cancel").Disabled(st.CustomBusy).OnClick(func() {
					st.CustomOpen = false
				})
				if st.CustomBusy {
					ui.Text(c, "Adding…").FontSize(11).TextColor(p.TextMuted)
				}
			})
		})
	})
	if dialog.Dismissed() || !open {
		st.CustomOpen = false
	}
}
