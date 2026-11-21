package app

// Show Usages after OpenIDEA's code-insight popup: a speed-search list at the
// caret, semantic read/write filters, an optional source preview and a transfer
// to the Find Usages tool window. The backend supplies the same streamed
// References answer as Find Usages.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const (
	usageScopeProject   = "project"
	usageScopeLibraries = "libraries"

	usagePopupEarlyDelay   = 300 * time.Millisecond
	usagePopupHintDelay    = 4 * time.Second
	usagePopupPreviewDelay = 60 * time.Millisecond
	usagePopupRows         = 14
	usagePopupRowHeight    = 24
	usagePopupPreviewTop   = 20
	usagePopupPreviewEnd   = 20
)

// usagePopupTarget identifies the editor position a popup search belongs to.
// The document version keeps a later scope switch from searching text that
// replaced the symbol.
type usagePopupTarget struct {
	path    string
	offset  int
	docID   uint32
	version uint32
	symbol  string
}

// usagePopupRow is one rendered reference. source keeps the exact multiline
// range across the transfer to Find Usages.
type usagePopupRow struct {
	source openidea.Location
	kind   string
	access string
	text   string
	endCol uint32
}

// usagePopupState is both the visible popup and the short-lived target after
// a single/no-result search, which lets the next shortcut press widen the
// same search to libraries.
type usagePopupState struct {
	open     bool
	symbol   string
	rows     []usagePopupRow
	note     string
	err      string
	loading  bool
	done     bool
	scope    string
	canWiden bool
	filter   string
	active   int

	target     usagePopupTarget
	channel    uint32
	generation uint64
	rowGen     uint64
	lineTexts  map[string]string

	early          bool
	earlyScheduled bool
	hintUntil      time.Time
	anchor         ui.Rect
	listScroll     ui.ScrollState

	preview      usagePopupPreview
	previewGen   uint64
	previewTimer *time.Timer
}

type usagePopupPreview struct {
	key     string
	path    string
	line    int
	col     uint32
	endCol  uint32
	start   int
	lines   []string
	err     string
	loading bool
}

// editorCaretAnchor approximates the caret's screen rectangle from the
// editor's last layout. MyGo exposes the line top and scroll offset but not a
// public caret rectangle, so the popup uses this point and clamps itself into
// the window when it draws.
func editorCaretAnchor(bounds ui.Rect, state *editor.State, fontSize float32) ui.Rect {
	if state == nil || bounds.W <= 0 || bounds.H <= 0 {
		return ui.Rect{}
	}
	pos := state.CursorPos()
	lineHeight := fontSize * max(float32(1), state.LineHeight)
	if lineHeight <= fontSize {
		lineHeight = fontSize * 1.45
	}
	gutter := float32(48)
	x := bounds.X + gutter + float32(pos.Col)*fontSize*0.62
	// LineTop is in the editor's content coordinates. Subtract ScrollY to
	// turn it into the line's current position inside the visible editor.
	y := bounds.Y + float32(state.LineTop(pos.Line)-state.ScrollY())
	x = max(bounds.X+8, min(x, bounds.X+bounds.W-8))
	y = max(bounds.Y+2, min(y, bounds.Y+bounds.H-lineHeight))
	return ui.Rect{X: x, Y: y, W: 1, H: lineHeight}
}

// ShowUsagesPopup implements Code › Show Usages (⌥⌘F7). Invoking it again
// while the popup or its hint shows Project Files searches the same target in
// Project and Libraries.
func (m *Model) ShowUsagesPopup() {
	existing := m.usagePopup
	if existing != nil && existing.canWiden &&
		((existing.open && existing.scope == usageScopeProject) || time.Now().Before(existing.hintUntil)) {
		m.searchUsagePopup(existing.target, true, true, existing)
		return
	}

	t := m.tab()
	if t == nil || t.doc == nil || !m.tabOpen(t) {
		m.status = "Open a file to show usages"
		m.invalidate()
		return
	}
	off := min(max(t.state.Cursor(), 0), len(t.doc.Text()))
	target := usagePopupTarget{
		path: t.doc.Path(), offset: off, docID: t.doc.ID(),
		version: t.doc.Version(), symbol: wordAt(t.doc.Text(), off),
	}
	if existing != nil && existing.open {
		m.searchUsagePopup(target, false, false, existing)
		return
	}
	m.searchUsagePopup(target, false, false, nil)
}

