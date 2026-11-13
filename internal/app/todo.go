package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// The TODO tool window ports src/features/todo: patterns scanned through the
// search RPC, grouped by file, with Project, Current File, Scope Based and
// Changes tabs. A pattern matches anywhere on a comment line, as IDEA's
// default TODO pattern does; only comments count while commentsOnly is on.

// TodoPattern is one keyword the window looks for: TODO, FIXME, HACK or XXX
// unless the settings page says otherwise.
type TodoPattern struct {
	ID      string `json:"id,omitempty"`
	Keyword string `json:"keyword"`
	// Color names a theme tone: info, danger, warning, success, violet, muted.
	Color   string `json:"color,omitempty"`
	Enabled bool   `json:"enabled,omitempty"`
}

// TodoPrefs is Settings › Editor › TODO as the preferences store it. The
// inverse names keep the zero value OpenIDEA's defaults: matching is case
// sensitive and only comments count. Patterns keeps a nil list, never
// written, apart from an empty one, where the user removed every pattern.
type TodoPrefs struct {
	Patterns   []TodoPattern `json:"patterns"`
	IgnoreCase bool          `json:"ignoreCase,omitempty"`
	AllText    bool          `json:"allText,omitempty"`
}

// sameTodoPatterns reports two pattern lists as the same keyword set.
func sameTodoPatterns(a, b []TodoPattern) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var defaultTodoPatterns = []TodoPattern{
	{ID: "todo", Keyword: "TODO", Color: "info", Enabled: true},
	{ID: "fixme", Keyword: "FIXME", Color: "danger", Enabled: true},
	{ID: "hack", Keyword: "HACK", Color: "warning", Enabled: true},
	{ID: "xxx", Keyword: "XXX", Color: "warning", Enabled: true},
}

// todoViolet is the violet tone's colour, which ui.Theme has no field for.
var todoViolet = ui.Hex("#9876aa")

// TodoItem is one keyword occurrence: its file, 0-based line and UTF-16
// column, the configured keyword and the text from the keyword to the end of
// the comment.
type TodoItem struct {
	Path    string
	Line    int
	Col     int
	Keyword string
	Text    string
}

// todoGroup is a file's matches as the window rows them.
type todoGroup struct {
	path  string
	items []TodoItem
}

// TodoPane is the tool window's state: the active tab, the keyword filters,
// the project scan and the Scope Based tab's own scan.
type TodoPane struct {
	visible bool
	// tab is "project", "file", "scope" or "changes".
	tab      string
	patterns []TodoPattern
	// caseSensitive and commentsOnly mirror upstream's TodoSettings defaults.
	caseSensitive bool
	commentsOnly  bool
	// hidden are the keywords the filter chips turned off.
	hidden map[string]bool

	channel     uint32
	request     uint32
	running     bool
	done        bool
	scannedOnce bool
	limit       bool
	err         string
	items       map[string][]TodoItem

	scopeKind    string
	scopeChannel uint32
	scopeRequest uint32
	scopeRunning bool
	scopeDone    bool
	scopeScanned string
	scopeErr     string
	scopeItems   map[string][]TodoItem

	// live caches the scan of the active buffer, which fronts the backend
	// result for that path the way upstream's live map does.
	livePath    string
	liveVersion uint64
	liveItems   []TodoItem

	// collapsed are the file groups whose rows are hidden.
	collapsed map[string]bool
	// step is the row the last Next or Previous navigation revealed.
	step int
}

const (
	maxTodoFiles = 2000
	maxTodoItems = 10000
)

// todoScopes are the Scope Based tab's built-in scopes, in OpenIDEA's order.
var todoScopes = []struct{ id, label string }{
	{"projectFiles", "项目文件"},
	{"productionFiles", "生产文件"},
	{"testFiles", "测试文件"},
	{"openFiles", "打开的文件"},
}

func (m *Model) resetTodo() {
	patterns, caseSensitive, commentsOnly := m.prefs.todoSettings()
	m.todo = TodoPane{
		tab:           "project",
		patterns:      patterns,
		caseSensitive: caseSensitive,
		commentsOnly:  commentsOnly,
		hidden:        map[string]bool{},
		collapsed:     map[string]bool{},
		scopeKind:     "projectFiles",
		step:          -1,
	}
}

