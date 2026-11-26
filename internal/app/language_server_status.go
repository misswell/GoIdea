package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const languageServerBusyDelay = 500 * time.Millisecond

// languageServerInfo is the state that both lsp_servers and languageServer
// events update. Tool stays the backend's install key while Name is what the
// status popup shows.
type languageServerInfo struct {
	ID        uint32
	Name      string
	Tool      string
	Status    string
	Message   string
	Languages []string
	Root      string
	PID       *uint32
}

func init() {
	registerStatusItem(&StatusItem{
		ID: "code-insight.servers", Title: "Language Services", Side: "right", Order: 4,
		Visible: func(m *Model) bool { return len(m.languageServerList()) > 0 },
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.languageServersStatus(c, p, oi)
		},
	})
}

func (m *Model) clearLanguageServers() {
	m.languageServerOpen = false
	m.languageServers = nil
	m.languageServerLastEvent = nil
	m.languageServerRemovedAt = nil
	m.languageServerBusyGen = nil
	m.languageServerJavaReady = nil
	m.languageServerRefresh++
}

// RefreshLanguageServers reads the server snapshot behind Code Insight's
// status widget. Events that arrive while the request is in flight win over
// their older snapshot rows.
func (m *Model) RefreshLanguageServers() {
	root := m.Root()
	if root == "" || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		if root == "" {
			m.clearLanguageServers()
		}
		return
	}
	m.languageServerRefresh++
	generation := m.languageServerRefresh
	since := m.languageServerSeq
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_servers", nil)
		var servers []lspServerState
		if err == nil {
			err = json.Unmarshal(raw, &servers)
		}
		m.post(func() {
			if generation != m.languageServerRefresh || m.Root() != root {
				return
			}
			if err != nil {
				m.notifyWarning(
					"language-servers-status",
					"Could not read language servers",
					err.Error(),
				)
				return
			}
			next := make(map[uint32]languageServerInfo, len(servers))
			for _, server := range servers {
				if server.ID == 0 {
					continue
				}
				if removed := m.languageServerRemovedAt[server.ID]; removed > since {
					continue
				}
				entry := languageServerInfo{
					ID: server.ID, Name: server.Name, Tool: server.Name,
					Status: server.Status, Languages: append([]string(nil), server.Languages...),
					Root: server.Root, PID: server.PID,
				}
				if previous, ok := m.languageServers[server.ID]; ok &&
					(m.languageServerLastEvent[server.ID] > since) {
					entry.Status = previous.Status
					entry.Message = previous.Message
				}
				next[server.ID] = entry
			}
			m.languageServers = next
			for _, server := range m.languageServerList() {
				if server.Status == "missing" && !m.languageServerServedByOther(server) {
					m.promptMissingLanguageTool(server.Tool)
				}
			}
			m.invalidate()
		})
	}()
}

// acceptLanguageServerStatus applies one lifecycle event and reports whether
// a missing server still needs the install prompt.
func (m *Model) acceptLanguageServerStatus(e openidea.LanguageServerEvent) bool {
	if e.Server == 0 {
		return e.Status == "missing"
	}
	if m.languageServers == nil {
		m.languageServers = map[uint32]languageServerInfo{}
	}
	if m.languageServerLastEvent == nil {
		m.languageServerLastEvent = map[uint32]uint64{}
	}
	if m.languageServerBusyGen == nil {
		m.languageServerBusyGen = map[uint32]uint64{}
	}
	if m.languageServerJavaReady == nil {
		m.languageServerJavaReady = map[uint32]string{}
	}
	m.languageServerSeq++
	seq := m.languageServerSeq
	m.languageServerLastEvent[e.Server] = seq
	delete(m.languageServerRemovedAt, e.Server)
	previous, known := m.languageServers[e.Server]
	becameReady := e.Status == "ready" && (!known || previous.Status != "ready")
	if e.Status != "ready" && e.Status != "busy" {
		delete(m.languageServerJavaReady, e.Server)
	}
	if e.Status == "ready" && hasLanguage(previous.Languages, "java") {
		root := previous.Root
		if root == "" {
			root = m.Root()
		}
		if root != "" {
			m.languageServerJavaReady[e.Server] = root
		}
	}
	m.languageServerBusyGen[e.Server]++
	generation := m.languageServerBusyGen[e.Server]
	if e.Status == "busy" && known && previous.Status == "ready" {
		time.AfterFunc(languageServerBusyDelay, func() {
			m.post(func() {
				if m.languageServerBusyGen[e.Server] == generation {
					m.mergeLanguageServerEvent(e)
					m.invalidate()
				}
			})
		})
		return false
	}
	m.mergeLanguageServerEvent(e)
	if becameReady {
		m.invalidateSemanticTokens()
	}
	if !known {
		m.RefreshLanguageServers()
	}
	return e.Status == "missing" && !m.languageServerServedByOther(m.languageServers[e.Server])
}

