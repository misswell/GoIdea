package adapter

import (
	"slices"
	"testing"

	"mayoi/internal/openidea"
)

func pluginTestLocationAt(path string, line, col uint32) *openidea.Location {
	return &openidea.Location{
		Path: path,
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: line, Col: col},
			End:   openidea.OutlinePos{Line: line, Col: col + 1},
		},
	}
}

func TestPluginTestMapperKeepsStateAndMapsEveryResultShape(t *testing.T) {
	mapper := NewPluginTestMapper()
	first, err := mapper.Map([]openidea.PluginTestEvent{
		{
			Type: "suiteStarted", ID: "suite", Name: "Cart",
			Location: pluginTestLocationAt("/work/cart.test.ts", 2, 4),
		},
		{
			Type: "testStarted", ID: "pass", Name: "adds", Parent: testStringPtr("suite"),
		},
		{
			Type: "testStarted", ID: "fail-1", Name: "fails", Parent: testStringPtr("suite"),
		},
		{
			Type: "testStarted", ID: "fail-2", Name: "fails", Parent: testStringPtr("suite"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 5 {
		t.Fatalf("start events = %+v", first)
	}
	if !slices.Equal(first[0].Path, []string{"Cart"}) ||
		!slices.Equal(first[2].Path, []string{"Cart", "adds"}) ||
		!slices.Equal(first[3].Path, []string{"Cart", "fails"}) ||
		!slices.Equal(first[4].Path, []string{"Cart", "fails [fail-2]"}) {
		t.Fatalf("test paths = %+v", first)
	}
	if first[1].Kind != "location" || first[1].Location == nil ||
		first[1].Location.Line != 3 || first[1].Location.Col != 5 ||
		first[1].Location.Resolve != "path" {
		t.Fatalf("plugin location = %+v", first[1].Location)
	}

	expected, actual, reason := "4", "5", "busy"
	duration := 12.5
	rest, err := mapper.Map([]openidea.PluginTestEvent{
		{Type: "output", ID: "pass", Text: "stdout"},
		{Type: "passed", ID: "pass", DurationMs: &duration},
		{
			Type: "failed", ID: "fail-1", DurationMs: &duration,
			Message: "not equal", Expected: &expected, Actual: &actual,
			Location: pluginTestLocationAt("/work/cart.test.ts", 8, 1),
		},
		{Type: "skipped", ID: "fail-2", Reason: &reason},
		{Type: "other", ID: "ignored"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree := NewTestResults()
	tree.Apply(append(first, rest...))

	pass := tree.Node([]string{"Cart", "adds"})
	if pass.Own != TestPassed || pass.DurationMs == nil || *pass.DurationMs != duration ||
		!slices.Equal(pass.Output, []string{"stdout"}) {
		t.Fatalf("passed node = %+v", pass)
	}
	failed := tree.Node([]string{"Cart", "fails"})
	if failed.Own != TestFailed || failed.Location == nil || failed.Location.Line != 9 ||
		!slices.Contains(failed.Output, "not equal") ||
		!slices.Contains(failed.Output, "Expected: 4") ||
		!slices.Contains(failed.Output, "Actual: 5") {
		t.Fatalf("failed node = %+v", failed)
	}
	skipped := tree.Node([]string{"Cart", "fails [fail-2]"})
	if skipped.Own != TestSkipped || !slices.Equal(skipped.Output, []string{"busy"}) {
		t.Fatalf("skipped node = %+v", skipped)
	}
}

func TestPluginTestMapperRejectsInvalidTrees(t *testing.T) {
	mapper := NewPluginTestMapper()
	if _, err := mapper.Map([]openidea.PluginTestEvent{{
		Type: "testStarted", ID: "child", Name: "orphan", Parent: testStringPtr("missing"),
	}}); err == nil {
		t.Fatal("a missing parent was accepted")
	}
	if _, err := mapper.Map([]openidea.PluginTestEvent{{
		Type: "passed", ID: "missing",
	}}); err == nil {
		t.Fatal("a result for an unknown id was accepted")
	}
	if _, err := mapper.Map([]openidea.PluginTestEvent{{
		Type: "suiteStarted", ID: "empty",
	}}); err == nil {
		t.Fatal("an empty test name was accepted")
	}
	if _, err := mapper.Map([]openidea.PluginTestEvent{{
		Type: "passed", ID: "event",
	}}); err == nil {
		t.Fatal("an unknown id was accepted")
	}
}

func testStringPtr(value string) *string {
	return &value
}
