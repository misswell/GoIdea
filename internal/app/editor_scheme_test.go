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

func TestEditorSchemeToggleActionsReachNativeConsumers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Main.java")
	backend := &editorSchemeTestBackend{info: openidea.DocInfo{
		ID: 11, Path: path, Text: "call([{ }]) \"([{\"; // ([{\n",
		Version: 1, Language: "java",
	}}
	doc, err := docbridge.Open(context.Background(), backend, Window, path)
	if err != nil {
		t.Fatal(err)
	}
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{
		cfg:      Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		tabs:     []*Tab{tab},
		active:   0,
		theme:    openideaTheme(true),
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_set":
			return json.RawMessage(`{}`), nil
		case "lsp_semantic_tokens":
			return json.RawMessage(`{
						"data":[0,0,4,0,1],
				"tokenTypes":["variable"],
				"tokenModifiers":["local"]
			}`), nil
		default:
			t.Fatalf("command = %q", command)
			return nil, nil
		}
	}
	m.rememberEditorSchemeTestModel()
	if !m.RunAction("editor.toggleRainbowBrackets") {
		t.Fatal("rainbow brackets action did not run")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !m.ActionToggled("editor.toggleRainbowBrackets") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !m.ActionToggled("editor.toggleRainbowBrackets") {
		t.Fatal("rainbow brackets setting did not commit")
	}
	if got := m.rainbowBracketBackgrounds(tab); len(got) != 6 {
		t.Fatalf("rainbow action produced %d native ranges", len(got))
	}
	if !m.RunAction("editor.toggleSemanticHighlighting") {
		t.Fatal("semantic highlighting action did not run")
	}
	deadline = time.Now().Add(2 * time.Second)
	for !m.ActionToggled("editor.toggleSemanticHighlighting") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !m.ActionToggled("editor.toggleSemanticHighlighting") {
		t.Fatal("semantic highlighting setting did not commit")
	}
	deadline = time.Now().Add(2 * time.Second)
	for len(tab.semanticRanges) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(tab.semanticRanges) != 1 {
		t.Fatalf("semantic action produced %d native token spans", len(tab.semanticRanges))
	}
	off := false
	setting := backendEditorSchemeSettings{}
	setting.Editor.SemanticHighlighting = &off
	setting.Editor.RainbowBrackets = &off
	m.mergeBackendEditorSchemeSettings(setting)
	if got := m.composeTextBackgrounds(tab, nil); len(got) != 0 {
		t.Fatalf("backend off sync retained native highlights: %+v", got)
	}
}

func (m *Model) rememberEditorSchemeTestModel() {
	rememberEditorColorSchemeRegistry(m.prefs.EditorScheme)
	rememberEditorColorSchemeActions(m.prefs.EditorScheme)
}

