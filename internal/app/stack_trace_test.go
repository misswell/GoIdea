package app

import (
	"strings"
	"testing"
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
