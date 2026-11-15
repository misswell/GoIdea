package adapter

// The port of src/features/run/testing/go.ts: `go test` output (plain, -v and
// -json) folded into test events, plus goArguments.ts's goTestParts and
// withoutGoSelection that the rerun derivation needs.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// ParseDurationMs is ParseDuration returning milliseconds or 0 handling the
// plain `([\d.]+s)` form the go parser matches.
func parseDurationPlain(text string) *float64 {
	if text == "" {
		return nil
	}
	var v float64
	s := strings.TrimSuffix(text, "s")
	if strings.HasSuffix(s, "m") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		if err != nil {
			return nil
		}
		v = f * 60
	} else {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		v = f
	}
	return &v
}

var (
	goRunRe        = regexp.MustCompile(`^=== (?:RUN|CONT|PAUSE)\s+(\S+)`)
	goResultRe     = regexp.MustCompile(`^\s*--- (PASS|FAIL|SKIP): (\S+) \(([\d.]+s)\)`)
	goPkgRe        = regexp.MustCompile(`^(ok|FAIL|---)\s+(\S+)\s+(?:\(cached\)|([\d.]+s)|\[[^\]]+\])`)
	goLocationRe   = regexp.MustCompile(`^\s+([\w.-]+\.go):(\d+):`)
	goBenchmarkRe  = regexp.MustCompile(`^(Benchmark\S*) *\t[ \t]*\d+ *(?:\t|$)`)
	goJSONLocation = regexp.MustCompile(`^\s*(.+\.go):(\d+)(?::(\d+))?:`)
)

// GoTestEvent is the parser's output event; kinds match upstream.
type GoTestEvent struct {
	Kind       string
	Path       []string
	Status     TestStatus
	DurationMs *float64
	Line       string
	Location   *TestLocation
	Passed     *int
	Failed     *int
	Skipped    *int
}

// toTestEvents converts the go parser's events into the shared ones.
func toTestEvents(ges []GoTestEvent) []TestEvent {
	out := make([]TestEvent, 0, len(ges))
	for _, e := range ges {
		ev := TestEvent{Kind: e.Kind, Path: e.Path, Status: e.Status, DurationMs: e.DurationMs, Line: e.Line, Location: e.Location, Passed: e.Passed, Failed: e.Failed, Skipped: e.Skipped}
		out = append(out, ev)
	}
	return out
}

// plainGoParser parses plain `go test` / `go test -v` output (go.ts
// plainGoParser): results buffer per package and flush under it when the
// trailing ok/FAIL line arrives.
type plainGoParser struct {
	buffer  []GoTestEvent
	current []string
	totals  struct{ passed, failed, skipped int }
	located map[string]bool
}

func newPlainGoParser() *plainGoParser {
	return &plainGoParser{located: map[string]bool{}}
}

func (p *plainGoParser) flush(pkg string) []GoTestEvent {
	out := make([]GoTestEvent, 0, len(p.buffer))
	for _, e := range p.buffer {
		if pkg != "" && len(e.Path) > 0 {
			e.Path = append([]string{pkg}, e.Path...)
		}
		out = append(out, e)
	}
	p.buffer = nil
	p.current = nil
	p.located = map[string]bool{}
	return out
}

