package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func newServicesTestModel(t *testing.T) *Model {
	t.Helper()
	m := &Model{root: t.TempDir()}
	m.ensureServicesProject()
	m.services.visible = true
	return m
}

func TestServicesListsOnlyAddedRunConfigurationTypes(t *testing.T) {
	m := newServicesTestModel(t)
	m.run.configs = []openidea.RunConfig{
		{ID: "app", Name: "Application", Kind: "application"},
		{ID: "maven", Name: "Maven", Kind: "maven"},
	}
	if nodes := m.serviceTree(); len(nodes) != 0 {
		t.Fatalf("Services listed configurations before a type was added: %+v", nodes)
	}

	m.addServiceType("application")
	nodes := m.serviceTree()
	if len(nodes) != 1 || nodes[0].label != "Application" {
		t.Fatalf("tree = %+v", nodes)
	}
	if len(nodes[0].children) != 1 || nodes[0].children[0].label != "Application" {
		t.Fatalf("configuration node = %+v", nodes[0].children)
	}
	for _, node := range nodes {
		if node.label == "Maven" || node.label == "Build" || node.label == "Tools" {
			t.Fatalf("unrelated content appeared in Services: %+v", nodes)
		}
	}
}

func TestServicesFoldersOrderHiddenAndRunningCounts(t *testing.T) {
	m := newServicesTestModel(t)
	infra := "Infra"
	business := "Business"
	m.run.configs = []openidea.RunConfig{
		{ID: "a", Name: "Gateway", Kind: "spring-boot", Folder: &infra},
		{ID: "b", Name: "Orders", Kind: "spring-boot", Folder: &business},
		{ID: "c", Name: "Loose", Kind: "spring-boot"},
	}
	m.services.types = []string{"spring-boot"}
	m.services.order["spring-boot"] = []string{"b", "a", "c"}
	m.services.hidden["spring-boot"] = []string{"c"}
	m.services.children = []*runChild{{
		config: m.run.configs[1], starting: true, service: true, root: m.Root(),
		groupID: "service:b",
	}}

	nodes := m.serviceTree()
	if len(nodes) != 1 || nodes[0].detail != "1 running" {
		t.Fatalf("group = %+v", nodes)
	}
	children := nodes[0].children
	if len(children) != 2 || children[0].nodeKind != "folder" ||
		children[0].label != "Business" || children[0].detail != "1 running" {
		t.Fatalf("children = %+v", children)
	}
	if len(children[0].children) != 1 || children[0].children[0].configID != "b" {
		t.Fatalf("Business= %+v", children[0].children)
	}
	if children[1].label != "Infra" || children[1].children[0].configID != "a" {
		t.Fatalf("Infra = %+v", children[1])
	}
	for _, child := range children {
		if child.label == "Loose" {
			t.Fatal("hidden configuration is still visible")
		}
	}
}

func TestServiceStartedPortMatchesOpenIDEA(t *testing.T) {
	cases := []struct {
		line string
		port int
	}{
		{"TomcatWebServer  : Tomcat started on port 8080 (http) with context path '/'", 8080},
		{"Tomcat started on port(s): 8081 (http) with context path ''", 8081},
		{"o.s.b.web.embedded.netty.NettyWebServer  : Netty started on port 9090", 9090},
		{"Netty started on port(s): 9091", 9091},
		{"Undertow started on port(s) 8082 (http)", 8082},
		{"Jetty started on port 9083 (http/1.1) with context path '/'", 9083},
		{"Tomcat initialized with port 8080 (http)", 0},
		{"Tomcat started on port 99999", 0},
	}
	for _, test := range cases {
		child := &runChild{output: []outputLine{{text: test.line}}}
		if got := servicePort(child); got != test.port {
			t.Errorf("servicePort(%q) = %d, want %d", test.line, got, test.port)
		}
	}
}

