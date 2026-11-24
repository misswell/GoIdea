package app

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// seLocationQuery is a Search Everywhere query with an optional source
// location removed. Lines and columns are zero-based after parsing.
type seLocationQuery struct {
	text string
	line int
	col  int
	has  bool
}

var seLocationSuffix = regexp.MustCompile(`^(.*?)(?::(\d+)(?::(\d+))?|\((\d+)(?:[,:](\d+))?\)):?$`)

func seTextQueryReady(query string) bool {
	return utf8.RuneCountInString(strings.TrimSpace(query)) >= 2
}

func seFileMatchIndices(relPath, name string, indices []uint32) []int {
	start := utf8.RuneCountInString(strings.TrimSuffix(relPath, name))
	end := start + utf8.RuneCountInString(name)
	out := make([]int, 0, len(indices))
	for _, index := range indices {
		if at := int(index); at >= start && at < end {
			out = append(out, at-start)
		}
	}
	return out
}

// parseSELocationQuery accepts the same chooser locations as OpenIDEA's
// common/query.ts: File.kt:42, File.kt:42:5, File.kt(42), quoted paths,
// file:// URLs and Windows separators.
func parseSELocationQuery(raw, root string) seLocationQuery {
	text := strings.TrimSpace(raw)
	if len(text) >= 2 {
		first, last := text[0], text[len(text)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			text = strings.TrimSpace(text[1 : len(text)-1])
		}
	}
	if strings.HasPrefix(strings.ToLower(text), "file://") {
		text = text[len("file://"):]
		if decoded, err := url.PathUnescape(text); err == nil {
			text = decoded
		}
	}
	if (len(text) >= 3 && text[1] == ':' && (text[2] == '\\' || text[2] == '/')) ||
		(strings.Contains(text, `\`) && !strings.Contains(text, "/")) {
		text = strings.ReplaceAll(text, `\`, "/")
	}

	var line, col int
	has := false
	if match := seLocationSuffix.FindStringSubmatch(text); match != nil {
		lineText := match[2]
		if lineText == "" {
			lineText = match[4]
		}
		lineNumber, err := strconv.Atoi(lineText)
		if err == nil && lineNumber > 0 {
			text = match[1]
			line = lineNumber - 1
			has = true
			colText := match[3]
			if colText == "" {
				colText = match[5]
			}
			if colNumber, err := strconv.Atoi(colText); err == nil && colNumber > 0 {
				col = colNumber - 1
			}
		}
	}

	root = strings.TrimRight(strings.ReplaceAll(root, `\`, "/"), "/")
	if root != "" {
		switch {
		case text == root:
			text = ""
		case strings.HasPrefix(text, root+"/"):
			text = text[len(root)+1:]
		}
	}
	return seLocationQuery{text: text, line: line, col: col, has: has}
}

type seCommandAction struct {
	tab   seTab
	label string
}

var seScopeCommands = []seCommandAction{
	{seTabClasses, "Classes"},
	{seTabFiles, "Files"},
	{seTabSymbols, "Symbols"},
	{seTabActions, "Actions"},
	{seTabText, "Text"},
}

// seParsedCommand is either the scope-command list shown while typing or a
// complete command that switches tabs and leaves its text as the query.
type seParsedCommand struct {
	list bool
	rows []seItem
	tab  seTab
	rest string
}

// parseSECommand implements OpenIDEA's slash commands. "/fi" lists matching
// scopes, "/files foo" runs Files with "foo", and paths remain searches.
func parseSECommand(query string) (seParsedCommand, bool) {
	if !strings.HasPrefix(query, "/") {
		return seParsedCommand{}, false
	}
	body := query[1:]
	space := strings.IndexFunc(body, unicode.IsSpace)
	if space >= 0 {
		word := strings.ToLower(body[:space])
		rest := strings.TrimLeftFunc(body[space:], unicode.IsSpace)
		for _, command := range seScopeCommands {
			if word == string(command.tab) {
				return seParsedCommand{tab: command.tab, rest: rest}, true
			}
		}
		return seParsedCommand{}, false
	}

	prefix := strings.ToLower(body)
	rows := make([]seItem, 0, len(seScopeCommands))
	for _, command := range seScopeCommands {
		name := "/" + string(command.tab)
		if strings.HasPrefix(name, "/"+prefix) {
			rows = append(rows, seItem{
				kind:    "command",
				title:   name,
				detail:  command.label,
				command: command.tab,
			})
		}
	}
	if len(rows) == 0 {
		return seParsedCommand{}, false
	}
	return seParsedCommand{list: true, rows: rows}, true
}
