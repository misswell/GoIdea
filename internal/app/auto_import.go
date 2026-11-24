package app

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

const (
	autoImportIdleDelay     = 1500 * time.Millisecond
	autoImportPasteWait     = 8 * time.Second
	autoImportPasteSettle   = 200 * time.Millisecond
	autoImportPasteTimeout  = autoImportPasteWait
	autoImportPasteMaxPicks = 64
)

type autoImportState struct {
	timer   *time.Timer
	gen     uint64
	running bool
	dirty   bool
	tried   map[string]bool
	paste   autoImportPasteState
}

type autoImportAction struct {
	Raw            json.RawMessage          `json:"-"`
	Title          string                   `json:"title"`
	Kind           string                   `json:"kind"`
	IsPreferred    bool                     `json:"isPreferred"`
	DisabledReason string                   `json:"disabledReason"`
	Diagnostics    []openidea.Diagnostic    `json:"diagnostics"`
	Edit           *autoImportWorkspaceEdit `json:"edit"`
	Command        *autoImportCommand       `json:"command"`
}

type autoImportWorkspaceEdit struct {
	Files         []autoImportFileEdit         `json:"files"`
	Preconditions []autoImportWorkspacePrecond `json:"preconditions,omitempty"`
}

type autoImportFileEdit struct {
	Kind    string               `json:"kind"`
	Path    string               `json:"path"`
	Edits   []autoImportTextEdit `json:"edits"`
	Version *int32               `json:"version,omitempty"`
}

type autoImportWorkspacePrecond struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type autoImportTextEdit struct {
	Range   openidea.OutlineRange `json:"range"`
	NewText string                `json:"new_text"`
}

type autoImportPasteRange struct {
	from int
	to   int
}

type autoImportPasteState struct {
	gen            uint64
	doc            *docbridge.Document
	docID          uint32
	path           string
	root           string
	text           string
	version        uint64
	backendVersion uint32
	ranges         []autoImportPasteRange
	diagnostics    map[string][]openidea.Diagnostic
	deadline       *time.Timer
	settle         *time.Timer
	abort          context.CancelFunc
	requests       []uint32
	running        bool
	synced         bool
	open           bool
	actions        []autoImportAction
	edit           *autoImportWorkspaceEdit
	selected       map[int]bool
}

var autoImportTitle = regexp.MustCompile(`(?i)\bimport\b`)

var (
	autoImportPasteFixTitle = regexp.MustCompile(
		`(?i)^(?:(?:add|insert|update) (?:an? )?import\b|import\b|add .+ to (?:an? )?existing import\b)`,
	)
	autoImportPasteUnresolved = regexp.MustCompile(
		`(?i)cannot find (?:name|namespace|type|value|function|macro|trait|module)|cannot be resolved|failed to resolve|unresolved (?:reference|name|type|symbol|import)|use of undeclared|undeclared (?:name|identifier)|undefined(?::| (?:name|variable))|not defined|unknown (?:identifier|symbol|type|name)|does not exist in (?:the )?current context|type or namespace name .+ could not be found`,
	)
	autoImportPasteUnresolvedCode = regexp.MustCompile(
		`^(?:2304|2552|2503|reportUndefinedVariable|UNRESOLVED_REFERENCE|undeclared_var_use|E0405|E0412|E0422|E0425|E0432|E0433|CS0103|CS0246)$`,
	)
	autoImportPasteNotImport = regexp.MustCompile(`(?i)^import (?:all|missing)\b`)
)

func (m *Model) markAutoImportDirty(t *Tab) {
	if t == nil || t.state == nil || t.state.ReadOnly {
		return
	}
	m.cancelAutoImportPaste(t)
	if !m.autoImportEnabled() {
		return
	}
	t.autoImport.dirty = true
	m.scheduleAutoImport(t)
}

func (m *Model) cancelAutoImportPaste(t *Tab) {
	if t == nil {
		return
	}
	pending := &t.autoImport.paste
	if pending.deadline != nil {
		pending.deadline.Stop()
	}
	if pending.settle != nil {
		pending.settle.Stop()
	}
	if pending.abort != nil {
		pending.abort()
	}
	for _, request := range pending.requests {
		m.cancelRequest(request)
	}
	generation := pending.gen + 1
	*pending = autoImportPasteState{gen: generation}
}

