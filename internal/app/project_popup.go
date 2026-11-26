package app

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// projectPopup mirrors OpenIDEA's title-bar ProjectWidget popup. It combines
// project actions, every open native project window and recent projects, with
// type-to-filter and keyboard selection.
type projectPopup struct {
	anchor ui.Rect
	query  string
	active int
	reveal bool
	focus  ui.Handle
	list   ui.ListState
}

type projectPopupEntryKind uint8

const (
	projectPopupActionEntry projectPopupEntryKind = iota
	projectPopupProjectEntry
)

type projectPopupEntry struct {
	kind        projectPopupEntryKind
	actionID    string
	label       string
	icon        string
	project     RecentProject
	match       recentMatch
	open        bool
	firstOpen   bool
	firstRecent bool
}

var projectPopupActions = []projectPopupEntry{
	{
		kind: projectPopupActionEntry, actionID: "file.newProject",
		label: "New Project…", icon: "general/add",
	},
	{
		kind: projectPopupActionEntry, actionID: "file.openProject",
		label: "Open…", icon: "nodes/folder",
	},
	{
		kind: projectPopupActionEntry, actionID: "vcs.clone",
		label: "Clone Repository…", icon: "general/vcs",
	},
	{
		kind: projectPopupActionEntry, actionID: "remote.connect",
		label: "Remote Development…", icon: "toolwindows/terminal",
	},
}

func (m *Model) openProjectPopup(anchor ui.Rect) {
	if anchor.H <= 0 {
		anchor = ui.Rect{X: 16, Y: oiTitleBarHeight, W: 220, H: oiTitleBarHeight}
	}
	m.projectPopup = &projectPopup{anchor: anchor}
	m.invalidate()
}

func (m *Model) closeProjectPopup() {
	m.projectPopup = nil
	m.invalidate()
}

func projectPopupEntriesFor(m *Model, query string) []projectPopupEntry {
	open := m.openProjectWindows()
	openPaths := make(map[string]bool, len(open))
	openRecents := make([]RecentProject, 0, len(open))
	for _, window := range open {
		if strings.TrimSpace(window.Root) == "" {
			continue
		}
		path := filepath.Clean(window.Root)
		openPaths[path] = true
		name := window.Title
		if name == "" {
			name = filepath.Base(path)
		}
		openRecents = append(openRecents, RecentProject{
			Path: path, Name: name, Branch: window.Branch,
		})
	}
	openMatches := filterProjectPopupRecents(openRecents, query)
	recent := make([]RecentProject, 0, len(m.recents))
	for _, project := range m.recents {
		path := filepath.Clean(project.Path)
		if openPaths[path] {
			continue
		}
		project.Path = path
		recent = append(recent, project)
	}
	recentMatches := filterProjectPopupRecents(recent, query)

	entries := make([]projectPopupEntry, 0,
		len(projectPopupActions)+len(openMatches)+len(recentMatches))
	if strings.TrimSpace(query) == "" {
		for _, action := range projectPopupActions {
			entries = append(entries, action)
		}
	}
	if len(openMatches) > 0 {
		for i, match := range openMatches {
			entries = append(entries, projectPopupEntry{
				kind: projectPopupProjectEntry, project: match.project, match: match,
				open: true, firstOpen: i == 0,
			})
		}
	}
	if len(recentMatches) > 0 {
		for i, match := range recentMatches {
			entries = append(entries, projectPopupEntry{
				kind: projectPopupProjectEntry, project: match.project, match: match,
				firstRecent: i == 0,
			})
		}
	}
	return entries
}

