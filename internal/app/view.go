package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const sidebarWidth float32 = 250

// toolbar is the title bar: the project, the branch widget, and the run
// controls, on the panel color OpenIDEA draws its chrome with.
func (m *Model) toolbar(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(36).Padding(0, 10).Gap(6).AlignItems(ui.Center).
		Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Row(c).Width(18).Height(18).Justify(ui.Center).AlignItems(ui.Center).
				Background(p.Accent).Children(func() {
				ui.Text(c, "G").FontSize(11).Bold().TextColor(p.AccentText)
			})
			name := "未打开项目"
			if root := m.Root(); root != "" {
				name = filepath.Base(root)
			}
			ui.Text(c, name).FontSize(12.5).Bold().TextColor(p.Text).
				SingleLine().Ellipsis("…")
			ui.Button(c, "打开项目").OnClick(func() { m.chooseProject() })
			if label := m.branchLabel(); label != "" {
				ui.Button(c, "⎇ "+label).OnClick(func() { m.ToggleBranches() })
			}
			ui.Spacer(c)
			runLabel := "▶ 运行"
			if m.run.running || m.run.awaiting {
				runLabel = "■ 停止"
			}
			m.toolChip(c, p, "run", runLabel, m.run.running || m.run.awaiting, func() {
				if m.run.running || m.run.awaiting {
					m.StopRun()
				} else {
					m.Run()
				}
			})
			debugLabel := "◪ 调试"
			debugOn := m.debug.session != 0 && !m.debug.ended
			if debugOn {
				if m.debug.paused {
					debugLabel = "▶ 继续"
				} else {
					debugLabel = "◪ 调试中"
				}
			}
			m.toolChip(c, p, "debug", debugLabel, debugOn, func() {
				switch {
				case m.debug.session == 0 || m.debug.ended:
					m.Debug()
				case m.debug.paused:
					m.DebugContinue()
				default:
					m.ShowDebug()
				}
			})
			termLabel := "❯ 终端"
			if m.term.visible {
				termLabel = "❯ 收起终端"
			}
			m.toolChip(c, p, "term", termLabel, m.term.visible, func() { m.ShowTerm() })
			state := "就绪"
			switch {
			case m.busy:
				state = "处理中…"
			case !m.BackendState().Running:
				state = "后端未连接"
			}
			if debugOn {
				if m.debug.paused {
					state = "调试已暂停"
				} else {
					state = "调试中"
				}
			}
			ui.Text(c, state).FontSize(11).TextColor(p.TextMuted).SingleLine()
		})
}

// toolChip is one run control of the title bar, highlighted while its tool
// is on.
func (m *Model) toolChip(c *ui.Context, p *ui.Theme, key, label string, on bool, onClick func()) {
	r := ui.Row(c.Key("toolbar:"+key)).Height(24).Padding(0, 7).AlignItems(ui.Center)
	if on {
		r.Background(p.SurfacePressed)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		tone := p.Text
		if on {
			tone = p.Accent
		}
		ui.Text(c, label).FontSize(11.5).TextColor(tone).SingleLine()
	})
	if r.Clicked() {
		onClick()
	}
}

