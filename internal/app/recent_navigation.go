package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	recentFileLimit     = 200
	recentPlaceLimit    = 60
	recentPlaceDistance = 5
	recentPlaceSettle   = 500 * time.Millisecond
)

type recentPlace struct {
	path      string
	pos       editor.Pos
	lines     []string
	firstLine int
	changed   bool
	at        time.Time
}

type recentObservation struct {
	tab     *Tab
	pos     editor.Pos
	version uint64
	changed bool
	since   time.Time
}

type recentNavigation struct {
	paths          []string
	edited         map[string]bool
	places         []recentPlace
	observed       recentObservation
	popup          *recentNavPopup
	history        navigationHistory
	edits          editHistory
	suppressUntil  time.Time
	suppressTarget string
	suppressLine   int
}

type recentNavPopup struct {
	locations     bool
	onlyChanged   bool
	switcher      bool
	switcherSaw   bool
	query         string
	files         []string
	switcherItems []recentSwitcherItem
	fileSelection recentFileSelection
	removed       map[string]bool
	tools         []*toolWindowSpec
	places        []recentPlace
	onTools       bool
	index         int
	loading       bool
	err           string
	now           time.Time
	scroll        ui.ScrollState
}

// recentSwitcherItem is one editor row in OpenIDEA's Ctrl+Tab switcher.
// Files reopen by path. Non-file editors keep their tab so the same page,
// such as an Agent task or the new Agent page, takes focus without reopening
// a different document.
type recentSwitcherItem struct {
	path    string
	title   string
	tab     *Tab
	nonFile bool
}

type recentFileSelection struct {
	anchor int
	keys   map[string]bool
}

type recentSelectionMode uint8

const (
	recentSelectionSingle recentSelectionMode = iota
	recentSelectionRange
	recentSelectionToggle
)

func (n *recentNavigation) visit(path string) {
	if path == "" {
		return
	}
	n.paths = slices.DeleteFunc(n.paths, func(p string) bool { return p == path })
	n.paths = append(n.paths, "")
	copy(n.paths[1:], n.paths)
	n.paths[0] = path
	if len(n.paths) > recentFileLimit {
		n.paths = n.paths[:recentFileLimit]
	}
}

func recentNearby(a, b recentPlace) bool {
	return a.path == b.path && a.pos.Line >= b.pos.Line-recentPlaceDistance &&
		a.pos.Line <= b.pos.Line+recentPlaceDistance
}

func (n *recentNavigation) record(place recentPlace) {
	// Keep the edit flag when the user moves the caret near an edited line.
	n.places = slices.DeleteFunc(n.places, func(old recentPlace) bool {
		if !recentNearby(old, place) {
			return false
		}
		place.changed = place.changed || old.changed
		return true
	})
	n.places = append(n.places, recentPlace{})
	copy(n.places[1:], n.places)
	n.places[0] = place
	if len(n.places) > recentPlaceLimit {
		clear(n.places[recentPlaceLimit:])
		n.places = n.places[:recentPlaceLimit]
	}
}

func (m *Model) observeRecentLocation(now time.Time, flush bool) {
	n := &m.recentNav
	o := &n.observed
	t := m.tab()
	if t == nil || t.kind != "" {
		*o = recentObservation{}
		return
	}
	buf := t.doc.Buffer()
	version, pos := buf.Version(), t.state.CursorPos()
	changed := version != t.seen
	if changed {
		if n.edited == nil {
			n.edited = make(map[string]bool)
		}
		n.edited[t.doc.Path()] = true
		if place, ok := m.currentNavigationLocation(now); ok {
			n.edits.record(place)
		}
	}
	if o.tab != t {
		*o = recentObservation{tab: t, pos: pos, version: version, changed: changed, since: now}
	} else if o.pos != pos || o.version != version {
		o.pos, o.version, o.since = pos, version, now
		o.changed = o.changed || changed
	}
	if o.since.IsZero() || (!flush && now.Sub(o.since) < recentPlaceSettle) {
		return
	}
	source := buf.Snapshot()
	start := max(0, pos.Line-1)
	lineText := source.Line(pos.Line)
	col := int(utf16Len(lineText[:min(pos.Col, len(lineText))]))
	place := recentPlace{
		path: t.doc.Path(), pos: editor.Pos{Line: pos.Line, Col: col},
		firstLine: start, changed: o.changed, at: now,
	}
	for line := start; line < min(source.LineCount(), pos.Line+2); line++ {
		text := source.Line(line)
		if len(text) > 300 {
			end := 300
			for !utf8.RuneStart(text[end]) {
				end--
			}
			text = strings.Clone(text[:end])
		}
		place.lines = append(place.lines, text)
	}
	n.record(place)
	o.since, o.changed = time.Time{}, false
}

