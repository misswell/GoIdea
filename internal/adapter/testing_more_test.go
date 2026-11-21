package adapter

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func feedTestParser(t *testing.T, tree *TestResults, feed func(string) []TestEvent, lines ...string) {
	t.Helper()
	for _, line := range lines {
		tree.Apply(feed(line))
	}
	tree.Finish(0)
}

func TestPHPUnitParserReportsSuitesFailuresAndLocations(t *testing.T) {
	parser := NewPHPUnitParser()
	tree := NewTestResults()
	suite := `App\Tests\CalcTest`
	failureLine := `##teamcity[testFailed name='testSub' message='Expected 2' details=' /app/vendor/phpunit/Assert.php:100|n /app/tests/CalcTest.php:21|n']`
	if message := ServiceMessageOf(failureLine); message == nil || message.Kind != "testFailed" ||
		message.Attrs["name"] != "testSub" {
		t.Fatalf("failure service message = %+v", message)
	}
	feedTestParser(t, tree, parser.Feed,
		`##teamcity[testSuiteStarted name='Unit']`,
		`##teamcity[testSuiteStarted name='App\Tests\CalcTest']`,
		`##teamcity[testStarted name='testAdd']`,
		`##teamcity[testFinished name='testAdd' duration='3']`,
		`##teamcity[testStarted name='testSub' locationHint='php_qn:///app/tests/CalcTest.php::\App\Tests\CalcTest::testSub']`,
		failureLine,
		`##teamcity[testFinished name='testSub' duration='1']`,
		`##teamcity[testStarted name='testSkip']`,
		`##teamcity[testIgnored name='testSkip' message='later']`,
		`##teamcity[testFinished name='testSkip' duration='0']`,
		`##teamcity[testSuiteFinished name='App\Tests\CalcTest']`,
		`##teamcity[testSuiteFinished name='Unit']`,
	)
	counts := tree.Counts()
	if counts.Passed != 1 || counts.Failed != 1 || counts.Skipped != 1 || counts.Total != 3 {
		t.Fatalf("counts = %+v", counts)
	}
	pass := tree.Get(strings.Join([]string{"Unit", suite, "testAdd"}, testResultsSep))
	if pass == nil || pass.Own != TestPassed || pass.DurationMs == nil || *pass.DurationMs != 3 {
		t.Fatalf("passed test = %+v", pass)
	}
	fail := tree.Get(strings.Join([]string{"Unit", suite, "testSub"}, testResultsSep))
	if fail == nil || fail.Location == nil || fail.Location.Path != "/app/tests/CalcTest.php" ||
		fail.Location.Line != 21 {
		t.Fatalf("failure location = %+v", fail)
	}
	if strings.Join(fail.Output, "\n") != "Expected 2\n /app/vendor/phpunit/Assert.php:100\n /app/tests/CalcTest.php:21\n" {
		t.Fatalf("failure output = %q", fail.Output)
	}
}

func TestPHPUnitArgumentsAndFailedFilter(t *testing.T) {
	got := PHPUnitWithTeamcity([]string{"vendor/bin/phpunit", "--stop-on-failure", "tests"})
	want := []string{"vendor/bin/phpunit", "--teamcity", "--stop-on-failure", "tests"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("teamcity args = %v, want %v", got, want)
	}
	if got := PHPUnitWithTeamcity([]string{"--testdox", "tests"}); len(got) != 2 || got[0] != "--testdox" {
		t.Fatalf("explicit reporter changed: %v", got)
	}
	args := PHPUnitArgs([]string{"--filter", "old", "--colors=never", "tests"}, "", "/(App.*)$/", true)
	if strings.Join(args, "\x00") != "--colors=never\x00--filter\x00/(App.*)$/\x00tests" {
		t.Fatalf("filtered args = %v", args)
	}
	filter := PHPUnitFilter([][]string{{"Unit", `App\Tests\CalcTest`, "testAdd", "testAdd with data set #0"}})
	if filter != `/(App\\Tests\\CalcTest::testAdd( with data set .*)?)$/` {
		t.Fatalf("filter = %q", filter)
	}
	filter = PHPUnitFilter([][]string{
		{"Unit", `App\Tests\CalcTest`, "testSecond"},
		{"Unit", `App\Tests\CalcTest`, "testFirst"},
	})
	if filter != `/(App\\Tests\\CalcTest::testSecond( with data set .*)?|App\\Tests\\CalcTest::testFirst( with data set .*)?)$/` {
		t.Fatalf("filter order = %q", filter)
	}
}

