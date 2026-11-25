package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

const deploymentRemoteHostWindow = "deployment.remoteHost"

type remoteDeploymentState struct {
	loaded   bool
	loading  bool
	err      string
	settings openidea.DeploymentSettings

	editorOpen     bool
	draft          openidea.DeploymentSettings
	selectedServer int
	editorErr      string
	editorBusy     bool
	editorOptions  bool
	testMsg        string
	testBusy       bool

	pickerOpen  bool
	pickerTitle string
	action      remoteDeploymentAction
	paths       []string
	eligible    []openidea.DeploymentServer

	busy        bool
	busyLabel   string
	request     uint32
	operation   remoteDeploymentAction
	opServerID  string
	status      string
	autoQueue   []deploymentAutoUpload
	autoLoading bool

	authOpen        bool
	authTrust       bool
	authKey         string
	authFingerprint string
	authTitle       string
	authLabel       string
	authValue       string
	authSave        bool
	authErr         string
	authBusy        bool
	authServer      openidea.DeploymentServer
	authRetry       func()

	syncOpen     bool
	syncServer   openidea.DeploymentServer
	syncPaths    []string
	syncEntries  []openidea.SyncEntry
	syncChoices  map[string]bool
	syncHideSame bool
	syncLoading  bool
	syncBusy     bool
	syncErr      string
	syncRequest  uint32

	remoteHostOpen    bool
	remoteHostServer  string
	remoteHostRoot    string
	remoteHostPath    string
	remoteHostEntries []openidea.RemoteEntry
	remoteHostLoading bool
	remoteHostErr     string
	remoteHostNew     bool
	remoteHostNewName string
}

type remoteDeploymentAction string

type deploymentAutoUpload struct {
	Path     string
	Explicit bool
}

const (
	deploymentUploadDefault   remoteDeploymentAction = "uploadDefault"
	deploymentUploadChoose    remoteDeploymentAction = "upload"
	deploymentDownloadDefault remoteDeploymentAction = "downloadDefault"
	deploymentDownloadChoose  remoteDeploymentAction = "download"
	deploymentDelete          remoteDeploymentAction = "delete"
	deploymentSyncDefault     remoteDeploymentAction = "syncDefault"
	deploymentSyncChoose      remoteDeploymentAction = "sync"
)

func init() {
	for _, action := range []*Action{
		{
			ID: "deployment.uploadToDefault", Title: "Upload to Default Server", Category: "Deployment",
			Description: "Uploads the selected files and folders to the default deployment server.",
			Keywords:    "deploy sftp ftp upload remote",
			When:        remoteProjectOpen,
			Run: func(m *Model) {
				m.startDeploymentAction(deploymentUploadDefault, m.deploymentTargets())
			},
		},
		{
			ID: "deployment.uploadTo", Title: "Upload to…", Category: "Deployment",
			Description: "Chooses a deployment server and uploads the current selection.",
			Keywords:    "deploy sftp ftp upload remote",
			When:        remoteProjectOpen,
			Run: func(m *Model) {
				m.startDeploymentAction(deploymentUploadChoose, m.deploymentTargets())
			},
		},
		{
			ID: "deployment.downloadFromDefault", Title: "Download from Default Server", Category: "Deployment",
			Description: "Replaces the selection with its default-server copies.",
			Keywords:    "deploy sftp ftp download remote",
			When:        remoteProjectOpen,
			Run: func(m *Model) {
				m.startDeploymentAction(deploymentDownloadDefault, m.deploymentTargets())
			},
		},
		{
			ID: "deployment.downloadFrom", Title: "Download from…", Category: "Deployment",
			Description: "Chooses a deployment server and downloads the current selection.",
			Keywords:    "deploy sftp ftp download remote",
			When:        remoteProjectOpen,
			Run: func(m *Model) {
				m.startDeploymentAction(deploymentDownloadChoose, m.deploymentTargets())
			},
		},
		{
			ID: "deployment.deleteFromServer", Title: "Delete from Server", Category: "Deployment",
			Description: "Deletes the selected project files from the deployment server.",
			Keywords:    "deploy sftp ftp delete remote",
			When:        remoteProjectOpen,
			Run: func(m *Model) {
				m.startDeploymentAction(deploymentDelete, m.deploymentTargets())
			},
		},
		{
			ID: "deployment.syncWith", Title: "Sync with Deployed to…", Category: "Deployment",
			Description: "Compares the selection with a deployment server and applies chosen changes.",
			Keywords:    "deploy sftp ftp compare synchronize diff remote",
			When:        remoteProjectOpen,
			Run: func(m *Model) {
				m.startDeploymentAction(deploymentSyncChoose, m.deploymentTargets())
			},
		},
		{
			ID: "deployment.syncWithDefault", Title: "Sync with Deployed to Default Server", Category: "Deployment",
			Description: "Compares the selection with the default deployment server.",
			Keywords:    "deploy sftp ftp compare synchronize default remote",
			When:        remoteProjectOpen,
			Run: func(m *Model) {
				m.startDeploymentAction(deploymentSyncDefault, m.deploymentTargets())
			},
		},
		{
			ID: "deployment.openInBrowser", Title: "Open in Browser (Deployed)", Category: "Deployment",
			Description: "Opens the deployed URL of the selected file.",
			Keywords:    "deploy web url browser remote",
			When:        remoteProjectOpen,
			Run:         func(m *Model) { m.openDeployedInBrowser(m.deploymentTargets()) },
		},
		{
			ID: "deployment.toggleAutoUpload", Title: "Automatic Upload", Category: "Deployment",
			Description: "Turns uploads to the default server on every save on or off.",
			Keywords:    "deploy auto upload on save",
			When:        remoteProjectOpen,
			Toggled: func(m *Model) bool {
				return m.remote.deployment.loaded &&
					m.remote.deployment.settings.AutoUpload != openidea.AutoUploadNever
			},
			Run: func(m *Model) { m.toggleDeploymentAutoUpload() },
		},
		{
			ID: "deployment.browseRemoteHost", Title: "Browse Remote Host", Category: "Deployment",
			Description: "Shows deployment-server files in the Remote Host tool window.",
			Keywords:    "deploy remote host files browse server",
			When:        remoteProjectOpen,
			Run:         func(m *Model) { m.OpenDeploymentRemoteHost() },
		},
		{
			ID: "deployment.configure", Title: "Deployment Configuration…", Category: "Deployment",
			Description: "Edits deployment servers, mappings, automatic upload and excludes.",
			Keywords:    "deploy sftp ftp servers mappings exclude settings",
			When:        remoteProjectOpen,
			Run:         func(m *Model) { m.OpenDeploymentSettings() },
		},
		{
			ID: "deployment.downloadRemoteInto", Title: "Download Remote Files Into Folder", Category: "Deployment",
			Description: "Downloads Remote Host files into a local folder.",
			Keywords:    "deploy remote host download folder",
			RunWithArg: func(m *Model, arg any) {
				m.downloadDeploymentRemoteArg(arg)
			},
		},
	} {
		_ = registerAction(action)
	}

	registerToolWindow(&toolWindowSpec{
		id: deploymentRemoteHostWindow, title: "Remote Host", icon: "web", anchor: dockRight,
		order: 65, stripe: true,
		open: func(m *Model) {
			m.remote.deployment.remoteHostOpen = true
			m.OpenDeploymentRemoteHost()
		},
		close: func(m *Model) { m.remote.deployment.remoteHostOpen = false },
		shown: func(m *Model) bool { return m.remote.deployment.remoteHostOpen },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.deploymentRemoteHostWindow(c, p)
		},
	})
}