// searchUsagePopup starts one backend reference search. state is reused for a
// reopened popup or a scope switch, preserving its filter and any results
// already shown.
func (m *Model) searchUsagePopup(target usagePopupTarget, libraries, widenable bool, state *usagePopupState) {
	t := m.tabForUsageTarget(target)
	if t == nil || t.doc == nil || !m.tabOpen(t) {
		m.status = "The file for these usages is no longer open"
		m.invalidate()
		return
	}
	reopened := state != nil && state.open
	if state == nil {
		state = &usagePopupState{
			symbol: target.symbol, active: 0,
			canWiden: widenable, anchor: m.usagePopupAnchor,
		}
		m.usagePopup = state
	}
	if libraries {
		state.scope = usageScopeLibraries
	} else if reopened {
		state.scope = usageScopeProject
	}
	state.target = target
	state.symbol = target.symbol
	state.loading = true
	state.done = false
	state.err = ""
	state.early = false
	state.earlyScheduled = false
	state.hintUntil = time.Time{}
	state.canWiden = state.canWiden || widenable
	state.generation++
	gen := state.generation
	if state.channel != 0 {
		m.cancelUsageChannel(state.channel)
	}
	ch := m.nextChannel()
	state.channel = ch
	m.status = "Searching for usages…"
	m.invalidate()

	docID := t.doc.ID()
	version := t.doc.Version()
	pos := t.doc.UTF16Pos(target.offset)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, usageTimeout)
		defer cancel()
		if err := t.doc.Sync(ctx); err != nil {
			m.post(func() { m.finishUsagePopup(state, gen, ch, err, nil, libraries, widenable) })
			return
		}
		if t.doc.ID() != docID || t.doc.Version() != version {
			m.post(func() {
				m.finishUsagePopup(state, gen, ch,
					fmt.Errorf("the source changed while Show Usages was starting"), nil, libraries, widenable)
			})
			return
		}
		raw, err := m.invoke(ctx, "lsp_references", map[string]any{
			"request": ch, "doc": docID, "pos": lspPos(pos),
			"includeDeclaration": true, "projectWide": true, "libraries": libraries,
			"onPartial": map[string]any{"$channel": ch},
		})
		var refs openidea.References
		if err == nil {
			err = json.Unmarshal(raw, &refs)
		}
		m.post(func() { m.finishUsagePopup(state, gen, ch, err, &refs, libraries, widenable) })
	}()
}

func (m *Model) tabForUsageTarget(target usagePopupTarget) *Tab {
	for _, t := range m.allTabs() {
		if t.doc != nil && t.doc.ID() == target.docID && t.doc.Path() == target.path {
			return t
		}
	}
	return nil
}

func (m *Model) cancelUsageChannel(ch uint32) {
	if ch == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": ch})
	}()
}

func (m *Model) cancelUsagePopupRequest() {
	state := m.usagePopup
	if state == nil {
		return
	}
	state.generation++
	state.loading = false
	if state.channel != 0 {
		m.cancelUsageChannel(state.channel)
		state.channel = 0
	}
}

func (m *Model) resetUsagePopup() {
	m.cancelUsagePopupRequest()
	stopUsagePopupPreview(m.usagePopup)
	m.usagePopup = nil
	m.invalidate()
}

func (m *Model) dismissUsagePopup() {
	m.cancelUsagePopupRequest()
	stopUsagePopupPreview(m.usagePopup)
	m.usagePopup = nil
	m.invalidate()
}

// Escape closes Show Usages, but first clears a speed-search query as the
// upstream popup does.
func (m *Model) escapeUsagePopup() {
	state := m.usagePopup
	if state == nil {
		return
	}
	if state.filter != "" {
		state.filter = ""
		state.active = 0
		state.listScroll.Y = 0
		m.invalidate()
		return
	}
	m.dismissUsagePopup()
}

func (m *Model) acceptUsagePopup(ch uint32, data json.RawMessage) {
	state := m.usagePopup
	if state == nil || state.channel != ch || !state.loading {
		return
	}
	var refs openidea.References
	if json.Unmarshal(data, &refs) != nil {
		return
	}
	m.setUsagePopupSnapshot(state, &refs)
	if !state.open && !state.earlyScheduled && len(state.rows) > 0 {
		state.earlyScheduled = true
		gen := state.generation
		go func() {
			timer := time.NewTimer(usagePopupEarlyDelay)
			defer timer.Stop()
			select {
			case <-m.ctx.Done():
				return
			case <-timer.C:
			}
			m.post(func() {
				s := m.usagePopup
				if s != state || s.generation != gen || s.channel != ch || !s.loading || len(s.rows) == 0 {
					return
				}
				s.open = true
				s.early = true
				s.anchor = m.usagePopupAnchor
				m.status = "Searching for more usages…"
				m.invalidate()
			})
		}()
	}
	m.invalidate()
}

