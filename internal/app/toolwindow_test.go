package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func toolModel() *Model {
	m := &Model{
		problems:    ProblemsPane{showErr: true, showWarn: true, showInfo: true},
		problemStep: -1,
	}
	// The terminal's toggle starts a shell on first use; the window
	// bookkeeping tests never get that far.
	m.term.starting = true
	return m
}

func TestToolWindowRemembersTheLastOneShown(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	if m.tool != "run" || !m.run.visible {
		t.Fatalf("ShowRun: tool=%q visible=%v", m.tool, m.run.visible)
	}
	m.ShowBuild()
	if m.tool != "build" || !m.build.visible {
		t.Fatalf("ShowBuild: tool=%q visible=%v", m.tool, m.build.visible)
	}
}

func TestHideActiveHidesOnlyTheLastOne(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ShowBuild()
	m.HideActiveToolWindow()
	if m.build.visible || !m.run.visible {
		t.Fatalf("hide active hid the wrong window: run=%v build=%v", m.run.visible, m.build.visible)
	}
	// The last-used mark survives the hide, so F12 can bring it back.
	if m.tool != "build" {
		t.Fatalf("tool = %q, want build", m.tool)
	}
	m.HideActiveToolWindow()
	if !m.run.visible {
		t.Fatal("hide active closed run, which is not the last-used window")
	}
}

func TestHideAllRestoresExactlyWhatItHid(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ShowBuild()
	m.HideAllToolWindows()
	if m.run.visible || m.build.visible || m.term.visible || m.problems.visible {
		t.Fatalf("hide all left a window: %+v", m)
	}
	m.HideAllToolWindows()
	if !m.run.visible || !m.build.visible {
		t.Fatalf("restore did not bring back run and build: run=%v build=%v", m.run.visible, m.build.visible)
	}
	if m.term.visible || m.problems.visible || m.debug.visible || m.log.visible {
		t.Fatal("restore opened a window that was not hidden")
	}
}

func TestJumpToLastToolWindowReopensIt(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.HideAllToolWindows()
	m.JumpToLastToolWindow()
	if !m.problems.visible {
		t.Fatal("F12 did not reopen the last tool window")
	}
}

func TestStepProblemWalksTheRowsAndWraps(t *testing.T) {
	m := problemsModel(map[string]map[string][]openidea.Diagnostic{
		"/p/A.java": {"jdtls": {
			diag(2, 0, "warning", "late"),
			diag(1, 0, "error", "first"),
		}},
	})
	m.problems.project = true
	first, ok := m.nextProblem(1)
	if !ok || first.Message != "first" {
		t.Fatalf("first step = %+v, ok=%v", first, ok)
	}
	second, _ := m.nextProblem(1)
	if second.Message != "late" {
		t.Fatalf("second step = %q", second.Message)
	}
	wrapped, _ := m.nextProblem(1)
	if wrapped.Message != "first" {
		t.Fatalf("wrap = %q", wrapped.Message)
	}
	back, _ := m.nextProblem(-1)
	if back.Message != "late" {
		t.Fatalf("prev from the first row = %q", back.Message)
	}
}

func TestStepProblemWithoutProblemsDoesNothing(t *testing.T) {
	m := problemsModel(nil)
	m.problems.project = true
	if _, ok := m.nextProblem(1); ok {
		t.Fatal("a problem was returned with no diagnostics")
	}
	if m.problemStep != -1 {
		t.Fatalf("problemStep = %d after an empty step", m.problemStep)
	}
}
