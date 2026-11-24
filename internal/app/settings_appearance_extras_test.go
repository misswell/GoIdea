package app

import (
	"path/filepath"
	"testing"
)

func TestSettingsAppearanceExtrasStageApplyPersistAndReachRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageAppearance)

	m.settingsDraft.useCustomFont = true
	m.settingsDraft.uiFontFamily = "Menlo"
	m.settingsDraft.uiFontFallback = "Arial"
	m.settingsDraft.uiFontSize = 15
	m.settingsDraft.contrastScrollbars = true
	m.settingsDraft.redGreenVision = true
	m.settingsDraft.differentToolWindowBackground = true
	m.settingsDraft.smallTreeIndents = true
	m.settingsDraft.fullPathInTitle = true
	m.settingsDraft.projectColorsInToolbar = false
	m.settingsDraft.dndWithAltOnly = true
	m.settingsDraft.menuIcons = false
	m.settingsDraft.mainMenuStyle = "merged"
	m.settingsDraft.mainMenuAlt = false
	m.settingsDraft.ideAntialiasing = "subpixel"
	m.settingsDraft.editorAntialiasing = "none"
	m.settingsDraft.widescreenToolWindows = true
	if !m.settingsDirtyPage(settingsPageAppearance) {
		t.Fatal("new Appearance controls did not dirty the page")
	}

	m.settingsPreview()
	if m.theme == nil {
		t.Fatal("Appearance preview did not install a theme")
	}
	if m.theme.Font != "Menlo, Arial" || m.theme.FontSize != 15 {
		t.Fatalf("custom UI font = %q %.0f", m.theme.Font, m.theme.FontSize)
	}
	if m.theme.ScrollbarWidth != 12 {
		t.Fatalf("contrast scrollbar width = %.0f", m.theme.ScrollbarWidth)
	}
	if m.theme.Danger == openideaTheme(true).Danger {
		t.Fatal("red-green accessibility palette did not change the error color")
	}

	m.applySettings()
	if got := m.treeIndentWidth(); got != 12 {
		t.Fatalf("small tree indent = %.0f", got)
	}
	appliedTheme := m.prefs.systemThemeWithAccent(false)
	if got := m.toolWindowBackground(appliedTheme); got != appliedTheme.Surface {
		t.Fatalf("different tool-window background = %v, want %v", got, appliedTheme.Surface)
	}
	stored := LoadPreferences(path)
	if !stored.Appearance.UseCustomFont ||
		stored.Appearance.UIFontFamily != "Menlo" ||
		stored.Appearance.UIFontFallback != "Arial" ||
		stored.Appearance.UIFontSize != 15 ||
		!stored.Appearance.ContrastScrollbars ||
		!stored.Appearance.RedGreenVision ||
		!stored.Appearance.DifferentToolWindowBackground ||
		!stored.Appearance.SmallTreeIndents ||
		!stored.Appearance.FullPathInTitle ||
		stored.projectColorsInToolbar() ||
		!stored.Appearance.DndWithAltOnly ||
		stored.menuIconsVisible() ||
		stored.Appearance.MainMenuStyle != "merged" ||
		stored.mainMenuAltFocuses() ||
		stored.Appearance.IDEAntialiasing != "subpixel" ||
		stored.Appearance.EditorAntialiasing != "none" ||
		!stored.Appearance.WidescreenToolWindows {
		t.Fatalf("stored appearance extras = %+v", stored.Appearance)
	}
}

func TestFullPathAppearanceReachesProjectWindowTitle(t *testing.T) {
	m := toolModel()
	m.prefs.Appearance.FullPathInTitle = true
	m.setRoot(t.TempDir())
	full := filepath.Join(m.Root(), "src", "Main.java")
	if got := m.windowTitle(full); got != m.projectLabel()+" – "+full {
		t.Fatalf("window title = %q", got)
	}
}

func TestWidescreenToolWindowsMoveBottomAreaBetweenTheSides(t *testing.T) {
	renderProblemsX := func(widescreen bool) float32 {
		m := toolModel()
		m.mgr = nil
		m.prefs.Appearance.WidescreenToolWindows = widescreen
		m.ShowProblems()
		tester := workbenchTester(t, m)
		r, ok := tester.Find("Problems")
		if !ok {
			t.Fatalf("Problems did not render (widescreen=%v): %v", widescreen, tester.Texts())
		}
		return r.X
	}
	normal := renderProblemsX(false)
	widescreen := renderProblemsX(true)
	if widescreen <= normal {
		t.Fatalf("bottom Problems x = %.1f normal, %.1f widescreen; side did not take full height", normal, widescreen)
	}
}
