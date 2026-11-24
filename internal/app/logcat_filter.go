package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"mayoi/internal/openidea"
)

// logcatMatcher is the compiled form of Android Studio's Logcat filter
// expression. It keeps the same field filters, negation, grouping and OR/AND
// behaviour as OpenIDEA.
type logcatMatcher interface {
	match(openidea.LogcatLine) bool
}

type logcatTerm struct {
	field string
	value string
	regex bool
	level map[string]bool
	re    *regexp.Regexp
}

type logcatNot struct{ child logcatMatcher }
type logcatAnd struct{ children []logcatMatcher }
type logcatOr struct{ children []logcatMatcher }

func (t logcatTerm) match(line openidea.LogcatLine) bool {
	contains := func(value string) bool {
		if t.regex {
			return t.re.MatchString(value)
		}
		return strings.Contains(strings.ToLower(value), strings.ToLower(t.value))
	}
	switch t.field {
	case "tag":
		return contains(line.Tag)
	case "message":
		return contains(line.Message)
	case "pid":
		id, err := strconv.Atoi(t.value)
		return err == nil && (line.PID == id || line.TID == id)
	case "level":
		return t.level[line.Level]
	default:
		if contains(line.Tag) || contains(line.Message) {
			return true
		}
		if !strings.HasPrefix(t.value, "#") {
			return false
		}
		id := strings.TrimPrefix(t.value, "#")
		return strconv.Itoa(line.PID) == id || strconv.Itoa(line.TID) == id
	}
}

func (n logcatNot) match(line openidea.LogcatLine) bool { return !n.child.match(line) }

func (n logcatAnd) match(line openidea.LogcatLine) bool {
	for _, child := range n.children {
		if !child.match(line) {
			return false
		}
	}
	return true
}

func (n logcatOr) match(line openidea.LogcatLine) bool {
	for _, child := range n.children {
		if child.match(line) {
			return true
		}
	}
	return false
}

type logcatQueryTerm struct {
	field string
	value string
	regex bool
}

type logcatQueryNode struct {
	kind     string
	term     logcatQueryTerm
	child    *logcatQueryNode
	children []*logcatQueryNode
}

type logcatQueryParser struct {
	source string
	index  int
}

func compileLogcatQuery(source string) (logcatMatcher, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return logcatAnd{}, nil
	}
	parser := &logcatQueryParser{source: source}
	node, err := parser.parseOr()
	if err != nil {
		return logcatAnd{}, err
	}
	if next := parser.peek(); next != 0 {
		return logcatAnd{}, fmt.Errorf("unexpected %q", string(next))
	}
	return buildLogcatMatcher(node)
}

func (p *logcatQueryParser) peek() byte {
	for p.index < len(p.source) && isLogcatSpace(p.source[p.index]) {
		p.index++
	}
	if p.index >= len(p.source) {
		return 0
	}
	return p.source[p.index]
}

func (p *logcatQueryParser) parseOr() (*logcatQueryNode, error) {
	var children []*logcatQueryNode
	first, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	if first != nil {
		children = append(children, first)
	}
	for p.peek() == '|' {
		p.index++
		next, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, fmt.Errorf("nothing to match after |")
		}
		children = append(children, next)
	}
	switch len(children) {
	case 0:
		return nil, nil
	case 1:
		return children[0], nil
	default:
		return &logcatQueryNode{kind: "or", children: children}, nil
	}
}

