package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type springConfig map[string]string

type springJSONField struct {
	name  string
	value any
}

// springJSONObject keeps declaration order in generated request bodies, the
// same way Jackson exposes constructor and field order for the common case.
type springJSONObject struct {
	fields []springJSONField
}

func (o springJSONObject) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, field := range o.fields {
		if i > 0 {
			out.WriteByte(',')
		}
		name, err := json.Marshal(field.name)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(field.value)
		if err != nil {
			return nil, err
		}
		out.Write(name)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

type springFieldDecl struct {
	name     string
	typ      string
	isStatic bool
}

type springClassDecl struct {
	name       string
	superclass string
	fields     []springFieldDecl
	ctorParams []springFieldDecl
}

type springSourceModel struct {
	packageName string
	imports     []string
	classes     []*springClassDecl
}

var (
	springPackageRE   = regexp.MustCompile(`(?m)\bpackage\s+([A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*)\s*;?`)
	springImportRE    = regexp.MustCompile(`(?m)\bimport\s+(?:static\s+)?([A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)+(?:\.\*)?)\s*;?`)
	springTypeDeclRE  = regexp.MustCompile(`\b(?:class|interface|object|record|enum)\s+([A-Za-z_$][\w$]*)`)
	springJavaExtends = regexp.MustCompile(`\bextends\s+([A-Za-z_$][\w$]*(?:\s*<[^{};]+>)?)`)
	springKotlinSuper = regexp.MustCompile(`:\s*([A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*(?:\s*<[^>{}]+>)?)\s*(?:\(|\b)`)
	springPathVarRE   = regexp.MustCompile(`\{(\*?[A-Za-z_][\w.-]*)(?::[^{}]*)?\}`)
	springProfileRE   = regexp.MustCompile(`^[\w.-]+$`)
)

var springTypeModifiers = map[string]bool{
	"abstract": true, "actual": true, "annotation": true, "companion": true,
	"const": true, "data": true, "expect": true, "external": true,
	"final": true, "inline": true, "internal": true, "lateinit": true,
	"open": true, "override": true, "private": true, "protected": true,
	"public": true, "readonly": true, "sealed": true, "static": true,
	"transient": true, "val": true, "value": true, "var": true,
	"volatile": true,
}

// springParseProperties reads a Java .properties file. It keeps the first
// value of a key, as Spring Boot's configuration flattening does here.
func springParseProperties(text string) springConfig {
	out := springConfig{}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		for strings.HasSuffix(line, "\\") && i+1 < len(lines) {
			line = strings.TrimSuffix(line, "\\") + strings.TrimSpace(lines[i+1])
			i++
		}
		split := -1
		escaped := false
		for at, r := range line {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '=' || r == ':' || r == ' ' || r == '\t' {
				split = at
				break
			}
		}
		if split < 0 {
			continue
		}
		key := strings.TrimSpace(line[:split])
		value := strings.TrimSpace(line[split:])
		value = strings.TrimLeft(value, "=: \t")
		if key == "" {
			continue
		}
		key = springUnescapeProperty(key)
		if _, exists := out[key]; !exists {
			out[key] = value
		}
	}
	return out
}

func springUnescapeProperty(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
			out.WriteByte(value[i])
		} else {
			out.WriteByte(value[i])
		}
	}
	return out.String()
}

// springParseYAML reads the block mappings and scalar values of the first
// YAML document. That is the subset OpenIDEA uses to find server.* settings.
func springParseYAML(text string) springConfig {
	out := springConfig{}
	type level struct {
		indent int
		key    string
	}
	var stack []level
	started := false
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), "---") {
			if started {
				break
			}
			continue
		}
		line := springStripYAMLComment(raw)
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") ||
			strings.HasPrefix(strings.TrimSpace(line), "- ") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		colon := strings.IndexByte(strings.TrimSpace(line), ':')
		if colon < 0 {
			continue
		}
		content := strings.TrimSpace(line)
		colon = strings.IndexByte(content, ':')
		key := strings.TrimSpace(content[:colon])
		key = strings.Trim(key, `"'`)
		if key == "" {
			continue
		}
		started = true
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		parts := make([]string, 0, len(stack)+1)
		for _, parent := range stack {
			parts = append(parts, parent.key)
		}
		parts = append(parts, key)
		path := strings.Join(parts, ".")
		value := strings.TrimSpace(content[colon+1:])
		if value == "" || value == "|" || value == ">" {
			stack = append(stack, level{indent: indent, key: key})
			continue
		}
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if _, exists := out[path]; !exists {
			out[path] = value
		}
	}
	return out
}

func springStripYAMLComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'', '"':
			if quote == 0 {
				quote = line[i]
			} else if quote == line[i] {
				quote = 0
			}
		case '#':
			if quote == 0 && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
				return line[:i]
			}
		}
	}
	return line
}

var springPlaceholderRE = regexp.MustCompile(`\$\{[^}:]*(?::([^}]*))?\}`)

func springResolveConfigValue(value string) (string, bool) {
	unknown := false
	resolved := springPlaceholderRE.ReplaceAllStringFunc(value, func(match string) string {
		parts := springPlaceholderRE.FindStringSubmatch(match)
		if len(parts) < 2 || parts[1] == "" && !strings.Contains(match, ":") {
			unknown = true
			return ""
		}
		return parts[1]
	})
	return resolved, !unknown
}

func springConfigValue(configs []springConfig, keys ...string) string {
	for _, config := range configs {
		for _, key := range keys {
			value, ok := springResolveConfigValue(config[key])
			if ok && value != "" {
				return value
			}
		}
	}
	return ""
}

// springHostFromConfig mirrors OpenIDEA's localhost scheme, port and context
// path derivation from Spring Boot configuration.
func springHostFromConfig(configs []springConfig) string {
	port := springConfigValue(configs, "server.port")
	sslEnabled := strings.EqualFold(springConfigValue(configs, "server.ssl.enabled"), "true")
	ssl := sslEnabled || springConfigValue(configs, "server.ssl.key-store") != ""
	scheme := "http"
	if ssl {
		scheme = "https"
	}
	portNumber := 8080
	if value, err := strconv.Atoi(port); err == nil && value > 0 {
		portNumber = value
	}
	context := strings.Join([]string{
		springConfigValue(configs, "server.servlet.context-path", "server.context-path", "spring.webflux.base-path"),
		springConfigValue(configs, "spring.mvc.servlet.path"),
	}, "/")
	for strings.Contains(context, "//") {
		context = strings.ReplaceAll(context, "//", "/")
	}
	context = strings.TrimSuffix(context, "/")
	if context != "" && !strings.HasPrefix(context, "/") {
		context = "/" + context
	}
	return fmt.Sprintf("%s://localhost:%d%s", scheme, portNumber, context)
}

func springConfigCandidates(sourcePath, projectRoot, profile string) []string {
	normal := filepath.ToSlash(sourcePath)
	var roots []string
	if at := strings.Index(normal, "/src/main/"); at >= 0 {
		roots = append(roots, filepath.FromSlash(normal[:at]))
	}
	if projectRoot != "" {
		root := filepath.Clean(projectRoot)
		found := false
		for _, existing := range roots {
			if filepath.Clean(existing) == root {
				found = true
				break
			}
		}
		if !found {
			roots = append(roots, root)
		}
	}
	suffix := "application"
	if profile != "" {
		suffix += "-" + profile
	}
	var paths []string
	for _, root := range roots {
		for _, extension := range []string{"properties", "yml", "yaml"} {
			paths = append(paths,
				filepath.Join(root, "src", "main", "resources", suffix+"."+extension),
			)
		}
	}
	return paths
}

func springReadConfigs(paths []string) []springConfig {
	var configs []springConfig
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.HasSuffix(strings.ToLower(path), ".properties") {
			configs = append(configs, springParseProperties(string(data)))
		} else {
			configs = append(configs, springParseYAML(string(data)))
		}
	}
	return configs
}

// springFindHost uses the controller's module first and the project root
// second. Active profile files override the base configuration in reverse
// profile order.
func springFindHost(sourcePath, projectRoot string) string {
	base := springReadConfigs(springConfigCandidates(sourcePath, projectRoot, ""))
	active := ""
	for _, config := range base {
		if value, ok := springResolveConfigValue(config["spring.profiles.active"]); ok && value != "" {
			active = value
			break
		}
	}
	var profiles []string
	for _, profile := range strings.Split(active, ",") {
		profile = strings.TrimSpace(profile)
		if springProfileRE.MatchString(profile) {
			profiles = append(profiles, profile)
		}
	}
	for left, right := 0, len(profiles)-1; left < right; left, right = left+1, right-1 {
		profiles[left], profiles[right] = profiles[right], profiles[left]
	}
	var profiled []springConfig
	for _, profile := range profiles {
		profiled = append(profiled, springReadConfigs(springConfigCandidates(sourcePath, projectRoot, profile))...)
	}
	return springHostFromConfig(append(profiled, base...))
}

