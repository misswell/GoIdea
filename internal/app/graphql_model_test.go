package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestGraphQLParseConfigAndEnvironment(t *testing.T) {
	config := graphqlParseConfig("graphql.config.yml", `schema:
  - schema.graphql
  - 'types/**/*.graphql'
extensions:
  endpoints:
    api:
      url: ${API_URL:http://localhost:4000}/graphql
      headers:
        Authorization: Bearer ${TOKEN}
`)
	if !reflect.DeepEqual(config.Schema, []string{"schema.graphql", "types/**/*.graphql"}) {
		t.Fatalf("schema = %#v", config.Schema)
	}
	if !reflect.DeepEqual(config.Variables, []string{"API_URL", "TOKEN"}) {
		t.Fatalf("variables = %#v", config.Variables)
	}
	if len(config.Endpoints) != 1 || config.Endpoints[0].URL !=
		"${API_URL:http://localhost:4000}/graphql" {
		t.Fatalf("endpoints = %#v", config.Endpoints)
	}
	expanded := graphqlExpandEndpoint(config.Endpoints[0], map[string]string{"TOKEN": "secret"})
	if expanded.URL != "http://localhost:4000/graphql" ||
		expanded.Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("expanded = %#v", expanded)
	}
	missing := graphqlMissingVariables(config.Endpoints[0], map[string]string{"API_URL": "https://api"})
	if !reflect.DeepEqual(missing, []string{"TOKEN"}) {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestGraphQLParseYAMLNestedListsAndMaps(t *testing.T) {
	value := graphqlParseYAML(`# graphql-config
items:
  - name: a
    url: x
  - b
documents: [a.graphql, "b.graphql"]
`)
	want := map[string]any{
		"items": []any{
			map[string]any{"name": "a", "url": "x"},
			"b",
		},
		"documents": []any{"a.graphql", "b.graphql"},
	}
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("parsed = %#v", value)
	}
}

