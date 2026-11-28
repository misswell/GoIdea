package app

// Native Remote Development surfaces. OpenIDEA keeps the SSH, WSL and dev
// container orchestration in its desktop shell and connects that transport to
// the same openidea-server protocol the local window uses. GoIDEA ports that
// orchestration here while MyGo draws every control.

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const remoteDevelopmentWindow = "remote"
const remotePortsWindow = "remote.ports"

var openRemoteWebPage = openReleasePage

type remoteDevelopmentState struct {
	connectOpen    bool
	connectTarget  openidea.RemoteTargetKind
	connectPath    string
	disconnectOpen bool
	noticeOpen     bool
	noticeTitle    string
	noticeBody     string

	configsLoading bool
	configsLoaded  bool
	configsErr     string
	sshConfigs     []openidea.SSHConfiguration
	sshConfigID    string
	sshChoice      string
	sshEditID      string
	distros        []string
	distro         string

	busy           bool
	busyLabel      string
	progress       *openidea.RemoteProgress
	connectErr     string
	connection     *remoteConnectionRun
	generation     uint64
	browseOpen     bool
	browseDir      string
	browseEntries  []openidea.RemoteDirectoryEntry
	browseErr      string
	browseRetryDir string
	browseRequest  uint64

	session       *openidea.RemoteSession
	backend       *remoteBackend
	secrets       *remoteSecretVault
	pendingTarget openidea.RemoteTarget
	pendingPath   string
	pendingSSH    openidea.SSHConfiguration
	restoreFiles  []string

	secretOpen   bool
	secretTitle  string
	secretLabel  string
	secretValue  string
	secretSave   bool
	secretConfig openidea.SSHConfiguration

	trustOpen        bool
	trustHost        string
	trustFingerprint string
	trustConfig      openidea.SSHConfiguration

	sshSettingsOpen    bool
	sshSettingsPending bool
	sshEditing         bool
	sshSelected        int
	sshName            string
	sshHost            string
	sshPort            string
	sshUser            string
	sshAuth            string
	sshKeyFile         string
	sshSecret          string
	sshSaveSecret      bool
	sshSettingsErr     string
	sshSettingsBusy    bool
	sshSettingsTesting bool
	sshTestResult      string
	sshTestErr         string
	sshTestRemedy      remoteSSHRemedy
	sshTestFingerprint string
	sshTestConfig      openidea.SSHConfiguration
	sshSettingsRequest uint64

	sshSessionOpen        bool
	sshSessionPendingID   string
	sshSessionWaiting     bool
	sshSessionBusy        bool
	sshSessionErr         string
	sshSessionResult      string
	sshSessionSecret      string
	sshSessionRemedy      remoteSSHRemedy
	sshSessionFingerprint string
	sshSessionConfig      openidea.SSHConfiguration
	sshSessionRequest     uint64

	ports       []openidea.ForwardedPort
	portsInput  string
	portsErr    string
	portsWindow bool

	importBusy    bool
	importLabel   string
	importRequest uint32
	importCancel  context.CancelFunc
	importDone    chan struct{}

	deployment remoteDeploymentState
	runTargets remoteRunTargetState

	envBusy         bool
	envLoaded       bool
	envErr          string
	envChanged      []string
	envDevcontainer *string
	envShellRead    bool
	envRequest      uint64

	webPreviews             map[string]bool
	devContainerCLIStatus   string
	devContainerCLIPath     string
	devContainerCLIVersion  string
	devContainerCLIMessage  string
	devContainerCLIChecking bool
}

type remoteEnvReloadResult struct {
	Changed      []string `json:"changed"`
	Devcontainer *string  `json:"devcontainer"`
	ShellRead    bool     `json:"shellRead"`
}

