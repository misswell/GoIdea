package app

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// TemplatePrefs is stored separately from the general preferences because
// snippets and file templates are long, multi-line values that users edit as
// documents. Keeping them apart also leaves an unreadable templates.json from
// blocking every ordinary editor preference.
type TemplatePrefs struct {
	Version int                   `json:"version"`
	Live    LiveTemplatePrefs     `json:"live"`
	Postfix PostfixTemplatePrefs  `json:"postfix"`
	Emmet   EmmetTemplatePrefs    `json:"emmet"`
	Files   FileTemplateSchemeSet `json:"files"`
}

type LiveTemplatePrefs struct {
	Enabled       bool           `json:"enabled"`
	CaseSensitive bool           `json:"caseSensitive"`
	Disabled      []string       `json:"disabled,omitempty"`
	Custom        []LiveTemplate `json:"custom,omitempty"`
}

type LiveTemplate struct {
	ID           string   `json:"id"`
	Abbreviation string   `json:"abbreviation"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Body         string   `json:"body"`
	Contexts     []string `json:"contexts,omitempty"`
	Surround     bool     `json:"surround,omitempty"`
}

type PostfixTemplatePrefs struct {
	Enabled  bool     `json:"enabled"`
	Disabled []string `json:"disabled,omitempty"`
}

type EmmetTemplatePrefs struct {
	Enabled bool `json:"enabled"`
}

type FileTemplateSchemeSet struct {
	Default  string                        `json:"default"`
	Schemes  map[string]FileTemplateScheme `json:"schemes"`
	Projects map[string]FileTemplateScheme `json:"projects,omitempty"`
}

type FileTemplateScheme struct {
	Templates []projectNewTemplate `json:"templates"`
	Includes  map[string]string    `json:"includes,omitempty"`
}

func defaultTemplatePrefs() TemplatePrefs {
	return TemplatePrefs{
		Version: 1,
		Live: LiveTemplatePrefs{
			Enabled: true,
		},
		Postfix: PostfixTemplatePrefs{Enabled: true},
		Emmet:   EmmetTemplatePrefs{Enabled: true},
		Files: FileTemplateSchemeSet{
			Default:  "Default",
			Schemes:  map[string]FileTemplateScheme{"Default": defaultFileTemplateScheme()},
			Projects: map[string]FileTemplateScheme{},
		},
	}
}

func defaultFileTemplateScheme() FileTemplateScheme {
	templates := make([]projectNewTemplate, len(projectNewTemplates))
	for i := range projectNewTemplates {
		templates[i] = cloneProjectNewTemplate(projectNewTemplates[i])
	}
	includes := make(map[string]string, len(projectNewIncludes))
	for name, body := range projectNewIncludes {
		includes[name] = body
	}
	return FileTemplateScheme{Templates: templates, Includes: includes}
}

func cloneProjectNewTemplate(t projectNewTemplate) projectNewTemplate {
	t.keywords = strings.TrimSpace(t.keywords)
	t.kinds = append([]projectNewKind(nil), t.kinds...)
	return t
}

func cloneFileTemplateScheme(s FileTemplateScheme) FileTemplateScheme {
	out := FileTemplateScheme{
		Templates: make([]projectNewTemplate, len(s.Templates)),
		Includes:  make(map[string]string, len(s.Includes)),
	}
	for i := range s.Templates {
		out.Templates[i] = cloneProjectNewTemplate(s.Templates[i])
	}
	for name, body := range s.Includes {
		out.Includes[name] = body
	}
	return out
}

func normalizeTemplatePrefs(p TemplatePrefs) TemplatePrefs {
	defaults := defaultTemplatePrefs()
	newPrefs := p.Version <= 0
	if newPrefs {
		p.Version = defaults.Version
	}
	if newPrefs {
		p.Live.Enabled = true
		p.Postfix.Enabled = true
		p.Emmet.Enabled = true
	}
	if p.Files.Schemes == nil || len(p.Files.Schemes) == 0 {
		p.Files.Schemes = defaults.Files.Schemes
	}
	if strings.TrimSpace(p.Files.Default) == "" {
		p.Files.Default = "Default"
	}
	if _, ok := p.Files.Schemes[p.Files.Default]; !ok {
		p.Files.Schemes[p.Files.Default] = defaultFileTemplateScheme()
	}
	if p.Files.Projects == nil {
		p.Files.Projects = map[string]FileTemplateScheme{}
	}
	for name, scheme := range p.Files.Schemes {
		p.Files.Schemes[name] = normalizeFileTemplateScheme(scheme)
	}
	for root, scheme := range p.Files.Projects {
		p.Files.Projects[root] = normalizeFileTemplateScheme(scheme)
	}
	p.Postfix.Disabled = uniqueStrings(p.Postfix.Disabled)
	p.Live.Disabled = uniqueStrings(p.Live.Disabled)
	return p
}

func normalizeFileTemplateScheme(s FileTemplateScheme) FileTemplateScheme {
	if len(s.Templates) == 0 {
		defaults := defaultFileTemplateScheme()
		s.Templates = defaults.Templates
	}
	if s.Includes == nil {
		s.Includes = map[string]string{}
	}
	seen := map[string]bool{}
	out := s.Templates[:0]
	for _, t := range s.Templates {
		t.id = strings.TrimSpace(t.id)
		t.name = strings.TrimSpace(t.name)
		if t.id == "" {
			t.id = fileTemplateID(t.name)
		}
		if t.name == "" || seen[t.id] {
			continue
		}
		seen[t.id] = true
		out = append(out, cloneProjectNewTemplate(t))
	}
	s.Templates = out
	if len(s.Templates) == 0 {
		return defaultFileTemplateScheme()
	}
	return s
}

var fileTemplateIDPattern = regexp.MustCompile(`[^a-z0-9]+`)

func fileTemplateID(name string) string {
	id := strings.Trim(fileTemplateIDPattern.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if id == "" {
		id = "template"
	}
	return id
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s *FileTemplateSchemeSet) effectiveScheme(root string) FileTemplateScheme {
	if root != "" {
		key := canonicalTemplateProject(root)
		if scheme, ok := s.Projects[key]; ok {
			return cloneFileTemplateScheme(normalizeFileTemplateScheme(scheme))
		}
	}
	scheme := s.Schemes[s.Default]
	if len(scheme.Templates) == 0 {
		scheme = defaultFileTemplateScheme()
	}
	return cloneFileTemplateScheme(normalizeFileTemplateScheme(scheme))
}

// ensureProjectScheme snapshots the current Default scheme the first time a
// project uses File and Code Templates. Later Default edits do not silently
// change an already customized project.
func (s *FileTemplateSchemeSet) ensureProjectScheme(root string) FileTemplateScheme {
	scheme := s.effectiveScheme(root)
	if s.Projects == nil {
		s.Projects = map[string]FileTemplateScheme{}
	}
	if root != "" {
		key := canonicalTemplateProject(root)
		if existing, ok := s.Projects[key]; ok {
			return cloneFileTemplateScheme(normalizeFileTemplateScheme(existing))
		}
		s.Projects[key] = cloneFileTemplateScheme(scheme)
	}
	return scheme
}

func (s *FileTemplateSchemeSet) setProjectScheme(root string, scheme FileTemplateScheme) {
	if root == "" {
		return
	}
	if s.Projects == nil {
		s.Projects = map[string]FileTemplateScheme{}
	}
	s.Projects[canonicalTemplateProject(root)] = cloneFileTemplateScheme(normalizeFileTemplateScheme(scheme))
}

func canonicalTemplateProject(root string) string {
	root = filepath.Clean(strings.TrimSpace(root))
	if abs, err := filepath.Abs(root); err == nil {
		return filepath.Clean(abs)
	}
	return root
}

type liveTemplateContext struct {
	Path      string
	Project   string
	FileName  string
	ClassName string
	Package   string
	User      string
	Now       time.Time
	Clipboard string
	Selection string
}

type snippetField struct {
	Name       string
	Start, End int
}

type templateExpansion struct {
	Text   string
	Caret  int
	Fields []snippetField
}

var defaultLiveTemplates = []LiveTemplate{
	{ID: "live.sout", Abbreviation: "sout", Name: "Print to standard output", Contexts: []string{"java"},
		Body: `System.out.println($END$);`},
	{ID: "live.serr", Abbreviation: "serr", Name: "Print to standard error", Contexts: []string{"java"},
		Body: `System.err.println($END$);`},
	{ID: "live.psvm", Abbreviation: "psvm", Name: "Main method", Contexts: []string{"java"},
		Body: "public static void main(String[] args) {\n    $END$\n}"},
	{ID: "live.ifn", Abbreviation: "ifn", Name: "If null", Contexts: []string{"java", "kotlin"},
		Body: "if ($EXPR$ == null) {\n    $END$\n}"},
	{ID: "live.inn", Abbreviation: "inn", Name: "If not null", Contexts: []string{"java", "kotlin"},
		Body: "if ($EXPR$ != null) {\n    $END$\n}"},
	{ID: "live.fori", Abbreviation: "fori", Name: "Indexed for loop", Contexts: []string{"java", "kotlin", "javascript", "typescript"},
		Body: "for (int $I$ = 0; $I$ < $LIMIT$; $I$++) {\n    $END$\n}"},
	{ID: "live.for", Abbreviation: "for", Name: "For each", Contexts: []string{"java", "kotlin", "javascript", "typescript"},
		Body: "for ($ITEM$ : $COLLECTION$) {\n    $END$\n}"},
	{ID: "live.func", Abbreviation: "func", Name: "Function", Contexts: []string{"go"},
		Body: "func $NAME$($PARAMS$) $RETURN$ {\n\t$END$\n}"},
	{ID: "live.iferr", Abbreviation: "iferr", Name: "If error", Contexts: []string{"go"},
		Body: "if err != nil {\n\treturn $END$err\n}"},
	{ID: "live.forr", Abbreviation: "forr", Name: "Range loop", Contexts: []string{"go"},
		Body: "for $INDEX$, $VALUE$ := range $COLLECTION$ {\n\t$END$\n}"},
	{ID: "live.test", Abbreviation: "test", Name: "Test function", Contexts: []string{"go"},
		Body: "func Test$NAME$(t *testing.T) {\n\t$END$\n}"},
	{ID: "live.log", Abbreviation: "log", Name: "Log", Contexts: []string{"javascript", "typescript"},
		Body: `console.log($END$);`},
	{ID: "live.af", Abbreviation: "af", Name: "Arrow function", Contexts: []string{"javascript", "typescript"},
		Body: "($PARAMS$) => {\n  $END$\n}"},
	{ID: "live.imr", Abbreviation: "imr", Name: "Import React", Contexts: []string{"javascript", "typescript", "tsx"},
		Body: "import React from \"react\";\n$END$"},
	{ID: "live.html5", Abbreviation: "html5", Name: "HTML document", Contexts: []string{"html"},
		Body: "<!doctype html>\n<html lang=\"en\">\n<head>\n  <meta charset=\"UTF-8\">\n  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n  <title>$TITLE$</title>\n</head>\n<body>\n$END$\n</body>\n</html>"},
	{ID: "live.php", Abbreviation: "php", Name: "PHP block", Contexts: []string{"php"},
		Body: "<?php\n$END$"},
	{ID: "live.fn", Abbreviation: "fn", Name: "Function", Contexts: []string{"rust"},
		Body: "fn $NAME$($PARAMS$) $RETURN$ {\n    $END$\n}"},
}

var defaultSurroundTemplates = []LiveTemplate{
	{ID: "surround.if", Abbreviation: "if", Name: "Surround with if", Surround: true,
		Contexts: []string{"java", "kotlin", "javascript", "typescript", "go"},
		Body:     "if ($SELECTION$) {\n    $END$\n}"},
	{ID: "surround.for", Abbreviation: "for", Name: "Surround with for", Surround: true,
		Contexts: []string{"java", "kotlin", "javascript", "typescript", "go"},
		Body:     "for ($SELECTION$) {\n    $END$\n}"},
	{ID: "surround.try", Abbreviation: "try", Name: "Surround with try/catch", Surround: true,
		Contexts: []string{"java", "kotlin", "javascript", "typescript"},
		Body:     "try {\n    $SELECTION$\n} catch ($EXCEPTION$) {\n    $END$\n}"},
	{ID: "surround.braces", Abbreviation: "{", Name: "Surround with braces", Surround: true,
		Contexts: []string{"java", "kotlin", "javascript", "typescript", "go", "rust"},
		Body:     "{\n    $SELECTION$\n}"},
	{ID: "surround.parens", Abbreviation: "(", Name: "Surround with parentheses", Surround: true,
		Body: "($SELECTION$)"},
	{ID: "surround.brackets", Abbreviation: "[", Name: "Surround with brackets", Surround: true,
		Body: "[$SELECTION$]"},
	{ID: "surround.quotes", Abbreviation: `"`, Name: "Surround with quotes", Surround: true,
		Body: `"$SELECTION$"`},
}

