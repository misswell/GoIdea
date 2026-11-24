package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestVCSBranchesPageLoadsBranchesAndTagsConcurrently(t *testing.T) {
	root := "/project/orders"
	started := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			repo := args.(map[string]any)["repo"]
			if repo != root {
				t.Errorf("%s repository = %#v, want %q", command, repo, root)
			}
			started <- command
			<-release
			switch command {
			case "vcs_branches":
				return mustVCSJSON(t, []openidea.Branch{
					{Name: "origin/main", Remote: true},
					{Name: "main", Current: true},
					{Name: "feature/login"},
				}), nil
			case "vcs_tags":
				return mustVCSJSON(t, []string{"v2.0", "v1.0"}), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
		postHook: func(fn func()) {
			fn()
			done <- struct{}{}
		},
	}
	m.setRoot("/project")
	m.vcs.repositories = []vcsRepository{{Root: root, Head: str("main")}}
	m.vcs.current = root

	m.refreshVCSBranchesPage(root)
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case command := <-started:
			seen[command] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("only these branch-page reads started: %v", seen)
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("branch page did not finish")
	}

	page := m.vcsBranchesPane(root)
	if !page.loaded || page.loading || len(page.branches) != 3 || len(page.tags) != 2 {
		t.Fatalf("branch page = %+v", page)
	}
	if page.branches[0].Name != "feature/login" ||
		page.branches[1].Name != "main" ||
		page.branches[2].Name != "origin/main" {
		t.Fatalf("branch order = %+v", page.branches)
	}
	if page.tags[0] != "v1.0" || page.tags[1] != "v2.0" {
		t.Fatalf("tag order = %+v", page.tags)
	}
	if page.selectedKey != "branch:main" {
		t.Fatalf("selected branch = %q, want main", page.selectedKey)
	}
}

func TestVCSStashPageMergesRootsAndRoutesActionsToOwner(t *testing.T) {
	first := "/project/orders"
	second := "/project/catalog"
	applied := make(chan map[string]any, 1)
	posted := make(chan struct{}, 64)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "vcs_stashes":
				switch args.(map[string]any)["repo"] {
				case first:
					return mustVCSJSON(t, []openidea.Stash{{Index: 0, Message: "On main: Orders"}}), nil
				case second:
					return mustVCSJSON(t, []openidea.Stash{{Index: 1, Message: "On feature: Catalog"}}), nil
				default:
					t.Fatalf("unexpected stash repository: %#v", args)
					return nil, nil
				}
			case "vcs_stash_apply":
				applied <- args.(map[string]any)
				return json.RawMessage(`null`), nil
			case "vcs_roots":
				return mustVCSJSON(t, []vcsRepository{
					{Root: first, Head: str("main")},
					{Root: second, Head: str("feature")},
				}), nil
			case "vcs_status":
				return json.RawMessage(`[]`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
		postHook: func(fn func()) {
			fn()
			select {
			case posted <- struct{}{}:
			default:
			}
		},
	}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("feature")},
	}
	m.refreshVCSStashPage()
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("stash page did not load")
	}
	page := m.vcsStashPageState()
	if !page.loaded || len(page.entries) != 2 {
		t.Fatalf("stash entries = %+v", page.entries)
	}
	if page.entries[0].Repo != first || page.entries[1].Repo != second {
		t.Fatalf("stash owners = %+v", page.entries)
	}

	m.applyVCSStash(page.entries[1], false)
	select {
	case args := <-applied:
		if args["repo"] != second || args["index"] != 1 || args["pop"] != false {
			t.Fatalf("stash apply args = %#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stash apply did not reach the backend")
	}
}

