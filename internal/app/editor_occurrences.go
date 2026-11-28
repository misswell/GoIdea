package app

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type editorOccurrenceSearch struct {
	buffer     *editor.Buffer
	version    uint64
	selections []editor.Selection
	primary    editor.Selection
	wholeWords bool
	notFound   bool
}

func (t *Tab) rememberEditorOccurrences(wholeWords, notFound bool) {
	t.occurrences = editorOccurrenceSearch{
		buffer: t.state.Buffer, version: t.state.Buffer.Version(),
		selections: t.state.Selections(), primary: t.state.Selection(),
		wholeWords: wholeWords, notFound: notFound,
	}
}

func (m *Model) addNextEditorOccurrence() {
	t := m.tab()
	state := t.state
	selections, primary := state.Selections(), state.Selection()
	text := state.Buffer.String()
	from, to := primary.Start(), primary.End()
	if from == to {
		from, to = editorIdentifierRange(text, from)
		if from == to {
			return
		}
		selections = slices.DeleteFunc(selections, func(selection editor.Selection) bool { return selection == primary })
		state.SetSelections(append(selections, editor.Selection{Anchor: from, Head: to})...)
		t.rememberEditorOccurrences(true, false)
		m.invalidate()
		return
	}

	run := t.occurrences
	if run.buffer != state.Buffer || run.version != state.Buffer.Version() ||
		run.primary != primary || !slices.Equal(run.selections, selections) {
		run = editorOccurrenceSearch{}
	}
	start := to
	if run.notFound {
		start = 0
	}
	hit := findEditorOccurrence(text, text[from:to], start, run.wholeWords)
	if hit < 0 {
		t.rememberEditorOccurrences(run.wholeWords, true)
		m.RunActionWithArg("editor.showHint", "Not found, press again to search from the top")
		return
	}
	end := hit + to - from
	for _, selection := range selections {
		if selection.Start() == hit && selection.End() == end {
			return
		}
	}
	state.SetSelections(append(selections, editorOccurrenceRange(primary, hit, end))...)
	t.rememberEditorOccurrences(run.wholeWords, false)
	m.invalidate()
}

func (m *Model) selectAllEditorOccurrences() {
	t := m.tab()
	state := t.state
	text, primary := state.Buffer.String(), state.Selection()
	from, to := primary.Start(), primary.End()
	wholeWords := from == to
	if wholeWords {
		from, to = editorIdentifierRange(text, from)
		if from == to {
			return
		}
	}
	query := text[from:to]
	var selections []editor.Selection
	primaryIndex := 0
	for hit := findEditorOccurrence(text, query, 0, wholeWords); hit >= 0; hit = findEditorOccurrence(text, query, hit+len(query), wholeWords) {
		if hit == from {
			primaryIndex = len(selections)
		}
		selections = append(selections, editorOccurrenceRange(primary, hit, hit+len(query)))
	}
	if len(selections) == 0 {
		return
	}
	main := selections[primaryIndex]
	selections = slices.Delete(selections, primaryIndex, primaryIndex+1)
	state.SetSelections(append(selections, main)...)
	t.occurrences = editorOccurrenceSearch{}
	m.invalidate()
}

func editorOccurrenceRange(like editor.Selection, from, to int) editor.Selection {
	if like.Head < like.Anchor {
		return editor.Selection{Anchor: to, Head: from}
	}
	return editor.Selection{Anchor: from, Head: to}
}

func editorIdentifierRange(text string, caret int) (int, int) {
	from, to := caret, caret
	for from > 0 {
		r, width := utf8.DecodeLastRuneInString(text[:from])
		if !editorOccurrenceIdentifier(r) {
			break
		}
		from -= width
	}
	for to < len(text) {
		r, width := utf8.DecodeRuneInString(text[to:])
		if !editorOccurrenceIdentifier(r) {
			break
		}
		to += width
	}
	return from, to
}

func editorOccurrenceIdentifier(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '$'
}

func findEditorOccurrence(text, query string, start int, wholeWords bool) int {
	for {
		index := strings.Index(text[start:], query)
		if index < 0 {
			return -1
		}
		from := start + index
		if !wholeWords {
			return from
		}
		to := from + len(query)
		before, _ := utf8.DecodeLastRuneInString(text[:from])
		after, _ := utf8.DecodeRuneInString(text[to:])
		if !editorOccurrenceIdentifier(before) && !editorOccurrenceIdentifier(after) {
			return from
		}
		start = to
	}
}
