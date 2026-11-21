package app

import (
	"context"
	"encoding/json"
	"slices"
	"time"
)

// backendAgentSettings is the agents object OpenIDEA returns from
// settings_get_global and accepts through settings_set.
type backendAgentSettings struct {
	ShowContextUsage *bool `json:"showContextUsage,omitempty"`
	ShowWorkTime     *bool `json:"showWorkTime,omitempty"`
	ShowUsageLimits  *bool `json:"showUsageLimits,omitempty"`
	ShowCost         *bool `json:"showCost,omitempty"`
	FoldTurns        *bool `json:"foldTurns,omitempty"`
	SmoothText       *bool `json:"smoothText,omitempty"`

	Thinking         string `json:"thinking,omitempty"`
	ToolCalls        string `json:"toolCalls,omitempty"`
	ExpandToolOutput bool   `json:"expandToolOutput"`
	SendWith         string `json:"sendWith,omitempty"`
	WhileRunning     string `json:"whileRunning,omitempty"`
	NewTaskApprovals string `json:"newTaskApprovals,omitempty"`

	Defaults   map[string]map[string]string `json:"defaults,omitempty"`
	MCPServers map[string]json.RawMessage   `json:"mcpServers,omitempty"`
}

type backendAgentSettingsEnvelope struct {
	Agents        *backendAgentSettings `json:"agents"`
	AgentSecurity *AgentSecurityPrefs   `json:"agentSecurity"`
}

func sameAgentPrefs(left, right AgentPrefs) bool {
	left.BackendDefaults = normalizeAgentSettingsJSON(left.BackendDefaults)
	right.BackendDefaults = normalizeAgentSettingsJSON(right.BackendDefaults)
	left.BackendMCPServers = normalizeAgentSettingsJSON(left.BackendMCPServers)
	right.BackendMCPServers = normalizeAgentSettingsJSON(right.BackendMCPServers)
	return left == right
}

func sameAgentSecurity(left, right AgentSecurityPrefs) bool {
	left = left.normalized()
	right = right.normalized()
	return slices.Equal(left.AllowedCommands, right.AllowedCommands) &&
		slices.Equal(left.BlockedCommands, right.BlockedCommands)
}

func normalizeAgentSettingsJSON(value string) string {
	if value == "" {
		return ""
	}
	var compact any
	if json.Unmarshal([]byte(value), &compact) != nil {
		return value
	}
	raw, err := json.Marshal(compact)
	if err != nil {
		return value
	}
	return string(raw)
}

func (m *Model) agentSettingsBackendAvailable() bool {
	return m.ctx != nil && (m.mgr != nil || m.invokeHook != nil)
}

// loadAgentSettings reads OpenIDEA's global settings into the client model.
// OpenIDEA remains the authority for the Agents settings and Command
// Permissions pages.
func (m *Model) loadAgentSettings() {
	if !m.agentSettingsBackendAvailable() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "settings_get_global", nil)
		if err != nil {
			return
		}
		var envelope backendAgentSettingsEnvelope
		if json.Unmarshal(raw, &envelope) != nil {
			return
		}
		m.post(func() {
			if envelope.Agents != nil {
				agents := agentPrefsFromBackend(*envelope.Agents)
				m.prefs.Agents = &agents
				if m.settingsOpen && !m.settingsDirtyPage(settingsPageAgents) {
					m.settingsDraft.agents = agents
				}
			}
			if envelope.AgentSecurity != nil {
				security := envelope.AgentSecurity.normalized()
				m.prefs.AgentSecurity = &security
				if m.settingsOpen && !m.settingsDirtyPage(settingsPageAgentPolicy) {
					m.settingsDraft.security = security
				}
			}
			m.invalidate()
		})
	}()
}