func (m *Model) finishUsagePopup(
	state *usagePopupState,
	gen uint64,
	ch uint32,
	err error,
	refs *openidea.References,
	libraries, widenable bool,
) {
	if m.usagePopup != state || state.generation != gen || state.channel != ch {
		return
	}
	state.loading = false
	state.done = true
	state.channel = 0
	if err != nil {
		if openidea.ErrorKind(err) == "cancelled" {
			return
		}
		state.err = err.Error()
		if state.open {
			m.status = "Showing usages failed"
		} else {
			m.status = "Showing usages failed: " + err.Error()
			m.usagePopup = nil
		}
		m.invalidate()
		return
	}
	if refs == nil {
		return
	}
	m.setUsagePopupSnapshot(state, refs)
	scope := ""
	switch {
	case refs.LibrariesSkipped:
		scope = usageScopeProject
	case libraries && widenable:
		scope = usageScopeLibraries
	}
	state.scope = scope
	state.canWiden = state.canWiden || widenable || scope != ""
	total := len(state.rows)

	if state.open || state.early || total > 1 {
		state.open = true
		state.anchor = m.usagePopupAnchor
		m.status = fmt.Sprintf("Found %d usages", total)
		m.invalidate()
		return
	}

	switch {
	case total == 0:
		switch {
		case state.scope == usageScopeProject && m.usagePopupShortcut() != "":
			state.hintUntil = time.Now().Add(usagePopupHintDelay)
			m.status = fmt.Sprintf(
				"No usages found in Project Files. Press %s again to search in 'Project and Libraries'",
				m.usagePopupShortcut())
		case state.scope == usageScopeProject:
			m.status = "No usages found in Project Files"
		default:
			name := state.symbol
			if name == "" {
				m.status = "No usages found"
			} else {
				m.status = fmt.Sprintf("No usages of '%s' found", name)
			}
		}
	case total == 1:
		state.hintUntil = time.Now().Add(usagePopupHintDelay)
		row := state.rows[0]
		m.pickUsagePopupRow(row)
		switch {
		case state.scope == usageScopeProject && m.usagePopupShortcut() != "":
			m.status = fmt.Sprintf(
				"Only one usage in Project Files. Press %s again to search in 'Project and Libraries'",
				m.usagePopupShortcut())
		case state.scope == usageScopeLibraries:
			m.status = "Only one usage in Project and Libraries"
		default:
			m.status = "Only one usage in Project Files"
		}
	}
	state.open = false
	m.invalidate()
}

func (m *Model) usagePopupShortcut() string {
	bindings := m.KeymapBindings("find.showUsages")
	if len(bindings) == 0 {
		return ""
	}
	return keymapDisplay(bindings[0])
}

func (m *Model) usageFindShortcut() string {
	bindings := m.KeymapBindings("find.usages")
	if len(bindings) == 0 {
		return ""
	}
	return keymapDisplay(bindings[0])
}

func (m *Model) setUsagePopupSnapshot(state *usagePopupState, refs *openidea.References) {
	locs := refs.Locations
	if len(locs) > maxUsages {
		locs = locs[:maxUsages]
	}
	classified := refs.Classified
	state.rowGen++
	rowGen := state.rowGen
	state.lineTexts = usageClassifiedLines(locs, classified)
	state.rows = buildUsagePopupRows(locs, classified, state.lineTexts)
	if state.active >= len(m.usagePopupVisibleRows()) {
		state.active = max(0, len(m.usagePopupVisibleRows())-1)
	}
	if refs.Incomplete != nil {
		state.note = incompleteText(*refs.Incomplete)
	} else {
		state.note = ""
	}

	go func() {
		texts := loadUsageLineTexts(locs, classified)
		m.post(func() {
			if m.usagePopup != state || state.rowGen != rowGen {
				return
			}
			state.lineTexts = texts
			state.rows = buildUsagePopupRows(locs, classified, texts)
			if state.active >= len(m.usagePopupVisibleRows()) {
				state.active = max(0, len(m.usagePopupVisibleRows())-1)
			}
			m.refreshUsagePopupPreview()
			m.invalidate()
		})
	}()
}

// buildUsagePopupRows follows OpenIDEA's groupUsages: deduplicate exact ranges,
// sort by path and position, prefer a source line and retain semantic metadata.
func buildUsagePopupRows(
	locs []openidea.Location,
	classified []openidea.ClassifiedReference,
	lineTexts map[string]string,
) []usagePopupRow {
	byLocation := make(map[string]openidea.ClassifiedReference, len(classified))
	for _, item := range classified {
		byLocation[usageLocationKey(item.Location)] = item
	}
	seen := make(map[string]bool, len(locs))
	rows := make([]usagePopupRow, 0, len(locs))
	for _, location := range locs {
		key := usageLocationKey(location)
		if seen[key] {
			continue
		}
		seen[key] = true
		meta := byLocation[key]
		text, supplied := lineTexts[key]
		if !supplied && meta.Text != nil {
			text = *meta.Text
		}
		kind := meta.Kind
		if kind == "" {
			kind = "unknown"
		}
		endCol := location.Range.End.Col
		if location.Range.End.Line != location.Range.Start.Line {
			endCol = max(location.Range.Start.Col, utf16Length(text))
		}
		rows = append(rows, usagePopupRow{
			source: location, kind: kind, access: meta.Access, text: text, endCol: endCol,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].source, rows[j].source
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line < b.Range.Start.Line
		}
		return a.Range.Start.Col < b.Range.Start.Col
	})
	return rows
}

func usageLocationKey(location openidea.Location) string {
	uri := ""
	if location.URI != nil {
		uri = *location.URI
	}
	return fmt.Sprintf("%s\x00%s\x00%d:%d-%d:%d",
		location.Path, uri,
		location.Range.Start.Line, location.Range.Start.Col,
		location.Range.End.Line, location.Range.End.Col)
}

func usageClassifiedLines(
	locs []openidea.Location,
	classified []openidea.ClassifiedReference,
) map[string]string {
	byLocation := make(map[string]openidea.ClassifiedReference, len(classified))
	for _, item := range classified {
		byLocation[usageLocationKey(item.Location)] = item
	}
	out := make(map[string]string)
	for _, location := range locs {
		if item, ok := byLocation[usageLocationKey(location)]; ok && item.Text != nil {
			out[usageLocationKey(location)] = *item.Text
		}
	}
	return out
}

