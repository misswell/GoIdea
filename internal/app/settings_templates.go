package app

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

type templateSettingsUI struct {
	liveID          string
	postfixID       string
	fileID          string
	fileKind        int
	fileIncludes    bool
	projectFileView bool
}

func cloneTemplatePrefs(prefs TemplatePrefs) TemplatePrefs {
	prefs = normalizeTemplatePrefs(prefs)
	prefs.Live.Disabled = append([]string(nil), prefs.Live.Disabled...)
	prefs.Live.Custom = append([]LiveTemplate(nil), prefs.Live.Custom...)
	for i := range prefs.Live.Custom {
		prefs.Live.Custom[i].Contexts = append([]string(nil), prefs.Live.Custom[i].Contexts...)
	}
	prefs.Postfix.Disabled = append([]string(nil), prefs.Postfix.Disabled...)
	schemes := make(map[string]FileTemplateScheme, len(prefs.Files.Schemes))
	for name, scheme := range prefs.Files.Schemes {
		schemes[name] = cloneFileTemplateScheme(scheme)
	}
	prefs.Files.Schemes = schemes
	projects := make(map[string]FileTemplateScheme, len(prefs.Files.Projects))
	for root, scheme := range prefs.Files.Projects {
		projects[root] = cloneFileTemplateScheme(scheme)
	}
	prefs.Files.Projects = projects
	return prefs
}

func templatePrefsEqual(left, right TemplatePrefs) bool {
	return reflect.DeepEqual(normalizeTemplatePrefs(cloneTemplatePrefs(left)), normalizeTemplatePrefs(cloneTemplatePrefs(right)))
}

func (m *Model) loadTemplateSettingsPage(key string) {
	if m.settingsDraft.templates.Version == 0 {
		m.settingsDraft.templates = cloneTemplatePrefs(*m.templates())
	}
	if key == settingsPageFileTemplates && m.Root() != "" {
		m.ensureSettingsFileTemplateScheme()
	}
}

func (m *Model) ensureSettingsFileTemplateScheme() {
	files := &m.settingsDraft.templates.Files
	if files.Schemes == nil {
		files.Schemes = map[string]FileTemplateScheme{}
	}
	if _, ok := files.Schemes[files.Default]; !ok {
		files.Schemes[files.Default] = defaultFileTemplateScheme()
	}
}

func (m *Model) settingsFileTemplateScheme() *FileTemplateScheme {
	m.ensureSettingsFileTemplateScheme()
	files := &m.settingsDraft.templates.Files
	if m.settingsTemplates.projectFileView && m.Root() != "" {
		key := canonicalTemplateProject(m.Root())
		scheme := files.Projects[key]
		if len(scheme.Templates) == 0 {
			scheme = cloneFileTemplateScheme(files.effectiveScheme(m.Root()))
			files.Projects[key] = scheme
		}
		return &scheme
	}
	scheme := files.Schemes[files.Default]
	return &scheme
}

// mutateSettingsFileTemplateScheme edits the chosen view rather than a copy.
func (m *Model) mutateSettingsFileTemplateScheme(edit func(*FileTemplateScheme)) {
	m.ensureSettingsFileTemplateScheme()
	files := &m.settingsDraft.templates.Files
	if m.settingsTemplates.projectFileView && m.Root() != "" {
		key := canonicalTemplateProject(m.Root())
		scheme := cloneFileTemplateScheme(files.effectiveScheme(m.Root()))
		edit(&scheme)
		files.Projects[key] = normalizeFileTemplateScheme(scheme)
		return
	}
	scheme := cloneFileTemplateScheme(files.Schemes[files.Default])
	edit(&scheme)
	files.Schemes[files.Default] = normalizeFileTemplateScheme(scheme)
}

