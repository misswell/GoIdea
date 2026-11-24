package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestVCSRevisionChoicesExcludeCurrentAndListTags(t *testing.T) {
	upstream := "origin/main"
	m := &Model{}
	choices := m.vcsRevisionChoices([]openidea.Branch{
		{Name: "main", Current: true},
		{Name: "origin/main", Remote: true},
		{Name: "feature", Upstream: &upstream},
	}, []string{"v1.0"})

	if len(choices) != 2 {
		t.Fatalf("choices = %#v, want feature and v1.0", choices)
	}
	if choices[0].value != "feature" || choices[0].detail != "Branch · tracks origin/main" {
		t.Fatalf("branch choice = %#v", choices[0])
	}
	if choices[1].value != "v1.0" || choices[1].detail != "Tag" {
		t.Fatalf("tag choice = %#v", choices[1])
	}
}

func TestVCSBranchAndTagNeedNames(t *testing.T) {
	for _, kind := range []string{vcsOpNewBranch, vcsOpNewTag} {
		m := &Model{vcsOps: VCSOperationPane{kind: kind, open: true, name: "  "}}
		m.submitVCSOperation()
		if m.vcsOps.err == "" {
			t.Fatalf("%s accepted an empty name", kind)
		}
	}
}

func TestVCSHardResetNeedsConfirmation(t *testing.T) {
	m := &Model{vcsOps: VCSOperationPane{
		kind:      vcsOpReset,
		open:      true,
		resetMode: 2,
		choices:   []vcsOperationChoice{{value: "HEAD~1"}},
	}}
	m.submitVCSOperation()
	if m.vcsOps.err == "" {
		t.Fatal("hard reset ran without the discard confirmation")
	}
	if m.vcsOps.busy {
		t.Fatal("hard reset started while it still needed confirmation")
	}
}

func TestVCSLogOperationDialogsKeepCommitRepository(t *testing.T) {
	first := "/project/orders"
	second := "/project/catalog"
	m := &Model{log: LogPane{
		visible:     true,
		aggregate:   true,
		repo:        first,
		selected:    0,
		commitRepos: []string{second},
		commits: []openidea.Commit{{
			ID: "222222222222", ShortID: "22222222", Summary: "Catalog commit",
		}},
	}}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("feature")},
	}

	m.NewTagAtCommit(map[string]any{"id": "222222222222", "repo": second})
	if m.vcsOps.repo != second {
		t.Fatalf("new tag repository = %q, want %q", m.vcsOps.repo, second)
	}
	m.OpenGitResetAtSelectedCommit()
	if m.vcsOps.repo != second {
		t.Fatalf("reset repository = %q, want %q", m.vcsOps.repo, second)
	}
	m.OpenGitPushAtSelectedCommit()
	if m.vcsOps.repo != second {
		t.Fatalf("push repository = %q, want %q", m.vcsOps.repo, second)
	}
}

func TestVCSOperationsQuickListFiltersAndDispatches(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.vcs.current = m.Root()
	m.vcsQuick = vcsQuickList{open: true}
	ids := map[string]bool{}
	for _, row := range m.vcsQuickRows() {
		ids[row.id] = true
	}
	for _, id := range []string{
		"vcs.manageRemotes", "vcs.compareWithBranch", "vcs.checkoutRevision",
		"vcs.worktrees", "vcs.newWorktree", "vcs.resolveConflicts",
	} {
		if !ids[id] {
			t.Errorf("VCS Operations quick list misses %s", id)
		}
	}
	m.vcsQuick = vcsQuickList{open: true, query: "show git log"}
	rows := m.vcsQuickRows()
	if len(rows) != 1 || rows[0].id != "vcs.log" {
		t.Fatalf("filtered VCS operations = %+v", rows)
	}
	m.activateVCSQuick()
	if m.vcsQuick.open {
		t.Fatal("the quick list stayed open after dispatch")
	}
	if !m.log.visible {
		t.Fatal("the Git Log action did not open")
	}
}