func agentPrefsFromBackend(settings backendAgentSettings) AgentPrefs {
	prefs := AgentPrefs{
		NewTaskApprovals:  settings.NewTaskApprovals,
		SendWith:          settings.SendWith,
		WhileRunning:      settings.WhileRunning,
		ToolCalls:         settings.ToolCalls,
		Thinking:          settings.Thinking,
		ExpandToolOutput:  settings.ExpandToolOutput,
		BackendDefaults:   marshalAgentSettingsMap(settings.Defaults),
		BackendMCPServers: marshalAgentSettingsMap(settings.MCPServers),
	}
	if settings.FoldTurns != nil {
		prefs.NoFoldTurns = !*settings.FoldTurns
	}
	if settings.SmoothText != nil {
		prefs.NoSmoothText = !*settings.SmoothText
	}
	if settings.ShowContextUsage != nil {
		prefs.NoContextUsage = !*settings.ShowContextUsage
	}
	if settings.ShowWorkTime != nil {
		prefs.NoWorkTime = !*settings.ShowWorkTime
	}
	if settings.ShowUsageLimits != nil {
		prefs.NoUsageLimits = !*settings.ShowUsageLimits
	}
	if settings.ShowCost != nil {
		prefs.ShowCost = *settings.ShowCost
	}
	return prefs.normalized()
}

func marshalAgentSettingsMap[T any](value T) string {
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

func backendAgentSettingsFromPrefs(prefs AgentPrefs) backendAgentSettings {
	prefs = prefs.normalized()
	showContext, showWork := !prefs.NoContextUsage, !prefs.NoWorkTime
	showLimits, fold, smooth := !prefs.NoUsageLimits, !prefs.NoFoldTurns, !prefs.NoSmoothText
	settings := backendAgentSettings{
		ShowContextUsage: &showContext,
		ShowWorkTime:     &showWork,
		ShowUsageLimits:  &showLimits,
		ShowCost:         &prefs.ShowCost,
		FoldTurns:        &fold,
		SmoothText:       &smooth,
		Thinking:         prefs.thinking(),
		ToolCalls:        prefs.toolCalls(),
		ExpandToolOutput: prefs.ExpandToolOutput,
		SendWith:         prefs.sendWith(),
		WhileRunning:     prefs.whileRunning(),
		NewTaskApprovals: prefs.newTaskApprovals(),
	}
	if prefs.BackendDefaults != "" {
		_ = json.Unmarshal([]byte(prefs.BackendDefaults), &settings.Defaults)
	}
	if prefs.BackendMCPServers != "" {
		_ = json.Unmarshal([]byte(prefs.BackendMCPServers), &settings.MCPServers)
	}
	return settings
}

// saveAgentSettingsBackend writes the two settings objects upstream owns. The
// local preferences mirror them so the native UI can draw immediately while a
// remote or restarted backend catches up.
func (m *Model) saveAgentSettingsBackend() {
	if !m.agentSettingsBackendAvailable() {
		return
	}
	agents := backendAgentSettingsFromPrefs(m.settingsDraft.agents)
	security := m.settingsDraft.security.normalized()
	settings := make([]struct {
		key   string
		value any
	}, 0, 12)
	for _, item := range []struct {
		key   string
		value any
	}{
		{"agents.showContextUsage", agents.ShowContextUsage},
		{"agents.showWorkTime", agents.ShowWorkTime},
		{"agents.showUsageLimits", agents.ShowUsageLimits},
		{"agents.showCost", agents.ShowCost},
		{"agents.foldTurns", agents.FoldTurns},
		{"agents.smoothText", agents.SmoothText},
		{"agents.thinking", agents.Thinking},
		{"agents.toolCalls", agents.ToolCalls},
		{"agents.expandToolOutput", agents.ExpandToolOutput},
		{"agents.sendWith", agents.SendWith},
		{"agents.whileRunning", agents.WhileRunning},
		{"agents.newTaskApprovals", agents.NewTaskApprovals},
		{"agentSecurity.allowedCommands", security.AllowedCommands},
		{"agentSecurity.blockedCommands", security.BlockedCommands},
	} {
		settings = append(settings, item)
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		for _, setting := range settings {
			raw, err := json.Marshal(setting.value)
			if err != nil {
				m.post(func() {
					m.settingsErr = "Could not save " + setting.key + ": " + err.Error()
					m.invalidate()
				})
				return
			}
			_, err = m.invoke(ctx, "settings_set", map[string]any{
				"scope": "global", "key": setting.key, "value": string(raw),
			})
			if err != nil {
				key := setting.key
				m.post(func() {
					m.settingsErr = "Could not save " + key + ": " + err.Error()
					m.invalidate()
				})
				return
			}
		}
	}()
}
