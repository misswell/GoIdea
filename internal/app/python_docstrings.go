package app

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	pythonRestField = regexp.MustCompile(
		`^\s*:([\p{L}_][\p{L}\p{N}_.]*)(?:\s+([^:\s][^:]*))?:`,
	)
	pythonEpytextField = regexp.MustCompile(
		`^\s*@([\p{L}_][\p{L}\p{N}_.]*)(?:\s+([^:\s][^:]*))?:`,
	)
	pythonRestDirective = regexp.MustCompile(`^(\s*\.\.\s+)([\p{L}_][\p{L}\p{N}_.-]*)::`)
	pythonGoogleHeader  = regexp.MustCompile(`^(\s*)([\p{L}][\p{L}\p{N} ]*?):\s*$`)
	pythonNumpyHeader   = regexp.MustCompile(`^(\s*)([\p{L}][\p{L}\p{N} ]*[\p{L}\p{N}])\s*$`)
	pythonNumpyRule     = regexp.MustCompile(`^\s*-{3,}\s*$`)
)

var (
	pythonGoogleListSections = map[string]bool{
		"args": true, "arguments": true, "params": true, "parameters": true,
		"keyword args": true, "other params": true, "attributes": true,
	}
	pythonGoogleSections = map[string]bool{
		"args": true, "arguments": true, "params": true, "parameters": true,
		"keyword args": true, "other params": true, "attributes": true,
		"returns": true, "yields": true, "receives": true, "raises": true,
		"exceptions": true, "warns": true, "warnings": true, "examples": true,
		"example": true, "notes": true, "note": true, "see also": true,
	}
	pythonNumpyListSections = map[string]bool{
		"parameters": true, "other parameters": true, "attributes": true,
		"returns": true, "yields": true, "receives": true, "raises": true,
		"warns": true,
	}
	pythonNumpySections = map[string]bool{
		"parameters": true, "other parameters": true, "attributes": true,
		"returns": true, "yields": true, "receives": true, "raises": true,
		"warns": true, "notes": true, "see also": true, "examples": true,
		"warnings": true,
	}
)

func (m *Model) pythonDocstringFormat() string {
	format := strings.ToLower(strings.TrimSpace(m.nativeRuntimeString(
		settingsPagePythonDocstrings, "editor.docstringFormat",
	)))
	switch format {
	case "plain", "rest", "epytext", "google", "numpy":
		return format
	default:
		return "rest"
	}
}

// pythonDocstringMarkdown maps PyCharm's docstring markup names and fields to
// explicit Markdown emphasis so the native documentation renderer preserves
// the same distinction between tags and the names they introduce.
func pythonDocstringMarkdown(value, format string) string {
	if format == "" || format == "plain" {
		return value
	}
	lines := strings.Split(value, "\n")
	switch format {
	case "epytext":
		pythonDocstringFields(lines, pythonEpytextField)
	case "google":
		pythonGoogleDocstring(lines)
	case "numpy":
		pythonNumpyDocstring(lines)
	default:
		pythonDocstringFields(lines, pythonRestField)
		for i, line := range lines {
			match := pythonRestDirective.FindStringSubmatchIndex(line)
			if match == nil {
				continue
			}
			start, end := match[4], match[5]
			lines[i] = line[:start] + "**" + line[start:end] + "::**" + line[end+2:]
		}
	}
	return strings.Join(lines, "\n")
}

func pythonDocstringFields(lines []string, pattern *regexp.Regexp) {
	for i, line := range lines {
		match := pattern.FindStringSubmatchIndex(line)
		if match == nil {
			continue
		}
		tagStart, tagEnd := match[2], match[3]
		line = line[:tagStart] + "**" + line[tagStart:tagEnd] + "**" + line[tagEnd:]
		if match[4] >= 0 && match[5] >= match[4] {
			shift := 4
			name := line[match[4]+shift : match[5]+shift]
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				nameStart := match[4] + shift + strings.Index(name, trimmed)
				line = line[:nameStart] + "*" + trimmed + "*" + line[nameStart+len(trimmed):]
			}
		}
		lines[i] = line
	}
}

