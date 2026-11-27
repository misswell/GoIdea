package app

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"mayoi/internal/openidea"
	"mayoi/internal/workspace"
)

// hiddenDirs are the folders the tree leaves out on top of what Mayoi's
// workspace hides: build output, caches and tool state that a Java project
// never wants to browse.
var hiddenDirs = map[string]bool{
	".mvn": true, "target": true, ".gradle": true,
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
	// group is a contributed root such as Scratches and Consoles, and
	// readOnly marks a contributed tree that project operations must not
	// edit. detail is the faint text after a root or member label.
	group    bool
	readOnly bool
	detail   string
	// icon overrides the file/folder icon for a contributed row. external
	// marks a library tree row whose directories come from fs_read_dir, and
	// children are prebuilt contributed children such as SDK groups.
	icon     string
	external bool
	children []*treeNode
	// excluded marks build output and tool folders the tree shows shaded,
	// as OpenIDEA shades target/build; module marks a Maven or Gradle module
	// folder, whose name is bold.
	excluded bool
	module   bool
	// package is a compacted package row (com.example), drawn with the
	// package icon rather than a folder.
	packageDir bool
	// member is a declaration shown below an expanded file. path is the
	// file that contains it, line and col are its selection position.
	member      bool
	kind        string
	line        int
	col         int
	hasChildren bool
}

// treeOptions are the parts of Project View that change its model rather
// than its interaction. They mirror OpenIDEA's TreeOptions.
type treeOptions struct {
	showHidden         bool
	showExcluded       bool
	showMembers        bool
	compact            bool
	foldersFirst       bool
	sortBy             string
	showRootPaths      bool
	unityRoot          string
	showUnityMeta      bool
	showUnityGenerated bool
}

func defaultTreeOptions() treeOptions {
	return treeOptions{
		showExcluded:  true,
		compact:       true,
		foldersFirst:  true,
		sortBy:        "name",
		showRootPaths: true,
	}
}

func treeOptionsFromPrefs(p ProjectTreePrefs) treeOptions {
	return treeOptions{
		showHidden:         p.ShowHidden,
		showExcluded:       p.showsExcluded(),
		showMembers:        p.ShowMembers,
		compact:            p.compacts(),
		foldersFirst:       p.foldersFirst(),
		sortBy:             p.normalizedSort(),
		showRootPaths:      p.showsRootPaths(),
		showUnityMeta:      p.ShowUnityMeta,
		showUnityGenerated: p.ShowUnityGenerated,
	}
}

// Tree is the project's file tree, with the rows visible in it. It is a view
// over Mayoi's workspace.Tree, which reads a directory at a time off the main
// thread and follows changes on disk. It is used on the main thread only.
type Tree struct {
	ws      *workspace.Tree
	root    *treeNode
	rows    []*treeNode
	open    map[string]bool // folders expanded, by path
	post    func(func())
	changed func()
	removed func([]string)
	// expandAll is the folder Expand All is still opening: children arrive one
	// read at a time, so rebuild keeps opening what each read reveals until
	// nothing below it is left closed.
	expandAll string
	sel       string // the row revealed or picked, drawn as the selection
	// sels and anchor carry a multi-selection made with Cmd/Ctrl-click and
	// Shift-click. sel remains the lead row whose context menu and preview
	// drive single-item actions.
	sels   map[string]bool
	anchor string

	// mode is OpenIDEA's Project View pane. Flat modes list editor paths
	// rather than the workspace, and Scratches is a contributed root.
	mode                string
	options             treeOptions
	fileIgnores         []string
	fileExcludes        workspace.Globs
	fileExcludePatterns []string
	openFiles           []string
	changedFiles        []string
	scratchRoot         string
	extraTrees          map[string]*workspace.Tree
	virtualDirs         map[string]bool
	// projectRoots is a workspace manifest's top-level projects. multiRoot
	// distinguishes an empty workspace from an ordinary single-folder
	// project, which keeps its primary root visible.
	projectRoots []string
	multiRoot    bool
	// collapsed remembers folders the user closed in a flat pane, so a
	// file change does not auto-expand them again.
	collapsed map[string]bool
	// outlines cache Show Members rows by file path. An empty slice records
	// that the backend had no members for the file.
	outlines map[string][]openidea.OutlineItem
	// expandedChanged persists a project's expanded folders.
	expandedChanged func([]string)

	// remoteLoader reads one folder through fs_read_dir instead of the local
	// workspace. Remote entries arrive off the main thread, so SSH and WSL
	// paths never pass through os.Stat.
	remoteLoader  func(dir string, done func([]openidea.RemoteDirectoryEntry, error))
	remoteEntries map[string][]openidea.RemoteDirectoryEntry
	remoteLoading map[string]bool
	remoteErrs    map[string]string
	remoteClosed  bool

	// externalRoot is OpenIDEA's External Libraries contribution. Its group
	// nodes come from sdk_project_libraries, while roots and nested entries
	// are read through the backend VFS so archives do not touch the host.
	externalRoot    *treeNode
	externalLoader  func(string, func([]openidea.RemoteDirectoryEntry, error))
	externalEntries map[string][]openidea.RemoteDirectoryEntry
	externalLoading map[string]bool
	externalErrs    map[string]string
	externalDirs    map[string]bool
	externalVersion map[string]uint64
	externalClosed  bool
}

