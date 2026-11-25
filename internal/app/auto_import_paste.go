package app

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"mayoi/internal/openidea"
)

func init() {
	_ = registerAction(&Action{
		ID:       "code.applyPastedImports",
		Title:    "Add Imports",
		Category: "Code",
		Keywords: "auto import paste unresolved symbol fix",
		RunWithArg: func(m *Model, arg any) {
			m.applyPastedImportSelection(arg)
		},
	})
}

func (m *Model) applyPastedImportSelection(arg any) {
	t := m.tab()
	if t == nil || !t.autoImport.paste.open || !m.autoImportPasteCurrent(t, t.autoImport.paste.gen) {
		return
	}
	indices, ok := pastedImportIndices(arg)
	if !ok {
		return
	}
	p := &t.autoImport.paste
	selected := make([]autoImportAction, 0, len(indices))
	for _, index := range indices {
		if index >= 0 && index < len(p.actions) {
			selected = append(selected, p.actions[index])
		}
	}
	if len(selected) == 0 {
		return
	}
	m.applyAutoImportPaste(t, selected)
}

func pastedImportIndices(arg any) ([]int, bool) {
	switch values := arg.(type) {
	case []int:
		return append([]int(nil), values...), true
	case []any:
		out := make([]int, 0, len(values))
		for _, value := range values {
			var index int
			switch n := value.(type) {
			case int:
				index = n
			case int32:
				index = int(n)
			case int64:
				index = int(n)
			case float64:
				index = int(n)
				if float64(index) != n {
					continue
				}
			default:
				continue
			}
			out = append(out, index)
		}
		return out, true
	case int:
		return []int{values}, true
	}
	return nil, false
}

func (m *Model) beginAutoImportPaste(t *Tab, ranges []autoImportPasteRange) {
	if len(ranges) == 0 || m.autoImportSetOnPaste() == "never" || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) || !m.tabOpen(t) {
		return
	}
	p := &t.autoImport.paste
	p.doc, p.docID, p.path, p.root = t.doc, t.doc.ID(), t.doc.Path(), m.root
	p.text, p.version = t.doc.Text(), t.doc.Buffer().Version()
	p.ranges = ranges
	p.diagnostics = make(map[string][]openidea.Diagnostic)
	gen := p.gen
	ctx, cancel := context.WithCancel(m.ctx)
	p.abort = cancel
	p.deadline = time.AfterFunc(autoImportPasteWait, func() {
		m.post(func() {
			if t.autoImport.paste.gen == gen {
				m.cancelAutoImportPaste(t)
				m.invalidate()
			}
		})
	})
	go func() {
		err := t.doc.Sync(ctx)
		m.post(func() {
			if !m.autoImportPasteCurrent(t, gen) {
				return
			}
			if err != nil {
				m.cancelAutoImportPaste(t)
				return
			}
			t.autoImport.paste.backendVersion = t.doc.Version()
			t.autoImport.paste.synced = true
			if len(t.autoImport.paste.diagnostics) > 0 {
				m.schedulePasteImportCollection(t)
			}
		})
	}()
}

func (m *Model) autoImportPasteCurrent(t *Tab, gen uint64) bool {
	if t == nil || t.doc == nil || t.state == nil {
		return false
	}
	p := &t.autoImport.paste
	return p.gen == gen && p.doc == t.doc && m.tabOpen(t) && p.docID == t.doc.ID() && p.root == m.root && !t.state.ReadOnly && !t.doc.ReadOnly() && m.autoImportSetOnPaste() != "never" && autoImportDocumentStable(t.doc, p.text, p.version)
}

func (m *Model) autoImportPastePublished(t *Tab, ev openidea.DiagnosticsEvent) {
	p := &t.autoImport.paste
	if p.running || !m.autoImportPasteCurrent(t, p.gen) {
		return
	}
	p.diagnostics[ev.Source] = append([]openidea.Diagnostic(nil), ev.Items...)
	if p.synced {
		m.schedulePasteImportCollection(t)
	}
}

func (m *Model) schedulePasteImportCollection(t *Tab) {
	p := &t.autoImport.paste
	if p.settle != nil {
		p.settle.Stop()
	}
	gen := p.gen
	p.settle = time.AfterFunc(autoImportPasteSettle, func() { m.post(func() { m.collectAutoImportPaste(t, gen) }) })
}

