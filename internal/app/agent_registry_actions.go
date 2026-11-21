package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

type acpImportOutcome struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Saved  bool   `json:"saved"`
	Reason string `json:"reason,omitempty"`
}

var agentIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var agentEnvKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func agentProviderByID(providers []agentProvider, id string) *agentProvider {
	for index := range providers {
		if providers[index].ID == id {
			return &providers[index]
		}
	}
	return nil
}

func (m *Model) registryAgentByID(id string) *registryAgent {
	for index := range m.agents.registry.Agents {
		if m.agents.registry.Agents[index].ID == id {
			return &m.agents.registry.Agents[index]
		}
	}
	return nil
}

func agentProviderMatches(provider agentProvider, query string) bool {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return true
	}
	text := strings.ToLower(provider.Name + " " + provider.ID)
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

func agentProviderRunnable(provider agentProvider) bool {
	return provider.Installed || provider.Source.Kind != "builtin" || provider.CommandOverridden
}

func registryRunnable(agent registryAgent) bool {
	return agent.Installed != nil
}

func parseAgentArgs(text string) []string {
	out := make([]string, 0)
	var current strings.Builder
	var quote rune
	flush := func() {
		if current.Len() == 0 {
			return
		}
		out = append(out, current.String())
		current.Reset()
	}
	for _, ch := range strings.TrimSpace(text) {
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
			} else {
				current.WriteRune(ch)
			}
		case ch == '"' || ch == '\'':
			quote = ch
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			flush()
		default:
			current.WriteRune(ch)
		}
	}
	flush()
	return out
}

func parseAgentEnv(text string) (map[string]string, error) {
	env := make(map[string]string)
	for _, raw := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ';' || r == '\n'
	}) {
		pair := strings.TrimSpace(raw)
		if pair == "" {
			continue
		}
		equal := strings.IndexByte(pair, '=')
		key := ""
		if equal >= 0 {
			key = strings.TrimSpace(pair[:equal])
		}
		if !agentEnvKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("Expected NAME=value: %s", pair)
		}
		env[key] = strings.TrimSpace(pair[equal+1:])
	}
	return env, nil
}

func formatAgentEnv(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+env[key])
	}
	return strings.Join(pairs, "; ")
}

func agentSlugID(name string) string {
	var out strings.Builder
	dash := false
	for _, ch := range strings.ToLower(strings.TrimSpace(name)) {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			if dash && out.Len() > 0 {
				out.WriteByte('-')
			}
			dash = false
			out.WriteRune(ch)
		} else {
			dash = true
		}
	}
	return out.String()
}

func (m *Model) prepareAgentProviderDrafts(provider agentProvider) (string, string) {
	st := &m.agents.registry
	if st.CommandDrafts == nil {
		st.CommandDrafts = make(map[string]string)
	}
	if st.EnvDrafts == nil {
		st.EnvDrafts = make(map[string]string)
	}
	command, ok := st.CommandDrafts[provider.ID]
	if !ok {
		if provider.CommandOverridden {
			command = provider.Program
		}
		st.CommandDrafts[provider.ID] = command
	}
	env, ok := st.EnvDrafts[provider.ID]
	if !ok {
		env = formatAgentEnv(provider.Env)
		st.EnvDrafts[provider.ID] = env
	}
	return command, env
}

func (m *Model) setAgentProviderError(id, message string) {
	st := &m.agents.registry
	if st.ProviderErr == nil {
		st.ProviderErr = make(map[string]string)
	}
	if message == "" {
		delete(st.ProviderErr, id)
	} else {
		st.ProviderErr[id] = message
	}
}

