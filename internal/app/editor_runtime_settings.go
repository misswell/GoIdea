package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func (m *Model) gutterIconsShown() bool {
	return m.nativeRuntimeBool(settingsPageGutterIcons, "editor.showGutterIcons")
}

func (m *Model) gutterGroupEnabled(group string) bool {
	if !m.gutterIconsShown() {
		return false
	}
	for _, disabled := range m.nativeRuntimeStringList(settingsPageGutterIcons, "editor.gutterIconsDisabled") {
		if strings.EqualFold(strings.TrimSpace(disabled), group) {
			return false
		}
	}
	return true
}

func (m *Model) editorSmartBool(key string) bool {
	return m.nativeRuntimeBool(settingsPageSmartKeys, "editor.smartKeys."+key)
}

func (m *Model) editorSmartString(key string) string {
	return m.nativeRuntimeString(settingsPageSmartKeys, "editor.smartKeys."+key)
}

func (m *Model) autoImportSetOnPaste() string {
	return m.nativeRuntimeString(settingsPageAutoImport, "editor.autoImport.importsOnPaste")
}

func (m *Model) autoImportAddUnambiguous() bool {
	return m.nativeRuntimeBool(settingsPageAutoImport, "editor.autoImport.addUnambiguous")
}

func (m *Model) autoImportOptimizeOnTheFly() bool {
	return m.nativeRuntimeBool(settingsPageAutoImport, "editor.autoImport.optimizeOnTheFly")
}

func (m *Model) editorCodeEditingBool(key string) bool {
	return m.nativeRuntimeBool(settingsPageCodeEditing, "editor.codeEditing."+key)
}

func (m *Model) editorCodeEditingNumber(key string) float64 {
	return m.nativeRuntimeNumber(settingsPageCodeEditing, "editor.codeEditing."+key)
}

func (m *Model) editorTextBackgrounds(t *Tab) []editor.TextBackground {
	if t == nil || t.doc == nil || t.state == nil {
		return nil
	}
	out := append([]editor.TextBackground(nil), m.selectionOccurrenceBackgrounds(t)...)
	oi := oiPaletteOf(m.theme)
	if m.editorCodeEditingBool("highlightBraces") {
		if start, end, ok := editorBracePairAt(t); ok {
			out = append(out, editor.TextBackground{
				Start: start,
				End:   end,
				Color: oi.accent.Alpha(0.16),
			})
		}
	}
	if m.editorCodeEditingBool("highlightUsages") && !m.usageHighlightSessionValid(t) {
		out = append(out, m.caretWordUsageBackgrounds(t)...)
	}
	return out
}

func (m *Model) selectionOccurrenceBackgrounds(t *Tab) []editor.TextBackground {
	if t == nil || t.state == nil || t.doc == nil ||
		!m.editorCodeEditingBool("highlightSelection") {
		return nil
	}
	selections := t.state.Selections()
	if len(selections) != 1 {
		return nil
	}
	start, end := selections[0].Anchor, selections[0].Head
	if start > end {
		start, end = end, start
	}
	text := t.doc.Text()
	if start < 0 || start >= end || end > len(text) || end-start > 1024 {
		return nil
	}
	needle := text[start:end]
	if strings.ContainsAny(needle, "\r\n") {
		return nil
	}
	oi := oiPaletteOf(m.theme)
	backgrounds := make([]editor.TextBackground, 0, 8)
	for offset := 0; offset <= len(text)-len(needle) && len(backgrounds) < 256; {
		found := strings.Index(text[offset:], needle)
		if found < 0 {
			break
		}
		found += offset
		if found != start {
			backgrounds = append(backgrounds, editor.TextBackground{
				Start: found,
				End:   found + len(needle),
				Color: oi.accent.Alpha(0.12),
			})
		}
		offset = found + max(1, len(needle))
	}
	return backgrounds
}