func TestGraphQLGlobAndDiscovery(t *testing.T) {
	for pattern, path := range map[string]string{
		"**/*.graphql":        "a/b/c.graphql",
		"src/*.{graphql,gql}": "src/q.gql",
		"./schema.graphql":    "schema.graphql",
	} {
		if !graphqlGlobToRegExp(pattern).MatchString(path) {
			t.Fatalf("%q did not match %q", pattern, path)
		}
	}
	if graphqlGlobToRegExp("src/*.graphql").MatchString("src/a/q.graphql") {
		t.Fatal("single star crossed a slash")
	}
	files := []graphqlSchemaFile{
		{Path: "/p/schema.graphql"},
		{Path: "/p/api/user.graphqls"},
		{Path: "/p/web/query.graphql"},
	}
	projects := graphqlDiscoverSchemas("/p", []graphqlConfiguredSchema{{
		Path: "/p/api/.graphqlrc.yml",
		Config: graphqlConfig{
			Schema: []string{"*.graphqls", "https://x.dev/graphql"},
		},
	}}, files)
	if len(projects) != 1 || projects[0].Root != "/p/api" ||
		!reflect.DeepEqual(projects[0].Remote, []string{"https://x.dev/graphql"}) ||
		len(projects[0].Files) != 1 || projects[0].Files[0].Path != "/p/api/user.graphqls" {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestGraphQLDiscoveryUsesWorkspaceIndexAndGitIgnore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	write := func(relative, text string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "ignored/\ngenerated/*.graphql\nbuild/\nnode_modules/\n.idea/\n")
	write("ignored/schema.graphql", "type Ignored { id: ID }\n")
	write("generated/ignored.graphql", "type Generated { id: ID }\n")
	write("build/schema.graphql", "type Build { id: ID }\n")
	write("node_modules/package/schema.graphql", "type Dependency { id: ID }\n")
	write(".idea/tool.graphql", "type Tool { id: ID }\n")
	write("api/schema.graphql", "type Query { id: ID }\n")

	files, configs, err := scanGraphQLProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 0 || len(files) != 1 ||
		filepath.ToSlash(files[0].Path) != filepath.ToSlash(filepath.Join(root, "api/schema.graphql")) {
		t.Fatalf("discovery included ignored files: files=%+v configs=%+v", files, configs)
	}
}

func TestGraphQLConfigReadsDoNotConsumeTheSchemaReadLimit(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < graphqlReadLimit; index++ {
		dir := filepath.Join(root, fmt.Sprintf("config-%03d", index))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(dir, "graphql.config.yml"),
			[]byte("schema: schema.graphql\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}
	schema := filepath.Join(root, "zzzz-schema.graphql")
	if err := os.WriteFile(schema, []byte("type Query { id: ID }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, configs, err := scanGraphQLProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != graphqlReadLimit {
		t.Fatalf("configs = %d, want %d", len(configs), graphqlReadLimit)
	}
	for _, file := range files {
		if filepath.Clean(file.Path) == filepath.Clean(schema) {
			if file.Stats.Types != 1 {
				t.Fatalf("schema was not analyzed after all configs: %+v", file)
			}
			return
		}
	}
	t.Fatal("the schema was not discovered")
}

func TestGraphQLEndpointDeclarationOrderAndJSONGutter(t *testing.T) {
	yamlConfig := `schema: schema.graphql
extensions:
  endpoints:
    zeta:
      url: https://zeta.example/graphql
    alpha: https://alpha.example/graphql
`
	config := graphqlParseConfig("graphql.config.yml", yamlConfig)
	if len(config.Endpoints) != 2 ||
		config.Endpoints[0].Name != "zeta" || config.Endpoints[1].Name != "alpha" {
		t.Fatalf("YAML endpoints = %+v", config.Endpoints)
	}
	jsonConfig := `{
  "extensions": {
    "endpoints": {
      "zeta": "https://zeta.example/graphql",
      "alpha": {
        "url": "https://alpha.example/graphql"
      }
    }
  }
}
`
	config = graphqlParseConfig("graphql.config.json", jsonConfig)
	if len(config.Endpoints) != 2 ||
		config.Endpoints[0].Name != "zeta" || config.Endpoints[1].Name != "alpha" {
		t.Fatalf("JSON endpoints = %+v", config.Endpoints)
	}
	lines := graphqlEndpointLines("graphql.config.json", jsonConfig)
	if len(lines) != 2 || lines[0].Endpoint != "zeta" || lines[0].Line != 3 ||
		lines[1].Endpoint != "alpha" || lines[1].Line != 4 {
		t.Fatalf("JSON endpoint lines = %+v", lines)
	}
}

func TestGraphQLEmptyEnvironmentDefaultIsPresent(t *testing.T) {
	endpoint := graphqlEndpoint{
		URL: "${EMPTY:}/graphql",
		Headers: map[string]string{
			"Authorization": "Bearer ${TOKEN}",
		},
	}
	if missing := graphqlMissingVariables(endpoint, nil); !reflect.DeepEqual(missing, []string{"TOKEN"}) {
		t.Fatalf("missing = %+v", missing)
	}
}

func TestGraphQLAnalyzeSchemaAndUnresolvedTypes(t *testing.T) {
	text := `
"""
type NotAType
"""
type Query { a(x: In): Int, color: Color }
# type Commented
extend type Query { b: [Missing!]! }
enum Color { RED }
input In { x: Int, d: Date }
scalar Date
union U = Query | Gone
directive @auth on FIELD_DEFINITION
`
	stats, defined, references := graphqlAnalyzeSchema(text)
	if stats != (graphqlSchemaStats{Types: 3, Inputs: 1, Enums: 1, Directives: 1}) {
		t.Fatalf("stats = %#v", stats)
	}
	wantDefined := []string{"Query", "Color", "In", "Date", "U"}
	if !reflect.DeepEqual(defined, wantDefined) {
		t.Fatalf("defined = %#v", defined)
	}
	if len(references) < 2 {
		t.Fatalf("references = %#v", references)
	}
	project := graphqlSchemaProject{
		Files: []graphqlSchemaFile{{
			Path: "/p/schema.graphql", Stats: stats, Defined: defined, References: references,
		}},
	}
	problems := graphqlUnresolvedTypes(project)
	if len(problems) != 2 {
		t.Fatalf("problems = %#v", problems)
	}
	if problems[0].Diagnostic.Message !=
		"The field type 'Missing' is not present when resolving type 'Query'" {
		t.Fatalf("first = %q", problems[0].Diagnostic.Message)
	}
	if problems[0].Diagnostic.Range.Start.Line != 6 ||
		problems[0].Diagnostic.Range.Start.Col != 24 {
		t.Fatalf("position = %#v", problems[0].Diagnostic.Range.Start)
	}
	if problems[1].Diagnostic.Message !=
		"The field type 'Gone' is not present when resolving type 'U'" {
		t.Fatalf("second = %q", problems[1].Diagnostic.Message)
	}
}

func TestGraphQLSchemaRevalidationUsesUnsavedText(t *testing.T) {
	path := "/p/schema.graphql"
	m := &Model{postHook: func(fn func()) { fn() }}
	m.resetGraphQL()
	m.graphql.discovered = true
	m.graphql.projects = []graphqlSchemaProject{{
		Key: "default", Root: "/p", Files: []graphqlSchemaFile{{Path: path}},
	}}
	m.revalidateGraphQLSchema(path, "type Query { id: ID }\n")
	if diagnostics := m.diags[realPath(path)]["graphql-schema"]; len(diagnostics) != 0 {
		t.Fatalf("initial diagnostics = %+v", diagnostics)
	}
	m.revalidateGraphQLSchema(path, "type Query { id: Missing }\n")
	diagnostics := m.diags[realPath(path)]["graphql-schema"]
	if len(diagnostics) != 1 ||
		!strings.Contains(diagnostics[0].Message, "'Missing'") {
		t.Fatalf("unsaved diagnostics = %+v", diagnostics)
	}
	m.revalidateGraphQLSchema(path, "type Query { id: ID }\n")
	if diagnostics := m.diags[realPath(path)]["graphql-schema"]; len(diagnostics) != 0 {
		t.Fatalf("revalidated diagnostics = %+v", diagnostics)
	}
}

func TestGraphQLTreeSectionsAndDiagnostics(t *testing.T) {
	files := []graphqlSchemaFile{{Path: "/p/schema.graphql"}}
	projects := graphqlDiscoverSchemas("/p", nil, files)
	root := graphqlProjectNodes(projects)[0]
	none := func(string) []openidea.Diagnostic { return nil }
	children := graphqlChildNodes(root, none)
	if len(children) != 3 || children[0].Kind != "summary" ||
		children[1].Kind != "errors" || children[2].Kind != "endpoints" {
		t.Fatalf("children = %#v", children)
	}
	if got := graphqlChildNodes(children[1], none); len(got) != 1 || got[0].Kind != "noErrors" {
		t.Fatalf("errors = %#v", got)
	}
	diagnostics := func(string) []openidea.Diagnostic {
		return []openidea.Diagnostic{
			{Severity: "error", Message: "error"},
			{Severity: "hint", Message: "hint"},
			{Severity: "warning", Message: "warning"},
		}
	}
	got := graphqlChildNodes(children[1], diagnostics)
	if len(got) != 2 || got[0].Diagnostic.Severity != "error" ||
		got[1].Diagnostic.Severity != "warning" {
		t.Fatalf("filtered errors = %#v", got)
	}
}

func TestGraphQLQueryContextIncludesUsedFragments(t *testing.T) {
	document := `# { not: "this" }
query A { ...F }
query B($id: ID!) { node(id: $id) { ... on User { name } ...G } }
fragment F on Query { x }
fragment G on Node { id ...H }
fragment H on Node { "}" }
`
	inB := strings.Index(document, "node(")
	got := graphqlQueryText(document, &graphqlRange{From: inB, To: inB})
	want := `query B($id: ID!) { node(id: $id) { ... on User { name } ...G } }

fragment G on Node { id ...H }

fragment H on Node { "}" }`
	if got != want {
		t.Fatalf("query = %q\nwant %q", got, want)
	}
	inA := strings.Index(document, "...F")
	if got := graphqlQueryText(document, &graphqlRange{From: inA, To: inA}); got !=
		"query A { ...F }\n\nfragment F on Query { x }" {
		t.Fatalf("first query = %q", got)
	}
	if got := graphqlQueryText(document, &graphqlRange{From: 3, To: 3}); got != strings.TrimSpace(document) {
		t.Fatalf("outside query = %q", got)
	}
}

func TestGraphQLQueryWireFormat(t *testing.T) {
	if got := graphqlResponseErrorCount(`{"errors":[{"message":"a"},{"message":"b"}]}`); got != 2 {
		t.Fatalf("errors = %d", got)
	}
	raw := graphqlJSONWithoutTemplates(map[string]any{
		"query":     "{ a { b }}",
		"variables": map[string]any{"o": map[string]any{"p": 1}},
	})
	if strings.Contains(raw, "{{") {
		t.Fatalf("raw JSON contains a template: %s", raw)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["query"] != "{ a { b }}" {
		t.Fatalf("parsed = %#v", parsed)
	}
	request := graphqlHTTPRequestText(
		graphqlEndpoint{
			URL: "http://h/graphql", Headers: map[string]string{"Authorization": "Bearer t"},
		},
		"{ a }", nil,
	)
	want := "POST http://h/graphql\nContent-Type: application/json\n" +
		"Authorization: Bearer t\n\n{\"query\":\"\\u007b a \\u007d\"}\n"
	if request != want {
		t.Fatalf("request = %q\nwant %q", request, want)
	}
}

func TestGraphQLEndpointLines(t *testing.T) {
	yaml := `schema: s.graphql
extensions:
  endpoints:
    countries:
      url: https://c/
    local: http://l/
`
	lines := graphqlEndpointLines("graphql.config.yml", yaml)
	if len(lines) != 2 || lines[0].Endpoint != "countries" || lines[0].Line != 3 ||
		lines[1].Endpoint != "local" || lines[1].Line != 5 {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestGraphQLIntrospectionToSDL(t *testing.T) {
	ref := func(name, kind string) graphqlTypeRef {
		copy := name
		return graphqlTypeRef{Name: &copy, Kind: kind}
	}
	nonNull := func(value graphqlTypeRef) graphqlTypeRef {
		return graphqlTypeRef{Kind: "NON_NULL", OfType: &value}
	}
	list := func(value graphqlTypeRef) graphqlTypeRef {
		return graphqlTypeRef{Kind: "LIST", OfType: &value}
	}
	rootDescription := "Root"
	deprecatedReason := "Use countries"
	defaultValue := `"VN"`
	response := map[string]any{
		"data": map[string]any{"__schema": map[string]any{
			"queryType": map[string]any{"name": "Query"},
			"directives": []any{
				map[string]any{"name": "include", "locations": []string{"FIELD"}, "args": []any{}},
			},
			"types": []any{
				map[string]any{
					"kind": "OBJECT", "name": "Query", "description": rootDescription,
					"interfaces": []any{},
					"fields": []any{
						map[string]any{
							"name": "countries",
							"args": []any{map[string]any{
								"name": "filter", "type": ref("Filter", "INPUT_OBJECT"),
							}},
							"type": nonNull(list(nonNull(ref("Country", "OBJECT")))),
						},
						map[string]any{
							"name": "old", "args": []any{}, "type": ref("String", "SCALAR"),
							"isDeprecated": true, "deprecationReason": deprecatedReason,
						},
					},
				},
				map[string]any{
					"kind": "OBJECT", "name": "Country", "interfaces": []any{},
					"fields": []any{map[string]any{
						"name": "code", "args": []any{},
						"type": nonNull(ref("ID", "SCALAR")),
					}},
				},
				map[string]any{
					"kind": "INPUT_OBJECT", "name": "Filter",
					"inputFields": []any{map[string]any{
						"name": "code", "type": ref("String", "SCALAR"), "defaultValue": defaultValue,
					}},
				},
				map[string]any{
					"kind": "ENUM", "name": "Color",
					"enumValues": []any{map[string]any{"name": "RED"}},
				},
				map[string]any{"kind": "SCALAR", "name": "String"},
				map[string]any{"kind": "OBJECT", "name": "__Type", "fields": []any{}},
			},
		}},
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	sdl, err := graphqlIntrospectionToSDL(string(body))
	if err != nil {
		t.Fatal(err)
	}
	want := `# This file was generated. Do not edit manually.

schema {
    query: Query
}

type Country {
    code: ID!
}

"Root"
type Query {
    countries(filter: Filter): [Country!]!
    old: String @deprecated(reason: "Use countries")
}

enum Color {
    RED
}

input Filter {
    code: String = "VN"
}
`
	if sdl != want {
		t.Fatalf("SDL:\n%s\nwant:\n%s", sdl, want)
	}
}

func TestGraphQLIntrospectionErrorAndTarget(t *testing.T) {
	if _, err := graphqlIntrospectionToSDL(`{"errors":[{"message":"Introspection disabled"}]}`); err == nil ||
		err.Error() != "Introspection disabled" {
		t.Fatalf("error = %v", err)
	}
	project := graphqlSchemaProject{
		Root: "/p",
		Pointers: []string{
			"types/*.graphqls",
			"./api.graphql",
		},
	}
	if got := graphqlIntrospectionTarget(project); got != "/p/api.graphql" {
		t.Fatalf("target = %q", got)
	}
}