func (m *Model) deploymentTargets() []string {
	var out []string
	seen := map[string]bool{}
	for _, target := range m.selectedTreeTargets() {
		if target.path == "" || target.root || seen[target.path] || !m.deploymentLocalPath(target.path) {
			continue
		}
		seen[target.path] = true
		out = append(out, target.path)
	}
	if len(out) > 0 {
		return out
	}
	if tab := m.tab(); tab != nil && tab.doc != nil {
		path := tab.doc.Path()
		if m.deploymentLocalPath(path) {
			return []string{path}
		}
	}
	return nil
}

func (m *Model) deploymentLocalPath(path string) bool {
	root := m.Root()
	if root == "" || path == "" {
		return false
	}
	if remoteConnected(m) {
		return strings.HasPrefix(path, "/") || strings.Contains(path, ":")
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// deploymentAutoUploadAfterSave queues one changed project file. The queue is
// bounded and coalesces repeated saves of the same path so a slow server
// cannot grow it without limit.
func (m *Model) deploymentAutoUploadAfterSave(t *Tab, explicit bool) {
	if t == nil || t.doc == nil || t.kind != "" {
		return
	}
	path := t.doc.Path()
	if !m.deploymentLocalPath(path) {
		return
	}
	state := &m.remote.deployment
	for i := range state.autoQueue {
		if state.autoQueue[i].Path == path {
			if explicit {
				state.autoQueue[i].Explicit = true
			}
			m.flushDeploymentAutoUploads()
			return
		}
	}
	if len(state.autoQueue) >= 256 {
		copy(state.autoQueue, state.autoQueue[1:])
		state.autoQueue = state.autoQueue[:len(state.autoQueue)-1]
	}
	state.autoQueue = append(state.autoQueue, deploymentAutoUpload{Path: path, Explicit: explicit})
	if state.loaded {
		m.flushDeploymentAutoUploads()
		return
	}
	if state.autoLoading {
		return
	}
	root := m.Root()
	state.autoLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Second)
		defer cancel()
		settings, err := m.loadDeploymentSettings(ctx)
		m.post(func() {
			state.autoLoading = false
			if m.Root() != root {
				state.autoQueue = nil
				return
			}
			if err != nil {
				state.err = err.Error()
				state.autoQueue = nil
				m.invalidate()
				return
			}
			state.loaded = true
			state.err = ""
			state.settings = settings
			m.flushDeploymentAutoUploads()
		})
	}()
}

func (m *Model) flushDeploymentAutoUploads() {
	state := &m.remote.deployment
	if !state.loaded || state.busy || state.authOpen || len(state.autoQueue) == 0 {
		return
	}
	switch state.settings.AutoUpload {
	case openidea.AutoUploadNever:
		state.autoQueue = nil
		return
	case openidea.AutoUploadExplicitSave:
		out := state.autoQueue[:0]
		for _, item := range state.autoQueue {
			if item.Explicit {
				out = append(out, item)
			}
		}
		state.autoQueue = out
		if len(state.autoQueue) == 0 {
			return
		}
	}
	server := deploymentServerByID(state.settings, state.settings.DefaultServer)
	if server == nil {
		state.autoQueue = nil
		return
	}
	item := state.autoQueue[0]
	state.autoQueue = state.autoQueue[1:]
	m.runDeploymentAction(deploymentUploadDefault, []string{item.Path}, *server)
}

func (m *Model) deploymentContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Model) loadDeploymentSettings(ctx context.Context) (openidea.DeploymentSettings, error) {
	raw, err := m.invoke(ctx, "settings_get", nil)
	if err != nil {
		return openidea.DeploymentSettings{}, err
	}
	settings, err := openidea.DecodeDeploymentSettings(raw)
	if err != nil {
		return openidea.DeploymentSettings{}, fmt.Errorf("read deployment settings: %w", err)
	}
	return settings, nil
}

func (m *Model) OpenDeploymentSettings() {
	state := &m.remote.deployment
	if state.editorBusy {
		return
	}
	if state.loading {
		return
	}
	state.loading = true
	state.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Second)
		defer cancel()
		settings, err := m.loadDeploymentSettings(ctx)
		m.post(func() {
			state.loading = false
			if err != nil {
				state.err = err.Error()
				m.notifyError("deployment.settings", "Cannot open Deployment settings", err.Error())
				m.invalidate()
				return
			}
			state.loaded = true
			state.settings = settings
			state.draft = settings
			state.selectedServer = 0
			if len(settings.Servers) == 0 {
				state.selectedServer = -1
			}
			state.editorOpen = true
			state.editorOptions = false
			state.editorErr = ""
			m.invalidate()
		})
	}()
}

func (m *Model) closeDeploymentSettings() {
	state := &m.remote.deployment
	if state.editorBusy {
		return
	}
	state.editorOpen = false
	state.editorErr = ""
	state.testMsg = ""
}

func (m *Model) addDeploymentServer(copyCurrent bool) {
	state := &m.remote.deployment
	var server openidea.DeploymentServer
	if copyCurrent && state.selectedServer >= 0 && state.selectedServer < len(state.draft.Servers) {
		server = state.draft.Servers[state.selectedServer]
		server.ID = adapter.UID()
		server.Name += " (copy)"
		server.Mappings = append([]openidea.DeploymentMapping(nil), server.Mappings...)
		server.ExcludedPaths = append([]string(nil), server.ExcludedPaths...)
	} else {
		server = openidea.DeploymentServer{
			Kind: openidea.DeploymentSFTP, ID: adapter.UID(),
			Name:     fmt.Sprintf("Server %d", len(state.draft.Servers)+1),
			FTP:      openidea.FTPConnection{Passive: true},
			Mappings: []openidea.DeploymentMapping{{}},
		}
		if len(m.remote.sshConfigs) > 0 {
			server.SSHConfig = m.remote.sshConfigs[0].ID
		}
	}
	state.draft.Servers = append(state.draft.Servers, server)
	state.selectedServer = len(state.draft.Servers) - 1
	if state.draft.DefaultServer == "" {
		state.draft.DefaultServer = server.ID
	}
	state.editorOptions = false
	state.testMsg = ""
	m.invalidate()
}

func (m *Model) removeDeploymentServer() {
	state := &m.remote.deployment
	i := state.selectedServer
	if i < 0 || i >= len(state.draft.Servers) {
		return
	}
	removed := state.draft.Servers[i].ID
	state.draft.Servers = append(state.draft.Servers[:i], state.draft.Servers[i+1:]...)
	if state.draft.DefaultServer == removed || !deploymentServerExists(state.draft.Servers, state.draft.DefaultServer) {
		state.draft.DefaultServer = ""
		if len(state.draft.Servers) > 0 {
			state.draft.DefaultServer = state.draft.Servers[0].ID
		}
	}
	if i >= len(state.draft.Servers) {
		i = len(state.draft.Servers) - 1
	}
	state.selectedServer = i
	state.testMsg = ""
	m.invalidate()
}

