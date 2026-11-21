package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type agentMCPKeyValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type agentMCPServer struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Scope     string             `json:"scope"`
	Transport string             `json:"transport"`
	Enabled   bool               `json:"enabled"`
	Command   *string            `json:"command"`
	Args      []string           `json:"args"`
	Env       []agentMCPKeyValue `json:"env"`
	URL       *string            `json:"url"`
	Headers   []agentMCPKeyValue `json:"headers"`
	Agents    []string           `json:"agents"`
}

type agentMCPServerInput struct {
	ID        *string            `json:"id"`
	Name      string             `json:"name"`
	Scope     string             `json:"scope"`
	Transport string             `json:"transport"`
	Enabled   bool               `json:"enabled"`
	Command   *string            `json:"command"`
	Args      []string           `json:"args"`
	Env       []agentMCPKeyValue `json:"env"`
	URL       *string            `json:"url"`
	Headers   []agentMCPKeyValue `json:"headers"`
	Agents    []string           `json:"agents"`
}

type agentMCPEditState struct {
	ID          string
	Mode        string
	Scope       string
	Transport   string
	Name        string
	Command     string
	ArgsText    string
	EnvText     string
	URL         string
	HeadersText string
	AgentsText  string
	Enabled     bool
	Busy        bool
	Err         string
}

type agentMCPSettingsState struct {
	Loaded        bool
	Loading       bool
	Request       uint64
	Servers       []agentMCPServer
	Err           string
	Notice        string
	Query         string
	CollapsedUser bool
	CollapsedWork bool
	BusyID        string
	Importing     bool
	Editing       *agentMCPEditState
}

func (m *Model) LoadAgentMCP(force bool) {
	st := &m.agents.mcp
	if st.Loading || (!force && st.Loaded) || !m.agentSettingsBackendAvailable() {
		return
	}
	st.Request++
	request := st.Request
	st.Loading = true
	st.Err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_mcp_list", nil)
		var servers []agentMCPServer
		if err == nil {
			err = json.Unmarshal(raw, &servers)
		}
		m.post(func() {
			if st.Request != request {
				return
			}
			st.Loading = false
			if err != nil {
				st.Err = err.Error()
			} else {
				st.Servers = servers
				st.Loaded = true
			}
			m.invalidate()
		})
	}()
}

func (m *Model) agentMCPSetEnabled(server agentMCPServer, enabled bool) {
	st := &m.agents.mcp
	if st.BusyID != "" || !m.agentSettingsBackendAvailable() {
		return
	}
	st.BusyID = server.ID
	st.Err = ""
	for index := range st.Servers {
		if st.Servers[index].ID == server.ID {
			st.Servers[index].Enabled = enabled
			break
		}
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_mcp_set_enabled", map[string]any{
			"id": server.ID, "enabled": enabled,
		})
		var servers []agentMCPServer
		if err == nil {
			err = json.Unmarshal(raw, &servers)
		}
		m.post(func() {
			st.BusyID = ""
			if err != nil {
				st.Err = err.Error()
				m.LoadAgentMCP(true)
			} else {
				st.Servers = servers
			}
			m.invalidate()
		})
	}()
}

func (m *Model) agentMCPRemove(server agentMCPServer) {
	st := &m.agents.mcp
	if st.BusyID != "" || !m.agentSettingsBackendAvailable() {
		return
	}
	st.BusyID = server.ID
	st.Err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_mcp_remove", map[string]any{"id": server.ID})
		var servers []agentMCPServer
		if err == nil {
			err = json.Unmarshal(raw, &servers)
		}
		m.post(func() {
			st.BusyID = ""
			if err != nil {
				st.Err = err.Error()
			} else {
				st.Servers = servers
			}
			m.invalidate()
		})
	}()
}

