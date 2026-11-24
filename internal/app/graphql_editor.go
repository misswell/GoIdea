package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const graphqlValidationDelay = 300 * time.Millisecond

func (m *Model) markGraphQLSent(tab *Tab, ranges []graphqlRange) {
	if tab == nil || tab.doc == nil || tab.state == nil {
		return
	}
	m.ensureGraphQL()
	path := graphqlSlash(tab.doc.Path())
	m.graphql.sentByPath[path] = graphqlSentContext{
		Version: tab.doc.Buffer().Version(),
		Ranges:  append([]graphqlRange(nil), ranges...),
	}
	theme := m.theme
	if theme == nil {
		theme = openideaTheme(false)
	}
	tab.state.TextBackgrounds = m.composeTextBackgrounds(tab, m.todoMarksOf(tab, theme))
	m.invalidate()
}

func (m *Model) graphqlUnsentTextBackgrounds(tab *Tab) []editor.TextBackground {
	if tab == nil || tab.doc == nil {
		return nil
	}
	m.ensureGraphQL()
	sent, ok := m.graphql.sentByPath[graphqlSlash(tab.doc.Path())]
	if !ok || sent.Version != tab.doc.Buffer().Version() {
		return nil
	}
	outside := graphqlOutsideRanges(len(tab.doc.Text()), sent.Ranges)
	if len(outside) == 0 {
		return nil
	}
	theme := m.theme
	if theme == nil {
		theme = openideaTheme(false)
	}
	color := oiPaletteOf(theme).textMuted.Alpha(0.10)
	out := make([]editor.TextBackground, 0, len(outside))
	for _, item := range outside {
		if item.From < item.To {
			out = append(out, editor.TextBackground{
				Start: item.From, End: item.To, Color: color,
			})
		}
	}
	return out
}

func (m *Model) activeGraphQLEditor() bool {
	tab := m.tab()
	return tab != nil && tab.doc != nil && tab.state != nil &&
		strings.EqualFold(tab.doc.Info().Language, "graphql")
}

func (m *Model) graphqlEditorToolbar(c *ui.Context, p *ui.Theme, tab *Tab) {
	if tab == nil || tab.doc == nil || !strings.EqualFold(tab.doc.Info().Language, "graphql") {
		return
	}
	m.ensureGraphQL()
	if !m.graphql.discovered && !m.graphql.discovering {
		m.ensureGraphQLDiscovered(nil)
	}
	path := graphqlSlash(tab.doc.Path())
	project := m.graphqlSchemaFor(path)
	configAvailable := project != nil && project.ConfigPath != ""
	endpoint := m.graphqlEndpointFor(path)
	oi := oiPaletteOf(p)
	ui.Column(c.Key("graphql:editor:" + path)).Shrink(0).Children(func() {
		ui.Row(c.Key("graphql:editor-toolbar:"+path)).Height(32).Shrink(0).
			Padding(0, 8).Gap(4).AlignItems(ui.Center).Background(p.Background).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			m.httpToolbarButton(c, p, "general/settings", "Open Configuration File",
				!configAvailable, m.OpenGraphQLConfiguration)
			m.httpToolbarButton(c, p, "general/settings", "Edit Environment Variables",
				project == nil || len(project.Variables) == 0, func() { m.EditGraphQLEnvironment(path) })
			graphqlToolbarSeparator(c, oi)
			variablesShown := m.graphql.variablesShown[path]
			m.graphqlEditorToggle(c, p, oi, "graphqlVariables", "Toggle Variables Editor",
				variablesShown, func() { m.toggleGraphQLVariables(path) })
			label := "No endpoints"
			if endpoint != nil {
				label = endpoint.Name + " - " + endpoint.URL
			}
			menu := ui.MenuButton(c.Key("graphql:endpoint:"+path), label, func(menu *ui.Menu) {
				if project == nil {
					return
				}
				for _, candidate := range project.Endpoints {
					candidate := candidate
					item := menu.Item(candidate.Name + " - " + candidate.URL)
					item.Checked(endpoint != nil && candidate.Name == endpoint.Name)
					if item.Chosen() {
						m.chooseGraphQLEndpoint(path, candidate.Name)
					}
				}
			}).Height(26).MaxWidth(300)
			menu.Disabled(project == nil || len(project.Endpoints) == 0)
			if m.graphql.running != nil {
				m.httpToolbarButton(c, p, "run/stop", "Stop GraphQL Query", false, m.cancelGraphQLRun)
			} else {
				m.httpToolbarButton(c, p, "run/run", "Execute GraphQL",
					endpoint == nil, m.RunGraphQLAtCaret)
			}
			m.httpToolbarButton(c, p, "general/refresh", "Rerun Introspection Query",
				endpoint == nil || m.graphql.running != nil, func() {
					m.RerunGraphQLIntrospection(path)
				})
			m.httpToolbarButton(c, p, "general/externalLink", "Open Introspection Schema",
				project == nil, m.OpenGraphQLIntrospectionSchema)
		})
		if !m.graphql.variablesShown[path] {
			return
		}
		if m.graphql.variablesDraftPath != path {
			m.graphql.variablesDraftPath = path
			m.graphql.variablesDraft = m.graphql.variables[path]
		}
		ui.Row(c.Key("graphql:variables:"+path)).Height(104).Shrink(0).
			Padding(6, 8).Gap(8).AlignItems(ui.Stretch).Background(p.Background).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, "Variables").FontSize(12).TextColor(oi.textMuted).Width(64)
			area := ui.TextArea(c.Key("graphql:variables-editor:"+path), &m.graphql.variablesDraft).
				Grow(1).Height(90).Font("monospace").FontSize(12).
				Placeholder(`{ "code": "VN" }`).Label("GraphQL variables")
			if area.Changed() {
				m.graphql.variables[path] = m.graphql.variablesDraft
			}
		})
	})
}

