package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/openidea"
)

const (
	projectStructureProject      = "project"
	projectStructureModules      = "modules"
	projectStructureDependencies = "dependencies"
	projectStructureLibraries    = "libraries"
	projectStructureFacets       = "facets"
	projectStructureArtifacts    = "artifacts"
	projectStructureSDKs         = "sdks"
)

var projectStructureSections = []struct {
	ID    string
	Title string
}{
	{projectStructureProject, "Project"},
	{projectStructureModules, "Modules"},
	{projectStructureDependencies, "Dependencies"},
	{projectStructureLibraries, "Libraries"},
	{projectStructureFacets, "Facets"},
	{projectStructureArtifacts, "Artifacts"},
	{projectStructureSDKs, "SDKs"},
}

var projectStructureSDKKinds = []struct {
	Kind  string
	Label string
}{
	{"jdk", "JDK"},
	{"python", "Python"},
	{"node", "Node.js"},
	{"go", "Go"},
	{"rust", "Rust"},
	{"php", "PHP"},
	{"ruby", "Ruby"},
	{"maven", "Maven"},
}

var projectDependencyScopes = []string{"compile", "test", "runtime", "provided"}
var projectArtifactKinds = []string{"jar", "war", "warExploded"}
var projectFacetKinds = []string{"web", "spring", "jpa"}

type projectLibrary struct {
	Name     string   `json:"name"`
	Classes  []string `json:"classes"`
	Sources  []string `json:"sources"`
	Javadocs []string `json:"javadocs"`
	Scope    string   `json:"scope"`
}

type moduleDependency struct {
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Scope    string `json:"scope"`
	Exported bool   `json:"exported"`
}

type moduleStructure struct {
	JDK                string             `json:"jdk"`
	LanguageLevel      string             `json:"languageLevel"`
	CompilerOutput     string             `json:"compilerOutput"`
	TestCompilerOutput string             `json:"testCompilerOutput"`
	Dependencies       []moduleDependency `json:"dependencies"`
}

type facetConfig struct {
	Kind       string `json:"kind"`
	Module     string `json:"module"`
	Descriptor string `json:"descriptor"`
	WebRoot    string `json:"webRoot"`
}

type artifactConfig struct {
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	Module           string   `json:"module"`
	OutputDirectory  string   `json:"outputDirectory"`
	MainClass        string   `json:"mainClass"`
	IncludeLibraries []string `json:"includeLibraries"`
}

type projectStructureSettings struct {
	SourceRoots        []string                   `json:"sourceRoots"`
	TestRoots          []string                   `json:"testRoots"`
	ResourceRoots      []string                   `json:"resourceRoots"`
	TestResourceRoots  []string                   `json:"testResourceRoots"`
	Excluded           []string                   `json:"excluded"`
	Unmarked           []string                   `json:"unmarked"`
	ContentRoots       []string                   `json:"contentRoots"`
	Dependencies       []string                   `json:"dependencies"`
	MemberDependencies map[string][]string        `json:"memberDependencies"`
	ExcludeFiles       string                     `json:"excludeFiles"`
	CompilerOutput     string                     `json:"compilerOutput"`
	Modules            map[string]moduleStructure `json:"modules"`
	Libraries          []projectLibrary           `json:"libraries"`
	Facets             []facetConfig              `json:"facets"`
	Artifacts          []artifactConfig           `json:"artifacts"`
}

type projectStructureSnapshot struct {
	Structure     projectStructureSettings `json:"projectStructure"`
	JDK           string                   `json:"-"`
	LanguageLevel string                   `json:"-"`
	JDTLSJDK      string                   `json:"-"`
}

type detectedProjectRoot struct {
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	DetectedFrom string `json:"detectedFrom"`
}

type webArtifact struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Module string `json:"module"`
	Tool   string `json:"tool"`
}

type projectStructureDirectory struct {
	Path string
	Name string
	Rel  string
}

type projectStructureImage struct {
	Settings   projectStructureSnapshot
	SDKs       map[string][]openidea.Sdk
	Detected   []detectedProjectRoot
	Build      buildModel
	Web        []webArtifact
	Libraries  []openidea.LibraryGroup
	LoadErrors []string
}

type projectStructureState struct {
	open    bool
	loading bool
	loaded  bool
	saving  bool
	scope   string
	section string

	applied projectStructureSnapshot
	draft   projectStructureSnapshot
	image   projectStructureImage
	err     string

	moduleKey string
	moduleTab string
	library   int
	facet     int
	artifact  int
	sdk       int
	member    int

	dirExpanded map[string]bool
	dirLoaded   map[string]bool
	dirChildren map[string][]projectStructureDirectory
	selectedDir string

	loadGen uint64
	saveGen uint64
}

func defaultProjectStructureSettings() projectStructureSettings {
	return projectStructureSettings{
		SourceRoots:        []string{},
		TestRoots:          []string{},
		ResourceRoots:      []string{},
		TestResourceRoots:  []string{},
		Excluded:           []string{},
		Unmarked:           []string{},
		ContentRoots:       []string{},
		Dependencies:       []string{},
		MemberDependencies: map[string][]string{},
		Modules:            map[string]moduleStructure{},
		Libraries:          []projectLibrary{},
		Facets:             []facetConfig{},
		Artifacts:          []artifactConfig{},
	}
}

