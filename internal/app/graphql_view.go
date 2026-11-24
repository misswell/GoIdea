package app

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

const graphqlHelpURL = "https://the-guild.dev/graphql/config/docs/user/usage"

func (m *Model) graphqlPanel(c *ui.Context, p *ui.Theme) {
	m.ensureGraphQL()
	if !m.graphql.discovered && !m.graphql.discovering {
		m.ensureGraphQLDiscovered(nil)
	}
	oi := oiPaletteOf(p)
	ui.Column(c.Key("graphql:panel")).Grow(1).MinWidth(0).MinHeight(0).
		Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		m.graphqlTabs(c, p)
		switch m.graphql.tab {
		case "result":
			m.graphqlResultPanel(c, p, oi)
		default:
			m.graphqlSchemasPanel(c, p, oi)
		}
	})
}

func (m *Model) graphqlTabs(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c.Key("graphql:tabs")).Height(28).Shrink(0).Padding(0, 6).Gap(2).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			m.graphqlTab(c, p, "schemas", "Schemas and Project Structure")
			m.graphqlTab(c, p, "result", "Query Result")
		})
}

func (m *Model) graphqlTab(c *ui.Context, p *ui.Theme, id, label string) {
	oi := oiPaletteOf(p)
	selected := m.graphql.tab == id
	tab := ui.Row(c.Key("graphql:tab:"+id)).Height(24).Padding(0, 10).AlignItems(ui.Center).
		Radius(5).Cursor(ui.CursorPointer)
	if selected {
		tab.Background(oi.selected)
	} else if tab.Hovered() {
		tab.Background(oi.hoverTint)
	}
	tab.OnClick(func() {
		m.graphql.tab = id
		m.invalidate()
	})
	tab.Children(func() {
		color := oi.textMuted
		if selected {
			color = oi.text
		}
		ui.Text(c, label).FontSize(12).TextColor(color).SingleLine()
	})
}

func (m *Model) graphqlSchemasPanel(c *ui.Context, p *ui.Theme, oi oiColors) {
	m.graphqlToolbar(c, p)
	if m.graphql.discoveryErr != "" {
		ui.Row(c.Key("graphql:discovery-error")).Shrink(0).Padding(6, 10).Gap(8).
			AlignItems(ui.Center).Background(p.Danger.Mix(p.Surface, 0.9)).Children(func() {
			oiIconEl(c, "status/error", 16, oi, p.Danger)
			ui.Text(c, m.graphql.discoveryErr).FontSize(12).TextColor(p.Danger).
				SingleLine().Ellipsis("…").Grow(1)
			ui.Button(c, "Retry").OnClick(m.RestartGraphQLDiscovery)
		})
	}
	if !m.graphql.discovered && m.graphql.discovering {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(7).Children(func() {
			ui.Text(c, "Discovering GraphQL schemas…").FontSize(13).TextColor(oi.textMuted)
		})
		return
	}
	if len(m.graphql.projects) == 0 {
		m.graphqlEmptyState(c, p, oi)
		return
	}
	roots := graphqlProjectNodes(m.graphql.projects)
	ui.Scroll(c.Key("graphql:tree")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(4, 4).Gap(1).Children(func() {
			for _, root := range roots {
				m.graphqlTreeRow(c, p, oi, root, 0)
			}
		})
	})
}

func (m *Model) graphqlEmptyState(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Padding(24).Gap(8).
		Children(func() {
			oiIconEl(c, "toolwindows/graphql", 32, oi, oi.textMuted)
			ui.Text(c, "No GraphQL schema").FontSize(13).Bold().TextColor(oi.text)
			ui.Text(c, "Add graphql-config or create GraphQL schema files in the project.").
				FontSize(12).TextColor(oi.textMuted)
			ui.Button(c, "Add Schema Configuration").OnClick(m.AddGraphQLConfiguration)
		})
}

func (m *Model) graphqlToolbar(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	configAvailable := m.graphqlConfigurationFor("") != ""
	ui.Row(c.Key("graphql:toolbar")).Height(32).Shrink(0).Padding(0, 6).Gap(3).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			m.httpToolbarButton(c, p, "general/add", "Add Schema Configuration",
				false, m.AddGraphQLConfiguration)
			graphqlToolbarSeparator(c, oi)
			m.httpToolbarButton(c, p, "general/settings", "Open Configuration File",
				!configAvailable, m.OpenGraphQLConfiguration)
			m.httpToolbarButton(c, p, "general/refresh", "Restart Schema Discovery",
				m.graphql.discovering, m.RestartGraphQLDiscovery)
			m.httpToolbarButton(c, p, "general/help", "Help",
				false, func() { c.OpenURL(graphqlHelpURL) })
			graphqlToolbarSeparator(c, oi)
			m.httpToolbarButton(c, p, "general/expandAll", "Expand All",
				len(m.graphql.projects) == 0, m.ExpandAllGraphQL)
			m.httpToolbarButton(c, p, "general/collapseAll", "Collapse All",
				len(m.graphql.projects) == 0, m.CollapseAllGraphQL)
		})
}