func (m *Model) applyAgentProviderSettings(providerID string) {
	provider := agentProviderByID(m.agents.providers, providerID)
	if provider == nil || !m.agentSettingsBackendAvailable() {
		return
	}
	st := &m.agents.registry
	if st.ProviderBusyID != "" {
		return
	}
	envText := st.EnvDrafts[providerID]
	env, err := parseAgentEnv(envText)
	if err != nil {
		m.setAgentProviderError(providerID, err.Error())
		m.invalidate()
		return
	}
	command := strings.TrimSpace(st.CommandDrafts[providerID])
	baseCommand := ""
	if provider.CommandOverridden {
		baseCommand = provider.Program
	}
	sendCommand := provider.CommandEditable && command != baseCommand
	sendEnv := formatAgentEnv(env) != formatAgentEnv(provider.Env)
	st.ProviderBusyID = providerID
	st.Err = ""
	m.setAgentProviderError(providerID, "")
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var firstErr error
		if sendCommand {
			var value any
			if command != "" {
				value = command
			}
			if _, err := m.invoke(ctx, "agent_set_command", map[string]any{
				"id": providerID, "command": value,
			}); err != nil {
				firstErr = err
			}
		}
		if firstErr == nil && sendEnv {
			_, firstErr = m.invoke(ctx, "agent_set_env", map[string]any{
				"id": providerID, "env": env,
			})
		}
		m.post(func() {
			st.ProviderBusyID = ""
			if firstErr != nil {
				m.setAgentProviderError(providerID, "Could not save launch settings: "+firstErr.Error())
				m.invalidate()
				return
			}
			delete(st.CommandDrafts, providerID)
			delete(st.EnvDrafts, providerID)
			m.refreshAgentProviders(true)
			m.LoadAgentRegistry(true)
			m.invalidate()
		})
	}()
}

func (m *Model) chooseAgentExecutable(providerID string) {
	provider := agentProviderByID(m.agents.providers, providerID)
	st := &m.agents.registry
	if provider == nil || st.ProviderBusyID != "" {
		return
	}
	start := strings.TrimSpace(st.CommandDrafts[providerID])
	if start == "" && provider.DefaultProgram != nil {
		start = *provider.DefaultProgram
	}
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Select " + provider.Name, DefaultPath: start,
		})
		m.post(func() {
			if err != nil {
				m.setAgentProviderError(providerID, err.Error())
				m.invalidate()
				return
			}
			if len(paths) == 0 {
				return
			}
			if st.CommandDrafts == nil {
				st.CommandDrafts = make(map[string]string)
			}
			st.CommandDrafts[providerID] = paths[0]
			m.setAgentProviderError(providerID, "")
			m.invalidate()
		})
	}()
}

func (m *Model) chooseCustomAgentExecutable() {
	st := &m.agents.registry
	if st.CustomBusy {
		return
	}
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Select executable",
			DefaultPath: strings.TrimSpace(st.CustomCommand),
		})
		m.post(func() {
			if err != nil {
				st.CustomErr = err.Error()
				m.invalidate()
				return
			}
			if len(paths) == 0 {
				return
			}
			st.CustomCommand = paths[0]
			st.CustomErr = ""
			m.invalidate()
		})
	}()
}

func (m *Model) openCustomAgentDialog() {
	st := &m.agents.registry
	st.CustomOpen = true
	st.CustomName = ""
	st.CustomID = ""
	st.CustomIDTouched = false
	st.CustomCommand = ""
	st.CustomArgs = ""
	st.CustomEnv = ""
	st.CustomBusy = false
	st.CustomErr = ""
	m.invalidate()
}

func (m *Model) updateCustomAgentName(name string) {
	st := &m.agents.registry
	st.CustomName = name
	if !st.CustomIDTouched {
		st.CustomID = agentSlugID(name)
	}
}

func (m *Model) submitCustomAgent() {
	st := &m.agents.registry
	name := strings.TrimSpace(st.CustomName)
	id := strings.TrimSpace(st.CustomID)
	command := strings.TrimSpace(st.CustomCommand)
	if st.CustomBusy || name == "" || id == "" || command == "" ||
		!agentIDPattern.MatchString(id) || !m.agentSettingsBackendAvailable() {
		return
	}
	env, err := parseAgentEnv(st.CustomEnv)
	if err != nil {
		st.CustomErr = err.Error()
		m.invalidate()
		return
	}
	input := map[string]any{
		"id": id, "name": name, "command": command,
		"args": parseAgentArgs(st.CustomArgs), "env": env,
	}
	st.CustomBusy = true
	st.CustomErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_custom_save", map[string]any{"input": input})
		m.post(func() {
			st.CustomBusy = false
			if err != nil {
				st.CustomErr = err.Error()
				m.invalidate()
				return
			}
			st.CustomOpen = false
			st.Notice = "Added custom agent " + name
			m.refreshAgentProviders(true)
			m.LoadAgentRegistry(true)
			m.invalidate()
		})
	}()
}

