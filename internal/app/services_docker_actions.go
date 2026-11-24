package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// runDockerAction keeps the operation identity stable while RPC callbacks arrive.
func (m *Model) runDockerAction(connection openidea.DockerConnection, title, command string, args map[string]any, streaming bool) bool {
	if m.dockerOperationBusy() {
		m.notifyInfo("docker-operation", "Another Docker operation is still running", "")
		return false
	}
	current := m.dockerConnection(connection.ID)
	if current == nil {
		return false
	}
	connection = *current
	epoch := m.dockerEpoch()
	m.clearServiceProviderError(dockerProviderID)
	op := &dockerOperation{
		connectionID: connection.ID,
		title:        title,
		request:      m.nextRequest(),
		streaming:    streaming,
	}
	if streaming {
		op.channel = m.nextChannel()
		args["request"] = op.request
		args["onOutput"] = map[string]any{"$channel": op.channel}
	}
	args["endpoint"] = connection.Endpoint
	m.services.docker.operation = op
	ctx, cancel := context.WithTimeout(m.dockerContext(), 10*time.Minute)
	op.cancel = cancel
	m.invalidate()
	go func() {
		defer cancel()
		_, err := m.invoke(ctx, command, args)
		m.post(func() {
			if m.services.docker.epoch != epoch {
				return
			}
			if m.services.docker.operation == op {
				op.request = 0
				op.channel = 0
				op.cancel = nil
				if !op.stopped && err != nil {
					op.err = err.Error()
					m.setServiceProviderError(dockerProviderID, "Docker operation failed: "+err.Error())
				}
				m.invalidate()
			}
			m.refreshDockerSnapshot(connection)
		})
	}()
	return true
}

func (m *Model) runDockerPrune(connection openidea.DockerConnection, target string) {
	title := map[string]string{
		"containers": "Pruning stopped containers…",
		"images":     "Pruning dangling images…",
		"allImages":  "Pruning unused images…",
		"volumes":    "Pruning unused volumes…",
		"networks":   "Pruning unused networks…",
		"system":     "Pruning the system…",
	}[target]
	m.runDockerAction(connection, title, "docker_prune", map[string]any{"target": target}, true)
}

func (m *Model) cancelDockerOperation() {
	op := m.services.docker.operation
	if op == nil || op.request == 0 {
		return
	}
	request := op.request
	op.stopped = true
	op.request = 0
	op.channel = 0
	op.connectionID = ""
	if op.cancel != nil {
		op.cancel()
		op.cancel = nil
	}
	m.cancelDockerRequest(request)
	m.invalidate()
}

func (m *Model) runDockerComposeOp(node serviceTreeNode, snapshot openidea.DockerSnapshot, op string) {
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	containers := dockerComposeContainers(snapshot, node.dockerID)
	var files []string
	var workingDir *string
	for _, c := range containers {
		if len(files) == 0 && len(c.ComposeFiles) > 0 {
			files = append([]string{}, c.ComposeFiles...)
		}
		if workingDir == nil && c.ComposeWorkingDir != nil {
			dir := *c.ComposeWorkingDir
			workingDir = &dir
		}
	}
	if files == nil {
		files = []string{}
	}
	title := map[string]string{
		"start":   "Starting Compose project " + node.dockerID + "…",
		"stop":    "Stopping Compose project " + node.dockerID + "…",
		"restart": "Restarting Compose project " + node.dockerID + "…",
		"down":    "Taking down Compose project " + node.dockerID + "…",
	}[op]
	m.runDockerAction(*connection, title, "docker_compose_op",
		map[string]any{"project": node.dockerID, "files": files, "workingDir": workingDir, "op": op}, true)
}

func (m *Model) confirmDockerComposeDown(node serviceTreeNode, snapshot openidea.DockerSnapshot) {
	m.confirmDockerAction("Compose Down",
		"Stop and remove the containers and networks of '"+node.dockerID+"'?",
		"Down", func() { m.runDockerComposeOp(node, snapshot, "down") })
}

