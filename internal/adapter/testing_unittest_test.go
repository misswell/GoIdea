package adapter

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

const unittestSample = `System check identified no issues (0 silenced).
test_documented (polls.tests.QuestionModelTests.test_documented)
Explains what is checked. ... ok
test_errors (polls.tests.QuestionModelTests.test_errors) ... ERROR
test_expected_failure (polls.tests.QuestionModelTests.test_expected_failure) ... expected failure
test_fails (polls.tests.QuestionModelTests.test_fails) ... FAIL
test_passes (polls.tests.QuestionModelTests.test_passes) ... ok
test_prints (polls.tests.QuestionModelTests.test_prints) ... hello from stderr
ok
test_skipped (polls.tests.QuestionModelTests.test_skipped) ... skipped 'not ready'
test_subtests (polls.tests.QuestionModelTests.test_subtests)
  test_subtests (polls.tests.QuestionModelTests.test_subtests) (i=1) ... FAIL
test_unexpected_success (polls.tests.QuestionModelTests.test_unexpected_success) ... unexpected success
test_ok (polls.more.test_views.ViewTests.test_ok) ... ok
setUpClass (polls.tests.BrokenSetupTests) ... ERROR

======================================================================
ERROR: test_errors (polls.tests.QuestionModelTests.test_errors)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/e2e/proj/polls/tests.py", line 15, in test_errors
    raise ValueError("boom")
ValueError: boom

======================================================================
ERROR: setUpClass (polls.tests.BrokenSetupTests)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/venvs/lib/python3.12/site-packages/django/test/testcases.py", line 50, in setUpClass
    result = test(result)
  File "/e2e/proj/polls/tests.py", line 50, in setUpClass
    raise RuntimeError("setup broke")

======================================================================
FAIL: test_fails (polls.tests.QuestionModelTests.test_fails)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/e2e/proj/polls/tests.py", line 12, in test_fails
    self.assertEqual("left", "right")
AssertionError: 'left' != 'right'

======================================================================
FAIL: test_subtests (polls.tests.QuestionModelTests.test_subtests) (i=1)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/e2e/proj/polls/tests.py", line 43, in test_subtests
    self.assertNotEqual(i, 1)
AssertionError: 1 == 1

======================================================================
UNEXPECTED SUCCESS: test_unexpected_success (polls.tests.QuestionModelTests.test_unexpected_success)
----------------------------------------------------------------------
Ran 10 tests in 0.001s

FAILED (failures=2, errors=2, skipped=1, expected failures=1, unexpected successes=1)`

func applyUnittest(t *testing.T, text string) *TestResults {
	t.Helper()
	parser := NewUnittestParser()
	tree := NewTestResults()
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		tree.Apply(parser.Feed(line))
	}
	tree.Apply(parser.End())
	tree.Finish(1)
	return tree
}

func TestUnittestParserReadsDjangoResultsSectionsAndSummary(t *testing.T) {
	tree := applyUnittest(t, unittestSample)
	counts := tree.Counts()
	if counts.Passed != 5 || counts.Failed != 5 || counts.Skipped != 1 || counts.Total != 11 {
		t.Fatalf("counts = %+v", counts)
	}
	if tree.Summary.Failed == nil || *tree.Summary.Failed != 5 ||
		tree.Summary.Skipped == nil || *tree.Summary.Skipped != 1 {
		t.Fatalf("summary = %+v", tree.Summary)
	}
	fail := tree.Get(strings.Join(
		[]string{"polls.tests", "QuestionModelTests", "test_fails"}, testResultsSep,
	))
	if fail == nil || fail.Own != TestFailed || fail.Location == nil ||
		fail.Location.Path != "/e2e/proj/polls/tests.py" || fail.Location.Line != 12 {
		t.Fatalf("failure = %+v", fail)
	}
	fixture := tree.Get(strings.Join([]string{"polls.tests", "BrokenSetupTests"}, testResultsSep))
	if fixture == nil || fixture.Location == nil || fixture.Location.Line != 50 {
		t.Fatalf("fixture failure = %+v", fixture)
	}
	printed := tree.Get(strings.Join(
		[]string{"polls.tests", "QuestionModelTests", "test_prints"}, testResultsSep,
	))
	if printed == nil || printed.Own != TestPassed {
		t.Fatalf("split result = %+v", printed)
	}
}

