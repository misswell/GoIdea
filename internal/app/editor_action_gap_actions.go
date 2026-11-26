package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

const (
	actionGapTimeout       = 60 * time.Second
	actionGapMaxScan       = 20000
	actionGapMaxOtherFiles = 4 << 20
)

var regexpRegionDesc = regexp.MustCompile(`(?i)\bdesc\s*=\s*"([^"]*)"`)

// installEditorActionGapActions registers the OpenIDEA editor actions that had
// keymap records but no native command. They use the normal action lifecycle,
// so a key, menu item, palette row, plugin or automation reaches one Run.
func installEditorActionGapActions() {
	editorAvailable := func(m *Model) bool { return m.activeEditorAvailable() }
	editorEditable := func(m *Model) bool { return m.activeEditorEditable() }
	editorFocused := func(m *Model) bool { return m.activeEditorFocused() }
	findPopup := func(m *Model) bool {
		return (m.toolVisible("find") || m.activeToolWindow() == "find" || m.search.visible) &&
			(m.search.running || m.search.done || len(m.search.files) > 0 ||
				m.search.historyOpen || m.search.err != "")
	}

	register := func(a *Action) {
		a.Builtin = true
		if a.When == nil {
			a.When = a.KeyWhen
		}
		_ = registerAction(a)
	}

	for _, spec := range []struct {
		level int
		id    string
		all   bool
	}{
		{1, "code.expandToLevel1", false},
		{2, "code.expandToLevel2", false},
		{3, "code.expandToLevel3", false},
		{4, "code.expandToLevel4", false},
		{5, "code.expandToLevel5", false},
		{1, "code.expandAllToLevel1", true},
		{2, "code.expandAllToLevel2", true},
		{3, "code.expandAllToLevel3", true},
		{4, "code.expandAllToLevel4", true},
		{5, "code.expandAllToLevel5", true},
	} {
		if _, exists := actionGet(spec.id); exists {
			continue
		}
		spec := spec
		register(&Action{
			ID: spec.id, Title: foldLevelTitle(spec.level, spec.all), Category: "Folding",
			Keywords: "unfold collapse code folding level",
			When:     editorAvailable, KeyWhen: editorFocused,
			Run: func(m *Model) { m.expandFoldingLevel(spec.level, spec.all) },
		})
	}

	register(&Action{
		ID: "code.autoIndentLines", Title: "Auto-Indent Lines", Category: "Code",
		Keywords: "reindent fix indentation",
		When:     editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.autoIndentEditorLines() },
	})
	register(&Action{
		ID: "code.completeStatement", Title: "Complete Current Statement", Category: "Code",
		Keywords: "semicolon braces newline statement",
		When:     editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.completeEditorStatement() },
	})
	register(&Action{
		ID: "code.cyclicExpandWord", Title: "Cyclic Expand Word", Category: "Code",
		Keywords: "hippie complete word dabbrev",
		When:     editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.cyclicExpandEditorWord(false) },
	})
	register(&Action{
		ID: "code.cyclicExpandWordBackward", Title: "Cyclic Expand Word (Backward)",
		Category: "Code", Keywords: "hippie complete word dabbrev",
		When: editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.cyclicExpandEditorWord(true) },
	})
	register(&Action{
		ID: "code.moveElementLeft", Title: "Move Element Left", Category: "Code",
		Keywords: "swap argument parameter array entry",
		When:     editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.moveEditorElement(-1) },
	})
	register(&Action{
		ID: "code.moveElementRight", Title: "Move Element Right", Category: "Code",
		Keywords: "swap argument parameter array entry",
		When:     editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.moveEditorElement(1) },
	})
	register(&Action{
		ID: "code.unwrap", Title: "Unwrap/Remove…", Category: "Code",
		Keywords: "unwrap remove statement",
		When:     editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.unwrapEditor() },
	})
	register(&Action{
		ID: "code.reformatDialog", Title: "Reformat File…", Category: "Code",
		Keywords: "format options file dialog",
		When:     editorEditable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.OpenReformatDialog() },
	})
	register(&Action{
		ID: "editor.contextInfo", Title: "Context Info", Category: "View",
		Keywords: "enclosing method class where am i",
		When:     editorAvailable, KeyWhen: editorFocused,
		Run: func(m *Model) { m.showEditorContextInfo() },
	})
	register(&Action{
		ID: "editor.toggleInsertState", Title: "Toggle Insert/Overwrite",
		Category: "Editor Actions", Keywords: "overwrite overtype block caret",
		When: editorAvailable, KeyWhen: editorFocused,
		Toggled: func(m *Model) bool {
			t := m.tab()
			return t != nil && t.insertOverwrite
		},
		Run: func(m *Model) { m.toggleEditorInsertState() },
	})

	for _, spec := range []struct {
		id    string
		title string
		run   func(*Model)
		arg   func(*Model, any)
		when  func(*Model) bool
	}{
		{"find.popup.nextResult", "Next Result", func(m *Model) { m.stepFindPopupResult(1) }, nil, findPopup},
		{"find.popup.previousResult", "Previous Result", func(m *Model) { m.stepFindPopupResult(-1) }, nil, findPopup},
		{"find.popup.openResult", "Open Result", func(m *Model) { m.openFindPopupResult() }, nil, findPopup},
		{"find.popup.recentSearches", "Recent Searches", func(m *Model) { m.ToggleRecentSearchFields() }, nil, findPopup},
	} {
		register(&Action{
			ID: spec.id, Title: spec.title, Category: "Find",
			When: spec.when, KeyWhen: spec.when, Run: spec.run, RunWithArg: spec.arg,
		})
	}

	for _, spec := range []struct {
		id    string
		title string
		run   func(*Model)
	}{
		{"navigate.customRegion", "Custom Folding…", func(m *Model) { m.ShowCustomFoldingRegions() }},
		{"navigate.gotoTest", "Test", func(m *Model) { m.GoToTestOrSubject() }},
		{"navigate.nextMethod", "Next Method", func(m *Model) { m.goToAdjacentMethod(1) }},
		{"navigate.previousMethod", "Previous Method", func(m *Model) { m.goToAdjacentMethod(-1) }},
		{"navigate.relatedSymbol", "Related Symbol…", func(m *Model) { m.ShowRelatedSymbols() }},
		{"navigate.superMethod", "Super Method", func(m *Model) { m.GoToSuperMethod() }},
	} {
		register(&Action{
			ID: spec.id, Title: spec.title, Category: "Navigate",
			When: editorAvailable, KeyWhen: editorFocused, Run: spec.run,
		})
	}

	for _, spec := range []struct {
		id    string
		title string
		kind  string
	}{
		{"refactor.pullMembersUp", "Pull Members Up…", "pullUp"},
		{"refactor.pushMembersDown", "Push Members Down…", "pushDown"},
	} {
		spec := spec
		register(&Action{
			ID: spec.id, Title: spec.title, Category: "Refactor",
			When: editorEditable, KeyWhen: editorFocused,
			Run: func(m *Model) { m.StartHierarchyRefactoring(spec.kind) },
		})
	}
}

func foldLevelTitle(level int, all bool) string {
	if all {
		return fmt.Sprintf("Expand All to Level %d", level)
	}
	return fmt.Sprintf("Expand to Level %d", level)
}

// expandFoldingLevel applies OpenIDEA's level semantics. Folds at or above
// the requested depth are open, while deeper regions stay collapsed. The
// caret command scopes that to the region around the caret. "All" scopes it
// to the document.
func (m *Model) expandFoldingLevel(level int, all bool) {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil || level < 1 {
		return
	}
	level = min(5, level)
	scopeLine := -1
	if !all {
		scopeLine = t.state.CursorPos().Line
	}
	ranges := foldingRanges(t.state.Buffer.Snapshot(), editorIndentUnit(t.state))
	selected := foldingRangesAtLevel(ranges, level, scopeLine)
	startLine := 0
	endLine := t.state.Buffer.Snapshot().LineCount() - 1
	if scopeLine >= 0 {
		root := 0
		for index, region := range ranges {
			if region.start <= scopeLine && scopeLine <= region.end &&
				(root == 0 || region.end-region.start < ranges[root].end-ranges[root].start) {
				root = index
			}
		}
		if len(ranges) == 0 {
			return
		}
		startLine = ranges[root].start
		endLine = ranges[root].end
	}
	t.state.Run(editor.UnfoldAll)
	for line := startLine; line <= endLine; line++ {
		if selected[line] {
			t.state.Fold(line)
		}
	}
	m.invalidate()
}

type foldingRange struct {
	start int
	end   int
	depth int
}

// foldingRanges mirrors the native editor's indentation and bracket fold
// candidates and records each fold's nesting depth.
func foldingRanges(snapshot editor.Snapshot, unit string) []foldingRange {
	if snapshot.LineCount() == 0 {
		return nil
	}
	if unit == "" {
		unit = "    "
	}
	var out []foldingRange
	stack := []foldingRange{}
	for line := 0; line < snapshot.LineCount()-1; line++ {
		for len(stack) > 0 && stack[len(stack)-1].end < line {
			stack = stack[:len(stack)-1]
		}
		text := snapshot.Line(line)
		if strings.TrimSpace(text) == "" {
			continue
		}
		end := -1
		if rangeEnd, ok := indentationFoldEnd(snapshot, line); ok {
			end = rangeEnd
		} else if rangeEnd, ok := bracketFoldEnd(snapshot, line); ok {
			end = rangeEnd
		}
		if end <= line {
			continue
		}
		region := foldingRange{start: line, end: end, depth: len(stack) + 1}
		for len(stack) > 0 && stack[len(stack)-1].end < end {
			stack = stack[:len(stack)-1]
		}
		region.depth = len(stack) + 1
		out = append(out, region)
		stack = append(stack, region)
	}
	return out
}

func foldingRangesAtLevel(ranges []foldingRange, level, scopeLine int) map[int]bool {
	selected := map[int]bool{}
	for _, region := range ranges {
		if scopeLine >= 0 && (region.start < scopeLine && scopeLine <= region.end) {
			continue
		}
		if region.depth > level {
			selected[region.start] = true
		}
	}
	return selected
}

func indentationFoldEnd(snapshot editor.Snapshot, line int) (int, bool) {
	width := indentationWidth(snapshot.Line(line))
	end := line
	for next := line + 1; next < snapshot.LineCount(); next++ {
		text := snapshot.Line(next)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if indentationWidth(text) <= width {
			break
		}
		end = next
	}
	return end, end > line
}

