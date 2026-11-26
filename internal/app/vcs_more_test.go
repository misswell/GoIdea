package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestParseConflictMarkersSupportsClassicAndDiff3(t *testing.T) {
	text := "before\n" +
		"<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> topic\n" +
		"middle\n" +
		"<<<<<<< ours\nlocal\n||||||| merged common ancestors\nbase\n=======\nremote\n>>>>>>> theirs\n" +
		"after\n"
	blocks := parseConflictMarkers(text)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].ours != "ours\n" || blocks[0].theirs != "theirs\n" || blocks[0].hasBase {
		t.Fatalf("classic block = %+v", blocks[0])
	}
	if !blocks[1].hasBase || blocks[1].base != "base\n" ||
		blocks[1].oursLabel != "ours" || blocks[1].theirsLabel != "theirs" {
		t.Fatalf("diff3 block = %+v", blocks[1])
	}
	if text[blocks[1].from:blocks[1].to] != blocks[1].raw {
		t.Fatalf("raw conflict span was not retained")
	}
}

func TestCommitWebURLSupportsCommonGitRemotes(t *testing.T) {
	const revision = "0123456789abcdef"
	tests := []struct {
		remote string
		want   string
	}{
		{"https://github.com/acme/goidea.git", "https://github.com/acme/goidea/commit/" + revision},
		{"git@github.com:acme/goidea.git", "https://github.com/acme/goidea/commit/" + revision},
		{"ssh://git@github.com/acme/goidea.git", "https://github.com/acme/goidea/commit/" + revision},
		{"git@bitbucket.org:acme/goidea.git", "https://bitbucket.org/acme/goidea/commits/" + revision},
		{"file:///tmp/goidea.git", ""},
	}
	for _, test := range tests {
		if got := commitWebURL(test.remote, revision); got != test.want {
			t.Errorf("commitWebURL(%q) = %q, want %q", test.remote, got, test.want)
		}
	}
}

func TestCommitOnlyPathsKeepsTheChangelistScope(t *testing.T) {
	modified := "modified"
	first := openidea.FileStatus{Path: "/repo/a.go", RelPath: "a.go", Unstaged: &modified}
	second := openidea.FileStatus{Path: "/repo/b.go", RelPath: "b.go", Unstaged: &modified}
	m := &Model{vcs: VCSPane{files: []openidea.FileStatus{first, second}}}
	c := &commitState{onlyPaths: map[string]bool{first.Path: true}, checks: map[string]bool{}}
	if !m.commitIncluded(c, first) {
		t.Fatal("the changelist's file was excluded")
	}
	if m.commitIncluded(c, second) {
		t.Fatal("a file outside the changelist was included")
	}
	plan := m.planCommit(c)
	if len(plan.included) != 1 || plan.included[0] != first.Path {
		t.Fatalf("plan included = %v", plan.included)
	}
}

func TestShelfTargetArguments(t *testing.T) {
	arg := map[string]any{
		"path": "a.go",
		"entry": map[string]any{
			"id": "shelf-7",
		},
	}
	if got := shelfTargetID(arg); got != "shelf-7" {
		t.Fatalf("id = %q", got)
	}
	if got := shelfTargetPath(arg); got != "a.go" {
		t.Fatalf("path = %q", got)
	}
}

func TestVCSMoreActionsAndCloneOutput(t *testing.T) {
	for _, id := range []string{
		"vcs.clone", "vcs.worktrees", "vcs.shelve", "vcs.showShelf",
		"vcs.applyPatch", "vcs.changelist.new", "vcs.resolveConflicts",
		"vcs.log.copyRevision", "vcs.log.interactiveRebase",
		"vcs.rollback", "vcs.showDiff", "vcs.annotate",
		"vcs.localHistory", "localHistory.putLabel",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("missing action %s", id)
		}
	}
	m := &Model{vcsMore: vcsMoreState{channel: 17}}
	if !m.acceptVCSMoreOutput(17, []byte(`"Receiving objects\n"`)) {
		t.Fatal("clone output was not accepted")
	}
	if got := strings.Join(m.vcsMore.output, "\n"); got != "Receiving objects" {
		t.Fatalf("output = %q", got)
	}
	if m.acceptVCSMoreOutput(18, []byte(`"wrong channel"`)) {
		t.Fatal("output for another channel was accepted")
	}
}