// loadUsageLineTexts reads each referenced source once and returns the exact
// preview line for every location the server did not already annotate.
func loadUsageLineTexts(
	locs []openidea.Location,
	classified []openidea.ClassifiedReference,
) map[string]string {
	given := usageClassifiedLines(locs, classified)
	type wantedLine struct {
		locations []string
	}
	byPath := make(map[string]map[int]*wantedLine)
	for _, location := range locs {
		key := usageLocationKey(location)
		if _, ok := given[key]; ok || strings.Contains(location.Path, "://") {
			continue
		}
		path := location.Path
		line := int(location.Range.Start.Line)
		if byPath[path] == nil {
			byPath[path] = make(map[int]*wantedLine)
		}
		wanted := byPath[path][line]
		if wanted == nil {
			wanted = &wantedLine{}
			byPath[path][line] = wanted
		}
		wanted.locations = append(wanted.locations, key)
	}
	out := make(map[string]string, len(given))
	for key, text := range given {
		out[key] = text
	}
	for path, wanted := range byPath {
		if len(out) >= maxUsages {
			break
		}
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
		line := 0
		last := 0
		for target := range wanted {
			last = max(last, target)
		}
		for scanner.Scan() && line <= last {
			if item := wanted[line]; item != nil {
				text := strings.TrimSuffix(scanner.Text(), "\r")
				for _, key := range item.locations {
					out[key] = text
				}
			}
			line++
		}
		_ = file.Close()
	}
	return out
}

func usageSupportsAccess(rows []usagePopupRow) bool {
	for _, row := range rows {
		if row.access != "" && row.access != "unknown" {
			return true
		}
	}
	return false
}

func usageAccessVisible(access string, prefs UsagesPrefs) bool {
	switch access {
	case "read":
		return prefs.ShowReadAccess()
	case "write":
		return prefs.ShowWriteAccess()
	default:
		// Unknown, readWrite and ordinary LSP locations remain visible under
		// either single-access mode.
		return true
	}
}

func (m *Model) usagePopupVisibleRows() []usagePopupRow {
	state := m.usagePopup
	if state == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(state.filter))
	out := make([]usagePopupRow, 0, len(state.rows))
	for _, row := range state.rows {
		if !usageAccessVisible(row.access, m.prefs.Usages) {
			continue
		}
		if query != "" {
			path := usageDisplayBase(row.source.Path)
			if !strings.Contains(strings.ToLower(path), query) &&
				!strings.Contains(strings.ToLower(row.text), query) {
				continue
			}
		}
		out = append(out, row)
	}
	return out
}

func (m *Model) usagePopupActiveRow() *usagePopupRow {
	rows := m.usagePopupVisibleRows()
	state := m.usagePopup
	if state == nil || len(rows) == 0 {
		return nil
	}
	state.active = min(max(state.active, 0), len(rows)-1)
	return &rows[state.active]
}

func (m *Model) stepUsagePopup(key ui.Key) {
	state := m.usagePopup
	rows := m.usagePopupVisibleRows()
	if state == nil || len(rows) == 0 {
		return
	}
	switch key {
	case ui.KeyDown:
		state.active = (max(state.active, -1) + 1) % len(rows)
	case ui.KeyUp:
		if state.active <= 0 {
			state.active = len(rows) - 1
		} else {
			state.active--
		}
	case ui.KeyPageDown:
		state.active = min(len(rows)-1, max(0, state.active+10))
	case ui.KeyPageUp:
		state.active = max(0, state.active-10)
	case ui.KeyHome:
		state.active = 0
	case ui.KeyEnd:
		state.active = len(rows) - 1
	default:
		return
	}
	m.ensureUsagePopupActive()
	m.refreshUsagePopupPreview()
	m.invalidate()
}

func (m *Model) ensureUsagePopupActive() {
	state := m.usagePopup
	if state == nil {
		return
	}
	top := float32(state.active * usagePopupRowHeight)
	visible := float32(min(usagePopupRows, max(1, len(m.usagePopupVisibleRows()))) * usagePopupRowHeight)
	switch {
	case top < state.listScroll.Y:
		state.listScroll.Y = top
	case top+usagePopupRowHeight > state.listScroll.Y+visible:
		state.listScroll.Y = top + usagePopupRowHeight - visible
	}
}

func (m *Model) filterUsagePopupChanged() {
	state := m.usagePopup
	if state == nil {
		return
	}
	if state.filter == " " {
		state.filter = ""
	}
	state.active = 0
	state.listScroll.Y = 0
	m.refreshUsagePopupPreview()
	m.invalidate()
}

func (m *Model) toggleUsageAccess(which string) {
	prefs := m.prefs.Usages
	switch which {
	case "read":
		prefs.HideRead = !prefs.HideRead
	case "write":
		prefs.HideWrite = !prefs.HideWrite
	default:
		return
	}
	if prefs.HideRead && prefs.HideWrite {
		if which == "read" {
			prefs.HideWrite = false
		} else {
			prefs.HideRead = false
		}
	}
	m.prefs.Usages = prefs
	m.savePreferences()
	if state := m.usagePopup; state != nil {
		state.active = 0
		state.listScroll.Y = 0
		m.refreshUsagePopupPreview()
	}
	m.ensureUsageSelection()
	m.invalidate()
}