func normalizeProjectStructure(ps projectStructureSettings) projectStructureSettings {
	d := defaultProjectStructureSettings()
	if ps.SourceRoots != nil {
		d.SourceRoots = append([]string(nil), ps.SourceRoots...)
	}
	if ps.TestRoots != nil {
		d.TestRoots = append([]string(nil), ps.TestRoots...)
	}
	if ps.ResourceRoots != nil {
		d.ResourceRoots = append([]string(nil), ps.ResourceRoots...)
	}
	if ps.TestResourceRoots != nil {
		d.TestResourceRoots = append([]string(nil), ps.TestResourceRoots...)
	}
	if ps.Excluded != nil {
		d.Excluded = append([]string(nil), ps.Excluded...)
	}
	if ps.Unmarked != nil {
		d.Unmarked = append([]string(nil), ps.Unmarked...)
	}
	if ps.ContentRoots != nil {
		d.ContentRoots = append([]string(nil), ps.ContentRoots...)
	}
	if ps.Dependencies != nil {
		d.Dependencies = append([]string(nil), ps.Dependencies...)
	}
	if ps.MemberDependencies != nil {
		for member, roots := range ps.MemberDependencies {
			if member == "" {
				continue
			}
			d.MemberDependencies[member] = append([]string(nil), roots...)
		}
	}
	d.ExcludeFiles = strings.TrimSpace(ps.ExcludeFiles)
	d.CompilerOutput = strings.TrimSpace(ps.CompilerOutput)
	if ps.Modules != nil {
		for key, module := range ps.Modules {
			key = normalizeProjectFolder(key)
			if key == "" {
				key = "."
			}
			if module.Dependencies == nil {
				module.Dependencies = []moduleDependency{}
			}
			d.Modules[key] = module
		}
	}
	if ps.Libraries != nil {
		for _, library := range ps.Libraries {
			library.Classes = nonNilStrings(library.Classes)
			library.Sources = nonNilStrings(library.Sources)
			library.Javadocs = nonNilStrings(library.Javadocs)
			if !slicesContain(projectDependencyScopes, library.Scope) {
				library.Scope = "compile"
			}
			d.Libraries = append(d.Libraries, library)
		}
	}
	if ps.Facets != nil {
		for _, facet := range ps.Facets {
			if !slicesContain(projectFacetKinds, facet.Kind) {
				continue
			}
			if facet.Module == "" {
				facet.Module = "."
			}
			d.Facets = append(d.Facets, facet)
		}
	}
	if ps.Artifacts != nil {
		for _, artifact := range ps.Artifacts {
			if !slicesContain(projectArtifactKinds, artifact.Kind) {
				continue
			}
			artifact.IncludeLibraries = nonNilStrings(artifact.IncludeLibraries)
			if artifact.Module == "" {
				artifact.Module = "."
			}
			d.Artifacts = append(d.Artifacts, artifact)
		}
	}
	return d
}

