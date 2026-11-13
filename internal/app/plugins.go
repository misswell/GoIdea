package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/plugin"
)

// goideaPluginHost is GoIDEA as its plugins see it. The registry reads the
// enabled state of each plugin through Settings, which GoIDEA keeps in its
// settings file under the same "plugins" key Mayoi uses. No plugin's
// JavaScript runs yet, so Call answers that plainly instead of pretending.
type goideaPluginHost struct {
	m      *Model
	prefs  atomic.Pointer[map[string]json.RawMessage]
	folder atomic.Pointer[string]
	logMu  sync.Mutex
	logs   map[string][]plugin.LogLine
}

func (h *goideaPluginHost) setSettings(all map[string]json.RawMessage) {
	cp := make(map[string]json.RawMessage, len(all))
	for k, v := range all {
		cp[k] = v
	}
	h.prefs.Store(&cp)
}

func (h *goideaPluginHost) setFolder(folder string) { h.folder.Store(&folder) }

// Call implements plugin.Host.
func (h *goideaPluginHost) Call(*plugin.Plugin, string, json.RawMessage) (any, error) {
	return nil, errors.New("GoIDEA does not run plugin API calls yet")
}

// Settings implements plugin.Host: the plugin's object of plugins.<name>.
func (h *goideaPluginHost) Settings(name string) json.RawMessage {
	if all := h.prefs.Load(); all != nil {
		return (*all)[name]
	}
	return nil
}

// MayoiSetting implements plugin.Host: GoIDEA exposes none of its own.
func (h *goideaPluginHost) MayoiSetting(string) any { return nil }

// Folder implements plugin.Host.
func (h *goideaPluginHost) Folder() string {
	if f := h.folder.Load(); f != nil {
		return *f
	}
	return ""
}

// Changed implements plugin.Host: what the page shows changed.
func (h *goideaPluginHost) Changed() { h.m.invalidate() }

// Logged implements plugin.Host.
func (h *goideaPluginHost) Logged(p *plugin.Plugin, line plugin.LogLine) {
	h.logMu.Lock()
	h.logs[p.Name] = append(h.logs[p.Name], line)
	if n := len(h.logs[p.Name]); n > 200 {
		h.logs[p.Name] = h.logs[p.Name][n-200:]
	}
	h.logMu.Unlock()
}

func (h *goideaPluginHost) lines(name string) []plugin.LogLine {
	h.logMu.Lock()
	defer h.logMu.Unlock()
	return append([]plugin.LogLine(nil), h.logs[name]...)
}

// The Available tab's index, as it is read.
const (
	pluginIndexIdle = iota
	pluginIndexLoading
	pluginIndexLoaded
	pluginIndexFailed
)

// pluginPage is the state of Welcome › Plugins: the registry over the
// user's plugin folder, and what the page is showing of it.
type pluginPage struct {
	dirs plugin.Dirs
	reg  *plugin.Registry
	host *goideaPluginHost

	tab         int // 0 Installed, 1 Available
	query       string
	selected    string
	indexSel    string
	detailTab   int
	gearOpen    bool
	installOpen bool
	installSpec string
	busy        string
	err         string
	confirm     string

	index      []plugin.IndexEntry
	indexState int
	indexErr   string
}

// pluginDirs are where GoIDEA keeps its plugins: beside its settings file,
// never the user's Mayoi or OpenIDEA folders. A model without a settings
// path (the render tests) has no folder, and finds no plugins.
func (m *Model) pluginDirs() plugin.Dirs {
	if m.cfg.SettingsPath == "" {
		return plugin.Dirs{}
	}
	data := filepath.Dir(m.cfg.SettingsPath)
	if data == "" || data == "." {
		return plugin.Dirs{}
	}
	return plugin.Dirs{
		Plugins: filepath.Join(data, "plugins"),
		Scripts: filepath.Join(data, "plugin-scripts"),
		Storage: filepath.Join(data, "plugin-storage"),
	}
}

