package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
)

var readEditorClipboardText = mygo.Clipboard.ReadText

func init() {
	register := func(
		id, title, group, shortcut string,
		when func(*Model) bool,
		run func(*Model),
		runWithArg func(*Model, any),
	) {
		if shortcut != "" {
			builtinShortcuts[id] = shortcut
		}
		hiddenActionIDs[id] = true
		_ = registerAction(&Action{
			ID: id, Title: title, Category: group, Builtin: true,
			When: when, KeyWhen: when, Run: run, RunWithArg: runWithArg,
		})
	}
	inlineShown := func(m *Model) bool {
		t := m.tab()
		return t != nil && t.state != nil && t.inlineCompletionVisible()
	}
	pasteHistoryOpen := func(m *Model) bool { return m.editorExtras.open }
	editorAvailable := func(m *Model) bool { return m.activeEditorAvailable() }
	editorEditable := func(m *Model) bool { return m.activeEditorEditable() }

	register("editor.inlineCompletion.accept", "Insert Inline Proposal",
		"Inline Completion", "Tab", inlineShown,
		func(m *Model) { m.AcceptInlineCompletion() }, nil)
	register("editor.inlineCompletion.acceptWord", "Insert Inline Proposal: Next Word",
		"Inline Completion", "Cmd+Right", inlineShown,
		func(m *Model) { m.AcceptInlineCompletionWord() }, nil)
	register("editor.inlineCompletion.dismiss", "Hide Inline Proposal",
		"Inline Completion", "Escape", inlineShown,
		func(m *Model) { m.DismissInlineCompletion() }, nil)
	register("editor.inlineCompletion.next", "Show Next Inline Proposal",
		"Inline Completion", "Alt+]", inlineShown,
		func(m *Model) { m.CycleInlineCompletion(1) }, nil)
	register("editor.inlineCompletion.previous", "Show Previous Inline Proposal",
		"Inline Completion", "Alt+[", inlineShown,
		func(m *Model) { m.CycleInlineCompletion(-1) }, nil)

	register("editor.pasteHistory.paste", "Paste",
		"Paste History", "Enter", pasteHistoryOpen, nil,
		func(m *Model, arg any) { m.PasteFromHistory(arg) })
	register("editor.pasteHistory.remove", "Delete",
		"Paste History", "Delete Backspace", pasteHistoryOpen,
		func(m *Model) { m.deleteSelectedPasteHistory() }, nil)
	register("editor.showHint", "Show Editor Hint",
		"Editor", "", func(m *Model) bool { return true }, nil,
		func(m *Model, arg any) {
			text := ""
			switch value := arg.(type) {
			case string:
				text = value
			case map[string]any:
				text, _ = value["text"].(string)
			}
			text = strings.TrimSpace(text)
			if text == "" {
				return
			}
			m.status = text
			m.invalidate()
		})

	type commandAction struct {
		id       string
		title    string
		group    string
		shortcut string
		when     func(*Model) bool
		command  editor.Command
	}
	commandActions := []commandAction{
		{
			"code.addCaretNextOccurrence",
			"Add Selection for Next Occurrence", "Edit", "Ctrl+G", editorAvailable, editor.SelectNext,
		},
		{
			"code.selectAllOccurrences",
			"Select All Occurrences", "Edit", "Ctrl+Cmd+G", editorAvailable, editor.SelectAllMatches,
		},
		{
			"code.extendSelection",
			"Extend Selection", "Edit", "Alt+Up", editorAvailable, editor.SelectLarger,
		},
		{
			"code.shrinkSelection",
			"Shrink Selection", "Edit", "Alt+Down", editorAvailable, editor.SelectSmaller,
		},
		{
			"code.fold",
			"Collapse", "Folding", "Cmd+-", editorAvailable, editor.Fold,
		},
		{
			"code.unfold",
			"Expand", "Folding", "Cmd+=", editorAvailable, editor.Unfold,
		},
		{
			"code.foldAll",
			"Collapse All", "Folding", "Cmd+Shift+-", editorAvailable, editor.FoldAll,
		},
		{
			"code.unfoldAll",
			"Expand All", "Folding", "Cmd+Shift+=", editorAvailable, editor.UnfoldAll,
		},
		{
			"code.collapseRecursively",
			"Collapse Recursively", "Folding", "Cmd+Alt+-", editorAvailable, editor.FoldRecursive,
		},
		{
			"code.expandRecursively",
			"Expand Recursively", "Folding", "Cmd+Alt+=", editorAvailable, editor.UnfoldRecursive,
		},
		{
			"code.foldSelection",
			"Fold Selection / Remove Region", "Folding", "Cmd+.", editorAvailable, editor.FoldSelection,
		},
		{
			"code.collapseBlock",
			"Fold Code Block", "Folding", "Cmd+Shift+.", editorAvailable, editor.Fold,
		},
		{
			"code.expandToLevel1",
			"Expand to Level 1", "Folding", "", editorAvailable, editor.FoldLevel1,
		},
		{
			"code.expandToLevel2",
			"Expand to Level 2", "Folding", "", editorAvailable, editor.FoldLevel2,
		},
		{
			"code.expandToLevel3",
			"Expand to Level 3", "Folding", "", editorAvailable, editor.FoldLevel3,
		},
		{
			"code.expandToLevel4",
			"Expand to Level 4", "Folding", "", editorAvailable, editor.FoldLevel4,
		},
		{
			"code.expandToLevel5",
			"Expand to Level 5", "Folding", "", editorAvailable, editor.FoldLevel5,
		},
		{
			"code.expandAllToLevel1",
			"Expand All to Level 1", "Folding", "", editorAvailable, editor.FoldLevel1,
		},
		{
			"code.expandAllToLevel2",
			"Expand All to Level 2", "Folding", "", editorAvailable, editor.FoldLevel2,
		},
		{
			"code.expandAllToLevel3",
			"Expand All to Level 3", "Folding", "", editorAvailable, editor.FoldLevel3,
		},
		{
			"code.expandAllToLevel4",
			"Expand All to Level 4", "Folding", "", editorAvailable, editor.FoldLevel4,
		},
		{
			"code.expandAllToLevel5",
			"Expand All to Level 5", "Folding", "", editorAvailable, editor.FoldLevel5,
		},
		{
			"code.completion",
			"Code Completion", "Code", "Ctrl+Space", editorEditable, editor.ShowCompletions,
		},
		{
			"code.startNewLine",
			"Start New Line", "Code", "Shift+Enter", editorEditable, editor.NewlineBelow,
		},
		{
			"code.startNewLineBefore",
			"Start New Line Before Current", "Code", "Cmd+Alt+Enter", editorEditable, editor.NewlineAbove,
		},
		{
			"code.splitLine",
			"Split Line", "Code", "Cmd+Enter", editorEditable, editor.Newline,
		},
		{
			"code.sortLines",
			"Sort Lines", "Edit", "", editorEditable, editor.SortLines,
		},
		{
			"code.reverseLines",
			"Reverse Lines", "Edit", "", editorEditable, editor.ReverseLines,
		},
		{
			"editor.scrollToCenter",
			"Scroll to Center", "Editor Actions", "Ctrl+L", editorAvailable, editor.ScrollCursorCenter,
		},
		{
			"editor.matchBrace",
			"Move Caret to Matching Brace", "Editor Actions", "Ctrl+M", editorAvailable, editor.MoveToBracket,
		},
		{
			"editor.cloneCaretAbove",
			"Clone Caret Above", "Editor Actions", "", editorAvailable, editor.AddCursorAbove,
		},
		{
			"editor.cloneCaretBelow",
			"Clone Caret Below", "Editor Actions", "", editorAvailable, editor.AddCursorBelow,
		},
	}
	for _, spec := range commandActions {
		spec := spec
		register(spec.id, spec.title, spec.group, spec.shortcut, spec.when,
			func(m *Model) { m.runEditorCommand(spec.command) }, nil)
	}

	register("code.addCaretsToLineEnds", "Add Carets to Ends of Selected Lines",
		"Edit", "Alt+Shift+G", editorAvailable,
		func(m *Model) { m.addCaretsToSelectedLineEnds() }, nil)
	register("code.unselectOccurrence", "Unselect Occurrence",
		"Edit", "Ctrl+Shift+G", editorAvailable,
		func(m *Model) { m.unselectLatestOccurrence() }, nil)
	register("code.toggleCase", "Toggle Case",
		"Code", "Cmd+Shift+U", editorEditable,
		func(m *Model) { m.toggleEditorCase() }, nil)
	register("code.pasteAsPlainText", "Paste as Plain Text",
		"Edit", "Cmd+Alt+Shift+V", editorEditable,
		func(m *Model) { m.pasteEditorPlainText() }, nil)
	register("code.showErrorDescription", "Error Description",
		"View", "Cmd+F1", editorAvailable,
		func(m *Model) { m.showErrorDescription() }, nil)
	register("code.applyQuickFix", "Apply Quick Fix",
		"Code", "Alt+Shift+Enter", editorEditable,
		func(m *Model) { m.ApplyQuickFix() }, nil)
	register("code.parameterInfo", "Parameter Info",
		"Code", "Cmd+P", editorAvailable,
		func(m *Model) { m.ShowParameterInfo() }, nil)
	register("code.expressionTypeInfo", "Expression Type Info",
		"View", "Ctrl+Shift+P", editorAvailable,
		func(m *Model) { m.ShowExpressionTypeInfo() }, nil)
	register("code.generate", "Generate…",
		"Code", "Cmd+N", editorAvailable,
		func(m *Model) { m.ShowGenerate() }, nil)
	register("code.implementMethods", "Implement Methods…",
		"Code", "Ctrl+I", editorAvailable,
		func(m *Model) { m.ShowImplementMethods() }, nil)
	register("code.overrideMethods", "Override Methods…",
		"Code", "Ctrl+O", editorAvailable,
		func(m *Model) { m.ShowOverrideMethods() }, nil)
	if action, ok := actionGet("code.generate"); ok {
		action.KeyWhen = func(m *Model) bool { return m.activeEditorSupportsGenerate() }
	}
	register("refactor.thisMenu", "Refactor This…",
		"Refactor", "Ctrl+T", editorAvailable,
		func(m *Model) { m.ShowRefactorThis() }, nil)
	register("refactor.changeSignature", "Change Signature…",
		"Refactor", "Cmd+F6", editorAvailable,
		func(m *Model) { m.showRefactor("changeSignature") }, nil)
	register("refactor.extractVariable", "Introduce Variable…",
		"Refactor", "Cmd+Alt+V", editorAvailable,
		func(m *Model) { m.showRefactor("extractVariable") }, nil)
	register("refactor.extractMethod", "Extract Method…",
		"Refactor", "Cmd+Alt+M", editorAvailable,
		func(m *Model) { m.showRefactor("extractMethod") }, nil)
	register("refactor.extractConstant", "Introduce Constant…",
		"Refactor", "Cmd+Alt+C", editorAvailable,
		func(m *Model) { m.showRefactor("extractConstant") }, nil)
	register("refactor.extractField", "Introduce Field…",
		"Refactor", "Cmd+Alt+F", editorAvailable,
		func(m *Model) { m.showRefactor("extractField") }, nil)
	register("refactor.extractParameter", "Introduce Parameter…",
		"Refactor", "Cmd+Alt+P", editorAvailable,
		func(m *Model) { m.showRefactor("extractParameter") }, nil)
	register("refactor.inline", "Inline…",
		"Refactor", "Cmd+Alt+N", editorAvailable,
		func(m *Model) { m.showRefactor("inline") }, nil)
	register("refactor.move", "Move…",
		"Refactor", "F6", editorAvailable,
		func(m *Model) { m.showRefactor("move") }, nil)
	register("find.highlightUsages", "Highlight Usages in File",
		"Find", "Cmd+Shift+F7", editorAvailable,
		func(m *Model) { m.ToggleHighlightUsages() }, nil)
	register("navigate.nextHighlightedUsage", "Next Highlighted Usage",
		"Navigate", "Ctrl+Alt+Down", editorAvailable,
		func(m *Model) { m.StepHighlightedUsage(1) }, nil)
	register("navigate.prevHighlightedUsage", "Previous Highlighted Usage",
		"Navigate", "Ctrl+Alt+Up", editorAvailable,
		func(m *Model) { m.StepHighlightedUsage(-1) }, nil)
	register("navigate.declaration", "Go to Declaration or Usages",
		"Navigate", "Cmd+B", editorAvailable,
		func(m *Model) {
			if t := m.tab(); t != nil && m.ctx != nil && (m.mgr != nil || m.invokeHook != nil) {
				m.lspGoto(t, t.state.Cursor(), "declaration")
			}
		}, nil)
}

