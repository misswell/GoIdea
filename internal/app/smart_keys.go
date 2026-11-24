package app

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type smartPairScope struct {
	close int
}

type smartKeyState struct {
	pairs []smartPairScope
}

var smartPairClosers = map[string]string{
	"(": ")",
	"[": "]",
	"{": "}",
	"'": "'",
	`"`: `"`,
}

// smartKeyInput implements the OpenIDEA Smart Keys that don't have a matching
// public MyGo state option. The native editor's all-or-nothing automatic
// closing is disabled so each bracket and quote setting can be honored.
func (m *Model) smartKeyInput(t *Tab, ev ui.InputEvent) bool {
	if t == nil || t.state == nil || t.doc == nil {
		return false
	}
	switch ev.Kind {
	case ui.InputKeyDown:
		if m.smartHomeInput(t, ev) {
			return true
		}
		if m.camelHumpInput(t, ev) {
			return true
		}
		if ev.Key == ui.KeyTab && ev.Mods == 0 && m.editorSmartBool("tabOutOfPair") &&
			m.smartTabOutOfPair(t) {
			return true
		}
		if ev.Key == ui.KeyEnter && ev.Mods == 0 && !t.completion.open {
			if m.editorSmartBool("insertDocStub") && m.insertDocCommentStub(t) {
				return true
			}
			if !m.editorSmartBool("smartIndent") && m.insertPlainNewline(t) {
				return true
			}
		}
		if ev.Key == ui.KeyBackspace && ev.Mods == 0 && !t.completion.open {
			switch m.editorSmartString("backspaceUnindent") {
			case "none":
				return m.deleteOneCharacter(t)
			case "nearest":
				return m.deleteToNearestIndent(t)
			}
		}
	case ui.InputText:
		if m.smartPairInput(t, ev.Text) {
			return true
		}
		m.shiftSmartPairScopes(t, ev.Text)
		if t.state.ReadOnly || ev.Text == "" || len(t.state.Selections()) != 1 {
			return false
		}
		if m.editorSmartBool("smartIndent") {
			return false
		}
		if m.insertPlainNewlineText(t, ev.Text) {
			return true
		}
	}
	return false
}

func (m *Model) smartHomeInput(t *Tab, ev ui.InputEvent) bool {
	if m.editorSmartBool("smartHome") {
		return false
	}
	key := ev.Key
	if key == ui.KeyUnknown {
		key = ev.PhysicalKey
	}
	commandMod := ev.Mods&ui.Cmd != 0
	shift := ev.Mods&ui.Shift != 0
	other := ev.Mods &^ (ui.Cmd | ui.Shift)
	if key != ui.KeyHome && !(key == ui.KeyLeft && commandMod && other == 0) {
		return false
	}
	if key == ui.KeyHome && ev.Mods != 0 && ev.Mods != ui.Shift {
		return false
	}
	m.moveSelectionsToPhysicalLineStart(t, shift)
	return true
}

func (m *Model) moveSelectionsToPhysicalLineStart(t *Tab, extend bool) {
	text := t.doc.Text()
	sels := t.state.Selections()
	for i := range sels {
		head := max(0, min(sels[i].Head, len(text)))
		start := strings.LastIndexByte(text[:head], '\n') + 1
		if extend {
			sels[i].Head = start
		} else {
			sels[i].Anchor, sels[i].Head = start, start
		}
	}
	t.state.SetSelections(sels...)
	m.invalidate()
}

func (m *Model) camelHumpInput(t *Tab, ev ui.InputEvent) bool {
	if m.editorSmartBool("camelHumps") {
		switch {
		case ev.Mods == ui.Alt && ev.Key == ui.KeyLeft:
			return t.state.Run(editor.MoveSubwordLeft)
		case ev.Mods == ui.Alt && ev.Key == ui.KeyRight:
			return t.state.Run(editor.MoveSubwordRight)
		case ev.Mods == ui.Alt|ui.Shift && ev.Key == ui.KeyLeft:
			return t.state.Run(editor.SelectSubwordLeft)
		case ev.Mods == ui.Alt|ui.Shift && ev.Key == ui.KeyRight:
			return t.state.Run(editor.SelectSubwordRight)
		case ev.Mods == ui.Alt && ev.Key == ui.KeyBackspace:
			return t.state.Run(editor.DeleteSubwordLeft)
		case ev.Mods == ui.Alt && ev.Key == ui.KeyDelete:
			return t.state.Run(editor.DeleteSubwordRight)
		}
		return false
	}
	switch {
	case ev.Mods == ui.Alt && ev.Key == ui.KeyLeft:
		return t.state.Run(editor.MoveWordLeft)
	case ev.Mods == ui.Alt && ev.Key == ui.KeyRight:
		return t.state.Run(editor.MoveWordRight)
	case ev.Mods == ui.Alt|ui.Shift && ev.Key == ui.KeyLeft:
		return t.state.Run(editor.SelectWordLeft)
	case ev.Mods == ui.Alt|ui.Shift && ev.Key == ui.KeyRight:
		return t.state.Run(editor.SelectWordRight)
	}
	return false
}

