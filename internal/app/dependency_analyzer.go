package app

// Dependency Analyzer backed by the pinned server's dependency RPCs. Maven's
// verbose dependency tree arrives in build_model after build_sync. Gradle's
// per-configuration report is fetched on demand through gradle_dependencies.
// Exclude and jump-to-source use maven_exclude_dependency and
// maven_dependency_declaration, the same operations as OpenIDEA.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

var gradleDependencyConfigurations = []string{
	"compileClasspath",
	"runtimeClasspath",
	"testCompileClasspath",
	"testRuntimeClasspath",
}

type buildDependencyAnalyzer struct {
	open       bool
	module     string
	moduleName string
	config     string
	mode       string
	query      string
	conflicts  bool
	scope      string
	selected   string
	expanded   map[string]bool

	deps    []mavenDependency
	loading bool
	err     string
	request uint32
	gen     uint64
}

type analyzerModule struct {
	key      string
	label    string
	tool     string
	pom      string
	buildDir string
	path     string
	name     string
}

type dependencyUsage struct {
	path       []mavenDependency
	dependency mavenDependency
}

type analyzedArtifact struct {
	key        string
	groupID    string
	artifact   string
	classifier string
	version    string
	scope      string
	requested  []string
	conflict   bool
	usages     []dependencyUsage
}

type dependencyTreeNode struct {
	key        string
	dependency mavenDependency
	path       []mavenDependency
	children   []dependencyTreeNode
}

type mavenDependencyLocation struct {
	Path  string                `json:"path"`
	Range openidea.OutlineRange `json:"range"`
}

func (m *Model) OpenBuildDependencyAnalyzer(arg any) {
	module := m.analyzerModuleForArg(arg)
	modules := m.analyzerModules()
	if module.key == "" && len(modules) > 0 {
		module = modules[0]
	}
	if previous := m.maven.analyzer; previous != nil {
		m.cancelDependencyAnalyzer(previous)
	}
	a := &buildDependencyAnalyzer{
		open:     true,
		module:   module.key,
		mode:     "list",
		scope:    "*",
		config:   "runtimeClasspath",
		expanded: map[string]bool{},
	}
	a.moduleName = module.label
	m.maven.analyzer = a
	if module.tool == "gradle" {
		m.reloadDependencyAnalyzer()
	}
	m.invalidate()
}

func (m *Model) analyzerModuleForArg(arg any) analyzerModule {
	pom := buildToolArgString(arg, "pom")
	buildDir := buildToolArgString(arg, "buildDir")
	gradlePath := buildToolArgString(arg, "gradlePath")
	path := buildToolArgString(arg, "path")
	if path != "" {
		if project, build := m.gradleProjectForBuildFile(path); project != nil {
			buildDir, gradlePath = build.Dir, project.Path
		}
	}
	if pom != "" {
		return analyzerModule{
			key: "mvn:" + pom, label: m.analyzerMavenLabel(pom), tool: "maven", pom: pom,
		}
	}
	if gradlePath != "" || buildDir != "" {
		module := analyzerModule{
			key:   "gradle:" + buildDir + "\x00" + gradlePath,
			label: m.analyzerGradleLabel(buildDir, gradlePath),
			tool:  "gradle", buildDir: buildDir, path: gradlePath,
		}
		return module
	}
	if m.maven.activeTool == "gradle" || m.maven.gradleVisible {
		if selection := m.maven.gradleSel; selection.build != nil && selection.project != nil {
			return analyzerModule{
				key:      "gradle:" + selection.build.Dir + "\x00" + selection.project.Path,
				label:    m.analyzerGradleLabel(selection.build.Dir, selection.project.Path),
				tool:     "gradle",
				buildDir: selection.build.Dir,
				path:     selection.project.Path,
			}
		}
	}
	project := m.maven.sel.project
	if project == nil {
		project = m.mavenRootProject()
	}
	if project != nil {
		return analyzerModule{
			key: "mvn:" + project.Pom, label: m.analyzerMavenLabel(project.Pom), tool: "maven", pom: project.Pom,
		}
	}
	if len(m.maven.model.Gradle) > 0 {
		build := &m.maven.model.Gradle[0]
		project := gradleRootProject(build)
		if project != nil {
			return analyzerModule{
				key:      "gradle:" + build.Dir + "\x00" + project.Path,
				label:    m.analyzerGradleLabel(build.Dir, project.Path),
				tool:     "gradle",
				buildDir: build.Dir,
				path:     project.Path,
			}
		}
	}
	return analyzerModule{}
}

