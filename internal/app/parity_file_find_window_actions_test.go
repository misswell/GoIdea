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
)

func TestParityFileFindWindowActionsAreRegisteredWithNativeState(t *testing.T) {
	for _, id := range []string{
		"file.exportSettings",
		"file.importSettings",
		"file.importJetBrainsSettings",
		"file.newProjectsSettings",
		"file.openInNewWindow",
		"file.renameProject",
		"file.togglePowerSave",
		"find.field.newLine",
		"find.field.recentSearches",
		"find.openInWindow",
		"find.scrollToResults",
		"window.detachTab",
		"window.mergeAllWindows",
		"window.new",
		"window.next",
		"window.previous",
	} {
		a, ok := actionGet(id)
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		if !a.Builtin || a.Title == "" || a.Keywords == "" {
			t.Fatalf("%s metadata = %+v", id, a)
		}
		if _, ok := Command(id); !ok {
			t.Fatalf("%s is absent from the command palette", id)
		}
	}
	m := &Model{}
	if m.ActionToggled("find.scrollToResults") != true {
		t.Fatal("Scroll to Results must default on")
	}
	m.ToggleFindScrollToResults()
	if m.ActionToggled("find.scrollToResults") {
		t.Fatal("Scroll to Results did not toggle off")
	}
}

func TestWindowParityActionsDetachMergeAndCycleRealGroups(t *testing.T) {
	a := parityTestTab("/p/a.go")
	b := parityTestTab("/p/b.go")
	c := parityTestTab("/p/c.go")
	m := &Model{tabs: []*Tab{a, b, c}, active: 1}

	if !m.RunAction("window.detachTab") {
		t.Fatal("window.detachTab did not run")
	}
	if m.split == nil || m.tab() != b || len(m.split.tabs) != 2 {
		t.Fatalf("detached groups = focused %v split %v", m.tabs, m.split)
	}
	if !m.RunAction("window.next") {
		t.Fatal("window.next did not run")
	}
	if m.tab() != a || len(m.split.tabs) != 1 || m.split.tabs[0] != b {
		t.Fatalf("next group = focused %v split %v", m.tabs, m.split.tabs)
	}
	if !m.RunAction("window.previous") || m.tab() != b {
		t.Fatalf("previous group did not return the cycle: %v", m.tabs)
	}
	if !m.RunAction("window.mergeAllWindows") || m.split != nil {
		t.Fatal("window.mergeAllWindows left separate groups")
	}
	if len(m.tabs) != 3 || m.tab() != b {
		t.Fatalf("merged tabs = %v active=%d", m.tabs, m.active)
	}
}

