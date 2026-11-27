package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// loadSearchEverywherePlugins loads the live plugin catalogue once per
// overlay. Endpoint search then asks each code plugin for its project routes.
func (m *Model) loadSearchEverywherePlugins() {
	s := m.se
	if s == nil || s.pluginsBusy || s.pluginsDone {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.pluginsDone = true
		s.endpointDone = true
		return
	}
	s.pluginsBusy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		snapshot, err := m.mgr.PluginsList(ctx, Window)
		m.post(func() {
			if m.se != s {
				return
			}
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" {
					s.pluginsError = "Loading endpoints failed: " + err.Error()
					s.pluginsBusy = false
					s.pluginsDone = true
				} else {
					s.pluginsBusy = false
					s.pluginsDone = false
				}
				s.plugins = nil
			} else {
				s.pluginsBusy = false
				s.pluginsDone = true
				s.pluginsError = ""
				s.plugins = seLiveEndpointPlugins(snapshot.Plugins)
			}
			if err != nil && openidea.ErrorKind(err) == "cancelled" {
				m.invalidate()
				return
			}
			if s.tab == seTabAll && !s.seEmpty() {
				m.seQueryEndpoints(s.seq, strings.TrimSpace(s.query))
			} else {
				s.endpointDone = true
			}
			m.invalidate()
		})
	}()
}

func seLiveEndpointPlugins(plugins []openidea.PluginInfo) []openidea.PluginInfo {
	out := make([]openidea.PluginInfo, 0, len(plugins))
	for _, plugin := range plugins {
		if !plugin.Enabled {
			continue
		}
		switch plugin.Status {
		case "active", "inactive", "static":
			out = append(out, plugin)
		}
	}
	return out
}

// seQueryEndpoints asks every live code plugin for project HTTP routes. The
// server caches each list until a matching project file changes.
func (m *Model) seQueryEndpoints(seq int, query string) {
	s := m.se
	if s == nil {
		return
	}
	if strings.TrimSpace(query) == "" {
		s.endpointDone = true
		m.searchEverywhereSectionDone()
		return
	}
	if s.pluginsBusy || !s.pluginsDone {
		return
	}
	if m.mgr == nil || m.ctx == nil {
		s.endpointDone = true
		m.searchEverywhereSectionDone()
		return
	}
	s.endpointSeq++
	generation := s.endpointSeq
	plugins := append([]openidea.PluginInfo(nil), s.plugins...)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		rawSources, sourceErr := m.mgr.Invoke(ctx, Window, "spring_route_sources", nil)
		var sources []openidea.RouteSource
		if sourceErr == nil {
			sourceErr = json.Unmarshal(rawSources, &sources)
		}
		var firstError error
		if sourceErr != nil {
			firstError = sourceErr
		}
		endpoints, endpointErr := m.pluginEndpointItems(sources, plugins, ctx)
		if firstError == nil && endpointErr != nil {
			firstError = endpointErr
		}
		sort.SliceStable(endpoints, func(i, j int) bool {
			a, b := endpoints[i], endpoints[j]
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			if a.Location.Path != b.Location.Path {
				return a.Location.Path < b.Location.Path
			}
			return a.Location.Range.Start.Line < b.Location.Range.Start.Line
		})
		m.post(func() {
			if m.se != s || s.seq != seq || s.endpointSeq != generation {
				return
			}
			s.endpoints = normalizeSearchEndpointPaths(endpoints)
			s.endpointDone = true
			if firstError != nil && len(endpoints) == 0 {
				s.err = "Finding endpoints failed: " + firstError.Error()
			}
			m.searchEverywhereSectionDone()
		})
	}()
}

