package app

import (
	"testing"

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
	if got := m.branchLabel(); got != "正在变基 main" {
		t.Fatalf("label = %q, want the rebase wording", got)
	}
	m.branch.head = nil
	if got := m.branchLabel(); got != "正在变基" {
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
