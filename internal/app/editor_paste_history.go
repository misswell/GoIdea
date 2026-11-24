package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const maxEditorClipboardEntries = 30

var pasteHistoryClipboardWrite = mygo.Clipboard.WriteText

type editorClipboardEntry struct {
	text     string
	linewise bool
}

type editorExtrasState struct {
	history  []editorClipboardEntry
	open     bool
	tab      *Tab
	query    string
	selected int
	chosen   ui.Selection[string]
	list     ui.ListState
}

// rememberEditorClipboard records text in the same newest-first, deduplicated
// history OpenIDEA keeps for Edit › Paste from History.
func (m *Model) rememberEditorClipboard(text string, linewise bool) {
	if text == "" && !linewise {
		return
	}
	history := m.editorExtras.history
	out := make([]editorClipboardEntry, 0, min(len(history)+1, maxEditorClipboardEntries))
	out = append(out, editorClipboardEntry{text: text, linewise: linewise})
	for _, entry := range history {
		if entry.text == text || len(out) == maxEditorClipboardEntries {
			continue
		}
		out = append(out, entry)
	}
	m.editorExtras.history = out
}

// copiedEditorText is what MyGo copies for the active selections: selected
// ranges, or each distinct caret line when every selection is empty.
func copiedEditorText(t *Tab) (text string, linewise bool) {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return "", false
	}
	selections := t.state.Selections()
	linewise = true
	var ranges []string
	for _, selection := range selections {
		if selection.Anchor == selection.Head {
			continue
		}
		linewise = false
		start, end := selection.Anchor, selection.Head
		if end < start {
			start, end = end, start
		}
		ranges = append(ranges, t.state.Buffer.Slice(start, end))
	}
	if len(ranges) > 0 {
		return strings.Join(ranges, "\n"), false
	}
	snapshot := t.state.Buffer.Snapshot()
	lines := make([]string, 0, len(selections))
	seen := map[int]bool{}
	for _, selection := range selections {
		line := snapshot.Pos(selection.Head).Line
		if seen[line] {
			continue
		}
		seen[line] = true
		lines = append(lines, snapshot.Line(line))
	}
	return strings.Join(lines, "\n"), linewise
}

func (m *Model) rememberActiveEditorClipboard() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	text, linewise := copiedEditorText(t)
	m.rememberEditorClipboard(text, linewise)
}

func (m *Model) OpenPasteFromHistory() {
	t := m.tab()
	if t == nil || t.state == nil || t.state.ReadOnly ||
		(t.doc != nil && t.doc.ReadOnly()) {
		return
	}
	if len(m.editorExtras.history) == 0 {
		m.status = "Nothing to paste: the clipboard history is empty"
		m.invalidate()
		return
	}
	s := &m.editorExtras
	s.open = true
	s.tab = t
	s.query = ""
	s.selected = 0
	s.chosen.Clear()
	if len(s.history) > 0 {
		s.chosen.Add(s.history[0].text)
	}
	s.list = ui.ListState{}
	m.invalidate()
}

func (m *Model) closePasteFromHistory() {
	m.editorExtras.open = false
	m.editorExtras.tab = nil
	m.editorExtras.query = ""
	m.editorExtras.chosen.Clear()
	m.invalidate()
}

func (m *Model) pasteHistoryVisibleEntries() []editorClipboardEntry {
	s := &m.editorExtras
	query := strings.ToLower(strings.TrimSpace(s.query))
	out := make([]editorClipboardEntry, 0, len(s.history))
	for _, entry := range s.history {
		if query == "" || strings.Contains(strings.ToLower(entry.text), query) {
			out = append(out, entry)
		}
	}
	return out
}

func (m *Model) filterPasteHistory(query string) {
	s := &m.editorExtras
	s.query = query
	s.selected = 0
	s.chosen.Clear()
	entries := m.pasteHistoryVisibleEntries()
	if len(entries) > 0 {
		s.chosen.Add(entries[0].text)
	}
	s.list = ui.ListState{}
	m.invalidate()
}

func (m *Model) stepPasteHistory(delta int) {
	s := &m.editorExtras
	entries := m.pasteHistoryVisibleEntries()
	if len(entries) == 0 {
		return
	}
	s.selected = max(0, min(s.selected+delta, len(entries)-1))
	m.invalidate()
}

func (m *Model) deleteSelectedPasteHistory() {
	s := &m.editorExtras
	if s.chosen.Len() == 0 {
		return
	}
	removed := map[string]bool{}
	for text := range s.chosen.All() {
		removed[text] = true
	}
	history := s.history[:0]
	for _, entry := range s.history {
		if !removed[entry.text] {
			history = append(history, entry)
		}
	}
	s.history = history
	s.chosen.Clear()
	entries := m.pasteHistoryVisibleEntries()
	if len(entries) == 0 {
		s.selected = 0
		return
	}
	s.selected = max(0, min(s.selected, len(entries)-1))
	s.chosen.Add(entries[s.selected].text)
	m.invalidate()
}