func bracketFoldEnd(snapshot editor.Snapshot, line int) (int, bool) {
	text := snapshot.Line(line)
	code := strings.TrimRight(text, " \t")
	code = strings.TrimRight(code[:lineCodeEnd(code, "")], " \t")
	if code == "" {
		return 0, false
	}
	open := code[len(code)-1]
	close := byte(0)
	switch open {
	case '{':
		close = '}'
	case '[':
		close = ']'
	case '(':
		close = ')'
	default:
		return 0, false
	}
	depth := 0
	for next := line; next < snapshot.LineCount(); next++ {
		part := snapshot.Line(next)
		if next == line {
			part = part[len(part)-1:]
		}
		quote := byte(0)
		for index := 0; index < len(part); index++ {
			c := part[index]
			if quote != 0 {
				if c == '\\' {
					index++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			switch c {
			case '"', '\'', '`':
				quote = c
			default:
				if c == open {
					depth++
				} else if c == close {
					depth--
					if depth == 0 && next > line {
						return next - 1, true
					}
				}
			}
		}
	}
	return 0, false
}

func indentationWidth(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

// autoIndentEditorLines reindents the lines of every selection as one undo
// step, preserving each selection's direction and covering the same text.
func (m *Model) autoIndentEditorLines() {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil || !m.activeEditorEditable() {
		return
	}
	snapshot := t.state.Buffer.Snapshot()
	sels := t.state.Selections()
	edits, selections := autoIndentEdits(snapshot, sels, editorIndentUnit(t.state))
	if len(edits) == 0 {
		m.status = "Nothing to auto-indent"
		m.invalidate()
		return
	}
	t.state.Buffer.Edit(edits...)
	t.state.SetSelections(selections...)
	m.finishEditorTransform(t)
}

func editorIndentUnit(state *editor.State) string {
	if state != nil && state.Indent == editor.IndentTabs {
		return "\t"
	}
	if state != nil && state.Indent == editor.IndentSpaces {
		size := state.TabSize
		if size <= 0 {
			size = 4
		}
		return strings.Repeat(" ", size)
	}
	return ""
}

func autoIndentEdits(
	snapshot editor.Snapshot,
	selections []editor.Selection,
	unit string,
) ([]editor.Edit, []editor.Selection) {
	if len(selections) == 0 {
		return nil, nil
	}
	type replacement struct {
		start int
		end   int
		text  string
	}
	byLine := map[int]replacement{}
	lineBias := map[int]int{}
	var shifts [][2]int
	for _, selection := range selections {
		first := snapshot.LineOf(selection.Start())
		last := snapshot.LineOf(selection.End())
		if selection.Empty() {
			first = snapshot.LineOf(selection.Head)
			last = first
		}
		if last > first && snapshot.LineStart(last) == selection.End() {
			last--
		}
		for line := first; line <= last; line++ {
			if _, ok := byLine[line]; ok {
				continue
			}
			start := snapshot.LineStart(line)
			end := snapshot.LineEnd(line)
			oldLine := snapshot.Slice(start, end)
			newLine := autoIndentedLine(snapshot, line, oldLine, unit)
			lead := gapLeadingWhitespace(oldLine)
			oldIndent := len(lead)
			newIndent := len(gapLeadingWhitespace(newLine))
			byLine[line] = replacement{start: start, end: start + oldIndent, text: gapLeadingWhitespace(newLine)}
			lineBias[line] = newIndent - oldIndent
			shifts = append(shifts, [2]int{start + oldIndent, newIndent - oldIndent})
		}
	}
	lines := make([]int, 0, len(byLine))
	for line := range byLine {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	edits := make([]editor.Edit, 0, len(lines))
	var selectionEdits []editor.Edit
	for _, line := range lines {
		r := byLine[line]
		if snapshot.Slice(r.start, r.end) != r.text {
			edits = append(edits, editor.Edit{Start: r.start, End: r.end, Text: r.text})
		}
		selectionEdits = append(selectionEdits, editor.Edit{Start: r.start, End: r.end, Text: r.text})
	}
	if len(edits) == 0 {
		return nil, nil
	}
	adjusted := make([]editor.Selection, 0, len(selections))
	for _, selection := range selections {
		anchor := mapOffsetThroughEdits(selection.Anchor, selectionEdits)
		head := mapOffsetThroughEdits(selection.Head, selectionEdits)
		anchorLine := snapshot.LineOf(selection.Anchor)
		headLine := snapshot.LineOf(selection.Head)
		if selection.Empty() {
			if bias, ok := lineBias[anchorLine]; ok &&
				selection.Anchor == snapshot.LineStart(anchorLine) {
				anchor += bias
				head += bias
			}
		}
		if selection.Anchor != selection.Head {
			if selection.Anchor == snapshot.LineStart(anchorLine) {
				anchor += lineBias[anchorLine]
			}
			if selection.Head != snapshot.LineStart(headLine) {
				head += lineBias[headLine]
			}
		}
		adjusted = append(adjusted, editor.Selection{Anchor: anchor, Head: head})
	}
	return edits, adjusted
}

func gapLeadingWhitespace(line string) string {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[:i]
}

func autoIndentedLine(snapshot editor.Snapshot, line int, text, unit string) string {
	trimmed := strings.TrimSpace(text)
	level := indentationBodyDepth(snapshot, line)
	if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") ||
		strings.HasPrefix(trimmed, ")") {
		level = max(0, level-1)
	}
	if unit == "" {
		unit = "    "
	}
	return strings.Repeat(unit, level) + trimmed
}

func indentationBodyDepth(snapshot editor.Snapshot, line int) int {
	depth := 0
	quote := byte(0)
	for current := 0; current < line; current++ {
		text := snapshot.Line(current)
		code := text[:lineCodeEnd(text, "")]
		lineDepth := 0
		for index := 0; index < len(code); index++ {
			c := code[index]
			if quote != 0 {
				if c == '\\' {
					index++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			switch c {
			case '"', '\'', '`':
				quote = c
			case '{', '[', '(':
				lineDepth++
			case '}', ']', ')':
				lineDepth--
			}
		}
		depth = max(0, depth+lineDepth)
	}
	return depth
}

func mapOffsetThroughEdits(offset int, edits []editor.Edit) int {
	mapped := offset
	for _, edit := range edits {
		if offset < edit.Start {
			break
		}
		if offset <= edit.End {
			return edit.Start + len(edit.Text)
		}
		mapped += len(edit.Text) - (edit.End - edit.Start)
	}
	return max(0, mapped)
}

// completeEditorStatement applies the selected completion first when present,
// then completes the line and moves below it as one undo step.
func (m *Model) completeEditorStatement() {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil || !m.activeEditorEditable() {
		return
	}
	if t.completion.open {
		if !m.acceptCurrentCompletion(t, "") {
			return
		}
	}
	if m.completeEditorStatementFromSyntax(t) {
		return
	}
	text := t.state.Buffer.String()
	cursor := t.state.Cursor()
	if selected := t.state.SelectedText(); selected != "" {
		cursor = t.state.Selection().Start()
	}
	language := editorStatementLanguage(t)
	change, ok := completeStatementAt(text, cursor, language, editorIndentUnit(t.state))
	if !ok {
		m.status = "No unfinished statement at the caret"
		m.invalidate()
		return
	}
	edits := completeStatementEdits(t.state.Buffer.String(), change)
	t.state.Buffer.Edit(edits...)
	caret := change.caret
	if len(edits) > 1 && change.end < len(text) {
		caret = change.caret + 1
	}
	t.state.Select(caret, caret)
	m.finishEditorTransform(t)
}

type statementSyntaxPosition struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

type statementSyntaxRange struct {
	Start statementSyntaxPosition `json:"start"`
	End   statementSyntaxPosition `json:"end"`
}

type statementSyntaxError struct {
	Kind     string               `json:"kind"`
	Range    statementSyntaxRange `json:"range"`
	Expected string               `json:"expected"`
}

type statementSyntaxErrors struct {
	Errors []statementSyntaxError `json:"errors"`
}

// completeEditorStatementFromSyntax repairs a broken line using the same
// backend syntax evidence as OpenIDEA. Text-only completion remains the
// fallback when the grammar, backend or query is unavailable.
func (m *Model) completeEditorStatementFromSyntax(t *Tab) bool {
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil ||
		m.ctx == nil || m.invokeHook == nil && m.mgr == nil {
		return false
	}
	docID := t.doc.ID()
	version := t.state.Buffer.Version()
	cursor := t.state.Cursor()
	language := editorStatementLanguage(t)
	request := m.nextRequest()
	go func() {
		ctx, cancel := context.WithTimeout(gapModelContext(m), actionGapTimeout)
		defer cancel()
		var errors statementSyntaxErrors
		raw, err := m.invoke(ctx, "syntax_errors", map[string]any{
			"doc": docID, "request": request,
		})
		if err == nil {
			err = json.Unmarshal(raw, &errors)
		}
		m.post(func() {
			if !m.tabOpen(t) || t.doc.ID() != docID ||
				t.state.Buffer.Version() != version || t.state.Cursor() != cursor {
				return
			}
			if err != nil {
				m.completeEditorStatementText(t, cursor, language)
				return
			}
			text := t.state.Buffer.String()
			if !applyStatementSyntaxRecovery(t, text, cursor, language, errors) {
				m.completeEditorStatementText(t, cursor, language)
			}
		})
	}()
	return true
}

func (m *Model) completeEditorStatementText(t *Tab, cursor int, language string) {
	change, ok := completeStatementAt(
		t.state.Buffer.String(), cursor, language, editorIndentUnit(t.state),
	)
	if !ok {
		m.status = "No unfinished statement at the caret"
		m.invalidate()
		return
	}
	t.state.Buffer.Edit(editor.Edit{Start: change.start, End: change.end, Text: change.insert})
	t.state.Select(change.caret, change.caret)
	m.finishEditorTransform(t)
}

func editorStatementLanguage(t *Tab) string {
	if t == nil || t.doc == nil {
		return ""
	}
	if id := strings.TrimSpace(t.doc.Info().Language); id != "" {
		return strings.ToLower(id)
	}
	if language := editor.LanguageFor(t.doc.Path()); language != nil {
		return strings.ToLower(language.Name)
	}
	return ""
}

func applyStatementSyntaxRecovery(
	t *Tab,
	text string,
	cursor int,
	language string,
	errors statementSyntaxErrors,
) bool {
	line := t.state.Buffer.Snapshot().LineOf(cursor)
	lineStart := t.state.Buffer.Snapshot().LineStart(line)
	codeEndOffset := t.state.Buffer.Snapshot().LineEnd(line) - lineStart
	openers := map[string]bool{")": true, "]": true, "}": true, ";": true}
	var closers []byte
	for _, problem := range errors.Errors {
		if problem.Kind != "missing" || problem.Range.Start.Line != line {
			continue
		}
		expected := strings.TrimSpace(problem.Expected)
		if !openers[expected] || expected == "}" {
			continue
		}
		if problem.Range.Start.Col <= codeEndOffset &&
			!strings.HasSuffix(strings.TrimSpace(text[lineStart:lineStart+problem.Range.Start.Col]), expected) {
			closers = append(closers, expected...)
		}
	}
	change, ok := completeStatementAt(
		text, cursor, language, editorIndentUnit(t.state),
	)
	if !ok {
		return false
	}
	inserted := string(closers)
	change.insert = strings.TrimPrefix(change.insert, inserted)
	t.state.Buffer.Edit(editor.Edit{
		Start: change.start, End: change.end, Text: inserted + change.insert,
	})
	t.state.Select(change.caret, change.caret)
	return true
}

type statementCompletion struct {
	start  int
	end    int
	insert string
	caret  int
}

func completeStatementAt(text string, cursor int, language, unit string) (statementCompletion, bool) {
	cursor = max(0, min(cursor, len(text)))
	lineStart := strings.LastIndexByte(text[:cursor], '\n') + 1
	lineEnd := strings.IndexByte(text[cursor:], '\n')
	if lineEnd < 0 {
		lineEnd = len(text)
	} else {
		lineEnd += cursor
	}
	line := text[lineStart:lineEnd]
	if strings.TrimSpace(line) == "" {
		return statementCompletion{}, false
	}
	code := strings.TrimRight(line[:lineCodeEnd(line, language)], " \t")
	codeEnd := len(code)
	comment := strings.TrimSpace(line[codeEnd:])
	trimmed := strings.TrimSpace(code)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "--") ||
		strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
		return statementCompletion{}, false
	}
	baseIndent := gapLeadingWhitespace(line)
	if unit == "" {
		unit = "    "
	}
	lineEnding := textLineEnding(text)
	originalFrom := lineStart + codeEnd
	blockHead := statementBlockHead(trimmed, language) &&
		!unclosedStatementBracketsHeader(trimmed)
	if blockHead && !strings.HasSuffix(code, "{") && !strings.HasSuffix(code, ":") {
		headerSuffix := " {"
		insert := headerSuffix + lineEnding + baseIndent + unit +
			lineEnding + baseIndent + "}"
		if comment != "" {
			insert = headerSuffix + " " + comment + lineEnding +
				baseIndent + unit + lineEnding + baseIndent + "}"
		}
		return statementCompletion{
			start: originalFrom, end: lineEnd, insert: insert,
			caret: originalFrom + len(headerSuffix) +
				lenOptionalSpacedComment(comment) + len(lineEnding) +
				len(baseIndent) + len(unit),
		}, true
	}
	finalCode, finish, nextDepth := finalizeStatementCode(code, language)
	textualBlockHead := statementBlockHead(trimmed, language)
	if (language == "python" || language == "gdscript") && textualBlockHead &&
		!strings.HasSuffix(finalCode, ":") {
		finish = ":"
	}
	nextIndent := baseIndent
	if nextDepth > 0 {
		nextIndent += strings.Repeat(unit, nextDepth)
	}
	replacement := finalCode[len(code):] + finish
	if comment != "" {
		replacement += " " + comment
	}
	replacement += lineEnding + nextIndent
	return statementCompletion{
		start: originalFrom, end: lineEnd, insert: replacement,
		caret: originalFrom + len(replacement),
	}, true
}

// completeStatementEdits keeps the parser-repaired prefix unchanged, inserts
// the missing closers and punctuation before a trailing comment, and appends
// the next indented line as a second non-overlapping edit.
func completeStatementEdits(text string, change statementCompletion) []editor.Edit {
	change.start = max(0, min(change.start, len(text)))
	change.end = max(change.start, min(change.end, len(text)))
	lineEnd := strings.IndexByte(text[change.end:], '\n')
	if lineEnd >= 0 {
		lineEnd += change.end
	} else {
		lineEnd = len(text)
	}
	commentStart := len(change.insert)
	newline := strings.Index(change.insert, textLineEnding(text))
	if newline >= 0 {
		commentStart = newline
	}
	inserted := change.insert[:commentStart]
	if !strings.Contains(inserted, commentHint(text[change.start:lineEnd])) {
		return []editor.Edit{{Start: change.start, End: lineEnd, Text: change.insert}}
	}
	closing := strings.TrimSuffix(inserted, commentHint(text[change.start:lineEnd]))
	closing = strings.TrimRight(closing, " \t")
	tail := change.insert[len(inserted):]
	edits := make([]editor.Edit, 0, 2)
	if closing != "" {
		edits = append(edits, editor.Edit{
			Start: change.start, End: change.start, Text: closing,
		})
	}
	if lineEnd < len(text) {
		lineEnd++
	}
	edits = append(edits, editor.Edit{
		Start: lineEnd, End: lineEnd, Text: tail,
	})
	return edits
}

func commentHint(rest string) string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "//") || strings.HasPrefix(rest, "#") ||
		strings.HasPrefix(rest, "--") || strings.HasPrefix(rest, "/*") {
		return rest
	}
	return ""
}

func lenOptionalSpacedComment(comment string) int {
	if comment == "" {
		return 0
	}
	return 1 + len(comment)
}

func finalizeStatementCode(code, language string) (text, finish string, nextDepth int) {
	text = code
	for _, closer := range unclosedStatementBrackets(text) {
		text += closer
	}
	braces := unclosedStatementBrace(text)
	if braces != "" {
		text += braces
		nextDepth = len(braces)
	}
	semicolon := map[string]bool{
		"java": true, "kotlin": true, "scala": true, "groovy": true,
		"c": true, "cpp": true, "objective-c": true, "objective-cpp": true,
		"csharp": true, "javascript": true, "javascriptreact": true,
		"typescript": true, "typescriptreact": true, "dart": true,
		"swift": true, "php": true,
	}[language]
	if semicolon && !strings.HasSuffix(text, ";") &&
		!strings.HasSuffix(text, "{") && !strings.HasSuffix(text, "}") &&
		!strings.HasSuffix(text, ",") && !strings.HasSuffix(text, ":") {
		finish = ";"
	}
	return text, finish, nextDepth
}

func unclosedStatementBrackets(code string) []string {
	stack := []byte{}
	quote := byte(0)
	for index := 0; index < len(code); index++ {
		c := code[index]
		if quote != 0 {
			if c == '\\' {
				index++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '(', '[', '{':
			stack = append(stack, c)
		case ')', ']', '}':
			if len(stack) == 0 {
				continue
			}
			open := stack[len(stack)-1]
			if open == '(' && c == ')' ||
				open == '[' && c == ']' ||
				open == '{' && c == '}' {
				stack = stack[:len(stack)-1]
			}
		}
	}
	out := make([]string, 0, len(stack))
	for index := len(stack) - 1; index >= 0; index-- {
		switch stack[index] {
		case '(':
			out = append(out, ")")
		case '[':
			out = append(out, "]")
		}
	}
	return out
}

func unclosedStatementBrace(code string) string {
	depth := 0
	quote := byte(0)
	for index := 0; index < len(code); index++ {
		c := code[index]
		if quote != 0 {
			if c == '\\' {
				index++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '{':
			depth++
		case '}':
			depth--
		}
	}
	if depth > 0 {
		return strings.Repeat("}", depth)
	}
	return ""
}

func lineCodeEnd(line, language string) int {
	quote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return i
			}
		case '#':
			if language == "python" || language == "ruby" || language == "shell" ||
				language == "bash" {
				return i
			}
		}
	}
	return len(line)
}

func statementBlockHead(code, language string) bool {
	lower := strings.ToLower(strings.TrimSpace(code))
	if strings.HasSuffix(lower, "{") || strings.HasSuffix(lower, ":") {
		return true
	}
	for _, prefix := range []string{
		"if ", "for ", "while ", "switch ", "case ", "try", "catch ", "else",
		"do", "def ", "function ", "class ", "struct ", "interface ", "enum ",
		"func ", "fn ", "match ", "when ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return language == "ruby" && strings.HasSuffix(lower, " do")
}

func unclosedStatementBracketsHeader(code string) bool {
	stack := []byte{}
	quote := byte(0)
	for index := 0; index < len(code); index++ {
		c := code[index]
		if quote != 0 {
			if c == '\\' {
				index++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '(', '[':
			stack = append(stack, c)
		case ')', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return len(stack) > 0
}

func textLineEnding(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// cyclicExpandEditorWord completes the prefix at the caret from this file,
// then from other open documents. Repeated calls cycle and return to the
// original prefix without changing undo grouping.
func (m *Model) cyclicExpandEditorWord(backward bool) {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil || !m.activeEditorEditable() {
		return
	}
	state := &t.wordExpand
	direction := directionForBackward(backward)
	version := t.state.Buffer.Version()
	cursor := t.state.Cursor()
	if len(state.candidates) > 0 && state.end == cursor &&
		state.sourceVersion == version && state.direction == direction &&
		t.state.Selection().Empty() {
		state.index = (state.index + 1) % (len(state.candidates) + 1)
		insert := state.prefix
		if state.index < len(state.candidates) {
			insert = state.candidates[state.index]
		}
		t.state.Buffer.Edit(editor.Edit{
			Start: state.from, End: state.end, Text: insert,
		})
		state.end = state.from + len(insert)
		state.sourceVersion = t.state.Buffer.Version()
		t.state.Select(state.end, state.end)
		m.invalidate()
		return
	}
	text := t.state.Buffer.String()
	from := editorWordPrefixStart(text, cursor)
	if t.wordExpand.cancelled {
		t.wordExpand.cancelled = false
		from = cursor
	}
	if from == cursor {
		m.status = "No word to expand at the caret"
		m.invalidate()
		return
	}
	prefix := text[from:cursor]
	candidates := editorWordCandidates(text, from, cursor, prefix, backward)
	candidates = append(candidates, m.otherEditorWords(t, func(word string) bool {
		return len(word) > len(prefix) && strings.HasPrefix(word, prefix)
	})...)
	candidates = dedupeStrings(candidates)
	if len(candidates) == 0 {
		m.status = "No suitable completions found"
		m.invalidate()
		return
	}
	newSession := state.from == 0 && state.end == 0 && state.direction == "" ||
		state.from != from || state.end != cursor ||
		state.direction != direction ||
		state.sourceVersion != t.state.Buffer.Version() ||
		!gapSameStrings(state.candidates, candidates)
	if newSession {
		state.from = from
		state.end = cursor
		state.prefix = prefix
		state.candidates = candidates
		state.index = 0
		state.direction = direction
		state.sourceVersion = t.state.Buffer.Version()
	} else {
		state.index = (state.index + 1) % (len(candidates) + 1)
	}
	insert := prefix
	if state.index < len(candidates) {
		insert = candidates[state.index]
	}
	t.state.Buffer.Edit(editor.Edit{Start: from, End: cursor, Text: insert})
	state.end = from + len(insert)
	state.sourceVersion = t.state.Buffer.Version()
	state.candidates = candidates
	t.state.Select(state.end, state.end)
	m.invalidate()
}

func directionForBackward(backward bool) string {
	if backward {
		return "backward"
	}
	return "forward"
}

type editorWordExpansion struct {
	from          int
	end           int
	prefix        string
	candidates    []string
	index         int
	sourceVersion uint64
	direction     string
	cancelled     bool
}

func editorWordPrefixStart(text string, caret int) int {
	i := min(max(0, caret), len(text))
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:i])
		if !editorWordRune(r) {
			break
		}
		i -= size
	}
	return i
}

func editorWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '$'
}

func editorWordCandidates(text string, from, caret int, prefix string, backward bool) []string {
	before := []string{}
	after := []string{}
	for index := 0; index < len(text); {
		r, size := utf8.DecodeRuneInString(text[index:])
		if !editorWordRune(r) {
			index += size
			continue
		}
		start := index
		for index < len(text) {
			r, size = utf8.DecodeRuneInString(text[index:])
			if !editorWordRune(r) {
				break
			}
			index += size
		}
		word := text[start:index]
		if start <= from && start+len(word) >= caret {
			continue
		}
		if len(word) <= len(prefix) || !strings.HasPrefix(word, prefix) {
			continue
		}
		if start < from {
			before = append(before, word)
		} else {
			after = append(after, word)
		}
	}
	reverseStrings(before)
	if backward {
		return append(before, after...)
	}
	return append(after, before...)
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := values[:0]
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func gapSameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (m *Model) otherEditorWords(current *Tab, keep func(string) bool) []string {
	var out []string
	budget := actionGapMaxOtherFiles
	groups := [][]*Tab{m.tabs}
	if m.split != nil {
		groups = append(groups, m.split.tabs)
	}
	seen := map[string]bool{}
	if current != nil && current.doc != nil {
		seen[realPath(current.doc.Path())] = true
	}
	for _, tabs := range groups {
		for _, tab := range tabs {
			if tab == nil || tab == current || tab.doc == nil || tab.state == nil ||
				tab.state.Buffer == nil {
				continue
			}
			path := realPath(tab.doc.Path())
			if seen[path] {
				continue
			}
			seen[path] = true
			text := tab.state.Buffer.String()
			if len(text) > budget {
				continue
			}
			budget -= len(text)
			for index := 0; index < len(text); {
				r, size := utf8.DecodeRuneInString(text[index:])
				if !editorWordRune(r) {
					index += size
					continue
				}
				start := index
				for index < len(text) {
					r, size = utf8.DecodeRuneInString(text[index:])
					if !editorWordRune(r) {
						break
					}
					index += size
				}
				if word := text[start:index]; keep(word) {
					out = append(out, word)
				}
			}
		}
	}
	return out
}

// moveEditorElement swaps the comma-separated list element at the caret with
// its neighbour, choosing the nearest enclosing list that has one.
func (m *Model) moveEditorElement(direction int) {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil || !m.activeEditorEditable() {
		return
	}
	if len(t.state.Selections()) != 1 {
		return
	}
	text := t.state.Buffer.String()
	cursor := t.state.Cursor()
	swap, ok := editorElementSwap(text, cursor, direction)
	if !ok {
		m.status = "No list element to move here"
		m.invalidate()
		return
	}
	a := text[swap.aStart:swap.aEnd]
	b := text[swap.bStart:swap.bEnd]
	var first, second editor.Edit
	if swap.aStart < swap.bStart {
		first = editor.Edit{Start: swap.aStart, End: swap.aEnd, Text: b}
		second = editor.Edit{Start: swap.bStart, End: swap.bEnd, Text: a}
	} else {
		first = editor.Edit{Start: swap.bStart, End: swap.bEnd, Text: a}
		second = editor.Edit{Start: swap.aStart, End: swap.aEnd, Text: b}
	}
	t.state.Buffer.Edit(first, second)
	t.state.Select(swap.caret, swap.caret)
	m.finishEditorTransform(t)
}

type actionGapSyntaxNode struct {
	Kind     string                `json:"kind"`
	Range    openidea.OutlineRange `json:"range"`
	Field    *string               `json:"field"`
	Named    bool                  `json:"named"`
	Error    bool                  `json:"error"`
	Missing  bool                  `json:"missing"`
	HasError bool                  `json:"hasError"`
}

type actionGapStructuralRanges struct {
	Statement       *actionGapSyntaxNode `json:"statement"`
	PrevStatement   *actionGapSyntaxNode `json:"prevStatement"`
	NextStatement   *actionGapSyntaxNode `json:"nextStatement"`
	Block           *actionGapSyntaxNode `json:"block"`
	Declaration     *actionGapSyntaxNode `json:"declaration"`
	PrevDeclaration *actionGapSyntaxNode `json:"prevDeclaration"`
	NextDeclaration *actionGapSyntaxNode `json:"nextDeclaration"`
}

// moveEditorStatement asks the backend's tree-sitter structure for the
// caret's statement and swaps it with the previous or next statement. The
// server nodes decide syntax and nesting. If no grammar or usable neighbor is
// available, the editor's line movement remains the fallback.
func (m *Model) moveEditorStatement(up bool) {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil || t.state.Buffer == nil ||
		!m.activeEditorEditable() {
		return
	}
	if len(t.state.Selections()) != 1 || !t.state.Selection().Empty() {
		if up {
			m.runSyncEditorCommand(editor.MoveLinesUp)
		} else {
			m.runSyncEditorCommand(editor.MoveLinesDown)
		}
		return
	}
	if m.ctx == nil || m.invokeHook == nil && m.mgr == nil {
		if up {
			m.runSyncEditorCommand(editor.MoveLinesUp)
		} else {
			m.runSyncEditorCommand(editor.MoveLinesDown)
		}
		return
	}
	docID := t.doc.ID()
	version := t.state.Buffer.Version()
	caret := t.state.Cursor()
	selection := t.state.Selection()
	pos := t.doc.UTF16Pos(caret)
	request := m.nextRequest()
	go func() {
		ctx, cancel := context.WithTimeout(gapModelContext(m), actionGapTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "syntax_structural_ranges", map[string]any{
			"doc": docID, "pos": lspPos(pos), "request": request,
		})
		var ranges actionGapStructuralRanges
		if err == nil {
			err = json.Unmarshal(raw, &ranges)
		}
		m.post(func() {
			if !m.tabOpen(t) || t.doc.ID() != docID ||
				t.state.Buffer.Version() != version || t.state.Cursor() != caret {
				return
			}
			if err == nil {
				if node := actionGapMoveNeighbor(ranges, up); node != nil {
					if m.applyActionGapStatementMove(t, ranges.Statement, node, selection, up) {
						return
					}
				}
			}
			if up {
				m.runSyncEditorCommand(editor.MoveLinesUp)
			} else {
				m.runSyncEditorCommand(editor.MoveLinesDown)
			}
		})
	}()
}

func (m *Model) runSyncEditorCommand(command editor.Command) {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	if t.state.Run(command) {
		m.invalidate()
	}
}

func actionGapMoveNeighbor(ranges actionGapStructuralRanges, up bool) *actionGapSyntaxNode {
	if ranges.Statement == nil || ranges.Statement.HasError {
		return nil
	}
	var neighbor *actionGapSyntaxNode
	if up {
		neighbor = ranges.PrevStatement
	} else {
		neighbor = ranges.NextStatement
	}
	if neighbor == nil || neighbor.HasError {
		return nil
	}
	return neighbor
}

func (m *Model) applyActionGapStatementMove(
	t *Tab,
	current, neighbor *actionGapSyntaxNode,
	selection editor.Selection,
	up bool,
) bool {
	if t == nil || t.doc == nil || t.state == nil || current == nil || neighbor == nil {
		return false
	}
	snapshot := t.state.Buffer.Snapshot()
	currentStart, currentEnd := actionGapLineSpan(t.doc, snapshot, current.Range)
	neighborStart, neighborEnd := actionGapLineSpan(t.doc, snapshot, neighbor.Range)
	if currentStart < 0 || currentEnd <= currentStart ||
		neighborStart < 0 || neighborEnd <= neighborStart ||
		currentStart < currentEnd && neighborStart < currentEnd &&
			neighborStart > currentStart ||
		up && neighborStart >= currentStart || !up && neighborStart <= currentStart {
		return false
	}
	currentText := snapshot.Slice(currentStart, currentEnd)
	neighborText := snapshot.Slice(neighborStart, neighborEnd)
	var edits []editor.Edit
	newAnchor, newHead := selection.Anchor, selection.Head
	if up {
		edits = []editor.Edit{
			{Start: neighborStart, End: neighborEnd, Text: currentText},
			{Start: currentStart, End: currentEnd, Text: neighborText},
		}
		delta := neighborStart - currentStart
		newAnchor += delta
		newHead += delta
	} else {
		edits = []editor.Edit{
			{Start: currentStart, End: currentEnd, Text: neighborText},
			{Start: neighborStart, End: neighborEnd, Text: currentText},
		}
		delta := neighborEnd - currentEnd
		newAnchor += delta
		newHead += delta
	}
	t.state.Buffer.Edit(edits...)
	t.state.SetSelections(editor.Selection{Anchor: newAnchor, Head: newHead})
	m.finishEditorTransform(t)
	return true
}

func actionGapLineSpan(_ *docbridge.Document, snapshot editor.Snapshot, r openidea.OutlineRange) (int, int) {
	first := int(r.Start.Line)
	last := int(r.End.Line)
	if last < first {
		last = first
	}
	startLine := max(0, min(first, snapshot.LineCount()-1))
	endLine := max(startLine, min(last, snapshot.LineCount()-1))
	start := snapshot.LineStart(startLine)
	end := snapshot.LineEnd(endLine)
	if endLine+1 < snapshot.LineCount() {
		end = snapshot.LineStart(endLine + 1)
	} else if end < snapshot.Len() {
		end = snapshot.Len()
	}
	return start, end
}

type editorElement struct {
	start int
	end   int
}

type editorSwap struct {
	aStart int
	aEnd   int
	bStart int
	bEnd   int
	caret  int
}

func editorElementSwap(text string, cursor, direction int) (editorSwap, bool) {
	items := []editorElement{}
	at := cursor
	for depth := 0; depth < 16 && len(items) < 2; depth++ {
		open, close, ok := enclosingCommaList(text, at)
		if !ok {
			return editorSwap{}, false
		}
		items = commaListElements(text, open+1, close)
		at = open
	}
	if len(items) < 2 || direction == 0 {
		return editorSwap{}, false
	}
	index := -1
	for i, item := range items {
		if item.start <= cursor && cursor <= item.end {
			index = i
			break
		}
	}
	if index < 0 {
		return editorSwap{}, false
	}
	next := index
	if direction < 0 {
		next--
	} else {
		next++
	}
	if next < 0 || next >= len(items) {
		return editorSwap{}, false
	}
	current, other := items[index], items[next]
	offset := max(0, min(cursor-current.start, current.end-current.start))
	start := other.start
	if direction > 0 {
		start = other.end - (current.end - current.start)
	}
	return editorSwap{
		aStart: current.start, aEnd: current.end,
		bStart: other.start, bEnd: other.end,
		caret: start + offset,
	}, true
}

func enclosingCommaList(text string, pos int) (int, int, bool) {
	pos = max(0, min(pos, len(text)))
	type bracketPair struct {
		open  int
		close int
	}
	stack := []bracketPair{}
	best := bracketPair{}
	quote := byte(0)
	lineComment := false
	blockComment := false
	for index := 0; index < len(text); index++ {
		c := text[index]
		if lineComment {
			if c == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if c == '*' && index+1 < len(text) && text[index+1] == '/' {
				blockComment = false
				index++
			}
			continue
		}
		if quote != 0 {
			if c == '\\' {
				index++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch {
		case c == '/' && index+1 < len(text) && text[index+1] == '/':
			lineComment = true
			index++
		case c == '/' && index+1 < len(text) && text[index+1] == '*':
			blockComment = true
			index++
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '(' || c == '[' || c == '{':
			stack = append(stack, bracketPair{open: index})
		case c == ')' || c == ']' || c == '}':
			if len(stack) == 0 {
				continue
			}
			pair := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !bracketsMatch(text[pair.open], c) {
				continue
			}
			if pair.open < pos && pos <= index &&
				(index-pair.open < best.close-best.open ||
					best.close == 0) {
				best = bracketPair{open: pair.open, close: index}
			}
		}
	}
	if best.close == 0 {
		return 0, 0, false
	}
	return best.open, best.close, true
}

func bracketsMatch(open, close byte) bool {
	return open == '(' && close == ')' ||
		open == '[' && close == ']' ||
		open == '{' && close == '}'
}

func commaListElements(text string, from, to int) []editorElement {
	from = max(0, min(from, len(text)))
	to = max(from, min(to, len(text)))
	out := []editorElement{}
	depth := 0
	quote := byte(0)
	lineComment := false
	blockComment := false
	start := from
	push := func(end int) {
		raw := text[start:end]
		lead := len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return
		}
		out = append(out, editorElement{start: start + lead, end: start + lead + len(trimmed)})
	}
	for i := from; i < to; i++ {
		c := text[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if c == '*' && i+1 < to && text[i+1] == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch {
		case c == '/' && i+1 < to && text[i+1] == '/':
			lineComment = true
			i++
		case c == '/' && i+1 < to && text[i+1] == '*':
			blockComment = true
			i++
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			push(i)
			start = i + 1
		}
	}
	push(to)
	return out
}

func isQuoteOrCommentByte(text string, pos int) bool {
	if pos < 0 || pos >= len(text) {
		return false
	}
	quote := byte(0)
	depthLine := 0
	depthBlock := 0
	for i := 0; i <= pos; i++ {
		c := text[i]
		if depthLine != 0 {
			if c == '\n' {
				depthLine = 0
			}
			continue
		}
		if depthBlock != 0 {
			if c == '*' && i+1 <= pos && text[i+1] == '/' {
				depthBlock = 0
				i++
			}
			continue
		}
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '/':
			if i+1 <= pos && text[i+1] == '/' {
				depthLine = 1
				i++
			} else if i+1 <= pos && text[i+1] == '*' {
				depthBlock = 1
				i++
			}
		}
	}
	return quote != 0 || depthLine != 0 || depthBlock != 0 ||
		pos > 0 && (text[pos-1] == '/' || text[pos-1] == '*' || text[pos-1] == '`' ||
			text[pos-1] == '"' || text[pos-1] == '\'')
}

// unwrapEditor removes the nearest ordinary block around the caret. Blocks
// that contain multiple statements are offered in the native chooser.
func (m *Model) unwrapEditor() {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil || !m.activeEditorEditable() {
		return
	}
	text := t.state.Buffer.String()
	options := editorUnwrapOptions(text, t.state.Cursor())
	if len(options) == 0 {
		m.status = "Nothing to unwrap here"
		m.invalidate()
		return
	}
	if len(options) == 1 {
		m.applyEditorUnwrap(options[0])
		return
	}
	m.unwrap = &unwrapPopupState{open: true, options: options, selected: 0}
	m.invalidate()
}

type editorUnwrapOption struct {
	label  string
	start  int
	end    int
	insert string
}

func editorUnwrapOptions(text string, cursor int) []editorUnwrapOption {
	var options []editorUnwrapOption
	seen := map[string]bool{}
	for pos := cursor; pos >= 0; {
		open := strings.LastIndexByte(text[:pos], '{')
		if open < 0 {
			break
		}
		close, ok := matchingBrace(text, open)
		if !ok || close < cursor {
			pos = open
			continue
		}
		inner := text[open+1 : close]
		if strings.TrimSpace(inner) != "" {
			insert, caret := unwrapBlockBody(inner)
			key := strconv.Itoa(open) + ":" + strconv.Itoa(close)
			if !seen[key] {
				seen[key] = true
				_ = caret
				options = append(options, editorUnwrapOption{
					label: "Remove braces " + shortEditorRange(text, open, close),
					start: open, end: close + 1, insert: insert,
				})
			}
		}
		pos = open
	}
	if ifStart, ifEnd, ok := enclosingIfStatement(text, cursor); ok {
		options = append([]editorUnwrapOption{{
			label: "Remove if condition", start: ifStart, end: ifEnd,
			insert: text[ifStart:ifEnd],
		}}, options...)
	}
	sort.SliceStable(options, func(i, j int) bool {
		return options[i].end-options[i].start < options[j].end-options[j].start
	})
	return options
}

func matchingBrace(text string, open int) (int, bool) {
	depth := 0
	quote := byte(0)
	for i := open + 1; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return i, true
			}
			depth--
		}
	}
	return 0, false
}

func unwrapBlockBody(inner string) (string, int) {
	lineStart := strings.LastIndexByte(inner, '\n')
	before := ""
	after := "\n"
	if lineStart >= 0 {
		before = ""
	}
	body := strings.TrimSpace(inner)
	if !strings.Contains(body, "\n") {
		return before + body, len(before + body)
	}
	base := ""
	if lineStart+1 < len(inner) {
		base = gapLeadingWhitespace(inner[lineStart+1:])
	}
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = strings.TrimPrefix(lines[i], base)
	}
	out := before + strings.Join(lines, "\n") + after
	return out, len(out)
}

