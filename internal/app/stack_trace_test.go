package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStackTraceNormalizeAndTitle(t *testing.T) {
	raw := "\r\nException in thread \"main\" java.lang.IllegalStateException: boom  \r\n\tat com.acme.App.main(App.java:42) \r\n\r\n"
	text := normalizeStackTrace(raw)
	if text != "Exception in thread \"main\" java.lang.IllegalStateException: boom\n\tat com.acme.App.main(App.java:42)" {
		t.Fatalf("normalized %q", text)
	}
	if title := stackTraceTitle(text); title != "IllegalStateException" {
		t.Fatalf("title %q", title)
	}
	if n := countStackFrames(text); n != 1 {
		t.Fatalf("frames %d", n)
	}
	if len([]rune(stackTraceTitle(strings.Repeat("a", 45)))) != 40 {
		t.Fatal("long title was not shortened")
	}
}

func TestStackTraceLocations(t *testing.T) {
	cases := []struct {
		input, path, resolve string
		line, col            int
	}{
		{"at com.acme.App$Inner.run(App.java:42)", "com/acme/App.java", "name", 42, 0},
		{`  File "app/main.py", line 12, in main`, "app/main.py", "path", 12, 0},
		{"at src/main.ts:3:7", "src/main.ts", "path", 3, 7},
		{"[ERROR] /p/A.java:[12,5] error", "/p/A.java", "path", 12, 5},
		{"src/main.go:15", "src/main.go", "path", 15, 0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			links := traceLinks(tc.input)
			if len(links) != 1 {
				t.Fatalf("links %+v", links)
			}
			loc := links[0].location
			if loc.Path != tc.path || loc.Resolve != tc.resolve || loc.Line != tc.line || loc.Col != tc.col {
				t.Fatalf("location %+v", loc)
			}
		})
	}
	if len(traceLinks(strings.Repeat("x", 4001))) != 0 {
		t.Fatal("oversized line must not be parsed")
	}
}

func TestStackTraceURLsAndFilesShareTheConsoleParser(t *testing.T) {
	line := `Failed at https://example.test/build?job=7).`
	links := traceLinks(line)
	if len(links) != 1 || links[0].url != "https://example.test/build?job=7" {
		t.Fatalf("URL links = %+v", links)
	}
	if countStackFrames(line) != 0 {
		t.Fatal("a URL-only line counted as a stack frame")
	}

	links = traceLinks(`See https://example.test/docs for src/main.go:15:3`)
	if len(links) != 2 || links[0].url == "" || links[1].location.Path != "src/main.go" ||
		links[1].location.Line != 15 || links[1].location.Col != 3 {
		t.Fatalf("mixed links = %+v", links)
	}
	if countStackFrames(`See https://example.test/docs for src/main.go:15:3`) != 1 {
		t.Fatal("the file in a mixed line was not counted as one frame")
	}
}

func TestRunOutputRendersURLsAsNativeLinks(t *testing.T) {
	m := &Model{}
	output := []outputLine{
		{kind: "stdout", text: "server ready"},
		{kind: "stdout", text: "see https://example.test/app"},
		{kind: "stderr", text: "at src/main.go:8:2"},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.renderRunOutput(c, c.Theme(), output)
	}, 800, 200)
	tt.Frame()
	if !tt.HasText("server ready") || !tt.HasText("https://example.test/app") {
		t.Fatalf("console text = %q", tt.Texts())
	}
	if err := tt.Click("https://example.test/app"); err != nil {
		t.Fatal(err)
	}
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != "https://example.test/app" {
		t.Fatalf("opened URLs = %v", got)
	}
}

func TestStackTraceTabs(t *testing.T) {
	m := &Model{stackTrace: &stackTraceState{open: true, active: -1}}
	if m.analyzeStackTrace() {
		t.Fatal("empty trace accepted")
	}
	m.stackTrace.text = "java.lang.IllegalStateException: boom\n  at com.example.App.run(App.java:12)"
	if !m.analyzeStackTrace() || m.stackTrace.open || !m.run.visible || len(m.stackTrace.tabs) != 1 || m.stackTrace.active != 0 {
		t.Fatalf("analysis did not open a Run tab: %+v", m.stackTrace)
	}
	m.stackTrace.tabs = []stackTraceTab{{title: "first"}, {title: "second"}, {title: "third"}}
	m.stackTrace.active = 2
	m.closeStackTraceTab(0)
	if m.stackTrace.active != 1 || len(m.stackTrace.tabs) != 2 {
		t.Fatalf("after closing earlier tab: %+v", m.stackTrace)
	}
	m.closeStackTraceTab(1)
	if m.stackTrace.active != -1 || len(m.stackTrace.tabs) != 1 {
		t.Fatalf("after closing active tab: %+v", m.stackTrace)
	}
}
