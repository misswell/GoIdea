package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type languageSDKDetection struct {
	root    string
	loading bool
	loaded  bool
	sdks    []openidea.Sdk
	err     string
	gen     uint64
}

type pythonInterpreterFlow struct {
	root    string
	busy    bool
	gen     uint64
	request uint32
	channel uint32
	err     string
	log     []string
}

// languageSDKSettingsState caches interpreter detection per project and owns
// the cancellable Python environment flow shared by the pages.
type languageSDKSettingsState struct {
	kinds    map[openidea.SdkKind]languageSDKDetection
	python   pythonInterpreterFlow
	catalog  pythonInterpreterCatalog
	packages pythonPackagesState
	php      phpRuntimeStatus
	status   languageStatusState
	known    map[string]openidea.Sdk
	lastUse  uint64
}

// pythonInterpreterCatalog is the project-aware half of All Interpreters.
// sdk_detect supplies the open project's interpreters, while this state adds
// interpreters used by other trusted recent projects and validates paths in
// the global "added interpreters" setting.
type pythonInterpreterCatalog struct {
	root     string
	loading  bool
	loaded   bool
	err      string
	gen      uint64
	projects []openidea.ProjectInterpreter
	probed   map[string]*openidea.Sdk
	probing  map[string]bool
	probeGen uint64
}

func nativeLanguageSDKKinds(page string) []openidea.SdkKind {
	switch page {
	case settingsPagePythonDeps, settingsPagePythonInterpret:
		return []openidea.SdkKind{openidea.SdkPython}
	case settingsPageNode:
		return []openidea.SdkKind{openidea.SdkNode, openidea.SdkPackageManager}
	case settingsPagePHP:
		return []openidea.SdkKind{openidea.SdkPhp}
	case settingsPageRuby:
		return []openidea.SdkKind{openidea.SdkRuby}
	case settingsPageRust:
		return []openidea.SdkKind{openidea.SdkRust}
	case settingsPageGo:
		return []openidea.SdkKind{openidea.SdkGo}
	case settingsPageTypeScript:
		return []openidea.SdkKind{openidea.SdkTypeScript}
	case settingsPageGradle:
		return []openidea.SdkKind{openidea.SdkJDK}
	case settingsPageMaven:
		return []openidea.SdkKind{openidea.SdkJDK, openidea.SdkMaven}
	}
	return nil
}

// ensureNativeLanguageSDKs starts any missing discovery for page. Detection
// requires a trusted project, so a global/new-project page keeps its editable
// paths and shows no list.
func (m *Model) ensureNativeLanguageSDKs(page string, force bool) {
	root := m.Root()
	if root == "" {
		return
	}
	if m.languageSDKs.kinds == nil {
		m.languageSDKs.kinds = map[openidea.SdkKind]languageSDKDetection{}
	}
	if m.mgr == nil && m.invokeHook == nil {
		for _, kind := range nativeLanguageSDKKinds(page) {
			state := m.languageSDKs.kinds[kind]
			if state.root != root || (!state.loaded && state.err == "") {
				state.root = root
				state.err = "The GoIDEA backend is not connected"
				m.languageSDKs.kinds[kind] = state
			}
		}
		return
	}
	for _, kind := range nativeLanguageSDKKinds(page) {
		state := m.languageSDKs.kinds[kind]
		if !force && state.root == root && (state.loading || state.loaded || state.err != "") {
			continue
		}
		if force {
			state.sdks = nil
		}
		state.root = root
		state.loading = true
		state.loaded = false
		state.err = ""
		state.gen++
		generation := state.gen
		m.languageSDKs.kinds[kind] = state
		m.invalidate()
		go func(kind openidea.SdkKind, root string, generation uint64) {
			ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
			defer cancel()
			raw, err := m.invoke(ctx, "sdk_detect", map[string]any{"kind": kind})
			var sdks []openidea.Sdk
			if err == nil {
				err = json.Unmarshal(raw, &sdks)
			}
			m.post(func() {
				current := m.languageSDKs.kinds[kind]
				if m.Root() != root || current.gen != generation {
					return
				}
				current.loading = false
				current.loaded = err == nil
				if err != nil {
					current.err = err.Error()
				} else {
					current.sdks = append([]openidea.Sdk(nil), sdks...)
					m.rememberLanguageSDKs(sdks)
				}
				m.languageSDKs.kinds[kind] = current
				m.invalidate()
			})
		}(kind, root, generation)
	}
}

// ensurePythonProjectInterpreters loads the interpreters trusted recent
// projects use. It is separate from sdk_detect because the open project is not
// repeated in that response.
func (m *Model) ensurePythonProjectInterpreters(force bool) {
	root := m.Root()
	if root == "" {
		return
	}
	state := m.languageSDKs.catalog
	if !force && state.root == root && (state.loading || state.loaded || state.err != "") {
		return
	}
	state.root = root
	state.loading = true
	state.loaded = false
	state.err = ""
	state.gen++
	state.probeGen++
	state.projects = nil
	state.probed = map[string]*openidea.Sdk{}
	state.probing = map[string]bool{}
	generation := state.gen
	m.languageSDKs.catalog = state
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.languageSDKs.catalog = state
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "sdk_python_project_interpreters", nil)
		var projects []openidea.ProjectInterpreter
		if err == nil {
			err = json.Unmarshal(raw, &projects)
		}
		m.post(func() {
			current := m.languageSDKs.catalog
			if current.gen != generation || m.Root() != root {
				return
			}
			current.loading = false
			current.loaded = err == nil
			if err != nil {
				current.err = err.Error()
			} else {
				current.projects = append([]openidea.ProjectInterpreter(nil), projects...)
			}
			m.languageSDKs.catalog = current
			m.invalidate()
		})
	}()
}

