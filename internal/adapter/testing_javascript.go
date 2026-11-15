package adapter

// The port of src/features/run/testing/js.ts: Jest and Vitest reporters
// (default + verbose) folded into test events, plus rerun.ts's jsArgs and
// scriptArgs.ts's appendScriptArgs for the failed-tests filter.

import (
	"regexp"
	"strconv"
	"strings"

	"mayoi/internal/openidea"
)

var (
	jsTestFilePattern = `[^\s>()]+\.(?:[cm]?[jt]sx?|vue|svelte|astro)`
	jsJestFileRe      = regexp.MustCompile(`^\s*(PASS|FAIL)\s+(` + jsTestFilePattern + `)(?:\s+\(.*\))?\s*$`)
	jsVitestFailRe    = regexp.MustCompile(`^\s*FAIL\s+(` + jsTestFilePattern + `)\s+>\s+(.+?)\s*$`)
	jsJestFailHeadRe  = regexp.MustCompile(`^\s*● (.+)$`)
	jsVitestConsoleRe = regexp.MustCompile(`^(?:stdout|stderr) \| (` + jsTestFilePattern + `)(?:\s+>\s+(.+?))?\s*$`)
	jsLocationRe      = regexp.MustCompile(`(?:\(|\s|^)(?:file://)?((?:/|\.{0,2}/?)?` + jsTestFilePattern + `):(\d+):(\d+)\)?`)
	jsJestSummaryRe   = regexp.MustCompile(`^Tests:\s+(.+?)\s*$`)
	jsVitestSumRe     = regexp.MustCompile(`^\s*Tests\s+(.+?)\s+\((\d+)\)\s*$`)
	jsDurationSumRe   = regexp.MustCompile(`^\s*(?:Time:|Duration)\s+([\d.]+\s*m?s)`)
	jsTrailingDurRe   = regexp.MustCompile(`\s+\(?(\d+(?:\.\d+)?\s?m?s)\)?\s*$`)
	jsFileCountRe     = regexp.MustCompile(`\s+\((\d+) tests?(?:\s*\|[^)]*)?\)`)
	jsVitestCountRe   = regexp.MustCompile(`(\d+) (passed|failed|skipped|todo|pending)`)
	jsFileStartRe     = regexp.MustCompile(`^(` + jsTestFilePattern + `)(?:\s|$)`)
	jsSubRe           = regexp.MustCompile(`^\s+>\s+(.+)$`)
	jsSuiteRe         = regexp.MustCompile(`^(.+?) \(\d+\)$`)
)

func jsMarkStatus(mark string) TestStatus {
	if strings.ContainsAny(mark, "✓✔√") {
		return TestPassed
	}
	if strings.ContainsAny(mark, "✕×✗✖") {
		return TestFailed
	}
	if strings.ContainsAny(mark, "○↓⊘✎") {
		return TestSkipped
	}
	return ""
}

func jsCounts(text string) (passed, failed, skipped *int) {
	for _, m := range jsVitestCountRe.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "passed":
			v := n
			passed = &v
		case "failed":
			v := n
			failed = &v
		default:
			old := 0
			if skipped != nil {
				old = *skipped
			}
			v := old + n
			skipped = &v
		}
	}
	return passed, failed, skipped
}

func jsTrailingDuration(body string) *float64 {
	if m := jsTrailingDurRe.FindStringSubmatch(body); m != nil {
		return ParseDuration(m[1])
	}
	return nil
}

// JsParser folds Jest/Vitest output into events (js.ts jsParser).
type JsParser struct {
	file          string
	describes     []jsDescribe
	inVerbose     bool
	failure       []string
	locationFound bool
	consoleOf     []string
	// inFailureDetails is set by a Jest fail header (● …) and cleared by the
	// Tests: summary: marked lines inside the details are failure output,
	// not new results.
	inFailureDetails bool
}

type jsDescribe struct {
	indent int
	name   string
}

// NewJsParser creates the js test output parser.
func NewJsParser() *JsParser { return &JsParser{} }

func (p *JsParser) startFailure(path []string) {
	p.failure = append([]string(nil), path...)
	p.locationFound = false
}

// jsPath builds the tree path of a verbose test line: the file, the open
// describe blocks, and the test's own ` > `-joined names.
func (p *JsParser) jsPath(names []string) []string {
	var path []string
	if p.file != "" {
		path = append(path, p.file)
	}
	for _, d := range p.describes {
		path = append(path, d.name)
	}
	path = append(path, names...)
	return path
}

