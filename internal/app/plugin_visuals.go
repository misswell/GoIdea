package app

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const (
	pluginVisualBatchLimit = 64
	pluginVisualTimeout    = 15 * time.Second
	pluginMarkerDelay      = 400 * time.Millisecond
)

type pluginVisualState struct {
	root string

	decorationGen     uint64
	decorationLoading bool
	decorations       map[string][]openidea.PluginDecorationSet

	iconGen     uint64
	iconLoading bool
	icons       map[string]pluginMarkerIcon

	markerGen        uint64
	markerAfter      time.Time
	lineLoading      map[uint32]bool
	lineSeen         map[uint32]uint64
	lineResults      map[uint32]pluginLineMarkerResult
	indexedLoading   bool
	indexedSeen      map[string]uint64
	indexedResults   map[string]pluginIndexedMarkerResult
	activeSent       string
	activeGen        uint64
	activeLoading    bool
	markerChooser    pluginMarkerChooser
	visualsAvailable bool
}

type pluginMarkerIcon struct {
	PluginID string
	ID       string
	Icon     string
	Tooltip  string
	Revision string
}

type pluginLineMarkerResult struct {
	LocalVersion   uint64
	BackendVersion uint32
	Markers        []openidea.PluginLineMarker
}

type pluginIndexedMarkerResult struct {
	LocalVersion   uint64
	BackendVersion uint32
	Markers        []openidea.PluginIndexedMarker
}

type pluginMarkerGroup struct {
	Line       int
	Markers    []string
	Targets    []openidea.Location
	Icon       string
	IconDecl   *pluginMarkerIcon
	targetKeys map[string]bool
}

type pluginMarkerChoice struct {
	Target openidea.Location
	Label  string
	Detail string
}

type pluginMarkerChooser struct {
	Open  bool
	Title string
	Items []pluginMarkerChoice
	Sel   int
}

func (m *Model) pluginVisualProjectChanged() {
	pg := m.plugins
	if pg == nil {
		return
	}
	root := m.Root()
	if pg.visual.root == root && pg.visual.visualsAvailable {
		return
	}
	pg.visual = resetPluginVisualState(pg.visual, root)
	if root == "" {
		return
	}
	m.loadPluginDecorations()
	m.loadPluginMarkerIcons()
	m.invalidatePluginMarkers(time.Now())
}

func resetPluginVisualState(previous pluginVisualState, root string) pluginVisualState {
	return pluginVisualState{
		root:             root,
		decorationGen:    previous.decorationGen,
		iconGen:          previous.iconGen,
		markerGen:        previous.markerGen,
		activeGen:        previous.activeGen,
		lineLoading:      map[uint32]bool{},
		lineSeen:         map[uint32]uint64{},
		lineResults:      map[uint32]pluginLineMarkerResult{},
		indexedSeen:      map[string]uint64{},
		indexedResults:   map[string]pluginIndexedMarkerResult{},
		decorations:      map[string][]openidea.PluginDecorationSet{},
		icons:            map[string]pluginMarkerIcon{},
		visualsAvailable: root != "",
	}
}

func (m *Model) pluginDecorationSets(ctx context.Context) (map[string][]openidea.PluginDecorationSet, error) {
	if m.invokeHook != nil {
		raw, err := m.invoke(ctx, "plugin_decoration_sets", nil)
		if err != nil {
			return nil, err
		}
		var sets map[string][]openidea.PluginDecorationSet
		return sets, json.Unmarshal(raw, &sets)
	}
	if m.mgr == nil {
		return nil, fmt.Errorf("GoIDEA backend is unavailable")
	}
	return m.mgr.PluginDecorationSets(ctx, Window)
}

func (m *Model) pluginListSnapshot(ctx context.Context) (openidea.PluginsSnapshot, error) {
	if m.invokeHook != nil {
		raw, err := m.invoke(ctx, "plugin_list", nil)
		if err != nil {
			return openidea.PluginsSnapshot{}, err
		}
		var snapshot openidea.PluginsSnapshot
		return snapshot, json.Unmarshal(raw, &snapshot)
	}
	if m.mgr == nil {
		return openidea.PluginsSnapshot{}, fmt.Errorf("GoIDEA backend is unavailable")
	}
	return m.mgr.PluginsList(ctx, Window)
}