// applyTodoSettings puts the stored TODO settings on the tool window: when
// they differ from the ones in use, the scans run again and every editor
// highlights the new keywords on its next frame.
func (m *Model) applyTodoSettings() {
	patterns, caseSensitive, commentsOnly := m.prefs.todoSettings()
	if sameTodoPatterns(patterns, m.todo.patterns) &&
		caseSensitive == m.todo.caseSensitive && commentsOnly == m.todo.commentsOnly {
		return
	}
	m.todo.patterns = patterns
	m.todo.caseSensitive = caseSensitive
	m.todo.commentsOnly = commentsOnly
	m.todo.step = -1
	m.todo.livePath, m.todo.liveVersion, m.todo.liveItems = "", 0, nil
	m.RunTodo()
	if m.todo.scopeScanned != "" {
		m.RunTodoScope()
	}
	for _, t := range m.tabs {
		t.todoSeen = 0
	}
	m.invalidate()
}

// todoHighlightDelay waits for typing to pause before an editor's TODO tints
// follow its buffer, as todo.highlight debounces its scan.
const todoHighlightDelay = 400 * time.Millisecond

// todoMarksOf is a tab's TODO spans, tinted in each keyword's tone: the
// native stand-in for todo.highlight's coloured comment text.
func (m *Model) todoMarksOf(t *Tab, p *ui.Theme) []editor.TextBackground {
	items := m.scanTodoText(t.doc.Path(), t.doc.Text())
	if len(items) == 0 {
		return nil
	}
	out := make([]editor.TextBackground, 0, len(items))
	for _, it := range items {
		start := t.doc.ByteOffset(openidea.OutlinePos{Line: uint32(it.Line), Col: uint32(it.Col)})
		end := t.doc.ByteOffset(openidea.OutlinePos{Line: uint32(it.Line), Col: uint32(it.Col) + utf16Len(it.Text)})
		if end <= start {
			continue
		}
		out = append(out, editor.TextBackground{
			Start: start, End: end, Color: m.todoTone(it.Keyword, p).Alpha(0.18),
		})
	}
	return out
}

// refreshTodoHighlights keeps every editor's TODO tints current: recomputed
// when typing paused after a change, or when the theme changed.
func (m *Model) refreshTodoHighlights(now time.Time, p *ui.Theme) {
	if p == nil {
		return
	}
	for _, t := range m.tabs {
		v := t.doc.Buffer().Version()
		if t.todoSeen == v && t.todoDark == p.Dark {
			continue
		}
		if t.todoSeen != 0 && v != t.todoSeen && now.Sub(t.edited) < todoHighlightDelay {
			continue
		}
		t.todoSeen = v
		t.todoDark = p.Dark
		t.state.TextBackgrounds = m.todoMarksOf(t, p)
	}
}

// ShowTodo opens the TODO tool window, scanning the project the first time so
// the other tabs have their index, as installing the index does at startup.
func (m *Model) ShowTodo() {
	m.todo.visible = true
	m.occupyBottom("todo")
	if m.Root() != "" {
		m.RefreshVCS()
	}
	if !m.todo.running && !m.todo.scannedOnce {
		m.RunTodo()
	}
	m.invalidate()
}

// ShowTodoCurrentFile is the action todo.showCurrentFile: the window opens on
// the Current File tab.
func (m *Model) ShowTodoCurrentFile() {
	m.todo.tab = "file"
	m.ShowTodo()
}

// ToggleTodo opens the window and a second press from the editor closes it.
func (m *Model) ToggleTodo() {
	if m.todo.visible {
		m.CloseTodo()
		return
	}
	m.ShowTodo()
}

// CloseTodo hides the tool window; its scan stays for the next open.
func (m *Model) CloseTodo() {
	m.todo.visible = false
	m.invalidate()
}

// todoRegex is upstream's todoRegexSource: \b(TODO|FIXME|HACK|XXX)\b.*, or ""
// when every pattern is off.
func (m *Model) todoRegex() string {
	kws := make([]string, 0, len(m.todo.patterns))
	for _, p := range m.todo.patterns {
		if k := strings.TrimSpace(p.Keyword); p.Enabled && k != "" {
			kws = append(kws, regexp.QuoteMeta(k))
		}
	}
	if len(kws) == 0 {
		return ""
	}
	return `\b(` + strings.Join(kws, "|") + `)\b.*`
}

// todoPatternFor is upstream's patternFor: the configured pattern a match
// starts with.
func (m *Model) todoPatternFor(matched string) (TodoPattern, bool) {
	for _, p := range m.todo.patterns {
		kw := strings.TrimSpace(p.Keyword)
		if !p.Enabled || kw == "" {
			continue
		}
		if m.todo.caseSensitive {
			if strings.HasPrefix(matched, kw) {
				return p, true
			}
		} else if strings.HasPrefix(strings.ToLower(matched), strings.ToLower(kw)) {
			return p, true
		}
	}
	return TodoPattern{}, false
}