func TestServicesPreferencesRoundTripPerProject(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{root: root, cfg: Config{SettingsPath: path}}
	m.ensureServicesProject()
	m.addServiceType("spring-boot")
	m.services.hidden["spring-boot"] = []string{"b"}
	m.services.order["spring-boot"] = []string{"b", "a"}
	m.saveServicesPrefs()

	stored := LoadPreferences(path)
	project := stored.Services.Projects[servicesProjectKey(root)]
	if len(project.Types) != 1 || project.Types[0] != "spring-boot" ||
		len(project.Hidden["spring-boot"]) != 1 ||
		len(project.Order["spring-boot"]) != 2 {
		t.Fatalf("stored Services = %+v", project)
	}

	other := &Model{root: t.TempDir(), prefs: stored}
	other.ensureServicesProject()
	if len(other.services.types) != 0 {
		t.Fatalf("another project inherited types: %+v", other.services.types)
	}
}

func TestServicesBadgeExcludesHiddenConfigurations(t *testing.T) {
	m := newServicesTestModel(t)
	cfg := openidea.RunConfig{ID: "app", Name: "Application", Kind: "application"}
	m.run.configs = []openidea.RunConfig{cfg}
	m.services.types = []string{"application"}
	m.services.children = []*runChild{{
		config: cfg, running: true, service: true, root: m.Root(),
	}}
	if got := m.runningServiceCount(); got != 1 {
		t.Fatalf("running count = %d, want 1", got)
	}
	m.services.hidden["application"] = []string{"app"}
	if got := m.runningServiceCount(); got != 0 {
		t.Fatalf("hidden configuration counted as %d services", got)
	}
}

func TestServicesDebugActivityEndsWithTheSession(t *testing.T) {
	m := newServicesTestModel(t)
	cfg := openidea.RunConfig{ID: "app", Name: "Application", Kind: "application"}
	m.run.configs = []openidea.RunConfig{cfg}
	m.services.types = []string{"application"}
	m.debug.lastID = cfg.ID
	m.debug.session = 17

	if !m.debugServiceActive(cfg.ID) {
		t.Fatal("active debug session was not considered running")
	}
	if got := m.runningServiceCount(); got != 1 {
		t.Fatalf("active debug count = %d, want 1", got)
	}

	m.debug.session = 0
	m.debug.ended = true
	m.debug.appendOutput("Application stopped", "console")
	if m.debugServiceActive(cfg.ID) {
		t.Fatal("ended debug session is still considered running")
	}
	if got := m.runningServiceCount(); got != 0 {
		t.Fatalf("ended debug count = %d, want 0", got)
	}
	if node := m.serviceConfigNode(cfg); node.debugging || node.detail == "Debugging" {
		t.Fatalf("ended debug node = %+v", node)
	}
}

func TestServicesKeepsHistoricalDebugOutputWithoutMarkingItActive(t *testing.T) {
	m := newServicesTestModel(t)
	cfg := openidea.RunConfig{ID: "app", Name: "Application", Kind: "application"}
	m.run.configs = []openidea.RunConfig{cfg}
	m.services.types = []string{"application"}
	m.services.selected = serviceConfigKey(cfg.ID)
	m.debug.lastID = cfg.ID
	m.debug.ended = true
	m.debug.appendOutput("Application stopped", "console")

	if !m.debugServiceHasOutput(cfg.ID) {
		t.Fatal("historical debug output was lost")
	}
	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1200, 700)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	for _, text := range []string{"Application stopped", "Finished", "Debug"} {
		if !tt.HasText(text) {
			t.Fatalf("historical debug console is missing %q: %v", text, tt.Texts())
		}
	}
}

func TestServiceAddCategoriesPutProjectTypesFirst(t *testing.T) {
	categories := categorizeServiceKinds([]string{
		"plugin-z", "spring-boot", "maven", "npm", "python", "cargo", "go", "shell",
	})
	if categories[0].name != "Java" {
		t.Fatalf("first category = %q", categories[0].name)
	}
	if categories[len(categories)-1].name != "Others" {
		t.Fatalf("last category = %q", categories[len(categories)-1].name)
	}
}

