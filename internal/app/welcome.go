package app

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/git"
)

// welcomeLogoSrc is OpenIDEA's own 128px app icon, shown in the Welcome
// screen's brand block as WelcomeView.tsx shows it.
//
//go:embed assets/openidea.png
var welcomeLogoSrc []byte

var (
	welcomeLogoOnce sync.Once
	welcomeLogo     *ui.Bitmap
)

func welcomeLogoImage() *ui.Bitmap {
	welcomeLogoOnce.Do(func() {
		if bmp, err := ui.DecodeBitmap(welcomeLogoSrc); err == nil {
			welcomeLogo = bmp
		}
	})
	return welcomeLogo
}

// welcome is OpenIDEA's Welcome screen: the brand rail of pages beside the
// page itself, projects, customization or plugins, as WelcomeView.tsx lays
// it out.
func (m *Model) welcome(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		m.welcomeSidebar(c, oi)
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(oi.island).Children(func() {
			switch m.welcomePage {
			case "customize":
				m.welcomeCustomize(c, oi)
			case "plugins":
				m.welcomePlugins(c, oi)
			default:
				m.welcomeProjects(c, oi)
			}
		})
	})
	m.welcomeCloneDialog(c, oi)
	m.welcomeRemoveDialog(c, oi)
}

// welcomeSidebar is the 240px rail: the brand, the tagline, the pages, and
// the gear at its foot.
func (m *Model) welcomeSidebar(c *ui.Context, oi oiColors) {
	ui.Column(c).Width(240).Shrink(0).Padding(28, 12, 14, 12).
		Background(oi.elev).BorderWidth(0, 1, 0, 0).BorderColor(oi.border).
		Children(func() {
			ui.Row(c).Gap(12).Padding(0, 6, 8, 6).AlignItems(ui.Center).Children(func() {
				if logo := welcomeLogoImage(); logo != nil {
					ui.Image(c, logo).Size(32, 32)
				}
				ui.Column(c).Justify(ui.Center).Children(func() {
					ui.Text(c, "OpenIDEA").FontSize(13).TextColor(oi.textStrong).SingleLine()
					if version := mygo.App.Version(); version != "" {
						ui.Text(c, version).FontSize(13).TextColor(oi.textMuted).SingleLine()
					}
				})
			})
			ui.Text(c, "An open-source IDE inspired by IntelliJ IDEA").
				FontSize(11).TextColor(oi.textMuted).Padding(0, 6).Margin(0, 0, 20, 0)
			ui.Column(c).Gap(2).Children(func() {
				m.welcomeNavItem(c, oi, "projects", "Projects")
				m.welcomeNavItem(c, oi, "customize", "Customize")
				m.welcomeNavItem(c, oi, "plugins", "Plugins")
			})
			ui.Spacer(c)
			ui.Row(c).Padding(0, 4).Children(func() {
				m.welcomeGear(c, oi)
			})
		})
}

// welcomeNavItem is one page of the rail, selected with OpenIDEA's rounded
// 34px row.
func (m *Model) welcomeNavItem(c *ui.Context, oi oiColors, page, label string) {
	active := m.welcomePage == page || (page == "projects" && m.welcomePage == "")
	r := ui.Row(c.Key("welcomeNav:"+page)).Height(34).Shrink(0).Padding(0, 24).
		Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if active {
		r.Background(oi.selected)
	} else if r.Hovered() {
		r.Background(oi.hoverTint)
	}
	r.Children(func() {
		tone := oi.text
		if active {
			tone = oi.textStrong
		}
		ui.Text(c, label).FontSize(13).TextColor(tone).SingleLine()
	})
	if r.Clicked() {
		m.welcomePage = page
		if page == "projects" {
			m.welcomePage = ""
		}
		m.invalidate()
	}
}

