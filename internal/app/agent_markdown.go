package app

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	htmlstd "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	mermaidpkg "mayoi/internal/mermaid"
)

type agentMarkdownBlock struct {
	Kind  string
	Text  string
	Lang  string
	Level int
	Rows  [][]string
}

type agentMarkdownRun struct {
	Text   string
	Href   string
	Code   bool
	Bold   bool
	Italic bool
	Strike bool
	// Image is the source of an image run, and Text its description.
	// Title is the tooltip a Markdown image gives.
	Image string
	Title string
}

var agentMarkdownLocalLocation = regexp.MustCompile(`^(.*?)(?::(\d+)(?::(\d+))?|#L(\d+)(?::(\d+))?)$`)
var agentMarkdownInlinePath = regexp.MustCompile(`^(?:\./)?([\w.@-]+(?:/[\w.@-]+)*\.[A-Za-z][A-Za-z0-9]*)(?::(\d+)(?::(\d+))?|#L(\d+)(?::(\d+))?)?$`)

type agentMarkdownImageRef struct {
	URL  string
	Path string
}

type agentMarkdownImageState struct {
	Source  ui.ImageSource
	Loading bool
	Err     string
}

type agentMarkdownDiagramState struct {
	Diagram *mermaidpkg.Diagram
	Loading bool
}

const (
	agentMarkdownImageLimit   = 20 * 1024 * 1024
	agentMarkdownImageCount   = 32
	agentMarkdownDiagramCount = 16
)

var agentMarkdownHTTP = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(request *http.Request, _ []*http.Request) error {
		request.Header.Del("Referer")
		if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
			return fmt.Errorf("unsupported image redirect")
		}
		return nil
	},
}

// agentMarkdownBlocks parses the practical Markdown OpenIDEA renders in an
// agent reply. Unknown inline syntax remains text, so no content is hidden.
func agentMarkdownBlocks(text string) []agentMarkdownBlock {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var blocks []agentMarkdownBlock
	var paragraph []string
	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		blocks = append(blocks, agentMarkdownBlock{
			Kind: "paragraph", Text: strings.Join(paragraph, "\n"),
		})
		paragraph = nil
	}
	for index := 0; index < len(lines); {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			flush()
			fence := trimmed[:3]
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, fence))
			index++
			var code []string
			for index < len(lines) {
				next := strings.TrimSpace(lines[index])
				if strings.HasPrefix(next, fence) {
					index++
					break
				}
				code = append(code, lines[index])
				index++
			}
			blocks = append(blocks, agentMarkdownBlock{
				Kind: "code", Lang: lang, Text: strings.Join(code, "\n"),
			})
			continue
		}
		if trimmed == "" {
			flush()
			index++
			continue
		}
		if level, heading := agentMarkdownHeading(trimmed); heading != "" {
			flush()
			blocks = append(blocks, agentMarkdownBlock{
				Kind: "heading", Level: level, Text: heading,
			})
			index++
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			flush()
			var quote []string
			for index < len(lines) {
				line = strings.TrimSpace(lines[index])
				if !strings.HasPrefix(line, ">") {
					break
				}
				line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
				quote = append(quote, line)
				index++
			}
			blocks = append(blocks, agentMarkdownBlock{
				Kind: "quote", Text: strings.Join(quote, "\n"),
			})
			continue
		}
		if agentMarkdownListItem(trimmed) {
			flush()
			var items []string
			for index < len(lines) {
				item := strings.TrimSpace(lines[index])
				if !agentMarkdownListItem(item) {
					break
				}
				items = append(items, agentMarkdownListText(item))
				index++
			}
			blocks = append(blocks, agentMarkdownBlock{
				Kind: "list", Text: strings.Join(items, "\n"),
			})
			continue
		}
		if index+1 < len(lines) && strings.Contains(line, "|") &&
			agentMarkdownTableRule(lines[index+1]) {
			flush()
			rows := [][]string{agentMarkdownCells(line)}
			index += 2
			for index < len(lines) && strings.Contains(lines[index], "|") &&
				strings.TrimSpace(lines[index]) != "" {
				rows = append(rows, agentMarkdownCells(lines[index]))
				index++
			}
			blocks = append(blocks, agentMarkdownBlock{Kind: "table", Rows: rows})
			continue
		}
		if strings.Trim(trimmed, "-_* ") == "" && len(trimmed) >= 3 {
			flush()
			blocks = append(blocks, agentMarkdownBlock{Kind: "rule"})
			index++
			continue
		}
		paragraph = append(paragraph, line)
		index++
	}
	flush()
	return blocks
}

