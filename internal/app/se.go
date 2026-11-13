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
	seRecentFiles   seSection = "recent"
	seRecentActions seSection = "recentActions"
	seClasses       seSection = "classes"
	seFiles         seSection = "files"
	seSymbols       seSection = "symbols"
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
	seRecentFiles:   "Recent Files",
	seRecentActions: "Recent Actions",
	seClasses:       "Classes",
	seFiles:         "Files",
	seSymbols:       "Symbols",
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
	seDebounce         = 120 * time.Millisecond
)

// seItem is one result row before it is drawn.
type seItem struct {
	kind   string // "file", "symbol", "action"
	title  string
	detail string
	path   string
	line   int
	col    int
	action string
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
	// extra widens a section past its first page, as the More rows do.
	extra map[seSection]int

	files     []openidea.FileMatch
	symbols   []openidea.WorkspaceSymbol
	text      []openidea.FileMatches
	textCount int

	fileDone, symbolDone, textDone bool
	textBusy                       bool
	textTruncated                  bool
	err                            string

	seq       int
	fileReq   uint32
	symbolReq uint32
	textCh    uint32
}

// seQueryTimeout bounds one backend lookup of the overlay.
const seQueryTimeout = 30 * time.Second

// OpenSearchEverywhere opens the overlay over the window.
func (m *Model) OpenSearchEverywhere() {
	if m.se != nil {
		return
	}
	m.se = &sePane{open: true, tab: seTabAll, extra: map[seSection]int{}}
	m.refreshSearchEverywhere()
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
	m.refreshSearchEverywhere()
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
			return []seSection{seRecentFiles, seRecentActions}
		case seTabFiles:
			return []seSection{seRecentFiles}
		case seTabActions:
			return []seSection{seRecentActions}
		default:
			return nil
		}
	}
	if tab == seTabAll {
		return []seSection{seClasses, seFiles, seSymbols, seActions, seText}
	}
	return []seSection{seSection(tab)}
}

// seSectionLimit is how many rows a section shows before a More row.
func (s *sePane) seSectionLimit(section seSection) int {
	base := seTabLimit
	if s.tab == seTabAll && section != seRecentFiles && section != seRecentActions {
		base = seAllLimit
	} else if section == seRecentFiles || section == seRecentActions {
		base = seRecentLimit
	}
	return base + s.extra[section]
}

