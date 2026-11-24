package app

// Dependency Analyzer's native MyGo interface. It mirrors OpenIDEA's
// DependencyAnalyzer: module and configuration selection, search, conflicts,
// list/tree modes, usage paths, Exclude, Jump to Source and Copy Coordinates.

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (m *Model) buildDependencyAnalyzerDialog(c *ui.Context, p *ui.Theme) {
	a := m.maven.analyzer
	if a == nil {
		return
	}
	open := a.open
	dialog := ui.Modal(c, &open, func() {
		ui.Column(c).Width(1040).Height(680).Gap(8).Children(func() {
			m.dependencyAnalyzerHeader(c, p, a)
			m.dependencyAnalyzerToolbar(c, p, a)
			m.dependencyAnalyzerBanner(c, p, a)
			if a.readyForTree() {
				m.dependencyAnalyzerBody(c, p, a)
			} else {
				ui.Text(c, "No dependency data").FontSize(13).TextColor(p.TextMuted).Grow(1)
			}
		})
	})
	if dialog.Dismissed() || !open {
		m.CloseBuildDependencyAnalyzer()
	}
}

func (a *buildDependencyAnalyzer) readyForTree() bool {
	if a == nil {
		return false
	}
	if strings.HasPrefix(a.module, "gradle:") {
		return len(a.deps) > 0 || a.loading
	}
	return true
}

func (m *Model) dependencyAnalyzerHeader(c *ui.Context, p *ui.Theme, a *buildDependencyAnalyzer) {
	oi := oiPaletteOf(p)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		oiIconEl(c, "build/dependencyAnalyzer", 18, oi, oi.textMuted)
		ui.Text(c, "Dependency Analyzer").FontSize(14).Bold().TextColor(p.Text).Grow(1).SingleLine()
		ui.Button(c, "Close").OnClick(m.CloseBuildDependencyAnalyzer)
	})
}

func (m *Model) dependencyAnalyzerToolbar(c *ui.Context, p *ui.Theme, a *buildDependencyAnalyzer) {
	modules := m.analyzerModules()
	labels := make([]string, 0, len(modules))
	byLabel := make(map[string]analyzerModule, len(modules))
	selectedLabel := ""
	module, moduleOK := m.analyzerCurrentModule()
	for _, candidate := range modules {
		labels = append(labels, candidate.label)
		byLabel[candidate.label] = candidate
		if candidate.key == a.module {
			selectedLabel = candidate.label
		}
	}
	if selectedLabel == "" && len(labels) > 0 {
		selectedLabel = labels[0]
	}

	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if len(labels) > 0 {
			if selectEl := ui.Select(c, &selectedLabel, labels); selectEl.Changed() {
				if selected, ok := byLabel[selectedLabel]; ok {
					m.chooseDependencyAnalyzerModule(selected.key)
				}
			}
		}
		in := ui.SearchField(c, &a.query).Width(230).Placeholder("Search dependencies")
		in.OnChange(func() {
			a.selected = ""
			m.invalidate()
		})
		conflicts := a.conflicts
		if ui.Checkbox(c, &conflicts, "Show conflicts only").Changed() {
			a.conflicts = conflicts
			a.selected = ""
			m.invalidate()
		}
		if moduleOK && module.tool == "gradle" {
			configuration := a.config
			options := append([]string(nil), gradleDependencyConfigurations...)
			if selectEl := ui.Select(c, &configuration, options); selectEl.Changed() {
				m.chooseDependencyAnalyzerConfiguration(configuration)
			}
		} else {
			scope := a.scope
			if scope == "" {
				scope = "*"
			}
			options := append([]string{"*"}, dependencyScopesOf(analyzeMavenDependencies(m.dependencyAnalyzerDependencies()))...)
			if selectEl := ui.Select(c, &scope, options); selectEl.Changed() {
				a.scope = scope
				a.selected = ""
				m.invalidate()
			}
		}
		m.mavenSeparator(c, p)
		m.analyzerModeButton(c, p, a, "list", "List")
		m.analyzerModeButton(c, p, a, "tree", "Tree")
		m.mavenSeparator(c, p)
		if moduleOK && module.tool == "gradle" {
			if a.loading {
				m.mavenAction(c, p, "buildTool:analyzer:refresh", "run/stop", "Stop", func() {
					a.loading = false
					a.gen++
					m.cancelDependencyAnalyzer(a)
					m.invalidate()
				})
			} else {
				m.mavenAction(c, p, "buildTool:analyzer:refresh", "general/refresh", "Refresh", func() {
					m.reloadDependencyAnalyzer()
				})
			}
		} else if m.maven.syncing {
			m.mavenAction(c, p, "buildTool:analyzer:refresh", "run/stop", "Stop", func() {
				m.CancelBuildToolReload(m.maven.syncTool)
			})
		} else {
			m.mavenAction(c, p, "buildTool:analyzer:refresh", "general/refresh", "Reload All Maven Projects", func() {
				m.SyncMaven()
			})
		}
	})
}

