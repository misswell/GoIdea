package app

// Search Everywhere after OpenIDEA's search feature: one overlay whose tabs
// narrow the results to classes, files, symbols, actions or text, with an All
// tab merging them under section headers. An empty query lists recent files
// and recent actions, as upstream's sectionsFor does.

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/highlight"
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

	seRecentActionCap = 12
	// seHistoryCap bounds the remembered Search Everywhere queries for each
	// tab, as OpenIDEA's MAX_HISTORY.
	seHistoryCap = 50
	seDebounce   = 60 * time.Millisecond

	seDialogWidth    = 760
	seDialogMaxListH = 672
	seDialogMinListH = 240
	sePreviewLines   = 41
	sePreviewBytes   = 512 << 10
)

// seItem is one result row before it is drawn.
type seItem struct {
	kind        string // "file", "symbol", "endpoint", "text", "action", "setting" or "command"
	title       string
	detail      string
	rel         string
	path        string
	line        int
	col         int
	endCol      int
	lineSet     bool
	container   string
	right       string
	action      string
	setting     string
	settingPath []string
	command     seTab
	folder      bool
	// disabled keeps a matching action visible while making it inert, as
	// OpenIDEA does for unavailable actions.
	disabled bool
	// score is the source's pre-computed relevance, used by the All tab's
	// relevance mix and by endpoint ranking.
	score int
	icon  *ui.SVG
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
	// selAnchor and selKeys implement OpenIDEA's Shift range and primary-click
	// toggle selections. A nil selKeys means only the lead row is selected.
	selAnchor int
	selKeys   map[string]bool
	// extra widens a section past its first page, as the More rows do.
	extra map[seSection]int
	// location is the parsed `File.kt:42` part of the current query.
	location seLocationQuery

	recent       []openidea.FileMatch
	files        []openidea.FileMatch
	dirs         []openidea.FileMatch
	indexSymbols []openidea.SymbolMatch
	symbols      []openidea.WorkspaceSymbol
	endpoints    []openidea.PluginEndpoint
	text         []openidea.FileMatches
	textCount    int
	// lastRows is the last settled view. While a new query streams, a loading
	// section keeps those rows until its source answers, matching OpenIDEA's
	// SectionState.
	lastRows []seRow

	fileDone, symbolDone, endpointDone, textDone bool
	textBusy                                     bool
	textTruncated                                bool
	err                                          string

	// The All/Actions tabs use includeNonProject for their checkbox. The
	// other tabs combine it with fileSet to choose IDEA's named scope.
	includeNonProject bool
	fileSet           string
	scopeRoots        []seSourceFolder
	scopeRootsDone    bool
	autoEverywhere    bool
	notFound          string

	// preview keeps the selected source beside the list, remembered across
	// openings as OpenIDEA's preview toggle is.
	preview       bool
	previewKey    string
	previewPath   string
	previewLine   int
	previewCol    int
	previewEndCol int
	previewFrom   int
	previewLines  []string
	previewTokens [][]highlight.Token
	previewErr    string
	previewRatio  float32

	historyIndex    int
	historyDraft    string
	pendingEnter    bool
	pendingSide     bool
	revealSelection bool

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

// seSourceFolder is one entry of the backend's project_roots response. The
// same root union drives OpenIDEA's Project Files, Production and Test scopes.
type seSourceFolder struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type seProjectRoots struct {
	Roots []seSourceFolder `json:"roots"`
	// The pinned backend also returns detected roots; unmarked entries are not
	// effective and therefore do not feed a named scope.
	Detected []seSourceFolder `json:"detected"`
}

func (m *Model) loadSearchEverywhereScopes() {
	s := m.se
	if s == nil || s.scopeRootsDone {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.scopeRootsDone = true
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "project_roots", nil)
		var roots seProjectRoots
		if err == nil {
			err = json.Unmarshal(raw, &roots)
		}
		m.post(func() {
			if m.se != s {
				return
			}
			s.scopeRootsDone = true
			if err == nil {
				s.scopeRoots = roots.Roots
			}
			m.invalidate()
		})
	}()
}

// searchEverywherePathAllowed applies IDEA's All Places / Project /
// Production / Test scope to a backend result.
func (m *Model) searchEverywherePathAllowed(path, fileSet string, includeNonProject bool) bool {
	if includeNonProject {
		return true
	}
	if fileSet == "" || fileSet == "all" {
		return sePathInside(m.Root(), path)
	}
	root := m.Root()
	if !sePathInside(root, path) {
		return false
	}
	relative := strings.Trim(seRelativePath(root, path), "/")
	var innermost *seSourceFolder
	if m.se != nil {
		for index := range m.se.scopeRoots {
			candidate := &m.se.scopeRoots[index]
			if candidate.Path == "" || relative == candidate.Path ||
				strings.HasPrefix(relative, strings.TrimRight(candidate.Path, "/")+"/") {
				if innermost == nil || len(candidate.Path) > len(innermost.Path) {
					innermost = candidate
				}
			}
		}
	}
	test := seLooksLikeTestPath(relative)
	if innermost != nil {
		switch strings.ToLower(innermost.Kind) {
		case "excluded":
			return false
		case "test", "testresources":
			test = true
		case "source", "resources":
			test = false
		}
	}
	if fileSet == "test" {
		return test
	}
	return !test
}

