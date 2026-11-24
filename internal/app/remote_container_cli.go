package app

import (
	"context"
	"strings"
	"time"
)

type remoteDevContainerCLI struct {
	Status  string
	Path    string
	Version string
	Message string
}

func (m *Model) loadDevContainerCLI(ctx context.Context) remoteDevContainerCLI {
	if m == nil || (m.mgr == nil && m.invokeHook == nil) {
		return remoteDevContainerCLI{Status: "unknown", Message: "The GoIDEA backend is not connected."}
	}
	list, err := m.languageToolList(ctx)
	if err != nil {
		return remoteDevContainerCLI{
			Status:  "unknown",
			Message: "Could not read the Dev Containers CLI status: " + err.Error(),
		}
	}
	for _, tool := range list {
		if tool.Name != "devcontainer" {
			continue
		}
		out := remoteDevContainerCLI{Status: tool.Status}
		if tool.Path != nil {
			out.Path = strings.TrimSpace(*tool.Path)
		}
		if tool.Version != nil {
			out.Version = strings.TrimSpace(*tool.Version)
		}
		switch tool.Status {
		case "installed":
			out.Message = "Managed by GoIDEA"
		case "onPath":
			out.Message = "Found on PATH"
		case "custom":
			out.Message = "Custom executable"
		default:
			out.Message = "Not installed"
			if tool.CanInstall {
				out.Message += " · GoIDEA can install it"
			} else if tool.UnavailableReason != nil && strings.TrimSpace(*tool.UnavailableReason) != "" {
				out.Message += " · " + strings.TrimSpace(*tool.UnavailableReason)
			}
		}
		return out
	}
	return remoteDevContainerCLI{Status: "missing", Message: "The backend does not list the Dev Containers CLI."}
}

func (m *Model) resolveManagedDevContainerCLI(ctx context.Context) string {
	status := m.loadDevContainerCLI(ctx)
	m.post(func() {
		m.remote.devContainerCLIStatus = status.Status
		m.remote.devContainerCLIPath = status.Path
		m.remote.devContainerCLIVersion = status.Version
		m.remote.devContainerCLIMessage = status.Message
		m.remote.devContainerCLIChecking = false
	})
	if status.Status == "installed" || status.Status == "custom" || status.Status == "onPath" {
		return status.Path
	}
	return ""
}

// CheckDevContainerCLI refreshes the managed CLI shown in Remote Development.
func (m *Model) CheckDevContainerCLI() {
	state := &m.remote
	if state.devContainerCLIChecking {
		return
	}
	state.devContainerCLIChecking = true
	state.devContainerCLIMessage = "Checking the Dev Containers CLI…"
	m.DockShow(remoteDevelopmentWindow)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 20*time.Second)
		defer cancel()
		status := m.loadDevContainerCLI(ctx)
		m.post(func() {
			state.devContainerCLIStatus = status.Status
			state.devContainerCLIPath = status.Path
			state.devContainerCLIVersion = status.Version
			state.devContainerCLIMessage = status.Message
			state.devContainerCLIChecking = false
			m.invalidate()
		})
	}()
}

// InstallDevContainerCLI uses the backend's managed tool installer.
func (m *Model) InstallDevContainerCLI() {
	m.InstallLanguageTool("devcontainer", false)
}

func (m *Model) devContainerCLIAvailable() bool {
	state := m.remote
	return state.devContainerCLIPath != "" &&
		(state.devContainerCLIStatus == "installed" ||
			state.devContainerCLIStatus == "custom" ||
			state.devContainerCLIStatus == "onPath")
}

func (m *Model) devContainerCLISummary() string {
	state := &m.remote
	if state.devContainerCLIMessage != "" {
		message := state.devContainerCLIMessage
		if state.devContainerCLIVersion != "" {
			message += " " + state.devContainerCLIVersion
		}
		return message
	}
	return "Not checked"
}
