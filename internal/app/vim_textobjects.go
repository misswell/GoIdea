package app

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type vimTextObjectCovered struct {
	start int
	end   int
}

// vimTextObjectRange implements the standard word and bracket objects used by
// OpenIDEA's Vim key handler. Ranges are byte offsets in the MyGo buffer.
func vimTextObjectRange(b *editor.Buffer, pos int, object string, inner bool, count int,
	covered *vimTextObjectCovered) (start, end int, linewise bool, ok bool) {
	if b == nil || b.Len() == 0 || count < 1 {
		return 0, 0, false, false
	}
	pos = max(0, min(pos, b.Len()))
	switch object {
	case "w", "W":
		start, end = vimWordObjectRange(b, pos, object == "W", count)
		if !inner {
			start, end = vimWordObjectAround(b, start, end, object == "W")
		}
	case "p":
		start, end, linewise = vimParagraphObjectRange(b, pos, count)
	case "f":
		start, end, linewise, ok = vimFunctionObjectRange(b, pos, inner)
	case "(", ")", "b":
		start, end, ok = vimPairObjectRange(b, pos, '(', ')', inner, covered)
	case "{", "}", "B":
		start, end, ok = vimPairObjectRange(b, pos, '{', '}', inner, covered)
	case "[", "]":
		start, end, ok = vimPairObjectRange(b, pos, '[', ']', inner, covered)
	case "<", ">":
		start, end, ok = vimPairObjectRange(b, pos, '<', '>', inner, covered)
	case `"`, `'`, "`":
		r, _ := utf8.DecodeRuneInString(object)
		start, end, ok = vimQuoteObjectRange(b, pos, r, inner)
	default:
		return 0, 0, false, false
	}
	if object == "w" || object == "W" || object == "p" ||
		object == "(" || object == ")" || object == "b" ||
		object == "{" || object == "}" || object == "B" ||
		object == "[" || object == "]" || object == "<" || object == ">" ||
		object == `"` || object == `'` || object == "`" {
		ok = end > start
	}
	return start, end, linewise, ok
}

func vimWordObjectRange(b *editor.Buffer, pos int, big bool, count int) (int, int) {
	if pos >= b.Len() {
		pos = vimRuneLeft(b, b.Len())
	}
	text := b.String()
	class := vimCharClassAt(text, pos, big)
	start, end := pos, pos
	for start > 0 && vimCharClassAt(text, vimRuneLeft(b, start), big) == class {
		start = vimRuneLeft(b, start)
	}
	for end < b.Len() && vimCharClassAt(text, end, big) == class {
		end = vimRuneRight(b, end)
	}
	for n := 1; n < count; n++ {
		for end < b.Len() && vimCharClassAt(text, end, big) == 0 {
			end = vimRuneRight(b, end)
		}
		nextClass := vimCharClassAt(text, end, big)
		for end < b.Len() && vimCharClassAt(text, end, big) == nextClass {
			end = vimRuneRight(b, end)
		}
	}
	return start, end
}

func vimWordObjectAround(b *editor.Buffer, start, end int, big bool) (int, int) {
	text := b.String()
	if end < b.Len() && vimCharClassAt(text, end, big) == 0 &&
		text[end:end+1] != "\n" {
		for end < b.Len() && vimCharClassAt(text, end, big) == 0 &&
			text[end:end+1] != "\n" {
			end = vimRuneRight(b, end)
		}
		return start, end
	}
	for start > 0 {
		previous := vimRuneLeft(b, start)
		if vimCharClassAt(text, previous, big) != 0 ||
			text[previous:start] == "\n" {
			break
		}
		start = previous
	}
	return start, end
}

func vimPairObjectRange(b *editor.Buffer, pos int, open, close rune, inner bool,
	covered *vimTextObjectCovered) (int, int, bool) {
	text := b.String()
	search := pos
	if search > 0 && search < len(text) {
		r, _ := utf8.DecodeRuneInString(text[search:])
		if r == close {
			search = vimRuneLeft(b, search)
		}
	}
	depth := 0
	openAt := -1
	for end := search + 1; end > 0; {
		r, size := utf8.DecodeLastRuneInString(text[:end])
		if size <= 0 {
			break
		}
		end -= size
		if r == close {
			depth++
		} else if r == open {
			if depth == 0 {
				openAt = end
				break
			}
			depth--
		}
	}
	if openAt < 0 {
		return 0, 0, false
	}
	depth = 0
	closeAt := -1
	for i := openAt; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			break
		}
		if r == open {
			depth++
		} else if r == close {
			depth--
			if depth == 0 {
				closeAt = i
				break
			}
		}
		i += size
	}
	if closeAt < 0 || pos < openAt || pos > closeAt+1 {
		return 0, 0, false
	}
	start, end := openAt, vimRuneRight(b, closeAt)
	if covered != nil && start >= covered.start && end <= covered.end {
		return 0, 0, false
	}
	if inner {
		start, end = vimRuneRight(b, openAt), closeAt
	}
	return start, end, true
}

