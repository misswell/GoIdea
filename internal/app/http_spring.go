package app

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type springAnnotation struct {
	name       string
	args       string
	start, end int
}

type springParam struct {
	name        string
	typ         string
	annotations []springAnnotation
}

type springQueryParam struct {
	name       string
	defaultVal *string
}

type springHeader struct {
	name  string
	value string
}

type springEndpoint struct {
	className   string
	methodName  string
	offset      int
	start, end  int
	classOffset int
	httpMethod  string
	paths       []string
	query       []springQueryParam
	headers     []springHeader
	consumes    *string
	produces    *string
	bodyType    *string
}

type springController struct {
	name       string
	offset     int
	start, end int
	endpoints  []springEndpoint
}

var (
	springAnnotationRE  = regexp.MustCompile(`@\s*(?:(?:[A-Za-z_$][\w$]*\.)+)?([A-Za-z_$][\w$]*)`)
	springClassRE       = regexp.MustCompile(`\b(?:class|interface|object|record|enum)\s+([A-Za-z_$][\w$]*)`)
	springQuoteRE       = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
	springTemplateRE    = regexp.MustCompile(`\$\{([A-Za-z_$][\w$]*)\}|\$([A-Za-z_$][\w$]*)`)
	springJavaConstRE   = regexp.MustCompile(`(?m)(?:static\s+final|final\s+static)\s+String\s+([A-Za-z_$][\w$]*)\s*=\s*([^;]+);`)
	springKotlinConstRE = regexp.MustCompile(`(?m)\b(?:const\s+)?val\s+([A-Za-z_$][\w$]*)\s*(?::\s*String)?[ \t]*=[ \t]*([^;\n]+)`)
)

var springMappingMethods = map[string]string{
	"GetMapping":    "GET",
	"PostMapping":   "POST",
	"PutMapping":    "PUT",
	"DeleteMapping": "DELETE",
	"PatchMapping":  "PATCH",
}

var springControllerAnnotations = map[string]bool{
	"Controller": true, "RestController": true,
}

var springMediaTypes = map[string]string{
	"ALL":                          "*/*",
	"APPLICATION_JSON":             "application/json",
	"APPLICATION_JSON_UTF8":        "application/json;charset=UTF-8",
	"APPLICATION_XML":              "application/xml",
	"APPLICATION_ATOM_XML":         "application/atom+xml",
	"APPLICATION_FORM_URLENCODED":  "application/x-www-form-urlencoded",
	"APPLICATION_OCTET_STREAM":     "application/octet-stream",
	"APPLICATION_PDF":              "application/pdf",
	"APPLICATION_PROBLEM_JSON":     "application/problem+json",
	"APPLICATION_PROBLEM_XML":      "application/problem+xml",
	"APPLICATION_NDJSON":           "application/x-ndjson",
	"APPLICATION_STREAM_JSON":      "application/stream+json",
	"APPLICATION_CBOR":             "application/cbor",
	"APPLICATION_GRAPHQL_RESPONSE": "application/graphql-response+json",
	"IMAGE_GIF":                    "image/gif",
	"IMAGE_JPEG":                   "image/jpeg",
	"IMAGE_PNG":                    "image/png",
	"MULTIPART_FORM_DATA":          "multipart/form-data",
	"MULTIPART_MIXED":              "multipart/mixed",
	"TEXT_EVENT_STREAM":            "text/event-stream",
	"TEXT_HTML":                    "text/html",
	"TEXT_MARKDOWN":                "text/markdown",
	"TEXT_PLAIN":                   "text/plain",
	"TEXT_XML":                     "text/xml",
}

// springMaskedSource blanks comments and literals while preserving byte
// offsets. Annotation and declaration scans can then match structure without
// being confused by braces or comments inside strings.
func springMaskedSource(text string) string {
	out := []byte(text)
	blank := func(from, to int) {
		for i := max(0, from); i < min(len(out), to); i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "//"):
			end := strings.IndexByte(text[i+2:], '\n')
			if end < 0 {
				blank(i, len(text))
				i = len(text)
			} else {
				blank(i, i+2+end)
				i += 2 + end
			}
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				blank(i, len(text))
				i = len(text)
			} else {
				blank(i, i+2+end+2)
				i += 2 + end + 2
			}
		case strings.HasPrefix(text[i:], `"""`):
			end := strings.Index(text[i+3:], `"""`)
			if end < 0 {
				blank(i, len(text))
				i = len(text)
			} else {
				blank(i, i+3+end+3)
				i += 3 + end + 3
			}
		case text[i] == '"' || text[i] == '\'':
			quote := text[i]
			j := i + 1
			for j < len(text) {
				if text[j] == '\\' {
					j += 2
					continue
				}
				if text[j] == quote {
					j++
					break
				}
				j++
			}
			blank(i, j)
			i = j
		default:
			i++
		}
	}
	return string(out)
}