func shortEditorRange(text string, start, end int) string {
	label := strings.TrimSpace(text[start:min(end, len(text))])
	label = strings.Join(strings.Fields(label), " ")
	if len(label) > 48 {
		label = label[:47] + "…"
	}
	if label == "" {
		return "(empty block)"
	}
	return "from " + label
}

func enclosingIfStatement(text string, cursor int) (int, int, bool) {
	start := strings.LastIndex(text[:cursor], "if ")
	if start < 0 {
		return 0, 0, false
	}
	lineStart := strings.LastIndexByte(text[:start], '\n') + 1
	lineEnd := strings.IndexByte(text[start:], '\n')
	if lineEnd < 0 {
		return 0, 0, false
	}
	lineEnd += start
	brace := strings.IndexByte(text[start:lineEnd], '{')
	if brace < 0 {
		return 0, 0, false
	}
	brace += start
	close, ok := matchingBrace(text, brace)
	if !ok || cursor > close {
		return 0, 0, false
	}
	return lineStart, min(len(text), closeLineEnd(text, close)), true
}

func closeLineEnd(text string, close int) int {
	if next := strings.IndexByte(text[close+1:], '\n'); next >= 0 {
		return close + 1 + next + 1
	}
	return len(text)
}

type unwrapPopupState struct {
	open     bool
	options  []editorUnwrapOption
	selected int
	list     ui.ListState
}

