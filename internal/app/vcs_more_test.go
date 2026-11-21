package app

import (
	"strings"
	"testing"

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
	m := &Model{
		commit: &commitState{checks: map[string]bool{}},
		log: LogPane{commits: []openidea.Commit{{
			ID: "abc", ShortID: "abc", Summary: "Fix timeout",
			Message: "Fix timeout\n\nExplain the cause.", Author: "Ada",
		}}},
	}
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