func (m *Model) settingsLiveTemplatesBody(c *ui.Context, oi oiColors) {
	prefs := &m.settingsDraft.templates.Live
	enabled := prefs.Enabled
	if ui.Checkbox(c, &enabled, "Enable Live Templates").Changed() {
		prefs.Enabled = enabled
		m.invalidate()
	}
	caseSensitive := prefs.CaseSensitive
	if ui.Checkbox(c, &caseSensitive, "Case sensitive abbreviations").Changed() {
		prefs.CaseSensitive = caseSensitive
		m.invalidate()
	}

	settingsSectionTitle(c, oi, "Templates")
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "＋").Tooltip("Add live template").OnClick(m.addSettingsLiveTemplate)
		copyButton := ui.Button(c, "Copy")
		copyButton.Disabled(m.settingsTemplates.liveID == "")
		copyButton.OnClick(m.copySettingsLiveTemplate)
		remove := ui.Button(c, "−")
		remove.Tooltip("Remove or disable")
		remove.Disabled(m.settingsTemplates.liveID == "")
		remove.OnClick(m.removeSettingsLiveTemplate)
	})
	templates := m.settingsLiveTemplateRows()
	m.ensureSettingsLiveSelection(templates)
	ui.Row(c).MinHeight(24).Gap(8).AlignItems(ui.Center).PaddingStart(4).Children(func() {
		settingsFormLabelDim(c, oi, "", 24)
		settingsFormLabelDim(c, oi, "Abbreviation", 120)
		settingsFormLabelDim(c, oi, "Description", 190)
		settingsFormLabelDim(c, oi, "Contexts", 150)
	})
	ui.Scroll(c).Height(160).Border(1, oi.border).Children(func() {
		ui.Column(c).Children(func() {
			for index, template := range templates {
				index, template := index, template
				disabled := stringSet(prefs.Disabled)
				row := ui.Row(c.Key("settings:live-template:"+template.ID)).MinHeight(27).
					Gap(8).AlignItems(ui.Center).Padding(0, 4)
				if template.ID == m.settingsTemplates.liveID {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.hover)
				}
				row.OnClick(func() {
					m.settingsTemplates.liveID = template.ID
					m.invalidate()
				})
				row.Children(func() {
					on := !disabled[template.ID]
					if ui.Checkbox(c, &on, "").Changed() {
						m.setSettingsTemplateEnabled(prefs.Disabled, template.ID, on)
					}
					ui.Text(c, template.Abbreviation).Font("monospace").FontSize(11.5).
						TextColor(oi.text).Width(120).SingleLine().Ellipsis("…")
					ui.Text(c, template.Name).FontSize(11.5).TextColor(oi.text).
						Width(190).SingleLine().Ellipsis("…")
					ui.Text(c, strings.Join(template.Contexts, ", ")).FontSize(11).
						TextColor(oi.textMuted).Width(150).SingleLine().Ellipsis("…")
				})
				_ = index
			}
		})
	})

	edit, ok := m.settingsLiveTemplate(m.settingsTemplates.liveID)
	if !ok {
		ui.Text(c, "Select a template to edit.").FontSize(12).TextColor(oi.textMuted).
			Padding(12, 0)
		return
	}
	settingsSectionTitle(c, oi, "Template Text")
	changed := templateFieldChanged(c, oi, "Abbreviation", &edit.Abbreviation, "sout")
	changed = templateFieldChanged(c, oi, "Description", &edit.Name, "Print expression") || changed
	changed = templateFieldChanged(c, oi, "Template text", &edit.Body, "$END$") || changed
	contexts := strings.Join(edit.Contexts, ", ")
	if templateFieldChanged(c, oi, "Contexts", &contexts, "java, go, html (empty = all)") {
		edit.Contexts = splitTemplateContexts(contexts)
		changed = true
	}
	if changed {
		m.commitSettingsLiveTemplate(edit)
	}
	ui.Text(c, "Use $END$ for the final caret and $SELECTION$ in surround templates. "+
		"Repeated variables select the same field after explicit expansion.").
		FontSize(11).TextColor(oi.textMuted).MaxWidth(720).Wrap()
}

