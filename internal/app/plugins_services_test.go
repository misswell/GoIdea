package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type pluginServicesFakeRender struct {
	view openidea.PluginView
	err  error
}

type pluginServicesFakeLogCall struct {
	pluginID string
	view     string
	key      string
	tab      string
	on       []string
	request  uint32
	channel  uint32
}

type pluginServicesFakeCommand struct {
	pluginID string
	command  string
	arg      string
}

type pluginServicesFakeAPI struct {
	mu sync.Mutex

	snapshot    openidea.PluginsSnapshot
	contextKeys []openidea.PluginContextKey
	renders     map[string][]pluginServicesFakeRender
	details     map[string][]openidea.PluginServiceTab

	notifications chan openidea.Notification
	listCalls     int
	renderCalls   map[string]int
	detailCalls   int
	events        []openidea.PluginViewEvent
	commands      []pluginServicesFakeCommand
	cancels       []uint32
	logs          []pluginServicesFakeLogCall
	commandErr    error
	detailErr     error
}

func newPluginServicesFakeAPI(snapshot openidea.PluginsSnapshot) *pluginServicesFakeAPI {
	return &pluginServicesFakeAPI{
		snapshot:      snapshot,
		notifications: make(chan openidea.Notification, 32),
		renders:       map[string][]pluginServicesFakeRender{},
		renderCalls:   map[string]int{},
		details:       map[string][]openidea.PluginServiceTab{},
	}
}

func (f *pluginServicesFakeAPI) PluginsList(context.Context, string) (openidea.PluginsSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return f.snapshot, nil
}

func (f *pluginServicesFakeAPI) PluginContextKeys(context.Context, string) ([]openidea.PluginContextKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]openidea.PluginContextKey(nil), f.contextKeys...), nil
}

func (f *pluginServicesFakeAPI) PluginViewRender(
	_ context.Context,
	_, pluginID, view string,
) (openidea.PluginView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := pluginID + "\x00" + view
	call := f.renderCalls[key]
	f.renderCalls[key] = call + 1
	replies := f.renders[key]
	if len(replies) == 0 {
		return openidea.PluginView{}, nil
	}
	if call >= len(replies) {
		call = len(replies) - 1
	}
	return replies[call].view, replies[call].err
}

func (f *pluginServicesFakeAPI) PluginViewEvent(
	_ context.Context,
	_, _, _ string,
	event openidea.PluginViewEvent,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return nil
}

func (f *pluginServicesFakeAPI) PluginServiceDetail(
	_ context.Context,
	_, _, _, key string,
) ([]openidea.PluginServiceTab, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detailCalls++
	return append([]openidea.PluginServiceTab(nil), f.details[key]...), f.detailErr
}

func (f *pluginServicesFakeAPI) PluginServiceLog(
	_ context.Context,
	_, pluginID, view, key, tab string,
	request, channel uint32,
	on []string,
) (*int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, pluginServicesFakeLogCall{
		pluginID: pluginID,
		view:     view,
		key:      key,
		tab:      tab,
		on:       append([]string(nil), on...),
		request:  request,
		channel:  channel,
	})
	code := 0
	return &code, nil
}

func (f *pluginServicesFakeAPI) PluginRunCommand(
	_ context.Context,
	_, pluginID, command, arg string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, pluginServicesFakeCommand{
		pluginID: pluginID,
		command:  command,
		arg:      arg,
	})
	return f.commandErr
}

func (f *pluginServicesFakeAPI) PluginCancel(_ context.Context, _ string, request uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, request)
	return nil
}

func (f *pluginServicesFakeAPI) Listen(int) (<-chan openidea.Notification, func()) {
	return f.notifications, func() {}
}

func (f *pluginServicesFakeAPI) setSnapshot(snapshot openidea.PluginsSnapshot) {
	f.mu.Lock()
	f.snapshot = snapshot
	f.mu.Unlock()
}

func (f *pluginServicesFakeAPI) setContextKeys(keys []openidea.PluginContextKey) {
	f.mu.Lock()
	f.contextKeys = append([]openidea.PluginContextKey(nil), keys...)
	f.mu.Unlock()
}

