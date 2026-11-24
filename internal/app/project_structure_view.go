package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	projectStructureWidth       float32 = 1040
	projectStructureMinWidth    float32 = 760
	projectStructureTitleHeight float32 = 40
	projectStructureSideWidth   float32 = 184
	projectStructureRowHeight   float32 = 24
	projectStructureFooter      float32 = 52
)

func (m *Model) projectStructureDialog(c *ui.Context, p *ui.Theme) {
	state := &m.projectStructure
	if !state.open {
		return
	}
	open := state.open
	dlg := ui.Modal(c, &open, func() {
		m.projectStructureSheet(c, p)
	})
	if dlg.Dismissed() || !open {
		m.CloseProjectStructure()
	}
}

func (m *Model) projectStructureSheet(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	width := projectStructureWidth
	if m.editorWidth > 0 {
		width = min(width, max(projectStructureMinWidth, m.editorWidth-48))
	}
	height := m.projectStructureHeight()
	ui.Column(c).Width(width).Height(height).
		Background(oi.elev).Radius(6).Children(func() {
		ui.Row(c).Height(projectStructureTitleHeight).Shrink(0).Padding(0, 16).
			AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Project Structure").FontSize(13).Bold().
				TextColor(oi.textStrong).SingleLine()
		})
		ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			m.projectStructureSidebar(c, oi)
			ui.Row(c).Width(1).Shrink(0).Background(oi.borderStrong)
			m.projectStructurePage(c, p, oi)
		})
		m.projectStructureFooter(c, oi)
	})
}

func (m *Model) projectStructureHeight() float32 {
	h := m.editorHeight - 48
	if h <= 0 || h > 720 {
		h = 720
	}
	if h < 480 {
		h = 480
	}
	return h
}

func (m *Model) projectStructureSidebar(c *ui.Context, oi oiColors) {
	ui.Column(c).Width(projectStructureSideWidth).Shrink(0).MinHeight(0).
		Padding(8, 6).Children(func() {
		ui.Text(c, "Project Settings").FontSize(11).Bold().TextColor(oi.textMuted).
			Padding(0, 8, 4, 8).SingleLine()
		for _, item := range projectStructureSections[:6] {
			m.projectStructureNavRow(c, oi, item.ID, item.Title)
		}
		ui.Row(c).Height(12)
		ui.Text(c, "Platform Settings").FontSize(11).Bold().TextColor(oi.textMuted).
			Padding(0, 8, 4, 8).SingleLine()
		m.projectStructureNavRow(c, oi, projectStructureSDKs, "SDKs")
	})
}

func (m *Model) projectStructureNavRow(c *ui.Context, oi oiColors, id, title string) {
	selected := m.projectStructure.section == id
	row := ui.Row(c).Key("project-structure-nav:"+id).Height(26).Padding(8, 8).
		Gap(7).AlignItems(ui.Center)
	if selected {
		row.Background(oi.selected)
	} else if row.Hovered() {
		row.Background(oi.hover)
	}
	row.Children(func() {
		glyph := "▫"
		switch id {
		case projectStructureProject:
			glyph = "▰"
		case projectStructureModules:
			glyph = "▣"
		case projectStructureDependencies:
			glyph = "⌘"
		case projectStructureLibraries:
			glyph = "▤"
		case projectStructureFacets:
			glyph = "◈"
		case projectStructureArtifacts:
			glyph = "▥"
		case projectStructureSDKs:
			glyph = "⚙"
		}
		ui.Text(c, glyph).Width(14).FontSize(11).
			TextColor(oi.textMuted).SingleLine()
		ui.Text(c, title).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
	})
	if row.Clicked() {
		m.SetProjectStructureSection(id)
	}
}

func (m *Model) projectStructurePage(c *ui.Context, p *ui.Theme, oi oiColors) {
	state := &m.projectStructure
	title := "Project"
	for _, item := range projectStructureSections {
		if item.ID == state.section {
			title = item.Title
			break
		}
	}
	group := "Project Settings"
	if state.section == projectStructureSDKs {
		group = "Platform Settings"
	}
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(38).Shrink(0).Padding(0, 14, 0, 18).
			Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, group).FontSize(12).TextColor(oi.textMuted).SingleLine()
			ui.Text(c, "›").FontSize(11).TextColor(oi.textSubtle).SingleLine()
			ui.Text(c, title).FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
			ui.Spacer(c)
			if m.projectStructureDirty() {
				ui.Button(c, "Reset").OnClick(func() { m.ResetProjectStructure() })
			}
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(10, 16, 18, 16).MinWidth(0).Children(func() {
				if state.loading && !state.loaded {
					projectStructureNote(c, oi, "Loading project structure…", false)
					return
				}
				if state.err != "" {
					projectStructureNote(c, oi, state.err, true)
				}
				switch state.section {
				case projectStructureModules:
					m.projectStructureModulesPage(c, oi)
				case projectStructureDependencies:
					m.projectStructureDependenciesPage(c, oi)
				case projectStructureLibraries:
					m.projectStructureLibrariesPage(c, oi)
				case projectStructureFacets:
					m.projectStructureFacetsPage(c, oi)
				case projectStructureArtifacts:
					m.projectStructureArtifactsPage(c, oi)
				case projectStructureSDKs:
					m.projectStructureSDKsPage(c, oi)
				default:
					m.projectStructureProjectPage(c, oi)
				}
			})
		})
	})
}

