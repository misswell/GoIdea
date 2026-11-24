package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// SearchPane is the Find window's project search: the query, the files the
// backend streamed back, and whether the search is still running.
type SearchPane struct {
	visible bool
	query   string
	// replaceMode shows the replacement field and enables Replace in Files.
	replaceMode   bool
	replacement   string
	preserveCase  bool
	isRegex       bool
	caseSensitive bool
	wholeWord     bool
	include       string
	exclude       string
	options       bool
	// scopeName selects a named custom scope. scopePath, when set, limits the
	// search to one directory and takes precedence.
	scopeName string

	channel uint32
	request uint32
	running bool
	// done is set by the summary; cancelled says the user stopped it early.
	done      bool
	cancelled bool
	// scopePath limits the search to one folder; empty means the whole project.
	scopePath string
	summary   string
	files     []openidea.FileMatches
	matches   int
	truncated bool
	err       string

	// selectedFile and selectedMatch choose one result for Replace.
	selectedFile  int
	selectedMatch int
	// historyOpen shows the multi-line field's recent searches. The stored
	// switch is inverse so the zero value keeps OpenIDEA's default enabled.
	historyOpen       bool
	noScrollToResults bool
	scrollSelected    bool
	history           []string

	replaceChannel  uint32
	replaceRequest  uint32
	replaceRunning  bool
	replaceDone     bool
	replaceProgress string
	replaceSummary  string
	replaceErr      string

	scopesOpen     bool
	scopesDraft    []SearchScope
	scopesSelected int
	scopesChoice   string
	scopesErr      string
}

const (
	maxSearchFiles   = 200
	maxSearchMatches = 2000
)

func (m *Model) resetSearch() { m.search = SearchPane{} }

// ShowSearch opens OpenIDEA's Find window in the bottom side: Enter in the
// query field runs it.
func (m *Model) ShowSearch() {
	if len(m.search.history) == 0 {
		m.search.history = append([]string(nil), m.prefs.SearchFieldHistory...)
	}
	m.search.scopePath = ""
	if m.search.scopeName == "" {
		m.search.scopeName = "Project"
	}
	m.DockShow("find")
}

// ShowSearchInFolder opens the Find window limited to dir, as Find/Replace in
// Folder does.
func (m *Model) ShowSearchInFolder(dir string) {
	if dir == "" {
		return
	}
	m.search.scopePath = dir
	m.search.scopeName = "Project"
	m.DockShow("find")
}

// ShowFind opens Find in Files or Replace in Files. The argument accepts the
// same query/directory/path fields as OpenIDEA's find.inPath action.
func (m *Model) ShowFind(replace bool, arg any) {
	if len(m.search.history) == 0 {
		m.search.history = append([]string(nil), m.prefs.SearchFieldHistory...)
	}
	request := searchActionObject(arg)
	if query, _ := request["query"].(string); query != "" {
		m.search.query = query
	} else if selected := m.activeSelection(); selected != "" {
		m.search.query = selected
	}
	for _, key := range []string{"directory", "path"} {
		if path, _ := request[key].(string); path != "" {
			m.search.scopePath = path
			break
		}
	}
	m.search.replaceMode = replace
	if m.search.scopeName == "" {
		m.search.scopeName = "Project"
	}
	m.DockShow("find")
}

func (m *Model) activeSelection() string {
	t := m.tab()
	if t == nil || t.state == nil {
		return ""
	}
	return strings.TrimSpace(t.state.SelectedText())
}

// RerunSearch repeats the current Find query.
func (m *Model) RerunSearch() {
	if m.search.query == "" || m.search.running || m.search.replaceRunning {
		return
	}
	m.RunSearch()
}

// AddNewLineToFindField implements find.field.newLine. A search field is
// multi-line in OpenIDEA, so Enter runs the search and this action inserts a
// line break without falling back to a shortcut for the surrounding editor.
func (m *Model) AddNewLineToFindField() {
	if m.Root() == "" {
		return
	}
	m.search.query += "\n"
	m.ShowSearch()
	m.searchQueryChanged()
	m.invalidate()
}

// ToggleRecentSearchFields opens the recent multi-line queries beside the
// Find field. Selecting a row restores the exact query, including line breaks.
func (m *Model) ToggleRecentSearchFields() {
	if len(m.search.history) == 0 {
		m.search.history = append([]string(nil), m.prefs.SearchFieldHistory...)
	}
	m.search.historyOpen = !m.search.historyOpen
	m.invalidate()
}

