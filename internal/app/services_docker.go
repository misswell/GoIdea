package app

// OpenIDEA's Services window hosts more than run configurations. The Docker
// provider uses the same openidea-server commands as upstream's desktop shell:
// connections and snapshots come from Docker settings, while container and
// Compose operations stay in the backend.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	dockerProviderID  = "docker"
	dockerNodePrefix  = "docker:"
	dockerMaxLogLines = 5000
	dockerMaxBytes    = 1 << 20

	dockerDetailDashboard = "dashboard"
	dockerDetailLog       = "log"
	dockerDetailProcesses = "processes"
	dockerDetailLayers    = "layers"
	dockerDetailInspect   = "inspect"
)

type dockerProviderState struct {
	settingsUI       dockerSettingsUI
	detailTab        string
	epoch            uint64
	loaded           bool
	loading          bool
	watching         bool
	backendLost      bool
	settings         openidea.DockerSettings
	snapshots        map[string]openidea.DockerSnapshot
	errors           map[string]string
	refreshing       map[string]bool
	refresh          map[string]bool
	refreshGen       map[string]uint32
	operation        *dockerOperation
	logs             map[string]*dockerLogStream
	events           map[string]*dockerEventStream
	inspect          map[string]*dockerInspectState
	stats            map[string]*dockerStatsState
	processes        map[string]*dockerProcessState
	history          map[string]*dockerHistoryState
	logins           map[string]*dockerRegistryLoginState
	logEditors       map[string]*Tab
	composeOverrides map[string]string
	dialog           *dockerDialog
	pollCancel       context.CancelFunc
	settingsCancel   context.CancelFunc
}

type dockerOperation struct {
	request      uint32
	channel      uint32
	connectionID string
	title        string
	output       []string
	err          string
	cancel       context.CancelFunc
	streaming    bool
	stopped      bool
}

type dockerLogStream struct {
	request    uint32
	channel    uint32
	key        string
	connection string
	container  string
	sourceLive bool
	lines      []openidea.DockerLogLine
	loading    bool
	follow     bool
	timestamps bool
	err        string
	cancel     context.CancelFunc
	stopped    bool
	editor     *Tab
}

type dockerEventStream struct {
	request uint32
	channel uint32
	events  []openidea.DockerEvent
	err     string
	cancel  context.CancelFunc
}

type dockerInspectState struct {
	loading bool
	text    string
	err     string
}

type dockerStatsState struct {
	loading bool
	stats   openidea.DockerContainerStats
	loaded  bool
	err     string
}

type dockerProcessState struct {
	loading bool
	list    openidea.DockerProcessList
	loaded  bool
	err     string
}

type dockerHistoryState struct {
	loading bool
	layers  []openidea.DockerImageLayer
	loaded  bool
	err     string
}

type dockerRegistryLoginState struct {
	connectionID       string
	connectionSelected bool
	loading            bool
	answer             string
	err                string
}

type dockerDialog struct {
	kind         string
	connectionID string
	object       string
	ids          []string
	title        string
	message      string
	value        string
	reference    string
	force        bool
	busy         bool
	err          string
	button       string
	action       func()
}

func (m *Model) dockerContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Model) dockerEpoch() uint64 {
	d := &m.services.docker
	if d.epoch == 0 {
		d.epoch = uint64(m.nextRequest())
	}
	return d.epoch
}

func (m *Model) invalidateDockerEpoch() uint64 {
	d := &m.services.docker
	d.epoch = uint64(m.nextRequest())
	return d.epoch
}

func (m *Model) openDockerServices() {
	d := &m.services.docker
	m.dockerEpoch()
	d.watching = true
	m.loadDockerServices()
	m.startDockerPoll()
	if d.loaded {
		for _, connection := range d.settings.Connections {
			m.refreshDockerSnapshot(connection)
		}
	}
}

func (m *Model) closeDockerServices() {
	d := &m.services.docker
	d.watching = false
	m.cancelDockerOperation()
	if d.settingsCancel != nil {
		d.settingsCancel()
		d.settingsCancel = nil
	}
	d.loading = false
	if d.pollCancel != nil {
		d.pollCancel()
		d.pollCancel = nil
	}
	for id, stream := range d.events {
		delete(d.events, id)
		m.stopDockerEventStream(stream)
	}
	for key, stream := range d.logs {
		if stream != nil && stream.editor == nil {
			delete(d.logs, key)
			m.stopDockerLogStream(stream)
		}
	}
}

func (m *Model) resetDockerServices() {
	d := &m.services.docker
	m.invalidateDockerEpoch()
	m.cancelDockerOperation()
	m.closeDockerServices()
	for key, stream := range d.logs {
		if stream != nil {
			m.stopDockerLogStream(stream)
			delete(d.logs, key)
		}
	}
	d.dialog = nil
	d.settingsUI = dockerSettingsUI{}
	d.settingsCancel = nil
	d.backendLost = false
}

func (m *Model) startDockerPoll() {
	d := &m.services.docker
	if !d.watching || d.pollCancel != nil {
		return
	}
	epoch := m.dockerEpoch()
	ctx, cancel := context.WithCancel(m.dockerContext())
	d.pollCancel = cancel
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.post(func() {
					if d.epoch != epoch || !d.watching {
						return
					}
					for _, connection := range d.settings.Connections {
						snapshot, hasSnapshot := d.snapshots[connection.ID]
						_, hasEvents := d.events[connection.ID]
						if !hasSnapshot || snapshot.Availability.Kind != "ready" || !hasEvents {
							m.refreshDockerSnapshot(connection)
						}
					}
				})
			}
		}
	}()
}

func (m *Model) loadDockerServices() {
	d := &m.services.docker
	if d.loaded || d.loading || (m.invokeHook == nil && m.mgr == nil) {
		return
	}
	epoch := m.dockerEpoch()
	ctx, cancel := context.WithTimeout(m.dockerContext(), 30*time.Second)
	d.settingsCancel = cancel
	d.loading = true
	m.invalidate()
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "docker_settings", nil)
		var settings openidea.DockerSettings
		if err == nil {
			err = json.Unmarshal(raw, &settings)
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			d.settingsCancel = nil
			d.loading = false
			d.loaded = true
			if err != nil {
				if d.errors == nil {
					d.errors = map[string]string{}
				}
				d.errors["settings"] = err.Error()
				m.invalidate()
				return
			}
			d.settings = settings
			d.backendLost = false
			delete(d.errors, "settings")
			m.pruneDockerState(settings)
			for _, connection := range settings.Connections {
				m.refreshDockerSnapshot(connection)
			}
			m.startDockerPoll()
			m.invalidate()
		})
	}()
}

func (m *Model) refreshDockerServices() {
	if !m.services.docker.loaded {
		m.services.docker.loaded = false
		m.loadDockerServices()
		return
	}
	for _, connection := range m.services.docker.settings.Connections {
		m.refreshDockerSnapshot(connection)
	}
	m.invalidate()
}

func (m *Model) refreshDockerSnapshot(connection openidea.DockerConnection) {
	d := &m.services.docker
	if connection.ID == "" {
		return
	}
	current := m.dockerConnection(connection.ID)
	if current == nil {
		return
	}
	connection = *current
	if d.refreshing == nil {
		d.refreshing = map[string]bool{}
	}
	if d.refresh == nil {
		d.refresh = map[string]bool{}
	}
	if d.refreshGen == nil {
		d.refreshGen = map[string]uint32{}
	}
	if d.refreshing[connection.ID] {
		d.refresh[connection.ID] = true
		return
	}
	if d.snapshots == nil {
		d.snapshots = map[string]openidea.DockerSnapshot{}
	}
	if d.errors == nil {
		d.errors = map[string]string{}
	}
	epoch := m.dockerEpoch()
	d.refreshing[connection.ID] = true
	generation := m.nextRequest()
	d.refreshGen[connection.ID] = generation
	delete(d.errors, connection.ID)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "docker_snapshot", map[string]any{"endpoint": connection.Endpoint})
		var snapshot openidea.DockerSnapshot
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.refreshGen[connection.ID] != generation {
				m.invalidate()
				return
			}
			delete(d.refreshing, connection.ID)
			current := m.dockerConnection(connection.ID)
			if current == nil || current.Endpoint != connection.Endpoint {
				delete(d.refresh, connection.ID)
				m.invalidate()
				return
			}
			if err != nil {
				d.errors[connection.ID] = err.Error()
			} else {
				d.snapshots[connection.ID] = snapshot
				m.pruneDockerSnapshotCaches(connection.ID, snapshot)
				if snapshot.Availability.Kind == "ready" {
					m.startDockerEvents(connection)
					if strings.HasPrefix(m.services.providerErrors[dockerProviderID],
						"Docker events for "+connection.Name+" stopped:") {
						m.clearServiceProviderError(dockerProviderID)
					}
				} else if stream := d.events[connection.ID]; stream != nil {
					delete(d.events, connection.ID)
					m.stopDockerEventStream(stream)
				}
			}
			refreshAgain := d.refresh[connection.ID]
			delete(d.refresh, connection.ID)
			m.invalidate()
			if refreshAgain {
				m.refreshDockerSnapshot(connection)
			}
		})
	}()
}

func (m *Model) dockerOperationBusy() bool {
	op := m.services.docker.operation
	return op != nil && op.request != 0
}