// pluginEndpointItems combines the framework provider (Spring scanner) with
// plugin endpoint providers before Search Everywhere ranks the routes.
func (m *Model) pluginEndpointItems(
	sources []openidea.RouteSource,
	plugins []openidea.PluginInfo,
	ctx context.Context,
) ([]openidea.PluginEndpoint, error) {
	routes := httpRoutesFromSources(sources)
	endpoints := make([]openidea.PluginEndpoint, 0, len(routes))
	for _, route := range routes {
		method := route.method
		handler := route.controller + "." + route.methodName
		path := route.url
		endpoints = append(endpoints, openidea.PluginEndpoint{
			Method: &method, Path: path, Handler: &handler,
			Location: openidea.Location{
				Path: route.path,
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: uint32(route.line), Col: uint32(route.col)},
				},
			},
		})
	}
	for _, plugin := range plugins {
		if ctx.Err() != nil {
			return endpoints, ctx.Err()
		}
		items, err := m.mgr.PluginEndpoints(ctx, Window, plugin.ID)
		if err != nil {
			if openidea.ErrorKind(err) == "cancelled" {
				return endpoints, err
			}
			continue
		}
		endpoints = append(endpoints, items...)
	}
	return endpoints, nil
}

func normalizeSearchEndpointPaths(endpoints []openidea.PluginEndpoint) []openidea.PluginEndpoint {
	for index := range endpoints {
		endpoints[index].Location.Path = realPath(endpoints[index].Location.Path)
	}
	return endpoints
}

// acceptPluginEvent refreshes the native contributions whose backend state
// changed.
func (m *Model) acceptPluginEvent(data json.RawMessage) {
	if m.acceptPluginContentEvent(data) {
		return
	}
	var event struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	switch event.Type {
	case "decorationsChanged":
		var change openidea.PluginDecorationChange
		if json.Unmarshal(data, &change) == nil {
			m.acceptPluginDecorationsChanged(change)
		}
	case "documentIndexesChanged":
		m.invalidatePluginMarkers(time.Now())
	case "commandsChanged":
		m.pluginCommandsChanged()
	case "endpointsChanged":
		m.acceptPluginEndpointsChanged()
	}
}

func (m *Model) acceptPluginEndpointsChanged() {
	m.invalidatePluginEndpointIndex()
	m.invalidateSpringRouteIndex()
	m.ensurePluginEndpointIndex()
	if m.se == nil {
		return
	}
	s := m.se
	if s.tab != seTabAll || s.seEmpty() || !s.pluginsDone {
		return
	}
	s.endpointDone = false
	m.seQueryEndpoints(s.seq, strings.TrimSpace(s.query))
	m.invalidate()
}

var seHTTPMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true,
	"HEAD": true, "OPTIONS": true, "TRACE": true,
}

func seEndpointMethod(endpoint openidea.PluginEndpoint) string {
	if endpoint.Method == nil || strings.TrimSpace(*endpoint.Method) == "" {
		return "*"
	}
	return strings.ToUpper(strings.TrimSpace(*endpoint.Method))
}

func seEndpointPath(path string) string {
	return strings.Trim(path, "/")
}

func seEndpointHandler(endpoint openidea.PluginEndpoint) string {
	if endpoint.Handler != nil {
		if handler := strings.TrimSpace(*endpoint.Handler); handler != "" {
			return handler
		}
	}
	return strings.TrimSpace(endpoint.Framework)
}

func seEndpointController(endpoint openidea.PluginEndpoint) string {
	if endpoint.Handler == nil {
		return ""
	}
	handler := strings.TrimSpace(*endpoint.Handler)
	if at := strings.LastIndexAny(handler, "#."); at >= 0 {
		return handler[:at]
	}
	return ""
}

func seEndpointContainer(endpoint openidea.PluginEndpoint) string {
	if endpoint.Handler != nil && strings.TrimSpace(*endpoint.Handler) != "" {
		return strings.TrimSpace(*endpoint.Handler)
	}
	return strings.TrimSpace(endpoint.Framework)
}

func seEndpointMethodName(endpoint openidea.PluginEndpoint) string {
	if endpoint.Handler == nil {
		return ""
	}
	handler := strings.TrimSpace(*endpoint.Handler)
	if at := strings.LastIndexAny(handler, "#."); at >= 0 {
		return handler[at+1:]
	}
	return handler
}