func normalizeProjectStructureSnapshot(s projectStructureSnapshot) projectStructureSnapshot {
	s.Structure = normalizeProjectStructure(s.Structure)
	s.JDK = strings.TrimSpace(s.JDK)
	s.LanguageLevel = strings.TrimSpace(s.LanguageLevel)
	s.JDTLSJDK = strings.TrimSpace(s.JDTLSJDK)
	return s
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func normalizeProjectFolder(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	value = strings.TrimPrefix(value, "./")
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return value
}

func (m *Model) OpenProjectStructure(section string) {
	state := &m.projectStructure
	if state.open {
		m.SetProjectStructureSection(section)
		return
	}
	state.open = true
	state.section = normalizeProjectStructureSection(section)
	state.moduleKey = "."
	state.moduleTab = "sources"
	state.library = 0
	state.facet = 0
	state.artifact = 0
	state.sdk = 0
	state.member = 0
	state.dirExpanded = map[string]bool{}
	state.dirLoaded = map[string]bool{}
	state.dirChildren = map[string][]projectStructureDirectory{}
	state.selectedDir = ""
	m.LoadProjectStructure()
}

func normalizeProjectStructureSection(section string) string {
	for _, item := range projectStructureSections {
		if item.ID == section {
			return section
		}
	}
	return projectStructureProject
}

func (m *Model) CloseProjectStructure() {
	state := &m.projectStructure
	state.open = false
	state.loading = false
	state.saving = false
	state.err = ""
	state.loadGen++
	state.saveGen++
	m.invalidate()
}

func (m *Model) SetProjectStructureSection(section string) {
	state := &m.projectStructure
	section = normalizeProjectStructureSection(section)
	if state.section == section {
		return
	}
	state.section = section
	m.ensureProjectStructureSelection()
	m.invalidate()
}

func (m *Model) LoadProjectStructure() {
	state := &m.projectStructure
	state.loadGen++
	generation := state.loadGen
	root := m.Root()
	scope := "project"
	if root == "" {
		scope = "global"
	}
	state.loading = true
	state.loaded = false
	state.scope = scope
	state.err = ""
	m.invalidate()

	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		image := projectStructureImage{
			SDKs: map[string][]openidea.Sdk{},
		}
		command := "settings_get"
		if scope == "global" {
			command = "settings_get_global"
		}
		raw, err := m.invoke(ctx, command, nil)
		if err != nil {
			image.LoadErrors = append(image.LoadErrors, "settings: "+err.Error())
		} else {
			var envelope struct {
				ProjectStructure *projectStructureSettings `json:"projectStructure"`
				Languages        struct {
					Java struct {
						JDK           string `json:"jdk"`
						LanguageLevel string `json:"languageLevel"`
						JDTLSJDK      string `json:"jdtlsJdk"`
					} `json:"java"`
				} `json:"languages"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				image.LoadErrors = append(image.LoadErrors, "settings: "+err.Error())
			} else {
				if envelope.ProjectStructure != nil {
					image.Settings.Structure = *envelope.ProjectStructure
				}
				image.Settings.JDK = envelope.Languages.Java.JDK
				image.Settings.LanguageLevel = envelope.Languages.Java.LanguageLevel
				image.Settings.JDTLSJDK = envelope.Languages.Java.JDTLSJDK
			}
		}
		image.Settings = normalizeProjectStructureSnapshot(image.Settings)
		if root == "" {
			m.post(func() {
				if state.loadGen != generation || state.open != true {
					return
				}
				state.loading = false
				state.loaded = len(image.LoadErrors) == 0
				state.image = image
				state.applied = image.Settings
				state.draft = image.Settings
				if len(image.LoadErrors) != 0 {
					state.err = strings.Join(image.LoadErrors, "\n")
				}
				m.invalidate()
			})
			return
		}

		var wait sync.WaitGroup
		var lock sync.Mutex
		recordError := func(prefix string, err error) {
			if err == nil {
				return
			}
			lock.Lock()
			image.LoadErrors = append(image.LoadErrors, prefix+": "+err.Error())
			lock.Unlock()
		}

		wait.Add(1)
		go func() {
			defer wait.Done()
			raw, err := m.invoke(ctx, "project_roots", nil)
			if err != nil {
				recordError("project roots", err)
				return
			}
			var roots struct {
				Detected []detectedProjectRoot `json:"detected"`
			}
			if err := json.Unmarshal(raw, &roots); err != nil {
				recordError("project roots", err)
				return
			}
			lock.Lock()
			image.Detected = roots.Detected
			lock.Unlock()
		}()

		wait.Add(1)
		go func() {
			defer wait.Done()
			raw, err := m.invoke(ctx, "build_model", nil)
			if err != nil {
				recordError("build model", err)
				return
			}
			var model buildModel
			if err := json.Unmarshal(raw, &model); err != nil {
				recordError("build model", err)
				return
			}
			lock.Lock()
			image.Build = model
			lock.Unlock()
		}()

		wait.Add(1)
		go func() {
			defer wait.Done()
			raw, err := m.invoke(ctx, "run_tomcat_artifacts", nil)
			if err != nil {
				// Older servers may not expose artifact discovery; the dialog
				// still carries every configurable Project Structure section.
				return
			}
			var items []webArtifact
			if err := json.Unmarshal(raw, &items); err != nil {
				recordError("web artifacts", err)
				return
			}
			lock.Lock()
			image.Web = items
			lock.Unlock()
		}()

		request := m.nextRequest()
		channel := m.nextChannel()
		wait.Add(1)
		go func() {
			defer wait.Done()
			groups, err := m.projectLibraries(ctx, request, channel)
			if err != nil {
				if !isCancelledError(err) {
					recordError("external libraries", err)
				}
				return
			}
			lock.Lock()
			image.Libraries = groups
			lock.Unlock()
		}()

		for _, item := range projectStructureSDKKinds {
			item := item
			wait.Add(1)
			go func() {
				defer wait.Done()
				raw, err := m.invoke(ctx, "sdk_detect", map[string]any{"kind": item.Kind})
				if err != nil {
					return
				}
				var sdks []openidea.Sdk
				if err := json.Unmarshal(raw, &sdks); err != nil {
					recordError(item.Label+" SDKs", err)
					return
				}
				lock.Lock()
				image.SDKs[item.Kind] = sdks
				lock.Unlock()
			}()
		}
		wait.Wait()
		sort.Slice(image.Detected, func(i, j int) bool {
			return image.Detected[i].Path < image.Detected[j].Path
		})
		image.Libraries = sortLibraryGroups(mergeLibraryGroups(image.Libraries))
		image.Settings = normalizeProjectStructureSnapshot(image.Settings)
		m.post(func() {
			if state.loadGen != generation || !state.open ||
				(scope == "project" && m.Root() != root) {
				return
			}
			state.loading = false
			state.loaded = len(image.LoadErrors) == 0
			state.image = image
			state.applied = image.Settings
			state.draft = image.Settings
			if len(image.LoadErrors) != 0 {
				state.err = strings.Join(image.LoadErrors, "\n")
			}
			m.ensureProjectStructureSelection()
			m.invalidate()
		})
	}()
}

func (m *Model) ensureProjectStructureSelection() {
	state := &m.projectStructure
	modules := m.projectStructureModules()
	found := false
	for _, module := range modules {
		if module.Key == state.moduleKey {
			found = true
			break
		}
	}
	if !found {
		state.moduleKey = "."
	}
	state.library = projectClampIndex(state.library, len(state.draft.Structure.Libraries))
	state.facet = projectClampIndex(state.facet, len(state.draft.Structure.Facets))
	state.artifact = projectClampIndex(state.artifact, len(state.draft.Structure.Artifacts))
	rows := m.projectStructureSDKRows()
	state.sdk = projectClampIndex(state.sdk, len(rows))
	members := m.projectStructureWorkspaceMembers()
	state.member = projectClampIndex(state.member, len(members))
	if state.moduleTab != "paths" {
		state.moduleTab = "sources"
	}
}

func projectClampIndex(index, length int) int {
	if length <= 0 {
		return -1
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func (m *Model) projectStructureDirty() bool {
	state := &m.projectStructure
	return !reflect.DeepEqual(
		normalizeProjectStructureSnapshot(state.applied),
		normalizeProjectStructureSnapshot(state.draft),
	)
}

func (m *Model) ResetProjectStructure() {
	state := &m.projectStructure
	state.draft = state.applied
	state.err = ""
	m.ensureProjectStructureSelection()
	m.invalidate()
}

type projectStructureWrite struct {
	key   string
	value any
}

func projectStructureWrites(before, after projectStructureSnapshot) []projectStructureWrite {
	before = normalizeProjectStructureSnapshot(before)
	after = normalizeProjectStructureSnapshot(after)
	var writes []projectStructureWrite
	add := func(key string, oldValue, newValue any) {
		if !reflect.DeepEqual(oldValue, newValue) {
			writes = append(writes, projectStructureWrite{key: key, value: newValue})
		}
	}
	add("projectStructure.sourceRoots", before.Structure.SourceRoots, after.Structure.SourceRoots)
	add("projectStructure.testRoots", before.Structure.TestRoots, after.Structure.TestRoots)
	add("projectStructure.resourceRoots", before.Structure.ResourceRoots, after.Structure.ResourceRoots)
	add("projectStructure.testResourceRoots", before.Structure.TestResourceRoots, after.Structure.TestResourceRoots)
	add("projectStructure.excluded", before.Structure.Excluded, after.Structure.Excluded)
	add("projectStructure.unmarked", before.Structure.Unmarked, after.Structure.Unmarked)
	add("projectStructure.contentRoots", before.Structure.ContentRoots, after.Structure.ContentRoots)
	add("projectStructure.dependencies", before.Structure.Dependencies, after.Structure.Dependencies)
	add("projectStructure.memberDependencies", before.Structure.MemberDependencies, after.Structure.MemberDependencies)
	add("projectStructure.excludeFiles", before.Structure.ExcludeFiles, after.Structure.ExcludeFiles)
	add("projectStructure.compilerOutput", before.Structure.CompilerOutput, after.Structure.CompilerOutput)
	add("projectStructure.modules", before.Structure.Modules, after.Structure.Modules)
	add("projectStructure.libraries", before.Structure.Libraries, after.Structure.Libraries)
	add("projectStructure.facets", before.Structure.Facets, after.Structure.Facets)
	add("projectStructure.artifacts", before.Structure.Artifacts, after.Structure.Artifacts)
	add("languages.java.jdk", before.JDK, after.JDK)
	add("languages.java.languageLevel", before.LanguageLevel, after.LanguageLevel)
	add("languages.java.jdtlsJdk", before.JDTLSJDK, after.JDTLSJDK)
	return writes
}

func (m *Model) saveProjectStructure(closeAfter bool) {
	state := &m.projectStructure
	if state.saving {
		return
	}
	if !m.projectStructureDirty() {
		if closeAfter {
			m.CloseProjectStructure()
		}
		return
	}
	state.saving = true
	state.err = ""
	state.saveGen++
	generation := state.saveGen
	scope := state.scope
	draft := normalizeProjectStructureSnapshot(state.draft)
	writes := projectStructureWrites(state.applied, draft)
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		for _, write := range writes {
			raw, err := json.Marshal(write.value)
			if err != nil {
				m.projectStructureSaveFailed(generation, write.key, err)
				return
			}
			if _, err := m.invoke(ctx, "settings_set", map[string]any{
				"scope": scope,
				"key":   write.key,
				"value": string(raw),
			}); err != nil {
				m.projectStructureSaveFailed(generation, write.key, err)
				return
			}
		}
		m.post(func() {
			if state.saveGen != generation {
				return
			}
			state.saving = false
			state.applied = draft
			state.err = ""
			state.loaded = true
			m.status = "Project structure updated"
			if m.externalLibraries != nil {
				m.RefreshExternalLibraries()
			}
			if m.tree != nil && m.Root() != "" {
				m.tree.Refresh()
			}
			m.invalidate()
			if closeAfter {
				m.CloseProjectStructure()
			}
		})
	}()
}

func (m *Model) projectStructureSaveFailed(generation uint64, key string, err error) {
	state := &m.projectStructure
	m.post(func() {
		if state.saveGen != generation {
			return
		}
		state.saving = false
		state.err = "Could not save " + key + ": " + err.Error()
		m.settingsErr = state.err
		m.invalidate()
	})
}

func (m *Model) ProjectStructureApply() {
	m.saveProjectStructure(false)
}

func (m *Model) ProjectStructureApplyAndClose() {
	m.saveProjectStructure(true)
}

func (m *Model) SetProjectStructureField(key string, value any) {
	state := &m.projectStructure
	d := &state.draft
	switch key {
	case "jdk":
		d.JDK = value.(string)
	case "languageLevel":
		d.LanguageLevel = value.(string)
	case "jdtlsJdk":
		d.JDTLSJDK = value.(string)
	case "compilerOutput":
		d.Structure.CompilerOutput = value.(string)
	case "excludeFiles":
		d.Structure.ExcludeFiles = value.(string)
	}
	m.invalidate()
}

func (m *Model) AddProjectStructureContentRoot(path string) {
	path = filepath.Clean(strings.TrimSpace(path))
	root := m.Root()
	if path == "" || root == "" || path == root || pathInside(root, path) {
		return
	}
	state := &m.projectStructure
	for _, current := range state.draft.Structure.ContentRoots {
		if filepath.Clean(current) == path {
			return
		}
	}
	state.draft.Structure.ContentRoots = append(state.draft.Structure.ContentRoots, path)
	sort.Strings(state.draft.Structure.ContentRoots)
	m.invalidate()
}

func (m *Model) RemoveProjectStructureContentRoot(path string) {
	state := &m.projectStructure
	next := state.draft.Structure.ContentRoots[:0]
	for _, current := range state.draft.Structure.ContentRoots {
		if filepath.Clean(current) != filepath.Clean(path) {
			next = append(next, current)
		}
	}
	state.draft.Structure.ContentRoots = next
	m.invalidate()
}

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (m *Model) MarkProjectStructureFolder(rel, role string) {
	rel = normalizeProjectFolder(rel)
	if rel == "" && role == "excluded" {
		return
	}
	state := &m.projectStructure
	d := &state.draft.Structure
	d.SourceRoots = projectRemoveString(d.SourceRoots, rel)
	d.TestRoots = projectRemoveString(d.TestRoots, rel)
	d.ResourceRoots = projectRemoveString(d.ResourceRoots, rel)
	d.TestResourceRoots = projectRemoveString(d.TestResourceRoots, rel)
	d.Excluded = projectRemoveString(d.Excluded, rel)
	d.Unmarked = projectRemoveString(d.Unmarked, rel)
	auto := ""
	for _, detected := range state.image.Detected {
		if normalizeProjectFolder(detected.Path) == rel {
			auto = detected.Kind
			break
		}
	}
	switch role {
	case "":
		if auto != "" {
			d.Unmarked = appendSorted(d.Unmarked, rel)
		}
	case "source":
		if auto != "source" {
			d.SourceRoots = appendSorted(d.SourceRoots, rel)
		}
	case "test":
		if auto != "test" {
			d.TestRoots = appendSorted(d.TestRoots, rel)
		}
	case "resources":
		if auto != "resources" {
			d.ResourceRoots = appendSorted(d.ResourceRoots, rel)
		}
	case "testResources":
		if auto != "testResources" {
			d.TestResourceRoots = appendSorted(d.TestResourceRoots, rel)
		}
	case "excluded":
		if auto != "excluded" {
			d.Excluded = appendSorted(d.Excluded, rel)
		}
	}
	m.invalidate()
}

func projectRemoveString(values []string, target string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func appendSorted(values []string, target string) []string {
	out := append(append([]string(nil), values...), target)
	sort.Strings(out)
	return out
}

func appendUniqueSorted(values []string, target string) []string {
	if slicesContain(values, target) {
		return append([]string(nil), values...)
	}
	return appendSorted(values, target)
}

func (m *Model) RestoreDetectedProjectRoots() {
	state := &m.projectStructure
	unmarked := state.draft.Structure.Unmarked
	next := unmarked[:0]
	for _, path := range unmarked {
		found := false
		for _, detected := range state.image.Detected {
			if normalizeProjectFolder(detected.Path) == normalizeProjectFolder(path) {
				found = true
				break
			}
		}
		if !found {
			next = append(next, path)
		}
	}
	state.draft.Structure.Unmarked = next
	m.invalidate()
}

func (m *Model) SetProjectStructureModule(key, tab string) {
	state := &m.projectStructure
	key = normalizeProjectFolder(key)
	if key == "" {
		key = "."
	}
	state.moduleKey = key
	if tab == "paths" {
		state.moduleTab = tab
	} else {
		state.moduleTab = "sources"
	}
	state.selectedDir = ""
	m.invalidate()
}

func (m *Model) AddProjectStructureModule(path string) bool {
	root := m.Root()
	path = filepath.Clean(strings.TrimSpace(path))
	if root == "" || (path != root && !pathInside(root, path)) {
		return false
	}
	rel, _ := filepath.Rel(root, path)
	rel = normalizeProjectFolder(filepath.ToSlash(rel))
	if rel == "" {
		rel = "."
	}
	state := &m.projectStructure
	if _, exists := state.draft.Structure.Modules[rel]; !exists {
		state.draft.Structure.Modules[rel] = moduleStructure{
			Dependencies: []moduleDependency{},
		}
	}
	state.moduleKey = rel
	m.invalidate()
	return true
}

func (m *Model) RemoveProjectStructureModule() {
	state := &m.projectStructure
	if state.moduleKey == "." {
		return
	}
	delete(state.draft.Structure.Modules, state.moduleKey)
	state.moduleKey = "."
	m.invalidate()
}

func (m *Model) SetProjectStructureModuleField(key, value string) {
	state := &m.projectStructure
	module := state.draft.Structure.Modules[state.moduleKey]
	switch key {
	case "jdk":
		module.JDK = value
	case "languageLevel":
		module.LanguageLevel = value
	case "compilerOutput":
		module.CompilerOutput = value
	case "testCompilerOutput":
		module.TestCompilerOutput = value
	}
	if module.Dependencies == nil {
		module.Dependencies = []moduleDependency{}
	}
	if state.draft.Structure.Modules == nil {
		state.draft.Structure.Modules = map[string]moduleStructure{}
	}
	state.draft.Structure.Modules[state.moduleKey] = module
	m.invalidate()
}

func (m *Model) AddProjectStructureDependency(path string) {
	root := m.Root()
	path = filepath.Clean(strings.TrimSpace(path))
	if root == "" || path == root || pathInside(root, path) {
		return
	}
	state := &m.projectStructure
	for _, current := range state.draft.Structure.Dependencies {
		if filepath.Clean(current) == path {
			return
		}
	}
	state.draft.Structure.Dependencies = append(state.draft.Structure.Dependencies, path)
	sort.Strings(state.draft.Structure.Dependencies)
	m.invalidate()
}

func (m *Model) ToggleProjectStructureDependency(path string, checked bool) {
	state := &m.projectStructure
	if checked {
		state.draft.Structure.Dependencies = appendUniqueSorted(
			state.draft.Structure.Dependencies,
			filepath.Clean(path),
		)
	} else {
		state.draft.Structure.Dependencies = projectRemoveString(
			state.draft.Structure.Dependencies,
			filepath.Clean(path),
		)
	}
	m.invalidate()
}

func (m *Model) ToggleProjectStructureMemberDependency(member, path string, checked bool) {
	state := &m.projectStructure
	values := state.draft.Structure.MemberDependencies[member]
	if checked {
		values = appendUniqueSorted(values, filepath.Clean(path))
	} else {
		values = projectRemoveString(values, filepath.Clean(path))
	}
	if len(values) == 0 {
		delete(state.draft.Structure.MemberDependencies, member)
	} else {
		state.draft.Structure.MemberDependencies[member] = values
	}
	m.invalidate()
}

func (m *Model) AddProjectStructureLibrary(path string, directory bool) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return
	}
	path = m.projectStructureStoredPath(path)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if directory || name == "" {
		name = filepath.Base(path)
	}
	if name == "" {
		name = "Library"
	}
	state := &m.projectStructure
	state.draft.Structure.Libraries = append(state.draft.Structure.Libraries, projectLibrary{
		Name:     name,
		Classes:  []string{path},
		Sources:  []string{},
		Javadocs: []string{},
		Scope:    "compile",
	})
	state.library = len(state.draft.Structure.Libraries) - 1
	m.invalidate()
}

func (m *Model) projectStructureStoredPath(path string) string {
	root := m.Root()
	path = filepath.Clean(path)
	if root != "" && pathInside(root, path) {
		rel, err := filepath.Rel(root, path)
		if err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return path
}

func (m *Model) RemoveProjectStructureLibrary() {
	state := &m.projectStructure
	if state.library < 0 || state.library >= len(state.draft.Structure.Libraries) {
		return
	}
	libraries := state.draft.Structure.Libraries
	state.draft.Structure.Libraries = append(libraries[:state.library], libraries[state.library+1:]...)
	state.library = projectClampIndex(state.library, len(state.draft.Structure.Libraries))
	m.invalidate()
}

func (m *Model) SelectProjectStructureLibrary(index int) {
	state := &m.projectStructure
	if index >= 0 && index < len(state.draft.Structure.Libraries) {
		state.library = index
		m.invalidate()
	}
}

func (m *Model) SetProjectStructureLibraryField(key, value string) {
	state := &m.projectStructure
	if state.library < 0 || state.library >= len(state.draft.Structure.Libraries) {
		return
	}
	library := state.draft.Structure.Libraries[state.library]
	switch key {
	case "name":
		library.Name = value
	case "scope":
		if slicesContain(projectDependencyScopes, value) {
			library.Scope = value
		}
	}
	state.draft.Structure.Libraries[state.library] = library
	m.invalidate()
}

func (m *Model) AppendProjectStructureLibraryPath(field, path string) {
	state := &m.projectStructure
	if state.library < 0 || state.library >= len(state.draft.Structure.Libraries) {
		return
	}
	path = m.projectStructureStoredPath(strings.TrimSpace(path))
	if path == "" {
		return
	}
	library := state.draft.Structure.Libraries[state.library]
	switch field {
	case "classes":
		library.Classes = appendUnique(library.Classes, path)
	case "sources":
		library.Sources = appendUnique(library.Sources, path)
	case "javadocs":
		library.Javadocs = appendUnique(library.Javadocs, path)
	}
	state.draft.Structure.Libraries[state.library] = library
	m.invalidate()
}

func (m *Model) RemoveProjectStructureLibraryPath(field, path string) {
	state := &m.projectStructure
	if state.library < 0 || state.library >= len(state.draft.Structure.Libraries) {
		return
	}
	library := state.draft.Structure.Libraries[state.library]
	switch field {
	case "classes":
		library.Classes = projectRemoveString(library.Classes, path)
	case "sources":
		library.Sources = projectRemoveString(library.Sources, path)
	case "javadocs":
		library.Javadocs = projectRemoveString(library.Javadocs, path)
	}
	state.draft.Structure.Libraries[state.library] = library
	m.invalidate()
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func (m *Model) AddProjectStructureFacet(kind string) {
	if !slicesContain(projectFacetKinds, kind) {
		return
	}
	facet := facetConfig{Kind: kind, Module: "."}
	switch kind {
	case "web":
		facet.Descriptor = "src/main/webapp/WEB-INF/web.xml"
		facet.WebRoot = "src/main/webapp"
	case "spring":
		facet.Descriptor = "src/main/resources/application.yml"
	case "jpa":
		facet.Descriptor = "src/main/resources/META-INF/persistence.xml"
	}
	state := &m.projectStructure
	state.draft.Structure.Facets = append(state.draft.Structure.Facets, facet)
	state.facet = len(state.draft.Structure.Facets) - 1
	m.invalidate()
}

func (m *Model) RemoveProjectStructureFacet() {
	state := &m.projectStructure
	if state.facet < 0 || state.facet >= len(state.draft.Structure.Facets) {
		return
	}
	facets := state.draft.Structure.Facets
	state.draft.Structure.Facets = append(facets[:state.facet], facets[state.facet+1:]...)
	state.facet = projectClampIndex(state.facet, len(state.draft.Structure.Facets))
	m.invalidate()
}

func (m *Model) SelectProjectStructureFacet(index int) {
	state := &m.projectStructure
	if index >= 0 && index < len(state.draft.Structure.Facets) {
		state.facet = index
		m.invalidate()
	}
}

func (m *Model) SetProjectStructureFacetField(key, value string) {
	state := &m.projectStructure
	if state.facet < 0 || state.facet >= len(state.draft.Structure.Facets) {
		return
	}
	facet := state.draft.Structure.Facets[state.facet]
	switch key {
	case "kind":
		if slicesContain(projectFacetKinds, value) {
			facet.Kind = value
		}
	case "module":
		facet.Module = value
	case "descriptor":
		facet.Descriptor = m.projectStructureStoredPath(value)
	case "webRoot":
		facet.WebRoot = m.projectStructureStoredPath(value)
	}
	state.draft.Structure.Facets[state.facet] = facet
	m.invalidate()
}

func (m *Model) AddProjectStructureArtifact(kind string) {
	if !slicesContain(projectArtifactKinds, kind) {
		return
	}
	base := filepath.Base(m.Root())
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "app"
	}
	suffix := "jar"
	if kind == "war" {
		suffix = "war"
	} else if kind == "warExploded" {
		suffix = "war exploded"
	}
	state := &m.projectStructure
	names := make([]string, 0, len(state.draft.Structure.Libraries))
	for _, library := range state.draft.Structure.Libraries {
		names = append(names, library.Name)
	}
	state.draft.Structure.Artifacts = append(state.draft.Structure.Artifacts, artifactConfig{
		Name:             base + ":" + suffix,
		Kind:             kind,
		Module:           ".",
		OutputDirectory:  "out/artifacts/" + strings.ReplaceAll(base+"_"+suffix, " ", "_"),
		IncludeLibraries: names,
	})
	state.artifact = len(state.draft.Structure.Artifacts) - 1
	m.invalidate()
}

func (m *Model) RemoveProjectStructureArtifact() {
	state := &m.projectStructure
	if state.artifact < 0 || state.artifact >= len(state.draft.Structure.Artifacts) {
		return
	}
	artifacts := state.draft.Structure.Artifacts
	state.draft.Structure.Artifacts = append(artifacts[:state.artifact], artifacts[state.artifact+1:]...)
	state.artifact = projectClampIndex(state.artifact, len(state.draft.Structure.Artifacts))
	m.invalidate()
}

func (m *Model) SelectProjectStructureArtifact(index int) {
	state := &m.projectStructure
	if index >= 0 && index < len(state.draft.Structure.Artifacts) {
		state.artifact = index
		m.invalidate()
	}
}

func (m *Model) SetProjectStructureArtifactField(key, value string) {
	state := &m.projectStructure
	if state.artifact < 0 || state.artifact >= len(state.draft.Structure.Artifacts) {
		return
	}
	artifact := state.draft.Structure.Artifacts[state.artifact]
	switch key {
	case "name":
		artifact.Name = value
	case "kind":
		if slicesContain(projectArtifactKinds, value) {
			artifact.Kind = value
		}
	case "module":
		artifact.Module = value
	case "outputDirectory":
		artifact.OutputDirectory = m.projectStructureStoredPath(value)
	case "mainClass":
		artifact.MainClass = value
	}
	state.draft.Structure.Artifacts[state.artifact] = artifact
	m.invalidate()
}

func (m *Model) ToggleProjectStructureArtifactLibrary(name string, checked bool) {
	state := &m.projectStructure
	if state.artifact < 0 || state.artifact >= len(state.draft.Structure.Artifacts) {
		return
	}
	artifact := state.draft.Structure.Artifacts[state.artifact]
	if checked {
		artifact.IncludeLibraries = appendUnique(artifact.IncludeLibraries, name)
	} else {
		artifact.IncludeLibraries = projectRemoveString(artifact.IncludeLibraries, name)
	}
	state.draft.Structure.Artifacts[state.artifact] = artifact
	m.invalidate()
}

func (m *Model) SelectProjectStructureSDK(index int) {
	state := &m.projectStructure
	if index >= 0 && index < len(m.projectStructureSDKRows()) {
		state.sdk = index
		m.invalidate()
	}
}

func (m *Model) UseProjectStructureSDK() {
	rows := m.projectStructureSDKRows()
	state := &m.projectStructure
	if state.sdk < 0 || state.sdk >= len(rows) || rows[state.sdk].Kind != "jdk" {
		return
	}
	state.draft.JDK = rows[state.sdk].Path
	m.invalidate()
}

func (m *Model) RefreshProjectStructureSDKs() {
	m.LoadProjectStructure()
}

func (m *Model) ToggleProjectStructureDirectory(path string) {
	state := &m.projectStructure
	path = filepath.Clean(path)
	state.dirExpanded[path] = !state.dirExpanded[path]
	if state.dirExpanded[path] && !state.dirLoaded[path] {
		m.loadProjectStructureDirectory(path)
	}
	m.invalidate()
}

func (m *Model) SelectProjectStructureDirectory(path string) {
	state := &m.projectStructure
	state.selectedDir = filepath.Clean(path)
	m.invalidate()
}

func (m *Model) loadProjectStructureDirectory(path string) {
	state := &m.projectStructure
	if state.dirLoaded[path] {
		return
	}
	state.dirLoaded[path] = true
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "fs_read_dir", map[string]any{"path": path})
		var entries []struct {
			Name       string  `json:"name"`
			Path       string  `json:"path"`
			Kind       string  `json:"kind"`
			TargetKind *string `json:"targetKind"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &entries)
		}
		m.post(func() {
			if !state.open || m.Root() != root {
				return
			}
			if err != nil {
				state.dirLoaded[path] = false
				state.err = "Could not read " + path + ": " + err.Error()
				m.invalidate()
				return
			}
			rows := make([]projectStructureDirectory, 0, len(entries))
			for _, entry := range entries {
				isDir := entry.Kind == "dir" ||
					entry.Kind == "symlink" && entry.TargetKind != nil && *entry.TargetKind == "dir"
				if !isDir || entry.Name == ".git" {
					continue
				}
				entryPath := entry.Path
				if entryPath == "" {
					entryPath = filepath.Join(path, entry.Name)
				}
				rel, _ := filepath.Rel(root, entryPath)
				rows = append(rows, projectStructureDirectory{
					Path: entryPath,
					Name: entry.Name,
					Rel:  normalizeProjectFolder(filepath.ToSlash(rel)),
				})
			}
			sort.Slice(rows, func(i, j int) bool {
				return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
			})
			state.dirChildren[path] = rows
			m.invalidate()
		})
	}()
}