func (m *Model) dockerBackendLost(err error) {
	d := &m.services.docker
	d.backendLost = true
	m.invalidateDockerEpoch()
	if d.settingsUI.saving {
		d.settingsUI.saving = false
		if err != nil {
			d.settingsUI.err = "The backend disconnected before Docker settings were saved: " + err.Error()
		}
	}
	if d.settingsUI.sshLoading {
		d.settingsUI.sshRequest = ""
		d.settingsUI.sshLoading = false
		d.settingsUI.sshErr = "The backend disconnected while loading SSH configurations."
	}
	if d.settingsCancel != nil {
		d.settingsCancel()
		d.settingsCancel = nil
	}
	d.loading = false
	d.loaded = false
	if d.pollCancel != nil {
		d.pollCancel()
		d.pollCancel = nil
	}
	m.cancelDockerOperation()
	for id, stream := range d.events {
		delete(d.events, id)
		m.stopDockerEventStream(stream)
	}
	for key, stream := range d.logs {
		m.stopDockerLogStream(stream)
		delete(d.logs, key)
	}
	d.snapshots = map[string]openidea.DockerSnapshot{}
	d.errors = map[string]string{}
	d.refreshing = map[string]bool{}
	d.refresh = map[string]bool{}
	d.refreshGen = map[string]uint32{}
	d.inspect = map[string]*dockerInspectState{}
	d.stats = map[string]*dockerStatsState{}
	d.processes = map[string]*dockerProcessState{}
	d.history = map[string]*dockerHistoryState{}
	if err != nil {
		d.errors["settings"] = "The backend disconnected: " + err.Error()
	}
	m.invalidate()
}

func (m *Model) dockerBackendRestored() {
	d := &m.services.docker
	d.backendLost = false
	delete(d.errors, "settings")
	if !d.watching {
		return
	}
	d.loaded = false
	d.loading = false
	m.loadDockerServices()
	m.startDockerPoll()
	m.invalidate()
}

func dockerConnectionKey(id string) string {
	return dockerNodePrefix + id
}

func dockerChildKey(connectionID, local string) string {
	return dockerConnectionKey(connectionID) + ":" + local
}

func (m *Model) dockerConnection(id string) *openidea.DockerConnection {
	for i := range m.services.docker.settings.Connections {
		if m.services.docker.settings.Connections[i].ID == id {
			return &m.services.docker.settings.Connections[i]
		}
	}
	return nil
}

func (m *Model) pruneDockerState(settings openidea.DockerSettings) {
	d := &m.services.docker
	connections := make(map[string]bool, len(settings.Connections))
	for _, connection := range settings.Connections {
		connections[connection.ID] = true
	}
	for id := range d.snapshots {
		if !connections[id] {
			m.stopDockerConnection(id)
		}
	}
	registries := make(map[string]bool, len(settings.Registries))
	for _, registry := range settings.Registries {
		registries[registry.ID] = true
	}
	for id := range d.logins {
		if !registries[id] {
			delete(d.logins, id)
		}
	}
}

func (m *Model) pruneDockerSnapshotCaches(connectionID string, snapshot openidea.DockerSnapshot) {
	d := &m.services.docker
	valid := map[string]bool{}
	contains := map[string]bool{}
	for _, container := range snapshot.Containers {
		valid[dockerDetailKey(connectionID, "container", container.ID)] = true
		contains[container.ID] = true
	}
	for _, image := range snapshot.Images {
		valid[dockerDetailKey(connectionID, "image", image.ID)] = true
	}
	for _, volume := range snapshot.Volumes {
		valid[dockerDetailKey(connectionID, "volume", volume.Name)] = true
	}
	for _, network := range snapshot.Networks {
		valid[dockerDetailKey(connectionID, "network", network.ID)] = true
	}
	prefix := connectionID + "\x00"
	prune := func(key string) bool {
		return strings.HasPrefix(key, prefix) && !valid[key]
	}
	for key := range d.inspect {
		if prune(key) {
			delete(d.inspect, key)
		}
	}
	for key := range d.stats {
		if prune(key) {
			delete(d.stats, key)
		}
	}
	for key := range d.processes {
		if prune(key) {
			delete(d.processes, key)
		}
	}
	for key := range d.history {
		if prune(key) {
			delete(d.history, key)
		}
	}
	for key, stream := range d.logs {
		if stream == nil || stream.connection != connectionID ||
			contains[stream.container] || stream.editor != nil {
			continue
		}
		delete(d.logs, key)
		m.stopDockerLogStream(stream)
	}
}

func (m *Model) dockerFilters() DockerFilterPrefs {
	return m.prefs.Services.Docker
}

func (m *Model) setDockerFilter(stopped, untagged *bool) {
	if stopped != nil {
		m.prefs.Services.Docker.ShowStoppedContainers = stopped
	}
	if untagged != nil {
		m.prefs.Services.Docker.ShowUntaggedImages = untagged
	}
	m.savePreferences()
	m.invalidate()
}

func dockerUntaggedImage(image openidea.DockerImage) bool {
	return (image.Repository == "" || image.Repository == "<none>") &&
		(image.Tag == "" || image.Tag == "<none>")
}

func dockerContainerVisible(container openidea.DockerContainer, filters DockerFilterPrefs) bool {
	return filters.showStoppedContainers() || dockerContainerRunning(container)
}

func dockerImageVisible(image openidea.DockerImage, filters DockerFilterPrefs) bool {
	return filters.showUntaggedImages() || !dockerUntaggedImage(image)
}

func (m *Model) dockerServiceNodes() []serviceTreeNode {
	d := &m.services.docker
	if !d.loaded {
		return nil
	}
	nodes := make([]serviceTreeNode, 0, len(d.settings.Connections))
	for _, connection := range d.settings.Connections {
		snapshot, hasSnapshot := d.snapshots[connection.ID]
		node := serviceTreeNode{
			key:        dockerConnectionKey(connection.ID),
			label:      connection.Name,
			nodeKind:   "docker",
			provider:   dockerProviderID,
			icon:       "docker/dockerConnected",
			dockerConn: connection.ID,
		}
		switch {
		case d.errors[connection.ID] != "":
			node.detail = "connection failed"
			node.icon = "fileTypes/docker"
		case d.refreshing[connection.ID] || !hasSnapshot:
			node.detail = "connecting…"
			node.icon = "docker/dockerConnected"
		default:
			node.detail = dockerConnectionDetail(snapshot)
			if snapshot.Availability.Kind != "ready" {
				node.icon = "fileTypes/docker"
			}
		}
		if !m.services.collapsed[node.key] && hasSnapshot && snapshot.Availability.Kind == "ready" {
			node.children = m.dockerChildren(connection.ID, snapshot)
		}
		nodes = append(nodes, node)
	}
	for _, registry := range d.settings.Registries {
		nodes = append(nodes, serviceTreeNode{key: "docker-registry:" + registry.ID, label: registry.Name, detail: registry.Address, nodeKind: "docker-registry", provider: dockerProviderID, icon: "docker/dockerRegistry", dockerID: registry.ID})
	}
	return nodes
}

func dockerConnectionDetail(snapshot openidea.DockerSnapshot) string {
	switch snapshot.Availability.Kind {
	case "ready":
		if snapshot.Availability.Context != nil && *snapshot.Availability.Context != "" {
			return *snapshot.Availability.Context
		}
		if snapshot.Availability.ServerVersion != "" {
			return "Docker " + snapshot.Availability.ServerVersion
		}
		return "connected"
	case "notInstalled":
		return "docker not installed"
	case "daemonDown":
		return "not running"
	default:
		return snapshot.Availability.Kind
	}
}

func (m *Model) dockerChildren(connectionID string, snapshot openidea.DockerSnapshot) []serviceTreeNode {
	children := make([]serviceTreeNode, 0)
	filters := m.dockerFilters()

	projects := map[string][]openidea.DockerContainer{}
	for _, container := range snapshot.Containers {
		if container.ComposeProject == nil || strings.TrimSpace(*container.ComposeProject) == "" {
			continue
		}
		project := strings.TrimSpace(*container.ComposeProject)
		projects[project] = append(projects[project], container)
	}
	names := make([]string, 0, len(projects))
	for project := range projects {
		names = append(names, project)
	}
	sort.Strings(names)
	for _, project := range names {
		allContainers := projects[project]
		sort.SliceStable(allContainers, func(i, j int) bool {
			left, right := allContainers[i].Name, allContainers[j].Name
			if allContainers[i].ComposeService != nil {
				left = *allContainers[i].ComposeService
			}
			if allContainers[j].ComposeService != nil {
				right = *allContainers[j].ComposeService
			}
			return left < right
		})
		running := 0
		for _, container := range allContainers {
			if dockerContainerRunning(container) {
				running++
			}
		}
		visible := make([]openidea.DockerContainer, 0, len(allContainers))
		for _, container := range allContainers {
			if dockerContainerVisible(container, filters) {
				visible = append(visible, container)
			}
		}
		if len(visible) == 0 {
			continue
		}
		node := serviceTreeNode{
			key:        dockerChildKey(connectionID, "compose:"+project),
			label:      "Docker-compose: " + project,
			detail:     fmt.Sprintf("%d/%d running", running, len(allContainers)),
			nodeKind:   "docker-compose",
			provider:   dockerProviderID,
			icon:       "docker/composeProjectStopped",
			dockerConn: connectionID,
			dockerID:   project,
		}
		if running > 0 {
			node.icon = "docker/composeProjectUp"
		}
		if !m.services.collapsed[node.key] {
			for _, container := range visible {
				node.children = append(node.children, m.dockerContainerNode(connectionID, container, true))
			}
		}
		children = append(children, node)
	}

	children = append(children,
		m.dockerGroupNode(connectionID, "devContainers", "Dev Containers",
			dockerContainers(snapshot, "devContainers", filters)),
		m.dockerGroupNode(connectionID, "containers", "Containers",
			dockerContainers(snapshot, "containers", filters)),
		m.dockerGroupNode(connectionID, "images", "Images", nil),
		m.dockerGroupNode(connectionID, "networks", "Networks", nil),
		m.dockerGroupNode(connectionID, "volumes", "Volumes", nil),
	)

	for i := len(children) - 3; i < len(children); i++ {
		group := &children[i]
		if m.services.collapsed[group.key] {
			continue
		}
		switch group.dockerID {
		case "images":
			seen := map[string]bool{}
			for _, image := range snapshot.Images {
				if !dockerImageVisible(image, filters) {
					continue
				}
				key := image.ID + "\x00" + dockerImageRef(image)
				if seen[key] {
					continue
				}
				seen[key] = true
				group.children = append(group.children, m.dockerImageNode(connectionID, image))
			}
		case "networks":
			for _, network := range snapshot.Networks {
				group.children = append(group.children, m.dockerNetworkNode(connectionID, network))
			}
		case "volumes":
			for _, volume := range snapshot.Volumes {
				group.children = append(group.children, m.dockerVolumeNode(connectionID, volume))
			}
		}
	}
	return children
}