func (m *Model) projectStructureFooter(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	ui.Row(c).Height(projectStructureFooter).Shrink(0).Padding(10, 16).
		Gap(10).AlignItems(ui.Center).Children(func() {
		if state.err != "" && !state.loading {
			ui.Text(c, "Could not save: "+state.err).FontSize(12).
				TextColor(oi.danger).SingleLine().Ellipsis("…").Grow(1)
		} else if state.saving {
			ui.Text(c, "Saving project structure…").FontSize(12).
				TextColor(oi.textSubtle).SingleLine().Grow(1)
		} else {
			ui.Spacer(c)
		}
		ui.Button(c, "Cancel").Disabled(state.saving).
			OnClick(func() { m.CloseProjectStructure() })
		ui.Button(c, "Apply").Disabled(state.saving || !m.projectStructureDirty()).
			OnClick(func() { m.ProjectStructureApply() })
		ui.Button(c, "OK").Disabled(state.saving).
			OnClick(func() { m.ProjectStructureApplyAndClose() })
	})
}

func projectStructureSectionTitle(c *ui.Context, oi oiColors, title string) {
	ui.Text(c, title).FontSize(12).Bold().TextColor(oi.textStrong).
		Padding(0, 0, 7, 0).SingleLine()
}

func projectStructureNote(c *ui.Context, oi oiColors, text string, errorText bool) {
	color := oi.textMuted
	if errorText {
		color = oi.danger
	}
	ui.Row(c).MinHeight(26).Padding(7, 8).Background(oi.island).
		Children(func() {
			ui.Text(c, text).FontSize(11).TextColor(color).Wrap()
		})
	ui.Row(c).Height(8)
}

func projectStructureLabel(c *ui.Context, oi oiColors, text string, width float32) {
	ui.Text(c, text).Width(width).FontSize(12).TextColor(oi.text).
		SingleLine().Ellipsis("…")
}

func (m *Model) projectStructureTextField(
	c *ui.Context,
	oi oiColors,
	label string,
	value *string,
	placeholder string,
	browse func(*Model),
) {
	ui.Row(c).MinHeight(31).Gap(8).AlignItems(ui.Center).Children(func() {
		projectStructureLabel(c, oi, label, 160)
		in := ui.TextInput(c, value).Grow(1).Placeholder(placeholder)
		in.OnChange(func() { m.invalidate() })
		if browse != nil {
			ui.Button(c, "Browse…").OnClick(func() { browse(m) })
		}
	})
}

func projectStructureChoiceRow(
	c *ui.Context,
	oi oiColors,
	label string,
	value *string,
	choices, labels []string,
	disabled bool,
) {
	ui.Row(c).MinHeight(31).Gap(8).AlignItems(ui.Center).Children(func() {
		projectStructureLabel(c, oi, label, 160)
		shown := *value
		for i, choice := range choices {
			if i < len(labels) && choice == *value {
				shown = labels[i]
				break
			}
		}
		sel := ui.Select(c, &shown, labels).Grow(1).Disabled(disabled)
		if sel.Changed() {
			for i, text := range labels {
				if text == shown && i < len(choices) {
					*value = choices[i]
					break
				}
			}
		}
	})
}

func (m *Model) projectStructureProjectPage(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	projectStructureSectionTitle(c, oi, "Project")
	name := filepath.Base(m.Root())
	ui.Row(c).MinHeight(31).Gap(8).AlignItems(ui.Center).Children(func() {
		projectStructureLabel(c, oi, "Project name", 160)
		ui.Text(c, name).Grow(1).FontSize(12).TextColor(oi.textStrong).SingleLine()
	})
	ui.Row(c).MinHeight(18).PaddingStart(168).Children(func() {
		hint := m.Root()
		if hint == "" {
			hint = "No project is currently open."
		}
		ui.Text(c, hint).FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	})
	ui.Row(c).Height(8)
	sdkChoices, sdkLabels := m.projectStructureJDKChoices(state.draft.JDK, "Auto")
	projectStructureChoiceRow(c, oi, "SDK", &state.draft.JDK, sdkChoices, sdkLabels, false)
	levels := append([]string{""}, javaLanguageLevels...)
	levelLabels := append([]string{"SDK default"}, javaLanguageLevels...)
	projectStructureChoiceRow(c, oi, "Language level", &state.draft.LanguageLevel, levels, levelLabels, false)
	m.projectStructureTextField(c, oi, "Compiler output", &state.draft.Structure.CompilerOutput, "out",
		func(m *Model) {
			m.BrowseProjectStructurePath(true, "Select Compiler Output Directory", func(path string) {
				m.SetProjectStructureField("compilerOutput", m.projectStructureStoredPath(path))
			})
		})
	ui.Row(c).Height(14)
	projectStructureSectionTitle(c, oi, "Language Server")
	sdkChoices, sdkLabels = m.projectStructureJDKChoices(state.draft.JDTLSJDK, "Auto")
	projectStructureChoiceRow(c, oi, "JDK", &state.draft.JDTLSJDK, sdkChoices, sdkLabels, false)
	ui.Row(c).MinHeight(20).PaddingStart(168).Children(func() {
		ui.Text(c, "Used to run Eclipse JDT Language Server independently of the project SDK.").
			FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	})
}