// currentNavigationLocation is the active text editor's path and UTF-16
// caret. Internal pages and tabs without a backend document have no place in
// OpenIDEA's navigation and edit histories.
func (m *Model) currentNavigationLocation(now time.Time) (navigationLocation, bool) {
	t := m.tab()
	if t == nil || t.kind != "" || t.doc == nil || t.state == nil {
		return navigationLocation{}, false
	}
	pos := t.state.CursorPos()
	line := t.doc.Buffer().Snapshot().Line(pos.Line)
	col := int(utf16Len(line[:min(pos.Col, len(line))]))
	return navigationLocation{
		path: t.doc.Path(),
		pos:  editor.Pos{Line: pos.Line, Col: col},
		at:   now,
	}, true
}

// recordNavigation keeps the current place current for ordinary caret moves,
// and records a back entry when the move came from a navigation command.
func (m *Model) recordNavigation(navigation bool) {
	place, ok := m.currentNavigationLocation(time.Now())
	if !ok {
		return
	}
	n := &m.recentNav
	if !n.suppressUntil.IsZero() {
		if time.Now().Before(n.suppressUntil) &&
			(place.path != n.suppressTarget || place.pos.Line != n.suppressLine) {
			return
		}
		n.suppressUntil = time.Time{}
		n.suppressTarget = ""
	}
	n.history.update(place, navigation)
}

func (m *Model) suppressNavigationUntil(path string, line int) {
	m.recentNav.suppressUntil = time.Now().Add(10 * time.Second)
	m.recentNav.suppressTarget = path
	m.recentNav.suppressLine = line
}

func (m *Model) clearNavigationSuppression(path string, line int) {
	if t := m.tab(); t != nil && t.doc != nil && t.doc.Path() == path &&
		t.state != nil && t.state.CursorPos().Line == line {
		m.recentNav.suppressUntil = time.Time{}
		m.recentNav.suppressTarget = ""
		m.recentNav.suppressLine = 0
	}
}

// removeNavigationTree drops a deleted file or directory from Back / Forward
// and Last / Next Edit Location, as OpenIDEA does when files disappear.
func (m *Model) removeNavigationTree(path string) {
	m.recentNav.history.removeTree(path, true)
	m.recentNav.edits.removeTree(path, true)
	if m.recentNav.suppressTarget == "" ||
		navigationPathUnder(m.recentNav.suppressTarget, path, true) {
		m.recentNav.suppressUntil = time.Time{}
		m.recentNav.suppressTarget = ""
		m.recentNav.suppressLine = 0
	}
}

// NavigateBack and NavigateForward are Navigate › Back / Forward.
func (m *Model) NavigateBack() { m.navigateHistory(-1) }

func (m *Model) NavigateForward() { m.navigateHistory(1) }

func (m *Model) navigateHistory(delta int) bool {
	m.recordNavigation(false)
	h := &m.recentNav.history
	var place navigationLocation
	var ok bool
	if delta < 0 {
		place, ok = h.back()
	} else {
		place, ok = h.forward()
	}
	if !ok {
		return false
	}
	m.suppressNavigationUntil(place.path, place.pos.Line)
	m.revealAt(place.path, place.pos.Line, place.pos.Col)
	m.clearNavigationSuppression(place.path, place.pos.Line)
	m.recordNavigation(false)
	return true
}

