package app

// Native Remote Development surfaces. The pinned backend exposes
// environment reloading but not the desktop shell's SSH, WSL, dev-container or
// port-forwarding orchestration. The UI keeps that boundary explicit instead
// of presenting controls that cannot work.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const remoteDevelopmentWindow = "remote"

const remoteBackendGap = "The backend bundled with this build does not expose SSH, WSL, dev-container, or port-forwarding RPCs."

type remoteDevelopmentState struct {
	connectOpen    bool
	connectTarget  string
	disconnectOpen bool
	noticeOpen     bool
	noticeTitle    string
	noticeBody     string

	envBusy         bool
	envLoaded       bool
	envErr          string
	envChanged      []string
	envDevcontainer *string
	envShellRead    bool
}

type remoteEnvReloadResult struct {
	Changed      []string `json:"changed"`
	Devcontainer *string  `json:"devcontainer"`
	ShellRead    bool     `json:"shellRead"`
}

func init() {
	for _, action := range []*Action{
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
			Run:         func(m *Model) { m.ShowRemoteReopenInContainerNotice() },
		},
		{
			ID:       "remote.showToolWindow",
			Title:    "Remote Development",
			Category: "View",
			Keywords: "remote host ports tool window ssh wsl devcontainer",
			Run:      func(m *Model) { m.DockToggle(remoteDevelopmentWindow) },
			Toggled:  func(m *Model) bool { return m.dockWindowShown(remoteDevelopmentWindow) },
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
}

func remoteProjectOpen(m *Model) bool {
	return m.Root() != "" && !m.showingWelcome
}

// OpenRemoteConnectDialog opens the native remote target chooser.
func (m *Model) OpenRemoteConnectDialog() {
	m.remote.connectOpen = true
	m.remote.connectTarget = ""
	m.invalidate()
}

func (m *Model) OpenRemoteDisconnectDialog() {
	m.remote.disconnectOpen = true
	m.invalidate()
}

// ShowRemoteReopenInContainerNotice explains the dev-container RPC gap without
// changing the current project.
func (m *Model) ShowRemoteReopenInContainerNotice() {
	m.showRemoteNotice(
		"Reopen in Container Unavailable",
		remoteBackendGap+" GoIDEA did not restart, close, or reopen the project.",
	)
}

func (m *Model) showRemoteNotice(title, body string) {
	m.remote.noticeOpen = true
	m.remote.noticeTitle = title
	m.remote.noticeBody = body
	m.invalidate()
}

func (m *Model) confirmRemoteDisconnect() {
	m.remote.disconnectOpen = false
	m.showRemoteNotice(
		"Remote Session Unchanged",
		"No remote connection was changed and the project remains open. The backend has no remote teardown RPC.",
	)
}

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
	m.status = "Reloading environment…"
	m.DockShow(remoteDevelopmentWindow)
	m.invalidate()

	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	go func() {
		defer cancel()
		raw, err := m.invoke(ctx, "env_reload", nil)
		m.post(func() {
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

func (m *Model) remoteDevelopmentPanel(c *ui.Context, p *ui.Theme) {
	state := &m.remote
	ui.Column(c).Key("remote:window").Grow(1).Padding(10, 12, 10, 12).Gap(10).
		Children(func() {
			ui.Row(c).Height(26).Shrink(0).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Remote Development").FontSize(13).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				ui.Text(c, "Not connected").FontSize(12).TextColor(p.TextMuted).SingleLine()
			})
			ui.Row(c).Grow(1).Gap(12).Children(func() {
				m.remoteHostPanel(c, p, state)
				m.remotePortsPanel(c, p)
			})
		})
}

func (m *Model) remoteHostPanel(c *ui.Context, p *ui.Theme, state *remoteDevelopmentState) {
	ui.Column(c).Grow(1).MinWidth(0).Padding(10).Gap(7).
		BorderWidth(1).BorderColor(p.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Remote Host").FontSize(12).Bold().TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			ui.Text(c, "Not connected").FontSize(11).TextColor(p.TextMuted).SingleLine()
		})
		ui.Text(c, "GoIDEA is using its local backend.").FontSize(11).
			TextColor(p.TextMuted)
		ui.Text(c, remoteBackendGap).FontSize(11).TextColor(p.TextMuted)
		ui.Row(c).Gap(6).Children(func() {
			ui.Button(c, "Connect…").OnClick(m.OpenRemoteConnectDialog)
			ui.Button(c, "Disconnect").Disabled(!remoteProjectOpen(m)).
				OnClick(m.OpenRemoteDisconnectDialog)
			ui.Button(c, "Reload Environment").
				Disabled(!remoteProjectOpen(m) || state.envBusy).
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
		}
	})
}

func (m *Model) remotePortsPanel(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Grow(1).MinWidth(0).Padding(10).Gap(7).
		BorderWidth(1).BorderColor(p.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Ports").FontSize(12).Bold().TextColor(p.Text).SingleLine()
			ui.Spacer(c)
			ui.Text(c, "Unavailable").FontSize(11).TextColor(p.TextMuted).SingleLine()
		})
		emptyState(c, p, "No ports are forwarded.")
		ui.Text(c, "Port forwarding needs host RPCs that the bundled backend does not expose.").
			FontSize(11).TextColor(p.TextMuted)
	})
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
	dialog := ui.Modal(c.Key("remote-connect-dialog"), &open, func() {
		ui.Column(c).Width(640).Padding(18).Gap(10).Children(func() {
			ui.Text(c, "Remote Development").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "Choose how to run the project on another machine.").FontSize(12).
				TextColor(p.TextMuted)
			ui.Text(c, remoteBackendGap).FontSize(12).TextColor(p.Danger)
			ui.Text(c, "Connection type").FontSize(11).Bold().TextColor(p.Text).SingleLine()
			ui.Row(c).Gap(6).Children(func() {
				for _, target := range []string{"SSH", "WSL", "Dev Container"} {
					target := target
					ui.Button(c, target).OnClick(func() {
						state.connectTarget = target
						m.invalidate()
					})
				}
			})
			if state.connectTarget == "" {
				ui.Text(c, "Select SSH, WSL, or Dev Container.").FontSize(12).TextColor(p.TextMuted)
			} else {
				ui.Text(c, state.connectTarget+" connection is unavailable because the backend has no remote orchestration RPC.").
					FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Connect").Disabled(true)
				ui.Button(c, "Close").OnClick(func() { state.connectOpen = false })
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
	dialog := ui.Modal(c.Key("remote-disconnect-dialog"), &open, func() {
		ui.Column(c).Width(600).Padding(18).Gap(10).Children(func() {
			ui.Text(c, "Disconnect from Remote Host?").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "Remote host: Not connected").FontSize(12).TextColor(p.TextMuted).SingleLine()
			ui.Text(c, "No remote session is managed by the bundled backend.").FontSize(12).
				TextColor(p.Text)
			ui.Text(c, "This server has no remote teardown RPC. Confirming will leave this project open.").
				FontSize(12).TextColor(p.TextMuted)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Disconnect").OnClick(m.confirmRemoteDisconnect)
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
	dialog := ui.Modal(c.Key("remote-notice-dialog"), &open, func() {
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
