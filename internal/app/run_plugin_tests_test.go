package app

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func waitPluginTestOutput(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
	}
}

func TestPluginTestOutputUsesBackendEventsAndFinishesAfterDrain(t *testing.T) {
	m := &Model{
		ctx:      context.Background(),
		root:     "/project",
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, method string, args any) (json.RawMessage, error) {
			if method != "plugin_parse_test_results" {
				t.Fatalf("unexpected command %s", method)
			}
			payload := args.(map[string]any)
			if payload["config"] != "plugin:example:tests" || payload["chunk"] != "plugin output\n" {
				t.Fatalf("parse payload = %#v", payload)
			}
			return json.RawMessage(`[
				{"type":"suiteStarted","id":"suite","name":"Cart"},
				{"type":"testStarted","id":"pass","parent":"suite","name":"adds"},
				{"type":"passed","id":"pass","durationMs":4}
			]`), nil
		},
	}
	m.run.channel = 7
	tree := adapter.NewTestResults()
	output := newRunPluginTestOutput(m, "plugin:example:tests", tree)
	t.Cleanup(output.stop)
	m.run.tests = tree
	m.run.testPlugin = output

	output.Feed("plugin output")
	finished := make(chan struct{})
	output.End(func() {
		tree.Finish(0)
		close(finished)
	})
	waitPluginTestOutput(t, finished, "plugin test completion")
	node := tree.Node([]string{"Cart", "adds"})
	if node.Own != adapter.TestPassed || node.DurationMs == nil || *node.DurationMs != 4 ||
		!tree.Finished {
		t.Fatalf("plugin test result = %+v, finished=%v", node, tree.Finished)
	}
}

func TestPluginTestOutputFallsBackWhenProviderHasNoParser(t *testing.T) {
	m := &Model{
		ctx:      context.Background(),
		root:     "/project",
		postHook: func(fn func()) { fn() },
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage("null"), nil
		},
	}
	m.run.channel = 8
	tree := adapter.NewTestResults()
	output := newRunPluginTestOutput(m, "plugin:fallback:tests", tree)
	t.Cleanup(output.stop)
	m.run.tests = tree
	m.run.testPlugin = output

	output.Feed("tests/test_math.py::test_add PASSED [100%]")
	finished := make(chan struct{})
	output.End(func() {
		tree.Finish(0)
		close(finished)
	})
	waitPluginTestOutput(t, finished, "plugin test fallback")
	node := tree.Node([]string{"tests/test_math.py", "test_add"})
	if node == nil || node.Own != adapter.TestPassed || !tree.Finished {
		t.Fatalf("fallback result = %+v, finished=%v", node, tree.Finished)
	}
}

func TestPluginTestOutputFallsBackWhenProviderReturnsInvalidEvents(t *testing.T) {
	m := &Model{
		ctx:      context.Background(),
		root:     "/project",
		postHook: func(fn func()) { fn() },
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`[{"type":"passed","id":"unknown"}]`), nil
		},
	}
	m.run.channel = 9
	tree := adapter.NewTestResults()
	output := newRunPluginTestOutput(m, "plugin:fallback:tests", tree)
	t.Cleanup(output.stop)
	m.run.tests = tree
	m.run.testPlugin = output

	output.Feed("tests/test_math.py::test_add PASSED [100%]")
	finished := make(chan struct{})
	output.End(func() {
		tree.Finish(0)
		close(finished)
	})
	waitPluginTestOutput(t, finished, "invalid plugin event fallback")
	node := tree.Node([]string{"tests/test_math.py", "test_add"})
	if node == nil || node.Own != adapter.TestPassed || !tree.Finished {
		t.Fatalf("fallback result = %+v, finished=%v", node, tree.Finished)
	}
}

func TestPluginTestOutputStopsWhenRunStateIsReset(t *testing.T) {
	m := &Model{
		ctx:      context.Background(),
		root:     "/project",
		postHook: func(fn func()) { fn() },
	}
	m.run.channel = 10
	tree := adapter.NewTestResults()
	output := newRunPluginTestOutput(m, "plugin:reset:tests", tree)
	m.run.tests = tree
	m.run.testPlugin = output

	m.resetRun()
	waitPluginTestOutput(t, output.ctx.Done(), "plugin parser cancellation")
	waitPluginTestOutput(t, output.done, "plugin parser exit")
}

