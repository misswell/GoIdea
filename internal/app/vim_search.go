package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

func (m *Model) vimSearchReady(t *Tab) bool {
	return t != nil && t.state != nil && t.state.Buffer != nil &&
		m.hasTab(t)
}

func (m *Model) vimExecuteSearch(t *Tab, st *vimTabState, prompt, input string) error {
	if !m.vimSearchReady(t) {
		return fmt.Errorf("E158: Invalid buffer name")
	}
	pattern := input
	if pattern == "" {
		if !m.vim.search.set {
			return fmt.Errorf("E35: No previous regular expression")
		}
		pattern = m.vim.search.pattern
	}
	if _, err := compileVimSearch(pattern, false); err != nil {
		return fmt.Errorf("E486: %w", err)
	}
	m.vim.search.pattern = pattern
	m.vim.search.set = true
	m.vim.search.forward = prompt != "?"
	if err := m.vimSearchStep(t, pattern, m.vim.search.forward); err != nil {
		return err
	}
	_ = st
	return nil
}

func (m *Model) vimRepeatSearch(t *Tab, st *vimTabState, count int, reverse bool) error {
	if !m.vimSearchReady(t) {
		return fmt.Errorf("E158: Invalid buffer name")
	}
	if !m.vim.search.set {
		return fmt.Errorf("E35: No previous regular expression")
	}
	forward := m.vim.search.forward
	if reverse {
		forward = !forward
	}
	for range max(1, count) {
		if err := m.vimSearchStep(t, m.vim.search.pattern, forward); err != nil {
			return err
		}
	}
	_ = st
	return nil
}

func (m *Model) vimSearchStep(t *Tab, pattern string, forward bool) error {
	re, err := compileVimSearch(pattern, false)
	if err != nil {
		return fmt.Errorf("E486: %w", err)
	}
	text := t.state.Buffer.String()
	matches := re.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return fmt.Errorf("E486: Pattern not found: %s", pattern)
	}
	cursor := t.state.Cursor()
	target := -1
	if forward {
		for _, match := range matches {
			if match[0] > cursor {
				target = match[0]
				break
			}
		}
		if target < 0 {
			target = matches[0][0]
		}
	} else {
		for i := len(matches) - 1; i >= 0; i-- {
			if matches[i][0] < cursor {
				target = matches[i][0]
				break
			}
		}
		if target < 0 {
			target = matches[len(matches)-1][0]
		}
	}
	t.state.Select(target, target)
	return nil
}

func compileVimSearch(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	if strings.HasPrefix(pattern, `\c`) {
		ignoreCase = true
		pattern = strings.TrimPrefix(pattern, `\c`)
	} else if strings.HasPrefix(pattern, `\C`) {
		ignoreCase = false
		pattern = strings.TrimPrefix(pattern, `\C`)
	}
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

type vimSubstituteSpec struct {
	all         bool
	start       int
	end         int
	hasRange    bool
	pattern     string
	replacement string
	global      bool
	ignoreCase  bool
	noError     bool
}

// vimExecuteSubstitute returns handled=false when ex is not a substitute
// command, leaving the remaining Ex command dispatcher untouched.
func (m *Model) vimExecuteSubstitute(t *Tab, ex string) (bool, error) {
	spec, ok, err := parseVimSubstitute(ex)
	if !ok || err != nil {
		return ok, err
	}
	if !m.vimSearchReady(t) {
		return true, fmt.Errorf("E158: Invalid buffer name")
	}
	last := t.state.Buffer.LineCount() - 1
	firstLine, lastLine := t.state.Buffer.Pos(t.state.Cursor()).Line, t.state.Buffer.Pos(t.state.Cursor()).Line
	if spec.all {
		firstLine, lastLine = 0, last
	} else if spec.hasRange {
		var rangeErr error
		firstLine, rangeErr = vimSubstituteLine(spec.start, t.state.Cursor(), t.state.Buffer, true)
		if rangeErr == nil {
			lastLine, rangeErr = vimSubstituteLine(spec.end, t.state.Cursor(), t.state.Buffer, false)
		}
		if rangeErr != nil {
			return true, rangeErr
		}
		if firstLine > lastLine {
			firstLine, lastLine = lastLine, firstLine
		}
	}
	if firstLine < 0 || lastLine > last || firstLine > lastLine {
		return true, fmt.Errorf("E16: Invalid range")
	}

	pattern := spec.pattern
	if pattern == "" {
		if !m.vim.search.set {
			return true, fmt.Errorf("E35: No previous regular expression")
		}
		pattern = m.vim.search.pattern
	}
	re, err := compileVimSearch(pattern, spec.ignoreCase)
	if err != nil {
		return true, fmt.Errorf("E486: %w", err)
	}

	b := t.state.Buffer
	var edits []editor.Edit
	var firstEdit int
	for line := firstLine; line <= lastLine; line++ {
		text := b.Line(line)
		lineStart := b.Offset(editor.Pos{Line: line})
		matches := re.FindAllStringSubmatchIndex(text, -1)
		if !spec.global && len(matches) > 1 {
			matches = matches[:1]
		}
		for _, match := range matches {
			replacement, replaceErr := vimExpandSubstitute(spec.replacement, text, match)
			if replaceErr != nil {
				return true, replaceErr
			}
			if len(edits) == 0 {
				firstEdit = lineStart + match[0]
			}
			edits = append(edits, editor.Edit{
				Start: lineStart + match[0],
				End:   lineStart + match[1],
				Text:  replacement,
			})
		}
	}
	if len(edits) == 0 {
		if spec.noError {
			return true, nil
		}
		return true, fmt.Errorf("E486: Pattern not found: %s", pattern)
	}
	b.Edit(edits...)
	target := min(max(0, firstEdit), b.Len())
	t.state.Select(target, target)
	m.vim.search.pattern = pattern
	m.vim.search.replacement = spec.replacement
	m.vim.search.set = true
	m.vim.search.forward = true
	return true, nil
}

func parseVimSubstitute(ex string) (vimSubstituteSpec, bool, error) {
	var spec vimSubstituteSpec
	text := strings.TrimSpace(ex)
	if text == "" {
		return spec, false, nil
	}
	pos := 0
	if text[pos] == '%' {
		spec.all = true
		pos++
	} else if vimSubstituteAtomStart(text[pos]) {
		start, next, ok := vimReadSubstituteAtom(text, pos)
		if !ok {
			return spec, false, nil
		}
		spec.start, pos = start, next
		spec.hasRange = true
		if pos < len(text) && text[pos] == ',' {
			pos++
			end, next, ok := vimReadSubstituteAtom(text, pos)
			if !ok {
				return spec, true, fmt.Errorf("E16: Invalid range")
			}
			spec.end, pos = end, next
		} else {
			spec.end = start
		}
	}
	for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t') {
		pos++
	}
	if strings.HasPrefix(text[pos:], "substitute") {
		pos += len("substitute")
	} else if strings.HasPrefix(text[pos:], "s") &&
		(pos+1 == len(text) || !isVimASCIILetter(text[pos+1])) {
		pos++
	} else {
		return spec, false, nil
	}
	for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t') {
		pos++
	}
	if pos >= len(text) || text[pos] == '\\' || text[pos] == '"' ||
		isVimASCIILetter(text[pos]) || isVimASCIIDigit(text[pos]) || text[pos] == '_' {
		return spec, true, fmt.Errorf("E146: Invalid delimiter")
	}
	delim := text[pos]
	pos++
	var err error
	spec.pattern, pos, err = vimReadSubstituteField(text, pos, delim, true)
	if err != nil {
		return spec, true, err
	}
	if pos < len(text) {
		spec.replacement, pos, err = vimReadSubstituteField(text, pos, delim, false)
		if err != nil {
			return spec, true, err
		}
	}
	flags := strings.TrimSpace(text[pos:])
	for _, flag := range flags {
		switch flag {
		case 'g':
			spec.global = true
		case 'i':
			spec.ignoreCase = true
		case 'e':
			spec.noError = true
		case ' ', '\t':
		default:
			return spec, true, fmt.Errorf("E488: Trailing characters: %c", flag)
		}
	}
	return spec, true, nil
}