func (m *Model) scheduleAutoImport(t *Tab) {
	if t == nil || !m.tabOpen(t) || t.doc == nil || t.state == nil ||
		t.state.ReadOnly || t.doc.ReadOnly() || !m.autoImportEnabled() {
		return
	}
	if t.autoImport.timer != nil {
		t.autoImport.timer.Stop()
	}
	t.autoImport.gen++
	gen := t.autoImport.gen
	t.autoImport.timer = time.AfterFunc(autoImportIdleDelay, func() {
		m.post(func() {
			if t.autoImport.gen == gen {
				t.autoImport.timer = nil
			}
			if !m.tabOpen(t) || t.autoImport.gen != gen || t.autoImport.running ||
				!m.autoImportEnabled() || !t.editorFocused {
				return
			}
			m.runAutoImport(t)
		})
	})
}

func (m *Model) autoImportEnabled() bool {
	return m.autoImportAddUnambiguous() || m.autoImportOptimizeOnTheFly()
}

func (m *Model) runAutoImport(t *Tab) {
	if t == nil || t.doc == nil || t.state == nil || !m.tabOpen(t) ||
		t.state.ReadOnly || t.doc.ReadOnly() || !t.editorFocused {
		return
	}
	addUnambiguous := m.autoImportAddUnambiguous()
	optimize := m.autoImportOptimizeOnTheFly() && t.autoImport.dirty
	if !addUnambiguous && !optimize {
		return
	}

	text := t.doc.Text()
	version := t.doc.Buffer().Version()
	caret := t.state.Cursor()
	caretLine := lineForOffset(text, caret)
	path := t.doc.Path()
	errors := m.autoImportErrors(t, caretLine)
	noErrorsAtStart := !autoImportHasErrors(m.autoImportDiagnostics(t))
	candidates := make([]autoImportCandidate, 0, min(8, len(errors)))
	for _, diagnostic := range errors {
		if len(candidates) >= 8 {
			break
		}
		key := autoImportDiagnosticKey(diagnostic)
		if t.autoImport.tried[key] {
			continue
		}
		if t.autoImport.tried == nil {
			t.autoImport.tried = map[string]bool{}
		}
		t.autoImport.tried[key] = true
		candidates = append(candidates, autoImportCandidate{key: key, diagnostic: diagnostic})
	}
	if len(t.autoImport.tried) > 500 {
		clear(t.autoImport.tried)
	}

	t.autoImport.running = true
	doc := t.doc
	docID := doc.ID()
	runGen := t.autoImport.gen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		var (
			applied     bool
			optimizeRan bool
		)
		if err := doc.Sync(ctx); err == nil {
			if addUnambiguous {
				action, ok := m.findUnambiguousImport(ctx, docID, path, caretLine, candidates)
				if ok && len(action) > 0 && autoImportDocumentStable(doc, text, version) {
					_, err := m.invoke(ctx, "lsp_apply_code_action", map[string]any{
						"doc": docID, "action": action,
					})
					applied = err == nil
				}
			}
			if optimize && !applied && noErrorsAtStart &&
				autoImportDocumentStable(doc, text, version) {
				optimizeRan = true
				action, ok := m.findOptimizeImportAction(ctx, docID, doc, text, path, caretLine)
				if ok && len(action) > 0 && autoImportDocumentStable(doc, text, version) {
					_, _ = m.invoke(ctx, "lsp_apply_code_action", map[string]any{
						"doc": docID, "action": action,
					})
				}
			}
		}
		m.post(func() {
			t.autoImport.running = false
			if !m.tabOpen(t) {
				return
			}
			if optimizeRan {
				t.autoImport.dirty = false
			}
			pending := t.autoImport.gen != runGen && t.autoImport.dirty
			m.invalidate()
			if pending {
				m.scheduleAutoImport(t)
			}
		})
	}()
}