func (m *Model) settingsPostfixBody(c *ui.Context, oi oiColors) {
	prefs := &m.settingsDraft.templates.Postfix
	enabled := prefs.Enabled
	if ui.Checkbox(c, &enabled, "Enable postfix completion").Changed() {
		prefs.Enabled = enabled
		m.invalidate()
	}
	ui.Text(c, "Type an expression and a template name after a dot, for example value.not or list.for.").
		FontSize(11).TextColor(oi.textMuted).MaxWidth(720).Wrap()
	settingsSectionTitle(c, oi, "Templates")
	ui.Row(c).MinHeight(24).Gap(8).AlignItems(ui.Center).PaddingStart(4).Children(func() {
		settingsFormLabelDim(c, oi, "", 24)
		settingsFormLabelDim(c, oi, "Postfix", 110)
		settingsFormLabelDim(c, oi, "Description", 300)
	})
	ui.Scroll(c).Height(min(float32(360), float32(30+len(defaultPostfixTemplates)*28))).
		Border(1, oi.border).Children(func() {
		ui.Column(c).Children(func() {
			for _, template := range defaultPostfixTemplates {
				template := template
				row := ui.Row(c.Key("settings:postfix:"+template.ID)).MinHeight(27).
					Gap(8).AlignItems(ui.Center).Padding(0, 4)
				if template.ID == m.settingsTemplates.postfixID {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.hover)
				}
				row.OnClick(func() {
					m.settingsTemplates.postfixID = template.ID
					m.invalidate()
				})
				row.Children(func() {
					on := !stringSet(prefs.Disabled)[template.ID]
					if ui.Checkbox(c, &on, "").Changed() {
						m.setSettingsTemplateEnabled(prefs.Disabled, template.ID, on)
					}
					ui.Text(c, "."+template.Trigger).Font("monospace").FontSize(11.5).
						TextColor(oi.text).Width(110)
					ui.Text(c, template.Detail).FontSize(11.5).TextColor(oi.textMuted).
						Width(300).SingleLine()
				})
			}
		})
	})
}

