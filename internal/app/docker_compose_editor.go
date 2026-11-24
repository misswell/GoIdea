package app

import (
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

// Docker Compose keeps schema support in the YAML language server. These
// helpers add OpenIDEA's file-local references: depends_on, links,
// volumes_from, networks, volumes, configs, secrets and extends.service.

var (
	composeValueCompletion = regexp.MustCompile(`^(\s*)(-\s+)?("([^"]+)"|'([^']+)'|([A-Za-z0-9._-]+))\s*:\s*([A-Za-z0-9._-]*)$`)
	composeListCompletion  = regexp.MustCompile(`^(\s*)-\s+([A-Za-z0-9._-]*)$`)
	composeKeyCompletion   = regexp.MustCompile(`^(\s*)([A-Za-z0-9._-]*)$`)
	composeExtendsService  = regexp.MustCompile(`^\s*service\s*:\s*([A-Za-z0-9._-]+)`)
)

func stripComposeComment(line string) (string, int) {
	var quote rune
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i], i
		}
	}
	return line, -1
}

func composeLineAt(text string, pos int) (lines []string, lineIndex, lineStart, col int) {
	lines = strings.Split(text, "\n")
	pos = max(0, min(pos, len(text)))
	offset := 0
	for i, raw := range lines {
		next := offset + len(raw) + 1
		if pos < next || i == len(lines)-1 {
			return lines, i, offset, min(max(0, pos-offset), len(strings.TrimSuffix(raw, "\r")))
		}
		offset = next
	}
	return lines, 0, 0, 0
}

func composeAncestorPath(lines []string, lineIndex, targetIndent int) []string {
	var path []string
	indent := targetIndent
	for i := lineIndex - 1; i >= 0; i-- {
		code, _ := stripComposeComment(strings.TrimSuffix(lines[i], "\r"))
		if strings.TrimSpace(code) == "" {
			continue
		}
		m := composeAnyKey.FindStringSubmatch(code)
		if m == nil {
			continue
		}
		lead := len(m[1])
		if lead >= indent {
			continue
		}
		path = append([]string{composeKeyName(m)}, path...)
		indent = lead
		if indent == 0 {
			break
		}
	}
	return path
}

func composeCompletionItems(names []string, detail string) []editor.Completion {
	items := make([]editor.Completion, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		items = append(items, editor.Completion{
			Label:         name,
			Insert:        name,
			Detail:        detail,
			Kind:          editor.CompletionConstant,
			Documentation: "Docker Compose " + detail,
		})
	}
	return items
}

func composeSectionNames(
	declarations map[string]map[string]composeDeclaration,
	section string,
) []string {
	if section == "networks" {
		names := make([]string, 0, len(declarations[section])+1)
		if _, ok := declarations[section]["default"]; !ok {
			names = append(names, "default")
		}
		for name := range declarations[section] {
			names = append(names, name)
		}
		return names
	}
	names := make([]string, 0, len(declarations[section]))
	for name := range declarations[section] {
		names = append(names, name)
	}
	return names
}

func withoutComposeName(names []string, excluded string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name != excluded {
			out = append(out, name)
		}
	}
	return out
}

func composeSiblingKeys(lines []string, current, parent int, indent int) map[string]bool {
	parentIndent := -1
	if parent >= 0 {
		code, _ := stripComposeComment(strings.TrimSuffix(lines[parent], "\r"))
		parentIndent = len(code) - len(strings.TrimLeft(code, " \t"))
	}
	out := map[string]bool{}
	for i := max(0, parent+1); i < len(lines); i++ {
		if i == current {
			continue
		}
		code, _ := stripComposeComment(strings.TrimSuffix(lines[i], "\r"))
		if strings.TrimSpace(code) == "" {
			continue
		}
		lead := len(code) - len(strings.TrimLeft(code, " \t"))
		if parentIndent >= 0 && lead <= parentIndent {
			break
		}
		if lead != indent {
			continue
		}
		if m := composeAnyKey.FindStringSubmatch(code); m != nil {
			out[composeKeyName(m)] = true
		}
	}
	return out
}

