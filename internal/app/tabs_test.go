package app

import "testing"

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