func (m *Model) settingsFileTemplatesBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Scheme")
	view := "Default"
	if m.settingsTemplates.projectFileView {
		view = "Project"
	}
	ui.Row(c).MinHeight(32).Gap(12).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Settings for", 150)
		for _, choice := range []struct {
			id, label string
		}{
			{"Default", "Default"},
			{"Project", "Project: " + filepath.Base(m.Root())},
		} {
			disabled := choice.id == "Project" && m.Root() == ""
			if ui.Radio(c, &view, choice.id, choice.label).Disabled(disabled).Changed() {
				m.settingsTemplates.projectFileView = choice.id == "Project"
				m.ensureSettingsFileTemplateScheme()
				m.invalidate()
			}
		}
	})
	ui.Text(c, "A project scheme is copied from Default when first opened and then keeps its own edits.").
		FontSize(11).TextColor(oi.textMuted).MaxWidth(760).Wrap()

	m.ensureSettingsFileTemplateScheme()
	scheme := m.settingsFileTemplateScheme()
	m.ensureSettingsFileSelection(scheme)
	settingsSectionTitle(c, oi, "Files")
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "＋ File").OnClick(m.addSettingsFileTemplate)
		copyButton := ui.Button(c, "Copy File")
		copyButton.Disabled(m.settingsTemplates.fileID == "")
		copyButton.OnClick(m.copySettingsFileTemplate)
		remove := ui.Button(c, "− File")
		remove.Disabled(m.settingsTemplates.fileID == "")
		remove.OnClick(m.removeSettingsFileTemplate)
		ui.Button(c, "＋ Include").OnClick(m.addSettingsFileInclude)
	})
	ui.Row(c).Height(150).Gap(10).AlignItems(ui.Stretch).Children(func() {
		ui.Scroll(c).Width(250).Border(1, oi.border).Children(func() {
			ui.Column(c).Children(func() {
				for _, template := range scheme.Templates {
					template := template
					row := ui.Row(c.Key("settings:file-template:"+template.id)).Height(26).
						Padding(0, 7).AlignItems(ui.Center)
					if !m.settingsTemplates.fileIncludes && template.id == m.settingsTemplates.fileID {
						row.Background(oi.selected)
					} else if row.Hovered() {
						row.Background(oi.hover)
					}
					row.Children(func() {
						fileIconEl(c, oi, "file."+template.extension, 15, oi.textMuted)
						ui.Text(c, template.name).FontSize(11.5).TextColor(oi.text).
							Grow(1).SingleLine().Ellipsis("…")
					})
					if row.Clicked() {
						m.settingsTemplates.fileID = template.id
						m.settingsTemplates.fileIncludes = false
						m.settingsTemplates.fileKind = 0
						m.invalidate()
					}
				}
				includeNames := make([]string, 0, len(scheme.Includes))
				for name := range scheme.Includes {
					includeNames = append(includeNames, name)
				}
				sort.Strings(includeNames)
				for _, name := range includeNames {
					name := name
					row := ui.Row(c.Key("settings:file-include:"+name)).Height(26).
						Padding(0, 7).AlignItems(ui.Center)
					if m.settingsTemplates.fileIncludes && m.settingsTemplates.fileID == name {
						row.Background(oi.selected)
					} else if row.Hovered() {
						row.Background(oi.hover)
					}
					row.Children(func() {
						oiIconEl(c, "nodes/template", 15, oi, oi.textMuted)
						ui.Text(c, name).FontSize(11.5).Font("monospace").TextColor(oi.text).
							Grow(1).SingleLine().Ellipsis("…")
					})
					if row.Clicked() {
						m.settingsTemplates.fileID = name
						m.settingsTemplates.fileIncludes = true
						m.invalidate()
					}
				}
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if m.settingsTemplates.fileIncludes {
				m.settingsFileIncludeEditor(c, oi, scheme)
			} else {
				m.settingsFileTemplateEditor(c, oi, scheme)
			}
		})
	})
}

func (m *Model) settingsFileTemplateEditor(c *ui.Context, oi oiColors, scheme *FileTemplateScheme) {
	index := m.settingsFileTemplateIndex(scheme)
	if index < 0 {
		ui.Text(c, "Add or select a file template.").FontSize(12).
			TextColor(oi.textMuted).Padding(0, 4)
		return
	}
	template := &scheme.Templates[index]
	if template.kinds == nil {
		template.kinds = []projectNewKind{{id: "file", label: "File", body: "$END$"}}
	}
	m.settingsTemplates.fileKind = min(max(0, m.settingsTemplates.fileKind), len(template.kinds)-1)
	updateTemplate := func(label, value string) {
		m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
			if index >= len(updated.Templates) {
				return
			}
			target := &updated.Templates[index]
			switch label {
			case "Name":
				target.name = value
			case "Extension":
				target.extension = value
			case "Default name":
				target.defaultName = value
			}
		})
	}
	row := func(label string, value string, placeholder string) {
		mutateText := value
		ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, label, 120)
			input := ui.TextInput(c, &mutateText)
			if placeholder != "" {
				input.Placeholder(placeholder)
			}
			if mutateText != value {
				updateTemplate(label, mutateText)
			}
		})
	}
	row("Name", template.name, "Java File")
	row("Extension", template.extension, "java")
	row("Default name", template.defaultName, "Main")
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Name style", 120)
		style := template.nameStyle
		if style == "" {
			style = "asis"
		}
		selectInput := ui.Select(c, &style, []string{"asis", "snake", "pascal"}).Width(140)
		if selectInput.Changed() {
			m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
				if index < len(updated.Templates) {
					updated.Templates[index].nameStyle = style
				}
			})
		}
	})
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Package style", 120)
		style := template.packageStyle
		selectInput := ui.Select(c, &style, []string{"", "jvm", "go", "php"}).Width(140)
		if selectInput.Changed() {
			m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
				if index < len(updated.Templates) {
					updated.Templates[index].packageStyle = style
				}
			})
		}
	})
	ui.Row(c).MinHeight(28).Gap(6).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Kinds", 120)
		labels := make([]string, len(template.kinds))
		for i, kind := range template.kinds {
			labels[i] = kind.label
		}
		chosen := labels[m.settingsTemplates.fileKind]
		sel := ui.Select(c, &chosen, labels).Width(170)
		if sel.Changed() {
			for i, label := range labels {
				if label == chosen {
					m.settingsTemplates.fileKind = i
					break
				}
			}
		}
		ui.Button(c, "＋").OnClick(func() {
			m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
				if index >= len(updated.Templates) {
					return
				}
				updated.Templates[index].kinds = append(updated.Templates[index].kinds, projectNewKind{
					id:    fmt.Sprintf("kind-%d", len(updated.Templates[index].kinds)+1),
					label: "New kind", body: "$END$",
				})
			})
		})
		remove := ui.Button(c, "−")
		remove.Disabled(len(template.kinds) == 1)
		remove.OnClick(func() {
			m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
				if index >= len(updated.Templates) || len(updated.Templates[index].kinds) <= 1 {
					return
				}
				kinds := updated.Templates[index].kinds
				kind := m.settingsTemplates.fileKind
				updated.Templates[index].kinds = append(kinds[:kind], kinds[kind+1:]...)
			})
			m.settingsTemplates.fileKind = 0
		})
	})
	kind := &template.kinds[m.settingsTemplates.fileKind]
	kindRow := func(label string, value *string, placeholder string) {
		mutateText := *value
		ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, label, 120)
			input := ui.TextInput(c, &mutateText)
			if placeholder != "" {
				input.Placeholder(placeholder)
			}
			if mutateText != *value {
				valueCopy := mutateText
				m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
					if index < len(updated.Templates) && m.settingsTemplates.fileKind < len(updated.Templates[index].kinds) {
						mutateKindField(&updated.Templates[index].kinds[m.settingsTemplates.fileKind], label, valueCopy)
					}
				})
			}
		})
	}
	kindRow("Kind name", &kind.label, "Class")
	kindRow("Kind extension", &kind.extension, "java")
	ui.Text(c, "Template body").FontSize(11.5).Bold().TextColor(oi.textStrong)
	body := kind.body
	area := ui.TextArea(c, &body).Height(130).Font("monospace").FontSize(11.5)
	if area.Changed() {
		m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
			if index < len(updated.Templates) && m.settingsTemplates.fileKind < len(updated.Templates[index].kinds) {
				updated.Templates[index].kinds[m.settingsTemplates.fileKind].body = body
			}
		})
	}
}