func agentMarkdownHeading(line string) (int, string) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(line[level:])
}

func agentMarkdownListItem(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") ||
		strings.HasPrefix(line, "+ ") {
		return true
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	return index > 0 && index+1 < len(line) && line[index] == '.' && line[index+1] == ' '
}

func agentMarkdownListText(line string) string {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") ||
		strings.HasPrefix(line, "+ ") {
		return strings.TrimSpace(line[2:])
	}
	if dot := strings.Index(line, ". "); dot > 0 {
		return strings.TrimSpace(line[dot+2:])
	}
	return line
}

func agentMarkdownTableRule(line string) bool {
	cells := agentMarkdownCells(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		cell = strings.TrimPrefix(cell, ":")
		cell = strings.TrimSuffix(cell, ":")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func agentMarkdownCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	return strings.Split(line, "|")
}

func agentMarkdownRuns(text string) []agentMarkdownRun {
	var runs []agentMarkdownRun
	var plain strings.Builder
	flush := func() {
		if plain.Len() == 0 {
			return
		}
		runs = append(runs, agentMarkdownRun{Text: plain.String()})
		plain.Reset()
	}
	for index := 0; index < len(text); {
		if text[index] == '\\' && index+1 < len(text) {
			plain.WriteByte(text[index+1])
			index += 2
			continue
		}
		if text[index] == '<' {
			if run, next, ok := agentMarkdownHTMLAt(text, index); ok {
				flush()
				if run != nil {
					runs = append(runs, *run)
				}
				index = next
				continue
			}
		}
		if text[index] == '!' {
			if image, next, ok := agentMarkdownImageAt(text, index); ok {
				flush()
				runs = append(runs, image)
				index = next
				continue
			}
		}
		if text[index] == '`' {
			if end := strings.IndexByte(text[index+1:], '`'); end >= 0 {
				flush()
				end += index + 1
				runs = append(runs, agentMarkdownRun{Text: text[index+1 : end], Code: true})
				index = end + 1
				continue
			}
		}
		if text[index] == '[' {
			if close := strings.Index(text[index+1:], "]("); close >= 0 {
				close += index + 1
				if end := strings.IndexByte(text[close+2:], ')'); end >= 0 {
					end += close + 2
					flush()
					runs = append(runs, agentMarkdownRun{
						Text: text[index+1 : close],
						Href: strings.TrimSpace(text[close+2 : end]),
					})
					index = end + 1
					continue
				}
			}
		}
		if index+1 < len(text) && (text[index:index+2] == "**" ||
			text[index:index+2] == "__") {
			marker := text[index : index+2]
			if end := strings.Index(text[index+2:], marker); end >= 0 {
				flush()
				end += index + 2
				runs = append(runs, agentMarkdownRun{Text: text[index+2 : end], Bold: true})
				index = end + 2
				continue
			}
		}
		if index+1 < len(text) && text[index:index+2] == "~~" {
			if end := strings.Index(text[index+2:], "~~"); end >= 0 {
				flush()
				end += index + 2
				runs = append(runs, agentMarkdownRun{Text: text[index+2 : end], Strike: true})
				index = end + 2
				continue
			}
		}
		if text[index] == '*' || text[index] == '_' {
			marker := text[index]
			if end := strings.IndexByte(text[index+1:], marker); end >= 0 {
				end += index + 1
				if end > index+1 {
					flush()
					runs = append(runs, agentMarkdownRun{Text: text[index+1 : end], Italic: true})
					index = end + 1
					continue
				}
			}
		}
		plain.WriteByte(text[index])
		index++
	}
	flush()
	return runs
}

func agentMarkdownHTMLAt(text string, start int) (*agentMarkdownRun, int, bool) {
	if strings.HasPrefix(text[start:], "<!--") {
		if end := strings.Index(text[start+4:], "-->"); end >= 0 {
			return nil, start + 4 + end + 3, true
		}
		return nil, start, false
	}
	end := strings.IndexByte(text[start+1:], '>')
	if end < 0 {
		return nil, start, false
	}
	end += start + 1
	tag := strings.TrimSpace(text[start+1 : end])
	tag = strings.TrimPrefix(tag, "/")
	if tag == "" {
		return nil, start, false
	}
	nameEnd := 0
	for nameEnd < len(tag) && (tag[nameEnd] >= 'a' && tag[nameEnd] <= 'z' ||
		tag[nameEnd] >= 'A' && tag[nameEnd] <= 'Z' ||
		tag[nameEnd] >= '0' && tag[nameEnd] <= '9' || tag[nameEnd] == '-') {
		nameEnd++
	}
	if nameEnd == 0 {
		return nil, start, false
	}
	name := strings.ToLower(tag[:nameEnd])
	if name == "script" || name == "style" {
		closing := "</" + name + ">"
		if closeAt := strings.Index(strings.ToLower(text[end+1:]), closing); closeAt >= 0 {
			return nil, end + 1 + closeAt + len(closing), true
		}
		return nil, len(text), true
	}
	if name != "img" {
		return nil, end + 1, true
	}
	attrs := agentMarkdownHTMLAttrs(tag[nameEnd:])
	alt := strings.TrimSpace(attrs["alt"])
	source := attrs["src"]
	return &agentMarkdownRun{Text: alt, Image: source, Title: attrs["title"]}, end + 1, true
}

func agentMarkdownHTMLAttrs(text string) map[string]string {
	attrs := make(map[string]string)
	for index := 0; index < len(text); {
		for index < len(text) && (text[index] == ' ' || text[index] == '\t' ||
			text[index] == '\r' || text[index] == '\n' || text[index] == '/') {
			index++
		}
		start := index
		for index < len(text) && !strings.ContainsRune(" \t\r\n=/", rune(text[index])) {
			index++
		}
		if start == index {
			break
		}
		name := strings.ToLower(text[start:index])
		for index < len(text) && (text[index] == ' ' || text[index] == '\t' ||
			text[index] == '\r' || text[index] == '\n') {
			index++
		}
		if index >= len(text) || text[index] != '=' {
			attrs[name] = ""
			continue
		}
		index++
		for index < len(text) && (text[index] == ' ' || text[index] == '\t' ||
			text[index] == '\r' || text[index] == '\n') {
			index++
		}
		if index >= len(text) {
			break
		}
		var value string
		if text[index] == '"' || text[index] == '\'' {
			quote := text[index]
			index++
			start = index
			for index < len(text) && text[index] != quote {
				index++
			}
			value = text[start:index]
			if index < len(text) {
				index++
			}
		} else {
			start = index
			for index < len(text) && !strings.ContainsRune(" \t\r\n", rune(text[index])) {
				index++
			}
			value = text[start:index]
		}
		attrs[name] = htmlstd.UnescapeString(value)
	}
	return attrs
}

func agentMarkdownImageAt(text string, start int) (agentMarkdownRun, int, bool) {
	if start+1 >= len(text) || text[start] != '!' || text[start+1] != '[' {
		return agentMarkdownRun{}, start, false
	}
	altEnd := -1
	for index := start + 2; index < len(text); index++ {
		if text[index] == '\\' {
			index++
			continue
		}
		if text[index] == ']' {
			altEnd = index
			break
		}
	}
	if altEnd < 0 || altEnd+1 >= len(text) || text[altEnd+1] != '(' {
		return agentMarkdownRun{}, start, false
	}
	depth := 1
	end := -1
	for index := altEnd + 2; index < len(text); index++ {
		switch text[index] {
		case '\\':
			index++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = index
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return agentMarkdownRun{}, start, false
	}
	dest, title := agentMarkdownDestination(text[altEnd+2 : end])
	if dest == "" {
		return agentMarkdownRun{}, start, false
	}
	return agentMarkdownRun{
		Text:  agentMarkdownUnescape(text[start+2 : altEnd]),
		Image: dest,
		Title: title,
	}, end + 1, true
}

func agentMarkdownDestination(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if raw[0] == '<' {
		if end := strings.IndexByte(raw, '>'); end > 0 {
			return agentMarkdownUnescape(strings.TrimSpace(raw[1:end])),
				agentMarkdownUnescape(agentMarkdownTitle(strings.TrimSpace(raw[end+1:])))
		}
	}
	split := len(raw)
	for index := 0; index < len(raw); index++ {
		if raw[index] == '\\' {
			index++
			continue
		}
		if raw[index] == ' ' || raw[index] == '\t' || raw[index] == '\n' {
			split = index
			break
		}
	}
	return agentMarkdownUnescape(strings.TrimSpace(raw[:split])),
		agentMarkdownUnescape(agentMarkdownTitle(strings.TrimSpace(raw[split:])))
}

func agentMarkdownTitle(raw string) string {
	if len(raw) >= 2 {
		open := raw[0]
		close := open
		if open == '(' {
			close = ')'
		}
		if (open == '"' || open == '\'' || open == '(') && raw[len(raw)-1] == close {
			return raw[1 : len(raw)-1]
		}
	}
	return ""
}

func agentMarkdownUnescape(text string) string {
	var out strings.Builder
	for index := 0; index < len(text); index++ {
		if text[index] == '\\' && index+1 < len(text) {
			index++
		}
		out.WriteByte(text[index])
	}
	return out.String()
}

func (m *Model) agentMarkdown(c *ui.Context, p *ui.Theme, oi oiColors,
	text, cwd string, find *regexp.Regexp) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if find != nil {
		agentSearchTextBlock(c, p, oi, text, oi.text, 0, find)
		return
	}
	for _, block := range agentMarkdownBlocks(text) {
		switch block.Kind {
		case "heading":
			size := float32(14)
			switch block.Level {
			case 1:
				size = 18
			case 2:
				size = 16
			case 3:
				size = 14
			default:
				size = 12
			}
			m.agentMarkdownParagraph(c, p, oi, block.Text, cwd, size, true)
		case "code":
			m.agentMarkdownCode(c, p, oi, block)
		case "quote":
			ui.Row(c).Gap(6).Children(func() {
				ui.Column(c).Width(2).Background(oi.border)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					m.agentMarkdownParagraph(c, p, oi, block.Text, cwd, 12, false)
				})
			})
		case "list":
			for _, item := range strings.Split(block.Text, "\n") {
				ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
					ui.Text(c, "•").Width(9).FontSize(12).TextColor(oi.textMuted)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						m.agentMarkdownParagraph(c, p, oi, item, cwd, 12, false)
					})
				})
			}
		case "table":
			m.agentMarkdownTable(c, p, oi, block, cwd)
		case "rule":
			ui.Column(c).Height(1).Background(oi.border)
		default:
			m.agentMarkdownParagraph(c, p, oi, block.Text, cwd, 12, false)
		}
	}
}

