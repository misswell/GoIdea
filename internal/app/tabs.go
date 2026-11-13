package app

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	return m.tabLabelIn(m.tabs, t)
}

// tabLabelIn is tabLabel over a window group: each strip decides its folder
// suffixes from its own tabs, as OpenIDEA's strips label independently.
func (m *Model) tabLabelIn(tabs []*Tab, t *Tab) (name, dir string) {
	paths := make([]string, len(tabs))
	for i, x := range tabs {
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

// previewIndex is where the preview tab sits, or -1 when the strip has none.
func (m *Model) previewIndex() int {
	for i, t := range m.tabs {
		if t.preview {
			return i
		}
	}
	return -1
}

// replacePreviewTab puts tab where the preview tab at is and closes the one it
// replaces, as openTab reuses the preview's slot.
func (m *Model) replacePreviewTab(at int, tab *Tab) {
	if at < 0 || at >= len(m.tabs) {
		m.insertTab(tab)
		return
	}
	old := m.tabs[at]
	m.tabs[at] = tab
	if m.rename != nil && m.rename.tab == old {
		m.rename = nil
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = m.reg.Close(ctx, old.doc.Path())
	}()
}

// dropPreviewTabs closes the preview tab, as a regular open trims it. The
// trimmed preview does not join the closed history, as openTab splices it out.
func (m *Model) dropPreviewTabs() {
	for {
		at := m.previewIndex()
		if at < 0 {
			return
		}
		m.removeTabNow(m.tabs[at])
	}
}

// promoteTab turns the preview tab into a regular one, as an edit, a pin or an
// explicit open does.
func (m *Model) promoteTab(t *Tab) {
	if t == nil || !t.preview {
		return
	}
	t.preview = false
	m.saveSession()
	m.invalidate()
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
// The limit counts both window groups, as OpenIDEA counts a group's tabs.
func (m *Model) enforceTabLimit() {
	limit := m.tabPrefs.limit()
	for {
		unpinned := 0
		for _, t := range m.allTabs() {
			if !t.pinned {
				unpinned++
			}
		}
		if unpinned <= limit {
			return
		}
		var candidates []*Tab
		for _, g := range []groupRef{groupFocused, groupSplit} {
			tabs := m.groupTabsAt(g)
			active := m.active
			if g == groupSplit && m.split != nil {
				active = m.split.active
			}
			candidates = append(candidates, tabLimitCandidates(tabs, active, (*Tab).unsaved)...)
		}
		if len(candidates) == 0 {
			return
		}
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].used < candidates[j].used })
		m.removeTab(candidates[0])
	}
}

// nextActiveAfterClose picks the tab to show after the tab at closeAt closed,
// from the strip that is left. "right" takes the tab that slid into its place,
// "mru" the most recently used one, and the default "left" the tab before it,
// or the new first tab when the closed one was first.
func nextActiveAfterClose(tabs []*Tab, closeAt int, policy string) int {
	switch policy {
	case "right":
		if closeAt < len(tabs) {
			return closeAt
		}
		return max(len(tabs)-1, 0)
	case "mru":
		best := 0
		for i := 1; i < len(tabs); i++ {
			if tabs[i].used > tabs[best].used {
				best = i
			}
		}
		return best
	}
	if closeAt > 0 {
		return closeAt - 1
	}
	return 0
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
	if t == nil {
		return
	}
	g, ok := m.groupOf(t)
	if !ok {
		return
	}
	if t.pinned == pinned && !t.preview {
		return
	}
	// Pinning promotes the preview tab, as togglePin clears its preview flag.
	t.preview = false
	if g == groupSplit {
		tabs, at := placeTab(m.split.tabs, t, pinned)
		m.split.tabs, m.split.active = tabs, at
		m.saveSession()
		m.invalidate()
		return
	}
	tabs, at := placeTab(m.tabs, t, pinned)
	m.tabs = tabs
	m.active = at
	m.saveSession()
	m.invalidate()
}

func (m *Model) hasTab(t *Tab) bool {
	_, ok := m.groupOf(t)
	return ok
}

func (m *Model) tabIndex(t *Tab) int {
	return tabIndexIn(m.tabs, t)
}

// tabIndexIn is the index of t in tabs, or -1.
func tabIndexIn(tabs []*Tab, t *Tab) int {
	for i, x := range tabs {
		if x == t {
			return i
		}
	}
	return -1
}

