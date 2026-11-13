package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Bookmark is a line the user marked to come back to, as OpenIDEA's line
// bookmark: the file, the zero-based line, and the text it held when marked.
// Description comes from a selection, so a marked block shows its text.
type Bookmark struct {
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
}

// bookmarkColor paints a bookmark in the gutter, apart from the colors Git
// changes and breakpoints use.
var bookmarkColor = ui.RGB(232, 168, 64)

// maxBookmarkText bounds the stored line or selection text, as OpenIDEA clips
// it, so a line with a long literal keeps a readable title.
const maxBookmarkText = 200

// bookmarksPane is the open Bookmarks popup, or nil.
type bookmarksPane struct {
	open bool
	sel  int
}

// bookmarksInProject keeps the bookmarks that belong to root and whose file
// still exists, sorted the way the popup lists them.
func bookmarksInProject(list []Bookmark, root string) []Bookmark {
	out := make([]Bookmark, 0, len(list))
	seen := make(map[string]bool, len(list))
	for _, b := range list {
		if b.Path == "" || b.Line < 0 || !inProject(root, b.Path) {
			continue
		}
		key := fmt.Sprintf("%s:%d", b.Path, b.Line)
		if seen[key] {
			continue
		}
		if st, err := os.Stat(b.Path); err != nil || st.IsDir() {
			continue
		}
		seen[key] = true
		out = append(out, b)
	}
	sortBookmarks(out)
	return out
}

// sortBookmarks orders by file, then line, as OpenIDEA sorts its bookmarks.
func sortBookmarks(list []Bookmark) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Path != list[j].Path {
			return list[i].Path < list[j].Path
		}
		return list[i].Line < list[j].Line
	})
}

// toggleBookmark removes the line's bookmark or adds one, reporting whether it
// was added.
func toggleBookmark(list []Bookmark, add Bookmark) ([]Bookmark, bool) {
	for i, b := range list {
		if b.Path == add.Path && b.Line == add.Line {
			out := append(append([]Bookmark(nil), list[:i]...), list[i+1:]...)
			return out, false
		}
	}
	out := append(append([]Bookmark(nil), list...), add)
	sortBookmarks(out)
	return out, true
}

// stepBookmark returns the bookmark delta steps from the caret, wrapping around
// the ends the way OpenIDEA's Next and Previous Bookmark do.
func stepBookmark(list []Bookmark, path string, line, delta int) (Bookmark, bool) {
	if len(list) == 0 {
		return Bookmark{}, false
	}
	sorted := append([]Bookmark(nil), list...)
	sortBookmarks(sorted)
	if delta >= 0 {
		for _, b := range sorted {
			if b.Path > path || (b.Path == path && b.Line > line) {
				return b, true
			}
		}
		return sorted[0], true
	}
	for i := len(sorted) - 1; i >= 0; i-- {
		b := sorted[i]
		if b.Path < path || (b.Path == path && b.Line < line) {
			return b, true
		}
	}
	return sorted[len(sorted)-1], true
}

// bookmarkLineText is a line's trimmed text, clipped, for the popup title.
func bookmarkLineText(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return clipBookmarkText(lines[line])
}

// clipBookmarkText trims and flattens text and keeps it short, as OpenIDEA's
// clip does before storing a bookmark.
func clipBookmarkText(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > maxBookmarkText {
		runes = runes[:maxBookmarkText]
	}
	return string(runes)
}

// bookmarkTitle is what the popup shows for a bookmark.
func bookmarkTitle(b Bookmark) string {
	if b.Description != "" {
		return b.Description
	}
	if b.Text != "" {
		return b.Text
	}
	return filepath.Base(b.Path)
}

// ToggleBookmark marks or unmarks the caret's line in the active file.
func (m *Model) ToggleBookmark() {
	t := m.tab()
	if t == nil {
		m.status = "先打开一个文件再添加书签"
		m.invalidate()
		return
	}
	pos := t.state.CursorPos()
	b := Bookmark{
		Path:        t.doc.Path(),
		Line:        pos.Line,
		Text:        bookmarkLineText(t.doc.Text(), pos.Line),
		Description: clipBookmarkText(t.state.SelectedText()),
	}
	var added bool
	m.bookmarks, added = toggleBookmark(m.bookmarks, b)
	if added {
		m.status = fmt.Sprintf("已添加书签 %s:%d", filepath.Base(b.Path), b.Line+1)
	} else {
		m.status = fmt.Sprintf("已移除书签 %s:%d", filepath.Base(b.Path), b.Line+1)
	}
	m.saveSession()
	m.refreshDecorations()
	m.invalidate()
}

