package adapter

// The port of src/features/run/testing/pytest.ts: pytest's verbose lines,
// `-rA` short summaries, progress dots, failure sections and the final
// summary line, folded into test events.

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	pyVerboseRe   = regexp.MustCompile(`^(\S+\.py::\S+)\s+(PASSED|FAILED|ERROR|SKIPPED|XFAIL|XPASS)(?:\s|$)`)
	pyPrefixRe    = regexp.MustCompile(`^(?:\[gw\d+\]\s+(?:\[\s*\d+%\]\s+)?)?(PASSED|FAILED|ERROR|SKIPPED|XFAIL|XPASS)\s+(\S+\.py::\S+?)(?:\s+-\s+.*)?$`)
	pyProgressRe  = regexp.MustCompile(`^(\S+\.py) ([.FEsxX]+)\s*(?:\[\s*\d+%\])?$`)
	pySectionRe   = regexp.MustCompile(`^_{3,} (.+?) _{3,}$`)
	pyLocationRe  = regexp.MustCompile(`^(\S+\.py):(\d+): `)
	pySummaryRe   = regexp.MustCompile(`^=+ (.*\b(?:passed|failed|error|errors|skipped|deselected|xfailed|xpassed|no tests ran)\b.*?) =+$`)
	pyBannerRe    = regexp.MustCompile(`^=+ .* =+$`)
	pyCountRe     = regexp.MustCompile(`(\d+) (passed|failed|errors?|skipped|xfailed|xpassed)`)
	pyDurationRe  = regexp.MustCompile(` in ([\d.]+s)`)
	pyTitleStrip  = regexp.MustCompile(`^(ERROR at \w+ of |ERROR collecting )`)
	pySectionName = regexp.MustCompile(`^[^.]+\.[^.]+\.`)
)

func pytestStatusOf(outcome string) TestStatus {
	switch outcome {
	case "PASSED", "XFAIL":
		return TestPassed
	case "SKIPPED":
		return TestSkipped
	default:
		return TestFailed
	}
}

// PytestParser folds pytest output into events (pytest.ts pytestParser).
type PytestParser struct {
	known         [][]string
	failure       []string
	locationFound bool
	pending       *pytestPending
	unresolved    []*pytestPending
}

type pytestPending struct {
	parts  []string
	events []TestEvent
}

// NewPytestParser creates the pytest output parser.
func NewPytestParser() *PytestParser { return &PytestParser{} }

func pytestEndsWith(path, parts []string) bool {
	if len(path) < len(parts) {
		return false
	}
	tail := path[len(path)-len(parts):]
	for i := range parts {
		if tail[i] != parts[i] {
			return false
		}
	}
	return true
}

// remember records a node id and returns the buffered failure events that
// now belong to it.
func (p *PytestParser) remember(path []string) []TestEvent {
	joined := strings.Join(path, "::")
	found := false
	for _, k := range p.known {
		if strings.Join(k, "::") == joined {
			found = true
			break
		}
	}
	if !found {
		p.known = append(p.known, append([]string(nil), path...))
	}
	var out []TestEvent
	for i := len(p.unresolved) - 1; i >= 0; i-- {
		u := p.unresolved[i]
		if !pytestEndsWith(path, u.parts) {
			continue
		}
		p.unresolved = append(p.unresolved[:i], p.unresolved[i+1:]...)
		for _, e := range u.events {
			if e.Kind == "output" || e.Kind == "location" {
				ne := e
				ne.Path = append([]string(nil), path...)
				out = append(out, ne)
			}
		}
	}
	return out
}

// pytestTitleParts splits a failure section title: `test_name` or
// `TestClass.test_name`.
func pytestTitleParts(title string) []string {
	title = pyTitleStrip.ReplaceAllString(title, "")
	return strings.Split(title, ".")
}

