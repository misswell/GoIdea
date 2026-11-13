package app

import "testing"

func TestMaximizeTakesTheWorkbenchAndGivesItBack(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ToggleMaximizeToolWindow()
	if !m.maximized || !m.run.visible {
		t.Fatalf("maximize: maximized=%v run=%v", m.maximized, m.run.visible)
	}
	m.ToggleMaximizeToolWindow()
	if m.maximized {
		t.Fatal("a second press did not restore the editor")
	}
	if !m.run.visible {
		t.Fatal("restoring the size closed the window")
	}
}

func TestMaximizeReopensTheLastWindow(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.HideActiveToolWindow()
	m.ToggleMaximizeToolWindow()
	if !m.run.visible || !m.maximized {
		t.Fatalf("maximize did not bring back the last window: run=%v maximized=%v", m.run.visible, m.maximized)
	}
}

func TestMaximizeWithNoLastWindowDoesNothing(t *testing.T) {
	m := toolModel()
	m.ToggleMaximizeToolWindow()
	if m.maximized || m.visibleTool() != "" {
		t.Fatalf("maximize opened something: visible=%q maximized=%v", m.visibleTool(), m.maximized)
	}
}

func TestHidingTheMaximizedWindowGivesTheEditorBack(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.ToggleMaximizeToolWindow()
	m.HideActiveToolWindow()
	if m.maximized || m.problems.visible {
		t.Fatalf("hide active left maximized=%v problems=%v", m.maximized, m.problems.visible)
	}
}

func TestHideAllRestoresMaximized(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.ToggleMaximizeToolWindow()
	m.HideAllToolWindows()
	if m.maximized || m.problems.visible {
		t.Fatal("hide all left the window maximized")
	}
	m.HideAllToolWindows()
	if !m.problems.visible || !m.maximized {
		t.Fatalf("restore lost the maximum: visible=%v maximized=%v", m.problems.visible, m.maximized)
	}
}

func TestResizeMovesTheBottomEdgeOneStep(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	if got := m.bottomSize(); got != bottomDefaultHeight {
		t.Fatalf("default height = %d, want %d", got, bottomDefaultHeight)
	}
	m.ResizeBottomToolWindow("up")
	if m.bottomHeight != bottomDefaultHeight+bottomStep {
		t.Fatalf("up: height = %d", m.bottomHeight)
	}
	m.ResizeBottomToolWindow("down")
	if m.bottomHeight != bottomDefaultHeight {
		t.Fatalf("down: height = %d", m.bottomHeight)
	}
	m.bottomHeight = bottomMin
	m.ResizeBottomToolWindow("down")
	if m.bottomHeight != bottomMin {
		t.Fatalf("shrinking below the minimum changed the height to %d", m.bottomHeight)
	}
}

func TestResizeCapsAtTheFrame(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.bottomHeight = 600
	m.frameHeight = 500
	m.ResizeBottomToolWindow("up")
	if want := 500 - 160; m.bottomHeight != want {
		t.Fatalf("height = %d, want the frame cap %d", m.bottomHeight, want)
	}
	// Without a drawn frame the step still applies.
	m.frameHeight = 0
	m.bottomHeight = 300
	m.ResizeBottomToolWindow("up")
	if m.bottomHeight != 300+bottomStep {
		t.Fatalf("height = %d without a frame", m.bottomHeight)
	}
}

func TestResizeIgnoresSidewaysArrows(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ResizeBottomToolWindow("left")
	m.ResizeBottomToolWindow("right")
	if m.bottomHeight != 0 {
		t.Fatalf("a sideways arrow resized the bottom slot to %d", m.bottomHeight)
	}
}

func TestResizeWaitsForAVisibleWindow(t *testing.T) {
	m := toolModel()
	m.ResizeBottomToolWindow("up")
	if m.bottomHeight != 0 {
		t.Fatalf("resize moved a closed slot to %d", m.bottomHeight)
	}
}

func TestResizeDoesNothingWhileMaximized(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ToggleMaximizeToolWindow()
	m.ResizeBottomToolWindow("up")
	if m.bottomHeight != 0 {
		t.Fatalf("resize changed the size of a maximized window to %d", m.bottomHeight)
	}
}

func TestRevealEditorRestoresTheWorkbench(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.ToggleMaximizeToolWindow()
	m.revealEditor()
	if m.maximized {
		t.Fatal("editor navigation left the tool window maximized")
	}
	if !m.run.visible {
		t.Fatal("editor navigation closed the tool window")
	}
}