// welcomeGear is the Manage button of the rail's foot, opening OpenIDEA's
// gear menu.
func (m *Model) welcomeGear(c *ui.Context, oi oiColors) {
	g := ui.ButtonBase(c.Key("welcomeGear")).Size(24, 24).Radius(6).
		AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
		Tooltip("Manage")
	if g.Hovered() {
		g.Background(oi.hoverTint)
	}
	g.Children(func() {
		ui.Icon(c, iconWelcomeGear).Size(16, 16).TextColor(oi.textSecondary)
	})
	if g.Clicked() {
		m.welcomeGearOpen = !m.welcomeGearOpen
		m.invalidate()
	}
	open := m.welcomeGearOpen
	ui.PopoverBase(c, g, &open, func(panel ui.Element) {
		panel.Width(240).Padding(4).Background(oi.popup).
			BorderWidth(1).BorderColor(oi.borderStrong).Radius(8)
		panel.Children(func() {
			m.welcomeGearItem(c, oi, "Settings…", func() {
				m.welcomeGearOpen = false
				m.OpenSettings()
			})
			m.welcomeGearItem(c, oi, "Command Palette…", func() {
				m.welcomeGearOpen = false
				m.OpenCommands()
			})
			menuSeparator(c, oi)
			m.welcomeGearItem(c, oi, "Show Git Log", func() {
				m.welcomeGearOpen = false
				m.ShowLog()
			})
		})
	})
	if !open {
		m.welcomeGearOpen = false
	}
}

func (m *Model) welcomeGearItem(c *ui.Context, oi oiColors, label string, run func()) {
	r := ui.Row(c.Key("welcomeGear:"+label)).Height(28).Shrink(0).Padding(0, 10).
		Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if r.Hovered() {
		r.Background(oi.hoverTint)
	}
	r.Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(oi.text).SingleLine()
	})
	if r.Clicked() {
		run()
	}
}

func menuSeparator(c *ui.Context, oi oiColors) {
	ui.Row(c).Height(1).Shrink(0).Margin(4, 0, 4, 0).Background(oi.border)
}

// welcomeProjects is the Projects page: the header of search and actions
// over the recent projects, or the landing when there are none, as
// ProjectsPage.tsx has it.
func (m *Model) welcomeProjects(c *ui.Context, oi oiColors) {
	if len(m.recents) == 0 {
		m.welcomeLanding(c, oi)
		return
	}
	m.welcomeHeader(c, oi)
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(14, 16, 14, 24).Children(func() {
			list := m.filteredRecents()
			if len(list) == 0 {
				ui.Text(c, "No matching projects").FontSize(13).TextColor(oi.textFaint).
					Padding(8, 10)
				return
			}
			for _, dir := range list {
				m.welcomeRecentRow(c, oi, dir)
			}
		})
	})
}

// welcomeHeader is the search field and the buttons above the list.
func (m *Model) welcomeHeader(c *ui.Context, oi oiColors) {
	ui.Row(c).Height(76).Shrink(0).MarginStart(14).Padding(0, 14, 0, 6).Gap(12).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Height(28).Gap(2).AlignItems(ui.Center).
				Children(func() {
					ui.Icon(c, iconWelcomeSearch).Size(14, 14).TextColor(oi.textMuted).
						MarginEnd(4)
					input := ui.TextInputBase(c, &m.welcomeQuery).Grow(1).AutoFocus()
					input.Placeholder("Search projects")
					if input.Changed() {
						m.invalidate()
					}
					if input.Submitted() {
						if list := m.filteredRecents(); len(list) > 0 {
							m.confirmSwitch(list[0])
						}
					}
				})
			ui.Row(c).Gap(8).Children(func() {
				m.welcomeButton(c, oi, "New Project", true, m.NewProject)
				m.welcomeButton(c, oi, "Open", true, m.chooseProject)
				m.welcomeButton(c, oi, "Clone Repository", true, m.openCloneDialog)
				m.welcomeButton(c, oi, "Remote Development", false, nil)
			})
		})
}