func TestVCSMultiRepoMergeAndRebaseRollbackOnlyCompletedRepositories(t *testing.T) {
	for _, kind := range []string{vcsOpMerge, vcsOpRebase} {
		t.Run(kind, func(t *testing.T) {
			project := vcsMultiRepoProject(t, 6)
			m, ctx, repositories := vcsMultiRepoModel(t, project, 6)
			target := "feature"

			for i, repository := range repositories {
				switch {
				case i < 2 || i == 3 || i == 4:
					vcsMultiRepoAdvanceFeature(t, repository.Root, fmt.Sprintf("%s-%d.txt", kind, i))
				case i == 2:
					vcsMultiRepoConflictFeature(t, repository.Root, kind)
				case i == 5:
					vcsMultiRepoGit(t, repository.Root, "switch", "-q", "main")
					vcsMultiRepoGit(t, repository.Root, "branch", "-D", target)
				}
			}

			before := make(map[string]string, len(repositories))
			for _, repository := range repositories {
				before[repository.Root] = vcsMultiRepoHead(t, repository.Root)
			}
			options := map[string]any{
				"noFf": false, "ffOnly": false, "squash": false,
				"noCommit": false, "message": nil,
			}
			if kind == vcsOpRebase {
				options = map[string]any{
					"onto": nil, "rebaseMerges": false, "keepEmpty": false,
				}
			}
			result := m.executeVCSMultiRepoHistory(ctx, kind, target, options)
			wantAttempted := []string{
				repositories[0].Root, repositories[1].Root, repositories[2].Root,
			}
			if !vcsMultiRepoSamePaths(result.attempted, wantAttempted) {
				t.Fatalf("%s attempted %v, want %v", kind, result.attempted, wantAttempted)
			}
			if len(result.completed) != 2 ||
				vcsRepositoryKey(result.completed[0].root) != vcsRepositoryKey(repositories[0].Root) ||
				vcsRepositoryKey(result.completed[1].root) != vcsRepositoryKey(repositories[1].Root) {
				t.Fatalf("%s completed %+v", kind, result.completed)
			}
			if len(result.failures) != 1 ||
				vcsRepositoryKey(result.failures[0].root) != vcsRepositoryKey(repositories[2].Root) {
				t.Fatalf("%s failures %+v", kind, result.failures)
			}
			for _, change := range result.completed {
				if change.before == change.after {
					t.Fatalf("%s did not move HEAD in %s", kind, change.root)
				}
				if got := vcsMultiRepoHead(t, change.root); got != change.after {
					t.Fatalf("%s %s HEAD = %s, want %s", kind, change.root, got, change.after)
				}
			}
			for _, repository := range repositories[3:] {
				if got := vcsMultiRepoHead(t, repository.Root); got != before[repository.Root] {
					t.Fatalf("%s touched a repository after the failure: %s", kind, repository.Root)
				}
			}
			if info := vcsMultiRepoBranchInfo(t, ctx, m, repositories[2].Root); info.Operation == nil {
				t.Fatalf("%s conflict was not left in progress", kind)
			}
			failedHead := vcsMultiRepoHead(t, repositories[2].Root)

			rollback := &vcsMultiRepoRollbackState{
				kind: kind, target: target,
				failureRoot: repositories[2].Root,
				failureText: result.failures[0].err.Error(),
				completed:   append([]vcsRepoHeadChange(nil), result.completed...),
			}
			m.vcsOps = VCSOperationPane{
				kind: kind, open: true, allRepositories: true,
				title: "Merge", request: m.nextRequest(),
				multiRepoRollback: rollback,
			}
			if kind == vcsOpRebase {
				m.vcsOps.title = "Rebase"
			}
			tester := ui.NewTester(func(c *ui.Context) {
				m.vcsOperationDialog(c, c.Theme())
			}, 760, 620)
			tester.Frame()
			for _, text := range []string{
				"Rollback", "Keep",
				"The failed repository is left as it is",
			} {
				if !tester.HasText(text) {
					t.Fatalf("%s rollback prompt misses %q: %v", kind, text, tester.Texts())
				}
			}
			if err := tester.Click("Rollback"); err != nil {
				t.Fatal(err)
			}
			vcsMultiRepoWait(t, func() bool {
				return m.vcsOps.multiRepoRollback == nil && !m.vcsOps.open
			})

			for _, change := range result.completed {
				if got := vcsMultiRepoHead(t, change.root); got != change.before {
					t.Fatalf("%s rollback restored %s to %s, want %s",
						kind, change.root, got, change.before)
				}
			}
			if got := vcsMultiRepoHead(t, repositories[2].Root); got != failedHead {
				t.Fatalf("%s rollback moved the failed repository: HEAD=%s want=%s completed=%+v",
					kind, got, failedHead, result.completed)
			}
			if info := vcsMultiRepoBranchInfo(t, ctx, m, repositories[2].Root); info.Operation == nil {
				t.Fatalf("%s rollback aborted the failed repository", kind)
			}
		})
	}
}