// pluginsPage makes the registry on first use, as Opening Welcome › Plugins
// does, and lists the plugins it finds.
func (m *Model) pluginsPage() *pluginPage {
	if m.plugins != nil {
		return m.plugins
	}
	h := &goideaPluginHost{m: m, logs: map[string][]plugin.LogLine{}}
	h.setSettings(m.prefs.Plugins)
	h.setFolder(m.Root())
	pg := &pluginPage{host: h, dirs: m.pluginDirs()}
	pg.reg = plugin.New(pg.dirs, h)
	pg.reg.Version = mygo.App.Version()
	m.plugins = pg
	pg.reg.Load()
	return pg
}

// pluginIsEnabled is the state the settings leave the plugin in, or its
// manifest's default before the page changed it.
func (m *Model) pluginIsEnabled(p *plugin.Plugin) bool {
	raw := m.prefs.Plugins[p.Name]
	if len(raw) == 0 {
		return p.DefaultOn()
	}
	var s struct {
		Enabled *bool `json:"enabled"`
	}
	json.Unmarshal(raw, &s)
	if s.Enabled == nil {
		return p.DefaultOn()
	}
	return *s.Enabled
}

// setPluginEnabled turns a plugin on or off, persists the choice and tells
// the registry, as the row's checkbox does.
func (m *Model) setPluginEnabled(name string, on bool) {
	pg := m.pluginsPage()
	if m.prefs.Plugins == nil {
		m.prefs.Plugins = map[string]json.RawMessage{}
	}
	var obj map[string]json.RawMessage
	if raw := m.prefs.Plugins[name]; len(raw) > 0 {
		json.Unmarshal(raw, &obj)
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	obj["enabled"], _ = json.Marshal(on)
	m.prefs.Plugins[name], _ = json.Marshal(obj)
	m.savePreferences()
	pg.host.setSettings(m.prefs.Plugins)
	pg.reg.Toggled(name)
	pg.err = ""
	m.invalidate()
}

// removePlugin deletes an installed plugin and lists the folder again.
func (m *Model) removePlugin(name string) {
	pg := m.pluginsPage()
	pg.confirm = ""
	if err := plugin.Remove(pg.dirs.Plugins, name); err != nil {
		pg.err = err.Error()
		m.invalidate()
		return
	}
	if pg.selected == name {
		pg.selected = ""
	}
	delete(m.prefs.Plugins, name)
	m.savePreferences()
	pg.host.setSettings(m.prefs.Plugins)
	pg.reg.Load()
	m.invalidate()
}

// installPlugin clones a plugin from a Git URL, owner/repo or index name,
// as Mayoi's own Plugins page does, off the main thread.
func (m *Model) installPlugin(spec string) {
	pg := m.pluginsPage()
	spec = strings.TrimSpace(spec)
	if spec == "" {
		pg.err = "Type a Git URL, owner/repo, or an index name."
		m.invalidate()
		return
	}
	if pg.busy != "" {
		return
	}
	url, ref, byName := plugin.SplitSpec(spec)
	pg.busy, pg.err, pg.installOpen = spec, "", false
	m.invalidate()
	dir := pg.dirs.Plugins
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if byName {
			entries, err := plugin.FetchIndex(ctx, pg.reg.HTTP)
			if err != nil {
				m.pluginInstallDone("", err)
				return
			}
			url = ""
			for _, e := range entries {
				if e.Name == spec {
					url = e.Repository
					break
				}
			}
			if url == "" {
				m.pluginInstallDone("", fmt.Errorf("the index lists no plugin named %s", spec))
				return
			}
		}
		manifest, err := plugin.Install(ctx, dir, url, ref)
		if err != nil {
			m.pluginInstallDone("", err)
			return
		}
		m.pluginInstallDone(manifest.Name, nil)
	}()
}

func (m *Model) pluginInstallDone(name string, err error) {
	m.post(func() {
		pg := m.pluginsPage()
		pg.busy = ""
		if err != nil {
			pg.err = "Install failed: " + err.Error()
			m.invalidate()
			return
		}
		pg.reg.Load()
		pg.tab, pg.selected, pg.query = 0, name, ""
		m.invalidate()
	})
}

