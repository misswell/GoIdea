package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

const testSpringJavaController = `package com.example.demo.web;

import com.example.demo.dto.CreateUser;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping(UserController.BASE + "/users")
public class UserController {
    static final String BASE = "/api";
    private static final String BY_ID = "/{id}";

    @GetMapping
    public String list(@RequestParam(defaultValue = "0") int page,
                       @RequestParam("q") String query,
                       @RequestParam Map<String, String> all) {
        return "";
    }

    @GetMapping(BY_ID)
    public String get(@PathVariable Long id, @RequestHeader("X-Trace") String trace) {
        return "";
    }

    @PostMapping(value = {"", "/new"}, consumes = MediaType.APPLICATION_JSON_VALUE,
                 produces = "application/json")
    public String create(@RequestBody CreateUser body) {
        return "";
    }

    @DeleteMapping(path = BY_ID, params = "force=true", headers = "X-Api=1")
    public void delete(@PathVariable Long id) {}

    record Rename(String name, boolean notify) {}
}
`

const testSpringKotlinController = `package com.example.demo.web

import org.springframework.web.bind.annotation.*

const val API = "/api/v2"

@RestController
@RequestMapping(API)
class OrderController {
    companion object {
        const val ORDERS = "/orders"
    }

    @GetMapping("${ORDERS}/{id}", ORDERS + "/by-id/{id}")
    fun get(@PathVariable id: Long): Order = service.get(id)

    @PostMapping(ORDERS)
    suspend fun create(@RequestBody order: NewOrder,
                       @RequestParam(required = false) dryRun: Boolean?): Order {
        return service.create(order)
    }

    @RequestMapping(value = [ORDERS + "/search"], method = [RequestMethod.POST, RequestMethod.GET])
    fun search(@RequestParam("q") query: String) = service.search(query)
}

data class NewOrder(
    val item: String,
    val quantity: Int,
    val tags: List<String> = emptyList(),
)
`

func springEndpointByMethod(t *testing.T, controllers []springController, name string) springEndpoint {
	t.Helper()
	if len(controllers) != 1 {
		t.Fatalf("controllers = %+v, want one", controllers)
	}
	for _, endpoint := range controllers[0].endpoints {
		if endpoint.methodName == name {
			return endpoint
		}
	}
	t.Fatalf("no endpoint %q in %+v", name, controllers[0].endpoints)
	return springEndpoint{}
}

func TestSpringParsesJavaMappingsAndRequestParts(t *testing.T) {
	controllers := springControllersInText(testSpringJavaController)
	if len(controllers) != 1 || controllers[0].name != "UserController" {
		t.Fatalf("controllers = %+v", controllers)
	}
	if got := []string{
		controllers[0].endpoints[0].methodName,
		controllers[0].endpoints[1].methodName,
		controllers[0].endpoints[2].methodName,
		controllers[0].endpoints[3].methodName,
	}; !slices.Equal(got, []string{"list", "get", "create", "delete"}) {
		t.Fatalf("endpoint order = %v", got)
	}
	list := springEndpointByMethod(t, controllers, "list")
	if !slices.Equal(list.paths, []string{"/api/users"}) {
		t.Errorf("list paths = %v", list.paths)
	}
	if len(list.query) != 2 || list.query[0].name != "page" || list.query[0].defaultVal == nil ||
		*list.query[0].defaultVal != "0" || list.query[1].name != "q" {
		t.Errorf("list query = %+v", list.query)
	}
	get := springEndpointByMethod(t, controllers, "get")
	if !slices.Equal(get.paths, []string{"/api/users/{id}"}) ||
		len(get.headers) != 1 || get.headers[0].value != "{{X-Trace}}" {
		t.Errorf("get endpoint = %+v", get)
	}
	create := springEndpointByMethod(t, controllers, "create")
	if !slices.Equal(create.paths, []string{"/api/users", "/api/users/new"}) ||
		create.bodyType == nil || *create.bodyType != "CreateUser" ||
		create.consumes == nil || *create.consumes != "application/json" ||
		create.produces == nil || *create.produces != "application/json" {
		t.Errorf("create endpoint = %+v", create)
	}
	delete := springEndpointByMethod(t, controllers, "delete")
	if len(delete.query) != 1 || delete.query[0].name != "force" || delete.query[0].defaultVal == nil ||
		*delete.query[0].defaultVal != "true" ||
		len(delete.headers) != 1 || delete.headers[0].name != "X-Api" || delete.headers[0].value != "1" {
		t.Errorf("delete endpoint = %+v", delete)
	}
}