func (m *Model) analyzerModules() []analyzerModule {
	var out []analyzerModule
	for _, project := range m.maven.model.MavenProjects() {
		if project.Packaging == "pom" && len(project.Dependencies) == 0 {
			continue
		}
		out = append(out, analyzerModule{
			key: "mvn:" + project.Pom, label: m.analyzerMavenLabel(project.Pom),
			tool: "maven", pom: project.Pom, name: project.DisplayName,
		})
	}
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		for j := range build.Projects {
			project := &build.Projects[j]
			if project.Build != nil && strings.HasSuffix(*project.Build, ":buildSrc") {
				continue
			}
			out = append(out, analyzerModule{
				key:      "gradle:" + build.Dir + "\x00" + project.Path,
				label:    m.analyzerGradleLabel(build.Dir, project.Path),
				tool:     "gradle",
				buildDir: build.Dir,
				path:     project.Path,
				name:     project.Name,
			})
		}
	}
	return out
}

func (m *Model) analyzerMavenLabel(pom string) string {
	for _, project := range m.maven.model.MavenProjects() {
		if project.Pom == pom {
			name := strings.TrimSpace(project.DisplayName)
			if name == "" {
				name = project.ArtifactID
			}
			if name == "" {
				name = "Maven"
			}
			if pom != "" {
				return name + " · " + pom
			}
			return name
		}
	}
	if pom == "" {
		return "Maven"
	}
	return pom
}

func (m *Model) analyzerGradleLabel(buildDir, path string) string {
	name, buildFile := path, ""
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		if build.Dir != buildDir {
			continue
		}
		buildFile = build.Dir
		for j := range build.Projects {
			if build.Projects[j].Path == path {
				name = build.Projects[j].Name
				if build.Projects[j].BuildFile != nil {
					buildFile = filepathToSlashJoin(build.Dir, *build.Projects[j].BuildFile)
				}
				break
			}
		}
	}
	if strings.TrimSpace(name) == "" {
		name = path
	}
	if name == "" {
		name = "Gradle"
	}
	if buildFile == "" {
		buildFile = buildDir
	}
	if buildFile == "" {
		buildFile = "."
	}
	return name + " · " + buildFile + " " + path
}

func filepathToSlashJoin(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return base
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	if base == "" || base == "." {
		return path
	}
	return strings.TrimSuffix(base, "/") + "/" + path
}

func (m *Model) findAnalyzerModule(key string) (analyzerModule, bool) {
	for _, module := range m.analyzerModules() {
		if module.key == key {
			return module, true
		}
	}
	return analyzerModule{}, false
}

func (m *Model) analyzerCurrentModule() (analyzerModule, bool) {
	a := m.maven.analyzer
	if a == nil {
		return analyzerModule{}, false
	}
	return m.findAnalyzerModule(a.module)
}

func (m *Model) CloseBuildDependencyAnalyzer() {
	a := m.maven.analyzer
	if a == nil {
		return
	}
	m.cancelDependencyAnalyzer(a)
	a.open = false
	m.maven.analyzer = nil
	m.invalidate()
}

func (m *Model) cancelDependencyAnalyzer(a *buildDependencyAnalyzer) {
	if a == nil || a.request == 0 || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	request := a.request
	a.request = 0
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

func (m *Model) reloadDependencyAnalyzer() {
	a := m.maven.analyzer
	module, ok := m.analyzerCurrentModule()
	if a == nil || !ok || module.tool != "gradle" {
		return
	}
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	if a.request != 0 {
		m.cancelDependencyAnalyzer(a)
	}
	request := m.nextRequest()
	a.request = request
	a.gen++
	gen := a.gen
	a.loading = true
	a.err = ""
	moduleKey := a.module
	configuration := a.config
	var buildDir any
	if module.buildDir != "" {
		buildDir = module.buildDir
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "gradle_dependencies", map[string]any{
			"gradlePath":    module.path,
			"configuration": configuration,
			"offline":       m.maven.gradleOffline,
			"request":       request,
			"buildDir":      buildDir,
		})
		var deps []gradleDependency
		if err == nil {
			err = json.Unmarshal(raw, &deps)
		}
		m.post(func() {
			if m.maven.analyzer != a || a.gen != gen || a.module != moduleKey {
				return
			}
			a.request = 0
			a.loading = false
			if err != nil {
				a.err = err.Error()
				m.invalidate()
				return
			}
			a.deps = mavenDependenciesFromGradle(deps)
			a.selected = ""
			a.expanded = map[string]bool{}
			m.invalidate()
		})
	}()
}

