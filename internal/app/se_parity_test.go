package app

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func seLoadedPane(tab seTab, query string) *sePane {
	pane := &sePane{
		open: true, tab: tab, query: query, extra: map[seSection]int{},
		filterOpen: false, fileDone: true, symbolDone: true,
		endpointDone: true, textDone: true, fileSet: "all", historyIndex: -1,
	}
	if strings.TrimSpace(query) != "" {
		pane.previewRatio = 33
	}
	return pane
}

func TestSearchEverywhereMixedAllTabMatchesOpenIDEA(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.se = seLoadedPane(seTabAll, "/api/users")
	m.se.endpoints = []openidea.PluginEndpoint{{
		Method: endpointString("POST"), Path: "/api/users", Framework: "Spring",
		Handler: endpointString("UsersController.create"),
		Location: openidea.Location{
			Path: "/project/src/UsersController.java",
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: 42, Col: 4},
			},
		},
	}}
	m.se.text = []openidea.FileMatches{{
		Path: "/project/src/Request.java", RelPath: "src/Request.java",
		Matches: []openidea.LineMatch{{
			Line: 8, Text: `requestPath("/api/users")`, Ranges: [][2]uint32{{13, 22}},
		}},
	}}
	m.se.textCount = 1
	rows := m.seRows()
	if len(rows) == 0 || rows[0].header {
		t.Fatalf("typed All results kept headers or were empty: %+v", rows)
	}
	if rows[0].item.kind != "symbol" || rows[0].item.symbol != "method" || rows[0].item.title != "POST /api/users" {
		t.Fatalf("the exact route did not rank before text: %+v", rows)
	}
	if rows[0].item.container != "UsersController.create" {
		t.Fatalf("route handler = %q", rows[0].item.container)
	}
	foundText := false
	for _, row := range rows {
		if row.item.kind == "text" {
			foundText = true
			if !strings.Contains(row.item.title, `requestPath("/api/users")`) {
				t.Fatalf("text row = %q", row.item.title)
			}
		}
	}
	if !foundText {
		t.Fatal("text matches disappeared from the All tab")
	}
}

func TestSearchEverywhereTypedSingleTabOmitsSectionHeader(t *testing.T) {
	m := &Model{}
	m.se = seLoadedPane(seTabClasses, "App")
	m.se.symbols = []openidea.WorkspaceSymbol{{
		Name: "App", Kind: "class",
		Location: openidea.Location{
			Path: "/project/src/App.java",
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: 3, Col: 6},
			},
		},
	}}
	rows := m.seRows()
	if len(rows) != 1 || rows[0].header {
		t.Fatalf("a typed Classes search kept a section header: %+v", rows)
	}
	if rows[0].item.title != "App" || rows[0].item.right != "App.java" {
		t.Fatalf("class row = %+v", rows[0].item)
	}
}

func TestSearchEverywhereMergesIndexAndLanguageServerSymbols(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	m.se = seLoadedPane(seTabSymbols, "App")
	m.se.indexSymbols = []openidea.SymbolMatch{
		{
			Name: "App", Kind: "class", Path: "/project/src/App.java", RelPath: "src/App.java",
			Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 3, Col: 6}},
		},
		{
			Name: "apply", Kind: "method", Path: "/project/src/Service.java", RelPath: "src/Service.java",
			Container: endpointString("Service"),
			Range:     openidea.OutlineRange{Start: openidea.OutlinePos{Line: 8, Col: 2}},
		},
	}
	m.se.symbols = []openidea.WorkspaceSymbol{
		{
			Name: "App", Kind: "class",
			Location: openidea.Location{
				Path:  "/project/src/App.java",
				Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 3, Col: 6}},
			},
		},
		{
			Name: "AppService", Kind: "interface",
			Location: openidea.Location{
				Path:  "/project/src/AppService.java",
				Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 1, Col: 6}},
			},
		},
	}
	rows := m.seRows()
	if len(rows) != 3 {
		t.Fatalf("index/LSP merge produced %d rows, want 3: %+v", len(rows), rows)
	}
	if rows[0].item.title != "App" || rows[1].item.title != "apply()" {
		t.Fatalf("index order was not preserved before lower-scored LSP hits: %+v", rows)
	}
	for _, row := range rows {
		if row.item.path == "/project/src/App.java" && row.item.title == "App" &&
			len(rows) > 1 && rows[2].item.path == "/project/src/App.java" {
			t.Fatalf("duplicate App symbol was not removed: %+v", rows)
		}
	}
}

