package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// Gutter colours follow IDEA: green additions, blue modifications, red
// deletions.
var (
	gutterAddedColor    = ui.RGB(80, 170, 110)
	gutterModifiedColor = ui.RGB(80, 140, 220)
	gutterDeletedColor  = ui.RGB(220, 90, 90)
)

// kickCodeIntel refreshes the active tab's change marks and outline once typing
// has settled. It runs from the frame pump; the calls go to other goroutines.
func (m *Model) kickCodeIntel(t *Tab, now time.Time) {
	if t != m.tab() {
		return
	}
	idle := now.Sub(t.edited)
	v := t.doc.Buffer().Version()
	if idle >= 220*time.Millisecond && (t.gitPath != t.doc.Path() || t.gitVersion != v) {
		m.kickGutter(t, v)
	}
	if idle >= 600*time.Millisecond && (t.outlineDoc != t.doc.ID() || t.outlineVersion != v) {
		m.kickOutline(t, v)
	}
}

// kickGutter diffs the buffer against HEAD through the backend. A project
// without Git (or a file HEAD does not have) leaves the gutter empty instead of
// repeating the refusal on every edit.
func (m *Model) kickGutter(t *Tab, v uint64) {
	if t.gitLoading || t.gitOff || t.doc.Info().Large {
		return
	}
	path := t.doc.Path()
	text := t.doc.Text()
	t.gitLoading = true
	t.gitPath = path
	t.gitVersion = v
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		head, err := m.headText(ctx, path)
		var marks []editor.GutterMark
		if err == nil && head != "" && head != text {
			marks, err = m.diffMarks(ctx, head, text)
		}
		m.post(func() {
			t.gitLoading = false
			if t.doc.Path() != path {
				return
			}
			if err != nil {
				t.gitOff = true
				t.gitMarks = nil
			} else {
				t.gitMarks = marks
			}
			m.refreshDecorations()
			m.invalidate()
		})
	}()
}

// headText is the committed text of a file; empty when HEAD does not have it.
func (m *Model) headText(ctx context.Context, path string) (string, error) {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_file_at", map[string]any{"path": path, "rev": "HEAD"})
	if err != nil {
		return "", err
	}
	var text *string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", err
	}
	if text == nil {
		return "", nil
	}
	return *text, nil
}

// diffMarks asks the backend for line hunks and turns them into gutter marks.
func (m *Model) diffMarks(ctx context.Context, oldText, newText string) ([]editor.GutterMark, error) {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_diff_hunks", map[string]any{
		"oldText": oldText, "newText": newText,
	})
	if err != nil {
		return nil, err
	}
	var hunks []openidea.LineHunk
	if err := json.Unmarshal(raw, &hunks); err != nil {
		return nil, err
	}
	return hunkMarks(hunks), nil
}

// hunkMarks turns hunks into gutter lines: additions and modifications cover
// every new line, a deletion sits on the line that took the removed text.
func hunkMarks(hunks []openidea.LineHunk) []editor.GutterMark {
	var marks []editor.GutterMark
	for _, h := range hunks {
		color := gutterModifiedColor
		switch h.Kind {
		case "added":
			color = gutterAddedColor
		case "deleted":
			marks = append(marks, editor.GutterMark{Line: int(h.Start), Color: gutterDeletedColor})
			continue
		}
		for i := uint32(0); i < h.Lines; i++ {
			marks = append(marks, editor.GutterMark{Line: int(h.Start) + int(i), Color: color})
		}
	}
	return marks
}

// kickOutline loads the document's symbols, which feed the breadcrumbs.
func (m *Model) kickOutline(t *Tab, v uint64) {
	if t.outlineLoading {
		return
	}
	doc := t.doc.ID()
	t.outlineLoading = true
	t.outlineDoc = doc
	t.outlineVersion = v
	t.outlineGen++
	gen := t.outlineGen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "doc_outline", map[string]any{"doc": doc})
		var items []openidea.OutlineItem
		if err == nil {
			err = json.Unmarshal(raw, &items)
		}
		m.post(func() {
			t.outlineLoading = false
			if t.outlineGen != gen || t.doc.ID() != doc {
				return
			}
			if err != nil {
				t.outline = nil
			} else {
				t.outline = items
			}
			m.invalidate()
		})
	}()
}

// outlinePath is the chain of symbols containing line, outermost first.
func outlinePath(items []openidea.OutlineItem, line int) []openidea.OutlineItem {
	for _, it := range items {
		if line < int(it.Range.Start.Line) || line > int(it.Range.End.Line) {
			continue
		}
		return append([]openidea.OutlineItem{it}, outlinePath(it.Children, line)...)
	}
	return nil
}

// SmokeCodeIntel reports the change marks and outline of an opened file: the
// offscreen check of the gutter and breadcrumbs.
func (m *Model) SmokeCodeIntel(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	if t := m.tab(); t == nil {
		candidate := findSmokeFile(root)
		if candidate == "" {
			return out, errors.New("项目里没有可打开的源码文件")
		}
		m.post(func() { m.openFile(candidate) })
		if !m.waitDebug(ctx, func(*DebugPane) bool { return m.tab() != nil }, 60*time.Second) {
			return out, errors.New("打开 " + candidate + " 超时")
		}
	}
	var path string
	var doc uint32
	m.post(func() {
		if t := m.tab(); t != nil {
			path, doc = t.doc.Path(), t.doc.ID()
		}
	})
	out["path"] = path

	head, err := m.headText(ctx, path)
	if err != nil {
		out["headError"] = err.Error()
	} else {
		out["headBytes"] = len(head)
		var marks []editor.GutterMark
		if head != "" {
			marks, err = m.diffMarks(ctx, head, m.tabText())
			if err != nil {
				out["diffError"] = err.Error()
			}
		}
		out["gutterMarks"] = len(marks)
	}

	deadline := time.Now().Add(60 * time.Second)
	var items []openidea.OutlineItem
	for {
		raw, err := m.mgr.Invoke(ctx, Window, "doc_outline", map[string]any{"doc": doc})
		if err == nil {
			if err = json.Unmarshal(raw, &items); err != nil {
				return out, err
			}
		}
		if len(items) > 0 || time.Now().After(deadline) {
			if err != nil {
				out["outlineError"] = err.Error()
			}
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	out["outlineItems"] = len(items)
	if len(items) > 0 {
		out["outlineFirst"] = items[0].Name
	}
	if chain := outlinePath(items, 0); len(chain) > 0 {
		names := make([]string, len(chain))
		for i, it := range chain {
			names[i] = it.Name
		}
		out["breadcrumb"] = strings.Join(names, " › ")
	}
	return out, nil
}

// tabText is the active buffer's text, for the smoke.
func (m *Model) tabText() string {
	var text string
	m.post(func() {
		if t := m.tab(); t != nil {
			text = t.doc.Text()
		}
	})
	return text
}

// findSmokeFile picks a source file to open when the smoke starts with no tab:
// a Java main class first, then any Java or Go file outside build outputs.
func findSmokeFile(root string) string {
	if main := findJavaMain(root); main != "" {
		return main
	}
	var first string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root {
				switch d.Name() {
				case ".git", ".idea", "target", "build", "out", "node_modules":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(p, ".java") || strings.HasSuffix(p, ".go") {
			first = p
			return filepath.SkipAll
		}
		return nil
	})
	return first
}