var (
	// The comment openers count anywhere on the line, at its start or after
	// whitespace, so `//` in a URL and `#` in a string do not pass.
	todoInlineMarker    = regexp.MustCompile(`(?:^|\s)(?://|/\*|<!--|#|--|\{-|\(\*)`)
	todoLineStartMarker = regexp.MustCompile(`^\s*(?:\*|;|%|"""|''')`)
	// A line the backend truncated does not start at the real line start, so
	// any marker counts: a false positive rather than a missed TODO.
	todoLenientMarker = regexp.MustCompile(`(//|/\*|<!--|#|--|\{-|\(\*|\*|;|%|"""|''')`)
)

// looksLikeComment is upstream's looksLikeComment.
func looksLikeComment(prefix string, truncated bool) bool {
	if truncated {
		return todoLenientMarker.MatchString(prefix)
	}
	return todoInlineMarker.MatchString(prefix) || todoLineStartMarker.MatchString(prefix)
}

var todoTextTail = regexp.MustCompile(`\s*(\*/|-->|-\}|\*\)|"""|''')\s*$`)

// cleanTodoText drops the comment closer and trailing whitespace, as
// cleanText does.
func cleanTodoText(s string) string {
	s = todoTextTail.ReplaceAllString(s, "")
	return strings.TrimRight(s, " \t\r\n")
}

// utf16Offset turns a UTF-16 code-unit offset into a byte offset in s, which
// is how search ranges map onto a Go string.
func utf16Offset(s string, units int) int {
	if units <= 0 {
		return 0
	}
	seen := 0
	for i, r := range s {
		if seen >= units {
			return i
		}
		if r > 0xffff {
			seen += 2
		} else {
			seen++
		}
	}
	return len(s)
}

// utf16Len is db.go's helper, which counts the code units the backend's ranges
// use.

// todoItemsFrom turns one file's search matches into items, following
// itemsFromMatches: ranges are UTF-16 offsets, the keyword need not lead the
// comment, and commentsOnly drops matches before a comment marker.
func (m *Model) todoItemsFrom(fm openidea.FileMatches) []TodoItem {
	var out []TodoItem
	for _, lm := range fm.Matches {
		for _, r := range lm.Ranges {
			sb := utf16Offset(lm.Text, int(r[0]))
			eb := utf16Offset(lm.Text, int(r[1]))
			if eb < sb || sb > len(lm.Text) {
				continue
			}
			matched := lm.Text[sb:eb]
			p, ok := m.todoPatternFor(matched)
			if !ok {
				continue
			}
			if m.todo.commentsOnly && !looksLikeComment(lm.Text[:sb], lm.TextOffset > 0) {
				continue
			}
			out = append(out, TodoItem{
				Path:    fm.Path,
				Line:    int(lm.Line),
				Col:     int(lm.TextOffset) + int(r[0]),
				Keyword: strings.TrimSpace(p.Keyword),
				Text:    cleanTodoText(matched),
			})
		}
	}
	return out
}

// scanTodoText applies the same rules to editor text, so unsaved lines show
// their TODOs as they are typed.
func (m *Model) scanTodoText(path, text string) []TodoItem {
	src := m.todoRegex()
	if src == "" {
		return nil
	}
	prefix := ""
	if !m.todo.caseSensitive {
		prefix = "(?i)"
	}
	re, err := regexp.Compile(prefix + src)
	if err != nil {
		return nil
	}
	var out []TodoItem
	for line, content := range strings.Split(text, "\n") {
		for _, loc := range re.FindAllStringIndex(content, -1) {
			matched := content[loc[0]:loc[1]]
			p, ok := m.todoPatternFor(matched)
			if !ok {
				continue
			}
			if m.todo.commentsOnly && !looksLikeComment(content[:loc[0]], false) {
				continue
			}
			out = append(out, TodoItem{
				Path:    path,
				Line:    line,
				Col:     int(utf16Len(content[:loc[0]])),
				Keyword: strings.TrimSpace(p.Keyword),
				Text:    cleanTodoText(matched),
			})
		}
	}
	return out
}

// todoLive is the active buffer's scan, rescanned only when its version
// changed; the path is "" without an editor.
func (m *Model) todoLive() (string, []TodoItem) {
	t := m.tab()
	if t == nil {
		return "", nil
	}
	path := realPath(t.doc.Path())
	version := t.doc.Buffer().Version()
	if m.todo.livePath == path && m.todo.liveVersion == version {
		return path, m.todo.liveItems
	}
	items := m.scanTodoText(path, t.doc.Text())
	m.todo.livePath, m.todo.liveVersion, m.todo.liveItems = path, version, items
	return path, items
}