func (m *Model) caretWordUsageBackgrounds(t *Tab) []editor.TextBackground {
	if t == nil || t.state == nil || t.doc == nil ||
		len(t.state.Selections()) != 1 {
		return nil
	}
	text := t.doc.Text()
	cursor := t.state.Cursor()
	start, end := templateWordAt(text, cursor)
	if start < 0 || end <= start || end-start > 256 {
		return nil
	}
	word := text[start:end]
	if !templateWordIsIdentifier(word) {
		return nil
	}
	oi := oiPaletteOf(m.theme)
	backgrounds := make([]editor.TextBackground, 0, 8)
	for offset := 0; offset <= len(text)-len(word) && len(backgrounds) < 256; {
		found := strings.Index(text[offset:], word)
		if found < 0 {
			break
		}
		found += offset
		wordEnd := found + len(word)
		leftBoundary := found == 0 || !isTemplateIdentifierByte(text[found-1])
		rightBoundary := wordEnd == len(text) || !isTemplateIdentifierByte(text[wordEnd])
		if found != start && leftBoundary && rightBoundary {
			backgrounds = append(backgrounds, editor.TextBackground{
				Start: found,
				End:   wordEnd,
				Color: oi.accent.Alpha(0.10),
			})
		}
		offset = wordEnd
	}
	return backgrounds
}

func templateWordAt(text string, offset int) (int, int) {
	if offset < 0 || offset > len(text) {
		return -1, -1
	}
	start := offset
	if start == len(text) || start > 0 && !isTemplateIdentifierByte(text[start]) {
		start--
	}
	if start < 0 || !isTemplateIdentifierByte(text[start]) {
		return -1, -1
	}
	end := start
	for start > 0 && isTemplateIdentifierByte(text[start-1]) {
		start--
	}
	for end < len(text) && isTemplateIdentifierByte(text[end]) {
		end++
	}
	return start, end
}

func templateWordIsIdentifier(word string) bool {
	if word == "" {
		return false
	}
	for i := 0; i < len(word); i++ {
		if !isTemplateIdentifierByte(word[i]) {
			return false
		}
	}
	return word[0] < '0' || word[0] > '9'
}

func editorBracePairAt(t *Tab) (int, int, bool) {
	if t == nil || t.state == nil || t.doc == nil {
		return 0, 0, false
	}
	text := t.doc.Text()
	cursor := t.state.Cursor()
	if cursor < 0 || cursor > len(text) {
		return 0, 0, false
	}
	at := cursor
	if at == len(text) || !editorIsBracketByte(text[at]) {
		at--
	}
	if at < 0 || !editorIsBracketByte(text[at]) {
		return 0, 0, false
	}
	open, close := text[at], byte(0)
	forward := true
	switch open {
	case '(':
		close, forward = ')', true
	case '[':
		close, forward = ']', true
	case '{':
		close, forward = '}', true
	case ')':
		open, close, forward = '(', ')', false
	case ']':
		open, close, forward = '[', ']', false
	case '}':
		open, close, forward = '{', '}', false
	default:
		return 0, 0, false
	}
	depth := 0
	if forward {
		for i := at; i < len(text); i++ {
			switch text[i] {
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return min(at, i), max(at, i) + 1, true
				}
			}
		}
		return 0, 0, false
	}
	for i := at; i >= 0; i-- {
		switch text[i] {
		case close:
			depth++
		case open:
			depth--
			if depth == 0 {
				return min(at, i), max(at, i) + 1, true
			}
		}
	}
	return 0, 0, false
}

func editorIsBracketByte(value byte) bool {
	return strings.ContainsRune("()[]{}", rune(value))
}

// editorPaste applies Smart Keys' string escaping and paste indentation in the
// native editor. It consumes the paste only when the text changes; ordinary
// pastes keep MyGo's caret, selection and undo behavior.
func (m *Model) editorPaste(t *Tab, text string) bool {
	if t == nil || t.doc == nil || t.state == nil || text == "" ||
		t.state.ReadOnly || t.doc.ReadOnly() {
		return false
	}
	selections := t.state.Selections()
	if len(selections) == 0 {
		return false
	}
	formatted := text
	if len(selections) == 1 {
		formatted = m.formatEditorPaste(t, text, selections[0])
	}
	if formatted == text && (m.autoImportSetOnPaste() == "never" || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil)) {
		return false
	}
	before := t.doc.Text()
	t.state.Insert(formatted)
	ranges := pastedSelectionRanges(before, t.doc.Text(), selections, t.state.Selections())
	m.markAutoImportDirty(t)
	m.beginAutoImportPaste(t, ranges)
	m.invalidate()
	return true
}

