package app

import (
	"sort"

	"github.com/egoist/mygo/ui/editor"
)

var keymapEditorCommands = map[string]editor.Command{
	"edit.undo":                          editor.Undo,
	"edit.redo":                          editor.Redo,
	"edit.cut":                           editor.Cut,
	"edit.copy":                          editor.Copy,
	"edit.paste":                         editor.Paste,
	"edit.selectAll":                     editor.SelectAll,
	"code.commentLine":                   editor.ToggleComment,
	"code.commentBlock":                  editor.ToggleBlockComment,
	"code.duplicateLine":                 editor.DuplicateSelection,
	"code.deleteLine":                    editor.DeleteLine,
	"code.moveLineUp":                    editor.MoveLinesUp,
	"code.moveLineDown":                  editor.MoveLinesDown,
	"code.moveStatementUp":               editor.MoveLinesUp,
	"code.moveStatementDown":             editor.MoveLinesDown,
	"code.joinLines":                     editor.JoinLines,
	"code.addCaretNextOccurrence":        editor.SelectNext,
	"code.selectAllOccurrences":          editor.SelectAllMatches,
	"code.extendSelection":               editor.SelectLarger,
	"code.shrinkSelection":               editor.SelectSmaller,
	"code.fold":                          editor.Fold,
	"code.unfold":                        editor.Unfold,
	"code.foldAll":                       editor.FoldAll,
	"code.unfoldAll":                     editor.UnfoldAll,
	"code.collapseRecursively":           editor.FoldRecursive,
	"code.expandRecursively":             editor.UnfoldRecursive,
	"code.foldSelection":                 editor.FoldSelection,
	"code.collapseBlock":                 editor.Fold,
	"code.completion":                    editor.ShowCompletions,
	"code.startNewLine":                  editor.NewlineBelow,
	"code.startNewLineBefore":            editor.NewlineAbove,
	"code.splitLine":                     editor.Newline,
	"code.sortLines":                     editor.SortLines,
	"code.reverseLines":                  editor.ReverseLines,
	"find.inFile":                        editor.Find,
	"find.replaceInFile":                 editor.FindReplace,
	"find.next":                          editor.FindNext,
	"find.previous":                      editor.FindPrevious,
	"editor.scrollToCenter":              editor.ScrollCursorCenter,
	"editor.matchBrace":                  editor.MoveToBracket,
	"editor.codeBlockStart":              editor.MoveToBracket,
	"editor.codeBlockEnd":                editor.MoveToBracket,
	"editor.codeBlockStartWithSelection": editor.SelectToBracket,
	"editor.codeBlockEndWithSelection":   editor.SelectToBracket,
	"editor.cloneCaretAbove":             editor.AddCursorAbove,
	"editor.cloneCaretBelow":             editor.AddCursorBelow,
	"editor.toggleSoftWrap":              editor.ToggleSoftWrap,
}

type keymapEditorBindingCandidate struct {
	ActionID string
	Binding  string
	Entry    editor.Binding
}

// editorBindings returns the application bindings for the selected preset.
// Source order is reversed because OpenIDEA's dispatcher gives later bindings
// priority, while MyGo's editor uses the first matching application binding.
func editorBindingCandidatesFor(
	presetID string,
	overrides map[string][]string,
) []keymapEditorBindingCandidate {
	preset := keymapPresetForID(presetID)
	actions := make([]keymapActionDefinition, 0, len(preset.Actions)+len(overrides))
	for _, action := range preset.Actions {
		actions = append(actions, keymapActionDefinition{
			ID:       action.ID,
			Bindings: keymapBindingsIn(presetID, overrides, action.ID),
		})
	}
	ids := make([]string, 0, len(overrides))
	seen := map[string]bool{}
	for _, action := range preset.Actions {
		seen[canonicalActionID(action.ID)] = true
	}
	for id := range overrides {
		if !seen[canonicalActionID(id)] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		actions = append(actions, keymapActionDefinition{
			ID:       id,
			Bindings: keymapBindingsIn(presetID, overrides, id),
		})
	}

	var bindings []keymapEditorBindingCandidate
	for i := len(actions) - 1; i >= 0; i-- {
		command, ok := keymapEditorCommands[canonicalActionID(actions[i].ID)]
		if !ok {
			continue
		}
		for _, binding := range actions[i].Bindings {
			strokes, ok := keymapInputStrokes(binding)
			if !ok {
				continue
			}
			entry := editor.Binding{
				Mods: strokes[0].Mods, Key: strokes[0].Key, Command: command,
			}
			if len(strokes) == 2 {
				entry.ThenMods = strokes[1].Mods
				entry.ThenKey = strokes[1].Key
			}
			bindings = append(bindings, keymapEditorBindingCandidate{
				ActionID: actions[i].ID,
				Binding:  keymapNormalize(binding),
				Entry:    entry,
			})
		}
	}
	return bindings
}

