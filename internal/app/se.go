package app

// Search Everywhere after OpenIDEA's search feature: one overlay whose tabs
// narrow the results to classes, files, symbols, actions or text, with an All
// tab merging them under section headers. An empty query lists recent files
// and recent actions, as upstream's sectionsFor does.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type seTab string

const (
	seTabAll     seTab = "all"
	seTabClasses seTab = "classes"
	seTabFiles   seTab = "files"
	seTabSymbols seTab = "symbols"
	seTabActions seTab = "actions"
	seTabText    seTab = "text"
)

type seSection string

const (
	seHistory       seSection = "history"
	seRecentFiles   seSection = "recent"
	seRecentActions seSection = "recentActions"
	seClasses       seSection = "classes"
	seFiles         seSection = "files"
	seSymbols       seSection = "symbols"
	seEndpoints     seSection = "endpoints"
	seActions       seSection = "actions"
	seText          seSection = "text"
)

// seTabs is OpenIDEA's tab row, in its order.
var seTabs = []struct {
	id    seTab
	label string
}{
	{seTabAll, "All"},
	{seTabClasses, "Classes"},
	{seTabFiles, "Files"},
	{seTabSymbols, "Symbols"},
	{seTabActions, "Actions"},
	{seTabText, "Text"},
}

var seSectionTitles = map[seSection]string{
	seHistory:       "Search History",
	seRecentFiles:   "Recent Files",
	seRecentActions: "Recent Actions",
	seClasses:       "Classes",
	seFiles:         "Files",
	seSymbols:       "Symbols",
	seEndpoints:     "Endpoints",
	seActions:       "Actions",
	seText:          "Text",
}

const (
	// seAllLimit is the rows per section the All tab shows, as ALL_TAB_LIMIT.
	seAllLimit = 6
	// seTabLimit is the rows a single tab shows, as TAB_LIMIT.
	seTabLimit = 50
	// seRecentLimit bounds the empty-query lists, as RECENT_LIMIT.
	seRecentLimit = 15
	// seMoreStep* are how many rows a More row adds, as MORE_STEP_*.
	seMoreStepAll = 15
	seMoreStepTab = 50

	seFileQueryLimit   = 100
	seSymbolQueryLimit = 100
	seTextFileLimit    = 40
	seTextMatchLimit   = 200
	seRecentActionCap  = 15
	// seHistoryCap bounds the remembered Search Everywhere queries.
	seHistoryCap = 10
	seDebounce   = 120 * time.Millisecond
)

// seItem is one result row before it is drawn.
type seItem struct {
	kind    string // "file", "symbol", "endpoint", "text", "action", "setting", "history" or "command"
	title   string
	detail  string
	path    string
	line    int
	col     int
	action  string
	setting string
	command seTab
	// disabled keeps a matching action visible while making it inert, as
	// OpenIDEA does for unavailable actions.
	disabled bool
	// indices are rune offsets into title, matched by the query for emphasis.
	indices []int
	// symbol is the workspace symbol's kind, which picks its node icon.
	symbol string
}

// seRow is a section header or one item in the flattened list.
type seRow struct {
	section seSection
	header  bool
	more    bool
	item    seItem
}

// sePane is the overlay's state.
type sePane struct {
	open  bool
	query string
	tab   seTab
	sel   int
	// selKey follows the chosen result while streamed sections reorder it.
	selKey string
	// extra widens a section past its first page, as the More rows do.
	extra map[seSection]int
	// location is the parsed `File.kt:42` part of the current query.
	location seLocationQuery

	files     []openidea.FileMatch
	symbols   []openidea.WorkspaceSymbol
	endpoints []openidea.PluginEndpoint
	text      []openidea.FileMatches
	textCount int

	fileDone, symbolDone, endpointDone, textDone bool
	textBusy                                     bool
	textTruncated                                bool
	err                                          string

	plugins      []openidea.PluginInfo
	pluginsBusy  bool
	pluginsDone  bool
	pluginsError string

	languages      []openidea.LanguageInfo
	languagesBusy  bool
	languagesDone  bool
	languagesError string

	seq         int
	fileReq     uint32
	symbolReq   uint32
	textCh      uint32
	endpointSeq int

	// filters is the funnel's state, and filterOpen whether its panel is up.
	filters    seFilters
	filterOpen bool
}

// seQueryTimeout bounds one backend lookup of the overlay.
const seQueryTimeout = 30 * time.Second