func init() {
	for _, action := range []*Action{
		{
			ID:          "web.openInBrowser",
			Title:       "Open in Browser",
			Category:    "View",
			Description: "Opens an HTML or SVG file in the default browser through the IDE backend.",
			Keywords:    "browser html svg preview run web",
			When:        remoteProjectOpen,
			RunWithArg:  func(m *Model, arg any) { m.OpenWebInBrowser(arg) },
		},
		{
			ID:          "web.togglePreview",
			Title:       "Built-in Preview",
			Category:    "View",
			Description: "Shows the HTML file in an external browser when the native view has no WebView.",
			Keywords:    "browser html preview live external",
			When:        webPreviewAvailable,
			RunWithArg:  func(m *Model, arg any) { m.ToggleWebPreview(arg) },
			Toggled:     webPreviewToggled,
		},
		{
			ID:          "remote.connect",
			Title:       "Remote Development…",
			Category:    "File",
			Description: "Opens a project folder on a remote host.",
			Keywords:    "ssh remote server gateway connect host wsl devcontainer",
			Run:         func(m *Model) { m.OpenRemoteConnectDialog() },
		},
		{
			ID:          "remote.disconnect",
			Title:       "Disconnect from Remote Host",
			Category:    "File",
			Description: "Closes a remote-host session after confirmation.",
			Keywords:    "ssh remote disconnect close",
			When:        remoteProjectOpen,
			Run:         func(m *Model) { m.OpenRemoteDisconnectDialog() },
		},
		{
			ID:          "remote.reloadEnvironment",
			Title:       "Reload Environment",
			Category:    "File",
			Description: "Reloads the login shell environment for new processes.",
			Keywords:    "env environment variables path shell remoteEnv devcontainer reload",
			When:        remoteProjectOpen,
			Run:         func(m *Model) { m.ReloadRemoteEnvironment() },
		},
		{
			ID:          "remote.reopenInContainer",
			Title:       "Reopen in Container",
			Category:    "File",
			Description: "Reopens the local project in its development container.",
			Keywords:    "devcontainer dev container docker reopen remote",
			When:        remoteProjectOpen,
			Run:         func(m *Model) { m.ReopenInContainer() },
		},
		{
			ID:          "remote.devContainerCLI",
			Title:       "Dev Containers CLI Status",
			Category:    "Tools",
			Description: "Checks the GoIDEA-managed Dev Containers CLI.",
			Keywords:    "devcontainer docker cli install status remote container",
			When:        remoteProjectOpen,
			Run:         func(m *Model) { m.CheckDevContainerCLI() },
		},
		{
			ID:       "remote.showToolWindow",
			Title:    "Remote Development",
			Category: "View",
			Keywords: "remote host ports tool window ssh wsl devcontainer",
			Run:      func(m *Model) { m.DockToggle(remoteDevelopmentWindow) },
			Toggled:  func(m *Model) bool { return m.dockWindowShown(remoteDevelopmentWindow) },
		},
		{
			ID:       "remote.showPorts",
			Title:    "Ports",
			Category: "View",
			Keywords: "remote forwarded ports tool window",
			When:     remotePortsAvailable,
			Run:      func(m *Model) { m.DockToggle(remotePortsWindow) },
			Toggled:  func(m *Model) bool { return m.dockWindowShown(remotePortsWindow) },
		},
		{
			ID:          "remote.cancelImport",
			Title:       "Cancel Remote Import",
			Category:    "File",
			Description: "Stops importing local files into the connected remote project.",
			Keywords:    "remote import upload cancel copy paste",
			When:        func(m *Model) bool { return m.remote.importBusy },
			Run:         func(m *Model) { m.CancelRemoteImport() },
		},
		{
			ID:          "ssh.startSession",
			Title:       "Start SSH Session…",
			Category:    "Tools",
			Description: "Opens a terminal on an SSH configuration.",
			Keywords:    "ssh remote terminal shell connect",
			When:        remoteProjectOpen,
			RunWithArg:  func(m *Model, arg any) { m.StartRemoteSSHSession(arg) },
		},
		{
			ID:          "ssh.configurations",
			Title:       "SSH Configurations…",
			Category:    "Tools",
			Description: "Edits the SSH configurations shared by remote tools.",
			Keywords:    "ssh remote host settings",
			Run:         func(m *Model) { m.openRemoteSSHSettings() },
		},
	} {
		_ = registerAction(action)
	}

	registerToolWindow(&toolWindowSpec{
		id:     remoteDevelopmentWindow,
		title:  "Remote Development",
		icon:   "toolwindows/terminal",
		anchor: dockBottom,
		order:  35,
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.remoteDevelopmentPanel(c, p)
		},
	})
	registerStatusItem(&StatusItem{
		ID: "remote.location", Title: "Remote Host", Side: "left", Order: 100,
		Visible: remoteConnected,
		Render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			if m.remote.session == nil {
				return
			}
			label := remoteSessionBadge(*m.remote.session)
			ui.Text(c, label).FontSize(11).TextColor(p.Text).
				Cursor(ui.CursorPointer).OnClick(m.OpenRemoteConnectDialog)
		},
	})
}

