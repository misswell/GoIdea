package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

func graphqlTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	root := t.TempDir()
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(root)
	m.ensureGraphQL()
	schema := filepath.Join(root, "schema.graphql")
	config := filepath.Join(root, "graphql.config.yml")
	m.graphql.discovered = true
	m.graphql.projects = []graphqlSchemaProject{{
		Key:        config,
		ConfigPath: config,
		Root:       root,
		Files:      []graphqlSchemaFile{{Path: schema}},
		Endpoints: []graphqlEndpoint{
			{Name: "first", URL: "https://first.example/graphql", Headers: map[string]string{}},
			{Name: "second", URL: "${HOST:https://second.example}/graphql", Headers: map[string]string{}},
		},
		Variables: []string{"HOST"},
	}}
	return m, root
}

func TestGraphQLPanelRendersSchemaTreeAndToolbar(t *testing.T) {
	m, root := graphqlTestModel(t)
	tt := ui.NewTester(func(c *ui.Context) {
		m.graphqlPanel(c, c.Theme())
	}, 900, 620)
	tt.Frame()
	for _, text := range []string{
		filepath.Base(root),
		"Schema discovery summary",
		"Schema errors",
		"No errors found",
		"Endpoints",
		"first",
		"https://first.example/graphql",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from the GraphQL panel: %q", text, tt.Texts())
		}
	}
	for _, label := range []string{
		"Add Schema Configuration",
		"Open Configuration File",
		"Restart Schema Discovery",
		"Help",
	} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("%q is not in the GraphQL toolbar", label)
		}
	}
}

func TestGraphQLResultRendererShowsSuccessErrorsAndRunning(t *testing.T) {
	m, _ := graphqlTestModel(t)
	m.graphql.tab = "result"
	m.graphql.result = &graphqlQueryResult{
		Path:      filepath.Join(m.Root(), "query.graphql"),
		Endpoint:  m.graphql.projects[0].Endpoints[0],
		Status:    200,
		HasStatus: true,
		TotalMs:   0,
		Body:      `{"data":{"countries":[]}}`,
		Size:      27,
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.graphqlResultPanel(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 900, 520)
	tt.Frame()
	if !tt.HasText("0 ms execution time") || !tt.HasText(`"countries"`) {
		t.Fatalf("successful result is missing: %q", tt.Texts())
	}

	m.graphql.result.Body = `{"errors":[{"message":"bad"},{"message":"worse"}]}`
	tt.Frame()
	if !tt.HasText("2 errors") {
		t.Fatalf("GraphQL errors are missing: %q", tt.Texts())
	}

	m.graphql.result = &graphqlQueryResult{
		Path:     filepath.Join(m.Root(), "query.graphql"),
		Endpoint: m.graphql.projects[0].Endpoints[0],
		Error:    "HTTP 502",
	}
	tt.Frame()
	if !tt.HasText("HTTP 502") {
		t.Fatalf("request error is missing: %q", tt.Texts())
	}

	m.graphql.result = nil
	m.graphql.running = &graphqlRun{}
	tt.Frame()
	if !tt.HasText("Running GraphQL query…") || !tt.HasText("Waiting for the response") {
		t.Fatalf("running state is missing: %q", tt.Texts())
	}
}

type graphqlDocBackend struct {
	info openidea.DocInfo
}

func (b *graphqlDocBackend) DocOpen(context.Context, string, string) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *graphqlDocBackend) DocEdit(
	context.Context,
	string,
	uint32,
	uint32,
	position.ChangeSet,
) (uint32, error) {
	return b.info.Version + 1, nil
}

func (b *graphqlDocBackend) DocSnapshot(context.Context, string, uint32) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *graphqlDocBackend) DocSave(context.Context, string, uint32) error  { return nil }
func (b *graphqlDocBackend) DocClose(context.Context, string, uint32) error { return nil }

func graphqlTabForTest(t *testing.T, path, text string) *Tab {
	t.Helper()
	backend := &graphqlDocBackend{info: openidea.DocInfo{
		ID: 81, Path: path, Text: text, Version: 1, Language: "graphql",
	}}
	doc, err := docbridge.Open(context.Background(), backend, Window, path)
	if err != nil {
		t.Fatal(err)
	}
	return &Tab{doc: doc, state: &editor.State{
		Buffer: doc.Buffer(), LineHeight: 1.4, Font: ui.Font{Size: 13},
	}}
}

