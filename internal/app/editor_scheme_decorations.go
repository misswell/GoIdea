package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	maxSemanticTokens  = 50_000
	maxRainbowBrackets = 50_000
)

type semanticTokenSpan struct {
	Start, End int
	Name       string
}

type semanticTokensPayload struct {
	Data           []int    `json:"data"`
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

type rainbowMark struct {
	Start int
	Depth int
}

type rainbowStackEntry struct {
	depth  int
	closer byte
	mark   int
}

type bracketCommentSyntax struct {
	line       string
	blockOpen  string
	blockClose string
}

var rainbowBracketColors = [3][2]ui.Color{
	{ui.Hex("#e8c25b"), ui.Hex("#0431fa")},
	{ui.Hex("#d78bd6"), ui.Hex("#319331")},
	{ui.Hex("#4ea7f0"), ui.Hex("#7b3814")},
}

var semanticRainbowColors = [5][2]ui.Color{
	{ui.Hex("#6fafbd"), ui.Hex("#9b3b6a")},
	{ui.Hex("#d79b6e"), ui.Hex("#114d77")},
	{ui.Hex("#9fbf6b"), ui.Hex("#9c6a2c")},
	{ui.Hex("#c98bc6"), ui.Hex("#005910")},
	{ui.Hex("#8e9cf0"), ui.Hex("#bc5150")},
}

func (m *Model) refreshEditorSchemeBackgrounds() {
	theme := m.theme
	if theme == nil {
		theme = openideaTheme(false)
	}
	for _, tab := range m.allTabs() {
		if tab == nil || tab.doc == nil || tab.state == nil {
			continue
		}
		tab.state.TextBackgrounds = m.composeTextBackgrounds(tab, m.todoMarksOf(tab, theme))
	}
}

func (m *Model) rainbowBracketBackgrounds(tab *Tab) []editor.TextBackground {
	if m == nil || tab == nil || tab.doc == nil || tab.state == nil ||
		!m.prefs.EditorScheme.RainbowBrackets {
		return nil
	}
	text := tab.doc.Buffer().Snapshot().String()
	language := strings.ToLower(strings.TrimSpace(tab.doc.Info().Language))
	if language == "" {
		if known := editor.LanguageFor(tab.doc.Path()); known != nil {
			language = strings.ToLower(strings.TrimSpace(known.Name))
		}
	}
	marks := scanRainbowBrackets(text, bracketCommentsFor(language, tab.doc.Path()))
	out := make([]editor.TextBackground, 0, len(marks))
	theme := m.theme
	if theme == nil {
		theme = openideaTheme(false)
	}
	for _, mark := range marks {
		color := oiPaletteOf(theme).danger
		if mark.Depth >= 0 {
			palette := rainbowBracketColors[mark.Depth%len(rainbowBracketColors)]
			if theme.Dark {
				color = palette[0]
			} else {
				color = palette[1]
			}
		}
		out = append(out, editor.TextBackground{
			Start: mark.Start, End: mark.Start + 1, Color: color.Alpha(0.32),
		})
	}
	return out
}

func bracketCommentsFor(language, path string) bracketCommentSyntax {
	switch language {
	case "python", "ruby", "shell", "bash", "zsh", "fish", "yaml", "toml", "ini":
		return bracketCommentSyntax{line: "#"}
	case "sql", "lua":
		return bracketCommentSyntax{line: "--", blockOpen: "/*", blockClose: "*/"}
	case "html", "xml":
		return bracketCommentSyntax{blockOpen: "<!--", blockClose: "-->"}
	case "c", "cpp", "java", "kotlin", "scala", "swift", "objective-c", "objective-cpp",
		"go", "rust", "javascript", "typescript", "json", "jsonc", "csharp", "php":
		return bracketCommentSyntax{line: "//", blockOpen: "/*", blockClose: "*/"}
	default:
		if strings.EqualFold(pathExt(path), ".properties") {
			return bracketCommentSyntax{line: "#"}
		}
		return bracketCommentSyntax{line: "//", blockOpen: "/*", blockClose: "*/"}
	}
}

func pathExt(path string) string {
	index := strings.LastIndexByte(path, '.')
	if index < 0 {
		return ""
	}
	return strings.ToLower(path[index:])
}

func scanRainbowBrackets(text string, comments bracketCommentSyntax) []rainbowMark {
	if text == "" {
		return nil
	}
	var stack []rainbowStackEntry
	marks := make([]rainbowMark, 0, min(len(text), maxRainbowBrackets))
	mode := byte(0) // 0 code, 1 line comment, 2 block comment, 3 string
	quote := byte(0)
	escaped := false
	for index := 0; index < len(text); index++ {
		ch := text[index]
		switch mode {
		case 1:
			if ch == '\n' {
				mode = 0
			}
			continue
		case 2:
			if comments.blockClose != "" && strings.HasPrefix(text[index:], comments.blockClose) {
				index += len(comments.blockClose) - 1
				mode = 0
			}
			continue
		case 3:
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if quote == '`' && ch == '\n' {
				continue
			}
			if ch == quote {
				quote = 0
				mode = 0
			}
			continue
		}
		if comments.line != "" && strings.HasPrefix(text[index:], comments.line) {
			mode = 1
			index += len(comments.line) - 1
			continue
		}
		if comments.blockOpen != "" && strings.HasPrefix(text[index:], comments.blockOpen) {
			mode = 2
			index += len(comments.blockOpen) - 1
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			if ch == '\'' && !quoteStartsString(text, index) {
				continue
			}
			mode = 3
			quote = ch
			continue
		}
		var closer byte
		if len(marks) >= maxRainbowBrackets {
			break
		}
		switch ch {
		case '(':
			closer = ')'
		case '[':
			closer = ']'
		case '{':
			closer = '}'
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1].closer != ch {
				marks = append(marks, rainbowMark{Start: index, Depth: -1})
				continue
			}
			entry := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			marks = append(marks, rainbowMark{Start: index, Depth: entry.depth})
			continue
		default:
			continue
		}
		depth := len(stack)
		mark := len(marks)
		marks = append(marks, rainbowMark{Start: index, Depth: depth})
		stack = append(stack, rainbowStackEntry{
			depth: depth, closer: closer, mark: mark,
		})
	}
	for _, entry := range stack {
		marks[entry.mark].Depth = -1
	}
	sortRainbowMarks(marks)
	return marks
}

