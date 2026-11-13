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
	// Mnemonic is one digit or capital letter, unique across bookmarks, so
	// Ctrl+digit or the Alt+F3 chooser reaches it.
	Mnemonic string `json:"mnemonic,omitempty"`
	// List is the bookmark list it belongs to; empty means the default list.
	List string `json:"list,omitempty"`
}

// bookmarkDefaultList is the id of the list new bookmarks go to.
const bookmarkDefaultList = "default"

// BookmarkList is one named group in the Bookmarks tool window.
type BookmarkList struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// bookmarkMnemonics are the keys a bookmark can be reached with.
const bookmarkMnemonics = "1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ"

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

// mnemonicPicker is the open Alt+F3 chooser, or nil.
type mnemonicPicker struct {
	open bool
}

// newListPane is the open New Bookmark List dialog, or nil.
type newListPane struct {
	open bool
	name string
	err  string
}

// assignBookmarkMnemonic gives the line's bookmark mnemonic, creating the
// bookmark when needed and taking the key from whichever bookmark had it.
// An empty mnemonic removes the key and keeps the bookmark.
func assignBookmarkMnemonic(list []Bookmark, b Bookmark, mnemonic string) []Bookmark {
	out := make([]Bookmark, 0, len(list)+1)
	found := false
	for _, cur := range list {
		if cur.Path == b.Path && cur.Line == b.Line {
			found = true
			cur.Mnemonic = mnemonic
			if cur.Description == "" {
				cur.Description = b.Description
			}
			if cur.Text == "" {
				cur.Text = b.Text
			}
		} else if cur.Mnemonic == mnemonic {
			cur.Mnemonic = ""
		}
		out = append(out, cur)
	}
	if !found {
		b.Mnemonic = mnemonic
		out = append(out, b)
	}
	sortBookmarks(out)
	return out
}

// toggleBookmarkMnemonic sets mnemonic on the line's bookmark, or removes the
// bookmark when the line already carries that key.
func toggleBookmarkMnemonic(list []Bookmark, b Bookmark, mnemonic string) ([]Bookmark, bool) {
	for i, cur := range list {
		if cur.Path == b.Path && cur.Line == b.Line && cur.Mnemonic == mnemonic {
			out := append(append([]Bookmark(nil), list[:i]...), list[i+1:]...)
			return out, false
		}
	}
	return assignBookmarkMnemonic(list, b, mnemonic), true
}

