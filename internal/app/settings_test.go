package app

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestLoadPreferencesDefaults(t *testing.T) {
	if p := LoadPreferences(filepath.Join(t.TempDir(), "missing.json")); !reflect.DeepEqual(p, Preferences{}) {
		t.Fatalf("missing file = %+v, want defaults", p)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := LoadPreferences(bad); !reflect.DeepEqual(p, Preferences{}) {
		t.Fatalf("broken file = %+v, want defaults", p)
	}
}

func TestRunWindowVisibilityFollowsConfigurationAndGlobalSetting(t *testing.T) {
	off, on := false, true
	m := &Model{}
	if !m.showRunWindow(nil, true) || m.showRunWindow(nil, false) {
		t.Fatal("default Run window visibility did not follow the requested start")
	}
	m.prefs.Run = &RunPrefs{ShowOnStart: &off}
	if m.showRunWindow(nil, true) {
		t.Fatal("run.showOnStart=false still revealed Run")
	}
	if !m.showRunWindow(&openidea.RunConfig{FocusToolWindow: true}, true) {
		t.Fatal("a focused configuration did not reveal Run")
	}
	if !m.showRunWindow(&openidea.RunConfig{OpenToolWindow: &on}, true) {
		t.Fatal("a configuration set to open Run did not reveal it")
	}
	if m.showRunWindow(&openidea.RunConfig{OpenToolWindow: &off}, true) {
		t.Fatal("a configuration set to keep Run closed revealed it")
	}
	if m.showRunWindow(&openidea.RunConfig{FocusToolWindow: true}, false) {
		t.Fatal("an explicit show=false did not keep Run hidden")
	}
}

func TestPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	want := Preferences{
		Theme: "dark", FontSize: 17,
		Problems: ProblemsPrefs{
			NoSeveritySort: true, SortByName: true, ShowPreview: true, Autoscroll: true,
		},
		Hierarchy: HierarchyPrefs{Autoscroll: true},
	}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := LoadPreferences(path); !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadPreferences = %+v, want %+v", got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		t.Fatalf("directory holds %v, want only settings.json", entries)
	}
}

func TestThemePreferenceFollowsChoice(t *testing.T) {
	if th := (Preferences{}).themePreference(); th != nil {
		t.Fatalf("unset theme = %+v, want system", th)
	}
	if th := (Preferences{Theme: "system"}).themePreference(); th != nil {
		t.Fatalf("system theme = %+v, want nil", th)
	}
	light := (Preferences{Theme: "light"}).themePreference()
	dark := (Preferences{Theme: "dark"}).themePreference()
	if light == nil || dark == nil {
		t.Fatalf("light = %v, dark = %v, want both set", light, dark)
	}
	if light.Dark || !dark.Dark || light.Background == dark.Background {
		t.Fatalf("themes do not differ: light=%+v dark=%+v", light, dark)
	}
}

func TestFontSizePrefDefaults(t *testing.T) {
	if got := (Preferences{}).fontSize(); got != defaultFontSize {
		t.Fatalf("unset font size = %v, want %v", got, defaultFontSize)
	}
	if got := (Preferences{FontSize: 16}).fontSize(); got != 16 {
		t.Fatalf("stored font size = %v, want 16", got)
	}
}

func TestStepFontClamps(t *testing.T) {
	tab := &Tab{state: &editor.State{}}
	m := &Model{tabs: []*Tab{tab}, prefs: Preferences{FontSize: maxFontSize}}
	m.applyPreferences()
	m.stepFont(5)
	if m.prefs.FontSize != maxFontSize {
		t.Fatalf("size above the range = %v, want %d", m.prefs.FontSize, maxFontSize)
	}
	if tab.state.Font.Size != maxFontSize {
		t.Fatalf("editor font = %v, want %d", tab.state.Font.Size, maxFontSize)
	}
	m.stepFont(-5)
	if m.prefs.FontSize != maxFontSize-5 {
		t.Fatalf("size after lower step = %v, want %d", m.prefs.FontSize, maxFontSize-5)
	}
	m.prefs.FontSize = minFontSize
	m.stepFont(-5)
	if m.prefs.FontSize != minFontSize {
		t.Fatalf("size below the range = %v, want %d", m.prefs.FontSize, minFontSize)
	}
	m.prefs.FontSize = 0
	m.stepFont(1)
	if m.prefs.FontSize != defaultFontSize+1 {
		t.Fatalf("size from the default = %v, want %d", m.prefs.FontSize, defaultFontSize+1)
	}
}

func TestCodeVisionSettingsDefaultsAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}, ctx: nil}
	staged := m.settingsDraftFromPrefs()
	if !staged.codeVisionUsages || !staged.codeVisionAuthor || !staged.codeVisionNoUsages ||
		staged.codeVisionUsagesPosition != "top" || staged.codeVisionAuthorPosition != "top" {
		t.Fatalf("Code Vision defaults = %+v", staged)
	}
	staged.codeVisionUsages = false
	staged.codeVisionAuthor = false
	staged.codeVisionNoUsages = false
	staged.codeVisionUsagesPosition = "right"
	staged.codeVisionAuthorPosition = "right"
	m.settingsDraft = staged
	m.settingsPage = settingsPageInlayHints
	if !m.settingsDirtyPage(settingsPageInlayHints) || !m.settingsPending() {
		t.Fatal("Code Vision changes were not staged")
	}
	m.applySettings()
	want := InlayPrefs{
		NoCodeVisionUsages:   true,
		NoCodeVisionAuthor:   true,
		HideCodeVisionNoUses: true,
		UsagesPosition:       "right",
		AuthorPosition:       "right",
	}
	if m.prefs.Inlay != want || LoadPreferences(path).Inlay != want {
		t.Fatalf("stored Code Vision = %+v / %+v, want %+v", m.prefs.Inlay, LoadPreferences(path).Inlay, want)
	}
	if m.settingsDraftFromPrefs().codeVisionUsages {
		t.Fatal("stored Code Vision did not reload into the settings draft")
	}
}

func TestAgentChatSettingsDefaultsAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	staged := m.settingsDraftFromPrefs()
	if staged.agents.newTaskApprovals() != "bypass" ||
		staged.agents.sendWith() != "enter" ||
		staged.agents.whileRunning() != "queue" ||
		staged.agents.toolCalls() != "grouped" ||
		staged.agents.thinking() != "auto" ||
		staged.agents.NoFoldTurns || staged.agents.NoSmoothText {
		t.Fatalf("Agents defaults = %+v", staged.agents)
	}
	staged.agents.NewTaskApprovals = "ask"
	staged.agents.SendWith = "modEnter"
	staged.agents.WhileRunning = "steer"
	staged.agents.ToolCalls = "each"
	staged.agents.Thinking = "expanded"
	staged.agents.NoFoldTurns = true
	staged.agents.ExpandToolOutput = true
	staged.agents.ShowCost = true
	m.settingsDraft = staged
	if !m.settingsDirtyPage(settingsPageAgents) || !m.settingsPending() {
		t.Fatal("Agents changes were not staged")
	}
	m.applySettings()
	stored := LoadPreferences(path).Agents
	if stored == nil || stored.NewTaskApprovals != "ask" ||
		stored.SendWith != "modEnter" || stored.WhileRunning != "steer" ||
		stored.ToolCalls != "each" || stored.Thinking != "expanded" ||
		!stored.NoFoldTurns || !stored.ExpandToolOutput || !stored.ShowCost {
		t.Fatalf("stored Agents settings = %+v", stored)
	}
	if got := m.agentPrefs(); got.SendWith != "modEnter" || got.WhileRunning != "steer" {
		t.Fatalf("active Agents settings = %+v", got)
	}
}

func TestCodeVisionSettingsPageRendersOpenIDEAControls(t *testing.T) {
	m := &Model{}
	m.settingsDraft = m.settingsDraftFromPrefs()
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsInlayHintsBody(c, oiPaletteOf(c.Theme()))
	}, 720, 420)
	tester.Frame()
	for _, text := range []string{
		"Code Vision", "Usages", "Position", "Top",
		"Show “no usages”", "Code author",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Inlay Hints page lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestApplyPreferencesPicksTheme(t *testing.T) {
	m := &Model{prefs: Preferences{Theme: "dark"}}
	m.applyPreferences()
	if m.theme == nil || !m.theme.Dark {
		t.Fatalf("theme = %+v, want dark", m.theme)
	}
	m.prefs.Theme = "system"
	m.applyPreferences()
	if m.theme != nil {
		t.Fatalf("theme = %+v, want system", m.theme)
	}
}

func TestSettingsTreeNestsAndGroups(t *testing.T) {
	tree := settingsTree()
	var roots []string
	for _, n := range tree {
		roots = append(roots, n.title)
	}
	want := []string{
		"Appearance & Behavior", "Version Control", "Keymap", "Editor",
		"Build, Execution, Deployment", "Languages & Frameworks", "Agents", "Security", "Tools",
	}
	if !reflect.DeepEqual(roots, want) {
		t.Fatalf("roots = %v, want %v", roots, want)
	}
	editor, ok := settingsFindNode(tree, settingsGroupKey("Editor"))
	if !ok || len(editor.children) != 14 {
		t.Fatalf("Editor folder = %+v, want General, Code Editing, Font, Color Scheme, Code Style, Inspections, Natural Languages, File Encodings, File Types, Inlay Hints, TODO, Vim, Live Templates and File and Code Templates", editor)
	}
	general := editor.children[0]
	if general.key != settingsPageGeneral || editor.children[1].key != settingsPageCodeEditing ||
		editor.children[2].key != settingsPageEditorFont ||
		editor.children[3].key != settingsPageColorScheme ||
		editor.children[4].key != settingsPageCodeStyle ||
		editor.children[5].key != settingsPageInspections ||
		editor.children[6].key != settingsPageNaturalLanguages ||
		editor.children[7].key != settingsPageFileEncodings ||
		editor.children[8].key != settingsPageFileTypes ||
		editor.children[9].key != settingsPageInlayHints || editor.children[10].key != settingsPageTodo ||
		editor.children[11].key != settingsPageVim ||
		editor.children[12].key != settingsPageLiveTemplates ||
		editor.children[13].key != settingsPageFileTemplates {
		t.Fatalf("Editor children = %+v", editor.children)
	}
	if len(general.children) != 12 ||
		general.children[0].key != settingsPageAutoImport ||
		general.children[1].key != settingsPageEditorAppearance ||
		general.children[2].key != settingsPageBreadcrumbs ||
		general.children[3].key != settingsPageCodeCompletion ||
		general.children[4].key != settingsPageCodeFolding ||
		general.children[5].key != settingsPageEditorTabs ||
		general.children[6].key != settingsPageGutterIcons ||
		general.children[7].key != settingsPageInlineCompletion ||
		general.children[8].key != settingsPageRichTextCopy ||
		general.children[9].key != settingsPageSmartKeys ||
		general.children[10].key != settingsPageStickyLines ||
		general.children[11].key != settingsPagePostfix {
		t.Fatalf("General children = %+v", general.children)
	}
	if got := settingsBreadcrumb(tree, settingsPageEditorTabs); !reflect.DeepEqual(got,
		[]string{"Editor", "General", "Editor Tabs"}) {
		t.Fatalf("Editor Tabs breadcrumb = %v", got)
	}
	if got := settingsBreadcrumb(tree, settingsPageBreadcrumbs); !reflect.DeepEqual(got,
		[]string{"Editor", "General", "Breadcrumbs"}) {
		t.Fatalf("Breadcrumbs breadcrumb = %v", got)
	}
	if got := settingsBreadcrumb(tree, settingsPageJava); !reflect.DeepEqual(got, []string{"Languages & Frameworks", "Java"}) {
		t.Fatalf("Java breadcrumb = %v", got)
	}
	if got := settingsBreadcrumb(tree, settingsPageKeymap); !reflect.DeepEqual(got, []string{"Keymap"}) {
		t.Fatalf("Keymap breadcrumb = %v", got)
	}
	if got := settingsBreadcrumb(tree, settingsPageDateFormats); !reflect.DeepEqual(got,
		[]string{"Appearance & Behavior", "System Settings", "Date Formats"}) {
		t.Fatalf("Date Formats breadcrumb = %v", got)
	}
	if got := settingsBreadcrumb(tree, settingsPageIssueNavigation); !reflect.DeepEqual(got,
		[]string{"Version Control", "Issue Navigation"}) {
		t.Fatalf("Issue Navigation breadcrumb = %v", got)
	}
}

func TestSettingsDialogResizesClampsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg: Config{SettingsPath: path},
		prefs: Preferences{
			SettingsWindowWidth:  1180,
			SettingsWindowHeight: 860,
			SettingsSidebarWidth: 260,
		},
		frameWidth:  1600,
		frameHeight: 1000,
	}
	if width, height := m.settingsDialogSize(); width != 1180 || height != 860 {
		t.Fatalf("stored Settings size = %vx%v", width, height)
	}
	m.resizeSettingsDialog(500, 300)
	if m.prefs.SettingsWindowWidth != 1552 || m.prefs.SettingsWindowHeight != 952 {
		t.Fatalf("clamped Settings size = %dx%d", m.prefs.SettingsWindowWidth, m.prefs.SettingsWindowHeight)
	}
	m.prefs.SettingsWindowHeight = 100
	m.frameWidth, m.frameHeight = 800, 600
	if width, height := m.settingsDialogSize(); width != 752 || height != 552 {
		t.Fatalf("small-window Settings size = %vx%v, want 752x552", width, height)
	}
	m.prefs.SettingsSidebarWidth = 500
	if got := m.settingsSideWidth(); got < 300.79 || got > 300.81 {
		t.Fatalf("sidebar in a small sheet = %v, want 300.8", got)
	}
	if err := m.prefs.Save(path); err != nil {
		t.Fatal(err)
	}
	stored := LoadPreferences(path)
	if stored.SettingsWindowWidth != m.prefs.SettingsWindowWidth {
		t.Fatalf("stored Settings width = %d, want %d", stored.SettingsWindowWidth, m.prefs.SettingsWindowWidth)
	}
}

