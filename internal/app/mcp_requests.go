package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"mayoi/internal/openidea"
)

// mcpRequestsState mirrors OpenIDEA's window-local MCP request host. Only
// confirmation requests wait for input; file opens and agent commands answer
// as soon as the native action has run.
type mcpRequestsState struct {
	confirms []mcpConfirmRequest
	sending  map[uint32]bool
}

type mcpConfirmRequest struct {
	id      uint32
	request openidea.McpUIRequest
	errText string
}

func (m *Model) acceptMCPRequest(event openidea.McpRequestEvent) {
	switch event.Request.Type {
	case "confirm":
		if m.mcpHasConfirm(event.ID) {
			return
		}
		m.mcp.confirms = append(m.mcp.confirms, mcpConfirmRequest{
			id: event.ID, request: event.Request,
		})
		m.invalidate()
	case "agentRun":
		m.startAgentRun(event.Request, func(err error) {
			m.sendMCPResponse(event.ID, err == nil, errorText(err), nil)
		})
	case "openFile":
		if err := m.openMCPFile(event.ID, event.Request); err != nil {
			m.sendMCPResponse(event.ID, false, err.Error(), nil)
		}
	default:
		m.sendMCPResponse(event.ID, false, "Unknown MCP request type: "+event.Request.Type, nil)
	}
}

func (m *Model) dropMCPRequest(id uint32) {
	if m.mcp.sending != nil {
		delete(m.mcp.sending, id)
	}
	for i, request := range m.mcp.confirms {
		if request.id == id {
			m.mcp.confirms = append(m.mcp.confirms[:i], m.mcp.confirms[i+1:]...)
			m.invalidate()
			return
		}
	}
}

func (m *Model) mcpHasConfirm(id uint32) bool {
	if m.mcp.sending[id] {
		return true
	}
	for _, request := range m.mcp.confirms {
		if request.id == id {
			return true
		}
	}
	return false
}

func (m *Model) answerMCPConfirm(allow bool) {
	if len(m.mcp.confirms) == 0 {
		return
	}
	request := m.mcp.confirms[0]
	m.mcp.confirms = m.mcp.confirms[1:]
	retry := &request
	m.sendMCPResponse(request.id, allow, "", retry)
	m.invalidate()
}

func (m *Model) denyAllMCPRequests() {
	pending := append([]mcpConfirmRequest(nil), m.mcp.confirms...)
	m.mcp.confirms = nil
	for _, request := range pending {
		m.sendMCPResponse(request.id, false, "", nil)
	}
	m.invalidate()
}

func (m *Model) sendMCPResponse(id uint32, allow bool, errText string, retry *mcpConfirmRequest) {
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	if m.mcp.sending == nil {
		m.mcp.sending = map[uint32]bool{}
	}
	if m.mcp.sending[id] {
		return
	}
	m.mcp.sending[id] = true
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		var responseErr any
		if errText != "" {
			responseErr = errText
		}
		_, err := m.invoke(ctx, "mcp_respond", map[string]any{
			"id": id, "allow": allow, "error": responseErr,
		})
		m.post(func() {
			if m.mcp.sending == nil || !m.mcp.sending[id] {
				return
			}
			delete(m.mcp.sending, id)
			if err != nil && retry != nil && !m.mcpHasConfirm(id) {
				retry.errText = err.Error()
				m.mcp.confirms = append([]mcpConfirmRequest{*retry}, m.mcp.confirms...)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) openMCPFile(id uint32, request openidea.McpUIRequest) error {
	if request.Path == "" {
		return errors.New("the file path is missing")
	}
	info, err := os.Stat(request.Path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", request.Path)
	}
	line, column := 0, 0
	if request.Line != nil {
		line = max(0, *request.Line-1)
	}
	if request.Column != nil {
		column = max(0, *request.Column-1)
	}
	path := realPath(request.Path)
	m.recordNavigation(true)
	m.reveal = &revealPos{path: path, line: line, col: column, endLine: -1, endCol: -1}
	m.openWith(path, openSpec{
		at: -1,
		done: func(*Tab) {
			m.sendMCPResponse(id, true, "", nil)
		},
		failed: func(_ string, err error) {
			m.sendMCPResponse(id, false, err.Error(), nil)
		},
	})
	return nil
}

func (m *Model) mcpUIReady(ready bool) {
	m.mcpUIReadyThen(ready, nil)
}

func (m *Model) mcpUIReadyThen(ready bool, done func(error)) {
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "mcp_ui_ready", map[string]any{"ready": ready})
		if done != nil {
			m.post(func() { done(err) })
		}
	}()
}

func (m *Model) mcpProjectReady() {
	m.denyAllMCPRequests()
	m.mcpStopReports()
	epoch := m.mcpReports.epoch
	m.mcpUIReadyThen(true, func(err error) {
		if err == nil && m.mcpReports.epoch == epoch {
			m.mcpStartReports()
		}
	})
}

func (m *Model) mcpProjectClosed() {
	m.denyAllMCPRequests()
	m.mcpStopReports()
	m.mcpUIReady(false)
}

func (m *Model) notifyMCPAgentClosed(name string) {
	if name == "" || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "mcp_agent_run_closed", map[string]any{"name": name})
	}()
}

func (m *Model) mcpConfirmDialog(c *ui.Context, p *ui.Theme) {
	if len(m.mcp.confirms) == 0 {
		return
	}
	request := m.mcp.confirms[0]
	open := true
	choice := 0
	waiting := len(m.mcp.confirms) - 1
	modal := nativeModal(c.Key(fmt.Sprintf("mcp-confirm:%d", request.id)), &open, func() {
		dialog := ui.Column(c).Width(480).Gap(10).Children(func() {
			ui.Text(c, fmt.Sprintf("%s wants to run %s", request.request.Client, request.request.Subject)).
				FontSize(13).Bold().TextColor(p.Text)
			ui.Text(c, "Project: "+request.request.Project).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, "Tool: "+request.request.Tool).FontSize(12).TextColor(p.TextMuted)
			if request.request.Detail != "" {
				ui.Text(c, request.request.Detail).FontSize(12).Font("monospace").TextColor(p.Text)
			}
			if request.errText != "" {
				ui.Text(c, "Could not send the answer: "+request.errText).
					FontSize(12).TextColor(p.Warning)
			}
			if waiting > 0 {
				ui.Text(c, fmt.Sprintf("%d more waiting", waiting)).
					FontSize(12).TextColor(p.TextMuted)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Allow").OnClick(func() { choice = 1 })
				ui.Button(c, "Deny").OnClick(func() { choice = 2 })
			})
		})
		// A modal normally focuses its first button, and a focused button
		// consumes Enter. Focus the dialog instead so Enter and Escape deny,
		// as OpenIDEA does, while Tab still reaches Allow and Deny.
		dialog.Focusable().AutoFocus().HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && (ev.Key == ui.KeyEnter || ev.Key == ui.KeyEscape) {
				m.answerMCPConfirm(false)
				return true
			}
			return false
		})
	})
	if modal.Dismissed() || !open {
		choice = 2
	}
	switch choice {
	case 1:
		m.answerMCPConfirm(true)
	case 2:
		m.answerMCPConfirm(false)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
