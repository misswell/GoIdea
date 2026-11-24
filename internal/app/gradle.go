package app

// The Gradle tool window mirrors OpenIDEA's BuildToolView for Gradle: one root
// project per independently linked build, grouped tasks, subprojects, the
// matching saved run configurations, and the same toolbar and sync banners as
// the Maven window. All build data still comes from openidea-server.

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type gradleTask struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

type gradleDependency struct {
	Group      string             `json:"group"`
	Name       string             `json:"name"`
	Requested  *string            `json:"requested"`
	Selected   *string            `json:"selected"`
	Project    bool               `json:"project"`
	Repeated   bool               `json:"repeated"`
	Constraint bool               `json:"constraint"`
	Unresolved bool               `json:"unresolved"`
	Children   []gradleDependency `json:"children"`
}

type gradleProject struct {
	Path      string       `json:"path"`
	Name      string       `json:"name"`
	Dir       string       `json:"dir"`
	BuildFile *string      `json:"buildFile"`
	Tasks     []gradleTask `json:"tasks"`
	Build     *string      `json:"build"`
}

type gradleModel struct {
	Dir         string            `json:"dir"`
	Projects    []gradleProject   `json:"projects"`
	JavaVersion *uint32           `json:"javaVersion"`
	Command     string            `json:"command"`
	Env         map[string]string `json:"env"`
	SyncedAt    *float64          `json:"syncedAt"`
	Stale       bool              `json:"stale"`
	SyncError   *string           `json:"syncError"`
}

func (g gradleModel) syncErrorText() string {
	if g.SyncError == nil {
		return ""
	}
	return strings.TrimSpace(*g.SyncError)
}

type gradleSelection struct {
	kind    string
	build   *gradleModel
	project *gradleProject
	task    *gradleTask
	config  *openidea.RunConfig
}

type gradleRow struct {
	key     string
	depth   int
	kind    string
	label   string
	detail  string
	build   *gradleModel
	project *gradleProject
	task    *gradleTask
	config  *openidea.RunConfig
	hasKids bool
}

// ShowGradle opens the Gradle window and loads the build model.
func (m *Model) ShowGradle() {
	m.maven.gradleVisible = true
	m.maven.activeTool = "gradle"
	m.DockShow("gradle")
	m.loadBuildModel()
}

// CloseGradle hides the window but keeps its model, expansion and selection.
func (m *Model) CloseGradle() {
	m.maven.gradleVisible = false
	m.DockHide("gradle")
}

// ToggleGradle is the View/Window action.
func (m *Model) ToggleGradle() {
	if m.dockWindowShown("gradle") {
		m.CloseGradle()
		return
	}
	m.ShowGradle()
}

func (m *Model) gradleRootRows() []gradleRow {
	var rows []gradleRow
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		project := gradleRootProject(build)
		if project == nil {
			continue
		}
		rows = append(rows, m.gradleProjectRow(build, project, 0))
	}
	return rows
}

func gradleRootProject(build *gradleModel) *gradleProject {
	if build == nil {
		return nil
	}
	for i := range build.Projects {
		if build.Projects[i].Path == ":" {
			return &build.Projects[i]
		}
	}
	if len(build.Projects) > 0 {
		return &build.Projects[0]
	}
	return nil
}

func (m *Model) gradleProjectRow(build *gradleModel, project *gradleProject, depth int) gradleRow {
	label := strings.TrimSpace(project.Name)
	if label == "" {
		label = strings.TrimSpace(filepath.Base(project.Dir))
	}
	if label == "" || label == "." {
		label = "Gradle"
	}
	detail := ""
	if build != nil && build.Dir != "" && build.Dir != "." {
		detail = build.Dir
	}
	return gradleRow{
		key:     gradleProjectKey(build, project),
		depth:   depth,
		kind:    "gradleProject",
		label:   label,
		detail:  detail,
		build:   build,
		project: project,
		hasKids: true,
	}
}

