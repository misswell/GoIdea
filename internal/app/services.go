package app

// OpenIDEA's Services tool window is a Run Dashboard for configuration types
// the user adds explicitly. This file keeps the per-project dashboard state
// and draws the native tree and detail pane. Process ownership lives in
// services_runtime.go and the same native actions power the toolbar and
// context menus from services_actions.go.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

const (
	serviceTypePrefix   = "run:type:"
	serviceFolderPrefix = "run:folder:"
	serviceConfigPrefix = "run:config:"
)

type servicesPane struct {
	visible bool
	view    serviceHostView

	// root scopes every persisted and live choice to the open project.
	root            string
	types           []string
	hidden          map[string][]string
	order           map[string][]string
	collapsed       map[string]bool
	selected        string
	selectedKeys    []string
	selectionAnchor string
	emptySelection  bool

	// children are independent run sessions. They do not share the Run
	// window's primary process, so Services can run several configurations at
	// once and each keeps its own console.
	children          []*runChild
	pendingChildExits map[uint32]*int32
	starting          map[string]bool
	debugQueue        []string
	importing         *serviceImportState
	addChooser        *serviceAddChooserState

	dragService string
	dragStart   ui.Point
	dragAt      ui.Point
	dropService string
	dropAfter   bool

	docker          dockerProviderState
	providerWatches map[string]serviceProviderWatch
	providerErrors  map[string]string
}

type serviceHostView string

const (
	serviceHostServices serviceHostView = "services"
	serviceHostDatabase serviceHostView = "database"
	serviceHostHTTP     serviceHostView = "http-client"
)

type serviceImportState struct {
	open    bool
	kind    string
	choices []serviceImportChoice
}

type serviceImportChoice struct {
	id     string
	name   string
	picked bool
}

type serviceTreeNode struct {
	key        string
	label      string
	detail     string
	nodeKind   string
	kind       string
	folder     string
	configID   string
	live       bool
	debugging  bool
	dim        bool
	expandable bool
	children   []serviceTreeNode
	icon       string
	provider   string

	dockerConn string
	dockerKind string
	dockerID   string
	dockerRef  string
}

// ShowServices opens the dashboard and loads the configurations it needs when
// the project has already been trusted.
func (m *Model) ShowServices() {
	m.ensureServicesProject()
	m.DockShow("services")
	if m.dockWindowShown("services") {
		m.ensureServiceProviderWatches()
	}
}

// CloseServices hides the dashboard without touching its running services.
func (m *Model) CloseServices() {
	m.disposeServiceProviderWatches()
	m.DockHide("services")
}

// ToggleServices is the View menu and command-palette action.
func (m *Model) ToggleServices() {
	if m.dockWindowShown("services") {
		m.CloseServices()
		return
	}
	m.ShowServices()
}

