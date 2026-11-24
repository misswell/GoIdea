package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestCommitIncludesOnlySelectedHunks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	head := "a\nb\nc\nd\ne\nf\n"
	disk := "A\nb\nc\nd\nE\nf\n"
	hunks := []openidea.LineHunk{
		{Kind: "modified", Start: 0, Lines: 1, OldStart: 0, OldLines: 1, OldText: "a\n"},
		{Kind: "modified", Start: 4, Lines: 1, OldStart: 4, OldLines: 1, OldText: "e\n"},
	}
	keys := commitHunkKeys(disk, hunks)
	if len(keys) != 2 {
		t.Fatalf("hunk keys = %#v", keys)
	}
	commitArgs := make(chan map[string]any, 1)
	posted := make(chan struct{}, 4)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
	}
	m.setRoot(root)
	m.vcs.repositories = []vcsRepository{{Root: root, Head: str("main")}}
	m.vcs.current = root
	m.vcs.files = []openidea.FileStatus{{
		Path: path, RelPath: "f.txt", Unstaged: str("modified"),
	}}
	c := m.newCommitState(root)
	c.message = "Commit the first hunk"
	c.checks[path] = true
	c.hunks[path] = &commitHunksState{
		hunks: hunks, keys: keys, selected: map[string]bool{keys[0]: true},
	}
	m.commit = c
	if selected, total, ok := m.commitHunkChoice(c, path); !ok || selected != 1 || total != 2 {
		t.Fatalf("hunk choice = %d/%d, %v", selected, total, ok)
	}
	if !m.commitIncluded(c, m.vcs.files[0]) {
		t.Fatal("a file with one selected hunk stayed out of the commit")
	}
	plan := m.planCommit(c)
	if len(plan.partial) != 1 || len(plan.partial[0].Keys) != 1 {
		t.Fatalf("partial plan = %#v", plan.partial)
	}

	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "vcs_file_at":
			return json.Marshal(&head)
		case "fs_read_text":
			return json.Marshal(disk)
		case "vcs_diff_hunks":
			return json.Marshal(hunks)
		case "vcs_commit":
			commitArgs <- args.(map[string]any)
			return json.RawMessage(`"0123456789abcdef"`), nil
		case "vcs_roots":
			return mustVCSJSON(t, m.vcs.repositories), nil
		case "vcs_status":
			return json.RawMessage(`[]`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.submitCommit()
	var args map[string]any
	select {
	case args = <-commitArgs:
	case <-time.After(2 * time.Second):
		t.Fatal("partial commit did not reach vcs_commit")
	}
	options, ok := args["options"].(openidea.CommitOptions)
	if !ok {
		t.Fatalf("commit options = %#v", args["options"])
	}
	if len(options.Partial) != 1 || options.Partial[0].Path != path ||
		len(options.Partial[0].Hunks) != 1 || options.Partial[0].Hunks[0].Start != 0 {
		t.Fatalf("partial options = %#v", options.Partial)
	}
	if len(options.Stage) != 0 {
		t.Fatalf("partial file was staged whole: %#v", options.Stage)
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("partial commit did not finish")
	}
}