type seEndpointHit struct {
	item  seItem
	score int
	rel   string
	line  int
}

// seEndpointItems mirrors OpenIDEA's matchRoutes: an optional HTTP method,
// exact and suffix route matching, handler names and a fuzzy path fallback.
func seEndpointItems(endpoints []openidea.PluginEndpoint, query string, limit int) []seItem {
	text := strings.TrimSpace(query)
	if text == "" || len(endpoints) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = seTabLimit
	}
	words := strings.Fields(text)
	method := ""
	if len(words) > 0 && seHTTPMethods[strings.ToUpper(words[0])] {
		method = strings.ToUpper(words[0])
	}
	rest := text
	if method != "" {
		rest = strings.TrimSpace(strings.TrimPrefix(text, words[0]))
	}
	wantPath := strings.ToLower(seEndpointPath(rest))
	wantName := strings.ToLower(rest)
	hits := make([]seEndpointHit, 0, min(len(endpoints), limit))
	for _, endpoint := range endpoints {
		httpMethod := seEndpointMethod(endpoint)
		if method != "" && httpMethod != "*" && httpMethod != method {
			continue
		}
		target := strings.ToLower(seEndpointPath(endpoint.Path))
		methodName := strings.ToLower(seEndpointMethodName(endpoint))
		controller := strings.ToLower(seEndpointController(endpoint))
		qualified := strings.ToLower(seEndpointHandler(endpoint))
		score := 0
		var indices []int
		switch {
		case wantPath != "" && target == wantPath:
			score = 1000
		case wantPath != "" && strings.Contains(wantPath, "/") &&
			(strings.HasSuffix(target, "/"+wantPath) || strings.HasSuffix(wantPath, "/"+target)):
			score = 900
		case len(wantPath) >= 3 && len(target) >= 3 &&
			(strings.Contains(target, wantPath) || strings.Contains(wantPath, target)):
			score = 700
		}
		if score != 0 {
			display := strings.ToLower(httpMethod + " " + endpoint.Path)
			if at := strings.LastIndex(display, wantPath); at >= 0 {
				start := len([]rune(display[:at]))
				for index := 0; index < len([]rune(wantPath)); index++ {
					indices = append(indices, start+index)
				}
			}
		}
		if score == 0 && wantName != "" {
			methodMatched := methodName != "" && methodName == wantName
			controllerMatched := controller != "" && controller == wantName
			if methodMatched || controllerMatched || qualified == wantName {
				if methodMatched && controllerMatched {
					score = 1000
				} else {
					score = 800
				}
			} else if strings.Contains(qualified, wantName) {
				score = 600
			} else if len(wantName) >= 3 && strings.Contains(target, wantName) {
				score = 500
			} else if match := seMatchText(target, wantName, false); len(match.runs) > 0 {
				score = 300 + match.score
			}
		}
		if score == 0 {
			continue
		}
		line := int(endpoint.Location.Range.Start.Line)
		col := int(endpoint.Location.Range.Start.Col)
		endCol := col
		if endpoint.Handler != nil {
			endCol += len([]rune(*endpoint.Handler))
		}
		title := httpMethod + " " + endpoint.Path
		container := seEndpointContainer(endpoint)
		hits = append(hits, seEndpointHit{
			item: seItem{
				kind: "symbol", symbol: "method", title: title,
				container: container, right: filepath.Base(endpoint.Location.Path),
				path: endpoint.Location.Path, line: line, col: col, endCol: endCol,
				lineSet: true, score: score, indices: indices,
				detail: endpoint.Framework,
			},
			score: score,
			rel:   endpoint.Location.Path,
			line:  line,
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		if hits[i].rel != hits[j].rel {
			return hits[i].rel < hits[j].rel
		}
		return hits[i].line < hits[j].line
	})
	items := make([]seItem, len(hits))
	for i, hit := range hits {
		items[i] = hit.item
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