func (m *Model) agentMarkdownParagraph(c *ui.Context, p *ui.Theme, oi oiColors,
	text, cwd string, size float32, bold bool) {
	runs := agentMarkdownRuns(text)
	if !slices.ContainsFunc(runs, func(run agentMarkdownRun) bool { return run.Image != "" }) {
		m.agentMarkdownTextRuns(c, oi, runs, cwd, size, bold)
		return
	}

	ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
		var plain []agentMarkdownRun
		flush := func() {
			if len(plain) == 0 {
				return
			}
			m.agentMarkdownTextRuns(c, oi, plain, cwd, size, bold)
			plain = nil
		}
		for _, run := range runs {
			if run.Image == "" {
				plain = append(plain, run)
				continue
			}
			flush()
			m.agentMarkdownImage(c, oi, run, cwd, size)
		}
		flush()
	})
}

func (m *Model) agentMarkdownTextRuns(c *ui.Context, oi oiColors, runs []agentMarkdownRun,
	cwd string, size float32, bold bool) {
	ui.RichText(c).Grow(1).MinWidth(0).FontSize(size).Children(func() {
		for _, run := range runs {
			run := run
			element := ui.Text(c, run.Text)
			if bold || run.Bold {
				element.Bold()
			}
			if run.Italic {
				element.Italic()
			}
			if run.Strike {
				element.Strikethrough()
			}
			if run.Code {
				element.Font(oiFontMono).Background(oi.hoverTint).Radius(3)
				if path, line, column := agentMarkdownInlineCodeTarget(cwd, run.Text); path != "" {
					element.TextColor(oi.accent).Underline().Cursor(ui.CursorPointer).
						Tooltip("Open " + path).OnClick(func() {
						m.revealAt(path, line, column)
					})
				}
			}
			if run.Href != "" {
				element.TextColor(oi.accent).Underline().Cursor(ui.CursorPointer).
					Tooltip(run.Href).OnClick(func() {
					m.openAgentMarkdownLink(c, cwd, run.Href)
				})
			}
		}
	})
}

