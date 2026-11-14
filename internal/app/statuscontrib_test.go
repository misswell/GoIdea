package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func statusIDs(items []*StatusItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

func noopStatusRender(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {}

func TestStatusRegistryRejectsBadContributions(t *testing.T) {
	registerStatusItem(nil)
	registerStatusItem(&StatusItem{ID: ""})
	if unregisterStatusItem("") {
		t.Fatal("an empty id was removed")
	}
	registerStatusItem(&StatusItem{ID: "test.status.nilrender", Side: "left"})
	if unregisterStatusItem("test.status.nilrender") {
		t.Fatal("an item without render was registered")
	}
}

func TestStatusRegistryOrderSideAndVisibility(t *testing.T) {
	registerStatusItem(&StatusItem{ID: "test.status.b", Side: "left", Order: 5, Render: noopStatusRender})
	registerStatusItem(&StatusItem{ID: "test.status.a", Side: "left", Order: 5, Render: noopStatusRender})
	registerStatusItem(&StatusItem{ID: "test.status.c", Side: "left", Order: 15, Render: noopStatusRender})
	defer unregisterStatusItem("test.status.b")
	defer unregisterStatusItem("test.status.a")
	defer unregisterStatusItem("test.status.c")

	// A duplicate id must not replace the original contribution.
	registerStatusItem(&StatusItem{ID: "test.status.b", Side: "left", Order: 99, Render: noopStatusRender})
	for _, it := range statusItems.items {
		if it.ID == "test.status.b" && it.Order != 5 {
			t.Fatalf("a duplicate id replaced the original: order %d", it.Order)
		}
	}

	m := &Model{}
	ids := statusIDs(m.statusSide("left"))
	if got := indexOf(ids, "test.status.b"); got != 0 {
		t.Fatalf("order 5 ties keep registration order: %v", ids)
	}
	if indexOf(ids, "test.status.a") != 1 {
		t.Fatalf("order 5 ties keep registration order: %v", ids)
	}
	if !(indexOf(ids, "test.status.a") < indexOf(ids, "status.file") &&
		indexOf(ids, "status.file") < indexOf(ids, "test.status.c")) {
		t.Fatalf("orders 5, 10 and 15 are out of order: %v", ids)
	}
	if indexOf(ids, "status.run") != -1 {
		t.Fatalf("a hidden run item drew: %v", ids)
	}
	if indexOf(ids, "status.problems") != -1 {
		t.Fatalf("a right item drew on the left: %v", ids)
	}

	m.run.visible = true
	ids = statusIDs(m.statusSide("left"))
	if indexOf(ids, "status.run") < indexOf(ids, "test.status.c") {
		t.Fatalf("the run item did not appear in order: %v", ids)
	}

	right := statusIDs(m.statusSide("right"))
	if indexOf(right, "status.problems") != 0 {
		t.Fatalf("right items are not in order: %v", right)
	}
	if indexOf(right, "test.status.c") != -1 {
		t.Fatalf("a left item drew on the right: %v", right)
	}
}

func TestStatusRegistryUnregister(t *testing.T) {
	registerStatusItem(&StatusItem{ID: "test.status.gone", Side: "right", Order: 1, Render: noopStatusRender})
	m := &Model{}
	if indexOf(statusIDs(m.statusSide("right")), "test.status.gone") == -1 {
		t.Fatal("the item was not registered")
	}
	if !unregisterStatusItem("test.status.gone") {
		t.Fatal("unregister reported nothing removed")
	}
	if indexOf(statusIDs(m.statusSide("right")), "test.status.gone") != -1 {
		t.Fatal("the item survived unregister")
	}
	if unregisterStatusItem("test.status.gone") {
		t.Fatal("a second unregister reported a removal")
	}
}

func TestStatusSideIsStableAcrossCalls(t *testing.T) {
	first := statusIDs((&Model{}).statusSide("right"))
	second := statusIDs((&Model{}).statusSide("right"))
	if len(first) != len(second) {
		t.Fatalf("the side changed between calls: %v %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("the side is unstable: %v %v", first, second)
		}
	}
}

func TestStatusBarShowsBranchAndBuild(t *testing.T) {
	m := &Model{}
	if indexOf(statusIDs(m.statusSide("right")), "status.vcs") != -1 {
		t.Fatal("a repository-less project shows a branch")
	}
	if indexOf(statusIDs(m.statusSide("left")), "status.build") != -1 {
		t.Fatal("an idle project shows a build")
	}

	head := "main"
	m.branch.head = &head
	m.build.busy = true
	if indexOf(statusIDs(m.statusSide("right")), "status.vcs") == -1 {
		t.Fatal("the branch item is hidden while a branch is checked out")
	}
	if indexOf(statusIDs(m.statusSide("left")), "status.build") == -1 {
		t.Fatal("the build item is hidden while a build runs")
	}

	// Without the title bar widgets the only "main" left is the status bar.
	m.prefs.MainToolbar = &toolbarLayout{Hidden: []string{
		toolbarProject, toolbarVCS, toolbarRun, toolbarSearch, toolbarTerminal}}
	tt := workbenchTester(t, m)
	if r, ok := tt.Find("main"); !ok || r.Y < 900-oiStatusBarHeight {
		t.Fatalf("the branch is not in the status bar: %+v, %v", r, ok)
	}
	if r, ok := tt.Find("Building…"); !ok || r.Y < 900-oiStatusBarHeight {
		t.Fatalf("the build state is not in the status bar: %+v, %v", r, ok)
	}
	m.prefs.MainToolbar = nil
}