// sidebar is OpenIDEA's left side: the rail of tool windows and the panel of
// the one showing, below its header.
func (m *Model) sidebar(c *ui.Context, p *ui.Theme) {
	projects := m.sideMode != "vcs" && m.sideMode != "structure" && m.sideMode != "bookmarks"
	changes := "更改"
	if len(m.vcs.files) > 0 {
		changes = fmt.Sprintf("更改 %d", len(m.vcs.files))
	}
	ui.Row(c).Width(sidebarWidth).Shrink(0).MinHeight(0).
		Background(p.Surface).BorderWidth(0, 1, 0, 0).BorderColor(p.Border).
		Children(func() {
			ui.Column(c).Width(48).Shrink(0).Padding(4, 4).Gap(2).Children(func() {
				m.sideRailItem(c, p, "项目", "▦", projects, func() {
					m.structure.visible = false
					m.sideMode = ""
					m.invalidate()
				})
				m.sideRailItem(c, p, "更改", "⎇", m.sideMode == "vcs", m.ShowVCS)
				m.sideRailItem(c, p, "结构", "≡", m.sideMode == "structure", m.ToggleStructure)
				m.sideRailItem(c, p, "书签", "⚑", m.sideMode == "bookmarks", m.ToggleBookmarksWindow)
				ui.Spacer(c)
				m.sideRailItem(c, p, "命令", "⌘", false, m.OpenCommands)
			})
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				title := "项目"
				switch m.sideMode {
				case "vcs":
					title = changes
				case "structure":
					title = "结构"
				case "bookmarks":
					title = "书签"
				}
				ui.Row(c).Height(32).Shrink(0).Padding(0, 8, 0, 6).Gap(4).
					AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
					Children(func() {
						ui.Text(c, title).FontSize(11.5).Bold().TextColor(p.TextMuted).
							SingleLine().Ellipsis("…")
						ui.Spacer(c)
						switch m.sideMode {
						case "vcs":
							label := "刷新"
							if m.vcs.loading {
								label = "读取中…"
							}
							ui.Button(c, label).OnClick(func() { m.RefreshVCS() })
						case "":
							if m.tree != nil {
								ui.Button(c, "⊞").OnClick(func() {
									m.tree.ExpandAll(m.Root())
									m.invalidate()
								})
								ui.Button(c, "⊟").OnClick(func() {
									m.tree.CollapseAll()
									m.invalidate()
								})
							}
						}
					})
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					if m.sideMode == "vcs" {
						m.vcsRows(c, p)
						return
					}
					if m.sideMode == "structure" {
						m.structureRowsUI(c, p)
						return
					}
					if m.sideMode == "bookmarks" {
						m.bookmarkWindowRows(c, p)
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
		})
}

// sideRailItem is one tool window of the rail: its glyph over the start of
// its name, with a count when it has one.
func (m *Model) sideRailItem(c *ui.Context, p *ui.Theme, label, glyph string, active bool, onClick func()) {
	r := ui.Row(c.Key("side:" + label)).Height(46).Shrink(0).AlignItems(ui.Center).
		Justify(ui.Center)
	if active {
		r.Background(p.SurfaceHover)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		ui.Column(c).Gap(1).AlignItems(ui.Center).Children(func() {
			tone := p.TextMuted
			if active {
				tone = p.Accent
			}
			ui.Text(c, glyph).FontSize(15).TextColor(tone)
			ui.Text(c, label).FontSize(9.5).TextColor(tone).SingleLine()
		})
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
	if m.vcs.opErr != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, m.vcs.opErr).FontSize(11).TextColor(p.Danger).SingleLine().
				Ellipsis("…")
		})
	}
	// The staging groups OpenIDEA's changes view draws: a file staged and then
	// edited again is listed under both.
	staged, unstaged := vcsStaged(m.vcs.files), vcsUnstaged(m.vcs.files)
	if len(staged) > 0 {
		m.vcsGroup(c, p, "已暂存", len(staged))
		for i, f := range staged {
			m.vcsRow(c, p, "staged", i, f)
		}
	}
	if len(unstaged) > 0 {
		m.vcsGroup(c, p, "未暂存", len(unstaged))
		for i, f := range unstaged {
			m.vcsRow(c, p, "unstaged", i, f)
		}
	}
}

func (m *Model) vcsGroup(c *ui.Context, p *ui.Theme, label string, n int) {
	ui.Row(c).Height(22).Padding(6, 8).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).Bold().TextColor(p.TextMuted).SingleLine()
		ui.Text(c, fmt.Sprintf("%d", n)).FontSize(10.5).TextColor(p.TextMuted)
	})
}