func TestVCSMultiRepoRevisionChoicesIncludeEveryRepository(t *testing.T) {
	project := vcsMultiRepoProject(t, 3)
	m, ctx, repositories := vcsMultiRepoModel(t, project, 3)
	vcsMultiRepoGit(t, repositories[1].Root, "branch", "topic-second")
	vcsMultiRepoGit(t, repositories[2].Root, "tag", "release-third")

	choices, err := m.loadVCSAllRevisionChoices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int, len(choices))
	for _, choice := range choices {
		counts[choice.value]++
	}
	for _, target := range []string{"feature", "topic-second", "release-third"} {
		if counts[target] != 1 {
			t.Fatalf("revision choice %q count = %d, choices = %+v", target, counts[target], choices)
		}
	}
}

func TestVCSMultiRepoPushResolvesEveryTargetAndKeepsGoingAfterFailure(t *testing.T) {
	project := vcsMultiRepoProject(t, 3)
	m, ctx, repositories := vcsMultiRepoModel(t, project, 3)

	firstRemote := vcsMultiRepoBareRemote(t)
	vcsMultiRepoPreparePush(t, repositories[0].Root, "topic", "review", firstRemote)
	vcsMultiRepoPrepareBrokenPush(t, repositories[1].Root)
	thirdRemote := vcsMultiRepoBareRemote(t)
	vcsMultiRepoPreparePush(t, repositories[2].Root, "work", "release", thirdRemote)

	result := m.executeVCSMultiRepoPush(ctx, m.nextRequest(), m.nextChannel(), false, true)
	if !vcsMultiRepoSamePaths(result.attempted, []string{
		repositories[0].Root, repositories[1].Root, repositories[2].Root,
	}) {
		t.Fatalf("push order %v", result.attempted)
	}
	if len(result.failures) != 1 ||
		vcsRepositoryKey(result.failures[0].root) != vcsRepositoryKey(repositories[1].Root) {
		t.Fatalf("push failures %+v", result.failures)
	}
	if len(result.completed) != 2 {
		t.Fatalf("push completed %+v", result.completed)
	}
	want := []vcsPushRepositoryPlan{
		{root: repositories[0].Root, branch: "topic", remote: "origin", target: "review"},
		{root: repositories[2].Root, branch: "work", remote: "origin", target: "release"},
	}
	remoteDirectories := []string{firstRemote, thirdRemote}
	for i, plan := range result.completed {
		if vcsRepositoryKey(plan.root) != vcsRepositoryKey(want[i].root) ||
			plan.branch != want[i].branch || plan.remote != want[i].remote ||
			plan.target != want[i].target {
			t.Fatalf("push plan %d = %+v, want %+v", i, plan, want[i])
		}
		head := vcsMultiRepoHead(t, plan.root)
		remoteHead := vcsMultiRepoGit(t, remoteDirectories[i], "rev-parse", plan.target)
		if remoteHead != head {
			t.Fatalf("remote %s/%s = %s, want %s", plan.remote, plan.target, remoteHead, head)
		}
	}
}

