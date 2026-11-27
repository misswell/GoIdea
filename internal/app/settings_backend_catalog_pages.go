package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func settingsBackendStatus(c *ui.Context, oi oiColors, loading bool, errText, noun string, retry func()) {
	if loading {
		ui.Row(c).Height(settingsFormRow).PaddingStart(settingsSectionIndent).
			Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(12, 12).TextColor(oi.textMuted).Label("Loading " + noun)
			ui.Text(c, "Loading "+noun+"…").FontSize(12).TextColor(oi.textMuted).SingleLine()
		})
		return
	}
	if errText == "" {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, errText).FontSize(12).TextColor(oi.danger).SingleLine().Ellipsis("…").Grow(1)
		ui.Button(c, "Retry").OnClick(retry)
	})
}

func settingsBackendScopeText(c *ui.Context, oi oiColors, scope string) {
	where := "Application"
	if scope == "project" {
		where = "This project"
	} else if scope == "mixed" {
		where = "this project and the application"
	}
	ui.Row(c).Height(26).Padding(0, settingsSectionIndent, 6, 0).
		Gap(6).AlignItems(ui.Center).Children(func() {
		oiIconEl(c, "status/info", 14, oi, oi.textSubtle)
		ui.Text(c, "Stored in "+where+".").FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
}

func settingsEditableStringList(
	c *ui.Context,
	oi oiColors,
	values []string,
	placeholder string,
	remove func(int),
) {
	if len(values) == 0 {
		ui.Text(c, "No entries").FontSize(12).TextColor(oi.textMuted).SingleLine()
		return
	}
	for i := range values {
		i := i
		ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &values[i]).Grow(1).Placeholder(placeholder)
			ui.Button(c, "Remove").Width(72).OnClick(func() { remove(i) })
		})
	}
}

func (m *Model) settingsFileTypesBody(c *ui.Context, oi oiColors) {
	state := &m.fileTypesSettings
	settingsBackendScopeText(c, oi, state.scope)
	settingsBackendStatus(c, oi, state.loading, state.err, "file types", m.loadFileTypesSettings)
	if !state.loaded {
		return
	}

	m.settingsSectionTitle(c, oi, "Recognized File Types")
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Filter file types", settingsLabelMaxWidth)
		ui.TextInput(c, &state.filter).Grow(1).MaxWidth(settingsControlMaxWidth).
			Placeholder("Name or pattern")
	})
	filtered := m.filteredFileTypeLanguages()
	if len(filtered) == 0 {
		ui.Text(c, "No file types match this filter.").FontSize(12).TextColor(oi.textMuted).SingleLine()
		return
	}
	labels := make([]string, len(filtered))
	selectedLabel := filtered[0].Name + " (" + filtered[0].ID + ")"
	for i, language := range filtered {
		labels[i] = language.Name + " (" + language.ID + ")"
		if language.ID == state.selected {
			selectedLabel = labels[i]
		}
	}
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "File type", settingsLabelMaxWidth)
		if ui.Select(c, &selectedLabel, labels).MinWidth(settingsSelectMinWidth).
			Grow(1).MaxWidth(settingsControlMaxWidth).Changed() {
			for i, label := range labels {
				if label == selectedLabel {
					state.selected = filtered[i].ID
					state.patternDraft = ""
					state.hashbangDraft = ""
					break
				}
			}
		}
	})
	language, ok := m.fileTypeLanguage()
	if !ok {
		return
	}
	patterns := fileTypeEffectivePatterns(language, state.draft)
	ui.Text(c, "File name patterns").FontSize(12).Bold().TextColor(oi.text).SingleLine()
	ui.Text(c, "* matches any characters and ? matches one character.").
		FontSize(11).TextColor(oi.textMuted).SingleLine()
	for _, pattern := range patterns {
		pattern := pattern
		ui.Row(c).MinHeight(28).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, pattern).FontSize(12).TextColor(oi.text).Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "Remove").Width(72).OnClick(func() {
				fileTypesRemovePattern(&state.draft, language, pattern)
				m.invalidate()
			})
		})
	}
	ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &state.patternDraft).Grow(1).Placeholder("*.ext")
		ui.Button(c, "Add").Width(56).OnClick(func() {
			fileTypesAddPattern(&state.draft, state.languages, language.ID, state.patternDraft)
			state.patternDraft = ""
			m.invalidate()
		})
	})

	ui.Row(c).Height(10)
	ui.Text(c, "Hashbang patterns").FontSize(12).Bold().TextColor(oi.text).SingleLine()
	assoc := state.draft.FileTypes[language.ID]
	settingsEditableStringList(c, oi, assoc.Hashbangs, "interpreter", func(index int) {
		m.removeFileTypeHashbang(language.ID, index)
	})
	ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &state.hashbangDraft).Grow(1).Placeholder("node")
		ui.Button(c, "Add").Width(56).OnClick(func() { m.addFileTypeHashbang(language.ID) })
	})
	if language.FirstLine != "" {
		ui.Text(c, "Also recognized by its first line: "+language.FirstLine).
			FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	}

	m.settingsSectionTitle(c, oi, "Files opened in associated applications")
	ui.Text(c, "Files matching these patterns open in the system application instead of an editor.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	settingsEditableStringList(c, oi, state.draft.AssociatedApplications, "*.xlsx", func(index int) {
		values := state.draft.AssociatedApplications
		if index >= 0 && index < len(values) {
			state.draft.AssociatedApplications = append(values[:index:index], values[index+1:]...)
			m.invalidate()
		}
	})
	ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &state.associatedDraft).Grow(1).Placeholder("*.xlsx")
		ui.Button(c, "Add").Width(56).OnClick(func() { m.addAssociatedApplication() })
	})

	ui.Row(c).Height(14)
	m.settingsSectionTitle(c, oi, "Override File Type")
	ui.Text(c, "Override the language of one project-relative or absolute file.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	overrides := m.fileTypeOverrideEntries()
	if len(overrides) == 0 {
		ui.Text(c, "No file type overrides").FontSize(12).TextColor(oi.textMuted).SingleLine()
	}
	for _, entry := range overrides {
		entry := entry
		ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, entry.Path).FontSize(12).TextColor(oi.text).Grow(1).SingleLine().Ellipsis("…")
			ui.Text(c, entry.Language).FontSize(11).TextColor(oi.textMuted).Width(150).SingleLine().Ellipsis("…")
			ui.Button(c, "Revert").Width(72).OnClick(func() { m.removeFileTypeOverride(entry.Path) })
		})
	}
	if len(state.languages) > 0 {
		labels := make([]string, len(state.languages))
		languageLabel := ""
		for i, language := range state.languages {
			labels[i] = language.Name + " (" + language.ID + ")"
			if language.ID == state.overrideLang {
				languageLabel = labels[i]
			}
		}
		ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &state.overridePath).Grow(1).Placeholder("src/example.txt")
			if ui.Select(c, &languageLabel, labels).Width(190).Changed() {
				for i, label := range labels {
					if label == languageLabel {
						state.overrideLang = state.languages[i].ID
						break
					}
				}
			}
			ui.Button(c, "Choose…").Width(78).OnClick(m.chooseFileTypeOverrideFile)
			ui.Button(c, "Override").
				Disabled(strings.TrimSpace(state.overridePath) == "" || state.overrideLang == "").
				OnClick(func() { m.addFileTypeOverride() })
		})
	}

	ui.Row(c).Height(14)
	m.settingsSectionTitle(c, oi, "Ignored Files and Folders")
	ui.Text(c, "Names hidden from the project tree, navigation, search, indexing and Git changes.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Text(c, "Ignore files and folders").FontSize(12).Bold().TextColor(oi.text).SingleLine()
	settingsEditableStringList(c, oi, state.draft.IgnoredNames, ".DS_Store", func(index int) {
		state.draft.IgnoredNames = removeSettingString(state.draft.IgnoredNames, index)
		m.invalidate()
	})
	ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &state.ignoredDraft).Grow(1).Placeholder("*.tmp")
		ui.Button(c, "Add").Width(56).OnClick(func() {
			state.draft.IgnoredNames = appendSettingString(state.draft.IgnoredNames, &state.ignoredDraft)
			m.invalidate()
		})
	})

	ui.Row(c).Height(14)
	m.settingsSectionTitle(c, oi, "Exclude patterns")
	ui.Text(c, "Glob patterns hidden from the project tree and skipped by search and indexing.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	settingsEditableStringList(c, oi, state.draft.Exclude, "**/dist", func(index int) {
		state.draft.Exclude = removeSettingString(state.draft.Exclude, index)
		m.invalidate()
	})
	ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &state.excludeDraft).Grow(1).Placeholder("**/generated")
		ui.Button(c, "Add").Width(56).OnClick(func() {
			state.draft.Exclude = appendSettingString(state.draft.Exclude, &state.excludeDraft)
			m.invalidate()
		})
	})
}

