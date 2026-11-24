package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestVCSRepositoryForPathUsesInnermostRoot(t *testing.T) {
	repositories := []vcsRepository{
		{Root: "/project"},
		{Root: "/project/orders"},
	}
	cases := map[string]string{
		"/project/App.kt":                  "/project",
		"/project/orders/Order.kt":         "/project/orders",
		"/project/orders/src/OrderLine.kt": "/project/orders",
		"/project/orders":                  "/project",
		"/project/orders/":                 "/project",
		"/elsewhere/App.kt":                "",
	}
	for path, want := range cases {
		if want != "" {
			want = filepath.Clean(want)
		}
		if got := vcsRepositoryForPath(path, repositories); got != want {
			t.Errorf("repository for %s = %q, want %q", path, got, want)
		}
	}
}

func TestVCSRepositoryArgForPathUsesInnermostRoot(t *testing.T) {
	m := &Model{vcs: VCSPane{
		current: "/project",
		repositories: []vcsRepository{
			{Root: "/project"},
			{Root: "/project/orders"},
		},
	}}
	if got := m.vcsRepositoryArgForPath("/project/orders/Order.kt"); got != "/project/orders" {
		t.Fatalf("path repository = %#v, want /project/orders", got)
	}
	if got := m.vcsRepositoryArgForPath("/outside/Order.kt"); got != "/project" {
		t.Fatalf("outside-path fallback repository = %#v, want the current repository", got)
	}
}

func TestOpenCommitFileResolvesSymlinkToInnermostRepository(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "orders")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repository, "Order.kt")
	if err := os.WriteFile(path, []byte("class Order\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "orders-link")
	if err := os.Symlink(repository, alias); err != nil {
		t.Fatal(err)
	}
	root = realPath(root)
	repository = realPath(repository)
	path = realPath(path)

	m := &Model{}
	m.setRoot(root)
	m.vcs.repositories = []vcsRepository{
		{Root: root, Head: str("main")},
		{Root: repository, Head: str("feature")},
	}
	m.vcs.current = root
	m.OpenCommitFile(filepath.Join(alias, "Order.kt"))
	if m.commit == nil {
		t.Fatal("Commit File did not open")
	}
	if m.commit.repo != repository {
		t.Fatalf("Commit File repository = %q, want %q", m.commit.repo, repository)
	}
	if m.commit.only != path {
		t.Fatalf("Commit File path = %q, want %q", m.commit.only, path)
	}
}

func TestOpenCommitDirSelectsExactNestedRepository(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "orders")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	root, nested = realPath(root), realPath(nested)

	m := &Model{}
	m.setRoot(root)
	m.vcs.repositories = []vcsRepository{
		{Root: root, Head: str("main")},
		{Root: nested, Head: str("feature")},
	}
	m.vcs.current = root
	m.OpenCommitDir(nested)
	if m.commit == nil {
		t.Fatal("Commit Directory did not open")
	}
	if m.commit.repo != nested {
		t.Fatalf("Commit Directory repository = %q, want %q", m.commit.repo, nested)
	}
	if m.commit.scope != nested {
		t.Fatalf("Commit Directory scope = %q, want %q", m.commit.scope, nested)
	}
}