// UseRecentSearchField restores one query from the Find field's history.
func (m *Model) UseRecentSearchField(index int) {
	s := &m.search
	if index < 0 || index >= len(s.history) {
		return
	}
	s.query = s.history[index]
	s.historyOpen = false
	m.searchQueryChanged()
	m.invalidate()
}

// ToggleFindScrollToResults switches OpenIDEA's default-on preference that
// follows the nearest result while the query changes.
func (m *Model) ToggleFindScrollToResults() {
	m.search.noScrollToResults = !m.search.noScrollToResults
	if !m.search.noScrollToResults {
		m.search.scrollSelected = len(m.search.files) > 0
	}
	m.invalidate()
}

// FindScrollToResults reports the visible checkmark for find.scrollToResults.
func (m *Model) FindScrollToResults() bool {
	return !m.search.noScrollToResults
}

// searchQueryChanged marks the selected hit for reveal on the next results
// frame when Scroll to Results During Typing is on.
func (m *Model) searchQueryChanged() {
	if !m.search.noScrollToResults {
		m.search.scrollSelected = true
	}
}

// rememberSearchFieldQuery records the exact multi-line query, newest first.
func (m *Model) rememberSearchFieldQuery(query string) {
	query = strings.TrimRight(query, "\r\n")
	if query == "" {
		return
	}
	s := &m.search
	out := []string{query}
	for _, old := range s.history {
		if old == query || len(out) >= 20 {
			continue
		}
		out = append(out, old)
	}
	s.history = out
	m.prefs.SearchFieldHistory = append([]string(nil), out...)
	m.savePreferences()
}

// FindInFolderSelected scopes Find to the folder selected in the project view,
// or to the project when the selection is not a folder.
func (m *Model) FindInFolderSelected() {
	dir := m.Root()
	if m.tree != nil {
		if sel := m.tree.Selected(); m.tree.IsDir(sel) {
			dir = sel
		}
	}
	if dir == m.Root() {
		m.ShowSearch()
		return
	}
	m.ShowSearchInFolder(dir)
}

// RunSearch starts a project-wide search, replacing any running one.
func (m *Model) RunSearch() {
	pattern := strings.TrimSpace(m.search.query)
	m.cancelSearch()
	if pattern == "" || m.Root() == "" || m.search.replaceRunning {
		m.search.files = nil
		m.search.matches = 0
		m.search.summary = ""
		m.search.err = ""
		m.search.done = false
		m.invalidate()
		return
	}
	m.rememberSearchFieldQuery(pattern)
	ch := m.nextChannel()
	m.search.channel = ch
	m.search.request = ch
	m.search.running = true
	m.search.done = false
	m.search.cancelled = false
	m.search.files = nil
	m.search.matches = 0
	m.search.truncated = false
	m.search.selectedFile = 0
	m.search.selectedMatch = 0
	m.search.scrollSelected = !m.search.noScrollToResults
	m.search.summary = ""
	m.search.err = ""
	m.invalidate()
	root := m.Root()
	query := m.searchQueryMap()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		if _, err := m.invoke(ctx, "search_validate", map[string]any{"query": query}); err != nil {
			m.post(func() {
				if m.search.request != ch {
					return
				}
				m.search.running = false
				m.search.err = "Search pattern is invalid: " + err.Error()
				m.invalidate()
			})
			return
		}
		_, err := m.invoke(ctx, "search_start", map[string]any{
			"request": ch,
			"query":   query,
			"onEvent": map[string]any{"$channel": ch},
		})
		m.post(func() {
			if m.search.request != ch {
				return
			}
			if err != nil && m.search.err == "" {
				m.search.running = false
				m.search.err = "Searching failed: " + err.Error()
			} else if m.search.done {
				m.search.running = false
			}
			if m.Root() != root {
				m.search.files = nil
			}
			m.invalidate()
		})
	}()
}

