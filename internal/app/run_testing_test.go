package app

import (
	"strings"
	"testing"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func TestRunPaneFoldsPHPUnitOutputWithoutShowingServiceMessages(t *testing.T) {
	m := &Model{}
	m.run.tests = adapter.NewTestResults()
	m.run.testPHPUnit = adapter.NewPHPUnitParser()
	m.run.testStdout = adapter.NewTestStdoutBuffer()
	text := "PHPUnit 11\n" +
		"##teamcity[testSuiteStarted name='CalcTest']\n" +
		"##teamcity[testStarted name='adds']\n" +
		"##teamcity[testFinished name='adds' duration='2']\n" +
		"OK (1 test)\n"
	if got, want := m.feedRunTests(text), "PHPUnit 11\nOK (1 test)\n"; got != want {
		t.Fatalf("console = %q, want %q", got, want)
	}
	counts := m.run.tests.Counts()
	if counts.Passed != 1 || counts.Total != 1 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestRunPaneFlushesAPartialPHPUnitConsoleLine(t *testing.T) {
	m := &Model{}
	m.run.tests = adapter.NewTestResults()
	m.run.testPHPUnit = adapter.NewPHPUnitParser()
	m.run.testStdout = adapter.NewTestStdoutBuffer()
	if got := m.feedRunTests("##teamcity[testStarted name='adds']\nOK (1 test"); got != "" {
		t.Fatalf("complete output = %q", got)
	}
	if got := m.flushRunTests(); got != "OK (1 test\n" {
		t.Fatalf("flushed output = %q", got)
	}
}

func TestRunPaneFoldsRubyReporterAndHidesRecord(t *testing.T) {
	m := &Model{}
	m.run.tests = adapter.NewTestResults()
	m.run.testRuby = adapter.NewRubyParser()
	m.run.testStdout = adapter.NewTestStdoutBuffer()
	text := "# Running:\n" +
		`.@@OPENIDEA@@{"t":"result","c":"MixedTest","n":"passes","s":"passed","d":0.1}` + "\n" +
		"Finished in 0.1s\n"
	if got, want := m.feedRunTests(text), "# Running:\n.Finished in 0.1s\n"; got != want {
		t.Fatalf("console = %q, want %q", got, want)
	}
	if counts := m.run.tests.Counts(); counts.Passed != 1 || counts.Total != 1 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestRunPaneFoldsUnittestOutput(t *testing.T) {
	m := &Model{}
	m.run.tests = adapter.NewTestResults()
	m.run.testUnittest = adapter.NewUnittestParser()
	m.run.testStdout = adapter.NewTestStdoutBuffer()
	text := "test_ok (polls.tests.ViewTests.test_ok) ... ok\n" +
		"Ran 1 test in 0.001s\n" +
		"OK\n"
	if got := m.feedRunTests(text); got != text {
		t.Fatalf("console = %q, want %q", got, text)
	}
	counts := m.run.tests.Counts()
	if counts.Passed != 1 || counts.Total != 1 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestRunPaneCollectsDotnetReportPaths(t *testing.T) {
	cfg := &openidea.RunConfig{
		Kind: "dotnet-test", Command: "dotnet", Args: []string{"test", "Tests.csproj"},
	}
	reports := adapter.NewDotnetReports(cfg)
	m := &Model{}
	m.run.tests = adapter.NewTestResults()
	m.run.testDotnet = reports
	m.run.testStdout = adapter.NewTestStdoutBuffer()
	path := "/tmp/" + reports.Prefix() + "_net9.0.trx"
	m.feedRunTests("Results File: " + path + "\n")
	if got := reports.Paths(); len(got) != 1 || !strings.Contains(got[0], path) {
		t.Fatalf("paths = %v", got)
	}
	if counts := m.run.tests.Counts(); counts.Total != 0 {
		t.Fatalf("a report path alone created test results: %+v", counts)
	}
}
