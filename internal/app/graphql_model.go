package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"mayoi/internal/openidea"
)

var graphqlSchemaExtensions = map[string]bool{
	"graphql":  true,
	"graphqls": true,
	"gql":      true,
}

var graphqlConfigNames = []string{
	"graphql.config.yml",
	"graphql.config.yaml",
	"graphql.config.json",
	"graphql.config.js",
	"graphql.config.ts",
	".graphqlrc",
	".graphqlrc.yml",
	".graphqlrc.yaml",
	".graphqlrc.json",
}

const graphqlConfigTemplate = `schema: schema.graphql
documents: '**/*.graphql'
`

const graphqlGeneratedHeader = "# This file was generated. Do not edit manually."

type graphqlEndpoint struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type graphqlConfig struct {
	Schema    []string
	Endpoints []graphqlEndpoint
	Dynamic   bool
	Variables []string
}

type graphqlSchemaStats struct {
	Types      int `json:"types"`
	Inputs     int `json:"inputs"`
	Enums      int `json:"enums"`
	Directives int `json:"directives"`
}

type graphqlTypeReference struct {
	Name  string
	Owner string
	Line  int
	Col   int
}

type graphqlSchemaFile struct {
	Path       string
	Stats      graphqlSchemaStats
	Defined    []string
	References []graphqlTypeReference
}

type graphqlSchemaProject struct {
	Key        string
	ConfigPath string
	Root       string
	Pointers   []string
	Files      []graphqlSchemaFile
	Remote     []string
	Endpoints  []graphqlEndpoint
	Dynamic    bool
	Variables  []string
}

type graphqlConfiguredSchema struct {
	Path   string
	Config graphqlConfig
}

func graphqlIsSchemaFile(name string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	return graphqlSchemaExtensions[ext]
}

func graphqlIsConfigFile(name string) bool {
	for _, candidate := range graphqlConfigNames {
		if name == candidate {
			return true
		}
	}
	return false
}

func graphqlParseYAML(text string) any {
	type yamlLine struct {
		indent int
		text   string
	}
	lines := make([]yamlLine, 0)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		clean := graphqlStripYAMLComment(raw)
		trimmed := strings.TrimSpace(clean)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		indent := len(clean) - len(strings.TrimLeftFunc(clean, unicode.IsSpace))
		lines = append(lines, yamlLine{indent: indent, text: trimmed})
	}
	index := 0
	isItem := func(value string) bool {
		return value == "-" || strings.HasPrefix(value, "- ")
	}
	var block func(int) any
	var list func(int) []any
	var mapping func(int) map[string]any

	block = func(indent int) any {
		if index >= len(lines) || lines[index].indent < indent {
			return ""
		}
		if isItem(lines[index].text) {
			return list(lines[index].indent)
		}
		return mapping(lines[index].indent)
	}
	list = func(indent int) []any {
		out := []any{}
		for index < len(lines) && lines[index].indent == indent && isItem(lines[index].text) {
			rest := strings.TrimSpace(strings.TrimPrefix(lines[index].text, "-"))
			if rest == "" {
				index++
				out = append(out, block(indent+1))
				continue
			}
			if _, _, ok := graphqlYAMLKey(rest); ok {
				lines[index] = yamlLine{indent: indent + 2, text: rest}
				out = append(out, mapping(indent+2))
				continue
			}
			index++
			out = append(out, graphqlYAMLScalar(rest))
		}
		return out
	}
	mapping = func(indent int) map[string]any {
		out := map[string]any{}
		for index < len(lines) && lines[index].indent == indent {
			key, value, ok := graphqlYAMLKey(lines[index].text)
			if !ok {
				break
			}
			index++
			if value != "" {
				out[key] = graphqlYAMLScalar(value)
			} else {
				out[key] = block(indent + 1)
			}
		}
		return out
	}
	return block(0)
}

func graphqlStripYAMLComment(line string) string {
	var quote rune
	for i, r := range line {
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
		if r == '#' && (i == 0 || unicode.IsSpace(rune(line[i-1]))) {
			return line[:i]
		}
	}
	return line
}

func graphqlYAMLKey(text string) (string, string, bool) {
	var quote rune
	for i, r := range text {
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
		if r != ':' {
			continue
		}
		key := strings.TrimSpace(text[:i])
		if key == "" {
			return "", "", false
		}
		value := strings.TrimSpace(text[i+1:])
		return graphqlUnquote(key), value, true
	}
	return "", "", false
}

func graphqlYAMLScalar(text string) any {
	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		inner := strings.TrimSpace(text[1 : len(text)-1])
		if inner == "" {
			return []any{}
		}
		parts := strings.Split(inner, ",")
		out := make([]any, 0, len(parts))
		for _, part := range parts {
			out = append(out, graphqlUnquote(strings.TrimSpace(part)))
		}
		return out
	}
	return graphqlUnquote(text)
}

func graphqlUnquote(text string) string {
	if len(text) >= 2 {
		first, last := text[0], text[len(text)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return text[1 : len(text)-1]
		}
	}
	return text
}