// Feed parses one output line.
func (p *JsParser) Feed(raw string) []TestEvent {
	var events []TestEvent
	line := strings.TrimRight(raw, " \t")
	var m []string

	if p.consoleOf != nil {
		if line != "" {
			events = append(events, TestEvent{Kind: "output", Path: p.consoleOf, Line: raw})
		} else {
			p.consoleOf = nil
		}
		return events
	}
	if m = jsVitestConsoleRe.FindStringSubmatch(line); m != nil {
		p.consoleOf = []string{m[1]}
		if m[2] != "" {
			p.consoleOf = append(p.consoleOf, regexp.MustCompile(`\s+>\s+`).Split(m[2], -1)...)
		}
		return events
	}

	if m = jsJestFileRe.FindStringSubmatch(line); m != nil {
		p.file = m[2]
		p.describes = nil
		p.inVerbose = true
		p.failure = nil
		status := TestFailed
		if m[1] == "PASS" {
			status = TestPassed
		}
		events = append(events, TestEvent{Kind: "result", Path: []string{p.file}, Status: status})
		return events
	}
	if strings.HasPrefix(strings.TrimLeft(line, " \t"), "⎯") {
		p.failure = nil
		return events
	}
	if m = jsVitestFailRe.FindStringSubmatch(line); m != nil {
		path := append([]string{m[1]}, regexp.MustCompile(`\s+>\s+`).Split(m[2], -1)...)
		events = append(events, TestEvent{Kind: "result", Path: path, Status: TestFailed})
		p.startFailure(path)
		return events
	}
	if m = jsJestSummaryRe.FindStringSubmatch(line); m != nil {
		passed, failed, skipped := jsCounts(m[1])
		events = append(events, TestEvent{Kind: "summary", Passed: passed, Failed: failed, Skipped: skipped})
		p.failure = nil
		p.inFailureDetails = false
		return events
	}
	if m = jsVitestSumRe.FindStringSubmatch(line); m != nil {
		passed, failed, skipped := jsCounts(m[1])
		events = append(events, TestEvent{Kind: "summary", Passed: passed, Failed: failed, Skipped: skipped})
		p.failure = nil
		return events
	}
	if m = jsDurationSumRe.FindStringSubmatch(line); m != nil {
		events = append(events, TestEvent{Kind: "summary", DurationMs: ParseDuration(m[1])})
		return events
	}

	trimmed := strings.TrimLeft(line, " \t")
	indent := len(line) - len(trimmed)
	if trimmed == "" {
		if p.inVerbose {
			p.inVerbose = false
		}
		if p.failure != nil {
			events = append(events, TestEvent{Kind: "output", Path: p.failure, Line: raw})
		}
		return events
	}
	mark := trimmed[0:1]
	var status TestStatus
	if len(trimmed) > 1 && trimmed[1] == ' ' {
		status = jsMarkStatus(mark)
	}

	// Vitest: ` ❯ src/a.test.ts:5:13` (failure location) or ` ❯ src/a.test.ts (2 tests | 1 failed)`.
	if mark == "❯" || mark == "→" {
		body := strings.TrimLeft(strings.TrimPrefix(trimmed, mark), " ")
		if loc := jsLocationRe.FindStringSubmatch(strings.TrimPrefix(strings.TrimPrefix(trimmed, mark), " ")); loc != nil && p.failure != nil && !p.locationFound {
			p.locationFound = true
			lineNo, _ := strconv.Atoi(loc[2])
			col, _ := strconv.Atoi(loc[3])
			events = append(events, TestEvent{Kind: "location", Path: p.failure,
				Location: &TestLocation{Path: loc[1], Line: lineNo, Col: col, Resolve: "path"}})
			return events
		}
		if fm := regexp.MustCompile(`^(` + jsTestFilePattern + `)`).FindStringSubmatch(body); fm != nil && jsFileCountRe.MatchString(body) {
			p.file = fm[1]
			events = append(events, TestEvent{Kind: "result", Path: []string{p.file}, Status: TestFailed, DurationMs: jsTrailingDuration(body)})
			return events
		}
		if sm := jsSuiteRe.FindStringSubmatch(body); sm != nil && p.file != "" && p.failure == nil {
			for len(p.describes) > 0 && p.describes[len(p.describes)-1].indent >= indent {
				p.describes = p.describes[:len(p.describes)-1]
			}
			p.describes = append(p.describes, jsDescribe{indent: indent, name: sm[1]})
			return events
		}
		if p.failure != nil {
			events = append(events, TestEvent{Kind: "output", Path: p.failure, Line: raw})
		}
		return events
	}

	if status != "" && !(p.failure != nil && (mark == "✕" || mark == "×" || mark == "✗" || mark == "✖") && p.inFailureDetails) {
		body := strings.TrimPrefix(trimmed, mark+" ")
		durationMs := jsTrailingDuration(body)
		body = jsTrailingDurRe.ReplaceAllString(body, "")
		if fm := jsFileStartRe.FindStringSubmatch(body); fm != nil {
			rest := body[len(fm[1]):]
			if sub := jsSubRe.FindStringSubmatch(rest); sub != nil {
				// Vitest verbose (flat): `✓ src/a.test.ts > suite > test 2ms`.
				names := regexp.MustCompile(`\s+>\s+`).Split(sub[1], -1)
				events = append(events, TestEvent{Kind: "result",
					Path: append([]string{fm[1]}, names...), Status: status, DurationMs: durationMs})
			} else {
				// Vitest file line: `✓ src/a.test.ts (3 tests) 5ms`.
				p.file = fm[1]
				p.describes = nil
				events = append(events, TestEvent{Kind: "result", Path: []string{p.file}, Status: status, DurationMs: durationMs})
			}
			return events
		}
		body = jsFileCountRe.ReplaceAllString(body, "")
		for len(p.describes) > 0 && p.describes[len(p.describes)-1].indent >= indent {
			p.describes = p.describes[:len(p.describes)-1]
		}
		names := regexp.MustCompile(`\s+(?:>|›)\s+`).Split(body, -1)
		events = append(events, TestEvent{Kind: "result", Path: p.jsPath(names), Status: status, DurationMs: durationMs})
		return events
	}

	if m = jsJestFailHeadRe.FindStringSubmatch(line); m != nil {
		names := regexp.MustCompile(`\s+›\s+`).Split(m[1], -1)
		if names[0] == "Test suite failed to run" {
			if p.file != "" {
				p.startFailure([]string{p.file})
			}
		} else {
			path := p.jsPath(names)
			events = append(events, TestEvent{Kind: "result", Path: path, Status: TestFailed})
			p.startFailure(path)
		}
		p.inVerbose = false
		p.inFailureDetails = true
		return events
	}

	if p.inVerbose && p.file != "" && trimmed != "" && indent > 0 && p.failure == nil {
		// Jest verbose describe header (indented, no mark).
		for len(p.describes) > 0 && p.describes[len(p.describes)-1].indent >= indent {
			p.describes = p.describes[:len(p.describes)-1]
		}
		p.describes = append(p.describes, jsDescribe{indent: indent, name: trimmed})
		return events
	}

	if p.failure != nil {
		events = append(events, TestEvent{Kind: "output", Path: p.failure, Line: raw})
		if !p.locationFound && !strings.Contains(line, "node_modules") {
			var loc []string
			if strings.Contains(line, "at") || regexp.MustCompile(`^\s*❯`).MatchString(line) {
				loc = jsLocationRe.FindStringSubmatch(line)
			}
			if loc != nil {
				p.locationFound = true
				lineNo, _ := strconv.Atoi(loc[2])
				col, _ := strconv.Atoi(loc[3])
				events = append(events, TestEvent{Kind: "location", Path: p.failure,
					Location: &TestLocation{Path: loc[1], Line: lineNo, Col: col, Resolve: "path"}})
			}
		}
	}
	return events
}

