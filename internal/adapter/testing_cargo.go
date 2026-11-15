package adapter

// The port of src/features/run/testing/cargo.ts: `cargo test` (libtest) and
// `cargo nextest` output folded into test events, and rerun.ts's cargoArgs
// for the failed-tests filter.

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	cargoRunningRe     = regexp.MustCompile(`^\s+Running (?:unittests )?(\S+) \(.+\)$`)
	cargoDoctestsRe    = regexp.MustCompile(`^\s*Doc-tests (\S+)`)
	cargoTestRe        = regexp.MustCompile(`^test (.+?) \.\.\. (ok|FAILED|ignored|bench)(?:,.*)?(?:\s+<([\d.]+s)>)?$`)
	cargoFailureHeadRe = regexp.MustCompile(`^---- (.+?) (?:stdout|stderr) ----$`)
	// Rust ≥ 1.73: `thread 'x' panicked at src/lib.rs:10:5:`; older: `panicked at 'msg', src/lib.rs:10:5`.
	cargoPanicRe      = regexp.MustCompile(`panicked at (?:'.*', |".*", )?([^\s:'"][^:'"]*):(\d+):(\d+)`)
	cargoSummaryRe    = regexp.MustCompile(`^test result: (?:ok|FAILED)\. (\d+) passed; (\d+) failed; (\d+) ignored;.*?(?:finished in ([\d.]+s))?$`)
	cargoNextestRe    = regexp.MustCompile(`^\s*(PASS|FAIL|SKIP|SIGSEGV|SIGABRT|TIMEOUT|LEAK)\s+\[\s*([\d.]+s)\]\s+(\S+)\s+(.+)$`)
	cargoNextestSumRe = regexp.MustCompile(`^\s*Summary \[\s*([\d.]+s)\]\s+(\d+) tests? run: (\d+) passed(?:.*?(\d+) failed)?(?:.*?(\d+) skipped)?`)
)

// CargoParser folds cargo test output into events (cargo.ts cargoParser).
type CargoParser struct {
	suite   string
	failure []string
	totals  struct {
		passed, failed, skipped int
		durationMs              float64
	}
}

// NewCargoParser creates the cargo test output parser.
func NewCargoParser() *CargoParser { return &CargoParser{} }

func (p *CargoParser) pathOf(name string) []string {
	parts := strings.Split(name, "::")
	if p.suite != "" {
		return append([]string{p.suite}, parts...)
	}
	return parts
}

func intPtr(v int) *int { return &v }

// Feed parses one output line.
func (p *CargoParser) Feed(line string) []TestEvent {
	var events []TestEvent
	if m := cargoRunningRe.FindStringSubmatch(line); m != nil {
		p.suite = m[1]
		p.failure = nil
		return events
	}
	if m := cargoDoctestsRe.FindStringSubmatch(line); m != nil {
		p.suite = "Doc-tests " + m[1]
		p.failure = nil
		return events
	}
	if m := cargoTestRe.FindStringSubmatch(line); m != nil {
		status := TestFailed
		switch m[2] {
		case "ok", "bench":
			status = TestPassed
		case "ignored":
			status = TestSkipped
		}
		events = append(events, TestEvent{Kind: "result", Path: p.pathOf(m[1]), Status: status, DurationMs: ParseDuration(m[3])})
		return events
	}
	if m := cargoFailureHeadRe.FindStringSubmatch(line); m != nil {
		p.failure = p.pathOf(m[1])
		return events
	}
	if m := cargoSummaryRe.FindStringSubmatch(line); m != nil {
		p.failure = nil
		passed, _ := strconv.Atoi(m[1])
		failed, _ := strconv.Atoi(m[2])
		skipped, _ := strconv.Atoi(m[3])
		p.totals.passed += passed
		p.totals.failed += failed
		p.totals.skipped += skipped
		if d := ParseDuration(m[4]); d != nil {
			p.totals.durationMs += *d
		}
		events = append(events, TestEvent{Kind: "summary",
			Passed: intPtr(p.totals.passed), Failed: intPtr(p.totals.failed), Skipped: intPtr(p.totals.skipped)})
		return events
	}
	if m := cargoNextestRe.FindStringSubmatch(line); m != nil {
		status := TestFailed
		switch m[1] {
		case "PASS":
			status = TestPassed
		case "SKIP":
			status = TestSkipped
		}
		events = append(events, TestEvent{Kind: "result",
			Path: append([]string{m[3]}, strings.Split(m[4], "::")...), Status: status, DurationMs: ParseDuration(m[2])})
		return events
	}
	if m := cargoNextestSumRe.FindStringSubmatch(line); m != nil {
		passed, _ := strconv.Atoi(m[3])
		failed := 0
		if m[4] != "" {
			failed, _ = strconv.Atoi(m[4])
		}
		skipped := 0
		if m[5] != "" {
			skipped, _ = strconv.Atoi(m[5])
		}
		events = append(events, TestEvent{Kind: "summary",
			DurationMs: ParseDuration(m[1]), Passed: intPtr(passed), Failed: intPtr(failed), Skipped: intPtr(skipped)})
		return events
	}
	if p.failure != nil {
		if line == "failures:" || strings.HasPrefix(line, "failures:") {
			p.failure = nil
		} else {
			events = append(events, TestEvent{Kind: "output", Path: p.failure, Line: line})
			if pm := cargoPanicRe.FindStringSubmatch(line); pm != nil {
				c, _ := strconv.Atoi(pm[3])
				events = append(events, TestEvent{Kind: "location", Path: p.failure,
					Location: &TestLocation{Path: pm[1], Line: atoi(pm[2]), Col: c, Resolve: "path"}})
			}
		}
	}
	return events
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// CargoArgs derives the arguments that run only the selected cargo tests
// (rerun.ts cargoArgs): the `::`-joined test paths after `--`, with the
// runner's own `--exact`.
func CargoArgs(base []string, paths [][]string) []string {
	names := map[string]bool{}
	var order []string
	for _, p := range paths {
		if len(p) > 0 && strings.HasPrefix(p[0], "Doc-tests ") {
			continue
		}
		// The libtest filter names the test without its cargo suite: strip
		// the leading src/lib.rs (rerun.ts: p.length > 1 ? p.slice(1) : p).
		joined := strings.Join(p, "::")
		if len(p) > 1 {
			joined = strings.Join(p[1:], "::")
		}
		if !names[joined] {
			names[joined] = true
			order = append(order, joined)
		}
	}
	if len(order) == 0 {
		return nil
	}
	before, after, hasAfter := splitDashDash(base)
	rest := after
	if hasAfter {
		rest = dropFlag(after, []string{"--exact"})
	}
	out := append([]string(nil), before...)
	out = append(out, "--")
	out = append(out, rest...)
	out = append(out, order...)
	out = append(out, "--exact")
	return out
}