// refreshSearchEverywhere restarts the lookups the tab needs.
func (m *Model) refreshSearchEverywhere() {
	s := m.se
	if s == nil {
		return
	}
	s.seq++
	seq := s.seq
	s.files = nil
	s.symbols = nil
	s.text = nil
	s.textCount = 0
	s.textBusy = false
	s.fileDone = false
	s.symbolDone = false
	s.textDone = false
	s.textTruncated = false
	s.err = ""
	if s.sel >= len(m.seRows()) {
		s.sel = 0
	}
	sections := sectionsFor(s.tab, s.seEmpty())
	query := strings.TrimSpace(s.query)
	wantsFiles, wantsSymbols, wantsText := false, false, false
	for _, sec := range sections {
		switch sec {
		case seRecentFiles, seFiles:
			wantsFiles = true
		case seClasses, seSymbols:
			wantsSymbols = true
		case seText:
			wantsText = true
		}
	}
	if m.Root() == "" {
		wantsFiles, wantsSymbols, wantsText = false, false, false
	}
	if wantsFiles {
		m.seQueryFiles(seq, query)
	}
	if wantsSymbols {
		m.seQuerySymbols(seq, query)
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
					s.err = "查找文件失败：" + err.Error()
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
					s.err = "查找符号失败：" + err.Error()
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
				"wholeWord": false, "include": []string{}, "exclude": []string{},
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
				s.err = "搜索失败：" + err.Error()
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
	m.invalidate()
}

// seActionItems is the registry filtered by the query, as Find Action does.
func (m *Model) seActionItems(query string) []seItem {
	var items []seItem
	for _, cmd := range commands() {
		if !commandMatches(cmd, query) {
			continue
		}
		items = append(items, seItem{
			kind:   "action",
			title:  cmd.title,
			detail: cmd.group,
			action: cmd.id,
		})
		if len(items) >= seTabLimit {
			break
		}
	}
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
			kind:   "action",
			title:  cmd.title,
			detail: cmd.group,
			action: cmd.id,
		})
	}
	return items
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
			items = append(items, seItem{
				kind:   "file",
				title:  filepath.Base(f.Path),
				detail: filepath.Dir(f.RelPath),
				path:   f.Path,
			})
		}
		return items
	}
	symbolItems := func(typesOnly bool) []seItem {
		items := make([]seItem, 0, len(s.symbols))
		for _, sym := range s.symbols {
			if typesOnly && !isTypeKind(sym.Kind) {
				continue
			}
			detail := filepath.Dir(sym.Location.Path)
			if sym.Container != nil && *sym.Container != "" {
				detail = *sym.Container + " · " + detail
			}
			items = append(items, seItem{
				kind:   "symbol",
				title:  sym.Name,
				detail: detail,
				path:   sym.Location.Path,
				line:   int(sym.Location.Range.Start.Line),
				col:    int(sym.Location.Range.Start.Col),
			})
		}
		return items
	}
	textItems := func() []seItem {
		items := make([]seItem, 0, seTextMatchLimit)
		for _, f := range s.text {
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
	for _, section := range sectionsFor(s.tab, s.seEmpty()) {
		switch section {
		case seRecentFiles:
			add(section, fileItems())
		case seRecentActions:
			add(section, m.seRecentActionItems())
		case seClasses:
			add(section, symbolItems(true))
		case seFiles:
			add(section, fileItems())
		case seSymbols:
			add(section, symbolItems(false))
		case seActions:
			add(section, m.seActionItems(strings.TrimSpace(s.query)))
		case seText:
			add(section, textItems())
		}
	}
	return rows
}

// seSelectable reports whether a row can take the selection.
func seSelectable(r seRow) bool { return !r.header }

// stepSearchEverywhere moves the selection, skipping headers and stopping at
// the ends.
func (m *Model) stepSearchEverywhere(delta int) {
	s := m.se
	if s == nil {
		return
	}
	rows := m.seRows()
	i := s.sel + delta
	for i >= 0 && i < len(rows) && !seSelectable(rows[i]) {
		i += delta
	}
	if i < 0 || i >= len(rows) {
		return
	}
	s.sel = i
	m.invalidate()
}

// clampSESelection keeps the selection on an item after the rows changed.
func (m *Model) clampSESelection() {
	s := m.se
	if s == nil {
		return
	}
	rows := m.seRows()
	if s.sel >= len(rows) || (s.sel >= 0 && !seSelectable(rows[s.sel])) {
		for i, r := range rows {
			if seSelectable(r) {
				s.sel = i
				return
			}
		}
		s.sel = -1
	}
}

// ActivateSearchEverywhere opens the chosen row or widens its section.
func (m *Model) ActivateSearchEverywhere() {
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
		m.invalidate()
		return
	}
	item := row.item
	switch item.kind {
	case "action":
		m.rememberRecentAction(item.action)
		m.CloseSearchEverywhere()
		m.RunCommand(item.action)
	case "file", "symbol", "text":
		m.CloseSearchEverywhere()
		m.revealAt(item.path, item.line, item.col)
	default:
		m.invalidate()
	}
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
						ui.Text(c, t.label).FontSize(11.5).TextColor(col).SingleLine()
					})
					b.OnClick(func() { m.SetSearchEverywhereTab(t.id) })
				}
			})
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
			hint := ""
			switch {
			case s.err != "":
				hint = s.err
			case s.seEmpty() && s.tab == seTabAll:
				hint = "输入以搜索类、文件、符号、操作和文本"
			case len(rows) == 0 && s.tab == seTabText && s.textBusy:
				hint = "正在搜索…"
			case len(rows) == 0:
				hint = "没有匹配的结果"
			default:
				hint = fmt.Sprintf("%d 个结果", m.seItemCount(rows))
			}
			ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			if len(rows) == 0 {
				return
			}
			ui.Scroll(c).Height(380).Children(func() {
				ui.Column(c).Children(func() {
					for i, row := range rows {
						i, row := i, row
						if row.header {
							ui.Text(c, seSectionTitles[row.section]).FontSize(10).
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
								ui.Text(c, "More…").FontSize(11.5).TextColor(p.TextMuted).SingleLine()
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
							ui.Text(c, row.item.title).FontSize(12).TextColor(p.Text).
								SingleLine().Ellipsis("…")
							ui.Spacer(c)
							ui.Text(c, row.item.detail).FontSize(10.5).
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
		needFile, needSymbol, needText := false, false, false
		for _, sec := range sectionsFor(tab, strings.TrimSpace(query) == "") {
			switch sec {
			case seRecentFiles, seFiles:
				needFile = true
			case seClasses, seSymbols:
				needSymbol = true
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
		out["files"] = files.items
		out["symbols"] = symbols.items
		out["actions"] = actions.items
		out["text"] = text.items
		out["tab"] = string(m.se.tab)
		m.CloseSearchEverywhere()
	})
	return out
}