// OpenSearchEverywhere opens the overlay over the window.
func (m *Model) OpenSearchEverywhere() {
	m.openSearchEverywhereAt(seTabAll)
}

func (m *Model) openSearchEverywhereAt(tab seTab) {
	if m.se != nil {
		m.SetSearchEverywhereTab(tab)
		return
	}
	m.se = &sePane{open: true, tab: tab, extra: map[seSection]int{}, filters: m.seFilters}
	if t := m.tab(); t != nil && t.state != nil {
		m.se.query = t.state.SelectedText()
	}
	m.refreshSearchEverywhere()
	m.loadSearchEverywhereLanguages()
	m.loadSearchEverywherePlugins()
	m.invalidate()
}

// CloseSearchEverywhere drops the overlay and cancels lookups in flight.
func (m *Model) CloseSearchEverywhere() {
	s := m.se
	if s == nil {
		return
	}
	m.se = nil
	if s.fileReq != 0 {
		m.cancelRequest(s.fileReq)
	}
	if s.symbolReq != 0 {
		m.cancelRequest(s.symbolReq)
	}
	if s.textCh != 0 {
		m.cancelRequest(s.textCh)
	}
	m.invalidate()
}

// SetSearchEverywhereTab switches the narrowing tab, as a click or Tab does.
func (m *Model) SetSearchEverywhereTab(tab seTab) {
	if m.se == nil {
		return
	}
	if m.se.tab == tab {
		return
	}
	m.se.tab = tab
	m.se.sel = 0
	m.se.selKey = ""
	m.se.filterOpen = false
	m.refreshSearchEverywhere()
	m.loadSearchEverywhereLanguages()
	m.invalidate()
}

// CycleSearchEverywhereTab walks the tab row, as Tab and Shift+Tab do.
func (m *Model) CycleSearchEverywhereTab(dir int) {
	if m.se == nil {
		return
	}
	i := 0
	for j, t := range seTabs {
		if t.id == m.se.tab {
			i = j
		}
	}
	n := len(seTabs)
	m.SetSearchEverywhereTab(seTabs[((i+dir)%n+n)%n].id)
}

// seEmpty reports whether the tab lists recents instead of results.
func (s *sePane) seEmpty() bool { return strings.TrimSpace(s.query) == "" }

// sectionsFor is which sections the tab queries, as upstream's sectionsFor.
func sectionsFor(tab seTab, emptyQuery bool) []seSection {
	if emptyQuery {
		switch tab {
		case seTabAll:
			return []seSection{seHistory, seRecentFiles, seRecentActions}
		case seTabFiles:
			return []seSection{seRecentFiles}
		case seTabActions:
			return []seSection{seRecentActions}
		default:
			return nil
		}
	}
	if tab == seTabAll {
		return []seSection{seClasses, seFiles, seSymbols, seEndpoints, seActions, seText}
	}
	return []seSection{seSection(tab)}
}

// seSectionLimit is how many rows a section shows before a More row.
func (s *sePane) seSectionLimit(section seSection) int {
	base := seTabLimit
	if s.tab == seTabAll && section != seRecentFiles && section != seRecentActions && section != seHistory {
		base = seAllLimit
	} else if section == seRecentFiles || section == seRecentActions || section == seHistory {
		base = seRecentLimit
	}
	return base + s.extra[section]
}

// loadSearchEverywhereLanguages loads the File Types definitions once for the
// funnel. Their extensions are how Files, Classes and Symbols apply language
// exclusions, matching OpenIDEA's extensionIndex.
func (m *Model) loadSearchEverywhereLanguages() {
	s := m.se
	if s == nil || s.languagesBusy || s.languagesDone {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.languagesDone = true
		return
	}
	s.languagesBusy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		languages, err := m.mgr.ToolLanguages(ctx, Window)
		m.post(func() {
			if m.se != s {
				return
			}
			s.languagesBusy = false
			s.languagesDone = true
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" {
					s.languagesError = "Loading file types failed: " + err.Error()
				}
				m.invalidate()
				return
			}
			s.languages = languages
			m.invalidate()
		})
	}()
}

