package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui/editor"
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

func TestPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	want := Preferences{Theme: "dark", FontSize: 17}
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
	want := []string{"Appearance & Behavior", "Editor", "Keymap", "Languages & Frameworks"}
	if !reflect.DeepEqual(roots, want) {
		t.Fatalf("roots = %v, want %v", roots, want)
	}
	editor, ok := settingsFindNode(tree, settingsGroupKey("Editor"))
	if !ok || len(editor.children) != 2 {
		t.Fatalf("Editor folder = %+v, want its two pages", editor)
	}
	if editor.children[0].key != settingsPageEditorAppearance || editor.children[1].key != settingsPageEditorFont {
		t.Fatalf("Editor children = %q, %q", editor.children[0].key, editor.children[1].key)
	}
	if got := settingsBreadcrumb(tree, settingsPageJava); !reflect.DeepEqual(got, []string{"Languages & Frameworks", "Java"}) {
		t.Fatalf("Java breadcrumb = %v", got)
	}
	if got := settingsBreadcrumb(tree, settingsPageKeymap); !reflect.DeepEqual(got, []string{"Keymap"}) {
		t.Fatalf("Keymap breadcrumb = %v", got)
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
	m.settingsToggleNode(settingsGroupKey("Editor"))
	closed := len(m.settingsVisibleRows())
	if closed != open-2 {
		t.Fatalf("rows open %d, closed %d, want two fewer", open, closed)
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
