package adapter

// Python unittest and Django test output, ported from
// src/features/run/testing/unittest.ts.

import (
	"regexp"
	"strconv"
	"strings"

	"mayoi/internal/openidea"
)

const unittestIdentifier = `[\p{L}_][\p{L}\p{N}_]*`

var (
	unittestHeaderRe = regexp.MustCompile(
		`^\s*((?:` + unittestIdentifier + `)(?:\.(?:` + unittestIdentifier + `))*) ` +
			`\(((?:` + unittestIdentifier + `)(?:\.(?:` + unittestIdentifier + `))+)\)` +
			`(?: \[.*?\])?(?: \(.*?\))?( \.\.\.|$)`,
	)
	unittestOutcomeRe = regexp.MustCompile(
		`^(ok|FAIL|ERROR|skipped(?: .*)?|expected failure|unexpected success)$`,
	)
	unittestTrailingOutcomeRe = regexp.MustCompile(
		` \.\.\. (ok|FAIL|ERROR|skipped(?: .*)?|expected failure|unexpected success)$`,
	)
	unittestSectionHeaderRe = regexp.MustCompile(`^(FAIL|ERROR|UNEXPECTED SUCCESS): (.+)$`)
	unittestFrameRe         = regexp.MustCompile(`^\s*File "([^"]+)", line (\d+), in (\S+)`)
	unittestLibraryRe       = regexp.MustCompile(
		`[/\\](?:site-packages|dist-packages)[/\\]|` +
			`/lib/python[\d.]*/(?:unittest/|[^/]+\.py$)|` +
			`\\Lib\\(?:unittest\\|[^\\]+\.py$)`,
	)
	unittestRanRe     = regexp.MustCompile(`^Ran (\d+) tests? in ([\d.]+s)$`)
	unittestResultRe  = regexp.MustCompile(`^(OK|FAILED|NO TESTS RAN)(?: \((.*)\))?$`)
	unittestCountRe   = regexp.MustCompile(`([a-z ]+)=(\d+)`)
	unittestVerboseRe = regexp.MustCompile(`^(-v|--verbosity)(=?\d)?$`)
	unittestManageRe  = regexp.MustCompile(`(^|[/\\])manage\.py$`)
)

const (
	unittestRule1 = "======================================================================"
	unittestRule2 = "----------------------------------------------------------------------"
)

func unittestStatus(outcome string) TestStatus {
	switch {
	case outcome == "ok" || outcome == "expected failure":
		return TestPassed
	case strings.HasPrefix(outcome, "skipped"):
		return TestSkipped
	default:
		return TestFailed
	}
}

func unittestTestPath(name, dotted string) []string {
	if strings.HasPrefix(dotted, "unittest.loader._Failed") {
		return []string{name}
	}
	if name == "setUpModule" || name == "tearDownModule" {
		return []string{dotted}
	}
	id := dotted
	if strings.HasSuffix(dotted, "."+name) {
		id = strings.TrimSuffix(dotted, "."+name)
	}
	dot := strings.LastIndex(id, ".")
	parts := []string{id}
	if dot >= 0 {
		parts = []string{id[:dot], id[dot+1:]}
	}
	if name == "setUpClass" || name == "tearDownClass" {
		return parts
	}
	return append(parts, name)
}

func unittestHeader(line string) (name, dotted, after string, ok bool) {
	match := unittestHeaderRe.FindStringSubmatchIndex(line)
	if match == nil {
		return "", "", "", false
	}
	name = line[match[2]:match[3]]
	dotted = line[match[4]:match[5]]
	after = strings.TrimPrefix(line[match[1]:], " ")
	return name, dotted, after, true
}

type unittestSection struct {
	path     []string
	name     string
	ruled    bool
	location *TestLocation
	fallback *TestLocation
}

// UnittestParser folds Python unittest and Django output into test events.
type UnittestParser struct {
	pending    []string
	section    *unittestSection
	ran        bool
	count      int
	durationMs *float64
}

// NewUnittestParser creates the unittest output parser.
func NewUnittestParser() *UnittestParser {
	return &UnittestParser{}
}

func (p *UnittestParser) closeSection() []TestEvent {
	section := p.section
	p.section = nil
	if section == nil {
		return nil
	}
	location := section.location
	if location == nil {
		location = section.fallback
	}
	if location == nil {
		return nil
	}
	return []TestEvent{{Kind: "location", Path: section.path, Location: location}}
}

func (p *UnittestParser) results(line string, events []TestEvent) ([]TestEvent, bool) {
	rest := line
	matched := false
	for {
		name, dotted, after, ok := unittestHeader(rest)
		if !ok {
			break
		}
		outcome := ""
		if match := unittestOutcomeRe.FindStringSubmatch(after); match != nil {
			outcome = match[1]
		}
		// A delayed result only belongs to a test method. Fixtures and
		// modules print theirs immediately.
		if outcome == "" && !strings.HasPrefix(name, "test") {
			break
		}
		matched = true
		path := unittestTestPath(name, dotted)
		if outcome != "" {
			events = append(events, TestEvent{
				Kind: "result", Path: path, Status: unittestStatus(outcome),
			})
			p.pending = nil
			return events, true
		}
		events = append(events, TestEvent{Kind: "start", Path: path})
		p.pending = path
		rest = after
	}
	return events, matched
}