func servicesProjectKey(root string) string {
	if root == "" {
		return ""
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	return filepath.ToSlash(filepath.Clean(root))
}

// resetServices stops sessions owned by the old project and drops their tree.
// It runs as part of resetProjectState, before the new project is installed.
func (m *Model) resetServices() {
	m.disposeServiceProviderWatches()
	for _, child := range append([]*runChild(nil), m.services.children...) {
		if child == nil {
			continue
		}
		m.retireRunChild(child)
		if !child.finished {
			m.stopRunChild(child)
		}
	}
	m.resetDockerServices()
	m.resetHTTPClient()
	m.services = servicesPane{}
}

func (m *Model) ensureServicesProject() {
	root := m.Root()
	if m.services.root == root {
		return
	}
	m.disposeServiceProviderWatches()
	m.services.root = root
	m.resetHTTPClient()
	m.services.selected = ""
	m.services.selectedKeys = nil
	m.services.selectionAnchor = ""
	m.services.emptySelection = false
	m.services.collapsed = map[string]bool{}
	m.services.starting = map[string]bool{}
	m.services.pendingChildExits = map[uint32]*int32{}
	m.services.debugQueue = nil
	m.services.importing = nil
	m.services.addChooser = nil
	m.services.providerWatches = map[string]serviceProviderWatch{}
	m.services.providerErrors = map[string]string{}
	if root == "" {
		m.services.types = nil
		m.services.hidden = map[string][]string{}
		m.services.order = map[string][]string{}
		return
	}

	key := servicesProjectKey(root)
	stored := m.prefs.Services.Projects[key]
	m.services.types = cleanServiceKinds(stored.Types)
	m.services.hidden = cloneServiceLists(stored.Hidden)
	m.services.order = cloneServiceLists(stored.Order)
	m.services.docker.composeOverrides = cloneServiceStringMap(stored.ComposeOverrides)
}

func cleanServiceKinds(types []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(types))
	for _, kind := range types {
		kind = strings.TrimSpace(kind)
		if kind == "" || seen[kind] || serviceDockerProviderKind(kind) {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out
}

func cloneServiceLists(src map[string][]string) map[string][]string {
	out := make(map[string][]string, len(src))
	for key, values := range src {
		seen := map[string]bool{}
		clean := make([]string, 0, len(values))
		for _, value := range values {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			clean = append(clean, value)
		}
		out[key] = clean
	}
	return out
}

func (m *Model) saveServicesPrefs() {
	root := m.Root()
	if root == "" {
		return
	}
	if m.prefs.Services.Projects == nil {
		m.prefs.Services.Projects = map[string]ServiceProjectPrefs{}
	}
	m.prefs.Services.Projects[servicesProjectKey(root)] = ServiceProjectPrefs{
		Types:            append([]string(nil), m.services.types...),
		Hidden:           cloneServiceLists(m.services.hidden),
		Order:            cloneServiceLists(m.services.order),
		ComposeOverrides: cloneServiceStringMap(m.services.docker.composeOverrides),
	}
	m.savePreferences()
}

func cloneServiceStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}

func (m *Model) serviceTypeAdded(kind string) bool {
	for _, candidate := range m.services.types {
		if candidate == kind {
			return true
		}
	}
	return false
}

func (m *Model) serviceConfigHidden(kind, id string) bool {
	for _, candidate := range m.services.hidden[kind] {
		if candidate == id {
			return true
		}
	}
	return false
}

func (m *Model) addServiceType(kind string) {
	if kind == "" || m.serviceTypeAdded(kind) || serviceDockerProviderKind(kind) {
		return
	}
	m.services.types = append(m.services.types, kind)
	m.saveServicesPrefs()
}

func (m *Model) removeServiceType(kind string) {
	out := m.services.types[:0]
	for _, candidate := range m.services.types {
		if candidate != kind {
			out = append(out, candidate)
		}
	}
	m.services.types = out
	typeKey := serviceTypeKey(kind)
	if m.services.selected == typeKey ||
		strings.HasPrefix(m.services.selected, serviceFolderKey(kind, "")) ||
		strings.HasPrefix(m.services.selected, serviceConfigPrefix) {
		m.services.selected = ""
	}
	m.services.selectedKeys = filterServiceSelection(m.services.selectedKeys, func(key string) bool {
		return key != typeKey &&
			!strings.HasPrefix(key, serviceFolderPrefix+kind+":") &&
			!m.serviceKeyBelongsToKind(key, kind)
	})
	m.services.debugQueue = filterServiceSelection(m.services.debugQueue, func(id string) bool {
		cfg := m.serviceConfigByID(id)
		return cfg == nil || cfg.Kind != kind
	})
	m.ensureServiceSelection(m.serviceTree())
	m.saveServicesPrefs()
	m.invalidate()
}

func (m *Model) hideServiceConfig(kind, id string) {
	if kind == "" || id == "" || m.serviceConfigHidden(kind, id) {
		return
	}
	m.services.hidden[kind] = append(m.services.hidden[kind], id)
	m.removeQueuedServiceDebug(id)
	key := serviceConfigKey(id)
	m.services.selectedKeys = filterServiceSelection(m.services.selectedKeys, func(candidate string) bool {
		return candidate != key
	})
	if m.services.selected == key {
		m.services.selected = ""
	}
	m.saveServicesPrefs()
	m.invalidate()
}

func (m *Model) showAllServiceConfigs(kind string) {
	delete(m.services.hidden, kind)
	m.saveServicesPrefs()
	m.invalidate()
}

func serviceTypeKey(kind string) string { return serviceTypePrefix + kind }

func serviceFolderKey(kind, folder string) string {
	return serviceFolderPrefix + kind + ":" + folder
}

func serviceConfigKey(id string) string { return serviceConfigPrefix + id }

func parseServiceFolderKey(key string) (kind, folder string, ok bool) {
	rest, ok := strings.CutPrefix(key, serviceFolderPrefix)
	if !ok {
		return "", "", false
	}
	at := strings.IndexByte(rest, ':')
	if at < 0 {
		return "", "", false
	}
	return rest[:at], rest[at+1:], true
}

func serviceDockerProviderKind(kind string) bool {
	switch kind {
	case "docker-compose", "docker-dockerfile", "docker-image":
		return true
	default:
		return false
	}
}

func (m *Model) serviceConfigsForType(kind string) []openidea.RunConfig {
	var configs []openidea.RunConfig
	seen := map[string]bool{}
	for _, cfg := range m.run.configs {
		if cfg.Kind != kind {
			continue
		}
		configs = append(configs, cfg)
		seen[cfg.ID] = true
	}
	// A configuration deleted after a live run still stays in the tree until
	// that service exits, matching OpenIDEA's removed-configuration session.
	for _, child := range m.services.children {
		if child == nil || child.config.Kind != kind || seen[child.config.ID] || !child.live() {
			continue
		}
		configs = append(configs, child.config)
		seen[child.config.ID] = true
	}
	return configs
}

func orderServiceConfigs(configs []openidea.RunConfig, saved []string) []openidea.RunConfig {
	rank := make(map[string]int, len(saved))
	for i, id := range saved {
		if _, exists := rank[id]; !exists {
			rank[id] = i
		}
	}
	out := append([]openidea.RunConfig(nil), configs...)
	sort.SliceStable(out, func(i, j int) bool {
		left, leftSet := rank[out[i].ID]
		right, rightSet := rank[out[j].ID]
		switch {
		case leftSet && rightSet:
			return left < right
		case leftSet:
			return true
		case rightSet:
			return false
		default:
			return false
		}
	})
	return out
}

func serviceFolder(cfg openidea.RunConfig) string {
	if cfg.Folder == nil {
		return ""
	}
	return strings.TrimSpace(*cfg.Folder)
}

// serviceTree collects the roots contributed by every registered provider in
// provider order. Providers own their internal shape and ordering.
func (m *Model) serviceTree() []serviceTreeNode {
	m.ensureServicesProject()
	var nodes []serviceTreeNode
	for _, provider := range registeredServiceProviders() {
		if provider.roots == nil {
			continue
		}
		roots := provider.roots(m)
		stampServiceProvider(provider.id, roots)
		nodes = append(nodes, roots...)
	}
	m.ensureServiceSelection(nodes)
	return nodes
}

// runServiceProviderRoots is OpenIDEA's runServiceItems for Run Dashboard
// configurations: only added types, folder nodes first, persisted order inside
// each type, and hidden configurations omitted.
func (m *Model) runServiceProviderRoots() []serviceTreeNode {
	nodes := make([]serviceTreeNode, 0, len(m.services.types))
	for _, kind := range m.services.types {
		if serviceDockerProviderKind(kind) {
			continue
		}
		configs := m.serviceConfigsForType(kind)
		visible := make([]openidea.RunConfig, 0, len(configs))
		for _, cfg := range configs {
			if !m.serviceConfigHidden(kind, cfg.ID) {
				visible = append(visible, cfg)
			}
		}
		visible = orderServiceConfigs(visible, m.services.order[kind])

		group := serviceTreeNode{
			key:        serviceTypeKey(kind),
			label:      adapter.KindLabel(kind),
			nodeKind:   "type",
			kind:       kind,
			expandable: true,
		}
		if running := m.runningServiceCountIn(visible); running > 0 {
			group.detail = fmt.Sprintf("%d running", running)
		}
		if m.services.collapsed[group.key] {
			nodes = append(nodes, group)
			continue
		}

		var folders []string
		for _, cfg := range visible {
			folder := serviceFolder(cfg)
			if folder == "" {
				continue
			}
			found := false
			for _, existing := range folders {
				if existing == folder {
					found = true
					break
				}
			}
			if !found {
				folders = append(folders, folder)
			}
		}
		for _, folder := range folders {
			members := make([]openidea.RunConfig, 0)
			for _, cfg := range visible {
				if serviceFolder(cfg) == folder {
					members = append(members, cfg)
				}
			}
			folderNode := serviceTreeNode{
				key:        serviceFolderKey(kind, folder),
				label:      folder,
				nodeKind:   "folder",
				kind:       kind,
				folder:     folder,
				expandable: true,
			}
			if running := m.runningServiceCountIn(members); running > 0 {
				folderNode.detail = fmt.Sprintf("%d running", running)
			}
			if !m.services.collapsed[folderNode.key] {
				for _, cfg := range members {
					folderNode.children = append(folderNode.children, m.serviceConfigNode(cfg))
				}
			}
			group.children = append(group.children, folderNode)
		}
		for _, cfg := range visible {
			if serviceFolder(cfg) == "" {
				group.children = append(group.children, m.serviceConfigNode(cfg))
			}
		}
		nodes = append(nodes, group)
	}
	return nodes
}

func (m *Model) serviceConfigNode(cfg openidea.RunConfig) serviceTreeNode {
	child := m.serviceChildForConfig(cfg.ID)
	node := serviceTreeNode{
		key:      serviceConfigKey(cfg.ID),
		label:    cfg.Name,
		nodeKind: "config",
		kind:     cfg.Kind,
		configID: cfg.ID,
		provider: runServiceProviderID,
	}
	if child != nil {
		node.live = child.live()
		node.detail = serviceExecutionDetail(child)
	}
	node.debugging = m.debugServiceActive(cfg.ID)
	if node.debugging && !node.live {
		node.detail = "Debugging"
		if m.debug.paused {
			node.detail = "Suspended"
		}
	} else if slicesContains(m.services.debugQueue, cfg.ID) {
		node.detail = "Debug queued"
	}
	return node
}

func (m *Model) ensureServiceSelection(nodes []serviceTreeNode) {
	index := make(map[string]*serviceTreeNode)
	walkServiceTree(nodes, func(node *serviceTreeNode) {
		index[node.key] = node
	})
	keys := m.serviceSelectionKeys()
	clean := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		if key == "" || seen[key] || index[key] == nil {
			continue
		}
		seen[key] = true
		clean = append(clean, key)
	}
	if len(clean) == 0 && index[m.services.selected] != nil {
		clean = append(clean, m.services.selected)
	}
	if len(clean) > 0 {
		m.services.selectedKeys = clean
		if !seen[m.services.selected] {
			m.services.selected = clean[len(clean)-1]
		}
		return
	}
	if m.services.emptySelection {
		m.services.selected = ""
		m.services.selectedKeys = nil
		return
	}
	m.services.selectedKeys = nil
	m.services.selected = ""
	var firstConfig string
	walkServiceTree(nodes, func(node *serviceTreeNode) {
		if firstConfig == "" && node.nodeKind == "config" {
			firstConfig = node.key
		}
	})
	if firstConfig != "" {
		m.services.selected = firstConfig
		m.services.selectedKeys = []string{firstConfig}
	} else if len(nodes) > 0 {
		m.services.selected = nodes[0].key
		m.services.selectedKeys = []string{nodes[0].key}
	}
}

