package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type remoteConnectionRun struct {
	cancel             context.CancelFunc
	done               chan struct{}
	previousRoot       string
	previousFiles      []string
	transportAttempted bool
}

// resetRemoteProjectState invalidates background remote replies and clears the
// per-project surfaces every time the window changes projects.
func (m *Model) resetRemoteProjectState() {
	state := &m.remote
	state.generation++
	state.browseRequest++
	state.envRequest++
	state.busy = false
	state.busyLabel = ""
	state.progress = nil
	state.browseOpen = false
	state.browseDir = ""
	state.browseEntries = nil
	state.browseErr = ""
	state.envBusy = false
	state.envLoaded = false
	state.envErr = ""
	state.envChanged = nil
	state.envDevcontainer = nil
	state.envShellRead = false
	state.ports = nil
	state.portsInput = ""
	state.portsErr = ""
	state.secretOpen = false
	state.secretValue = ""
	state.trustOpen = false
	state.sshSessionErr = ""
}

func remoteSSHConfigurationChoices(configs []openidea.SSHConfiguration) []string {
	seen := map[string]int{}
	out := make([]string, 0, len(configs))
	for _, config := range configs {
		label := strings.TrimSpace(config.Name)
		if label == "" {
			label = config.Label()
		}
		if label == "" {
			label = config.ID
		}
		seen[label]++
		if seen[label] > 1 {
			label += " (" + config.ID + ")"
		}
		out = append(out, label)
	}
	return out
}

func selectedRemoteSSHConfiguration(state *remoteDevelopmentState) *openidea.SSHConfiguration {
	labels := remoteSSHConfigurationChoices(state.sshConfigs)
	for i, label := range labels {
		if label == state.sshChoice {
			config := state.sshConfigs[i]
			return &config
		}
	}
	for i := range state.sshConfigs {
		if state.sshConfigs[i].ID == state.sshConfigID {
			config := state.sshConfigs[i]
			return &config
		}
	}
	if len(state.sshConfigs) > 0 {
		config := state.sshConfigs[0]
		return &config
	}
	return nil
}

func (m *Model) remoteConnectReady() bool {
	if remoteConnected(m) || m.remote.busy || strings.TrimSpace(m.remote.connectPath) == "" {
		return false
	}
	switch m.remote.connectTarget {
	case openidea.RemoteTargetWSL:
		return strings.TrimSpace(m.remote.distro) != ""
	case openidea.RemoteTargetSSH:
		return selectedRemoteSSHConfiguration(&m.remote) != nil
	default:
		return false
	}
}

func (m *Model) loadRemoteChoices() {
	state := &m.remote
	if state.configsLoading || state.configsLoaded {
		return
	}
	state.configsLoading = true
	state.configsErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 20*time.Second)
		defer cancel()
		var (
			configs    []openidea.SSHConfiguration
			configsErr error
			distros    []string
		)
		if m.mgr != nil || m.invokeHook != nil {
			raw, err := m.invoke(ctx, "settings_get_global", nil)
			if err != nil {
				configsErr = err
			} else {
				configs, configsErr = openidea.DecodeSSHConfigurations(raw)
			}
		}
		distros, _ = listWSLDistros(ctx, defaultRemoteRunner)
		m.post(func() {
			state.configsLoading = false
			state.configsLoaded = true
			state.sshConfigs = configs
			state.configsErr = ""
			if configsErr != nil {
				state.configsErr = "Cannot read SSH configurations: " + configsErr.Error()
			}
			state.distros = distros
			if len(state.sshConfigs) > 0 {
				exists := false
				labels := remoteSSHConfigurationChoices(state.sshConfigs)
				for _, label := range labels {
					if label == state.sshChoice {
						exists = true
					}
				}
				if !exists {
					state.sshChoice = labels[0]
					state.sshConfigID = state.sshConfigs[0].ID
				}
			}
			if state.distro == "" && len(state.distros) > 0 {
				state.distro = state.distros[0]
			}
			if state.connectTarget == "" {
				state.connectTarget = openidea.RemoteTargetSSH
			}
			if state.connectPath == "" {
				state.connectPath = "~"
			}
			if state.sshSettingsPending {
				state.sshSettingsPending = false
				m.beginRemoteSSHSettings()
			}
			if state.sshSessionOpen {
				state.sshSessionWaiting = false
				if state.sshSessionPendingID != "" {
					id := state.sshSessionPendingID
					state.sshSessionPendingID = ""
					m.startLoadedRemoteSSHSession(id)
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) remoteBaseContext() context.Context {
	if m != nil && m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Model) browseRemoteFolders(dir string) {
	state := &m.remote
	config := selectedRemoteSSHConfiguration(state)
	if config == nil || state.busy {
		return
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "~"
	}
	state.browseErr = ""
	state.busy = true
	state.busyLabel = "Reading remote folders…"
	state.browseRequest++
	request := state.browseRequest
	generation := state.generation
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 45*time.Second)
		defer cancel()
		target := openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: config.ID}
		backend := m.newRemoteBackend(target, *config)
		entries, err := backend.browse(ctx, dir)
		m.post(func() {
			if state.browseRequest != request || state.generation != generation {
				return
			}
			state.busy = false
			state.busyLabel = ""
			current := selectedRemoteSSHConfiguration(state)
			if current == nil || current.ID != config.ID ||
				state.connectTarget != openidea.RemoteTargetSSH {
				return
			}
			if err != nil {
				state.browseErr = err.Error()
				m.handleRemoteConnectError(err, target, dir, *config)
			} else {
				sort.SliceStable(entries, func(i, j int) bool {
					leftHidden := strings.HasPrefix(entries[i].Name, ".")
					rightHidden := strings.HasPrefix(entries[j].Name, ".")
					if leftHidden != rightHidden {
						return !leftHidden
					}
					return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
				})
				state.browseOpen = true
				state.browseEntries = entries
				state.browseDir = remoteListedDirectory(dir, entries)
				state.connectPath = state.browseDir
			}
			m.invalidate()
		})
	}()
}

func remoteListedDirectory(requested string, entries []openidea.RemoteDirectoryEntry) string {
	trimmed := strings.TrimRight(requested, "/")
	if strings.HasPrefix(trimmed, "/") {
		if trimmed == "" {
			return "/"
		}
		return trimmed
	}
	if len(entries) > 0 {
		return remoteParentPath(entries[0].Path)
	}
	return requested
}