func remoteSessionBadge(session openidea.RemoteSession) string {
	kind := "SSH"
	switch session.Target.Kind {
	case openidea.RemoteTargetWSL:
		kind = "WSL"
	case openidea.RemoteTargetDevContainer:
		kind = "Dev Container"
	}
	return kind + ": " + session.Host
}

func isWebPath(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".html", ".htm", ".xhtml", ".shtml", ".svg":
		return true
	default:
		return false
	}
}

func isHTMLPath(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".html", ".htm", ".xhtml", ".shtml":
		return true
	default:
		return false
	}
}

func webPreviewAvailable(m *Model) bool {
	tab := m.tab()
	return tab != nil && tab.doc != nil && isHTMLPath(tab.doc.Path())
}

func webPreviewToggled(m *Model) bool {
	tab := m.tab()
	return tab != nil && tab.doc != nil && m.remote.webPreviews[tab.doc.Path()]
}

func webPathArg(arg any, fallback string) string {
	switch value := arg.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		if path, ok := value["path"].(string); ok {
			return strings.TrimSpace(path)
		}
	}
	return fallback
}

func (m *Model) OpenWebInBrowser(arg any) {
	target := ""
	if tab := m.tab(); tab != nil && tab.doc != nil {
		target = tab.doc.Path()
	}
	if selected, ok := m.selectedTreeEntry(); ok && !selected.dir {
		target = selected.path
	}
	target = webPathArg(arg, target)
	if target == "" || !isWebPath(target) {
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		m.notifyError("web-open", "Cannot open the browser", "The GoIDEA backend is not connected.")
		return
	}
	baseContext := m.remoteBaseContext()
	tabs := append([]*Tab(nil), m.allTabs()...)
	var remoteBackend *remoteBackend
	if remoteConnected(m) {
		remoteBackend = m.remote.backend
	}
	m.status = "Opening " + m.projectTreePathBase(target) + " in the browser…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(baseContext, 30*time.Second)
		defer cancel()
		if err := m.saveOpenTabs(ctx, tabs); err != nil {
			m.post(func() {
				m.notifyError("web-open", "Cannot open the browser", err.Error())
				m.invalidate()
			})
			return
		}
		raw, err := m.invoke(ctx, "web_url", map[string]any{"path": target})
		var rawURL string
		if err == nil {
			err = json.Unmarshal(raw, &rawURL)
		}
		if err == nil && remoteBackend != nil {
			rawURL, err = remoteBackend.forwardWebURL(rawURL)
		}
		m.post(func() {
			if err != nil {
				m.notifyError("web-open", "Cannot open the browser", err.Error())
				m.invalidate()
				return
			}
			openRemoteWebPage(rawURL)
			m.status = "Opened " + rawURL
			m.invalidate()
		})
	}()
}

func (m *Model) ToggleWebPreview(arg any) {
	target := ""
	if tab := m.tab(); tab != nil && tab.doc != nil {
		target = tab.doc.Path()
	}
	target = webPathArg(arg, target)
	if target == "" || !isHTMLPath(target) {
		return
	}
	if m.remote.webPreviews == nil {
		m.remote.webPreviews = map[string]bool{}
	}
	on := !m.remote.webPreviews[target]
	m.remote.webPreviews[target] = on
	if on {
		m.OpenWebInBrowser(target)
		return
	}
	m.status = "Built-in Preview is off for " + m.projectTreePathBase(target)
	m.invalidate()
}

func remoteProjectOpen(m *Model) bool {
	return m.Root() != "" && !m.showingWelcome
}

func remoteConnected(m *Model) bool {
	return m != nil && m.remote.session != nil
}

func remotePortsAvailable(m *Model) bool {
	return remoteConnected(m) && (m.remote.session.Target.Kind == openidea.RemoteTargetSSH ||
		m.remote.session.Target.Kind == openidea.RemoteTargetDevContainer)
}

