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
			termLabel := "终端"
			if m.term.visible {
				termLabel = "收起终端"
			}
			ui.Button(c, termLabel).OnClick(func() { m.ShowTerm() })
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
			ui.Row(c).Height(30).Padding(0, 6).Gap(4).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
				Children(func() {
					m.sidebarTab(c, p, "项目", m.sideMode != "vcs" && m.sideMode != "search" && m.sideMode != "usages", func() {
						m.sideMode = ""
						m.invalidate()
					})
					changes := "更改"
					if len(m.vcs.files) > 0 {
						changes = fmt.Sprintf("更改 %d", len(m.vcs.files))
					}
					m.sidebarTab(c, p, changes, m.sideMode == "vcs", m.ShowVCS)
					results := "搜索"
					if m.search.matches > 0 {
						results = fmt.Sprintf("搜索 %d", m.search.matches)
					}
					m.sidebarTab(c, p, results, m.sideMode == "search", m.ShowSearch)
					if m.usages.symbol != "" {
						label := "用法"
						if n := len(m.usages.locs); n > 0 {
							label = fmt.Sprintf("用法 %d", n)
						}
						m.sidebarTab(c, p, label, m.sideMode == "usages", func() {
							m.sideMode = "usages"
							m.invalidate()
						})
					}
					ui.Spacer(c)
					if m.sideMode == "vcs" {
						label := "刷新"
						if m.vcs.loading {
							label = "读取中…"
						}
						ui.Button(c, label).OnClick(func() { m.RefreshVCS() })
					}
				})
			ui.Scroll(c).Grow(1).Children(func() {
				if m.sideMode == "vcs" {
					m.vcsRows(c, p)
					return
				}
				if m.sideMode == "search" {
					m.searchRows(c, p)
					return
				}
				if m.sideMode == "usages" {
					m.usagesRows(c, p)
					return
				}
				if m.tree == nil {
					return
				}
				for _, row := range m.tree.Rows() {
					m.treeRow(c, p, row)
				}
			})
		})
}

func (m *Model) sidebarTab(c *ui.Context, p *ui.Theme, label string, active bool, onClick func()) {
	r := ui.Row(c.Key("side:"+label)).Height(22).Padding(0, 8).AlignItems(ui.Center)
	if active {
		r.Background(p.Background)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		color := p.TextMuted
		if active {
			color = p.Text
		}
		ui.Text(c, label).FontSize(12).TextColor(color).SingleLine()
	})
	if r.Clicked() {
		onClick()
	}
}

func (m *Model) vcsRows(c *ui.Context, p *ui.Theme) {
	if m.vcs.err != "" {
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, m.vcs.err).FontSize(11.5).TextColor(p.Warning)
		})
		return
	}
	if len(m.vcs.files) == 0 {
		hint := "没有未提交的更改"
		if !m.vcs.loaded && !m.vcs.loading {
			hint = "点“刷新”读取 Git 状态"
		}
		if m.vcs.loading {
			hint = "正在读取 Git 状态…"
		}
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
		})
		return
	}
	for i, f := range m.vcs.files {
		i, f := i, f
		letter, word, tone := vcsBadge(f)
		color := p.TextMuted
		switch tone {
		case "add":
			color = p.Success
		case "mod":
			color = p.Accent
		case "del":
			color = p.Danger
		}
		r := ui.Row(c.Key(fmt.Sprintf("vcs:%d", i))).Height(22).Padding(0, 8).Gap(6).
			AlignItems(ui.Center)
		if r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			ui.Text(c, letter).FontSize(11).Bold().TextColor(color)
			ui.Text(c, filepath.Base(f.RelPath)).FontSize(12).TextColor(p.Text).
				SingleLine().Ellipsis("…")
			ui.Spacer(c)
			ui.Text(c, word).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		})
		if r.Clicked() {
			m.ShowDiff(f.Path)
		}
		// A double click takes the row further, to the file itself, as IDEA
		// does from its changes view.
		if r.DoubleClicked() {
			m.closeDiff()
			m.openFile(f.Path)
			m.invalidate()
		}
	}
}