func appendSettingString(values []string, draft *string) []string {
	value := strings.TrimSpace(*draft)
	*draft = ""
	if value == "" || containsStringFold(values, value) {
		return values
	}
	return append(values, value)
}

func removeSettingString(values []string, index int) []string {
	if index < 0 || index >= len(values) {
		return values
	}
	return append(values[:index:index], values[index+1:]...)
}

func (m *Model) filteredFileTypeLanguages() []openIDEALanguageInfo {
	state := &m.fileTypesSettings
	query := strings.ToLower(strings.TrimSpace(state.filter))
	if query == "" {
		return state.languages
	}
	var out []openIDEALanguageInfo
	for _, language := range state.languages {
		if strings.Contains(strings.ToLower(language.Name), query) ||
			strings.Contains(strings.ToLower(language.ID), query) {
			out = append(out, language)
			continue
		}
		for _, pattern := range fileTypeEffectivePatterns(language, state.draft) {
			if strings.Contains(strings.ToLower(pattern), query) {
				out = append(out, language)
				break
			}
		}
	}
	return out
}

func (m *Model) addFileTypeHashbang(id string) {
	state := &m.fileTypesSettings
	value := strings.TrimSpace(state.hashbangDraft)
	if id == "" || value == "" {
		return
	}
	assoc := cloneFileTypeAssociation(state.draft.FileTypes[id])
	if !containsStringFold(assoc.Hashbangs, value) {
		assoc.Hashbangs = append(assoc.Hashbangs, value)
	}
	state.draft.FileTypes[id] = assoc
	state.hashbangDraft = ""
	m.invalidate()
}

func (m *Model) removeFileTypeHashbang(id string, index int) {
	state := &m.fileTypesSettings
	assoc := cloneFileTypeAssociation(state.draft.FileTypes[id])
	if index < 0 || index >= len(assoc.Hashbangs) {
		return
	}
	assoc.Hashbangs = append(assoc.Hashbangs[:index:index], assoc.Hashbangs[index+1:]...)
	if len(assoc.Patterns) == 0 && len(assoc.RemovedPatterns) == 0 && len(assoc.Hashbangs) == 0 {
		delete(state.draft.FileTypes, id)
	} else {
		state.draft.FileTypes[id] = assoc
	}
	m.invalidate()
}

func (m *Model) addAssociatedApplication() {
	state := &m.fileTypesSettings
	value := strings.TrimSpace(state.associatedDraft)
	if value == "" || containsStringFold(state.draft.AssociatedApplications, value) {
		state.associatedDraft = ""
		return
	}
	state.draft.AssociatedApplications = append(state.draft.AssociatedApplications, value)
	state.associatedDraft = ""
	m.invalidate()
}

