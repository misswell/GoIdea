package adapter

// Minitest and RSpec reporting, ported from
// src/features/run/testing/ruby.ts.

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"mayoi/internal/openidea"
)

const rubyRecordMark = "@@OPENIDEA@@"

var (
	rubyResultRe     = regexp.MustCompile(`^(.+?)#(\S+) = ([\d.]+ s) = ([.FES])$`)
	rubyStartedRe    = regexp.MustCompile(`^(.+?)#(\S+) = (.*)$`)
	rubyLateResultRe = regexp.MustCompile(`^([\d.]+ s) = ([.FES])$`)
	rubySectionRe    = regexp.MustCompile(`^\s*\d+\) (Failure|Error|Skipped):$`)
	rubySectionTest  = regexp.MustCompile(`^(.+?)#(\S+?)(?: \[(.+?):(\d+)\])?:$`)
	rubyFrameRe      = regexp.MustCompile(`^\s+(\S+?):(\d+):in `)
	rubySummaryRe    = regexp.MustCompile(`^(\d+) runs, \d+ assertions, (\d+) failures, (\d+) errors, (\d+) skips`)
	rubyFinishedRe   = regexp.MustCompile(`^Finished in ([\d.]+s)`)
	rubyLibraryRe    = regexp.MustCompile(`[/\\](?:gems|rubygems|lib[/\\]ruby)[/\\]`)
)

func rubyStatus(mark string) TestStatus {
	switch mark {
	case ".":
		return TestPassed
	case "S":
		return TestSkipped
	default:
		return TestFailed
	}
}

func rubyLocation(path []string) *TestLocation {
	if len(path) < 2 {
		return nil
	}
	return &TestLocation{Path: path[1], ClassName: path[0], Resolve: "rubyTest"}
}

type rubyRecord struct {
	Type     string   `json:"t"`
	Class    string   `json:"c"`
	Name     string   `json:"n"`
	Status   string   `json:"s"`
	Duration *float64 `json:"d"`
	File     string   `json:"f"`
	Line     int      `json:"l"`
	Message  string   `json:"m"`
	Runs     *int     `json:"runs"`
	Failures *int     `json:"failures"`
	Errors   *int     `json:"errors"`
	Skips    *int     `json:"skips"`
}

func rubyRecordEvents(payload string) []TestEvent {
	var record rubyRecord
	if json.Unmarshal([]byte(payload), &record) != nil {
		return nil
	}
	if record.Type == "summary" {
		runs, failures, errors, skips := 0, 0, 0, 0
		if record.Runs != nil {
			runs = *record.Runs
		}
		if record.Failures != nil {
			failures = *record.Failures
		}
		if record.Errors != nil {
			errors = *record.Errors
		}
		if record.Skips != nil {
			skips = *record.Skips
		}
		failed := failures + errors
		passed := max(0, runs-failed-skips)
		event := TestEvent{Kind: "summary", Passed: &passed, Failed: &failed, Skipped: &skips}
		if record.Duration != nil {
			ms := math.Round(*record.Duration*1000*1000) / 1000
			event.DurationMs = &ms
		}
		return []TestEvent{event}
	}
	if record.Class == "" || record.Name == "" {
		return nil
	}
	path := []string{record.Class, record.Name}
	var events []TestEvent
	switch record.Type {
	case "start":
		events = append(events, TestEvent{Kind: "start", Path: path})
	case "result":
		if record.Status != "" {
			event := TestEvent{Kind: "result", Path: path, Status: TestStatus(record.Status)}
			if record.Duration != nil {
				ms := math.Round(*record.Duration*1000*1000) / 1000
				event.DurationMs = &ms
			}
			events = append(events, event)
			for _, line := range strings.Split(record.Message, "\n") {
				if line != "" {
					events = append(events, TestEvent{Kind: "output", Path: path, Line: line})
				}
			}
		}
	}
	if record.File != "" && record.Line > 0 {
		events = append(events, TestEvent{
			Kind: "location",
			Path: path,
			Location: &TestLocation{
				Path: record.File, Line: record.Line, Resolve: "path",
			},
		})
	}
	return events
}

// RubyConsoleText hides the IDE reporter's machine-readable record from the
// console while keeping any progress text printed before it. A line with a
// record deliberately keeps its original newline in the record, so the text
// before the marker concatenates with the next progress character.
func RubyConsoleText(line string) string {
	if at := strings.Index(line, rubyRecordMark); at >= 0 {
		return line[:at]
	}
	return line + "\n"
}

type rubySection struct {
	path       []string
	located    bool
	expectTest bool
}

// RubyParser folds Minitest, RSpec and IDE reporter output into events.
type RubyParser struct {
	records bool
	seen    map[string]bool
	pending []string
	section *rubySection
	finish  *float64
}

