package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// OpenSettings opens the settings sheet on its Appearance page.
func (m *Model) OpenSettings() { m.OpenSettingsPage(settingsPageAppearance) }

// OpenSettingsPage opens the settings sheet on page, as OpenIDEA's Settings
// action takes the page in its argument. An id this build does not register
// falls back to Appearance, which it always carries.
func (m *Model) OpenSettingsPage(page string) {
	if _, ok := settingsPageByID(page); !ok {
		page = settingsPageAppearance
	}
	if m.settingsOpen {
		m.settingsSelectPage(page)
		if m.settingsPage == settingsPageLanguageServers {
			m.loadLanguageServerSettings()
		}
		m.invalidate()
		return
	}
	m.settingsOpen = true
	m.settingsPage = page
	m.settingsQuery = ""
	m.settingsErr = ""
	m.settingsCollapsed = map[string]bool{}
	m.settingsDraft = m.settingsDraftFromPrefs()
	m.settingsHistory = []string{page}
	m.settingsHistoryAt = 0
	m.LoadJavaTools()
	if page == settingsPageJava {
		m.javaSettings = javaSettingsState{}
		m.LoadJavaSettings()
	}
	if settingsPageUsesAgentBackend(page) {
		m.loadAgentSettings()
	}
	if page == settingsPageLanguageServers {
		m.loadLanguageServerSettings()
	}
	if page == settingsPageProjectTrust {
		m.loadProjectTrust()
	}
	m.invalidate()
}

func settingsPageUsesAgentBackend(page string) bool {
	switch page {
	case settingsPageAgents, settingsPageAgentPolicy,
		settingsPageAgentRegistry, settingsPageAgentMCP:
		return true
	default:
		return false
	}
}

// closeSettings closes the dialog as Cancel does: the staged choices are
// dropped and the stored ones go back on the window.
func (m *Model) closeSettings() {
	if m.languageToolSettings.saving {
		return
	}
	if !m.settingsOpen {
		return
	}
	m.settingsOpen = false
	m.keymapRecorder = keymapRecorderState{}
	m.settingsQuery = ""
	m.settingsCollapsed = nil
	m.settingsErr = ""
	m.applyPreferences()
	m.resetJavaSettingsSession()
	m.resetLanguageServerSettingsSession()
	m.invalidate()
}

// themeForName is the theme a stored value draws, or nil to follow the
// system's appearance.
func themeForName(name string) *ui.Theme {
	switch name {
	case "light":
		return openideaTheme(false)
	case "dark":
		return openideaTheme(true)
	}
	return nil
}

// applyEditorFont sets the configured Font page choices plus the shared
// runtime zoom on every open editor.
func (m *Model) applyEditorFont(size float32) {
	m.applyEditorFontPrefs(m.prefs.Font, size)
}

// applyEditorFontPrefs is applyEditorFont with a staged Font page value.
func (m *Model) applyEditorFontPrefs(prefs FontPrefs, size float32) {
	size += float32(m.editorFontZoom)
	for _, t := range m.allTabs() {
		if t == nil || t.state == nil {
			continue
		}
		t.state.Font = ui.Font{
			Family:   prefs.fontFamily(),
			Size:     size,
			Weight:   prefs.weight(),
			Features: prefs.features(),
		}
		if t.kind != tabKindWhatsNew {
			t.state.LineHeight = prefs.lineHeightPixels(size) / size
		}
	}
}

// applyEditorAppearance gives every open editor the gutter and whitespace
// choices of Settings › Editor › Appearance.
func (m *Model) applyEditorAppearance() {
	m.applyAppearanceToTabs(m.prefs.Editor)
}

// applyAppearanceToTabs is the same with an appearance the dialog stages.
func (m *Model) applyAppearanceToTabs(a EditorAppearance) {
	for _, t := range m.allTabs() {
		m.applyTabAppearance(t, a)
	}
}