func (m *Model) dockerGroupNode(connectionID, kind, label string, containers []openidea.DockerContainer) serviceTreeNode {
	count := len(containers)
	icon := "docker/containersList"
	switch kind {
	case "devContainers":
		icon = "docker/devContainers"
	case "images":
		icon = "docker/imagesList"
		seen := map[string]bool{}
		for _, image := range m.services.docker.snapshots[connectionID].Images {
			if !dockerImageVisible(image, m.dockerFilters()) {
				continue
			}
			key := image.ID + "\x00" + dockerImageRef(image)
			if !seen[key] {
				seen[key] = true
				count++
			}
		}
	case "networks":
		icon = "docker/network"
		count = len(m.services.docker.snapshots[connectionID].Networks)
	case "volumes":
		icon = "docker/volume"
		count = len(m.services.docker.snapshots[connectionID].Volumes)
	}
	node := serviceTreeNode{
		key:        dockerChildKey(connectionID, "group:"+kind),
		label:      label,
		nodeKind:   "docker-group",
		provider:   dockerProviderID,
		icon:       icon,
		dockerConn: connectionID,
		dockerID:   kind,
	}
	if count > 0 {
		node.detail = fmt.Sprintf("%d", count)
	}
	if kind == "devContainers" || kind == "containers" {
		if !m.services.collapsed[node.key] {
			for _, container := range containers {
				node.children = append(node.children, m.dockerContainerNode(connectionID, container, false))
			}
		}
	}
	return node
}

func dockerContainers(snapshot openidea.DockerSnapshot, group string, filters DockerFilterPrefs) []openidea.DockerContainer {
	out := make([]openidea.DockerContainer, 0)
	for _, container := range snapshot.Containers {
		if container.ComposeProject != nil && strings.TrimSpace(*container.ComposeProject) != "" {
			continue
		}
		if !dockerContainerVisible(container, filters) {
			continue
		}
		dev := dockerDevContainer(container)
		if group == "devContainers" && !dev || group == "containers" && dev {
			continue
		}
		out = append(out, container)
	}
	return out
}

func dockerDevContainer(container openidea.DockerContainer) bool {
	if container.Labels == nil {
		return false
	}
	_, local := container.Labels["devcontainer.local_folder"]
	_, idea := container.Labels["com.intellij.devcontainer.id"]
	_, vscode := container.Labels["vsch.local.folder"]
	return local || idea || vscode
}

func dockerContainerRunning(container openidea.DockerContainer) bool {
	switch container.State {
	case "running", "restarting", "paused":
		return true
	}
	return false
}

func dockerShortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func dockerImageRef(image openidea.DockerImage) string {
	if image.Repository == "" || image.Repository == "<none>" {
		return dockerShortID(image.ID)
	}
	if image.Tag == "" || image.Tag == "<none>" {
		return image.Repository
	}
	return image.Repository + ":" + image.Tag
}

func (m *Model) dockerContainerNode(connectionID string, container openidea.DockerContainer, compose bool) serviceTreeNode {
	label := container.Name
	if compose && container.ComposeService != nil && *container.ComposeService != "" {
		label = *container.ComposeService
	}
	icon := "docker/singleStoppedContainer"
	if container.State == "paused" {
		icon = "run/pause"
	} else if dockerContainerRunning(container) {
		icon = "docker/singleRunningContainer"
	}
	if compose {
		icon = "docker/dockerComposeServiceStopped"
		if dockerContainerRunning(container) {
			icon = "docker/dockerComposeServiceUp"
		}
	}
	detail := container.Status
	if compose && container.Name != "" {
		detail += " · " + container.Name
	} else if container.Image != "" {
		detail += " · " + container.Image
	}
	return serviceTreeNode{
		key:        dockerChildKey(connectionID, "container:"+container.ID),
		label:      label,
		detail:     detail,
		nodeKind:   "docker-container",
		provider:   dockerProviderID,
		icon:       icon,
		dockerConn: connectionID,
		dockerID:   container.ID,
		live:       dockerContainerRunning(container),
	}
}

func (m *Model) dockerImageNode(connectionID string, image openidea.DockerImage) serviceTreeNode {
	return serviceTreeNode{
		key:        dockerChildKey(connectionID, "image:"+image.ID+":"+dockerImageRef(image)),
		label:      dockerImageRef(image),
		detail:     image.Size + " · " + image.CreatedSince,
		nodeKind:   "docker-image",
		provider:   dockerProviderID,
		icon:       "docker/singleImage",
		dockerConn: connectionID,
		dockerID:   image.ID,
		dockerRef:  dockerImageRef(image),
	}
}

func (m *Model) dockerVolumeNode(connectionID string, volume openidea.DockerVolume) serviceTreeNode {
	return serviceTreeNode{
		key:        dockerChildKey(connectionID, "volume:"+volume.Name),
		label:      volume.Name,
		detail:     volume.Driver,
		nodeKind:   "docker-volume",
		provider:   dockerProviderID,
		icon:       "docker/volume",
		dockerConn: connectionID,
		dockerID:   volume.Name,
	}
}

func (m *Model) dockerNetworkNode(connectionID string, network openidea.DockerNetwork) serviceTreeNode {
	return serviceTreeNode{
		key:        dockerChildKey(connectionID, "network:"+network.ID),
		label:      network.Name,
		detail:     network.Driver,
		nodeKind:   "docker-network",
		provider:   dockerProviderID,
		icon:       "docker/network",
		dockerConn: connectionID,
		dockerID:   network.ID,
	}
}

func dockerDetailTabLabel(tab string) string {
	switch tab {
	case dockerDetailLog:
		return "Log"
	case dockerDetailProcesses:
		return "Processes"
	case dockerDetailLayers:
		return "Layers"
	case dockerDetailInspect:
		return "Inspect"
	default:
		return "Dashboard"
	}
}

func dockerNodeDetailTabs(node serviceTreeNode) []string {
	switch node.nodeKind {
	case "docker-container":
		return []string{
			dockerDetailDashboard, dockerDetailLog,
			dockerDetailProcesses, dockerDetailInspect,
		}
	case "docker-image":
		return []string{dockerDetailDashboard, dockerDetailLayers, dockerDetailInspect}
	case "docker-volume", "docker-network":
		return []string{dockerDetailDashboard, dockerDetailInspect}
	default:
		return nil
	}
}

func dockerDetailTabSupported(node serviceTreeNode, tab string) bool {
	for _, candidate := range dockerNodeDetailTabs(node) {
		if candidate == tab {
			return true
		}
	}
	return false
}

// setDockerDetailTab mirrors OpenIDEA's shared Services detail tab. Selecting
// a tab lazily starts only the work that tab shows.
func (m *Model) setDockerDetailTab(
	connection openidea.DockerConnection,
	node serviceTreeNode,
	tab string,
) {
	if !dockerDetailTabSupported(node, tab) {
		tab = dockerDetailDashboard
	}
	m.services.docker.detailTab = tab
	if tab != dockerDetailLog {
		m.retainDockerDetailLog("")
	}
	switch tab {
	case dockerDetailLog:
		snapshot := m.services.docker.snapshots[connection.ID]
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			m.ensureDockerDetailLog(connection, node, dockerContainerRunning(*container))
		}
	case dockerDetailProcesses:
		snapshot := m.services.docker.snapshots[connection.ID]
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil &&
			dockerContainerRunning(*container) {
			m.loadDockerProcesses(connection, node.dockerID)
		}
	case dockerDetailLayers:
		m.loadDockerHistory(connection, node.dockerID)
	case dockerDetailInspect:
		m.loadDockerInspect(connection, strings.TrimPrefix(node.nodeKind, "docker-"), node.dockerID)
	case dockerDetailDashboard:
		m.loadDockerDashboardData(connection, node)
	}
	m.invalidate()
}

func (m *Model) loadDockerDashboardData(
	connection openidea.DockerConnection,
	node serviceTreeNode,
) {
	switch node.nodeKind {
	case "docker-container":
		snapshot := m.services.docker.snapshots[connection.ID]
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			if dockerContainerRunning(*container) {
				m.loadDockerStats(connection, node.dockerID)
			}
			m.loadDockerInspect(connection, "container", node.dockerID)
		}
	case "docker-image":
		m.loadDockerInspect(connection, "image", node.dockerID)
	case "docker-network":
		m.loadDockerInspect(connection, "network", node.dockerID)
	}
}

func (m *Model) ensureDockerDashboardData(
	connection openidea.DockerConnection,
	node serviceTreeNode,
) {
	d := &m.services.docker
	switch node.nodeKind {
	case "docker-container":
		snapshot := d.snapshots[connection.ID]
		container := dockerContainerByID(snapshot, node.dockerID)
		if container == nil {
			return
		}
		if dockerContainerRunning(*container) &&
			d.stats[dockerDetailKey(connection.ID, "container", node.dockerID)] == nil {
			m.loadDockerStats(connection, node.dockerID)
		}
		if d.inspect[dockerDetailKey(connection.ID, "container", node.dockerID)] == nil {
			m.loadDockerInspect(connection, "container", node.dockerID)
		}
	case "docker-image":
		if d.inspect[dockerDetailKey(connection.ID, "image", node.dockerID)] == nil {
			m.loadDockerInspect(connection, "image", node.dockerID)
		}
	case "docker-network":
		if d.inspect[dockerDetailKey(connection.ID, "network", node.dockerID)] == nil {
			m.loadDockerInspect(connection, "network", node.dockerID)
		}
	}
}