// commandName is scriptArgs.ts commandName's npm family check.
func jsIsNpmFamily(command string) bool {
	return regexp.MustCompile(`(?i)(^|[/\\])(npm|yarn|bun)(\.\w+)?$`).MatchString(strings.TrimSpace(command))
}

// AppendScriptArgs appends arguments after `--` for the npm family
// (scriptArgs.ts appendScriptArgs): pnpm and friends forward `--` to the
// script, where it ends the test runner's option parsing.
func AppendScriptArgs(command string, args, extra []string) []string {
	separator := []string(nil)
	if !jsHasArg(args, "--") && jsIsNpmFamily(command) {
		separator = []string{"--"}
	}
	return append(append(append([]string(nil), args...), separator...), extra...)
}

func jsHasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// JsArgs derives the arguments that run only the selected js tests
// (rerun.ts jsArgs): the test files as targets and the test names as a
// `-t` regular expression.
func JsArgs(config *openidea.RunConfig, paths [][]string) []string {
	seenFile := map[string]bool{}
	var files []string
	seenName := map[string]bool{}
	var names []string
	for _, p := range paths {
		if len(p) > 0 && p[0] != "" && !seenFile[p[0]] {
			seenFile[p[0]] = true
			files = append(files, p[0])
		}
		if len(p) > 1 {
			joined := strings.Join(p[1:], " ")
			if !seenName[joined] {
				seenName[joined] = true
				names = append(names, joined)
			}
		}
	}
	if len(files) == 0 {
		return nil
	}
	before, after, hasAfter := splitDashDash(config.Args)
	flags := []string{"-t", "--testNamePattern"}
	args := before
	if hasAfter {
		args = append(before, append([]string{"--"}, dropFlag(after, flags)...)...)
	} else {
		args = dropFlag(before, flags)
	}
	var filter []string
	if len(names) > 0 {
		anchored := make([]string, 0, len(names))
		for _, n := range names {
			anchored = append(anchored, "^"+escapeRegExp(n)+"$")
		}
		filter = []string{"-t", "^(?:" + strings.Join(anchored, "|") + ")$"}
	}
	extra := append(append([]string(nil), files...), filter...)
	return AppendScriptArgs(config.Command, args, extra)
}