func (m *Model) chooseDependencyAnalyzerModule(key string) {
	a := m.maven.analyzer
	if a == nil {
		return
	}
	module, ok := m.findAnalyzerModule(key)
	if !ok {
		return
	}
	m.cancelDependencyAnalyzer(a)
	a.module = key
	a.moduleName = module.label
	a.selected = ""
	a.expanded = map[string]bool{}
	a.deps = nil
	a.err = ""
	a.loading = false
	if module.tool == "gradle" {
		m.reloadDependencyAnalyzer()
	} else {
		m.invalidate()
	}
}

func (m *Model) chooseDependencyAnalyzerConfiguration(config string) {
	a := m.maven.analyzer
	if a == nil || config == a.config {
		return
	}
	a.config = config
	m.reloadDependencyAnalyzer()
}

func (m *Model) dependencyAnalyzerDependencies() []mavenDependency {
	a := m.maven.analyzer
	module, ok := m.analyzerCurrentModule()
	if a == nil || !ok {
		return nil
	}
	if module.tool == "gradle" {
		return a.deps
	}
	if project := m.mavenProjectByPom(module.pom); project != nil {
		return project.Dependencies
	}
	return nil
}

func (m *Model) mavenDependencyDeclaration(
	ctx context.Context,
	pom string,
	dep mavenDependency,
) (mavenDependencyLocation, error) {
	var location mavenDependencyLocation
	raw, err := m.invoke(ctx, "maven_dependency_declaration", map[string]any{
		"pom": pom,
		"dependency": map[string]any{
			"groupId": dep.GroupID, "artifactId": dep.ArtifactID,
		},
	})
	if err != nil {
		return location, err
	}
	err = json.Unmarshal(raw, &location)
	return location, err
}

func (m *Model) excludeMavenDependency(
	ctx context.Context,
	pom string,
	direct, exclusion mavenDependency,
) error {
	_, err := m.invoke(ctx, "maven_exclude_dependency", map[string]any{
		"pom": pom,
		"dependency": map[string]any{
			"groupId": direct.GroupID, "artifactId": direct.ArtifactID,
		},
		"exclusion": map[string]any{
			"groupId": exclusion.GroupID, "artifactId": exclusion.ArtifactID,
		},
	})
	return err
}

func (m *Model) JumpToMavenDependency(pom string, dep mavenDependency) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		location, err := m.mavenDependencyDeclaration(ctx, pom, dep)
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.status = "Cannot find the declaration: " + err.Error()
				m.invalidate()
				return
			}
			m.revealRange(
				location.Path,
				int(location.Range.Start.Line), int(location.Range.Start.Col),
				int(location.Range.End.Line), int(location.Range.End.Col),
			)
		})
	}()
}

func (m *Model) ExcludeMavenDependency(pom string, direct, exclusion mavenDependency) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	root := m.Root()
	m.status = "Excluding " + exclusion.GroupID + ":" + exclusion.ArtifactID + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		err := m.excludeMavenDependency(ctx, pom, direct, exclusion)
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.status = "Cannot exclude the dependency: " + err.Error()
				m.invalidate()
				return
			}
			m.status = "Excluded " + exclusion.GroupID + ":" + exclusion.ArtifactID
			m.SyncMaven()
			m.invalidate()
		})
	}()
}

func (m *Model) analyzerJumpToSource(usage dependencyUsage) {
	module, ok := m.analyzerCurrentModule()
	if !ok || module.tool != "maven" || len(usage.path) == 0 {
		return
	}
	m.JumpToMavenDependency(module.pom, usage.path[0])
}

func (m *Model) analyzerExclude(usage dependencyUsage) {
	module, ok := m.analyzerCurrentModule()
	target, derived := dependencyExclusionFor(usage)
	if !ok || module.tool != "maven" || !derived {
		return
	}
	m.ExcludeMavenDependency(module.pom, target.direct, target.exclusion)
}

type dependencyExclusion struct {
	direct    mavenDependency
	exclusion mavenDependency
}

func dependencyExclusionFor(usage dependencyUsage) (dependencyExclusion, bool) {
	if len(usage.path) < 2 {
		return dependencyExclusion{}, false
	}
	return dependencyExclusion{
		direct: usage.path[0], exclusion: usage.dependency,
	}, true
}

