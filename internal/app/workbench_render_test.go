package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// workbenchTester renders the Islands workbench offscreen at 1440×900, so the
// shell is checked without opening a window.
func workbenchTester(t *testing.T, m *Model) *ui.Tester {
	t.Helper()
	if m.mgr == nil {
		// NewManager only builds its channels; it does not start a server, so
		// the status bar can read the backend state without a process.
		m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.workbench(c, c.Theme())
	}, 1440, 900)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	if out := os.Getenv("WORKBENCH_SHOT"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	return tt
}

func TestWorkbenchShellRenders(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	tt := workbenchTester(t, m)
	for _, text := range []string{
		filepath.Base(root), "Search Everywhere", "Go to File",
		"Drop files here to open them", "openidea-server offline",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not in the workbench: %q", text, tt.Texts())
		}
	}
	// The Project tool window title sits in the 240px island right of the
	// 34px stripe, never under it, and the island fills the work area from the
	// title bar down rather than floating at its content height.
	if r, ok := tt.Find("Project"); !ok || r.X < 40 || r.X+4 > 46+oiLeftWidth || r.Y > 80 {
		t.Fatalf("Project is not at the top of the left island: %+v, %v", r, ok)
	}
	// The status bar occupies the last 34px of the window.
	if r, ok := tt.Find("openidea-server offline"); !ok || r.Y < 900-oiStatusBarHeight {
		t.Fatalf("the backend state is not in the status bar: %+v, %v", r, ok)
	}
}

func TestWorkbenchStripeTogglesTheIsland(t *testing.T) {
	m := &Model{}
	tt := workbenchTester(t, m)
	// The Project button is active by default, so its press hides the island.
	tt.ClickAt(25, 60)
	tt.Frame()
	if !m.leftHidden {
		t.Fatalf("the Project stripe button did not hide the island")
	}
	if tt.HasText("Project") {
		t.Fatalf("the left island kept its header: %q", tt.Texts())
	}
	tt.ClickAt(25, 60)
	tt.Frame()
	if m.leftHidden {
		t.Fatalf("a second press did not bring the island back")
	}
	if !tt.HasText("Project") {
		t.Fatalf("the left island did not return: %q", tt.Texts())
	}
}

func TestWorkbenchToolWindowTitles(t *testing.T) {
	m := &Model{}
	m.ShowStructure()
	tt := workbenchTester(t, m)
	project, ok := tt.Find("Project")
	if !ok {
		t.Fatalf("the Project island is missing: %q", tt.Texts())
	}
	r, ok := tt.Find("Structure")
	if !ok || r.X < 40 || r.X >= 46+oiLeftWidth {
		t.Fatalf("Structure is not the left island title: %+v, %v", r, ok)
	}
	if r.Y <= project.Y {
		t.Fatalf("Structure is not below the Project island: %+v vs %+v", r, project)
	}
}

// leftGapY is the middle of the gap between the left side's two islands at
// 1440×900, both showing at the default divide.
func leftGapY() float32 {
	side := float32(900 - oiTitleBarHeight - oiStatusBarHeight - 12)
	area := (side - oiIslandGap) / 2
	return float32(oiTitleBarHeight+6) + area + oiIslandGap/2
}

func TestWorkbenchLeftSplitHandleMovesTheLine(t *testing.T) {
	m := &Model{}
	m.ShowStructure()
	tt := workbenchTester(t, m)
	before, ok := tt.Find("Structure")
	if !ok {
		t.Fatalf("the Structure island is missing: %q", tt.Texts())
	}
	y := leftGapY()
	tt.Move(160, y)
	if got := tt.Cursor(); got != ui.CursorResizeRow {
		t.Fatalf("the gap shows cursor %v, want the row resize cursor", got)
	}
	tt.Press(160, y)
	tt.Move(160, y+80)
	tt.Release(160, y+80)
	tt.Frame()
	if m.leftShare() <= 0.5 {
		t.Fatalf("the drag down did not give the primary island more room: %v", m.leftShare())
	}
	after, ok := tt.Find("Structure")
	if !ok || after.Y <= before.Y+40 {
		t.Fatalf("the split island did not move down with the line: %+v -> %+v", before, after)
	}
}

// leftGapX is the middle of the gap on the left side's inner edge at 1440×900,
// the side at its default width.
func leftGapX() float32 {
	return 6 + oiStripeWidth + oiLeftWidth + oiIslandGap/2
}

