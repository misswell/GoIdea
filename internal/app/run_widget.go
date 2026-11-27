package app

// The title bar's Run widget, ported from OpenIDEA's RunWidget: the
// configuration chooser, Run and Debug for the selected configuration, Stop
// and Rerun while a session is live, and the More Actions menu. Every control
// runs a command of the registry, so the widget, the menu bar, the palette and
// the shortcuts share one implementation.

import (
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// runWidget draws OpenIDEA's RunWidget.
func (m *Model) runWidget(c *ui.Context, p *ui.Theme, oi oiColors) {
	// The configurations arrive after the project is trusted; ask the first
	// time the widget draws so its chooser is filled as early as OpenIDEA's.
	if m.run.trusted && !m.run.loaded && !m.run.loading {
		m.loadRunConfigs()
	}
	ui.Row(c.Key("title:run-widget")).Gap(4).AlignItems(ui.Center).Children(func() {
		m.runConfigChooser(c, p, oi)
		cfg := m.run.current()
		if cfg == nil {
			cfg = m.run.inline
		}
		selectedID := ""
		if cfg != nil {
			selectedID = cfg.ID
		}
		if m.run.currentFile {
			selectedID = "current-file"
		}
		live := (m.run.running || m.run.awaiting || m.run.busy) && selectedID != "" &&
			m.run.lastID == selectedID
		starting := live && (m.run.busy || m.run.awaiting && !m.run.running)
		debugging := m.debug.session != 0 && !m.debug.ended && selectedID != "" &&
			m.debug.lastID == selectedID
		debugStarting := m.debug.starting && selectedID != "" && m.debug.lastID == selectedID
		name := m.run.selectedLabel()
		if starting || debugStarting {
			action := "Run"
			if debugStarting {
				action = "Debug"
			}
			tk := oiTokensFor(m, p)
			ui.Box(c).Size(tk.StripeButton, tk.StripeButton).Shrink(0).
				AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Spinner(c).Size(18, 18).TextColor(oi.textMuted).
					Label("Starting " + action)
			})
			m.runWidgetButton(c, p, oi, "stop", titled("Stop", m.debug.tabName), "run/stop_stroke", 16, oi.danger, m.StopDebug)
			return
		}
		if live || debugging {
			// While a session runs the widget offers Rerun, and Resume when
			// the debugger is paused, as OpenIDEA does. The solid buttons
			// carry the white stroke drawings upstream swaps in.
			if debugging && m.debug.paused {
				m.runWidgetButton(c, p, oi, "resume", "Resume Program", "run/resume", 20, ui.Transparent, func() {
					m.RunCommand("run.continue")
				})
			}
			rerunIcon := "run/rerun_stroke"
			if debugging {
				rerunIcon = "run/restartDebug_stroke"
			}
			m.runWidgetButton(c, p, oi, "rerun", titled("Rerun", name), rerunIcon, 16, oi.success, func() {
				if debugging {
					m.DebugRerun()
					return
				}
				m.Rerun()
			})
			m.runWidgetButton(c, p, oi, "stop", titled("Stop", name), "run/stop_stroke", 16, oi.danger, func() {
				m.RunCommand("run.stop")
			})
			return
		}
		if m.run.running || m.run.awaiting || m.run.busy {
			m.runWidgetButton(c, p, oi, "stop", "Stop Process", "run/stop_stroke", 16, oi.danger, func() {
				m.RunCommand("run.stop")
			})
		}
		debugOnly := cfg != nil && isRemoteDebugKind(cfg.Kind)
		runTip := titled("Run", name)
		if debugOnly {
			runTip = "'" + name + "' can only be debugged"
		}
		m.runWidgetButtonDisabled(c, p, oi, "run", runTip, "run/run", 20, ui.Transparent,
			debugOnly || (!m.run.currentFile && cfg == nil), func() {
				m.RunCommand("run.start")
			})
		m.runWidgetButtonDisabled(c, p, oi, "debug", titled("Debug", name), "run/debug", 20, ui.Transparent,
			!m.run.currentFile && cfg == nil, func() {
				m.RunCommand("run.debug")
			})
	})
	m.runMoreButton(c, p, oi)
}

// titled writes IDEA's tooltip for a configuration: Run 'name', or Run while
// no configuration is selected.
func titled(verb, name string) string {
	if name == "" {
		return verb
	}
	return verb + " '" + name + "'"
}