func (m *Model) serviceSelectionKeys() []string {
	if len(m.services.selectedKeys) > 0 {
		return append([]string(nil), m.services.selectedKeys...)
	}
	if m.services.selected != "" {
		return []string{m.services.selected}
	}
	return nil
}

func (m *Model) serviceNodeSelected(key string) bool {
	return slicesContains(m.serviceSelectionKeys(), key)
}

func (m *Model) selectedServiceNodes(nodes []serviceTreeNode) []serviceTreeNode {
	selected := make(map[string]bool)
	for _, key := range m.serviceSelectionKeys() {
		selected[key] = true
	}
	descendants := map[string]bool{}
	walkServiceTree(nodes, func(node *serviceTreeNode) {
		if !selected[node.key] {
			return
		}
		for _, child := range node.children {
			walkServiceTree([]serviceTreeNode{child}, func(descendant *serviceTreeNode) {
				descendants[descendant.key] = true
			})
		}
	})
	var out []serviceTreeNode
	walkServiceTree(nodes, func(node *serviceTreeNode) {
		if selected[node.key] && !descendants[node.key] {
			out = append(out, *node)
		}
	})
	return out
}

func walkServiceTree(nodes []serviceTreeNode, visit func(*serviceTreeNode)) {
	for i := range nodes {
		visit(&nodes[i])
		walkServiceTree(nodes[i].children, visit)
	}
}

