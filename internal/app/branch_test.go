package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func branchModel() *Model {
	m := &Model{}
	m.setRoot("/tmp/branch-project")
	return m
}

func TestBranchLabelNamesTheHeadAndItsDistance(t *testing.T) {
	m := branchModel()
	m.branch.head = str("main")
	m.branch.upstream = str("origin/main")
	m.branch.ahead, m.branch.behind = 2, 3
	if got := m.branchLabel(); got != "main ↓3 ↑2" {
		t.Fatalf("label = %q, want main with both counts", got)
	}
	m.branch.ahead, m.branch.behind = 0, 0
	if got := m.branchLabel(); got != "main" {
		t.Fatalf("label = %q, want just the branch", got)
	}
}

func TestBranchLabelFallsBackToTheDetachedCommit(t *testing.T) {
	m := branchModel()
	m.branch.headCommit = str("0123456789abcdef")
	if got := m.branchLabel(); got != "01234567" {
		t.Fatalf("label = %q, want the short commit", got)
	}
}

func TestBranchLabelWordsAnOperation(t *testing.T) {
	m := branchModel()
	m.branch.head = str("main")
	m.branch.operation = str("rebase")
	if got := m.branchLabel(); got != "Rebasing main" {
		t.Fatalf("label = %q, want the rebase wording", got)
	}
	m.branch.head = nil
	if got := m.branchLabel(); got != "Rebasing" {
		t.Fatalf("label = %q, want the bare operation", got)
	}
}

func TestBranchRowsFilterAndWrap(t *testing.T) {
	m := branchModel()
	m.branch.branches = []openidea.Branch{
		{Name: "main", Current: true},
		{Name: "feature/login", Upstream: str("origin/feature/login")},
		{Name: "origin/main", Remote: true},
	}
	if got := len(m.branchRows()); got != 3 {
		t.Fatalf("unfiltered rows = %d, want 3", got)
	}
	m.branch.query = "FEATURE"
	rows := m.branchRows()
	if len(rows) != 1 || rows[0].Name != "feature/login" {
		t.Fatalf("filtered rows = %+v, want only feature/login", rows)
	}
	m.branch.query = ""
	// Down past the last row wraps to the first; up from the first wraps back.
	m.stepBranch(len(m.branch.branches) - 1)
	m.stepBranch(1)
	if m.branch.sel != 0 {
		t.Fatalf("sel = %d after wrapping down, want 0", m.branch.sel)
	}
	m.stepBranch(-1)
	if m.branch.sel != len(m.branch.branches)-1 {
		t.Fatalf("sel = %d after wrapping up, want %d", m.branch.sel, len(m.branch.branches)-1)
	}
}

func TestCurrentBranchRowIsTheStartingPoint(t *testing.T) {
	m := branchModel()
	m.branch.branches = []openidea.Branch{
		{Name: "main"},
		{Name: "dev", Current: true},
	}
	if got := m.currentBranchRow(); got != 1 {
		t.Fatalf("current row = %d, want 1", got)
	}
}

func TestCheckingOutTheCurrentBranchOnlyClosesThePopup(t *testing.T) {
	m := branchModel()
	m.branch.head = str("main")
	m.branch.open = true
	m.checkoutBranch("main")
	if m.branch.open {
		t.Fatal("checking out the current branch left the popup open")
	}
	if m.branch.busy {
		t.Fatal("checking out the current branch asked the backend")
	}
}

func TestBranchesDoNothingWithoutAProject(t *testing.T) {
	m := &Model{}
	m.ToggleBranches()
	if m.branch.open {
		t.Fatal("the popup opened without a project")
	}
}

func TestBranchPromptRendersNativeRenameControls(t *testing.T) {
	m := branchModel()
	m.branch.open = true
	m.branch.branches = []openidea.Branch{{
		Name: "main", Current: true, Upstream: str("origin/main"),
	}}
	m.startBranchRename(m.branch.branches[0])
	tester := ui.NewTester(func(c *ui.Context) {
		m.branchesDialog(c, c.Theme())
	}, 760, 520)
	tester.Frame()
	for _, text := range []string{
		"Rename Branch main", "New branch name:", "Unset upstream branch", "Rename", "Cancel",
	} {
		if !tester.HasText(text) {
			t.Fatalf("branch rename prompt is missing %q: %v", text, tester.Texts())
		}
	}
}

func TestBranchRenamePromptUsesOpenIDEANameValidation(t *testing.T) {
	branches := []openidea.Branch{
		{Name: "main"},
		{Name: "topic"},
		{Name: "origin/taken", Remote: true},
	}
	cases := map[string]string{
		"":             "Branch name must not be empty",
		"topic":        "Branch 'topic' already exists",
		"bad name":     "Invalid branch name",
		"topic..copy":  "Invalid branch name",
		"feature/x":    "",
		"origin/taken": "",
	}
	for name, want := range cases {
		if got := branchNameProblem(name, branches, "main"); got != want {
			t.Fatalf("branchNameProblem(%q) = %q, want %q", name, got, want)
		}
	}
	if got := branchNameProblem("main", branches, "main"); got != "Enter a new name" {
		t.Fatalf("unchanged branch validation = %q", got)
	}
}