func (m *Model) agentMarkdownCode(c *ui.Context, p *ui.Theme, oi oiColors,
	block agentMarkdownBlock) {
	language := strings.ToLower(strings.Fields(block.Lang + " ")[0])
	if language == "mermaid" {
		m.agentMarkdownMermaid(c, p, oi, block)
		return
	}
	ui.Column(c).Gap(3).Padding(5, 6).Radius(5).
		Background(oi.hoverTint).BorderWidth(1, 1, 1, 1).
		BorderColor(oi.border).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			label := block.Lang
			if label == "" {
				label = "Code"
			}
			ui.Text(c, label).Grow(1).FontSize(10).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
			ui.Button(c, "Copy").OnClick(func() { c.WriteClipboard(block.Text) })
		})
		ui.Text(c, block.Text).Font(oiFontMono).FontSize(11).
			TextColor(oi.text).Selectable()
	})
}

func (m *Model) agentMarkdownMermaid(c *ui.Context, p *ui.Theme, oi oiColors,
	block agentMarkdownBlock) {
	style := agentMarkdownMermaidStyle(oi)
	state := m.requestAgentMarkdownDiagram(block.Text, style)
	ui.Column(c).Gap(3).Padding(5, 6).Radius(5).
		Background(oi.hoverTint).BorderWidth(1, 1, 1, 1).
		BorderColor(oi.border).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Mermaid").Grow(1).FontSize(10).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
			ui.Button(c, "Copy").OnClick(func() { c.WriteClipboard(block.Text) })
		})
		if state.Loading || state.Diagram == nil {
			ui.Text(c, "Rendering diagram…").FontSize(11).TextColor(oi.textMuted)
			return
		}
		diagram := state.Diagram
		if diagram.Err != "" {
			ui.Text(c, "Unable to draw Mermaid: "+diagram.Err).FontSize(11).
				TextColor(oi.danger).Wrap()
			return
		}
		if diagram.W <= 0 || diagram.H <= 0 {
			ui.Text(c, "Unable to draw Mermaid: the diagram has no size").
				FontSize(11).TextColor(oi.danger)
			return
		}
		ui.Box(c).WidthPercent(100).MaxHeight(440).
			AspectRatio(diagram.W / diagram.H).Background(style.BG).
			Label("Mermaid diagram").Draw(diagram.Draw)
	})
}