func (m *Model) pluginLineMarkers(ctx context.Context, doc uint32) ([]openidea.PluginLineMarker, error) {
	if m.invokeHook != nil {
		raw, err := m.invoke(ctx, "plugin_line_markers", map[string]any{"doc": doc})
		if err != nil {
			return nil, err
		}
		var markers []openidea.PluginLineMarker
		return markers, json.Unmarshal(raw, &markers)
	}
	if m.mgr == nil {
		return nil, fmt.Errorf("GoIDEA backend is unavailable")
	}
	return m.mgr.PluginLineMarkers(ctx, Window, doc)
}

func (m *Model) pluginNavigationMarkers(
	ctx context.Context,
	paths []string,
) ([]openidea.PluginFileMarkers, error) {
	if m.invokeHook != nil {
		raw, err := m.invoke(ctx, "plugin_navigation_markers", map[string]any{"paths": paths})
		if err != nil {
			return nil, err
		}
		var entries []openidea.PluginFileMarkers
		return entries, json.Unmarshal(raw, &entries)
	}
	if m.mgr == nil {
		return nil, fmt.Errorf("GoIDEA backend is unavailable")
	}
	return m.mgr.PluginNavigationMarkers(ctx, Window, paths)
}

func (m *Model) pluginSetActiveFile(ctx context.Context, path string) error {
	if m.invokeHook != nil {
		var activePath any
		if path != "" {
			activePath = path
		}
		_, err := m.invoke(ctx, "plugin_set_active_file", map[string]any{"path": activePath})
		return err
	}
	if m.mgr == nil {
		return fmt.Errorf("GoIDEA backend is unavailable")
	}
	return m.mgr.PluginSetActiveFile(ctx, Window, path)
}

func (m *Model) loadPluginDecorations() {
	pg := m.plugins
	if pg == nil || pg.visual.root == "" || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	pg.visual.decorationGen++
	gen, root := pg.visual.decorationGen, pg.visual.root
	pg.visual.decorationLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), pluginVisualTimeout)
		defer cancel()
		sets, err := m.pluginDecorationSets(ctx)
		m.post(func() {
			s := &pg.visual
			if s.root != root || s.decorationGen != gen {
				return
			}
			s.decorationLoading = false
			if err != nil {
				return
			}
			if sets == nil {
				sets = map[string][]openidea.PluginDecorationSet{}
			}
			s.decorations = sets
			m.refreshPluginVisualDecorations()
		})
	}()
}

func (m *Model) loadPluginMarkerIcons() {
	pg := m.plugins
	if pg == nil || pg.visual.root == "" || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	pg.visual.iconGen++
	gen, root := pg.visual.iconGen, pg.visual.root
	pg.visual.iconLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), pluginVisualTimeout)
		defer cancel()
		snapshot, err := m.pluginListSnapshot(ctx)
		icons := pluginMarkerIcons(snapshot)
		m.post(func() {
			s := &pg.visual
			if s.root != root || s.iconGen != gen {
				return
			}
			s.iconLoading = false
			if err != nil {
				return
			}
			s.icons = icons
			m.refreshPluginVisualDecorations()
		})
	}()
}

func pluginMarkerIcons(snapshot openidea.PluginsSnapshot) map[string]pluginMarkerIcon {
	out := map[string]pluginMarkerIcon{}
	for _, contribution := range snapshot.UI {
		if contribution.Point != "markerIcons" {
			continue
		}
		var decl openidea.PluginMarkerIconDecl
		if json.Unmarshal(contribution.Decl, &decl) != nil || decl.ID == "" {
			continue
		}
		tooltip := ""
		if decl.Tooltip != nil {
			tooltip = strings.TrimSpace(*decl.Tooltip)
		}
		out[pluginMarkerIconKey(contribution.PluginID, decl.ID)] = pluginMarkerIcon{
			PluginID: contribution.PluginID,
			ID:       decl.ID,
			Icon:     decl.Icon,
			Tooltip:  tooltip,
			Revision: decl.Revision,
		}
	}
	return out
}

