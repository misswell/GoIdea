package app

// The Welcome screen's recent-project model: a project carries what the row
// shows (name, open time, branch, remote, group), groups are user-made
// headings, and the search is a subsequence match over the name and path with
// the matched letters returned for highlighting.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// welcomeGroupDialog is the New Group / Rename Group prompt.
type welcomeGroupDialog struct {
	open   bool
	rename string
	text   string
}

// sessionRecents reads the session's recents, migrating the older path list
// the first time the richer model is read.
func sessionRecents(s Session) []RecentProject {
	if len(s.Recents) > 0 {
		return normalizeRecentProjects(s.Recents)
	}
	out := make([]RecentProject, 0, len(s.RecentProjects))
	now := time.Now().UnixMilli()
	for i, path := range s.RecentProjects {
		out = append(out, RecentProject{Path: path, Name: filepath.Base(path),
			OpenedAt: now - int64(i)*1000})
	}
	return normalizeStoredRecents(s.RecentGroups, out)
}

// recentPaths is the folder list older sessions keep reading.
func recentPaths(list []RecentProject) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.Path)
	}
	return out
}

// recentMatch is one row of the filtered list: the project, the letters the
// query matched in its name and path, and the score that ordered it.
type recentMatch struct {
	project RecentProject
	nameIdx []int
	pathIdx []int
	score   int
}

// recentRow is one visible row of Welcome › Projects. Groups appear before
// their members without a query; a query shows a flat list of matches.
type recentRow struct {
	group *RecentGroup
	match *recentMatch
	count int
}

// fuzzySubsequence matches query as a subsequence of text, case insensitively,
// and returns the matched byte offsets. Consecutive and word-start matches
// score higher.
func fuzzySubsequence(query, text string) (idx []int, score int, ok bool) {
	if query == "" {
		return nil, 0, true
	}
	q := []rune(strings.ToLower(query))
	t := []rune(strings.ToLower(text))
	if len(q) > len(t) {
		return nil, 0, false
	}
	out := make([]int, 0, len(q))
	qi := 0
	prev := -2
	for ti, r := range t {
		if qi < len(q) && r == q[qi] {
			// Byte offset of the rune, for the text renderer.
			off := len(string(t[:ti]))
			out = append(out, off)
			score += 10
			if ti == prev+1 {
				score += 8
			}
			if ti == 0 || !isWordRune(t[ti-1]) {
				score += 6
			}
			prev = ti
			qi++
		}
	}
	if qi != len(q) {
		return nil, 0, false
	}
	return out, score, true
}

func isWordRune(r rune) bool {
	return r == '_' || r == '-' || r == '.' || r == '/' || r >= '0' && r <= '9' ||
		r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// filteredRecents is the Welcome list for the typed query: name matches score
// above path matches, ties keep the stored order.
func (m *Model) filteredRecents() []recentMatch {
	return m.filteredRecentsFor(m.welcomeQuery, "")
}

// filteredRecentsFor is the same recent-project search over an optional query,
// hiding one path as the File › Open Recent popup hides the current project.
func (m *Model) filteredRecentsFor(query, exclude string) []recentMatch {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		out := make([]recentMatch, 0, len(m.recents))
		for _, p := range m.recents {
			if exclude != "" && p.Path == exclude {
				continue
			}
			name := p.Name
			if name == "" {
				name = filepath.Base(p.Path)
			}
			out = append(out, recentMatch{project: p})
		}
		return out
	}
	var out []recentMatch
	for _, p := range m.recents {
		if exclude != "" && p.Path == exclude {
			continue
		}
		name := p.Name
		if name == "" {
			name = filepath.Base(p.Path)
		}
		// Group names match too, so typing a group lists its projects.
		if p.Group != "" && strings.Contains(strings.ToLower(p.Group), q) {
			out = append(out, recentMatch{project: p, score: 1})
			continue
		}
		nameIdx, nameScore, nameOK := fuzzySubsequence(q, name)
		pathIdx, pathScore, pathOK := fuzzySubsequence(q, p.Path)
		if !nameOK && !pathOK {
			continue
		}
		mt := recentMatch{project: p}
		if nameOK {
			mt.nameIdx = nameIdx
			mt.score = nameScore * 4
		}
		if pathOK {
			mt.pathIdx = pathIdx
			mt.score += pathScore
		}
		out = append(out, mt)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].project.OpenedAt > out[j].project.OpenedAt
	})
	return out
}

