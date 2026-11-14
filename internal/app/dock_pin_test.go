package app

import "testing"

func TestPinnedWindowKeepsItsSlot(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	m.SetToolWindowPinned("problems", true)
	m.ShowTodo()
	m.DockMove("todo", dockRight)
	if m.dock.slots[dockRight] != "problems" || m.dock.slots[dockRightSplit] != "todo" {
		t.Fatalf("the pinned window lost its slot: %v", m.dock.slots)
	}
	if !m.problems.visible || !m.todo.visible {
		t.Fatalf("a window's content closed: problems=%v todo=%v", m.problems.visible, m.todo.visible)
	}

	// With both right slots pinned, a third right window waits.
	m.SetToolWindowPinned("todo", true)
	m.DockShow("hierarchy")
	if m.dockWindowShown("hierarchy") {
		t.Fatalf("hierarchy opened over pinned windows: %v", m.dock.slots)
	}
	if m.status == "" {
		t.Fatal("the refusal was silent")
	}
}

func TestUnpinLetsAnotherWindowReplace(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	m.SetToolWindowPinned("problems", true)
	m.SetToolWindowPinned("problems", false)
	m.DockShow("hierarchy")
	if m.dock.slots[dockRight] != "hierarchy" {
		t.Fatalf("the unpinned slot kept its window: %v", m.dock.slots)
	}
	if m.problems.visible {
		t.Fatal("the replaced window is still open")
	}
}

func TestPinTogglesThroughTheCommand(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.tool = "problems"
	if !m.RunCommand("view.pinToolWindow") || !m.dockPinned("problems") {
		t.Fatal("the command did not pin")
	}
	if !m.RunCommand("view.pinToolWindow") || m.dockPinned("problems") {
		t.Fatal("the command did not unpin")
	}
}

func TestPinnedStatePersists(t *testing.T) {
	m := toolModel()
	m.setRoot(t.TempDir())
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	m.SetToolWindowPinned("problems", true)
	m.rememberLayout()
	l := normalizeLayout(m.currentLayout())
	if l.Dock == nil || len(l.Dock.Pinned) != 1 || l.Dock.Pinned[0] != "problems" {
		t.Fatalf("the pin is not in the layout: %+v", l.Dock)
	}
	restored := toolModel()
	restored.layouts = map[string]Layout{layoutKey(m.Root()): l}
	restored.applyLayout(m.Root())
	if !restored.dockPinned("problems") {
		t.Fatalf("the pin was not restored: %v", restored.dock.pinned)
	}
}

func TestDockUnpinnedHidesWhenAnotherToolWindowTakesFocus(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	if m.toolViewMode("problems") != "dockPinned" {
		t.Fatal("the default View Mode is not Dock Pinned")
	}
	m.SetToolViewMode("problems", "dockUnpinned")
	m.DockShow("hierarchy")
	if m.dockWindowShown("problems") {
		t.Fatalf("a Dock Unpinned window stayed: %v", m.dock.slots)
	}
	if !m.dockWindowShown("hierarchy") {
		t.Fatal("the newly focused window closed")
	}
}

func TestViewModePersistsAndUndockIsRefused(t *testing.T) {
	m := toolModel()
	m.setRoot(t.TempDir())
	m.ShowProblems()
	m.SetToolViewMode("problems", "dockUnpinned")
	m.rememberLayout()
	l := normalizeLayout(m.currentLayout())
	if l.Dock == nil || l.Dock.ViewModes["problems"] != "dockUnpinned" {
		t.Fatalf("the View Mode is not in the layout: %+v", l.Dock)
	}
	restored := toolModel()
	restored.layouts = map[string]Layout{layoutKey(m.Root()): l}
	restored.applyLayout(m.Root())
	if restored.toolViewMode("problems") != "dockUnpinned" {
		t.Fatalf("View Mode after restore = %q", restored.toolViewMode("problems"))
	}
	m.SetToolViewMode("problems", "undock")
	if m.toolViewMode("problems") != "dockUnpinned" || m.status == "" {
		t.Fatal("Undock changed the mode without a floating window")
	}
}
