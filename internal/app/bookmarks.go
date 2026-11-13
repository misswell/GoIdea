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

// newListPane is the open new or rename Bookmark List dialog, or nil.
type newListPane struct {
	open     bool
	name     string
	err      string
	renameID string
	// move names a bookmark the new list takes, as the move chooser's New
	// List… does in OpenIDEA.
	move *movePane
}

// movePane is the open Move to List chooser, or nil.
type movePane struct {
	open bool
	path string
	line int
	// file names a file or folder bookmark rather than a line one.
	file bool
}

// deleteListPane is the open Delete Bookmark List confirmation, or nil.
type deleteListPane struct {
	open  bool
	id    string
	name  string
	count int
}

// descPane is the open Edit Bookmark Description dialog, or nil.
type descPane struct {
	open bool
	path string
	line int
	text string
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

// PathBookmark is a bookmarked file or folder, as OpenIDEA's Toggle File
// Bookmark marks the project view or editor tab selection.
type PathBookmark struct {
	Path string `json:"path"`
	// Dir marks a folder, which the project view's bookmark targets add.
	Dir  bool   `json:"dir,omitempty"`
	List string `json:"list,omitempty"`
}

// togglePathBookmark adds the file or folder's bookmark, or removes it when
// present, reporting whether it was added.
func togglePathBookmark(list []PathBookmark, path string, dir bool) ([]PathBookmark, bool) {
	for i, pb := range list {
		if pb.Path == path {
			out := append(append([]PathBookmark(nil), list[:i]...), list[i+1:]...)
			return out, false
		}
	}
	out := append(append([]PathBookmark(nil), list...), PathBookmark{Path: path, Dir: dir})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, true
}

// pathBookmarksInProject keeps the file and folder bookmarks inside root that
// still exist.
func pathBookmarksInProject(list []PathBookmark, root string) []PathBookmark {
	out := make([]PathBookmark, 0, len(list))
	seen := make(map[string]bool, len(list))
	for _, pb := range list {
		if pb.Path == "" || !inProject(root, pb.Path) || seen[pb.Path] {
			continue
		}
		st, err := os.Stat(pb.Path)
		if err != nil || st.IsDir() != pb.Dir {
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
	m.bookmarksView.sel = clampSel(m.bookmarksView.sel, len(m.bookmarkPopupItems()))
	m.invalidate()
}

// OpenBookmarkDescription opens the description dialog for the popup's
// highlighted line, as OpenIDEA's F2 does in the Bookmarks view.
func (m *Model) OpenBookmarkDescription() {
	v := m.bookmarksView
	if v == nil {
		return
	}
	items := m.bookmarkPopupItems()
	if len(items) == 0 {
		return
	}
	it := items[clampSel(v.sel, len(items))]
	if it.File {
		return
	}
	b, _ := bookmarkAt(m.bookmarks, it.Path, it.Line)
	text := b.Description
	if text == "" {
		text = b.Text
	}
	m.editDesc = &descPane{open: true, path: it.Path, line: it.Line, text: text}
	m.invalidate()
}

// closeBookmarkDescription drops the description dialog.
func (m *Model) closeBookmarkDescription() {
	if m.editDesc == nil {
		return
	}
	m.editDesc = nil
	m.invalidate()
}

// SaveBookmarkDescription stores the typed description on the bookmark.
func (m *Model) SaveBookmarkDescription() {
	v := m.editDesc
	if v == nil {
		return
	}
	list, ok := setBookmarkDescription(m.bookmarks, v.path, v.line, v.text)
	if !ok {
		m.editDesc = nil
		m.invalidate()
		return
	}
	m.bookmarks = list
	m.editDesc = nil
	m.status = "已更新书签描述"
	m.saveSession()
	m.invalidate()
}

// setBookmarkDescription sets the line's description; an empty one falls back
// to the line text when the popup shows the title.
func setBookmarkDescription(list []Bookmark, path string, line int, text string) ([]Bookmark, bool) {
	out := append([]Bookmark(nil), list...)
	for i := range out {
		if out[i].Path != path || out[i].Line != line {
			continue
		}
		out[i].Description = strings.TrimSpace(text)
		return out, true
	}
	return list, false
}

// ShowBookmarksWindow reveals the Bookmarks tool window in the left side's
// split area, the slot OpenIDEA gives it below the Project window (Cmd+2).
func (m *Model) ShowBookmarksWindow() {
	m.structure.visible = false
	m.leftSplit = "bookmarks"
	m.leftHidden = false
	m.rememberLayout()
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
	m.pathBookmarks, added = togglePathBookmark(m.pathBookmarks, path, false)
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

// ToggleFolderBookmark marks or unmarks a project view row, which OpenIDEA's
// project view bookmark target does for files and folders alike.
func (m *Model) ToggleFolderBookmark(path string, dir bool) {
	if path == "" {
		return
	}
	var added bool
	m.pathBookmarks, added = togglePathBookmark(m.pathBookmarks, path, dir)
	if added {
		for i := range m.pathBookmarks {
			if m.pathBookmarks[i].Path == path {
				m.pathBookmarks[i].List = m.defaultBookmarkList()
			}
		}
	}
	name := filepath.Base(path)
	if dir {
		name += "/"
	}
	if added {
		m.status = "已添加书签 " + name
	} else {
		m.status = "已移除书签 " + name
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

// moveBookmarkToList sends one line bookmark to another list.
func moveBookmarkToList(list []Bookmark, path string, line int, to string) []Bookmark {
	out := append([]Bookmark(nil), list...)
	for i := range out {
		if out[i].Path == path && out[i].Line == line {
			out[i].List = to
		}
	}
	return out
}

// movePathBookmarkToList sends one file or folder bookmark to another list.
func movePathBookmarkToList(list []PathBookmark, path, to string) []PathBookmark {
	out := append([]PathBookmark(nil), list...)
	for i := range out {
		if out[i].Path == path {
			out[i].List = to
		}
	}
	return out
}

// OpenMoveBookmark opens the Move to List chooser for a line bookmark.
func (m *Model) OpenMoveBookmark(path string, line int) {
	m.move = &movePane{open: true, path: path, line: line}
	m.invalidate()
}

// OpenMoveFileBookmark opens the chooser for a file or folder bookmark.
func (m *Model) OpenMoveFileBookmark(path string) {
	m.move = &movePane{open: true, path: path, file: true}
	m.invalidate()
}

// closeMoveBookmark drops the chooser.
func (m *Model) closeMoveBookmark() {
	if m.move == nil {
		return
	}
	m.move = nil
	m.invalidate()
}

// moveSourceList names the list the chooser's bookmark lives in.
func (m *Model) moveSourceList(v *movePane) string {
	if v.file {
		for _, pb := range m.pathBookmarks {
			if pb.Path == v.path {
				return bookmarkListOf(pb.List)
			}
		}
		return ""
	}
	for _, b := range m.bookmarks {
		if b.Path == v.path && b.Line == v.line {
			return bookmarkListOf(b.List)
		}
	}
	return ""
}

// MoveBookmarkToList moves the chooser's bookmark into id.
func (m *Model) MoveBookmarkToList(id string) {
	v := m.move
	if v == nil {
		return
	}
	for _, l := range normalizeBookmarkLists(m.bookmarkLists) {
		if l.ID != id {
			continue
		}
		if v.file {
			m.pathBookmarks = movePathBookmarkToList(m.pathBookmarks, v.path, id)
		} else {
			m.bookmarks = moveBookmarkToList(m.bookmarks, v.path, v.line, id)
		}
		m.move = nil
		m.status = "已将书签移动到“" + l.Name + "”"
		m.saveSession()
		m.invalidate()
		return
	}
}

// OpenNewBookmarkListForMove asks for a list name, then moves the bookmark the
// chooser names into it, as OpenIDEA's move menu's New List… does.
func (m *Model) OpenNewBookmarkListForMove() {
	v := m.move
	if v == nil {
		return
	}
	m.move = nil
	m.newList = &newListPane{open: true, move: v}
	m.invalidate()
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

// OpenRenameBookmarkList opens the naming dialog for an existing list.
func (m *Model) OpenRenameBookmarkList(id string) {
	for _, l := range normalizeBookmarkLists(m.bookmarkLists) {
		if l.ID == id {
			m.newList = &newListPane{open: true, name: l.Name, renameID: id}
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

// CreateBookmarkList adds the typed list and sends new bookmarks there, or
// renames the list the dialog was opened for.
func (m *Model) CreateBookmarkList() {
	v := m.newList
	if v == nil {
		return
	}
	if v.renameID != "" {
		lists, name, err := renameBookmarkList(normalizeBookmarkLists(m.bookmarkLists), v.renameID, v.name)
		if err != "" {
			v.err = err
			m.invalidate()
			return
		}
		m.bookmarkLists = lists
		m.newList = nil
		m.status = "已将书签列表重命名为“" + name + "”"
		m.saveSession()
		m.invalidate()
		return
	}
	lists, id, err := addBookmarkList(normalizeBookmarkLists(m.bookmarkLists), v.name)
	if err != "" {
		v.err = err
		m.invalidate()
		return
	}
	m.bookmarkLists = lists
	m.newList = nil
	name := strings.TrimSpace(lists[len(lists)-1].Name)
	if v.move != nil {
		// The move chooser's New List… adds the list and puts the bookmark in
		// it, without changing where new bookmarks go, as OpenIDEA does.
		if v.move.file {
			m.pathBookmarks = movePathBookmarkToList(m.pathBookmarks, v.move.path, id)
		} else {
			m.bookmarks = moveBookmarkToList(m.bookmarks, v.move.path, v.move.line, id)
		}
		m.status = "已新建书签列表“" + name + "”并移动书签"
	} else {
		m.defaultList = id
		m.status = "已新建书签列表“" + name + "”"
	}
	m.saveSession()
	m.refreshDecorations()
	m.invalidate()
}

// AskDeleteBookmarkList asks before a list and its bookmarks go; an empty
// list is dropped without a prompt.
func (m *Model) AskDeleteBookmarkList(id string) {
	for _, l := range normalizeBookmarkLists(m.bookmarkLists) {
		if l.ID != id {
			continue
		}
		n := len(bookmarksInList(m.bookmarks, id)) + len(pathBookmarksInList(m.pathBookmarks, id))
		if n == 0 {
			m.deleteBookmarkList(id)
			return
		}
		m.deleteList = &deleteListPane{open: true, id: id, name: l.Name, count: n}
		m.invalidate()
		return
	}
}

// closeDeleteBookmarkList drops the confirmation.
func (m *Model) closeDeleteBookmarkList() {
	if m.deleteList == nil {
		return
	}
	m.deleteList = nil
	m.invalidate()
}

// ConfirmDeleteBookmarkList removes the list the confirmation names.
func (m *Model) ConfirmDeleteBookmarkList() {
	v := m.deleteList
	if v == nil {
		return
	}
	m.deleteBookmarkList(v.id)
}

// deleteBookmarkList drops a list with the bookmarks in it.
func (m *Model) deleteBookmarkList(id string) {
	name := ""
	for _, l := range normalizeBookmarkLists(m.bookmarkLists) {
		if l.ID == id {
			name = l.Name
		}
	}
	lists, marks, paths, def := removeBookmarkList(normalizeBookmarkLists(m.bookmarkLists),
		m.bookmarks, m.pathBookmarks, m.defaultList, id, "书签")
	m.bookmarkLists = lists
	m.bookmarks = marks
	m.pathBookmarks = paths
	m.defaultList = def
	m.deleteList = nil
	m.status = "已删除书签列表“" + name + "”"
	m.saveSession()
	m.refreshDecorations()
	m.invalidate()
}

// newListDialog draws the New or Rename Bookmark List prompt and applies it.
func (m *Model) newListDialog(c *ui.Context, p *ui.Theme) {
	v := m.newList
	if v == nil {
		return
	}
	renaming := v.renameID != ""
	title, hint, action := "新建书签列表", "新书签会加入这个列表。", "创建"
	if renaming {
		title, hint, action = "重命名书签列表", "列表里的书签会保留。", "重命名"
	}
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &v.name).AutoFocus()
			in.OnChange(func() { v.err = "" })
			if in.Submitted() {
				m.CreateBookmarkList()
			}
			ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).
				SingleLine()
			if v.err != "" {
				ui.Text(c, v.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { m.closeNewBookmarkList() })
				ui.Button(c, action).OnClick(func() { m.CreateBookmarkList() })
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeNewBookmarkList()
	}
}

// moveBookmarkDialog asks which list the chooser's bookmark moves into.
func (m *Model) moveBookmarkDialog(c *ui.Context, p *ui.Theme) {
	v := m.move
	if v == nil {
		return
	}
	open := v.open
	src := m.moveSourceList(v)
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(360).Gap(6).Children(func() {
			ui.Text(c, "移动到列表").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "选择书签的新列表：").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			for _, l := range normalizeBookmarkLists(m.bookmarkLists) {
				if l.ID == src {
					continue
				}
				id, name := l.ID, l.Name
				ui.Button(c, name).OnClick(func() { m.MoveBookmarkToList(id) })
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "新建列表…").OnClick(m.OpenNewBookmarkListForMove)
				ui.Button(c, "取消").OnClick(m.closeMoveBookmark)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeMoveBookmark()
	}
}

// bookmarkDescriptionDialog edits the highlighted popup row's description.
func (m *Model) bookmarkDescriptionDialog(c *ui.Context, p *ui.Theme) {
	v := m.editDesc
	if v == nil {
		return
	}
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(8).Children(func() {
			ui.Text(c, "编辑书签描述").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &v.text).AutoFocus()
			if in.Submitted() {
				m.SaveBookmarkDescription()
			}
			ui.Text(c, "描述显示在书签列表里；留空则显示该行文本。").FontSize(10.5).
				TextColor(p.TextMuted).SingleLine()
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { m.closeBookmarkDescription() })
				ui.Button(c, "确定").OnClick(func() { m.SaveBookmarkDescription() })
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeBookmarkDescription()
	}
}

// deleteListDialog asks before a list and its bookmarks go.
func (m *Model) deleteListDialog(c *ui.Context, p *ui.Theme) {
	v := m.deleteList
	if v == nil {
		return
	}
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(10).Children(func() {
			ui.Text(c, "删除书签列表").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, fmt.Sprintf("删除列表“%s”？其中的 %d 个书签会一并删除。", v.name, v.count)).
				FontSize(11.5).TextColor(p.TextMuted)
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "删除").OnClick(m.ConfirmDeleteBookmarkList)
				ui.Button(c, "取消").OnClick(m.closeDeleteBookmarkList)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeDeleteBookmarkList()
	}
}

// ToggleBookmarksWindow hides the tool window when it is already showing.
func (m *Model) ToggleBookmarksWindow() {
	if m.leftSplit == "bookmarks" {
		m.leftSplit = ""
		m.rememberLayout()
		m.invalidate()
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
	var jumpPath, removePath, makeDefault, renameList, deleteList string
	var moveFilePath, moveLinePath string
	var jumpLine, removeLine, moveLine = -1, -1, -1
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
			if m.rowChip(c, p, "bookmark-window:rename:"+l.ID, "重命名") {
				renameList = l.ID
			}
			if m.rowChip(c, p, "bookmark-window:deleteList:"+l.ID, "删除") {
				deleteList = l.ID
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
				label := pathBookmarkLabel(root, pb.Path)
				if pb.Dir {
					label += "/"
				}
				ui.Text(c, label).FontSize(11.5).TextColor(p.Text).
					SingleLine().Ellipsis("…")
				ui.Spacer(c)
				if m.rowChip(c, p, "bookmark-window:pathMove:"+l.ID+":"+pb.Path, "移动") {
					moveFilePath = pb.Path
				}
				if m.rowChip(c, p, "bookmark-window:pathDel:"+l.ID+":"+pb.Path, "删除") {
					removePath = pb.Path
				}
			})
			if r.Clicked() {
				if pb.Dir {
					m.ShowProject()
					if m.tree != nil {
						m.tree.reveal(pb.Path)
					}
					m.invalidate()
				} else {
					jumpPath = pb.Path
				}
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
					if m.rowChip(c, p, key+":move", "移动") {
						moveLinePath, moveLine = b.Path, b.Line
					}
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
	case renameList != "":
		m.OpenRenameBookmarkList(renameList)
	case deleteList != "":
		m.AskDeleteBookmarkList(deleteList)
	case makeDefault != "":
		m.SetDefaultBookmarkList(makeDefault)
	case moveLine >= 0:
		m.OpenMoveBookmark(moveLinePath, moveLine)
	case moveFilePath != "":
		m.OpenMoveFileBookmark(moveFilePath)
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

// renameBookmarkList renames one list; a blank, taken or unknown name comes
// back as the error text.
func renameBookmarkList(lists []BookmarkList, id, name string) ([]BookmarkList, string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return lists, "", "列表名称不能为空"
	}
	found := false
	for _, l := range lists {
		if l.ID == id {
			found = true
			continue
		}
		if l.Name == name {
			return lists, "", "已经有名为“" + name + "”的列表"
		}
	}
	if !found {
		return lists, "", "书签列表不存在"
	}
	out := append([]BookmarkList(nil), lists...)
	for i := range out {
		if out[i].ID == id {
			out[i].Name = name
		}
	}
	return out, name, ""
}

// removeBookmarkList drops a list with its bookmarks; the default moves to the
// first list left, or a fresh default when none is left.
func removeBookmarkList(lists []BookmarkList, marks []Bookmark, paths []PathBookmark,
	defaultID, id, fallback string) ([]BookmarkList, []Bookmark, []PathBookmark, string) {
	out := make([]BookmarkList, 0, len(lists))
	for _, l := range lists {
		if l.ID != id {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		out = []BookmarkList{{ID: bookmarkDefaultList, Name: fallback}}
	}
	keepMarks := make([]Bookmark, 0, len(marks))
	for _, b := range marks {
		if bookmarkListOf(b.List) != id {
			keepMarks = append(keepMarks, b)
		}
	}
	keepPaths := make([]PathBookmark, 0, len(paths))
	for _, pb := range paths {
		if bookmarkListOf(pb.List) != id {
			keepPaths = append(keepPaths, pb)
		}
	}
	def := defaultID
	kept := false
	for _, l := range out {
		if l.ID == def {
			kept = true
		}
	}
	if !kept {
		def = out[0].ID
	}
	return out, keepMarks, keepPaths, def
}

// bookmarkPopupItem is a popup row the user can open: a line or a file.
type bookmarkPopupItem struct {
	Path string
	Line int
	File bool
}

// bookmarkPopupRow is a list header or an item in the Bookmarks popup.
type bookmarkPopupRow struct {
	Header string
	Item   bookmarkPopupItem
	IsItem bool
}

// bookmarkPopupRows groups file and line bookmarks under their list, as the
// OpenIDEA popup does, leaving out lists with nothing in them.
func bookmarkPopupRows(lists []BookmarkList, marks []Bookmark, paths []PathBookmark) []bookmarkPopupRow {
	var rows []bookmarkPopupRow
	for _, l := range lists {
		lp := pathBookmarksInList(paths, l.ID)
		lm := bookmarksInList(marks, l.ID)
		if len(lp)+len(lm) == 0 {
			continue
		}
		rows = append(rows, bookmarkPopupRow{Header: l.Name})
		for _, pb := range lp {
			rows = append(rows, bookmarkPopupRow{IsItem: true,
				Item: bookmarkPopupItem{Path: pb.Path, File: true}})
		}
		for _, b := range lm {
			rows = append(rows, bookmarkPopupRow{IsItem: true,
				Item: bookmarkPopupItem{Path: b.Path, Line: b.Line}})
		}
	}
	return rows
}

// bookmarkPopupItems keeps the rows the popup can select.
func bookmarkPopupItems(rows []bookmarkPopupRow) []bookmarkPopupItem {
	out := make([]bookmarkPopupItem, 0, len(rows))
	for _, r := range rows {
		if r.IsItem {
			out = append(out, r.Item)
		}
	}
	return out
}

// bookmarkPopupRows returns the popup rows for the open project.
func (m *Model) bookmarkPopupRows() []bookmarkPopupRow {
	return bookmarkPopupRows(normalizeBookmarkLists(m.bookmarkLists), m.bookmarks, m.pathBookmarks)
}

// bookmarkPopupItems returns the popup's selectable rows for the open project.
func (m *Model) bookmarkPopupItems() []bookmarkPopupItem {
	return bookmarkPopupItems(m.bookmarkPopupRows())
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
	if v == nil || len(m.bookmarkPopupItems()) == 0 {
		return
	}
	items := m.bookmarkPopupItems()
	v.sel = clampSel(v.sel+delta, len(items))
	m.invalidate()
}

// openSelectedBookmark jumps from the popup's highlighted row.
func (m *Model) openSelectedBookmark() {
	v := m.bookmarksView
	if v == nil || len(m.bookmarkPopupItems()) == 0 {
		return
	}
	items := m.bookmarkPopupItems()
	it := items[clampSel(v.sel, len(items))]
	m.closeBookmarks()
	if it.File {
		m.openFile(it.Path)
		m.invalidate()
		return
	}
	m.applyBookmark(Bookmark{Path: it.Path, Line: it.Line})
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

// bookmarksDialog draws the Bookmarks popup: lists with their files and marked
// lines, the highlighted one Opening on Enter or a click, and a delete chip per
// row.
func (m *Model) bookmarksDialog(c *ui.Context, p *ui.Theme) {
	v := m.bookmarksView
	if v == nil {
		return
	}
	rows := m.bookmarkPopupRows()
	items := bookmarkPopupItems(rows)
	open := v.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Gap(8).Children(func() {
			ui.Text(c, "书签").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if len(items) == 0 {
				ui.Text(c, "还没有书签；在编辑器中按 F3 标记当前行。").FontSize(11).
					TextColor(p.TextMuted).SingleLine()
				return
			}
			ui.Text(c, fmt.Sprintf("%d 个书签 · Enter 打开 · Cmd+F3 关闭", len(items))).FontSize(10.5).
				TextColor(p.TextMuted).SingleLine()
			root := m.Root()
			var jump, remove = -1, -1
			item := 0
			ui.Scroll(c).Height(320).Children(func() {
				ui.Column(c).Children(func() {
					for _, r := range rows {
						r := r
						if !r.IsItem {
							ui.Row(c).Height(22).Padding(6, 8).Children(func() {
								ui.Text(c, r.Header).FontSize(10.5).Bold().
									TextColor(p.TextMuted).SingleLine()
							})
							continue
						}
						i := item
						item++
						row := ui.Row(c.Key(fmt.Sprintf("bookmark:%d", i))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == v.sel || row.Hovered() {
							row.Background(p.SurfaceHover)
						}
						row.Children(func() {
							if r.Item.File {
								ui.Text(c, pathBookmarkLabel(root, r.Item.Path)).FontSize(12).
									TextColor(p.Text).SingleLine().Ellipsis("…")
								ui.Spacer(c)
							} else {
								b, _ := bookmarkAt(m.bookmarks, r.Item.Path, r.Item.Line)
								if b.Mnemonic != "" {
									ui.Text(c, b.Mnemonic).FontSize(10.5).Bold().
										TextColor(p.Accent).SingleLine()
								}
								ui.Text(c, bookmarkTitle(b)).FontSize(12).TextColor(p.Text).
									SingleLine().Ellipsis("…")
								ui.Spacer(c)
								ui.Text(c, symbolWhere(root, b.Path, b.Line)).FontSize(10.5).
									TextColor(p.Accent).SingleLine().Ellipsis("…")
							}
							if m.rowChip(c, p, fmt.Sprintf("bookmark:del:%d", i), "删除") {
								remove = i
							}
						})
						if row.Clicked() {
							jump = i
						}
					}
				})
			})
			switch {
			case remove >= 0:
				if items[remove].File {
					m.removePathBookmark(items[remove].Path)
				} else {
					m.removeBookmark(items[remove].Path, items[remove].Line)
				}
			case jump >= 0:
				m.closeBookmarks()
				if items[jump].File {
					m.openFile(items[jump].Path)
					m.invalidate()
				} else {
					m.applyBookmark(Bookmark{Path: items[jump].Path, Line: items[jump].Line})
				}
			}
		})
	})
	if d.Dismissed() || !open {
		m.closeBookmarks()
	}
}

// bookmarkAt finds the bookmark on a line.
func bookmarkAt(list []Bookmark, path string, line int) (Bookmark, bool) {
	for _, b := range list {
		if b.Path == path && b.Line == line {
			return b, true
		}
	}
	return Bookmark{}, false
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
		// The e2e run reuses its data directory, so start from one fresh list.
		m.bookmarkLists = normalizeBookmarkLists(nil)
		m.defaultList = bookmarkDefaultList
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
		out["windowMode"] = m.leftSplit
		out["windowRows"] = len(m.bookmarks)
		m.ShowProject()
		m.ToggleFileBookmark()
		out["fileMarked"] = len(m.pathBookmarks)
		m.ShowBookmarks()
		out["popupItems"] = len(m.bookmarkPopupItems())
		m.bookmarksView.sel = 1
		m.OpenBookmarkDescription()
		m.editDesc.text = "入口"
		m.SaveBookmarkDescription()
		for _, b := range m.bookmarks {
			if b.Description == "入口" {
				out["described"] = b.Description
			}
		}
		m.closeBookmarks()
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
		// A list renames through the same dialog and deletes with its marks.
		m.OpenNewBookmarkList()
		m.newList.name = "Temp"
		m.CreateBookmarkList()
		t.state.GoTo(editor.Pos{Line: first, Col: 0})
		m.ToggleBookmark()
		out["tempMark"] = len(m.bookmarks)
		m.OpenRenameBookmarkList(m.defaultList)
		m.newList.name = "Temp 2"
		m.CreateBookmarkList()
		for _, l := range m.bookmarkLists {
			if l.ID == m.defaultList {
				out["renamed"] = l.Name
			}
		}
		out["listsBeforeDelete"] = len(m.bookmarkLists)
		m.AskDeleteBookmarkList(m.defaultList)
		m.ConfirmDeleteBookmarkList()
		out["listsAfterDelete"] = len(m.bookmarkLists)
		out["marksAfterDelete"] = len(m.bookmarks)
		// A line bookmark moves between lists through the chooser, including
		// into a list the chooser's New List… just made.
		m.OpenNewBookmarkList()
		m.newList.name = "Move A"
		m.CreateBookmarkList()
		aID := m.defaultList
		m.OpenNewBookmarkList()
		m.newList.name = "Move B"
		m.CreateBookmarkList()
		bID := m.defaultList
		t.state.GoTo(editor.Pos{Line: first, Col: 0})
		m.ToggleBookmark()
		m.OpenMoveBookmark(t.doc.Path(), first)
		m.MoveBookmarkToList(aID)
		for _, b := range m.bookmarks {
			if b.Path == t.doc.Path() && b.Line == first {
				out["movedTo"] = b.List == aID
			}
		}
		m.OpenMoveBookmark(t.doc.Path(), first)
		m.MoveBookmarkToList(bID)
		for _, b := range m.bookmarks {
			if b.Path == t.doc.Path() && b.Line == first {
				out["movedAgain"] = b.List == bID
			}
		}
		before := m.defaultList
		m.OpenMoveBookmark(t.doc.Path(), first)
		m.OpenNewBookmarkListForMove()
		m.newList.name = "Move C"
		m.CreateBookmarkList()
		cID := ""
		for _, l := range m.bookmarkLists {
			if l.Name == "Move C" {
				cID = l.ID
			}
		}
		for _, b := range m.bookmarks {
			if b.Path == t.doc.Path() && b.Line == first {
				out["movedToNew"] = b.List == cID
			}
		}
		out["moveKeptDefault"] = m.defaultList == before
		t.state.GoTo(editor.Pos{Line: first, Col: 0})
		m.ToggleBookmark()
		// A folder is a bookmark target too, as the project view's Add Bookmark
		// bookmarks the selected folder.
		folder := filepath.Dir(t.doc.Path())
		m.ToggleFolderBookmark(folder, true)
		for _, pb := range m.pathBookmarks {
			if pb.Path == folder {
				out["folderMarked"] = pb.Dir
			}
		}
		m.ShowBookmarksWindow()
		out["folderListed"] = len(pathBookmarksInList(m.pathBookmarks, m.defaultList))
		m.ToggleFolderBookmark(folder, true)
		out["folderCleared"] = len(m.pathBookmarks)
	})
	select {
	case <-done:
	case <-ctx.Done():
		return out, ctx.Err()
	}
	return out, nil
}
