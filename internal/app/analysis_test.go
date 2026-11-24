package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestAnalysisActionsAndToolWindowsRegistered(t *testing.T) {
	for _, id := range []string{
		"analyze.inspectCode",
		"analyze.inspectFile",
		"analyze.locateDuplicates",
		"code.cleanup",
		"code.cleanupFile",
		"find.structural",
		"find.replaceStructural",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("action %s is not registered", id)
		}
	}
	for _, id := range []string{
		analysisInspectionWindow,
		analysisDuplicatesWindow,
		analysisStructuralWindow,
		analysisCleanupWindow,
	} {
		if dockSpec(id) == nil {
			t.Errorf("tool window %s is not registered", id)
		}
	}
}

func TestAnalysisInspectionRawResultsReuseChosenProfile(t *testing.T) {
	source := "javac"
	code := "unused"
	m := &Model{}
	m.inspectionsSettings.globalDraft = inspectionsSettings{
		Profile: "default",
		Default: map[string]inspectionOverrideSettings{},
	}
	state := &m.analysis.inspection
	state.profile = "default"
	state.rawFiles = map[string]analysisInspectedFile{
		"/project/App.java": {
			Path: "/project/App.java", RelPath: "App.java",
			Diagnostics: []openidea.Diagnostic{
				{
					Range: analysisRangeForTest(1, 0, 1, 4), Severity: "warning",
					Message: "Unused value", Source: &source, Code: &code,
				},
				{
					Range: analysisRangeForTest(2, 0, 2, 1), Severity: "error",
					Message: "Missing semicolon", Source: &source,
				},
			},
		},
	}
	m.syncAnalysisInspectionResults()
	if got := len(m.analysisInspectionProblems()); got != 2 {
		t.Fatalf("default profile problems = %d, want 2", got)
	}
	if got := state.rawFiles["/project/App.java"].Diagnostics[0].Severity; got != "warning" {
		t.Fatalf("raw diagnostic severity changed to %q", got)
	}

	off := false
	m.inspectionsSettings.globalDraft.Default["lsp:javac:unused"] =
		inspectionOverrideSettings{Enabled: &off}
	m.syncAnalysisInspectionResults()
	if got := len(m.analysisInspectionProblems()); got != 1 {
		t.Fatalf("code-disabled profile problems = %d, want 1", got)
	}
	if got := state.rawFiles["/project/App.java"].Diagnostics[0].Severity; got != "warning" {
		t.Fatalf("profile projection mutated raw diagnostics to %q", got)
	}

	m.inspectionsSettings.globalDraft.Default["lsp:javac:unused"] =
		inspectionOverrideSettings{Severity: "error"}
	m.syncAnalysisInspectionResults()
	problems := m.analysisInspectionProblems()
	if len(problems) != 2 {
		t.Fatalf("severity projection problems = %d, want 2", len(problems))
	}
	var projected analysisInspectionProblem
	for _, problem := range problems {
		if problem.Kind == "javac(unused)" {
			projected = problem
			break
		}
	}
	if projected.Kind == "" || analysisSeverity(projected.Diag) != "error" {
		t.Fatalf("projected problem = %+v", problems)
	}
}

func TestAnalysisInspectionGroupingAndWorstSeverity(t *testing.T) {
	source := "javac"
	problems := []analysisInspectionProblem{
		{
			File: analysisInspectedFile{Path: "/p/A.java", RelPath: "A.java"},
			Diag: openidea.Diagnostic{
				Range: analysisRangeForTest(4, 0, 4, 1), Severity: "warning",
				Message: "warning", Source: &source,
			},
			Kind: "javac",
		},
		{
			File: analysisInspectedFile{Path: "/p/A.java", RelPath: "A.java"},
			Diag: openidea.Diagnostic{
				Range: analysisRangeForTest(2, 0, 2, 1), Severity: "error",
				Message: "error", Source: &source,
			},
			Kind: "javac",
		},
	}
	merged := analysisInspectionNodes(problems, false)
	if len(merged) != 1 || merged[0].Kind != "inspection" ||
		merged[0].Severity != "error" || merged[0].Count != 2 {
		t.Fatalf("merged grouping = %+v", merged)
	}
	if len(merged[0].Children) != 1 || len(merged[0].Children[0].Children) != 2 {
		t.Fatalf("merged children = %+v", merged[0].Children)
	}
	bySeverity := analysisInspectionNodes(problems, true)
	if len(bySeverity) != 2 {
		t.Fatalf("severity groups = %d, want 2", len(bySeverity))
	}
	if bySeverity[0].Severity != "error" || bySeverity[1].Severity != "warning" {
		t.Fatalf("severity order = %+v", bySeverity)
	}
}