func (m *Model) retainDockerDetailLog(key string) {
	for streamKey, stream := range m.services.docker.logs {
		if stream == nil || stream.editor != nil || streamKey == key {
			continue
		}
		delete(m.services.docker.logs, streamKey)
		m.stopDockerLogStream(stream)
	}
}

func (m *Model) dockerDetailTabRow(
	c *ui.Context,
	p *ui.Theme,
	connection openidea.DockerConnection,
	node serviceTreeNode,
) string {
	tabs := dockerNodeDetailTabs(node)
	if len(tabs) == 0 {
		return ""
	}
	active := m.services.docker.detailTab
	if !dockerDetailTabSupported(node, active) {
		active = dockerDetailDashboard
	}
	ui.Row(c).Gap(2).Shrink(0).Children(func() {
		for _, tab := range tabs {
			tab := tab
			button := ui.Button(c, dockerDetailTabLabel(tab))
			if tab == active {
				button.Background(p.Accent.Mix(p.Surface, 0.82)).TextColor(p.Text)
			} else {
				button.Background(p.Background).TextColor(p.TextMuted)
			}
			if button.Clicked() {
				m.setDockerDetailTab(connection, node, tab)
			}
		}
	})
	return active
}

func (m *Model) dockerServiceProviderActivate(node serviceTreeNode) {
	if node.nodeKind == "docker-registry" {
		for _, registry := range m.services.docker.settings.Registries {
			if registry.ID == node.dockerID {
				m.editDockerRegistry(&registry)
				return
			}
		}
		return
	}
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil || node.nodeKind != "docker-container" {
		return
	}
	m.setDockerDetailTab(*connection, node, dockerDetailLog)
}

func (m *Model) dockerServiceDetail(c *ui.Context, p *ui.Theme, node serviceTreeNode) {
	if node.nodeKind == "docker-registry" {
		m.dockerRegistryServiceDetail(c, p, node)
		return
	}
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		ui.Text(c, "This Docker connection no longer exists.").FontSize(12).TextColor(p.TextMuted)
		return
	}
	snapshot, hasSnapshot := m.services.docker.snapshots[connection.ID]
	if errText := m.services.docker.errors[connection.ID]; errText != "" {
		ui.Column(c).Grow(1).Padding(22, 18).Gap(9).Children(func() {
			ui.Text(c, connection.Name).FontSize(15).Bold().TextColor(p.Text)
			ui.Text(c, "Could not connect to Docker: "+errText).FontSize(12).
				TextColor(p.Danger).SingleLine().Ellipsis("…")
			if ui.Button(c, "Retry").Clicked() {
				m.refreshDockerSnapshot(*connection)
			}
		})
		return
	}
	if !hasSnapshot {
		ui.Column(c).Grow(1).Padding(22, 18).Gap(8).Children(func() {
			ui.Text(c, connection.Name).FontSize(15).Bold().TextColor(p.Text)
			ui.Text(c, "Connecting to Docker…").FontSize(12).TextColor(p.TextMuted)
			if ui.Button(c, "Retry").Clicked() {
				m.refreshDockerSnapshot(*connection)
			}
		})
		return
	}
	if snapshot.Availability.Kind != "ready" && node.nodeKind == "docker" {
		m.dockerConnectionUnavailable(c, p, *connection, snapshot)
		return
	}

	switch node.nodeKind {
	case "docker":
		m.dockerConnectionDetail(c, p, *connection, snapshot)
	case "docker-group":
		m.dockerGroupDetail(c, p, node, snapshot)
	case "docker-compose":
		m.dockerComposeDetail(c, p, node, snapshot)
	case "docker-container":
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			m.dockerContainerDetail(c, p, node, snapshot, *container)
		}
	case "docker-image":
		if image := dockerImageForNode(snapshot, node); image != nil {
			m.dockerImageDetail(c, p, node, *image)
		}
	case "docker-volume":
		if volume := dockerVolumeByName(snapshot, node.dockerID); volume != nil {
			m.dockerVolumeDetail(c, p, node, *volume)
		}
	case "docker-network":
		if network := dockerNetworkByID(snapshot, node.dockerID); network != nil {
			m.dockerNetworkDetail(c, p, node, *network)
		}
	}
}

func (m *Model) dockerRegistryServiceDetail(c *ui.Context, p *ui.Theme, node serviceTreeNode) {
	d := &m.services.docker
	for _, registry := range d.settings.Registries {
		if registry.ID != node.dockerID {
			continue
		}
		login := d.logins[registry.ID]
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(22, 18).Gap(8).Children(func() {
				ui.Text(c, registry.Name).FontSize(15).Bold().TextColor(p.Text)
				serviceFact(c, p, "Address", registry.Address, false)
				serviceFact(c, p, "Username", registry.Username, false)
				connection, _ := m.dockerRegistryEndpoint(registry.ID)
				button := ui.Button(c, "Login via: "+connection.Name)
				button.Menu(func(menu *ui.Menu) {
					state := d.logins[registry.ID]
					if state == nil {
						state = &dockerRegistryLoginState{}
						d.logins[registry.ID] = state
					}
					if menu.Item("Docker CLI default").
						Checked(state.connectionSelected && state.connectionID == "").Chosen() {
						state.connectionID = ""
						state.connectionSelected = true
						state.answer, state.err = "", ""
						m.invalidate()
					}
					if len(d.settings.Connections) > 0 {
						menu.Separator()
					}
					for _, candidate := range d.settings.Connections {
						candidateID := candidate.ID
						if !menu.Item(candidate.Name).
							Checked(state.connectionSelected && state.connectionID == candidateID).Chosen() {
							continue
						}
						state.connectionID = candidateID
						state.connectionSelected = true
						state.answer, state.err = "", ""
						m.invalidate()
					}
				})
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "Edit Registry…").Clicked() {
						m.editDockerRegistry(&registry)
					}
					test := ui.Button(c, "Test Login")
					test.Disabled(strings.TrimSpace(registry.Username) == "" || (login != nil && login.loading))
					if test.Clicked() {
						m.testDockerRegistryLogin(registry)
					}
					if ui.Button(c, "Remove Registry…").Clicked() {
						m.removeDockerRegistry(registry)
					}
				})
				if login == nil {
					return
				}
				if login.loading {
					ui.Text(c, "Testing registry login…").FontSize(12).TextColor(p.TextMuted)
				}
				if login.answer != "" {
					ui.Text(c, login.answer).FontSize(12).TextColor(p.Success)
				}
				if login.err != "" {
					ui.Text(c, login.err).FontSize(12).TextColor(p.Danger)
				}
			})
		})
		return
	}
	ui.Text(c, "This Docker registry no longer exists.").FontSize(12).TextColor(p.TextMuted)
}

func (m *Model) dockerDialogView(c *ui.Context, p *ui.Theme) {
	dialog := m.services.docker.dialog
	if dialog == nil {
		return
	}
	open := true
	modal := ui.Modal(c.Key("services:docker-dialog"), &open, func() {
		ui.Column(c).Width(480).Padding(18).Gap(10).Children(func() {
			ui.Text(c, dialog.title).FontSize(15).Bold().TextColor(p.Text)
			if dialog.message != "" {
				ui.Text(c, dialog.message).FontSize(12).TextColor(p.TextMuted)
			}
			switch dialog.kind {
			case "pull":
				ui.Text(c, "Image to pull:").FontSize(12).TextColor(p.TextMuted)
				ui.TextInput(c, &dialog.value).Grow(1).Font("monospace").
					Placeholder("e.g. postgres:16 or ghcr.io/org/app:latest")
			case "remove":
				for _, id := range dialog.ids {
					serviceFact(c, p, "Object", dockerShortID(id), true)
				}
				if dialog.object == "image" {
					ui.Checkbox(c, &dialog.force, "Force (also untag / remove when used by stopped containers)")
				}
			}
			if dialog.err != "" {
				ui.Text(c, dialog.err).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				label := dialog.button
				if label == "" {
					label = "OK"
				}
				submit := ui.Button(c, label)
				submit.Disabled(dialog.busy ||
					(dialog.kind == "pull" && strings.TrimSpace(dialog.value) == ""))
				if submit.Clicked() {
					m.submitDockerDialog()
				}
				cancel := ui.Button(c, "Cancel")
				cancel.Disabled(dialog.busy)
				if cancel.Clicked() {
					m.services.docker.dialog = nil
					m.invalidate()
				}
			})
		})
	})
	if dialog.busy {
		return
	}
	if !open || modal.Dismissed() {
		m.services.docker.dialog = nil
		m.invalidate()
	}
}

func (m *Model) dockerConnectionUnavailable(c *ui.Context, p *ui.Theme,
	connection openidea.DockerConnection, snapshot openidea.DockerSnapshot) {
	title := "Docker is not available"
	message := "Docker is not available."
	switch snapshot.Availability.Kind {
	case "notInstalled":
		title = "Docker is not installed"
		message = "Install Docker Desktop, OrbStack, Colima or the Docker Engine and make sure docker is on PATH."
	case "daemonDown":
		title = "Docker is not running"
		message = snapshot.Availability.Message
	}
	ui.Column(c).Grow(1).Padding(22, 18).Gap(9).Children(func() {
		ui.Text(c, title).FontSize(15).Bold().TextColor(p.Text)
		ui.Text(c, message).FontSize(12).TextColor(p.TextMuted)
		serviceFact(c, p, "Endpoint", dockerEndpointText(connection.Endpoint), true)
		if snapshot.Availability.Kind == "daemonDown" {
			hint := "Start Docker; this view connects automatically."
			if snapshot.Availability.Context != nil && *snapshot.Availability.Context != "" {
				hint = "Start Docker (context '" + *snapshot.Availability.Context + "'); this view connects automatically."
			}
			ui.Text(c, hint).FontSize(11).TextColor(p.TextMuted)
			if ui.Button(c, "Retry").Clicked() {
				m.refreshDockerSnapshot(connection)
			}
		}
	})
}

