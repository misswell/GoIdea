package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func runSurfaceTestModel() *Model {
	primary := openidea.RunConfig{ID: "primary", Name: "Application", Kind: "application"}
	childConfig := openidea.RunConfig{ID: "child", Name: "Child", Kind: "shell"}
	exit := int32(0)
	return &Model{
		run: RunPane{
			visible: true, trusted: true, loaded: true,
			configs: []openidea.RunConfig{primary},
			sel:     0,
			channel: 11, runID: 7, lastName: "Application",
			exitCode: &exit, started: time.Now().Add(-2 * time.Second),
			ended: time.Now(),
			output: []outputLine{
				{kind: "stdout", text: "server ready"},
				{kind: "stderr", text: "warning: retry scheduled"},
				{kind: "stdout", text: "see https://example.test/run"},
			},
			children: []*runChild{{
				channel: 12, runID: 8, name: "Child", config: childConfig,
				finished: true, exitCode: &exit,
				started: time.Now().Add(-time.Second), ended: time.Now(),
				output: []outputLine{{kind: "stdout", text: "child finished"}},
			}},
			agentActive: 0,
			agentRuns: []*agentRunSession{{
				id: 3, request: agentRunRequest{Name: "Agent"},
				channel: 13, runID: 9, started: time.Now(),
				output: []outputLine{{kind: "stdout", text: "agent output"}},
			}},
		},
		stackTrace: &stackTraceState{
			active: -1,
			tabs:   []stackTraceTab{{title: "Boom", lines: []string{"at src/main.go:8"}}},
		},
	}
}

func TestRunWindowUnifiedTabsSessionRailAndEmptyState(t *testing.T) {
	m := runSurfaceTestModel()
	tt := ui.NewTester(func(c *ui.Context) { m.runPanel(c, c.Theme()) }, 1000, 500)
	tt.Frame()
	for _, text := range []string{
		"Application Run Tab", "Child Run Tab", "Agent Run Tab", "Boom Run Tab",
		"Run", "Options",
		"Rerun", "Debug", "Stop", "Find…", "Scroll to End", "Soft-Wrap",
		"Clear All", "Edit Configurations…", "More Run Actions", "Hide Run Tool Window",
	} {
		if !tt.HasText(text) {
			t.Fatalf("run window is missing %q; texts: %s", text, strings.Join(tt.Texts(), "\n"))
		}
	}

	m.run = RunPane{visible: true, trusted: true, loaded: true}
	m.stackTrace = nil
	tt = ui.NewTester(func(c *ui.Context) { m.runPanel(c, c.Theme()) }, 1000, 500)
	tt.Frame()
	for _, text := range []string{
		"To run your code, do one of the following:",
		"Click the Run icon in the editor gutter",
		`Select "Run…" in the editor context menu`,
		"run configuration",
	} {
		if !tt.HasText(text) {
			t.Fatalf("empty run window is missing %q; texts: %s", text, strings.Join(tt.Texts(), "\n"))
		}
	}
}

