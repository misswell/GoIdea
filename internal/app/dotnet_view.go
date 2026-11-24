package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

func init() {
	registerToolWindow(&toolWindowSpec{
		id: "dotnet", title: ".NET", icon: "fileTypes/Csharp",
		anchor: dockRight, order: 40, stripe: false,
		open:  func(m *Model) { m.ensureDotnet() },
		shown: func(m *Model) bool { return m.dotnet.root != "" },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.dotnetPanel(c, p)
		},
	})
}

func (m *Model) dotnetPanel(c *ui.Context, p *ui.Theme) {
	m.ensureDotnet()
	s := &m.dotnet
	oi := oiPaletteOf(p)
	ui.Column(c.Key("dotnet:panel")).Grow(1).MinWidth(0).MinHeight(0).
		Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		m.dotnetToolbar(c, p, oi)
		if s.error != "" {
			ui.Row(c.Key("dotnet:error")).Shrink(0).Padding(7, 10).Gap(8).
				AlignItems(ui.Center).Background(oi.danger.Mix(p.Surface, 0.9)).
				Children(func() {
					oiIconEl(c, "status/error", 16, oi, oi.danger)
					ui.Text(c, s.error).FontSize(12).TextColor(oi.danger).
						Grow(1).SingleLine().Ellipsis("…")
				})
		}
		ui.Scroll(c.Key("dotnet:body")).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(8).Gap(9).Children(func() {
				m.dotnetDiscoveryErrors(c, p, oi)
				m.dotnetSolutions(c, p, oi)
				m.dotnetProjectChooser(c, p, oi)
				project := m.dotnetSelectedProject()
				if project == nil {
					m.dotnetEmptyState(c, p, oi)
					return
				}
				m.dotnetConfigurationRow(c, p, oi, project)
				m.dotnetBuildRow(c, p, oi)
				m.dotnetReferences(c, p, oi, project)
				m.dotnetPackages(c, p, oi)
				if project.Test {
					m.dotnetTests(c, p, oi, project)
				}
				m.dotnetOutput(c, p, oi)
			})
		})
	})
}

func (m *Model) dotnetToolbar(c *ui.Context, p *ui.Theme, oi oiColors) {
	s := &m.dotnet
	ui.Row(c.Key("dotnet:toolbar")).Height(32).Shrink(0).Padding(0, 6).Gap(5).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			reload := ui.Button(c, "Reload .NET Workspace")
			reload.Disabled(s.busy)
			if reload.Clicked() {
				m.ReloadDotnet()
			}
			if s.busy {
				if ui.Button(c, "Cancel").Clicked() {
					m.CancelDotnet()
				}
			}
			ui.Spacer(c)
			if s.busy {
				ui.Text(c, "Working…").FontSize(12).TextColor(oi.textMuted).SingleLine()
			}
		})
}

func (m *Model) dotnetDiscoveryErrors(c *ui.Context, p *ui.Theme, oi oiColors) {
	for index, message := range m.dotnet.model.Errors {
		ui.Row(c.Key(fmt.Sprintf("dotnet:discovery-error:%d", index))).
			Padding(6, 8).Gap(6).AlignItems(ui.Center).
			Background(oi.danger.Mix(p.Surface, 0.92)).Radius(5).Children(func() {
			oiIconEl(c, "status/warning", 14, oi, oi.danger)
			ui.Text(c, "Project evaluation failed").FontSize(12).Bold().
				TextColor(oi.text).SingleLine()
			ui.Text(c, message).FontSize(11).TextColor(oi.textMuted).
				Grow(1).SingleLine().Ellipsis("…").Tooltip(message)
		})
	}
}