func (m *Model) dockerConnectionDetail(c *ui.Context, p *ui.Theme,
	connection openidea.DockerConnection, snapshot openidea.DockerSnapshot) {
	busy := m.dockerOperationBusy()
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(9).Children(func() {
			ui.Text(c, connection.Name).FontSize(15).Bold().TextColor(p.Text)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "Refresh").Clicked() {
					m.refreshDockerSnapshot(connection)
				}
				pull := ui.Button(c, "Pull Image…")
				pull.Disabled(busy)
				if pull.Clicked() {
					m.openDockerPullDialog(connection.ID, "")
				}
				if ui.Button(c, "Edit…").Clicked() {
					m.editDockerConnection(&connection)
				}
				prune := ui.Button(c, "Prune System…")
				prune.Disabled(busy)
				if prune.Clicked() {
					m.confirmDockerPrune(connection, "system")
				}
				if ui.Button(c, "Remove Connection…").Clicked() {
					m.removeDockerConnection(connection)
				}
			})
			serviceFact(c, p, "Endpoint", dockerEndpointText(connection.Endpoint), true)
			if snapshot.Executable != nil && *snapshot.Executable != "" {
				serviceFact(c, p, "Executable", *snapshot.Executable, true)
			}
			if snapshot.Availability.ClientVersion != "" {
				serviceFact(c, p, "Client", snapshot.Availability.ClientVersion, false)
			}
			if snapshot.Availability.ServerVersion != "" {
				serviceFact(c, p, "Server", snapshot.Availability.ServerVersion, false)
			}
			if snapshot.Availability.Context != nil && *snapshot.Availability.Context != "" {
				serviceFact(c, p, "Context", *snapshot.Availability.Context, false)
			}
			for _, errorText := range snapshot.Errors {
				ui.Text(c, errorText).FontSize(12).TextColor(p.Warning)
			}
			m.dockerOperationStatus(c, p)
		})
	})
}

func (m *Model) dockerGroupDetail(c *ui.Context, p *ui.Theme,
	node serviceTreeNode, snapshot openidea.DockerSnapshot) {
	busy := m.dockerOperationBusy()
	count := 0
	switch node.dockerID {
	case "devContainers":
		count = len(dockerContainers(snapshot, "devContainers", m.dockerFilters()))
	case "containers":
		count = len(dockerContainers(snapshot, "containers", m.dockerFilters()))
	case "images":
		count = len(snapshot.Images)
	case "networks":
		count = len(snapshot.Networks)
	case "volumes":
		count = len(snapshot.Volumes)
	}
	ui.Column(c).Grow(1).Padding(22, 18).Gap(8).Children(func() {
		ui.Text(c, node.label).FontSize(15).Bold().TextColor(p.Text)
		ui.Text(c, fmt.Sprintf("%d %s", count, plural(count, "object", "objects"))).
			FontSize(12).TextColor(p.TextMuted)
		switch node.dockerID {
		case "containers":
			prune := ui.Button(c, "Prune Stopped Containers…")
			prune.Disabled(busy)
			if prune.Clicked() {
				if connection := m.dockerConnection(node.dockerConn); connection != nil {
					m.confirmDockerPrune(*connection, "containers")
				}
			}
		case "images":
			pull := ui.Button(c, "Pull Image…")
			pull.Disabled(busy)
			if pull.Clicked() {
				m.openDockerPullDialog(node.dockerConn, "")
			}
			prune := ui.Button(c, "Prune Dangling Images…")
			prune.Disabled(busy)
			if prune.Clicked() {
				if connection := m.dockerConnection(node.dockerConn); connection != nil {
					m.confirmDockerPrune(*connection, "images")
				}
			}
		case "networks":
			prune := ui.Button(c, "Prune Unused Networks…")
			prune.Disabled(busy)
			if prune.Clicked() {
				if connection := m.dockerConnection(node.dockerConn); connection != nil {
					m.confirmDockerPrune(*connection, "networks")
				}
			}
		case "volumes":
			prune := ui.Button(c, "Prune Unused Volumes…")
			prune.Disabled(busy)
			if prune.Clicked() {
				if connection := m.dockerConnection(node.dockerConn); connection != nil {
					m.confirmDockerPrune(*connection, "volumes")
				}
			}
		}
		m.dockerOperationStatus(c, p)
	})
}

func (m *Model) dockerComposeDetail(c *ui.Context, p *ui.Theme,
	node serviceTreeNode, snapshot openidea.DockerSnapshot) {
	busy := m.dockerOperationBusy()
	containers := dockerComposeContainers(snapshot, node.dockerID)
	running := 0
	for _, container := range containers {
		if dockerContainerRunning(container) {
			running++
		}
	}
	stopped := running < len(containers)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(9).Children(func() {
			ui.Text(c, "Compose project '"+node.dockerID+"'").FontSize(15).Bold().TextColor(p.Text)
			ui.Row(c).Gap(6).Children(func() {
				start := ui.Button(c, "Start")
				start.Disabled(busy || !stopped)
				if start.Clicked() {
					m.runDockerComposeOp(node, snapshot, "start")
				}
				stop := ui.Button(c, "Stop")
				stop.Disabled(busy || running == 0)
				if stop.Clicked() {
					m.runDockerComposeOp(node, snapshot, "stop")
				}
				restart := ui.Button(c, "Restart")
				restart.Disabled(busy)
				if restart.Clicked() {
					m.runDockerComposeOp(node, snapshot, "restart")
				}
				down := ui.Button(c, "Compose Down")
				down.Disabled(busy)
				if down.Clicked() {
					m.confirmDockerComposeDown(node, snapshot)
				}
			})
			serviceFact(c, p, "Project", node.dockerID, false)
			serviceFact(c, p, "Status", fmt.Sprintf("%d of %d running", running, len(containers)), false)
			if len(containers) > 0 {
				first := containers[0]
				if first.ComposeWorkingDir != nil && *first.ComposeWorkingDir != "" {
					serviceFact(c, p, "Working directory", *first.ComposeWorkingDir, true)
				}
				if len(first.ComposeFiles) > 0 {
					serviceFact(c, p, "Compose files", strings.Join(first.ComposeFiles, ", "), true)
				}
			}
			for _, container := range containers {
				name := container.Name
				if container.ComposeService != nil && *container.ComposeService != "" {
					name = *container.ComposeService
				}
				serviceFact(c, p, "Service", name+" · "+container.Status, false)
			}
			m.dockerOperationStatus(c, p)
		})
	})
}

func (m *Model) dockerContainerDetail(c *ui.Context, p *ui.Theme,
	node serviceTreeNode, snapshot openidea.DockerSnapshot, container openidea.DockerContainer) {
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	busy := m.dockerOperationBusy()
	running := dockerContainerRunning(container)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(9).Children(func() {
			ui.Text(c, container.Name).FontSize(15).Bold().TextColor(p.Text).
				SingleLine().Ellipsis("…")
			ui.Row(c).Gap(6).Children(func() {
				start := ui.Button(c, "Start")
				start.Disabled(busy || running)
				if start.Clicked() {
					m.runDockerContainerOp(node, "start")
				}
				stop := ui.Button(c, "Stop")
				stop.Disabled(busy || !running)
				if stop.Clicked() {
					m.runDockerContainerOp(node, "stop")
				}
				restart := ui.Button(c, "Restart")
				restart.Disabled(busy)
				if restart.Clicked() {
					m.runDockerContainerOp(node, "restart")
				}
				if container.State == "paused" {
					unpause := ui.Button(c, "Unpause")
					unpause.Disabled(busy)
					if unpause.Clicked() {
						m.runDockerContainerOp(node, "unpause")
					}
				} else {
					pause := ui.Button(c, "Pause")
					pause.Disabled(busy || !running)
					if pause.Clicked() {
						m.runDockerContainerOp(node, "pause")
					}
				}
				terminal := ui.Button(c, "Create Terminal")
				terminal.Disabled(!running)
				if terminal.Clicked() {
					m.openDockerTerminal(*connection, container)
				}
			})
			ui.Row(c).Gap(6).Children(func() {
				if ui.Button(c, "Show Log").Clicked() {
					m.setDockerDetailTab(*connection, node, dockerDetailLog)
				}
				if ui.Button(c, "Open Log in Editor").Clicked() {
					m.openDockerLogEditor(*connection, container)
				}
				deleteButton := ui.Button(c, "Delete…")
				deleteButton.Disabled(busy)
				if deleteButton.Clicked() {
					m.confirmDockerRemove(node, "container", []string{container.ID})
				}
			})
			active := m.dockerDetailTabRow(c, p, *connection, node)
			m.dockerOperationStatus(c, p)
			switch active {
			case dockerDetailLog:
				stream := m.ensureDockerDetailLog(*connection, node, running)
				if stream != nil {
					m.dockerLogView(c, p, stream)
				}
			case dockerDetailProcesses:
				state := m.services.docker.processes[dockerDetailKey(connection.ID, "container", container.ID)]
				if running && state == nil {
					m.loadDockerProcesses(*connection, container.ID)
					state = m.services.docker.processes[dockerDetailKey(connection.ID, "container", container.ID)]
				}
				if !running {
					ui.Text(c, "Start the container to view its running processes.").
						FontSize(12).TextColor(p.TextMuted)
				} else {
					m.dockerProcessesView(c, p, node, container)
				}
			case dockerDetailInspect:
				state := m.services.docker.inspect[dockerDetailKey(connection.ID, "container", container.ID)]
				if state == nil {
					m.loadDockerInspect(*connection, "container", container.ID)
					state = m.services.docker.inspect[dockerDetailKey(connection.ID, "container", container.ID)]
				}
				m.dockerInspectView(c, p, node, "container", container.ID)
			default:
				m.ensureDockerDashboardData(*connection, node)
				ui.Text(c, "Container Overview").FontSize(12).Bold().TextColor(p.Text)
				serviceFact(c, p, "Name", container.Name, true)
				dockerCopyableFact(c, p, "Container ID", dockerShortID(container.ID), container.ID, true)
				dockerCopyableFact(c, p, "Image", container.Image, container.Image, true)
				serviceFact(c, p, "State", container.State, false)
				serviceFact(c, p, "Status", container.Status, false)
				created := container.CreatedAt
				if inspect := m.dockerContainerInspect(connection.ID, container.ID); inspect != nil && inspect.Created != "" {
					created = inspect.Created
				}
				if created != "" {
					serviceFact(c, p, "Created", created, true)
				}
				if container.ComposeProject != nil && *container.ComposeProject != "" {
					compose := *container.ComposeProject
					if container.ComposeService != nil && *container.ComposeService != "" {
						compose += " · " + *container.ComposeService
					}
					serviceFact(c, p, "Compose", compose, false)
				}
				if container.Command != "" {
					serviceFact(c, p, "Command", container.Command, true)
				}
				m.dockerContainerInspectDetails(c, p, node, container)
				if running {
					state := m.services.docker.stats[dockerDetailKey(connection.ID, "container", container.ID)]
					if state == nil {
						m.loadDockerStats(*connection, container.ID)
					}
					m.dockerStatsView(c, p, node, container)
				}
				m.dockerContainerPortBindings(c, p, node, container)
				m.dockerContainerMountsView(c, p, node, container)
				m.dockerContainerNetworksView(c, p, node, container)
				m.dockerContainerEnvironmentView(c, p, node, container)
				labels := container.Labels
				if inspect := m.dockerContainerInspect(connection.ID, container.ID); inspect != nil &&
					len(inspect.Config.Labels) > 0 {
					labels = inspect.Config.Labels
				}
				m.dockerLabelsView(c, p, "Labels", labels)
			}
		})
	})
}

