package app

import (
	"path/filepath"
	"strings"

	"mayoi/internal/workspace"
)

// hiddenDirs are the folders the tree leaves out on top of what Mayoi's
// workspace hides: build output, caches and tool state that a Java project
// never wants to browse.
var hiddenDirs = map[string]bool{
	".idea": true, ".mvn": true, "target": true, ".gradle": true,
	"build": true, "out": true, "bin": true, "node_modules": true, ".qoder": true,
}

// treeNode is one row of the file tree: a folder whose children load when it
// first opens, or a file.
type treeNode struct {
	path     string
	name     string
	dir      bool
	depth    int
	expanded bool
}

// Tree is the project's file tree, with the rows visible in it. It is a view
// over Mayoi's workspace.Tree, which reads a directory at a time off the main
// thread and follows changes on disk. It is used on the main thread only.
type Tree struct {
	ws   *workspace.Tree
	root *treeNode
	rows []*treeNode
	open map[string]bool // folders expanded, by path
	// expandAll is the folder Expand All is still opening: children arrive one
	// read at a time, so rebuild keeps opening what each read reveals until
	// nothing below it is left closed.
	expandAll string
	sel       string // the row revealed or picked, drawn as the selection
}

// NewTree returns the tree of the folder root, with the root open. post runs a
// function on the main thread, as the workspace does; changed draws the window
// again after files change on disk.
func NewTree(root string, post func(func()), changed func()) *Tree {
	t := &Tree{open: map[string]bool{}}
	if root == "" || post == nil {
		return t
	}
	t.root = &treeNode{path: root, name: filepath.Base(root), dir: true, expanded: true}
	t.open[root] = true
	// Children arrive through post after a background read: rebuild and
	// redraw then, or the rows of a folder read late would wait for the
	// next click.
	t.ws = workspace.NewTree(root, func(fn func()) {
		post(func() {
			fn()
			t.rebuild()
			if changed != nil {
				changed()
			}
		})
	})
	t.ws.OnChange = func(_, _ []string) {
		t.rebuild()
		if changed != nil {
			changed()
		}
	}
	t.ws.Load(root)
	t.rebuild()
	return t
}

// Close stops the tree watching the folder.
func (t *Tree) Close() {
	if t.ws != nil {
		t.ws.Close()
		t.ws = nil
	}
}

// Refresh rereads the open folders, as a new or renamed entry asks.
func (t *Tree) Refresh() {
	if t.ws == nil {
		return
	}
	t.ws.Refresh()
	t.rebuild()
}

// Rows are the visible rows, each already flattened to its depth.
func (t *Tree) Rows() []*treeNode { return t.rows }

// Toggle opens or closes the folder at path.
func (t *Tree) Toggle(path string) {
	if t.ws == nil || !t.ws.IsDir(path) {
		return
	}
	t.expandAll = ""
	if t.open[path] {
		delete(t.open, path)
	} else {
		t.open[path] = true
		t.ws.Load(path)
	}
	t.rebuild()
}

// ExpandAll opens dir and every folder below it. A folder is read when it
// first opens, so the pass carries on through the reads' own rebuilds until
// nothing below dir is left closed.
func (t *Tree) ExpandAll(dir string) {
	if t.ws == nil || dir == "" {
		return
	}
	t.open[dir] = true
	t.ws.Load(dir)
	t.expandAll = dir
	t.rebuild()
}

// CollapseAll closes every folder but the root, as OpenIDEA's Collapse All
// does.
func (t *Tree) CollapseAll() {
	if t.ws == nil || t.root == nil {
		return
	}
	t.expandAll = ""
	for p := range t.open {
		delete(t.open, p)
	}
	t.open[t.root.path] = true
	t.rebuild()
}

// Select marks path as the tree's selection, so a reveal from the editor
// leaves a visible row.
func (t *Tree) Select(path string) { t.sel = path }

// Selected is the row Select or reveal last named.
func (t *Tree) Selected() string { return t.sel }

// IsDir reports whether the named row is a folder among the tree's rows.
func (t *Tree) IsDir(path string) bool {
	for _, r := range t.rows {
		if r.path == path {
			return r.dir
		}
	}
	return false
}

// reveal opens the folders leading to path, as a file opened from elsewhere.
func (t *Tree) reveal(path string) {
	if t.ws == nil || t.root == nil {
		return
	}
	rel, err := filepath.Rel(t.root.path, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}
	dir := t.root.path
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		if dir == path {
			break
		}
		// A folder the workspace has not read yet is not known to be
		// one; it opens here and its rows arrive when it is read.
		t.open[dir] = true
		t.ws.Load(dir)
	}
	t.open[t.root.path] = true
	t.rebuild()
}

// rebuild flattens the open folders into rows, skipping what hidden reports.
func (t *Tree) rebuild() {
	t.rows = t.rows[:0]
	if t.root == nil {
		return
	}
	if t.expandAll != "" {
		t.openUnder(t.expandAll)
	}
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		t.rows = append(t.rows, n)
		if !n.dir || !t.open[n.path] || t.ws == nil {
			return
		}
		t.ws.Load(n.path)
		for _, p := range t.ws.Children(n.path) {
			name := filepath.Base(p)
			dir := t.ws.IsDir(p)
			if hidden(name, dir) {
				continue
			}
			walk(&treeNode{path: p, name: name, dir: dir, depth: n.depth + 1, expanded: t.open[p]})
		}
	}
	walk(t.root)
	if t.expandAll != "" && t.expandSettled(t.expandAll) {
		t.expandAll = ""
	}
}

// openUnder opens every folder the workspace already knows below dir, so the
// next read of one of them widens the tree. It stops once nothing below dir
// is left closed.
func (t *Tree) openUnder(dir string) {
	if !t.open[dir] || t.ws.Loading(dir) || !t.ws.Loaded(dir) {
		return
	}
	for _, p := range t.ws.Children(dir) {
		if !t.ws.IsDir(p) || hidden(filepath.Base(p), true) {
			continue
		}
		if !t.open[p] {
			t.open[p] = true
			t.ws.Load(p)
		}
		t.openUnder(p)
	}
}

// expandSettled reports whether every folder below dir is open and read.
func (t *Tree) expandSettled(dir string) bool {
	if !t.open[dir] || t.ws.Loading(dir) || !t.ws.Loaded(dir) {
		return false
	}
	for _, p := range t.ws.Children(dir) {
		if !t.ws.IsDir(p) || hidden(filepath.Base(p), true) {
			continue
		}
		if !t.expandSettled(p) {
			return false
		}
	}
	return true
}

// hidden reports whether the tree leaves a name out: tool folders and their
// caches, and the dot files an editor or the system writes.
func hidden(name string, dir bool) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	return dir && hiddenDirs[name]
}
