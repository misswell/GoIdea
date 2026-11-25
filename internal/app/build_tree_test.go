package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestBuildWindowShowsCompilerMessagesBesideTheConsole(t *testing.T) {
	path := "/project/src/App.java"
	m := &Model{}
	m.build.visible = true
	m.build.title = "Build Project"
	m.build.messages = []openidea.BuildMessage{
		{
			Severity: "error", Text: "cannot find symbol", Path: &path,
			Range: &openidea.OutlineRange{Start: openidea.OutlinePos{Line: 4, Col: 8}},
		},
		{
			Severity: "warning", Text: "unused import", Path: &path,
			Range: &openidea.OutlineRange{Start: openidea.OutlinePos{Line: 8, Col: 1}},
		},
	}
	m.build.output = []outputLine{{kind: "system", text: "mvn compile"}}
	m.run.trusted = true
	tester := ui.NewTester(func(c *ui.Context) {
		m.buildPanel(c, c.Theme())
	}, 1000, 600)
	tester.Frame()
	for _, text := range []string{"Compiler messages", "App.java", "cannot find symbol", "unused import", "mvn compile"} {
		if !tester.HasText(text) {
			t.Fatalf("%q is missing from Build: %q", text, tester.Texts())
		}
	}
}
