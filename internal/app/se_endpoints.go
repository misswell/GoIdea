package app

import (
	"context"
	"encoding/json"
	"fmt"
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
			s.pluginsBusy = false
			s.pluginsDone = true
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" {
					s.pluginsError = "Loading endpoints failed: " + err.Error()
				}
				s.endpointDone = true
				m.invalidate()
				return
			}
			s.plugins = seLiveEndpointPlugins(snapshot.Plugins)
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
		return
	}
	if s.pluginsBusy || !s.pluginsDone {
		return
	}
	if s.pluginsError != "" {
		s.endpointDone = true
		s.err = s.pluginsError
		return
	}
	if len(s.plugins) == 0 {
		s.endpointDone = true
		return
	}
	s.endpointSeq++
	generation := s.endpointSeq
	plugins := append([]openidea.PluginInfo(nil), s.plugins...)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, seQueryTimeout)
		defer cancel()
		var endpoints []openidea.PluginEndpoint
		var firstError error
		for _, plugin := range plugins {
			if ctx.Err() != nil {
				if firstError == nil {
					firstError = ctx.Err()
				}
				break
			}
			items, err := m.mgr.PluginEndpoints(ctx, Window, plugin.ID)
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" && firstError == nil {
					firstError = err
				}
				continue
			}
			endpoints = append(endpoints, items...)
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
			s.endpoints = endpoints
			s.endpointDone = true
			if firstError != nil && len(endpoints) == 0 {
				s.err = "Finding endpoints failed: " + firstError.Error()
			}
			m.invalidate()
		})
	}()
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

func seEndpointName(endpoint openidea.PluginEndpoint) string {
	if endpoint.Handler == nil {
		return ""
	}
	handler := *endpoint.Handler
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
func seEndpointItems(endpoints []openidea.PluginEndpoint, query string) []seItem {
	text := strings.TrimSpace(query)
	if text == "" || len(endpoints) == 0 {
		return nil
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
	hits := make([]seEndpointHit, 0, min(len(endpoints), seTabLimit))
	for _, endpoint := range endpoints {
		httpMethod := seEndpointMethod(endpoint)
		if method != "" && httpMethod != "*" && httpMethod != method {
			continue
		}
		target := strings.ToLower(seEndpointPath(endpoint.Path))
		name := seEndpointName(endpoint)
		qualified := strings.ToLower(name)
		score := 0
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
		if score == 0 && wantName != "" {
			switch {
			case strings.EqualFold(name, wantName):
				score = 800
			case qualified == wantName || strings.Contains(qualified, wantName):
				score = 600
			case len(wantName) >= 3 && strings.Contains(target, wantName):
				score = 500
			default:
				if _, fuzzyScore, ok := fuzzySubsequence(wantName, target); ok {
					score = 300 + fuzzyScore
				}
			}
		}
		if score == 0 {
			continue
		}
		line := int(endpoint.Location.Range.Start.Line)
		col := int(endpoint.Location.Range.Start.Col)
		title := httpMethod + " " + endpoint.Path
		detail := endpoint.Framework
		if name != "" {
			detail = name + " · " + detail
		}
		if endpoint.Location.Path != "" {
			detail += fmt.Sprintf(" · %s:%d", filepath.Base(endpoint.Location.Path), line+1)
		}
		hits = append(hits, seEndpointHit{
			item: seItem{
				kind: "endpoint", symbol: "method", title: title, detail: detail,
				path: endpoint.Location.Path, line: line, col: col,
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
	return items
}
