package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type agentRegistry struct {
	Agents  []registryAgent `json:"agents"`
	Warning *string         `json:"warning"`
}

type registryAgent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Repository   *string  `json:"repository"`
	Website      *string  `json:"website"`
	Authors      []string `json:"authors"`
	License      *string  `json:"license"`
	Distribution *string  `json:"distribution"`
	NoChecksum   bool     `json:"noChecksum"`
	Installed    *string  `json:"installed"`
	Update       bool     `json:"update"`
	Installing   bool     `json:"installing"`
}

type agentRegistryState struct {
	Loaded           bool
	Loading          bool
	Request          uint32
	Agents           []registryAgent
	Warning          string
	Err              string
	Query            string
	Selected         string
	BusyID           string
	InstallRequest   uint32
	InstallChannel   uint32
	InstallMessage   string
	InstallDetail    string
	InstallConfirmID string
	CommandDrafts    map[string]string
	EnvDrafts        map[string]string
	ProviderBusyID   string
	ProviderErr      map[string]string
	ImportBusy       bool
	Notice           string
	CustomOpen       bool
	CustomName       string
	CustomID         string
	CustomIDTouched  bool
	CustomCommand    string
	CustomArgs       string
	CustomEnv        string
	CustomBusy       bool
	CustomErr        string
	RemoveOpen       bool
	RemoveID         string
	RemoveCustom     bool
	PollTimer        *time.Timer
}

type agentRegistryRow struct {
	Provider *agentProvider
	Registry *registryAgent
}

var featuredRegistryAgents = []string{
	"copilot", "gemini", "cursor", "opencode", "amp-acp", "devin",
	"factory-droid", "junie", "cline", "kilo", "antigravity-acp",
	"grok-build", "qoder", "pi-acp", "poolside", "deepagents",
}

func registryAgentMatches(agent registryAgent, query string) bool {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		agent.Name, agent.ID, agent.Description, strings.Join(agent.Authors, " "),
	}, " "))
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

func sortRegistryAgents(agents []registryAgent) {
	featured := make(map[string]int, len(featuredRegistryAgents))
	for index, id := range featuredRegistryAgents {
		featured[id] = index
	}
	sort.SliceStable(agents, func(i, j int) bool {
		leftInstalled, rightInstalled := agents[i].Installed != nil, agents[j].Installed != nil
		if leftInstalled != rightInstalled {
			return leftInstalled
		}
		leftRank, leftFound := featured[agents[i].ID]
		rightRank, rightFound := featured[agents[j].ID]
		if !leftFound {
			leftRank = len(featuredRegistryAgents)
		}
		if !rightFound {
			rightRank = len(featuredRegistryAgents)
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return strings.ToLower(agents[i].Name) < strings.ToLower(agents[j].Name)
	})
}

func agentRegistryRows(providers []agentProvider, agents []registryAgent,
	query string) ([]agentRegistryRow, []agentRegistryRow) {
	known := make(map[string]bool, len(providers))
	installed := make([]agentRegistryRow, 0, len(providers)+len(agents))
	available := make([]agentRegistryRow, 0)
	for providerIndex := range providers {
		provider := &providers[providerIndex]
		known[provider.ID] = true
		var registry *registryAgent
		for registryIndex := range agents {
			if agents[registryIndex].ID == provider.ID {
				registry = &agents[registryIndex]
				break
			}
		}
		matches := agentProviderMatches(*provider, query)
		if registry != nil && query != "" {
			matches = registryAgentMatches(*registry, query)
		}
		if !matches {
			continue
		}
		row := agentRegistryRow{Provider: provider, Registry: registry}
		if agentProviderRunnable(*provider) || registry != nil && registryRunnable(*registry) {
			installed = append(installed, row)
		} else {
			available = append(available, row)
		}
	}
	for registryIndex := range agents {
		agent := &agents[registryIndex]
		if known[agent.ID] || !registryAgentMatches(*agent, query) {
			continue
		}
		row := agentRegistryRow{Registry: agent}
		if registryRunnable(*agent) {
			installed = append(installed, row)
		} else {
			available = append(available, row)
		}
	}
	return installed, available
}

func (m *Model) LoadAgentRegistry(force bool) {
	st := &m.agents.registry
	if st.Loading || (!force && st.Loaded) || !m.agentSettingsBackendAvailable() {
		return
	}
	st.Request = m.nextRequest()
	st.Loading = true
	st.Err = ""
	request := st.Request
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_registry_list", map[string]any{"request": request})
		var registry agentRegistry
		if err == nil {
			err = json.Unmarshal(raw, &registry)
		}
		m.post(func() {
			if st.Request != request {
				return
			}
			st.Loading = false
			if err != nil {
				st.Err = err.Error()
			} else {
				sortRegistryAgents(registry.Agents)
				st.Agents = registry.Agents
				st.Warning = ""
				if registry.Warning != nil {
					st.Warning = *registry.Warning
				}
				st.Loaded = true
				if st.Selected == "" && len(st.Agents) > 0 {
					st.Selected = st.Agents[0].ID
				}
			}
			m.ensureAgentRegistryPoll()
			m.invalidate()
		})
	}()
}

