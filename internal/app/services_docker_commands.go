package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// dockerServicesCommands registers the action ids contributed by OpenIDEA's
// Services feature. The tree, details pane, toolbar and command palette all
// execute these entries.
func dockerServicesCommands() []appCommand {
	return []appCommand{
		{
			"docker.setComposeFileOverride", "Set Docker Compose File Detection", "Docker", "",
			"compose yaml detect kept excluded",
			func(*Model) {},
		},
		{
			"docker.refresh", "Refresh Docker", "Docker", "",
			"reload update docker services containers images",
			func(*Model) {},
		},
		{
			"docker.start", "Start Container", "Docker", "",
			"run docker compose start container",
			func(*Model) {},
		},
		{
			"docker.stop", "Stop Container", "Docker", "",
			"stop docker compose container",
			func(*Model) {},
		},
		{
			"docker.restart", "Restart Container", "Docker", "",
			"rerun restart docker compose container",
			func(*Model) {},
		},
		{
			"docker.remove", "Delete Docker Object…", "Docker", "",
			"remove delete docker container compose image volume network",
			func(*Model) {},
		},
		{
			"docker.showDashboard", "Show Docker Dashboard", "Docker", "",
			"docker details services dashboard",
			func(*Model) {},
		},
		{
			"docker.showLog", "Show Container Log", "Docker", "",
			"docker container logs output",
			func(*Model) {},
		},
		{
			"docker.openLogInEditor", "Open Container Log in Editor", "Docker", "",
			"docker container logs output editor tab",
			func(*Model) {},
		},
		{
			"docker.showProcesses", "Show Container Processes", "Docker", "",
			"docker container top processes",
			func(*Model) {},
		},
		{
			"docker.showLayers", "Show Image Layers", "Docker", "",
			"docker image history layers",
			func(*Model) {},
		},
		{
			"docker.inspect", "Inspect Docker Object", "Docker", "",
			"docker inspect details json",
			func(*Model) {},
		},
		{
			"docker.toggleStoppedContainers", "Toggle Stopped Containers", "Docker", "",
			"docker filter stopped hidden containers",
			func(*Model) {},
		},
		{
			"docker.toggleUntaggedImages", "Toggle Untagged Images", "Docker", "",
			"docker filter untagged dangling images",
			func(*Model) {},
		},
		{
			"docker.execShell", "Create Terminal in Container", "Docker", "",
			"docker exec shell terminal attach",
			func(*Model) {},
		},
		{
			"docker.pullImage", "Pull Docker Image…", "Docker", "",
			"docker pull image registry",
			func(*Model) {},
		},
		{
			"docker.prune", "Prune Docker…", "Docker", "",
			"docker clean remove unused stopped containers images volumes networks system",
			func(*Model) {},
		},
		{
			"docker.composeUp", "Compose Up", "Docker", "",
			"docker compose up start service stack",
			func(*Model) {},
		},
	}
}

func dockerActionState(a *Action) {
	switch a.ID {
	case "docker.setComposeFileOverride", "docker.refresh", "docker.start", "docker.stop",
		"docker.restart", "docker.remove", "docker.showDashboard", "docker.showLog",
		"docker.openLogInEditor", "docker.showProcesses", "docker.showLayers",
		"docker.inspect", "docker.toggleStoppedContainers", "docker.toggleUntaggedImages",
		"docker.execShell", "docker.pullImage", "docker.prune", "docker.composeUp":
	default:
		return
	}
	a.RunWithArg = func(m *Model, arg any) {
		m.runDockerActionCommand(a.ID, arg)
	}
	a.When = func(m *Model) bool {
		return m.Root() != "" && !m.showingWelcome
	}
	a.Enabled = func(m *Model) bool {
		return m.dockerActionCommandEnabled(a.ID)
	}
	if a.ID == "docker.toggleStoppedContainers" {
		a.Toggled = func(m *Model) bool {
			return m.dockerFilters().showStoppedContainers()
		}
	}
	if a.ID == "docker.toggleUntaggedImages" {
		a.Toggled = func(m *Model) bool {
			return m.dockerFilters().showUntaggedImages()
		}
	}
}

func (m *Model) selectedDockerTarget() *serviceTreeNode {
	keys := m.serviceSelectionKeys()
	for i := len(keys) - 1; i >= 0; i-- {
		node := m.serviceNodeByKey(m.serviceTree(), keys[i])
		if node != nil && node.provider == dockerProviderID &&
			node.nodeKind != "docker-registry" {
			return node
		}
	}
	return nil
}

func (m *Model) dockerCurrentConnection() *openidea.DockerConnection {
	if node := m.selectedDockerTarget(); node != nil && node.dockerConn != "" {
		if connection := m.dockerConnection(node.dockerConn); connection != nil {
			return connection
		}
	}
	if len(m.services.docker.settings.Connections) == 0 {
		return nil
	}
	connection := m.services.docker.settings.Connections[0]
	return &connection
}

