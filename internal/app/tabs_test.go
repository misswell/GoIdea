package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func strip(tabs ...*Tab) []*Tab { return tabs }

// wantOrder reports whether got holds exactly want, in order.
func wantOrder(t *testing.T, got []*Tab, want ...*Tab) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d tabs, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tab %d is not the wanted one", i)
		}
	}
}

func TestPinningJoinsThePinnedGroupAndUnpinningLeavesIt(t *testing.T) {
	a, b, c := &Tab{}, &Tab{}, &Tab{}
	tabs := strip(a, b, c)

	tabs, at := placeTab(tabs, b, true)
	if at != 0 {
		t.Fatalf("pin b at %d, want 0", at)
	}
	wantOrder(t, tabs, b, a, c)

	tabs, at = placeTab(tabs, c, true)
	if at != 1 {
		t.Fatalf("pin c at %d, want 1", at)
	}
	wantOrder(t, tabs, b, c, a)

	// Unpinning puts the tab right after the pinned group, as OpenIDEA does.
	tabs, at = placeTab(tabs, b, false)
	if at != 1 {
		t.Fatalf("unpin b at %d, want 1", at)
	}
	wantOrder(t, tabs, c, b, a)
	if !c.pinned || b.pinned || a.pinned {
		t.Fatal("pinned flags do not follow the toggle")
	}
}

func TestPinActiveTabKeepsTheTabSelected(t *testing.T) {
	a, b, c := &Tab{}, &Tab{}, &Tab{}
	m := toolModel()
	m.tabs = strip(a, b, c)
	m.active = 1

	m.PinActiveTab()
	if m.active != 0 || !b.pinned {
		t.Fatalf("pin: active %d pinned %v", m.active, b.pinned)
	}
	wantOrder(t, m.tabs, b, a, c)

	m.PinActiveTab()
	if m.active != 0 || b.pinned {
		t.Fatalf("unpin: active %d pinned %v", m.active, b.pinned)
	}
	wantOrder(t, m.tabs, b, a, c)
}

func TestCloseOtherTabsLeavesPinnedOnes(t *testing.T) {
	a, b, c, d := &Tab{pinned: true}, &Tab{}, &Tab{}, &Tab{}
	wantOrder(t, closeOthersTargets(strip(a, b, c, d), c), b, d)
}

func TestCloseTabsToTheSidesSkipPinnedOnes(t *testing.T) {
	a, b, c, d := &Tab{}, &Tab{pinned: true}, &Tab{}, &Tab{}
	wantOrder(t, closeSideTargets(strip(a, b, c, d), c, -1), a)
	wantOrder(t, closeSideTargets(strip(a, b, c, d), c, 1), d)
}

func TestCloseAllProtectsPinnedOnlyWhenKept(t *testing.T) {
	a, b, c := &Tab{pinned: true}, &Tab{}, &Tab{}
	wantOrder(t, closeAllTargets(strip(a, b, c), false), a, b, c)
	wantOrder(t, closeAllTargets(strip(a, b, c), true), b, c)
}

func TestCloseUnmodifiedKeepsModifiedAndPinnedTabs(t *testing.T) {
	a, b, c, d := &Tab{pinned: true}, &Tab{}, &Tab{}, &Tab{}
	modified := func(t *Tab) bool { return t == c }
	wantOrder(t, closeUnmodifiedTargets(strip(a, b, c, d), modified), b, d)
}