// RunTodo rescans the project, replacing the running scan as upstream's
// scanProject does.
func (m *Model) RunTodo() {
	m.cancelTodo()
	src := m.todoRegex()
	m.todo.done = false
	m.todo.scannedOnce = false
	m.todo.err = ""
	m.todo.limit = false
	m.todo.step = -1
	if m.Root() == "" || src == "" {
		m.todo.items = map[string][]TodoItem{}
		m.todo.done = true
		m.todo.scannedOnce = true
		m.invalidate()
		return
	}
	ch := m.nextChannel()
	m.todo.channel = ch
	m.todo.request = ch
	m.todo.running = true
	m.todo.items = map[string][]TodoItem{}
	m.invalidate()
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "search_start", map[string]any{
			"request": ch,
			"query":   m.todoQuery(map[string]any{"kind": "project"}),
			"onEvent": map[string]any{"$channel": ch},
		})
		m.post(func() {
			if m.todo.request != ch {
				return
			}
			m.todo.running = false
			m.todo.scannedOnce = true
			if err != nil && m.todo.err == "" {
				m.todo.err = "TODO 扫描失败：" + err.Error()
			}
			if m.Root() != root {
				m.todo.items = map[string][]TodoItem{}
			}
			m.invalidate()
		})
	}()
}

// todoQuery is upstream's buildTodoQuery for one scope.
func (m *Model) todoQuery(scope map[string]any) map[string]any {
	return map[string]any{
		"pattern":       m.todoRegex(),
		"isRegex":       true,
		"caseSensitive": m.todo.caseSensitive,
		"wholeWord":     false,
		"include":       []string{},
		"exclude":       []string{},
		"scope":         scope,
		"maxMatches":    0,
	}
}

// cancelTodo stops a running project scan; its summary then reports
// cancelled and the handler ignores the stale request.
func (m *Model) cancelTodo() {
	if !m.todo.running || m.todo.request == 0 {
		return
	}
	id := m.todo.request
	m.todo.running = false
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
	}()
}

// acceptTodoEvent consumes one streamed message of the project scan.
func (m *Model) acceptTodoEvent(data json.RawMessage) {
	var ev openidea.SearchEvent
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	switch ev.Type {
	case "file":
		if ev.File == nil {
			return
		}
		items := m.todoItemsFrom(*ev.File)
		if len(items) == 0 {
			return
		}
		if len(m.todo.items) >= maxTodoFiles || m.todoCount() >= maxTodoItems {
			m.todo.limit = true
			return
		}
		if m.todo.items == nil {
			m.todo.items = map[string][]TodoItem{}
		}
		path := realPath(ev.File.Path)
		m.todo.items[path] = append(m.todo.items[path], items...)
	case "done":
		m.todo.done = true
		m.todo.scannedOnce = true
		if ev.Summary != nil && ev.Summary.LimitHit {
			m.todo.limit = true
		}
	}
	m.invalidate()
}

// todoCount is the project scan's item count.
func (m *Model) todoCount() int {
	n := 0
	for _, items := range m.todo.items {
		n += len(items)
	}
	return n
}

// todoScope is the backend scope of the Scope Based tab, nil when there is
// nothing to scan.
func (m *Model) todoScope() map[string]any {
	switch m.todo.scopeKind {
	case "productionFiles":
		return map[string]any{"kind": "fileSet", "set": "production"}
	case "testFiles":
		return map[string]any{"kind": "fileSet", "set": "test"}
	case "openFiles":
		paths := make([]string, 0, len(m.tabs))
		for _, t := range m.tabs {
			paths = append(paths, realPath(t.doc.Path()))
		}
		if len(paths) == 0 {
			return nil
		}
		return map[string]any{"kind": "files", "paths": paths}
	}
	return map[string]any{"kind": "project"}
}

// RunTodoScope rescans the Scope Based tab's scope.
func (m *Model) RunTodoScope() {
	if m.todo.scopeRequest != 0 && m.todo.scopeRunning {
		id := m.todo.scopeRequest
		m.todo.scopeRunning = false
		go func() {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			defer cancel()
			_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
		}()
	}
	kind := m.todo.scopeKind
	scope := m.todoScope()
	m.todo.scopeItems = map[string][]TodoItem{}
	m.todo.scopeScanned = kind
	m.todo.scopeDone = false
	m.todo.scopeErr = ""
	m.todo.step = -1
	if m.Root() == "" || scope == nil || m.todoRegex() == "" {
		m.todo.scopeDone = true
		m.invalidate()
		return
	}
	ch := m.nextChannel()
	m.todo.scopeChannel = ch
	m.todo.scopeRequest = ch
	m.todo.scopeRunning = true
	m.invalidate()
	root := m.Root()
	query := m.todoQuery(scope)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "search_start", map[string]any{
			"request": ch,
			"query":   query,
			"onEvent": map[string]any{"$channel": ch},
		})
		m.post(func() {
			if m.todo.scopeRequest != ch {
				return
			}
			m.todo.scopeRunning = false
			if err != nil && m.todo.scopeErr == "" {
				m.todo.scopeErr = "范围扫描失败：" + err.Error()
			}
			if m.Root() != root {
				m.todo.scopeItems = map[string][]TodoItem{}
			}
			m.invalidate()
		})
	}()
}