// OpenRemoteConnectDialog opens the native remote target chooser.
func (m *Model) OpenRemoteConnectDialog() {
	if m.remote.busy {
		m.DockShow(remoteDevelopmentWindow)
		return
	}
	m.remote.connectOpen = true
	if m.remote.connectTarget == "" {
		m.remote.connectTarget = openidea.RemoteTargetSSH
	}
	if m.remote.connectPath == "" {
		m.remote.connectPath = "~"
	}
	m.loadRemoteChoices()
	m.invalidate()
}

func (m *Model) OpenRemoteDisconnectDialog() {
	m.remote.disconnectOpen = true
	m.invalidate()
}

func (m *Model) showRemoteNotice(title, body string) {
	m.remote.noticeOpen = true
	m.remote.noticeTitle = title
	m.remote.noticeBody = body
	m.invalidate()
}

func (m *Model) confirmRemoteDisconnect() { m.DisconnectRemoteProject() }

// ReloadRemoteEnvironment calls env_reload. That RPC refreshes the login shell
// environment and the trusted project's devcontainer.json remoteEnv. Running
// terminals and processes keep the environment they already have.
func (m *Model) ReloadRemoteEnvironment() {
	state := &m.remote
	if state.envBusy {
		return
	}
	if m.Root() == "" {
		state.envBusy = false
		state.envLoaded = false
		state.envErr = "Open a project before reloading the environment."
		m.DockShow(remoteDevelopmentWindow)
		m.invalidate()
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		state.envBusy = false
		state.envLoaded = false
		state.envErr = "The GoIDEA backend is not connected."
		m.DockShow(remoteDevelopmentWindow)
		m.invalidate()
		return
	}

	state.envBusy = true
	state.envLoaded = false
	state.envErr = ""
	state.envChanged = nil
	state.envDevcontainer = nil
	state.envShellRead = false
	root := m.Root()
	generation := state.generation
	state.envRequest++
	request := state.envRequest
	m.status = "Reloading environment…"
	m.DockShow(remoteDevelopmentWindow)
	m.invalidate()

	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "env_reload", nil)
		m.post(func() {
			if state.envRequest != request || state.generation != generation ||
				m.Root() != root {
				return
			}
			state.envBusy = false
			if err != nil {
				state.envErr = "Could not reload the environment: " + err.Error()
				m.status = state.envErr
				m.invalidate()
				return
			}
			var result remoteEnvReloadResult
			if decodeErr := json.Unmarshal(raw, &result); decodeErr != nil {
				state.envErr = "Could not read the environment reload response: " + decodeErr.Error()
				m.status = state.envErr
				m.invalidate()
				return
			}
			state.envLoaded = true
			state.envChanged = append([]string(nil), result.Changed...)
			state.envDevcontainer = result.Devcontainer
			state.envShellRead = result.ShellRead
			state.envErr = ""
			m.status = "Environment reloaded"
			m.invalidate()
		})
	}()
}

func (m *Model) RestartRemoteLanguageServers() {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	root := m.Root()
	generation := m.remote.generation
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_servers", nil)
		var servers []lspServerState
		if err == nil {
			err = json.Unmarshal(raw, &servers)
		}
		if err != nil {
			m.post(func() {
				if m.Root() != root || m.remote.generation != generation {
					return
				}
				m.notifyError("lsp", "Failed to restart language servers", err.Error())
				m.invalidate()
			})
			return
		}
		for _, server := range servers {
			if server.ID == 0 {
				continue
			}
			if _, restartErr := m.invoke(ctx, "lsp_restart", map[string]any{"server": server.ID}); restartErr != nil {
				m.post(func() {
					if m.Root() != root || m.remote.generation != generation {
						return
					}
					m.notifyError("lsp", "Failed to restart language server", restartErr.Error())
					m.invalidate()
				})
				return
			}
		}
		m.post(func() {
			if m.Root() != root || m.remote.generation != generation {
				return
			}
			m.status = "Restarted language servers"
			m.RefreshLanguageServers()
			m.invalidate()
		})
	}()
}