func (m *Model) vcsRow(c *ui.Context, p *ui.Theme, side string, i int, f openidea.FileStatus) {
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
	r := ui.Row(c.Key(fmt.Sprintf("vcs:%s:%d", side, i))).Height(22).Padding(0, 8).Gap(6).
		AlignItems(ui.Center)
	if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		ui.Text(c, letter).FontSize(11).Bold().TextColor(color)
		ui.Text(c, filepath.Base(f.RelPath)).FontSize(12).TextColor(p.Text).
			SingleLine().Ellipsis("…")
		ui.Spacer(c)
		if side == "staged" {
			if m.rowChip(c, p, "vcs:unstage:"+f.Path, "取消暂存") {
				m.UnstageChange(f)
			}
		} else {
			if m.rowChip(c, p, "vcs:stage:"+f.Path, "暂存") {
				m.StageChange(f)
			}
			if m.rowChip(c, p, "vcs:rollback:"+f.Path, "回滚") {
				m.AskRollback(f)
			}
		}
		ui.Text(c, word).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
	})
	if r.Clicked() {
		m.ShowDiff(f.Path)
	}
	// A double click takes the row further, to the file itself, as IDEA does
	// from its changes view.
	if r.DoubleClicked() {
		m.closeDiff()
		m.openFile(f.Path)
		m.invalidate()
	}
}

// rowChip is one small action on a list row; it reports a click.
func (m *Model) rowChip(c *ui.Context, p *ui.Theme, key, label string) bool {
	chip := ui.Row(c.Key(key)).Height(16).Padding(0, 5).AlignItems(ui.Center)
	if chip.Hovered() {
		chip.Background(p.SurfacePressed)
	}
	chip.Children(func() {
		ui.Text(c, label).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
	})
	return chip.Clicked()
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
		if s.scopePath != "" {
			if rel, err := filepath.Rel(m.Root(), s.scopePath); err == nil {
				ui.Text(c, "范围："+filepath.ToSlash(rel)).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
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
	if row.path == m.tree.Selected() {
		r.Background(p.Selection)
	} else if row.dir && row.expanded {
		r.Background(p.Background)
	}
	if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	bookmarked := false
	for _, pb := range m.pathBookmarks {
		if pb.Path == row.path {
			bookmarked = true
			break
		}
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
		if bookmarked || r.Hovered() {
			ui.Spacer(c)
			chip := ui.Row(c.Key("treeBookmark:"+row.path)).Height(16).Padding(0, 5).
				AlignItems(ui.Center)
			if chip.Hovered() {
				chip.Background(p.SurfacePressed)
			}
			chip.Children(func() {
				tone := p.TextMuted
				if bookmarked {
					tone = p.Accent
				}
				ui.Text(c, "书签").FontSize(10.5).TextColor(tone).SingleLine()
			})
			if chip.Clicked() {
				m.ToggleFolderBookmark(row.path, row.dir)
			}
		}
	})
	if r.Clicked() {
		m.tree.Select(row.path)
		if row.dir {
			m.tree.Toggle(row.path)
		} else {
			m.openFile(row.path)
		}
		m.invalidate()
	}
	target := treeTarget{path: row.path, dir: row.dir, root: row.path == m.Root()}
	r.ContextMenu(func(menu *ui.Menu) { m.treeMenu(c, menu, target) })
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
				tab.Background(p.Background).BorderWidth(2, 0, 0, 0).BorderColor(p.Accent)
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
		// An open project with no editor tab: the editor island stays empty,
		// as OpenIDEA leaves it, with the one hint that gets a file open.
		ui.Column(c).Grow(1).FillHeight().AlignItems(ui.Center).Justify(ui.Center).Gap(10).
			Background(p.Background).Children(func() {
			ui.Text(c, "没有打开的文件").FontSize(13).TextColor(p.TextMuted)
			ui.Button(c, "快速打开…").OnClick(func() { m.OpenQuickFiles() })
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
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
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
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
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

// termPanel is the integrated terminal. With one group it shows that group's
// tabs, scrollback and input line; a split shows two groups, side by side for
// Split Right or stacked for Split Down, as OpenIDEA splits its terminal.
func (m *Model) termPanel(c *ui.Context, p *ui.Theme) {
	t := &m.term
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	// Find in Terminal, bounded to the focus: Cmd+F in the editor stays the
	// editor's.
	if col.Shortcut(ui.Cmd, ui.KeyF) {
		m.ShowTermFind()
	}
	col.Children(func() {
		ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "终端").FontSize(12).Bold().TextColor(p.Text)
			if n := t.total(); n > 0 {
				ui.Text(c, fmt.Sprintf("%d 个会话", n)).
					FontSize(11).TextColor(p.TextMuted)
			}
			ui.Spacer(c)
			ui.Button(c, "新建").OnClick(func() { m.OpenTermTab() })
			ui.Button(c, "拆分").OnClick(func() { m.SplitTermRight() })
			ui.Button(c, "拆分↓").OnClick(func() { m.SplitTermDown() })
			if t.split != "" {
				ui.Button(c, "合并").OnClick(func() { m.UnsplitTerm() })
			}
			if s := t.current(); s == nil || (s.id == 0 && !s.starting) {
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
		if t.finding {
			ui.Row(c).Height(30).Shrink(0).Padding(3, 8).Gap(6).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
				ui.Text(c, "查找").FontSize(11.5).TextColor(p.TextMuted)
				in := ui.TextInput(c, &t.find).Grow(1)
				if in.Submitted() {
					m.FindTermStep(1)
				}
				if label := m.termFindLabel(); label != "" {
					ui.Text(c, label).FontSize(11).TextColor(p.TextMuted)
				}
				ui.Button(c, "上一个").OnClick(func() { m.FindTermStep(-1) })
				ui.Button(c, "下一个").OnClick(func() { m.FindTermStep(1) })
				ui.Button(c, "关闭").OnClick(func() { m.HideTermFind() })
			})
		}
		if t.split != "" && len(t.groups) > 1 {
			if t.split == "down" {
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					m.termPaneBody(c, p, 0)
					ui.Row(c).Height(1).Shrink(0).Background(p.Border)
					m.termPaneBody(c, p, 1)
				})
			} else {
				ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
					m.termPaneBody(c, p, 0)
					ui.Column(c).Width(1).Shrink(0).Background(p.Border)
					m.termPaneBody(c, p, 1)
				})
			}
			return
		}
		m.termPaneBody(c, p, 0)
	})
}