// refreshSearchEverywhere restarts the lookups the tab needs.
func (m *Model) refreshSearchEverywhere() {
	s := m.se
	if s == nil {
		return
	}
	if parsed, ok := parseSECommand(s.query); ok {
		if parsed.list {
			s.seq++
			s.files = nil
			s.symbols = nil
			s.endpoints = nil
			s.text = nil
			s.textCount = 0
			s.textBusy = false
			s.fileDone = true
			s.symbolDone = true
			s.endpointDone = true
			s.textDone = true
			s.textTruncated = false
			s.err = ""
			m.clampSESelection()
			m.invalidate()
			return
		}
		s.tab = parsed.tab
		s.query = parsed.rest
		s.extra = map[seSection]int{}
		s.sel = 0
		s.selKey = ""
	}
	s.seq++
	seq := s.seq
	s.files = nil
	s.symbols = nil
	s.endpoints = nil
	s.text = nil
	s.textCount = 0
	s.textBusy = false
	s.fileDone = false
	s.symbolDone = false
	s.endpointDone = false
	s.textDone = false
	s.textTruncated = false
	s.err = ""
	m.clampSESelection()
	sections := sectionsFor(s.tab, s.seEmpty())
	rawQuery := strings.TrimSpace(s.query)
	s.location = parseSELocationQuery(rawQuery, m.Root())
	query := strings.TrimSpace(s.location.text)
	wantsFiles, wantsSymbols, wantsEndpoints, wantsText := false, false, false, false
	for _, sec := range sections {
		switch sec {
		case seRecentFiles, seFiles:
			wantsFiles = true
		case seClasses, seSymbols:
			wantsSymbols = true
		case seEndpoints:
			wantsEndpoints = true
		case seText:
			wantsText = true
		}
	}
	if wantsText && !seTextQueryReady(query) {
		wantsText = false
		s.textDone = true
	}
	if m.Root() == "" {
		wantsFiles, wantsSymbols, wantsEndpoints, wantsText = false, false, false, false
		s.fileDone = true
		s.symbolDone = true
		s.endpointDone = true
		s.textDone = true
	}
	if wantsFiles {
		m.seQueryFiles(seq, query)
	}
	if wantsSymbols {
		m.seQuerySymbols(seq, query)
	}
	if wantsEndpoints {
		m.seQueryEndpoints(seq, query)
	}
	if wantsText {
		m.seQueryText(seq, query)
	}
	m.invalidate()
}

// seQueryFiles asks the file index; an empty query lists recent files.
func (m *Model) seQueryFiles(seq int, query string) {
	s := m.se
	req := m.nextRequest()
	s.fileReq = req
	go func() {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(seDebounce):
		}
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		limit := seFileQueryLimit
		if s.tab == seTabFiles && !s.seEmpty() {
			limit = seTabLimit + 1
		}
		raw, err := m.mgr.Invoke(ctx, Window, "find_files", map[string]any{
			"query": query, "limit": limit,
		})
		var files []openidea.FileMatch
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			if m.se != s || s.seq != seq {
				return
			}
			s.fileReq = 0
			s.fileDone = true
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" {
					s.err = "Finding files failed: " + err.Error()
				}
			} else {
				s.files = files
			}
			m.invalidate()
		})
	}()
}

// seQuerySymbols asks the language server for workspace symbols.
func (m *Model) seQuerySymbols(seq int, query string) {
	s := m.se
	req := m.nextRequest()
	s.symbolReq = req
	go func() {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(seDebounce):
		}
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_workspace_symbols", map[string]any{
			"request": req, "query": query,
		})
		var syms []openidea.WorkspaceSymbol
		if err == nil {
			err = json.Unmarshal(raw, &syms)
		}
		m.post(func() {
			if m.se != s || s.seq != seq {
				return
			}
			s.symbolReq = 0
			s.symbolDone = true
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" && s.err == "" {
					s.err = "Finding symbols failed: " + err.Error()
				}
			} else if len(syms) > seSymbolQueryLimit {
				syms = syms[:seSymbolQueryLimit]
			}
			s.symbols = syms
			m.invalidate()
		})
	}()
}

// seQueryText streams a project text search, keeping only what the section
// shows; the rest of the stream is dropped.
func (m *Model) seQueryText(seq int, query string) {
	s := m.se
	ch := m.nextChannel()
	s.textCh = ch
	s.textBusy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "search_start", map[string]any{
			"request": ch,
			"query": map[string]any{
				"pattern": query, "isRegex": false, "caseSensitive": false,
				"wholeWord": false, "include": searchMaskList(s.filters.Text), "exclude": []string{},
				"scope": map[string]any{"kind": "project"}, "maxMatches": 0,
			},
			"onEvent": map[string]any{"$channel": ch},
		})
		m.post(func() {
			if m.se != s || s.seq != seq {
				return
			}
			s.textCh = 0
			if err != nil && openidea.ErrorKind(err) != "cancelled" && s.err == "" {
				s.err = "Searching failed: " + err.Error()
			}
			m.invalidate()
		})
	}()
}