// loadPluginIndex reads the plugin index for the Available tab, off the
// main thread, once.
func (m *Model) loadPluginIndex() {
	pg := m.pluginsPage()
	if pg.indexState == pluginIndexLoading {
		return
	}
	pg.indexState, pg.indexErr = pluginIndexLoading, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, err := plugin.FetchIndex(ctx, pg.reg.HTTP)
		m.post(func() {
			if err != nil {
				pg.indexState, pg.indexErr = pluginIndexFailed, err.Error()
			} else {
				pg.index, pg.indexState = entries, pluginIndexLoaded
			}
			m.invalidate()
		})
	}()
}

// pluginList are the installed plugins the search leaves, bundled first and
// then by name as the registry lists them.
func (m *Model) pluginList(pg *pluginPage) []*plugin.Plugin {
	all := pg.reg.Plugins()
	q := strings.ToLower(strings.TrimSpace(pg.query))
	if q == "" {
		return all
	}
	var out []*plugin.Plugin
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Title()), q) ||
			strings.Contains(strings.ToLower(p.Description), q) ||
			strings.Contains(strings.ToLower(p.Author), q) {
			out = append(out, p)
		}
	}
	return out
}

func (m *Model) pluginSelected(pg *pluginPage, list []*plugin.Plugin) *plugin.Plugin {
	for _, p := range list {
		if p.Name == pg.selected {
			return p
		}
	}
	if len(list) > 0 {
		pg.selected = list[0].Name
		return list[0]
	}
	return nil
}

func (pg *pluginPage) source(p *plugin.Plugin) string {
	o := pg.reg.Origin(p.Name)
	switch {
	case p.Bundled:
		return "Bundled"
	case p.Script:
		return "Script"
	case o.FromGit:
		return "Git"
	case o.Linked:
		return "Linked"
	}
	return "Local"
}

func pluginInitial(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// pluginPageUI is Welcome › Plugins, laid out as OpenIDEA's PluginsPage:
// the Available/Installed tabs over a searchable list, and the selected
// plugin's details beside it.
func (m *Model) pluginPageUI(c *ui.Context, oi oiColors) {
	pg := m.pluginsPage()
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Padding(12, 16, 8, 16).Gap(8).
		Children(func() {
			m.pluginTopBar(c, oi, pg)
			if pg.err != "" {
				ui.Text(c, pg.err).FontSize(12).TextColor(oi.danger).Wrap()
			}
			if pg.installOpen {
				m.pluginInstallRow(c, oi, pg)
			}
			ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).BorderWidth(1).
				BorderColor(oi.borderStrong).Radius(6).Children(func() {
				m.pluginSide(c, oi, pg)
				if pg.tab == 1 {
					m.pluginAvailableDetails(c, oi, pg)
				} else {
					m.pluginDetails(c, oi, pg)
				}
			})
		})
}

func (m *Model) pluginTopBar(c *ui.Context, oi oiColors, pg *pluginPage) {
	ui.Row(c).Height(38).Shrink(0).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		m.pluginTabLabel(c, oi, pg, "Available", 1, "")
		m.pluginTabLabel(c, oi, pg, "Installed", 0,
			fmt.Sprintf("%d", len(pg.reg.Plugins())))
		ui.Row(c).Height(18).Padding(0, 6).Radius(9).Background(oi.selected).
			AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Preview").FontSize(10).TextColor(oi.link).SingleLine()
		})
		ui.Spacer(c)
		if pg.busy != "" {
			ui.Text(c, "Installing…").FontSize(11).TextColor(oi.textMuted).SingleLine()
		}
		m.pluginGear(c, oi, pg)
	})
}

func (m *Model) pluginTabLabel(c *ui.Context, oi oiColors, pg *pluginPage, label string, tab int, count string) {
	active := pg.tab == tab
	r := ui.Row(c.Key("pluginTab:"+label)).Height(38).Shrink(0).Padding(0, 8).
		Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		BorderWidth(0, 0, 2, 0)
	if active {
		r.BorderColor(oi.accent)
	} else {
		r.BorderColor(oi.island)
	}
	r.Children(func() {
		tone := oi.textMuted
		if active {
			tone = oi.textStrong
		}
		ui.Text(c, label).FontSize(13).TextColor(tone).SingleLine()
		if count != "" {
			ui.Text(c, count).FontSize(11).TextColor(oi.textMuted).SingleLine()
		}
	})
	if r.Clicked() {
		pg.tab = tab
		pg.err = ""
		if tab == 1 && pg.indexState == pluginIndexIdle {
			m.loadPluginIndex()
		}
		m.invalidate()
	}
}