func (p TemplatePrefs) liveTemplates() []LiveTemplate {
	if !p.Live.Enabled {
		return nil
	}
	disabled := stringSet(p.Live.Disabled)
	byID := map[string]LiveTemplate{}
	for _, t := range defaultLiveTemplates {
		byID[t.ID] = t
	}
	for _, t := range p.Live.Custom {
		if t.ID == "" {
			t.ID = "live." + strings.TrimSpace(t.Abbreviation)
		}
		byID[t.ID] = t
	}
	out := make([]LiveTemplate, 0, len(byID))
	for _, t := range byID {
		if !disabled[t.ID] && strings.TrimSpace(t.Abbreviation) != "" && t.Body != "" {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Abbreviation == out[j].Abbreviation {
			return out[i].Name < out[j].Name
		}
		return out[i].Abbreviation < out[j].Abbreviation
	})
	return out
}

func (p TemplatePrefs) surroundTemplates() []LiveTemplate {
	out := append([]LiveTemplate(nil), defaultSurroundTemplates...)
	for _, t := range p.Live.Custom {
		if t.Surround {
			out = append(out, t)
		}
	}
	return out
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

var templateVariableRE = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)\$|\$\{([A-Za-z_][A-Za-z0-9_]*)(?::([^}]*))?\}`)

// expandTemplate renders a live template body. Repeated variables select the
// same field, $END$ chooses the final caret, and an unset value uses the
// variable's readable name so the inserted code remains editable.
func expandTemplate(body string, ctx liveTemplateContext) templateExpansion {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if ctx.Now.IsZero() {
		ctx.Now = time.Now()
	}
	values := map[string]string{
		"FILE_NAME":  filepath.Base(ctx.FileName),
		"CLASS_NAME": ctx.ClassName,
		"PACKAGE":    ctx.Package,
		"PROJECT":    ctx.Project,
		"USER":       ctx.User,
		"DATE":       ctx.Now.Format("2006-01-02"),
		"TIME":       ctx.Now.Format("15:04"),
		"YEAR":       strconv.Itoa(ctx.Now.Year()),
		"SELECTION":  ctx.Selection,
		"CLIPBOARD":  ctx.Clipboard,
	}
	var out strings.Builder
	occurrences := map[string][]int{}
	var fields []snippetField
	caret := -1
	for i := 0; i < len(body); {
		if body[i] == '\\' && i+1 < len(body) && body[i+1] == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}
		if body[i] != '$' {
			out.WriteByte(body[i])
			i++
			continue
		}
		match := templateVariableRE.FindStringSubmatchIndex(body[i:])
		if match == nil || match[0] != 0 {
			out.WriteByte('$')
			i++
			continue
		}
		end := i + match[1]
		name := body[i+match[2] : i+match[3]]
		if name == "" {
			name = body[i+match[4] : i+match[5]]
		}
		defaultValue := ""
		if match[6] >= 0 {
			defaultValue = body[i+match[6] : i+match[7]]
		}
		i = end
		if name == "END" {
			if caret < 0 {
				caret = out.Len()
			}
			continue
		}
		value, ok := values[name]
		if !ok || value == "" {
			value = defaultValue
		}
		if value == "" {
			value = templateReadableName(name)
		}
		start := out.Len()
		out.WriteString(value)
		endAt := out.Len()
		occurrences[name] = append(occurrences[name], len(fields))
		fields = append(fields, snippetField{Name: name, Start: start, End: endAt})
	}
	text := out.String()
	if caret < 0 {
		if len(fields) > 0 {
			caret = fields[0].Start
		} else {
			caret = len(text)
		}
	}
	return templateExpansion{Text: text, Caret: caret, Fields: fields}
}

func templateReadableName(name string) string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '_' || r == '-' })
	for i := range words {
		if words[i] == "" {
			continue
		}
		runes := []rune(words[i])
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	if len(words) == 0 {
		return name
	}
	return strings.Join(words, " ")
}

func templateLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "javascript"
	case ".ts", ".mts", ".cts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".html", ".htm":
		return "html"
	case ".vue", ".svelte":
		return "html"
	case ".css", ".scss", ".sass", ".less":
		return "css"
	case ".xml":
		return "xml"
	case ".php":
		return "php"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	}
	return strings.TrimPrefix(ext, ".")
}

func templateContextApplies(t LiveTemplate, language string) bool {
	if len(t.Contexts) == 0 {
		return true
	}
	for _, context := range t.Contexts {
		if context == language || context == "any" {
			return true
		}
	}
	return false
}

type postfixCandidate struct {
	Expression string
	From, To   int
	Text       templateExpansion
	TemplateID string
}

type postfixDefinition struct {
	ID      string
	Trigger string
	Detail  string
}

var defaultPostfixTemplates = []postfixDefinition{
	{ID: "postfix.not", Trigger: "not", Detail: "Negate expression"},
	{ID: "postfix.if", Trigger: "if", Detail: "Check expression"},
	{ID: "postfix.else", Trigger: "else", Detail: "Check negated expression"},
	{ID: "postfix.null", Trigger: "null", Detail: "Check for null or nil"},
	{ID: "postfix.nn", Trigger: "nn", Detail: "Check for non-null"},
	{ID: "postfix.ret", Trigger: "ret", Detail: "Return expression"},
	{ID: "postfix.var", Trigger: "var", Detail: "Introduce variable"},
	{ID: "postfix.for", Trigger: "for", Detail: "Iterate expression"},
	{ID: "postfix.fori", Trigger: "fori", Detail: "Indexed iteration"},
	{ID: "postfix.opt", Trigger: "opt", Detail: "Check Optional"},
}

// postfixExpansionCandidates handles the expression before ".not" and also
// the shorter operand suffix, matching the language-aware Choice that
// OpenIDEA shows for postfix templates.
func postfixExpansionCandidates(text string, offset int, language string, prefs PostfixTemplatePrefs, ctx liveTemplateContext) []postfixCandidate {
	if !prefs.Enabled || offset < 2 || offset > len(text) {
		return nil
	}
	if text[offset-1] != '.' && !(offset < len(text) && text[offset] == '.') {
		// Completion requests arrive after the trigger and potentially after
		// part of the template name, for example "value.not".
	}
	dot := strings.LastIndexByte(text[:offset], '.')
	if dot < 0 {
		return nil
	}
	typed := text[dot+1 : offset]
	if typed == "" {
		return nil
	}
	definition := postfixDefinition{}
	found := false
	for _, candidate := range defaultPostfixTemplates {
		if strings.HasPrefix(candidate.Trigger, typed) {
			definition, found = candidate, true
			break
		}
	}
	if !found || stringSet(prefs.Disabled)[definition.ID] {
		return nil
	}
	left := text[:dot]
	lineStart := strings.LastIndexByte(left, '\n') + 1
	fullFrom := lineStart
	for fullFrom < dot && (text[fullFrom] == ' ' || text[fullFrom] == '\t') {
		fullFrom++
	}
	fullExpr := strings.TrimSpace(text[fullFrom:dot])
	if fullExpr == "" {
		return nil
	}
	operandStart := dot
	operandStart = templateExpressionOperandStart(text, operandStart)
	operands := []struct {
		expression string
		from       int
	}{
		{text[operandStart:dot], operandStart},
	}
	if operandStart > fullFrom {
		operands = append(operands, struct {
			expression string
			from       int
		}{fullExpr, fullFrom})
	}
	out := make([]postfixCandidate, 0, len(operands))
	seen := map[string]bool{}
	for _, operand := range operands {
		expression := strings.TrimSpace(operand.expression)
		if expression == "" || seen[expression] {
			continue
		}
		seen[expression] = true
		expansion := applyPostfix(definition.ID, expression, language, ctx)
		out = append(out, postfixCandidate{
			Expression: expression, From: operand.from, To: offset,
			Text: expansion, TemplateID: definition.ID,
		})
	}
	return out
}

func templateExpressionOperandStart(text string, end int) int {
	i := end
	for i > 0 && (text[i-1] == ' ' || text[i-1] == '\t') {
		i--
	}
	depth := 0
	for i > 0 {
		r := rune(text[i-1])
		switch r {
		case ')', ']', '}':
			depth++
			i--
			continue
		case '(', '[', '{':
			if depth > 0 {
				depth--
				i--
				continue
			}
			return i
		}
		if depth == 0 {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' || r == '.' || r == '\'' || r == '"' {
				i--
				continue
			}
			break
		}
		i--
	}
	for i < end && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return i
}

func applyPostfix(id, expression, language string, ctx liveTemplateContext) templateExpansion {
	negative := expression
	if strings.ContainsAny(expression, " \t") || strings.ContainsAny(expression, "<>=&|") {
		negative = "(" + expression + ")"
	}
	switch id {
	case "postfix.not":
		return expandTemplate("!$SELECTION$", withExpr(ctx, negative))
	case "postfix.if":
		return expandTemplate(postfixBlock("$SELECTION$", language), withExpr(ctx, expression))
	case "postfix.else":
		return expandTemplate(postfixBlock("!($SELECTION$)", language), withExpr(ctx, expression))
	case "postfix.null":
		if language == "go" {
			return expandTemplate("if $SELECTION$ == nil {\n\t$END$\n}", withExpr(ctx, expression))
		}
		return expandTemplate("if ($SELECTION$ == null) {\n    $END$\n}", withExpr(ctx, expression))
	case "postfix.nn":
		if language == "go" {
			return expandTemplate("if $SELECTION$ != nil {\n\t$END$\n}", withExpr(ctx, expression))
		}
		return expandTemplate("if ($SELECTION$ != null) {\n    $END$\n}", withExpr(ctx, expression))
	case "postfix.ret":
		return expandTemplate("return $SELECTION$", withExpr(ctx, expression))
	case "postfix.var":
		if language == "go" {
			return expandTemplate("$NAME$ := $SELECTION$\n$END$", withExpr(ctx, expression))
		}
		return expandTemplate("const $NAME$ = $SELECTION$;\n$END$", withExpr(ctx, expression))
	case "postfix.for":
		return expandTemplate("for ($SELECTION$) {\n    $END$\n}", withExpr(ctx, expression))
	case "postfix.fori":
		return expandTemplate("for (int $I$ = 0; $I$ < $SELECTION$; $I$++) {\n    $END$\n}", withExpr(ctx, expression))
	case "postfix.opt":
		if strings.Contains(expression, "Optional") || language == "java" {
			return expandTemplate("if ($SELECTION$.isPresent()) {\n    $END$\n}", withExpr(ctx, expression))
		}
		return expandTemplate("if ($SELECTION$) {\n    $END$\n}", withExpr(ctx, expression))
	}
	return expandTemplate("$SELECTION$", withExpr(ctx, negative))
}

func postfixBlock(expr, language string) string {
	if language == "go" {
		return "if " + expr + " {\n\t$END$\n}"
	}
	return "if " + expr + " {\n    $END$\n}"
}

func withExpr(ctx liveTemplateContext, expr string) liveTemplateContext {
	ctx.Selection = expr
	return ctx
}

type emmetNode struct {
	Tag      string
	ID       string
	Classes  []string
	Attrs    [][2]string
	Text     string
	Count    int
	Children []*emmetNode
}

// expandEmmet handles the HTML forms used by live template completion:
// child (>) and sibling (+) chains, classes, ids, attributes, text and *N.
func expandEmmet(abbreviation string) (string, bool) {
	parser := emmetParser{source: strings.TrimSpace(abbreviation)}
	nodes, ok := parser.parseSiblings()
	if !ok || len(nodes) == 0 || parser.pos != len(parser.source) {
		return "", false
	}
	var out strings.Builder
	for i, node := range nodes {
		if i > 0 {
			out.WriteByte('\n')
		}
		var rendered []string
		emmetRenderNode(node, 0, &rendered)
		out.WriteString(strings.Join(rendered, "\n"))
	}
	return out.String(), true
}

type emmetParser struct {
	source string
	pos    int
}

func (p *emmetParser) parseSiblings() ([]*emmetNode, bool) {
	var nodes []*emmetNode
	for {
		node, ok := p.parseNode()
		if !ok {
			return nil, false
		}
		nodes = append(nodes, node)
		if p.pos >= len(p.source) || p.source[p.pos] != '+' {
			break
		}
		p.pos++
	}
	return nodes, true
}

func (p *emmetParser) parseNode() (*emmetNode, bool) {
	node := &emmetNode{Tag: "div", Count: 1}
	start := p.pos
	for p.pos < len(p.source) && (unicode.IsLetter(rune(p.source[p.pos])) || unicode.IsDigit(rune(p.source[p.pos])) || strings.ContainsRune("-_:$", rune(p.source[p.pos]))) {
		p.pos++
	}
	if p.pos > start {
		node.Tag = strings.ToLower(p.source[start:p.pos])
	}
	for p.pos < len(p.source) {
		switch p.source[p.pos] {
		case '.':
			p.pos++
			value := p.readName()
			if value == "" {
				return nil, false
			}
			node.Classes = append(node.Classes, value)
		case '#':
			p.pos++
			node.ID = p.readName()
			if node.ID == "" {
				return nil, false
			}
		case '[':
			p.pos++
			end := strings.IndexByte(p.source[p.pos:], ']')
			if end < 0 {
				return nil, false
			}
			spec := p.source[p.pos : p.pos+end]
			p.pos += end + 1
			name, value, found := strings.Cut(spec, "=")
			if !found {
				value = name
			}
			value = strings.Trim(value, `"'`)
			node.Attrs = append(node.Attrs, [2]string{strings.TrimSpace(name), value})
		case '{':
			p.pos++
			end := strings.IndexByte(p.source[p.pos:], '}')
			if end < 0 {
				return nil, false
			}
			node.Text = p.source[p.pos : p.pos+end]
			p.pos += end + 1
		case '*':
			p.pos++
			start := p.pos
			for p.pos < len(p.source) && p.source[p.pos] >= '0' && p.source[p.pos] <= '9' {
				p.pos++
			}
			if start == p.pos {
				return nil, false
			}
			node.Count, _ = strconv.Atoi(p.source[start:p.pos])
			node.Count = max(1, min(node.Count, 100))
		case '>':
			p.pos++
			children, ok := p.parseSiblingsUntilChildEnd()
			if !ok {
				return nil, false
			}
			node.Children = children
			return node, true
		default:
			return node, true
		}
	}
	return node, true
}