// bookmarkByMnemonic finds the one bookmark carrying the key.
func bookmarkByMnemonic(list []Bookmark, mnemonic string) (Bookmark, bool) {
	if mnemonic == "" {
		return Bookmark{}, false
	}
	for _, b := range list {
		if b.Mnemonic == mnemonic {
			return b, true
		}
	}
	return Bookmark{}, false
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

// PathBookmark is a bookmarked file, as OpenIDEA's Toggle File Bookmark marks
// the project view or editor tab selection.
type PathBookmark struct {
	Path string `json:"path"`
	List string `json:"list,omitempty"`
}

// togglePathBookmark adds the file's bookmark, or removes it when present,
// reporting whether it was added.
func togglePathBookmark(list []PathBookmark, path string) ([]PathBookmark, bool) {
	for i, pb := range list {
		if pb.Path == path {
			out := append(append([]PathBookmark(nil), list[:i]...), list[i+1:]...)
			return out, false
		}
	}
	out := append(append([]PathBookmark(nil), list...), PathBookmark{Path: path})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, true
}

// pathBookmarksInProject keeps the file bookmarks inside root that still exist.
func pathBookmarksInProject(list []PathBookmark, root string) []PathBookmark {
	out := make([]PathBookmark, 0, len(list))
	seen := make(map[string]bool, len(list))
	for _, pb := range list {
		if pb.Path == "" || !inProject(root, pb.Path) || seen[pb.Path] {
			continue
		}
		if st, err := os.Stat(pb.Path); err != nil || st.IsDir() {
			continue
		}
		seen[pb.Path] = true
		out = append(out, pb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// pathBookmarkLabel is the file's path relative to the project root.
func pathBookmarkLabel(root, path string) string {
	if root == "" {
		return path
	}
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
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

// caretBookmark is the active file's caret line as a bookmark.
func (m *Model) caretBookmark() (Bookmark, bool) {
	t := m.tab()
	if t == nil {
		m.status = "先打开一个文件再添加书签"
		m.invalidate()
		return Bookmark{}, false
	}
	pos := t.state.CursorPos()
	return Bookmark{
		Path:        t.doc.Path(),
		Line:        pos.Line,
		Text:        bookmarkLineText(t.doc.Text(), pos.Line),
		Description: clipBookmarkText(t.state.SelectedText()),
		List:        m.defaultBookmarkList(),
	}, true
}

// ToggleBookmark marks or unmarks the caret's line in the active file.
func (m *Model) ToggleBookmark() {
	b, ok := m.caretBookmark()
	if !ok {
		return
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

// OpenBookmarkMnemonics opens the Alt+F3 chooser for the caret's line.
func (m *Model) OpenBookmarkMnemonics() {
	if _, ok := m.caretBookmark(); !ok {
		return
	}
	m.bookmarksView = nil
	m.mnemonics = &mnemonicPicker{open: true}
	m.invalidate()
}

// closeBookmarkMnemonics drops the chooser.
func (m *Model) closeBookmarkMnemonics() {
	if m.mnemonics == nil {
		return
	}
	m.mnemonics = nil
	m.invalidate()
}

// SetBookmarkMnemonic gives the caret's line the key, creating the bookmark
// first when needed; an empty key clears it.
func (m *Model) SetBookmarkMnemonic(mnemonic string) {
	b, ok := m.caretBookmark()
	if !ok {
		return
	}
	m.bookmarks = assignBookmarkMnemonic(m.bookmarks, b, mnemonic)
	if mnemonic == "" {
		m.status = fmt.Sprintf("已清除 %s:%d 的书签助记符", filepath.Base(b.Path), b.Line+1)
	} else {
		m.status = fmt.Sprintf("书签助记符 %s → %s:%d", mnemonic, filepath.Base(b.Path), b.Line+1)
	}
	m.closeBookmarkMnemonics()
	m.saveSession()
	m.refreshDecorations()
	m.invalidate()
}

// ToggleBookmarkMnemonic is Ctrl+Shift+digit: the line already carries that
// key, so the bookmark goes; otherwise the key is assigned to it.
func (m *Model) ToggleBookmarkMnemonic(mnemonic string) {
	b, ok := m.caretBookmark()
	if !ok {
		return
	}
	var added bool
	m.bookmarks, added = toggleBookmarkMnemonic(m.bookmarks, b, mnemonic)
	if added {
		m.status = fmt.Sprintf("书签助记符 %s → %s:%d", mnemonic, filepath.Base(b.Path), b.Line+1)
	} else {
		m.status = fmt.Sprintf("已移除书签 %s:%d", filepath.Base(b.Path), b.Line+1)
	}
	m.saveSession()
	m.refreshDecorations()
	m.invalidate()
}

// JumpBookmarkMnemonic is Ctrl+digit: go to the bookmark carrying the key.
func (m *Model) JumpBookmarkMnemonic(mnemonic string) {
	b, ok := bookmarkByMnemonic(m.bookmarks, mnemonic)
	if !ok {
		m.status = fmt.Sprintf("没有助记符为 %s 的书签", mnemonic)
		m.invalidate()
		return
	}
	m.applyBookmark(b)
}

// bookmarkDigitShortcuts runs Ctrl+Shift+digit and Ctrl+digit, the two
// OpenIDEA keys that set and reach bookmarks by mnemonic.
func (m *Model) bookmarkDigitShortcuts(c *ui.Context) {
	if m.overlayOpen() {
		return
	}
	digits := []struct {
		key   ui.Key
		label string
	}{
		{ui.Key1, "1"}, {ui.Key2, "2"}, {ui.Key3, "3"}, {ui.Key4, "4"},
		{ui.Key5, "5"}, {ui.Key6, "6"}, {ui.Key7, "7"}, {ui.Key8, "8"},
		{ui.Key9, "9"}, {ui.Key0, "0"},
	}
	for _, d := range digits {
		if c.Shortcut(ui.Ctrl|ui.Shift, d.key) {
			m.ToggleBookmarkMnemonic(d.label)
			return
		}
	}
	for _, d := range digits {
		if c.Shortcut(ui.Ctrl, d.key) {
			m.JumpBookmarkMnemonic(d.label)
			return
		}
	}
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

// ShowBookmarksWindow reveals the Bookmarks tool window in the sidebar, the
// window OpenIDEA binds to Cmd+2.
func (m *Model) ShowBookmarksWindow() {
	m.structure.visible = false
	m.sideMode = "bookmarks"
	m.invalidate()
}

// ToggleFileBookmark marks or unmarks the active editor's file, which is the
// target OpenIDEA's Toggle File Bookmark uses when no project view selection
// is given.
func (m *Model) ToggleFileBookmark() {
	t := m.tab()
	if t == nil {
		m.status = "先打开一个文件"
		m.invalidate()
		return
	}
	path := t.doc.Path()
	var added bool
	m.pathBookmarks, added = togglePathBookmark(m.pathBookmarks, path)
	if added {
		for i := range m.pathBookmarks {
			if m.pathBookmarks[i].Path == path {
				m.pathBookmarks[i].List = m.defaultBookmarkList()
			}
		}
	}
	if added {
		m.status = "已添加文件书签 " + filepath.Base(path)
	} else {
		m.status = "已移除文件书签 " + filepath.Base(path)
	}
	m.saveSession()
	m.invalidate()
}

// removePathBookmark drops the bookmarked file.
func (m *Model) removePathBookmark(path string) {
	for i, pb := range m.pathBookmarks {
		if pb.Path != path {
			continue
		}
		m.pathBookmarks = append(append([]PathBookmark(nil), m.pathBookmarks[:i]...),
			m.pathBookmarks[i+1:]...)
		m.status = "已移除文件书签 " + filepath.Base(pb.Path)
		m.saveSession()
		m.invalidate()
		return
	}
}

// removeBookmark drops the bookmark on one line.
func (m *Model) removeBookmark(path string, line int) {
	for i, b := range m.bookmarks {
		if b.Path != path || b.Line != line {
			continue
		}
		m.removeBookmarkAt(i)
		return
	}
}

// defaultBookmarkList is the list new bookmarks go to, defaulting to the first
// list when the stored id is gone.
func (m *Model) defaultBookmarkList() string {
	lists := normalizeBookmarkLists(m.bookmarkLists)
	for _, l := range lists {
		if l.ID == m.defaultList {
			return l.ID
		}
	}
	return lists[0].ID
}

// SetDefaultBookmarkList makes a list the destination of new bookmarks.
func (m *Model) SetDefaultBookmarkList(id string) {
	for _, l := range normalizeBookmarkLists(m.bookmarkLists) {
		if l.ID == id {
			m.defaultList = id
			m.status = "新书签将加入“" + l.Name + "”"
			m.saveSession()
			m.invalidate()
			return
		}
	}
}

// OpenNewBookmarkList opens the naming dialog for a new bookmark list.
func (m *Model) OpenNewBookmarkList() {
	m.newList = &newListPane{open: true}
	m.invalidate()
}

// closeNewBookmarkList drops the naming dialog.
func (m *Model) closeNewBookmarkList() {
	if m.newList == nil {
		return
	}
	m.newList = nil
	m.invalidate()
}

// CreateBookmarkList adds the typed list and sends new bookmarks there.
func (m *Model) CreateBookmarkList() {
	v := m.newList
	if v == nil {
		return
	}
	lists, id, err := addBookmarkList(normalizeBookmarkLists(m.bookmarkLists), v.name)
	if err != "" {
		v.err = err
		m.invalidate()
		return
	}
	m.bookmarkLists = lists
	m.defaultList = id
	m.newList = nil
	m.status = "已新建书签列表“" + strings.TrimSpace(lists[len(lists)-1].Name) + "”"
	m.saveSession()
	m.invalidate()
}

// newListDialog draws the New Bookmark List prompt and creates the list.
func (m *Model) newListDialog(c *ui.Context, p *ui.Theme) {
	v := m.newList
	if v == nil {
		return
	}
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(8).Children(func() {
			ui.Text(c, "新建书签列表").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &v.name).AutoFocus()
			in.OnChange(func() { v.err = "" })
			if in.Submitted() {
				m.CreateBookmarkList()
			}
			ui.Text(c, "新书签会加入这个列表。").FontSize(10.5).TextColor(p.TextMuted).
				SingleLine()
			if v.err != "" {
				ui.Text(c, v.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { m.closeNewBookmarkList() })
				ui.Button(c, "创建").OnClick(func() { m.CreateBookmarkList() })
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeNewBookmarkList()
	}
}

// ToggleBookmarksWindow hides the tool window when it is already showing.
func (m *Model) ToggleBookmarksWindow() {
	if m.sideMode == "bookmarks" {
		m.ShowProject()
		return
	}
	m.ShowBookmarksWindow()
}

// bookmarkWindowRows draws the Bookmarks tool window: each list with its
// bookmarked files and lines, a click Opening one, and a chip that removes it.
// A list header click makes that list the destination of new bookmarks.
func (m *Model) bookmarkWindowRows(c *ui.Context, p *ui.Theme) {
	lists := normalizeBookmarkLists(m.bookmarkLists)
	root := m.Root()
	ui.Row(c).Height(26).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%d 个书签", len(m.bookmarks)+len(m.pathBookmarks))).
			FontSize(10.5).TextColor(p.TextMuted).SingleLine()
		ui.Spacer(c)
		if m.rowChip(c, p, "bookmark-window:newList", "新建列表") {
			m.OpenNewBookmarkList()
		}
	})
	empty := len(m.bookmarks) == 0 && len(m.pathBookmarks) == 0
	if empty {
		ui.Row(c).Padding(10, 8).Children(func() {
			ui.Text(c, "还没有书签；在编辑器中按 F3 标记当前行。").FontSize(11).
				TextColor(p.TextMuted)
		})
	}
	var jumpPath, removePath, makeDefault string
	var jumpLine, removeLine = -1, -1
	for _, l := range lists {
		l := l
		paths := pathBookmarksInList(m.pathBookmarks, l.ID)
		marks := bookmarksInList(m.bookmarks, l.ID)
		ui.Row(c).Height(22).Padding(6, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, l.Name).FontSize(10.5).Bold().TextColor(p.TextMuted).SingleLine()
			ui.Text(c, fmt.Sprintf("%d", len(paths)+len(marks))).FontSize(10).
				TextColor(p.TextMuted)
			ui.Spacer(c)
			if l.ID == m.defaultList {
				ui.Text(c, "默认").FontSize(9.5).TextColor(p.TextMuted)
			} else if m.rowChip(c, p, "bookmark-window:default:"+l.ID, "设为默认") {
				makeDefault = l.ID
			}
		})
		for _, pb := range paths {
			pb := pb
			r := ui.Row(c.Key("bookmark-window:path:"+l.ID+":"+pb.Path)).Height(22).
				Padding(0, 8).Gap(6).AlignItems(ui.Center)
			if r.Hovered() {
				r.Background(p.SurfaceHover)
			}
			r.Children(func() {
				ui.Text(c, pathBookmarkLabel(root, pb.Path)).FontSize(11.5).TextColor(p.Text).
					SingleLine().Ellipsis("…")
				ui.Spacer(c)
				if m.rowChip(c, p, "bookmark-window:pathDel:"+l.ID+":"+pb.Path, "删除") {
					removePath = pb.Path
				}
			})
			if r.Clicked() {
				jumpPath = pb.Path
			}
		}
		for _, b := range marks {
			b := b
			key := fmt.Sprintf("bookmark-window:%s:%s:%d", l.ID, b.Path, b.Line)
			ui.Column(c).Children(func() {
				r := ui.Row(c.Key(key)).Height(22).Padding(0, 8).Gap(6).
					AlignItems(ui.Center)
				if r.Hovered() {
					r.Background(p.SurfaceHover)
				}
				r.Children(func() {
					if b.Mnemonic != "" {
						ui.Text(c, b.Mnemonic).FontSize(10.5).Bold().TextColor(p.Accent).
							SingleLine()
					}
					ui.Text(c, bookmarkTitle(b)).FontSize(12).TextColor(p.Text).
						SingleLine().Ellipsis("…")
					ui.Spacer(c)
					if m.rowChip(c, p, key+":del", "删除") {
						removePath, removeLine = b.Path, b.Line
					}
				})
				if r.Clicked() {
					jumpPath, jumpLine = b.Path, b.Line
				}
				ui.Row(c).Padding(0, 8).Children(func() {
					ui.Text(c, symbolWhere(root, b.Path, b.Line)).FontSize(10).
						TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				})
			})
		}
	}
	switch {
	case makeDefault != "":
		m.SetDefaultBookmarkList(makeDefault)
	case removeLine >= 0:
		m.removeBookmark(removePath, removeLine)
	case removePath != "":
		m.removePathBookmark(removePath)
	case jumpLine >= 0:
		m.applyBookmark(Bookmark{Path: jumpPath, Line: jumpLine})
	case jumpPath != "":
		m.openFile(jumpPath)
		m.invalidate()
	}
}

// bookmarksInList keeps the bookmarks that belong to listID.
func bookmarksInList(list []Bookmark, listID string) []Bookmark {
	out := make([]Bookmark, 0, len(list))
	for _, b := range list {
		if bookmarkListOf(b.List) == listID {
			out = append(out, b)
		}
	}
	sortBookmarks(out)
	return out
}

// pathBookmarksInList keeps the file bookmarks that belong to listID.
func pathBookmarksInList(list []PathBookmark, listID string) []PathBookmark {
	out := make([]PathBookmark, 0, len(list))
	for _, pb := range list {
		if bookmarkListOf(pb.List) == listID {
			out = append(out, pb)
		}
	}
	return out
}

// bookmarkListOf names the list a bookmark without one belongs to.
func bookmarkListOf(id string) string {
	if id == "" {
		return bookmarkDefaultList
	}
	return id
}

// normalizeBookmarkLists makes the default list exist, drops unnamed or
// duplicate entries, and puts the default first.
func normalizeBookmarkLists(lists []BookmarkList) []BookmarkList {
	var out, rest []BookmarkList
	seen := make(map[string]bool, len(lists))
	for _, l := range lists {
		l.Name = strings.TrimSpace(l.Name)
		if l.ID == "" || l.Name == "" || seen[l.ID] {
			continue
		}
		seen[l.ID] = true
		if l.ID == bookmarkDefaultList {
			out = append(out, l)
			continue
		}
		rest = append(rest, l)
	}
	if len(out) == 0 {
		out = append(out, BookmarkList{ID: bookmarkDefaultList, Name: "书签"})
	}
	return append(out, rest...)
}

// addBookmarkList creates a list with a fresh id; an unusable name comes back
// as the error text.
func addBookmarkList(lists []BookmarkList, name string) ([]BookmarkList, string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return lists, "", "列表名称不能为空"
	}
	for _, l := range lists {
		if l.Name == name {
			return lists, "", "已经有名为“" + name + "”的列表"
		}
	}
	for i := len(lists) + 1; ; i++ {
		id := fmt.Sprintf("list%d", i)
		taken := false
		for _, l := range lists {
			if l.ID == id {
				taken = true
				break
			}
		}
		if taken {
			continue
		}
		out := append(append([]BookmarkList(nil), lists...), BookmarkList{ID: id, Name: name})
		return out, id, ""
	}
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
							if b.Mnemonic != "" {
								ui.Text(c, b.Mnemonic).FontSize(10.5).Bold().
									TextColor(p.Accent).SingleLine()
							}
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

// mnemonicDialog draws the Alt+F3 chooser: every digit and capital letter, the
// ones in use in the accent color, and a row that clears the caret's key.
func (m *Model) mnemonicDialog(c *ui.Context, p *ui.Theme) {
	v := m.mnemonics
	if v == nil {
		return
	}
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(440).Gap(8).Children(func() {
			ui.Text(c, "书签助记符").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			t := m.tab()
			if t == nil {
				ui.Text(c, "先打开一个文件。").FontSize(11).TextColor(p.TextMuted).SingleLine()
				return
			}
			pos := t.state.CursorPos()
			current := ""
			for _, b := range m.bookmarks {
				if b.Path == t.doc.Path() && b.Line == pos.Line {
					current = b.Mnemonic
				}
			}
			ui.Text(c, fmt.Sprintf("%s:%d · 已有该键的书签会失去它", filepath.Base(t.doc.Path()), pos.Line+1)).
				FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			clear := false
			pick := ""
			for _, group := range []string{"1234567890", "ABCDEFGHI", "JKLMNOPQR", "STUVWXYZ"} {
				ui.Row(c).Gap(2).Children(func() {
					for _, r := range group {
						key := string(r)
						chip := ui.Row(c.Key("mnemonic:"+key)).Width(28).Height(24).
							Padding(0, 6).AlignItems(ui.Center)
						switch {
						case key == current:
							chip.Background(p.SurfacePressed)
						case chip.Hovered():
							chip.Background(p.SurfaceHover)
						}
						taken := false
						if _, ok := bookmarkByMnemonic(m.bookmarks, key); ok {
							taken = true
						}
						chip.Children(func() {
							color := p.Text
							if taken {
								color = p.Accent
							}
							ui.Text(c, key).FontSize(12).Bold().TextColor(color)
						})
						if chip.Clicked() {
							pick = key
						}
					}
				})
			}
			if m.rowChip(c, p, "mnemonic:clear", "移除助记符") {
				clear = true
			}
			switch {
			case pick != "":
				m.SetBookmarkMnemonic(pick)
			case clear:
				m.SetBookmarkMnemonic("")
			}
		})
	})
	if d.Dismissed() || !open {
		m.closeBookmarkMnemonics()
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
		m.OpenNewBookmarkList()
		m.newList.name = "Work"
		m.CreateBookmarkList()
		out["lists"] = len(m.bookmarkLists)
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
		t.state.GoTo(editor.Pos{Line: first, Col: 0})
		m.SetBookmarkMnemonic("1")
		setLine := -1
		for _, b := range m.bookmarks {
			if b.Mnemonic == "1" {
				setLine = b.Line
			}
		}
		out["mnemonicSet"] = setLine
		t.state.GoTo(editor.Pos{Line: 0, Col: 0})
		m.JumpBookmarkMnemonic("1")
		out["mnemonicJump"] = m.tab().state.CursorPos().Line
		// The key is unique: giving it to the second line takes it from the first.
		t.state.GoTo(editor.Pos{Line: second, Col: 0})
		m.SetBookmarkMnemonic("1")
		movedLine := -1
		for _, b := range m.bookmarks {
			if b.Mnemonic == "1" {
				movedLine = b.Line
			}
		}
		out["mnemonicMoved"] = movedLine
		assigned := 0
		for _, b := range m.bookmarks {
			if m.defaultList != bookmarkDefaultList && b.List == m.defaultList {
				assigned++
			}
		}
		out["listAssigned"] = assigned
		m.ShowBookmarksWindow()
		out["windowMode"] = m.sideMode
		out["windowRows"] = len(m.bookmarks)
		m.ShowProject()
		m.ToggleFileBookmark()
		out["fileMarked"] = len(m.pathBookmarks)
		m.ToggleFileBookmark()
		out["fileCleared"] = len(m.pathBookmarks)
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