// NewTree returns the tree of the folder root, with the root open. post runs a
// function on the main thread, as the workspace does; changed draws the window
// again after files change on disk.
func NewTree(root string, post func(func()), changed func()) *Tree {
	t := &Tree{
		post:                post,
		changed:             changed,
		open:                map[string]bool{},
		sels:                map[string]bool{},
		mode:                "project",
		options:             defaultTreeOptions(),
		fileIgnores:         cloneStrings(defaultIgnoredNames),
		fileExcludes:        workspace.ParseGlobs(strings.Join(defaultFileExcludes, ",")),
		fileExcludePatterns: cloneStrings(defaultFileExcludes),
		collapsed:           map[string]bool{},
		outlines:            map[string][]openidea.OutlineItem{},
		extraTrees:          map[string]*workspace.Tree{},
		virtualDirs:         map[string]bool{},
		remoteEntries:       map[string][]openidea.RemoteDirectoryEntry{},
		remoteLoading:       map[string]bool{},
		remoteErrs:          map[string]string{},
		externalEntries:     map[string][]openidea.RemoteDirectoryEntry{},
		externalLoading:     map[string]bool{},
		externalErrs:        map[string]string{},
		externalDirs:        map[string]bool{},
		externalVersion:     map[string]uint64{},
	}
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
	t.ws.IncludeHidden = true
	t.ws.OnChange = t.workspaceChanged
	t.ws.Load(root)
	t.rebuild()
	return t
}

// SetRemovedHandler reports paths deleted outside the app on the main thread.
func (t *Tree) SetRemovedHandler(fn func([]string)) { t.removed = fn }

func (t *Tree) workspaceChanged(changed, removed []string) {
	t.rebuild()
	if t.removed != nil && len(removed) > 0 {
		t.removed(removed)
	}
	if t.changed != nil {
		t.changed()
	}
}

// Close stops the tree watching the folder.
func (t *Tree) Close() {
	t.remoteClosed = true
	t.externalClosed = true
	for filePath := range t.externalVersion {
		t.externalVersion[filePath]++
	}
	if t.ws != nil {
		t.ws.Close()
		t.ws = nil
	}
	for root, tree := range t.extraTrees {
		tree.Close()
		delete(t.extraTrees, root)
	}
}

func (t *Tree) addExtraTree(root string) {
	if root == "" || t.post == nil {
		return
	}
	tree := workspace.NewTree(root, func(fn func()) {
		t.post(func() {
			fn()
			t.rebuild()
			if t.changed != nil {
				t.changed()
			}
		})
	})
	tree.OnChange = t.workspaceChanged
	tree.IncludeHidden = true
	t.extraTrees[root] = tree
	tree.Load(root)
}

func (t *Tree) workspaceFor(path string) *workspace.Tree {
	path = filepath.Clean(path)
	for root, tree := range t.extraTrees {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return tree
		}
	}
	return t.ws
}

func (t *Tree) rootForPath(path string) string {
	if t.remoteLoader != nil {
		root := t.remoteRoot()
		path = filepath.Clean(path)
		if root != "" && (path == root || strings.HasPrefix(path, root+"/")) {
			return root
		}
		return ""
	}
	path = filepath.Clean(path)
	if t.ws != nil {
		root := filepath.Clean(t.ws.Root)
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return root
		}
	}
	for root := range t.extraTrees {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return root
		}
	}
	return ""
}