func mutateKindField(kind *projectNewKind, label, value string) {
	switch label {
	case "Kind name":
		kind.label = value
	case "Kind extension":
		kind.extension = value
	}
}

func (m *Model) settingsFileIncludeEditor(c *ui.Context, oi oiColors, scheme *FileTemplateScheme) {
	name := m.settingsTemplates.fileID
	body, ok := scheme.Includes[name]
	if !ok {
		return
	}
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Include name", 120)
		newName := name
		ui.TextInput(c, &newName)
		if newName != name && strings.TrimSpace(newName) != "" {
			m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
				value := updated.Includes[name]
				delete(updated.Includes, name)
				updated.Includes[strings.TrimSpace(newName)] = value
			})
			m.settingsTemplates.fileID = strings.TrimSpace(newName)
		}
		ui.Button(c, "Remove").OnClick(func() {
			m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
				delete(updated.Includes, name)
			})
			m.settingsTemplates.fileID = ""
			m.settingsTemplates.fileIncludes = false
		})
	})
	area := ui.TextArea(c, &body).Grow(1).MinHeight(120).Font("monospace").FontSize(11.5)
	if area.Changed() {
		m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
			updated.Includes[name] = body
		})
	}
	ui.Text(c, "Reference this text in a kind with #parse(\"Include name\").").
		FontSize(11).TextColor(oi.textMuted)
}

func templateField(c *ui.Context, oi oiColors, label string, value *string, placeholder string) {
	_ = templateFieldChanged(c, oi, label, value, placeholder)
}

func templateFieldChanged(c *ui.Context, oi oiColors, label string, value *string, placeholder string) bool {
	original := *value
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label, 150)
		input := ui.TextInput(c, value)
		if placeholder != "" {
			input.Placeholder(placeholder)
		}
	})
	return *value != original
}

func splitTemplateContexts(value string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' }) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return uniqueStrings(out)
}

func (m *Model) settingsLiveTemplateRows() []LiveTemplate {
	byID := map[string]LiveTemplate{}
	for _, template := range defaultLiveTemplates {
		byID[template.ID] = template
	}
	for _, template := range m.settingsDraft.templates.Live.Custom {
		byID[template.ID] = template
	}
	out := make([]LiveTemplate, 0, len(byID))
	for _, template := range byID {
		out = append(out, template)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Abbreviation == out[j].Abbreviation {
			return out[i].Name < out[j].Name
		}
		return out[i].Abbreviation < out[j].Abbreviation
	})
	return out
}

func (m *Model) ensureSettingsLiveSelection(templates []LiveTemplate) {
	for _, template := range templates {
		if template.ID == m.settingsTemplates.liveID {
			return
		}
	}
	if len(templates) == 0 {
		m.settingsTemplates.liveID = ""
	} else {
		m.settingsTemplates.liveID = templates[0].ID
	}
}

func (m *Model) settingsLiveTemplate(id string) (LiveTemplate, bool) {
	if id == "" {
		return LiveTemplate{}, false
	}
	for _, template := range m.settingsDraft.templates.Live.Custom {
		if template.ID == id {
			template.Contexts = append([]string(nil), template.Contexts...)
			return template, true
		}
	}
	for _, template := range defaultLiveTemplates {
		if template.ID == id {
			template.Contexts = append([]string(nil), template.Contexts...)
			return template, true
		}
	}
	return LiveTemplate{}, false
}