// welcomeButton is one secondary button of the header.
func (m *Model) welcomeButton(c *ui.Context, oi oiColors, label string, enabled bool, run func()) {
	b := ui.ButtonBase(c.Key("welcomeButton:"+label)).Height(28).MinWidth(72).
		Padding(0, 16).Radius(4).AlignItems(ui.Center).Justify(ui.Center).
		Cursor(ui.CursorPointer).BorderWidth(1)
	if enabled {
		b.BorderColor(oi.buttonBorder)
		if b.Hovered() {
			b.Background(oi.buttonHover)
		}
	} else {
		b.BorderColor(oi.border).Tooltip("Not available in GoIDEA yet")
	}
	b.Children(func() {
		tone := oi.text
		if !enabled {
			tone = oi.textFaint
		}
		ui.Text(c, label).FontSize(13).TextColor(tone).SingleLine()
	})
	if enabled && b.Clicked() && run != nil {
		run()
	}
}

// welcomeLanding is what the Projects page shows before a first project:
// the welcome, the four actions, and the open hint.
func (m *Model) welcomeLanding(c *ui.Context, oi oiColors) {
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(12).
		Padding(0, 0, 40, 0).Children(func() {
		ui.Text(c, "Welcome to OpenIDEA").FontSize(18).Bold().TextColor(oi.textStrong)
		ui.Text(c, "Open a folder from disk or clone a repository from version control.").
			FontSize(13).TextColor(oi.textMuted)
		ui.Row(c).Gap(24).Margin(16, 0, 0, 0).Children(func() {
			m.welcomeLandingAction(c, oi, "New Project", iconWelcomeFolder, true, m.NewProject)
			m.welcomeLandingAction(c, oi, "Open", iconWelcomeOpen, true, m.chooseProject)
			m.welcomeLandingAction(c, oi, "Clone Repository", iconWelcomeClone, true, m.openCloneDialog)
			m.welcomeLandingAction(c, oi, "Remote Development", iconWelcomeRemote, false, nil)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
			ui.Text(c, "Open a folder").FontSize(13).TextColor(oi.textFaint)
			ui.Text(c, openAccelerator()).FontSize(12).TextColor(oi.textFaint)
		})
	})
}

// welcomeLandingAction is one of the landing's tiles: the icon in its
// 48px square over the action's name.
func (m *Model) welcomeLandingAction(c *ui.Context, oi oiColors, label string, icon *ui.SVG, enabled bool, run func()) {
	b := ui.ButtonBase(c.Key("welcomeLanding:" + label)).Width(150).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	b.Children(func() {
		ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
			tile := ui.Row(c).Size(48, 48).Radius(10).Background(oi.elev).
				BorderWidth(1).BorderColor(oi.buttonBorder).
				Justify(ui.Center).AlignItems(ui.Center)
			if enabled && tile.Hovered() {
				tile.Background(oi.hover)
			}
			tile.Children(func() {
				tone := oi.textMuted
				if !enabled {
					tone = oi.textFaint
				}
				ui.Icon(c, icon).Size(20, 20).TextColor(tone)
			})
			tone := oi.textStrong
			if !enabled {
				tone = oi.textFaint
			}
			ui.Text(c, label).FontSize(13).TextColor(tone).SingleLine()
		})
	})
	if !enabled {
		b.Tooltip("Not available in GoIDEA yet")
	}
	if enabled && b.Clicked() && run != nil {
		run()
	}
}

// welcomeRecentRow is one recent project: its initials over the name, the
// folder and the branch, and the menu that shows on hover.
func (m *Model) welcomeRecentRow(c *ui.Context, oi oiColors, dir string) {
	name := filepath.Base(dir)
	r := ui.Row(c.Key("welcomeRecent:"+dir)).Padding(8, 10).Gap(12).
		AlignItems(ui.Start).Radius(8)
	hovered := r.Hovered()
	if hovered {
		r.Background(oi.hoverTint)
	}
	moreClicked := false
	r.Children(func() {
		start, end := projectColors(name)
		ui.Row(c).Size(20, 20).Shrink(0).Radius(4).Margin(1, 0, 0, 0).
			LinearGradient(ui.LinearGradient{From: start, To: end, Angle: 135}).
			Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Text(c, projectInitials(name)).FontSize(11).Bold().
				TextColor(ui.Hex("#ffffff")).LetterSpacing(0.02).SingleLine()
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(13).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			ui.Text(c, shortenHome(dir)).FontSize(13).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…")
			if branch := m.branchOf(dir); branch != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconWelcomeBranch).Size(14, 14).TextColor(oi.textMuted)
					ui.Text(c, branch).FontSize(13).TextColor(oi.textMuted).SingleLine()
				})
			}
		})
		more := ui.ButtonBase(c.Key("welcomeRecentMore:"+dir)).Size(24, 24).Radius(6).
			AlignSelf(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
		if hovered {
			if more.Hovered() {
				more.Background(oi.hoverTint)
			}
			more.Children(func() {
				ui.Icon(c, iconWelcomeMore).Size(16, 16).TextColor(oi.textMuted)
			})
			more.Menu(func(menu *ui.Menu) { m.welcomeRecentMenu(menu, dir) })
			if more.Clicked() {
				moreClicked = true
			}
		}
	})
	r.ContextMenu(func(menu *ui.Menu) { m.welcomeRecentMenu(menu, dir) })
	if r.Clicked() && !moreClicked {
		m.confirmSwitch(dir)
	}
}

