package openidea

import (
	"context"
	"encoding/json"
)

// The caller owns channel allocation, notification routing and cancellation.
// Cancellable RPCs use cancel_request with the same request ID. Server errors,
// including unsupported commands on older servers, are returned unchanged.
func (m *Manager) DockerEvents(ctx context.Context, window string, endpoint DockerEndpoint, request, channel uint32) error {
	_, err := m.Invoke(ctx, window, "docker_events", map[string]any{"endpoint": endpoint, "request": request, "onEvent": streamChannel(channel)})
	return err
}
func (m *Manager) DockerLogs(ctx context.Context, window string, endpoint DockerEndpoint, id string, tail uint32, follow, timestamps bool, request, channel uint32) error {
	_, err := m.Invoke(ctx, window, "docker_logs", map[string]any{"endpoint": endpoint, "id": id, "tail": tail, "follow": follow, "timestamps": timestamps, "request": request, "onLine": streamChannel(channel)})
	return err
}
func (m *Manager) DockerPull(ctx context.Context, window string, endpoint DockerEndpoint, reference string, request, channel uint32) error {
	_, err := m.Invoke(ctx, window, "docker_pull", map[string]any{"endpoint": endpoint, "reference": reference, "request": request, "onOutput": streamChannel(channel)})
	return err
}
func (m *Manager) DockerPrune(ctx context.Context, window string, endpoint DockerEndpoint, target string, request, channel uint32) error {
	_, err := m.Invoke(ctx, window, "docker_prune", map[string]any{"endpoint": endpoint, "target": target, "request": request, "onOutput": streamChannel(channel)})
	return err
}
func (m *Manager) DockerComposeOp(ctx context.Context, window string, endpoint DockerEndpoint, project string, files []string, workingDir *string, op string, request, channel uint32) error {
	if files == nil {
		files = []string{}
	}
	_, err := m.Invoke(ctx, window, "docker_compose_op", map[string]any{"endpoint": endpoint, "project": project, "files": files, "workingDir": workingDir, "op": op, "request": request, "onOutput": streamChannel(channel)})
	return err
}

// PluginTerminalSessionSpawn returns the server's terminal session data intact.
func (m *Manager) PluginTerminalSessionSpawn(ctx context.Context, window, operation string, cols, rows, channel uint32) (json.RawMessage, error) {
	return m.Invoke(ctx, window, "plugin_terminal_session_spawn", map[string]any{"operation": operation, "cols": cols, "rows": rows, "onOutput": streamChannel(channel)})
}
func streamChannel(id uint32) map[string]any { return map[string]any{"$channel": id} }

// LSPCodeActions returns the action payloads unchanged for application by the backend.
func (m *Manager) LSPCodeActions(ctx context.Context, window string, doc uint32, selection OutlineRange, only []string) ([]json.RawMessage, error) {
	if only == nil {
		only = []string{}
	}
	raw, err := m.Invoke(ctx, window, "lsp_code_actions", map[string]any{"doc": doc, "range": selection, "only": only})
	if err != nil {
		return nil, err
	}
	var actions []json.RawMessage
	err = json.Unmarshal(raw, &actions)
	return actions, err
}

// ReplacePreview retains the backend's edit objects in their original shape.
type ReplacePreview struct {
	Before string            `json:"before"`
	After  string            `json:"after"`
	Edits  []json.RawMessage `json:"edits"`
}

func (m *Manager) ReplacePreview(ctx context.Context, window string, query any, replacement string, preserveCase bool, path string, only []SearchPosition) (ReplacePreview, error) {
	var result ReplacePreview
	err := m.InvokeInto(ctx, window, "replace_preview", map[string]any{"query": query, "replacement": replacement, "preserveCase": preserveCase, "path": path, "only": only}, &result)
	return result, err
}
func (m *Manager) VCSRebase(ctx context.Context, window string, repo *string, onto string) error {
	_, err := m.Invoke(ctx, window, "vcs_rebase", map[string]any{"repo": repo, "onto": onto})
	return err
}