func TestUnittestParserReadsMultipleResultsUnicodeAndImportFailure(t *testing.T) {
	text := `test_a (pkg.C.test_a) ... test_b (pkg.C.test_b) ... ERROR
test_città (pkg.C.test_città) ... FAIL
broken.tests (unittest.loader._FailedTest.broken.tests) ... ERROR

======================================================================
FAIL: test_città (pkg.C.test_città)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/project/tests/unittest/test_x.py", line 9, in test_città
    self.assertTrue(False)
AssertionError: False is not true

======================================================================
ERROR: broken.tests (unittest.loader._FailedTest.broken.tests)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/project/broken/tests.py", line 1, in <module>
ModuleNotFoundError: no module

----------------------------------------------------------------------
Ran 3 tests in 0.004s

FAILED (failures=1, errors=2)`
	tree := applyUnittest(t, text)
	if tree.Get(strings.Join([]string{"pkg", "C", "test_a"}, testResultsSep)) == nil {
		t.Fatal("first result on a shared line is missing")
	}
	second := tree.Get(strings.Join([]string{"pkg", "C", "test_b"}, testResultsSep))
	if second == nil || second.Own != TestFailed {
		t.Fatalf("second result on a shared line = %+v", second)
	}
	unicode := tree.Get(strings.Join([]string{"pkg", "C", "test_città"}, testResultsSep))
	if unicode == nil || unicode.Location == nil ||
		unicode.Location.Path != "/project/tests/unittest/test_x.py" {
		t.Fatalf("project unittest location = %+v", unicode)
	}
	broken := tree.Get("broken.tests")
	if broken == nil || broken.Own != TestFailed || broken.Location == nil ||
		broken.Location.Line != 1 {
		t.Fatalf("import failure = %+v", broken)
	}
}

func TestUnittestReportingAndDjangoRerunArguments(t *testing.T) {
	python := &openidea.RunConfig{Kind: "python", Command: "python3", Args: []string{"manage.py", "test"}}
	if got := FrameworkFor(python); got != "unittest" {
		t.Fatalf("python framework = %q", got)
	}
	if got := FrameworkFor(&openidea.RunConfig{Kind: "python", Command: "python3", Args: []string{"script.py"}}); got != "pytest" {
		t.Fatalf("plain python framework = %q", got)
	}
	if got := FrameworkFor(&openidea.RunConfig{
		Kind: "shell", Command: "npx", Args: []string{"gradle", "test"},
	}); got != "js" {
		t.Fatalf("frontend command containing gradle = %q", got)
	}
	if got := UnittestWithVerbose(python); strings.Join(got, " ") != "manage.py test -v 2" {
		t.Fatalf("verbose args = %v", got)
	}
	if got := UnittestWithVerbose(&openidea.RunConfig{Args: []string{"manage.py", "test", "-v", "3"}}); len(got) != 4 {
		t.Fatalf("chosen verbosity changed: %v", got)
	}

	config := &openidea.RunConfig{
		ID: "django:tests", Name: "Django tests", Kind: "django-test",
		Command: "python3", Args: []string{"manage.py", "test", "polls", "--keepdb"},
		Params: map[string]any{"target": "polls", "settings": "site.settings"},
	}
	derived := RerunFailedConfig(config, [][]string{
		{"polls.tests", "QuestionTests", "test_fails"},
		{"polls.tests", "QuestionTests", "test_errors"},
	})
	if derived == nil {
		t.Fatal("no failed Django configuration")
	}
	if got := strings.Join(derived.Args, " "); got != "manage.py test polls.tests.QuestionTests.test_fails polls.tests.QuestionTests.test_errors --keepdb" {
		t.Fatalf("Django args = %q", got)
	}
	if derived.Params["target"] != "polls.tests.QuestionTests.test_fails polls.tests.QuestionTests.test_errors" ||
		derived.Params["settings"] != "site.settings" {
		t.Fatalf("Django params = %#v", derived.Params)
	}
}