func (m *Model) dotnetSolutions(c *ui.Context, p *ui.Theme, oi oiColors) {
	if len(m.dotnet.model.Solutions) == 0 {
		return
	}
	ui.Column(c.Key("dotnet:solutions")).Gap(2).Children(func() {
		ui.Text(c, "Solutions").FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
		for _, solution := range m.dotnet.model.Solutions {
			solution := solution
			expanded := m.dotnet.expanded[solution.Path]
			row := ui.Row(c.Key("dotnet:solution:"+solution.Path)).
				Height(25).Padding(0, 5).Gap(5).AlignItems(ui.Center).Radius(4).
				Cursor(ui.CursorPointer)
			if row.Hovered() {
				row.Background(oi.hoverTint)
			}
			row.OnClick(func() {
				m.dotnet.expanded[solution.Path] = !expanded
				m.invalidate()
			})
			row.Tooltip(solution.Path)
			row.Children(func() {
				if expanded {
					ui.Text(c, "▾").FontSize(11).TextColor(oi.textMuted)
				} else {
					ui.Text(c, "▸").FontSize(11).TextColor(oi.textMuted)
				}
				fileIconEl(c, oi, solution.Path, 14, oi.icon)
				ui.Text(c, filepath.Base(solution.Path)).FontSize(12).
					TextColor(oi.text).SingleLine().Ellipsis("…")
			})
			if !expanded {
				continue
			}
			ui.Column(c.Key("dotnet:solution-projects:"+solution.Path)).
				Padding(0, 0, 0, 18).Gap(1).Children(func() {
				for _, projectPath := range solution.Projects {
					projectPath := projectPath
					button := ui.ButtonBase(c.Key("dotnet:solution-project:"+projectPath)).
						Height(25).Padding(0, 6).Radius(4).AlignItems(ui.Center).
						OnClick(func() { m.SelectDotnet(projectPath, "", "") })
					button.Disabled(m.dotnet.busy)
					button.Tooltip(projectPath)
					if filepath.Clean(projectPath) == filepath.Clean(m.dotnet.selected) {
						button.Background(oi.selected)
					} else if button.Hovered() {
						button.Background(oi.hoverTint)
					}
					button.Children(func() {
						fileIconEl(c, oi, projectPath, 14, oi.icon)
						ui.Text(c, m.dotnetRelativePath(projectPath)).FontSize(12).
							TextColor(oi.text).SingleLine().Ellipsis("…")
					})
				}
			})
		}
	})
}

func (m *Model) dotnetProjectChooser(c *ui.Context, p *ui.Theme, oi oiColors) {
	if len(m.dotnet.model.Projects) == 0 {
		return
	}
	labels := make([]string, 0, len(m.dotnet.model.Projects))
	for _, project := range m.dotnet.model.Projects {
		labels = append(labels, m.dotnetRelativePath(project.Path))
	}
	current := m.dotnetRelativePath(m.dotnet.selected)
	ui.Row(c.Key("dotnet:project-row")).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, ".NET project").FontSize(12).TextColor(p.TextMuted).
			Width(104).SingleLine()
		selectBox := ui.Select(c.Key("dotnet:project"), &current, labels).Grow(1)
		selectBox.Disabled(m.dotnet.busy)
		if selectBox.Changed() {
			for index, label := range labels {
				if label == current {
					m.SelectDotnet(m.dotnet.model.Projects[index].Path, "", "")
					break
				}
			}
		}
	})
}

func (m *Model) dotnetConfigurationRow(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	project *dotnetProject,
) {
	configurations := append([]string(nil), project.Configurations...)
	if m.dotnet.configuration != "" &&
		!dotnetContains(configurations, m.dotnet.configuration) {
		configurations = append([]string{m.dotnet.configuration}, configurations...)
	}
	frameworks := append([]string(nil), project.Frameworks...)
	if m.dotnet.framework != "" &&
		!dotnetContains(frameworks, m.dotnet.framework) {
		frameworks = append([]string{m.dotnet.framework}, frameworks...)
	}
	ui.Row(c.Key("dotnet:configuration-row")).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Width(200).Gap(3).Children(func() {
			ui.Text(c, "Configuration").FontSize(11).TextColor(oi.textMuted).SingleLine()
			current := m.dotnet.configuration
			selectBox := ui.Select(c.Key("dotnet:configuration"), &current, configurations).
				Width(190)
			selectBox.Disabled(m.dotnet.busy)
			if selectBox.Changed() {
				m.SelectDotnet(project.Path, current, m.dotnet.framework)
			}
		})
		ui.Column(c).Grow(1).MinWidth(120).Gap(3).Children(func() {
			ui.Text(c, "Target framework").FontSize(11).TextColor(oi.textMuted).SingleLine()
			current := m.dotnet.framework
			selectBox := ui.Select(c.Key("dotnet:framework"), &current, frameworks).Grow(1)
			selectBox.Disabled(m.dotnet.busy)
			if selectBox.Changed() {
				m.SelectDotnet(project.Path, m.dotnet.configuration, current)
			}
		})
	})
}

func (m *Model) dotnetBuildRow(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Row(c.Key("dotnet:build-actions")).Gap(6).AlignItems(ui.Center).Children(func() {
		for _, operation := range []struct {
			id, label, action string
		}{
			{"build", "Build", "dotnet.build"},
			{"restore", "Restore", "dotnet.restore"},
			{"clean", "Clean", "dotnet.clean"},
		} {
			button := ui.Button(c.Key("dotnet:"+operation.id), operation.label)
			button.Disabled(m.dotnet.busy)
			if button.Clicked() {
				m.RunAction(operation.action)
			}
		}
		ui.Spacer(c)
		if project := m.dotnetSelectedProject(); project != nil {
			ui.Text(c, project.Name).FontSize(11).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…").Tooltip(project.Path)
		}
	})
}