var graphqlEnvironmentVariable = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::([^}]*))?\}`)

func graphqlAsStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			return []string{typed}
		}
	case []any:
		out := []string{}
		for _, item := range typed {
			out = append(out, graphqlAsStrings(item)...)
		}
		return out
	case map[string]any:
		out := make([]string, 0, len(typed))
		for key := range typed {
			out = append(out, key)
		}
		sort.Strings(out)
		return out
	}
	return nil
}

func graphqlAsHeaders(value any) map[string]string {
	out := map[string]string{}
	values, ok := value.(map[string]any)
	if !ok {
		return out
	}
	for key, item := range values {
		if text, ok := item.(string); ok {
			out[key] = text
		}
	}
	return out
}

func graphqlAsEndpoints(name, text string, value any) []graphqlEndpoint {
	values, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	byName := make(map[string]graphqlEndpoint, len(values))
	for endpointName, raw := range values {
		switch endpoint := raw.(type) {
		case string:
			byName[endpointName] = graphqlEndpoint{
				Name: endpointName, URL: endpoint, Headers: map[string]string{},
			}
		case map[string]any:
			url, ok := endpoint["url"].(string)
			if !ok {
				continue
			}
			byName[endpointName] = graphqlEndpoint{
				Name: endpointName, URL: url, Headers: graphqlAsHeaders(endpoint["headers"]),
			}
		}
	}
	order := graphqlEndpointDeclarationOrder(name, text)
	out := make([]graphqlEndpoint, 0, len(byName))
	seen := make(map[string]bool, len(byName))
	for _, endpointName := range order {
		if endpoint, ok := byName[endpointName]; ok && !seen[endpointName] {
			out = append(out, endpoint)
			seen[endpointName] = true
		}
	}
	leftovers := make([]string, 0, len(byName)-len(out))
	for endpointName := range byName {
		if !seen[endpointName] {
			leftovers = append(leftovers, endpointName)
		}
	}
	sort.Strings(leftovers)
	for _, endpointName := range leftovers {
		out = append(out, byName[endpointName])
	}
	return out
}

// graphqlOrderedJSON keeps object-key order while decoding graphql-config.
// encoding/json decodes into maps, which would turn endpoint declaration
// order into alphabetical order and change the default endpoint.
type graphqlOrderedJSON struct {
	Keys   []string
	Object map[string]*graphqlOrderedJSON
	Array  []*graphqlOrderedJSON
	Value  any
}

func graphqlDecodeOrderedJSON(decoder *json.Decoder) (*graphqlOrderedJSON, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	node := &graphqlOrderedJSON{}
	delim, ok := token.(json.Delim)
	if !ok {
		node.Value = token
		return node, nil
	}
	switch delim {
	case '{':
		node.Object = map[string]*graphqlOrderedJSON{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("GraphQL configuration has a non-string key")
			}
			value, err := graphqlDecodeOrderedJSON(decoder)
			if err != nil {
				return nil, err
			}
			node.Keys = append(node.Keys, key)
			node.Object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
	case '[':
		for decoder.More() {
			value, err := graphqlDecodeOrderedJSON(decoder)
			if err != nil {
				return nil, err
			}
			node.Array = append(node.Array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unexpected GraphQL configuration delimiter %q", delim)
	}
	return node, nil
}

func graphqlJSONEndpointOrder(text string) []string {
	decoder := json.NewDecoder(strings.NewReader(text))
	root, err := graphqlDecodeOrderedJSON(decoder)
	if err != nil {
		return nil
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil
	}
	extensions := root.Object["extensions"]
	if extensions == nil {
		return nil
	}
	endpoints := extensions.Object["endpoints"]
	if endpoints == nil {
		return nil
	}
	return append([]string(nil), endpoints.Keys...)
}

func graphqlYAMLEndpointOrder(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	sectionPattern := regexp.MustCompile(`^\s*["']?endpoints["']?\s*:`)
	section, sectionIndent := -1, 0
	for index, line := range lines {
		clean := graphqlStripYAMLComment(line)
		if sectionPattern.MatchString(clean) {
			section = index
			sectionIndent = len(clean) - len(strings.TrimLeftFunc(clean, unicode.IsSpace))
			break
		}
	}
	if section < 0 {
		return nil
	}
	childIndent := -1
	out := []string{}
	for _, line := range lines[section+1:] {
		clean := graphqlStripYAMLComment(line)
		trimmed := strings.TrimSpace(clean)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		indent := len(clean) - len(strings.TrimLeftFunc(clean, unicode.IsSpace))
		if indent <= sectionIndent {
			break
		}
		if childIndent < 0 {
			childIndent = indent
		}
		if indent != childIndent || strings.HasPrefix(trimmed, "-") {
			continue
		}
		if key, _, ok := graphqlYAMLKey(trimmed); ok {
			out = append(out, key)
		}
	}
	return out
}

func graphqlEndpointDeclarationOrder(name, text string) []string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".json") || strings.HasPrefix(strings.TrimSpace(text), "{") {
		return graphqlJSONEndpointOrder(text)
	}
	if strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".ts") {
		return nil
	}
	return graphqlYAMLEndpointOrder(text)
}

func graphqlParseConfig(name, text string) graphqlConfig {
	variables := []string{}
	seen := map[string]bool{}
	for _, match := range graphqlEnvironmentVariable.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		variables = append(variables, match[1])
	}
	empty := graphqlConfig{Variables: variables}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".ts") {
		empty.Dynamic = true
		return empty
	}

	var data any
	var err error
	if strings.HasSuffix(lower, ".json") || strings.HasPrefix(strings.TrimSpace(text), "{") {
		err = json.Unmarshal([]byte(text), &data)
	} else {
		data = graphqlParseYAML(text)
	}
	if err != nil {
		return empty
	}
	object, ok := data.(map[string]any)
	if !ok {
		return empty
	}
	extensions, _ := object["extensions"].(map[string]any)
	empty.Schema = append(empty.Schema, graphqlAsStrings(object["schema"])...)
	empty.Schema = append(empty.Schema, graphqlAsStrings(object["schemaPath"])...)
	empty.Endpoints = graphqlAsEndpoints(name, text, extensions["endpoints"])
	return empty
}