func (p *plainGoParser) feed(line string) []GoTestEvent {
	var events []GoTestEvent
	if m := goRunRe.FindStringSubmatch(line); m != nil {
		path := strings.Split(m[1], "/")
		p.buffer = append(p.buffer, GoTestEvent{Kind: "start", Path: path})
		p.current = path
		return nil
	}
	if m := goResultRe.FindStringSubmatch(line); m != nil {
		status := TestPassed
		switch m[1] {
		case "FAIL":
			status = TestFailed
		case "SKIP":
			status = TestSkipped
		}
		path := strings.Split(m[2], "/")
		p.buffer = append(p.buffer, GoTestEvent{Kind: "result", Path: path, Status: status, DurationMs: parseDurationPlain(m[3])})
		p.current = path
		if len(path) == 1 {
			switch status {
			case TestPassed:
				p.totals.passed++
			case TestFailed:
				p.totals.failed++
			case TestSkipped:
				p.totals.skipped++
			}
		}
		return nil
	}
	if m := goPkgRe.FindStringSubmatch(line); m != nil && m[1] != "---" {
		pkg := m[2]
		events = p.flush(pkg)
		hasResult := false
		for _, e := range events {
			if e.Kind == "result" {
				hasResult = true
				break
			}
		}
		// Without -v a passing package prints nothing per test: show the
		// package itself.
		if !hasResult {
			status := TestFailed
			if m[1] == "ok" {
				status = TestPassed
			}
			events = append(events, GoTestEvent{Kind: "result", Path: []string{pkg}, Status: status, DurationMs: parseDurationPlain(m[3])})
		}
		passed, failed, skipped := p.totals.passed, p.totals.failed, p.totals.skipped
		events = append(events, GoTestEvent{Kind: "summary", Passed: &passed, Failed: &failed, Skipped: &skipped})
		return events
	}
	if p.current != nil && strings.HasPrefix(line, " ") {
		p.buffer = append(p.buffer, GoTestEvent{Kind: "output", Path: p.current, Line: line})
		key := strings.Join(p.current, "/")
		if loc := goLocationRe.FindStringSubmatch(line); loc != nil && !p.located[key] {
			p.located[key] = true
			lineNo, _ := strconv.Atoi(loc[2])
			p.buffer = append(p.buffer, GoTestEvent{Kind: "location", Path: p.current,
				Location: &TestLocation{Path: loc[1], Line: lineNo, Resolve: "name"}})
		}
	}
	return events
}

func (p *plainGoParser) end() []GoTestEvent { return p.flush("") }

// goJSONRecord is one validated `go test -json` record (go.ts GoEvent).
type goJSONRecord struct {
	Action     string
	Package    string
	Test       *string
	Output     *string
	Elapsed    *float64
	ImportPath string
}

var goJSONActions = map[string]bool{
	"start": true, "run": true, "pause": true, "cont": true, "output": true,
	"pass": true, "bench": true, "fail": true, "skip": true,
	"build-output": true, "build-fail": true,
}