func TestVCSShelfPageResolvesRepositoryAndLegacyShelf(t *testing.T) {
	first := filepath.Clean("/project")
	second := filepath.Clean("/project/catalog")
	primary := first
	explicit := second
	m := &Model{}
	m.setRoot("/project")
	m.vcs.current = second
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("feature")},
	}
	page := m.vcsShelfPageState()
	page.loaded = true
	page.shelves = []shelvedChangeList{
		{ID: "legacy", Name: "Legacy shelf", Repo: nil},
		{ID: "catalog", Name: "Catalog shelf", Repo: &explicit},
	}

	if got := m.vcsShelfRepository(page.shelves[0]); got != primary {
		t.Fatalf("legacy shelf repository = %q, want %q", got, primary)
	}
	if got := m.vcsShelfRepository(page.shelves[1]); got != explicit {
		t.Fatalf("explicit shelf repository = %q, want %q", got, explicit)
	}
	page.query = "catalog"
	rows := m.vcsFilteredShelves()
	if len(rows) != 1 || rows[0].ID != "catalog" {
		t.Fatalf("filtered shelves = %+v", rows)
	}
}

func TestVCSContentPageResetCancelsInFlightReads(t *testing.T) {
	canceled := make(chan string, 2)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(ctx context.Context, command string, _ any) (json.RawMessage, error) {
			<-ctx.Done()
			canceled <- command
			return nil, ctx.Err()
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.vcs.current = "/project"
	m.vcs.repositories = []vcsRepository{{Root: "/project", Head: str("main")}}

	m.refreshVCSBranchesPage("/project")
	m.refreshVCSStashPage()
	m.resetVCS()
	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case command := <-canceled:
			seen[command] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("in-flight VCS reads not canceled: %v", seen)
		}
	}
	if !seen["vcs_branches"] || !seen["vcs_tags"] || !seen["vcs_stashes"] {
		t.Fatalf("canceled commands = %v", seen)
	}
}

func TestVCSShelfFilterMatchesRepositoryLabel(t *testing.T) {
	first := filepath.Clean("/project/app")
	second := filepath.Clean("/project/catalog")
	withRepo := second
	m := &Model{}
	m.setRoot("/project")
	m.vcs.current = second
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("catalog-work")},
	}
	page := m.vcsShelfPageState()
	page.shelves = []shelvedChangeList{
		{ID: "legacy", Name: "Rename model"},
		{ID: "catalog", Name: "Catalog model", Repo: &withRepo},
	}
	page.query = strings.ToLower(m.vcsRepositoryLabelForRoot(second))
	rows := m.vcsFilteredShelves()
	if len(rows) != 1 || rows[0].ID != "catalog" {
		t.Fatalf("repository-filtered shelves = %+v", rows)
	}
}

func TestVCSAggregateLogMergesRootsAndRoutesCommitDetails(t *testing.T) {
	first := "/project/orders"
	second := "/project/catalog"
	type response struct {
		raw string
	}
	responses := map[string]chan response{
		first:  make(chan response, 1),
		second: make(chan response, 1),
	}
	started := make(chan string, 2)
	detailRepo := make(chan string, 2)
	posted := make(chan struct{}, 32)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			select {
			case posted <- struct{}{}:
			default:
			}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			values := args.(map[string]any)
			repo, _ := values["repo"].(string)
			switch command {
			case "vcs_log":
				started <- repo
				next := <-responses[repo]
				return json.RawMessage(next.raw), nil
			case "vcs_log_details":
				detailRepo <- repo
				return json.RawMessage(`{"message":"details"}`), nil
			case "vcs_commits_changes":
				detailRepo <- repo
				return json.RawMessage(`[]`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("feature")},
	}
	m.log = LogPane{aggregate: true, repo: first}

	m.RefreshLog()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case repo := <-started:
			seen[repo] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("aggregate log did not query every root: %v", seen)
		}
	}
	responses[first] <- response{raw: `[
		{"id":"111111111111","shortId":"11111111","summary":"First","timeMs":10}
	]`}
	responses[second] <- response{raw: `[
		{"id":"222222222222","shortId":"22222222","summary":"Second","timeMs":20}
	]`}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("aggregate log did not finish")
	}
	if len(m.log.commits) != 2 ||
		m.log.commits[0].ID != "222222222222" ||
		m.log.commits[1].ID != "111111111111" {
		t.Fatalf("aggregate commits = %+v", m.log.commits)
	}
	if len(m.log.commitRepos) != 2 ||
		m.log.commitRepos[0] != second ||
		m.log.commitRepos[1] != first {
		t.Fatalf("aggregate owners = %+v", m.log.commitRepos)
	}
	if m.log.selected != 0 || m.log.detailRepo != second {
		t.Fatalf("selected aggregate commit = index %d, repo %q", m.log.selected, m.log.detailRepo)
	}
	for _, command := range []string{"vcs_log_details", "vcs_commits_changes"} {
		select {
		case repo := <-detailRepo:
			if repo != second {
				t.Fatalf("%s repository = %q, want %q", command, repo, second)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not reach the owner repository", command)
		}
	}
}