func graphqlToolbarSeparator(c *ui.Context, oi oiColors) {
	ui.Column(c).Width(1).Height(16).Shrink(0).Background(oi.border)
}

func (m *Model) ExpandAllGraphQL() {
	m.ensureGraphQL()
	m.graphql.expanded = map[string]bool{}
	for _, project := range m.graphql.projects {
		root := graphqlSchemaNode{Kind: "project", Key: project.Key, Project: &project}
		m.expandGraphQLNode(root)
	}
	m.invalidate()
}

func (m *Model) expandGraphQLNode(node graphqlSchemaNode) {
	children := graphqlChildNodes(node, m.graphqlDiagnosticsFor)
	if len(children) == 0 {
		return
	}
	m.graphql.expanded[node.Key] = true
	for _, child := range children {
		m.expandGraphQLNode(child)
	}
}

func (m *Model) CollapseAllGraphQL() {
	m.ensureGraphQL()
	m.graphql.expanded = map[string]bool{}
	for _, project := range m.graphql.projects {
		m.graphql.expanded[project.Key] = false
		m.graphql.expanded[project.Key+"#errors"] = false
		m.graphql.expanded[project.Key+"#endpoints"] = false
	}
	m.invalidate()
}

func graphqlNodeExpandedByDefault(kind string) bool {
	return kind == "project" || kind == "errors" || kind == "endpoints"
}

func (m *Model) graphqlNodeExpanded(node graphqlSchemaNode) bool {
	if expanded, ok := m.graphql.expanded[node.Key]; ok {
		return expanded
	}
	return graphqlNodeExpandedByDefault(node.Kind)
}

func (m *Model) graphqlTreeRow(c *ui.Context, p *ui.Theme, oi oiColors, node graphqlSchemaNode, depth int) {
	children := graphqlChildNodes(node, m.graphqlDiagnosticsFor)
	expandable := len(children) > 0
	expanded := expandable && m.graphqlNodeExpanded(node)
	label := graphqlNodeLabel(node)
	row := ui.Row(c.Key("graphql:node:" + node.Key)).Height(24).
		PaddingStart(float32(4 + depth*16)).PaddingEnd(8).Gap(5).Radius(5).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Role(ui.RoleTreeItem).Focusable().Label(label).Expanded(expanded)
	if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Children(func() {
		if expandable {
			arrow := "›"
			if expanded {
				arrow = "⌄"
			}
			ui.Text(c, arrow).FontSize(12).TextColor(oi.textMuted).Width(12)
		} else {
			ui.Spacer(c).Width(12)
		}
		switch node.Kind {
		case "file":
			fileIconEl(c, oi, node.File.Path, 16, oi.textMuted)
		default:
			oiIconEl(c, graphqlNodeIcon(node), 16, oi, oi.textMuted)
		}
		ui.Text(c, label).FontSize(12).TextColor(oi.text).
			SingleLine().Ellipsis("…")
		switch node.Kind {
		case "project":
			ui.Spacer(c)
			ui.Text(c, node.Project.Root).FontSize(11).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		case "summary":
			ui.Text(c, "- "+graphqlStatsSummary(*node.Project)).FontSize(11).
				TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		case "file":
			relative, err := filepath.Rel(node.Project.Root, node.File.Path)
			if err == nil && relative != "." && relative != filepath.Base(node.File.Path) {
				ui.Text(c, filepath.Dir(relative)).FontSize(11).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…")
			}
		case "error":
			if node.Diagnostic != nil {
				ui.Spacer(c)
				ui.Text(c, fmt.Sprintf(
					"%s:%d", filepath.Base(node.Path), node.Diagnostic.Range.Start.Line+1,
				)).FontSize(11).TextColor(oi.textMuted).SingleLine()
			}
		case "errors":
			count := len(graphqlSchemaProblems(*node.Project, m.graphqlDiagnosticsFor))
			if count > 0 {
				ui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(oi.textMuted)
			}
		case "endpoint":
			ui.Spacer(c)
			ui.Text(c, node.Endpoint.URL).FontSize(11).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		}
	})
	if expandable && row.Clicked() {
		m.graphql.expanded[node.Key] = !expanded
		m.invalidate()
	}
	if expandable && row.DoubleClicked() {
		m.activateGraphQLNode(node)
	}
	if !expandable && row.Clicked() {
		m.activateGraphQLNode(node)
	}
	row.OnShortcut(ui.Modifiers(0), ui.KeyEnter, func() {
		if expandable {
			m.graphql.expanded[node.Key] = !expanded
			m.invalidate()
		} else {
			m.activateGraphQLNode(node)
		}
	})
	if expanded {
		for _, child := range children {
			m.graphqlTreeRow(c, p, oi, child, depth+1)
		}
	}
}