type fileTypeOverrideEntry struct {
	Path     string
	Language string
}

func fileTypeOverrideKey(path, root string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	native := filepath.FromSlash(path)
	if root != "" && filepath.IsAbs(native) {
		if relative, err := filepath.Rel(root, native); err == nil && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			native = relative
		}
	}
	cleaned := filepath.Clean(native)
	if cleaned == "." {
		return ""
	}
	return filepath.ToSlash(cleaned)
}

func (m *Model) fileTypeLanguageLabel(id string) string {
	for _, language := range m.fileTypesSettings.languages {
		if language.ID == id {
			return language.Name + " (" + language.ID + ")"
		}
	}
	if id == "" {
		return "Unknown"
	}
	return id
}

func (m *Model) fileTypeOverrideEntries() []fileTypeOverrideEntry {
	overrides := m.fileTypesSettings.draft.FileTypeOverrides
	out := make([]fileTypeOverrideEntry, 0, len(overrides))
	for path, id := range overrides {
		out = append(out, fileTypeOverrideEntry{Path: path, Language: m.fileTypeLanguageLabel(id)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (m *Model) chooseFileTypeOverrideFile() {
	state := &m.fileTypesSettings
	root := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: m.win, Title: "Override File Type", ButtonLabel: "Choose",
			DefaultPath: root, ShowHiddenFiles: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			state.overridePath = fileTypeOverrideKey(paths[0], root)
			m.invalidate()
		})
	}()
}

func (m *Model) addFileTypeOverride() {
	state := &m.fileTypesSettings
	key := fileTypeOverrideKey(state.overridePath, m.Root())
	if key == "" || state.overrideLang == "" {
		return
	}
	for _, language := range state.languages {
		if language.ID == state.overrideLang {
			state.draft.FileTypeOverrides[key] = language.ID
			state.overridePath = ""
			m.invalidate()
			return
		}
	}
}

func (m *Model) removeFileTypeOverride(path string) {
	delete(m.fileTypesSettings.draft.FileTypeOverrides, path)
	m.invalidate()
}

func (m *Model) settingsNaturalLanguagesBody(c *ui.Context, oi oiColors) {
	m.settingsSectionTitle(c, oi, "Languages")
	ui.Row(c).MinHeight(settingsFormRow).Gap(12).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "English (USA, UK)", 220)
		ui.Text(c, "Used by Proofreading › Typo").FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
	ui.Text(c, "Bundled Hunspell dictionaries (SCOWL en_US and en_GB) and common programming terms.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(720)
}

func (m *Model) settingsSpellingBody(c *ui.Context, oi oiColors) {
	state := &m.spellingSettings
	settingsBackendStatus(c, oi, state.loading, state.err, "spelling settings", m.loadSpellingSettings)
	if !state.loaded {
		return
	}
	m.settingsSectionTitle(c, oi, "Accepted words")
	if m.Root() != "" {
		ui.Text(c, "Project dictionary").FontSize(12).Bold().TextColor(oi.text).SingleLine()
		ui.Text(c, "Stored with the project.").FontSize(11).TextColor(oi.textMuted).SingleLine()
		settingsEditableStringList(c, oi, state.projectDraft.ProjectWords, "word", func(index int) {
			m.removeSpellingWord("project", index)
		})
		ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &state.projectWordDraft).Grow(1).Placeholder("word")
			ui.Button(c, "Add").Width(56).OnClick(func() { m.addSpellingWord("project") })
		})
	}
	ui.Text(c, "Application dictionary").FontSize(12).Bold().TextColor(oi.text).SingleLine()
	ui.Text(c, "Accepted in every project.").FontSize(11).TextColor(oi.textMuted).SingleLine()
	settingsEditableStringList(c, oi, state.globalDraft.IDEWords, "word", func(index int) {
		m.removeSpellingWord("global", index)
	})
	ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &state.ideWordDraft).Grow(1).Placeholder("word")
		ui.Button(c, "Add").Width(56).OnClick(func() { m.addSpellingWord("global") })
	})

	m.settingsSectionTitle(c, oi, "Custom dictionaries")
	ui.Text(c, "Plain-text word lists or Hunspell .dic files. Relative paths resolve against the project root.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	settingsEditableStringList(c, oi, state.globalDraft.CustomDictionaries, "~/dictionaries/team.dic", func(index int) {
		m.removeSpellingDictionary(index)
	})
	ui.Row(c).MinHeight(settingsFormRow).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &state.dictionaryDraft).Grow(1).Placeholder("~/dictionaries/team.dic")
		ui.Button(c, "Add").Width(56).OnClick(m.addSpellingDictionary)
	})
}

func (m *Model) addSpellingWord(scope string) {
	state := &m.spellingSettings
	value := strings.TrimSpace(state.ideWordDraft)
	target := &state.globalDraft.IDEWords
	if scope == "project" {
		value = strings.TrimSpace(state.projectWordDraft)
		target = &state.projectDraft.ProjectWords
	}
	if value == "" {
		return
	}
	if !containsStringFold(*target, value) {
		*target = append(*target, value)
	}
	if scope == "project" {
		state.projectWordDraft = ""
	} else {
		state.ideWordDraft = ""
	}
	m.invalidate()
}

func (m *Model) removeSpellingWord(scope string, index int) {
	state := &m.spellingSettings
	target := &state.globalDraft.IDEWords
	if scope == "project" {
		target = &state.projectDraft.ProjectWords
	}
	if index < 0 || index >= len(*target) {
		return
	}
	*target = append((*target)[:index:index], (*target)[index+1:]...)
	m.invalidate()
}

func (m *Model) addSpellingDictionary() {
	state := &m.spellingSettings
	value := strings.TrimSpace(state.dictionaryDraft)
	if value == "" || containsStringFold(state.globalDraft.CustomDictionaries, value) {
		state.dictionaryDraft = ""
		return
	}
	state.globalDraft.CustomDictionaries = append(state.globalDraft.CustomDictionaries, value)
	state.dictionaryDraft = ""
	m.invalidate()
}

