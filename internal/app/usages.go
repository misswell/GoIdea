package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	usageTimeout    = 90 * time.Second
	maxUsages       = 2000
	usageSummaryKey = "\x00summary"

	// referenceRetryDelay is how long an empty answer waits before asking
	// again, long enough for the server to index the edit it just received.
	referenceRetryDelay = 300 * time.Millisecond
)

// usagesSearch is one content tab in the Find Usages tool window.
type usagesSearch struct {
	symbol    string
	path      string
	offset    int
	libraries bool
	channel   uint32
	running   bool
	done      bool
	// locs is the newest snapshot; streaming partials are cumulative.
	locs             []openidea.Location
	classified       []openidea.ClassifiedReference
	incomplete       string
	librariesSkipped bool
	truncated        bool
	err              string
}

// UsagesPane is Find Usages: the active search plus every content tab the
// window keeps. The embedded search state preserves the existing status-bar
// and action gates while tabs retain their own results.
type UsagesPane struct {
	visible bool
	usagesSearch
	tabs      []usagesTab
	activeTab int
	nextTabID int
}

type usagesTab struct {
	id        int
	pinned    bool
	state     usagesSearch
	excluded  map[string]bool
	collapsed map[string]bool
	selected  string
	preview   usagePopupPreview
}

func (m *Model) resetUsages() {
	m.cancelUsages()
	m.usages = UsagesPane{}
	m.resetUsagePopup()
}

func (u *UsagesPane) tabIndex(id int) int {
	for i := range u.tabs {
		if u.tabs[i].id == id {
			return i
		}
	}
	return -1
}

func (u *UsagesPane) activeTabIndex() int {
	return u.tabIndex(u.activeTab)
}

func (u *UsagesPane) saveActive() {
	if i := u.activeTabIndex(); i >= 0 {
		u.tabs[i].state = u.usagesSearch
	}
}

func (u *UsagesPane) loadActive(id int) bool {
	i := u.tabIndex(id)
	if i < 0 {
		return false
	}
	u.activeTab = id
	u.usagesSearch = u.tabs[i].state
	return true
}

func (u *UsagesPane) newTab() int {
	u.saveActive()
	u.nextTabID++
	id := u.nextTabID
	u.tabs = append(u.tabs, usagesTab{
		id: id, excluded: map[string]bool{}, collapsed: map[string]bool{},
	})
	u.activeTab = id
	u.usagesSearch = usagesSearch{}
	return id
}

func (u *UsagesPane) tabForChannel(ch uint32) (int, *usagesTab) {
	if u.activeTabIndex() >= 0 && u.channel == ch {
		u.saveActive()
	}
	for i := range u.tabs {
		if u.tabs[i].state.channel == ch {
			return i, &u.tabs[i]
		}
	}
	return -1, nil
}

func (u *UsagesPane) syncActiveFromTab(i int) {
	if i < 0 || i >= len(u.tabs) || u.tabs[i].id != u.activeTab {
		return
	}
	u.usagesSearch = u.tabs[i].state
}

// SelectUsagesTab activates an existing content tab.
func (m *Model) SelectUsagesTab(id int) {
	if id == m.usages.activeTab {
		return
	}
	m.usages.saveActive()
	if !m.usages.loadActive(id) {
		return
	}
	m.ensureUsageSelection()
	m.invalidate()
}

// PinUsagesTab toggles a search tab's pin without changing its result.
func (m *Model) PinUsagesTab(id int, pinned bool) {
	i := m.usages.tabIndex(id)
	if i < 0 || m.usages.tabs[i].pinned == pinned {
		return
	}
	m.usages.tabs[i].pinned = pinned
	m.invalidate()
}

// CloseUsagesTab closes one result tab and selects its neighbour.
func (m *Model) CloseUsagesTab(id int) {
	i := m.usages.tabIndex(id)
	if i < 0 {
		return
	}
	m.usages.saveActive()
	tab := m.usages.tabs[i]
	if tab.state.running && tab.state.channel != 0 {
		m.cancelUsageChannel(tab.state.channel)
	}
	wasActive := id == m.usages.activeTab
	m.usages.tabs = append(m.usages.tabs[:i], m.usages.tabs[i+1:]...)
	if len(m.usages.tabs) == 0 {
		m.usages.activeTab = 0
		m.usages.usagesSearch = usagesSearch{}
		m.invalidate()
		return
	}
	if wasActive {
		next := min(i, len(m.usages.tabs)-1)
		m.usages.loadActive(m.usages.tabs[next].id)
	}
	m.invalidate()
}

// CloseOtherUsagesTabs keeps the requested result and discards the rest.
func (m *Model) CloseOtherUsagesTabs(id int) {
	i := m.usages.tabIndex(id)
	if i < 0 {
		return
	}
	m.usages.saveActive()
	keep := m.usages.tabs[i]
	for _, tab := range m.usages.tabs {
		if tab.id != id && tab.state.running && tab.state.channel != 0 {
			m.cancelUsageChannel(tab.state.channel)
		}
	}
	m.usages.tabs = []usagesTab{keep}
	m.usages.loadActive(id)
	m.invalidate()
}