func (m *Model) agentMCPImport() {
	st := &m.agents.mcp
	if st.Importing || !m.agentSettingsBackendAvailable() {
		return
	}
	before := 0
	for _, server := range st.Servers {
		if server.Scope == "workspace" {
			before++
		}
	}
	st.Importing = true
	st.Err = ""
	st.Notice = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_mcp_import", nil)
		var servers []agentMCPServer
		if err == nil {
			err = json.Unmarshal(raw, &servers)
		}
		m.post(func() {
			st.Importing = false
			if err != nil {
				st.Err = err.Error()
			} else {
				st.Servers = servers
				after := 0
				for _, server := range servers {
					if server.Scope == "workspace" {
						after++
					}
				}
				switch added := after - before; added {
				case 0:
					st.Notice = "No new MCP servers in the project's .mcp.json or .vscode/mcp.json."
				case 1:
					st.Notice = "Imported 1 MCP server from the project."
				default:
					st.Notice = fmt.Sprintf("Imported %d MCP servers from the project.", added)
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) openAgentMCPEditor(mode, scope string, server *agentMCPServer) {
	edit := &agentMCPEditState{
		Mode: mode, Scope: scope, Transport: "stdio", Enabled: true,
	}
	if server != nil {
		edit.ID = server.ID
		edit.Scope = server.Scope
		edit.Transport = server.Transport
		edit.Name = server.Name
		edit.Enabled = server.Enabled
		if server.Command != nil {
			edit.Command = *server.Command
		}
		edit.ArgsText = strings.Join(server.Args, " ")
		edit.EnvText = formatAgentMCPKeyValues(server.Env)
		if server.URL != nil {
			edit.URL = *server.URL
		}
		edit.HeadersText = formatAgentMCPKeyValues(server.Headers)
		edit.AgentsText = strings.Join(server.Agents, ", ")
	}
	m.agents.mcp.Editing = edit
	m.invalidate()
}

func formatAgentMCPKeyValues(values []agentMCPKeyValue) string {
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, value.Name+"="+value.Value)
	}
	return strings.Join(lines, "\n")
}

func parseAgentMCPKeyValues(text string) []agentMCPKeyValue {
	out := make([]agentMCPKeyValue, 0)
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		eq, colon := strings.IndexByte(line, '='), strings.IndexByte(line, ':')
		index := eq
		if index < 0 || colon >= 0 && colon < index {
			index = colon
		}
		if index < 0 {
			out = append(out, agentMCPKeyValue{Name: line})
			continue
		}
		name := strings.TrimSpace(line[:index])
		if name == "" {
			continue
		}
		out = append(out, agentMCPKeyValue{
			Name: name, Value: strings.TrimSpace(line[index+1:]),
		})
	}
	return out
}

func parseAgentMCPArgs(text string) []string {
	var out []string
	var current strings.Builder
	var quote rune
	flush := func() {
		if current.Len() > 0 {
			out = append(out, current.String())
			current.Reset()
		}
	}
	for _, ch := range strings.TrimSpace(text) {
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
			} else {
				current.WriteRune(ch)
			}
		case ch == '\'' || ch == '"':
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

func parseAgentMCPAgents(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func (m *Model) saveAgentMCPEditor() {
	st := &m.agents.mcp
	edit := st.Editing
	if edit == nil || edit.Busy || !m.agentSettingsBackendAvailable() {
		return
	}
	name := strings.TrimSpace(edit.Name)
	command, url := strings.TrimSpace(edit.Command), strings.TrimSpace(edit.URL)
	if name == "" || edit.Transport == "stdio" && command == "" ||
		edit.Transport != "stdio" && url == "" {
		return
	}
	var id *string
	if edit.ID != "" {
		value := edit.ID
		id = &value
	}
	input := agentMCPServerInput{
		ID: id, Name: name, Scope: edit.Scope, Transport: edit.Transport,
		Enabled: edit.Enabled, Args: []string{}, Env: []agentMCPKeyValue{},
		Headers: []agentMCPKeyValue{},
		Agents:  parseAgentMCPAgents(edit.AgentsText),
	}
	if edit.Transport == "stdio" {
		input.Command = &command
		input.Args = parseAgentMCPArgs(edit.ArgsText)
		input.Env = parseAgentMCPKeyValues(edit.EnvText)
	} else {
		input.URL = &url
		input.Headers = parseAgentMCPKeyValues(edit.HeadersText)
	}
	edit.Busy = true
	edit.Err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "agent_mcp_save", map[string]any{"input": input})
		var servers []agentMCPServer
		if err == nil {
			err = json.Unmarshal(raw, &servers)
		}
		m.post(func() {
			if st.Editing != edit {
				return
			}
			edit.Busy = false
			if err != nil {
				edit.Err = err.Error()
			} else {
				st.Servers = servers
				st.Editing = nil
			}
			m.invalidate()
		})
	}()
}

func agentMCPServerSummary(server agentMCPServer) string {
	endpoint := ""
	if server.Transport == "stdio" {
		parts := make([]string, 0, len(server.Args)+1)
		if server.Command != nil {
			parts = append(parts, *server.Command)
		}
		parts = append(parts, server.Args...)
		endpoint = strings.Join(parts, " ")
	} else if server.URL != nil {
		endpoint = strings.ToUpper(server.Transport) + " · " + *server.URL
	}
	if len(server.Agents) > 0 {
		agents := strings.Join(server.Agents, ", ")
		if endpoint == "" {
			return agents
		}
		return endpoint + " (" + agents + ")"
	}
	return endpoint
}

func (m *Model) settingsAgentMCPBody(c *ui.Context, oi oiColors) {
	st := &m.agents.mcp
	m.LoadAgentMCP(false)

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "External MCP Servers").FontSize(16).Bold().
			TextColor(oi.textStrong).Grow(1).SingleLine()
		ui.Button(c, "Import from Project").Disabled(st.Importing).
			Tooltip("Import MCP servers from .mcp.json and .vscode/mcp.json").
			OnClick(m.agentMCPImport)
		ui.Button(c, "Browse").OnClick(func() {
			openReleasePage("https://github.com/modelcontextprotocol/servers")
		})
	})
	ui.Text(c, "An open standard that lets AI use external tools and services. "+
		"MCP servers provide tools for file operations, databases, APIs, and more.").
		FontSize(12).TextColor(oi.textMuted)
	ui.Row(c).Height(32).AlignItems(ui.Center).Children(func() {
		in := ui.TextInput(c.Key("agent-mcp-search"), &st.Query).Grow(1).
			Placeholder("Type to search...")
		if in.Changed() {
			m.invalidate()
		}
	})
	if st.Err != "" {
		ui.Text(c, st.Err).FontSize(12).TextColor(oi.danger).SingleLine().Ellipsis("…")
	}
	if st.Notice != "" {
		ui.Text(c, st.Notice).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	}
	if st.Loading && !st.Loaded {
		ui.Text(c, "Loading MCP servers…").FontSize(12).TextColor(oi.textMuted).SingleLine()
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Gap(12).Children(func() {
			m.agentMCPSection(c, oi, "user", "User", st.CollapsedUser)
			m.agentMCPSection(c, oi, "workspace", "Workspace", st.CollapsedWork)
		})
	})
	m.agentMCPEditorDialog(c, oi)
}

