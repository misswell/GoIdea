package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type quickFixCandidate struct {
	start int
	end   int
	rank  int
}

// ApplyQuickFix is code.applyQuickFix. It checks the problems on the caret
// line from most severe and closest outwards, asks the language servers for a
// quick fix at each, and applies the preferred enabled action. A tab-local
// generation drops every reply after a newer request, an edit or a caret move.
func (m *Model) ApplyQuickFix() {
	t := m.tab()
	if t == nil || t.state == nil || t.doc == nil || !m.tabOpen(t) ||
		!m.activeEditorEditable() {
		return
	}
	candidates := quickFixCandidates(t)
	if len(candidates) == 0 {
		m.status = "No quick fixes available"
		m.invalidate()
		return
	}
	if m.ctx == nil || m.mgr == nil && m.invokeHook == nil {
		m.status = "Quick fixes require the GoIDEA backend"
		m.invalidate()
		return
	}
	t.quickFixGen++
	m.queryQuickFix(
		t,
		t.quickFixGen,
		candidates,
		0,
		t.state.Buffer.Version(),
		t.state.Cursor(),
	)
}

func quickFixCandidates(t *Tab) []quickFixCandidate {
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return nil
	}
	snapshot := t.state.Buffer.Snapshot()
	caret := t.state.Cursor()
	line := snapshot.LineOf(caret)
	lineStart := snapshot.LineStart(line)
	lineEnd := snapshot.LineEnd(line)
	var out []quickFixCandidate
	for _, diagnostic := range t.state.Diagnostics() {
		if diagnostic.Severity == editor.SeverityHint ||
			diagnostic.End < lineStart || diagnostic.Start > lineEnd {
			continue
		}
		rank := int(diagnostic.Severity)
		if caret < diagnostic.Start || caret > diagnostic.End {
			rank += 3
		}
		out = append(out, quickFixCandidate{
			start: diagnostic.Start,
			end:   diagnostic.End,
			rank:  rank,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		if out[i].start != out[j].start {
			return out[i].start < out[j].start
		}
		return out[i].end < out[j].end
	})
	deduped := out[:0]
	for _, candidate := range out {
		if len(deduped) > 0 {
			last := deduped[len(deduped)-1]
			if last.start == candidate.start && last.end == candidate.end {
				continue
			}
		}
		deduped = append(deduped, candidate)
	}
	return deduped
}

func (m *Model) queryQuickFix(
	t *Tab,
	gen uint64,
	candidates []quickFixCandidate,
	index int,
	version uint64,
	caret int,
) {
	if index >= len(candidates) {
		m.status = "No quick fixes available"
		m.invalidate()
		return
	}
	candidate := candidates[index]
	doc := t.doc
	docID := doc.ID()
	start := doc.UTF16Pos(candidate.start)
	end := doc.UTF16Pos(candidate.end)
	request := m.nextRequest()
	m.status = "Finding quick fixes…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		if err := doc.Sync(ctx); err != nil {
			m.finishQuickFixQuery(
				t, gen, candidates, index, docID, version, caret,
				nil, err,
			)
			return
		}
		raw, err := m.invoke(ctx, "lsp_code_action_query", map[string]any{
			"request": request, "doc": docID,
			"range": map[string]any{"start": lspPos(start), "end": lspPos(end)},
			"only":  []string{"quickfix"},
		})
		var query openidea.CodeActionQuery
		if err == nil {
			err = json.Unmarshal(raw, &query)
		}
		m.finishQuickFixQuery(
			t, gen, candidates, index, docID, version, caret,
			&query, err,
		)
	}()
}

func (m *Model) finishQuickFixQuery(
	t *Tab,
	gen uint64,
	candidates []quickFixCandidate,
	index int,
	docID uint32,
	version uint64,
	caret int,
	query *openidea.CodeActionQuery,
	err error,
) {
	m.post(func() {
		if !m.quickFixRequestCurrent(t, gen, docID, version, caret) {
			return
		}
		if err != nil {
			m.status = "Finding quick fixes failed: " + err.Error()
			m.invalidate()
			return
		}
		if action, ok := chooseQuickFix(query.Actions); ok {
			m.applyQuickFixAction(t, gen, docID, version, caret, action)
			return
		}
		m.queryQuickFix(t, gen, candidates, index+1, version, caret)
	})
}

func chooseQuickFix(raw []json.RawMessage) (intentAction, bool) {
	actions := parseIntentActions(raw)
	var first intentAction
	found := false
	for _, action := range actions {
		if action.disabled != "" ||
			action.kind != "quickfix" && !strings.HasPrefix(action.kind, "quickfix.") {
			continue
		}
		if !found {
			first = action
			found = true
		}
		if action.preferred {
			return action, true
		}
	}
	return first, found
}

func (m *Model) applyQuickFixAction(
	t *Tab,
	gen uint64,
	docID uint32,
	version uint64,
	caret int,
	action intentAction,
) {
	m.status = "Applying quick fix…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, intentTimeout)
		defer cancel()
		_, err := m.invoke(ctx, "lsp_apply_code_action", map[string]any{
			"doc": docID, "action": action.raw,
		})
		m.post(func() {
			if !m.quickFixRequestCurrent(t, gen, docID, version, caret) {
				return
			}
			if err != nil {
				m.status = "Applying quick fix failed: " + err.Error()
			} else {
				m.status = "Applied quick fix: " + action.title
			}
			m.invalidate()
		})
	}()
}

func (m *Model) quickFixRequestCurrent(
	t *Tab,
	gen uint64,
	docID uint32,
	version uint64,
	caret int,
) bool {
	return t != nil && t.doc != nil && t.state != nil && t.state.Buffer != nil &&
		m.tabOpen(t) && t.quickFixGen == gen && t.doc.ID() == docID &&
		t.state.Buffer.Version() == version && t.state.Cursor() == caret
}
