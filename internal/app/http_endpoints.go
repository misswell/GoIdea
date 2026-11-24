package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

var httpMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE", "CONNECT"}

var httpHeaderNames = []string{
	"Accept",
	"Accept-Charset",
	"Accept-Encoding",
	"Accept-Language",
	"Access-Control-Request-Headers",
	"Access-Control-Request-Method",
	"Authorization",
	"Cache-Control",
	"Connection",
	"Content-Disposition",
	"Content-Encoding",
	"Content-Language",
	"Content-Length",
	"Content-Type",
	"Cookie",
	"Date",
	"Expect",
	"Forwarded",
	"From",
	"Host",
	"If-Match",
	"If-Modified-Since",
	"If-None-Match",
	"If-Range",
	"If-Unmodified-Since",
	"Max-Forwards",
	"Origin",
	"Pragma",
	"Proxy-Authorization",
	"Range",
	"Referer",
	"TE",
	"Upgrade",
	"User-Agent",
	"Via",
	"X-Correlation-ID",
	"X-CSRF-Token",
	"X-Forwarded-For",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
	"X-Request-ID",
	"X-Requested-With",
}

var httpMediaTypes = []string{
	"application/json",
	"application/xml",
	"application/x-www-form-urlencoded",
	"multipart/form-data",
	"text/plain",
	"text/html",
	"text/xml",
	"text/csv",
	"application/octet-stream",
	"application/graphql",
	"application/x-ndjson",
	"application/problem+json",
	"*/*",
}

var httpHeaderValues = map[string][]string{
	"accept":                        httpMediaTypes,
	"content-type":                  httpMediaTypes,
	"accept-encoding":               {"gzip, deflate, br", "gzip", "deflate", "br", "identity"},
	"content-encoding":              {"gzip", "deflate", "br"},
	"accept-charset":                {"utf-8", "iso-8859-1"},
	"accept-language":               {"en-US", "en", "zh-CN", "zh"},
	"authorization":                 {"Bearer ", "Basic "},
	"proxy-authorization":           {"Bearer ", "Basic "},
	"cache-control":                 {"no-cache", "no-store", "max-age=0", "must-revalidate"},
	"pragma":                        {"no-cache"},
	"connection":                    {"keep-alive", "close", "Upgrade"},
	"upgrade":                       {"websocket", "h2c"},
	"expect":                        {"100-continue"},
	"x-requested-with":              {"XMLHttpRequest"},
	"content-disposition":           {"attachment", "inline", `form-data; name=""`},
	"access-control-request-method": httpMethods,
}

type httpRouteEntry struct {
	method     string
	url        string
	controller string
	methodName string
	path       string
	line       int
	col        int
}

type httpEndpointCompletionTarget struct {
	from   int
	method string
	prefix string
}

type httpHeaderTarget struct {
	kind   string
	name   string
	from   int
	prefix string
}

func httpEndpointURL(path string) string {
	var out strings.Builder
	for i := 0; i < len(path); {
		if path[i] != '{' || i+1 < len(path) && path[i+1] == '{' {
			out.WriteByte(path[i])
			i++
			continue
		}
		close := strings.IndexByte(path[i+1:], '}')
		if close < 0 {
			out.WriteString(path[i:])
			break
		}
		name := path[i+1 : i+1+close]
		if name == "" || strings.ContainsAny(name, "{}/ ") {
			out.WriteByte(path[i])
			i++
			continue
		}
		out.WriteString("{{")
		out.WriteString(name)
		out.WriteString("}}")
		i += close + 2
	}
	return out.String()
}

func httpRenderEndpoint(method, url, handler, host string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	labelMethod := method
	if method == "" || method == "*" {
		method = "GET"
	}
	name := strings.TrimSpace(handler)
	if name == "" {
		name = labelMethod + " " + url
	}
	return "### " + name + "\n" + method + " " +
		strings.TrimSuffix(host, "/") + httpEndpointURL(url) + "\n"
}

func httpStringArg(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text, ok := value.(string); ok {
				return text
			}
		}
	}
	return ""
}

