package app

// Services actions follow OpenIDEA's run.services.* contributions. The same
// actions back the Add Service menu, tree context menus, toolbar buttons and
// Find Action, as upstream's action registry does.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

type serviceCategory struct {
	name  string
	kinds []string
}

var serviceCategoryOrder = []string{
	"Java", "Build Tools", "JavaScript", "Python", "Rust", "Go", "C/C++",
	"Shell", "Servers", "Docker",
}

func servicesCommands() []appCommand {
	return append([]appCommand{
		{
			"run.services.runAll", "Run All", "Run", "",
			"services dashboard start every configuration group",
			func(m *Model) { m.runServicesAction("run.services.runAll", nil) },
		},
		{
			"run.services.restartAll", "Restart All", "Run", "",
			"services dashboard restart every configuration group",
			func(m *Model) { m.runServicesAction("run.services.restartAll", nil) },
		},
		{
			"run.services.stopAll", "Stop All", "Run", "",
			"services dashboard stop every configuration group",
			func(m *Model) { m.runServicesAction("run.services.stopAll", nil) },
		},
		{
			"run.services.openBrowser", "Open in Browser", "Run", "",
			"services localhost http port browser",
			func(m *Model) { m.runServicesAction("run.services.openBrowser", nil) },
		},
		{
			"run.services.addType", "Add Service", "Run", "",
			"services dashboard add run configuration type",
			func(m *Model) { m.runServicesAction("run.services.addType", nil) },
		},
		{
			"run.services.hideConfig", "Remove from Services", "Run", "",
			"services dashboard hide remove configuration",
			func(m *Model) { m.runServicesAction("run.services.hideConfig", nil) },
		},
	}, dockerServicesCommands()...)
}

func serviceActionState(a *Action) {
	switch a.ID {
	case "run.services.runAll", "run.services.restartAll", "run.services.stopAll",
		"run.services.openBrowser", "run.services.addType", "run.services.hideConfig":
		a.RunWithArg = func(m *Model, arg any) {
			m.runServicesAction(a.ID, arg)
		}
		a.When = func(m *Model) bool {
			return m.Root() != "" && !m.showingWelcome
		}
	}
}

func (m *Model) runServicesAction(id string, arg any) {
	key, _ := arg.(string)
	switch id {
	case "run.services.runAll":
		m.runServiceSelectionScope("run-all", key)
	case "run.services.restartAll":
		m.runServiceSelectionScope("restart-all", key)
	case "run.services.stopAll":
		m.runServiceSelectionScope("stop-all", key)
	case "run.services.openBrowser":
		m.runServiceSelectionScope("open-browser", key)
	case "run.services.addType":
		if kind, ok := arg.(string); ok {
			m.beginServiceImport(kind)
		} else {
			m.ShowServices()
			m.services.addChooser = &serviceAddChooserState{open: true}
			m.invalidate()
		}
	case "run.services.hideConfig":
		id, ok := arg.(string)
		if !ok {
			return
		}
		for _, cfg := range m.run.configs {
			if cfg.ID == id {
				m.hideServiceConfig(cfg.Kind, cfg.ID)
				return
			}
		}
		if cfg := m.serviceConfigByID(id); cfg != nil {
			m.hideServiceConfig(cfg.Kind, cfg.ID)
		}
	}
}

func (m *Model) runServiceSelectionScope(actionID, nodeKey string) {
	var nodes []serviceTreeNode
	if nodeKey != "" {
		all := m.serviceTree()
		node := m.serviceNodeByKey(all, nodeKey)
		if node == nil {
			return
		}
		nodes = []serviceTreeNode{*node}
	} else {
		nodes = m.selectedServiceNodes(m.serviceTree())
	}
	if len(nodes) == 0 {
		return
	}
	switch actionID {
	case "run-all", "restart-all":
		var configs []openidea.RunConfig
		for _, node := range nodes {
			configs = append(configs, m.serviceScopeConfigs(&node)...)
		}
		m.runServiceConfigs(configs, actionID == "restart-all")
	case "stop-all":
		var configs []openidea.RunConfig
		for _, node := range nodes {
			configs = append(configs, m.serviceScopeConfigs(&node)...)
		}
		m.stopServiceConfigs(configs)
	case "open-browser":
		ports := map[string]string{}
		for _, node := range nodes {
			for url, name := range m.serviceScopePorts(&node) {
				ports[url] = name
			}
		}
		m.openServiceBrowser(ports)
	}
}