func seLooksLikeTestPath(relative string) bool {
	relative = strings.ToLower(strings.ReplaceAll(relative, `\`, "/"))
	name := filepath.Base(relative)
	path := "/" + relative
	if strings.HasPrefix(relative, "src/test/") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/__tests__/") {
		return true
	}
	for _, suffix := range []string{
		"_test.go", "_test.py", "test.py", "_test.rs", ".test.js", ".test.jsx",
		".test.ts", ".test.tsx", ".spec.js", ".spec.jsx", ".spec.ts", ".spec.tsx",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return name == "conftest.py" ||
		(strings.HasSuffix(name, ".java") &&
			(strings.HasSuffix(strings.TrimSuffix(name, ".java"), "Test") ||
				strings.HasSuffix(strings.TrimSuffix(name, ".java"), "Tests") ||
				strings.HasSuffix(strings.TrimSuffix(name, ".java"), "Spec") ||
				strings.HasSuffix(strings.TrimSuffix(name, ".java"), "IT")))
}

// searchEverywhereScratchMatches lists files under GoIDEA's Scratch and
// Consoles root, matching them like the project index, for IDEA's "Include
// non-project items". The walk is bounded to four levels and 2,000 files.
func (m *Model) searchEverywhereScratchMatches(query string) []openidea.FileMatch {
	root, err := m.scratchDir()
	if err != nil {
		return nil
	}
	var out []openidea.FileMatch
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." {
			return nil
		}
		relative = strings.ReplaceAll(relative, `\`, "/")
		depth := strings.Count(relative, "/")
		if entry.IsDir() {
			if depth >= 4 {
				return fs.SkipDir
			}
			return nil
		}
		if len(out) >= 2000 {
			return fs.SkipAll
		}
		display := "Scratches/" + relative
		score := seItemMatchScore(filepath.Base(relative), display, query)
		if score == 0 {
			return nil
		}
		out = append(out, openidea.FileMatch{
			Path: path, RelPath: display, Score: uint32(score),
		})
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// seQueryTimeout bounds one backend lookup of the overlay.
const seQueryTimeout = 30 * time.Second

// OpenSearchEverywhere opens the overlay over the window.
func (m *Model) OpenSearchEverywhere() {
	m.openSearchEverywhereAt(seTabAll)
}

func (m *Model) openSearchEverywhereAt(tab seTab) {
	if m.se != nil {
		if m.se.tab == tab {
			m.setSearchEverywhereIncludeNonProject(!m.se.includeNonProject)
			return
		}
		m.SetSearchEverywhereTab(tab)
		return
	}
	m.se = &sePane{
		open: true, tab: tab, extra: map[seSection]int{}, filters: m.seFilters,
		preview: m.sePreviewState, previewRatio: 33, fileSet: "all",
		autoEverywhere: true, historyIndex: -1, revealSelection: true,
	}
	if t := m.tab(); t != nil && t.state != nil {
		m.se.query = t.state.SelectedText()
	}
	if strings.TrimSpace(m.se.query) == "" {
		if history := m.searchHistoryFor(tab); len(history) > 0 {
			m.se.query = history[0]
		}
	}
	m.refreshSearchEverywhere()
	m.loadSearchEverywhereLanguages()
	m.loadSearchEverywherePlugins()
	m.loadSearchEverywhereScopes()
	m.invalidate()
}

// CloseSearchEverywhere drops the overlay and cancels lookups in flight.
func (m *Model) CloseSearchEverywhere() {
	s := m.se
	if s == nil {
		return
	}
	m.rememberSearchQuery(s.tab, s.query)
	m.cancelSearchEverywhereSources()
	m.se = nil
	m.invalidate()
}

func (m *Model) cancelSearchEverywhereSources() {
	s := m.se
	if s == nil || m.ctx == nil {
		return
	}
	for _, id := range []uint32{s.fileReq, s.symbolReq, s.textCh} {
		if id != 0 {
			m.cancelRequest(id)
		}
	}
	s.fileReq = 0
	s.symbolReq = 0
	s.textCh = 0
}

// SetSearchEverywhereTab switches the narrowing tab, as a click or Tab does.
func (m *Model) SetSearchEverywhereTab(tab seTab) {
	if m.se == nil {
		return
	}
	if m.se.tab == tab {
		return
	}
	if m.se.autoEverywhere && m.se.notFound != "" {
		m.se.includeNonProject = false
		m.se.notFound = ""
	}
	m.se.tab = tab
	m.se.sel = 0
	m.se.selKey = ""
	m.se.selAnchor = 0
	m.se.selKeys = nil
	m.se.filterOpen = false
	m.se.historyIndex = -1
	m.refreshSearchEverywhere()
	m.loadSearchEverywhereLanguages()
	m.invalidate()
}

// setSearchEverywhereIncludeNonProject is the All/Actions checkbox and the
// All Places scope. It counts as the user's explicit scope choice, so a later
// empty project query no longer widens automatically.
func (m *Model) setSearchEverywhereIncludeNonProject(on bool) {
	if m.se == nil || m.se.includeNonProject == on {
		return
	}
	m.se.includeNonProject = on
	m.se.autoEverywhere = false
	m.se.notFound = ""
	m.se.sel = 0
	m.se.selKey = ""
	m.se.selAnchor = 0
	m.se.selKeys = nil
	m.refreshSearchEverywhere()
	m.invalidate()
}

// setSearchEverywhereFileSet applies Project Files / Production / Test.
func (m *Model) setSearchEverywhereFileSet(fileSet string) {
	if m.se == nil {
		return
	}
	switch fileSet {
	case "all", "production", "test":
	default:
		fileSet = "all"
	}
	m.se.fileSet = fileSet
	m.se.includeNonProject = false
	m.se.autoEverywhere = false
	m.se.notFound = ""
	m.loadSearchEverywhereScopes()
	m.se.sel = 0
	m.se.selKey = ""
	m.se.selAnchor = 0
	m.se.selKeys = nil
	m.refreshSearchEverywhere()
	m.invalidate()
}

// setSearchEverywherePreview toggles the source preview and remembers it.
func (m *Model) setSearchEverywherePreview(on bool) {
	if m.se == nil {
		return
	}
	m.se.preview = on
	m.sePreviewState = on
	m.prefs.SearchPreview = on
	if m.cfg.SettingsPath != "" {
		_ = m.savePreferences()
	}
	if !on {
		m.clearSearchEverywherePreview()
	} else {
		m.syncSearchEverywherePreview()
	}
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
		return []seSection{seClasses, seFiles, seSymbols, seEndpoints, seActions, seText}
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

// loadSearchEverywhereLanguages loads the File Types definitions once for the
// funnel. Their extensions are how Files, Classes and Symbols apply language
// exclusions, matching OpenIDEA's extensionIndex.
func (m *Model) loadSearchEverywhereLanguages() {
	s := m.se
	if s == nil || s.languagesBusy || s.languagesDone {
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		s.languagesDone = true
		return
	}
	s.languagesBusy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		catalog, err := m.toolLanguages(ctx)
		languages := make([]openidea.LanguageInfo, 0, len(catalog))
		for _, language := range catalog {
			languages = append(languages, openidea.LanguageInfo{
				ID: language.ID, Name: language.Name,
				Extensions: language.Extensions, Filenames: language.Filenames,
			})
		}
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
			m.searchEverywhereSectionDone()
		})
	}()
}

// refreshSearchEverywhere restarts the lookups the tab needs.
func (m *Model) refreshSearchEverywhere() {
	s := m.se
	if s == nil {
		return
	}
	if s.tab != seTabAll && s.tab != seTabActions && s.filters.active(s.tab) {
		m.loadSearchEverywhereLanguages()
	}
	if s.notFound != "" && s.autoEverywhere && s.includeNonProject {
		if len(s.notFound) > 1 && strings.Contains(s.query, s.notFound) {
			// Keep the automatically widened scope while the user is still
			// refining the query that had no project results.
		} else {
			s.includeNonProject = false
			s.notFound = ""
		}
	}
	if parsed, ok := parseSECommand(s.query); ok {
		if parsed.list {
			m.cancelSearchEverywhereSources()
			s.seq++
			s.lastRows = nil
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
		s.query = parsed.rest
		s.tab = parsed.tab
		s.historyIndex = -1
		s.filterOpen = false
	}
	m.cancelSearchEverywhereSources()
	s.seq++
	seq := s.seq
	m.settleSearchEverywhereSnapshot()
	s.extra = map[seSection]int{}
	s.sel = 0
	s.selKey = ""
	s.selAnchor = 0
	s.selKeys = nil
	s.textBusy = false
	s.fileDone = false
	s.symbolDone = false
	s.endpointDone = false
	s.textDone = false
	s.textTruncated = false
	s.err = ""
	s.pendingEnter = false
	m.clampSESelection()
	sections := sectionsFor(s.tab, s.seEmpty())
	rawQuery := strings.TrimSpace(s.query)
	s.location = parseSELocationQuery(rawQuery, m.Root())
	query := strings.TrimSpace(s.location.text)
	wantsRecent, wantsFiles, wantsSymbols, wantsEndpoints, wantsText := false, false, false, false, false
	for _, sec := range sections {
		switch sec {
		case seRecentFiles:
			wantsRecent = true
		case seFiles:
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
	if !wantsRecent && !wantsFiles {
		s.fileDone = true
	}
	if !wantsSymbols {
		s.symbolDone = true
	}
	if !wantsEndpoints {
		s.endpointDone = true
	}
	if !wantsText {
		s.textDone = true
	}
	if m.Root() == "" {
		wantsRecent, wantsFiles, wantsSymbols, wantsEndpoints, wantsText = false, false, false, false, false
		s.fileDone = true
		s.symbolDone = true
		s.endpointDone = true
		s.textDone = true
	}
	if s.fileDone {
		s.recent = nil
		s.files = nil
		s.dirs = nil
	}
	if s.symbolDone {
		s.indexSymbols = nil
		s.symbols = nil
	}
	if s.endpointDone {
		s.endpoints = nil
	}
	if s.textDone {
		s.text = nil
		s.textCount = 0
	}
	if wantsRecent {
		m.seQueryRecentFiles(seq)
	} else if wantsFiles {
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

// seQueryRecentFiles lists the backend's recent files for an empty query, as
// OpenIDEA's Recent Files section does.
func (m *Model) seQueryRecentFiles(seq int) {
	s := m.se
	if s == nil {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.fileDone = true
		m.searchEverywhereSectionDone()
		return
	}
	req := m.nextRequest()
	s.fileReq = req
	root := m.Root()
	limit := s.seSectionLimit(seRecentFiles)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "recent_files", nil)
		var paths []string
		if err == nil {
			err = json.Unmarshal(raw, &paths)
		}
		m.post(func() {
			if m.se != s || s.seq != seq {
				return
			}
			s.fileReq = 0
			s.fileDone = true
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" && s.err == "" {
					s.err = "Loading recent files failed: " + err.Error()
				}
			} else {
				s.recent = make([]openidea.FileMatch, 0, min(len(paths), limit))
				for index, path := range paths {
					if index >= limit {
						break
					}
					if path == "" {
						continue
					}
					s.recent = append(s.recent, openidea.FileMatch{
						Path: path, RelPath: seRelativePath(root, path),
					})
				}
			}
			m.searchEverywhereSectionDone()
		})
	}()
}

// seQueryFiles asks the file index for files and, where the tab can show
// directories, for folder matches too. Empty queries use recent_files above.
func (m *Model) seQueryFiles(seq int, query string) {
	s := m.se
	if s == nil {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.fileDone = true
		m.searchEverywhereSectionDone()
		return
	}
	req := m.nextRequest()
	s.fileReq = req
	tab := s.tab
	includeNonProject := s.includeNonProject
	fileSet := s.fileSet
	locationHas := s.location.has
	languageFilter := tab == seTabFiles && s.filters.active(seTabFiles)
	go func() {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(seDebounce):
		}
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		limit := s.seSectionLimit(seFiles) + 1
		scopeFilter := !includeNonProject && fileSet != "all"
		if scopeFilter || languageFilter {
			limit = limit*4 + 1
		}
		withDirs := strings.TrimSpace(query) != "" && !languageFilter && !locationHas

		var files, dirs []openidea.FileMatch
		var fileErr, dirErr error
		var wg sync.WaitGroup
		fetch := func(command string, dst *[]openidea.FileMatch, errp *error, done *sync.WaitGroup) {
			defer done.Done()
			raw, err := m.mgr.Invoke(ctx, Window, command, map[string]any{
				"query": query, "limit": limit,
			})
			if err == nil {
				err = json.Unmarshal(raw, dst)
			}
			*errp = err
		}
		wg.Add(1)
		go fetch("find_files", &files, &fileErr, &wg)
		if withDirs {
			wg.Add(1)
			go fetch("find_dirs", &dirs, &dirErr, &wg)
		}
		wg.Wait()

		m.post(func() {
			if m.se != s || s.seq != seq {
				return
			}
			s.fileReq = 0
			s.fileDone = true
			files = m.mergeSearchEverywhereFiles(files, dirs, query)
			err := fileErr
			if err == nil && dirErr != nil && len(files) == 0 {
				err = dirErr
			}
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" {
					s.err = "Finding files failed: " + err.Error()
				}
			} else {
				s.files = files
			}
			m.searchEverywhereSectionDone()
		})
	}()
}

func (m *Model) mergeSearchEverywhereFiles(
	files, dirs []openidea.FileMatch, query string,
) []openidea.FileMatch {
	s := m.se
	if s == nil {
		return files
	}
	merged := make([]openidea.FileMatch, 0, len(files)+len(dirs))
	seen := map[string]bool{}
	for _, file := range files {
		if m.searchEverywherePathAllowed(file.Path, s.fileSet, s.includeNonProject) &&
			(s.tab != seTabFiles || !s.filters.languageExcluded(seTabFiles, file.Path, s.languages)) {
			merged = append(merged, file)
			seen[file.Path] = true
		}
	}
	if s.includeNonProject && strings.TrimSpace(query) != "" {
		for _, file := range m.searchEverywhereScratchMatches(query) {
			if !seen[file.Path] {
				merged = append(merged, file)
				seen[file.Path] = true
			}
		}
	}
	for _, dir := range dirs {
		if !seen[dir.Path] && m.searchEverywherePathAllowed(dir.Path, s.fileSet, s.includeNonProject) &&
			(s.tab != seTabFiles || !s.filters.languageExcluded(seTabFiles, dir.Path, s.languages)) {
			merged = append(merged, dir)
			seen[dir.Path] = true
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return !strings.HasSuffix(merged[i].RelPath, "/") &&
			strings.HasSuffix(merged[j].RelPath, "/")
	})
	return merged
}

// seQuerySymbols asks both the project index and the language server, then
// merges them in the All/Classes/Symbols sections as OpenIDEA does.
func (m *Model) seQuerySymbols(seq int, query string) {
	s := m.se
	if s == nil {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.symbolDone = true
		m.searchEverywhereSectionDone()
		return
	}
	req := m.nextRequest()
	s.symbolReq = req
	filter := "all"
	limit := s.seSectionLimit(seSymbols)
	if s.tab == seTabClasses {
		limit = s.seSectionLimit(seClasses)
		filter = "types"
	} else if s.tab == seTabAll {
		limit *= 2
	}
	go func() {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(seDebounce):
		}
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		var indexed []openidea.SymbolMatch
		var lsp []openidea.WorkspaceSymbol
		var indexErr, lspErr error
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			raw, err := m.mgr.Invoke(ctx, Window, "find_symbols", map[string]any{
				"query": query, "filter": filter, "limit": limit,
			})
			if err == nil {
				err = json.Unmarshal(raw, &indexed)
			}
			indexErr = err
		}()
		go func() {
			defer wait.Done()
			raw, err := m.mgr.Invoke(ctx, Window, "lsp_workspace_symbols", map[string]any{
				"request": req, "query": query,
			})
			if err == nil {
				err = json.Unmarshal(raw, &lsp)
			}
			lspErr = err
		}()
		wait.Wait()
		err := indexErr
		if err != nil {
			err = lspErr
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
			}
			for _, symbol := range indexed {
				path := symbol.Path
				if path == "" {
					path = symbol.RelPath
				}
				if !m.searchEverywherePathAllowed(path, s.fileSet, s.includeNonProject) {
					continue
				}
				if s.tab != seTabAll && s.filters.languageExcluded(s.tab, path, s.languages) {
					continue
				}
				if s.tab != seTabAll && s.filters.kindExcluded(s.tab, symbol.Kind) {
					continue
				}
				s.indexSymbols = append(s.indexSymbols, symbol)
			}
			for _, symbol := range lsp {
				if !m.searchEverywherePathAllowed(symbol.Location.Path, s.fileSet, s.includeNonProject) {
					continue
				}
				if s.tab != seTabAll && s.filters.languageExcluded(s.tab, symbol.Location.Path, s.languages) {
					continue
				}
				if s.tab != seTabAll && s.filters.kindExcluded(s.tab, symbol.Kind) {
					continue
				}
				s.symbols = append(s.symbols, symbol)
			}
			m.searchEverywhereSectionDone()
		})
	}()
}

// seQueryText streams a project text search, keeping only what the section
// shows; the rest of the stream is dropped.
func (m *Model) seQueryText(seq int, query string) {
	s := m.se
	if s == nil {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.textDone = true
		s.textBusy = false
		m.searchEverywhereSectionDone()
		return
	}
	ch := m.nextChannel()
	s.textCh = ch
	s.textBusy = true
	mask := s.filters.Text
	if s.tab != seTabText {
		mask = ""
	}
	fileSet := s.fileSet
	limit := s.seSectionLimit(seText)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		scope := map[string]any{"kind": "project"}
		if fileSet != "all" {
			scope = map[string]any{"kind": "fileSet", "set": fileSet}
		}
		_, err := m.mgr.Invoke(ctx, Window, "search_start", map[string]any{
			"request": ch,
			"query": map[string]any{
				"pattern": query, "isRegex": false, "caseSensitive": false,
				"wholeWord": false, "include": searchMaskList(mask), "exclude": []string{},
				"scope": scope, "maxMatches": limit,
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
			m.searchEverywhereSectionDone()
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
		s.text = append(s.text, *ev.File)
		s.textCount += len(ev.File.Matches)
		if s.textCount >= s.seSectionLimit(seText) {
			s.textTruncated = true
		}
	case "done":
		s.textBusy = false
		s.textDone = true
	}
	m.clampSESelection()
	m.searchEverywhereSectionDone()
}

func (s *sePane) searchEverywhereLoading() bool {
	for _, section := range sectionsFor(s.tab, s.seEmpty()) {
		switch section {
		case seRecentFiles, seFiles:
			if !s.fileDone {
				return true
			}
		case seClasses, seSymbols:
			if !s.symbolDone {
				return true
			}
		case seEndpoints:
			if !s.endpointDone || s.pluginsBusy {
				return true
			}
		case seText:
			if !s.textDone || s.textBusy {
				return true
			}
		}
	}
	return false
}

// searchEverywhereSectionDone keeps selection and preview aligned while
// sources stream, waits for an early Enter until the top result is settled,
// and applies IDEA's automatic "search everywhere instead" fallback.
func (m *Model) searchEverywhereSectionDone() {
	s := m.se
	if s == nil {
		return
	}
	if s.tab != seTabAll && s.filters.active(s.tab) &&
		s.tab != seTabText &&
		!s.languagesBusy && !s.languagesDone {
		m.loadSearchEverywhereLanguages()
		return
	}
	m.clampSESelection()
	m.syncSearchEverywherePreview()
	if !s.searchEverywhereLoading() && !s.seEmpty() && s.autoEverywhere &&
		!s.includeNonProject && s.tab != seTabActions && s.notFound == "" &&
		len(m.seRows()) == 0 {
		query := strings.TrimSpace(parseSELocationQuery(strings.TrimSpace(s.query), m.Root()).text)
		if query != "" {
			s.includeNonProject = true
			s.notFound = query
			m.refreshSearchEverywhere()
			return
		}
	}
	if s.pendingEnter && !s.searchEverywhereLoading() {
		side := s.pendingSide
		s.pendingEnter = false
		s.pendingSide = false
		m.activateSearchEverywhereNow(side)
		return
	}
	m.settleSearchEverywhereSnapshot()
	m.invalidate()
}

// settleSearchEverywhereSnapshot records every settled source independently.
// Slow sections keep their previous items, so a fast action or file response
// remains usable while a later search is still streaming.
func (m *Model) settleSearchEverywhereSnapshot() {
	s := m.se
	if s == nil {
		return
	}
	sections := sectionsFor(s.tab, s.seEmpty())
	current := searchEverywhereRowsBySection(m.buildSearchEverywhereRows())
	previous := searchEverywhereRowsBySection(s.lastRows)
	for _, section := range sections {
		if m.searchEverywhereSectionIsLoading(section) {
			current[section] = s.retainedSearchEverywhereRows(section, previous[section])
		}
	}
	s.lastRows = cloneSearchEverywhereRows(m.composeSearchEverywhereRows(sections, current))
}

// retainedSearchEverywhereRows limits a retained group to that section's
// current page. It also prevents a More row from surviving a complete scan.
func (s *sePane) retainedSearchEverywhereRows(
	section seSection,
	group searchEverywhereSectionRows,
) searchEverywhereSectionRows {
	limit := s.seSectionLimit(section)
	if len(group.items) > limit {
		group.items = group.items[:limit]
	}
	if len(group.items) < limit {
		group.more = false
	}
	return group
}

// seActionItems is the registry filtered by the query, as Find Action does.
func (m *Model) seActionItems(query string) []seItem {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	var items []seItem
	for _, cmd := range commands() {
		enabled := m.ActionEnabled(cmd.id)
		action, hasAction := actionGetForModel(m, cmd.id)
		category, description, keywords := cmd.group, "", cmd.keywords
		icon := (*ui.SVG)(nil)
		if hasAction {
			if action.Category != "" {
				category = action.Category
			}
			description = action.Description
			keywords = action.Keywords
			icon = action.Icon
		}

		titleHit := seMatchText(cmd.title, query, true)
		score := titleHit.score
		var indices []int
		if len(titleHit.runs) > 0 {
			indices = seRunIndices(titleHit.runs)
		} else {
			extra := strings.Join([]string{category, description, keywords}, "\n")
			extraHit := seMatchText(extra, query, true)
			idHit := strings.Contains(strings.ToLower(cmd.id), strings.ToLower(query))
			if len(extraHit.runs) == 0 && !idHit {
				continue
			}
			score = extraHit.score - 400
		}
		if !enabled {
			score -= 150
		}
		for recentRank, id := range m.recentActions {
			if id == cmd.id {
				score += 40 - min(30, recentRank*3)
				break
			}
		}
		items = append(items, seItem{
			kind:     "action",
			title:    cmd.title,
			detail:   category,
			action:   cmd.id,
			disabled: !enabled,
			indices:  indices,
			score:    score,
			icon:     icon,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	return items
}

// seRecentActionItems is the recently run commands, newest first.
func (m *Model) seRecentActionItems() []seItem {
	var items []seItem
	for _, id := range m.recentActions {
		cmd, ok := actionGet(id)
		if !ok {
			continue
		}
		items = append(items, seItem{
			kind:     "action",
			title:    cmd.Title,
			detail:   cmd.Category,
			action:   cmd.ID,
			disabled: !m.ActionEnabled(cmd.ID),
			icon:     cmd.Icon,
			score:    0,
		})
	}
	return items
}

// seSettingItems indexes every settings page and every option label as a
// separate result, following OpenIDEA's settingsSearchEntries. A matching
// option keeps its page breadcrumb in the Settings column.
func (m *Model) seSettingItems(query string) []seItem {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	tree := settingsTree()
	var items []seItem
	seen := map[string]bool{}
	for _, node := range settingsFlattenAll(tree) {
		page := node.page
		if page == nil {
			continue
		}
		path := settingsBreadcrumb(tree, node.key)
		if len(path) > 0 {
			path = path[:len(path)-1]
		}
		pageHit := seMatchText(page.title, query, true)
		score := pageHit.score
		if len(pageHit.runs) == 0 {
			keywordHit := seMatchText(strings.Join(page.keywords, " "), query, true)
			if len(keywordHit.runs) == 0 {
				score = -1
			} else {
				score = keywordHit.score - 400
			}
		}
		if score >= 0 {
			key := "page:" + page.id + ":" + page.title
			if !seen[key] {
				seen[key] = true
				items = append(items, seItem{
					kind:        "setting",
					title:       page.title,
					setting:     page.id,
					settingPath: append([]string{}, path...),
					indices:     seRunIndices(pageHit.runs),
					score:       score - 120,
				})
			}
		}
		for _, label := range uniqueStrings(page.labels) {
			label = strings.TrimSpace(label)
			if label == "" {
				continue
			}
			hit := seMatchText(label, query, true)
			if len(hit.runs) == 0 {
				continue
			}
			key := "option:" + page.id + ":" + label
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, seItem{
				kind:        "setting",
				title:       label,
				setting:     page.id,
				settingPath: append(append([]string{}, path...), page.title),
				indices:     seRunIndices(hit.runs),
				score:       hit.score - 120,
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
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

// seMatchText ports common/rank.ts matchText. It supports contiguous and
// word/camel-hump matches, Find Action's word-aligned fragments, and the
// upstream multi-word token fallback. Runs are half-open [start,end).
type seMatchTextResult struct {
	score int
	runs  [][2]int
}

func seMatchText(text, query string, wordAligned bool) seMatchTextResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return seMatchTextResult{}
	}
	hit := seFuzzyMatch(text, query)
	if hit != nil && (!wordAligned || seWordAligned(text, hit)) {
		return seMatchTextResult{score: seScoreIndices(text, query, hit), runs: seIndexRuns(hit)}
	}
	tokens := strings.Fields(query)
	if len(tokens) < 2 {
		return seMatchTextResult{}
	}
	all := map[int]bool{}
	score := 0
	for _, token := range tokens {
		tokenHit := seFuzzyMatch(text, token)
		if tokenHit == nil {
			return seMatchTextResult{}
		}
		for _, index := range tokenHit {
			all[index] = true
		}
		score += seScoreIndices(text, token, tokenHit)
	}
	indices := make([]int, 0, len(all))
	for index := range all {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return seMatchTextResult{
		score: int(math.Round(float64(score)/float64(len(tokens)))) - 50,
		runs:  seIndexRuns(indices),
	}
}

func seFuzzyMatch(text, query string) []int {
	runes := []rune(text)
	lowerText := []rune(strings.ToLower(text))
	lowerQuery := []rune(strings.ToLower(query))
	if len(lowerQuery) == 0 {
		return []int{}
	}
	for start := 0; start+len(lowerQuery) <= len(lowerText); start++ {
		if slices.Equal(lowerText[start:start+len(lowerQuery)], lowerQuery) {
			out := make([]int, len(lowerQuery))
			for index := range out {
				out[index] = start + index
			}
			return out
		}
	}
	if hump := seHumpMatch(runes, lowerQuery); hump != nil {
		return hump
	}
	out := make([]int, 0, len(lowerQuery))
	queryAt := 0
	for index, char := range lowerText {
		if queryAt < len(lowerQuery) && char == lowerQuery[queryAt] {
			out = append(out, index)
			queryAt++
		}
	}
	if queryAt != len(lowerQuery) {
		return nil
	}
	return out
}

func seHumpMatch(text, query []rune) []int {
	var out []int
	queryAt := 0
	for index := 0; index < len(text) && queryAt < len(query); index++ {
		if unicode.ToLower(text[index]) != query[queryAt] {
			continue
		}
		aligned := seWordStart(text, index)
		if len(out) > 0 {
			aligned = aligned || out[len(out)-1] == index-1
		}
		if aligned {
			out = append(out, index)
			queryAt++
		}
	}
	if queryAt != len(query) {
		return nil
	}
	return out
}

func seWordStart(text []rune, index int) bool {
	if index <= 0 {
		return true
	}
	previous, current := text[index-1], text[index]
	if strings.ContainsRune(" \t\r\n_-./\\:", previous) {
		return true
	}
	return unicode.IsUpper(current) && unicode.IsLower(previous)
}

func seWordAligned(text string, indices []int) bool {
	if len(indices) < 2 {
		return true
	}
	runes := []rune(text)
	for index := 1; index < len(indices); index++ {
		if indices[index] == indices[index-1]+1 {
			continue
		}
		if !seWordStart(runes, indices[index]) {
			return false
		}
	}
	return true
}

func seScoreIndices(text, query string, indices []int) int {
	lowerText, lowerQuery := strings.ToLower(text), strings.ToLower(query)
	score := 0
	switch {
	case lowerText == lowerQuery:
		score = 1000
	case strings.HasPrefix(lowerText, lowerQuery):
		score = 800
	case seContiguous(indices):
		score = 600 - min(100, max(0, indices[0]))*2
	default:
		runes := []rune(text)
		starts := 0
		for _, index := range indices {
			if seWordStart(runes, index) {
				starts++
			}
		}
		if starts >= min(2, len(indices)) {
			score = 400 + starts*10
		} else {
			spread := 0
			if len(indices) >= 2 {
				spread = min(150, indices[len(indices)-1]-indices[0]-len(indices)+1)
			}
			score = 200 - spread
		}
	}
	if len(indices) > 0 {
		runes := []rune(text)
		if strings.HasPrefix(strings.ToLower(string(runes[indices[0]:])), lowerQuery) {
			score += 5
		}
	}
	return score - min(60, utf8.RuneCountInString(text))
}

func seContiguous(indices []int) bool {
	for index := 1; index < len(indices); index++ {
		if indices[index] != indices[index-1]+1 {
			return false
		}
	}
	return true
}

func seIndexRuns(indices []int) [][2]int {
	var runs [][2]int
	for _, index := range indices {
		if len(runs) > 0 && runs[len(runs)-1][1] == index {
			runs[len(runs)-1][1] = index + 1
		} else {
			runs = append(runs, [2]int{index, index + 1})
		}
	}
	return runs
}

func seRunIndices(runs [][2]int) []int {
	var out []int
	for _, run := range runs {
		for index := run[0]; index < run[1]; index++ {
			out = append(out, index)
		}
	}
	return out
}

func seUintIndices(indices []uint32) []int {
	out := make([]int, len(indices))
	for index, value := range indices {
		out[index] = int(value)
	}
	return out
}

// isTypeKind reports whether a workspace symbol is a class-like declaration,
// which is what the Classes tab keeps.
func isTypeKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "class", "interface", "struct", "enum", "trait", "typealias":
		return true
	}
	return false
}

func seSymbolDisplayName(name, kind string) string {
	switch strings.ToLower(kind) {
	case "function", "method", "constructor":
		if !strings.HasSuffix(name, ")") {
			return name + "()"
		}
	}
	return name
}

// seItemMatchScore mirrors the comparable degree the All tab used when it
// merged name, path, symbol, endpoint and action results.
func seItemMatchScore(title, extra, query string) int {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0
	}
	name := strings.ToLower(title)
	basis := name
	if extra != "" {
		basis += "\n" + strings.ToLower(extra)
	}
	switch {
	case name == q:
		return 4000
	case strings.HasPrefix(name, q):
		return 3000
	case strings.Contains(" "+name, " "+q):
		return 1400
	case strings.Contains(name, q):
		return 1000
	case strings.Contains(basis, q):
		return 500
	default:
		if _, score, ok := fuzzySubsequence(q, basis); ok {
			return 300 + score
		}
	}
	return 0
}

func seItemWeight(item seItem, query string) int {
	switch item.kind {
	case "file":
		if match := seMatchText(item.title, query, false); len(match.runs) > 0 {
			return match.score
		}
		if match := seMatchText(item.rel, query, false); len(match.runs) > 0 {
			return match.score - 200
		}
		return 0
	case "symbol":
		if item.score != 0 {
			return item.score
		}
		return seItemMatchScore(item.title, item.container, query)
	case "action", "setting":
		return item.score
	case "text":
		return 350
	default:
		return 0
	}
}

func seRelativePath(root, path string) string {
	normalizedRoot := strings.TrimRight(strings.ReplaceAll(root, `\`, "/"), "/")
	normalizedPath := strings.ReplaceAll(path, `\`, "/")
	if normalizedRoot != "" && strings.HasPrefix(normalizedPath, normalizedRoot+"/") {
		return strings.TrimPrefix(normalizedPath, normalizedRoot+"/")
	}
	return normalizedPath
}

func sePathInside(root, path string) bool {
	if root == "" {
		return true
	}
	normalizedRoot := strings.TrimRight(strings.ReplaceAll(root, `\`, "/"), "/")
	normalizedPath := strings.ReplaceAll(path, `\`, "/")
	return normalizedPath == normalizedRoot || strings.HasPrefix(normalizedPath, normalizedRoot+"/")
}

// seTextRow collapses the indentation to OpenIDEA's two visible spaces and
// maps the backend's match ranges onto the shortened display text.
func seTextRow(text string, textOffset uint32, ranges [][2]uint32, query string) (string, int, int, []int) {
	runes := []rune(text)
	white := 0
	for white < len(runes) && (runes[white] == ' ' || runes[white] == '\t') {
		white++
	}
	indent := min(white, 2)
	lead := white - indent
	trimmedEnd := len(runes)
	for trimmedEnd > white {
		r := runes[trimmedEnd-1]
		if r != ' ' && r != '\t' && r != '\r' {
			break
		}
		trimmedEnd--
	}
	display := string(runes[:indent]) + string(runes[white:trimmedEnd])
	var indices []int
	for _, match := range ranges {
		start := max(int(match[0]), white)
		end := min(int(match[1]), trimmedEnd)
		for at := start; at < end; at++ {
			position := at - lead
			if position >= 0 && position < len([]rune(display)) {
				indices = append(indices, position)
			}
		}
	}
	if len(indices) == 0 {
		indices = seMatchIndices(display, query)
	}
	col, endCol := 0, 0
	if len(ranges) > 0 {
		col = int(ranges[0][0]) + int(textOffset)
		endCol = int(ranges[0][1]) + int(textOffset)
	}
	return display, col, endCol, indices
}

func cloneSearchHistories(source map[seTab][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string][]string, len(source))
	for tab, queries := range source {
		if len(queries) == 0 {
			continue
		}
		out[string(tab)] = append([]string(nil), queries...)
	}
	return out
}

// seRows flattens the loaded results into the rows the dialog renders. An
// empty query keeps section headers. A typed All query interleaves every
// source by relevance and omits headers, as OpenIDEA's buildRows does.
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
	rows := m.buildSearchEverywhereRows()
	sections := sectionsFor(s.tab, s.seEmpty())
	if len(s.lastRows) == 0 {
		return rows
	}

	current := searchEverywhereRowsBySection(rows)
	previous := searchEverywhereRowsBySection(s.lastRows)
	retained := false
	for _, section := range sections {
		group := current[section]
		if m.searchEverywhereSectionIsLoading(section) {
			if old := previous[section]; len(old.items) > 0 {
				group = s.retainedSearchEverywhereRows(section, old)
				retained = true
			}
		}
		current[section] = group
	}
	if retained {
		return m.composeSearchEverywhereRows(sections, current)
	}
	s.lastRows = cloneSearchEverywhereRows(rows)
	return rows
}

func (m *Model) buildSearchEverywhereRows() []seRow {
	s := m.se
	root := m.Root()
	location := parseSELocationQuery(strings.TrimSpace(s.query), root)
	query := strings.TrimSpace(location.text)

	fileItems := func(section seSection) []seItem {
		source := s.files
		if section == seRecentFiles {
			source = s.recent
		}
		items := make([]seItem, 0, len(source))
		for _, f := range source {
			if section != seRecentFiles {
				if !m.searchEverywherePathAllowed(f.Path, s.fileSet, s.includeNonProject) {
					continue
				}
				if s.tab != seTabAll && s.filters.languageExcluded(s.tab, f.Path, s.languages) {
					continue
				}
			}
			rel := strings.ReplaceAll(f.RelPath, `\`, "/")
			if rel == "" {
				rel = seRelativePath(root, f.Path)
			}
			folder := strings.HasSuffix(rel, "/")
			name := filepath.Base(strings.TrimSuffix(rel, "/"))
			indices := seFileMatchIndices(rel, name, f.Indices)
			line, col := 0, 0
			if location.has {
				line, col = location.line, location.col
			}
			items = append(items, seItem{
				kind:      "file",
				title:     name,
				rel:       rel,
				container: filepath.Dir(strings.TrimSuffix(rel, "/")),
				path:      f.Path,
				line:      line,
				col:       col,
				lineSet:   location.has,
				folder:    folder,
				indices:   indices,
				score:     seItemMatchScore(name, rel, query),
			})
		}
		return items
	}
	symbolItems := func(tab seTab, typesOnly, excludeTypes bool) []seItem {
		type candidate struct {
			item  seItem
			score int
			order int
		}
		var candidates []candidate
		seen := map[string]bool{}
		keep := func(kind string) bool {
			if typesOnly && !isTypeKind(kind) {
				return false
			}
			if excludeTypes && isTypeKind(kind) {
				return false
			}
			return s.tab == seTabAll || !s.filters.kindExcluded(tab, kind)
		}
		for index, sym := range s.indexSymbols {
			if !keep(sym.Kind) {
				continue
			}
			if s.tab != seTabAll && s.filters.languageExcluded(tab, sym.Path, s.languages) ||
				!m.searchEverywherePathAllowed(sym.Path, s.fileSet, s.includeNonProject) {
				continue
			}
			key := fmt.Sprintf("%s\x00%d\x00%s", sym.Path, sym.Range.Start.Line, sym.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			container := ""
			if sym.Container != nil {
				container = *sym.Container
			}
			score := 3000 - index
			if strings.EqualFold(sym.Name, query) {
				score = 4000 - index
			}
			candidates = append(candidates, candidate{
				item: seItem{
					kind:      "symbol",
					symbol:    strings.ToLower(sym.Kind),
					title:     seSymbolDisplayName(sym.Name, sym.Kind),
					rel:       sym.RelPath,
					container: container,
					right:     filepath.Base(sym.Path),
					path:      sym.Path,
					line:      int(sym.Range.Start.Line),
					col:       int(sym.Range.Start.Col),
					endCol:    int(sym.Range.Start.Col) + len([]rune(sym.Name)),
					lineSet:   true,
					indices:   seUintIndices(sym.Indices),
					score:     score,
				},
				score: score,
				order: len(candidates),
			})
		}
		for index, sym := range s.symbols {
			if !keep(sym.Kind) {
				continue
			}
			if s.tab != seTabAll && s.filters.languageExcluded(tab, sym.Location.Path, s.languages) ||
				!m.searchEverywherePathAllowed(sym.Location.Path, s.fileSet, s.includeNonProject) {
				continue
			}
			key := fmt.Sprintf("%s\x00%d\x00%s", sym.Location.Path, sym.Location.Range.Start.Line, sym.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			container := ""
			if sym.Container != nil {
				container = *sym.Container
			}
			hit := seMatchText(sym.Name, query, false)
			if len(hit.runs) == 0 {
				continue
			}
			score := hit.score
			if strings.EqualFold(sym.Name, query) {
				score = 3500
			}
			candidates = append(candidates, candidate{
				item: seItem{
					kind:      "symbol",
					symbol:    strings.ToLower(sym.Kind),
					title:     seSymbolDisplayName(sym.Name, sym.Kind),
					rel:       seRelativePath(root, sym.Location.Path),
					container: container,
					right:     filepath.Base(sym.Location.Path),
					path:      sym.Location.Path,
					line:      int(sym.Location.Range.Start.Line),
					col:       int(sym.Location.Range.Start.Col),
					endCol:    int(sym.Location.Range.Start.Col) + len([]rune(sym.Name)),
					lineSet:   true,
					indices:   seRunIndices(hit.runs),
					score:     score,
				},
				score: score,
				order: len(candidates) + index,
			})
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].score != candidates[j].score {
				return candidates[i].score > candidates[j].score
			}
			return candidates[i].order < candidates[j].order
		})
		items := make([]seItem, len(candidates))
		for index := range candidates {
			items[index] = candidates[index].item
		}
		return items
	}
	textItems := func() []seItem {
		limit := s.seSectionLimit(seText)
		items := make([]seItem, 0, limit)
		for _, f := range s.text {
			if s.tab == seTabText && !seTextMaskMatches(s.filters.Text, f.Path) {
				continue
			}
			for _, hit := range f.Matches {
				text, col, endCol, indices := seTextRow(hit.Text, hit.TextOffset, hit.Ranges, query)
				rel := f.RelPath
				if rel == "" {
					rel = seRelativePath(root, f.Path)
				}
				items = append(items, seItem{
					kind:    "text",
					title:   text,
					rel:     rel,
					right:   filepath.Base(f.Path),
					path:    f.Path,
					line:    int(hit.Line),
					col:     col,
					endCol:  endCol,
					lineSet: true,
					indices: indices,
					score:   350,
				})
				if len(items) >= limit {
					return items
				}
			}
		}
		return items
	}
	sectionItems := func(section seSection) []seItem {
		switch section {
		case seRecentFiles:
			return fileItems(section)
		case seRecentActions:
			return m.seRecentActionItems()
		case seClasses:
			return symbolItems(seTabClasses, true, false)
		case seFiles:
			return fileItems(section)
		case seSymbols:
			return symbolItems(seTabSymbols, false, s.tab == seTabAll)
		case seEndpoints:
			items := seEndpointItems(s.endpoints, query, s.seSectionLimit(seEndpoints))
			for index := range items {
				items[index].rel = seRelativePath(root, items[index].path)
			}
			return items
		case seActions:
			actions := m.seActionItems(query)
			if s.tab == seTabActions && !s.includeNonProject {
				filtered := actions[:0]
				for _, item := range actions {
					if !item.disabled {
						filtered = append(filtered, item)
					}
				}
				actions = filtered
			}
			merged := append(actions, m.seSettingItems(query)...)
			sort.SliceStable(merged, func(i, j int) bool {
				return merged[i].score > merged[j].score
			})
			return merged
		case seText:
			return textItems()
		default:
			return nil
		}
	}

	sections := sectionsFor(s.tab, s.seEmpty())
	if !s.seEmpty() && s.tab == seTabAll {
		type mixedItem struct {
			item        seItem
			section     seSection
			weight      int
			sectionRank int
			index       int
		}
		var mixed []mixedItem
		hasMore := map[seSection]bool{}
		for rank, section := range sections {
			all := sectionItems(section)
			limit := s.seSectionLimit(section)
			capWeight := int(^uint(0) >> 1)
			for index, item := range all {
				if index >= limit {
					hasMore[section] = true
					break
				}
				weight := seItemWeight(item, query)
				if weight > capWeight {
					weight = capWeight
				}
				capWeight = weight
				mixed = append(mixed, mixedItem{
					item: item, section: section, weight: weight,
					sectionRank: rank, index: index,
				})
			}
		}
		sort.SliceStable(mixed, func(i, j int) bool {
			if mixed[i].weight != mixed[j].weight {
				return mixed[i].weight > mixed[j].weight
			}
			if mixed[i].sectionRank != mixed[j].sectionRank {
				return mixed[i].sectionRank < mixed[j].sectionRank
			}
			return mixed[i].index < mixed[j].index
		})
		rows := make([]seRow, 0, len(mixed)+len(sections))
		for _, mixed := range mixed {
			rows = append(rows, seRow{section: mixed.section, item: mixed.item})
		}
		for _, section := range sections {
			if hasMore[section] {
				rows = append(rows, seRow{section: section, more: true})
			}
		}
		return rows
	}

	var rows []seRow
	for _, section := range sections {
		items := sectionItems(section)
		if len(items) == 0 {
			continue
		}
		limit := s.seSectionLimit(section)
		if s.seEmpty() {
			rows = append(rows, seRow{section: section, header: true})
		}
		for index, item := range items {
			if index >= limit {
				rows = append(rows, seRow{section: section, more: true})
				break
			}
			rows = append(rows, seRow{section: section, item: item})
		}
	}
	return rows
}

type searchEverywhereSectionRows struct {
	items []seItem
	more  bool
}

func cloneSearchEverywhereRows(rows []seRow) []seRow {
	return append([]seRow(nil), rows...)
}

// searchEverywhereRowsBySection separates a rendered snapshot without losing
// each source's order. OpenIDEA keeps section items while that section is
// loading and publishes the first new item from it.
func searchEverywhereRowsBySection(rows []seRow) map[seSection]searchEverywhereSectionRows {
	groups := map[seSection]searchEverywhereSectionRows{}
	for _, row := range rows {
		if row.header {
			continue
		}
		group := groups[row.section]
		if row.more {
			group.more = true
		} else {
			group.items = append(group.items, row.item)
		}
		groups[row.section] = group
	}
	return groups
}

func (m *Model) searchEverywhereSectionIsLoading(section seSection) bool {
	s := m.se
	if s == nil {
		return false
	}
	switch section {
	case seRecentFiles, seFiles:
		return !s.fileDone
	case seClasses, seSymbols:
		return !s.symbolDone
	case seEndpoints:
		return !s.endpointDone || s.pluginsBusy
	case seText:
		return !s.textDone || s.textBusy
	}
	return false
}

func (m *Model) composeSearchEverywhereRows(
	sections []seSection,
	groups map[seSection]searchEverywhereSectionRows,
) []seRow {
	s := m.se
	if s == nil {
		return nil
	}
	query := strings.TrimSpace(parseSELocationQuery(strings.TrimSpace(s.query), m.Root()).text)
	if s.tab == seTabAll && !s.seEmpty() {
		type mixedItem struct {
			item        seItem
			section     seSection
			weight      int
			sectionRank int
			index       int
		}
		var mixed []mixedItem
		hasMore := map[seSection]bool{}
		for rank, section := range sections {
			capWeight := int(^uint(0) >> 1)
			for index, item := range groups[section].items {
				if index >= s.seSectionLimit(section) {
					hasMore[section] = true
					break
				}
				weight := seItemWeight(item, query)
				if weight > capWeight {
					weight = capWeight
				}
				capWeight = weight
				mixed = append(mixed, mixedItem{
					item: item, section: section, weight: weight,
					sectionRank: rank, index: index,
				})
			}
		}
		sort.SliceStable(mixed, func(i, j int) bool {
			if mixed[i].weight != mixed[j].weight {
				return mixed[i].weight > mixed[j].weight
			}
			if mixed[i].sectionRank != mixed[j].sectionRank {
				return mixed[i].sectionRank < mixed[j].sectionRank
			}
			return mixed[i].index < mixed[j].index
		})
		rows := make([]seRow, 0, len(mixed)+len(sections))
		for _, item := range mixed {
			rows = append(rows, seRow{section: item.section, item: item.item})
		}
		for _, section := range sections {
			if groups[section].more || hasMore[section] {
				rows = append(rows, seRow{section: section, more: true})
			}
		}
		return rows
	}

	var rows []seRow
	for _, section := range sections {
		group := groups[section]
		if len(group.items) == 0 && !group.more {
			continue
		}
		if s.seEmpty() {
			rows = append(rows, seRow{section: section, header: true})
		}
		for _, item := range group.items {
			rows = append(rows, seRow{section: section, item: item})
		}
		if group.more {
			rows = append(rows, seRow{section: section, more: true})
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
		return "setting:" + it.setting + ":" + it.title
	default:
		return fmt.Sprintf("%s:%s:%d:%d:%s", it.kind, it.path, it.line, it.col, it.title)
	}
}

// selectSERow applies OpenIDEA's row-selection modes without activating the
// result. Shift extends from the anchor and a primary-modifier click toggles
// one row; a plain click leaves only that row selected.
func (m *Model) selectSERow(index int, mode recentSelectionMode) {
	s := m.se
	if s == nil {
		return
	}
	rows := m.seRows()
	if index < 0 || index >= len(rows) || !seSelectable(rows[index]) {
		return
	}
	switch mode {
	case recentSelectionRange:
		anchor := clampSymbolSel(s.selAnchor, len(rows))
		if s.selKeys == nil {
			anchor = clampSymbolSel(s.sel, len(rows))
		}
		start, end := anchor, index
		if start > end {
			start, end = end, start
		}
		keys := make(map[string]bool, end-start+1)
		for _, row := range rows[start : end+1] {
			if seSelectable(row) {
				keys[seRowKey(row)] = true
			}
		}
		s.sel = index
		s.selKey = seRowKey(rows[index])
		s.selAnchor = anchor
		s.selKeys = keys
	case recentSelectionToggle:
		keys := make(map[string]bool, len(s.selKeys)+1)
		if s.selKeys == nil {
			if s.sel >= 0 && s.sel < len(rows) && seSelectable(rows[s.sel]) {
				keys[seRowKey(rows[s.sel])] = true
			}
		} else {
			for key, selected := range s.selKeys {
				if selected {
					keys[key] = true
				}
			}
		}
		key := seRowKey(rows[index])
		if keys[key] {
			delete(keys, key)
			if len(keys) == 0 {
				s.sel = index
				s.selKey = key
				s.selAnchor = index
				s.selKeys = nil
				break
			}
			lead := index
			for rowIndex := len(rows) - 1; rowIndex >= 0; rowIndex-- {
				if keys[seRowKey(rows[rowIndex])] {
					lead = rowIndex
					break
				}
			}
			s.sel = lead
			s.selKey = seRowKey(rows[lead])
			s.selAnchor = lead
			s.selKeys = keys
			break
		}
		keys[key] = true
		s.sel = index
		s.selKey = key
		s.selAnchor = index
		s.selKeys = keys
	default:
		s.sel = index
		s.selKey = seRowKey(rows[index])
		s.selAnchor = index
		s.selKeys = nil
	}
	s.revealSelection = true
	m.syncSearchEverywherePreview()
	m.invalidate()
}

// isSERowSelected reports the lead and the additional selected rows.
func (s *sePane) isSERowSelected(index int, row seRow) bool {
	if s == nil || index < 0 {
		return false
	}
	if index == s.sel {
		return true
	}
	return s.selKeys != nil && s.selKeys[seRowKey(row)]
}

// selectedSERows returns the selected rows in list order, as OpenIDEA's
// selectedRows helper does. With one-row selection it returns the lead.
func (m *Model) selectedSERows(rows []seRow) []seRow {
	s := m.se
	if s == nil || len(rows) == 0 || s.sel < 0 || s.sel >= len(rows) ||
		!seSelectable(rows[s.sel]) {
		return nil
	}
	if s.selKeys == nil {
		return []seRow{rows[s.sel]}
	}
	out := make([]seRow, 0, len(s.selKeys))
	for _, row := range rows {
		if seSelectable(row) && s.selKeys[seRowKey(row)] {
			out = append(out, row)
		}
	}
	if len(out) == 0 {
		return []seRow{rows[s.sel]}
	}
	return out
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
	if len(rows) == 0 {
		return
	}
	i := s.sel + delta
	for i >= 0 && i < len(rows) && !seSelectable(rows[i]) {
		i += delta
	}
	if i < 0 || i >= len(rows) {
		if delta > 0 {
			i = 0
		} else {
			i = len(rows) - 1
		}
		for i >= 0 && i < len(rows) && !seSelectable(rows[i]) {
			i += delta
		}
	}
	if i < 0 || i >= len(rows) {
		return
	}
	s.sel = i
	s.selKey = seRowKey(rows[i])
	s.selAnchor = i
	s.selKeys = nil
	s.revealSelection = true
	m.syncSearchEverywherePreview()
	m.invalidate()
}

// jumpSearchEverywhereEdge ports List's Home and End keys.
func (m *Model) jumpSearchEverywhereEdge(last bool) {
	s := m.se
	if s == nil {
		return
	}
	rows := m.seRows()
	index, step := 0, 1
	if last {
		index, step = len(rows)-1, -1
	}
	for index >= 0 && index < len(rows) && !seSelectable(rows[index]) {
		index += step
	}
	if index < 0 || index >= len(rows) {
		return
	}
	m.selectSERow(index, recentSelectionSingle)
}

// stepSearchEverywhereGroup ports PageUp/PageDown and the primary-modifier
// arrow shortcuts: the next/previous first row of a section.
func (m *Model) stepSearchEverywhereGroup(delta int) {
	s := m.se
	if s == nil {
		return
	}
	rows := m.seRows()
	starts := make([]int, 0)
	last := seSection("")
	for index, row := range rows {
		if row.header || row.more || !seSelectable(row) {
			continue
		}
		if row.section != last || len(starts) == 0 {
			starts = append(starts, index)
			last = row.section
		}
	}
	if len(starts) < 2 {
		return
	}
	target := starts[len(starts)-1]
	if delta > 0 {
		for _, start := range starts {
			if start > s.sel {
				target = start
				break
			}
		}
	} else {
		current := starts[0]
		for index := len(starts) - 1; index >= 0; index-- {
			if starts[index] <= s.sel {
				current = starts[index]
				break
			}
		}
		target = current
		if target == s.sel {
			for index := len(starts) - 1; index >= 0; index-- {
				if starts[index] < s.sel {
					target = starts[index]
					break
				}
			}
		}
	}
	s.sel = target
	s.selKey = seRowKey(rows[target])
	s.selAnchor = target
	s.selKeys = nil
	s.revealSelection = true
	m.syncSearchEverywherePreview()
	m.invalidate()
}

// clampSESelection keeps the selection on an item after the rows changed.
func (m *Model) clampSESelection() {
	s := m.se
	if s == nil {
		return
	}
	rows := m.seRows()
	if s.selKeys != nil {
		valid := make(map[string]bool, len(s.selKeys))
		for _, row := range rows {
			if seSelectable(row) && s.selKeys[seRowKey(row)] {
				valid[seRowKey(row)] = true
			}
		}
		if len(valid) == 0 {
			s.selKeys = nil
		} else {
			s.selKeys = valid
		}
	}
	if s.selKey != "" {
		for i, row := range rows {
			if seSelectable(row) && seRowKey(row) == s.selKey {
				s.sel = i
				return
			}
		}
	}
	if s.sel < 0 {
		for i, row := range rows {
			if !seSelectable(row) || row.more {
				continue
			}
			s.sel = i
			s.selKey = seRowKey(row)
			s.selKeys = nil
			s.revealSelection = true
			return
		}
	}
	if s.sel >= 0 && s.sel < len(rows) && seSelectable(rows[s.sel]) {
		s.selKey = seRowKey(rows[s.sel])
		return
	}
	if s.sel >= len(rows) || (s.sel >= 0 && !seSelectable(rows[s.sel])) {
		for i, r := range rows {
			if seSelectable(r) && !r.more {
				s.sel = i
				s.selKey = seRowKey(r)
				s.revealSelection = true
				return
			}
		}
		s.sel = -1
		s.selKey = ""
		s.selKeys = nil
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
	if s.searchEverywhereLoading() {
		s.pendingEnter = true
		s.pendingSide = side
		m.invalidate()
		return
	}
	m.activateSearchEverywhereNow(side)
}

func (m *Model) activateSearchEverywhereNow(side bool) {
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
	if selected := m.selectedSERows(rows); len(selected) > 1 {
		navigable := make([]seRow, 0, len(selected))
		for _, candidate := range selected {
			if searchEverywhereCanSplit(candidate) {
				navigable = append(navigable, candidate)
			}
		}
		if len(navigable) > 0 {
			m.openSearchEverywhereRows(navigable, seRowKey(rows[s.sel]), side)
			return
		}
		m.CloseSearchEverywhere()
		return
	}
	row := rows[s.sel]
	if row.more {
		step := seMoreStepTab
		if s.tab == seTabAll {
			step = seMoreStepAll
		}
		previousLimit := s.seSectionLimit(row.section)
		s.extra[row.section] += step
		seen := 0
		s.selKey = ""
		s.selAnchor = 0
		s.selKeys = nil
		for _, candidate := range m.seRows() {
			if candidate.header || candidate.more || candidate.section != row.section {
				continue
			}
			if seen == previousLimit {
				s.selKey = seRowKey(candidate)
				break
			}
			seen++
		}
		m.clampSESelection()
		m.syncSearchEverywherePreview()
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
		s.selAnchor = 0
		s.selKeys = nil
		s.extra = map[seSection]int{}
		s.filterOpen = false
		s.historyIndex = -1
		m.refreshSearchEverywhere()
	case "action":
		if item.disabled {
			return
		}
		m.rememberRecentAction(item.action)
		if action, ok := actionGet(item.action); ok && action.Toggled != nil {
			m.RunCommand(item.action)
			m.invalidate()
			return
		}
		m.CloseSearchEverywhere()
		m.RunCommand(item.action)
	case "setting":
		m.rememberSearchQuery(s.tab, strings.TrimSpace(s.query))
		m.CloseSearchEverywhere()
		m.OpenSettingsPage(item.setting)
	case "file":
		m.rememberSearchQuery(s.tab, strings.TrimSpace(s.query))
		m.CloseSearchEverywhere()
		if item.folder {
			m.selectInProjectView(item.path)
		} else {
			if side {
				m.reveal = &revealPos{
					path: item.path, line: item.line, col: item.col,
					endLine: -1, endCol: -1,
				}
				m.openWith(item.path, openSpec{
					at: -1,
					done: func(tab *Tab) {
						m.revealSearchEverywhereTarget(tab, item)
						m.moveRecentTabToSide(tab, true)
					},
				})
			} else {
				m.revealAt(item.path, item.line, item.col)
			}
		}
	case "symbol", "text":
		m.rememberSearchQuery(s.tab, strings.TrimSpace(s.query))
		m.CloseSearchEverywhere()
		if side {
			endLine, endCol := -1, -1
			if item.kind == "text" && item.endCol > item.col {
				endLine, endCol = item.line, item.endCol
			}
			m.reveal = &revealPos{
				path: item.path, line: item.line, col: item.col,
				endLine: endLine, endCol: endCol,
			}
			m.openWith(item.path, openSpec{
				at: -1,
				done: func(tab *Tab) {
					m.revealSearchEverywhereTarget(tab, item)
					m.moveRecentTabToSide(tab, true)
				},
			})
		} else if item.kind == "text" && item.endCol > item.col {
			m.revealRange(item.path, item.line, item.col, item.line, item.endCol)
		} else {
			m.revealAt(item.path, item.line, item.col)
		}
	default:
		m.invalidate()
	}
}

// openSearchEverywhereRows ports OpenIDEA's multi-result openInListOrder:
// every chosen navigation result opens as a regular tab, then the lead row
// receives focus and its caret or selection position.
func (m *Model) openSearchEverywhereRows(rows []seRow, leadKey string, side bool) {
	s := m.se
	if s == nil {
		return
	}
	m.rememberSearchQuery(s.tab, strings.TrimSpace(s.query))
	m.CloseSearchEverywhere()

	targets := make([]seRow, 0, len(rows))
	for _, row := range rows {
		if searchEverywhereCanSplit(row) {
			targets = append(targets, row)
		}
	}
	if len(targets) == 0 {
		return
	}

	var openNext func(int)
	openNext = func(index int) {
		if index >= len(targets) {
			for _, row := range targets {
				if seRowKey(row) == leadKey {
					m.focusSearchEverywhereTarget(row, side)
					return
				}
			}
			m.focusSearchEverywhereTarget(targets[len(targets)-1], side)
			return
		}
		item := targets[index].item
		m.openWith(item.path, openSpec{
			at: -1,
			done: func(tab *Tab) {
				m.revealSearchEverywhereTarget(tab, item)
				if side {
					m.moveRecentTabToSide(tab, false)
				}
				openNext(index + 1)
			},
			failed: func(string, error) {
				openNext(index + 1)
			},
		})
	}
	openNext(0)
}

func (m *Model) focusSearchEverywhereTarget(row seRow, side bool) {
	item := row.item
	m.focusRecentLead(item.path, side)
	var tab *Tab
	switch {
	case side && m.split != nil:
		for _, candidate := range m.split.tabs {
			if samePath(tabPath(candidate), item.path) {
				tab = candidate
				break
			}
		}
	default:
		for _, candidate := range m.tabs {
			if samePath(tabPath(candidate), item.path) {
				tab = candidate
				break
			}
		}
	}
	if tab != nil {
		m.revealSearchEverywhereTarget(tab, item)
	}
}

func (m *Model) revealSearchEverywhereTarget(tab *Tab, item seItem) {
	if tab == nil {
		return
	}
	endLine := -1
	endCol := -1
	if item.kind == "text" && item.endCol > item.col {
		endLine = item.line
		endCol = item.endCol
	}
	m.reveal = &revealPos{
		path: item.path, line: item.line, col: item.col,
		endLine: endLine, endCol: endCol,
	}
	m.applyReveal(tab)
}

func (m *Model) searchHistoryFor(tab seTab) []string {
	if m.searchHistories == nil {
		m.searchHistories = map[seTab][]string{}
		legacy := append([]string(nil), m.searchHistory...)
		if len(legacy) > 0 {
			m.searchHistories[seTabAll] = legacy
		}
	}
	return m.searchHistories[tab]
}

// rememberSearchQuery puts an executed query first in its tab's history,
// once, as OpenIDEA's Search Everywhere history does.
func (m *Model) rememberSearchQuery(tab seTab, query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	m.searchHistoryFor(tab)
	list := []string{query}
	for _, old := range m.searchHistories[tab] {
		if !strings.EqualFold(old, query) {
			list = append(list, old)
		}
	}
	if len(list) > seHistoryCap {
		list = list[:seHistoryCap]
	}
	m.searchHistories[tab] = list
	if tab == seTabAll {
		m.searchHistory = append([]string(nil), list...)
	}
	m.saveSession()
}

// BrowseSearchEverywhereHistory walks the current tab's history, as Alt+Up
// and Alt+Down do in OpenIDEA's query field.
func (m *Model) BrowseSearchEverywhereHistory(delta int) {
	s := m.se
	if s == nil || delta == 0 {
		return
	}
	history := m.searchHistoryFor(s.tab)
	if len(history) == 0 {
		return
	}
	position := s.historyIndex
	if position < -1 {
		position = -1
	}
	if delta > 0 {
		position = min(len(history)-1, position+delta)
	} else {
		position = max(-1, position+delta)
	}
	if position == s.historyIndex {
		return
	}
	if s.historyIndex == -1 {
		s.historyDraft = s.query
	}
	s.historyIndex = position
	if position == -1 {
		s.query = s.historyDraft
	} else {
		s.query = history[position]
	}
	m.refreshSearchEverywhere()
	m.invalidate()
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

func (m *Model) clearSearchEverywherePreview() {
	s := m.se
	if s == nil {
		return
	}
	s.previewKey = ""
	s.previewPath = ""
	s.previewLine = 0
	s.previewCol = 0
	s.previewEndCol = 0
	s.previewFrom = 0
	s.previewLines = nil
	s.previewTokens = nil
	s.previewErr = ""
}

func (m *Model) selectedSearchEverywhereItem() (seItem, bool) {
	s := m.se
	if s == nil {
		return seItem{}, false
	}
	rows := m.seRows()
	if s.sel < 0 || s.sel >= len(rows) || !seSelectable(rows[s.sel]) {
		return seItem{}, false
	}
	return rows[s.sel].item, true
}

// syncSearchEverywherePreview reads the selected file once and keeps twenty
// source lines around its match, as OpenIDEA's CodePreview does.
func (m *Model) syncSearchEverywherePreview() {
	s := m.se
	if s == nil {
		return
	}
	if !s.preview {
		m.clearSearchEverywherePreview()
		return
	}
	item, ok := m.selectedSearchEverywhereItem()
	if !ok || item.path == "" || item.folder {
		m.clearSearchEverywherePreview()
		return
	}
	key := fmt.Sprintf("%s:%d:%d:%d", item.path, item.line, item.col, item.endCol)
	if key == s.previewKey {
		return
	}
	m.clearSearchEverywherePreview()
	s.previewKey = key
	s.previewPath = item.path
	s.previewLine = item.line
	s.previewCol = item.col
	s.previewEndCol = item.endCol
	info, err := os.Stat(item.path)
	if err != nil {
		s.previewErr = err.Error()
		return
	}
	if info.IsDir() {
		s.previewErr = "Choose a file to preview its source"
		return
	}
	if info.Size() > sePreviewBytes {
		s.previewErr = "The result is too large to preview"
		return
	}
	raw, err := os.ReadFile(item.path)
	if err != nil {
		s.previewErr = err.Error()
		return
	}
	source := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(source, "\n")
	line := max(0, min(item.line, len(lines)-1))
	from := max(0, line-sePreviewLines/2)
	to := min(len(lines), from+sePreviewLines)
	if to-from < sePreviewLines {
		from = max(0, to-sePreviewLines)
	}
	s.previewFrom = from
	s.previewLine = line
	s.previewLines = append([]string(nil), lines[from:to]...)
	lexed := highlight.Lex(highlight.Lexer(item.path), source)
	s.previewTokens = make([][]highlight.Token, len(s.previewLines))
	for index := range s.previewLines {
		row := from + index
		if row < len(lexed) {
			s.previewTokens[index] = lexed[row].Tokens
		}
	}
}

// searchEverywhereDialog draws the overlay.
func (m *Model) searchEverywhereDialog(c *ui.Context, p *ui.Theme) {
	s := m.se
	if s == nil {
		return
	}
	open := s.open
	oi := oiPaletteOf(p)
	width := float32(seDialogWidth)
	if m.editorWidth > 0 {
		width = min(width, max(520, m.editorWidth-48))
	}
	top := float32(36)
	if m.editorHeight > 0 {
		top = max(28, min(96, m.editorHeight*0.09))
	}
	dialog := nativeTopSurfaceModal(c, &open, top, func() {
		ui.Column(c).Width(width).Background(oi.popup).
			Border(1, oi.popupBorder).Radius(6).Clip().Children(func() {
			m.searchEverywhereHeader(c, p, s)
			m.searchEverywhereQuery(c, p, s)
			rows := m.seRows()
			m.clampSESelection()
			if s.preview && len(rows) > 0 {
				previewHeight := float32(532)
				if m.editorHeight > 0 {
					previewHeight = min(previewHeight, max(280, m.editorHeight-180))
				}
				ui.SplitPercent(c, &s.previewRatio,
					func() { m.searchEverywhereResults(c, p, s, rows) },
					func() { m.searchEverywherePreviewPane(c, p, s) },
				).Height(previewHeight)
			} else {
				m.searchEverywhereResults(c, p, s, rows)
			}
			if len(rows) > 0 {
				m.searchEverywhereFooter(c, p, s, rows)
			}
		})
	})
	if dialog.Dismissed() || !open {
		m.CloseSearchEverywhere()
	}
}

func (m *Model) searchEverywhereHeader(c *ui.Context, p *ui.Theme, s *sePane) {
	oi := oiPaletteOf(p)
	filterButton := ui.Element{}
	filterButtonBuilt := false
	ui.Row(c).Height(44).Shrink(0).Padding(0, 8).
		Background(oi.frame).BorderWidth(0, 0, 1, 0).BorderColor(oi.popupBorder).
		AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(2).AlignItems(ui.Center).Children(func() {
			for _, tab := range seTabs {
				tab := tab
				active := s.tab == tab.id
				button := ui.ButtonBase(c.Key("se:tab:"+string(tab.id))).
					Height(26).Padding(0, 9).Radius(4).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).BorderWidth(1, 1, 1, 1).
					BorderColor(ui.Color{})
				if active {
					button.Background(oi.tabSelected).BorderColor(oi.tabSelectedBorder)
				} else if button.Hovered() {
					button.Background(oi.toolbarHover)
				}
				button.Children(func() {
					tone := oi.text
					if active {
						tone = oi.textStrong
					}
					ui.Text(c, tab.label).FontSize(12).TextColor(tone).SingleLine()
				})
				button.OnClick(func() { m.SetSearchEverywhereTab(tab.id) })
			}
		})
		ui.Spacer(c)
		if s.tab == seTabAll || s.tab == seTabActions {
			checked := s.includeNonProject
			label := "Include non-project items"
			if s.tab == seTabActions {
				label = "Include disabled actions"
			}
			if ui.Checkbox(c, &checked, label).FontSize(11).Changed() {
				m.setSearchEverywhereIncludeNonProject(checked)
			}
		} else {
			scope := "all"
			if s.includeNonProject {
				scope = "allPlaces"
			} else {
				scope = s.fileSet
			}
			options := []string{"All Places", "Project Files", "Project Production Files", "Project Test Files"}
			label := map[string]string{
				"allPlaces": "All Places", "all": "Project Files",
				"production": "Project Production Files", "test": "Project Test Files",
			}[scope]
			if label == "" {
				label = "Project Files"
			}
			if ui.Select(c, &label, options).Width(170).Changed() {
				switch label {
				case "All Places":
					m.setSearchEverywhereIncludeNonProject(true)
				case "Project Production Files":
					m.setSearchEverywhereFileSet("production")
				case "Project Test Files":
					m.setSearchEverywhereFileSet("test")
				default:
					m.setSearchEverywhereFileSet("all")
				}
			}
		}
		previewButton := m.searchEverywhereIconButton(c, "se:preview", "general/listFiles",
			"Preview", s.preview, oi)
		previewButton.OnClick(func() { m.setSearchEverywherePreview(!s.preview) })
		if seFilterTab(s.tab) {
			filterButton = m.searchEverywhereIconButton(c, "se:filter", "general/filter",
				"Filter", s.filterOpen || s.filters.active(s.tab), oi)
			filterButtonBuilt = true
			filterButton.OnClick(func() { s.filterOpen = !s.filterOpen })
		}
		findButton := m.searchEverywhereIconButton(c, "se:find-tool-window", "general/openInToolWindow",
			"Open in Find Tool Window", false, oi)
		findButton.OnClick(func() { m.openSearchEverywhereInFindToolWindow() })
	})
	if filterButtonBuilt {
		ui.Popover(c, filterButton, &s.filterOpen, func() {
			ui.Column(c).Width(250).Padding(6, 4).Gap(2).Children(func() {
				m.seFilterPanel(c, p, s)
			})
		})
	}
}

func (m *Model) searchEverywhereIconButton(
	c *ui.Context, key, icon, tooltip string, pressed bool, oi oiColors,
) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Size(24, 24).Shrink(0).Radius(4).
		Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if pressed {
		button.Background(oi.toolbarHover)
	} else if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Tooltip(tooltip)
	button.Children(func() { oiIconEl(c, icon, 16, oi, oi.icon) })
	return button
}

func (m *Model) searchEverywhereQuery(c *ui.Context, p *ui.Theme, s *sePane) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(44).Shrink(0).Padding(6, 8).Children(func() {
		field := ui.Row(c).Height(32).Grow(1).Gap(6).Padding(0, 10).
			Background(oi.popup).Border(2, oi.borderStrong).Radius(4).
			AlignItems(ui.Center)
		field.Children(func() {
			historyButton := ui.ButtonBase(c.Key("se:history")).Height(22).Radius(4).
				Padding(0, 2).Gap(1).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if historyButton.Hovered() {
				historyButton.Background(oi.toolbarHover)
			}
			historyButton.Tooltip("Search history")
			historyButton.Children(func() {
				oiIconEl(c, "general/search", 16, oi, oi.textSubtle)
				oiIconEl(c, "general/chevronDown", 10, oi, oi.textSubtle)
			})
			historyButton.Menu(func(menu *ui.Menu) {
				history := m.searchHistoryFor(s.tab)
				if len(history) == 0 {
					menu.Item("No recent searches").Disabled(true)
					return
				}
				menu.Item("Recent Searches").Disabled(true)
				for _, query := range history[:min(15, len(history))] {
					query := query
					if menu.Item(query).Chosen() {
						s.query = query
						s.historyIndex = -1
						m.refreshSearchEverywhere()
					}
				}
			})
			input := ui.TextInputBase(c, &s.query).Grow(1).MinWidth(0).
				FontSize(12).TextColor(oi.textStrong).AutoFocus()
			input.OnChange(func() {
				s.historyIndex = -1
				m.refreshSearchEverywhere()
			})
			input.OnShortcut(0, ui.KeyDown, func() { m.stepSearchEverywhere(1) })
			input.OnShortcut(0, ui.KeyUp, func() { m.stepSearchEverywhere(-1) })
			input.OnShortcut(0, ui.KeyHome, func() { m.jumpSearchEverywhereEdge(false) })
			input.OnShortcut(0, ui.KeyEnd, func() { m.jumpSearchEverywhereEdge(true) })
			input.OnShortcut(0, ui.KeyPageDown, func() { m.stepSearchEverywhereGroup(1) })
			input.OnShortcut(0, ui.KeyPageUp, func() { m.stepSearchEverywhereGroup(-1) })
			primary := sePrimaryModifier()
			input.OnShortcut(primary, ui.KeyDown, func() { m.stepSearchEverywhereGroup(1) })
			input.OnShortcut(primary, ui.KeyUp, func() { m.stepSearchEverywhereGroup(-1) })
			input.OnShortcut(ui.Alt, ui.KeyUp, func() { m.BrowseSearchEverywhereHistory(1) })
			input.OnShortcut(ui.Alt, ui.KeyDown, func() { m.BrowseSearchEverywhereHistory(-1) })
			input.OnShortcut(0, ui.KeyTab, func() { m.CycleSearchEverywhereTab(1) })
			input.OnShortcut(ui.Shift, ui.KeyTab, func() { m.CycleSearchEverywhereTab(-1) })
			input.OnShortcut(0, ui.KeyEnter, func() { m.activateSearchEverywhere(false) })
			input.OnShortcut(ui.Shift, ui.KeyEnter, func() { m.activateSearchEverywhere(true) })
			input.OnShortcut(ui.Alt, ui.KeyEnter, func() { m.openSearchEverywhereIntentions() })
			if s.tab == seTabText {
				input.OnShortcut(primary, ui.KeyEnter, func() { m.openSearchEverywhereInFindToolWindow() })
			}
			if s.searchEverywhereLoading() && !s.seEmpty() {
				ui.Spinner(c).Size(14, 14).TextColor(oi.textSubtle).Label("Searching")
			}
			hint := ""
			switch s.tab {
			case seTabAll:
				hint = "Type / to see commands"
			case seTabText:
				if sePrimaryModifier() == ui.Cmd {
					hint = "⌘↵ to open in Find tool window"
				} else {
					hint = "Ctrl+Enter to open in Find tool window"
				}
			}
			if hint != "" {
				ui.Text(c, hint).FontSize(11).TextColor(oi.textFaint).SingleLine()
			}
		})
	})
}

func (m *Model) searchEverywhereResults(c *ui.Context, p *ui.Theme, s *sePane, rows []seRow) {
	oi := oiPaletteOf(p)
	if len(rows) == 0 {
		if s.err != "" {
			errorState(c, p, s.err)
			return
		}
		if s.searchEverywhereLoading() {
			return
		}
		message := ""
		if s.seEmpty() {
			if s.tab != seTabAll {
				message = "Type to search"
			}
		} else if s.includeNonProject {
			message = "Nothing found"
		} else {
			message = "Nothing found in project files"
		}
		if message != "" {
			ui.Row(c).Height(64).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, message).FontSize(12).TextColor(oi.textFaint).SingleLine()
			})
		}
		return
	}
	height := float32(len(rows) * 24)
	if !s.seEmpty() {
		height = max(float32(seDialogMinListH), height)
	}
	height = min(float32(seDialogMaxListH), height)
	ui.Scroll(c).Height(height + 8).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(0, 0, 8, 0).Children(func() {
			for index, row := range rows {
				index, row := index, row
				if row.header {
					ui.Text(c, seSectionTitles[row.section]).FontSize(11).
						TextColor(oi.textSubtle).Padding(6, 8).SingleLine()
					continue
				}
				if row.more {
					more := ui.Row(c.Key(fmt.Sprintf("se:more:%s", row.section))).
						Height(24).Shrink(0).PaddingStart(26).PaddingEnd(8).
						AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if more.Hovered() {
						more.Background(oi.toolbarHover)
					}
					more.Children(func() {
						ui.Text(c, "… more "+strings.ToLower(seSectionTitles[row.section])).
							FontSize(12).TextColor(oi.textSubtle).SingleLine()
					})
					more.OnClick(func() {
						m.selectSERow(index, recentSelectionSingle)
						m.ActivateSearchEverywhere()
					})
					continue
				}
				active := index == s.sel
				selected := s.isSERowSelected(index, row)
				repeat := index > 0 && row.item.kind == "text" && rows[index-1].item.kind == "text" &&
					row.item.path == rows[index-1].item.path
				itemRow := ui.Row(c.Key(fmt.Sprintf("se:%d", index))).
					Height(24).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if active {
					itemRow.Background(oi.selected)
					if s.revealSelection {
						itemRow.ScrollIntoView()
						s.revealSelection = false
					}
				} else if selected {
					itemRow.Background(oi.selectedInactive)
				} else if itemRow.Hovered() {
					itemRow.Background(oi.toolbarHover)
				}
				itemRow.Children(func() {
					m.seRowIcon(c, p, row.item)
					m.searchEverywhereResultContent(c, p, row, active, repeat)
				})
				mods := itemRow.ClickModifiers()
				itemRow.OnClick(func() {
					mode := recentSelectionSingle
					switch {
					case mods&ui.Shift != 0:
						mode = recentSelectionRange
					case mods&sePrimaryModifier() != 0:
						mode = recentSelectionToggle
					}
					m.selectSERow(index, mode)
					if mode == recentSelectionSingle {
						m.ActivateSearchEverywhere()
					}
				})
				itemRow.ContextMenu(func(menu *ui.Menu) {
					m.selectSERow(index, recentSelectionSingle)
					m.searchEverywhereContextMenu(menu, row)
				})
			}
		})
	})
}

func (m *Model) searchEverywhereContextMenu(menu *ui.Menu, row seRow) {
	item := row.item
	if item.kind == "setting" {
		if menu.Item("Open Settings Page").Chosen() {
			m.CloseSearchEverywhere()
			m.OpenSettingsPage(item.setting)
		}
		return
	}
	if item.kind == "action" {
		if item.disabled {
			menu.Item("Run").Disabled(true)
			return
		}
		if menu.Item("Run").Chosen() {
			m.activateSearchEverywhereNow(false)
		}
		return
	}
	if menu.Item("Open").Chosen() {
		m.activateSearchEverywhereNow(false)
	}
	if searchEverywhereCanSplit(row) {
		if menu.Item("Open to the Side").Chosen() {
			m.activateSearchEverywhereNow(true)
		}
	}
	menu.Separator()
	if menu.Item("Copy Path").Chosen() {
		mygo.Clipboard.WriteText(item.path)
	}
	if menu.Item("Copy Path from Project Root").Chosen() {
		text := item.rel
		if text == "" {
			text = item.path
		}
		mygo.Clipboard.WriteText(text)
	}
	if item.kind == "symbol" {
		if menu.Item("Copy Name").Chosen() {
			mygo.Clipboard.WriteText(strings.TrimSuffix(item.title, "()"))
		}
	}
	if item.kind == "file" || item.kind == "text" {
		if menu.Item("Find in Directory…").Chosen() {
			m.CloseSearchEverywhere()
			m.RunActionWithArg("find.inPath", map[string]any{
				"directory": filepath.Dir(item.path),
			})
		}
	}
}

func (m *Model) searchEverywhereResultContent(
	c *ui.Context, p *ui.Theme, row seRow, active, repeat bool,
) {
	oi := oiPaletteOf(p)
	it := row.item
	switch it.kind {
	case "file":
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Baseline).Gap(6).Children(func() {
			m.seHighlightedText(c, it.title, it.indices, 12, oi.textStrong)
			if it.lineSet {
				ui.Text(c, fmt.Sprintf(":%d", it.line+1)).FontSize(12).
					TextColor(oi.textMuted).SingleLine()
			}
			if it.container != "" {
				ui.Text(c, it.container).MinWidth(0).FontSize(12).
					TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
			}
		})
	case "symbol", "endpoint":
		containerTone := oi.textSubtle
		if active {
			containerTone = oi.textSecondary
		}
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Baseline).Gap(6).Children(func() {
			m.seHighlightedText(c, it.title, it.indices, 12, oi.textStrong)
			if it.container != "" {
				ui.Text(c, it.container).MinWidth(0).FontSize(12).
					TextColor(containerTone).SingleLine().Ellipsis("…")
			}
		})
		right := it.right
		if row.section == seClasses && it.rel != "" {
			right = filepath.Dir(it.rel)
			if right == "." {
				right = filepath.Base(it.rel)
			}
		}
		if right != "" {
			ui.Text(c, right).MaxWidth(300).FontSize(12).
				TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
	case "text":
		m.seCodeLine(c, it, active, oi)
		tone := oi.textSubtle
		if active {
			tone = oi.textMuted
		} else if repeat {
			tone = oi.textFaint
		}
		ui.Text(c, fmt.Sprintf("%s %d", it.right, it.line+1)).MaxWidth(300).
			FontSize(12).TextColor(tone).SingleLine().Ellipsis("…")
	case "action":
		action := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(6)
		action.Children(func() {
			tone := oi.textStrong
			if it.disabled {
				tone = oi.textFaint
			}
			m.seHighlightedText(c, it.title, it.indices, 12, tone)
			if shortcut := openIDEAShortcutForAction(it.action); shortcut != "" {
				ui.Text(c, shortcut).FontSize(11).TextColor(oi.textSubtle).SingleLine()
			}
			ui.Spacer(c)
			if item, ok := actionGet(it.action); ok && item.Toggled != nil {
				on := item.Toggled(m)
				if ui.Toggle(c, &on, "").Disabled(it.disabled).Changed() {
					m.rememberRecentAction(it.action)
					m.RunCommand(it.action)
				}
			} else if it.detail != "" {
				ui.Text(c, it.detail).MaxWidth(300).FontSize(12).
					TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
			}
		})
	case "setting":
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(6).Children(func() {
			m.seHighlightedText(c, it.title, it.indices, 12, oi.textStrong)
			ui.Spacer(c)
			ui.Text(c, "Settings: "+strings.Join(it.settingPath, " | ")).MaxWidth(320).FontSize(12).
				TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		})
	case "command":
		m.seHighlightedText(c, it.title, it.indices, 12, oi.textStrong)
		ui.Spacer(c)
		ui.Text(c, it.detail).FontSize(12).TextColor(oi.textSubtle).SingleLine()
	}
}

func openIDEAShortcutForAction(id string) string {
	if _, ok := actionGet(id); !ok {
		return ""
	}
	return keymapShortcutLabel(id)
}

func sePrimaryModifier() ui.Modifiers {
	if keymapHostOS == "darwin" {
		return ui.Cmd
	}
	return ui.Ctrl
}

func (m *Model) seHighlightedText(
	c *ui.Context, text string, indices []int, size float32, color ui.Color,
) {
	if len(indices) == 0 {
		ui.Text(c, text).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
		return
	}
	runes := []rune(text)
	spans := make([]ui.Span, 0, len(indices)*2+1)
	highlight := map[int]bool{}
	for _, index := range indices {
		if index >= 0 && index < len(runes) {
			highlight[index] = true
		}
	}
	for index := 0; index < len(runes); {
		on := highlight[index]
		end := index + 1
		for end < len(runes) && highlight[end] == on {
			end++
		}
		span := ui.Span{Text: string(runes[index:end]), Size: size, Color: color}
		if on {
			span.Color = oiPaletteOf(c.Theme()).accent
			span.Weight = 600
		}
		spans = append(spans, span)
		index = end
	}
	ui.RichText(c, spans...).FontSize(size).SingleLine().Ellipsis("…")
}

func (m *Model) seCodeLine(c *ui.Context, item seItem, active bool, oi oiColors) {
	indices := item.indices
	ui.RichText(c, seCodeSpans(item.title, indices, active, oi)...).
		Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
}

func seCodeSpans(text string, indices []int, active bool, oi oiColors) []ui.Span {
	runes := []rune(text)
	highlight := map[int]bool{}
	for _, index := range indices {
		if index >= 0 && index < len(runes) {
			highlight[index] = true
		}
	}
	spans := make([]ui.Span, 0, len(indices)*2+1)
	base := oi.text
	if active {
		base = oi.textStrong
	}
	for index := 0; index < len(runes); {
		on := highlight[index]
		end := index + 1
		for end < len(runes) && highlight[end] == on {
			end++
		}
		span := ui.Span{Text: string(runes[index:end]), Size: 12, Color: base}
		if on {
			span.Background = oi.selectedInactive
			span.Weight = 600
			span.Color = oi.textStrong
		}
		spans = append(spans, span)
		index = end
	}
	return spans
}

func (m *Model) searchEverywhereFooter(c *ui.Context, p *ui.Theme, s *sePane, rows []seRow) {
	oi := oiPaletteOf(p)
	active, ok := selectedSearchEverywhereRow(rows, s)
	ui.Row(c).Height(30).Shrink(0).Padding(0, 12).Gap(14).
		Background(oi.frame).BorderWidth(1, 0, 0, 0).BorderColor(oi.popupBorder).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, searchEverywhereDescribe(active, ok)).Grow(1).MinWidth(0).
			FontSize(11).TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		if ok && searchEverywhereCanSplit(active) {
			link := ui.ButtonBase(c.Key("se:open-in-right-split")).Padding(0).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if link.Hovered() {
				link.Background(oi.toolbarHover)
			}
			link.Children(func() {
				ui.Text(c, "Open In Right Split").FontSize(12).TextColor(oi.link).SingleLine()
			})
			link.OnClick(func() { m.ActivateSearchEverywhereInOppositeGroup() })
		}
	})
}

func selectedSearchEverywhereRow(rows []seRow, s *sePane) (seRow, bool) {
	if s == nil || s.sel < 0 || s.sel >= len(rows) || !seSelectable(rows[s.sel]) || rows[s.sel].more {
		return seRow{}, false
	}
	return rows[s.sel], true
}

func searchEverywhereDescribe(row seRow, ok bool) string {
	if !ok {
		return ""
	}
	item := row.item
	switch item.kind {
	case "file":
		return item.rel
	case "symbol", "endpoint", "text":
		if item.rel != "" {
			return fmt.Sprintf("%s:%d", item.rel, item.line+1)
		}
		return fmt.Sprintf("%s:%d", item.path, item.line+1)
	case "setting":
		return strings.Join(append(append([]string{}, item.settingPath...), item.title), " | ")
	case "action":
		description := ""
		if action, ok := actionGet(item.action); ok {
			description = action.Description
		}
		if description == "" && item.detail != "" {
			description = item.detail + " | " + item.title
		}
		if description == "" {
			description = item.title
		}
		return description + "  ·  ⌥↵ to assign a shortcut"
	}
	return ""
}

func searchEverywhereCanSplit(item seRow) bool {
	if item.header || item.more {
		return false
	}
	switch item.item.kind {
	case "file":
		return !item.item.folder
	case "symbol", "endpoint", "text":
		return true
	}
	return false
}

func (m *Model) searchEverywherePreviewPane(c *ui.Context, p *ui.Theme, s *sePane) {
	oi := oiPaletteOf(p)
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(28).Shrink(0).Padding(0, 10).Gap(6).
			Background(oi.frame).AlignItems(ui.Center).Children(func() {
			if s.previewPath != "" {
				fileIconEl(c, oi, s.previewPath, 15, oi.textMuted)
				ui.Text(c, filepath.Base(s.previewPath)).FontSize(11).
					TextColor(oi.textSecondary).SingleLine()
				if s.previewLine >= 0 {
					ui.Text(c, fmt.Sprintf("%d", s.previewLine+1)).FontSize(11).
						TextColor(oi.textFaint).SingleLine()
				}
			}
		})
		switch {
		case s.previewErr != "":
			ui.Row(c).Padding(8, 10).Children(func() {
				ui.Text(c, s.previewErr).FontSize(11).TextColor(oi.warning).
					SingleLine().Ellipsis("…")
			})
		case len(s.previewLines) > 0:
			ui.ScrollBoth(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(5, 0).Children(func() {
					for index, line := range s.previewLines {
						lineNumber := s.previewFrom + index
						row := ui.Row(c.Key(fmt.Sprintf("se:preview:%d", lineNumber))).
							Height(20).Shrink(0).Padding(0, 6).Gap(9).
							AlignItems(ui.Center)
						if lineNumber == s.previewLine {
							row.Background(oi.hoverTint)
						}
						row.Children(func() {
							ui.Text(c, fmt.Sprintf("%d", lineNumber+1)).Width(42).
								FontSize(10).Font(oiFontMono).TextColor(oi.textFaint).SingleLine()
							if lineNumber == s.previewLine && s.previewCol < s.previewEndCol {
								var tokens []highlight.Token
								if index < len(s.previewTokens) {
									tokens = s.previewTokens[index]
								}
								ui.RichText(c, sePreviewLineSpans(
									strings.ReplaceAll(line, "\t", "    "),
									tokens, s.previewCol, s.previewEndCol, p, oi,
								)...).FontSize(11).Font(oiFontMono).SingleLine()
							} else {
								var tokens []highlight.Token
								if index < len(s.previewTokens) {
									tokens = s.previewTokens[index]
								}
								ui.RichText(c, recentSnippetSpans(
									strings.ReplaceAll(line, "\t", "    "),
									tokens, editor.DefaultTheme(p), oi.textMuted,
								)...).FontSize(11).Font(oiFontMono).SingleLine()
							}
						})
					}
				})
			})
		default:
			ui.Row(c).Padding(8, 10).Children(func() {
				ui.Text(c, "Choose a result to preview its source").
					FontSize(11).TextColor(oi.textFaint).SingleLine()
			})
		}
	})
}

func sePreviewLineSpans(
	text string, tokens []highlight.Token, from, to int, p *ui.Theme, oi oiColors,
) []ui.Span {
	spans := recentSnippetSpans(text, tokens, editor.DefaultTheme(p), oi.textMuted)
	cursor := 0
	for index := range spans {
		length := len([]rune(spans[index].Text))
		start, end := cursor, cursor+length
		if start < to && end > from {
			spans[index].Background = oi.selectedInactive
			spans[index].Color = oi.textStrong
		}
		cursor = end
	}
	return spans
}

func (m *Model) openSearchEverywhereInFindToolWindow() {
	s := m.se
	if s == nil {
		return
	}
	query := strings.TrimSpace(parseSELocationQuery(strings.TrimSpace(s.query), m.Root()).text)
	m.CloseSearchEverywhere()
	m.RunActionWithArg("find.inPath", map[string]any{"query": query})
}

func (m *Model) openSearchEverywhereIntentions() {
	item, ok := m.selectedSearchEverywhereItem()
	if !ok {
		return
	}
	switch item.kind {
	case "action":
		if item.disabled {
			return
		}
		m.CloseSearchEverywhere()
		m.OpenSettingsPage("keymap")
		m.settingsRecordKeymap(item.action)
	case "setting":
		m.CloseSearchEverywhere()
		m.OpenSettingsPage(item.setting)
	}
}

// seRowIcon is the row's leading 16px drawing: the file type for files and
// text matches, the symbol's node icon for declarations, and an empty slot
// where upstream's action rows have no icon of their own.
func (m *Model) seRowIcon(c *ui.Context, p *ui.Theme, it seItem) {
	oi := oiPaletteOf(p)
	switch it.kind {
	case "file":
		if it.folder {
			folderIconEl(c, oi, "directory", 16, oi.textSubtle)
		} else {
			fileIconEl(c, oi, it.path, 16, oi.textSubtle)
		}
	case "text":
		oiIconEl(c, "openidea/nodes/textMatch", 16, oi, oi.textSubtle)
	case "action":
		if it.icon != nil {
			ui.Icon(c, it.icon).Size(16, 16).TextColor(oi.textSubtle)
			return
		}
		ui.Row(c).Size(16, 16).Shrink(0)
	case "setting":
		oiIconEl(c, "general/settings", 16, oi, oi.textSubtle)
	case "command":
		oiIconEl(c, "general/search", 16, oi, oi.textSubtle)
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
				if r.section != "" && !seen[r.section] {
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