// NewRubyParser creates the Ruby test output parser.
func NewRubyParser() *RubyParser {
	return &RubyParser{seen: map[string]bool{}}
}

func (p *RubyParser) locate(path []string) []TestEvent {
	key := strings.Join(path, "\x00")
	if p.seen[key] {
		return nil
	}
	p.seen[key] = true
	location := rubyLocation(path)
	if location == nil {
		return nil
	}
	return []TestEvent{{Kind: "location", Path: path, Location: location}}
}

// Feed turns one output line into test events.
func (p *RubyParser) Feed(line string) []TestEvent {
	if at := strings.Index(line, rubyRecordMark); at >= 0 {
		p.records = true
		return rubyRecordEvents(line[at+len(rubyRecordMark):])
	}
	if p.records {
		return nil
	}
	if m := rubySummaryRe.FindStringSubmatch(line); m != nil {
		p.section = nil
		runs, _ := strconv.Atoi(m[1])
		failures, _ := strconv.Atoi(m[2])
		errors, _ := strconv.Atoi(m[3])
		skips, _ := strconv.Atoi(m[4])
		failed := failures + errors
		passed := max(0, runs-failed-skips)
		event := TestEvent{Kind: "summary", Passed: &passed, Failed: &failed, Skipped: &skips}
		event.DurationMs = p.finish
		return []TestEvent{event}
	}
	if m := rubyFinishedRe.FindStringSubmatch(line); m != nil {
		p.finish = ParseDuration(m[1])
		p.pending = nil
		return nil
	}
	if rubySectionRe.MatchString(line) {
		p.section = &rubySection{expectTest: true}
		return nil
	}
	if p.section != nil {
		if p.section.expectTest {
			p.section.expectTest = false
			if m := rubySectionTest.FindStringSubmatch(line); m != nil {
				p.section.path = []string{m[1], m[2]}
				events := []TestEvent{{Kind: "output", Path: p.section.path, Line: line}}
				if m[3] != "" {
					lineNo, _ := strconv.Atoi(m[4])
					p.section.located = true
					events = append(events, TestEvent{Kind: "location", Path: p.section.path,
						Location: &TestLocation{Path: m[3], Line: lineNo, Resolve: "path"}})
				}
				return events
			}
		}
		if len(p.section.path) > 0 {
			events := []TestEvent{{Kind: "output", Path: p.section.path, Line: line}}
			if m := rubyFrameRe.FindStringSubmatch(line); m != nil && !p.section.located &&
				!rubyLibraryRe.MatchString(m[1]) {
				lineNo, _ := strconv.Atoi(m[2])
				p.section.located = true
				events = append(events, TestEvent{Kind: "location", Path: p.section.path,
					Location: &TestLocation{Path: m[1], Line: lineNo, Resolve: "path"}})
			}
			return events
		}
		return nil
	}
	if m := rubyResultRe.FindStringSubmatch(line); m != nil {
		path := []string{m[1], m[2]}
		p.pending = nil
		events := []TestEvent{{Kind: "result", Path: path, Status: rubyStatus(m[4]), DurationMs: ParseDuration(m[3])}}
		return append(events, p.locate(path)...)
	}
	if p.pending != nil {
		if m := rubyLateResultRe.FindStringSubmatch(line); m != nil {
			path := p.pending
			p.pending = nil
			return []TestEvent{{Kind: "result", Path: path, Status: rubyStatus(m[2]), DurationMs: ParseDuration(m[1])}}
		}
	}
	if m := rubyStartedRe.FindStringSubmatch(line); m != nil &&
		(regexp.MustCompile(`^(test|\w*_\d+)`).MatchString(m[2])) {
		p.pending = []string{m[1], m[2]}
		events := []TestEvent{{Kind: "start", Path: p.pending}}
		events = append(events, p.locate(p.pending)...)
		if m[3] != "" {
			events = append(events, TestEvent{Kind: "output", Path: p.pending, Line: m[3]})
		}
		return events
	}
	if p.pending != nil {
		return []TestEvent{{Kind: "output", Path: p.pending, Line: line}}
	}
	return nil
}

// RubyVerboseArgs adds verbose output to Minitest and RSpec where upstream
// does, while leaving an explicit verbosity choice intact.
func RubyVerboseArgs(config *openidea.RunConfig) []string {
	args := append([]string(nil), config.Args...)
	if config.Kind == "rspec" {
		return args
	}
	if config.Kind == "rake" {
		for i, arg := range args {
			if !strings.HasPrefix(arg, "TESTOPTS=") {
				continue
			}
			if regexp.MustCompile(`(^|\s)(-v|--verbose)(\s|$)`).MatchString(arg) {
				return args
			}
			args[i] = arg + " -v"
			return args
		}
		return append(args, "TESTOPTS=-v")
	}
	for _, arg := range args {
		if arg == "-v" || arg == "--verbose" {
			return args
		}
	}
	return append(args, "-v")
}
