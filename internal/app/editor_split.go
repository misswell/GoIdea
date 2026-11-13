package app

import (
	"context"

	"github.com/egoist/mygo/ui"
)

// This file is OpenIDEA's window groups: the editor can show two groups side
// by side or stacked, each with its own tabs and its own active tab. The
// focused group stays in Model.tabs and Model.active, so the tab commands
// that act on the active tab keep working unchanged; the other group lives in
// Model.split.

// splitGroup is the second editor window group: its tabs, the tab it shows,
// and the strip state that goes with a strip.
type splitGroup struct {
	tabs         []*Tab
	active       int
	scroll       ui.ScrollState
	pinnedScroll ui.ScrollState
	sideScroll   ui.ScrollState
	shown        int
	menuOpen     bool
}

// groupRef names a window group in the commands that act on one.
type groupRef int

const (
	groupFocused groupRef = iota
	groupSplit
)

// groupView is a window group as a strip draws it: tabButtons, tabStrip and
// the vertical strip take one, so the focused group and the split group draw
// with the same code.
type groupView struct {
	ref      groupRef
	tabs     []*Tab
	active   int
	scroll   *ui.ScrollState
	pinned   *ui.ScrollState
	side     *ui.ScrollState
	shown    *int
	menuOpen *bool
	focused  bool
}

// tab is the group's active tab, or nil.
func (g groupView) tab() *Tab {
	if g.active < 0 || g.active >= len(g.tabs) {
		return nil
	}
	return g.tabs[g.active]
}

// focusedView is the window group the commands act on.
func (m *Model) focusedView() groupView {
	return groupView{
		ref: groupFocused, tabs: m.tabs, active: m.active,
		scroll: &m.tabsScroll, pinned: &m.tabsPinnedScroll,
		side: &m.tabsSideScroll, shown: &m.tabsShown,
		menuOpen: &m.tabsMenuOpen, focused: true,
	}
}

// splitView is the other window group; its zero view has no tabs.
func (m *Model) splitView() groupView {
	if m.split == nil {
		return groupView{ref: groupSplit}
	}
	s := m.split
	return groupView{
		ref: groupSplit, tabs: s.tabs, active: s.active,
		scroll: &s.scroll, pinned: &s.pinnedScroll,
		side: &s.sideScroll, shown: &s.shown, menuOpen: &s.menuOpen,
	}
}

// groupTabsAt returns the tabs of g.
func (m *Model) groupTabsAt(g groupRef) []*Tab {
	if g == groupSplit {
		if m.split == nil {
			return nil
		}
		return m.split.tabs
	}
	return m.tabs
}

// allTabs is every open tab, the focused group's first.
func (m *Model) allTabs() []*Tab {
	out := make([]*Tab, 0, len(m.tabs)+len(m.groupTabsAt(groupSplit)))
	out = append(out, m.tabs...)
	out = append(out, m.groupTabsAt(groupSplit)...)
	return out
}

// groupOf finds the group showing t.
func (m *Model) groupOf(t *Tab) (groupRef, bool) {
	for _, x := range m.tabs {
		if x == t {
			return groupFocused, true
		}
	}
	if m.split != nil {
		for _, x := range m.split.tabs {
			if x == t {
				return groupSplit, true
			}
		}
	}
	return groupFocused, false
}

// focusGroup makes g the group the commands act on, as clicking a tab in the
// other pane does. The two groups keep their places: their tabs swap and the
// split's side flips with them, so neither pane moves.
func (m *Model) focusGroup(g groupRef) {
	if g == groupFocused || m.split == nil {
		return
	}
	s := m.split
	m.tabs, s.tabs = s.tabs, m.tabs
	m.active, s.active = s.active, m.active
	m.tabsScroll, s.scroll = s.scroll, m.tabsScroll
	m.tabsPinnedScroll, s.pinnedScroll = s.pinnedScroll, m.tabsPinnedScroll
	m.tabsSideScroll, s.sideScroll = s.sideScroll, m.tabsSideScroll
	m.tabsShown, s.shown = s.shown, m.tabsShown
	m.tabsMenuOpen, s.menuOpen = s.menuOpen, m.tabsMenuOpen
	m.splitFirst = !m.splitFirst
	// A drag belongs to the strip it started in.
	m.tabDrag = tabDragState{}
	m.applyActive()
	m.saveSession()
	m.invalidate()
}