func TestServiceImportShowsPickedConfigurations(t *testing.T) {
	m := newServicesTestModel(t)
	m.run.configs = []openidea.RunConfig{
		{ID: "a", Name: "Gateway", Kind: "spring-boot"},
		{ID: "b", Name: "Orders", Kind: "spring-boot"},
	}
	m.beginServiceImport("spring-boot")
	state := m.services.importing
	if state == nil || len(state.choices) != 2 || !state.choices[0].picked || !state.choices[1].picked {
		t.Fatalf("import state = %+v", state)
	}
	state.choices[1].picked = false
	m.finishServiceImport(true)
	if !m.serviceTypeAdded("spring-boot") || len(m.services.hidden["spring-boot"]) != 1 ||
		m.services.hidden["spring-boot"][0] != "b" {
		t.Fatalf("import result = %+v", m.services)
	}
}

func TestServiceDisplayOrderMovesAndPersists(t *testing.T) {
	m := newServicesTestModel(t)
	m.run.configs = []openidea.RunConfig{
		{ID: "a", Name: "A", Kind: "application"},
		{ID: "b", Name: "B", Kind: "application"},
		{ID: "c", Name: "C", Kind: "application"},
	}
	m.services.types = []string{"application"}
	m.moveServiceConfig("application", "c", -1)
	if got := m.serviceVisibleConfigsForType("application"); got[0].ID != "a" ||
		got[1].ID != "c" || got[2].ID != "b" {
		t.Fatalf("ordered configurations = %+v", got)
	}
	if got := m.services.order["application"]; len(got) != 3 ||
		got[0] != "a" || got[1] != "c" || got[2] != "b" {
		t.Fatalf("stored order = %v", got)
	}
	m.reorderServiceConfig("b", "a", false)
	if got := m.serviceVisibleConfigsForType("application"); got[0].ID != "b" ||
		got[1].ID != "a" || got[2].ID != "c" {
		t.Fatalf("drag order = %+v", got)
	}
}

func TestServicesOwnTheirRunEvents(t *testing.T) {
	m := newServicesTestModel(t)
	cfg := openidea.RunConfig{ID: "app", Name: "Application", Kind: "application"}
	child := &runChild{
		channel: 41, runID: 42, config: cfg, service: true, root: m.Root(),
		running: true, done: make(chan error, 1),
	}
	m.services.children = []*runChild{child}
	if got := m.childByChannel(41); got != child {
		t.Fatalf("channel routed to %+v", got)
	}
	code := int32(0)
	if !m.acceptChildExit(42, &code) || !child.finished || child.running {
		t.Fatalf("service exit was not applied: %+v", child)
	}
	if m.run.running || m.run.err != "" {
		t.Fatalf("service event changed the Run pane: %+v", m.run)
	}
}

func TestServicesRunConfigurationsConcurrentlyWithoutUsingRunSessions(t *testing.T) {
	hook := &runChainHook{hold: true}
	m := newRunChainModel(t, hook,
		map[string]any{"id": "one", "name": "One", "kind": "shell", "command": "echo"},
		map[string]any{"id": "two", "name": "Two", "kind": "shell", "command": "echo"},
	)
	serviceCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.ctx = serviceCtx
	m.services.root = m.Root()
	m.services.types = []string{"shell"}
	m.services.hidden = map[string][]string{}
	m.services.order = map[string][]string{}
	m.services.starting = map[string]bool{}
	m.runServiceConfigs(m.run.configs, false)
	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		return len(m.services.children) == 2
	})

	hook.mu.Lock()
	started := strings.Join(hook.started, ",")
	hook.mu.Unlock()
	if started != "one,two" {
		t.Fatalf("run_start order = %q", started)
	}
	if len(m.run.children) != 0 || m.run.runID != 0 {
		t.Fatalf("Services changed the Run pane sessions: %+v", m.run)
	}
	m.post(func() { m.stopServiceConfigs(m.run.configs) })
	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		return len(m.services.children) == 2 &&
			m.services.children[0].finished && m.services.children[1].finished
	})
}