func (m *Model) installRegistryAgent(agent registryAgent) {
	st := &m.agents.registry
	if st.BusyID != "" || agent.Distribution == nil ||
		agent.Installed != nil && !agent.Update || !m.agentSettingsBackendAvailable() {
		return
	}
	st.BusyID = agent.ID
	st.InstallRequest = m.nextRequest()
	st.InstallChannel = m.nextChannel()
	st.InstallMessage = "Preparing " + agent.Name + "…"
	st.InstallDetail = ""
	st.Err = ""
	request, channel := st.InstallRequest, st.InstallChannel
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "agent_registry_install", map[string]any{
			"id": agent.ID, "request": request,
			"onEvent": map[string]any{"$channel": channel},
		})
		m.post(func() {
			if st.InstallRequest != request {
				return
			}
			st.BusyID = ""
			st.InstallChannel = 0
			if err != nil {
				st.Err = "Could not install " + agent.Name + ": " + err.Error()
			} else {
				st.InstallMessage = agent.Name + " is installed"
				m.refreshAgentProviders(true)
			}
			m.LoadAgentRegistry(true)
			m.invalidate()
		})
	}()
}

func (m *Model) removeRegistryAgent(agent registryAgent) {
	st := &m.agents.registry
	if st.BusyID != "" || agent.Installed == nil || !m.agentSettingsBackendAvailable() {
		return
	}
	st.BusyID = agent.ID
	st.Err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "agent_registry_remove", map[string]any{"id": agent.ID})
		m.post(func() {
			st.BusyID = ""
			if err != nil {
				st.Err = "Could not remove " + agent.Name + ": " + err.Error()
			} else {
				m.refreshAgentProviders(true)
			}
			m.LoadAgentRegistry(true)
			m.invalidate()
		})
	}()
}