// acceptSearchEverywhereEvent consumes one streamed text-search message.
func (m *Model) acceptSearchEverywhereEvent(data json.RawMessage) {
	s := m.se
	if s == nil {
		return
	}
	var ev openidea.SearchEvent
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	switch ev.Type {
	case "file":
		if ev.File == nil || len(ev.File.Matches) == 0 {
			return
		}
		if len(s.text) >= seTextFileLimit {
			s.textTruncated = true
			return
		}
		s.text = append(s.text, *ev.File)
		s.textCount += len(ev.File.Matches)
		if s.textCount >= seTextMatchLimit {
			s.textTruncated = true
		}
	case "done":
		s.textBusy = false
		s.textDone = true
	}
	m.clampSESelection()
	m.invalidate()
}

// seActionItems is the registry filtered by the query, as Find Action does.
func (m *Model) seActionItems(query string) []seItem {
	var items []seItem
	for _, cmd := range commands() {
		if !commandMatches(cmd, query) {
			continue
		}
		enabled := m.ActionEnabled(cmd.id)
		detail := cmd.group
		if cmd.shortcut != "" {
			detail += " · " + cmd.shortcut
		}
		items = append(items, seItem{
			kind:     "action",
			title:    cmd.title,
			detail:   detail,
			action:   cmd.id,
			disabled: !enabled,
			indices:  seMatchIndices(cmd.title, query),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].disabled != items[j].disabled {
			return !items[i].disabled
		}
		return false
	})
	return items
}

// seRecentActionItems is the recently run commands, newest first.
func (m *Model) seRecentActionItems() []seItem {
	var items []seItem
	for _, id := range m.recentActions {
		cmd, ok := Command(id)
		if !ok {
			continue
		}
		items = append(items, seItem{
			kind:     "action",
			title:    cmd.title,
			detail:   cmd.group,
			action:   cmd.id,
			disabled: !m.ActionEnabled(cmd.id),
		})
	}
	return items
}

// seSettingItems lists the settings pages whose title, group, keywords or
// options match, as OpenIDEA merges settings into Find Action results.
func (m *Model) seSettingItems(query string) []seItem {
	terms := settingsQueryTerms(query)
	if len(terms) == 0 {
		return nil
	}
	var items []seItem
	for _, page := range settingsPages() {
		if !settingsMatches(settingsPageTexts(page), terms) {
			continue
		}
		items = append(items, seItem{
			kind:    "setting",
			title:   page.title,
			detail:  page.group,
			setting: page.id,
			indices: seMatchIndices(page.title, query),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := strings.HasPrefix(strings.ToLower(items[i].title), strings.ToLower(strings.TrimSpace(query)))
		right := strings.HasPrefix(strings.ToLower(items[j].title), strings.ToLower(strings.TrimSpace(query)))
		return left && !right
	})
	return items
}

// seMatchIndices returns the rune offsets of a case-insensitive subsequence
// match, which is how the chooser highlights a result title.
func seMatchIndices(text, query string) []int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	want := []rune(query)
	var out []int
	at := 0
	for i, r := range []rune(strings.ToLower(text)) {
		if at >= len(want) {
			break
		}
		if r == want[at] {
			out = append(out, i)
			at++
		}
	}
	if at != len(want) {
		return nil
	}
	return out
}

// isTypeKind reports whether a workspace symbol is a class-like declaration,
// which is what the Classes tab keeps.
func isTypeKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "class", "interface", "enum", "record", "annotation", "struct", "trait", "typealias", "object":
		return true
	}
	return false
}

