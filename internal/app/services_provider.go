package app

// Service providers are the native equivalent of OpenIDEA's
// ServiceProviderContribution. A provider owns its roots and may contribute
// actions, a detail pane, Add Service entries, lifecycle work and a live count.

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

const runServiceProviderID = "run"

type serviceProvider struct {
	id    string
	order int

	roots    func(*Model) []serviceTreeNode
	actions  func(*Model, serviceTreeNode) []serviceItemAction
	menu     func(*Model, *ui.Menu, serviceTreeNode)
	detail   func(*Model, *ui.Context, *ui.Theme, serviceTreeNode)
	addMenu  func(*Model, *ui.Menu)
	watch    func(*Model) func()
	expand   func(*Model, serviceTreeNode, bool)
	activate func(*Model, serviceTreeNode)
	selectFn func(*Model, serviceTreeNode)
	banner   func(*Model, *ui.Context, *ui.Theme)
	badge    func(*Model) int
}

type serviceItemAction struct {
	id       string
	title    string
	icon     string
	disabled bool
	run      func()
}

type serviceProviderWatch struct {
	provider *serviceProvider
	dispose  func()
}

var serviceProviderRegistry = struct {
	sync.RWMutex
	providers map[string]*serviceProvider
}{providers: map[string]*serviceProvider{}}

// registerServiceProvider installs a provider and returns its disposer. The
// disposer removes the same registration only, so replacing an id cannot
// unregister the replacement.
func registerServiceProvider(provider *serviceProvider) func() {
	if provider == nil || provider.id == "" {
		return func() {}
	}
	serviceProviderRegistry.Lock()
	serviceProviderRegistry.providers[provider.id] = provider
	serviceProviderRegistry.Unlock()
	return func() {
		serviceProviderRegistry.Lock()
		if serviceProviderRegistry.providers[provider.id] == provider {
			delete(serviceProviderRegistry.providers, provider.id)
		}
		serviceProviderRegistry.Unlock()
	}
}

func registeredServiceProviders() []*serviceProvider {
	serviceProviderRegistry.RLock()
	providers := make([]*serviceProvider, 0, len(serviceProviderRegistry.providers))
	for _, provider := range serviceProviderRegistry.providers {
		providers = append(providers, provider)
	}
	serviceProviderRegistry.RUnlock()
	sort.SliceStable(providers, func(i, j int) bool {
		if providers[i].order != providers[j].order {
			return providers[i].order < providers[j].order
		}
		return providers[i].id < providers[j].id
	})
	return providers
}

func registeredServiceProvider(id string) *serviceProvider {
	serviceProviderRegistry.RLock()
	defer serviceProviderRegistry.RUnlock()
	return serviceProviderRegistry.providers[id]
}

func (m *Model) serviceProviderForNode(node serviceTreeNode) *serviceProvider {
	if provider := registeredServiceProvider(node.provider); provider != nil {
		return provider
	}
	return nil
}

func stampServiceProvider(id string, nodes []serviceTreeNode) {
	for i := range nodes {
		nodes[i].provider = id
		stampServiceProvider(id, nodes[i].children)
	}
}

// ensureServiceProviderWatches starts each visible provider once and stops a
// watcher when its provider is removed or replaced. The Services window calls
// this on every frame, so registration changes take effect without a restart.
func (m *Model) ensureServiceProviderWatches() {
	if !m.services.visible {
		return
	}
	active := make(map[string]*serviceProvider)
	for _, provider := range registeredServiceProviders() {
		if provider.watch == nil {
			continue
		}
		active[provider.id] = provider
		current := m.services.providerWatches[provider.id]
		if current.provider == provider {
			continue
		}
		if current.dispose != nil {
			current.dispose()
		}
		if m.services.providerWatches == nil {
			m.services.providerWatches = map[string]serviceProviderWatch{}
		}
		m.services.providerWatches[provider.id] = serviceProviderWatch{
			provider: provider,
			dispose:  provider.watch(m),
		}
	}
	for id := range m.services.providerWatches {
		if active[id] != nil {
			continue
		}
		m.disposeServiceProviderWatch(id)
	}
}

func (m *Model) disposeServiceProviderWatch(id string) {
	watch, ok := m.services.providerWatches[id]
	if !ok {
		return
	}
	delete(m.services.providerWatches, id)
	if watch.dispose != nil {
		watch.dispose()
	}
}

func (m *Model) disposeServiceProviderWatches() {
	for id := range m.services.providerWatches {
		m.disposeServiceProviderWatch(id)
	}
}

func (m *Model) setServiceProviderError(providerID, message string) {
	if providerID == "" {
		return
	}
	if message == "" {
		if m.services.providerErrors != nil {
			delete(m.services.providerErrors, providerID)
		}
		return
	}
	if m.services.providerErrors == nil {
		m.services.providerErrors = map[string]string{}
	}
	m.services.providerErrors[providerID] = message
	m.status = message
	m.invalidate()
}