func TestServicesRunAllSkipsConfigurationsThatAreAlreadyRunning(t *testing.T) {
	hook := &runChainHook{hold: true}
	m := newRunChainModel(t, hook,
		map[string]any{"id": "one", "name": "One", "kind": "shell", "command": "echo"},
		map[string]any{"id": "two", "name": "Two", "kind": "shell", "command": "echo"},
	)
	m.services.root = m.Root()
	m.services.types = []string{"shell"}
	m.services.hidden = map[string][]string{}
	m.services.order = map[string][]string{}
	m.services.starting = map[string]bool{}
	m.services.children = []*runChild{{
		channel: 41, runID: 42, config: m.run.configs[0], service: true,
		root: m.Root(), running: true, done: make(chan error, 1),
	}}

	m.runServiceConfigs(m.run.configs, false)
	waitRunChain(t, 2*time.Second, func() bool {
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == 1
	})

	hook.mu.Lock()
	started := append([]string(nil), hook.started...)
	hook.mu.Unlock()
	if len(started) != 1 || started[0] != "two" {
		t.Fatalf("Run All started %v, want only the stopped configuration", started)
	}
	m.stopServiceConfigs(m.run.configs)
}

func TestServicesRestartAllStopsEveryLiveConfigurationBeforeStartingAgain(t *testing.T) {
	hook := &runChainHook{hold: true}
	m := newRunChainModel(t, hook,
		map[string]any{"id": "one", "name": "One", "kind": "shell", "command": "echo"},
		map[string]any{"id": "two", "name": "Two", "kind": "shell", "command": "echo"},
	)
	m.services.root = m.Root()
	m.services.types = []string{"shell"}
	m.services.hidden = map[string][]string{}
	m.services.order = map[string][]string{}
	m.services.starting = map[string]bool{}

	m.runServiceConfigs(m.run.configs, false)
	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		return len(m.services.children) == 2 &&
			m.runningServiceCountIn(m.run.configs) == 2
	})
	m.runServiceConfigs(m.run.configs, true)
	waitRunChain(t, 3*time.Second, func() bool {
		m.post(func() {})
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == 4 && len(hook.stops) == 2
	})

	hook.mu.Lock()
	started := append([]string(nil), hook.started...)
	stops := append([]uint32(nil), hook.stops...)
	hook.mu.Unlock()
	if strings.Join(started, ",") != "one,two,one,two" {
		t.Fatalf("Restart All start order = %v", started)
	}
	if len(stops) != 2 || stops[0]+stops[1] != 3 ||
		(stops[0] != 1 && stops[0] != 2) {
		t.Fatalf("Restart All stopped runs = %v", stops)
	}
	m.post(func() { m.stopServiceConfigs(m.run.configs) })
}

func TestServicesFolderScopeContainsOnlyThatFoldersConfigurations(t *testing.T) {
	m := newServicesTestModel(t)
	infra := "Infra"
	business := "Business"
	m.run.configs = []openidea.RunConfig{
		{ID: "gateway", Name: "Gateway", Kind: "spring-boot", Folder: &infra},
		{ID: "orders", Name: "Orders", Kind: "spring-boot", Folder: &business},
		{ID: "billing", Name: "Billing", Kind: "spring-boot", Folder: &infra},
		{ID: "loose", Name: "Loose", Kind: "spring-boot"},
	}
	m.services.types = []string{"spring-boot"}
	m.services.order["spring-boot"] = []string{"orders", "billing", "gateway", "loose"}

	group := m.serviceTree()[0]
	var infraNode serviceTreeNode
	for _, child := range group.children {
		if child.nodeKind == "folder" && child.folder == infra {
			infraNode = child
			break
		}
	}
	if infraNode.nodeKind == "" {
		t.Fatalf("Infra folder is missing: %+v", group.children)
	}
	configs := m.serviceScopeConfigs(&infraNode)
	if len(configs) != 2 || configs[0].ID != "billing" || configs[1].ID != "gateway" {
		t.Fatalf("Infra scope = %+v", configs)
	}
}