func TestFindFieldNewLineHistoryAndScrollStatePersist(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{root: "/p", cfg: Config{SettingsPath: settingsPath}}
	if !m.RunAction("find.field.newLine") || m.search.query != "\n" {
		t.Fatalf("new-line action query = %q", m.search.query)
	}
	m.search.query = "alpha\nbeta"
	m.rememberSearchFieldQuery(m.search.query)
	if !m.RunAction("find.field.recentSearches") || !m.search.historyOpen {
		t.Fatal("recent searches did not open")
	}
	m.UseRecentSearchField(0)
	if m.search.query != "alpha\nbeta" || m.search.historyOpen {
		t.Fatalf("recent query = %q open=%v", m.search.query, m.search.historyOpen)
	}
	m.ToggleFindScrollToResults()
	if !m.search.noScrollToResults {
		t.Fatal("scroll preference was not disabled")
	}
	stored := LoadPreferences(settingsPath)
	if len(stored.SearchFieldHistory) != 1 || stored.SearchFieldHistory[0] != "alpha\nbeta" {
		t.Fatalf("stored find history = %#v", stored.SearchFieldHistory)
	}

	m.search.historyOpen = true
	tester := ui.NewTester(func(c *ui.Context) {
		m.searchRows(c, c.Theme())
	}, 1100, 700)
	tester.Frame()
	for _, text := range []string{"Recent Searches", "Scrolling Off", "alpha ↵ beta"} {
		if !tester.HasText(text) {
			t.Fatalf("Find field lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestFindFieldEnterRunsSearchAndShortcutAddsNewLine(t *testing.T) {
	started := make(chan string, 1)
	applied := make(chan struct{}, 1)
	m := &Model{
		ctx:  context.Background(),
		root: t.TempDir(),
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "search_start" {
				query := input.(map[string]any)["query"].(map[string]any)
				started <- query["pattern"].(string)
			}
			return nil, nil
		},
	}
	m.search.query = "alpha"
	tester := ui.NewTester(func(c *ui.Context) {
		m.searchRows(c, c.Theme())
	}, 900, 500)
	tester.Frame()
	tester.Key(0, ui.KeyEnter)
	select {
	case pattern := <-started:
		if pattern != "alpha" {
			t.Fatalf("Enter searched %q", pattern)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enter did not start Find")
	}
	parityWaitForSignal(t, applied, "Find request")
	if m.search.query != "alpha" {
		t.Fatalf("Enter added a newline to the query: %q", m.search.query)
	}

	tester.Key(ui.Cmd|ui.Shift, ui.KeyEnter)
	if m.search.query != "alpha\n" {
		t.Fatalf("multiline shortcut query = %q", m.search.query)
	}
}

func TestOpenFindResultsInWindowCarriesMultilineQuery(t *testing.T) {
	started := make(chan string, 1)
	applied := make(chan struct{}, 2)
	tab := parityTestTab(filepath.Join(t.TempDir(), "Source.java"))
	m := &Model{
		ctx:  context.Background(),
		root: t.TempDir(),
		tabs: []*Tab{tab},
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "search_start" {
				query := input.(map[string]any)["query"].(map[string]any)
				started <- query["pattern"].(string)
			}
			return nil, nil
		},
	}
	if !m.RunActionWithArg("find.openInWindow", "alpha\nbeta") {
		t.Fatal("find.openInWindow did not run")
	}
	select {
	case pattern := <-started:
		if pattern != "alpha\nbeta" {
			t.Fatalf("Find window searched %q", pattern)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Find window did not start its search")
	}
	parityWaitForSignal(t, applied, "Find window request")
	if !m.search.visible || m.search.query != "alpha\nbeta" {
		t.Fatalf("Find window state = %+v", m.search)
	}
}

func TestSettingsBundleExportImportRoundTripsPreferencesAndTemplates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goidea-settings.json")
	done := make(chan string, 2)
	applied := make(chan struct{}, 2)
	m := &Model{
		ctx: context.Background(),
		cfg: Config{SettingsPath: filepath.Join(dir, "settings.json")},
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args := input.(map[string]any)
			switch command {
			case "settings_export":
				bundle := map[string]any{
					"format": "openidea-settings", "version": 1,
					"settings": "version = 1\n", "storage": args["storage"],
				}
				raw, _ := json.Marshal(bundle)
				if err := os.WriteFile(args["path"].(string), raw, 0o600); err != nil {
					t.Errorf("write export: %v", err)
				}
			case "settings_import":
				raw, err := os.ReadFile(args["path"].(string))
				if err != nil {
					t.Errorf("read import: %v", err)
					return nil, err
				}
				var bundle struct {
					Storage map[string]string `json:"storage"`
				}
				if err := json.Unmarshal(raw, &bundle); err != nil {
					return nil, err
				}
				out, _ := json.Marshal(map[string]any{
					"settings": map[string]any{},
					"storage":  bundle.Storage,
				})
				done <- command
				return out, nil
			default:
				t.Errorf("unexpected RPC %s", command)
			}
			done <- command
			return json.RawMessage(`null`), nil
		},
		prefs: Preferences{
			FontSize: 17,
			Font: FontPrefs{
				Family: "JetBrains Mono",
			},
			SearchFieldHistory: []string{"alpha\nbeta"},
		},
	}
	if err := m.saveTemplates(defaultTemplatePrefs()); err != nil {
		t.Fatal(err)
	}
	if !m.RunActionWithArg("file.exportSettings", path) {
		t.Fatal("file.exportSettings did not run")
	}
	parityWaitForCall(t, done, "settings_export")
	parityWaitForSignal(t, applied, "settings export")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var exported struct {
		Storage map[string]string `json:"storage"`
	}
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Storage[goideaPreferencesStorage] == "" ||
		exported.Storage[goideaTemplatesStorage] == "" {
		t.Fatalf("bundle storage = %#v", exported.Storage)
	}

	m.prefs = Preferences{}
	m.templatePrefs = nil
	if !m.RunActionWithArg("file.importSettings", path) {
		t.Fatal("file.importSettings did not run")
	}
	parityWaitForCall(t, done, "settings_import")
	parityWaitForSignal(t, applied, "settings import")
	if m.prefs.FontSize != 17 || m.prefs.Font.Family != "JetBrains Mono" ||
		len(m.prefs.SearchFieldHistory) != 1 {
		t.Fatalf("imported preferences = %+v", m.prefs)
	}
	if templates := m.templates(); templates.Version != 1 {
		t.Fatalf("imported templates = %+v", templates)
	}
}

func TestJetBrainsSettingsPlanRendersConfirmsAndAppliesGlobalWrites(t *testing.T) {
	dir := t.TempDir()
	fontXML := `<component name="DefaultFont">` +
		`<option name="FONT_FAMILY" value="JetBrains Mono" />` +
		`<option name="FONT_SIZE" value="16" />` +
		`<option name="LINE_SPACING" value="1.25" />` +
		`<option name="USE_LIGATURES" value="true" />` +
		`</component>`
	editorXML := `<component name="EditorSettings">` +
		`<option name="STRIP_TRAILING_SPACES" value="Whole" />` +
		`<option name="IS_ENSURE_NEWLINE_AT_EOF" value="true" />` +
		`<option name="IS_WHITESPACES_SHOWN" value="true" />` +
		`<option name="USE_SOFT_WRAPS" value="MAIN_EDITOR,CONSOLE" />` +
		`</component>`
	plan := planJetBrainsSettings(filepath.Join(dir, "settings.zip"),
		[]string{
			"options/editor-font.xml", "options/editor.xml",
			"keymaps/Default.xml", "colors/Dark.icls", "templates/java.xml",
		},
		map[string]string{
			"options/editor-font.xml": fontXML,
			"options/editor.xml":      editorXML,
		})
	if len(plan.Writes) != 9 {
		t.Fatalf("JetBrains plan writes = %+v", plan.Writes)
	}
	if !strings.Contains(strings.Join(plan.Skipped, ","), "Keymap") ||
		!strings.Contains(strings.Join(plan.Skipped, ","), "Color schemes") {
		t.Fatalf("JetBrains skipped = %v", plan.Skipped)
	}

	calls := make(chan string, len(plan.Writes))
	applied := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		cfg: Config{SettingsPath: filepath.Join(dir, "settings.json")},
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Errorf("unexpected RPC %s", command)
			}
			args := input.(map[string]any)
			if args["scope"] != "global" {
				t.Errorf("JetBrains write scope = %#v", args)
			}
			calls <- args["key"].(string)
			return json.RawMessage(`{}`), nil
		},
	}
	dialog := m.openParityDialog()
	dialog.parity = parityDialogState{
		Kind: "jetbrains-import", Title: "Import JetBrains Settings",
		Body: "These settings will be imported:", Plan: &plan,
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.drawParityWorkspaceDialog(c, c.Theme(), dialog)
	}, 760, 720)
	tester.Frame()
	for _, text := range []string{
		"Import JetBrains Settings", "Font: JetBrains Mono",
		"Not imported: Keymap, Live templates, Color schemes",
	} {
		if !tester.HasText(text) {
			t.Fatalf("JetBrains import lacks %q: %q", text, tester.Texts())
		}
	}
	if err := tester.Click("Import"); err != nil {
		t.Fatal(err)
	}
	for range plan.Writes {
		select {
		case <-calls:
		case <-time.After(2 * time.Second):
			t.Fatal("JetBrains settings_set did not finish")
		}
	}
	parityWaitForSignal(t, applied, "JetBrains settings apply")
	if m.workspaceDialog != nil {
		t.Fatal("JetBrains import dialog stayed open")
	}
	if m.prefs.Font.Family != "JetBrains Mono" || m.prefs.FontSize != 16 ||
		m.prefs.Font.LineHeight != 20 || !m.prefs.Editor.SoftWrap ||
		!m.prefs.Editor.ShowWhitespace || !m.prefs.SaveFiles.TrimAllLines {
		t.Fatalf("local JetBrains settings = %+v", m.prefs)
	}
}