func deploymentServerExists(servers []openidea.DeploymentServer, id string) bool {
	if id == "" {
		return false
	}
	for _, server := range servers {
		if server.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) saveDeploymentSettings() {
	state := &m.remote.deployment
	if state.editorBusy {
		return
	}
	state.draft.DefaultServer = strings.TrimSpace(state.draft.DefaultServer)
	state.draft.Exclude = parseDeploymentExcludes(strings.Join(state.draft.Exclude, ";"))
	if state.draft.DefaultServer != "" && !deploymentServerExists(state.draft.Servers, state.draft.DefaultServer) {
		state.editorErr = "The default deployment server no longer exists."
		m.invalidate()
		return
	}
	for _, server := range state.draft.Servers {
		if strings.TrimSpace(server.ID) == "" || strings.TrimSpace(server.Name) == "" {
			state.editorErr = "Every deployment server needs an id and a name."
			m.invalidate()
			return
		}
		if server.Kind == openidea.DeploymentSFTP && strings.TrimSpace(server.SSHConfig) == "" {
			state.editorErr = server.Name + " needs an SSH configuration."
			m.invalidate()
			return
		}
		if (server.Kind == openidea.DeploymentFTP || server.Kind == openidea.DeploymentFTPS) &&
			strings.TrimSpace(server.FTP.Host) == "" {
			state.editorErr = server.Name + " needs an FTP host."
			m.invalidate()
			return
		}
	}
	raw, err := json.Marshal(state.draft)
	if err != nil {
		state.editorErr = err.Error()
		m.invalidate()
		return
	}
	state.editorBusy = true
	state.editorErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Second)
		defer cancel()
		saved, err := m.invoke(ctx, "settings_set", map[string]any{
			"scope": "project", "key": "deployment", "value": string(raw),
		})
		var settings openidea.DeploymentSettings
		if err == nil {
			settings, err = openidea.DecodeDeploymentSettings(saved)
		}
		m.post(func() {
			state.editorBusy = false
			if err != nil {
				state.editorErr = "Could not save deployment settings: " + err.Error()
				m.invalidate()
				return
			}
			state.loaded = true
			state.settings = settings
			state.draft = settings
			state.editorOpen = false
			state.testMsg = ""
			m.status = "Deployment settings saved"
			m.invalidate()
			m.flushDeploymentAutoUploads()
		})
	}()
}

func parseDeploymentExcludes(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range strings.FieldsFunc(text, func(r rune) bool { return r == ';' || r == '\n' }) {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func (m *Model) startDeploymentAction(action remoteDeploymentAction, paths []string) {
	state := &m.remote.deployment
	if state.busy {
		state.status = "A deployment operation is already running."
		m.DockShow(remoteDevelopmentWindow)
		m.invalidate()
		return
	}
	if len(paths) == 0 {
		m.notifyInfo("deployment.noTarget", "Select files in the Project view or open a file",
			"Deployment actions work on project files and folders.")
		return
	}
	state.busy = true
	state.busyLabel = "Checking deployment mappings…"
	state.operation = action
	state.opServerID = ""
	state.status = ""
	m.DockShow(remoteDevelopmentWindow)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 45*time.Second)
		defer cancel()
		settings, err := m.loadDeploymentSettings(ctx)
		if err != nil {
			m.post(func() {
				state.busy = false
				state.busyLabel = ""
				state.status = ""
				m.notifyError("deployment.settings", "Cannot read deployment settings", err.Error())
				m.invalidate()
			})
			return
		}
		raw, err := m.invoke(ctx, "deploy_coverage", map[string]any{"paths": paths})
		var coverage []openidea.DeploymentCoverage
		if err == nil {
			err = json.Unmarshal(raw, &coverage)
		}
		m.post(func() {
			state.busy = false
			state.busyLabel = ""
			if err != nil {
				m.notifyError("deployment.coverage", "Cannot check deployment mappings", err.Error())
				m.invalidate()
				return
			}
			state.loaded = true
			state.settings = settings
			m.finishDeploymentCoverage(action, paths, coverage)
		})
	}()
}

func (m *Model) finishDeploymentCoverage(
	action remoteDeploymentAction,
	paths []string,
	coverage []openidea.DeploymentCoverage,
) {
	state := &m.remote.deployment
	var eligible []openidea.DeploymentServer
	for _, server := range state.settings.Servers {
		for _, result := range coverage {
			if result.Server == server.ID && len(result.Paths) > 0 {
				eligible = append(eligible, server)
				break
			}
		}
	}
	if len(eligible) == 0 {
		m.notifyWarning("deployment.noCoverage", "No deployment server covers the selection",
			"Add or change a mapping in Deployment Configuration.")
		return
	}
	choose := action == deploymentUploadChoose || action == deploymentDownloadChoose ||
		action == deploymentSyncChoose
	if !choose {
		for _, server := range eligible {
			if server.ID == state.settings.DefaultServer {
				m.runDeploymentAction(action, paths, server)
				return
			}
		}
		if len(eligible) == 1 {
			m.runDeploymentAction(action, paths, eligible[0])
			return
		}
	}
	state.pickerOpen = true
	state.action = action
	state.paths = append([]string(nil), paths...)
	state.eligible = eligible
	switch action {
	case deploymentUploadChoose, deploymentUploadDefault:
		state.pickerTitle = "Upload to"
	case deploymentDownloadChoose, deploymentDownloadDefault:
		state.pickerTitle = "Download from"
	case deploymentSyncChoose, deploymentSyncDefault:
		state.pickerTitle = "Sync with Deployed to"
	default:
		state.pickerTitle = "Deployment"
	}
	m.invalidate()
}

func (m *Model) chooseDeploymentServer(server openidea.DeploymentServer) {
	state := &m.remote.deployment
	action, paths := state.action, append([]string(nil), state.paths...)
	state.pickerOpen = false
	state.eligible = nil
	state.paths = nil
	m.runDeploymentAction(action, paths, server)
}

func (m *Model) runDeploymentAction(
	action remoteDeploymentAction,
	paths []string,
	server openidea.DeploymentServer,
) {
	state := &m.remote.deployment
	if action == deploymentSyncChoose || action == deploymentSyncDefault {
		m.openDeploymentSync(paths, server)
		return
	}
	if state.busy {
		return
	}
	if action == deploymentDelete {
		m.deploymentDeleteConfirmOpen(paths, server)
		return
	}
	request := m.nextRequest()
	state.busy = true
	state.request = request
	state.operation = action
	state.opServerID = server.ID
	state.status = ""
	verb := "Uploading"
	if action == deploymentDownloadDefault || action == deploymentDownloadChoose {
		verb = "Downloading"
	}
	state.busyLabel = verb + " to " + server.Label() + "…"
	if verb == "Downloading" {
		state.busyLabel = verb + " from " + server.Label() + "…"
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Minute)
		defer cancel()
		command := "deploy_upload"
		if action == deploymentDownloadDefault || action == deploymentDownloadChoose {
			command = "deploy_download"
		}
		raw, err := m.invoke(ctx, command, map[string]any{
			"paths": paths, "server": server.ID, "request": request,
		})
		var report openidea.TransferReport
		if err == nil {
			err = json.Unmarshal(raw, &report)
		}
		m.post(func() {
			state.busy = false
			state.busyLabel = ""
			state.request = 0
			defer m.flushDeploymentAutoUploads()
			if err != nil {
				if !m.offerDeploymentAuth(err.Error(), server, func() {
					m.runDeploymentAction(action, paths, server)
				}) {
					m.notifyError("deployment.failure", "Deployment failed for "+server.Label(), err.Error())
				}
				m.invalidate()
				return
			}
			m.showDeploymentReport(command, server, report)
			m.RefreshDeploymentRemoteHost()
			m.invalidate()
		})
	}()
}