func (m *Model) removeSpellingDictionary(index int) {
	state := &m.spellingSettings
	values := state.globalDraft.CustomDictionaries
	if index < 0 || index >= len(values) {
		return
	}
	state.globalDraft.CustomDictionaries = append(values[:index:index], values[index+1:]...)
	m.invalidate()
}

func (m *Model) settingsFormatterBody(c *ui.Context, oi oiColors, page string) {
	state := &m.formatterSettings
	settingsBackendScopeText(c, oi, state.scope)
	settingsBackendStatus(c, oi, state.loading, state.err, formatterToolName(state.tool)+" settings", func() {
		m.loadFormatterSettings(page)
	})
	if !state.loaded || state.page != page {
		return
	}
	name := formatterToolName(state.tool)
	m.settingsSectionTitle(c, oi, name)
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, name+" path", settingsLabelMaxWidth)
		ui.TextInput(c, &state.draft.Path).Grow(1).MaxWidth(settingsPathMaxWidth).
			Placeholder("Auto")
		ui.Button(c, "Browse…").OnClick(func() { m.browseFormatterPath(false) })
	})
	if state.tool == "ruff" {
		ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, "Configuration directory", settingsLabelMaxWidth)
			ui.TextInput(c, &state.draft.ConfigurationDirectory).Grow(1).
				MaxWidth(settingsPathMaxWidth).Placeholder("Project configuration")
			ui.Button(c, "Browse…").OnClick(func() { m.browseFormatterPath(true) })
		})
		ui.Text(c, "Empty uses the nearest pyproject.toml ([tool.ruff]), ruff.toml or .ruff.toml.").
			FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	}
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Formatting provider", settingsLabelMaxWidth)
		shown := "Automatic (language server first)"
		if state.draft.Prefer {
			shown = name
		}
		choices := []string{"Automatic (language server first)", name}
		if ui.Select(c, &shown, choices).Width(260).Changed() {
			state.draft.Prefer = shown == name
			m.invalidate()
		}
	})
	runOnSave := state.draft.RunOnSave
	if settingsCheckbox(c, &runOnSave, "Run on save").Changed() {
		state.draft.RunOnSave = runOnSave
		m.invalidate()
	}
	ui.Text(c, "Also enable Reformat code on save in Editor › General.").
		FontSize(11).TextColor(oi.textMuted).SingleLine()

	m.settingsSectionTitle(c, oi, "Enabled languages")
	languages := formatterLanguageChoices(state.tool, state.draft.Languages)
	for _, language := range languages {
		language := language
		on := containsStringFold(state.draft.Languages, language)
		if settingsCheckbox(c, &on, formatterLanguageLabel(language)).Changed() {
			m.setFormatterLanguage(language, on)
		}
	}
}

func formatterLanguageChoices(tool string, current []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range append(formatterDefaultSettings(tool).Languages, current...) {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func formatterLanguageLabel(id string) string {
	names := map[string]string{
		"markdown": "Markdown", "yaml": "YAML", "css": "CSS", "scss": "SCSS",
		"less": "Less", "html": "HTML", "graphql": "GraphQL", "javascript": "JavaScript",
		"javascriptreact": "JavaScript JSX", "typescript": "TypeScript",
		"typescriptreact": "TypeScript JSX", "json": "JSON", "jsonc": "JSON with Comments",
		"vue": "Vue", "python": "Python",
	}
	if label := names[id]; label != "" {
		return label
	}
	return id
}

func (m *Model) setFormatterLanguage(id string, enabled bool) {
	state := &m.formatterSettings
	for i, value := range state.draft.Languages {
		if value == id {
			if !enabled {
				state.draft.Languages = append(state.draft.Languages[:i:i], state.draft.Languages[i+1:]...)
			}
			m.invalidate()
			return
		}
	}
	if enabled {
		state.draft.Languages = append(state.draft.Languages, id)
	}
	m.invalidate()
}

func (m *Model) browseFormatterPath(directory bool) {
	state := &m.formatterSettings
	current := state.draft.Path
	title := "Select " + formatterToolName(state.tool)
	if directory {
		current = state.draft.ConfigurationDirectory
		title = "Select Ruff Configuration Directory"
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: m.win, Title: title, ButtonLabel: "Choose", DefaultPath: current, Directory: directory,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			path := filepath.Clean(paths[0])
			if directory {
				state.draft.ConfigurationDirectory = path
			} else {
				state.draft.Path = path
			}
			m.invalidate()
		})
	}()
}

func inspectionProfileExists(settings inspectionsSettings, profile string, hasProject bool) bool {
	if profile == "default" {
		return true
	}
	if profile == "project" {
		return hasProject
	}
	if level, key, ok := strings.Cut(profile, ":"); ok && key != "" {
		switch level {
		case "ide":
			_, exists := settings.IDEProfiles[key]
			return exists
		case "project":
			_, exists := settings.ProjectProfiles[key]
			return exists && hasProject
		}
	}
	return false
}

func (m *Model) combinedInspectionDraft() inspectionsSettings {
	state := &m.inspectionsSettings
	combined := state.projectDraft
	if m.Root() == "" {
		return state.globalDraft
	}
	combined.Default = state.globalDraft.Default
	combined.IDEProfiles = state.globalDraft.IDEProfiles
	return combined
}

type inspectionProfileEntry struct {
	ID    string
	Label string
	Level string
}

type inspectionProfileLocation struct {
	Level string
	Key   string
}