func TestVCSRemoteAllRepositoriesAndExplicitRepositoryFetch(t *testing.T) {
	project := vcsMultiRepoProject(t, 2)
	m, ctx, repositories := vcsMultiRepoModel(t, project, 2)
	remotes := make([]string, len(repositories))
	for i, repository := range repositories {
		remotes[i] = vcsMultiRepoBareRemote(t)
		vcsMultiRepoGit(t, repository.Root, "remote", "add", "origin", remotes[i])
		vcsMultiRepoGit(t, repository.Root, "push", "-q", "-u", "origin", "main")
	}

	vcsMultiRepoAdvanceRemote(t, remotes[0], "remote-one.txt")
	vcsMultiRepoAdvanceRemote(t, remotes[1], "remote-two.txt")
	secondBefore := vcsMultiRepoGit(t, repositories[1].Root,
		"rev-parse", "refs/remotes/origin/main")

	fetched := m.executeVCSRemoteRepositories(
		ctx, m.nextRequest(), m.nextChannel(), vcsOpFetch,
		map[string]any{"op": "fetch"}, []string{repositories[0].Root},
	)
	if len(fetched.completed) != 1 || len(fetched.failures) != 0 {
		t.Fatalf("explicit fetch = %+v", fetched)
	}
	firstFetched := vcsMultiRepoGit(t, repositories[0].Root,
		"rev-parse", "refs/remotes/origin/main")
	if firstFetched != vcsMultiRepoGit(t, remotes[0], "rev-parse", "main") {
		t.Fatalf("first repository was not fetched")
	}
	secondAfterFetch := vcsMultiRepoGit(t, repositories[1].Root,
		"rev-parse", "refs/remotes/origin/main")
	if secondAfterFetch != secondBefore {
		t.Fatalf("explicit fetch also updated %s", repositories[1].Root)
	}

	fetchedAll := m.executeVCSRemoteRepositories(
		ctx, m.nextRequest(), m.nextChannel(), vcsOpFetch,
		map[string]any{"op": "fetch"},
		[]string{repositories[0].Root, repositories[1].Root},
	)
	if len(fetchedAll.completed) != 2 || len(fetchedAll.failures) != 0 {
		t.Fatalf("all-repository fetch = %+v", fetchedAll)
	}
	for i, repository := range repositories {
		localRemote := vcsMultiRepoGit(t, repository.Root,
			"rev-parse", "refs/remotes/origin/main")
		remote := vcsMultiRepoGit(t, remotes[i], "rev-parse", "main")
		if localRemote != remote {
			t.Fatalf("repository %d remote ref = %s, remote = %s", i, localRemote, remote)
		}
	}

	updated := m.executeVCSRemoteRepositories(
		ctx, m.nextRequest(), m.nextChannel(), vcsOpPull,
		map[string]any{"op": "pull", "rebase": false},
		[]string{repositories[0].Root, repositories[1].Root},
	)
	if len(updated.completed) != 2 || len(updated.failures) != 0 {
		t.Fatalf("all-repository update = %+v", updated)
	}
	for i, repository := range repositories {
		local := vcsMultiRepoHead(t, repository.Root)
		remote := vcsMultiRepoGit(t, remotes[i], "rev-parse", "main")
		if local != remote {
			t.Fatalf("repository %d HEAD = %s, remote = %s", i, local, remote)
		}
	}
}

func vcsMultiRepoProject(t *testing.T, count int) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	project := t.TempDir()
	vcsMultiRepoInit(t, project)
	for i := 1; i < count; i++ {
		vcsMultiRepoInit(t, filepath.Join(project, fmt.Sprintf("repo-%d", i)))
	}
	return project
}

func vcsMultiRepoInit(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, root, "init", "-q", "-b", "main")
	vcsMultiRepoGit(t, root, "config", "user.name", "GoIDEA VCS Test")
	vcsMultiRepoGit(t, root, "config", "user.email", "vcs-test@invalid")
	if err := os.WriteFile(
		filepath.Join(root, "README.txt"),
		[]byte(filepath.Base(root)+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, root, "add", "README.txt")
	vcsMultiRepoGit(t, root, "commit", "-q", "-m", "baseline")
	vcsMultiRepoGit(t, root, "branch", "feature")
}

