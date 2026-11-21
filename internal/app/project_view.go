package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type projectViewModeSpec struct {
	id    string
	label string
}

var projectViewModeSpecs = []projectViewModeSpec{
	{id: "project", label: "Project"},
	{id: "projectFiles", label: "Project Files"},
	{id: "openFiles", label: "Open Files"},
	{id: "changedFiles", label: "All Changed Files"},
	{id: "scratches", label: "Scratches and Consoles"},
	{id: "unity", label: "Unity"},
}

type projectTreeOptionSpec struct {
	id    string
	label string
}

var projectTreeAppearanceOptions = []projectTreeOptionSpec{
	{id: "showHidden", label: "Show Hidden Files"},
	{id: "showExcluded", label: "Show Excluded Files"},
	{id: "showMembers", label: "Show Members"},
	{id: "showRootPaths", label: "Show Root Paths"},
	{id: "compact", label: "Compact Middle Packages"},
	{id: "foldersFirst", label: "Sort Folders First"},
}

var projectTreeBehaviorOptions = []projectTreeOptionSpec{
	{id: "autoscrollFromSource", label: "Always Select Opened File"},
	{id: "autoscrollToSource", label: "Autoscroll to Source"},
	{id: "singleClickOpen", label: "Open Files with Single Click"},
	{id: "singleClickExpand", label: "Expand Directories with Single Click"},
	{id: "previewTab", label: "Enable Preview Tab"},
}

var projectTreeUnityOptions = []projectTreeOptionSpec{
	{id: "showUnityMeta", label: "Show Meta Files"},
	{id: "showUnityGenerated", label: "Show Generated Files"},
}

var projectTreeSortSpecs = []projectTreeOptionSpec{
	{id: "name", label: "Sort by Name"},
	{id: "type", label: "Sort by Type"},
	{id: "modified", label: "Sort by Modification Time"},
}

func projectViewModeLabel(mode string) string {
	for _, spec := range projectViewModeSpecs {
		if spec.id == mode {
			return spec.label
		}
	}
	return "Project"
}

func projectTreeCommands() []appCommand {
	out := make([]appCommand, 0, len(projectTreeAppearanceOptions)+len(projectTreeBehaviorOptions)+
		len(projectTreeUnityOptions)+len(projectViewModeSpecs)+len(projectTreeSortSpecs))
	options := append(append([]projectTreeOptionSpec(nil), projectTreeAppearanceOptions...),
		projectTreeBehaviorOptions...)
	options = append(options, projectTreeUnityOptions...)
	for _, spec := range options {
		id, label := spec.id, spec.label
		out = append(out, appCommand{
			id:       "project.option." + id,
			title:    "Project View: " + label,
			group:    "View",
			keywords: "project tree view option toggle " + id,
			run: func(m *Model) {
				m.SetProjectTreeOption(id, !m.projectTreeOption(id))
			},
		})
	}
	for _, spec := range projectViewModeSpecs {
		mode, label := spec.id, spec.label
		out = append(out, appCommand{
			id:       "project.viewMode." + mode,
			title:    "Project View: Show " + label,
			group:    "View",
			keywords: "project pane scope view as " + mode,
			run:      func(m *Model) { m.SetProjectViewMode(mode) },
		})
	}
	for _, spec := range projectTreeSortSpecs {
		sortBy, label := spec.id, spec.label
		out = append(out, appCommand{
			id:       "project.sortBy." + sortBy,
			title:    "Project View: " + label,
			group:    "View",
			keywords: "project tree sort order " + sortBy,
			run:      func(m *Model) { m.SetProjectTreeSort(sortBy) },
		})
	}
	return out
}