func TestAnalysisInspectionNavigationSkipsExcludedProblems(t *testing.T) {
	m := gotoLineTestModel("line one\nline two\nline three", 0)
	m.setRoot(t.TempDir())
	path := m.tab().doc.Path()
	source := "javac"
	state := &m.analysis.inspection
	first := analysisInspectionProblem{
		File: analysisInspectedFile{Path: path, RelPath: filepath.Base(path)},
		Diag: openidea.Diagnostic{
			Range: analysisRangeForTest(1, 0, 1, 4), Severity: "warning",
			Message: "first", Source: &source,
		},
		Kind: "javac",
	}
	second := analysisInspectionProblem{
		File: analysisInspectedFile{Path: path, RelPath: filepath.Base(path)},
		Diag: openidea.Diagnostic{
			Range: analysisRangeForTest(2, 0, 2, 4), Severity: "error",
			Message: "second", Source: &source,
		},
		Kind: "javac",
	}
	state.files = map[string]analysisInspectedFile{
		path: {
			Path: path, RelPath: filepath.Base(path),
			Diagnostics: []openidea.Diagnostic{first.Diag, second.Diag},
		},
	}
	state.excluded = map[string]bool{
		analysisInspectionProblemID(first): true,
	}
	m.StepAnalysisInspection(1)
	if state.selected != analysisInspectionProblemID(second) {
		t.Fatalf("selected = %q, want second problem", state.selected)
	}
	if line := m.tab().state.CursorPos().Line; line != 2 {
		t.Fatalf("navigation line = %d, want 2", line)
	}

	state.autoscroll = true
	state.selected = analysisInspectionProblemID(second)
	m.selectAnalysisInspectionProblem(first, false)
	if got := m.tab().state.CursorPos().Line; got != 1 {
		t.Fatalf("autoscroll preview line = %d, want 1", got)
	}
}