func TestSpringRendersOpenIDEARequestShapes(t *testing.T) {
	controllers := springControllersInText(testSpringJavaController)
	list := springEndpointByMethod(t, controllers, "list")
	if got, want := springRenderRequest(list, "http://localhost:8080", "", nil),
		"### UserController.list\nGET http://localhost:8080/api/users?page=0&q={{q}}\n"; got != want {
		t.Errorf("list request:\n%s\nwant:\n%s", got, want)
	}
	get := springEndpointByMethod(t, controllers, "get")
	if got, want := springRenderRequest(get, "http://localhost:8080", "", nil),
		"### UserController.get\nGET http://localhost:8080/api/users/{{id}}\nX-Trace: {{X-Trace}}\n"; got != want {
		t.Errorf("get request:\n%s\nwant:\n%s", got, want)
	}
	create := springEndpointByMethod(t, controllers, "create")
	got := springRenderRequest(create, "http://localhost:8080", "/api/users/new", springJSONObject{})
	want := "### UserController.create\nPOST http://localhost:8080/api/users/new\n" +
		"Accept: application/json\nContent-Type: application/json\n\n{}\n"
	if got != want {
		t.Errorf("create request:\n%s\nwant:\n%s", got, want)
	}
}

func TestSpringParsesKotlinMappingsAndDataClassBodies(t *testing.T) {
	controllers := springControllersInText(testSpringKotlinController)
	if len(controllers) != 1 || controllers[0].name != "OrderController" {
		t.Fatalf("controllers = %+v", controllers)
	}
	get := springEndpointByMethod(t, controllers, "get")
	if !slices.Equal(get.paths, []string{"/api/v2/orders/{id}", "/api/v2/orders/by-id/{id}"}) {
		t.Errorf("Kotlin get paths = %v", get.paths)
	}
	create := springEndpointByMethod(t, controllers, "create")
	if create.bodyType == nil || *create.bodyType != "NewOrder" ||
		len(create.query) != 1 || create.query[0].name != "dryRun" {
		t.Errorf("Kotlin create = %+v", create)
	}
	search := springEndpointByMethod(t, controllers, "search")
	if search.httpMethod != "POST" || !slices.Equal(search.paths, []string{"/api/v2/orders/search"}) {
		t.Errorf("Kotlin search = %+v", search)
	}
	root := t.TempDir()
	sourcePath := filepath.Join(root, "src", "main", "kotlin", "OrderController.kt")
	lookup := springBodyLookup(sourcePath, root, testSpringKotlinController, controllers[0].endpoints)
	encoded, err := json.Marshal(springSkeleton("NewOrder", lookup, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `{"item":"","quantity":0,"tags":[]}`; got != want {
		t.Fatalf("Kotlin body = %s, want %s", got, want)
	}
}

func TestSpringKotlinPrimaryConstructorClassWithoutBody(t *testing.T) {
	const source = `package demo
data class Address(
    val city: String,
    val zip: Int = 0,
)

fun later() = "not a class body"
`
	model := springParseSourceModel(source)
	if len(model.classes) != 1 || model.classes[0].name != "Address" {
		t.Fatalf("classes = %+v", model.classes)
	}
	class := model.classes[0]
	if len(class.ctorParams) != 2 || class.ctorParams[0].name != "city" ||
		class.ctorParams[1].name != "zip" || len(class.fields) != 0 {
		t.Fatalf("Address = %+v", class)
	}
}

func TestSpringKotlinMultipleRequestMethodsAndTopLevelClassLimit(t *testing.T) {
	const source = `@RestController
@RequestMapping("/api")
class Controller {
    @RequestMapping(value = ["/items"], method = [RequestMethod.POST, RequestMethod.GET])
    fun items() = Unit
}

data class Item(val name: String)

fun topLevel() {}
`
	controllers := springControllersInText(source)
	if len(controllers) != 1 || len(controllers[0].endpoints) != 1 {
		t.Fatalf("controllers = %+v", controllers)
	}
	endpoint := controllers[0].endpoints[0]
	if endpoint.httpMethod != "POST" || !slices.Equal(endpoint.paths, []string{"/api/items"}) {
		t.Fatalf("endpoint = %+v", endpoint)
	}
	model := springParseSourceModel(source)
	if len(model.classes) != 2 || len(model.classes[1].ctorParams) != 1 ||
		model.classes[1].ctorParams[0].name != "name" {
		t.Fatalf("source model = %+v", model.classes)
	}
}

func TestSpringAnnotationFieldsKeepMixedPositionalAndNamedArguments(t *testing.T) {
	annotation := springAnnotation{
		name: "PostMapping",
		args: `"/one", "/two", consumes = MediaType.APPLICATION_JSON_VALUE`,
	}
	fields := springAnnotationFields(annotation)
	if got, want := fields["value"], `["/one", "/two"]`; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
	if got, want := fields["consumes"], "MediaType.APPLICATION_JSON_VALUE"; got != want {
		t.Fatalf("consumes = %q, want %q", got, want)
	}
}

func TestSpringReadsServerConfigurationAndProfiles(t *testing.T) {
	props := springParseProperties("# comment\nserver.port=9090\n" +
		"server.servlet.context-path = /shop/\nspring.application.name: demo\n")
	if got := springHostFromConfig([]springConfig{props}); got != "http://localhost:9090/shop" {
		t.Fatalf("properties host = %q", got)
	}
	yml := springParseYAML("spring:\n  application:\n    name: demo # x\n" +
		"server:\n  port: ${PORT:8181}\n  servlet:\n    context-path: \"/api\"\n" +
		"---\nserver:\n  port: 1\n")
	if got := springHostFromConfig([]springConfig{yml}); got != "http://localhost:8181/api" {
		t.Fatalf("YAML host = %q", got)
	}
	if got := springHostFromConfig([]springConfig{springParseProperties("server.port=${PORT}\n")}); got != "http://localhost:8080" {
		t.Fatalf("unknown placeholder host = %q", got)
	}

	root := t.TempDir()
	resources := filepath.Join(root, "api", "src", "main", "resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "application.yml"),
		[]byte("server:\n  port: 8001\nspring:\n  profiles:\n    active: local,dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "application-local.properties"),
		[]byte("server.port=7000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "application-dev.yml"),
		[]byte("server:\n  port: 9100\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "api", "src", "main", "java", "demo", "Controller.java")
	if got := springFindHost(source, root); got != "http://localhost:9100" {
		t.Fatalf("profile host = %q", got)
	}
}

func TestSpringBodyLookupBuildsOrderedJSON(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "src", "main", "java", "com", "example", "demo", "web", "UserController.java")
	dtoPath := filepath.Join(root, "src", "main", "java", "com", "example", "demo", "dto", "CreateUser.java")
	addressPath := filepath.Join(root, "src", "main", "java", "com", "example", "demo", "dto", "Address.java")
	for _, path := range []string{sourcePath, dtoPath, addressPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(dtoPath, []byte(`package com.example.demo.dto;
public class CreateUser {
  private String name;
  private int age;
  private Address address;
  private java.util.List<String> roles;
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(addressPath, []byte(`package com.example.demo.dto;
public record Address(String city, String zip) {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	endpoints := springControllersInText(testSpringJavaController)
	lookup := springBodyLookup(sourcePath, root, testSpringJavaController, endpoints[0].endpoints)
	value := springSkeleton("CreateUser", lookup, 0)
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"","age":0,"address":{"city":"","zip":""},"roles":[]}`
	if string(encoded) != want {
		t.Fatalf("body = %s, want %s", encoded, want)
	}
}

func TestSpringAppendAndGutterMarksFollowOpenIDEA(t *testing.T) {
	block := "### A.a\nGET http://localhost:8080/a\n"
	if insert, from, caret := springAppendRequests("", []string{block}); insert != block ||
		from != 0 || caret != len("### A.a\n") {
		t.Fatalf("empty append = %q, %d, %d", insert, from, caret)
	}
	existing := "### X\nGET http://x\n"
	insert, from, caret := springAppendRequests(existing, []string{block, block})
	if !strings.HasPrefix(insert, "\n### A.a") || from != len(existing) ||
		!strings.HasPrefix((existing + insert)[caret:], "GET http://localhost:8080/a") {
		t.Fatalf("existing append = %q, %d, %d", insert, from, caret)
	}
	controllers := springControllersInText(testSpringJavaController)
	marks := springMarksForText(controllers, testSpringJavaController)
	if len(marks) != 1+len(controllers[0].endpoints) {
		t.Fatalf("marks = %+v", marks)
	}
	if marks[0].title != "Generate Requests for Controller" {
		t.Errorf("controller mark = %+v", marks[0])
	}
	if !marks[1].hasEndpoint || marks[1].title != "GET /api/users" {
		t.Errorf("endpoint mark = %+v", marks[1])
	}
}

func TestSpringEditorMenuAndActionsAreRegistered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "UserController.java")
	doc := docbridge.Virtual(path, testSpringJavaController)
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer(), Label: "UserController.java"}}
	tab.state.Select(strings.Index(testSpringJavaController, "public String get"), strings.Index(testSpringJavaController, "public String get"))
	m := &Model{tabs: []*Tab{tab}, active: 0}
	tab.state.ContextMenu = func(menu *ui.Menu) { m.editorMenu(menu, tab) }
	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, tab.state).Fill()
	}, 900, 500)
	tester.Frame()
	if err := tester.RightClick("UserController.java"); err != nil {
		t.Fatal(err)
	}
	menu := tester.Menu()
	for _, label := range []string{"Generate Request in HTTP Client", "Generate Requests for Controller"} {
		if !slices.Contains(menu, label) {
			t.Errorf("editor menu %v misses %q", menu, label)
		}
	}
	commands := builtinCommands()
	for _, id := range []string{"http.generateSpringRequest", "http.generateSpringControllerRequests"} {
		found := false
		for _, command := range commands {
			if command.id == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("built-in commands miss %q", id)
		}
	}
}