func (f *pluginServicesFakeAPI) counts() (list, detail int, renders map[string]int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	renders = make(map[string]int, len(f.renderCalls))
	for key, count := range f.renderCalls {
		renders[key] = count
	}
	return f.listCalls, f.detailCalls, renders
}

func (f *pluginServicesFakeAPI) recordedEvents() []openidea.PluginViewEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]openidea.PluginViewEvent(nil), f.events...)
}

func (f *pluginServicesFakeAPI) recordedCommands() []pluginServicesFakeCommand {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pluginServicesFakeCommand(nil), f.commands...)
}

func (f *pluginServicesFakeAPI) recordedLogs() []pluginServicesFakeLogCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pluginServicesFakeLogCall(nil), f.logs...)
}

func pluginServicesJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testPluginServicesViewDecl(t *testing.T, id, location, when string) openidea.PluginUIContribution {
	t.Helper()
	decl := map[string]any{"id": id, "title": "Kubernetes", "location": location}
	if when != "" {
		decl["when"] = when
	}
	return openidea.PluginUIContribution{
		PluginID: "p",
		Point:    "views",
		Key:      id,
		Decl:     pluginServicesJSON(t, decl),
	}
}

func testPluginServicesDeleteContributions(t *testing.T) []openidea.PluginUIContribution {
	t.Helper()
	return []openidea.PluginUIContribution{
		{
			PluginID: "p",
			Point:    "commands",
			Key:      "delete",
			Decl: pluginServicesJSON(t, map[string]any{
				"id": "delete", "title": "Delete pod",
			}),
		},
		{
			PluginID: "p",
			Point:    "menus",
			Key:      "viewItem.context/pods/delete",
			Decl: pluginServicesJSON(t, map[string]any{
				"menu": "viewItem.context", "view": "pods", "command": "delete",
				"when": "viewItemKind == pod", "submenu": "Pod",
			}),
		},
	}
}

func pluginServicesSnapshot(t *testing.T, uiItems ...openidea.PluginUIContribution) openidea.PluginsSnapshot {
	t.Helper()
	return openidea.PluginsSnapshot{
		Plugins: []openidea.PluginInfo{{ID: "p", Name: "Pods", Enabled: true, Status: "active"}},
		UI:      uiItems,
	}
}

func pluginServicesTreeView(t *testing.T, label string) openidea.PluginView {
	t.Helper()
	return openidea.PluginView{
		Toolbar: []openidea.PluginViewButton{{
			ID: "delete", Label: "Delete", Enabled: true,
		}},
		Body: struct {
			Kind  string          `json:"kind"`
			Value json.RawMessage `json:"value"`
		}{
			Kind: "tree",
			Value: pluginServicesJSON(t, []openidea.PluginViewTreeItem{
				{Key: "ctx", Label: "minikube", Expandable: true, Expanded: true},
				{
					Key: "ns", Parent: ptr("ctx"), Label: "default",
					Expandable: true, Expanded: true,
				},
				{
					Key: "pod:a", Parent: ptr("ns"), Label: label,
					Detail: ptr("Running"), Icon: ptr("folder"),
				},
			}),
		},
	}
}

type pluginServicesPostQueue chan func()

func newPluginServicesTestRuntime(
	t *testing.T,
	api *pluginServicesFakeAPI,
) (*Model, *pluginServicesRuntime, pluginServicesPostQueue) {
	t.Helper()
	m := newServicesTestModel(t)
	posts := make(pluginServicesPostQueue, 128)
	m.postHook = func(fn func()) { posts <- fn }
	page := &pluginPage{}
	m.plugins = page
	runtime := newPluginServicesRuntime(m, page, api)
	page.openideaServices = runtime
	t.Cleanup(runtime.stop)
	return m, runtime, posts
}

func pumpPluginServices(
	t *testing.T,
	posts pluginServicesPostQueue,
	done func() bool,
) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		if done() {
			return
		}
		select {
		case fn := <-posts:
			fn()
		case <-poll.C:
		case <-deadline:
			t.Fatal("timed out waiting for the plugin Services bridge")
		}
	}
}