type projectStructureModuleRow struct {
	Key  string
	Name string
	Tool string
}

func (m *Model) projectStructureModules() []projectStructureModuleRow {
	state := &m.projectStructure
	seen := map[string]bool{}
	var rows []projectStructureModuleRow
	add := func(key, name, tool string) {
		key = normalizeProjectFolder(key)
		if key == "" {
			key = "."
		}
		if seen[key] {
			return
		}
		seen[key] = true
		rows = append(rows, projectStructureModuleRow{Key: key, Name: name, Tool: tool})
	}
	projectName := filepath.Base(m.Root())
	if projectName == "." || projectName == string(filepath.Separator) || projectName == "" {
		projectName = "Root Module"
	}
	tool := "custom"
	build := state.image.Build
	if build.Maven != nil {
		tool = "maven"
	}
	if len(build.Gradle) != 0 {
		tool = "gradle"
	}
	add(".", projectName, tool)
	if build.Maven != nil {
		for _, project := range build.Maven.Projects {
			name := project.DisplayName
			if name == "" {
				name = project.ArtifactID
			}
			if name == "" {
				name = project.Dir
			}
			add(project.Dir, name, "maven")
		}
	}
	for _, gradle := range build.Gradle {
		for _, project := range gradle.Projects {
			name := project.Name
			if name == "" {
				name = project.Path
			}
			add(project.Dir, name, "gradle")
		}
	}
	for key := range state.draft.Structure.Modules {
		name := key
		if key == "." {
			name = projectName
		} else {
			name = filepath.Base(key)
		}
		add(key, name, "custom")
	}
	sort.SliceStable(rows[1:], func(i, j int) bool {
		return rows[i+1].Key < rows[j+1].Key
	})
	return rows
}

