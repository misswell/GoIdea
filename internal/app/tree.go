package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hiddenDirs are the folders the tree leaves out: build output, caches and
// tool state that a Java project never wants to browse.
var hiddenDirs = map[string]bool{
	".git": true, ".idea": true, ".gradle": true, ".mvn": true,
	"target": true, "build": true, "out": true, "bin": true,
	"node_modules": true, ".DS_Store": true, ".qoder": true,
}

// treeNode is one row of the file tree: a folder whose children load when it
// first opens, or a file.
type treeNode struct {
	path     string
	name     string
	dir      bool
	depth    int
	expanded bool
	loaded   bool
	children []*treeNode
}

// Tree is the project's file tree, with the rows visible in it. It is used on
// the main thread only.
type Tree struct {
	root *treeNode
	rows []*treeNode
}

// NewTree returns the tree of the folder root, with the root open.
func NewTree(root string) *Tree {
	t := &Tree{}
	if root == "" {
		return t
	}
	t.root = &treeNode{path: root, name: filepath.Base(root), dir: true, expanded: true}
	t.load(t.root)
	t.rebuild()
	return t
}

// Rows are the visible rows, each already flattened to its depth.
func (t *Tree) Rows() []*treeNode { return t.rows }

// Toggle opens or closes the folder at path.
func (t *Tree) Toggle(path string) {
	n := t.find(path)
	if n == nil || !n.dir {
		return
	}
	n.expanded = !n.expanded
	if n.expanded && !n.loaded {
		t.load(n)
	}
	t.rebuild()
}

// reveal opens the folders leading to path, as a file opened from elsewhere.
func (t *Tree) reveal(path string) {
	if t.root == nil || !strings.HasPrefix(path, t.root.path+string(filepath.Separator)) {
		return
	}
	rel, err := filepath.Rel(t.root.path, path)
	if err != nil {
		return
	}
	parts := strings.Split(rel, string(filepath.Separator))
	cur := t.root
	for i, part := range parts {
		if i == len(parts)-1 {
			break
		}
		var next *treeNode
		for _, c := range cur.children {
			if c.name == part {
				next = c
				break
			}
		}
		if next == nil || !next.dir {
			return
		}
		if !next.loaded {
			t.load(next)
		}
		next.expanded = true
		cur = next
	}
	t.rebuild()
}

func (t *Tree) find(path string) *treeNode {
	var walk func(n *treeNode) *treeNode
	walk = func(n *treeNode) *treeNode {
		if n.path == path {
			return n
		}
		for _, c := range n.children {
			if found := walk(c); found != nil {
				return found
			}
		}
		return nil
	}
	if t.root == nil {
		return nil
	}
	return walk(t.root)
}

// load reads the direct children of an open folder.
func (t *Tree) load(n *treeNode) {
	n.loaded = true
	entries, err := os.ReadDir(n.path)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		dir := e.IsDir()
		if dir && hiddenDirs[name] {
			continue
		}
		n.children = append(n.children, &treeNode{
			path:  filepath.Join(n.path, name),
			name:  name,
			dir:   dir,
			depth: n.depth + 1,
		})
	}
	sort.Slice(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if a.dir != b.dir {
			return a.dir
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
}

func (t *Tree) rebuild() {
	t.rows = t.rows[:0]
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		t.rows = append(t.rows, n)
		if !n.dir || !n.expanded {
			return
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	if t.root != nil {
		walk(t.root)
	}
}