// applyTabAppearance is the same for one editor, including a tab opened after
// the choice was made. Distraction Free Mode hides the gutter over whatever
// the settings say.
func (m *Model) applyTabAppearance(t *Tab, a EditorAppearance) {
	if t == nil || t.state == nil {
		return
	}
	t.state.LineNumbers = editor.LineNumbersOn
	switch {
	case a.NoLineNumbers || m.zen:
		t.state.LineNumbers = editor.LineNumbersOff
	case a.LineNumberMode == "relative":
		t.state.LineNumbers = editor.LineNumbersRelative
	}
	t.state.NoIndentGuides = a.NoIndentGuides
	t.state.NoFolding = a.NoFolding || m.zen
	t.state.SoftWrap = a.SoftWrap
	if t.doc != nil {
		t.state.SoftWrap = a.WrapsFile(t.doc.Path())
	}
	t.state.CursorShape = editor.CursorBar
	if a.BlockCaret {
		t.state.CursorShape = editor.CursorBlock
	}
	t.state.NoScrollPastEnd = !a.ScrollPastEnd
	if a.ShowWhitespace {
		t.state.Whitespace = editor.WhitespaceAll
	} else {
		t.state.Whitespace = editor.WhitespaceSelection
	}
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
	m.status = "Appearance updated"
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

const (
	minEditorFontZoom = -6
	maxEditorFontZoom = 36
)

// editorFontSize is the configured editor font with the shared runtime zoom
// applied. New tabs use it so they match every editor already open.
func (m *Model) editorFontSize() float32 {
	return m.prefs.fontSize() + float32(m.editorFontZoom)
}

// stepEditorFontZoom changes all open editors together without rewriting the
// configured font size, as OpenIDEA's Increase/Decrease Font Size actions do.
func (m *Model) stepEditorFontZoom(delta int) {
	next := max(minEditorFontZoom, min(m.editorFontZoom+delta, maxEditorFontZoom))
	if next == m.editorFontZoom {
		return
	}
	m.editorFontZoom = next
	m.applyEditorFont(m.prefs.fontSize())
	m.status = fmt.Sprintf("Editor font zoom %+d", next)
	m.invalidate()
}

// resetEditorFontZoom removes the shared runtime zoom and leaves the
// configured size unchanged.
func (m *Model) resetEditorFontZoom() {
	if m.editorFontZoom == 0 {
		return
	}
	m.editorFontZoom = 0
	m.applyEditorFont(m.prefs.fontSize())
	m.status = "Editor font size reset"
	m.invalidate()
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
	m.status = fmt.Sprintf("Editor font size %d", size)
	m.invalidate()
}

// applyPreferences puts the stored settings on the model: the theme the
// window draws with and the font size of every open editor.
func (m *Model) applyPreferences() {
	m.theme = m.prefs.themeWithAccent()
	m.zen = m.prefs.Zen
	m.crumbs = m.prefs.Breadcrumbs
	m.tabPrefs = m.prefs.Tabs
	m.applyEditorFont(m.prefs.fontSize())
	m.applyEditorAppearance()
	m.invalidateCodeVision()
	m.sortTabsAlphabetically()
}

// savePreferences writes the settings; a failed write is reported rather than
// silently keeping the choice only for this run.
func (m *Model) savePreferences() {
	if m.cfg.SettingsPath == "" {
		return
	}
	if err := m.prefs.Save(m.cfg.SettingsPath); err != nil {
		m.status = "Saving settings failed: " + err.Error()
	}
}

// SmokeSettings drives the settings sheet offscreen the way the dialog's
// controls do: a staged theme previews before Apply, Apply writes the choice
// to the file, the font clamps at the top, and Cancel drops what was staged.
func (m *Model) SmokeSettings() map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		was := m.prefs
		m.OpenSettings()
		out["opened"] = m.settingsOpen
		out["page"] = m.settingsPage
		out["treeRows"] = len(m.settingsVisibleRows())
		m.settingsSelectPage(settingsPageEditorFont)
		out["selected"] = m.settingsPage
		m.settingsSetTheme("dark")
		out["previewDark"] = m.theme != nil && m.theme.Dark
		out["stagedTheme"] = m.settingsDraft.theme
		out["prefsBeforeApply"] = m.prefs.Theme
		out["pending"] = m.settingsPending()
		m.applySettings()
		out["theme"] = m.prefs.Theme
		out["darkTheme"] = m.theme != nil && m.theme.Dark
		out["storedTheme"] = LoadPreferences(m.settingsWritePath()).Theme
		before := m.prefs.fontSize()
		m.settingsSetFont(before + 1)
		out["fontBefore"] = before
		out["fontAfter"] = m.settingsDraft.fontSize
		m.settingsSetFont(99)
		out["fontMax"] = m.settingsDraft.fontSize
		m.applySettings()
		if t := m.tab(); t != nil {
			out["editorFont"] = t.state.Font.Size
		}
		// A staged change the user cancels puts the stored theme back.
		m.settingsSetTheme("light")
		out["previewLight"] = m.theme != nil && !m.theme.Dark
		m.closeSettings()
		out["closed"] = !m.settingsOpen
		out["themeAfterCancel"] = m.prefs.Theme
		out["previewAfterCancel"] = m.theme != nil && m.theme.Dark
		m.prefs = was
		m.applyPreferences()
		m.savePreferences()
	})
	return out
}