func (m *Model) dockerTargetContainers(node serviceTreeNode) []openidea.DockerContainer {
	snapshot := m.services.docker.snapshots[node.dockerConn]
	switch node.nodeKind {
	case "docker-container":
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			return []openidea.DockerContainer{*container}
		}
	case "docker-compose":
		return dockerComposeContainers(snapshot, node.dockerID)
	}
	return nil
}

func (m *Model) dockerActionCommandEnabled(id string) bool {
	if id == "docker.setComposeFileOverride" {
		return true
	}
	if id == "docker.refresh" || id == "docker.toggleStoppedContainers" ||
		id == "docker.toggleUntaggedImages" {
		return m.services.docker.loaded || len(m.services.docker.settings.Connections) > 0
	}
	if id == "docker.pullImage" || id == "docker.prune" {
		return m.dockerCurrentConnection() != nil
	}
	if id == "docker.composeUp" {
		t := m.tab()
		if t == nil || t.kind != "" || t.doc == nil || t.state == nil {
			return false
		}
		return m.isComposeFile(t.doc.Path(), t.doc.Text())
	}
	node := m.selectedDockerTarget()
	if node == nil {
		return false
	}
	containers := m.dockerTargetContainers(*node)
	switch id {
	case "docker.start":
		return slicesContainsFunc(containers, func(c openidea.DockerContainer) bool {
			return !dockerContainerRunning(c)
		})
	case "docker.stop":
		return slicesContainsFunc(containers, dockerContainerRunning)
	case "docker.restart":
		return node.nodeKind == "docker-container" || node.nodeKind == "docker-compose"
	case "docker.remove":
		switch node.nodeKind {
		case "docker-container", "docker-compose", "docker-image",
			"docker-volume", "docker-network":
			return true
		}
		return false
	case "docker.showDashboard":
		return node.nodeKind == "docker" || node.nodeKind == "docker-group" ||
			node.nodeKind == "docker-compose" || dockerDetailTabSupported(*node, dockerDetailDashboard)
	case "docker.showLog", "docker.openLogInEditor":
		return node.nodeKind == "docker-container"
	case "docker.showProcesses", "docker.execShell":
		if node.nodeKind != "docker-container" {
			return false
		}
		return slicesContainsFunc(containers, dockerContainerRunning)
	case "docker.showLayers":
		return node.nodeKind == "docker-image"
	case "docker.inspect":
		return dockerDetailTabSupported(*node, dockerDetailInspect)
	}
	return false
}

func slicesContainsFunc[T any](values []T, match func(T) bool) bool {
	for _, value := range values {
		if match(value) {
			return true
		}
	}
	return false
}

func (m *Model) runDockerActionCommand(id string, arg any) {
	if id == "docker.setComposeFileOverride" {
		m.setDockerComposeFileOverride(arg)
		return
	}
	if id == "docker.refresh" {
		m.refreshDockerServices()
		return
	}
	if id == "docker.toggleStoppedContainers" {
		next := !m.dockerFilters().showStoppedContainers()
		m.setDockerFilter(&next, nil)
		return
	}
	if id == "docker.toggleUntaggedImages" {
		next := !m.dockerFilters().showUntaggedImages()
		m.setDockerFilter(nil, &next)
		return
	}
	if id == "docker.pullImage" {
		if connection := m.dockerCurrentConnection(); connection != nil {
			m.ShowServices()
			m.openDockerPullDialog(connection.ID, "")
		}
		return
	}
	if id == "docker.prune" {
		connection := m.dockerCurrentConnection()
		if connection == nil {
			return
		}
		target := "system"
		switch value := arg.(type) {
		case string:
			if value != "" {
				target = value
			}
		case map[string]any:
			if value, _ := value["target"].(string); value != "" {
				target = value
			}
		}
		m.ShowServices()
		m.confirmDockerPrune(*connection, target)
		return
	}
	if id == "docker.composeUp" {
		m.composeUpAtCaret()
		return
	}

	node := m.selectedDockerTarget()
	if node == nil {
		return
	}
	connection := m.dockerConnection(node.dockerConn)
	if connection == nil {
		return
	}
	snapshot := m.services.docker.snapshots[connection.ID]
	containers := m.dockerTargetContainers(*node)
	switch id {
	case "docker.start":
		m.runDockerSelectedContainers(*node, snapshot, containers, "start", false)
	case "docker.stop":
		m.runDockerSelectedContainers(*node, snapshot, containers, "stop", true)
	case "docker.restart":
		m.runDockerSelectedContainers(*node, snapshot, containers, "restart", false)
	case "docker.remove":
		if node.nodeKind == "docker-compose" {
			m.confirmDockerComposeDown(*node, snapshot)
		} else {
			object := strings.TrimPrefix(node.nodeKind, "docker-")
			m.confirmDockerRemove(*node, object, []string{node.dockerID})
		}
	case "docker.showDashboard", "docker.showLog", "docker.showProcesses",
		"docker.showLayers", "docker.inspect":
		tab := map[string]string{
			"docker.showDashboard": dockerDetailDashboard,
			"docker.showLog":       dockerDetailLog,
			"docker.showProcesses": dockerDetailProcesses,
			"docker.showLayers":    dockerDetailLayers,
			"docker.inspect":       dockerDetailInspect,
		}[id]
		m.ShowServices()
		m.setDockerDetailTab(*connection, *node, tab)
	case "docker.openLogInEditor":
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			m.ShowServices()
			m.openDockerLogEditor(*connection, *container)
		}
	case "docker.execShell":
		if container := dockerContainerByID(snapshot, node.dockerID); container != nil {
			m.openDockerTerminal(*connection, *container)
		}
	}
}

