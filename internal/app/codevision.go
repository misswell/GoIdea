package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

const (
	codeVisionDelay      = 900 * time.Millisecond
	codeVisionMargin     = 10
	codeVisionMaxTargets = 40
	codeVisionWorkers    = 4
	codeVisionTimeout    = 45 * time.Second
)

type codeVisionTarget struct {
	name       string
	position   openidea.OutlinePos
	start, end int
}

// refreshCodeVision adds OpenIDEA's Code Vision content to the editor's inlay
// layer: a declaration's usage count and its latest committed author. It keeps
// requests bounded to the declarations around the viewport, as upstream does.
func (m *Model) refreshCodeVision(t *Tab, now time.Time, id uint32, first, last int, version uint64) {
	if t == nil || t.doc == nil || t.state == nil || t.outlineDoc != id || t.outlineVersion != version {
		return
	}
	s := &t.inlay
	prefs := m.prefs.Inlay
	if !prefs.CodeVisionUsages() && !prefs.CodeVisionAuthor() {
		if len(s.vision) != 0 || s.visionLoading {
			s.vision = nil
			s.visionCount = 0
			s.visionLoading = false
			s.visionLoaded = true
			applyEditorInlays(t)
		}
		return
	}
	if s.visionDocID != id || s.visionVersion != version {
		s.visionCounts = nil
		s.visionIncomplete = nil
		s.visionBlame = nil
		s.visionBlameLoaded = false
		s.visionLoading = false
		s.visionLoaded = false
		s.visionRetry = time.Time{}
	}
	if s.visionDocID != id || s.visionVersion != version ||
		s.visionFirst != first || s.visionLast != last {
		s.visionDocID, s.visionVersion = id, version
		s.visionFirst, s.visionLast = first, last
		s.visionChanged = now
		s.visionLoaded = false
		s.visionRetry = time.Time{}
	}
	if s.visionLoading || s.visionLoaded || now.Before(s.visionRetry) ||
		now.Sub(s.visionChanged) < codeVisionDelay || t.syncing || t.synced != version {
		return
	}
	targets := visibleCodeVisionTargets(t.outline, first, last)
	if len(targets) == 0 {
		s.visionLoading = false
		s.visionLoaded = true
		s.visionCount = 0
		s.vision = nil
		applyEditorInlays(t)
		return
	}

	path := t.doc.Path()
	text := t.doc.Text()
	datePrefs := m.prefs.dateFormatPrefs()
	region := m.systemRegion
	generation := s.visionGen
	s.visionLoading = true
	m.invalidate()

	type cachedResult struct {
		count      *int
		incomplete int
		request    bool
	}
	results := make([]cachedResult, len(targets))
	for index, target := range targets {
		key := codeVisionTargetKey(target)
		if count, ok := s.visionCounts[key]; ok {
			results[index].count = &count
		}
		results[index].incomplete = s.visionIncomplete[key]
		results[index].request = prefs.CodeVisionUsages() &&
			results[index].count == nil && results[index].incomplete <= 3
	}
	blame := append([]openidea.BlameLine(nil), s.visionBlame...)
	blameLoaded := s.visionBlameLoaded
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, codeVisionTimeout)
		defer cancel()
		var blameCh chan []openidea.BlameLine
		if prefs.CodeVisionAuthor() && !blameLoaded {
			blameCh = make(chan []openidea.BlameLine, 1)
			go func() {
				raw, err := m.invoke(ctx, "vcs_blame", map[string]any{
					"path": path, "revision": nil, "text": text,
				})
				if err != nil {
					blameCh <- nil
					return
				}
				var lines []openidea.BlameLine
				if json.Unmarshal(raw, &lines) != nil {
					lines = nil
				}
				blameCh <- lines
			}()
		}

		jobs := make(chan int)
		var workers sync.WaitGroup
		for range codeVisionWorkers {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for index := range jobs {
					if !results[index].request {
						continue
					}
					target := targets[index]
					request := m.nextChannel()
					partial := m.nextChannel()
					raw, err := m.invoke(ctx, "lsp_references", map[string]any{
						"request":            request,
						"doc":                id,
						"pos":                lspPos(target.position),
						"includeDeclaration": false,
						"projectWide":        false,
						"libraries":          false,
						"onPartial":          map[string]any{"$channel": partial},
					})
					if err != nil {
						continue
					}
					var refs openidea.References
					if json.Unmarshal(raw, &refs) != nil {
						continue
					}
					if refs.Incomplete != nil {
						results[index].incomplete++
						continue
					}
					count := len(refs.Locations)
					results[index].count = &count
				}
			}()
		}
	enqueue:
		for index := range targets {
			if !results[index].request {
				continue
			}
			select {
			case jobs <- index:
			case <-ctx.Done():
				break enqueue
			}
		}
		close(jobs)
		workers.Wait()
		newBlame := blame
		if blameCh != nil {
			if lines := <-blameCh; lines != nil {
				newBlame = lines
			}
		}

		m.post(func() {
			s.visionLoading = false
			if _, open := m.groupOf(t); !open || t.doc.ID() != id {
				return
			}
			if s.visionGen != generation || s.visionDocID != id || s.visionVersion != version ||
				t.doc.Buffer().Version() != version {
				return
			}
			if s.visionCounts == nil {
				s.visionCounts = make(map[string]int)
			}
			if s.visionIncomplete == nil {
				s.visionIncomplete = make(map[string]int)
			}
			retry := false
			for index, target := range targets {
				key := codeVisionTargetKey(target)
				if results[index].count != nil {
					s.visionCounts[key] = *results[index].count
				}
				if results[index].incomplete != 0 {
					s.visionIncomplete[key] = results[index].incomplete
					if results[index].incomplete <= 3 {
						retry = true
					}
				}
			}
			if blameCh != nil && newBlame != nil {
				s.visionBlame = newBlame
				s.visionBlameLoaded = true
			}
			if s.visionFirst != first || s.visionLast != last {
				s.visionLoaded = false
				if retry {
					s.visionRetry = time.Now().Add(300 * time.Millisecond)
				}
				m.invalidate()
				return
			}
			s.vision = codeVisionInlays(t, targets, s.visionCounts, s.visionBlame, prefs, datePrefs, region)
			s.visionLoaded = true
			s.visionCount = len(s.vision)
			if retry {
				s.visionLoaded = false
				s.visionRetry = time.Now().Add(300 * time.Millisecond)
			}
			applyEditorInlays(t)
			m.invalidate()
		})
	}()
}

