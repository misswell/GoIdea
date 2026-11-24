package app

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type pluginEndpointMark struct {
	line      int
	endpoints []openidea.PluginEndpoint
}

func httpRoutesFromPluginEndpoints(endpoints []openidea.PluginEndpoint) []httpRouteEntry {
	routes := make([]httpRouteEntry, 0, len(endpoints))
	for _, endpoint := range endpoints {
		method := seEndpointMethod(endpoint)
		handler := ""
		if endpoint.Handler != nil {
			handler = strings.TrimSpace(*endpoint.Handler)
		}
		methodName := handler
		if at := strings.LastIndexAny(handler, "#."); at >= 0 {
			methodName = handler[at+1:]
		}
		routes = append(routes, httpRouteEntry{
			method: method, url: endpoint.Path,
			controller: strings.TrimSpace(endpoint.Framework), methodName: methodName,
			path: endpoint.Location.Path, line: int(endpoint.Location.Range.Start.Line),
			col: int(endpoint.Location.Range.Start.Col),
		})
	}
	return routes
}

func pluginEndpointMarksForPath(
	endpoints []openidea.PluginEndpoint,
	path string,
) []pluginEndpointMark {
	byLine := map[int][]openidea.PluginEndpoint{}
	for _, endpoint := range endpoints {
		if endpoint.Location.Path == "" || !samePath(endpoint.Location.Path, path) {
			continue
		}
		line := int(endpoint.Location.Range.Start.Line)
		byLine[line] = append(byLine[line], endpoint)
	}
	lines := make([]int, 0, len(byLine))
	for line := range byLine {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	marks := make([]pluginEndpointMark, 0, len(lines))
	for _, line := range lines {
		items := byLine[line]
		sort.SliceStable(items, func(i, j int) bool {
			left, right := seEndpointMethod(items[i]), seEndpointMethod(items[j])
			if left != right {
				return left < right
			}
			return items[i].Path < items[j].Path
		})
		marks = append(marks, pluginEndpointMark{line: line, endpoints: items})
	}
	return marks
}

func (m *Model) queryPluginEndpoints(ctx context.Context) ([]openidea.PluginEndpoint, error) {
	if m == nil || m.mgr == nil {
		return nil, nil
	}
	snapshot, err := m.mgr.PluginsList(ctx, Window)
	if err != nil {
		return nil, err
	}
	plugins := seLiveEndpointPlugins(snapshot.Plugins)
	endpoints := make([]openidea.PluginEndpoint, 0)
	for _, plugin := range plugins {
		if err := ctx.Err(); err != nil {
			return endpoints, err
		}
		items, endpointErr := m.mgr.PluginEndpoints(ctx, Window, plugin.ID)
		if endpointErr != nil {
			if openidea.ErrorKind(endpointErr) != "cancelled" {
				continue
			}
			return endpoints, endpointErr
		}
		endpoints = append(endpoints, items...)
	}
	sort.SliceStable(endpoints, func(i, j int) bool {
		left, right := endpoints[i], endpoints[j]
		if left.Location.Path != right.Location.Path {
			return left.Location.Path < right.Location.Path
		}
		if left.Location.Range.Start.Line != right.Location.Range.Start.Line {
			return left.Location.Range.Start.Line < right.Location.Range.Start.Line
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return seEndpointMethod(left) < seEndpointMethod(right)
	})
	return endpoints, nil
}

func (m *Model) ensurePluginEndpointIndex() {
	d := &m.http
	if d.pluginEndpointsLoaded || d.pluginEndpointsLoading || m.Root() == "" {
		return
	}
	d.pluginEndpointsLoading = true
	d.pluginEndpointSeq++
	generation := d.pluginEndpointSeq
	root := m.Root()
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		requestContext, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		endpoints, err := m.queryPluginEndpoints(requestContext)
		m.post(func() {
			if m.http.pluginEndpointSeq != generation || m.Root() != root {
				return
			}
			m.http.pluginEndpointsLoading = false
			m.http.pluginEndpointsLoaded = true
			if err == nil || len(endpoints) > 0 {
				m.http.pluginEndpoints = endpoints
			}
			m.invalidate()
		})
	}()
}

func (m *Model) invalidatePluginEndpointIndex() {
	m.http.pluginEndpointSeq++
	m.http.pluginEndpointsLoaded = false
	m.http.pluginEndpointsLoading = false
	m.http.pluginEndpoints = nil
}

func (m *Model) pluginEndpointGutter(
	c *ui.Context,
	p *ui.Theme,
	t *Tab,
	editorRow, view ui.Element,
) {
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil {
		return
	}
	m.ensurePluginEndpointIndex()
	marks := pluginEndpointMarksForPath(m.http.pluginEndpoints, t.doc.Path())
	if len(marks) == 0 {
		return
	}
	viewBounds, rowBounds := view.Bounds(), editorRow.Bounds()
	if viewBounds.W <= 0 || viewBounds.H <= 0 {
		return
	}
	first, last := t.state.VisibleLines()
	first = max(0, first)
	last = min(last, t.state.Buffer.LineCount()-1)
	oi := oiPaletteOf(p)
	for _, mark := range marks {
		if mark.line < first || mark.line > last {
			continue
		}
		top := float32(t.state.LineTop(mark.line) - t.state.ScrollY())
		height := float32(t.state.LineTop(mark.line+1) - t.state.LineTop(mark.line))
		if height <= 0 {
			height = t.state.Font.Size * t.state.LineHeight
		}
		if top+height <= 0 || top >= viewBounds.H {
			continue
		}
		localX := viewBounds.X - rowBounds.X + 1
		localY := viewBounds.Y - rowBounds.Y + top + max(0, (height-14)/2)
		mark := mark
		button := ui.Row(c.Key("plugin-endpoint-gutter:"+t.doc.Path()+":"+strconv.Itoa(mark.line))).
			Absolute().Left(localX).Top(localY).Size(14, 14).
			AlignItems(ui.Center).Justify(ui.Center).Radius(3).
			Label("Open in HTTP Client").Tooltip("Open in HTTP Client").
			Cursor(ui.CursorPointer)
		if button.Hovered() {
			button.Background(oi.hoverTint)
		}
		button.Menu(func(menu *ui.Menu) {
			for _, endpoint := range mark.endpoints {
				endpoint := endpoint
				if menu.Item(seEndpointMethod(endpoint) + " " + endpoint.Path).Chosen() {
					m.OpenHTTPEndpoint(endpoint)
				}
			}
		})
		button.Children(func() {
			oiIconEl(c, "fileTypes/http", 14, oi, oi.textMuted)
		})
	}
}