func pythonGoogleDocstring(lines []string) {
	listIndent := -1
	entryIndent := -1
	for i := 0; i < len(lines); i++ {
		match := pythonGoogleHeader.FindStringSubmatchIndex(lines[i])
		if match != nil {
			name := strings.ToLower(strings.TrimSpace(lines[i][match[4]:match[5]]))
			next := i + 1
			for next < len(lines) && strings.TrimSpace(lines[next]) == "" {
				next++
			}
			headerIndent := match[3] - match[2]
			followingIndent := 0
			if next < len(lines) {
				followingIndent = pythonIndent(lines[next])
			}
			if pythonGoogleSections[name] && followingIndent > headerIndent {
				line := lines[i]
				lines[i] = line[:match[4]] + "**" + line[match[4]:match[5]] + "**" + line[match[5]:]
				listIndent, entryIndent = -1, -1
				if pythonGoogleListSections[name] {
					listIndent = headerIndent
				}
				continue
			}
		}
		if listIndent < 0 || strings.TrimSpace(lines[i]) == "" {
			continue
		}
		indent := pythonIndent(lines[i])
		if indent <= listIndent {
			listIndent, entryIndent = -1, -1
			continue
		}
		if entryIndent < 0 {
			entryIndent = indent
		}
		if indent != entryIndent {
			continue
		}
		content := lines[i][indent:]
		colon := strings.IndexByte(content, ':')
		if colon < 1 || colon+1 < len(content) && !unicode.IsSpace(rune(content[colon+1])) {
			continue
		}
		entry := strings.TrimSpace(content[:colon])
		if open := strings.IndexByte(entry, '('); open >= 0 {
			entry = strings.TrimSpace(entry[:open])
		}
		if entry == "" || !pythonDocstringEntryName(entry) {
			continue
		}
		line := lines[i]
		lines[i] = line[:indent] + "*" + entry + "*" + line[indent+len(entry):]
	}
}

func pythonNumpyDocstring(lines []string) {
	listIndent := -1
	entryIndent := -1
	for i := 0; i < len(lines); i++ {
		match := pythonNumpyHeader.FindStringSubmatchIndex(lines[i])
		if match != nil && i+1 < len(lines) && pythonNumpyRule.MatchString(lines[i+1]) {
			name := strings.ToLower(strings.TrimSpace(lines[i][match[4]:match[5]]))
			if pythonNumpySections[name] {
				line := lines[i]
				lines[i] = line[:match[4]] + "**" + line[match[4]:match[5]] + "**" + line[match[5]:]
				listIndent, entryIndent = -1, -1
				if pythonNumpyListSections[name] {
					listIndent = match[3] - match[2]
				}
				i++
				continue
			}
		}
		if listIndent < 0 || strings.TrimSpace(lines[i]) == "" {
			continue
		}
		indent := pythonIndent(lines[i])
		if indent < listIndent {
			listIndent, entryIndent = -1, -1
			continue
		}
		if entryIndent < 0 {
			entryIndent = indent
		}
		if indent != entryIndent {
			continue
		}
		content := lines[i][indent:]
		colon := strings.IndexByte(content, ':')
		if colon < 1 {
			continue
		}
		entry := strings.TrimSpace(content[:colon])
		if entry == "" || !pythonDocstringEntryName(entry) {
			continue
		}
		line := lines[i]
		lines[i] = line[:indent] + "*" + entry + "*" + line[indent+len(entry):]
	}
}

func pythonIndent(line string) int {
	return len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
}

func pythonDocstringEntryName(value string) bool {
	for _, ch := range value {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || strings.ContainsRune("*,. []_", ch) {
			continue
		}
		return false
	}
	return true
}