func (m *Model) pluginGear(c *ui.Context, oi oiColors, pg *pluginPage) {
	g := ui.ButtonBase(c.Key("pluginGear")).Size(24, 24).Radius(6).
		AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
		Tooltip("Manage Plugins")
	if g.Hovered() {
		g.Background(oi.hoverTint)
	}
	g.Children(func() {
		ui.Icon(c, iconWelcomeGear).Size(16, 16).TextColor(oi.textSecondary)
	})
	if g.Clicked() {
		pg.gearOpen = !pg.gearOpen
		m.invalidate()
	}
	open := pg.gearOpen
	ui.PopoverBase(c, g, &open, func(panel ui.Element) {
		panel.Width(240).Padding(4).Background(oi.popup).
			BorderWidth(1).BorderColor(oi.borderStrong).Radius(8)
		panel.Children(func() {
			m.welcomeGearItem(c, oi, "Install Plugin from Git…", func() {
				pg.gearOpen = false
				pg.installOpen = true
				pg.err = ""
				m.invalidate()
			})
			m.welcomeGearItem(c, oi, "Reload Installed Plugins", func() {
				pg.gearOpen = false
				pg.reg.Load()
				m.invalidate()
			})
		})
	})
}

func (m *Model) pluginInstallRow(c *ui.Context, oi oiColors, pg *pluginPage) {
	ui.Row(c).MinHeight(32).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		input := ui.TextInputBase(c, &pg.installSpec).Grow(1).Height(28)
		input.Placeholder("Git URL, owner/repo or an index name")
		if input.Submitted() {
			m.installPlugin(pg.installSpec)
		}
		m.pluginButton(c, oi, "Install", true, func() {
			m.installPlugin(pg.installSpec)
		})
		m.pluginButton(c, oi, "Cancel", false, func() {
			pg.installOpen = false
			m.invalidate()
		})
	})
}

// pluginButton is one small button of the page.
func (m *Model) pluginButton(c *ui.Context, oi oiColors, label string, primary bool, run func()) {
	b := ui.ButtonBase(c.Key("pluginButton:"+label)).Height(24).MinWidth(56).
		Padding(0, 10).Radius(4).AlignItems(ui.Center).Justify(ui.Center).
		Cursor(ui.CursorPointer).BorderWidth(1).BorderColor(oi.buttonBorder)
	if primary {
		b.Background(oi.accent).BorderColor(oi.accent)
	} else if b.Hovered() {
		b.Background(oi.buttonHover)
	}
	b.Children(func() {
		tone := oi.text
		if primary {
			tone = ui.Hex("#ffffff")
		}
		ui.Text(c, label).FontSize(12).TextColor(tone).SingleLine()
	})
	if b.Clicked() {
		run()
	}
}

func (m *Model) pluginSide(c *ui.Context, oi oiColors, pg *pluginPage) {
	ui.Column(c).Width(280).Shrink(0).MinHeight(0).
		BorderWidth(0, 1, 0, 0).BorderColor(oi.border).Children(func() {
		ui.Row(c).Height(44).Shrink(0).Padding(8, 10).Gap(2).AlignItems(ui.Center).
			Children(func() {
				ui.Icon(c, iconWelcomeSearch).Size(14, 14).
					TextColor(oi.textMuted).MarginEnd(4)
				input := ui.TextInputBase(c, &pg.query).Grow(1)
				if pg.tab == 1 {
					input.Placeholder("Search plugins")
				} else {
					input.Placeholder("Search installed plugins")
				}
				if input.Changed() {
					m.invalidate()
				}
			})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(0, 0, 4, 0).Children(func() {
				if pg.tab == 1 {
					m.pluginIndexList(c, oi, pg)
				} else {
					m.pluginInstalledList(c, oi, pg)
				}
			})
		})
	})
}