func TestProjectRenamePromptAndDirectRPCUpdateProjectIdentity(t *testing.T) {
	dir := t.TempDir()
	done := make(chan string, 1)
	applied := make(chan struct{}, 1)
	projectRoot := filepath.Join(dir, "old-project")
	m := &Model{
		ctx:         context.Background(),
		root:        projectRoot,
		projectName: "Old",
		cfg:         Config{SessionPath: filepath.Join(dir, "session.json")},
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		workspaceDialog: nil,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "project_rename" {
				t.Fatalf("unexpected RPC %s", command)
			}
			args := input.(map[string]any)
			if args["name"] != "New Project" {
				t.Fatalf("rename args = %#v", args)
			}
			done <- command
			return json.RawMessage(`{"root":"` + filepath.ToSlash(projectRoot) +
				`","name":"New Project","displayName":"New Project","settings":{}}`), nil
		},
	}
	if !m.RunAction("file.renameProject") || m.workspaceDialog == nil {
		t.Fatal("Rename Project did not open its prompt")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.drawParityWorkspaceDialog(c, c.Theme(), m.workspaceDialog)
	}, 640, 420)
	tester.Frame()
	for _, text := range []string{"Rename Project", "New project name:", "Leave empty to use the folder name (old-project)."} {
		if !tester.HasText(text) {
			t.Fatalf("Rename Project lacks %q: %q", text, tester.Texts())
		}
	}
	m.workspaceDialog = nil
	if !m.RunActionWithArg("file.renameProject", "New Project") {
		t.Fatal("direct Rename Project did not run")
	}
	parityWaitForCall(t, done, "project_rename")
	parityWaitForSignal(t, applied, "project rename")
	m.sessionWG.Wait()
	if m.projectName != "New Project" || m.projectLabel() != "New Project" {
		t.Fatalf("project identity = %q / %q", m.projectName, m.projectLabel())
	}
}