func pluginMarkerIconKey(pluginID, id string) string {
	return pluginID + "\x00" + id
}

func (m *Model) syncPluginVisuals(now time.Time) {
	pg := m.plugins
	if pg == nil || pg.visual.root == "" || pg.visual.root != m.Root() {
		return
	}
	m.syncPluginActiveFile()
	if now.Before(pg.visual.markerAfter) {
		return
	}
	for _, tab := range m.pluginVisualTabs() {
		m.startPluginLineMarkers(tab)
	}
	m.startPluginIndexedMarkers()
}

func (m *Model) pluginVisualTabs() []*Tab {
	seen := map[uint32]bool{}
	out := make([]*Tab, 0, pluginVisualBatchLimit)
	add := func(tab *Tab) {
		if tab == nil || tab.kind != "" || tab.doc == nil || tab.state == nil ||
			tab.doc.ID() == 0 || tab.doc.Info().Large || len(out) >= pluginVisualBatchLimit {
			return
		}
		id := tab.doc.ID()
		if seen[id] {
			return
		}
		seen[id] = true
		out = append(out, tab)
	}
	add(m.tab())
	add(m.splitView().tab())
	for _, tab := range m.tabs {
		add(tab)
	}
	if m.split != nil {
		for _, tab := range m.split.tabs {
			add(tab)
		}
	}
	return out
}

func (m *Model) startPluginLineMarkers(tab *Tab) {
	pg := m.plugins
	if pg == nil || tab == nil || tab.doc == nil || tab.state == nil ||
		(m.mgr == nil && m.invokeHook == nil) {
		return
	}
	localVersion := tab.doc.Buffer().Version()
	if localVersion != tab.synced {
		return
	}
	doc := tab.doc.ID()
	backendVersion := tab.doc.Version()
	s := &pg.visual
	if seen, ok := s.lineSeen[doc]; s.lineLoading[doc] || ok && seen == localVersion {
		return
	}
	s.lineSeen[doc] = localVersion
	s.lineLoading[doc] = true
	gen, root := s.markerGen, s.root
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), pluginVisualTimeout)
		defer cancel()
		markers, err := m.pluginLineMarkers(ctx, doc)
		m.post(func() {
			current := &pg.visual
			if current.root != root || current.markerGen != gen {
				return
			}
			delete(current.lineLoading, doc)
			if err != nil {
				return
			}
			current.lineResults[doc] = pluginLineMarkerResult{
				LocalVersion:   localVersion,
				BackendVersion: backendVersion,
				Markers:        markers,
			}
			m.refreshPluginVisualDecorations()
		})
	}()
}

func (m *Model) startPluginIndexedMarkers() {
	pg := m.plugins
	if pg == nil || pg.visual.indexedLoading || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	type requestFile struct {
		Path           string
		LocalVersion   uint64
		BackendVersion uint32
	}
	requested := make([]requestFile, 0, pluginVisualBatchLimit)
	paths := make([]string, 0, pluginVisualBatchLimit)
	for _, tab := range m.pluginVisualTabs() {
		localVersion := tab.doc.Buffer().Version()
		if localVersion != tab.synced {
			continue
		}
		path := tab.doc.Path()
		if seen, ok := pg.visual.indexedSeen[path]; ok && seen == localVersion {
			continue
		}
		pg.visual.indexedSeen[path] = localVersion
		requested = append(requested, requestFile{
			Path: path, LocalVersion: localVersion, BackendVersion: tab.doc.Version(),
		})
		paths = append(paths, path)
		if len(paths) == pluginVisualBatchLimit {
			break
		}
	}
	if len(paths) == 0 {
		return
	}
	pg.visual.indexedLoading = true
	gen, root := pg.visual.markerGen, pg.visual.root
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), pluginVisualTimeout)
		defer cancel()
		entries, err := m.pluginNavigationMarkers(ctx, paths)
		m.post(func() {
			current := &pg.visual
			if current.root != root || current.markerGen != gen {
				return
			}
			current.indexedLoading = false
			if err != nil {
				return
			}
			byPath := make(map[string]openidea.PluginFileMarkers, len(entries))
			for _, entry := range entries {
				byPath[entry.Path] = entry
			}
			for _, request := range requested {
				entry, ok := byPath[request.Path]
				if !ok || entry.Version != nil && *entry.Version != request.BackendVersion {
					continue
				}
				current.indexedResults[request.Path] = pluginIndexedMarkerResult{
					LocalVersion:   request.LocalVersion,
					BackendVersion: request.BackendVersion,
					Markers:        entry.Markers,
				}
			}
			m.refreshPluginVisualDecorations()
		})
	}()
}