func sortRainbowMarks(marks []rainbowMark) {
	sort.Slice(marks, func(i, j int) bool { return marks[i].Start < marks[j].Start })
}

func isLetterOrDigit(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_'
}

func quoteStartsString(text string, start int) bool {
	if start < 0 || start >= len(text) || text[start] != '\'' {
		return false
	}
	if start > 0 && isLetterOrDigit(text[start-1]) {
		return false
	}
	lineEnd := strings.IndexByte(text[start+1:], '\n')
	if lineEnd >= 0 {
		lineEnd += start + 1
	}
	close := strings.IndexByte(text[start+1:], '\'')
	if close < 0 {
		return false
	}
	close += start + 1
	return lineEnd < 0 || close < lineEnd
}

func (m *Model) semanticTokenBackgrounds(tab *Tab) []editor.TextBackground {
	if m == nil || tab == nil || tab.doc == nil || tab.state == nil ||
		!m.prefs.EditorScheme.SemanticHighlighting || tab.doc.ID() == 0 {
		return nil
	}
	version := tab.doc.Buffer().Version()
	if !tab.semanticLoaded || tab.semanticVersion != version {
		m.requestSemanticTokens(tab, version)
		return nil
	}
	theme := m.theme
	if theme == nil {
		theme = openideaTheme(false)
	}
	out := make([]editor.TextBackground, 0, len(tab.semanticRanges))
	for _, span := range tab.semanticRanges {
		palette := semanticRainbowColors[rainbowIndex(span.Name)%len(semanticRainbowColors)]
		color := palette[0]
		if !theme.Dark {
			color = palette[1]
		}
		out = append(out, editor.TextBackground{
			Start: span.Start, End: span.End, Color: color.Alpha(0.20),
		})
	}
	return out
}

func (m *Model) requestSemanticTokens(tab *Tab, version uint64) {
	if tab.semanticLoading || time.Now().Before(tab.semanticRetryAt) ||
		(m.mgr == nil && m.invokeHook == nil) {
		return
	}
	doc := tab.doc
	docID := doc.ID()
	text := doc.Buffer().Snapshot().String()
	tab.semanticLoading = true
	tab.semanticGeneration++
	generation := tab.semanticGeneration
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_semantic_tokens", map[string]any{"doc": docID})
		m.post(func() {
			t := tab
			t.semanticLoading = false
			if !m.tabOpen(t) || t.doc.ID() != docID ||
				t.semanticGeneration != generation ||
				t.doc.Buffer().Version() != version {
				return
			}
			if err != nil {
				t.semanticRetryAt = time.Now().Add(5 * time.Second)
				m.invalidate()
				return
			}
			var tokens *semanticTokensPayload
			if len(strings.TrimSpace(string(raw))) != 0 && string(raw) != "null" {
				if decodeErr := json.Unmarshal(raw, &tokens); decodeErr != nil {
					t.semanticRetryAt = time.Now().Add(5 * time.Second)
					m.invalidate()
					return
				}
			}
			t.semanticVersion = version
			t.semanticLoaded = true
			t.semanticRanges = semanticTokenSpans(text, tokens)
			t.state.TextBackgrounds = m.composeTextBackgrounds(t, m.todoMarksOf(t, m.theme))
			m.invalidate()
		})
	}()
}

