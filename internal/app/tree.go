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
	// excluded marks build output and tool folders the tree shows shaded,
	// as OpenIDEA shades target/build; module marks a Maven or Gradle module
	// folder, whose name is bold.
	excluded bool
	module   bool
	// package is a compacted package row (com.example), drawn with the
	// package icon rather than a folder.
	packageDir bool
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

// RefreshDir rereads dir and the folders below it the tree read, as Reload
// from Disk does for the selected folder.
func (t *Tree) RefreshDir(dir string) {
	if t.ws == nil {
		return
	}
	t.ws.RefreshDir(dir)
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
			leaf := p
			pkg := false
			if dir && inPackageTree(p) {
				if joined, deep, ok := t.packageChain(p); ok {
					name, leaf, pkg = joined, deep, true
				}
			}
			walk(&treeNode{path: leaf, name: name, dir: dir, depth: n.depth + 1,
				expanded: t.open[leaf], excluded: dir && excludedDir(name), module: dir && t.isModule(leaf, name),
				packageDir: pkg})
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

// packageChain joins the package directories below a source root: while a
// directory holds exactly one visible entry and that entry is a directory, the
// chain draws as one dotted row, as OpenIDEA's tree shows
// com.example.app on one line. The row's path is the deepest directory, so
// opening the row reads the packages under it.
func (t *Tree) packageChain(dir string) (name, leaf string, ok bool) {
	if t.ws == nil || !t.ws.IsDir(dir) {
		return "", dir, false
	}
	base := filepath.Base(dir)
	if !t.ws.Loaded(dir) {
		t.ws.Load(dir)
		return "", dir, false
	}
	kids := t.visibleChildren(dir)
	if len(kids) != 1 || !t.ws.IsDir(kids[0]) {
		return "", dir, false
	}
	if deeper, deepLeaf, ok := t.packageChain(kids[0]); ok {
		return base + "." + deeper, deepLeaf, true
	}
	return base + "." + filepath.Base(kids[0]), kids[0], true
}

// visibleChildren lists a directory's entries the tree draws.
func (t *Tree) visibleChildren(dir string) []string {
	var out []string
	for _, p := range t.ws.Children(dir) {
		name := filepath.Base(p)
		if hidden(name, t.ws.IsDir(p)) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// isSourceRoot reports whether path is a source root of the project: a
// language folder under src/<configuration>/ (src/main/java and siblings).
func isSourceRoot(path string) bool {
	base := filepath.Base(path)
	switch base {
	case "java", "kotlin", "groovy", "scala":
	default:
		return false
	}
	p := filepath.ToSlash(path)
	for _, root := range []string{"/src/main/", "/src/test/", "/src/integrationTest/"} {
		if strings.Contains(p, root) {
			return true
		}
	}
	return false
}

// inPackageTree reports whether path is a package below a source root
// (src/main/java and its siblings): only those chains compact, never logs,
// src or other plain folders.
func inPackageTree(path string) bool {
	p := filepath.ToSlash(path)
	for _, root := range []string{
		"/src/main/java", "/src/test/java",
		"/src/main/kotlin", "/src/test/kotlin",
		"/src/main/groovy", "/src/test/groovy",
		"/src/main/scala", "/src/test/scala",
	} {
		if strings.Contains(p, root+"/") {
			return true
		}
	}
	return false
}

// hidden reports whether the tree leaves a name out. OpenIDEA shows most
// dotfiles and the build output; only VCS metadata and system files are gone
// (build output draws shaded, through excludedDir).
func hidden(name string, dir bool) bool {
	switch name {
	case ".git", ".hg", ".svn", ".DS_Store", "Thumbs.db":
		return true
	}
	return false
}

// excludedDir reports whether a shown folder is build output or tool state,
// which IDEs dim and shade: "excluded" content.
func excludedDir(name string) bool {
	return hiddenDirs[name] || name == ".idea" || name == ".vscode" || name == ".openidea"
}

// isModule reports whether a folder is a Maven or Gradle module: it has a
// pom.xml or a build.gradle(.kts) beside its other entries.
func (t *Tree) isModule(path, name string) bool {
	if !t.ws.Loaded(path) {
		return false
	}
	for _, p := range t.ws.Children(path) {
		switch filepath.Base(p) {
		case "pom.xml", "build.gradle", "build.gradle.kts":
			return true
		}
	}
	return false
}