// probePythonInterpreters resolves added paths that sdk_detect and the recent
// project list did not already classify. A resolved path is remembered in the
// shared SDK cache; an explicit nil remains in probed so the row can be shown
// as Invalid.
func (m *Model) probePythonInterpreters() {
	root := m.Root()
	catalog := m.languageSDKs.catalog
	if root == "" || !catalog.loaded {
		return
	}
	known := map[string]bool{}
	for _, sdk := range m.languageSDKList(openidea.SdkPython) {
		known[pythonInterpreterPathKey(sdk.Path, root)] = true
	}
	for _, entry := range catalog.projects {
		known[pythonInterpreterPathKey(entry.SDK.Path, root)] = true
	}
	var todo []string
	for _, path := range m.pythonInterpreterAdded() {
		key := pythonInterpreterPathKey(path, root)
		if key == "" || known[key] {
			continue
		}
		if _, done := catalog.probed[path]; done || catalog.probing[path] {
			continue
		}
		catalog.probing[path] = true
		todo = append(todo, path)
	}
	if len(todo) == 0 {
		m.languageSDKs.catalog = catalog
		return
	}
	catalog.probeGen++
	generation := catalog.probeGen
	m.languageSDKs.catalog = catalog
	go func(paths []string) {
		results := make(map[string]*openidea.Sdk, len(paths))
		for _, path := range paths {
			ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
			raw, err := m.invoke(ctx, "sdk_python_interpreter", map[string]any{"path": path})
			cancel()
			var sdk *openidea.Sdk
			if err == nil {
				err = json.Unmarshal(raw, &sdk)
			}
			if err != nil {
				sdk = nil
			}
			results[path] = sdk
		}
		m.post(func() {
			current := m.languageSDKs.catalog
			if current.probeGen != generation || m.Root() != root {
				return
			}
			if current.probed == nil {
				current.probed = map[string]*openidea.Sdk{}
			}
			for path, sdk := range results {
				delete(current.probing, path)
				current.probed[path] = sdk
				if sdk != nil {
					m.rememberLanguageSDKs([]openidea.Sdk{*sdk})
				}
			}
			m.languageSDKs.catalog = current
			m.invalidate()
		})
	}(todo)
}

func (m *Model) refreshPythonInterpreters() {
	state := m.languageSDKs.catalog
	state.root = ""
	state.loaded = false
	state.loading = false
	state.err = ""
	state.probed = map[string]*openidea.Sdk{}
	state.probing = map[string]bool{}
	m.languageSDKs.catalog = state
	m.ensureNativeLanguageSDKs(settingsPagePythonInterpret, true)
	m.ensurePythonProjectInterpreters(true)
	m.invalidate()
}

func (m *Model) rememberLanguageSDKs(sdks []openidea.Sdk) {
	if m.languageSDKs.known == nil {
		m.languageSDKs.known = map[string]openidea.Sdk{}
	}
	for _, sdk := range sdks {
		if sdk.Path != "" {
			m.languageSDKs.known[languageSDKPathKey(sdk.Path)] = sdk
		}
	}
}

func languageSDKPathKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func sameLanguageSDKPath(a, b string) bool {
	return languageSDKPathKey(a) == languageSDKPathKey(b)
}

func (m *Model) languageSDKList(kind openidea.SdkKind) []openidea.Sdk {
	state := m.languageSDKs.kinds[kind]
	return append([]openidea.Sdk(nil), state.sdks...)
}

func languageSDKLabel(sdk openidea.Sdk) string {
	name := strings.TrimSpace(sdk.Name)
	if name == "" {
		name = filepath.Base(strings.TrimSpace(sdk.Path))
	}
	if sdk.Version != nil {
		version := strings.TrimSpace(*sdk.Version)
		if version != "" && !strings.Contains(name, version) {
			name += " (" + version + ")"
		}
	}
	source := strings.TrimSpace(sdk.Source)
	if source != "" && source != "system" && source != "path" && source != "custom" &&
		!strings.Contains(strings.ToLower(name), source) {
		name += " · " + source
	}
	path := strings.TrimSpace(sdk.Path)
	if path != "" {
		name += " · " + path
	}
	return name
}

