package adapter

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

// jestOutput is a captured `jest --verbose` transcript of one failing suite.
const jestOutput = `FAIL ./calc.test.js
  calc
    ✓ adds ok (1 ms)
    ✓ adds negatives (1 ms)
    ✕ fails on purpose

  ● calc › fails on purpose

    expect(received).toBe(expected) // Object.is equality

    Expected: 3
    Received: 2

    > 19 |     expect(add(1, 1)).toBe(3);

      at Object.toBe (calc.test.js:19:23)

Test Suites: 1 failed, 1 total
Tests:       1 failed, 2 passed, 3 total
Snapshots:   0 total
Time:        0.121 s, estimated 1 s
`

// jestTree folds the transcript into the tree the way the run pane does.
func jestTree(t *testing.T, text string) *TestResults {
	t.Helper()
	parser := NewJsParser()
	tree := NewTestResults()
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		tree.Apply(parser.Feed(line))
	}
	return tree
}

func TestJestParserBuildsOneNodePerTest(t *testing.T) {
	tree := jestTree(t, jestOutput)
	counts := tree.Counts()
	if counts.Passed != 2 || counts.Failed != 1 || counts.Total != 3 {
		t.Fatalf("counts = %+v, want 2 passed / 1 failed / 3 total", counts)
	}
	// The multi-byte marks (`✓` `✕`) must be decoded as characters: a
	// byte-wise read makes them describe headers and loses the results.
	if len(tree.Roots) != 1 || tree.Roots[0].Name != "./calc.test.js" {
		t.Fatalf("roots = %v, want the test file", tree.Roots)
	}
	suite := tree.Roots[0]
	if len(suite.Children) != 1 || suite.Children[0].Name != "calc" {
		t.Fatalf("suite children = %v, want the describe block", suite.Children)
	}
	var names []string
	for _, n := range suite.Children[0].Children {
		names = append(names, n.Name)
	}
	if strings.Join(names, ",") != "adds ok,adds negatives,fails on purpose" {
		t.Fatalf("tests = %v, want the three test names", names)
	}
	// The fail header lands on the verbose line's node instead of a nested
	// duplicate.
	paths := FailedTestPaths(tree)
	if len(paths) != 1 || strings.Join(paths[0], "/") != "./calc.test.js/calc/fails on purpose" {
		t.Fatalf("failed paths = %v, want one path under the describe", paths)
	}
}

func TestJestFailureDetailsAndLocation(t *testing.T) {
	tree := jestTree(t, jestOutput)
	node := tree.Roots[0].Children[0].Children[2]
	if node.Own != TestFailed {
		t.Fatalf("failed test status = %q", node.Own)
	}
	if len(node.Output) == 0 {
		t.Fatalf("the failure details must attach to the test node")
	}
	if node.Location == nil || node.Location.Path != "calc.test.js" || node.Location.Line != 19 || node.Location.Col != 23 {
		t.Fatalf("location = %+v, want calc.test.js:19:23", node.Location)
	}
}

func TestJestSummaryCounts(t *testing.T) {
	tree := jestTree(t, jestOutput)
	if tree.Summary.Passed == nil || *tree.Summary.Passed != 2 {
		t.Fatalf("summary passed = %v, want 2 from the Tests: line", tree.Summary.Passed)
	}
	if tree.Summary.Failed == nil || *tree.Summary.Failed != 1 {
		t.Fatalf("summary failed = %v, want 1 from the Tests: line", tree.Summary.Failed)
	}
}

func TestJsArgsAnchorsTheTestNames(t *testing.T) {
	cfg := &openidea.RunConfig{ID: "npm:test", Name: "test", Kind: "npm", Command: "npm",
		Args: []string{"run", "test"}}
	args := JsArgs(cfg, [][]string{{"./calc.test.js", "calc", "fails on purpose"}})
	// escapeRegExp escapes regex metacharacters only: spaces stay as typed.
	want := []string{"run", "test", "--", "./calc.test.js", "-t",
		`^(?:^calc fails on purpose$)$`}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("JsArgs = %v, want %v", args, want)
	}
}

func TestJsArgsDropsTheRunsOwnNameFilter(t *testing.T) {
	// After `--`, the run's own -t pattern is dropped and the derived one
	// appended (js.ts drops -t/--testNamePattern on both sides of `--`).
	cfg := &openidea.RunConfig{ID: "npm:test", Name: "test", Kind: "npm", Command: "npm",
		Args: []string{"run", "test", "--", "-t", "old pattern"}}
	args := JsArgs(cfg, [][]string{{"./calc.test.js", "calc", "fails on purpose"}})
	want := []string{"run", "test", "--", "./calc.test.js", "-t", "^(?:^calc fails on purpose$)$"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("JsArgs = %v, want %v", args, want)
	}
}
