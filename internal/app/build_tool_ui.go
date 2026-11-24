package app

// Native actions shared by the Maven and Gradle tool windows. The tree data
// always comes from build_model or the dependency RPCs, while these helpers
// wire the same rows to MyGo menus, run configurations, editors and dialogs.

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

var buildToolClipboardWrite = mygo.Clipboard.WriteText

func (m *Model) buildToolErrorBanner(
	c *ui.Context,
	p *ui.Theme,
	key, message string,
	refresh func(),
) {
	oi := oiPaletteOf(p)
	ui.Row(c).MinHeight(28).Padding(2, 10).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		oiIconEl(c, "status/warning", 16, oi, oi.danger)
		ui.Text(c, message).FontSize(12).TextColor(oi.danger).
			SingleLine().Grow(1).Ellipsis("…")
		m.bannerButton(c, p, key+":retry", "Refresh", refresh)
	})
}

func (m *Model) selectMavenBuildRow(row mavenRow) {
	m.maven.activeTool = "maven"
	m.maven.selKey = row.key
	switch row.kind {
	case "phase", "goal", "runConfig":
		m.maven.sel = mavenSelection{
			kind: row.kind, project: row.project, goal: row.goal, config: row.config,
		}
	}
	m.invalidate()
}

func (m *Model) selectGradleBuildRow(row gradleRow) {
	m.maven.activeTool = "gradle"
	m.maven.gradleSelKey = row.key
	switch row.kind {
	case "task", "runConfig":
		m.maven.gradleSel = gradleSelection{
			kind: row.kind, build: row.build, project: row.project,
			task: row.task, config: row.config,
		}
	}
	m.invalidate()
}

func (m *Model) mavenConfigForRow(row mavenRow) (*openidea.RunConfig, bool) {
	switch row.kind {
	case "runConfig":
		if row.config == nil {
			return nil, false
		}
		cfg := *row.config
		return &cfg, true
	case "phase", "goal":
		if row.project == nil || row.goal == "" {
			return nil, false
		}
		cfg, _ := m.mavenGoalRunConfig(row.project, []string{row.goal})
		return &cfg, true
	}
	return nil, false
}

func (m *Model) gradleConfigForRow(row gradleRow) (*openidea.RunConfig, bool) {
	switch row.kind {
	case "runConfig":
		if row.config == nil {
			return nil, false
		}
		cfg := m.withGradleRunOptions(*row.config)
		return &cfg, true
	case "task":
		if row.build == nil || row.project == nil || row.task == nil {
			return nil, false
		}
		cfg := gradleTaskConfig(
			row.build,
			row.project,
			[]string{row.task.Name},
			m.maven.gradleOffline,
			m.maven.gradleSkipTests,
		)
		return &cfg, true
	}
	return nil, false
}

func (m *Model) runBuildToolConfig(cfg *openidea.RunConfig) {
	if cfg == nil {
		return
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		m.status = "Could not start the build configuration: " + err.Error()
		m.invalidate()
		return
	}
	m.startRunWith(raw, cfg.Name)
}

func (m *Model) debugBuildToolConfig(cfg *openidea.RunConfig) {
	if cfg != nil {
		m.DebugRunConfig(cfg)
	}
}

func (m *Model) editBuildToolConfig(cfg *openidea.RunConfig) {
	if cfg == nil {
		return
	}
	for i := range m.run.configs {
		if m.run.configs[i].ID == cfg.ID {
			m.run.sel = i
			m.OpenRunConfigs()
			return
		}
	}
	m.CreateRunConfiguration(*cfg)
}

func (m *Model) createBuildToolConfig(cfg *openidea.RunConfig) {
	if cfg != nil {
		m.CreateRunConfiguration(*cfg)
	}
}