func parseInspectionProfile(profile string) inspectionProfileLocation {
	switch profile {
	case "project":
		return inspectionProfileLocation{Level: "project"}
	case "default":
		return inspectionProfileLocation{Level: "ide"}
	}
	if level, key, ok := strings.Cut(profile, ":"); ok &&
		(level == "ide" || level == "project") && key != "" {
		return inspectionProfileLocation{Level: level, Key: key}
	}
	return inspectionProfileLocation{Level: "ide"}
}

func inspectionProfileLabel(settings inspectionsSettings, profile string) string {
	location := parseInspectionProfile(profile)
	if location.Key == "" {
		if profile == "project" {
			return "Project Default"
		}
		return "Default"
	}
	if location.Level == "project" {
		if custom, ok := settings.ProjectProfiles[location.Key]; ok {
			return custom.Name
		}
	} else if custom, ok := settings.IDEProfiles[location.Key]; ok {
		return custom.Name
	}
	return profile
}

func inspectionProfileEntries(settings inspectionsSettings, hasProject bool) []inspectionProfileEntry {
	var out []inspectionProfileEntry
	if hasProject {
		out = append(out, inspectionProfileEntry{ID: "project", Label: "Project Default", Level: "project"})
		for key, custom := range settings.ProjectProfiles {
			out = append(out, inspectionProfileEntry{ID: "project:" + key, Label: custom.Name, Level: "project"})
		}
		sort.Slice(out[1:], func(i, j int) bool {
			return inspectionProfileEntryLess(out[i+1], out[j+1])
		})
	}
	out = append(out, inspectionProfileEntry{ID: "default", Label: "Default", Level: "ide"})
	var ide []inspectionProfileEntry
	for key, custom := range settings.IDEProfiles {
		ide = append(ide, inspectionProfileEntry{ID: "ide:" + key, Label: custom.Name, Level: "ide"})
	}
	sort.Slice(ide, func(i, j int) bool { return inspectionProfileEntryLess(ide[i], ide[j]) })
	return append(out, ide...)
}

func inspectionProfileEntryLess(a, b inspectionProfileEntry) bool {
	aLabel := strings.ToLower(a.Label)
	bLabel := strings.ToLower(b.Label)
	if aLabel != bLabel {
		return aLabel < bLabel
	}
	return a.ID < b.ID
}

func validateInspectionProfileName(
	settings inspectionsSettings,
	name string,
	level string,
	ownID string,
) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Profile name cannot be empty"
	}
	for _, profile := range inspectionProfileEntries(settings, true) {
		if profile.Level == level && profile.ID != ownID && profile.Label == name {
			return fmt.Sprintf("A profile named '%s' already exists", name)
		}
	}
	return ""
}

func copyInspectionProfileName(settings inspectionsSettings, profile, level string) string {
	base := inspectionProfileLabel(settings, profile) + " copy"
	if validateInspectionProfileName(settings, base, level, "") == "" {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s %d", base, index)
		if validateInspectionProfileName(settings, candidate, level, "") == "" {
			return candidate
		}
	}
}

func inspectionProfileKey(name string, existing map[string]customInspectionProfile) string {
	var key strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			key.WriteRune(r)
			lastDash = false
		} else if key.Len() > 0 && !lastDash {
			key.WriteByte('-')
			lastDash = true
		}
	}
	base := strings.Trim(key.String(), "-")
	if base == "" {
		base = "profile"
	}
	if _, exists := existing[base]; !exists {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%d", base, index)
		if _, exists := existing[candidate]; !exists {
			return candidate
		}
	}
}

func inspectionOverridesFor(settings inspectionsSettings, profile string) map[string]inspectionOverrideSettings {
	location := parseInspectionProfile(profile)
	if location.Key == "" {
		if profile == "project" {
			return settings.Project
		}
		return settings.Default
	}
	if location.Level == "project" {
		return settings.ProjectProfiles[location.Key].Overrides
	}
	return settings.IDEProfiles[location.Key].Overrides
}

func (m *Model) activeInspectionProfile() string {
	combined := m.combinedInspectionDraft()
	if inspectionProfileExists(combined, combined.Profile, m.Root() != "") {
		return combined.Profile
	}
	return "default"
}

func (m *Model) activeInspectionOverrides() map[string]inspectionOverrideSettings {
	return inspectionOverridesFor(m.combinedInspectionDraft(), m.activeInspectionProfile())
}

func (m *Model) setActiveInspectionOverrides(overrides map[string]inspectionOverrideSettings) {
	profile := m.activeInspectionProfile()
	state := &m.inspectionsSettings
	switch profile {
	case "project":
		state.projectDraft.Project = overrides
	case "default":
		state.globalDraft.Default = overrides
	default:
		level, key, _ := strings.Cut(profile, ":")
		if level == "project" {
			custom := state.projectDraft.ProjectProfiles[key]
			custom.Overrides = overrides
			state.projectDraft.ProjectProfiles[key] = custom
		} else {
			custom := state.globalDraft.IDEProfiles[key]
			custom.Overrides = overrides
			state.globalDraft.IDEProfiles[key] = custom
		}
	}
	m.invalidate()
}

func (m *Model) setActiveInspectionProfile(profile string) {
	if m.Root() == "" {
		m.inspectionsSettings.globalDraft.Profile = profile
		return
	}
	m.inspectionsSettings.projectDraft.Profile = profile
}