func graphqlExpandVariables(text string, values map[string]string) string {
	return graphqlEnvironmentVariable.ReplaceAllStringFunc(text, func(match string) string {
		parts := graphqlEnvironmentVariable.FindStringSubmatch(match)
		if len(parts) < 2 {
			return ""
		}
		name := parts[1]
		if value, ok := values[name]; ok {
			return value
		}
		if len(parts) >= 3 {
			return parts[2]
		}
		return ""
	})
}

func graphqlMissingVariables(endpoint graphqlEndpoint, values map[string]string) []string {
	seen := map[string]bool{}
	matchText := func(text string) {
		for _, match := range graphqlEnvironmentVariable.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 || match[2] < 0 || match[3] < 0 {
				continue
			}
			name := text[match[2]:match[3]]
			if values[name] != "" {
				continue
			}
			// The optional fallback is present when its submatch exists,
			// even when it is empty: ${NAME:} is not a missing variable.
			if len(match) >= 6 && match[4] >= 0 {
				continue
			}
			seen[name] = true
		}
	}
	matchText(endpoint.URL)
	for _, value := range endpoint.Headers {
		matchText(value)
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func graphqlExpandEndpoint(endpoint graphqlEndpoint, values map[string]string) graphqlEndpoint {
	out := endpoint
	out.URL = graphqlExpandVariables(endpoint.URL, values)
	out.Headers = make(map[string]string, len(endpoint.Headers))
	for key, value := range endpoint.Headers {
		out.Headers[key] = graphqlExpandVariables(value, values)
	}
	return out
}

func graphqlEndpointLines(name, text string) []struct {
	Endpoint string
	Line     int
} {
	endpoints := graphqlParseConfig(name, text).Endpoints
	if len(endpoints) == 0 {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	section := -1
	sectionPattern := regexp.MustCompile(`^\s*["']?endpoints["']?\s*:`)
	for index, line := range lines {
		if sectionPattern.MatchString(line) {
			section = index
			break
		}
	}
	if section < 0 {
		return nil
	}
	out := make([]struct {
		Endpoint string
		Line     int
	}, 0, len(endpoints))
	for _, endpoint := range endpoints {
		pattern := regexp.MustCompile(`^\s*["']?` + regexp.QuoteMeta(endpoint.Name) + `["']?\s*:`)
		for index := section + 1; index < len(lines); index++ {
			if pattern.MatchString(lines[index]) {
				out = append(out, struct {
					Endpoint string
					Line     int
				}{Endpoint: endpoint.Name, Line: index})
				break
			}
		}
	}
	return out
}

func graphqlGlobToRegExp(pattern string) *regexp.Regexp {
	glob := strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	var out strings.Builder
	out.WriteByte('^')
	for index := 0; index < len(glob); index++ {
		char := glob[index]
		switch char {
		case '*':
			if index+1 < len(glob) && glob[index+1] == '*' {
				index++
				if index+1 < len(glob) && glob[index+1] == '/' {
					index++
					out.WriteString(`(?:.*/)?`)
				} else {
					out.WriteString(`.*`)
				}
			} else {
				out.WriteString(`[^/]*`)
			}
		case '?':
			out.WriteString(`[^/]`)
		case '{':
			end := strings.IndexByte(glob[index+1:], '}')
			if end < 0 {
				out.WriteString(`\{`)
				continue
			}
			end += index + 1
			parts := strings.Split(glob[index+1:end], ",")
			out.WriteString(`(?:`)
			for i, part := range parts {
				if i > 0 {
					out.WriteByte('|')
				}
				out.WriteString(regexp.QuoteMeta(part))
			}
			out.WriteByte(')')
			index = end
		default:
			out.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	out.WriteByte('$')
	compiled, err := regexp.Compile(out.String())
	if err != nil {
		return regexp.MustCompile(`^\x00$`)
	}
	return compiled
}

func graphqlIsURL(pointer string) bool {
	lower := strings.ToLower(pointer)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

var graphqlBuiltinTypes = map[string]bool{
	"String": true, "Int": true, "Float": true, "Boolean": true, "ID": true,
}

var graphqlDefinitionPattern = regexp.MustCompile(
	`(?m)\b(extend[ \t]+)?(type|interface|enum|input|union|scalar)[ \t]+([_A-Za-z][_0-9A-Za-z]*)|\bdirective[ \t]+@([_A-Za-z][_0-9A-Za-z]*)`,
)

func graphqlCodeOnly(text string) string {
	code := []byte(text)
	for index := 0; index < len(code); {
		if index+2 < len(code) && string(code[index:index+3]) == `"""` {
			start := index
			index += 3
			for index+2 < len(code) && string(code[index:index+3]) != `"""` {
				index++
			}
			if index+2 < len(code) {
				index += 3
			} else {
				index = len(code)
			}
			for at := start; at < index; at++ {
				if code[at] != '\n' {
					code[at] = ' '
				}
			}
			continue
		}
		if code[index] == '#' {
			start := index
			for index < len(code) && code[index] != '\n' {
				index++
			}
			for at := start; at < index; at++ {
				code[at] = ' '
			}
			continue
		}
		if code[index] == '"' {
			start := index
			index++
			escaped := false
			for index < len(code) && code[index] != '\n' {
				if code[index] == '"' && !escaped {
					index++
					break
				}
				if code[index] == '\\' && !escaped {
					escaped = true
				} else {
					escaped = false
				}
				index++
			}
			for at := start; at < index; at++ {
				if code[at] != '\n' {
					code[at] = ' '
				}
			}
			continue
		}
		index++
	}
	return string(code)
}

func graphqlAnalyzeSchema(text string) (graphqlSchemaStats, []string, []graphqlTypeReference) {
	code := graphqlCodeOnly(text)
	lineStarts := []int{0}
	for index, char := range []byte(code) {
		if char == '\n' {
			lineStarts = append(lineStarts, index+1)
		}
	}
	position := func(offset int) (int, int) {
		line := sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > offset }) - 1
		if line < 0 {
			line = 0
		}
		return line, offset - lineStarts[line]
	}
	stats := graphqlSchemaStats{}
	definedSet := map[string]bool{}
	defined := []string{}
	references := []graphqlTypeReference{}
	refer := func(name, owner string, offset int) {
		line, col := position(offset)
		references = append(references, graphqlTypeReference{Name: name, Owner: owner, Line: line, Col: col})
	}

	nextDefinition := regexp.MustCompile(
		`\n[ \t]*(?:extend\b|type\b|interface\b|enum\b|input\b|union\b|scalar\b|directive\b|schema\b)`,
	)
	typeWord := regexp.MustCompile(`[_A-Za-z][_0-9A-Za-z]*`)
	fieldType := regexp.MustCompile(`:\s*\[*\s*([_A-Za-z][_0-9A-Za-z]*)`)
	for _, match := range graphqlDefinitionPattern.FindAllStringSubmatchIndex(code, -1) {
		directive := ""
		kind := ""
		name := ""
		if match[8] >= 0 {
			directive = code[match[8]:match[9]]
		} else {
			kind = code[match[4]:match[5]]
			name = code[match[6]:match[7]]
		}
		if directive != "" {
			stats.Directives++
			continue
		}
		extended := match[2] >= 0
		if !extended && !definedSet[name] {
			definedSet[name] = true
			defined = append(defined, name)
			switch kind {
			case "input":
				stats.Inputs++
			case "enum":
				stats.Enums++
			default:
				stats.Types++
			}
		}
		after := match[1]
		headerEnd := len(code)
		if brace := strings.IndexByte(code[after:], '{'); brace >= 0 {
			headerEnd = after + brace
		}
		if next := nextDefinition.FindStringIndex(code[after:]); next != nil && after+next[0] < headerEnd {
			headerEnd = after + next[0]
		}
		header := code[after:headerEnd]
		if kind == "union" || strings.Contains(header, "implements") {
			list := ""
			offset := after
			if kind == "union" {
				if equal := strings.IndexByte(header, '='); equal >= 0 {
					list = header[equal+1:]
					offset = after + equal + 1
				}
			} else if implemented := strings.Index(header, "implements"); implemented >= 0 {
				list = header[implemented+len("implements"):]
				offset = after + implemented + len("implements")
			}
			for _, ref := range typeWord.FindAllStringIndex(list, -1) {
				refer(list[ref[0]:ref[1]], name, offset+ref[0])
			}
		}
		if kind == "enum" || kind == "scalar" || kind == "union" ||
			headerEnd >= len(code) || code[headerEnd] != '{' {
			continue
		}
		body, bodyStart, ok := graphqlBlockAt(code, headerEnd)
		if !ok {
			continue
		}
		for _, ref := range fieldType.FindAllStringSubmatchIndex(body, -1) {
			typeName := body[ref[2]:ref[3]]
			refer(typeName, name, bodyStart+ref[3]-len(typeName))
		}
	}
	return stats, defined, references
}

func graphqlBlockAt(code string, open int) (string, int, bool) {
	depth := 0
	for index := open; index < len(code); index++ {
		switch code[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return code[open+1 : index], open + 1, true
			}
		}
	}
	return "", open + 1, false
}

func graphqlUnresolvedTypes(project graphqlSchemaProject) []graphqlProblem {
	known := map[string]bool{}
	for name := range graphqlBuiltinTypes {
		known[name] = true
	}
	for _, file := range project.Files {
		for _, name := range file.Defined {
			known[name] = true
		}
	}
	out := []graphqlProblem{}
	for _, file := range project.Files {
		for _, ref := range file.References {
			if known[ref.Name] {
				continue
			}
			source, code := "graphql", (*string)(nil)
			out = append(out, graphqlProblem{
				Path: file.Path,
				Diagnostic: openidea.Diagnostic{
					Range: openidea.OutlineRange{
						Start: openidea.OutlinePos{Line: uint32(ref.Line), Col: uint32(ref.Col)},
						End: openidea.OutlinePos{
							Line: uint32(ref.Line), Col: uint32(ref.Col + len(ref.Name)),
						},
					},
					Severity: "error",
					Message: fmt.Sprintf(
						"The field type '%s' is not present when resolving type '%s'", ref.Name, ref.Owner,
					),
					Source: &source,
					Code:   code,
				},
			})
		}
	}
	return out
}

func graphqlSchemaStatsOf(project graphqlSchemaProject) graphqlSchemaStats {
	total := graphqlSchemaStats{}
	for _, file := range project.Files {
		total.Types += file.Stats.Types
		total.Inputs += file.Stats.Inputs
		total.Enums += file.Stats.Enums
		total.Directives += file.Stats.Directives
	}
	return total
}

func graphqlSlash(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func graphqlDiscoverSchemas(
	root string,
	configs []graphqlConfiguredSchema,
	files []graphqlSchemaFile,
) []graphqlSchemaProject {
	root = graphqlSlash(root)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if len(configs) == 0 {
		return []graphqlSchemaProject{{
			Key: "default", Root: root, Files: append([]graphqlSchemaFile(nil), files...),
		}}
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].Path < configs[j].Path })
	projects := make([]graphqlSchemaProject, 0, len(configs))
	for _, configured := range configs {
		path := graphqlSlash(configured.Path)
		dir := graphqlSlash(filepath.Dir(path))
		globs := []*regexp.Regexp{}
		pointers := []string{}
		remote := []string{}
		for _, pointer := range configured.Config.Schema {
			if graphqlIsURL(pointer) {
				remote = append(remote, pointer)
				continue
			}
			pointers = append(pointers, pointer)
			globs = append(globs, graphqlGlobToRegExp(pointer))
		}
		selected := []graphqlSchemaFile{}
		for _, file := range files {
			relative, err := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(file.Path))
			if err != nil {
				continue
			}
			relative = filepath.ToSlash(relative)
			if relative == ".." || strings.HasPrefix(relative, "../") {
				continue
			}
			for _, glob := range globs {
				if glob.MatchString(relative) {
					selected = append(selected, file)
					break
				}
			}
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].Path < selected[j].Path })
		projects = append(projects, graphqlSchemaProject{
			Key: path, ConfigPath: path, Root: dir, Pointers: pointers, Files: selected,
			Remote: remote, Endpoints: configured.Config.Endpoints, Dynamic: configured.Config.Dynamic,
			Variables: configured.Config.Variables,
		})
	}
	return projects
}