func TestCommitMessageHistoryWritesTheSelectedMessage(t *testing.T) {
	root := "/project"
	m := &Model{
		commit: &commitState{checks: map[string]bool{}},
		prefs: Preferences{VCS: VCSPrefs{CommitMessages: map[string][]string{
			root: {"Fix timeout\n\nExplain the cause."},
		}}},
	}
	m.setRoot(root)
	m.OpenCommitMessageHistory()
	if m.vcsMore.kind != vcsMoreMessage || len(m.vcsMore.items) != 1 {
		t.Fatalf("message history = %+v", m.vcsMore)
	}
	m.activateVCSMoreItem(0)
	if m.commit.message != "Fix timeout\n\nExplain the cause." {
		t.Fatalf("commit message = %q", m.commit.message)
	}
}

func TestVCSMoreDialogRendersNativeCommands(t *testing.T) {
	m := &Model{vcsMore: vcsMoreState{
		open: true, kind: vcsMoreShelf, action: "shelf", title: "Shelf",
		hint: "Shelved changes", shelves: []shelvedChangeList{{
			ID: "shelf-1", Name: "Login fix",
			Files: []openidea.RevisionChange{{Path: "Login.java", Kind: "modified"}},
		}},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsMoreDialog(c, c.Theme())
	}, 1000, 760)
	tester.Frame()
	for _, text := range []string{
		"Shelf", "Unshelve", "Show Diff", "Rename…", "Delete",
		"Create Patch…", "Copy Patch", "Import Patches…", "Login fix",
	} {
		if !tester.HasText(text) {
			t.Errorf("VCS More dialog does not contain %q", text)
		}
	}
}

func TestNormalizeChangelistStateRepairsAndCanonicalizesAssignments(t *testing.T) {
	state := normalizeChangelistState(changelistState{
		Lists: []changelistEntry{
			{ID: "", Name: "ignored"},
			{ID: "feature", Name: " Feature ", Comment: " ticket "},
			{ID: "feature", Name: "duplicate"},
		},
		Active: "missing",
		Files: map[string]string{
			"./src/../src/Main.java": "feature",
			"orphan.java":            "missing",
		},
	})
	if len(state.Lists) != 1 || state.Lists[0].ID != "feature" ||
		state.Lists[0].Name != "Feature" || state.Lists[0].Comment != "ticket" {
		t.Fatalf("normalized lists = %+v", state.Lists)
	}
	if state.Active != "feature" {
		t.Fatalf("active list = %q, want feature", state.Active)
	}
	if got := state.Files["src/Main.java"]; got != "feature" {
		t.Fatalf("canonical assignment = %q, want feature", got)
	}
	if len(state.Files) != 1 {
		t.Fatalf("invalid assignments remain: %+v", state.Files)
	}

	empty := normalizeChangelistState(changelistState{})
	if len(empty.Lists) != 1 || empty.Lists[0].ID != defaultChangelistID ||
		empty.Active != defaultChangelistID || empty.Files == nil {
		t.Fatalf("empty state = %+v", empty)
	}
}

func TestValidateChangelistNameIsCaseInsensitive(t *testing.T) {
	state := changelistState{Lists: []changelistEntry{
		{ID: "default", Name: "Changes"},
		{ID: "feature", Name: "Feature"},
	}}
	if err := validateChangelistName(state, " feature ", ""); err == nil {
		t.Fatal("duplicate name was accepted")
	}
	if err := validateChangelistName(state, "feature", "feature"); err != nil {
		t.Fatalf("renaming a list to its own name failed: %v", err)
	}
	if err := validateChangelistName(state, " ", ""); err == nil {
		t.Fatal("empty name was accepted")
	}
}

func TestChangelistAssignmentsUseProjectRelativePathsAcrossRepositories(t *testing.T) {
	root := filepath.Clean("/project")
	first := filepath.Join(root, "app")
	second := filepath.Join(root, "orders")
	firstFile := filepath.Join(first, "src", "Main.java")
	secondFile := filepath.Join(second, "Order.java")
	m := &Model{vcs: VCSPane{
		repositories: []vcsRepository{{Root: first}, {Root: second}},
		files: []openidea.FileStatus{
			{Path: firstFile, RelPath: "src/Main.java", Unstaged: str("modified")},
			{Path: secondFile, RelPath: "Order.java", Unstaged: str("modified")},
		},
	}}
	m.setRoot(root)
	state := changelistState{
		Lists: []changelistEntry{
			{ID: "default", Name: "Changes"},
			{ID: "feature", Name: "Feature"},
		},
		Active: "default",
		Files: map[string]string{
			filepath.ToSlash(firstFile): "feature",
		},
	}
	m.syncVCSChangelistAssignments(&state, false)
	if state.Files["app/src/Main.java"] != "feature" {
		t.Fatalf("nested repository assignment = %#v", state.Files)
	}
	if state.Files["orders/Order.java"] != "default" {
		t.Fatalf("new active-list assignment = %#v", state.Files)
	}
	if m.changelistForPath(state, firstFile) != "feature" ||
		m.changelistForPath(state, secondFile) != "default" {
		t.Fatalf("path lookup did not use canonical assignments: %#v", state.Files)
	}
}