func TestRunConsoleControlsChangeNativePaneState(t *testing.T) {
	m := runSurfaceTestModel()
	m.run.agentRuns = nil
	m.run.agentActive = 0
	m.run.activeChild = 0
	tt := ui.NewTester(func(c *ui.Context) { m.runPanel(c, c.Theme()) }, 1000, 500)
	tt.Frame()

	if err := tt.Click("Find…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !m.run.consoleState(runPrimaryTabKey()).searching {
		t.Fatal("Find did not open the native console find bar")
	}
	if !tt.HasText("Find in Run Console") {
		t.Fatal("Find did not render its native query field")
	}

	if err := tt.Click("Scroll to End"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !m.run.consoleState(runPrimaryTabKey()).noFollow {
		t.Fatal("Scroll to End did not turn following off")
	}

	if err := tt.Click("Soft-Wrap"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !m.run.consoleState(runPrimaryTabKey()).noSoftWrap {
		t.Fatal("Soft-Wrap did not turn wrapping off")
	}

	if err := tt.Click("Clear All"); err != nil {
		t.Fatal(err)
	}
	if len(m.run.output) != 0 {
		t.Fatal("Clear All did not empty the active console")
	}
}

func TestRunFinishedTabTooltipClosingLastTabAndFailureState(t *testing.T) {
	m := runSurfaceTestModel()
	m.run.agentRuns = nil
	m.run.agentActive = 0
	m.run.children = nil
	m.stackTrace = nil
	tabs := m.runTabs()
	if len(tabs) != 1 {
		t.Fatalf("tabs = %+v", tabs)
	}
	if got := tabs[0].tooltip; got == "" || !strings.Contains(got, "exit code 0") ||
		!strings.Contains(got, "Finished in") {
		t.Fatalf("finished tooltip = %q", got)
	}

	code := int32(2)
	m.run.exitCode = &code
	tabs = m.runTabs()
	if !tabs[0].failed || tabs[0].icon != "run/testFailed" {
		t.Fatalf("failed tab = %+v", tabs[0])
	}

	m.run.stopped = false
	m.run.exitCode = nil
	m.run.output = []outputLine{{kind: "stdout", text: "done"}}
	tt := ui.NewTester(func(c *ui.Context) { m.runPanel(c, c.Theme()) }, 900, 400)
	tt.Frame()
	if err := tt.Click("Close Application Run Tab"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.run.hasPrimarySession() {
		t.Fatal("closing the last run tab did not remove the session")
	}
	if m.run.visible {
		t.Fatal("closing the last run tab did not hide the Run tool window")
	}
}

func TestRunConsoleStdinAndEofUseNativeRunRPCs(t *testing.T) {
	m := runSurfaceTestModel()
	m.run.agentRuns = nil
	m.run.agentActive = 0
	m.run.children = nil
	m.stackTrace = nil
	m.run.running = true
	m.run.stdinClosed = false
	m.run.exitCode = nil
	m.postHook = func(fn func()) { fn() }
	calls := make(chan struct {
		command string
		args    map[string]any
	}, 2)
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		args, _ := input.(map[string]any)
		calls <- struct {
			command string
			args    map[string]any
		}{command: command, args: args}
		return nil, nil
	}

	session := m.primaryRunSession()
	m.sendRunConsoleInput(session, "Ada\n")
	first := <-calls
	if first.command != "run_stdin" || first.args["id"] != uint32(7) ||
		first.args["data"] != "Ada\n" {
		t.Fatalf("stdin call = %#v", first)
	}
	if len(m.run.output) != 4 || m.run.output[3].kind != "input" ||
		m.run.output[3].text != "Ada" {
		t.Fatalf("stdin echo = %+v", m.run.output)
	}

	session = m.primaryRunSession()
	m.closeRunConsoleInput(session)
	second := <-calls
	if second.command != "run_close_stdin" || second.args["id"] != uint32(7) ||
		!m.run.stdinClosed {
		t.Fatalf("close stdin call = %#v, closed=%v", second, m.run.stdinClosed)
	}
}

func TestRunTestResultsUseTheSessionSplitAndSelectedOutput(t *testing.T) {
	m := runSurfaceTestModel()
	m.run.agentRuns = nil
	m.run.agentActive = 0
	m.run.children = nil
	m.stackTrace = nil
	m.run.exitCode = nil
	m.run.running = false
	m.run.tests = adapter.NewTestResults()
	m.run.tests.Apply([]adapter.TestEvent{
		{Kind: "start", Path: []string{"Suite", "Broken"}},
		{Kind: "result", Path: []string{"Suite", "Broken"}, Status: adapter.TestFailed},
		{Kind: "output", Path: []string{"Suite", "Broken"}, Line: "expected 3 but was 4"},
		{Kind: "result", Path: []string{"Suite", "Passing"}, Status: adapter.TestPassed},
	})
	tt := ui.NewTester(func(c *ui.Context) { m.runPanel(c, c.Theme()) }, 1000, 500)
	tt.Frame()
	if !tt.HasText("1 failing") || !tt.HasText("1 passed") {
		t.Fatalf("test header counts are missing: %s", strings.Join(tt.Texts(), "\n"))
	}
	if _, ok := tt.Find("✗     Broken"); !ok {
		t.Fatalf("failed test row is missing: %s", strings.Join(tt.Texts(), "\n"))
	}
	if err := tt.Click("✗     Broken"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Show all output") || !tt.HasText("expected 3 but was 4") {
		t.Fatalf("selected-test output is missing: %s", strings.Join(tt.Texts(), "\n"))
	}
	if err := tt.Click("Show all output"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.run.selTest != "" {
		t.Fatalf("Show all output left selection %q", m.run.selTest)
	}
}

func TestServiceSessionShownInRunUsesTheSameNativeSessionSurface(t *testing.T) {
	m := runSurfaceTestModel()
	service := &runChild{
		channel: 21, runID: 31, name: "Gateway", service: true, running: true,
		config:  openidea.RunConfig{ID: "gateway", Name: "Gateway", Kind: "spring-boot"},
		started: time.Now(),
		output:  []outputLine{{kind: "stdout", text: "Gateway listening"}},
	}
	m.run.children = nil
	m.run.agentRuns = nil
	m.run.agentActive = 0
	m.run.viewChild = service
	m.services.children = []*runChild{service}
	m.stackTrace = nil
	tt := ui.NewTester(func(c *ui.Context) { m.runPanel(c, c.Theme()) }, 1000, 500)
	tt.Frame()
	for _, text := range []string{
		"Gateway Run Tab", "Show in Services", "Gateway listening",
		"Rerun", "Stop", "Find…", "Soft-Wrap", "Clear All", "Edit Configurations…",
	} {
		if !tt.HasText(text) {
			t.Fatalf("service Run view is missing %q: %s", text, strings.Join(tt.Texts(), "\n"))
		}
	}
	if err := tt.Click("Show in Services"); err != nil {
		t.Fatal(err)
	}
	if m.run.viewChild != nil {
		t.Fatal("Show in Services did not release the temporary Run view")
	}
}

func TestRunTabsUseOpenIDEARemoteTargetLabels(t *testing.T) {
	m := runSurfaceTestModel()
	target := "remote-1"
	m.run.configs[0].Target = &target
	m.remote.runTargets.settings.Targets = []openidea.RunTarget{{
		Kind: openidea.RunTargetSSH, ID: target, Name: "Staging",
	}}
	tabs := m.runTabs()
	if len(tabs) == 0 || tabs[0].label != "Application @ Staging" {
		t.Fatalf("remote run tab label = %+v", tabs)
	}
}