func TestServicesRunAllSkipsDebugOnlyConfigurations(t *testing.T) {
	hook := &runChainHook{}
	m := newRunChainModel(t, hook,
		map[string]any{"id": "app", "name": "Application", "kind": "shell", "command": "echo"},
		map[string]any{"id": "remote", "name": "Remote", "kind": "remote-jvm"},
	)
	m.services.root = m.Root()
	m.services.types = []string{"shell", "remote-jvm"}
	m.services.hidden = map[string][]string{}
	m.services.order = map[string][]string{}
	m.services.starting = map[string]bool{}

	runnable := serviceRunnableConfigs(m.run.configs)
	if len(runnable) != 1 || runnable[0].ID != "app" {
		t.Fatalf("runnable Services configurations = %+v", runnable)
	}
	m.runServiceConfigs(m.run.configs, false)
	waitRunChain(t, 2*time.Second, func() bool {
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == 1
	})
	hook.mu.Lock()
	started := append([]string(nil), hook.started...)
	hook.mu.Unlock()
	if len(started) != 1 || started[0] != "app" {
		t.Fatalf("Run All started %v, want only app", started)
	}
}

func TestServicesActionsAreRegisteredButHidden(t *testing.T) {
	for _, id := range []string{
		"run.services.runAll", "run.services.restartAll", "run.services.stopAll",
		"run.services.openBrowser", "run.services.addType", "run.services.hideConfig",
	} {
		if _, ok := actionGet(id); !ok {
			t.Errorf("%s is not registered", id)
		}
		if !hiddenActionIDs[id] {
			t.Errorf("%s is not hidden from the palette", id)
		}
	}
}

func TestServicesPanelRendersTreeAndFacts(t *testing.T) {
	m := newServicesTestModel(t)
	m.run.configs = []openidea.RunConfig{{
		ID: "app", Name: "Application", Kind: "application",
		Command: "java", Args: []string{"Main"},
	}}
	m.services.types = []string{"application"}
	m.services.selected = serviceConfigKey("app")
	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1200, 700)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	for _, text := range []string{
		"+ Add Service", "Expand All", "Collapse All", "Application",
		"Command", "java Main", "Not started",
	} {
		if !tt.HasText(text) {
			t.Fatalf("Services is missing %q: %v", text, tt.Texts())
		}
	}
}