func (m *Model) dotnetReferences(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	project *dotnetProject,
) {
	if len(project.References) == 0 {
		return
	}
	ui.Column(c.Key("dotnet:references")).Gap(2).Children(func() {
		ui.Text(c, "Project references").FontSize(12).Bold().
			TextColor(oi.textStrong).SingleLine()
		for _, reference := range project.References {
			reference := reference
			available := dotnetProjectIndex(m.dotnet.model.Projects, reference) >= 0
			button := ui.ButtonBase(c.Key("dotnet:reference:"+reference)).
				Height(25).Padding(0, 6).Gap(5).Radius(4).AlignItems(ui.Center).
				OnClick(func() { m.SelectDotnet(reference, "", "") })
			button.Disabled(m.dotnet.busy || !available)
			button.Tooltip(reference)
			if button.Hovered() {
				button.Background(oi.hoverTint)
			}
			button.Children(func() {
				fileIconEl(c, oi, reference, 14, oi.icon)
				ui.Text(c, m.dotnetRelativePath(reference)).FontSize(12).
					TextColor(oi.text).SingleLine().Ellipsis("…")
			})
		}
	})
}

func (m *Model) dotnetPackages(c *ui.Context, p *ui.Theme, oi oiColors) {
	ui.Column(c.Key("dotnet:packages")).Gap(5).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "NuGet packages").FontSize(12).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Spacer(c)
			refresh := ui.Button(c, "Refresh packages")
			refresh.Disabled(m.dotnet.busy)
			if refresh.Clicked() {
				m.RunAction("dotnet.packages")
			}
		})
		ui.Row(c.Key("dotnet:search-row")).Gap(6).AlignItems(ui.Center).Children(func() {
			input := ui.TextInput(c.Key("dotnet:search"), &m.dotnet.query).
				Grow(1).Placeholder("Search NuGet packages").SingleLine()
			input.Disabled(m.dotnet.busy)
			search := ui.Button(c, "Search")
			search.Disabled(m.dotnet.busy || strings.TrimSpace(m.dotnet.query) == "")
			if search.Clicked() {
				m.RunActionWithArg("dotnet.search", map[string]any{
					"query": m.dotnet.query,
				})
			}
		})
		m.dotnetFoundRows(c, p, oi)
		m.dotnetInstalledRows(c, p, oi)
	})
}

func (m *Model) dotnetFoundRows(c *ui.Context, p *ui.Theme, oi oiColors) {
	if len(m.dotnet.found) == 0 {
		return
	}
	versions := map[string][]string{}
	order := make([]string, 0)
	for _, pkg := range m.dotnet.found {
		if _, ok := versions[pkg.ID]; !ok {
			order = append(order, pkg.ID)
		}
		if pkg.Version != "" && !dotnetContains(versions[pkg.ID], pkg.Version) {
			versions[pkg.ID] = append(versions[pkg.ID], pkg.Version)
		}
	}
	ui.Column(c.Key("dotnet:found")).Gap(2).Children(func() {
		for _, id := range order {
			id := id
			available := versions[id]
			if len(available) == 0 {
				available = []string{""}
			}
			selected := m.dotnet.versions[id]
			if !dotnetContains(available, selected) {
				selected = available[0]
			}
			ui.Row(c.Key("dotnet:found:" + id)).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, id).FontSize(12).TextColor(oi.text).Grow(1).
					SingleLine().Ellipsis("…").Tooltip(id)
				version := selected
				versionSelect := ui.Select(c.Key("dotnet:found-version:"+id), &version, available).
					Width(132)
				versionSelect.Disabled(m.dotnet.busy)
				if versionSelect.Changed() {
					if m.dotnet.versions == nil {
						m.dotnet.versions = map[string]string{}
					}
					m.dotnet.versions[id] = version
				}
				install := ui.Button(c, "Install")
				install.Disabled(m.dotnet.busy)
				if install.Clicked() {
					value := version
					m.RunActionWithArg("dotnet.packageChange", map[string]any{
						"id": id, "version": value,
					})
				}
				more := ui.Button(c, "Versions")
				more.Disabled(m.dotnet.busy)
				if more.Clicked() {
					m.RunActionWithArg("dotnet.search", map[string]any{
						"query": id, "exact": true,
					})
				}
			})
		}
	})
}

