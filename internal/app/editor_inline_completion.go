package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// inlineCompletionChoice is one suggestion converted to this buffer's byte
// offsets. Keeping the conversion beside the request lets later frames apply
// the exact range the server answered about.
type inlineCompletionChoice struct {
	text    string
	start   int
	end     int
	command json.RawMessage
}

type inlineCompletionState struct {
	choices []inlineCompletionChoice
	index   int
	caret   int
	version uint64
	request uint32
	gen     uint64
	timer   *time.Timer
}

func (t *Tab) inlineCompletionVisible() bool {
	return t != nil && t.state != nil && len(t.inline.choices) > 0
}

func (m *Model) clearInlineCompletionState(t *Tab) {
	if t == nil || t.state == nil {
		return
	}
	if t.inline.timer != nil {
		t.inline.timer.Stop()
		t.inline.timer = nil
	}
	t.state.SetSuggestion(t.state.Cursor(), "")
	t.inline.choices = nil
	t.inline.index = 0
	t.inline.request = 0
	t.inline.gen++
}

// inlineCompletionInput owns OpenIDEA's inline-completion keys. The action
// registry exposes the same operations, while this callback keeps editor focus
// and Tab/Escape arbitration deterministic before MyGo's native bindings run.
func (m *Model) inlineCompletionInput(t *Tab, ev ui.InputEvent) bool {
	if t == nil || t.state == nil {
		return false
	}
	switch ev.Kind {
	case ui.InputText, ui.InputCompose, ui.InputCommand:
		m.clearInlineCompletionState(t)
		if !t.state.ReadOnly {
			m.scheduleInlineCompletion(t)
		}
		return false
	case ui.InputKeyDown:
		if ev.Key == ui.KeyEscape && ev.Mods == 0 && t.inlineCompletionVisible() {
			m.DismissInlineCompletion()
			return true
		}
		if ev.Key == ui.KeyTab && ev.Mods == 0 && t.inlineCompletionVisible() {
			m.AcceptInlineCompletion()
			return true
		}
		if ev.Key == ui.KeyRight && (ev.Mods == ui.Cmd || ev.Mods == ui.Ctrl) &&
			t.inlineCompletionVisible() {
			m.AcceptInlineCompletionWord()
			return true
		}
		if ev.Mods == ui.Alt && ev.Key == ui.KeyBackslash {
			m.TriggerInlineCompletion()
			return true
		}
		if ev.Mods == ui.Alt && ev.Key == ui.KeyBracketRight &&
			t.inlineCompletionVisible() {
			m.CycleInlineCompletion(1)
			return true
		}
		if ev.Mods == ui.Alt && ev.Key == ui.KeyBracketLeft &&
			t.inlineCompletionVisible() {
			m.CycleInlineCompletion(-1)
			return true
		}
		if caretMovementKey(ev.Key) || ev.Key == ui.KeyPageUp || ev.Key == ui.KeyPageDown ||
			ev.Key == ui.KeyHome || ev.Key == ui.KeyEnd {
			m.clearInlineCompletionState(t)
			m.invalidate()
			return false
		}
		if ev.Mods&(ui.Cmd|ui.Ctrl) == 0 &&
			(ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete || ev.Key == ui.KeyEnter) {
			m.clearInlineCompletionState(t)
			if !t.state.ReadOnly {
				m.scheduleInlineCompletion(t)
			}
		}
	}
	return false
}

// scheduleInlineCompletion follows OpenIDEA's editor extension: typing asks
// only when the setting permits it, after 75 ms or the configured pause.
func (m *Model) scheduleInlineCompletion(t *Tab) {
	if t == nil || t.state == nil || t.doc == nil || t.state.ReadOnly ||
		!m.inlineCompletionAllowed(t) {
		return
	}
	if t.inline.timer != nil {
		t.inline.timer.Stop()
	}
	delay := 75 * time.Millisecond
	if m.inlineCompletionSettingBool("editor.inlineCompletion.onlyOnPause") {
		ms := m.inlineCompletionSettingNumber("editor.inlineCompletion.pauseMs")
		if ms < 0 {
			ms = 0
		}
		if ms > 3000 {
			ms = 3000
		}
		delay = time.Duration(ms) * time.Millisecond
	}
	gen := t.inline.gen + 1
	t.inline.gen = gen
	t.inline.timer = time.AfterFunc(delay, func() {
		m.post(func() {
			if !m.tabOpen(t) || t.inline.gen != gen || t.state == nil ||
				t.state.ReadOnly || !t.state.Selection().Empty() {
				return
			}
			m.requestInlineCompletion(t, true)
		})
	})
}

