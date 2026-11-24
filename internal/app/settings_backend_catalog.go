package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

var defaultAssociatedApplications = []string{
	"*.xlsx", "*.xls", "*.docx", "*.doc", "*.pptx", "*.ppt", "*.pdf",
}

var defaultIgnoredNames = []string{
	"*.pyc", "*.pyo", "*.rbc", "*.yarb", "*~", ".DS_Store", ".git", ".hg", ".jj", ".svn",
	"CVS", "__pycache__", "_svn", "vssver.scc", "vssver2.scc", "Thumbs.db", "desktop.ini",
}

var defaultFileExcludes = []string{"**/.git", "**/node_modules", "**/target"}

type fileTypeAssociation struct {
	Patterns        []string `json:"patterns"`
	RemovedPatterns []string `json:"removedPatterns"`
	Hashbangs       []string `json:"hashbangs"`
}

type fileTypesSettings struct {
	FileTypes              map[string]fileTypeAssociation `json:"fileTypes"`
	FileTypeOverrides      map[string]string              `json:"fileTypeOverrides"`
	AssociatedApplications []string                       `json:"associatedApplications"`
	IgnoredNames           []string                       `json:"ignoredNames"`
	Exclude                []string                       `json:"exclude"`
}

type fileTypesSettingsState struct {
	loading         bool
	loaded          bool
	saving          bool
	session         bool
	scope           string
	applied         fileTypesSettings
	draft           fileTypesSettings
	filter          string
	selected        string
	languages       []openIDEALanguageInfo
	patternDraft    string
	hashbangDraft   string
	associatedDraft string
	ignoredDraft    string
	excludeDraft    string
	overridePath    string
	overrideLang    string
	err             string
	loadGen         uint64
	saveGen         uint64

	runtime       fileTypesSettings
	runtimeLoaded bool
	runtimeGen    uint64
}

type inspectionOverrideSettings struct {
	Enabled  *bool           `json:"enabled,omitempty"`
	Severity string          `json:"severity,omitempty"`
	Options  map[string]bool `json:"options,omitempty"`
}

type customInspectionProfile struct {
	Name      string                                `json:"name"`
	Overrides map[string]inspectionOverrideSettings `json:"overrides"`
}

type inspectionProfilePrompt struct {
	Mode    string
	Level   string
	Profile string
	Name    string
	Error   string
}

type inspectionsSettings struct {
	Profile         string                                `json:"profile"`
	Default         map[string]inspectionOverrideSettings `json:"default"`
	Project         map[string]inspectionOverrideSettings `json:"project"`
	IDEProfiles     map[string]customInspectionProfile    `json:"ideProfiles"`
	ProjectProfiles map[string]customInspectionProfile    `json:"projectProfiles"`
}

type inspectionsSettingsState struct {
	loading        bool
	loaded         bool
	saving         bool
	session        bool
	globalApplied  inspectionsSettings
	globalDraft    inspectionsSettings
	projectApplied inspectionsSettings
	projectDraft   inspectionsSettings
	languages      []openIDEALanguageInfo
	selected       string
	query          string
	profilePrompt  *inspectionProfilePrompt
	err            string
	loadGen        uint64
	saveGen        uint64
}

type spellingSettings struct {
	IDEWords           []string `json:"ideWords"`
	ProjectWords       []string `json:"projectWords"`
	CustomDictionaries []string `json:"customDictionaries"`
}

type spellingSettingsState struct {
	loading          bool
	loaded           bool
	saving           bool
	session          bool
	globalApplied    spellingSettings
	globalDraft      spellingSettings
	projectApplied   spellingSettings
	projectDraft     spellingSettings
	ideWordDraft     string
	projectWordDraft string
	dictionaryDraft  string
	err              string
	loadGen          uint64
	saveGen          uint64
}

type externalFormatterSettings struct {
	Path                   string   `json:"path"`
	ConfigurationDirectory string   `json:"configurationDirectory"`
	Languages              []string `json:"languages"`
	Prefer                 bool     `json:"prefer"`
	RunOnSave              bool     `json:"runOnSave"`
}

type formatterSettingsState struct {
	loading bool
	loaded  bool
	saving  bool
	session bool
	scope   string
	tool    string
	page    string
	applied externalFormatterSettings
	draft   externalFormatterSettings
	err     string
	loadGen uint64
	saveGen uint64
	pages   map[string]formatterPageState
}

type formatterPageState struct {
	page    string
	tool    string
	scope   string
	applied externalFormatterSettings
	draft   externalFormatterSettings
}

func (s *formatterSettingsState) cacheCurrent() {
	if s == nil || s.page == "" || !s.loaded {
		return
	}
	if s.pages == nil {
		s.pages = map[string]formatterPageState{}
	}
	s.pages[s.page] = formatterPageState{
		page: s.page, tool: s.tool, scope: s.scope,
		applied: cloneFormatterSettings(s.applied), draft: cloneFormatterSettings(s.draft),
	}
}

func (s *formatterSettingsState) restorePage(cached formatterPageState) {
	s.page, s.tool, s.scope = cached.page, cached.tool, cached.scope
	s.applied = cloneFormatterSettings(cached.applied)
	s.draft = cloneFormatterSettings(cached.draft)
	s.loading = false
	s.loaded = true
	s.err = ""
}

func (s *formatterSettingsState) allPages() []formatterPageState {
	s.cacheCurrent()
	pages := make([]formatterPageState, 0, len(s.pages))
	for _, page := range s.pages {
		pages = append(pages, formatterPageState{
			page: page.page, tool: page.tool, scope: page.scope,
			applied: cloneFormatterSettings(page.applied), draft: cloneFormatterSettings(page.draft),
		})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].page < pages[j].page })
	return pages
}

type inspectionOptionSpec struct {
	Key     string
	Label   string
	Default bool
}

type builtinInspectionSpec struct {
	ID             string
	Producer       string
	Name           string
	Group          string
	Description    string
	DefaultLevel   string
	DefaultEnabled *bool
	Options        []inspectionOptionSpec
}