func TestGraphQLVariablesEditorAndEndpointMenuRender(t *testing.T) {
	m, root := graphqlTestModel(t)
	path := filepath.Join(root, "schema.graphql")
	tab := graphqlTabForTest(t, path, "type Query { id: ID }\n")
	m.tabs = []*Tab{tab}
	m.active = 0
	m.graphql.variablesShown[path] = true
	m.graphql.variables[path] = `{"id":"1"}`

	tt := ui.NewTester(func(c *ui.Context) {
		m.graphqlEditorToolbar(c, c.Theme(), tab)
	}, 900, 190)
	tt.Frame()
	if !tt.HasText("Variables") {
		t.Fatalf("variables editor is missing: %q", tt.Texts())
	}
	if _, ok := tt.Find("GraphQL variables"); !ok {
		t.Fatal("the variables editor has no accessible label")
	}
	if !tt.HasText("first - https://first.example/graphql") {
		t.Fatalf("endpoint menu is missing: %q", tt.Texts())
	}
	if _, ok := tt.Find("Execute GraphQL"); !ok {
		t.Fatal("Execute GraphQL is missing")
	}
	if _, ok := tt.Find("Rerun Introspection Query"); !ok {
		t.Fatal("Rerun Introspection Query is missing")
	}
}

func TestGraphQLEnvironmentDialogRendersOffscreen(t *testing.T) {
	m, root := graphqlTestModel(t)
	m.graphql.environmentDialog = &graphqlEnvironmentDialog{
		ConfigPath: filepath.Join(root, "graphql.config.yml"),
		Variables:  []string{"HOST", "REGION"},
		Values:     map[string]string{"HOST": "example.test", "REGION": "eu"},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.graphqlEnvironmentDialog(c, c.Theme())
	}, 700, 420)
	tt.Frame()
	for _, text := range []string{
		"GraphQL Environment Variables",
		"HOST",
		"REGION",
		"Copy",
		"Paste",
		"Cancel",
		"OK",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from the environment dialog: %q", text, tt.Texts())
		}
	}
}

func TestGraphQLEndpointGutterAndConfirmationRenderOffscreen(t *testing.T) {
	m, root := graphqlTestModel(t)
	path := filepath.Join(root, "graphql.config.yml")
	text := `schema: schema.graphql
extensions:
  endpoints:
    first:
      url: https://first.example/graphql
    second: https://second.example/graphql
`
	tab := graphqlTabForTest(t, path, text)
	tab.state.GoTo(editor.Pos{Line: 3, Col: 0})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(640).Height(280).Children(func() {
			editorRow := ui.Row(c).Width(640).Height(260).AlignItems(ui.Stretch)
			editorRow.Children(func() {
				view := editor.View(c, tab.state).Width(640).Height(260)
				m.graphqlGutter(c, c.Theme(), tab, editorRow, view)
			})
		})
	}, 640, 280)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("Run Introspection Query"); !ok {
		t.Fatal("the endpoint introspection gutter button is missing")
	}

	m.graphql.introspectionAsk = &graphqlIntrospectionConfirmation{
		Target:   filepath.Join(root, "schema.graphql"),
		SDL:      graphqlGeneratedHeader + "\n\ntype Query { id: ID }\n",
		Endpoint: m.graphql.projects[0].Endpoints[0],
	}
	confirmation := ui.NewTester(func(c *ui.Context) {
		m.graphqlIntrospectionDialog(c, c.Theme())
	}, 700, 360)
	confirmation.Frame()
	for _, text := range []string{
		"Get GraphQL Schema from Endpoint",
		"Replace the contents of schema.graphql with the schema of first?",
		"Cancel",
		"Replace",
	} {
		if !confirmation.HasText(text) && text != "Replace" {
			t.Fatalf("%q is missing from the confirmation: %q", text, confirmation.Texts())
		}
	}
	if _, ok := confirmation.Find("Replace"); !ok {
		t.Fatal("Replace is missing from the confirmation")
	}
}

