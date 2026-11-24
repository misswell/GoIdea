package app

import (
	"reflect"
	"testing"
)

func TestParseSELocationQueryMatchesOpenIDEA(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		root string
		want seLocationQuery
	}{
		{"plain", "OrderController", "", seLocationQuery{text: "OrderController"}},
		{"line", "File.kt:42", "", seLocationQuery{text: "File.kt", line: 41, has: true}},
		{"line and column", "File.kt:42:5", "", seLocationQuery{text: "File.kt", line: 41, col: 4, has: true}},
		{"empty column", "src/a.ts:10:", "", seLocationQuery{text: "src/a.ts", line: 9, has: true}},
		{"stack line", "Main.java(12)", "", seLocationQuery{text: "Main.java", line: 11, has: true}},
		{"stack line and column", "lib.rs(3,7)", "", seLocationQuery{text: "lib.rs", line: 2, col: 6, has: true}},
		{"zero line", "File.kt:0", "", seLocationQuery{text: "File.kt:0"}},
		{"non-numeric", "a:b", "", seLocationQuery{text: "a:b"}},
		{"project root", "/dev/acme/src/Order.kt:3", "/dev/acme", seLocationQuery{text: "src/Order.kt", line: 2, has: true}},
		{"outside root", "/elsewhere/x.kt", "/dev/acme", seLocationQuery{text: "/elsewhere/x.kt"}},
		{"quoted", `"src/a.ts"`, "", seLocationQuery{text: "src/a.ts"}},
		{"file URL", "file:///dev/acme/My%20File.kt", "/dev/acme", seLocationQuery{text: "My File.kt"}},
		{"windows separators", `src\main\App.kt`, "", seLocationQuery{text: "src/main/App.kt"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseSELocationQuery(c.raw, c.root); got != c.want {
				t.Fatalf("parseSELocationQuery(%q, %q) = %+v, want %+v", c.raw, c.root, got, c.want)
			}
		})
	}
}

func TestParseSECommandListsAndRunsScopes(t *testing.T) {
	list, ok := parseSECommand("/fi")
	if !ok || !list.list || len(list.rows) != 1 {
		t.Fatalf("/fi did not list Files: %+v, %v", list, ok)
	}
	if got := list.rows[0]; got.kind != "command" || got.title != "/files" ||
		got.detail != "Files" || got.command != seTabFiles {
		t.Fatalf("/fi row = %+v", got)
	}

	all, ok := parseSECommand("/")
	if !ok || !all.list || len(all.rows) != len(seScopeCommands) {
		t.Fatalf("/ did not list every scope: %+v, %v", all, ok)
	}
	names := make([]string, len(all.rows))
	for i, row := range all.rows {
		names[i] = row.title
	}
	if want := []string{"/classes", "/files", "/symbols", "/actions", "/text"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("scope order = %v, want %v", names, want)
	}

	run, ok := parseSECommand("/files  src/window.go ")
	if !ok || run.list || run.tab != seTabFiles || run.rest != "src/window.go " {
		t.Fatalf("/files command = %+v, %v", run, ok)
	}
	if _, ok := parseSECommand("/usr/local/bin"); ok {
		t.Fatal("an absolute path was treated as a command")
	}
	if _, ok := parseSECommand("/unknown value"); ok {
		t.Fatal("an unknown command was treated as a command")
	}
}
