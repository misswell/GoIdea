package app

import (
	"strings"
	"testing"
)

func TestDockDropSlotChoosesTheEdgeAndSplit(t *testing.T) {
	cases := []struct {
		x, y, w, h float32
		want       dockSlot
		ok         bool
	}{
		{10, 100, 1200, 800, dockLeft, true},
		{10, 700, 1200, 800, dockLeftSplit, true},
		{1190, 100, 1200, 800, dockRight, true},
		{1190, 700, 1200, 800, dockRightSplit, true},
		{600, 700, 1200, 800, dockBottom, true},
		{900, 700, 1200, 800, dockBottomSplit, true},
		{600, 300, 1200, 800, 0, false},
	}
	for _, c := range cases {
		got, ok := dockDropSlot(c.x, c.y, c.w, c.h)
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("drop at (%v,%v) = %v, %v", c.x, c.y, got, ok)
		}
	}
}

func TestDockDragGhostMovesOnRelease(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.dockDragGhost("problems", 600, 300, 1200, 800, true)
	if m.dockDragArmed {
		t.Fatal("a drop in the middle armed a move")
	}
	m.dockDragGhost("problems", 600, 300, 1200, 800, false)
	if m.dock.slots[dockBottom] != "problems" {
		t.Fatalf("a centre drag moved the window: %v", m.dock.slots)
	}

	m.dockDragGhost("problems", 10, 100, 1200, 800, true)
	if !m.dockDragArmed || !strings.Contains(m.status, "Left Top") {
		t.Fatalf("the edge was not previewed: %q", m.status)
	}
	m.dockDragGhost("problems", 10, 100, 1200, 800, false)
	if m.dock.slots[dockLeft] != "problems" {
		t.Fatalf("the release did not dock left: %v", m.dock.slots)
	}
	if !m.problems.visible {
		t.Fatal("the moved window lost its content")
	}
}

func TestDockDropOntoAPinnedSlotDiverts(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.ShowTodo()
	m.DockMove("todo", dockRight)
	m.SetToolWindowPinned("todo", true)
	m.ShowProblems()
	m.dockDragGhost("problems", 1190, 100, 1200, 800, true)
	m.dockDragGhost("problems", 1190, 100, 1200, 800, false)
	if m.dock.slots[dockRight] != "todo" || m.dock.slots[dockRightSplit] != "problems" {
		t.Fatalf("the pinned slot was not protected: %v", m.dock.slots)
	}
}