// findUnambiguousImport asks once per changed error, as OpenIDEA does. A
// result is usable only when exactly one enabled import quickfix is offered
// and its edit leaves the caret line alone.
func (m *Model) findUnambiguousImport(
	ctx context.Context,
	docID uint32,
	path string,
	caretLine int,
	candidates []autoImportCandidate,
) (json.RawMessage, bool) {
	for _, candidate := range candidates {
		actions, err := m.autoImportCodeActions(ctx, docID, candidate.diagnostic.Range, "quickfix")
		if err != nil {
			continue
		}
		actions = autoImportActionsForDiagnostic(actions, candidate.diagnostic)
		var found *autoImportAction
		count := 0
		for i := range actions {
			action := &actions[i]
			if action.DisabledReason != "" || !autoImportTitle.MatchString(action.Title) ||
				action.Kind != "" && !strings.HasPrefix(action.Kind, "quickfix") {
				continue
			}
			count++
			found = action
		}
		if count != 1 || autoImportEditTouchesLine(*found, path, caretLine) {
			continue
		}
		return append(json.RawMessage(nil), found.Raw...), true
	}
	return nil, false
}

// findOptimizeImportAction asks for the whole-file source action and skips a
// caret-line edit or an edit that would not change the current text.
func (m *Model) findOptimizeImportAction(
	ctx context.Context,
	docID uint32,
	doc *docbridge.Document,
	text, path string,
	caretLine int,
) (json.RawMessage, bool) {
	if doc == nil {
		return nil, false
	}
	actions, err := m.autoImportCodeActions(ctx, docID, openidea.OutlineRange{
		Start: openidea.OutlinePos{},
		End:   doc.UTF16Pos(len(text)),
	}, "source.organizeImports")
	if err != nil {
		return nil, false
	}
	for i := range actions {
		action := &actions[i]
		if action.DisabledReason != "" ||
			!strings.HasPrefix(action.Kind, "source.organizeImports") ||
			action.Edit == nil {
			continue
		}
		if autoImportEditTouchesLine(*action, path, caretLine) ||
			autoImportEditIsNoop(*action, path, text) {
			return nil, false
		}
		return append(json.RawMessage(nil), action.Raw...), true
	}
	return nil, false
}

func (m *Model) autoImportCodeActions(
	ctx context.Context,
	docID uint32,
	actionRange openidea.OutlineRange,
	only string,
) ([]autoImportAction, error) {
	return m.autoImportCodeActionsRequest(ctx, m.nextRequest(), docID, actionRange, only)
}

func (m *Model) autoImportCodeActionsRequest(
	ctx context.Context,
	request uint32,
	docID uint32,
	actionRange openidea.OutlineRange,
	only string,
) ([]autoImportAction, error) {
	raw, err := m.invoke(ctx, "lsp_code_action_query", map[string]any{
		"request": request,
		"doc":     docID,
		"range": map[string]any{
			"start": lspPos(actionRange.Start),
			"end":   lspPos(actionRange.End),
		},
		"only": []string{only},
	})
	if err != nil {
		return nil, err
	}
	var query openidea.CodeActionQuery
	if err := json.Unmarshal(raw, &query); err != nil {
		return nil, err
	}
	actions := make([]autoImportAction, 0, len(query.Actions))
	for _, rawAction := range query.Actions {
		var action autoImportAction
		if json.Unmarshal(rawAction, &action) != nil ||
			strings.TrimSpace(action.Title) == "" {
			continue
		}
		action.Raw = append(json.RawMessage(nil), rawAction...)
		actions = append(actions, action)
	}
	return actions, nil
}

// autoImportActionsForDiagnostic keeps tagged actions only when they name the
// problem being fixed. Servers that omit diagnostic tags keep range matches.
func autoImportActionsForDiagnostic(
	actions []autoImportAction,
	problem openidea.Diagnostic,
) []autoImportAction {
	tagged := false
	for _, action := range actions {
		if len(action.Diagnostics) > 0 {
			tagged = true
			break
		}
	}
	if !tagged {
		return actions
	}
	out := actions[:0]
	for _, action := range actions {
		for _, diagnostic := range action.Diagnostics {
			if autoImportDiagnosticMatches(diagnostic, problem) {
				out = append(out, action)
				break
			}
		}
	}
	return out
}