func sendPluginServicesEvent(
	t *testing.T,
	api *pluginServicesFakeAPI,
	kind, pluginID, view string,
) {
	t.Helper()
	event := map[string]any{"type": kind}
	if pluginID != "" {
		event["pluginId"] = pluginID
	}
	if view != "" {
		event["view"] = view
	}
	params := pluginServicesJSON(t, map[string]any{"window": Window, "event": event})
	api.notifications <- openidea.Notification{Method: "plugin", Params: params}
}

func waitPluginServicesState(
	t *testing.T,
	runtime *pluginServicesRuntime,
	key pluginServicesViewKey,
	posts pluginServicesPostQueue,
	rootLabel string,
) *pluginServicesViewState {
	t.Helper()
	var state *pluginServicesViewState
	pumpPluginServices(t, posts, func() bool {
		state = runtime.states[key]
		if state == nil {
			return false
		}
		return !state.loading && len(state.roots) > 0 &&
			(rootLabel == "" || strings.Contains(serviceTreeLabels(state.roots), rootLabel))
	})
	return state
}

func serviceTreeLabels(nodes []serviceTreeNode) string {
	var labels []string
	walkServiceTree(nodes, func(node *serviceTreeNode) {
		labels = append(labels, node.label)
	})
	return strings.Join(labels, "\n")
}

