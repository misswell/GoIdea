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
		// The server formats the text it was last given; typing since the
		// debounce sync has to land first or its ranges mean other text.
		if serr := t.doc.Sync(ctx); serr != nil {
			m.finishFormat(t, serr, false, 0)
			return
		}
		raw, err := m.mgr.Invoke(ctx, Window, "lsp_format", map[string]any{
			"doc":   docID,
			"range": nil,
			"options": map[string]any{
				"tabSize": 4, "insertSpaces": true, "onSave": false, "tool": nil,
			},
		})
		var edits []openidea.TextEdit
		if err == nil {
			err = json.Unmarshal(raw, &edits)
		}
		var applied int
		var stale bool
		if err == nil {
			// The ranges are against the text the server formatted; typing
			// since then makes them mean something else.
			if t.doc.Buffer().Version() != version {
				stale = true
			} else if len(edits) > 0 {
				es := t.doc.ByteEdits(edits)
				if aerr := t.doc.ApplyEdits(ctx, es...); aerr != nil {
					err = aerr
				} else {
					applied = len(es)
				}
			}
		}
		m.finishFormat(t, err, stale, applied)
	}()
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