func (m *Model) searchRows(c *ui.Context, p *ui.Theme) {
	s := &m.search
	ui.Row(c).Padding(6, 8).Children(func() {
		in := ui.TextInput(c, &s.query).Grow(1)
		if in.Submitted() {
			m.RunSearch()
		}
	})
	ui.Row(c).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		if s.running {
			ui.Button(c, "停止").OnClick(func() {
				m.cancelSearch()
				m.invalidate()
			})
		} else {
			ui.Button(c, "搜索").OnClick(func() { m.RunSearch() })
		}
		ui.Spacer(c)
		text := s.summary
		if s.running {
			text = "搜索中…"
		}
		if s.cancelled && text != "" {
			text = "已停止 · " + text
		}
		if text != "" {
			ui.Text(c, text).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
	})
	if s.err != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, s.err).FontSize(11).TextColor(p.Warning)
		})
	}
	if s.truncated {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, "结果过多，仅显示前面的部分").FontSize(10.5).TextColor(p.TextMuted)
		})
	}
	if len(s.files) == 0 {
		hint := "输入关键词后按 Enter"
		switch {
		case s.running:
			hint = "正在遍历项目文件…"
		case s.done:
			hint = "没有匹配"
		}
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
		})
		return
	}
	shown := 0
	for fi, f := range s.files {
		f := f
		ui.Row(c).Height(22).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			Children(func() {
				ui.Text(c, filepath.Base(f.RelPath)).FontSize(11.5).Bold().TextColor(p.Accent).
					SingleLine().Ellipsis("…")
				ui.Spacer(c)
				ui.Text(c, fmt.Sprintf("%d", len(f.Matches))).FontSize(10.5).TextColor(p.TextMuted)
			})
		for mi, mm := range f.Matches {
			if mi >= 40 || shown >= 200 {
				break
			}
			mi, mm := mi, mm
			shown++
			r := ui.Row(c.Key(fmt.Sprintf("hit:%d:%d", fi, mi))).Height(20).Padding(0, 8).
				Gap(6).AlignItems(ui.Center).PaddingStart(14)
			if r.Hovered() {
				r.Background(p.SurfaceHover)
			}
			r.Children(func() {
				ui.Text(c, fmt.Sprintf("%d", mm.Line+1)).FontSize(10.5).TextColor(p.TextMuted)
				ui.Text(c, strings.TrimSpace(mm.Text)).FontSize(11).TextColor(p.Text).
					SingleLine().Ellipsis("…")
			})
			if r.Clicked() {
				col := 0
				if len(mm.Ranges) > 0 {
					col = int(mm.Ranges[0][0])
				}
				m.revealAt(f.Path, int(mm.Line), col)
			}
		}
	}
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
		m.breadcrumbs(c, p, t)
		editor.View(c, t.state).Grow(1).MinHeight(0)
	})
}

// breadcrumbs is the file and symbol chain the caret sits in; a symbol jumps
// to its declaration.
func (m *Model) breadcrumbs(c *ui.Context, p *ui.Theme, t *Tab) {
	pos := t.state.CursorPos()
	chain := outlinePath(t.outline, pos.Line)
	ui.Row(c).Height(26).Shrink(0).Padding(0, 10).Gap(4).AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, filepath.Base(t.doc.Path())).FontSize(11.5).
				TextColor(p.TextMuted).SingleLine()
			for i, it := range chain {
				it := it
				ui.Text(c, "›").FontSize(11).TextColor(p.TextMuted)
				chip := ui.Row(c.Key(fmt.Sprintf("bc:%d:%s", i, it.Name))).Padding(2, 4).
					AlignItems(ui.Center)
				if chip.Hovered() {
					chip.Background(p.SurfaceHover)
				}
				chip.Children(func() {
					ui.Text(c, it.Name).FontSize(11.5).TextColor(p.Text).SingleLine()
				})
				if chip.Clicked() {
					line, col := t.doc.BytePos(
						int(it.SelectionRange.Start.Line), int(it.SelectionRange.Start.Col))
					t.state.GoTo(editor.Pos{Line: line, Col: col})
					m.invalidate()
				}
			}
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

func (m *Model) buildPanel(c *ui.Context, p *ui.Theme) {
	b := &m.build
	ui.Column(c).Height(220).Shrink(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		m.buildHeader(c, p)
		if b.err != "" {
			ui.Row(c).Height(24).Padding(0, 10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, b.err).FontSize(11.5).TextColor(p.Warning).SingleLine().Ellipsis("…")
			})
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(8, 10).Children(func() {
				if !m.run.trusted {
					ui.Text(c, "此项目未受信任：构建默认禁止。").
						FontSize(12).TextColor(p.TextMuted)
					ui.Row(c).Padding(6, 0).Children(func() {
						ui.Button(c, "信任此项目并启用运行").OnClick(func() { m.TrustProject() })
					})
					return
				}
				if len(b.output) == 0 {
					ui.Text(c, "尚无输出。按“构建项目”开始。").
						FontSize(11.5).TextColor(p.TextMuted)
					return
				}
				if b.truncated {
					ui.Text(c, "（输出过长，已省略较早的行）").
						FontSize(11).TextColor(p.TextMuted)
				}
				ui.Text(c, b.renderText()).FontSize(11.5).TextColor(p.Text)
			})
		})
	})
}

func (m *Model) buildHeader(c *ui.Context, p *ui.Theme) {
	b := &m.build
	ui.Row(c).Height(30).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "构建").FontSize(12).Bold().TextColor(p.Text)
		title := b.title
		if title == "" {
			title = "构建"
		}
		if b.system != "" {
			title += " · " + b.system
		}
		ui.Text(c, title).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
		ui.Spacer(c)
		summary := m.buildSummaryText()
		if summary != "" {
			ui.Text(c, summary).FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
		if b.busy {
			ui.Button(c, "停止").OnClick(func() { m.StopBuild() })
		} else {
			ui.Button(c, "构建").OnClick(func() { m.BuildProject() })
		}
		ui.Button(c, "清空").OnClick(func() { m.ClearBuild() })
		ui.Button(c, "收起").OnClick(func() { m.ShowBuild() })
	})
}