func (m *Model) addCaretsToSelectedLineEnds() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	snapshot := t.state.Buffer.Snapshot()
	var carets []editor.Selection
	for _, selection := range t.state.Selections() {
		if selection.Empty() {
			carets = append(carets, selection)
			continue
		}
		first := snapshot.LineOf(selection.Start())
		last := snapshot.LineOf(selection.End())
		if last > first && snapshot.LineStart(last) == selection.End() {
			last--
		}
		for line := first; line <= last; line++ {
			offset := snapshot.LineEnd(line)
			carets = append(carets, editor.Selection{Anchor: offset, Head: offset})
		}
	}
	if len(carets) == len(t.state.Selections()) {
		allEmpty := true
		for _, selection := range t.state.Selections() {
			allEmpty = allEmpty && selection.Empty()
		}
		if allEmpty {
			return
		}
	}
	t.state.SetSelections(carets...)
	m.invalidate()
}

func (m *Model) unselectLatestOccurrence() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	selections := t.state.Selections()
	if len(selections) == 0 {
		return
	}
	latest := t.state.Selection()
	if len(selections) == 1 {
		if latest.Empty() {
			return
		}
		t.state.Select(latest.Head, latest.Head)
		m.invalidate()
		return
	}
	remove := -1
	for i, selection := range selections {
		if selection == latest {
			remove = i
			break
		}
	}
	if remove < 0 {
		return
	}
	selections = append(selections[:remove], selections[remove+1:]...)
	t.state.SetSelections(selections...)
	m.invalidate()
}

