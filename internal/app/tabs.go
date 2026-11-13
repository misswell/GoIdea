package app

import (
	"path/filepath"

	"github.com/egoist/mygo"
)

// Pinned tabs keep the left of the strip, as OpenIDEA keeps them: a pinned
// tab joins the end of the pinned group, an unpinned one leaves it for right
// after the group, and the bulk closes leave pinned tabs alone.

// placeTab moves t into the pinned group or just after it and returns the new
// strip with t's index, as OpenIDEA's togglePin splices the tab at the count
// of the remaining pinned tabs.
func placeTab(tabs []*Tab, t *Tab, pinned bool) ([]*Tab, int) {
	rest := make([]*Tab, 0, len(tabs))
	for _, x := range tabs {
		if x != t {
			rest = append(rest, x)
		}
	}
	t.pinned = pinned
	at := 0
	for _, x := range rest {
		if x.pinned {
			at++
		}
	}
	rest = append(rest, nil)
	copy(rest[at+1:], rest[at:])
	rest[at] = t
	return rest, at
}

// PinActiveTab pins or unpins the active tab, keeping it selected as
// OpenIDEA's Pin Tab does.
func (m *Model) PinActiveTab() {
	if t := m.tab(); t != nil {
		m.setTabPinned(t, !t.pinned)
	}
}

func (m *Model) setTabPinned(t *Tab, pinned bool) {
	if t == nil || !m.hasTab(t) || t.pinned == pinned {
		return
	}
	tabs, at := placeTab(m.tabs, t, pinned)
	m.tabs = tabs
	m.active = at
	m.saveSession()
	m.invalidate()
}

func (m *Model) hasTab(t *Tab) bool {
	for _, x := range m.tabs {
		if x == t {
			return true
		}
	}
	return false
}

func (m *Model) tabIndex(t *Tab) int {
	for i, x := range m.tabs {
		if x == t {
			return i
		}
	}
	return -1
}

func (m *Model) activateTab(t *Tab) {
	if i := m.tabIndex(t); i >= 0 {
		m.activate(i)
	}
}

// closeTab closes a tab, asking about unsaved text first.
func (m *Model) closeTab(i int) {
	if i < 0 || i >= len(m.tabs) {
		return
	}
	m.closeTabThen(m.tabs[i], nil)
}

// closeTabThen closes t the way OpenIDEA's guarded close does: an unsaved
// document asks first, and a cancel or a failed save leaves the tab open and
// stops whatever would have followed.
func (m *Model) closeTabThen(t *Tab, then func()) {
	if t == nil {
		return
	}
	if !m.hasTab(t) {
		if then != nil {
			then()
		}
		return
	}
	if !t.unsaved() {
		m.removeTab(t)
		if then != nil {
			then()
		}
		return
	}
	name := filepath.Base(t.doc.Path())
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "关闭文件",
			Message: "是否保存对 " + name + " 的更改？",
			Buttons: []string{"保存", "不保存", "取消"}, DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			return
		}
		m.post(func() {
			switch res.Button {
			case 0:
				m.saveTabAsync(t, func() {
					m.removeTab(t)
					if then != nil {
						then()
					}
				})
			case 1:
				m.removeTab(t)
				if then != nil {
					then()
				}
			}
		})
	}()
}

// closeTabsThen closes the tabs one after another, stopping at the first
// cancel or failed save as OpenIDEA's guarded close does.
func (m *Model) closeTabsThen(victims []*Tab, then func()) {
	var next func(int)
	next = func(i int) {
		if i >= len(victims) {
			if then != nil {
				then()
			}
			return
		}
		m.closeTabThen(victims[i], func() { next(i + 1) })
	}
	next(0)
}

// closeOthersTargets are the tabs Close Other Tabs closes: everything but
// keep, while pinned tabs stay as OpenIDEA's closeOthers protects them.
func closeOthersTargets(tabs []*Tab, keep *Tab) []*Tab {
	var out []*Tab
	for _, t := range tabs {
		if t != keep && !t.pinned {
			out = append(out, t)
		}
	}
	return out
}