func (m *Model) serviceVisibleKeys(nodes []serviceTreeNode) []string {
	var keys []string
	var visit func([]serviceTreeNode)
	visit = func(items []serviceTreeNode) {
		for i := range items {
			keys = append(keys, items[i].key)
			if !m.services.collapsed[items[i].key] {
				visit(items[i].children)
			}
		}
	}
	visit(nodes)
	return keys
}

func (m *Model) selectServiceNode(key string, mods ui.Modifiers) {
	nodes := m.serviceTree()
	order := m.serviceVisibleKeys(nodes)
	index := -1
	for i, candidate := range order {
		if candidate == key {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	oldLead := m.services.selected
	keys := m.serviceSelectionKeys()
	switch {
	case mods&ui.Shift != 0 && m.services.selectionAnchor != "":
		anchor := -1
		for i, candidate := range order {
			if candidate == m.services.selectionAnchor {
				anchor = i
				break
			}
		}
		if anchor < 0 {
			anchor = index
		}
		from, to := min(anchor, index), max(anchor, index)
		keys = append([]string(nil), order[from:to+1]...)
		m.services.selected = key
		m.services.emptySelection = false
	case mods&(ui.Cmd|ui.Ctrl) != 0:
		if slicesContains(keys, key) {
			keys = filterServiceSelection(keys, func(candidate string) bool {
				return candidate != key
			})
			if m.services.selected == key {
				if len(keys) == 0 {
					m.services.selected = ""
				} else {
					m.services.selected = keys[len(keys)-1]
				}
			}
			m.services.emptySelection = len(keys) == 0
		} else {
			keys = append(keys, key)
			m.services.selected = key
			m.services.emptySelection = false
		}
		m.services.selectionAnchor = key
	default:
		keys = []string{key}
		m.services.selected = key
		m.services.selectionAnchor = key
		m.services.emptySelection = false
	}
	m.services.selectedKeys = filterServiceSelection(keys, func(string) bool { return true })
	if m.services.selected != oldLead {
		if node := m.serviceNodeByKey(nodes, m.services.selected); node != nil {
			if provider := m.serviceProviderForNode(*node); provider != nil && provider.selectFn != nil {
				provider.selectFn(m, *node)
			}
		}
	}
	m.invalidate()
}

func (m *Model) ensureServiceContextSelection(node serviceTreeNode) {
	if m.serviceNodeSelected(node.key) {
		m.services.selected = node.key
		return
	}
	m.selectOnlyServiceKey(node.key)
}

func (m *Model) selectOnlyServiceKey(key string) {
	m.services.selected = key
	if key == "" {
		m.services.selectedKeys = nil
		m.services.selectionAnchor = ""
		m.services.emptySelection = true
		return
	}
	m.services.selectedKeys = []string{key}
	m.services.selectionAnchor = key
	m.services.emptySelection = false
}

func filterServiceSelection(values []string, keep func(string) bool) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] || !keep(value) {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (m *Model) serviceKeyBelongsToKind(key, kind string) bool {
	id, ok := strings.CutPrefix(key, serviceConfigPrefix)
	if !ok {
		return false
	}
	cfg := m.serviceConfigByID(id)
	return cfg != nil && cfg.Kind == kind
}

func (m *Model) serviceNodeByKey(nodes []serviceTreeNode, key string) *serviceTreeNode {
	for i := range nodes {
		if found := serviceNodeByKeyIn(&nodes[i], key); found != nil {
			return found
		}
	}
	return nil
}

func serviceNodeByKeyIn(node *serviceTreeNode, key string) *serviceTreeNode {
	if node == nil {
		return nil
	}
	if node.key == key {
		return node
	}
	for i := range node.children {
		if found := serviceNodeByKeyIn(&node.children[i], key); found != nil {
			return found
		}
	}
	return nil
}

func (m *Model) serviceConfigByID(id string) *openidea.RunConfig {
	for i := range m.run.configs {
		if m.run.configs[i].ID == id {
			return &m.run.configs[i]
		}
	}
	if child := m.serviceChildForConfig(id); child != nil {
		return &child.config
	}
	return nil
}

func (m *Model) servicesPanel(c *ui.Context, p *ui.Theme) {
	if !m.services.visible {
		return
	}
	m.ensureServicesProject()
	m.ensureServiceProviderWatches()
	m.advanceServiceDebugQueue()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		m.servicesHostTabs(c, p)
		switch m.services.view {
		case serviceHostDatabase:
			m.servicesDatabasePanel(c, p)
		case serviceHostHTTP:
			m.httpClientPanel(c, p)
		default:
			m.servicesToolbar(c, p)
			m.servicesProviderBanners(c, p)
			ui.Row(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Width(360).Shrink(0).MinWidth(240).MinHeight(0).
					BorderWidth(0, 0, 0, 1).BorderColor(p.Border).Children(func() {
					m.servicesTree(c, p)
				})
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					m.servicesDetail(c, p)
				})
			})
		}
	})
}