func (m *Model) searchQueryMap() map[string]any {
	p := &m.search
	scope := map[string]any{"kind": "project"}
	switch {
	case p.scopePath != "":
		scope = map[string]any{"kind": "directory", "path": p.scopePath}
	case p.scopeName != "" && p.scopeName != "Project":
		for _, custom := range m.searchScopes {
			if custom.Name == p.scopeName {
				scope = map[string]any{"kind": "pattern", "pattern": custom.Pattern}
				break
			}
		}
	}
	return map[string]any{
		"pattern":       strings.TrimSpace(p.query),
		"isRegex":       p.isRegex,
		"caseSensitive": p.caseSensitive,
		"wholeWord":     p.wholeWord,
		"include":       searchMaskList(p.include),
		"exclude":       searchMaskList(p.exclude),
		"scope":         scope,
		"maxMatches":    maxSearchMatches,
	}
}

func searchMaskList(text string) []string {
	out := []string{}
	for _, part := range strings.Split(text, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// cancelSearch stops a running search; its summary then reports cancelled.
func (m *Model) cancelSearch() {
	if !m.search.running || m.search.request == 0 {
		return
	}
	id := m.search.request
	m.search.running = false
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": id})
	}()
}

// acceptSearchEvent consumes one streamed search message.
func (m *Model) acceptSearchEvent(data json.RawMessage) {
	var ev openidea.SearchEvent
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	switch ev.Type {
	case "file":
		if ev.File == nil || len(ev.File.Matches) == 0 {
			return
		}
		if len(m.search.files) >= maxSearchFiles || m.search.matches >= maxSearchMatches {
			m.search.truncated = true
			return
		}
		m.search.files = append(m.search.files, *ev.File)
		m.search.matches += len(ev.File.Matches)
	case "done":
		m.search.running = false
		m.search.done = true
		if ev.Summary != nil {
			m.search.cancelled = ev.Summary.Cancelled
			m.search.summary = fmt.Sprintf("%s, %s, %d ms",
				count(int(ev.Summary.Files), "file", "files"),
				count(int(ev.Summary.Matches), "match", "matches"), int(ev.Summary.ElapsedMs))
		}
	}
	m.invalidate()
}

// ReplaceSearchSelected replaces the selected Find result.
func (m *Model) ReplaceSearchSelected() {
	m.startReplace(false)
}

// ReplaceSearchAll replaces every result of the current Find query.
func (m *Model) ReplaceSearchAll() {
	m.startReplace(true)
}

func (m *Model) startReplace(all bool) {
	s := &m.search
	if !s.replaceMode || s.query == "" || s.replaceRunning || len(s.files) == 0 || m.Root() == "" {
		return
	}
	var targets []openidea.ReplaceTarget
	if all {
		targets = make([]openidea.ReplaceTarget, 0, len(s.files))
		for _, file := range s.files {
			targets = append(targets, openidea.ReplaceTarget{Path: file.Path})
		}
	} else {
		if s.selectedFile < 0 || s.selectedFile >= len(s.files) {
			s.replaceErr = "Choose a search result to replace"
			m.invalidate()
			return
		}
		file := s.files[s.selectedFile]
		if s.selectedMatch < 0 || s.selectedMatch >= len(file.Matches) {
			s.selectedMatch = 0
		}
		match := file.Matches[s.selectedMatch]
		col := uint32(0)
		if len(match.Ranges) > 0 {
			col = match.Ranges[0][0]
		}
		targets = []openidea.ReplaceTarget{{
			Path: file.Path,
			Only: []openidea.SearchPosition{{Line: match.Line, Col: col}},
		}}
	}
	ch := m.nextChannel()
	s.replaceChannel = ch
	s.replaceRequest = ch
	s.replaceRunning = true
	s.replaceDone = false
	s.replaceProgress = fmt.Sprintf("0 of %d files", len(targets))
	s.replaceSummary = ""
	s.replaceErr = ""
	query := m.searchQueryMap()
	replacement := s.replacement
	preserveCase := s.preserveCase
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "replace_in_files", map[string]any{
			"request":      ch,
			"query":        query,
			"replacement":  replacement,
			"preserveCase": preserveCase,
			"targets":      targets,
			"onEvent":      map[string]any{"$channel": ch},
		})
		m.post(func() {
			if m.search.replaceRequest != ch {
				return
			}
			m.search.replaceRunning = false
			if err != nil && m.search.replaceErr == "" {
				m.search.replaceErr = "Replacing failed: " + err.Error()
			}
			m.invalidate()
		})
	}()
}

// CancelReplace stops a Replace in Files run between files.
func (m *Model) CancelReplace() {
	s := &m.search
	if !s.replaceRunning || s.replaceRequest == 0 {
		return
	}
	id := s.replaceRequest
	s.replaceRunning = false
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": id})
	}()
}