func (m *Model) copyInspectionProfile(profile, name, level string) error {
	if level != "ide" && level != "project" {
		return fmt.Errorf("unknown inspection profile level %q", level)
	}
	if level == "project" && m.Root() == "" {
		return fmt.Errorf("a project is required for a project profile")
	}
	combined := m.combinedInspectionDraft()
	name = strings.TrimSpace(name)
	if err := validateInspectionProfileName(combined, name, level, ""); err != "" {
		return fmt.Errorf("%s", err)
	}
	custom := customInspectionProfile{
		Name:      name,
		Overrides: cloneInspectionOverrides(inspectionOverridesFor(combined, profile)),
	}
	state := &m.inspectionsSettings
	var key string
	if level == "project" {
		if state.projectDraft.ProjectProfiles == nil {
			state.projectDraft.ProjectProfiles = map[string]customInspectionProfile{}
		}
		key = inspectionProfileKey(name, state.projectDraft.ProjectProfiles)
		state.projectDraft.ProjectProfiles[key] = custom
	} else {
		if state.globalDraft.IDEProfiles == nil {
			state.globalDraft.IDEProfiles = map[string]customInspectionProfile{}
		}
		key = inspectionProfileKey(name, state.globalDraft.IDEProfiles)
		state.globalDraft.IDEProfiles[key] = custom
	}
	m.setActiveInspectionProfile(level + ":" + key)
	m.invalidate()
	return nil
}

func (m *Model) renameInspectionProfile(profile, name string) error {
	location := parseInspectionProfile(profile)
	if location.Key == "" {
		return fmt.Errorf("default profiles cannot be renamed")
	}
	combined := m.combinedInspectionDraft()
	name = strings.TrimSpace(name)
	if err := validateInspectionProfileName(combined, name, location.Level, profile); err != "" {
		return fmt.Errorf("%s", err)
	}
	state := &m.inspectionsSettings
	if location.Level == "project" {
		custom, ok := state.projectDraft.ProjectProfiles[location.Key]
		if !ok {
			return fmt.Errorf("inspection profile does not exist")
		}
		custom.Name = name
		state.projectDraft.ProjectProfiles[location.Key] = custom
	} else {
		custom, ok := state.globalDraft.IDEProfiles[location.Key]
		if !ok {
			return fmt.Errorf("inspection profile does not exist")
		}
		custom.Name = name
		state.globalDraft.IDEProfiles[location.Key] = custom
	}
	m.invalidate()
	return nil
}

func (m *Model) deleteInspectionProfile(profile string) error {
	location := parseInspectionProfile(profile)
	if location.Key == "" {
		return fmt.Errorf("default profiles cannot be deleted")
	}
	wasActive := m.activeInspectionProfile() == profile
	state := &m.inspectionsSettings
	if location.Level == "project" {
		if _, ok := state.projectDraft.ProjectProfiles[location.Key]; !ok {
			return fmt.Errorf("inspection profile does not exist")
		}
		delete(state.projectDraft.ProjectProfiles, location.Key)
		if wasActive {
			m.setActiveInspectionProfile("project")
		}
	} else {
		if _, ok := state.globalDraft.IDEProfiles[location.Key]; !ok {
			return fmt.Errorf("inspection profile does not exist")
		}
		delete(state.globalDraft.IDEProfiles, location.Key)
		if wasActive {
			m.setActiveInspectionProfile("default")
		}
	}
	m.invalidate()
	return nil
}

func (m *Model) openInspectionProfilePrompt(mode, level string) {
	state := &m.inspectionsSettings
	profile := m.activeInspectionProfile()
	combined := m.combinedInspectionDraft()
	prompt := &inspectionProfilePrompt{
		Mode: mode, Level: level, Profile: profile,
	}
	switch mode {
	case "copy":
		prompt.Name = copyInspectionProfileName(combined, profile, level)
	case "rename":
		prompt.Name = inspectionProfileLabel(combined, profile)
	}
	state.profilePrompt = prompt
	m.invalidate()
}

func (m *Model) commitInspectionProfilePrompt() {
	prompt := m.inspectionsSettings.profilePrompt
	if prompt == nil {
		return
	}
	var err error
	switch prompt.Mode {
	case "copy":
		err = m.copyInspectionProfile(prompt.Profile, prompt.Name, prompt.Level)
	case "rename":
		err = m.renameInspectionProfile(prompt.Profile, prompt.Name)
	case "delete":
		err = m.deleteInspectionProfile(prompt.Profile)
	}
	if err != nil {
		prompt.Error = err.Error()
		m.invalidate()
		return
	}
	m.inspectionsSettings.profilePrompt = nil
	m.invalidate()
}

func (m *Model) inspectionProfileMenu(menu *ui.Menu) {
	profile := m.activeInspectionProfile()
	combined := m.combinedInspectionDraft()
	location := parseInspectionProfile(profile)
	custom := location.Key != ""
	otherLevel := "project"
	otherLabel := "Copy to Project…"
	if location.Level == "project" {
		otherLevel = "ide"
		otherLabel = "Copy to IDE…"
	}
	if menu.Item("Duplicate…").Chosen() {
		m.openInspectionProfilePrompt("copy", location.Level)
	}
	if menu.Item(otherLabel).Disabled(otherLevel == "project" && m.Root() == "").Chosen() {
		m.openInspectionProfilePrompt("copy", otherLevel)
	}
	if menu.Item("Rename…").Disabled(!custom).Chosen() {
		m.openInspectionProfilePrompt("rename", location.Level)
	}
	if menu.Item("Restore Defaults").
		Disabled(len(inspectionOverridesFor(combined, profile)) == 0).
		Chosen() {
		m.setActiveInspectionOverrides(map[string]inspectionOverrideSettings{})
	}
	if custom {
		menu.Separator()
		if menu.Item("Delete").Chosen() {
			m.openInspectionProfilePrompt("delete", location.Level)
		}
	}
}

type inspectionPageRow struct {
	id           string
	name         string
	group        string
	description  string
	defaultOn    bool
	defaultLevel string
	options      []inspectionOptionSpec
}