func remoteParentPath(value string) string {
	trimmed := strings.TrimRight(value, "/")
	index := strings.LastIndex(trimmed, "/")
	if index <= 0 {
		return "/"
	}
	return trimmed[:index]
}

func remoteStageLabel(stage openidea.RemoteStage) string {
	switch stage {
	case openidea.RemoteStageConnecting:
		return "Connecting to the host…"
	case openidea.RemoteStageContainer:
		return "Starting the dev container…"
	case openidea.RemoteStageChecking:
		return "Checking the IDE backend on the host…"
	case openidea.RemoteStageDownloading:
		return "Downloading the IDE backend…"
	case openidea.RemoteStageUploading:
		return "Installing the IDE backend on the host…"
	case openidea.RemoteStageStarting:
		return "Starting the IDE backend…"
	case openidea.RemoteStageOpening:
		return "Opening the project…"
	default:
		return string(stage)
	}
}

func (m *Model) connectRemoteFromDialog() {
	state := &m.remote
	if !m.remoteConnectReady() {
		return
	}
	state.connectErr = ""
	state.browseErr = ""
	requested := strings.TrimSpace(state.connectPath)
	if requested == "" {
		requested = "~"
	}
	var (
		target openidea.RemoteTarget
		config openidea.SSHConfiguration
	)
	switch state.connectTarget {
	case openidea.RemoteTargetWSL:
		target = openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: state.distro}
	case openidea.RemoteTargetSSH:
		choice := selectedRemoteSSHConfiguration(state)
		if choice == nil {
			state.connectErr = "No SSH configuration is selected."
			m.invalidate()
			return
		}
		config = *choice
		state.sshConfigID = config.ID
		target = openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: config.ID}
	default:
		state.connectErr = "Choose SSH or WSL."
		m.invalidate()
		return
	}
	m.requestRemoteConnection(target, requested, config)
}

func (m *Model) openRemoteLocation(target openidea.RemoteTarget, remoteRoot string) {
	if m.remote.busy {
		return
	}
	if target.Kind != openidea.RemoteTargetSSH {
		m.requestRemoteConnection(target, remoteRoot, openidea.SSHConfiguration{})
		return
	}
	generation := m.remote.generation
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "settings_get_global", nil)
		if err != nil {
			m.post(func() {
				if m.remote.generation != generation {
					return
				}
				m.remote.connectErr = "Could not read SSH configurations: " + err.Error()
				m.OpenRemoteConnectDialog()
			})
			return
		}
		configs, err := openidea.DecodeSSHConfigurations(raw)
		if err != nil {
			m.post(func() {
				if m.remote.generation != generation {
					return
				}
				m.remote.connectErr = "Could not read SSH configurations: " + err.Error()
				m.OpenRemoteConnectDialog()
			})
			return
		}
		var selected *openidea.SSHConfiguration
		for i := range configs {
			if configs[i].ID == target.Config {
				config := configs[i]
				selected = &config
				break
			}
		}
		m.post(func() {
			if m.remote.generation != generation {
				return
			}
			m.remote.sshConfigs = configs
			m.remote.configsLoaded = true
			if selected == nil {
				m.remote.connectErr = "The SSH configuration " + target.Config + " no longer exists."
				m.OpenRemoteConnectDialog()
				return
			}
			m.remote.sshConfigID = selected.ID
			m.remote.sshChoice = remoteSSHConfigurationChoices(configs)[indexOfSSHConfig(configs, selected.ID)]
			m.requestRemoteConnection(target, remoteRoot, *selected)
		})
	}()
}

func indexOfSSHConfig(configs []openidea.SSHConfiguration, id string) int {
	for i := range configs {
		if configs[i].ID == id {
			return i
		}
	}
	return 0
}

func (m *Model) requestRemoteConnection(
	target openidea.RemoteTarget,
	requested string,
	config openidea.SSHConfiguration,
) {
	if m.remote.busy {
		return
	}
	root := m.Root()
	generation := m.remote.generation
	if !m.hasDirty() {
		m.startRemoteConnection(target, requested, config)
		return
	}
	detail := strings.Join(m.dirtyPaths(), "\n")
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Open Remote Project",
			Message: "There are unsaved changes", Detail: detail,
			Buttons:       []string{"Save and Open", "Discard Changes", "Cancel"},
			DefaultButton: 0, CancelButton: 2,
		})
		if err != nil {
			return
		}
		m.post(func() {
			if m.Root() != root || m.remote.generation != generation {
				return
			}
			switch result.Button {
			case 0:
				tabs := append([]*Tab(nil), m.tabs...)
				m.remote.busy = true
				m.remote.busyLabel = "Saving…"
				m.invalidate()
				go func() {
					if err := m.saveOpenTabs(m.remoteBaseContext(), tabs); err != nil {
						m.post(func() {
							if m.Root() != root || m.remote.generation != generation {
								return
							}
							m.remote.busy = false
							m.remote.connectErr = "Could not save the open files: " + err.Error()
							m.invalidate()
						})
						return
					}
					m.post(func() {
						if m.Root() != root || m.remote.generation != generation {
							return
						}
						m.remote.busy = false
						m.startRemoteConnection(target, requested, config)
					})
				}()
			case 1:
				m.startRemoteConnection(target, requested, config)
			}
		})
	}()
}

func (m *Model) newRemoteBackend(target openidea.RemoteTarget, config openidea.SSHConfiguration) *remoteBackend {
	dataDir := ""
	if m.cfg.SessionPath != "" {
		dataDir = filepath.Dir(m.cfg.SessionPath)
	}
	version := strings.TrimSpace(m.cfg.ServerMinVersion)
	if version == "" {
		version = "0.1.52"
	}
	backend := newRemoteBackend(target, config, version, dataDir, m.cfg.ServerPath, m.cfg.Log)
	if m.remote.secrets == nil {
		m.remote.secrets = newRemoteSecretVault()
	}
	adjustRemoteBackendSecret(backend, m.remote.secrets)
	return backend
}

