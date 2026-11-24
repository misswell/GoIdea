package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const intellijThemeJSON = `{
  "name": "Deep Sea",
  "dark": true,
  "colors": { "base": "#101820", "accent": "#33AAFF" },
  "ui": {
    "*": {
      "background": "base",
      "foreground": "#c0c8d0",
      "selectionBackground": "#203040"
    },
    "Button": { "default": { "startBackground": "accent" } },
    "Popup.background": "#181f28",
    "Label.infoForeground": "#80888F80",
    "EditorTabs": {
      "underlineColor": "#405060",
      "underlinedBorderColor": "#bd93f9"
    },
    "CompletionPopup.matchForeground": "#ff79c6"
  }
}`

const intellijICLS = `<scheme name="Deep Sea" version="142" parent_scheme="Darcula">
  <colors>
    <option name="CARET_COLOR" value="ffcc00" />
    <option name="LINE_NUMBERS_COLOR" value="0" />
    <option name="UNKNOWN_KEY" value="123456" />
    <option name="GUTTER_BACKGROUND" value="313335" />
  </colors>
  <attributes>
    <option name="DEFAULT_KEYWORD">
      <value><option name="FOREGROUND" value="cc7832" /></value>
    </option>
    <option name="DEFAULT_LINE_COMMENT">
      <value>
        <option name="FOREGROUND" value="808080" />
        <option name="FONT_TYPE" value="2" />
      </value>
    </option>
    <option name="TEXT">
      <value>
        <option name="FOREGROUND" value="a9b7c6" />
        <option name="BACKGROUND" value="2b2b2b" />
      </value>
    </option>
    <option name="ERRORS_ATTRIBUTES">
      <value><option name="EFFECT_COLOR" value="ff0000" /></value>
    </option>
    <option name="DEFAULT_CONSTANT" baseAttributes="DEFAULT_IDENTIFIER" />
    <option name="DEFAULT_INSTANCE_FIELD"><value /></option>
  </attributes>
</scheme>`