func (m *Model) commitSettingsLiveTemplate(edited LiveTemplate) {
	if strings.TrimSpace(edited.ID) == "" {
		return
	}
	custom := &m.settingsDraft.templates.Live.Custom
	var builtin *LiveTemplate
	for i := range defaultLiveTemplates {
		if defaultLiveTemplates[i].ID == edited.ID {
			builtin = &defaultLiveTemplates[i]
			break
		}
	}
	for i := range *custom {
		if (*custom)[i].ID != edited.ID {
			continue
		}
		if builtin != nil && reflect.DeepEqual(edited, *builtin) {
			*custom = append((*custom)[:i], (*custom)[i+1:]...)
			return
		}
		(*custom)[i] = edited
		return
	}
	if builtin != nil && reflect.DeepEqual(edited, *builtin) {
		return
	}
	*custom = append(*custom, edited)
}

func (m *Model) addSettingsLiveTemplate() {
	id := fmt.Sprintf("live.custom.%d", len(m.settingsDraft.templates.Live.Custom)+1)
	template := LiveTemplate{ID: id, Abbreviation: "tpl", Name: "New live template", Body: "$END$"}
	m.settingsDraft.templates.Live.Custom = append(m.settingsDraft.templates.Live.Custom, template)
	m.settingsTemplates.liveID = id
	m.invalidate()
}

func (m *Model) copySettingsLiveTemplate() {
	template, ok := m.settingsLiveTemplate(m.settingsTemplates.liveID)
	if !ok {
		return
	}
	clone := template
	clone.ID = fmt.Sprintf("live.custom.%d", len(m.settingsDraft.templates.Live.Custom)+1)
	clone.Name += " (copy)"
	m.settingsDraft.templates.Live.Custom = append(m.settingsDraft.templates.Live.Custom, clone)
	m.settingsTemplates.liveID = clone.ID
	m.invalidate()
}

func (m *Model) removeSettingsLiveTemplate() {
	id := m.settingsTemplates.liveID
	if id == "" {
		return
	}
	custom := m.settingsDraft.templates.Live.Custom
	for i := range custom {
		if custom[i].ID == id {
			m.settingsDraft.templates.Live.Custom = append(custom[:i], custom[i+1:]...)
			m.settingsTemplates.liveID = ""
			m.invalidate()
			return
		}
	}
	m.setSettingsTemplateEnabled(m.settingsDraft.templates.Live.Disabled, id, false)
}

func (m *Model) setSettingsTemplateEnabled(disabled []string, id string, enabled bool) {
	seen := stringSet(disabled)
	var out []string
	for _, value := range disabled {
		if value != id {
			out = append(out, value)
		}
	}
	if !enabled && !seen[id] {
		out = append(out, id)
	}
	if out == nil {
		out = []string{}
	}
	switch m.settingsPage {
	case settingsPagePostfix:
		m.settingsDraft.templates.Postfix.Disabled = out
	default:
		m.settingsDraft.templates.Live.Disabled = out
	}
	m.invalidate()
}

func (m *Model) ensureSettingsFileSelection(scheme *FileTemplateScheme) {
	if m.settingsTemplates.fileIncludes {
		if _, ok := scheme.Includes[m.settingsTemplates.fileID]; !ok {
			m.settingsTemplates.fileIncludes = false
			m.settingsTemplates.fileID = ""
		}
	}
	if !m.settingsTemplates.fileIncludes && m.settingsFileTemplateIndex(scheme) < 0 {
		if len(scheme.Templates) == 0 {
			m.settingsTemplates.fileID = ""
		} else {
			m.settingsTemplates.fileID = scheme.Templates[0].id
			m.settingsTemplates.fileKind = 0
		}
	}
}

func (m *Model) settingsFileTemplateIndex(scheme *FileTemplateScheme) int {
	for i := range scheme.Templates {
		if scheme.Templates[i].id == m.settingsTemplates.fileID {
			return i
		}
	}
	return -1
}

