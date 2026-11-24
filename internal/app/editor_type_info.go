package app

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"mayoi/internal/openidea"
)

var (
	typeInfoFence  = regexp.MustCompile("(?s)```([\\w+#-]*)[^\\n]*\\n(.*?)```")
	typeInfoInline = regexp.MustCompile("`([^`]+)`")
	typeInfoPath   = regexp.MustCompile(`^[\w$]+(?:(?:::|\.)[\w$]+)*$`)
	typeInfoColon  = regexp.MustCompile(
		`^(?:\([\w ]+\)\s*)?(?:(?:let|const|var|val|mut|static|readonly|export|declare|public|private|protected|internal|final|ref)\s+)*[\w$.]+\??\s*:\s*(.+)$`,
	)
	typeInfoGo = regexp.MustCompile(`^(?:var|field|const)\s+[\w$]+\s+(.+?)(?:\s*=.*)?$`)
)

// ShowExpressionTypeInfo is code.expressionTypeInfo. It reads the type from
// the same hover markup as OpenIDEA and shows it at the symbol or expression.
func (m *Model) ShowExpressionTypeInfo() {
	t := m.tab()
	if t == nil || t.state == nil || t.doc == nil || !m.tabOpen(t) {
		return
	}
	if m.ctx == nil || m.mgr == nil && m.invokeHook == nil {
		m.status = "Type information requires the GoIDEA backend"
		m.invalidate()
		return
	}
	caret := t.state.Cursor()
	version := t.state.Buffer.Version()
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(caret)
	request := m.nextRequest()
	t.typeInfoGen++
	gen := t.typeInfoGen
	m.status = "Finding type information…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		if err := t.doc.Sync(ctx); err != nil {
			m.finishExpressionTypeInfo(t, gen, docID, version, caret, nil, err)
			return
		}
		raw, err := m.invoke(ctx, "lsp_hover", map[string]any{
			"request": request, "doc": docID, "pos": lspPos(pos),
		})
		var hover *openidea.Hover
		if err == nil {
			err = json.Unmarshal(raw, &hover)
		}
		m.finishExpressionTypeInfo(t, gen, docID, version, caret, hover, err)
	}()
}

func (m *Model) finishExpressionTypeInfo(
	t *Tab,
	gen uint64,
	docID uint32,
	version uint64,
	caret int,
	hover *openidea.Hover,
	err error,
) {
	m.post(func() {
		if !m.typeInfoRequestCurrent(t, gen, docID, version, caret) {
			return
		}
		if err != nil {
			m.status = "Finding type information failed: " + err.Error()
			m.invalidate()
			return
		}
		text, ok := hoverTypeInfo(hover)
		if !ok {
			m.status = "No type information found"
			m.invalidate()
			return
		}
		start, end := caret, caret
		if hover.Range != nil {
			start, end = t.doc.ByteRange(*hover.Range)
		}
		t.state.ShowHover(start, end, text, nil)
		m.status = "Expression type info"
		m.invalidate()
	})
}

func (m *Model) typeInfoRequestCurrent(
	t *Tab,
	gen uint64,
	docID uint32,
	version uint64,
	caret int,
) bool {
	return t != nil && t.doc != nil && t.state != nil && t.state.Buffer != nil &&
		m.tabOpen(t) && t.typeInfoGen == gen && t.doc.ID() == docID &&
		t.state.Buffer.Version() == version && t.state.Cursor() == caret
}

func hoverTypeInfo(hover *openidea.Hover) (string, bool) {
	if hover == nil {
		return "", false
	}
	markup := hover.Contents
	if markup.Kind != "markdown" {
		for _, line := range strings.Split(markup.Value, "\n") {
			if text := strings.TrimSpace(line); text != "" {
				return typeFromSignature(text), true
			}
		}
		return "", false
	}
	var blocks []string
	for _, match := range typeInfoFence.FindAllStringSubmatch(markup.Value, -1) {
		if code := strings.TrimSpace(match[2]); code != "" {
			blocks = append(blocks, code)
		}
	}
	for _, block := range blocks {
		if !typeInfoPath.MatchString(block) {
			return typeFromSignature(block), true
		}
	}
	if len(blocks) > 0 {
		return typeFromSignature(blocks[0]), true
	}
	if match := typeInfoInline.FindStringSubmatch(markup.Value); len(match) == 2 {
		return typeFromSignature(match[1]), true
	}
	return "", false
}

func typeFromSignature(signature string) string {
	text := strings.Join(strings.Fields(signature), " ")
	if match := typeInfoColon.FindStringSubmatch(text); len(match) == 2 {
		if before, _, ok := strings.Cut(match[1], " = "); ok {
			return strings.TrimSpace(before)
		}
		return strings.TrimSpace(match[1])
	}
	if match := typeInfoGo.FindStringSubmatch(text); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return text
}