func TestSettingsDialogResizeKeepsTheOppositeEdgeAnchored(t *testing.T) {
	m := &Model{
		prefs:       Preferences{SettingsWindowWidth: 980, SettingsWindowHeight: 720},
		frameWidth:  1600,
		frameHeight: 1000,
	}
	m.resizeSettingsDialog(120, 80)
	if m.settingsOffsetX != 60 {
		t.Fatalf("horizontal resize compensation = %v, want 60", m.settingsOffsetX)
	}
	if m.settingsOffsetY != 40 {
		t.Fatalf("vertical resize compensation = %v, want 40", m.settingsOffsetY)
	}
	m.resizeSettingsDialog(-120, -80)
	if m.settingsOffsetX != 0 || m.settingsOffsetY != 0 {
		t.Fatalf("restored offsets = %v, %v, want zero", m.settingsOffsetX, m.settingsOffsetY)
	}
}

func TestSettingsDialogCornerDoubleClickExpandsAndRestores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg:                    Config{SettingsPath: path},
		frameWidth:             1500,
		frameHeight:            900,
		settingsWindowResizing: false,
	}
	m.prefs.SettingsWindowWidth, m.prefs.SettingsWindowHeight = 0, 0
	m.toggleExpandedSettingsDialog()
	if width, height := m.settingsDialogSize(); width != 1452 || height != 852 {
		t.Fatalf("expanded Settings size = %vx%v", width, height)
	}
	m.toggleExpandedSettingsDialog()
	if width, height := m.settingsDialogSize(); width != settingsDialogWidth || height != settingsDialogMaxHeight {
		t.Fatalf("restored Settings size = %vx%v", width, height)
	}
}

func TestSettingsResizeKeepsTheDraggedEdgeUnderThePointer(t *testing.T) {
	m := &Model{
		frameWidth:  1500,
		frameHeight: 900,
	}
	m.prefs.SettingsWindowWidth = 700
	m.prefs.SettingsWindowHeight = 500
	width, height := m.settingsDialogSize()
	m.resizeSettingsDialog(200, 100)
	if m.settingsOffsetX != 100 || m.settingsOffsetY != 50 {
		t.Fatalf("resize compensation = (%v, %v), want (100, 50)",
			m.settingsOffsetX, m.settingsOffsetY)
	}
	if gotWidth, gotHeight := m.settingsDialogSize(); gotWidth <= width || gotHeight <= height {
		t.Fatalf("resize did not grow the sheet: %vx%v from %vx%v",
			gotWidth, gotHeight, width, height)
	}
}

func TestSettingsFilterKeepsAncestors(t *testing.T) {
	rows := settingsFilterTree(settingsTree(), "editor font", nil)
	if len(rows) != 1 || rows[0].key != settingsGroupKey("Editor") {
		t.Fatalf("filtered roots = %+v, want the Editor folder", rows)
	}
	if kids := rows[0].children; len(kids) != 1 || kids[0].key != settingsPageEditorFont || !kids[0].matched {
		t.Fatalf("filtered children = %+v, want the matched Font page", kids)
	}
	if rows := settingsFilterTree(settingsTree(), "no such setting", nil); len(rows) != 0 {
		t.Fatalf("unmatched query kept %+v", rows)
	}
}

func TestSettingsQuerySelectsFirstMatch(t *testing.T) {
	m := &Model{}
	m.OpenSettingsPage(settingsPageKeymap)
	m.settingsQuery = "java"
	m.settingsQueryChanged()
	if m.settingsPage != settingsPageJava {
		t.Fatalf("page after searching java = %q", m.settingsPage)
	}
	m.settingsQuery = ""
	m.settingsQueryChanged()
	if m.settingsPage != settingsPageJava {
		t.Fatalf("clearing the query moved the page to %q", m.settingsPage)
	}
}

func TestSettingsHistoryBackAndForward(t *testing.T) {
	m := &Model{}
	m.OpenSettingsPage(settingsPageAppearance)
	m.settingsSelectPage(settingsPageKeymap)
	m.settingsSelectPage(settingsPageEditorFont)
	m.settingsStepHistory(-1)
	if m.settingsPage != settingsPageKeymap {
		t.Fatalf("back = %q, want keymap", m.settingsPage)
	}
	m.settingsStepHistory(-1)
	if m.settingsPage != settingsPageAppearance {
		t.Fatalf("back again = %q, want appearance", m.settingsPage)
	}
	m.settingsStepHistory(1)
	if m.settingsPage != settingsPageKeymap {
		t.Fatalf("forward = %q, want keymap", m.settingsPage)
	}
}