func (m *Model) addSettingsFileTemplate() {
	id := fmt.Sprintf("custom-%d", len(m.settingsDraft.templates.Files.Projects)+len(m.settingsDraft.templates.Files.Schemes)+1)
	m.mutateSettingsFileTemplateScheme(func(scheme *FileTemplateScheme) {
		scheme.Templates = append(scheme.Templates, projectNewTemplate{
			id: id, name: "New File", extension: "txt", nameStyle: "asis",
			kinds: []projectNewKind{{id: "file", label: "File", body: "$END$"}},
		})
	})
	m.settingsTemplates.fileID = id
	m.settingsTemplates.fileIncludes = false
	m.settingsTemplates.fileKind = 0
	m.invalidate()
}

func (m *Model) copySettingsFileTemplate() {
	scheme := m.settingsFileTemplateScheme()
	index := m.settingsFileTemplateIndex(scheme)
	if index < 0 {
		return
	}
	clone := cloneProjectNewTemplate(scheme.Templates[index])
	clone.id = fmt.Sprintf("%s-copy-%d", clone.id, len(scheme.Templates)+1)
	clone.name += " (copy)"
	m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
		updated.Templates = append(updated.Templates, clone)
	})
	m.settingsTemplates.fileID = clone.id
	m.invalidate()
}

func (m *Model) removeSettingsFileTemplate() {
	id := m.settingsTemplates.fileID
	if id == "" {
		return
	}
	m.mutateSettingsFileTemplateScheme(func(scheme *FileTemplateScheme) {
		for i := range scheme.Templates {
			if scheme.Templates[i].id == id && len(scheme.Templates) > 1 {
				scheme.Templates = append(scheme.Templates[:i], scheme.Templates[i+1:]...)
				return
			}
		}
	})
	m.settingsTemplates.fileID = ""
	m.invalidate()
}

func (m *Model) addSettingsFileInclude() {
	name := fmt.Sprintf("Include %d", len(m.settingsDraft.templates.Files.Projects)+1)
	scheme := m.settingsFileTemplateScheme()
	for {
		if _, exists := scheme.Includes[name]; !exists {
			break
		}
		name += " copy"
	}
	m.mutateSettingsFileTemplateScheme(func(updated *FileTemplateScheme) {
		if updated.Includes == nil {
			updated.Includes = map[string]string{}
		}
		updated.Includes[name] = "$END$"
	})
	m.settingsTemplates.fileID = name
	m.settingsTemplates.fileIncludes = true
	m.invalidate()
}

func (m *Model) templateSettingsDirty(key string) bool {
	stored := m.templates()
	staged := &m.settingsDraft.templates
	switch key {
	case settingsPageLiveTemplates:
		return !reflect.DeepEqual(normalizeTemplatePrefs(*stored).Live, normalizeTemplatePrefs(*staged).Live)
	case settingsPagePostfix:
		return !reflect.DeepEqual(normalizeTemplatePrefs(*stored).Postfix, normalizeTemplatePrefs(*staged).Postfix)
	case settingsPageFileTemplates:
		return !reflect.DeepEqual(normalizeTemplatePrefs(*stored).Files, normalizeTemplatePrefs(*staged).Files)
	default:
		return false
	}
}

func (m *Model) resetTemplateSettingsPage(key string) {
	defaults := defaultTemplatePrefs()
	switch key {
	case settingsPageLiveTemplates:
		m.settingsDraft.templates.Live = defaults.Live
		m.settingsTemplates.liveID = ""
	case settingsPagePostfix:
		m.settingsDraft.templates.Postfix = defaults.Postfix
	case settingsPageFileTemplates:
		files := &m.settingsDraft.templates.Files
		if m.settingsTemplates.projectFileView && m.Root() != "" {
			// Project Reset removes only the current project's override. Other
			// projects and the global Default scheme keep their edits.
			delete(files.Projects, canonicalTemplateProject(m.Root()))
		} else {
			// Default Reset replaces the active global scheme while keeping
			// unrelated schemes and every project override.
			if files.Schemes == nil {
				files.Schemes = map[string]FileTemplateScheme{}
			}
			files.Default = defaults.Files.Default
			files.Schemes[files.Default] = cloneFileTemplateScheme(defaults.Files.Schemes[defaults.Files.Default])
			if files.Projects == nil {
				files.Projects = map[string]FileTemplateScheme{}
			}
		}
		m.settingsTemplates.fileID = ""
		m.settingsTemplates.fileIncludes = false
		m.settingsTemplates.fileKind = 0
	}
	m.invalidate()
}