// closeSideTargets are the unpinned tabs left (side -1) or right (side +1) of
// keep, as Close Tabs to the Left and Right protect pinned tabs.
func closeSideTargets(tabs []*Tab, keep *Tab, side int) []*Tab {
	at := -1
	for i, t := range tabs {
		if t == keep {
			at = i
			break
		}
	}
	if at < 0 {
		return nil
	}
	var out []*Tab
	for i, t := range tabs {
		if t.pinned || i == at {
			continue
		}
		if side < 0 && i < at || side > 0 && i > at {
			out = append(out, t)
		}
	}
	return out
}

// closeAllTargets are every tab Close All Tabs closes, pinned ones included
// unless they are the ones kept.
func closeAllTargets(tabs []*Tab, keepPinned bool) []*Tab {
	var out []*Tab
	for _, t := range tabs {
		if keepPinned && t.pinned {
			continue
		}
		out = append(out, t)
	}
	return out
}

// closeUnmodifiedTargets are the tabs Close Unmodified Tabs closes: pinned
// tabs stay, and so do tabs the modified test reports.
func closeUnmodifiedTargets(tabs []*Tab, modified func(*Tab) bool) []*Tab {
	var out []*Tab
	for _, t := range tabs {
		if !t.pinned && !modified(t) {
			out = append(out, t)
		}
	}
	return out
}

// tabModifiedForClose is IDEA's modified tab: unsaved text, or VCS changes
// shown in the gutter. A file Git cannot answer for is not called modified.
func tabModifiedForClose(t *Tab) bool {
	return t.unsaved() || (!t.gitOff && len(t.gitMarks) > 0)
}

// CloseOtherTabs closes every tab but t, keeping the pinned ones.
func (m *Model) CloseOtherTabs(t *Tab) {
	if t == nil {
		t = m.tab()
	}
	if t == nil {
		return
	}
	m.activateTab(t)
	m.closeTabsThen(closeOthersTargets(m.tabs, t), nil)
}

// CloseTabsToLeft closes the unpinned tabs before t.
func (m *Model) CloseTabsToLeft(t *Tab) {
	m.closeTabsThen(closeSideTargets(m.tabs, t, -1), nil)
}

// CloseTabsToRight closes the unpinned tabs after t.
func (m *Model) CloseTabsToRight(t *Tab) {
	m.closeTabsThen(closeSideTargets(m.tabs, t, 1), nil)
}

// CloseAllTabs closes every tab, or every unpinned tab when keepPinned is
// set, as OpenIDEA's Close All but Pinned does.
func (m *Model) CloseAllTabs(keepPinned bool) {
	m.closeTabsThen(closeAllTargets(m.tabs, keepPinned), nil)
}

// CloseUnmodifiedTabs closes the tabs without unsaved text or VCS changes.
func (m *Model) CloseUnmodifiedTabs() {
	m.closeTabsThen(closeUnmodifiedTargets(m.tabs, tabModifiedForClose), nil)
}

// anyPinned reports whether the strip holds a pinned tab.
func (m *Model) anyPinned() bool {
	for _, t := range m.tabs {
		if t.pinned {
			return true
		}
	}
	return false
}

// anyReadOnlyUnpinned reports whether a read-only tab could be closed by
// Close All Read-Only Tabs, which OpenIDEA offers only when one is open.
func (m *Model) anyReadOnlyUnpinned() bool {
	for _, t := range m.tabs {
		if !t.pinned && t.doc != nil && t.doc.Info().ReadOnly {
			return true
		}
	}
	return false
}

// CloseReadOnlyTabs closes the unpinned tabs whose files are read-only.
func (m *Model) CloseReadOnlyTabs() {
	var victims []*Tab
	for _, t := range m.tabs {
		if !t.pinned && t.doc != nil && t.doc.Info().ReadOnly {
			victims = append(victims, t)
		}
	}
	m.closeTabsThen(victims, nil)
}