func TestEditorSchemeActionsAreRegisteredAndToggled(t *testing.T) {
	for _, id := range []string{
		"editor.toggleSemanticHighlighting",
		"editor.toggleRainbowBrackets",
		"editor.colorScheme.dark",
		"editor.colorScheme.solarized-light",
		"editor.colorScheme.import",
		"editor.colorScheme.export",
		"editor.colorScheme.delete",
	} {
		action, ok := actionGet(id)
		if !ok {
			t.Fatalf("action %q is not registered", id)
		}
		if (action.Run == nil && action.RunWithArg == nil) ||
			(strings.HasPrefix(id, "editor.colorScheme.") &&
				!strings.HasSuffix(id, ".import") &&
				!strings.HasSuffix(id, ".export") &&
				!strings.HasSuffix(id, ".delete") &&
				action.RunWithArg == nil) {
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

func TestDynamicColorSchemeActionUsesTheSchemeID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	if !m.RunActionWithArg("editor.colorScheme.dark", nil) {
		t.Fatal("dynamic dark scheme action did not run")
	}
	if m.prefs.EditorScheme.ColorScheme != "dark" {
		t.Fatalf("selected color scheme = %q, want dark", m.prefs.EditorScheme.ColorScheme)
	}
	if !m.ActionEnabled("editor.colorScheme.dark") {
		t.Fatal("known color scheme action is not enabled")
	}
	if m.ActionEnabled("editor.colorScheme.missing") {
		t.Fatal("unknown color scheme action is enabled")
	}
}

func TestEditorColorSchemeDynamicActionsResolveIDsNamesAndArguments(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg:      Config{SettingsPath: settingsPath},
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		prefs: Preferences{EditorScheme: EditorSchemePrefs{
			ColorScheme: "dark",
			CustomSchemes: map[string]CustomColorScheme{
				"custom-harbor": {Name: "Harbor & Moss", Parent: "dark"},
			},
		}},
	}
	m.rememberEditorSchemeTestModel()
	if !m.ActionEnabled("editor.colorScheme.custom-harbor") {
		t.Fatal("custom scheme action did not resolve for the model")
	}
	for _, test := range []struct {
		id   string
		want string
	}{
		{"editor.colorScheme.dark", "dark"},
		{"editor.colorScheme.Islands Dark", "dark"},
		{"editor.colorScheme.solarized-light", "solarized-light"},
		{"editor.colorScheme.custom-harbor", "custom-harbor"},
		{"editor.colorScheme.Harbor & Moss", "custom-harbor"},
	} {
		action, ok := actionGet(test.id)
		if !ok {
			t.Fatalf("%q did not resolve", test.id)
		}
		if action.RunWithArg == nil || action.Title != "Color Scheme: "+
			editorColorSchemeActionLabel(m.prefs.EditorScheme, test.want) {
			t.Fatalf("%q resolved to %+v", test.id, action)
		}
	}
	if _, ok := actionGet("editor.colorScheme.import"); !ok {
		t.Fatal("the concrete import action no longer resolves")
	}
	if _, ok := actionGet("editor.colorScheme.missing"); ok {
		t.Fatal("an unknown scheme resolved")
	}

	if !m.ActionToggled("editor.colorScheme.dark") ||
		m.ActionToggled("editor.colorScheme.Islands Light") {
		t.Fatal("dynamic action toggled state does not follow the selected scheme")
	}
	if !m.RunActionWithArg("editor.colorScheme.Islands Light", nil) {
		t.Fatal("a scheme name did not run its dynamic action")
	}
	if m.prefs.EditorScheme.ColorScheme != "light" || !m.ActionToggled("editor.colorScheme.light") {
		t.Fatalf("scheme name action selected %q", m.prefs.EditorScheme.ColorScheme)
	}
	if !m.RunActionWithArg("editor.colorScheme.dark", "Harbor & Moss") {
		t.Fatal("a scheme argument did not run")
	}
	if m.prefs.EditorScheme.ColorScheme != "custom-harbor" {
		t.Fatalf("scheme argument selected %q", m.prefs.EditorScheme.ColorScheme)
	}
	if got := LoadPreferences(settingsPath).EditorScheme.ColorScheme; got != "custom-harbor" {
		t.Fatalf("stored scheme = %q", got)
	}
}

func TestEditorColorSchemeAppliesNativeTokenTheme(t *testing.T) {
	m := &Model{
		cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")},
		prefs: Preferences{EditorScheme: EditorSchemePrefs{
			ColorScheme: "custom-harbor",
			CustomSchemes: map[string]CustomColorScheme{
				"custom-harbor": {
					Name: "Harbor", Parent: "dark",
					Attributes: map[string]ColorAttributes{
						"keyword": {
							Foreground: "#112233",
							Bold:       boolPointer(true),
							Italic:     boolPointer(false),
						},
					},
				},
			},
			ColorOverrides: map[string]ColorAttributeSet{
				"custom-harbor": {
					"string": {Foreground: "#445566", Italic: boolPointer(true)},
				},
			},
		}},
	}
	theme := m.editorColorSchemeTheme()
	if theme == nil {
		t.Fatal("custom color scheme produced no native editor theme")
	}
	if theme.Background != ui.Hex("#191a1c") ||
		theme.Tokens[editor.TokenKeyword].Color != ui.Hex("#112233") ||
		!theme.Tokens[editor.TokenKeyword].Bold ||
		theme.Tokens[editor.TokenKeyword].Italic ||
		theme.Tokens[editor.TokenString].Color != ui.Hex("#445566") ||
		!theme.Tokens[editor.TokenString].Italic {
		t.Fatalf("native scheme theme = background %v, keyword %+v, string %+v",
			theme.Background, theme.Tokens[editor.TokenKeyword], theme.Tokens[editor.TokenString])
	}
	m.prefs.EditorScheme.ColorScheme = "solarized-light"
	theme = m.editorColorSchemeTheme()
	if theme == nil || theme.Background != ui.Hex("#fdf6e3") ||
		theme.Tokens[editor.TokenKeyword].Color != ui.Hex("#859900") {
		t.Fatalf("Solarized Light theme = %+v", theme)
	}
	m.prefs.EditorScheme.ColorScheme = themeDefaultColorScheme
	if theme := m.editorColorSchemeTheme(); theme != nil {
		t.Fatalf("theme-default should use the native default theme, got %+v", theme)
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