func (m *Model) dotnetInstalledRows(c *ui.Context, p *ui.Theme, oi oiColors) {
	if len(m.dotnet.packages) == 0 {
		return
	}
	ui.Column(c.Key("dotnet:installed")).Gap(2).Children(func() {
		for _, pkg := range m.dotnet.packages {
			pkg := pkg
			ui.Row(c.Key("dotnet:installed:"+pkg.ID+":"+pkg.Version)).
				Padding(0, 4).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, pkg.ID).FontSize(12).TextColor(oi.text).Grow(1).
					SingleLine().Ellipsis("…").Tooltip(pkg.ID)
				ui.Text(c, pkg.Version).FontSize(11).TextColor(oi.textMuted).
					SingleLine().Width(100).Ellipsis("…")
				if pkg.Transitive {
					ui.Text(c, "Transitive").FontSize(10).TextColor(oi.textMuted).
						SingleLine()
					return
				}
				versions := ui.Button(c, "Versions")
				versions.Disabled(m.dotnet.busy)
				if versions.Clicked() {
					m.RunActionWithArg("dotnet.search", map[string]any{
						"query": pkg.ID, "exact": true,
					})
				}
				remove := ui.Button(c, "Remove")
				remove.Disabled(m.dotnet.busy)
				if remove.Clicked() {
					m.RunActionWithArg("dotnet.packageChange", map[string]any{
						"id": pkg.ID, "version": nil,
					})
				}
			})
		}
	})
}

func (m *Model) dotnetTests(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	project *dotnetProject,
) {
	ui.Column(c.Key("dotnet:tests")).Gap(4).Children(func() {
		ui.Text(c, "Tests").FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			discover := ui.Button(c, "Discover .NET Tests")
			discover.Disabled(m.dotnet.busy)
			if discover.Clicked() {
				m.RunAction("dotnet.discover")
			}
			run := ui.Button(c, "Run all tests")
			run.Disabled(m.dotnet.busy)
			if run.Clicked() {
				m.RunAction("dotnet.test")
			}
			ui.Text(c, string(project.Runner)).FontSize(10).TextColor(oi.textMuted).
				SingleLine()
		})
		if project.Runner != dotnetRunnerVSTest && !project.TRX {
			ui.Row(c.Key("dotnet:trx-warning")).Padding(6, 8).Gap(6).
				AlignItems(ui.Center).Radius(5).
				Background(oi.warning.Mix(p.Surface, 0.92)).Children(func() {
				oiIconEl(c, "status/warning", 14, oi, oi.warning)
				ui.Text(c, "Install the TRX reporting extension to show MTP results.").
					FontSize(11).TextColor(oi.text).Grow(1).SingleLine()
				install := ui.Button(c, "Install TRX reporting")
				install.Disabled(m.dotnet.busy)
				if install.Clicked() {
					m.RunAction("dotnet.installTrx")
				}
			})
		}
		for _, test := range m.dotnet.tests {
			test := test
			ui.Row(c.Key("dotnet:test:"+test)).Padding(0, 5).Height(24).
				Gap(5).Radius(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, test).FontSize(12).TextColor(oi.text).Grow(1).
					SingleLine().Ellipsis("…").Tooltip(test)
				run := ui.Button(c, "Run")
				run.Disabled(m.dotnet.busy)
				if run.Clicked() {
					m.RunActionWithArg("dotnet.test", test)
				}
			})
		}
	})
}

func (m *Model) dotnetOutput(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.dotnet.output == "" {
		return
	}
	expanded := m.dotnet.expanded["output"]
	ui.Column(c.Key("dotnet:output")).Gap(3).Children(func() {
		row := ui.Row(c.Key("dotnet:output-header")).Height(24).Padding(0, 5).
			Gap(5).AlignItems(ui.Center).Radius(4).Cursor(ui.CursorPointer)
		if row.Hovered() {
			row.Background(oi.hoverTint)
		}
		row.OnClick(func() {
			m.dotnet.expanded["output"] = !expanded
			m.invalidate()
		})
		row.Children(func() {
			if expanded {
				ui.Text(c, "▾").FontSize(11).TextColor(oi.textMuted)
			} else {
				ui.Text(c, "▸").FontSize(11).TextColor(oi.textMuted)
			}
			ui.Text(c, "Output").FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
		})
		if expanded {
			ui.TextArea(c.Key("dotnet:output-text"), &m.dotnet.output).
				Height(180).ReadOnly(true).Font("monospace").FontSize(11)
		}
	})
}

func (m *Model) dotnetEmptyState(c *ui.Context, p *ui.Theme, oi oiColors) {
	if m.dotnet.busy {
		ui.Column(c).Grow(1).Padding(20).Gap(7).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "Discovering .NET projects…").FontSize(13).
				TextColor(oi.textMuted)
		})
		return
	}
	ui.Column(c).Padding(18).Gap(7).AlignItems(ui.Center).Children(func() {
		oiIconEl(c, "fileTypes/Csharp", 30, oi, oi.textMuted)
		ui.Text(c, "No .NET project selected").FontSize(13).Bold().
			TextColor(oi.text)
		ui.Text(c, "Reload the workspace after adding a project or solution.").
			FontSize(12).TextColor(oi.textMuted)
	})
}

func (m *Model) dotnetRelativePath(path string) string {
	root := m.Root()
	if root == "" {
		return path
	}
	relative, err := filepath.Rel(root, path)
	if err == nil && relative != "." &&
		relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}

func dotnetContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
