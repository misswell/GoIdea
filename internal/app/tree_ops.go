package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// treeTarget is one selection of the project view, the subject of its
// context menu.
type treeTarget struct {
	path string
	dir  bool
	root bool
}

// folder is the directory an action works in: the target itself for a
// folder, its parent for a file.
func (t treeTarget) folder() string {
	if t.dir {
		return t.path
	}
	return filepath.Dir(t.path)
}

// treeAct is what choosing a project view menu item does.
type treeAct int

const (
	treeActNone treeAct = iota
	treeActSeparator
	treeActNewFile
	treeActNewDir
	treeActCut
	treeActCopy
	treeActPaste
	treeActCopyPath
	treeActJump
	treeActRename
	treeActDelete
	treeActTerminal
)

// treeMenuItem is one row of the project view menu: an action, a separator or
// a submenu, as OpenIDEA's ProjectViewPopupMenu has them.
type treeMenuItem struct {
	Act     treeAct
	Label   string
	Items   []treeMenuItem
	Enabled bool
}

// treeMenuItems is the menu of target t, as OpenIDEA orders it: New, the
// clipboard group, Jump to Source, Open in Terminal, then Rename and Delete.
// Items that cannot apply to the selection are disabled.
func treeMenuItems(t treeTarget, clipboard bool) []treeMenuItem {
	item := func(act treeAct, label string, on bool) treeMenuItem {
		return treeMenuItem{Act: act, Label: label, Enabled: on}
	}
	sep := treeMenuItem{Act: treeActSeparator}
	return []treeMenuItem{
		{Label: "新建", Items: []treeMenuItem{
			item(treeActNewFile, "文件…", true),
			item(treeActNewDir, "目录…", true),
		}},
		sep,
		item(treeActCut, "剪切", !t.root),
		item(treeActCopy, "复制", !t.root),
		{Label: "复制路径/引用", Items: []treeMenuItem{
			item(treeActCopyPath, "绝对路径", true),
			item(treeActCopyPath, "文件名", true),
			item(treeActCopyPath, "从内容根开始的路径", true),
		}},
		item(treeActPaste, "粘贴", clipboard),
		sep,
		item(treeActJump, "跳到源码", !t.dir),
		item(treeActTerminal, "在终端中打开", t.dir),
		sep,
		item(treeActRename, "重命名…", !t.root),
		item(treeActDelete, "删除…", !t.root),
	}
}

// treeMenuLabels flattens the menu to its labels, the separators dropped and
// submenus joined with a slash, for the offscreen check.
func treeMenuLabels(items []treeMenuItem) []string {
	var out []string
	for _, it := range items {
		if it.Act == treeActSeparator {
			continue
		}
		if len(it.Items) > 0 {
			out = append(out, it.Label+"/"+strings.Join(treeMenuLabels(it.Items), "/"))
			continue
		}
		out = append(out, it.Label)
	}
	return out
}

// treeMenu draws target t's context menu.
func (m *Model) treeMenu(c *ui.Context, menu *ui.Menu, t treeTarget) {
	m.buildTreeMenu(c, menu, treeMenuItems(t, m.treeClip != nil), t)
}

func (m *Model) buildTreeMenu(c *ui.Context, menu *ui.Menu, items []treeMenuItem, t treeTarget) {
	for _, it := range items {
		it := it
		switch {
		case it.Act == treeActSeparator:
			menu.Separator()
		case len(it.Items) > 0:
			menu.Submenu(it.Label, func(sub *ui.Menu) { m.buildTreeMenu(c, sub, it.Items, t) })
		case menu.Item(it.Label).Disabled(!it.Enabled).Chosen():
			m.runTreeAct(c, it, t)
		}
	}
}