func (m *Model) inlineCompletionSettingBool(key string) bool {
	return m.nativeRuntimeBool(settingsPageInlineCompletion, key)
}

func (m *Model) inlineCompletionSettingNumber(key string) int {
	return int(m.nativeRuntimeNumber(settingsPageInlineCompletion, key))
}

func (m *Model) inlineCompletionAllowed(t *Tab) bool {
	if m.PowerSaveMode() {
		return false
	}
	if !m.inlineCompletionSettingBool("editor.inlineCompletion.enabled") {
		return false
	}
	language := ""
	if t.state != nil && t.state.Buffer != nil && t.state.Buffer.Language() != nil {
		language = t.state.Buffer.Language().Name
	}
	return !nativeSettingsLanguageDisabled(
		m,
		settingsPageInlineCompletion,
		"editor.inlineCompletion.disabledLanguages",
		language,
	)
}

// TriggerInlineCompletion runs OpenIDEA's explicit "Call Inline Completion"
// action and reports the same hints the desktop client does.
func (m *Model) TriggerInlineCompletion() {
	t := m.tab()
	if t == nil || t.state == nil || t.doc == nil {
		return
	}
	if t.state.ReadOnly {
		m.status = "Inline completion is unavailable in a read-only editor"
		m.invalidate()
		return
	}
	if !m.inlineCompletionAllowed(t) {
		m.status = "Inline completion is off for this language"
		m.invalidate()
		return
	}
	m.requestInlineCompletion(t, false)
}

func (m *Model) requestInlineCompletion(t *Tab, automatic bool) {
	if t == nil || t.state == nil || t.doc == nil || !m.tabOpen(t) {
		return
	}
	selection := t.state.Selection()
	if !selection.Empty() || len(t.state.Selections()) != 1 {
		if !automatic {
			m.status = "Place one caret in empty text to call inline completion"
			m.invalidate()
		}
		return
	}
	if t.inline.request != 0 {
		m.cancelRequest(t.inline.request)
	}
	caret := t.state.Cursor()
	docID := t.doc.ID()
	version := t.state.Buffer.Version()
	pos := t.doc.UTF16Pos(caret)
	id := m.nextRequest()
	t.inline.request = id
	t.inline.gen++
	gen := t.inline.gen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_inline_completion", map[string]any{
			"request": id, "doc": docID, "pos": lspPos(pos), "automatic": automatic,
		})
		var items []openidea.InlineCompletionItem
		if err == nil {
			err = json.Unmarshal(raw, &items)
		}
		m.post(func() {
			if t.inline.request == id {
				t.inline.request = 0
			}
			if t.inline.gen != gen || !m.tabOpen(t) || t.doc == nil || t.doc.ID() != docID {
				return
			}
			if err != nil {
				if !automatic {
					kind := openidea.ErrorKind(err)
					if kind == "unavailable" || kind == "notFound" {
						m.status = "Inline completion is unavailable for this language"
					} else {
						m.status = "Inline completion failed: " + err.Error()
					}
					m.invalidate()
				}
				return
			}
			if t.state.Buffer.Version() != version || t.state.Cursor() != caret ||
				!t.state.Selection().Empty() {
				return
			}
			t.inline.choices = inlineChoicesFor(t, caret, items)
			t.inline.index = 0
			t.inline.caret = caret
			t.inline.version = version
			if len(t.inline.choices) == 0 {
				t.state.SetSuggestion(caret, "")
				if !automatic {
					m.status = "No inline suggestions"
					m.invalidate()
				}
				return
			}
			m.showInlineChoice(t)
			m.status = ""
			m.invalidate()
		})
	}()
}

