package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type mcpServerSettingsState struct {
	status  openidea.McpServerStatus
	loaded  bool
	loading bool
	session bool
	root    string
	err     string
	loadGen uint64
	copied  string
}

func (m *Model) loadMCPServerStatus(force bool) {
	root := m.Root()
	state := m.mcpServer
	if !force && state.session && state.root == root && (state.loading || state.loaded) {
		return
	}
	state.session = true
	state.loading = true
	state.loaded = false
	state.root = root
	state.err = ""
	state.copied = ""
	state.loadGen++
	generation := state.loadGen
	m.mcpServer = state
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.loaded = true
		state.status = openidea.McpServerStatus{}
		m.mcpServer = state
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 10*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "mcp_server_status", nil)
		var status openidea.McpServerStatus
		if err == nil {
			err = json.Unmarshal(raw, &status)
		}
		m.post(func() {
			current := m.mcpServer
			if current.loadGen != generation || m.Root() != root {
				return
			}
			current.loading = false
			current.loaded = true
			current.status = status
			if err != nil {
				current.status = openidea.McpServerStatus{}
				current.err = "Could not read MCP server status: " + err.Error()
			}
			m.mcpServer = current
			m.invalidate()
		})
	}()
}

func mcpServerStatusText(status openidea.McpServerStatus) string {
	if status.Running {
		if status.Port != nil {
			return fmt.Sprintf("Running on port %d", *status.Port)
		}
		return "Running"
	}
	if status.Error != "" {
		return "Failed to start: " + status.Error
	}
	return "Not running"
}

func mcpHTTPClientConfiguration(status openidea.McpServerStatus) string {
	if status.URL == "" || status.Token == "" {
		return ""
	}
	entry := map[string]any{
		"type": "http",
		"url":  status.URL,
		"headers": map[string]string{
			"Authorization": "Bearer " + status.Token,
		},
	}
	raw, _ := json.MarshalIndent(map[string]any{
		"mcpServers": map[string]any{"openidea": entry},
	}, "", "  ")
	return string(raw)
}

func mcpStdioClientConfiguration(status openidea.McpServerStatus) string {
	if len(status.StdioCommand) == 0 || status.StdioCommand[0] == "" {
		return ""
	}
	entry := map[string]any{
		"type":    "stdio",
		"command": status.StdioCommand[0],
		"args":    append([]string{}, status.StdioCommand[1:]...),
	}
	raw, _ := json.MarshalIndent(map[string]any{
		"mcpServers": map[string]any{"openidea": entry},
	}, "", "  ")
	return string(raw)
}

func (m *Model) renderMCPServerEndpoint(c *ui.Context, oi oiColors) {
	m.loadMCPServerStatus(false)
	state := m.mcpServer
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Status", 230)
		if state.loading {
			ui.Text(c, "Loading status…").FontSize(12).TextColor(oi.textMuted).SingleLine()
			return
		}
		if state.err != "" {
			ui.Text(c, state.err).FontSize(12).TextColor(oi.danger).
				SingleLine().Ellipsis("…").Grow(1)
			ui.Button(c, "Retry").OnClick(func() { m.loadMCPServerStatus(true) })
			return
		}
		color := oi.textMuted
		if state.status.Running {
			color = oi.text
		} else if state.status.Error != "" {
			color = oi.danger
		}
		ui.Text(c, mcpServerStatusText(state.status)).FontSize(12).
			TextColor(color).SingleLine().Ellipsis("…")
	})

	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Endpoint", 230)
		endpoint := state.status.URL
		placeholder := "Not running"
		if state.status.Running {
			placeholder = "No project open"
		}
		ui.TextInput(c, &endpoint).ReadOnly(true).Grow(1).Placeholder(placeholder)
	})
	endpointHint := "Open a project to get its endpoint."
	if state.status.Project != "" {
		endpointHint = "Agents name this project with the X-OpenIDEA-Project header or the ?project= query."
	}
	ui.Text(c, endpointHint).FontSize(11).TextColor(oi.textMuted).
		PaddingStart(20).SingleLine().Ellipsis("…")

	ui.Text(c, "Client configuration").FontSize(12).Bold().TextColor(oi.text).
		PaddingStart(20).SingleLine()
	ui.Text(c, "HTTP holds this run's token. Stdio starts GoIDEA's executable and keeps working after a restart.").
		FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
	httpConfig := mcpHTTPClientConfiguration(state.status)
	stdioConfig := mcpStdioClientConfiguration(state.status)
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		ui.Button(c, "Copy HTTP Config").Disabled(httpConfig == "").OnClick(func() {
			mygo.Clipboard.WriteText(httpConfig)
			m.mcpServer.copied = "HTTP"
			m.invalidate()
		})
		ui.Button(c, "Copy Stdio Config").Disabled(stdioConfig == "").OnClick(func() {
			mygo.Clipboard.WriteText(stdioConfig)
			m.mcpServer.copied = "Stdio"
			m.invalidate()
		})
		if state.copied != "" {
			ui.Text(c, "Copied "+state.copied+" configuration to the clipboard").
				FontSize(11).TextColor(oi.textMuted).SingleLine()
		}
	})
}