func (m *Model) acceptReplaceEvent(data json.RawMessage) {
	var event openidea.ReplaceEvent
	if json.Unmarshal(data, &event) != nil {
		return
	}
	s := &m.search
	switch event.Type {
	case "file":
		s.replaceProgress = fmt.Sprintf("%d of %d files · %d replaced",
			event.Done, event.Total, event.Replacements)
	case "failed":
		s.replaceProgress = fmt.Sprintf("%d of %d files", event.Done, event.Total)
		s.replaceErr = fmt.Sprintf("%s: %s", filepath.Base(event.Path), event.Message)
	case "done":
		s.replaceRunning = false
		s.replaceDone = true
		if event.Summary != nil {
			verb := "Replaced"
			if event.Summary.Cancelled {
				verb = "Stopped after replacing"
			}
			s.replaceSummary = fmt.Sprintf("%s %d occurrences in %d files",
				verb, event.Summary.Replacements, event.Summary.Files)
			if event.Summary.Failed > 0 {
				s.replaceSummary += fmt.Sprintf(" (%d failed)", event.Summary.Failed)
			}
		}
	}
	m.invalidate()
}

// OpenSearchScopes opens the native editor for named file-pattern scopes.
func (m *Model) OpenSearchScopes() {
	m.search.scopesOpen = true
	m.search.scopesDraft = append([]SearchScope(nil), m.searchScopes...)
	m.search.scopesSelected = 0
	m.search.scopesChoice = ""
	if len(m.search.scopesDraft) > 0 {
		m.search.scopesChoice = m.search.scopesDraft[0].Name
	}
	m.search.scopesErr = ""
	m.invalidate()
}

func (m *Model) addSearchScope() {
	s := &m.search
	for index := 1; ; index++ {
		name := "Unnamed"
		if index > 1 {
			name = fmt.Sprintf("Unnamed (%d)", index)
		}
		found := false
		for _, scope := range s.scopesDraft {
			if scope.Name == name {
				found = true
				break
			}
		}
		if !found {
			s.scopesDraft = append(s.scopesDraft, SearchScope{Name: name})
			s.scopesSelected = len(s.scopesDraft) - 1
			s.scopesChoice = name
			m.invalidate()
			return
		}
	}
}

func (m *Model) removeSearchScope() {
	s := &m.search
	if s.scopesSelected < 0 || s.scopesSelected >= len(s.scopesDraft) {
		return
	}
	s.scopesDraft = append(s.scopesDraft[:s.scopesSelected], s.scopesDraft[s.scopesSelected+1:]...)
	if s.scopesSelected >= len(s.scopesDraft) {
		s.scopesSelected = max(0, len(s.scopesDraft)-1)
	}
	if len(s.scopesDraft) > 0 {
		s.scopesChoice = s.scopesDraft[s.scopesSelected].Name
	} else {
		s.scopesChoice = ""
	}
	m.invalidate()
}

func (m *Model) saveSearchScopes() {
	s := &m.search
	names := map[string]bool{}
	for index, scope := range s.scopesDraft {
		scope.Name = strings.TrimSpace(scope.Name)
		scope.Pattern = strings.TrimSpace(scope.Pattern)
		if scope.Name == "" {
			s.scopesErr = "Every scope needs a name"
			m.invalidate()
			return
		}
		if names[scope.Name] {
			s.scopesErr = fmt.Sprintf("Duplicate scope name '%s'", scope.Name)
			m.invalidate()
			return
		}
		if scope.Pattern == "" {
			s.scopesErr = fmt.Sprintf("Scope '%s' has no pattern", scope.Name)
			m.invalidate()
			return
		}
		names[scope.Name] = true
		s.scopesDraft[index] = scope
	}
	m.searchScopes = append([]SearchScope(nil), s.scopesDraft...)
	if s.scopeName != "Project" && !names[s.scopeName] {
		s.scopeName = "Project"
	}
	s.scopesOpen = false
	s.scopesErr = ""
	m.saveSession()
	m.invalidate()
}

func searchActionObject(arg any) map[string]any {
	switch value := arg.(type) {
	case map[string]any:
		return value
	case json.RawMessage:
		var out map[string]any
		if json.Unmarshal(value, &out) == nil {
			return out
		}
	case []byte:
		var out map[string]any
		if json.Unmarshal(value, &out) == nil {
			return out
		}
	}
	return map[string]any{}
}