// termPaneBody draws one tab group: its own tab strip, its active tab's
// scrollback, and the input line when this group takes typing.
func (m *Model) termPaneBody(c *ui.Context, p *ui.Theme, g int) {
	t := &m.term
	if g < 0 || g >= len(t.groups) {
		return
	}
	gr := &t.groups[g]
	pane := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0)
	pane.Children(func() {
		ui.Row(c).Height(26).Shrink(0).Padding(0, 6).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			for i := range gr.tabs {
				s := &gr.tabs[i]
				label := s.shell
				if label == "" {
					label = fmt.Sprintf("终端 %d", i+1)
				}
				switch {
				case s.starting:
					label = "正在启动…"
				case s.id == 0 && s.ended:
					label += "（已结束）"
				}
				selected := t.active == g && gr.active == i
				tabColor := p.TextMuted
				if selected {
					tabColor = p.Text
				}
				tab := ui.Row(c.Key(fmt.Sprintf("termTab:%d", s.channel))).
					Height(22).Padding(0, 6).Gap(2).AlignItems(ui.Center)
				if selected {
					tab.Background(p.SurfacePressed)
				}
				tab.Children(func() {
					ui.Text(c, label).FontSize(11.5).TextColor(tabColor).
						SingleLine().Ellipsis("…")
					ui.Button(c, "×").OnClick(func() { m.CloseTermTab(g, i) })
				})
				if tab.Clicked() {
					t.active = g
					if i != gr.active {
						gr.active = i
					}
					m.invalidate()
				}
			}
			if len(gr.tabs) == 0 {
				ui.Text(c, "未运行").FontSize(11.5).TextColor(p.TextMuted)
			}
			ui.Spacer(c)
			if t.split != "" {
				ui.Button(c, "+").OnClick(func() { m.OpenTermTabIn(g) })
			}
		})
		// The scrollback is one text row per line, so find can mark the lines it
		// matched and scroll the chosen one into view.
		s := gr.tab()
		scroll := ui.Scroll(c).Grow(1).MinHeight(0)
		if s != nil {
			scroll.TrackScroll(&s.scroll)
		}
		scroll.Children(func() {
			ui.Column(c).Padding(8, 10).Children(func() {
				if s == nil || len(s.lines) == 0 {
					hint := "终端输出显示在这里。Enter 发送这一行。"
					if s == nil || (s.id == 0 && !s.starting) {
						hint = "点“新建”打开一个登录 shell。"
					}
					ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
					return
				}
				if s.truncated {
					ui.Text(c, "（输出过长，已省略较早的行）").
						FontSize(11).TextColor(p.TextMuted)
				}
				var hits map[int]bool
				if t.finding && t.find != "" && t.active == g {
					hits = map[int]bool{}
					for _, line := range termHits(s.lines, t.find) {
						hits[line] = true
					}
				}
				for i, line := range s.lines {
					text := line
					if text == "" {
						text = " "
					}
					row := ui.Text(c, text).FontSize(11.5).TextColor(p.Text).
						Key(fmt.Sprintf("termLine:%d:%d", s.channel, i))
					if hits[i] {
						if i == s.findAt {
							row.Background(p.Selection)
						} else {
							row.Background(p.SurfacePressed)
						}
					}
					if s.reveal && i == s.findAt {
						row.ScrollIntoView()
					}
				}
				s.reveal = false
			})
		})
		if t.active == g || len(t.groups) == 1 {
			ui.Row(c).Height(30).Shrink(0).Padding(3, 8).Gap(6).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
				ui.Text(c, "❯").FontSize(12).TextColor(p.Accent)
				in := ui.TextInput(c, &t.input).Grow(1)
				if in.Submitted() {
					m.SubmitTermInput()
				}
			})
		}
	})
}