var builtinInspectionCatalog = []builtinInspectionSpec{
	{
		ID: "builtin:merge-conflict-markers", Producer: "merge-conflict-markers",
		Name: "Unresolved merge conflict markers", Group: "Version control",
		Description:  "Reports conflict marker lines left in a file by a merge.",
		DefaultLevel: "error",
	},
	{
		ID: "builtin:dependency-versions", Producer: "dependency-versions",
		Name: "Newer dependency version available", Group: "Cargo",
		Description:  "Reports newer crate versions that Cargo.toml requirements do not allow.",
		DefaultLevel: "weakWarning",
	},
	{
		ID: "builtin:syntax", Producer: "syntax", Name: "Syntax errors", Group: "General",
		Description: "Reports syntax errors when no language server checks the file.", DefaultLevel: "error",
	},
	{
		ID: "builtin:file-references", Producer: "file-references",
		Name: "Unresolved file references", Group: "General",
		Description: "Reports paths in HTML attributes, CSS url() and @import that do not exist.",
	},
	{
		ID: "builtin:build", Producer: "build", Name: "Build problems", Group: "General",
		Description: "Shows compiler errors and warnings at their source positions.",
	},
	{
		ID: "java:missing-serial-version", Producer: "jdt-serial-version",
		Name: "Serializable class without 'serialVersionUID'", Group: "Java",
		Description:  "Reports Serializable classes without a serialVersionUID field.",
		DefaultLevel: "warning", DefaultEnabled: boolPointer(false),
	},
	{
		ID: "builtin:phpstan", Producer: "phpstan", Name: "PHPStan", Group: "PHP",
		Description:  "Reports issues found by the project's PHPStan configuration.",
		DefaultLevel: "warning",
	},
	{
		ID: "builtin:sql", Producer: "sql-inspections",
		Name: "Unresolved database references", Group: "SQL",
		Description: "Reports tables, columns and aliases missing from the data source schema.",
	},
	{
		ID: "builtin:spelling", Producer: "spelling", Name: "Typo", Group: "Proofreading",
		Description:  "Reports misspelled words in comments, literals, names and prose.",
		DefaultLevel: "typo",
		Options: []inspectionOptionSpec{
			{Key: "processCode", Label: "Process code", Default: true},
			{Key: "processLiterals", Label: "Process literals", Default: true},
			{Key: "processComments", Label: "Process comments", Default: true},
		},
	},
}

type settingsBackendWrite struct {
	scope string
	key   string
	value any
}

func cloneStrings(in []string) []string {
	return append([]string(nil), in...)
}

func normalizeSettingStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cloneFileTypeAssociation(in fileTypeAssociation) fileTypeAssociation {
	return fileTypeAssociation{
		Patterns: cloneStrings(in.Patterns), RemovedPatterns: cloneStrings(in.RemovedPatterns),
		Hashbangs: cloneStrings(in.Hashbangs),
	}
}

func cloneFileTypesSettings(in fileTypesSettings) fileTypesSettings {
	out := fileTypesSettings{
		FileTypes: map[string]fileTypeAssociation{}, FileTypeOverrides: map[string]string{},
		AssociatedApplications: cloneStrings(in.AssociatedApplications),
		IgnoredNames:           cloneStrings(in.IgnoredNames),
		Exclude:                cloneStrings(in.Exclude),
	}
	for id, assoc := range in.FileTypes {
		out.FileTypes[id] = cloneFileTypeAssociation(assoc)
	}
	for path, id := range in.FileTypeOverrides {
		out.FileTypeOverrides[path] = id
	}
	return out
}

func sameFileTypeAssociation(a, b fileTypeAssociation) bool {
	return sameStrings(a.Patterns, b.Patterns) &&
		sameStrings(a.RemovedPatterns, b.RemovedPatterns) &&
		sameStrings(a.Hashbangs, b.Hashbangs)
}

func sameFileTypesSettings(a, b fileTypesSettings) bool {
	if !sameStrings(a.AssociatedApplications, b.AssociatedApplications) ||
		!sameStrings(a.IgnoredNames, b.IgnoredNames) ||
		!sameStrings(a.Exclude, b.Exclude) ||
		len(a.FileTypes) != len(b.FileTypes) || len(a.FileTypeOverrides) != len(b.FileTypeOverrides) {
		return false
	}
	for id, assoc := range a.FileTypes {
		if other, ok := b.FileTypes[id]; !ok || !sameFileTypeAssociation(assoc, other) {
			return false
		}
	}
	for path, id := range a.FileTypeOverrides {
		if b.FileTypeOverrides[path] != id {
			return false
		}
	}
	return true
}

func cloneInspectionOverride(in inspectionOverrideSettings) inspectionOverrideSettings {
	out := inspectionOverrideSettings{Severity: in.Severity}
	if in.Enabled != nil {
		value := *in.Enabled
		out.Enabled = &value
	}
	if len(in.Options) > 0 {
		out.Options = map[string]bool{}
		for key, value := range in.Options {
			out.Options[key] = value
		}
	}
	return out
}

func cloneInspectionOverrides(in map[string]inspectionOverrideSettings) map[string]inspectionOverrideSettings {
	out := make(map[string]inspectionOverrideSettings, len(in))
	for id, value := range in {
		out[id] = cloneInspectionOverride(value)
	}
	return out
}

func cloneInspectionProfiles(in map[string]customInspectionProfile) map[string]customInspectionProfile {
	out := make(map[string]customInspectionProfile, len(in))
	for key, profile := range in {
		out[key] = customInspectionProfile{
			Name: profile.Name, Overrides: cloneInspectionOverrides(profile.Overrides),
		}
	}
	return out
}

func cloneInspectionSettings(in inspectionsSettings) inspectionsSettings {
	return inspectionsSettings{
		Profile: in.Profile, Default: cloneInspectionOverrides(in.Default),
		Project:         cloneInspectionOverrides(in.Project),
		IDEProfiles:     cloneInspectionProfiles(in.IDEProfiles),
		ProjectProfiles: cloneInspectionProfiles(in.ProjectProfiles),
	}
}

func sameInspectionOverride(a, b inspectionOverrideSettings) bool {
	if a.Severity != b.Severity || (a.Enabled == nil) != (b.Enabled == nil) ||
		(a.Enabled != nil && *a.Enabled != *b.Enabled) || len(a.Options) != len(b.Options) {
		return false
	}
	for key, value := range a.Options {
		if b.Options[key] != value {
			return false
		}
	}
	return true
}

func sameInspectionOverrides(a, b map[string]inspectionOverrideSettings) bool {
	if len(a) != len(b) {
		return false
	}
	for id, value := range a {
		if other, ok := b[id]; !ok || !sameInspectionOverride(value, other) {
			return false
		}
	}
	return true
}

func sameInspectionProfiles(a, b map[string]customInspectionProfile) bool {
	if len(a) != len(b) {
		return false
	}
	for key, profile := range a {
		other, ok := b[key]
		if !ok || profile.Name != other.Name || !sameInspectionOverrides(profile.Overrides, other.Overrides) {
			return false
		}
	}
	return true
}

