package openidea

import (
	"context"
	"encoding/json"
)

// NotebookSnapshot is the lossless view of an nbformat 4 document. The
// backend remains the authority for cell ids, versions, history and file
// preservation, so GoIDEA never rewrites the notebook itself.
type NotebookSnapshot struct {
	ID            string          `json:"id"`
	Path          string          `json:"path"`
	Version       uint32          `json:"version"`
	Dirty         bool            `json:"dirty"`
	CanUndo       bool            `json:"canUndo"`
	CanRedo       bool            `json:"canRedo"`
	NBFormat      uint32          `json:"nbformat"`
	NBFormatMinor uint32          `json:"nbformatMinor"`
	Metadata      json.RawMessage `json:"metadata"`
	Cells         []NotebookCell  `json:"cells"`
}

// NotebookCell is a code, Markdown or raw notebook cell. Outputs and
// attachments stay raw JSON so an unknown nbformat extension is preserved by
// the backend and ignored safely by the native renderer.
type NotebookCell struct {
	ID             string            `json:"id"`
	CellType       string            `json:"cellType"`
	Source         string            `json:"source"`
	Metadata       json.RawMessage   `json:"metadata"`
	Outputs        []json.RawMessage `json:"outputs"`
	ExecutionCount *uint32           `json:"executionCount"`
	Attachments    json.RawMessage   `json:"attachments"`
}

// NotebookEdit is the tagged payload accepted by notebook_edit.
type NotebookEdit struct {
	Type           string            `json:"type"`
	Cell           string            `json:"cell,omitempty"`
	Source         string            `json:"source,omitempty"`
	After          *string           `json:"after,omitempty"`
	CellType       string            `json:"cellType,omitempty"`
	Before         *string           `json:"before,omitempty"`
	Outputs        []json.RawMessage `json:"outputs,omitempty"`
	ExecutionCount *uint32           `json:"executionCount,omitempty"`
}

// NotebookOutputEvent is one streamed kernel event. Its content is raw
// Jupyter JSON because display bundles may carry MIME types MyGo does not yet
// render natively.
type NotebookOutputEvent struct {
	ID           string          `json:"id"`
	Path         string          `json:"path"`
	Cell         string          `json:"cell"`
	MsgID        string          `json:"msgId"`
	Kind         string          `json:"kind"`
	Content      json.RawMessage `json:"content"`
	Version      uint32          `json:"version"`
	Dirty        bool            `json:"dirty"`
	ChangedCells []string        `json:"changedCells"`
}

// NotebookCellDoc is a code cell's language-server document. The cell URI is
// also the path used by completions, hover, navigation and diagnostics.
type NotebookCellDoc struct {
	Cell string `json:"cell"`
	Doc  uint32 `json:"doc"`
	URI  string `json:"uri"`
}

// NotebookKernelEvent is the backend's kernel lifecycle event.
type NotebookKernelEvent struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	State   string `json:"state"`
	Message string `json:"message"`
}

func (m *Manager) NotebookOpen(ctx context.Context, window, path string) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_open", map[string]any{"path": path})
}

func (m *Manager) NotebookGet(ctx context.Context, window, id string) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_get", map[string]any{"id": id})
}

func (m *Manager) NotebookCellDocs(ctx context.Context, window, id string) ([]NotebookCellDoc, error) {
	raw, err := m.Invoke(ctx, window, "notebook_cell_docs", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	var docs []NotebookCellDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (m *Manager) NotebookEdit(ctx context.Context, window, id string, version uint32, edit NotebookEdit) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_edit", map[string]any{
		"id": id, "version": version, "edit": edit,
	})
}

func (m *Manager) NotebookSave(ctx context.Context, window, id string) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_save", map[string]any{"id": id})
}

func (m *Manager) NotebookReload(ctx context.Context, window, id string) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_reload", map[string]any{"id": id})
}

func (m *Manager) NotebookUndo(ctx context.Context, window, id string) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_undo", map[string]any{"id": id})
}

func (m *Manager) NotebookRedo(ctx context.Context, window, id string) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_redo", map[string]any{"id": id})
}

func (m *Manager) NotebookClose(ctx context.Context, window, id string) error {
	_, err := m.Invoke(ctx, window, "notebook_close", map[string]any{"id": id})
	return err
}

func (m *Manager) NotebookInterrupt(ctx context.Context, window, id string) error {
	_, err := m.Invoke(ctx, window, "notebook_interrupt", map[string]any{"id": id})
	return err
}

func (m *Manager) NotebookRestart(ctx context.Context, window, id string) (NotebookSnapshot, error) {
	return invokeNotebook(ctx, m, window, "notebook_restart", map[string]any{"id": id})
}

func invokeNotebook(ctx context.Context, m *Manager, window, command string, args any) (NotebookSnapshot, error) {
	raw, err := m.Invoke(ctx, window, command, args)
	if err != nil {
		return NotebookSnapshot{}, err
	}
	var snapshot NotebookSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return NotebookSnapshot{}, err
	}
	return snapshot, nil
}
