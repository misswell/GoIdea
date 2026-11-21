package app

import (
	"strings"
	"unicode"

	"mayoi/internal/openidea"
)

// documentationCommentAt returns the doc comment containing offset. Block
// comments and consecutive line comments are supported. The returned text
// includes the comment markers so extraction and cleaning stay separate.
func documentationCommentAt(text string, offset int) (string, bool) {
	if len(text) == 0 {
		return "", false
	}
	offset = min(max(offset, 0), len(text))

	for _, opener := range []string{"/**", "/*!"} {
		open := strings.LastIndex(text[:offset], opener)
		if open < 0 {
			continue
		}
		if strings.Contains(text[open+len(opener):offset], "*/") {
			continue
		}
		closeAt := strings.Index(text[offset:], "*/")
		if closeAt < 0 {
			return "", false
		}
		return text[open : offset+closeAt+2], true
	}

	lineStart := strings.LastIndex(text[:offset], "\n") + 1
	lineEnd := lineStart + strings.IndexByte(text[lineStart:], '\n')
	if lineEnd < lineStart {
		lineEnd = len(text)
	}
	line := strings.TrimSpace(text[lineStart:lineEnd])
	prefix, ok := documentationLinePrefix(line)
	if !ok {
		return "", false
	}
	start := lineStart
	for start > 0 {
		previousEnd := start - 1
		previousStart := strings.LastIndex(text[:previousEnd], "\n") + 1
		previous := strings.TrimSpace(text[previousStart:previousEnd])
		if _, ok := documentationLinePrefix(previous); !ok {
			break
		}
		start = previousStart
	}
	end := lineEnd
	for end < len(text) && text[end] == '\n' {
		nextStart := end + 1
		if nextStart >= len(text) {
			break
		}
		nextEnd := strings.IndexByte(text[nextStart:], '\n')
		if nextEnd < 0 {
			nextEnd = len(text)
		} else {
			nextEnd += nextStart
		}
		next := strings.TrimSpace(text[nextStart:nextEnd])
		if !strings.HasPrefix(next, prefix) {
			break
		}
		end = nextEnd
	}
	return text[start:end], true
}

func documentationLinePrefix(line string) (string, bool) {
	for _, prefix := range []string{"///", "//!"} {
		if strings.HasPrefix(line, prefix) {
			return prefix, true
		}
	}
	// Go and JVM projects conventionally use plain // for declarations.
	if strings.HasPrefix(line, "//") {
		return "//", true
	}
	return "", false
}

// cleanDocumentationComment removes comment markers and the per-line star and
// indentation Javadoc-style comments use. The remaining text is Markdown for
// the native documentation renderer.
func cleanDocumentationComment(raw string) string {
	raw = strings.TrimSpace(raw)
	block := strings.HasPrefix(raw, "/**") || strings.HasPrefix(raw, "/*!")
	if block {
		raw = strings.TrimPrefix(strings.TrimPrefix(raw, "/**"), "/*!")
		raw = strings.TrimSuffix(raw, "*/")
	}
	lines := strings.Split(raw, "\n")
	if block {
		baseIndent := -1
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				lines[i] = ""
				continue
			}
			if strings.HasPrefix(line, "*") {
				line = strings.TrimPrefix(line, "*")
				line = strings.TrimPrefix(line, " ")
			}
			indent := len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
			if baseIndent < 0 || indent < baseIndent {
				baseIndent = indent
			}
			lines[i] = line
		}
		if baseIndent > 0 {
			for i, line := range lines {
				if len(line) >= baseIndent {
					lines[i] = line[baseIndent:]
				}
			}
		}
	} else {
		for i, line := range lines {
			line = strings.TrimLeftFunc(line, unicode.IsSpace)
			line = strings.TrimPrefix(line, "///")
			line = strings.TrimPrefix(line, "//!")
			line = strings.TrimPrefix(line, "//")
			lines[i] = strings.TrimPrefix(line, " ")
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (m *Model) showRenderedDocumentationComment(t *Tab) bool {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return false
	}
	text := t.state.Buffer.Snapshot().String()
	raw, ok := documentationCommentAt(text, t.state.Cursor())
	if !ok {
		return false
	}
	comment := cleanDocumentationComment(raw)
	if comment == "" {
		return false
	}
	language := ""
	if t.doc != nil {
		language = t.doc.Info().Language
	}
	blocks := documentationBlocks(openidea.Markup{Kind: "markdown", Value: comment}, language)
	if len(blocks) == 0 {
		return false
	}
	m.finishDocumentationRequest()
	m.documentation.popup = &documentationView{
		entries: []documentationEntry{{blocks: blocks}},
	}
	m.documentation.tab = t
	m.documentation.offset = t.state.Cursor()
	m.documentation.version = t.state.Buffer.Version()
	m.documentation.anchor = m.usagePopupAnchor
	return true
}