// acceptTodoScopeEvent consumes one streamed message of the scope scan.
func (m *Model) acceptTodoScopeEvent(data json.RawMessage) {
	var ev openidea.SearchEvent
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	switch ev.Type {
	case "file":
		if ev.File == nil {
			return
		}
		items := m.todoItemsFrom(*ev.File)
		if len(items) == 0 {
			return
		}
		if m.todo.scopeItems == nil {
			m.todo.scopeItems = map[string][]TodoItem{}
		}
		path := realPath(ev.File.Path)
		m.todo.scopeItems[path] = append(m.todo.scopeItems[path], items...)
	case "done":
		m.todo.scopeDone = true
	}
	m.invalidate()
}

// SelectTodoTab switches the window's tab, scanning the Scope Based tab's
// scope when it is first shown.
func (m *Model) SelectTodoTab(tab string) {
	if m.todo.tab == tab {
		return
	}
	m.todo.tab = tab
	m.todo.step = -1
	if tab == "scope" && m.todo.scopeScanned != m.todo.scopeKind {
		m.RunTodoScope()
	}
	if tab == "changes" {
		m.RefreshVCS()
	}
	m.invalidate()
}

// SetTodoScope picks the Scope Based tab's scope and rescans it.
func (m *Model) SetTodoScope(kind string) {
	if m.todo.scopeKind == kind {
		return
	}
	m.todo.scopeKind = kind
	m.todo.scopeScanned = ""
	m.RunTodoScope()
	m.invalidate()
}

// toggleTodoKeyword is a filter chip: the keyword's rows go away or return.
func (m *Model) toggleTodoKeyword(keyword string) {
	if m.todo.hidden[keyword] {
		delete(m.todo.hidden, keyword)
	} else {
		m.todo.hidden[keyword] = true
	}
	m.todo.step = -1
	m.invalidate()
}

// toggleTodoCase flips Match Case and rescans, as changing the setting does.
func (m *Model) toggleTodoCase() {
	m.todo.caseSensitive = !m.todo.caseSensitive
	m.RunTodo()
	if m.todo.scopeScanned != "" {
		m.RunTodoScope()
	}
	m.invalidate()
}

// ToggleTodoGroup shows or hides one file's rows.
func (m *Model) ToggleTodoGroup(path string) {
	if m.todo.collapsed[path] {
		delete(m.todo.collapsed, path)
	} else {
		m.todo.collapsed[path] = true
	}
	m.invalidate()
}

// todoChangedPaths is the set of files the VCS changes list reports.
func (m *Model) todoChangedPaths() map[string]bool {
	out := make(map[string]bool, len(m.vcs.files))
	for _, f := range m.vcs.files {
		out[realPath(f.Path)] = true
	}
	return out
}

// todoMap is the unfiltered item map of the active tab, with the active
// buffer's unsaved lines fronting the backend result for its own path.
func (m *Model) todoMap() map[string][]TodoItem {
	src := map[string][]TodoItem{}
	switch m.todo.tab {
	case "scope":
		for p, items := range m.todo.scopeItems {
			src[p] = items
		}
	case "changes":
		changed := m.todoChangedPaths()
		for p, items := range m.todo.items {
			if changed[p] {
				src[p] = items
			}
		}
	default:
		for p, items := range m.todo.items {
			src[p] = items
		}
	}
	path, live := m.todoLive()
	switch m.todo.tab {
	case "file":
		if path == "" {
			return nil
		}
		return map[string][]TodoItem{path: live}
	case "scope":
		if _, ok := src[path]; ok && path != "" {
			src[path] = live
		}
	case "changes":
		if path != "" && m.todoChangedPaths()[path] {
			src[path] = live
		}
	default:
		if path != "" {
			src[path] = live
		}
	}
	return src
}

