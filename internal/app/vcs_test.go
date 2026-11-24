package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func str(s string) *string { return &s }

func TestVCSBadge(t *testing.T) {
	cases := []struct {
		name         string
		staged       *string
		unstaged     *string
		letter, word string
	}{
		{"modified in worktree", nil, str("modified"), "M", "Modified"},
		{"staged addition", str("added"), nil, "A", "Added"},
		{"staged and edited", str("added"), str("modified"), "M", "Modified"},
		{"deleted", nil, str("deleted"), "D", "Deleted"},
		{"renamed", str("renamed"), nil, "R", "Renamed"},
		{"conflicted", nil, str("conflicted"), "!", "Conflict"},
		{"untracked", nil, str("untracked"), "U", "Untracked"},
		{"ignored", nil, str("ignored"), "I", "Ignored"},
		{"no kind", nil, nil, "?", "Changed"},
	}
	for _, c := range cases {
		f := openidea.FileStatus{RelPath: "a.java", Staged: c.staged, Unstaged: c.unstaged}
		letter, word, tone := vcsBadge(f)
		if letter != c.letter || word != c.word || tone == "" {
			t.Errorf("%s: got %q %q %q, want %q %q", c.name, letter, word, tone, c.letter, c.word)
		}
	}
}

func TestVCSGroupsSplitByStagingState(t *testing.T) {
	files := []openidea.FileStatus{
		{Path: "/p/a.java", RelPath: "a.java", Staged: str("added")},
		{Path: "/p/b.java", RelPath: "b.java", Unstaged: str("modified")},
		{Path: "/p/c.java", RelPath: "c.java", Staged: str("added"), Unstaged: str("modified")},
		{Path: "/p/d.java", RelPath: "d.java"},
	}
	staged, unstaged := vcsStaged(files), vcsUnstaged(files)
	if len(staged) != 2 || staged[0].RelPath != "a.java" || staged[1].RelPath != "c.java" {
		t.Fatalf("staged group = %v, want a.java and c.java", staged)
	}
	if len(unstaged) != 2 || unstaged[0].RelPath != "b.java" || unstaged[1].RelPath != "c.java" {
		t.Fatalf("unstaged group = %v, want b.java and c.java", unstaged)
	}
}

func TestRollbackAsksBeforeDiscarding(t *testing.T) {
	m := &Model{}
	m.AskRollback(openidea.FileStatus{Path: "/p/a.java", RelPath: "a.java"})
	if m.rollback == nil || !m.rollback.open || len(m.rollback.files) != 1 ||
		m.rollback.files[0].Path != "/p/a.java" || m.rollback.name != "a.java" {
		t.Fatalf("rollback = %+v, want an open confirmation for a.java", m.rollback)
	}
	m.closeRollback()
	if m.rollback != nil {
		t.Fatalf("rollback still %+v after closing", m.rollback)
	}
	m.ConfirmRollback()
	if m.rollback != nil || m.status != "" {
		t.Fatalf("confirming without a dialog changed something: %+v, %q", m.rollback, m.status)
	}
}

func TestChangeActionsDoNothingWithoutAProject(t *testing.T) {
	m := &Model{}
	m.StageChange(openidea.FileStatus{Path: "/p/a.java", RelPath: "a.java"})
	m.UnstageChange(openidea.FileStatus{Path: "/p/a.java", RelPath: "a.java"})
	if m.vcs.busy || m.status != "" || m.vcs.opErr != "" {
		t.Fatalf("a change action ran without a project: busy=%v status=%q err=%q",
			m.vcs.busy, m.status, m.vcs.opErr)
	}
}