func TestAnalysisScopePayload(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	tests := []struct {
		name      string
		kind      string
		directory string
		pattern   string
		want      map[string]any
	}{
		{name: "project", kind: analysisScopeProject, want: map[string]any{"kind": "project"}},
		{
			name: "directory", kind: analysisScopeDirectory, directory: "/project/src",
			want: map[string]any{"kind": "directory", "path": "/project/src"},
		},
		{
			name: "custom", kind: analysisScopeCustom, pattern: "src/**/*.java",
			want: map[string]any{"kind": "pattern", "pattern": "src/**/*.java"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.analysisScopePayload(tt.kind, tt.directory, tt.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if !mapsEqual(got, tt.want) {
				t.Fatalf("payload = %#v, want %#v", got, tt.want)
			}
		})
	}
	if _, err := m.analysisScopePayload(analysisScopeFile, "", ""); err == nil {
		t.Fatal("current-file scope succeeded without an open file")
	}
}

func mapsEqual(a, b map[string]any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func TestAnalysisInspectionEventsFilteringAndCSV(t *testing.T) {
	m := &Model{analysis: analysisState{inspection: analysisInspectionState{
		request: 7,
		files:   map[string]analysisInspectedFile{},
	}}}
	m.acceptAnalysisInspectEvent(7, json.RawMessage(`{
		"type":"started","total":2
	}`))
	m.acceptAnalysisInspectEvent(7, json.RawMessage(`{
		"type":"file","done":1,"file":{
			"path":"/project/App.java","relPath":"App.java","diagnostics":[
				{"range":{"start":{"line":1,"col":2},"end":{"line":1,"col":6}},
				 "severity":"warning","message":"Unused value","source":"javac","code":"unused"},
				{"range":{"start":{"line":3,"col":0},"end":{"line":3,"col":1}},
				 "severity":"error","message":"Missing semicolon","source":"javac"}
			]
		}
	}`))
	m.acceptAnalysisInspectEvent(7, json.RawMessage(`{
		"type":"done","summary":{"files":1,"problems":2,"skipped":1,"failed":0,"cancelled":false,"elapsedMs":42}
	}`))
	state := &m.analysis.inspection
	if state.running || state.total != 2 || state.done != 2 || state.summary == nil {
		t.Fatalf("event state = %+v", state)
	}
	if got := len(m.analysisInspectionProblems()); got != 2 {
		t.Fatalf("problem count = %d, want 2", got)
	}
	state.filter = "missing"
	problems := m.analysisInspectionProblems()
	if len(problems) != 1 || problems[0].Kind != "javac" || analysisSeverity(problems[0].Diag) != "error" {
		t.Fatalf("filtered problems = %+v", problems)
	}
	text, err := analysisInspectionCSV(problems)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Severity,Inspection,Path,Line,Column,Message") ||
		!strings.Contains(text, "/project/App.java,4,1,Missing semicolon") {
		t.Fatalf("CSV = %q", text)
	}
}

func TestAnalysisStructuralEditHonorsSelectionAndPreconditions(t *testing.T) {
	replacement := "newValue()"
	m := &Model{analysis: analysisState{structural: analysisStructuralState{
		files: map[string]analysisStructuralFile{
			"/project/A.java": {
				Path: "/project/A.java", RelPath: "A.java",
				Matches: []analysisStructuralMatch{
					{Range: analysisRangeForTest(1, 2, 1, 8), Replacement: &replacement},
					{Range: analysisRangeForTest(4, 2, 4, 8), Replacement: &replacement},
				},
			},
		},
		included: map[string]bool{analysisMatchKey("/project/A.java", 1): false},
		prepared: &analysisWorkspaceEdit{Preconditions: []analysisWorkspacePrecondition{{
			Path: "/project/A.java", Hash: "sha256:old",
		}}},
	}}}
	edit := m.analysisStructuralEdit()
	if len(edit.Files) != 1 || len(edit.Files[0].Edits) != 1 {
		t.Fatalf("edit = %+v", edit)
	}
	if edit.Files[0].Edits[0].NewText != replacement {
		t.Fatalf("new text = %q", edit.Files[0].Edits[0].NewText)
	}
	if len(edit.Preconditions) != 1 || edit.Preconditions[0].Hash != "sha256:old" {
		t.Fatalf("preconditions = %+v", edit.Preconditions)
	}
}

func analysisRangeForTest(line, col, endLine, endCol uint32) openidea.OutlineRange {
	return openidea.OutlineRange{
		Start: openidea.OutlinePos{Line: line, Col: col},
		End:   openidea.OutlinePos{Line: endLine, Col: endCol},
	}
}

func TestAnalysisCleanupFiltersToLanguageServerExtensions(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`[
			{"id":"java","extensions":[".java"],"filenames":[],"languageServers":[{"name":"jdtls"}]},
			{"id":"text","extensions":[".txt"],"filenames":["README"],"languageServers":[]}
		]`), nil
	}
	got, err := m.analysisCleanupFilterServerFiles(context.Background(), []string{
		"/project/App.java", "/project/notes.txt", "/project/README", "/project/App.java",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/project/App.java" {
		t.Fatalf("cleanup files = %v", got)
	}
}

func TestAnalysisDuplicateRPCPayload(t *testing.T) {
	ctx := context.Background()
	got := make(chan map[string]any, 1)
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "analysis_locate_duplicates" {
			got <- args.(map[string]any)
			return json.RawMessage(`{"groups":[],"files":2,"tokens":80,"truncated":false,"cancelled":false,"elapsedMs":9}`), nil
		}
		return nil, nil
	}
	m.analysis.duplicates.minText = "50"
	m.analysis.duplicates.anonymizeIdentifiers = true
	m.analysis.duplicates.anonymizeLiterals = false
	m.RunAnalysisDuplicates()
	args := <-got
	if args["scope"].(map[string]any)["kind"] != "project" {
		t.Fatalf("scope = %#v", args["scope"])
	}
	options := args["options"].(analysisDuplicateOptions)
	if options.MinTokens != 50 || !options.AnonymizeIdentifiers || options.AnonymizeLiterals {
		t.Fatalf("options = %+v", options)
	}
}