func (m *Model) startRemoteConnection(
	target openidea.RemoteTarget,
	requested string,
	config openidea.SSHConfiguration,
) {
	state := &m.remote
	if state.busy {
		return
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "~"
	}
	state.pendingTarget = target
	state.pendingPath = requested
	state.pendingSSH = config
	state.busy = true
	state.busyLabel = remoteStageLabel(openidea.RemoteStageConnecting)
	state.progress = &openidea.RemoteProgress{Stage: openidea.RemoteStageConnecting}
	state.connectErr = ""
	state.browseErr = ""
	state.secretOpen = false
	state.trustOpen = false
	state.generation++
	generation := state.generation
	m.invalidate()

	previousRoot := m.Root()
	previousFiles := m.openProjectFiles()
	ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 15*time.Minute)
	run := &remoteConnectionRun{
		cancel:        cancel,
		done:          make(chan struct{}),
		previousRoot:  previousRoot,
		previousFiles: append([]string(nil), previousFiles...),
	}
	state.connection = run
	go func() {
		defer close(run.done)
		defer cancel()
		backend := m.newRemoteBackend(target, config)
		progress := func(update openidea.RemoteProgress) {
			m.post(func() {
				if !state.busy || state.generation != generation {
					return
				}
				copied := update
				state.progress = &copied
				state.busyLabel = remoteStageLabel(update.Stage)
				m.invalidate()
			})
		}
		if target.Kind == openidea.RemoteTargetDevContainer {
			if err := m.requireLocalDevContainerTrust(ctx, requested); err != nil {
				m.post(func() {
					if state.generation != generation {
						return
					}
					state.busy = false
					state.progress = nil
					state.connection = nil
					state.connectErr = err.Error()
					m.invalidate()
				})
				return
			}
			backend.devContainerCLI = m.resolveManagedDevContainerCLI(ctx)
		}
		if err := backend.prepare(ctx, progress); err != nil {
			backend.closeForwards()
			m.post(func() {
				if state.generation != generation {
					return
				}
				state.busy = false
				state.busyLabel = ""
				state.progress = nil
				state.connection = nil
				m.handleRemoteConnectError(err, target, requested, config)
				m.invalidate()
			})
			return
		}
		if m.reg != nil {
			m.reg.CloseAll(ctx)
		}
		if m.mgr == nil {
			backend.closeForwards()
			m.post(func() {
				if state.generation != generation {
					return
				}
				state.busy = false
				state.progress = nil
				state.connection = nil
				state.connectErr = "The GoIDEA backend manager is unavailable."
				m.invalidate()
			})
			return
		}
		run.transportAttempted = true
		if err := m.mgr.UseTransport(ctx, backend.launcher()); err != nil {
			_ = m.mgr.UseLocal(ctx)
			m.post(func() {
				if state.generation != generation {
					backend.closeForwards()
					return
				}
				state.busy = false
				state.progress = nil
				state.connection = nil
				state.connectErr = "Could not start the remote backend: " + err.Error()
				m.restoreAfterRemoteFailure(previousRoot, previousFiles)
			})
			return
		}
		root := remoteResolvedRoot(requested, backend, target)
		progress(openidea.RemoteProgress{Stage: openidea.RemoteStageOpening})
		info, err := m.openRemoteBackendProject(ctx, root)
		if err != nil {
			_ = m.mgr.UseLocal(ctx)
			backend.closeForwards()
			m.post(func() {
				if state.generation != generation {
					return
				}
				state.busy = false
				state.progress = nil
				state.connection = nil
				state.connectErr = "The project could not be opened on " +
					remoteTargetDisplay(target) + ": " + err.Error()
				m.restoreAfterRemoteFailure(previousRoot, previousFiles)
			})
			return
		}
		server := m.mgr.State().Info
		session := openidea.RemoteSession{
			Target: target, Root: info.Root, Host: remoteHostName(target, backend),
			OS: server.OS, ServerVersion: server.Version,
		}
		if server := m.mgr.Server(); server != nil {
			session.Commands = server.Commands()
		}
		forwardErr := backend.forwardConfiguredPorts()
		m.post(func() {
			if state.generation != generation {
				backend.closeForwards()
				return
			}
			m.finishRemoteConnection(generation, backend, session, previousRoot, info, forwardErr)
		})
	}()
}

func (m *Model) cancelRemoteConnection() {
	state := &m.remote
	run := state.connection
	if !state.busy || run == nil {
		return
	}
	state.generation++
	state.busyLabel = "Cancelling remote connection…"
	state.progress = nil
	state.connectErr = ""
	state.connectOpen = false
	run.cancel()
	m.invalidate()

	go func() {
		<-run.done
		var localErr error
		if run.transportAttempted && m.mgr != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			localErr = m.mgr.UseLocal(ctx)
			cancel()
		}
		m.post(func() {
			if state.connection != run {
				return
			}
			state.connection = nil
			state.progress = nil
			state.busyLabel = ""
			if localErr != nil {
				m.remoteBackendUnavailable(
					"Remote connection was cancelled, but the local backend could not start",
					localErr,
				)
				return
			}
			state.busy = false
			if run.transportAttempted {
				m.restoreAfterRemoteFailure(run.previousRoot, run.previousFiles)
			}
			m.status = "Remote connection cancelled."
			m.invalidate()
		})
	}()
}

func (m *Model) openProjectFiles() []string {
	out := make([]string, 0, len(m.tabs))
	for _, tab := range m.tabs {
		if tab != nil && tab.doc != nil {
			out = append(out, tab.doc.Path())
		}
	}
	for _, tab := range m.splitTabs() {
		if tab != nil && tab.doc != nil {
			out = append(out, tab.doc.Path())
		}
	}
	return out
}

func (m *Model) splitTabs() []*Tab {
	if m.split == nil {
		return nil
	}
	return m.split.tabs
}

func remoteResolvedRoot(requested string, backend *remoteBackend, target openidea.RemoteTarget) string {
	requested = strings.TrimSpace(requested)
	if target.Kind == openidea.RemoteTargetDevContainer && backend.container != nil {
		return path.Clean(backend.container.Folder)
	}
	if requested == "" || requested == "~" {
		return path.Clean(backend.home)
	}
	if strings.HasPrefix(requested, "~/") {
		return path.Join(backend.home, strings.TrimPrefix(requested, "~/"))
	}
	return path.Clean(requested)
}

func remoteHostName(target openidea.RemoteTarget, backend *remoteBackend) string {
	switch target.Kind {
	case openidea.RemoteTargetSSH:
		return backend.ssh.Label()
	case openidea.RemoteTargetWSL:
		return target.Distro
	case openidea.RemoteTargetDevContainer:
		return target.Label()
	default:
		return target.Label()
	}
}

func remoteTargetDisplay(target openidea.RemoteTarget) string {
	switch target.Kind {
	case openidea.RemoteTargetSSH:
		return "the SSH host"
	case openidea.RemoteTargetWSL:
		return "WSL " + target.Distro
	case openidea.RemoteTargetDevContainer:
		return "the dev container"
	default:
		return string(target.Kind)
	}
}