func (p *emmetParser) parseSiblingsUntilChildEnd() ([]*emmetNode, bool) {
	var nodes []*emmetNode
	for {
		node, ok := p.parseNode()
		if !ok {
			return nil, false
		}
		nodes = append(nodes, node)
		if p.pos >= len(p.source) || p.source[p.pos] != '+' {
			break
		}
		p.pos++
	}
	return nodes, true
}

func (p *emmetParser) readName() string {
	start := p.pos
	for p.pos < len(p.source) {
		r := rune(p.source[p.pos])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ':' || r == '$' {
			p.pos++
			continue
		}
		break
	}
	return p.source[start:p.pos]
}

func emmetRenderNode(node *emmetNode, depth int, out *[]string) {
	for range max(1, node.Count) {
		indent := strings.Repeat("  ", depth)
		open := "<" + node.Tag + emmetAttributes(node)
		void := emmetVoidElements[node.Tag]
		if void {
			*out = append(*out, indent+open+">")
			continue
		}
		if node.Text != "" || len(node.Children) == 0 {
			text := node.Text
			*out = append(*out, indent+open+">"+text+"</"+node.Tag+">")
			continue
		}
		*out = append(*out, indent+open+">")
		for _, child := range node.Children {
			emmetRenderNode(child, depth+1, out)
		}
		*out = append(*out, indent+"</"+node.Tag+">")
	}
}