func (m *Model) inspectionRows() []inspectionPageRow {
	state := &m.inspectionsSettings
	seen := map[string]bool{}
	var out []inspectionPageRow
	for _, spec := range builtinInspectionCatalog {
		entry := inspectionPageRow{id: spec.ID, name: spec.Name, group: spec.Group,
			description: spec.Description, defaultOn: true, defaultLevel: spec.DefaultLevel, options: spec.Options}
		if spec.DefaultEnabled != nil {
			entry.defaultOn = *spec.DefaultEnabled
		}
		seen[entry.id] = true
		out = append(out, entry)
	}
	for _, language := range state.languages {
		for _, server := range language.LanguageServers {
			id := "lsp:" + server
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, inspectionPageRow{
				id: id, name: server, group: language.Name, defaultOn: true,
				description: "Problems reported by " + server + " in " + language.Name + " files.",
			})
		}
	}
	for _, overrides := range []map[string]inspectionOverrideSettings{state.globalDraft.Default, state.projectDraft.Project} {
		for id := range overrides {
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, inspectionPageRow{id: id, name: id, group: "Other", defaultOn: true})
		}
	}
	for _, profile := range []map[string]customInspectionProfile{state.globalDraft.IDEProfiles, state.projectDraft.ProjectProfiles} {
		for _, custom := range profile {
			for id := range custom.Overrides {
				if seen[id] {
					continue
				}
				seen[id] = true
				out = append(out, inspectionPageRow{id: id, name: id, group: "Other", defaultOn: true})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].group != out[j].group {
			return out[i].group < out[j].group
		}
		return out[i].name < out[j].name
	})
	return out
}

func inspectionSeverityLabel(value string) string {
	switch value {
	case "error":
		return "Error"
	case "warning":
		return "Warning"
	case "weakWarning":
		return "Weak Warning"
	case "serverProblem":
		return "Server Problem"
	case "typo":
		return "Typo"
	case "none":
		return "No highlighting"
	default:
		return "As reported"
	}
}

func inspectionSeverityValue(label string) string {
	switch label {
	case "Error":
		return "error"
	case "Warning":
		return "warning"
	case "Weak Warning":
		return "weakWarning"
	case "Server Problem":
		return "serverProblem"
	case "Typo":
		return "typo"
	case "No highlighting":
		return "none"
	default:
		return ""
	}
}

func (m *Model) settingsInspectionsBody(c *ui.Context, oi oiColors) {
	state := &m.inspectionsSettings
	settingsBackendStatus(c, oi, state.loading, state.err, "inspection profiles", m.loadInspectionsSettings)
	if !state.loaded {
		return
	}
	combined := m.combinedInspectionDraft()
	active := m.activeInspectionProfile()
	profiles := inspectionProfileEntries(combined, m.Root() != "")
	labels := make([]string, len(profiles))
	label := "Default"
	for i, profile := range profiles {
		labels[i] = profile.Label
		if parseInspectionProfile(profile.ID).Key != "" {
			if profile.Level == "project" {
				labels[i] += " (Project)"
			} else {
				labels[i] += " (IDE)"
			}
		}
		if profile.ID == active {
			label = labels[i]
		}
	}
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Profile", settingsLabelMaxWidth)
		if ui.Select(c, &label, labels).MinWidth(settingsSelectMinWidth).
			Grow(1).MaxWidth(settingsControlMaxWidth).Changed() {
			for i, option := range labels {
				if option == label {
					m.setActiveInspectionProfile(profiles[i].ID)
					break
				}
			}
			m.invalidate()
		}
		ui.Button(c, "Profile Actions").Menu(m.inspectionProfileMenu)
		location := parseInspectionProfile(active)
		stored := "IDE"
		if location.Level == "project" {
			stored = "Project"
		}
		ui.Text(c, "Stored in "+stored).FontSize(11).TextColor(oi.textMuted).SingleLine()
	})
	ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Search inspections", settingsLabelMaxWidth)
		ui.TextInput(c, &state.query).Grow(1).MaxWidth(settingsControlMaxWidth).
			Placeholder("Name, id or description")
	})
	overrides := m.activeInspectionOverrides()
	query := strings.ToLower(strings.TrimSpace(state.query))
	rows := m.inspectionRows()
	var shown []inspectionPageRow
	for _, row := range rows {
		hay := strings.ToLower(row.group + " " + row.name + " " + row.id + " " + row.description)
		if query == "" || strings.Contains(hay, query) {
			shown = append(shown, row)
		}
	}
	if len(shown) == 0 {
		ui.Text(c, "Nothing to show").FontSize(12).TextColor(oi.textMuted).SingleLine()
		return
	}
	for _, row := range shown {
		row := row
		override := overrides[row.id]
		on := row.defaultOn
		if override.Enabled != nil {
			on = *override.Enabled
		}
		ui.Row(c).MinHeight(settingsFormRow).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Checkbox(c, &on, "").Changed() {
				m.setInspectionEnabled(row, on)
			}
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, row.name).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
				ui.Text(c, row.group+" · "+row.id).FontSize(10).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			})
			ui.Text(c, "Severity").FontSize(11).TextColor(oi.textMuted).SingleLine()
			severity := inspectionSeverityLabel(override.Severity)
			choices := []string{"As reported", "Error", "Warning", "Weak Warning", "Server Problem", "Typo", "No highlighting"}
			if ui.Select(c, &severity, choices).Width(150).Disabled(!on).Changed() {
				override := overrides[row.id]
				override.Severity = inspectionSeverityValue(severity)
				if inspectionOverrideEmpty(override) {
					delete(overrides, row.id)
				} else {
					overrides[row.id] = override
				}
				m.setActiveInspectionOverrides(overrides)
			}
		})
		if len(row.options) > 0 {
			ui.Row(c).Gap(14).PaddingStart(30).Children(func() {
				for _, option := range row.options {
					value := option.Default
					if configured, ok := overrides[row.id].Options[option.Key]; ok {
						value = configured
					}
					if ui.Checkbox(c, &value, option.Label).Disabled(!on).Changed() {
						m.setInspectionOption(row, option, value)
					}
				}
			})
		}
	}
}