func sameInspectionSettings(a, b inspectionsSettings) bool {
	return a.Profile == b.Profile && sameInspectionOverrides(a.Default, b.Default) &&
		sameInspectionOverrides(a.Project, b.Project) &&
		sameInspectionProfiles(a.IDEProfiles, b.IDEProfiles) &&
		sameInspectionProfiles(a.ProjectProfiles, b.ProjectProfiles)
}

func cloneSpellingSettings(in spellingSettings) spellingSettings {
	return spellingSettings{
		IDEWords: cloneStrings(in.IDEWords), ProjectWords: cloneStrings(in.ProjectWords),
		CustomDictionaries: cloneStrings(in.CustomDictionaries),
	}
}

func sameSpellingSettings(a, b spellingSettings) bool {
	return sameStrings(a.IDEWords, b.IDEWords) && sameStrings(a.ProjectWords, b.ProjectWords) &&
		sameStrings(a.CustomDictionaries, b.CustomDictionaries)
}

func cloneFormatterSettings(in externalFormatterSettings) externalFormatterSettings {
	in.Languages = cloneStrings(in.Languages)
	return in
}

func sameFormatterSettings(a, b externalFormatterSettings) bool {
	return a.Path == b.Path && a.ConfigurationDirectory == b.ConfigurationDirectory &&
		a.Prefer == b.Prefer && a.RunOnSave == b.RunOnSave &&
		sameStrings(a.Languages, b.Languages)
}

func formatterDefaultSettings(tool string) externalFormatterSettings {
	if tool == "ruff" {
		return externalFormatterSettings{Languages: []string{"python"}}
	}
	return externalFormatterSettings{Languages: []string{
		"markdown", "yaml", "css", "scss", "less", "html", "graphql",
		"javascript", "javascriptreact", "typescript", "typescriptreact",
		"json", "jsonc", "vue",
	}}
}

func formatterToolForPage(page string) (tool string, ok bool) {
	switch page {
	case settingsPagePrettier:
		return "prettier", true
	case settingsPageOxfmt:
		return "oxfmt", true
	case settingsPageRuff:
		return "ruff", true
	default:
		return "", false
	}
}

func (m *Model) loadBackendCatalogSettingsPage(page string) {
	switch page {
	case settingsPageFileTypes:
		m.loadFileTypesSettings()
	case settingsPageInspections:
		m.loadInspectionsSettings()
	case settingsPageSpelling:
		m.loadSpellingSettings()
	case settingsPagePrettier, settingsPageOxfmt, settingsPageRuff:
		m.loadFormatterSettings(page)
	default:
		if nativeSettingsPageLoadable(page) {
			m.loadNativeSettingsPage(page)
		}
	}
}