// ToggleUsagesCollapsed collapses or expands one file group.
func (m *Model) ToggleUsagesCollapsed(path string) {
	i := m.usages.activeTabIndex()
	if i < 0 {
		return
	}
	if m.usages.tabs[i].collapsed == nil {
		m.usages.tabs[i].collapsed = map[string]bool{}
	}
	m.usages.tabs[i].collapsed[path] = !m.usages.tabs[i].collapsed[path]
	m.invalidate()
}

// SetAllUsagesCollapsed mirrors the Expand All and Collapse All toolbar
// controls. Only the summary level remains open when collapsing, as in
// OpenIDEA's tree.
func (m *Model) SetAllUsagesCollapsed(collapsed bool) {
	i := m.usages.activeTabIndex()
	if i < 0 {
		return
	}
	m.usages.tabs[i].collapsed = map[string]bool{}
	if collapsed {
		for _, group := range groupUsageRows(m.usageVisibleRows()) {
			m.usages.tabs[i].collapsed[group.path] = true
		}
	}
	m.invalidate()
}

// SearchUsagesInProjectAndLibraries reruns the active search with dependency
// and JDK references included.
func (m *Model) SearchUsagesInProjectAndLibraries() {
	m.rerunUsages(true)
}

func (m *Model) rerunUsages(libraries bool) {
	if m.usages.path == "" || m.usages.activeTabIndex() < 0 {
		return
	}
	for _, t := range m.allTabs() {
		if t.doc != nil && t.doc.Path() == m.usages.path {
			off := min(max(m.usages.offset, 0), len(t.doc.Text()))
			m.findUsagesAtTab(t, off, m.usages.activeTab, libraries)
			return
		}
	}
	m.status = "The file for this Find Usages result is no longer open"
	m.invalidate()
}

// showUsages opens the Find Usages window in the bottom side.
func (m *Model) showUsages() { m.DockShow("usages") }

// showUsageResults adds an already-finished result as its own content tab.
// Safe Delete and the Show Usages popup use this instead of replacing every
// search the window already holds.
func (m *Model) showUsageResults(
	symbol string,
	locs []openidea.Location,
	classified []openidea.ClassifiedReference,
	note string,
) {
	m.usages.newTab()
	m.usages.usagesSearch = usagesSearch{
		symbol: symbol, done: true, locs: locs, classified: classified,
		incomplete: note, truncated: len(locs) >= maxUsages,
	}
	m.usages.saveActive()
	m.ensureUsageSelection()
	m.showUsages()
	m.status = fmt.Sprintf("Found %d usages", len(locs))
	m.invalidate()
}

// FindUsages looks up every use of the symbol at the caret and shows the
// result in OpenIDEA's Find Usages window, which sits in the bottom side. The
// backend streams partial answers, so the list fills while JDT is still
// searching.
func (m *Model) FindUsages() {
	t := m.tab()
	if t == nil || !m.tabOpen(t) {
		return
	}
	m.findUsagesAt(t, t.state.Cursor())
}

// RerunUsages repeats the last Find Usages request from the file and caret
// that produced it, as the Find Usages tool window's rerun control does.
func (m *Model) RerunUsages() {
	if !m.toolVisible("usages") || m.usages.path == "" {
		return
	}
	path, offset := m.usages.path, m.usages.offset
	if m.usages.activeTabIndex() < 0 {
		m.usages.newTab()
	}
	tabID := m.usages.activeTab
	libraries := m.usages.libraries
	a := m.allTabs()
	for _, t := range a {
		if t.doc == nil || t.doc.Path() != path {
			continue
		}
		off := min(max(offset, 0), len(t.doc.Text()))
		m.findUsagesAtTab(t, off, tabID, libraries)
		return
	}
	m.status = "The file for this Find Usages result is no longer open"
	m.invalidate()
}

// findUsagesAt starts a Find Usages request at a byte offset in t.
func (m *Model) findUsagesAt(t *Tab, off int) {
	m.findUsagesAtTab(t, off, m.usages.newTab(), false)
}