func (m *Model) remoteDevelopmentPanel(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	ui.Column(c).Key("remote:window").Grow(1).Padding(10, 12, 10, 12).Gap(10).
		Children(func() {
			ui.Row(c).Height(26).Shrink(0).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Remote Development").FontSize(13).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				ui.Text(c, m.remoteConnectionLabel()).FontSize(12).
					TextColor(m.remoteConnectionColor(p)).SingleLine()
			})
			ui.Row(c).Grow(1).Gap(12).Children(func() {
				m.remoteHostPanel(c, p, state)
				m.remotePortsPanel(c, p)
				m.deploymentStatusPanel(c, p)
			})
		})
}

func (m *Model) remoteHostPanel(c *ui.Context, p *ui.Theme, state *remoteDevelopmentState) {
	ui.Column(c).Grow(1).MinWidth(0).Padding(10).Gap(7).
		BorderWidth(1).BorderColor(p.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Remote Host").FontSize(12).Bold().TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			ui.Text(c, m.remoteConnectionLabel()).FontSize(11).
				TextColor(m.remoteConnectionColor(p)).SingleLine()
		})
		if state.session == nil {
			ui.Text(c, "GoIDEA is using its local backend.").FontSize(11).
				TextColor(p.TextMuted)
			ui.Text(c, "Open a project on an SSH host, in WSL, or in a dev container.").
				FontSize(11).TextColor(p.TextMuted)
		} else {
			ui.Text(c, state.session.Host+" · "+state.session.Root).FontSize(11).
				TextColor(p.Text).SingleLine().Ellipsis("…")
			ui.Text(c, remoteTargetLabel(state.session.Target)+" · GoIDEA backend "+
				state.session.ServerVersion+" · "+state.session.OS).
				FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			ui.Text(c, m.remoteConnectionStatus()).FontSize(11).
				TextColor(m.remoteConnectionColor(p)).SingleLine().Ellipsis("…")
		}
		if state.busy {
			loadingState(c, p, state.busyLabel)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Dev Containers CLI").FontSize(11).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, m.devContainerCLISummary()).FontSize(11).TextColor(p.TextMuted).
				Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "Check").Disabled(state.devContainerCLIChecking).
				OnClick(m.CheckDevContainerCLI)
			if !state.devContainerCLIChecking && !m.devContainerCLIAvailable() {
				ui.Button(c, "Install…").OnClick(m.InstallDevContainerCLI)
			}
		})
		if state.progress != nil {
			ui.Text(c, remoteStageLabel(state.progress.Stage)).FontSize(11).
				TextColor(p.TextMuted).SingleLine()
			if fraction := state.progress.Fraction; fraction != nil {
				ui.Text(c, fmt.Sprintf("%.0f%%", *fraction*100)).FontSize(11).
					TextColor(p.TextMuted).SingleLine()
			}
		}
		if state.importBusy {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				label := state.importLabel
				if label == "" {
					label = "Importing local files…"
				}
				ui.Text(c, label).FontSize(11).TextColor(p.TextMuted).
					Grow(1).SingleLine().Ellipsis("…")
				ui.Button(c, "Cancel").OnClick(m.CancelRemoteImport)
			})
		}
		if state.connectErr != "" {
			errorState(c, p, state.connectErr)
			if !state.busy && !state.secretOpen && !state.trustOpen &&
				(state.pendingTarget.Kind != "" || state.session != nil) {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Button(c, "Retry").OnClick(func() {
						if !m.retryRemoteConnection() {
							m.retryRemoteBackend()
						}
					})
				})
			}
		} else if state.session != nil {
			backendState := m.BackendState()
			if !backendState.Running && backendState.Phase != "starting" &&
				backendState.Phase != "restarting" {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Button(c, "Retry").Disabled(state.busy).
						OnClick(m.retryRemoteBackend)
				})
			}
		}
		ui.Row(c).Gap(6).Children(func() {
			ui.Button(c, "Connect…").Disabled(state.busy || remoteConnected(m)).
				OnClick(m.OpenRemoteConnectDialog)
			ui.Button(c, "Disconnect").Disabled(!remoteConnected(m)).
				OnClick(m.OpenRemoteDisconnectDialog)
			ui.Button(c, "Reload Environment").
				Disabled(!remoteProjectOpen(m) || state.envBusy || state.busy).
				OnClick(m.ReloadRemoteEnvironment)
		})
		switch {
		case state.envBusy:
			loadingState(c, p, "Reloading environment…")
		case state.envErr != "":
			errorState(c, p, state.envErr)
		case state.envLoaded:
			ui.Text(c, "Environment").FontSize(11).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, m.remoteEnvironmentSummary()).FontSize(11).TextColor(p.TextMuted)
			if len(state.envChanged) > 0 {
				ui.Button(c, "Restart Language Servers").
					OnClick(m.RestartRemoteLanguageServers)
			}
		}
	})
}