func TestTabLabelFollowsTheStripSettings(t *testing.T) {
	main := "/p/src/Main.java"
	twin := "/p/test/Main.java"
	single := []string{main}

	if name, dir := tabLabelFor(main, TabPrefs{}, false, single); name != "Main.java" || dir != "" {
		t.Fatalf("default label = %q %q", name, dir)
	}
	if name, _ := tabLabelFor(main, TabPrefs{HideExtension: true}, false, single); name != "Main" {
		t.Fatalf("label without the extension = %q", name)
	}
	if name, dir := tabLabelFor(main, TabPrefs{}, false, []string{twin, main}); name != "Main.java" || dir != "src" {
		t.Fatalf("ambiguous label = %q %q", name, dir)
	}
	if name, dir := tabLabelFor(main, TabPrefs{NoDirectoryForNonUnique: true}, false,
		[]string{twin, main}); name != "Main.java" || dir != "" {
		t.Fatalf("label with the folder suffix off = %q %q", name, dir)
	}
	if name, _ := tabLabelFor(main, TabPrefs{MarkModified: true}, true, single); name != "*Main.java" {
		t.Fatalf("modified label = %q", name)
	}
	if name, _ := tabLabelFor(main, TabPrefs{MarkModified: true}, false, single); name != "Main.java" {
		t.Fatalf("saved label = %q", name)
	}
}

func TestInsertTabAfterTheActiveOneOrAtTheEnd(t *testing.T) {
	a, b := &Tab{}, &Tab{}
	m := &Model{tabs: strip(a, b), active: 0}
	fresh := &Tab{}
	m.insertTab(fresh)
	wantOrder(t, m.tabs, a, fresh, b)

	pinned := &Tab{pinned: true}
	m = &Model{tabs: strip(pinned, a), active: 0, tabPrefs: TabPrefs{OpenAtEnd: true}}
	fresh = &Tab{}
	m.insertTab(fresh)
	wantOrder(t, m.tabs, pinned, a, fresh)

	m = &Model{tabs: strip(pinned, a), active: 0}
	fresh = &Tab{}
	m.insertTab(fresh)
	wantOrder(t, m.tabs, pinned, fresh, a)
}

func TestSortTabsByTitlePinsFirstAndKeepsTheActiveTab(t *testing.T) {
	pinned, a, b, c := &Tab{pinned: true}, &Tab{}, &Tab{}, &Tab{}
	key := func(t *Tab) string {
		return map[*Tab]string{pinned: "b", a: "c", b: "a", c: "d"}[t]
	}
	tabs, active := sortTabsByTitle(strip(a, b, pinned, c), 0, key)
	wantOrder(t, tabs, pinned, b, a, c)
	if active != 2 {
		t.Fatalf("active = %d, want 2", active)
	}
	if tabs, active := sortTabsByTitle(strip(a, b), -1, key); active != -1 || len(tabs) != 2 {
		t.Fatalf("invalid active = %d on %d tabs", active, len(tabs))
	}
}

func TestNextActiveAfterCloseFollowsThePolicy(t *testing.T) {
	a, b, c := &Tab{used: 1}, &Tab{used: 3}, &Tab{used: 2}
	// The strip left after closing b.
	if got := nextActiveAfterClose(strip(a, c), 1, "left"); got != 0 {
		t.Fatalf("left of the closed tab = %d", got)
	}
	if got := nextActiveAfterClose(strip(a, c), 1, "right"); got != 1 {
		t.Fatalf("right of the closed tab = %d", got)
	}
	if got := nextActiveAfterClose(strip(a, c), 0, "mru"); got != 1 {
		t.Fatalf("most recently used = %d", got)
	}
	if got := nextActiveAfterClose(strip(&Tab{}, &Tab{}), 0, "mru"); got != 0 {
		t.Fatalf("nothing used yet = %d", got)
	}
	// A closed first tab has nothing to its left, so the new first opens.
	if got := nextActiveAfterClose(strip(b, c), 0, "left"); got != 0 {
		t.Fatalf("closed first tab = %d", got)
	}
	// A closed last tab has nothing to its right, so the last one opens.
	if got := nextActiveAfterClose(strip(a, b), 2, "right"); got != 1 {
		t.Fatalf("closed last tab = %d", got)
	}
	if got := nextActiveAfterClose(nil, 0, "right"); got != 0 {
		t.Fatalf("empty strip = %d", got)
	}
}

