package app

import (
	"net/url"
	"sort"
	"strings"

	"mayoi/internal/openidea"
)

const externalLibrariesRootID = "contributed:externalLibraries"

func externalLibraryGroupID(group openidea.LibraryGroup) string {
	return "contributed:externalLibraries:" +
		url.PathEscape(group.Kind) + ":" + url.PathEscape(group.Name)
}

// SetExternalLibraries replaces OpenIDEA's contributed External Libraries
// subtree. Contributed parent rows are built locally while every browsable
// root and child directory uses the backend VFS.
func (t *Tree) SetExternalLibraries(
	groups []openidea.LibraryGroup,
	loader func(string, func([]openidea.RemoteDirectoryEntry, error)),
) {
	if t.externalEntries == nil {
		t.externalEntries = map[string][]openidea.RemoteDirectoryEntry{}
	}
	if t.externalLoading == nil {
		t.externalLoading = map[string]bool{}
	}
	if t.externalErrs == nil {
		t.externalErrs = map[string]string{}
	}
	if t.externalDirs == nil {
		t.externalDirs = map[string]bool{}
	}
	if t.externalVersion == nil {
		t.externalVersion = map[string]uint64{}
	}

	t.externalLoader = loader
	t.externalDirs = map[string]bool{externalLibrariesRootID: true}
	if len(groups) == 0 {
		t.externalRoot = nil
		t.externalEntries = map[string][]openidea.RemoteDirectoryEntry{}
		t.externalLoading = map[string]bool{}
		t.externalErrs = map[string]string{}
		t.externalVersion = map[string]uint64{}
		t.rebuild()
		return
	}

	groups = sortLibraryGroups(mergeLibraryGroups(groups))
	root := &treeNode{
		path: externalLibrariesRootID, name: "External Libraries",
		dir: true, expanded: t.open[externalLibrariesRootID],
		group: true, readOnly: true, icon: "nodes/library",
	}
	validRoots := make([]string, 0)
	for _, group := range groups {
		groupID := externalLibraryGroupID(group)
		t.externalDirs[groupID] = true
		groupNode := &treeNode{
			path: groupID, name: externalLibraryGroupLabel(group),
			dir: true, depth: 1, expanded: t.open[groupID],
			group: true, readOnly: true, icon: externalLibraryGroupIcon(group.Kind),
		}
		for _, libraryRoot := range group.Roots {
			if libraryRoot.Path == "" {
				continue
			}
			validRoots = append(validRoots, libraryRoot.Path)
			t.externalDirs[libraryRoot.Path] = true
			groupNode.children = append(groupNode.children, &treeNode{
				path: libraryRoot.Path, name: libraryRoot.Name,
				dir: true, depth: 2, expanded: t.open[libraryRoot.Path],
				readOnly: true, external: true,
				detail: "library root", icon: externalLibraryRootIcon(libraryRoot),
			})
		}
		root.children = append(root.children, groupNode)
	}
	t.externalRoot = root

	for filePath := range t.externalEntries {
		if !pathBelongsToAnyLibraryRoot(filePath, validRoots) {
			delete(t.externalEntries, filePath)
			delete(t.externalErrs, filePath)
			delete(t.externalLoading, filePath)
			t.externalVersion[filePath]++
			delete(t.externalDirs, filePath)
		}
	}
	for filePath := range t.externalErrs {
		if !pathBelongsToAnyLibraryRoot(filePath, validRoots) {
			delete(t.externalErrs, filePath)
		}
	}
	t.rebuild()
}

func pathBelongsToAnyLibraryRoot(candidate string, roots []string) bool {
	for _, root := range roots {
		if libraryPathWithin(root, candidate) {
			return true
		}
	}
	return false
}

func (t *Tree) isExternalTreePath(filePath string) bool {
	if filePath == "" {
		return false
	}
	if t.externalDirs[filePath] {
		return true
	}
	if strings.HasPrefix(filePath, externalLibrariesRootID) {
		return true
	}
	if t.externalRoot == nil {
		return false
	}
	for _, group := range t.externalRoot.children {
		for _, root := range group.children {
			if libraryPathWithin(root.path, filePath) {
				return true
			}
		}
	}
	return false
}