func TestServicesHostTabsRenderAndSwitchNativePanels(t *testing.T) {
	m := newServicesTestModel(t)
	m.db = &dbView{loading: true}
	m.ensureHTTPClient()
	m.run.configs = []openidea.RunConfig{{
		ID: "kept", Name: "Kept Application", Kind: "application",
	}}
	m.services.types = []string{"application"}
	m.services.selected = serviceConfigKey("kept")
	m.db.source = "kept-source"
	m.http.detailTab = "headers"

	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1200, 700)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	for _, text := range []string{"Services", "Database", "HTTP Client"} {
		if !tt.HasText(text) {
			t.Fatalf("Services host is missing %q: %v", text, tt.Texts())
		}
	}

	if err := tt.Click("Database"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.services.view != serviceHostDatabase || !tt.HasText("Loading data sources…") {
		t.Fatalf("Database host tab = view %q, texts %v", m.services.view, tt.Texts())
	}

	if err := tt.Click("HTTP Client"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.services.view != serviceHostHTTP || !tt.HasText("No requests yet") {
		t.Fatalf("HTTP Client host tab = view %q, texts %v", m.services.view, tt.Texts())
	}

	if err := tt.Click("Services"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.services.view != serviceHostServices || m.services.selected != serviceConfigKey("kept") ||
		m.db.source != "kept-source" || m.http.detailTab != "headers" {
		t.Fatalf(
			"host tabs did not preserve pane state: view=%q selected=%q source=%q detail=%q",
			m.services.view, m.services.selected, m.db.source, m.http.detailTab,
		)
	}
}

func TestServicesProviderRegistryOrdersRootsAndDispatchesDetail(t *testing.T) {
	m := newServicesTestModel(t)
	later := &serviceProvider{
		id:    "test-later",
		order: 20,
		roots: func(*Model) []serviceTreeNode {
			return []serviceTreeNode{{key: "test:later", label: "Later", nodeKind: "test"}}
		},
	}
	earlier := &serviceProvider{
		id:    "test-earlier",
		order: 5,
		roots: func(*Model) []serviceTreeNode {
			return []serviceTreeNode{{key: "test:earlier", label: "Earlier", nodeKind: "test"}}
		},
		detail: func(_ *Model, c *ui.Context, p *ui.Theme, node serviceTreeNode) {
			ui.Text(c, "Dynamic detail: "+node.label).FontSize(12).TextColor(p.Text)
		},
	}
	t.Cleanup(registerServiceProvider(later))
	t.Cleanup(registerServiceProvider(earlier))

	nodes := m.serviceTree()
	if len(nodes) != 2 || nodes[0].key != "test:earlier" ||
		nodes[1].key != "test:later" ||
		nodes[0].provider != "test-earlier" || nodes[1].provider != "test-later" {
		t.Fatalf("provider roots = %+v", nodes)
	}

	m.selectOnlyServiceKey("test:earlier")
	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1000, 650)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	if !tt.HasText("Dynamic detail: Earlier") {
		t.Fatalf("provider detail was not rendered: %v", tt.Texts())
	}
}

func TestServicesProviderWatchStartsOnceAndDisposesWithRegistration(t *testing.T) {
	m := newServicesTestModel(t)
	starts, stops := 0, 0
	provider := &serviceProvider{
		id:    "test-watch",
		order: 1,
		watch: func(*Model) func() {
			starts++
			return func() { stops++ }
		},
	}
	dispose := registerServiceProvider(provider)
	t.Cleanup(dispose)

	m.ensureServiceProviderWatches()
	m.ensureServiceProviderWatches()
	if starts != 1 || stops != 0 {
		t.Fatalf("watch lifecycle = starts %d, stops %d", starts, stops)
	}
	dispose()
	m.ensureServiceProviderWatches()
	if starts != 1 || stops != 1 {
		t.Fatalf("removed provider lifecycle = starts %d, stops %d", starts, stops)
	}
}

func TestServicesProviderActionsBatchCommonCommands(t *testing.T) {
	m := newServicesTestModel(t)
	var delivered []string
	provider := &serviceProvider{
		id:    "test-actions",
		order: 1,
		roots: func(*Model) []serviceTreeNode {
			return []serviceTreeNode{
				{key: "test:a", label: "A", nodeKind: "test"},
				{key: "test:b", label: "B", nodeKind: "test"},
			}
		},
		actions: func(_ *Model, node serviceTreeNode) []serviceItemAction {
			run := func(id string, disabled bool) serviceItemAction {
				return serviceItemAction{
					id: id, title: id, disabled: disabled,
					run: func() { delivered = append(delivered, id+":"+node.key) },
				}
			}
			return []serviceItemAction{
				run("run", false),
				run("stop", node.key == "test:b"),
				run("restart", false),
				run("debug", false),
			}
		},
	}
	t.Cleanup(registerServiceProvider(provider))

	nodes := m.serviceTree()
	m.services.selected = "test:a"
	m.services.selectedKeys = []string{"test:a", "test:b"}
	actions := m.serviceSelectionActions(nodes)
	if len(actions) != 4 {
		t.Fatalf("common actions = %+v", actions)
	}
	for _, action := range actions {
		if action.disabled {
			t.Fatalf("%s was disabled", action.id)
		}
		if action.run != nil {
			action.run()
		}
	}
	got := strings.Join(delivered, ",")
	for _, want := range []string{
		"run:test:a", "run:test:b", "stop:test:a",
		"restart:test:a", "restart:test:b", "debug:test:a", "debug:test:b",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("batch actions delivered %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "stop:test:b") {
		t.Fatalf("disabled action ran: %q", got)
	}
}

func TestRunServiceActionsExposeBatchRunStopRestartAndDebug(t *testing.T) {
	m := newServicesTestModel(t)
	m.run.configs = []openidea.RunConfig{
		{ID: "a", Name: "A", Kind: "application"},
		{ID: "b", Name: "B", Kind: "application"},
	}
	m.services.types = []string{"application"}
	nodes := m.serviceTree()
	m.services.selected = serviceConfigKey("a")
	m.services.selectedKeys = []string{serviceConfigKey("a"), serviceConfigKey("b")}
	actions := m.serviceSelectionActions(nodes)
	ids := map[string]serviceItemAction{}
	for _, action := range actions {
		ids[action.id] = action
	}
	for _, id := range []string{"run", "stop", "restart", "debug"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("batch action %q missing from %+v", id, actions)
		}
	}
}

func TestRunServiceDebugBatchQueuesBehindTheSingleDebugger(t *testing.T) {
	m := newServicesTestModel(t)
	m.run.configs = []openidea.RunConfig{
		{ID: "a", Name: "A", Kind: "application"},
		{ID: "b", Name: "B", Kind: "application"},
	}
	m.services.types = []string{"application"}
	nodes := m.serviceTree()
	m.services.selected = serviceConfigKey("a")
	m.services.selectedKeys = []string{serviceConfigKey("a"), serviceConfigKey("b")}
	m.debug.lastID = "a"
	m.debug.starting = true

	var debugAction *serviceItemAction
	for _, action := range m.serviceSelectionActions(nodes) {
		if action.id == "debug" {
			action := action
			debugAction = &action
			break
		}
	}
	if debugAction == nil || debugAction.run == nil {
		t.Fatal("batch Debug action is missing")
	}
	debugAction.run()
	debugAction.run()
	if got := strings.Join(m.services.debugQueue, ","); got != "b" {
		t.Fatalf("debug queue = %q", got)
	}
	m.stopServiceConfigs([]openidea.RunConfig{{ID: "b", Name: "B", Kind: "application"}})
	if len(m.services.debugQueue) != 0 {
		t.Fatalf("stopped service stayed queued: %v", m.services.debugQueue)
	}
}

func TestServicesSelectionSupportsCommandRangeAndEmptySelection(t *testing.T) {
	m := newServicesTestModel(t)
	m.run.configs = []openidea.RunConfig{
		{ID: "a", Name: "A", Kind: "application"},
		{ID: "b", Name: "B", Kind: "application"},
		{ID: "c", Name: "C", Kind: "application"},
	}
	m.services.types = []string{"application"}
	m.serviceTree()

	m.selectServiceNode(serviceConfigKey("a"), 0)
	m.selectServiceNode(serviceConfigKey("c"), ui.Shift)
	if got := strings.Join(m.services.selectedKeys, ","); got !=
		serviceConfigKey("a")+","+serviceConfigKey("b")+","+serviceConfigKey("c") {
		t.Fatalf("Shift selection = %s", got)
	}

	m.selectServiceNode(serviceConfigKey("b"), ui.Cmd|ui.Ctrl)
	if got := strings.Join(m.services.selectedKeys, ","); got !=
		serviceConfigKey("a")+","+serviceConfigKey("c") {
		t.Fatalf("Command toggle = %s", got)
	}
	m.selectServiceNode(serviceConfigKey("a"), ui.Cmd|ui.Ctrl)
	m.selectServiceNode(serviceConfigKey("c"), ui.Cmd|ui.Ctrl)
	if len(m.services.selectedKeys) != 0 || m.services.selected != "" {
		t.Fatalf("empty selection = keys %v lead %q", m.services.selectedKeys, m.services.selected)
	}
	m.ensureServiceSelection(m.serviceTree())
	if len(m.services.selectedKeys) != 0 || m.services.selected != "" {
		t.Fatalf("empty selection was replaced: keys %v lead %q", m.services.selectedKeys, m.services.selected)
	}
}

func TestServicesExpandAllLoadsAndCollapsesLazyProviderNodes(t *testing.T) {
	m := newServicesTestModel(t)
	expanded := false
	var calls []string
	provider := &serviceProvider{
		id:    "test-lazy",
		order: 1,
		roots: func(*Model) []serviceTreeNode {
			node := serviceTreeNode{
				key: "test:lazy", label: "Lazy", nodeKind: "test", expandable: true,
			}
			if expanded {
				node.children = []serviceTreeNode{{
					key: "test:child", label: "Child", nodeKind: "test",
				}}
			}
			return []serviceTreeNode{node}
		},
		expand: func(_ *Model, node serviceTreeNode, open bool) {
			expanded = open
			calls = append(calls, fmt.Sprintf("%s:%t", node.key, open))
		},
	}
	t.Cleanup(registerServiceProvider(provider))

	m.expandAllServices()
	if strings.Join(calls, ",") != "test:lazy:true" || !expanded ||
		m.services.collapsed["test:lazy"] {
		t.Fatalf("Expand All = calls %v, expanded %t, collapsed %+v",
			calls, expanded, m.services.collapsed)
	}
	m.collapseAllServices()
	if strings.Join(calls, ",") != "test:lazy:true,test:lazy:false" ||
		expanded || !m.services.collapsed["test:lazy"] {
		t.Fatalf("Collapse All = calls %v, expanded %t, collapsed %+v",
			calls, expanded, m.services.collapsed)
	}
}

func TestServicesTreeRowsExpandFromTheKeyboard(t *testing.T) {
	m := newServicesTestModel(t)
	provider := &serviceProvider{
		id:    "test-keyboard",
		order: 1,
		roots: func(*Model) []serviceTreeNode {
			return []serviceTreeNode{{
				key: "test:keyboard", label: "Keyboard", nodeKind: "test",
				children: []serviceTreeNode{{
					key: "test:keyboard:child", label: "Child", nodeKind: "test",
				}},
			}}
		},
	}
	t.Cleanup(registerServiceProvider(provider))
	m.services.collapsed["test:keyboard"] = true

	tester := ui.NewTester(func(c *ui.Context) {
		m.servicesTree(c, c.Theme())
	}, 420, 260)
	tester.SetDark(true)
	tester.SetScale(1)
	tester.Frame()
	if err := tester.Click("Keyboard"); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeyEnter)
	tester.Frame()

	if m.services.collapsed["test:keyboard"] {
		t.Fatal("Enter did not expand the focused tree row")
	}
	if !tester.HasText("Child") {
		t.Fatalf("expanded tree child is not rendered: %v", tester.Texts())
	}
}

func TestServicesProviderBannerAndAddMenuContribution(t *testing.T) {
	m := newServicesTestModel(t)
	addCalls := 0
	provider := &serviceProvider{
		id:    "test-ui",
		order: 1,
		banner: func(m *Model, c *ui.Context, p *ui.Theme) {
			ui.Text(c, "Provider is loading").FontSize(12).TextColor(p.TextMuted)
		},
		addMenu: func(_ *Model, menu *ui.Menu) {
			addCalls++
			if menu.Item("Test Service…").Chosen() {
				addCalls++
			}
		},
	}
	t.Cleanup(registerServiceProvider(provider))
	m.setServiceProviderError(provider.id, "Provider failed")

	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1000, 650)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	for _, text := range []string{"Provider is loading", "Provider failed"} {
		if !tt.HasText(text) {
			t.Fatalf("provider UI is missing %q: %v", text, tt.Texts())
		}
	}
	if err := tt.Click("+ Add Service"); err != nil {
		t.Fatalf("click Add Service: %v", err)
	}
	tt.Frame()
	if addCalls == 0 {
		t.Fatal("provider Add Service builder was not called")
	}
}
