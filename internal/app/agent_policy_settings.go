package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// settingsAgentPolicyBody is Settings › Agents › Command Permissions. It edits
// the same agentSecurity.allowedCommands and blockedCommands arrays OpenIDEA
// stores, with the default blocked patterns and wildcard syntax left intact.
func (m *Model) settingsAgentPolicyBody(c *ui.Context, oi oiColors) {
	policy := &m.settingsDraft.security
	ui.Text(c, "Allowed commands run without asking, with or without extra arguments. "+
		"Blocked commands are always rejected, even if allowed, and support * and ? wildcards. "+
		"Commands with substitutions, redirections or variable assignments always ask.").
		FontSize(12).TextColor(oi.textMuted)

	ui.Row(c).Grow(1).MinHeight(0).Gap(18).Children(func() {
		m.agentPolicyColumn(c, oi, "Allowed Commands", "e.g. cargo test",
			&m.settingsDraft.securityAllowDraft, &policy.AllowedCommands,
			"Add allowed command", func(value string) []string {
				return append(policy.AllowedCommands, value)
			})
		m.agentPolicyColumn(c, oi, "Blocked Commands", "e.g. rm -rf *",
			&m.settingsDraft.securityBlockDraft, &policy.BlockedCommands,
			"Add blocked command", func(value string) []string {
				return append(policy.BlockedCommands, value)
			})
	})
}

func (m *Model) agentPolicyColumn(c *ui.Context, oi oiColors, title, placeholder string,
	draft *string, patterns *[]string, addLabel string, add func(string) []string) {
	ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(13).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Scroll(c).Grow(1).MinHeight(150).MaxHeight(420).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				if len(*patterns) == 0 {
					ui.Text(c, "No command patterns").FontSize(11).TextColor(oi.textMuted).
						SingleLine()
				}
				for index, pattern := range *patterns {
					index, pattern := index, pattern
					row := ui.Row(c.Key("agent-policy:"+title+":"+pattern)).MinHeight(28).
						Padding(1, 5).Gap(6).Radius(4).AlignItems(ui.Center)
					if row.Hovered() {
						row.Background(oi.hoverTint)
					}
					row.Children(func() {
						ui.Text(c, pattern).Grow(1).MinWidth(0).FontSize(12).
							TextColor(oi.text).SingleLine().Ellipsis("…").Tooltip(pattern)
						ui.Button(c, "×").Tooltip("Remove " + pattern).OnClick(func() {
							next := append([]string(nil), (*patterns)...)
							next = append(next[:index], next[index+1:]...)
							*patterns = uniqueAgentCommandPatterns(next)
							m.invalidate()
						})
					})
				}
			})
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("agent-policy-add:"+title), draft).Grow(1).
				Placeholder(placeholder)
			submit := in.Submitted()
			if ui.Button(c, "Add").Label(addLabel).Clicked() || submit {
				value := strings.TrimSpace(*draft)
				if value != "" && !slicesContain(*patterns, value) {
					*patterns = uniqueAgentCommandPatterns(add(value))
				}
				*draft = ""
				m.invalidate()
			}
		})
	})
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
