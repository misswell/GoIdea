package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func TestRightSideHoldsTwoWindowsAndKeepsContent(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	m.ShowTodo()
	m.DockMove("todo", dockRightSplit)
	if !m.dockWindowShown("problems") || !m.dockWindowShown("todo") {
		t.Fatalf("the right slots: problems=%v todo=%v", m.dockWindowShown("problems"), m.dockWindowShown("todo"))
	}
	if m.rightTool != "problems" || m.rightSplitTool != "todo" {
		t.Fatalf("mirrors: right=%q split=%q", m.rightTool, m.rightSplitTool)
	}
	if !m.problems.visible || !m.todo.visible {
		t.Fatalf("content lost on the way: problems=%v todo=%v", m.problems.visible, m.todo.visible)
	}
	m.DockHide("problems")
	if m.dockWindowShown("problems") || !m.dockWindowShown("todo") {
		t.Fatalf("closing the primary right window took the other: %v %v",
			m.dockWindowShown("problems"), m.dockWindowShown("todo"))
	}
	m.DockHide("todo")
}

func TestAWindowNeverSitsInTwoSlots(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.DockMove("problems", dockBottomSplit)
	m.ShowTodo()
	m.DockMove("todo", dockBottomSplit)
	count := 0
	for s := dockSlot(0); s < dockSlotCount; s++ {
		if m.dock.slots[s] == "problems" {
			count++
		}
	}
	if count > 1 {
		t.Fatalf("problems sits in %d slots", count)
	}
	if m.dockWindowShown("problems") {
		t.Fatalf("the replaced window stayed content-open")
	}
	if !m.todo.visible {
		t.Fatalf("the new split window has no content")
	}
}

func TestHideAllRestoresBothRightSlots(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	m.ShowTodo()
	m.DockMove("todo", dockRightSplit)
	m.HideAllToolWindows()
	if m.dockRightShown() || m.problems.visible || m.todo.visible {
		t.Fatalf("hide all left windows: right=%v p=%v t=%v", m.dockRightShown(), m.problems.visible, m.todo.visible)
	}
	m.HideAllToolWindows()
	if !m.dockWindowShown("problems") || !m.dockWindowShown("todo") {
		t.Fatalf("restore missed the right slots: %v %v",
			m.dockWindowShown("problems"), m.dockWindowShown("todo"))
	}
	if !m.problems.visible || !m.todo.visible {
		t.Fatalf("restored windows have no content")
	}
}

func TestDockLayoutRoundTrips(t *testing.T) {
	m := toolModel()
	m.setRoot(t.TempDir())
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	m.ShowTodo()
	m.DockMove("todo", dockRightSplit)
	m.rightRatio = 0.35
	m.rememberLayout()

	l := normalizeLayout(m.currentLayout())
	if l.Dock == nil || l.Dock.Version != dockLayoutVersion {
		t.Fatalf("no versioned dock: %+v", l.Dock)
	}
	restored := toolModel()
	restored.layouts = map[string]Layout{layoutKey(m.Root()): l}
	restored.applyLayout(m.Root())
	if !restored.dockWindowShown("problems") || !restored.dockWindowShown("todo") {
		t.Fatalf("restored dock: %v", restored.dock.slots)
	}
	if restored.rightTool != "problems" || restored.rightSplitTool != "todo" {
		t.Fatalf("restored mirrors: %q %q", restored.rightTool, restored.rightSplitTool)
	}
	if restored.rightShare() != 0.35 {
		t.Fatalf("ratio = %v", restored.rightShare())
	}
	if !restored.problems.visible || !restored.todo.visible {
		t.Fatalf("restored content: %v %v", restored.problems.visible, restored.todo.visible)
	}
}

func TestLegacyLayoutMigratesIntoTheDock(t *testing.T) {
	legacy := Layout{Bottom: []string{"problems"}, Right: "hierarchy", RightWidth: 320}
	m := toolModel()
	m.setRoot(t.TempDir())
	m.layouts = map[string]Layout{layoutKey(m.Root()): legacy}
	m.applyLayout(m.Root())
	if m.dock.slots[dockBottom] != "problems" || !m.problems.visible {
		t.Fatalf("legacy bottom did not migrate: %v", m.dock.slots)
	}
	if m.dock.slots[dockRight] != "hierarchy" || !m.dockWindowShown("hierarchy") {
		t.Fatalf("legacy right did not migrate: %v", m.dock.slots)
	}
}

func TestProblemsRenderOnTheRight(t *testing.T) {
	m := problemsModel(map[string]map[string][]openidea.Diagnostic{
		"/p/A.java": {"jdtls": {diag(1, 0, "error", "right side failure")}},
	})
	m.problems.project = true
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	tt := workbenchTester(t, m)
	if !tt.HasText("right side failure") {
		t.Fatalf("the right Problems window is blank: %q", tt.Texts())
	}
}