func TestAnalysisStructuralValidatesThenStreamsWithReplaceQuery(t *testing.T) {
	ctx := context.Background()
	type call struct {
		command string
		args    map[string]any
	}
	calls := make(chan call, 2)
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		calls <- call{command: command, args: args.(map[string]any)}
		return nil, nil
	}
	state := &m.analysis.structural
	state.pattern = "$a$.equals($b$)"
	state.replacement = "Objects.equals($a$, $b$)"
	state.language = "java"
	state.replace = true
	state.scope.kind = analysisScopeProject
	m.RunAnalysisStructural()

	first := <-calls
	second := <-calls
	if first.command != "structural_validate" || second.command != "structural_search" {
		t.Fatalf("RPC order = %s, %s", first.command, second.command)
	}
	query := second.args["query"].(map[string]any)
	if query["pattern"] != state.pattern || query["language"] != "java" {
		t.Fatalf("search query = %#v", query)
	}
	if replacement, ok := query["replacement"].(*string); !ok || replacement == nil || *replacement != state.replacement {
		t.Fatalf("replacement = %#v", query["replacement"])
	}
	if request := second.args["request"].(uint32); request == 0 {
		t.Fatal("structural search request id is zero")
	}
	if channel := second.args["onEvent"].(map[string]any)["$channel"].(uint32); channel == 0 {
		t.Fatal("structural search channel id is zero")
	}
	deadline := time.Now().Add(time.Second)
	for state.running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state.running {
		t.Fatal("structural RPC completion did not clear the running state")
	}
}

func TestAnalysisStructuralDoneEventPreparesReplacementBeforeRPCReply(t *testing.T) {
	ctx := context.Background()
	prepared := make(chan map[string]any, 1)
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { fn() },
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "prepare_workspace_edit" {
			prepared <- args.(map[string]any)
			return json.RawMessage(`{"files":[],"preconditions":[]}`), nil
		}
		return nil, nil
	}
	state := &m.analysis.structural
	state.replace = true
	state.request = 7
	state.files = map[string]analysisStructuralFile{
		"/project/App.java": {
			Path:    "/project/App.java",
			RelPath: "App.java",
			Matches: []analysisStructuralMatch{{
				Range:       openidea.OutlineRange{},
				Replacement: stringPointer("replacement"),
			}},
		},
	}

	m.acceptAnalysisStructuralEvent(7, json.RawMessage(
		`{"type":"done","summary":{"files":1,"matches":1,"cancelled":false}}`,
	))

	select {
	case args := <-prepared:
		if edit := args["edit"].(analysisWorkspaceEdit); len(edit.Files) != 1 {
			t.Fatalf("prepared edit = %+v", edit)
		}
	case <-time.After(time.Second):
		t.Fatal("done event did not prepare the structural replacement")
	}
}