type graphqlProblem struct {
	Path       string
	Diagnostic openidea.Diagnostic
}

type graphqlSchemaNode struct {
	Kind       string
	Key        string
	Project    *graphqlSchemaProject
	File       *graphqlSchemaFile
	Remote     string
	Path       string
	Diagnostic *openidea.Diagnostic
	Endpoint   *graphqlEndpoint
}

func graphqlProjectNodes(projects []graphqlSchemaProject) []graphqlSchemaNode {
	out := make([]graphqlSchemaNode, 0, len(projects))
	for index := range projects {
		project := &projects[index]
		out = append(out, graphqlSchemaNode{Kind: "project", Key: project.Key, Project: project})
	}
	return out
}

func graphqlSchemaProblems(
	project graphqlSchemaProject,
	diagnosticsFor func(string) []openidea.Diagnostic,
) []graphqlProblem {
	own := graphqlUnresolvedTypes(project)
	seen := map[string]bool{}
	for _, problem := range own {
		key := fmt.Sprintf("%s:%d:%d", problem.Path, problem.Diagnostic.Range.Start.Line, problem.Diagnostic.Range.Start.Col)
		seen[key] = true
	}
	out := append([]graphqlProblem(nil), own...)
	for _, file := range project.Files {
		for _, diagnostic := range diagnosticsFor(file.Path) {
			if diagnostic.Severity != "error" && diagnostic.Severity != "warning" {
				continue
			}
			key := fmt.Sprintf("%s:%d:%d", file.Path, diagnostic.Range.Start.Line, diagnostic.Range.Start.Col)
			if seen[key] {
				continue
			}
			out = append(out, graphqlProblem{Path: file.Path, Diagnostic: diagnostic})
		}
	}
	return out
}