func nativeSDKEmptyLabel(kind openidea.SdkKind) string {
	switch kind {
	case openidea.SdkPython:
		return "Auto · project virtualenv or Python on PATH"
	case openidea.SdkNode:
		return "Default · node on PATH"
	case openidea.SdkPhp:
		return "Default · php on PATH"
	case openidea.SdkPackageManager:
		return "Auto · packageManager or lock file"
	case openidea.SdkRust:
		return "Default · rustup default or PATH"
	case openidea.SdkGo:
		return "Default · go on PATH"
	case openidea.SdkTypeScript:
		return "Bundled · language server TypeScript"
	case openidea.SdkJDK:
		return "Project JDK"
	case openidea.SdkMaven:
		return "Auto · Maven wrapper or mvn on PATH"
	case openidea.SdkRuby:
		return "Auto · .ruby-version, mise, asdf, or ruby on PATH"
	}
	return "Auto"
}

func nativeSDKChoices(sdks []openidea.Sdk, current, emptyLabel string, allowEmpty bool) ([]string, []string) {
	var values, labels []string
	seen := map[string]bool{}
	if allowEmpty {
		values = append(values, "")
		labels = append(labels, emptyLabel)
		seen[""] = true
	}
	for _, sdk := range sdks {
		key := languageSDKPathKey(sdk.Path)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		values = append(values, sdk.Path)
		labels = append(labels, languageSDKLabel(sdk))
	}
	current = strings.TrimSpace(current)
	if current != "" && !seen[languageSDKPathKey(current)] {
		values = append(values, current)
		labels = append(labels, "Current · "+current)
	}
	return values, labels
}

func (m *Model) renderNativeSDKPath(c *ui.Context, oi oiColors, page string, field nativeSettingsField) {
	kind := openidea.SdkKind(field.sdkKind)
	state := m.languageSDKs.kinds[kind]
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, field.label, 230)
		value := nativeSettingsString(m, page, field.key)
		choices, labels := nativeSDKChoices(state.sdks, value, nativeSDKEmptyLabel(kind), true)
		shown := labels[0]
		for i, choice := range choices {
			if sameLanguageSDKPath(choice, value) {
				shown = labels[i]
				break
			}
		}
		if ui.Select(c, &shown, labels).Grow(1).Changed() {
			for i, label := range labels {
				if shown == label {
					m.setNativeSettingsValue(page, field.key, choices[i])
					break
				}
			}
		}
	})
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).PaddingStart(238).Children(func() {
		value := nativeSettingsString(m, page, field.key)
		input := ui.TextInput(c, &value).Grow(1).Placeholder(field.placeholder)
		if input.Changed() {
			m.setNativeSettingsValue(page, field.key, strings.TrimSpace(value))
		}
		ui.Button(c, "Browse…").Width(82).OnClick(func() {
			m.browseNativeSettingPath(page, field.key, field.placeholder, field.directory)
		})
	})
	if state.loading {
		ui.Text(c, "Detecting installed runtimes…").FontSize(11).TextColor(oi.textMuted).
			PaddingStart(238).SingleLine()
	} else if state.err != "" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).PaddingStart(238).Children(func() {
			ui.Text(c, "Could not detect runtimes: "+state.err).FontSize(11).TextColor(oi.danger).
				Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "Retry").OnClick(func() {
				m.ensureNativeLanguageSDKs(page, true)
			})
		})
	}
}

func (m *Model) renderNativePath(c *ui.Context, page string, field nativeSettingsField) {
	ui.Text(c, field.label).FontSize(12).Bold().TextColor(oiPaletteOf(c.Theme()).text).
		PaddingStart(20).SingleLine()
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		value := nativeSettingsString(m, page, field.key)
		input := ui.TextInput(c, &value).Grow(1).Placeholder(field.placeholder)
		if input.Changed() {
			m.setNativeSettingsValue(page, field.key, strings.TrimSpace(value))
		}
		ui.Button(c, "Browse…").Width(82).OnClick(func() {
			m.browseNativeSettingPath(page, field.key, field.placeholder, field.directory)
		})
	})
}

func (m *Model) browseNativeSettingPath(page, key, title string, directory bool) {
	if title == "" {
		title = "Select Path"
	}
	root := m.Root()
	current := nativeSettingsString(m, page, key)
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       title,
			ButtonLabel: "Choose",
			DefaultPath: current,
			Directory:   directory,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			if m.Root() != root || m.settingsPage != page {
				return
			}
			m.setNativeSettingsValue(page, key, filepath.Clean(paths[0]))
		})
	}()
}

func nativePackageManagerChoices(sdks []openidea.Sdk) ([]string, []string) {
	values := []string{"", "npm", "pnpm", "yarn", "bun"}
	labels := []string{nativeSDKEmptyLabel(openidea.SdkPackageManager)}
	index := map[string]int{"npm": 1, "pnpm": 2, "yarn": 3, "bun": 4}
	for _, sdk := range sdks {
		name := strings.ToLower(strings.TrimSpace(sdk.Name))
		i, ok := index[name]
		if !ok {
			continue
		}
		label := name
		if sdk.Version != nil && strings.TrimSpace(*sdk.Version) != "" {
			label += " " + strings.TrimSpace(*sdk.Version)
		}
		labels[i] = label + " · " + sdk.Path
	}
	return values, labels
}

