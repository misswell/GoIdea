package app

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type keymapReferenceState struct {
	open  bool
	query string
}

type keymapReferenceEntry struct {
	id        string
	title     string
	shortcuts []string
}

type keymapReferenceGroup struct {
	category string
	entries  []keymapReferenceEntry
}

var keymapReferencePrefixCategories = map[string]string{
	"navigate":   "Navigate",
	"find":       "Find",
	"code":       "Code",
	"refactor":   "Refactor",
	"file":       "File",
	"editor":     "Editor",
	"project":    "Project View",
	"toolwindow": "Tool Windows",
	"run":        "Run",
	"debug":      "Debug",
	"vcs":        "Git",
}

// OpenAbout opens the Help › About sheet.
func (m *Model) OpenAbout() {
	m.aboutOpen = true
	m.invalidate()
}

func (m *Model) closeAbout() {
	m.aboutOpen = false
	m.invalidate()
}

func (m *Model) aboutDialog(c *ui.Context, p *ui.Theme) {
	if !m.aboutOpen {
		return
	}
	open := m.aboutOpen
	dialog := ui.Modal(c, &open, func() { m.aboutSheet(c, p) })
	if dialog.Dismissed() || !open {
		m.closeAbout()
	}
}

func (m *Model) aboutSheet(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	version := mygo.App.Version()
	if version == "" {
		version = "Development"
	}
	ui.Column(c).Width(520).Padding(22, 24, 18, 24).Gap(14).
		Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
		ui.Text(c, "About GoIDEA").FontSize(14).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
			if logo := welcomeLogoImage(); logo != nil {
				ui.Image(c, logo).Size(64, 64)
			}
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				ui.Text(c, "GoIDEA "+version).FontSize(16).Bold().TextColor(oi.textStrong).SingleLine()
				ui.Text(c, "A fast, native IDE for Java and the languages around it.").
					FontSize(12).TextColor(oi.text)
				m.aboutFact(c, oi, "Platform", runtime.GOOS+" "+runtime.GOARCH)
				m.aboutFact(c, oi, "Runtime", runtime.Version())
				if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
					m.aboutFact(c, oi, "Configuration", dir)
				}
			})
		})
		ui.Box(c).Height(1).Shrink(0).Background(oi.border)
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Link(c, "Keyboard Reference", "").OnClick(m.OpenKeymapReference)
			ui.Link(c, "Show Log", "").OnClick(m.ShowAppLog)
			ui.Link(c, "Report Issue", "https://github.com/misswell/GoIdea/issues/new/choose")
			ui.Spacer(c)
			if ui.Button(c, "Copy Info").Clicked() {
				mygo.Clipboard.WriteText(m.aboutText(version))
				m.status = "About information copied"
			}
			if ui.PrimaryButton(c, "Close").AutoFocus().Clicked() {
				m.closeAbout()
			}
		})
		ui.Text(c, "Copyright © GoIDEA contributors. Licensed under MIT.").
			FontSize(11).TextColor(oi.textMuted)
		ui.Text(c, "GoIDEA and its bundled components keep their respective licenses.").
			FontSize(11).TextColor(oi.textMuted)
	})
}

func (m *Model) aboutFact(c *ui.Context, oi oiColors, label, value string) {
	ui.Row(c).Gap(6).Children(func() {
		ui.Text(c, label+":").Width(88).Shrink(0).FontSize(11).
			TextColor(oi.textMuted).SingleLine()
		ui.Text(c, value).FontSize(11).TextColor(oi.text).SingleLine().Ellipsis("…")
	})
}

func (m *Model) aboutText(version string) string {
	var rows []string
	rows = append(rows, "GoIDEA "+version)
	rows = append(rows, "Platform: "+runtime.GOOS+" "+runtime.GOARCH)
	rows = append(rows, "Runtime: "+runtime.Version())
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
		rows = append(rows, "Configuration: "+dir)
	}
	return strings.Join(rows, "\n")
}

// OpenKeymapReference opens Help › Keyboard Reference.
func (m *Model) OpenKeymapReference() {
	m.keymapReference.open = true
	m.keymapReference.query = ""
	m.invalidate()
}

func (m *Model) closeKeymapReference() {
	m.keymapReference.open = false
	m.keymapReference.query = ""
	m.invalidate()
}

func (m *Model) keymapReferenceDialog(c *ui.Context, p *ui.Theme) {
	if !m.keymapReference.open {
		return
	}
	open := m.keymapReference.open
	dialog := ui.Modal(c, &open, func() { m.keymapReferenceSheet(c, p) })
	if dialog.Dismissed() || !open {
		m.closeKeymapReference()
	}
}