func (m *Model) syncPluginActiveFile() {
	pg := m.plugins
	if pg == nil || pg.visual.activeLoading || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	path := ""
	if tab := m.tab(); tab != nil && tab.kind == "" && tab.doc != nil &&
		tab.doc.ID() != 0 && !tab.doc.Info().Large {
		path = tab.doc.Path()
	}
	key := pg.visual.root + "\x00" + path
	if pg.visual.activeSent == key {
		return
	}
	pg.visual.activeSent = key
	pg.visual.activeLoading = true
	gen, root := pg.visual.activeGen, pg.visual.root
	go func() {
		ctx, cancel := context.WithTimeout(m.workspaceContext(), 5*time.Second)
		defer cancel()
		_ = m.pluginSetActiveFile(ctx, path)
		m.post(func() {
			if pg.visual.root != root || pg.visual.activeGen != gen {
				return
			}
			pg.visual.activeLoading = false
		})
	}()
}

func (m *Model) invalidatePluginMarkers(now time.Time) {
	pg := m.plugins
	if pg == nil {
		return
	}
	s := &pg.visual
	s.markerGen++
	s.markerAfter = now.Add(pluginMarkerDelay)
	s.lineLoading = map[uint32]bool{}
	s.lineSeen = map[uint32]uint64{}
	s.lineResults = map[uint32]pluginLineMarkerResult{}
	s.indexedLoading = false
	s.indexedSeen = map[string]uint64{}
	s.indexedResults = map[string]pluginIndexedMarkerResult{}
	m.refreshPluginVisualDecorations()
}

func (m *Model) pluginEditorChanged(tab *Tab, now time.Time) {
	pg := m.plugins
	if pg == nil || tab == nil || tab.doc == nil {
		return
	}
	s := &pg.visual
	id, path := tab.doc.ID(), tab.doc.Path()
	delete(s.lineSeen, id)
	delete(s.lineResults, id)
	delete(s.indexedSeen, path)
	delete(s.indexedResults, path)
	s.markerAfter = now.Add(pluginMarkerDelay)
	m.refreshPluginVisualDecorations()
}

func (m *Model) acceptPluginDecorationsChanged(event openidea.PluginDecorationChange) {
	pg := m.plugins
	if pg == nil || event.PluginID == "" {
		return
	}
	if pg.visual.decorations == nil {
		pg.visual.decorations = map[string][]openidea.PluginDecorationSet{}
	}
	if event.Sets == nil {
		delete(pg.visual.decorations, event.PluginID)
	} else {
		pg.visual.decorations[event.PluginID] = event.Sets
	}
	m.refreshPluginVisualDecorations()
}

func (m *Model) pluginCommandsChanged() {
	pg := m.plugins
	if pg == nil || pg.visual.root == "" {
		return
	}
	m.loadPluginDecorations()
	m.loadPluginMarkerIcons()
	m.invalidatePluginMarkers(time.Now())
}

func (m *Model) pluginVisualBackendRestarted() {
	if m.plugins == nil {
		return
	}
	m.plugins.visual.root = ""
	m.pluginVisualProjectChanged()
}

func (m *Model) refreshPluginVisualDecorations() {
	seen := map[uint32]bool{}
	for _, tab := range m.pluginVisualTabs() {
		if tab == nil || tab.doc == nil || tab.state == nil {
			continue
		}
		id := tab.doc.ID()
		if seen[id] {
			continue
		}
		seen[id] = true
		m.refreshTabDecorations(tab)
		theme := m.theme
		if theme == nil {
			theme = openideaTheme(false)
		}
		tab.state.TextBackgrounds = m.composeTextBackgrounds(tab, m.todoMarksOf(tab, theme))
	}
	m.invalidate()
}