func (m *Model) toggleUsagePopupPreview() {
	m.prefs.Usages.PopupPreview = !m.prefs.Usages.PopupPreview
	if !m.prefs.Usages.PopupPreview {
		stopUsagePopupPreview(m.usagePopup)
	}
	m.savePreferences()
	m.refreshUsagePopupPreview()
	m.invalidate()
}

// SetUsagePopupScope searches the same target again in scope.
func (m *Model) SetUsagePopupScope(scope string) {
	state := m.usagePopup
	if state == nil || scope == state.scope || (scope != usageScopeProject && scope != usageScopeLibraries) {
		return
	}
	state.canWiden = true
	m.searchUsagePopup(state.target, scope == usageScopeLibraries, true, state)
}

func (m *Model) pickUsagePopupRow(row usagePopupRow) {
	if strings.Contains(row.source.Path, "://") {
		m.status = "The usage is in an external dependency and cannot open as a file"
		m.invalidate()
		return
	}
	m.dismissUsagePopup()
	m.revealAt(row.source.Path, int(row.source.Range.Start.Line), int(row.source.Range.Start.Col))
}

func (m *Model) pickActiveUsagePopup() {
	row := m.usagePopupActiveRow()
	if row != nil {
		m.pickUsagePopupRow(*row)
	}
}

// OpenUsageResultsInFindWindow transfers the current popup results, including
// semantic ranges and access metadata, without running another search.
func (m *Model) OpenUsageResultsInFindWindow() {
	state := m.usagePopup
	if state == nil || !state.open {
		return
	}
	locs := make([]openidea.Location, 0, len(state.rows))
	classified := make([]openidea.ClassifiedReference, 0, len(state.rows))
	for _, row := range state.rows {
		locs = append(locs, row.source)
		classified = append(classified, openidea.ClassifiedReference{
			Location: row.source, Kind: row.kind, Access: row.access,
			Text: usageStringPointer(row.text),
		})
	}
	note := state.note
	symbol := state.symbol
	if state.channel != 0 {
		m.cancelUsageChannel(state.channel)
	}
	state.generation++
	state.channel = 0
	state.loading = false
	stopUsagePopupPreview(state)
	m.usagePopup = nil
	m.showUsageResults(symbol, locs, classified, note)
}

func usageStringPointer(value string) *string { return &value }

func (m *Model) refreshUsagePopupPreview() {
	state := m.usagePopup
	if state == nil || !state.open || !m.prefs.Usages.PopupPreview {
		return
	}
	row := m.usagePopupActiveRow()
	if row == nil {
		state.previewGen++
		stopUsagePopupPreview(state)
		state.preview = usagePopupPreview{}
		return
	}
	key := usageLocationKey(row.source)
	if state.preview.key == key && (state.preview.loading || state.preview.err != "" || len(state.preview.lines) > 0) {
		return
	}
	if strings.Contains(row.source.Path, "://") {
		state.previewGen++
		stopUsagePopupPreview(state)
		state.preview = usagePopupPreview{
			key: key, path: row.source.Path, line: int(row.source.Range.Start.Line),
			col: row.source.Range.Start.Col, endCol: row.endCol, err: "Source is not available for this dependency",
		}
		return
	}
	state.previewGen++
	gen := state.previewGen
	stopUsagePopupPreview(state)
	state.preview = usagePopupPreview{
		key: key, path: row.source.Path, line: int(row.source.Range.Start.Line),
		col: row.source.Range.Start.Col, endCol: row.endCol, loading: true,
	}
	path, line := row.source.Path, int(row.source.Range.Start.Line)
	state.previewTimer = time.AfterFunc(usagePopupPreviewDelay, func() {
		m.post(func() {
			if m.usagePopup != state || state.previewGen != gen ||
				!state.open || !m.prefs.Usages.PopupPreview {
				return
			}
			state.previewTimer = nil
			go func() {
				start, lines, err := readUsagePreview(path, line, usagePopupPreviewTop, usagePopupPreviewEnd)
				m.post(func() {
					if m.usagePopup != state || state.previewGen != gen {
						return
					}
					state.preview.loading = false
					state.preview.start, state.preview.lines = start, lines
					if err != nil {
						state.preview.err = err.Error()
					}
					m.invalidate()
				})
			}()
		})
	})
}

func stopUsagePopupPreview(state *usagePopupState) {
	if state == nil || state.previewTimer == nil {
		return
	}
	state.previewTimer.Stop()
	state.previewTimer = nil
}

func readUsagePreview(path string, line, before, after int) (int, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer file.Close()
	start := max(0, line-before)
	end := line + after
	lines := make([]string, 0, end-start+1)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	current := 0
	for scanner.Scan() && current <= end {
		if current >= start {
			lines = append(lines, strings.TrimSuffix(scanner.Text(), "\r"))
		}
		current++
	}
	if err := scanner.Err(); err != nil {
		return start, nil, err
	}
	return start, lines, nil
}

func usageDisplayBase(path string) string {
	if strings.Contains(path, "://") {
		trimmed := strings.TrimRight(path, "/")
		if i := strings.LastIndex(trimmed, "/"); i >= 0 {
			return trimmed[i+1:]
		}
	}
	return filepath.Base(path)
}