func TestRubyParserReadsMinitestFailureSections(t *testing.T) {
	parser := NewRubyParser()
	tree := NewTestResults()
	feedTestParser(t, tree, parser.Feed,
		"MixedTest#test_pass = 0.00 s = .",
		"MixedTest#test_fail = 0.00 s = F",
		"MixedTest#test_skip = 0.00 s = S",
		"MixedTest#test_prints = hello from test",
		"0.00 s = .",
		"MixedTest#test_after_prints = next test",
		"0.00 s = .",
		"Spec style#test_0001_works = 0.00 s = .",
		"",
		"Finished in 0.000893s, 7838.7459 runs/s",
		"",
		"  1) Failure:",
		"MixedTest#test_fail [test/mixed_test.rb:5]:",
		"Expected: 4",
		"  Actual: 3",
		"",
		"  2) Skipped:",
		"MixedTest#test_skip [test/mixed_test.rb:7]:",
		"later",
		"",
		"6 runs, 4 assertions, 1 failures, 0 errors, 1 skips",
	)
	counts := tree.Counts()
	if counts.Passed != 4 || counts.Failed != 1 || counts.Skipped != 1 || counts.Total != 6 {
		t.Fatalf("counts = %+v", counts)
	}
	printed := tree.Get(strings.Join([]string{"MixedTest", "test_prints"}, testResultsSep))
	if printed == nil || printed.Own != TestPassed || len(printed.Output) == 0 || printed.Output[0] != "hello from test" {
		t.Fatalf("printed test = %+v", printed)
	}
	after := tree.Get(strings.Join([]string{"MixedTest", "test_after_prints"}, testResultsSep))
	if after == nil || after.Own != TestPassed || len(after.Output) == 0 || after.Output[0] != "next test" {
		t.Fatalf("test after printed output = %+v", after)
	}
	failed := tree.Get(strings.Join([]string{"MixedTest", "test_fail"}, testResultsSep))
	if failed == nil || failed.Location == nil || failed.Location.Path != "test/mixed_test.rb" ||
		failed.Location.Line != 5 {
		t.Fatalf("failure = %+v", failed)
	}
	if tree.Summary.Failed == nil || *tree.Summary.Failed != 1 ||
		tree.Summary.Passed == nil || *tree.Summary.Passed != 4 {
		t.Fatalf("summary = %+v", tree.Summary)
	}
}

func TestRubyReporterRecordsStayOutOfTheConsole(t *testing.T) {
	parser := NewRubyParser()
	line := `F@@OPENIDEA@@{"t":"result","c":"MixedTest","n":"test_fail","s":"failed","d":0.000063,"f":"/tmp/mixed_test.rb","l":9,"m":"Expected 4"}`
	events := parser.Feed(line)
	tree := NewTestResults()
	tree.Apply(events)
	node := tree.Get(strings.Join([]string{"MixedTest", "test_fail"}, testResultsSep))
	if node == nil || node.Own != TestFailed || node.Location == nil || node.Location.Line != 9 ||
		strings.Join(node.Output, "\n") != "Expected 4" {
		t.Fatalf("record node = %+v", node)
	}
	if node.DurationMs == nil || *node.DurationMs != 0.063 {
		t.Fatalf("record duration = %v", node.DurationMs)
	}
	if got := RubyConsoleText(line); got != "F" {
		t.Fatalf("console text = %q", got)
	}
	if got := RubyConsoleText("Finished in 0.1s"); got != "Finished in 0.1s\n" {
		t.Fatalf("ordinary console text = %q", got)
	}
}

func TestRubyVerboseArguments(t *testing.T) {
	ruby := &openidea.RunConfig{Kind: "ruby", Args: []string{"-Ilib", "test/a_test.rb"}}
	if got := RubyVerboseArgs(ruby); strings.Join(got, " ") != "-Ilib test/a_test.rb -v" {
		t.Fatalf("ruby args = %v", got)
	}
	rake := &openidea.RunConfig{Kind: "rake", Args: []string{"test", "TESTOPTS=--seed 1"}}
	if got := RubyVerboseArgs(rake); got[len(got)-1] != "TESTOPTS=--seed 1 -v" {
		t.Fatalf("rake args = %v", got)
	}
	rspec := &openidea.RunConfig{Kind: "rspec", Args: []string{"spec", "--format", "progress"}}
	if got := RubyVerboseArgs(rspec); len(got) != 3 {
		t.Fatalf("rspec args changed: %v", got)
	}
}

