package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const sidebarWidth float32 = 250

func (m *Model) toolbar(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(40).Padding(0, 10).Gap(10).AlignItems(ui.Center).
		Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Text(c, "GoIDEA").FontSize(14).Bold().TextColor(p.Text)
			ui.Button(c, "打开项目").OnClick(func() { m.chooseProject() })
			debugLabel := "调试"
			if m.debug.session != 0 && !m.debug.ended {
				if m.debug.paused {
					debugLabel = "继续"
				} else {
					debugLabel = "调试中"
				}
			}
			ui.Button(c, debugLabel).OnClick(func() {
				switch {
				case m.debug.session == 0 || m.debug.ended:
					m.Debug()
				case m.debug.paused:
					m.DebugContinue()
				default:
					m.ShowDebug()
				}
			})
			runLabel := "运行"
			if m.run.running || m.run.awaiting {
				runLabel = "停止"
			}
			ui.Button(c, runLabel).OnClick(func() {
				if m.run.running || m.run.awaiting {
					m.StopRun()
				} else {
					m.Run()
				}
			})
			root := m.Root()
			name := "未打开项目"
			if root != "" {
				name = root
			}
			ui.Text(c, name).FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			state := "就绪"
			switch {
			case m.busy:
				state = "处理中…"
			case !m.BackendState().Running:
				state = "后端未连接"
			}
			if m.debug.session != 0 && !m.debug.ended {
				if m.debug.paused {
					state = "调试已暂停"
				} else {
					state = "调试中"
				}
			}
			ui.Text(c, state).FontSize(12).TextColor(p.TextMuted)
		})
}

func (m *Model) sidebar(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Width(sidebarWidth).Shrink(0).MinHeight(0).
		Background(p.Surface).BorderWidth(0, 1, 0, 0).BorderColor(p.Border).
		Children(func() {
			ui.Row(c).Height(30).Padding(0, 10).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
				Children(func() {
					ui.Text(c, "项目").FontSize(12).Bold().TextColor(p.TextMuted)
				})
			ui.Scroll(c).Grow(1).Children(func() {
				if m.tree == nil {
					return
				}
				for _, row := range m.tree.Rows() {
					m.treeRow(c, p, row)
				}
			})
		})
}

func (m *Model) treeRow(c *ui.Context, p *ui.Theme, row *treeNode) {
	r := ui.Row(c.Key("tree:"+row.path)).Height(24).Padding(0, 6).
		PaddingStart(6 + float32(row.depth)*14).Gap(5).AlignItems(ui.Center)
	if row.dir && row.expanded {
		r.Background(p.Background)
	}
	if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		label := row.name
		if row.dir {
			arrow := "▸"
			if row.expanded {
				arrow = "▾"
			}
			ui.Text(c, arrow).FontSize(10).TextColor(p.TextMuted)
		}
		color := p.Text
		if row.dir {
			color = p.Accent
		}
		ui.Text(c, label).FontSize(12.5).TextColor(color).SingleLine().Ellipsis("…")
	})
	if r.Clicked() {
		if row.dir {
			m.tree.Toggle(row.path)
		} else {
			m.openFile(row.path)
		}
		m.invalidate()
	}
}

func (m *Model) tabStrip(c *ui.Context, p *ui.Theme) {
	if len(m.tabs) == 0 {
		return
	}
	ui.Row(c).Height(34).Shrink(0).Background(p.Surface).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		for i, t := range m.tabs {
			i, t := i, t
			tab := ui.Row(c.Key(fmt.Sprintf("tab:%d", i))).Height(34).Padding(0, 10).Gap(8).
				AlignItems(ui.Center)
			if i == m.active {
				tab.Background(p.Background)
			} else if tab.Hovered() {
				tab.Background(p.SurfaceHover)
			}
			tab.Children(func() {
				name := filepath.Base(t.doc.Path())
				if t.dirty() {
					ui.Text(c, "●").FontSize(9).TextColor(p.Accent)
				}
				ui.Text(c, name).FontSize(12.5).TextColor(p.Text).SingleLine()
				close := ui.Row(c).Width(18).Height(18).AlignItems(ui.Center).Justify(ui.Center)
				if close.Hovered() {
					close.Background(p.SurfacePressed)
				}
				close.Children(func() { ui.Text(c, "×").FontSize(12).TextColor(p.TextMuted) })
				if close.Clicked() {
					m.closeTab(i)
				}
			})
			if tab.Clicked() {
				m.activate(i)
			}
		}
		ui.Row(c).Grow(1)
	})
}