func (m *Model) renderNativePackageManager(c *ui.Context, oi oiColors, page string, field nativeSettingsField) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, field.label, 230)
		value := nativeSettingsString(m, page, field.key)
		values, labels := nativePackageManagerChoices(m.languageSDKList(openidea.SdkPackageManager))
		shown := labels[0]
		for i, choice := range values {
			if choice == value {
				shown = labels[i]
				break
			}
		}
		if ui.Select(c, &shown, labels).Grow(1).Changed() {
			for i, label := range labels {
				if shown == label {
					m.setNativeSettingsValue(page, field.key, values[i])
					break
				}
			}
		}
	})
}

type pythonInterpreterRow struct {
	SDK         openidea.Sdk
	Current     bool
	Hidden      bool
	Added       bool
	Invalid     bool
	Project     string
	ProjectName string
}

func absolutePythonInterpreterPath(path, root string) string {
	path = strings.TrimSpace(path)
	if path == "" || root == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "~") ||
		(len(path) >= 2 && path[1] == ':' && (path[2] == '/' || path[2] == '\\')) {
		return path
	}
	return filepath.Join(root, path)
}

func pythonInterpreterPathKey(path, root string) string {
	return languageSDKPathKey(absolutePythonInterpreterPath(path, root))
}

func samePythonInterpreterPath(a, b, root string) bool {
	left := pythonInterpreterPathKey(a, root)
	right := pythonInterpreterPathKey(b, root)
	return left != "" && left == right
}

type pythonInterpreterProject struct {
	Root string
	Name string
}

func pythonInterpreterProjects(root string, fromProjects []openidea.ProjectInterpreter) []pythonInterpreterProject {
	projects := make([]pythonInterpreterProject, 0, len(fromProjects)+1)
	seen := map[string]bool{}
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		key := languageSDKPathKey(dir)
		if dir == "" || seen[key] {
			return
		}
		seen[key] = true
		projects = append(projects, pythonInterpreterProject{Root: dir, Name: filepath.Base(dir)})
	}
	add(root)
	for _, entry := range fromProjects {
		add(entry.Project)
	}
	return projects
}

func owningPythonInterpreterProject(path, root string, projects []pythonInterpreterProject) (string, string) {
	candidate := pythonInterpreterPathKey(path, root)
	if candidate == "" {
		return "", ""
	}
	var owner pythonInterpreterProject
	ownerKey := ""
	for _, project := range projects {
		projectKey := languageSDKPathKey(project.Root)
		if projectKey == "" ||
			(candidate != projectKey && !strings.HasPrefix(candidate, projectKey+string(filepath.Separator)) &&
				!strings.HasPrefix(candidate, projectKey+"/")) {
			continue
		}
		if len(projectKey) > len(ownerKey) {
			owner = project
			ownerKey = projectKey
		}
	}
	return owner.Root, owner.Name
}

// buildPythonInterpreterRows ports OpenIDEA's buildInterpreters and uniqueNames:
// detected/current-project interpreters, other projects' interpreters, added
// paths, invalid paths, and user-hidden entries all share one deduplicated list.
func buildPythonInterpreterRows(
	detected []openidea.Sdk,
	fromProjects []openidea.ProjectInterpreter,
	added, hidden []string,
	current, currentProject string,
	projects []pythonInterpreterProject,
	probed map[string]*openidea.Sdk,
) []pythonInterpreterRow {
	rows := []pythonInterpreterRow{}
	seen := map[string]int{}
	hiddenSet := map[string]bool{}
	addedSet := map[string]bool{}
	for _, path := range hidden {
		hiddenSet[pythonInterpreterPathKey(path, currentProject)] = true
	}
	for _, path := range added {
		addedSet[pythonInterpreterPathKey(path, currentProject)] = true
	}
	add := func(sdk openidea.Sdk, project string, isAdded bool) {
		key := pythonInterpreterPathKey(sdk.Path, currentProject)
		if key == "" {
			return
		}
		if i, exists := seen[key]; exists {
			rows[i].Added = rows[i].Added || isAdded
			if rows[i].Project == "" && project != "" {
				rows[i].Project = project
				rows[i].ProjectName = filepath.Base(project)
			}
			return
		}
		owner, ownerName := owningPythonInterpreterProject(project, currentProject, projects)
		if project != "" {
			owner = project
			ownerName = filepath.Base(project)
		} else {
			home := sdk.Path
			if sdk.Home != nil && strings.TrimSpace(*sdk.Home) != "" {
				home = *sdk.Home
			}
			owner, ownerName = owningPythonInterpreterProject(home, currentProject, projects)
		}
		seen[key] = len(rows)
		rows = append(rows, pythonInterpreterRow{
			SDK:         sdk,
			Current:     samePythonInterpreterPath(sdk.Path, current, currentProject),
			Hidden:      hiddenSet[key],
			Added:       isAdded || addedSet[key],
			Project:     owner,
			ProjectName: ownerName,
		})
	}
	for _, sdk := range detected {
		add(sdk, "", false)
	}
	for _, entry := range fromProjects {
		add(entry.SDK, entry.Project, false)
	}
	missing := make([]string, 0)
	for _, path := range added {
		key := pythonInterpreterPathKey(path, currentProject)
		if _, ok := seen[key]; ok {
			continue
		}
		if probedSDK, probedAlready := probed[path]; probedAlready {
			if probedSDK != nil {
				add(*probedSDK, "", true)
			} else {
				missing = append(missing, path)
			}
			continue
		}
		add(openidea.Sdk{Path: path, Name: path, Source: "custom"}, "", true)
	}
	for _, path := range missing {
		key := pythonInterpreterPathKey(path, currentProject)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = len(rows)
		rows = append(rows, pythonInterpreterRow{
			SDK:         openidea.Sdk{Path: path, Name: path, Source: "custom"},
			Hidden:      hiddenSet[key],
			Added:       true,
			Invalid:     true,
			Project:     currentProject,
			ProjectName: filepath.Base(currentProject),
		})
	}
	if current != "" {
		if i, ok := seen[pythonInterpreterPathKey(current, currentProject)]; ok {
			rows[i].Current = true
			rows[i].Hidden = false
		} else {
			rows = append(rows, pythonInterpreterRow{
				SDK:         openidea.Sdk{Path: current, Name: current, Source: "custom"},
				Current:     true,
				Invalid:     true,
				Project:     currentProject,
				ProjectName: filepath.Base(currentProject),
			})
		}
	}
	filtered := rows[:0]
	for _, row := range rows {
		if row.Hidden && !row.Current {
			continue
		}
		filtered = append(filtered, row)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Invalid != filtered[j].Invalid {
			return !filtered[i].Invalid
		}
		left := strings.ToLower(filtered[i].SDK.Name)
		right := strings.ToLower(filtered[j].SDK.Name)
		if left != right {
			return left < right
		}
		return filtered[i].SDK.Path < filtered[j].SDK.Path
	})
	totals := map[string]int{}
	for _, row := range filtered {
		totals[row.SDK.Name]++
	}
	indexes := map[string]int{}
	for i := range filtered {
		if totals[filtered[i].SDK.Name] < 2 {
			continue
		}
		indexes[filtered[i].SDK.Name]++
		if indexes[filtered[i].SDK.Name] > 1 {
			filtered[i].SDK.Name += fmt.Sprintf(" (%d)", indexes[filtered[i].SDK.Name])
		}
	}
	return filtered
}