// recentRows applies OpenIDEA's Welcome list order. Groups come first, with
// their expanded members beneath them, followed by every ungrouped project.
// Searching keeps the flat filtered result.
func (m *Model) recentRows() []recentRow {
	list := m.filteredRecents()
	if strings.TrimSpace(m.welcomeQuery) != "" || len(m.recentGroups) == 0 {
		out := make([]recentRow, 0, len(list))
		for i := range list {
			out = append(out, recentRow{match: &list[i]})
		}
		return out
	}
	rows := make([]recentRow, 0, len(list)+len(m.recentGroups))
	grouped := make(map[string]bool, len(list))
	for i := range m.recentGroups {
		group := m.recentGroups[i]
		shown := make([]recentMatch, 0, 4)
		for _, match := range list {
			if match.project.Group == group.Name {
				shown = append(shown, match)
				grouped[match.project.Path] = true
			}
		}
		rows = append(rows, recentRow{group: &m.recentGroups[i], count: len(shown)})
		if group.Collapsed || len(shown) == 0 {
			continue
		}
		for i := range shown {
			rows = append(rows, recentRow{match: &shown[i]})
		}
	}
	for _, match := range list {
		if !grouped[match.project.Path] {
			match := match
			rows = append(rows, recentRow{match: &match})
		}
	}
	return rows
}

// WelcomeSelect moves the row Enter opens; the ends hold.
func (m *Model) WelcomeSelect(delta int) {
	list := m.recentProjectRows()
	if len(list) == 0 {
		m.welcomeSel = 0
		return
	}
	m.welcomeSel += delta
	if m.welcomeSel < 0 {
		m.welcomeSel = 0
	}
	if m.welcomeSel >= len(list) {
		m.welcomeSel = len(list) - 1
	}
	m.invalidate()
}

func (m *Model) recentProjectRows() []recentRow {
	rows := m.recentRows()
	out := rows[:0]
	for _, row := range rows {
		if row.group == nil {
			out = append(out, row)
		}
	}
	return out
}

// WelcomeOpenSelected opens the selected row, as Enter does.
func (m *Model) WelcomeOpenSelected() {
	list := m.recentProjectRows()
	if len(list) == 0 {
		return
	}
	if m.welcomeSel >= len(list) {
		m.welcomeSel = len(list) - 1
	}
	row := list[m.welcomeSel]
	m.OpenRecentProject(row.match.project.Path)
}

// WelcomeClearQuery empties the search field and the selection.
func (m *Model) WelcomeClearQuery() {
	m.welcomeQuery = ""
	m.welcomeSel = 0
	m.invalidate()
}

// recentGroupNamed is the stored group, when it exists.
func (m *Model) recentGroupNamed(name string) (int, RecentGroup, bool) {
	name = strings.TrimSpace(name)
	for i, g := range m.recentGroups {
		if g.Name == name {
			return i, g, true
		}
	}
	return 0, RecentGroup{}, false
}

// normalizeRecentGroups trims and de-duplicates stored names, keeps each
// project in at most its first group, and leaves unknown memberships
// ungrouped.
func normalizeRecentGroups(groups []RecentGroup, projects []RecentProject) ([]RecentGroup, []RecentProject) {
	projects = append([]RecentProject(nil), projects...)
	seenPaths := make(map[string]bool, len(projects))
	deduped := projects[:0]
	for _, project := range projects {
		if project.Path == "" || seenPaths[project.Path] {
			continue
		}
		seenPaths[project.Path] = true
		deduped = append(deduped, project)
	}
	projects = deduped
	valid := make(map[string]bool, len(groups))
	normalized := make([]RecentGroup, 0, len(groups))
	for _, group := range groups {
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" || valid[group.Name] {
			continue
		}
		valid[group.Name] = true
		normalized = append(normalized, group)
	}
	for i := range projects {
		projects[i].Group = strings.TrimSpace(projects[i].Group)
		if !valid[projects[i].Group] {
			projects[i].Group = ""
		}
	}
	return normalized, projects
}

// normalizeStoredRecents is the load path: groups are sanitized before the
// ungrouped-only retention cap is applied, so grouped rows survive.
func normalizeStoredRecents(groups []RecentGroup, projects []RecentProject) []RecentProject {
	groups, stored := normalizeRecentGroups(groups, projects)
	stored = normalizeRecentProjects(stored)
	_, stored = normalizeRecentGroups(groups, stored)
	return stored
}

// setRecentGroupsAndProjects stores one normalized group/project pair and
// refreshes any menu that lists recents.
func (m *Model) setRecentGroupsAndProjects(groups []RecentGroup, projects []RecentProject) {
	normalizedGroups, stored := normalizeRecentGroups(groups, projects)
	stored = normalizeRecentRows(stored)
	m.recentGroups, m.recents = normalizeRecentGroups(normalizedGroups, stored)
	m.windowMenuChanged()
}