func (m *Model) servicesToolbar(c *ui.Context, p *ui.Theme) {
	nodes := m.serviceTree()
	actions := m.serviceSelectionActions(nodes)
	selected := m.selectedServiceNodes(nodes)
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		add := ui.Button(c, "+ Add Service")
		add.Menu(func(menu *ui.Menu) { m.serviceAddMenu(menu) })
		if ui.Button(c, "Expand All").Clicked() {
			m.expandAllServices()
		}
		if ui.Button(c, "Collapse All").Clicked() {
			m.collapseAllServices()
		}
		for _, action := range actions {
			action := action
			button := ui.Button(c, action.title)
			button.Disabled(action.disabled)
			if button.Clicked() && action.run != nil {
				action.run()
			}
		}
		ui.Spacer(c)
		if len(selected) > 1 {
			ui.Text(c, fmt.Sprintf("%d selected", len(selected))).FontSize(12).
				TextColor(p.TextMuted).SingleLine()
		}
		if running := m.runningServiceCount(); running > 0 {
			ui.Text(c, fmt.Sprintf("%d running", running)).FontSize(12).
				TextColor(p.Success).SingleLine()
		}
	})
}

func (m *Model) servicesTree(c *ui.Context, p *ui.Theme) {
	nodes := m.serviceTree()
	if len(nodes) == 0 {
		ui.Column(c).Grow(1).Padding(18, 14).Gap(6).Children(func() {
			ui.Text(c, "No services configured").FontSize(13).Bold().TextColor(p.Text)
			ui.Text(c, "Use + Add Service to add a provider entry.").
				FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(4, 4).Gap(1).Children(func() {
			var visit func([]serviceTreeNode, int)
			visit = func(items []serviceTreeNode, depth int) {
				for _, node := range items {
					m.serviceTreeRow(c, p, node, depth)
					if !m.services.collapsed[node.key] {
						visit(node.children, depth+1)
					}
				}
			}
			visit(nodes, 0)
		})
	})
}

func (m *Model) serviceTreeRow(c *ui.Context, p *ui.Theme, node serviceTreeNode, depth int) {
	lead := node.key == m.services.selected
	selected := m.serviceNodeSelected(node.key)
	expandable := len(node.children) > 0 || node.expandable
	expanded := expandable && !m.services.collapsed[node.key]
	row := ui.Row(c.Key("services:"+node.key)).Height(24).Padding(4+float32(depth)*14, 5).
		Gap(5).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Role(ui.RoleTreeItem).Focusable().Label(node.label).Expanded(expanded)
	if lead {
		row.Background(p.Accent.Mix(p.Surface, 0.86))
	} else if selected {
		row.Background(p.SurfaceHover)
	} else if m.services.dragService != "" && node.configID != "" &&
		m.services.dropService == node.configID {
		row.Background(p.Accent.Mix(p.Surface, 0.82))
	} else if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	if row.Clicked() {
		m.selectServiceNode(node.key, row.ClickModifiers())
	}
	if row.DoubleClicked() {
		if expandable {
			open := m.services.collapsed[node.key] || len(node.children) == 0
			m.setServiceNodeExpanded(node, open)
		} else if provider := m.serviceProviderForNode(node); provider != nil && provider.activate != nil {
			provider.activate(m, node)
		}
	}
	if expandable {
		row.OnShortcut(ui.Modifiers(0), ui.KeyEnter, func() {
			open := m.services.collapsed[node.key] || len(node.children) == 0
			m.setServiceNodeExpanded(node, open)
		})
	}
	row.ContextMenu(func(menu *ui.Menu) {
		m.ensureServiceContextSelection(node)
		m.serviceSelectionContextMenu(menu, node)
	})
	m.serviceRowDrag(row, node)
	row.Children(func() {
		if expandable {
			caret := "▸"
			if !m.services.collapsed[node.key] {
				caret = "▾"
			}
			ui.Text(c, caret).FontSize(10).TextColor(p.TextMuted).Width(10)
		}
		oi := oiPaletteOf(p)
		icon := "run/run"
		tint := oi.icon
		switch {
		case node.icon != "":
			icon = node.icon
		case node.nodeKind == "folder":
			icon = "nodes/folder"
		case node.debugging:
			icon = "run/debug"
		case node.live:
			tint = oi.success
		}
		oiIconEl(c, icon, 14, oi, tint)
		color := p.Text
		if node.dim {
			color = p.TextMuted
		}
		ui.Text(c, node.label).FontSize(12).TextColor(color).SingleLine().
			Ellipsis("…").Grow(1)
		if node.detail != "" {
			ui.Text(c, node.detail).FontSize(11).TextColor(p.TextMuted).SingleLine()
		}
	})
}

func (m *Model) servicesDetail(c *ui.Context, p *ui.Theme) {
	if m.services.docker.settingsUI.open {
		m.retainDockerDetailLog("")
		m.dockerSettingsView(c, p)
		return
	}
	node := m.serviceNodeByKey(m.serviceTree(), m.services.selected)
	keepDockerLog := ""
	if node != nil && node.provider == dockerProviderID &&
		node.nodeKind == "docker-container" &&
		m.services.docker.detailTab == dockerDetailLog {
		keepDockerLog = node.key
	}
	m.retainDockerDetailLog(keepDockerLog)
	if node == nil {
		ui.Column(c).Grow(1).Padding(20, 16).Gap(6).Children(func() {
			ui.Text(c, "Select a service").FontSize(13).Bold().TextColor(p.Text)
			ui.Text(c, "Its console and run configuration details appear here.").
				FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	if provider := m.serviceProviderForNode(*node); provider != nil && provider.detail != nil {
		provider.detail(m, c, p, *node)
		return
	}
	ui.Column(c).Grow(1).Padding(22, 18).Gap(7).Children(func() {
		ui.Text(c, node.label).FontSize(15).Bold().TextColor(p.Text)
		if node.detail != "" {
			ui.Text(c, node.detail).FontSize(12).TextColor(p.TextMuted)
		}
	})
}

func (m *Model) serviceConfigDetail(c *ui.Context, p *ui.Theme, node serviceTreeNode) {
	cfg := m.serviceConfigByID(node.configID)
	if cfg == nil {
		ui.Text(c, "This configuration no longer exists.").FontSize(12).TextColor(p.TextMuted)
		return
	}
	if child := m.serviceChildForConfig(cfg.ID); child != nil {
		m.serviceConsole(c, p, child)
		return
	}
	if m.debugServiceActive(cfg.ID) || m.debugServiceHasOutput(cfg.ID) {
		m.serviceDebugConsole(c, p, cfg)
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(10).Children(func() {
			ui.Text(c, cfg.Name).FontSize(15).Bold().TextColor(p.Text).SingleLine().Ellipsis("…")
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "Run").Disabled(serviceDebugOnlyKind(cfg.Kind)).Clicked() {
					m.startServiceConfig(cfg.ID, false)
				}
				if ui.Button(c, "Debug").Clicked() {
					m.debugServiceConfig(cfg.ID)
				}
			})
			serviceFact(c, p, "Configuration", cfg.Name, false)
			serviceFact(c, p, "Type", adapter.KindLabel(cfg.Kind), false)
			if cfg.Module != nil && *cfg.Module != "" {
				serviceFact(c, p, "Module", *cfg.Module, false)
			}
			serviceFact(c, p, "Command", serviceCommandLine(*cfg), true)
			serviceFact(c, p, "Status", "Not started", false)
		})
	})
}