func (m *Model) pythonInterpreterAdded() []string {
	return nativeSettingsListValue(m, settingsPagePythonInterpret, "languages.python.interpreters")
}

func (m *Model) setPythonInterpreterAdded(values []string) {
	m.setNativeSettingsValue(settingsPagePythonInterpret, "languages.python.interpreters", uniqueLanguageSDKPaths(values))
}

func (m *Model) pythonInterpreterHidden() []string {
	return nativeSettingsListValue(m, settingsPagePythonInterpret, "languages.python.hiddenInterpreters")
}

func (m *Model) setPythonInterpreterHidden(values []string) {
	m.setNativeSettingsValue(settingsPagePythonInterpret, "languages.python.hiddenInterpreters", uniqueLanguageSDKPaths(values))
}

func uniqueLanguageSDKPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		found := false
		for _, existing := range out {
			if sameLanguageSDKPath(existing, path) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, path)
		}
	}
	return out
}

func (m *Model) adoptPythonInterpreter(sdk openidea.Sdk, makeCurrent bool) {
	if sdk.Path == "" {
		return
	}
	root := m.Root()
	if m.languageSDKs.known == nil {
		m.languageSDKs.known = map[string]openidea.Sdk{}
	}
	key := languageSDKPathKey(sdk.Path)
	m.languageSDKs.known[key] = sdk
	added := m.pythonInterpreterAdded()
	found := false
	for _, path := range added {
		if samePythonInterpreterPath(path, sdk.Path, root) {
			found = true
			break
		}
	}
	if !found {
		added = append(added, sdk.Path)
	}
	m.setPythonInterpreterAdded(added)
	hidden := m.pythonInterpreterHidden()
	kept := hidden[:0]
	for _, path := range hidden {
		if !samePythonInterpreterPath(path, sdk.Path, root) {
			kept = append(kept, path)
		}
	}
	m.setPythonInterpreterHidden(kept)
	if makeCurrent {
		m.setNativeSettingsValue(settingsPagePythonInterpret, "languages.python.interpreter", sdk.Path)
	}
	if m.languageSDKs.kinds == nil {
		m.languageSDKs.kinds = map[openidea.SdkKind]languageSDKDetection{}
	}
	state := m.languageSDKs.kinds[openidea.SdkPython]
	duplicate := false
	for _, entry := range state.sdks {
		if samePythonInterpreterPath(entry.Path, sdk.Path, root) {
			duplicate = true
			break
		}
	}
	if !duplicate {
		state.sdks = append([]openidea.Sdk{sdk}, state.sdks...)
		m.languageSDKs.kinds[openidea.SdkPython] = state
	}
	m.invalidate()
}

