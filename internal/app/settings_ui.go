package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// OpenSettings opens the settings sheet.
func (m *Model) OpenSettings() {
	if m.settingsOpen {
		return
	}
	m.settingsOpen = true
	m.invalidate()
}

func (m *Model) closeSettings() {
	if !m.settingsOpen {
		return
	}
	m.settingsOpen = false
	m.invalidate()
}

// SetThemePref stores the appearance choice and redraws the window in it at
// once; "system" follows the system's appearance.
func (m *Model) SetThemePref(theme string) {
	if theme == "system" {
		theme = ""
	}
	if m.prefs.Theme == theme {
		return
	}
	m.prefs.Theme = theme
	m.applyPreferences()
	m.savePreferences()
	m.status = "外观已更新"
	m.invalidate()
}

// stepFont moves the editor font size by delta, clamped to the readable
// range, and applies it to the open editors.
func (m *Model) stepFont(delta int) {
	size := int(m.prefs.fontSize()) + delta
	size = max(minFontSize, min(size, maxFontSize))
	if float32(size) == m.prefs.fontSize() {
		return
	}
	m.prefs.FontSize = float32(size)
	m.applyPreferences()
	m.savePreferences()
	m.status = fmt.Sprintf("编辑器字号 %d", size)
	m.invalidate()
}

// applyPreferences puts the stored settings on the model: the theme the
// window draws with and the font size of every open editor.
func (m *Model) applyPreferences() {
	m.theme = m.prefs.themePreference()
	size := m.prefs.fontSize()
	for _, t := range m.tabs {
		t.state.Font = ui.Font{Family: t.state.Font.Family, Size: size}
	}
}

// savePreferences writes the settings; a failed write is reported rather than
// silently keeping the choice only for this run.
func (m *Model) savePreferences() {
	if m.cfg.SettingsPath == "" {
		return
	}
	if err := m.prefs.Save(m.cfg.SettingsPath); err != nil {
		m.status = "保存设置失败：" + err.Error()
	}
}

// settingsDialog draws the settings sheet: appearance and editor font size.
func (m *Model) settingsDialog(c *ui.Context, p *ui.Theme) {
	if !m.settingsOpen {
		return
	}
	open := m.settingsOpen
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Gap(10).Children(func() {
			ui.Text(c, "设置").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "外观").FontSize(11).TextColor(p.TextMuted).SingleLine()
			for _, o := range [][2]string{
				{"system", "跟随系统"},
				{"light", "浅色"},
				{"dark", "深色"},
			} {
				o := o
				current := m.prefs.Theme == o[0] || (o[0] == "system" && m.prefs.Theme == "")
				r := ui.Row(c.Key("theme:"+o[0])).Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center)
				if current || r.Hovered() {
					r.Background(p.SurfaceHover)
				}
				r.Children(func() {
					mark := "○"
					if current {
						mark = "●"
					}
					ui.Text(c, mark).FontSize(12).TextColor(p.Accent).Width(16)
					ui.Text(c, o[1]).FontSize(12).TextColor(p.Text)
				})
				if r.Clicked() {
					m.SetThemePref(o[0])
				}
			}
			ui.Text(c, "编辑器").FontSize(11).TextColor(p.TextMuted).SingleLine()
			ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "字号").FontSize(12).TextColor(p.Text)
				ui.Spacer(c)
				ui.Button(c, "−").OnClick(func() { m.stepFont(-1) })
				ui.Text(c, fmt.Sprintf("%.0f", m.prefs.fontSize())).FontSize(12).
					TextColor(p.Text).Width(28)
				ui.Button(c, "+").OnClick(func() { m.stepFont(1) })
			})
			ui.Text(c, fmt.Sprintf("范围 %d–%d，打开的编辑器立即生效", minFontSize, maxFontSize)).
				FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		})
	})
	if dlg.Dismissed() || !open {
		m.closeSettings()
	}
}

// SmokeSettings drives the settings sheet offscreen: theme and font changes go
// through the same calls the sheet's controls make, and the stored file is
// read back to prove the choice reached the disk.
func (m *Model) SmokeSettings() map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		wasTheme, wasFont := m.prefs.Theme, m.prefs.FontSize
		m.OpenSettings()
		out["opened"] = m.settingsOpen
		m.SetThemePref("dark")
		out["theme"] = m.prefs.Theme
		out["darkTheme"] = m.theme != nil
		if stored := LoadPreferences(m.cfg.SettingsPath); stored.Theme != "dark" {
			out["storedTheme"] = stored.Theme
		} else {
			out["storedTheme"] = "dark"
		}
		before := m.prefs.fontSize()
		m.stepFont(1)
		out["fontBefore"] = before
		out["fontAfter"] = m.prefs.fontSize()
		for i := 0; i < 40; i++ {
			m.stepFont(1)
		}
		out["fontMax"] = m.prefs.fontSize()
		if t := m.tab(); t != nil {
			out["editorFont"] = t.state.Font.Size
		}
		m.closeSettings()
		out["closed"] = !m.settingsOpen
		m.prefs.Theme, m.prefs.FontSize = wasTheme, wasFont
		m.applyPreferences()
		m.savePreferences()
	})
	return out
}