func (m *Model) dockerImageDetail(c *ui.Context, p *ui.Theme,
	node serviceTreeNode, image openidea.DockerImage) {
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	busy := m.dockerOperationBusy()
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(9).Children(func() {
			ui.Text(c, dockerImageRef(image)).FontSize(15).Bold().TextColor(p.Text)
			ui.Row(c).Gap(6).Children(func() {
				if ui.Button(c, "Show Layers").Clicked() {
					m.setDockerDetailTab(*connection, node, dockerDetailLayers)
				}
				if image.Repository != "" && image.Repository != "<none>" {
					pull := ui.Button(c, "Pull Again")
					pull.Disabled(busy)
					if pull.Clicked() {
						m.openDockerPullDialog(node.dockerConn, dockerImageRef(image))
					}
				}
				deleteButton := ui.Button(c, "Delete…")
				deleteButton.Disabled(busy)
				if deleteButton.Clicked() {
					m.confirmDockerRemove(node, "image", []string{image.ID})
				}
			})
			active := m.dockerDetailTabRow(c, p, *connection, node)
			m.dockerOperationStatus(c, p)
			switch active {
			case dockerDetailLayers:
				if m.services.docker.history[dockerDetailKey(connection.ID, "image", image.ID)] == nil {
					m.loadDockerHistory(*connection, image.ID)
				}
				m.dockerHistoryView(c, p, node, image.ID)
			case dockerDetailInspect:
				if m.services.docker.inspect[dockerDetailKey(connection.ID, "image", image.ID)] == nil {
					m.loadDockerInspect(*connection, "image", image.ID)
				}
				m.dockerInspectView(c, p, node, "image", image.ID)
			default:
				m.ensureDockerDashboardData(*connection, node)
				ui.Text(c, "Image Overview").FontSize(12).Bold().TextColor(p.Text)
				serviceFact(c, p, "Name", image.Repository+":"+image.Tag, true)
				dockerCopyableFact(c, p, "Image ID", dockerShortID(image.ID), dockerShortID(image.ID), true)
				serviceFact(c, p, "Repository", image.Repository, true)
				serviceFact(c, p, "Tag", image.Tag, true)
				serviceFact(c, p, "Size", image.Size, false)
				serviceFact(c, p, "Created", image.CreatedSince, false)
				m.dockerImageInspectDetails(c, p, node, image)
				tags := dockerImageTagsForID(m.services.docker.snapshots[connection.ID], image)
				if inspect := m.dockerImageInspect(connection.ID, image.ID); inspect != nil {
					tags = mergeDockerImageTags(tags, inspect.RepoTags)
				}
				if len(tags) > 0 {
					serviceFact(c, p, "Tags", strings.Join(tags, ", "), true)
				}
				ui.Text(c, "Containers Using Image").FontSize(12).Bold().TextColor(p.Text)
				if containers := dockerContainersUsingImage(m.services.docker.snapshots[connection.ID], image); len(containers) > 0 {
					names := make([]string, 0, len(containers))
					for _, container := range containers {
						names = append(names, container.Name)
					}
					serviceFact(c, p, "Containers Using Image", strings.Join(names, ", "), true)
				} else {
					ui.Text(c, "No containers are using this image").
						FontSize(11).TextColor(p.TextMuted)
				}
			}
		})
	})
}

func (m *Model) dockerVolumeDetail(c *ui.Context, p *ui.Theme,
	node serviceTreeNode, volume openidea.DockerVolume) {
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	busy := m.dockerOperationBusy()
	snapshot := m.services.docker.snapshots[connection.ID]
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(9).Children(func() {
			ui.Text(c, volume.Name).FontSize(15).Bold().TextColor(p.Text)
			ui.Row(c).Gap(6).Children(func() {
				deleteButton := ui.Button(c, "Delete…")
				deleteButton.Disabled(busy)
				if deleteButton.Clicked() {
					m.confirmDockerRemove(node, "volume", []string{volume.Name})
				}
			})
			active := m.dockerDetailTabRow(c, p, *connection, node)
			m.dockerOperationStatus(c, p)
			if active == dockerDetailInspect {
				if m.services.docker.inspect[dockerDetailKey(connection.ID, "volume", volume.Name)] == nil {
					m.loadDockerInspect(*connection, "volume", volume.Name)
				}
				m.dockerInspectView(c, p, node, "volume", volume.Name)
				return
			}
			m.ensureDockerDashboardData(*connection, node)
			ui.Text(c, "Volume Overview").FontSize(12).Bold().TextColor(p.Text)
			serviceFact(c, p, "Name", volume.Name, true)
			serviceFact(c, p, "Driver", volume.Driver, false)
			dockerCopyableFact(c, p, "Mountpoint", volume.Mountpoint, volume.Mountpoint, true)
			if volume.Scope != "" {
				serviceFact(c, p, "Scope", volume.Scope, false)
			}
			if volume.ComposeProject != nil && *volume.ComposeProject != "" {
				serviceFact(c, p, "Compose", *volume.ComposeProject, false)
			}
			ui.Text(c, "Containers Using Volume").FontSize(12).Bold().TextColor(p.Text)
			if containers := dockerContainersUsingVolume(snapshot, volume); len(containers) > 0 {
				serviceFact(c, p, "Containers Using Volume", dockerContainerNames(containers), true)
			} else {
				ui.Text(c, "No containers are using this volume").
					FontSize(11).TextColor(p.TextMuted)
			}
			m.dockerLabelsView(c, p, "Labels", volume.Labels)
		})
	})
}

func (m *Model) dockerNetworkDetail(c *ui.Context, p *ui.Theme,
	node serviceTreeNode, network openidea.DockerNetwork) {
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	busy := m.dockerOperationBusy()
	snapshot := m.services.docker.snapshots[connection.ID]
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22, 18).Gap(9).Children(func() {
			ui.Text(c, network.Name).FontSize(15).Bold().TextColor(p.Text)
			ui.Row(c).Gap(6).Children(func() {
				deleteButton := ui.Button(c, "Delete…")
				deleteButton.Disabled(busy)
				if deleteButton.Clicked() {
					m.confirmDockerRemove(node, "network", []string{network.ID})
				}
			})
			active := m.dockerDetailTabRow(c, p, *connection, node)
			m.dockerOperationStatus(c, p)
			if active == dockerDetailInspect {
				if m.services.docker.inspect[dockerDetailKey(connection.ID, "network", network.ID)] == nil {
					m.loadDockerInspect(*connection, "network", network.ID)
				}
				m.dockerInspectView(c, p, node, "network", network.ID)
				return
			}
			m.ensureDockerDashboardData(*connection, node)
			ui.Text(c, "Network Overview").FontSize(12).Bold().TextColor(p.Text)
			serviceFact(c, p, "Name", network.Name, true)
			dockerCopyableFact(c, p, "Network ID", dockerShortID(network.ID), network.ID, true)
			serviceFact(c, p, "Driver", network.Driver, false)
			serviceFact(c, p, "Scope", network.Scope, false)
			if network.CreatedAt != "" {
				serviceFact(c, p, "Created", network.CreatedAt, false)
			}
			if inspect := m.dockerNetworkInspect(node, network.ID); inspect != nil {
				if ipam := dockerNetworkIPAMText(inspect); ipam != "" {
					serviceFact(c, p, "Subnet / Gateway", ipam, true)
				}
			}
			serviceFact(c, p, "IPv6", boolText(network.IPv6), false)
			serviceFact(c, p, "Internal", boolText(network.Internal), false)
			if network.ComposeProject != nil && *network.ComposeProject != "" {
				serviceFact(c, p, "Compose project", *network.ComposeProject, false)
			}
			ui.Text(c, "Connected Containers").FontSize(12).Bold().TextColor(p.Text)
			if containers := dockerContainersInNetwork(snapshot, network); len(containers) > 0 {
				serviceFact(c, p, "Connected Containers", dockerContainerNames(containers), true)
				inspect := m.dockerNetworkInspect(node, network.ID)
				for _, container := range containers {
					if address := dockerNetworkContainerAddress(inspect, container.ID); address != "" {
						serviceFact(c, p, container.Name, address, true)
					}
				}
			} else {
				ui.Text(c, "No containers are connected to this network").
					FontSize(11).TextColor(p.TextMuted)
			}
			m.dockerLabelsView(c, p, "Labels", network.Labels)
		})
	})
}