func vcsMultiRepoModel(
	t *testing.T,
	project string,
	wantRepositories int,
) (*Model, context.Context, []vcsRepository) {
	t.Helper()
	server := os.Getenv("GOIDEA_SERVER")
	if server == "" {
		server, _ = filepath.Abs(filepath.Join("..", "..", "resources", "openidea-server"))
	}
	info, err := os.Stat(server)
	if err != nil || info.IsDir() {
		t.Skip("GOIDEA_SERVER is not set and resources/openidea-server is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	t.Cleanup(cancel)
	manager := openidea.NewManager(openidea.LaunchConfig{
		ServerPath: server, Home: t.TempDir(), Log: io.Discard,
		StartupTimeout: 30 * time.Second,
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.Invoke(ctx, Window, "open_project", map[string]any{
		"path": project,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": project, "trusted": true, "folder": true,
	}); err != nil {
		t.Fatal(err)
	}
	repositories := vcsMultiRepoWaitRepositories(t, ctx, manager, wantRepositories)
	m := &Model{
		ctx: ctx, mgr: manager,
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(project)
	m.vcs.loaded = true
	m.vcs.repositories = append([]vcsRepository(nil), repositories...)
	m.vcs.current = repositories[0].Root
	return m, ctx, repositories
}

func vcsMultiRepoWaitRepositories(
	t *testing.T,
	ctx context.Context,
	manager *openidea.Manager,
	want int,
) []vcsRepository {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		raw, err := manager.Invoke(ctx, Window, "vcs_roots", nil)
		if err == nil {
			var repositories []vcsRepository
			if err := json.Unmarshal(raw, &repositories); err == nil && len(repositories) >= want {
				return repositories
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("vcs_roots did not discover %d repositories: %v", want, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func vcsMultiRepoBranchInfo(
	t *testing.T,
	ctx context.Context,
	m *Model,
	root string,
) openidea.BranchInfo {
	t.Helper()
	raw, err := m.invoke(ctx, "vcs_branch_info", map[string]any{
		"repo": vcsRepositoryArgFor(root),
	})
	if err != nil {
		t.Fatal(err)
	}
	var info openidea.BranchInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	return info
}

func vcsMultiRepoAdvanceFeature(t *testing.T, root, name string) {
	t.Helper()
	vcsMultiRepoGit(t, root, "switch", "-q", "feature")
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, root, "add", name)
	vcsMultiRepoGit(t, root, "commit", "-q", "-m", "feature change")
	vcsMultiRepoGit(t, root, "switch", "-q", "main")
}

func vcsMultiRepoConflictFeature(t *testing.T, root, kind string) {
	t.Helper()
	vcsMultiRepoGit(t, root, "switch", "-q", "feature")
	if err := os.WriteFile(
		filepath.Join(root, "conflict.txt"),
		[]byte("feature "+kind+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, root, "add", "conflict.txt")
	vcsMultiRepoGit(t, root, "commit", "-q", "-m", "feature conflict")
	vcsMultiRepoGit(t, root, "switch", "-q", "main")
	if err := os.WriteFile(
		filepath.Join(root, "conflict.txt"),
		[]byte("main "+kind+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, root, "add", "conflict.txt")
	vcsMultiRepoGit(t, root, "commit", "-q", "-m", "main conflict")
}

func vcsMultiRepoBareRemote(t *testing.T) string {
	t.Helper()
	remote := t.TempDir()
	vcsMultiRepoGit(t, remote, "init", "-q", "--bare", "-b", "main")
	return remote
}

func vcsMultiRepoPreparePush(
	t *testing.T,
	root, branch, target, remote string,
) {
	t.Helper()
	vcsMultiRepoGit(t, root, "branch", "-m", branch)
	vcsMultiRepoGit(t, root, "remote", "add", "origin", remote)
	vcsMultiRepoGit(t, root, "push", "-q", "-u", "origin", branch+":"+target)
	path := filepath.Join(root, "push.txt")
	if err := os.WriteFile(path, []byte(branch+" push\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, root, "add", "push.txt")
	vcsMultiRepoGit(t, root, "commit", "-q", "-m", "push "+branch)
}

func vcsMultiRepoPrepareBrokenPush(t *testing.T, root string) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "missing.git")
	vcsMultiRepoGit(t, root, "remote", "add", "origin", missing)
	head := vcsMultiRepoHead(t, root)
	vcsMultiRepoGit(t, root, "update-ref", "refs/remotes/origin/main", head)
	vcsMultiRepoGit(t, root, "branch", "--set-upstream-to=origin/main", "main")
	path := filepath.Join(root, "broken-push.txt")
	if err := os.WriteFile(path, []byte("broken push\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, root, "add", "broken-push.txt")
	vcsMultiRepoGit(t, root, "commit", "-q", "-m", "broken push")
}

func vcsMultiRepoAdvanceRemote(t *testing.T, remote, name string) {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	vcsMultiRepoGit(t, filepath.Dir(clone), "clone", "-q", remote, clone)
	vcsMultiRepoGit(t, clone, "config", "user.name", "GoIDEA VCS Test")
	vcsMultiRepoGit(t, clone, "config", "user.email", "vcs-test@invalid")
	if err := os.WriteFile(filepath.Join(clone, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vcsMultiRepoGit(t, clone, "add", name)
	vcsMultiRepoGit(t, clone, "commit", "-q", "-m", "remote "+name)
	vcsMultiRepoGit(t, clone, "push", "-q", "origin", "main")
}

func vcsMultiRepoHead(t *testing.T, root string) string {
	t.Helper()
	return vcsMultiRepoGit(t, root, "rev-parse", "HEAD")
}

func vcsMultiRepoGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	commandArgs := []string{
		"-C", root,
		"-c", "user.name=GoIDEA VCS Test",
		"-c", "user.email=vcs-test@invalid",
	}
	commandArgs = append(commandArgs, args...)
	command := exec.Command("git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %v: %v\n%s", root, args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vcsMultiRepoSamePaths(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if vcsRepositoryKey(got[i]) != vcsRepositoryKey(want[i]) {
			return false
		}
	}
	return true
}

func vcsMultiRepoWait(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the VCS operation")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