// LastEditLocation and NextEditLocation walk OpenIDEA's edit history.
func (m *Model) LastEditLocation() {
	caret, _ := m.currentNavigationLocation(time.Now())
	place, ok := m.recentNav.edits.back(&caret)
	if ok {
		m.navigateToEdit(place)
	}
}

func (m *Model) NextEditLocation() {
	place, ok := m.recentNav.edits.forward()
	if ok {
		m.navigateToEdit(place)
	}
}

func (m *Model) navigateToEdit(place navigationLocation) {
	m.recordNavigation(false)
	m.suppressNavigationUntil(place.path, place.pos.Line)
	m.revealAt(place.path, place.pos.Line, place.pos.Col)
	m.clearNavigationSuppression(place.path, place.pos.Line)
	m.recordNavigation(false)
}

func (m *Model) openRecentNavigation(locations bool) {
	if m.Root() == "" || m.showingWelcome {
		return
	}
	n := &m.recentNav
	now := time.Now()
	m.observeRecentLocation(now, true)
	if p := n.popup; p != nil && p.locations == locations {
		p.onlyChanged = !p.onlyChanged
		m.refreshRecentNavigation(true)
		return
	}
	p := &recentNavPopup{locations: locations, now: now}
	n.popup = p
	m.refreshRecentNavigation(true)
	if locations || m.ctx == nil || m.mgr == nil {
		return
	}
	p.loading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "recent_files", nil)
		var paths []string
		if err == nil {
			err = json.Unmarshal(raw, &paths)
		}
		m.post(func() {
			if m.recentNav.popup != p {
				return
			}
			p.loading = false
			if err != nil {
				p.err = "Could not load recent files: " + err.Error()
			} else {
				for _, path := range paths {
					if path != "" && !slices.Contains(n.paths, path) && len(n.paths) < recentFileLimit {
						n.paths = append(n.paths, path)
					}
				}
			}
			m.refreshRecentNavigation(true)
		})
	}()
}

// CycleSwitcher ports OpenIDEA's Ctrl+Tab switcher. The first press opens the
// recent-file popup in switcher mode; later presses move its selection.
func (m *Model) CycleSwitcher(delta int) {
	if delta == 0 {
		return
	}
	p := m.recentNav.popup
	if p != nil && !p.locations && p.switcher {
		m.moveRecentNavigation(delta, false, false)
		return
	}
	m.openRecentNavigation(false)
	p = m.recentNav.popup
	if p == nil {
		return
	}
	p.switcher = true
	p.onlyChanged = false
	m.refreshRecentNavigation(true)
	if delta < 0 && p.switcherTotal() > 0 {
		files, tools := len(p.switcherItems), len(p.tools)
		if files > 0 {
			p.onTools, p.index = false, files-1
		} else if tools > 0 {
			p.onTools, p.index = true, tools-1
		}
	}
	m.invalidate()
}

func recentMatches(query, value string) bool {
	_, _, ok := fuzzySubsequence(query, value)
	return ok
}

