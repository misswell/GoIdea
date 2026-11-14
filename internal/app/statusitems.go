package app

// The built-in status items, wrapping the fields the bar already showed. The
// left side carries the file (or the status line while no tab is open) and the
// run state; the right side carries problems, the caret, the encoding and the
// backend.

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func init() {
	registerStatusItem(&StatusItem{ID: "status.file", Side: "left", Order: 10,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if t := m.tab(); t != nil {
				ui.Text(c, shortenHome(t.doc.Path())).FontSize(13).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…")
			} else if m.status != "" {
				ui.Text(c, m.status).FontSize(13).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…")
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.run", Side: "left", Order: 20,
		Visible: func(m *Model) bool { return m.run.visible },
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			text := ""
			tone := oi.textMuted
			switch {
			case m.run.running:
				text = "Running"
			case m.run.exitCode != nil && *m.run.exitCode == 0:
				text = "Done"
			case m.run.exitCode != nil:
				text = fmt.Sprintf("Exit code %d", *m.run.exitCode)
				tone = oi.warning
			}
			if text != "" {
				ui.Text(c, text).FontSize(13).TextColor(tone).SingleLine()
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.vcs", Side: "right", Order: 5,
		Visible: func(m *Model) bool { return m.branchLabel() != "" },
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			label := m.branchLabel()
			tk := oiTokensOf(p)
			b := ui.ButtonBase(c.Key("status:branch")).Height(tk.StatusBarHeight-tk.SpaceL).
				Padding(0, tk.SpaceM).Radius(tk.RadiusS).Gap(tk.SpaceS).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(oi.toolbarHover)
			}
			b.Tooltip("Git branches")
			b.Children(func() {
				ui.Icon(c, iconWelcomeBranch).Size(14, 14).TextColor(oi.textMuted)
				ui.Text(c, label).FontSize(tk.FontSizeM).TextColor(oi.textMuted).SingleLine()
			})
			if b.Clicked() {
				m.ToggleBranches()
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.build", Side: "left", Order: 30,
		Visible: func(m *Model) bool { return m.build.busy },
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			ui.Text(c, "Building…").FontSize(13).TextColor(oi.warning).SingleLine()
		}})
	registerStatusItem(&StatusItem{ID: "status.problems", Side: "right", Order: 10,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.problemsStatus(c, p)
		}})
	registerStatusItem(&StatusItem{ID: "status.position", Side: "right", Order: 20,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if t := m.tab(); t != nil {
				pos := t.state.CursorPos()
				ui.Text(c, fmt.Sprintf("Ln %d, Col %d", pos.Line+1, pos.Col+1)).
					FontSize(13).TextColor(oi.textMuted).SingleLine()
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.encoding", Side: "right", Order: 30,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			info := "UTF-8"
			if t := m.tab(); t != nil {
				if enc := t.doc.Info().Encoding; enc != "" {
					info = enc
				}
			}
			ui.Text(c, info).FontSize(13).TextColor(oi.textMuted).SingleLine()
		}})
	registerStatusItem(&StatusItem{ID: "status.backend", Side: "right", Order: 40,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			state := "openidea-server connected"
			if !m.BackendState().Running {
				state = "openidea-server offline"
			}
			ui.Text(c, state).FontSize(13).TextColor(oi.textMuted).SingleLine()
		}})
}