func (t *Tree) isDir(path string) bool {
	path = filepath.Clean(path)
	if t.externalDirs[path] {
		return true
	}
	if entries, ok := t.externalEntries[path]; ok {
		return entries != nil || t.externalLoading[path]
	}
	for _, entries := range t.externalEntries {
		for _, entry := range entries {
			if entry.Path == path {
				return entry.IsDir
			}
		}
	}
	if t.virtualDirs[path] {
		return true
	}
	if t.remoteLoader != nil {
		if entries, ok := t.remoteEntries[path]; ok {
			return entries != nil || t.remoteLoading[path]
		}
		for _, entries := range t.remoteEntries {
			for _, entry := range entries {
				if entry.Path == path {
					return entry.IsDir
				}
			}
		}
		return path == t.remoteRoot()
	}
	if tree := t.workspaceFor(path); tree != nil && tree.IsDir(path) {
		return true
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (t *Tree) expansionChanged() {
	if t.expandedChanged != nil {
		t.expandedChanged(t.Expanded())
	}
}

// Mode returns OpenIDEA's Project View pane id.
func (t *Tree) Mode() string {
	if t.mode == "" {
		return "project"
	}
	return t.mode
}

// SetMode switches the pane and rebuilds the visible rows.
func (t *Tree) SetMode(mode string) {
	switch mode {
	case "project", "projectFiles", "openFiles", "changedFiles", "scratches", "unity":
	default:
		mode = "project"
	}
	if t.mode == mode {
		return
	}
	t.mode = mode
	t.expandAll = ""
	if mode == "openFiles" || mode == "changedFiles" {
		t.expandFlatParents()
	}
	t.rebuild()
}

// SetOptions applies Project View's appearance and ordering options.
func (t *Tree) SetOptions(options treeOptions) {
	if t.options == options {
		return
	}
	hiddenChanged := t.options.showHidden != options.showHidden
	t.options = options
	if hiddenChanged {
		if t.ws != nil {
			t.ws.IncludeHidden = true
			t.ws.Refresh()
		}
		for _, tree := range t.extraTrees {
			tree.IncludeHidden = true
			tree.Refresh()
		}
	}
	t.rebuild()
}

// SetFileFilters applies files.ignoredNames and files.exclude to the Project
// View. Ignored names follow Show Hidden Files; exclude patterns follow Show
// Excluded Files and dim their rows while that option is on.
func (t *Tree) SetFileFilters(ignoredNames, exclude []string) {
	ignoredNames = cloneStrings(ignoredNames)
	exclude = cloneStrings(exclude)
	if sameStrings(t.fileIgnores, ignoredNames) &&
		sameStrings(t.fileExcludePatterns, exclude) {
		return
	}
	t.fileIgnores = ignoredNames
	t.fileExcludes = workspace.ParseGlobs(strings.Join(exclude, ","))
	t.fileExcludePatterns = exclude
	t.rebuild()
}

// SetExpanded restores a project's open folders and keeps the project root
// open, as OpenIDEA restores its expansion state per project.
func (t *Tree) SetExpanded(paths []string) {
	clear(t.open)
	if t.ws != nil && t.ws.Root != "" {
		t.open[t.ws.Root] = true
	}
	if t.multiRoot {
		for _, root := range t.projectRoots {
			t.open[root] = true
		}
	}
	for _, path := range paths {
		if path != "" && !t.collapsed[path] {
			t.open[path] = true
		}
	}
	t.rebuild()
}

// Expanded returns the folders the user has open, for the session.
func (t *Tree) Expanded() []string {
	out := make([]string, 0, len(t.open))
	for path, open := range t.open {
		if open {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	if len(out) > maxProjectTreeExpanded {
		out = out[:maxProjectTreeExpanded]
	}
	return out
}

// SetExpandedChanged installs the callback that saves a project's expansion.
func (t *Tree) SetExpandedChanged(fn func([]string)) { t.expandedChanged = fn }

const maxProjectTreeExpanded = 3000

// SetFilteredFiles supplies the paths Open Files and All Changed Files list.
func (t *Tree) SetFilteredFiles(openFiles, changedFiles []string) {
	if equalProjectTreePaths(t.openFiles, openFiles) &&
		equalProjectTreePaths(t.changedFiles, changedFiles) {
		return
	}
	t.openFiles = append(t.openFiles[:0], openFiles...)
	t.changedFiles = append(t.changedFiles[:0], changedFiles...)
	if t.Mode() == "openFiles" || t.Mode() == "changedFiles" {
		t.expandFlatParents()
	}
	t.rebuild()
}

// SetScratchRoot adds the contributed Scratches and Consoles root. The root
// has its own workspace watcher because it lives outside the project.
func (t *Tree) SetScratchRoot(root string) {
	root = filepath.Clean(root)
	if root == "." || root == "" {
		root = ""
	}
	if root == t.scratchRoot {
		return
	}
	t.scratchRoot = root
	if root == "" {
		t.rebuild()
		return
	}
	if _, ok := t.extraTrees[root]; !ok {
		t.addExtraTree(root)
	}
	t.open[root] = true
	t.rebuild()
}

// SetProjectRoots applies a workspace manifest to the Project and Project
// Files views. OpenIDEA shows each member as a top-level folder, sorted by the
// Project View order; an empty workspace shows no rows.
func (t *Tree) SetProjectRoots(roots []string, enabled bool) {
	normalized := make([]string, 0, len(roots))
	seen := make(map[string]bool, len(roots))
	for _, root := range roots {
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		if seen[root] {
			continue
		}
		seen[root] = true
		normalized = append(normalized, root)
	}
	t.multiRoot = enabled
	t.projectRoots = normalized

	active := make(map[string]bool, len(normalized)+1)
	if t.scratchRoot != "" {
		active[t.scratchRoot] = true
	}
	if enabled {
		primary := ""
		if t.ws != nil {
			primary = filepath.Clean(t.ws.Root)
		}
		for _, root := range normalized {
			active[root] = true
			t.open[root] = true
			if root != primary {
				if _, ok := t.extraTrees[root]; !ok {
					t.addExtraTree(root)
				}
			}
		}
	}
	for root, tree := range t.extraTrees {
		if active[root] {
			continue
		}
		tree.Close()
		delete(t.extraTrees, root)
	}
	t.rebuild()
}

// Outline returns a file's cached member rows and whether they are loaded.
func (t *Tree) Outline(path string) ([]openidea.OutlineItem, bool) {
	items, ok := t.outlines[path]
	return items, ok
}

// SetOutline caches a document's members. It reports whether the call changed
// the tree.
func (t *Tree) SetOutline(path string, items []openidea.OutlineItem) {
	if equalOutlineItems(t.outlines[path], items) {
		return
	}
	t.outlines[path] = append([]openidea.OutlineItem(nil), items...)
	t.rebuild()
}

// ResetOutline drops a file's member cache after an edit or reload.
func (t *Tree) ResetOutline(path string) {
	delete(t.outlines, path)
	t.rebuild()
}

func equalProjectTreePaths(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// Refresh rereads the open folders, as a new or renamed entry asks.
func (t *Tree) Refresh() {
	if t.remoteLoader != nil {
		t.refreshRemote("")
		return
	}
	if t.ws == nil && t.externalRoot == nil {
		return
	}
	if t.ws != nil {
		t.ws.Refresh()
	}
	for _, tree := range t.extraTrees {
		tree.Refresh()
	}
	t.refreshExternal("")
	t.rebuild()
}

// RefreshDir rereads dir and the folders below it the tree read, as Reload
// from Disk does for the selected folder.
func (t *Tree) RefreshDir(dir string) {
	if t.remoteLoader != nil {
		t.refreshRemote(dir)
		return
	}
	if t.isExternalTreePath(dir) {
		t.refreshExternal(dir)
		return
	}
	tree := t.workspaceFor(dir)
	if tree == nil {
		return
	}
	tree.RefreshDir(dir)
	t.rebuild()
}

// Rows are the visible rows, each already flattened to its depth.
func (t *Tree) Rows() []*treeNode { return t.rows }

// Toggle opens or closes the folder at path.
func (t *Tree) Toggle(path string) {
	isDir := t.isDir(path)
	if t.isExternalTreePath(path) && isDir {
		t.expandAll = ""
		t.open[path] = !t.open[path]
		if t.open[path] {
			delete(t.collapsed, path)
			if t.externalNeedsLoad(path) {
				t.loadExternal(path)
			}
		} else {
			t.collapsed[path] = true
		}
		t.rebuild()
		t.expansionChanged()
		return
	}
	if t.remoteLoader != nil && isDir {
		t.expandAll = ""
		t.open[path] = !t.open[path]
		if t.open[path] {
			delete(t.collapsed, path)
			t.loadRemote(path)
		} else {
			t.collapsed[path] = true
		}
		t.rebuild()
		t.expansionChanged()
		return
	}
	if t.options.showMembers && !isDir {
		if t.open[path] {
			delete(t.open, path)
			t.collapsed[path] = true
		} else {
			t.open[path] = true
			delete(t.collapsed, path)
		}
		t.rebuild()
		t.expansionChanged()
		return
	}
	if (t.Mode() == "openFiles" || t.Mode() == "changedFiles") && isDir {
		if t.open[path] {
			delete(t.open, path)
			t.collapsed[path] = true
		} else {
			t.open[path] = true
			delete(t.collapsed, path)
		}
		t.rebuild()
		t.expansionChanged()
		return
	}
	tree := t.workspaceFor(path)
	if tree == nil || !isDir {
		return
	}
	t.expandAll = ""
	if t.open[path] {
		delete(t.open, path)
		t.collapsed[path] = true
	} else {
		t.open[path] = true
		delete(t.collapsed, path)
		tree.Load(path)
	}
	t.rebuild()
	t.expansionChanged()
}

// ToggleMember opens or closes a declaration's child rows.
func (t *Tree) ToggleMember(path string, line, col int, name string) {
	key := path + "\x00" + strconv.Itoa(line) + ":" + strconv.Itoa(col) + ":" + name
	if t.open[key] {
		delete(t.open, key)
	} else {
		t.open[key] = true
	}
	t.rebuild()
	t.expansionChanged()
}

// ExpandAll opens dir and every folder below it. A folder is read when it
// first opens, so the pass carries on through the reads' own rebuilds until
// nothing below dir is left closed.
func (t *Tree) ExpandAll(dir string) {
	if t.isExternalTreePath(dir) {
		if dir == "" {
			return
		}
		t.open[dir] = true
		delete(t.collapsed, dir)
		if t.externalNeedsLoad(dir) {
			t.loadExternal(dir)
		}
		t.expandAll = dir
		t.rebuild()
		t.expansionChanged()
		return
	}
	if t.remoteLoader != nil {
		if dir == "" {
			return
		}
		t.open[dir] = true
		delete(t.collapsed, dir)
		t.loadRemote(dir)
		t.expandAll = dir
		t.rebuild()
		t.expansionChanged()
		return
	}
	tree := t.workspaceFor(dir)
	if tree == nil || dir == "" {
		return
	}
	t.open[dir] = true
	delete(t.collapsed, dir)
	tree.Load(dir)
	t.expandAll = dir
	t.rebuild()
	t.expansionChanged()
}

// CollapseAll closes every folder but the root, as OpenIDEA's Collapse All
// does.
func (t *Tree) CollapseAll() {
	if t.root == nil || (t.ws == nil && t.remoteLoader == nil && t.externalRoot == nil) {
		return
	}
	t.expandAll = ""
	for p := range t.open {
		delete(t.open, p)
		t.collapsed[p] = true
	}
	if t.multiRoot {
		for _, root := range t.projectRoots {
			t.open[root] = true
			delete(t.collapsed, root)
		}
	} else {
		t.open[t.root.path] = true
		delete(t.collapsed, t.root.path)
	}
	t.rebuild()
	t.expansionChanged()
}

// Select marks path as the tree's only selection, so a reveal from the editor
// leaves a visible row.
func (t *Tree) Select(path string) {
	t.sel = path
	t.anchor = path
	t.sels = map[string]bool{}
	if path != "" {
		t.sels[path] = true
	}
}

// Selected is the row Select or reveal last named.
func (t *Tree) Selected() string { return t.sel }

// SelectedPaths returns the current selection in row order. Member rows are
// omitted because file operations cannot act on them.
func (t *Tree) SelectedPaths() []string {
	if t.sel == "" {
		return nil
	}
	out := make([]string, 0, len(t.sels))
	for _, row := range t.rows {
		if row.member || !t.sels[row.path] {
			continue
		}
		out = append(out, row.path)
	}
	if len(out) == 0 && t.sels == nil {
		out = append(out, t.sel)
	}
	return out
}

func (t *Tree) leadRowIndex() int {
	fallback := -1
	for index, row := range t.rows {
		if row.member {
			continue
		}
		if fallback < 0 {
			fallback = index
		}
		if row.path == t.sel {
			return index
		}
	}
	return fallback
}

// StepSelection moves the keyboard lead through visible project rows. Extend
// leaves the range anchor in place, as Shift+Arrow does in OpenIDEA's tree.
func (t *Tree) StepSelection(delta int, extend bool) {
	if len(t.rows) == 0 || delta == 0 {
		return
	}
	index := t.leadRowIndex()
	next := index
	for {
		next += delta
		if next < 0 || next >= len(t.rows) {
			return
		}
		if !t.rows[next].member {
			break
		}
	}
	if extend {
		t.clickSelection(t.rows[next].path, true, false)
		return
	}
	t.Select(t.rows[next].path)
}

// SelectAll selects every visible file and folder row, excluding outline
// members that cannot receive file operations.
func (t *Tree) SelectAll() {
	t.sels = map[string]bool{}
	for _, row := range t.rows {
		if row.member {
			continue
		}
		t.sels[row.path] = true
		if t.sel == "" {
			t.sel = row.path
			t.anchor = row.path
		}
	}
	if t.anchor == "" {
		t.anchor = t.sel
	}
}

// CollapseSelection closes the lead folder or moves to its parent row.
func (t *Tree) CollapseSelection() bool {
	index := t.leadRowIndex()
	if index < 0 {
		return false
	}
	row := t.rows[index]
	if row.member || !row.dir {
		return false
	}
	if row.expanded {
		t.Toggle(row.path)
		t.Select(row.path)
		return true
	}
	for previous := index - 1; previous >= 0; previous-- {
		parent := t.rows[previous]
		if !parent.member && parent.dir && parent.depth < row.depth {
			t.Select(parent.path)
			return true
		}
	}
	return false
}

// ExpandSelection opens the lead folder or moves to its first visible child.
func (t *Tree) ExpandSelection() bool {
	index := t.leadRowIndex()
	if index < 0 {
		return false
	}
	row := t.rows[index]
	if row.member || !row.dir {
		return false
	}
	if row.expanded {
		for next := index + 1; next < len(t.rows); next++ {
			child := t.rows[next]
			if !child.member && child.depth > row.depth {
				t.Select(child.path)
				return true
			}
			if child.depth <= row.depth {
				break
			}
		}
		return false
	}
	t.Toggle(row.path)
	t.Select(row.path)
	return true
}

// clickSelection applies the project tree's Cmd/Ctrl and Shift selection
// rules. active is the row under the pointer.
func (t *Tree) clickSelection(path string, extend, toggle bool) {
	switch {
	case extend && (t.anchor != "" || t.sel != ""):
		anchor := t.anchor
		if anchor == "" {
			anchor = t.sel
		}
		start, end := -1, -1
		for index, row := range t.rows {
			switch row.path {
			case anchor:
				start = index
			case path:
				end = index
			}
		}
		if start >= 0 && end >= 0 {
			if start > end {
				start, end = end, start
			}
			next := map[string]bool{}
			for _, row := range t.rows[start : end+1] {
				if !row.member {
					next[row.path] = true
				}
			}
			t.sels, t.sel = next, path
			return
		}
	case toggle:
		if t.sels == nil {
			t.sels = map[string]bool{}
		}
		if t.sels[path] {
			delete(t.sels, path)
			t.sel, t.anchor = path, path
			return
		}
		if !t.sels[path] {
			t.sels[path] = true
		}
		t.sel, t.anchor = path, path
		return
	}
	t.Select(path)
}

// IsDir reports whether the named row is a folder among the tree's rows.
func (t *Tree) IsDir(path string) bool {
	for _, r := range t.rows {
		if r.path == path {
			return r.dir
		}
	}
	return t.isDir(path)
}

// ReadOnly reports whether path is a contributed SDK or library row.
func (t *Tree) ReadOnly(path string) bool {
	for _, row := range t.rows {
		if row.path == path {
			return row.readOnly
		}
	}
	return t.isExternalTreePath(path)
}

// reveal opens the folders leading to path, as a file opened from elsewhere.
func (t *Tree) reveal(path string) {
	if t.remoteLoader != nil {
		t.revealRemote(path)
		return
	}
	if t.revealExternal(path) {
		return
	}
	if t.ws == nil || t.root == nil || path == "" {
		return
	}
	base := t.rootForPath(path)
	tree := t.workspaceFor(path)
	if base == "" || tree == nil {
		return
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}
	dir := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		if dir == path {
			break
		}
		// A folder the workspace has not read yet is not known to be
		// one; it opens here and its rows arrive when it is read.
		t.open[dir] = true
		delete(t.collapsed, dir)
		tree.Load(dir)
	}
	t.open[base] = true
	delete(t.collapsed, base)
	t.rebuild()
	t.expansionChanged()
}

// rebuild flattens the open folders into rows, skipping what hidden reports.
func (t *Tree) rebuild() {
	t.rows = t.rows[:0]
	if t.root == nil {
		return
	}
	if t.remoteLoader != nil {
		t.rebuildRemote()
		return
	}
	if t.expandAll != "" {
		if t.isExternalTreePath(t.expandAll) {
			t.openExternalUnder(t.expandAll)
		} else {
			t.openUnder(t.expandAll)
		}
	}
	switch t.Mode() {
	case "openFiles":
		t.rebuildPathForest(t.openFiles)
	case "changedFiles":
		t.rebuildPathForest(t.changedFiles)
	case "scratches":
		if t.scratchRoot != "" {
			t.walk(t.newRootNode(t.scratchRoot, "Scratches and Consoles", true, false), t.workspaceFor(t.scratchRoot))
		}
	default:
		if t.multiRoot {
			roots := append([]string(nil), t.projectRoots...)
			t.sortProjectPaths(roots, func(path string) bool { return true })
			for _, root := range roots {
				t.walk(t.newRootNode(root, filepath.Base(root), false, false), t.workspaceFor(root))
			}
		} else {
			t.walk(t.root, t.ws)
		}
		if t.Mode() == "project" && t.externalRoot != nil {
			t.walkExternal(t.externalRoot)
		}
		// Project includes contributed roots, while Project Files is the
		// workspace alone, as OpenIDEA's two panes.
		if t.Mode() == "project" && t.scratchRoot != "" {
			root := t.newRootNode(t.scratchRoot, "Scratches and Consoles", true, false)
			t.walk(root, t.workspaceFor(t.scratchRoot))
		}
	}
	if t.expandAll != "" {
		if t.isExternalTreePath(t.expandAll) {
			if t.externalExpandSettled(t.expandAll) {
				t.expandAll = ""
			}
		} else if t.expandSettled(t.expandAll) {
			t.expandAll = ""
		}
	}
	if len(t.sels) > 0 {
		visible := make(map[string]bool, len(t.rows))
		for _, row := range t.rows {
			visible[row.path] = true
		}
		for path := range t.sels {
			if !visible[path] {
				delete(t.sels, path)
			}
		}
		if !t.sels[t.sel] && len(t.sels) > 0 {
			for _, row := range t.rows {
				if t.sels[row.path] {
					t.sel = row.path
					break
				}
			}
		}
	}
}

func (t *Tree) newRootNode(path, label string, group, readOnly bool) *treeNode {
	detail := ""
	if t.options.showRootPaths {
		detail = tildifyPath(path)
	}
	return &treeNode{
		path: path, name: label, dir: true, expanded: t.open[path],
		group: group, readOnly: readOnly, detail: detail,
	}
}

func (t *Tree) walk(n *treeNode, tree *workspace.Tree) {
	if n == nil {
		return
	}
	if n == t.root && n.name == filepath.Base(n.path) {
		if t.options.showRootPaths {
			n.detail = tildifyPath(n.path)
		} else {
			n.detail = ""
		}
	}
	t.rows = append(t.rows, n)
	if !n.dir || !t.open[n.path] || tree == nil {
		return
	}
	tree.Load(n.path)
	for _, p := range t.sortedNormalChildren(tree, n.path) {
		name := filepath.Base(p)
		dir := tree.IsDir(p)
		if !t.entryVisible(p, name, dir) {
			continue
		}
		leaf := p
		leafName := name
		pkg := false
		if dir && t.options.compact && inPackageTree(p) {
			if joined, deep, ok := t.packageChain(tree, p); ok {
				leafName, leaf, pkg = joined, deep, true
			}
		}
		child := &treeNode{
			path: leaf, name: leafName, dir: dir, depth: n.depth + 1,
			expanded: t.open[leaf], excluded: t.pathExcluded(leaf) || dir && excludedDir(name),
			module:     dir && tree.IsDir(leaf) && t.isModule(tree, leaf),
			packageDir: pkg,
		}
		t.walk(child, tree)
		if !dir && t.options.showMembers && t.open[leaf] {
			t.appendMembers(leaf, n.depth+2)
		}
	}
}

func (t *Tree) sortedNormalChildren(tree *workspace.Tree, dir string) []string {
	if tree == nil {
		return nil
	}
	children := append([]string(nil), tree.Children(dir)...)
	t.sortProjectPaths(children, tree.IsDir)
	return children
}

func (t *Tree) entryVisible(path, name string, dir bool) bool {
	if ignoredFileName(name, t.fileIgnores) {
		return t.options.showHidden
	}
	if hiddenName(name, t.options.showHidden) {
		return false
	}
	if dir {
		if t.Mode() == "unity" && !t.options.showUnityGenerated && unityGeneratedDir(name) {
			return false
		}
		if excludedDir(name) && !t.options.showExcluded {
			return false
		}
	} else if t.Mode() == "unity" {
		if !t.unityFileVisible(path) {
			return false
		}
	}
	if t.pathExcluded(path) && !t.options.showExcluded {
		return false
	}
	return true
}

func ignoredFileName(name string, patterns []string) bool {
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		if pattern == "" || strings.Contains(pattern, "/") {
			continue
		}
		if match, err := path.Match(pattern, name); err == nil && match {
			return true
		}
	}
	return false
}

func (t *Tree) pathExcluded(value string) bool {
	if len(t.fileExcludes) == 0 {
		return false
	}
	root := t.rootForPath(value)
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, value)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	return t.fileExcludes.Match(filepath.ToSlash(rel))
}

func unityGeneratedDir(name string) bool {
	switch name {
	case "Library", "Temp", "Obj", "obj", "Logs", "Build", "Builds", "UserSettings", ".vs", ".idea", ".vscode":
		return true
	default:
		return false
	}
}

func (t *Tree) unityFileVisible(path string) bool {
	root := ""
	if t.ws != nil {
		root = t.ws.Root
	}
	if root == "" {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return true
	}
	if filepath.Ext(rel) == ".meta" && !t.options.showUnityMeta {
		return false
	}
	if !t.options.showUnityGenerated {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) > 0 {
			if unityGeneratedDir(parts[0]) {
				return false
			}
		}
		if len(parts) == 1 {
			switch strings.ToLower(filepath.Ext(parts[0])) {
			case ".csproj", ".sln", ".slnx", ".suo", ".user", ".pidb", ".booproj", ".unityproj":
				return false
			}
		}
	}
	return true
}

func (t *Tree) appendMembers(path string, depth int) {
	items := t.outlines[path]
	var walk func([]openidea.OutlineItem, int)
	walk = func(nodes []openidea.OutlineItem, level int) {
		for _, item := range nodes {
			detail := ""
			if item.Detail != nil {
				detail = *item.Detail
			}
			node := &treeNode{
				path: path, name: item.Name, depth: level, member: true,
				kind: item.Kind, line: int(item.SelectionRange.Start.Line),
				col: int(item.SelectionRange.Start.Col), detail: detail,
				expanded: t.open[memberKey(path, item)], hasChildren: len(item.Children) > 0,
			}
			t.rows = append(t.rows, node)
			if node.expanded {
				walk(item.Children, level+1)
			}
		}
	}
	walk(items, depth)
}

func memberKey(path string, item openidea.OutlineItem) string {
	return path + "\x00" + strconv.Itoa(int(item.SelectionRange.Start.Line)) +
		":" + strconv.Itoa(int(item.SelectionRange.Start.Col)) + ":" + item.Name
}

// rebuildPathForest lists Open Files or All Changed Files as their ancestors,
// with the project and contributed scratch roots kept separate.
func (t *Tree) rebuildPathForest(files []string) {
	groups := map[string][]string{}
	for _, path := range files {
		if path == "" {
			continue
		}
		base := t.rootForPath(path)
		if base == "" {
			base = filepath.Dir(path)
		}
		groups[filepath.Clean(base)] = append(groups[filepath.Clean(base)], path)
	}
	roots := make([]string, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	t.sortProjectPaths(roots, func(path string) bool { return true })
	for _, root := range roots {
		label := filepath.Base(root)
		group, readOnly := false, false
		if root == t.scratchRoot {
			label = "Scratches and Consoles"
			group, readOnly = true, false
		}
		node := t.newRootNode(root, label, group, readOnly)
		t.appendFlatTree(node, append([]string(nil), groups[root]...))
	}
}

func (t *Tree) appendFlatTree(root *treeNode, files []string) {
	t.rows = append(t.rows, root)
	if !t.open[root.path] {
		return
	}
	var walk func(base string, depth int, paths []string)
	walk = func(base string, depth int, paths []string) {
		type entry struct {
			path string
			dir  bool
			kids []string
		}
		byName := map[string]*entry{}
		for _, path := range paths {
			rel, err := filepath.Rel(base, path)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				continue
			}
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) == 0 {
				continue
			}
			child := filepath.Join(base, parts[0])
			item := byName[parts[0]]
			if item == nil {
				item = &entry{path: child, dir: len(parts) > 1}
				byName[parts[0]] = item
			}
			if len(parts) > 1 {
				item.dir = true
				item.kids = append(item.kids, path)
			}
		}
		entries := make([]*entry, 0, len(byName))
		for _, entry := range byName {
			entries = append(entries, entry)
		}
		sort.SliceStable(entries, func(i, j int) bool {
			return t.pathLess(entries[i].path, entries[i].dir, entries[j].path, entries[j].dir)
		})
		for _, entry := range entries {
			if !t.entryVisible(entry.path, filepath.Base(entry.path), entry.dir) {
				continue
			}
			node := &treeNode{
				path: entry.path, name: filepath.Base(entry.path), dir: entry.dir,
				depth: depth, expanded: t.open[entry.path], excluded: entry.dir && excludedDir(filepath.Base(entry.path)),
			}
			t.rows = append(t.rows, node)
			if entry.dir {
				if node.expanded {
					walk(entry.path, depth+1, entry.kids)
				}
			} else if t.options.showMembers && t.open[entry.path] {
				t.appendMembers(entry.path, depth+1)
			}
		}
	}
	walk(root.path, 1, files)
}