func composeCompletionsAt(text string, pos int, explicit bool) (int, []editor.Completion, bool) {
	lines, lineIndex, _, col := composeLineAt(text, pos)
	rawLine := strings.TrimSuffix(lines[lineIndex], "\r")
	code, comment := stripComposeComment(rawLine)
	if comment >= 0 && col > comment {
		return 0, nil, false
	}
	before := code[:min(col, len(code))]
	after := code[min(col, len(code)):]
	declarations := composeDeclarations(text)
	services := composeSectionNames(declarations, "services")

	if m := composeValueCompletion.FindStringSubmatch(before); m != nil {
		prefix := m[7]
		if !explicit && prefix == "" {
			return 0, nil, false
		}
		key := ""
		for i := 4; i <= 6; i++ {
			if m[i] != "" {
				key = m[i]
				break
			}
		}
		keyIndent := len(m[1])
		if m[2] != "" {
			keyIndent += 2
		}
		path := composeAncestorPath(lines, lineIndex, keyIndent)
		if len(path) != 3 || path[0] != "services" {
			return 0, nil, false
		}
		section := path[2]
		switch {
		case section == "extends" && key == "service":
			return pos - len(prefix), composeCompletionItems(
				withoutComposeName(services, path[1]), "service",
			), true
		case (section == "volumes" || section == "configs" || section == "secrets") && key == "source":
			return pos - len(prefix), composeCompletionItems(
				composeSectionNames(declarations, section), strings.TrimSuffix(section, "s"),
			), true
		default:
			return 0, nil, false
		}
	}

	if m := composeListCompletion.FindStringSubmatch(before); m != nil {
		prefix := m[2]
		if !explicit && prefix == "" {
			return 0, nil, false
		}
		path := composeAncestorPath(lines, lineIndex, len(m[1])+1)
		if len(path) != 3 || path[0] != "services" {
			return 0, nil, false
		}
		section := path[2]
		switch section {
		case "depends_on", "links", "volumes_from":
			return pos - len(prefix), composeCompletionItems(
				withoutComposeName(services, path[1]), "service",
			), true
		case "networks", "volumes", "configs", "secrets":
			return pos - len(prefix), composeCompletionItems(
				composeSectionNames(declarations, section), strings.TrimSuffix(section, "s"),
			), true
		default:
			return 0, nil, false
		}
	}

	if m := composeKeyCompletion.FindStringSubmatch(before); m != nil {
		prefix := m[2]
		if !explicit && prefix == "" {
			return 0, nil, false
		}
		keyIndent := len(m[1])
		path := composeAncestorPath(lines, lineIndex, keyIndent)
		if len(path) != 3 || path[0] != "services" {
			return 0, nil, false
		}
		section := path[2]
		if section != "depends_on" && section != "networks" {
			return 0, nil, false
		}
		siblings := composeSiblingKeys(lines, lineIndex, -1, keyIndent)
		names := composeSectionNames(declarations, section)
		filtered := names[:0]
		for _, name := range names {
			if name != path[1] && !siblings[name] {
				filtered = append(filtered, name)
			}
		}
		hasColon := strings.HasPrefix(strings.TrimSpace(after), ":")
		items := composeCompletionItems(filtered, strings.TrimSuffix(section, "s"))
		if !hasColon {
			for i := range items {
				if section == "depends_on" {
					items[i].Insert = items[i].Label + ":\n" + m[1] + "  condition: service_started"
				} else {
					items[i].Insert = items[i].Label + ":"
				}
			}
		}
		return pos - len(prefix), items, len(items) > 0
	}
	return 0, nil, false
}

func composeWordAt(line string, col int) (word string, from, to int, ok bool) {
	isWord := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
			b >= '0' && b <= '9' || b == '_' || b == '.' || b == '-'
	}
	col = max(0, min(col, len(line)))
	from, to = col, col
	for from > 0 && isWord(line[from-1]) {
		from--
	}
	for to < len(line) && isWord(line[to]) {
		to++
	}
	if from >= to {
		return "", 0, 0, false
	}
	return line[from:to], from, to, true
}

func composeReferenceAt(text string, pos int) (composeDeclaration, bool) {
	lines, lineIndex, _, col := composeLineAt(text, pos)
	code, comment := stripComposeComment(strings.TrimSuffix(lines[lineIndex], "\r"))
	if comment >= 0 && col > comment {
		return composeDeclaration{}, false
	}
	word, _, _, ok := composeWordAt(code, col)
	if !ok {
		return composeDeclaration{}, false
	}
	lead := len(code) - len(strings.TrimLeft(code, " \t"))
	trimmed := strings.TrimLeft(code, " \t")
	listKey := strings.HasPrefix(trimmed, "- ") &&
		composeAnyKey.MatchString(strings.TrimPrefix(trimmed, "- "))
	listItem := strings.HasPrefix(trimmed, "- ")
	targetIndent := lead
	switch {
	case listKey:
		targetIndent += 2
	case listItem:
		targetIndent++
	}
	path := composeAncestorPath(lines, lineIndex, targetIndent)
	if len(path) != 3 || path[0] != "services" {
		return composeDeclaration{}, false
	}
	declarations := composeDeclarations(text)
	section := path[2]
	switch section {
	case "depends_on", "links", "volumes_from":
		if d, ok := declarations["services"][word]; ok {
			return d, true
		}
	case "networks", "volumes", "configs", "secrets":
		if d, ok := declarations[section][word]; ok {
			return d, true
		}
	case "extends":
		if m := composeExtendsService.FindStringSubmatch(code); m != nil && m[1] == word {
			if d, ok := declarations["services"][word]; ok {
				return d, true
			}
		}
	}
	return composeDeclaration{}, false
}

func (m *Model) requestComposeCompletion(t *Tab, req editor.CompletionRequest) bool {
	if !m.tabOpen(t) || t.doc == nil || t.state == nil {
		return false
	}
	text := t.doc.Buffer().Snapshot().String()
	if !m.isComposeFile(t.doc.Path(), text) {
		return false
	}
	start, items, ok := composeCompletionsAt(text, req.Offset, req.Explicit)
	if !ok || len(items) == 0 {
		return false
	}
	m.showCompletionResults(t, req, start, items, nil)
	return true
}

func (m *Model) composeGotoDeclaration(t *Tab, off int) bool {
	if !m.tabOpen(t) || t.doc == nil || t.state == nil {
		return false
	}
	text := t.doc.Buffer().Snapshot().String()
	if !m.isComposeFile(t.doc.Path(), text) {
		return false
	}
	declaration, ok := composeReferenceAt(text, off)
	if !ok {
		return false
	}
	m.recordNavigation(true)
	t.state.Select(declaration.from, declaration.to)
	t.state.ScrollTo(declaration.line)
	m.invalidate()
	return true
}