func (m *Model) deploymentDeleteConfirmOpen(paths []string, server openidea.DeploymentServer) {
	state := &m.remote.deployment
	state.authOpen = true
	state.authTrust = false
	state.authKey = ""
	state.authTitle = "Delete from Server"
	state.authLabel = ""
	state.authValue = ""
	state.authErr = "Delete the remote copies of " + strconv.Itoa(len(paths)) +
		" selected item(s) from " + server.Label() + "?"
	state.authServer = server
	state.authRetry = func() { m.deleteDeploymentPaths(paths, server) }
	m.invalidate()
}

func (m *Model) deleteDeploymentPaths(paths []string, server openidea.DeploymentServer) {
	state := &m.remote.deployment
	state.busy = true
	state.busyLabel = "Deleting from " + server.Label() + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 10*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "deploy_delete_remote", map[string]any{
			"paths": paths, "server": server.ID,
		})
		var report openidea.TransferReport
		if err == nil {
			err = json.Unmarshal(raw, &report)
		}
		m.post(func() {
			state.busy = false
			state.busyLabel = ""
			if err != nil {
				if !m.offerDeploymentAuth(err.Error(), server, func() {
					m.deleteDeploymentPaths(paths, server)
				}) {
					m.notifyError("deployment.delete", "Cannot delete from "+server.Label(), err.Error())
				}
			} else {
				m.showDeploymentReport("deploy_delete_remote", server, report)
				m.RefreshDeploymentRemoteHost()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) cancelDeploymentOperation() {
	state := &m.remote.deployment
	request := state.request
	if request == 0 {
		request = state.syncRequest
	}
	if request == 0 {
		return
	}
	m.status = "Cancelling deployment operation…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 15*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "cancel_request", map[string]any{"id": request})
		m.post(func() {
			if err != nil {
				state.status = "Could not cancel the deployment operation: " + err.Error()
			} else {
				state.status = "Deployment operation cancelled."
			}
			m.invalidate()
		})
	}()
}

func (m *Model) deploymentOperationCanCancel() bool {
	state := &m.remote.deployment
	return state.request != 0 || state.syncRequest != 0
}

func (m *Model) showDeploymentReport(command string, server openidea.DeploymentServer, report openidea.TransferReport) {
	verb := "Uploaded"
	switch command {
	case "deploy_download":
		verb = "Downloaded"
	case "deploy_delete_remote":
		verb = "Deleted"
	case "deploy_sync":
		verb = "Synchronized"
	}
	title := fmt.Sprintf("%s %d file(s) %s", verb, report.Files, server.Label())
	if report.Files == 0 {
		title = "No files " + strings.ToLower(strings.TrimSuffix(verb, "ed")) + "ed " + server.Label()
	}
	var details []string
	if report.Skipped > 0 {
		details = append(details, fmt.Sprintf("Skipped %d path(s).", report.Skipped))
	}
	for i, item := range report.Errors {
		if i >= 8 {
			details = append(details, fmt.Sprintf("and %d more error(s).", len(report.Errors)-i))
			break
		}
		details = append(details, item.Path+": "+item.Message)
	}
	body := strings.Join(details, "\n")
	switch {
	case len(report.Errors) > 0:
		m.notifyError("deployment.report", title, body)
	case report.Skipped > 0:
		m.notifyWarning("deployment.report", title, body)
	default:
		m.status = title
	}
}

func (m *Model) offerDeploymentAuth(message string, server openidea.DeploymentServer, retry func()) bool {
	state := &m.remote.deployment
	state.authRetry = retry
	state.authServer = server
	state.authErr = ""
	state.authBusy = false
	switch {
	case sshHostKeyNeedsTrust(message):
		state.authOpen = true
		state.authTrust = true
		state.authKey = server.SSHConfig
		state.authFingerprint = sshHostKeyFingerprint(message)
		state.authTitle = "Trust SSH Host"
		state.authLabel = server.SSHConfig
		if state.authFingerprint == "" {
			state.authErr = "The SSH host key fingerprint was not included. Retry the operation."
		}
	case sshAuthenticationNeedsSecret(message):
		state.authOpen = true
		state.authTrust = false
		state.authFingerprint = ""
		if server.Kind == openidea.DeploymentFTP || server.Kind == openidea.DeploymentFTPS {
			state.authKey = "ftp:" + server.ID
			state.authTitle = "FTP Password"
			state.authLabel = server.FTP.User + "@" + server.FTP.Host
		} else {
			state.authKey = server.SSHConfig
			state.authTitle = "SSH Password or Passphrase"
			state.authLabel = server.SSHConfig
		}
	default:
		return false
	}
	m.invalidate()
	return true
}

func (m *Model) retryDeploymentAuth() {
	state := &m.remote.deployment
	if state.authBusy || state.authRetry == nil {
		return
	}
	if !state.authTrust && strings.TrimSpace(state.authValue) == "" {
		state.authErr = "Enter the password or passphrase."
		m.invalidate()
		return
	}
	state.authBusy = true
	state.authErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Second)
		defer cancel()
		command, args := "ssh_set_secret", map[string]any{
			"id": state.authKey, "secret": state.authValue, "save": state.authSave,
		}
		if state.authTrust {
			command, args = "ssh_trust_host", map[string]any{
				"id": state.authKey, "fingerprint": state.authFingerprint,
			}
		}
		_, err := m.invoke(ctx, command, args)
		m.post(func() {
			state.authBusy = false
			if err != nil {
				state.authErr = err.Error()
				m.invalidate()
				return
			}
			retry := state.authRetry
			state.authOpen = false
			state.authValue = ""
			state.authFingerprint = ""
			state.authRetry = nil
			if retry != nil {
				retry()
			}
			m.invalidate()
		})
	}()
}