func pasteDiagnosticMatches(text string, ranges []autoImportPasteRange, d openidea.Diagnostic) bool {
	if d.Unnecessary || d.Deprecated || (!autoImportPasteUnresolved.MatchString(d.Message) && (d.Code == nil || !autoImportPasteUnresolvedCode.MatchString(*d.Code))) {
		return false
	}
	from, ok := autoImportRangeStart(text, d.Range.Start)
	if !ok {
		return false
	}
	to, ok := autoImportRangeStart(text, d.Range.End)
	if !ok || from >= to {
		return false
	}
	for _, r := range ranges {
		if r.from <= from && to <= r.to {
			return true
		}
	}
	return false
}

func pastedImport(actions []autoImportAction) (autoImportAction, bool) {
	var found autoImportAction
	n := 0
	for _, a := range actions {
		if a.DisabledReason == "" && (a.Kind == "" || a.Kind == "quickfix" || strings.HasPrefix(a.Kind, "quickfix.")) && autoImportPasteFixTitle.MatchString(a.Title) && !autoImportPasteNotImport.MatchString(a.Title) {
			found = a
			n++
		}
	}
	return found, n == 1
}

func compareImportPos(a, b openidea.OutlinePos) int {
	if a.Line < b.Line {
		return -1
	}
	if a.Line > b.Line {
		return 1
	}
	if a.Col < b.Col {
		return -1
	}
	if a.Col > b.Col {
		return 1
	}
	return 0
}

func mergePastedImports(path string, actions []autoImportAction) *autoImportWorkspaceEdit {
	var edits []autoImportTextEdit
	var version *int32
	guards := map[string]string{}
	for _, a := range actions {
		if a.Edit == nil || len(a.Edit.Files) == 0 {
			return nil
		}
		for _, p := range a.Edit.Preconditions {
			if h, ok := guards[p.Path]; ok && h != p.Hash {
				return nil
			}
			guards[p.Path] = p.Hash
		}
		for _, f := range a.Edit.Files {
			if f.Kind != "edit" || f.Path != path || len(f.Edits) == 0 {
				return nil
			}
			if version != nil && f.Version != nil && *version != *f.Version {
				return nil
			}
			if version == nil {
				version = f.Version
			}
			for _, e := range f.Edits {
				if strings.TrimSpace(e.NewText) == "" || compareImportPos(e.Range.Start, e.Range.End) > 0 {
					return nil
				}
				exists := false
				for _, old := range edits {
					if old == e {
						exists = true
						break
					}
				}
				if !exists {
					edits = append(edits, e)
				}
			}
		}
	}
	sort.Slice(edits, func(i, j int) bool {
		n := compareImportPos(edits[i].Range.Start, edits[j].Range.Start)
		if n != 0 {
			return n < 0
		}
		return compareImportPos(edits[i].Range.End, edits[j].Range.End) < 0
	})
	var merged []autoImportTextEdit
	for _, e := range edits {
		if len(merged) > 0 {
			prev := &merged[len(merged)-1]
			same := compareImportPos(prev.Range.Start, e.Range.Start) == 0
			both := compareImportPos(prev.Range.Start, prev.Range.End) == 0 && compareImportPos(e.Range.Start, e.Range.End) == 0
			if same && both {
				if !strings.HasSuffix(prev.NewText, "\n") || !strings.HasSuffix(e.NewText, "\n") {
					return nil
				}
				prev.NewText += e.NewText
				continue
			}
			if same || compareImportPos(e.Range.Start, prev.Range.End) < 0 {
				return nil
			}
		}
		merged = append(merged, e)
	}
	if len(merged) == 0 {
		return nil
	}
	out := &autoImportWorkspaceEdit{Files: []autoImportFileEdit{{Kind: "edit", Path: path, Version: version, Edits: merged}}}
	for path, hash := range guards {
		out.Preconditions = append(out.Preconditions, autoImportWorkspacePrecond{Path: path, Hash: hash})
	}
	return out
}