func mavenDependenciesFromGradle(deps []gradleDependency) []mavenDependency {
	out := make([]mavenDependency, 0, len(deps))
	for _, dep := range deps {
		if dep.Constraint {
			continue
		}
		requested := ""
		if dep.Requested != nil {
			requested = plainGradleVersion(*dep.Requested)
		}
		selected := ""
		if dep.Selected != nil {
			selected = plainGradleVersion(*dep.Selected)
		}
		version := selected
		if dep.Project {
			if selected != "" {
				version = "project " + selected
			}
			requested = ""
		} else if version == "" {
			version = requested
		}
		var requestedDiff *string
		if !dep.Project && requested != "" && version != "" && requested != version && exactGradleVersion(requested) {
			value := requested
			requestedDiff = &value
		}
		out = append(out, mavenDependency{
			GroupID: dep.Group, ArtifactID: dep.Name, Version: version,
			Type: "jar", Scope: "compile", Children: mavenDependenciesFromGradle(dep.Children),
			Requested: requestedDiff, Unresolved: dep.Unresolved, GradleBuild: dep.Project,
		})
	}
	return out
}

func plainGradleVersion(version string) string {
	version = strings.TrimSpace(version)
	for _, prefix := range []string{"{strictly ", "{require ", "{prefer "} {
		if strings.HasPrefix(version, prefix) && strings.HasSuffix(version, "}") {
			return strings.TrimSuffix(strings.TrimPrefix(version, prefix), "}")
		}
	}
	return version
}

func exactGradleVersion(version string) bool {
	return !strings.ContainsAny(version, "[](),+{}") && !strings.Contains(version, "latest.")
}

func dependencyArtifactKey(dep mavenDependency) string {
	parts := []string{dep.GroupID, dep.ArtifactID}
	if dep.Classifier != nil && *dep.Classifier != "" {
		parts = append(parts, *dep.Classifier)
	}
	return strings.Join(parts, ":")
}

func dependencyRequestedVersion(dep mavenDependency) string {
	if dep.Requested != nil {
		return *dep.Requested
	}
	if dep.Managed != nil && *dep.Managed != "" {
		return *dep.Managed
	}
	return dep.Version
}