func (m *Model) runTreeAct(c *ui.Context, it treeMenuItem, t treeTarget) {
	switch {
	case it.Act == treeActNewFile:
		m.OpenNewTreeEntry(t.folder(), false)
	case it.Act == treeActNewDir:
		m.OpenNewTreeEntry(t.folder(), true)
	case it.Act == treeActCut:
		m.SetTreeClip(t.path, true)
	case it.Act == treeActCopy:
		m.SetTreeClip(t.path, false)
	case it.Act == treeActPaste:
		m.PasteTreeEntry(t.folder())
	case it.Act == treeActCopyPath:
		m.CopyTreePath(c, t.path, it.Label)
	case it.Act == treeActJump:
		m.openFile(t.path)
		m.invalidate()
	case it.Act == treeActRename:
		m.OpenTreeRename(t.path)
	case it.Act == treeActDelete:
		m.AskTreeDelete(t.path, t.dir)
	case it.Act == treeActTerminal:
		m.OpenTerminalAt(t.folder())
	}
}

// treeClipState is what Copy or Cut last put on the project view's own
// clipboard, waiting for Paste.
type treeClipState struct {
	path string
	cut  bool
}

// treeOpKind is the prompt the project view shows.
type treeOpKind int

const (
	treeOpNewFile treeOpKind = iota
	treeOpNewDir
	treeOpRename
	treeOpDelete
)

// treeOpState is a New, Rename or Delete prompt of the project view.
type treeOpState struct {
	kind treeOpKind
	path string // the folder to create in, or the entry to rename or delete
	dir  bool   // path is a folder
	name string
	open bool
	err  string
	busy bool
}

// SetTreeClip remembers path for Paste, cut or copy.
func (m *Model) SetTreeClip(path string, cut bool) {
	if path == "" || path == m.Root() {
		return
	}
	m.treeClip = &treeClipState{path: path, cut: cut}
	word := "已复制 "
	if cut {
		word = "已剪切 "
	}
	m.status = word + filepath.Base(path)
	m.invalidate()
}

// OpenNewTreeEntry asks for the name of a file or folder to create in dir.
func (m *Model) OpenNewTreeEntry(dir string, asDir bool) {
	if dir == "" {
		return
	}
	kind := treeOpNewFile
	if asDir {
		kind = treeOpNewDir
	}
	m.treeOp = &treeOpState{kind: kind, path: dir, dir: true, open: true}
	m.invalidate()
}

// OpenTreeRename asks for path's new name, which keeps it in its folder.
func (m *Model) OpenTreeRename(path string) {
	if path == "" || path == m.Root() {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		m.status = "重命名失败：" + err.Error()
		m.invalidate()
		return
	}
	m.treeOp = &treeOpState{
		kind: treeOpRename, path: path, dir: info.IsDir(),
		name: filepath.Base(path), open: true,
	}
	m.invalidate()
}

// AskTreeDelete opens the confirmation that drops path.
func (m *Model) AskTreeDelete(path string, dir bool) {
	if path == "" || path == m.Root() {
		return
	}
	m.treeOp = &treeOpState{kind: treeOpDelete, path: path, dir: dir, open: true}
	m.invalidate()
}

// CloseTreeOp cancels the prompt.
func (m *Model) CloseTreeOp() {
	m.treeOp = nil
	m.invalidate()
}

