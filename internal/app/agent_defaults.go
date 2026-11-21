package app

import (
	"context"
	"encoding/json"
	"time"
)

func agentDefaultsMap(prefs AgentPrefs) map[string]map[string]string {
	defaults := make(map[string]map[string]string)
	if prefs.BackendDefaults != "" {
		_ = json.Unmarshal([]byte(prefs.BackendDefaults), &defaults)
	}
	return defaults
}

func (m *Model) agentDefaults() map[string]map[string]string {
	return agentDefaultsMap(m.agentPrefs())
}

func (m *Model) agentDefaultChoices(providerID string) map[string]string {
	choices := m.agentDefaults()[providerID]
	if choices == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(choices))
	for key, value := range choices {
		out[key] = value
	}
	return out
}

func agentConfigValueExists(option agentConfigOption, value string) bool {
	for _, optionValue := range option.Values {
		if optionValue.Value == value {
			return true
		}
	}
	return false
}

func agentDefaultConfigOptions(provider agentProvider,
	chosen map[string]string) []agentConfigOption {
	base := make([]agentConfigOption, 0, len(provider.Config))
	for _, option := range provider.Config {
		category := ""
		if option.Category != nil {
			category = *option.Category
		}
		if category != "thought_level" && len(option.Values) > 1 {
			base = append(base, option)
		}
	}
	model := chosen["model"]
	if model == "" {
		for _, option := range base {
			if option.Category != nil && *option.Category == "model" &&
				option.Current != nil {
				model = *option.Current
				break
			}
		}
	}
	for _, option := range provider.ThoughtLevels[model] {
		if len(option.Values) > 1 {
			base = append(base, option)
		}
	}
	return base
}

func (m *Model) setAgentDefaultChoice(providerID, optionID, value string) {
	all := m.agentDefaults()
	choices := all[providerID]
	if choices == nil {
		choices = make(map[string]string)
	}
	if value == "" {
		delete(choices, optionID)
	} else {
		choices[optionID] = value
	}
	if len(choices) == 0 {
		delete(all, providerID)
	} else {
		all[providerID] = choices
	}
	raw, err := json.Marshal(all)
	if err != nil {
		return
	}
	prefs := m.agentPrefs()
	if len(all) == 0 {
		prefs.BackendDefaults = ""
	} else {
		prefs.BackendDefaults = string(raw)
	}
	prefs = prefs.normalized()
	m.prefs.Agents = &prefs
	if m.settingsOpen && !m.settingsDirtyPage(settingsPageAgents) {
		m.settingsDraft.agents = prefs
	}
	m.savePreferences()
	delete(m.agents.newConfig, optionID)
	m.prepareNewAgentConfig()
	m.invalidate()
	if !m.agentSettingsBackendAvailable() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		encoded, _ := json.Marshal(all)
		_, err := m.invoke(ctx, "settings_set", map[string]any{
			"scope": "global", "key": "agents.defaults", "value": string(encoded),
		})
		if err != nil {
			m.post(func() {
				m.settingsErr = "Could not save agent defaults: " + err.Error()
				m.invalidate()
			})
		}
	}()
}
