package adapter

// PHPUnit and Pest test reporting, ported from
// src/features/run/testing/phpunit.ts. Both emit TeamCity service messages
// when the launch adds --teamcity.

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	phpUnitHintRe = regexp.MustCompile(`^(?:php|pest)_qn://(.+?\.php)(?:::|$)`)
	phpUnitFileRe = regexp.MustCompile(`(?:^|\s)((?:[A-Za-z]:)?[^\s:]+\.php):(\d+)\s*$`)
	phpUnitVendor = regexp.MustCompile(`[/\\]vendor[/\\]`)
	phpUnitRunner = regexp.MustCompile(`(^|[/\\])(phpunit|pest)(\.phar|\.bat)?$`)
)

// PHPUnitHintFile extracts the source file from PHPUnit's locationHint.
func PHPUnitHintFile(hint string) string {
	if m := phpUnitHintRe.FindStringSubmatch(hint); m != nil {
		return m[1]
	}
	return ""
}

// PHPUnitFailureLocation finds the first project PHP stack frame in details.
func PHPUnitFailureLocation(details string) *TestLocation {
	for _, line := range strings.Split(details, "\n") {
		if m := phpUnitFileRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil &&
			!phpUnitVendor.MatchString(m[1]) {
			lineNo, _ := strconv.Atoi(m[2])
			return &TestLocation{Path: m[1], Line: lineNo, Resolve: "path"}
		}
	}
	return nil
}

type phpUnitOpenTest struct {
	path []string
	file string
	done bool
}

// PHPUnitParser folds PHPUnit/Pest TeamCity service messages into events.
type PHPUnitParser struct {
	suites []string
	open   map[string]phpUnitOpenTest
}

// NewPHPUnitParser creates the PHPUnit output parser.
func NewPHPUnitParser() *PHPUnitParser {
	return &PHPUnitParser{open: map[string]phpUnitOpenTest{}}
}

func (p *PHPUnitParser) suitePath() []string {
	out := make([]string, 0, len(p.suites))
	for _, suite := range p.suites {
		if at := strings.LastIndex(suite, "::"); at > 0 {
			out = append(out, suite[at+2:])
		} else {
			out = append(out, suite)
		}
	}
	return out
}

// Feed turns one output line into test events.
func (p *PHPUnitParser) Feed(line string) []TestEvent {
	message := ServiceMessageOf(line)
	if message == nil {
		return nil
	}
	name := message.Attrs["name"]
	switch message.Kind {
	case "testSuiteStarted":
		p.suites = append(p.suites, name)
	case "testSuiteFinished":
		if len(p.suites) > 0 && p.suites[len(p.suites)-1] == name {
			p.suites = p.suites[:len(p.suites)-1]
		}
	case "testStarted":
		path := append(p.suitePath(), name)
		p.open[name] = phpUnitOpenTest{
			path: path,
			file: PHPUnitHintFile(message.Attrs["locationHint"]),
		}
		return []TestEvent{{Kind: "start", Path: path}}
	case "testFailed", "testIgnored":
		test, ok := p.open[name]
		if !ok {
			return nil
		}
		details := message.Attrs["details"]
		text := message.Attrs["message"]
		if details != "" {
			if text != "" {
				text += "\n"
			}
			text += details
		}
		events := make([]TestEvent, 0, strings.Count(text, "\n")+3)
		for _, output := range strings.Split(text, "\n") {
			events = append(events, TestEvent{Kind: "output", Path: test.path, Line: output})
		}
		location := PHPUnitFailureLocation(details)
		if location == nil && test.file != "" {
			location = &TestLocation{Path: test.file, Resolve: "path"}
		}
		if location != nil {
			events = append(events, TestEvent{Kind: "location", Path: test.path, Location: location})
		}
		status := TestFailed
		if message.Kind == "testIgnored" {
			status = TestSkipped
		}
		events = append(events, TestEvent{Kind: "result", Path: test.path, Status: status})
		test.done = true
		p.open[name] = test
		return events
	case "testFinished":
		test, ok := p.open[name]
		if !ok {
			return nil
		}
		delete(p.open, name)
		if test.done {
			return nil
		}
		event := TestEvent{Kind: "result", Path: test.path, Status: TestPassed}
		if duration := message.Attrs["duration"]; duration != "" {
			if ms, err := strconv.ParseFloat(strings.TrimSuffix(duration, "ms"), 64); err == nil {
				event.DurationMs = &ms
			}
		}
		return []TestEvent{event}
	}
	return nil
}