// findUsagesAtTab starts or refreshes one content tab. Reusing the tab id on
// rerun keeps its pin, exclusions and collapse state.
func (m *Model) findUsagesAtTab(t *Tab, off, tabID int, libraries bool) {
	if t == nil || t.doc == nil || !m.tabOpen(t) {
		return
	}
	off = min(max(off, 0), len(t.doc.Text()))
	symbol := wordAt(t.doc.Text(), off)
	m.cancelUsages()
	ch := m.nextChannel()
	m.usages.activeTab = tabID
	m.usages.usagesSearch = usagesSearch{
		symbol: symbol, path: t.doc.Path(), offset: off,
		libraries: libraries, channel: ch, running: true,
	}
	m.usages.saveActive()
	m.showUsages()
	m.status = "Finding usages…"
	m.invalidate()
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(off)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, usageTimeout)
		defer cancel()
		// The backend searches the text it was last given; typing since the
		// debounce sync would make the caret point at other text.
		if serr := t.doc.Sync(ctx); serr != nil {
			m.post(func() { m.finishUsages(ch, serr, nil) })
			return
		}
		ask := func() (openidea.References, error) {
			raw, err := m.invoke(ctx, "lsp_references", map[string]any{
				"request": ch, "doc": docID, "pos": lspPos(pos),
				"includeDeclaration": true, "projectWide": true, "libraries": libraries,
				"onPartial": map[string]any{"$channel": ch},
			})
			var refs openidea.References
			if err == nil {
				err = json.Unmarshal(raw, &refs)
			}
			return refs, err
		}
		refs, err := ask()
		// JDT applies the didChange just pushed in the background; its search
		// index catches up a moment later, so an empty answer right after an
		// edit can be premature.
		if err == nil && len(refs.Locations) == 0 {
			select {
			case <-ctx.Done():
			case <-time.After(referenceRetryDelay):
				refs, err = ask()
			}
		}
		m.post(func() { m.finishUsages(ch, err, &refs) })
	}()
}

// cancelUsages stops the running lookup; its final answer is then dropped.
func (m *Model) cancelUsages() {
	if !m.usages.running || m.usages.channel == 0 {
		return
	}
	id := m.usages.channel
	m.usages.running = false
	if i := m.usages.activeTabIndex(); i >= 0 {
		m.usages.tabs[i].state.running = false
	}
	m.cancelUsageChannel(id)
}

// acceptUsages consumes one streamed snapshot of the lookup.
func (m *Model) acceptUsages(ch uint32, data json.RawMessage) {
	tabIndex, tab := m.usages.tabForChannel(ch)
	if tab == nil {
		return
	}
	var refs openidea.References
	if json.Unmarshal(data, &refs) != nil {
		return
	}
	setUsagesState(&tab.state, &refs)
	m.usages.syncActiveFromTab(tabIndex)
	m.ensureUsageSelection()
	m.invalidate()
}

func (m *Model) finishUsages(ch uint32, err error, refs *openidea.References) {
	tabIndex, tab := m.usages.tabForChannel(ch)
	if tab == nil {
		return
	}
	tab.state.running = false
	tab.state.done = true
	switch {
	case err != nil:
		if openidea.ErrorKind(err) == "cancelled" {
			if tab.id == m.usages.activeTab {
				m.status = "Stopped finding usages"
			}
			break
		}
		tab.state.err = "Finding usages failed: " + err.Error()
		if tab.id == m.usages.activeTab {
			m.status = "Finding usages failed"
		}
	case refs != nil:
		setUsagesState(&tab.state, refs)
		switch {
		case len(tab.state.locs) == 0:
			if tab.id == m.usages.activeTab {
				m.status = "No usages found"
			}
		default:
			if tab.id == m.usages.activeTab {
				m.status = fmt.Sprintf("Found %d usages", len(tab.state.locs))
			}
		}
	}
	m.usages.syncActiveFromTab(tabIndex)
	m.ensureUsageSelection()
	m.invalidate()
}

// setUsagesState replaces one tab's list with a snapshot, keeping it bounded.
func setUsagesState(state *usagesSearch, refs *openidea.References) {
	locs := refs.Locations
	state.truncated = len(locs) > maxUsages
	if state.truncated {
		locs = locs[:maxUsages]
	}
	state.locs = locs
	state.classified = refs.Classified
	state.incomplete = ""
	if refs.Incomplete != nil {
		state.incomplete = incompleteText(*refs.Incomplete)
	}
	state.librariesSkipped = refs.LibrariesSkipped
}

// incompleteText names what a reference query may have missed.
func incompleteText(kind string) string {
	switch kind {
	case "indexing":
		return "The language server is still indexing; results may be incomplete"
	case "tooManyFiles":
		return "Too many files mention the name; only some were checked"
	case "filesSkipped":
		return "Some files were too large or unreadable and were not checked"
	case "serverFailed":
		return "The server query failed; results may be incomplete"
	}
	if kind == "" {
		return ""
	}
	return "Results may be incomplete (" + kind + ")"
}

// wordAt expands the identifier around off, the name the usages pane titles
// itself with; an offset on punctuation leaves it empty.
func wordAt(text string, off int) string {
	if off < 0 {
		off = 0
	}
	if off > len(text) {
		off = len(text)
	}
	isWord := func(b byte) bool {
		return b == '_' || b == '$' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
			b >= 0x80
	}
	start, end := off, off
	for start > 0 && isWord(text[start-1]) {
		start--
	}
	for end < len(text) && isWord(text[end]) {
		end++
	}
	return text[start:end]
}

// usageFile groups the locations of one file, in the order the backend listed
// them.
type usageFile struct {
	path string
	locs []openidea.Location
}

type usageFileRow struct {
	path string
	rows []usagePopupRow
}

