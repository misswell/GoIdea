package app

import "testing"

func TestSplitShowsTwoBottomWindowsAtOnce(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	if m.primaryTool() != "" || m.splitTool() != "run" {
		t.Fatalf("after the move: primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
	m.ShowProblems()
	if m.primaryTool() != "problems" || m.splitTool() != "run" {
		t.Fatalf("two areas: primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
	if !m.problems.visible || !m.run.visible {
		t.Fatalf("a window was closed: problems=%v run=%v", m.problems.visible, m.run.visible)
	}
}

func TestAnAreaReplacesOnlyItsOwnWindow(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	m.ShowBuild()
	if m.primaryTool() != "build" || m.splitTool() != "run" {
		t.Fatalf("primary replacement: primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
	if m.problems.visible {
		t.Fatal("the replaced primary window stayed open")
	}
	if !m.run.visible {
		t.Fatal("replacing the primary window closed the split one")
	}
	m.ShowTerm()
	// The terminal is a primary window too, so it replaces the build.
	if m.primaryTool() != "term" || !m.run.visible {
		t.Fatalf("after the terminal: primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
}

func TestMovingAWindowTakesTheTargetArea(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	// Problems takes the primary area, and moving it into the split replaces
	// the run there, as Move to › Bottom Right does.
	m.MoveActiveToolWindowToSplit()
	if m.visibleTool() != "problems" || m.splitTool() != "problems" {
		t.Fatalf("after moving problems: primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
	if m.run.visible {
		t.Fatal("the replaced split window stayed open")
	}
	m.MoveActiveToolWindowToBottom()
	if m.primaryTool() != "problems" || m.splitTool() != "" {
		t.Fatalf("after moving back: primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
}

func TestHideAllRestoresBothAreas(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	m.HideAllToolWindows()
	if m.primaryTool() != "" || m.splitTool() != "" {
		t.Fatal("hide all left a window open")
	}
	m.HideAllToolWindows()
	if m.primaryTool() != "problems" || m.splitTool() != "run" {
		t.Fatalf("restore: primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
}

func TestMaximizeKeepsBothAreas(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	m.ToggleMaximizeToolWindow()
	if !m.maximized || m.primaryTool() != "problems" || m.splitTool() != "run" {
		t.Fatalf("maximize: %v primary=%q split=%q", m.maximized, m.primaryTool(), m.splitTool())
	}
	m.ToggleMaximizeToolWindow()
	if m.maximized || m.primaryTool() != "problems" || m.splitTool() != "run" {
		t.Fatal("restoring the size lost an area")
	}
}

func TestMoveWithoutAVisibleWindowDoesNothing(t *testing.T) {
	m := toolModel()
	m.MoveActiveToolWindowToSplit()
	if len(m.splitNames) != 0 {
		t.Fatalf("a closed window moved: %v", m.splitNames)
	}
}