func TestConvertIntellijThemeMapsKeysNamedColorsAndWildcard(t *testing.T) {
	converted, err := convertIntellijTheme(intellijThemeJSON, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if converted.Theme.ID != "deep-sea" || converted.Theme.Name != "Deep Sea" ||
		converted.Theme.Kind != "dark" {
		t.Fatalf("theme = %+v", converted.Theme)
	}
	want := map[string]string{
		"--bg-app":              "#101820",
		"--accent":              "#33aaff",
		"--bg-popup":            "#181f28",
		"--bg-selected":         "#203040",
		"--fg-muted":            "#80888f80",
		"--fg":                  "#c0c8d0",
		"--border-tab-selected": "#bd93f9",
		"--fg-completion-match": "#ff79c6",
	}
	for token, color := range want {
		if got := converted.Theme.Tokens[token]; got != color {
			t.Errorf("%s = %q, want %q", token, got, color)
		}
	}
	if converted.Scheme != nil {
		t.Fatal("a UI theme without an .icls produced a scheme")
	}
}

func TestConvertIntellijICLSMatchesOpenIDEAMapping(t *testing.T) {
	scheme := convertIntellijICLS(intellijICLS)
	if scheme.Name != "Deep Sea" {
		t.Fatalf("scheme name = %q", scheme.Name)
	}
	want := map[string]string{
		"--caret":                   "#ffcc00",
		"--fg-line-number":          "#000000",
		"--syn-keyword":             "#cc7832",
		"--syn-comment":             "#808080",
		"--syn-comment-style":       "italic",
		"--fg-editor":               "#a9b7c6",
		"--bg-editor":               "#2b2b2b",
		"--bg-editor-gutter":        "#2b2b2b",
		"--wave-error":              "#ff0000",
		"--editor-error-decoration": "underline wavy #ff0000",
		"--syn-property":            "#a9b7c6",
	}
	for token, color := range want {
		if got := scheme.Tokens[token]; got != color {
			t.Errorf("%s = %q, want %q", token, got, color)
		}
	}
	if _, exists := scheme.Tokens["--syn-constant"]; exists {
		t.Fatal("a baseAttributes-only option overrode the parent scheme")
	}
	if dark, known := inferIntellijSchemeDark(intellijICLS); !known || !dark {
		t.Fatalf("Darcula parent = %v, %v", dark, known)
	}
}

func TestIntellijThemePackageCarriesEveryThemeAndScheme(t *testing.T) {
	vendor, version := "Someone", "2.3"
	source := openidea.IntellijThemeSource{
		Name: "Deep Sea Themes", PluginID: intellijStringPointer("com.example.DeepSea"),
		Version: &version, Vendor: &vendor,
		Themes: []openidea.IntellijThemeFile{
			{ID: "deep-id", Theme: intellijThemeJSON, EditorScheme: intellijStringPointer(intellijICLS)},
			{ID: "copy-id", Theme: intellijThemeJSON},
			{ID: "light-id", Theme: `{"name":"Shallow","dark":false}`},
		},
	}
	pkg, err := intellijThemePackageFromSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.ID != "intellij.com-example-deepsea" {
		t.Fatalf("package id = %q", pkg.ID)
	}
	var paths []string
	for _, file := range pkg.Files {
		paths = append(paths, file.Path)
	}
	wantPaths := []string{
		"plugin.toml",
		"themes/deep-sea.theme.json",
		"themes/deep-sea.scheme.json",
		"themes/deep-sea-2.theme.json",
		"themes/shallow.theme.json",
	}
	if strings.Join(paths, "\n") != strings.Join(wantPaths, "\n") {
		t.Fatalf("paths = %v", paths)
	}
	manifest := pkg.Files[0].Text
	for _, text := range []string{
		`id = "intellij.com-example-deepsea"`,
		`version = "2.3.0"`,
		`authors = ["Someone"]`,
		`engines.openidea = "^1.0"`,
		`editor-scheme = "themes/deep-sea.scheme.json"`,
	} {
		if !strings.Contains(manifest, text) {
			t.Errorf("manifest lacks %q:\n%s", text, manifest)
		}
	}
	if count := strings.Count(manifest, "[[contributes.themes]]"); count != 3 {
		t.Fatalf("manifest has %d theme contributions", count)
	}
	var shallow intellijThemeFile
	if err := json.NewDecoder(strings.NewReader(pkg.Files[4].Text)).Decode(&shallow); err != nil {
		t.Fatal(err)
	}
	if shallow.ID != "shallow" || shallow.Kind != "light" {
		t.Fatalf("shallow theme = %+v", shallow)
	}
}

func TestIntellijPluginManifestEscapesTOMLStrings(t *testing.T) {
	info := intellijThemePackageInfo{
		ID: "theme-sea", Name: `Sea "Deep"`, Version: "1.0.0",
		Description: "a\\b\nc", Authors: []string{"Someone"},
	}
	converted := intellijConvertedTheme{
		Theme:  intellijThemeFile{ID: "sea", Name: "Sea", Kind: "dark", Tokens: map[string]string{}},
		Scheme: &intellijSchemeFile{Name: "Sea", Tokens: map[string]string{}},
	}
	manifest := intellijPluginManifest(info, []intellijConvertedTheme{converted},
		"Converted from\nDeepSea.theme.json")
	for _, want := range []string{
		"# Converted from DeepSea.theme.json\n",
		`name = "Sea \"Deep\""`,
		`description = "a\\b\u000ac"`,
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest lacks %q:\n%s", want, manifest)
		}
	}
}

func TestGroupIntellijThemeRowsKeepsBackendOrder(t *testing.T) {
	themes := []openidea.InstalledIntellijTheme{
		{IDE: "IntelliJ IDEA 2025.2", Plugin: "Deep Sea Themes", Path: "/ij/DeepSea",
			ThemeID: "shallow", ThemeName: "Shallow", Active: true},
		{IDE: "IntelliJ IDEA 2025.2", Plugin: "Deep Sea Themes", Path: "/ij/DeepSea",
			ThemeID: "deep", ThemeName: "Deep Sea"},
		{IDE: "DataGrip 2026.2", Plugin: "Night Owl", Path: "/dg/NightOwl",
			ThemeID: "night-owl", ThemeName: "Night Owl"},
	}
	rows := groupIntellijThemeRows(themes)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if !rows[0].Active || rows[0].Plugin != "Deep Sea Themes" || len(rows[0].Themes) != 2 {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].Active || rows[1].IDE != "DataGrip 2026.2" {
		t.Fatalf("second row = %+v", rows[1])
	}
}

