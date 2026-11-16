package app

// The built-in status items, wrapping the fields the bar already showed. The
// left side carries the file (or the status line while no tab is open) and the
// run state; the right side carries problems, the caret, the encoding and the
// backend.

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func init() {
	registerStatusItem(&StatusItem{ID: "status.startup", Side: "left", Order: 1,
		Visible: func(m *Model) bool {
			p := m.Phase()
			return p != "" && p != "ready"
		},
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			text := map[string]string{
				"preflight": "Checking the GoIDEA backend…",
				"server":    "Starting the GoIDEA backend…",
				"opening":   "Opening the project…",
				"restoring": "Restoring files…",
				"failed":    "Start failed",
			}[m.Phase()]
			if text != "" {
				ui.Text(c, text).FontSize(13).TextColor(oi.warning).SingleLine()
			}
		}})
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
				text := fmt.Sprintf("Ln %d, Col %d", pos.Line+1, pos.Col+1)
				if sel := t.state.Selection(); sel.Start() != sel.End() {
					text += fmt.Sprintf(" (%d chars)", utf16Len(t.state.SelectedText()))
				}
				ui.Text(c, text).FontSize(13).TextColor(oi.textMuted).SingleLine()
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.language", Side: "right", Order: 18,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if t := m.tab(); t != nil {
				if lang := t.doc.Info().Language; lang != "" && lang != "plaintext" {
					ui.Text(c, strings.ToUpper(lang[:1])+lang[1:]).FontSize(13).
						TextColor(oi.textMuted).SingleLine()
				}
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.indent", Side: "right", Order: 24,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if t := m.tab(); t != nil {
				size := t.state.TabSize
				if size <= 0 {
					size = 4
				}
				label := fmt.Sprintf("%d spaces", size)
				if t.state.Indent == editor.IndentTabs {
					label = "Tab"
				}
				ui.Text(c, label).FontSize(13).TextColor(oi.textMuted).SingleLine()
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.eol", Side: "right", Order: 28,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			label := "LF"
			if t := m.tab(); t != nil {
				switch strings.ToLower(t.doc.Info().LineEnding) {
				case "crlf":
					label = "CRLF"
				case "cr":
					label = "CR"
				}
			}
			ui.Text(c, label).FontSize(13).TextColor(oi.textMuted).SingleLine()
		}})
	registerStatusItem(&StatusItem{ID: "status.encoding", Side: "right", Order: 30,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			info := "UTF-8"
			if t := m.tab(); t != nil {
				if enc := t.doc.Info().Encoding; enc != "" {
					info = strings.ToUpper(enc)
				}
			}
			ui.Text(c, info).FontSize(13).TextColor(oi.textMuted).SingleLine()
		}})
	registerStatusItem(&StatusItem{ID: "status.backend", Side: "right", Order: 40,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			state := "GoIDEA backend connected"
			if !m.BackendState().Running {
				state = "GoIDEA backend not connected"
			}
			ui.Text(c, state).FontSize(13).TextColor(oi.textMuted).SingleLine()
		}})
}
