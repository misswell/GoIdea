package app

// The built-in title bar widgets, wrapping the controls the bar already drew:
// the project and VCS widgets on the left, the run widget and the search and
// terminal tools on the right.

import "github.com/egoist/mygo/ui"

func init() {
	registerTitleWidget(&TitleWidget{ID: toolbarProject, Title: "Project Widget", Side: "left",
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.projectWidget(c, p, oi)
		}})
	registerTitleWidget(&TitleWidget{ID: toolbarVCS, Title: "VCS Widget", Side: "left",
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if label := m.branchLabel(); label != "" {
				m.branchWidget(c, p, oi, label)
			} else {
				m.vcsFallbackWidget(c, p, oi)
			}
		}})
	registerTitleWidget(&TitleWidget{ID: toolbarRun, Title: "Run",
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.runWidget(c, p, oi)
		}})
	registerTitleWidget(&TitleWidget{ID: toolbarSearch, Title: "Search Everywhere",
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.titleTool(c, p, oi, "search", "Search Everywhere", iconWelcomeSearch, func() {
				m.RunCommand(toolbarAction(toolbarSearch))
			})
		}})
	registerTitleWidget(&TitleWidget{ID: toolbarTerminal, Title: "Terminal",
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.titleTool(c, p, oi, "term", "Terminal", iconTerminal, func() {
				m.RunCommand(toolbarAction(toolbarTerminal))
			})
		}})
}