func utf16Length(text string) uint32 {
	n := 0
	for _, r := range text {
		n += utf16.RuneLen(r)
	}
	return uint32(n)
}

func utf16ColumnByteOffset(text string, column uint32) int {
	if column == 0 {
		return 0
	}
	units := uint32(0)
	for offset, r := range text {
		if units >= column {
			return offset
		}
		width := utf16.RuneLen(r)
		if units+uint32(width) > column {
			return offset
		}
		units += uint32(width)
	}
	return len(text)
}

func usagePreviewText(row usagePopupRow) (string, int, int) {
	text := row.text
	leadBytes := len(text) - len(strings.TrimLeft(text, " \t\r\n"))
	trimmed := strings.TrimSpace(text)
	leading := utf16Length(text[:leadBytes])
	from := int(row.source.Range.Start.Col)
	if from > int(leading) {
		from -= int(leading)
	} else {
		from = 0
	}
	to := int(row.endCol)
	if to > int(leading) {
		to -= int(leading)
	} else {
		to = 0
	}
	from = min(max(from, 0), utf8.RuneCountInString(trimmed))
	to = min(max(to, from), utf8.RuneCountInString(trimmed))
	return trimmed, utf16ColumnByteOffset(trimmed, uint32(from)), utf16ColumnByteOffset(trimmed, uint32(to))
}

func usageHighlightedSpans(text string, from, to int, oi oiColors) []ui.Span {
	from = min(max(from, 0), len(text))
	to = min(max(to, from), len(text))
	spans := make([]ui.Span, 0, 3)
	if from > 0 {
		spans = append(spans, ui.Span{Text: text[:from], Color: oi.text})
	}
	if to > from {
		spans = append(spans, ui.Span{
			Text: text[from:to], Color: oi.textStrong, Weight: 600, Background: oi.selected, Radius: 3,
		})
	}
	if to < len(text) {
		spans = append(spans, ui.Span{Text: text[to:], Color: oi.text})
	}
	if len(spans) == 0 {
		spans = append(spans, ui.Span{Text: text, Color: oi.text})
	}
	return spans
}

// usagesPopup draws the registered Show Usages overlay.
func (m *Model) usagesPopup(c *ui.Context, p *ui.Theme) {
	state := m.usagePopup
	if state == nil || !state.open {
		return
	}
	rows := m.usagePopupVisibleRows()
	if len(rows) == 0 {
		state.active = -1
	} else {
		state.active = min(max(state.active, 0), len(rows)-1)
	}
	m.refreshUsagePopupPreview()

	oi := oiPaletteOf(p)
	width, height := c.Size()
	preview := m.prefs.Usages.PopupPreview
	w := max(float32(420), min(float32(900), width-24))
	if !preview {
		w = max(float32(420), min(float32(700), width-24))
	}
	bodyRows := min(usagePopupRows, max(1, len(rows)))
	h := min(float32(620), float32(80+bodyRows*usagePopupRowHeight))
	if preview {
		h = min(float32(620), max(h, 430))
	}
	x := float32(24)
	y := float32(64)
	if state.anchor.W > 0 {
		x = state.anchor.X
		y = state.anchor.Y + max(18, state.anchor.H+4)
	}
	x = max(12, min(x, width-w-12))
	if y+h > height-12 {
		if state.anchor.Y > h+12 {
			y = state.anchor.Y - h - 4
		} else {
			y = max(12, height-h-12)
		}
	}

	ui.Overlay(c, func() {
		box := ui.Column(c.Key("code:usages-popup")).Role(ui.RolePopup).Label("Usages").
			Focusable().AutoFocus().Absolute().Left(x).Top(y).Width(w).Height(h).
			MinWidth(0).Clip().Background(oi.popup).Border(1, oi.borderStrong).Radius(7).
			Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.32))
		box.Children(func() {
			m.usagePopupHeader(c, p, oi, state)
			if usageSupportsAccess(state.rows) {
				m.usagePopupAccessRow(c, oi)
			}
			if state.note != "" {
				ui.Row(c).Shrink(0).Padding(3, 10).Children(func() {
					ui.Text(c, state.note).FontSize(11).TextColor(oi.warning).
						SingleLine().Ellipsis("…")
				})
			}
			ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
				m.usagePopupList(c, p, oi, state, rows)
				if preview {
					m.usagePopupPreview(c, p, oi, state)
				}
			})
		})
		switch {
		case box.PressedOutside():
			m.dismissUsagePopup()
		case box.OverlayShortcut(0, ui.KeyEscape):
			m.escapeUsagePopup()
		}
	})
}

