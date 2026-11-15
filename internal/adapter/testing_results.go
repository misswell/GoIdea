package adapter

// The port of OpenIDEA's test-results pipeline (src/features/run/testing):
// results.ts's tree (a node's aggregated status, counts, first failure) and
// types.ts's events. The parser feeds events; the run pane folds them into
// the tree of the pane's last run. JVM is the framework GoIDEA parses today
// (testing_jvm.go); the rerun derivation (testing_rerun.go) uses the tree.

import (
	"fmt"
	"regexp"
	"strings"
)

// TestStatus is a test's lifecycle state (types.ts).
type TestStatus string

const (
	TestRunning TestStatus = "running"
	TestPassed  TestStatus = "passed"
	TestFailed  TestStatus = "failed"
	TestSkipped TestStatus = "skipped"
)

// TestLocation is a test's source position (types.ts); Line is 1-based.
type TestLocation struct {
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Col       int    `json:"col,omitempty"`
	Resolve   string `json:"resolve"` // "path" | "name" | "rubyTest"
	ClassName string `json:"className,omitempty"`
}

// TestEvent is one parsed output event (types.ts).
type TestEvent struct {
	Kind       string        `json:"kind"` // start | result | location | output | summary
	Path       []string      `json:"path,omitempty"`
	Status     TestStatus    `json:"status,omitempty"`
	DurationMs *float64      `json:"durationMs,omitempty"`
	Location   *TestLocation `json:"location,omitempty"`
	Line       string        `json:"line,omitempty"`
	Passed     *int          `json:"passed,omitempty"`
	Failed     *int          `json:"failed,omitempty"`
	Skipped    *int          `json:"skipped,omitempty"`
}

// TestNode is one node of the results tree (results.ts).
type TestNode struct {
	ID         string
	Name       string
	Path       []string
	Children   []*TestNode
	Own        TestStatus // "" = unset
	DurationMs *float64
	Location   *TestLocation
	Output     []string
}

// maxTestOutputLines bounds the output kept per node.
const maxTestOutputLines = 400

// testNodeStatus aggregates a node's status over its subtree (results.ts
// nodeStatus).
func testNodeStatus(node *TestNode) TestStatus {
	if len(node.Children) == 0 {
		return node.Own
	}
	any, running, failed, passed := false, false, false, false
	for _, c := range node.Children {
		s := testNodeStatus(c)
		if s == "" {
			continue
		}
		any = true
		switch s {
		case TestFailed:
			failed = true
		case TestRunning:
			running = true
		case TestPassed:
			passed = true
		}
	}
	if failed || node.Own == TestFailed {
		return TestFailed
	}
	if running {
		return TestRunning
	}
	if !any {
		return node.Own
	}
	if passed {
		return TestPassed
	}
	return TestSkipped
}

// TestCounts is a run's tallies (results.ts counts()).
type TestCounts struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Running int `json:"running"`
	Total   int `json:"total"`
}

// TestSummary is what summary events merged into (results.ts summary).
type TestSummary struct {
	Passed  *int
	Failed  *int
	Skipped *int
}

// TestResults folds parser events into the tree (results.ts TestResults).
type TestResults struct {
	Roots    []*TestNode
	byID     map[string]*TestNode
	Summary  TestSummary
	Version  int
	Finished bool
}

const testResultsSep = "\x00"

// Node returns (creating) the node at path.
func (r *TestResults) Node(path []string) *TestNode {
	id := strings.Join(path, testResultsSep)
	if node, ok := r.byID[id]; ok {
		return node
	}
	node := &TestNode{ID: id, Name: "", Path: append([]string(nil), path...)}
	if len(path) > 0 {
		node.Name = path[len(path)-1]
	}
	if r.byID == nil {
		r.byID = map[string]*TestNode{}
	}
	r.byID[id] = node
	if len(path) <= 1 {
		r.Roots = append(r.Roots, node)
	} else {
		parent := r.Node(path[:len(path)-1])
		parent.Children = append(parent.Children, node)
	}
	return node
}

