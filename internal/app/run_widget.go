package app

// The title bar's Run widget, ported from OpenIDEA's RunWidget: the
// configuration chooser, Run and Debug for the selected configuration, Stop
// and Rerun while a session is live, and the More Actions menu. Every control
// runs a command of the registry, so the widget, the menu bar, the palette and
// the shortcuts share one implementation.

import "github.com/egoist/mygo/ui"

// runWidget draws OpenIDEA's RunWidget.
func (m *Model) runWidget(c *ui.Context, p *ui.Theme, oi oiColors) {
	// The configurations arrive after the project is trusted; ask the first
	// time the widget draws so its chooser is filled as early as OpenIDEA's.
	if m.run.trusted && !m.run.loaded && !m.run.loading {
		m.loadRunConfigs()
	}
	ui.Row(c.Key("title:run-widget")).Gap(4).AlignItems(ui.Center).Children(func() {
		m.runConfigChooser(c, p, oi)
		live := m.run.running || m.run.awaiting
		debugging := m.debug.session != 0 && !m.debug.ended
		name := ""
		if cfg := m.run.current(); cfg != nil {
			name = cfg.Name
		}
		if live || debugging {
			// While a session runs the widget offers Rerun, and Resume when
			// the debugger is paused, as OpenIDEA does.
			if debugging && m.debug.paused {
				m.runWidgetButton(c, p, oi, "resume", "Resume Program", iconRun, ui.Transparent, func() {
					m.RunCommand("run.continue")
				})
			}
			rerun := "run.start"
			if debugging {
				rerun = "run.debug"
			}
			m.runWidgetButton(c, p, oi, "rerun", titled("Rerun", name), iconRun, oi.success, func() {
				m.RunCommand(rerun)
			})
			m.runWidgetButton(c, p, oi, "stop", titled("Stop", name), iconStop, oi.danger, func() {
				m.RunCommand("run.stop")
			})
			return
		}
		m.runWidgetButton(c, p, oi, "run", titled("Run", name), iconRun, ui.Transparent, func() {
			m.RunCommand("run.start")
		})
		m.runWidgetButton(c, p, oi, "debug", titled("Debug", name), iconDebug, ui.Transparent, func() {
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
	if cfg := m.run.current(); cfg != nil {
		label = cfg.Name
	}
	w := ui.ButtonBase(c.Key("title:run-config")).Height(30).Padding(0, 8).Gap(4).
		Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if w.Hovered() {
		w.Background(oi.toolbarHover)
	}
	w.Tooltip("Run/Debug Configurations")
	w.Children(func() {
		ui.Icon(c, iconRun).Size(16, 16).TextColor(oi.textSecondary)
		ui.Text(c, label).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
		ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(oi.textSubtle)
	})
	w.Menu(func(menu *ui.Menu) {
		for i, cfg := range m.run.configs {
			i, cfg := i, cfg
			it := menu.Item(cfg.Name)
			it.Checked(i == m.run.sel)
			if it.Chosen() {
				m.run.sel = i
				m.invalidate()
			}
		}
		if len(m.run.configs) == 0 {
			menu.Item("No configurations").Disabled(true)
		}
		menu.Separator()
		if menu.Item("Reload Run Configurations").Chosen() {
			m.RunCommand("run.reloadConfigs")
		}
	})
}

// runWidgetButton is one 30px control of the widget, solid in the colour of
// its state while a session is live and transparent otherwise.
func (m *Model) runWidgetButton(c *ui.Context, p *ui.Theme, oi oiColors, key, tip string, icon *ui.SVG, fill ui.Color, onClick func()) {
	b := ui.ButtonBase(c.Key("title:run-"+key)).Size(oiStripeButton, oiStripeButton).
		Radius(6).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	tone := oi.textSecondary
	if fill != ui.Transparent {
		b.Background(fill)
		tone = ui.Hex("#ffffff")
	} else if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip(tip)
	b.Children(func() {
		ui.Icon(c, icon).Size(18, 18).TextColor(tone)
	})
	if b.Clicked() {
		onClick()
	}
}

// runMoreButton is the widget's More Actions menu.
func (m *Model) runMoreButton(c *ui.Context, p *ui.Theme, oi oiColors) {
	b := ui.ButtonBase(c.Key("title:run-more")).Size(oiStripeButton, oiStripeButton).
		Radius(6).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	b.Tooltip("More Actions")
	b.Children(func() {
		ui.Icon(c, iconWelcomeMore).Size(18, 18).TextColor(oi.textSecondary)
	})
	b.Menu(func(menu *ui.Menu) {
		name := ""
		if cfg := m.run.current(); cfg != nil {
			name = cfg.Name
		}
		if menu.Item(titled("Run", name)).Chosen() {
			m.RunCommand("run.start")
		}
		if menu.Item(titled("Debug", name)).Chosen() {
			m.RunCommand("run.debug")
		}
		menu.Separator()
		if menu.Item("Show Run Window").Chosen() {
			m.RunCommand("view.run")
		}
		menu.Separator()
		if menu.Item("Reload Run Configurations").Chosen() {
			m.RunCommand("run.reloadConfigs")
		}
	})
}