func (m *Model) gradleChildRows(row gradleRow) []gradleRow {
	switch row.kind {
	case "gradleProject":
		if row.build == nil || row.project == nil {
			return nil
		}
		out := []gradleRow{{
			key: row.key + ":tasks", depth: row.depth + 1, kind: "tasks",
			label: "Tasks", build: row.build, project: row.project, hasKids: true,
		}}
		if row.project.Path == ":" {
			configs := m.gradleConfigsOf(row.build)
			if len(configs) > 0 {
				out = append(out, gradleRow{
					key: row.key + ":runs", depth: row.depth + 1, kind: "runConfigs",
					label: "Run Configurations", build: row.build, project: row.project, hasKids: true,
				})
			}
		}
		for _, project := range gradleSubprojects(row.build, row.project.Path) {
			out = append(out, m.gradleProjectRow(row.build, project, row.depth+1))
		}
		return out
	case "tasks":
		if row.build == nil || row.project == nil {
			return nil
		}
		groups := gradleTaskGroups(*row.project)
		out := make([]gradleRow, 0, len(groups))
		for _, group := range groups {
			out = append(out, gradleRow{
				key: row.key + ":" + group.name, depth: row.depth + 1, kind: "taskGroup",
				label: group.name, build: row.build, project: row.project, hasKids: len(group.tasks) > 0,
			})
		}
		return out
	case "taskGroup":
		if row.project == nil {
			return nil
		}
		for _, group := range gradleTaskGroups(*row.project) {
			if group.name != row.label {
				continue
			}
			out := make([]gradleRow, 0, len(group.tasks))
			for _, task := range group.tasks {
				out = append(out, gradleRow{
					key: row.key + ":" + task.Name, depth: row.depth + 1, kind: "task",
					label: task.Name, detail: task.Description, build: row.build,
					project: row.project, task: &task,
				})
			}
			return out
		}
	case "runConfigs":
		if row.build == nil {
			return nil
		}
		configs := m.gradleConfigsOf(row.build)
		out := make([]gradleRow, 0, len(configs))
		for _, config := range configs {
			out = append(out, gradleRow{
				key: row.key + ":" + config.ID, depth: row.depth + 1, kind: "runConfig",
				label: config.Name, build: row.build, config: config,
			})
		}
		return out
	}
	return nil
}

type gradleTaskGroup struct {
	name  string
	tasks []gradleTask
}

func gradleTaskGroups(project gradleProject) []gradleTaskGroup {
	groups := map[string][]gradleTask{}
	for _, task := range project.Tasks {
		name := strings.TrimSpace(task.Group)
		if name == "" {
			name = "other"
		}
		groups[name] = append(groups[name], task)
	}
	out := make([]gradleTaskGroup, 0, len(groups))
	for name, tasks := range groups {
		sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })
		out = append(out, gradleTaskGroup{name: name, tasks: tasks})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name == "other" {
			return false
		}
		if out[j].name == "other" {
			return true
		}
		return out[i].name < out[j].name
	})
	return out
}

func gradleSubprojects(build *gradleModel, path string) []*gradleProject {
	if build == nil {
		return nil
	}
	var out []*gradleProject
	for i := range build.Projects {
		project := &build.Projects[i]
		if project.Path == path || project.Path == ":" {
			continue
		}
		parent := ":"
		if at := strings.LastIndex(project.Path, ":"); at > 0 {
			parent = project.Path[:at]
		}
		if parent == path {
			out = append(out, project)
			continue
		}
		if path == ":" {
			foundParent := false
			for j := range build.Projects {
				if build.Projects[j].Path == parent {
					foundParent = true
					break
				}
			}
			if !foundParent {
				out = append(out, project)
			}
		}
	}
	return out
}

func gradleProjectKey(build *gradleModel, project *gradleProject) string {
	if build == nil || project == nil {
		return "gradle:"
	}
	return "gradle:" + build.Dir + ":" + project.Path
}

func (m *Model) gradleConfigsOf(build *gradleModel) []*openidea.RunConfig {
	var out []*openidea.RunConfig
	if build == nil {
		return out
	}
	for i := range m.run.configs {
		config := &m.run.configs[i]
		if config.Kind != "gradle" {
			continue
		}
		owner := m.gradleConfigOwner(config)
		if owner != nil && owner.Dir == build.Dir {
			out = append(out, config)
		}
	}
	return out
}

func (m *Model) gradleConfigOwner(config *openidea.RunConfig) *gradleModel {
	if config == nil {
		return nil
	}
	cwd := ""
	if config.Cwd != nil {
		cwd = *config.Cwd
	}
	cwd = gradleAbsolutePath(m.Root(), cwd)
	var owner *gradleModel
	ownerDepth := -1
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		candidates := []string{build.Dir}
		for _, project := range build.Projects {
			candidates = append(candidates, project.Dir)
		}
		for _, candidate := range candidates {
			dir := gradleAbsolutePath(m.Root(), candidate)
			if !gradlePathContains(dir, cwd) {
				continue
			}
			depth := len(strings.Split(strings.Trim(dir, "/"), "/"))
			if depth > ownerDepth {
				owner = build
				ownerDepth = depth
			}
		}
	}
	return owner
}