func TestSettingsCollapseHidesChildren(t *testing.T) {
	m := &Model{}
	m.OpenSettingsPage(settingsPageAppearance)
	open := len(m.settingsVisibleRows())
	editor, ok := settingsFindNode(settingsTree(), settingsGroupKey("Editor"))
	if !ok {
		t.Fatal("Editor group missing")
	}
	hidden := len(settingsFlattenAll(editor.children))
	m.settingsToggleNode(settingsGroupKey("Editor"))
	closed := len(m.settingsVisibleRows())
	if closed != open-hidden {
		t.Fatalf("rows open %d, closed %d, want %d fewer", open, closed, hidden)
	}
	if m.settingsNormalizeKey("no.such.page") != "" {
		t.Fatalf("unknown key normalized to a row")
	}
}

func TestSettingsStagesUntilApplyOrCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}, prefs: Preferences{Theme: "light", FontSize: 15, ZenWidth: 90}}
	m.applyPreferences()

	m.OpenSettingsPage(settingsPageAppearance)
	m.settingsSetTheme("dark")
	if m.prefs.Theme != "light" {
		t.Fatalf("staging wrote the theme = %q", m.prefs.Theme)
	}
	if m.theme == nil || !m.theme.Dark {
		t.Fatalf("staged theme was not previewed")
	}
	if !m.settingsPending() {
		t.Fatalf("a staged theme did not enable Apply")
	}
	m.closeSettings()
	if m.settingsOpen || m.theme != nil && m.theme.Dark || m.prefs.Theme != "light" {
		t.Fatalf("cancel kept open %v theme %+v pref %q", m.settingsOpen, m.theme, m.prefs.Theme)
	}

	m.OpenSettingsPage(settingsPageAppearance)
	m.settingsSetTheme("dark")
	m.applySettings()
	if m.prefs.Theme != "dark" || LoadPreferences(path).Theme != "dark" {
		t.Fatalf("apply stored %q / %q", m.prefs.Theme, LoadPreferences(path).Theme)
	}

	m.settingsSelectPage(settingsPageEditorFont)
	m.settingsSetFont(18)
	m.settingsSetZenWidth(120)
	if !m.settingsPending() {
		t.Fatalf("font and width changes did not enable Apply")
	}
	m.applySettings()
	if stored := LoadPreferences(path); stored.FontSize != 18 || stored.ZenWidth != 120 {
		t.Fatalf("stored font %v width %d", stored.FontSize, stored.ZenWidth)
	}
	m.settingsSetFont(99)
	if m.settingsDraft.fontSize != maxFontSize {
		t.Fatalf("font above the range = %v", m.settingsDraft.fontSize)
	}
	m.settingsSelectPage(settingsPageEditorAppearance)
	m.settingsResetPage()
	if m.settingsDraft.zenWidth != defaultZenWidth {
		t.Fatalf("reset width = %d, want %d", m.settingsDraft.zenWidth, defaultZenWidth)
	}
}

func TestSettingsFontStagesPreviewAndPersistsEveryTypographyChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := &Tab{state: &editor.State{}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageEditorFont)

	m.settingsSetFontFamily("Menlo")
	m.settingsSetFont(18)
	m.settingsSetFontLineHeight(27)
	m.settingsSetFontLigatures(true)
	m.settingsSetFontWeight(600)
	m.settingsSetFontFallback("Arial")

	if m.prefs.Font != (FontPrefs{}) || m.prefs.FontSize != 0 {
		t.Fatalf("font staging wrote preferences: %+v size=%v", m.prefs.Font, m.prefs.FontSize)
	}
	if tab.state.Font.Family != "Menlo, Arial" || tab.state.Font.Size != 18 ||
		tab.state.Font.Weight != 600 || tab.state.Font.Features != "" {
		t.Fatalf("font preview = %+v", tab.state.Font)
	}
	if !m.settingsPending() {
		t.Fatal("a staged font did not enable Apply")
	}

	m.applySettings()
	want := FontPrefs{Family: "Menlo", LineHeight: 27, Ligatures: true, Weight: 600, Fallback: "Arial"}
	if m.prefs.Font != want || m.prefs.FontSize != 18 {
		t.Fatalf("applied font = %+v size=%v", m.prefs.Font, m.prefs.FontSize)
	}
	if stored := LoadPreferences(path); stored.Font != want || stored.FontSize != 18 {
		t.Fatalf("stored font = %+v size=%v", stored.Font, stored.FontSize)
	}
}

func TestSettingsKeymapRecorderStagesAndPersistsABinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageKeymap)

	const actionID = "app.commandPalette"
	before := m.KeymapBindings(actionID)
	m.settingsRecordKeymap(actionID)
	if !m.keymapRecorder.open {
		t.Fatal("the shortcut recorder did not open")
	}
	if !m.settingsRecordKeymapInput(ui.InputEvent{
		Kind: ui.InputKeyDown,
		Key:  ui.KeyP,
		Mods: ui.Ctrl | ui.Alt,
	}) {
		t.Fatal("the shortcut recorder did not consume the chord")
	}
	if m.keymapRecorder.binding != "Ctrl+Alt+P" {
		t.Fatalf("captured binding = %q", m.keymapRecorder.binding)
	}
	m.settingsCommitRecordedKeymap()

	if got := m.KeymapBindings(actionID); !reflect.DeepEqual(got, before) {
		t.Fatalf("default bindings changed before Apply: %v", got)
	}
	if !m.settingsPending() {
		t.Fatal("a staged keymap did not enable Apply")
	}
	m.applySettings()
	if got := m.KeymapBindings(actionID); !reflect.DeepEqual(got, []string{"Ctrl+Alt+P"}) {
		t.Fatalf("applied binding = %v", got)
	}
	if got := LoadPreferences(path).Keymap[actionID]; !reflect.DeepEqual(got, []string{"Ctrl+Alt+P"}) {
		t.Fatalf("stored binding = %v", got)
	}
}