func filterProjectPopupRecents(projects []RecentProject, query string) []recentMatch {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		out := make([]recentMatch, 0, len(projects))
		for _, project := range projects {
			out = append(out, recentMatch{project: projectWithDisplayName(project)})
		}
		return out
	}
	out := make([]recentMatch, 0, len(projects))
	for _, project := range projects {
		project = projectWithDisplayName(project)
		nameIdx, nameScore, nameOK := fuzzySubsequence(query, project.Name)
		pathIdx, pathScore, pathOK := fuzzySubsequence(query, project.Path)
		if !nameOK && !pathOK {
			continue
		}
		mt := recentMatch{project: project}
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

func projectWithDisplayName(project RecentProject) RecentProject {
	if strings.TrimSpace(project.Name) == "" {
		project.Name = filepath.Base(project.Path)
	}
	return project
}

func (m *Model) stepProjectPopup(delta int) {
	p := m.projectPopup
	if p == nil {
		return
	}
	entries := projectPopupEntriesFor(m, p.query)
	if len(entries) == 0 {
		p.active = 0
		m.invalidate()
		return
	}
	p.active = (min(max(p.active, 0), len(entries)-1) + delta + len(entries)) % len(entries)
	p.reveal = true
	m.invalidate()
}

func (m *Model) jumpProjectPopup(end bool) {
	p := m.projectPopup
	if p == nil {
		return
	}
	entries := projectPopupEntriesFor(m, p.query)
	if len(entries) == 0 {
		p.active = 0
	} else if end {
		p.active = len(entries) - 1
	} else {
		p.active = 0
	}
	p.reveal = true
	m.invalidate()
}

func (m *Model) typeProjectPopup(text string) {
	p := m.projectPopup
	if p == nil || text == "" {
		return
	}
	p.query += text
	p.active = 0
	p.reveal = true
	m.invalidate()
}

func (m *Model) eraseProjectPopup() {
	p := m.projectPopup
	if p == nil || p.query == "" {
		return
	}
	runes := []rune(p.query)
	p.query = string(runes[:len(runes)-1])
	p.active = 0
	p.reveal = true
	m.invalidate()
}

func (m *Model) removeSelectedProjectPopupRecent() {
	p := m.projectPopup
	if p == nil {
		return
	}
	entries := projectPopupEntriesFor(m, p.query)
	if len(entries) == 0 {
		return
	}
	p.active = min(max(p.active, 0), len(entries)-1)
	entry := entries[p.active]
	if entry.kind == projectPopupProjectEntry && !entry.open {
		m.projectPopup = nil
		m.askRemoveRecent(entry.project.Path)
	}
}

func (m *Model) chooseProjectPopup() {
	p := m.projectPopup
	if p == nil {
		return
	}
	entries := projectPopupEntriesFor(m, p.query)
	if len(entries) == 0 {
		return
	}
	p.active = min(max(p.active, 0), len(entries)-1)
	entry := entries[p.active]
	m.projectPopup = nil
	if entry.kind == projectPopupActionEntry {
		m.RunAction(entry.actionID)
		return
	}
	if entry.open && m.focusOpenProject(entry.project.Path) {
		m.invalidate()
		return
	}
	m.OpenRecentProject(entry.project.Path)
}

func (m *Model) projectPopupDialog(c *ui.Context, p *ui.Theme) {
	popup := m.projectPopup
	if popup == nil {
		return
	}
	oi := oiPaletteOf(p)
	entries := projectPopupEntriesFor(m, popup.query)
	if len(entries) > 0 {
		popup.active = min(max(popup.active, 0), len(entries)-1)
	} else {
		popup.active = 0
	}
	width, height := c.Size()
	popupWidth := min(float32(500), max(float32(320), width-24))
	popupHeightLimit := max(float32(140), height-popup.anchor.Y-popup.anchor.H-18)
	contentHeight := float32(12)
	if popup.query != "" {
		contentHeight += 34
	}
	if popup.query == "" {
		contentHeight += float32(len(projectPopupActions) * 28)
	}
	if count := countProjectPopupEntries(entries, true); count > 0 {
		if len(entries)-count > 0 {
			contentHeight += 7
		}
		contentHeight += 22 + float32(count*51)
	}
	if count := countProjectPopupEntries(entries, false); count > 0 {
		if countProjectPopupEntries(entries, true) > 0 || popup.query == "" {
			contentHeight += 7
		}
		contentHeight += 22 + float32(count*51)
	}
	popupHeight := min(max(float32(140), contentHeight), popupHeightLimit)
	x := max(float32(8), min(popup.anchor.X, width-popupWidth-8))
	y := max(float32(8), popup.anchor.Y+popup.anchor.H+6)

	ui.Overlay(c, func() {
		box := ui.Column(c.Key("project-popup")).Role(ui.RolePopup).
			Label("Projects").Focusable().AutoFocus().Bind(&popup.focus).
			Absolute().Left(x).Top(y).Width(popupWidth).Height(popupHeight).
			Padding(6, 0).Clip().Background(oi.popup).
			Border(1, oi.borderStrong).Radius(8).
			Shadow(0, 12, 30, 0, ui.RGBA(0, 0, 0, 0.3))
		box.Children(func() {
			if popup.query != "" {
				m.projectPopupSearch(c, oi, popup)
			}
			if len(entries) == 0 {
				ui.Row(c).Height(34).Padding(0, 10).AlignItems(ui.Center).Children(func() {
					oiIconEl(c, "general/search", 14, oi, oi.textMuted)
					ui.Text(c, "No matching projects").FontSize(12).
						TextColor(oi.textFaint).SingleLine()
				})
				return
			}
			if popup.reveal {
				popup.list.ScrollTo(popup.active, ui.End)
			}
			ui.List(c.Key("project-popup-list"), &popup.list, len(entries), func(i int) {
				m.projectPopupRow(c, p, oi, popup, entries[i], i)
			}).Grow(1).MinWidth(0).MinHeight(0)
		})
		box.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputText && ev.Text != "" {
				m.typeProjectPopup(ev.Text)
				return true
			}
			if ev.Kind != ui.InputKeyDown {
				return false
			}
			switch {
			case ev.Key == ui.KeyUp:
				m.stepProjectPopup(-1)
			case ev.Key == ui.KeyDown:
				m.stepProjectPopup(1)
			case ev.Key == ui.KeyHome:
				m.jumpProjectPopup(false)
			case ev.Key == ui.KeyEnd:
				m.jumpProjectPopup(true)
			case ev.Key == ui.KeyEnter:
				m.chooseProjectPopup()
			case ev.Key == ui.KeyEscape && popup.query != "":
				popup.query = ""
				popup.active = 0
				popup.reveal = true
				m.invalidate()
			case ev.Key == ui.KeyEscape:
				m.closeProjectPopup()
			case ev.Key == ui.KeyBackspace && ev.Mods&ui.Cmd != 0 && popup.query != "":
				popup.query = ""
				popup.active = 0
				popup.reveal = true
				m.invalidate()
			case ev.Key == ui.KeyBackspace:
				m.eraseProjectPopup()
			case ev.Key == ui.KeyDelete ||
				ev.Key == ui.KeyBackspace && ev.Mods&ui.Cmd != 0:
				m.removeSelectedProjectPopupRecent()
			default:
				return false
			}
			return true
		})
		if box.PressedOutside() {
			m.closeProjectPopup()
		}
	})
	popup.reveal = false
}