func gradleAbsolutePath(root, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	if !filepath.IsAbs(path) && root != "" {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	return strings.TrimSuffix(filepath.ToSlash(path), "/")
}

func gradlePathContains(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, "/")+"/")
}

func (m *Model) gradlePanel(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).Children(func() {
		m.gradleToolbar(c, p)
		m.gradleBanner(c, p)
		if m.maven.err != "" {
			m.buildToolErrorBanner(c, p, "gradle:modelError", m.maven.err, func() {
				m.loadBuildModel()
			})
		}
		if !m.maven.loading && len(m.maven.model.Gradle) == 0 {
			ui.Text(c, "No Gradle projects").FontSize(12).TextColor(p.TextMuted).Padding(12, 10)
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(0, 6).Children(func() {
				m.gradleTreeRows(c, p, m.gradleRootRows())
			})
		})
	})
}

func (m *Model) gradleToolbar(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(32).Padding(0, 6).Gap(2).AlignItems(ui.Center).Children(func() {
		if m.maven.syncing {
			m.mavenAction(c, p, "gradle:reload", "run/stop", "Stop", func() { m.CancelBuildToolReload("gradle") })
		} else if m.maven.loading {
			m.mavenActionDisabled(c, p, "gradle:reload", "general/refresh", "Loading Gradle Projects", false)
		} else {
			m.mavenAction(c, p, "gradle:reload", "general/refresh", "Reload All Gradle Projects", func() { m.SyncGradle() })
		}
		m.mavenAction(c, p, "gradle:link", "general/add", "Link Gradle Projects", func() { m.LinkGradleProjects() })
		m.mavenSeparator(c, p)
		runnable := m.maven.gradleSel.kind == "task" || m.maven.gradleSel.kind == "runConfig"
		if runnable {
			m.mavenAction(c, p, "gradle:run", "run/run", "Run Gradle Task", func() { m.RunGradleSelection() })
		} else {
			m.mavenActionDisabled(c, p, "gradle:run", "run/run", "Run Gradle Task", false)
		}
		m.mavenAction(c, p, "gradle:task", "general/runAnything", "Execute Gradle Task", func() {
			m.OpenBuildToolExecute("gradle")
		})
		m.mavenToggle(c, p, "gradle:offline", "build/toggleOfflineMode", "Work Offline", m.maven.gradleOffline,
			func() { m.ToggleGradleOffline() })
		m.mavenAction(c, p, "gradle:analyze", "build/dependencyAnalyzer", "Analyze Dependencies", func() {
			m.AnalyzeBuildDependencies()
		})
		m.mavenToggle(c, p, "gradle:skipTests", "run/showIgnored", "Skip Tests", m.maven.gradleSkipTests,
			func() { m.ToggleGradleSkipTests() })
		m.mavenSeparator(c, p)
		m.mavenAction(c, p, "gradle:expand", "general/expandAll", "Expand All", func() { m.ExpandGradleAll() })
		m.mavenAction(c, p, "gradle:collapse", "general/collapseAll", "Collapse All", func() { m.CollapseGradleAll() })
	})
}

func (m *Model) gradleBanner(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	if m.maven.syncing {
		ui.Row(c).MinHeight(28).Padding(2, 10).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, m.maven.syncMsg).FontSize(12).TextColor(oi.textSecondary).SingleLine().Grow(1).Ellipsis("…")
			if fraction := m.maven.syncFrac; fraction != nil {
				ui.Text(c, fmt.Sprintf("%d%%", int(*fraction*100))).FontSize(12).TextColor(oi.textSubtle)
			}
		})
		return
	}
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		switch {
		case build.Stale:
			ui.Row(c).MinHeight(28).Padding(2, 10).Gap(6).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
				oiIconEl(c, "build/gradleLoadChanges", 16, oi, oi.textMuted)
				ui.Text(c, "Build files changed since the last reload").FontSize(12).TextColor(oi.textSecondary).
					SingleLine().Grow(1).Ellipsis("…")
				m.bannerButton(c, p, "gradle:load:"+build.Dir, "Load Gradle Changes", func() { m.SyncGradle() })
			})
		case build.syncErrorText() != "":
			ui.Row(c).MinHeight(28).Padding(2, 10).Gap(6).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
				oiIconEl(c, "status/warning", 16, oi, oi.warning)
				ui.Text(c, strings.SplitN(build.syncErrorText(), "\n", 2)[0]).FontSize(12).TextColor(oi.warning).
					SingleLine().Grow(1).Ellipsis("…")
				dir := build.Dir
				m.bannerButton(c, p, "gradle:retry:"+build.Dir, "Refresh", func() { m.SyncGradleBuild(dir) })
			})
		}
	}
}