func (m *Model) formatEditorPaste(t *Tab, text string, selection editor.Selection) string {
	source := t.doc.Text()
	language := strings.ToLower(strings.TrimSpace(t.doc.Info().Language))
	if language == "" {
		if known := editor.LanguageFor(t.doc.Path()); known != nil {
			language = strings.ToLower(strings.TrimSpace(known.Name))
		}
	}
	if m.editorSmartBool("escapeOnPaste") {
		start, end := selection.Anchor, selection.Head
		if start > end {
			start, end = end, start
		}
		if quote, ok := plainPasteStringAt(source, start, end, language); ok {
			text = escapePasteString(text, quote, language)
		}
	}
	mode := strings.TrimSpace(m.editorSmartString("reformatOnPaste"))
	if mode != "indentBlock" && mode != "indentEachLine" {
		return text
	}
	if !strings.Contains(text, "\n") || strings.TrimSpace(text) == "" {
		return text
	}
	start := min(selection.Anchor, selection.Head)
	if start < 0 {
		start = 0
	}
	start = min(start, len(source))
	lineStart := strings.LastIndexByte(source[:start], '\n') + 1
	lineEnd := strings.IndexByte(source[start:], '\n')
	if lineEnd < 0 {
		lineEnd = len(source)
	} else {
		lineEnd += start
	}
	before := source[lineStart:start]
	atIndent := strings.TrimSpace(before) == ""
	targetText := before
	if !atIndent {
		targetText = source[lineStart:lineEnd]
	}
	tabSize := t.state.TabSize
	if tabSize <= 0 {
		tabSize = 4
	}
	targetCol := pasteLeadingColumns(targetText, tabSize)
	unit := "  "
	if language == "go" {
		unit = "\t"
	}
	if mode == "indentBlock" {
		return indentPastedBlock(text, targetCol, unit, tabSize)
	}
	return indentPastedLines(text, targetCol, unit, tabSize, language)
}

var pasteStringLanguages = map[string]bool{
	"java": true, "kotlin": true, "javascript": true, "typescript": true,
	"python": true, "rust": true, "go": true, "c": true, "cpp": true,
}

// plainPasteStringAt recognizes the plain single-line string literal around a
// selection. It intentionally rejects triple strings and interpolation-heavy
// forms so escaping only happens where the source language has one meaning for
// the inserted text.
func plainPasteStringAt(source string, start, end int, language string) (byte, bool) {
	if !pasteStringLanguages[language] || start < 0 || end < start || end > len(source) {
		return 0, false
	}
	lineStart := strings.LastIndexByte(source[:start], '\n') + 1
	lineEnd := strings.IndexByte(source[end:], '\n')
	if lineEnd < 0 {
		lineEnd = len(source)
	} else {
		lineEnd += end
	}
	line := source[lineStart:lineEnd]
	localStart, localEnd := start-lineStart, end-lineStart
	allowed := func(i int) bool {
		return line[i] == '"' || (line[i] == '\'' &&
			(language == "javascript" || language == "typescript" || language == "python"))
	}
	unescaped := func(i int) bool {
		backslashes := 0
		for i--; i >= 0 && line[i] == '\\'; i-- {
			backslashes++
		}
		return backslashes%2 == 0
	}
	for open := localStart - 1; open >= 0; open-- {
		if !allowed(open) || !unescaped(open) {
			continue
		}
		quote := line[open]
		if strings.HasPrefix(line[open:], strings.Repeat(string(quote), 3)) {
			return 0, false
		}
		for close := open + 1; close < len(line); close++ {
			if line[close] != quote || !unescaped(close) {
				continue
			}
			if localStart > open && localEnd <= close {
				return quote, true
			}
			return 0, false
		}
		return 0, false
	}
	return 0, false
}

