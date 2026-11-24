package app

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"mayoi/internal/openidea"
)

func TestPluginEndpointsBecomeHTTPRoutesAndCompletions(t *testing.T) {
	post, get, handler := "POST", "GET", "Orders#create"
	endpoints := []openidea.PluginEndpoint{
		{
			Method: &post, Path: "/orders", Framework: "Ktor", Handler: &handler,
			Location: openidea.Location{
				Path:  "/project/src/Orders.kt",
				Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 7, Col: 4}},
			},
		},
		{
			Method: &get, Path: "/orders/{id}", Framework: "Ktor",
			Location: openidea.Location{
				Path:  "/project/src/Orders.kt",
				Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 10, Col: 4}},
			},
		},
	}
	routes := httpRoutesFromPluginEndpoints(endpoints)
	if len(routes) != 2 || routes[0].method != "POST" || routes[0].url != "/orders" ||
		routes[0].controller != "Ktor" || routes[0].methodName != "create" ||
		routes[0].line != 7 || routes[1].line != 10 {
		t.Fatalf("plugin routes = %+v", routes)
	}
	target := httpCompletionTarget("POST /ord", len("POST /ord"))
	items := httpRouteCompletions(routes, *target)
	if len(items) != 1 || items[0].Label != "/orders" ||
		items[0].Detail != "Ktor.create" {
		t.Fatalf("plugin completions = %+v", items)
	}
}

func TestPluginEndpointGutterGroupsRoutesByNativeLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Orders.kt")
	post, get := "POST", "GET"
	endpoints := []openidea.PluginEndpoint{
		{
			Method: &post, Path: "/orders",
			Location: openidea.Location{
				Path:  path,
				Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 4}},
			},
		},
		{
			Method: &get, Path: "/other",
			Location: openidea.Location{
				Path:  path,
				Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 2}},
			},
		},
		{
			Method: &get, Path: "/same-line",
			Location: openidea.Location{
				Path:  path,
				Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 4}},
			},
		},
	}
	marks := pluginEndpointMarksForPath(endpoints, path)
	if len(marks) != 2 || marks[0].line != 2 || marks[1].line != 4 ||
		len(marks[1].endpoints) != 2 || seEndpointMethod(marks[1].endpoints[0]) != "GET" ||
		marks[1].endpoints[1].Path != "/orders" {
		t.Fatalf("plugin endpoint marks = %+v", marks)
	}
}

func TestPluginEndpointEventInvalidatesHTTPProviderCaches(t *testing.T) {
	m := &Model{}
	m.http.routeLoaded = true
	m.http.routes = []httpRouteEntry{{url: "/stale"}}
	m.http.pluginEndpointsLoaded = true
	m.http.pluginEndpoints = []openidea.PluginEndpoint{{Path: "/stale"}}
	m.acceptPluginEvent(json.RawMessage(`{"type":"endpointsChanged"}`))
	if m.http.routeLoaded || m.http.routes != nil ||
		m.http.pluginEndpointsLoaded || m.http.pluginEndpoints != nil {
		t.Fatalf("endpoint caches were not invalidated: %+v", m.http)
	}
}