func (m *Model) remotePortsPanel(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	ui.Column(c).Grow(1).MinWidth(0).Padding(10).Gap(7).
		BorderWidth(1).BorderColor(p.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Ports").FontSize(12).Bold().TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			label := "No remote session"
			if remotePortsAvailable(m) {
				label = fmt.Sprintf("%d forwarded", len(state.ports))
			} else if remoteConnected(m) {
				label = "Not supported in WSL"
			}
			ui.Text(c, label).FontSize(11).TextColor(p.TextMuted).SingleLine()
		})
		if !remotePortsAvailable(m) {
			message := "Connect to an SSH host or dev container to forward ports."
			if remoteConnected(m) {
				message = "Forwarding ports is not supported in a WSL project yet."
			}
			ui.Text(c, message).FontSize(11).TextColor(p.TextMuted)
			return
		}
		ui.Row(c).Gap(6).Children(func() {
			input := ui.TextInput(c, &state.portsInput).Height(26).Grow(1).
				Placeholder("Remote port, e.g. 3000")
			input.OnSubmit(func() { m.forwardRemotePortFromInput() })
			ui.Button(c, "Forward Port").Disabled(!remotePortInputValid(state.portsInput)).
				OnClick(m.forwardRemotePortFromInput)
		})
		if state.portsErr != "" {
			errorState(c, p, state.portsErr)
			ui.Row(c).Gap(6).Children(func() {
				ui.Button(c, "Retry").OnClick(m.retryRemotePorts)
				ui.Button(c, "Refresh").OnClick(m.refreshRemotePorts)
			})
		}
		if len(state.ports) == 0 {
			emptyState(c, p, "No forwarded ports")
			ui.Text(c, "Forward a port of the remote host to reach it from this machine.").
				FontSize(11).TextColor(p.TextMuted)
			return
		}
		for _, forwarded := range state.ports {
			forwarded := forwarded
			ui.Row(c).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, remoteForwardLabel(forwarded)).FontSize(11).
					TextColor(p.Text).Width(150).SingleLine().Ellipsis("…")
				ui.Text(c, "→").FontSize(11).TextColor(p.TextMuted).SingleLine()
				ui.Text(c, fmt.Sprintf("localhost:%d", forwarded.LocalPort)).FontSize(11).
					TextColor(p.Text).Grow(1).SingleLine()
				ui.Button(c, "Open").OnClick(func() {
					m.openRemoteForwardedPort(forwarded.LocalPort)
				})
				ui.Button(c, "Copy").OnClick(func() {
					address := fmt.Sprintf("localhost:%d", forwarded.LocalPort)
					c.WriteClipboard(address)
					m.status = "Copied " + address
					m.invalidate()
				})
				ui.Button(c, "Stop").OnClick(func() {
					m.stopRemoteForward(forwarded.RemotePort, forwarded.Host)
				})
			})
		}
	})
}

func (m *Model) remoteConnectionLabel() string {
	if m.remote.session == nil {
		return "Not connected"
	}
	return remoteTargetLabel(m.remote.session.Target)
}

func (m *Model) remoteConnectionColor(p *ui.Theme) ui.Color {
	if m.remote.session == nil {
		return p.TextMuted
	}
	state := m.BackendState()
	switch {
	case state.Running:
		return p.Success
	case state.Phase == "starting" || state.Phase == "restarting":
		return p.Warning
	default:
		return p.Danger
	}
}

func (m *Model) remoteConnectionStatus() string {
	if m.remote.session == nil {
		return ""
	}
	state := m.BackendState()
	switch state.Phase {
	case "starting":
		return "Connecting to the remote backend…"
	case "restarting":
		if state.Err != nil {
			return "Connection lost; reconnecting…"
		}
		return "Reconnecting to the remote host…"
	}
	if state.Running {
		return "Connected"
	}
	if state.Err != nil {
		return "Connection unavailable: " + state.Err.Error()
	}
	if state.Phase != "" {
		return "Connection state: " + state.Phase
	}
	return "Connection state unavailable"
}