func escapePasteString(text string, quote byte, language string) string {
	var out strings.Builder
	for _, r := range text {
		switch {
		case r == '\\':
			out.WriteString(`\\`)
		case r == rune(quote):
			out.WriteByte('\\')
			out.WriteRune(r)
		case r == '$' && language == "kotlin":
			out.WriteString(`\$`)
		case r == '\n':
			out.WriteString(`\n`)
		case r == '\r':
			out.WriteString(`\r`)
		case r == '\t':
			out.WriteString(`\t`)
		case r < 0x20:
			if language == "rust" {
				fmt.Fprintf(&out, `\u{%x}`, r)
			} else if language == "c" || language == "cpp" {
				fmt.Fprintf(&out, `\%03o`, r)
			} else {
				fmt.Fprintf(&out, `\u%04x`, r)
			}
		case (language == "javascript" || language == "typescript") &&
			(r == '\u2028' || r == '\u2029'):
			fmt.Fprintf(&out, `\u%04x`, r)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func pasteLeadingColumns(text string, tabSize int) int {
	col := 0
	for _, r := range text {
		switch r {
		case ' ':
			col++
		case '\t':
			col += tabSize - col%tabSize
		default:
			return col
		}
	}
	return col
}

func pasteIndentString(cols, tabSize int, unit string) string {
	if cols <= 0 {
		return ""
	}
	if unit == "\t" {
		return strings.Repeat("\t", cols/tabSize) + strings.Repeat(" ", cols%tabSize)
	}
	return strings.Repeat(" ", cols)
}

// indentPastedBlock ports OpenIDEA's Indent Block calculation: preserve the
// copied block's relative indentation while moving its base to the paste
// column.
func indentPastedBlock(text string, targetCol int, unit string, tabSize int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return text
	}
	var rest []string
	base := int(^uint(0) >> 1)
	firstIndented := len(lines[0]) > 0 && (lines[0][0] == ' ' || lines[0][0] == '\t')
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rest = append(rest, line)
		base = min(base, pasteLeadingColumns(line, tabSize))
	}
	if firstIndented {
		base = min(base, pasteLeadingColumns(lines[0], tabSize))
	}
	if len(rest) == 0 || base == int(^uint(0)>>1) {
		return text
	}
	first := strings.TrimRight(lines[0], " \t")
	if !firstIndented && base > 0 && first != "" && strings.ContainsRune(":{([]", rune(first[len(first)-1])) {
		base = max(0, base-tabSize)
	}
	delta := targetCol - base
	for i, line := range lines {
		if i == 0 {
			lines[i] = strings.TrimLeft(line, " \t")
			continue
		}
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		cols := max(0, pasteLeadingColumns(line, tabSize)+delta)
		lines[i] = pasteIndentString(cols, tabSize, unit) + strings.TrimLeft(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// indentPastedLines is OpenIDEA's default Indent Each Line behavior. The native
// language parser is not available synchronously, so this uses the structural
// braces and Python suite endings that determine the common indentation cases.
func indentPastedLines(text string, targetCol int, unit string, tabSize int, language string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return text
	}
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			lines[i] = ""
			continue
		}
		if i == 0 {
			lines[i] = strings.TrimLeft(line, " \t")
		} else {
			if startsClosingDelimiter(trimmed) {
				depth = max(0, depth-1)
			}
			cols := targetCol + depth*tabSize
			lines[i] = pasteIndentString(cols, tabSize, unit) + strings.TrimLeft(line, " \t")
		}
		depth += pasteIndentDelta(trimmed, language)
	}
	return strings.Join(lines, "\n")
}

func startsClosingDelimiter(line string) bool {
	return line != "" && strings.ContainsRune("}])", rune(line[0]))
}

func pasteIndentDelta(line, language string) int {
	delta := 0
	inString := byte(0)
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inString != 0 {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == inString {
				inString = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inString = ch
			continue
		}
		switch ch {
		case '{', '(', '[':
			delta++
		case '}', ')', ']':
			delta--
		}
	}
	if language == "python" && strings.HasSuffix(strings.TrimSpace(line), ":") {
		delta++
	}
	return delta
}

var colorPreviewPattern = regexp.MustCompile(
	`(?i)(?:#[0-9a-f]{8}|#[0-9a-f]{6}|#[0-9a-f]{3,4}|rgba?\(\s*\d{1,3}\s*,\s*\d{1,3}\s*,\s*\d{1,3}(?:\s*,\s*(?:0|1|0?\.\d+))?\s*\))`,
)

// colorPreviewGutterMarks mirrors OpenIDEA's Color preview provider with the
// native editor's one-dot gutter. A line keeps its first color, and existing
// breakpoint, coverage, Git, or plugin marks keep priority.
func (m *Model) colorPreviewGutterMarks(t *Tab, taken map[int]bool) []editor.GutterMark {
	if t == nil || t.doc == nil || !m.gutterGroupEnabled("colors") {
		return nil
	}
	lines := strings.Split(t.doc.Text(), "\n")
	marks := make([]editor.GutterMark, 0)
	for line, text := range lines {
		if taken[line] {
			continue
		}
		match := colorPreviewPattern.FindString(text)
		if match == "" {
			continue
		}
		color, ok := parsedPreviewColor(match)
		if !ok {
			continue
		}
		marks = append(marks, editor.GutterMark{Line: line, Color: color})
	}
	return marks
}

func parsedPreviewColor(value string) (ui.Color, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		hex := strings.TrimPrefix(value, "#")
		switch len(hex) {
		case 3, 4:
			expanded := make([]byte, 0, len(hex)*2)
			for i := range hex {
				expanded = append(expanded, hex[i], hex[i])
			}
			hex = string(expanded)
		case 6, 8:
		default:
			return ui.Color{}, false
		}
		red, err := strconv.ParseUint(hex[0:2], 16, 8)
		if err != nil {
			return ui.Color{}, false
		}
		green, err := strconv.ParseUint(hex[2:4], 16, 8)
		if err != nil {
			return ui.Color{}, false
		}
		blue, err := strconv.ParseUint(hex[4:6], 16, 8)
		if err != nil {
			return ui.Color{}, false
		}
		alpha := uint64(255)
		if len(hex) == 8 {
			alpha, err = strconv.ParseUint(hex[6:8], 16, 8)
			if err != nil {
				return ui.Color{}, false
			}
		}
		return ui.Color{R: uint8(red), G: uint8(green), B: uint8(blue), A: uint8(alpha)}, true
	}
	switch {
	case strings.HasPrefix(value, "rgb("):
		value = strings.TrimSuffix(strings.TrimPrefix(value, "rgb("), ")")
	case strings.HasPrefix(value, "rgba("):
		value = strings.TrimSuffix(strings.TrimPrefix(value, "rgba("), ")")
	default:
		return ui.Color{}, false
	}
	parts := strings.Split(value, ",")
	if len(parts) != 3 && len(parts) != 4 {
		return ui.Color{}, false
	}
	channel := func(text string) (uint8, bool) {
		number, err := strconv.ParseUint(strings.TrimSpace(text), 10, 8)
		return uint8(number), err == nil
	}
	red, ok := channel(parts[0])
	if !ok {
		return ui.Color{}, false
	}
	green, ok := channel(parts[1])
	if !ok {
		return ui.Color{}, false
	}
	blue, ok := channel(parts[2])
	if !ok {
		return ui.Color{}, false
	}
	alpha := float32(1)
	if len(parts) == 4 {
		number, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 32)
		if err != nil {
			return ui.Color{}, false
		}
		alpha = min(float32(1), max(float32(0), float32(number)))
	}
	return ui.RGBA(red, green, blue, alpha), true
}