func TestDeleteChangelistMovesFilesAndCommitUsesOnlyChosenList(t *testing.T) {
	first := "/project/a.java"
	second := "/project/b.java"
	m := &Model{vcs: VCSPane{
		files: []openidea.FileStatus{
			{Path: first, RelPath: "a.java", Unstaged: str("modified")},
			{Path: second, RelPath: "b.java", Unstaged: str("modified")},
		},
	}}
	m.setRoot("/project")
	m.ctx = context.Background()
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "vcs_changelists_save" {
			t.Fatalf("unexpected command %q", command)
		}
		return json.RawMessage(`null`), nil
	}
	m.postHook = func(fn func()) { fn() }
	m.vcsMore.lists = changelistState{
		Lists: []changelistEntry{
			{ID: "default", Name: "Changes"},
			{ID: "feature", Name: "Feature"},
		},
		Active: "default",
		Files: map[string]string{
			"a.java": "feature",
		},
	}
	m.vcsMore.changelistsLoaded = true
	m.deleteChangelist("feature")
	if m.vcsMore.lists.Files["a.java"] != "default" {
		t.Fatalf("deleted list assignment = %#v", m.vcsMore.lists.Files)
	}
}

func TestLoadChangelistsSynchronizesNewChangesWithTheActiveList(t *testing.T) {
	file := "/project/src/Main.java"
	loaded := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		vcs: VCSPane{
			repositories: []vcsRepository{{Root: "/project"}},
			files: []openidea.FileStatus{{
				Path: file, RelPath: "src/Main.java", Unstaged: str("modified"),
			}},
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "vcs_changelists_load" {
				t.Fatalf("unexpected command %q", command)
			}
			return mustVCSJSON(t, changelistState{
				Lists: []changelistEntry{
					{ID: "default", Name: "Changes"},
					{ID: "feature", Name: "Feature"},
				},
				Active: "feature",
				Files:  map[string]string{},
			}), nil
		},
		postHook: func(fn func()) {
			fn()
			select {
			case loaded <- struct{}{}:
			default:
			}
		},
	}
	m.setRoot("/project")
	m.loadChangelists(false)
	select {
	case <-loaded:
	case <-time.After(2 * time.Second):
		t.Fatal("changelists did not load")
	}
	if got := m.vcsMore.lists.Files["src/Main.java"]; got != "feature" {
		t.Fatalf("new change assignment = %q, want feature; files=%#v", got, m.vcsMore.lists.Files)
	}
}

func TestCommitDefaultsExcludeInactiveChangelists(t *testing.T) {
	first := "/project/a.java"
	second := "/project/b.java"
	m := &Model{
		vcs: VCSPane{files: []openidea.FileStatus{
			{Path: first, RelPath: "a.java", Unstaged: str("modified")},
			{Path: second, RelPath: "b.java", Unstaged: str("modified")},
		}},
		vcsMore: vcsMoreState{
			changelistsLoaded: true,
			lists: changelistState{
				Lists: []changelistEntry{
					{ID: "default", Name: "Changes"},
					{ID: "feature", Name: "Feature"},
				},
				Active: "default",
				Files:  map[string]string{"a.java": "feature"},
			},
		},
	}
	m.setRoot("/project")
	c := &commitState{allRepositories: true, checks: map[string]bool{}}
	if got := m.activeChangelistPaths(); len(got) != 1 || got[0] != second {
		t.Fatalf("active changelist paths = %#v, want %q", got, second)
	}
	if m.commitIncluded(c, m.vcs.files[0]) {
		t.Fatal("inactive changelist was included by default")
	}
	if !m.commitIncluded(c, m.vcs.files[1]) {
		t.Fatal("active changelist was excluded by default")
	}
	plan := m.planCommit(c)
	if len(plan.included) != 1 || plan.included[0] != second {
		t.Fatalf("commit plan = %+v", plan)
	}
	c.checks[first] = true
	if !m.commitIncluded(c, m.vcs.files[0]) {
		t.Fatal("explicit inclusion did not override the active-list default")
	}
}