func (m *Model) bannerRow(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Shrink(0).Padding(8, 12).Gap(10).AlignItems(ui.Center).
		Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Text(c, m.banner).FontSize(12).TextColor(p.Warning).SingleLine().Ellipsis("…").Grow(1)
			if m.bannerPath != "" {
				ui.Button(c, "重新同步").OnClick(func() { m.resolveActive(false) })
				ui.Button(c, "以本地为准").OnClick(func() { m.resolveActive(true) })
			}
			ui.Button(c, "关闭").OnClick(func() {
				m.banner, m.bannerPath = "", ""
				m.invalidate()
			})
		})
}

func (m *Model) editorArea(c *ui.Context, p *ui.Theme) {
	t := m.tab()
	if t == nil {
		ui.Column(c).Grow(1).FillHeight().AlignItems(ui.Center).Justify(ui.Center).Gap(8).
			Background(p.Background).Children(func() {
			ui.Text(c, "GoIDEA").FontSize(22).Bold().TextColor(p.Text)
			ui.Text(c, "打开一个 Java 项目，然后从左侧选择文件").FontSize(13).TextColor(p.TextMuted)
			ui.Button(c, "打开项目").OnClick(func() { m.chooseProject() })
		})
		return
	}
	ui.Column(c.Key("editor:" + t.doc.Path())).Grow(1).MinHeight(0).MinWidth(0).
		Background(p.Background).Children(func() {
		editor.View(c, t.state).Grow(1).MinHeight(0)
	})
}

func (m *Model) runPanel(c *ui.Context, p *ui.Theme) {
	if !m.run.visible {
		return
	}
	ui.Column(c).Height(240).Shrink(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		m.runHeader(c, p)
		if m.run.err != "" {
			ui.Row(c).Height(24).Padding(0, 10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, m.run.err).FontSize(11.5).TextColor(p.Warning).SingleLine().Ellipsis("…")
			})
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(8, 10).Children(func() {
				if !m.run.trusted {
					ui.Text(c, "此项目未受信任：构建和运行默认禁止。").
						FontSize(12).TextColor(p.TextMuted)
					if m.run.trustErr != "" {
						ui.Text(c, m.run.trustErr).FontSize(11.5).TextColor(p.Warning)
					}
					ui.Row(c).Padding(6, 0).Children(func() {
						ui.Button(c, "信任此项目并启用运行").OnClick(func() { m.TrustProject() })
					})
					return
				}
				if len(m.run.output) == 0 {
					ui.Text(c, "尚无输出。按“运行”开始。").
						FontSize(11.5).TextColor(p.TextMuted)
					return
				}
				if m.run.truncated {
					ui.Text(c, "（输出过长，已省略较早的行）").
						FontSize(11).TextColor(p.TextMuted)
				}
				ui.Text(c, m.run.renderText()).FontSize(11.5).TextColor(p.Text)
			})
		})
	})
}

func (m *Model) runHeader(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(30).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "运行").FontSize(12).Bold().TextColor(p.Text)
		if m.run.trusted {
			ui.Button(c, "‹").OnClick(func() { m.CycleRunConfig(-1) })
			name := "正在读取配置…"
			if cfg := m.run.current(); cfg != nil {
				name = cfg.Name
			}
			ui.Text(c, name).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…").Grow(1)
			ui.Button(c, "›").OnClick(func() { m.CycleRunConfig(1) })
		} else {
			ui.Text(c, "未信任的项目不会构建或运行").
				FontSize(11.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…").Grow(1)
		}
		if m.run.running || m.run.awaiting {
			ui.Button(c, "停止").OnClick(func() { m.StopRun() })
		} else {
			ui.Button(c, "运行").OnClick(func() { m.Run() })
		}
		ui.Button(c, "清空").OnClick(func() { m.ClearRun() })
		ui.Button(c, "收起").OnClick(func() { m.ShowRun() })
	})
}

