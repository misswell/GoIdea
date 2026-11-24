package app

import (
	"fmt"
	"os"
	"strings"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func (m *Model) initChildTests(child *runChild, dotnetReports *adapter.DotnetReports) {
	if child == nil {
		return
	}
	child.tests = nil
	switch adapter.FrameworkFor(&child.config) {
	case "jvm":
		child.tests = adapter.NewTestResults()
		child.testParser = adapter.NewJvmParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "go":
		child.tests = adapter.NewTestResults()
		child.testGo = adapter.NewGoParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "cargo":
		child.tests = adapter.NewTestResults()
		child.testCargo = adapter.NewCargoParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "pytest":
		child.tests = adapter.NewTestResults()
		child.testPytest = adapter.NewPytestParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "unittest":
		child.tests = adapter.NewTestResults()
		child.testUnittest = adapter.NewUnittestParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "js":
		child.tests = adapter.NewTestResults()
		child.testJs = adapter.NewJsParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "phpunit":
		child.tests = adapter.NewTestResults()
		child.testPHPUnit = adapter.NewPHPUnitParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "ruby":
		child.tests = adapter.NewTestResults()
		child.testRuby = adapter.NewRubyParser()
		child.testStdout = adapter.NewTestStdoutBuffer()
	case "dotnet":
		if dotnetReports == nil {
			dotnetReports = adapter.NewDotnetReports(&child.config)
		}
		child.tests = adapter.NewTestResults()
		child.testDotnet = dotnetReports
		child.testStdout = adapter.NewTestStdoutBuffer()
	}
}

func (m *Model) feedChildTests(child *runChild, text string) string {
	if child == nil || child.testStdout == nil || child.tests == nil {
		return text
	}
	return m.foldChildTestLines(child, child.testStdout.Feed(stripANSI(text)), text)
}

func (m *Model) flushChildTests(child *runChild) string {
	if child == nil || child.testStdout == nil || child.tests == nil {
		return ""
	}
	lines := child.testStdout.End()
	return m.foldChildTestLines(child, lines, strings.Join(lines, ""))
}

func (m *Model) foldChildTestLines(child *runChild, lines []string, fallback string) string {
	var display []string
	for _, line := range lines {
		var events []adapter.TestEvent
		if child.testGo != nil {
			events = child.testGo.Feed(line)
			display = append(display, adapter.GoConsoleText(line))
		} else if child.testCargo != nil {
			events = child.testCargo.Feed(line)
		} else if child.testPytest != nil {
			events = child.testPytest.Feed(line)
		} else if child.testUnittest != nil {
			events = child.testUnittest.Feed(line)
		} else if child.testJs != nil {
			events = child.testJs.Feed(line)
		} else if child.testPHPUnit != nil {
			events = child.testPHPUnit.Feed(line)
			if adapter.ServiceMessageOf(line) == nil {
				display = append(display, line+"\n")
			}
		} else if child.testRuby != nil {
			events = child.testRuby.Feed(line)
			display = append(display, adapter.RubyConsoleText(line))
		} else if child.testDotnet != nil {
			child.testDotnet.Feed(line)
		} else if child.testParser != nil {
			events = child.testParser.Feed(line)
		}
		child.tests.Apply(events)
	}
	if child.testGo != nil || child.testPHPUnit != nil || child.testRuby != nil {
		return strings.Join(display, "")
	}
	return fallback
}

func (m *Model) finishChildTests(child *runChild, code int) {
	if child == nil || child.tests == nil {
		return
	}
	if display := m.flushChildTests(child); display != "" {
		child.append(openidea.OutputChunk{Kind: "stdout", Text: display})
	}
	if child.testUnittest != nil {
		child.tests.Apply(child.testUnittest.End())
	}
	child.tests.Finish(code)
	reports := child.testDotnet
	if reports == nil {
		return
	}
	root := m.Root()
	go func() {
		events, err := reports.ReadReports(func(path string) (string, error) {
			data, readErr := os.ReadFile(path)
			return string(data), readErr
		})
		m.post(func() {
			if m.Root() != root || m.childByChannel(child.channel) != child || child.tests == nil {
				return
			}
			child.tests.Apply(events)
			if err != nil {
				m.notifyWarning("run.dotnet-report", "Could not read .NET test results", notifySuffix(err.Error()))
			}
			m.invalidate()
		})
	}()
}

func childTreeLines(child *runChild) []testTreeLine {
	if child == nil || child.tests == nil || child.tests.IsEmpty() {
		return nil
	}
	mark := func(node *adapter.TestNode) string {
		switch adapter.TestNodeStatusOf(node) {
		case adapter.TestPassed:
			return "✓"
		case adapter.TestFailed:
			return "✗"
		case adapter.TestSkipped:
			return "⊘"
		case adapter.TestRunning:
			return "⟳"
		default:
			return "·"
		}
	}
	var out []testTreeLine
	var walk func(*adapter.TestNode, int)
	walk = func(node *adapter.TestNode, depth int) {
		text := strings.Repeat("    ", depth) + node.Name
		if node.DurationMs != nil {
			text += fmt.Sprintf(" (%.1f ms)", *node.DurationMs)
		}
		out = append(out, testTreeLine{
			id: node.ID, mark: mark(node), text: text, loc: node.Location,
		})
		for _, next := range node.Children {
			walk(next, depth+1)
		}
	}
	for _, root := range child.tests.Roots {
		walk(root, 0)
	}
	return out
}