func agentMarkdownMermaidStyle(oi oiColors) mermaidpkg.Style {
	return mermaidpkg.Style{
		BG: oi.island, FG: oi.text.Over(oi.island), Muted: oi.textMuted.Over(oi.island),
		Line: oi.textSubtle.Over(oi.island), Surface: oi.elev,
		SurfaceAlt: oi.hover, Border: oi.borderStrong,
		Accent: oi.accent, Danger: oi.danger,
		Mono: oiFontMono, MonoWeight: 400,
	}
}

func (m *Model) requestAgentMarkdownDiagram(source string,
	style mermaidpkg.Style) *agentMarkdownDiagramState {
	key := agentMarkdownDiagramKey(source, style)
	if state := m.agents.markdownDiagrams[key]; state != nil {
		m.touchAgentMarkdownDiagram(key)
		return state
	}
	state := &agentMarkdownDiagramState{Loading: true}
	if m.agents.markdownDiagrams == nil {
		m.agents.markdownDiagrams = make(map[string]*agentMarkdownDiagramState)
	}
	m.agents.markdownDiagrams[key] = state
	m.agents.markdownDiagramOrder = append(m.agents.markdownDiagramOrder, key)
	for len(m.agents.markdownDiagrams) > agentMarkdownDiagramCount &&
		len(m.agents.markdownDiagramOrder) > 0 {
		oldest := m.agents.markdownDiagramOrder[0]
		m.agents.markdownDiagramOrder = m.agents.markdownDiagramOrder[1:]
		delete(m.agents.markdownDiagrams, oldest)
	}
	go func() {
		diagram := mermaidpkg.Render(source, style)
		m.post(func() {
			state.Loading = false
			state.Diagram = diagram
			m.invalidate()
		})
	}()
	return state
}

func agentMarkdownDiagramKey(source string, style mermaidpkg.Style) string {
	sum := sha256.New()
	_, _ = io.WriteString(sum, source)
	for _, color := range []ui.Color{
		style.BG, style.FG, style.Muted, style.Line, style.Surface,
		style.SurfaceAlt, style.Border, style.Accent, style.Danger,
	} {
		_, _ = fmt.Fprintf(sum, "\x00%d,%d,%d,%d", color.R, color.G, color.B, color.A)
	}
	_, _ = fmt.Fprintf(sum, "\x00%s\x00%s\x00%d", style.Sans, style.Mono, style.MonoWeight)
	return fmt.Sprintf("%x", sum.Sum(nil))
}