func TestWorkbenchLeftWidthHandleResizesTheSide(t *testing.T) {
	m := &Model{}
	tt := workbenchTester(t, m)
	before, ok := tt.Find("Project")
	if !ok {
		t.Fatalf("the Project island is missing: %q", tt.Texts())
	}
	x := leftGapX()
	tt.Move(x, 300)
	if got := tt.Cursor(); got != ui.CursorResizeColumn {
		t.Fatalf("the gap shows cursor %v, want the column resize cursor", got)
	}
	tt.Press(x, 300)
	tt.Move(x+60, 300)
	tt.Release(x+60, 300)
	tt.Frame()
	if got := m.leftWidth; got != 300 {
		t.Fatalf("the drag set the width to %v, want 300", got)
	}
	if got := m.leftSideWidth(); got != 300 {
		t.Fatalf("the side draws at %v, want 300", got)
	}
	after, ok := tt.Find("Project")
	if !ok || after.X != before.X {
		t.Fatalf("the island's title moved with the resize: %+v -> %+v", before, after)
	}
}

func TestWorkbenchLeftWidthDragPastTheMinimumClosesTheSide(t *testing.T) {
	m := &Model{leftSplit: "structure"}
	tt := workbenchTester(t, m)
	x := leftGapX()
	// 180 DIPs off the default 240 leaves 60, under half the smallest
	// width, so the release closes the side.
	tt.Press(x, 300)
	tt.Move(x-180, 300)
	tt.Release(x-180, 300)
	tt.Frame()
	if m.leftShown() || m.leftSplit != "" {
		t.Fatalf("the side stayed: hidden=%v split=%q", m.leftHidden, m.leftSplit)
	}
	if tt.HasText("Project") || tt.HasText("Structure") {
		t.Fatalf("the side kept its islands: %q", tt.Texts())
	}
	if got := m.leftWidth; got != 0 {
		t.Fatalf("a closed side kept a width: %v", got)
	}
}

// bottomGapY is the middle of the gap above the bottom area at 1440×900, the
// area at its default height.
func bottomGapY() float32 {
	return 900 - oiStatusBarHeight - float32(bottomDefaultHeight) - oiIslandGap/2
}

func TestWorkbenchBottomGapResizesTheArea(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	tt := workbenchTester(t, m)
	y := bottomGapY()
	tt.Move(700, y)
	if got := tt.Cursor(); got != ui.CursorResizeRow {
		t.Fatalf("the gap shows cursor %v, want the row resize cursor", got)
	}
	tt.Press(700, y)
	tt.Move(700, y-100)
	tt.Release(700, y-100)
	tt.Frame()
	if got := m.bottomHeight; got != bottomDefaultHeight+100 {
		t.Fatalf("the drag set the height to %d, want %d", got, bottomDefaultHeight+100)
	}
	if got := m.bottomDrawHeight(); got != bottomDefaultHeight+100 {
		t.Fatalf("the area draws at %d, want %d", got, bottomDefaultHeight+100)
	}
}

func TestWorkbenchBottomGapKeepsTheMinimumHeight(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	tt := workbenchTester(t, m)
	y := bottomGapY()
	// 230 down leaves 50, under the smallest height but above half of it, so
	// the release keeps the smallest height instead of closing the windows.
	tt.Press(700, y)
	tt.Move(700, y+230)
	tt.Release(700, y+230)
	tt.Frame()
	if got := m.bottomHeight; got != bottomDragMin {
		t.Fatalf("the height = %d, want the smallest %d", got, bottomDragMin)
	}
	if m.visibleTool() == "" {
		t.Fatal("the drag closed the windows without going past half the minimum")
	}
}

func TestWorkbenchBottomGapDragPastTheMinimumClosesTheWindows(t *testing.T) {
	m := toolModel()
	m.ShowProblems()
	tt := workbenchTester(t, m)
	y := bottomGapY()
	// 250 down leaves 30, under half the smallest height, so the release
	// closes every bottom window.
	tt.Press(700, y)
	tt.Move(700, y+250)
	tt.Release(700, y+250)
	tt.Frame()
	if m.visibleTool() != "" || m.problems.visible {
		t.Fatalf("the windows stayed: %q", m.visibleTool())
	}
	if m.bottomSize() != bottomDefaultHeight {
		t.Fatalf("a closed area kept a height: %d", m.bottomSize())
	}
}

func TestWorkbenchHierarchyRightWindowRenders(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	m.hierarchy = hierarchyPane{
		mode: "call", direction: "incoming",
		rootName: "main", rootPath: "/p/App.java", rootLine: 4, rootCol: 20,
		children: map[string][]hierarchyNode{
			hierarchyRoot: {{id: hierarchyRoot + "/a", parent: hierarchyRoot,
				item: hierarchyItem{Name: "callerA", Path: "/p/App.java"}}},
		},
		expanded: map[string]bool{}, loading: map[string]bool{}, failed: map[string]string{},
	}
	m.rightTool = "hierarchy"
	tt := workbenchTester(t, m)
	for _, text := range []string{"Hierarchy", "Callers", "Callees", "callerA"} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not in the right window: %q", text, tt.Texts())
		}
	}
	// The window sits at the right edge, beside the editor and its stripe.
	if r, ok := tt.Find("Hierarchy"); !ok || r.X < 1000 {
		t.Fatalf("Hierarchy is not on the right: %+v, %v", r, ok)
	}
}