func (m *Model) toggleDeploymentAutoUpload() {
	state := &m.remote.deployment
	value := openidea.AutoUploadAlways
	if state.loaded && state.settings.AutoUpload != openidea.AutoUploadNever {
		value = openidea.AutoUploadNever
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 20*time.Second)
		defer cancel()
		encoded, _ := json.Marshal(value)
		raw, err := m.invoke(ctx, "settings_set", map[string]any{
			"scope": "project", "key": "deployment.autoUpload", "value": string(encoded),
		})
		var settings openidea.DeploymentSettings
		if err == nil {
			settings, err = openidea.DecodeDeploymentSettings(raw)
		}
		m.post(func() {
			if err != nil {
				m.notifyError("deployment.autoUpload", "Cannot change automatic upload", err.Error())
				return
			}
			state.loaded = true
			state.settings = settings
			if value == openidea.AutoUploadNever {
				m.status = "Automatic upload is off"
			} else {
				m.status = "Automatic upload is on"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) openDeployedInBrowser(paths []string) {
	if len(paths) == 0 {
		return
	}
	path := paths[0]
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "deploy_info", map[string]any{"path": path, "server": nil})
		var info *openidea.DeployedPath
		if err == nil && len(raw) > 0 && string(raw) != "null" {
			err = json.Unmarshal(raw, &info)
		}
		m.post(func() {
			switch {
			case err != nil:
				m.notifyError("deployment.openInBrowser", "Cannot open in browser", err.Error())
			case info == nil:
				m.notifyInfo("deployment.openInBrowser", "This file is not mapped to the default server", "")
			case info.WebURL == nil || strings.TrimSpace(*info.WebURL) == "":
				m.notifyInfo("deployment.openInBrowser", "No web server URL for this file", "")
			default:
				openRemoteWebPage(*info.WebURL)
				m.status = "Opened " + *info.WebURL
			}
			m.invalidate()
		})
	}()
}

func (m *Model) deploymentPickerDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	if !state.pickerOpen {
		return
	}
	open := state.pickerOpen
	dialog := nativeModal(c.Key("deployment-server-picker"), &open, func() {
		ui.Column(c).Width(520).Padding(18).Gap(8).Children(func() {
			ui.Text(c, state.pickerTitle).FontSize(14).Bold().TextColor(p.Text).SingleLine()
			for _, server := range state.eligible {
				server := server
				ui.Button(c, server.Label()).OnClick(func() {
					m.chooseDeploymentServer(server)
				})
			}
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() { state.pickerOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.pickerOpen = false
	}
}

func (m *Model) deploymentSettingsDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	if !state.editorOpen {
		return
	}
	open := state.editorOpen
	dialog := nativeModal(c.Key("deployment-settings"), &open, func() {
		ui.Column(c).Width(820).MaxHeight(720).Padding(18).Gap(10).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Deployment").FontSize(14).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				ui.Button(c, "Servers").OnClick(func() { state.editorOptions = false })
				ui.Button(c, "Options").OnClick(func() { state.editorOptions = true })
			})
			if state.editorOptions {
				m.deploymentOptionsEditor(c, p)
			} else {
				m.deploymentServersEditor(c, p)
			}
			if state.editorErr != "" {
				errorState(c, p, state.editorErr)
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Save").Disabled(state.editorBusy).OnClick(m.saveDeploymentSettings)
				ui.Button(c, "Cancel").Disabled(state.editorBusy).OnClick(m.closeDeploymentSettings)
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.closeDeploymentSettings()
	}
}

func (m *Model) deploymentServersEditor(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	ui.Row(c).Grow(1).Gap(12).Children(func() {
		ui.Column(c).Width(250).Gap(4).Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				ui.Button(c, "+").OnClick(func() { m.addDeploymentServer(false) })
				ui.Button(c, "Copy").Disabled(state.selectedServer < 0).
					OnClick(func() { m.addDeploymentServer(true) })
				ui.Button(c, "−").Disabled(state.selectedServer < 0).
					OnClick(m.removeDeploymentServer)
			})
			for i, server := range state.draft.Servers {
				i, server := i, server
				label := server.Label()
				if server.ID == state.draft.DefaultServer {
					label += "  ·  Default"
				}
				ui.Button(c, label).OnClick(func() {
					state.selectedServer = i
					state.testMsg = ""
					state.editorOptions = false
					m.invalidate()
				})
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			i := state.selectedServer
			if i < 0 || i >= len(state.draft.Servers) {
				emptyState(c, p, "No deployment server")
				ui.Text(c, "Add a server, then map project folders onto its filesystem.").
					FontSize(11).TextColor(p.TextMuted)
				return
			}
			server := &state.draft.Servers[i]
			deploymentField(c, p, "Name", &server.Name)
			kind := string(server.Kind)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Type").FontSize(11).TextColor(p.Text).Width(120)
				ui.Select(c, &kind, []string{"sftp", "ftp", "ftps", "local"}).Grow(1)
			})
			server.Kind = openidea.DeploymentType(kind)
			switch server.Kind {
			case openidea.DeploymentSFTP:
				ui.Text(c, "SSH configuration").FontSize(11).TextColor(p.Text)
				choices := remoteSSHConfigurationChoices(m.remote.sshConfigs)
				if len(choices) == 0 {
					deploymentField(c, p, "Configuration ID", &server.SSHConfig)
				} else {
					selected := server.SSHConfig
					labels := append([]string(nil), choices...)
					index := indexOfSSHConfig(m.remote.sshConfigs, server.SSHConfig)
					if index >= 0 {
						selected = labels[index]
					}
					ui.Select(c, &selected, labels).Grow(1)
					if j := slicesIndex(labels, selected); j >= 0 {
						server.SSHConfig = m.remote.sshConfigs[j].ID
					}
				}
			case openidea.DeploymentFTP, openidea.DeploymentFTPS:
				deploymentField(c, p, "FTP host", &server.FTP.Host)
				port := 0
				if server.FTP.Port != nil {
					port = *server.FTP.Port
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Port").FontSize(11).TextColor(p.Text).Width(120)
					f := float64(port)
					field := ui.NumberInput(c, &f, 0, 65535, 1).Width(100)
					if field.Changed() {
						if f <= 0 {
							server.FTP.Port = nil
						} else {
							n := int(f)
							server.FTP.Port = &n
						}
					}
				})
				deploymentField(c, p, "User", &server.FTP.User)
				ui.Checkbox(c, &server.FTP.Passive, "Passive mode")
			}
			deploymentField(c, p, "Root path", &server.RootPath)
			deploymentField(c, p, "Web URL", &server.WebURL)
			ui.Text(c, "Mappings").FontSize(11).Bold().TextColor(p.Text)
			for j := range server.Mappings {
				mapping := &server.Mappings[j]
				ui.Row(c).Gap(4).Children(func() {
					ui.TextInput(c, &mapping.Local).Placeholder("Local folder").Grow(1)
					ui.TextInput(c, &mapping.Deployment).Placeholder("Remote folder").Grow(1)
					ui.TextInput(c, &mapping.Web).Placeholder("Web path").Width(130)
					ui.Button(c, "−").OnClick(func() {
						server.Mappings = append(server.Mappings[:j], server.Mappings[j+1:]...)
						m.invalidate()
					})
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Add Mapping").OnClick(func() {
					server.Mappings = append(server.Mappings, openidea.DeploymentMapping{})
					m.invalidate()
				})
				ui.Button(c, "Test Connection").Disabled(state.testBusy).
					OnClick(func() { m.testDeploymentServer(*server) })
				if server.ID != state.draft.DefaultServer {
					ui.Button(c, "Make Default").OnClick(func() {
						state.draft.DefaultServer = server.ID
						m.invalidate()
					})
				}
			})
			if state.testMsg != "" {
				ui.Text(c, state.testMsg).FontSize(11).TextColor(p.TextMuted)
			}
		})
	})
}

func (m *Model) testDeploymentServer(server openidea.DeploymentServer) {
	state := &m.remote.deployment
	if state.testBusy {
		return
	}
	state.testBusy = true
	state.testMsg = "Testing " + server.Label() + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "deploy_test_connection", map[string]any{
			"connection": server.Connection(), "path": server.RootPath,
		})
		var message string
		if err == nil && len(raw) > 0 && string(raw) != "null" {
			_ = json.Unmarshal(raw, &message)
		}
		m.post(func() {
			state.testBusy = false
			if err != nil {
				if !m.offerDeploymentAuth(err.Error(), server, func() {
					m.testDeploymentServer(server)
				}) {
					state.testMsg = "Connection failed: " + err.Error()
				}
				m.invalidate()
				return
			}
			if message == "" {
				message = "Connected to " + server.Label()
			}
			state.testMsg = message
			m.invalidate()
		})
	}()
}

func deploymentField(c *ui.Context, p *ui.Theme, label string, value *string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(p.Text).Width(120)
		ui.TextInput(c, value).Grow(1)
	})
}

func slicesIndex(values []string, value string) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return -1
}