func (t *Tree) loadExternal(dir string) {
	if dir == "" || t.externalClosed || t.externalLoading[dir] {
		return
	}
	t.externalLoading[dir] = true
	delete(t.externalErrs, dir)
	version := t.externalVersion[dir] + 1
	t.externalVersion[dir] = version
	if t.externalLoader == nil {
		t.externalLoading[dir] = false
		t.externalEntries[dir] = []openidea.RemoteDirectoryEntry{}
		return
	}
	t.externalLoader(dir, func(entries []openidea.RemoteDirectoryEntry, err error) {
		finish := func() {
			if t.externalClosed || t.externalVersion[dir] != version {
				return
			}
			delete(t.externalLoading, dir)
			if err != nil {
				t.externalErrs[dir] = err.Error()
			} else {
				copied := append([]openidea.RemoteDirectoryEntry(nil), entries...)
				t.externalEntries[dir] = copied
				for _, entry := range copied {
					t.externalDirs[entry.Path] = entry.IsDir
				}
			}
			t.rebuild()
			if t.changed != nil {
				t.changed()
			}
		}
		if t.post == nil {
			finish()
		} else {
			t.post(finish)
		}
	})
}

func (t *Tree) refreshExternal(dir string) {
	if t.externalRoot == nil {
		return
	}
	if dir == "" {
		t.externalEntries = map[string][]openidea.RemoteDirectoryEntry{}
		t.externalErrs = map[string]string{}
		t.externalLoading = map[string]bool{}
		for candidate := range t.externalVersion {
			t.externalVersion[candidate]++
		}
		for open := range t.open {
			if t.isExternalTreePath(open) && open != externalLibrariesRootID {
				t.loadExternal(open)
			}
		}
		t.rebuild()
		return
	}
	for candidate := range t.externalEntries {
		if candidate == dir || libraryPathWithin(dir, candidate) {
			delete(t.externalEntries, candidate)
			delete(t.externalErrs, candidate)
			delete(t.externalLoading, candidate)
			t.externalVersion[candidate]++
		}
	}
	t.loadExternal(dir)
	t.rebuild()
}

func (t *Tree) walkExternal(node *treeNode) {
	if node == nil {
		return
	}
	t.rows = append(t.rows, node)
	if !node.dir || !t.open[node.path] {
		return
	}
	if len(node.children) > 0 {
		for _, child := range node.children {
			t.walkExternal(child)
		}
		return
	}
	entries, loaded := t.externalEntries[node.path]
	if !loaded {
		t.loadExternal(node.path)
		return
	}
	entries = append([]openidea.RemoteDirectoryEntry(nil), entries...)
	sortRemoteEntries(entries, t.options.foldersFirst, t.options.sortBy)
	for _, entry := range entries {
		name := entry.Name
		if name == "" {
			name = pathBaseAny(entry.Path)
		}
		if !t.entryVisible(entry.Path, name, entry.IsDir) {
			continue
		}
		child := &treeNode{
			path: entry.Path, name: name, dir: entry.IsDir,
			depth: node.depth + 1, expanded: t.open[entry.Path],
			readOnly: true, external: true,
			excluded: entry.IsDir && excludedDir(name),
		}
		if entry.IsDir {
			child.icon = "nodes/folder"
		} else {
			child.icon = fileIconAsset(name)
		}
		t.walkExternal(child)
		if !entry.IsDir && t.options.showMembers && t.open[entry.Path] {
			t.appendMembers(entry.Path, node.depth+2)
		}
	}
}