const sampleTRX = `<TestRun xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <Results>
    <UnitTestResult testId="a" testName="adds (1)" outcome="Passed" duration="00:00:00.0015"/>
    <UnitTestResult testId="b" testName="adds (2)" outcome="Failed" duration="00:01:02.5">
      <Output><ErrorInfo>
        <Message>Expected &lt;3&gt;</Message>
        <StackTrace>at Sample.Tests.Adds(Int32 value) in C:\My Project\Tests.cs:line 12</StackTrace>
      </ErrorInfo><StdOut>captured output</StdOut></Output>
    </UnitTestResult>
    <UnitTestResult testId="c" testName="skipped" outcome="NotExecuted"/>
  </Results>
  <TestDefinitions>
    <UnitTest id="a"><TestMethod codeBase="C:/My Project/bin/net9.0/Tests.dll" className="Sample.Tests" name="Adds"/></UnitTest>
    <UnitTest id="b"><TestMethod codeBase="C:/My Project/bin/net9.0/Tests.dll" className="Sample.Tests" name="Adds"/></UnitTest>
    <UnitTest id="c"><TestMethod codeBase="C:/My Project/bin/net9.0/Tests.dll" className="Sample.Tests" name="Skipped"/></UnitTest>
  </TestDefinitions>
</TestRun>`

func TestDotnetTrxParserReadsOutcomeDurationOutputAndLocation(t *testing.T) {
	events, err := TrxEvents(sampleTRX)
	if err != nil {
		t.Fatal(err)
	}
	tree := NewTestResults()
	tree.Apply(events)
	counts := tree.Counts()
	if counts.Passed != 1 || counts.Failed != 1 || counts.Skipped != 1 || counts.Total != 3 {
		t.Fatalf("counts = %+v", counts)
	}
	failed := tree.FirstFailure()
	if failed == nil || failed.Name != "adds (2)" || failed.DurationMs == nil || *failed.DurationMs != 62500 {
		t.Fatalf("failure = %+v", failed)
	}
	if strings.Join(failed.Output, "\n") != "Expected <3>\nat Sample.Tests.Adds(Int32 value) in C:\\My Project\\Tests.cs:line 12\ncaptured output" {
		t.Fatalf("failure output = %q", failed.Output)
	}
	if failed.Location == nil || failed.Location.Path != `C:\My Project\Tests.cs` || failed.Location.Line != 12 {
		t.Fatalf("failure location = %+v", failed.Location)
	}
}

func TestDotnetReportsPatchArgumentsAndReadOnlyItsReports(t *testing.T) {
	config := &openidea.RunConfig{
		Kind: "dotnet-test", Command: "dotnet", Args: []string{"test", "Tests.csproj", "--", "x=y"},
	}
	reports := NewDotnetReports(config)
	args := reports.Args(config)
	if len(args) != 6 || args[2] != "--logger" ||
		args[3] != "trx;LogFilePrefix="+reports.Prefix() || args[4] != "--" {
		t.Fatalf("dotnet args = %v", args)
	}
	windows := `C:\My Project\TestResults\` + reports.Prefix() + `_net9.0.trx`
	unix := `/tmp/My Project/` + reports.Prefix() + `_net8.0.trx`
	reports.Feed(`Results File: C:\old.trx`)
	reports.Feed(`结果文件: ` + windows)
	reports.Feed(`Results File: ` + unix)
	paths := reports.Paths()
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
	read := 0
	events, err := reports.ReadReports(func(path string) (string, error) {
		if path != windows && path != unix {
			t.Fatalf("unexpected report path %q", path)
		}
		read++
		return sampleTRX, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read != 2 || len(events) == 0 {
		t.Fatalf("read %d reports and returned %d events", read, len(events))
	}
}

func TestDotnetFailedConfigurationKeepsRunSettings(t *testing.T) {
	config := &openidea.RunConfig{
		ID: "dotnet:tests", Name: "Tests", Kind: "dotnet-test", Command: "dotnet",
		Args: []string{"test", "--filter", "Category=Unit", "--", "RunConfiguration.MaxCpuCount=1"},
	}
	filter := DotnetFilter([][]string{
		{"C:/Tests.dll", "Sample.CalculatorTests", "Fails"},
		{"C:/Tests.dll", "Sample.CalculatorTests", "Parameterized"},
	})
	args := DotnetArgs(config.Args, filter)
	derived := RerunConfigWithArgs(config, args)
	if derived == nil {
		t.Fatal("no failed configuration")
	}
	got := strings.Join(derived.Args, "\x00")
	want := `test\x00--filter\x00FullyQualifiedName=Sample.CalculatorTests.Fails|FullyQualifiedName=Sample.CalculatorTests.Parameterized\x00--\x00RunConfiguration.MaxCpuCount=1`
	if got != strings.ReplaceAll(want, `\x00`, "\x00") {
		t.Fatalf("derived args = %v", derived.Args)
	}
}