func TestAmendCommitAllowsNoVisibleChanges(t *testing.T) {
	root := t.TempDir()
	commitArgs := make(chan map[string]any, 1)
	posted := make(chan struct{}, 4)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
	}
	m.setRoot(root)
	m.vcs.repositories = []vcsRepository{{Root: root, Head: str("main")}}
	m.vcs.current = root
	c := m.newCommitState(root)
	c.message = "Keep the existing commit"
	c.amend = true
	m.commit = c
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "vcs_commit":
			commitArgs <- args.(map[string]any)
			return json.RawMessage(`"0123456789abcdef"`), nil
		case "vcs_roots":
			return mustVCSJSON(t, m.vcs.repositories), nil
		case "vcs_status":
			return json.RawMessage(`[]`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.submitCommit()
	select {
	case args := <-commitArgs:
		options, ok := args["options"].(openidea.CommitOptions)
		if !ok || !options.Amend {
			t.Fatalf("amend options = %#v", args["options"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("amend with no visible files did not reach vcs_commit")
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("amend with no visible files did not finish")
	}
}

func TestCommitRefusesWholeFileAfterHunkLoadFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	m := &Model{}
	m.setRoot(root)
	m.vcs.repositories = []vcsRepository{{Root: root, Head: str("main")}}
	m.vcs.current = root
	m.vcs.files = []openidea.FileStatus{{
		Path: path, RelPath: "f.txt", Unstaged: str("modified"),
	}}
	c := m.newCommitState(root)
	c.message = "Commit the selected lines"
	c.checks[path] = true
	c.hunks[path] = &commitHunksState{err: "backend unavailable"}
	m.commit = c
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		t.Fatal("a failed hunk load reached the backend")
		return nil, nil
	}

	m.submitCommit()
	if m.commitIncluded(c, m.vcs.files[0]) {
		t.Fatal("a failed hunk load fell back to including the whole file")
	}
	if !strings.Contains(c.err, "could not be loaded") {
		t.Fatalf("commit error = %q", c.err)
	}
}

func TestRollbackPlansRenamesAddedFilesFoldersAndSubmodules(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "vendor")
	if err := os.MkdirAll(filepath.Join(nested, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, _ = filepath.EvalSymlinks(root)
	nested = filepath.Join(root, "vendor")
	innerFile := filepath.Join(nested, "src", "lib.txt")
	if err := os.WriteFile(innerFile, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	renamed := openidea.FileStatus{
		Path: filepath.Join(root, "new.txt"), RelPath: "new.txt",
		Staged: str("renamed"), RenamedFrom: str("old.txt"),
	}
	added := openidea.FileStatus{
		Path: filepath.Join(root, "added.txt"), RelPath: "added.txt", Staged: str("added"),
	}
	nestedRoot := openidea.FileStatus{
		Path: nested, RelPath: "vendor", Unstaged: str("untracked"),
	}
	inner := openidea.FileStatus{
		Path: innerFile, RelPath: "vendor/src/lib.txt", Unstaged: str("modified"),
	}
	conflict := openidea.FileStatus{
		Path: filepath.Join(root, "conflict.txt"), RelPath: "conflict.txt",
		Unstaged: str("conflicted"),
	}
	m := &Model{vcs: VCSPane{
		repositories: []vcsRepository{{Root: root}, {Root: nested}},
		files:        []openidea.FileStatus{renamed, added, nestedRoot, inner, conflict},
		current:      root,
	}}
	m.setRoot(root)
	m.AskRollback(openidea.FileStatus{Path: root, RelPath: "."})
	if m.rollback == nil {
		t.Fatal("folder rollback did not open")
	}
	var paths []string
	for _, file := range m.rollback.files {
		paths = append(paths, file.Path)
	}
	if slicesContainPath(paths, nested) {
		t.Fatalf("rollback included the nested repository root: %#v", paths)
	}
	if !slicesContainPath(paths, renamed.Path) || !slicesContainPath(paths, innerFile) {
		t.Fatalf("folder rollback omitted changed files: %#v", paths)
	}

	plan := planRollback(m.rollback.files, false)
	for _, want := range []string{
		renamed.Path, filepath.Join(root, "old.txt"), added.Path,
	} {
		if !slicesContainPath(plan.unstage, want) {
			t.Errorf("unstage omits %s: %#v", want, plan.unstage)
		}
	}
	if slicesContainPath(plan.discard, added.Path) {
		t.Fatal("a staged addition was discarded without opting in")
	}
	for _, want := range []string{renamed.Path, filepath.Join(root, "old.txt"), innerFile, conflict.Path} {
		if !slicesContainPath(plan.discard, want) {
			t.Errorf("discard omits %s: %#v", want, plan.discard)
		}
	}
	withAdded := planRollback(m.rollback.files, true)
	if !slicesContainPath(withAdded.discard, added.Path) {
		t.Fatal("Delete local copies did not discard the staged addition")
	}

	calls := make(chan string, 3)
	posted := make(chan struct{}, 2)
	m.ctx = context.Background()
	m.postHook = func(fn func()) {
		fn()
		posted <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "vcs_unstage", "vcs_discard":
			calls <- command
			return json.RawMessage(`null`), nil
		case "vcs_roots":
			return mustVCSJSON(t, m.vcs.repositories), nil
		case "vcs_status":
			return json.RawMessage(`[]`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.ConfirmRollback()
	if first, second := <-calls, <-calls; first != "vcs_unstage" || second != "vcs_discard" {
		t.Fatalf("rollback RPC order = %q, %q", first, second)
	}
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("rollback did not finish")
	}
}

func slicesContainPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}

func TestUndoCommitGuardsPublishedProtectedAndInProgressHistory(t *testing.T) {
	main := "main"
	head := "333333333333"
	operation := "rebase"
	latest := openidea.Commit{ID: head, Parents: []string{"222222222222"}}
	branch := openidea.BranchInfo{Head: &main, HeadCommit: &head}
	cases := []struct {
		name   string
		commit openidea.Commit
		branch openidea.BranchInfo
		want   string
	}{
		{"latest", latest, branch, ""},
		{"nonlatest", openidea.Commit{ID: "222222222222"}, branch, "latest commit"},
		{"merge", openidea.Commit{ID: head, Parents: []string{"a", "b"}}, branch, "merge commit"},
		{"operation", latest, openidea.BranchInfo{Head: &main, HeadCommit: &head, Operation: &operation}, "rebase is in progress"},
		{"detached", latest, openidea.BranchInfo{HeadCommit: &head}, "detached HEAD"},
	}
	for _, tc := range cases {
		if got := undoCommitProblem(tc.commit, tc.branch); !strings.Contains(got, tc.want) {
			t.Errorf("%s problem = %q, want %q", tc.name, got, tc.want)
		}
	}
	upstream := "origin/main"
	pushed := branch
	pushed.Upstream, pushed.Ahead = &upstream, 0
	if !undoCommitHeadPushed(pushed) {
		t.Fatal("HEAD on its upstream was not treated as pushed")
	}
	if problem := undoCommitPushedProtectedProblem(
		pushed, func(name string) bool { return name == "main" },
	); !strings.Contains(problem, "protected branch 'main'") {
		t.Fatalf("protected pushed problem = %q", problem)
	}
	pushed.Ahead = 1
	if problem := undoCommitPushedProtectedProblem(
		pushed, func(name string) bool { return name == "main" },
	); problem != "" {
		t.Fatalf("unpushed protected history was refused: %q", problem)
	}

	settings := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.git.protectedBranches": "main",
	})
	settings.vcs.current = settings.Root()
	settings.vcs.repositories = []vcsRepository{{Root: settings.Root(), Head: &main}}
	rawBranch, _ := json.Marshal(pushed)
	pushed.Ahead = 0
	rawBranch, _ = json.Marshal(pushed)
	undoCalls := make(chan struct{}, 1)
	settings.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "vcs_branch_info":
			return rawBranch, nil
		case "vcs_undo_commit":
			undoCalls <- struct{}{}
			return json.RawMessage(`"message"`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	updated := make(chan struct{}, 2)
	settings.postHook = func(fn func()) {
		fn()
		updated <- struct{}{}
	}
	settings.log.commits = []openidea.Commit{latest}
	settings.log.selected = 0
	settings.UndoCommitArg(map[string]any{"id": head, "repo": settings.Root()})
	select {
	case <-updated:
	case <-time.After(2 * time.Second):
		t.Fatal("Undo Commit preflight did not finish")
	}
	if settings.vcsMore.open {
		t.Fatal("protected pushed history opened an Undo Commit confirmation")
	}
	select {
	case <-undoCalls:
		t.Fatal("protected pushed history invoked vcs_undo_commit")
	default:
	}
}

func TestDeleteBranchRemovesHoldingWorktreeWithSafeForceFallback(t *testing.T) {
	root := t.TempDir()
	linked := filepath.Join(t.TempDir(), "linked")
	topic := "topic"
	worktrees := []openidea.Worktree{{
		Path: linked, Branch: &topic,
	}}
	holding, ok := branchHoldingWorktree(topic, worktrees)
	if !ok || holding.Path != linked {
		t.Fatalf("holding worktree = %+v, %v", holding, ok)
	}

	calls := make(chan string, 6)
	posted := make(chan struct{}, 8)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
	}
	m.setRoot(root)
	m.vcs.current = root
	m.vcs.repositories = []vcsRepository{{Root: root, Head: str("main")}}
	m.branch.open = true
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "vcs_worktrees":
			calls <- "worktrees"
			return mustVCSJSON(t, worktrees), nil
		case "vcs_remove_worktree":
			request := args.(map[string]any)
			force := request["force"].(bool)
			calls <- fmt.Sprintf("remove:%t", force)
			if !force {
				return nil, errors.New("worktree has local changes")
			}
			return json.RawMessage(`null`), nil
		case "vcs_delete_branch":
			calls <- "delete"
			return mustVCSJSON(t, openidea.DeletedBranch{Name: topic}), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.deleteLocalBranch(openidea.Branch{Name: topic})
	waitBranchPrompt(t, &m.branch, posted, "delete-worktree")
	m.submitBranchPrompt()
	waitBranchPrompt(t, &m.branch, posted, "force-worktree")
	m.submitBranchPrompt()
	for m.branch.busy {
		select {
		case <-posted:
		case <-time.After(2 * time.Second):
			t.Fatal("forced worktree branch deletion did not finish")
		}
	}
	var got []string
	for len(calls) > 0 {
		got = append(got, <-calls)
	}
	want := []string{"worktrees", "remove:false", "remove:true", "delete"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("branch/worktree RPC order = %#v, want %#v", got, want)
	}
}

func waitBranchPrompt(
	t *testing.T,
	branch *BranchPane,
	posted <-chan struct{},
	kind string,
) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		if branch.prompt == kind && !branch.busy {
			return
		}
		select {
		case <-posted:
		case <-deadline.C:
			t.Fatalf("branch prompt = %q busy=%v, want %q", branch.prompt, branch.busy, kind)
		}
	}
}