func (m *Model) projectStructureJDKChoices(current, emptyLabel string) ([]string, []string) {
	choices := []string{""}
	labels := []string{emptyLabel}
	seen := map[string]bool{"": true}
	for _, sdk := range m.projectStructure.image.SDKs["jdk"] {
		path := strings.TrimSpace(sdk.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		choices = append(choices, path)
		labels = append(labels, javaSDKLabel(sdk))
	}
	current = strings.TrimSpace(current)
	if current != "" && !seen[current] {
		choices = append(choices, current)
		labels = append(labels, "Current · "+current)
	}
	return choices, labels
}

func (m *Model) projectStructureModulesPage(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	modules := m.projectStructureModules()
	m.ensureProjectStructureSelection()
	ui.Row(c).Grow(1).MinHeight(360).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(230).Shrink(0).MinHeight(0).Children(func() {
			ui.Row(c).MinHeight(32).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "Add").OnClick(func() {
					m.BrowseProjectStructurePath(true, "Select Module Directory", func(path string) {
						m.AddProjectStructureModule(path)
					})
				})
				ui.Button(c, "Import…").OnClick(func() { m.ImportMavenModule() })
				remove := ui.Button(c, "−").Disabled(state.moduleKey == "." ||
					state.draft.Structure.Modules[state.moduleKey].JDK == "" &&
						state.draft.Structure.Modules[state.moduleKey].LanguageLevel == "" &&
						state.draft.Structure.Modules[state.moduleKey].CompilerOutput == "" &&
						len(state.draft.Structure.Modules[state.moduleKey].Dependencies) == 0)
				remove.OnClick(func() { m.RemoveProjectStructureModule() })
			})
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Children(func() {
					for _, module := range modules {
						selected := module.Key == state.moduleKey
						row := ui.Row(c).Key("project-structure-module:"+module.Key).
							Height(30).Padding(7, 8).Gap(6).AlignItems(ui.Center)
						if selected {
							row.Background(oi.selected)
						} else if row.Hovered() {
							row.Background(oi.hover)
						}
						row.Children(func() {
							ui.Text(c, "▣").FontSize(12).TextColor(oi.textMuted)
							ui.Text(c, module.Name).Grow(1).FontSize(12).TextColor(oi.text).
								SingleLine().Ellipsis("…")
							ui.Text(c, module.Tool).FontSize(10).TextColor(oi.textSubtle).SingleLine()
						})
						if row.Clicked() {
							m.SetProjectStructureModule(module.Key, state.moduleTab)
						}
					}
				})
			})
		})
		ui.Row(c).Width(1).Background(oi.borderStrong)
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).PaddingStart(14).Children(func() {
			ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "▣").FontSize(12).TextColor(oi.textMuted)
				ui.Text(c, state.moduleKey).FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
				ui.Spacer(c)
				sources := ui.Button(c, "Sources")
				if state.moduleTab == "sources" {
					sources.Background(oi.selected)
				}
				sources.OnClick(func() { m.SetProjectStructureModule(state.moduleKey, "sources") })
				paths := ui.Button(c, "Paths")
				if state.moduleTab == "paths" {
					paths.Background(oi.selected)
				}
				paths.OnClick(func() { m.SetProjectStructureModule(state.moduleKey, "paths") })
			})
			if state.moduleTab == "paths" {
				m.projectStructureModulePaths(c, oi)
			} else {
				m.projectStructureModuleSources(c, oi)
			}
		})
	})
}

func (m *Model) projectStructureModulePaths(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	module := state.draft.Structure.Modules[state.moduleKey]
	ui.Row(c).Height(8)
	projectStructureSectionTitle(c, oi, "Module SDK & Compiler Output")
	choices, labels := m.projectStructureJDKChoices(module.JDK, "Project SDK")
	oldJDK := module.JDK
	projectStructureChoiceRow(c, oi, "Module SDK", &oldJDK, choices, labels, false)
	if oldJDK != module.JDK {
		m.SetProjectStructureModuleField("jdk", oldJDK)
		module = state.draft.Structure.Modules[state.moduleKey]
	}
	levels := append([]string{""}, javaLanguageLevels...)
	levelLabels := append([]string{"Project default"}, javaLanguageLevels...)
	oldLevel := module.LanguageLevel
	projectStructureChoiceRow(c, oi, "Language level", &oldLevel, levels, levelLabels,
		state.moduleKey != ".")
	if oldLevel != module.LanguageLevel {
		m.SetProjectStructureModuleField("languageLevel", oldLevel)
	}
	out := module.CompilerOutput
	ui.Row(c).MinHeight(31).Gap(8).AlignItems(ui.Center).Children(func() {
		projectStructureLabel(c, oi, "Output path", 160)
		in := ui.TextInput(c, &out).Grow(1).Placeholder("target/classes").Disabled(state.moduleKey != ".")
		in.OnChange(func() { m.SetProjectStructureModuleField("compilerOutput", out) })
		browse := ui.Button(c, "Browse…").Disabled(state.moduleKey != ".")
		browse.OnClick(func() {
			m.BrowseProjectStructurePath(true, "Select Compiler Output Directory", func(path string) {
				m.SetProjectStructureModuleField("compilerOutput", m.projectStructureStoredPath(path))
			})
		})
	})
	testOut := module.TestCompilerOutput
	ui.Row(c).MinHeight(31).Gap(8).AlignItems(ui.Center).Children(func() {
		projectStructureLabel(c, oi, "Test output path", 160)
		ui.TextInput(c, &testOut).Grow(1).Placeholder("target/test-classes").Disabled(true)
		ui.Button(c, "Browse…").Disabled(true)
	})
	if state.moduleKey != "." {
		projectStructureNote(c, oi,
			"Per-module language levels and compiler outputs are not supported yet. Configure them in Maven or Gradle.",
			false)
	}
}

