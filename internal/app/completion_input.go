package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

var completionCommitChars = map[string]bool{
	".": true,
	",": true,
	";": true,
	"=": true,
	" ": true,
	":": true,
	"(": true,
}

// completionInput applies the Code Completion keys before MyGo receives the
// event. A focused lookup always commits its selection. An automatic lookup
// does so only when "Insert selected suggestion by pressing space, dot…" is on.
func (m *Model) completionInput(t *Tab, ev ui.InputEvent) bool {
	if t == nil || t.state == nil || t.completion.consumeText != "" {
		if ev.Kind == ui.InputText && t != nil && t.completion.consumeText == ev.Text {
			t.completion.consumeText = ""
			return true
		}
		if t != nil && ev.Kind != ui.InputText {
			t.completion.consumeText = ""
		}
	}

	switch ev.Kind {
	case ui.InputKeyDown:
		if ev.Key == ui.KeyEnter && t.completion.open {
			return m.acceptCurrentCompletion(t, "")
		}
		text, ok := completionCommitTextForKey(ev)
		if !ok || !m.completionShouldCommit(t) {
			return false
		}
		if !m.acceptCurrentCompletion(t, text) {
			return false
		}
		// macOS and Windows report the typed character as InputText after the
		// key-down. Consume that event when acceptance already inserted "(".
		return text == "(" && t.completion.consumeText == "("
	case ui.InputText:
		if !completionCommitChars[ev.Text] || !t.completion.open {
			return false
		}
		if m.completionShouldCommit(t) {
			if !m.acceptCurrentCompletion(t, ev.Text) {
				return false
			}
			// A platform that reports punctuation only as InputText must not
			// lose the character after accepting the selected item.
			if t.completion.consumeText == ev.Text {
				t.completion.consumeText = ""
				return true
			}
			t.state.Insert(ev.Text)
			return true
		}
		m.cancelCompletionTimers(t)
		t.completion.open = false
	}
	return false
}

func (m *Model) completionShouldCommit(t *Tab) bool {
	if t == nil || !t.completion.open {
		return false
	}
	if t.completion.focused {
		return true
	}
	return m.completionInsertOnSpecialKeys()
}

func completionCommitTextForKey(ev ui.InputEvent) (string, bool) {
	key := ev.Key
	if key == ui.KeyUnknown {
		key = ev.PhysicalKey
	}
	switch key {
	case ui.KeyPeriod:
		if ev.Mods == 0 {
			return ".", true
		}
	case ui.KeyComma:
		if ev.Mods == 0 {
			return ",", true
		}
	case ui.KeySemicolon:
		switch ev.Mods {
		case 0:
			return ";", true
		case ui.Shift:
			return ":", true
		}
	case ui.KeyEqual:
		if ev.Mods == 0 {
			return "=", true
		}
	case ui.KeySpace:
		if ev.Mods == 0 {
			return " ", true
		}
	case ui.Key9:
		if ev.Mods == ui.Shift {
			return "(", true
		}
	}
	// Some platforms report punctuation through the physical key while the
	// layout key is unknown. Use the same joined modifier spelling.
	if ev.Key == ui.KeyUnknown && strings.TrimSpace(ev.Text) != "" {
		return ev.Text, completionCommitChars[ev.Text]
	}
	return "", false
}