func codeVisionTargetKey(target codeVisionTarget) string {
	return fmt.Sprintf("%d:%d:%s", target.position.Line, target.position.Col, target.name)
}

func codeVisionInlays(t *Tab, targets []codeVisionTarget, counts map[string]int, blame []openidea.BlameLine, prefs InlayPrefs, datePrefs DateFormatPrefs, region dateRegion) []editor.Inlay {
	if t == nil || t.doc == nil {
		return nil
	}
	snapshot := t.doc.Buffer().Snapshot()
	out := make([]editor.Inlay, 0, len(targets)*2)
	for _, target := range targets {
		author := ""
		if prefs.CodeVisionAuthor() {
			author = codeVisionAuthor(blame, target.start, target.end, datePrefs, region)
		}
		usage := ""
		if count, ok := counts[codeVisionTargetKey(target)]; prefs.CodeVisionUsages() && ok &&
			(count > 0 || prefs.CodeVisionNoUsages()) {
			usage = codeVisionUsage(count)
		}
		if author == "" && usage == "" {
			continue
		}
		declaration := t.doc.ByteOffset(target.position)
		lineEnd := snapshot.LineEnd(int(target.position.Line))
		authorOffset, usageOffset := declaration, declaration
		if prefs.AuthorAtRight() {
			authorOffset = lineEnd
		}
		if prefs.UsagesAtRight() {
			usageOffset = lineEnd
		}
		switch {
		case author != "" && usage != "" && authorOffset == usageOffset:
			out = append(out, codeVisionInlay(authorOffset, []string{author, usage}, authorOffset != declaration))
		default:
			if author != "" {
				out = append(out, codeVisionInlay(authorOffset, []string{author}, authorOffset != declaration))
			}
			if usage != "" {
				out = append(out, codeVisionInlay(usageOffset, []string{usage}, usageOffset != declaration))
			}
		}
	}
	return out
}