func (m *Model) collectAutoImportPaste(t *Tab, gen uint64) {
	if !m.autoImportPasteCurrent(t, gen) {
		return
	}
	p := &t.autoImport.paste
	if p.running {
		return
	}
	var diagnostics []openidea.Diagnostic
	seen := map[string]bool{}
	for _, items := range p.diagnostics {
		for _, d := range items {
			key := autoImportDiagnosticKey(d)
			if !seen[key] && pasteDiagnosticMatches(p.text, p.ranges, d) {
				seen[key] = true
				diagnostics = append(diagnostics, d)
			}
		}
	}
	if len(diagnostics) == 0 {
		return
	}
	p.running = true
	docID, path, backendVersion := p.docID, p.path, p.backendVersion
	ctx, cancel := context.WithCancel(m.ctx)
	oldCancel := p.abort
	p.abort = func() { cancel(); oldCancel() }
	go func() {
		var found []autoImportAction
		for _, d := range diagnostics {
			if ctx.Err() != nil {
				break
			}
			request := m.nextRequest()
			stop := context.AfterFunc(ctx, func() { m.cancelRequest(request) })
			actions, err := m.autoImportCodeActionsRequest(ctx, request, docID, d.Range, "quickfix")
			stop()
			if err != nil {
				continue
			}
			a, ok := pastedImport(autoImportActionsForDiagnostic(actions, d))
			if !ok {
				continue
			}
			a = pasteCommandEdit(a, path)
			if a.Edit == nil {
				raw, err := m.invoke(ctx, "lsp_resolve_code_action", map[string]any{"doc": docID, "action": a.Raw})
				if err != nil || json.Unmarshal(raw, &a) != nil {
					continue
				}
			}
			a = pasteCommandEdit(a, path)
			if _, ok = pastedImport([]autoImportAction{a}); !ok || mergePastedImports(path, []autoImportAction{a}) == nil {
				continue
			}
			if mergePastedImports(path, append(append([]autoImportAction(nil), found...), a)) != nil {
				found = append(found, a)
			}
		}
		var prepared autoImportWorkspaceEdit
		if len(found) > 0 {
			merged := mergePastedImports(path, found)
			if backendVersion > uint32(1<<31-1) {
				found = nil
				return
			}
			expected := int32(backendVersion)
			if merged.Files[0].Version != nil && *merged.Files[0].Version != expected {
				found = nil
				return
			}
			merged.Files[0].Version = &expected
			raw, err := m.invoke(ctx, "prepare_workspace_edit", map[string]any{"edit": merged})
			if err != nil || json.Unmarshal(raw, &prepared) != nil || len(prepared.Preconditions) == 0 || len(prepared.Files) != 1 {
				found = nil
			}
		}
		m.post(func() {
			if !m.autoImportPasteCurrent(t, gen) {
				return
			}
			p := &t.autoImport.paste
			p.edit = &prepared
			if len(found) == 0 {
				m.cancelAutoImportPaste(t)
				return
			}
			if p.deadline != nil {
				p.deadline.Stop()
			}
			p.actions = found
			if m.autoImportSetOnPaste() == "always" {
				m.applyAutoImportPaste(t, found)
				return
			}
			p.open = true
			p.selected = map[int]bool{}
			for i := range found {
				p.selected[i] = true
			}
			m.invalidate()
		})
	}()
}

func (m *Model) applyAutoImportPaste(t *Tab, actions []autoImportAction) {
	p := &t.autoImport.paste
	if !m.autoImportPasteCurrent(t, p.gen) {
		m.cancelAutoImportPaste(t)
		return
	}
	edit := mergePastedImports(p.path, actions)
	if edit != nil && p.edit != nil {
		edit.Preconditions = append([]autoImportWorkspacePrecond(nil), p.edit.Preconditions...)
		edit.Files[0].Version = p.edit.Files[0].Version
	}
	doc, text, version := p.doc, p.text, p.version
	m.cancelAutoImportPaste(t)
	m.invalidate()
	if edit == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, autoImportPasteTimeout)
		defer cancel()
		if !autoImportDocumentStable(doc, text, version) {
			return
		}
		_, _ = m.invoke(ctx, "apply_workspace_edit", map[string]any{"edit": edit})
	}()
}

func (m *Model) autoImportPasteDialog(c *ui.Context, theme *ui.Theme) {
	t := m.tab()
	if t == nil {
		return
	}
	p := &t.autoImport.paste
	if !m.autoImportPasteCurrent(t, p.gen) {
		m.cancelAutoImportPaste(t)
		return
	}
	open := p.open
	d := nativeModal(c, &open, func() {
		ui.Column(c).Width(460).Gap(8).Children(func() {
			ui.Text(c, "Add Imports").FontSize(13).Bold().TextColor(theme.Text)
			for i, a := range p.actions {
				selected := p.selected[i]
				if ui.Checkbox(c, &selected, a.Title).Changed() {
					p.selected[i] = selected
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Add Imports").OnClick(func() {
					var selected []autoImportAction
					for i, a := range p.actions {
						if p.selected[i] {
							selected = append(selected, a)
						}
					}
					m.applyAutoImportPaste(t, selected)
				})
				ui.Button(c, "Cancel").OnClick(func() { m.cancelAutoImportPaste(t); m.invalidate() })
			})
		})
	})
	if d.Dismissed() || !open {
		m.cancelAutoImportPaste(t)
		m.invalidate()
	}
}