func decodeFileTypesSettings(raw json.RawMessage) (fileTypesSettings, error) {
	var envelope struct {
		Files fileTypesSettings `json:"files"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fileTypesSettings{}, err
	}
	settings := envelope.Files
	if settings.FileTypes == nil {
		settings.FileTypes = map[string]fileTypeAssociation{}
	}
	if settings.FileTypeOverrides == nil {
		settings.FileTypeOverrides = map[string]string{}
	}
	if settings.AssociatedApplications == nil {
		settings.AssociatedApplications = cloneStrings(defaultAssociatedApplications)
	}
	if settings.IgnoredNames == nil {
		settings.IgnoredNames = cloneStrings(defaultIgnoredNames)
	}
	if settings.Exclude == nil {
		settings.Exclude = cloneStrings(defaultFileExcludes)
	}
	return settings, nil
}

func (m *Model) settingsBackendScope() string {
	if m.Root() != "" {
		return "project"
	}
	return "global"
}

func (m *Model) invokeSettingsScope(ctx context.Context, scope string) (json.RawMessage, error) {
	command := "settings_get"
	if scope == "global" {
		command = "settings_get_global"
	}
	return m.invoke(ctx, command, nil)
}

func (m *Model) loadFileTypesSettings() {
	state := &m.fileTypesSettings
	if state.session && (state.loading || state.loaded) {
		return
	}
	state.session = true
	state.scope = m.settingsBackendScope()
	state.loading = true
	state.loaded = false
	state.err = ""
	state.loadGen++
	generation := state.loadGen
	root := m.Root()
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invokeSettingsScope(ctx, state.scope)
		var settings fileTypesSettings
		if err == nil {
			settings, err = decodeFileTypesSettings(raw)
		}
		var languages []openIDEALanguageInfo
		if err == nil {
			languages, err = m.toolLanguages(ctx)
		}
		m.post(func() {
			if state.loadGen != generation || state.scope != m.settingsBackendScope() ||
				(state.scope == "project" && m.Root() != root) {
				return
			}
			state.loading = false
			state.loaded = err == nil
			if err != nil {
				state.err = "Could not read File Types: " + err.Error()
				m.invalidate()
				return
			}
			state.applied = cloneFileTypesSettings(settings)
			state.draft = cloneFileTypesSettings(settings)
			state.runtime = cloneFileTypesSettings(settings)
			state.runtimeLoaded = true
			state.languages = languages
			m.ensureFileTypeSelection()
			m.applyFileTypeRuntime()
			m.invalidate()
		})
	}()
}

// openIDEALanguageInfo mirrors the subset of the backend languages reply the
// File Types page needs. It is declared in this file to keep its JSON names
// explicit while the existing tool client keeps its narrower public type.
type openIDEALanguageInfo struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Extensions      []string `json:"extensions"`
	Filenames       []string `json:"filenames"`
	LanguageServers []string `json:"languageServers"`
	FirstLine       string   `json:"firstLine"`
	ScratchTemplate string   `json:"scratchTemplate"`
}

func (m *Model) toolLanguages(ctx context.Context) ([]openIDEALanguageInfo, error) {
	raw, err := m.invoke(ctx, "languages", nil)
	if err != nil {
		return nil, err
	}
	var languages []openIDEALanguageInfo
	if err := json.Unmarshal(raw, &languages); err != nil {
		return nil, err
	}
	sort.SliceStable(languages, func(i, j int) bool {
		return strings.ToLower(languages[i].Name) < strings.ToLower(languages[j].Name)
	})
	return languages, nil
}

func (m *Model) loadInspectionsSettings() {
	state := &m.inspectionsSettings
	if state.session && (state.loading || state.loaded) {
		return
	}
	state.session = true
	state.loading = true
	state.loaded = false
	state.err = ""
	state.loadGen++
	generation := state.loadGen
	root := m.Root()
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		read := func(scope string) (inspectionsSettings, error) {
			raw, err := m.invokeSettingsScope(ctx, scope)
			if err != nil {
				return inspectionsSettings{}, err
			}
			var envelope struct {
				Inspections inspectionsSettings `json:"inspections"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				return inspectionsSettings{}, err
			}
			return envelope.Inspections, nil
		}
		global, err := read("global")
		project := inspectionsSettings{}
		if err == nil && root != "" {
			project, err = read("project")
		}
		var languages []openIDEALanguageInfo
		if err == nil {
			languages, _ = m.toolLanguages(ctx)
		}
		m.post(func() {
			if state.loadGen != generation || (root != "" && m.Root() != root) {
				return
			}
			state.loading = false
			state.loaded = err == nil
			if err != nil {
				state.err = "Could not read Inspections: " + err.Error()
				m.invalidate()
				return
			}
			state.globalApplied = cloneInspectionSettings(global)
			state.globalDraft = cloneInspectionSettings(global)
			if root == "" {
				project = global
			}
			state.projectApplied = cloneInspectionSettings(project)
			state.projectDraft = cloneInspectionSettings(project)
			state.languages = languages
			if state.selected == "" {
				state.selected = builtinInspectionCatalog[0].ID
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadSpellingSettings() {
	state := &m.spellingSettings
	if state.session && (state.loading || state.loaded) {
		return
	}
	state.session = true
	state.loading = true
	state.loaded = false
	state.err = ""
	state.loadGen++
	generation := state.loadGen
	root := m.Root()
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		read := func(scope string) (spellingSettings, error) {
			raw, err := m.invokeSettingsScope(ctx, scope)
			if err != nil {
				return spellingSettings{}, err
			}
			var envelope struct {
				Spelling spellingSettings `json:"spelling"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				return spellingSettings{}, err
			}
			return envelope.Spelling, nil
		}
		global, err := read("global")
		project := spellingSettings{}
		if err == nil && root != "" {
			project, err = read("project")
		}
		m.post(func() {
			if state.loadGen != generation || (root != "" && m.Root() != root) {
				return
			}
			state.loading = false
			state.loaded = err == nil
			if err != nil {
				state.err = "Could not read Spelling: " + err.Error()
				m.invalidate()
				return
			}
			state.globalApplied = cloneSpellingSettings(global)
			state.globalDraft = cloneSpellingSettings(global)
			if root == "" {
				project = global
			}
			state.projectApplied = cloneSpellingSettings(project)
			state.projectDraft = cloneSpellingSettings(project)
			m.invalidate()
		})
	}()
}

func (m *Model) loadFormatterSettings(page string) {
	tool, ok := formatterToolForPage(page)
	if !ok {
		return
	}
	state := &m.formatterSettings
	if state.session && state.page == page && (state.loading || state.loaded) {
		return
	}
	state.cacheCurrent()
	if cached, ok := state.pages[page]; ok {
		state.restorePage(cached)
		m.invalidate()
		return
	}
	state.session = true
	state.page = page
	state.tool = tool
	state.scope = m.settingsBackendScope()
	state.loading = true
	state.loaded = false
	state.err = ""
	state.loadGen++
	generation := state.loadGen
	root := m.Root()
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invokeSettingsScope(ctx, state.scope)
		var settings externalFormatterSettings
		if err == nil {
			var envelope struct {
				Languages map[string]externalFormatterSettings `json:"languages"`
			}
			err = json.Unmarshal(raw, &envelope)
			if err == nil {
				settings = envelope.Languages[tool]
				if settings.Languages == nil {
					settings = formatterDefaultSettings(tool)
				}
			}
		}
		m.post(func() {
			if state.loadGen != generation || state.page != page || state.scope != m.settingsBackendScope() ||
				(state.scope == "project" && m.Root() != root) {
				return
			}
			state.loading = false
			state.loaded = err == nil
			if err != nil {
				state.err = "Could not read " + formatterToolName(tool) + " settings: " + err.Error()
				m.invalidate()
				return
			}
			state.applied = cloneFormatterSettings(settings)
			state.draft = cloneFormatterSettings(settings)
			m.invalidate()
		})
	}()
}

func formatterToolName(tool string) string {
	if tool == "oxfmt" {
		return "oxfmt"
	}
	return strings.ToUpper(tool[:1]) + tool[1:]
}

func (m *Model) ensureFileTypeSelection() {
	state := &m.fileTypesSettings
	selected := false
	for _, language := range state.languages {
		if language.ID == state.selected {
			selected = true
			break
		}
	}
	if !selected {
		if len(state.languages) > 0 {
			state.selected = state.languages[0].ID
		} else {
			state.selected = ""
		}
	}
	for _, language := range state.languages {
		if language.ID == state.overrideLang {
			return
		}
	}
	if len(state.languages) > 0 {
		state.overrideLang = state.languages[0].ID
	} else {
		state.overrideLang = ""
	}
}

func (m *Model) fileTypeLanguage() (openIDEALanguageInfo, bool) {
	for _, language := range m.fileTypesSettings.languages {
		if language.ID == m.fileTypesSettings.selected {
			return language, true
		}
	}
	return openIDEALanguageInfo{}, false
}

func fileTypeBuiltinPatterns(language openIDEALanguageInfo) []string {
	out := make([]string, 0, len(language.Extensions)+len(language.Filenames))
	for _, extension := range language.Extensions {
		extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
		if extension != "" {
			out = append(out, "*."+extension)
		}
	}
	for _, filename := range language.Filenames {
		if filename = strings.TrimSpace(filename); filename != "" {
			out = append(out, filename)
		}
	}
	return out
}

func fileTypeEffectivePatterns(language openIDEALanguageInfo, settings fileTypesSettings) []string {
	assoc := settings.FileTypes[language.ID]
	removed := map[string]bool{}
	for _, pattern := range assoc.RemovedPatterns {
		removed[strings.ToLower(pattern)] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, pattern := range fileTypeBuiltinPatterns(language) {
		key := strings.ToLower(pattern)
		if !removed[key] && !seen[key] {
			out = append(out, pattern)
			seen[key] = true
		}
	}
	for _, pattern := range assoc.Patterns {
		key := strings.ToLower(pattern)
		if pattern != "" && !seen[key] {
			out = append(out, pattern)
			seen[key] = true
		}
	}
	return out
}

func fileTypesAddPattern(settings *fileTypesSettings, languages []openIDEALanguageInfo, id, pattern string) {
	pattern = strings.TrimSpace(pattern)
	if settings == nil || pattern == "" {
		return
	}
	for _, language := range languages {
		if language.ID == id {
			continue
		}
		for _, current := range fileTypeEffectivePatterns(language, *settings) {
			if current == pattern {
				fileTypesRemovePattern(settings, language, current)
				break
			}
		}
	}
	var selected openIDEALanguageInfo
	for _, language := range languages {
		if language.ID == id {
			selected = language
			break
		}
	}
	assoc := cloneFileTypeAssociation(settings.FileTypes[id])
	builtin := false
	for _, current := range fileTypeBuiltinPatterns(selected) {
		if current == pattern {
			builtin = true
			break
		}
	}
	if builtin {
		kept := assoc.RemovedPatterns[:0]
		for _, current := range assoc.RemovedPatterns {
			if current != pattern {
				kept = append(kept, current)
			}
		}
		assoc.RemovedPatterns = kept
	} else {
		found := false
		for _, current := range assoc.Patterns {
			found = found || current == pattern
		}
		if !found {
			assoc.Patterns = append(assoc.Patterns, pattern)
		}
	}
	settings.FileTypes[id] = assoc
}

func fileTypesRemovePattern(settings *fileTypesSettings, language openIDEALanguageInfo, pattern string) {
	if settings == nil || pattern == "" {
		return
	}
	assoc := cloneFileTypeAssociation(settings.FileTypes[language.ID])
	builtin := false
	for _, current := range fileTypeBuiltinPatterns(language) {
		if current == pattern {
			builtin = true
			break
		}
	}
	if builtin {
		found := false
		for _, current := range assoc.RemovedPatterns {
			found = found || current == pattern
		}
		if !found {
			assoc.RemovedPatterns = append(assoc.RemovedPatterns, pattern)
		}
	}
	kept := assoc.Patterns[:0]
	for _, current := range assoc.Patterns {
		if current != pattern {
			kept = append(kept, current)
		}
	}
	assoc.Patterns = kept
	if len(assoc.Patterns) == 0 && len(assoc.RemovedPatterns) == 0 && len(assoc.Hashbangs) == 0 {
		delete(settings.FileTypes, language.ID)
		return
	}
	settings.FileTypes[language.ID] = assoc
}

func (m *Model) fileTypesDirty() bool {
	state := &m.fileTypesSettings
	return state.loaded && !sameFileTypesSettings(state.applied, state.draft)
}

func (m *Model) inspectionsDirty() bool {
	state := &m.inspectionsSettings
	return state.loaded && (!sameInspectionSettings(state.globalApplied, state.globalDraft) ||
		!sameInspectionSettings(state.projectApplied, state.projectDraft))
}

func (m *Model) spellingDirty() bool {
	state := &m.spellingSettings
	return state.loaded && (!sameSpellingSettings(state.globalApplied, state.globalDraft) ||
		!sameSpellingSettings(state.projectApplied, state.projectDraft))
}

func (m *Model) formatterDirty() bool {
	state := &m.formatterSettings
	state.cacheCurrent()
	for _, page := range state.pages {
		if !sameFormatterSettings(page.applied, page.draft) {
			return true
		}
	}
	return false
}

func (m *Model) formatterPageDirty(page string) bool {
	state := &m.formatterSettings
	if state.loaded && state.page == page {
		return !sameFormatterSettings(state.applied, state.draft)
	}
	cached, ok := state.pages[page]
	return ok && !sameFormatterSettings(cached.applied, cached.draft)
}

func (m *Model) resetFileTypesDraft() {
	state := &m.fileTypesSettings
	state.draft = fileTypesSettings{
		FileTypes: map[string]fileTypeAssociation{}, FileTypeOverrides: map[string]string{},
		AssociatedApplications: cloneStrings(defaultAssociatedApplications),
		IgnoredNames:           cloneStrings(defaultIgnoredNames),
		Exclude:                cloneStrings(defaultFileExcludes),
	}
	state.patternDraft = ""
	state.hashbangDraft = ""
	state.associatedDraft = ""
	state.ignoredDraft = ""
	state.excludeDraft = ""
	state.overridePath = ""
	m.ensureFileTypeSelection()
}

func (m *Model) resetInspectionsDraft() {
	state := &m.inspectionsSettings
	empty := inspectionsSettings{Profile: "default", Default: map[string]inspectionOverrideSettings{},
		Project: map[string]inspectionOverrideSettings{}, IDEProfiles: map[string]customInspectionProfile{},
		ProjectProfiles: map[string]customInspectionProfile{}}
	state.globalDraft = cloneInspectionSettings(empty)
	state.projectDraft = cloneInspectionSettings(empty)
	state.profilePrompt = nil
}

func (m *Model) resetSpellingDraft() {
	state := &m.spellingSettings
	state.globalDraft = spellingSettings{}
	state.projectDraft = spellingSettings{}
	state.ideWordDraft = ""
	state.projectWordDraft = ""
	state.dictionaryDraft = ""
}

func (m *Model) resetFormatterDraft() {
	state := &m.formatterSettings
	state.draft = formatterDefaultSettings(state.tool)
}

func (m *Model) resetBackendCatalogSettingsSession() {
	fileTypeRuntime := m.fileTypesSettings.runtime
	fileTypeRuntimeLoaded := m.fileTypesSettings.runtimeLoaded
	fileTypeRuntimeGen := m.fileTypesSettings.runtimeGen
	m.fileTypesSettings = fileTypesSettingsState{
		runtime: fileTypeRuntime, runtimeLoaded: fileTypeRuntimeLoaded, runtimeGen: fileTypeRuntimeGen,
	}
	m.inspectionsSettings = inspectionsSettingsState{}
	m.spellingSettings = spellingSettingsState{}
	m.formatterSettings = formatterSettingsState{}
}

func (m *Model) pendingBackendCatalogWrites() []settingsBackendWrite {
	var writes []settingsBackendWrite
	if state := &m.fileTypesSettings; m.fileTypesDirty() {
		if !sameFileTypeAssociationMaps(state.applied.FileTypes, state.draft.FileTypes) {
			writes = append(writes, settingsBackendWrite{state.scope, "files.fileTypes", state.draft.FileTypes})
		}
		if !sameStringMaps(state.applied.FileTypeOverrides, state.draft.FileTypeOverrides) {
			writes = append(writes, settingsBackendWrite{state.scope, "files.fileTypeOverrides", state.draft.FileTypeOverrides})
		}
		if !sameStrings(state.applied.AssociatedApplications, state.draft.AssociatedApplications) {
			writes = append(writes, settingsBackendWrite{state.scope, "files.associatedApplications", state.draft.AssociatedApplications})
		}
		if !sameStrings(state.applied.IgnoredNames, state.draft.IgnoredNames) {
			writes = append(writes, settingsBackendWrite{state.scope, "files.ignoredNames", state.draft.IgnoredNames})
		}
		if !sameStrings(state.applied.Exclude, state.draft.Exclude) {
			writes = append(writes, settingsBackendWrite{state.scope, "files.exclude", state.draft.Exclude})
		}
	}
	if state := &m.inspectionsSettings; m.inspectionsDirty() {
		if !sameInspectionOverrides(state.globalApplied.Default, state.globalDraft.Default) {
			writes = append(writes, settingsBackendWrite{"global", "inspections.default", state.globalDraft.Default})
		}
		if !sameInspectionProfiles(state.globalApplied.IDEProfiles, state.globalDraft.IDEProfiles) {
			writes = append(writes, settingsBackendWrite{"global", "inspections.ideProfiles", state.globalDraft.IDEProfiles})
		}
		profileScope := "project"
		if m.Root() == "" {
			profileScope = "global"
		}
		appliedProfile := state.projectApplied.Profile
		draftProfile := state.projectDraft.Profile
		if profileScope == "global" {
			appliedProfile, draftProfile = state.globalApplied.Profile, state.globalDraft.Profile
		}
		if appliedProfile != draftProfile {
			writes = append(writes, settingsBackendWrite{profileScope, "inspections.profile", draftProfile})
		}
		if m.Root() != "" {
			if !sameInspectionOverrides(state.projectApplied.Project, state.projectDraft.Project) {
				writes = append(writes, settingsBackendWrite{"project", "inspections.project", state.projectDraft.Project})
			}
			if !sameInspectionProfiles(state.projectApplied.ProjectProfiles, state.projectDraft.ProjectProfiles) {
				writes = append(writes, settingsBackendWrite{"project", "inspections.projectProfiles", state.projectDraft.ProjectProfiles})
			}
		}
	}
	if state := &m.spellingSettings; m.spellingDirty() {
		if !sameStrings(state.globalApplied.IDEWords, state.globalDraft.IDEWords) {
			writes = append(writes, settingsBackendWrite{"global", "spelling.ideWords", state.globalDraft.IDEWords})
		}
		if !sameStrings(state.globalApplied.CustomDictionaries, state.globalDraft.CustomDictionaries) {
			writes = append(writes, settingsBackendWrite{"global", "spelling.customDictionaries", state.globalDraft.CustomDictionaries})
		}
		if m.Root() != "" && !sameStrings(state.projectApplied.ProjectWords, state.projectDraft.ProjectWords) {
			writes = append(writes, settingsBackendWrite{"project", "spelling.projectWords", state.projectDraft.ProjectWords})
		}
	}
	for _, page := range m.formatterSettings.allPages() {
		if sameFormatterSettings(page.applied, page.draft) {
			continue
		}
		prefix := "languages." + page.tool + "."
		if page.applied.Path != page.draft.Path {
			writes = append(writes, settingsBackendWrite{page.scope, prefix + "path", page.draft.Path})
		}
		if page.applied.ConfigurationDirectory != page.draft.ConfigurationDirectory {
			writes = append(writes, settingsBackendWrite{page.scope, prefix + "configurationDirectory", page.draft.ConfigurationDirectory})
		}
		if !sameStrings(page.applied.Languages, page.draft.Languages) {
			writes = append(writes, settingsBackendWrite{page.scope, prefix + "languages", page.draft.Languages})
		}
		if page.applied.Prefer != page.draft.Prefer {
			writes = append(writes, settingsBackendWrite{page.scope, prefix + "prefer", page.draft.Prefer})
		}
		if page.applied.RunOnSave != page.draft.RunOnSave {
			writes = append(writes, settingsBackendWrite{page.scope, prefix + "runOnSave", page.draft.RunOnSave})
		}
	}
	if !m.backendFormatOnSaveKnown || m.backendFormatOnSave != m.prefs.General.ReformatOnSave {
		writes = append(writes, settingsBackendWrite{"global", "editor.formatOnSave", m.prefs.General.ReformatOnSave})
	}
	writes = append(writes, m.pendingNativeSettingsWrites()...)
	return writes
}

func sameFileTypeAssociationMaps(a, b map[string]fileTypeAssociation) bool {
	if len(a) != len(b) {
		return false
	}
	for id, value := range a {
		if other, ok := b[id]; !ok || !sameFileTypeAssociation(value, other) {
			return false
		}
	}
	return true
}

func sameStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

type backendCatalogSaveSnapshot struct {
	fileTypes        *fileTypesSettings
	inspections      *inspectionsSettingsSaveSnapshot
	spelling         *spellingSettingsSaveSnapshot
	formatters       map[string]formatterPageState
	native           map[string]nativeSettingsPageState
	fileTypesRuntime fileTypesSettings
}

// saveBackendCatalogThenJava writes the backend-backed pages staged by this
// dialog, then hands off to the existing Java/language-server chain so OK
// closes only after every backend write has settled.
func (m *Model) saveBackendCatalogThenJava(closeAfter bool) {
	if m.mgr == nil && m.invokeHook == nil {
		m.saveJavaSettingsThenLanguageServers(closeAfter)
		return
	}
	writes := m.pendingBackendCatalogWrites()
	if len(writes) == 0 {
		m.saveJavaSettingsThenLanguageServers(closeAfter)
		return
	}
	if m.fileTypesSettings.saving || m.inspectionsSettings.saving ||
		m.spellingSettings.saving || m.formatterSettings.saving || m.nativeSettings.saving {
		return
	}
	var snapshot backendCatalogSaveSnapshot
	if m.fileTypesDirty() {
		value := cloneFileTypesSettings(m.fileTypesSettings.draft)
		snapshot.fileTypes = &value
		snapshot.fileTypesRuntime = value
		m.fileTypesSettings.saving = true
		m.fileTypesSettings.saveGen++
	}
	if m.inspectionsDirty() {
		value := inspectionsSettingsSaveSnapshot{
			global:  cloneInspectionSettings(m.inspectionsSettings.globalDraft),
			project: cloneInspectionSettings(m.inspectionsSettings.projectDraft),
		}
		snapshot.inspections = &value
		m.inspectionsSettings.saving = true
		m.inspectionsSettings.saveGen++
	}
	if m.spellingDirty() {
		value := spellingSettingsSaveSnapshot{
			global:  cloneSpellingSettings(m.spellingSettings.globalDraft),
			project: cloneSpellingSettings(m.spellingSettings.projectDraft),
		}
		snapshot.spelling = &value
		m.spellingSettings.saving = true
		m.spellingSettings.saveGen++
	}
	if m.formatterDirty() {
		snapshot.formatters = map[string]formatterPageState{}
		for _, page := range m.formatterSettings.allPages() {
			if !sameFormatterSettings(page.applied, page.draft) {
				snapshot.formatters[page.page] = page
			}
		}
		m.formatterSettings.saving = true
		m.formatterSettings.saveGen++
	}
	nativeWrites := m.pendingNativeSettingsWrites()
	if len(nativeWrites) > 0 {
		snapshot.native = map[string]nativeSettingsPageState{}
		for page, state := range m.nativeSettings.pages {
			if m.nativeSettingsDirty(page) {
				snapshot.native[page] = nativeSettingsClonePage(state)
			}
		}
		m.nativeSettings.saving = true
	}
	m.settingsErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		for _, write := range writes {
			encoded, err := json.Marshal(write.value)
			if err == nil {
				_, err = m.invoke(ctx, "settings_set", map[string]any{
					"scope": write.scope, "key": write.key, "value": string(encoded),
				})
			}
			if err != nil {
				m.post(func() { m.finishBackendCatalogSave(write, err) })
				return
			}
		}
		m.invalidateFileEncodingSettings()
		m.post(func() {
			if snapshot.fileTypes != nil {
				m.fileTypesSettings.applied = cloneFileTypesSettings(*snapshot.fileTypes)
				m.fileTypesSettings.runtime = cloneFileTypesSettings(snapshot.fileTypesRuntime)
				m.fileTypesSettings.runtimeLoaded = true
				m.fileTypesSettings.saving = false
			}
			if snapshot.inspections != nil {
				m.inspectionsSettings.globalApplied = cloneInspectionSettings(snapshot.inspections.global)
				m.inspectionsSettings.projectApplied = cloneInspectionSettings(snapshot.inspections.project)
				m.inspectionsSettings.saving = false
			}
			if snapshot.spelling != nil {
				m.spellingSettings.globalApplied = cloneSpellingSettings(snapshot.spelling.global)
				m.spellingSettings.projectApplied = cloneSpellingSettings(snapshot.spelling.project)
				m.spellingSettings.saving = false
			}
			if len(snapshot.formatters) > 0 {
				for page, value := range snapshot.formatters {
					m.formatterSettings.pages[page] = value
					if m.formatterSettings.page == page {
						m.formatterSettings.applied = cloneFormatterSettings(value.draft)
						m.formatterSettings.draft = cloneFormatterSettings(value.draft)
					}
				}
				m.formatterSettings.saving = false
			}
			if len(snapshot.native) > 0 {
				for page, value := range snapshot.native {
					state := m.nativeSettings.pages[page]
					state.applied = nativeSettingsCloneValues(value.draft)
					state.draft = nativeSettingsCloneValues(value.draft)
					m.nativeSettings.pages[page] = state
				}
				m.applyRunRuntimeSettings(snapshot.native)
				m.applyDebuggerRuntimeSettings(snapshot.native)
				m.applyEditorSchemeNativePages(snapshot.native)
				m.trimAgentRuns()
				m.refreshDecorations()
				m.nativeSettings.saving = false
				if nativeSDKPageRequiresRuntimeReload(snapshot.native) && m.run.trusted {
					m.loadRunConfigs()
					m.RefreshExternalLibraries()
				}
				if _, changed := snapshot.native[settingsPageAgentMCPServer]; changed && m.mcpServer.session {
					m.loadMCPServerStatus(true)
				}
			}
			m.backendFormatOnSave = m.prefs.General.ReformatOnSave
			m.backendFormatOnSaveKnown = true
			m.settingsErr = ""
			m.applyFileTypeRuntime()
			m.status = "Settings updated"
			m.saveJavaSettingsThenLanguageServers(closeAfter)
			m.invalidate()
		})
	}()
}

// The inspection snapshot is stored separately from the nested settings type
// so successful writes can commit both scopes atomically.
type inspectionsSettingsSaveSnapshot struct {
	global  inspectionsSettings
	project inspectionsSettings
}

type spellingSettingsSaveSnapshot struct {
	global  spellingSettings
	project spellingSettings
}

func (m *Model) finishBackendCatalogSave(write settingsBackendWrite, err error) {
	m.fileTypesSettings.saving = false
	m.inspectionsSettings.saving = false
	m.spellingSettings.saving = false
	m.formatterSettings.saving = false
	m.nativeSettings.saving = false
	m.settingsErr = fmt.Sprintf("Could not save %s %s: %v", write.scope, write.key, err)
	m.invalidate()
}

func (m *Model) settingsBackendCatalogSaving() bool {
	return m.fileTypesSettings.saving || m.inspectionsSettings.saving ||
		m.spellingSettings.saving || m.formatterSettings.saving || m.nativeSettings.saving
}

func (m *Model) refreshFileTypeRuntime(ctx context.Context) fileTypesSettings {
	if m.mgr == nil && m.invokeHook == nil {
		return fileTypesSettings{AssociatedApplications: cloneStrings(defaultAssociatedApplications)}
	}
	state := &m.fileTypesSettings
	state.runtimeGen++
	generation := state.runtimeGen
	scope := m.settingsBackendScope()
	root := m.Root()
	raw, err := m.invokeSettingsScope(ctx, scope)
	if err != nil {
		return fileTypesSettings{AssociatedApplications: cloneStrings(defaultAssociatedApplications)}
	}
	settings, err := decodeFileTypesSettings(raw)
	if err != nil {
		return fileTypesSettings{AssociatedApplications: cloneStrings(defaultAssociatedApplications)}
	}
	languages, languageErr := m.toolLanguages(ctx)
	var editorState struct {
		Editor struct {
			FormatOnSave *bool `json:"formatOnSave"`
		} `json:"editor"`
	}
	editorDecoded := json.Unmarshal(raw, &editorState) == nil && editorState.Editor.FormatOnSave != nil
	backendFormatOnSave := m.prefs.General.ReformatOnSave
	if editorDecoded {
		backendFormatOnSave = *editorState.Editor.FormatOnSave
	}
	if editorDecoded && backendFormatOnSave != m.prefs.General.ReformatOnSave {
		_ = m.invokeSettingsSet(ctx, "global", "editor.formatOnSave", m.prefs.General.ReformatOnSave)
		backendFormatOnSave = m.prefs.General.ReformatOnSave
	}
	m.post(func() {
		if state.runtimeGen != generation || scope != m.settingsBackendScope() ||
			(scope == "project" && m.Root() != root) {
			return
		}
		state.runtime = cloneFileTypesSettings(settings)
		state.runtimeLoaded = true
		if languageErr == nil {
			state.languages = languages
		}
		if editorDecoded {
			m.backendFormatOnSave = backendFormatOnSave
			m.backendFormatOnSaveKnown = true
		}
		m.applyFileTypeRuntime()
		m.invalidate()
	})
	return settings
}

func (m *Model) invokeSettingsSet(ctx context.Context, scope, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = m.invoke(ctx, "settings_set", map[string]any{
		"scope": scope, "key": key, "value": string(raw),
	})
	return err
}

func (m *Model) associatedApplicationPatterns() []string {
	if m.fileTypesSettings.runtimeLoaded {
		return m.fileTypesSettings.runtime.AssociatedApplications
	}
	return defaultAssociatedApplications
}

func wildcardFileNameMatch(pattern, name string) bool {
	pattern = strings.ToLower(pattern)
	name = strings.ToLower(name)
	if pattern == "" {
		return false
	}
	var match func(int, int) bool
	match = func(pi, ni int) bool {
		for pi < len(pattern) {
			switch pattern[pi] {
			case '*':
				for pi+1 < len(pattern) && pattern[pi+1] == '*' {
					pi++
				}
				if pi+1 == len(pattern) {
					return true
				}
				for next := ni; next <= len(name); next++ {
					if match(pi+1, next) {
						return true
					}
				}
				return false
			case '?':
				if ni >= len(name) {
					return false
				}
				pi++
				ni++
			default:
				if ni >= len(name) || pattern[pi] != name[ni] {
					return false
				}
				pi++
				ni++
			}
		}
		return ni == len(name)
	}
	return match(0, 0)
}

func pathMatchesAssociatedApplication(path string, patterns []string) bool {
	name := filepath.Base(path)
	for _, pattern := range patterns {
		if wildcardFileNameMatch(strings.TrimSpace(pattern), name) {
			return true
		}
	}
	return false
}

func partitionAssociatedPaths(paths, patterns []string) (editorPaths, associated []string) {
	for _, path := range paths {
		if pathMatchesAssociatedApplication(path, patterns) {
			associated = append(associated, path)
		} else {
			editorPaths = append(editorPaths, path)
		}
	}
	return editorPaths, associated
}

type mygoLanguageBase struct {
	extensions []string
	filenames  []string
}

var (
	mygoLanguageBasesOnce sync.Once
	mygoLanguageBases     map[string]mygoLanguageBase
)

func ensureMygoLanguageBases() {
	mygoLanguageBasesOnce.Do(func() {
		mygoLanguageBases = map[string]mygoLanguageBase{}
		for _, language := range editor.Languages() {
			if language == nil {
				continue
			}
			mygoLanguageBases[mygoLanguageID(language.Name)] = mygoLanguageBase{
				extensions: cloneStrings(language.Extensions), filenames: cloneStrings(language.Filenames),
			}
		}
	})
}

func mygoLanguageID(name string) string {
	name = strings.ToLower(name)
	replacer := strings.NewReplacer(" ", "", "+", "plus", "#", "sharp", "-", "")
	name = replacer.Replace(name)
	switch name {
	case "cplusplus", "cpp":
		return "cpp"
	case "csharp":
		return "csharp"
	case "shellscript", "bash", "sh":
		return "shell"
	case "plaintext", "text":
		return "plaintext"
	}
	return name
}

func mygoLanguageForID(id string) *editor.Language {
	want := mygoLanguageID(id)
	for _, language := range editor.Languages() {
		if language != nil && mygoLanguageID(language.Name) == want {
			return language
		}
	}
	return editor.LanguageNamed(id)
}

func (m *Model) fileTypeLanguageFor(path string) *editor.Language {
	language := editor.LanguageFor(path)
	if !m.fileTypesSettings.runtimeLoaded {
		return language
	}
	key := fileTypeOverrideKey(path, m.Root())
	if id := m.fileTypesSettings.runtime.FileTypeOverrides[key]; id != "" {
		if overridden := mygoLanguageForID(id); overridden != nil {
			return overridden
		}
	}
	return language
}

func (m *Model) applyFileTypeRuntime() {
	ensureMygoLanguageBases()
	for _, language := range editor.Languages() {
		if language == nil {
			continue
		}
		base, ok := mygoLanguageBases[mygoLanguageID(language.Name)]
		if !ok {
			continue
		}
		language.Extensions = cloneStrings(base.extensions)
		language.Filenames = cloneStrings(base.filenames)
	}
	if !m.fileTypesSettings.runtimeLoaded {
		return
	}
	for id, assoc := range m.fileTypesSettings.runtime.FileTypes {
		languageKey := mygoLanguageID(id)
		var target *editor.Language
		for _, language := range editor.Languages() {
			if language != nil && mygoLanguageID(language.Name) == languageKey {
				target = language
				break
			}
		}
		if target == nil {
			continue
		}
		removed := map[string]bool{}
		for _, pattern := range assoc.RemovedPatterns {
			removed[strings.ToLower(pattern)] = true
		}
		filterPatterns := func(in []string, extension bool) []string {
			out := in[:0]
			for _, value := range in {
				pattern := value
				if extension {
					pattern = "*" + value
				}
				if !removed[strings.ToLower(pattern)] {
					out = append(out, value)
				}
			}
			return out
		}
		target.Extensions = filterPatterns(target.Extensions, true)
		target.Filenames = filterPatterns(target.Filenames, false)
		for _, pattern := range assoc.Patterns {
			if strings.HasPrefix(pattern, "*.") && !strings.ContainsAny(pattern[2:], "*?") {
				extension := "." + strings.TrimPrefix(pattern, "*.")
				if !containsStringFold(target.Extensions, extension) {
					target.Extensions = append(target.Extensions, extension)
				}
				continue
			}
			if !strings.ContainsAny(pattern, "*?") && !containsStringFold(target.Filenames, pattern) {
				target.Filenames = append(target.Filenames, pattern)
			}
		}
	}
	for _, tab := range m.allTabs() {
		if tab == nil || tab.doc == nil || tab.state == nil || tab.state.Buffer == nil {
			continue
		}
		tab.state.Buffer.SetLanguage(m.fileTypeLanguageFor(tab.doc.Path()))
	}
	m.applyProjectTreeFileFilters()
	m.invalidate()
}

func (m *Model) applyProjectTreeFileFilters() {
	if m.tree == nil {
		return
	}
	ignoredNames := defaultIgnoredNames
	exclude := defaultFileExcludes
	if m.fileTypesSettings.runtimeLoaded {
		ignoredNames = m.fileTypesSettings.runtime.IgnoredNames
		exclude = m.fileTypesSettings.runtime.Exclude
	}
	m.tree.SetFileFilters(ignoredNames, exclude)
}

func containsStringFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
