package app

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestKeymapPresetRegistryMatchesPinnedOpenIDEASet(t *testing.T) {
	want := []struct {
		id    string
		title string
	}{
		{"intellij-macos", "macOS"},
		{"intellij-windows", "Windows"},
		{"vscode-macos", "VS Code (macOS)"},
		{"vscode-windows", "VS Code (Windows)"},
		{"eclipse-macos", "Eclipse (macOS)"},
		{"eclipse-windows", "Eclipse"},
	}
	if len(keymapPresetOrder) != len(want) {
		t.Fatalf("preset count = %d, want %d", len(keymapPresetOrder), len(want))
	}
	seen := map[string]bool{}
	for i, expected := range want {
		preset := keymapPresetOrder[i]
		if preset.ID != expected.id || preset.Title != expected.title {
			t.Errorf("preset %d = %q / %q, want %q / %q",
				i, preset.ID, preset.Title, expected.id, expected.title)
		}
		if seen[preset.ID] {
			t.Errorf("duplicate preset id %q", preset.ID)
		}
		seen[preset.ID] = true
		if len(preset.Actions) == 0 || len(preset.Bindings) == 0 {
			t.Errorf("%s has no generated keymap", preset.ID)
		}
	}
}

func TestKeymapPresetsFollowTheHostAndPinnedOverrides(t *testing.T) {
	wantDefault := "intellij-windows"
	if runtime.GOOS == "darwin" {
		wantDefault = "intellij-macos"
	}
	if got := keymapPresetDefinitionsByOS(); got != wantDefault {
		t.Fatalf("default preset = %q, want %q", got, wantDefault)
	}
	if got := keymapNormalize("cmd+shift+alt+ctrl+k"); got != "Ctrl+Alt+Shift+Cmd+K" {
		t.Fatalf("canonical binding = %q", got)
	}

	eclipseMac := keymapPresetsByID["eclipse-macos"]
	intellijMac := keymapPresetsByID["intellij-macos"]
	if got, want := eclipseMac.CanonicalBindings["file.save"], intellijMac.CanonicalBindings["file.save"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("Eclipse did not inherit Save: %v, want %v", got, want)
	}
	if got := eclipseMac.CanonicalBindings["code.deleteLine"]; !reflect.DeepEqual(got, []string{"Cmd+D"}) {
		t.Fatalf("Eclipse macOS Delete Line = %v, want Cmd+D", got)
	}
	if got := eclipseMac.CanonicalBindings["code.duplicateLine"]; len(got) != 0 {
		t.Fatalf("Eclipse macOS kept the inherited Duplicate Line binding %v", got)
	}
	if got := eclipseMac.CanonicalBindings["vcs.commit"]; len(got) != 0 {
		t.Fatalf("Eclipse macOS kept the inherited Commit binding %v", got)
	}

	vscodeWindows := keymapPresetsByID["vscode-windows"]
	if got := vscodeWindows.CanonicalBindings["code.addCaretNextOccurrence"]; !reflect.DeepEqual(got, []string{"Ctrl+D"}) {
		t.Fatalf("VS Code Windows Add Selection = %v, want Ctrl+D", got)
	}
	for id, bindings := range vscodeWindows.CanonicalBindings {
		for _, binding := range bindings {
			for _, stroke := range strings.Fields(binding) {
				parts := strings.Split(stroke, "+")
				if slicesContain(parts, "Cmd") && slicesContain(parts, "Ctrl") {
					t.Errorf("%s retained a mixed Meta/Ctrl stroke: %q", id, binding)
				}
			}
		}
	}
}

func TestKeymapEditorConflictUsesEditorContextFirst(t *testing.T) {
	tab := keymapPresetTestTab("a\nb\nc\n", 2, 2)
	m := &Model{
		tabs:   []*Tab{tab},
		active: 0,
		prefs:  Preferences{KeymapPreset: "eclipse-macos"},
	}
	m.setRoot("/project")
	tab.editorFocused = true
	m.applyKeymapPreferences()

	if !keymapEditorBindingsHave(tab.state.Bindings, ui.Cmd, ui.KeyD, editor.DeleteLine) {
		t.Fatal("Eclipse Cmd+D did not belong to Delete Line while the editor was focused")
	}
	if keymapEditorBindingsHave(tab.state.Bindings, ui.Cmd, ui.KeyD, editor.DuplicateSelection) {
		t.Fatal("Eclipse Cmd+D still duplicated the line")
	}

	tab.editorFocused = false
	m.applyKeymapPreferences()
	if keymapEditorBindingsHave(tab.state.Bindings, ui.Cmd, ui.KeyD, editor.DeleteLine) {
		t.Fatal("Eclipse Cmd+D still deleted a line while another context owned the keyboard")
	}
}

func TestKeymapSettingsStageApplyReloadAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := keymapPresetTestTab("a\nb\n", 2, 2)
	m := &Model{
		cfg:    Config{SettingsPath: path},
		tabs:   []*Tab{tab},
		active: 0,
	}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageKeymap)
	m.settingsDraft.keymapPreset = "vscode-macos"
	m.settingsDraft.keymap = map[string][]string{"file.save": {"Alt+P"}}
	if !m.settingsPending() {
		t.Fatal("the staged keymap did not enable Apply")
	}
	m.applySettings()
	if m.settingsErr != "" {
		t.Fatalf("Apply failed: %s", m.settingsErr)
	}
	if got := m.KeymapBindings("file.save"); !reflect.DeepEqual(got, []string{"Alt+P"}) {
		t.Fatalf("applied Save binding = %v", got)
	}
	if !keymapEditorBindingsHave(tab.state.Bindings, ui.Cmd, ui.KeyD, editor.SelectNext) {
		t.Fatal("Apply did not rebuild the open editor with VS Code macOS bindings")
	}
	stored := LoadPreferences(path)
	if stored.KeymapPreset != "vscode-macos" ||
		!reflect.DeepEqual(stored.Keymap["file.save"], []string{"Alt+P"}) {
		t.Fatalf("stored keymap = preset %q overrides %v", stored.KeymapPreset, stored.Keymap)
	}
	reloaded := &Model{prefs: stored}
	if got := reloaded.KeymapBindings("file.save"); !reflect.DeepEqual(got, []string{"Alt+P"}) {
		t.Fatalf("reloaded Save binding = %v", got)
	}

	m.settingsResetPage()
	if m.settingsDraft.keymapPreset != keymapPresetDefinitionsByOS() ||
		len(m.settingsDraft.keymap) != 0 {
		t.Fatalf("Reset Keymap staged preset %q and overrides %v",
			m.settingsDraft.keymapPreset, m.settingsDraft.keymap)
	}
	m.applySettings()
	if m.settingsErr != "" {
		t.Fatalf("Apply after Reset failed: %s", m.settingsErr)
	}
	if m.prefs.KeymapPreset != keymapPresetDefinitionsByOS() || len(m.prefs.Keymap) != 0 {
		t.Fatalf("Reset Keymap left preset %q and overrides %v",
			m.prefs.KeymapPreset, m.prefs.Keymap)
	}
}

func TestKeymapPresetSurfacesShowAllSixChoices(t *testing.T) {
	labels := keymapPresetTitles()

	var gearKeymap menuEntry
	for _, entry := range gearGroup(&Model{}) {
		if entry.label == "Keymap" {
			gearKeymap = entry
			break
		}
	}
	if got := menuEntryLabels(gearKeymap.sub); !reflect.DeepEqual(got, labels) {
		t.Fatalf("gear Keymap choices = %v, want %v", got, labels)
	}

	quick := quickSwitchMenuEntries(&Model{})
	var quickKeymap menuEntry
	for _, entry := range quick {
		if entry.label == "Keymap" {
			quickKeymap = entry
			break
		}
	}
	if got := menuEntryLabels(quickKeymap.sub); !reflect.DeepEqual(got, labels) {
		t.Fatalf("Quick Switch Keymap choices = %v, want %v", got, labels)
	}

	m := &Model{
		cfg:      Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageKeymap)
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsDialog(c, c.Theme())
	}, 1200, 900)
	tester.Frame()
	if !tester.HasText("Keymap:") || !tester.HasText("Restore Default Keymap") {
		t.Fatal("the offscreen Keymap settings page did not render its controls")
	}
	selected := keymapPresetForID(m.settingsDraft.keymapPreset).Title
	if err := tester.Click(selected); err != nil {
		t.Fatalf("open the preset Select: %v", err)
	}
	texts := tester.Texts()
	if len(texts) < len(labels) ||
		!reflect.DeepEqual(texts[len(texts)-len(labels):], labels) {
		t.Fatalf("settings preset choices = %v, want %v", texts, labels)
	}
	if err := tester.Click("VS Code (macOS)"); err != nil {
		t.Fatalf("choose VS Code (macOS): %v", err)
	}
	if got := m.settingsDraft.keymapPreset; got != "vscode-macos" {
		t.Fatalf("Settings staged preset = %q, want vscode-macos", got)
	}
}