func pathBaseAny(filePath string) string {
	filePath = strings.TrimRight(filePath, `/\`)
	if at := strings.LastIndexAny(filePath, `/\`); at >= 0 {
		return filePath[at+1:]
	}
	return filePath
}

func (t *Tree) openExternalUnder(dir string) {
	node := t.externalNode(dir)
	if node == nil || !t.open[dir] {
		return
	}
	if len(node.children) > 0 {
		for _, child := range node.children {
			t.open[child.path] = true
			delete(t.collapsed, child.path)
			t.openExternalUnder(child.path)
		}
		return
	}
	entries, loaded := t.externalEntries[dir]
	if !loaded {
		t.loadExternal(dir)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir || !t.entryVisible(entry.Path, entry.Name, true) {
			continue
		}
		t.open[entry.Path] = true
		delete(t.collapsed, entry.Path)
		t.openExternalUnder(entry.Path)
	}
}

func (t *Tree) externalExpandSettled(dir string) bool {
	if t.externalLoading[dir] {
		return false
	}
	node := t.externalNode(dir)
	if node == nil {
		return true
	}
	if len(node.children) > 0 {
		for _, child := range node.children {
			if !t.open[child.path] {
				t.open[child.path] = true
				t.loadExternal(child.path)
				return false
			}
			if !t.externalExpandSettled(child.path) {
				return false
			}
		}
		return true
	}
	entries, loaded := t.externalEntries[dir]
	if !loaded {
		t.loadExternal(dir)
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir || !t.entryVisible(entry.Path, entry.Name, true) {
			continue
		}
		if !t.open[entry.Path] {
			t.open[entry.Path] = true
			t.loadExternal(entry.Path)
			return false
		}
		if !t.externalExpandSettled(entry.Path) {
			return false
		}
	}
	return true
}

func (t *Tree) externalNode(filePath string) *treeNode {
	if t.externalRoot == nil {
		return nil
	}
	if filePath == t.externalRoot.path {
		return t.externalRoot
	}
	for _, group := range t.externalRoot.children {
		if filePath == group.path {
			return group
		}
		for _, root := range group.children {
			if filePath == root.path {
				return root
			}
		}
	}
	return &treeNode{path: filePath, dir: t.isDir(filePath), external: true}
}

func (t *Tree) externalNeedsLoad(filePath string) bool {
	node := t.externalNode(filePath)
	return node != nil && node.external && len(node.children) == 0
}

func (t *Tree) revealExternal(filePath string) bool {
	if t.externalRoot == nil || filePath == "" {
		return false
	}
	type match struct {
		group *treeNode
		root  *treeNode
	}
	var best *match
	for _, group := range t.externalRoot.children {
		for _, root := range group.children {
			if !libraryPathWithin(root.path, filePath) {
				continue
			}
			if best == nil || len(root.path) > len(best.root.path) {
				best = &match{group: group, root: root}
			}
		}
	}
	if best == nil {
		return false
	}
	t.open[t.externalRoot.path] = true
	delete(t.collapsed, t.externalRoot.path)
	t.open[best.group.path] = true
	delete(t.collapsed, best.group.path)
	t.open[best.root.path] = true
	delete(t.collapsed, best.root.path)
	t.loadExternal(best.root.path)

	remainder := strings.TrimPrefix(filePath, best.root.path)
	remainder = strings.TrimLeft(remainder, `/\`)
	current := best.root.path
	parts := strings.FieldsFunc(remainder, func(r rune) bool { return r == '/' || r == '\\' })
	for _, part := range parts {
		separator := "/"
		if !strings.HasSuffix(current, "!") && !strings.Contains(current, "/") &&
			strings.Contains(best.root.path, `\`) {
			separator = `\`
		}
		current = strings.TrimRight(current, `/\`) + separator + part
		if current != filePath {
			t.open[current] = true
			delete(t.collapsed, current)
			t.loadExternal(current)
		}
	}
	t.rebuild()
	t.expansionChanged()
	return true
}

func sortedExternalRoots(groups []openidea.LibraryGroup) []string {
	var roots []string
	for _, group := range groups {
		for _, root := range group.Roots {
			if root.Path != "" {
				roots = append(roots, root.Path)
			}
		}
	}
	sort.Strings(roots)
	return roots
}