func (m *Model) deploymentOptionsEditor(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	ui.Text(c, "Default server").FontSize(11).Bold().TextColor(p.Text).SingleLine()
	if len(state.draft.Servers) > 0 {
		labels := make([]string, len(state.draft.Servers))
		selected := ""
		for i, server := range state.draft.Servers {
			labels[i] = server.Label()
			if server.ID == state.draft.DefaultServer {
				selected = labels[i]
			}
		}
		if selected == "" {
			selected = labels[0]
			state.draft.DefaultServer = state.draft.Servers[0].ID
		}
		ui.Select(c, &selected, labels).Width(320)
		if i := slicesIndex(labels, selected); i >= 0 {
			state.draft.DefaultServer = state.draft.Servers[i].ID
		}
	} else {
		ui.Text(c, "Add a deployment server first.").FontSize(11).TextColor(p.TextMuted)
	}
	auto := string(state.draft.AutoUpload)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Upload changed files").FontSize(11).TextColor(p.Text).Width(220)
		ui.Select(c, &auto, []string{"never", "onExplicitSave", "always"}).Width(220)
	})
	state.draft.AutoUpload = openidea.AutoUpload(auto)
	ui.Checkbox(c, &state.draft.DeleteRemote, "Delete remote files when their local file is deleted")
	ui.Checkbox(c, &state.draft.PreserveTimestamps, "Preserve timestamps on uploaded files")
	ui.Text(c, "Exclude items by name (separate with semicolons)").FontSize(11).Bold().
		TextColor(p.Text).SingleLine()
	exclude := strings.Join(state.draft.Exclude, ";")
	ui.TextInput(c, &exclude).Height(30)
	state.draft.Exclude = parseDeploymentExcludes(exclude)
	ui.Text(c, "Wildcards use * and ?; matching names are skipped at every depth.").
		FontSize(11).TextColor(p.TextMuted)
}

func (m *Model) deploymentAuthDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	if !state.authOpen {
		return
	}
	open := state.authOpen
	dialog := nativeModal(c.Key("deployment-auth"), &open, func() {
		ui.Column(c).Width(560).Padding(18).Gap(10).Children(func() {
			ui.Text(c, state.authTitle).FontSize(14).Bold().TextColor(p.Text).SingleLine()
			if state.authTrust {
				ui.Text(c, "Trust the SSH host key for "+state.authLabel+"?").FontSize(12).
					TextColor(p.TextMuted)
				if state.authFingerprint != "" {
					ui.Text(c, state.authFingerprint).FontSize(11).TextColor(p.Text).SingleLine()
				}
				if state.authErr != "" {
					errorState(c, p, state.authErr)
				}
				ui.Row(c).Children(func() {
					ui.Spacer(c)
					ui.Button(c, "Trust Host").
						Disabled(state.authBusy || state.authFingerprint == "").
						OnClick(m.retryDeploymentAuth)
					ui.Button(c, "Cancel").Disabled(state.authBusy).OnClick(func() {
						state.authOpen = false
						state.authRetry = nil
					})
				})
				return
			}
			ui.Text(c, state.authLabel).FontSize(12).TextColor(p.TextMuted).SingleLine()
			ui.TextInput(c, &state.authValue).Height(30)
			ui.Checkbox(c, &state.authSave, "Save password")
			if state.authErr != "" {
				errorState(c, p, state.authErr)
			}
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Connect").Disabled(state.authBusy || state.authValue == "").
					OnClick(m.retryDeploymentAuth)
				ui.Button(c, "Cancel").Disabled(state.authBusy).OnClick(func() {
					state.authOpen = false
					state.authValue = ""
					state.authRetry = nil
				})
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.authOpen = false
		state.authValue = ""
		state.authRetry = nil
	}
}

func (m *Model) openDeploymentSync(paths []string, server openidea.DeploymentServer) {
	state := &m.remote.deployment
	if state.syncLoading || state.syncBusy {
		return
	}
	request := m.nextRequest()
	state.syncOpen = true
	state.syncServer = server
	state.syncPaths = append([]string(nil), paths...)
	state.syncEntries = nil
	state.syncChoices = map[string]bool{}
	state.syncHideSame = true
	state.syncLoading = true
	state.syncErr = ""
	state.syncRequest = request
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Minute)
		defer cancel()
		var entries []openidea.SyncEntry
		for _, path := range paths {
			raw, err := m.invoke(ctx, "deploy_compare", map[string]any{
				"path": path, "server": server.ID, "request": request,
			})
			if err != nil {
				m.post(func() {
					state.syncLoading = false
					state.syncRequest = 0
					if !m.offerDeploymentAuth(err.Error(), server, func() {
						m.openDeploymentSync(paths, server)
					}) {
						state.syncErr = err.Error()
					}
					m.invalidate()
				})
				return
			}
			var group []openidea.SyncEntry
			if err := json.Unmarshal(raw, &group); err != nil {
				m.post(func() {
					state.syncLoading = false
					state.syncRequest = 0
					state.syncErr = err.Error()
					m.invalidate()
				})
				return
			}
			entries = append(entries, group...)
		}
		m.post(func() {
			state.syncLoading = false
			state.syncRequest = 0
			state.syncEntries = mergeDeploymentSyncEntries(entries)
			state.syncChoices = initialDeploymentSyncChoices(state.syncEntries)
			m.invalidate()
		})
	}()
}

func mergeDeploymentSyncEntries(entries []openidea.SyncEntry) []openidea.SyncEntry {
	seen := map[string]int{}
	out := make([]openidea.SyncEntry, 0, len(entries))
	for _, entry := range entries {
		if i, ok := seen[entry.Path]; ok {
			out[i] = entry
			continue
		}
		seen[entry.Path] = len(out)
		out = append(out, entry)
	}
	return out
}

func initialDeploymentSyncChoices(entries []openidea.SyncEntry) map[string]bool {
	out := make(map[string]bool, len(entries))
	for _, entry := range entries {
		out[entry.Path] = entry.Status != openidea.SyncSame
	}
	return out
}

func deploymentSyncAction(status openidea.SyncStatus) openidea.SyncAction {
	switch status {
	case openidea.SyncRemoteOnly, openidea.SyncRemoteNewer:
		return openidea.SyncDownload
	default:
		return openidea.SyncUpload
	}
}

func (m *Model) closeDeploymentSync() {
	state := &m.remote.deployment
	if state.syncRequest != 0 {
		request := state.syncRequest
		go func() {
			ctx, cancel := context.WithTimeout(m.deploymentContext(), 10*time.Second)
			defer cancel()
			_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
		}()
	}
	state.syncOpen = false
	state.syncEntries = nil
	state.syncChoices = nil
	state.syncErr = ""
	state.syncRequest = 0
}

func (m *Model) applyDeploymentSync() {
	state := &m.remote.deployment
	if state.syncBusy || state.syncLoading {
		return
	}
	var steps []openidea.SyncStep
	for _, entry := range state.syncEntries {
		if state.syncChoices[entry.Path] && entry.Status != openidea.SyncSame {
			steps = append(steps, openidea.SyncStep{
				Path: entry.Path, Action: deploymentSyncAction(entry.Status),
			})
		}
	}
	if len(steps) == 0 {
		return
	}
	request := m.nextRequest()
	state.syncBusy = true
	state.syncRequest = request
	state.syncErr = ""
	server := state.syncServer
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Minute)
		defer cancel()
		var tabs []*Tab
		seen := map[*Tab]bool{}
		for _, step := range steps {
			if step.Action != openidea.SyncUpload {
				continue
			}
			index := m.tabForPath(filepath.Join(m.Root(), filepath.FromSlash(step.Path)))
			if index >= 0 && !seen[m.tabs[index]] {
				tabs = append(tabs, m.tabs[index])
				seen[m.tabs[index]] = true
			}
		}
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				state.syncBusy = false
				state.syncRequest = 0
				state.syncErr = err.Error()
				m.invalidate()
			})
			return
		}
		raw, err := m.invoke(ctx, "deploy_sync", map[string]any{
			"steps": steps, "server": server.ID, "request": request,
		})
		var report openidea.TransferReport
		if err == nil {
			err = json.Unmarshal(raw, &report)
		}
		m.post(func() {
			state.syncBusy = false
			state.syncRequest = 0
			if err != nil {
				if !m.offerDeploymentAuth(err.Error(), server, m.applyDeploymentSync) {
					state.syncErr = err.Error()
				}
				m.invalidate()
				return
			}
			m.closeDeploymentSync()
			m.showDeploymentReport("deploy_sync", server, report)
			m.RefreshDeploymentRemoteHost()
			m.invalidate()
		})
	}()
}