func appendRunnableBuildMenu(
	m *Model,
	menu *ui.Menu,
	cfg *openidea.RunConfig,
	runConfig bool,
) {
	if cfg == nil {
		return
	}
	if menu.Item("Run '" + cfg.Name + "'").Chosen() {
		m.runBuildToolConfig(cfg)
	}
	if menu.Item("Debug '" + cfg.Name + "'").Chosen() {
		m.debugBuildToolConfig(cfg)
	}
	menu.Separator()
	if runConfig {
		if menu.Item("Edit Configuration…").Chosen() {
			m.editBuildToolConfig(cfg)
		}
		return
	}
	if menu.Item("Create Run Configuration…").Chosen() {
		m.createBuildToolConfig(cfg)
	}
}

func (m *Model) mavenRowContextMenu(menu *ui.Menu, row mavenRow) {
	cfg, runnable := m.mavenConfigForRow(row)
	appendRunnableBuildMenu(m, menu, cfg, runnable && row.kind == "runConfig")

	project := row.project
	switch row.kind {
	case "project":
		if project == nil {
			return
		}
		if menu.Item("Unlink Maven Project").Chosen() {
			m.UnlinkMavenProjects(project.Pom)
		}
		if menu.Item("Open pom.xml").Chosen() {
			m.openMavenPOM(project)
		}
		if menu.Item("Reload All Maven Projects").Chosen() {
			m.SyncMaven()
		}
		if menu.Item("Generate Sources and Update Folders").Chosen() {
			m.GenerateMavenSources(project.Pom)
		}
		if menu.Item("Execute Maven Goal…").Chosen() {
			m.OpenBuildToolExecute("maven")
		}
		if menu.Item("Analyze Dependencies…").Chosen() {
			m.OpenBuildDependencyAnalyzer(map[string]any{"pom": project.Pom})
		}
		if menu.Item("Add Dependency…").Chosen() {
			m.OpenAddMavenDependency(project.Pom)
		}
	case "dependencies":
		if project != nil {
			if menu.Item("Analyze Dependencies…").Chosen() {
				m.OpenBuildDependencyAnalyzer(map[string]any{"pom": project.Pom})
			}
			if menu.Item("Add Dependency…").Chosen() {
				m.OpenAddMavenDependency(project.Pom)
			}
		}
	case "dependency":
		if row.dep == nil || project == nil {
			return
		}
		if menu.Item("Quick Documentation").Chosen() {
			m.showMavenDependencyDocumentation(project, *row.dep, row.direct, row.direct == row.dep)
		}
		if row.direct == row.dep && menu.Item("Jump to Source").Chosen() {
			m.JumpToMavenDependency(project.Pom, *row.dep)
		}
		if row.direct != nil && row.direct != row.dep && menu.Item("Exclude Dependency").Chosen() {
			m.ExcludeMavenDependency(project.Pom, *row.direct, *row.dep)
		}
		if menu.Item("Analyze Dependencies…").Chosen() {
			m.OpenBuildDependencyAnalyzer(map[string]any{"pom": project.Pom})
		}
		if menu.Item("Add Dependency…").Chosen() {
			m.OpenAddMavenDependency(project.Pom)
		}
		if menu.Item("Copy Coordinates").Chosen() {
			buildToolClipboardWrite(mavenDependencyCoordinates(*row.dep))
		}
	}
}

func (m *Model) gradleRowContextMenu(menu *ui.Menu, row gradleRow) {
	cfg, runnable := m.gradleConfigForRow(row)
	appendRunnableBuildMenu(m, menu, cfg, runnable && row.kind == "runConfig")

	if row.kind != "gradleProject" || row.build == nil || row.project == nil {
		return
	}
	if row.project.BuildFile != nil && *row.project.BuildFile != "" {
		if menu.Item("Open " + filepath.Base(*row.project.BuildFile)).Chosen() {
			m.openGradleBuildFile(row.build, row.project)
		}
	}
	if row.project.Path == ":" && menu.Item("Unlink Gradle Project").Chosen() {
		m.UnlinkGradleProject(row.build.Dir)
	}
	if menu.Item("Reload Gradle Project").Chosen() {
		m.SyncGradleBuild(row.build.Dir)
	}
	if menu.Item("Execute Gradle Task…").Chosen() {
		m.maven.gradleSel = gradleSelection{
			kind: "gradleProject", build: row.build, project: row.project,
		}
		m.OpenBuildToolExecute("gradle")
	}
	if menu.Item("Analyze Dependencies…").Chosen() {
		m.OpenBuildDependencyAnalyzer(map[string]any{
			"buildDir": row.build.Dir, "gradlePath": row.project.Path,
		})
	}
}