// treeOpName is the trimmed name the prompt typed, with the one path
// separator a name cannot hold reported before any disk write.
func treeOpName(v *treeOpState) (string, error) {
	name := strings.TrimSpace(v.name)
	switch {
	case name == "":
		return "", errors.New("名称不能为空")
	case name == "." || name == "..":
		return "", errors.New("名称不合法")
	case strings.ContainsAny(name, `/\`):
		return "", errors.New("名称不能包含路径分隔符")
	}
	return name, nil
}

// ConfirmTreeOp applies the prompt.
func (m *Model) ConfirmTreeOp() {
	v := m.treeOp
	if v == nil || v.busy || m.Root() == "" {
		return
	}
	if v.kind == treeOpDelete {
		m.deleteTreeEntry(v)
		return
	}
	name, err := treeOpName(v)
	if err != nil {
		v.err = err.Error()
		m.invalidate()
		return
	}
	from, to := v.path, ""
	command := "fs_create_file"
	args := map[string]any{"path": filepath.Join(v.path, name)}
	switch v.kind {
	case treeOpNewDir:
		command = "fs_create_dir"
	case treeOpRename:
		from = v.path
		to = filepath.Join(filepath.Dir(v.path), name)
		command = "fs_rename"
		args = map[string]any{"from": from, "to": to}
		if to == from {
			m.CloseTreeOp()
			return
		}
	}
	target, _ := args["path"].(string)
	if target == "" {
		target = to
	}
	if _, err := os.Stat(target); err == nil {
		v.err = "已存在同名文件或目录"
		m.invalidate()
		return
	}
	if v.kind == treeOpRename {
		if bad := m.treeDirtyUnder(from); bad != "" {
			v.err = "先在编辑器中保存或关闭 " + bad
			m.invalidate()
			return
		}
	}
	v.busy = true
	v.err = ""
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, command, args)
		m.post(func() {
			v.busy = false
			if m.Root() != root {
				return
			}
			if err != nil {
				v.err = "操作失败：" + err.Error()
				m.invalidate()
				return
			}
			m.treeOp = nil
			switch v.kind {
			case treeOpNewFile:
				m.status = "已创建 " + name
				m.refreshTree(filepath.Dir(target))
				m.openFile(target)
			case treeOpNewDir:
				m.status = "已创建目录 " + name
				m.refreshTree(filepath.Dir(target))
			case treeOpRename:
				m.closedTreeTabs(from)
				m.moveBookmarkPaths(from, to)
				m.refreshTree(filepath.Dir(to))
				m.status = "已重命名为 " + name
			}
			m.invalidate()
		})
	}()
}

// deleteTreeEntry asks the backend to move path to the trash.
func (m *Model) deleteTreeEntry(v *treeOpState) {
	if bad := m.treeDirtyUnder(v.path); bad != "" {
		v.err = "先在编辑器中保存或关闭 " + bad
		m.invalidate()
		return
	}
	v.busy = true
	v.err = ""
	root := m.Root()
	path, name := v.path, filepath.Base(v.path)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "fs_delete", map[string]any{"path": path})
		m.post(func() {
			v.busy = false
			if m.Root() != root {
				return
			}
			if err != nil {
				v.err = "删除失败：" + err.Error()
				m.invalidate()
				return
			}
			m.treeOp = nil
			m.closedTreeTabs(path)
			m.dropBookmarksUnder(path)
			m.refreshTree(filepath.Dir(path))
			m.status = "已删除 " + name
			m.invalidate()
		})
	}()
}

// PasteTreeEntry copies or moves the clipboard entry into dir.
func (m *Model) PasteTreeEntry(dir string) {
	clip := m.treeClip
	if clip == nil || dir == "" || m.Root() == "" {
		return
	}
	to := filepath.Join(dir, filepath.Base(clip.path))
	if to == clip.path {
		m.status = "已经在该目录中"
		m.invalidate()
		return
	}
	if _, err := os.Stat(to); err == nil {
		m.status = "目标中已存在 " + filepath.Base(to)
		m.invalidate()
		return
	}
	command, args := "fs_copy", map[string]any{"from": clip.path, "to": to}
	if clip.cut {
		command, args = "fs_rename", map[string]any{"from": clip.path, "to": to}
	}
	if clip.cut {
		if bad := m.treeDirtyUnder(clip.path); bad != "" {
			m.status = "先在编辑器中保存或关闭 " + bad
			m.invalidate()
			return
		}
	}
	root := m.Root()
	from := clip.path
	m.status = "正在粘贴 " + filepath.Base(from) + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, command, args)
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.status = "粘贴失败：" + err.Error()
				m.invalidate()
				return
			}
			if clip.cut {
				m.treeClip = nil
				m.closedTreeTabs(from)
				m.moveBookmarkPaths(from, to)
			}
			m.refreshTree(dir)
			m.status = "已粘贴 " + filepath.Base(to)
			m.invalidate()
		})
	}()
}

// CopyTreePath puts the absolute path, the file name or the path from the
// project root on the system clipboard.
func (m *Model) CopyTreePath(c *ui.Context, path, mode string) {
	if c == nil || path == "" {
		return
	}
	text := path
	switch mode {
	case "文件名":
		text = filepath.Base(path)
	case "从内容根开始的路径":
		if rel, err := filepath.Rel(m.Root(), path); err == nil {
			text = rel
		}
	}
	c.WriteClipboard(text)
	m.status = "已复制 " + text
	m.invalidate()
}

// refreshTree rereads the project view and opens dir so a new entry shows.
func (m *Model) refreshTree(dir string) {
	if m.tree == nil {
		return
	}
	m.tree.Refresh()
	m.tree.reveal(dir)
}

// treeTabsUnder lists the open tabs of path or of a file inside it.
func (m *Model) treeTabsUnder(path string) []*Tab {
	var out []*Tab
	for _, t := range m.tabs {
		if underTreePath(t.doc.Path(), path) {
			out = append(out, t)
		}
	}
	return out
}

// treeDirtyUnder names the first unsaved tab of path, "" when none.
func (m *Model) treeDirtyUnder(path string) string {
	for _, t := range m.treeTabsUnder(path) {
		if t.unsaved() {
			return filepath.Base(t.doc.Path())
		}
	}
	return ""
}

// closedTreeTabs closes the open tabs of path after it moved or went away.
// The caller has already refused when one of them was unsaved.
func (m *Model) closedTreeTabs(path string) {
	for _, t := range m.treeTabsUnder(path) {
		m.removeTab(t)
	}
}

// underTreePath reports whether p is path or inside it.
func underTreePath(p, path string) bool {
	return p == path || strings.HasPrefix(p, path+string(filepath.Separator))
}

// moveBookmarkPaths follows a rename, so bookmarks of the old path still
// reach the entry.
func (m *Model) moveBookmarkPaths(from, to string) {
	if from == to {
		return
	}
	follow := func(p string) string {
		if p == from {
			return to
		}
		if strings.HasPrefix(p, from+string(filepath.Separator)) {
			return to + p[len(from):]
		}
		return p
	}
	changed := false
	for i := range m.bookmarks {
		if next := follow(m.bookmarks[i].Path); next != m.bookmarks[i].Path {
			m.bookmarks[i].Path = next
			changed = true
		}
	}
	for i := range m.pathBookmarks {
		if next := follow(m.pathBookmarks[i].Path); next != m.pathBookmarks[i].Path {
			m.pathBookmarks[i].Path = next
			changed = true
		}
	}
	if changed {
		m.saveSession()
		m.refreshDecorations()
	}
}

// dropBookmarksUnder forgets the bookmarks of a deleted entry.
func (m *Model) dropBookmarksUnder(path string) {
	marks := m.bookmarks[:0]
	for _, b := range m.bookmarks {
		if !underTreePath(b.Path, path) {
			marks = append(marks, b)
		}
	}
	paths := m.pathBookmarks[:0]
	for _, pb := range m.pathBookmarks {
		if !underTreePath(pb.Path, path) {
			paths = append(paths, pb)
		}
	}
	m.bookmarks, m.pathBookmarks = marks, paths
	m.saveSession()
	m.refreshDecorations()
}

// SmokeFiles drives the project view's New, Rename, Copy, Cut and Delete
// against the real backend, on a folder it removes at the end.
func (m *Model) SmokeFiles(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("没有打开项目")
	}
	base := filepath.Join(root, "goidea-tree-smoke")
	_ = os.RemoveAll(base)
	out := map[string]any{
		"menu": treeMenuLabels(treeMenuItems(treeTarget{path: root, dir: true, root: true}, false)),
	}
	run := func(fn func()) {
		done := make(chan struct{})
		m.post(func() { defer close(done); fn() })
		<-done
	}
	wait := func(path string, want bool, what string) error {
		deadline := time.Now().Add(30 * time.Second)
		for {
			_, err := os.Stat(path)
			if (err == nil) == want {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s 超时", what)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	create := func(dir, name string, asDir bool) error {
		run(func() {
			m.OpenNewTreeEntry(dir, asDir)
			if m.treeOp != nil {
				m.treeOp.name = name
			}
			m.ConfirmTreeOp()
		})
		return wait(filepath.Join(dir, name), true, "新建 "+name)
	}
	rename := func(path, name string) error {
		run(func() {
			m.OpenTreeRename(path)
			if m.treeOp != nil {
				m.treeOp.name = name
			}
			m.ConfirmTreeOp()
		})
		if err := wait(path, false, "重命名前的 "+name); err != nil {
			return err
		}
		return wait(filepath.Join(filepath.Dir(path), name), true, "重命名 "+name)
	}
	paste := func(from, dir string, cut bool) error {
		run(func() {
			m.SetTreeClip(from, cut)
			m.PasteTreeEntry(dir)
		})
		return wait(filepath.Join(dir, filepath.Base(from)), true, "粘贴 "+filepath.Base(from))
	}
	drop := func(path string, dir bool) error {
		run(func() {
			m.AskTreeDelete(path, dir)
			m.ConfirmTreeOp()
		})
		return wait(path, false, "删除 "+filepath.Base(path))
	}

	if err := create(root, filepath.Base(base), true); err != nil {
		return out, err
	}
	out["newDir"] = true
	one := filepath.Join(base, "one.txt")
	if err := create(base, "one.txt", false); err != nil {
		return out, err
	}
	out["newFile"] = true
	two := filepath.Join(base, "two.txt")
	if err := rename(one, "two.txt"); err != nil {
		return out, err
	}
	out["renamed"] = true
	if err := paste(two, root, false); err != nil {
		return out, err
	}
	out["copied"] = true
	if err := drop(filepath.Join(root, "two.txt"), false); err != nil {
		return out, err
	}
	out["deletedFile"] = true
	if err := paste(two, root, true); err != nil {
		return out, err
	}
	out["moved"] = true
	if err := paste(filepath.Join(root, "two.txt"), base, true); err != nil {
		return out, err
	}
	if err := drop(two, false); err != nil {
		return out, err
	}
	if err := drop(base, true); err != nil {
		return out, err
	}
	out["deletedDir"] = true
	return out, nil
}

// treeOpDialog draws the project view's New, Rename or Delete prompt.
func (m *Model) treeOpDialog(c *ui.Context, p *ui.Theme) {
	v := m.treeOp
	if v == nil {
		return
	}
	open := v.open
	title, hint, action := "新建文件", "新文件在项目视图中创建，并在这里打开。", "创建"
	switch v.kind {
	case treeOpNewDir:
		title, hint, action = "新建目录", "新目录在项目视图中创建。", "创建"
	case treeOpRename:
		title, hint, action = "重命名", "新名称替代原名，条目仍在同一目录中。", "重命名"
	case treeOpDelete:
		title = "删除"
	}
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if v.kind == treeOpDelete {
				ui.Text(c, fmt.Sprintf("把“%s”移到废纸篓？", filepath.Base(v.path))).
					FontSize(11.5).TextColor(p.TextMuted)
			} else {
				in := ui.TextInput(c, &v.name).AutoFocus()
				in.OnChange(func() { v.err = "" })
				if in.Submitted() {
					m.ConfirmTreeOp()
				}
				ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
			if v.err != "" {
				ui.Text(c, v.err).FontSize(11).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { m.CloseTreeOp() })
				if v.kind == treeOpDelete {
					ui.Button(c, "删除").Disabled(v.busy).OnClick(func() { m.ConfirmTreeOp() })
				} else {
					ui.Button(c, action).Disabled(v.busy).OnClick(func() { m.ConfirmTreeOp() })
				}
			})
		})
	})
	if d.Dismissed() || !open {
		m.CloseTreeOp()
	}
}