func (m *Model) clearServiceProviderError(providerID string) {
	if m.services.providerErrors == nil {
		return
	}
	if _, ok := m.services.providerErrors[providerID]; !ok {
		return
	}
	delete(m.services.providerErrors, providerID)
	m.invalidate()
}

func (m *Model) servicesProviderBanners(c *ui.Context, p *ui.Theme) {
	for _, provider := range registeredServiceProviders() {
		if provider.banner != nil {
			provider.banner(m, c, p)
		}
		if message := m.services.providerErrors[provider.id]; message != "" {
			serviceProviderBanner(c, p, message, p.Danger)
		}
	}
}

func serviceProviderBanner(c *ui.Context, p *ui.Theme, text string, color ui.Color) {
	ui.Row(c).MinHeight(30).Shrink(0).Padding(8, 10).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Background(p.Warning.Mix(p.Surface, 0.9)).Children(func() {
		ui.Text(c, text).FontSize(12).TextColor(color).Grow(1).
			SingleLine().Ellipsis("…")
	})
}

func (m *Model) serviceProviderActions(node serviceTreeNode) []serviceItemAction {
	provider := m.serviceProviderForNode(node)
	if provider == nil || provider.actions == nil {
		return nil
	}
	return provider.actions(m, node)
}

func (m *Model) serviceSelectionActions(nodes []serviceTreeNode) []serviceItemAction {
	selected := m.selectedServiceNodes(nodes)
	if len(selected) == 0 {
		return nil
	}
	groups := make([][]serviceItemAction, 0, len(selected))
	for _, node := range selected {
		groups = append(groups, m.serviceProviderActions(node))
	}
	first := groups[0]
	actions := make([]serviceItemAction, 0, len(first))
	for _, candidate := range first {
		matches := make([]serviceItemAction, 0, len(groups))
		inEveryGroup := true
		for _, group := range groups {
			found := false
			for _, action := range group {
				if action.id == candidate.id {
					matches = append(matches, action)
					found = true
					break
				}
			}
			if !found {
				inEveryGroup = false
				break
			}
		}
		if !inEveryGroup {
			continue
		}
		combined := candidate
		enabled := make([]serviceItemAction, 0, len(matches))
		for _, action := range matches {
			if !action.disabled {
				enabled = append(enabled, action)
			}
		}
		combined.disabled = len(enabled) == 0
		combined.run = func() {
			for _, action := range enabled {
				if action.run != nil {
					action.run()
				}
			}
		}
		actions = append(actions, combined)
	}
	return actions
}

func (m *Model) runServiceProviderMenu(menu *ui.Menu, node serviceTreeNode) {
	m.serviceContextMenu(menu, node)
}

func (m *Model) dockerServiceProviderMenu(menu *ui.Menu, node serviceTreeNode) {
	m.dockerContextMenu(menu, node)
}

func (m *Model) runServiceProviderDetail(c *ui.Context, p *ui.Theme, node serviceTreeNode) {
	switch node.nodeKind {
	case "type", "folder":
		configs := m.serviceScopeConfigs(&node)
		title := node.label
		if node.nodeKind == "type" {
			title = adapter.KindLabel(node.kind)
		}
		ui.Column(c).Grow(1).Padding(22, 18).Gap(7).Children(func() {
			ui.Text(c, title).FontSize(15).Bold().TextColor(p.Text)
			if len(configs) == 0 {
				ui.Text(c, "No configurations of this type are shown.").
					FontSize(12).TextColor(p.TextMuted)
			} else {
				ui.Text(c, fmt.Sprintf("%d configurations", len(configs))).
					FontSize(12).TextColor(p.TextMuted)
			}
		})
	case "config":
		m.serviceConfigDetail(c, p, node)
	}
}

func (m *Model) dockerServiceProviderDetail(c *ui.Context, p *ui.Theme, node serviceTreeNode) {
	m.dockerServiceDetail(c, p, node)
}

func (m *Model) runServiceProviderAddMenu(menu *ui.Menu) {
	project, rest := m.serviceAddKinds()
	item := menu.Submenu("Run Configuration Type", func(submenu *ui.Menu) {
		addCategories := func(categories []serviceCategory) {
			for _, category := range categories {
				category := category
				submenu.Submenu(category.name, func(kindMenu *ui.Menu) {
					for _, kind := range category.kinds {
						kind := kind
						if kindMenu.Item(adapter.KindLabel(kind)).Chosen() {
							m.RunCommandWithArg("run.services.addType", kind)
						}
					}
				})
			}
		}
		addCategories(project)
		if len(project) > 0 && len(rest) > 0 {
			submenu.Separator()
		}
		addCategories(rest)
	})
	item.Disabled(len(project)+len(rest) == 0)
}