func groupUsages(locs []openidea.Location) []usageFile {
	var files []usageFile
	index := map[string]int{}
	for _, loc := range locs {
		i, ok := index[loc.Path]
		if !ok {
			i = len(files)
			index[loc.Path] = i
			files = append(files, usageFile{path: loc.Path})
		}
		files[i].locs = append(files[i].locs, loc)
	}
	return files
}

func groupUsageRows(rows []usagePopupRow) []usageFileRow {
	var files []usageFileRow
	index := map[string]int{}
	for _, row := range rows {
		path := row.source.Path
		i, ok := index[path]
		if !ok {
			i = len(files)
			index[path] = i
			files = append(files, usageFileRow{path: path})
		}
		files[i].rows = append(files[i].rows, row)
	}
	return files
}

func (m *Model) usageVisibleRows() []usagePopupRow {
	rows := buildUsagePopupRows(m.usages.locs, m.usages.classified, nil)
	out := make([]usagePopupRow, 0, len(rows))
	for _, row := range rows {
		if !usageAccessVisible(row.access, m.prefs.Usages) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (m *Model) usageRowExcluded(row usagePopupRow) bool {
	i := m.usages.activeTabIndex()
	return i >= 0 && m.usages.tabs[i].excluded[usageLocationKey(row.source)]
}

func (m *Model) usageFileExcluded(rows []usagePopupRow) bool {
	if len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		if !m.usageRowExcluded(row) {
			return false
		}
	}
	return true
}

func (m *Model) ensureUsageSelection() {
	i := m.usages.activeTabIndex()
	if i < 0 {
		return
	}
	rows := m.usageVisibleRows()
	if len(rows) == 0 {
		m.usages.tabs[i].selected = ""
		m.usages.tabs[i].preview = usagePopupPreview{}
		return
	}
	selected := m.usages.tabs[i].selected
	if selected != "" {
		for _, row := range rows {
			if usageLocationKey(row.source) == selected {
				if m.prefs.Usages.ShowFindPreview() {
					m.loadUsageRowPreview(i, row)
				}
				return
			}
		}
	}
	first := rows[0]
	for _, row := range rows {
		if !m.usageRowExcluded(row) {
			first = row
			break
		}
	}
	m.usages.tabs[i].selected = usageLocationKey(first.source)
	if m.prefs.Usages.ShowFindPreview() {
		m.loadUsageRowPreview(i, first)
	}
}

// StepUsages walks the included usage rows in display order. Excluded rows
// stay visible for Include, but occurrence navigation skips them as IDEA does.
func (m *Model) StepUsages(dir int) {
	rows := m.usageVisibleRows()
	if len(rows) == 0 {
		return
	}
	i := m.usages.activeTabIndex()
	if i < 0 {
		return
	}
	selected := m.usages.tabs[i].selected
	start := -1
	for index, row := range rows {
		if usageLocationKey(row.source) == selected {
			start = index
			break
		}
	}
	if dir < 0 && start < 0 {
		start = 0
	}
	for offset := 1; offset <= len(rows); offset++ {
		index := (start + dir*offset + len(rows)) % len(rows)
		row := rows[index]
		if m.usageRowExcluded(row) {
			continue
		}
		m.selectUsageRow(row)
		m.markUsageOpened(row.source)
		return
	}
}

func (m *Model) excludeSelectedUsage() {
	row, ok := m.selectedUsageRow()
	if !ok {
		return
	}
	m.toggleUsageExcluded(row)
	if !m.usageRowExcluded(row) {
		return
	}
	m.StepUsages(1)
}

func (m *Model) handleUsagesInput(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	switch {
	case ev.Mods == 0 && (ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete):
		if _, ok := m.selectedUsageRow(); !ok {
			return false
		}
		m.excludeSelectedUsage()
	case ev.Mods == 0 && ev.Key == ui.KeyF4:
		row, ok := m.selectedUsageRow()
		if !ok {
			return false
		}
		m.markUsageOpened(row.source)
	default:
		return false
	}
	return true
}

func (m *Model) selectUsageRow(row usagePopupRow) {
	i := m.usages.activeTabIndex()
	if i < 0 {
		return
	}
	m.usages.tabs[i].selected = usageLocationKey(row.source)
	if m.prefs.Usages.ShowFindPreview() {
		m.loadUsageRowPreview(i, row)
	}
	m.invalidate()
}

func (m *Model) loadUsageRowPreview(tabIndex int, row usagePopupRow) {
	if tabIndex < 0 || tabIndex >= len(m.usages.tabs) {
		return
	}
	start, lines, err := readUsagePreview(
		row.source.Path, int(row.source.Range.Start.Line), 4, 8,
	)
	preview := usagePopupPreview{
		key: usageLocationKey(row.source), path: row.source.Path,
		line: int(row.source.Range.Start.Line), col: row.source.Range.Start.Col,
		endCol: row.endCol, start: start, lines: lines,
	}
	if err != nil {
		preview.err = err.Error()
	}
	m.usages.tabs[tabIndex].preview = preview
}

func (m *Model) toggleUsageExcluded(row usagePopupRow) {
	i := m.usages.activeTabIndex()
	if i < 0 {
		return
	}
	if m.usages.tabs[i].excluded == nil {
		m.usages.tabs[i].excluded = map[string]bool{}
	}
	key := usageLocationKey(row.source)
	m.usages.tabs[i].excluded[key] = !m.usages.tabs[i].excluded[key]
	m.invalidate()
}

func (m *Model) toggleUsageFileExcluded(path string) {
	i := m.usages.activeTabIndex()
	if i < 0 {
		return
	}
	if m.usages.tabs[i].excluded == nil {
		m.usages.tabs[i].excluded = map[string]bool{}
	}
	var rows []usagePopupRow
	for _, row := range buildUsagePopupRows(m.usages.locs, m.usages.classified, nil) {
		if row.source.Path == path {
			rows = append(rows, row)
		}
	}
	allExcluded := len(rows) > 0
	for _, row := range rows {
		if !m.usages.tabs[i].excluded[usageLocationKey(row.source)] {
			allExcluded = false
			break
		}
	}
	for _, row := range rows {
		if allExcluded {
			delete(m.usages.tabs[i].excluded, usageLocationKey(row.source))
		} else {
			m.usages.tabs[i].excluded[usageLocationKey(row.source)] = true
		}
	}
	m.invalidate()
}

func (m *Model) toggleUsageFindPreview() {
	m.prefs.Usages.NoFindPreview = !m.prefs.Usages.NoFindPreview
	m.savePreferences()
	if m.prefs.Usages.ShowFindPreview() {
		if row, ok := m.selectedUsageRow(); ok && m.usages.activeTabIndex() >= 0 {
			m.loadUsageRowPreview(m.usages.activeTabIndex(), row)
		}
	}
	m.invalidate()
}

func (m *Model) selectedUsageRow() (usagePopupRow, bool) {
	i := m.usages.activeTabIndex()
	if i < 0 || m.usages.tabs[i].selected == "" {
		return usagePopupRow{}, false
	}
	for _, row := range m.usageVisibleRows() {
		if usageLocationKey(row.source) == m.usages.tabs[i].selected {
			return row, true
		}
	}
	return usagePopupRow{}, false
}

// usagesRows draws OpenIDEA's Find Usages window: one content tab per search,
// toolbar, access filters, file groups and an optional source preview.
func (m *Model) usagesRows(c *ui.Context, p *ui.Theme) {
	m.usagesTabStrip(c, p)
	m.usagesToolbar(c, p)
	if m.usages.err != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, m.usages.err).FontSize(12).TextColor(p.Warning)
		})
	}
	if m.usages.librariesSkipped && !m.usages.libraries {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, "Dependencies and the JDK were not searched; project sources only").FontSize(12).TextColor(p.TextMuted)
		})
	}
	if m.usages.truncated {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, fmt.Sprintf("Too many usages; showing the first %d", maxUsages)).FontSize(12).TextColor(p.TextMuted)
		})
	}
	rows := m.usageVisibleRows()
	if m.usages.done && !m.usages.libraries && len(rows) == 0 && m.usages.err == "" {
		ui.Row(c).Padding(4, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Usages in Project Files").FontSize(12).TextColor(p.TextMuted)
			ui.Button(c, "Search in Project and Libraries").OnClick(m.SearchUsagesInProjectAndLibraries)
		})
	}
	if usageSupportsAccess(buildUsagePopupRows(m.usages.locs, m.usages.classified, nil)) {
		m.usagesAccessFilters(c, p)
	}
	if len(rows) == 0 {
		hint := "Put the caret on a symbol, then find its usages"
		switch {
		case m.usages.running:
			hint = "The language server is searching…"
		case m.usages.done && len(m.usages.locs) > 0:
			hint = "No usages match the access filters"
		case m.usages.done:
			hint = "No usages found"
		}
		if m.usages.err != "" {
			return
		}
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	if m.prefs.Usages.ShowFindPreview() {
		ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				m.usagesList(c, p, rows)
			})
			ui.Column(c).Width(1).Shrink(0).Background(p.Border)
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				m.usagesSourcePreview(c, p)
			})
		})
		return
	}
	m.usagesList(c, p, rows)
}