func (m *Model) projectStructureWorkspaceMembers() []string {
	if m.workspace == nil {
		return nil
	}
	return append([]string(nil), m.workspace.Projects...)
}

func (m *Model) projectStructureDependencyCandidates() []string {
	state := &m.projectStructure
	var roots []string
	for _, path := range state.draft.Structure.Dependencies {
		roots = appendUnique(roots, filepath.Clean(path))
	}
	sort.Strings(roots)
	return roots
}

type projectStructureSDKRow struct {
	Kind    string
	Label   string
	Path    string
	Name    string
	Version string
}

func (m *Model) projectStructureSDKRows() []projectStructureSDKRow {
	state := &m.projectStructure
	var rows []projectStructureSDKRow
	for _, kind := range projectStructureSDKKinds {
		for _, sdk := range state.image.SDKs[kind.Kind] {
			version := ""
			if sdk.Version != nil {
				version = strings.TrimSpace(*sdk.Version)
			}
			name := strings.TrimSpace(sdk.Name)
			if name == "" {
				name = filepath.Base(sdk.Path)
			}
			rows = append(rows, projectStructureSDKRow{
				Kind: kind.Kind, Label: kind.Label, Path: sdk.Path,
				Name: name, Version: version,
			})
		}
	}
	return rows
}

