package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// OpenSettings opens the settings sheet on its Appearance page.
func (m *Model) OpenSettings() { m.OpenSettingsPage("appearance") }

// OpenSettingsPage opens the settings sheet on page, as OpenIDEA's Settings
// action takes the page in its argument.
func (m *Model) OpenSettingsPage(page string) {
	switch page {
	case "appearance", "keymap", "java":
	default:
		page = "appearance"
	}
	m.settingsPage = page
	if m.settingsOpen {
		m.invalidate()
		return
	}
	m.settingsOpen = true
	m.LoadJavaTools()
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

// ToggleTheme switches the window between the dark and the light theme, as
// OpenIDEA's Toggle Light/Dark Theme action does.
func (m *Model) ToggleTheme() {
	if m.prefs.Theme == "light" {
		m.SetThemePref("dark")
	} else {
		m.SetThemePref("light")
	}
}

// stepFont moves the editor font size by delta, clamped to the readable
// range, and applies it to the open editors.
func (m *Model) stepFont(delta int) {
	m.setFontSize(int(m.prefs.fontSize()) + delta)
}

// setFontSize stores an editor font size, clamped to the readable range, and
// applies it to the open editors.
func (m *Model) setFontSize(size int) {
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
	m.zen = m.prefs.Zen
	size := m.prefs.fontSize()
	for _, t := range m.tabs {
		t.state.Font = ui.Font{Family: t.state.Font.Family, Size: size}
	}
	m.applyZenGutter()
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

// settingsDialog draws Settings as IDEA does: the page tree on the left, the
// chosen page on the right. Appearance, Keymap and Java are the pages
// OpenIDEA's Settings window leads with.
func (m *Model) settingsDialog(c *ui.Context, p *ui.Theme) {
	if !m.settingsOpen {
		return
	}
	open := m.settingsOpen
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(760).Height(520).Gap(0).Children(func() {
			ui.Row(c).Height(46).Padding(0, 14).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Settings").FontSize(14).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				ui.Button(c, "Close").OnClick(m.closeSettings)
			})
			ui.Row(c).Grow(1).Children(func() {
				ui.Column(c).Width(200).Gap(2).Padding(0, 8).Children(func() {
					m.settingsNavItem(c, p, "appearance", "Appearance")
					m.settingsNavItem(c, p, "keymap", "Keymap")
					m.settingsNavItem(c, p, "java", "Java")
				})
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).Gap(10).Padding(0, 14).Children(func() {
						switch m.settingsPage {
						case "keymap":
							m.settingsKeymapPage(c, p)
						case "java":
							m.settingsJavaPage(c, p)
						default:
							m.settingsAppearancePage(c, p)
						}
					})
				})
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.closeSettings()
	}
}

func (m *Model) settingsNavItem(c *ui.Context, p *ui.Theme, page, label string) {
	r := ui.Row(c.Key("settings:"+page)).Height(28).Padding(0, 10).AlignItems(ui.Center)
	if m.settingsPage == page {
		r.Background(p.SurfaceHover)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.Text).SingleLine()
	})
	if r.Clicked() {
		m.settingsPage = page
		m.invalidate()
	}
}

func (m *Model) settingsAppearancePage(c *ui.Context, p *ui.Theme) {
	ui.Text(c, "Appearance").FontSize(13).Bold().TextColor(p.Text).SingleLine()
	ui.Text(c, "Theme").FontSize(11).TextColor(p.TextMuted).SingleLine()
	for _, o := range [][2]string{
		{"system", "Use System Settings"},
		{"light", "Light"},
		{"dark", "Dark"},
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
	ui.Text(c, "Editor").FontSize(11).TextColor(p.TextMuted).SingleLine()
	ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Font size").FontSize(12).TextColor(p.Text)
		ui.Spacer(c)
		ui.Button(c, "−").OnClick(func() { m.stepFont(-1) })
		ui.Text(c, fmt.Sprintf("%.0f", m.prefs.fontSize())).FontSize(12).
			TextColor(p.Text).Width(28)
		ui.Button(c, "+").OnClick(func() { m.stepFont(1) })
	})
	ui.Text(c, fmt.Sprintf("Range %d–%d, applied to open editors at once", minFontSize, maxFontSize)).
		FontSize(10.5).TextColor(p.TextMuted).SingleLine()
	ui.Text(c, "Distraction Free Mode").FontSize(11).TextColor(p.TextMuted).SingleLine()
	ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Text width").FontSize(12).TextColor(p.Text)
		ui.Spacer(c)
		ui.Button(c, "−").OnClick(func() { m.StepZenWidth(-5) })
		ui.Text(c, fmt.Sprintf("%d", m.prefs.zenWidth())).FontSize(12).
			TextColor(p.Text).Width(36)
		ui.Button(c, "+").OnClick(func() { m.StepZenWidth(5) })
	})
	ui.Text(c, fmt.Sprintf("Columns of text, %d–%d", minZenWidth, maxZenWidth)).
		FontSize(10.5).TextColor(p.TextMuted).SingleLine()
}

