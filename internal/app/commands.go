package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// appCommand is one entry of the command palette. keywords widen what the
// query matches, so 快速打开 is found by "quick" and 显示 Git 更改 by "git".
type appCommand struct {
	id       string
	title    string
	group    string
	shortcut string
	keywords string
	run      func(*Model)
}

// commands is every action the palette offers, in the order it lists them.
// The registry is the palette's only source, so the menu stays the place for
// platform roles while this is where actions are discovered.
func commands() []appCommand {
	return []appCommand{
		{"project.open", "打开项目…", "文件", "Cmd+O", "open project folder", func(m *Model) { m.ChooseProject() }},
		{"file.quickOpen", "快速打开…", "文件", "Cmd+P", "quick open file goto recent", func(m *Model) { m.OpenQuickFiles() }},
		{"file.save", "保存", "文件", "Cmd+S", "save write", func(m *Model) { m.SaveActive() }},
		{"file.close", "关闭标签页", "文件", "Cmd+W", "close tab", func(m *Model) { m.closeTab(m.active) }},
		{"vcs.show", "显示 Git 更改", "Git", "Cmd+9", "git vcs changes status", func(m *Model) { m.ShowVCS() }},
		{"vcs.log", "显示 Git 日志", "Git", "", "git vcs log history commits", func(m *Model) { m.ShowLog() }},
		{"vcs.commit", "提交…", "Git", "Cmd+K", "git commit message stage", func(m *Model) { m.OpenCommit() }},
		{"vcs.diff", "与 HEAD 比较", "Git", "Cmd+Shift+D", "git diff compare head", func(m *Model) { m.CompareActive() }},
		{"search.project", "在项目中查找", "搜索", "Cmd+Shift+F", "search find in project", func(m *Model) { m.ShowSearch() }},
		{"view.project", "显示项目", "视图", "Cmd+1", "project tree files sidebar", func(m *Model) { m.ShowProject() }},
		{"view.terminal", "显示终端", "视图", "Alt+F12", "terminal shell", func(m *Model) { m.ShowTerm() }},
		{"view.run", "显示运行窗口", "视图", "Cmd+4", "run output console", func(m *Model) { m.ShowRun() }},
		{"view.debug", "显示调试窗口", "视图", "Cmd+5", "debug variables frames", func(m *Model) { m.ShowDebug() }},
		{"view.problems", "显示问题", "视图", "Cmd+6", "problems diagnostics errors warnings inspections", func(m *Model) { m.ShowProblems() }},
		{"view.structure", "显示结构", "视图", "Cmd+7", "structure outline symbols members file", func(m *Model) { m.ShowStructure() }},
		{"view.hideActive", "隐藏当前工具窗口", "视图", "Shift+Escape", "hide tool window active", func(m *Model) { m.HideActiveToolWindow() }},
		{"view.hideAll", "隐藏全部工具窗口", "视图", "Cmd+Shift+F12", "hide all tool windows restore", func(m *Model) { m.HideAllToolWindows() }},
		{"view.jumpLast", "跳转到上一个工具窗口", "视图", "F12", "jump last tool window", func(m *Model) { m.JumpToLastToolWindow() }},
		{"view.commit", "提交窗口", "视图", "Cmd+0", "commit tool window git", func(m *Model) { m.OpenCommit() }},
		{"problems.next", "下一个问题", "视图", "Cmd+Alt+Down", "problems next occurrence navigate", func(m *Model) { m.StepProblem(1) }},
		{"problems.prev", "上一个问题", "视图", "Cmd+Alt+Up", "problems previous occurrence navigate", func(m *Model) { m.StepProblem(-1) }},
		{"view.database", "数据库…", "视图", "", "database sql sqlite postgres mysql query", func(m *Model) { m.OpenDatabase() }},
		{"code.intentions", "意图操作…", "代码", "Alt+Enter", "intention action quickfix", func(m *Model) { m.ShowIntentions() }},
		{"code.symbols", "转到符号…", "代码", "Cmd+Shift+O", "go to symbol navigate", func(m *Model) { m.OpenSymbols() }},
		{"code.usages", "查找用法", "代码", "Alt+F7", "find usages references", func(m *Model) { m.FindUsages() }},
		{"code.rename", "重命名符号", "重构", "Shift+F6", "rename symbol refactor", func(m *Model) { m.RenameActive() }},
		{"code.undoRefactor", "撤销重构", "重构", "Cmd+Alt+Z", "undo refactor", func(m *Model) { m.UndoRefactor() }},
		{"code.redoRefactor", "重做重构", "重构", "Cmd+Alt+Shift+Z", "redo refactor", func(m *Model) { m.RedoRefactor() }},
		{"code.format", "格式化代码", "代码", "Cmd+Alt+L", "format code", func(m *Model) { m.FormatActive() }},
		{"run.start", "运行", "运行", "F5", "run main class", func(m *Model) { m.Run() }},
		{"run.debug", "调试", "运行", "F9", "debug session", func(m *Model) { m.Debug() }},
		{"run.stop", "停止", "运行", "Shift+F5", "stop terminate", func(m *Model) { m.StopRun(); m.StopDebug() }},
		{"run.breakpoint", "切换断点", "运行", "Cmd+F8", "toggle breakpoint", func(m *Model) { m.ToggleBreakpoint() }},
		{"run.stepOver", "步过", "运行", "F8", "step over", func(m *Model) { m.StepOver() }},
		{"run.stepInto", "步入", "运行", "F7", "step into", func(m *Model) { m.StepInto() }},
		{"run.stepOut", "步出", "运行", "Shift+F8", "step out", func(m *Model) { m.StepOut() }},
		{"run.continue", "继续", "运行", "", "continue resume", func(m *Model) { m.DebugContinue() }},
		{"run.pause", "暂停", "运行", "", "pause suspend", func(m *Model) { m.DebugPause() }},
		{"run.trust", "信任此项目并启用运行", "运行", "", "trust project", func(m *Model) { m.TrustProject() }},
		{"build.buildProject", "构建项目", "构建", "Cmd+F9", "build project compile make mvn gradle cmake", func(m *Model) { m.BuildProject() }},
		{"build.buildModule", "构建模块", "构建", "Cmd+Shift+F9", "build module compile file", func(m *Model) { m.BuildModule() }},
		{"build.rebuildProject", "重新构建项目", "构建", "", "rebuild clean build", func(m *Model) { m.RebuildProject() }},
		{"build.buildArtifacts", "构建工件", "构建", "", "artifacts package assemble jar release", func(m *Model) { m.BuildArtifacts() }},
		{"build.stop", "停止构建", "构建", "", "stop cancel build", func(m *Model) { m.StopBuild() }},
		{"view.build", "显示构建窗口", "视图", "", "build output console", func(m *Model) { m.ShowBuild() }},
		{"java.tools", "安装 Java 语言服务与调试器", "运行", "", "java jdtls language server debugger install toolchain", func(m *Model) { m.InstallJavaToolchain() }},
		{"app.settings", "设置…", "文件", "Cmd+,", "settings preferences theme font appearance", func(m *Model) { m.OpenSettings() }},
	}
}

