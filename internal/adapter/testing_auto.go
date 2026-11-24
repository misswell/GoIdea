package adapter

// TestOutputParser is the shared feed contract of OpenIDEA's test parsers.
type TestOutputParser interface {
	Feed(line string) []TestEvent
}

type endingTestOutputParser interface {
	TestOutputParser
	End() []TestEvent
}

// AutoTestParser feeds an unknown tool's output to every built-in parser side
// by side, matching OpenIDEA's combinedParser in testing/index.ts.
type AutoTestParser struct {
	parsers []TestOutputParser
}

// NewAutoTestParser creates the same parser set OpenIDEA uses for `make test`
// and other configurations whose framework cannot be inferred.
func NewAutoTestParser() *AutoTestParser {
	return &AutoTestParser{parsers: []TestOutputParser{
		NewCargoParser(),
		NewJsParser(),
		NewPytestParser(),
		NewUnittestParser(),
		NewPHPUnitParser(),
		NewGoParser(),
		NewJvmParser(),
		NewRubyParser(),
	}}
}

// Feed returns the events from each parser in construction order.
func (p *AutoTestParser) Feed(line string) []TestEvent {
	if p == nil {
		return nil
	}
	var events []TestEvent
	for _, parser := range p.parsers {
		events = append(events, parser.Feed(line)...)
	}
	return events
}

// End flushes the parsers that buffer their final results.
func (p *AutoTestParser) End() []TestEvent {
	if p == nil {
		return nil
	}
	var events []TestEvent
	for _, parser := range p.parsers {
		if ending, ok := parser.(endingTestOutputParser); ok {
			events = append(events, ending.End()...)
		}
	}
	return events
}