func graphqlChildNodes(
	node graphqlSchemaNode,
	diagnosticsFor func(string) []openidea.Diagnostic,
) []graphqlSchemaNode {
	switch node.Kind {
	case "project":
		return []graphqlSchemaNode{
			{Kind: "summary", Key: node.Key + "#summary", Project: node.Project},
			{Kind: "errors", Key: node.Key + "#errors", Project: node.Project},
			{Kind: "endpoints", Key: node.Key + "#endpoints", Project: node.Project},
		}
	case "summary":
		out := []graphqlSchemaNode{}
		for index := range node.Project.Files {
			file := &node.Project.Files[index]
			out = append(out, graphqlSchemaNode{
				Kind: "file", Key: node.Key + ":" + file.Path, Project: node.Project, File: file,
			})
		}
		for _, url := range node.Project.Remote {
			out = append(out, graphqlSchemaNode{Kind: "remote", Key: node.Key + ":" + url, Remote: url})
		}
		return out
	case "errors":
		problems := graphqlSchemaProblems(*node.Project, diagnosticsFor)
		if len(problems) == 0 {
			return []graphqlSchemaNode{{Kind: "noErrors", Key: node.Key + ":none"}}
		}
		out := make([]graphqlSchemaNode, 0, len(problems))
		for index, problem := range problems {
			diagnostic := problem.Diagnostic
			out = append(out, graphqlSchemaNode{
				Kind:       "error",
				Key:        fmt.Sprintf("%s:%d:%s:%d:%d", node.Key, index, problem.Path, diagnostic.Range.Start.Line, diagnostic.Range.Start.Col),
				Path:       problem.Path,
				Diagnostic: &diagnostic,
			})
		}
		return out
	case "endpoints":
		if len(node.Project.Endpoints) == 0 {
			return []graphqlSchemaNode{{Kind: "noEndpoints", Key: node.Key + ":none", Project: node.Project}}
		}
		out := make([]graphqlSchemaNode, 0, len(node.Project.Endpoints))
		for index := range node.Project.Endpoints {
			endpoint := &node.Project.Endpoints[index]
			out = append(out, graphqlSchemaNode{
				Kind: "endpoint", Key: node.Key + ":" + endpoint.Name, Endpoint: endpoint,
			})
		}
		return out
	}
	return nil
}

type graphqlDefinition struct {
	Kind string
	Name string
	From int
	To   int
}