func (m *Model) usagePopupHeader(c *ui.Context, p *ui.Theme, oi oiColors, state *usagePopupState) {
	rows := m.usagePopupVisibleRows()
	total := len(state.rows)
	shown := len(rows)
	count := fmt.Sprintf("%d found", total)
	if shown != total || strings.TrimSpace(state.filter) != "" {
		count = fmt.Sprintf("%d of %d found", shown, total)
	}
	ui.Row(c).Shrink(0).Height(34).Padding(0, 9).Gap(7).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		title := "Usages"
		if state.symbol != "" {
			title = "Usages of " + state.symbol
		}
		ui.Text(c, title).FontSize(12).Bold().TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
		ui.Text(c, count).FontSize(11).TextColor(oi.textMuted).SingleLine()
		if state.scope != "" {
			scopeLabel := usageScopeLabel(state.scope)
			choice := ui.Select(c, &scopeLabel,
				[]string{usageScopeLabel(usageScopeProject), usageScopeLabel(usageScopeLibraries)}).
				Width(150).Label("Scope")
			if choice.Changed() {
				if scopeLabel == usageScopeLabel(usageScopeLibraries) {
					m.SetUsagePopupScope(usageScopeLibraries)
				} else {
					m.SetUsagePopupScope(usageScopeProject)
				}
			}
		}
		if state.loading {
			ui.Text(c, "Searching…").FontSize(11).TextColor(oi.textMuted).SingleLine()
		}
		if state.filter != "" {
			color := oi.link
			if shown == 0 {
				color = oi.warning
			}
			ui.Row(c).Padding(2, 6).Radius(4).Background(oi.elev).Children(func() {
				ui.Text(c, state.filter).FontSize(11).TextColor(color).SingleLine()
			})
		}
		ui.Spacer(c)
		m.usagePopupToggleButton(c, oi, "Preview", m.prefs.Usages.PopupPreview, m.toggleUsagePopupPreview)
		open := ui.ButtonBase(c.Key("usages:open-find")).Height(24).Padding(0, 5).Radius(4).
			AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Tooltip("Open in Find Window (" + m.usageFindShortcut() + ")")
		if open.Hovered() {
			open.Background(oi.toolbarHover)
		}
		open.Children(func() {
			label := "Open in Find Window"
			if shortcut := m.usageFindShortcut(); shortcut != "" {
				label += " " + shortcut
			}
			ui.Text(c, label).FontSize(11).TextColor(oi.link).SingleLine()
		})
		if open.Clicked() {
			m.OpenUsageResultsInFindWindow()
		}

		// The speed-search text stays visually part of the header, as in
		// OpenIDEA, while the tiny input owns text composition and undo.
		input := ui.TextInputBase(c.Key("usages:filter"), &state.filter).Size(1, 1).
			Opacity(0).AutoFocus().Label("Filter usages")
		input.OnChange(m.filterUsagePopupChanged)
		input.OnTextKey(func(k ui.TextKey) bool {
			if k.Mods != 0 {
				return false
			}
			switch k.Key {
			case ui.KeyEscape:
				if state.filter != "" {
					k.SetText("")
					state.filter = ""
					state.active = 0
					state.listScroll.Y = 0
					m.invalidate()
				} else {
					m.dismissUsagePopup()
				}
				return true
			case ui.KeyEnter:
				m.pickActiveUsagePopup()
				return true
			case ui.KeyUp, ui.KeyDown, ui.KeyPageUp, ui.KeyPageDown, ui.KeyHome, ui.KeyEnd:
				m.stepUsagePopup(k.Key)
				return true
			case ui.KeyBackspace:
				return state.filter == ""
			}
			return false
		})
		input.OnShortcut(ui.Alt, ui.KeyF7, m.OpenUsageResultsInFindWindow)
	})
}

func usageScopeLabel(scope string) string {
	if scope == usageScopeLibraries {
		return "Project and Libraries"
	}
	return "Project Files"
}

func (m *Model) usagePopupToggleButton(
	c *ui.Context, oi oiColors, label string, pressed bool, run func(),
) {
	button := ui.ButtonBase(c.Key("usages-toggle:"+label)).Height(22).Padding(0, 7).Radius(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip("Show " + label)
	if pressed {
		button.Background(oi.selected)
	} else if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Children(func() {
		color := oi.textMuted
		if pressed {
			color = oi.textStrong
		}
		ui.Text(c, label).FontSize(11).TextColor(color).SingleLine()
	})
	if button.Clicked() {
		run()
	}
}

func (m *Model) usagePopupAccessRow(c *ui.Context, oi oiColors) {
	ui.Row(c).Shrink(0).Height(28).Padding(0, 8).Gap(5).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Text(c, "Access").FontSize(10).TextColor(oi.textMuted).SingleLine()
		m.usagePopupAccessButton(c, oi, "Read", m.prefs.Usages.ShowReadAccess(), func() {
			m.toggleUsageAccess("read")
		})
		m.usagePopupAccessButton(c, oi, "Write", m.prefs.Usages.ShowWriteAccess(), func() {
			m.toggleUsageAccess("write")
		})
		ui.Spacer(c)
		ui.Text(c, "Space or Type to search · ↑↓ navigate · Esc clears or closes").
			FontSize(10).TextColor(oi.textSubtle).SingleLine()
	})
}