// seRows flattens the loaded results into headers, items and More rows.
func (m *Model) seRows() []seRow {
	s := m.se
	if s == nil {
		return nil
	}
	if parsed, ok := parseSECommand(s.query); ok && parsed.list {
		rows := make([]seRow, 0, len(parsed.rows))
		for _, item := range parsed.rows {
			rows = append(rows, seRow{item: item})
		}
		return rows
	}
	query := strings.TrimSpace(parseSELocationQuery(strings.TrimSpace(s.query), m.Root()).text)
	var rows []seRow
	add := func(section seSection, items []seItem) {
		if len(items) == 0 {
			return
		}
		limit := s.seSectionLimit(section)
		rows = append(rows, seRow{section: section, header: true})
		for i, it := range items {
			if i >= limit {
				rows = append(rows, seRow{section: section, more: true})
				break
			}
			rows = append(rows, seRow{section: section, item: it})
		}
	}
	fileItems := func() []seItem {
		items := make([]seItem, 0, len(s.files))
		for _, f := range s.files {
			if s.tab != seTabAll && s.filters.languageExcluded(s.tab, f.Path, s.languages) {
				continue
			}
			name := filepath.Base(f.RelPath)
			indices := seFileMatchIndices(f.RelPath, name, f.Indices)
			line, col := 0, 0
			if parsed := parseSELocationQuery(strings.TrimSpace(s.query), m.Root()); parsed.has {
				line, col = parsed.line, parsed.col
			}
			items = append(items, seItem{
				kind:    "file",
				title:   name,
				detail:  filepath.Dir(f.RelPath),
				path:    f.Path,
				line:    line,
				col:     col,
				indices: indices,
			})
		}
		return items
	}
	symbolItems := func(tab seTab, typesOnly bool) []seItem {
		items := make([]seItem, 0, len(s.symbols))
		for _, sym := range s.symbols {
			if typesOnly && !isTypeKind(sym.Kind) {
				continue
			}
			if s.filters.kindExcluded(tab, sym.Kind) {
				continue
			}
			if s.filters.languageExcluded(tab, sym.Location.Path, s.languages) {
				continue
			}
			detail := filepath.Dir(sym.Location.Path)
			if sym.Container != nil && *sym.Container != "" {
				detail = *sym.Container + " · " + detail
			}
			items = append(items, seItem{
				kind:    "symbol",
				symbol:  strings.ToLower(sym.Kind),
				title:   sym.Name,
				detail:  detail,
				path:    sym.Location.Path,
				line:    int(sym.Location.Range.Start.Line),
				col:     int(sym.Location.Range.Start.Col),
				indices: seMatchIndices(sym.Name, query),
			})
		}
		return items
	}
	textItems := func() []seItem {
		items := make([]seItem, 0, seTextMatchLimit)
		for _, f := range s.text {
			if !seTextMaskMatches(s.filters.Text, f.Path) {
				continue
			}
			for _, hit := range f.Matches {
				col := 0
				if len(hit.Ranges) > 0 {
					col = int(hit.Ranges[0][0])
				}
				items = append(items, seItem{
					kind:   "text",
					title:  strings.TrimSpace(hit.Text),
					detail: fmt.Sprintf("%s:%d", filepath.Base(f.Path), hit.Line+1),
					path:   f.Path,
					line:   int(hit.Line),
					col:    col,
				})
				if len(items) >= seTextMatchLimit {
					return items
				}
			}
		}
		return items
	}
	historyItems := func() []seItem {
		items := make([]seItem, 0, len(m.searchHistory))
		for _, q := range m.searchHistory {
			items = append(items, seItem{kind: "history", title: q})
		}
		return items
	}
	for _, section := range sectionsFor(s.tab, s.seEmpty()) {
		switch section {
		case seHistory:
			add(section, historyItems())
		case seRecentFiles:
			add(section, fileItems())
		case seRecentActions:
			add(section, m.seRecentActionItems())
		case seClasses:
			add(section, symbolItems(seTabClasses, true))
		case seFiles:
			add(section, fileItems())
		case seSymbols:
			add(section, symbolItems(seTabSymbols, false))
		case seEndpoints:
			add(section, seEndpointItems(s.endpoints, strings.TrimSpace(s.query)))
		case seActions:
			items := append(m.seActionItems(query), m.seSettingItems(query)...)
			add(section, items)
		case seText:
			add(section, textItems())
		}
	}
	return rows
}

// seSelectable reports whether a row can take the selection.
func seSelectable(r seRow) bool { return !r.header }

func seRowKey(r seRow) string {
	switch {
	case r.header:
		return "section:" + string(r.section)
	case r.more:
		return "more:" + string(r.section)
	}
	it := r.item
	switch it.kind {
	case "command":
		return "command:" + string(it.command)
	case "action":
		return "action:" + it.action
	case "setting":
		return "setting:" + it.setting
	case "history":
		return "history:" + strings.ToLower(it.title)
	default:
		return fmt.Sprintf("%s:%s:%d:%d:%s", it.kind, it.path, it.line, it.col, it.title)
	}
}

// stepSearchEverywhere moves the selection, skipping headers and stopping at
// the ends.
func (m *Model) stepSearchEverywhere(delta int) {
	s := m.se
	if s == nil {
		return
	}
	m.clampSESelection()
	rows := m.seRows()
	i := s.sel + delta
	for i >= 0 && i < len(rows) && !seSelectable(rows[i]) {
		i += delta
	}
	if i < 0 || i >= len(rows) {
		return
	}
	s.sel = i
	s.selKey = seRowKey(rows[i])
	m.invalidate()
}

