package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func endpointString(value string) *string { return &value }

func TestSearchEndpointsRankAndFilterLikeOpenIDEA(t *testing.T) {
	endpoints := []openidea.PluginEndpoint{
		{
			Method: endpointString("POST"), Path: "/api/photos/{id}", Framework: "Spring",
			Handler: endpointString("PhotosController#show"),
			Location: openidea.Location{
				Path: "/p/src/PhotosController.java",
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 12, Col: 4},
				},
			},
		},
		{
			Method: endpointString("GET"), Path: "/api/photos", Framework: "Spring",
			Handler: endpointString("PhotosController#list"),
			Location: openidea.Location{
				Path: "/p/src/PhotosController.java",
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 8, Col: 4},
				},
			},
		},
		{
			Method: nil, Path: "/api/search", Framework: "FastAPI",
			Handler: endpointString("search_photos"),
			Location: openidea.Location{
				Path: "/p/app.py",
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 20, Col: 0},
				},
			},
		},
	}

	exact := seEndpointItems(endpoints, "/api/photos", 50)
	if len(exact) != 2 || exact[0].title != "GET /api/photos" ||
		exact[1].title != "POST /api/photos/{id}" {
		t.Fatalf("exact path match = %+v", exact)
	}
	if exact[0].kind != "symbol" || exact[0].symbol != "method" ||
		exact[0].container != "PhotosController#list" {
		t.Fatalf("route result was not a display-compatible symbol: %+v", exact[0])
	}
	post := seEndpointItems(endpoints, "POST photos", 50)
	if len(post) != 2 || post[0].title != "POST /api/photos/{id}" ||
		post[1].title != "* /api/search" {
		t.Fatalf("method + name match = %+v", post)
	}
	handler := seEndpointItems(endpoints, "search_photos", 50)
	if len(handler) != 1 || handler[0].path != "/p/app.py" {
		t.Fatalf("handler match = %+v", handler)
	}
	if got := seEndpointItems(endpoints, "DELETE /api/search", 50); len(got) != 1 ||
		got[0].title != "* /api/search" {
		t.Fatalf("DELETE returned %+v", got)
	}
	if got := seEndpointItems(endpoints, "DELETE /api/photos", 50); len(got) != 0 {
		t.Fatalf("DELETE photos returned %+v", got)
	}
}

func TestEndpointRowsCarryTheDeclarationLocation(t *testing.T) {
	item := seItem{
		kind: "symbol", symbol: "method", title: "GET /api/photos", path: "/p/PhotosController.java",
		line: 8, col: 4,
	}
	if item.path != "/p/PhotosController.java" || item.line != 8 || item.col != 4 {
		t.Fatalf("endpoint row = %+v", item)
	}
}

func TestOnlyLiveEnabledPluginsContributeEndpoints(t *testing.T) {
	plugins := []openidea.PluginInfo{
		{ID: "active", Enabled: true, Status: "active"},
		{ID: "static", Enabled: true, Status: "static"},
		{ID: "disabled", Enabled: false, Status: "static"},
		{ID: "failed", Enabled: true, Status: "failed"},
		{ID: "consent", Enabled: true, Status: "needsConsent"},
	}
	live := seLiveEndpointPlugins(plugins)
	if len(live) != 2 || live[0].ID != "active" || live[1].ID != "static" {
		t.Fatalf("live endpoint plugins = %+v", live)
	}
}
