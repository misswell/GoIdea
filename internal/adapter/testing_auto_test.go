package adapter

import (
	"testing"

	"mayoi/internal/openidea"
)

func TestAutoTestParserCombinesUnknownToolFormats(t *testing.T) {
	if got := FrameworkFor(&openidea.RunConfig{
		Kind: "shell", Command: "make", Args: []string{"test"},
	}); got != "auto" {
		t.Fatalf("unknown framework = %q", got)
	}
	parser := NewAutoTestParser()
	tree := NewTestResults()
	for _, line := range []string{
		`{"Action":"run","Package":"example.test","Test":"TestAdd"}`,
		`{"Action":"pass","Package":"example.test","Test":"TestAdd","Elapsed":0.01}`,
		"tests/test_math.py::test_add PASSED [100%]",
	} {
		tree.Apply(parser.Feed(line))
	}
	tree.Apply(parser.End())
	if got := tree.Node([]string{"example.test", "TestAdd"}); got == nil || got.Own != TestPassed {
		t.Fatalf("go result = %+v", got)
	}
	if got := tree.Node([]string{"tests/test_math.py", "test_add"}); got == nil || got.Own != TestPassed {
		t.Fatalf("pytest result = %+v", got)
	}
}