// clampSESelection keeps the selection on an item after the rows changed.
func (m *Model) clampSESelection() {
	s := m.se
	if s == nil {
		return
	}
	rows := m.seRows()
	if s.selKey != "" {
		for i, row := range rows {
			if seSelectable(row) && seRowKey(row) == s.selKey {
				s.sel = i
				return
			}
		}
	}
	if s.sel >= 0 && s.sel < len(rows) && seSelectable(rows[s.sel]) {
		s.selKey = seRowKey(rows[s.sel])
		return
	}
	if s.sel >= len(rows) || (s.sel >= 0 && !seSelectable(rows[s.sel])) {
		for i, r := range rows {
			if seSelectable(r) {
				s.sel = i
				s.selKey = seRowKey(r)
				return
			}
		}
		s.sel = -1
		s.selKey = ""
	}
}

// ActivateSearchEverywhere opens the chosen row or widens its section.
func (m *Model) ActivateSearchEverywhere() {
	m.activateSearchEverywhere(false)
}

// ActivateSearchEverywhereInOppositeGroup opens a file result in the other
// editor group, as Shift+Enter does in OpenIDEA.
func (m *Model) ActivateSearchEverywhereInOppositeGroup() {
	m.activateSearchEverywhere(true)
}

func (m *Model) activateSearchEverywhere(side bool) {
	s := m.se
	if s == nil {
		return
	}
	m.clampSESelection()
	rows := m.seRows()
	if s.sel < 0 || s.sel >= len(rows) {
		m.invalidate()
		return
	}
	row := rows[s.sel]
	if row.more {
		step := seMoreStepTab
		if s.tab == seTabAll {
			step = seMoreStepAll
		}
		s.extra[row.section] += step
		s.selKey = seRowKey(row)
		m.invalidate()
		return
	}
	item := row.item
	switch item.kind {
	case "command":
		s.tab = item.command
		s.query = ""
		s.sel = 0
		s.selKey = ""
		s.extra = map[seSection]int{}
		s.filterOpen = false
		m.refreshSearchEverywhere()
	case "history":
		s.query = item.title
		s.sel = 0
		s.selKey = ""
		s.extra = map[seSection]int{}
		m.refreshSearchEverywhere()
	case "action":
		if item.disabled {
			return
		}
		m.rememberRecentAction(item.action)
		m.CloseSearchEverywhere()
		m.RunCommand(item.action)
	case "setting":
		m.rememberSearchQuery(strings.TrimSpace(s.query))
		m.CloseSearchEverywhere()
		m.OpenSettingsPage(item.setting)
	case "file", "symbol", "endpoint", "text":
		m.rememberSearchQuery(strings.TrimSpace(s.query))
		m.CloseSearchEverywhere()
		m.revealAt(item.path, item.line, item.col)
		if side {
			m.OpenInOppositeGroup()
		}
	default:
		m.invalidate()
	}
}

// rememberSearchQuery puts an executed query first in the search history,
// once, as OpenIDEA's Search Everywhere history does.
func (m *Model) rememberSearchQuery(query string) {
	if query == "" {
		return
	}
	list := []string{query}
	for _, old := range m.searchHistory {
		if !strings.EqualFold(old, query) {
			list = append(list, old)
		}
	}
	if len(list) > seHistoryCap {
		list = list[:seHistoryCap]
	}
	m.searchHistory = list
	m.saveSession()
}

// rememberRecentAction puts a run command first in the recent list.
func (m *Model) rememberRecentAction(id string) {
	list := []string{id}
	for _, old := range m.recentActions {
		if old != id {
			list = append(list, old)
		}
	}
	if len(list) > seRecentActionCap {
		list = list[:seRecentActionCap]
	}
	m.recentActions = list
}