func TestBranchPromptRenameAndRemoteDeleteState(t *testing.T) {
	m := branchModel()
	local := openidea.Branch{Name: "topic", Upstream: str("origin/topic")}
	m.startBranchRename(local)
	if m.branch.prompt != "rename" || m.branch.promptName != "topic" ||
		m.branch.promptBranch.Name != "topic" || m.branch.promptUnset {
		t.Fatalf("rename prompt = %+v", m.branch)
	}
	m.branch.promptName = "renamed"
	m.branch.promptUnset = true
	m.cancelBranchPrompt()
	if m.branch.prompt != "" || m.branch.promptName != "" || m.branch.promptUnset {
		t.Fatalf("cancel left branch prompt state behind: %+v", m.branch)
	}

	remote := openidea.Branch{Name: "origin/topic", Remote: true}
	m.startRemoteBranchDelete(remote)
	if m.branch.prompt != "delete-remote" || m.branch.promptBranch.Name != "origin/topic" {
		t.Fatalf("remote delete prompt = %+v", m.branch)
	}
	if got := shortRemoteBranchName(remote.Name); got != "topic" {
		t.Fatalf("short remote branch = %q, want topic", got)
	}
}

func TestBranchCheckoutAndRebaseRunsTheTwoBackendSteps(t *testing.T) {
	calls := make(chan string, 2)
	done := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			select {
			case done <- struct{}{}:
			default:
			}
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command == "vcs_checkout" || command == "vcs_rebase_with" {
				calls <- command
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(t.TempDir())
	m.checkoutAndRebaseBranch("topic", "main")
	for _, want := range []string{"vcs_checkout", "vcs_rebase_with"} {
		select {
		case got := <-calls:
			if got != want {
				t.Fatalf("backend call = %q, want %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s", want)
		}
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("checkout and rebase completion did not post")
	}
	if m.branch.busy {
		t.Fatal("checkout and rebase left the branch busy")
	}
}

func TestBranchMergeAndRebasePreselectTheBranchArg(t *testing.T) {
	m := branchModel()
	m.ctx = context.Background()
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
		return json.RawMessage(`[]`), nil
	}
	m.OpenGitMergeAt("topic")
	if !m.vcsOps.open || m.vcsOps.kind != vcsOpMerge || m.vcsOps.preferredStart != "topic" {
		t.Fatalf("merge state = %+v", m.vcsOps)
	}
	m.OpenGitRebaseAt("release")
	if !m.vcsOps.open || m.vcsOps.kind != vcsOpRebase || m.vcsOps.preferredStart != "release" {
		t.Fatalf("rebase state = %+v", m.vcsOps)
	}
}

func TestDeletingAnUnmergedBranchOffersRestoreAndViewCommits(t *testing.T) {
	root := t.TempDir()
	deleted := openidea.DeletedBranch{
		Name: "topic", Tip: "0123456789abcdef", Upstream: str("origin/topic"), Unmerged: true,
	}
	raw, err := json.Marshal(deleted)
	if err != nil {
		t.Fatal(err)
	}
	posted := make(chan struct{}, 1)
	restored := make(chan map[string]any, 1)
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
			switch command {
			case "vcs_delete_branch":
				return raw, nil
			case "vcs_restore_branch":
				restored <- args.(map[string]any)
				return json.RawMessage(`null`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot(root)
	m.branch.branches = []openidea.Branch{{Name: "topic"}}
	m.branch.open = true
	m.deleteLocalBranch(openidea.Branch{Name: "topic"})
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("delete branch completion did not post")
	}

	var notification *ideNotification
	for i := range m.notify.items {
		if m.notify.items[i].key == "vcs.deleted.topic" {
			notification = &m.notify.items[i]
		}
	}
	if notification == nil {
		t.Fatal("deleted branch notification was not shown")
	}
	if !strings.Contains(notification.body, "Unmerged commits") {
		t.Fatalf("notification body = %q", notification.body)
	}
	labels := make([]string, 0, len(notification.actions))
	for _, action := range notification.actions {
		labels = append(labels, action.label)
	}
	if strings.Join(labels, ",") != "Restore,View Commits,Delete Tracked Branch" {
		t.Fatalf("notification actions = %v", labels)
	}
	notification.actions[0].run(m)
	select {
	case args := <-restored:
		if _, ok := args["deleted"]; !ok {
			t.Fatalf("restore args = %#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restore branch was not invoked")
	}

	key := gitLogScopedKey(filepath.Clean(root), "topic")
	if m.prefs.VCS.FavoriteBranches == nil {
		m.prefs.VCS.FavoriteBranches = map[string]bool{}
	}
	m.prefs.VCS.FavoriteBranches[key] = true
	if !m.branchFavorite("topic") {
		t.Fatal("the branch favorite did not use the scoped key")
	}
}
