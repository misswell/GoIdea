package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestFilterUnifiedPatchMatchesChangedAndRenamedPaths(t *testing.T) {
	section := func(oldPath, newPath string) string {
		return fmt.Sprintf(
			"diff --git %s %s\nindex 1111111..2222222 100644\n--- %s\n+++ %s\n@@ -1 +1 @@\n-old\n+new\n",
			oldPath, newPath, oldPath, newPath,
		)
	}
	first := section("a/a.ts", "b/a.ts")
	renamed := section("a/old.ts", "b/new.ts")
	quoted := section(`"a/old name.ts"`, `"b/new name.ts"`)
	patch := first + renamed + quoted

	if got := filterUnifiedPatch(patch, map[string]bool{"a.ts": true}); got != first {
		t.Fatalf("modified-path filter = %q, want %q", got, first)
	}
	if got := filterUnifiedPatch(patch, map[string]bool{"old.ts": true}); got != renamed {
		t.Fatalf("old rename path filter = %q, want %q", got, renamed)
	}
	if got := filterUnifiedPatch(patch, map[string]bool{"new.ts": true, "new name.ts": true}); got != renamed+quoted {
		t.Fatalf("new and quoted path filter = %q, want %q", got, renamed+quoted)
	}
	if got := filterUnifiedPatch(patch, map[string]bool{"unrelated.ts": true}); got != "" {
		t.Fatalf("unrelated filter = %q, want empty", got)
	}
}

func TestTargetRelativeLogPathUsesRepositoryRoot(t *testing.T) {
	repository := filepath.Clean("/project/orders")
	if got := targetRelativeLogPath(repository, filepath.Join(repository, "src", "Order.java")); got != "src/Order.java" {
		t.Fatalf("repository-relative path = %q, want src/Order.java", got)
	}
	outside := filepath.Join("/project", "catalog", "Catalog.java")
	if got := targetRelativeLogPath(repository, outside); got != filepath.ToSlash(outside) {
		t.Fatalf("outside path = %q, want absolute %q", got, filepath.ToSlash(outside))
	}
}