func (m *Model) projectStructureModuleSources(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	ui.Row(c).MinHeight(31).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Mark as:").FontSize(11).TextColor(oi.textMuted).SingleLine()
		role := m.projectStructureSelectedFolderRole()
		for _, item := range []struct {
			ID    string
			Label string
		}{
			{"source", "Sources"},
			{"test", "Tests"},
			{"resources", "Resources"},
			{"testResources", "Test Resources"},
			{"excluded", "Excluded"},
		} {
			item := item
			button := ui.Button(c, item.Label).Disabled(state.selectedDir == "")
			if role == item.ID {
				button.Background(oi.selected)
			}
			button.OnClick(func() {
				if role == item.ID {
					m.MarkProjectStructureFolder(m.projectStructureSelectedFolderRel(), "")
				} else {
					m.MarkProjectStructureFolder(m.projectStructureSelectedFolderRel(), item.ID)
				}
			})
		}
	})
	ui.Row(c).Height(6)
	ui.Row(c).Grow(1).MinHeight(210).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			ui.Scroll(c).Grow(1).MinHeight(0).Background(oi.island).Children(func() {
				ui.Column(c).Padding(4, 4).Children(func() {
					for i, row := range m.projectStructureDirectoryRows() {
						m.projectStructureDirectoryRow(c, oi, i, row)
					}
				})
			})
			if state.moduleKey == "." {
				m.projectStructureExcludeFiles(c, oi)
			}
		})
		ui.Row(c).Width(12)
		ui.Scroll(c).Width(255).Children(func() {
			ui.Column(c).Children(func() {
				if state.moduleKey == "." {
					m.projectStructureContentRoots(c, oi)
				}
				m.projectStructureMarkedRoots(c, oi)
			})
		})
	})
}

type projectStructureDirectoryRow struct {
	Path     string
	Name     string
	Rel      string
	Depth    int
	Expanded bool
	Content  bool
}

func (m *Model) projectStructureDirectoryRows() []projectStructureDirectoryRow {
	state := &m.projectStructure
	root := m.Root()
	if root == "" {
		return nil
	}
	var rows []projectStructureDirectoryRow
	var walk func(path, name, rel string, depth int, content bool)
	walk = func(path, name, rel string, depth int, content bool) {
		expanded := state.dirExpanded[path]
		rows = append(rows, projectStructureDirectoryRow{
			Path: path, Name: name, Rel: rel, Depth: depth, Expanded: expanded, Content: content,
		})
		if !expanded {
			return
		}
		for _, child := range state.dirChildren[path] {
			walk(child.Path, child.Name, child.Rel, depth+1, content)
		}
	}
	if state.moduleKey == "." {
		walk(root, filepath.Base(root), "", 0, false)
		for _, content := range state.draft.Structure.ContentRoots {
			walk(filepath.Clean(content), filepath.Base(content), "", 0, true)
		}
	} else {
		path := filepath.Join(root, filepath.FromSlash(state.moduleKey))
		walk(path, filepath.Base(path), state.moduleKey, 0, false)
	}
	return rows
}

func (m *Model) projectStructureDirectoryRow(
	c *ui.Context,
	oi oiColors,
	index int,
	row projectStructureDirectoryRow,
) {
	state := &m.projectStructure
	r := ui.Row(c).Key(fmt.Sprintf("project-structure-dir:%d", index)).
		Height(projectStructureRowHeight).
		PaddingStart(float32(4 + row.Depth*14)).PaddingEnd(4).
		Gap(4).AlignItems(ui.Center)
	if state.selectedDir == row.Path {
		r.Background(oi.selected)
	} else if r.Hovered() {
		r.Background(oi.hover)
	}
	r.Children(func() {
		caret := ui.Row(c).Width(14).Height(projectStructureRowHeight).
			AlignItems(ui.Center).Justify(ui.Center)
		caret.Children(func() {
			glyph := "▸"
			if row.Expanded {
				glyph = "▾"
			}
			ui.Text(c, glyph).FontSize(9).TextColor(oi.textMuted).SingleLine()
		})
		if caret.Clicked() {
			m.ToggleProjectStructureDirectory(row.Path)
		}
		role := m.projectStructureRoleFor(row.Rel)
		glyph := "▱"
		switch role {
		case "source":
			glyph = "S"
		case "test":
			glyph = "T"
		case "resources":
			glyph = "R"
		case "testResources":
			glyph = "TR"
		case "excluded":
			glyph = "×"
		}
		ui.Text(c, glyph).Width(16).FontSize(10).TextColor(oi.textMuted).SingleLine()
		color := oi.text
		if role == "excluded" {
			color = oi.textSubtle
		}
		ui.Text(c, row.Name).FontSize(12).TextColor(color).SingleLine().Ellipsis("…")
		if row.Content {
			ui.Text(c, "content root").FontSize(10).TextColor(oi.textSubtle).SingleLine()
		}
	})
	if r.Clicked() {
		m.SelectProjectStructureDirectory(row.Path)
	}
}

func (m *Model) projectStructureSelectedFolderRel() string {
	state := &m.projectStructure
	if state.selectedDir == "" {
		return ""
	}
	root := m.Root()
	rel, err := filepath.Rel(root, state.selectedDir)
	if err != nil {
		return ""
	}
	return normalizeProjectFolder(filepath.ToSlash(rel))
}

func (m *Model) projectStructureSelectedFolderRole() string {
	return m.projectStructureRoleFor(m.projectStructureSelectedFolderRel())
}

func (m *Model) projectStructureRoleFor(rel string) string {
	state := &m.projectStructure
	rel = normalizeProjectFolder(rel)
	d := state.draft.Structure
	for _, item := range []struct {
		Role   string
		Values []string
	}{
		{"source", d.SourceRoots},
		{"test", d.TestRoots},
		{"resources", d.ResourceRoots},
		{"testResources", d.TestResourceRoots},
		{"excluded", d.Excluded},
	} {
		for _, value := range item.Values {
			if normalizeProjectFolder(value) == rel {
				return item.Role
			}
		}
	}
	for _, detected := range state.image.Detected {
		if normalizeProjectFolder(detected.Path) != rel {
			continue
		}
		for _, unmarked := range d.Unmarked {
			if normalizeProjectFolder(unmarked) == rel {
				return ""
			}
		}
		return detected.Kind
	}
	return ""
}

func (m *Model) projectStructureExcludeFiles(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	ui.Row(c).MinHeight(34).Gap(8).AlignItems(ui.Center).Children(func() {
		projectStructureLabel(c, oi, "Exclude files:", 110)
		in := ui.TextInput(c, &state.draft.Structure.ExcludeFiles).Grow(1).Placeholder("*.log;tmp*")
		in.OnChange(func() { m.invalidate() })
	})
	ui.Text(c, "Separate name patterns with ;, use * for any symbols and ? for one.").
		FontSize(10).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
}