func (m *Model) pluginGroupHeader(c *ui.Context, oi oiColors, title string, count int) {
	ui.Row(c).Height(28).Shrink(0).Padding(0, 12).Gap(6).AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, title).FontSize(11).TextColor(oi.textMuted).SingleLine()
			ui.Text(c, fmt.Sprintf("%d", count)).FontSize(11).
				TextColor(oi.textFaint).SingleLine()
		})
}

func (m *Model) pluginInstalledList(c *ui.Context, oi oiColors, pg *pluginPage) {
	list := m.pluginList(pg)
	selected := m.pluginSelected(pg, list)
	var downloaded, linked []*plugin.Plugin
	for _, p := range list {
		if pg.reg.Origin(p.Name).Linked {
			linked = append(linked, p)
		} else {
			downloaded = append(downloaded, p)
		}
	}
	if len(downloaded) > 0 {
		m.pluginGroupHeader(c, oi, "Downloaded", len(downloaded))
		for _, p := range downloaded {
			m.pluginRow(c, oi, pg, p, selected == p)
		}
	}
	if len(linked) > 0 {
		m.pluginGroupHeader(c, oi, "Linked for development", len(linked))
		for _, p := range linked {
			m.pluginRow(c, oi, pg, p, selected == p)
		}
	}
	if len(list) == 0 {
		text := "No plugins match."
		if len(pg.reg.Plugins()) == 0 {
			text = "No plugins installed."
		}
		ui.Text(c, text).FontSize(12).TextColor(oi.textMuted).Padding(0, 12)
	}
}

func (m *Model) pluginRow(c *ui.Context, oi oiColors, pg *pluginPage, p *plugin.Plugin, selected bool) {
	r := ui.Row(c.Key("pluginRow:"+p.Name)).Height(48).Shrink(0).
		Padding(0, 8).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	switch {
	case selected:
		r.Background(oi.selected)
	case r.Hovered():
		r.Background(oi.hoverTint)
	}
	r.Children(func() {
		m.pluginTile(c, oi, pluginInitial(p.Title()), 32)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, p.Title()).FontSize(13).TextColor(oi.textStrong).
				SingleLine().Ellipsis(p.Title())
			meta := pg.source(p)
			if p.Version != "" {
				meta = p.Version + " · " + meta
			}
			if p.Author != "" {
				meta = p.Author + " · " + meta
			}
			ui.Text(c, meta).FontSize(11).TextColor(oi.textMuted).
				SingleLine().Ellipsis(meta)
		})
		on := m.pluginIsEnabled(p)
		box := ui.Checkbox(c, &on, "")
		if box.Changed() {
			m.setPluginEnabled(p.Name, on)
		}
	})
	if r.Clicked() && !selected {
		pg.selected = p.Name
		m.invalidate()
	}
}

func (m *Model) pluginIndexList(c *ui.Context, oi oiColors, pg *pluginPage) {
	switch pg.indexState {
	case pluginIndexFailed:
		ui.Text(c, pg.indexErr).FontSize(12).TextColor(oi.danger).
			Padding(0, 12).Wrap()
		return
	case pluginIndexLoaded:
	default:
		ui.Text(c, "Loading the plugin index…").FontSize(12).
			TextColor(oi.textMuted).Padding(0, 12)
		return
	}
	q := strings.ToLower(strings.TrimSpace(pg.query))
	var shown []plugin.IndexEntry
	for _, e := range pg.index {
		if q == "" || strings.Contains(strings.ToLower(e.Name), q) ||
			strings.Contains(strings.ToLower(e.DisplayName), q) ||
			strings.Contains(strings.ToLower(e.Description), q) ||
			strings.Contains(strings.ToLower(e.Author), q) {
			shown = append(shown, e)
		}
	}
	if len(shown) == 0 {
		ui.Text(c, "No plugins match.").FontSize(12).TextColor(oi.textMuted).
			Padding(0, 12)
		return
	}
	for _, e := range shown {
		selected := pg.indexSel == e.Name
		r := ui.Row(c.Key("pluginIndex:"+e.Name)).MinHeight(48).Shrink(0).
			Padding(0, 12).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		switch {
		case selected:
			r.Background(oi.selected)
		case r.Hovered():
			r.Background(oi.hoverTint)
		}
		title := e.DisplayName
		if title == "" {
			title = e.Name
		}
		installed := pg.reg.Plugin(e.Name) != nil
		r.Children(func() {
			m.pluginTile(c, oi, pluginInitial(title), 32)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, title).FontSize(13).TextColor(oi.textStrong).
					SingleLine().Ellipsis(title)
				ui.Text(c, e.Author).FontSize(11).TextColor(oi.textMuted).
					SingleLine().Ellipsis(e.Author)
			})
			if installed {
				ui.Text(c, "Installed").FontSize(11).TextColor(oi.textMuted).SingleLine()
			} else {
				spec := e.Repository
				if spec == "" {
					spec = e.Name
				}
				m.pluginButton(c, oi, "Install", false, func() {
					m.installPlugin(spec)
				})
			}
		})
		if r.Clicked() {
			pg.indexSel = e.Name
			m.invalidate()
		}
	}
}