func (m *Model) applyEditorUnwrap(option editorUnwrapOption) {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil || !m.activeEditorEditable() {
		return
	}
	insert := option.insert
	if strings.Contains(option.label, "if condition") {
		insert = stripIfWrapper(insert)
	}
	t.state.Buffer.Edit(editor.Edit{Start: option.start, End: option.end, Text: insert})
	caret := option.start + len(multilineIndent(insert))
	t.state.Select(caret, caret)
	m.unwrap = nil
	m.finishEditorTransform(t)
}

func multilineIndent(text string) string {
	if lineEnd := strings.IndexByte(text, '\n'); lineEnd >= 0 {
		return text[:lineEnd]
	}
	return gapLeadingWhitespace(text)
}

func stripIfWrapper(block string) string {
	open := strings.IndexByte(block, '{')
	close, ok := matchingBrace(block, open)
	if open < 0 || !ok {
		return strings.TrimSpace(block)
	}
	body, _ := unwrapBlockBody(block[open+1 : close])
	return body
}

// OpenReformatDialog deliberately routes through Settings › Code Style, where
// the backend formatter options live. Applying Reformat itself stays on the
// normal format action, so a dialog cannot silently format stale text.
func (m *Model) OpenReformatDialog() {
	if !m.activeEditorEditable() {
		return
	}
	m.OpenSettingsPage(settingsPageCodeStyleOther)
	m.status = "Reformat options · edits apply to the current file"
	m.invalidate()
}