func (m *Model) openRemoteBackendProject(
	ctx context.Context,
	root string,
) (openidea.ProjectInfo, error) {
	if !validRemoteServerPath(root) {
		return openidea.ProjectInfo{}, fmt.Errorf("not an absolute server path: %s", root)
	}
	if _, err := m.mgr.Invoke(ctx, Window, "open_project", map[string]any{"path": root}); err != nil {
		return openidea.ProjectInfo{}, err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
		if err == nil {
			if info, decodeErr := openidea.DecodeProjectInfo(raw); decodeErr == nil {
				if !validRemoteServerPath(info.Root) {
					return openidea.ProjectInfo{}, fmt.Errorf(
						"GoIDEA backend opened an unexpected project folder: %s", info.Root,
					)
				}
				return info, nil
			}
		}
		if time.Now().After(deadline) {
			return openidea.ProjectInfo{}, fmt.Errorf("the remote backend did not open %s within 60 seconds", root)
		}
		select {
		case <-ctx.Done():
			return openidea.ProjectInfo{}, ctx.Err()
		case <-time.After(60 * time.Millisecond):
		}
	}
}

// validRemoteServerPath accepts only an absolute POSIX path. A remote response
// must never be interpreted as a UNC share or Windows device path.
func validRemoteServerPath(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "/") &&
		!strings.HasPrefix(value, "//") &&
		!strings.Contains(value, `\`)
}

func (m *Model) finishRemoteConnection(
	generation uint64,
	backend *remoteBackend,
	session openidea.RemoteSession,
	previousRoot string,
	info openidea.ProjectInfo,
	forwardErr error,
) {
	state := &m.remote
	if state.generation != generation {
		backend.closeForwards()
		return
	}
	if previousRoot != "" && layoutKey(previousRoot) != layoutKey(session.Root) {
		if m.layouts == nil {
			m.layouts = map[string]Layout{}
		}
		m.layouts[layoutKey(previousRoot)] = m.currentLayout()
	}
	state.connection = nil
	m.resetRemoteProjectState()
	m.resetProjectState(session.Root)
	m.setRoot(session.Root)
	m.showingWelcome = false
	m.tree = nil
	m.tabs, m.active = nil, 0
	state.backend = backend
	state.session = &session
	state.ports = nil
	state.portsErr = ""
	if forwardErr != nil {
		state.portsErr = "Could not forward dev container ports: " + forwardErr.Error()
	}
	state.busy = false
	state.busyLabel = ""
	state.progress = nil
	state.connectErr = ""
	state.connectOpen = false
	state.pendingTarget = openidea.RemoteTarget{}
	state.pendingPath = ""
	state.pendingSSH = openidea.SSHConfiguration{}
	restoreFiles := append([]string(nil), state.restoreFiles...)
	state.restoreFiles = nil
	m.tree = NewRemoteTree(session.Root, m.remoteTreeLoader(), m.post, m.invalidate)
	m.tree.SetRemovedHandler(func(paths []string) {
		for _, path := range paths {
			m.removeNavigationTree(path)
		}
	})
	m.setProjectInfo(info)
	m.configureProjectTreeView()
	if len(restoreFiles) > 0 {
		m.openBatch(restoreFiles, nil)
	}
	m.syncRemotePortsWindow()
	m.restoreRemotePorts()
	m.status = session.Host + " · " + session.Root
	m.rememberRemoteProject(session)
	m.saveSession()
	m.checkTrust(session.Root)
	m.refreshReadiness()
	m.RefreshLanguageServers()
	m.RefreshVCS()
	m.RefreshBranch()
	m.connectAgents(false)
	m.setStartPhase("ready")
	m.DockShow(remoteDevelopmentWindow)
	m.invalidate()
}

func (m *Model) remoteTreeLoader() func(string, func([]openidea.RemoteDirectoryEntry, error)) {
	return func(dir string, done func([]openidea.RemoteDirectoryEntry, error)) {
		go func() {
			ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
			defer cancel()
			raw, err := m.invoke(ctx, "fs_read_dir", map[string]any{"path": dir})
			if err != nil {
				done(nil, err)
				return
			}
			var entries []struct {
				Name string `json:"name"`
				Path string `json:"path"`
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(raw, &entries); err != nil {
				done(nil, err)
				return
			}
			out := make([]openidea.RemoteDirectoryEntry, 0, len(entries))
			for _, entry := range entries {
				out = append(out, openidea.RemoteDirectoryEntry{
					Name: entry.Name, Path: entry.Path,
					IsDir: strings.EqualFold(entry.Kind, "dir") || strings.EqualFold(entry.Kind, "directory"),
				})
			}
			done(out, nil)
		}()
	}
}

func (m *Model) handleRemoteConnectError(
	err error,
	target openidea.RemoteTarget,
	requested string,
	config openidea.SSHConfiguration,
) {
	state := &m.remote
	var secret *remoteSecretRequiredError
	var trust *remoteHostTrustError
	switch {
	case errors.As(err, &secret):
		state.secretOpen = true
		state.secretTitle = "SSH Authentication"
		state.secretLabel = "Password:"
		if strings.EqualFold(secret.Auth, "keyPair") {
			state.secretLabel = "Key passphrase:"
		}
		state.secretValue = ""
		state.secretSave = config.SaveSecret
		state.secretConfig = config
		state.pendingTarget = target
		state.pendingPath = requested
		state.pendingSSH = config
		state.connectErr = secret.Error()
	case errors.As(err, &trust):
		state.trustOpen = true
		state.trustHost = trust.Host
		state.trustFingerprint = trust.Fingerprint
		state.trustConfig = config
		state.pendingTarget = target
		state.pendingPath = requested
		state.pendingSSH = config
		state.connectErr = trust.Error()
	default:
		state.connectErr = err.Error()
	}
}

func (m *Model) submitRemoteSecret() {
	state := &m.remote
	if !state.secretOpen || strings.TrimSpace(state.secretValue) == "" {
		return
	}
	if state.secrets == nil {
		state.secrets = newRemoteSecretVault()
	}
	if err := state.secrets.set(state.secretConfig.ID, state.secretValue, state.secretSave); err != nil {
		state.connectErr = err.Error()
		m.invalidate()
		return
	}
	target, requested, config := state.pendingTarget, state.pendingPath, state.secretConfig
	state.secretOpen = false
	state.secretValue = ""
	state.connectErr = ""
	m.startRemoteConnection(target, requested, config)
}

func (m *Model) submitRemoteHostTrust() {
	state := &m.remote
	if !state.trustOpen {
		return
	}
	target, requested, config := state.pendingTarget, state.pendingPath, state.trustConfig
	expected := state.trustFingerprint
	generation := state.generation
	state.trustOpen = false
	state.connectErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		backend := m.newRemoteBackend(target, config)
		err := backend.trustHostMatching(ctx, expected)
		m.post(func() {
			if state.generation != generation {
				return
			}
			if err != nil {
				state.connectErr = err.Error()
				m.invalidate()
				return
			}
			m.startRemoteConnection(target, requested, config)
		})
	}()
}

func (m *Model) restoreAfterRemoteFailure(previousRoot string, previousFiles []string) {
	if previousRoot == "" {
		m.resetRemoteProjectState()
		m.resetProjectState("")
		m.setRoot("")
		m.showingWelcome = true
		m.syncRemotePortsWindow()
		m.invalidate()
		return
	}
	m.openProject(previousRoot, previousFiles)
}

func (m *Model) DisconnectRemoteProject() {
	state := &m.remote
	if state.busy || state.session == nil {
		state.disconnectOpen = false
		return
	}
	state.disconnectOpen = false
	m.requestProjectLeave(m.disconnectRemoteAfterProcessGuard)
}

func (m *Model) disconnectRemoteAfterProcessGuard() {
	state := &m.remote
	if state.busy || state.session == nil {
		return
	}
	if m.hasDirty() {
		detail := strings.Join(m.dirtyPaths(), "\n")
		go func() {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{
				Parent: m.win, Type: mygo.MessageWarning, Title: "Disconnect from Remote Host",
				Message: "There are unsaved changes", Detail: detail,
				Buttons:       []string{"Save and Disconnect", "Discard Changes", "Cancel"},
				DefaultButton: 0, CancelButton: 2,
			})
			if err != nil {
				return
			}
			m.post(func() {
				switch result.Button {
				case 0:
					tabs := append([]*Tab(nil), m.tabs...)
					importDone := m.cancelRemoteImport()
					state.busy = true
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
						defer cancel()
						if err := waitRemoteImportDone(ctx, importDone); err != nil {
							m.post(func() {
								state.busy = false
								state.connectErr = err.Error()
								m.invalidate()
							})
							return
						}
						if err := m.saveOpenTabs(m.remoteBaseContext(), tabs); err != nil {
							m.post(func() {
								state.busy = false
								state.connectErr = "Could not save the open files: " + err.Error()
								m.invalidate()
							})
							return
						}
						m.post(m.disconnectRemoteNow)
					}()
				case 1:
					m.disconnectRemoteNow()
				}
			})
		}()
		return
	}
	m.disconnectRemoteNow()
}

func (m *Model) disconnectRemoteNow() {
	state := &m.remote
	importDone := m.cancelRemoteImport()
	state.disconnectOpen = false
	state.generation++
	generation := state.generation
	state.busy = true
	state.busyLabel = "Disconnecting from the remote host…"
	m.invalidate()
	backend := state.backend
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		if err := waitRemoteImportDone(ctx, importDone); err != nil {
			m.post(func() {
				if state.generation != generation {
					return
				}
				state.busy = false
				state.busyLabel = ""
				state.connectErr = err.Error()
				m.invalidate()
			})
			return
		}
		if backend != nil {
			backend.closeForwards()
		}
		if m.reg != nil {
			m.reg.CloseAll(ctx)
		}
		if m.mgr != nil {
			_, _ = m.mgr.Invoke(ctx, Window, "close_project", nil)
			if err := m.mgr.UseLocal(ctx); err != nil {
				m.post(func() {
					if state.generation != generation {
						return
					}
					m.remoteBackendUnavailable(
						"Disconnected from the remote host, but the local backend could not start",
						err,
					)
				})
				return
			}
		}
		m.post(func() {
			if state.generation != generation {
				return
			}
			state.busy = false
			state.busyLabel = ""
			state.session = nil
			state.backend = nil
			state.ports = nil
			state.progress = nil
			state.connectErr = ""
			m.syncRemotePortsWindow()
			m.resetRemoteProjectState()
			m.resetProjectState("")
			m.setRoot("")
			m.tree = nil
			m.showingWelcome = true
			m.status = "Open a Java project"
			m.saveSession()
			m.setStartPhase("ready")
			m.invalidate()
		})
	}()
}

func (m *Model) switchRemoteProjectToLocal(dir string, files []string) {
	state := &m.remote
	if state.busy || state.session == nil {
		return
	}
	importDone := m.cancelRemoteImport()
	state.generation++
	generation := state.generation
	state.busy = true
	state.busyLabel = "Switching to the local backend…"
	m.invalidate()
	backend := state.backend
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		defer cancel()
		if err := waitRemoteImportDone(ctx, importDone); err != nil {
			m.post(func() {
				if state.generation != generation {
					return
				}
				state.busy = false
				state.busyLabel = ""
				state.connectErr = err.Error()
				m.invalidate()
			})
			return
		}
		if backend != nil {
			backend.closeForwards()
		}
		if m.reg != nil {
			m.reg.CloseAll(ctx)
		}
		if err := m.mgr.UseLocal(ctx); err != nil {
			m.post(func() {
				if state.generation != generation {
					return
				}
				m.remoteBackendUnavailable(
					"Could not switch the remote project to the local backend",
					err,
				)
			})
			return
		}
		m.post(func() {
			if state.generation != generation {
				return
			}
			state.busy = false
			state.busyLabel = ""
			state.session = nil
			state.backend = nil
			state.ports = nil
			state.progress = nil
			state.connectErr = ""
			m.syncRemotePortsWindow()
			m.resetRemoteProjectState()
			m.resetProjectState(dir)
			m.setRoot("")
			m.tree = nil
			m.openProject(dir, files)
		})
	}()
}

func (m *Model) remoteBackendUnavailable(message string, err error) {
	state := &m.remote
	backend := state.backend
	if backend != nil {
		backend.closeForwards()
	}
	state.session = nil
	state.backend = nil
	state.busy = false
	state.busyLabel = ""
	state.progress = nil
	state.ports = nil
	state.connectErr = message + ": " + err.Error()
	m.syncRemotePortsWindow()
	m.resetRemoteProjectState()
	m.resetProjectState("")
	m.setRoot("")
	m.tree = nil
	m.showingWelcome = true
	m.status = "Open a Java project"
	m.banner = state.connectErr
	m.saveSession()
	m.setStartPhase("ready")
	m.DockShow(remoteDevelopmentWindow)
	m.invalidate()
}

func (m *Model) ReopenInContainer() {
	if remoteConnected(m) {
		m.showRemoteNotice("Reopen in Container", "Disconnect from the remote host before reopening this project in its local dev container.")
		return
	}
	root := m.Root()
	if root == "" || m.showingWelcome {
		return
	}
	m.CheckDevContainerCLI()
	config := ""
	target := openidea.RemoteTarget{Kind: openidea.RemoteTargetDevContainer, Folder: root, Config: config}
	m.requestRemoteConnection(target, root, openidea.SSHConfiguration{})
}

func (m *Model) requireLocalDevContainerTrust(ctx context.Context, folder string) error {
	if m.mgr == nil {
		return fmt.Errorf("The GoIDEA backend manager is unavailable.")
	}
	raw, err := m.mgr.Invoke(ctx, Window, "project_trust_status", map[string]any{"path": folder})
	if err != nil {
		return fmt.Errorf("Could not check whether %s is trusted: %w", folder, err)
	}
	var status openidea.TrustStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return fmt.Errorf("Could not read the project trust status: %w", err)
	}
	if !status.Trusted {
		return fmt.Errorf("Trust the local project before starting its dev container.")
	}
	return nil
}

func (m *Model) remotePortStorePath() string {
	if m.cfg.SessionPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(m.cfg.SessionPath), "remote-ports.json")
}

func remotePortStoreKey(session *openidea.RemoteSession) string {
	if session == nil {
		return ""
	}
	switch session.Target.Kind {
	case openidea.RemoteTargetSSH:
		return "ssh:" + session.Target.Config + ":" + session.Root
	case openidea.RemoteTargetDevContainer:
		return "devcontainer:" + session.Target.Folder + ":" + session.Target.Config
	default:
		return ""
	}
}

func (m *Model) saveRemotePorts() {
	state := &m.remote
	key := remotePortStoreKey(state.session)
	path := m.remotePortStorePath()
	if key == "" || path == "" {
		return
	}
	stored := loadRemotePortStore(path)
	var own []uint16
	for _, forwarded := range state.ports {
		if forwarded.Host == nil &&
			(state.backend == nil || !state.backend.forwardIsConfigured(forwarded.RemotePort, forwarded.Host)) {
			own = append(own, forwarded.RemotePort)
		}
	}
	if len(own) == 0 {
		delete(stored, key)
	} else {
		stored[key] = own
	}
	if err := saveRemotePortStore(path, stored); err != nil {
		state.portsErr = "Could not save forwarded ports: " + err.Error()
	}
}

func (m *Model) restoreRemotePorts() {
	state := &m.remote
	backend := state.backend
	key := remotePortStoreKey(state.session)
	path := m.remotePortStorePath()
	if backend == nil || key == "" || path == "" {
		return
	}
	var failures []string
	for _, port := range loadRemotePortStore(path)[key] {
		if _, err := backend.startForward(port, nil); err != nil {
			failures = append(failures, fmt.Sprintf("%d: %v", port, err))
			continue
		}
	}
	state.ports = backend.listForwards()
	if len(failures) > 0 {
		message := "Could not restore forwarded ports: " + strings.Join(failures, ", ")
		if state.portsErr == "" {
			state.portsErr = message
		} else {
			state.portsErr += "\n" + message
		}
	}
}

func (m *Model) forwardRemotePortFromInput() {
	state := &m.remote
	if state.backend == nil {
		state.portsErr = "No remote session is connected."
		m.invalidate()
		return
	}
	value := strings.TrimSpace(state.portsInput)
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 || port > 65535 {
		state.portsErr = "Port must be between 1 and 65535."
		m.invalidate()
		return
	}
	forwarded, err := state.backend.startForward(uint16(port), nil)
	if err != nil {
		state.portsErr = err.Error()
		m.invalidate()
		return
	}
	state.portsInput = ""
	state.portsErr = ""
	state.ports = state.backend.listForwards()
	m.saveRemotePorts()
	m.syncRemotePortsWindow()
	m.status = fmt.Sprintf("Forwarded remote port %d to localhost:%d", forwarded.RemotePort, forwarded.LocalPort)
	m.invalidate()
}

func remotePortInputValid(value string) bool {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	return err == nil && port > 0 && port <= 65535
}

func (m *Model) stopRemoteForward(port uint16, host *string) {
	state := &m.remote
	if state.backend == nil {
		return
	}
	state.backend.stopForward(port, host)
	state.ports = state.backend.listForwards()
	state.portsErr = ""
	m.saveRemotePorts()
	m.invalidate()
}

func (m *Model) openRemoteForwardedPort(localPort uint16) {
	openReleasePage(fmt.Sprintf("http://localhost:%d/", localPort))
}

func remoteForwardLabel(forwarded openidea.ForwardedPort) string {
	if forwarded.Host != nil && *forwarded.Host != "" {
		return net.JoinHostPort(*forwarded.Host, strconv.Itoa(int(forwarded.RemotePort)))
	}
	return strconv.Itoa(int(forwarded.RemotePort))
}

func (m *Model) syncRemotePortsWindow() {
	available := remotePortsAvailable(m)
	if available {
		if dockSpec(remotePortsWindow) == nil {
			registerToolWindow(&toolWindowSpec{
				id: remotePortsWindow, title: "Ports", icon: "web", anchor: dockBottom,
				order: 40, stripe: true,
				open: func(model *Model) {
					model.remote.portsWindow = true
					model.remote.ports = model.backendListForwards()
				},
				close: func(model *Model) { model.remote.portsWindow = false },
				shown: func(model *Model) bool { return model.remote.portsWindow },
				render: func(model *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
					model.remotePortsToolWindow(c, p)
				},
			})
		}
		return
	}
	if dockSpec(remotePortsWindow) != nil {
		if m.dockWindowShown(remotePortsWindow) {
			m.DockHide(remotePortsWindow)
		}
		unregisterToolWindow(remotePortsWindow)
	}
	if m.tool == remotePortsWindow {
		m.tool = ""
	}
	m.remote.portsWindow = false
}

func (m *Model) backendListForwards() []openidea.ForwardedPort {
	if m.remote.backend == nil {
		return nil
	}
	return append([]openidea.ForwardedPort(nil), m.remote.backend.listForwards()...)
}

func (m *Model) remotePortsToolWindow(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	ui.Column(c).Key("remote:ports").Grow(1).Padding(10, 12, 10, 12).Gap(8).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			input := ui.TextInput(c, &state.portsInput).Height(26).Grow(1).
				Placeholder("Remote port, e.g. 3000")
			input.OnSubmit(func() { m.forwardRemotePortFromInput() })
			ui.Button(c, "Forward Port").Disabled(!remotePortInputValid(state.portsInput)).
				OnClick(m.forwardRemotePortFromInput)
		})
		if state.portsErr != "" {
			errorState(c, p, state.portsErr)
		}
		if len(state.ports) == 0 {
			emptyState(c, p, "No forwarded ports")
			ui.Text(c, "Forward a port of the remote host to reach it from this machine.").
				FontSize(12).TextColor(p.TextMuted)
			return
		}
		ui.Row(c).Height(22).Gap(8).Children(func() {
			ui.Text(c, "Remote Port").FontSize(11).Bold().TextColor(p.TextMuted).Width(160)
			ui.Text(c, "Local Address").FontSize(11).Bold().TextColor(p.TextMuted).Grow(1)
			ui.Text(c, "Host").FontSize(11).Bold().TextColor(p.TextMuted).Width(180)
			ui.Text(c, "Actions").FontSize(11).Bold().TextColor(p.TextMuted).Width(170)
		})
		for _, forwarded := range state.ports {
			forwarded := forwarded
			ui.Row(c).Height(30).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, strconv.Itoa(int(forwarded.RemotePort))).FontSize(12).
					TextColor(p.Text).Width(160).SingleLine()
				ui.Text(c, fmt.Sprintf("localhost:%d", forwarded.LocalPort)).FontSize(12).
					TextColor(p.Text).Grow(1).SingleLine()
				host := "Remote host"
				if forwarded.Host != nil && *forwarded.Host != "" {
					host = *forwarded.Host
				}
				ui.Text(c, host).FontSize(12).TextColor(p.TextMuted).Width(180).SingleLine()
				ui.Row(c).Width(170).Gap(4).Children(func() {
					ui.Button(c, "Open").OnClick(func() { m.openRemoteForwardedPort(forwarded.LocalPort) })
					ui.Button(c, "Copy").OnClick(func() {
						address := fmt.Sprintf("localhost:%d", forwarded.LocalPort)
						mygo.Clipboard.WriteText(address)
						m.status = "Copied " + address
						m.invalidate()
					})
					ui.Button(c, "Stop").OnClick(func() {
						m.stopRemoteForward(forwarded.RemotePort, forwarded.Host)
					})
				})
			})
		}
	})
}

func (m *Model) rememberRemoteProject(session openidea.RemoteSession) {
	name := path.Base(session.Root)
	if name == "." || name == "/" {
		name = session.Host
	}
	location := remoteProjectLocation(session.Target, session.Root)
	entry := RecentProject{
		Path: location, Name: name, OpenedAt: time.Now().UnixMilli(),
		Remote: remoteTargetDisplay(session.Target),
	}
	list := []RecentProject{entry}
	for _, project := range m.recents {
		if project.Path != location {
			list = append(list, project)
		}
	}
	m.recents = normalizeRecentProjects(list)
}

func remoteProjectLocation(target openidea.RemoteTarget, root string) string {
	switch target.Kind {
	case openidea.RemoteTargetSSH:
		return "ssh://" + url.PathEscape(target.Config) + ensureRemoteAbsolutePath(root)
	case openidea.RemoteTargetWSL:
		return "wsl://" + url.PathEscape(target.Distro) + ensureRemoteAbsolutePath(root)
	case openidea.RemoteTargetDevContainer:
		authority := url.PathEscape(target.Folder)
		if target.Config != "" {
			authority += ":" + url.PathEscape(target.Config)
		}
		return "devcontainer://" + authority + ensureRemoteAbsolutePath(root)
	default:
		return root
	}
}

func ensureRemoteAbsolutePath(root string) string {
	if strings.HasPrefix(root, "/") {
		return root
	}
	return "/" + root
}

func parseRemoteProjectLocation(location string) (openidea.RemoteTarget, string, bool) {
	if target, root, ok := parseWSLUNCPath(location); ok {
		return target, root, true
	}
	for _, scheme := range []string{"ssh://", "wsl://", "devcontainer://"} {
		if !strings.HasPrefix(location, scheme) {
			continue
		}
		rest := strings.TrimPrefix(location, scheme)
		slash := strings.Index(rest, "/")
		if slash <= 0 {
			return openidea.RemoteTarget{}, "", false
		}
		authority, remoteRoot := rest[:slash], rest[slash:]
		name, config, _ := strings.Cut(authority, ":")
		name, err := url.PathUnescape(name)
		if err != nil {
			return openidea.RemoteTarget{}, "", false
		}
		config, err = url.PathUnescape(config)
		if err != nil {
			return openidea.RemoteTarget{}, "", false
		}
		switch scheme {
		case "ssh://":
			return openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: name}, remoteRoot, true
		case "wsl://":
			return openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: name}, remoteRoot, true
		default:
			return openidea.RemoteTarget{
				Kind: openidea.RemoteTargetDevContainer, Folder: name, Config: config,
			}, remoteRoot, true
		}
	}
	return openidea.RemoteTarget{}, "", false
}

func parseWSLUNCPath(location string) (openidea.RemoteTarget, string, bool) {
	slashed := strings.ReplaceAll(strings.TrimSpace(location), `\`, "/")
	if !strings.HasPrefix(slashed, "//") {
		return openidea.RemoteTarget{}, "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(slashed, "//"), "/", 3)
	if len(parts) < 2 {
		return openidea.RemoteTarget{}, "", false
	}
	switch strings.ToLower(parts[0]) {
	case "wsl$", "wsl.localhost":
	default:
		return openidea.RemoteTarget{}, "", false
	}
	distro := strings.TrimSpace(parts[1])
	target := openidea.RemoteTarget{Kind: openidea.RemoteTargetWSL, Distro: distro}
	if err := target.Validate(); err != nil {
		return openidea.RemoteTarget{}, "", false
	}
	root := "/"
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		root = path.Clean("/" + strings.TrimPrefix(parts[2], "/"))
	}
	return target, root, true
}

func (m *Model) openRemoteSSHSettings() {
	state := &m.remote
	state.sshSettingsOpen = true
	state.sshSettingsPending = false
	state.sshSettingsErr = ""
	if !state.configsLoaded || state.configsLoading {
		state.sshSettingsPending = true
		m.loadRemoteChoices()
		m.invalidate()
		return
	}
	m.beginRemoteSSHSettings()
}

func (m *Model) beginRemoteSSHSettings() {
	state := &m.remote
	state.sshSettingsOpen = true
	state.sshSettingsErr = ""
	state.sshSelected = 0
	state.sshEditing = len(state.sshConfigs) == 0
	if len(state.sshConfigs) > 0 {
		m.loadRemoteSSHForm(state.sshConfigs[0])
	} else {
		state.sshName, state.sshHost, state.sshPort = "", "", ""
		state.sshUser, state.sshAuth, state.sshKeyFile = "", "openSsh", ""
		state.sshSaveSecret = false
	}
	m.invalidate()
}

func (m *Model) loadRemoteSSHForm(config openidea.SSHConfiguration) {
	state := &m.remote
	state.sshName = config.Name
	state.sshHost = config.Host
	state.sshPort = ""
	if config.Port != nil {
		state.sshPort = strconv.Itoa(*config.Port)
	}
	state.sshUser = config.User
	state.sshAuth = config.Auth
	state.sshKeyFile = config.KeyFile
	state.sshSaveSecret = config.SaveSecret
}

func (m *Model) selectRemoteSSHConfig(index int) {
	state := &m.remote
	if index < 0 || index >= len(state.sshConfigs) {
		return
	}
	state.sshSelected = index
	state.sshEditing = false
	m.loadRemoteSSHForm(state.sshConfigs[index])
	m.invalidate()
}

func (m *Model) saveRemoteSSHConfigurations(
	ctx context.Context,
	configs []openidea.SSHConfiguration,
) ([]openidea.SSHConfiguration, error) {
	value, err := json.Marshal(configs)
	if err != nil {
		return nil, err
	}
	raw, err := m.invoke(ctx, "settings_set", map[string]any{
		"scope": "global", "key": "ssh.configs", "value": string(value),
	})
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return append([]openidea.SSHConfiguration(nil), configs...), nil
	}
	saved, err := openidea.DecodeSSHConfigurations(raw)
	if err != nil {
		return nil, fmt.Errorf("read the saved SSH configurations: %w", err)
	}
	return saved, nil
}

func remoteSSHConfigIndex(configs []openidea.SSHConfiguration, id string) int {
	for i := range configs {
		if configs[i].ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) saveRemoteSSHConfiguration() {
	state := &m.remote
	if state.sshSettingsBusy {
		return
	}
	config := openidea.SSHConfiguration{
		Name: strings.TrimSpace(state.sshName), Host: strings.TrimSpace(state.sshHost),
		User: strings.TrimSpace(state.sshUser), Auth: state.sshAuth,
		KeyFile: strings.TrimSpace(state.sshKeyFile), SaveSecret: state.sshSaveSecret,
	}
	if config.Host == "" {
		state.sshSettingsErr = "Host is required."
		m.invalidate()
		return
	}
	if portText := strings.TrimSpace(state.sshPort); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port <= 0 || port > 65535 {
			state.sshSettingsErr = "Port must be between 1 and 65535."
			m.invalidate()
			return
		}
		config.Port = &port
	}
	configs := append([]openidea.SSHConfiguration(nil), state.sshConfigs...)
	if state.sshEditing || len(configs) == 0 {
		config.ID = fmt.Sprintf("ssh-%d", time.Now().UnixMilli())
		configs = append(configs, config)
	} else {
		if state.sshSelected < 0 || state.sshSelected >= len(configs) {
			state.sshSettingsErr = "Select an SSH configuration."
			m.invalidate()
			return
		}
		config.ID = configs[state.sshSelected].ID
		configs[state.sshSelected] = config
	}
	state.sshSettingsBusy = true
	state.sshSettingsErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		saved, err := m.saveRemoteSSHConfigurations(ctx, configs)
		m.post(func() {
			state.sshSettingsBusy = false
			if err != nil {
				state.sshSettingsErr = "Could not save SSH configurations: " + err.Error()
				m.invalidate()
				return
			}
			state.sshConfigs = saved
			state.sshSelected = remoteSSHConfigIndex(saved, config.ID)
			if state.sshSelected < 0 {
				state.sshSelected = 0
			}
			labels := remoteSSHConfigurationChoices(saved)
			if len(labels) > 0 {
				state.sshChoice = labels[state.sshSelected]
				state.sshConfigID = saved[state.sshSelected].ID
			}
			state.sshSettingsOpen = false
			state.sshEditing = false
			m.invalidate()
		})
	}()
}

func (m *Model) deleteRemoteSSHConfiguration() {
	state := &m.remote
	if state.sshSelected < 0 || state.sshSelected >= len(state.sshConfigs) || state.sshEditing {
		return
	}
	configs := append([]openidea.SSHConfiguration(nil), state.sshConfigs[:state.sshSelected]...)
	configs = append(configs, state.sshConfigs[state.sshSelected+1:]...)
	state.sshSettingsBusy = true
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		saved, err := m.saveRemoteSSHConfigurations(ctx, configs)
		m.post(func() {
			state.sshSettingsBusy = false
			if err != nil {
				state.sshSettingsErr = "Could not delete the SSH configuration: " + err.Error()
				m.invalidate()
				return
			}
			state.sshConfigs = saved
			state.sshChoice = ""
			state.sshSelected = 0
			if len(saved) == 0 {
				state.sshEditing = true
				state.sshName, state.sshHost, state.sshPort = "", "", ""
				state.sshUser, state.sshAuth, state.sshKeyFile = "", "openSsh", ""
			} else {
				m.loadRemoteSSHForm(saved[0])
				state.sshChoice = remoteSSHConfigurationChoices(saved)[0]
				state.sshConfigID = saved[0].ID
			}
			m.invalidate()
		})
	}()
}

func remoteConfigFromState(state *remoteDevelopmentState) openidea.SSHConfiguration {
	var port *int
	if value, err := strconv.Atoi(strings.TrimSpace(state.sshPort)); err == nil && value > 0 && value <= 65535 {
		port = &value
	}
	config := openidea.SSHConfiguration{
		Name: strings.TrimSpace(state.sshName), Host: strings.TrimSpace(state.sshHost),
		Port: port, User: strings.TrimSpace(state.sshUser), Auth: state.sshAuth,
		KeyFile: strings.TrimSpace(state.sshKeyFile), SaveSecret: state.sshSaveSecret,
	}
	return config
}