func graphqlTopLevelDefinitions(document string) []graphqlDefinition {
	code := graphqlCodeOnly(document)
	out := []graphqlDefinition{}
	for index := 0; index < len(code); {
		for index < len(code) && (unicode.IsSpace(rune(code[index])) || code[index] == ',') {
			index++
		}
		if index >= len(code) {
			break
		}
		from := index
		open := strings.IndexByte(code[index:], '{')
		if open < 0 {
			break
		}
		open += index
		depth := 0
		to := len(code)
		for at := open; at < len(code); at++ {
			switch code[at] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					to = at + 1
					at = len(code)
				}
			}
		}
		header := strings.TrimSpace(code[from:open])
		kind, name := "other", ""
		headerPattern := regexp.MustCompile(`^(query|mutation|subscription|fragment)\b\s*([_A-Za-z][_0-9A-Za-z]*)?`)
		if match := headerPattern.FindStringSubmatch(header); match != nil {
			name = match[2]
			if match[1] == "fragment" {
				kind = "fragment"
			} else {
				kind = "operation"
			}
		} else if header == "" {
			kind = "operation"
		}
		out = append(out, graphqlDefinition{Kind: kind, Name: name, From: from, To: to})
		index = to
	}
	return out
}

type graphqlRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type graphqlQuerySelectionContext struct {
	Text   string
	Ranges []graphqlRange
}

func graphqlQueryContext(document string, selection *graphqlRange) graphqlQuerySelectionContext {
	if selection != nil && selection.To > selection.From {
		return graphqlQuerySelectionContext{
			Text: strings.TrimSpace(document[selection.From:selection.To]),
			Ranges: []graphqlRange{{
				From: selection.From, To: selection.To,
			}},
		}
	}
	whole := graphqlQuerySelectionContext{
		Text: strings.TrimSpace(document),
		Ranges: []graphqlRange{{
			From: 0, To: len(document),
		}},
	}
	definitions := graphqlTopLevelDefinitions(document)
	caret := -1
	if selection != nil {
		caret = selection.From
	}
	operationIndex := -1
	for index, definition := range definitions {
		if definition.Kind == "operation" && definition.From <= caret && caret <= definition.To {
			operationIndex = index
			break
		}
	}
	if operationIndex < 0 {
		return whole
	}
	code := graphqlCodeOnly(document)
	fragments := map[string]graphqlDefinition{}
	for _, definition := range definitions {
		if definition.Kind == "fragment" && definition.Name != "" {
			fragments[definition.Name] = definition
		}
	}
	used := map[string]bool{}
	spreadPattern := regexp.MustCompile(`\.\.\.\s*([_A-Za-z][_0-9A-Za-z]*)`)
	var visit func(graphqlDefinition)
	visit = func(definition graphqlDefinition) {
		for _, match := range spreadPattern.FindAllStringSubmatch(code[definition.From:definition.To], -1) {
			name := match[1]
			fragment, ok := fragments[name]
			if name == "on" || !ok || used[name] {
				continue
			}
			used[name] = true
			visit(fragment)
		}
	}
	visit(definitions[operationIndex])
	sent := []graphqlDefinition{}
	for index, definition := range definitions {
		if index == operationIndex || definition.Kind == "fragment" && used[definition.Name] {
			sent = append(sent, definition)
		}
	}
	texts := make([]string, 0, len(sent))
	ranges := make([]graphqlRange, 0, len(sent))
	for _, definition := range sent {
		texts = append(texts, document[definition.From:definition.To])
		ranges = append(ranges, graphqlRange{From: definition.From, To: definition.To})
	}
	return graphqlQuerySelectionContext{Text: strings.Join(texts, "\n\n"), Ranges: ranges}
}

func graphqlQueryText(document string, selection *graphqlRange) string {
	return graphqlQueryContext(document, selection).Text
}

func graphqlOutsideRanges(length int, ranges []graphqlRange) []graphqlRange {
	sorted := append([]graphqlRange(nil), ranges...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From < sorted[j].From })
	out := []graphqlRange{}
	position := 0
	for _, current := range sorted {
		if current.From > position {
			out = append(out, graphqlRange{From: position, To: current.From})
		}
		if current.To > position {
			position = current.To
		}
	}
	if position < length {
		out = append(out, graphqlRange{From: position, To: length})
	}
	return out
}