func remoteTargetLabel(target openidea.RemoteTarget) string {
	switch target.Kind {
	case openidea.RemoteTargetSSH:
		return "SSH: " + target.Config
	case openidea.RemoteTargetWSL:
		return "WSL: " + target.Distro
	case openidea.RemoteTargetDevContainer:
		return "Dev Container: " + target.Label()
	default:
		return string(target.Kind)
	}
}

func (m *Model) remoteEnvironmentSummary() string {
	state := &m.remote
	changed := "No variables changed."
	if len(state.envChanged) == 1 {
		changed = "Changed: " + state.envChanged[0] + "."
	} else if len(state.envChanged) > 1 {
		names := state.envChanged
		more := 0
		if len(names) > 8 {
			more = len(names) - 8
			names = names[:8]
		}
		changed = "Changed: " + strings.Join(names, ", ")
		if more > 0 {
			changed += fmt.Sprintf(" and %d more", more)
		}
		changed += "."
	}

	shell := "The login shell was not read."
	if state.envShellRead {
		shell = "The login shell was read."
	}
	devcontainer := "No devcontainer.json remote environment was applied."
	if state.envDevcontainer != nil {
		devcontainer = "Applied devcontainer.json remote environment from " + *state.envDevcontainer + "."
	}
	return changed + " " + shell + " " + devcontainer + " New terminals and processes get the updated environment; running ones keep their old environment."
}

func (m *Model) remoteConnectDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	if !state.connectOpen {
		return
	}
	open := state.connectOpen
	dialog := nativeModal(c.Key("remote-connect-dialog"), &open, func() {
		ui.Column(c).Width(640).Padding(18).Gap(10).Children(func() {
			ui.Text(c, "Remote Development").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "The project runs on the remote host; the IDE backend is installed there on first use.").
				FontSize(12).TextColor(p.TextMuted)
			if remoteConnected(m) {
				ui.Text(c, "Disconnect from the current remote host before opening another remote project.").
					FontSize(12).TextColor(p.TextMuted)
			}
			if len(state.distros) > 0 {
				ui.Text(c, "Connect through:").FontSize(11).Bold().TextColor(p.Text).SingleLine()
				kind := "SSH"
				if state.connectTarget == openidea.RemoteTargetWSL {
					kind = "WSL"
				}
				if choice := ui.Select(c, &kind, []string{"SSH", "WSL"}).Width(180); choice.Changed() {
					if kind == "WSL" {
						state.connectTarget = openidea.RemoteTargetWSL
					} else {
						state.connectTarget = openidea.RemoteTargetSSH
					}
					state.browseOpen = false
					m.invalidate()
				}
			}
			if state.connectTarget == openidea.RemoteTargetWSL {
				ui.Text(c, "WSL distribution:").FontSize(11).Bold().TextColor(p.Text).SingleLine()
				if len(state.distros) == 0 {
					ui.Text(c, "No WSL distributions were found.").FontSize(12).
						TextColor(p.TextMuted)
				} else {
					ui.Select(c, &state.distro, state.distros).Width(320)
				}
			} else {
				ui.Text(c, "SSH configuration:").FontSize(11).Bold().TextColor(p.Text).SingleLine()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					switch {
					case state.configsLoading:
						ui.Text(c, "Loading SSH configurations…").FontSize(12).
							TextColor(p.TextMuted).Grow(1)
					case state.configsErr != "":
						ui.Text(c, state.configsErr).FontSize(12).
							TextColor(p.Danger).Grow(1)
					case len(state.sshConfigs) == 0:
						ui.Text(c, "No SSH configurations").FontSize(12).
							TextColor(p.TextMuted).Grow(1)
					default:
						labels := remoteSSHConfigurationChoices(state.sshConfigs)
						if state.sshChoice == "" {
							state.sshChoice = labels[0]
						}
						ui.Select(c, &state.sshChoice, labels).Grow(1)
					}
					ui.Button(c, "Configure…").OnClick(m.openRemoteSSHSettings)
				})
				if state.configsErr != "" {
					ui.Button(c, "Retry SSH Configurations").Disabled(state.configsLoading).
						OnClick(m.retryRemoteChoices)
				}
			}
			ui.Text(c, "Project directory:").FontSize(11).Bold().TextColor(p.Text).SingleLine()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				pathInput := ui.TextInput(c, &state.connectPath).Grow(1).Height(28).
					Placeholder("~ or /absolute/remote/path")
				pathInput.OnSubmit(func() { m.connectRemoteFromDialog() })
				if state.connectTarget == openidea.RemoteTargetSSH {
					ui.Button(c, "Browse").Disabled(state.busy ||
						selectedRemoteSSHConfiguration(state) == nil).
						OnClick(func() { m.browseRemoteFolders(state.connectPath) })
				}
			})
			if state.browseErr != "" {
				errorState(c, p, state.browseErr)
				ui.Button(c, "Retry Browse").OnClick(m.retryRemoteFolderBrowse)
			}
			if state.browseOpen {
				ui.Column(c).Height(150).Gap(2).BorderWidth(1).
					BorderColor(p.Border).Padding(6).Children(func() {
					ui.Text(c, state.browseDir).FontSize(11).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
					if state.browseDir != "/" {
						ui.Button(c, "..").OnClick(func() {
							m.browseRemoteFolders(remoteParentPath(state.browseDir))
						})
					}
					for _, entry := range state.browseEntries {
						entry := entry
						ui.Button(c, entry.Name).OnClick(func() {
							m.browseRemoteFolders(entry.Path)
						})
					}
					if len(state.browseEntries) == 0 {
						emptyState(c, p, "No subfolders")
					}
				})
			}
			if state.progress != nil {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, remoteStageLabel(state.progress.Stage)).FontSize(12).
						TextColor(p.Text).Grow(1).SingleLine()
					if state.progress.Fraction != nil {
						ui.Text(c, fmt.Sprintf("%.0f%%", *state.progress.Fraction*100)).
							FontSize(11).TextColor(p.TextMuted).SingleLine()
					}
				})
			}
			if state.connectErr != "" {
				errorState(c, p, state.connectErr)
				if state.pendingTarget.Kind != "" && !state.busy {
					ui.Button(c, "Retry").OnClick(func() { m.retryRemoteConnection() })
				}
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				label := "Open"
				if state.busy {
					label = "Connecting…"
				}
				ui.Button(c, label).Disabled(!m.remoteConnectReady()).
					OnClick(m.connectRemoteFromDialog)
				ui.Button(c, "Cancel").OnClick(func() {
					if state.busy {
						m.cancelRemoteConnection()
						return
					}
					state.connectOpen = false
				})
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.connectOpen = false
	}
}