func (m *Model) cancelRegistryInstall() {
	st := &m.agents.registry
	if st.InstallRequest == 0 || !m.agentSettingsBackendAvailable() {
		return
	}
	request := st.InstallRequest
	st.InstallMessage = "Cancelling…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

func (m *Model) acceptRegistryInstallChunk(channel uint32, data json.RawMessage) {
	st := &m.agents.registry
	if channel == 0 || channel != st.InstallChannel {
		return
	}
	var event struct {
		Type     string   `json:"type"`
		Text     string   `json:"text"`
		Fraction *float64 `json:"fraction"`
		Message  string   `json:"message"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	switch event.Type {
	case "line":
		if event.Text != "" {
			st.InstallDetail = event.Text
		}
	case "progress":
		if event.Fraction != nil {
			st.InstallMessage = fmt.Sprintf("%s %.0f%%", event.Message, *event.Fraction*100)
		} else if event.Message != "" {
			st.InstallMessage = event.Message
		}
	}
	m.invalidate()
}

func registryAgentStatus(agent registryAgent) string {
	if agent.Installing {
		return "Installing"
	}
	if agent.Installed != nil {
		if agent.Update {
			return "Update " + agent.Version
		}
		return "Installed " + *agent.Installed
	}
	if agent.Distribution == nil {
		return "Unavailable"
	}
	return "Not installed"
}

func (m *Model) settingsAgentRegistryBody(c *ui.Context, oi oiColors) {
	st := &m.agents.registry
	m.LoadAgentRegistry(false)
	m.refreshAgentProviders(false)
	m.ensureAgentRegistryPoll()

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, "Coding Agents").FontSize(16).Bold().TextColor(oi.textStrong).
				SingleLine()
			ui.Text(c, "Discover, install and configure the coding agents this project can use.").
				FontSize(11).TextColor(oi.textMuted).SingleLine()
		})
		ui.Button(c, "Refresh").Disabled(st.Loading).OnClick(func() {
			m.LoadAgentRegistry(true)
			m.refreshAgentProviders(true)
		})
	})
	ui.Row(c).Height(32).AlignItems(ui.Center).Children(func() {
		in := ui.TextInput(c.Key("agent-registry-search"), &st.Query).Grow(1).
			Placeholder("Search agents")
		if in.Changed() {
			m.invalidate()
		}
	})
	if st.Warning != "" {
		ui.Text(c, "Showing the saved registry: "+st.Warning).FontSize(11).
			TextColor(oi.warning)
	}
	if m.agents.providersErr != "" {
		ui.Text(c, "Cannot load the installed agents: "+m.agents.providersErr).
			FontSize(11).TextColor(oi.danger)
	}
	if st.Err != "" {
		ui.Text(c, st.Err).FontSize(12).TextColor(oi.danger)
	}
	if st.BusyID != "" {
		ui.Column(c).Gap(1).Padding(3, 5).Radius(4).Background(oi.hoverTint).
			Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, st.InstallMessage).Grow(1).FontSize(11).
						TextColor(oi.text).SingleLine().Ellipsis("…")
					ui.Button(c, "Cancel").OnClick(m.cancelRegistryInstall)
				})
				if st.InstallDetail != "" {
					ui.Text(c, st.InstallDetail).FontSize(10).TextColor(oi.textMuted).
						SingleLine().Ellipsis("…").Tooltip(st.InstallDetail)
				}
			})
	}
	if (st.Loading && !st.Loaded) || (m.agents.providersLoading && len(m.agents.providers) == 0) {
		ui.Text(c, "Loading agents…").FontSize(12).TextColor(oi.textMuted).
			SingleLine()
		return
	}

	installed, available := agentRegistryRows(m.agents.providers, st.Agents, st.Query)
	all := append(append([]agentRegistryRow(nil), installed...), available...)
	selectedFound := false
	for _, row := range all {
		if agentRegistryRowID(row) == st.Selected {
			selectedFound = true
			break
		}
	}
	if !selectedFound {
		st.Selected = ""
		if len(all) > 0 {
			st.Selected = agentRegistryRowID(all[0])
		}
	}

	ui.Row(c).Grow(1).MinHeight(0).Gap(12).Children(func() {
		ui.Column(c).Width(320).Grow(0).MinHeight(0).Children(func() {
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					m.agentRegistrySection(c, oi, "Installed", installed)
					m.agentRegistrySection(c, oi, "Available", available)
				})
			})
			ui.Row(c).Gap(6).Padding(4, 4, 0, 4).Children(func() {
				ui.Button(c, "Add Custom Agent…").Grow(1).Disabled(st.CustomBusy).
					OnClick(m.openCustomAgentDialog)
				label := "Import acp.json"
				if st.ImportBusy {
					label = "Importing…"
				}
				ui.Button(c, label).Grow(1).Disabled(st.ImportBusy).
					OnClick(m.chooseAndImportACPJSON)
			})
			if st.Notice != "" {
				ui.Text(c, st.Notice).FontSize(10).TextColor(oi.textMuted).
					Padding(2, 6).Tooltip(st.Notice).SingleLine().Ellipsis("…")
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			m.agentRegistryDetails(c, oi)
		})
	})
}

func agentRegistryRowID(row agentRegistryRow) string {
	if row.Provider != nil {
		return row.Provider.ID
	}
	if row.Registry != nil {
		return row.Registry.ID
	}
	return ""
}

func agentRegistryRowName(row agentRegistryRow) string {
	if row.Provider != nil && row.Provider.Name != "" {
		return row.Provider.Name
	}
	if row.Registry != nil {
		return row.Registry.Name
	}
	return agentRegistryRowID(row)
}

func agentRegistryRowStatus(row agentRegistryRow) string {
	if row.Registry != nil && row.Registry.Installing {
		return "Installing"
	}
	if row.Provider != nil {
		provider := row.Provider
		if provider.Source.Kind == "registry" {
			if provider.Source.Update != nil && *provider.Source.Update != "" {
				return "ACP Registry · update to " + *provider.Source.Update
			}
			return "ACP Registry · " + provider.Source.Version
		}
		if provider.Source.Kind == "custom" {
			return "Agent file"
		}
		if provider.CommandOverridden {
			return "Custom executable"
		}
		if provider.Installed {
			return "Found on PATH"
		}
		return "Not found on PATH"
	}
	if row.Registry != nil {
		return registryAgentStatus(*row.Registry)
	}
	return ""
}

func (m *Model) agentRegistrySection(c *ui.Context, oi oiColors, title string,
	rows []agentRegistryRow) {
	st := &m.agents.registry
	ui.Row(c).Height(25).Padding(2, 6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%s  %d", title, len(rows))).FontSize(10).Bold().
			TextColor(oi.textMuted).SingleLine()
	})
	if len(rows) == 0 {
		ui.Text(c, "No matching agents").FontSize(11).TextColor(oi.textMuted).
			Padding(2, 8).SingleLine()
		return
	}
	for _, item := range rows {
		id := agentRegistryRowID(item)
		row := ui.Row(c.Key("agent-registry:"+id)).MinHeight(42).
			Padding(3, 6).Gap(7).Radius(4).AlignItems(ui.Center).
			Cursor(ui.CursorPointer)
		if row.Hovered() || id == st.Selected {
			row.Background(oi.hoverTint)
		}
		row.Children(func() {
			ui.Text(c, agentProviderGlyph(id)).Width(20).FontSize(12).
				TextColor(oi.accent)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, agentRegistryRowName(item)).FontSize(12).TextColor(oi.text).SingleLine().
					Ellipsis("…")
				ui.Text(c, agentRegistryRowStatus(item)).FontSize(10).
					TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			})
			if item.Registry != nil && item.Registry.Update ||
				item.Provider != nil && item.Provider.Source.Update != nil {
				ui.Text(c, "↑").FontSize(11).TextColor(oi.accent)
			}
		})
		if row.Clicked() {
			st.Selected = id
			m.invalidate()
		}
	}
}

func agentProviderGlyph(id string) string {
	if id == "" {
		return "◆"
	}
	return strings.ToUpper(string([]rune(id)[0]))
}

func (m *Model) agentRegistryDetails(c *ui.Context, oi oiColors) {
	st := &m.agents.registry
	provider := agentProviderByID(m.agents.providers, st.Selected)
	registry := m.registryAgentByID(st.Selected)
	if provider == nil && registry == nil {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Select an agent to see its details").FontSize(12).
				TextColor(oi.textMuted).Padding(4, 0).SingleLine()
		})
		return
	}
	id := st.Selected
	name := id
	if provider != nil && provider.Name != "" {
		name = provider.Name
	} else if registry != nil && registry.Name != "" {
		name = registry.Name
	}
	facts := make([]string, 0, 3)
	if provider != nil {
		facts = append(facts, agentRegistryRowStatus(agentRegistryRow{
			Provider: provider, Registry: registry,
		}))
	} else if registry != nil {
		if registry.Version != "" {
			facts = append(facts, registry.Version)
		}
		if registry.Distribution != nil {
			facts = append(facts, registryDistributionLabel(registry.Distribution))
		}
		if len(registry.Authors) > 0 {
			facts = append(facts, strings.Join(registry.Authors, ", "))
		}
	}
	providerRunnable := provider != nil && agentProviderRunnable(*provider)
	registryInstalled := registry != nil && registry.Installed != nil
	installed := providerRunnable || registryInstalled
	canInstall := registry != nil && registry.Distribution != nil
	updateVersion := ""
	if provider != nil && provider.Source.Kind == "registry" &&
		provider.Source.Update != nil {
		updateVersion = *provider.Source.Update
	} else if registryInstalled && registry.Update {
		updateVersion = registry.Version
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Gap(8).Padding(4, 6).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, agentProviderGlyph(id)).FontSize(16).TextColor(oi.accent)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, name).FontSize(15).Bold().TextColor(oi.textStrong).
						SingleLine().Ellipsis("…")
					ui.Text(c, strings.Join(facts, " · ")).FontSize(10).
						TextColor(oi.textMuted).SingleLine().Ellipsis("…")
				})
				if !installed && canInstall {
					ui.Button(c, "Install").Disabled(st.BusyID != "").OnClick(func() {
						m.requestRegistryInstall(*registry)
					})
				}
				if installed && updateVersion != "" && canInstall {
					ui.Button(c, "Update").Disabled(st.BusyID != "").OnClick(func() {
						m.requestRegistryInstall(*registry)
					})
				}
				registryManaged := provider != nil && provider.Source.Kind == "registry" ||
					registryInstalled
				if installed && registryManaged {
					ui.Button(c, "Remove").Disabled(st.BusyID != "").OnClick(func() {
						m.requestRemoveAgent(id, false)
					})
				}
				if provider != nil && provider.Source.Kind == "custom" {
					ui.Button(c, "Remove").Disabled(st.BusyID != "").OnClick(func() {
						m.requestRemoveAgent(id, true)
					})
				}
			})
			if updateVersion != "" {
				ui.Text(c, updateVersion+" is available").FontSize(11).
					TextColor(oi.accent)
			}
			if registry != nil && registry.Description != "" {
				ui.Text(c, registry.Description).FontSize(12).TextColor(oi.text)
			}
			if registry != nil && registry.License != nil && *registry.License != "" {
				ui.Text(c, "License: "+*registry.License).FontSize(11).
					TextColor(oi.textMuted)
			}
			if registry != nil && registry.Distribution != nil {
				ui.Text(c, "Distribution: "+registryDistributionLabel(registry.Distribution)).
					FontSize(11).TextColor(oi.textMuted).SingleLine()
			}
			if registry != nil && registry.NoChecksum {
				ui.Text(c, "No checksum provided: the download can't be verified.").
					FontSize(11).TextColor(oi.warning)
			}
			if registry != nil && (registry.Repository != nil || registry.Website != nil) {
				ui.Row(c).Wrap().Gap(6).Children(func() {
					if registry.Repository != nil && isHTTPURL(*registry.Repository) {
						ui.Button(c, "Repository").OnClick(func() {
							openReleasePage(*registry.Repository)
						})
					}
					if registry.Website != nil && isHTTPURL(*registry.Website) &&
						(registry.Repository == nil || *registry.Website != *registry.Repository) {
						ui.Button(c, "Website").OnClick(func() {
							openReleasePage(*registry.Website)
						})
					}
				})
			}
			if provider != nil {
				m.agentProviderDetails(c, oi, *provider)
			}
			if registry != nil {
				ui.Text(c, "Agents in the ACP Registry are third-party software. They run on this computer with your permissions and can read and change your files. Install only agents you trust.").
					FontSize(10).TextColor(oi.textMuted)
			}
		})
	})
}

func isHTTPURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}

func registryDistributionLabel(distribution *string) string {
	if distribution == nil {
		return "Unavailable"
	}
	switch *distribution {
	case "binary":
		return "Binary"
	case "npx":
		return "npm"
	case "uvx":
		return "uv"
	default:
		return *distribution
	}
}

func (m *Model) agentProviderDetails(c *ui.Context, oi oiColors, provider agentProvider) {
	st := &m.agents.registry
	command, envText := m.prepareAgentProviderDrafts(provider)
	launch := strings.TrimSpace(strings.Join(
		append([]string{provider.Program}, provider.Args...), " "))
	readOnlyReason := ""
	if provider.CommandReadOnlyReason != nil {
		readOnlyReason = *provider.CommandReadOnlyReason
	}
	envEditable := readOnlyReason != "registry" && readOnlyReason != "customAgent"
	busy := st.ProviderBusyID == provider.ID
	ui.Column(c).Gap(6).Padding(4, 0, 8, 0).
		BorderWidth(1, 0, 0, 0).BorderColor(oi.border).Children(func() {
		if provider.LoginHint != nil && *provider.LoginHint != "" {
			ui.Text(c, "Sign In").FontSize(12).Bold().TextColor(oi.textStrong)
			ui.Text(c, *provider.LoginHint).FontSize(11).TextColor(oi.text).Selectable()
		}
		ui.Text(c, "New Tasks").FontSize(12).Bold().TextColor(oi.textStrong)
		defaults := m.agentDefaultChoices(provider.ID)
		defaultOptions := agentDefaultConfigOptions(provider, defaults)
		if len(defaultOptions) == 0 {
			ui.Text(c, "The agent's models and modes can be chosen here after its first session.").
				FontSize(10).TextColor(oi.textMuted)
		}
		for _, option := range defaultOptions {
			option := option
			fallbackName := ""
			for _, value := range option.Values {
				if option.Current != nil && value.Value == *option.Current {
					fallbackName = value.Name
					if fallbackName == "" {
						fallbackName = value.Value
					}
					break
				}
			}
			fallback := "Agent's default"
			if fallbackName != "" {
				fallback += " (" + fallbackName + ")"
			}
			labels := []string{fallback}
			values := map[string]string{fallback: ""}
			selected := fallback
			for _, value := range option.Values {
				label := value.Name
				if label == "" {
					label = value.Value
				}
				labels = append(labels, label)
				values[label] = value.Value
				if defaults[option.ID] == value.Value {
					selected = label
				}
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, option.Name).Width(112).FontSize(11).
					TextColor(oi.textMuted).SingleLine().Ellipsis("…")
				if choice := ui.Select(c, &selected, labels).Grow(1); choice.Changed() {
					m.setAgentDefaultChoice(provider.ID, option.ID, values[selected])
				}
			})
		}
		ui.Text(c, "What you pick on the New Task page wins; these apply until then and after switching agents.").
			FontSize(10).TextColor(oi.textMuted)
		ui.Text(c, "Launch").FontSize(12).Bold().TextColor(oi.textStrong)
		if launch != "" {
			ui.Text(c, launch).Font(oiFontMono).FontSize(10).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…").Tooltip(launch)
		}
		if provider.CommandEditable {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Executable").Width(112).FontSize(11).TextColor(oi.textMuted)
				input := ui.TextInput(c.Key("agent-executable:"+provider.ID), &command).
					Grow(1).Font(oiFontMono).Disabled(busy).
					Placeholder("Auto-detect")
				if input.Changed() {
					st.CommandDrafts[provider.ID] = command
					m.invalidate()
				}
				ui.Button(c, "Browse…").Disabled(busy).OnClick(func() {
					m.chooseAgentExecutable(provider.ID)
				})
				ui.Button(c, "Reset").Disabled(busy || command == "").OnClick(func() {
					st.CommandDrafts[provider.ID] = ""
					m.invalidate()
				})
			})
			ui.Text(c, "Leave empty to find the executable on PATH. Changes apply to new sessions.").
				FontSize(10).TextColor(oi.textMuted)
		} else {
			message := "This agent is configured in its agent file."
			switch readOnlyReason {
			case "packageRunner":
				message = "This agent runs through a package runner and has no executable to choose."
			case "registry":
				message = "This agent is installed from the ACP Registry, which launches it."
			}
			ui.Text(c, message).FontSize(10).TextColor(oi.textMuted)
		}
		if envEditable {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Environment").Width(112).FontSize(11).TextColor(oi.textMuted)
				input := ui.TextInput(c.Key("agent-env:"+provider.ID), &envText).
					Grow(1).Font(oiFontMono).Disabled(busy).
					Placeholder("NAME=value; OTHER=value")
				if input.Changed() {
					st.EnvDrafts[provider.ID] = envText
					m.invalidate()
				}
			})
			ui.Text(c, "Passed to the agent process and replaces variables with the same name.").
				FontSize(10).TextColor(oi.textMuted)
		}
		if err := st.ProviderErr[provider.ID]; err != "" {
			ui.Text(c, err).FontSize(11).TextColor(oi.danger)
		}
		if provider.CommandEditable || envEditable {
			ui.Row(c).Gap(6).Children(func() {
				ui.Button(c, "Apply").Disabled(busy).OnClick(func() {
					m.applyAgentProviderSettings(provider.ID)
				})
				if busy {
					ui.Text(c, "Saving…").FontSize(11).TextColor(oi.textMuted)
				}
			})
		}
		if len(provider.Commands) > 0 {
			ui.Text(c, fmt.Sprintf("%d commands available", len(provider.Commands))).
				FontSize(11).TextColor(oi.textMuted).SingleLine()
		}
	})
}
