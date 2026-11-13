package app

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo"
)

// tabPath is the file a tab edits; a tab under construction has none.
func tabPath(t *Tab) string {
	if t == nil || t.doc == nil {
		return ""
	}
	return t.doc.Path()
}

// tabLabel is the name a tab shows, as EditorArea.tsx builds it: the base name
// without its extension when Show file extension is off, "*" in front when
// Mark modified is on and the text is unsaved, and the parent folder beside it
// when another tab shows the same name.
func (m *Model) tabLabel(t *Tab) (name, dir string) {
	paths := make([]string, len(m.tabs))
	for i, x := range m.tabs {
		paths[i] = tabPath(x)
	}
	return tabLabelFor(tabPath(t), m.tabPrefs, t.unsaved(), paths)
}

// tabLabelFor is tabLabel over paths: siblings holds every path in the strip,
// so the folder suffix appears only when the name is not unique.
func tabLabelFor(path string, prefs TabPrefs, dirty bool, siblings []string) (name, dir string) {
	base := filepath.Base(path)
	if prefs.HideExtension {
		if ext := filepath.Ext(base); ext != "" && ext != base {
			base = strings.TrimSuffix(base, ext)
		}
	}
	name = base
	if prefs.MarkModified && dirty {
		name = "*" + name
	}
	if prefs.NoDirectoryForNonUnique {
		return name, ""
	}
	same := 0
	for _, p := range siblings {
		if filepath.Base(p) == filepath.Base(path) {
			same++
		}
	}
	if same > 1 {
		dir = filepath.Base(filepath.Dir(path))
	}
	return name, dir
}

// tabSortKey is the title Sort tabs alphabetically compares, folded to lower
// case as sortTabs avoids case deciding the order.
func tabSortKey(t *Tab) string {
	return strings.ToLower(filepath.Base(tabPath(t)))
}

// sortTabsAlphabetically orders pinned tabs first and each group by title,
// keeping the active tab selected, as editor.tabs.sortAlphabetically sorts.
func (m *Model) sortTabsAlphabetically() {
	if !m.tabPrefs.SortAlphabetically || len(m.tabs) < 2 {
		return
	}
	tabs, active := sortTabsByTitle(m.tabs, m.active, tabSortKey)
	m.tabs, m.active = tabs, active
}

// sortTabsByTitle is the alphabetical order: pinned tabs first, each group by
// key, stable so equal titles keep their places. It returns the new strip and
// where the active tab went.
func sortTabsByTitle(tabs []*Tab, active int, key func(*Tab) string) ([]*Tab, int) {
	if active < 0 || active >= len(tabs) {
		return tabs, active
	}
	ordered := append([]*Tab(nil), tabs...)
	keys := make(map[*Tab]string, len(ordered))
	for _, t := range ordered {
		keys[t] = key(t)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.pinned != b.pinned {
			return a.pinned
		}
		return keys[a] < keys[b]
	})
	at := -1
	for i, t := range ordered {
		if t == tabs[active] {
			at = i
			break
		}
	}
	return ordered, at
}

// insertTab puts a new tab after the active one, or at the end when Open new
// tabs at the end is on, as openTab's index does.
func (m *Model) insertTab(t *Tab) {
	if m.tabPrefs.OpenAtEnd || m.active < 0 || m.active >= len(m.tabs) {
		m.tabs = append(m.tabs, t)
		return
	}
	at := m.active + 1
	pins := 0
	for _, x := range m.tabs {
		if x.pinned {
			pins++
		}
	}
	at = max(at, pins)
	m.tabs = append(m.tabs, nil)
	copy(m.tabs[at+1:], m.tabs[at:])
	m.tabs[at] = t
}

// tabLimitCandidates are the tabs the limit may close, least recently used
// first: pinned tabs, the active one and tabs with unsaved text stay, so the
// limit can not drop an edit or the tab in front.
func tabLimitCandidates(tabs []*Tab, active int, unsaved func(*Tab) bool) []*Tab {
	var out []*Tab
	for i, t := range tabs {
		if t.pinned || i == active || unsaved(t) {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].used < out[j].used })
	return out
}

// enforceTabLimit closes tabs beyond Tab limit, as enforceTabLimit does:
// pinned tabs do not count, and a strip with nothing closeable keeps its tabs.
func (m *Model) enforceTabLimit() {
	limit := m.tabPrefs.limit()
	for {
		unpinned := 0
		for _, t := range m.tabs {
			if !t.pinned {
				unpinned++
			}
		}
		if unpinned <= limit {
			return
		}
		candidates := tabLimitCandidates(m.tabs, m.active, (*Tab).unsaved)
		if len(candidates) == 0 {
			return
		}
		m.removeTab(candidates[0])
	}
}

// noteTabUse marks t as the most recently used tab, as the mru list ranks
// tabs for the tab limit.
func (m *Model) noteTabUse(t *Tab) {
	if t == nil {
		return
	}
	m.tabUse++
	t.used = m.tabUse
}

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
