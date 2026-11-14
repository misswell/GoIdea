package app

// The Services tool window after OpenIDEA's services feature: one tree over
// the build tool, the run configurations and the language tooling, with each
// row running the action it names. The rows come from real project state: the
// detected build system, the backend's run configurations and the settings
// that hold the JDK.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// servicesPane is the Services tool window's state.
type servicesPane struct {
	visible bool
}

// serviceRow is one line of the tree.
type serviceRow struct {
	section string
	label   string
	detail  string
	run     func(m *Model)
}

// ShowServices opens Services in the bottom area and reads the build model so
// the build rows come from the engine rather than from file detection.
func (m *Model) ShowServices() {
	m.DockShow("services")
	m.loadBuildModel()
}

// CloseServices hides it without running a toggle's side effects.
func (m *Model) CloseServices() { m.DockHide("services") }

// ToggleServices is the Window menu and command palette action.
func (m *Model) ToggleServices() { m.DockToggle("services") }

// serviceRows reads the project's services: its build systems, the backend's
// run configurations and the Java tooling settings.
func (m *Model) serviceRows() []serviceRow {
	var rows []serviceRow
	root := m.Root()
	has := func(name string) bool {
		if root == "" {
			return false
		}
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	status := ""
	if m.build.success != nil {
		if *m.build.success {
			status = "last build succeeded"
		} else {
			status = "last build failed"
		}
	}
	if model := m.maven.model; model.Maven != nil && len(model.Maven.Projects) > 0 {
		detail := fmt.Sprintf("%d modules", len(model.Maven.Projects))
		if status != "" {
			detail += " · " + status
		}
		rows = append(rows, serviceRow{section: "Build", label: "Maven", detail: detail,
			run: func(m *Model) { m.BuildProject() }})
	} else if has("pom.xml") {
		rows = append(rows, serviceRow{section: "Build", label: "Maven", detail: status,
			run: func(m *Model) { m.BuildProject() }})
	}
	if model := m.maven.model; len(model.Gradle) > 0 {
		for _, g := range model.Gradle {
			label := g.Dir
			if label == "" {
				label = "Gradle"
			}
			rows = append(rows, serviceRow{section: "Build", label: label, detail: "Gradle " + status,
				run: func(m *Model) { m.BuildProject() }})
		}
	} else if has("build.gradle") || has("build.gradle.kts") {
		rows = append(rows, serviceRow{section: "Build", label: "Gradle", detail: status,
			run: func(m *Model) { m.BuildProject() }})
	}
	if len(rows) == 0 {
		rows = append(rows, serviceRow{section: "Build", label: "No build system found",
			detail: "add a pom.xml or a Gradle build"})
	}
	for i, cfg := range m.run.configs {
		i, cfg := i, cfg
		detail := cfg.Kind
		if cfg.Detected {
			detail += " · detected"
		}
		running := m.run.running && i == m.run.sel
		switch {
		case running:
			detail = "running"
		case m.run.exitCode != nil && i == m.run.sel:
			detail = fmt.Sprintf("exit %d", *m.run.exitCode)
		}
		rows = append(rows, serviceRow{section: "Run", label: cfg.Name, detail: detail,
			run: func(m *Model) {
				m.run.sel = i
				m.Run()
			}})
		if running {
			// The running configuration's process is a row of its own, as
			// OpenIDEA lists a service with its stop control.
			rows = append(rows, serviceRow{section: "Run", label: "Stop " + cfg.Name,
				detail: "running", run: func(m *Model) { m.StopRun() }})
		}
	}
	if len(m.run.configs) == 0 {
		rows = append(rows, serviceRow{section: "Run", label: "Run configurations…", detail: "",
			run: func(m *Model) { m.ShowRun() }})
	}
	rows = append(rows,
		serviceRow{section: "Tools", label: "Java toolchain", detail: "Settings › Languages & Frameworks › Java",
			run: func(m *Model) { m.OpenSettingsPage(settingsPageJava) }},
		serviceRow{section: "Tools", label: "Notifications", detail: "Settings › Notifications",
			run: func(m *Model) { m.OpenSettingsPage(settingsPageNotifications) }},
	)
	return rows
}

// servicesPanel draws the tree with its section headers.
func (m *Model) servicesPanel(c *ui.Context, p *ui.Theme) {
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(6, 8).Gap(1).Children(func() {
				section := ""
				for i, row := range m.serviceRows() {
					row := row
					if row.section != section {
						section = row.section
						ui.Text(c, section).FontSize(10.5).Bold().TextColor(p.TextMuted).
							Padding(8, 4).SingleLine()
					}
					r := ui.Row(c.Key("service:"+section+":"+row.label)).Height(22).Padding(16, 6).
						Gap(8).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if r.Hovered() {
						r.Background(p.SurfaceHover)
					}
					r.Children(func() {
						ui.Text(c, row.label).FontSize(12).TextColor(p.Text).SingleLine()
						ui.Spacer(c)
						if row.detail != "" {
							ui.Text(c, row.detail).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
						}
						_ = i
					})
					if row.run != nil && r.Clicked() {
						row.run(m)
					}
				}
			})
		})
	})
	_ = col
}