func TestVCSRefreshDiscoversRepositoriesAndPreservesSelection(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "app")
	nested := filepath.Join(root, "orders")
	repositories := []vcsRepository{
		{Root: primary, Head: str("main"), HeadCommit: str("111111111111")},
		{Root: nested, Head: str("feature/ABC-123"), HeadCommit: str("222222222222")},
	}
	files := []openidea.FileStatus{
		{Path: filepath.Join(primary, "App.kt"), RelPath: "App.kt", Unstaged: str("modified")},
		{Path: filepath.Join(nested, "Order.kt"), RelPath: "Order.kt", Staged: str("added")},
	}
	rootsRaw := mustVCSJSON(t, repositories)
	filesRaw := mustVCSJSON(t, files)
	posted := make(chan struct{}, 4)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "vcs_roots":
				return rootsRaw, nil
			case "vcs_status":
				return filesRaw, nil
			case "vcs_branch_info":
				return json.RawMessage(`{"head":"feature/ABC-123","headCommit":"222222222222"}`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot(root)
	m.vcs.current = nested
	m.vcs.selectedFiles = map[string]string{nested: filepath.Join(nested, "Order.kt")}
	m.RefreshVCS()
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("repository discovery did not finish")
	}
	if !m.vcs.loaded || len(m.vcs.repositories) != 2 {
		t.Fatalf("VCS repositories = %+v, loaded=%v", m.vcs.repositories, m.vcs.loaded)
	}
	if m.vcs.current != nested {
		t.Fatalf("current repository = %q, want %q", m.vcs.current, nested)
	}
	if got := m.currentVCSFiles(); len(got) != 1 || got[0].Path != filepath.Join(nested, "Order.kt") {
		t.Fatalf("selected repository files = %+v", got)
	}
	if !m.vcsFileSelected(filepath.Join(nested, "Order.kt")) {
		t.Fatal("repository refresh dropped the selected changed file")
	}

	m.selectVCSFile(filepath.Join(nested, "Order.kt"))
	m.vcs.current = primary
	m.selectVCSFile(filepath.Join(primary, "App.kt"))
	m.vcs.current = nested
	if !m.vcsFileSelected(filepath.Join(nested, "Order.kt")) ||
		m.vcsFileSelected(filepath.Join(primary, "App.kt")) {
		t.Fatal("switching repository tabs did not preserve each file selection")
	}
}

func TestVCSBranchAndRemoteActionsUseCurrentRepository(t *testing.T) {
	first := "/project"
	current := "/project/orders"
	posted := make(chan struct{}, 4)
	remoteRepos := make(chan any, 2)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "vcs_roots":
				return mustVCSJSON(t, []vcsRepository{
					{Root: first, Head: str("main")},
					{Root: current, Head: str("feature")},
				}), nil
			case "vcs_status":
				return json.RawMessage(`[]`), nil
			case "vcs_branch_info":
				if repo := args.(map[string]any)["repo"]; repo != current {
					t.Errorf("branch repo = %#v, want %q", repo, current)
				}
				return json.RawMessage(`{"head":"feature","headCommit":"abcdef0123456789"}`), nil
			case "vcs_remote":
				remoteRepos <- args.(map[string]any)["repo"]
				return json.RawMessage(`null`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot("/project")
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: current, Head: str("feature")},
	}
	m.vcs.current = current
	m.RefreshBranch()
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("the selected repository branch did not load")
	}
	if !m.branch.loaded {
		t.Fatal("the selected repository branch did not load")
	}
	m.runVCSRemote(vcsOpPush, "Push", map[string]any{"op": "push"})
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("the selected repository push did not finish")
	}
	if m.vcsOps.busy {
		t.Fatal("the selected repository push did not finish")
	}
	select {
	case repo := <-remoteRepos:
		if repo != current {
			t.Fatalf("push repo = %#v, want %q", repo, current)
		}
	case <-time.After(time.Second):
		t.Fatal("Push did not reach the selected repository")
	}

	m.vcsOps = VCSOperationPane{}
	m.runVCSRemote(vcsOpPull, "Update", map[string]any{"op": "pull"})
	select {
	case repo := <-remoteRepos:
		if repo != current {
			t.Fatalf("update repo = %#v, want %q", repo, current)
		}
	case <-time.After(time.Second):
		t.Fatal("Update did not reach the selected repository")
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("the selected repository update did not finish")
	}
}

