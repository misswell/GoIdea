package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// welcome is OpenIDEA's Welcome screen: the rail of pages, the brand, and the
// Projects page with its actions and recent folders.
func (m *Model) welcome(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		m.welcomeRail(c, p)
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Background).
			Children(func() {
				switch m.welcomePage {
				case "plugins":
					m.welcomePlugins(c, p)
				default:
					m.welcomeProjects(c, p)
				}
			})
	})
}

func (m *Model) welcomeRail(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Width(64).Shrink(0).Padding(6, 8).Gap(2).
		Background(p.Surface).BorderWidth(0, 1, 0, 0).BorderColor(p.Border).
		Children(func() {
			m.welcomeRailItem(c, p, "projects", "▦", "项目", func() {
				m.welcomePage = ""
				m.invalidate()
			})
			m.welcomeRailItem(c, p, "customize", "◐", "自定义", m.OpenSettings)
			m.welcomeRailItem(c, p, "plugins", "⬡", "插件", func() {
				m.welcomePage = "plugins"
				m.invalidate()
			})
			ui.Spacer(c)
			m.welcomeRailItem(c, p, "settings", "⚙", "设置", m.OpenSettings)
		})
}

// welcomeRailItem is one page of the rail: its glyph over its name, drawn as
// the selected page when it is the one showing.
func (m *Model) welcomeRailItem(c *ui.Context, p *ui.Theme, page, glyph, label string, onClick func()) {
	active := m.welcomePage == page || (page == "projects" && m.welcomePage == "")
	r := ui.Row(c.Key("welcomeRail:" + page)).Height(52).Shrink(0).AlignItems(ui.Center).
		Justify(ui.Center)
	if active {
		r.Background(p.SurfaceHover)
	} else if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		ui.Column(c).Gap(2).AlignItems(ui.Center).Children(func() {
			tone := p.TextMuted
			if active {
				tone = p.Accent
			}
			ui.Text(c, glyph).FontSize(17).TextColor(tone)
			ui.Text(c, label).FontSize(10).TextColor(tone).SingleLine()
		})
	})
	if r.Clicked() {
		onClick()
	}
}

func (m *Model) welcomeProjects(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Grow(1).MinHeight(0).Padding(44, 36).Gap(16).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Width(30).Height(30).Justify(ui.Center).AlignItems(ui.Center).
				Background(p.Accent).Children(func() {
				ui.Text(c, "G").FontSize(16).Bold().TextColor(p.AccentText)
			})
			ui.Column(c).Children(func() {
				ui.Text(c, "GoIDEA").FontSize(20).Bold().TextColor(p.Text).SingleLine()
				ui.Text(c, "Java 与 OpenIDEA 的后端").FontSize(11).TextColor(p.TextMuted).SingleLine()
			})
		})
		ui.Text(c, "项目").FontSize(18).Bold().TextColor(p.Text)
		search := ui.TextInput(c, &m.welcomeQuery).Width(380)
		search.Placeholder("搜索项目")
		ui.Row(c).Gap(10).Children(func() {
			m.welcomeTile(c, p, "welcomeNew", "新建项目", "选择一个空文件夹", false, m.NewProject)
			m.welcomeTile(c, p, "welcomeOpen", "打开", "打开已有的项目文件夹", false, m.chooseProject)
			m.welcomeTile(c, p, "welcomeClone", "克隆仓库", "尚未接入", true, nil)
			m.welcomeTile(c, p, "welcomeRemote", "远程开发", "尚未接入", true, nil)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "最近的项目").FontSize(11).Bold().TextColor(p.TextMuted).SingleLine()
			if n := len(m.recents); n > 0 {
				ui.Textf(c, "%d", n).FontSize(10.5).TextColor(p.TextMuted)
			}
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			m.welcomeRecents(c, p)
		})
	})
}

// welcomeTile is one large action of the Projects page.
func (m *Model) welcomeTile(c *ui.Context, p *ui.Theme, key, title, hint string, disabled bool, onClick func()) {
	r := ui.Row(c.Key(key)).Width(172).Height(64).Padding(10, 12).
		Background(p.Surface).BorderWidth(1, 1, 1, 1).BorderColor(p.Border)
	if !disabled && r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	r.Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			tone := p.Text
			if disabled {
				tone = p.TextMuted
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(tone).SingleLine()
			ui.Text(c, hint).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		})
	})
	if !disabled && r.Clicked() && onClick != nil {
		onClick()
	}
}

func (m *Model) welcomeRecents(c *ui.Context, p *ui.Theme) {
	list := m.filteredRecents()
	if len(list) == 0 {
		hint := "还没有最近的项目。点“打开”选择项目文件夹。"
		if strings.TrimSpace(m.welcomeQuery) != "" {
			hint = "没有匹配的项目"
		}
		ui.Row(c).Padding(4, 2).Children(func() {
			ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	for _, dir := range list {
		dir := dir
		r := ui.Row(c.Key("welcomeRecent:"+dir)).Height(48).Padding(0, 10).Gap(12).
			AlignItems(ui.Center)
		if r.Hovered() {
			r.Background(p.SurfaceHover)
		}
		r.Children(func() {
			ui.Row(c).Width(30).Height(30).Justify(ui.Center).AlignItems(ui.Center).
				Background(p.Accent.Mix(p.Background, 0.22)).Children(func() {
				ui.Text(c, projectInitial(dir)).FontSize(13).Bold().TextColor(p.Text)
			})
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, filepath.Base(dir)).FontSize(13).TextColor(p.Text).
					SingleLine().Ellipsis("…")
				ui.Text(c, shortenHome(dir)).FontSize(10.5).TextColor(p.TextMuted).
					SingleLine().Ellipsis("…")
			})
			ui.Spacer(c)
			ui.Text(c, "↩").FontSize(12).TextColor(p.TextMuted)
		})
		if r.Clicked() {
			m.confirmSwitch(dir)
		}
	}
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

// projectInitial is the letter the Welcome screen's avatar shows.
func projectInitial(dir string) string {
	name := filepath.Base(dir)
	for _, r := range name {
		if r == '.' || r == '_' || r == '-' || r == ' ' || r == '/' || r == '\\' {
			continue
		}
		return strings.ToUpper(string(r))
	}
	return "·"
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
func (m *Model) NewProject() { m.chooseProjectDialog("新建项目", "创建") }

func (m *Model) welcomePlugins(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Grow(1).Padding(44, 36).Gap(14).Children(func() {
		ui.Text(c, "插件").FontSize(18).Bold().TextColor(p.Text)
		ui.Text(c, "OpenIDEA 从它的插件目录安装插件；GoIDEA 还没有接入插件管理器。").
			FontSize(12).TextColor(p.TextMuted)
		ui.Row(c).Children(func() {
			ui.Button(c, "打开设置").OnClick(func() { m.OpenSettings() })
		})
	})
}

// chooseProject asks for a folder and opens it.
func (m *Model) chooseProject() { m.chooseProjectDialog("打开项目", "打开") }

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
