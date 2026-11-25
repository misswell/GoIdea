package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func graphqlFormatEnvironment(names []string, values map[string]string) string {
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, name+"="+values[name])
	}
	return strings.Join(lines, "\n")
}

func graphqlParseEnvironment(text string, names []string) map[string]string {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		at := strings.IndexByte(line, '=')
		if at <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:at])
		if allowed[name] {
			out[name] = strings.TrimSpace(line[at+1:])
		}
	}
	return out
}

func (m *Model) graphqlIntrospectionDialog(c *ui.Context, p *ui.Theme) {
	confirmation := m.graphql.introspectionAsk
	if confirmation == nil {
		return
	}
	open := true
	oi := oiPaletteOf(p)
	nativeModal(c.Key("graphql:introspection-confirmation"), &open, func() {
		ui.Column(c).Width(520).Padding(16).Gap(12).Children(func() {
			ui.Text(c, "Get GraphQL Schema from Endpoint").FontSize(15).Bold().TextColor(oi.text)
			ui.Text(c, fmt.Sprintf(
				"Replace the contents of %s with the schema of %s?",
				filepath.Base(confirmation.Target), confirmation.Endpoint.Name,
			)).FontSize(12).TextColor(oi.textMuted)
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					open = false
					m.cancelGraphQLIntrospection()
				})
				ui.Button(c, "Replace").OnClick(func() {
					open = false
					m.confirmGraphQLIntrospection()
				})
			})
		})
	})
	if !open && m.graphql.introspectionAsk == confirmation {
		m.cancelGraphQLIntrospection()
	}
}

func (m *Model) openGraphQLErrorDialog(title, message string) {
	m.graphql.errorDialog = &graphqlErrorDialog{
		Title: title, Message: message,
	}
	m.invalidate()
}

func (m *Model) graphqlErrorDialog(c *ui.Context, p *ui.Theme) {
	dialog := m.graphql.errorDialog
	if dialog == nil {
		return
	}
	open := true
	oi := oiPaletteOf(p)
	nativeModal(c.Key("graphql:error-dialog"), &open, func() {
		ui.Column(c).Width(580).Padding(18).Gap(12).Children(func() {
			ui.Text(c, dialog.Title).FontSize(15).Bold().TextColor(oi.text)
			ui.Scroll(c).MaxHeight(280).Children(func() {
				ui.Text(c, dialog.Message).FontSize(12).TextColor(oi.text).
					Role(ui.RoleText)
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				ui.Button(c, "OK").OnClick(func() {
					open = false
					m.graphql.errorDialog = nil
				})
			})
		})
	})
	if !open && m.graphql.errorDialog == dialog {
		m.graphql.errorDialog = nil
	}
}

func (m *Model) graphqlEnvironmentDialog(c *ui.Context, p *ui.Theme) {
	dialog := m.graphql.environmentDialog
	if dialog == nil {
		return
	}
	open := true
	oi := oiPaletteOf(p)
	nativeModal(c.Key("graphql:environment-dialog"), &open, func() {
		ui.Column(c).Width(520).Padding(16).Gap(12).Children(func() {
			ui.Text(c, "GraphQL Environment Variables").FontSize(15).Bold().TextColor(oi.text)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Copy").OnClick(func() {
					mygo.Clipboard.WriteText(graphqlFormatEnvironment(dialog.Variables, dialog.Values))
				})
				ui.Button(c, "Paste").OnClick(func() {
					for name, value := range graphqlParseEnvironment(
						mygo.Clipboard.ReadText(), dialog.Variables,
					) {
						dialog.Values[name] = value
					}
					m.invalidate()
				})
			})
			ui.Row(c).Height(24).Padding(4, 8).Gap(8).AlignItems(ui.Center).
				Background(oi.selected).Children(func() {
				ui.Text(c, "Name").FontSize(11).Bold().TextColor(oi.textMuted).Grow(1)
				ui.Text(c, "Value").FontSize(11).Bold().TextColor(oi.textMuted).Grow(2)
			})
			ui.Column(c).Gap(4).Children(func() {
				for _, name := range dialog.Variables {
					name := name
					value := dialog.Values[name]
					ui.Row(c.Key("graphql:environment:" + name)).Height(30).Gap(8).
						AlignItems(ui.Center).Children(func() {
						ui.Text(c, name).FontSize(12).TextColor(oi.text).Grow(1).
							SingleLine().Ellipsis("…")
						input := ui.TextInput(c.Key("graphql:environment-value:"+name), &value).
							Grow(2).Height(28).Placeholder("Value")
						if input.Submitted() {
							dialog.Values[name] = value
							m.closeGraphQLEnvironment(true)
						}
					})
					dialog.Values[name] = value
				}
			})
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					open = false
					m.closeGraphQLEnvironment(false)
				})
				ui.Button(c, "OK").OnClick(func() {
					open = false
					m.closeGraphQLEnvironment(true)
				})
			})
		})
	})
	if !open && m.graphql.environmentDialog == dialog {
		m.closeGraphQLEnvironment(false)
	}
}