// NextBookmark jumps to the bookmark after the caret, wrapping at the end.
func (m *Model) NextBookmark() { m.jumpBookmark(1) }

// PrevBookmark jumps to the bookmark before the caret, wrapping at the start.
func (m *Model) PrevBookmark() { m.jumpBookmark(-1) }

// jumpBookmark moves to the next or previous bookmark from the caret.
func (m *Model) jumpBookmark(delta int) {
	path, line := "", -1
	if t := m.tab(); t != nil {
		path = t.doc.Path()
		line = t.state.CursorPos().Line
	}
	b, ok := stepBookmark(m.bookmarks, path, line, delta)
	if !ok {
		m.status = "还没有书签"
		m.invalidate()
		return
	}
	m.applyBookmark(b)
}

// applyBookmark opens the bookmark's file and puts the caret on its line.
func (m *Model) applyBookmark(b Bookmark) {
	m.revealAt(b.Path, b.Line, 0)
	m.status = fmt.Sprintf("书签 %s:%d", filepath.Base(b.Path), b.Line+1)
	m.invalidate()
}

// ShowBookmarks opens the Bookmarks popup.
func (m *Model) ShowBookmarks() {
	if m.bookmarksView == nil {
		m.bookmarksView = &bookmarksPane{}
	}
	m.bookmarksView.open = true
	m.bookmarksView.sel = clampSel(m.bookmarksView.sel, len(m.bookmarks))
	m.invalidate()
}

// closeBookmarks drops the Bookmarks popup.
func (m *Model) closeBookmarks() {
	if m.bookmarksView == nil {
		return
	}
	m.bookmarksView = nil
	m.invalidate()
}

// stepBookmarkSel moves the popup's highlighted row, stopping at the ends.
func (m *Model) stepBookmarkSel(delta int) {
	v := m.bookmarksView
	if v == nil || len(m.bookmarks) == 0 {
		return
	}
	v.sel = clampSel(v.sel+delta, len(m.bookmarks))
	m.invalidate()
}

// openSelectedBookmark jumps from the popup's highlighted row.
func (m *Model) openSelectedBookmark() {
	v := m.bookmarksView
	if v == nil || len(m.bookmarks) == 0 {
		return
	}
	b := m.bookmarks[clampSel(v.sel, len(m.bookmarks))]
	m.closeBookmarks()
	m.applyBookmark(b)
}

// removeBookmarkAt drops the bookmark the popup row shows and keeps the
// highlighted row inside the shorter list.
func (m *Model) removeBookmarkAt(i int) {
	if i < 0 || i >= len(m.bookmarks) {
		return
	}
	b := m.bookmarks[i]
	m.bookmarks = append(append([]Bookmark(nil), m.bookmarks[:i]...), m.bookmarks[i+1:]...)
	if v := m.bookmarksView; v != nil {
		v.sel = clampSel(v.sel, len(m.bookmarks))
	}
	m.status = fmt.Sprintf("已移除书签 %s:%d", filepath.Base(b.Path), b.Line+1)
	m.saveSession()
	m.refreshDecorations()
	m.invalidate()
}

func clampSel(sel, n int) int {
	if n <= 0 {
		return 0
	}
	if sel < 0 {
		return 0
	}
	if sel >= n {
		return n - 1
	}
	return sel
}

// bookmarkMarks is a file's bookmarks as gutter dots. A line that already has a
// breakpoint or Git mark keeps that color, so the gutter shows one dot per line.
func (m *Model) bookmarkMarks(path string, taken map[int]bool) []editor.GutterMark {
	var marks []editor.GutterMark
	for _, b := range m.bookmarks {
		if b.Path != path || taken[b.Line] {
			continue
		}
		marks = append(marks, editor.GutterMark{Line: b.Line, Color: bookmarkColor})
	}
	return marks
}