func TestSearchEverywhereEmptyAllUsesRecentsNotHistoryRows(t *testing.T) {
	m := &Model{}
	m.se = seLoadedPane(seTabAll, "")
	m.se.recent = []openidea.FileMatch{{
		Path: "/project/src/App.java", RelPath: "src/App.java",
	}}
	m.recentActions = []string{"file.save"}
	rows := m.seRows()
	if len(rows) == 0 || !rows[0].header || rows[0].section != seRecentFiles {
		t.Fatalf("the empty All tab did not start with Recent Files: %+v", rows)
	}
	for _, row := range rows {
		if row.header && row.section == seSection("history") {
			t.Fatalf("Search History leaked into the result list: %+v", rows)
		}
	}
}

func TestSearchEverywhereHistoryIsPerTabAndBrowses(t *testing.T) {
	m := &Model{}
	m.rememberSearchQuery(seTabAll, "All one")
	m.rememberSearchQuery(seTabFiles, "File one")
	m.rememberSearchQuery(seTabFiles, "File two")
	if got := m.searchHistoryFor(seTabFiles); len(got) != 2 ||
		got[0] != "File two" || got[1] != "File one" {
		t.Fatalf("Files history = %v", got)
	}
	m.se = seLoadedPane(seTabFiles, "draft")
	m.BrowseSearchEverywhereHistory(1)
	if m.se.query != "File two" || m.se.historyIndex != 0 {
		t.Fatalf("Alt+Up did not show the newest Files query: %+v", m.se)
	}
	m.BrowseSearchEverywhereHistory(-1)
	if m.se.query != "draft" || m.se.historyIndex != -1 {
		t.Fatalf("Alt+Down did not return to the draft: %+v", m.se)
	}

	history := m.searchHistoryFor(seTabAll)
	m.se = nil
	m.OpenSearchEverywhere()
	if m.se == nil || history[0] != "All one" || m.se.query != "All one" {
		t.Fatalf("All tab did not reopen with its own history: history=%v se=%+v", history, m.se)
	}
	m.CloseSearchEverywhere()
}

func TestSearchEverywhereReopenTogglesScopeAndCloseAddsHistory(t *testing.T) {
	m := &Model{}
	m.OpenSearchEverywhere()
	m.se.query = "RouteFixture"
	m.se.includeNonProject = false
	m.OpenSearchEverywhere()
	if m.se == nil || !m.se.includeNonProject {
		t.Fatalf("reopening the current All tab did not toggle Include non-project items: %+v", m.se)
	}
	m.CloseSearchEverywhere()
	if got := m.searchHistoryFor(seTabAll); len(got) != 1 || got[0] != "RouteFixture" {
		t.Fatalf("closing did not remember the query: %v", got)
	}
}

func TestSearchEverywhereScopeFiltersProductionAndTest(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	m.se = seLoadedPane(seTabFiles, "Controller")
	m.se.scopeRoots = []seSourceFolder{
		{Path: "src/main/java", Kind: "source"},
		{Path: "src/test/java", Kind: "test"},
	}
	m.se.files = []openidea.FileMatch{
		{Path: "/project/src/main/java/AppController.java", RelPath: "src/main/java/AppController.java"},
		{Path: "/project/src/test/java/AppControllerTest.java", RelPath: "src/test/java/AppControllerTest.java"},
	}
	m.se.fileSet = "production"
	if rows := m.seRows(); len(rows) != 1 || rows[0].item.title != "AppController.java" {
		t.Fatalf("production scope listed %+v", rows)
	}
	m.se.fileSet = "test"
	if rows := m.seRows(); len(rows) != 1 || rows[0].item.title != "AppControllerTest.java" {
		t.Fatalf("test scope listed %+v", rows)
	}
}

func TestSearchEverywhereShiftAndPrimaryClickSelection(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	m.se = seLoadedPane(seTabFiles, "App")
	m.se.files = []openidea.FileMatch{
		{Path: "/project/src/App.java", RelPath: "src/App.java"},
		{Path: "/project/src/AppView.java", RelPath: "src/AppView.java"},
		{Path: "/project/src/AppService.java", RelPath: "src/AppService.java"},
	}
	m.selectSERow(0, recentSelectionSingle)
	m.selectSERow(2, recentSelectionRange)
	if selected := m.selectedSERows(m.seRows()); len(selected) != 3 {
		t.Fatalf("Shift-click selected %d rows, want 3", len(selected))
	}
	m.selectSERow(1, recentSelectionToggle)
	selected := m.selectedSERows(m.seRows())
	if len(selected) != 2 ||
		selected[0].item.title != "App.java" ||
		selected[1].item.title != "AppService.java" {
		t.Fatalf("primary-click toggle selected %+v", selected)
	}
	m.selectSERow(1, recentSelectionSingle)
	if selected := m.selectedSERows(m.seRows()); len(selected) != 1 ||
		selected[0].item.title != "AppView.java" {
		t.Fatalf("plain click did not replace the selection: %+v", selected)
	}
}