func TestVCSRepositorySwitchIgnoresStaleBranchResponse(t *testing.T) {
	first := "/project"
	current := "/project/orders"
	type branchResponse struct {
		raw string
		err error
	}
	responses := map[string]chan branchResponse{
		first:   make(chan branchResponse, 1),
		current: make(chan branchResponse, 1),
	}
	requested := make(chan string, 2)
	posted := make(chan struct{}, 2)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "vcs_branch_info" {
				return json.RawMessage(`null`), nil
			}
			repo := args.(map[string]any)["repo"]
			root, _ := repo.(string)
			requested <- root
			response := <-responses[root]
			return json.RawMessage(response.raw), response.err
		},
	}
	m.setRoot("/project")
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: current, Head: str("feature")},
	}
	m.vcs.current = first

	m.RefreshBranch()
	if repo := <-requested; repo != first {
		t.Fatalf("initial branch request used %q, want %q", repo, first)
	}
	m.SelectVCSRepository(current)
	if repo := <-requested; repo != current {
		t.Fatalf("switched branch request used %q, want %q", repo, current)
	}

	responses[first] <- branchResponse{
		raw: `{"head":"main","headCommit":"111111111111","upstream":null,"ahead":0,"behind":0,"operation":null}`,
	}
	<-posted
	if m.branch.head != nil {
		t.Fatalf("stale branch response selected %q", *m.branch.head)
	}

	responses[current] <- branchResponse{
		raw: `{"head":"feature","headCommit":"222222222222","upstream":null,"ahead":0,"behind":0,"operation":null}`,
	}
	<-posted
	if m.branch.head == nil || *m.branch.head != "feature" {
		t.Fatalf("current branch = %v, want feature", m.branch.head)
	}
}

func TestCommitCommitsEveryRepositoryWithCurrentLast(t *testing.T) {
	first := "/project"
	current := "/project/orders"
	m := &Model{}
	m.setRoot("/project")
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: current, Head: str("feature")},
	}
	m.vcs.current = current
	m.vcs.files = []openidea.FileStatus{
		{Path: first + "/App.kt", RelPath: "App.kt", Unstaged: str("modified")},
		{Path: current + "/Order.kt", RelPath: "Order.kt", Unstaged: str("modified")},
	}
	m.OpenCommit()
	if m.commit == nil || m.commit.repo != current {
		t.Fatalf("commit target = %+v, want %q", m.commit, current)
	}
	files := m.commitFiles(m.commit)
	if len(files) != 2 || files[0].Path != first+"/App.kt" ||
		files[1].Path != current+"/Order.kt" {
		t.Fatalf("commit files = %+v, want both repositories", files)
	}

	commitArgs := make(chan map[string]any, 2)
	posted := make(chan struct{}, 4)
	m.ctx = context.Background()
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "vcs_commit":
			commitArgs <- args.(map[string]any)
			return json.RawMessage(`"abc12345"`), nil
		case "vcs_roots":
			return mustVCSJSON(t, m.vcs.repositories), nil
		case "vcs_status":
			return json.RawMessage(`[]`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.commit.message = "Commit every repository"
	m.submitCommit()
	var calls []map[string]any
	for len(calls) < 2 {
		select {
		case args := <-commitArgs:
			calls = append(calls, args)
		case <-time.After(2 * time.Second):
			t.Fatalf("Commit reached %d of 2 repositories", len(calls))
		}
	}
	if got := []any{calls[0]["repo"], calls[1]["repo"]}; fmt.Sprint(got) != fmt.Sprint([]any{first, current}) {
		t.Fatalf("vcs_commit repository order = %#v, want selected repository last", got)
	}
	for i, want := range []string{first + "/App.kt", current + "/Order.kt"} {
		options, ok := calls[i]["options"].(openidea.CommitOptions)
		if !ok {
			t.Fatalf("vcs_commit options = %#v", calls[i]["options"])
		}
		if len(options.Stage) != 1 || options.Stage[0] != want {
			t.Fatalf("repository %d stage = %#v, want %q", i, options.Stage, want)
		}
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("Commit did not finish")
	}
	if !strings.Contains(m.status, "2 repositories") {
		t.Fatalf("commit status = %q", m.status)
	}
}