func usageTabLabel(tab usagesTab) string {
	if tab.state.symbol == "" {
		return "Usages"
	}
	if tab.state.libraries {
		return fmt.Sprintf("Usages of '%s' in Project and Libraries", tab.state.symbol)
	}
	if tab.state.path != "" {
		return fmt.Sprintf("Usages of '%s' in Project Files", tab.state.symbol)
	}
	return "Usages of '" + tab.state.symbol + "'"
}

func (m *Model) usagesTabStrip(c *ui.Context, p *ui.Theme) {
	if len(m.usages.tabs) == 0 {
		return
	}
	oi := oiPaletteOf(p)
	ui.Row(c).Height(27).Shrink(0).Padding(0, 4).Gap(2).AlignItems(ui.Stretch).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		for _, tab := range m.usages.tabs {
			tab := tab
			active := tab.id == m.usages.activeTab
			b := ui.ButtonBase(c.Key(fmt.Sprintf("usages-tab:%d", tab.id))).
				Height(27).MaxWidth(300).Padding(0, 8).Radius(4).AlignItems(ui.Center)
			if active {
				b.Background(oi.selected)
			} else if b.Hovered() {
				b.Background(oi.hoverTint)
			}
			b.Children(func() {
				if tab.pinned {
					ui.Text(c, "●").FontSize(9).TextColor(p.Accent)
				}
				ui.Text(c, usageTabLabel(tab)).FontSize(12).
					TextColor(oi.textStrong).SingleLine().Ellipsis("…")
				if ui.Button(c, "×").Clicked() {
					m.CloseUsagesTab(tab.id)
				}
			})
			if b.Clicked() {
				m.SelectUsagesTab(tab.id)
			}
			b.Menu(func(menu *ui.Menu) {
				pinLabel := "Pin Tab"
				if tab.pinned {
					pinLabel = "Unpin Tab"
				}
				if menu.Item(pinLabel).Chosen() {
					m.PinUsagesTab(tab.id, !tab.pinned)
				}
				menu.Separator()
				if menu.Item("Close Tab").Chosen() {
					m.CloseUsagesTab(tab.id)
				}
				if menu.Item("Close Other Tabs").Disabled(len(m.usages.tabs) < 2).Chosen() {
					m.CloseOtherUsagesTabs(tab.id)
				}
			})
		}
	})
}