func countProjectPopupEntries(entries []projectPopupEntry, open bool) int {
	count := 0
	for _, entry := range entries {
		if entry.kind == projectPopupProjectEntry && entry.open == open {
			count++
		}
	}
	return count
}

func (m *Model) projectPopupSearch(c *ui.Context, oi oiColors, popup *projectPopup) {
	ui.Row(c).Height(28).Margin(0, 6, 4, 6).Padding(0, 8).Gap(6).
		AlignItems(ui.Center).Radius(5).Background(oi.elev).
		Border(1, oi.accent).Children(func() {
		oiIconEl(c, "general/search", 14, oi, oi.textMuted)
		ui.Text(c, popup.query).FontSize(12).TextColor(oi.textStrong).
			SingleLine().Grow(1)
	})
}

func (m *Model) projectPopupRow(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	popup *projectPopup,
	entry projectPopupEntry,
	index int,
) {
	if entry.firstOpen || entry.firstRecent {
		if index > 0 {
			ui.Row(c).Height(1).Margin(5, 12).Background(oi.border)
		}
		title := "Open Projects"
		if entry.firstRecent {
			title = "Recent Projects"
		}
		ui.Text(c, title).FontSize(11).Bold().TextColor(oi.textMuted).
			Padding(4, 16, 2, 16).SingleLine()
	}
	active := index == popup.active
	row := ui.Row(c.Key(projectPopupEntryKey(entry))).
		Margin(0, 6).Padding(0, 8).Radius(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	switch {
	case active:
		row.Background(oi.selected)
	case row.Hovered():
		row.Background(oi.toolbarHover)
		if popup.active != index {
			popup.active = index
			m.invalidate()
		}
	}
	row.OnClick(func() {
		popup.active = index
		m.chooseProjectPopup()
	})
	if entry.kind == projectPopupProjectEntry && !entry.open {
		row.ContextMenu(func(menu *ui.Menu) {
			m.welcomeRecentMenu(menu, entry.project.Path, m.closeProjectPopup)
		})
	}
	switch entry.kind {
	case projectPopupActionEntry:
		color := oi.textStrong
		if active {
			color = oi.accent
		}
		row.Height(28).Gap(8).Children(func() {
			oiIconEl(c, entry.icon, 16, oi, oi.textMuted)
			ui.Text(c, entry.label).FontSize(13).TextColor(color).
				SingleLine().Ellipsis("…").Grow(1)
		})
	case projectPopupProjectEntry:
		nameColor := oi.text
		if active {
			nameColor = oi.accent
		}
		row.MinHeight(51).Padding(6, 8).Gap(10).AlignItems(ui.Start).Children(func() {
			recentProjectTile(c, entry.project.Name)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Baseline).Children(func() {
					m.projectPopupHighlightedText(c, oi, entry.project.Name,
						entry.match.nameIdx, 13, map[bool]ui.Color{
							false: nameColor, true: oi.accent,
						})
					if entry.project.Branch != "" {
						ui.Spacer(c)
						oiIconEl(c, "general/vcs", 11, oi, oi.textSubtle)
						ui.Text(c, entry.project.Branch).FontSize(11).
							TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
					}
				})
				m.projectPopupHighlightedText(c, oi, shortenHome(entry.project.Path),
					entry.match.pathIdx, 11, map[bool]ui.Color{
						false: oi.textSubtle, true: oi.accent,
					})
			})
		})
	}
}

func projectPopupEntryKey(entry projectPopupEntry) string {
	if entry.kind == projectPopupActionEntry {
		return "project-popup-action:" + entry.actionID
	}
	if entry.open {
		return "project-popup-open:" + entry.project.Path
	}
	return "project-popup-recent:" + entry.project.Path
}

func (m *Model) projectPopupHighlightedText(
	c *ui.Context,
	oi oiColors,
	text string,
	indices []int,
	size float32,
	tones map[bool]ui.Color,
) {
	if len(indices) == 0 {
		ui.Text(c, text).FontSize(size).TextColor(tones[false]).
			SingleLine().Ellipsis("…")
		return
	}
	matched := make(map[int]bool, len(indices))
	for _, index := range indices {
		matched[index] = true
	}
	ui.Row(c).Gap(0).Children(func() {
		start := 0
		hit := matched[0]
		for i := 1; i <= len(text); i++ {
			if i < len(text) && matched[i] == hit {
				continue
			}
			ui.Text(c, text[start:i]).FontSize(size).
				TextColor(tones[hit]).SingleLine()
			start = i
			hit = matched[i]
		}
	})
}