func (m *Model) keymapReferenceSheet(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	groups := m.keymapReferenceGroups(m.keymapReference.query)
	count := 0
	for _, group := range groups {
		count += len(group.entries)
	}
	ui.Column(c).Width(720).Height(560).Padding(16, 18, 14, 18).Gap(10).
		Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
		ui.Text(c, "Keyboard Reference").FontSize(14).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &m.keymapReference.query).Grow(1).
				Placeholder("Filter shortcuts").AutoFocus()
			in.OnChange(func() { m.invalidate() })
			ui.Text(c, fmt.Sprintf("%s · %d shortcuts", keymapPreset(), count)).
				FontSize(11).TextColor(oi.textMuted).SingleLine()
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(14).Padding(0, 2, 8, 2).Children(func() {
				if len(groups) == 0 {
					ui.Text(c, "No shortcuts match the filter.").
						FontSize(12).TextColor(oi.textMuted)
					return
				}
				for _, group := range groups {
					group := group
					ui.Column(c).Gap(3).Children(func() {
						ui.Text(c, group.category).FontSize(11).Bold().
							TextColor(oi.textMuted).SingleLine()
						for _, entry := range group.entries {
							entry := entry
							ui.Row(c).Height(24).Padding(0, 6).Gap(10).
								AlignItems(ui.Center).Children(func() {
								ui.Text(c, entry.title).FontSize(12).
									TextColor(oi.text).SingleLine().Ellipsis("…")
								ui.Spacer(c)
								for _, shortcut := range entry.shortcuts {
									ui.Text(c, keymapDisplay(shortcut)).FontSize(10).
										TextColor(oi.text).Padding(2, 6).
										Background(oi.island).Border(1, oi.border).
										Radius(3).SingleLine()
								}
							})
						}
					})
				}
			})
		})
		ui.Row(c).Justify(ui.End).Children(func() {
			if ui.PrimaryButton(c, "Close").Clicked() {
				m.closeKeymapReference()
			}
		})
	})
}

func (m *Model) keymapReferenceGroups(query string) []keymapReferenceGroup {
	query = strings.ToLower(strings.TrimSpace(query))
	byCategory := map[string][]keymapReferenceEntry{}
	for _, action := range actionList() {
		if hiddenActionIDs[action.ID] {
			continue
		}
		shortcuts := m.KeymapBindings(action.ID)
		if len(shortcuts) == 0 {
			continue
		}
		title := action.Title
		if title == "" {
			title = keymapReferenceTitle(action.ID)
		}
		if query != "" {
			haystack := strings.ToLower(title + " " + action.ID + " " + strings.Join(shortcuts, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		category := action.Category
		if category == "" {
			prefix := action.ID
			if dot := strings.IndexByte(prefix, '.'); dot >= 0 {
				prefix = prefix[:dot]
			}
			category = keymapReferencePrefixCategories[prefix]
		}
		if category == "" {
			category = "Other"
		}
		byCategory[category] = append(byCategory[category], keymapReferenceEntry{
			id: action.ID, title: title, shortcuts: shortcuts,
		})
	}
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.Slice(categories, func(i, j int) bool {
		if categories[i] == "Other" {
			return false
		}
		if categories[j] == "Other" {
			return true
		}
		return categories[i] < categories[j]
	})
	groups := make([]keymapReferenceGroup, 0, len(categories))
	for _, category := range categories {
		entries := byCategory[category]
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].title == entries[j].title {
				return entries[i].id < entries[j].id
			}
			return entries[i].title < entries[j].title
		})
		groups = append(groups, keymapReferenceGroup{category: category, entries: entries})
	}
	return groups
}

func keymapReferenceTitle(id string) string {
	last := id
	if dot := strings.LastIndexByte(last, '.'); dot >= 0 {
		last = last[dot+1:]
	}
	var out []rune
	for i, r := range []rune(last) {
		if i == 0 {
			out = append(out, unicode.ToUpper(r))
			continue
		}
		if unicode.IsUpper(r) {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	return string(out)
}

// ShowAppLog reveals the current backend log in Finder or Explorer.
func (m *Model) ShowAppLog() {
	path := m.appLogPath()
	if path == "" {
		m.notifyError("app-log", "Cannot show the log", "The application log is not available.")
		return
	}
	mygo.Shell.ShowItemInFolder(path)
}

func (m *Model) appLogPath() string {
	if m.cfg.Log == nil {
		return ""
	}
	return m.cfg.Log.Name()
}
