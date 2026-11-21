package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func blame(commit, author string, ms float64, summary string) openidea.BlameLine {
	return openidea.BlameLine{Commit: commit, Author: author, TimeMs: ms, Summary: summary}
}

func TestAnnotateRowsGroupConsecutiveBlame(t *testing.T) {
	lines := []openidea.BlameLine{
		blame("aaa", "Alice", 1000, "First"),
		blame("aaa", "Alice", 1000, "First"),
		blame("bbb", "Bob", 2000, "Second"),
		blame("aaa", "Alice", 1000, "First"),
	}
	rows := annotateRows(lines)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].From != 1 || rows[0].To != 2 || rows[0].Commit != "aaa" {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].From != 3 || rows[1].To != 3 || rows[1].Commit != "bbb" {
		t.Fatalf("second row = %+v", rows[1])
	}
	if rows[2].From != 4 || rows[2].To != 4 || rows[2].Commit != "aaa" {
		t.Fatalf("third row = %+v", rows[2])
	}
}

func TestAnnotateRowsKeepSameCommitApartForDifferentAuthors(t *testing.T) {
	rows := annotateRows([]openidea.BlameLine{
		blame("aaa", "Alice", 1000, "First"),
		blame("aaa", "Bob", 1000, "First"),
	})
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
}

func TestAnnotateRowsEmpty(t *testing.T) {
	if rows := annotateRows(nil); len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

func TestAnnotateLinesRanges(t *testing.T) {
	if got := annotateLines(annotateRow{From: 12, To: 12}); got != "12" {
		t.Fatalf("single line = %q", got)
	}
	if got := annotateLines(annotateRow{From: 12, To: 18}); got != "12–18" {
		t.Fatalf("range = %q", got)
	}
}

func TestAnnotateTimeDateAndUncommitted(t *testing.T) {
	prefs := DateFormatPrefs{OverrideSystem: true, Pattern: "yyyy-MM-dd"}
	if got := annotateTime(prefs, dateRegion{}, 0); got != "" {
		t.Fatalf("uncommitted date = %q, want empty", got)
	}
	got := annotateTime(prefs, dateRegion{}, 1_700_000_000_000)
	if len(got) != 10 || got[4] != '-' || got[7] != '-' {
		t.Fatalf("date = %q, want yyyy-mm-dd", got)
	}
}

func TestAnnotateShortAuthorAndUncommittedCommit(t *testing.T) {
	if got := annotateShortAuthor("Thomas Müller"); got != "Thomas" {
		t.Fatalf("short author = %q", got)
	}
	if got := annotateShortAuthor("AlexandriaTheGreat"); got != "AlexandriaT…" {
		t.Fatalf("truncated author = %q", got)
	}
	if !annotateUncommitted("0000000000000000000000000000000000000000") {
		t.Fatal("all-zero commit was not treated as uncommitted")
	}
	if annotateUncommitted("0000a000") {
		t.Fatal("a real commit was treated as uncommitted")
	}
}

func TestSplitCommitMessageKeepsMessageBody(t *testing.T) {
	subject, body := splitCommitMessage("Fix the parser\r\n\r\nPreserve CRLF and a second line.\r\n")
	if subject != "Fix the parser" || body != "Preserve CRLF and a second line." {
		t.Fatalf("commit message = %q / %q", subject, body)
	}
}

func TestEnsureBlameCommitLoadsEmailAndFullMessage(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "src", "Main.java")
	doc := docbridge.Virtual(path, "class Main {}\n")
	tab := &Tab{doc: doc}
	blameState := &tabBlameState{ready: true}
	finished := make(chan struct{}, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "vcs_log" {
				t.Fatalf("unexpected command %q", command)
			}
			got := args.(map[string]any)
			if got["repo"] != root {
				t.Fatalf("repo = %v, want %q", got["repo"], root)
			}
			query := got["query"].(map[string]any)
			if query["branch"] != "abc123" {
				t.Fatalf("branch = %v", query["branch"])
			}
			return json.Marshal([]openidea.Commit{{
				ID: "abc123", Author: "Alice", Email: "alice@example.test",
				TimeMs: 1_700_000_000_000, Summary: "Fix parser",
				Message: "Fix parser\n\nKeep the full body.",
			}})
		},
	}
	m.setRoot(root)
	tab.blame = blameState
	m.ensureBlameCommit(tab, blameState, blame("abc123", "A. Alice", 1_700_000_000_000, "Fix parser"))
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("commit details did not finish")
	}
	detail := blameState.commits["abc123"]
	if detail.loading || detail.err != "" || detail.commit.Email != "alice@example.test" {
		t.Fatalf("commit detail = %+v", detail)
	}
	subject, body := splitCommitMessage(lineMessage(
		blame("abc123", "A. Alice", 1_700_000_000_000, "Fix parser"), detail.commit))
	if subject != "Fix parser" || body != "Keep the full body." {
		t.Fatalf("displayed message = %q / %q", subject, body)
	}
}

func TestShowAnnotateTogglesTheOpenTabGutter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "Main.java")
	doc := docbridge.Virtual(path, "class Main {}\n")
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	finished := make(chan struct{}, 1)
	m := &Model{
		ctx:  context.Background(),
		tabs: []*Tab{tab},
		postHook: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "vcs_blame" {
				t.Fatalf("unexpected command %q", command)
			}
			if got := args.(map[string]any)["path"]; got != path {
				t.Fatalf("blame path = %v, want %q", got, path)
			}
			return json.Marshal([]openidea.BlameLine{
				blame("aaa", "Alice", 1_700_000_000_000, "Initial import"),
			})
		},
	}
	m.setRoot(root)

	m.ShowAnnotate(path)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Git blame did not finish")
	}
	if tab.blame == nil || !tab.blame.ready || len(tab.blame.lines) != 1 {
		t.Fatalf("annotation gutter = %+v", tab.blame)
	}

	m.ShowAnnotate(path)
	if tab.blame != nil {
		t.Fatal("a second annotation action did not close the gutter")
	}
}

func TestAnnotateGutterRendersAuthorAndDateBesideEditor(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Main.java")
	doc := docbridge.Virtual(path, "class Main {}\nclass Other {}\n")
	m := &Model{}
	m.setRoot(root)
	tab := m.newTab(doc)
	tab.blame = &tabBlameState{
		ready: true, version: doc.Buffer().Version(),
		lines: []openidea.BlameLine{
			blame("aaa", "Alice", 1_700_000_000_000, "Initial import"),
			blame("bbb", "Bob", 1_700_000_100_000, "Follow-up"),
		},
	}
	m.tabs = []*Tab{tab}
	m.active = 0
	m.prefs.DateFormat = &DateFormatPrefs{
		OverrideSystem: true, Pattern: "yyyy-MM-dd", Use24Hour: true, Pretty: false,
	}

	tt := workbenchTester(t, m)
	tt.Frame()
	if !tt.HasText("Alice") || !tt.HasText("Bob") ||
		!tt.HasText(annotateTime(m.prefs.dateFormatPrefs(), dateRegion{}, 1_700_000_000_000)) {
		t.Fatalf("native annotation gutter is absent: %q", tt.Texts())
	}
}