func (m *Model) refreshRecentNavigation(reset bool) {
	n := &m.recentNav
	p := n.popup
	q := strings.ToLower(strings.TrimSpace(p.query))
	p.files, p.places, p.tools = p.files[:0], p.places[:0], p.tools[:0]
	if p.locations {
		for _, place := range n.places {
			if p.onlyChanged && !place.changed {
				continue
			}
			if recentMatches(q, filepath.Base(place.path)) ||
				strings.Contains(strings.ToLower(projectRelative(m.Root(), place.path)), q) ||
				slices.ContainsFunc(place.lines, func(line string) bool { return strings.Contains(strings.ToLower(line), q) }) {
				p.places = append(p.places, place)
			}
		}
	} else {
		if p.switcher {
			p.switcherItems = m.recentSwitcherItems(q)
			p.switcherItems = slices.DeleteFunc(p.switcherItems, func(item recentSwitcherItem) bool {
				return p.removed[item.path]
			})
			for _, item := range p.switcherItems {
				if !item.nonFile {
					p.files = append(p.files, item.path)
				}
			}
		} else {
			for _, path := range n.paths {
				if p.removed[path] {
					continue
				}
				if (!p.onlyChanged || n.edited[path]) && recentMatches(q, projectRelative(m.Root(), path)) {
					p.files = append(p.files, path)
				}
			}
		}
		for _, id := range toolWindowOrder {
			spec := dockSpec(id)
			if recentMatches(q, spec.title) {
				p.tools = append(p.tools, spec)
			}
		}
		slices.SortStableFunc(p.tools, func(a, b *toolWindowSpec) int {
			return int(a.anchor-b.anchor)*1000 + a.order - b.order
		})
	}
	if reset {
		p.index = 0
		editorCount := len(p.files)
		if p.switcher {
			editorCount = len(p.switcherItems)
		}
		p.onTools = !p.locations && editorCount == 0 && len(p.tools) > 0
		if t := m.tab(); !p.locations && q == "" && editorCount > 1 && t != nil && t.kind == "" && tabPath(t) == p.files[0] {
			p.index = 1
		}
		p.scroll.Y = 0
		p.fileSelection = recentFileSelection{anchor: p.index}
	}
	p.index = clampSymbolSel(p.index, p.count())
	p.normalizeRecentFileSelection()
	m.invalidate()
}

// openRecentSwitcherPaths returns the file rows of recentSwitcherItems.
func (m *Model) openRecentSwitcherPaths() []string {
	items := m.recentSwitcherItems("")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if !item.nonFile {
			out = append(out, item.path)
		}
	}
	return out
}