func springBareType(typ string) string {
	typ = strings.TrimSpace(typ)
	typ = strings.TrimSuffix(typ, "?")
	for {
		open := strings.IndexByte(typ, '<')
		if open < 0 || !strings.HasSuffix(typ, ">") {
			break
		}
		name := strings.TrimSpace(typ[:open])
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			name = name[dot+1:]
		}
		switch name {
		case "Mono", "Optional", "HttpEntity", "RequestEntity", "Deferred", "CompletableFuture":
			typ = strings.TrimSpace(typ[open+1 : len(typ)-1])
			continue
		}
		break
	}
	for {
		space := strings.IndexAny(typ, " \t\n")
		if space <= 0 || !springTypeModifiers[strings.TrimSpace(typ[:space])] {
			break
		}
		typ = strings.TrimSpace(typ[space+1:])
	}
	for {
		dot := strings.IndexByte(typ, '.')
		if dot <= 0 {
			break
		}
		allPackage := true
		for _, part := range strings.Split(typ[:dot], ".") {
			if part == "" || part[0] < 'a' || part[0] > 'z' {
				allPackage = false
				break
			}
		}
		if !allPackage {
			break
		}
		typ = typ[dot+1:]
	}
	return typ
}

var (
	springNumberType = regexp.MustCompile(`^(?:byte|short|int|long|float|double|Byte|Short|Integer|Long|Float|Double|Int|BigDecimal|BigInteger|Number|UByte|UShort|UInt|ULong)\??$`)
	springBoolType   = regexp.MustCompile(`^(?:boolean|Boolean)\??$`)
	springStringType = regexp.MustCompile(`^(?:String|CharSequence|char|Character|Char|UUID|LocalDate|LocalDateTime|LocalTime|Instant|OffsetDateTime|ZonedDateTime|Date|Duration|URI|URL)\??$`)
	springListType   = regexp.MustCompile(`^(?:(?:List|ArrayList|LinkedList|Set|HashSet|TreeSet|LinkedHashSet|Collection|Iterable|MutableList|MutableSet|Flux|Array)(?:<.*>)?\??|.*Array(?:<.*>)?\??|\[\]|\.\.\.)$`)
	springMapType    = regexp.MustCompile(`^(?:Map|HashMap|LinkedHashMap|TreeMap|MutableMap|MultiValueMap|JsonNode|ObjectNode)(?:<.*>)?\??$`)
)

func springSkeleton(typ string, lookup func(string) *springClassDecl, depth int) any {
	bare := springBareType(typ)
	switch {
	case springNumberType.MatchString(bare):
		return 0
	case springBoolType.MatchString(bare):
		return false
	case springStringType.MatchString(bare):
		return ""
	case springListType.MatchString(bare):
		return []any{}
	case springMapType.MatchString(bare), bare == "Object", bare == "Any", bare == "":
		return springJSONObject{}
	}
	if at := strings.IndexByte(bare, '<'); at >= 0 {
		bare = bare[:at]
	}
	class := lookup(bare)
	if class == nil || depth > 1 {
		return springJSONObject{}
	}
	var chain []*springClassDecl
	for current := class; current != nil && len(chain) <= 5; {
		already := false
		for _, item := range chain {
			if item == current {
				already = true
				break
			}
		}
		if already {
			break
		}
		chain = append([]*springClassDecl{current}, chain...)
		parent := springBareType(current.superclass)
		if at := strings.IndexByte(parent, '<'); at >= 0 {
			parent = parent[:at]
		}
		if parent == "" {
			break
		}
		current = lookup(parent)
	}
	out := springJSONObject{}
	seen := map[string]bool{}
	for _, item := range chain {
		for _, field := range append(append([]springFieldDecl(nil), item.ctorParams...), item.fields...) {
			if field.name == "" || field.isStatic || seen[field.name] {
				continue
			}
			seen[field.name] = true
			out.fields = append(out.fields, springJSONField{
				name: field.name, value: springSkeleton(field.typ, lookup, depth+1),
			})
		}
	}
	return out
}

