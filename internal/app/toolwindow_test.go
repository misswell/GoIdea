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

func TestOneWindowHoldsTheBottomSlot(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ShowProblems()
	if !m.problems.visible || m.run.visible {
		t.Fatalf("opening problems did not take the slot: run=%v problems=%v", m.run.visible, m.problems.visible)
	}
	if m.tool != "problems" {
		t.Fatalf("tool = %q, want problems", m.tool)
	}
	// A second Cmd+6 closes the window it is on, not the one it replaced.
	m.ToggleProblems()
	if m.problems.visible || m.visibleTool() != "" {
		t.Fatalf("second press left a window open: %q", m.visibleTool())
	}
}

func TestShowRunAgainClosesItAndF12Reopens(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ShowRun()
	if m.run.visible {
		t.Fatal("a second Cmd+4 did not close the Run window")
	}
	// The last-used mark survives the hide, so F12 brings it back.
	if m.tool != "run" {
		t.Fatalf("tool = %q after hiding, want run", m.tool)
	}
	m.JumpToLastToolWindow()
	if !m.run.visible {
		t.Fatal("F12 did not reopen the Run window")
	}
}

func TestHideActiveHidesTheWindowInTheSlot(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.HideActiveToolWindow()
	if m.problems.visible {
		t.Fatal("hide active left the Problems window open")
	}
	if m.tool != "problems" {
		t.Fatalf("tool = %q, want problems", m.tool)
	}
	// Nothing is showing, so a second press changes nothing.
	m.HideActiveToolWindow()
	if m.problems.visible {
		t.Fatal("hide active reopened a window")
	}
}

func TestHideAllRestoresTheSlot(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ShowProblems()
	m.HideAllToolWindows()
	if m.run.visible || m.problems.visible {
		t.Fatal("hide all left a window open")
	}
	m.HideAllToolWindows()
	if !m.problems.visible {
		t.Fatal("restore did not bring back the window that held the slot")
	}
	if m.run.visible {
		t.Fatal("restore brought back a window the slot had replaced")
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