func (m *Model) touchAgentMarkdownDiagram(key string) {
	for index, cached := range m.agents.markdownDiagramOrder {
		if cached != key {
			continue
		}
		copy(m.agents.markdownDiagramOrder[index:], m.agents.markdownDiagramOrder[index+1:])
		m.agents.markdownDiagramOrder[len(m.agents.markdownDiagramOrder)-1] = key
		return
	}
	m.agents.markdownDiagramOrder = append(m.agents.markdownDiagramOrder, key)
}

func (m *Model) agentMarkdownTable(c *ui.Context, p *ui.Theme, oi oiColors,
	block agentMarkdownBlock, cwd string) {
	if len(block.Rows) == 0 {
		return
	}
	ui.Column(c).Gap(1).Children(func() {
		for rowIndex, row := range block.Rows {
			ui.Row(c).Wrap().Gap(1).Children(func() {
				for _, cell := range row {
					cellColumn := ui.Column(c).Grow(1).MinWidth(96).Padding(3, 5).
						Background(oi.hoverTint)
					if rowIndex > 0 && rowIndex%2 == 0 {
						cellColumn.Background(oi.island)
					}
					cellColumn.Children(func() {
						m.agentMarkdownParagraph(c, p, oi, strings.TrimSpace(cell), cwd, 11, rowIndex == 0)
					})
				}
			})
		}
	})
}