// OpenHTTPEndpoint handles OpenIDEA's http.openEndpoint action. Endpoint
// contributions pass method/url/handler/path maps, and the direct action can
// also receive a plugin endpoint value.
func (m *Model) OpenHTTPEndpoint(arg any) {
	if arg == nil {
		return
	}
	var method, url, handler, sourcePath string
	switch value := arg.(type) {
	case map[string]any:
		method = httpStringArg(value, "method")
		url = httpStringArg(value, "url", "pathUrl")
		handler = httpStringArg(value, "handler", "label")
		sourcePath = httpStringArg(value, "path", "sourcePath")
	case openidea.PluginEndpoint:
		if value.Method != nil {
			method = *value.Method
		}
		url = value.Path
		if value.Handler != nil {
			handler = *value.Handler
		}
		sourcePath = value.Location.Path
	case *openidea.PluginEndpoint:
		if value != nil {
			m.OpenHTTPEndpoint(*value)
		}
		return
	default:
		return
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}
	root := m.Root()
	go func() {
		host := "http://localhost:8080"
		if sourcePath != "" {
			host = springFindHost(sourcePath, root)
		}
		block := httpRenderEndpoint(method, url, handler, host)
		m.post(func() { m.appendGeneratedHTTPRequests([]string{block}) })
	}()
}

func httpRequestLine(line string) (method, url string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		return "", "", false
	}
	for _, candidate := range httpMethods {
		if fields[0] == candidate {
			return candidate, fields[1], true
		}
	}
	return "", "", false
}

// httpCompletionTarget mirrors OpenIDEA's endpoint completion guard: only the
// URL of the first request line in a block is completed.
func httpCompletionTarget(text string, pos int) *httpEndpointCompletionTarget {
	pos = max(0, min(pos, len(text)))
	before := text[:pos]
	lineStart := strings.LastIndexByte(before, '\n') + 1
	line := before[lineStart:]
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 || !slicesContains(httpMethods, fields[0]) {
		return nil
	}
	prefixText := before[:lineStart]
	blockLines := strings.Split(prefixText, "\n")
	blockStart := 0
	for i, candidate := range blockLines {
		if strings.HasPrefix(strings.TrimSpace(candidate), "###") {
			blockStart = i + 1
		}
	}
	for _, candidate := range blockLines[blockStart:] {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if _, _, ok := httpRequestLine(candidate); ok ||
			strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			return nil
		}
	}
	url := fields[1]
	origin := ""
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		rest := url[strings.Index(url, "://")+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			origin = url[:len(url)-len(rest)+slash]
		} else {
			origin = url
		}
	} else if strings.HasPrefix(url, "{{") {
		if close := strings.Index(url, "}}"); close >= 0 {
			origin = url[:close+2]
		}
	}
	if url != "" && origin == "" && !strings.HasPrefix(url, "/") {
		return nil
	}
	return &httpEndpointCompletionTarget{
		from: pos - len(url) + len(origin), method: fields[0], prefix: url[len(origin):],
	}
}

func httpHeaderCompletionTarget(text string, pos int) *httpHeaderTarget {
	pos = max(0, min(pos, len(text)))
	before := text[:pos]
	lineStart := strings.LastIndexByte(before, '\n') + 1
	line := before[lineStart:]
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
		return nil
	}
	block := before[:lineStart]
	if at := strings.LastIndex(block, "###"); at >= 0 {
		block = block[at+3:]
	}
	lines := strings.Split(block, "\n")
	if len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	request := false
	for _, candidate := range lines {
		trimmed := strings.TrimSpace(candidate)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if !request {
			_, _, request = httpRequestLine(candidate)
			if !request {
				lower := strings.ToLower(trimmed)
				request = strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
					strings.HasPrefix(trimmed, "{{")
			}
		} else if trimmed == "" {
			return nil
		}
	}
	if !request {
		return nil
	}
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		if !httpHeaderNamePrefix(line) {
			return nil
		}
		return &httpHeaderTarget{kind: "name", from: lineStart, prefix: line}
	}
	prefix := strings.TrimLeft(line[colon+1:], " \t")
	return &httpHeaderTarget{
		kind: "value", name: strings.TrimSpace(line[:colon]),
		from: pos - len(prefix), prefix: prefix,
	}
}

func httpHeaderNamePrefix(value string) bool {
	for _, ch := range value {
		valid := ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' ||
			ch >= 'a' && ch <= 'z' || ch == '_' || ch == '-'
		if !valid {
			return false
		}
	}
	return true
}

