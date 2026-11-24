package app

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type usageHighlight struct {
	Start int
	End   int
	Kind  string
}

// ToggleHighlightUsages implements find.highlightUsages. The first invocation
// asks the language server for the usages at the caret and keeps them tinted.
// Pressing it again on a highlighted usage, or Escape, clears them.
func (m *Model) ToggleHighlightUsages() {
	t := m.tab()
	if t == nil || t.state == nil || t.doc == nil || !m.tabOpen(t) {
		return
	}
	caret := t.state.Cursor()
	if t.usageHighlightSticky && usageHighlightAt(t.usageHighlights, caret) >= 0 {
		m.clearUsageHighlights(t)
		m.invalidate()
		return
	}
	if m.ctx == nil || m.mgr == nil && m.invokeHook == nil {
		m.status = "Highlighting usages requires the GoIDEA backend"
		m.invalidate()
		return
	}

	t.usageHighlightGen++
	gen := t.usageHighlightGen
	t.usageHighlightBusy = true
	version := t.state.Buffer.Version()
	docID := t.doc.ID()
	pos := t.doc.UTF16Pos(caret)
	m.status = "Highlighting usages…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		if err := t.doc.Sync(ctx); err != nil {
			m.finishUsageHighlights(t, gen, docID, version, caret, nil, err)
			return
		}
		raw, err := m.invoke(ctx, "lsp_document_highlights", map[string]any{
			"doc": docID, "pos": lspPos(pos),
		})
		var highlights []openidea.Highlight
		if err == nil {
			err = json.Unmarshal(raw, &highlights)
		}
		m.finishUsageHighlights(t, gen, docID, version, caret, highlights, err)
	}()
}

func (m *Model) finishUsageHighlights(
	t *Tab,
	gen uint64,
	docID uint32,
	version uint64,
	caret int,
	highlights []openidea.Highlight,
	err error,
) {
	m.post(func() {
		if !m.tabOpen(t) || t.doc == nil || t.doc.ID() != docID || t.usageHighlightGen != gen {
			return
		}
		t.usageHighlightBusy = false
		if t.state.Buffer.Version() != version || t.state.Cursor() != caret {
			m.invalidate()
			return
		}
		if err != nil {
			t.usageHighlights = nil
			t.usageHighlightSticky = false
			m.status = "Highlighting usages failed: " + err.Error()
			m.invalidate()
			return
		}
		ranges := make([]usageHighlight, 0, len(highlights))
		for _, highlight := range highlights {
			start, end := t.doc.ByteRange(highlight.Range)
			if end <= start {
				continue
			}
			ranges = append(ranges, usageHighlight{Start: start, End: end, Kind: highlight.Kind})
		}
		sort.SliceStable(ranges, func(i, j int) bool {
			if ranges[i].Start != ranges[j].Start {
				return ranges[i].Start < ranges[j].Start
			}
			return ranges[i].End < ranges[j].End
		})
		t.usageHighlights = ranges
		t.usageHighlightVersion = version
		t.usageHighlightSticky = len(ranges) > 0
		if len(ranges) == 0 {
			m.status = "No usages found in file"
		} else {
			name := t.state.Buffer.Snapshot().Slice(ranges[0].Start, ranges[0].End)
			m.status = usageHighlightsStatus(len(ranges), name)
		}
		m.refreshUsageHighlightBackgrounds(t)
		m.invalidate()
	})
}

func usageHighlightsStatus(n int, name string) string {
	noun := "usages"
	if n == 1 {
		noun = "usage"
	}
	if name == "" {
		name = "the element"
	} else {
		name = "'" + name + "'"
	}
	return count(n, noun, noun) + " of " + name +
		" found (press Cmd+Shift+F7 again to remove, Escape to clear)"
}

// StepHighlightedUsage moves within the sticky highlight session, wrapping in
// the requested direction. Without a session it keeps OpenIDEA's fallback and
// steps through problems.
func (m *Model) StepHighlightedUsage(direction int) {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	if m.usageHighlightSessionValid(t) {
		caret := t.state.Cursor()
		current := usageHighlightAt(t.usageHighlights, caret)
		if current >= 0 && len(t.usageHighlights) > 1 {
			target := t.usageHighlights[(current+direction+len(t.usageHighlights))%len(t.usageHighlights)]
			inside := caret - t.usageHighlights[current].Start
			at := target.Start + min(inside, target.End-target.Start)
			t.state.Select(at, at)
			m.invalidate()
			return
		}
	}
	m.StepProblem(direction)
}

// ClearStickyUsageHighlights removes every visible sticky find.highlightUsages
// session. It reports whether Escape handled an active session.
func (m *Model) ClearStickyUsageHighlights() bool {
	cleared := false
	for _, t := range m.allTabs() {
		if t == nil || !t.usageHighlightSticky {
			continue
		}
		m.clearUsageHighlights(t)
		cleared = true
	}
	if cleared {
		m.invalidate()
	}
	return cleared
}

func (m *Model) clearUsageHighlights(t *Tab) {
	if t == nil {
		return
	}
	t.usageHighlightGen++
	t.usageHighlightBusy = false
	t.usageHighlights = nil
	t.usageHighlightVersion = 0
	t.usageHighlightSticky = false
	m.refreshUsageHighlightBackgrounds(t)
}

func (m *Model) usageHighlightSessionValid(t *Tab) bool {
	return t != nil && t.state != nil && t.state.Buffer != nil &&
		t.usageHighlightSticky &&
		t.usageHighlightVersion == t.state.Buffer.Version() &&
		len(t.usageHighlights) > 0
}

func usageHighlightAt(highlights []usageHighlight, offset int) int {
	for i, highlight := range highlights {
		if highlight.Start <= offset && offset <= highlight.End {
			return i
		}
	}
	return -1
}

func (m *Model) usageHighlightBackgrounds(t *Tab) []editor.TextBackground {
	if t == nil || t.state == nil || t.state.Buffer == nil || !m.usageHighlightSessionValid(t) {
		return nil
	}
	out := make([]editor.TextBackground, 0, len(t.usageHighlights))
	for _, highlight := range t.usageHighlights {
		out = append(out, editor.TextBackground{
			Start: highlight.Start,
			End:   highlight.End,
			Color: m.usageHighlightColor(highlight.Kind),
		})
	}
	return out
}

func (m *Model) usageHighlightColor(kind string) ui.Color {
	oi := oiPaletteOf(m.theme)
	if kind == "write" {
		return oi.warning.Alpha(0.24)
	}
	return oi.accent.Alpha(0.18)
}

func (m *Model) refreshUsageHighlightBackgrounds(t *Tab) {
	if t == nil || t.state == nil || t.doc == nil {
		return
	}
	theme := m.theme
	if theme == nil {
		theme = openideaTheme(false)
	}
	t.state.TextBackgrounds = m.composeTextBackgrounds(t, m.todoMarksOf(t, theme))
}