func TestVCSChangesDeleteUsesOpenIDEATrashFlow(t *testing.T) {
	files := []openidea.FileStatus{
		{Path: "/project/a.txt", RelPath: "a.txt", Unstaged: str("untracked")},
		{Path: "/project/b.txt", RelPath: "b.txt", Unstaged: str("modified")},
	}
	if got := vcsDeletableFiles(files); len(got) != 2 ||
		got[0] != "/project/a.txt" || got[1] != "/project/b.txt" {
		t.Fatalf("deletable files = %#v", got)
	}
	for _, deleted := range []openidea.FileStatus{
		{Path: "/project/deleted.txt", Unstaged: str("deleted")},
		{Path: "/project/staged.txt", Staged: str("deleted")},
	} {
		if got := vcsDeletableFiles([]openidea.FileStatus{deleted}); len(got) != 0 {
			t.Fatalf("already deleted file %#v became deletable: %#v", deleted, got)
		}
	}
	if got := vcsDeletableFiles([]openidea.FileStatus{
		{Path: "/project/a.txt", Unstaged: str("modified")},
		{Path: "/project/deleted.txt", Unstaged: str("deleted")},
	}); len(got) != 0 {
		t.Fatalf("mixed deletable/deleted selection = %#v, want no Delete action", got)
	}

	deletedPaths := make(chan string, 2)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
		},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "fs_delete":
				deletedPaths <- args.(map[string]any)["path"].(string)
			case "vcs_status":
				return json.RawMessage(`[]`), nil
			default:
				return json.RawMessage(`null`), nil
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot("/project")
	m.vcs.current = "/project"
	m.AskDeleteVCSFiles(files)
	if !m.vcsMore.open || m.vcsMore.kind != vcsMoreConfirm ||
		m.vcsMore.action != "changes.deleteFiles" {
		t.Fatalf("Delete confirmation = %+v", m.vcsMore)
	}
	if !strings.Contains(m.vcsMore.hint, "Delete 2 items?") ||
		!strings.Contains(m.vcsMore.hint, "Items are moved to the Trash.") {
		t.Fatalf("Delete hint = %q", m.vcsMore.hint)
	}
	m.submitVCSMore()
	for _, want := range []string{"/project/a.txt", "/project/b.txt"} {
		select {
		case got := <-deletedPaths:
			if got != want {
				t.Fatalf("fs_delete path = %q, want %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("fs_delete did not receive %q", want)
		}
	}
}

func TestVCSToolWindowRendersRepositoryTabsAndSwitchesThem(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	m.sideMode = "vcs"
	m.vcs = VCSPane{
		loaded:  true,
		current: "/project/orders",
		repositories: []vcsRepository{
			{Root: "/project", Head: str("main"), HeadCommit: str("111111111111")},
			{Root: "/project/orders", Head: str("feature/ABC-123"), HeadCommit: str("222222222222")},
		},
		files: []openidea.FileStatus{
			{Path: "/project/App.kt", RelPath: "App.kt", Unstaged: str("modified")},
			{Path: "/project/orders/Order.kt", RelPath: "Order.kt", Staged: str("added")},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsToolWindow(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 520, 520)
	tester.Frame()
	for _, text := range []string{
		"project", "orders", "main", "feature/ABC-123", "Order.kt",
		"Staged Changes", "Refresh", "Commit", "Update", "Push",
	} {
		if !tester.HasText(text) {
			t.Errorf("Git tool window is missing %q: %v", text, tester.Texts())
		}
	}
	if tester.HasText("App.kt") || tester.HasText("Unstaged Changes") {
		t.Fatal("the primary repository's change appeared under the orders tab")
	}
	rect, ok := tester.Find("project")
	if !ok {
		t.Fatalf("the primary repository tab is missing: %v", tester.Texts())
	}
	tester.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tester.Frame()
	if m.vcs.current != "/project" {
		t.Fatalf("clicking repository tab selected %q, want /project", m.vcs.current)
	}
	if !tester.HasText("App.kt") || !tester.HasText("Unstaged Changes") ||
		tester.HasText("Order.kt") || tester.HasText("Staged Changes") {
		t.Fatalf("repository tab did not switch its changes: %v", tester.Texts())
	}
}

func TestVCSToolWindowRendersNoRepositoryDetachedAndErrorStates(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		m := &Model{}
		m.setRoot("/project")
		m.vcs.loaded = true
		tester := vcsStateTester(m)
		if !tester.HasText("Version control is not configured") ||
			!tester.HasText("Open a Git repository to commit changes.") {
			t.Fatalf("no-repository state is missing: %v", tester.Texts())
		}
	})
	t.Run("detached", func(t *testing.T) {
		m := &Model{}
		m.setRoot("/project")
		m.vcs = VCSPane{
			loaded:       true,
			current:      "/project",
			repositories: []vcsRepository{{Root: "/project", HeadCommit: str("abcdef0123456789")}},
		}
		tester := vcsStateTester(m)
		if !tester.HasText("Detached at abcdef01") {
			t.Fatalf("detached-head state is missing: %v", tester.Texts())
		}
	})
	t.Run("error", func(t *testing.T) {
		m := &Model{}
		m.setRoot("/project")
		m.vcs.loaded = true
		m.vcs.err = "Could not discover Git repositories: backend unavailable"
		tester := vcsStateTester(m)
		if !tester.HasText(m.vcs.err) {
			t.Fatalf("repository error is missing: %v", tester.Texts())
		}
	})
}

func TestVCSRepositorySwitchRestoresBranchLogContentAndStagingState(t *testing.T) {
	first := "/project"
	current := "/project/orders"
	m := &Model{}
	m.setRoot("/project")
	m.vcs = VCSPane{
		current: first,
		repositories: []vcsRepository{
			{Root: first, Head: str("main")},
			{Root: current, Head: str("feature")},
		},
		content: map[string]vcsContentTab{first: vcsContentLog},
	}
	m.branch = BranchPane{loaded: true, head: str("main")}
	m.log = LogPane{
		visible: true, loaded: true, repo: first,
		commits: []openidea.Commit{{ID: "111111111111", ShortID: "11111111"}},
	}
	m.vcs.busyByRepo = map[string]bool{first: true}
	m.vcs.opErrByRepo = map[string]string{first: "first error"}

	m.SelectVCSRepository(current)
	if m.branch.head != nil || len(m.log.commits) != 0 || m.log.repo != current {
		t.Fatalf("new repository inherited the first view: branch=%+v log=%+v", m.branch, m.log)
	}
	if m.vcsRepositoryBusy(current) || m.vcsRepositoryOpErr(current) != "" {
		t.Fatal("the second repository inherited the first staging state")
	}
	m.branch = BranchPane{loaded: true, head: str("feature")}
	m.log.loaded = true
	m.log.commits = []openidea.Commit{{ID: "222222222222", ShortID: "22222222"}}
	m.SelectVCSTab(vcsContentWorktrees)

	m.SelectVCSRepository(first)
	if m.branch.head == nil || *m.branch.head != "main" {
		t.Fatalf("first branch = %v, want main", m.branch.head)
	}
	if len(m.log.commits) != 1 || m.log.commits[0].ID != "111111111111" {
		t.Fatalf("first log = %+v", m.log.commits)
	}
	if m.currentVCSContent() != vcsContentLog {
		t.Fatalf("first content tab = %v, want Log", m.currentVCSContent())
	}
	if !m.vcsRepositoryBusy(first) || m.vcsRepositoryOpErr(first) != "first error" {
		t.Fatal("first repository staging state was not restored")
	}

	m.SelectVCSRepository(current)
	if m.branch.head == nil || *m.branch.head != "feature" {
		t.Fatalf("second branch = %v, want feature", m.branch.head)
	}
	if len(m.log.commits) != 1 || m.log.commits[0].ID != "222222222222" {
		t.Fatalf("second log = %+v", m.log.commits)
	}
	if m.currentVCSContent() != vcsContentWorktrees {
		t.Fatalf("second content tab = %v, want Worktrees", m.currentVCSContent())
	}
}

func TestVCSRepositorySwitchIgnoresStaleLogResponse(t *testing.T) {
	first := "/project"
	current := "/project/orders"
	type response struct {
		raw string
		err error
	}
	responses := map[string]chan response{
		first:   make(chan response, 1),
		current: make(chan response, 1),
	}
	requested := make(chan string, 2)
	posted := make(chan struct{}, 2)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
	}
	var hook func(context.Context, string, any) (json.RawMessage, error)
	hook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "vcs_log" {
			return json.RawMessage(`null`), nil
		}
		repo, _ := args.(map[string]any)["repo"].(string)
		requested <- repo
		next := <-responses[repo]
		return json.RawMessage(next.raw), next.err
	}
	m.invokeHook = hook
	m.setRoot("/project")
	m.vcs = VCSPane{
		current: first,
		repositories: []vcsRepository{
			{Root: first, Head: str("main")},
			{Root: current, Head: str("feature")},
		},
	}
	// A branch-scoped log stays in one repository even when the tool window is
	// showing the project-wide aggregate by default.
	m.log = LogPane{visible: true, repo: first, branch: "HEAD"}

	m.RefreshLog()
	if repo := <-requested; repo != first {
		t.Fatalf("initial log request used %q, want %q", repo, first)
	}
	m.invokeHook = nil
	m.SelectVCSRepository(current)
	m.invokeHook = hook
	m.ensureVCSLogForRepository(current)
	if repo := <-requested; repo != current {
		t.Fatalf("switched log request used %q, want %q", repo, current)
	}

	responses[first] <- response{raw: `[]`}
	<-posted
	if m.log.repo != current || m.log.loaded {
		t.Fatalf("stale first log became active: %+v", m.log)
	}

	responses[current] <- response{raw: `[]`}
	<-posted
	if m.log.repo != current || !m.log.loaded {
		t.Fatalf("current log did not load: %+v", m.log)
	}
}