func serviceFact(c *ui.Context, p *ui.Theme, label, value string, monospace bool) {
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.TextMuted).Width(92)
		text := ui.Text(c, value).FontSize(12).TextColor(p.Text)
		if monospace {
			text.Font("monospace")
		}
	})
}

func serviceCommandLine(cfg openidea.RunConfig) string {
	args := adapter.FormatArgs(cfg.Args)
	if args == "" {
		return cfg.Command
	}
	return strings.TrimSpace(cfg.Command + " " + args)
}

func (m *Model) serviceConsole(c *ui.Context, p *ui.Theme, child *runChild) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		m.serviceConsoleHeader(c, p, child)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(10, 12).Children(func() {
				if child.truncated {
					ui.Text(c, "(Output truncated: earlier lines were omitted)").
						FontSize(12).TextColor(p.TextMuted)
				}
				if len(child.output) == 0 {
					ui.Text(c, "No output yet.").FontSize(12).TextColor(p.TextMuted)
				} else {
					ui.Text(c, child.renderText()).FontSize(12).TextColor(p.Text)
				}
			})
		})
	})
}

func (m *Model) serviceConsoleHeader(c *ui.Context, p *ui.Theme, child *runChild) {
	ui.Row(c).Height(32).Shrink(0).Padding(0, 8).Gap(7).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, child.name).FontSize(12).Bold().TextColor(p.Text).
			SingleLine().Ellipsis("…").Grow(1)
		ui.Text(c, serviceExecutionDetail(child)).FontSize(12).
			TextColor(serviceStatusColor(p, child)).SingleLine()
		if child.live() {
			if ui.Button(c, "Rerun").Clicked() {
				m.startServiceConfig(child.config.ID, true)
			}
			if ui.Button(c, "Stop").Clicked() {
				m.stopRunChild(child)
			}
		} else {
			if ui.Button(c, "Run").Clicked() {
				m.startServiceConfig(child.config.ID, false)
			}
		}
		if port := servicePort(child); port != 0 {
			if ui.Button(c, "Open").Clicked() {
				openReleasePage(fmt.Sprintf("http://localhost:%d", port))
			}
		}
	})
}

