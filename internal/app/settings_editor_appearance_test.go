package app

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestSettingsEditorAppearanceDefaultsMatchOpenIDEA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := &Tab{state: &editor.State{}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageEditorAppearance)

	if m.settingsDraft.caretBlinking != true ||
		m.settingsDraft.caretBlinkPeriod != defaultCaretBlinkPeriod ||
		m.settingsDraft.smoothCaretBlinking ||
		m.settingsDraft.smoothCaretMovement ||
		m.settingsDraft.caretMovementStyle != "snappy" ||
		!m.settingsDraft.hardWrapGuides ||
		m.settingsDraft.methodSeparators ||
		!m.settingsDraft.whitespaceLeading ||
		!m.settingsDraft.whitespaceInner ||
		!m.settingsDraft.whitespaceTrailing ||
		m.settingsDraft.rainbowIndentGuides ||
		!m.settingsDraft.intentionBulb {
		t.Fatalf("draft defaults = %+v", m.settingsDraft)
	}
	if want := []int{defaultHardWrapColumn}; !slices.Equal(tab.state.WrapGuides, want) {
		t.Fatalf("default wrap guides = %v, want %v", tab.state.WrapGuides, want)
	}
}

func TestSettingsEditorAppearanceStagesAppliesAndResets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	tab := &Tab{state: &editor.State{}}
	m := &Model{cfg: Config{SettingsPath: path}, tabs: []*Tab{tab}}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageEditorAppearance)

	m.settingsSetCaretBlinking(false)
	m.settingsSetCaretBlinkPeriod(725)
	m.settingsSetSmoothCaretBlinking(true)
	m.settingsSetSmoothCaretMovement(true)
	m.settingsSetCaretMovementStyle("gliding")
	m.settingsSetHardWrapGuides(false)
	m.settingsSetLineNumberMode("hybrid")
	m.settingsSetMethodSeparators(true)
	m.settingsSetWhitespaceParts(false, true, false)
	m.settingsSetRainbowIndentGuides(true)
	m.settingsSetIntentionBulb(false)

	if m.prefs.Editor != (EditorAppearance{}) {
		t.Fatalf("staging wrote preferences: %+v", m.prefs.Editor)
	}
	if !m.settingsDirtyPage(settingsPageEditorAppearance) {
		t.Fatal("staged appearance did not dirty its page")
	}
	if tab.state.LineNumbers != editor.LineNumbersRelative {
		t.Fatalf("hybrid preview uses line numbers %v", tab.state.LineNumbers)
	}
	if len(tab.state.WrapGuides) != 0 {
		t.Fatalf("disabled hard wrap kept guides %v", tab.state.WrapGuides)
	}

	m.applySettings()
	stored := LoadPreferences(path)
	got := stored.Editor
	if !got.NoCaretBlinking ||
		got.CaretBlinkPeriod != 725 ||
		!got.SmoothCaretBlink ||
		!got.SmoothCaretMovement ||
		got.CaretMovementStyle != "gliding" ||
		!got.NoHardWrapGuides ||
		got.LineNumberMode != "hybrid" ||
		!got.MethodSeparators ||
		!got.NoWhitespaceLeading ||
		got.NoWhitespaceInner ||
		!got.NoWhitespaceTrailing ||
		!got.RainbowIndentGuides ||
		!got.NoIntentionBulb {
		t.Fatalf("stored editor appearance = %+v", got)
	}

	m.settingsSelectPage(settingsPageEditorAppearance)
	m.settingsResetPage()
	if !m.settingsDraft.caretBlinking ||
		m.settingsDraft.caretBlinkPeriod != defaultCaretBlinkPeriod ||
		m.settingsDraft.smoothCaretBlinking ||
		m.settingsDraft.smoothCaretMovement ||
		m.settingsDraft.caretMovementStyle != "snappy" ||
		!m.settingsDraft.hardWrapGuides ||
		m.settingsDraft.lineNumberMode != "absolute" ||
		m.settingsDraft.methodSeparators ||
		!m.settingsDraft.whitespaceLeading ||
		!m.settingsDraft.whitespaceInner ||
		!m.settingsDraft.whitespaceTrailing ||
		m.settingsDraft.rainbowIndentGuides ||
		!m.settingsDraft.intentionBulb {
		t.Fatalf("Reset editor appearance = %+v", m.settingsDraft)
	}
}

func TestSettingsEditorWhitespaceUsesTheNearestMyGoMode(t *testing.T) {
	tests := []struct {
		name                     string
		leading, inner, trailing bool
		want                     editor.Whitespace
	}{
		{name: "none", want: editor.WhitespaceNone},
		{name: "trailing", trailing: true, want: editor.WhitespaceTrailing},
		{name: "inner", inner: true, want: editor.WhitespaceAll},
		{name: "all", leading: true, inner: true, trailing: true, want: editor.WhitespaceAll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tab := &Tab{state: &editor.State{}}
			m := &Model{tabs: []*Tab{tab}}
			m.applyTabAppearance(tab, EditorAppearance{
				ShowWhitespace:       true,
				NoWhitespaceLeading:  !tt.leading,
				NoWhitespaceInner:    !tt.inner,
				NoWhitespaceTrailing: !tt.trailing,
			})
			if tab.state.Whitespace != tt.want {
				t.Fatalf("whitespace = %v, want %v", tab.state.Whitespace, tt.want)
			}
		})
	}
}

func TestEditorAppearanceValidatesCaretAndMovementValues(t *testing.T) {
	a := EditorAppearance{}
	if got := a.caretBlinkMillis(); got != defaultCaretBlinkPeriod {
		t.Fatalf("default blink = %d", got)
	}
	if got := a.caretMovement(); got != "snappy" {
		t.Fatalf("default movement = %q", got)
	}
	a.CaretBlinkPeriod = 50
	if got := a.caretBlinkMillis(); got != defaultCaretBlinkPeriod {
		t.Fatalf("short blink = %d", got)
	}
	a.CaretBlinkPeriod = 3000
	if got := a.caretBlinkMillis(); got != defaultCaretBlinkPeriod {
		t.Fatalf("long blink = %d", got)
	}
	a.CaretBlinkPeriod = 900
	a.CaretMovementStyle = "gliding"
	if got := a.caretBlinkMillis(); got != 900 {
		t.Fatalf("custom blink = %d", got)
	}
	if got := a.caretMovement(); got != "gliding" {
		t.Fatalf("custom movement = %q", got)
	}
}