func TestGraphQLActionsMenuToolWindowAndOverlaysAreRegistered(t *testing.T) {
	wantCommands := map[string]string{
		"graphql.showToolWindow":    "GraphQL",
		"graphql.addConfiguration":  "Add Schema Configuration",
		"graphql.editConfiguration": "Open Configuration File",
		"graphql.introspect":        "Rerun Introspection Query",
		"graphql.restartDiscovery":  "Restart Schema Discovery",
		"graphql.runQuery":          "Execute GraphQL",
	}
	for id, title := range wantCommands {
		command, ok := Command(id)
		if !ok || command.title != title {
			t.Fatalf("%s = %+v, %v", id, command, ok)
		}
	}
	if command, _ := Command("graphql.runQuery"); command.shortcut != "Cmd+Enter" {
		t.Fatalf("graphql.runQuery shortcut = %q", command.shortcut)
	}
	if toolWindowRegistry["graphql"] == nil || toolWindowRegistry["graphql"].title != "GraphQL" {
		t.Fatal("GraphQL is not registered as a tool window")
	}
	for _, id := range []string{
		"graphql.environment", "graphql.error", "graphql.introspectionConfirmation",
	} {
		if overlays.byID[id] == nil {
			t.Fatalf("%s is not registered", id)
		}
	}

	var tools menuEntry
	for _, group := range mainMenuGroups() {
		if group.label == "Tools" {
			tools = group
			break
		}
	}
	var graphql menuEntry
	for _, item := range tools.sub {
		if item.label == "GraphQL" && len(item.sub) > 0 {
			graphql = item
			break
		}
	}
	if len(graphql.sub) != len(wantCommands) {
		t.Fatalf("Tools › GraphQL has %d entries, want %d", len(graphql.sub), len(wantCommands))
	}
	seen := map[string]bool{}
	for _, item := range graphql.sub {
		seen[item.command] = true
	}
	for id := range wantCommands {
		if !seen[id] {
			t.Errorf("Tools › GraphQL is missing %s", id)
		}
	}
}

func TestGraphQLHTTPExecutePayloadAndZeroTotalTime(t *testing.T) {
	m, root := graphqlTestModel(t)
	body := `{"data":{"ok":true}}`
	done := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		done <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "http_execute" {
			t.Fatalf("command = %q", command)
		}
		payload := args.(map[string]any)
		if payload["path"] != nil || payload["index"] != uint32(0) || payload["env"] != nil {
			t.Fatalf("payload = %+v", payload)
		}
		text := payload["text"].(string)
		for _, want := range []string{
			"POST https://first.example/graphql",
			`{"query":"query GoIDEA \u007b ok \u007d"}`,
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("request text is missing %q:\n%s", want, text)
			}
		}
		raw, err := json.Marshal(openidea.HTTPExchange{Response: &openidea.HTTPResponse{
			Status: 200, Body: &body, Size: float64(len(body)), TotalMs: 0, TTFBMs: 91,
		}})
		return raw, err
	}
	project := &m.graphql.projects[0]
	endpoint := project.Endpoints[0]
	path := filepath.Join(root, "query.graphql")
	m.sendGraphQLQuery(
		path, project, endpoint, "query GoIDEA { ok }", nil, false,
	)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GraphQL request did not complete")
	}
	if m.graphql.result == nil || m.graphql.result.TotalMs != 0 ||
		m.graphql.result.Status != 200 || m.graphql.result.Body != body {
		t.Fatalf("result = %+v", m.graphql.result)
	}
}