func (m *Model) inspectionProfilePromptError(prompt *inspectionProfilePrompt) string {
	if prompt == nil || (prompt.Mode != "copy" && prompt.Mode != "rename") {
		return ""
	}
	ownID := ""
	if prompt.Mode == "rename" {
		ownID = prompt.Profile
	}
	return validateInspectionProfileName(m.combinedInspectionDraft(), prompt.Name, prompt.Level, ownID)
}

func (m *Model) settingsInspectionProfileDialog(c *ui.Context, p *ui.Theme) {
	prompt := m.inspectionsSettings.profilePrompt
	if !m.settingsOpen || prompt == nil {
		return
	}
	oi := oiPaletteOf(p)
	open := true
	title := "Copy Profile"
	action := "Copy"
	if prompt.Mode == "rename" {
		title, action = "Rename Profile", "Rename"
	} else if prompt.Mode == "delete" {
		title, action = "Delete Profile", "Delete"
	}
	dlg := nativeModal(c.Key("settings:inspection-profile:"+prompt.Mode), &open, func() {
		ui.Column(c).Width(420).Padding(18).Gap(10).
			Background(oi.elev).Border(1, oi.border).Radius(6).
			Children(func() {
				ui.Text(c, title).FontSize(14).Bold().TextColor(oi.textStrong).SingleLine()
				combined := m.combinedInspectionDraft()
				profileLabel := inspectionProfileLabel(combined, prompt.Profile)
				if prompt.Mode == "delete" {
					ui.Text(c, "Delete inspection profile '"+profileLabel+"'?").
						FontSize(12).TextColor(oi.text).MaxWidth(380)
				} else {
					label := "Name"
					if prompt.Mode == "copy" {
						label = "Copy '" + profileLabel + "' as"
					}
					ui.Text(c, label).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
					input := ui.TextInput(c, &prompt.Name).Width(380).AutoFocus()
					input.OnChange(func() { prompt.Error = "" })
					if input.Submitted() && m.inspectionProfilePromptError(prompt) == "" {
						m.commitInspectionProfilePrompt()
						return
					}
				}
				if validation := m.inspectionProfilePromptError(prompt); validation != "" {
					ui.Text(c, validation).FontSize(12).TextColor(oi.danger).SingleLine().Ellipsis("…")
				}
				if prompt.Error != "" {
					ui.Text(c, prompt.Error).FontSize(12).TextColor(oi.danger).SingleLine().Ellipsis("…")
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					ui.Button(c, "Cancel").OnClick(func() {
						m.inspectionsSettings.profilePrompt = nil
						m.invalidate()
					})
					ui.Button(c, action).
						Disabled(m.inspectionProfilePromptError(prompt) != "").
						OnClick(m.commitInspectionProfilePrompt)
				})
			})
	})
	if dlg.Dismissed() || !open {
		m.inspectionsSettings.profilePrompt = nil
		m.invalidate()
	}
}

func inspectionOverrideEmpty(value inspectionOverrideSettings) bool {
	return value.Enabled == nil && value.Severity == "" && len(value.Options) == 0
}

func (m *Model) setInspectionEnabled(row inspectionPageRow, enabled bool) {
	overrides := cloneInspectionOverrides(m.activeInspectionOverrides())
	value := overrides[row.id]
	if enabled == row.defaultOn {
		value.Enabled = nil
	} else {
		value.Enabled = boolPointer(enabled)
	}
	if inspectionOverrideEmpty(value) {
		delete(overrides, row.id)
	} else {
		overrides[row.id] = value
	}
	m.setActiveInspectionOverrides(overrides)
}

func (m *Model) setInspectionOption(row inspectionPageRow, option inspectionOptionSpec, enabled bool) {
	overrides := cloneInspectionOverrides(m.activeInspectionOverrides())
	value := overrides[row.id]
	if value.Options == nil {
		value.Options = map[string]bool{}
	}
	if enabled == option.Default {
		delete(value.Options, option.Key)
	} else {
		value.Options[option.Key] = enabled
	}
	if len(value.Options) == 0 {
		value.Options = nil
	}
	if inspectionOverrideEmpty(value) {
		delete(overrides, row.id)
	} else {
		overrides[row.id] = value
	}
	m.setActiveInspectionOverrides(overrides)
}

func (m *Model) settingsRunTargetsBody(c *ui.Context, oi oiColors) {
	m.settingsSectionTitle(c, oi, "Run Targets")
	ui.Text(c, "Run applications on local machines, SSH hosts or Docker containers.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(720)
	ui.Text(c, "The native Run Targets editor configures SSH targets, project folders, interpreters and containers.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(720)
	ui.Row(c).MinHeight(36).Children(func() {
		ui.Button(c, "Open Run Targets").OnClick(m.OpenRunTargetSettings)
	})
}

func (m *Model) settingsDeploymentBody(c *ui.Context, oi oiColors) {
	m.settingsSectionTitle(c, oi, "Deployment")
	ui.Text(c, "Configure local, SFTP and FTP deployment servers, mappings and excluded paths.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(720)
	ui.Text(c, "The native Deployment editor tests connections and saves the project's deployment configuration.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(720)
	ui.Row(c).MinHeight(36).Children(func() {
		ui.Button(c, "Open Deployment").OnClick(m.OpenDeploymentSettings)
	})
}

func (m *Model) settingsSSHBody(c *ui.Context, oi oiColors) {
	m.settingsSectionTitle(c, oi, "SSH Configurations")
	ui.Text(c, "Manage SSH hosts used for remote development, deployment and run targets.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(720)
	ui.Text(c, "The native editor supports OpenSSH config, key pairs, passwords, passphrases and connection tests.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(720)
	ui.Row(c).MinHeight(36).Children(func() {
		ui.Button(c, "Open SSH Configurations").OnClick(m.openRemoteSSHSettings)
	})
}