func (t *Tree) expandFlatParents() {
	for _, list := range [][]string{t.openFiles, t.changedFiles} {
		for _, path := range list {
			base := t.rootForPath(path)
			if base == "" {
				continue
			}
			t.open[base] = true
			t.virtualDirs[base] = true
			rel, err := filepath.Rel(base, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			dir := base
			parts := strings.Split(rel, string(filepath.Separator))
			for _, part := range parts[:max(0, len(parts)-1)] {
				dir = filepath.Join(dir, part)
				t.virtualDirs[dir] = true
				if !t.collapsed[dir] {
					t.open[dir] = true
				}
			}
		}
	}
}

func (t *Tree) sortProjectPaths(paths []string, isDir func(string) bool) {
	sort.SliceStable(paths, func(i, j int) bool {
		return t.pathLess(paths[i], isDir(paths[i]), paths[j], isDir(paths[j]))
	})
}

func (t *Tree) pathLess(left string, leftDir bool, right string, rightDir bool) bool {
	if t.options.foldersFirst && leftDir != rightDir {
		return leftDir
	}
	switch t.options.sortBy {
	case "type":
		le, re := strings.ToLower(projectExtension(left)), strings.ToLower(projectExtension(right))
		if compareProjectNames(le, re) != 0 {
			return compareProjectNames(le, re) < 0
		}
	case "modified":
		lm, rm := projectModified(left), projectModified(right)
		if !lm.Equal(rm) {
			return lm.After(rm)
		}
	}
	return compareProjectNames(filepath.Base(left), filepath.Base(right)) < 0
}

func projectExtension(path string) string {
	dot := strings.LastIndex(filepath.Base(path), ".")
	if dot <= 0 {
		return ""
	}
	return filepath.Base(path)[dot+1:]
}

func projectModified(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func compareProjectNames(left, right string) int {
	a, b := strings.ToLower(left), strings.ToLower(right)
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		if isASCIIDigit(a[ai]) && isASCIIDigit(b[bi]) {
			aj, bj := ai, bi
			for aj < len(a) && isASCIIDigit(a[aj]) {
				aj++
			}
			for bj < len(b) && isASCIIDigit(b[bj]) {
				bj++
			}
			an, _ := strconv.ParseUint(a[ai:aj], 10, 64)
			bn, _ := strconv.ParseUint(b[bi:bj], 10, 64)
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
			ai, bi = aj, bj
			continue
		}
		if a[ai] != b[bi] {
			if a[ai] < b[bi] {
				return -1
			}
			return 1
		}
		ai++
		bi++
	}
	if len(a)-ai != len(b)-bi {
		if len(a)-ai < len(b)-bi {
			return -1
		}
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

func tildifyPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.ToSlash(path)
	}
	if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(filepath.Join("~", rel))
	}
	return filepath.ToSlash(path)
}