// welcomeRecentMenu is the row's menu, from the hover button and the
// context menu alike.
func (m *Model) welcomeRecentMenu(menu *ui.Menu, dir string) {
	if menu.Item("Open").Chosen() {
		m.confirmSwitch(dir)
	}
	if menu.Item(revealLabel()).Chosen() {
		revealPath(dir)
	}
	if menu.Item("Copy Path").Chosen() {
		mygo.Clipboard.WriteText(dir)
	}
	menu.Separator()
	if menu.Item("Remove from Recent Projects…").Chosen() {
		m.askRemoveRecent(dir)
	}
}

func revealLabel() string {
	switch runtime.GOOS {
	case "darwin":
		return "Reveal in Finder"
	case "windows":
		return "Show in File Explorer"
	}
	return "Open Containing Folder"
}

func revealPath(dir string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(dir))
	}
	_ = cmd.Start()
}

func openAccelerator() string {
	if runtime.GOOS == "darwin" {
		return "⌘O"
	}
	return "Ctrl+O"
}

// branchOf is the branch of a recent project, read once in the background;
// "" until known, and for folders that are not work trees.
func (m *Model) branchOf(dir string) string {
	if b, ok := m.welcomeBranches[dir]; ok {
		return b
	}
	if m.welcomeBranchLoading == nil {
		m.welcomeBranchLoading = map[string]bool{}
	}
	if m.welcomeBranchLoading[dir] {
		return ""
	}
	m.welcomeBranchLoading[dir] = true
	go func() {
		name := ""
		if repo, err := git.Open(dir); err == nil {
			name = repo.BranchName()
		}
		m.post(func() {
			if m.welcomeBranches == nil {
				m.welcomeBranches = map[string]string{}
			}
			m.welcomeBranches[dir] = name
			delete(m.welcomeBranchLoading, dir)
			m.invalidate()
		})
	}()
	return ""
}

// welcomeCustomize is the Customize page: the settings GoIDEA has, laid
// out as CustomizePage.tsx lays its sections and fields.
func (m *Model) welcomeCustomize(c *ui.Context, oi oiColors) {
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(28, 32).Gap(24).AlignItems(ui.Start).Children(func() {
			m.welcomeSection(c, oi, "Color theme", func() {
				m.welcomeField(c, oi, "Theme", func() {
					choice := themeChoiceLabel(m.prefs.Theme)
					sync := m.prefs.Theme == ""
					sel := ui.Select(c, &choice, []string{"Dark", "Light", "Sync with OS"}).Width(160)
					sel.Disabled(sync)
					if sel.Changed() {
						switch choice {
						case "Dark":
							m.SetThemePref("dark")
						case "Light":
							m.SetThemePref("light")
						default:
							m.SetThemePref("system")
						}
					}
					cb := ui.Checkbox(c, &sync, "Sync with OS")
					if cb.Changed() {
						switch {
						case sync:
							m.SetThemePref("system")
						case oi.dark:
							m.SetThemePref("dark")
						default:
							m.SetThemePref("light")
						}
					}
				})
			})
			m.welcomeSection(c, oi, "Accessibility", func() {
				m.welcomeField(c, oi, "Editor font", func() {
					size := fmt.Sprintf("%.0f", m.prefs.fontSize())
					sel := ui.Select(c, &size, editorFontSizes()).Width(120)
					if sel.Changed() {
						var n int
						if _, err := fmt.Sscanf(size, "%d", &n); err == nil {
							m.setFontSize(n)
						}
					}
				})
			})
			link := ui.Text(c, "All settings…").FontSize(13).TextColor(oi.link).
				Cursor(ui.CursorPointer)
			if link.Clicked() {
				m.OpenSettings()
			}
		})
	})
}