func TestGraphQLFailedRequestNotificationRetries(t *testing.T) {
	m, root := graphqlTestModel(t)
	requests := make(chan string, 2)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "http_execute" {
			return nil, errors.New("unexpected command " + command)
		}
		requests <- args.(map[string]any)["text"].(string)
		return nil, errors.New("HTTP 502")
	}
	project := &m.graphql.projects[0]
	path := filepath.Join(root, "query.graphql")
	m.sendGraphQLQuery(path, project, project.Endpoints[0], "query { ok }", nil, false)
	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("the failed GraphQL request did not start")
	}

	waitFor(t, "the GraphQL retry notification", func() bool {
		for _, notification := range m.notify.items {
			if notification.key == "graphql-run" {
				return true
			}
		}
		return false
	})
	var notification *ideNotification
	for i := range m.notify.items {
		if m.notify.items[i].key == "graphql-run" {
			notification = &m.notify.items[i]
			break
		}
	}
	if notification == nil || len(notification.actions) != 2 ||
		notification.actions[0].label != "Retry" ||
		notification.actions[1].label != "Show error" {
		t.Fatalf("failed GraphQL notification = %+v", notification)
	}
	notification.actions[0].run(m)
	select {
	case text := <-requests:
		if !strings.Contains(text, "POST https://first.example/graphql") ||
			!strings.Contains(text, `"query":"query \u007b ok \u007d"`) {
			t.Fatalf("retry request = %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Retry did not resend the failed GraphQL request")
	}
	notification.actions[1].run(m)
	if m.graphql.errorDialog == nil || m.graphql.errorDialog.Message != "HTTP 502" {
		t.Fatalf("Show error dialog = %+v", m.graphql.errorDialog)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.graphqlErrorDialog(c, c.Theme())
	}, 700, 420)
	tt.Frame()
	for _, text := range []string{"GraphQL error", "HTTP 502", "OK"} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from the GraphQL error dialog: %q", text, tt.Texts())
		}
	}
}

func TestGraphQLRunGreysRangesOutsideTheSentOperation(t *testing.T) {
	m, root := graphqlTestModel(t)
	path := filepath.Join(root, "query.graphql")
	text := "query A { a }\nquery B { b }\n"
	tab := graphqlTabForTest(t, path, text)
	m.tabs = []*Tab{tab}
	m.active = 0

	second := strings.Index(text, "query B")
	context := graphqlQueryContext(text, &graphqlRange{From: second, To: second})
	m.markGraphQLSent(tab, context.Ranges)
	wantRanges := graphqlOutsideRanges(len(text), context.Ranges)
	unsent := m.graphqlUnsentTextBackgrounds(tab)
	gotRanges := make([]graphqlRange, 0, len(unsent))
	for _, background := range unsent {
		gotRanges = append(gotRanges, graphqlRange{
			From: background.Start, To: background.End,
		})
	}
	if !reflect.DeepEqual(gotRanges, wantRanges) {
		t.Fatalf("unsent backgrounds = %+v, want %+v", gotRanges, wantRanges)
	}

	tab.doc.Buffer().Edit(editor.Edit{
		Start: len(text), End: len(text), Text: "query C { c }\n",
	})
	if got := m.graphqlUnsentTextBackgrounds(tab); len(got) != 0 {
		t.Fatalf("stale sent ranges survived an edit: %+v", got)
	}
}

