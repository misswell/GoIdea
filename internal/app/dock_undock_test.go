package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestUndockOverlaysTheEditorWithoutTakingItsTrack(t *testing.T) {
	m := toolModel()
	m.setRoot(t.TempDir())
	m.ShowProblems()
	m.SetToolViewMode("problems", "undock")

	if got := m.toolViewMode("problems"); got != "undock" {
		t.Fatalf("View Mode = %q, want undock", got)
	}
	if m.bottomTool() != "" {
		t.Fatalf("the undocked window still owns the bottom track: %q", m.bottomTool())
	}
	if !m.dockWindowShown("problems") || !m.undockedWindow("problems") {
		t.Fatal("the undocked window lost its content or mode")
	}

	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1200, 800)
	tt.Frame()
	if got := tt.Texts(); !containsText(got, "Problems") {
		t.Fatalf("the undocked window was not drawn: %v", got)
	}

	m.SetToolViewMode("problems", "dockPinned")
	if m.undockedWindow("problems") || !m.dockWindowShown("problems") {
		t.Fatal("Dock Pinned did not return the window to its track")
	}
	m.rememberLayout()
	layout := normalizeLayout(m.currentLayout())
	if layout.Dock == nil || layout.Dock.ViewModes["problems"] != "" {
		t.Fatalf("Dock Pinned remained in the layout: %+v", layout.Dock)
	}
}

func TestUndockPersistsAndRestores(t *testing.T) {
	m := toolModel()
	m.setRoot(t.TempDir())
	m.ShowProblems()
	m.SetToolViewMode("problems", "undock")
	m.rememberLayout()
	layout := normalizeLayout(m.currentLayout())
	if layout.Dock == nil || layout.Dock.ViewModes["problems"] != "undock" {
		t.Fatalf("Undock is not in the layout: %+v", layout.Dock)
	}

	restored := toolModel()
	restored.layouts = map[string]Layout{layoutKey(m.Root()): layout}
	restored.applyLayout(m.Root())
	if restored.toolViewMode("problems") != "undock" {
		t.Fatalf("restored View Mode = %q", restored.toolViewMode("problems"))
	}
	if !restored.undockedWindow("problems") {
		t.Fatal("the restored undocked window is not shown over the editor")
	}
}

func TestUndockedWindowLeavesTheSideTrack(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.DockMove("problems", dockRight)
	m.SetToolViewMode("problems", "undock")
	if m.rightShown() {
		t.Fatal("an undocked right window still reserves the right side")
	}
	if m.remainingDockOccupant(dockRight) != "" {
		t.Fatal("the undocked window still occupies its right slot")
	}
}

func containsText(texts []string, want string) bool {
	for _, text := range texts {
		if text == want {
			return true
		}
	}
	return false
}