func (m *Model) gradleTreeRows(c *ui.Context, p *ui.Theme, rows []gradleRow) {
	for _, row := range rows {
		row := row
		m.gradleRow(c, p, row)
		if row.hasKids && m.maven.expanded[row.key] {
			m.gradleTreeRows(c, p, m.gradleChildRows(row))
		}
	}
}

func (m *Model) gradleRow(c *ui.Context, p *ui.Theme, row gradleRow) {
	oi := oiPaletteOf(p)
	element := ui.Row(c.Key(row.key)).
		Height(24).PaddingStart(4 + float32(row.depth)*m.treeIndentWidth()).PaddingEnd(8).
		Gap(4).Radius(5).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if m.maven.gradleSelKey == row.key {
		element.Background(oi.selectedInactive)
	} else if element.Hovered() {
		element.Background(oi.hoverTint)
	}
	element.Children(func() {
		chevron := ui.Row(c).Size(16, 16).Shrink(0).Justify(ui.Center).AlignItems(ui.Center)
		if row.hasKids {
			glyph := "general/chevronRight"
			if m.maven.expanded[row.key] {
				glyph = "general/chevronDown"
			}
			chevron.Children(func() { oiIconEl(c, glyph, 16, oi, oi.textMuted) })
		}
		if icon := gradleRowIcon(row.kind); icon != "" {
			oiIconEl(c, icon, 16, oi, oi.textMuted).MarginEnd(2)
		}
		tone := oi.text
		if m.maven.gradleSelKey == row.key {
			tone = oi.textStrong
		}
		label := ui.Text(c, row.label).FontSize(13).TextColor(tone).SingleLine().Ellipsis("…")
		if row.kind == "task" && row.task != nil && row.task.Name == "test" && m.maven.gradleSkipTests {
			label.TextColor(oi.textMuted).Strikethrough()
		}
		if row.detail != "" {
			ui.Text(c, row.detail).FontSize(12).TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
	})
	if row.hasKids {
		element.OnClick(func() {
			if m.maven.expanded == nil {
				m.maven.expanded = map[string]bool{}
			}
			m.maven.expanded[row.key] = !m.maven.expanded[row.key]
			m.invalidate()
		})
	}
	if row.kind == "task" || row.kind == "runConfig" {
		element.OnClick(func() {
			m.maven.gradleSel = gradleSelection{
				kind: row.kind, build: row.build, project: row.project,
				task: row.task, config: row.config,
			}
			m.maven.gradleSelKey = row.key
			m.maven.activeTool = "gradle"
			m.invalidate()
		})
		if element.DoubleClicked() {
			m.maven.gradleSel = gradleSelection{
				kind: row.kind, build: row.build, project: row.project,
				task: row.task, config: row.config,
			}
			m.maven.gradleSelKey = row.key
			m.maven.activeTool = "gradle"
			m.RunGradleSelection()
		}
	}
	if row.kind == "gradleProject" || row.kind == "task" || row.kind == "runConfig" {
		element.ContextMenu(func(menu *ui.Menu) {
			m.selectGradleBuildRow(row)
			m.gradleRowContextMenu(menu, row)
		})
	}
}

func gradleRowIcon(kind string) string {
	switch kind {
	case "gradleProject":
		return "fileTypes/gradle"
	case "tasks", "runConfigs":
		return "nodes/folder"
	case "taskGroup":
		return "build/taskGroup"
	case "task", "runConfig":
		return "externalSystem/task"
	}
	return ""
}

func (m *Model) ExpandGradleAll() {
	if m.maven.expanded == nil {
		m.maven.expanded = map[string]bool{}
	}
	var walk func([]gradleRow)
	walk = func(rows []gradleRow) {
		for _, row := range rows {
			if row.hasKids {
				m.maven.expanded[row.key] = true
				walk(m.gradleChildRows(row))
			}
		}
	}
	walk(m.gradleRootRows())
	m.invalidate()
}

func (m *Model) RunGradleSelection() {
	selection := m.maven.gradleSel
	switch selection.kind {
	case "runConfig":
		if selection.config == nil {
			return
		}
		config := m.withGradleRunOptions(*selection.config)
		raw, err := json.Marshal(config)
		if err != nil {
			m.status = "Could not start the Gradle task: " + err.Error()
			m.invalidate()
			return
		}
		m.startGradleConfig(raw, config.Name)
	case "task":
		if selection.build == nil || selection.project == nil || selection.task == nil {
			return
		}
		config := gradleTaskConfig(
			selection.build,
			selection.project,
			[]string{selection.task.Name},
			m.maven.gradleOffline,
			m.maven.gradleSkipTests,
		)
		raw, err := json.Marshal(config)
		if err != nil {
			m.status = "Could not start the Gradle task: " + err.Error()
			m.invalidate()
			return
		}
		m.startGradleConfig(raw, config.Name)
	}
}

func (m *Model) startGradleConfig(raw json.RawMessage, name string) {
	if !m.run.trusted {
		m.run.pendingTrust = true
		m.trustAsk = true
		m.invalidate()
		return
	}
	if m.run.busy || m.run.running {
		m.status = "A run is already in progress"
		m.invalidate()
		return
	}
	if !m.startRunWith(raw, name) {
		m.status = "The Gradle task was not started"
		m.invalidate()
	}
}

func gradleTaskConfig(
	build *gradleModel,
	project *gradleProject,
	tasks []string,
	offline bool,
	skipTests bool,
) openidea.RunConfig {
	args := make([]string, 0, len(tasks)+3)
	var names []string
	for _, task := range tasks {
		task = strings.TrimSpace(task)
		if task == "" {
			continue
		}
		original := task
		if !strings.HasPrefix(task, "-") && !strings.HasPrefix(task, ":") {
			path := project.Path
			if path != "" && path != ":" {
				task = path + ":" + task
			}
		}
		args = append(args, task)
		if !strings.HasPrefix(original, "-") {
			names = append(names, original)
		}
	}
	if offline && !containsString(args, "--offline") {
		args = append(args, "--offline")
	}
	if skipTests && gradleHasTestTask([]gradleModel{*build}) && !gradleExcludesTest(tasks) {
		args = append(args, "-x", "test")
	}
	command := strings.TrimSpace(build.Command)
	if command == "" {
		command = "gradle"
	}
	name := strings.TrimSpace(project.Name)
	if name == "" {
		name = strings.TrimSpace(filepath.Base(build.Dir))
	}
	if name == "" || name == "." {
		name = "Gradle"
	}
	name += " [" + strings.Join(names, ",") + "]"
	var cwd *string
	if build.Dir != "" {
		dir := build.Dir
		cwd = &dir
	}
	isTest := !skipTests
	if isTest {
		isTest = false
		for _, task := range names {
			short := task
			if at := strings.LastIndex(task, ":"); at >= 0 {
				short = task[at+1:]
			}
			switch short {
			case "test", "check", "build", "integrationTest":
				isTest = true
			}
		}
	}
	return openidea.RunConfig{
		ID:           "gradle:task:" + build.Dir + ":" + project.Path + ":" + strings.Join(tasks, " "),
		Name:         name,
		Kind:         "gradle",
		Command:      command,
		Args:         args,
		Cwd:          cwd,
		Env:          map[string]string{},
		Detected:     false,
		IsTest:       isTest,
		DebugAdapter: nil,
		EnvFiles:     []string{},
	}
}

func (m *Model) withGradleRunOptions(config openidea.RunConfig) openidea.RunConfig {
	args := append([]string(nil), config.Args...)
	if m.maven.gradleOffline && !containsString(args, "--offline") {
		args = append(args, "--offline")
	}
	if m.maven.gradleSkipTests && gradleHasTestTask(m.maven.model.Gradle) && !gradleExcludesTest(args) {
		args = append(args, "-x", "test")
	}
	config.Args = args
	return config
}

func gradleHasTestTask(models []gradleModel) bool {
	for _, build := range models {
		for _, project := range build.Projects {
			for _, task := range project.Tasks {
				if task.Name == "test" {
					return true
				}
			}
		}
	}
	return false
}

func gradleExcludesTest(args []string) bool {
	for i, arg := range args {
		switch {
		case arg == "-x" || arg == "--exclude-task":
			if i+1 < len(args) && args[i+1] == "test" {
				return true
			}
		case arg == "--exclude-task=test", arg == "-xtest":
			return true
		}
	}
	return false
}