func (m *Model) deploymentSyncDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	if !state.syncOpen {
		return
	}
	open := state.syncOpen
	dialog := nativeModal(c.Key("deployment-sync"), &open, func() {
		ui.Column(c).Width(900).MaxHeight(760).Padding(18).Gap(8).Children(func() {
			ui.Text(c, "Sync with Deployed to "+state.syncServer.Label()).FontSize(14).Bold().
				TextColor(p.Text).SingleLine()
			if state.syncLoading {
				loadingState(c, p, "Comparing local and remote files…")
			} else {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Checkbox(c, &state.syncHideSame, "Hide identical files")
					ui.Spacer(c)
					if len(state.syncEntries) > 0 {
						all := true
						for _, entry := range state.syncEntries {
							if entry.Status != openidea.SyncSame && !state.syncChoices[entry.Path] {
								all = false
								break
							}
						}
						ui.Button(c, "Select All").OnClick(func() {
							for _, entry := range state.syncEntries {
								state.syncChoices[entry.Path] = entry.Status != openidea.SyncSame
							}
							m.invalidate()
						})
						if all {
							ui.Button(c, "Clear").OnClick(func() {
								for path := range state.syncChoices {
									state.syncChoices[path] = false
								}
								m.invalidate()
							})
						}
					}
				})
				columns := []string{"Action", "Size", "File", "Remote path"}
				ui.Row(c).Height(24).Gap(8).Children(func() {
					for _, column := range columns {
						ui.Text(c, column).FontSize(10).Bold().TextColor(p.TextMuted).Grow(1).SingleLine()
					}
				})
				shown := 0
				for _, entry := range state.syncEntries {
					entry := entry
					if state.syncHideSame && entry.Status == openidea.SyncSame {
						continue
					}
					shown++
					ui.Row(c).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
						checked := state.syncChoices[entry.Path]
						element := ui.Checkbox(c, &checked, deploymentSyncLabel(entry.Status))
						element.Disabled(entry.Status == openidea.SyncSame)
						if element.Changed() {
							state.syncChoices[entry.Path] = checked
							m.invalidate()
						}
						ui.Text(c, deploymentStampSize(entry.Local)).FontSize(10).
							TextColor(p.TextMuted).Width(70).SingleLine()
						ui.Text(c, entry.Path).FontSize(11).TextColor(p.Text).Grow(1).
							SingleLine().Ellipsis("…")
						ui.Text(c, entry.RemotePath).FontSize(10).Font("monospace").
							TextColor(p.TextMuted).Width(230).SingleLine().Ellipsis("…")
					})
				}
				if shown == 0 {
					emptyState(c, p, "No files to compare")
				}
			}
			if state.syncErr != "" {
				errorState(c, p, state.syncErr)
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Apply Selected").Disabled(state.syncLoading || state.syncBusy ||
					len(state.syncEntries) == 0).OnClick(m.applyDeploymentSync)
				ui.Button(c, "Close").OnClick(m.closeDeploymentSync)
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.closeDeploymentSync()
	}
}

func deploymentSyncLabel(status openidea.SyncStatus) string {
	switch status {
	case openidea.SyncLocalOnly:
		return "Upload"
	case openidea.SyncRemoteOnly:
		return "Download"
	case openidea.SyncLocalNewer:
		return "Upload newer"
	case openidea.SyncRemoteNewer:
		return "Download newer"
	case openidea.SyncDifferent:
		return "Upload changed"
	case openidea.SyncSame:
		return "Identical"
	default:
		return string(status)
	}
}

func deploymentStampSize(stamp *openidea.FileStamp) string {
	if stamp == nil {
		return "—"
	}
	if stamp.Size < 1024 {
		return fmt.Sprintf("%d B", stamp.Size)
	}
	if stamp.Size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(stamp.Size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(stamp.Size)/(1024*1024))
}

func (m *Model) deploymentStatusPanel(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	ui.Column(c).Width(290).Padding(10).Gap(7).BorderWidth(1).BorderColor(p.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Deployment").FontSize(12).Bold().TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			if state.loaded {
				ui.Text(c, strconv.Itoa(len(state.settings.Servers))+" servers").
					FontSize(10).TextColor(p.TextMuted).SingleLine()
			}
		})
		defaultServer := "No default server"
		if state.loaded && state.settings.DefaultServer != "" {
			for _, server := range state.settings.Servers {
				if server.ID == state.settings.DefaultServer {
					defaultServer = server.Label()
					break
				}
			}
		}
		ui.Text(c, defaultServer).FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		if state.busy {
			loadingState(c, p, state.busyLabel)
			if m.deploymentOperationCanCancel() {
				ui.Button(c, "Cancel").OnClick(m.cancelDeploymentOperation)
			}
		}
		if state.status != "" {
			ui.Text(c, state.status).FontSize(11).TextColor(p.TextMuted)
		}
		ui.Row(c).Gap(6).Children(func() {
			ui.Button(c, "Configure…").OnClick(m.OpenDeploymentSettings)
			ui.Button(c, "Remote Host").OnClick(m.OpenDeploymentRemoteHost)
		})
	})
}

func (m *Model) downloadDeploymentRemoteArg(arg any) {
	value, ok := arg.(map[string]any)
	if !ok {
		return
	}
	serverID, _ := value["server"].(string)
	target, _ := value["target"].(string)
	paths := stringPaths(value["paths"])
	if serverID == "" || target == "" || len(paths) == 0 {
		return
	}
	m.DeploymentRemoteDownload(serverID, paths, target)
}

func (m *Model) OpenDeploymentRemoteHost() {
	state := &m.remote.deployment
	state.remoteHostOpen = true
	m.DockShow(deploymentRemoteHostWindow)
	if state.loaded {
		server := deploymentServerByID(state.settings, state.remoteHostServer)
		if server == nil && len(state.settings.Servers) > 0 {
			server = deploymentServerByID(state.settings, state.settings.DefaultServer)
			if server == nil {
				server = &state.settings.Servers[0]
			}
		}
		if server != nil {
			m.loadDeploymentRemoteHost(*server, state.remoteHostPath)
		}
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Second)
		defer cancel()
		settings, err := m.loadDeploymentSettings(ctx)
		m.post(func() {
			if err != nil {
				state.remoteHostErr = err.Error()
				m.invalidate()
				return
			}
			state.loaded = true
			state.settings = settings
			if len(settings.Servers) == 0 {
				state.remoteHostErr = "No deployment server configured."
				m.invalidate()
				return
			}
			server := deploymentServerByID(settings, settings.DefaultServer)
			if server == nil {
				server = &settings.Servers[0]
			}
			m.loadDeploymentRemoteHost(*server, "")
		})
	}()
}