func (m *Model) toggleEditorCase() {
	t := m.tab()
	if t == nil || t.state == nil || !m.activeEditorEditable() {
		return
	}
	snapshot := t.state.Buffer.Snapshot()
	selections := t.state.Selections()
	targets := make([]editor.Selection, len(selections))
	lower := false
	for i, selection := range selections {
		target := selection
		if target.Empty() {
			start, end := editorWordRange(snapshot.String(), target.Head)
			if start == end {
				continue
			}
			target = editor.Selection{Anchor: start, Head: end}
		}
		targets[i] = target
		text := snapshot.Slice(target.Start(), target.End())
		if text != strings.ToLower(text) {
			lower = true
		}
	}
	var edits []editor.Edit
	final := make([]editor.Selection, 0, len(selections))
	delta := 0
	for i, target := range targets {
		if target.Empty() {
			final = append(final, selections[i])
			continue
		}
		text := snapshot.Slice(target.Start(), target.End())
		if lower {
			text = strings.ToLower(text)
		} else {
			text = strings.ToUpper(text)
		}
		edits = append(edits, editor.Edit{Start: target.Start(), End: target.End(), Text: text})
		if selections[i].Empty() {
			start := target.Start() + delta
			final = append(final, editor.Selection{Anchor: start, Head: start + len(text)})
		} else if selections[i].Head >= selections[i].Anchor {
			final = append(final, editor.Selection{
				Anchor: target.Start() + delta,
				Head:   target.Start() + delta + len(text),
			})
		} else {
			final = append(final, editor.Selection{
				Anchor: target.Start() + delta + len(text),
				Head:   target.Start() + delta,
			})
		}
		delta += len(text) - (target.End() - target.Start())
	}
	if len(edits) == 0 {
		return
	}
	t.state.Buffer.Edit(edits...)
	t.state.SetSelections(final...)
	m.invalidate()
}

func editorWordRange(text string, offset int) (int, int) {
	offset = max(0, min(offset, len(text)))
	start, end := offset, offset
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		start -= size
	}
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		end += size
	}
	return start, end
}

func (m *Model) pasteEditorPlainText() {
	t := m.tab()
	if t == nil || t.state == nil || !m.activeEditorEditable() {
		return
	}
	t.state.Insert(readEditorClipboardText())
	m.invalidate()
}

func (m *Model) showErrorDescription() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	caret := t.state.Cursor()
	diagnostics := t.state.Diagnostics()
	best := -1
	for i, diagnostic := range diagnostics {
		if diagnostic.Start <= caret && caret <= diagnostic.End {
			if best < 0 || diagnostic.Severity < diagnostics[best].Severity {
				best = i
			}
		}
	}
	if best < 0 {
		m.status = "No error or warning under the caret"
		m.invalidate()
		return
	}
	diagnostic := diagnostics[best]
	text := diagnostic.Message
	if diagnostic.Source != "" {
		text = diagnostic.Source + ": " + text
	}
	t.state.ShowHover(diagnostic.Start, diagnostic.End, text, nil)
	m.invalidate()
}