func (m *Model) debugPanel(c *ui.Context, p *ui.Theme) {
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).
		Background(p.Surface).BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
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

// bottomSlot draws the bottom side: the window of its primary area and, when a
// window was moved there, the split area's window beside it, as OpenIDEA lays
// out the two areas of a side. The side keeps one height whichever it shows.
func (m *Model) bottomSlot(c *ui.Context, p *ui.Theme) {
	primary, split := m.primaryTool(), m.splitTool()
	if primary == "" && split == "" {
		return
	}
	row := ui.Row(c).Shrink(0).AlignItems(ui.Stretch)
	if m.maximized {
		row.Grow(1)
	} else {
		row.Height(float32(m.bottomSize()))
	}
	row.Children(func() {
		if primary != "" {
			m.toolPanel(c, p, primary)
		}
		if primary != "" && split != "" {
			ui.Column(c).Width(1).Shrink(0).Background(p.Border)
		}
		if split != "" {
			m.toolPanel(c, p, split)
		}
	})
}

func (m *Model) toolPanel(c *ui.Context, p *ui.Theme, name string) {
	switch name {
	case "run":
		m.runPanel(c, p)
	case "debug":
		m.debugPanel(c, p)
	case "build":
		m.buildPanel(c, p)
	case "term":
		m.termPanel(c, p)
	case "log":
		m.logPanel(c, p)
	case "problems":
		m.problemsPanel(c, p)
	case "find":
		m.searchPanel(c, p)
	case "usages":
		m.usagesPanel(c, p)
	}
}

func (m *Model) searchPanel(c *ui.Context, p *ui.Theme) {
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
		ui.Scroll(c).Grow(1).Children(func() { m.searchRows(c, p) })
	})
}

func (m *Model) usagesPanel(c *ui.Context, p *ui.Theme) {
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
		ui.Scroll(c).Grow(1).Children(func() { m.usagesRows(c, p) })
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
