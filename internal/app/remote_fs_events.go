package app

import (
	"path"
	"sort"
	"strings"

	"mayoi/internal/openidea"
)

// acceptFsChanged applies the backend's file-system event to the native Project
// View and open editors. OpenIDEA reloads only loaded folders, drops deleted
// listings and keeps unsaved editor text.
func (m *Model) acceptFsChanged(event openidea.FsChangedEvent) {
	if m == nil || m.Root() == "" {
		return
	}
	m.noteExternalLibraryManifestChange(event)
	refresh := changedParentFolders(event)
	if m.tree != nil {
		if len(refresh) == 0 {
			m.tree.Refresh()
		} else {
			for _, dir := range refresh {
				m.tree.RefreshDir(dir)
			}
		}
	}

	for _, file := range uniqueChangedPaths(event.Created, event.Modified) {
		if m.tree != nil {
			m.tree.ResetOutline(file)
		}
		for _, tab := range m.treeTabsUnder(file) {
			if tab == nil || tab.kind != "" || tab.reloading {
				continue
			}
			if tab.unsaved() {
				m.banner = "The file changed outside the IDE: " +
					m.projectTreePathBase(file) + " (the editor's content was kept)"
				continue
			}
			m.reloadTab(tab)
		}
	}

	for _, file := range uniqueChangedPaths(event.Removed) {
		if m.tree != nil {
			m.tree.ResetOutline(file)
		}
		if dirty := m.treeDirtyUnder(file); dirty != "" {
			m.banner = "The file was deleted outside the IDE: " + dirty +
				" (the editor's unsaved content was kept)"
		} else {
			m.closedTreeTabs(file)
		}
		m.removeNavigationTree(file)
	}
	m.dropStaleRunTemporaries(event.Removed)

	if len(event.Created)+len(event.Modified)+len(event.Removed) > 0 && m.vcs.loaded {
		m.RefreshVCS()
	}
	if acceptsSpringRouteChanges(event) {
		m.invalidateSpringRouteIndex()
	}
	if acceptsGraphQLDiscoveryChanges(event) &&
		(m.graphql.discovered || m.graphql.discovering) {
		m.graphql.discovered = false
		m.discoverGraphQLSchemas()
	}
	if gradleCatalogSourceChanged(event) {
		m.invalidateGradleVersionCatalogs()
	}
	m.invalidate()
}

func acceptsSpringRouteChanges(event openidea.FsChangedEvent) bool {
	for _, file := range uniqueChangedPaths(event.Created, event.Modified, event.Removed) {
		switch strings.ToLower(path.Ext(file)) {
		case ".java", ".kt", ".kts":
			return true
		}
	}
	return false
}

func acceptsGraphQLDiscoveryChanges(event openidea.FsChangedEvent) bool {
	for _, file := range uniqueChangedPaths(event.Created, event.Modified, event.Removed) {
		name := path.Base(file)
		if graphqlIsSchemaFile(name) || graphqlIsConfigFile(name) {
			return true
		}
	}
	return false
}

// changedParentFolders is OpenIDEA's planRefresh for the parent directories
// whose listing can change. Remote paths are POSIX paths, while local paths
// use the host separator.
func changedParentFolders(event openidea.FsChangedEvent) []string {
	seen := map[string]bool{}
	for _, file := range uniqueChangedPaths(event.Created, event.Modified, event.Removed) {
		parent := path.Dir(file)
		if parent == "." || parent == "" {
			continue
		}
		seen[parent] = true
	}
	out := make([]string, 0, len(seen))
	for dir := range seen {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}

func uniqueChangedPaths(groups ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, group := range groups {
		for _, item := range group {
			item = strings.TrimSpace(item)
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}
