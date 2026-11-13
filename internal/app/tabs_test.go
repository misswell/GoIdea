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