// PHPUnitWithTeamcity adds --teamcity after the runner unless the command
// already selected a result format.
func PHPUnitWithTeamcity(args []string) []string {
	for _, arg := range args {
		if arg == "--teamcity" || strings.HasPrefix(arg, "--testdox") ||
			strings.HasPrefix(arg, "--log-") || strings.HasPrefix(arg, "--no-output") {
			return append([]string(nil), args...)
		}
	}
	at := phpUnitRunnerLength(args)
	out := make([]string, 0, len(args)+1)
	out = append(out, args[:at]...)
	out = append(out, "--teamcity")
	out = append(out, args[at:]...)
	return out
}

func phpUnitRunnerLength(args []string) int {
	if len(args) > 0 && phpUnitRunner.MatchString(args[0]) {
		return 1
	}
	return 0
}

var phpUnitValueOptions = map[string]bool{
	"-c": true, "--configuration": true, "--bootstrap": true, "--filter": true,
	"--testsuite": true, "--group": true, "--exclude-group": true,
	"--covers": true, "--uses": true, "--cache-directory": true,
}

// PHPUnitArgs replaces the target and filter of a PHPUnit launch.
func PHPUnitArgs(args []string, target, filter string, replaceFilter bool) []string {
	runner := phpUnitRunnerLength(args)
	out := append([]string(nil), args[:runner]...)
	var targets []string
	rest := args[runner:]
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case !strings.HasPrefix(arg, "-"):
			targets = append(targets, arg)
		case phpUnitValueOptions[arg] && i+1 < len(rest):
			if !(replaceFilter && arg == "--filter") {
				out = append(out, arg, rest[i+1])
			}
			i++
		case replaceFilter && strings.HasPrefix(arg, "--filter="):
		default:
			out = append(out, arg)
		}
	}
	if filter != "" {
		out = append(out, "--filter", filter)
	}
	if target != "" {
		out = append(out, target)
	} else {
		out = append(out, targets...)
	}
	return out
}

var phpUnitClassName = regexp.MustCompile(`^\\?[A-Z]\w*(?:\\\w+)*$`)
var phpUnitDataSet = regexp.MustCompile(` with data set .*$`)

func phpUnitTestName(name, parent string) bool {
	return regexp.MustCompile(`^[a-z_]\w*$`).MatchString(name) ||
		!phpUnitClassName.MatchString(name) ||
		(phpUnitClassName.MatchString(parent) && strings.Contains(parent, `\`))
}

var phpUnitRegexMeta = regexp.MustCompile(`[\\.+*?()|[\]{}^$]`)

func phpUnitEscape(value string) string {
	return phpUnitRegexMeta.ReplaceAllStringFunc(value, func(s string) string { return `\` + s })
}

// PHPUnitFilter builds PHPUnit's regular expression for failed test paths.
func PHPUnitFilter(paths [][]string) string {
	seen := map[string]bool{}
	var parts []string
	add := func(part string) {
		if !seen[part] {
			seen[part] = true
			parts = append(parts, part)
		}
	}
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		last := path[len(path)-1]
		data := phpUnitDataSet.MatchString(last)
		method := phpUnitDataSet.ReplaceAllString(last, "")
		offset := 2
		if data {
			offset = 3
		}
		var class string
		if len(path) >= offset {
			class = path[len(path)-offset]
		}
		if phpUnitTestName(method, class) {
			pattern := phpUnitEscape(class) + `::` + phpUnitEscape(method)
			pattern += `( with data set .*)?`
			add(pattern)
		} else {
			add(phpUnitEscape(last) + `::.*`)
		}
	}
	return `/(` + strings.Join(parts, "|") + `)$/`
}
