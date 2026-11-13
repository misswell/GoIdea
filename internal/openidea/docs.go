package openidea

import (
	"context"
	"encoding/json"

	"goidea/internal/position"
)

// Docs is a docbridge.Backend over one manager and window: documents live in
// the backend window, and calls wait for a restart if the server is down.
type Docs struct {
	M      *Manager
	Window string
}

// DocOpen opens a file and returns its text, version and metadata.
func (d Docs) DocOpen(ctx context.Context, window, path string) (DocInfo, error) {
	var info DocInfo
	err := d.call(ctx, window, "doc_open", map[string]any{"path": path}, &info)
	return info, err
}

// DocEdit applies a UTF-16 change set to baseVersion and returns the new
// version, or an outOfSync error when the version is stale.
func (d Docs) DocEdit(ctx context.Context, window string, doc uint32, baseVersion uint32, changes position.ChangeSet) (uint32, error) {
	var v uint32
	err := d.call(ctx, window, "doc_edit", map[string]any{
		"doc":         doc,
		"baseVersion": baseVersion,
		"changes":     changes,
	}, &v)
	return v, err
}

// DocSnapshot returns the document's text and version, for resync.
func (d Docs) DocSnapshot(ctx context.Context, window string, doc uint32) (DocInfo, error) {
	var info DocInfo
	err := d.call(ctx, window, "doc_snapshot", map[string]any{"doc": doc}, &info)
	return info, err
}

// DocSave writes a dirty document.
func (d Docs) DocSave(ctx context.Context, window string, doc uint32) error {
	return d.call(ctx, window, "doc_save", map[string]any{"doc": doc}, nil)
}

// DocClose releases the document, which makes backend edits to it stop.
func (d Docs) DocClose(ctx context.Context, window string, doc uint32) error {
	return d.call(ctx, window, "doc_close", map[string]any{"doc": doc}, nil)
}

func (d Docs) call(ctx context.Context, window, cmd string, args, out any) error {
	raw, err := d.M.Invoke(ctx, window, cmd, args)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return decodeReply(raw, out)
}

func decodeReply(raw []byte, out any) error { return json.Unmarshal(raw, out) }