func equalOutlineItems(left, right []openidea.OutlineItem) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		a, b := left[i], right[i]
		if a.Name != b.Name || a.Kind != b.Kind || a.Range != b.Range ||
			a.SelectionRange != b.SelectionRange ||
			(a.Detail == nil) != (b.Detail == nil) ||
			a.Detail != nil && *a.Detail != *b.Detail ||
			!equalOutlineItems(a.Children, b.Children) {
			return false
		}
	}
	return true
}

// openUnder opens every folder the workspace already knows below dir, so the
// next read of one of them widens the tree. It stops once nothing below dir
// is left closed.
func (t *Tree) openUnder(dir string) {
	if t.remoteLoader != nil {
		t.openRemoteUnder(dir)
		return
	}
	tree := t.workspaceFor(dir)
	if tree == nil || !t.open[dir] || tree.Loading(dir) || !tree.Loaded(dir) {
		return
	}
	for _, p := range tree.Children(dir) {
		if !tree.IsDir(p) || !t.entryVisible(p, filepath.Base(p), true) {
			continue
		}
		if !t.open[p] {
			t.open[p] = true
			tree.Load(p)
		}
		t.openUnder(p)
	}
}

// expandSettled reports whether every folder below dir is open and read.
func (t *Tree) expandSettled(dir string) bool {
	if t.remoteLoader != nil {
		return t.remoteExpandSettled(dir)
	}
	tree := t.workspaceFor(dir)
	if tree == nil || !t.open[dir] || tree.Loading(dir) || !tree.Loaded(dir) {
		return false
	}
	for _, p := range tree.Children(dir) {
		if !tree.IsDir(p) || !t.entryVisible(p, filepath.Base(p), true) {
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
func (t *Tree) packageChain(tree *workspace.Tree, dir string) (name, leaf string, ok bool) {
	if tree == nil || !tree.IsDir(dir) {
		return "", dir, false
	}
	base := filepath.Base(dir)
	if !tree.Loaded(dir) {
		tree.Load(dir)
		return "", dir, false
	}
	kids := t.visibleChildren(tree, dir)
	if len(kids) != 1 || !tree.IsDir(kids[0]) {
		return "", dir, false
	}
	if deeper, deepLeaf, ok := t.packageChain(tree, kids[0]); ok {
		return base + "." + deeper, deepLeaf, true
	}
	return base + "." + filepath.Base(kids[0]), kids[0], true
}

// visibleChildren lists a directory's entries the tree draws.
func (t *Tree) visibleChildren(tree *workspace.Tree, dir string) []string {
	var out []string
	for _, p := range tree.Children(dir) {
		name := filepath.Base(p)
		dir := tree.IsDir(p)
		if !t.entryVisible(p, name, dir) {
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

// hiddenName reports whether Project View's ignored names leave a name out.
// OpenIDEA always shows ordinary dotfiles; Show Hidden Files brings ignored
// VCS metadata and OS files back.
func hiddenName(name string, showHidden bool) bool {
	if showHidden {
		return false
	}
	switch name {
	case ".git", ".hg", ".svn", ".DS_Store", "Thumbs.db":
		return true
	}
	return false
}

// excludedDir reports whether a shown folder is build output or tool state,
// which IDEs dim and shade: "excluded" content.
func excludedDir(name string) bool {
	return hiddenDirs[name] ||
		name == ".venv" || name == "__pycache__" || name == ".next" || name == ".turbo" || name == ".cache"
}

// isModule reports whether a folder is a Maven or Gradle module: it has a
// pom.xml or a build.gradle(.kts) beside its other entries.
func (t *Tree) isModule(tree *workspace.Tree, path string) bool {
	if tree == nil || !tree.Loaded(path) {
		return false
	}
	for _, p := range tree.Children(path) {
		switch filepath.Base(p) {
		case "pom.xml", "build.gradle", "build.gradle.kts":
			return true
		}
	}
	return false
}
