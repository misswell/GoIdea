package app

// The Welcome screen's recent-project model: a project carries what the row
// shows (name, open time, branch, remote, group), groups are user-made
// headings, and the search is a subsequence match over the name and path with
// the matched letters returned for highlighting.

import (
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
	return normalizeRecentProjects(out)
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
	q := strings.ToLower(strings.TrimSpace(m.welcomeQuery))
	if q == "" {
		out := make([]recentMatch, 0, len(m.recents))
		for _, p := range m.recents {
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

// WelcomeSelect moves the row Enter opens; the ends hold.
func (m *Model) WelcomeSelect(delta int) {
	list := m.filteredRecents()
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

// WelcomeOpenSelected opens the selected row, as Enter does.
func (m *Model) WelcomeOpenSelected() {
	list := m.filteredRecents()
	if len(list) == 0 {
		return
	}
	if m.welcomeSel >= len(list) {
		m.welcomeSel = len(list) - 1
	}
	m.confirmSwitch(list[m.welcomeSel].project.Path)
}

// WelcomeClearQuery empties the search field and the selection.
func (m *Model) WelcomeClearQuery() {
	m.welcomeQuery = ""
	m.welcomeSel = 0
	m.invalidate()
}

// recentGroupNamed is the stored group, when it exists.
func (m *Model) recentGroupNamed(name string) (int, RecentGroup, bool) {
	for i, g := range m.recentGroups {
		if g.Name == name {
			return i, g, true
		}
	}
	return 0, RecentGroup{}, false
}

// CreateRecentGroup adds a group; the name is trimmed and unique.
func (m *Model) CreateRecentGroup(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if _, _, ok := m.recentGroupNamed(name); ok {
		return
	}
	m.recentGroups = append(m.recentGroups, RecentGroup{Name: name})
	m.saveRecents()
	m.invalidate()
}

// RenameRecentGroup renames a group and moves its projects with it.
func (m *Model) RenameRecentGroup(old, name string) {
	name = strings.TrimSpace(name)
	if old == "" || name == "" || old == name {
		return
	}
	if _, _, ok := m.recentGroupNamed(name); ok {
		return
	}
	for i := range m.recentGroups {
		if m.recentGroups[i].Name == old {
			m.recentGroups[i].Name = name
		}
	}
	for i := range m.recents {
		if m.recents[i].Group == old {
			m.recents[i].Group = name
		}
	}
	m.saveRecents()
	m.invalidate()
}

// DeleteRecentGroup removes the heading; its projects stay, ungrouped.
func (m *Model) DeleteRecentGroup(name string) {
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
	m.saveRecents()
	m.invalidate()
}

// ToggleRecentGroup collapses or expands a group's rows.
func (m *Model) ToggleRecentGroup(name string) {
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
	if group != "" {
		if _, _, ok := m.recentGroupNamed(group); !ok {
			m.recentGroups = append(m.recentGroups, RecentGroup{Name: group})
		}
	}
	for i := range m.recents {
		if m.recents[i].Path == path {
			m.recents[i].Group = group
		}
	}
	m.saveRecents()
	m.invalidate()
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