func codeVisionInlay(offset int, parts []string, atRight bool) editor.Inlay {
	text := strings.Join(parts, " · ") + "  "
	if atRight {
		text = "  " + text
	}
	return editor.Inlay{Offset: offset, Text: text}
}

// invalidateCodeVision drops visible results after a settings or VCS change.
// Per-document counts stay cached until the document version changes.
func (m *Model) invalidateCodeVision() {
	for _, t := range m.allTabs() {
		if t == nil || t.state == nil {
			continue
		}
		s := &t.inlay
		s.visionGen++
		s.visionLoading = false
		s.visionLoaded = false
		s.visionChanged = time.Time{}
		s.visionRetry = time.Time{}
		s.vision = nil
		s.visionCount = 0
		applyEditorInlays(t)
	}
}

func visibleCodeVisionTargets(items []openidea.OutlineItem, first, last int) []codeVisionTarget {
	allowed := map[string]bool{
		"class": true, "interface": true, "struct": true, "enum": true,
		"trait": true, "function": true, "method": true, "constructor": true,
		"typeAlias": true, "macro": true,
	}
	from := max(0, first-codeVisionMargin)
	to := last + codeVisionMargin
	seen := map[int]bool{}
	var out []codeVisionTarget
	var walk func([]openidea.OutlineItem)
	walk = func(nodes []openidea.OutlineItem) {
		for _, item := range nodes {
			line := int(item.SelectionRange.Start.Line)
			if allowed[item.Kind] && line >= from && line <= to &&
				!seen[line] && !codeVisionAnonymous(item.Name) {
				seen[line] = true
				out = append(out, codeVisionTarget{
					name:     item.Name,
					position: item.SelectionRange.Start,
					start:    int(item.Range.Start.Line),
					end:      int(item.Range.End.Line),
				})
			}
			walk(item.Children)
		}
	}
	walk(items)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].position.Line != out[j].position.Line {
			return out[i].position.Line < out[j].position.Line
		}
		return out[i].position.Col < out[j].position.Col
	})
	if len(out) > codeVisionMaxTargets {
		out = out[:codeVisionMaxTargets]
	}
	return out
}

func codeVisionAnonymous(name string) bool {
	return (strings.HasPrefix(name, "<") && strings.HasSuffix(name, ">")) ||
		strings.HasSuffix(name, " callback")
}

func codeVisionUsage(count int) string {
	if count == 0 {
		return "no usages"
	}
	if count == 1 {
		return "1 usage"
	}
	return fmt.Sprintf("%d usages", count)
}

func codeVisionAuthor(lines []openidea.BlameLine, start, end int, datePrefs DateFormatPrefs, region dateRegion) string {
	if len(lines) == 0 {
		return ""
	}
	latest := map[string]float64{}
	for line := max(0, start); line <= end && line < len(lines); line++ {
		blame := lines[line]
		if blame.Author == "" || strings.Trim(blame.Commit, "0") == "" {
			continue
		}
		if latest[blame.Author] < blame.TimeMs {
			latest[blame.Author] = blame.TimeMs
		}
	}
	if len(latest) == 0 {
		return ""
	}
	authors := make([]string, 0, len(latest))
	for author := range latest {
		authors = append(authors, author)
	}
	sort.Slice(authors, func(i, j int) bool {
		if latest[authors[i]] != latest[authors[j]] {
			return latest[authors[i]] > latest[authors[j]]
		}
		return authors[i] < authors[j]
	})
	label := authors[0]
	if len(authors) > 1 {
		label += fmt.Sprintf(" +%d", len(authors)-1)
	}
	if ms := latest[authors[0]]; ms > 0 {
		label += " " + formatAppDate(datePrefs, region, ms, true)
	}
	return label
}