func (m *Model) projectStructureContentRoots(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "+ Content Root").OnClick(func() {
			m.BrowseProjectStructurePath(true, "Add Content Root", func(path string) {
				m.AddProjectStructureContentRoot(path)
			})
		})
	})
	for _, path := range state.draft.Structure.ContentRoots {
		path := path
		ui.Row(c).MinHeight(24).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, filepath.Base(path)).Grow(1).FontSize(11).TextColor(oi.text).SingleLine().Ellipsis("…")
			if ui.Button(c, "×").Clicked() {
				m.RemoveProjectStructureContentRoot(path)
			}
		})
	}
	ui.Row(c).Height(8)
}

func (m *Model) projectStructureMarkedRoots(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	for _, item := range []struct {
		Role  string
		Title string
		Value []string
	}{
		{"source", "Source Folders", state.draft.Structure.SourceRoots},
		{"test", "Test Source Folders", state.draft.Structure.TestRoots},
		{"resources", "Resource Folders", state.draft.Structure.ResourceRoots},
		{"testResources", "Test Resource Folders", state.draft.Structure.TestResourceRoots},
		{"excluded", "Excluded Folders", state.draft.Structure.Excluded},
	} {
		titleColor := oi.textStrong
		if item.Role == "test" {
			titleColor = oi.vcsAdded
		} else if item.Role == "excluded" {
			titleColor = oi.textMuted
		} else if item.Role == "resources" || item.Role == "testResources" {
			titleColor = oi.link
		}
		ui.Text(c, item.Title).FontSize(11).Bold().TextColor(titleColor).
			Padding(0, 0, 3, 0).SingleLine()
		if len(item.Value) == 0 {
			ui.Text(c, "None").FontSize(11).TextColor(oi.textSubtle).SingleLine()
		}
		for _, path := range item.Value {
			path := path
			ui.Row(c).MinHeight(22).Gap(4).AlignItems(ui.Center).Children(func() {
				shown := path
				if shown == "" {
					shown = filepath.Base(m.Root())
				}
				ui.Text(c, shown).Grow(1).FontSize(11).TextColor(oi.text).
					SingleLine().Ellipsis("…")
				if ui.Button(c, "×").Clicked() {
					m.MarkProjectStructureFolder(path, "")
				}
			})
		}
		ui.Row(c).Height(6)
	}
	if len(state.draft.Structure.Unmarked) > 0 {
		ui.Row(c).MinHeight(26).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d detected folders are unmarked.", len(state.draft.Structure.Unmarked))).
				FontSize(10).TextColor(oi.warning).SingleLine().Ellipsis("…")
			if ui.Button(c, "Restore").Clicked() {
				m.RestoreDetectedProjectRoots()
			}
		})
	}
	ui.Text(c, "Build-file roots are detected automatically. User marks override detection.").
		FontSize(10).TextColor(oi.textMuted).Wrap()
}

func (m *Model) projectStructureDependenciesPage(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	members := m.projectStructureWorkspaceMembers()
	if len(members) <= 1 {
		projectStructureSectionTitle(c, oi, "Project Dependencies")
		projectStructureNote(c, oi,
			"Dependency projects are indexed and navigable together with this project.", false)
		deps := m.projectStructureDependencyCandidates()
		if len(deps) == 0 {
			projectStructureNote(c, oi, "No candidate projects yet. Use Add Project… to pick one.", false)
		}
		for _, path := range deps {
			path := path
			checked := slicesContain(state.draft.Structure.Dependencies, path)
			ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Checkbox(c, &checked, filepath.Base(path)).Changed() {
					m.ToggleProjectStructureDependency(path, checked)
				}
				ui.Text(c, path).Grow(1).FontSize(11).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…")
			})
		}
		ui.Row(c).Children(func() {
			ui.Button(c, "+ Add Project…").OnClick(func() {
				m.BrowseProjectStructurePath(true, "Add Project Dependency", func(path string) {
					m.AddProjectStructureDependency(path)
				})
			})
		})
		return
	}

	projectStructureNote(c, oi,
		"Every workspace root is equal. Pick a root and check the roots it depends on.", false)
	m.ensureProjectStructureSelection()
	ui.Row(c).Grow(1).MinHeight(320).Children(func() {
		ui.Column(c).Width(230).Children(func() {
			for i, member := range members {
				selected := i == state.member
				row := ui.Row(c).Key("project-structure-member:"+member).Height(28).
					Padding(6, 8).Gap(5).AlignItems(ui.Center)
				if selected {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.hover)
				}
				row.Children(func() {
					ui.Text(c, "▣").FontSize(11).TextColor(oi.textMuted)
					ui.Text(c, filepath.Base(member)).FontSize(12).TextColor(oi.text).
						SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					state.member = i
					m.invalidate()
				}
			}
		})
		ui.Row(c).Width(1).Background(oi.borderStrong)
		ui.Column(c).Grow(1).PaddingStart(14).Children(func() {
			active := members[state.member]
			projectStructureSectionTitle(c, oi, "Dependencies of "+filepath.Base(active))
			for _, other := range members {
				if other == active {
					continue
				}
				other := other
				checked := slicesContain(state.draft.Structure.MemberDependencies[active], other)
				ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).Children(func() {
					if ui.Checkbox(c, &checked, filepath.Base(other)).Changed() {
						m.ToggleProjectStructureMemberDependency(active, other, checked)
					}
					ui.Text(c, other).Grow(1).FontSize(11).TextColor(oi.textMuted).
						SingleLine().Ellipsis("…")
				})
			}
			ui.Button(c, "+ Add Project…").OnClick(func() {
				m.BrowseProjectStructurePath(true, "Add Project Dependency", func(path string) {
					m.ToggleProjectStructureMemberDependency(active, path, true)
				})
			})
		})
	})
}

