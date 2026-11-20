package app

import (
	"testing"

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

func TestVCSOperationsQuickListFiltersAndDispatches(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
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