func serviceStatusColor(p *ui.Theme, child *runChild) ui.Color {
	switch {
	case child.err != "":
		return p.Warning
	case child.live():
		return p.Success
	default:
		return p.TextMuted
	}
}

func (m *Model) serviceDebugConsole(c *ui.Context, p *ui.Theme, cfg *openidea.RunConfig) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).Height(32).Shrink(0).Padding(0, 8).Gap(7).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, cfg.Name).FontSize(12).Bold().TextColor(p.Text).
				SingleLine().Ellipsis("…").Grow(1)
			status := "Debugging"
			if m.debug.paused {
				status = "Suspended"
			}
			if m.debug.ended {
				status = "Finished"
			}
			ui.Text(c, status).FontSize(12).TextColor(p.TextMuted)
			if !m.debug.ended && (m.debug.session != 0 || m.debug.starting) {
				if ui.Button(c, "Stop").Clicked() {
					m.StopDebug()
				}
			} else if ui.Button(c, "Debug").Clicked() {
				m.debugServiceConfig(cfg.ID)
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(10, 12).Children(func() {
				if len(m.debug.output) == 0 {
					ui.Text(c, "No debugger output yet.").FontSize(12).TextColor(p.TextMuted)
				} else {
					ui.Text(c, m.debug.text()).FontSize(12).TextColor(p.Text)
				}
			})
		})
	})
}

