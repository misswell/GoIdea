package app

import "testing"

func TestEveryToolWindowIsRegistered(t *testing.T) {
	want := []string{
		"run", "debug", "build", "term", "log", "problems", "todo", "find",
		"usages", "services", "structure", "bookmarks", "notifications", "hierarchy",
	}
	for _, id := range want {
		spec := dockSpec(id)
		if spec == nil {
			t.Fatalf("%s is not registered", id)
		}
		if spec.title == "" || spec.anchor < dockLeft || spec.anchor >= dockSlotCount {
			t.Fatalf("%s has no title or anchor", id)
		}
	}
	if isToolWindow("run") == false {
		t.Fatalf("the legacy list lost run")
	}
}

func TestOneWindowPerBottomSlotAcrossAllKinds(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	m.MoveActiveToolWindowToSplit() // problems takes the split
	m.ShowRun()
	if m.primaryTool() != "run" || m.splitTool() != "problems" {
		t.Fatalf("primary=%q split=%q", m.primaryTool(), m.splitTool())
	}
	m.ShowBuild()
	if !m.build.visible || m.run.visible {
		t.Fatalf("build did not replace run: build=%v run=%v", m.build.visible, m.run.visible)
	}
	if !m.problems.visible {
		t.Fatalf("replacing the primary window closed the split one")
	}
	m.ShowSearch()
	if !m.search.visible || m.build.visible {
		t.Fatalf("find did not replace build")
	}
	m.showUsages()
	if !m.usages.visible || m.search.visible {
		t.Fatalf("usages did not replace find")
	}
	m.DockHide("problems")
	m.DockHide("usages")
}

func TestLeftSplitWindowsShareOneSlot(t *testing.T) {
	m := toolModel()
	m.ShowStructure()
	if m.leftSplit != "structure" || !m.structure.visible {
		t.Fatalf("structure did not open: %q", m.leftSplit)
	}
	m.ShowBookmarksWindow()
	if m.leftSplit != "bookmarks" || m.structure.visible {
		t.Fatalf("bookmarks did not replace structure: split=%q structure=%v", m.leftSplit, m.structure.visible)
	}
	m.ToggleBookmarksWindow()
	if m.leftSplit != "" || m.dockOccupant(dockLeftSplit) != "" {
		t.Fatalf("bookmarks did not close: split=%q slot=%q", m.leftSplit, m.dockOccupant(dockLeftSplit))
	}
}
