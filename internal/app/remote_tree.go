package app

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"mayoi/internal/openidea"
	"mayoi/internal/workspace"
)

// NewRemoteTree returns a Project View backed by the connected remote
// server's fs_read_dir command. It has the same rows and expansion behaviour
// as NewTree without touching the local filesystem.
func NewRemoteTree(
	root string,
	loader func(string, func([]openidea.RemoteDirectoryEntry, error)),
	post func(func()),
	changed func(),
) *Tree {
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
		virtualDirs:         map[string]bool{},
		remoteLoader:        loader,
		remoteEntries:       map[string][]openidea.RemoteDirectoryEntry{},
		remoteLoading:       map[string]bool{},
		remoteErrs:          map[string]string{},
		externalEntries:     map[string][]openidea.RemoteDirectoryEntry{},
		externalLoading:     map[string]bool{},
		externalErrs:        map[string]string{},
		externalDirs:        map[string]bool{},
		externalVersion:     map[string]uint64{},
	}
	if root == "" {
		return t
	}
	root = path.Clean(root)
	t.root = &treeNode{path: root, name: path.Base(root), dir: true, expanded: true}
	t.open[root] = true
	t.loadRemote(root)
	t.rebuild()
	return t
}

func (t *Tree) remoteRoot() string {
	if t.root == nil {
		return ""
	}
	return t.root.path
}