// PasteFromHistory pastes the selected rows in visible history order. A
// command argument can select one row by text, as OpenIDEA's action accepts.
func (m *Model) PasteFromHistory(arg any) {
	entries := m.pasteHistoryVisibleEntries()
	var chosen []editorClipboardEntry
	if only, ok := arg.(string); ok {
		for _, entry := range entries {
			if entry.text == only {
				chosen = append(chosen, entry)
				break
			}
		}
	} else {
		for _, entry := range entries {
			if m.editorExtras.chosen.Has(entry.text) {
				chosen = append(chosen, entry)
			}
		}
	}
	if len(chosen) == 0 {
		return
	}
	parts := make([]string, len(chosen))
	for i, entry := range chosen {
		parts[i] = entry.text
	}
	text := strings.Join(parts, "\n")
	linewise := len(chosen) == 1 && chosen[0].linewise
	tab := m.editorExtras.tab
	m.closePasteFromHistory()
	if tab == nil || tab.state == nil || tab.state.ReadOnly ||
		(tab.doc != nil && tab.doc.ReadOnly()) {
		return
	}
	m.rememberEditorClipboard(text, linewise)
	pasteHistoryClipboardWrite(text)
	m.pasteEditorHistoryText(tab, text, linewise)
}

// pasteEditorHistoryText follows OpenIDEA's dispatchPaste for one or many
// carets. Line-wise history is inserted above every distinct caret line.
func (m *Model) pasteEditorHistoryText(t *Tab, text string, linewise bool) {
	if t == nil || t.state == nil || t.state.Buffer == nil || t.state.ReadOnly {
		return
	}
	t.state.SetSuggestion(t.state.Cursor(), "")
	selections := t.state.Selections()
	if len(selections) == 1 {
		if linewise && selections[0].Empty() {
			snapshot := t.state.Buffer.Snapshot()
			line := snapshot.Pos(selections[0].Head).Line
			start := snapshot.LineStart(line)
			insert := text + "\n"
			t.state.Buffer.Edit(editor.Edit{Start: start, End: start, Text: insert})
			m.markAutoImportDirty(t)
			m.beginAutoImportPaste(t, []autoImportPasteRange{{start, start + len(insert)}})
			caret := selections[0].Head + len(insert)
			t.state.SetSelections(editor.Selection{Anchor: caret, Head: caret})
			m.invalidate()
			return
		}
		before := t.state.Buffer.String()
		t.state.Insert(text)
		ranges := pastedSelectionRanges(before, t.state.Buffer.String(), selections, t.state.Selections())
		m.markAutoImportDirty(t)
		m.beginAutoImportPaste(t, ranges)
		m.invalidate()
		return
	}
	snapshot := t.state.Buffer.Snapshot()
	type insertion struct {
		start int
		end   int
		text  string
	}
	insertions := make([]insertion, 0, len(selections))
	if linewise {
		allEmpty := true
		seenLine := map[int]bool{}
		for _, selection := range selections {
			if !selection.Empty() {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			for _, selection := range selections {
				line := snapshot.Pos(selection.Head).Line
				if seenLine[line] {
					continue
				}
				seenLine[line] = true
				start := snapshot.LineStart(line)
				insertions = append(insertions, insertion{start: start, end: start, text: text + "\n"})
			}
		}
	}
	if len(insertions) == 0 {
		parts := pasteLinesPerCaret(text, len(selections))
		for i, selection := range selections {
			start, end := selection.Anchor, selection.Head
			if end < start {
				start, end = end, start
			}
			value := text
			if parts != nil {
				value = parts[i]
			}
			insertions = append(insertions, insertion{start: start, end: end, text: value})
		}
	}
	edits := make([]editor.Edit, len(insertions))
	for i, change := range insertions {
		edits[i] = editor.Edit{Start: change.start, End: change.end, Text: change.text}
	}
	t.state.Buffer.Edit(edits...)
	ranges := pasteEditRanges(edits)
	m.markAutoImportDirty(t)
	m.beginAutoImportPaste(t, ranges)
	final := make([]editor.Selection, len(selections))
	for i, selection := range selections {
		start, end := selection.Anchor, selection.Head
		if end < start {
			start, end = end, start
		}
		caret := end
		for _, change := range insertions {
			if change.start < start {
				caret += len(change.text) - (change.end - change.start)
			} else if change.start == start {
				caret = change.start + len(change.text)
				break
			}
		}
		final[i] = editor.Selection{Anchor: caret, Head: caret}
	}
	t.state.SetSelections(final...)
	m.invalidate()
}

func pasteLinesPerCaret(text string, carets int) []string {
	if carets <= 1 {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	if len(lines) == 2 && lines[1] == "" {
		lines = lines[:1]
	}
	if len(lines) == 1 {
		out := make([]string, carets)
		for i := range out {
			out[i] = lines[0]
		}
		return out
	}
	out := make([]string, carets)
	copy(out, lines)
	return out
}

func editorClipboardLabel(text string, maxLen int) (label, detail string) {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	first := ""
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			first = strings.TrimSpace(line)
			break
		}
	}
	runes := []rune(first)
	if len(runes) > maxLen {
		label = string(runes[:max(0, maxLen-1)]) + "…"
	} else {
		label = first
	}
	if label == "" {
		label = "(whitespace)"
	}
	count := len(lines)
	if count > 1 && strings.HasSuffix(text, "\n") {
		count--
	}
	if count > 1 {
		detail = fmt.Sprintf("%d lines", count)
	} else {
		detail = "1 line"
	}
	return label, detail
}