func TestTabActivationDefaultsToTheLeft(t *testing.T) {
	if got := (TabPrefs{}).activation(); got != "left" {
		t.Fatalf("default activation = %q", got)
	}
	if got := (TabPrefs{ActivateOnClose: "mru"}).activation(); got != "mru" {
		t.Fatalf("stored activation = %q", got)
	}
	if got := (TabPrefs{ActivateOnClose: "bogus"}).activation(); got != "left" {
		t.Fatalf("unknown activation = %q", got)
	}
}

func TestPreviewTabPromotesAndIsNotPinned(t *testing.T) {
	if !(TabPrefs{}).previews() || (TabPrefs{NoPreviewTab: true}).previews() {
		t.Fatal("previews does not follow the switch")
	}

	a, b := &Tab{}, &Tab{preview: true}
	m := &Model{tabs: strip(a, b), active: 1}
	if at := m.previewIndex(); at != 1 {
		t.Fatalf("preview at %d, want 1", at)
	}
	m.promoteTab(b)
	if b.preview || m.previewIndex() != -1 {
		t.Fatal("promote left the preview flag on")
	}

	// Pinning the preview tab promotes it, as togglePin clears the flag.
	c := &Tab{preview: true}
	m = &Model{tabs: strip(a, c), active: 1}
	m.setTabPinned(c, true)
	if c.preview || !c.pinned {
		t.Fatalf("pin: preview %v pinned %v", c.preview, c.pinned)
	}
}

func TestTabLimitCandidatesKeepPinnedActiveAndUnsaved(t *testing.T) {
	pinned := &Tab{pinned: true, used: 1}
	saved := &Tab{used: 3}
	active := &Tab{used: 2}
	dirty := &Tab{used: 4}
	older := &Tab{used: 5}
	tabs := strip(pinned, saved, active, dirty, older)
	unsaved := func(t *Tab) bool { return t == dirty }
	wantOrder(t, tabLimitCandidates(tabs, 2, unsaved), saved, older)
	if got := tabLimitCandidates(strip(pinned, active), 1, func(*Tab) bool { return false }); len(got) != 0 {
		t.Fatalf("nothing closeable = %d tabs", len(got))
	}
}

func TestClosedTabsKeepTheNewestAndReopenNewestFirst(t *testing.T) {
	m := &Model{}
	for i := 0; i < maxClosedTabs+2; i++ {
		m.rememberClosed(closedTab{path: fmt.Sprintf("/p/f%d.java", i), index: i})
	}
	if len(m.closedTabs) != maxClosedTabs {
		t.Fatalf("history holds %d, want %d", len(m.closedTabs), maxClosedTabs)
	}
	if m.closedTabs[0].path != "/p/f2.java" {
		t.Fatalf("oldest kept = %q", m.closedTabs[0].path)
	}
	e, ok := m.nextClosed()
	if !ok || e.path != "/p/f51.java" || e.index != maxClosedTabs+1 {
		t.Fatalf("newest = %+v %v", e, ok)
	}
	for len(m.closedTabs) > 0 {
		m.nextClosed()
	}
	if _, ok := m.nextClosed(); ok {
		t.Fatal("an empty history still had an entry")
	}
	// Nothing to undo reaches no window and opens nothing.
	m.ReopenClosedTab()
}

func TestInsertTabAtFollowsThePinnedGroup(t *testing.T) {
	pinned, a, b := &Tab{pinned: true}, &Tab{}, &Tab{}
	m := &Model{tabs: strip(pinned, a, b)}

	last := &Tab{}
	m.insertTabAt(last, 99)
	wantOrder(t, m.tabs, pinned, a, b, last)

	fresh := &Tab{}
	m.insertTabAt(fresh, 1)
	wantOrder(t, m.tabs, pinned, fresh, a, b, last)

	newPin := &Tab{pinned: true}
	m.insertTabAt(newPin, 4)
	wantOrder(t, m.tabs, pinned, newPin, fresh, a, b, last)
}

