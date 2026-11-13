package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func diag(line, col uint32, severity, message string) openidea.Diagnostic {
	return openidea.Diagnostic{
		Range:    openidea.OutlineRange{Start: openidea.OutlinePos{Line: line, Col: col}},
		Severity: severity,
		Message:  message,
	}
}

func problemsModel(files map[string]map[string][]openidea.Diagnostic) *Model {
	return &Model{
		diags:       files,
		problems:    ProblemsPane{showErr: true, showWarn: true, showInfo: true},
		problemStep: -1,
	}
}

func TestProblemSeverity(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"error", "error"},
		{"warning", "warning"},
		{"info", "info"},
		{"hint", "hint"},
		{"Warning", "warning"},
		{"fatal", "error"},
		{"", "error"},
	} {
		if got := problemSeverity(tc.in); got != tc.want {
			t.Errorf("problemSeverity(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestProblemGroupPutsHintsWithInformation(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"error", "error"},
		{"warning", "warning"},
		{"info", "info"},
		{"hint", "info"},
	} {
		if got := severityGroup(tc.in); got != tc.want {
			t.Errorf("severityGroup(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestProblemsOfSortsBySeverityThenPosition(t *testing.T) {
	m := problemsModel(map[string]map[string][]openidea.Diagnostic{
		"/p/A.java": {
			"jdtls": {
				diag(5, 0, "warning", "unused"),
				diag(9, 0, "error", "late error"),
				diag(2, 4, "error", "early error"),
			},
			"checkstyle": {
				diag(1, 0, "hint", "style"),
				diag(2, 1, "error", "early error first column"),
			},
		},
	})
	got := m.problemsOf("/p/A.java")
	want := []string{
		"early error first column", "early error", "late error", "unused", "style",
	}
	if len(got) != len(want) {
		t.Fatalf("problemsOf returned %d items, want %d", len(got), len(want))
	}
	for i, msg := range want {
		if got[i].Message != msg {
			t.Errorf("item %d = %q, want %q", i, got[i].Message, msg)
		}
	}
	if got[0].Source != "checkstyle" || got[3].Source != "jdtls" {
		t.Errorf("sources = %q and %q, want checkstyle and jdtls", got[0].Source, got[3].Source)
	}
	if got[0].Line != 2 || got[0].Col != 1 {
		t.Errorf("first position = %d:%d, want 2:1", got[0].Line, got[0].Col)
	}
	if items := m.problemsOf("/p/Missing.java"); items != nil {
		t.Errorf("unknown file returned %v", items)
	}
}

func TestProblemListFiltersAndPutsErrorsFirst(t *testing.T) {
	m := problemsModel(map[string]map[string][]openidea.Diagnostic{
		"/p/a/Only.java":   {"jdtls": {diag(3, 0, "warning", "unused field")}},
		"/p/b/Broken.java": {"jdtls": {diag(7, 2, "error", "cannot resolve")}},
	})
	m.problems.project = true

	groups := m.problemList()
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0][0].Path != "/p/b/Broken.java" || groups[1][0].Path != "/p/a/Only.java" {
		t.Errorf("order = %q then %q, want errors first", groups[0][0].Path, groups[1][0].Path)
	}

	m.problems.showWarn = false
	groups = m.problemList()
	if len(groups) != 1 || groups[0][0].Severity != "error" {
		t.Fatalf("warning filter left %v", groups)
	}

	m.problems.showErr = false
	if groups = m.problemList(); len(groups) != 0 {
		t.Fatalf("all filters off left %v", groups)
	}

	m.problems.showWarn = true
	m.problems.project = false
	if groups = m.problemList(); len(groups) != 0 {
		t.Fatalf("no active tab listed %v", groups)
	}
}

func TestProblemCounts(t *testing.T) {
	m := problemsModel(map[string]map[string][]openidea.Diagnostic{
		"/p/A.java": {"jdtls": {
			diag(1, 0, "error", "e1"),
			diag(2, 0, "warning", "w1"),
		}},
		"/p/B.java": {"jdtls": {
			diag(1, 0, "hint", "h1"),
			diag(2, 0, "info", "i1"),
			diag(3, 0, "error", "e2"),
		}},
	})
	errors, warnings, infos := m.problemCounts()
	if errors != 2 || warnings != 1 || infos != 2 {
		t.Errorf("counts = %d errors, %d warnings, %d infos, want 2, 1, 2",
			errors, warnings, infos)
	}
}