// javaToolsSection draws the Java language server / debugger rows and the
// install action, so a fresh data directory can get a working Java toolchain
// without leaving the app.
func (m *Model) javaToolsSection(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "JDK").FontSize(12).TextColor(p.TextMuted).SingleLine()
		ui.Spacer(c)
		ui.Button(c, "Download JDK…").OnClick(func() {
			m.OpenJDKDownloadDialog(0)
		})
	})
	ui.Text(c, "Java Language Services and Debugger").FontSize(12).TextColor(p.TextMuted).SingleLine()
	if m.Root() == "" {
		ui.Text(c, "Open a Java project to install it").FontSize(12).TextColor(p.TextMuted).SingleLine()
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
			ui.Text(c, label).FontSize(12).TextColor(color).SingleLine()
			if t.Version != "" {
				ui.Text(c, t.Version).FontSize(12).TextColor(p.TextMuted).SingleLine()
			}
			ui.Spacer(c)
			if t.Status == "missing" && t.CanInstall && !busy {
				ui.Button(c, "Install").OnClick(func() { m.InstallJavaToolchain() })
			}
			if t.Status == "missing" && !t.CanInstall && t.Reason != "" && !busy {
				ui.Text(c, t.Reason).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
		})
	}
	switch {
	case m.toolsLoading:
		ui.Text(c, "Reading the tool status…").FontSize(12).TextColor(p.TextMuted).SingleLine()
	case !m.toolsLoaded:
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "The tool status has not been read").FontSize(12).TextColor(p.TextMuted)
			ui.Spacer(c)
			ui.Button(c, "Check").OnClick(func() { m.LoadJavaTools() })
		})
	case busy:
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.toolsBusy).FontSize(12).TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			ui.Button(c, "Cancel").OnClick(func() { m.CancelToolInstall() })
		})
	}
	if busy && m.toolsProgress != "" {
		ui.Text(c, m.toolsProgress).FontSize(12).TextColor(p.TextMuted).SingleLine()
	}
	if len(m.toolsLog) > 0 {
		from := max(0, len(m.toolsLog)-2)
		for _, line := range m.toolsLog[from:] {
			ui.Text(c, line).FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
	}
	if m.toolsErr != "" {
		ui.Text(c, m.toolsErr).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
	}
	if !busy && len(m.javaTools) > 0 && len(javaToolsMissing(m.javaTools)) == 0 {
		ui.Text(c, "Java editing, building and debugging are available").FontSize(12).TextColor(p.Success).SingleLine()
	}
}

// SetAccentPref stores the appearance's accent colour and redraws the window
// with it at once; an unknown id is ignored.
func (m *Model) SetAccentPref(id string) {
	known := false
	for _, c := range accentChoices {
		if c.id == id {
			known = true
			break
		}
	}
	if !known || m.prefs.Accent == id {
		return
	}
	m.prefs.Accent = id
	m.applyPreferences()
	m.savePreferences()
	m.status = "Accent color updated"
	m.invalidate()
}

// SetStartupPref stores what a start shows: "" or "welcome" shows the Welcome
// screen, "reopen" opens the last project.
func (m *Model) SetStartupPref(mode string) {
	switch mode {
	case "", "welcome", "reopen":
	default:
		return
	}
	if m.prefs.Startup == mode {
		return
	}
	m.prefs.Startup = mode
	m.savePreferences()
	m.invalidate()
}

// startupReopensProject is the opt-in "Reopen last project on startup"; the
// Welcome screen is what an untouched start shows.
func (m *Model) startupReopensProject() bool { return m.prefs.Startup == "reopen" }

// keymapPreset is the keymap the build ships, shown in Appearance.
func keymapPreset() string { return keymapPresetTitle }