// settleGroups collapses a window group left without tabs, as OpenIDEA removes
// an empty pane: a split without tabs is gone, and a focused group without
// tabs leaves the split group as the editor.
func (m *Model) settleGroups() {
	if m.split == nil {
		m.splitFirst = false
		return
	}
	if len(m.split.tabs) == 0 {
		m.split = nil
		m.splitFirst = false
		return
	}
	if len(m.tabs) == 0 {
		m.focusGroup(groupSplit)
		m.split = nil
		m.splitFirst = false
	}
}

// duplicateTab makes a second tab of t's document. The registry hands out the
// same document, so both tabs edit one buffer and one undo history, as two
// OpenIDEA panes of a file do; the caller closes it like any other tab.
func (m *Model) duplicateTab(t *Tab) *Tab {
	if t == nil || t.doc == nil {
		return nil
	}
	doc, err := m.reg.Open(context.Background(), t.doc.Path())
	if err != nil {
		m.fail(t.doc.Path(), err)
		return nil
	}
	return m.newTab(doc)
}

// addTabTo puts t at the end of g.
func (m *Model) addTabTo(g groupRef, t *Tab) {
	if g == groupSplit && m.split != nil {
		m.split.tabs = append(m.split.tabs, t)
		m.split.active = len(m.split.tabs) - 1
		return
	}
	m.tabs = append(m.tabs, t)
	m.active = len(m.tabs) - 1
}

// removeFromGroup takes t out of g and reports its index, or -1. The active
// tab follows the strip's close rules.
func (m *Model) removeFromGroup(g groupRef, t *Tab) int {
	if g == groupSplit && m.split != nil {
		s := m.split
		idx := tabIndexIn(s.tabs, t)
		if idx < 0 {
			return -1
		}
		s.tabs = append(s.tabs[:idx], s.tabs[idx+1:]...)
		switch {
		case len(s.tabs) == 0:
			s.active = 0
		case idx == s.active:
			s.active = nextActiveAfterClose(s.tabs, idx, m.tabPrefs.activation())
		case idx < s.active:
			s.active--
		}
		return idx
	}
	idx := tabIndexIn(m.tabs, t)
	if idx < 0 {
		return -1
	}
	wasActive := idx == m.active
	m.tabs = append(m.tabs[:idx], m.tabs[idx+1:]...)
	switch {
	case len(m.tabs) == 0:
		m.active = 0
	case wasActive:
		m.active = nextActiveAfterClose(m.tabs, idx, m.tabPrefs.activation())
	case idx < m.active:
		m.active--
	}
	return idx
}

// SplitEditorRight is editor.splitRight: a second window group opens beside
// the editor with a copy of its tab.
func (m *Model) SplitEditorRight() { m.splitEditor(false) }

// SplitEditorDown splits the editor below instead.
func (m *Model) SplitEditorDown() { m.splitEditor(true) }

// splitEditor copies the active tab into the other window group and focuses
// it, as OpenIDEA's splitPane copies the pane's tab into the new one. The
// two-group subset keeps one other group, so a further split adds its copy
// beside the first.
func (m *Model) splitEditor(down bool) {
	t := m.tab()
	if t == nil {
		return
	}
	dup := m.duplicateTab(t)
	if dup == nil {
		return
	}
	if m.split == nil {
		m.split = &splitGroup{tabs: []*Tab{dup}}
		m.splitFirst = false
	} else {
		m.split.tabs = append(m.split.tabs, dup)
		m.split.active = len(m.split.tabs) - 1
	}
	m.splitDown = down
	m.focusGroup(groupSplit)
	m.revealEditor()
	m.status = "已拆分编辑器"
	m.invalidate()
}