func (p *logcatQueryParser) parseAnd() (*logcatQueryNode, error) {
	var children []*logcatQueryNode
	for {
		next := p.peek()
		if next == 0 || next == '|' || next == ')' {
			break
		}
		if next == '&' {
			p.index++
			continue
		}
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	switch len(children) {
	case 0:
		return nil, nil
	case 1:
		return children[0], nil
	default:
		return &logcatQueryNode{kind: "and", children: children}, nil
	}
}

func (p *logcatQueryParser) parseUnary() (*logcatQueryNode, error) {
	next := p.peek()
	if next == '-' || next == '!' {
		p.index++
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &logcatQueryNode{kind: "not", child: child}, nil
	}
	if next == '(' {
		p.index++
		child, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if child == nil {
			return nil, fmt.Errorf("empty parentheses")
		}
		if p.peek() != ')' {
			return nil, fmt.Errorf("missing )")
		}
		p.index++
		return child, nil
	}
	word := p.parseWord()
	if word == "" {
		return nil, fmt.Errorf("unexpected %q", string(next))
	}
	return &logcatQueryNode{kind: "term", term: logcatQueryWord(word)}, nil
}

func (p *logcatQueryParser) parseWord() string {
	var word strings.Builder
	for p.index < len(p.source) {
		ch := p.source[p.index]
		if isLogcatSpace(ch) || ch == '|' || ch == '&' || ch == '(' || ch == ')' {
			break
		}
		if ch == '"' || ch == '\'' {
			quote := ch
			p.index++
			for p.index < len(p.source) && p.source[p.index] != quote {
				word.WriteByte(p.source[p.index])
				p.index++
			}
			if p.index < len(p.source) {
				p.index++
			}
			continue
		}
		word.WriteByte(ch)
		p.index++
	}
	return word.String()
}

func logcatQueryWord(raw string) logcatQueryTerm {
	if colon := strings.IndexByte(raw, ':'); colon > 0 {
		key := strings.ToLower(raw[:colon])
		regex := strings.HasSuffix(key, "~")
		key = strings.TrimSuffix(key, "~")
		switch key {
		case "tag":
			return logcatQueryTerm{field: "tag", value: raw[colon+1:], regex: regex}
		case "message", "msg":
			return logcatQueryTerm{field: "message", value: raw[colon+1:], regex: regex}
		case "pid":
			return logcatQueryTerm{field: "pid", value: raw[colon+1:], regex: regex}
		case "level":
			return logcatQueryTerm{field: "level", value: raw[colon+1:]}
		}
	}
	return logcatQueryTerm{field: "text", value: raw}
}

func buildLogcatMatcher(node *logcatQueryNode) (logcatMatcher, error) {
	if node == nil {
		return logcatAnd{}, nil
	}
	switch node.kind {
	case "not":
		child, err := buildLogcatMatcher(node.child)
		if err != nil {
			return nil, err
		}
		return logcatNot{child: child}, nil
	case "and", "or":
		children := make([]logcatMatcher, 0, len(node.children))
		for _, child := range node.children {
			matcher, err := buildLogcatMatcher(child)
			if err != nil {
				return nil, err
			}
			children = append(children, matcher)
		}
		if node.kind == "and" {
			return logcatAnd{children: children}, nil
		}
		return logcatOr{children: children}, nil
	case "term":
		term := logcatTerm{
			field: node.term.field,
			value: node.term.value,
			regex: node.term.regex,
		}
		if term.field == "level" {
			term.level = make(map[string]bool)
			for _, value := range strings.Split(term.value, ",") {
				level, ok := logcatLevelName(strings.TrimSpace(value))
				if !ok {
					return nil, fmt.Errorf("unknown level %q", strings.TrimSpace(value))
				}
				term.level[level] = true
			}
		} else if term.regex {
			pattern, err := regexp.Compile("(?i)" + term.value)
			if err != nil {
				return nil, fmt.Errorf("invalid regular expression: %w", err)
			}
			term.re = pattern
		}
		return term, nil
	default:
		return nil, fmt.Errorf("invalid query")
	}
}

func logcatLevelName(value string) (string, bool) {
	switch strings.ToLower(value) {
	case "verbose", "v":
		return "verbose", true
	case "debug", "d":
		return "debug", true
	case "info", "i":
		return "info", true
	case "warn", "warning", "w":
		return "warn", true
	case "error", "e":
		return "error", true
	case "fatal", "assert", "a", "f":
		return "fatal", true
	case "unknown":
		return "unknown", true
	default:
		return "", false
	}
}

func isLogcatSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}