func springPathVariables(path string) string {
	return springPathVarRE.ReplaceAllStringFunc(path, func(match string) string {
		parts := springPathVarRE.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		return "{{" + strings.TrimPrefix(parts[1], "*") + "}}"
	})
}

func springRenderRequest(endpoint springEndpoint, host, path string, body any) string {
	if path == "" && len(endpoint.paths) > 0 {
		path = endpoint.paths[0]
	}
	path = springPathVariables(path)
	query := make([]string, 0, len(endpoint.query))
	for _, parameter := range endpoint.query {
		value := "{{" + parameter.name + "}}"
		if parameter.defaultVal != nil {
			value = *parameter.defaultVal
		}
		query = append(query, parameter.name+"="+value)
	}
	lines := []string{
		"### " + endpoint.className + "." + endpoint.methodName,
		endpoint.httpMethod + " " + strings.TrimSuffix(host, "/") + path,
	}
	if len(query) > 0 {
		lines[1] += "?" + strings.Join(query, "&")
	}
	headers := append([]springHeader(nil), endpoint.headers...)
	contentType := ""
	if endpoint.consumes != nil {
		contentType = *endpoint.consumes
	} else if endpoint.bodyType != nil {
		contentType = "application/json"
	}
	if endpoint.produces != nil {
		headers = append([]springHeader{{name: "Accept", value: *endpoint.produces}}, headers...)
	}
	if contentType != "" && (endpoint.bodyType != nil || endpoint.consumes != nil) {
		headers = append(headers, springHeader{name: "Content-Type", value: contentType})
	}
	for _, header := range headers {
		lines = append(lines, header.name+": "+header.value)
	}
	if endpoint.bodyType != nil {
		lines = append(lines, "")
		if body == nil {
			body = springJSONObject{}
		}
		if contentType == "application/x-www-form-urlencoded" {
			if object, ok := body.(springJSONObject); ok {
				var fields []string
				for _, field := range object.fields {
					fields = append(fields, field.name+"=")
				}
				lines = append(lines, strings.Join(fields, "&"))
			} else {
				encoded, _ := json.Marshal(body)
				lines = append(lines, string(encoded))
			}
		} else {
			encoded, err := json.MarshalIndent(body, "", "  ")
			if err != nil {
				encoded = []byte("{}")
			}
			lines = append(lines, string(encoded))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func springAppendRequests(existing string, blocks []string) (insert string, from, caret int) {
	from = len(existing)
	separator := ""
	if strings.TrimSpace(existing) != "" {
		switch {
		case strings.HasSuffix(existing, "\n\n"):
		case strings.HasSuffix(existing, "\n"):
			separator = "\n"
		default:
			separator = "\n\n"
		}
	}
	insert = separator + strings.Join(blocks, "\n")
	caret = from + len(separator)
	if len(blocks) > 0 {
		if line := strings.IndexByte(blocks[0], '\n'); line >= 0 {
			caret += line + 1
		}
	}
	return insert, from, caret
}

// springParseSourceModel is the declaration-level reader used for generated
// JSON bodies. It is intentionally limited to class fields, constructor
// parameters and the immediate superclass.
func springParseSourceModel(text string) springSourceModel {
	mask := springMaskedSource(text)
	model := springSourceModel{}
	if match := springPackageRE.FindStringSubmatch(mask); len(match) > 1 {
		model.packageName = match[1]
	}
	for _, match := range springImportRE.FindAllStringSubmatch(mask, -1) {
		if len(match) > 1 {
			model.imports = append(model.imports, match[1])
		}
	}
	types := springTypeDeclRE.FindAllStringSubmatchIndex(mask, -1)
	for typeIndex, match := range types {
		nameStart, nameEnd := match[2], match[3]
		name := mask[nameStart:nameEnd]
		limit := len(mask)
		if typeIndex+1 < len(types) {
			limit = types[typeIndex+1][0]
		}
		headerEnd, bodyOpen, ctorOpen, ctorClose := springClassDeclaration(mask, nameEnd, limit)
		bodyClose := -1
		if bodyOpen >= 0 {
			bodyClose = springMatchingDelimiter(mask, bodyOpen, '{', '}')
			if bodyClose < 0 {
				continue
			}
		}
		header := mask[nameEnd:headerEnd]
		class := &springClassDecl{name: name}
		if super := springJavaExtends.FindStringSubmatch(header); len(super) > 1 {
			class.superclass = strings.TrimSpace(super[1])
		} else if super := springKotlinSuper.FindStringSubmatch(header); len(super) > 1 {
			class.superclass = strings.TrimSpace(super[1])
		}
		if ctorOpen >= 0 && ctorClose > ctorOpen {
			for _, part := range springSplit(mask[ctorOpen+1:ctorClose], ',') {
				if field, ok := springParseClassField(part); ok {
					class.ctorParams = append(class.ctorParams, field)
				}
			}
		}
		if bodyOpen >= 0 && bodyClose > bodyOpen {
			body := text[bodyOpen+1 : bodyClose]
			bodyMask := mask[bodyOpen+1 : bodyClose]
			class.fields = springFieldsInClass(text, mask, name, body, bodyMask, bodyOpen+1)
		}
		model.classes = append(model.classes, class)
	}
	return model
}

// springClassDeclaration finds the primary constructor and optional body of
// a Java or Kotlin class without confusing a following top-level function's
// brace for the class body. Kotlin permits a primary-constructor-only class,
// so a missing body is a valid declaration.
func springClassDeclaration(mask string, nameEnd, limit int) (
	headerEnd, bodyOpen, ctorOpen, ctorClose int,
) {
	bodyOpen = -1
	ctorOpen = -1
	ctorClose = -1
	headerEnd = limit
	for pos := nameEnd; pos < limit; {
		switch mask[pos] {
		case '<':
			close := springMatchingDelimiter(mask, pos, '<', '>')
			if close < 0 {
				return limit, -1, -1, -1
			}
			pos = close + 1
		case '(':
			close := springMatchingDelimiter(mask, pos, '(', ')')
			if close < 0 {
				return limit, -1, -1, -1
			}
			if ctorOpen < 0 {
				ctorOpen, ctorClose = pos, close
			}
			pos = close + 1
		case '{':
			return pos, pos, ctorOpen, ctorClose
		case ';':
			return pos, -1, ctorOpen, ctorClose
		case '\n':
			next := springNextNonSpace(mask, pos+1)
			if next >= limit || mask[next] == '{' || mask[next] == ':' {
				pos = next
				continue
			}
			wordEnd := next
			for wordEnd < limit && ((mask[wordEnd] >= 'a' && mask[wordEnd] <= 'z') ||
				(mask[wordEnd] >= 'A' && mask[wordEnd] <= 'Z')) {
				wordEnd++
			}
			switch mask[next:wordEnd] {
			case "extends", "implements", "constructor":
				pos = next
			default:
				return max(nameEnd, pos), -1, ctorOpen, ctorClose
			}
		default:
			pos++
		}
	}
	return max(nameEnd, headerEnd), bodyOpen, ctorOpen, ctorClose
}

func springFieldsInClass(
	text, mask, className, body, bodyMask string,
	bodyStart int,
) []springFieldDecl {
	_ = text
	var fields []springFieldDecl
	seen := map[string]bool{}
	add := func(field springFieldDecl, ok bool) {
		if !ok || field.name == "" || seen[field.name] {
			return
		}
		seen[field.name] = true
		fields = append(fields, field)
	}
	for _, part := range springSplit(body, ';') {
		if strings.Contains(part, "(") && !strings.Contains(part, "=") {
			continue
		}
		add(springParseClassField(part))
	}
	javaCtor := regexp.MustCompile(`\b` + regexp.QuoteMeta(className) + `\s*\(`)
	if loc := javaCtor.FindStringIndex(bodyMask); loc != nil {
		open := loc[1] - 1
		if close := springMatchingDelimiter(bodyMask, open, '(', ')'); close > open {
			for _, part := range springSplit(body[open+1:close], ',') {
				add(springParseClassField(part))
			}
		}
	}
	lineOffset := 0
	for _, line := range strings.Split(bodyMask, "\n") {
		if depth := springNestingDepth(bodyMask, 0, lineOffset); depth == 0 {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "val ") || strings.HasPrefix(trimmed, "var ") ||
				strings.Contains(trimmed, " val ") || strings.Contains(trimmed, " var ") {
				add(springParseClassField(strings.TrimSuffix(trimmed, ";")))
			}
		}
		lineOffset += len(line) + 1
	}
	_ = bodyStart
	return fields
}

func springParseClassField(part string) (springFieldDecl, bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return springFieldDecl{}, false
	}
	annotations := springAnnotations(part, springMaskedSource(part))
	if len(annotations) > 0 {
		clean := []byte(part)
		for _, annotation := range annotations {
			for i := annotation.start; i < annotation.end && i < len(clean); i++ {
				if clean[i] != '\n' && clean[i] != '\r' {
					clean[i] = ' '
				}
			}
		}
		part = strings.TrimSpace(string(clean))
	}
	if at := springTopLevelIndex(part, '='); at >= 0 {
		part = strings.TrimSpace(part[:at])
	}
	if strings.HasPrefix(part, "(") || strings.HasPrefix(part, ")") ||
		strings.Contains(part, "->") {
		return springFieldDecl{}, false
	}
	isStatic := false
	for mention := range strings.FieldsSeq(part) {
		if mention == "static" || mention == "const" {
			isStatic = true
		}
	}
	for {
		space := strings.IndexAny(part, " \t\n")
		if space <= 0 {
			break
		}
		word := strings.TrimSpace(part[:space])
		if !springTypeModifiers[word] {
			break
		}
		part = strings.TrimSpace(part[space+1:])
	}
	if colon := springTopLevelIndex(part, ':'); colon >= 0 {
		left := strings.TrimSpace(part[:colon])
		leftFields := strings.Fields(left)
		if len(leftFields) == 0 {
			return springFieldDecl{}, false
		}
		name := leftFields[len(leftFields)-1]
		typ := strings.TrimSpace(part[colon+1:])
		if !springIdentifier(name) || typ == "" {
			return springFieldDecl{}, false
		}
		return springFieldDecl{name: name, typ: typ, isStatic: isStatic}, true
	}
	fields := strings.Fields(part)
	if len(fields) < 2 {
		return springFieldDecl{}, false
	}
	name := fields[len(fields)-1]
	name = strings.TrimSuffix(name, ",")
	typ := strings.Join(fields[:len(fields)-1], " ")
	if !springIdentifier(name) || strings.ContainsAny(typ, "()") || typ == "" {
		return springFieldDecl{}, false
	}
	return springFieldDecl{name: name, typ: typ, isStatic: isStatic}, true
}

func springIdentifier(value string) bool {
	if value == "" || !((value[0] >= 'A' && value[0] <= 'Z') ||
		(value[0] >= 'a' && value[0] <= 'z') || value[0] == '_' || value[0] == '$') {
		return false
	}
	for i := 1; i < len(value); i++ {
		ch := value[i]
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '$') {
			return false
		}
	}
	return true
}

