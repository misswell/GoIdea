package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestCoverageMetricsRollUpThroughCompactDirectories(t *testing.T) {
	const root = "/project"
	// Exercise the pinned coverage_suite payload as well as the tree. Branches
	// with no counters still contribute line totals to their directories.
	const report = `{"runner":"lcov","hasBranches":true,"files":[
		{"path":"/project/src/example/main.go","lines":[{"line":0,"hits":2,"branchesTotal":2,"branchesCovered":1}],"linesTotal":4,"linesCovered":3,"branchesTotal":2,"branchesCovered":1},
		{"path":"/project/src/example/other.go","lines":[],"linesTotal":2,"linesCovered":0,"branchesTotal":0,"branchesCovered":0}
	]}`
	var data coverageData
	if err := json.Unmarshal([]byte(report), &data); err != nil {
		t.Fatal(err)
	}
	suite := makeCoverageSuite(root, "Test report", "", data, 1)
	if suite.totals.LinesTotal != 6 || suite.totals.LinesCovered != 3 || suite.totals.BranchesCovered != 1 || suite.totals.BranchesTotal != 2 {
		t.Fatalf("wrong coverage totals: %+v", suite.totals)
	}
	if len(suite.tree) != 1 || suite.tree[0].name != "src/example" || len(suite.tree[0].children) != 2 {
		t.Fatalf("wrong compact coverage tree: %+v", suite.tree)
	}
	if suite.badges[root+"/src"] != "50% lines" || suite.badges[root+"/src/example/main.go"] != "75% lines" {
		t.Fatalf("wrong coverage badges: %v", suite.badges)
	}
	sortCoverageTree(suite.tree, ui.SortOrder{Column: "branches"})
	if n := suite.tree[0].children[0]; n.name != "main.go" || n.BranchesCovered != 1 {
		t.Fatalf("branch coverage without counters must sort last: %+v", n)
	}
}

func TestCoverageReportsRestoreWithoutDeletingSourceReport(t *testing.T) {
	root := t.TempDir()
	m := &Model{cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	m.setRoot(root)
	path := filepath.Join(root, "lcov.info")
	if err := os.WriteFile(path, []byte("coverage report"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		suite := m.addCoverageSuite(root, "lcov.info", path, coverageData{Runner: "lcov"})
		m.rememberCoverageImport(suite)
	}
	if len(m.coverage.suites) != 1 || len(m.coverage.imports) != 1 {
		t.Fatalf("reimport should replace earlier suite: %+v", m.coverage)
	}
	m.coverage = coverageState{}
	m.loadCoverageImports()
	entries := m.coverage.reportEntries()
	if len(entries) != 1 || entries[0].suite != nil || entries[0].path != path {
		t.Fatalf("import history did not survive reopening the project: %+v", entries)
	}
	m.deleteCoverageReport(entries[0])
	m.coverage = coverageState{}
	m.loadCoverageImports()
	if len(m.coverage.reportEntries()) != 0 {
		t.Fatal("removed coverage report was still in the import history")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("removing a coverage report must keep its source file: %v", err)
	}
}

func TestCoverageWindowRendersLineAndBranchMetrics(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	m.addCoverageSuite(root, "coverage.info", "", coverageData{Runner: "lcov", HasBranches: true, Files: []coverageFile{{
		Path: filepath.Join(root, "Example.java"), coverageCounts: coverageCounts{LinesCovered: 3, LinesTotal: 4, BranchesCovered: 1, BranchesTotal: 2},
	}}})
	p := openideaTheme(false)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(p)
		m.coveragePanel(c, p)
		if m.coverage.manage {
			m.coverageManageDialog(c, p)
		}
	}, 620, 460)
	tt.Frame()
	for _, label := range []string{
		"coverage.info", "Element", "Line, %", "Branch, %", "Example.java",
		"75% (3/4)", "50% (1/2)",
	} {
		if !tt.HasText(label) {
			t.Errorf("native Coverage tool window is missing %q", label)
		}
	}
	m.ManageCoverageReports()
	tt.Frame()
	if !tt.HasText("Coverage Reports") || !tt.HasText("Import…") ||
		!tt.HasText("Delete") || !tt.HasText("Show Selected") || !tt.HasText("lcov") {
		t.Fatal("native Coverage Reports manager is missing its report or controls")
	}
}

