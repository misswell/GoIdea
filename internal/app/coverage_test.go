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
	for _, label := range []string{"coverage.info", "Element", "Line, %", "Branch, %", "Example.java", "75% (3/4)", "50% (1/2)"} {
		if !tt.HasText(label) {
			t.Errorf("native Coverage tool window is missing %q", label)
		}
	}
	m.ManageCoverageReports()
	tt.Frame()
	if !tt.HasText("Coverage Reports") || !tt.HasText("Show Selected") || !tt.HasText("lcov") {
		t.Fatal("native Coverage Reports manager is missing its report or controls")
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