type springProjectIndex struct {
	root   string
	byName map[string][]string
	cache  map[string]springSourceModel
}

func springNewProjectIndex(root string) *springProjectIndex {
	index := &springProjectIndex{
		root: root, byName: map[string][]string{}, cache: map[string]springSourceModel{},
	}
	if root == "" {
		return index
	}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".gradle", ".idea", "build", "node_modules", "out", "target":
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".java" || extension == ".kt" {
			index.byName[entry.Name()] = append(index.byName[entry.Name()], path)
		}
		return nil
	})
	for name := range index.byName {
		sort.Strings(index.byName[name])
	}
	return index
}

func (i *springProjectIndex) read(path string) (springSourceModel, bool) {
	if model, ok := i.cache[path]; ok {
		return model, true
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return springSourceModel{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return springSourceModel{}, false
	}
	model := springParseSourceModel(string(data))
	i.cache[path] = model
	return model, true
}

type springClassFound struct {
	class *springClassDecl
	path  string
	model springSourceModel
}

func (i *springProjectIndex) class(name string, fromPath string, from springSourceModel) *springClassDecl {
	return i.find(name, fromPath, from).class
}

func (i *springProjectIndex) find(name string, fromPath string, from springSourceModel) springClassFound {
	for _, class := range from.classes {
		if class.name == name {
			return springClassFound{class: class, path: fromPath, model: from}
		}
	}
	type candidate struct {
		pkg  string
		path string
	}
	var candidates []candidate
	for _, imported := range from.imports {
		if imported == from.packageName+".*" {
			continue
		}
		if strings.HasSuffix(imported, "."+name) {
			pkg := strings.TrimSuffix(imported, "."+name)
			candidates = append(candidates, candidate{pkg: pkg})
		}
	}
	packages := []string{from.packageName}
	for _, imported := range from.imports {
		if strings.HasSuffix(imported, ".*") {
			packages = append(packages, strings.TrimSuffix(imported, ".*"))
		}
	}
	for _, pkg := range packages {
		candidates = append(candidates, candidate{pkg: pkg})
	}
	extensions := []string{".java", ".kt"}
	if strings.HasSuffix(strings.ToLower(fromPath), ".kt") {
		extensions = []string{".kt", ".java"}
	}
	sourceRoot := springSourceRoot(fromPath, from.packageName)
	for _, entry := range candidates {
		var directories []string
		if entry.pkg != "" {
			for _, root := range []string{sourceRoot, i.root} {
				if root == "" {
					continue
				}
				directories = append(directories, filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(entry.pkg, ".", "/"))))
			}
		}
		directories = append(directories, filepath.Dir(fromPath))
		for _, directory := range directories {
			for _, extension := range extensions {
				path := filepath.Join(directory, name+extension)
				if model, ok := i.read(path); ok {
					for _, class := range model.classes {
						if class.name == name {
							return springClassFound{class: class, path: path, model: model}
						}
					}
				}
			}
		}
	}
	for _, extension := range extensions {
		paths := i.byName[name+extension]
		for _, path := range paths {
			matchesPackage := false
			for _, entry := range candidates {
				if entry.pkg == "" {
					continue
				}
				suffix := "/" + strings.ReplaceAll(entry.pkg, ".", "/") + "/" + name + extension
				if strings.HasSuffix(filepath.ToSlash(path), suffix) {
					matchesPackage = true
					break
				}
			}
			if !matchesPackage && from.packageName != "" {
				continue
			}
			if model, ok := i.read(path); ok {
				for _, class := range model.classes {
					if class.name == name {
						return springClassFound{class: class, path: path, model: model}
					}
				}
			}
		}
	}
	return springClassFound{}
}