func (m *Model) removePythonInterpreter(row pythonInterpreterRow) {
	root := m.Root()
	added := m.pythonInterpreterAdded()
	kept := added[:0]
	for _, path := range added {
		if !samePythonInterpreterPath(path, row.SDK.Path, root) {
			kept = append(kept, path)
		}
	}
	m.setPythonInterpreterAdded(kept)
	hidden := m.pythonInterpreterHidden()
	found := false
	for _, path := range hidden {
		if samePythonInterpreterPath(path, row.SDK.Path, root) {
			found = true
			break
		}
	}
	if !found {
		hidden = append(hidden, row.SDK.Path)
	}
	m.setPythonInterpreterHidden(hidden)
	if row.Current && samePythonInterpreterPath(
		nativeSettingsString(m, settingsPagePythonInterpret, "languages.python.interpreter"),
		row.SDK.Path,
		root,
	) {
		m.setNativeSettingsValue(settingsPagePythonInterpret, "languages.python.interpreter", "")
	}
	m.invalidate()
}

func (m *Model) restorePythonInterpreter(row pythonInterpreterRow) {
	root := m.Root()
	hidden := m.pythonInterpreterHidden()
	kept := hidden[:0]
	for _, path := range hidden {
		if !samePythonInterpreterPath(path, row.SDK.Path, root) {
			kept = append(kept, path)
		}
	}
	m.setPythonInterpreterHidden(kept)
}

func (m *Model) addExistingPythonInterpreter(path string) {
	path = strings.TrimSpace(path)
	root := m.Root()
	if path == "" || m.languageSDKs.python.busy {
		return
	}
	flow := m.languageSDKs.python
	flow.root = root
	flow.busy = true
	flow.err = ""
	flow.gen++
	generation := flow.gen
	m.languageSDKs.python = flow
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "sdk_python_interpreter", map[string]any{"path": path})
		var sdk *openidea.Sdk
		if err == nil {
			err = json.Unmarshal(raw, &sdk)
		}
		m.post(func() {
			current := m.languageSDKs.python
			if current.gen != generation || m.Root() != root {
				return
			}
			current.busy = false
			if err != nil {
				current.err = "Could not check the interpreter: " + err.Error()
			} else if sdk == nil {
				current.err = "Cannot run this Python interpreter. Choose a working executable or virtualenv folder."
			} else {
				m.adoptPythonInterpreter(*sdk, false)
			}
			m.languageSDKs.python = current
			m.invalidate()
		})
	}()
}

func (m *Model) createPythonEnvironment(tool, base, location string) {
	tool = strings.TrimSpace(tool)
	base = strings.TrimSpace(base)
	location = strings.TrimSpace(location)
	root := m.Root()
	if root == "" || m.languageSDKs.python.busy || base == "" || location == "" {
		return
	}
	flow := m.languageSDKs.python
	flow.root = root
	flow.busy = true
	flow.err = ""
	flow.log = nil
	flow.request = m.nextRequest()
	flow.channel = m.nextChannel()
	flow.gen++
	generation := flow.gen
	request, channel := flow.request, flow.channel
	m.languageSDKs.python = flow
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "sdk_python_create_venv", map[string]any{
			"tool": tool, "base": base, "location": location,
			"request": request, "onOutput": map[string]any{"$channel": channel},
		})
		var sdk openidea.Sdk
		if err == nil {
			err = json.Unmarshal(raw, &sdk)
		}
		m.post(func() {
			current := m.languageSDKs.python
			if current.gen != generation || m.Root() != root {
				return
			}
			current.busy = false
			current.request = 0
			current.channel = 0
			if err != nil {
				if ctx.Err() == context.Canceled {
					current.err = ""
				} else {
					current.err = "Could not create the environment: " + err.Error()
				}
			} else {
				m.adoptPythonInterpreter(sdk, true)
			}
			m.languageSDKs.python = current
			m.invalidate()
		})
	}()
}

func (m *Model) cancelPythonEnvironment() {
	flow := m.languageSDKs.python
	if !flow.busy || flow.request == 0 || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	request := flow.request
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"request": request})
	}()
}

