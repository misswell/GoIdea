package app

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

// inlayState keeps one request in flight for a visible editor.
type inlayState struct {
	docID       uint32
	version     uint64
	first, last int
	changed     time.Time
	retry       time.Time
	loading     bool
	loaded      bool
	count       int
	hints       []editor.Inlay
	// debug holds values for the selected paused frame.
	debug []editor.Inlay

	visionDocID   uint32
	visionVersion uint64
	visionGen     uint64
	visionFirst   int
	visionLast    int
	visionChanged time.Time
	visionRetry   time.Time
	visionLoading bool
	visionLoaded  bool
	visionCount   int
	vision        []editor.Inlay
	// visionCounts and visionBlame stay valid for one document version, so
	// scrolling asks only for declarations that entered the viewport.
	visionCounts      map[string]int
	visionIncomplete  map[string]int
	visionBlame       []openidea.BlameLine
	visionBlameLoaded bool

	inlineBlameDocID      uint32
	inlineBlameVersion    uint64
	inlineBlameLine       int
	inlineBlameChanged    time.Time
	inlineBlameRetry      time.Time
	inlineBlameLoading    bool
	inlineBlameLoaded     bool
	inlineBlameLines      []openidea.BlameLine
	inlineBlameInlay      editor.Inlay
	inlineBlameHasLine    bool
	inlineBlameGeneration uint64
}

type inlayHint struct {
	Position     openidea.OutlinePos `json:"position"`
	Label        string              `json:"label"`
	PaddingLeft  bool                `json:"paddingLeft"`
	PaddingRight bool                `json:"paddingRight"`
}

// refreshInlays waits for the buffer and viewport to settle before asking
// OpenIDEA for hints. MyGo moves old hints with edits until the reply arrives.
func (m *Model) refreshInlays(t *Tab, now time.Time) {
	if t == nil || t.doc == nil || t.state == nil || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	id := t.doc.ID()
	if id == 0 || t.doc.Info().Large {
		return
	}
	buffer := t.doc.Buffer()
	first, last := t.state.VisibleLines()
	if last < first || first < 0 {
		return
	}
	last = min(last, buffer.LineCount()-1)
	version := buffer.Version()
	s := &t.inlay
	if s.docID != id || s.version != version || s.first != first || s.last != last {
		s.docID, s.version, s.first, s.last = id, version, first, last
		s.changed = now
		s.loaded = false
		s.retry = time.Time{}
	}
	m.refreshCodeVision(t, now, id, first, last, version)
	m.refreshInlineBlame(t, now, id, version)
	if s.loading || s.loaded || now.Before(s.retry) || now.Sub(s.changed) < 400*time.Millisecond || t.syncing || t.synced != version {
		return
	}
	snapshot := buffer.Snapshot()
	start := t.doc.UTF16Pos(snapshot.LineStart(first))
	end := t.doc.UTF16Pos(snapshot.LineEnd(last))
	s.loading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "lsp_inlay_hints", map[string]any{
			"doc": id, "range": map[string]any{"start": lspPos(start), "end": lspPos(end)},
		})
		var hints []inlayHint
		if err == nil {
			err = json.Unmarshal(raw, &hints)
		}
		m.post(func() {
			s.loading = false
			if _, open := m.groupOf(t); !open || t.doc.ID() != id {
				return
			}
			if s.docID != id || s.version != version || s.first != first || s.last != last || buffer.Version() != version {
				return
			}
			if err != nil {
				s.retry = time.Now().Add(lspBackoff)
				return
			}
			s.hints = editorInlays(t.doc, hints)
			s.loaded = true
			s.count = len(s.hints)
			applyEditorInlays(t)
			m.invalidate()
		})
	}()
}

func editorInlays(doc *docbridge.Document, hints []inlayHint) []editor.Inlay {
	out := make([]editor.Inlay, 0, len(hints))
	for _, hint := range hints {
		if hint.Label == "" {
			continue
		}
		label := hint.Label
		if hint.PaddingLeft {
			label = " " + label
		}
		if hint.PaddingRight {
			label += " "
		}
		out = append(out, editor.Inlay{Offset: doc.ByteOffset(hint.Position), Text: label})
	}
	return out
}

func applyEditorInlays(t *Tab) {
	if t == nil || t.state == nil {
		return
	}
	t.state.SetInlays(mergedEditorInlays(&t.inlay))
}

func mergedEditorInlays(s *inlayState) []editor.Inlay {
	if s == nil {
		return nil
	}
	out := make([]editor.Inlay, 0, len(s.hints)+len(s.vision)+len(s.debug)+1)
	out = append(out, s.hints...)
	out = append(out, s.vision...)
	out = append(out, s.debug...)
	if s.inlineBlameHasLine {
		out = append(out, s.inlineBlameInlay)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Offset < out[j].Offset
	})
	return out
}