// pasteCommandEdit extracts an LSP workspace edit embedded in a command. It
// never executes arbitrary commands, and rejects edits outside this document.
type autoImportCommand struct {
	Arguments []json.RawMessage `json:"arguments"`
}
type pasteLSPTextEdit struct {
	Range struct {
		Start, End struct{ Line, Character uint32 }
	} `json:"range"`
	NewText string `json:"newText"`
}
type pasteLSPWorkspaceEdit struct {
	Changes         map[string][]pasteLSPTextEdit `json:"changes"`
	DocumentChanges []struct {
		TextDocument *struct {
			URI     string `json:"uri"`
			Version *int32 `json:"version"`
		} `json:"textDocument"`
		Edits []pasteLSPTextEdit `json:"edits"`
	} `json:"documentChanges"`
}

func pasteCommandEdit(a autoImportAction, path string) autoImportAction {
	if a.Edit != nil || a.Command == nil {
		return a
	}
	var files []autoImportFileEdit
	add := func(uri string, version *int32, edits []pasteLSPTextEdit) bool {
		u, err := url.Parse(uri)
		if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
			return false
		}
		decoded, err := url.PathUnescape(u.EscapedPath())
		if err != nil || filepath.Clean(decoded) != filepath.Clean(path) {
			return false
		}
		file := autoImportFileEdit{Kind: "edit", Path: path, Version: version}
		for _, e := range edits {
			file.Edits = append(file.Edits, autoImportTextEdit{Range: openidea.OutlineRange{Start: openidea.OutlinePos{Line: e.Range.Start.Line, Col: e.Range.Start.Character}, End: openidea.OutlinePos{Line: e.Range.End.Line, Col: e.Range.End.Character}}, NewText: e.NewText})
		}
		files = append(files, file)
		return true
	}
	for _, raw := range a.Command.Arguments {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			raw = json.RawMessage(value)
		}
		var edit pasteLSPWorkspaceEdit
		if json.Unmarshal(raw, &edit) != nil {
			continue
		}
		if edit.DocumentChanges != nil {
			for _, change := range edit.DocumentChanges {
				if change.TextDocument == nil || change.Edits == nil || !add(change.TextDocument.URI, change.TextDocument.Version, change.Edits) {
					return a
				}
			}
		} else {
			for uri, edits := range edit.Changes {
				if !add(uri, nil, edits) {
					return a
				}
			}
		}
	}
	if len(files) > 0 {
		a.Edit = &autoImportWorkspaceEdit{Files: files}
	}
	return a
}

// pastedSelectionRanges reconstructs the edit from before/after caret positions.
// This also handles a clipboard containing a different piece for each caret.
func pastedSelectionRanges(before, after string, selections, final []editor.Selection) []autoImportPasteRange {
	type replacement struct{ start, end int }
	if len(selections) != len(final) {
		return nil
	}
	changes := make([]replacement, 0, len(selections))
	ends := make([]int, 0, len(final))
	for _, s := range selections {
		changes = append(changes, replacement{min(s.Anchor, s.Head), max(s.Anchor, s.Head)})
	}
	for _, s := range final {
		if !s.Empty() {
			return nil
		}
		ends = append(ends, s.Head)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].start < changes[j].start })
	sort.Ints(ends)
	var result strings.Builder
	var ranges []autoImportPasteRange
	previous := 0
	for i, c := range changes {
		if c.start < previous || c.end > len(before) {
			return nil
		}
		result.WriteString(before[previous:c.start])
		start := result.Len()
		end := ends[i]
		if end < start || end > len(after) {
			return nil
		}
		result.WriteString(after[start:end])
		if end > start {
			ranges = append(ranges, autoImportPasteRange{start, end})
		}
		previous = c.end
	}
	result.WriteString(before[previous:])
	if result.String() != after {
		return nil
	}
	return ranges
}

func pasteEditRanges(edits []editor.Edit) []autoImportPasteRange {
	sorted := append([]editor.Edit(nil), edits...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	var ranges []autoImportPasteRange
	delta, end := 0, 0
	for _, e := range sorted {
		if e.Start < end || e.End < e.Start {
			return nil
		}
		start := e.Start + delta
		if len(e.Text) > 0 {
			ranges = append(ranges, autoImportPasteRange{start, start + len(e.Text)})
		}
		delta += len(e.Text) - (e.End - e.Start)
		end = e.End
	}
	return ranges
}