func TestPluginTestOutputCapsUnparsedInput(t *testing.T) {
	m := &Model{
		ctx:      context.Background(),
		root:     "/project",
		postHook: func(fn func()) { fn() },
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("oversized input reached the backend")
			return nil, nil
		},
	}
	tree := adapter.NewTestResults()
	output := newRunPluginTestOutput(m, "plugin:limit:tests", tree)
	t.Cleanup(output.stop)
	m.run.tests = tree
	m.run.testPlugin = output

	output.Feed(strings.Repeat("x", pluginTestPendingMax+1))
	output.mu.Lock()
	failed := output.failed
	pending := output.pendingChars
	output.mu.Unlock()
	if !failed || pending != 0 {
		t.Fatalf("oversized parser state = failed %v pending %d", failed, pending)
	}
}

func TestUnknownTestConfigurationUsesCombinedParser(t *testing.T) {
	m := &Model{ctx: context.Background(), root: "/project"}
	m.configureRunTests(&openidea.RunConfig{
		Kind:    "shell",
		Command: "make",
		Args:    []string{"test"},
		IsTest:  true,
	}, nil)
	if m.run.tests == nil || m.run.testAuto == nil || m.run.testStdout == nil ||
		m.run.testGo != nil || m.run.testPlugin != nil {
		t.Fatalf("unknown test parser state = %+v", m.run)
	}
	m.foldRunTestLines(
		[]string{"tests/test_math.py::test_add PASSED [100%]"},
		"",
	)
	node := m.run.tests.Node([]string{"tests/test_math.py", "test_add"})
	if node == nil || node.Own != adapter.TestPassed {
		t.Fatalf("combined parser result = %+v", node)
	}

	m.configureRunTests(&openidea.RunConfig{
		Kind:    "go",
		Command: "go",
		Args:    []string{"run", "."},
		IsTest:  false,
	}, nil)
	if m.run.tests != nil || m.run.testStdout != nil || m.run.testAuto != nil {
		t.Fatalf("non-test run kept test parsers: %+v", m.run)
	}
}

func TestPluginFailedSelectionUsesConfigAndDropsAfterProjectSwitch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type call struct {
		method string
		args   map[string]any
	}
	calls := make(chan call, 1)
	hooked := make(chan struct{})
	m := &Model{
		ctx:      ctx,
		root:     "/project",
		postHook: func(fn func()) { fn() },
		invokeHook: func(ctx context.Context, method string, args any) (json.RawMessage, error) {
			calls <- call{method: method, args: args.(map[string]any)}
			<-ctx.Done()
			close(hooked)
			return nil, ctx.Err()
		},
	}
	m.run.lastID = "plugin:example:tests"
	base := &openidea.RunConfig{
		ID: "plugin:example:tests", Name: "Plugin test", Kind: "plugin",
		Command: "runner", Args: []string{"test", "--old-filter"},
		Params: map[string]any{"framework": "plugin"},
	}
	paths := [][]string{{"suite", "fails"}}
	m.selectPluginFailedTests(base, base.ID, paths)

	select {
	case got := <-calls:
		if got.method != "plugin_select_tests" ||
			got.args["config"] != base.ID ||
			!reflect.DeepEqual(got.args["tests"], paths) {
			t.Fatalf("selection call = %#v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("plugin_select_tests was not called")
	}
	m.root = "/another-project"
	cancel()
	waitPluginTestOutput(t, hooked, "plugin selection cancellation")

	derived := adapter.RerunConfigWithArgs(base, []string{"test", "--only", "suite::fails"})
	if derived == nil || derived.Name != "Failed: Plugin test" ||
		!slices.Equal(derived.Args, []string{"test", "--only", "suite::fails"}) ||
		derived.Env == nil {
		t.Fatalf("derived plugin option = %+v", derived)
	}
}
