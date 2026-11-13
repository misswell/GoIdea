package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func statusOf(path, staged, unstaged string) openidea.FileStatus {
	f := openidea.FileStatus{Path: path, RelPath: path}
	if staged != "" {
		s := staged
		f.Staged = &s
	}
	if unstaged != "" {
		u := unstaged
		f.Unstaged = &u
	}
	return f
}

func TestCommitFileIncludesOnlyItsFile(t *testing.T) {
	m := &Model{}
	m.commit = &commitState{only: "/p/a.go", checks: map[string]bool{}}
	if !m.commitIncluded(m.commit, statusOf("/p/a.go", "", "modified")) {
		t.Fatalf("the named file stayed out of its own commit")
	}
	if m.commitIncluded(m.commit, statusOf("/p/b.go", "", "modified")) {
		t.Fatalf("another changed file joined a single-file commit")
	}
	if !m.commitIncluded(m.commit, statusOf("/p/a.go", "", "untracked")) {
		t.Fatalf("a new file cannot be committed alone")
	}
	if m.commitIncluded(m.commit, statusOf("/p/a.go", "", "conflicted")) {
		t.Fatalf("a conflicted file joined a single-file commit")
	}
}

func TestCommitFileKeepsTheOrdinaryRulesWithoutIt(t *testing.T) {
	m := &Model{}
	c := &commitState{checks: map[string]bool{}}
	if !m.commitIncluded(c, statusOf("/p/a.go", "", "modified")) {
		t.Fatalf("a modified file is not included by default")
	}
	if m.commitIncluded(c, statusOf("/p/new.go", "", "untracked")) {
		t.Fatalf("an unversioned file is included without opting in")
	}
	c.checks["/p/new.go"] = true
	if !m.commitIncluded(c, statusOf("/p/new.go", "", "untracked")) {
		t.Fatalf("an opted-in file stayed out")
	}
}

func TestStatusForFindsAChangedFile(t *testing.T) {
	m := &Model{vcs: VCSPane{files: []openidea.FileStatus{statusOf("/p/a.go", "", "modified")}}}
	f, ok := m.statusFor("/p/a.go")
	if !ok || f.Path != "/p/a.go" {
		t.Fatalf("statusFor = %+v, %v", f, ok)
	}
	if _, ok := m.statusFor("/p/b.go"); ok {
		t.Fatalf("statusFor invented a change")
	}
}
