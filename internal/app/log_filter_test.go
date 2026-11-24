package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestGitLogFilterQueryUsesOpenIDEAFilters(t *testing.T) {
	if got := splitGitLogFilterValues(" Ada, me\nAda, , release/* "); len(got) != 3 ||
		got[0] != "Ada" || got[1] != "me" || got[2] != "release/*" {
		t.Fatalf("split filters = %#v", got)
	}
	m := &Model{
		prefs: Preferences{VCS: VCSPrefs{Log: LogPrefs{
			Sort:        "standard",
			FirstParent: boolPointer(true),
			NoMerges:    boolPointer(true),
		}}},
		log: LogPane{
			filterAuthors:   "me, Ada",
			filterText:      "fix.*",
			filterPaths:     "src, docs/*.md",
			filterBranches:  "main, release/*",
			filterSince:     "2024-01-02",
			filterUntil:     "2024-01-03",
			filterRegex:     true,
			filterMatchCase: true,
		},
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "vcs_config_get" {
			t.Fatalf("unexpected command %q", command)
		}
		values := args.(map[string]any)
		if values["key"] != "user.email" || values["repo"] != "/project" {
			t.Fatalf("vcs_config_get args = %#v", values)
		}
		return json.RawMessage(`"me@example.com"`), nil
	}
	spec, err := m.gitLogQuerySpec("/project/README.md", "feature", 75)
	if err != nil {
		t.Fatal(err)
	}
	query, err := m.gitLogQuery(context.Background(), "/project", spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := gitLogTestStringValue(query["branch"]); got != "feature" {
		t.Fatalf("branch = %q", got)
	}
	if got := gitLogTestStringValues(query["branches"]); len(got) != 1 || got[0] != "release/*" {
		t.Fatalf("additional branches = %#v", got)
	}
	if got := gitLogTestStringValue(query["path"]); got != "/project/README.md" {
		t.Fatalf("path = %q", got)
	}
	if got := gitLogTestStringValues(query["paths"]); len(got) != 1 || got[0] != "docs/*.md" {
		t.Fatalf("additional paths = %#v", got)
	}
	if got := gitLogTestStringValue(query["author"]); got != "me@example.com" {
		t.Fatalf("author = %q", got)
	}
	if got := gitLogTestStringValues(query["authors"]); len(got) != 1 || got[0] != "Ada" {
		t.Fatalf("additional authors = %#v", got)
	}
	if got := gitLogTestStringValue(query["text"]); got != "fix.*" {
		t.Fatalf("text = %q", got)
	}
	if query["textRegex"] != true || query["textMatchCase"] != true ||
		query["firstParent"] != true || query["noMerges"] != true {
		t.Fatalf("boolean filters = %#v", query)
	}
	if query["order"] != "standard" || query["limit"] != uint32(75) {
		t.Fatalf("order/limit = %#v", query)
	}
	since := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)
	until := time.Date(2024, 1, 4, 0, 0, 0, 0, time.Local)
	if query["sinceMs"] != float64(since.UnixMilli()) ||
		query["untilMs"] != float64(until.UnixMilli()) {
		t.Fatalf("date filters = since %#v until %#v", query["sinceMs"], query["untilMs"])
	}
}

func TestGitLogFilterBarRendersNativeControls(t *testing.T) {
	m := &Model{log: LogPane{
		filtersOpen:    true,
		filterAuthors:  "me",
		filterText:     "needle",
		filterPaths:    "src",
		filterBranches: "main",
		filterSince:    "2024-01-01",
		filterUntil:    "2024-02-01",
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.gitLogFilterBar(c, c.Theme())
	}, 1180, 180)
	tester.Frame()
	for _, text := range []string{
		"Text", "Regex", "Match Case", "Apply", "Clear",
		"User", "Branches", "Paths", "Since", "Until",
	} {
		if !tester.HasText(text) {
			t.Errorf("filter bar is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestRefreshLogSendsNativeFilterQuery(t *testing.T) {
	queryArgs := make(chan map[string]any, 1)
	posted := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		log: LogPane{
			repo:           "/project",
			visible:        true,
			filterAuthors:  "Ada",
			filterText:     "needle",
			filterBranches: "main,release/*",
			filterPaths:    "src,docs/*.md",
			filterSince:    "2024-01-01",
		},
	}
	m.setRoot("/project")
	m.vcs.current = "/project"
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "vcs_log" {
			t.Fatalf("unexpected command %q", command)
		}
		values := args.(map[string]any)
		if values["repo"] != "/project" {
			t.Fatalf("vcs_log repo = %#v", values["repo"])
		}
		queryArgs <- values["query"].(map[string]any)
		return json.RawMessage(`[]`), nil
	}
	m.RefreshLog()
	select {
	case query := <-queryArgs:
		if got := gitLogTestStringValue(query["author"]); got != "Ada" {
			t.Fatalf("author = %q", got)
		}
		if got := gitLogTestStringValue(query["text"]); got != "needle" {
			t.Fatalf("text = %q", got)
		}
		if got := gitLogTestStringValue(query["branch"]); got != "main" {
			t.Fatalf("branch = %q", got)
		}
		if got := gitLogTestStringValue(query["path"]); got != "src" {
			t.Fatalf("path = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("vcs_log was not called")
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("filtered log did not finish")
	}
	if m.log.loading || m.log.err != "" || m.log.filterErr != "" {
		t.Fatalf("filtered log state = loading %v err %q filterErr %q",
			m.log.loading, m.log.err, m.log.filterErr)
	}
}

func TestGitLogRejectsInvalidDateWithoutStartingAQuery(t *testing.T) {
	m := &Model{log: LogPane{filterSince: "tomorrow"}}
	m.ApplyGitLogFilters()
	if m.log.loading {
		t.Fatal("invalid date started a log query")
	}
	if !strings.Contains(m.log.filterErr, "YYYY-MM-DD") {
		t.Fatalf("filter error = %q", m.log.filterErr)
	}
}

func gitLogTestStringValue(value any) string {
	if value == nil {
		return ""
	}
	if pointer, ok := value.(*string); ok {
		if pointer == nil {
			return ""
		}
		return *pointer
	}
	text, _ := value.(string)
	return text
}

func gitLogTestStringValues(value any) []string {
	values, _ := value.([]string)
	return values
}