func (m *Model) projectTreeOption(id string) bool {
	p := m.prefs.ProjectTree
	switch id {
	case "showHidden":
		return p.ShowHidden
	case "showExcluded":
		return p.showsExcluded()
	case "showMembers":
		return p.ShowMembers
	case "showRootPaths":
		return p.showsRootPaths()
	case "compact":
		return p.compacts()
	case "foldersFirst":
		return p.foldersFirst()
	case "autoscrollFromSource":
		return p.AutoscrollFromSource
	case "autoscrollToSource":
		return p.AutoscrollToSource
	case "singleClickOpen":
		return p.SingleClickOpen
	case "singleClickExpand":
		return p.SingleClickExpand
	case "previewTab":
		return p.PreviewTab
	case "showUnityMeta":
		return p.ShowUnityMeta
	case "showUnityGenerated":
		return p.ShowUnityGenerated
	default:
		return false
	}
}

// SetProjectTreeOption applies one Project View toggle through the registry
// action path and saves it like OpenIDEA's localStorage-backed options.
func (m *Model) SetProjectTreeOption(id string, enabled bool) {
	p := &m.prefs.ProjectTree
	switch id {
	case "showHidden":
		p.ShowHidden = enabled
	case "showExcluded":
		p.ShowExcluded = boolPointer(enabled)
	case "showMembers":
		p.ShowMembers = enabled
	case "showRootPaths":
		p.ShowRootPaths = boolPointer(enabled)
	case "compact":
		p.Compact = boolPointer(enabled)
	case "foldersFirst":
		p.FoldersFirst = boolPointer(enabled)
	case "autoscrollFromSource":
		p.AutoscrollFromSource = enabled
	case "autoscrollToSource":
		p.AutoscrollToSource = enabled
	case "singleClickOpen":
		p.SingleClickOpen = enabled
	case "singleClickExpand":
		p.SingleClickExpand = enabled
	case "previewTab":
		p.PreviewTab = enabled
	case "showUnityMeta":
		p.ShowUnityMeta = enabled
	case "showUnityGenerated":
		p.ShowUnityGenerated = enabled
	default:
		return
	}
	m.applyProjectTreeOptions()
	m.savePreferences()
	if id == "autoscrollFromSource" && enabled {
		m.followActiveInProjectView(true)
	}
	if id == "showMembers" && enabled {
		m.loadExpandedProjectTreeMembers()
	}
	m.invalidate()
}

// SetProjectTreeSort chooses Name, Type or Modification Time ordering.
func (m *Model) SetProjectTreeSort(sortBy string) {
	switch sortBy {
	case "name", "type", "modified":
	default:
		return
	}
	m.prefs.ProjectTree.SortBy = sortBy
	m.applyProjectTreeOptions()
	m.savePreferences()
	m.invalidate()
}

// SetProjectViewMode switches Project View's pane. Unity is unavailable when
// the open folder has no Unity project file.
func (m *Model) SetProjectViewMode(mode string) {
	if mode == "unity" && m.projectUnityVersion == "" {
		return
	}
	switch mode {
	case "project", "projectFiles", "openFiles", "changedFiles", "scratches", "unity":
	default:
		return
	}
	m.prefs.ProjectTree.Mode = mode
	m.savePreferences()
	if m.tree != nil {
		m.syncProjectTreeState()
		m.tree.SetMode(mode)
	}
	m.sideMode = ""
	m.leftHidden = false
	m.rememberLayout()
	m.invalidate()
}

func (m *Model) applyProjectTreeOptions() {
	if m.tree == nil {
		return
	}
	m.tree.SetOptions(treeOptionsFromPrefs(m.prefs.ProjectTree))
	m.syncProjectTreeState()
	m.invalidate()
}

// configureProjectTreeView restores global options and this project's pane
// state after a project opens.
func (m *Model) configureProjectTreeView() {
	if m.tree == nil {
		return
	}
	root := m.Root()
	if root == "" {
		return
	}
	m.projectUnityVersion = unityProjectVersion(root)
	mode := m.prefs.ProjectTree.normalizedMode()
	if mode == "unity" && m.projectUnityVersion == "" {
		mode = "project"
	} else if mode == "project" && m.prefs.ProjectTree.Mode == "" && m.projectUnityVersion != "" {
		// OpenIDEA opens a newly recognized Unity project in Unity view
		// while keeping Project Files available in the pane menu.
		mode = "unity"
	}
	m.prefs.ProjectTree.Mode = mode

	scratchRoot := ""
	if dir, err := m.scratchDir(); err == nil {
		scratchRoot = dir
		_ = os.MkdirAll(scratchRoot, 0o755)
	}
	m.tree.SetScratchRoot(scratchRoot)
	m.tree.SetOptions(treeOptionsFromPrefs(m.prefs.ProjectTree))
	m.tree.SetExpanded(m.projectViews[layoutKey(root)])
	m.tree.SetExpandedChanged(func(paths []string) {
		m.rememberProjectTreeExpanded(root, paths)
	})
	m.tree.SetMode(mode)
	m.syncProjectTreeState()
	if m.prefs.ProjectTree.ShowMembers {
		m.loadExpandedProjectTreeMembers()
	}
	m.savePreferences()
}