func (m *Model) setServiceNodeExpanded(node serviceTreeNode, open bool) {
	changed := m.services.collapsed[node.key] == open
	m.services.collapsed[node.key] = !open
	if changed {
		if provider := m.serviceProviderForNode(node); provider != nil && provider.expand != nil {
			provider.expand(m, node, open)
		}
	}
	m.invalidate()
}

func (m *Model) expandAllServices() {
	requested := map[string]bool{}
	for pass := 0; pass < 8; pass++ {
		changed := false
		walkServiceTree(m.serviceTree(), func(node *serviceTreeNode) {
			if len(node.children) == 0 && !node.expandable {
				return
			}
			if !m.services.collapsed[node.key] && len(node.children) == 0 {
				if !requested[node.key] {
					requested[node.key] = true
					changed = true
					if provider := m.serviceProviderForNode(*node); provider != nil && provider.expand != nil {
						provider.expand(m, *node, true)
					}
				}
				return
			}
			if !m.services.collapsed[node.key] {
				return
			}
			changed = true
			m.setServiceNodeExpanded(*node, true)
		})
		if !changed {
			break
		}
	}
	m.invalidate()
}

func (m *Model) collapseAllServices() {
	walkServiceTree(m.serviceTree(), func(node *serviceTreeNode) {
		if len(node.children) == 0 && !node.expandable {
			return
		}
		m.setServiceNodeExpanded(*node, false)
	})
	m.invalidate()
}