func (m *Model) graphqlEditorToggle(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	icon, title string,
	pressed bool,
	onClick func(),
) {
	b := ui.Row(c.Key("graphql:toggle:"+title)).Size(24, 24).
		AlignItems(ui.Center).Justify(ui.Center).Radius(4).
		Label(title).Tooltip(title).Cursor(ui.CursorPointer)
	if pressed {
		b.Background(oi.selected)
	} else if b.Hovered() {
		b.Background(oi.hoverTint)
	}
	b.OnClick(onClick)
	b.Children(func() {
		oiIconEl(c, "general/"+icon, 16, oi, oi.textMuted)
	})
}

func (m *Model) graphqlGutter(
	c *ui.Context,
	p *ui.Theme,
	tab *Tab,
	editorRow, view ui.Element,
) {
	if tab == nil || tab.doc == nil || tab.state == nil || tab.state.Buffer == nil ||
		!graphqlIsConfigFile(filepath.Base(tab.doc.Path())) {
		return
	}
	lines := graphqlEndpointLines(filepath.Base(tab.doc.Path()), tab.state.Buffer.String())
	if len(lines) == 0 {
		return
	}
	viewBounds, rowBounds := view.Bounds(), editorRow.Bounds()
	if viewBounds.W <= 0 || viewBounds.H <= 0 {
		return
	}
	first, last := tab.state.VisibleLines()
	first = max(0, first)
	last = min(last, tab.state.Buffer.LineCount()-1)
	oi := oiPaletteOf(p)
	path := tab.doc.Path()
	for _, endpoint := range lines {
		if endpoint.Line < first || endpoint.Line > last {
			continue
		}
		top := float32(tab.state.LineTop(endpoint.Line) - tab.state.ScrollY())
		height := float32(tab.state.LineTop(endpoint.Line+1) - tab.state.LineTop(endpoint.Line))
		if height <= 0 {
			height = tab.state.Font.Size * tab.state.LineHeight
		}
		if top+height <= 0 || top >= viewBounds.H {
			continue
		}
		localX := viewBounds.X - rowBounds.X + 1
		localY := viewBounds.Y - rowBounds.Y + top + max(0, (height-14)/2)
		button := ui.Row(c.Key("graphql:endpoint-gutter:"+endpoint.Endpoint)).
			Absolute().Left(localX).Top(localY).Size(14, 14).
			AlignItems(ui.Center).Justify(ui.Center).Radius(3).
			Role(ui.RoleButton).Focusable().Label("Run Introspection Query").
			Tooltip("Run Introspection Query").Cursor(ui.CursorPointer)
		if button.Hovered() {
			button.Background(oi.hoverTint)
		}
		button.OnClick(func() {
			m.introspectGraphQLEndpoint(path, endpoint.Endpoint)
		})
		button.OnShortcut(ui.Modifiers(0), ui.KeyEnter, func() {
			m.introspectGraphQLEndpoint(path, endpoint.Endpoint)
		})
		button.Children(func() {
			oiIconEl(c, "run/run", 14, oi, oi.success)
		})
	}
}

func (m *Model) noteGraphQLSchemaEdit(tab *Tab, now time.Time) {
	if tab == nil || tab.doc == nil || tab.state == nil || tab.state.Buffer == nil ||
		!graphqlIsSchemaFile(filepath.Base(tab.doc.Path())) {
		return
	}
	m.ensureGraphQL()
	path := graphqlSlash(realPath(tab.doc.Path()))
	if m.graphqlSchemaFor(path) == nil {
		return
	}
	version := tab.state.Buffer.Version()
	if edit, ok := m.graphql.schemaEdits[path]; ok && edit.Version == version {
		return
	}
	m.graphql.schemaEdits[path] = graphqlSchemaEdit{
		Version: version,
		Text:    tab.state.Buffer.String(),
		Edited:  now,
	}
}

func (m *Model) revalidatePendingGraphQLSchemas(now time.Time) {
	for path, edit := range m.graphql.schemaEdits {
		if now.Sub(edit.Edited) < graphqlValidationDelay {
			continue
		}
		delete(m.graphql.schemaEdits, path)
		m.revalidateGraphQLSchema(path, edit.Text)
	}
}