// todoGroups is the window's rows: the active tab's items that pass the
// keyword filter, grouped by file, ordered by path and position.
func (m *Model) todoGroups() []todoGroup {
	src := m.todoMap()
	groups := make([]todoGroup, 0, len(src))
	for path, items := range src {
		var kept []TodoItem
		for _, it := range items {
			if m.todo.hidden[it.Keyword] {
				continue
			}
			kept = append(kept, it)
		}
		if len(kept) == 0 {
			continue
		}
		sort.SliceStable(kept, func(i, j int) bool {
			if kept[i].Line != kept[j].Line {
				return kept[i].Line < kept[j].Line
			}
			return kept[i].Col < kept[j].Col
		})
		groups = append(groups, todoGroup{path: path, items: kept})
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].path < groups[j].path })
	return groups
}

// todoTabCount is the rows a tab would show with the current filters.
func (m *Model) todoTabCount(tab string) int {
	was := m.todo.tab
	m.todo.tab = tab
	groups := m.todoGroups()
	m.todo.tab = was
	n := 0
	for _, g := range groups {
		n += len(g.items)
	}
	return n
}

// nextTodo advances the step and returns the row, wrapping at both ends.
func (m *Model) nextTodo(dir int) (TodoItem, bool) {
	var rows []TodoItem
	for _, g := range m.todoGroups() {
		if m.todo.collapsed[g.path] {
			continue
		}
		rows = append(rows, g.items...)
	}
	if len(rows) == 0 {
		return TodoItem{}, false
	}
	m.todo.step = (m.todo.step + dir + len(rows)) % len(rows)
	return rows[m.todo.step], true
}

// StepTodo walks the window's rows like navigate.nextOccurrence and
// prevOccurrence do for the occurrence navigator.
func (m *Model) StepTodo(dir int) {
	if item, ok := m.nextTodo(dir); ok {
		m.revealAt(item.Path, item.Line, item.Col)
	}
}

// todoTone is the theme tone of a keyword's pattern.
func (m *Model) todoTone(keyword string, p *ui.Theme) ui.Color {
	for _, pat := range m.todo.patterns {
		if !strings.EqualFold(strings.TrimSpace(pat.Keyword), keyword) {
			continue
		}
		switch pat.Color {
		case "danger":
			return p.Danger
		case "warning":
			return p.Warning
		case "success":
			return p.Success
		case "violet":
			return todoViolet
		case "muted":
			return p.TextMuted
		}
		return p.Accent
	}
	return p.Accent
}

// todoPanel draws the tool window: a header with the tabs, the keyword
// filters and the actions, then the TODO items grouped by file.
func (m *Model) todoPanel(c *ui.Context, p *ui.Theme) {
	if !m.todo.visible {
		return
	}
	groups := m.todoGroups()
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
		ui.Row(c).Height(30).Padding(0, 8).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "TODO").FontSize(12).Bold().TextColor(p.Text)
			m.todoTab(c, p, "project", "项目", m.todoTabCount("project"))
			m.todoTab(c, p, "file", "当前文件", m.todoTabCount("file"))
			m.todoTab(c, p, "scope", "按范围", 0)
			m.todoTab(c, p, "changes", "更改", m.todoTabCount("changes"))
			ui.Spacer(c)
			for _, pat := range m.todo.patterns {
				if !pat.Enabled || strings.TrimSpace(pat.Keyword) == "" {
					continue
				}
				m.todoKeywordFilter(c, p, pat)
			}
			m.todoAction(c, p, "↑", false, func() { m.StepTodo(-1) })
			m.todoAction(c, p, "↓", false, func() { m.StepTodo(1) })
			m.todoAction(c, p, "Aa", m.todo.caseSensitive, func() { m.toggleTodoCase() })
			m.todoAction(c, p, "刷新", false, func() {
				if m.todo.tab == "scope" {
					m.RunTodoScope()
					return
				}
				m.RunTodo()
			})
			m.todoAction(c, p, "关闭", false, func() { m.CloseTodo() })
		})
		if m.todo.tab == "scope" {
			m.todoScopeBar(c, p)
		}
		if m.todo.limit {
			ui.Row(c).Padding(2, 10).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
				Children(func() {
					ui.Text(c, "匹配过多，只显示前面的部分").FontSize(10.5).
						TextColor(p.Warning).SingleLine()
				})
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(4, 0).Children(func() {
				if hint := m.todoEmptyHint(); hint != "" {
					ui.Text(c, hint).FontSize(11.5).TextColor(p.TextMuted)
					return
				}
				for gi, g := range groups {
					gi, g := gi, g
					m.todoFileRows(c, p, gi, g)
				}
			})
		})
	})
}