func emmetAttributes(node *emmetNode) string {
	var parts []string
	if len(node.Classes) > 0 {
		parts = append(parts, `class="`+strings.Join(node.Classes, " ")+`"`)
	}
	if node.ID != "" {
		parts = append(parts, `id="`+node.ID+`"`)
	}
	for _, attr := range node.Attrs {
		if attr[0] == "" {
			continue
		}
		parts = append(parts, attr[0]+`="`+attr[1]+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

var emmetVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

var htmlOpeningTagRE = regexp.MustCompile(`(?s)^<([A-Za-z][A-Za-z0-9:-]*)([^>]*)>`)
var htmlAttrRE = regexp.MustCompile(`([A-Za-z_:][-A-Za-z0-9_:.]*)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+)))?`)

// emmetUpdateTag merges an Emmet abbreviation into an existing opening tag,
// preserving classes and attributes that the abbreviation does not replace.
func emmetUpdateTag(selection, abbreviation string) (string, bool) {
	match := htmlOpeningTagRE.FindStringSubmatchIndex(selection)
	if match == nil {
		return "", false
	}
	tag := selection[match[2]:match[3]]
	attrs := emmetAttributesForSelection(selection[match[4]:match[5]])
	nodes, ok := (&emmetParser{source: strings.TrimSpace(abbreviation)}).parseSiblings()
	if !ok || len(nodes) == 0 {
		return "", false
	}
	update := nodes[0]
	if strings.HasPrefix(strings.TrimSpace(abbreviation), ".") || strings.HasPrefix(strings.TrimSpace(abbreviation), "#") ||
		strings.HasPrefix(strings.TrimSpace(abbreviation), "[") {
		update.Tag = tag
	}
	if len(update.Classes) > 0 {
		attrs.classes = mergeClassNames(attrs.classes, update.Classes)
	}
	if update.ID != "" {
		attrs.id = update.ID
	}
	for _, attr := range update.Attrs {
		if attrs.others == nil {
			attrs.others = map[string]string{}
		}
		attrs.others[attr[0]] = attr[1]
	}
	var rebuilt strings.Builder
	rebuilt.WriteByte('<')
	rebuilt.WriteString(update.Tag)
	if len(attrs.classes) > 0 {
		rebuilt.WriteString(` class="`)
		rebuilt.WriteString(strings.Join(attrs.classes, " "))
		rebuilt.WriteByte('"')
	}
	if attrs.id != "" {
		rebuilt.WriteString(` id="`)
		rebuilt.WriteString(attrs.id)
		rebuilt.WriteByte('"')
	}
	names := make([]string, 0, len(attrs.others))
	for name := range attrs.others {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rebuilt.WriteByte(' ')
		rebuilt.WriteString(name)
		rebuilt.WriteString(`="`)
		rebuilt.WriteString(attrs.others[name])
		rebuilt.WriteByte('"')
	}
	rebuilt.WriteString(selection[match[1]-1 : match[1]])
	openingEnd := match[1]
	updateOpen := rebuilt.String()
	if strings.HasSuffix(updateOpen, "/>") {
		updateOpen = strings.TrimSuffix(updateOpen, "/>") + ">"
	}
	updated := updateOpen + selection[openingEnd:]
	if !emmetVoidElements[tag] {
		closeOld := "</" + tag + ">"
		closeNew := "</" + update.Tag + ">"
		updated = strings.Replace(updated, closeOld, closeNew, 1)
	}
	return updated, true
}

type selectionHTMLAttributes struct {
	classes []string
	id      string
	others  map[string]string
}

func emmetAttributesForSelection(raw string) selectionHTMLAttributes {
	out := selectionHTMLAttributes{others: map[string]string{}}
	for _, match := range htmlAttrRE.FindAllStringSubmatch(raw, -1) {
		name := match[1]
		value := match[2]
		if value == "" {
			value = match[3]
		}
		if value == "" {
			value = match[4]
		}
		switch name {
		case "class":
			out.classes = strings.Fields(value)
		case "id":
			out.id = value
		default:
			out.others[name] = value
		}
	}
	return out
}

type persistedProjectTemplate struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Keywords       string             `json:"keywords,omitempty"`
	Extension      string             `json:"extension,omitempty"`
	NameStyle      string             `json:"nameStyle,omitempty"`
	PackageStyle   string             `json:"packageStyle,omitempty"`
	DottedPackages bool               `json:"dottedPackages,omitempty"`
	DefaultName    string             `json:"defaultName,omitempty"`
	FixedName      string             `json:"fixedName,omitempty"`
	Kinds          []persistedNewKind `json:"kinds,omitempty"`
}

type persistedNewKind struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Body      string `json:"body"`
	Extension string `json:"extension,omitempty"`
}

func (s FileTemplateScheme) MarshalJSON() ([]byte, error) {
	type persistedScheme struct {
		Templates []persistedProjectTemplate `json:"templates"`
		Includes  map[string]string          `json:"includes,omitempty"`
	}
	out := persistedScheme{Includes: s.Includes}
	for _, template := range s.Templates {
		item := persistedProjectTemplate{
			ID: template.id, Name: template.name, Keywords: template.keywords,
			Extension: template.extension, NameStyle: template.nameStyle,
			PackageStyle: template.packageStyle, DottedPackages: template.dottedPackages,
			DefaultName: template.defaultName, FixedName: template.fixedName,
		}
		for _, kind := range template.kinds {
			item.Kinds = append(item.Kinds, persistedNewKind{
				ID: kind.id, Label: kind.label, Body: kind.body, Extension: kind.extension,
			})
		}
		out.Templates = append(out.Templates, item)
	}
	return json.Marshal(out)
}

func (s *FileTemplateScheme) UnmarshalJSON(raw []byte) error {
	var stored struct {
		Templates []persistedProjectTemplate `json:"templates"`
		Includes  map[string]string          `json:"includes,omitempty"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return err
	}
	s.Includes = stored.Includes
	s.Templates = make([]projectNewTemplate, 0, len(stored.Templates))
	for _, item := range stored.Templates {
		template := projectNewTemplate{
			id: item.ID, name: item.Name, keywords: item.Keywords,
			extension: item.Extension, nameStyle: item.NameStyle,
			packageStyle: item.PackageStyle, dottedPackages: item.DottedPackages,
			defaultName: item.DefaultName, fixedName: item.FixedName,
		}
		for _, kind := range item.Kinds {
			template.kinds = append(template.kinds, projectNewKind{
				id: kind.ID, label: kind.Label, body: kind.Body, extension: kind.Extension,
			})
		}
		s.Templates = append(s.Templates, template)
	}
	return nil
}

func mergeClassNames(left, right []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(left)+len(right))
	for _, value := range append(append([]string(nil), left...), right...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func templateMarshal(prefs TemplatePrefs) ([]byte, error) {
	return json.MarshalIndent(normalizeTemplatePrefs(prefs), "", "  ")
}