// commandPicker is the open command palette: the query and the commands it
// still admits.
type commandPicker struct {
	open  bool
	query string
	items []appCommand
	sel   int
}

// OpenCommands opens the command palette over the window.
func (m *Model) OpenCommands() {
	if m.palette != nil {
		return
	}
	m.palette = &commandPicker{open: true}
	m.palette.refresh()
	m.invalidate()
}

func (m *Model) closeCommands() {
	if m.palette == nil {
		return
	}
	m.palette = nil
	m.invalidate()
}

// refresh filters the registry against the typed query and keeps the chosen
// row in range.
func (p *commandPicker) refresh() {
	all := commands()
	p.items = p.items[:0]
	for _, cmd := range all {
		if commandMatches(cmd, p.query) {
			p.items = append(p.items, cmd)
		}
	}
	p.sel = clampSymbolSel(p.sel, len(p.items))
}

// commandMatches reports whether the query is a subsequence of what the
// command is called or the words it is known by.
func commandMatches(cmd appCommand, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	q := []rune(query)
	hay := strings.ToLower(cmd.title + " " + cmd.group + " " + cmd.keywords)
	i := 0
	for _, r := range hay {
		if r == q[i] {
			i++
			if i == len(q) {
				return true
			}
		}
	}
	return false
}

// stepCommand moves the chosen row by delta, stopping at the ends.
func (m *Model) stepCommand(delta int) {
	p := m.palette
	if p == nil || len(p.items) == 0 {
		return
	}
	p.sel = clampSymbolSel(p.sel+delta, len(p.items))
	m.invalidate()
}

// runSelectedCommand runs the chosen command and closes the palette first,
// since the command may open a dialog of its own.
func (m *Model) runSelectedCommand() {
	p := m.palette
	if p == nil || len(p.items) == 0 {
		m.invalidate()
		return
	}
	cmd := p.items[clampSymbolSel(p.sel, len(p.items))]
	m.closeCommands()
	cmd.run(m)
}

// commandsDialog draws the command palette.
func (m *Model) commandsDialog(c *ui.Context, p *ui.Theme) {
	pal := m.palette
	if pal == nil {
		return
	}
	open := pal.open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(600).Gap(8).Children(func() {
			ui.Text(c, "命令").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &pal.query).AutoFocus()
			in.OnChange(func() { pal.refresh() })
			in.OnShortcut(0, ui.KeyDown, func() { m.stepCommand(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepCommand(-1) })
			if in.Submitted() {
				m.runSelectedCommand()
			}
			hint := "输入命令名，Enter 执行"
			switch {
			case strings.TrimSpace(pal.query) != "" && len(pal.items) == 0:
				hint = "没有匹配的命令"
			case strings.TrimSpace(pal.query) != "":
				hint = fmt.Sprintf("%d 个命令", len(pal.items))
			}
			ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			if len(pal.items) == 0 {
				return
			}
			ui.Scroll(c).Height(380).Children(func() {
				ui.Column(c).Children(func() {
					for i, cmd := range pal.items {
						i, cmd := i, cmd
						r := ui.Row(c.Key("cmd:"+cmd.id)).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == pal.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							ui.Text(c, cmd.title).FontSize(12).TextColor(p.Text).
								SingleLine().Ellipsis("…")
							ui.Spacer(c)
							ui.Text(c, cmd.group).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
							if cmd.shortcut != "" {
								ui.Text(c, cmd.shortcut).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
							}
						})
						if r.Clicked() {
							pal.sel = i
							m.runSelectedCommand()
						}
					}
				})
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeCommands()
	}
}

// SmokeCommands drives the command palette offscreen: listing, filtering and
// running a command through the same path a keystroke takes.
func (m *Model) SmokeCommands() map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		m.OpenCommands()
		if m.palette == nil {
			out["error"] = "命令面板没有打开"
			return
		}
		out["all"] = len(m.palette.items)
		m.palette.query = "git"
		m.palette.refresh()
		out["git"] = len(m.palette.items)
		if len(m.palette.items) > 0 {
			out["first"] = m.palette.items[0].title
		}
		m.palette.query = "显示 Git 更改"
		m.palette.refresh()
		m.runSelectedCommand()
		out["sideMode"] = m.sideMode
		out["closed"] = m.palette == nil
	})
	return out
}