// showEditorContextInfo reports the enclosing declaration chain using the
// backend outline already loaded for the active buffer.
func (m *Model) showEditorContextInfo() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	line := t.state.CursorPos().Line
	chain := outlinePath(t.outline, line)
	if len(chain) == 0 {
		m.status = editorContextText(t.state.Buffer.Snapshot(), line)
		m.invalidate()
		return
	}
	headers := make([]string, 0, len(chain))
	for _, item := range chain {
		headers = append(headers, strings.TrimSpace(item.Name))
	}
	m.status = strings.Join(headers, " › ")
	m.invalidate()
}

func editorContextText(snapshot editor.Snapshot, line int) string {
	var parts []string
	var scopes []foldingRange
	for _, region := range foldingRanges(snapshot, "    ") {
		if region.start < line && line <= region.end {
			scopes = append(scopes, region)
		}
	}
	sort.SliceStable(scopes, func(i, j int) bool {
		if scopes[i].depth != scopes[j].depth {
			return scopes[i].depth < scopes[j].depth
		}
		return scopes[i].start < scopes[j].start
	})
	for _, region := range scopes {
		if text := strings.TrimSpace(snapshot.Line(region.start)); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "No context information here"
	}
	return strings.Join(parts, " › ")
}

// toggleEditorInsertState flips per-tab overwrite mode. Typed text is
// intercepted in editorCaretInput, where it replaces at most the current line.
func (m *Model) toggleEditorInsertState() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	if t.state.ReadOnly || t.doc != nil && t.doc.ReadOnly() {
		m.status = "Overwrite mode is unavailable in a read-only editor"
		m.invalidate()
		return
	}
	t.insertOverwrite = !t.insertOverwrite
	if t.insertOverwrite {
		m.status = "Overwrite mode"
	} else {
		m.status = "Insert mode"
	}
	m.invalidate()
}

func (m *Model) overwriteEditorInput(t *Tab, ev ui.InputEvent) bool {
	if t == nil || t.state == nil || t.state.Buffer == nil || !t.insertOverwrite ||
		ev.Kind != ui.InputText || ev.Text == "" || strings.ContainsAny(ev.Text, "\r\n") ||
		!m.activeEditorEditable() {
		return false
	}
	text := t.state.Buffer.String()
	var edits []editor.Edit
	selections := t.state.Selections()
	carets := make([]editor.Selection, 0, len(selections))
	for _, selection := range selections {
		from, to := selection.Start(), selection.End()
		if selection.Empty() {
			line := strings.LastIndexByte(text[:min(from, len(text))], '\n') + 1
			lineEnd := strings.IndexByte(text[min(from, len(text)):], '\n')
			if lineEnd < 0 {
				lineEnd = len(text)
			} else {
				lineEnd += from
			}
			to = min(lineEnd, from+len(ev.Text))
			_ = line
		}
		edits = append(edits, editor.Edit{Start: from, End: to, Text: ev.Text})
		carets = append(carets, editor.Selection{Anchor: from + len(ev.Text), Head: from + len(ev.Text)})
	}
	t.state.Buffer.Edit(edits...)
	t.state.SetSelections(carets...)
	m.finishEditorTransform(t)
	return true
}

func (m *Model) finishEditorTransform(t *Tab) {
	if t != nil && t.doc != nil {
		m.markAutoImportDirty(t)
		m.promoteTab(t)
	}
	m.invalidate()
}