func (t *Tree) loadRemote(dir string) {
	if t.remoteLoader == nil || dir == "" || t.remoteClosed || t.remoteLoading[dir] {
		return
	}
	t.remoteLoading[dir] = true
	delete(t.remoteErrs, dir)
	t.remoteLoader(dir, func(entries []openidea.RemoteDirectoryEntry, err error) {
		finish := func() {
			if t.remoteClosed {
				return
			}
			delete(t.remoteLoading, dir)
			if err != nil {
				t.remoteErrs[dir] = err.Error()
			} else {
				t.remoteEntries[dir] = append([]openidea.RemoteDirectoryEntry(nil), entries...)
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

func (t *Tree) refreshRemote(dir string) {
	if dir == "" {
		t.remoteEntries = map[string][]openidea.RemoteDirectoryEntry{}
		t.remoteErrs = map[string]string{}
		for open := range t.open {
			if t.isKnownRemoteDir(open) {
				t.loadRemote(open)
			}
		}
		t.rebuild()
		return
	}
	for candidate := range t.remoteEntries {
		if sameRemoteFolder(candidate, dir) || strings.HasPrefix(candidate, strings.TrimSuffix(dir, "/")+"/") {
			delete(t.remoteEntries, candidate)
			delete(t.remoteErrs, candidate)
		}
	}
	t.loadRemote(dir)
	t.rebuild()
}

func sameRemoteFolder(left, right string) bool {
	return path.Clean(left) == path.Clean(right)
}

func (t *Tree) isKnownRemoteDir(candidate string) bool {
	if candidate == t.remoteRoot() {
		return true
	}
	for _, entries := range t.remoteEntries {
		for _, entry := range entries {
			if entry.IsDir && entry.Path == candidate {
				return true
			}
		}
	}
	return false
}

func (t *Tree) rebuildRemote() {
	t.walkRemote(t.root)
	if t.expandAll != "" {
		t.openRemoteUnder(t.expandAll)
		if t.remoteExpandSettled(t.expandAll) {
			t.expandAll = ""
		}
	}
	if len(t.sels) > 0 {
		visible := make(map[string]bool, len(t.rows))
		for _, row := range t.rows {
			visible[row.path] = true
		}
		for selected := range t.sels {
			if !visible[selected] {
				delete(t.sels, selected)
			}
		}
	}
}

func (t *Tree) walkRemote(node *treeNode) {
	if node == nil {
		return
	}
	if node == t.root && t.options.showRootPaths {
		node.detail = tildifyRemotePath(node.path)
	}
	t.rows = append(t.rows, node)
	if !node.dir || !t.open[node.path] {
		return
	}
	entries, loaded := t.remoteEntries[node.path]
	if !loaded {
		t.loadRemote(node.path)
		return
	}
	entries = append([]openidea.RemoteDirectoryEntry(nil), entries...)
	sortRemoteEntries(entries, t.options.foldersFirst, t.options.sortBy)
	for _, entry := range entries {
		name := entry.Name
		if name == "" {
			name = path.Base(entry.Path)
		}
		if !t.entryVisible(entry.Path, name, entry.IsDir) {
			continue
		}
		child := &treeNode{
			path:     entry.Path,
			name:     name,
			dir:      entry.IsDir,
			depth:    node.depth + 1,
			expanded: t.open[entry.Path],
			excluded: t.pathExcluded(entry.Path) || entry.IsDir && excludedDir(name),
		}
		t.walkRemote(child)
		if !entry.IsDir && t.options.showMembers && t.open[entry.Path] {
			t.appendMembers(entry.Path, node.depth+2)
		}
	}
}

func sortRemoteEntries(entries []openidea.RemoteDirectoryEntry, foldersFirst bool, sortBy string) {
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if foldersFirst && left.IsDir != right.IsDir {
			return left.IsDir
		}
		leftName := left.Name
		if leftName == "" {
			leftName = path.Base(left.Path)
		}
		rightName := right.Name
		if rightName == "" {
			rightName = path.Base(right.Path)
		}
		if sortBy == "type" {
			leftExt := strings.ToLower(path.Ext(leftName))
			rightExt := strings.ToLower(path.Ext(rightName))
			if compared := compareProjectNames(leftExt, rightExt); compared != 0 {
				return compared < 0
			}
		}
		return compareProjectNames(leftName, rightName) < 0
	})
}

func (t *Tree) revealRemote(filePath string) {
	root := t.remoteRoot()
	if root == "" || filePath == "" {
		return
	}
	relative := strings.TrimPrefix(path.Clean(filePath), strings.TrimSuffix(root, "/"))
	relative = strings.TrimPrefix(relative, "/")
	if relative == "" {
		return
	}
	current := root
	parts := strings.Split(relative, "/")
	for i, part := range parts {
		current = path.Join(current, part)
		t.open[root] = true
		if i == len(parts)-1 {
			break
		}
		t.open[current] = true
		delete(t.collapsed, current)
		t.loadRemote(current)
	}
	t.rebuild()
}

func (t *Tree) openRemoteUnder(dir string) {
	if !t.open[dir] {
		return
	}
	entries, loaded := t.remoteEntries[dir]
	if !loaded {
		t.loadRemote(dir)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir || !t.entryVisible(entry.Path, entry.Name, true) {
			continue
		}
		t.open[entry.Path] = true
		delete(t.collapsed, entry.Path)
		t.openRemoteUnder(entry.Path)
	}
}

func (t *Tree) remoteExpandSettled(dir string) bool {
	if t.remoteLoading[dir] {
		return false
	}
	entries, loaded := t.remoteEntries[dir]
	if !loaded {
		t.loadRemote(dir)
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir || !t.entryVisible(entry.Path, entry.Name, true) {
			continue
		}
		if !t.open[entry.Path] {
			t.open[entry.Path] = true
			t.loadRemote(entry.Path)
			return false
		}
		if !t.remoteExpandSettled(entry.Path) {
			return false
		}
	}
	return true
}

func tildifyRemotePath(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

func (m *Model) projectTreeUsesRemotePaths() bool {
	// The active session is the authority. During a connection the session is
	// installed just before the remote tree, and a remote editor must still
	// reload through the backend in that interval.
	return remoteConnected(m)
}

func (m *Model) projectTreeCaseInsensitive() bool {
	if !m.projectTreeUsesRemotePaths() {
		return caseInsensitiveFileSystem()
	}
	return m.remote.session != nil && strings.EqualFold(m.remote.session.OS, "windows")
}

func (m *Model) projectTreeNameKey(value string) string {
	if m.projectTreeCaseInsensitive() {
		return strings.ToLower(value)
	}
	return value
}

func (m *Model) projectTreeNamesEqual(left, right string) bool {
	if m.projectTreeCaseInsensitive() {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (m *Model) projectTreeSamePath(left, right string) bool {
	return m.projectTreeNamesEqual(
		m.projectTreePathClean(left),
		m.projectTreePathClean(right),
	)
}

func (m *Model) projectTreeUnderPath(candidate, parent string) bool {
	candidate = m.projectTreePathClean(candidate)
	parent = m.projectTreePathClean(parent)
	if m.projectTreeCaseInsensitive() {
		candidate, parent = strings.ToLower(candidate), strings.ToLower(parent)
	}
	separator := "/"
	if !m.projectTreeUsesRemotePaths() {
		separator = string(filepath.Separator)
	}
	prefix := parent
	if !strings.HasSuffix(prefix, separator) {
		prefix += separator
	}
	return candidate == parent || strings.HasPrefix(candidate, prefix)
}

func (m *Model) projectTreePathClean(value string) string {
	if m.projectTreeUsesRemotePaths() {
		return path.Clean(value)
	}
	return filepath.Clean(value)
}

func (m *Model) projectTreePathBase(value string) string {
	if m.projectTreeUsesRemotePaths() {
		return path.Base(value)
	}
	return filepath.Base(value)
}

func (m *Model) projectTreePathDir(value string) string {
	if m.projectTreeUsesRemotePaths() {
		return path.Dir(value)
	}
	return filepath.Dir(value)
}

func (m *Model) projectTreePathJoin(elements ...string) string {
	if m.projectTreeUsesRemotePaths() {
		return path.Join(elements...)
	}
	return filepath.Join(elements...)
}

func (m *Model) projectTreePathRelative(base, target string) (string, error) {
	if !m.projectTreeUsesRemotePaths() {
		return filepath.Rel(base, target)
	}
	base = path.Clean(base)
	target = path.Clean(target)
	if target == base {
		return ".", nil
	}
	if strings.HasPrefix(target, strings.TrimSuffix(base, "/")+"/") {
		return strings.TrimPrefix(target, strings.TrimSuffix(base, "/")+"/"), nil
	}
	return "", fmt.Errorf("%s is not below %s", target, base)
}

func (m *Model) projectTreeIsDir(value string) bool {
	if m != nil && m.projectTreeUsesRemotePaths() && m.tree == nil {
		return false
	}
	if m != nil && m.tree != nil {
		return m.tree.IsDir(value)
	}
	info, err := os.Stat(value)
	return err == nil && info.IsDir()
}

func (m *Model) projectTreeEntryKnown(value string) bool {
	if !m.projectTreeUsesRemotePaths() {
		_, err := os.Stat(value)
		return err == nil
	}
	if m.tree == nil {
		return false
	}
	value = path.Clean(value)
	if value == m.tree.remoteRoot() {
		return true
	}
	for _, entries := range m.tree.remoteEntries {
		for _, entry := range entries {
			if path.Clean(entry.Path) == value {
				return true
			}
		}
	}
	return false
}

func (m *Model) projectTreeEntryNames(dir string) []string {
	if !m.projectTreeUsesRemotePaths() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names
	}
	entries := m.tree.remoteEntries[path.Clean(dir)]
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name
		if name == "" {
			name = path.Base(entry.Path)
		}
		names = append(names, name)
	}
	return names
}