// todoEmptyHint is the placeholder, "" while there are rows to draw.
func (m *Model) todoEmptyHint() string {
	if len(m.todoGroups()) > 0 {
		return ""
	}
	switch m.todo.tab {
	case "file":
		if m.tab() == nil {
			return "没有打开的文件"
		}
		return "此文件中没有 TODO"
	case "scope":
		if m.todo.scopeRunning || (!m.todo.scopeDone && m.todo.scopeScanned != "") {
			return "正在扫描 TODO…"
		}
		if m.todo.scopeErr != "" {
			return m.todo.scopeErr
		}
		return "此范围中没有 TODO"
	case "changes":
		return "更改的文件中没有 TODO"
	}
	if m.todo.err != "" {
		return m.todo.err
	}
	if m.todo.running || !m.todo.scannedOnce {
		return "正在扫描 TODO…"
	}
	return "没有找到 TODO"
}

// todoTab is one scope tab in the header; the active tab is filled.
func (m *Model) todoTab(c *ui.Context, p *ui.Theme, id, label string, count int) {
	active := m.todo.tab == id
	r := ui.Row(c.Key("todotab:"+id)).Height(22).Padding(0, 6).Gap(4).
		AlignItems(ui.Center)
	if active {
		r.Background(p.Background)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	color := p.TextMuted
	if active {
		color = p.Text
	}
	r.Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(color).SingleLine()
		if count > 0 {
			ui.Text(c, fmt.Sprintf("%d", count)).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		}
	})
	if r.Clicked() && !active {
		m.SelectTodoTab(id)
	}
}

// todoKeywordFilter toggles one keyword; the chip dims while the keyword is
// hidden.
func (m *Model) todoKeywordFilter(c *ui.Context, p *ui.Theme, pat TodoPattern) {
	key := strings.TrimSpace(pat.Keyword)
	r := ui.Row(c.Key("todokey:"+key)).Height(22).Padding(0, 4).
		AlignItems(ui.Center)
	if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	color := m.todoTone(key, p)
	if m.todo.hidden[key] {
		color = p.TextMuted
	}
	r.Children(func() {
		ui.Text(c, key).FontSize(11).TextColor(color).SingleLine()
	})
	if r.Clicked() {
		m.toggleTodoKeyword(key)
	}
}

// todoAction is a header action such as Match Case or Refresh.
func (m *Model) todoAction(c *ui.Context, p *ui.Theme, label string, active bool, run func()) {
	r := ui.Row(c.Key("todoact:"+label)).Height(22).Padding(0, 5).
		AlignItems(ui.Center)
	if active {
		r.Background(p.Background)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	color := p.TextMuted
	if active {
		color = p.Text
	}
	r.Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(color).SingleLine()
	})
	if r.Clicked() {
		run()
	}
}

// todoScopeBar is the Scope Based tab's chooser, upstream's ScopeBar.
func (m *Model) todoScopeBar(c *ui.Context, p *ui.Theme) {
	labels := make([]string, 0, len(todoScopes))
	current := ""
	for _, s := range todoScopes {
		labels = append(labels, s.label)
		if s.id == m.todo.scopeKind {
			current = s.label
		}
	}
	chosen := current
	ui.Row(c).Height(32).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "范围：").FontSize(10.5).TextColor(p.TextMuted)
		ui.Select(c, &chosen, labels).Width(240)
		if m.todo.scopeRunning {
			ui.Text(c, "扫描中…").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		}
	})
	if chosen != current && chosen != "" {
		for _, s := range todoScopes {
			if s.label == chosen {
				m.SetTodoScope(s.id)
				break
			}
		}
	}
}