func TestSearchEverywhereSlashCommandsRunAndListScopes(t *testing.T) {
	command, ok := parseSECommand("/fi")
	if !ok || !command.list || len(command.rows) != 1 ||
		command.rows[0].command != seTabFiles || command.rows[0].title != "/files" {
		t.Fatalf("slash list = %+v, ok=%v", command, ok)
	}
	run, ok := parseSECommand("/files App")
	if !ok || run.list || run.tab != seTabFiles || run.rest != "App" {
		t.Fatalf("slash run = %+v, ok=%v", run, ok)
	}

	m := &Model{}
	m.se = seLoadedPane(seTabAll, "/files App")
	m.refreshSearchEverywhere()
	if m.se.tab != seTabFiles || m.se.query != "App" {
		t.Fatalf("typed command did not switch tabs: %+v", m.se)
	}
}

func TestSearchEverywhereActionsHideDisabledUntilChecked(t *testing.T) {
	const id = "test.searchEverywhere.disabledFixture"
	if err := registerAction(&Action{
		ID: id, Title: "Search Everywhere Disabled Fixture", Category: "Test",
		Enabled: func(*Model) bool { return false },
		Run:     func(*Model) {},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unregisterAction(id) })

	m := &Model{}
	m.se = seLoadedPane(seTabActions, "Disabled Fixture")
	if rows := m.seRows(); len(rows) != 0 {
		t.Fatalf("disabled actions leaked into Find Action: %+v", rows)
	}
	m.se.includeNonProject = true
	rows := m.seRows()
	if len(rows) != 1 || rows[0].item.action != id || !rows[0].item.disabled {
		t.Fatalf("Include disabled actions listed %+v", rows)
	}
}

func TestSearchEverywhereFilterPopupRendersOffscreen(t *testing.T) {
	m := &Model{}
	m.se = seLoadedPane(seTabFiles, "App")
	m.se.filterOpen = true
	m.se.languagesDone = true
	m.se.languages = []openidea.LanguageInfo{
		{ID: "java", Name: "Java", Extensions: []string{"java"}},
	}
	tt := ui.NewTester(m.View, 1440, 900)
	tt.SetDark(true)
	tt.Frame()
	for _, text := range []string{"File Types", "Java"} {
		if !tt.HasText(text) {
			t.Fatalf("the Search Everywhere filter lacks %q: %q", text, tt.Texts())
		}
	}
}

func TestSearchEverywherePreviewReadsTheSelectedSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "App.java")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\nfive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	m.setRoot(root)
	m.se = seLoadedPane(seTabFiles, "App")
	m.se.preview = true
	m.se.sel = 0
	m.se.files = []openidea.FileMatch{{Path: path, RelPath: "App.java"}}
	m.syncSearchEverywherePreview()
	if m.se.previewPath != path || len(m.se.previewLines) < 5 || m.se.previewLine != 0 {
		t.Fatalf("preview = %+v", m.se)
	}
	if !strings.Contains(strings.Join(m.se.previewLines[:5], "\n"), "five") {
		t.Fatalf("preview omitted the file: %q", m.se.previewLines)
	}
}

