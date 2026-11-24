package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const inlineBlameDelay = 300 * time.Millisecond

func (m *Model) refreshInlineBlame(t *Tab, now time.Time, id uint32, version uint64) {
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	s := &t.inlay
	enabled := m.nativeRuntimeBool(settingsPageVCSGit, "vcs.git.inlineBlame") &&
		t.kind == "" && !t.doc.ReadOnly() && !t.doc.Info().Large &&
		nearestGitRoot(t.doc.Path(), m.Root()) != ""
	if enabled {
		status, ok := m.statusFor(t.doc.Path())
		if !ok {
			enabled = false
		} else {
			kind := effectiveVCSKind(status)
			enabled = kind != "untracked" && kind != "ignored"
		}
	}
	if !enabled {
		if s.inlineBlameHasLine || s.inlineBlameLoading || s.inlineBlameLoaded {
			s.inlineBlameLoading = false
			s.inlineBlameLoaded = false
			s.inlineBlameHasLine = false
			s.inlineBlameInlay = editor.Inlay{}
			s.inlineBlameLines = nil
			s.inlineBlameGeneration++
			applyEditorInlays(t)
		}
		return
	}
	line := t.state.CursorPos().Line
	if s.inlineBlameDocID != id || s.inlineBlameVersion != version {
		s.inlineBlameDocID, s.inlineBlameVersion = id, version
		s.inlineBlameLine = line
		s.inlineBlameChanged = now
		s.inlineBlameRetry = time.Time{}
		s.inlineBlameLoading = false
		s.inlineBlameLoaded = false
		s.inlineBlameLines = nil
		s.inlineBlameHasLine = false
		s.inlineBlameInlay = editor.Inlay{}
		s.inlineBlameGeneration++
		applyEditorInlays(t)
	} else if s.inlineBlameLine != line {
		s.inlineBlameLine = line
		s.inlineBlameChanged = now
		s.inlineBlameRetry = time.Time{}
		if s.inlineBlameLoaded {
			m.showInlineBlameLine(t, now)
		}
	}
	if s.inlineBlameLoaded || s.inlineBlameLoading || now.Before(s.inlineBlameRetry) ||
		now.Sub(s.inlineBlameChanged) < inlineBlameDelay || t.syncing || t.synced != version {
		return
	}
	path := t.doc.Path()
	text := t.doc.Text()
	generation := s.inlineBlameGeneration
	s.inlineBlameLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_blame", map[string]any{
			"path": path, "revision": nil, "text": text,
		})
		var lines []openidea.BlameLine
		if err == nil {
			err = json.Unmarshal(raw, &lines)
		}
		m.post(func() {
			s.inlineBlameLoading = false
			if _, open := m.groupOf(t); !open || t.doc.ID() != id {
				return
			}
			if s.inlineBlameGeneration != generation ||
				s.inlineBlameDocID != id || s.inlineBlameVersion != version ||
				t.doc.Buffer().Version() != version {
				return
			}
			if err != nil {
				s.inlineBlameRetry = time.Now().Add(lspBackoff)
				return
			}
			s.inlineBlameLines = lines
			s.inlineBlameLoaded = true
			m.showInlineBlameLine(t, time.Now())
		})
	}()
}

func (m *Model) showInlineBlameLine(t *Tab, now time.Time) {
	if t == nil || t.doc == nil || t.state == nil {
		return
	}
	s := &t.inlay
	line := t.state.CursorPos().Line
	if line < 0 || line >= len(s.inlineBlameLines) {
		s.inlineBlameHasLine = false
		s.inlineBlameInlay = editor.Inlay{}
		applyEditorInlays(t)
		return
	}
	blame := s.inlineBlameLines[line]
	text := inlineBlameText(blame, time.UnixMilli(int64(now.UnixMilli())))
	if text == "" {
		s.inlineBlameHasLine = false
		s.inlineBlameInlay = editor.Inlay{}
		applyEditorInlays(t)
		return
	}
	offset := t.doc.Buffer().Snapshot().LineEnd(line)
	s.inlineBlameInlay = editor.Inlay{Offset: offset, Text: "    " + text}
	s.inlineBlameHasLine = true
	applyEditorInlays(t)
	m.invalidate()
}

func inlineBlameText(line openidea.BlameLine, now time.Time) string {
	if annotateUncommitted(line.Commit) {
		return "You, now · Uncommitted changes"
	}
	author := strings.TrimSpace(line.Author)
	summary := strings.Join(strings.Fields(line.Summary), " ")
	date := relativeBlameDate(line.TimeMs, now)
	switch {
	case author != "" && date != "":
		author += ", " + date
	case author == "":
		author = date
	}
	switch {
	case author != "" && summary != "":
		return author + " · " + summary
	case author != "":
		return author
	default:
		return summary
	}
}

func relativeBlameDate(ms float64, now time.Time) string {
	if ms <= 0 {
		return ""
	}
	seconds := math.Max(0, now.Sub(time.UnixMilli(int64(ms))).Seconds())
	units := []struct {
		name string
		size float64
	}{
		{"year", 365 * 86400},
		{"month", 30 * 86400},
		{"day", 86400},
		{"hour", 3600},
		{"minute", 60},
	}
	for _, unit := range units {
		if seconds < unit.size {
			continue
		}
		n := int64(seconds / unit.size)
		if n == 1 {
			return "1 " + unit.name + " ago"
		}
		return fmt.Sprintf("%d %ss ago", n, unit.name)
	}
	return "now"
}