func (m *Model) usagesToolbar(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(28).Shrink(0).Padding(0, 7).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		rerun := ui.Button(c, "Rerun").Disabled(m.usages.running || m.usages.path == "")
		rerun.OnClick(m.RerunUsages)
		rerun.Tooltip("Rerun Find Usages")
		previous := ui.Button(c, "↑").Disabled(len(m.usageVisibleRows()) == 0)
		previous.OnClick(func() { m.StepUsages(-1) })
		previous.Tooltip("Previous Usage")
		next := ui.Button(c, "↓").Disabled(len(m.usageVisibleRows()) == 0)
		next.OnClick(func() { m.StepUsages(1) })
		next.Tooltip("Next Usage")
		ui.Button(c, "Expand All").Disabled(len(m.usages.locs) == 0).
			OnClick(func() { m.SetAllUsagesCollapsed(false) })
		ui.Button(c, "Collapse All").Disabled(len(m.usages.locs) == 0).
			OnClick(func() { m.SetAllUsagesCollapsed(true) })
		ui.Button(c, "Options…").Menu(func(menu *ui.Menu) {
			if menu.Item("Preview Source").Checked(m.prefs.Usages.ShowFindPreview()).Chosen() {
				m.toggleUsageFindPreview()
			}
		})
		ui.Text(c, fmt.Sprintf("%d found", len(m.usageVisibleRows()))).FontSize(12).
			TextColor(oi.textMuted).SingleLine()
		ui.Spacer(c)
		status := count(len(m.usages.locs), "usage", "usages")
		if m.usages.running {
			status = "Searching…"
		}
		if m.usages.done && len(m.usages.locs) == 0 && m.usages.err == "" {
			status = "Not found"
		}
		ui.Text(c, status).FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
}

func (m *Model) usagesAccessFilters(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(24).Shrink(0).Padding(0, 8).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Access").FontSize(11).TextColor(oi.textMuted)
		m.usagePopupAccessButton(c, oi, "Read", m.prefs.Usages.ShowReadAccess(), func() {
			m.toggleUsageAccess("read")
		})
		m.usagePopupAccessButton(c, oi, "Write", m.prefs.Usages.ShowWriteAccess(), func() {
			m.toggleUsageAccess("write")
		})
	})
}