// stepFindPopupResult walks the existing Find results in reading order. It
// uses the same preview path as a click and opens in a regular tab on Enter.
func (m *Model) stepFindPopupResult(direction int) {
	if direction == 0 {
		return
	}
	s := &m.search
	if len(s.files) == 0 {
		m.status = "No Find results to step through"
		m.invalidate()
		return
	}
	index, total := searchResultIndex(s)
	if total == 0 {
		return
	}
	next := (index + direction + total) % total
	file, match, ok := searchResultAt(s, next)
	if !ok {
		return
	}
	s.selectedFile, s.selectedMatch = file, match
	s.refreshPreview()
	s.scrollSelected = true
	m.previewSearchResult(file, match)
	m.status = fmt.Sprintf("Result %d of %d", next+1, total)
	m.invalidate()
}

func (m *Model) openFindPopupResult() {
	s := &m.search
	if len(s.files) == 0 {
		m.status = "No Find result to open"
		m.invalidate()
		return
	}
	file, match, ok := searchResultAt(s, searchResultIndexValue(s))
	if !ok {
		return
	}
	s.selectedFile, s.selectedMatch = file, match
	hit := s.files[file].Matches[match]
	col := uint32(0)
	if len(hit.Ranges) > 0 {
		col = hit.Ranges[0][0]
	}
	s.historyOpen = false
	m.revealAt(s.files[file].Path, int(hit.Line), int(col))
}

func searchResultIndex(s *SearchPane) (index, total int) {
	total = 0
	for _, file := range s.files {
		total += len(file.Matches)
	}
	return searchResultIndexValue(s), total
}

func searchResultIndexValue(s *SearchPane) int {
	index := 0
	for fileIndex := 0; fileIndex < s.selectedFile && fileIndex < len(s.files); fileIndex++ {
		index += len(s.files[fileIndex].Matches)
	}
	index += s.selectedMatch
	return max(0, index)
}

func searchResultAt(s *SearchPane, index int) (int, int, bool) {
	if index < 0 {
		return 0, 0, false
	}
	for fileIndex, file := range s.files {
		if index < len(file.Matches) {
			return fileIndex, index, true
		}
		index -= len(file.Matches)
	}
	return 0, 0, false
}

func (m *Model) previewSearchResult(file, match int) {
	s := &m.search
	if file < 0 || file >= len(s.files) || match < 0 || match >= len(s.files[file].Matches) {
		return
	}
	hit := s.files[file].Matches[match]
	col := 0
	if len(hit.Ranges) > 0 {
		col = int(hit.Ranges[0][0])
	}
	m.previewAt(s.files[file].Path, int(hit.Line), col)
}

// goToAdjacentMethod uses the document outline, falling back to lexical
// method/type headers while the language server is still indexing.
func (m *Model) goToAdjacentMethod(direction int) {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return
	}
	caret := t.state.CursorPos()
	stops := methodOutlineStops(t.outline)
	if len(stops) > 0 && caret.Line == 0 && caret.Col == 0 {
		stops = stops[:0]
		for _, item := range flattenOutlineItems(t.outline) {
			kind := strings.ToLower(item.Kind)
			if kind == "function" || kind == "method" || kind == "constructor" {
				stops = append(stops, editor.Pos{
					Line: int(item.SelectionRange.Start.Line),
					Col:  int(item.SelectionRange.Start.Col),
				})
			}
		}
		sort.SliceStable(stops, func(i, j int) bool {
			if stops[i].Line != stops[j].Line {
				return stops[i].Line < stops[j].Line
			}
			return stops[i].Col < stops[j].Col
		})
		stops = dedupePositions(stops)
	}
	if len(stops) == 0 {
		stops = methodTextStops(t.state.Buffer.String())
	}
	target, ok := adjacentMethodStop(stops, editor.Pos{Line: caret.Line, Col: caret.Col}, direction)
	if !ok {
		if direction > 0 {
			m.status = "No next method"
		} else {
			m.status = "No previous method"
		}
		m.invalidate()
		return
	}
	m.recordNavigation(true)
	m.goToBufferPosition(t, target)
	m.status = "Method: " + strings.TrimSpace(t.state.Buffer.Snapshot().Line(target.Line))
	m.invalidate()
}

func methodOutlineStops(items []openidea.OutlineItem) []editor.Pos {
	methods := map[string]bool{
		"function": true, "method": true, "constructor": true,
		"class": true, "interface": true, "struct": true, "enum": true,
		"trait": true, "module": true, "namespace": true,
	}
	var out []editor.Pos
	var walk func([]openidea.OutlineItem)
	walk = func(nodes []openidea.OutlineItem) {
		for _, node := range nodes {
			if methods[strings.ToLower(node.Kind)] {
				out = append(out, editor.Pos{
					Line: int(node.SelectionRange.Start.Line),
					Col:  int(node.SelectionRange.Start.Col),
				})
			}
			walk(node.Children)
		}
	}
	walk(items)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Col < out[j].Col
	})
	return dedupePositions(out)
}

func dedupePositions(values []editor.Pos) []editor.Pos {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func methodTextStops(text string) []editor.Pos {
	lines := strings.Split(text, "\n")
	var out []editor.Pos
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !looksLikeMemberHeader(trimmed) {
			continue
		}
		col := len(line) - len(strings.TrimLeft(line, " \t"))
		out = append(out, editor.Pos{Line: index, Col: col})
	}
	return out
}

func looksLikeMemberHeader(line string) bool {
	if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "*") {
		return false
	}
	if strings.Contains(line, "=") && !strings.Contains(line, "(") {
		return false
	}
	for _, prefix := range []string{
		"public ", "protected ", "private ", "static ", "abstract ", "final ",
		"class ", "interface ", "enum ", "struct ", "func ", "function ", "def ",
		"fn ", "void ", "int ", "String ", "boolean ", "async ", "export ",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return strings.Contains(line, "(") && strings.Contains(line, ")") &&
		(strings.Contains(line, "{") || strings.HasSuffix(line, ":"))
}

func adjacentMethodStop(stops []editor.Pos, caret editor.Pos, direction int) (editor.Pos, bool) {
	if direction > 0 {
		for _, stop := range stops {
			if stop.Line > caret.Line ||
				stop.Line == caret.Line && caret.Col == 0 && stop.Col > 0 ||
				stop.Line == caret.Line && caret.Col > 0 && stop.Col > caret.Col {
				return stop, true
			}
		}
		return editor.Pos{}, false
	}
	for index := len(stops) - 1; index >= 0; index-- {
		if stops[index].Less(caret) {
			return stops[index], true
		}
	}
	return editor.Pos{}, false
}

func (m *Model) goToBufferPosition(t *Tab, pos editor.Pos) {
	if t == nil || t.state == nil {
		return
	}
	t.state.GoTo(pos)
	m.focusApply = true
	m.invalidate()
}

// ShowCustomFoldingRegions lists // region, #region and editor-fold markers
// and moves the caret to the chosen start.
func (m *Model) ShowCustomFoldingRegions() {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return
	}
	regions := customFoldingRegions(t.state.Buffer.String())
	if len(regions) == 0 {
		m.status = "No custom folding regions in this file"
		m.invalidate()
		return
	}
	m.customRegions = &customRegionPopup{open: true, regions: regions}
	m.invalidate()
}

type customFoldingRegion struct {
	name  string
	start int
}

func customFoldingRegions(text string) []customFoldingRegion {
	lines := strings.Split(text, "\n")
	stack := []customFoldingRegion{}
	var out []customFoldingRegion
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name, ok := customRegionStart(trimmed); ok {
			region := customFoldingRegion{name: name, start: index}
			stack = append(stack, region)
		} else if customRegionEnd(trimmed) && len(stack) > 0 {
			out = append(out, stack[len(stack)-1])
			stack = stack[:len(stack)-1]
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

func customRegionStart(line string) (string, bool) {
	if strings.HasPrefix(line, "<!--") && strings.Contains(strings.ToLower(line), "<editor-fold") &&
		strings.Contains(strings.ToLower(line), "</editor-fold") {
		name := ""
		if match := regexpRegionDesc.FindStringSubmatch(line); len(match) > 1 {
			name = match[1]
		}
		return name, true
	}
	for _, prefix := range []string{"//", "#", ";"} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		for strings.HasPrefix(body, prefix) {
			body = strings.TrimSpace(strings.TrimPrefix(body, prefix))
		}
		lower := strings.ToLower(body)
		if !strings.HasPrefix(lower, "region") && !strings.HasPrefix(lower, "editor-fold") {
			continue
		}
		name := strings.TrimSpace(body[len(strings.Fields(body)[0]):])
		if match := regexpRegionDesc.FindStringSubmatch(body); len(match) > 1 {
			name = match[1]
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(body)), "<editor-fold") {
			if match := regexpRegionDesc.FindStringSubmatch(body); len(match) > 1 {
				name = match[1]
			}
		}
		name = strings.Trim(name, "\"'")
		return name, true
	}
	return "", false
}

func customRegionEnd(line string) bool {
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "<!--") && strings.Contains(lower, "</editor-fold") {
		return true
	}
	for _, prefix := range []string{"//", "#", ";"} {
		lower = strings.TrimSpace(strings.TrimPrefix(lower, prefix))
	}
	return strings.HasPrefix(lower, "endregion") ||
		strings.HasPrefix(lower, "</editor-fold")
}

type customRegionPopup struct {
	open     bool
	regions  []customFoldingRegion
	selected int
	list     ui.ListState
}

// GoToTestOrSubject follows OpenIDEA's naming conventions. A single match
// opens immediately; multiple matches use the native chooser below.
func (m *Model) GoToTestOrSubject() {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	path := t.doc.Path()
	name := filepath.Base(path)
	stem, ext := splitFileName(name)
	targetNames := testCounterpartNames(stem, ext, isTestEditorPath(path))
	if len(targetNames) == 0 {
		m.status = "No test or test subject name for this file"
		m.invalidate()
		return
	}
	if m.invokeHook == nil && (m.mgr == nil || m.ctx == nil) {
		m.status = "Test navigation requires the GoIDEA backend"
		m.invalidate()
		return
	}
	m.status = "Finding tests…"
	m.invalidate()
	root := m.Root()
	doc := t.doc
	t.testNavGen++
	gen := t.testNavGen
	go func() {
		ctx, cancel := context.WithTimeout(gapModelContext(m), actionGapTimeout)
		defer cancel()
		found := map[string]bool{}
		var firstErr error
		for _, targetName := range targetNames {
			raw, err := m.invoke(ctx, "find_files", map[string]any{
				"query": targetName, "limit": 100,
			})
			if err != nil {
				if firstErr == nil && openidea.ErrorKind(err) != "cancelled" {
					firstErr = err
				}
				continue
			}
			var files []openidea.FileMatch
			if err := json.Unmarshal(raw, &files); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			for _, file := range files {
				if filepath.Base(file.Path) == targetName && realPath(file.Path) != realPath(path) {
					found[file.Path] = true
				}
			}
		}
		m.post(func() {
			if !m.tabOpen(t) || t.doc != doc || t.testNavGen != gen {
				return
			}
			paths := make([]string, 0, len(found))
			for path := range found {
				paths = append(paths, path)
			}
			sort.SliceStable(paths, func(i, j int) bool {
				ri := rankTestCounterpart(root, path, paths[i])
				rj := rankTestCounterpart(root, path, paths[j])
				if ri != rj {
					return ri < rj
				}
				return paths[i] < paths[j]
			})
			switch {
			case len(paths) == 0 && firstErr != nil:
				m.status = "Finding tests failed: " + firstErr.Error()
			case len(paths) == 0:
				m.status = "No test or test subject found"
			case len(paths) == 1:
				m.status = ""
				m.revealAt(paths[0], 0, 0)
				return
			default:
				items := make([]testNavTarget, 0, len(paths))
				for _, candidate := range paths {
					items = append(items, testNavTarget{path: candidate, label: filepath.Base(candidate)})
				}
				m.testNav = &testNavigationPopup{open: true, items: items, selected: 0}
			}
			m.invalidate()
		})
	}()
}