func (m *Model) welcomeSection(c *ui.Context, oi oiColors, title string, body func()) {
	ui.Column(c).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Text(c, title).FontSize(13).Bold().TextColor(oi.textStrong).
			Margin(0, 0, 2, 0).SingleLine()
		body()
	})
}

func (m *Model) welcomeField(c *ui.Context, oi oiColors, label string, body func()) {
	ui.Row(c).MinHeight(28).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(oi.text).Width(96).SingleLine()
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(body)
	})
}

func themeChoiceLabel(theme string) string {
	switch theme {
	case "dark":
		return "Dark"
	case "light":
		return "Light"
	}
	return "Sync with OS"
}

func editorFontSizes() []string {
	out := make([]string, 0, maxFontSize-minFontSize+1)
	for size := minFontSize; size <= maxFontSize; size++ {
		out = append(out, fmt.Sprintf("%d", size))
	}
	return out
}

// welcomePlugins is Welcome › Plugins, over the real plugin registry.
func (m *Model) welcomePlugins(c *ui.Context, oi oiColors) {
	m.pluginPageUI(c, oi)
}

// openCloneDialog shows the Clone Repository dialog, with the folder of
// the newest project, or ~/IdeaProjects, as the place to clone into.
func (m *Model) openCloneDialog() {
	if m.welcomeCloneOpen {
		return
	}
	m.welcomeCloneOpen = true
	m.welcomeCloneErr = ""
	if m.welcomeCloneDir == "" {
		m.welcomeCloneDir = defaultCloneParent()
	}
	m.invalidate()
}

func (m *Model) closeCloneDialog() {
	if !m.welcomeCloneOpen {
		return
	}
	m.welcomeCloneOpen = false
	m.welcomeCloneErr = ""
	// A clone still running is discarded rather than opening over the
	// dialog the user closed.
	m.welcomeCloneSeq++
	m.welcomeCloneBusy = false
	m.invalidate()
}

func (m *Model) welcomeCloneDialog(c *ui.Context, oi oiColors) {
	if !m.welcomeCloneOpen {
		return
	}
	open := m.welcomeCloneOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(460).Gap(10).Children(func() {
			ui.Text(c, "Clone Repository").FontSize(14).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Text(c, "Repository URL").FontSize(11).TextColor(oi.textMuted).
				SingleLine()
			urlInput := ui.TextInput(c, &m.welcomeCloneURL).FillWidth()
			urlInput.Placeholder("https://github.com/user/repo.git")
			ui.Text(c, "Directory").FontSize(11).TextColor(oi.textMuted).SingleLine()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &m.welcomeCloneDir).Grow(1)
				ui.Button(c, "Choose…").OnClick(m.chooseCloneDir)
			})
			if dest := m.cloneDestination(); dest != "" {
				ui.Text(c, "Clones into "+shortenHome(dest)).FontSize(11).
					TextColor(oi.textFaint).SingleLine().Ellipsis("…")
			}
			if m.welcomeCloneErr != "" {
				ui.Text(c, m.welcomeCloneErr).FontSize(12).TextColor(oi.danger).
					SingleLine().Ellipsis("…")
			}
			ui.Row(c).Justify(ui.End).Gap(8).AlignItems(ui.Center).Children(func() {
				if m.welcomeCloneBusy {
					ui.Text(c, "Cloning…").FontSize(12).TextColor(oi.textMuted).
						SingleLine()
				}
				cancel := ui.Button(c, "Cancel")
				cancel.Disabled(m.welcomeCloneBusy)
				if cancel.Clicked() {
					m.closeCloneDialog()
				}
				clone := ui.PrimaryButton(c, "Clone")
				clone.Disabled(m.welcomeCloneBusy)
				if clone.Clicked() {
					m.startClone()
				}
			})
		})
	})
	if !open {
		m.closeCloneDialog()
	}
}