func (m *Model) usagesList(c *ui.Context, p *ui.Theme, rows []usagePopupRow) {
	root := m.Root()
	oi := oiPaletteOf(p)
	allRows := buildUsagePopupRows(m.usages.locs, m.usages.classified, nil)
	accessFiltered := usageSupportsAccess(allRows) &&
		(!m.prefs.Usages.ShowReadAccess() || !m.prefs.Usages.ShowWriteAccess())
	tabIndex := m.usages.activeTabIndex()
	var collapsed map[string]bool
	var selected string
	if tabIndex >= 0 {
		collapsed = m.usages.tabs[tabIndex].collapsed
		selected = m.usages.tabs[tabIndex].selected
	}
	ui.Scroll(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		ui.Column(c).Padding(4, 0).Children(func() {
			summary := ui.Row(c.Key("usage-summary")).Height(24).
				PaddingStart(4).PaddingEnd(8).Gap(6).Radius(5).AlignItems(ui.Center)
			if summary.Hovered() {
				summary.Background(oi.hoverTint)
			}
			summary.Children(func() {
				caret := "▾"
				if collapsed[usageSummaryKey] {
					caret = "▸"
				}
				if ui.Button(c, caret).Clicked() {
					if tabIndex >= 0 {
						m.usages.tabs[tabIndex].collapsed[usageSummaryKey] =
							!m.usages.tabs[tabIndex].collapsed[usageSummaryKey]
					}
					m.invalidate()
				}
				ui.Text(c, "Found usages").FontSize(13).Bold().TextColor(oi.textStrong).
					SingleLine()
				ui.Spacer(c)
				resultText := count(len(rows), "result", "results")
				if accessFiltered {
					resultText = fmt.Sprintf("%d / %d results", len(rows), len(m.usages.locs))
				}
				ui.Text(c, resultText).FontSize(12).
					TextColor(oi.textMuted).SingleLine()
				if m.usages.incomplete != "" {
					ui.Text(c, m.usages.incomplete).FontSize(11).TextColor(oi.warning).
						SingleLine().Ellipsis("…")
				}
			})
			if collapsed[usageSummaryKey] {
				return
			}
			for fi, f := range groupUsageRows(rows) {
				fi, f := fi, f
				rel := f.path
				if root != "" {
					if r, err := filepath.Rel(root, f.path); err == nil && !strings.HasPrefix(r, "..") {
						rel = r
					}
				}
				dir, base := filePathParts(rel)
				isCollapsed := collapsed[f.path]
				excludedFile := m.usageFileExcluded(f.rows)
				head := ui.Row(c.Key(fmt.Sprintf("usage-file:%d", fi))).Height(24).
					PaddingStart(4).PaddingEnd(8).Gap(6).Radius(5).AlignItems(ui.Center)
				if head.Hovered() {
					head.Background(oi.hoverTint)
				}
				head.Children(func() {
					caret := "▾"
					if isCollapsed {
						caret = "▸"
					}
					if ui.Button(c, caret).Clicked() {
						m.ToggleUsagesCollapsed(f.path)
					}
					fileIconEl(c, oi, f.path, 16, oi.textMuted)
					name := ui.Text(c, base).FontSize(13).TextColor(oi.textStrong).
						SingleLine().Ellipsis("…")
					if excludedFile {
						name.TextColor(oi.textSubtle).Strikethrough()
					}
					if dir != "." && dir != "" {
						ui.Text(c, dir).FontSize(13).TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
					}
					ui.Spacer(c)
					ui.Text(c, count(len(f.rows), "usage", "usages")).FontSize(13).
						TextColor(oi.textSubtle)
				})
				if head.Clicked() {
					m.selectUsageRow(f.rows[0])
				}
				if head.DoubleClicked() {
					m.markUsageOpened(f.rows[0].source)
				}
				head.Menu(func(menu *ui.Menu) {
					label := "Exclude"
					if excludedFile {
						label = "Include"
					}
					if menu.Item(label).Chosen() {
						m.toggleUsageFileExcluded(f.path)
					}
					if menu.Item("Open First Usage").Chosen() {
						row := f.rows[0].source
						m.markUsageOpened(row)
					}
				})
				if isCollapsed {
					continue
				}
				for li, row := range f.rows {
					li, row := li, row
					key := usageLocationKey(row.source)
					excluded := m.usageRowExcluded(row)
					r := ui.Row(c.Key(fmt.Sprintf("usage:%d:%d", fi, li))).Height(24).
						PaddingStart(24).PaddingEnd(8).Gap(6).AlignItems(ui.Center)
					if key == selected {
						r.Background(oi.selected)
					} else if r.Hovered() {
						r.Background(oi.hoverTint)
					}
					r.Children(func() {
						ui.Text(c, fmt.Sprintf("%d", row.source.Range.Start.Line+1)).Width(34).
							Font("monospace").FontSize(11).TextColor(oi.textSubtle).SingleLine()
						if row.text != "" {
							text, from, to := usagePreviewText(row)
							spans := usageHighlightedSpans(text, from, to, oi)
							if excluded {
								for i := range spans {
									spans[i].Color = oi.textSubtle
									spans[i].Weight = 0
									spans[i].Background = ui.Transparent
									spans[i].Strikethrough = true
								}
							}
							ui.RichText(c, spans...).
								Font("monospace").FontSize(11).SingleLine().Grow(1).MinWidth(0)
						} else if strings.Contains(row.source.Path, "://") {
							ui.Text(c, "External dependency").FontSize(12).TextColor(oi.textMuted).SingleLine()
						} else {
							ui.Text(c, fmt.Sprintf("%d:%d", row.source.Range.Start.Line+1, row.source.Range.Start.Col+1)).
								Font("monospace").FontSize(11).TextColor(oi.text).SingleLine()
						}
					})
					if r.Clicked() {
						m.selectUsageRow(row)
					}
					if r.DoubleClicked() {
						m.markUsageOpened(row.source)
					}
					r.Menu(func(menu *ui.Menu) {
						label := "Exclude"
						if excluded {
							label = "Include"
						}
						if menu.Item(label).Chosen() {
							m.toggleUsageExcluded(row)
						}
						if menu.Item("Jump to Source").Chosen() {
							m.markUsageOpened(row.source)
						}
					})
				}
			}
		})
	})
}