func (m *Model) usagePopupAccessButton(
	c *ui.Context, oi oiColors, label string, pressed bool, run func(),
) {
	button := ui.ButtonBase(c.Key("usages-access:"+label)).Height(20).Padding(0, 6).Radius(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Tooltip("Show " + strings.ToLower(label) + " accesses")
	if pressed {
		button.Background(oi.selected)
	} else if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Children(func() {
		color := oi.textSubtle
		if pressed {
			color = oi.text
		}
		ui.Text(c, label).FontSize(10).TextColor(color).SingleLine()
	})
	if button.Clicked() {
		run()
	}
}

func (m *Model) usagePopupList(
	c *ui.Context, p *ui.Theme, oi oiColors, state *usagePopupState, rows []usagePopupRow,
) {
	if len(rows) == 0 {
		text := "No usages match the access filters"
		if strings.TrimSpace(state.filter) != "" {
			text = fmt.Sprintf("Nothing matches '%s'", state.filter)
		}
		ui.Row(c).Grow(1).Padding(8, 10).Children(func() {
			ui.Text(c, text).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		})
		return
	}
	height := float32(min(usagePopupRows, len(rows)) * usagePopupRowHeight)
	ui.Scroll(c.Key("usages:list")).Height(height).Grow(1).MinWidth(0).
		TrackScroll(&state.listScroll).Children(func() {
		first := max(0, int(state.listScroll.Y)/usagePopupRowHeight-1)
		last := min(len(rows), first+usagePopupRows+3)
		ui.Column(c).MinWidth(0).Children(func() {
			if first > 0 {
				ui.Spacer(c).Height(float32(first * usagePopupRowHeight))
			}
			for index := first; index < last; index++ {
				index := index
				row := rows[index]
				el := ui.Row(c.Key("usage-row:"+usageLocationKey(row.source))).
					Height(usagePopupRowHeight).Padding(0, 7).Gap(6).AlignItems(ui.Center)
				if index == state.active || el.Hovered() {
					el.Background(oi.selected)
				}
				el.Children(func() {
					fileIconEl(c, oi, row.source.Path, 16, oi.textMuted)
					text, from, to := usagePreviewText(row)
					if text == "" {
						text = fmt.Sprintf("[%d:%d]", row.source.Range.Start.Line+1, row.source.Range.Start.Col+1)
					}
					ui.RichText(c, usageHighlightedSpans(text, from, to, oi)...).
						Font("monospace").FontSize(11).SingleLine().Grow(1)
					ui.Text(c, fmt.Sprintf("%s:%d", usageDisplayBase(row.source.Path),
						row.source.Range.Start.Line+1)).
						FontSize(10).TextColor(oi.textMuted).SingleLine()
				})
				if el.Hovered() && state.active != index {
					state.active = index
					m.refreshUsagePopupPreview()
				}
				if el.Clicked() {
					state.active = index
					m.pickUsagePopupRow(row)
				}
			}
			if tail := len(rows) - last; tail > 0 {
				ui.Spacer(c).Height(float32(tail * usagePopupRowHeight))
			}
		})
	})
}

func (m *Model) usagePopupPreview(c *ui.Context, p *ui.Theme, oi oiColors, state *usagePopupState) {
	ui.Column(c).Width(340).Shrink(0).MinWidth(0).MinHeight(0).
		BorderWidth(0, 0, 0, 1).BorderColor(oi.border).Children(func() {
		ui.Row(c).Height(26).Shrink(0).Padding(0, 8).AlignItems(ui.Center).
			Background(oi.elev).Children(func() {
			ui.Text(c, "Preview").FontSize(10).Bold().TextColor(oi.textMuted).SingleLine()
			ui.Spacer(c)
			if state.preview.path != "" {
				ui.Text(c, usageDisplayBase(state.preview.path)).FontSize(10).
					TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
			}
		})
		switch {
		case state.preview.loading:
			ui.Row(c).Padding(8, 10).Children(func() {
				ui.Text(c, "Loading preview…").FontSize(11).TextColor(oi.textMuted)
			})
		case state.preview.err != "":
			ui.Row(c).Padding(8, 10).Children(func() {
				ui.Text(c, state.preview.err).FontSize(11).TextColor(oi.warning).SingleLine().Ellipsis("…")
			})
		case len(state.preview.lines) > 0:
			ui.ScrollBoth(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(5, 0).Children(func() {
					for i, line := range state.preview.lines {
						lineNo := state.preview.start + i
						row := ui.Row(c.Key(fmt.Sprintf("usage-preview:%d", lineNo))).
							Height(18).Padding(0, 5).Gap(8).AlignItems(ui.Center)
						if lineNo == state.preview.line {
							row.Background(oi.hoverTint)
						}
						row.Children(func() {
							ui.Text(c, fmt.Sprintf("%d", lineNo+1)).Width(42).
								Font("monospace").FontSize(10).TextColor(oi.textSubtle)
							if lineNo == state.preview.line {
								row := usagePopupRow{
									source: openidea.Location{
										Path: state.preview.path,
										Range: openidea.OutlineRange{
											Start: openidea.OutlinePos{Line: uint32(lineNo), Col: state.preview.col},
											End:   openidea.OutlinePos{Line: uint32(lineNo), Col: state.preview.endCol},
										},
									},
									text: line, endCol: state.preview.endCol,
								}
								text, from, to := usagePreviewText(row)
								ui.RichText(c, usageHighlightedSpans(text, from, to, oi)...).
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
				ui.Text(c, "Choose a usage to preview its source").FontSize(11).TextColor(oi.textMuted)
			})
		}
	})
}