func agentMarkdownImageTarget(source, cwd string) (agentMarkdownImageRef, bool) {
	source = strings.TrimSpace(source)
	if source == "" || agentMarkdownHasControl(source) {
		return agentMarkdownImageRef{}, false
	}
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "data:image/") {
		if comma := strings.IndexByte(source, ','); comma < 0 {
			return agentMarkdownImageRef{}, false
		}
		return agentMarkdownImageRef{URL: source}, true
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(source, "//") {
		address := source
		if strings.HasPrefix(address, "//") {
			address = "https:" + address
		}
		parsed, err := url.Parse(address)
		if err != nil || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") {
			return agentMarkdownImageRef{}, false
		}
		return agentMarkdownImageRef{URL: address}, true
	}

	var path string
	if strings.HasPrefix(lower, "file:") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Scheme != "file" {
			return agentMarkdownImageRef{}, false
		}
		path, err = url.PathUnescape(parsed.Path)
		if err != nil {
			return agentMarkdownImageRef{}, false
		}
		if agentMarkdownFileWindowsAbsolute.MatchString(path) {
			path = path[1:]
		}
		if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
			path = "//" + parsed.Host + path
		}
	} else {
		if agentMarkdownNamesScheme(source) && !agentMarkdownWindowsPath(source) {
			return agentMarkdownImageRef{}, false
		}
		rawPath := source
		if cut := strings.IndexAny(rawPath, "?#"); cut >= 0 {
			rawPath = rawPath[:cut]
		}
		var err error
		path, err = url.PathUnescape(rawPath)
		if err != nil {
			return agentMarkdownImageRef{}, false
		}
		if agentMarkdownWindowsPath(path) || strings.HasPrefix(path, `\\`) {
			path = strings.ReplaceAll(path, `\`, "/")
		} else if !strings.HasPrefix(path, "/") {
			if path == "" {
				return agentMarkdownImageRef{}, false
			}
			if agentMarkdownWindowsPath(cwd) || strings.HasPrefix(cwd, `\\`) {
				path = strings.ReplaceAll(path, `\`, "/")
				cwd = strings.ReplaceAll(cwd, `\`, "/")
			}
			path = strings.TrimSuffix(cwd, "/") + "/" + path
		}
	}
	if path == "" || agentMarkdownHasControl(path) {
		return agentMarkdownImageRef{}, false
	}
	return agentMarkdownImageRef{Path: path}, true
}

var agentMarkdownWindowsAbsolute = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)
var agentMarkdownFileWindowsAbsolute = regexp.MustCompile(`^/[a-zA-Z]:/`)
var agentMarkdownScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

func agentMarkdownWindowsPath(path string) bool {
	return agentMarkdownWindowsAbsolute.MatchString(path)
}

func agentMarkdownNamesScheme(value string) bool {
	return agentMarkdownScheme.MatchString(value) && !agentMarkdownWindowsPath(value)
}

func agentMarkdownHasControl(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

func (m *Model) agentMarkdownImage(c *ui.Context, oi oiColors, run agentMarkdownRun,
	cwd string, size float32) {
	target, ok := agentMarkdownImageTarget(run.Image, cwd)
	if !ok {
		agentMarkdownImageError(c, oi, run.Image, size)
		return
	}
	if target.URL != "" && !strings.HasPrefix(strings.ToLower(target.URL), "data:") {
		host := target.URL
		if parsed, err := url.Parse(target.URL); err == nil && parsed.Host != "" {
			host = parsed.Host
		}
		if m.agents.markdownAsked == nil {
			m.agents.markdownAsked = make(map[string]bool)
		}
		if !m.agents.markdownAsked[target.URL] {
			ui.Button(c, "Load image from "+host).FontSize(size).Tooltip(target.URL).
				OnClick(func() {
					m.agents.markdownAsked[target.URL] = true
					m.requestAgentMarkdownImage(target, run.Image, false)
					m.invalidate()
				})
			return
		}
	}
	state := m.requestAgentMarkdownImage(target, run.Image, true)
	if state == nil || state.Loading || state.Source == nil {
		label := strings.TrimSpace(run.Text)
		if label == "" {
			label = "Loading image…"
		}
		if state != nil && state.Err != "" {
			agentMarkdownImageError(c, oi, run.Image, size)
			return
		}
		ui.Text(c, label).FontSize(size).TextColor(oi.textMuted).Padding(4, 6)
		return
	}
	image := ui.Image(c, state.Source).MaxWidthPercent(100).MaxHeight(480).
		Fit(ui.Contain).Label(run.Text)
	tooltip := run.Title
	if tooltip == "" {
		tooltip = run.Image
	}
	image.Tooltip(tooltip)
}

func agentMarkdownImageError(c *ui.Context, oi oiColors, source string, size float32) {
	ui.Text(c, "Unable to load image: "+source).FontSize(size).
		TextColor(oi.textMuted).Wrap()
}

func (m *Model) requestAgentMarkdownImage(target agentMarkdownImageRef,
	source string, load bool) *agentMarkdownImageState {
	key := agentMarkdownImageKey(target, source)
	if state := m.agents.markdownImages[key]; state != nil {
		m.touchAgentMarkdownImage(key)
		return state
	}
	state := &agentMarkdownImageState{}
	if m.agents.markdownImages == nil {
		m.agents.markdownImages = make(map[string]*agentMarkdownImageState)
	}
	m.agents.markdownImages[key] = state
	m.agents.markdownImageOrder = append(m.agents.markdownImageOrder, key)
	for len(m.agents.markdownImages) > agentMarkdownImageCount &&
		len(m.agents.markdownImageOrder) > 0 {
		oldest := m.agents.markdownImageOrder[0]
		m.agents.markdownImageOrder = m.agents.markdownImageOrder[1:]
		delete(m.agents.markdownImages, oldest)
	}
	if !load {
		return state
	}
	state.Loading = true
	go func() {
		decoded, err := loadAgentMarkdownImage(target)
		m.post(func() {
			state.Loading = false
			if err != nil {
				state.Err = err.Error()
			} else {
				state.Source = decoded
			}
			m.invalidate()
		})
	}()
	return state
}

func agentMarkdownImageKey(target agentMarkdownImageRef, source string) string {
	identity := target.URL
	if identity == "" {
		identity = target.Path
		if info, err := os.Stat(identity); err == nil {
			identity += fmt.Sprintf("\x00%d\x00%d", info.ModTime().UnixNano(), info.Size())
		}
	}
	if identity == "" {
		identity = source
	}
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("%x", sum[:])
}

func (m *Model) touchAgentMarkdownImage(key string) {
	for index, cached := range m.agents.markdownImageOrder {
		if cached != key {
			continue
		}
		copy(m.agents.markdownImageOrder[index:], m.agents.markdownImageOrder[index+1:])
		m.agents.markdownImageOrder[len(m.agents.markdownImageOrder)-1] = key
		return
	}
	m.agents.markdownImageOrder = append(m.agents.markdownImageOrder, key)
}

func loadAgentMarkdownImage(target agentMarkdownImageRef) (ui.ImageSource, error) {
	if target.URL != "" {
		if strings.HasPrefix(strings.ToLower(target.URL), "data:") {
			data, err := agentMarkdownDataImage(target.URL)
			if err != nil {
				return nil, err
			}
			return decodeAgentMarkdownImage(data, "")
		}
		response, err := agentMarkdownHTTP.Get(target.URL)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("the image request returned %s", response.Status)
		}
		if response.ContentLength > agentMarkdownImageLimit {
			return nil, fmt.Errorf("images must be 20 MB or smaller")
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, agentMarkdownImageLimit+1))
		if err != nil {
			return nil, err
		}
		if len(data) > agentMarkdownImageLimit {
			return nil, fmt.Errorf("images must be 20 MB or smaller")
		}
		return decodeAgentMarkdownImage(data, response.Header.Get("Content-Type"))
	}
	info, err := os.Stat(target.Path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("the image path is a directory")
	}
	if info.Size() > agentMarkdownImageLimit {
		return nil, fmt.Errorf("images must be 20 MB or smaller")
	}
	data, err := os.ReadFile(target.Path)
	if err != nil {
		return nil, err
	}
	return decodeAgentMarkdownImage(data, target.Path)
}

func agentMarkdownDataImage(source string) ([]byte, error) {
	comma := strings.IndexByte(source, ',')
	if comma < 0 {
		return nil, fmt.Errorf("the image data URL is incomplete")
	}
	metadata, payload := source[:comma], source[comma+1:]
	var data []byte
	var err error
	if strings.Contains(strings.ToLower(metadata), ";base64") {
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(payload)
		}
	} else {
		var decoded string
		decoded, err = url.PathUnescape(payload)
		data = []byte(decoded)
	}
	if err != nil {
		return nil, err
	}
	if len(data) > agentMarkdownImageLimit {
		return nil, fmt.Errorf("images must be 20 MB or smaller")
	}
	return data, nil
}

func decodeAgentMarkdownImage(data []byte, name string) (ui.ImageSource, error) {
	contentType := http.DetectContentType(data)
	if strings.EqualFold(filepath.Ext(name), ".svg") ||
		strings.Contains(strings.ToLower(contentType), "svg") {
		return ui.ParseSVG(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("that file is not a supported image")
	}
	return ui.DecodeBitmap(data)
}

func (m *Model) openAgentMarkdownLink(c *ui.Context, cwd, href string) {
	href = strings.TrimSpace(strings.Trim(href, "<>"))
	if href == "" || strings.HasPrefix(href, "#") {
		return
	}
	lower := strings.ToLower(href)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		c.OpenURL(href)
		return
	}
	if strings.HasPrefix(href, "//") || agentMarkdownNamesScheme(href) {
		return
	}
	path, line, column := agentMarkdownLocalTarget(cwd, href)
	if path != "" {
		m.revealAt(path, max(0, line-1), max(0, column-1))
	}
}

func agentMarkdownInlineCodeTarget(cwd, text string) (string, int, int) {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, " \t\r\n") || strings.Contains(text, "://") ||
		agentMarkdownHasControl(text) {
		return "", 0, 0
	}
	absolute := strings.HasPrefix(text, "/")
	matchText := text
	if absolute {
		matchText = strings.TrimPrefix(text, "/")
	}
	matches := agentMarkdownInlinePath.FindStringSubmatch(matchText)
	if matches == nil {
		return "", 0, 0
	}
	path := matches[1]
	if absolute {
		path = "/" + path
	} else {
		path = strings.TrimSuffix(cwd, "/") + "/" + path
	}
	line, column := 0, 0
	value := matches[2]
	if value == "" {
		value = matches[4]
	}
	if value != "" {
		number, _ := strconv.Atoi(value)
		line = max(0, number-1)
	}
	if matches[3] != "" {
		number, _ := strconv.Atoi(matches[3])
		column = max(0, number-1)
	}
	if _, err := os.Stat(path); err != nil {
		return "", 0, 0
	}
	return path, line, column
}

func agentMarkdownLocalTarget(cwd, href string) (string, int, int) {
	if strings.HasPrefix(strings.ToLower(href), "file://") {
		parsed, err := url.Parse(href)
		if err != nil {
			return "", 0, 0
		}
		href = parsed.Path
	}
	if decoded, err := url.PathUnescape(href); err == nil {
		href = decoded
	}
	line, column := 1, 1
	if matches := agentMarkdownLocalLocation.FindStringSubmatch(href); matches != nil {
		href = matches[1]
		for _, value := range matches[2:] {
			if value == "" {
				continue
			}
			number, _ := strconv.Atoi(value)
			if line == 1 {
				line = max(1, number)
			} else {
				column = max(1, number)
			}
		}
	}
	if href == "" {
		return "", 0, 0
	}
	if !filepath.IsAbs(href) {
		href = filepath.Join(cwd, href)
	}
	href = filepath.Clean(href)
	if _, err := os.Stat(href); err != nil {
		return "", 0, 0
	}
	return href, line, column
}

func agentMarkdownDebugLabel(block agentMarkdownBlock) string {
	if block.Kind == "code" && block.Lang != "" {
		return fmt.Sprintf("%s code", block.Lang)
	}
	return block.Kind
}