func TestGraphQLCancelSendsCancelRequest(t *testing.T) {
	m, root := graphqlTestModel(t)
	started := make(chan uint32, 1)
	cancelled := make(chan uint32, 1)
	m.invokeHook = func(ctx context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "http_execute":
			request := args.(map[string]any)["request"].(uint32)
			started <- request
			<-ctx.Done()
			return nil, ctx.Err()
		case "cancel_request":
			request := args.(map[string]any)["id"].(uint32)
			cancelled <- request
			return json.RawMessage(`null`), nil
		default:
			return nil, errors.New("unexpected command " + command)
		}
	}
	project := &m.graphql.projects[0]
	m.sendGraphQLQuery(
		filepath.Join(root, "query.graphql"),
		project,
		project.Endpoints[0],
		"query { ok }",
		nil,
		false,
	)
	var request uint32
	select {
	case request = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("http_execute did not start")
	}
	m.cancelGraphQLRun()
	select {
	case got := <-cancelled:
		if got != request {
			t.Fatalf("cancel request = %d, want %d", got, request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel_request was not sent")
	}
	if m.graphql.running != nil {
		t.Fatal("the cancelled run is still marked running")
	}
}

func TestGraphQLRerunIntrospectionUsesLastEndpoint(t *testing.T) {
	m, root := graphqlTestModel(t)
	project := &m.graphql.projects[0]
	project.Endpoints[1].URL = "https://second.example/graphql"
	m.graphql.endpointBySchema[project.Key] = project.Endpoints[0].Name
	m.graphql.lastIntrospection = &graphqlLastIntrospection{
		Path:     project.ConfigPath,
		Endpoint: project.Endpoints[1].Name,
	}
	requests := make(chan string, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "http_execute" {
			return nil, errors.New("unexpected command " + command)
		}
		requests <- args.(map[string]any)["text"].(string)
		return nil, errors.New("stop after capture")
	}
	m.RerunGraphQLIntrospection(filepath.Join(root, "query.graphql"))
	select {
	case text := <-requests:
		if !strings.Contains(text, "POST https://second.example/graphql") {
			t.Fatalf("introspection request used the wrong endpoint: %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rerun introspection did not send a request")
	}
}

func TestGraphQLIntrospectionReplacementIsUndoable(t *testing.T) {
	target := filepath.Join(t.TempDir(), "schema.graphql")
	old := graphqlGeneratedHeader + "\n\ntype Old { id: ID }\n"
	next := graphqlGeneratedHeader + "\n\ntype Query { id: ID }\n"
	if err := os.WriteFile(target, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &graphqlDocBackend{info: openidea.DocInfo{
		ID: 82, Path: target, Text: old, Version: 1,
	}}
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		reg:      docbridge.NewRegistry(backend, Window),
	}
	m.ensureGraphQL()
	opened := make(chan *Tab, 1)
	once := sync.Once{}
	m.postHook = func(fn func()) {
		fn()
		if tab := m.tab(); tab != nil {
			once.Do(func() { opened <- tab })
		}
	}
	m.writeGraphQLIntrospection(target, next, graphqlEndpoint{Name: "first", URL: "https://first/graphql"})
	var tab *Tab
	select {
	case tab = <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("the introspection schema did not open")
	}
	if got := tab.state.Buffer.String(); got != next {
		t.Fatalf("buffer = %q, want %q", got, next)
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != old {
		t.Fatalf("direct write changed the file: %q, %v", raw, err)
	}
	if !tab.state.Buffer.Undo() || tab.state.Buffer.String() != old {
		t.Fatalf("undo did not restore the previous schema: %q", tab.state.Buffer.String())
	}
}

func TestGraphQLEnvironmentRoundTripAndPersistence(t *testing.T) {
	names := []string{"FIRST", "SECOND"}
	values := map[string]string{"FIRST": "one", "SECOND": "two"}
	text := graphqlFormatEnvironment(names, values)
	if !reflect.DeepEqual(graphqlParseEnvironment(text, names), values) {
		t.Fatalf("environment round trip = %q", text)
	}
	settings := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: settings}}
	m.saveGraphQLEnvironment("/p/graphql.config.yml", values)
	loaded := LoadPreferences(settings)
	if got := loaded.GraphQL.Environment["/p/graphql.config.yml"]; !reflect.DeepEqual(got, values) {
		t.Fatalf("persisted environment = %+v", got)
	}
}

func TestGraphQLSchemaDebounceAndFilesystemInvalidation(t *testing.T) {
	m, root := graphqlTestModel(t)
	path := filepath.Join(root, "schema.graphql")
	tab := graphqlTabForTest(t, path, "type Query { id: ID }\n")
	m.tabs = []*Tab{tab}
	m.active = 0
	m.graphqlSchemaFor(path)
	tab.state.Buffer.Set("type Query { id: Missing }\n")
	start := time.Unix(100, 0)
	m.noteGraphQLSchemaEdit(tab, start)
	m.revalidatePendingGraphQLSchemas(start.Add(graphqlValidationDelay - time.Millisecond))
	if got := m.diags[realPath(path)]["graphql-schema"]; len(got) != 0 {
		t.Fatalf("revalidated before the debounce: %+v", got)
	}
	m.revalidatePendingGraphQLSchemas(start.Add(graphqlValidationDelay))
	if got := m.diags[realPath(path)]["graphql-schema"]; len(got) != 1 {
		t.Fatalf("diagnostics after the debounce = %+v", got)
	}

	before := m.graphql.generation
	m.graphql.discovered = true
	m.postHook = func(func()) {}
	m.acceptFsChanged(openidea.FsChangedEvent{Modified: []string{path}})
	if m.graphql.discovered || !m.graphql.discovering {
		t.Fatalf("filesystem change did not restart discovery: %+v", m.graphql)
	}
	if m.graphql.generation <= before {
		t.Fatalf("generation = %d, want greater than %d", m.graphql.generation, before)
	}
	m.resetGraphQL()
	if m.graphql.generation != before+2 {
		t.Fatalf("reset generation = %d, want %d", m.graphql.generation, before+2)
	}
}