func (m *Model) analyzerModeButton(
	c *ui.Context,
	p *ui.Theme,
	a *buildDependencyAnalyzer,
	mode, label string,
) {
	oi := oiPaletteOf(p)
	button := ui.ButtonBase(c.Key("buildTool:analyzer:mode:"+mode)).Height(24).Padding(0, 8).
		Radius(4).Cursor(ui.CursorPointer)
	if a.mode == mode {
		button.Background(oi.selectedInactive)
	}
	button.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(oi.textStrong).SingleLine()
	})
	button.OnClick(func() {
		a.mode = mode
		a.selected = ""
		m.invalidate()
	})
}

func (m *Model) dependencyAnalyzerBanner(c *ui.Context, p *ui.Theme, a *buildDependencyAnalyzer) {
	oi := oiPaletteOf(p)
	module, ok := m.analyzerCurrentModule()
	switch {
	case a.loading:
		ui.Row(c).MinHeight(28).Padding(2, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, "Resolving dependencies…").FontSize(12).TextColor(oi.textSecondary).SingleLine().Grow(1)
		})
	case a.err != "":
		m.buildToolErrorBanner(c, p, "buildTool:analyzer:error", "Cannot resolve dependencies: "+a.err, func() {
			m.reloadDependencyAnalyzer()
		})
	case ok && module.tool == "maven" && m.maven.model.Maven != nil && m.maven.model.Maven.SyncedAt == nil:
		ui.Row(c).MinHeight(28).Padding(2, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, "Dependency trees are read when Maven projects are reloaded").
				FontSize(12).TextColor(oi.textSecondary).SingleLine().Grow(1)
			m.bannerButton(c, p, "buildTool:analyzer:reload", "Reload All Maven Projects", func() {
				m.SyncMaven()
			})
		})
	case ok && module.tool == "maven" && m.maven.model.Maven != nil && m.maven.model.Maven.Stale:
		ui.Row(c).MinHeight(28).Padding(2, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, "Dependency trees may be out of date").FontSize(12).
				TextColor(oi.textSecondary).SingleLine().Grow(1)
			m.bannerButton(c, p, "buildTool:analyzer:stale", "Load Maven Changes", func() {
				m.SyncMaven()
			})
		})
	}
}

func (m *Model) dependencyAnalyzerBody(c *ui.Context, p *ui.Theme, a *buildDependencyAnalyzer) {
	oi := oiPaletteOf(p)
	deps := m.dependencyAnalyzerDependencies()
	artifacts := analyzeMavenDependencies(deps)
	conflicted := map[string]bool{}
	for _, artifact := range artifacts {
		if artifact.conflict {
			conflicted[artifact.key] = true
		}
	}
	ui.Row(c).Grow(1).MinHeight(0).Gap(0).Children(func() {
		ui.Column(c).Width(570).Height(580).MinHeight(0).
			BorderWidth(0, 0, 0, 1).BorderColor(oi.border).Children(func() {
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(2, 4).Children(func() {
					if a.mode == "tree" {
						tree := dependencyTree(deps, a.query, a.conflicts, a.scope, conflicted)
						m.analyzerTreeRows(c, p, a, artifacts, conflicted, tree, 0)
					} else {
						rows := filterAnalyzedArtifacts(artifacts, a.query, a.conflicts, a.scope)
						for _, artifact := range rows {
							m.analyzerArtifactRow(c, p, a, artifact)
						}
						if len(rows) == 0 {
							ui.Text(c, "No dependencies match the filter").FontSize(12).
								TextColor(oi.textSubtle).Padding(10, 8)
						}
					}
				})
			})
		})
		ui.Column(c).Grow(1).Height(580).MinHeight(0).Children(func() {
			selected := a.selectedArtifact(artifacts)
			if selected == nil {
				ui.Text(c, "Select a dependency to see its usages").FontSize(12).
					TextColor(oi.textSubtle).Padding(12, 10)
				return
			}
			m.analyzerUsages(c, p, a, *selected)
		})
	})
}