func (m *Model) pluginTile(c *ui.Context, oi oiColors, initial string, size float32) {
	ui.Row(c).Size(size, size).Shrink(0).Radius(6).Background(oi.elev).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Text(c, initial).FontSize(size * 0.42).TextColor(oi.textSecondary).SingleLine()
	})
}

func (m *Model) pluginDetails(c *ui.Context, oi oiColors, pg *pluginPage) {
	list := m.pluginList(pg)
	p := m.pluginSelected(pg, list)
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		if p == nil {
			m.pluginDetailsEmpty(c, oi, pg)
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(12, 16, 16, 16).Gap(2).Children(func() {
				m.pluginHeader(c, oi, pg, p)
				m.pluginDetailTabs(c, oi, pg)
				if pg.detailTab == 1 {
					ui.Text(c, "This version has no release notes.").FontSize(12).
						TextColor(oi.textMuted).Padding(0, 4).Margin(8, 0, 0, 0)
				} else {
					if p.Description != "" {
						ui.Text(c, p.Description).FontSize(12).TextColor(oi.text).
							Wrap().Margin(8, 0, 0, 0)
					}
					m.pluginFacts(c, oi, pg, p)
					m.pluginPoints(c, oi, p)
					m.pluginLogs(c, oi, pg, p)
				}
			})
		})
	})
}

func (m *Model) pluginDetailsEmpty(c *ui.Context, oi oiColors, pg *pluginPage) {
	ui.Column(c).Grow(1).Padding(16, 20).Gap(6).AlignItems(ui.Start).Children(func() {
		ui.Text(c, "No plugin selected").FontSize(14).TextColor(oi.textStrong).SingleLine()
		ui.Text(c, "Plugins are folders with a plugin.json manifest.").
			FontSize(12).TextColor(oi.textMuted).Wrap()
		ui.Text(c, "Install one from Git with the gear menu.").
			FontSize(12).TextColor(oi.textMuted).Wrap()
		if pg.dirs.Plugins != "" {
			ui.Text(c, shortenHome(pg.dirs.Plugins)).FontSize(11).
				TextColor(oi.textFaint).Margin(0, 4, 0, 0).Wrap()
		}
	})
}

func (m *Model) pluginHeader(c *ui.Context, oi oiColors, pg *pluginPage, p *plugin.Plugin) {
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		m.pluginTile(c, oi, pluginInitial(p.Title()), 40)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Title()).FontSize(15).Bold().
					TextColor(oi.textStrong).SingleLine()
				if p.Version != "" {
					ui.Text(c, p.Version).FontSize(12).TextColor(oi.textMuted).SingleLine()
				}
			})
			meta := pg.source(p)
			if p.Author != "" {
				meta = p.Author + " · " + meta
			}
			ui.Text(c, meta).FontSize(12).TextColor(oi.textSecondary).SingleLine()
		})
		ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
			m.pluginButton(c, oi, "Reload", false, func() {
				pg.reg.Restart(p.Name)
				m.invalidate()
			})
			if !p.Bundled {
				m.pluginButton(c, oi, "Remove", false, func() {
					pg.confirm = p.Name
					m.invalidate()
				})
			}
		})
	})
	if pg.confirm == p.Name {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(10, 0, 0, 0).Children(func() {
			ui.Text(c, fmt.Sprintf("Remove %s? Its folder is deleted.", p.Title())).
				FontSize(12).TextColor(oi.danger).Grow(1).MinWidth(0).Wrap()
			m.pluginButton(c, oi, "Remove", true, func() {
				m.removePlugin(p.Name)
			})
			m.pluginButton(c, oi, "Cancel", false, func() {
				pg.confirm = ""
				m.invalidate()
			})
		})
	}
}