// renderText is the output as one string; empty lines keep their height.
func (p *RunPane) renderText() string {
	var b strings.Builder
	for i, line := range p.output {
		if i > 0 {
			b.WriteByte('\n')
		}
		if line.text == "" {
			b.WriteByte(' ')
		} else {
			b.WriteString(line.text)
		}
	}
	return b.String()
}

func (m *Model) debugPanel(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Height(300).Shrink(0).MinHeight(0).
		Background(p.Surface).BorderWidth(1, 0, 0, 0).BorderColor(p.Border).
		Children(func() {
			m.debugHeader(c, p)
			if m.debug.err != "" {
				ui.Row(c).Shrink(0).Padding(4, 10).Gap(8).AlignItems(ui.Center).
					Background(p.Background).Children(func() {
					ui.Text(c, m.debug.err).FontSize(11.5).TextColor(p.Danger).
						SingleLine().Ellipsis("…").Grow(1)
					ui.Button(c, "关闭").OnClick(func() {
						m.debug.err = ""
						m.invalidate()
					})
				})
			}
			ui.Row(c).Grow(1).MinHeight(0).Children(func() {
				m.debugFrames(c, p)
				m.debugVars(c, p)
				m.debugConsole(c, p)
			})
		})
}

func (m *Model) debugHeader(c *ui.Context, p *ui.Theme) {
	d := &m.debug
	ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "调试").FontSize(12).Bold().TextColor(p.Text)
		config := d.config
		if config == "" {
			config = "当前文件的主类"
		}
		ui.Text(c, config).FontSize(12).TextColor(p.TextMuted).
			SingleLine().Ellipsis("…").Grow(1)
		if d.session == 0 && !d.starting {
			ui.Button(c, "调试").OnClick(func() { m.Debug() })
		}
		if d.paused {
			ui.Button(c, "继续").OnClick(func() { m.DebugContinue() })
			ui.Button(c, "步过").OnClick(func() { m.StepOver() })
			ui.Button(c, "步入").OnClick(func() { m.StepInto() })
			ui.Button(c, "步出").OnClick(func() { m.StepOut() })
		} else if d.session != 0 && !d.ended {
			ui.Button(c, "暂停").OnClick(func() { m.DebugPause() })
		}
		if d.session != 0 && !d.ended {
			ui.Button(c, "停止").OnClick(func() { m.StopDebug() })
		}
		ui.Button(c, "收起").OnClick(func() { m.ShowDebug() })
	})
}

func (m *Model) debugFrames(c *ui.Context, p *ui.Theme) {
	d := &m.debug
	ui.Column(c).Width(260).Shrink(0).MinHeight(0).
		BorderWidth(0, 1, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(24).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			thread := "调用栈"
			if len(d.threads) > 0 {
				thread = fmt.Sprintf("调用栈 · %s", d.threads[0].Name)
			}
			ui.Text(c, thread).FontSize(11.5).Bold().TextColor(p.TextMuted).
				SingleLine().Ellipsis("…")
		})
		ui.Scroll(c).Grow(1).Children(func() {
			if !d.paused || len(d.frames) == 0 {
				ui.Row(c).Padding(8, 8).Children(func() {
					ui.Text(c, d.status).FontSize(11.5).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
				})
				return
			}
			for i, f := range d.frames {
				i, f := i, f
				row := ui.Row(c.Key(fmt.Sprintf("dframe:%d", i))).Height(22).Padding(0, 8).
					Gap(6).AlignItems(ui.Center)
				if i == d.selFrame {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, fmt.Sprintf("%s  :%d", f.Name, f.Line+1)).
						FontSize(11.5).TextColor(p.Text).SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					m.SelectFrame(i)
				}
			}
		})
	})
}