func (m *Model) runDockerContainerOp(node serviceTreeNode, op string) {
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	m.runDockerContainerOpIDs(*connection, []string{node.dockerID}, op)
}

func (m *Model) runDockerContainerOpIDs(connection openidea.DockerConnection, ids []string, op string) {
	ids = append([]string(nil), ids...)
	if len(ids) == 0 {
		return
	}
	title := map[string]string{
		"start":   "Starting container…",
		"stop":    "Stopping container…",
		"restart": "Restarting container…",
		"pause":   "Pausing container…",
		"unpause": "Unpausing container…",
		"kill":    "Killing container…",
	}[op]
	if title == "" {
		title = "Changing container…"
	}
	if len(ids) > 1 {
		title = strings.Replace(title, "container", "containers", 1)
	}
	m.runDockerAction(connection, title, "docker_container_op",
		map[string]any{"ids": ids, "op": op}, false)
}

func (m *Model) confirmDockerRemove(node serviceTreeNode, object string, ids []string) {
	ids = append([]string(nil), ids...)
	if len(ids) == 0 || m.dockerConnection(node.dockerConn) == nil {
		return
	}
	label := node.label
	if label == "" {
		label = dockerShortID(ids[0])
	}
	title := "Delete Docker " + object
	message := "Delete " + object + " '" + label + "'?"
	switch object {
	case "container":
		title = "Delete Container"
		if node.live {
			message = "Delete container '" + label + "'? It is running and will be stopped."
		} else {
			message = "Delete container '" + label + "'?"
		}
	case "image":
		title = "Delete Image"
		message = "Delete image '" + label + "'?"
	case "volume":
		title = "Delete Volume"
		message = "Delete volume '" + label + "'?"
	case "network":
		title = "Delete Network"
		message = "Delete network '" + label + "'?"
	default:
		return
	}
	m.services.docker.dialog = &dockerDialog{
		kind:         "remove",
		connectionID: node.dockerConn,
		object:       object,
		ids:          ids,
		title:        title,
		message:      message,
		button:       "Delete",
		force:        false,
	}
	m.invalidate()
}

func (m *Model) confirmDockerAction(title, message, button string, action func()) {
	m.services.docker.dialog = &dockerDialog{
		kind: "confirm", title: title, message: message,
		button: button, action: action,
	}
	m.invalidate()
}

func (m *Model) startDockerLogs(connection openidea.DockerConnection, node serviceTreeNode) {
	m.startDockerLogsWithOptions(connection, node, node.live, false)
}

func (m *Model) startDockerLogsWithOptions(connection openidea.DockerConnection, node serviceTreeNode, follow, timestamps bool) {
	d := &m.services.docker
	current := m.dockerConnection(connection.ID)
	if current == nil {
		return
	}
	connection = *current
	epoch := m.dockerEpoch()
	if d.logs == nil {
		d.logs = map[string]*dockerLogStream{}
	}
	for key, existing := range d.logs {
		if key == node.key || existing == nil || existing.editor != nil {
			continue
		}
		delete(d.logs, key)
		m.stopDockerLogStream(existing)
	}
	if previous := d.logs[node.key]; previous != nil {
		m.stopDockerLogStream(previous)
	}
	stream := &dockerLogStream{
		key: node.key, connection: node.dockerConn, container: node.dockerID,
		sourceLive: follow,
		request:    m.nextRequest(), channel: m.nextChannel(),
		loading: true, follow: follow, timestamps: timestamps,
	}
	if snapshot, ok := d.snapshots[connection.ID]; ok {
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			stream.sourceLive = dockerContainerRunning(*container)
		}
	}
	d.logs[node.key] = stream
	var ctx context.Context
	var cancel context.CancelFunc
	if follow {
		ctx, cancel = context.WithCancel(m.dockerContext())
	} else {
		ctx, cancel = context.WithTimeout(m.dockerContext(), 60*time.Second)
	}
	stream.cancel = cancel
	m.invalidate()
	go func() {
		defer cancel()
		_, err := m.invoke(ctx, "docker_logs", map[string]any{
			"request": stream.request, "endpoint": connection.Endpoint, "id": node.dockerID,
			"tail": 1000, "follow": follow, "timestamps": timestamps,
			"onLine": map[string]any{"$channel": stream.channel},
		})
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.logs[node.key] != stream {
				return
			}
			stream.loading = false
			stream.request = 0
			stream.channel = 0
			stream.cancel = nil
			if !stream.stopped && err != nil {
				stream.err = err.Error()
			}
			m.syncDockerLogEditor(stream)
			m.invalidate()
		})
	}()
}

