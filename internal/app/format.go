package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"mayoi/internal/openidea"
)

// FormatActive reformats the active document through its language server, as
// IDEA's Reformat Code: the server's edits replace the text as one undo step.
func (m *Model) FormatActive() {
	if t := m.tab(); t != nil {
		m.formatTab(t)
	}
}

func (m *Model) formatTab(t *Tab) {
	if !m.tabOpen(t) || t.formatting {
		return
	}
	t.formatting = true
	m.status = "Formatting " + filepath.Base(t.doc.Path()) + "…"
	m.invalidate()
	docID := t.doc.ID()
	version := t.doc.Buffer().Version()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		applied, stale, err := m.applyLanguageFormat(ctx, t, docID, version, false)
		m.finishFormat(t, err, stale, applied)
	}()
}

// applyLanguageFormat asks the language server for whole-document formatting
// edits and applies them when the document still matches the version sent.
// It is shared by Reformat Code and Format on Save.
func (m *Model) applyLanguageFormat(
	ctx context.Context,
	t *Tab,
	docID uint32,
	version uint64,
	onSave bool,
) (applied int, stale bool, err error) {
	// The server formats the text it was last given; typing since the
	// debounce sync has to land first or its ranges mean other text.
	if err = t.doc.Sync(ctx); err != nil {
		return 0, false, err
	}
	raw, err := m.invoke(ctx, "lsp_format", map[string]any{
		"doc":   docID,
		"range": nil,
		"options": map[string]any{
			"tabSize": 4, "insertSpaces": true, "onSave": onSave, "tool": nil,
		},
	})
	var edits []openidea.TextEdit
	if err == nil {
		err = json.Unmarshal(raw, &edits)
	}
	if err != nil {
		return 0, false, err
	}
	// The ranges are against the text the server formatted; typing since then
	// makes them mean something else.
	if t.doc.Buffer().Version() != version {
		return 0, true, nil
	}
	if len(edits) == 0 {
		return 0, false, nil
	}
	es := t.doc.ByteEdits(edits)
	if err := t.doc.ApplyEdits(ctx, es...); err != nil {
		return 0, false, err
	}
	return len(es), false, nil
}

func (m *Model) finishFormat(t *Tab, err error, stale bool, applied int) {
	m.post(func() {
		t.formatting = false
		switch {
		case err != nil:
			m.fail(t.doc.Path(), fmt.Errorf("Formatting failed: %w", err))
		case stale:
			m.status = "The text changed and was not formatted; try again"
		case applied == 0:
			m.status = "Formatting done; nothing changed"
		default:
			m.status = fmt.Sprintf("Formatted: %d changes", applied)
		}
		m.invalidate()
	})
}