func (m *Model) composeTextBackgrounds(tab *Tab, base []editor.TextBackground) []editor.TextBackground {
	if tab == nil || tab.doc == nil {
		return base
	}
	out := append([]editor.TextBackground(nil), base...)
	_, pluginText := m.pluginDecorationBackgrounds(tab)
	out = append(out, pluginText...)
	out = append(out, m.rainbowBracketBackgrounds(tab)...)
	out = append(out, m.semanticTokenBackgrounds(tab)...)
	out = append(out, m.graphqlUnsentTextBackgrounds(tab)...)
	return append(out, m.usageHighlightBackgrounds(tab)...)
}

func (m *Model) pluginDecorationBackgrounds(tab *Tab) ([]editor.LineBackground, []editor.TextBackground) {
	pg := m.plugins
	if pg == nil || tab == nil || tab.doc == nil {
		return nil, nil
	}
	oi := oiPaletteOf(m.theme)
	var lines []editor.LineBackground
	var text []editor.TextBackground
	for _, sets := range pg.visual.decorations {
		for _, set := range sets {
			if set.Tree || !pluginPathsEqual(set.Root, pg.visual.root) ||
				!pluginPathsEqual(set.Path, tab.doc.Path()) {
				continue
			}
			for _, item := range set.Items {
				color, ok := pluginDecorationColor(item.Style, oi)
				if !ok {
					continue
				}
				start := tab.doc.ByteOffset(item.Range.Start)
				end := tab.doc.ByteOffset(item.Range.End)
				if start > end {
					start, end = end, start
				}
				if start < end {
					text = append(text, editor.TextBackground{Start: start, End: end, Color: color})
					continue
				}
				// MyGo has no editor widget API. Keep an after-only decoration
				// visible as a quiet row tint; its text is shown on hover.
				if item.After != nil || item.Hover != nil {
					line := int(item.Range.Start.Line)
					if line >= 0 && line < tab.doc.Buffer().LineCount() {
						lines = append(lines, editor.LineBackground{
							First: line, Last: line, Color: color.Alpha(0.08),
						})
					}
				}
			}
		}
	}
	return lines, text
}

func pluginDecorationColor(style string, oi oiColors) (ui.Color, bool) {
	var color ui.Color
	switch style {
	case "highlight":
		color = oi.accent
	case "search":
		color = oi.warning
	case "added":
		color = oi.vcsAdded
	case "modified":
		color = oi.vcsModified
	case "removed":
		color = oi.vcsDeleted
	case "warning":
		color = oi.warning
	case "error":
		color = oi.danger
	case "info":
		color = oi.link
	case "underline":
		color = oi.text
	case "strikethrough":
		color = oi.textMuted
	case "muted":
		color = oi.textFaint
	default:
		return ui.Color{}, false
	}
	return color.Alpha(0.22), true
}

func pluginPathsEqual(left, right string) bool {
	return workspacePathEqual(left, right)
}

func (m *Model) pluginDecorationHover(tab *Tab, req editor.HoverRequest) bool {
	pg := m.plugins
	if pg == nil || tab == nil || tab.doc == nil || req.Offset < 0 {
		return false
	}
	for _, sets := range pg.visual.decorations {
		for _, set := range sets {
			if set.Tree || !pluginPathsEqual(set.Root, pg.visual.root) ||
				!pluginPathsEqual(set.Path, tab.doc.Path()) {
				continue
			}
			for _, item := range set.Items {
				if _, ok := pluginDecorationColor(item.Style, oiPaletteOf(m.theme)); !ok {
					continue
				}
				start := tab.doc.ByteOffset(item.Range.Start)
				end := tab.doc.ByteOffset(item.Range.End)
				if start > end {
					start, end = end, start
				}
				if req.Offset < start || start < end && req.Offset >= end || start == end && req.Offset != start {
					continue
				}
				var parts []string
				if item.Hover != nil && strings.TrimSpace(*item.Hover) != "" {
					parts = append(parts, strings.TrimSpace(*item.Hover))
				}
				if item.After != nil && strings.TrimSpace(*item.After) != "" {
					parts = append(parts, strings.TrimSpace(*item.After))
				}
				if len(parts) == 0 {
					continue
				}
				tab.state.ShowHover(start, end, strings.Join(parts, "\n"), nil)
				return true
			}
		}
	}
	return false
}

