package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// runAnythingTester draws the workbench with the Run Anything popup over it,
// the way View composes the overlay.
func runAnythingTester(t *testing.T, m *Model) *ui.Tester {
	t.Helper()
	if m.mgr == nil {
		m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.workbench(c, c.Theme())
		m.runAnythingDialog(c, c.Theme())
	}, 1440, 900)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	return tt
}

func TestRunAnythingMatchesEveryWord(t *testing.T) {
	cases := []struct {
		text, query string
		want        bool
	}{
		{"mvn -pl api test", "", true},
		{"mvn -pl api test", "   ", true},
		{"mvn -pl api test", "MVN test", true},
		{"mvn -pl api test", "api mvn", true},
		{"mvn -pl api test", "gradle", false},
		{"Application", "app", true},
	}
	for _, c := range cases {
		if got := runAnythingMatches(c.text, c.query); got != c.want {
			t.Errorf("runAnythingMatches(%q, %q) = %v, want %v", c.text, c.query, got, c.want)
		}
	}
}

func TestRunAnythingConfigsPutsPrefixMatchesFirst(t *testing.T) {
	configs := []openidea.RunConfig{
		{Name: "Run Tests"},
		{Name: "All Tests"},
		{Name: "Application"},
		{Name: "test the api"},
	}
	got := runAnythingConfigs(configs, "test")
	want := []int{3, 0, 1}
	if len(got) != len(want) {
		t.Fatalf("matches = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matches = %v, want %v", got, want)
		}
	}
	if got := runAnythingConfigs(configs, "zzz"); len(got) != 0 {
		t.Fatalf("nonsense query matched %v", got)
	}
}

func TestBuildRunAnythingRowsCapsEachSection(t *testing.T) {
	var history []string
	for i := 0; i < 12; i++ {
		history = append(history, "cmd "+string(rune('a'+i)))
	}
	var configs []openidea.RunConfig
	for i := 0; i < 12; i++ {
		configs = append(configs, openidea.RunConfig{Name: "Config " + string(rune('a'+i))})
	}
	rows := buildRunAnythingRows("", history, configs)
	if !rows[0].isItem && rows[0].section != "Recent" || !rows[1].isItem {
		t.Fatalf("rows do not open with the Recent section: %+v", rows[0])
	}
	v := runAnythingState{rows: rows}
	if n := v.itemCount(); n != 16 {
		t.Fatalf("listed %d items, want %d", n, 2*runAnythingGroupLimit)
	}
	var sections []string
	for _, r := range rows {
		if r.section != "" {
			sections = append(sections, r.section)
		}
	}
	if len(sections) != 2 || sections[0] != "Recent" || sections[1] != "Run Configurations" {
		t.Fatalf("sections = %v", sections)
	}
}

func TestBuildRunAnythingRowsFiltersRecent(t *testing.T) {
	rows := buildRunAnythingRows("npm", []string{"mvn test", "npm run dev"}, nil)
	if len(rows) != 2 || rows[1].label != "npm run dev" {
		t.Fatalf("matching rows = %+v", rows)
	}
	rows = buildRunAnythingRows("", nil, nil)
	if len(rows) != 0 {
		t.Fatalf("empty popup listed %+v", rows)
	}
}

func TestStepRunAnythingWrapsAndStartsAtEnds(t *testing.T) {
	m := &Model{}
	m.runAnything = &runAnythingState{open: true, sel: -1, rows: []runAnythingRow{
		{section: "Recent"},
		{isItem: true, label: "a"},
		{isItem: true, label: "b"},
	}}
	m.stepRunAnything(-1)
	if m.runAnything.sel != 1 {
		t.Fatalf("sel = %d, want 1", m.runAnything.sel)
	}
	m.stepRunAnything(1)
	if m.runAnything.sel != 0 {
		t.Fatalf("sel did not wrap to 0: %d", m.runAnything.sel)
	}
	if row := m.selectedRunAnythingRow(); row == nil || row.label != "a" {
		t.Fatalf("selected row = %+v", row)
	}
}

func TestCompleteRunAnythingFillsTheFirstItem(t *testing.T) {
	m := &Model{}
	m.runHistory = nil
	m.runAnything = &runAnythingState{open: true, sel: -1, rows: []runAnythingRow{
		{isItem: true, label: "mvn test", line: "mvn test"},
	}}
	m.completeRunAnything()
	if m.runAnything.query != "mvn test" {
		t.Fatalf("query = %q", m.runAnything.query)
	}
}

func TestRunAnythingShellConfig(t *testing.T) {
	root := t.TempDir()
	cfg, raw, err := runAnythingShellConfig("go test ./...", root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "go test ./..." || cfg.Kind != "external" {
		t.Fatalf("configuration = %+v", cfg)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["kind"] != "external" || body["cwd"] != root || body["command"] != "/bin/sh" {
		t.Fatalf("payload = %v", body)
	}
	if body["detected"] != false || body["isTest"] != false {
		t.Fatalf("payload claims a detected configuration: %v", body)
	}
	args, _ := body["args"].([]any)
	if len(args) != 2 || args[0] != "-c" || args[1] != "go test ./..." {
		t.Fatalf("args = %v", args)
	}
}

func TestRunAnythingShellConfigTruncatesALongName(t *testing.T) {
	line := strings.Repeat("x", 80)
	cfg, _, err := runAnythingShellConfig(line, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if r := []rune(cfg.Name); len(r) != 58 || !strings.HasSuffix(cfg.Name, "…") {
		t.Fatalf("name = %q", cfg.Name)
	}
}

// The popup is OpenIDEA's RunAnything: its title, the command line, the
// configuration group and the footer sentence.
func TestRunAnythingDialogRenders(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.run.loaded = true
	m.run.loading = true
	m.run.configs = []openidea.RunConfig{{ID: "app", Name: "Application", Kind: "application"}}
	m.runHistory = map[string][]string{m.Root(): {"mvn test"}}
	if !m.RunCommand("runAnything.show") {
		t.Fatalf("runAnything.show is not a registry action")
	}
	tt := runAnythingTester(t, m)
	for _, text := range []string{
		"Run Anything", "Clear History",
		"Recent", "mvn test", "Run Configurations", "Application",
		"Enter to run, Tab to complete. Other commands run in the project directory.",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not in the popup: %q", text, tt.Texts())
		}
	}
}

// Without history or configurations the popup asks for a command, and a typed
// line promises to run in the project folder.
func TestRunAnythingEmptyStates(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.OpenRunAnything()
	tt := runAnythingTester(t, m)
	if !tt.HasText("Type a command") {
		t.Fatalf("empty popup lacks its hint: %q", tt.Texts())
	}
	if tt.HasText("Clear History") {
		t.Fatalf("Clear History is offered with no history")
	}
	m.runAnything.query = "echo hi"
	m.refreshRunAnything()
	m.invalidate()
	tt.Frame()
	if !tt.HasText("Press Enter to run the command in the project directory") {
		t.Fatalf("typed line lacks its hint: %q", tt.Texts())
	}
}

// The gear menu opens with Run Anything, then a rule, as TitleBar.tsx does.
func TestGearMenuOpensWithRunAnything(t *testing.T) {
	g := gearGroup(&Model{})
	if g[0].label != "Run Anything…" || g[0].command != "runAnything.show" {
		t.Fatalf("first gear item = %+v", g[0])
	}
	if !g[1].sep {
		t.Fatalf("Run Anything is not followed by a rule: %+v", g[1])
	}
}