func autoImportDiagnosticMatches(left, right openidea.Diagnostic) bool {
	if left.Message != right.Message || left.Range != right.Range {
		return false
	}
	if left.Code != nil && right.Code != nil && *left.Code != *right.Code {
		return false
	}
	if left.Source != nil && right.Source != nil && *left.Source != *right.Source {
		return false
	}
	return true
}

func autoImportEditTouchesLine(action autoImportAction, path string, line int) bool {
	if action.Edit == nil {
		return false
	}
	for _, file := range action.Edit.Files {
		if file.Kind != "edit" || realPath(file.Path) != realPath(path) {
			continue
		}
		for _, edit := range file.Edits {
			if int(edit.Range.Start.Line) <= line && line <= int(edit.Range.End.Line) {
				return true
			}
		}
	}
	return false
}

func autoImportEditIsNoop(action autoImportAction, path, text string) bool {
	if action.Edit == nil {
		return true
	}
	for _, file := range action.Edit.Files {
		if file.Kind != "edit" || realPath(file.Path) != realPath(path) {
			continue
		}
		for _, edit := range file.Edits {
			start, startOK := autoImportRangeStart(text, edit.Range.Start)
			end, endOK := autoImportRangeStart(text, edit.Range.End)
			if !startOK || !endOK || start > end || text[start:end] != edit.NewText {
				return false
			}
		}
	}
	return true
}

// autoImportRangeStart maps a line and UTF-16 column to a byte offset in the
// snapshot. The name keeps its shared return type for both range ends.
func autoImportRangeStart(text string, pos openidea.OutlinePos) (int, bool) {
	lineStart := 0
	for line := uint32(0); line < pos.Line; line++ {
		next := strings.IndexByte(text[lineStart:], '\n')
		if next < 0 {
			return 0, false
		}
		lineStart += next + 1
	}
	lineEnd := strings.IndexByte(text[lineStart:], '\n')
	if lineEnd < 0 {
		lineEnd = len(text)
	} else {
		lineEnd += lineStart
	}
	line := text[lineStart:lineEnd]
	if pos.Col > utf16Len(line) {
		return 0, false
	}
	return lineStart + utf16Offset(line, int(pos.Col)), true
}

type autoImportCandidate struct {
	key        string
	diagnostic openidea.Diagnostic
}

func (m *Model) autoImportErrors(t *Tab, caretLine int) []openidea.Diagnostic {
	var out []openidea.Diagnostic
	for _, diagnostic := range m.autoImportDiagnostics(t) {
		if !strings.EqualFold(strings.TrimSpace(diagnostic.Severity), "error") {
			continue
		}
		startLine := int(diagnostic.Range.Start.Line)
		endLine := int(diagnostic.Range.End.Line)
		if startLine <= caretLine && caretLine <= endLine {
			continue
		}
		out = append(out, diagnostic)
	}
	return out
}

func (m *Model) autoImportDiagnostics(t *Tab) []openidea.Diagnostic {
	if t == nil || t.doc == nil {
		return nil
	}
	var out []openidea.Diagnostic
	for _, items := range m.diags[realPath(t.doc.Path())] {
		out = append(out, items...)
	}
	return out
}

func autoImportHasErrors(items []openidea.Diagnostic) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Severity), "error") {
			return true
		}
	}
	return false
}

func autoImportDiagnosticKey(diagnostic openidea.Diagnostic) string {
	return strings.Join([]string{
		strconv.FormatUint(uint64(diagnostic.Range.Start.Line), 10),
		strconv.FormatUint(uint64(diagnostic.Range.Start.Col), 10),
		diagnostic.Message,
	}, ":")
}

func lineForOffset(text string, offset int) int {
	if offset < 0 {
		return 0
	}
	offset = min(offset, len(text))
	return strings.Count(text[:offset], "\n")
}

func autoImportDocumentStable(doc *docbridge.Document, text string, version uint64) bool {
	return doc != nil && doc.Text() == text && doc.Buffer().Version() == version
}