func (m *Model) pluginMarkerGroups(tab *Tab) map[int]*pluginMarkerGroup {
	out := map[int]*pluginMarkerGroup{}
	pg := m.plugins
	if pg == nil || tab == nil || tab.doc == nil {
		return out
	}
	localVersion := tab.doc.Buffer().Version()
	backendVersion := tab.doc.Version()
	lines := tab.doc.Buffer().LineCount()
	group := func(line int) *pluginMarkerGroup {
		g := out[line]
		if g == nil {
			g = &pluginMarkerGroup{Line: line, targetKeys: map[string]bool{}}
			out[line] = g
		}
		return g
	}
	if result, ok := pg.visual.lineResults[tab.doc.ID()]; ok &&
		result.LocalVersion == localVersion && result.BackendVersion == backendVersion {
		for _, marker := range result.Markers {
			line := int(marker.Line)
			if line < 0 || line >= lines {
				continue
			}
			g := group(line)
			if g.Icon == "" && marker.Icon != nil {
				g.Icon = strings.TrimSpace(*marker.Icon)
			}
			if text := strings.TrimSpace(marker.Tooltip); text != "" {
				g.Markers = appendUniqueString(g.Markers, text)
			}
			if marker.Target != nil {
				g.addTarget(*marker.Target)
			}
		}
	}
	if result, ok := pg.visual.indexedResults[tab.doc.Path()]; ok &&
		result.LocalVersion == localVersion && result.BackendVersion == backendVersion {
		for _, marker := range result.Markers {
			line := int(marker.Range.Start.Line)
			if line < 0 || line >= lines {
				continue
			}
			g := group(line)
			if marker.Icon != nil {
				if decl, found := pg.visual.icons[pluginMarkerIconKey(marker.PluginID, *marker.Icon)]; found {
					g.IconDecl = &decl
					if g.Icon == "" {
						g.Icon = decl.Icon
					}
					if decl.Tooltip != "" {
						g.Markers = appendUniqueString(g.Markers, decl.Tooltip)
					}
				}
			}
			for _, target := range marker.Targets {
				g.addTarget(target)
			}
		}
	}
	for line, g := range out {
		if len(g.Markers) == 0 && len(g.Targets) == 0 {
			delete(out, line)
		}
	}
	return out
}

func (g *pluginMarkerGroup) addTarget(target openidea.Location) {
	key := pluginMarkerTargetKey(target)
	if g.targetKeys == nil {
		g.targetKeys = map[string]bool{}
	}
	if g.targetKeys[key] {
		return
	}
	g.targetKeys[key] = true
	g.Targets = append(g.Targets, target)
}

