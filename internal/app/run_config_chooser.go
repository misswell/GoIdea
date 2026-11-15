package app

// The Select Run/Debug Configuration popup, the port of upstream
// ConfigChooser behind run.chooseConfig (Ctrl+Alt+R) and
// run.chooseDebugConfig (Ctrl+Alt+D): run a configuration (Shift+Enter to
// debug it instead), debug one, or — from the title bar widget — only select
// one. The chosen configuration runs as saved now, through the same
// stop-and-rerun confirm the Run action uses.

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// runChooser is the configuration chooser popup while it is open.
type runChooser struct {
	open  bool
	mode  string // "run" | "debug" | "select"
	query string
	sel   int
}

// OpenRunChooser opens the chooser: "run" runs the chosen configuration
// (Shift+Enter debugs it), "debug" debugs it, "select" only selects it for
// the run pane, as the widget's chooser does.
func (m *Model) OpenRunChooser(mode string) {
	if m.chooser != nil && m.chooser.open {
		// A second press closes the open chooser, as upstream toggles it.
		m.chooser = nil
		m.invalidate()
		return
	}
	m.chooser = &runChooser{open: true, mode: mode, sel: m.run.sel}
	if !m.run.loaded && !m.run.loading {
		m.loadRunConfigs()
	}
	m.invalidate()
}

// closeRunChooser drops the popup.
func (m *Model) closeRunChooser() {
	m.chooser = nil
	m.invalidate()
}

// chooserRows filters the configurations by the query, over name and kind.
func (m *Model) chooserRows() []*openidea.RunConfig {
	c := m.chooser
	var out []*openidea.RunConfig
	q := strings.ToLower(strings.TrimSpace(c.query))
	for i := range m.run.configs {
		cfg := &m.run.configs[i]
		if q != "" &&
			!strings.Contains(strings.ToLower(cfg.Name), q) &&
			!strings.Contains(strings.ToLower(adapter.KindLabel(cfg.Kind)), q) {
			continue
		}
		out = append(out, cfg)
	}
	return out
}

// stepChooser moves the selection within the filtered rows.
func (m *Model) stepChooser(delta int) {
	c := m.chooser
	if c == nil {
		return
	}
	n := len(m.chooserRows())
	if n == 0 {
		return
	}
	c.sel = ((c.sel+delta)%n + n) % n
	m.invalidate()
}

// chooseActivate applies the mode to the chosen row: how is "run", "debug"
// or "select".
func (m *Model) chooseActivate(how string) {
	c := m.chooser
	if c == nil {
		return
	}
	rows := m.chooserRows()
	if c.sel < 0 || c.sel >= len(rows) {
		return
	}
	cfg := rows[c.sel]
	switch how {
	case "select":
		for i := range m.run.configs {
			if m.run.configs[i].ID == cfg.ID {
				m.run.sel = i
				m.run.inline = nil
				break
			}
		}
		m.closeRunChooser()
	case "debug":
		m.closeRunChooser()
		m.DebugRunConfig(cfg)
	default:
		m.closeRunChooser()
		m.askOrStart(cfg.Raw(), cfg.Name)
	}
}

// chooseCurrentFile debugs the active file's own configuration, the
// "current file" row upstream's chooser shows in debug mode.
func (m *Model) chooseCurrentFile() {
	m.closeRunChooser()
	m.DebugContext()
}

// runChooserDialog draws the configuration chooser.
func (m *Model) runChooserDialog(c *ui.Context, p *ui.Theme) {
	ch := m.chooser
	if ch == nil {
		return
	}
	open := ch.open
	rows := m.chooserRows()
	if ch.sel >= len(rows) {
		ch.sel = len(rows) - 1
	}
	title := map[string]string{
		"run":    "选择运行/调试配置…（Enter 运行，Shift+Enter 调试）",
		"debug":  "选择要调试的配置…（Enter 调试）",
		"select": "选择运行/调试配置…（Enter 选中）",
	}[ch.mode]
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &ch.query).AutoFocus()
			in.OnShortcut(0, ui.KeyDown, func() { m.stepChooser(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepChooser(-1) })
			if in.Submitted() {
				switch {
				case ch.mode == "select":
					m.chooseActivate("select")
				case ch.mode == "debug":
					m.chooseActivate("debug")
				case in.SubmittedMods()&ui.Shift != 0:
					m.chooseActivate("debug")
				default:
					m.chooseActivate("run")
				}
			}
			ui.Scroll(c).Height(320).Children(func() {
				ui.Column(c).Children(func() {
					if len(rows) == 0 {
						ui.Text(c, "没有匹配的运行配置").FontSize(11).TextColor(p.TextMuted).SingleLine()
					}
					for i, cfg := range rows {
						i, cfg := i, cfg
						r := ui.Row(c.Key("runcfg:"+cfg.ID)).Height(26).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == ch.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.OnClick(func() {
							ch.sel = i
							switch ch.mode {
							case "select":
								m.chooseActivate("select")
							case "debug":
								m.chooseActivate("debug")
							default:
								m.chooseActivate("run")
							}
						})
						r.Children(func() {
							checked := m.run.current() != nil && m.run.current().ID == cfg.ID
							ui.Text(c, cfg.Name).FontSize(12).Bold().TextColor(p.Text).SingleLine().Ellipsis("…")
							ui.Text(c, adapter.KindLabel(cfg.Kind)).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
							if checked {
								ui.Text(c, "当前").FontSize(10).TextColor(p.TextMuted).SingleLine()
							}
						})
					}
					if ch.mode != "select" && m.tab() != nil {
						r := ui.Row(c.Key("runcfg:current-file")).Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center)
						if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.OnClick(m.chooseCurrentFile)
						r.Children(func() {
							ui.Text(c, "当前文件").FontSize(12).TextColor(p.Text).SingleLine()
							ui.Text(c, "调试活动文件").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
						})
					}
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(m.closeRunChooser)
			})
		})
	})
	if d := open; !d {
		m.closeRunChooser()
	}
}
