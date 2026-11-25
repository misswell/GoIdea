package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestFindWindowShowsResultTreeAndCodePreview(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "App.java")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "class App {\n  void run() {\n    System.out.println(\"hit\");\n  }\n}\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	m.search.visible = true
	m.search.query = "hit"
	m.search.done = true
	m.search.files = []openidea.FileMatches{{
		Path: path, RelPath: "src/App.java",
		Matches: []openidea.LineMatch{{
			Line: 2, Text: "    System.out.println(\"hit\");", Ranges: [][2]uint32{{24, 27}},
		}},
	}}
	m.search.matches = 1
	m.search.refreshPreview()
	tester := ui.NewTester(func(c *ui.Context) {
		m.searchPanel(c, c.Theme())
	}, 1000, 600)
	tester.Frame()
	for _, want := range []string{
		"Find", "src/App.java", "System.out.println", "class App", "void run",
	} {
		if !tester.HasText(want) {
			t.Fatalf("%q is missing from Find: %q", want, tester.Texts())
		}
	}
}