// todoFileRows is a file's header row and, unless collapsed, its items.
func (m *Model) todoFileRows(c *ui.Context, p *ui.Theme, gi int, g todoGroup) {
	dir := filepath.Dir(g.path)
	dir = strings.TrimPrefix(dir, m.Root())
	dir = strings.TrimPrefix(dir, string(filepath.Separator))
	collapsed := m.todo.collapsed[g.path]
	head := ui.Row(c.Key(fmt.Sprintf("todofile:%d", gi))).Height(22).Gap(6).
		Padding(0, 6).AlignItems(ui.Center)
	if head.Hovered() {
		head.Background(p.SurfaceHover)
	}
	head.Children(func() {
		glyph := "▾"
		if collapsed {
			glyph = "▸"
		}
		ui.Text(c, glyph).FontSize(10).TextColor(p.TextMuted)
		ui.Text(c, filepath.Base(g.path)).FontSize(12).Bold().TextColor(p.Text).
			SingleLine()
		if dir != "" && dir != "." {
			ui.Text(c, dir).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("%d 项", len(g.items))).FontSize(10.5).
			TextColor(p.TextMuted).SingleLine()
	})
	if head.Clicked() {
		m.ToggleTodoGroup(g.path)
	}
	if collapsed {
		return
	}
	for i, item := range g.items {
		i, item := i, item
		color := m.todoTone(item.Keyword, p)
		r := ui.Row(c.Key(fmt.Sprintf("todo:%d:%d", gi, i))).Height(22).Padding(0, 6).
			Gap(6).AlignItems(ui.Center).PaddingStart(22)
		if r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			ui.Text(c, fmt.Sprintf("(%d, %d)", item.Line+1, item.Col+1)).FontSize(10.5).
				TextColor(p.TextMuted).SingleLine()
			kw := item.Keyword
			rest := strings.TrimPrefix(item.Text, item.Keyword)
			if strings.HasPrefix(item.Text, kw) {
				ui.Text(c, kw).FontSize(11).Bold().TextColor(color).SingleLine()
				if rest != "" {
					ui.Text(c, rest).FontSize(11).Italic().TextColor(color).
						SingleLine().Ellipsis("…")
				}
			} else {
				ui.Text(c, item.Text).FontSize(11).Italic().TextColor(color).
					SingleLine().Ellipsis("…")
			}
		})
		if r.Clicked() {
			m.previewAt(item.Path, item.Line, item.Col)
		}
		if r.DoubleClicked() {
			m.revealAt(item.Path, item.Line, item.Col)
		}
	}
}

// SmokeTodo runs a real project scan, then the open-files scope scan, and
// reports what streamed back: the counts, the first item and the tab rows.
func (m *Model) SmokeTodo(ctx context.Context) (map[string]any, error) {
	if m.Root() == "" {
		return nil, errors.New("没有打开项目")
	}
	out := map[string]any{}
	m.post(func() {
		m.todo.tab = "project"
		m.ShowTodo()
	})
	deadline := time.Now().Add(120 * time.Second)
	for {
		var done, running, visible bool
		var files, items int
		var summary string
		var first TodoItem
		m.post(func() {
			done, running = m.todo.done, m.todo.running
			visible = m.todo.visible
			files, items = len(m.todo.items), m.todoCount()
			for _, g := range m.todoGroups() {
				sum := fmt.Sprintf("%s:%d", filepath.Base(g.path), len(g.items))
				if summary == "" {
					summary = sum
					first = g.items[0]
				}
			}
		})
		if done || (!running && items == 0) || time.Now().After(deadline) {
			out["done"] = done
			out["files"] = files
			out["items"] = items
			out["firstFile"] = summary
			out["firstLine"] = first.Line + 1
			out["firstKeyword"] = first.Keyword
			out["firstText"] = strings.TrimSpace(first.Text)
			out["visible"] = visible
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	// The Scope Based tab's open-files scope proves the `files` scope path.
	m.post(func() {
		m.todo.scopeKind = "openFiles"
		m.todo.scopeScanned = ""
		m.RunTodoScope()
	})
	deadline = time.Now().Add(90 * time.Second)
	for {
		var done bool
		var count int
		var errText string
		m.post(func() {
			done, errText = m.todo.scopeDone, m.todo.scopeErr
			for _, items := range m.todo.scopeItems {
				count += len(items)
			}
		})
		if done || errText != "" || time.Now().After(deadline) {
			out["openFilesItems"] = count
			if errText != "" {
				out["openFilesError"] = errText
			}
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	// Settings › Editor › TODO registers, stages and applies; an open editor
	// tints the TODOs of the file it shows.
	m.post(func() {
		if m.tab() == nil {
			if p := findSmokeFile(m.Root()); p != "" {
				m.openFile(p)
			}
		}
	})
	openDeadline := time.Now().Add(30 * time.Second)
	for {
		opened := false
		m.post(func() { opened = m.tab() != nil })
		if opened || time.Now().After(openDeadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	m.post(func() {
		m.OpenSettingsPage(settingsPageTodo)
		out["settingsPage"] = m.settingsPage == settingsPageTodo
		out["settingsDefaults"] = len(m.settingsDraft.todoPatterns)
		m.settingsAddTodoPattern()
		m.applySettings()
		out["settingsApplied"] = m.prefs.Todo != nil && len(m.prefs.Todo.Patterns) == 5
		m.settingsRestoreTodoPatterns()
		m.applySettings()
		out["settingsRestored"] = m.prefs.Todo == nil
		m.closeSettings()
		m.refreshTodoHighlights(time.Now(), openideaTheme(true))
		if t := m.tab(); t != nil {
			out["highlightSpans"] = len(t.state.TextBackgrounds)
		}
	})
	return out, nil
}