func (m *Model) chooseAndImportACPJSON() {
	st := &m.agents.registry
	if st.ImportBusy || !m.agentSettingsBackendAvailable() {
		return
	}
	st.ImportBusy = true
	st.Notice = ""
	m.invalidate()
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Select acp.json", Multiple: false,
			Filters: []mygo.FileFilter{{Name: "ACP config", Extensions: []string{"json"}}},
		})
		if err != nil || len(paths) == 0 {
			m.post(func() {
				st.ImportBusy = false
				if err != nil {
					st.Notice = err.Error()
				}
				m.invalidate()
			})
			return
		}
		data, err := os.ReadFile(filepath.Clean(paths[0]))
		if err != nil {
			m.post(func() {
				st.ImportBusy = false
				st.Notice = err.Error()
				m.invalidate()
			})
			return
		}
		m.importAgentACPText(string(data))
	}()
}

func formatACPImportNotice(outcomes []acpImportOutcome) string {
	saved := make([]string, 0)
	skipped := make([]string, 0)
	for _, outcome := range outcomes {
		if outcome.Saved {
			saved = append(saved, outcome.Name)
		} else {
			skipped = append(skipped, outcome.Name+" ("+outcome.Reason+")")
		}
	}
	parts := []string{"Imported nothing from acp.json."}
	if len(saved) > 0 {
		parts[0] = "Imported " + strings.Join(saved, ", ") + " from acp.json."
	}
	if len(skipped) > 0 {
		parts = append(parts, "Skipped "+strings.Join(skipped, ", ")+".")
	}
	return strings.Join(parts, " ")
}

func (m *Model) importAgentACPText(text string) {
	st := &m.agents.registry
	if !m.agentSettingsBackendAvailable() {
		st.ImportBusy = false
		return
	}
	st.ImportBusy = true
	st.Notice = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_custom_import", map[string]any{"text": text})
		var outcomes []acpImportOutcome
		if err == nil {
			err = json.Unmarshal(raw, &outcomes)
		}
		m.post(func() {
			st.ImportBusy = false
			if err != nil {
				st.Notice = err.Error()
				m.invalidate()
				return
			}
			st.Notice = formatACPImportNotice(outcomes)
			m.refreshAgentProviders(true)
			m.LoadAgentRegistry(true)
			m.invalidate()
		})
	}()
}

func (m *Model) requestRegistryInstall(agent registryAgent) {
	if agent.NoChecksum {
		m.agents.registry.InstallConfirmID = agent.ID
		m.invalidate()
		return
	}
	m.installRegistryAgent(agent)
}

func (m *Model) confirmRegistryInstall() {
	st := &m.agents.registry
	id := st.InstallConfirmID
	st.InstallConfirmID = ""
	if agent := m.registryAgentByID(id); agent != nil {
		m.installRegistryAgent(*agent)
	}
}

func (m *Model) requestRemoveAgent(id string, custom bool) {
	st := &m.agents.registry
	st.RemoveOpen = true
	st.RemoveID = id
	st.RemoveCustom = custom
	m.invalidate()
}

func (m *Model) confirmRemoveAgent() {
	st := &m.agents.registry
	id, custom := st.RemoveID, st.RemoveCustom
	st.RemoveOpen = false
	st.RemoveID = ""
	st.RemoveCustom = false
	if custom {
		m.removeCustomAgent(id)
		return
	}
	if agent := m.registryAgentByID(id); agent != nil {
		m.removeRegistryAgent(*agent)
	}
}

func (m *Model) removeCustomAgent(id string) {
	st := &m.agents.registry
	if id == "" || st.BusyID != "" || !m.agentSettingsBackendAvailable() {
		return
	}
	st.BusyID = id
	st.Err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_custom_remove", map[string]any{"id": id})
		m.post(func() {
			st.BusyID = ""
			if err != nil {
				st.Err = "Could not remove the custom agent: " + err.Error()
			} else {
				st.Notice = "Removed custom agent " + id
				m.refreshAgentProviders(true)
				m.LoadAgentRegistry(true)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) ensureAgentRegistryPoll() {
	st := &m.agents.registry
	if !m.settingsOpen || m.settingsPage != settingsPageAgentRegistry ||
		st.PollTimer != nil || !st.Loaded {
		return
	}
	background := false
	for _, agent := range st.Agents {
		if agent.Installing {
			background = true
			break
		}
	}
	if !background {
		return
	}
	st.PollTimer = time.AfterFunc(1500*time.Millisecond, func() {
		m.post(func() {
			st.PollTimer = nil
			if !m.settingsOpen || m.settingsPage != settingsPageAgentRegistry {
				return
			}
			m.LoadAgentRegistry(true)
		})
	})
}