func (m *Model) projectStructureLibrariesPage(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	m.ensureProjectStructureSelection()
	ui.Row(c).Grow(1).MinHeight(360).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(230).Shrink(0).Children(func() {
			ui.Row(c).MinHeight(31).Gap(5).Children(func() {
				ui.Button(c, "+ JAR").OnClick(func() {
					m.BrowseProjectStructurePath(false, "Select Library JAR", func(path string) {
						m.AddProjectStructureLibrary(path, false)
					})
				})
				ui.Button(c, "+ Folder").OnClick(func() {
					m.BrowseProjectStructurePath(true, "Select Classes Directory", func(path string) {
						m.AddProjectStructureLibrary(path, true)
					})
				})
				ui.Button(c, "−").Disabled(state.library < 0).
					OnClick(func() { m.RemoveProjectStructureLibrary() })
			})
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Children(func() {
					if len(state.draft.Structure.Libraries) == 0 {
						ui.Text(c, "No project libraries added.").FontSize(11).
							TextColor(oi.textSubtle).Padding(8, 8).Wrap()
					}
					for i, library := range state.draft.Structure.Libraries {
						selected := i == state.library
						row := ui.Row(c).Key(fmt.Sprintf("project-structure-library:%d", i)).
							Height(30).Padding(7, 8).Gap(6).AlignItems(ui.Center)
						if selected {
							row.Background(oi.selected)
						} else if row.Hovered() {
							row.Background(oi.hover)
						}
						row.Children(func() {
							ui.Text(c, "▤").FontSize(11).TextColor(oi.textMuted)
							ui.Text(c, library.Name).Grow(1).FontSize(12).TextColor(oi.text).
								SingleLine().Ellipsis("…")
							ui.Text(c, library.Scope).FontSize(10).TextColor(oi.textSubtle).SingleLine()
						})
						if row.Clicked() {
							m.SelectProjectStructureLibrary(i)
						}
					}
				})
			})
		})
		ui.Row(c).Width(1).Background(oi.borderStrong)
		ui.Scroll(c).Grow(1).MinHeight(0).PaddingStart(14).Children(func() {
			ui.Column(c).Children(func() {
				if state.library >= 0 && state.library < len(state.draft.Structure.Libraries) {
					m.projectStructureLibraryDetails(c, oi, state.library)
				} else {
					projectStructureSectionTitle(c, oi, "Project Libraries")
					projectStructureNote(c, oi,
						"Add JARs for Java code assistance and Run/Debug, or class directories for Run/Debug only.",
						false)
				}
				if len(state.image.Libraries) > 0 {
					ui.Row(c).Height(12)
					projectStructureSectionTitle(c, oi, "Resolved Build & SDK Libraries")
					for _, group := range state.image.Libraries {
						ui.Row(c).MinHeight(24).Gap(6).Children(func() {
							ui.Text(c, "▤").TextColor(oi.textMuted)
							ui.Text(c, group.Name).FontSize(11).TextColor(oi.text).
								SingleLine().Ellipsis("…")
							ui.Text(c, group.Kind).FontSize(10).TextColor(oi.textSubtle).SingleLine()
						})
					}
				}
			})
		})
	})
}

func (m *Model) projectStructureLibraryDetails(c *ui.Context, oi oiColors, index int) {
	state := &m.projectStructure
	library := &state.draft.Structure.Libraries[index]
	projectStructureSectionTitle(c, oi, "Library Details")
	ui.Row(c).MinHeight(31).Gap(8).AlignItems(ui.Center).Children(func() {
		projectStructureLabel(c, oi, "Name", 120)
		in := ui.TextInput(c, &library.Name).Grow(1)
		in.OnChange(func() { m.invalidate() })
	})
	scopes := projectDependencyScopes
	scopeLabels := []string{"Compile", "Test", "Runtime", "Provided"}
	oldScope := library.Scope
	projectStructureChoiceRow(c, oi, "Scope", &oldScope, scopes, scopeLabels, false)
	if oldScope != library.Scope {
		m.SetProjectStructureLibraryField("scope", oldScope)
	}
	for _, item := range []struct {
		Field  string
		Title  string
		Add    string
		Browse string
		Dir    bool
		Values []string
	}{
		{"classes", "Classes", "Add JAR…", "Select Library JAR", false, library.Classes},
		{"sources", "Sources", "Attach Sources…", "Select Source Archive", false, library.Sources},
	} {
		item := item
		ui.Row(c).Height(8)
		ui.Text(c, item.Title).FontSize(11).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Row(c).Children(func() {
			ui.Button(c, "+ "+item.Add).OnClick(func() {
				m.BrowseProjectStructurePath(item.Dir, item.Browse, func(path string) {
					m.AppendProjectStructureLibraryPath(item.Field, path)
				})
			})
		})
		if len(item.Values) == 0 {
			ui.Text(c, "None").FontSize(11).TextColor(oi.textSubtle).SingleLine()
		}
		for _, path := range item.Values {
			path := path
			ui.Row(c).MinHeight(23).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, path).Grow(1).FontSize(11).TextColor(oi.text).
					SingleLine().Ellipsis("…")
				if ui.Button(c, "×").Clicked() {
					m.RemoveProjectStructureLibraryPath(item.Field, path)
				}
			})
		}
	}
}