// searchEverywhereDialog draws the overlay.
func (m *Model) searchEverywhereDialog(c *ui.Context, p *ui.Theme) {
	s := m.se
	if s == nil {
		return
	}
	open := s.open
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(680).Gap(8).Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				for _, t := range seTabs {
					t := t
					on := s.tab == t.id
					b := ui.ButtonBase(c.Key("se:tab:"+string(t.id))).Height(24).
						Padding(0, 8).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if on {
						b.Background(p.SurfaceHover)
					} else if b.Hovered() {
						b.Background(p.SurfaceHover)
					}
					b.Children(func() {
						col := p.TextMuted
						if on {
							col = p.Text
						}
						ui.Text(c, t.label).FontSize(12).TextColor(col).SingleLine()
					})
					b.OnClick(func() { m.SetSearchEverywhereTab(t.id) })
				}
				if seFilterTab(s.tab) {
					ui.Spacer(c)
					b := ui.ButtonBase(c.Key("se:filter")).Height(24).Padding(0, 8).
						Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if s.filterOpen || s.filters.active(s.tab) {
						b.Background(p.SurfaceHover)
					} else if b.Hovered() {
						b.Background(p.SurfaceHover)
					}
					b.Tooltip("Filter")
					b.Children(func() {
						ui.Text(c, "Filter").FontSize(12).TextColor(p.TextMuted).SingleLine()
					})
					b.OnClick(func() { s.filterOpen = !s.filterOpen })
				}
			})
			if s.filterOpen && seFilterTab(s.tab) {
				m.seFilterPanel(c, p, s)
			}
			in := ui.TextInput(c, &s.query).AutoFocus()
			in.OnChange(func() {
				m.refreshSearchEverywhere()
				m.clampSESelection()
			})
			in.OnShortcut(0, ui.KeyDown, func() { m.stepSearchEverywhere(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepSearchEverywhere(-1) })
			in.OnShortcut(0, ui.KeyTab, func() { m.CycleSearchEverywhereTab(1) })
			in.OnShortcut(ui.Shift, ui.KeyTab, func() { m.CycleSearchEverywhereTab(-1) })
			if in.Submitted() {
				m.ActivateSearchEverywhere()
			}
			rows := m.seRows()
			if s.sel >= len(rows) {
				s.sel = 0
			}
			if len(rows) == 0 {
				switch {
				case s.err != "":
					errorState(c, p, s.err)
				case s.tab == seTabText && s.textBusy:
					loadingState(c, p, "Searching")
				case s.seEmpty() && s.tab == seTabAll:
					emptyState(c, p, "Type to search classes, files, symbols, actions and text")
				default:
					emptyState(c, p, "No matching results")
				}
				return
			}
			ui.Text(c, count(m.seItemCount(rows), "result", "results")).
				FontSize(12).TextColor(p.TextMuted).SingleLine()
			ui.Scroll(c).Height(380).Children(func() {
				ui.Column(c).Children(func() {
					for i, row := range rows {
						i, row := i, row
						if row.header {
							ui.Text(c, seSectionTitles[row.section]).FontSize(12).
								TextColor(p.TextMuted).Padding(6, 8).SingleLine()
							continue
						}
						if row.more {
							r := ui.Row(c.Key(fmt.Sprintf("se:more:%s", row.section))).Height(24).
								Padding(0, 8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
							if r.Hovered() {
								r.Background(p.SurfaceHover)
							}
							r.Children(func() {
								ui.Text(c, "More…").FontSize(12).TextColor(p.TextMuted).SingleLine()
							})
							r.OnClick(func() {
								s.sel = i
								m.ActivateSearchEverywhere()
							})
							continue
						}
						r := ui.Row(c.Key(fmt.Sprintf("se:%d", i))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
						if i == s.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							m.seRowIcon(c, p, row.item)
							ui.Text(c, row.item.title).FontSize(12).TextColor(p.Text).
								SingleLine().Ellipsis("…")
							ui.Spacer(c)
							ui.Text(c, row.item.detail).FontSize(12).
								TextColor(p.TextMuted).SingleLine().Ellipsis("…")
						})
						r.OnClick(func() {
							s.sel = i
							m.ActivateSearchEverywhere()
						})
					}
				})
			})
		})
	})
	if !open {
		m.CloseSearchEverywhere()
	}
}

// seRowIcon is the row's leading 16px drawing: the file type for files and
// text matches, the symbol's node icon for declarations, and an empty slot
// where upstream's action rows have no icon of their own.
func (m *Model) seRowIcon(c *ui.Context, p *ui.Theme, it seItem) {
	oi := oiPaletteOf(p)
	switch it.kind {
	case "file":
		fileIconEl(c, oi, it.path, 16, oi.textSubtle)
	case "endpoint":
		oiIconEl(c, "fileTypes/http", 16, oi, oi.textSubtle)
	case "text":
		oiIconEl(c, "openidea/nodes/textMatch", 16, oi, oi.textSubtle)
	case "symbol":
		if asset := symbolIcon(it.symbol); asset != "" {
			oiIconEl(c, asset, 16, oi, oi.textSubtle)
			return
		}
		letter := "?"
		if it.symbol != "" {
			letter = strings.ToUpper(it.symbol[:1])
		}
		ui.Row(c).Size(16, 16).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Text(c, letter).FontSize(12).TextColor(oi.textSubtle).SingleLine()
		})
	default:
		ui.Row(c).Size(16, 16).Shrink(0)
	}
}

