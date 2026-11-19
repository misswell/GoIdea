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
	paths    []string
	edited   map[string]bool
	places   []recentPlace
	observed recentObservation
	popup    *recentNavPopup
}

type recentNavPopup struct {
	locations   bool
	onlyChanged bool
	query       string
	files       []string
	tools       []*toolWindowSpec
	places      []recentPlace
	onTools     bool
	index       int
	loading     bool
	err         string
	now         time.Time
	scroll      ui.ScrollState
}

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
		for _, path := range n.paths {
			if (!p.onlyChanged || n.edited[path]) && recentMatches(q, projectRelative(m.Root(), path)) {
				p.files = append(p.files, path)
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
		p.onTools = !p.locations && len(p.files) == 0 && len(p.tools) > 0
		if t := m.tab(); !p.locations && q == "" && len(p.files) > 1 && t != nil && t.doc.Path() == p.files[0] {
			p.index = 1
		}
		p.scroll.Y = 0
	}
	p.index = clampSymbolSel(p.index, p.count())
	m.invalidate()
}

func (p *recentNavPopup) count() int {
	if p.locations {
		return len(p.places)
	}
	if p.onTools {
		return len(p.tools)
	}
	return len(p.files)
}

func (m *Model) closeRecentNavigation() {
	m.recentNav.popup = nil
	m.invalidate()
}

func (m *Model) chooseRecentNavigation() {
	p := m.recentNav.popup
	if p.index < 0 || p.index >= p.count() {
		return
	}
	m.closeRecentNavigation()
	switch {
	case p.locations:
		place := p.places[p.index]
		m.revealAt(place.path, place.pos.Line, place.pos.Col)
	case p.onTools:
		m.DockShow(p.tools[p.index].id)
	default:
		m.openFile(p.files[p.index])
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