func goJSONEvent(line string) *goJSONRecord {
	if !strings.HasPrefix(line, "{") {
		return nil
	}
	var raw struct {
		Action     *string  `json:"Action"`
		Package    *string  `json:"Package"`
		ImportPath *string  `json:"ImportPath"`
		Test       *string  `json:"Test"`
		Output     *string  `json:"Output"`
		Elapsed    *float64 `json:"Elapsed"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil
	}
	if raw.Action == nil {
		return nil
	}
	pkg := ""
	if raw.Package != nil {
		pkg = *raw.Package
	} else if raw.ImportPath != nil {
		pkg = *raw.ImportPath
	}
	if pkg == "" || !goJSONActions[*raw.Action] {
		return nil
	}
	e := &goJSONRecord{Action: *raw.Action, Package: pkg}
	if raw.Test != nil {
		e.Test = raw.Test
	}
	if raw.Output != nil {
		e.Output = raw.Output
	}
	if raw.Elapsed != nil {
		e.Elapsed = raw.Elapsed
	}
	return e
}

// goPackageState tracks one package's tests and benchmark measurements.
type goPackageState struct {
	tests      bool
	failed     bool
	benchmarks map[string]bool
}

// GoParser is the port of go.ts goParser: JSON records when `-json` is on,
// plain output otherwise.
type GoParser struct {
	plain    *plainGoParser
	located  map[string]bool
	finished map[string]bool
	packages map[string]*goPackageState
	totals   struct{ passed, failed, skipped int }
}

// NewGoParser creates the go test output parser.
func NewGoParser() *GoParser {
	return &GoParser{
		plain:    newPlainGoParser(),
		located:  map[string]bool{},
		finished: map[string]bool{},
		packages: map[string]*goPackageState{},
	}
}

func benchmarkName(pkg *goPackageState, name string) string {
	if pkg.benchmarks[name] {
		return name
	}
	withoutCpu := regexp.MustCompile(`-\d+$`).ReplaceAllString(name, "")
	if pkg.benchmarks[withoutCpu] {
		return withoutCpu
	}
	return name
}

func (p *GoParser) finish(pkg *goPackageState, path []string, status TestStatus, durationMs *float64) []TestEvent {
	key := strings.Join(path, "\x00")
	if p.finished[key] {
		return nil
	}
	p.finished[key] = true
	if len(path) == 2 {
		switch status {
		case TestPassed:
			p.totals.passed++
		case TestFailed:
			p.totals.failed++
			pkg.failed = true
		case TestSkipped:
			p.totals.skipped++
		}
	} else if len(path) == 1 && (!pkg.tests || (status == TestFailed && !pkg.failed)) {
		switch status {
		case TestPassed:
			p.totals.passed++
		case TestFailed:
			p.totals.failed++
		case TestSkipped:
			p.totals.skipped++
		}
	}
	return []TestEvent{{Kind: "result", Path: path, Status: status, DurationMs: durationMs}}
}

// Feed parses one output line.
func (p *GoParser) Feed(line string) []TestEvent {
	e := goJSONEvent(line)
	if e == nil {
		return toTestEvents(p.plain.feed(line))
	}
	pkg, ok := p.packages[e.Package]
	if !ok {
		pkg = &goPackageState{benchmarks: map[string]bool{}}
		p.packages[e.Package] = pkg
	}
	benchmark := ""
	if e.Test != nil && strings.HasPrefix(*e.Test, "Benchmark") {
		benchmark = *e.Test
	}
	var name *string
	if e.Test != nil {
		n := *e.Test
		if benchmark != "" && e.Action != "run" {
			n = benchmarkName(pkg, benchmark)
		}
		name = &n
	}
	path := []string{e.Package}
	hasName := name != nil && *name != ""
	if hasName {
		path = append(path, strings.Split(*name, "/")...)
		pkg.tests = true
	}
	switch e.Action {
	case "run":
		delete(p.finished, strings.Join(path, "\x00"))
		if benchmark != "" {
			pkg.benchmarks[benchmark] = false
		}
		return []TestEvent{{Kind: "start", Path: path}}
	case "bench":
		if name != nil && *name != "" {
			pkg.benchmarks[*name] = true
		}
		return nil
	case "output", "build-output":
		if e.Output == nil || *e.Output == "" {
			return nil
		}
		if e.Action == "output" && (!hasName || benchmark != "") {
			if m := goBenchmarkRe.FindStringSubmatch(*e.Output); m != nil {
				measured := benchmarkName(pkg, m[1])
				if pkg.benchmarks[measured] && (!hasName || *name == measured) {
					pkg.benchmarks[measured] = true
				}
			}
		}
		text := strings.TrimSuffix(*e.Output, "\n")
		var events []TestEvent
		for _, l := range strings.Split(text, "\n") {
			events = append(events, TestEvent{Kind: "output", Path: path, Line: l})
		}
		key := strings.Join(path, "\x00")
		for _, l := range strings.Split(text, "\n") {
			if loc := goJSONLocation.FindStringSubmatch(l); loc != nil && !p.located[key] {
				p.located[key] = true
				lineNo, _ := strconv.Atoi(loc[2])
				resolve := "name"
				if strings.HasPrefix(loc[1], "/") {
					resolve = "path"
				}
				location := &TestLocation{Path: loc[1], Line: lineNo, Resolve: resolve}
				if loc[3] != "" {
					col, _ := strconv.Atoi(loc[3])
					location.Col = col
				}
				events = append(events, TestEvent{Kind: "location", Path: path, Location: location})
				break
			}
		}
		return events
	}
	if e.Action != "pass" && e.Action != "fail" && e.Action != "skip" && e.Action != "build-fail" {
		return nil
	}
	status := TestFailed
	switch e.Action {
	case "pass":
		status = TestPassed
	case "skip":
		status = TestSkipped
	}
	var durationMs *float64
	if e.Elapsed != nil && *e.Elapsed >= 0 {
		v := *e.Elapsed * 1000
		durationMs = &v
	}
	var events []TestEvent
	if !hasName {
		// Quiet benchmarks can emit measurements without a terminal event.
		for test, measured := range pkg.benchmarks {
			if measured {
				events = append(events, p.finish(pkg, append([]string{e.Package}, strings.Split(test, "/")...), TestPassed, nil)...)
			}
		}
		pkg.benchmarks = map[string]bool{}
	}
	events = append(events, p.finish(pkg, path, status, durationMs)...)
	if len(events) > 0 {
		passed, failed, skipped := p.totals.passed, p.totals.failed, p.totals.skipped
		events = append(events, TestEvent{Kind: "summary", Passed: &passed, Failed: &failed, Skipped: &skipped})
	}
	return events
}

// End flushes the plain parser's buffer at process exit.
func (p *GoParser) End() []TestEvent { return toTestEvents(p.plain.end()) }

// GoConsoleText is GoTestOutput's display (go.ts): a `-json` record's line
// renders as its Output text, any other line renders verbatim.
func GoConsoleText(line string) string {
	if e := goJSONEvent(line); e != nil {
		if e.Output != nil {
			return *e.Output
		}
		return ""
	}
	return line + "\n"
}

// goBooleanFlags lists the boolean flags of `go test`.
var goBooleanFlags = map[string]bool{
	"-a": true, "-n": true, "-race": true, "-msan": true, "-asan": true,
	"-cover": true, "-linkshared": true, "-modcacherw": true, "-trimpath": true,
	"-v": true, "-work": true, "-x": true, "-json": true, "-c": true, "-i": true,
	"-short": true, "-failfast": true, "-benchmem": true, "-fullpath": true,
	"-buildvcs": true,
}

// GoTestArgs are the parts of a `go test` command line: flags, targets and
// the program arguments after -args (goArguments.ts goTestParts).
type GoTestArgs struct {
	Flags   []string
	Targets []string
	Program []string
}

// GoTestParts splits a `go test` command line into its parts.
func GoTestParts(args []string) GoTestArgs {
	var out GoTestArgs
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-args" || arg == "--" {
			out.Program = append([]string(nil), args[i:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			out.Flags = append(out.Flags, arg)
			normalized := strings.Replace(arg, "-test.", "-", 1)
			if !strings.Contains(arg, "=") && !goBooleanFlags[normalized] && i+1 < len(args) {
				i++
				out.Flags = append(out.Flags, args[i])
			}
		} else {
			out.Targets = append(out.Targets, arg)
		}
	}
	return out
}

// WithoutGoSelection drops the run/skip/bench/fuzz selectors from flags
// (goArguments.ts withoutGoSelection).
func WithoutGoSelection(flags []string) []string {
	var out []string
	program := false
	for i := 0; i < len(flags); i++ {
		arg := flags[i]
		if arg == "--" {
			out = append(out, flags[i:]...)
			break
		}
		if arg == "-args" {
			program = true
		}
		selector := regexp.MustCompile(`^-(?:test\.)?(run|skip|bench|fuzz)(=|$)`)
		if program {
			selector = regexp.MustCompile(`^-test\.(run|skip|bench|fuzz)(=|$)`)
		}
		m := selector.FindStringSubmatch(arg)
		if m != nil {
			if m[2] != "=" {
				i++
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}