type testNavTarget struct {
	path  string
	label string
}

type testNavigationPopup struct {
	open     bool
	items    []testNavTarget
	selected int
	list     ui.ListState
}

func splitFileName(name string) (string, string) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return stem, ext
}

func isTestEditorPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") ||
		strings.HasSuffix(base, "test.java") || strings.HasSuffix(base, "tests.java") ||
		strings.HasSuffix(base, "spec.java") || strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, "_test.py") || strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.tsx")
}

func testCounterpartNames(stem, ext string, test bool) []string {
	if test {
		base := stem
		for _, suffix := range []string{"Tests", "Test", "Spec", "_test", ".test", ".spec"} {
			base = strings.TrimSuffix(base, suffix)
		}
		if base == "" {
			return nil
		}
		return []string{base + ext}
	}
	var names []string
	for _, suffix := range []string{"Test", "Tests", "Spec"} {
		names = append(names, stem+suffix+ext)
	}
	names = append(names, stem+"_test"+ext, stem+".test"+ext, stem+".spec"+ext)
	return dedupeStrings(names)
}

func rankTestCounterpart(root, subject, candidate string) int {
	subjectDir := filepath.Dir(subject)
	candidateDir := filepath.Dir(candidate)
	if candidateDir == subjectDir {
		return 0
	}
	subjectRoot := testRootDirectory(root, subject)
	candidateRoot := testRootDirectory(root, candidate)
	if subjectRoot != "" && candidateRoot != "" && subjectRoot != candidateRoot {
		return 1
	}
	return 2
}

func testRootDirectory(root, path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for index, part := range parts {
		if part == "test" || part == "tests" || part == "src" {
			return strings.Join(parts[:index], "/")
		}
	}
	return projectRelative(root, filepath.Dir(path))
}

// ShowRelatedSymbols uses the current symbol as a workspace query. The native
// symbol picker then opens the chosen declaration with the normal stale-safe
// navigation path.
func (m *Model) ShowRelatedSymbols() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	word := wordAtCaret(t.state.Buffer.String(), t.state.Cursor())
	if word == "" {
		m.status = "Place the caret on a symbol"
		m.invalidate()
		return
	}
	m.OpenSymbols()
	if m.symbols == nil {
		return
	}
	m.symbols.query = word
	m.symbols.title = "Related Symbols"
	m.querySymbols()
}

// GoToSuperMethod asks the type hierarchy for supertypes and follows their
// declaration. Go/Rust fallback to implementation/declaration requests, as
// OpenIDEA does without an LSP type hierarchy.
func (m *Model) GoToSuperMethod() {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	if m.ctx == nil || m.invokeHook == nil && m.mgr == nil {
		m.status = "Super Method requires the GoIDEA backend"
		m.invalidate()
		return
	}
	docID := t.doc.ID()
	version := t.state.Buffer.Version()
	caret := t.state.Cursor()
	pos := t.doc.UTF16Pos(caret)
	language := editorStatementLanguage(t)
	t.hierarchyGen++
	gen := t.hierarchyGen
	m.status = "Finding super method…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(gapModelContext(m), actionGapTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_type_hierarchy", map[string]any{
			"doc": docID, "pos": lspPos(pos), "direction": "outgoing",
		})
		var targets []openidea.Location
		if err == nil {
			var typed []hierarchyItem
			if json.Unmarshal(raw, &typed) == nil {
				for _, item := range typed {
					targets = append(targets, openidea.Location{
						Path: item.Path, Range: item.SelectionRange, URI: item.URI,
					})
				}
			} else {
				err = fmt.Errorf("cannot read type hierarchy")
			}
		}
		if len(targets) == 0 && (err == nil || openidea.ErrorKind(err) == "unavailable") &&
			(language == "go" || language == "rust") {
			kind := "declaration"
			if language == "go" {
				kind = "implementation"
			}
			var rawFallback json.RawMessage
			rawFallback, err = m.invoke(ctx, "lsp_goto", map[string]any{
				"doc": docID, "pos": lspPos(pos), "kind": kind,
			})
			if err == nil {
				_ = json.Unmarshal(rawFallback, &targets)
			}
		}
		m.post(func() {
			if !m.tabOpen(t) || t.doc.ID() != docID || t.hierarchyGen != gen ||
				t.state.Buffer.Version() != version || t.state.Cursor() != caret {
				return
			}
			targets = dedupeLocations(targets)
			if len(targets) > 0 {
				targets = targets[:1]
			}
			switch {
			case err != nil && openidea.ErrorKind(err) != "unavailable":
				m.status = "Finding super method failed: " + err.Error()
			case len(targets) == 0:
				m.status = "No super method found"
			case strings.Contains(targets[0].Path, "://"):
				m.status = "The super method is in an external dependency and cannot open as a file"
			default:
				m.status = "Super method: " + wordAtCaret(t.state.Buffer.String(), caret)
				m.revealAt(targets[0].Path,
					int(targets[0].Range.Start.Line), int(targets[0].Range.Start.Col))
				return
			}
			m.invalidate()
		})
	}()
}

func dedupeLocations(locations []openidea.Location) []openidea.Location {
	seen := map[string]bool{}
	out := locations[:0]
	for _, location := range locations {
		key := fmt.Sprintf("%s:%d:%d:%d:%d", location.Path,
			location.Range.Start.Line, location.Range.Start.Col,
			location.Range.End.Line, location.Range.End.Col)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, location)
	}
	return out
}

// StartHierarchyRefactoring uses OpenIDEA's JDT hierarchy RPC. The backend
// computes the workspace edit and keeps it grouped for undo; this client
// applies the server-provided edit after checking that the original document
// is still the same version.
func (m *Model) StartHierarchyRefactoring(kind string) {
	t := m.tab()
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	if kind != "pullUp" && kind != "pushDown" {
		return
	}
	if t.doc.ReadOnly() || t.state.ReadOnly {
		m.status = "Cannot perform refactoring. The file is read-only"
		m.invalidate()
		return
	}
	if m.ctx == nil || m.invokeHook == nil && m.mgr == nil {
		m.status = "Hierarchy refactoring requires the GoIDEA backend"
		m.invalidate()
		return
	}
	docID := t.doc.ID()
	version := t.state.Buffer.Version()
	pos := t.doc.UTF16Pos(t.state.Cursor())
	t.hierarchyGen++
	gen := t.hierarchyGen
	m.status = "Checking hierarchy members…"
	m.invalidate()
	request := m.nextRequest()
	go func() {
		ctx, cancel := context.WithTimeout(gapModelContext(m), actionGapTimeout)
		defer cancel()
		if err := t.doc.Sync(ctx); err != nil {
			m.post(func() {
				if m.tabOpen(t) && t.hierarchyGen == gen {
					m.status = "Checking hierarchy members failed: " + err.Error()
					m.invalidate()
				}
			})
			return
		}
		raw, err := m.invoke(ctx, "lsp_hierarchy_members", map[string]any{
			"request": request, "doc": docID, "pos": lspPos(pos), "kind": kind,
		})
		var info hierarchyMembersInfo
		if err == nil {
			err = json.Unmarshal(raw, &info)
		}
		m.post(func() {
			if !m.tabOpen(t) || t.doc.ID() != docID || t.hierarchyGen != gen ||
				t.state.Buffer.Version() != version {
				return
			}
			if err != nil {
				m.status = hierarchyRefactorError(kind, err)
				m.invalidate()
				return
			}
			m.hierarchyRefactor = &hierarchyRefactorState{
				open: true, kind: kind, tab: t, doc: docID, version: version,
				pos: pos, info: info, target: firstHierarchyTarget(info, kind),
				selected: []bool{}, list: ui.ListState{},
			}
			for _, target := range info.Targets {
				if target.ID == m.hierarchyRefactor.target {
					m.hierarchyRefactor.targetLabel = target.QualifiedName
					break
				}
			}
			for _, member := range info.Members {
				m.hierarchyRefactor.selected = append(m.hierarchyRefactor.selected, member.Selected)
				m.hierarchyRefactor.makeAbstract = append(m.hierarchyRefactor.makeAbstract, false)
			}
			m.status = ""
			m.invalidate()
		})
	}()
}

type hierarchyMembersInfo struct {
	ClassName string                `json:"className"`
	Targets   []hierarchyClassInfo  `json:"targets"`
	Members   []hierarchyMemberInfo `json:"members"`
}

type hierarchyClassInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualifiedName"`
	Interface     bool   `json:"interface"`
}

type hierarchyMemberInfo struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Selected     bool   `json:"selected"`
	Abstractable bool   `json:"abstractable"`
}

type hierarchyRefactorState struct {
	open         bool
	kind         string
	tab          *Tab
	doc          uint32
	version      uint64
	pos          openidea.OutlinePos
	info         hierarchyMembersInfo
	target       string
	targetLabel  string
	selected     []bool
	makeAbstract []bool
	list         ui.ListState
	busy         bool
	error        string
	problems     []string
}

func firstHierarchyTarget(info hierarchyMembersInfo, kind string) string {
	if kind != "pullUp" || len(info.Targets) == 0 {
		return ""
	}
	return info.Targets[0].ID
}

func hierarchyRefactorError(kind string, err error) string {
	title := "Pull Members Up"
	if kind == "pushDown" {
		title = "Push Members Down"
	}
	if errors.Is(err, context.Canceled) || openidea.ErrorKind(err) == "cancelled" {
		return title + " cancelled"
	}
	return "Cannot perform refactoring. " + err.Error()
}