func (m *Model) projectStructureFacetsPage(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	m.ensureProjectStructureSelection()
	ui.Row(c).Grow(1).MinHeight(360).Children(func() {
		ui.Column(c).Width(230).Children(func() {
			ui.Row(c).MinHeight(31).Gap(5).Children(func() {
				for _, kind := range []struct{ ID, Label string }{
					{"web", "Web"}, {"spring", "Spring"}, {"jpa", "JPA"},
				} {
					kind := kind
					ui.Button(c, "+ "+kind.Label).OnClick(func() {
						m.AddProjectStructureFacet(kind.ID)
					})
				}
				ui.Button(c, "−").Disabled(state.facet < 0).
					OnClick(func() { m.RemoveProjectStructureFacet() })
			})
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Children(func() {
					if len(state.draft.Structure.Facets) == 0 {
						ui.Text(c, "No custom facets configured.").FontSize(11).
							TextColor(oi.textSubtle).Padding(8, 8).Wrap()
					}
					for i, facet := range state.draft.Structure.Facets {
						selected := i == state.facet
						row := ui.Row(c).Key(fmt.Sprintf("project-structure-facet:%d", i)).
							Height(30).Padding(7, 8).Gap(5).AlignItems(ui.Center)
						if selected {
							row.Background(oi.selected)
						} else if row.Hovered() {
							row.Background(oi.hover)
						}
						row.Children(func() {
							ui.Text(c, "◈").TextColor(oi.textMuted)
							label := strings.ToUpper(facet.Kind[:1]) + facet.Kind[1:]
							ui.Text(c, label).Grow(1).FontSize(12).TextColor(oi.text).SingleLine()
							ui.Text(c, facet.Module).FontSize(10).TextColor(oi.textSubtle).SingleLine()
						})
						if row.Clicked() {
							m.SelectProjectStructureFacet(i)
						}
					}
				})
			})
		})
		ui.Row(c).Width(1).Background(oi.borderStrong)
		ui.Scroll(c).Grow(1).PaddingStart(14).Children(func() {
			ui.Column(c).Children(func() {
				if state.facet >= 0 && state.facet < len(state.draft.Structure.Facets) {
					m.projectStructureFacetDetails(c, oi, state.facet)
				} else {
					projectStructureSectionTitle(c, oi, "Framework Facets")
					projectStructureNote(c, oi,
						"Facets describe the frameworks used by a module. A Web facet also registers WEB-INF/lib JARs.",
						false)
				}
				if rows := m.projectStructureDetectedFacets(); len(rows) > 0 {
					ui.Row(c).Height(12)
					projectStructureSectionTitle(c, oi, "Auto-detected from Build")
					for _, row := range rows {
						ui.Text(c, "✓ "+row).FontSize(11).TextColor(oi.textMuted).
							Padding(2, 0).SingleLine().Ellipsis("…")
					}
				}
			})
		})
	})
}

func (m *Model) projectStructureFacetDetails(c *ui.Context, oi oiColors, index int) {
	facet := &m.projectStructure.draft.Structure.Facets[index]
	projectStructureSectionTitle(c, oi, "Facet Configuration")
	kinds := projectFacetKinds
	kindLabels := []string{"Web", "Spring", "JPA"}
	oldKind := facet.Kind
	projectStructureChoiceRow(c, oi, "Framework", &oldKind, kinds, kindLabels, false)
	if oldKind != facet.Kind {
		m.SetProjectStructureFacetField("kind", oldKind)
	}
	m.projectStructureTextField(c, oi, "Module", &facet.Module, ".",
		nil)
	m.projectStructureTextField(c, oi, "Deployment descriptor", &facet.Descriptor, "",
		func(m *Model) {
			m.BrowseProjectStructurePath(false, "Select Deployment Descriptor", func(path string) {
				m.SetProjectStructureFacetField("descriptor", path)
			})
		})
	if facet.Kind == "web" {
		m.projectStructureTextField(c, oi, "Web resource directory", &facet.WebRoot, "src/main/webapp",
			func(m *Model) {
				m.BrowseProjectStructurePath(true, "Select Web Resource Directory", func(path string) {
					m.SetProjectStructureFacetField("webRoot", path)
				})
			})
	}
}

func (m *Model) projectStructureArtifactsPage(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	m.ensureProjectStructureSelection()
	projectStructureNote(c, oi,
		"Custom artifact packaging is not supported yet. Build and deploy artifacts defined by Maven or Gradle.",
		false)
	ui.Row(c).Grow(1).MinHeight(320).Children(func() {
		ui.Column(c).Width(245).Children(func() {
			ui.Row(c).MinHeight(31).Gap(5).Children(func() {
				for _, item := range []struct{ ID, Label string }{
					{"jar", "JAR"}, {"warExploded", "WAR Exploded"}, {"war", "WAR"},
				} {
					ui.Button(c, item.Label).Disabled(true)
				}
				ui.Button(c, "−").Disabled(true)
			})
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Children(func() {
					if len(state.draft.Structure.Artifacts) == 0 {
						ui.Text(c, "No custom artifacts configured.").FontSize(11).
							TextColor(oi.textSubtle).Padding(8, 8).Wrap()
					}
					for i, artifact := range state.draft.Structure.Artifacts {
						selected := i == state.artifact
						row := ui.Row(c).Key(fmt.Sprintf("project-structure-artifact:%d", i)).
							Height(30).Padding(7, 8).Gap(5).AlignItems(ui.Center)
						if selected {
							row.Background(oi.selectedInactive)
						}
						row.Children(func() {
							ui.Text(c, "▥").TextColor(oi.textMuted)
							ui.Text(c, artifact.Name).Grow(1).FontSize(11).TextColor(oi.textMuted).
								SingleLine().Ellipsis("…")
							ui.Text(c, artifact.Kind).FontSize(10).TextColor(oi.textSubtle).SingleLine()
						})
					}
				})
			})
		})
		ui.Row(c).Width(1).Background(oi.borderStrong)
		ui.Scroll(c).Grow(1).PaddingStart(14).Children(func() {
			ui.Column(c).Children(func() {
				if state.artifact >= 0 && state.artifact < len(state.draft.Structure.Artifacts) {
					artifact := &state.draft.Structure.Artifacts[state.artifact]
					projectStructureSectionTitle(c, oi, "Artifact Settings")
					for _, item := range []struct {
						Label string
						Value *string
					}{
						{"Name", &artifact.Name},
						{"Module", &artifact.Module},
						{"Output directory", &artifact.OutputDirectory},
						{"Main class", &artifact.MainClass},
					} {
						ui.Row(c).MinHeight(31).Gap(8).Children(func() {
							projectStructureLabel(c, oi, item.Label, 130)
							ui.TextInput(c, item.Value).Grow(1).Disabled(true)
						})
					}
				}
				if len(state.image.Web) > 0 {
					ui.Row(c).Height(12)
					projectStructureSectionTitle(c, oi, "Detected Build Web Artifacts")
					for _, item := range state.image.Web {
						ui.Text(c, "▥ "+item.Name+" ("+item.Module+") · "+item.Tool).
							FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
					}
				}
			})
		})
	})
}