// recentSwitcherItems follows OpenIDEA's switcherItems: editors active this
// session first, then remaining files in recent-file order, then remaining
// non-file editors in tab order. File panes of one path collapse to one row.
func (m *Model) recentSwitcherItems(query string) []recentSwitcherItem {
	tabs := m.allTabs()
	fileTabs := make(map[string]*Tab)
	for _, tab := range tabs {
		path := tabPath(tab)
		if path == "" || tab.kind != "" {
			continue
		}
		key := realPath(path)
		if fileTabs[key] == nil {
			fileTabs[key] = tab
		}
	}

	active := make([]*Tab, 0, len(tabs))
	for _, tab := range tabs {
		if tab.used > 0 {
			active = append(active, tab)
		}
	}
	slices.SortStableFunc(active, func(a, b *Tab) int {
		switch {
		case a.used > b.used:
			return -1
		case a.used < b.used:
			return 1
		default:
			return 0
		}
	})

	var items []recentSwitcherItem
	seenFiles := make(map[string]bool)
	seenTabs := make(map[*Tab]bool)
	addFile := func(tab *Tab) {
		path := tabPath(tab)
		key := realPath(path)
		if path == "" || seenFiles[key] {
			return
		}
		seenFiles[key] = true
		items = append(items, recentSwitcherItem{path: path, title: filepath.Base(path)})
	}
	addNonFile := func(tab *Tab) {
		path := tabPath(tab)
		if tab == nil || path == "" || seenTabs[tab] {
			return
		}
		seenTabs[tab] = true
		items = append(items, recentSwitcherItem{
			path: path, title: m.agentEditorTitle(tab), tab: tab, nonFile: true,
		})
	}
	for _, tab := range active {
		if tab.kind == "" {
			addFile(tab)
		} else {
			addNonFile(tab)
		}
	}
	for _, path := range m.recentNav.paths {
		if tab := fileTabs[realPath(path)]; tab != nil {
			addFile(tab)
		}
	}
	for _, tab := range tabs {
		if tab.kind == "" {
			addFile(tab)
		}
	}
	for _, tab := range tabs {
		if tab.kind != "" {
			addNonFile(tab)
		}
	}

	q := strings.TrimSpace(query)
	if q == "" {
		return items
	}
	out := items[:0]
	for _, item := range items {
		value := item.title
		if !item.nonFile {
			value = projectRelative(m.Root(), item.path)
			if !recentMatches(q, filepath.Base(value)) && !recentMatches(q, value) {
				continue
			}
		} else if !recentMatches(q, value) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (p *recentNavPopup) count() int {
	if p.locations {
		return len(p.places)
	}
	if p.switcher && !p.onTools {
		return len(p.switcherItems)
	}
	if p.onTools {
		return len(p.tools)
	}
	return len(p.files)
}

func (p *recentNavPopup) switcherTotal() int {
	return len(p.switcherItems) + len(p.tools)
}

func (p *recentNavPopup) normalizeRecentFileSelection() {
	if len(p.files) == 0 {
		p.fileSelection = recentFileSelection{}
		return
	}
	p.index = clampSymbolSel(p.index, len(p.files))
	p.fileSelection.anchor = clampSymbolSel(p.fileSelection.anchor, len(p.files))
	if p.fileSelection.keys == nil {
		return
	}
	for path := range p.fileSelection.keys {
		if !slices.Contains(p.files, path) {
			delete(p.fileSelection.keys, path)
		}
	}
	if len(p.fileSelection.keys) == 0 {
		p.fileSelection.keys = nil
	}
}

func (p *recentNavPopup) isRecentFileSelected(index int) bool {
	if index < 0 || index >= len(p.files) {
		return false
	}
	if p.fileSelection.keys == nil {
		return index == p.index
	}
	return p.fileSelection.keys[p.files[index]]
}

func (p *recentNavPopup) selectedRecentFiles() []string {
	if len(p.files) == 0 || p.index < 0 || p.index >= len(p.files) {
		return nil
	}
	if p.fileSelection.keys == nil {
		return []string{p.files[p.index]}
	}
	out := make([]string, 0, len(p.fileSelection.keys))
	for _, path := range p.files {
		if p.fileSelection.keys[path] {
			out = append(out, path)
		}
	}
	if len(out) == 0 {
		return []string{p.files[p.index]}
	}
	return out
}

func (p *recentNavPopup) selectRecentFile(index int, mode recentSelectionMode) {
	if index < 0 || index >= len(p.files) {
		return
	}
	switch mode {
	case recentSelectionRange:
		anchor := clampSymbolSel(p.fileSelection.anchor, len(p.files))
		if p.fileSelection.keys == nil {
			anchor = clampSymbolSel(p.index, len(p.files))
		}
		start, end := anchor, index
		if start > end {
			start, end = end, start
		}
		keys := make(map[string]bool, end-start+1)
		for _, path := range p.files[start : end+1] {
			keys[path] = true
		}
		p.fileSelection = recentFileSelection{anchor: anchor, keys: keys}
		p.index = index
	case recentSelectionToggle:
		keys := make(map[string]bool, len(p.fileSelection.keys)+1)
		if p.fileSelection.keys == nil {
			if p.index >= 0 && p.index < len(p.files) {
				keys[p.files[p.index]] = true
			}
		} else {
			for path, selected := range p.fileSelection.keys {
				if selected {
					keys[path] = true
				}
			}
		}
		path := p.files[index]
		if keys[path] {
			delete(keys, path)
			if len(keys) == 0 {
				p.fileSelection = recentFileSelection{anchor: index}
				p.index = index
				return
			}
			lead := index
			for i := len(p.files) - 1; i >= 0; i-- {
				if keys[p.files[i]] {
					lead = i
					break
				}
			}
			p.fileSelection = recentFileSelection{anchor: lead, keys: keys}
			p.index = lead
			return
		}
		keys[path] = true
		p.fileSelection = recentFileSelection{anchor: index, keys: keys}
		p.index = index
	default:
		p.fileSelection = recentFileSelection{anchor: index}
		p.index = index
	}
}

func (m *Model) closeRecentNavigation() {
	m.recentNav.popup = nil
	m.invalidate()
}

func (m *Model) chooseRecentNavigation(side bool) {
	p := m.recentNav.popup
	if p == nil || p.index < 0 || p.index >= p.count() {
		return
	}
	m.closeRecentNavigation()
	switch {
	case p.locations:
		place := p.places[p.index]
		m.openRecentLocation(place, side)
	case p.onTools:
		m.DockShow(p.tools[p.index].id)
	case p.switcher:
		item := p.switcherItems[p.index]
		m.openRecentSwitcherItem(item, side)
	default:
		files := p.selectedRecentFiles()
		lead := p.files[p.index]
		if len(files) <= 1 {
			if len(files) == 1 {
				lead = files[0]
			}
			m.openRecentFile(lead, side)
			return
		}
		m.openRecentFiles(files, lead, side)
	}
}

func (m *Model) openRecentSwitcherItem(item recentSwitcherItem, side bool) {
	if item.tab == nil {
		m.openRecentFile(item.path, side)
		return
	}
	if !side {
		m.activateTab(item.tab)
		return
	}
	m.openTabInSide(item.tab)
}

// openTabInSide opens or focuses a copy in the opposite editor group without
// removing the tab from its current group.
func (m *Model) openTabInSide(tab *Tab) {
	if tab == nil {
		return
	}
	if m.split == nil {
		dup := m.duplicateTab(tab)
		if dup == nil {
			return
		}
		m.split = &splitGroup{tabs: []*Tab{dup}, active: 0}
		m.splitDown = false
		m.splitFirst = false
		m.focusGroup(groupSplit)
		return
	}
	for i, sideTab := range m.split.tabs {
		if sideTab.kind == tab.kind && samePath(tabPath(sideTab), tabPath(tab)) {
			m.split.active = i
			m.focusGroup(groupSplit)
			return
		}
	}
	dup := m.duplicateTab(tab)
	if dup == nil {
		return
	}
	m.split.tabs = append(m.split.tabs, dup)
	m.split.active = len(m.split.tabs) - 1
	m.focusGroup(groupSplit)
}

func (m *Model) openRecentFile(path string, side bool) {
	if !side {
		m.openFile(path)
		return
	}
	m.openWith(path, openSpec{
		at: -1,
		done: func(tab *Tab) {
			m.moveRecentTabToSide(tab, true)
		},
	})
}

func (m *Model) openRecentFiles(paths []string, lead string, side bool) {
	if len(paths) == 0 {
		return
	}
	var openNext func(int)
	openNext = func(index int) {
		if index >= len(paths) {
			m.focusRecentLead(lead, side)
			return
		}
		next := func(*Tab) { openNext(index + 1) }
		m.openWith(paths[index], openSpec{
			at: -1,
			done: func(tab *Tab) {
				if side {
					m.moveRecentTabToSide(tab, false)
				}
				next(tab)
			},
			failed: func(string, error) {
				next(nil)
			},
		})
	}
	openNext(0)
}

func (m *Model) focusRecentLead(path string, side bool) {
	if side && m.split != nil {
		for i, tab := range m.split.tabs {
			if samePath(tabPath(tab), path) {
				m.split.active = i
				m.focusGroup(groupSplit)
				return
			}
		}
	}
	for i, tab := range m.tabs {
		if samePath(tabPath(tab), path) {
			if side {
				m.moveRecentTabToSide(tab, true)
				return
			}
			m.activate(i)
			return
		}
	}
	if m.split != nil {
		for i, tab := range m.split.tabs {
			if samePath(tabPath(tab), path) {
				m.activateIn(groupSplit, i)
				return
			}
		}
	}
}

func (m *Model) openRecentLocation(place recentPlace, side bool) {
	if !side {
		m.revealAt(place.path, place.pos.Line, place.pos.Col)
		return
	}
	m.recordNavigation(true)
	m.reveal = &revealPos{
		path: place.path, line: place.pos.Line, col: place.pos.Col,
		endLine: -1, endCol: -1,
	}
	m.openWith(place.path, openSpec{
		at: -1,
		done: func(tab *Tab) {
			m.moveRecentTabToSide(tab, true)
		},
	})
}

// moveRecentTabToSide puts tab in the other editor group. Intermediate files
// in a multi-open stay there while the current group remains focused, so all
// selected files land in one side group before the lead receives focus.
func (m *Model) moveRecentTabToSide(tab *Tab, focus bool) {
	if tab == nil {
		return
	}
	group, ok := m.groupOf(tab)
	if !ok {
		return
	}
	if group == groupFocused {
		if len(m.tabs) <= 1 {
			if focus && m.split != nil {
				for i, sideTab := range m.split.tabs {
					if samePath(tabPath(sideTab), tabPath(tab)) {
						m.split.active = i
						m.focusGroup(groupSplit)
						return
					}
				}
			}
			dup := m.duplicateTab(tab)
			if dup == nil {
				return
			}
			if m.split == nil {
				m.split = &splitGroup{tabs: []*Tab{dup}}
				m.splitDown = false
				m.splitFirst = false
			} else {
				m.split.tabs = append(m.split.tabs, dup)
				m.split.active = len(m.split.tabs) - 1
			}
			if focus {
				m.focusGroup(groupSplit)
			} else {
				m.revealEditor()
				m.applyActive()
				m.saveSession()
				m.invalidate()
			}
			return
		}
		if m.split == nil {
			if tabIndexIn(m.tabs, tab) < 0 {
				return
			}
			m.removeFromGroup(groupFocused, tab)
			m.split = &splitGroup{tabs: []*Tab{tab}, active: 0}
			m.splitDown = false
			m.splitFirst = false
		} else {
			m.removeFromGroup(groupFocused, tab)
			m.split.tabs = append(m.split.tabs, tab)
			m.split.active = len(m.split.tabs) - 1
		}
	}
	if focus {
		m.focusGroup(groupSplit)
	} else {
		m.revealEditor()
		m.applyActive()
		m.saveSession()
		m.invalidate()
	}
}

func (m *Model) removeRecentPlace() {
	n := &m.recentNav
	p := n.popup
	if !p.locations || p.index < 0 || p.index >= len(p.places) {
		return
	}
	place := p.places[p.index]
	n.places = slices.DeleteFunc(n.places, func(old recentPlace) bool { return recentNearby(old, place) })
	m.refreshRecentNavigation(false)
}

// removeSelectedRecentNavigation is OpenIDEA's Delete in either recent popup.
// A tool window is hidden. A file is closed through the guarded tab close and
// then forgotten from the recent index; cancelling the unsaved-file prompt
// leaves both the tab and the row intact.
func (m *Model) removeSelectedRecentNavigation() {
	p := m.recentNav.popup
	if p == nil || p.locations || p.index < 0 {
		return
	}
	if p.onTools {
		if p.index < len(p.tools) {
			m.DockHide(p.tools[p.index].id)
		}
		return
	}
	key := ""
	var tab *Tab
	if p.switcher {
		if p.index >= len(p.switcherItems) {
			return
		}
		item := p.switcherItems[p.index]
		key, tab = item.path, item.tab
	} else {
		if p.index >= len(p.files) {
			return
		}
		key = p.files[p.index]
	}
	remove := func() {
		if p.switcher {
			if p.removed == nil {
				p.removed = make(map[string]bool)
			}
			p.removed[key] = true
		} else {
			m.forgetRecentFile(key)
		}
		m.refreshRecentNavigation(false)
	}
	if tab == nil {
		for _, candidate := range m.allTabs() {
			if samePath(tabPath(candidate), key) {
				tab = candidate
				break
			}
		}
	}
	if tab != nil {
		m.closeTabThen(tab, remove)
		return
	}
	remove()
}

func (m *Model) forgetRecentFile(path string) {
	n := &m.recentNav
	n.paths = slices.DeleteFunc(n.paths, func(old string) bool { return samePath(old, path) })
	delete(n.edited, path)
	if m.ctx == nil || m.mgr == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "remove_recent", map[string]any{"path": path})
	}()
}