func (m *Model) smartTabOutOfPair(t *Tab) bool {
	if len(t.smart.pairs) == 0 {
		return false
	}
	sels := t.state.Selections()
	if len(sels) != 1 || !sels[0].Empty() {
		return false
	}
	cursor := sels[0].Head
	for i, pair := range t.smart.pairs {
		if pair.close != cursor {
			continue
		}
		t.state.Select(cursor+1, cursor+1)
		t.smart.pairs = append(t.smart.pairs[:i], t.smart.pairs[i+1:]...)
		m.invalidate()
		return true
	}
	return false
}

func (m *Model) smartPairInput(t *Tab, text string) bool {
	if t.state.ReadOnly || utf8.RuneCountInString(text) != 1 {
		return false
	}
	open := text
	close, isPair := smartPairClosers[open]
	if isPair && !m.smartPairOpenEnabled(open) {
		isPair = false
	}
	if m.smartPairStepOver(t, text) {
		return true
	}
	if !isPair {
		return false
	}
	sels := t.state.Selections()
	if len(sels) == 0 {
		return false
	}
	surround := m.editorSmartBool("surroundSelection")
	for _, selection := range sels {
		if !selection.Empty() && !surround {
			return false
		}
	}
	m.applySmartPair(t, sels, open, close)
	return true
}

func (m *Model) smartPairOpenEnabled(open string) bool {
	switch open {
	case "(", "[", "{":
		return m.editorSmartBool("pairBrackets")
	case "'", `"`:
		return m.editorSmartBool("pairQuotes")
	default:
		return false
	}
}

func (m *Model) smartPairStepOver(t *Tab, text string) bool {
	if text != ")" && text != "]" && text != "}" && text != "'" && text != `"` {
		return false
	}
	sels := t.state.Selections()
	if len(sels) != 1 || !sels[0].Empty() {
		return false
	}
	cursor := sels[0].Head
	for i, pair := range t.smart.pairs {
		if pair.close != cursor {
			continue
		}
		doc := t.doc.Text()
		if cursor >= len(doc) || doc[cursor:cursor+1] != text {
			continue
		}
		t.state.Select(cursor+1, cursor+1)
		t.smart.pairs = append(t.smart.pairs[:i], t.smart.pairs[i+1:]...)
		m.invalidate()
		return true
	}
	return false
}

type smartPairEdit struct {
	start, end int
	text       string
}

func (m *Model) applySmartPair(t *Tab, sels []editor.Selection, open, close string) {
	doc := t.doc.Text()
	edits := make([]smartPairEdit, 0, len(sels))
	ranges := make([]editor.Selection, len(sels))
	for i, selection := range sels {
		start, end := min(selection.Anchor, selection.Head), max(selection.Anchor, selection.Head)
		if start < 0 || end < start || end > len(doc) {
			return
		}
		replacement := open + close
		if start != end {
			replacement = open + doc[start:end] + close
		}
		edits = append(edits, smartPairEdit{start: start, end: end, text: replacement})
		ranges[i] = editor.Selection{Anchor: start, Head: end}
	}
	ordered := append([]smartPairEdit(nil), edits...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].start < ordered[j].start })
	editorEdits := make([]editor.Edit, len(ordered))
	for i, edit := range ordered {
		editorEdits[i] = editor.Edit{Start: edit.start, End: edit.end, Text: edit.text}
	}
	t.state.Buffer.Edit(editorEdits...)

	t.smart.pairs = t.smart.pairs[:0]
	for i := range ranges {
		start, end := ranges[i].Anchor, ranges[i].Head
		delta := 0
		for _, edit := range ordered {
			if edit.start >= start {
				break
			}
			delta += len(edit.text) - (edit.end - edit.start)
		}
		if start == end {
			caret := start + delta + len(open)
			ranges[i] = editor.Selection{Anchor: caret, Head: caret}
			if len(sels) == 1 {
				t.smart.pairs = append(t.smart.pairs, smartPairScope{close: caret})
			}
			continue
		}
		ranges[i] = editor.Selection{
			Anchor: start + delta + len(open),
			Head:   end + delta + len(open),
		}
	}
	t.state.SetSelections(ranges...)
	m.invalidate()
}

func (m *Model) shiftSmartPairScopes(t *Tab, text string) {
	if len(t.smart.pairs) == 0 {
		return
	}
	if strings.ContainsRune(text, '\n') {
		t.smart.pairs = nil
		return
	}
	sels := t.state.Selections()
	if len(sels) != 1 || !sels[0].Empty() {
		t.smart.pairs = nil
		return
	}
	cursor := sels[0].Head
	for i := range t.smart.pairs {
		if t.smart.pairs[i].close >= cursor {
			t.smart.pairs[i].close += len(text)
		}
	}
}