func (m *Model) dockerStatsView(c *ui.Context, p *ui.Theme, node serviceTreeNode, container openidea.DockerContainer) {
	state := m.services.docker.stats[dockerDetailKey(node.dockerConn, "container", container.ID)]
	if state == nil {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Resource Usage").FontSize(12).Bold().TextColor(p.Text).Grow(1)
		refresh := ui.Button(c, "Refresh")
		refresh.Disabled(state.loading)
		if refresh.Clicked() {
			if connection := m.dockerConnection(node.dockerConn); connection != nil {
				m.loadDockerStats(*connection, container.ID)
			}
		}
	})
	if state.loading {
		ui.Text(c, "Loading…").FontSize(11).TextColor(p.TextMuted)
	}
	if state.err != "" {
		ui.Text(c, "Metrics unavailable: "+state.err).FontSize(11).TextColor(p.Danger)
		return
	}
	if state.loaded {
		stats := state.stats
		serviceFact(c, p, "CPU", stats.CPUPerc, true)
		serviceFact(c, p, "Memory", stats.MemUsage+" ("+stats.MemPerc+")", true)
		serviceFact(c, p, "Network I/O", stats.NetIO, true)
		serviceFact(c, p, "Block I/O", stats.BlockIO, true)
		serviceFact(c, p, "PIDs", stats.Pids, true)
	} else if !state.loading {
		ui.Text(c, "Metrics unavailable").FontSize(11).TextColor(p.TextMuted)
	}
}

func (m *Model) dockerProcessesView(c *ui.Context, p *ui.Theme, node serviceTreeNode, container openidea.DockerContainer) {
	state := m.services.docker.processes[dockerDetailKey(node.dockerConn, "container", container.ID)]
	if state == nil {
		return
	}
	title := "Processes"
	if state.loaded {
		title += fmt.Sprintf(" (%d)", len(state.list.Processes))
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(12).Bold().TextColor(p.Text).Grow(1)
		refresh := ui.Button(c, "Refresh")
		refresh.Disabled(state.loading)
		if refresh.Clicked() {
			if connection := m.dockerConnection(node.dockerConn); connection != nil {
				m.loadDockerProcesses(*connection, container.ID)
			}
		}
	})
	if state.loading {
		ui.Text(c, "Reading container processes…").FontSize(11).TextColor(p.TextMuted)
	}
	if state.err != "" {
		ui.Text(c, state.err).FontSize(11).TextColor(p.Danger)
	}
	if !state.loaded {
		return
	}
	if len(state.list.Processes) == 0 {
		ui.Text(c, "(No processes reported.)").FontSize(11).TextColor(p.TextMuted)
		return
	}
	lines := []string{strings.Join(state.list.Titles, "\t")}
	for _, row := range state.list.Processes {
		lines = append(lines, strings.Join(row, "\t"))
	}
	ui.Scroll(c).Height(180).Children(func() {
		ui.Text(c, strings.Join(lines, "\n")).FontSize(11).Font("monospace").TextColor(p.Text)
	})
}

func (m *Model) dockerHistoryView(c *ui.Context, p *ui.Theme, node serviceTreeNode, imageID string) {
	state := m.services.docker.history[dockerDetailKey(node.dockerConn, "image", imageID)]
	if state == nil {
		return
	}
	ui.Text(c, "Image Layers").FontSize(12).Bold().TextColor(p.Text)
	if state.loading {
		ui.Text(c, "Reading image history…").FontSize(11).TextColor(p.TextMuted)
	}
	if state.err != "" {
		ui.Text(c, state.err).FontSize(11).TextColor(p.Danger)
	}
	if !state.loaded {
		return
	}
	if len(state.layers) == 0 {
		ui.Text(c, "(No image layers reported.)").FontSize(11).TextColor(p.TextMuted)
		return
	}
	ui.Scroll(c).Height(220).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			for i, layer := range state.layers {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, fmt.Sprintf("%d", i+1)).FontSize(11).TextColor(p.TextMuted).Width(28)
					ui.Text(c, layer.Size).FontSize(11).Font("monospace").TextColor(p.Text).Width(80)
					ui.Text(c, layer.CreatedBy).FontSize(11).Font("monospace").
						TextColor(p.TextMuted).Grow(1).SingleLine().Ellipsis("…")
					ui.Text(c, layer.CreatedSince).FontSize(11).TextColor(p.TextMuted)
				})
			}
		})
	})
}

func (m *Model) dockerInspectView(c *ui.Context, p *ui.Theme, node serviceTreeNode, object, id string) {
	state := m.services.docker.inspect[dockerDetailKey(node.dockerConn, object, id)]
	if state == nil {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Inspect JSON").FontSize(12).Bold().TextColor(p.Text).Grow(1)
		if state.loading {
			ui.Text(c, "Loading…").FontSize(11).TextColor(p.TextMuted)
		}
		copyButton := ui.Button(c, "Copy")
		copyButton.Disabled(state.text == "")
		if copyButton.Clicked() {
			mygo.Clipboard.WriteText(state.text)
		}
		if ui.Button(c, "Refresh").Clicked() {
			if connection := m.dockerConnection(node.dockerConn); connection != nil {
				m.loadDockerInspect(*connection, object, id)
			}
		}
	})
	if state.err != "" {
		ui.Text(c, state.err).FontSize(11).TextColor(p.Danger)
		return
	}
	if state.text == "" {
		if state.loading {
			ui.Text(c, "Loading…").FontSize(11).TextColor(p.TextMuted)
		}
		return
	}
	ui.Scroll(c).Height(320).Children(func() {
		ui.Text(c, state.text).FontSize(11).Font("monospace").TextColor(p.TextMuted)
	})
}

func (m *Model) dockerOperationStatus(c *ui.Context, p *ui.Theme) {
	op := m.services.docker.operation
	if op == nil {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Text(c, op.title).FontSize(12).Bold().TextColor(p.Text)
		if op.request != 0 {
			if op.streaming && ui.Button(c, "Cancel").Clicked() {
				m.cancelDockerOperation()
			}
		} else {
			if ui.Button(c, "Clear").Clicked() {
				m.services.docker.operation = nil
				m.invalidate()
			}
		}
	})
	if op.err != "" {
		ui.Text(c, op.err).FontSize(12).TextColor(p.Danger)
	} else if op.stopped {
		ui.Text(c, "Cancelled").FontSize(12).TextColor(p.TextMuted)
	}
	if len(op.output) > 0 {
		ui.Scroll(c).Height(150).Children(func() {
			ui.Text(c, strings.Join(op.output, "\n")).FontSize(11).Font("monospace").
				TextColor(p.TextMuted)
		})
	}
}

func (m *Model) dockerLogView(c *ui.Context, p *ui.Theme, stream *dockerLogStream) {
	restart := false
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Log").FontSize(12).Bold().TextColor(p.Text)
		if stream.loading {
			ui.Text(c, "Loading…").FontSize(11).TextColor(p.TextMuted)
		}
		follow := stream.follow
		if ui.Checkbox(c, &follow, "Follow").Changed() {
			stream.follow = follow
			restart = true
		}
		timestamps := stream.timestamps
		if ui.Checkbox(c, &timestamps, "Timestamps").Changed() {
			stream.timestamps = timestamps
			restart = true
		}
		if stream.loading {
			if ui.Button(c, "Stop").Clicked() {
				m.stopDockerLogStream(stream)
				m.invalidate()
			}
		}
		if ui.Button(c, "Restart").Clicked() {
			restart = true
		}
		if ui.Button(c, "Clear").Clicked() {
			m.stopDockerLogStream(stream)
			delete(m.services.docker.logs, stream.key)
			m.invalidate()
		}
	})
	if stream.err != "" {
		ui.Text(c, stream.err).FontSize(12).TextColor(p.Danger)
	}
	ui.Scroll(c).Height(240).Children(func() {
		if len(stream.lines) == 0 {
			ui.Text(c, "(No log lines yet.)").FontSize(11).Font("monospace").TextColor(p.TextMuted)
			return
		}
		ui.RichText(c, dockerLogSpans(stream.lines, p)...).FontSize(11).Font("monospace")
	})
	if restart {
		if connection := m.dockerConnection(stream.connection); connection != nil {
			m.startDockerLogsWithOptions(*connection, serviceTreeNode{
				key: stream.key, dockerConn: stream.connection, dockerID: stream.container,
			}, stream.follow, stream.timestamps)
		}
	}
}

func dockerEndpointText(endpoint openidea.DockerEndpoint) string {
	switch endpoint.Kind {
	case "default":
		return "Docker CLI default"
	case "socket":
		return endpoint.Path
	case "tcp":
		if endpoint.TLSCertDir != nil && *endpoint.TLSCertDir != "" {
			return endpoint.Host + " (TLS)"
		}
		return endpoint.Host
	case "context":
		return "context " + endpoint.Name
	default:
		return endpoint.Kind
	}
}

func dockerContainerByID(snapshot openidea.DockerSnapshot, id string) *openidea.DockerContainer {
	for i := range snapshot.Containers {
		if snapshot.Containers[i].ID == id {
			return &snapshot.Containers[i]
		}
	}
	return nil
}

func dockerImageByID(snapshot openidea.DockerSnapshot, id string) *openidea.DockerImage {
	for i := range snapshot.Images {
		if snapshot.Images[i].ID == id {
			return &snapshot.Images[i]
		}
	}
	return nil
}

func dockerImageForNode(snapshot openidea.DockerSnapshot, node serviceTreeNode) *openidea.DockerImage {
	for i := range snapshot.Images {
		image := &snapshot.Images[i]
		if image.ID != node.dockerID {
			continue
		}
		if node.dockerRef == "" || dockerImageRef(*image) == node.dockerRef {
			return image
		}
	}
	return nil
}