// groupNameError is the inline validation of an input dialog.
func (m *Model) groupNameError(value, current string) string {
	name := strings.TrimSpace(value)
	if name == "" {
		return "Enter a group name"
	}
	if name == current {
		return ""
	}
	if _, _, ok := m.recentGroupNamed(name); ok {
		return fmt.Sprintf("A group named '%s' already exists", name)
	}
	return ""
}

// CreateRecentGroup adds a group; the name is trimmed and unique.
func (m *Model) CreateRecentGroup(name string) {
	name = strings.TrimSpace(name)
	if m.groupNameError(name, "") != "" {
		return
	}
	m.recentGroups = append(m.recentGroups, RecentGroup{Name: name})
	m.setRecentGroupsAndProjects(m.recentGroups, m.recents)
	m.saveRecents()
	m.invalidate()
}

// RenameRecentGroup renames a group and moves its projects with it.
func (m *Model) RenameRecentGroup(old, name string) {
	old = strings.TrimSpace(old)
	name = strings.TrimSpace(name)
	if old == "" || name == "" || old == name {
		return
	}
	if m.groupNameError(name, old) != "" {
		return
	}
	found := false
	for i := range m.recentGroups {
		if m.recentGroups[i].Name == old {
			m.recentGroups[i].Name = name
			found = true
		}
	}
	if !found {
		return
	}
	for i := range m.recents {
		if m.recents[i].Group == old {
			m.recents[i].Group = name
		}
	}
	m.setRecentGroupsAndProjects(m.recentGroups, m.recents)
	m.saveRecents()
	m.invalidate()
}

// DeleteRecentGroup removes the heading; its projects stay, ungrouped.
func (m *Model) DeleteRecentGroup(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	kept := m.recentGroups[:0]
	for _, g := range m.recentGroups {
		if g.Name != name {
			kept = append(kept, g)
		}
	}
	m.recentGroups = kept
	for i := range m.recents {
		if m.recents[i].Group == name {
			m.recents[i].Group = ""
		}
	}
	m.setRecentGroupsAndProjects(m.recentGroups, m.recents)
	m.saveRecents()
	m.invalidate()
}

// ToggleRecentGroup collapses or expands a group's rows.
func (m *Model) ToggleRecentGroup(name string) {
	name = strings.TrimSpace(name)
	for i := range m.recentGroups {
		if m.recentGroups[i].Name == name {
			m.recentGroups[i].Collapsed = !m.recentGroups[i].Collapsed
			m.saveRecents()
			m.invalidate()
			return
		}
	}
}

// MoveRecentToGroup assigns a project to a group, creating it when asked.
func (m *Model) MoveRecentToGroup(path, group string) {
	group = strings.TrimSpace(group)
	if path == "" {
		return
	}
	remote := false
	for _, project := range m.recents {
		if project.Path == path {
			remote = project.Remote != "" ||
				strings.HasPrefix(path, "ssh://") ||
				strings.HasPrefix(path, "wsl://") ||
				strings.HasPrefix(path, "devcontainer://")
			break
		}
	}
	if remote {
		return
	}
	if group != "" {
		if _, _, ok := m.recentGroupNamed(group); !ok {
			if m.groupNameError(group, "") != "" {
				group = ""
			} else {
				m.recentGroups = append(m.recentGroups, RecentGroup{Name: group})
			}
		}
	}
	for i := range m.recents {
		if m.recents[i].Path == path {
			m.recents[i].Group = group
		}
	}
	m.setRecentGroupsAndProjects(m.recentGroups, m.recents)
	m.saveRecents()
	m.invalidate()
}

// OpenRecentInNewWindow opens a local or remote recent in an independent
// current window when the host has no multi-window support.
func (m *Model) OpenRecentInNewWindow(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	if strings.HasPrefix(path, "ssh://") ||
		strings.HasPrefix(path, "wsl://") ||
		strings.HasPrefix(path, "devcontainer://") ||
		strings.HasPrefix(strings.ReplaceAll(path, `\`, "/"), "//wsl$") ||
		strings.HasPrefix(strings.ReplaceAll(path, `\`, "/"), "//wsl.localhost") {
		if _, _, remote := parseRemoteProjectLocation(path); !remote {
			m.status = "The remote project location is invalid"
			m.invalidate()
			return
		}
		if !m.openProjectInNativeWindow(path, nil) {
			target, root, _ := parseRemoteProjectLocation(path)
			m.openRemoteLocation(target, root)
		}
		return
	}
	if !m.RunActionWithArg("file.openInNewWindow", path) {
		m.OpenRecentProject(path)
	}
}

// welcomeGroupCount is the projects a group holds.
func (m *Model) welcomeGroupCount(name string) int {
	n := 0
	for _, p := range m.recents {
		if p.Group == name {
			n++
		}
	}
	return n
}