func vimSubstituteAtomStart(c byte) bool {
	return isVimASCIIDigit(c) || c == '.' || c == '$'
}

func vimReadSubstituteAtom(text string, pos int) (int, int, bool) {
	start := pos
	if pos < len(text) && text[pos] == '.' {
		return -1, pos + 1, true
	}
	if pos < len(text) && text[pos] == '$' {
		return -2, pos + 1, true
	}
	for pos < len(text) && isVimASCIIDigit(text[pos]) {
		pos++
	}
	if pos == start {
		return 0, start, false
	}
	line := 0
	for i := start; i < pos; i++ {
		line = line*10 + int(text[i]-'0')
	}
	return line, pos, true
}

func vimSubstituteLine(atom, cursor int, b *editor.Buffer, first bool) (int, error) {
	last := b.LineCount() - 1
	current := b.Pos(cursor).Line
	switch atom {
	case -1:
		return current, nil
	case -2:
		return last, nil
	default:
		line := atom - 1
		if line < 0 || line > last {
			_ = first
			return 0, fmt.Errorf("E16: Invalid range")
		}
		return line, nil
	}
}

func vimReadSubstituteField(text string, pos int, delim byte, pattern bool) (string, int, error) {
	var out strings.Builder
	for pos < len(text) {
		c := text[pos]
		if c == delim {
			return out.String(), pos + 1, nil
		}
		if c != '\\' {
			out.WriteByte(c)
			pos++
			continue
		}
		if pos+1 >= len(text) {
			if pattern {
				return "", pos, fmt.Errorf("E54: Unmatched \\")
			}
			out.WriteByte('\\')
			pos++
			break
		}
		next := text[pos+1]
		if next == delim {
			out.WriteByte(delim)
		} else {
			out.WriteByte('\\')
			out.WriteByte(next)
		}
		pos += 2
	}
	if pattern {
		return "", pos, fmt.Errorf("E54: Unmatched delimiter")
	}
	return out.String(), pos, nil
}

func vimExpandSubstitute(template, source string, match []int) (string, error) {
	var out strings.Builder
	for i := 0; i < len(template); i++ {
		switch template[i] {
		case '&':
			out.WriteString(vimSubmatch(source, match, 0))
		case '\\':
			if i+1 >= len(template) {
				out.WriteByte('\\')
				continue
			}
			i++
			next := template[i]
			if next >= '0' && next <= '9' {
				out.WriteString(vimSubmatch(source, match, int(next-'0')))
			} else {
				switch next {
				case 'n':
					out.WriteByte('\n')
				case 'r':
					out.WriteByte('\r')
				case 't':
					out.WriteByte('\t')
				default:
					out.WriteByte(next)
				}
			}
		default:
			out.WriteByte(template[i])
		}
	}
	return out.String(), nil
}

func vimSubmatch(source string, match []int, group int) string {
	index := group * 2
	if index+1 >= len(match) || match[index] < 0 {
		return ""
	}
	return source[match[index]:match[index+1]]
}

func isVimASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isVimASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