func TestCoverageActionsFollowOpenIDEAContextState(t *testing.T) {
	m := &Model{}
	for _, id := range []string{
		"run.coverage", "run.coverageContext", "coverage.importReport",
		"coverage.manageReports", "coverage.show", "coverage.hide",
	} {
		if m.ActionEnabled(id) {
			t.Fatalf("%s is enabled without a project", id)
		}
	}

	root := t.TempDir()
	m.setRoot(root)
	for _, id := range []string{
		"run.coverage", "run.coverageContext", "coverage.importReport",
		"coverage.manageReports",
	} {
		if !m.ActionEnabled(id) {
			t.Fatalf("%s is disabled in an open project", id)
		}
	}
	if m.ActionEnabled("coverage.show") || m.ActionEnabled("coverage.hide") {
		t.Fatal("Show or Hide Coverage is enabled without a suite")
	}

	m.addCoverageSuite(root, "coverage.info", "", coverageData{Runner: "lcov"})
	if !m.ActionEnabled("coverage.hide") || m.ActionEnabled("coverage.show") {
		t.Fatal("a newly selected visible suite did not enable Hide Coverage")
	}
	if !m.RunAction("coverage.hide") || m.coverage.visible {
		t.Fatal("Hide Coverage did not hide the selected suite")
	}
	if !m.ActionEnabled("coverage.show") || m.ActionEnabled("coverage.hide") {
		t.Fatal("a hidden suite did not enable Show Coverage")
	}
	if !m.RunAction("coverage.show") || !m.coverage.visible {
		t.Fatal("Show Coverage did not restore the selected suite")
	}
	if !m.RunAction("coverage.manageReports") || !m.coverage.manage {
		t.Fatal("Manage Coverage Reports did not open its native overlay")
	}
}