func springMatchingDelimiter(mask string, open int, left, right byte) int {
	if open < 0 || open >= len(mask) || mask[open] != left {
		return -1
	}
	depth := 0
	for i := open; i < len(mask); i++ {
		switch mask[i] {
		case left:
			depth++
		case right:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func springNestingDepth(mask string, from, at int) int {
	depth := 0
	for i := max(0, from); i < min(at, len(mask)); i++ {
		switch mask[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
	}
	return depth
}

func springNextNonSpace(text string, from int) int {
	for from < len(text) {
		switch text[from] {
		case ' ', '\t', '\r', '\n':
			from++
		default:
			return from
		}
	}
	return from
}

func springAnnotations(text, mask string) []springAnnotation {
	matches := springAnnotationRE.FindAllStringSubmatchIndex(mask, -1)
	out := make([]springAnnotation, 0, len(matches))
	for _, match := range matches {
		annotation := springAnnotation{
			name:  mask[match[2]:match[3]],
			start: match[0],
			end:   match[1],
		}
		next := springNextNonSpace(mask, annotation.end)
		if next < len(mask) && mask[next] == '(' {
			if close := springMatchingDelimiter(mask, next, '(', ')'); close >= 0 {
				annotation.args = text[next+1 : close]
				annotation.end = close + 1
			}
		}
		out = append(out, annotation)
	}
	return out
}

func springAnnotationsBefore(annotations []springAnnotation, mask string, start, end int) []springAnnotation {
	boundary := 0
	for i := start - 1; i >= 0; i-- {
		switch mask[i] {
		case ';', '{', '}':
			boundary = i + 1
			i = -1
		}
	}
	out := make([]springAnnotation, 0)
	for _, annotation := range annotations {
		if annotation.start >= boundary && annotation.end <= end {
			out = append(out, annotation)
		}
	}
	return out
}

func springAnnotationNamed(annotations []springAnnotation, name string) *springAnnotation {
	for i := range annotations {
		if annotations[i].name == name {
			return &annotations[i]
		}
	}
	return nil
}

func springTopLevelIndex(text string, want byte) int {
	mask := springMaskedSource(text)
	round, square, curly, angle := 0, 0, 0, 0
	for i := 0; i < len(mask); i++ {
		switch mask[i] {
		case '(':
			round++
		case ')':
			round--
		case '[':
			square++
		case ']':
			square--
		case '{':
			curly++
		case '}':
			curly--
		case '<':
			angle++
		case '>':
			if angle > 0 {
				angle--
			}
		default:
			if mask[i] == want && round == 0 && square == 0 && curly == 0 && angle == 0 {
				return i
			}
		}
	}
	return -1
}

func springSplit(text string, separator rune) []string {
	sep := byte(separator)
	var out []string
	for len(text) > 0 {
		at := springTopLevelIndex(text, sep)
		if at < 0 {
			out = append(out, strings.TrimSpace(text))
			break
		}
		out = append(out, strings.TrimSpace(text[:at]))
		text = text[at+1:]
	}
	return out
}

func springAnnotationFields(annotation springAnnotation) map[string]string {
	fields := map[string]string{}
	args := strings.TrimSpace(annotation.args)
	if args == "" {
		return fields
	}
	var positional []string
	for _, part := range springSplit(args, ',') {
		if at := springTopLevelIndex(part, '='); at >= 0 {
			key := strings.TrimSpace(part[:at])
			if key != "" {
				fields[key] = strings.TrimSpace(part[at+1:])
			}
		} else if part != "" {
			positional = append(positional, part)
		}
	}
	switch len(positional) {
	case 1:
		fields["value"] = positional[0]
	case 2:
		fields["value"] = "[" + positional[0] + ", " + positional[1] + "]"
	default:
		if len(positional) > 2 {
			fields["value"] = "[" + strings.Join(positional, ", ") + "]"
		}
	}
	return fields
}

func springConstants(text, mask string) map[string]string {
	out := map[string]string{}
	for _, regex := range []*regexp.Regexp{springJavaConstRE, springKotlinConstRE} {
		for _, match := range regex.FindAllStringSubmatchIndex(mask, -1) {
			name := mask[match[2]:match[3]]
			if name == "" {
				continue
			}
			if _, exists := out[name]; !exists {
				statement := text[match[0]:match[1]]
				if at := strings.IndexByte(statement, '='); at >= 0 {
					value := strings.TrimSpace(strings.TrimSuffix(statement[at+1:], ";"))
					out[name] = value
				}
			}
		}
	}
	return out
}

func springResolveExpression(expr string, constants map[string]string, enumValues bool, seen map[string]bool) []string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	if strings.HasPrefix(expr, "{") && strings.HasSuffix(expr, "}") {
		return springResolveList(expr[1:len(expr)-1], constants, enumValues, seen)
	}
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		return springResolveList(expr[1:len(expr)-1], constants, enumValues, seen)
	}
	for _, prefix := range []string{"arrayOf(", "listOf(", "setOf("} {
		if strings.HasPrefix(expr, prefix) && strings.HasSuffix(expr, ")") {
			return springResolveList(expr[len(prefix):len(expr)-1], constants, enumValues, seen)
		}
	}
	parts := springSplit(expr, '+')
	if len(parts) > 1 {
		var joined strings.Builder
		for _, part := range parts {
			values := springResolveExpression(part, constants, false, seen)
			if len(values) != 1 {
				return nil
			}
			joined.WriteString(values[0])
		}
		return []string{joined.String()}
	}
	if loc := springQuoteRE.FindString(expr); loc == expr {
		if value, err := strconv.Unquote(expr); err == nil {
			return []string{springExpandTemplate(value, constants, seen)}
		}
	}
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return springResolveExpression(expr[1:len(expr)-1], constants, enumValues, seen)
	}
	if _, err := strconv.ParseFloat(expr, 64); err == nil {
		return []string{expr}
	}
	tokens := strings.FieldsFunc(expr, func(r rune) bool {
		return !(r == '_' || r == '$' || r == '.' || r >= '0' && r <= '9' ||
			r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	})
	if len(tokens) != 1 || tokens[0] != strings.TrimSpace(expr) {
		return nil
	}
	qualified := strings.Split(tokens[0], ".")
	name := qualified[len(qualified)-1]
	if len(qualified) > 1 && qualified[len(qualified)-2] == "MediaType" {
		if media := springMediaTypes[strings.TrimSuffix(name, "_VALUE")]; media != "" {
			return []string{media}
		}
	}
	if enumValues {
		return []string{name}
	}
	if seen[name] || constants[name] == "" {
		return nil
	}
	seen[name] = true
	return springResolveExpression(constants[name], constants, false, seen)
}

func springExpandTemplate(value string, constants map[string]string, seen map[string]bool) string {
	if !strings.Contains(value, "$") {
		return value
	}
	return springTemplateRE.ReplaceAllStringFunc(value, func(match string) string {
		parts := springTemplateRE.FindStringSubmatch(match)
		name := ""
		if len(parts) > 2 {
			name = parts[1]
			if name == "" {
				name = parts[2]
			}
		}
		if name == "" || seen[name] || constants[name] == "" {
			return match
		}
		copySeen := make(map[string]bool, len(seen)+1)
		for key, value := range seen {
			copySeen[key] = value
		}
		copySeen[name] = true
		values := springResolveExpression(constants[name], constants, false, copySeen)
		if len(values) == 1 {
			return values[0]
		}
		return match
	})
}

func springResolveList(
	expr string,
	constants map[string]string,
	enumValues bool,
	seen map[string]bool,
) []string {
	var out []string
	for _, part := range springSplit(expr, ',') {
		copySeen := make(map[string]bool, len(seen))
		for key, value := range seen {
			copySeen[key] = value
		}
		out = append(out, springResolveExpression(part, constants, enumValues, copySeen)...)
	}
	return out
}

func springJoinPaths(base, path string) string {
	joined := "/" + base + "/" + path
	for strings.Contains(joined, "//") {
		joined = strings.ReplaceAll(joined, "//", "/")
	}
	if len(joined) > 1 {
		joined = strings.TrimSuffix(joined, "/")
	}
	return joined
}

func springExpandedPaths(class *springAnnotation, method *springAnnotation, constants map[string]string) []string {
	pathsOf := func(annotation *springAnnotation) []string {
		if annotation == nil {
			return nil
		}
		fields := springAnnotationFields(*annotation)
		expr := fields["path"]
		if strings.TrimSpace(expr) == "" {
			expr = fields["value"]
		}
		return springResolveExpression(expr, constants, false, map[string]bool{})
	}
	bases, own := pathsOf(class), pathsOf(method)
	if len(bases) == 0 {
		bases = []string{""}
	}
	if len(own) == 0 {
		own = []string{""}
	}
	out := make([]string, 0, len(bases)*len(own))
	for _, base := range bases {
		for _, path := range own {
			out = append(out, springJoinPaths(base, path))
		}
	}
	return out
}

func springAnnotationString(annotation *springAnnotation, keys ...string) *string {
	if annotation == nil {
		return nil
	}
	fields := springAnnotationFields(*annotation)
	for _, key := range keys {
		if strings.TrimSpace(fields[key]) == "" {
			continue
		}
		values := springResolveExpression(fields[key], nil, false, map[string]bool{})
		if len(values) > 0 {
			value := values[0]
			return &value
		}
	}
	return nil
}

func springHTTPMethod(method, class *springAnnotation, constants map[string]string) string {
	if method != nil {
		if value := springMappingMethods[method.name]; value != "" {
			return value
		}
		fields := springAnnotationFields(*method)
		if values := springResolveExpression(fields["method"], constants, true, map[string]bool{}); len(values) > 0 {
			return strings.ToUpper(values[0])
		}
	}
	if class != nil {
		fields := springAnnotationFields(*class)
		if values := springResolveExpression(fields["method"], constants, true, map[string]bool{}); len(values) > 0 {
			return strings.ToUpper(values[0])
		}
	}
	return "GET"
}

func springMedia(annotation *springAnnotation, key string, constants map[string]string) *string {
	if annotation == nil {
		return nil
	}
	fields := springAnnotationFields(*annotation)
	if values := springResolveExpression(fields[key], constants, false, map[string]bool{}); len(values) > 0 {
		value := values[0]
		return &value
	}
	return nil
}

func springFirstMedia(method, class *springAnnotation, key string, constants map[string]string) *string {
	if value := springMedia(method, key, constants); value != nil {
		return value
	}
	return springMedia(class, key, constants)
}

func springParseParam(text string) springParam {
	annotations := springAnnotations(text, springMaskedSource(text))
	clean := []byte(text)
	for _, annotation := range annotations {
		for i := annotation.start; i < annotation.end && i < len(clean); i++ {
			if clean[i] != '\n' && clean[i] != '\r' {
				clean[i] = ' '
			}
		}
	}
	declaration := strings.TrimSpace(string(clean))
	for _, modifier := range []string{"final ", "val ", "var "} {
		declaration = strings.TrimSpace(strings.TrimPrefix(declaration, modifier))
	}
	param := springParam{annotations: annotations}
	if colon := springTopLevelIndex(declaration, ':'); colon >= 0 {
		left := strings.TrimSpace(declaration[:colon])
		fields := strings.Fields(left)
		if len(fields) > 0 {
			param.name = fields[len(fields)-1]
		}
		param.typ = strings.TrimSpace(declaration[colon+1:])
		return param
	}
	fields := strings.Fields(declaration)
	if len(fields) >= 2 {
		param.name = fields[len(fields)-1]
		param.typ = strings.Join(fields[:len(fields)-1], " ")
	}
	return param
}

func springIsMapType(typ string) bool {
	typ = strings.TrimSpace(typ)
	for _, prefix := range []string{
		"java.util.Map", "Map", "MultiValueMap", "HttpHeaders",
		"LinkedMultiValueMap", "HashMap",
	} {
		if typ == prefix || strings.HasPrefix(typ, prefix+"<") {
			return true
		}
	}
	return false
}

func springParamValue(param springParam, annotation string, constants map[string]string) (string, *string) {
	ann := springAnnotationNamed(param.annotations, annotation)
	if ann == nil {
		return "", nil
	}
	fields := springAnnotationFields(*ann)
	name := param.name
	for _, key := range []string{"value", "name"} {
		if values := springResolveExpression(fields[key], constants, false, map[string]bool{}); len(values) > 0 && values[0] != "" {
			name = values[0]
			break
		}
	}
	var defaultValue *string
	if values := springResolveExpression(fields["defaultValue"], constants, false, map[string]bool{}); len(values) > 0 {
		value := values[0]
		defaultValue = &value
	}
	return name, defaultValue
}

func springConditionPairs(annotation *springAnnotation, key string, constants map[string]string) []string {
	if annotation == nil {
		return nil
	}
	fields := springAnnotationFields(*annotation)
	return springResolveExpression(fields[key], constants, false, map[string]bool{})
}

func springControllersInText(text string) []springController {
	if !regexp.MustCompile(`@(?:[\w.]+\.)?(?:Rest)?Controller\b`).MatchString(text) {
		return nil
	}
	mask := springMaskedSource(text)
	annotations := springAnnotations(text, mask)
	constants := springConstants(text, mask)
	classes := springClassRE.FindAllStringSubmatchIndex(mask, -1)
	out := make([]springController, 0)
	for _, classMatch := range classes {
		nameStart, nameEnd := classMatch[2], classMatch[3]
		classAnnotations := springAnnotationsBefore(annotations, mask, nameStart, nameStart)
		var controllerAnn *springAnnotation
		for i := range classAnnotations {
			if springControllerAnnotations[classAnnotations[i].name] {
				controllerAnn = &classAnnotations[i]
				break
			}
		}
		if controllerAnn == nil {
			continue
		}
		bodyOpenRelative := strings.IndexByte(mask[nameEnd:], '{')
		if bodyOpenRelative < 0 {
			continue
		}
		bodyOpen := nameEnd + bodyOpenRelative
		bodyClose := springMatchingDelimiter(mask, bodyOpen, '{', '}')
		if bodyClose < 0 {
			continue
		}
		classMapping := springAnnotationNamed(classAnnotations, "RequestMapping")
		controller := springController{
			name:   mask[nameStart:nameEnd],
			offset: nameStart,
			start:  classAnnotations[0].start,
			end:    bodyClose + 1,
		}
		seen := map[int]bool{}
		for _, mapping := range annotations {
			if mapping.start <= bodyOpen || mapping.start >= bodyClose ||
				springMappingMethods[mapping.name] == "" && mapping.name != "RequestMapping" {
				continue
			}
			// Mapping annotations sit directly in the controller body. A
			// nested class has its own controller pass and is one level deeper.
			if springNestingDepth(mask, bodyOpen+1, mapping.start) != 0 {
				continue
			}
			paren := strings.IndexByte(mask[mapping.end:], '(')
			if paren < 0 {
				continue
			}
			paren += mapping.end
			beforeParen := mask[mapping.end:paren]
			if strings.ContainsAny(beforeParen, ";{}") {
				continue
			}
			signatureStart := paren - 1
			for signatureStart >= bodyOpen &&
				(mask[signatureStart] == ' ' || mask[signatureStart] == '\t' ||
					mask[signatureStart] == '\r' || mask[signatureStart] == '\n') {
				signatureStart--
			}
			nameAt := signatureStart
			for nameAt >= bodyOpen {
				ch := mask[nameAt]
				if ch != '_' && ch != '$' && (ch < '0' || ch > '9') &&
					(ch < 'A' || ch > 'Z') && (ch < 'a' || ch > 'z') {
					break
				}
				nameAt--
			}
			nameAt++
			if nameAt > signatureStart {
				continue
			}
			parenClose := springMatchingDelimiter(mask, paren, '(', ')')
			if parenClose < 0 {
				continue
			}
			methodEnd, ok := springFindMethodEnd(mask, annotations, parenClose, bodyClose)
			if !ok || seen[nameAt] {
				continue
			}
			boundary := mapping.start
			for i := mapping.start - 1; i > bodyOpen; i-- {
				switch mask[i] {
				case ';', '{', '}':
					boundary = i + 1
					i = bodyOpen
				}
			}
			methodAnnotations := make([]springAnnotation, 0)
			for _, annotation := range annotations {
				if annotation.start >= boundary && annotation.end <= paren {
					methodAnnotations = append(methodAnnotations, annotation)
				}
			}
			endpoint := springEndpoint{
				className:   controller.name,
				methodName:  mask[nameAt : signatureStart+1],
				offset:      nameAt,
				start:       mapping.start,
				end:         methodEnd,
				classOffset: nameStart,
				httpMethod:  springHTTPMethod(&mapping, classMapping, constants),
				paths:       springExpandedPaths(classMapping, &mapping, constants),
				consumes:    springFirstMedia(&mapping, classMapping, "consumes", constants),
				produces:    springFirstMedia(&mapping, classMapping, "produces", constants),
			}
			for _, raw := range springSplit(text[paren+1:parenClose], ',') {
				param := springParseParam(raw)
				if name, defaultValue := springParamValue(param, "RequestParam", constants); name != "" && !springIsMapType(param.typ) {
					endpoint.query = append(endpoint.query, springQueryParam{name: name, defaultVal: defaultValue})
					continue
				}
				if name, defaultValue := springParamValue(param, "RequestHeader", constants); name != "" && !springIsMapType(param.typ) {
					value := "{{" + name + "}}"
					if defaultValue != nil {
						value = *defaultValue
					}
					endpoint.headers = append(endpoint.headers, springHeader{name: name, value: value})
					continue
				}
				if springAnnotationNamed(param.annotations, "RequestBody") != nil {
					typ := strings.TrimSpace(param.typ)
					if typ == "" {
						typ = "Object"
					}
					endpoint.bodyType = &typ
				}
			}
			for _, condition := range append(
				springConditionPairs(classMapping, "params", constants),
				springConditionPairs(&mapping, "params", constants)...,
			) {
				if strings.Contains(condition, "!") {
					continue
				}
				name, value, _ := strings.Cut(condition, "=")
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				found := false
				for _, query := range endpoint.query {
					if query.name == name {
						found = true
						break
					}
				}
				if !found {
					var defaultValue *string
					if value = strings.TrimSpace(value); value != "" {
						defaultValue = &value
					}
					endpoint.query = append(endpoint.query, springQueryParam{name: name, defaultVal: defaultValue})
				}
			}
			for _, condition := range append(
				springConditionPairs(classMapping, "headers", constants),
				springConditionPairs(&mapping, "headers", constants)...,
			) {
				if strings.Contains(condition, "!") {
					continue
				}
				name, value, ok := strings.Cut(condition, "=")
				if ok && strings.TrimSpace(name) != "" {
					endpoint.headers = append(endpoint.headers, springHeader{
						name: strings.TrimSpace(name), value: strings.TrimSpace(value),
					})
				}
			}
			seen[nameAt] = true
			controller.endpoints = append(controller.endpoints, endpoint)
		}
		if len(controller.endpoints) > 0 {
			sort.SliceStable(controller.endpoints, func(i, j int) bool {
				return controller.endpoints[i].offset < controller.endpoints[j].offset
			})
			out = append(out, controller)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// springFindMethodEnd accepts Java block and abstract methods and Kotlin
// expression-body methods. Without this, a Kotlin `fun get(...) = ...` range
// runs into the next mapping and hides later endpoints.
func springFindMethodEnd(
	mask string,
	annotations []springAnnotation,
	parenClose, classClose int,
) (int, bool) {
	at := springNextNonSpace(mask, parenClose+1)
	nextMapping := classClose
	for _, annotation := range annotations {
		if annotation.start <= parenClose {
			continue
		}
		if springMappingMethods[annotation.name] != "" || annotation.name == "RequestMapping" {
			nextMapping = annotation.start
			break
		}
	}
	for i := at; i < classClose; i++ {
		switch mask[i] {
		case '{':
			if close := springMatchingDelimiter(mask, i, '{', '}'); close >= 0 {
				return close + 1, true
			}
			return 0, false
		case ';':
			return i + 1, true
		case '=':
			if nextMapping > i {
				return nextMapping, true
			}
		case '\n':
			if next := springNextNonSpace(mask, i+1); next >= classClose {
				return classClose, true
			}
		}
	}
	if nextMapping < classClose {
		return nextMapping, true
	}
	return classClose, classClose > parenClose
}

func springAt(controllers []springController, offset int) (*springController, *springEndpoint) {
	var controller *springController
	for i := range controllers {
		if controllers[i].start <= offset && offset <= controllers[i].end {
			controller = &controllers[i]
		}
	}
	if controller == nil {
		return nil, nil
	}
	for i := range controller.endpoints {
		if controller.endpoints[i].start <= offset && offset <= controller.endpoints[i].end {
			return controller, &controller.endpoints[i]
		}
	}
	return controller, nil
}