func TestVCSAggregateLogResetCancelsEveryRoot(t *testing.T) {
	first := "/project/orders"
	second := "/project/catalog"
	started := make(chan string, 2)
	canceled := make(chan string, 2)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(ctx context.Context, _ string, args any) (json.RawMessage, error) {
			repo := args.(map[string]any)["repo"].(string)
			started <- repo
			<-ctx.Done()
			canceled <- repo
			return nil, ctx.Err()
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("feature")},
	}
	m.log = LogPane{aggregate: true, repo: first}

	m.RefreshLog()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case repo := <-started:
			seen[repo] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("aggregate reads did not start: %v", seen)
		}
	}
	m.resetLog()
	canceledRoots := map[string]bool{}
	for len(canceledRoots) < 2 {
		select {
		case repo := <-canceled:
			canceledRoots[repo] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("aggregate reads were not canceled: %v", canceledRoots)
		}
	}
}

func TestVCSAggregateLogTransitionDropsStaleSingleRepositoryResponse(t *testing.T) {
	first := "/project/orders"
	second := "/project/catalog"
	singleStarted := make(chan struct{})
	releaseSingle := make(chan struct{})
	aggregateStarted := make(chan string, 4)
	posted := make(chan struct{}, 4)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "vcs_log" {
				return json.RawMessage(`null`), nil
			}
			values := args.(map[string]any)
			query := values["query"].(map[string]any)
			if query["branch"] == "HEAD" {
				close(singleStarted)
				<-releaseSingle
				return json.RawMessage(`[
					{"id":"111111111111","shortId":"11111111","summary":"Stale"}
				]`), nil
			}
			aggregateStarted <- values["repo"].(string)
			return json.RawMessage(`[]`), nil
		},
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
	}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("feature")},
	}
	m.log = LogPane{visible: true, repo: first, branch: "HEAD"}

	m.RefreshLog()
	select {
	case <-singleStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("single-repository log did not start")
	}
	m.log.branch = ""
	m.enableVCSAggregateLog(first)
	m.RefreshLog()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case repo := <-aggregateStarted:
			seen[repo] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("aggregate log did not query every root: %v", seen)
		}
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("aggregate log did not finish")
	}
	if !m.log.aggregate || !m.log.loaded || m.log.loading || m.log.stale {
		t.Fatalf("aggregate log state = %+v", m.log)
	}
	generation := m.vcs.logGeneration

	close(releaseSingle)
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("stale single-repository response did not return")
	}
	if m.vcs.logGeneration != generation ||
		!m.log.aggregate || !m.log.loaded || m.log.loading || m.log.stale ||
		len(m.log.commits) != 0 {
		t.Fatalf("stale response changed the aggregate log: generation=%d pane=%+v",
			m.vcs.logGeneration, m.log)
	}
	select {
	case repo := <-aggregateStarted:
		t.Fatalf("stale response triggered an unbounded refresh for %q", repo)
	case <-time.After(100 * time.Millisecond):
	}
}