func (m *Model) loadExpandedProjectTreeMembers() {
	if m.tree == nil || !m.prefs.ProjectTree.ShowMembers {
		return
	}
	for _, path := range m.tree.Expanded() {
		if !strings.Contains(path, "\x00") && !m.tree.IsDir(path) {
			m.ensureProjectTreeMembers(path)
		}
	}
}

func (m *Model) rememberProjectTreeExpanded(root string, paths []string) {
	if root == "" {
		return
	}
	key := layoutKey(root)
	if m.projectViews == nil {
		m.projectViews = map[string][]string{}
	}
	if equalProjectTreePaths(m.projectViews[key], paths) {
		return
	}
	m.projectViews[key] = append([]string(nil), paths...)
	m.saveSession()
}

func unityProjectVersion(root string) string {
	settings := filepath.Join(root, "ProjectSettings", "ProjectVersion.txt")
	raw, err := os.ReadFile(settings)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "m_EditorVersion:"); ok {
			if version := strings.TrimSpace(value); version != "" {
				return version
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "Assets")); err == nil {
		return "Unity"
	}
	return ""
}

// syncProjectTreeState feeds the tree the current tabs and VCS paths. It is
// cheap when nothing changed, so the workbench can call it before drawing.
func (m *Model) syncProjectTreeState() {
	if m.tree == nil {
		return
	}
	m.tree.SetFilteredFiles(m.projectTreeOpenFiles(), m.projectTreeChangedFiles())
	for _, tab := range m.projectTreeFileTabs() {
		if tab.outlineDoc == tab.doc.ID() {
			m.tree.SetOutline(tab.doc.Path(), tab.outline)
		}
	}
	if m.prefs.ProjectTree.AutoscrollFromSource {
		m.followActiveInProjectView(false)
	}
}

func (m *Model) projectTreeFileTabs() []*Tab {
	out := make([]*Tab, 0, len(m.tabs)+1)
	out = append(out, m.tabs...)
	if m.split != nil {
		out = append(out, m.split.tabs...)
	}
	return out
}

