package app

import (
	"path/filepath"
	"testing"
)

// groupModel is a model editing two window groups without a backend: the
// focused group shows a and b, the split group c.
func groupModel(a, b, c *Tab) *Model {
	m := toolModel()
	m.tabs = []*Tab{a, b}
	m.active = 1
	m.split = &splitGroup{tabs: []*Tab{c}, active: 0}
	return m
}

func TestFocusGroupSwapsThePanesAndTheirPositions(t *testing.T) {
	a, b, c := &Tab{}, &Tab{}, &Tab{}
	m := groupModel(a, b, c)
	m.tabsShown = 3
	m.split.shown = 7

	m.focusGroup(groupSplit)

	if len(m.tabs) != 1 || m.tabs[0] != c {
		t.Fatalf("focused group holds %d tabs, want only c", len(m.tabs))
	}
	if m.active != 0 || m.tabsShown != 7 {
		t.Fatalf("focused group active=%d shown=%d, want 0 and 7", m.active, m.tabsShown)
	}
	if len(m.split.tabs) != 2 || m.split.tabs[0] != a || m.split.active != 1 {
		t.Fatalf("other group: %d tabs, active %d", len(m.split.tabs), m.split.active)
	}
	if m.split.shown != 3 {
		t.Fatalf("other group shown = %d, want the focused group's 3", m.split.shown)
	}
	// The groups keep their places on screen: the side they are drawn on
	// flips with the contents.
	if !m.splitFirst {
		t.Fatal("the focused pane did not move to where the other one was")
	}
	if g, ok := m.groupOf(c); !ok || g != groupFocused {
		t.Fatalf("c is in group %v (%v), want the focused group", g, ok)
	}
}

func TestFocusGroupWithoutASplitDoesNothing(t *testing.T) {
	a := &Tab{}
	m := toolModel()
	m.tabs = []*Tab{a}
	m.focusGroup(groupSplit)
	if len(m.tabs) != 1 || m.split != nil || m.splitFirst {
		t.Fatalf("a lone group changed: tabs=%d split=%v first=%v", len(m.tabs), m.split, m.splitFirst)
	}
}

func TestSettleGroupsCollapsesAnEmptyPane(t *testing.T) {
	a, b, c := &Tab{}, &Tab{}, &Tab{}

	// A split without tabs is gone.
	m := groupModel(a, b, c)
	m.split.tabs = nil
	m.settleGroups()
	if m.split != nil || m.splitFirst {
		t.Fatal("an empty split survived")
	}
	if len(m.tabs) != 2 || m.active != 1 {
		t.Fatalf("the focused group changed: %d tabs, active %d", len(m.tabs), m.active)
	}

	// A focused group without tabs leaves the other group as the editor.
	m = groupModel(a, b, c)
	m.tabs = nil
	m.active = 0
	m.settleGroups()
	if m.split != nil || m.splitFirst {
		t.Fatal("the collapsed groups kept a split")
	}
	if len(m.tabs) != 1 || m.tabs[0] != c {
		t.Fatalf("the surviving group holds %d tabs, want c", len(m.tabs))
	}
}

func TestRemoveFromGroupKeepsItsActiveTabValid(t *testing.T) {
	a, b, c := &Tab{}, &Tab{}, &Tab{}
	m := groupModel(a, b, c)

	// Removing a tab after the active one leaves the active tab alone.
	if at := m.removeFromGroup(groupSplit, c); at != 0 {
		t.Fatalf("remove c at %d", at)
	}
	if m.split.active != 0 {
		t.Fatalf("active = %d, want 0", m.split.active)
	}

	// In the focused group, removing the active tab of two picks the one that
	// takes its place.
	if at := m.removeFromGroup(groupFocused, b); at != 1 {
		t.Fatalf("remove b at %d", at)
	}
	if m.active != 0 || len(m.tabs) != 1 || m.tabs[0] != a {
		t.Fatalf("after removing the active tab: active=%d tabs=%d", m.active, len(m.tabs))
	}

	// Removing a tab before the active one shifts it down.
	m.tabs = []*Tab{a, b, c}
	m.active = 2
	if at := m.removeFromGroup(groupFocused, a); at != 0 {
		t.Fatalf("remove a at %d", at)
	}
	if m.active != 1 {
		t.Fatalf("active = %d, want 1", m.active)
	}

	// Removing the last tab leaves the group empty and its active index at 0.
	m.tabs = []*Tab{a}
	m.removeFromGroup(groupFocused, a)
	if len(m.tabs) != 0 || m.active != 0 {
		t.Fatalf("empty group: %d tabs, active %d", len(m.tabs), m.active)
	}

	// A tab the group does not hold reports -1.
	if at := m.removeFromGroup(groupFocused, b); at != -1 {
		t.Fatalf("missing tab removed at %d", at)
	}
}

func TestSessionKeepsTheSplitGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := Session{
		SplitFiles:  []string{"/p/Main.java", "/p/Util.java"},
		SplitActive: 1,
		SplitDown:   true,
		SplitFirst:  true,
	}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got := LoadSession(path)
	if len(got.SplitFiles) != 2 || got.SplitFiles[0] != "/p/Main.java" || got.SplitFiles[1] != "/p/Util.java" {
		t.Fatalf("split files = %v", got.SplitFiles)
	}
	if got.SplitActive != 1 || !got.SplitDown || !got.SplitFirst {
		t.Fatalf("split state: active=%d down=%v first=%v", got.SplitActive, got.SplitDown, got.SplitFirst)
	}
}