func deploymentServerByID(settings openidea.DeploymentSettings, id string) *openidea.DeploymentServer {
	for i := range settings.Servers {
		if settings.Servers[i].ID == id {
			return &settings.Servers[i]
		}
	}
	return nil
}

func (m *Model) loadDeploymentRemoteHost(server openidea.DeploymentServer, path string) {
	state := &m.remote.deployment
	if state.remoteHostLoading {
		return
	}
	state.remoteHostLoading = true
	state.remoteHostErr = ""
	state.remoteHostServer = server.ID
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 60*time.Second)
		defer cancel()
		root := state.remoteHostRoot
		if root == "" {
			raw, err := m.invoke(ctx, "deploy_server_root", map[string]any{"server": server.ID})
			if err != nil {
				m.post(func() {
					state.remoteHostLoading = false
					if !m.offerDeploymentAuth(err.Error(), server, func() {
						state.remoteHostRoot = ""
						m.loadDeploymentRemoteHost(server, path)
					}) {
						state.remoteHostErr = err.Error()
					}
					m.invalidate()
				})
				return
			}
			var info openidea.DeploymentRoot
			if err := json.Unmarshal(raw, &info); err != nil {
				m.post(func() {
					state.remoteHostLoading = false
					state.remoteHostErr = err.Error()
					m.invalidate()
				})
				return
			}
			root = info.Root
		}
		if strings.TrimSpace(path) == "" {
			path = root
		}
		raw, err := m.invoke(ctx, "remote_list", map[string]any{
			"connection": server.Connection(), "path": path,
		})
		var entries []openidea.RemoteEntry
		if err == nil {
			err = json.Unmarshal(raw, &entries)
		}
		m.post(func() {
			state.remoteHostLoading = false
			if err != nil {
				if !m.offerDeploymentAuth(err.Error(), server, func() {
					m.loadDeploymentRemoteHost(server, path)
				}) {
					state.remoteHostErr = err.Error()
				}
				m.invalidate()
				return
			}
			state.remoteHostRoot = root
			state.remoteHostPath = path
			state.remoteHostEntries = entries
			state.remoteHostNew = false
			state.remoteHostNewName = ""
			m.invalidate()
		})
	}()
}

func (m *Model) RefreshDeploymentRemoteHost() {
	state := &m.remote.deployment
	if !state.remoteHostOpen {
		return
	}
	server := deploymentServerByID(state.settings, state.remoteHostServer)
	if server != nil {
		m.loadDeploymentRemoteHost(*server, state.remoteHostPath)
	}
}

func (m *Model) deploymentRemoteHostWindow(c *ui.Context, p *ui.Theme) {
	state := &m.remote.deployment
	ui.Column(c).Key("deployment:remote-host").Grow(1).Padding(10).Gap(6).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			names := make([]string, len(state.settings.Servers))
			selected := ""
			for i, server := range state.settings.Servers {
				names[i] = server.Label()
				if server.ID == state.remoteHostServer {
					selected = names[i]
				}
			}
			if len(names) > 0 {
				if selected == "" {
					selected = names[0]
				}
				ui.Select(c, &selected, names).Width(180)
				if i := slicesIndex(names, selected); i >= 0 &&
					state.settings.Servers[i].ID != state.remoteHostServer {
					server := state.settings.Servers[i]
					state.remoteHostRoot = ""
					state.remoteHostPath = ""
					m.loadDeploymentRemoteHost(server, "")
				}
			} else {
				ui.Text(c, "No server").FontSize(11).TextColor(p.TextMuted)
			}
			ui.Spacer(c)
			ui.Button(c, "Refresh").OnClick(m.RefreshDeploymentRemoteHost)
			ui.Button(c, "Configure…").OnClick(m.OpenDeploymentSettings)
		})
		ui.Text(c, state.remoteHostPath).FontSize(10).Font("monospace").TextColor(p.TextMuted).
			SingleLine().Ellipsis("…")
		if state.remoteHostLoading {
			loadingState(c, p, "Loading remote files…")
		}
		if state.remoteHostErr != "" {
			errorState(c, p, state.remoteHostErr)
		}
		if state.remoteHostPath != "" && state.remoteHostPath != state.remoteHostRoot {
			ui.Button(c, "..").OnClick(func() {
				server := deploymentServerByID(state.settings, state.remoteHostServer)
				if server != nil {
					m.loadDeploymentRemoteHost(*server, remoteParentPath(state.remoteHostPath))
				}
			})
		}
		for _, entry := range state.remoteHostEntries {
			entry := entry
			ui.Row(c).Height(28).Gap(6).AlignItems(ui.Center).Children(func() {
				label := entry.Name
				if entry.IsDir {
					label = "▸ " + label + "/"
				}
				ui.Button(c, label).Grow(1).OnClick(func() {
					server := deploymentServerByID(state.settings, state.remoteHostServer)
					if server == nil {
						return
					}
					if entry.IsDir {
						m.loadDeploymentRemoteHost(*server, entry.Path)
					} else {
						m.openDeploymentRemoteFile(*server, entry.Path)
					}
				})
				ui.Text(c, formatRemoteEntrySize(entry.Size)).FontSize(10).
					TextColor(p.TextMuted).Width(70).SingleLine()
				ui.Button(c, "Download").OnClick(func() {
					server := deploymentServerByID(state.settings, state.remoteHostServer)
					if server != nil {
						m.DeploymentRemoteDownload(server.ID, []string{entry.Path}, m.Root())
					}
				})
			})
		}
		if !state.remoteHostLoading && len(state.remoteHostEntries) == 0 {
			emptyState(c, p, "This folder is empty")
		}
	})
}

func formatRemoteEntrySize(size uint64) string {
	if size == 0 {
		return ""
	}
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func (m *Model) openDeploymentRemoteFile(server openidea.DeploymentServer, path string) {
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "remote_open", map[string]any{
			"connection": server.Connection(), "path": path,
		})
		var local string
		if err == nil {
			err = json.Unmarshal(raw, &local)
		}
		m.post(func() {
			if err != nil {
				if !m.offerDeploymentAuth(err.Error(), server, func() {
					m.openDeploymentRemoteFile(server, path)
				}) {
					m.notifyError("deployment.remoteOpen", "Cannot open remote file", err.Error())
				}
				m.invalidate()
				return
			}
			m.openProjectPath(local)
		})
	}()
}

// DeploymentRemoteDownload downloads remote files, each under its own name, into target.
func (m *Model) DeploymentRemoteDownload(serverID string, paths []string, target string) {
	state := &m.remote.deployment
	server := deploymentServerByID(state.settings, serverID)
	if server == nil {
		return
	}
	request := m.nextRequest()
	state.busy = true
	state.request = request
	state.busyLabel = "Downloading from " + server.Label() + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.deploymentContext(), 30*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "remote_download_into", map[string]any{
			"connection": server.Connection(), "paths": paths, "target": target, "request": request,
		})
		var report openidea.TransferReport
		if err == nil {
			err = json.Unmarshal(raw, &report)
		}
		m.post(func() {
			state.busy = false
			state.request = 0
			state.busyLabel = ""
			if err != nil {
				if !m.offerDeploymentAuth(err.Error(), *server, func() {
					m.DeploymentRemoteDownload(serverID, paths, target)
				}) {
					m.notifyError("deployment.downloadInto", "Cannot download remote files", err.Error())
				}
			} else {
				m.showDeploymentReport("deploy_download", *server, report)
			}
			m.invalidate()
		})
	}()
}