func (m *Model) openMavenPOM(project *mavenProject) {
	if path := m.mavenPOMPath(project); path != "" {
		m.openFile(path)
	}
}

func (m *Model) openGradleBuildFile(build *gradleModel, project *gradleProject) {
	if path := m.gradleBuildFilePath(build, project); path != "" {
		m.openFile(path)
	}
}

func (m *Model) mavenPOMPath(project *mavenProject) string {
	if project == nil || strings.TrimSpace(project.Pom) == "" {
		return ""
	}
	if filepath.IsAbs(project.Pom) {
		return filepath.Clean(project.Pom)
	}
	base := m.Root()
	if project.ProjectRoot != nil && strings.TrimSpace(*project.ProjectRoot) != "" {
		projectRoot := strings.TrimSpace(*project.ProjectRoot)
		if filepath.IsAbs(projectRoot) {
			base = projectRoot
		} else {
			base = filepath.Join(base, projectRoot)
		}
	}
	return filepath.Clean(filepath.Join(base, project.Pom))
}

func (m *Model) mavenConfigBelongsTo(config *openidea.RunConfig, project mavenProject) bool {
	if config == nil {
		return false
	}
	cwd := ""
	if config.Cwd != nil {
		cwd = strings.TrimSpace(*config.Cwd)
	}
	// The backend already emits project-relative directories. This direct
	// comparison is also what keeps linked projects whose projectRoot differs
	// from the opened workspace attached to the correct Maven module.
	if jsonPathClean(filepath.ToSlash(cwd)) == jsonPathClean(filepath.ToSlash(strings.TrimSpace(project.Dir))) {
		return true
	}
	base := m.Root()
	if project.ProjectRoot != nil && strings.TrimSpace(*project.ProjectRoot) != "" {
		projectRoot := strings.TrimSpace(*project.ProjectRoot)
		if filepath.IsAbs(projectRoot) {
			base = projectRoot
		} else {
			base = filepath.Join(base, projectRoot)
		}
	}
	projectDir := strings.TrimSpace(project.Dir)
	if !filepath.IsAbs(projectDir) {
		projectDir = filepath.Join(base, projectDir)
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(m.Root(), cwd)
	}
	return filepath.Clean(projectDir) == filepath.Clean(cwd)
}

func (m *Model) gradleBuildFilePath(build *gradleModel, project *gradleProject) string {
	if project == nil || project.BuildFile == nil || strings.TrimSpace(*project.BuildFile) == "" {
		return ""
	}
	path := strings.TrimSpace(*project.BuildFile)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	root := m.Root()
	if strings.Contains(filepath.ToSlash(path), "/") {
		// OpenIDEA emits project build files relative to the workspace root.
		return filepath.Clean(filepath.Join(root, path))
	}
	if build != nil && strings.TrimSpace(build.Dir) != "" {
		root = filepath.Join(root, build.Dir)
	} else if strings.TrimSpace(project.Dir) != "" {
		root = filepath.Join(root, project.Dir)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func (m *Model) CollapseGradleAll() {
	for key := range m.maven.expanded {
		if strings.HasPrefix(key, "gradle:") {
			delete(m.maven.expanded, key)
		}
	}
	m.invalidate()
}

func mavenDependencyCoordinates(dep mavenDependency) string {
	parts := []string{dep.GroupID, dep.ArtifactID}
	if dep.Classifier != nil && *dep.Classifier != "" {
		parts = append(parts, *dep.Classifier)
	}
	if dep.Version != "" {
		parts = append(parts, dep.Version)
	}
	return strings.Join(parts, ":")
}
