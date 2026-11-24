package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

func TestEditorSchemeActionsAreRegisteredAndToggled(t *testing.T) {
	for _, id := range []string{
		"editor.toggleSemanticHighlighting",
		"editor.toggleRainbowBrackets",
		"editor.colorScheme.import",
		"editor.colorScheme.export",
		"editor.colorScheme.delete",
	} {
		action, ok := actionGet(id)
		if !ok {
			t.Fatalf("action %q is not registered", id)
		}
		if action.Run == nil || (strings.HasPrefix(id, "editor.colorScheme.") && action.RunWithArg == nil) {
			t.Fatalf("action %q has no runnable path", id)
		}
	}
	m := &Model{prefs: Preferences{EditorScheme: EditorSchemePrefs{
		SemanticHighlighting: true,
		RainbowBrackets:      true,
	}}}
	if !m.ActionToggled("editor.toggleSemanticHighlighting") ||
		!m.ActionToggled("editor.toggleRainbowBrackets") {
		t.Fatal("scheme action toggled state did not follow preferences")
	}
	m.prefs.EditorScheme.SemanticHighlighting = false
	m.prefs.EditorScheme.RainbowBrackets = false
	if m.ActionToggled("editor.toggleSemanticHighlighting") ||
		m.ActionToggled("editor.toggleRainbowBrackets") {
		t.Fatal("scheme action toggled state did not clear")
	}
}

func TestEditorSchemeSettingsDefaultsAndLocalSync(t *testing.T) {
	def, ok := nativeSettingsDefinition(settingsPageColorSchemeLang)
	if !ok {
		t.Fatal("Color Scheme Language Defaults is not defined")
	}
	for _, section := range def.sections {
		for _, field := range section.fields {
			switch field.key {
			case editorSemanticHighlightingKey, editorRainbowBracketsKey:
				if value := nativeSettingsDefault(field); value != false {
					t.Fatalf("%s default = %v, want OpenIDEA's false", field.key, value)
				}
			}
		}
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyEditorSchemeNativePages(map[string]nativeSettingsPageState{
		settingsPageColorSchemeLang: {
			loaded: true,
			applied: map[string]any{
				editorSemanticHighlightingKey: true,
				editorRainbowBracketsKey:      false,
			},
			draft: map[string]any{
				editorSemanticHighlightingKey: true,
				editorRainbowBracketsKey:      false,
			},
		},
	})
	if !m.prefs.EditorScheme.SemanticHighlighting || m.prefs.EditorScheme.RainbowBrackets {
		t.Fatalf("native page sync = %+v", m.prefs.EditorScheme)
	}
	stored := LoadPreferences(path)
	if !stored.EditorScheme.SemanticHighlighting {
		t.Fatalf("stored scheme settings = %+v", stored.EditorScheme)
	}
	off, on := false, true
	settings := backendEditorSchemeSettings{}
	settings.Editor.SemanticHighlighting = &off
	settings.Editor.RainbowBrackets = &on
	m.mergeBackendEditorSchemeSettings(settings)
	if m.prefs.EditorScheme.SemanticHighlighting || !m.prefs.EditorScheme.RainbowBrackets {
		t.Fatalf("backend sync = %+v", m.prefs.EditorScheme)
	}
}

func TestEditorSchemeTogglePersistsGloballyAndLocally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg:      Config{SettingsPath: path},
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Fatalf("command = %q, want settings_set", command)
			}
			request, ok := args.(map[string]any)
			if !ok {
				t.Fatalf("settings_set args = %#v", args)
			}
			if request["scope"] != "global" ||
				request["key"] != editorRainbowBracketsKey ||
				request["value"] != "true" {
				t.Fatalf("settings_set request = %#v", request)
			}
			return json.RawMessage(`{}`), nil
		},
	}
	m.toggleRainbowBrackets()
	deadline := time.Now().Add(2 * time.Second)
	for !LoadPreferences(path).EditorScheme.RainbowBrackets && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !m.ActionToggled("editor.toggleRainbowBrackets") {
		t.Fatal("rainbow toggle did not commit after settings_set")
	}
	stored := LoadPreferences(path)
	if !stored.EditorScheme.RainbowBrackets {
		t.Fatalf("local preferences = %+v", stored.EditorScheme)
	}
}

func TestRainbowBracketsUseDepthColorsAndSkipStrings(t *testing.T) {
	doc := docbridge.Virtual("/project/Test.java", "call([{ }]) \"([{\"; // ([{\n")
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{
		prefs: Preferences{EditorScheme: EditorSchemePrefs{RainbowBrackets: true}},
		theme: openideaTheme(true),
	}
	backgrounds := m.composeTextBackgrounds(tab, nil)
	if len(backgrounds) != 6 {
		t.Fatalf("rainbow backgrounds = %+v, want six code brackets", backgrounds)
	}
	startColors := map[int]ui.Color{}
	allColors := map[ui.Color]bool{}
	for _, background := range backgrounds {
		startColors[background.Start] = background.Color
		allColors[background.Color] = true
	}
	text := doc.Buffer().Snapshot().String()
	if got := text[backgrounds[0].Start : backgrounds[0].Start+1]; got != "(" {
		t.Fatalf("first bracket at %v", backgrounds)
	}
	if len(allColors) != 3 {
		t.Fatalf("depth colours = %v, want three", allColors)
	}
	if startColors[4] == startColors[5] || startColors[5] == startColors[6] {
		t.Fatalf("adjacent depths share a colour: %v", startColors)
	}
	m.prefs.EditorScheme.RainbowBrackets = false
	if got := m.composeTextBackgrounds(tab, nil); len(got) != 0 {
		t.Fatalf("disabled rainbow backgrounds = %+v", got)
	}
}