// SplitAndMoveRight is OpenIDEA's Split and Move Right: the active tab leaves
// its group for a new one on the right.
func (m *Model) SplitAndMoveRight() { m.splitAndMove(false) }

// SplitAndMoveDown is the same, below the editor.
func (m *Model) SplitAndMoveDown() { m.splitAndMove(true) }

// splitAndMove moves the active tab into the other window group, making the
// group when there is none. A lone tab would leave its group empty, which
// OpenIDEA collapses, so the move leaves the editor as it was.
func (m *Model) splitAndMove(down bool) {
	t := m.tab()
	if t == nil {
		return
	}
	if m.split == nil {
		if len(m.tabs) < 2 {
			return
		}
		m.split = &splitGroup{}
		m.splitFirst = false
	}
	m.splitDown = down
	m.moveToGroup(t, groupSplit)
}

// moveToGroup takes t to the other group and focuses it there.
func (m *Model) moveToGroup(t *Tab, to groupRef) {
	from, ok := m.groupOf(t)
	if !ok || from == to {
		return
	}
	m.removeFromGroup(from, t)
	m.addTabTo(to, t)
	m.settleGroups()
	if g, ok := m.groupOf(t); ok && g == groupSplit {
		m.focusGroup(groupSplit)
	}
	m.revealEditor()
	m.applyActive()
	m.saveSession()
	m.invalidate()
}

// MoveToOppositeGroup moves the active tab to the other window group, as the
// tab menu's entry does.
func (m *Model) MoveToOppositeGroup() {
	t := m.tab()
	if t == nil || m.split == nil {
		return
	}
	m.moveToGroup(t, groupSplit)
}

// OpenInOppositeGroup opens a copy of the active tab in the other group,
// splitting first when the editor has one group, as OpenIDEA's
// openInOpposite does.
func (m *Model) OpenInOppositeGroup() {
	t := m.tab()
	if t == nil {
		return
	}
	dup := m.duplicateTab(t)
	if dup == nil {
		return
	}
	if m.split == nil {
		m.split = &splitGroup{}
		m.splitFirst = false
	}
	m.split.tabs = append(m.split.tabs, dup)
	m.split.active = len(m.split.tabs) - 1
	m.focusGroup(groupSplit)
	m.revealEditor()
	m.status = "已在另一组中打开 " + tabPath(t)
	m.invalidate()
}

// ChangeSplitterOrientation flips the split between side by side and stacked.
func (m *Model) ChangeSplitterOrientation() {
	if m.split == nil {
		return
	}
	m.splitDown = !m.splitDown
	m.saveSession()
	m.invalidate()
}

// Unsplit closes the active group's tabs, guarded, and leaves the other group
// as the editor, as OpenIDEA's unsplit.
func (m *Model) Unsplit() {
	if m.split == nil {
		return
	}
	victims := append([]*Tab(nil), m.tabs...)
	m.closeTabsThen(victims, func() {
		m.settleGroups()
		m.applyActive()
		m.invalidate()
	})
}

// UnsplitAll closes the other group's tabs, guarded, keeping the active group.
func (m *Model) UnsplitAll() {
	if m.split == nil {
		return
	}
	victims := append([]*Tab(nil), m.split.tabs...)
	m.closeTabsThen(victims, func() {
		m.split = nil
		m.splitFirst = false
		m.applyActive()
		m.saveSession()
		m.invalidate()
	})
}

// GoToNextSplitter focuses the other window group, as editor.nextSplitter
// does; two groups make next and previous the same step.
func (m *Model) GoToNextSplitter() {
	if m.split == nil {
		return
	}
	m.focusGroup(groupSplit)
}

// GoToPreviousSplitter is GoToNextSplitter for the other direction.
func (m *Model) GoToPreviousSplitter() {
	m.GoToNextSplitter()
}