func TestShortcutsDispatchPresetCommandsChordsAndEditorMeanings(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")

	m.prefs.KeymapPreset = "intellij-macos"
	column := keymapPresetTestTab("abc\n", 0, 0)
	dispatchKeymapTestKeys(m, column, keymapTestKey{ui.Cmd | ui.Shift, ui.Key8})
	if !column.columnMode || !m.columnSelectionEnabled() {
		t.Fatal("IntelliJ macOS Cmd+Shift+8 did not toggle Column Selection Mode")
	}

	m.prefs.KeymapPreset = "vscode-macos"
	zen := keymapPresetTestTab("abc\n", 0, 0)
	dispatchKeymapTestKeys(m, zen,
		keymapTestKey{ui.Cmd, ui.KeyK},
		keymapTestKey{0, ui.KeyZ},
	)
	if !m.zen {
		t.Fatal("the VS Code Cmd+K Z two-stroke binding did not run Distraction Free Mode")
	}
	if strokes, ok := keymapInputStrokes("Cmd+K Cmd+I"); !ok || len(strokes) != 2 {
		t.Fatalf("VS Code Cmd+K Cmd+I did not parse as a two-stroke chord: %v, %v", strokes, ok)
	}

	cases := []struct {
		name    string
		preset  string
		text    string
		start   int
		end     int
		mods    ui.Modifiers
		key     ui.Key
		want    string
		selects int
	}{
		{"IntelliJ macOS delete", "intellij-macos", "a\nb\nc\n", 2, 2, ui.Cmd, ui.KeyBackspace, "a\nc\n", 0},
		{"IntelliJ Windows delete", "intellij-windows", "a\nb\nc\n", 2, 2, ui.Ctrl, ui.KeyY, "a\nc\n", 0},
		{"Eclipse macOS delete", "eclipse-macos", "a\nb\nc\n", 2, 2, ui.Cmd, ui.KeyD, "a\nc\n", 0},
		{"IntelliJ macOS duplicate", "intellij-macos", "a\nb\nc\n", 2, 2, ui.Cmd, ui.KeyD, "a\nb\nb\nc\n", 0},
		{"VS Code macOS add selection", "vscode-macos", "foo foo\n", 0, 3, ui.Cmd, ui.KeyD, "foo foo\n", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tab := keymapPresetTestTab(tc.text, tc.start, tc.end)
			m.prefs.KeymapPreset = tc.preset
			dispatchKeymapTestKeys(m, tab, keymapTestKey{tc.mods, tc.key})
			if got := tab.state.Buffer.String(); got != tc.want {
				t.Fatalf("buffer = %q, want %q", got, tc.want)
			}
			if tc.selects > 0 && len(tab.state.Selections()) != tc.selects {
				t.Fatalf("selection count = %d, want %d", len(tab.state.Selections()), tc.selects)
			}
		})
	}
}

type keymapTestKey struct {
	mods ui.Modifiers
	key  ui.Key
}

func keymapPresetTestTab(text string, start, end int) *Tab {
	state := &editor.State{Buffer: editor.NewBuffer(text)}
	state.Select(start, end)
	return &Tab{state: state, editorFocused: true}
}

func dispatchKeymapTestKeys(m *Model, tab *Tab, keys ...keymapTestKey) {
	m.tabs = []*Tab{tab}
	m.active = 0
	tab.editorFocused = true
	m.applyKeymapPreferences()
	tester := ui.NewTester(func(c *ui.Context) {
		m.shortcuts(c)
		editor.View(c, tab.state).Fill().AutoFocus()
	}, 900, 420)
	tester.Frame()
	for _, key := range keys {
		tester.Key(key.mods, key.key)
		tester.Frame()
	}
	tester.Frame()
}

func keymapEditorBindingsHave(bindings []editor.Binding, mods ui.Modifiers, key ui.Key, command editor.Command) bool {
	for _, binding := range bindings {
		if binding.Mods == mods && binding.Key == key && binding.Command == command &&
			binding.ThenMods == 0 && binding.ThenKey == 0 {
			return true
		}
	}
	return false
}

func keymapPresetTitles() []string {
	out := make([]string, 0, len(keymapPresetOrder))
	for _, preset := range keymapPresetOrder {
		out = append(out, preset.Title)
	}
	return out
}

func menuEntryLabels(entries []menuEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.sep {
			out = append(out, entry.label)
		}
	}
	return out
}