func TestAnalysisInspectionPointerFiltersNavigateAndCancelOffscreen(t *testing.T) {
	m := gotoLineTestModel("line one\nline two\nline three", 0)
	m.setRoot(t.TempDir())
	m.ctx = context.Background()
	docPath := m.tab().doc.Path()
	source := "javac"
	state := &m.analysis.inspection
	state.scope.kind = analysisScopeFile
	state.files = map[string]analysisInspectedFile{
		docPath: {
			Path: docPath, RelPath: "test.java",
			Diagnostics: []openidea.Diagnostic{
				{
					Range:    analysisRangeForTest(1, 0, 1, 4),
					Severity: "warning", Message: "Unused value", Source: &source,
				},
				{
					Range:    analysisRangeForTest(2, 0, 2, 5),
					Severity: "error", Message: "Missing semicolon", Source: &source,
				},
			},
		},
	}
	state.summary = &analysisInspectSummary{Files: 1, Problems: 2}

	tester := ui.NewTester(func(c *ui.Context) {
		m.analysisInspectionPanel(c, c.Theme())
	}, 900, 620)
	tester.Frame()
	if err := tester.Click("Warnings"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !tester.HasText("Unused value") || tester.HasText("Missing semicolon") {
		t.Fatalf("severity filter = %q", tester.Texts())
	}
	if err := tester.Click(":2"); err != nil {
		t.Fatal(err)
	}
	problems := m.analysisInspectionProblems()
	if len(problems) != 1 || state.selected != analysisInspectionProblemID(problems[0]) {
		t.Fatalf("problem selection = %q", state.selected)
	}
	if got := m.tab().state.CursorPos().Line; got != 1 {
		t.Fatalf("problem navigation line = %d, want 1", got)
	}

	cancelled := make(chan map[string]any, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "cancel_request" {
			cancelled <- args.(map[string]any)
		}
		return nil, nil
	}
	state.running = true
	state.request = 71
	tester.Frame()
	if err := tester.Click("Stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-cancelled:
		if args["id"] != uint32(71) {
			t.Fatalf("cancelled request = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("inspection Stop did not cancel the backend request")
	}
}

func TestAnalysisDuplicatesPointerNavigateCollapseAndCancelOffscreen(t *testing.T) {
	m := gotoLineTestModel("line one\nline two\nline three", 0)
	m.setRoot(t.TempDir())
	m.ctx = context.Background()
	docPath := m.tab().doc.Path()
	state := &m.analysis.duplicates
	state.scope.kind = analysisScopeFile
	state.report = &analysisDuplicatesReport{
		Files: 1, Tokens: 30,
		Groups: []analysisDuplicateGroup{{
			Tokens: 30, Lines: 2, Language: "java",
			Fragments: []analysisDuplicateFragment{
				{
					Path: docPath, RelPath: "test.java",
					Range: analysisRangeForTest(1, 0, 2, 4),
					Text:  "line two\nline three",
				},
				{
					Path: filepath.Join(m.Root(), "Other.java"), RelPath: "Other.java",
					Range: analysisRangeForTest(1, 0, 2, 4),
					Text:  "line two\nother three",
				},
			},
		}},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		m.analysisDuplicatesPanel(c, c.Theme())
	}, 900, 620)
	tester.Frame()
	if err := tester.Click("test.java"); err != nil {
		t.Fatal(err)
	}
	if state.selected != 0 || m.tab().state.CursorPos().Line != 2 {
		t.Fatalf("fragment pointer navigation = selected:%d cursor:%+v",
			state.selected, m.tab().state.CursorPos())
	}
	tester.Frame()
	if !tester.HasText("other three") {
		t.Fatalf("side-by-side fragments missing: %q", tester.Texts())
	}
	if err := tester.Click("java"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !state.collapsed[0] {
		t.Fatalf("group collapse = collapsed:%v texts:%q", state.collapsed, tester.Texts())
	}

	cancelled := make(chan map[string]any, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "cancel_request" {
			cancelled <- args.(map[string]any)
		}
		return nil, nil
	}
	state.running = true
	state.request = 72
	tester.Frame()
	if err := tester.Click("Stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-cancelled:
		if args["id"] != uint32(72) {
			t.Fatalf("cancelled request = %#v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("duplicate Stop did not cancel the backend request")
	}
}

func TestAnalysisFormsPersistAndDefaultsMatchOpenIDEA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := Preferences{Analysis: AnalysisPrefs{
		Inspection: InspectionViewPrefs{GroupBySeverity: true, Autoscroll: true},
		Duplicates: DuplicateSearchPrefs{
			MinTokens: 75, NoAnonymizeIdentifiers: true,
		},
		Structural: StructuralFormPrefs{
			Pattern: "$a$.equals($b$)", Replacement: "Objects.equals($a$, $b$)",
			Language: "java",
		},
		Cleanup: CodeCleanupPrefs{
			Configured: true, NoFixAll: true, NoReformat: true,
		},
	}}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got := LoadPreferences(path)
	if got.Analysis != want.Analysis {
		t.Fatalf("analysis preferences = %+v, want %+v", got.Analysis, want.Analysis)
	}
	defaults := (Preferences{}).Analysis
	if defaults.Duplicates.minTokens() != 60 ||
		!defaults.Duplicates.anonymizeIdentifiers() ||
		!defaults.Duplicates.anonymizeLiterals() {
		t.Fatalf("duplicate defaults = %+v", defaults.Duplicates)
	}
	options := defaults.Cleanup.options()
	if !options.fixAll || !options.optimizeImports || !options.reformat {
		t.Fatalf("cleanup defaults = %+v", options)
	}
}

func TestAnalysisProjectGenerationCancelsRequestsAndRejectsLateResults(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	cancelled := make(chan uint32, 3)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "analysis_locate_duplicates":
			started <- struct{}{}
			<-release
			return json.RawMessage(`{"groups":[{"tokens":20,"lines":2,"language":"java","fragments":[]}],"files":1,"tokens":20}`), nil
		case "cancel_request":
			request, _ := args.(map[string]any)["id"].(uint32)
			cancelled <- request
			return nil, nil
		default:
			return nil, nil
		}
	}
	m.analysis.inspection.request = 71
	m.analysis.duplicates.request = 72
	m.analysis.structural.request = 73
	m.analysis.duplicates.minText = "60"
	m.RunAnalysisDuplicates()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("duplicate search did not start")
	}
	m.resetAnalysisState()
	if m.analysis.generation == 0 {
		t.Fatal("project generation did not advance")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !m.analysis.duplicates.running && m.analysis.duplicates.report == nil {
		time.Sleep(time.Millisecond)
	}
	if m.analysis.duplicates.report != nil {
		t.Fatal("late duplicate result escaped project reset")
	}
	got := map[uint32]bool{}
	for {
		select {
		case request := <-cancelled:
			got[request] = true
		default:
			if !got[71] || !got[72] || !got[73] {
				t.Fatalf("cancelled requests = %v", got)
			}
			return
		}
	}
}

func TestAnalysisStructuralResultsTransferToFindUsages(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	state := &m.analysis.structural
	state.pattern = "  $a$.equals($b$)\n"
	state.files = map[string]analysisStructuralFile{
		"/project/B.java": {
			Path: "/project/B.java", RelPath: "B.java",
			Matches: []analysisStructuralMatch{{Range: analysisRangeForTest(4, 2, 4, 10)}},
		},
		"/project/A.java": {
			Path: "/project/A.java", RelPath: "A.java",
			Matches: []analysisStructuralMatch{
				{Range: analysisRangeForTest(1, 3, 1, 8)},
				{Range: analysisRangeForTest(2, 1, 2, 9)},
			},
		},
	}
	state.summary = &analysisStructuralSummary{Files: 2, Matches: 3}
	m.showAnalysisStructuralResults()
	if !state.resultsShown {
		t.Fatal("structural results were not marked shown")
	}
	if m.usages.activeTabIndex() < 0 || m.usages.symbol != "$a$.equals($b$)" {
		t.Fatalf("usage result = %+v", m.usages.usagesSearch)
	}
	if len(m.usages.locs) != 3 {
		t.Fatalf("usage locations = %d, want 3", len(m.usages.locs))
	}
}

func TestAnalysisStructuralApplyRejectsStalePreconditions(t *testing.T) {
	replacement := "newValue()"
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command == "apply_workspace_edit" {
			return nil, &openidea.Error{Kind: "outOfSync", Message: "files changed"}
		}
		return nil, nil
	}
	state := &m.analysis.structural
	state.replace = true
	state.files = map[string]analysisStructuralFile{
		"/project/A.java": {
			Path: "/project/A.java", RelPath: "A.java",
			Matches: []analysisStructuralMatch{{
				Range: analysisRangeForTest(1, 2, 1, 7), Replacement: &replacement,
			}},
		},
	}
	state.prepared = &analysisWorkspaceEdit{
		Preconditions: []analysisWorkspacePrecondition{{Path: "/project/A.java", Hash: "sha256:old"}},
	}
	m.ApplyAnalysisStructuralReplace()
	deadline := time.Now().Add(time.Second)
	for state.applying && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(state.err, "files changed since the preview") {
		t.Fatalf("stale apply error = %q", state.err)
	}
	if state.prepared != nil {
		t.Fatal("stale structural edit kept its preview")
	}
}

func TestAnalysisCleanupWalksBackendEntriesAndSkipsExcluded(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "fs_read_dir" {
			return nil, nil
		}
		switch args.(map[string]any)["path"] {
		case "/project":
			return json.RawMessage(`[
				{"name":"src","path":"/project/src","kind":"dir"},
				{"name":"ignored","path":"/project/ignored","kind":"dir","ignored":true},
				{"name":"excluded","path":"/project/excluded","kind":"dir","excluded":true},
				{"name":"hidden","path":"/project/hidden","kind":"dir","hidden":true},
				{"name":"link","path":"/project/link","kind":"symlink"},
				{"name":"Root.java","path":"/project/Root.java","kind":"file"}
			]`), nil
		case "/project/src":
			return json.RawMessage(`[
				{"name":"A.java","path":"/project/src/A.java","kind":"file"},
				{"name":"skip.txt","path":"/project/src/skip.txt","kind":"file"}
			]`), nil
		default:
			return json.RawMessage(`[]`), nil
		}
	}
	files, err := m.analysisWalkCleanupFiles(context.Background(), "/project", func(path string) bool {
		return strings.HasSuffix(path, ".java")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != "/project/Root.java" || files[1] != "/project/src/A.java" {
		t.Fatalf("cleanup walk = %v", files)
	}
}

func TestAnalysisCleanupConfirmsMoreThanOneHundredFiles(t *testing.T) {
	entries := make([]map[string]any, 0, analysisCleanupConfirmAbove+1)
	for i := 0; i < analysisCleanupConfirmAbove+1; i++ {
		entries = append(entries, map[string]any{
			"name": "App" + fmt.Sprint(i) + ".java",
			"path": "/project/App" + fmt.Sprint(i) + ".java",
			"kind": "file",
		})
	}
	entriesJSON, _ := json.Marshal(entries)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "languages":
			return json.RawMessage(`[
				{"id":"java","extensions":[".java"],"filenames":[],"languageServers":[{"name":"jdtls"}]}
			]`), nil
		case "fs_read_dir":
			return entriesJSON, nil
		default:
			return nil, nil
		}
	}
	state := &m.analysis.cleanup
	state.scope.kind = analysisScopeProject
	state.fixAll = true
	m.RunAnalysisCleanup()
	deadline := time.Now().Add(time.Second)
	for state.pending == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state.pending == nil || len(state.pending.files) != analysisCleanupConfirmAbove+1 {
		t.Fatalf("pending cleanup = %+v", state.pending)
	}
	if state.running || state.processed != 0 {
		t.Fatalf("cleanup started before confirmation: %+v", state)
	}
	m.CancelAnalysisCleanupConfirmation()
	if state.pending != nil {
		t.Fatal("cleanup confirmation was not cleared")
	}
}