func graphqlResponseErrorCount(body string) int {
	if strings.TrimSpace(body) == "" {
		return 0
	}
	var parsed struct {
		Errors []any `json:"errors"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return 0
	}
	return len(parsed.Errors)
}

func graphqlJSONWithoutTemplates(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var out strings.Builder
	inString, escaped := false, false
	for _, char := range string(raw) {
		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			if char == '{' {
				out.WriteString(`\u007b`)
				continue
			}
			if char == '}' {
				out.WriteString(`\u007d`)
				continue
			}
		} else if char == '"' {
			inString = true
		}
		out.WriteRune(char)
	}
	return out.String()
}

func graphqlHTTPRequestText(endpoint graphqlEndpoint, query string, variables map[string]any) string {
	payload := map[string]any{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	headers := map[string]string{"Content-Type": "application/json"}
	for key, value := range endpoint.Headers {
		headers[key] = value
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		if name != "Content-Type" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append([]string{"Content-Type"}, names...)
	var out strings.Builder
	out.WriteString("POST ")
	out.WriteString(endpoint.URL)
	out.WriteByte('\n')
	for _, name := range names {
		out.WriteString(name)
		out.WriteString(": ")
		out.WriteString(headers[name])
		out.WriteByte('\n')
	}
	out.WriteByte('\n')
	out.WriteString(graphqlJSONWithoutTemplates(payload))
	out.WriteByte('\n')
	return out.String()
}

func graphqlParseVariables(text string) (map[string]any, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Variables must be a JSON object")
	}
	return object, nil
}

func graphqlFormatBody(body string) string {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, []byte(body), "", "  "); err != nil {
		return body
	}
	return formatted.String()
}

func graphqlFormatSize(size int) string {
	if size < 1024 {
		return strconv.Itoa(size) + " bytes"
	}
	return fmt.Sprintf("%.1f kb", float64(size)/1024)
}

type graphqlSchemaOfFn func([]graphqlSchemaProject, string) *graphqlSchemaProject

func graphqlSchemaOf(projects []graphqlSchemaProject, path string) *graphqlSchemaProject {
	path = graphqlSlash(path)
	for index := range projects {
		project := &projects[index]
		for _, file := range project.Files {
			if graphqlSlash(file.Path) == path {
				return project
			}
		}
	}
	directory := graphqlSlash(filepath.Dir(path))
	var best *graphqlSchemaProject
	for index := range projects {
		project := &projects[index]
		if project.ConfigPath == "" {
			continue
		}
		if directory == project.Root || strings.HasPrefix(directory, project.Root+"/") {
			if best == nil || len(project.Root) > len(best.Root) {
				best = project
			}
		}
	}
	return best
}

const graphqlIntrospectionQuery = `query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types { ...FullType }
    directives { name description locations args { ...InputValue } }
  }
}
fragment FullType on __Type {
  kind name description
  fields(includeDeprecated: true) {
    name description args { ...InputValue } type { ...TypeRef } isDeprecated deprecationReason
  }
  inputFields { ...InputValue }
  interfaces { ...TypeRef }
  enumValues(includeDeprecated: true) { name description isDeprecated deprecationReason }
  possibleTypes { ...TypeRef }
}
fragment InputValue on __InputValue { name description type { ...TypeRef } defaultValue }
fragment TypeRef on __Type {
  kind name ofType { kind name ofType { kind name ofType { kind name ofType {
    kind name ofType { kind name ofType { kind name ofType { kind name } } } } } } }
}`

type graphqlTypeRef struct {
	Kind   string          `json:"kind"`
	Name   *string         `json:"name"`
	OfType *graphqlTypeRef `json:"ofType"`
}

type graphqlInputValue struct {
	Name         string         `json:"name"`
	Description  *string        `json:"description"`
	Type         graphqlTypeRef `json:"type"`
	DefaultValue *string        `json:"defaultValue"`
}

type graphqlDeprecation struct {
	IsDeprecated      bool    `json:"isDeprecated"`
	DeprecationReason *string `json:"deprecationReason"`
}

type graphqlField struct {
	Name        string              `json:"name"`
	Description *string             `json:"description"`
	Args        []graphqlInputValue `json:"args"`
	Type        graphqlTypeRef      `json:"type"`
	graphqlDeprecation
}

type graphqlEnumValue struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	graphqlDeprecation
}

type graphqlFullType struct {
	Kind          string              `json:"kind"`
	Name          string              `json:"name"`
	Description   *string             `json:"description"`
	Fields        []graphqlField      `json:"fields"`
	InputFields   []graphqlInputValue `json:"inputFields"`
	Interfaces    []graphqlTypeRef    `json:"interfaces"`
	EnumValues    []graphqlEnumValue  `json:"enumValues"`
	PossibleTypes []graphqlTypeRef    `json:"possibleTypes"`
}

type graphqlNamedType struct {
	Name string `json:"name"`
}

type graphqlDirective struct {
	Name        string              `json:"name"`
	Description *string             `json:"description"`
	Locations   []string            `json:"locations"`
	Args        []graphqlInputValue `json:"args"`
}

type graphqlIntrospectionSchema struct {
	QueryType        *graphqlNamedType  `json:"queryType"`
	MutationType     *graphqlNamedType  `json:"mutationType"`
	SubscriptionType *graphqlNamedType  `json:"subscriptionType"`
	Types            []graphqlFullType  `json:"types"`
	Directives       []graphqlDirective `json:"directives"`
}

type graphqlIntrospectionResponse struct {
	Data struct {
		Schema *graphqlIntrospectionSchema `json:"__schema"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func graphqlTypeRefString(ref graphqlTypeRef) string {
	if ref.Kind == "NON_NULL" && ref.OfType != nil {
		return graphqlTypeRefString(*ref.OfType) + "!"
	}
	if ref.Kind == "LIST" && ref.OfType != nil {
		return "[" + graphqlTypeRefString(*ref.OfType) + "]"
	}
	if ref.Name != nil {
		return *ref.Name
	}
	return ""
}

func graphqlQuoted(text string) string {
	raw, _ := json.Marshal(text)
	return string(raw)
}

func graphqlPrintDescription(text *string, indent string) string {
	if text == nil || *text == "" {
		return ""
	}
	if !strings.Contains(*text, "\n") {
		return indent + graphqlQuoted(*text) + "\n"
	}
	lines := strings.Split(strings.ReplaceAll(*text, `"""`, `\"""`), "\n")
	for index, line := range lines {
		if line != "" {
			lines[index] = indent + line
		}
	}
	return indent + `"""` + "\n" + strings.Join(lines, "\n") + "\n" + indent + `"""` + "\n"
}

func graphqlPrintDeprecated(value graphqlDeprecation) string {
	if !value.IsDeprecated {
		return ""
	}
	if value.DeprecationReason != nil && *value.DeprecationReason != "" &&
		*value.DeprecationReason != "No longer supported" {
		return " @deprecated(reason: " + graphqlQuoted(*value.DeprecationReason) + ")"
	}
	return " @deprecated"
}

func graphqlPrintInputValue(value graphqlInputValue) string {
	out := value.Name + ": " + graphqlTypeRefString(value.Type)
	if value.DefaultValue != nil {
		out += " = " + *value.DefaultValue
	}
	return out
}