// buildSummaryText is the header's one-line result: the progress while a
// build runs, then the outcome and counts.
func (m *Model) buildSummaryText() string {
	b := &m.build
	if b.busy {
		if b.progress != "" {
			return b.progress
		}
		return "正在构建…"
	}
	if b.success == nil {
		return ""
	}
	if *b.success {
		if b.warnings > 0 {
			return fmt.Sprintf("成功 · %s · %d 个警告", formatBuildDuration(b.duration), b.warnings)
		}
		return "成功 · " + formatBuildDuration(b.duration)
	}
	return fmt.Sprintf("失败 · %d 个错误 · %d 个警告", b.errors, b.warnings)
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

// termPanel is the integrated terminal: scrollback above, one input line that
// sends Enter-terminated text to the shell.
func (m *Model) termPanel(c *ui.Context, p *ui.Theme) {
	t := &m.term
	ui.Column(c).Height(220).Shrink(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "终端").FontSize(12).Bold().TextColor(p.Text)
			shell := t.shell
			switch {
			case t.starting:
				shell = "正在启动…"
			case t.id == 0 && shell == "":
				shell = "未运行"
			case t.id == 0:
				shell += "（已结束）"
			}
			ui.Text(c, shell).FontSize(11.5).TextColor(p.TextMuted).
				SingleLine().Ellipsis("…").Grow(1)
			if t.id == 0 && !t.starting {
				ui.Button(c, "启动").OnClick(func() { m.SpawnTerm() })
			} else {
				ui.Button(c, "结束").OnClick(func() { m.KillTerm() })
			}
			ui.Button(c, "清空").OnClick(func() { m.ClearTerm() })
			ui.Button(c, "收起").OnClick(func() { m.ShowTerm() })
		})
		if t.err != "" {
			ui.Row(c).Shrink(0).Padding(4, 10).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, t.err).FontSize(11.5).TextColor(p.Danger).
					SingleLine().Ellipsis("…").Grow(1)
				ui.Button(c, "关闭").OnClick(func() {
					t.err = ""
					m.invalidate()
				})
			})
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(8, 10).Children(func() {
				if len(t.lines) == 0 {
					hint := "终端输出显示在这里。Enter 发送这一行。"
					if t.id == 0 && !t.starting {
						hint = "点“启动”打开登录 shell。"
					}
					ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
					return
				}
				if t.truncated {
					ui.Text(c, "（输出过长，已省略较早的行）").
						FontSize(11).TextColor(p.TextMuted)
				}
				ui.Text(c, t.render()).FontSize(11.5).TextColor(p.Text)
			})
		})
		ui.Row(c).Height(30).Shrink(0).Padding(3, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "❯").FontSize(12).TextColor(p.Accent)
			in := ui.TextInput(c, &t.input).Grow(1)
			if in.Submitted() {
				m.SubmitTermInput()
			}
		})
	})
}

// render is the scrollback as one string; empty lines keep their height.
func (p *TermPane) render() string {
	var b strings.Builder
	for i, line := range p.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if line == "" {
			b.WriteByte(' ')
		} else {
			b.WriteString(line)
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
			for i, w := range d.watches {
				i, w := i, w
				row := ui.Row(c.Key(fmt.Sprintf("dwatch:%d", i))).Height(22).Padding(0, 8).
					Gap(6).AlignItems(ui.Center)
				row.Children(func() {
					ui.Text(c, w.Expr).FontSize(11.5).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
					ui.Text(c, "=").FontSize(11.5).TextColor(p.TextMuted)
					value, color := w.Value, p.Text
					if w.Err != "" {
						value, color = w.Err, p.Danger
					} else if d.paused && d.evalBusy {
						value = "…"
					} else if value == "" {
						value = "（空）"
					}
					ui.Text(c, value).FontSize(11.5).TextColor(color).
						SingleLine().Ellipsis("…").Grow(1)
					if b := ui.Button(c, "×"); b.Clicked() {
						m.RemoveWatch(i)
					}
				})
			}
			if len(d.vars) == 0 {
				if len(d.watches) > 0 {
					return
				}
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
		ui.Row(c).Height(32).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
			if !d.paused {
				ui.Text(c, "暂停后可在当前帧求值").FontSize(10.5).
					TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				return
			}
			in := ui.TextInput(c, &d.evalText).Grow(1)
			if in.Submitted() {
				m.SubmitWatch()
			}
			ui.Button(c, "求值").Disabled(d.evalBusy).OnClick(func() { m.SubmitWatch() })
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

// bottomSlot draws the bottom slot's one tool window, as OpenIDEA's bottom
// slot shows a single window in its primary area.
func (m *Model) bottomSlot(c *ui.Context, p *ui.Theme) {
	switch {
	case m.run.visible:
		m.runPanel(c, p)
	case m.debug.visible:
		m.debugPanel(c, p)
	case m.build.visible:
		m.buildPanel(c, p)
	case m.term.visible:
		m.termPanel(c, p)
	case m.log.visible:
		m.logPanel(c, p)
	case m.problems.visible:
		m.problemsPanel(c, p)
	}
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
			m.problemsStatus(c, p)
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