func TestNewProjectsSettingsUsesGlobalJavaScopeAndTitle(t *testing.T) {
	dir := t.TempDir()
	calls := make(chan string, 4)
	applied := make(chan struct{}, 2)
	m := &Model{
		ctx:  context.Background(),
		root: dir,
		cfg:  Config{SettingsPath: filepath.Join(dir, "settings.json")},
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "settings_get_global":
				calls <- command
				return json.RawMessage(`{"languages":{"java":{"jdk":"21"}}}`), nil
			case "sdk_detect":
				calls <- command
				return json.RawMessage(`[]`), nil
			case "languages":
				calls <- command
				return json.RawMessage(`[]`), nil
			default:
				t.Errorf("unexpected RPC %s", command)
				return nil, context.Canceled
			}
		},
	}
	if !m.RunActionWithArg("file.newProjectsSettings", settingsPageJava) {
		t.Fatal("Settings for New Projects did not run")
	}
	if !m.settingsOpen || m.settingsPage != settingsPageJava ||
		!m.settingsDraft.newProjects || m.javaSettings.scope != "global" {
		t.Fatalf("new-project settings = open %v page %q mode %v scope %q",
			m.settingsOpen, m.settingsPage, m.settingsDraft.newProjects, m.javaSettings.scope)
	}
	m.SetLanguageServerScope("project")
	if m.languageToolSettings.scope != "global" {
		t.Fatalf("new-project settings selected project scope %q", m.languageToolSettings.scope)
	}
	parityWaitForSignal(t, applied, "Java settings")
	parityWaitForSignal(t, applied, "language server settings")
	for range 4 {
		select {
		case <-calls:
		case <-time.After(2 * time.Second):
			t.Fatal("New Projects settings RPC did not finish")
		}
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsTitle(c, oiPaletteOf(c.Theme()))
	}, 500, 60)
	tester.Frame()
	if !tester.HasText("Settings for New Projects") {
		t.Fatalf("new-project title is missing: %q", tester.Texts())
	}
}