func (m *Model) agentMCPSection(c *ui.Context, oi oiColors, scope, title string, collapsed bool) {
	st := &m.agents.mcp
	servers := make([]agentMCPServer, 0)
	needle := strings.ToLower(strings.TrimSpace(st.Query))
	for _, server := range st.Servers {
		if server.Scope != scope {
			continue
		}
		haystack := strings.ToLower(server.Name + "\n" + agentMCPServerSummary(server) + "\n" +
			strings.Join(server.Agents, " "))
		if needle == "" || strings.Contains(haystack, needle) {
			servers = append(servers, server)
		}
	}
	sort.SliceStable(servers, func(i, j int) bool {
		return strings.ToLower(servers[i].Name) < strings.ToLower(servers[j].Name)
	})

	ui.Column(c).Gap(3).Children(func() {
		ui.Row(c).Height(30).Gap(6).AlignItems(ui.Center).Children(func() {
			arrow := "▾"
			if collapsed {
				arrow = "▸"
			}
			ui.ButtonBase(c.Key("agent-mcp-section:"+scope)).Height(26).Padding(0, 5).
				Radius(4).Cursor(ui.CursorPointer).OnClick(func() {
				if scope == "user" {
					st.CollapsedUser = !st.CollapsedUser
				} else {
					st.CollapsedWork = !st.CollapsedWork
				}
				m.invalidate()
			}).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, arrow).FontSize(12).TextColor(oi.textMuted)
					ui.Text(c, title).FontSize(13).Bold().TextColor(oi.textStrong)
					ui.Text(c, fmt.Sprint(len(servers))).FontSize(10).TextColor(oi.textMuted)
				})
			})
			ui.Spacer(c)
			ui.Button(c, "Add").Tooltip("Add " + title + " MCP Server").OnClick(func() {
				m.openAgentMCPEditor("add", scope, nil)
			})
		})
		if collapsed {
			return
		}
		if scope == "workspace" {
			ui.Text(c, "Servers added to the project's settings file outside GoIDEA stay off until you turn them on here.").
				FontSize(10).TextColor(oi.textMuted)
		}
		if len(servers) == 0 {
			ui.Text(c, "No MCP servers").FontSize(11).TextColor(oi.textMuted).SingleLine()
			return
		}
		for _, server := range servers {
			server := server
			row := ui.Row(c.Key("agent-mcp:"+server.ID)).MinHeight(42).Padding(2, 5).
				Gap(8).AlignItems(ui.Center).Radius(4)
			if row.Hovered() {
				row.Background(oi.hoverTint)
			}
			row.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, server.Name).FontSize(12).TextColor(oi.text).SingleLine().
						Ellipsis("…")
					summary := agentMCPServerSummary(server)
					if summary != "" {
						ui.Text(c, summary).FontSize(10).TextColor(oi.textMuted).
							SingleLine().Ellipsis("…").Tooltip(summary)
					}
				})
				enabled := server.Enabled
				check := ui.Checkbox(c, &enabled, "").Disabled(st.BusyID != "")
				if check.Changed() {
					m.agentMCPSetEnabled(server, enabled)
				}
				ui.Button(c, "⋯").Disabled(st.BusyID != "").OnClick(func() {
					m.openAgentMCPEditor("edit", server.Scope, &server)
				}).Menu(func(menu *ui.Menu) {
					label := "Enable Server"
					if server.Enabled {
						label = "Disable Server"
					}
					if menu.Item(label).Chosen() {
						m.agentMCPSetEnabled(server, !server.Enabled)
					}
					if menu.Item("Configure…").Chosen() {
						m.openAgentMCPEditor("edit", server.Scope, &server)
					}
					if menu.Item("Copy Server Name").Chosen() {
						mygo.Clipboard.WriteText(server.Name)
					}
					menu.Separator()
					if menu.Item("Delete Server").Chosen() {
						m.agentMCPRemove(server)
					}
				})
			})
		}
	})
}

