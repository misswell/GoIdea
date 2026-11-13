package app

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// layoutModel is a model with the bottom windows a layout needs, its session
// file in a temporary folder and the folder itself as the project.
func layoutModel(t *testing.T) (*Model, string) {
	t.Helper()
	root := t.TempDir()
	m := toolModel()
	m.cfg = Config{SessionPath: filepath.Join(t.TempDir(), "session.json")}
	m.layouts = map[string]Layout{}
	m.setRoot(root)
	// A session write runs in the background, so the temporary folder must
	// not be removed under it; holding the write lock proves it finished.
	t.Cleanup(func() {
		for i := 0; i < 2000; i++ {
			m.sessionMu.Lock()
			done := m.sessionWrote >= m.sessionSeq
			m.sessionMu.Unlock()
			if done {
				return
			}
			time.Sleep(time.Millisecond)
		}
	})
	return m, root
}

func TestCurrentLayoutKeepsTheWorkbenchArrangement(t *testing.T) {
	m, _ := layoutModel(t)
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	m.bottomHeight = bottomDefaultHeight + bottomStep
	m.sideMode = "vcs"

	l := normalizeLayout(m.currentLayout())
	if l.Side != "vcs" {
		t.Fatalf("side = %q, want vcs", l.Side)
	}
	if len(l.Bottom) != 2 || l.Bottom[0] != "run" || l.Bottom[1] != "problems" {
		t.Fatalf("bottom = %v, want [run problems]", l.Bottom)
	}
	if !l.Split["run"] || len(l.Split) != 1 {
		t.Fatalf("split = %v, want only run", l.Split)
	}
	if l.Tool != "problems" {
		t.Fatalf("tool = %q, want problems", l.Tool)
	}
	if l.BottomHeight != bottomDefaultHeight+bottomStep {
		t.Fatalf("bottom height = %d", l.BottomHeight)
	}
}

func TestLayoutComesBackWithTheProject(t *testing.T) {
	m, root := layoutModel(t)
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	m.bottomHeight = bottomDefaultHeight + bottomStep
	m.sideMode = "vcs"
	m.leftHidden = false
	m.saveSessionNow()

	// A later start reads the session and restores the arrangement once the
	// project is open, as OpenIDEA loads a layout per project root.
	s := LoadSession(m.cfg.SessionPath)
	restored := toolModel()
	restored.layouts = s.Layouts
	restored.setRoot(root)
	restored.applyLayout(root)

	if restored.sideMode != "vcs" {
		t.Fatalf("side = %q, want vcs", restored.sideMode)
	}
	if restored.bottomHeight != bottomDefaultHeight+bottomStep {
		t.Fatalf("bottom height = %d", restored.bottomHeight)
	}
	if restored.splitTool() != "run" || restored.primaryTool() != "problems" {
		t.Fatalf("areas: split=%q primary=%q", restored.splitTool(), restored.primaryTool())
	}
	if restored.tool != "problems" {
		t.Fatalf("tool = %q, want problems", restored.tool)
	}
}

func TestClosingTheLastWindowIsPartOfTheLayout(t *testing.T) {
	m, root := layoutModel(t)
	m.ShowRun()
	m.HideActiveToolWindow()
	m.saveSessionNow()

	s := LoadSession(m.cfg.SessionPath)
	restored := toolModel()
	restored.layouts = s.Layouts
	restored.setRoot(root)
	restored.applyLayout(root)
	if restored.visibleTool() != "" {
		t.Fatalf("a window came back that was closed: %q", restored.visibleTool())
	}
}