func (m *Model) debugVars(c *ui.Context, p *ui.Theme) {
	d := &m.debug
	ui.Column(c).Width(320).Shrink(0).MinHeight(0).
		BorderWidth(0, 1, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(24).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			name := "变量"
			if len(d.scopes) > 0 {
				name = "变量 · " + d.scopes[0].Name
			}
			ui.Text(c, name).FontSize(11.5).Bold().TextColor(p.TextMuted).
				SingleLine().Ellipsis("…")
		})
		ui.Scroll(c).Grow(1).Children(func() {
			if len(d.vars) == 0 {
				hint := "暂停后显示局部变量"
				if d.paused {
					hint = "没有局部变量"
				}
				ui.Row(c).Padding(8, 8).Children(func() {
					ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
				})
				return
			}
			for _, v := range d.vars {
				ui.Row(c).Height(22).Padding(0, 8).Gap(6).AlignItems(ui.Center).
					Children(func() {
						ui.Text(c, v.Name).FontSize(11.5).TextColor(p.TextMuted).
							SingleLine().Ellipsis("…")
						ui.Text(c, "=").FontSize(11.5).TextColor(p.TextMuted)
						ui.Text(c, v.Value).FontSize(11.5).TextColor(p.Text).
							SingleLine().Ellipsis("…").Grow(1)
					})
			}
		})
	})
}

func (m *Model) debugConsole(c *ui.Context, p *ui.Theme) {
	d := &m.debug
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(24).Padding(0, 8).Children(func() {
			ui.Text(c, "调试控制台").FontSize(11.5).Bold().TextColor(p.TextMuted)
		})
		ui.Scroll(c).Grow(1).Children(func() {
			if len(d.output) == 0 {
				ui.Row(c).Padding(8, 8).Children(func() {
					ui.Text(c, "程序的输出显示在这里").FontSize(11.5).TextColor(p.TextMuted)
				})
				return
			}
			if d.truncated {
				ui.Row(c).Padding(4, 8).Children(func() {
					ui.Text(c, "（输出过长，已省略较早的行）").
						FontSize(11).TextColor(p.TextMuted)
				})
			}
			for _, line := range d.output {
				color := p.Text
				if line.kind == "stderr" {
					color = p.Warning
				}
				text := line.text
				if text == "" {
					text = " "
				}
				ui.Row(c).Height(18).Padding(0, 8).Children(func() {
					ui.Text(c, text).FontSize(11.5).TextColor(color).SingleLine()
				})
			}
		})
	})
}

func (m *Model) statusBar(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(24).Shrink(0).Padding(0, 10).Gap(16).AlignItems(ui.Center).
		Background(p.Surface).BorderWidth(1, 0, 0, 0).BorderColor(p.Border).
		Children(func() {
			ui.Text(c, m.status).FontSize(11.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			if m.run.visible {
				text := ""
				color := p.TextMuted
				switch {
				case m.run.running:
					text = "运行中"
				case m.run.exitCode != nil && *m.run.exitCode == 0:
					text = "运行成功"
				case m.run.exitCode != nil:
					text = fmt.Sprintf("退出码 %d", *m.run.exitCode)
					color = p.Warning
				}
				if text != "" {
					ui.Text(c, text).FontSize(11.5).TextColor(color)
				}
			}
			if t := m.tab(); t != nil {
				pos := t.state.CursorPos()
				ui.Text(c, fmt.Sprintf("行 %d，列 %d", pos.Line+1, pos.Col+1)).
					FontSize(11.5).TextColor(p.TextMuted)
			}
			info := "UTF-8"
			if t := m.tab(); t != nil {
				if enc := t.doc.Info().Encoding; enc != "" {
					info = enc
				}
			}
			ui.Text(c, info).FontSize(11.5).TextColor(p.TextMuted)
			state := "openidea-server 已连接"
			if !m.BackendState().Running {
				state = "openidea-server 未连接"
			}
			ui.Text(c, state).FontSize(11.5).TextColor(p.TextMuted)
		})
}
