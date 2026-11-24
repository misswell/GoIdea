package app

import (
	"encoding/json"
	"regexp"
	"strings"
)

type httpBodyFormat string

const (
	httpBodyJSON httpBodyFormat = "json"
	httpBodyXML  httpBodyFormat = "xml"
	httpBodyHTML httpBodyFormat = "html"
	httpBodyText httpBodyFormat = "text"
)

func httpBodyKind(contentType *string, body string) httpBodyFormat {
	kind := ""
	if contentType != nil {
		kind = strings.ToLower(*contentType)
	}
	switch {
	case strings.Contains(kind, "json"):
		return httpBodyJSON
	case strings.Contains(kind, "html"):
		return httpBodyHTML
	case strings.Contains(kind, "xml"):
		return httpBodyXML
	}
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var value any
		if json.Unmarshal([]byte(body), &value) == nil {
			return httpBodyJSON
		}
		return httpBodyText
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "<!doctype html") ||
		strings.HasPrefix(lower, "<html ") || lower == "<html" {
		return httpBodyHTML
	}
	if strings.HasPrefix(trimmed, "<?xml") || httpXMLStartRE.MatchString(trimmed) {
		return httpBodyXML
	}
	return httpBodyText
}

var httpXMLStartRE = regexp.MustCompile(`^<([A-Za-z][\w:.-]*)(?:\s|/?>)`)

func httpPrettyJSON(body string) string {
	var value any
	if json.Unmarshal([]byte(body), &value) != nil {
		return body
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return body
	}
	return string(formatted)
}

var (
	httpMarkupTokenRE = regexp.MustCompile(`(?s)<!--.*?-->|<!\[CDATA\[.*?\]\]>|<[^>]+>|[^<]+|<`)
	httpMarkupNameRE  = regexp.MustCompile(`^<\s*([\w:.-]+)`)
)

var httpVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

func httpMarkupTokens(body string, html bool) []string {
	var tokens []string
	for offset := 0; offset < len(body); {
		loc := httpMarkupTokenRE.FindStringIndex(body[offset:])
		if loc == nil {
			tokens = append(tokens, body[offset:])
			break
		}
		loc[0] += offset
		loc[1] += offset
		token := body[loc[0]:loc[1]]
		tokens = append(tokens, token)
		offset = loc[1]
		nameMatch := httpMarkupNameRE.FindStringSubmatch(token)
		if !html || len(nameMatch) < 2 || !httpRawElement(nameMatch[1]) || strings.HasSuffix(token, "/>") {
			continue
		}
		name := strings.ToLower(nameMatch[1])
		lowerRest := strings.ToLower(body[offset:])
		closeAt := strings.Index(lowerRest, "</"+name)
		if closeAt < 0 {
			tokens[len(tokens)-1] = body[loc[0]:]
			break
		}
		closeAt += offset
		end := strings.IndexByte(body[closeAt:], '>')
		if end < 0 {
			end = len(body)
		} else {
			end += closeAt + 1
		}
		tokens[len(tokens)-1] = body[loc[0]:end]
		offset = end
	}
	return tokens
}

func httpRawElement(name string) bool {
	switch strings.ToLower(name) {
	case "script", "style", "pre":
		return true
	default:
		return false
	}
}

func httpMarkupTag(token string) bool {
	return strings.HasPrefix(token, "<!") || strings.HasPrefix(token, "<?") ||
		strings.HasPrefix(token, "</") || strings.HasPrefix(token, "<")
}

func httpPrettyMarkup(body string, html bool, indent string) string {
	tokens := httpMarkupTokens(body, html)
	if len(tokens) == 0 {
		return body
	}
	var out []string
	depth := 0
	pad := func() string {
		return strings.Repeat(indent, max(0, depth))
	}
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if !httpMarkupTag(token) {
			if text := strings.TrimSpace(token); text != "" {
				out = append(out, pad()+text)
			}
			continue
		}
		if strings.HasPrefix(token, "<!") || strings.HasPrefix(token, "<?") {
			out = append(out, pad()+token)
			continue
		}
		if strings.HasPrefix(token, "</") {
			depth--
			out = append(out, pad()+token)
			continue
		}
		name := ""
		if match := httpMarkupNameRE.FindStringSubmatch(token); len(match) > 1 {
			name = strings.ToLower(match[1])
		}
		raw := html && httpRawElement(name) && strings.Contains(strings.ToLower(token), "</"+name)
		if raw || strings.HasSuffix(token, "/>") || html && httpVoidElements[name] {
			out = append(out, pad()+token)
			continue
		}
		if i+2 < len(tokens) && !httpMarkupTag(tokens[i+1]) &&
			strings.HasPrefix(tokens[i+2], "</") {
			out = append(out, pad()+token+strings.TrimSpace(tokens[i+1])+tokens[i+2])
			i += 2
			continue
		}
		if i+1 < len(tokens) && strings.HasPrefix(tokens[i+1], "</") {
			out = append(out, pad()+token+tokens[i+1])
			i++
			continue
		}
		out = append(out, pad()+token)
		depth++
	}
	return strings.Join(out, "\n")
}

func httpPrettyBody(kind httpBodyFormat, body string) string {
	switch kind {
	case httpBodyJSON:
		return httpPrettyJSON(body)
	case httpBodyXML:
		return httpPrettyMarkup(body, false, "  ")
	case httpBodyHTML:
		return httpPrettyMarkup(body, true, "  ")
	default:
		return body
	}
}
