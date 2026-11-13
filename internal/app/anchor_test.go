package app

import "testing"

func TestFindOpensInTheBottomSide(t *testing.T) {
	m := toolModel()
	m.ShowSearch()
	if !m.search.visible || m.primaryTool() != "find" {
		t.Fatalf("find: visible=%v primary=%q", m.search.visible, m.primaryTool())
	}
	if m.sideMode != "" {
		t.Fatalf("find took the sidebar: side=%q", m.sideMode)
	}
}

func TestFindLeavesTheSplitAreaAlone(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowSearch()
	if m.primaryTool() != "find" || m.splitTool() != "run" {
		t.Fatalf("primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
}

func TestUsagesOpenInTheBottomSide(t *testing.T) {
	m := toolModel()
	m.showUsages()
	if !m.usages.visible || m.primaryTool() != "usages" {
		t.Fatalf("usages: visible=%v primary=%q", m.usages.visible, m.primaryTool())
	}
	m.ShowSearch()
	if m.usages.visible || m.primaryTool() != "find" {
		t.Fatalf("find took the primary area: usages=%v primary=%q", m.usages.visible, m.primaryTool())
	}
}

func TestHidingFindKeepsTheOtherArea(t *testing.T) {
	m := toolModel()
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowSearch()
	m.HideActiveToolWindow()
	if m.search.visible || m.splitTool() != "run" {
		t.Fatalf("after hiding find: visible=%v split=%q", m.search.visible, m.splitTool())
	}
}