func (m *Model) activateGraphQLNode(node graphqlSchemaNode) {
	switch node.Kind {
	case "project":
		if node.Project.ConfigPath != "" {
			m.openFile(node.Project.ConfigPath)
		}
	case "file":
		m.openFile(node.File.Path)
	case "error":
		if node.Diagnostic != nil {
			m.openFile(node.Path)
			m.revealAt(
				node.Path,
				int(node.Diagnostic.Range.Start.Line),
				int(node.Diagnostic.Range.Start.Col),
			)
		}
	}
}

func (m *Model) graphqlResultPanel(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.graphql.running != nil {
		ui.Row(c.Key("graphql:result-toolbar")).Height(32).Shrink(0).Padding(0, 8).
			Gap(7).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).
			BorderColor(oi.border).Children(func() {
			ui.Text(c, "Running GraphQL query…").FontSize(12).TextColor(oi.textMuted).Grow(1)
			ui.Button(c, "Stop").OnClick(m.cancelGraphQLRun)
		})
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "Waiting for the response").FontSize(12).TextColor(oi.textMuted)
		})
		return
	}
	result := m.graphql.result
	if result == nil {
		ui.Column(c.Key("graphql:no-result")).Grow(1).AlignItems(ui.Center).
			Justify(ui.Center).Gap(8).Children(func() {
			oiIconEl(c, "toolwindows/graphql", 32, oi, oi.textMuted)
			ui.Text(c, "No query results").FontSize(13).Bold().TextColor(oi.text)
			ui.Text(c, "Run a query with Execute GraphQL in the toolbar of a GraphQL file.").
				FontSize(12).TextColor(oi.textMuted)
		})
		return
	}
	errors := graphqlResponseErrorCount(result.Body)
	failed := result.Error != "" || errors > 0 || result.HasStatus && result.Status >= 400
	status := "No response"
	if result.Error != "" {
		status = filepath.Base(result.Path) + ": " + result.Error
	} else if result.HasStatus {
		status = fmt.Sprintf(
			"%s: %.0f ms execution time, %s response",
			filepath.Base(result.Path), result.TotalMs, graphqlFormatSize(result.Size),
		)
		if errors == 1 {
			status += ", 1 error"
		} else if errors > 1 {
			status += fmt.Sprintf(", %d errors", errors)
		}
	}
	ui.Row(c.Key("graphql:result-status")).Height(32).Shrink(0).Padding(0, 8).
		Gap(7).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).
		BorderColor(oi.border).Children(func() {
		icon, color := "status/check", oi.success
		if failed {
			icon, color = "status/error", oi.danger
		}
		oiIconEl(c, icon, 16, oi, color)
		ui.Text(c, status).FontSize(12).TextColor(color).SingleLine().
			Ellipsis("…").Grow(1).Tooltip(result.Endpoint.Name + " - " + result.Endpoint.URL)
	})
	text := result.Body
	if result.Error != "" {
		text = result.Error
	} else {
		text = graphqlFormatBody(result.Body)
	}
	ui.Scroll(c.Key("graphql:result-body")).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			lines := strings.Count(text, "\n") + 1
			ui.Column(c).Width(44).Shrink(0).Padding(8, 5).Background(oi.border.Mix(p.Background, 0.55)).
				Children(func() {
					var numbers strings.Builder
					for line := 1; line <= lines; line++ {
						if line > 1 {
							numbers.WriteByte('\n')
						}
						numbers.WriteString(strconv.Itoa(line))
					}
					ui.Text(c, numbers.String()).FontSize(11).TextColor(oi.textMuted)
				})
			ui.Column(c).Grow(1).Padding(8, 8).Children(func() {
				if text == "" {
					text = " "
				}
				ui.Text(c, text).FontSize(12).TextColor(oi.text)
			})
		})
	})
}