func (m *Model) acceptLanguageSDKChunk(id uint32, data json.RawMessage) bool {
	if m.acceptPythonPackageChunk(id, data) {
		return true
	}
	flow := m.languageSDKs.python
	if id == 0 || flow.channel == 0 || id != flow.channel {
		return false
	}
	var line string
	if json.Unmarshal(data, &line) != nil {
		return true
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	flow.log = append(flow.log, line)
	if len(flow.log) > 200 {
		flow.log = append([]string(nil), flow.log[len(flow.log)-200:]...)
	}
	m.languageSDKs.python = flow
	m.invalidate()
	return true
}

func (m *Model) resetLanguageSDKSession() {
	// Keep request/channel identities so late channel events from a canceled
	// flow cannot be mistaken for a new one.
	flow := m.languageSDKs.python
	if flow.busy {
		m.cancelPythonEnvironment()
	}
	packages := m.languageSDKs.packages
	if packages.job != nil {
		m.cancelPythonPackageJob()
	}
	m.languageSDKs = languageSDKSettingsState{
		python: pythonInterpreterFlow{
			gen:     flow.gen + 1,
			request: flow.request,
			channel: flow.channel,
		},
		packages: pythonPackagesState{jobGen: packages.jobGen + 1},
	}
}

func (m *Model) renderPythonInterpreters(c *ui.Context, oi oiColors, field nativeSettingsField) {
	page := settingsPagePythonInterpret
	m.ensurePythonProjectInterpreters(false)
	m.probePythonInterpreters()
	pageState := m.nativeSettings.pages[page]
	if pageState.aux == nil {
		pageState.aux = map[string]string{}
	}
	root := m.Root()
	detected := m.languageSDKList(openidea.SdkPython)
	catalog := m.languageSDKs.catalog
	projects := pythonInterpreterProjects(root, catalog.projects)
	rows := buildPythonInterpreterRows(
		detected,
		catalog.projects,
		m.pythonInterpreterAdded(),
		m.pythonInterpreterHidden(),
		nativeSettingsString(m, page, "languages.python.interpreter"),
		root,
		projects,
		catalog.probed,
	)
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, field.label).FontSize(12).Bold().TextColor(oi.text).Grow(1).SingleLine()
		ui.Button(c, "Add existing…").OnClick(func() {
			m.chooseExistingPythonInterpreter()
		})
		ui.Button(c, "Refresh").OnClick(func() {
			m.refreshPythonInterpreters()
		})
	})
	query := pageState.aux["python.interpreters.search"]
	hideOther := nativeSettingsBool(m, page, "languages.python.hideOtherProjectInterpreters")
	showPaths := nativeSettingsBool(m, page, "languages.python.showInterpreterPaths")
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		input := ui.TextInput(c, &query).Grow(1).Placeholder("Search interpreters")
		if input.Changed() {
			pageState := m.nativeSettings.pages[page]
			if pageState.aux == nil {
				pageState.aux = map[string]string{}
			}
			pageState.aux["python.interpreters.search"] = query
			m.nativeSettings.pages[page] = pageState
		}
		hint := fmt.Sprintf("%d total", len(rows))
		if hideOther {
			hint += " · other projects hidden"
		}
		ui.Text(c, hint).FontSize(10).TextColor(oi.textMuted).SingleLine()
	})
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	shown := make([]pythonInterpreterRow, 0, len(rows))
	for _, row := range rows {
		if hideOther && root != "" && row.Project != "" &&
			!samePythonInterpreterPath(row.Project, root, root) {
			continue
		}
		if len(terms) > 0 {
			haystack := strings.ToLower(strings.Join([]string{
				row.SDK.Name, row.SDK.Path, row.SDK.Source, row.Project,
			}, "\n"))
			matches := true
			for _, term := range terms {
				if !strings.Contains(haystack, term) {
					matches = false
					break
				}
			}
			if !matches {
				continue
			}
		}
		shown = append(shown, row)
	}
	if len(rows) == 0 {
		if catalog.loading || m.languageSDKs.kinds[openidea.SdkPython].loading {
			ui.Text(c, "Detecting interpreters…").
				FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine()
		} else {
			ui.Text(c, "No interpreters found. Add one or create an environment below.").
				FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
		}
	} else if len(shown) == 0 {
		ui.Text(c, "No interpreters match the search or filter.").
			FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
	}
	for _, row := range shown {
		row := row
		ui.Row(c).MinHeight(34).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			name := row.SDK.Name
			if name == "" {
				name = filepath.Base(row.SDK.Path)
			}
			detail := row.SDK.Path
			if !showPaths {
				detail = row.SDK.Source
			} else if row.Project != "" {
				detail = row.ProjectName + " · " + detail
			}
			ui.Column(c).Grow(1).Children(func() {
				ui.Row(c).Gap(6).Children(func() {
					ui.Text(c, name).FontSize(12).TextColor(oi.text).SingleLine().Ellipsis("…")
					if row.Current {
						ui.Text(c, "Project interpreter").FontSize(10).TextColor(oi.accent).SingleLine()
					}
					if row.Invalid {
						ui.Text(c, "Invalid").FontSize(10).TextColor(oi.danger).SingleLine()
					}
					if row.Project != "" && !samePythonInterpreterPath(row.Project, root, root) {
						ui.Text(c, row.ProjectName).FontSize(10).TextColor(oi.textMuted).SingleLine()
					}
				})
				if detail != "" {
					ui.Text(c, detail).FontSize(10).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
				}
			})
			if !row.Current && !row.Invalid {
				ui.Button(c, "Set").Width(48).OnClick(func() {
					m.setNativeSettingsValue(page, "languages.python.interpreter", row.SDK.Path)
				})
			}
			if row.Hidden {
				ui.Button(c, "Restore").Width(64).OnClick(func() {
					m.restorePythonInterpreter(row)
				})
			}
			ui.Button(c, "Remove").Width(68).OnClick(func() {
				m.removePythonInterpreter(row)
			})
		})
	}
	if catalog.err != "" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			ui.Text(c, "Could not read interpreters from other projects: "+catalog.err).
				FontSize(11).TextColor(oi.danger).Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "Retry").OnClick(func() {
				m.ensurePythonProjectInterpreters(true)
			})
		})
	}
	flow := m.languageSDKs.python
	if flow.err != "" {
		ui.Text(c, flow.err).FontSize(11).TextColor(oi.danger).PaddingStart(20).SingleLine().Ellipsis("…")
	}
	if flow.busy {
		ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			ui.Text(c, "Working…").FontSize(11).TextColor(oi.textMuted).Grow(1).SingleLine()
			ui.Button(c, "Cancel").Width(64).OnClick(func() {
				m.cancelPythonEnvironment()
			})
		})
		if len(flow.log) > 0 {
			ui.Text(c, flow.log[len(flow.log)-1]).FontSize(10).TextColor(oi.textMuted).
				PaddingStart(20).SingleLine().Ellipsis("…")
		}
	}
	m.renderPythonEnvironmentCreator(c, oi)
}