func (m *Model) projectStructureDetectedFacets() []string {
	state := &m.projectStructure
	if state.image.Build.Maven == nil {
		return nil
	}
	var rows []string
	for _, project := range state.image.Build.Maven.Projects {
		module := project.Dir
		if module == "" {
			module = "."
		}
		if project.Packaging == "war" {
			rows = append(rows, "Web ("+module+") · Maven WAR packaging")
		}
		for _, dependency := range project.Dependencies {
			if strings.Contains(strings.ToLower(dependency.GroupID), "spring") {
				rows = append(rows, "Spring ("+module+") · Spring dependencies in pom.xml")
			}
			artifact := strings.ToLower(dependency.ArtifactID)
			if strings.Contains(artifact, "persistence") || strings.Contains(artifact, "hibernate") {
				rows = append(rows, "JPA ("+module+") · JPA / Hibernate dependencies in pom.xml")
			}
		}
	}
	return appendUniqueStrings(rows)
}

func appendUniqueStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if !slicesContain(out, value) {
			out = append(out, value)
		}
	}
	return out
}

func (m *Model) BrowseProjectStructurePath(directory bool, title string, apply func(string)) {
	parent := m.win
	defaultPath := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       title,
			ButtonLabel: "Choose",
			DefaultPath: defaultPath,
			Directory:   directory,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			apply(filepath.Clean(paths[0]))
			m.invalidate()
		})
	}()
}