func (m *Model) pluginDetailTabs(c *ui.Context, oi oiColors, pg *pluginPage) {
	ui.Row(c).Gap(4).Margin(12, 0, 0, 0).Children(func() {
		for i, label := range []string{"Overview", "What's New"} {
			active := pg.detailTab == i
			r := ui.Row(c.Key("pluginDetailTab:"+label)).Height(28).Shrink(0).
				Padding(0, 8).AlignItems(ui.Center).Cursor(ui.CursorPointer).
				BorderWidth(0, 0, 2, 0)
			if active {
				r.BorderColor(oi.accent)
			} else {
				r.BorderColor(oi.island)
			}
			tone := oi.textMuted
			if active {
				tone = oi.textStrong
			}
			r.Children(func() {
				ui.Text(c, label).FontSize(12).TextColor(tone).SingleLine()
			})
			if r.Clicked() {
				pg.detailTab = i
				m.invalidate()
			}
		}
	})
}

func (m *Model) pluginFact(c *ui.Context, oi oiColors, label, value string) {
	ui.Row(c).MinHeight(22).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(oi.textMuted).Width(96).SingleLine()
		ui.Text(c, value).FontSize(12).TextColor(oi.text).Grow(1).MinWidth(0).
			SingleLine().Ellipsis(value)
	})
}

func (m *Model) pluginFacts(c *ui.Context, oi oiColors, pg *pluginPage, p *plugin.Plugin) {
	ui.Column(c).Gap(2).Margin(12, 0, 0, 0).Children(func() {
		m.pluginFact(c, oi, "Id", p.Name)
		m.pluginFact(c, oi, "Version", pluginOrDash(p.Version))
		m.pluginFact(c, oi, "Author", pluginOrDash(p.Author))
		m.pluginFact(c, oi, "Source", pg.source(p))
		if p.Mayoi != "" {
			m.pluginFact(c, oi, "Needs Mayoi", p.Mayoi)
		}
		if p.Dir != "" {
			m.pluginFact(c, oi, "Location", shortenHome(p.Dir))
		}
		m.pluginFact(c, oi, "State", m.pluginState(pg, p))
	})
}

func pluginOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (m *Model) pluginState(pg *pluginPage, p *plugin.Plugin) string {
	st := p.Stats()
	switch {
	case st.Crashed:
		return "Stopped after " + fmt.Sprintf("%d", st.Crashes) + " crashes"
	case st.Err != "":
		return "Failed: " + st.Err
	case st.Disabled:
		return "Disabled"
	case st.Active:
		return "Running"
	}
	return "Enabled"
}

type pluginPoint struct {
	title string
	items []string
}

// pluginPoints are the plugin's contributions, grouped as the details
// section lists them.
func pluginPoints(p *plugin.Plugin) []pluginPoint {
	var out []pluginPoint
	add := func(title string, items []string) {
		if len(items) > 0 {
			out = append(out, pluginPoint{title: title, items: items})
		}
	}
	var commands []string
	for _, c := range p.Contributes.Commands {
		commands = append(commands, c.Title)
	}
	add("Commands", commands)
	var settings []string
	for key := range p.Contributes.Settings {
		settings = append(settings, key)
	}
	sort.Strings(settings)
	add("Settings", settings)
	var views []string
	for _, v := range p.Contributes.Views {
		views = append(views, v.Title)
	}
	add("Views", views)
	var themes []string
	for _, t := range p.Contributes.Themes {
		themes = append(themes, t.Label)
	}
	add("Themes", themes)
	var languages []string
	for _, l := range p.Contributes.Languages {
		languages = append(languages, l.ID)
	}
	add("Languages", languages)
	var servers []string
	for _, s := range p.Contributes.LanguageServers {
		servers = append(servers, s.ID)
	}
	add("Language servers", servers)
	var status []string
	for _, s := range p.Contributes.StatusItems {
		status = append(status, s.ID)
	}
	add("Status bar items", status)
	return out
}