func (m *Model) projectStructureSDKsPage(c *ui.Context, oi oiColors) {
	state := &m.projectStructure
	rows := m.projectStructureSDKRows()
	m.ensureProjectStructureSelection()
	ui.Row(c).Grow(1).MinHeight(360).Children(func() {
		ui.Column(c).Width(250).Children(func() {
			ui.Row(c).MinHeight(31).Gap(5).Children(func() {
				ui.Button(c, "Download JDK…").OnClick(func() {
					m.OpenJDKDownloadDialog(0)
				})
				ui.Button(c, "Detect again").OnClick(func() {
					m.RefreshProjectStructureSDKs()
				})
			})
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Children(func() {
					if len(rows) == 0 {
						ui.Text(c, "No SDK installations detected yet.").FontSize(11).
							TextColor(oi.textSubtle).Padding(8, 8).Wrap()
					}
					for i, row := range rows {
						selected := i == state.sdk
						r := ui.Row(c).Key(fmt.Sprintf("project-structure-sdk:%d", i)).
							Height(30).Padding(7, 8).Gap(6).AlignItems(ui.Center)
						if selected {
							r.Background(oi.selected)
						} else if r.Hovered() {
							r.Background(oi.hover)
						}
						r.Children(func() {
							ui.Text(c, "⚙").TextColor(oi.textMuted)
							ui.Text(c, row.Name).Grow(1).FontSize(12).TextColor(oi.text).
								SingleLine().Ellipsis("…")
							ui.Text(c, row.Label).FontSize(10).TextColor(oi.textSubtle).SingleLine()
						})
						if r.Clicked() {
							m.SelectProjectStructureSDK(i)
						}
					}
				})
			})
		})
		ui.Row(c).Width(1).Background(oi.borderStrong)
		ui.Scroll(c).Grow(1).PaddingStart(14).Children(func() {
			ui.Column(c).Children(func() {
				if state.sdk >= 0 && state.sdk < len(rows) {
					row := rows[state.sdk]
					projectStructureSectionTitle(c, oi, "SDK Details")
					ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, row.Label).FontSize(10).TextColor(oi.textMuted).
							Padding(0, 5).Background(oi.badge)
						ui.Text(c, row.Name).FontSize(13).Bold().TextColor(oi.textStrong).SingleLine()
						if row.Version != "" {
							ui.Text(c, row.Version).FontSize(11).TextColor(oi.textMuted).SingleLine()
						}
						ui.Spacer(c)
						if row.Kind == "jdk" {
							ui.Button(c, "Use as Project SDK").
								Disabled(state.draft.JDK == row.Path).
								OnClick(func() { m.UseProjectStructureSDK() })
						}
					})
					ui.Row(c).MinHeight(31).Gap(8).Children(func() {
						projectStructureLabel(c, oi, "SDK home path", 130)
						ui.TextInput(c, &row.Path).Grow(1).Disabled(true)
					})
				}
				ui.Row(c).Height(12)
				projectStructureSectionTitle(c, oi, "Detected SDKs")
				ui.Row(c).MinHeight(24).Gap(10).Background(oi.island).Padding(5, 6).Children(func() {
					ui.Text(c, "Type").Width(90).FontSize(10).Bold().TextColor(oi.textMuted)
					ui.Text(c, "Name").Width(180).FontSize(10).Bold().TextColor(oi.textMuted)
					ui.Text(c, "Version").Width(70).FontSize(10).Bold().TextColor(oi.textMuted)
					ui.Text(c, "Home Path").Grow(1).FontSize(10).Bold().TextColor(oi.textMuted)
				})
				for _, row := range rows {
					ui.Row(c).MinHeight(24).Gap(10).Padding(5, 6).Children(func() {
						ui.Text(c, row.Label).Width(90).FontSize(10).TextColor(oi.textMuted).
							SingleLine()
						ui.Text(c, row.Name).Width(180).FontSize(11).TextColor(oi.text).
							SingleLine().Ellipsis("…")
						version := row.Version
						if version == "" {
							version = "—"
						}
						ui.Text(c, version).Width(70).FontSize(11).TextColor(oi.textMuted).SingleLine()
						ui.Text(c, tildifyPath(row.Path)).Grow(1).FontSize(11).TextColor(oi.textMuted).
							SingleLine().Ellipsis("…")
					})
				}
			})
		})
	})
}

// sortProjectStructureRows keeps rendering deterministic when a backend
// returns SDK roots in discovery order.
func sortProjectStructureRows(rows []projectStructureSDKRow) []projectStructureSDKRow {
	out := append([]projectStructureSDKRow(nil), rows...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Path < out[j].Path
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