func (m *Model) mergeLanguageServerEvent(e openidea.LanguageServerEvent) {
	if m.languageServers == nil {
		m.languageServers = map[uint32]languageServerInfo{}
	}
	entry := m.languageServers[e.Server]
	entry.ID = e.Server
	entry.Tool = e.Name
	entry.Name = e.Name
	if e.Label != nil && strings.TrimSpace(*e.Label) != "" {
		entry.Name = strings.TrimSpace(*e.Label)
	}
	entry.Status = e.Status
	entry.Message = ""
	if e.Message != nil {
		entry.Message = *e.Message
	}
	m.languageServers[e.Server] = entry
}

func (m *Model) acceptLanguageServerRemoved(e openidea.LanguageServerRemovedEvent) {
	m.languageServerSeq++
	seq := m.languageServerSeq
	if m.languageServerRemovedAt == nil {
		m.languageServerRemovedAt = map[uint32]uint64{}
	}
	m.languageServerRemovedAt[e.Server] = seq
	delete(m.languageServerLastEvent, e.Server)
	if m.languageServerBusyGen != nil {
		m.languageServerBusyGen[e.Server]++
	}
	delete(m.languageServerJavaReady, e.Server)
	delete(m.languageServers, e.Server)
	m.invalidate()
}

func (m *Model) languageServerList() []languageServerInfo {
	out := make([]languageServerInfo, 0, len(m.languageServers))
	for _, server := range m.languageServers {
		out = append(out, server)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (m *Model) languageServerServedByOther(server languageServerInfo) bool {
	if len(server.Languages) == 0 {
		return false
	}
	for _, other := range m.languageServers {
		if other.ID == server.ID || other.Status == "missing" {
			continue
		}
		all := true
		for _, language := range server.Languages {
			if !slices.Contains(other.Languages, language) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func languageServerSummary(servers []languageServerInfo, servedOther func(languageServerInfo) bool) (string, string) {
	if len(servers) == 0 {
		return "", ""
	}
	bad, busy, ready := 0, 0, 0
	active := make([]string, 0, len(servers))
	for _, server := range servers {
		switch server.Status {
		case "crashed", "misconfigured":
			bad++
		case "missing":
			if !servedOther(server) {
				bad++
			}
		case "starting", "busy":
			busy++
		case "ready":
			ready++
		}
		if server.Status != "stopped" {
			active = append(active, server.Name)
		}
	}
	tone := "idle"
	switch {
	case bad > 0:
		tone = "error"
	case busy > 0:
		tone = "busy"
	case ready > 0:
		tone = "ok"
	}
	names := active
	if len(names) == 0 {
		names = make([]string, 0, len(servers))
		for _, server := range servers {
			names = append(names, server.Name)
		}
	}
	names = slices.Compact(names)
	text := ""
	if len(names) == 1 {
		text = names[0]
	} else if len(names) > 1 {
		text = fmt.Sprintf("%s +%d", names[0], len(names)-1)
	}
	return text, tone
}

func languageServerStatusLabel(status string) string {
	switch status {
	case "starting":
		return "Starting…"
	case "ready":
		return "Running"
	case "busy":
		return "Busy"
	case "crashed":
		return "Crashed"
	case "stopped":
		return "Stopped"
	case "missing":
		return "Not installed"
	case "misconfigured":
		return "Not started"
	}
	return status
}

func languageServerTone(status string, oi oiColors) ui.Color {
	switch status {
	case "ready":
		return gutterAddedColor
	case "starting", "busy":
		return oi.warning
	case "crashed", "missing", "misconfigured":
		return oi.danger
	}
	return oi.textSubtle
}

func (m *Model) languageServersStatus(c *ui.Context, p *ui.Theme, oi oiColors) {
	servers := m.languageServerList()
	text, tone := languageServerSummary(servers, m.languageServerServedByOther)
	if text == "" {
		return
	}
	tk := oiTokensFor(m, p)
	b := ui.ButtonBase(c.Key("status:language-servers")).
		Height(tk.StatusBarHeight-tk.SpaceL).Padding(0, tk.SpaceM).
		Radius(tk.RadiusS).Gap(tk.SpaceS).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip("Language servers")
	if b.Hovered() || m.languageServerOpen {
		b.Background(oi.toolbarHover)
	}
	b.Children(func() {
		dot := oi.textSubtle
		switch tone {
		case "ok":
			dot = gutterAddedColor
		case "busy":
			dot = oi.warning
		case "error":
			dot = oi.danger
		}
		ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
		ui.Text(c, text).FontSize(tk.FontSizeM).TextColor(oi.textMuted).SingleLine()
	})
	if b.Clicked() {
		m.languageServerOpen = !m.languageServerOpen
		m.invalidate()
	}
	open := m.languageServerOpen
	ui.PopoverBase(c, b, &open, func(panel ui.Element) {
		panel.Width(480).Padding(10).Gap(7).Background(oi.popup).
			BorderWidth(1).BorderColor(oi.borderStrong).Radius(8)
		panel.Children(func() {
			ui.Text(c, "Language Servers").FontSize(13).Bold().
				TextColor(oi.textStrong).SingleLine()
			for _, server := range servers {
				m.languageServerStatusRow(c, oi, server)
			}
		})
	})
	if !open {
		m.languageServerOpen = false
	}
}

func (m *Model) languageServerStatusRow(c *ui.Context, oi oiColors, server languageServerInfo) {
	ui.Column(c.Key(fmt.Sprintf("language-server:%d", server.ID))).
		Padding(5, 6).Gap(2).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(7, 7).Radius(3.5).
				Background(languageServerTone(server.Status, oi))
			ui.Text(c, server.Name).FontSize(12).TextColor(oi.textStrong).SingleLine()
			ui.Text(c, languageServerStatusLabel(server.Status)).
				FontSize(11).TextColor(oi.textMuted).SingleLine()
			ui.Spacer(c)
			m.languageServerAction(c, server)
		})
		meta := strings.Join(server.Languages, ", ")
		if server.Root != "" && server.Root != m.Root() {
			root := server.Root
			if rel, err := filepath.Rel(m.Root(), server.Root); err == nil &&
				rel != "." && !strings.HasPrefix(rel, "..") {
				root = rel
			}
			if meta != "" {
				meta += " · "
			}
			meta += root
		}
		if meta != "" {
			ui.Text(c, meta).FontSize(11).TextColor(oi.textSubtle).
				SingleLine().Ellipsis("…")
		}
		if server.Message != "" {
			ui.Text(c, server.Message).FontSize(11).
				TextColor(languageServerTone(server.Status, oi)).Wrap()
		}
	})
}

func (m *Model) languageServerAction(c *ui.Context, server languageServerInfo) {
	if server.Status == "misconfigured" {
		if need, ok := requiredJDK(server.Message); ok {
			ui.Button(c, fmt.Sprintf("Download JDK %d", need.version)).OnClick(func() {
				m.languageServerOpen = false
				m.RunActionWithArg("java.downloadJdk", map[string]any{
					"version": need.version, "languageServer": need.languageServer,
				})
			})
			return
		}
	}
	if server.Status == "missing" {
		ui.Button(c, "Install").OnClick(func() {
			m.installLanguageServerFromStatus(server.Tool)
		})
		return
	}
	if server.Status == "missing" || server.Status == "starting" || server.ID == 0 {
		return
	}
	ui.Button(c, "Restart").OnClick(func() { m.restartLanguageServer(server.ID) })
}

func (m *Model) installLanguageServerFromStatus(name string) {
	m.languageServerOpen = false
	name = strings.TrimSpace(name)
	if name == "" {
		m.OpenSettingsPage(settingsPageLanguageServers)
		return
	}
	if m.toolsLoaded {
		for _, tool := range m.languageTools {
			if tool.Name == name && tool.CanInstall {
				m.InstallLanguageTool(name, false)
				return
			}
		}
		m.OpenSettingsPage(settingsPageLanguageServers)
		m.languageToolPick = name
		return
	}
	m.loadLanguageToolsForPrompt("status:"+name, func() {
		m.installLanguageServerFromStatus(name)
	})
}

func (m *Model) restartLanguageServer(id uint32) {
	if id == 0 {
		return
	}
	m.languageServerOpen = false
	entry := m.languageServers[id]
	entry.Status = "starting"
	entry.Message = ""
	m.languageServers[id] = entry
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if _, err := m.invoke(ctx, "lsp_restart", map[string]any{"server": id}); err != nil {
			m.post(func() {
				entry := m.languageServers[id]
				entry.Status = "crashed"
				entry.Message = "Failed to restart: " + err.Error()
				m.languageServers[id] = entry
				m.invalidate()
			})
		}
	}()
}