func analyzeMavenDependencies(deps []mavenDependency) []analyzedArtifact {
	byKey := map[string]*analyzedArtifact{}
	var walk func([]mavenDependency, []mavenDependency)
	walk = func(list []mavenDependency, parents []mavenDependency) {
		for _, dep := range list {
			path := append(append([]mavenDependency(nil), parents...), dep)
			key := dependencyArtifactKey(dep)
			artifact := byKey[key]
			if artifact == nil {
				artifact = &analyzedArtifact{
					key: key, groupID: dep.GroupID, artifact: dep.ArtifactID,
					classifier: stringValue(dep.Classifier), scope: dep.Scope,
				}
				byKey[key] = artifact
			}
			artifact.usages = append(artifact.usages, dependencyUsage{path: path, dependency: dep})
			if dep.Omitted == nil && artifact.version == "" {
				artifact.version = dep.Version
				artifact.scope = dep.Scope
			}
			requested := dependencyRequestedVersion(dep)
			if requested != "" && !containsString(artifact.requested, requested) {
				artifact.requested = append(artifact.requested, requested)
			}
			if (dep.Omitted != nil && *dep.Omitted == "conflict") || dep.Requested != nil {
				artifact.conflict = true
			}
			walk(dep.Children, path)
		}
	}
	walk(deps, nil)
	out := make([]analyzedArtifact, 0, len(byKey))
	for _, artifact := range byKey {
		out = append(out, *artifact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func dependencyScopesOf(artifacts []analyzedArtifact) []string {
	order := []string{"compile", "provided", "runtime", "test", "system", "import"}
	found := map[string]bool{}
	for _, artifact := range artifacts {
		for _, usage := range artifact.usages {
			found[usage.dependency.Scope] = true
		}
	}
	var out []string
	for _, scope := range order {
		if found[scope] {
			out = append(out, scope)
			delete(found, scope)
		}
	}
	var rest []string
	for scope := range found {
		rest = append(rest, scope)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func filterAnalyzedArtifacts(
	artifacts []analyzedArtifact,
	query string,
	conflicts bool,
	scope string,
) []analyzedArtifact {
	query = strings.ToLower(strings.TrimSpace(query))
	var out []analyzedArtifact
	for _, artifact := range artifacts {
		if conflicts && !artifact.conflict {
			continue
		}
		if scope != "" && scope != "*" {
			matched := false
			for _, usage := range artifact.usages {
				if usage.dependency.Scope == scope {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if query != "" {
			haystack := strings.ToLower(artifact.key + ":" + artifact.version + " " + strings.Join(artifact.requested, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, artifact)
	}
	return out
}

func dependencyTree(
	deps []mavenDependency,
	query string,
	conflicts bool,
	scope string,
	conflicted map[string]bool,
) []dependencyTreeNode {
	narrow := conflicts || strings.TrimSpace(query) != "" || (scope != "" && scope != "*")
	var build func([]mavenDependency, string, []mavenDependency) []dependencyTreeNode
	build = func(list []mavenDependency, parent string, parentPath []mavenDependency) []dependencyTreeNode {
		var out []dependencyTreeNode
		for i, dep := range list {
			key := fmt.Sprintf("%s/%d", parent, i)
			path := append(append([]mavenDependency(nil), parentPath...), dep)
			children := build(dep.Children, key, path)
			matches := true
			if conflicts {
				matches = conflicted[dependencyArtifactKey(dep)]
			}
			if scope != "" && scope != "*" {
				matches = matches && dep.Scope == scope
			}
			if query = strings.TrimSpace(query); query != "" {
				needle := strings.ToLower(query)
				matches = matches && strings.Contains(
					strings.ToLower(dependencyArtifactKey(dep)+":"+dep.Version),
					needle,
				)
			}
			if !narrow || matches || len(children) > 0 {
				out = append(out, dependencyTreeNode{
					key: key, dependency: dep, path: path, children: children,
				})
			}
		}
		return out
	}
	return build(deps, "", nil)
}

func dependencyUsageStatus(dep mavenDependency) string {
	if dep.Unresolved {
		return "not resolved"
	}
	if dep.Omitted != nil {
		switch *dep.Omitted {
		case "conflict":
			if dep.Conflict != nil && *dep.Conflict != "" {
				return "omitted for conflict with " + *dep.Conflict
			}
			return "omitted for conflict"
		case "duplicate":
			return "omitted for duplicate"
		case "cycle":
			return "omitted for cycle"
		}
	}
	if dep.Requested != nil && *dep.Requested != "" {
		return "requested " + *dep.Requested
	}
	if dep.Managed != nil && *dep.Managed != "" {
		return "version managed from " + *dep.Managed
	}
	return "resolved"
}

func sortedDependencyUsages(usages []dependencyUsage) []dependencyUsage {
	out := append([]dependencyUsage(nil), usages...)
	rank := func(usage dependencyUsage) int {
		if usage.dependency.Omitted == nil {
			return 0
		}
		if *usage.dependency.Omitted == "conflict" {
			return 1
		}
		return 2
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return len(out[i].path) < len(out[j].path)
	})
	return out
}

func sameDependencyPath(left, right []mavenDependency) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if dependencyArtifactKey(left[i]) != dependencyArtifactKey(right[i]) ||
			left[i].Version != right[i].Version {
			return false
		}
	}
	return true
}

func (m *Model) gradleProjectForBuildFile(path string) (*gradleProject, *gradleModel) {
	path = filepathClean(path)
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		for j := range build.Projects {
			project := &build.Projects[j]
			if resolved := m.gradleBuildFilePath(build, project); resolved != "" && filepathClean(resolved) == path {
				return project, build
			}
		}
	}
	return nil, nil
}

func filepathClean(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	cleaned := jsonPathClean(path)
	return strings.TrimSuffix(cleaned, "/")
}

func jsonPathClean(path string) string {
	// filepath.Clean is platform-specific on Windows, while build_model emits
	// project-relative slash paths. A small lexical clean keeps comparisons
	// stable on every host.
	absolute := strings.HasPrefix(path, "/")
	parts := strings.Split(path, "/")
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) > 0 && stack[len(stack)-1] != ".." {
				stack = stack[:len(stack)-1]
			} else if !absolute {
				stack = append(stack, part)
			}
		default:
			stack = append(stack, part)
		}
	}
	result := strings.Join(stack, "/")
	if absolute {
		result = "/" + result
	}
	if result == "" {
		return "."
	}
	return result
}

func (a *buildDependencyAnalyzer) selectedArtifact(artifacts []analyzedArtifact) *analyzedArtifact {
	for i := range artifacts {
		if artifacts[i].key == a.selected {
			return &artifacts[i]
		}
	}
	return nil
}