func (m *Model) insertDocCommentStub(t *Tab) bool {
	sels := t.state.Selections()
	if len(sels) != 1 || !sels[0].Empty() || t.state.ReadOnly {
		return false
	}
	caret := sels[0].Head
	text := t.doc.Text()
	lineStart := strings.LastIndexByte(text[:caret], '\n') + 1
	lineEnd := strings.IndexByte(text[caret:], '\n')
	if lineEnd < 0 {
		lineEnd = len(text)
	} else {
		lineEnd += caret
	}
	before := text[lineStart:caret]
	after := text[caret:lineEnd]
	if strings.TrimSpace(before) != "/**" || strings.TrimSpace(after) != "" {
		return false
	}
	next := ""
	if lineEnd < len(text) {
		nextEnd := strings.IndexByte(text[lineEnd+1:], '\n')
		if nextEnd < 0 {
			next = text[lineEnd+1:]
		} else {
			next = text[lineEnd+1 : lineEnd+1+nextEnd]
		}
	}
	if strings.HasPrefix(strings.TrimSpace(next), "*") {
		return false
	}
	indent := before[:len(before)-len(strings.TrimLeft(before, " \t"))]
	summary := "\n" + indent + " * "
	insert := summary + "\n" + indent + " */"
	t.state.Buffer.Edit(editor.Edit{Start: caret, End: caret, Text: insert})
	newCaret := caret + len(summary)
	t.state.Select(newCaret, newCaret)
	m.invalidate()
	return true
}

func (m *Model) insertPlainNewline(t *Tab) bool {
	return m.insertNewlinePreservingIndent(t)
}

func (m *Model) insertPlainNewlineText(t *Tab, text string) bool {
	if text != "\n" {
		return false
	}
	return m.insertNewlinePreservingIndent(t)
}

func (m *Model) insertNewlinePreservingIndent(t *Tab) bool {
	sels := t.state.Selections()
	if len(sels) != 1 || !sels[0].Empty() || t.state.ReadOnly {
		return false
	}
	caret := sels[0].Head
	text := t.doc.Text()
	lineStart := strings.LastIndexByte(text[:caret], '\n') + 1
	line := text[lineStart:caret]
	indentEnd := len(line) - len(strings.TrimLeft(line, " \t"))
	insert := "\n" + line[:indentEnd]
	t.state.Buffer.Edit(editor.Edit{Start: caret, End: caret, Text: insert})
	t.state.Select(caret+len(insert), caret+len(insert))
	m.invalidate()
	return true
}

func (m *Model) deleteOneCharacter(t *Tab) bool {
	sels := t.state.Selections()
	if len(sels) == 0 || t.state.ReadOnly {
		return false
	}
	edits := make([]editor.Edit, 0, len(sels))
	for _, selection := range sels {
		if !selection.Empty() {
			return false
		}
		caret := selection.Head
		if caret <= 0 {
			return false
		}
		_, size := utf8.DecodeLastRuneInString(t.doc.Text()[:caret])
		edits = append(edits, editor.Edit{Start: caret - size, End: caret})
	}
	t.state.Buffer.Edit(edits...)
	selection := t.state.Selection()
	caret := selection.Head
	if len(edits) == 1 {
		caret = edits[0].Start
	}
	t.state.Select(caret, caret)
	t.smart.pairs = nil
	m.invalidate()
	return true
}

func (m *Model) deleteToNearestIndent(t *Tab) bool {
	sels := t.state.Selections()
	if len(sels) != 1 || !sels[0].Empty() || t.state.ReadOnly {
		return false
	}
	caret := sels[0].Head
	text := t.doc.Text()
	lineStart := strings.LastIndexByte(text[:caret], '\n') + 1
	indentEnd := lineStart + len(text[lineStart:]) - len(strings.TrimLeft(text[lineStart:], " \t"))
	if caret == lineStart || caret > indentEnd {
		return false
	}
	tabSize := t.state.TabSize
	if tabSize <= 0 {
		tabSize = 4
	}
	col := smartVisualColumn(text[lineStart:caret], tabSize)
	targetCol := max(0, (col-1)/tabSize*tabSize)
	target := lineStart + smartByteAtColumn(text[lineStart:caret], targetCol, tabSize)
	if target >= caret {
		return false
	}
	t.state.Buffer.Edit(editor.Edit{Start: target, End: caret})
	t.state.Select(target, target)
	t.smart.pairs = nil
	m.invalidate()
	return true
}

func smartVisualColumn(text string, tabSize int) int {
	col := 0
	for _, ch := range text {
		if ch == '\t' {
			col += tabSize - col%tabSize
		} else {
			col++
		}
	}
	return col
}

func smartByteAtColumn(text string, target, tabSize int) int {
	col := 0
	for offset, ch := range text {
		if col >= target {
			return offset
		}
		if ch == '\t' {
			col += tabSize - col%tabSize
		} else {
			col++
		}
	}
	return len(text)
}