func TestOpenIDEAPluginServicesProjectsNativeTreeActionsDetailMenuAndAdd(t *testing.T) {
	snapshot := pluginServicesSnapshot(t, testPluginServicesDeleteContributions(t)...)
	snapshot.UI = append(snapshot.UI,
		testPluginServicesViewDecl(t, "pods", "services", "plugin.p.ready"),
		testPluginServicesViewDecl(t, "sidebar", "sidebar", ""),
	)
	api := newPluginServicesFakeAPI(snapshot)
	api.contextKeys = []openidea.PluginContextKey{{PluginID: "p", Key: "ready", Value: "true"}}

	view := pluginServicesTreeView(t, "nginx")
	api.renders["p\x00pods"] = []pluginServicesFakeRender{{view: view}}
	api.details["pod:a"] = []openidea.PluginServiceTab{
		{
			ID: "info", Label: "Info",
			Body: openidea.PluginServicePane{Kind: "text", Value: pluginServicesJSON(t, "Replica: 3")},
		},
		{
			ID: "events", Label: "Events",
			Body: openidea.PluginServicePane{Kind: "message", Value: pluginServicesJSON(t, "Healthy")},
		},
		{
			ID: "log", Label: "Log",
			Body: openidea.PluginServicePane{
				Kind: "log",
				Value: pluginServicesJSON(t, openidea.PluginServiceLog{
					Program: "kubectl",
					Args:    []string{"logs", "nginx"},
					Toggles: []openidea.PluginServiceLogToggle{{
						ID: "follow", Label: "Follow", Args: []string{"--follow"},
					}},
				}),
			},
		},
	}

	m, runtime, posts := newPluginServicesTestRuntime(t, api)
	runtime.start()
	state := waitPluginServicesState(t, runtime, pluginServicesViewKey{pluginID: "p", viewID: "pods"}, posts, "nginx")
	if state == nil {
		t.Fatal("the Services view was not registered")
	}
	if dialect := registeredServiceProvider("plugin.p.sidebar"); dialect != nil {
		t.Fatal("a non-Services view became a Services provider")
	}

	node := m.serviceNodeByKey(m.serviceTree(), "plugin:p/pods/pod:a")
	if node == nil || node.provider != "plugin.p.pods" || node.label != "nginx" {
		t.Fatalf("projected node = %+v", node)
	}
	m.services.emptySelection = false
	m.services.selected = node.key
	m.services.selectedKeys = []string{node.key}
	m.services.collapsed["plugin:p/pods/ctx"] = false
	m.services.collapsed["plugin:p/pods/ns"] = false

	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1200, 700)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	pumpPluginServices(t, posts, func() bool {
		_, details, _ := api.counts()
		return details == 1 && !state.detail.loading
	})
	tt.Frame()
	for _, text := range []string{
		"+ Add Service", "minikube", "default", "nginx", "Running",
		"Delete", "Info", "Events", "Log", "Replica: 3",
	} {
		if !tt.HasText(text) {
			t.Fatalf("native Services is missing %q: %v", text, tt.Texts())
		}
	}

	if err := tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	pumpPluginServices(t, posts, func() bool {
		events := api.recordedEvents()
		if len(events) < 2 || events[len(events)-1].Kind != "button" {
			return false
		}
		return events[len(events)-2].Kind == "select"
	})
	events := api.recordedEvents()
	first := events[len(events)-2]
	second := events[len(events)-1]
	if first.Kind != "select" || first.Key != "pod:a" ||
		second.Kind != "button" || second.ID != "delete" {
		t.Fatalf("toolbar events = %+v", events)
	}

	if err := tt.RightClick("nginx"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Equal(menu, []string{"Delete pod"}) {
		t.Fatalf("item menu = %v", menu)
	}
	if err := tt.ChooseMenuItem("Delete pod"); err != nil {
		t.Fatal(err)
	}
	pumpPluginServices(t, posts, func() bool { return len(api.recordedCommands()) == 1 })
	command := api.recordedCommands()[0]
	if command.pluginID != "p" || command.command != "delete" || command.arg != "pod:a" {
		t.Fatalf("menu command = %+v", command)
	}

	if err := tt.Click("Events"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Healthy") {
		t.Fatalf("message detail is missing: %v", tt.Texts())
	}

	if err := tt.Click("Log"); err != nil {
		t.Fatal(err)
	}
	pumpPluginServices(t, posts, func() bool {
		logs := api.recordedLogs()
		return len(logs) == 1 && !state.detail.log.loading
	})
	firstLog := api.recordedLogs()[0]
	if firstLog.key != "pod:a" || firstLog.tab != "log" || len(firstLog.on) != 0 {
		t.Fatalf("log request = %+v", firstLog)
	}
	tt.Frame()
	if err := tt.Click("Follow"); err != nil {
		t.Fatal(err)
	}
	pumpPluginServices(t, posts, func() bool { return len(api.recordedLogs()) == 2 })
	secondLog := api.recordedLogs()[1]
	if !slices.Equal(secondLog.on, []string{"follow"}) {
		t.Fatalf("toggled log request = %+v", secondLog)
	}

	addActions := 0
	disposeAdd := registerServiceProvider(&serviceProvider{
		id:    "plugin-services-test-add",
		order: 1,
		addMenu: func(_ *Model, menu *ui.Menu) {
			if menu.Item("Native test service").Chosen() {
				addActions++
			}
		},
	})
	t.Cleanup(disposeAdd)
	tt.Frame()
	if err := tt.Click("+ Add Service"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Native test service") {
		t.Fatalf("native Add Service menu = %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Native test service"); err != nil {
		t.Fatal(err)
	}
	if addActions != 1 {
		t.Fatalf("Add Service action calls = %d", addActions)
	}
}

func TestOpenIDEAPluginServicesRefreshesContextAndUnloads(t *testing.T) {
	snapshot := pluginServicesSnapshot(t, testPluginServicesViewDecl(t, "pods", "services", "plugin.p.ready"))
	api := newPluginServicesFakeAPI(snapshot)
	api.contextKeys = []openidea.PluginContextKey{{PluginID: "p", Key: "ready", Value: "false"}}
	first := pluginServicesTreeView(t, "First")
	second := pluginServicesTreeView(t, "Second")
	api.renders["p\x00pods"] = []pluginServicesFakeRender{{view: first}, {view: second}}

	m, runtime, posts := newPluginServicesTestRuntime(t, api)
	m.services.emptySelection = true
	runtime.start()
	key := pluginServicesViewKey{pluginID: "p", viewID: "pods"}
	pumpPluginServices(t, posts, func() bool {
		lists, _, _ := api.counts()
		return lists == 1 && runtime.catalogLoaded && runtime.states[key] == nil
	})

	api.setContextKeys([]openidea.PluginContextKey{{PluginID: "p", Key: "ready", Value: "true"}})
	sendPluginServicesEvent(t, api, "contextChanged", "", "")
	waitPluginServicesState(t, runtime, key, posts, "First")

	sendPluginServicesEvent(t, api, "viewChanged", "p", "pods")
	waitPluginServicesState(t, runtime, key, posts, "Second")
	_, _, renders := api.counts()
	if renders["p\x00pods"] != 2 {
		t.Fatalf("view refresh calls = %+v", renders)
	}

	api.setSnapshot(pluginServicesSnapshot(t))
	sendPluginServicesEvent(t, api, "commandsChanged", "", "")
	pumpPluginServices(t, posts, func() bool {
		lists, _, _ := api.counts()
		return lists == 2 && runtime.catalogLoaded && runtime.states[key] == nil &&
			registeredServiceProvider(key.providerID()) == nil
	})
}

func TestOpenIDEAPluginServicesShowsErrorsAndRecovers(t *testing.T) {
	items := testPluginServicesDeleteContributions(t)
	items = append(items, testPluginServicesViewDecl(t, "pods", "services", ""))
	snapshot := pluginServicesSnapshot(t, items...)
	api := newPluginServicesFakeAPI(snapshot)
	api.renders["p\x00pods"] = []pluginServicesFakeRender{
		{err: errors.New("cluster unavailable")},
		{view: pluginServicesTreeView(t, "Recovered")},
	}

	m, runtime, posts := newPluginServicesTestRuntime(t, api)
	runtime.start()
	key := pluginServicesViewKey{pluginID: "p", viewID: "pods"}
	var state *pluginServicesViewState
	pumpPluginServices(t, posts, func() bool {
		state = runtime.states[key]
		return state != nil && !state.loading && state.err != ""
	})

	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1000, 650)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	if !tt.HasText("Kubernetes: cluster unavailable") {
		t.Fatalf("render error is missing: %v", tt.Texts())
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	waitPluginServicesState(t, runtime, key, posts, "Recovered")
	tt.Frame()
	if !tt.HasText("Recovered") || tt.HasText("Kubernetes: cluster unavailable") {
		t.Fatalf("retry did not replace the error: %v", tt.Texts())
	}

	m.services.emptySelection = false
	m.services.selected = "plugin:p/pods/pod:a"
	m.services.selectedKeys = []string{m.services.selected}
	m.services.collapsed["plugin:p/pods/ctx"] = false
	m.services.collapsed["plugin:p/pods/ns"] = false
	api.mu.Lock()
	api.detailErr = errors.New("detail unavailable")
	api.mu.Unlock()
	tt.Frame()
	pumpPluginServices(t, posts, func() bool {
		return state.detail != nil && !state.detail.loading && state.detail.err != ""
	})
	tt.Frame()
	if !tt.HasText("detail unavailable") {
		t.Fatalf("detail error is missing: %v", tt.Texts())
	}
	api.mu.Lock()
	api.detailErr = nil
	api.mu.Unlock()
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	pumpPluginServices(t, posts, func() bool {
		return state.detail != nil && !state.detail.loading && state.detail.err == ""
	})
	tt.Frame()
	if !tt.HasText("Nothing to show") {
		t.Fatalf("detail retry did not recover: %v", tt.Texts())
	}

	api.mu.Lock()
	api.commandErr = errors.New("permission denied")
	api.mu.Unlock()
	tt.Frame()
	if err := tt.RightClick("Recovered"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete pod"); err != nil {
		t.Fatal(err)
	}
	pumpPluginServices(t, posts, func() bool { return strings.Contains(state.commandErr, "permission denied") })
	tt.Frame()
	if !tt.HasText("Kubernetes: Plugin command delete failed: permission denied") {
		t.Fatalf("command error is missing: %v", tt.Texts())
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Kubernetes: Plugin command delete failed: permission denied") {
		t.Fatalf("dismissed command error is still visible: %v", tt.Texts())
	}
}

func TestPluginServicesWhenMatchesOpenIDEAGrammar(t *testing.T) {
	context := map[string]any{
		"projectOpen":                 true,
		"workspaceOpen":               false,
		"activeToolWindow":            "services",
		"plugin.p.ready":              true,
		"plugin.p.replicas":           json.Number("3"),
		"plugin.p.mode":               "watch",
		"toolWindow.services.visible": true,
	}
	cases := []struct {
		expression string
		want       bool
	}{
		{"", true},
		{"projectOpen && !workspaceOpen", true},
		{"activeToolWindow == 'services'", true},
		{"activeToolWindow == debug", false},
		{"plugin.p.ready && (plugin.p.replicas == 3 || plugin.p.replicas != 3)", true},
		{"plugin.p.mode != watch", false},
		{"toolWindow.services.visible && !missing", true},
		{"(projectOpen || missing) && activeToolWindow == services", true},
		{"plugin.p.mode ==", false},
	}
	for _, test := range cases {
		if got := pluginServicesWhen(test.expression, context); got != test.want {
			t.Errorf("pluginServicesWhen(%q) = %t, want %t", test.expression, got, test.want)
		}
	}
}

type pluginContentFakeController struct {
	active bool
	finds  int
	ids    []string
}

func (f *pluginContentFakeController) Find() {
	f.finds++
}

func (f *pluginContentFakeController) RunAction(id string) {
	f.ids = append(f.ids, id)
}

func (f *pluginContentFakeController) Active() bool {
	return f.active
}

func TestOpenIDEAPluginServicesLogActionsRouteByTargetAndActivePane(t *testing.T) {
	items := []openidea.PluginUIContribution{
		testPluginServicesViewDecl(t, "pods", "services", ""),
	}
	api := newPluginServicesFakeAPI(pluginServicesSnapshot(t, items...))
	api.renders["p\x00pods"] = []pluginServicesFakeRender{{view: pluginServicesTreeView(t, "nginx")}}
	api.details["pod:a"] = []openidea.PluginServiceTab{{
		ID: "log", Label: "Log",
		Body: openidea.PluginServicePane{
			Kind: "log",
			Value: pluginServicesJSON(t, openidea.PluginServiceLog{
				Program: "kubectl",
				Args:    []string{"logs", "nginx"},
				Toggles: []openidea.PluginServiceLogToggle{{
					ID: "follow", Label: "Follow", Args: []string{"--follow"},
				}},
			}),
		},
	}}

	m, runtime, posts := newPluginServicesTestRuntime(t, api)
	runtime.start()
	state := waitPluginServicesState(t, runtime, pluginServicesViewKey{pluginID: "p", viewID: "pods"}, posts, "nginx")
	m.services.emptySelection = false
	m.services.selected = "plugin:p/pods/pod:a"
	m.services.selectedKeys = []string{m.services.selected}
	m.services.collapsed["plugin:p/pods/ctx"] = false
	m.services.collapsed["plugin:p/pods/ns"] = false

	tt := ui.NewTester(func(c *ui.Context) { m.servicesPanel(c, c.Theme()) }, 1000, 650)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()
	pumpPluginServices(t, posts, func() bool {
		return state.detail != nil && !state.detail.loading
	})
	tt.Frame()
	pumpPluginServices(t, posts, func() bool {
		return state.detail.log != nil && !state.detail.log.loading
	})
	tt.Frame()
	logState := state.detail.log
	if logState == nil || !tt.HasText("Find") || !tt.HasText("Soft-wrap") {
		t.Fatalf("log controls are missing: %v", tt.Texts())
	}
	logState.append(openidea.PluginOutputLine{Text: "starting nginx"})
	logState.append(openidea.PluginOutputLine{Text: "nginx ready"})
	if matches := pluginLogFindCount(logState.lines, pluginLogFindExpression("NGINX")); matches != 2 {
		t.Fatalf("native log find matches = %d", matches)
	}
	target := logState.targetKey()

	if !m.RunActionWithArg(pluginServicesLogFindActionID, target) || !logState.findOpen {
		t.Fatal("plugins.log.find did not open the native find bar")
	}
	logState.findText = "NGINX"
	tt.Frame()
	if !tt.HasText("1 of 2") {
		t.Fatalf("native log find bar is missing its match position: %v", tt.Texts())
	}
	if !m.RunActionWithArg(pluginServicesLogWrapActionID, target) || logState.wrap {
		t.Fatal("plugins.log.wrap did not toggle soft wrapping")
	}
	tt.Frame()
	if !m.RunActionWithArg(pluginServicesLogToggleArgumentActionID, map[string]any{
		"target": target, "id": "follow", "checked": true,
	}) {
		t.Fatal("plugins.log.toggleArgument was unavailable")
	}
	pumpPluginServices(t, posts, func() bool { return len(api.recordedLogs()) == 2 })
	if got := api.recordedLogs()[1].on; !slices.Equal(got, []string{"follow"}) {
		t.Fatalf("toggle action arguments = %v", got)
	}
	logState = state.detail.log
	if logState == nil || logState.wrap || !logState.findOpen || logState.findText != "NGINX" {
		t.Fatalf("restart lost native log controls: %+v", logState)
	}
	if !m.RunActionWithArg(pluginServicesLogClearActionID, target) ||
		len(logState.lines) != 0 || logState.findMatch != 0 {
		t.Fatal("plugins.log.clear did not clear output")
	}

	if !m.RunActionWithArg(pluginServicesLogFindActionID, nil) || !logState.findOpen {
		t.Fatal("the active plugin log pane did not receive plugins.log.find")
	}
	if !m.RunActionWithArg(pluginServicesLogClearActionID, "missing-target") {
		t.Fatal("a registered no-op action did not run")
	}
	if m.status != "The plugin Services log is no longer available." {
		t.Fatalf("missing-target status = %q", m.status)
	}
}

func TestOpenIDEAPluginServicesContentActionsUseRegisteredController(t *testing.T) {
	m := newServicesTestModel(t)
	for _, id := range []string{
		pluginServicesLogFindActionID,
		pluginServicesLogClearActionID,
		pluginServicesLogWrapActionID,
		pluginServicesLogToggleArgumentActionID,
		pluginServicesContentFindActionID,
		pluginServicesContentActionID,
	} {
		action, ok := actionGet(id)
		if !ok || action.Category != "Plugins" || !hiddenActionIDs[id] {
			t.Fatalf("plugin pane action %q = %#v", id, action)
		}
	}

	controller := &pluginContentFakeController{}
	dispose := registerPluginContentController("content:test", controller)
	t.Cleanup(dispose)
	if !m.RunActionWithArg(pluginServicesContentFindActionID, "content:test") || controller.finds != 1 {
		t.Fatal("plugins.content.find did not reach the target native controller")
	}
	if !m.RunActionWithArg(pluginServicesContentActionID, map[string]any{
		"target": "content:test", "id": "save",
	}) {
		t.Fatal("plugins.content.action was unavailable")
	}
	if !slices.Equal(controller.ids, []string{"save"}) {
		t.Fatalf("content actions = %v", controller.ids)
	}
	if !m.RunActionWithArg(pluginServicesContentFindActionID, nil) {
		t.Fatal("the content find action should remain registered without an active surface")
	}
	if m.status != "No plugin content editor is active." {
		t.Fatalf("inactive content status = %q", m.status)
	}
	controller.active = true
	if !m.RunActionWithArg(pluginServicesContentFindActionID, nil) || controller.finds != 2 {
		t.Fatal("plugins.content.find did not reach the active native controller")
	}

	dispose()
	if !m.RunActionWithArg(pluginServicesContentFindActionID, nil) {
		t.Fatal("the content find action should remain registered without a surface")
	}
	if m.status != "No plugin content editor is active." {
		t.Fatalf("missing content surface status = %q", m.status)
	}
}