// revealPos is a caret position to apply when its file becomes active. The
// column is UTF-16, as search hits and language servers report it; it turns
// into the editor's byte column once the tab has the document open.
type revealPos struct {
	path      string
	line, col int
	// endLine and endCol select a range; -1 puts a caret at line and col.
	endLine, endCol int
}

// revealAt opens a file in a regular tab and moves the caret to line and col
// (0-based).
func (m *Model) revealAt(path string, line, col int) {
	m.revealTo(path, line, col, false)
}

// previewAt opens a file in the preview tab and moves the caret to line and
// col, as a single click in a results list does.
func (m *Model) previewAt(path string, line, col int) {
	m.revealTo(path, line, col, true)
}

// revealTo opens path in a tab, preview or regular, and puts the caret where
// the navigation points.
func (m *Model) revealTo(path string, line, col int, preview bool) {
	path = realPath(path)
	m.recordNavigation(true)
	m.reveal = &revealPos{path: path, line: line, col: col, endLine: -1, endCol: -1}
	if preview {
		m.openPreview(path)
	} else {
		m.openFile(path)
	}
	if t := m.tab(); t != nil && t.doc.Path() == path {
		m.applyReveal(t)
	}
}

// revealRange opens path in a regular tab and selects the declaration a
// navigation points at, 0-based.
func (m *Model) revealRange(path string, line, col, endLine, endCol int) {
	path = realPath(path)
	m.recordNavigation(true)
	m.reveal = &revealPos{path: path, line: line, col: col, endLine: endLine, endCol: endCol}
	m.openFile(path)
	if t := m.tab(); t != nil && t.doc.Path() == path {
		m.applyReveal(t)
	}
}

// applyReveal moves the caret once the file a search hit points at is active.
func (m *Model) applyReveal(t *Tab) {
	if m.reveal == nil || t.doc.Path() != m.reveal.path {
		return
	}
	r := m.reveal
	m.reveal = nil
	if r.endLine >= 0 {
		start, end := t.doc.ByteRange(openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: uint32(r.line), Col: uint32(r.col)},
			End:   openidea.OutlinePos{Line: uint32(r.endLine), Col: uint32(r.endCol)},
		})
		t.state.Select(start, end)
		t.state.ScrollTo(r.line)
		m.recordNavigation(true)
		m.invalidate()
		return
	}
	line, col := t.doc.BytePos(r.line, r.col)
	t.state.GoTo(editor.Pos{Line: line, Col: col})
	t.state.ScrollTo(line)
	m.recordNavigation(true)
	m.invalidate()
}

// SmokeSearch runs one real project search and reports what streamed back.
func (m *Model) SmokeSearch(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	if m.Root() == "" {
		return out, errors.New("No project is open")
	}
	m.post(func() {
		m.search.query = "class "
		m.ShowSearch()
		m.RunSearch()
	})
	deadline := time.Now().Add(90 * time.Second)
	for {
		var done, running bool
		var files, matches int
		var summary, firstFile, firstText, errorText string
		var firstLine int
		m.post(func() {
			done, running = m.search.done, m.search.running
			files, matches, summary, errorText = len(m.search.files), m.search.matches, m.search.summary, m.search.err
			firstFile = m.search.firstRel()
			if f := m.search.firstFile(); f != nil && len(f.Matches) > 0 {
				firstLine = int(f.Matches[0].Line) + 1
				firstText = strings.TrimSpace(f.Matches[0].Text)
			}
		})
		if done || (!running && files == 0) || time.Now().After(deadline) {
			out["files"] = files
			out["matches"] = matches
			out["summary"] = summary
			out["done"] = done
			if running {
				out["timeout"] = true
			}
			out["firstFile"] = firstFile
			out["firstLine"] = firstLine
			out["firstText"] = firstText
			if errorText != "" {
				return out, errors.New(errorText)
			}
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// firstFile is the first file with hits, or nil.
func (p *SearchPane) firstFile() *openidea.FileMatches {
	if len(p.files) == 0 {
		return nil
	}
	return &p.files[0]
}

// firstRel is the first hit file's project-relative path.
func (p *SearchPane) firstRel() string {
	if f := p.firstFile(); f != nil {
		return f.RelPath
	}
	return ""
}