func springSourceRoot(path, packageName string) string {
	root := filepath.Dir(path)
	for range strings.Split(strings.Trim(packageName, "."), ".") {
		if packageName == "" {
			break
		}
		root = filepath.Dir(root)
	}
	return root
}

// springBodyLookup resolves @RequestBody classes from the controller file,
// imports, its source root, and finally the project source index.
func springBodyLookup(
	sourcePath, projectRoot, text string,
	endpoints []springEndpoint,
) func(string) *springClassDecl {
	index := springNewProjectIndex(projectRoot)
	source := springParseSourceModel(text)
	index.cache[sourcePath] = source
	known := map[string]springClassFound{}
	loading := map[string]bool{}
	var load func(string, springClassFound) springClassFound
	load = func(name string, from springClassFound) springClassFound {
		name = springBareType(name)
		if at := strings.IndexByte(name, '<'); at >= 0 {
			name = name[:at]
		}
		if found, ok := known[name]; ok {
			return found
		}
		if name == "" || loading[name] {
			return springClassFound{}
		}
		loading[name] = true
		found := index.find(name, from.path, from.model)
		if found.class == nil {
			delete(loading, name)
			return springClassFound{}
		}
		known[name] = found
		if parent := springBareType(found.class.superclass); parent != "" && !springNumberType.MatchString(parent) {
			load(parent, found)
		}
		for _, field := range append(append([]springFieldDecl(nil), found.class.ctorParams...), found.class.fields...) {
			fieldType := springBareType(field.typ)
			if springNumberType.MatchString(fieldType) || springBoolType.MatchString(fieldType) ||
				springStringType.MatchString(fieldType) || springListType.MatchString(fieldType) ||
				springMapType.MatchString(fieldType) {
				continue
			}
			load(fieldType, found)
		}
		delete(loading, name)
		return found
	}
	controller := springClassFound{path: sourcePath, model: source}
	for _, endpoint := range endpoints {
		if endpoint.bodyType != nil {
			load(*endpoint.bodyType, controller)
		}
	}
	return func(name string) *springClassDecl {
		name = springBareType(name)
		if at := strings.IndexByte(name, '<'); at >= 0 {
			name = name[:at]
		}
		if found, ok := known[name]; ok {
			return found.class
		}
		return load(name, controller).class
	}
}
