package app

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// addRunConfigTypeMenu mirrors OpenIDEA's Add New Configuration menu: built-in
// and plugin kinds are grouped, and picking a row creates that type.
func (m *Model) addRunConfigTypeMenu(menu *ui.Menu) {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	types := adapter.RunConfigTypes(d.items)
	groups := make([]string, 0)
	byGroup := map[string][]adapter.RunConfigType{}
	for _, typ := range types {
		if _, ok := byGroup[typ.Group]; !ok {
			groups = append(groups, typ.Group)
		}
		byGroup[typ.Group] = append(byGroup[typ.Group], typ)
	}
	for _, group := range groups {
		rows := byGroup[group]
		menu.Submenu(group, func(sub *ui.Menu) {
			for _, row := range rows {
				kind := row.ID
				if sub.Item(row.Label).Chosen() {
					m.AddRunConfigOfType(kind)
				}
			}
		})
	}
}

// beforeLaunchMenu is the task chooser under "Add task".
func (m *Model) beforeLaunchMenu(cfg *openidea.RunConfig, menu *ui.Menu) {
	d := m.runCfgDlg
	if cfg == nil || d == nil {
		return
	}
	add := func(kind, target string) {
		cfg.BeforeLaunch = append(cfg.BeforeLaunch, openidea.BeforeLaunchTask{Kind: kind, Target: target})
		m.invalidate()
	}
	var runnable []*openidea.RunConfig
	for _, candidate := range d.items {
		if candidate != nil && candidate.ID != cfg.ID && candidate.Kind != "compound" {
			runnable = append(runnable, candidate)
		}
	}
	menu.Submenu("Run Another Configuration", func(sub *ui.Menu) {
		if len(runnable) == 0 {
			sub.Item("No runnable configurations").Disabled(true)
			return
		}
		for _, candidate := range runnable {
			id := candidate.ID
			if sub.Item(candidate.Name).Chosen() {
				add("run", id)
			}
		}
	})
	if menu.Item("Build Project").Chosen() {
		add("build", "")
	}
	if cfg.Module == nil {
		menu.Item("Build Module…").Disabled(true)
	} else {
		module := *cfg.Module
		if menu.Item("Build Module…").Chosen() {
			add("buildModule", module)
		}
	}
	if menu.Item("Build Artifacts").Chosen() {
		add("buildArtifacts", "")
	}
	if cfg.Kind == "dotnet" && menu.Item("Build .NET project").Chosen() {
		add("dotnetBuild", "")
	}
	if len(m.prefs.ExternalTools) > 0 {
		menu.Submenu("Run External Tool", func(sub *ui.Menu) {
			for _, tool := range m.prefs.ExternalTools {
				tool := tool
				if sub.Item(tool.Name).Chosen() {
					add("tool", tool.ID)
				}
			}
		})
	} else {
		menu.Item("Run External Tool").Disabled(true)
	}
	menu.Separator()
	if menu.Item("Run Shell Command").Chosen() {
		add("command", "")
	}
	menu.Submenu("Activate Tool Window", func(sub *ui.Menu) {
		windows := append([]string(nil), toolWindows...)
		sort.Strings(windows)
		for _, id := range windows {
			windowID := id
			if sub.Item(toolWindowTitle(windowID)).Chosen() {
				add("toolWindow", windowID)
			}
		}
	})
}

// toolWindowTitle is the user-facing title of a registered tool-window id.
func toolWindowTitle(id string) string {
	if spec := dockSpec(id); spec != nil && strings.TrimSpace(spec.title) != "" {
		return spec.title
	}
	if id == "" {
		return "Tool Window"
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

func beforeLaunchTaskLabel(
	task openidea.BeforeLaunchTask,
	configs []*openidea.RunConfig,
	tools []ExternalToolPrefs,
) string {
	target := task.Target
	switch task.Kind {
	case "run":
		for _, cfg := range configs {
			if cfg.ID == target {
				return "Run '" + cfg.Name + "'"
			}
		}
		return "Run '" + target + "'"
	case "build":
		for _, cfg := range configs {
			if cfg.ID == target {
				return cfg.Name
			}
		}
		return "Build Project"
	case "buildModule":
		if target == "" {
			return "Build Module"
		}
		return "Build Module '" + target + "'"
	case "buildArtifacts":
		return "Build Artifacts"
	case "dotnetBuild":
		return "Build .NET project"
	case "tool":
		for _, tool := range tools {
			if tool.ID == target {
				return "External tool '" + tool.Name + "'"
			}
		}
		return "External tool '" + target + "'"
	case "command":
		if target == "" {
			return "Run shell command"
		}
		return "Run '" + target + "'"
	case "toolWindow":
		return "Activate '" + toolWindowTitle(target) + "' tool window"
	default:
		return task.Kind
	}
}

func moveBeforeLaunchTask(cfg *openidea.RunConfig, index, delta int) {
	if cfg == nil {
		return
	}
	next := index + delta
	if index < 0 || index >= len(cfg.BeforeLaunch) || next < 0 || next >= len(cfg.BeforeLaunch) {
		return
	}
	cfg.BeforeLaunch[index], cfg.BeforeLaunch[next] = cfg.BeforeLaunch[next], cfg.BeforeLaunch[index]
}

func removeBeforeLaunchTask(cfg *openidea.RunConfig, index int) {
	if cfg == nil || index < 0 || index >= len(cfg.BeforeLaunch) {
		return
	}
	cfg.BeforeLaunch = append(cfg.BeforeLaunch[:index], cfg.BeforeLaunch[index+1:]...)
}

func (m *Model) compoundMemberMenu(cfg *openidea.RunConfig, menu *ui.Menu) {
	d := m.runCfgDlg
	if cfg == nil || d == nil {
		return
	}
	added := make(map[string]bool, len(cfg.Compound))
	for _, id := range cfg.Compound {
		added[id] = true
	}
	candidates := 0
	for _, candidate := range d.items {
		if candidate == nil || candidate.ID == cfg.ID || added[candidate.ID] {
			continue
		}
		probe := *cfg
		probe.Compound = append(append([]string(nil), cfg.Compound...), candidate.ID)
		if adapter.CompoundCycle(&probe, d.items) {
			continue
		}
		candidates++
		id := candidate.ID
		if menu.Item(candidate.Name).Chosen() {
			cfg.Compound = append(cfg.Compound, id)
			m.invalidate()
		}
	}
	if candidates == 0 {
		menu.Item("No available configurations").Disabled(true)
	}
}