func inlineChoicesFor(t *Tab, caret int, items []openidea.InlineCompletionItem) []inlineCompletionChoice {
	if t == nil || t.doc == nil || t.state == nil {
		return nil
	}
	n := t.state.Buffer.Len()
	out := make([]inlineCompletionChoice, 0, len(items))
	for _, item := range items {
		if item.InsertText == "" {
			continue
		}
		start, end := caret, caret
		if item.Range != nil {
			start, end = t.doc.ByteRange(*item.Range)
		}
		start = max(0, min(start, n))
		end = max(start, min(end, n))
		if caret < start || end < caret {
			continue
		}
		typed := t.state.Buffer.Slice(start, caret)
		if !strings.HasPrefix(item.InsertText, typed) || len(typed) >= len(item.InsertText) {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing.start == start && existing.end == end && existing.text == item.InsertText {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		out = append(out, inlineCompletionChoice{
			text: item.InsertText, start: start, end: end, command: item.Command,
		})
	}
	return out
}

func (m *Model) showInlineChoice(t *Tab) {
	if t == nil || t.state == nil || len(t.inline.choices) == 0 {
		return
	}
	t.inline.index = max(0, min(t.inline.index, len(t.inline.choices)-1))
	choice := t.inline.choices[t.inline.index]
	caret := t.inline.caret
	if caret < choice.start || caret > len(choice.text)+choice.start {
		t.state.SetSuggestion(caret, "")
		return
	}
	typed := t.state.Buffer.Slice(choice.start, caret)
	ghost := strings.TrimPrefix(choice.text, typed)
	if choice.end > caret {
		suffix := t.state.Buffer.Slice(caret, choice.end)
		if suffix != "" && strings.HasSuffix(ghost, suffix) {
			ghost = strings.TrimSuffix(ghost, suffix)
		}
	}
	t.state.SetSuggestion(caret, ghost)
}

func (m *Model) AcceptInlineCompletion() {
	m.acceptInlineCompletion(false)
}

func (m *Model) AcceptInlineCompletionWord() {
	m.acceptInlineCompletion(true)
}

func (m *Model) acceptInlineCompletion(word bool) {
	t := m.tab()
	if t == nil || t.state == nil || len(t.inline.choices) == 0 {
		return
	}
	if t.state.Cursor() != t.inline.caret ||
		t.state.Buffer.Version() != t.inline.version {
		m.DismissInlineCompletion()
		return
	}
	choice := t.inline.choices[t.inline.index]
	caret := t.inline.caret
	if word {
		typed := t.state.Buffer.Slice(choice.start, caret)
		rest := strings.TrimPrefix(choice.text, typed)
		chunk := inlineNextWord(rest)
		if chunk == "" {
			m.DismissInlineCompletion()
			return
		}
		t.state.Insert(chunk)
		added := len(chunk)
		for i := range t.inline.choices {
			if t.inline.choices[i].end >= caret {
				t.inline.choices[i].end += added
			}
		}
		t.inline.caret += added
		t.inline.version = t.state.Buffer.Version()
		if t.inline.caret >= choice.start+len(choice.text) {
			m.DismissInlineCompletion()
			return
		}
		m.showInlineChoice(t)
		m.invalidate()
		return
	}
	t.state.Buffer.Edit(editor.Edit{
		Start: choice.start, End: choice.end, Text: choice.text,
	})
	t.state.SetSelections(editor.Selection{
		Anchor: choice.start + len(choice.text),
		Head:   choice.start + len(choice.text),
	})
	t.state.SetSuggestion(choice.start+len(choice.text), "")
	gen := t.inline.gen + 1
	if t.inline.timer != nil {
		t.inline.timer.Stop()
	}
	t.inline = inlineCompletionState{gen: gen}
	m.executeInlineCommand(t, choice.command)
	m.invalidate()
}

func inlineNextWord(rest string) string {
	runes := []rune(rest)
	if len(runes) == 0 {
		return ""
	}
	i := 0
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}
	if i == len(runes) {
		return string(runes)
	}
	if unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '$' {
		for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) ||
			runes[i] == '_' || runes[i] == '$') {
			i++
		}
		return string(runes[:i])
	}
	for i < len(runes) && !unicode.IsSpace(runes[i]) &&
		!unicode.IsLetter(runes[i]) && !unicode.IsDigit(runes[i]) &&
		runes[i] != '_' && runes[i] != '$' {
		i++
	}
	return string(runes[:i])
}

func (m *Model) executeInlineCommand(t *Tab, command json.RawMessage) {
	if len(command) == 0 || string(command) == "null" || t == nil || t.doc == nil {
		return
	}
	docID := t.doc.ID()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		_, _ = m.invoke(ctx, "lsp_execute_command", map[string]any{
			"doc": docID, "command": json.RawMessage(command),
		})
	}()
}

func (m *Model) DismissInlineCompletion() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	if t.inline.timer != nil {
		t.inline.timer.Stop()
		t.inline.timer = nil
	}
	t.state.SetSuggestion(t.state.Cursor(), "")
	t.inline.choices = nil
	t.inline.index = 0
	t.inline.request = 0
	t.inline.gen++
	m.invalidate()
}

func (m *Model) CycleInlineCompletion(direction int) {
	t := m.tab()
	if t == nil || t.state == nil || len(t.inline.choices) <= 1 || direction == 0 {
		return
	}
	t.inline.index = (t.inline.index + direction + len(t.inline.choices)) % len(t.inline.choices)
	m.showInlineChoice(t)
	m.invalidate()
}