func (m *Model) remoteDisconnectDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	if !state.disconnectOpen {
		return
	}
	open := state.disconnectOpen
	dialog := nativeModal(c.Key("remote-disconnect-dialog"), &open, func() {
		ui.Column(c).Width(600).Padding(18).Gap(10).Children(func() {
			ui.Text(c, "Disconnect from Remote Host?").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			if state.session == nil {
				ui.Text(c, "No remote session is connected.").FontSize(12).
					TextColor(p.TextMuted)
			} else {
				ui.Text(c, "Remote host: "+state.session.Host).FontSize(12).
					TextColor(p.TextMuted).SingleLine()
				ui.Text(c, "Project: "+state.session.Root).FontSize(12).
					TextColor(p.Text).SingleLine().Ellipsis("…")
				ui.Text(c, "Forwarded ports are stopped and the project returns to Welcome.").
					FontSize(12).TextColor(p.TextMuted)
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Disconnect").Disabled(state.busy || state.session == nil).
					OnClick(m.confirmRemoteDisconnect)
				ui.Button(c, "Cancel").OnClick(func() { state.disconnectOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.disconnectOpen = false
	}
}

func (m *Model) remoteNoticeDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	if !state.noticeOpen {
		return
	}
	open := state.noticeOpen
	dialog := nativeModal(c.Key("remote-notice-dialog"), &open, func() {
		ui.Column(c).Width(580).Padding(18).Gap(10).Children(func() {
			ui.Text(c, state.noticeTitle).FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, state.noticeBody).FontSize(12).TextColor(p.TextMuted)
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Close").OnClick(func() { state.noticeOpen = false })
			})
		})
	})
	if !open || dialog.Dismissed() {
		state.noticeOpen = false
	}
}