func vimQuoteObjectRange(b *editor.Buffer, pos int, quote rune, inner bool) (int, int, bool) {
	text := b.String()
	line := b.Pos(pos).Line
	lineStart := b.Offset(editor.Pos{Line: line})
	lineEnd := lineStart + len(b.Line(line))
	openAt, closeAt := -1, -1
	for i := lineStart; i < lineEnd; {
		r, size := utf8.DecodeRuneInString(text[i:lineEnd])
		if r == quote {
			escaped := i > lineStart && strings.HasSuffix(text[lineStart:i], `\`)
			if !escaped {
				if openAt < 0 || pos > i {
					if openAt >= 0 {
						closeAt = i
					}
					openAt = i
				} else {
					closeAt = i
					break
				}
			}
		}
		i += size
	}
	if openAt < 0 || closeAt < 0 {
		return 0, 0, false
	}
	if inner {
		return vimRuneRight(b, openAt), closeAt, true
	}
	return openAt, vimRuneRight(b, closeAt), true
}

func vimParagraphObjectRange(b *editor.Buffer, pos int, count int) (int, int, bool) {
	line := b.Pos(pos).Line
	blank := func(index int) bool {
		return strings.TrimSpace(b.Line(index)) == ""
	}
	if blank(line) {
		startLine, endLine := line, line
		for startLine > 0 && blank(startLine-1) {
			startLine--
		}
		for endLine+1 < b.LineCount() && blank(endLine+1) {
			endLine++
		}
		start := b.Offset(editor.Pos{Line: startLine})
		end := b.Offset(editor.Pos{Line: min(b.LineCount(), endLine+1)})
		return start, end, true
	}
	startLine, endLine := line, line
	for n := 1; n < count; n++ {
		if endLine+1 < b.LineCount() {
			endLine++
		}
	}
	for startLine > 0 && !blank(startLine-1) {
		startLine--
	}
	for endLine+1 < b.LineCount() && !blank(endLine+1) {
		endLine++
	}
	start := b.Offset(editor.Pos{Line: startLine})
	end := b.Offset(editor.Pos{Line: min(b.LineCount(), endLine+1)})
	return start, end, true
}

func vimFunctionObjectRange(b *editor.Buffer, pos int, inner bool) (start, end int, linewise bool, ok bool) {
	text := b.String()
	type pair struct {
		open  int
		close int
	}
	var stack []int
	var pairs []pair
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			break
		}
		switch r {
		case '{':
			stack = append(stack, i)
		case '}':
			if len(stack) > 0 {
				open := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				pairs = append(pairs, pair{open: open, close: i})
			}
		}
		i += size
	}
	best := pair{}
	found := false
	for _, candidate := range pairs {
		openLine := b.Pos(candidate.open).Line
		headerStart := b.Offset(editor.Pos{Line: openLine})
		if pos < headerStart || pos > candidate.close+1 {
			continue
		}
		if !vimLooksLikeFunctionHeader(text[headerStart:candidate.open]) {
			continue
		}
		if !found || candidate.close-candidate.open < best.close-best.open {
			best, found = candidate, true
		}
	}
	if !found {
		return 0, 0, false, false
	}
	openLine := b.Pos(best.open).Line
	closeLine := b.Pos(best.close).Line
	if openLine == closeLine {
		if inner {
			return vimRuneRight(b, best.open), best.close, false, best.close > best.open+1
		}
		start = b.Offset(editor.Pos{Line: openLine})
		end = b.Offset(editor.Pos{Line: min(b.LineCount(), closeLine+1)})
		return start, end, true, true
	}
	if inner {
		start = b.Offset(editor.Pos{Line: openLine + 1})
		end = b.Offset(editor.Pos{Line: closeLine})
		return start, end, true, end > start
	}
	start = b.Offset(editor.Pos{Line: openLine})
	end = b.Offset(editor.Pos{Line: min(b.LineCount(), closeLine+1)})
	return start, end, true, true
}

func vimLooksLikeFunctionHeader(header string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	lower := strings.ToLower(header)
	for _, marker := range []string{
		"function", "func ", "constructor", "=>", "->",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if !strings.Contains(header, ")") {
		return false
	}
	for _, prefix := range []string{
		"if ", "if(", "for ", "for(", "while ", "while(", "switch ",
		"switch(", "catch ", "catch(", "try ", "synchronized ", "synchronized(",
		"class ", "interface ", "enum ", "record ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}
