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
	registerStatusItem(&StatusItem{ID: "status.info", Side: "left", Order: 5,
		Visible: func(m *Model) bool { return m.status != "" },
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			ui.Text(c, m.status).FontSize(13).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
		}})
	registerStatusItem(&StatusItem{ID: "status.file", Title: "Navigation Bar", Side: "left", Order: 10,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if m.navigationBarPosition() == "statusBar" && m.tab() != nil {
				m.renderNavigationBar(c, p, oi)
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.run", Title: "Run Status", Side: "left", Order: 20,
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
	registerStatusItem(&StatusItem{ID: "status.vcs", Title: "Git Branch", Side: "right", Order: 5,
		Visible: func(m *Model) bool { return m.branchLabel() != "" },
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			label := m.branchLabel()
			tk := oiTokensFor(m, p)
			b := ui.ButtonBase(c.Key("status:branch")).Height(tk.StatusBarHeight-tk.SpaceL).
				Padding(0, tk.SpaceM).Radius(tk.RadiusS).Gap(tk.SpaceS).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(oi.toolbarHover)
			}
			b.Tooltip("Git branches")
			b.Children(func() {
				oiIconEl(c, "general/vcs", 14, oi, oi.textMuted)
				ui.Text(c, label).FontSize(tk.FontSizeM).TextColor(oi.textMuted).SingleLine()
			})
			if b.Clicked() {
				m.ToggleBranches()
			}
		}})
	registerStatusItem(&StatusItem{ID: "agents.status", Title: "Agents", Side: "right", Order: 3,
		Visible: func(m *Model) bool {
			input, running, unread, _ := agentStatusSummary(&m.agents)
			return input > 0 || running > 0 || unread > 0
		},
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			input, running, unread, target := agentStatusSummary(&m.agents)
			if target == "" {
				return
			}
			tk := oiTokensFor(m, p)
			b := ui.ButtonBase(c.Key("status:agents")).Height(tk.StatusBarHeight-tk.SpaceL).
				Padding(0, tk.SpaceM).Radius(tk.RadiusS).Gap(tk.SpaceS).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(oi.toolbarHover)
			}
			b.Tooltip("Agents")
			b.Children(func() {
				part := func(label string, tone ui.Color) {
					ui.Text(c, label).FontSize(tk.FontSizeM).TextColor(tone).SingleLine()
				}
				if input > 0 {
					part(fmt.Sprintf("%d needs input", input), oi.warning)
				}
				if running > 0 {
					if input > 0 {
						part(" · ", oi.textSubtle)
					}
					part(fmt.Sprintf("%d running", running), oi.textMuted)
				}
				if unread > 0 {
					if input > 0 || running > 0 {
						part(" · ", oi.textSubtle)
					}
					part(fmt.Sprintf("%d unread", unread), oi.accent)
				}
			})
			if b.Clicked() {
				m.OpenAgentTask(target)
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.build", Title: "Build Status", Side: "left", Order: 30,
		Visible: func(m *Model) bool { return m.build.busy },
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			ui.Text(c, "Building…").FontSize(13).TextColor(oi.warning).SingleLine()
		}})
	registerStatusItem(&StatusItem{ID: "status.problems", Title: "Problems", Side: "right", Order: 10,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.problemsStatus(c, p)
		}})
	registerStatusItem(&StatusItem{ID: "status.position", Title: "Line:Column Number", Side: "right", Order: 20,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if t := m.tab(); t != nil {
				pos := t.state.CursorPos()
				text := fmt.Sprintf("Ln %d, Col %d", pos.Line+1, pos.Col+1)
				if sel := t.state.Selection(); sel.Start() != sel.End() {
					text += fmt.Sprintf(" (%d chars)", utf16Len(t.state.SelectedText()))
				}
				b := ui.Row(c).AlignItems(ui.Center).
					Tooltip("Go to Line:Column… (Cmd+L)")
				if b.Hovered() {
					b.Background(oi.hover)
				}
				b.OnClick(func() { m.RunCommand("navigate.gotoLine") })
				b.Children(func() { ui.Text(c, text).FontSize(13).TextColor(oi.textMuted).SingleLine() })
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.language", Title: "Language", Side: "right", Order: 18,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			if t := m.tab(); t != nil {
				if lang := t.doc.Info().Language; lang != "" && lang != "plaintext" {
					ui.Text(c, strings.ToUpper(lang[:1])+lang[1:]).FontSize(13).
						TextColor(oi.textMuted).SingleLine()
				}
			}
		}})
	registerStatusItem(&StatusItem{ID: "status.indent", Title: "Indentation", Side: "right", Order: 24,
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
	registerStatusItem(&StatusItem{ID: "status.eol", Title: "Line Separator", Side: "right", Order: 28,
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
	registerStatusItem(&StatusItem{ID: "status.encoding", Title: "File Encoding", Side: "right", Order: 30,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			info := "UTF-8"
			if t := m.tab(); t != nil {
				if enc := t.doc.Info().Encoding; enc != "" {
					info = strings.ToUpper(enc)
				}
			}
			ui.Text(c, info).FontSize(13).TextColor(oi.textMuted).SingleLine()
		}})
	registerStatusItem(&StatusItem{ID: "status.backend", Title: "Backend Status", Side: "right", Order: 40,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			state := "GoIDEA backend connected"
			if !m.BackendState().Running {
				state = "GoIDEA backend not connected"
			}
			ui.Text(c, state).FontSize(13).TextColor(oi.textMuted).SingleLine()
		}})
}