// symbolIcon is the expui node drawing for a symbol kind, as SYMBOL_ICONS maps
// it; "" when the kind has none and upstream draws a letter badge instead.
func symbolIcon(kind string) string {
	switch kind {
	case "class":
		return "nodes/class"
	case "interface", "trait":
		return "nodes/interface"
	case "enum":
		return "nodes/enum"
	case "struct", "record":
		return "nodes/record"
	case "method":
		return "nodes/method"
	case "function":
		return "nodes/function"
	case "constructor":
		return "nodes/constructor"
	case "field":
		return "nodes/field"
	case "property":
		return "nodes/property"
	case "variable":
		return "nodes/variable"
	case "constant":
		return "nodes/constant"
	case "parameter":
		return "nodes/parameter"
	case "module":
		return "nodes/module"
	case "namespace", "package":
		return "nodes/package"
	case "type":
		return "nodes/type"
	case "annotation":
		return "nodes/annotation"
	case "snippet":
		return "nodes/template"
	}
	return ""
}

// seItemCount counts the item rows, leaving out headers and More rows.
func (m *Model) seItemCount(rows []seRow) int {
	n := 0
	for _, r := range rows {
		if seSelectable(r) && !r.more {
			n++
		}
	}
	return n
}

// SmokeSearchEverywhere drives the overlay over a real project and reports
// what each tab produced, for the release e2e.
func (m *Model) SmokeSearchEverywhere(ctx context.Context, query string) map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() { m.OpenSearchEverywhere() })
	type state struct {
		rows     int
		items    int
		sections []string
	}
	sample := func(tab seTab) state {
		var st state
		mygo.RunOnMain(func() {
			if m.se == nil {
				return
			}
			m.SetSearchEverywhereTab(tab)
			m.se.query = query
			m.refreshSearchEverywhere()
		})
		needFile, needSymbol, needEndpoints, needText := false, false, false, false
		for _, sec := range sectionsFor(tab, strings.TrimSpace(query) == "") {
			switch sec {
			case seRecentFiles, seFiles:
				needFile = true
			case seClasses, seSymbols:
				needSymbol = true
			case seEndpoints:
				needEndpoints = true
			case seText:
				needText = true
			}
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			var done bool
			mygo.RunOnMain(func() {
				s := m.se
				done = s != nil &&
					(!needFile || s.fileDone) &&
					(!needSymbol || s.symbolDone) &&
					(!needEndpoints || s.endpointDone) &&
					(!needText || !s.textBusy) &&
					m.seItemCount(m.seRows()) > 0
			})
			if done || time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return st
			case <-time.After(200 * time.Millisecond):
			}
		}
		mygo.RunOnMain(func() {
			rows := m.seRows()
			st.rows = len(rows)
			st.items = m.seItemCount(rows)
			seen := map[seSection]bool{}
			for _, r := range rows {
				if r.header && !seen[r.section] {
					seen[r.section] = true
					st.sections = append(st.sections, string(r.section))
				}
			}
		})
		return st
	}
	all := sample(seTabAll)
	classes := sample(seTabClasses)
	files := sample(seTabFiles)
	symbols := sample(seTabSymbols)
	actions := sample(seTabActions)
	text := sample(seTabText)
	mygo.RunOnMain(func() {
		if m.se == nil {
			out["open"] = false
			return
		}
		out["open"] = true
		out["allRows"] = all.rows
		out["allSections"] = all.sections
		out["classes"] = classes.items
		out["files"] = files.items
		out["symbols"] = symbols.items
		out["endpoints"] = len(m.se.endpoints)
		out["actions"] = actions.items
		out["text"] = text.items
		out["tab"] = string(m.se.tab)
		entryTabs := map[string]string{}
		for _, id := range []string{"navigate.gotoClass", "navigate.gotoFile", "navigate.gotoSymbol", "navigate.findAction"} {
			if m.RunCommand(id) && m.se != nil {
				entryTabs[id] = string(m.se.tab)
			}
		}
		out["entryTabs"] = entryTabs
		m.CloseSearchEverywhere()
	})
	return out
}
