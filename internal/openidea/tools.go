package openidea

import (
	"context"
	"encoding/json"
)

// ToolInfo is one language server, debug adapter or plugin the backend knows,
// with where it is and whether the backend can install it.
type ToolInfo struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"displayName"`
	Kind          string   `json:"kind"`
	Command       string   `json:"command"`
	Languages     []string `json:"languages"`
	Status        string   `json:"status"` // installed | onPath | custom | missing
	Path          *string  `json:"path"`
	Version       *string  `json:"version"`
	InstallMethod *string  `json:"installMethod"`
	CanInstall    bool     `json:"canInstall"`
	// UnavailableReason says why install is impossible, e.g. no npm.
	UnavailableReason *string `json:"unavailableReason"`
	CanUninstall      bool    `json:"canUninstall"`
	InstallHint       *string `json:"installHint"`
	Homepage          *string `json:"homepage"`
	CustomError       *string `json:"customError"`
}

// InstallEvent is one message of tools_install: a line of installer output or
// a download progress report.
type InstallEvent struct {
	Type     string   `json:"type"` // line | progress
	Text     string   `json:"text"`
	Fraction *float64 `json:"fraction"`
	Message  string   `json:"message"`
}

// ToolsList returns every known tool with its state in window's project. The
// list is empty until the window has a project, because tools are resolved
// against its settings.
func (m *Manager) ToolsList(ctx context.Context, window string) ([]ToolInfo, error) {
	raw, err := m.Invoke(ctx, window, "tools_list", nil)
	if err != nil {
		return nil, err
	}
	var list []ToolInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// ToolInstall installs a tool into the backend's own data directory, streaming
// progress on channel id events. request, which cancel_request cancels, must be
// nonzero; the call returns the tool's new state.
func (m *Manager) ToolInstall(ctx context.Context, window, name string, request, channel uint32) (ToolInfo, error) {
	raw, err := m.Invoke(ctx, window, "tools_install", map[string]any{
		"name":    name,
		"request": request,
		"onEvent": map[string]any{"$channel": channel},
	})
	if err != nil {
		return ToolInfo{}, err
	}
	var info ToolInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ToolInfo{}, err
	}
	return info, nil
}

// ToolUninstall removes a tool the backend installed.
func (m *Manager) ToolUninstall(ctx context.Context, window, name string) (ToolInfo, error) {
	raw, err := m.Invoke(ctx, window, "tools_uninstall", map[string]any{"name": name})
	if err != nil {
		return ToolInfo{}, err
	}
	var info ToolInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ToolInfo{}, err
	}
	return info, nil
}