func editorBindingsFor(presetID string, overrides map[string][]string) []editor.Binding {
	candidates := editorBindingCandidatesFor(presetID, overrides)
	bindings := make([]editor.Binding, 0, len(candidates))
	for _, candidate := range candidates {
		bindings = append(bindings, candidate.Entry)
	}
	return bindings
}

func (m *Model) editorBindings() []editor.Binding {
	candidates := editorBindingCandidatesFor(m.keymapPresetID(), m.keymapUserBindings())
	byBinding := map[string][]keymapDispatchCandidate{}
	for _, action := range m.keymapEffectiveActions() {
		for _, binding := range action.Bindings {
			strokes, ok := keymapInputStrokes(binding)
			if !ok {
				continue
			}
			normalized := keymapNormalize(binding)
			byBinding[normalized] = append(byBinding[normalized], keymapDispatchCandidate{
				ActionID: action.ID,
				Binding:  normalized,
				Strokes:  strokes,
			})
		}
	}
	bindings := make([]editor.Binding, 0, len(candidates))
	for _, candidate := range candidates {
		winner, ok := m.keymapWinningCandidate(byBinding[candidate.Binding])
		if !ok {
			continue
		}
		if canonicalActionID(winner.ActionID) != canonicalActionID(candidate.ActionID) {
			continue
		}
		bindings = append(bindings, candidate.Entry)
	}
	return bindings
}

// intellijEditorBindings keeps the focused editor helper tests independent of
// the host's persisted preset. Application code uses Model.editorBindings.
func intellijEditorBindings() []editor.Binding {
	return editorBindingsFor("intellij-macos", nil)
}

// runEditorCommand runs a command of the active editor: the Code menu's line
// actions and the editor's own keys both come here, so they share one
// implementation and one undo history.
func (m *Model) runEditorCommand(cmd editor.Command) bool {
	t := m.tab()
	if t == nil || t.state == nil {
		return false
	}
	if t.state.Run(cmd) {
		m.invalidate()
		return true
	}
	return false
}

// activeEditorAvailable gates an action that needs an open code editor.
func (m *Model) activeEditorAvailable() bool {
	t := m.tab()
	return t != nil && t.state != nil
}

func (m *Model) activeEditorFocused() bool {
	t := m.tab()
	return t != nil && t.state != nil && t.editorFocused
}

// activeEditorEditable gates Cut and Paste. Tests that build a tab without a
// document still exercise the editor state, while production tabs use the
// document's read-only flag.
func (m *Model) activeEditorEditable() bool {
	t := m.tab()
	return t != nil && t.state != nil && !t.state.ReadOnly &&
		(t.doc == nil || !t.doc.ReadOnly())
}

func (m *Model) activeEditorCanUndo() bool {
	t := m.tab()
	return t != nil && t.state != nil && t.state.Buffer != nil && t.state.Buffer.CanUndo()
}

func (m *Model) activeEditorCanRedo() bool {
	t := m.tab()
	return t != nil && t.state != nil && t.state.Buffer != nil && t.state.Buffer.CanRedo()
}

func (m *Model) activeEditorSoftWrapped() bool {
	t := m.tab()
	return t != nil && t.state != nil && t.state.SoftWrap
}

func (m *Model) activeEditorRenderedDocs() bool {
	t := m.tab()
	if t == nil {
		return false
	}
	if t.renderedDocsOverride != nil {
		return *t.renderedDocsOverride
	}
	if m.settingsOpen {
		return m.settingsDraft.renderDocComments
	}
	return m.prefs.Appearance.RenderDocComments
}

// toggleActiveEditorSoftWrap changes the active editor's effective wrapping
// for this tab, leaving the configured file patterns alone.
func (m *Model) toggleActiveEditorSoftWrap() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	t.state.SoftWrap = !t.state.SoftWrap
	m.invalidate()
}

// toggleActiveEditorRenderedDocs overrides the global setting for this tab,
// as View › Active Editor does in OpenIDEA.
func (m *Model) toggleActiveEditorRenderedDocs() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	on := !m.activeEditorRenderedDocs()
	t.renderedDocsOverride = &on
	state := "off"
	if on {
		state = "on"
		if !m.showRenderedDocumentationComment(t) {
			m.status = "Render documentation comments on · no doc comment at caret"
			m.invalidate()
			return
		}
	} else if m.documentation.popup != nil {
		m.closeDocumentationPopup()
	}
	m.status = "Render documentation comments " + state
	m.invalidate()
}