func TestVCSStatusRefreshGenerationDropsResponseAfterReset(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	posted := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "vcs_roots":
				close(started)
				<-release
			case "vcs_status":
				return json.RawMessage(`[]`), nil
			}
			return json.RawMessage(`[]`), nil
		},
	}
	m.setRoot("/project")
	m.RefreshVCS()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("status refresh did not start")
	}
	m.resetVCS()
	close(release)
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("stale status refresh did not return")
	}
	if m.vcs.loading || m.vcs.loaded || len(m.vcs.repositories) != 0 || m.vcs.err != "" {
		t.Fatalf("stale status refresh changed the reset pane: %+v", m.vcs)
	}
}

func TestVCSToolWindowContentTabsAreRepositoryScoped(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	m.sideMode = "vcs"
	m.vcs = VCSPane{
		loaded:  true,
		current: "/project/orders",
		repositories: []vcsRepository{
			{Root: "/project", Head: str("main")},
			{Root: "/project/orders", Head: str("feature")},
		},
		content: map[string]vcsContentTab{
			"/project":        vcsContentChanges,
			"/project/orders": vcsContentWorktrees,
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsToolWindow(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 520, 520)
	tester.Frame()
	for _, text := range []string{
		"Local Changes", "Branches", "Shelf", "Stash", "Log", "Worktrees",
		"New…", "No worktrees",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Git content tabs are missing %q: %v", text, tester.Texts())
		}
	}
	rect, ok := tester.Find("project")
	if !ok {
		t.Fatalf("primary repository tab is missing: %v", tester.Texts())
	}
	tester.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tester.Frame()
	if m.currentVCSContent() != vcsContentChanges || !tester.HasText("No local changes") {
		t.Fatalf("repository content selection was not restored: %v", tester.Texts())
	}
	if err := tester.Click("Worktrees"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if m.currentVCSContent() != vcsContentWorktrees || !tester.HasText("No worktrees") {
		t.Fatalf("Worktrees tab did not open: %v", tester.Texts())
	}
	rect, ok = tester.Find("orders")
	if !ok {
		t.Fatalf("second repository tab is missing: %v", tester.Texts())
	}
	tester.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tester.Frame()
	if m.currentVCSContent() != vcsContentWorktrees || !tester.HasText("No worktrees") {
		t.Fatalf("repository did not restore its Worktrees tab: %v", tester.Texts())
	}
}

func TestVCSBranchFavoritesUseCurrentRepositoryScope(t *testing.T) {
	first := "/project"
	second := "/project/orders"
	m := &Model{}
	m.setRoot("/project")
	m.vcs = VCSPane{
		current: second,
		repositories: []vcsRepository{
			{Root: first, Head: str("main")},
			{Root: second, Head: str("feature")},
		},
	}
	m.prefs.VCS.FavoriteBranches = map[string]bool{
		gitLogScopedKey(first, "main"):  true,
		gitLogScopedKey(second, "main"): true,
	}
	if !m.branchFavorite("main") {
		t.Fatal("current repository favorite was not found")
	}
	delete(m.prefs.VCS.FavoriteBranches, gitLogScopedKey(second, "main"))
	if m.branchFavorite("main") {
		t.Fatal("another repository's favorite leaked into the current branch popup")
	}
}

func vcsStateTester(m *Model) *ui.Tester {
	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsToolWindow(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 520, 420)
	tester.Frame()
	return tester
}

func mustVCSJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
