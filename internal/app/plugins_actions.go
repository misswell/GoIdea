package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/plugin"
)

func init() {
	for _, action := range []*Action{
		{
			ID:       "plugins.installFromDisk",
			Title:    "Install Plugin from Disk…",
			Category: "Plugins",
			Keywords: "plugin extension local folder import",
			RunWithArg: func(m *Model, arg any) {
				m.InstallPluginFromDisk(pluginPathArg(arg))
			},
		},
		{
			ID:       "plugins.updateAll",
			Title:    "Update All Plugins",
			Category: "Plugins",
			Keywords: "plugin extension upgrade marketplace",
			Run:      func(m *Model) { m.UpdateAllPlugins() },
		},
		{
			ID:       "plugins.reloadAll",
			Title:    "Reload Plugins",
			Category: "Plugins",
			Keywords: "plugin extension development restart",
			Run:      func(m *Model) { m.ReloadPlugins() },
		},
	} {
		_ = registerAction(action)
	}
}

func pluginPathArg(arg any) string {
	switch value := arg.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]string:
		return strings.TrimSpace(value["path"])
	case map[string]any:
		path, _ := value["path"].(string)
		return strings.TrimSpace(path)
	default:
		return ""
	}
}

// InstallPluginFromDisk links a local plugin folder into GoIDEA's isolated
// plugin directory. A missing path opens the native folder chooser.
func (m *Model) InstallPluginFromDisk(path string) {
	if m == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		parent := m.win
		go func() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent:      parent,
				Title:       "Install Plugin from Disk",
				ButtonLabel: "Install",
				Directory:   true,
			})
			if err != nil {
				m.post(func() { m.fail("", err) })
				return
			}
			if len(paths) == 0 {
				return
			}
			m.post(func() { m.installPluginFolder(paths[0]) })
		}()
		return
	}
	m.installPluginFolder(path)
}

func (m *Model) installPluginFolder(path string) {
	pg := m.pluginsPage()
	if pg.dirs.Plugins == "" {
		pg.err = "GoIDEA has no writable plugin folder."
		m.invalidate()
		return
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		pg.err = err.Error()
		m.invalidate()
		return
	}
	manifest, err := plugin.Link(pg.dirs.Plugins, path)
	if err != nil {
		pg.err = err.Error()
		m.invalidate()
		return
	}
	pg.err = ""
	pg.busy = ""
	pg.tab = 0
	pg.selected = manifest.Name
	pg.reg.Load()
	pg.sync(m)
	title := manifest.DisplayName
	if title == "" {
		title = manifest.Name
	}
	m.status = "Installed " + title
	m.invalidate()
}

// ReloadPlugins re-reads the isolated plugin folder and replaces the
// actions and views the previous generation contributed.
func (m *Model) ReloadPlugins() {
	if m == nil {
		return
	}
	pg := m.pluginsPage()
	pg.reg.Load()
	pg.sync(m)
	pg.err = ""
	m.status = "Plugins reloaded"
	m.invalidate()
}

// UpdateAllPlugins updates every plugin installed from Git. Individual
// failures are collected so one unavailable repository does not stop the
// remaining updates.
func (m *Model) UpdateAllPlugins() {
	if m == nil {
		return
	}
	pg := m.pluginsPage()
	if pg.busy != "" {
		return
	}
	records := plugin.ReadRecords(pg.dirs.Plugins)
	names := make([]string, 0, len(records))
	for name := range records {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		m.status = "No Git-installed plugins to update"
		m.invalidate()
		return
	}
	pg.busy = "all"
	pg.err = ""
	m.status = "Updating plugins…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		changed := 0
		var failures []string
		for _, name := range names {
			updated, err := plugin.Update(ctx, pg.dirs.Plugins, name)
			if err != nil {
				failures = append(failures, name+": "+err.Error())
				continue
			}
			if updated {
				changed++
			}
		}
		m.post(func() {
			pg.busy = ""
			if len(failures) > 0 {
				pg.err = fmt.Sprintf("Could not update %d plugin(s): %s",
					len(failures), strings.Join(failures, " · "))
			}
			pg.reg.Load()
			pg.sync(m)
			switch {
			case changed == 0 && len(failures) == 0:
				m.status = "All plugins are up to date"
			case changed == 0:
				m.status = "No plugins were updated"
			default:
				m.status = fmt.Sprintf("Updated %d plugin(s)", changed)
			}
			m.invalidate()
		})
	}()
}