func (m *Model) invalidateSemanticTokens() {
	if m == nil {
		return
	}
	for _, tab := range m.allTabs() {
		if tab == nil || tab.state == nil {
			continue
		}
		tab.semanticGeneration++
		tab.semanticLoading = false
		tab.semanticLoaded = false
		tab.semanticVersion = 0
		tab.semanticRetryAt = time.Time{}
		tab.semanticRanges = nil
		if tab.doc == nil {
			continue
		}
		theme := m.theme
		if theme == nil {
			theme = openideaTheme(false)
		}
		tab.state.TextBackgrounds = m.composeTextBackgrounds(tab, m.todoMarksOf(tab, theme))
	}
	m.invalidate()
}

func semanticTokenSpans(text string, tokens *semanticTokensPayload) []semanticTokenSpan {
	if tokens == nil || len(tokens.Data) == 0 || len(tokens.Data)%5 != 0 {
		return nil
	}
	lineStarts := semanticLineStarts(text)
	out := make([]semanticTokenSpan, 0, min(len(tokens.Data)/5, maxSemanticTokens))
	marksLocals := false
	for _, modifier := range tokens.TokenModifiers {
		if modifier == "local" {
			marksLocals = true
			break
		}
	}
	for index := 0; index+4 < len(tokens.Data) && len(out) < maxSemanticTokens; index += 5 {
		line, column, length := tokens.Data[index], tokens.Data[index+1], tokens.Data[index+2]
		typeIndex, modifierBits := tokens.Data[index+3], tokens.Data[index+4]
		if line < 0 || line >= len(lineStarts) || column < 0 || length <= 0 ||
			typeIndex < 0 || typeIndex >= len(tokens.TokenTypes) {
			continue
		}
		typeName := strings.ToLower(tokens.TokenTypes[typeIndex])
		if typeName != "variable" && typeName != "parameter" {
			continue
		}
		modifiers := semanticModifierNames(modifierBits, tokens.TokenModifiers)
		if semanticConstant(typeName, modifiers) ||
			hasAnyString(modifiers, "static", "constant", "defaultLibrary", "global",
				"classScope", "fileScope", "selfParameter") {
			continue
		}
		if typeName == "variable" && marksLocals && !hasAnyString(modifiers, "local") {
			continue
		}
		start, end := utf16RangeInLines(text, lineStarts, line, column, length)
		if start < 0 || end <= start || end > len(text) || strings.ContainsRune(text[start:end], '\n') {
			continue
		}
		out = append(out, semanticTokenSpan{Start: start, End: end, Name: text[start:end]})
	}
	return out
}

func semanticLineStarts(text string) []int {
	starts := []int{0}
	for index, ch := range text {
		if ch == '\n' {
			starts = append(starts, index+1)
		}
	}
	return starts
}

func utf16RangeInLines(text string, starts []int, line, column, length int) (int, int) {
	if line < 0 || line >= len(starts) || column < 0 || length <= 0 {
		return -1, -1
	}
	lineEnd := len(text)
	if line+1 < len(starts) {
		lineEnd = starts[line+1] - 1
	}
	start, units := starts[line], 0
	for start < lineEnd && units < column {
		r, size := utf8.DecodeRuneInString(text[start:lineEnd])
		step := utf16Units(r)
		if units+step > column {
			break
		}
		start += size
		units += step
	}
	end, units := start, 0
	for end < lineEnd && units < length {
		r, size := utf8.DecodeRuneInString(text[end:lineEnd])
		step := utf16Units(r)
		if units+step > length {
			break
		}
		end += size
		units += step
	}
	return start, end
}

func utf16Units(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

func semanticModifierNames(bits int, modifiers []string) []string {
	out := make([]string, 0, len(modifiers))
	for index, name := range modifiers {
		if index >= 31 {
			break
		}
		if bits&(1<<index) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func semanticConstant(typeName string, modifiers []string) bool {
	return hasAnyString(modifiers, "constant") ||
		(typeName == "variable" && hasAnyString(modifiers, "readonly") &&
			hasAnyString(modifiers, "static"))
}

func hasAnyString(values []string, targets ...string) bool {
	for _, value := range values {
		for _, target := range targets {
			if value == target {
				return true
			}
		}
	}
	return false
}

func rainbowIndex(name string) int {
	var hash uint32 = 0x811c9dc5
	for _, r := range name {
		units := []uint32{uint32(r)}
		if r >= 0x10000 {
			value := uint32(r - 0x10000)
			units = []uint32{0xd800 + value>>10, 0xdc00 + value&0x3ff}
		}
		for _, unit := range units {
			hash ^= unit
			hash *= 0x01000193
		}
	}
	return int(hash % 5)
}