func serviceCategoryOf(kind string) string {
	switch kind {
	case "spring-boot", "application", "jar-application", "native",
		"java-file", "kotlin", "kotlinc", "scala-cli":
		return "Java"
	case "maven", "gradle", "cmake", "make", "just":
		return "Build Tools"
	case "npm", "node", "bun", "deno":
		return "JavaScript"
	case "python", "pytest", "django-test":
		return "Python"
	case "cargo", "rustc":
		return "Rust"
	case "go":
		return "Go"
	case "cc", "cmake-application":
		return "C/C++"
	case "shell", "program":
		return "Shell"
	case "tomcat":
		return "Servers"
	case "docker-compose", "docker-dockerfile", "docker-image":
		return "Docker"
	default:
		return "Others"
	}
}

func serviceDebugOnlyKind(kind string) bool {
	switch kind {
	case "remote-jvm", "go-remote", "python-remote", "node-remote", "godot-attach", "php-remote":
		return true
	default:
		return false
	}
}

func serviceCategoryRank(category string) int {
	for i, candidate := range serviceCategoryOrder {
		if candidate == category {
			return i
		}
	}
	return len(serviceCategoryOrder)
}

func categorizeServiceKinds(kinds []string) []serviceCategory {
	byCategory := map[string][]string{}
	for _, kind := range kinds {
		category := serviceCategoryOf(kind)
		byCategory[category] = append(byCategory[category], kind)
	}
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.SliceStable(categories, func(i, j int) bool {
		leftRank := serviceCategoryRank(categories[i])
		rightRank := serviceCategoryRank(categories[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if categories[i] == "Others" {
			return false
		}
		if categories[j] == "Others" {
			return true
		}
		return categories[i] < categories[j]
	})
	out := make([]serviceCategory, 0, len(categories))
	for _, category := range categories {
		kinds := byCategory[category]
		sort.Slice(kinds, func(i, j int) bool {
			return adapter.KindLabel(kinds[i]) < adapter.KindLabel(kinds[j])
		})
		out = append(out, serviceCategory{name: category, kinds: kinds})
	}
	return out
}

func (m *Model) serviceAddKinds() (project, rest []serviceCategory) {
	added := map[string]bool{}
	for _, kind := range m.services.types {
		added[kind] = true
	}
	used := map[string]bool{}
	for _, cfg := range m.run.configs {
		used[cfg.Kind] = true
	}
	known := map[string]bool{}
	for _, typ := range adapter.RunConfigTypes(runConfigPointers(m.run.configs)) {
		known[typ.ID] = true
	}
	var kinds []string
	for kind := range known {
		if kind == "compound" || added[kind] || serviceDockerProviderKind(kind) {
			continue
		}
		kinds = append(kinds, kind)
	}
	categories := categorizeServiceKinds(kinds)
	for _, category := range categories {
		inProject := false
		for _, kind := range category.kinds {
			if used[kind] {
				inProject = true
				break
			}
		}
		if inProject {
			project = append(project, category)
		} else {
			rest = append(rest, category)
		}
	}
	return project, rest
}

func (m *Model) serviceAddMenu(menu *ui.Menu) {
	added := false
	for _, provider := range registeredServiceProviders() {
		if provider.addMenu == nil {
			continue
		}
		if added {
			menu.Separator()
		}
		provider.addMenu(m, menu)
		added = true
	}
}

func (m *Model) beginServiceImport(kind string) {
	configs := m.serviceConfigsForType(kind)
	if len(configs) <= 1 {
		m.addServiceType(kind)
		m.showAllServiceConfigs(kind)
		m.invalidate()
		return
	}
	state := &serviceImportState{open: true, kind: kind}
	for _, cfg := range configs {
		state.choices = append(state.choices, serviceImportChoice{
			id: cfg.ID, name: cfg.Name, picked: !m.serviceConfigHidden(kind, cfg.ID),
		})
	}
	if !m.serviceTypeAdded(kind) {
		for i := range state.choices {
			state.choices[i].picked = true
		}
	}
	m.services.importing = state
	m.invalidate()
}

func (m *Model) finishServiceImport(ok bool) {
	state := m.services.importing
	if state == nil {
		return
	}
	m.services.importing = nil
	if !ok {
		m.invalidate()
		return
	}
	m.addServiceType(state.kind)
	hidden := make([]string, 0)
	for _, choice := range state.choices {
		if !choice.picked {
			hidden = append(hidden, choice.id)
		}
	}
	if len(hidden) == 0 {
		delete(m.services.hidden, state.kind)
	} else {
		m.services.hidden[state.kind] = hidden
	}
	m.saveServicesPrefs()
	m.invalidate()
}

func (m *Model) serviceImportDialog(c *ui.Context, p *ui.Theme) {
	state := m.services.importing
	if state == nil {
		return
	}
	dlg := ui.Modal(c.Key("services:add-service"), &state.open, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			ui.Text(c, "Add Service ("+adapter.KindLabel(state.kind)+")").
				FontSize(14).Bold().TextColor(p.Text)
			ui.Scroll(c).Height(280).Children(func() {
				ui.Column(c).Gap(5).Children(func() {
					for i := range state.choices {
						choice := &state.choices[i]
						ui.Checkbox(c, &choice.picked, choice.name)
					}
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				picked := 0
				for _, choice := range state.choices {
					if choice.picked {
						picked++
					}
				}
				if ui.Button(c, "OK").Disabled(picked == 0).Clicked() {
					m.finishServiceImport(true)
				}
				if ui.Button(c, "Cancel").Clicked() {
					m.finishServiceImport(false)
				}
			})
		})
	})
	_ = dlg
	if !state.open && m.services.importing == state {
		m.finishServiceImport(false)
	}
}

func (m *Model) runServiceProviderActions(node serviceTreeNode) []serviceItemAction {
	configs := m.serviceScopeConfigs(&node)
	ports := m.serviceScopePorts(&node)
	if node.nodeKind != "config" {
		runnable := serviceRunnableConfigs(configs)
		return []serviceItemAction{
			{
				id: "run-all", title: "Run All", icon: "run/run",
				disabled: len(runnable) == 0 || m.serviceScopeAllRunning(runnable),
				run:      func() { m.runServiceConfigs(runnable, false) },
			},
			{
				id: "restart-all", title: "Restart All", icon: "run/rerun",
				disabled: len(runnable) == 0,
				run:      func() { m.runServiceConfigs(runnable, true) },
			},
			{
				id: "stop-all", title: "Stop All", icon: "run/stop",
				disabled: !m.serviceScopeAnyRunning(configs),
				run:      func() { m.stopServiceConfigs(configs) },
			},
			{
				id: "open-browser", title: "Open in Browser", icon: "ui/web",
				disabled: len(ports) == 0,
				run:      func() { m.openServiceBrowser(ports) },
			},
		}
	}

	cfg := m.serviceConfigByID(node.configID)
	if cfg == nil {
		return nil
	}
	child := m.serviceChildForConfig(cfg.ID)
	live := child != nil && child.live()
	runTitle := "Run"
	if live {
		runTitle = "Rerun"
	}
	return []serviceItemAction{
		{
			id: "run", title: runTitle, icon: "run/run",
			disabled: serviceDebugOnlyKind(cfg.Kind),
			run:      func() { m.startServiceConfig(cfg.ID, live) },
		},
		{
			id: "restart", title: "Restart", icon: "run/rerun",
			disabled: serviceDebugOnlyKind(cfg.Kind),
			run:      func() { m.startServiceConfig(cfg.ID, true) },
		},
		{
			id: "debug", title: "Debug", icon: "run/debug",
			run: func() { m.debugServiceConfig(cfg.ID) },
		},
		{
			id: "open-browser", title: "Open in Browser", icon: "ui/web",
			disabled: len(ports) == 0,
			run:      func() { m.openServiceBrowser(ports) },
		},
		{
			id: "stop", title: "Stop", icon: "run/stop",
			disabled: !live && !m.debugServiceActive(cfg.ID),
			run:      func() { m.stopServiceConfigs([]openidea.RunConfig{*cfg}) },
		},
	}
}

func (m *Model) serviceSelectionContextMenu(menu *ui.Menu, node serviceTreeNode) {
	nodes := m.serviceTree()
	selected := m.selectedServiceNodes(nodes)
	if len(selected) == 1 {
		if provider := m.serviceProviderForNode(node); provider != nil && provider.menu != nil {
			provider.menu(m, menu, node)
			return
		}
	}
	for _, action := range m.serviceSelectionActions(nodes) {
		action := action
		if menu.Item(action.title).Disabled(action.disabled).Chosen() && action.run != nil {
			action.run()
		}
	}
}

func (m *Model) serviceContextMenu(menu *ui.Menu, node serviceTreeNode) {
	switch node.nodeKind {
	case "type", "folder":
		configs := m.serviceScopeConfigs(&node)
		running := m.serviceScopeAnyRunning(configs)
		allRunning := m.serviceScopeAllRunning(configs)
		runConfigs := serviceRunnableConfigs(configs)
		if menu.Item("Run All").Disabled(len(runConfigs) == 0 || allRunning).Chosen() {
			m.RunCommandWithArg("run.services.runAll", node.key)
		}
		if menu.Item("Restart All").Disabled(len(runConfigs) == 0).Chosen() {
			m.RunCommandWithArg("run.services.restartAll", node.key)
		}
		if menu.Item("Stop All").Disabled(!running).Chosen() {
			m.RunCommandWithArg("run.services.stopAll", node.key)
		}
		ports := m.serviceScopePorts(&node)
		m.serviceBrowserMenuItem(menu, ports)
		if node.nodeKind == "type" {
			menu.Separator()
			if menu.Item("Add Service").Disabled(len(m.serviceConfigsForType(node.kind)) == 0).Chosen() {
				m.RunCommandWithArg("run.services.addType", node.kind)
			}
			if menu.Item("Remove from Services").Chosen() {
				m.removeServiceType(node.kind)
			}
		}
	case "config":
		cfg := m.serviceConfigByID(node.configID)
		if cfg == nil {
			return
		}
		child := m.serviceChildForConfig(cfg.ID)
		live := child != nil && child.live()
		label := "Run"
		if live {
			label = "Rerun"
		}
		if menu.Item(label).Disabled(serviceDebugOnlyKind(cfg.Kind)).Chosen() {
			m.startServiceConfig(cfg.ID, live)
		}
		if menu.Item("Debug").Chosen() {
			m.debugServiceConfig(cfg.ID)
		}
		ports := m.serviceScopePorts(&node)
		m.serviceBrowserMenuItem(menu, ports)
		if menu.Item("Stop").Disabled(!live && !m.debugServiceActive(cfg.ID)).Chosen() {
			m.stopServiceConfigs([]openidea.RunConfig{*cfg})
		}
		menu.Separator()
		configs := m.serviceVisibleConfigsForType(cfg.Kind)
		at := -1
		for i, candidate := range configs {
			if candidate.ID == cfg.ID {
				at = i
				break
			}
		}
		if menu.Item("Move Up").Disabled(at <= 0).Chosen() {
			m.moveServiceConfig(cfg.Kind, cfg.ID, -1)
		}
		if menu.Item("Move Down").Disabled(at < 0 || at >= len(configs)-1).Chosen() {
			m.moveServiceConfig(cfg.Kind, cfg.ID, 1)
		}
		menu.Separator()
		if menu.Item("Show in Run Tool Window").Disabled(child == nil).Chosen() {
			m.showServiceInRun(child)
		}
		if menu.Item("Show in Debug Tool Window").
			Disabled(!m.debugServiceActive(cfg.ID)).Chosen() {
			m.showServiceInDebug(cfg.ID)
		}
		if menu.Item("Edit Configuration…").Chosen() {
			m.editServiceConfig(cfg.ID)
		}
		if menu.Item("Remove from Services").Chosen() {
			m.RunCommandWithArg("run.services.hideConfig", cfg.ID)
		}
	}
}

func (m *Model) moveServiceConfig(kind, id string, delta int) {
	configs := m.serviceVisibleConfigsForType(kind)
	from := -1
	for i, cfg := range configs {
		if cfg.ID == id {
			from = i
			break
		}
	}
	to := from + delta
	if from < 0 || to < 0 || to >= len(configs) || from == to {
		return
	}
	configs[from], configs[to] = configs[to], configs[from]
	ids := make([]string, len(configs))
	for i := range configs {
		ids[i] = configs[i].ID
	}
	m.services.order[kind] = ids
	m.selectOnlyServiceKey(serviceConfigKey(id))
	m.saveServicesPrefs()
	m.invalidate()
}

func (m *Model) serviceRowDrag(row ui.Element, node serviceTreeNode) {
	if node.nodeKind != "config" {
		return
	}
	dx, dy, pressed := row.Dragged()
	if pressed {
		lx, ly, _ := row.PointerPosition()
		at := ui.Point{X: row.Bounds().X + lx, Y: row.Bounds().Y + ly}
		if m.services.dragService == "" && dx*dx+dy*dy > 16 {
			m.services.dragService = node.configID
			m.services.dragStart = at
		}
		if m.services.dragService == node.configID {
			m.services.dragAt = at
			return
		}
		source := m.serviceConfigByID(m.services.dragService)
		if source != nil && source.Kind == node.kind && node.configID != source.ID {
			m.services.dropService = node.configID
			m.services.dropAfter = at.Y >= row.Bounds().Y+row.Bounds().H/2
			m.invalidate()
		}
		return
	}
	if m.services.dragService != node.configID {
		return
	}
	source := m.services.dragService
	target := m.services.dropService
	after := m.services.dropAfter
	m.services.dragService = ""
	m.services.dragStart = ui.Point{}
	m.services.dragAt = ui.Point{}
	m.services.dropService = ""
	m.services.dropAfter = false
	if target != "" {
		m.reorderServiceConfig(source, target, after)
	}
	m.invalidate()
}

func (m *Model) reorderServiceConfig(sourceID, targetID string, after bool) {
	source := m.serviceConfigByID(sourceID)
	target := m.serviceConfigByID(targetID)
	if source == nil || target == nil || source.Kind != target.Kind || source.ID == target.ID {
		return
	}
	configs := m.serviceVisibleConfigsForType(source.Kind)
	ids := make([]string, 0, len(configs))
	for _, cfg := range configs {
		if cfg.ID != sourceID {
			ids = append(ids, cfg.ID)
		}
	}
	at := -1
	for i, id := range ids {
		if id == targetID {
			at = i
			break
		}
	}
	if at < 0 {
		return
	}
	if after {
		at++
	}
	ids = append(ids, "")
	copy(ids[at+1:], ids[at:])
	ids[at] = sourceID
	m.services.order[source.Kind] = ids
	m.selectOnlyServiceKey(serviceConfigKey(sourceID))
	m.saveServicesPrefs()
}

func (m *Model) serviceBrowserMenuItem(menu *ui.Menu, ports map[string]string) {
	if len(ports) == 0 {
		menu.Item("Open in Browser").Disabled(true)
		return
	}
	if len(ports) == 1 {
		for url := range ports {
			if menu.Item("Open in Browser").Chosen() {
				openReleasePage(url)
			}
		}
		return
	}
	menu.Submenu("Open in Browser", func(submenu *ui.Menu) {
		urls := serviceURLsFromPorts(ports)
		for _, url := range urls {
			url := url
			label := fmt.Sprintf("%s — %s", ports[url], url)
			if submenu.Item(label).Chosen() {
				openReleasePage(url)
			}
		}
	})
}

func (m *Model) serviceBrowserMenu(menu *ui.Menu, ports map[string]string) {
	if len(ports) == 0 {
		menu.Item("No running service exposes an HTTP port").Disabled(true)
		return
	}
	for _, url := range serviceURLsFromPorts(ports) {
		url := url
		label := fmt.Sprintf("%s — %s", ports[url], url)
		if menu.Item(label).Chosen() {
			openReleasePage(url)
		}
	}
}

func serviceScopeLabel(node *serviceTreeNode) string {
	if node == nil {
		return "Services"
	}
	if node.nodeKind == "type" {
		return adapter.KindLabel(node.kind)
	}
	return strings.TrimSpace(node.label)
}