func (m *Model) projectTreeOpenFiles() []string {
	seen := map[string]bool{}
	var out []string
	for _, tab := range m.projectTreeFileTabs() {
		if tab == nil || tab.kind != "" || tab.doc == nil {
			continue
		}
		path := tab.doc.Path()
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func (m *Model) projectTreeChangedFiles() []string {
	seen := map[string]bool{}
	var out []string
	for _, file := range m.vcs.files {
		if file.Path == "" || seen[file.Path] {
			continue
		}
		seen[file.Path] = true
		out = append(out, file.Path)
	}
	sortProjectTreePathList(out)
	return out
}

func sortProjectTreePathList(paths []string) {
	sort.Strings(paths)
}

func (m *Model) followActiveInProjectView(reveal bool) {
	if m.tree == nil {
		return
	}
	tab := m.tab()
	if tab == nil || tab.kind != "" || tab.doc == nil {
		return
	}
	path := tab.doc.Path()
	if path == "" || m.tree.Selected() == path {
		return
	}
	if reveal && m.prefs.ProjectTree.Mode != "openFiles" &&
		m.prefs.ProjectTree.Mode != "changedFiles" &&
		m.prefs.ProjectTree.Mode != "scratches" {
		m.tree.reveal(path)
	}
	m.tree.Select(path)
}

// ensureProjectTreeMembers loads Show Members lazily, as OpenIDEA's
// withDocument-backed outline loader does.
func (m *Model) ensureProjectTreeMembers(path string) {
	if m.tree == nil || path == "" {
		return
	}
	if _, loaded := m.tree.Outline(path); loaded {
		return
	}
	if _, loading := m.projectTreeMembersLoading[path]; loading {
		return
	}
	if m.projectTreeMembersLoading == nil {
		m.projectTreeMembersLoading = map[string]bool{}
	}
	m.projectTreeMembersLoading[path] = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var items []openidea.OutlineItem
		if m.reg != nil {
			doc, err := m.reg.Open(ctx, path)
			if err == nil {
				raw, outlineErr := m.invoke(ctx, "doc_outline", map[string]any{"doc": doc.ID()})
				if outlineErr == nil {
					_ = json.Unmarshal(raw, &items)
				}
				_ = m.reg.Close(ctx, path)
			}
		}
		m.post(func() {
			delete(m.projectTreeMembersLoading, path)
			if m.tree != nil {
				m.tree.SetOutline(path, items)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) projectTreeTitle(c *ui.Context, p *ui.Theme, oi oiColors) {
	title := projectViewModeLabel(m.prefs.ProjectTree.normalizedMode())
	button := ui.ButtonBase(c.Key("panel:project-mode")).Height(24).Padding(0, 4).
		Gap(3).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Tooltip("Select Project View mode")
	button.Children(func() {
		ui.Text(c, title).FontSize(13).TextColor(oi.textStrong).SingleLine()
		oiIconEl(c, "general/chevronDown", 14, oi, oi.textMuted)
	})
	button.Menu(func(menu *ui.Menu) { m.projectViewModeMenu(menu) })
}

func (m *Model) projectViewModeMenu(menu *ui.Menu) {
	mode := m.prefs.ProjectTree.normalizedMode()
	for _, spec := range projectViewModeSpecs {
		if spec.id == "unity" && m.projectUnityVersion == "" {
			continue
		}
		if menu.Item(spec.label).Checked(spec.id == mode).Chosen() {
			m.SetProjectViewMode(spec.id)
		}
	}
}

func (m *Model) projectViewOptionsButton(c *ui.Context, p *ui.Theme, oi oiColors) {
	button := ui.ButtonBase(c.Key("panel:project-options")).Size(20, 20).Radius(4).
		Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Tooltip("Project View options")
	button.Children(func() { oiIconEl(c, "general/moreVertical", 16, oi, oi.icon) })
	button.Menu(func(menu *ui.Menu) { m.projectViewOptionsMenu(menu) })
}

func (m *Model) projectViewOptionsMenu(menu *ui.Menu) {
	if m.projectUnityVersion != "" && m.prefs.ProjectTree.normalizedMode() == "unity" {
		for _, spec := range projectTreeUnityOptions {
			if menu.Item(spec.label).Checked(m.projectTreeOption(spec.id)).Chosen() {
				m.SetProjectTreeOption(spec.id, !m.projectTreeOption(spec.id))
			}
		}
		menu.Separator()
	}
	for _, spec := range projectTreeAppearanceOptions {
		if menu.Item(spec.label).Checked(m.projectTreeOption(spec.id)).Chosen() {
			m.SetProjectTreeOption(spec.id, !m.projectTreeOption(spec.id))
		}
	}
	menu.Separator()
	menu.Submenu("Sort By", func(sub *ui.Menu) {
		sortBy := m.prefs.ProjectTree.normalizedSort()
		for _, spec := range projectTreeSortSpecs {
			if sub.Item(strings.TrimPrefix(spec.label, "Sort by ")).Checked(spec.id == sortBy).Chosen() {
				m.SetProjectTreeSort(spec.id)
			}
		}
	})
	menu.Separator()
	for _, spec := range projectTreeBehaviorOptions {
		if menu.Item(spec.label).Checked(m.projectTreeOption(spec.id)).Chosen() {
			m.SetProjectTreeOption(spec.id, !m.projectTreeOption(spec.id))
		}
	}
}