// confirmHierarchyRefactor checks the chosen target/members, then applies the
// server's workspace edit as one undoable operation.
func (m *Model) confirmHierarchyRefactor() {
	state := m.hierarchyRefactor
	if state == nil || state.busy {
		return
	}
	if !m.tabOpen(state.tab) || state.tab.doc.ID() != state.doc {
		m.hierarchyRefactor = nil
		return
	}
	choices := make([]hierarchyChoiceInfo, 0, len(state.info.Members))
	for index, member := range state.info.Members {
		if index >= len(state.selected) || !state.selected[index] {
			continue
		}
		makeAbstract := index < len(state.makeAbstract) && state.makeAbstract[index]
		if state.kind == "pullUp" {
			for _, target := range state.info.Targets {
				if target.ID == state.target && target.Interface && member.Kind == "method" {
					makeAbstract = true
				}
			}
		}
		choices = append(choices, hierarchyChoiceInfo{ID: member.ID, MakeAbstract: makeAbstract})
	}
	if len(choices) == 0 {
		state.error = "Select at least one member"
		m.invalidate()
		return
	}
	state.busy = true
	state.error = ""
	t := state.tab
	docID := state.doc
	version := state.version
	pos := state.pos
	target := state.target
	kind := state.kind
	request := m.nextRequest()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(gapModelContext(m), actionGapTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_hierarchy_refactor", map[string]any{
			"request": request, "doc": docID, "pos": lspPos(pos), "kind": kind,
			"target": gapNullableString(target), "choices": choices,
		})
		var result hierarchyRefactoredInfo
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		if err != nil {
			m.post(func() {
				if state != m.hierarchyRefactor {
					return
				}
				state.busy = false
				state.error = hierarchyRefactorError(kind, err)
				m.invalidate()
			})
			return
		}
		fatal := ""
		for _, problem := range result.Problems {
			if problem.Severity == "fatal" && fatal == "" {
				fatal = problem.Message
			} else if problem.Severity == "error" || problem.Severity == "warning" {
				m.post(func() {
					if state == m.hierarchyRefactor {
						state.problems = append(state.problems, problem.Message)
					}
				})
			}
		}
		if fatal != "" || len(result.Edit) == 0 || string(result.Edit) == "null" {
			m.post(func() {
				if state != m.hierarchyRefactor {
					return
				}
				state.busy = false
				if fatal != "" {
					state.error = "Cannot perform refactoring. " + fatal
				} else {
					state.error = "Nothing to change"
				}
				m.invalidate()
			})
			return
		}
		if !m.tabOpen(t) || t.doc.ID() != docID || t.state.Buffer.Version() != version {
			m.post(func() {
				if state != m.hierarchyRefactor {
					return
				}
				state.busy = false
				state.error = "The file changed; open the refactoring again"
				m.invalidate()
			})
			return
		}
		_, err = m.invoke(ctx, "apply_workspace_edit", map[string]any{"edit": result.Edit})
		m.post(func() {
			if state != m.hierarchyRefactor {
				return
			}
			state.busy = false
			if err != nil {
				state.error = "Applying the refactoring failed: " + err.Error()
				m.invalidate()
				return
			}
			m.hierarchyRefactor = nil
			m.status = "Hierarchy refactoring applied"
			m.invalidate()
		})
	}()
}

func gapNullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

type hierarchyChoiceInfo struct {
	ID           string `json:"id"`
	MakeAbstract bool   `json:"makeAbstract"`
}

type hierarchyRefactoredInfo struct {
	Problems []struct {
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"problems"`
	Edit json.RawMessage `json:"edit"`
}

func (m *Model) customRegionsDialog(c *ui.Context, p *ui.Theme) {
	state := m.customRegions
	if state == nil {
		return
	}
	open := state.open
	state.list.Selected = &state.selected
	dialog := nativeModal(c, &open, func() {
		ui.Column(c).Width(460).Gap(8).Children(func() {
			ui.Text(c, "Custom Folding Regions").FontSize(13).Bold().TextColor(p.Text)
			list := ui.List(c, &state.list, len(state.regions), func(index int) {
				region := state.regions[index]
				label := region.name
				if label == "" {
					label = "(no description)"
				}
				row := ui.Row(c).Height(26).Padding(0, 10).Gap(8).AlignItems(ui.Center)
				row.Children(func() {
					ui.Text(c, label).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
					ui.Spacer(c)
					ui.Text(c, fmt.Sprintf(":%d", region.start+1)).FontSize(12).
						TextColor(p.TextMuted).Font("monospace")
				})
				if row.Clicked() {
					m.openCustomRegion(region)
				}
			}).Height(320).AutoFocus()
			if list.Submitted() {
				if state.selected >= 0 && state.selected < len(state.regions) {
					m.openCustomRegion(state.regions[state.selected])
				}
			}
		})
	})
	if dialog.Dismissed() || !open {
		state.open = false
		m.customRegions = nil
		m.invalidate()
	}
}

func (m *Model) openCustomRegion(region customFoldingRegion) {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	m.customRegions = nil
	t.state.Select(t.state.Buffer.Snapshot().LineStart(region.start),
		t.state.Buffer.Snapshot().LineStart(region.start))
	t.state.ScrollTo(region.start)
	m.recordNavigation(true)
	m.invalidate()
}

func (m *Model) customRegionKeydown(ev ui.InputEvent) bool {
	state := m.customRegions
	if state == nil || !state.open || ev.Kind != ui.InputKeyDown {
		return false
	}
	switch ev.Key {
	case ui.KeyDown:
		if len(state.regions) > 0 {
			state.selected = min(len(state.regions)-1, state.selected+1)
		}
		return true
	case ui.KeyUp:
		state.selected = max(0, state.selected-1)
		return true
	case ui.KeyEnter:
		if state.selected >= 0 && state.selected < len(state.regions) {
			m.openCustomRegion(state.regions[state.selected])
		}
		return true
	}
	return false
}

func (m *Model) customRegionShortcut(c *ui.Context) bool {
	switch {
	case c.Shortcut(0, ui.KeyDown):
		return m.customRegionKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyDown})
	case c.Shortcut(0, ui.KeyUp):
		return m.customRegionKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp})
	case c.Shortcut(0, ui.KeyEnter):
		return m.customRegionKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter})
	}
	return false
}

func (m *Model) testNavigationDialog(c *ui.Context, p *ui.Theme) {
	state := m.testNav
	if state == nil {
		return
	}
	open := state.open
	state.list.Selected = &state.selected
	dialog := nativeModal(c, &open, func() {
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, "Choose Test or Test Subject").FontSize(13).Bold().TextColor(p.Text)
			list := ui.List(c, &state.list, len(state.items), func(index int) {
				item := state.items[index]
				row := ui.Row(c).Height(26).Padding(0, 10).Gap(8).AlignItems(ui.Center)
				row.Children(func() {
					ui.Text(c, item.label).FontSize(12).TextColor(p.Text).SingleLine()
					ui.Spacer(c)
					ui.Text(c, projectRelative(m.Root(), filepath.Dir(item.path))).FontSize(12).
						TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					m.openTestNavigationTarget(item)
				}
			}).Height(320).AutoFocus()
			if list.Submitted() && state.selected >= 0 && state.selected < len(state.items) {
				m.openTestNavigationTarget(state.items[state.selected])
			}
		})
	})
	if dialog.Dismissed() || !open {
		m.testNav = nil
		m.invalidate()
	}
}

func (m *Model) openTestNavigationTarget(item testNavTarget) {
	m.testNav = nil
	m.revealAt(item.path, 0, 0)
}

func (m *Model) testNavigationKeydown(ev ui.InputEvent) bool {
	state := m.testNav
	if state == nil || !state.open || ev.Kind != ui.InputKeyDown {
		return false
	}
	switch ev.Key {
	case ui.KeyDown:
		if len(state.items) > 0 {
			state.selected = min(len(state.items)-1, state.selected+1)
		}
		return true
	case ui.KeyUp:
		state.selected = max(0, state.selected-1)
		return true
	case ui.KeyEnter:
		if state.selected >= 0 && state.selected < len(state.items) {
			m.openTestNavigationTarget(state.items[state.selected])
		}
		return true
	}
	return false
}

func (m *Model) testNavigationShortcut(c *ui.Context) bool {
	switch {
	case c.Shortcut(0, ui.KeyDown):
		return m.testNavigationKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyDown})
	case c.Shortcut(0, ui.KeyUp):
		return m.testNavigationKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp})
	case c.Shortcut(0, ui.KeyEnter):
		return m.testNavigationKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter})
	}
	return false
}

func (m *Model) unwrapDialog(c *ui.Context, p *ui.Theme) {
	state := m.unwrap
	if state == nil {
		return
	}
	open := state.open
	state.list.Selected = &state.selected
	dialog := nativeModal(c, &open, func() {
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, "Choose the Statement to Unwrap/Remove").FontSize(13).
				Bold().TextColor(p.Text)
			list := ui.List(c, &state.list, len(state.options), func(index int) {
				option := state.options[index]
				row := ui.Row(c).Height(28).Padding(0, 10).AlignItems(ui.Center)
				row.Children(func() {
					ui.Text(c, option.label).FontSize(12).TextColor(p.Text).
						SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					m.applyEditorUnwrap(option)
				}
			}).Height(300).AutoFocus()
			if list.Submitted() && state.selected >= 0 && state.selected < len(state.options) {
				m.applyEditorUnwrap(state.options[state.selected])
			}
		})
	})
	if dialog.Dismissed() || !open {
		m.unwrap = nil
		m.invalidate()
	}
}

func (m *Model) unwrapKeydown(ev ui.InputEvent) bool {
	state := m.unwrap
	if state == nil || !state.open || ev.Kind != ui.InputKeyDown {
		return false
	}
	switch ev.Key {
	case ui.KeyDown:
		if len(state.options) > 0 {
			state.selected = min(len(state.options)-1, state.selected+1)
		}
		return true
	case ui.KeyUp:
		state.selected = max(0, state.selected-1)
		return true
	case ui.KeyEnter:
		if state.selected >= 0 && state.selected < len(state.options) {
			m.applyEditorUnwrap(state.options[state.selected])
		}
		return true
	}
	return false
}

func (m *Model) unwrapShortcut(c *ui.Context) bool {
	switch {
	case c.Shortcut(0, ui.KeyDown):
		return m.unwrapKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyDown})
	case c.Shortcut(0, ui.KeyUp):
		return m.unwrapKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp})
	case c.Shortcut(0, ui.KeyEnter):
		return m.unwrapKeydown(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter})
	}
	return false
}

func (m *Model) hierarchyRefactorDialog(c *ui.Context, p *ui.Theme) {
	state := m.hierarchyRefactor
	if state == nil {
		return
	}
	open := state.open
	title := "Pull Members Up"
	if state.kind == "pushDown" {
		title = "Push Members Down"
	}
	dialog := nativeModal(c, &open, func() {
		ui.Column(c).Width(620).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text)
			if state.info.ClassName != "" {
				ui.Text(c, state.info.ClassName).FontSize(12).TextColor(p.TextMuted)
			}
			if state.kind == "pullUp" && len(state.info.Targets) > 0 {
				labels := make([]string, len(state.info.Targets))
				for index, target := range state.info.Targets {
					labels[index] = target.QualifiedName
				}
				if state.targetLabel == "" {
					state.targetLabel = labels[0]
				}
				ui.Select(c, &state.targetLabel, labels)
				for _, target := range state.info.Targets {
					if target.QualifiedName == state.targetLabel {
						state.target = target.ID
						break
					}
				}
			}
			scroll := ui.Scroll(c).Height(320).Children(func() {
				ui.Column(c).Children(func() {
					for index, member := range state.info.Members {
						index := index
						selected := index < len(state.selected) && state.selected[index]
						row := ui.Checkbox(c, &selected, member.Label)
						if row.Changed() && index < len(state.selected) {
							state.selected[index] = selected
						}
					}
				})
			})
			_ = scroll
			if state.error != "" {
				ui.Text(c, state.error).FontSize(12).TextColor(p.Warning).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() {
					state.open = false
					m.hierarchyRefactor = nil
				})
				button := ui.Button(c, "Refactor")
				if state.busy {
					button.Disabled(true)
				}
				button.OnClick(m.confirmHierarchyRefactor)
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.hierarchyRefactor = nil
		m.invalidate()
	}
}

func gapModelContext(m *Model) context.Context {
	if m != nil && m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}