// bookmarksDialog draws the Bookmarks popup: every marked line, the highlighted
// one Opening on Enter or a click, and a delete chip per row.
func (m *Model) bookmarksDialog(c *ui.Context, p *ui.Theme) {
	v := m.bookmarksView
	if v == nil {
		return
	}
	list := append([]Bookmark(nil), m.bookmarks...)
	sortBookmarks(list)
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, "书签").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if len(list) == 0 {
				ui.Text(c, "还没有书签；在编辑器中按 F3 标记当前行。").FontSize(11).
					TextColor(p.TextMuted).SingleLine()
				return
			}
			ui.Text(c, fmt.Sprintf("%d 个书签 · Enter 打开 · Cmd+F3 关闭", len(list))).FontSize(10.5).
				TextColor(p.TextMuted).SingleLine()
			root := m.Root()
			var jump, remove = -1, -1
			ui.Scroll(c).Height(320).Children(func() {
				ui.Column(c).Children(func() {
					for i, b := range list {
						i, b := i, b
						r := ui.Row(c.Key(fmt.Sprintf("bookmark:%d", i))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == v.sel || r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							ui.Text(c, bookmarkTitle(b)).FontSize(12).TextColor(p.Text).
								SingleLine().Ellipsis("…")
							ui.Spacer(c)
							ui.Text(c, symbolWhere(root, b.Path, b.Line)).FontSize(10.5).
								TextColor(p.Accent).SingleLine().Ellipsis("…")
							if m.rowChip(c, p, fmt.Sprintf("bookmark:del:%d", i), "删除") {
								remove = i
							}
						})
						if r.Clicked() {
							jump = i
						}
					}
				})
			})
			switch {
			case remove >= 0:
				m.removeBookmarkAt(remove)
			case jump >= 0:
				m.closeBookmarks()
				m.applyBookmark(list[jump])
			}
		})
	})
	if d.Dismissed() || !open {
		m.closeBookmarks()
	}
}

// SmokeBookmark proves the bookmark actions on the open file: two marks, their
// gutter dots, stepping between them, and removing them again.
func (m *Model) SmokeBookmark(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("没有打开项目")
	}
	if t := m.tab(); t == nil {
		candidate := findSmokeFile(root)
		if candidate == "" {
			return nil, errors.New("项目里没有可打开的源码文件")
		}
		m.post(func() { m.openFile(candidate) })
		if !m.waitDebug(ctx, func(*DebugPane) bool { return m.tab() != nil }, 60*time.Second) {
			return nil, errors.New("打开 " + candidate + " 超时")
		}
	}
	out := map[string]any{}
	done := make(chan struct{})
	m.post(func() {
		defer close(done)
		t := m.tab()
		if t == nil {
			return
		}
		first, second := 2, 5
		t.state.GoTo(editor.Pos{Line: first, Col: 0})
		m.ToggleBookmark()
		t.state.GoTo(editor.Pos{Line: second, Col: 0})
		m.ToggleBookmark()
		out["added"] = len(m.bookmarks)
		dots := 0
		for _, mark := range t.state.GutterMarks {
			if mark.Color == bookmarkColor {
				dots++
			}
		}
		out["gutterMarks"] = dots
		t.state.GoTo(editor.Pos{Line: 0, Col: 0})
		m.NextBookmark()
		out["nextFirst"] = m.tab().state.CursorPos().Line
		m.NextBookmark()
		out["nextSecond"] = m.tab().state.CursorPos().Line
		m.PrevBookmark()
		out["prev"] = m.tab().state.CursorPos().Line
		t.state.GoTo(editor.Pos{Line: first, Col: 0})
		m.ToggleBookmark()
		out["afterRemove"] = len(m.bookmarks)
		t.state.GoTo(editor.Pos{Line: second, Col: 0})
		m.ToggleBookmark()
		out["cleared"] = len(m.bookmarks)
	})
	select {
	case <-done:
	case <-ctx.Done():
		return out, ctx.Err()
	}
	return out, nil
}