func pluginMarkerTargetKey(target openidea.Location) string {
	return target.Path + "\x00" + strconv.FormatUint(uint64(target.Range.Start.Line), 10) +
		"\x00" + strconv.FormatUint(uint64(target.Range.Start.Col), 10) +
		"\x00" + strconv.FormatUint(uint64(target.Range.End.Line), 10) +
		"\x00" + strconv.FormatUint(uint64(target.Range.End.Col), 10)
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func (m *Model) pluginGutterMarks(tab *Tab) []editor.GutterMark {
	groups := m.pluginMarkerGroups(tab)
	if len(groups) == 0 {
		return nil
	}
	oi := oiPaletteOf(m.theme)
	out := make([]editor.GutterMark, 0, len(groups))
	for _, group := range groups {
		if gutterGroup := pluginMarkerGutterGroup(group); gutterGroup != "" &&
			!m.gutterGroupEnabled(gutterGroup) {
			continue
		}
		out = append(out, editor.GutterMark{
			Line:  group.Line,
			Color: pluginMarkerColor(group, oi),
		})
	}
	return out
}

// pluginMarkerGutterGroup maps the marker conventions the pinned backend uses
// for language structure to the matching Gutter Icons switch. Plugins that
// publish another icon stay independent of these core groups.
func pluginMarkerGutterGroup(group *pluginMarkerGroup) string {
	if group == nil {
		return ""
	}
	icon := strings.ToLower(strings.TrimSpace(group.Icon))
	if group.IconDecl != nil {
		icon += " " + strings.ToLower(strings.TrimSpace(group.IconDecl.Icon)) +
			" " + strings.ToLower(strings.TrimSpace(group.IconDecl.ID))
	}
	switch {
	case strings.Contains(icon, "implemented"), strings.Contains(icon, "overridden"):
		return "inheritors"
	case strings.Contains(icon, "implementing"), strings.Contains(icon, "overriding"):
		return "supers"
	case strings.Contains(icon, "run"), strings.Contains(icon, "execute"):
		return "run"
	default:
		return ""
	}
}

func pluginMarkerColor(group *pluginMarkerGroup, oi oiColors) ui.Color {
	if group == nil {
		return oi.accent
	}
	icon := group.Icon
	if group.IconDecl != nil {
		look := strings.ToLower(group.IconDecl.Icon + " " + group.IconDecl.ID)
		switch {
		case strings.Contains(look, "run"), strings.Contains(look, "execute"):
			return oi.success
		case strings.Contains(look, "implement"):
			return oi.vcsAdded
		case strings.Contains(look, "override"):
			return oi.vcsModified
		case strings.Contains(look, "warn"), strings.Contains(look, "test"):
			return oi.warning
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(group.IconDecl.PluginID + "\x00" + group.IconDecl.ID))
		colors := []ui.Color{oi.accent, oi.link, oi.success, oi.warning, oi.vcsModified, oi.ignored}
		return colors[int(h.Sum32())%len(colors)]
	}
	switch strings.ToLower(strings.TrimSpace(icon)) {
	case "implemented":
		return oi.vcsAdded
	case "implementing":
		return oi.warning
	case "overridden":
		return oi.vcsModified
	case "overriding":
		return oi.link
	case "run":
		return oi.success
	case "navigate", "":
		return oi.accent
	default:
		return oi.accent
	}
}

func (m *Model) pluginMarkerTooltip(tab *Tab, line int) []string {
	group := m.pluginMarkerGroups(tab)[line]
	if group == nil {
		return nil
	}
	parts := append([]string(nil), group.Markers...)
	for _, target := range group.Targets {
		label := implTargetLabel(m.Root(), implTargetOf(target))
		parts = appendUniqueString(parts, label)
	}
	if len(parts) == 0 {
		return nil
	}
	if len(group.Targets) > 1 {
		return append([]string{fmt.Sprintf("Related targets (%d)", len(group.Targets))}, parts...)
	}
	return parts
}

func joinPluginMarkerTooltip(parts []string) string {
	return strings.Join(parts, "\n")
}

func (m *Model) editorGutterTooltip(c *ui.Context, view ui.Element, tab *Tab) {
	parts := make([]string, 0, 2)
	if title := m.coverageGutterTitle(c, view, tab); title != "" {
		parts = append(parts, title)
	}
	if title := m.pluginGutterTooltip(c, view, tab); title != "" {
		parts = append(parts, title)
	}
	view.Tooltip(strings.Join(parts, "\n"))
}

func (m *Model) pluginGutterTooltip(c *ui.Context, view ui.Element, tab *Tab) string {
	if c == nil || tab == nil || tab.state == nil || m.plugins == nil {
		return ""
	}
	x, y, over := view.PointerPosition()
	if !over || x < 0 || x > coverageGutterWidth(c, tab.state) {
		return ""
	}
	line := coverageLineAtY(tab.state, y)
	if line < 0 {
		return ""
	}
	parts := m.pluginMarkerTooltip(tab, line)
	return joinPluginMarkerTooltip(parts)
}

func (m *Model) pluginMarkerClick(tab *Tab, line int) bool {
	group := m.pluginMarkerGroups(tab)[line]
	if group == nil {
		return false
	}
	switch len(group.Targets) {
	case 0:
		m.status = joinPluginMarkerTooltip(group.Markers)
		m.invalidate()
	case 1:
		m.openPluginMarkerTarget(group.Targets[0])
	default:
		m.openPluginMarkerChooser(group)
	}
	return true
}

func (m *Model) openPluginMarkerChooser(group *pluginMarkerGroup) {
	pg := m.plugins
	if pg == nil || group == nil {
		return
	}
	items := make([]pluginMarkerChoice, 0, len(group.Targets))
	for _, target := range group.Targets {
		items = append(items, pluginMarkerChoice{
			Target: target,
			Label:  implTargetLabel(m.Root(), implTargetOf(target)),
			Detail: joinPluginMarkerTooltip(group.Markers),
		})
	}
	pg.visual.markerChooser = pluginMarkerChooser{
		Open:  true,
		Title: fmt.Sprintf("Plugin Marker Targets (%d)", len(items)),
		Items: items,
	}
	m.invalidate()
}

func (m *Model) choosePluginMarker(index int) {
	pg := m.plugins
	if pg == nil || index < 0 || index >= len(pg.visual.markerChooser.Items) {
		return
	}
	target := pg.visual.markerChooser.Items[index].Target
	pg.visual.markerChooser.Open = false
	m.openPluginMarkerTarget(target)
}

func (m *Model) stepPluginMarkerChooser(delta int) {
	pg := m.plugins
	if pg == nil {
		return
	}
	chooser := &pg.visual.markerChooser
	if n := len(chooser.Items); n > 0 {
		chooser.Sel = (chooser.Sel + delta + n) % n
		m.invalidate()
	}
}

func (m *Model) openPluginMarkerTarget(target openidea.Location) {
	if target.URI != nil && *target.URI != "" || strings.Contains(target.Path, "://") {
		m.status = "The plugin marker points into an external dependency and cannot open as a file"
		m.invalidate()
		return
	}
	m.revealRange(
		target.Path,
		int(target.Range.Start.Line), int(target.Range.Start.Col),
		int(target.Range.End.Line), int(target.Range.End.Col),
	)
}

func (m *Model) pluginMarkerDialog(c *ui.Context, theme *ui.Theme) {
	pg := m.plugins
	if pg == nil || !pg.visual.markerChooser.Open {
		return
	}
	oi := oiPaletteOf(theme)
	chooser := &pg.visual.markerChooser
	open := chooser.Open
	chosen := -1
	nativeModal(c, &open, func() {
		box := ui.Column(c).Width(560).Gap(8)
		box.Children(func() {
			ui.Text(c, chooser.Title).FontSize(13).Bold().TextColor(oi.textStrong).
				SingleLine().Ellipsis("…")
			ui.Text(c, "Choose a plugin marker target").
				FontSize(12).TextColor(oi.textMuted).SingleLine()
			ui.Scroll(c).Height(float32(min(320, max(90, len(chooser.Items)*30)))).Children(func() {
				ui.Column(c).Children(func() {
					for index, item := range chooser.Items {
						index, item := index, item
						row := ui.ButtonBase(c.Key(fmt.Sprintf("plugin-marker-target:%d", index))).
							Height(28).Padding(0, 8).Radius(4)
						if index == chooser.Sel || row.Hovered() {
							row.Background(oi.hover)
						}
						row.Children(func() {
							ui.Text(c, item.Label).FontSize(12).TextColor(oi.text).
								SingleLine().Ellipsis("…")
						})
						if row.Clicked() {
							chooser.Sel = index
							chosen = index
						}
					}
				})
			})
			ui.Text(c, "↑/↓ select · Enter opens · Esc closes").
				FontSize(11).TextColor(oi.textMuted).SingleLine()
		})
		switch {
		case box.OverlayShortcut(0, ui.KeyEscape):
			open = false
		case box.OverlayShortcut(0, ui.KeyUp):
			m.stepPluginMarkerChooser(-1)
		case box.OverlayShortcut(0, ui.KeyDown):
			m.stepPluginMarkerChooser(1)
		case box.OverlayShortcut(0, ui.KeyEnter):
			chosen = chooser.Sel
		}
	})
	chooser.Open = open && chosen < 0
	if chosen >= 0 {
		m.choosePluginMarker(chosen)
	}
}