func TestSettingsAppearanceSwitchesReachTheEditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := &Tab{state: &editor.State{}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()
	if tab.state.LineNumbers != editor.LineNumbersOn || tab.state.Whitespace != editor.WhitespaceSelection ||
		tab.state.NoIndentGuides {
		t.Fatalf("defaults: line %v whitespace %v noindent %v",
			tab.state.LineNumbers, tab.state.Whitespace, tab.state.NoIndentGuides)
	}

	m.OpenSettingsPage(settingsPageEditorAppearance)
	m.settingsSetLineNumbers(false)
	m.settingsSetShowWhitespace(true)
	m.settingsSetIndentGuides(false)
	if m.prefs.Editor != (EditorAppearance{}) {
		t.Fatalf("staging wrote the prefs: %+v", m.prefs.Editor)
	}
	if tab.state.LineNumbers != editor.LineNumbersOff || tab.state.Whitespace != editor.WhitespaceAll ||
		!tab.state.NoIndentGuides {
		t.Fatalf("staged appearance not previewed")
	}
	m.closeSettings()
	if tab.state.LineNumbers != editor.LineNumbersOn || tab.state.Whitespace != editor.WhitespaceSelection ||
		tab.state.NoIndentGuides {
		t.Fatalf("cancel kept the staged appearance")
	}

	m.OpenSettingsPage(settingsPageEditorAppearance)
	m.settingsSetLineNumbers(false)
	m.settingsSetShowWhitespace(true)
	m.settingsSetIndentGuides(false)
	if !m.settingsDirtyPage(settingsPageEditorAppearance) {
		t.Fatalf("staged appearance did not dirty its page")
	}
	m.applySettings()
	want := EditorAppearance{NoLineNumbers: true, ShowWhitespace: true, NoIndentGuides: true}
	if m.prefs.Editor != want || LoadPreferences(path).Editor != want {
		t.Fatalf("stored appearance = %+v / %+v", m.prefs.Editor, LoadPreferences(path).Editor)
	}
	if tab.state.LineNumbers != editor.LineNumbersOff || tab.state.Whitespace != editor.WhitespaceAll ||
		!tab.state.NoIndentGuides {
		t.Fatalf("applied appearance not on the editor")
	}

	// Distraction Free Mode hides the gutter over the setting, and leaving it
	// goes back to what the setting says.
	m.prefs.Zen = true
	m.applyPreferences()
	if tab.state.LineNumbers != editor.LineNumbersOff {
		t.Fatalf("zen kept the gutter")
	}
	m.prefs.Zen = false
	m.applyPreferences()
	if tab.state.LineNumbers != editor.LineNumbersOff || tab.state.Whitespace != editor.WhitespaceAll {
		t.Fatalf("leaving zen dropped the stored appearance")
	}

	m.settingsSelectPage(settingsPageEditorAppearance)
	m.settingsResetPage()
	d := m.settingsDraft
	if !d.lineNumbers || d.showWhitespace || !d.indentGuides {
		t.Fatalf("reset appearance = %+v", d)
	}
}

func TestSettingsBlockCaretReachesTheEditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := &Tab{state: &editor.State{}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()
	if tab.state.CursorShape != editor.CursorBar {
		t.Fatalf("defaults draw a block caret")
	}

	m.OpenSettingsPage(settingsPageEditorAppearance)
	m.settingsSetBlockCaret(true)
	if m.prefs.Editor.BlockCaret {
		t.Fatalf("staging wrote the prefs")
	}
	if tab.state.CursorShape != editor.CursorBlock {
		t.Fatalf("staged block caret not previewed")
	}
	if !m.settingsDirtyPage(settingsPageEditorAppearance) {
		t.Fatalf("staged block caret did not dirty its page")
	}
	m.closeSettings()
	if tab.state.CursorShape != editor.CursorBar {
		t.Fatalf("cancel kept the block caret")
	}

	m.OpenSettingsPage(settingsPageEditorAppearance)
	m.settingsSetBlockCaret(true)
	m.applySettings()
	if !m.prefs.Editor.BlockCaret || !LoadPreferences(path).Editor.BlockCaret {
		t.Fatalf("stored block caret = %+v", LoadPreferences(path).Editor)
	}
	if tab.state.CursorShape != editor.CursorBlock {
		t.Fatalf("applied block caret not on the editor")
	}

	m.settingsSelectPage(settingsPageEditorAppearance)
	m.settingsResetPage()
	if m.settingsDraft.blockCaret {
		t.Fatalf("reset kept the block caret")
	}
	if tab.state.CursorShape != editor.CursorBar {
		t.Fatalf("reset preview kept the block caret")
	}
}

func TestSettingsCodeFoldingStagesAndApplies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := &Tab{state: &editor.State{}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()
	if tab.state.NoFolding {
		t.Fatalf("defaults hide the folding outline")
	}

	m.OpenSettingsPage(settingsPageCodeFolding)
	m.settingsSetShowFolding(false)
	if !tab.state.NoFolding {
		t.Fatalf("staged outline not previewed")
	}
	if m.settingsDirtyPage(settingsPageEditorAppearance) {
		t.Fatalf("a folding change dirtied the Appearance page")
	}
	if !m.settingsPending() {
		t.Fatalf("a folding change did not enable Apply")
	}
	m.applySettings()
	if stored := LoadPreferences(path); !stored.Editor.NoFolding {
		t.Fatalf("stored folding = %+v", stored.Editor)
	}
	if !tab.state.NoFolding {
		t.Fatalf("applied folding not on the editor")
	}

	m.settingsSelectPage(settingsPageCodeFolding)
	m.settingsResetPage()
	if !m.settingsDraft.showFolding {
		t.Fatalf("reset folding = %+v", m.settingsDraft)
	}
}