func (m *Model) editorPasteHistoryDialog(c *ui.Context, p *ui.Theme) {
	s := &m.editorExtras
	if !s.open {
		return
	}
	entries := m.pasteHistoryVisibleEntries()
	s.list.Key = func(i int) any {
		if i < 0 || i >= len(entries) {
			return ""
		}
		return entries[i].text
	}
	s.list.Selection = &s.chosen
	s.list.Selected = &s.selected
	open := true
	dialog := ui.Modal(c.Key("editor.paste-history"), &open, func() {
		ui.Column(c).Width(680).Gap(8).Children(func() {
			ui.Text(c, "Choose Content to Paste").FontSize(13).Bold().SingleLine()
			input := ui.TextInput(c, &s.query).
				AutoFocus().
				Placeholder("Filter clipboard history")
			input.OnChange(func() { m.filterPasteHistory(s.query) })
			input.OnShortcut(0, ui.KeyUp, func() { m.stepPasteHistory(-1) })
			input.OnShortcut(0, ui.KeyDown, func() { m.stepPasteHistory(1) })
			if input.Submitted() {
				m.PasteFromHistory(nil)
			}
			if len(entries) == 0 {
				message := "Nothing to paste: the clipboard history is empty"
				if len(s.history) > 0 {
					message = "No matches"
				}
				ui.Text(c, message).Padding(12).FontSize(12).TextColor(p.TextMuted)
			} else {
				list := ui.List(c, &s.list, len(entries)).
					Label("Clipboard history").
					Grow(1).
					ItemKey(s.list.Key).
					Selection(&s.chosen).
					Rows(func(row ui.ListRow) {
						if row.Index < 0 || row.Index >= len(entries) {
							return
						}
						entry := entries[row.Index]
						label, detail := editorClipboardLabel(entry.text, 80)
						r := ui.Row(row.Context).
							Key(entry.text).
							Height(28).
							Padding(0, 8).
							Gap(12).
							AlignItems(ui.Center).
							Selected(s.chosen.Has(entry.text))
						if row.Index == s.selected {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							ui.Text(row.Context, label).
								Grow(1).
								FontSize(12).
								TextColor(p.Text).
								SingleLine().
								Ellipsis("…")
							ui.Text(row.Context, detail).
								FontSize(11).
								TextColor(p.TextMuted).
								SingleLine()
						})
						if r.RowDoubleClicked() {
							m.PasteFromHistory(entry.text)
						}
					})
				list.OnShortcut(0, ui.KeyEnter, func() { m.PasteFromHistory(nil) })
				list.OnShortcut(ui.Cmd, ui.KeyEnter, func() { m.PasteFromHistory(nil) })
				list.OnShortcut(0, ui.KeyDelete, m.deleteSelectedPasteHistory)
				list.OnShortcut(0, ui.KeyBackspace, func() {
					if s.query == "" {
						m.deleteSelectedPasteHistory()
					}
				})
				if list.Submitted() {
					m.PasteFromHistory(nil)
				}
			}
			preview := ""
			if len(entries) > 0 {
				index := max(0, min(s.selected, len(entries)-1))
				preview = entries[index].text
			}
			if preview != "" {
				ui.Column(c).
					Height(110).
					Padding(8).
					Background(p.Surface).
					Border(1, p.Border).
					Clip().
					Children(func() {
						ui.Text(c, preview).
							FontSize(11).
							TextColor(p.TextMuted)
					})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Delete").
					Disabled(s.chosen.Len() == 0).
					Clicked() {
					m.deleteSelectedPasteHistory()
				}
				ui.Spacer(c)
				if ui.Button(c, "Paste").
					Disabled(s.chosen.Len() == 0).
					Clicked() {
					m.PasteFromHistory(nil)
				}
				if ui.Button(c, "Cancel").Clicked() {
					open = false
				}
			})
		})
	})
	dialog.Padding(0)
	if dialog.Dismissed() || !open {
		m.closePasteFromHistory()
	}
}