// Feed parses one output line.
func (p *PytestParser) Feed(line string) []TestEvent {
	var events []TestEvent
	if m := pyVerboseRe.FindStringSubmatch(line); m != nil {
		path := splitNodeID(m[1])
		events = append(events, TestEvent{Kind: "result", Path: path, Status: pytestStatusOf(m[2])})
		events = append(events, p.remember(path)...)
		return events
	}
	if m := pyPrefixRe.FindStringSubmatch(line); m != nil {
		path := splitNodeID(m[2])
		events = append(events, TestEvent{Kind: "result", Path: path, Status: pytestStatusOf(m[1])})
		events = append(events, p.remember(path)...)
		p.failure = nil
		p.pending = nil
		return events
	}
	if m := pyProgressRe.FindStringSubmatch(line); m != nil {
		progress := m[2]
		failed := strings.ContainsAny(progress, "FE")
		allSkipped := regexp.MustCompile(`^[sx]+$`).MatchString(progress)
		status := TestPassed
		if failed {
			status = TestFailed
		} else if allSkipped {
			status = TestSkipped
		}
		events = append(events, TestEvent{Kind: "result", Path: []string{m[1]}, Status: status})
		return events
	}
	if m := pySummaryRe.FindStringSubmatch(line); m != nil {
		p.failure = nil
		counts := map[string]int{}
		for _, c := range pyCountRe.FindAllStringSubmatch(m[1], -1) {
			n, _ := strconv.Atoi(c[1])
			switch c[2] {
			case "passed", "xfailed":
				counts["passed"] += n
			case "skipped":
				counts["skipped"] = n
			default:
				counts["failed"] += n
			}
		}
		event := TestEvent{Kind: "summary"}
		if v, ok := counts["passed"]; ok {
			event.Passed = &v
		}
		if v, ok := counts["failed"]; ok {
			event.Failed = &v
		}
		if v, ok := counts["skipped"]; ok {
			event.Skipped = &v
		}
		if d := pyDurationRe.FindStringSubmatch(m[1]); d != nil {
			event.DurationMs = ParseDuration(d[1])
		}
		events = append(events, event)
		return events
	}
	if m := pySectionRe.FindStringSubmatch(line); m != nil {
		parts := pytestTitleParts(m[1])
		hit := []string(nil)
		for _, k := range p.known {
			if pytestEndsWith(k, parts) {
				hit = k
				break
			}
		}
		if hit != nil {
			p.failure = append([]string(nil), hit...)
			p.pending = nil
		} else {
			p.failure = append([]string(nil), parts...)
			p.pending = &pytestPending{parts: append([]string(nil), parts...)}
			p.unresolved = append(p.unresolved, p.pending)
		}
		p.locationFound = false
		return events
	}
	if pyBannerRe.MatchString(line) {
		p.failure = nil
		p.pending = nil
		return events
	}
	if p.failure != nil {
		sink := &events
		if p.pending != nil {
			p.pending.events = append(p.pending.events, TestEvent{Kind: "output", Path: p.failure, Line: line})
			sink = nil
		} else {
			*sink = append(*sink, TestEvent{Kind: "output", Path: p.failure, Line: line})
		}
		if loc := pyLocationRe.FindStringSubmatch(line); loc != nil && !p.locationFound {
			p.locationFound = true
			lineNo, _ := strconv.Atoi(loc[2])
			locEvent := TestEvent{Kind: "location", Path: p.failure,
				Location: &TestLocation{Path: loc[1], Line: lineNo, Resolve: "path"}}
			if p.pending != nil {
				p.pending.events = append(p.pending.events, locEvent)
			} else {
				events = append(events, locEvent)
			}
		}
	}
	return events
}

// End reports the failure sections that never got a node id under their
// title (pytest.ts end).
func (p *PytestParser) End() []TestEvent {
	var out []TestEvent
	for _, u := range p.unresolved {
		out = append(out, TestEvent{Kind: "result", Path: u.parts, Status: TestFailed})
		out = append(out, u.events...)
	}
	p.unresolved = nil
	return out
}

// splitNodeID splits a pytest node id into its path parts.
func splitNodeID(nodeid string) []string {
	var parts []string
	for _, s := range strings.Split(nodeid, "::") {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}