func (m *Model) analyzerArtifactRow(
	c *ui.Context,
	p *ui.Theme,
	a *buildDependencyAnalyzer,
	artifact analyzedArtifact,
) {
	usage := dependencyUsage{}
	if len(artifact.usages) > 0 {
		usage = artifact.usages[0]
		for _, candidate := range artifact.usages {
			if len(candidate.path) == 1 {
				usage = candidate
				break
			}
		}
	}
	m.analyzerDependencyRow(c, p, a, artifact.key, artifact, usage)
}

func (m *Model) analyzerDependencyRow(
	c *ui.Context,
	p *ui.Theme,
	a *buildDependencyAnalyzer,
	key string,
	artifact analyzedArtifact,
	usage dependencyUsage,
) {
	oi := oiPaletteOf(p)
	row := ui.Row(c.Key("analyzer:"+key)).Height(25).Padding(6, 6).Gap(6).
		Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if a.selected == artifact.key {
		row.Background(oi.selectedInactive)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Children(func() {
		icon := "nodes/libraryFolder"
		if artifact.conflict {
			icon = "status/warning"
		} else if usage.dependency.GradleBuild {
			icon = "maven/mavenModule"
		}
		oiIconEl(c, icon, 16, oi, oi.textMuted)
		ui.Text(c, artifact.key).FontSize(12).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
		if artifact.version != "" {
			ui.Text(c, artifact.version).FontSize(11).TextColor(oi.textSubtle).SingleLine()
		}
		if artifact.conflict {
			var losing []string
			for _, version := range artifact.requested {
				if version != artifact.version {
					losing = append(losing, version)
				}
			}
			if len(losing) > 0 {
				ui.Text(c, strings.Join(losing, ", ")).FontSize(11).TextColor(oi.warning).SingleLine()
			}
		}
		if artifact.scope != "" && artifact.scope != "compile" {
			ui.Text(c, "("+artifact.scope+")").FontSize(11).TextColor(oi.textSubtle)
		}
	})
	row.OnClick(func() {
		a.selected = artifact.key
		m.invalidate()
	})
	if row.DoubleClicked() {
		m.analyzerJumpToSource(usage)
	}
	row.ContextMenu(func(menu *ui.Menu) {
		a.selected = artifact.key
		m.analyzerUsageMenu(menu, usage)
	})
}

func (m *Model) analyzerTreeRows(
	c *ui.Context,
	p *ui.Theme,
	a *buildDependencyAnalyzer,
	artifacts []analyzedArtifact,
	conflicted map[string]bool,
	nodes []dependencyTreeNode,
	depth int,
) {
	oi := oiPaletteOf(p)
	narrow := a.conflicts || strings.TrimSpace(a.query) != "" || (a.scope != "" && a.scope != "*")
	for _, node := range nodes {
		artifactKey := dependencyArtifactKey(node.dependency)
		var artifact analyzedArtifact
		for _, candidate := range artifacts {
			if candidate.key == artifactKey {
				artifact = candidate
				break
			}
		}
		usage := dependencyUsage{path: node.path, dependency: node.dependency}
		row := ui.Row(c.Key("analyzer:tree:" + node.key)).
			Height(25).PaddingStart(4 + float32(depth)*16).PaddingEnd(6).Gap(5).
			Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if a.selected == artifact.key {
			row.Background(oi.selectedInactive)
		} else if row.Hovered() {
			row.Background(oi.hoverTint)
		}
		row.Children(func() {
			chevron := ui.Row(c).Size(16, 16).Shrink(0).Justify(ui.Center).AlignItems(ui.Center)
			if len(node.children) > 0 {
				glyph := "general/chevronRight"
				if narrow || a.expanded[node.key] {
					glyph = "general/chevronDown"
				}
				chevron.Children(func() { oiIconEl(c, glyph, 16, oi, oi.textMuted) })
				chevron.OnClick(func() {
					a.expanded[node.key] = !a.expanded[node.key]
					m.invalidate()
				})
			}
			icon := "nodes/libraryFolder"
			if conflicted[artifactKey] {
				icon = "status/warning"
			}
			oiIconEl(c, icon, 16, oi, oi.textMuted)
			ui.Text(c, artifactKey).FontSize(12).TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			if node.dependency.Version != "" {
				ui.Text(c, node.dependency.Version).FontSize(11).TextColor(oi.textSubtle).SingleLine()
			}
			if node.dependency.Scope != "" && node.dependency.Scope != "compile" {
				ui.Text(c, "("+node.dependency.Scope+")").FontSize(11).TextColor(oi.textSubtle)
			}
			status := dependencyUsageStatus(node.dependency)
			if status != "resolved" {
				ui.Text(c, status).FontSize(11).TextColor(oi.warning).SingleLine().Ellipsis("…")
			}
		})
		row.OnClick(func() {
			a.selected = artifact.key
			m.invalidate()
		})
		if row.DoubleClicked() {
			m.analyzerJumpToSource(usage)
		}
		row.ContextMenu(func(menu *ui.Menu) {
			a.selected = artifact.key
			m.analyzerUsageMenu(menu, usage)
		})
		if len(node.children) > 0 && (narrow || a.expanded[node.key]) {
			m.analyzerTreeRows(c, p, a, artifacts, conflicted, node.children, depth+1)
		}
	}
}

func (m *Model) analyzerUsageMenu(menu *ui.Menu, usage dependencyUsage) {
	if menu.Item("Quick Documentation").Chosen() {
		m.showAnalyzerDependencyDocumentation(usage)
	}
	module, ok := m.analyzerCurrentModule()
	if ok && module.tool == "maven" && len(usage.path) > 0 {
		if menu.Item("Jump to Source").Chosen() {
			m.analyzerJumpToSource(usage)
		}
		if _, canExclude := dependencyExclusionFor(usage); canExclude && menu.Item("Exclude").Chosen() {
			m.analyzerExclude(usage)
		}
	}
	if menu.Item("Copy Coordinates").Chosen() {
		buildToolClipboardWrite(mavenDependencyCoordinates(usage.dependency))
	}
}

func (m *Model) analyzerUsages(
	c *ui.Context,
	p *ui.Theme,
	a *buildDependencyAnalyzer,
	artifact analyzedArtifact,
) {
	oi := oiPaletteOf(p)
	heading := mavenDependencyCoordinates(mavenDependency{
		GroupID: artifact.groupID, ArtifactID: artifact.artifact,
		Classifier: stringPtr(artifact.classifier), Version: artifact.version,
	})
	if heading == "" {
		heading = artifact.key
	}
	ui.Row(c).Height(34).Padding(8, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, heading).FontSize(13).Bold().TextColor(oi.textStrong).Grow(1).SingleLine().Ellipsis("…")
		m.mavenAction(c, p, "analyzer:copy:"+artifact.key, "general/copy", "Copy Coordinates", func() {
			buildToolClipboardWrite(heading)
		})
	})
	if artifact.conflict {
		ui.Row(c).Padding(8, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			oiIconEl(c, "status/warning", 16, oi, oi.warning)
			ui.Text(c, "Versions requested: "+strings.Join(artifact.requested, ", ")).
				FontSize(11).TextColor(oi.warning).SingleLine().Ellipsis("…")
		})
	}
	module, moduleOK := m.analyzerCurrentModule()
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(4, 6).Gap(8).Children(func() {
			for index, usage := range sortedDependencyUsages(artifact.usages) {
				m.analyzerUsageBlock(c, p, module, moduleOK, index, usage)
			}
		})
	})
}