func (m *Model) dockerServiceProviderAddMenu(menu *ui.Menu) {
	if menu.Item("Docker Registry…").Chosen() {
		m.editDockerRegistry(nil)
	}
	if menu.Item("Docker Connection…").Chosen() {
		m.editDockerConnection(nil)
	}
	menu.Separator()
	if menu.Item("Docker Settings…").Chosen() {
		m.openDockerSettings()
	}
}

func (m *Model) runServiceProviderWatch() func() {
	if m.run.trusted && !m.run.loaded && !m.run.loading && m.ctx != nil &&
		(m.mgr != nil || m.invokeHook != nil) {
		m.loadRunConfigs()
	}
	return nil
}

func (m *Model) dockerServiceProviderWatch() func() {
	if m.invokeHook == nil && m.mgr == nil {
		return nil
	}
	m.openDockerServices()
	return m.closeDockerServices
}

func (m *Model) runServiceProviderActivate(node serviceTreeNode) {
	if node.nodeKind != "config" {
		return
	}
	cfg := m.serviceConfigByID(node.configID)
	if cfg == nil || serviceDebugOnlyKind(cfg.Kind) {
		return
	}
	child := m.serviceChildForConfig(cfg.ID)
	if child != nil && child.live() {
		return
	}
	m.startServiceConfig(cfg.ID, false)
}

func (m *Model) runServiceProviderBanner(c *ui.Context, p *ui.Theme) {
	switch {
	case m.run.loading:
		serviceProviderBanner(c, p, "Loading run configurations…", p.TextMuted)
	case m.run.trustErr != "":
		serviceProviderBanner(c, p, "Could not read project trust: "+m.run.trustErr, p.Danger)
	case m.debug.err != "" && m.debug.lastID != "" && m.serviceConfigByID(m.debug.lastID) != nil:
		serviceProviderBanner(c, p, "Debugger: "+m.debug.err, p.Danger)
	}
}

func (m *Model) dockerServiceProviderBanner(c *ui.Context, p *ui.Theme) {
	d := &m.services.docker
	if d.loading {
		serviceProviderBanner(c, p, "Loading Docker settings…", p.TextMuted)
		return
	}
	if message := d.errors["settings"]; message != "" {
		ui.Row(c).MinHeight(30).Shrink(0).Padding(6, 10).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
			Background(p.Warning.Mix(p.Surface, 0.9)).Children(func() {
			ui.Text(c, "Could not read Docker settings: "+message).FontSize(12).
				TextColor(p.Danger).Grow(1).SingleLine().Ellipsis("…")
			if ui.Button(c, "Retry").Clicked() {
				delete(d.errors, "settings")
				d.loaded = false
				m.loadDockerServices()
			}
		})
		return
	}
	if op := d.operation; op != nil && op.request != 0 {
		line := ""
		if len(op.output) > 0 {
			line = op.output[len(op.output)-1]
		}
		ui.Row(c).MinHeight(30).Shrink(0).Padding(6, 10).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
			Background(p.Surface).Children(func() {
			ui.Text(c, op.title).FontSize(12).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, line).FontSize(11).TextColor(p.TextMuted).Grow(1).
				SingleLine().Ellipsis("…")
			if op.streaming && ui.Button(c, "Cancel").Clicked() {
				m.cancelDockerOperation()
			}
		})
	}
}