func dockerVolumeByName(snapshot openidea.DockerSnapshot, name string) *openidea.DockerVolume {
	for i := range snapshot.Volumes {
		if snapshot.Volumes[i].Name == name {
			return &snapshot.Volumes[i]
		}
	}
	return nil
}

func dockerNetworkByID(snapshot openidea.DockerSnapshot, id string) *openidea.DockerNetwork {
	for i := range snapshot.Networks {
		if snapshot.Networks[i].ID == id {
			return &snapshot.Networks[i]
		}
	}
	return nil
}

func dockerComposeContainers(snapshot openidea.DockerSnapshot, project string) []openidea.DockerContainer {
	out := make([]openidea.DockerContainer, 0)
	for _, container := range snapshot.Containers {
		if container.ComposeProject != nil && *container.ComposeProject == project {
			out = append(out, container)
		}
	}
	return out
}

// dockerContextMenu exposes the same actions from the tree and the details pane.
func (m *Model) dockerContextMenu(menu *ui.Menu, node serviceTreeNode) {
	if node.nodeKind == "docker-registry" {
		for _, registry := range m.services.docker.settings.Registries {
			if registry.ID != node.dockerID {
				continue
			}
			if menu.Item("Edit Registry…").Chosen() {
				m.editDockerRegistry(&registry)
			}
			if menu.Item("Test Login").
				Disabled(strings.TrimSpace(registry.Username) == "").Chosen() {
				m.testDockerRegistryLogin(registry)
			}
			menu.Separator()
			if menu.Item("Remove Registry…").Chosen() {
				m.removeDockerRegistry(registry)
			}
		}
		return
	}
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	snapshot := m.services.docker.snapshots[connection.ID]
	busy := m.dockerOperationBusy()
	switch node.nodeKind {
	case "docker":
		if menu.Item("Refresh").Chosen() {
			m.refreshDockerSnapshot(*connection)
		}
		if menu.Item("Pull Image…").Disabled(busy).Chosen() {
			m.openDockerPullDialog(connection.ID, "")
		}
		menu.Submenu("View Options", func(submenu *ui.Menu) {
			m.dockerViewOptionsMenu(submenu, true, true)
		})
		menu.Submenu("Prune", func(submenu *ui.Menu) {
			m.dockerPruneMenu(submenu, *connection)
		})
		menu.Separator()
		if menu.Item("Edit Connection…").Chosen() {
			m.editDockerConnection(connection)
		}
		if menu.Item("Remove Connection…").Chosen() {
			m.removeDockerConnection(*connection)
		}
	case "docker-group":
		switch node.dockerID {
		case "containers", "devContainers":
			m.dockerViewOptionsMenu(menu, true, false)
		case "images":
			m.dockerViewOptionsMenu(menu, false, true)
			if menu.Item("Pull Image…").Disabled(busy).Chosen() {
				m.openDockerPullDialog(connection.ID, "")
			}
		}
		target := node.dockerID
		if target == "devContainers" {
			target = "containers"
		}
		if target == "containers" || target == "images" ||
			target == "networks" || target == "volumes" {
			if menu.Item(dockerPruneLabel(target) + "…").Disabled(busy).Chosen() {
				m.confirmDockerPrune(*connection, target)
			}
		}
	case "docker-compose":
		containers := dockerComposeContainers(snapshot, node.dockerID)
		running := false
		stopped := false
		for _, container := range containers {
			if dockerContainerRunning(container) {
				running = true
			} else {
				stopped = true
			}
		}
		if menu.Item("Start").Disabled(busy || !stopped).Chosen() {
			m.runDockerComposeOp(node, snapshot, "start")
		}
		if menu.Item("Stop").Disabled(busy || !running).Chosen() {
			m.runDockerComposeOp(node, snapshot, "stop")
		}
		if menu.Item("Restart").Disabled(busy).Chosen() {
			m.runDockerComposeOp(node, snapshot, "restart")
		}
		menu.Separator()
		if menu.Item("Compose Down").Disabled(busy).Chosen() {
			m.confirmDockerComposeDown(node, snapshot)
		}
	case "docker-container":
		container := dockerContainerByID(snapshot, node.dockerID)
		if container == nil {
			return
		}
		running := dockerContainerRunning(*container)
		if menu.Item("Start").Disabled(busy || running).Chosen() {
			m.runDockerContainerOp(node, "start")
		}
		if menu.Item("Stop").Disabled(busy || !running).Chosen() {
			m.runDockerContainerOp(node, "stop")
		}
		if menu.Item("Restart").Disabled(busy).Chosen() {
			m.runDockerContainerOp(node, "restart")
		}
		if container.State == "paused" {
			if menu.Item("Unpause").Disabled(busy).Chosen() {
				m.runDockerContainerOp(node, "unpause")
			}
		} else if menu.Item("Pause").Disabled(busy || !running).Chosen() {
			m.runDockerContainerOp(node, "pause")
		}
		menu.Separator()
		if menu.Item("Dashboard").Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailDashboard)
		}
		if menu.Item("Show Log").Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailLog)
		}
		if menu.Item("Open Log in Editor").Chosen() {
			m.openDockerLogEditor(*connection, *container)
		}
		if menu.Item("Inspect").Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailInspect)
		}
		if menu.Item("Show Processes").Disabled(!running).Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailProcesses)
		}
		if menu.Item("Create Terminal").Disabled(!running).Chosen() {
			m.openDockerTerminal(*connection, *container)
		}
		menu.Separator()
		if menu.Item("Copy Container ID").Chosen() {
			mygo.Clipboard.WriteText(container.ID)
		}
		if menu.Item("Copy Image Name").Chosen() {
			mygo.Clipboard.WriteText(container.Image)
		}
		menu.Separator()
		if menu.Item("Delete…").Disabled(busy).Chosen() {
			m.confirmDockerRemove(node, "container", []string{node.dockerID})
		}
	case "docker-image":
		image := dockerImageForNode(snapshot, node)
		if image != nil {
			if menu.Item("Dashboard").Chosen() {
				m.setDockerDetailTab(*connection, node, dockerDetailDashboard)
			}
			if menu.Item("Show Layers").Chosen() {
				m.setDockerDetailTab(*connection, node, dockerDetailLayers)
			}
			if menu.Item("Inspect").Chosen() {
				m.setDockerDetailTab(*connection, node, dockerDetailInspect)
			}
			if image.Repository != "" && image.Repository != "<none>" {
				if menu.Item("Pull Again").Disabled(busy).Chosen() {
					m.openDockerPullDialog(connection.ID, dockerImageRef(*image))
				}
			}
			if menu.Item("Copy Image ID").Chosen() {
				mygo.Clipboard.WriteText(dockerShortID(image.ID))
			}
			menu.Separator()
		}
		if menu.Item("Delete…").Disabled(busy).Chosen() {
			m.confirmDockerRemove(node, "image", []string{node.dockerID})
		}
	case "docker-volume":
		if menu.Item("Dashboard").Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailDashboard)
		}
		if menu.Item("Inspect").Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailInspect)
		}
		menu.Separator()
		if menu.Item("Delete…").Disabled(busy).Chosen() {
			m.confirmDockerRemove(node, "volume", []string{node.dockerID})
		}
	case "docker-network":
		if menu.Item("Dashboard").Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailDashboard)
		}
		if menu.Item("Inspect").Chosen() {
			m.setDockerDetailTab(*connection, node, dockerDetailInspect)
		}
		menu.Separator()
		if menu.Item("Delete…").Disabled(busy).Chosen() {
			m.confirmDockerRemove(node, "network", []string{node.dockerID})
		}
	}
}

func (m *Model) dockerViewOptionsMenu(menu *ui.Menu, stopped, untagged bool) {
	filters := m.dockerFilters()
	if stopped {
		show := filters.showStoppedContainers()
		if menu.Item("Stopped Containers").Checked(show).Chosen() {
			next := !show
			m.setDockerFilter(&next, nil)
		}
	}
	if untagged {
		show := filters.showUntaggedImages()
		if menu.Item("Untagged Images").Checked(show).Chosen() {
			next := !show
			m.setDockerFilter(nil, &next)
		}
	}
}

func dockerPruneLabel(target string) string {
	switch target {
	case "containers":
		return "Prune Unused Containers"
	case "images":
		return "Prune Dangling Images"
	case "allImages":
		return "Prune Unused Images"
	case "volumes":
		return "Prune Unused Volumes"
	case "networks":
		return "Prune Unused Networks"
	default:
		return "Prune Everything Unused"
	}
}

func (m *Model) dockerPruneMenu(menu *ui.Menu, connection openidea.DockerConnection) {
	busy := m.dockerOperationBusy()
	for _, target := range []string{"containers", "images", "allImages", "volumes", "networks", "system"} {
		target := target
		if menu.Item(dockerPruneLabel(target) + "…").Disabled(busy).Chosen() {
			m.confirmDockerPrune(connection, target)
		}
	}
}

func (m *Model) confirmDockerPrune(connection openidea.DockerConnection, target string) {
	type prompt struct {
		title, message string
	}
	prompts := map[string]prompt{
		"containers": {
			"Prune Stopped Containers",
			"Remove all stopped containers?",
		},
		"images": {
			"Prune Dangling Images",
			"Remove all dangling (untagged) images?",
		},
		"allImages": {
			"Prune Unused Images",
			"Remove all images not used by a container?",
		},
		"networks": {
			"Prune Unused Networks",
			"Remove all networks not used by a container?",
		},
		"volumes": {
			"Prune Unused Volumes",
			"Remove all anonymous volumes not used by a container? Their data is lost.",
		},
		"system": {
			"Prune Everything Unused (System Prune)",
			"Remove stopped containers, unused networks, dangling images and the build cache?",
		},
	}
	entry, ok := prompts[target]
	if !ok {
		return
	}
	m.confirmDockerAction(entry.title, entry.message, "Prune",
		func() { m.runDockerPrune(connection, target) })
}