// HasResult reports whether the test at path finished with a result.
func (r *TestResults) HasResult(path []string) bool {
	node, ok := r.byID[strings.Join(path, testResultsSep)]
	if !ok {
		return false
	}
	return node.Own != "" && node.Own != TestRunning
}

// Apply folds events into the tree (results.ts apply).
func (r *TestResults) Apply(events []TestEvent) {
	if len(events) == 0 {
		return
	}
	r.Version++
	for _, e := range events {
		switch e.Kind {
		case "start":
			if len(e.Path) == 0 {
				continue
			}
			n := r.Node(e.Path)
			if n.Own == "" {
				n.Own = TestRunning
			}
		case "result":
			if len(e.Path) == 0 {
				continue
			}
			n := r.Node(e.Path)
			// A failure is sticky (jest reports `FAIL file` before its tests).
			if n.Own != TestFailed || e.Status == TestFailed {
				n.Own = e.Status
			}
			if e.DurationMs != nil {
				n.DurationMs = e.DurationMs
			}
		case "location":
			n := r.Node(e.Path)
			if n.Location == nil || (n.Location.Resolve == "rubyTest" && e.Location.Resolve != "rubyTest") {
				n.Location = e.Location
			}
		case "output":
			n := r.Node(e.Path)
			if len(n.Output) < maxTestOutputLines {
				n.Output = append(n.Output, e.Line)
			}
		case "summary":
			if e.Passed != nil {
				r.Summary.Passed = e.Passed
			}
			if e.Failed != nil {
				r.Summary.Failed = e.Failed
			}
			if e.Skipped != nil {
				r.Summary.Skipped = e.Skipped
			}
		}
	}
}

// Finish marks still-running tests as finished (skipped) when the process
// ends (results.ts finish).
func (r *TestResults) Finish(exitCode int) {
	r.Finished = true
	r.Version++
	for _, n := range r.byID {
		if n.Own == TestRunning {
			if len(n.Children) > 0 {
				n.Own = ""
			} else if exitCode == 0 {
				n.Own = TestPassed
			} else {
				n.Own = TestSkipped
			}
		}
	}
}

// Counts tallies the leaves; summary lines are authoritative when the
// per-test output was terse (results.ts counts()).
func (r *TestResults) Counts() TestCounts {
	c := TestCounts{}
	var visit func(n *TestNode)
	visit = func(n *TestNode) {
		if len(n.Children) > 0 {
			for _, ch := range n.Children {
				visit(ch)
			}
			return
		}
		if n.Own == "" {
			return
		}
		c.Total++
		switch n.Own {
		case TestPassed:
			c.Passed++
		case TestFailed:
			c.Failed++
		case TestSkipped:
			c.Skipped++
		case TestRunning:
			c.Running++
		}
	}
	for _, root := range r.Roots {
		visit(root)
	}
	if r.Summary.Passed != nil && *r.Summary.Passed > c.Passed {
		c.Passed = *r.Summary.Passed
	}
	if r.Summary.Failed != nil && *r.Summary.Failed > c.Failed {
		c.Failed = *r.Summary.Failed
	}
	if r.Summary.Skipped != nil && *r.Summary.Skipped > c.Skipped {
		c.Skipped = *r.Summary.Skipped
	}
	if total := c.Passed + c.Failed + c.Skipped + c.Running; total > c.Total {
		c.Total = total
	}
	return c
}

// FirstFailure is the first failed leaf in tree order (results.ts).
func (r *TestResults) FirstFailure() *TestNode {
	var visit func(nodes []*TestNode) *TestNode
	visit = func(nodes []*TestNode) *TestNode {
		for _, n := range nodes {
			if testNodeStatus(n) != TestFailed {
				continue
			}
			if len(n.Children) == 0 {
				return n
			}
			if hit := visit(n.Children); hit != nil {
				return hit
			}
			return n
		}
		return nil
	}
	return visit(r.Roots)
}