func TestPatchSourcesRejectMixedRepositoriesAndUseRelativePaths(t *testing.T) {
	root := filepath.Clean("/project")
	first := filepath.Join(root, "app")
	second := filepath.Join(root, "orders")
	m := &Model{vcs: VCSPane{
		repositories: []vcsRepository{{Root: first}, {Root: second}},
		files: []openidea.FileStatus{
			{Path: filepath.Join(first, "src", "Main.java"), RelPath: "src/Main.java", Unstaged: str("modified")},
			{Path: filepath.Join(second, "Order.java"), RelPath: "Order.java", Unstaged: str("modified")},
		},
	}}
	m.setRoot(root)
	files, err := m.patchFilesForPaths([]string{filepath.Join(first, "src", "Main.java")})
	if err != nil || len(files) != 1 || files[0].repository != first || files[0].path != filepath.Join(first, "src", "Main.java") {
		t.Fatalf("single-repository patch source = %+v, %v", files, err)
	}
	files, err = m.patchFilesForPaths([]string{filepath.Join(first, "src", "Main.java"), filepath.Join(second, "Order.java")})
	if err != nil {
		t.Fatal(err)
	}
	byRepository, err := patchSourcesByRepository(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(byRepository) != 2 {
		t.Fatalf("patch repositories = %#v", byRepository)
	}
}

func TestCreatePatchForPathsRejectsFilesFromSeveralRepositories(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "app")
	second := filepath.Join(root, "orders")
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	firstFile := filepath.Join(first, "Main.java")
	secondFile := filepath.Join(second, "Order.java")
	for _, path := range []string{firstFile, secondFile} {
		if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &Model{
		ctx: context.Background(),
		vcs: VCSPane{
			repositories: []vcsRepository{{Root: first}, {Root: second}},
			files: []openidea.FileStatus{
				{Path: firstFile, RelPath: "Main.java", Unstaged: str("modified")},
				{Path: secondFile, RelPath: "Order.java", Unstaged: str("modified")},
			},
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			t.Fatalf("mixed-repository patch unexpectedly invoked %s", command)
			return nil, nil
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(root)
	first, second = realPath(first), realPath(second)
	firstFile, secondFile = realPath(firstFile), realPath(secondFile)
	m.vcs.repositories = []vcsRepository{{Root: first}, {Root: second}}
	m.vcs.files = []openidea.FileStatus{
		{Path: firstFile, RelPath: "Main.java", Unstaged: str("modified")},
		{Path: secondFile, RelPath: "Order.java", Unstaged: str("modified")},
	}
	files, err := m.patchFilesForPaths([]string{firstFile, secondFile})
	if err != nil || len(files) != 2 {
		t.Fatalf("mixed patch files = %+v, %v", files, err)
	}
	byRepository, err := patchSourcesByRepository(files)
	if err != nil || len(byRepository) != 2 {
		t.Fatalf("mixed patch repositories = %#v, %v", byRepository, err)
	}
	m.CreatePatchForPaths([]string{firstFile, secondFile})
	if !strings.Contains(m.status, "one repository") {
		t.Fatalf("mixed-repository status = %q", m.status)
	}
}

func TestCreatePatchForSelectedLogCommitUsesItsRepository(t *testing.T) {
	first := "/project/orders"
	second := "/project/catalog"
	requested := make(chan map[string]any, 1)
	m := &Model{
		ctx: context.Background(),
		log: LogPane{
			selected:    0,
			commitRepos: []string{second},
			commits: []openidea.Commit{{
				ID: "222222222222", ShortID: "22222222", Summary: "Catalog commit",
			}},
		},
	}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{{Root: first}, {Root: second}}
	m.invokeHook = func(ctx context.Context, command string, args any) (json.RawMessage, error) {
		if command == "vcs_create_patch" {
			requested <- args.(map[string]any)
			return json.RawMessage(`"diff --git a/a b/a\n"`), nil
		}
		return json.RawMessage(`null`), nil
	}
	m.CreatePatchForCommits(nil)
	select {
	case args := <-requested:
		if args["repo"] != second {
			t.Fatalf("commit patch repo = %#v, want %q", args["repo"], second)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("commit patch did not reach the backend")
	}
}

func TestPatchPreviewChoosesTheRepositoryContainingItsFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "app")
	second := filepath.Join(root, "orders")
	for _, dir := range []string{filepath.Join(first, "src"), filepath.Join(second, "src")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(second, "src", "Order.java"), []byte("class Order {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, second = realPath(first), realPath(second)
	m := &Model{
		ctx: context.Background(),
		vcs: VCSPane{
			repositories: []vcsRepository{{Root: first}, {Root: second}},
			current:      first,
		},
	}
	m.setRoot(root)
	patch := "diff --git a/src/Order.java b/src/Order.java\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/src/Order.java\n" +
		"+++ b/src/Order.java\n" +
		"@@ -1 +1 @@\n-old\n+new\n"
	if got := m.patchRepositoryFromHeader(patch); got != second {
		t.Fatalf("patch repository = %q, want %q", got, second)
	}
}

func TestVCSMorePromptRendersWorktreeFields(t *testing.T) {
	m := &Model{vcsMore: vcsMoreState{
		open: true, kind: vcsMorePrompt, action: vcsMoreActionWorktreeAdd,
		title: "New Worktree", hint: "Enter a directory and revision",
		input: "/tmp/project-worktree", input2: "HEAD",
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsMoreDialog(c, c.Theme())
	}, 900, 700)
	tester.Frame()
	for _, text := range []string{
		"New Worktree", "Directory", "Starting revision",
		"New branch (optional)", "Create", "Close",
	} {
		if !tester.HasText(text) {
			t.Errorf("worktree prompt does not contain %q", text)
		}
	}
}

func TestWorktreeMoveTargetMatchesOpenIDEAFolderChoice(t *testing.T) {
	for _, tc := range []struct {
		worktree string
		folder   string
		want     string
	}{
		{"/work/app-feature", "/work", "/work/app-feature"},
		{"/work/app-feature", "/archive", "/archive/app-feature"},
		{`C:\Work\app-feature`, `D:\archive`, `D:\archive\app-feature`},
	} {
		if got := worktreeMoveTarget(tc.worktree, tc.folder); got != tc.want {
			t.Errorf("worktreeMoveTarget(%q, %q) = %q, want %q", tc.worktree, tc.folder, got, tc.want)
		}
	}
}

func TestWorktreeContainsProjectUsesPathBoundaries(t *testing.T) {
	for _, tc := range []struct {
		worktree, project string
		want              bool
	}{
		{"/work/app", "/work/app/module", true},
		{"/work/app/", "/work/app", true},
		{"/work/app", "/work/application", false},
		{`C:\Work\App`, `c:\work\app\module`, true},
	} {
		if got := worktreeContainsProject(tc.worktree, tc.project); got != tc.want {
			t.Errorf("worktreeContainsProject(%q, %q) = %v, want %v", tc.worktree, tc.project, got, tc.want)
		}
	}
}

func TestVCSMoreWorktreeDialogOffersMoveAndProtectsProject(t *testing.T) {
	m := &Model{}
	m.setRoot("/work/app")
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMoreWorktrees, action: "worktrees", title: "Git Worktrees",
		worktrees: []openidea.Worktree{
			{Path: "/work/app", Main: true},
			{Path: "/work/app-feature", Branch: strp("feature")},
		},
		choice: 1,
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsMoreDialog(c, c.Theme())
	}, 1000, 760)
	tester.Frame()
	for _, text := range []string{"Git Worktrees", "Open", "New…", "Move…", "Remove", "app-feature"} {
		if !tester.HasText(text) {
			t.Errorf("worktree dialog does not contain %q", text)
		}
	}
	m.setRoot("/work/app-feature/module")
	tester.Frame()
	if !worktreeContainsProject("/work/app-feature", m.Root()) {
		t.Fatal("the linked worktree containing the open project was not protected")
	}
	m.vcsMore.kind = vcsMorePrompt
	m.vcsMore.action = vcsMoreActionWorktreeMove
	m.vcsMore.title = "Move Worktree"
	m.vcsMore.input2, m.vcsMore.input3 = "/work/app-feature", "/work/archive/app-feature"
	tester.Frame()
	for _, text := range []string{"Move Worktree", "Worktree", "New directory", "Move", "Close"} {
		if !tester.HasText(text) {
			t.Errorf("worktree move dialog does not contain %q", text)
		}
	}
}
