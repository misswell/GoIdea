package adapter

// The port of src/features/run/testing/jvm.ts and teamcity.ts: Maven
// Surefire's failure lines and summaries, Gradle's test logging and its init
// script's TeamCity service messages, folded into TestEvents.

import (
	"regexp"
	"strconv"
	"strings"
)

// serviceMessage extracts one `##teamcity[kind attr='…']` message
// (teamcity.ts).
var teamcityMessageRe = regexp.MustCompile(`^\s*##teamcity\[(\w+)((?:\s+[\w.-]+='(?:[^|']|\|.)*')*)\s*\]\s*$`)
var teamcityAttrRe = regexp.MustCompile(`([\w.-]+)='((?:[^|']|\|.)*)'`)

// teamcityUnescape decodes TeamCity escapes: `|n` `|r` `|'` `|[` `|]` `||`
// and `|0xNNNN`.
func teamcityUnescape(value string) string {
	out := strings.Builder{}
	for i := 0; i < len(value); i++ {
		if value[i] != '|' || i+1 >= len(value) {
			out.WriteByte(value[i])
			continue
		}
		i++
		c := value[i]
		if strings.HasPrefix(value[i:], "0x") && i+4 < len(value)+1 && len(value[i:]) >= 6 {
			if v, err := strconv.ParseInt(value[i : i+6][2:], 16, 32); err == nil {
				out.WriteRune(rune(v))
				i += 5
				continue
			}
		}
		switch c {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

// ServiceMessage is one TeamCity service message (teamcity.ts).
type ServiceMessage struct {
	Kind  string
	Attrs map[string]string
}

// ServiceMessageOf returns the service message on line, or nil.
func ServiceMessageOf(line string) *ServiceMessage {
	m := teamcityMessageRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	attrs := map[string]string{}
	for _, a := range teamcityAttrRe.FindAllStringSubmatch(m[2], -1) {
		attrs[a[1]] = teamcityUnescape(a[2])
	}
	return &ServiceMessage{Kind: m[1], Attrs: attrs}
}

// The jvm.ts line patterns.
var (
	jvmGradleRe = regexp.MustCompile(`^(\S[^>]*?(?:\s>\s[^>]+?)+)\s+(PASSED|FAILED|SKIPPED)(?:\s+\(([\d.]+\s*m?s)\))?$`)
	// `12 tests completed, 2 failed, 1 skipped`
	jvmGradleSummaryRe = regexp.MustCompile(`^(\d+) tests? completed(?:, (\d+) failed)?(?:, (\d+) skipped)?`)
	// Stack frames of the project's code.
	jvmFrameRe = regexp.MustCompile(`(?:([\w$]+(?:\.[\w$]+)*)\.[\w$<>-]+)?\(([\w$.-]+\.(?:java|kt|kts|scala|groovy)):(\d+)\)|\bat ([\w$.-]+\.(?:java|kt|kts|scala|groovy)):(\d+)`)
	// `[ERROR] com.acme.OrderTest.shouldX -- Time elapsed: 0.01 s <<< FAILURE!   (Surefire 3)`
	jvmSurefire3Re = regexp.MustCompile(`^(?:\[\w+\]\s+)?([\w$.]+)\.([\w$]+)(?:\(\))?\s+--\s+Time elapsed: ([\d.]+ s(?:ec)?)\s+<<<\s+(FAILURE|ERROR)!`)
	// `shouldX(com.acme.OrderTest)  Time elapsed: 0.01 sec  <<< FAILURE!          (Surefire 2)`
	jvmSurefire2Re     = regexp.MustCompile(`^(?:\[\w+\]\s+)?([\w$]+)\(([\w$.]+)\)\s+Time elapsed: ([\d.]+ s(?:ec)?)\s+<<<\s+(FAILURE|ERROR)!`)
	jvmSurefireClassRe = regexp.MustCompile(`^(?:\[\w+\]\s+)?Tests run: (\d+), Failures: (\d+), Errors: (\d+), Skipped: (\d+), Time elapsed: ([\d.]+ s(?:ec)?).*?(?:-- in |- in )([\w$.]+)\s*$`)
	jvmSurefireTotalRe = regexp.MustCompile(`^(?:\[\w+\]\s+)?Tests run: (\d+), Failures: (\d+), Errors: (\d+), Skipped: (\d+)\s*$`)
	// `at org.junit…`, also behind a class loader or module.
	jvmLibraryFrameRe = regexp.MustCompile(`^\s*at\s+(?:\S*\/)?(?:org\.junit|junit\.|org\.opentest4j|org\.testng|kotlin\.|jdk\.internal|java\.|javax\.|sun\.|org\.gradle)`)
)

// jvmFrameLocation is the source position of a stack frame in the project's
// code, nil for library frames (jvm.ts frameLocation).
func jvmFrameLocation(line string) *TestLocation {
	f := jvmFrameRe.FindStringSubmatch(line)
	if f == nil || jvmLibraryFrameRe.MatchString(line) {
		return nil
	}
	if f[2] != "" {
		lineNo, _ := strconv.Atoi(f[3])
		return &TestLocation{Path: f[2], Line: lineNo, Resolve: "name", ClassName: f[1]}
	}
	lineNo, _ := strconv.Atoi(f[5])
	return &TestLocation{Path: f[4], Line: lineNo, Resolve: "name"}
}

// JvmParser folds Maven Surefire / Gradle test output into events
// (jvm.ts jvmParser).
type JvmParser struct {
	failure []string
	located bool
	// The init script reports every test: Gradle's own test logging would
	// repeat them by simple class name.
	reported bool
	outcomes map[string]string // "failed" | "skipped"
}

// NewJvmParser creates the JVM test output parser.
func NewJvmParser() *JvmParser {
	return &JvmParser{outcomes: map[string]string{}}
}

func (p *JvmParser) startFailure(path []string) {
	p.failure = append([]string(nil), path...)
	p.located = false
}

// jvmInitScriptEvents turns the init script's service messages into events
// (jvm.ts initScriptEvents): tests are [class, method], a parameterized
// method's invocations share its node.
func (p *JvmParser) jvmInitScriptEvents(msg *ServiceMessage) []TestEvent {
	if msg.Attrs["className"] == "" || msg.Attrs["name"] == "" {
		return nil
	}
	name := strings.TrimSpace(strings.SplitN(msg.Attrs["name"], "(", 2)[0])
	if name == "" {
		name = msg.Attrs["name"]
	}
	path := []string{msg.Attrs["className"], name}
	key := strings.Join(path, "\x00")
	failed := p.outcomes[key] == "failed"
	switch msg.Kind {
	case "testStarted":
		if !failed {
			delete(p.outcomes, key)
		}
		return []TestEvent{{Kind: "start", Path: path}}
	case "testStdOut", "testStdErr":
		text := strings.TrimRight(msg.Attrs["out"], "\r\n")
		if text == "" {
			return nil
		}
		var events []TestEvent
		for _, line := range strings.Split(text, "\r\n") {
			for _, l := range strings.Split(line, "\n") {
				events = append(events, TestEvent{Kind: "output", Path: path, Line: l})
			}
		}
		return events
	case "testFailed":
		p.outcomes[key] = "failed"
		details := msg.Attrs["details"]
		if details == "" {
			details = msg.Attrs["message"]
		}
		lines := strings.Split(details, "\n")
		var events []TestEvent
		var location *TestLocation
		for _, line := range lines {
			events = append(events, TestEvent{Kind: "output", Path: path, Line: line})
			if location == nil {
				location = jvmFrameLocation(line)
			}
		}
		if location != nil {
			events = append(events, TestEvent{Kind: "location", Path: path, Location: location})
		}
		return events
	case "testIgnored":
		if !failed {
			p.outcomes[key] = "skipped"
		}
		return nil
	case "testFinished":
		status := p.outcomes[key]
		if status == "" {
			status = "passed"
		}
		event := TestEvent{Kind: "result", Path: path, Status: TestStatus(status)}
		if d := msg.Attrs["duration"]; d != "" {
			if v, err := strconv.ParseFloat(strings.TrimSuffix(d, "ms"), 64); err == nil {
				event.DurationMs = &v
			}
		}
		return []TestEvent{event}
	}
	return nil
}

// Feed turns one output line into events (jvm.ts feed).
func (p *JvmParser) Feed(line string) []TestEvent {
	var events []TestEvent
	if msg := ServiceMessageOf(line); msg != nil {
		p.reported = true
		p.failure = nil
		return p.jvmInitScriptEvents(msg)
	}
	trimmed := strings.TrimSpace(line)
	if m := jvmGradleRe.FindStringSubmatch(trimmed); m != nil && !strings.HasPrefix(line, " ") {
		if p.reported {
			p.failure = nil
			return events
		}
		// `method()` is a JUnit 5 display name; the reports name the method.
		parts := strings.Split(m[1], " > ")
		for i, part := range parts {
			if i == len(parts)-1 {
				parts[i] = strings.TrimSuffix(part, "()")
			}
		}
		status := TestPassed
		switch m[2] {
		case "FAILED":
			status = TestFailed
		case "SKIPPED":
			status = TestSkipped
		}
		events = append(events, TestEvent{Kind: "result", Path: parts, Status: status, DurationMs: ParseDuration(m[3])})
		if status == TestFailed {
			p.startFailure(parts)
		} else {
			p.failure = nil
		}
		return events
	}
	if m := jvmGradleSummaryRe.FindStringSubmatch(line); m != nil {
		failed, _ := strconv.Atoi(orZero(m[2]))
		skipped, _ := strconv.Atoi(orZero(m[3]))
		total, _ := strconv.Atoi(m[1])
		passed := total - failed - skipped
		events = append(events, intSummaryEvent(passed, failed, skipped))
		p.failure = nil
		return events
	}
	surefire3 := jvmSurefire3Re.FindStringSubmatch(line)
	var m2 []string
	if surefire3 != nil {
		m2 = surefire3
	} else {
		m2 = jvmSurefire2Re.FindStringSubmatch(line)
	}
	if m2 != nil {
		cls, method := m2[1], m2[2]
		if surefire3 == nil {
			cls, method = m2[2], m2[1]
		}
		path := []string{cls, method}
		events = append(events, TestEvent{Kind: "result", Path: path, Status: TestFailed, DurationMs: ParseDuration(m2[3])})
		p.startFailure(path)
		return events
	}
	if m := jvmSurefireClassRe.FindStringSubmatch(line); m != nil {
		failures, _ := strconv.Atoi(m[2])
		errors, _ := strconv.Atoi(m[3])
		run, _ := strconv.Atoi(m[1])
		skipped, _ := strconv.Atoi(m[4])
		status := TestPassed
		if failures+errors > 0 {
			status = TestFailed
		} else if skipped == run {
			status = TestSkipped
		}
		events = append(events, TestEvent{Kind: "result", Path: []string{m[6]}, Status: status, DurationMs: ParseDuration(m[5])})
		p.failure = nil
		return events
	}
	if m := jvmSurefireTotalRe.FindStringSubmatch(line); m != nil {
		failures, _ := strconv.Atoi(m[2])
		errors, _ := strconv.Atoi(m[3])
		run, _ := strconv.Atoi(m[1])
		skipped, _ := strconv.Atoi(m[4])
		passed := run - failures - errors - skipped
		events = append(events, intSummaryEvent(passed, failures+errors, skipped))
		p.failure = nil
		return events
	}
	if p.failure != nil {
		if strings.TrimSpace(line) == "" || jvmInfoWarningRe.MatchString(line) {
			if strings.TrimSpace(line) == "" {
				return events
			}
			p.failure = nil
			return events
		}
		events = append(events, TestEvent{Kind: "output", Path: p.failure, Line: line})
		if !p.located {
			if location := jvmFrameLocation(line); location != nil {
				p.located = true
				events = append(events, TestEvent{Kind: "location", Path: p.failure, Location: location})
			}
		}
	}
	return events
}

var jvmInfoWarningRe = regexp.MustCompile(`^\[(INFO|WARNING)\]`)

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func intSummaryEvent(passed, failed, skipped int) TestEvent {
	return TestEvent{Kind: "summary", Passed: &passed, Failed: &failed, Skipped: &skipped}
}

// MoveToReportedClasses moves Gradle's console results (simple class names)
// to the qualified classes the XML reports name (jvm.ts).
func MoveToReportedClasses(tests *TestResults, classNames []string) {
	bySimpleName := map[string]*string{}
	for _, c := range classNames {
		simple := c[strings.LastIndex(c, ".")+1:]
		if simple == c {
			continue
		}
		if existing, ok := bySimpleName[simple]; ok {
			var nilPtr *string
			bySimpleName[simple] = nilPtr
			_ = existing
		} else {
			cc := c
			bySimpleName[simple] = &cc
		}
	}
	for _, root := range append([]*TestNode(nil), tests.Roots...) {
		if qualified := bySimpleName[root.Name]; qualified != nil {
			tests.Move([]string{root.Name}, []string{*qualified})
		}
	}
}