func (m *Model) runDockerSelectedContainers(
	node serviceTreeNode,
	snapshot openidea.DockerSnapshot,
	containers []openidea.DockerContainer,
	op string,
	runningOnly bool,
) {
	if node.nodeKind == "docker-compose" {
		has := slicesContainsFunc(containers, func(c openidea.DockerContainer) bool {
			return op == "restart" || dockerContainerRunning(c) == runningOnly
		})
		if has {
			m.runDockerComposeOp(node, snapshot, op)
		}
		return
	}
	ids := make([]string, 0, len(containers))
	for _, container := range containers {
		if op != "restart" && dockerContainerRunning(container) != runningOnly {
			continue
		}
		if container.ID != "" {
			ids = append(ids, container.ID)
		}
	}
	if len(ids) > 0 {
		connection := m.dockerConnection(node.dockerConn)
		if connection != nil {
			m.runDockerContainerOpIDs(*connection, ids, op)
		}
	}
}

func (m *Model) composeUpAtCaret() {
	t := m.tab()
	if t == nil || t.kind != "" || t.doc == nil || t.state == nil {
		return
	}
	text := t.doc.Text()
	if !m.isComposeFile(t.doc.Path(), text) {
		return
	}
	target, ok := dockerComposeTargetAtLine(text, t.state.CursorPos().Line)
	if !ok {
		return
	}
	actions := m.dockerGutterActions(target, t.doc.Path())
	if len(actions) == 0 {
		return
	}
	m.RunActionWithArg("run.runConfig", map[string]any{
		"config": actions[0].config, "temporary": true,
	})
}

func dockerComposeTargetAtLine(text string, line int) (dockerTarget, bool) {
	targets := composeTargets(text)
	if len(targets) == 0 {
		return dockerTarget{}, false
	}
	all := targets[0]
	if line <= all.line {
		return all, true
	}
	lines := strings.Split(text, "\n")
	endOfServices := len(lines)
	for i := all.line + 1; i < len(lines); i++ {
		value := lines[i]
		if strings.TrimSpace(value) == "" || strings.HasPrefix(strings.TrimSpace(value), "#") {
			continue
		}
		if len(value) == len(strings.TrimLeft(value, " \t")) {
			endOfServices = i
			break
		}
	}
	if line >= endOfServices {
		return all, true
	}
	for i := len(targets) - 1; i >= 1; i-- {
		if line >= targets[i].line {
			return targets[i], true
		}
	}
	return all, true
}

func (m *Model) setDockerComposeFileOverride(arg any) {
	values, _ := arg.(map[string]any)
	path, _ := values["path"].(string)
	status, _ := values["status"].(string)
	path = strings.TrimSpace(path)
	if path == "" || status != "kept" && status != "excluded" {
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.Root(), filepath.FromSlash(path))
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if m.services.docker.composeOverrides == nil {
		m.services.docker.composeOverrides = map[string]string{}
	}
	m.services.docker.composeOverrides[filepath.Clean(path)] = status
	m.saveServicesPrefs()
	m.persistDockerComposeOverrides()
	m.invalidate()
}

func (m *Model) isComposeFile(path, text string) bool {
	if !isYAMLFile(path) {
		return false
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	switch m.services.docker.composeOverrides[filepath.Clean(path)] {
	case "excluded":
		return false
	case "kept":
		return true
	}
	return isNamedComposeFile(path) || isComposeContent(text)
}

func (m *Model) dockerFileTargets(path, text string) []dockerTarget {
	if isDockerfile(path) {
		return dockerfileTargets(text)
	}
	if m.isComposeFile(path, text) {
		return composeTargets(text)
	}
	return nil
}

func (m *Model) persistDockerComposeOverrides() {
	if m.invokeHook == nil && m.mgr == nil {
		return
	}
	root := m.Root()
	overrides := make(map[string]string, len(m.services.docker.composeOverrides))
	for path, status := range m.services.docker.composeOverrides {
		key := path
		if root != "" {
			if relative, err := filepath.Rel(root, path); err == nil &&
				relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				key = filepath.ToSlash(relative)
			}
		}
		overrides[key] = status
	}
	value, err := json.Marshal(overrides)
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.dockerContext(), 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "settings_set", map[string]any{
			"scope": "project", "key": "files.composeFileOverrides", "value": string(value),
		})
		if err != nil {
			m.post(func() {
				m.notifyError("docker-compose-detection",
					"Could not save Docker Compose file detection", err.Error())
			})
		}
	}()
}