// Move relocates the subtree at from into into, merging (results.ts move),
// used by moveToReportedClasses for Gradle's simple class names.
func (r *TestResults) Move(from, into []string) {
	source, ok := r.byID[strings.Join(from, testResultsSep)]
	if !ok || strings.Join(from, testResultsSep) == strings.Join(into, testResultsSep) {
		return
	}
	r.Version++
	if len(from) > 1 {
		parent, ok := r.byID[strings.Join(from[:len(from)-1], testResultsSep)]
		if ok {
			for i, ch := range parent.Children {
				if ch == source {
					parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
					break
				}
			}
		}
	} else {
		for i, root := range r.Roots {
			if root == source {
				r.Roots = append(r.Roots[:i], r.Roots[i+1:]...)
				break
			}
		}
	}
	r.mergeInto(source, into)
}

func (r *TestResults) mergeInto(source *TestNode, into []string) {
	delete(r.byID, source.ID)
	target := r.Node(into)
	if source.Own != "" && (target.Own == "" || source.Own == TestFailed) {
		target.Own = source.Own
	}
	if target.DurationMs == nil {
		target.DurationMs = source.DurationMs
	}
	if target.Location == nil {
		target.Location = source.Location
	}
	if len(target.Output) == 0 {
		target.Output = source.Output
	}
	for _, child := range source.Children {
		r.mergeInto(child, append(append([]string(nil), into...), child.Name))
	}
}

// IsEmpty reports whether the tree has no roots (results.ts isEmpty).
func (r *TestResults) IsEmpty() bool { return len(r.Roots) == 0 }

// NodeOutputText renders a node's output followed by its descendants' in
// tree order (results.ts nodeOutput).
func NodeOutputText(node *TestNode) string {
	var b strings.Builder
	for _, line := range node.Output {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, ch := range node.Children {
		b.WriteString(NodeOutputText(ch))
	}
	return b.String()
}

// durationUnit matches one `\d+(unit)` token of a duration text, in the
// longest-first order upstream's regex alternation uses.
var durationUnit = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(ms|s|sec|m|min)\b`)

// NewTestResults creates an empty results tree.
func NewTestResults() *TestResults { return &TestResults{} }

// NewTestStdoutBuffer creates the line buffer a streamed run's output feeds.
func NewTestStdoutBuffer() *TestStdoutBuffer { return &TestStdoutBuffer{} }

// TestStdoutBuffer buffers streamed output into complete unsplit lines for
// the test parser (types.ts TestStdout): Feed returns the complete lines the
// text carried; End flushes the last partial one at process exit.
type TestStdoutBuffer struct {
	pending string
}

// Feed consumes streamed text and returns its complete lines.
func (b *TestStdoutBuffer) Feed(text string) []string {
	b.pending += text
	parts := strings.Split(b.pending, "\n")
	b.pending = parts[len(parts)-1]
	lines := parts[:len(parts)-1]
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimSuffix(line, "\r"))
	}
	return out
}

// End flushes the buffered partial line when the process ends.
func (b *TestStdoutBuffer) End() []string {
	pending := b.pending
	b.pending = ""
	if pending == "" {
		return nil
	}
	return []string{strings.TrimSuffix(pending, "\r")}
}

// ParseDuration parses `3 ms`, `(1.2 s)`, `0.00s`, `12ms`, `1m 3s` into
// milliseconds (types.ts parseDuration).
func ParseDuration(text string) *float64 {
	if text == "" {
		return nil
	}
	s := strings.NewReplacer("(", "", ")", "").Replace(strings.TrimSpace(text))
	total := 0.0
	matched := false
	for _, m := range durationUnit.FindAllStringSubmatch(s, -1) {
		matched = true
		var v float64
		fmt.Sscanf(m[1], "%g", &v)
		switch m[2] {
		case "ms":
			total += v
		case "m", "min":
			total += v * 60000
		default:
			total += v * 1000
		}
	}
	if !matched {
		return nil
	}
	rounded := float64(int64(total*1000+0.5)) / 1000
	return &rounded
}