func (m *Model) analyzerUsageBlock(
	c *ui.Context,
	p *ui.Theme,
	module analyzerModule,
	moduleOK bool,
	index int,
	usage dependencyUsage,
) {
	oi := oiPaletteOf(p)
	ui.Column(c).Key(fmt.Sprintf("analyzer:usage:%d:%s", index, usage.dependency.Version)).
		Gap(4).Padding(7, 8).Radius(5).Background(oi.island).
		Border(1, oi.border).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			status := dependencyUsageStatus(usage.dependency)
			icon := "status/info"
			tone := oi.textMuted
			if status == "resolved" {
				icon, tone = "status/success", oi.success
			} else if strings.Contains(status, "conflict") || status == "not resolved" {
				icon, tone = "status/warning", oi.warning
			}
			oiIconEl(c, icon, 15, oi, tone)
			ui.Text(c, usage.dependency.Version).FontSize(12).Bold().TextColor(oi.textStrong)
			ui.Text(c, status).FontSize(11).TextColor(tone).Grow(1).SingleLine().Ellipsis("…")
			if moduleOK && module.tool == "maven" {
				if _, canExclude := dependencyExclusionFor(usage); canExclude {
					if ui.Button(c, "Exclude").Clicked() {
						m.analyzerExclude(usage)
					}
				}
				m.mavenAction(c, p, fmt.Sprintf("analyzer:jump:%d", index), "autoscroll/autoscrollToSource",
					"Jump to Source", func() { m.analyzerJumpToSource(usage) })
			}
		})
		ui.Text(c, module.name).FontSize(11).TextColor(oi.textSubtle).SingleLine()
		for depth, dependency := range usage.path {
			line := mavenDependencyCoordinates(dependency)
			ui.Text(c, strings.Repeat("  ", depth)+"↳ "+line).
				FontSize(11).TextColor(oi.text).SingleLine().Ellipsis("…")
		}
	})
}