func (m *Model) activateTab(t *Tab) {
	g, ok := m.groupOf(t)
	if !ok {
		return
	}
	m.focusGroup(g)
	m.activateIn(groupFocused, m.tabIndex(t))
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

// CloseOtherTabs closes every tab but t in t's window group, keeping the
// pinned ones.
func (m *Model) CloseOtherTabs(t *Tab) {
	if t == nil {
		t = m.tab()
	}
	if t == nil {
		return
	}
	m.activateTab(t)
	g, ok := m.groupOf(t)
	if !ok {
		return
	}
	m.closeTabsThen(closeOthersTargets(m.groupTabsAt(g), t), nil)
}

// CloseTabsToLeft closes the unpinned tabs before t in t's window group.
func (m *Model) CloseTabsToLeft(t *Tab) {
	if t == nil {
		return
	}
	if g, ok := m.groupOf(t); ok {
		m.closeTabsThen(closeSideTargets(m.groupTabsAt(g), t, -1), nil)
	}
}

// CloseTabsToRight closes the unpinned tabs after t in t's window group.
func (m *Model) CloseTabsToRight(t *Tab) {
	if t == nil {
		return
	}
	if g, ok := m.groupOf(t); ok {
		m.closeTabsThen(closeSideTargets(m.groupTabsAt(g), t, 1), nil)
	}
}

// CloseAllTabs closes every tab, or every unpinned tab when keepPinned is
// set, as OpenIDEA's Close All but Pinned does.
func (m *Model) CloseAllTabs(keepPinned bool) {
	m.closeTabsThen(closeAllTargets(m.allTabs(), keepPinned), func() { m.settleGroups() })
}

// CloseUnmodifiedTabs closes the tabs without unsaved text or VCS changes.
func (m *Model) CloseUnmodifiedTabs() {
	m.closeTabsThen(closeUnmodifiedTargets(m.allTabs(), tabModifiedForClose), func() { m.settleGroups() })
}

// anyPinned reports whether any window group holds a pinned tab.
func (m *Model) anyPinned() bool {
	for _, t := range m.allTabs() {
		if t.pinned {
			return true
		}
	}
	return false
}

// anyReadOnlyUnpinned reports whether a read-only tab could be closed by
// Close All Read-Only Tabs, which OpenIDEA offers only when one is open.
func (m *Model) anyReadOnlyUnpinned() bool {
	for _, t := range m.allTabs() {
		if !t.pinned && t.doc != nil && t.doc.Info().ReadOnly {
			return true
		}
	}
	return false
}

// CloseReadOnlyTabs closes the unpinned tabs whose files are read-only.
func (m *Model) CloseReadOnlyTabs() {
	var victims []*Tab
	for _, t := range m.allTabs() {
		if !t.pinned && t.doc != nil && t.doc.Info().ReadOnly {
			victims = append(victims, t)
		}
	}
	m.closeTabsThen(victims, func() { m.settleGroups() })
}

// closedTab is a tab Reopen Closed Tab can bring back: its file, the place it
// had in the strip and whether it was pinned.
type closedTab struct {
	path   string
	index  int
	pinned bool
}

// maxClosedTabs is how many closed tabs the history keeps, as the tabs store's
// MAX_CLOSED does.
const maxClosedTabs = 50

// rememberClosed pushes t onto the closed history as its newest entry, dropping
// the oldest past the cap.
func (m *Model) rememberClosed(t closedTab) {
	m.closedTabs = append(m.closedTabs, t)
	if len(m.closedTabs) > maxClosedTabs {
		m.closedTabs = m.closedTabs[len(m.closedTabs)-maxClosedTabs:]
	}
}

// insertTabAt puts tab at at, clamped to the strip: a pinned tab goes to the
// end of the pinned group, as OpenIDEA's openTab places one.
func (m *Model) insertTabAt(tab *Tab, at int) {
	if at < 0 || at > len(m.tabs) {
		at = len(m.tabs)
	}
	if tab.pinned {
		pins := 0
		for pins < len(m.tabs) && m.tabs[pins].pinned {
			pins++
		}
		at = min(at, pins)
	}
	m.tabs = append(m.tabs, nil)
	copy(m.tabs[at+1:], m.tabs[at:])
	m.tabs[at] = tab
}

// tabForPath is the index of the focused group's tab showing path, or -1.
func (m *Model) tabForPath(path string) int {
	path = realPath(path)
	for i, t := range m.tabs {
		if t.doc != nil && t.doc.Path() == path {
			return i
		}
	}
	return -1
}

// tabOpenInAnyGroup reports whether either window group shows path.
func (m *Model) tabOpenInAnyGroup(path string) bool {
	path = realPath(path)
	for _, t := range m.allTabs() {
		if t.doc != nil && t.doc.Path() == path {
			return true
		}
	}
	return false
}

// nextClosed is the newest history entry whose file is not open again, and
// whether there was one; entries that are open are dropped as reopenClosed's
// loop drops them.
func (m *Model) nextClosed() (closedTab, bool) {
	for len(m.closedTabs) > 0 {
		e := m.closedTabs[len(m.closedTabs)-1]
		m.closedTabs = m.closedTabs[:len(m.closedTabs)-1]
		if !m.tabOpenInAnyGroup(e.path) {
			return e, true
		}
	}
	return closedTab{}, false
}

// ReopenClosedTab brings back the most recently closed tab where it sat, as
// editor.reopenClosedTab does.
func (m *Model) ReopenClosedTab() {
	if e, ok := m.nextClosed(); ok {
		m.openWith(e.path, openSpec{at: e.index, pinned: e.pinned})
	}
}