func (m *Model) markUsageOpened(loc openidea.Location) {
	if strings.Contains(loc.Path, "://") {
		m.status = "The usage is in an external dependency and cannot open as a file"
		m.invalidate()
		return
	}
	m.revealAt(loc.Path, int(loc.Range.Start.Line), int(loc.Range.Start.Col))
}

func usageLineHighlightedSpans(
	text string,
	rows []usagePopupRow,
	oi oiColors,
	excluded bool,
) []ui.Span {
	type spanRange struct{ from, to int }
	var ranges []spanRange
	trimmed := strings.TrimSpace(text)
	for _, row := range rows {
		line, from, to := usagePreviewText(row)
		if line == "" {
			continue
		}
		if trimmed == "" {
			trimmed = line
		}
		from = min(max(from, 0), len(trimmed))
		to = min(max(to, from), len(trimmed))
		if to <= from {
			continue
		}
		ranges = append(ranges, spanRange{from: from, to: to})
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].from != ranges[j].from {
			return ranges[i].from < ranges[j].from
		}
		return ranges[i].to < ranges[j].to
	})
	out := make([]ui.Span, 0, len(ranges)*2+1)
	cursor := 0
	for _, span := range ranges {
		from, to := max(cursor, span.from), max(cursor, span.to)
		if from >= to {
			continue
		}
		if from > cursor {
			out = append(out, ui.Span{Text: trimmed[cursor:from], Color: oi.text})
		}
		out = append(out, ui.Span{
			Text: trimmed[from:to], Color: oi.textStrong, Weight: 600,
			Background: oi.selected, Radius: 3,
		})
		cursor = to
	}
	if cursor < len(trimmed) {
		out = append(out, ui.Span{Text: trimmed[cursor:], Color: oi.text})
	}
	if len(out) == 0 {
		out = append(out, ui.Span{Text: trimmed, Color: oi.text})
	}
	if excluded {
		for i := range out {
			out[i].Color = oi.textSubtle
			out[i].Weight = 0
			out[i].Background = ui.Transparent
			out[i].Strikethrough = true
		}
	}
	return out
}

func (m *Model) usagesSourcePreview(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	row, hasRow := m.selectedUsageRow()
	var preview usagePopupPreview
	if i := m.usages.activeTabIndex(); i >= 0 {
		preview = m.usages.tabs[i].preview
		if hasRow && preview.key != usageLocationKey(row.source) {
			if len(preview.lines) == 0 {
				m.loadUsageRowPreview(i, row)
				preview = m.usages.tabs[i].preview
			}
		}
	}
	ui.Row(c).Height(26).Shrink(0).Padding(0, 8).AlignItems(ui.Center).
		Background(oi.elev).Children(func() {
		ui.Text(c, "Preview").FontSize(10).Bold().TextColor(oi.textMuted).SingleLine()
		ui.Spacer(c)
		if preview.path != "" {
			ui.Text(c, usageDisplayBase(preview.path)).FontSize(10).
				TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
	})
	switch {
	case !hasRow:
		ui.Row(c).Padding(8, 10).Children(func() {
			ui.Text(c, "Choose a usage to preview its source").FontSize(11).TextColor(oi.textMuted)
		})
	case preview.err != "":
		ui.Row(c).Padding(8, 10).Children(func() {
			ui.Text(c, preview.err).FontSize(11).TextColor(oi.warning).SingleLine().Ellipsis("…")
		})
	case len(preview.lines) > 0:
		ui.ScrollBoth(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(5, 0).Children(func() {
				for i, line := range preview.lines {
					lineNo := preview.start + i
					r := ui.Row(c.Key(fmt.Sprintf("usage-preview:%d", lineNo))).
						Height(18).Padding(0, 5).Gap(8).AlignItems(ui.Center)
					if lineNo == preview.line {
						r.Background(oi.hoverTint)
					}
					r.Children(func() {
						ui.Text(c, fmt.Sprintf("%d", lineNo+1)).Width(42).
							Font("monospace").FontSize(10).TextColor(oi.textSubtle)
						if lineNo == preview.line {
							var lineRows []usagePopupRow
							for _, candidate := range buildUsagePopupRows(
								m.usages.locs, m.usages.classified, nil,
							) {
								if candidate.source.Path == preview.path &&
									int(candidate.source.Range.Start.Line) == lineNo {
									lineRows = append(lineRows, candidate)
								}
							}
							ui.RichText(c, usageLineHighlightedSpans(
								line, lineRows, oi, hasRow && m.usageRowExcluded(row),
							)...).
								Font("monospace").FontSize(11).SingleLine()
						} else {
							ui.Text(c, line).Font("monospace").FontSize(11).
								TextColor(oi.textMuted).SingleLine()
						}
					})
				}
			})
		})
	default:
		ui.Row(c).Padding(8, 10).Children(func() {
			ui.Text(c, "Loading preview…").FontSize(11).TextColor(oi.textMuted)
		})
	}
}