func TestIntellijThemeChooserLoadsAndRendersNativeOverlay(t *testing.T) {
	themes := []openidea.InstalledIntellijTheme{
		{IDE: "IntelliJ IDEA 2025.2", Plugin: "Deep Sea Themes", Path: "/ij/DeepSea",
			ThemeID: "shallow", ThemeName: "Shallow", Active: true},
		{IDE: "IntelliJ IDEA 2025.2", Plugin: "Deep Sea Themes", Path: "/ij/DeepSea",
			ThemeID: "deep", ThemeName: "Deep Sea"},
		{IDE: "DataGrip 2026.2", Plugin: "Night Owl", Path: "/dg/NightOwl",
			ThemeID: "night-owl", ThemeName: "Night Owl"},
	}
	m := &Model{
		ctx:      context.Background(),
		postHook: func(run func()) { run() },
	}
	m.root = "/project"
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "plugin_installed_intellij_themes" {
			return nil, fmt.Errorf("unexpected RPC %s", command)
		}
		return json.Marshal(themes)
	}
	m.ShowIntellijThemes()
	waitForIntellijTheme(t, func() bool {
		return m.intellijThemes != nil && !m.intellijThemes.loading
	})
	if m.intellijThemes.selected != 0 || len(m.intellijThemes.rows) != 2 {
		t.Fatalf("chooser = %+v", m.intellijThemes)
	}
	overlay := overlays.byID["plugins.intellijThemes"]
	if overlay == nil || !overlay.Modal || !overlay.Open(m) {
		t.Fatalf("plugins.intellijThemes overlay = %+v", overlay)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.intellijThemesDialog(c, c.Theme())
	}, 760, 620)
	tester.Frame()
	for _, text := range []string{
		"Import Theme from JetBrains IDE",
		"Deep Sea Themes",
		"In use",
		"2 themes · IntelliJ IDEA 2025.2",
		"Shallow, Deep Sea",
		"Night Owl",
		"1 theme · DataGrip 2026.2",
	} {
		if _, ok := tester.Find(text); !ok {
			t.Errorf("chooser lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestImportIntellijThemeStagesConvertsPreviewsAndConsents(t *testing.T) {
	source := openidea.IntellijThemeSource{
		Name: "Deep Sea Themes", PluginID: intellijStringPointer("com.example.deepsea"),
		Version: intellijStringPointer("2.3"), Vendor: intellijStringPointer("Someone"),
		Themes: []openidea.IntellijThemeFile{
			{ID: "deep", Theme: intellijThemeJSON, EditorScheme: intellijStringPointer(intellijICLS)},
		},
	}
	preview := openidea.PluginInstallPreview{
		ID: "intellij.com-example-deepsea", Name: "Deep Sea Themes", Version: "2.3.0",
		NeedsConsent: true, Unsigned: true,
		Capabilities: []openidea.PluginCapability{
			{ID: "net.http:example", Risk: "medium", Optional: true},
			{ID: "net:ask", Risk: "low", Optional: true},
		},
	}
	var (
		mu      sync.Mutex
		calls   []string
		files   []openidea.PluginPackageFile
		consent openidea.PluginConsent
	)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(run func()) { run() },
	}
	m.root = "/project"
	m.invokeHook = func(_ context.Context, command string, raw any) (json.RawMessage, error) {
		mu.Lock()
		calls = append(calls, command)
		mu.Unlock()
		switch command {
		case "plugin_read_intellij_theme":
			return json.Marshal(source)
		case "plugin_stage_package":
			input := raw.(map[string]any)
			mu.Lock()
			files = input["files"].([]openidea.PluginPackageFile)
			mu.Unlock()
			return json.Marshal("/app/plugins/.downloads/generated")
		case "plugin_preview_install":
			return json.Marshal(preview)
		case "plugin_install_dir":
			input := raw.(map[string]any)
			mu.Lock()
			consent = input["consent"].(openidea.PluginConsent)
			mu.Unlock()
			return json.Marshal(openidea.PluginInfo{
				ID: preview.ID, Name: preview.Name, Version: preview.Version,
				Enabled: true, Status: "static",
			})
		default:
			return nil, fmt.Errorf("unexpected RPC %s", command)
		}
	}
	m.ImportIntellijTheme("/downloads/DeepSea.zip")
	waitForIntellijTheme(t, func() bool {
		return m.intellijConsent != nil
	})
	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	gotFiles := append([]openidea.PluginPackageFile(nil), files...)
	mu.Unlock()
	wantCalls := []string{
		"plugin_read_intellij_theme", "plugin_stage_package", "plugin_preview_install",
	}
	if strings.Join(gotCalls, "\n") != strings.Join(wantCalls, "\n") {
		t.Fatalf("calls = %v", gotCalls)
	}
	wantFiles := []string{
		"plugin.toml",
		"themes/deep-sea.theme.json",
		"themes/deep-sea.scheme.json",
	}
	for i, file := range gotFiles {
		if file.Path != wantFiles[i] {
			t.Errorf("file %d = %q, want %q", i, file.Path, wantFiles[i])
		}
	}
	if m.intellijConsent.accepted["net.http:example"] {
		t.Fatal("an optional medium-risk permission started enabled")
	}
	if !m.intellijConsent.accepted["net:ask"] {
		t.Fatal("an optional low-risk permission did not start enabled")
	}

	overlay := overlays.byID["plugins.consent"]
	if overlay == nil || !overlay.Modal || !overlay.Open(m) {
		t.Fatalf("plugins.consent overlay = %+v", overlay)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.intellijConsentDialog(c, c.Theme())
	}, 640, 720)
	tester.Frame()
	for _, text := range []string{
		"Install Deep Sea Themes 2.3.0?",
		"This package is not signed. Only continue if you trust where it came from.",
		"net.http:example",
		"net:ask",
		"Install",
	} {
		if _, ok := tester.Find(text); !ok {
			t.Errorf("consent sheet lacks %q: %q", text, tester.Texts())
		}
	}

	m.installIntellijThemeConsent()
	waitForIntellijTheme(t, func() bool {
		return m.intellijConsent == nil
	})
	mu.Lock()
	gotCalls = append([]string(nil), calls...)
	gotConsent := consent
	mu.Unlock()
	if len(gotCalls) != 4 || gotCalls[3] != "plugin_install_dir" {
		t.Fatalf("calls after install = %v", gotCalls)
	}
	if strings.Join(gotConsent.Optional, ",") != "net:ask" {
		t.Fatalf("consent = %+v", gotConsent)
	}
	if !strings.Contains(m.status, "Installed Deep Sea Themes 2.3.0") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestImportIntellijThemeInstallsWithoutConsentWhenPreviewAllowsIt(t *testing.T) {
	source := openidea.IntellijThemeSource{
		Name:   "Plain Theme",
		Themes: []openidea.IntellijThemeFile{{ID: "plain", Theme: `{"name":"Plain","dark":false}`}},
	}
	preview := openidea.PluginInstallPreview{
		ID: "intellij.plain-theme", Name: "Plain Theme", Version: "1.0.0",
		NeedsConsent: false,
		Capabilities: []openidea.PluginCapability{
			{ID: "net:ask", Risk: "low", Optional: true, Granted: true},
		},
	}
	var (
		mu      sync.Mutex
		consent openidea.PluginConsent
	)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(run func()) { run() },
	}
	m.root = "/project"
	m.invokeHook = func(_ context.Context, command string, raw any) (json.RawMessage, error) {
		switch command {
		case "plugin_read_intellij_theme":
			return json.Marshal(source)
		case "plugin_stage_package":
			return json.Marshal("/app/plugins/.downloads/plain")
		case "plugin_preview_install":
			return json.Marshal(preview)
		case "plugin_install_dir":
			mu.Lock()
			consent = raw.(map[string]any)["consent"].(openidea.PluginConsent)
			mu.Unlock()
			return json.Marshal(openidea.PluginInfo{
				ID: preview.ID, Name: preview.Name, Version: preview.Version,
				Enabled: true, Status: "static",
			})
		default:
			return nil, fmt.Errorf("unexpected RPC %s", command)
		}
	}
	m.ImportIntellijTheme("/downloads/Plain.theme.json")
	waitForIntellijTheme(t, func() bool {
		return strings.Contains(m.status, "Installed Plain Theme 1.0.0")
	})
	if m.intellijConsent != nil {
		t.Fatal("a preview without needsConsent opened the permission sheet")
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(consent.Optional, ",") != "net:ask" {
		t.Fatalf("automatic consent = %+v", consent)
	}
}

func TestIntellijThemeActionsUseOpenIDEAIdentity(t *testing.T) {
	fromFile, ok := Command("plugins.importIntellijTheme")
	if !ok || fromFile.title != "Import IntelliJ Theme from File…" || fromFile.group != "Plugins" {
		t.Fatalf("plugins.importIntellijTheme = %+v, %v", fromFile, ok)
	}
	installed, ok := Command("plugins.importInstalledIntellijTheme")
	if !ok || installed.title != "Import Theme from JetBrains IDE…" || installed.group != "Plugins" {
		t.Fatalf("plugins.importInstalledIntellijTheme = %+v, %v", installed, ok)
	}
	m := &Model{}
	if m.ActionEnabled("plugins.importIntellijTheme") ||
		m.ActionEnabled("plugins.importInstalledIntellijTheme") {
		t.Fatal("a theme import action is enabled without a project")
	}
	m.root = "/project"
	if !m.ActionEnabled("plugins.importIntellijTheme") ||
		!m.ActionEnabled("plugins.importInstalledIntellijTheme") {
		t.Fatal("theme import actions stay disabled with a project")
	}
}

func intellijStringPointer(value string) *string {
	return &value
}

func waitForIntellijTheme(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("IntelliJ theme state did not settle")
}