func TestCoverageEmptyStateMatchesOpenIDEA(t *testing.T) {
	m := &Model{}
	p := openideaTheme(false)
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(p)
		m.coveragePanel(c, p)
	}, 620, 460)
	tester.Frame()
	for _, text := range []string{
		"No coverage data",
		"Run a configuration with coverage to see which lines its tests execute.",
		"Run with Coverage",
		"Import a report collected in CI from disk",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Coverage empty state is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestCoverageCollectsAnExitBeforeTheRunStartReply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	main := make(chan func(), 4)
	m := &Model{ctx: ctx, postHook: func(fn func()) { main <- fn }}
	m.setRoot(t.TempDir())
	cfg, err := openidea.DecodeRunConfig([]byte(`{"id":"test","name":"Go Tests","kind":"go","command":"go","args":["test","./..."]}`))
	if err != nil {
		t.Fatal(err)
	}
	m.run.trusted, m.run.loaded = true, true
	m.run.configs = []openidea.RunConfig{*cfg}
	const report = `{"kind":"goProfile","path":"/tmp/cover.out","module":"example.com/demo","moduleDir":"/tmp"}`
	m.invokeHook = func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		switch cmd {
		case "coverage_prepare":
			return json.RawMessage(`{"status":"ready","runner":"go test -cover","launch":{"id":"test","name":"Go Tests","kind":"go","command":"go","args":["test","-coverprofile=/tmp/cover.out","-json","./..."]},"report":` + report + `}`), nil
		case "run_start":
			m.post(func() {
				code := int32(0)
				m.acceptExit(openidea.RunExitedEvent{Run: 42, Code: &code})
			})
			return json.RawMessage(`{"id":42,"running":true}`), nil
		case "coverage_collect":
			if raw := args.(map[string]any)["report"].(json.RawMessage); string(raw) != report {
				return nil, fmt.Errorf("coverage report payload changed: %s", raw)
			}
			return json.RawMessage(`{"runner":"go test -cover","hasBranches":false,"files":[{"path":"example.go","linesTotal":3,"linesCovered":3}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected RPC %q", cmd)
		}
	}
	m.RunWithCoverage()
	for m.coverage.activeSuite() == nil {
		select {
		case fn := <-main:
			fn()
		case <-ctx.Done():
			t.Fatalf("short-lived coverage run did not collect: %s %s", m.run.err, m.coverage.err)
		}
	}
	if suite := m.coverage.activeSuite(); suite.totals.LinesCovered != 3 || suite.name != "Go Tests" || m.run.running {
		t.Fatalf("wrong completed coverage run: %+v, running = %v", suite, m.run.running)
	}
}

func TestCoverageGutterMarksFollowEditsUntilAnotherRun(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Example.java")
	doc := docbridge.Virtual(path, "one\ntwo\nthree")
	tab := &Tab{
		doc: doc,
		state: &editor.State{
			Buffer:     doc.Buffer(),
			Font:       ui.Font{Family: oiFontMono, Size: 13},
			LineHeight: editorLineHeight(13),
		},
	}
	m := &Model{tabs: []*Tab{tab}, active: 0}
	m.setRoot(root)
	m.addCoverageSuite(root, "coverage.info", "", coverageData{
		Runner: "lcov",
		Files: []coverageFile{{
			Path: path,
			Lines: []coverageLine{
				{Line: 0, Hits: 1},
				{Line: 1, Hits: 0, BranchesTotal: 2, BranchesCovered: 1},
				{Line: 2, Hits: 2, BranchesTotal: 2, BranchesCovered: 2},
			},
		}},
	})

	assertMarks := func(label string, want ...int) {
		t.Helper()
		marks := m.coverageGutterMarksForTab(tab)
		if len(marks) != len(want) {
			t.Fatalf("%s marks = %+v, want lines %v", label, marks, want)
		}
		for i, line := range want {
			if marks[i].Line != line {
				t.Fatalf("%s mark %d = %d, want %d", label, i, marks[i].Line, line)
			}
		}
	}
	assertMarks("initial", 0, 1, 2)

	doc.Buffer().Edit(editor.Edit{Start: 0, End: 0, Text: "zero\n"})
	m.refreshTabDecorations(tab)
	assertMarks("insert line above", 1, 2, 3)

	// Editing text on a covered line keeps the line's marker. Removing a
	// covered line removes only its marker.
	doc.Buffer().Edit(editor.Edit{
		Start: len("zero\nthree"),
		End:   len("zero\nthree"),
		Text:  " changed",
	})
	m.refreshTabDecorations(tab)
	if got, want := tab.state.GutterMarks, []int{1, 2, 3}; len(got) != len(want) ||
		got[0].Line != want[0] || got[1].Line != want[1] || got[2].Line != want[2] {
		t.Fatalf("same-line edit marks = %+v, want lines %v", got, want)
	}

	doc.Buffer().Edit(editor.Edit{
		Start: 0,
		End:   len("zero\n"),
		Text:  "",
	})
	m.refreshTabDecorations(tab)
	assertMarks("delete line above", 0, 1, 2)
}

func TestCoverageGutterTooltipMatchesOpenIDEATitles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Example.java")
	doc := docbridge.Virtual(path, "covered\nuncovered\npartial")
	tab := &Tab{
		doc: doc,
		state: &editor.State{
			Buffer:     doc.Buffer(),
			Font:       ui.Font{Family: oiFontMono, Size: 13},
			LineHeight: editorLineHeight(13),
		},
	}
	m := &Model{}
	suite := makeCoverageSuite(root, "coverage.info", "", coverageData{
		Runner: "lcov",
		Files: []coverageFile{{
			Path: path,
			Lines: []coverageLine{
				{Line: 0, Hits: 3},
				{Line: 1, Hits: 0, BranchesTotal: 3, BranchesCovered: 1},
				{Line: 2, Hits: 7, BranchesTotal: 4, BranchesCovered: 2},
			},
		}},
	}, 1)
	m.coverage.suites = []*coverageSuite{suite}
	m.coverage.active, m.coverage.visible = suite.id, true
	p := openideaTheme(false)
	var bounds ui.Rect
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(p)
		view := editor.View(c, tab.state).Fill()
		bounds = view.Bounds()
		m.coverageGutterTooltip(c, view, tab)
	}, 600, 180)
	tester.Frame()

	for _, tc := range []struct {
		line int
		text string
	}{
		{0, "Hits: 3"},
		{1, "Not covered, 1/3 branches covered"},
		{2, "Hits: 7, 2/4 branches covered"},
	} {
		y := bounds.Y + float32(tab.state.LineTop(tc.line)-tab.state.ScrollY()) +
			tab.state.LineHeight/2
		tester.Move(bounds.X+4, y)
		tester.Frame()
		tester.Advance(time.Second)
		if !tester.HasText(tc.text) {
			t.Fatalf("gutter tooltip for line %d is missing %q: %v",
				tc.line+1, tc.text, tester.Texts())
		}
	}
}
