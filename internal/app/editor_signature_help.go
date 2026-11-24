package app

import (
	"context"
	"encoding/json"
	"fmt"

	"mayoi/internal/openidea"
)

// ShowParameterInfo is code.parameterInfo. It asks the language servers for
// the call at the caret and shows the active signature in the native hover,
// with the server documentation below it.
func (m *Model) ShowParameterInfo() {
	m.ShowParameterInfoAt(m.tab())
}

func (m *Model) ShowParameterInfoAt(t *Tab) {
	if t == nil || t.state == nil || t.doc == nil || !m.tabOpen(t) {
		return
	}
	if m.ctx == nil || m.mgr == nil && m.invokeHook == nil {
		m.status = "Parameter info requires the GoIDEA backend"
		m.invalidate()
		return
	}
	caret := t.state.Cursor()
	version := t.state.Buffer.Version()
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(caret)
	t.signatureGen++
	gen := t.signatureGen
	m.status = "Finding parameter info…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		if err := t.doc.Sync(ctx); err != nil {
			m.finishParameterInfo(t, gen, docID, version, caret, nil, err)
			return
		}
		raw, err := m.invoke(ctx, "lsp_signature_help", map[string]any{
			"doc": docID, "pos": lspPos(pos),
		})
		var help *openidea.SignatureHelp
		if err == nil {
			err = json.Unmarshal(raw, &help)
		}
		m.finishParameterInfo(t, gen, docID, version, caret, help, err)
	}()
}

func (m *Model) finishParameterInfo(
	t *Tab,
	gen uint64,
	docID uint32,
	version uint64,
	caret int,
	help *openidea.SignatureHelp,
	err error,
) {
	m.post(func() {
		if !m.parameterInfoCurrent(t, gen, docID, version, caret) {
			return
		}
		if err != nil {
			m.status = "Finding parameter info failed: " + err.Error()
			m.invalidate()
			return
		}
		text, ok := parameterInfoText(help)
		if !ok {
			m.status = "No parameter information found"
			m.invalidate()
			return
		}
		t.state.ShowHover(caret, caret, text, nil)
		m.status = "Parameter info"
		m.invalidate()
	})
}

func (m *Model) parameterInfoCurrent(
	t *Tab,
	gen uint64,
	docID uint32,
	version uint64,
	caret int,
) bool {
	return t != nil && t.doc != nil && t.state != nil && t.state.Buffer != nil &&
		m.tabOpen(t) && t.signatureGen == gen && t.doc.ID() == docID &&
		t.state.Buffer.Version() == version && t.state.Cursor() == caret
}

func parameterInfoText(help *openidea.SignatureHelp) (string, bool) {
	if help == nil || len(help.Signatures) == 0 {
		return "", false
	}
	index := int(help.ActiveSignature)
	if index < 0 || index >= len(help.Signatures) {
		index = 0
	}
	signature := help.Signatures[index]
	if signature.Label == "" {
		return "", false
	}
	text := signature.Label
	if len(signature.Parameters) > 0 {
		parameter := int(help.ActiveParameter)
		if parameter >= 0 && parameter < len(signature.Parameters) {
			text = fmt.Sprintf("%s\nParameter %d of %d", text, parameter+1, len(signature.Parameters))
		}
	}
	if signature.Documentation != nil {
		if docs := hoverText(*signature.Documentation); docs != "" {
			text += "\n\n" + docs
		}
	}
	return text, true
}
