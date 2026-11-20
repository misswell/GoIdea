package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type debugView uint8

const (
	debugViewVariables debugView = iota
	debugViewConsole
	debugViewMemory
)

type debugConsoleView struct {
	searching bool
	query     string
	noFollow  bool
	wrap      bool
	expr      string
	busy      bool
	list      ui.ListState
}

func (m *Model) debugPanel(c *ui.Context, p *ui.Theme) {
	d, oi := &m.debug, oiPaletteOf(p)
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Radius(8).Clip().
		Background(oi.island).Children(func() {
		ui.Row(c).Height(36).Shrink(0).Padding(0, 10).Gap(12).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, "Debug").FontSize(13).TextColor(oi.text)
			if d.tabName != "" || d.session != 0 {
				name := d.tabName
				if name == "" {
					name = d.config
				}
				ui.Row(c).Height(24).Padding(0, 5).Gap(6).Radius(5).
					Background(oi.tabSelected).AlignItems(ui.Center).Children(func() {
					oiIconEl(c, "run/debug", 16, oi, oi.success)
					ui.Text(c, name).FontSize(13).TextColor(oi.text).SingleLine().Ellipsis("…")
					debugIcon(c, oi, "close-session", "general/close", "Close Debug Session", true, false, m.closeDebugSession)
				})
			}
			ui.Spacer(c)
			debugIcon(c, oi, "hide", "general/hide", "Hide Debug", true, false, func() { m.DockHide("debug") })
		})
		if d.err != "" {
			ui.Row(c).Shrink(0).Padding(4, 10).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, d.err).FontSize(12).TextColor(p.Danger).Grow(1)
				ui.Button(c, "Close").OnClick(func() { d.err = ""; m.invalidate() })
			})
		}
		if d.tabName == "" && d.session == 0 && !d.starting {
			ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "Not debugging").FontSize(13).TextColor(oi.textMuted)
				ui.Button(c, "Start Debugging").OnClick(m.Debug)
			})
			return
		}
		m.debugActions(c, oi)
		switch d.view {
		case debugViewConsole:
			m.debugConsole(c, p, oi)
		case debugViewMemory:
			m.debugMemory(c, p, oi)
		default:
			ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				m.debugFrames(c, p)
				m.debugVars(c, p)
			})
		}
	})
}

func debugIcon(c *ui.Context, oi oiColors, key, icon, tip string, enabled, active bool, run func()) ui.Element {
	b := ui.ButtonBase(c.Key("debug:"+key)).Size(25, 24).Shrink(0).Radius(4).
		Disabled(!enabled).AlignItems(ui.Center).Justify(ui.Center).Tooltip(tip)
	switch {
	case active:
		b.Background(oi.selected)
	case enabled && b.Hovered():
		b.Background(oi.toolbarHover)
	}
	b.Children(func() {
		el := oiIconEl(c, icon, 16, oi, oi.icon)
		if !enabled {
			el.Opacity(0.38)
		}
	})
	if run != nil && enabled && b.Clicked() {
		run()
	}
	return b
}

func (m *Model) debugActions(c *ui.Context, oi oiColors) {
	d := &m.debug
	running := d.session != 0 && !d.ended
	paused := running && d.paused
	ui.Row(c).Height(34).Shrink(0).Padding(0, 5).Gap(3).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		separator := func() { ui.Box(c).Size(1, 17).Shrink(0).Background(oi.border) }
		debugIcon(c, oi, "rerun", "run/restartDebug", "Rerun Debug", len(d.lastRaw) > 0 && !d.starting, false, m.DebugRerun)
		debugIcon(c, oi, "stop", "run/stop", "Stop", running || d.starting, false, m.StopDebug)
		separator()
		debugIcon(c, oi, "resume", "run/resume", "Resume Program", paused, false, m.DebugContinue)
		debugIcon(c, oi, "pause", "run/pause", "Pause Program", running && !paused && !d.starting && !d.pausing, false, m.DebugPause)
		debugIcon(c, oi, "over", "run/stepOver", "Step Over", paused, false, m.StepOver)
		debugIcon(c, oi, "into", "run/stepInto", "Step Into", paused, false, m.StepInto)
		debugIcon(c, oi, "out", "run/stepOut", "Step Out", paused, false, m.StepOut)
		separator()
		debugIcon(c, oi, "breakpoints", "run/viewBreakpoints", "View Breakpoints", true, false, nil).
			Menu(func(menu *ui.Menu) { m.debugBreakpointMenu(menu) })
		debugIcon(c, oi, "mute", "run/muteBreakpoints", "Mute Breakpoints", true, d.muted, m.ToggleBreakpointsMuted)
		debugIcon(c, oi, "more", "general/moreVertical", "More Debug Actions", true, false, nil).
			Menu(func(menu *ui.Menu) {
				addMenuEntries(menu, m, []menuEntry{
					action("Evaluate Expression…", "debug.evaluate", ""),
					action("Show Execution Point", "debug.showExecutionPoint", ""),
					action("Force Step Into", "debug.forceStepInto", ""),
					action("Force Step Over", "debug.forceStepOver", ""),
					action("Reload Changed Classes", "debug.reloadChangedClasses", ""),
				})
			})
		separator()
		for _, tab := range []struct {
			id    debugView
			label string
		}{{debugViewVariables, "Threads & Variables"}, {debugViewConsole, "Console"}, {debugViewMemory, "Memory"}} {
			b := ui.ButtonBase(c.Key("debug-view:"+tab.label)).Height(25).Padding(0, 8).
				Radius(4).AlignItems(ui.Center)
			if d.view == tab.id {
				b.Background(oi.selected)
			} else if b.Hovered() {
				b.Background(oi.toolbarHover)
			}
			b.Children(func() { ui.Text(c, tab.label).FontSize(13).TextColor(oi.text).SingleLine() })
			if b.Clicked() {
				d.view = tab.id
				if d.view == debugViewMemory && !d.memory.loaded {
					m.refreshDebugMemory()
				}
				m.invalidate()
			}
		}
	})
}