func (m *Model) settingsKeymapPage(c *ui.Context, p *ui.Theme) {
	ui.Text(c, "Keymap").FontSize(13).Bold().TextColor(p.Text).SingleLine()
	ui.Text(c, "Keymap: "+keymapPresetTitle).FontSize(12).TextColor(p.Text).SingleLine()
	ui.Text(c, "The actions this build carries, with the keys of the IntelliJ macOS map.").
		FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
	for _, cmd := range commands() {
		if cmd.shortcut == "" {
			continue
		}
		ui.Row(c.Key("keymap:"+cmd.id)).Height(24).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cmd.title).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			ui.Text(c, cmd.shortcut).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		})
	}
}

func (m *Model) settingsJavaPage(c *ui.Context, p *ui.Theme) {
	ui.Text(c, "Java").FontSize(13).Bold().TextColor(p.Text).SingleLine()
	m.javaToolsSection(c, p)
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

// javaToolsSection draws the Java language server / debugger rows and the
// install action, so a fresh data directory can get a working Java toolchain
// without leaving the app.
func (m *Model) javaToolsSection(c *ui.Context, p *ui.Theme) {
	ui.Text(c, "Java 语言服务与调试器").FontSize(11).TextColor(p.TextMuted).SingleLine()
	if m.Root() == "" {
		ui.Text(c, "打开一个 Java 项目后可安装").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		return
	}
	busy := m.toolsBusy != ""
	for _, t := range m.javaTools {
		t := t
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, t.Display).FontSize(12).TextColor(p.Text).Width(230).SingleLine().Ellipsis("…")
			label := javaToolStatus(t.Status)
			color := p.TextMuted
			if t.Status != "missing" {
				color = p.Success
			}
			ui.Text(c, label).FontSize(11).TextColor(color).SingleLine()
			if t.Version != "" {
				ui.Text(c, t.Version).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
			ui.Spacer(c)
			if t.Status == "missing" && t.CanInstall && !busy {
				ui.Button(c, "安装").OnClick(func() { m.InstallJavaToolchain() })
			}
			if t.Status == "missing" && !t.CanInstall && t.Reason != "" && !busy {
				ui.Text(c, t.Reason).FontSize(10.5).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
		})
	}
	switch {
	case m.toolsLoading:
		ui.Text(c, "正在读取工具状态…").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
	case !m.toolsLoaded:
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "尚未读取工具状态").FontSize(10.5).TextColor(p.TextMuted)
			ui.Spacer(c)
			ui.Button(c, "检查").OnClick(func() { m.LoadJavaTools() })
		})
	case busy:
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.toolsBusy).FontSize(11).TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			ui.Button(c, "取消").OnClick(func() { m.CancelToolInstall() })
		})
	}
	if busy && m.toolsProgress != "" {
		ui.Text(c, m.toolsProgress).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
	}
	if len(m.toolsLog) > 0 {
		from := max(0, len(m.toolsLog)-2)
		for _, line := range m.toolsLog[from:] {
			ui.Text(c, line).FontSize(10).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
	}
	if m.toolsErr != "" {
		ui.Text(c, m.toolsErr).FontSize(10.5).TextColor(p.Danger).SingleLine().Ellipsis("…")
	}
	if !busy && len(m.javaTools) > 0 && len(javaToolsMissing(m.javaTools)) == 0 {
		ui.Text(c, "Java 编辑、构建与调试可用").FontSize(10.5).TextColor(p.Success).SingleLine()
	}
}