func graphqlPrintArgs(values []graphqlInputValue) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, graphqlPrintInputValue(value))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func graphqlPrintBlock(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return " {\n" + strings.Join(lines, "\n") + "\n}"
}

func graphqlByName[T any](values []T, name func(T) string) []T {
	out := append([]T(nil), values...)
	sort.Slice(out, func(i, j int) bool { return name(out[i]) < name(out[j]) })
	return out
}

func graphqlPrintType(value graphqlFullType) string {
	const indent = "    "
	head := graphqlPrintDescription(value.Description, "")
	switch value.Kind {
	case "SCALAR":
		return head + "scalar " + value.Name
	case "OBJECT", "INTERFACE":
		implemented := ""
		if len(value.Interfaces) > 0 {
			refs := make([]string, 0, len(value.Interfaces))
			for _, ref := range value.Interfaces {
				refs = append(refs, graphqlTypeRefString(ref))
			}
			implemented = " implements " + strings.Join(refs, " & ")
		}
		fields := graphqlByName(value.Fields, func(field graphqlField) string { return field.Name })
		lines := make([]string, 0, len(fields))
		for _, field := range fields {
			lines = append(lines, graphqlPrintDescription(field.Description, indent)+
				indent+field.Name+graphqlPrintArgs(field.Args)+": "+
				graphqlTypeRefString(field.Type)+graphqlPrintDeprecated(field.graphqlDeprecation))
		}
		kind := "type"
		if value.Kind == "INTERFACE" {
			kind = "interface"
		}
		return head + kind + " " + value.Name + implemented + graphqlPrintBlock(lines)
	case "UNION":
		parts := make([]string, 0, len(value.PossibleTypes))
		for _, ref := range value.PossibleTypes {
			parts = append(parts, graphqlTypeRefString(ref))
		}
		return head + "union " + value.Name + " = " + strings.Join(parts, " | ")
	case "ENUM":
		lines := make([]string, 0, len(value.EnumValues))
		for _, enumValue := range value.EnumValues {
			lines = append(lines, graphqlPrintDescription(enumValue.Description, indent)+
				indent+enumValue.Name+graphqlPrintDeprecated(enumValue.graphqlDeprecation))
		}
		return head + "enum " + value.Name + graphqlPrintBlock(lines)
	case "INPUT_OBJECT":
		inputs := graphqlByName(value.InputFields, func(input graphqlInputValue) string { return input.Name })
		lines := make([]string, 0, len(inputs))
		for _, input := range inputs {
			lines = append(lines, graphqlPrintDescription(input.Description, indent)+
				indent+graphqlPrintInputValue(input))
		}
		return head + "input " + value.Name + graphqlPrintBlock(lines)
	}
	return ""
}

func graphqlIntrospectionToSDL(body string) (string, error) {
	var response graphqlIntrospectionResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return "", err
	}
	schema := response.Data.Schema
	if schema == nil {
		messages := make([]string, 0, len(response.Errors))
		for _, item := range response.Errors {
			messages = append(messages, item.Message)
		}
		if len(messages) == 0 {
			return "", fmt.Errorf("The response has no __schema")
		}
		return "", fmt.Errorf("%s", strings.Join(messages, "\n"))
	}
	roots := []string{}
	if schema.QueryType != nil && schema.QueryType.Name != "" {
		roots = append(roots, "    query: "+schema.QueryType.Name)
	}
	if schema.MutationType != nil && schema.MutationType.Name != "" {
		roots = append(roots, "    mutation: "+schema.MutationType.Name)
	}
	if schema.SubscriptionType != nil && schema.SubscriptionType.Name != "" {
		roots = append(roots, "    subscription: "+schema.SubscriptionType.Name)
	}
	parts := []string{"schema {\n" + strings.Join(roots, "\n") + "\n}"}
	builtinDirectives := map[string]bool{
		"include": true, "skip": true, "deprecated": true, "specifiedBy": true,
	}
	directives := graphqlByName(schema.Directives, func(value graphqlDirective) string { return value.Name })
	for _, directive := range directives {
		if builtinDirectives[directive.Name] {
			continue
		}
		parts = append(parts, graphqlPrintDescription(directive.Description, "")+
			"directive @"+directive.Name+graphqlPrintArgs(directive.Args)+
			" on "+strings.Join(directive.Locations, " | "))
	}
	builtinScalars := map[string]bool{
		"String": true, "Int": true, "Float": true, "Boolean": true, "ID": true,
	}
	types := make([]graphqlFullType, 0, len(schema.Types))
	for _, value := range schema.Types {
		if strings.HasPrefix(value.Name, "__") ||
			value.Kind == "SCALAR" && builtinScalars[value.Name] {
			continue
		}
		types = append(types, value)
	}
	for _, kind := range []string{"INTERFACE", "UNION", "OBJECT", "ENUM", "SCALAR", "INPUT_OBJECT"} {
		matching := []graphqlFullType{}
		for _, value := range types {
			if value.Kind == kind {
				matching = append(matching, value)
			}
		}
		matching = graphqlByName(matching, func(value graphqlFullType) string { return value.Name })
		for _, value := range matching {
			if printed := graphqlPrintType(value); printed != "" {
				parts = append(parts, printed)
			}
		}
	}
	return graphqlGeneratedHeader + "\n\n" + strings.Join(parts, "\n\n") + "\n", nil
}

func graphqlIntrospectionTarget(project graphqlSchemaProject) string {
	file := "schema.graphql"
	for _, pointer := range project.Pointers {
		if !strings.ContainsAny(pointer, "*?{[") {
			file = pointer
			break
		}
	}
	return graphqlSlash(filepath.Join(filepath.FromSlash(project.Root), filepath.FromSlash(file)))
}
