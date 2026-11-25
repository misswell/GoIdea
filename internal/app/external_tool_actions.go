package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

var externalActionMu sync.Mutex

// dynamicActionGet resolves actions backed by model preferences. External
// Tools are registered when their model exists; this fallback lets callers
// resolve the action before a window has synchronized the registry.
func dynamicActionGet(id string) (*Action, bool) {
	const prefix = "externalTools."
	toolID, ok := strings.CutPrefix(id, prefix)
	if !ok || toolID == "" {
		return nil, false
	}
	return &Action{
		ID:       id,
		Title:    "External Tool",
		Category: "External Tools",
		When:     func(m *Model) bool { return m != nil && m.Root() != "" },
		Run:      func(m *Model) { m.RunExternalTool(toolID) },
	}, true
}

func dynamicActionGetForModel(m *Model, id string) (*Action, bool) {
	const prefix = "externalTools."
	toolID, ok := strings.CutPrefix(id, prefix)
	if !ok || m == nil {
		return nil, false
	}
	tool, ok := m.externalTool(toolID)
	if !ok || !tool.Enabled {
		return nil, false
	}
	return &Action{
		ID:          id,
		Title:       tool.Name,
		Category:    tool.Group,
		Description: tool.Description,
		Keywords:    strings.Join([]string{"external tool", "tools", tool.Description}, " "),
		When:        func(m *Model) bool { return m != nil && m.Root() != "" },
		Run:         func(m *Model) { m.RunExternalTool(tool.ID) },
	}, true
}

// syncExternalToolActions keeps the action registry in step with Settings ›
// Tools › External Tools. Disabled tools stay in settings and in before-launch
// task lists, but do not appear in Find Action.
func (m *Model) syncExternalToolActions() {
	externalActionMu.Lock()
	defer externalActionMu.Unlock()
	for _, action := range actionList() {
		if strings.HasPrefix(action.ID, "externalTools.") {
			unregisterAction(action.ID)
		}
	}
	for _, tool := range m.prefs.ExternalTools {
		if !tool.Enabled || tool.ID == "" || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		tool := tool
		action := &Action{
			ID:          "externalTools." + tool.ID,
			Title:       tool.Name,
			Category:    tool.Group,
			Description: tool.Description,
			Keywords:    strings.Join([]string{"external tool", "tools", tool.Description}, " "),
			When:        func(m *Model) bool { return m.Root() != "" },
			Run:         func(m *Model) { m.RunExternalTool(tool.ID) },
		}
		if action.Category == "" {
			action.Category = "External Tools"
		}
		_ = registerAction(action)
	}
}

// RunExternalTool expands one tool against the active editor and starts it in
// the Run tool window, as OpenIDEA's externalTools.<id> action does.
func (m *Model) RunExternalTool(id string) {
	go func() {
		tool, ok := m.externalTool(id)
		if !ok {
			m.post(func() {
				m.notifyError("external-tool", "External tool not found", id)
			})
			return
		}
		cfg, err := m.expandExternalTool(tool)
		if err != nil {
			m.post(func() {
				m.notifyError("external-tool", "'"+tool.Name+"' was not run", err.Error())
			})
			return
		}
		raw, err := json.Marshal(cfg)
		if err != nil {
			m.post(func() {
				m.notifyError("external-tool", "'"+tool.Name+"' was not run", err.Error())
			})
			return
		}
		var wasVisible bool
		var previousTool string
		m.post(func() {
			wasVisible = m.run.visible
			previousTool = m.visibleTool()
			if !m.startRunWith(raw, tool.Name) {
				m.notifyError("external-tool", "'"+tool.Name+"' was not started", "")
				return
			}
			if !tool.OpenConsole && !wasVisible {
				m.run.visible = false
				if previousTool != "" {
					m.activateTool(previousTool)
				}
			}
		})
		if tool.SynchronizeAfter {
			m.waitForExternalToolThenRefresh()
		}
	}()
}

func (m *Model) waitForExternalToolThenRefresh() {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var done bool
		m.post(func() {
			done = !m.run.busy && !m.run.running && !m.run.awaiting
		})
		if done {
			m.post(func() { m.ReloadProjectTree() })
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