func (m *Model) ensureDockerDetailLog(
	connection openidea.DockerConnection,
	node serviceTreeNode,
	live bool,
) *dockerLogStream {
	stream := m.services.docker.logs[node.key]
	if stream != nil && stream.editor == nil && stream.sourceLive != live {
		delete(m.services.docker.logs, node.key)
		m.stopDockerLogStream(stream)
		stream = nil
	}
	if stream == nil {
		m.startDockerLogsWithOptions(connection, node, live, false)
		stream = m.services.docker.logs[node.key]
	}
	return stream
}

func (m *Model) stopDockerLogStream(stream *dockerLogStream) {
	if stream == nil || !stream.loading {
		return
	}
	request := stream.request
	stream.stopped = true
	stream.request = 0
	stream.channel = 0
	if stream.cancel != nil {
		stream.cancel()
		stream.cancel = nil
	}
	m.cancelDockerRequest(request)
	stream.loading = false
}

func (m *Model) startDockerEvents(connection openidea.DockerConnection) {
	d := &m.services.docker
	if !d.watching || connection.ID == "" {
		return
	}
	current := m.dockerConnection(connection.ID)
	if current == nil {
		return
	}
	connection = *current
	epoch := m.dockerEpoch()
	if d.events == nil {
		d.events = map[string]*dockerEventStream{}
	}
	if d.events[connection.ID] != nil {
		return
	}
	ctx, cancel := context.WithCancel(m.dockerContext())
	stream := &dockerEventStream{request: m.nextRequest(), channel: m.nextChannel(), cancel: cancel}
	d.events[connection.ID] = stream
	go func() {
		defer cancel()
		_, err := m.invoke(ctx, "docker_events", map[string]any{
			"request": stream.request, "endpoint": connection.Endpoint,
			"onEvent": map[string]any{"$channel": stream.channel},
		})
		unexpected := err != nil && ctx.Err() == nil
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.events[connection.ID] != stream {
				return
			}
			delete(d.events, connection.ID)
			stream.request = 0
			stream.channel = 0
			stream.cancel = nil
			if unexpected {
				stream.err = err.Error()
				m.setServiceProviderError(dockerProviderID,
					"Docker events for "+connection.Name+" stopped: "+err.Error())
			}
			if d.watching {
				m.refreshDockerSnapshot(connection)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) stopDockerEventStream(stream *dockerEventStream) {
	if stream == nil {
		return
	}
	request := stream.request
	stream.request = 0
	stream.channel = 0
	if stream.cancel != nil {
		stream.cancel()
		stream.cancel = nil
	}
	m.cancelDockerRequest(request)
}

func (m *Model) cancelDockerRequest(request uint32) {
	if request == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 2*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

func dockerDetailKey(connectionID, object, id string) string {
	return connectionID + "\x00" + object + "\x00" + id
}

func (m *Model) loadDockerInspect(connection openidea.DockerConnection, object, id string) {
	d := &m.services.docker
	if object == "" || id == "" {
		return
	}
	if d.inspect == nil {
		d.inspect = map[string]*dockerInspectState{}
	}
	key := dockerDetailKey(connection.ID, object, id)
	if state := d.inspect[key]; state != nil && state.loading {
		return
	}
	epoch := m.dockerEpoch()
	state := &dockerInspectState{loading: true}
	d.inspect[key] = state
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "docker_inspect", map[string]any{
			"endpoint": connection.Endpoint, "object": object, "id": id,
		})
		var text string
		if err == nil {
			err = json.Unmarshal(raw, &text)
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.inspect[key] != state {
				return
			}
			state.loading = false
			if err != nil {
				state.err = err.Error()
			} else {
				state.text = dockerBoundText(text, 1024*1024)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadDockerStats(connection openidea.DockerConnection, id string) {
	d := &m.services.docker
	if d.stats == nil {
		d.stats = map[string]*dockerStatsState{}
	}
	key := dockerDetailKey(connection.ID, "container", id)
	if state := d.stats[key]; state != nil && state.loading {
		return
	}
	epoch := m.dockerEpoch()
	state := &dockerStatsState{loading: true}
	d.stats[key] = state
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "docker_container_stats", map[string]any{
			"endpoint": connection.Endpoint, "id": id,
		})
		var stats openidea.DockerContainerStats
		if err == nil {
			err = json.Unmarshal(raw, &stats)
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.stats[key] != state {
				return
			}
			state.loading = false
			if err != nil {
				state.err = err.Error()
			} else {
				state.stats, state.loaded = stats, true
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadDockerProcesses(connection openidea.DockerConnection, id string) {
	d := &m.services.docker
	if d.processes == nil {
		d.processes = map[string]*dockerProcessState{}
	}
	key := dockerDetailKey(connection.ID, "container", id)
	if state := d.processes[key]; state != nil && state.loading {
		return
	}
	epoch := m.dockerEpoch()
	state := &dockerProcessState{loading: true}
	d.processes[key] = state
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "docker_container_top", map[string]any{
			"endpoint": connection.Endpoint, "id": id,
		})
		var list openidea.DockerProcessList
		if err == nil {
			err = json.Unmarshal(raw, &list)
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.processes[key] != state {
				return
			}
			state.loading = false
			if err != nil {
				state.err = err.Error()
			} else {
				state.list, state.loaded = list, true
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadDockerHistory(connection openidea.DockerConnection, id string) {
	d := &m.services.docker
	if d.history == nil {
		d.history = map[string]*dockerHistoryState{}
	}
	key := dockerDetailKey(connection.ID, "image", id)
	if state := d.history[key]; state != nil && state.loading {
		return
	}
	epoch := m.dockerEpoch()
	state := &dockerHistoryState{loading: true}
	d.history[key] = state
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "docker_image_history", map[string]any{
			"endpoint": connection.Endpoint, "id": id,
		})
		var layers []openidea.DockerImageLayer
		if err == nil {
			err = json.Unmarshal(raw, &layers)
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.history[key] != state {
				return
			}
			state.loading = false
			if err != nil {
				state.err = err.Error()
			} else {
				state.layers, state.loaded = layers, true
			}
			m.invalidate()
		})
	}()
}

func dockerBoundText(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "\n… output truncated by GoIDEA"
}

func (m *Model) openDockerPullDialog(connectionID, reference string) {
	m.services.docker.dialog = &dockerDialog{
		kind: "pull", connectionID: connectionID, title: "Pull Docker Image",
		message: "Pull through the selected Docker connection.", button: "Pull",
		value: reference, reference: reference,
	}
	m.invalidate()
}

func (m *Model) submitDockerDialog() {
	d := &m.services.docker
	dialog := d.dialog
	if dialog == nil || dialog.busy {
		return
	}
	if dialog.kind == "confirm" {
		action := dialog.action
		d.dialog = nil
		if action != nil {
			action()
		}
		m.invalidate()
		return
	}
	connection := m.dockerConnection(dialog.connectionID)
	if connection == nil {
		dialog.err = "The Docker connection no longer exists."
		return
	}
	switch dialog.kind {
	case "pull":
		reference := strings.TrimSpace(dialog.value)
		if reference == "" || strings.HasPrefix(reference, "-") {
			dialog.err = "Enter an image name, for example postgres:16."
			return
		}
		if !m.runDockerAction(*connection, "Pulling "+reference, "docker_pull",
			map[string]any{"reference": reference}, true) {
			dialog.err = "Another Docker operation is still running."
			return
		}
		d.dialog = nil
	case "remove":
		force := dialog.force
		if dialog.object == "container" && containsDockerRunningSelection(connection.ID, dialog.ids, m.services.docker.snapshots[connection.ID]) {
			force = true
		}
		if !m.runDockerAction(*connection, "Remove "+dialog.object, "docker_remove",
			map[string]any{"object": dialog.object, "ids": append([]string(nil), dialog.ids...), "force": force}, false) {
			dialog.err = "Another Docker operation is still running."
			return
		}
		d.dialog = nil
	}
	m.invalidate()
}

func containsDockerRunningSelection(connectionID string, ids []string, snapshot openidea.DockerSnapshot) bool {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	for _, container := range snapshot.Containers {
		if wanted[container.ID] && dockerContainerRunning(container) {
			return true
		}
	}
	return false
}

func (m *Model) openDockerTerminal(connection openidea.DockerConnection, container openidea.DockerContainer) {
	if !dockerContainerRunning(container) {
		return
	}
	snapshot := m.services.docker.snapshots[connection.ID]
	program := "docker"
	if snapshot.Executable != nil && strings.TrimSpace(*snapshot.Executable) != "" {
		program = *snapshot.Executable
	}
	args := append([]string(nil), snapshot.GlobalArgs...)
	args = append(args, "exec", "-it", container.ID, "sh", "-c",
		"command -v bash >/dev/null 2>&1 && exec bash || exec sh")
	m.OpenTerminalCommand(program, args, container.Name, "")
}

func (m *Model) dockerRegistryEndpoint(registryID string) (openidea.DockerConnection, string) {
	login := m.services.docker.logins[registryID]
	if login != nil && login.connectionSelected {
		if login.connectionID != "" {
			if connection := m.dockerConnection(login.connectionID); connection != nil {
				return *connection, connection.ID
			}
		}
		return openidea.DockerConnection{
			ID: "default", Name: "Docker CLI default",
			Endpoint: openidea.DockerEndpoint{Kind: "default"},
		}, ""
	}
	for _, connection := range m.services.docker.settings.Connections {
		return connection, connection.ID
	}
	return openidea.DockerConnection{
		ID: "default", Name: "Docker CLI default",
		Endpoint: openidea.DockerEndpoint{Kind: "default"},
	}, ""
}

func (m *Model) testDockerRegistryLogin(registry openidea.DockerRegistry) {
	m.testDockerRegistryLoginWithPassword(registry, nil)
}

func (m *Model) testDockerRegistryLoginWithPassword(registry openidea.DockerRegistry, password *string) {
	d := &m.services.docker
	if d.logins == nil {
		d.logins = map[string]*dockerRegistryLoginState{}
	}
	login := d.logins[registry.ID]
	if login == nil {
		login = &dockerRegistryLoginState{}
		d.logins[registry.ID] = login
	}
	if login.loading {
		return
	}
	if !login.connectionSelected {
		_, connectionID := m.dockerRegistryEndpoint(registry.ID)
		login.connectionID = connectionID
		login.connectionSelected = true
	}
	connection, connectionID := m.dockerRegistryEndpoint(registry.ID)
	login.connectionID = connectionID
	login.loading = true
	login.answer, login.err = "", ""
	epoch := m.dockerEpoch()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 60*time.Second)
		defer cancel()
		var passwordValue any
		if password != nil && *password != "" {
			passwordValue = *password
		}
		raw, err := m.invoke(ctx, "docker_registry_login", map[string]any{
			"endpoint": connection.Endpoint, "registry": registry, "password": passwordValue,
		})
		var answer string
		if err == nil {
			err = json.Unmarshal(raw, &answer)
		}
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			if d.logins[registry.ID] != login {
				return
			}
			login.loading = false
			if err != nil {
				login.err = dockerRegistryError(err, password)
				m.notifyError("docker-login", "Cannot log in to "+registry.Name, login.err)
			} else {
				login.answer = answer
				m.notifyInfo("docker-login", registry.Name+": "+answer, "")
			}
			m.invalidate()
		})
	}()
}

func dockerRegistryError(err error, password *string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if password != nil && *password != "" {
		message = strings.ReplaceAll(message, *password, "[redacted]")
	}
	return message
}

func (m *Model) saveDockerSettingsRemoval(settings openidea.DockerSettings, connectionID string, after func()) {
	d := &m.services.docker
	if d.settingsUI.saving {
		return
	}
	d.settingsUI.saving = true
	d.settingsUI.err = ""
	epoch := m.dockerEpoch()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "docker_save_settings", map[string]any{"settings": settings})
		m.post(func() {
			if d.epoch != epoch {
				return
			}
			d.settingsUI.saving = false
			if err != nil {
				d.settingsUI.err = err.Error()
				m.notifyError("docker-settings", "Cannot save Docker settings", err.Error())
				m.invalidate()
				return
			}
			d.settings = settings
			delete(d.errors, "settings")
			m.pruneDockerState(settings)
			if connectionID != "" {
				m.stopDockerConnection(connectionID)
			}
			if after != nil {
				after()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) stopDockerConnection(connectionID string) {
	d := &m.services.docker
	if op := d.operation; op != nil && op.connectionID == connectionID {
		m.cancelDockerOperation()
	}
	if stream := d.events[connectionID]; stream != nil {
		delete(d.events, connectionID)
		m.stopDockerEventStream(stream)
	}
	for key, stream := range d.logs {
		if stream != nil && stream.connection == connectionID {
			delete(d.logs, key)
			m.stopDockerLogStream(stream)
		}
	}
	delete(d.snapshots, connectionID)
	delete(d.errors, connectionID)
	delete(d.refreshing, connectionID)
	delete(d.refresh, connectionID)
	delete(d.refreshGen, connectionID)
	prefix := connectionID + "\x00"
	for key := range d.inspect {
		if strings.HasPrefix(key, prefix) {
			delete(d.inspect, key)
		}
	}
	for key := range d.stats {
		if strings.HasPrefix(key, prefix) {
			delete(d.stats, key)
		}
	}
	for key := range d.processes {
		if strings.HasPrefix(key, prefix) {
			delete(d.processes, key)
		}
	}
	for key := range d.history {
		if strings.HasPrefix(key, prefix) {
			delete(d.history, key)
		}
	}
	for _, login := range d.logins {
		if login != nil && login.connectionSelected && login.connectionID == connectionID {
			login.connectionID = ""
			login.connectionSelected = false
			login.answer, login.err = "", ""
		}
	}
}

func (m *Model) removeDockerConnection(connection openidea.DockerConnection) {
	m.confirmDockerAction("Remove Docker Connection",
		"Remove the connection '"+connection.Name+"' from Services?", "Remove", func() {
			settings := openidea.DockerSettings{
				Connections: append([]openidea.DockerConnection{}, m.services.docker.settings.Connections...),
				Registries:  append([]openidea.DockerRegistry{}, m.services.docker.settings.Registries...),
			}
			kept := settings.Connections[:0]
			for _, candidate := range settings.Connections {
				if candidate.ID != connection.ID {
					kept = append(kept, candidate)
				}
			}
			settings.Connections = kept
			m.saveDockerSettingsRemoval(settings, connection.ID, func() {
				if strings.HasPrefix(m.services.selected, dockerConnectionKey(connection.ID)) {
					m.services.selected = ""
				}
			})
		})
}

func (m *Model) removeDockerRegistry(registry openidea.DockerRegistry) {
	m.confirmDockerAction("Remove Docker Registry",
		"Remove the registry '"+registry.Name+"' and its saved password?", "Remove", func() {
			settings := openidea.DockerSettings{
				Connections: append([]openidea.DockerConnection{}, m.services.docker.settings.Connections...),
				Registries:  append([]openidea.DockerRegistry{}, m.services.docker.settings.Registries...),
			}
			kept := settings.Registries[:0]
			for _, candidate := range settings.Registries {
				if candidate.ID != registry.ID {
					kept = append(kept, candidate)
				}
			}
			settings.Registries = kept
			m.saveDockerSettingsRemoval(settings, "", func() {
				delete(m.services.docker.logins, registry.ID)
				if m.services.selected == "docker-registry:"+registry.ID {
					m.services.selected = ""
				}
			})
		})
}

func dockerAppendBounded(lines []string, text string) []string {
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if len(line) > 16384 {
			line = line[:16384]
		}
		lines = append(lines, line)
	}
	if len(lines) > dockerMaxLogLines {
		lines = append([]string(nil), lines[len(lines)-dockerMaxLogLines:]...)
	}
	total := 0
	start := len(lines)
	for start > 0 {
		next := total + len(lines[start-1]) + 1
		if next > dockerMaxBytes {
			break
		}
		total = next
		start--
	}
	if start > 0 {
		lines = append([]string(nil), lines[start:]...)
	}
	return lines
}

func dockerAppendLogBounded(
	lines []openidea.DockerLogLine,
	chunk openidea.DockerLogLine,
) []openidea.DockerLogLine {
	for _, text := range strings.Split(strings.TrimSuffix(chunk.Text, "\n"), "\n") {
		if len(text) > 16384 {
			text = text[:16384]
		}
		lines = append(lines, openidea.DockerLogLine{Stderr: chunk.Stderr, Text: text})
	}
	if len(lines) > dockerMaxLogLines {
		lines = append([]openidea.DockerLogLine(nil), lines[len(lines)-dockerMaxLogLines:]...)
	}
	total := 0
	start := len(lines)
	for start > 0 {
		next := total + len(lines[start-1].Text) + 1
		if next > dockerMaxBytes {
			break
		}
		total = next
		start--
	}
	if start > 0 {
		lines = append([]openidea.DockerLogLine(nil), lines[start:]...)
	}
	return lines
}

func (m *Model) acceptDockerChunk(id uint32, data json.RawMessage) bool {
	if id == 0 {
		return false
	}
	if op := m.services.docker.operation; op != nil && op.channel == id {
		var line string
		if json.Unmarshal(data, &line) == nil {
			op.output = dockerAppendBounded(op.output, line)
			m.invalidate()
		}
		return true
	}
	for _, stream := range m.services.docker.logs {
		if stream.channel == id {
			var line openidea.DockerLogLine
			if json.Unmarshal(data, &line) == nil {
				stream.lines = dockerAppendLogBounded(stream.lines, line)
				m.syncDockerLogEditor(stream)
				m.invalidate()
			}
			return true
		}
	}
	for connectionID, stream := range m.services.docker.events {
		if stream.channel != id {
			continue
		}
		var event openidea.DockerEvent
		if json.Unmarshal(data, &event) == nil && event.Object != "" {
			stream.events = append(stream.events, event)
			if len(stream.events) > 100 {
				stream.events = append([]openidea.DockerEvent(nil), stream.events[len(stream.events)-100:]...)
			}
			if connection := m.dockerConnection(connectionID); connection != nil {
				m.refreshDockerSnapshot(*connection)
			}
			m.invalidate()
		}
		return true
	}
	return false
}