func TestSettingsFoldImportsOpensEditorsAndUnfoldsWhenDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	buffer := editor.NewBuffer("package demo\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() { fmt.Println() }\n")
	buffer.SetLanguage(editor.LanguageFor("main.go"))
	tab := &Tab{state: &editor.State{Buffer: buffer}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()

	if !slices.Contains(tab.state.Folded(), 2) {
		t.Fatalf("default import fold = %v, want line 2", tab.state.Folded())
	}
	m.OpenSettingsPage(settingsPageCodeFolding)
	m.settingsSetFoldImports(false)
	if slices.Contains(tab.state.Folded(), 2) {
		t.Fatalf("staged import-off kept folds %v", tab.state.Folded())
	}
	m.applySettings()
	if stored := LoadPreferences(path); !stored.Editor.NoFoldImports {
		t.Fatalf("stored import folding = %+v", stored.Editor)
	}

	m.settingsSelectPage(settingsPageCodeFolding)
	m.settingsResetPage()
	if !m.settingsDraft.foldImports || !slices.Contains(tab.state.Folded(), 2) {
		t.Fatalf("reset import folding = draft %v folds %v", m.settingsDraft.foldImports, tab.state.Folded())
	}
}

func TestSettingsBreadcrumbsStagePlacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	if m.crumbs.Hidden || m.crumbs.AtBottom() {
		t.Fatalf("defaults hidden=%v bottom=%v", m.crumbs.Hidden, m.crumbs.AtBottom())
	}

	m.OpenSettingsPage(settingsPageBreadcrumbs)
	m.settingsSetBreadcrumbPlacement("bottom")
	if !sameBreadcrumbPrefs(m.prefs.Breadcrumbs, BreadcrumbPrefs{}) {
		t.Fatalf("staging wrote the prefs: %+v", m.prefs.Breadcrumbs)
	}
	if m.crumbs.Hidden || !m.crumbs.AtBottom() {
		t.Fatalf("staged placement not previewed: %+v", m.crumbs)
	}
	if !m.settingsDirtyPage(settingsPageBreadcrumbs) || !m.settingsPending() {
		t.Fatalf("a staged placement did not dirty its page")
	}
	m.closeSettings()
	if m.crumbs.AtBottom() {
		t.Fatalf("cancel kept the staged placement")
	}

	m.OpenSettingsPage(settingsPageBreadcrumbs)
	if languages := m.settingsBreadcrumbLanguages(); !slices.Contains(languages, "go") ||
		!slices.Contains(languages, "javascript") {
		t.Fatalf("MyGo language catalog missing from Breadcrumbs: %v", languages)
	}
	m.settingsSetShowBreadcrumbs(false)
	m.settingsSetBreadcrumbPlacement("bottom")
	m.settingsToggleBreadcrumbLanguage("go")
	m.applySettings()
	want := BreadcrumbPrefs{Hidden: true, Placement: "bottom", DisabledLanguages: []string{"go"}}
	stored := LoadPreferences(path).Breadcrumbs
	if !sameBreadcrumbPrefs(m.prefs.Breadcrumbs, want) || !sameBreadcrumbPrefs(stored, want) {
		t.Fatalf("stored breadcrumbs = %+v / %+v", m.prefs.Breadcrumbs, stored)
	}
	if !m.crumbs.Hidden || !m.crumbs.AtBottom() {
		t.Fatalf("applied breadcrumbs not on the window: %+v", m.crumbs)
	}

	m.settingsSelectPage(settingsPageBreadcrumbs)
	m.settingsResetPage()
	if d := m.settingsDraft; !d.showBreadcrumbs || d.breadcrumbPlacement != "top" {
		t.Fatalf("reset breadcrumbs = %+v", d)
	}
	// The radios offer two placements, so anything else is ignored.
	m.settingsSetBreadcrumbPlacement("left")
	if m.settingsDraft.breadcrumbPlacement != "top" {
		t.Fatalf("an unknown placement was staged")
	}
}

func TestSettingsUpdatesStageSourceAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.OpenSettingsPage(settingsPageUpdates)
	m.settingsDraft.updateSource = "cnb"
	m.settingsDraft.updateAutoCheck = false
	m.settingsDraft.updateWhatsNew = false
	if !m.settingsDirtyPage(settingsPageUpdates) || !m.settingsPending() {
		t.Fatal("Updates choices were not staged")
	}
	m.applySettings()
	stored := LoadPreferences(path).Updates
	if stored == nil || stored.Source != "cnb" || !stored.NoAutoCheck || !stored.NoWhatsNew {
		t.Fatalf("stored Updates = %+v", stored)
	}
	if _, err := m.updateFeedForSource(); err == nil {
		t.Fatal("the unconfigured CNB source silently selected another feed")
	}
}

func TestSettingsEditorTabsStageAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	if m.tabPrefs != (TabPrefs{}) || m.settingsDraftFromPrefs().tabPlacement != "top" {
		t.Fatalf("defaults = %+v / %q", m.tabPrefs, m.settingsDraftFromPrefs().tabPlacement)
	}

	m.OpenSettingsPage(settingsPageEditorTabs)
	m.settingsSetTabPlacement("bottom")
	m.settingsSetShowExtension(false)
	m.settingsSetMarkModified(true)
	m.settingsSetClosePosition("left")
	m.settingsSetSortTabs(true)
	m.settingsSetOpenAtEnd(true)
	m.settingsSetTabLimit(5)
	m.settingsSetActivateOnClose("mru")
	m.settingsSetNoPreviewTab(true)
	if m.prefs.Tabs != (TabPrefs{}) {
		t.Fatalf("staging wrote the prefs: %+v", m.prefs.Tabs)
	}
	if m.tabPrefs.Placement != "bottom" || !m.tabPrefs.HideExtension || !m.tabPrefs.MarkModified ||
		!m.tabPrefs.CloseButtonLeft || !m.tabPrefs.SortAlphabetically || !m.tabPrefs.OpenAtEnd ||
		m.tabPrefs.TabLimit != 5 || m.tabPrefs.activation() != "mru" || m.tabPrefs.previews() {
		t.Fatalf("staged strip not previewed: %+v", m.tabPrefs)
	}
	if !m.settingsDirtyPage(settingsPageEditorTabs) || !m.settingsPending() {
		t.Fatalf("staging did not enable Apply")
	}
	m.closeSettings()
	if m.tabPrefs != (TabPrefs{}) {
		t.Fatalf("cancel kept the staged strip: %+v", m.tabPrefs)
	}

	m.OpenSettingsPage(settingsPageEditorTabs)
	m.settingsSetTabPlacement("bottom")
	m.settingsSetShowExtension(false)
	m.settingsSetMarkModified(true)
	m.settingsSetClosePosition("left")
	m.settingsSetSortTabs(true)
	m.settingsSetOpenAtEnd(true)
	m.settingsSetTabLimit(5)
	m.settingsSetActivateOnClose("mru")
	m.settingsSetNoPreviewTab(true)
	m.applySettings()
	want := TabPrefs{
		Placement: "bottom", HideExtension: true, MarkModified: true, CloseButtonLeft: true,
		SortAlphabetically: true, OpenAtEnd: true, TabLimit: 5, ActivateOnClose: "mru",
		NoPreviewTab: true,
	}
	if m.prefs.Tabs != want || LoadPreferences(path).Tabs != want {
		t.Fatalf("stored tabs = %+v / %+v", m.prefs.Tabs, LoadPreferences(path).Tabs)
	}
	if m.tabPrefs != want {
		t.Fatalf("applied strip = %+v", m.tabPrefs)
	}

	m.settingsSelectPage(settingsPageEditorTabs)
	m.settingsResetPage()
	if d := m.settingsDraft; d.tabPlacement != "top" || !d.showExtension || d.markModified ||
		d.closePosition != "right" || d.sortAlphabetically || d.openAtEnd || d.tabLimit != defaultTabLimit ||
		d.activateOnClose != "left" || d.noPreviewTab {
		t.Fatalf("reset tabs = %+v", d)
	}
	// The select offers all four edges and None, so anything else is ignored.
	m.settingsSetTabPlacement("left")
	if d := m.settingsDraft; d.tabPlacement != "left" || !m.tabPrefs.Vertical() {
		t.Fatalf("left placement = %+v / %+v", d, m.tabPrefs)
	}
	m.settingsSetTabPlacement("right")
	if d := m.settingsDraft; d.tabPlacement != "right" || !m.tabPrefs.Vertical() {
		t.Fatalf("right placement = %+v / %+v", d, m.tabPrefs)
	}
	m.settingsSetTabPlacement("middle")
	m.settingsSetClosePosition("middle")
	m.settingsSetActivateOnClose("middle")
	if d := m.settingsDraft; d.tabPlacement != "right" || d.closePosition != "right" ||
		d.activateOnClose != "left" {
		t.Fatalf("unknown choices were staged: %+v", d)
	}
	m.prefs.Tabs.Placement = "left"
	if d := m.settingsDraftFromPrefs(); d.tabPlacement != "left" {
		t.Fatalf("stored left placement = %+v", d)
	}
	m.prefs.Tabs.Placement = ""
	if d := m.settingsDraftFromPrefs(); d.tabPlacement != "top" {
		t.Fatalf("stored top placement = %+v", d)
	}
}

func TestSettingsTabOverflowAndFileIconStageAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	if d := m.settingsDraftFromPrefs(); d.multipleRows || d.squeezeTabs || !d.showFileIcon {
		t.Fatalf("defaults = %+v", d)
	}

	m.OpenSettingsPage(settingsPageEditorTabs)
	m.settingsSetMultipleRows(true)
	m.settingsSetSqueezeTabs(true)
	m.settingsSetShowFileIcon(false)
	if m.prefs.Tabs != (TabPrefs{}) {
		t.Fatalf("staging wrote the prefs: %+v", m.prefs.Tabs)
	}
	if !m.tabPrefs.MultipleRows || !m.tabPrefs.SqueezeTabs || !m.tabPrefs.HideFileIcon {
		t.Fatalf("staged strip = %+v", m.tabPrefs)
	}
	m.closeSettings()
	if m.tabPrefs != (TabPrefs{}) {
		t.Fatalf("cancel kept the staged strip: %+v", m.tabPrefs)
	}

	m.OpenSettingsPage(settingsPageEditorTabs)
	m.settingsSetMultipleRows(true)
	m.settingsSetSqueezeTabs(true)
	m.settingsSetShowFileIcon(false)
	m.applySettings()
	want := TabPrefs{MultipleRows: true, SqueezeTabs: true, HideFileIcon: true}
	if m.prefs.Tabs != want || LoadPreferences(path).Tabs != want {
		t.Fatalf("stored tabs = %+v / %+v", m.prefs.Tabs, LoadPreferences(path).Tabs)
	}
	if m.tabPrefs != want {
		t.Fatalf("applied strip = %+v", m.tabPrefs)
	}

	m.settingsSelectPage(settingsPageEditorTabs)
	m.settingsResetPage()
	if d := m.settingsDraft; d.multipleRows || d.squeezeTabs || !d.showFileIcon {
		t.Fatalf("reset tabs = %+v", d)
	}
}

func TestSettingsGeneralWrapsAndVirtualSpace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := &Tab{state: &editor.State{}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()
	if tab.state.SoftWrap {
		t.Fatalf("defaults soft-wrap")
	}
	if !tab.state.NoScrollPastEnd {
		t.Fatalf("defaults scroll past the end, OpenIDEA stops at it")
	}

	if (EditorAppearance{}).WrapsFile("README.md") {
		t.Fatalf("soft wrap off still wrapped")
	}
	wrap := EditorAppearance{SoftWrap: true}
	if !wrap.WrapsFile("README.md") || wrap.WrapsFile("Main.java") {
		t.Fatalf("default patterns: md %v java %v", wrap.WrapsFile("README.md"), wrap.WrapsFile("Main.java"))
	}
	if !(EditorAppearance{SoftWrap: true, SoftWrapFiles: "*"}).WrapsFile("Main.java") {
		t.Fatalf("* did not wrap every file")
	}
	own := EditorAppearance{SoftWrap: true, SoftWrapFiles: "*.java; *.kt"}
	if !own.WrapsFile("Main.java") || !own.WrapsFile("App.kt") || own.WrapsFile("a.md") {
		t.Fatalf("custom patterns: java %v kt %v md %v",
			own.WrapsFile("Main.java"), own.WrapsFile("App.kt"), own.WrapsFile("a.md"))
	}

	m.OpenSettingsPage(settingsPageGeneral)
	m.settingsSetSoftWrap(true)
	m.settingsSetScrollPastEnd(true)
	m.settingsSetSoftWrapFiles("*.java")
	if !tab.state.SoftWrap || tab.state.NoScrollPastEnd {
		t.Fatalf("staged general not previewed: wrap %v noScroll %v",
			tab.state.SoftWrap, tab.state.NoScrollPastEnd)
	}
	if m.settingsDirtyPage(settingsPageEditorAppearance) {
		t.Fatalf("a General change dirtied the Appearance page")
	}
	if !m.settingsDirtyPage(settingsPageGeneral) || !m.settingsPending() {
		t.Fatalf("a General change did not enable Apply")
	}
	m.applySettings()
	stored := LoadPreferences(path)
	if !stored.Editor.SoftWrap || !stored.Editor.ScrollPastEnd || stored.Editor.SoftWrapFiles != "*.java" {
		t.Fatalf("stored general = %+v", stored.Editor)
	}

	m.settingsSelectPage(settingsPageGeneral)
	m.settingsResetPage()
	d := m.settingsDraft
	if d.softWrap || d.scrollPastEnd || d.softWrapFiles != defaultSoftWrapFiles {
		t.Fatalf("reset general = %+v", d)
	}
}

func TestSettingsDateFormatsStageApplyAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.OpenSettingsPage(settingsPageDateFormats)
	if m.settingsDraft.dateFormat != defaultDateFormatPrefs() {
		t.Fatalf("Date Formats defaults = %+v", m.settingsDraft.dateFormat)
	}

	m.settingsDraft.dateFormat = DateFormatPrefs{
		OverrideSystem: true,
		Pattern:        "yyyy/MM/dd",
		Use24Hour:      false,
		Pretty:         false,
	}
	if !m.settingsDirtyPage(settingsPageDateFormats) || !m.settingsPending() {
		t.Fatal("Date Formats change was not staged")
	}
	m.applySettings()
	got := LoadPreferences(path).dateFormatPrefs()
	if !got.OverrideSystem || got.Pattern != "yyyy/MM/dd" || got.Use24Hour || got.Pretty {
		t.Fatalf("stored Date Formats = %+v", got)
	}
	if m.prefs.DateFormat == nil || !sameDateFormatPrefs(*m.prefs.DateFormat, got) {
		t.Fatalf("applied Date Formats = %+v", m.prefs.DateFormat)
	}

	m.settingsSelectPage(settingsPageDateFormats)
	m.settingsResetPage()
	if m.settingsDraft.dateFormat != defaultDateFormatPrefs() {
		t.Fatalf("reset Date Formats = %+v", m.settingsDraft.dateFormat)
	}
}

func TestSettingsSystemReopenProjectStagesAndApplies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.OpenSettingsPage(settingsPageSystem)
	if !m.settingsDraft.reopenLast {
		t.Fatal("System Settings did not start at OpenIDEA's reopen default")
	}
	m.settingsDraft.reopenLast = false
	if !m.settingsDirtyPage(settingsPageSystem) || !m.settingsPending() {
		t.Fatal("System Settings reopen choice was not staged")
	}
	m.applySettings()
	if got := LoadPreferences(path).Startup; got != "welcome" {
		t.Fatalf("stored startup = %q", got)
	}
	m.settingsSelectPage(settingsPageSystem)
	m.settingsResetPage()
	if !m.settingsDraft.reopenLast {
		t.Fatal("reset System Settings did not restore the reopen default")
	}
}

func TestSettingsIssueNavigationStagesAppliesAndEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.OpenSettingsPage(settingsPageIssueNavigation)
	m.settingsAddIssuePreset(IssueLinkPrefs{
		Pattern: `[A-Z]+-\d+`, Link: "https://tracker.example/$0",
	})
	if !m.settingsDirtyPage(settingsPageIssueNavigation) || !m.settingsPending() {
		t.Fatal("Issue Navigation row was not staged")
	}
	m.settingsEditIssueLink(0)
	m.settingsDraft.issueLinkDraft.Link = "https://tracker.example/browse/$0"
	m.settingsCommitIssueLink()
	m.applySettings()
	stored := LoadPreferences(path).VCS.IssueNavigation
	if len(stored) != 1 || stored[0].Pattern != `[A-Z]+-\d+` ||
		stored[0].Link != "https://tracker.example/browse/$0" {
		t.Fatalf("stored Issue Navigation = %+v", stored)
	}

	m.settingsSelectPage(settingsPageIssueNavigation)
	m.settingsRemoveIssueLink(0)
	if len(m.settingsDraft.issueLinks) != 0 {
		t.Fatalf("removed Issue Navigation row remains: %+v", m.settingsDraft.issueLinks)
	}
	m.settingsResetPage()
	m.applySettings()
	if got := LoadPreferences(path).VCS.IssueNavigation; got != nil {
		t.Fatalf("reset Issue Navigation = %+v", got)
	}
}

func TestIssueNavigationEditorCommitsWithEnterAndCancelsWithEscape(t *testing.T) {
	m := &Model{}
	m.settingsDraft = m.settingsDraftFromPrefs()
	m.settingsAddIssuePreset(IssueLinkPrefs{
		Pattern: `[A-Z]+-\d+`,
		Link:    "https://tracker.example/$0",
	})
	m.settingsEditIssueLink(0)
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsIssueNavigationBody(c, oiPaletteOf(c.Theme()))
	}, 900, 500)
	tester.Frame()

	tester.Type("x")
	tester.Key(0, ui.KeyEnter)
	if m.settingsDraft.issueEditing != -1 ||
		m.settingsDraft.issueLinks[0].Pattern != `[A-Z]+-\d+x` {
		t.Fatalf("Enter result: editing=%d links=%+v",
			m.settingsDraft.issueEditing, m.settingsDraft.issueLinks)
	}

	m.settingsEditIssueLink(0)
	tester.Frame()
	tester.Type("y")
	tester.Key(0, ui.KeyEscape)
	if m.settingsDraft.issueEditing != -1 ||
		m.settingsDraft.issueLinks[0].Pattern != `[A-Z]+-\d+x` {
		t.Fatalf("Escape result: editing=%d links=%+v",
			m.settingsDraft.issueEditing, m.settingsDraft.issueLinks)
	}
}