func (m *Model) chooseCloneDir() {
	parent := m.win
	start := m.welcomeCloneDir
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Clone Repository", ButtonLabel: "Choose",
			DefaultPath: start, Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		dir := paths[0]
		m.post(func() {
			m.welcomeCloneDir = dir
			m.invalidate()
		})
	}()
}

func defaultCloneParent() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "IdeaProjects")
}

// cloneDestination is the folder the dialog's URL clones into: the chosen
// directory and the repository's name, as clone.tsx makes it.
func (m *Model) cloneDestination() string {
	dir := strings.TrimSpace(m.welcomeCloneDir)
	name := cloneDirName(m.welcomeCloneURL)
	if dir == "" || name == "" {
		return ""
	}
	return filepath.Join(dir, name)
}

var cloneNameRe = regexp.MustCompile(`[^\w.-]+`)

// cloneDirName is the repository's name from its URL: the last path
// segment, without .git, with the characters a folder cannot keep replaced,
// as clone.tsx has it.
func cloneDirName(url string) string {
	s := strings.TrimSpace(url)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRight(s, `/\`)
	if len(s) > 4 && strings.EqualFold(s[len(s)-4:], ".git") {
		s = s[:len(s)-4]
	}
	var last string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == '\\' || r == ':'
	}) {
		last = part
	}
	last = cloneNameRe.ReplaceAllString(last, "-")
	return strings.TrimLeft(last, ".-")
}

// startClone checks the dialog's fields and clones in the background;
// the project opens once the clone is done.
func (m *Model) startClone() {
	url := strings.TrimSpace(m.welcomeCloneURL)
	switch {
	case url == "":
		m.welcomeCloneErr = "Enter a repository URL"
	case strings.ContainsAny(url, " \t\r\n"):
		m.welcomeCloneErr = "The URL must not contain spaces"
	case !strings.ContainsAny(url, "/:"):
		m.welcomeCloneErr = "Invalid repository URL"
	case strings.TrimSpace(m.welcomeCloneDir) == "":
		m.welcomeCloneErr = "Choose a directory"
	case !filepath.IsAbs(strings.TrimSpace(m.welcomeCloneDir)):
		m.welcomeCloneErr = "The directory must be an absolute path"
	}
	if m.welcomeCloneErr != "" {
		m.invalidate()
		return
	}
	dest := m.cloneDestination()
	if dest == "" {
		m.welcomeCloneErr = "Invalid repository URL"
		m.invalidate()
		return
	}
	if _, err := os.Stat(dest); err == nil {
		m.welcomeCloneErr = dest + " already exists"
		m.invalidate()
		return
	}
	m.welcomeCloneSeq++
	seq := m.welcomeCloneSeq
	m.welcomeCloneBusy = true
	m.welcomeCloneErr = ""
	m.invalidate()
	go func() {
		err := git.Clone(url, dest)
		m.post(func() {
			if seq != m.welcomeCloneSeq {
				return
			}
			m.welcomeCloneBusy = false
			if err != nil {
				m.welcomeCloneErr = err.Error()
				m.invalidate()
				return
			}
			m.welcomeCloneOpen = false
			m.welcomeCloneURL = ""
			m.invalidate()
			m.confirmSwitch(dest)
		})
	}()
}

func (m *Model) askRemoveRecent(dir string) {
	m.welcomeRemovePath = dir
	m.welcomeRemoveOpen = true
	m.invalidate()
}

func (m *Model) welcomeRemoveDialog(c *ui.Context, oi oiColors) {
	if !m.welcomeRemoveOpen {
		return
	}
	open := m.welcomeRemoveOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, "Remove Recent Project").FontSize(14).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Textf(c, "Remove '%s' from the list of recent projects?",
				filepath.Base(m.welcomeRemovePath)).FontSize(13).TextColor(oi.text)
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				cancel := ui.Button(c, "Cancel")
				if cancel.Clicked() {
					m.welcomeRemoveOpen = false
					m.welcomeRemovePath = ""
					m.invalidate()
				}
				remove := ui.PrimaryButton(c, "Remove")
				if remove.Clicked() {
					dir := m.welcomeRemovePath
					m.welcomeRemoveOpen = false
					m.welcomeRemovePath = ""
					m.forgetProject(dir)
				}
			})
		})
	})
	if !open {
		m.welcomeRemoveOpen = false
		m.welcomeRemovePath = ""
	}
}

// forgetProject drops a folder from the recent projects and from the
// session, so it stays gone at the next start.
func (m *Model) forgetProject(dir string) {
	if dir == "" {
		return
	}
	var list []string
	for _, p := range m.recents {
		if p != dir {
			list = append(list, p)
		}
	}
	m.recents = list
	delete(m.welcomeBranches, dir)
	delete(m.welcomeBranchLoading, dir)
	m.saveRecents()
	m.invalidate()
}

func (m *Model) saveRecents() {
	path := m.cfg.SessionPath
	if path == "" {
		return
	}
	s := LoadSession(path)
	s.RecentProjects = m.recents
	go func() { _ = s.Save(path) }()
}

// filteredRecents is the Welcome screen's list, filtered by its search field.
func (m *Model) filteredRecents() []string {
	q := strings.ToLower(strings.TrimSpace(m.welcomeQuery))
	if q == "" {
		return m.recents
	}
	var out []string
	for _, dir := range m.recents {
		if strings.Contains(strings.ToLower(dir), q) {
			out = append(out, dir)
		}
	}
	return out
}

// rememberProject puts dir first in the Welcome screen's recents.
func (m *Model) rememberProject(dir string) {
	if dir == "" {
		return
	}
	dir = filepath.Clean(dir)
	list := []string{dir}
	for _, p := range m.recents {
		if p != dir {
			list = append(list, p)
		}
	}
	m.recents = normalizeRecents(list)
}

// projectInitials is the two letters of a project's avatar, as OpenIDEA's
// projectColor.ts makes them: two camel-case capitals, else the first
// letters of the first and last words, else one letter.
func projectInitials(name string) string {
	text := strings.TrimSpace(name)
	camel := camelInitials(text)
	if len([]rune(camel)) == 2 {
		return camel
	}
	for _, seps := range []string{" ", ",", "-", "_", ".", "`'\""} {
		words := strings.FieldsFunc(text, func(r rune) bool {
			return strings.ContainsRune(seps, r)
		})
		if len(words) >= 2 {
			first := []rune(words[0])[0]
			last := []rune(words[len(words)-1])[0]
			return strings.ToUpper(string([]rune{first, last}))
		}
	}
	if camel == "" {
		return "?"
	}
	return camel
}

// camelInitials is the first letter of the leading word and its next
// capital, uppercased: "MyProject" → "MP", "idea" → "I".
func camelInitials(text string) string {
	start := -1
	for i, r := range text {
		if unicode.IsLetter(r) {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(text)
	for i, r := range text[start:] {
		if i == 0 {
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			end = start + i
			break
		}
	}
	word := []rune(text[start:end])
	if len(word) == 0 {
		return ""
	}
	out := []rune{word[0]}
	for _, r := range word[1:] {
		if r != unicode.ToLower(r) {
			out = append(out, r)
			break
		}
	}
	return strings.ToUpper(string(out))
}

// shortenHome writes the home folder as "~" in a path shown to the user.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

// NewProject asks for the folder of a new project and opens it, as OpenIDEA's
// New Project picks the folder of the project it creates.
func (m *Model) NewProject() { m.chooseProjectDialog("New Project", "Create") }

// chooseProject asks for a folder and opens it.
func (m *Model) chooseProject() { m.chooseProjectDialog("Open Project", "Open") }

func (m *Model) chooseProjectDialog(title, label string) {
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: title, ButtonLabel: label,
			Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		dir := paths[0]
		m.post(func() { m.confirmSwitch(dir) })
	}()
}