func (m *Model) agentMCPEditorDialog(c *ui.Context, oi oiColors) {
	st := &m.agents.mcp
	edit := st.Editing
	if edit == nil {
		return
	}
	open := true
	dlg := ui.Modal(c.Key("agent-mcp-editor"), &open, func() {
		ui.Column(c).Width(560).Gap(9).Children(func() {
			title := "Add MCP Server"
			action := "Add"
			if edit.Mode == "edit" {
				title, action = "Configure MCP Server", "Save"
			}
			ui.Text(c, title).FontSize(15).Bold().TextColor(oi.textStrong)
			m.agentMCPFormRow(c, oi, "Name:", func() {
				ui.TextInput(c.Key("agent-mcp-name"), &edit.Name).Grow(1).
					Placeholder("my-mcp-server")
			})
			m.agentMCPFormRow(c, oi, "Scope:", func() {
				labels := []string{"User (Global settings)", "Workspace settings"}
				shown := labels[0]
				if edit.Scope == "workspace" {
					shown = labels[1]
				}
				if choice := ui.Select(c, &shown, labels).Grow(1); choice.Changed() {
					if shown == labels[1] {
						edit.Scope = "workspace"
					} else {
						edit.Scope = "user"
					}
				}
			})
			m.agentMCPFormRow(c, oi, "Type:", func() {
				labels := []string{"stdio (Local Command)", "http (Streamable HTTP)", "sse (Server-Sent Events)"}
				values := []string{"stdio", "http", "sse"}
				shown := labels[0]
				for index, value := range values {
					if edit.Transport == value {
						shown = labels[index]
						break
					}
				}
				if choice := ui.Select(c, &shown, labels).Grow(1); choice.Changed() {
					for index, label := range labels {
						if shown == label {
							edit.Transport = values[index]
							break
						}
					}
				}
			})
			if edit.Transport == "stdio" {
				m.agentMCPFormRow(c, oi, "Command:", func() {
					ui.TextInput(c.Key("agent-mcp-command"), &edit.Command).Grow(1).
						Placeholder("npx")
				})
				m.agentMCPFormRow(c, oi, "Arguments:", func() {
					ui.TextInput(c.Key("agent-mcp-args"), &edit.ArgsText).Grow(1).
						Placeholder("-y @modelcontextprotocol/server-filesystem /path")
				})
				m.agentMCPFormRow(c, oi, "Environment:", func() {
					ui.TextArea(c.Key("agent-mcp-env"), &edit.EnvText).Height(72).
						Placeholder("API_KEY=...\nANOTHER_VAR=value")
				})
				ui.Text(c, "One KEY=VALUE entry per line.").FontSize(10).
					TextColor(oi.textMuted).SingleLine()
			} else {
				m.agentMCPFormRow(c, oi, "URL:", func() {
					ui.TextInput(c.Key("agent-mcp-url"), &edit.URL).Grow(1).
						Placeholder("https://mcp.linear.app/mcp")
				})
				m.agentMCPFormRow(c, oi, "HTTP headers:", func() {
					ui.TextArea(c.Key("agent-mcp-headers"), &edit.HeadersText).Height(72).
						Placeholder("Authorization=Bearer ...\nX-Custom-Header=value")
				})
				ui.Text(c, "One Header=Value entry per line.").FontSize(10).
					TextColor(oi.textMuted).SingleLine()
			}
			m.agentMCPFormRow(c, oi, "Agents:", func() {
				ui.TextInput(c.Key("agent-mcp-agents"), &edit.AgentsText).Grow(1).
					Placeholder("claude, codex")
			})
			ui.Text(c, "Leave empty to enable for all agents, or list agent IDs separated by commas.").
				FontSize(10).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			if edit.Err != "" {
				ui.Text(c, edit.Err).FontSize(12).TextColor(oi.danger)
			}
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				canSave := strings.TrimSpace(edit.Name) != "" &&
					(edit.Transport == "stdio" && strings.TrimSpace(edit.Command) != "" ||
						edit.Transport != "stdio" && strings.TrimSpace(edit.URL) != "")
				ui.Button(c, action).Disabled(!canSave || edit.Busy).
					OnClick(m.saveAgentMCPEditor)
				ui.Button(c, "Cancel").OnClick(func() { st.Editing = nil })
			})
		})
	})
	if dlg.Dismissed() || !open {
		st.Editing = nil
	}
}

func (m *Model) agentMCPFormRow(c *ui.Context, oi oiColors, label string, body func()) {
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).Width(126).FontSize(12).TextColor(oi.textMuted).SingleLine()
		ui.Column(c).Grow(1).MinWidth(0).Children(body)
	})
}