func (m *Model) requestHTTPCompletion(t *Tab, req editor.CompletionRequest) bool {
	if t == nil || t.doc == nil || t.state == nil || !isHTTPRequestFile(t.doc.Path()) {
		return false
	}
	text := t.state.Buffer.String()
	if target := httpHeaderCompletionTarget(text, req.Offset); target != nil {
		if target.kind == "name" {
			if target.prefix == "" && !req.Explicit {
				return true
			}
			items := make([]editor.Completion, 0, len(httpHeaderNames))
			for _, name := range httpHeaderNames {
				items = append(items, editor.Completion{
					Label: name, Detail: "Header", Kind: editor.CompletionField, Insert: name + ": ",
				})
			}
			m.showCompletionResults(t, req, target.from, items, nil)
			return true
		}
		values := httpHeaderValues[strings.ToLower(target.name)]
		if len(values) == 0 {
			return true
		}
		items := make([]editor.Completion, 0, len(values))
		for _, value := range values {
			items = append(items, editor.Completion{
				Label: value, Detail: target.name, Kind: editor.CompletionConstant,
			})
		}
		m.showCompletionResults(t, req, target.from, items, nil)
		return true
	}
	target := httpCompletionTarget(text, req.Offset)
	if target == nil {
		return false
	}
	if !m.http.routeLoaded {
		m.loadSpringRouteIndex(t, &req)
		return true
	}
	items := httpRouteCompletions(m.http.routes, *target)
	if len(items) > 0 {
		m.showCompletionResults(t, req, target.from, items, nil)
	}
	return true
}

func httpRouteCompletions(routes []httpRouteEntry, target httpEndpointCompletionTarget) []editor.Completion {
	seen := map[string]bool{}
	items := make([]editor.Completion, 0)
	for _, route := range routes {
		if route.method != "*" && !strings.EqualFold(route.method, target.method) ||
			!strings.HasPrefix(route.url, target.prefix) || seen[route.url] {
			continue
		}
		seen[route.url] = true
		handler := route.controller + "." + route.methodName
		items = append(items, editor.Completion{
			Label: httpEndpointURL(route.url), Detail: handler, Kind: editor.CompletionText,
			Documentation: route.method + " " + route.url + "\n" + filepath.Base(route.path),
		})
	}
	return items
}

func (m *Model) loadSpringRouteIndex(t *Tab, completion *editor.CompletionRequest) {
	if m.http.routeLoading {
		return
	}
	root := m.Root()
	if root == "" {
		return
	}
	m.http.routeLoading = true
	m.http.routeSeq++
	generation := m.http.routeSeq
	var req *editor.CompletionRequest
	if completion != nil {
		copy := *completion
		req = &copy
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "spring_route_sources", map[string]any{})
		var sources []openidea.RouteSource
		if err == nil {
			err = json.Unmarshal(raw, &sources)
		}
		routes := make([]httpRouteEntry, 0)
		if err == nil {
			routes = httpRoutesFromSources(sources)
		}
		pluginEndpoints, pluginErr := m.queryPluginEndpoints(ctx)
		if err == nil && pluginErr == nil {
			routes = append(routes, httpRoutesFromPluginEndpoints(pluginEndpoints)...)
		}
		m.post(func() {
			if m.http.routeSeq != generation || m.Root() != root {
				return
			}
			m.http.routeLoading = false
			m.http.routeLoaded = true
			m.http.routes = routes
			if pluginErr == nil || len(pluginEndpoints) > 0 {
				m.http.pluginEndpoints = pluginEndpoints
				m.http.pluginEndpointsLoaded = true
				m.http.pluginEndpointsLoading = false
			}
			if err != nil {
				m.http.routeErr = err.Error()
			} else {
				m.http.routeErr = ""
			}
			if req != nil && m.tabOpen(t) && t.state.Buffer.Version() == req.Version &&
				t.state.Cursor() == req.Offset {
				m.requestHTTPCompletion(t, *req)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) invalidateSpringRouteIndex() {
	m.http.routeLoaded = false
	m.http.routeErr = ""
	m.http.routes = nil
	m.http.routeSeq++
}

func httpRoutesFromSources(sources []openidea.RouteSource) []httpRouteEntry {
	var routes []httpRouteEntry
	for _, source := range sources {
		controllers := springControllersInText(source.Text)
		for _, controller := range controllers {
			for _, endpoint := range controller.endpoints {
				line, col := httpLineCol(source.Text, endpoint.offset)
				for _, url := range endpoint.paths {
					routes = append(routes, httpRouteEntry{
						method: endpoint.httpMethod, url: url,
						controller: controller.name, methodName: endpoint.methodName,
						path: source.Path, line: line, col: col,
					})
				}
			}
		}
	}
	return routes
}

func httpLineCol(text string, offset int) (line, col int) {
	offset = max(0, min(offset, len(text)))
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}

func (m *Model) refreshSpringRouteIndex() {
	m.invalidateSpringRouteIndex()
	m.loadSpringRouteIndex(nil, nil)
}

func (m *Model) SpringRouteCompletionStatus() string {
	switch {
	case m.http.routeLoading:
		return "scanning"
	case m.http.routeErr != "":
		return m.http.routeErr
	case m.http.routeLoaded:
		return fmt.Sprintf("%d routes", len(m.http.routes))
	default:
		return ""
	}
}
