package app

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func setTabPath(t *Tab, path string) {
	t.doc = docbridge.Virtual(path, "")
	t.state = &editor.State{Buffer: t.doc.Buffer()}
}

type windowActionHost struct {
	fakeWorkspaceHost
	group  []WorkspaceWindow
	keep   string
	close  []string
	order  []string
	focus  string
	detach bool
}

func (h *windowActionHost) WorkspaceWindows(*Model) []WorkspaceWindow { return h.group }
func (h *windowActionHost) CloseWorkspaceWindows(_ *Model, keep string, ids []string) {
	h.keep = keep
	h.close = ids
}
func (h *windowActionHost) ReorderWorkspaceWindows(_ *Model, ids []string) bool {
	h.order = ids
	return true
}
func (h *windowActionHost) FocusWorkspaceWindow(id string) bool { h.focus = id; return true }
func (h *windowActionHost) DetachWorkspaceWindow(*Model) bool {
	h.detach = true
	return true
}

func TestWindowCompatibilityActionsTargetProjectsAndPreserveEditorTabs(t *testing.T) {
	file := &Tab{}
	setTabPath(file, "/p/a.go")
	host := &windowActionHost{group: []WorkspaceWindow{{ID: "a"}, {ID: "b", Selected: true}, {ID: "c"}, {ID: "d"}}}
	m := &Model{tabs: []*Tab{file}, workspaceHost: host}
	for _, tc := range []struct {
		id   string
		arg  any
		keep string
		want []string
	}{
		{"window.closeOtherTabs", "c", "c", []string{"a", "b", "d"}},
		{"window.closeTabsToLeft", nil, "b", []string{"a"}},
		{"window.closeTabsToRight", "a", "a", []string{"b", "c", "d"}},
		{"window.closeTab", "d", "", []string{"d"}},
		{"window.closeTab", "/p/a.go", "", nil},
	} {
		host.keep, host.close = "", nil
		if !m.RunActionWithArg(tc.id, tc.arg) || host.keep != tc.keep || !slices.Equal(host.close, tc.want) || len(m.tabs) != 1 || m.tab() != file {
			t.Fatalf("%s(%v) kept %q and closed %v, editor tabs %v", tc.id, tc.arg, host.keep, host.close, m.tabs)
		}
	}
	if got := projectWindowTabOrder(host.group, "b", 3); !slices.Equal(got, []string{"a", "c", "d", "b"}) {
		t.Fatalf("forward drag: %v", got)
	}
	if got := projectWindowTabOrder(host.group, "d", 1); !slices.Equal(got, []string{"a", "d", "b", "c"}) {
		t.Fatalf("backward drag: %v", got)
	}
	host.focus = ""
	if !m.RunAction("window.nextTab") || host.focus != "c" {
		t.Fatalf("next project tab focused %q, want c", host.focus)
	}
	host.focus = ""
	if !m.RunAction("window.previousTab") || host.focus != "a" {
		t.Fatalf("previous project tab focused %q, want a", host.focus)
	}
	host.focus = ""
	if !m.RunActionWithArg("window.selectTab", "c") || host.focus != "c" {
		t.Fatalf("select project tab focused %q, want c", host.focus)
	}
	if !m.RunAction("window.detachTab") || !host.detach {
		t.Fatal("detach project tab did not reach the native workspace host")
	}
	host.group = nil
	for _, id := range []string{"window.closeTab", "window.closeOtherTabs", "window.selectTab", "window.reorderTabs", "window.nextTab", "window.previousTab"} {
		if m.ActionEnabled(id) {
			t.Fatalf("standalone editor enabled %s", id)
		}
	}
}

func TestLightEditOpenProjectTargetsTheActiveFilesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "src", "Standalone.java")
	tab := &Tab{}
	setTabPath(tab, path)
	m := &Model{tabs: []*Tab{tab}, active: 0}
	if !m.ActionEnabled("lightEdit.openProject") {
		t.Fatal("LightEdit Open Project is disabled for a standalone file")
	}
	if got, want := filepath.Dir(tab.doc.Path()), filepath.Dir(path); got != want {
		t.Fatalf("LightEdit project target = %q, want %q", got, want)
	}

	m.root = path
	if m.ActionEnabled("lightEdit.openProject") {
		t.Fatal("LightEdit Open Project stayed enabled inside a project")
	}
}

func TestProjectWindowTabStripTargetsBackgroundProject(t *testing.T) {
	h := &windowActionHost{group: []WorkspaceWindow{{ID: "one", Title: "First", Selected: true}, {ID: "two", Title: "Second"}}}
	m := &Model{workspaceHost: h}
	tester := ui.NewTester(func(c *ui.Context) { m.workspaceWindowTabs(c, c.Theme()) }, 800, 100)
	tester.Frame()
	if err := tester.RightClick("Second"); err != nil {
		t.Fatal(err)
	}
	assertMenuLabels(t, tester.Menu(), "Close Tab", "Close Other Tabs", "Close Tabs to the Left", "Close Tabs to the Right", "Move Tab to New Window")
	if err := tester.ChooseMenuItem("Close Other Tabs"); err != nil {
		t.Fatal(err)
	}
	if h.keep != "two" || !slices.Equal(h.close, []string{"one"}) {
		t.Fatalf("background menu kept %q, closed %v", h.keep, h.close)
	}
	first, ok := tester.Find("First")
	if !ok {
		t.Fatal("missing First project tab")
	}
	second, ok := tester.Find("Second")
	if !ok {
		t.Fatal("missing Second project tab")
	}
	x, y := first.X+5, first.Y+first.H/2
	to := second.X + 5
	tester.Press(x, y)
	for i := 1; i <= 4; i++ {
		tester.Move(x+(to-x)*float32(i)/4, y)
	}
	tester.Release(to, y)
	if !slices.Equal(h.order, []string{"two", "one"}) || h.focus != "" {
		t.Fatalf("drag reordered %v and focused %q", h.order, h.focus)
	}

	if err := tester.Click("Close Second"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.close, []string{"two"}) {
		t.Fatalf("close button targeted %v", h.close)
	}
}