// runConfigChooser is the chooser button: the icon, the selected
// configuration's name and the chevron, with the configurations in its menu.
func (m *Model) runConfigChooser(c *ui.Context, p *ui.Theme, oi oiColors) {
	label := "Select Configuration"
	if selected := m.run.selectedLabel(); selected != "" {
		label = selected
	}
	w := ui.ButtonBase(c.Key("title:run-config")).Height(30).Padding(0, 8).Gap(4).
		Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if w.Hovered() {
		w.Background(oi.toolbarHover)
	}
	w.Tooltip("Run/Debug Configurations")
	w.Children(func() {
		oiIconEl(c, "run/run", 16, oi, oi.icon)
		ui.Text(c, label).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
		oiIconEl(c, "general/chevronDown", 14, oi, oi.textSubtle)
	})
	w.Menu(func(menu *ui.Menu) {
		if menu.Item("Choose Run/Debug Configuration…").Chosen() {
			m.RunCommand("run.chooseConfig")
		}
		menu.Separator()
		current := menu.Item("Current File")
		current.Checked(m.run.currentFile)
		if current.Chosen() {
			m.selectRunConfig("current-file")
			m.invalidate()
		}
		for i, cfg := range m.run.configs {
			i, cfg := i, cfg
			it := menu.Item(cfg.Name)
			it.Checked(!m.run.currentFile && m.run.inline == nil && i == m.run.sel)
			if it.Chosen() {
				m.selectRunConfig(cfg.ID)
				m.invalidate()
			}
		}
		for _, cfg := range m.run.temporaries {
			cfg := cfg
			if cfg == nil {
				continue
			}
			it := menu.Item(cfg.Name)
			it.Checked(m.run.inline != nil && m.run.inline.ID == cfg.ID)
			if it.Chosen() {
				m.selectRunConfig(cfg.ID)
				m.invalidate()
			}
		}
		if len(m.run.configs) == 0 && len(m.run.temporaries) == 0 {
			menu.Item("No configurations").Disabled(true)
		}
		menu.Separator()
		if menu.Item("Edit Configurations…").Chosen() {
			m.RunCommand("run.editConfigs")
		}
		if menu.Item("Reload Run Configurations").Chosen() {
			m.RunCommand("run.reloadConfigs")
		}
	})
}

// runWidgetButton is one 30px control of the widget, solid in the colour of
// its state while a session is live and transparent otherwise. icon is the
// drawing under expui/run and size its DIP size, as the React widget sets.
func (m *Model) runWidgetButton(c *ui.Context, p *ui.Theme, oi oiColors, key, tip, icon string, size float32, fill ui.Color, onClick func()) {
	m.runWidgetButtonDisabled(c, p, oi, key, tip, icon, size, fill, false, onClick)
}

func (m *Model) runWidgetButtonDisabled(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	key, tip, icon string,
	size float32,
	fill ui.Color,
	disabled bool,
	onClick func(),
) {
	tk := oiTokensFor(m, p)
	b := ui.ButtonBase(c.Key("title:run-"+key)).Size(tk.StripeButton, tk.StripeButton).
		Radius(6).Disabled(disabled).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if disabled {
		// Keep the disabled control in the same footprint and let its drawing
		// fade like the other title-bar icon buttons.
	} else if fill != ui.Transparent {
		b.Background(fill)
	} else if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	label := tip
	switch label {
	case "Run":
		label = "Run configuration"
	case "Debug":
		label = "Debug configuration"
	}
	b.Tooltip(tip).Description(tip).Label(label)
	b.Children(func() {
		el := oiIconEl(c, icon, size, oi, oi.icon)
		if disabled {
			el.Opacity(0.38)
		}
	})
	if !disabled && b.Clicked() {
		onClick()
	}
}

// debugOnlyRunConfiguration reports the remote attach/listen kinds OpenIDEA
// marks as debug-only. Running one would do nothing because the target process
// belongs to another machine or another command line.
func debugOnlyRunConfiguration(cfg *openidea.RunConfig) bool {
	return cfg != nil && isRemoteDebugKind(cfg.Kind)
}

// runMoreButton is the widget's More Actions menu.
func (m *Model) runMoreButton(c *ui.Context, p *ui.Theme, oi oiColors) {
	tk := oiTokensFor(m, p)
	b := ui.ButtonBase(c.Key("title:run-more")).Size(tk.StripeButton, tk.StripeButton).
		Radius(6).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip("More Actions")
	b.Children(func() {
		oiIconEl(c, "general/moreVertical", 20, oi, oi.icon)
	})
	b.Menu(func(menu *ui.Menu) {
		hasExecutors := false
		if cfg := m.run.current(); cfg != nil {
			if profilerSupports(*cfg) {
				hasExecutors = true
				if menu.Item("Profile '" + cfg.Name + "'").Chosen() {
					m.RunCommand("run.profile")
				}
			}
			if allocationProfilerSupports(*cfg) {
				hasExecutors = true
				if menu.Item("Profile '" + cfg.Name + "' Allocations").Chosen() {
					m.RunCommand("run.profileAllocations")
				}
				if menu.Item("Profile '" + cfg.Name + "' Live Objects").Chosen() {
					m.RunCommand("run.profileLiveObjects")
				}
			}
		}
		if hasExecutors {
			menu.Separator()
		}
		if menu.Item("Run Context Configuration").Chosen() {
			m.RunCommand("run.runContext")
		}
		if menu.Item("Debug Context Configuration").Chosen() {
			m.RunCommand("run.debugContext")
		}
		menu.Separator()
		if menu.Item("Edit Configurations…").Chosen() {
			m.RunCommand("run.editConfigs")
		}
		if menu.Item("Reload Run Configurations").Chosen() {
			m.RunCommand("run.reloadConfigs")
		}
	})
}
