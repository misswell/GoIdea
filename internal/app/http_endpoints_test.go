package app

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func testHTTPCaret(text string) (string, int) {
	at := strings.IndexByte(text, '|')
	return strings.Replace(text, "|", "", 1), at
}

func TestHTTPEndpointCompletionTargetsOnlyFirstRequestURL(t *testing.T) {
	for _, url := range []string{"/us", "http://localhost:8000/us", "https://example.com/us", "{{host}}/us"} {
		text := "### Users\nGET " + url
		target := httpCompletionTarget(text, len(text))
		if target == nil || target.method != "GET" ||
			text[target.from:] != "/us" {
			t.Errorf("%q target = %+v", url, target)
		}
	}
	for _, text := range []string{
		"GET /users\nX-Test: /us",
		"POST /users\n\nGET /us",
		"# GET /us",
		"@host = /us",
		"GET /users HTTP/1.1",
	} {
		if target := httpCompletionTarget(text, len(text)); target != nil {
			t.Errorf("%q target = %+v, want nil", text, target)
		}
	}
	text := "### One\nGET /old\n### Next\nPOST /us"
	if target := httpCompletionTarget(text, len(text)); target == nil || target.method != "POST" {
		t.Fatalf("second block target = %+v", target)
	}
}

func TestHTTPHeaderCompletionTargetsNamesAndValues(t *testing.T) {
	text, at := testHTTPCaret("POST http://localhost:8080/users\nCon|")
	target := httpHeaderCompletionTarget(text, at)
	if target == nil || target.kind != "name" || target.prefix != "Con" ||
		target.from != strings.Index(text, "Con") {
		t.Fatalf("name target = %+v", target)
	}
	text, at = testHTTPCaret("POST http://x/a\nContent-Type: app|")
	target = httpHeaderCompletionTarget(text, at)
	if target == nil || target.kind != "value" || target.name != "Content-Type" ||
		target.prefix != "app" || text[target.from:at] != "app" {
		t.Fatalf("value target = %+v", target)
	}
	for _, marked := range []string{
		"POS|",
		"POST http://x/a\nContent-Type: application/json\n\n{\"a|",
		"POST http://x/a\n# Acc|",
		"@host = http://x\nAcc|",
		"POST http://x/a\n\n###\nAcc|",
	} {
		text, at := testHTTPCaret(marked)
		if target := httpHeaderCompletionTarget(text, at); target != nil {
			t.Errorf("%q target = %+v, want nil", marked, target)
		}
	}
}

func TestHTTPEndpointRenderNormalizesPathVariable(t *testing.T) {
	if got, want := httpRenderEndpoint("GET", "/users/{id}", "users.show", "http://localhost:8080"),
		"### users.show\nGET http://localhost:8080/users/{{id}}\n"; got != want {
		t.Fatalf("rendered endpoint = %q, want %q", got, want)
	}
	if got, want := httpRenderEndpoint("*", "/users/{id}", "", "http://localhost:8080"),
		"### GET /users/{id}\nGET http://localhost:8080/users/{{id}}\n"; got == want {
		// The any-method route keeps its original label rather than borrowing
		// the GET request line.
		t.Fatalf("any-method endpoint lost its label: %q", got)
	} else if got != "### * /users/{id}\nGET http://localhost:8080/users/{{id}}\n" {
		t.Fatalf("any-method endpoint = %q", got)
	}
}

func TestSpringRouteIndexBuildsSharedHTTPCompletionEntries(t *testing.T) {
	sources := []openidea.RouteSource{{
		Path: "/p/DeepLinkController.java",
		Text: `@RestController
@RequestMapping("/deeplink")
class DeepLinkController {
    @PostMapping("/menu/search")
    String searchMenus() { return "ok"; }
}`,
	}}
	if controllers := springControllersInText(sources[0].Text); len(controllers) != 1 {
		t.Fatalf("controllers = %+v", controllers)
	}
	routes := httpRoutesFromSources(sources)
	if len(routes) != 1 {
		t.Fatalf("routes = %+v", routes)
	}
	route := routes[0]
	if route.method != "POST" || route.url != "/deeplink/menu/search" ||
		route.controller != "DeepLinkController" || route.methodName != "searchMenus" ||
		route.line != 4 {
		t.Fatalf("route = %+v", route)
	}
	target := httpCompletionTarget("POST /deep", len("POST /deep"))
	items := httpRouteCompletions(routes, *target)
	if len(items) != 1 || items[0].Label != "/deeplink/menu/search" ||
		items[0].Detail != "DeepLinkController.searchMenus" {
		t.Fatalf("completion items = %+v", items)
	}
}