func TestSearchEverywherePreviewLayoutRendersOffscreen(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "App.java")
	if err := os.WriteFile(path, []byte("package demo;\n\nclass App {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	m.setRoot(root)
	m.se = seLoadedPane(seTabFiles, "App")
	m.se.preview = true
	m.se.files = []openidea.FileMatch{{Path: path, RelPath: "App.java"}}
	m.se.sel = 0
	m.syncSearchEverywherePreview()
	tt := ui.NewTester(m.View, 1440, 900)
	tt.SetDark(true)
	tt.Frame()
	for _, text := range []string{"App.java", "class App {"} {
		if !tt.HasText(text) {
			t.Fatalf("the preview lacks %q: %q", text, tt.Texts())
		}
	}
	if out := os.Getenv("SE_PREVIEW_SHOT"); out != "" {
		file, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchEverywhereDialogUsesOpenHeaderRowsAndFooter(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.se = seLoadedPane(seTabAll, "/api/users")
	m.se.preview = false
	m.se.endpoints = []openidea.PluginEndpoint{{
		Method: endpointString("POST"), Path: "/api/users", Framework: "Spring",
		Handler: endpointString("UsersController.create"),
		Location: openidea.Location{
			Path:  "/project/src/UsersController.java",
			Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 42, Col: 4}},
		},
	}}
	m.se.text = []openidea.FileMatches{{
		Path: "/project/logs/application.log", RelPath: "logs/application.log",
		Matches: []openidea.LineMatch{{Line: 12, Text: "POST /api/users", Ranges: [][2]uint32{{0, 14}}}},
	}}
	m.se.sel = 0
	tt := ui.NewTester(m.View, 1440, 900)
	tt.SetDark(false)
	tt.Frame()
	for _, text := range []string{
		"All", "Classes", "Files", "Symbols", "Actions", "Text",
		"Include non-project items", "Type / to see commands",
		"POST /api/users", "UsersController.create",
		"Open In Right Split",
	} {
		if !tt.HasText(text) {
			t.Fatalf("Search Everywhere lacks %q: %q", text, tt.Texts())
		}
	}
	if tt.HasText("2 results") || tt.HasText("Endpoints") {
		t.Fatalf("the All query kept the removed count/header row: %q", tt.Texts())
	}
	if out := os.Getenv("SE_SHOT"); out != "" {
		file, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchEverywhereMatchesOpenIDEARankingAndSettingsEntries(t *testing.T) {
	exact := seMatchText("Order", "order", true)
	prefix := seMatchText("OrderService", "order", true)
	substring := seMatchText("NewOrderService", "order", true)
	humps := seMatchText("OptimizeImports", "oi", true)
	if exact.score <= prefix.score || prefix.score <= substring.score || substring.score <= humps.score {
		t.Fatalf("ranking is not exact > prefix > substring > hump: %d %d %d %d",
			exact.score, prefix.score, substring.score, humps.score)
	}
	if got := seRunIndices(seMatchText("Optimize Imports", "imp opt", false).runs); !reflect.DeepEqual(
		got, []int{0, 1, 2, 9, 10, 11},
	) {
		t.Fatalf("multi-word indices = %v", got)
	}

	m := &Model{}
	items := m.seSettingItems("line numbers")
	if len(items) == 0 || items[0].title != "Show line numbers" {
		t.Fatalf("settings options were not indexed individually: %+v", items)
	}
	if items[0].setting == "" || len(items[0].settingPath) < 2 ||
		!reflect.DeepEqual(items[0].settingPath, []string{"Editor", "General", "Appearance"}) {
		t.Fatalf("settings entry lacks its page breadcrumb: %+v", items[0])
	}
}

func TestSearchEverywhereNavigationStopsAtEndsAndWraps(t *testing.T) {
	m := &Model{}
	m.se = seLoadedPane(seTabFiles, "App")
	m.se.files = []openidea.FileMatch{
		{Path: "/project/src/App.java", RelPath: "src/App.java"},
		{Path: "/project/src/AppView.java", RelPath: "src/AppView.java"},
	}
	m.clampSESelection()
	if m.se.sel != 0 {
		t.Fatalf("initial selection = %d", m.se.sel)
	}
	m.stepSearchEverywhere(1)
	if m.se.sel != 1 {
		t.Fatalf("next selection = %d", m.se.sel)
	}
	m.stepSearchEverywhere(1)
	if m.se.sel != 0 {
		t.Fatalf("forward wrap selection = %d", m.se.sel)
	}
	m.stepSearchEverywhere(-1)
	if m.se.sel != 1 {
		t.Fatalf("backward wrap selection = %d", m.se.sel)
	}
	m.jumpSearchEverywhereEdge(false)
	if m.se.sel != 0 {
		t.Fatalf("Home selection = %d", m.se.sel)
	}
	m.jumpSearchEverywhereEdge(true)
	if m.se.sel != 1 {
		t.Fatalf("End selection = %d", m.se.sel)
	}
}

func TestSearchEverywhereKeepsOldSectionUntilNewQueryAnswers(t *testing.T) {
	m := &Model{}
	m.se = seLoadedPane(seTabFiles, "old")
	m.se.files = []openidea.FileMatch{{
		Path: "/project/Old.java", RelPath: "Old.java",
	}}
	if rows := m.seRows(); len(rows) != 1 || rows[0].item.title != "Old.java" {
		t.Fatalf("initial rows = %+v", rows)
	}
	m.settleSearchEverywhereSnapshot()

	m.se.query = "new"
	m.se.fileDone = false
	if rows := m.seRows(); len(rows) != 1 || rows[0].item.title != "Old.java" {
		t.Fatalf("old rows disappeared before the new source answered: %+v", rows)
	}
	m.se.files = []openidea.FileMatch{{
		Path: "/project/New.java", RelPath: "New.java",
	}}
	if rows := m.seRows(); len(rows) != 1 || rows[0].item.title != "Old.java" {
		t.Fatalf("unsettled new data replaced the old rows: %+v", rows)
	}

	m.se.files = []openidea.FileMatch{{
		Path: "/project/New.java", RelPath: "New.java",
	}}
	m.se.fileDone = true
	if rows := m.seRows(); len(rows) != 1 || rows[0].item.title != "New.java" {
		t.Fatalf("new rows were not published: %+v", rows)
	}
}