func TestSemanticTokenSpansFilterLocalsAndUseUTF16Columns(t *testing.T) {
	tokens := &semanticTokensPayload{
		Data: []int{
			0, 4, 5, 0, 1,
			0, 17, 5, 0, 2,
			0, 25, 5, 1, 0,
		},
		TokenTypes:     []string{"variable", "parameter"},
		TokenModifiers: []string{"local", "static"},
	}
	text := "int total = total + MAX;\n"
	spans := semanticTokenSpans(text, tokens)
	if len(spans) != 1 || spans[0].Name != "total" || spans[0].Start != 4 {
		t.Fatalf("semantic spans = %+v, want the local variable only", spans)
	}

	tokens = &semanticTokensPayload{
		Data:           []int{0, 3, 3, 0, 0},
		TokenTypes:     []string{"parameter"},
		TokenModifiers: []string{},
	}
	text = "😀 \u0061rg\n"
	spans = semanticTokenSpans(text, tokens)
	if len(spans) != 1 || spans[0].Name != "arg" || spans[0].Start != 5 {
		t.Fatalf("UTF-16 semantic spans = %+v", spans)
	}
}

type editorSchemeTestBackend struct {
	info openidea.DocInfo
}

func (b *editorSchemeTestBackend) DocOpen(
	_ context.Context,
	_,
	path string,
) (openidea.DocInfo, error) {
	b.info.Path = path
	return b.info, nil
}

func (b *editorSchemeTestBackend) DocEdit(
	_ context.Context,
	_ string,
	_ uint32,
	_ uint32,
	_ position.ChangeSet,
) (uint32, error) {
	return b.info.Version, nil
}

func (b *editorSchemeTestBackend) DocSnapshot(
	_ context.Context,
	_ string,
	_ uint32,
) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *editorSchemeTestBackend) DocSave(context.Context, string, uint32) error  { return nil }
func (b *editorSchemeTestBackend) DocClose(context.Context, string, uint32) error { return nil }

func TestSemanticHighlightingConsumesBackendTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Main.java")
	backend := &editorSchemeTestBackend{info: openidea.DocInfo{
		ID: 7, Path: path, Text: "int total = total + 1;\n",
		Version: 3, Language: "java",
	}}
	doc, err := docbridge.Open(context.Background(), backend, Window, path)
	if err != nil {
		t.Fatal(err)
	}
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	requested := make(chan map[string]any, 1)
	m := &Model{
		ctx:    context.Background(),
		tabs:   []*Tab{tab},
		active: 0,
		prefs:  Preferences{EditorScheme: EditorSchemePrefs{SemanticHighlighting: true}},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "lsp_semantic_tokens" {
				t.Fatalf("command = %q, want lsp_semantic_tokens", command)
			}
			requested <- args.(map[string]any)
			return json.RawMessage(`{
				"data":[0,4,5,0,1],
				"tokenTypes":["variable"],
				"tokenModifiers":["local"]
			}`), nil
		},
		postHook: func(fn func()) { fn() },
	}
	if got := m.semanticTokenBackgrounds(tab); len(got) != 0 {
		t.Fatalf("first frame should issue the request, got %+v", got)
	}
	select {
	case args := <-requested:
		if args["doc"] != doc.ID() {
			t.Fatalf("semantic token args = %#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lsp_semantic_tokens was not requested")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(tab.semanticRanges) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	backgrounds := m.composeTextBackgrounds(tab, nil)
	if len(backgrounds) != 1 ||
		backgrounds[0].Start != 4 || backgrounds[0].End != 9 {
		t.Fatalf("semantic backgrounds = %+v", backgrounds)
	}
	m.prefs.EditorScheme.SemanticHighlighting = false
	if got := m.composeTextBackgrounds(tab, nil); len(got) != 0 {
		t.Fatalf("disabled semantic backgrounds = %+v", got)
	}
}

func TestParseAndExportICLSRoundTripsSupportedAttributes(t *testing.T) {
	raw := `<scheme name="Harbor Test" parent_scheme="Default">
  <colors>
    <option name="CARET_ROW_COLOR" value="eef2f7" />
  </colors>
  <attributes>
    <option name="DEFAULT_KEYWORD"><value>
      <option name="FOREGROUND" value="005cc5" />
      <option name="FONT_TYPE" value="2" />
    </value></option>
  </attributes>
</scheme>`
	parsed, err := parseICLS(raw, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme.Name != "Harbor Test" || parsed.Scheme.Parent != "light" {
		t.Fatalf("scheme metadata = %+v", parsed.Scheme)
	}
	if got := parsed.Scheme.Attributes["keyword"].Foreground; got != "#005cc5" {
		t.Fatalf("keyword foreground = %q", got)
	}
	if bold := parsed.Scheme.Attributes["keyword"].Bold; bold != nil && *bold {
		t.Fatalf("font type 2 should not be bold: %+v", parsed.Scheme.Attributes["keyword"])
	}
	if italic := parsed.Scheme.Attributes["keyword"].Italic; italic == nil || !*italic {
		t.Fatalf("font type 2 should be italic: %+v", parsed.Scheme.Attributes["keyword"])
	}
	prefs := EditorSchemePrefs{
		ColorScheme:   "custom-harbor",
		CustomSchemes: map[string]CustomColorScheme{"custom-harbor": parsed.Scheme},
	}
	exported, err := exportICLS("custom-harbor", prefs)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := parseICLS(exported, "roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	if got := roundTrip.Scheme.Attributes["keyword"].Foreground; got != "#005cc5" {
		t.Fatalf("round-trip keyword foreground = %q\n%s", got, exported)
	}
}

func TestColorSchemeImportPlanningAndChoices(t *testing.T) {
	prefs := EditorSchemePrefs{
		ColorScheme: "dark",
		CustomSchemes: map[string]CustomColorScheme{
			"custom-old": {Name: "Harbor", Parent: "dark"},
		},
		ColorOverrides: map[string]ColorAttributeSet{
			"custom-old": {"keyword": {Foreground: "#50fa7b"}},
		},
	}
	next, err := applyImportedColorScheme(prefs, CustomColorScheme{
		Name: "Harbor", Parent: "dark",
	}, true, "custom-new")
	if err != nil {
		t.Fatal(err)
	}
	if next.ColorScheme != "custom-old" || len(next.CustomSchemes) != 1 {
		t.Fatalf("overwrite plan = %+v", next)
	}
	if _, ok := next.ColorOverrides["custom-old"]; ok {
		t.Fatalf("overwritten scheme kept stale overrides: %+v", next.ColorOverrides)
	}

	next, err = applyImportedColorScheme(prefs, CustomColorScheme{
		Name: "Harbor", Parent: "dark",
	}, false, "custom-new")
	if err != nil {
		t.Fatal(err)
	}
	if got := next.CustomSchemes["custom-new"].Name; got != "Harbor (2)" {
		t.Fatalf("keep-both name = %q", got)
	}
	choices := (&Model{prefs: Preferences{EditorScheme: prefs}}).
		colorSchemeChoices(knownColorSchemeChoices())
	found := false
	for _, choice := range choices {
		if choice.value == "custom-old" && choice.label == "Harbor" {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom scheme missing from choices: %+v", choices)
	}
	deleted := deleteEditorColorScheme(prefs, "custom-old")
	if deleted.ColorScheme != "dark" || len(deleted.CustomSchemes) != 0 ||
		len(deleted.ColorOverrides) != 0 {
		t.Fatalf("non-selected delete plan = %+v", deleted)
	}
	selected := prefs
	selected.ColorScheme = "custom-old"
	deleted = deleteEditorColorScheme(selected, "custom-old")
	if deleted.ColorScheme != themeDefaultColorScheme {
		t.Fatalf("selected delete plan = %+v", deleted)
	}
}

func TestImportColorSchemeWritesBackendAndLocalSettings(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "Harbor.icls")
	if err := os.WriteFile(source, []byte(`<scheme name="Harbor" parent_scheme="Darcula">
  <attributes>
    <option name="DEFAULT_STRING"><value>
      <option name="FOREGROUND" value="e5c07b" />
    </value></option>
  </attributes>
</scheme>`), 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	requests := make(chan map[string]any, 3)
	m := &Model{
		cfg:      Config{SettingsPath: settingsPath},
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Fatalf("command = %q, want settings_set", command)
			}
			request, _ := args.(map[string]any)
			requests <- request
			return json.RawMessage(`{}`), nil
		},
	}
	m.ImportColorScheme(source)
	var seen []string
	for len(seen) < 3 {
		select {
		case request := <-requests:
			seen = append(seen, request["key"].(string))
		case <-time.After(2 * time.Second):
			t.Fatalf("settings_set keys = %v", seen)
		}
	}
	if strings.Join(seen, ",") != strings.Join([]string{
		editorCustomSchemesKey, editorColorOverridesKey, editorColorSchemeKey,
	}, ",") {
		t.Fatalf("settings_set order = %v", seen)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(LoadPreferences(settingsPath).EditorScheme.CustomSchemes) == 0 &&
		time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(m.prefs.EditorScheme.CustomSchemes) != 1 {
		t.Fatalf("local custom schemes = %+v", m.prefs.EditorScheme.CustomSchemes)
	}
	stored := LoadPreferences(settingsPath)
	if len(stored.EditorScheme.CustomSchemes) != 1 {
		t.Fatalf("stored custom schemes = %+v", stored.EditorScheme.CustomSchemes)
	}
}