func (m *Model) dockerServiceProviderActions(node serviceTreeNode) []serviceItemAction {
	if node.nodeKind == "docker-registry" {
		return nil
	}
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return nil
	}
	snapshot := m.services.docker.snapshots[connection.ID]
	containers := dockerActionContainers(snapshot, node)
	startIDs := make([]string, 0, len(containers))
	stopIDs := make([]string, 0, len(containers))
	for _, container := range containers {
		if dockerContainerRunning(container) {
			stopIDs = append(stopIDs, container.ID)
		} else {
			startIDs = append(startIDs, container.ID)
		}
	}
	container := dockerActionContainer(node, containers)
	busy := m.dockerOperationBusy()

	start := func() {
		if node.nodeKind == "docker-compose" {
			m.runDockerComposeOp(node, snapshot, "start")
			return
		}
		m.runDockerContainerOpIDs(*connection, startIDs, "start")
	}
	stop := func() {
		if node.nodeKind == "docker-compose" {
			m.runDockerComposeOp(node, snapshot, "stop")
			return
		}
		m.runDockerContainerOpIDs(*connection, stopIDs, "stop")
	}
	restart := func() {
		if node.nodeKind == "docker-compose" {
			m.runDockerComposeOp(node, snapshot, "restart")
			return
		}
		if container != nil {
			m.runDockerContainerOpIDs(*connection, []string{container.ID}, "restart")
		}
	}
	deleteObject := func() {
		switch node.nodeKind {
		case "docker-compose":
			m.confirmDockerComposeDown(node, snapshot)
		case "docker-container", "docker-image", "docker-volume", "docker-network":
			m.confirmDockerRemove(node, strings.TrimPrefix(node.nodeKind, "docker-"), []string{node.dockerID})
		}
	}
	terminal := func() {
		if container != nil {
			m.openDockerTerminal(*connection, *container)
		}
	}
	return []serviceItemAction{
		{
			id: "refresh", title: "Refresh", icon: "run/rerun",
			run: func() { m.refreshDockerSnapshot(*connection) },
		},
		{
			id: "start", title: "Start", icon: "run/run",
			disabled: busy || len(startIDs) == 0,
			run:      start,
		},
		{
			id: "stop", title: "Stop", icon: "run/stop",
			disabled: busy || len(stopIDs) == 0,
			run:      stop,
		},
		{
			id: "restart", title: "Restart", icon: "run/rerun",
			disabled: busy || node.nodeKind != "docker-container" &&
				node.nodeKind != "docker-compose",
			run: restart,
		},
		{
			id: "delete", title: dockerDeleteActionTitle(node),
			disabled: busy || node.nodeKind != "docker-compose" &&
				node.nodeKind != "docker-container" &&
				node.nodeKind != "docker-image" &&
				node.nodeKind != "docker-volume" &&
				node.nodeKind != "docker-network",
			run: deleteObject,
		},
		{
			id: "terminal", title: "Create Terminal", icon: "run/console",
			disabled: container == nil || !dockerContainerRunning(*container),
			run:      terminal,
		},
		{
			id: "pull", title: "Pull Image…", icon: "docker/dockerFile",
			disabled: busy,
			run:      func() { m.openDockerPullDialog(connection.ID, "") },
		},
	}
}

func dockerActionContainers(
	snapshot openidea.DockerSnapshot,
	node serviceTreeNode,
) []openidea.DockerContainer {
	switch node.nodeKind {
	case "docker-compose":
		return dockerComposeContainers(snapshot, node.dockerID)
	case "docker-container":
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			return []openidea.DockerContainer{*container}
		}
	}
	return nil
}

func dockerActionContainer(
	node serviceTreeNode,
	containers []openidea.DockerContainer,
) *openidea.DockerContainer {
	if node.nodeKind != "docker-container" {
		return nil
	}
	for i := range containers {
		if containers[i].ID == node.dockerID {
			container := containers[i]
			return &container
		}
	}
	return nil
}

func dockerDeleteActionTitle(node serviceTreeNode) string {
	switch node.nodeKind {
	case "docker-compose":
		return "Compose Down…"
	case "docker-container", "docker-image", "docker-volume", "docker-network":
		return "Delete…"
	default:
		return "Delete"
	}
}

func markDockerServiceExpandable(nodes []serviceTreeNode) {
	for i := range nodes {
		switch nodes[i].nodeKind {
		case "docker", "docker-group", "docker-compose":
			nodes[i].expandable = true
		}
		markDockerServiceExpandable(nodes[i].children)
	}
}

func init() {
	registerServiceProvider(&serviceProvider{
		id:    runServiceProviderID,
		order: 0,
		roots: func(m *Model) []serviceTreeNode {
			return m.runServiceProviderRoots()
		},
		actions: func(m *Model, node serviceTreeNode) []serviceItemAction {
			return m.runServiceProviderActions(node)
		},
		menu:     (*Model).runServiceProviderMenu,
		detail:   (*Model).runServiceProviderDetail,
		addMenu:  (*Model).runServiceProviderAddMenu,
		watch:    (*Model).runServiceProviderWatch,
		activate: (*Model).runServiceProviderActivate,
		banner:   (*Model).runServiceProviderBanner,
		badge:    (*Model).runServiceRunningCount,
	})
	registerServiceProvider(&serviceProvider{
		id:    dockerProviderID,
		order: 10,
		roots: func(m *Model) []serviceTreeNode {
			nodes := m.dockerServiceNodes()
			markDockerServiceExpandable(nodes)
			return nodes
		},
		actions:  (*Model).dockerServiceProviderActions,
		menu:     (*Model).dockerServiceProviderMenu,
		detail:   (*Model).dockerServiceProviderDetail,
		addMenu:  (*Model).dockerServiceProviderAddMenu,
		watch:    (*Model).dockerServiceProviderWatch,
		activate: (*Model).dockerServiceProviderActivate,
		banner:   (*Model).dockerServiceProviderBanner,
	})
}