func (p *UnittestParser) startSection(line string) []TestEvent {
	match := unittestSectionHeaderRe.FindStringSubmatch(line)
	if match == nil {
		return nil
	}
	name, dotted, _, ok := unittestHeader(match[2])
	if !ok {
		return nil
	}
	path := unittestTestPath(name, dotted)
	sectionName := name
	if strings.HasPrefix(dotted, "unittest.loader._Failed") {
		sectionName = "<module>"
	}
	p.section = &unittestSection{path: path, name: sectionName}
	return []TestEvent{
		{Kind: "result", Path: path, Status: TestFailed},
		{Kind: "output", Path: path, Line: line},
	}
}

func (p *UnittestParser) feedSection(line string) []TestEvent {
	if line == unittestRule2 {
		if !p.section.ruled {
			p.section.ruled = true
			return nil
		}
		return p.closeSection()
	}
	events := []TestEvent{{Kind: "output", Path: p.section.path, Line: line}}
	if match := unittestFrameRe.FindStringSubmatch(line); match != nil &&
		!unittestLibraryRe.MatchString(match[1]) {
		lineNo, _ := strconv.Atoi(match[2])
		location := &TestLocation{Path: match[1], Line: lineNo, Resolve: "path"}
		if match[3] == p.section.name {
			if p.section.location == nil {
				p.section.location = location
			}
		} else if p.section.fallback == nil {
			p.section.fallback = location
		}
	}
	return events
}

func (p *UnittestParser) summary(line string) []TestEvent {
	match := unittestResultRe.FindStringSubmatch(line)
	if match == nil || !p.ran {
		return nil
	}
	counts := map[string]int{}
	for _, item := range unittestCountRe.FindAllStringSubmatch(match[2], -1) {
		value, _ := strconv.Atoi(item[2])
		counts[strings.TrimSpace(item[1])] = value
	}
	failed := counts["failures"] + counts["errors"] + counts["unexpected successes"]
	skipped := counts["skipped"]
	passed := max(0, p.count-failed-skipped)
	p.ran = false
	return []TestEvent{{
		Kind:       "summary",
		Passed:     &passed,
		Failed:     &failed,
		Skipped:    &skipped,
		DurationMs: p.durationMs,
	}}
}

// Feed turns one output line into test events.
func (p *UnittestParser) Feed(line string) []TestEvent {
	if line == unittestRule1 {
		p.pending = nil
		return p.closeSection()
	}
	if p.section != nil && unittestRanRe.MatchString(line) {
		events := p.closeSection()
		return append(events, p.Feed(line)...)
	}
	if p.section != nil {
		return p.feedSection(line)
	}
	if events := p.startSection(line); events != nil {
		return events
	}
	if match := unittestRanRe.FindStringSubmatch(line); match != nil {
		p.count, _ = strconv.Atoi(match[1])
		p.durationMs = ParseDuration(match[2])
		p.ran = true
		return nil
	}
	if events := p.summary(line); events != nil {
		return events
	}
	if events, ok := p.results(line, nil); ok {
		return events
	}
	if p.pending != nil {
		outcome := ""
		if match := unittestOutcomeRe.FindStringSubmatch(line); match != nil {
			outcome = match[1]
		} else if match := unittestTrailingOutcomeRe.FindStringSubmatch(line); match != nil {
			outcome = match[1]
		}
		if outcome != "" {
			pending := p.pending
			p.pending = nil
			return []TestEvent{{Kind: "result", Path: pending, Status: unittestStatus(outcome)}}
		}
	}
	return nil
}

// End flushes a location collected by a failure section cut off at EOF.
func (p *UnittestParser) End() []TestEvent {
	return p.closeSection()
}

// UnittestWithVerbose requests one result line per test, matching
// withTestReporting's unittest branch.
func UnittestWithVerbose(config *openidea.RunConfig) []string {
	args := append([]string(nil), config.Args...)
	for _, arg := range args {
		if unittestVerboseRe.MatchString(arg) {
			return args
		}
	}
	return append(args, "-v", "2")
}

func unittestManageTestAt(args []string) int {
	for i := 0; i+1 < len(args); i++ {
		if unittestManageRe.MatchString(args[i]) && args[i+1] == "test" {
			return i
		}
	}
	return -1
}

// UnittestLabels turns failed test tree paths into dotted Django labels.
func UnittestLabels(paths [][]string) []string {
	seen := map[string]bool{}
	var labels []string
	for _, path := range paths {
		label := strings.Join(path, ".")
		if label != "" && !seen[label] {
			seen[label] = true
			labels = append(labels, label)
		}
	}
	return labels
}

// UnittestArgs replaces a Django configuration's target with labels.
func UnittestArgs(base, labels []string) []string {
	at := unittestManageTestAt(base)
	if at < 0 || len(labels) == 0 {
		return nil
	}
	rest := base[at+2:]
	count := len(rest)
	for i, arg := range rest {
		if strings.HasPrefix(arg, "-") {
			count = i
			break
		}
	}
	out := append([]string(nil), base[:at+2]...)
	out = append(out, labels...)
	return append(out, rest[count:]...)
}