func TestPowerSaveWritesGlobalSettingStopsEditorSurfacesAndPersists(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	done := make(chan map[string]any, 2)
	applied := make(chan struct{}, 2)
	doc := docbridge.Virtual("/p/A.java", "class A {}")
	tab := &Tab{
		doc: doc,
		state: &editor.State{
			Buffer: doc.Buffer(),
			OnCompletion: func(editor.CompletionRequest) {
			},
			OnHover: func(editor.HoverRequest) {
			},
		},
		inline: inlineCompletionState{
			choices: []inlineCompletionChoice{{text: "suggestion"}},
			timer:   time.AfterFunc(time.Hour, func() {}),
		},
	}
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
		cfg:  Config{SettingsPath: settingsPath},
		postHook: func(fn func()) {
			fn()
			applied <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Fatalf("unexpected RPC %s", command)
			}
			args := input.(map[string]any)
			done <- args
			return json.RawMessage(`{}`), nil
		},
	}
	if !m.RunAction("file.togglePowerSave") {
		t.Fatal("Power Save Mode did not run")
	}
	args := parityWaitForArgs(t, done)
	parityWaitForSignal(t, applied, "Power Save Mode enable")
	if args["scope"] != "global" || args["key"] != "general.powerSaveMode" || args["value"] != "true" {
		t.Fatalf("power-save write = %#v", args)
	}
	if !m.prefs.General.PowerSaveMode || !m.ActionToggled("file.togglePowerSave") ||
		tab.state.OnCompletion != nil || tab.state.OnHover != nil ||
		tab.inlineCompletionVisible() || m.inlineCompletionAllowed(tab) {
		t.Fatalf("power-save state = %+v", m.prefs.General)
	}
	if stored := LoadPreferences(settingsPath); !stored.General.PowerSaveMode {
		t.Fatal("Power Save Mode did not persist")
	}
	m.QuickDocumentation()
	if !strings.Contains(m.status, "documentation popups are off") {
		t.Fatalf("Power Save documentation status = %q", m.status)
	}
	if !m.RunAction("file.togglePowerSave") {
		t.Fatal("Power Save Mode did not toggle off")
	}
	args = parityWaitForArgs(t, done)
	parityWaitForSignal(t, applied, "Power Save Mode disable")
	if args["value"] != "false" || tab.state.OnCompletion == nil || tab.state.OnHover == nil ||
		!m.inlineCompletionAllowed(tab) {
		t.Fatalf("power-save restore = %#v", args)
	}
}

func parityTestTab(path string) *Tab {
	doc := docbridge.Virtual(path, "")
	return &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
}

func parityWaitForCall(t *testing.T, calls <-chan string, want string) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("RPC = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", want)
	}
}

func parityWaitForSignal(t *testing.T, calls <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not finish", what)
	}
}

func parityWaitForArgs(t *testing.T, calls <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case args := <-calls:
		return args
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for settings_set")
		return nil
	}
}