func (m *Model) chooseExistingPythonInterpreter() {
	parent := m.win
	root := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       "Select Python Interpreter",
			ButtonLabel: "Choose",
			Directory:   false,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			if m.Root() != root {
				return
			}
			m.addExistingPythonInterpreter(paths[0])
		})
	}()
}

func (m *Model) renderPythonEnvironmentCreator(c *ui.Context, oi oiColors) {
	page := settingsPagePythonInterpret
	state := m.nativeSettings.pages[page]
	if state.aux == nil {
		state.aux = map[string]string{}
	}
	settingsSectionTitle(c, oi, "Create Environment")
	tool := state.aux["python.venv.tool"]
	if tool == "" {
		tool = "venv"
	}
	tools := []string{"venv", "uv", "poetry", "conda"}
	toolLabels := []string{"Virtualenv", "uv", "Poetry", "Conda"}
	shownTool := toolLabels[0]
	for i, value := range tools {
		if value == tool {
			shownTool = toolLabels[i]
		}
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Environment type", 230)
		if ui.Select(c, &shownTool, toolLabels).Grow(1).Changed() {
			for i, label := range toolLabels {
				if shownTool == label {
					state.aux["python.venv.tool"] = tools[i]
					break
				}
			}
			m.nativeSettings.pages[page] = state
			m.invalidate()
		}
	})
	detected := m.languageSDKList(openidea.SdkPython)
	bases := detected[:0]
	for _, sdk := range detected {
		if sdk.Source != "venv" {
			bases = append(bases, sdk)
		}
	}
	base := state.aux["python.venv.base"]
	if base == "" && len(bases) > 0 {
		base = bases[0].Path
	}
	baseValues, baseLabels := nativeSDKChoices(bases, base, "Select a base interpreter", false)
	if len(baseValues) > 0 {
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, "Base interpreter", 230)
			shown := baseLabels[0]
			for i, value := range baseValues {
				if sameLanguageSDKPath(value, base) {
					shown = baseLabels[i]
					break
				}
			}
			if ui.Select(c, &shown, baseLabels).Grow(1).Changed() {
				for i, label := range baseLabels {
					if shown == label {
						state.aux["python.venv.base"] = baseValues[i]
						break
					}
				}
				m.nativeSettings.pages[page] = state
				m.invalidate()
			}
		})
	}
	location := state.aux["python.venv.location"]
	if location == "" {
		location = filepath.Join(m.Root(), ".venv")
	}
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		input := ui.TextInput(c, &location).Grow(1).Placeholder(filepath.Join(".venv"))
		if input.Changed() {
			state.aux["python.venv.location"] = location
			m.nativeSettings.pages[page] = state
		}
		ui.Button(c, "Browse…").Width(82).OnClick(func() {
			parent := m.win
			root := m.Root()
			go func() {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
					Parent:      parent,
					Title:       "Select Environment Directory",
					ButtonLabel: "Choose",
					DefaultPath: location,
					Directory:   true,
				})
				if err != nil || len(paths) == 0 {
					return
				}
				m.post(func() {
					if m.Root() != root {
						return
					}
					current := m.nativeSettings.pages[page]
					if current.aux == nil {
						current.aux = map[string]string{}
					}
					current.aux["python.venv.location"] = filepath.Clean(paths[0])
					m.nativeSettings.pages[page] = current
					m.invalidate()
				})
			}()
		})
	})
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		ui.Button(c, "Create").Width(72).OnClick(func() {
			m.createPythonEnvironment(tool, base, location)
		})
		ui.Text(c, "The new interpreter becomes this project's interpreter.").
			FontSize(11).TextColor(oi.textMuted).Grow(1).SingleLine().Ellipsis("…")
	})
}

func nativeSDKPageRequiresRuntimeReload(pages map[string]nativeSettingsPageState) bool {
	for page := range pages {
		switch page {
		case settingsPagePythonDeps, settingsPagePythonInterpret,
			settingsPageGo, settingsPageGoBuildTags, settingsPageGoModules,
			settingsPageNode, settingsPagePHP, settingsPageRuby, settingsPageRust,
			settingsPageTypeScript, settingsPageGradle, settingsPageMaven,
			settingsPageJava:
			return true
		}
	}
	return false
}

func languageSDKSettingsError(prefix string, err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %v", prefix, err)
}