func (m *Model) pluginPoints(c *ui.Context, oi oiColors, p *plugin.Plugin) {
	for _, point := range pluginPoints(p) {
		ui.Column(c).Gap(2).Margin(16, 0, 0, 0).Children(func() {
			ui.Text(c, point.title).FontSize(12).Bold().
				TextColor(oi.textSecondary).SingleLine()
			ui.Column(c).BorderWidth(1).BorderColor(oi.border).Radius(4).
				Children(func() {
					for i, item := range point.items {
						if i > 0 {
							ui.Row(c).Height(1).Shrink(0).Background(oi.border)
						}
						ui.Row(c).Height(26).Shrink(0).Padding(0, 10).
							AlignItems(ui.Center).Children(func() {
							ui.Text(c, item).FontSize(12).TextColor(oi.text).
								SingleLine().Ellipsis(item)
						})
					}
				})
		})
	}
}

func (m *Model) pluginLogs(c *ui.Context, oi oiColors, pg *pluginPage, p *plugin.Plugin) {
	lines := pg.host.lines(p.Name)
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	if len(lines) == 0 {
		return
	}
	ui.Column(c).Gap(2).Margin(16, 0, 0, 0).Children(func() {
		ui.Text(c, "Log").FontSize(12).Bold().TextColor(oi.textSecondary).SingleLine()
		for _, line := range lines {
			tone := oi.textMuted
			if line.Level == "error" {
				tone = oi.danger
			}
			text := line.Level + ": " + line.Message
			ui.Text(c, text).FontSize(11).TextColor(tone).SingleLine().Ellipsis(text)
		}
	})
}

// pluginAvailableDetails shows the index entry beside the list, or the
// empty state of the Available tab.
func (m *Model) pluginAvailableDetails(c *ui.Context, oi oiColors, pg *pluginPage) {
	var entry *plugin.IndexEntry
	for i := range pg.index {
		if pg.index[i].Name == pg.indexSel {
			entry = &pg.index[i]
			break
		}
	}
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		if entry == nil {
			ui.Column(c).Grow(1).Padding(16, 20).Gap(6).AlignItems(ui.Start).
				Children(func() {
					ui.Text(c, "No plugin selected").FontSize(14).
						TextColor(oi.textStrong).SingleLine()
					ui.Text(c, "Install a plugin from a Git URL with the gear menu.").
						FontSize(12).TextColor(oi.textMuted).Wrap()
				})
			return
		}
		title := entry.DisplayName
		if title == "" {
			title = entry.Name
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(12, 16, 16, 16).Gap(2).Children(func() {
				ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
					m.pluginTile(c, oi, pluginInitial(title), 40)
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, title).FontSize(15).Bold().
							TextColor(oi.textStrong).SingleLine()
						ui.Text(c, entry.Author).FontSize(12).
							TextColor(oi.textSecondary).SingleLine()
					})
					if pg.reg.Plugin(entry.Name) == nil {
						spec := entry.Repository
						if spec == "" {
							spec = entry.Name
						}
						m.pluginButton(c, oi, "Install", true, func() {
							m.installPlugin(spec)
						})
					}
				})
				if entry.Description != "" {
					ui.Text(c, entry.Description).FontSize(12).TextColor(oi.text).
						Wrap().Margin(10, 0, 0, 0)
				}
				ui.Column(c).Gap(2).Margin(12, 0, 0, 0).Children(func() {
					m.pluginFact(c, oi, "Name", entry.Name)
					if entry.Repository != "" {
						m.pluginFact(c, oi, "Repository", entry.Repository)
					}
					if len(entry.Tags) > 0 {
						m.pluginFact(c, oi, "Tags", strings.Join(entry.Tags, ", "))
					}
				})
			})
		})
	})
}