func TestTabForPathIgnoresTabsWithoutADocument(t *testing.T) {
	m := &Model{tabs: strip(&Tab{})}
	if at := m.tabForPath("/p/Main.java"); at != -1 {
		t.Fatalf("tabForPath = %d", at)
	}
}

func TestTabMenuRowsNumberTheStripAndMarkTheActiveTab(t *testing.T) {
	a, b, c := &Tab{}, &Tab{}, &Tab{}
	// The second row of a pinned strip: its tabs keep their workbench
	// index while the menu numbers them from the row's start.
	rows := tabMenuRows(strip(a, b, c), 2, 3)
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	for j, row := range rows {
		if row.Index != 2+j || row.Tab != strip(a, b, c)[j] {
			t.Fatalf("row %d = %+v", j, row)
		}
	}
	if rows[0].Active || !rows[1].Active || rows[2].Active {
		t.Fatalf("active rows = %v %v %v", rows[0].Active, rows[1].Active, rows[2].Active)
	}
}

func wantPlaces(t *testing.T, got []int, want ...int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got places %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got places %v, want %v", got, want)
		}
	}
}

func TestTabOrderSplicesTheLiftedTabIntoItsDrop(t *testing.T) {
	// previewOrder removes the dragged id and splices it at index, and an
	// index past the dragged tab counts the tab itself.
	wantPlaces(t, tabOrder(4, 0, 3, true), 1, 2, 0, 3)
	wantPlaces(t, tabOrder(4, 3, 1, true), 0, 3, 1, 2)
	wantPlaces(t, tabOrder(4, 1, 1, true), 0, 1, 2, 3)
	wantPlaces(t, tabOrder(4, 1, 2, true), 0, 1, 2, 3)
	wantPlaces(t, tabOrder(3, 2, 0, true), 2, 0, 1)
	wantPlaces(t, tabOrder(3, 0, 3, true), 1, 2, 0)
	wantPlaces(t, tabOrder(3, 1, 2, false), 0, 1, 2)
}

func TestReorderTabsMovesTheTabAndFollowsTheActiveOne(t *testing.T) {
	a, b, c, d := &Tab{}, &Tab{}, &Tab{}, &Tab{}

	tabs, at := reorderTabs(strip(a, b, c, d), 3, 3, 1)
	wantOrder(t, tabs, a, d, b, c)
	if at != 1 {
		t.Fatalf("active at %d, want 1", at)
	}

	tabs, at = reorderTabs(strip(a, b, c, d), 0, 0, 4)
	wantOrder(t, tabs, b, c, d, a)
	if at != 3 {
		t.Fatalf("active at %d, want 3", at)
	}

	// Dropping a tab back onto its own place leaves the strip alone.
	tabs, at = reorderTabs(strip(a, b, c, d), 2, 1, 2)
	wantOrder(t, tabs, a, b, c, d)
	if at != 2 {
		t.Fatalf("active at %d, want 2", at)
	}
	tabs, at = reorderTabs(strip(a, b, c, d), 2, 1, 1)
	wantOrder(t, tabs, a, b, c, d)
	if at != 2 {
		t.Fatalf("active at %d, want 2", at)
	}
}

func TestTabDropIndexTakesTheRowAndTheSlotMidpoint(t *testing.T) {
	// Two wrapped rows of two 100-wide tabs.
	rects := []ui.Rect{
		{X: 0, Y: 0, W: 100, H: 28},
		{X: 102, Y: 0, W: 100, H: 28},
		{X: 0, Y: 30, W: 100, H: 28},
		{X: 102, Y: 30, W: 100, H: 28},
	}
	cases := []struct {
		x, y float32
		want int
	}{
		{10, 10, 0},
		{60, 10, 1},
		{170, 10, 2},
		{10, 40, 2},
		{60, 40, 3},
		{170, 40, 4},
		{10, -20, 0},
		{170, 200, 4},
	}
	for _, c := range cases {
		if got := tabDropIndex(c.x, c.y, rects); got != c.want {
			t.Fatalf("tabDropIndex(%v, %v) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}