func (m *Model) closeDebugSession() {
	d := &m.debug
	if d.starting || d.session != 0 && !d.ended {
		m.StopDebug()
		return
	}
	visible, bps, muted, watches := d.visible, d.bps, d.muted, d.watches
	m.resetDebug()
	m.debug.visible, m.debug.bps, m.debug.muted, m.debug.watches = visible, bps, muted, watches
	m.invalidate()
}

func (m *Model) debugBreakpointMenu(menu *ui.Menu) {
	paths := make([]string, 0, len(m.debug.bps))
	for path := range m.debug.bps {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		menu.Item("No Breakpoints").Disabled(true)
		return
	}
	for _, path := range paths {
		for _, line := range m.debug.breakpointLines(path) {
			rel, err := filepath.Rel(m.Root(), path)
			if err != nil {
				rel = path
			}
			if menu.Item(fmt.Sprintf("%s:%d", rel, line+1)).Chosen() {
				m.openFile(path)
				m.revealAt(path, line, 0)
			}
		}
	}
	menu.Separator()
	if menu.Item("Remove All Breakpoints").Chosen() {
		m.RemoveAllBreakpoints()
	}
}

func (m *Model) debugConsole(c *ui.Context, p *ui.Theme, oi oiColors) {
	d, v := &m.debug, &m.debug.console
	ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(32).Shrink(0).Padding(4, 3).Gap(2).
			BorderWidth(0, 1, 0, 0).BorderColor(oi.border).Children(func() {
			debugIcon(c, oi, "find", "general/search", "Find in Console", true, v.searching, func() {
				v.searching = !v.searching
				v.query = ""
				m.invalidate()
			})
			debugIcon(c, oi, "follow", "run/stepInto", "Scroll to End", true, !v.noFollow, func() {
				v.noFollow = !v.noFollow
				if !v.noFollow {
					v.list.ScrollToEnd()
				}
				m.invalidate()
			})
			debugIcon(c, oi, "wrap", "general/softWrap", "Use Soft Wraps", true, v.wrap, func() {
				v.wrap = !v.wrap
				m.invalidate()
			})
			debugIcon(c, oi, "clear", "general/delete", "Clear Console", len(d.output) > 0, false, func() {
				d.output = nil
				d.truncated = false
				m.invalidate()
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Padding(0, 10).Children(func() {
			if v.searching {
				ui.TextInput(c.Key("debug-console-find"), &v.query).Placeholder("Find in Console").AutoFocus()
			}
			if d.truncated {
				ui.Text(c, "Earlier output was omitted").TextColor(p.TextMuted).FontSize(12)
			}
			lines := d.output
			if query := strings.ToLower(strings.TrimSpace(v.query)); v.searching && query != "" {
				lines = make([]outputLine, 0, len(d.output))
				for _, line := range d.output {
					if strings.Contains(strings.ToLower(line.text), query) {
						lines = append(lines, line)
					}
				}
			}
			ui.List(c.Key("debug-console-output"), &v.list, len(lines), func(i int) {
				line := lines[i]
				color := oi.text
				switch line.kind {
				case "stderr":
					color = oi.warning
				case "system", "console":
					color = oi.textMuted
				}
				text := ui.Text(c, line.text).MinHeight(18).FontSize(12).Font(oiFontMono).TextColor(color)
				if !v.wrap {
					text.SingleLine()
				}
			}).Grow(1).MinHeight(0).Padding(6, 4)
			ui.Row(c).Height(30).Shrink(0).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).
				BorderColor(oi.border).Children(func() {
				if !d.paused || d.frameID == 0 {
					ui.Text(c, "Evaluate in the debug console").FontSize(12).TextColor(oi.textMuted)
					return
				}
				in := ui.TextInput(c.Key("debug-console-expr"), &v.expr).Grow(1).
					Placeholder("Evaluate in the debug console")
				if in.Submitted() {
					m.evaluateDebugConsole()
				}
			})
		})
	})
}

func (m *Model) evaluateDebugConsole() {
	d, v := &m.debug, &m.debug.console
	expr := strings.TrimSpace(v.expr)
	if expr == "" || v.busy || !d.paused || d.frameID == 0 {
		return
	}
	v.expr, v.busy = "", true
	session, frame := d.session, d.frameID
	d.appendOutput("› "+expr, "system")
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_evaluate_in", map[string]any{
			"session": session, "frameId": frame, "expression": expr, "context": "repl",
		})
		var value openidea.Variable
		if err == nil {
			err = json.Unmarshal(raw, &value)
		}
		m.post(func() {
			if m.debug.session != session {
				return
			}
			v.busy = false
			if err != nil {
				d.appendOutput(err.Error(), "stderr")
			} else {
				d.appendOutput(value.Value, "stdout")
			}
			m.invalidate()
		})
	}()
}