func TestGetFromRevisionUsesTheCommitRepositoryForRelativePaths(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "orders")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	repository = realPath(repository)
	target := filepath.Join(repository, "src", "Order.java")
	requested := make(chan map[string]any, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command == "vcs_file_at" {
				requested <- args.(map[string]any)
				return json.RawMessage(`"class Order {}"`), nil
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(filepath.Dir(repository))
	m.vcs.repositories = []vcsRepository{{Root: repository}}
	m.vcsMore.open = true
	m.vcsMore.input3 = repository
	m.getLogChangeFromRevision("src/Order.java", "abc123", repository)
	select {
	case args := <-requested:
		if args["path"] != target || args["rev"] != "abc123" {
			t.Fatalf("vcs_file_at args = %#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Get from Revision did not read the file")
	}
}

func TestFileWebURLSupportsHostedRepositoryLayouts(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		revision string
		path     string
		dir      bool
		line     int
		want     string
	}{
		{
			name: "github", remote: "git@github.com:acme/goidea.git",
			revision: "main", path: "src/main.go", line: 12,
			want: "https://github.com/acme/goidea/blob/main/src/main.go#L12",
		},
		{
			name: "gitlab directory", remote: "https://gitlab.com/acme/goidea.git",
			revision: "dev", path: "src", dir: true,
			want: "https://gitlab.com/acme/goidea/-/tree/dev/src",
		},
		{
			name: "bitbucket file", remote: "ssh://git@bitbucket.org/acme/goidea.git",
			revision: "feature", path: "src/main.go", line: 7,
			want: "https://bitbucket.org/acme/goidea/src/feature/src/main.go#lines-7",
		},
		{name: "local", remote: "file:///tmp/goidea.git", revision: "HEAD", path: "a.go", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := fileWebURL(test.remote, test.revision, test.path, test.dir, test.line)
			if got != test.want {
				t.Fatalf("fileWebURL() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInteractiveRebasePlanUsesRootBaseAndRejectsNoop(t *testing.T) {
	m := &Model{log: LogPane{
		visible:  true,
		selected: 1,
		commits: []openidea.Commit{
			{ID: "newwest", ShortID: "newwest", Message: "Newest", Parents: []string{"oldest"}},
			{ID: "oldest", ShortID: "oldest", Message: "Oldest"},
		},
	}}
	m.OpenInteractiveRebase(nil)
	if !m.vcsMore.open || m.vcsMore.action != "interactive.rebase" {
		t.Fatalf("rebase dialog = %+v", m.vcsMore)
	}
	if m.vcsMore.rebaseBase != "" {
		t.Fatalf("root rebase base = %q, want no base", m.vcsMore.rebaseBase)
	}
	if len(m.vcsMore.rebaseCommits) != 2 ||
		m.vcsMore.rebaseCommits[0].ID != "oldest" ||
		m.vcsMore.rebaseCommits[1].ID != "newwest" {
		t.Fatalf("rebase order = %+v", m.vcsMore.rebaseCommits)
	}
	if got := m.interactiveRebaseProblem(); got != "Change at least one action or message" {
		t.Fatalf("initial problem = %q", got)
	}

	m.vcsMore.rebaseActions[0] = "reword"
	if got := m.interactiveRebaseProblem(); got != "Change at least one action or message" {
		t.Fatalf("unchanged reword problem = %q", got)
	}
	m.vcsMore.rebaseMessages[0] = "Rewritten root message"
	if got := m.interactiveRebaseProblem(); got != "" {
		t.Fatalf("changed reword problem = %q", got)
	}
}

func TestNewTagAtCommitUsesTheRequestedRevision(t *testing.T) {
	m := &Model{log: LogPane{
		visible:  true,
		selected: 0,
		commits: []openidea.Commit{
			{ID: "aaaaaaaa", ShortID: "aaaaaaaa", Summary: "First"},
			{ID: "bbbbbbbb", ShortID: "bbbbbbbb", Summary: "Second"},
		},
	}}
	m.setRoot(t.TempDir())
	m.NewTagAtCommit("bbbbbbbb")
	if !m.vcsOps.open || m.vcsOps.kind != vcsOpNewTag {
		t.Fatalf("tag operation = %+v", m.vcsOps)
	}
	if len(m.vcsOps.choices) != 1 || m.vcsOps.choices[0].value != "bbbbbbbb" {
		t.Fatalf("tag choices = %+v", m.vcsOps.choices)
	}
}

func TestInteractiveRebaseScopesAggregateHistoryToCommitRepository(t *testing.T) {
	first := "/project/orders"
	second := "/project/catalog"
	operationArgs := make(chan map[string]any, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command == "vcs_operation" {
				operationArgs <- args.(map[string]any)
			}
			return json.RawMessage(`null`), nil
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: str("main")},
		{Root: second, Head: str("feature")},
	}
	m.vcs.current = first
	m.log = LogPane{
		visible:     true,
		aggregate:   true,
		repo:        first,
		selected:    2,
		commitRepos: []string{second, first, second, first, second, first},
		commits: []openidea.Commit{
			{ID: "bbbb", ShortID: "bbbb", Summary: "Catalog newest", Parents: []string{"bbbb-old"}},
			{ID: "aaaa-new", ShortID: "aaaa", Summary: "Orders newest", Parents: []string{"aaaa"}},
			{ID: "bbbb-old", ShortID: "bbbb", Summary: "Catalog older", Parents: []string{"bbbb-root"}},
			{ID: "aaaa", ShortID: "aaaa", Summary: "Orders older", Parents: []string{"aaaa-root"}},
			{ID: "bbbb-root", ShortID: "bbbb", Summary: "Catalog root"},
			{ID: "aaaa-root", ShortID: "aaaa", Summary: "Orders root"},
		},
	}

	m.OpenInteractiveRebase(map[string]any{"id": "bbbb-old", "repo": second})
	if m.vcsMore.input3 != second {
		t.Fatalf("interactive rebase repo = %q, want %q", m.vcsMore.input3, second)
	}
	if len(m.vcsMore.rebaseCommits) != 2 ||
		m.vcsMore.rebaseCommits[0].ID != "bbbb-old" ||
		m.vcsMore.rebaseCommits[1].ID != "bbbb" {
		t.Fatalf("interactive rebase commits = %+v", m.vcsMore.rebaseCommits)
	}
	if m.vcsMore.rebaseBase != "bbbb-root" {
		t.Fatalf("interactive rebase base = %q, want bbbb-root", m.vcsMore.rebaseBase)
	}

	m.vcsMore.rebaseActions[0] = "reword"
	m.vcsMore.rebaseMessages[0] = "Rewritten catalog commit"
	m.log.visible = false
	m.submitInteractiveRebase()
	select {
	case args := <-operationArgs:
		if args["repo"] != second {
			t.Fatalf("interactive rebase operation repo = %#v, want %q", args["repo"], second)
		}
		op := args["op"].(map[string]any)
		entries := op["entries"].([]map[string]any)
		if len(entries) != 2 ||
			entries[0]["commit"] != "bbbb-old" ||
			entries[1]["commit"] != "bbbb" {
			t.Fatalf("interactive rebase entries = %+v", entries)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interactive rebase did not reach the backend")
	}
}

func TestGitLogBranchesPanelRendersNativeControls(t *testing.T) {
	m := &Model{log: LogPane{
		showBranches:   true,
		branchSelected: 0,
		favoriteBranches: map[string]bool{
			"main": true,
		},
		branches: []openidea.Branch{
			{Name: "main", Current: true},
			{Name: "origin/main", Remote: true},
		},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.logBranchesPanel(c, c.Theme())
	}, 600, 600)
	tester.Frame()
	for _, text := range []string{"Branches", "main", "origin/main", "★"} {
		if !tester.HasText(text) {
			t.Errorf("branches pane does not contain %q", text)
		}
	}
}

func TestGitLogChangedFileActionsAreRegistered(t *testing.T) {
	for _, id := range []string{
		"vcs.log.branches.new",
		"vcs.log.branches.update",
		"vcs.log.branches.delete",
		"vcs.log.branches.toggleFavorite",
		"vcs.log.branches.navigate",
		"vcs.log.changes.showDiff",
		"vcs.log.changes.compareWithLocal",
		"vcs.log.changes.compareBeforeWithLocal",
		"vcs.log.changes.jumpToSource",
		"vcs.log.changes.getFromRevision",
		"vcs.log.changes.revert",
		"vcs.log.changes.groupByDirectory",
		"vcs.log.changes.diffPreview",
		"vcs.log.goToRef",
		"vcs.log.revealBookmark",
		"vcs.log.toggleBookmark",
		"vcs.log.openExternal",
		"vcs.log.newTag",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("missing action %s", id)
		}
	}
}

func TestLogPreferencesUseOpenIDEADefaultsAndRoundTrip(t *testing.T) {
	var defaults LogPrefs
	if !defaults.showBranches() || defaults.collapseLinear() ||
		!defaults.groupByDirectory() || !defaults.diffPreview() {
		t.Fatalf("log defaults = %+v", defaults)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	falseValue := false
	want := LogPrefs{
		Sort: "topo", CollapseLinear: &falseValue, ShowBranches: &falseValue,
		GroupByDirectory: &falseValue, DiffPreview: &falseValue,
		DiffLocation: "bottom", ColumnOrder: []string{"author", "hash"},
	}
	if err := (Preferences{VCS: VCSPrefs{Log: want}}).Save(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadPreferences(path).VCS.Log; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored log prefs = %+v, want %+v", got, want)
	}
}

func TestGitLogBranchFavoriteIsRepositoryScoped(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg: Config{SettingsPath: path},
		log: LogPane{
			branches: []openidea.Branch{{Name: "main", Current: true}},
		},
	}
	m.setRoot(root)
	m.ToggleFavoriteGitLogBranch(nil)
	key := gitLogScopedKey(root, "main")
	if !m.log.favoriteBranches["main"] || !LoadPreferences(path).VCS.FavoriteBranches[key] {
		t.Fatalf("branch favorite = %+v / stored %+v",
			m.log.favoriteBranches, LoadPreferences(path).VCS.FavoriteBranches)
	}
	m.ToggleFavoriteGitLogBranch(nil)
	if m.log.favoriteBranches["main"] || LoadPreferences(path).VCS.FavoriteBranches[key] {
		t.Fatalf("branch favorite was not removed: %+v", LoadPreferences(path).VCS.FavoriteBranches)
	}
}

func TestCommitBookmarkPathAndTogglePersist(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "settings.json")
	commit := openidea.Commit{ID: "abcdef123456", ShortID: "abcdef1", Summary: "Fix the log"}
	m := &Model{
		cfg: Config{SettingsPath: path},
		log: LogPane{selected: 0, commits: []openidea.Commit{commit}},
	}
	m.setRoot(root)
	bookmark := commitBookmarkPath(root, commit.ID)
	target, ok := parseCommitBookmark(bookmark)
	if !ok || filepath.Clean(target.repo) != filepath.Clean(root) || target.id != commit.ID {
		t.Fatalf("parsed bookmark = %+v, %v", target, ok)
	}
	m.ToggleCommitBookmark(nil)
	key := gitLogScopedKey(root, commit.ID)
	if !m.commitIsBookmarked(&commit) || LoadPreferences(path).VCS.CommitBookmarks[key] != commit.Summary {
		t.Fatalf("commit bookmark = %+v / stored %+v",
			m.log.bookmarks, LoadPreferences(path).VCS.CommitBookmarks)
	}
	m.ToggleCommitBookmark(nil)
	if m.commitIsBookmarked(&commit) {
		t.Fatal("commit bookmark was not removed")
	}
}

func TestParseCommitBookmarkAcceptsBridgeFormats(t *testing.T) {
	cases := []struct {
		arg  any
		repo string
		id   string
	}{
		{"git-log:%2Ftmp%2Frepo#abcdef", "/tmp/repo", "abcdef"},
		{"/tmp/repo\x00abcdef", "/tmp/repo", "abcdef"},
		{map[string]any{"repo": "/tmp/repo", "id": "abcdef"}, "/tmp/repo", "abcdef"},
	}
	for _, c := range cases {
		target, ok := parseCommitBookmark(c.arg)
		if !ok || target.repo != c.repo || target.id != c.id {
			t.Errorf("parseCommitBookmark(%v) = %+v, %v", c.arg, target, ok)
		}
	}
}
