package app

import (
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

func (m *Model) completionAutoPopup() bool {
	return m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.autoPopup")
}

func (m *Model) completionAutoInsertSingle() bool {
	return m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.autoInsertSingle")
}

func (m *Model) completionInsertOnSpecialKeys() bool {
	return m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.insertOnSpecialKeys")
}

func (m *Model) completionShowDocumentation() bool {
	return m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.showDocumentation")
}

func (m *Model) completionDocumentationDelay() time.Duration {
	delay := m.nativeRuntimeNumber(settingsPageCodeCompletion, "editor.completion.documentationDelay")
	return time.Duration(max(0, delay)) * time.Millisecond
}

func (m *Model) completionParameterInfo() bool {
	return m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.parameterInfo")
}

func (m *Model) completionParameterInfoDelay() time.Duration {
	delay := m.nativeRuntimeNumber(settingsPageCodeCompletion, "editor.completion.parameterInfoDelay")
	return time.Duration(max(0, delay)) * time.Millisecond
}

func (m *Model) completionInsertParentheses() bool {
	return m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.insertParentheses")
}

func (m *Model) applyCompletionSettings(prefix string, items []editor.Completion) []editor.Completion {
	if m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.matchCase") {
		mode := m.nativeRuntimeString(settingsPageCodeCompletion, "editor.completion.caseSensitivity")
		filtered := items[:0]
		for _, item := range items {
			label := item.FilterText
			if label == "" {
				label = item.Label
			}
			if completionMatchesCase(label, prefix, mode) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if m.nativeRuntimeBool(settingsPageCodeCompletion, "editor.completion.sortAlphabetically") {
		sort.SliceStable(items, func(i, j int) bool {
			left, right := strings.ToLower(items[i].Label), strings.ToLower(items[j].Label)
			if left == right {
				return items[i].Label < items[j].Label
			}
			return left < right
		})
	}
	return items
}

// completionMatchesCase mirrors OpenIDEA's Code Completion setting. "first"
// checks only the first typed letter's case; "all" requires the typed letters
// to appear as a case-sensitive subsequence.
func completionMatchesCase(label, typed, mode string) bool {
	typedRunes := []rune(typed)
	labelRunes := []rune(label)
	if len(typedRunes) == 0 {
		return true
	}
	if mode != "all" {
		if len(labelRunes) == 0 {
			return false
		}
		return strings.EqualFold(string(labelRunes[0]), string(typedRunes[0])) &&
			labelRunes[0] == typedRunes[0]
	}
	at := 0
	for _, ch := range labelRunes {
		if at < len(typedRunes) && ch == typedRunes[at] {
			at++
		}
	}
	return at == len(typedRunes)
}