func TestStoredLayoutDropsWhatItCannotMean(t *testing.T) {
	l := normalizeLayout(Layout{
		Side:         "nope",
		LeftTool:     "nope",
		LeftHidden:   true,
		Bottom:       []string{"run", "run", "nope", "problems"},
		Tool:         "nope",
		Split:        map[string]bool{"run": true, "usages": false, "nope": true},
		BottomHeight: 5,
	})
	if l.Side != "" {
		t.Fatalf("side = %q, want the default", l.Side)
	}
	if l.LeftTool != "" {
		t.Fatalf("left tool = %q, want none", l.LeftTool)
	}
	if len(l.Bottom) != 2 || l.Bottom[0] != "run" || l.Bottom[1] != "problems" {
		t.Fatalf("bottom = %v", l.Bottom)
	}
	if l.Tool != "" {
		t.Fatalf("tool = %q, want none", l.Tool)
	}
	if len(l.Split) != 1 || !l.Split["run"] {
		t.Fatalf("split = %v", l.Split)
	}
	if l.BottomHeight != 0 {
		t.Fatalf("bottom height = %d, want the default", l.BottomHeight)
	}
	if !l.LeftHidden {
		t.Fatal("a closed left area is a layout a project can keep")
	}
}

func TestStoredLeftToolsMigrateToTheSplitArea(t *testing.T) {
	for _, side := range []string{"structure", "bookmarks"} {
		l := normalizeLayout(Layout{Side: side})
		if l.Side != "" || l.LeftTool != side {
			t.Fatalf("side %q became side=%q tool=%q", side, l.Side, l.LeftTool)
		}
	}
}

func TestLeftSplitShowsOneWindowAtATime(t *testing.T) {
	m, _ := layoutModel(t)
	m.ShowStructure()
	if m.leftSplit != "structure" || !m.structure.visible || m.leftHidden {
		t.Fatalf("structure did not take the split: split=%q visible=%v hidden=%v",
			m.leftSplit, m.structure.visible, m.leftHidden)
	}
	m.ShowBookmarksWindow()
	if m.leftSplit != "bookmarks" || m.structure.visible {
		t.Fatalf("bookmarks did not take the split: split=%q structure=%v",
			m.leftSplit, m.structure.visible)
	}
	m.ToggleBookmarksWindow()
	if m.leftSplit != "" {
		t.Fatalf("bookmarks did not close: split=%q", m.leftSplit)
	}
}

func TestLayoutsAreBoundedAndKeepTheOpenProject(t *testing.T) {
	m, root := layoutModel(t)
	for i := 0; i < maxLayouts+10; i++ {
		m.layouts[filepath.Join("/projects", fmt.Sprintf("p%d", i))] = Layout{Side: "vcs"}
	}
	m.layouts[layoutKey(root)] = Layout{Side: "bookmarks"}
	m.saveSessionNow()

	s := LoadSession(m.cfg.SessionPath)
	if len(s.Layouts) > maxLayouts {
		t.Fatalf("%d layouts were stored, want at most %d", len(s.Layouts), maxLayouts)
	}
	if _, ok := s.Layouts[layoutKey(root)]; !ok {
		t.Fatal("the open project's layout was pruned")
	}
}

func TestRestoreDefaultLayoutMergesTheSplitArea(t *testing.T) {
	m, _ := layoutModel(t)
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	m.ResetToolWindowLayout()

	if len(m.splitNames) != 0 {
		t.Fatalf("split = %v after the reset", m.splitNames)
	}
	if m.run.visible {
		t.Fatal("a window in the split area stayed open")
	}
	if !m.problems.visible || m.tool != "problems" {
		t.Fatalf("the primary window changed: visible=%v tool=%q", m.problems.visible, m.tool)
	}
}

func TestAnOlderSessionWriteNeverReplacesANewerOne(t *testing.T) {
	m, root := layoutModel(t)
	newer := Session{Root: root}
	older := Session{Root: "/somewhere/else"}
	m.writeSession(2, m.cfg.SessionPath, newer)
	m.writeSession(1, m.cfg.SessionPath, older)

	s := LoadSession(m.cfg.SessionPath)
	if s.Root != root {
		t.Fatalf("root = %q, want the newer snapshot %q", s.Root, root)
	}
}
