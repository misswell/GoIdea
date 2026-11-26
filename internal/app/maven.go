package app

// The Maven/Gradle tool window after OpenIDEA's BuildToolView: the toolbar,
// the "Build files changed" banner with Load Maven Changes, and the project
// tree — Profiles, every non-aggregated project with Lifecycle, Plugins,
// Dependencies, Run Configurations and its modules. The model, the sync and
// the profiles come from the backend; nothing is rescanned here.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// standardLifecycle is Maven's default lifecycle in the order IDEA lists it.
var mvnw = regexp.MustCompile(`(?i)(?:^|[/\\])mvnw(?:\.cmd|\.bat)?$`)

var standardLifecycle = []string{
	"clean", "validate", "compile", "test", "package", "verify", "install", "site", "deploy",
}

type mavenDependency struct {
	GroupID    string            `json:"groupId"`
	ArtifactID string            `json:"artifactId"`
	Version    string            `json:"version"`
	Scope      string            `json:"scope"`
	Type       string            `json:"type"`
	Classifier *string           `json:"classifier"`
	Optional   bool              `json:"optional"`
	Omitted    *string           `json:"omitted"`
	Conflict   *string           `json:"conflictWith"`
	Managed    *string           `json:"managedFrom"`
	Children   []mavenDependency `json:"children"`

	// Gradle dependencies use these analyzer-only fields. They are filled from
	// gradle_dependencies and are not part of build_model's Maven JSON.
	Requested   *string `json:"-"`
	Unresolved  bool    `json:"-"`
	GradleBuild bool    `json:"-"`
}

type mavenPlugin struct {
	GroupID    string   `json:"groupId"`
	ArtifactID string   `json:"artifactId"`
	Version    *string  `json:"version"`
	Prefix     string   `json:"prefix"`
	Goals      []string `json:"goals"`
}

type mavenProject struct {
	ProjectRoot  *string           `json:"projectRoot"`
	Pom          string            `json:"pom"`
	Dir          string            `json:"dir"`
	GroupID      string            `json:"groupId"`
	ArtifactID   string            `json:"artifactId"`
	Version      string            `json:"version"`
	Packaging    string            `json:"packaging"`
	DisplayName  string            `json:"displayName"`
	Parent       *string           `json:"parent"`
	Modules      []string          `json:"modules"`
	Aggregated   bool              `json:"aggregated"`
	Plugins      []mavenPlugin     `json:"plugins"`
	Profiles     []string          `json:"profiles"`
	JavaVersion  *uint32           `json:"javaVersion"`
	Command      string            `json:"command"`
	Options      []string          `json:"options"`
	Env          map[string]string `json:"env"`
	Dependencies []mavenDependency `json:"dependencies"`
}

type mavenProfile struct {
	ProjectRoot     *string `json:"projectRoot"`
	ID              string  `json:"id"`
	ActiveByDefault bool    `json:"activeByDefault"`
}

type mavenModel struct {
	Projects []mavenProject `json:"projects"`
	Profiles []mavenProfile `json:"profiles"`
	SyncedAt *float64       `json:"syncedAt"`
	Stale    bool           `json:"stale"`
	SyncErr  *string        `json:"syncError"`
}

type buildModel struct {
	Maven  *mavenModel   `json:"maven"`
	Gradle []gradleModel `json:"gradle"`
}

// mavenSelection is the tree row the toolbar's Run button acts on.
type mavenSelection struct {
	kind    string // "phase", "goal", "runConfig"
	project *mavenProject
	goal    string
	config  *openidea.RunConfig
}

// mavenPane is the tool window's state.
type mavenPane struct {
	visible bool
	// gradleVisible is the peer Gradle tool window. Both share the build model
	// and sync progress, but keep independent selection and tree expansion.
	gradleVisible bool
	activeTool    string
	loading       bool

	// syncing is Load Maven Changes in flight: its message and fraction.
	syncing  bool
	syncTool string
	syncMsg  string
	syncFrac *float32
	request  uint32
	channel  uint32

	// profiles are the user's choices by profile key; a choice that differs
	// from the POM's activeByDefault is sent to the sync.
	profiles map[string]bool
	// offline and skipTests are the toolbar's two toggles.
	offline   bool
	skipTests bool
	// Gradle has independent upstream settings. It must not inherit Maven's
	// offline or skip-tests switches.
	gradleOffline   bool
	gradleSkipTests bool
	// settingsRoot is the project whose build-tool settings are loaded.
	// OpenIDEA stores these switches per project, independently from the
	// backend's Java settings.
	settingsRoot string

	err          string
	model        buildModel
	expanded     map[string]bool
	expandedInit bool
	sel          mavenSelection
	selKey       string
	gradleSel    gradleSelection
	gradleSelKey string
	currentPOM   string
	gen          uint64

	analyzer      *buildDependencyAnalyzer
	addDependency *addMavenDependencyDialog
}

// ShowMaven opens the Maven window and loads the build model.
func (m *Model) ShowMaven() {
	m.maven.visible = true
	if m.maven.gradleVisible {
		m.maven.gradleVisible = false
	}
	m.maven.activeTool = "maven"
	m.DockShow("maven")
	m.loadBuildModel()
}

// CloseMaven hides it without side effects.
func (m *Model) CloseMaven() { m.maven.visible = false; m.DockHide("maven") }

// ToggleMaven is the Window menu and palette action.
func (m *Model) ToggleMaven() { m.maven.visible = !m.maven.visible; m.DockToggle("maven") }

// loadBuildModel asks the backend for the reactor and Gradle builds.
func (m *Model) loadBuildModel() {
	root := m.Root()
	m.loadBuildToolSettings(root)
	if m.maven.loading || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) || root == "" {
		return
	}
	m.maven.loading = true
	m.maven.err = ""
	m.maven.gen++
	gen := m.maven.gen
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "build_model", nil)
		var model buildModel
		if err == nil {
			err = json.Unmarshal(raw, &model)
		}
		m.post(func() {
			if m.maven.gen != gen {
				return
			}
			m.maven.loading = false
			if m.Root() != root {
				m.maven.model = buildModel{}
				m.maven.err = ""
				m.invalidate()
				return
			}
			if err != nil {
				m.maven.err = "Could not read the build model: " + err.Error()
			} else {
				m.applyBuildToolModel(model)
				if m.maven.visible || m.maven.gradleVisible {
					m.syncInitialBuildModels()
				}
			}
			m.invalidate()
		})
	}()
}

// syncInitialBuildModels mirrors BuildToolView's first model load: a build
// with no syncedAt is imported once through the real build_sync RPC. The
// visible tool goes first; the other one follows after that sync returns.
func (m *Model) syncInitialBuildModels() {
	if m.maven.syncing || m.maven.loading || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	mavenUnsynced := m.maven.model.Maven != nil && m.maven.model.Maven.SyncedAt == nil
	var gradleUnsynced []string
	for _, build := range m.maven.model.Gradle {
		if build.SyncedAt == nil {
			gradleUnsynced = append(gradleUnsynced, build.Dir)
		}
	}
	if m.maven.activeTool == "gradle" && len(gradleUnsynced) > 0 {
		m.SyncGradleBuilds(gradleUnsynced)
		return
	}
	if mavenUnsynced {
		m.SyncMaven()
		return
	}
	if len(gradleUnsynced) > 0 {
		m.SyncGradleBuilds(gradleUnsynced)
	}
}

// mavenInitTree expands the top-level projects once per model, as OpenIDEA's
// initialExpanded does, and keeps the profile choices the POMs declare.
func (m *Model) mavenInitTree() {
	if m.maven.expanded == nil {
		m.maven.expanded = map[string]bool{}
	}
	if m.maven.profiles == nil {
		m.maven.profiles = map[string]bool{}
	}
	if !m.maven.expandedInit {
		for _, p := range m.maven.model.MavenProjects() {
			if !p.Aggregated {
				m.maven.expanded[mavenProjectKey(p)] = true
			}
		}
		m.maven.expandedInit = true
	}
	if m.maven.model.Maven != nil {
		for _, prof := range m.maven.model.Maven.Profiles {
			key := mavenProfileKey(prof)
			if _, chosen := m.maven.profiles[key]; !chosen {
				m.maven.profiles[key] = prof.ActiveByDefault
			}
		}
	}
	if selected := m.maven.sel.project; selected != nil {
		if project := m.mavenProjectByPom(selected.Pom); project != nil {
			m.maven.sel.project = project
		} else {
			m.maven.sel = mavenSelection{}
			m.maven.selKey = ""
		}
	}
	m.normalizeGradleSelection()
	m.refreshBuildToolRunConfigs()
}

// mavenProfilesList is the sync's -P list: the choices that differ from the
// defaults, deactivated ones prefixed with "!".
func (m *Model) mavenProfilesList() []string {
	out := []string{}
	if m.maven.model.Maven == nil {
		return out
	}
	for _, prof := range m.maven.model.Maven.Profiles {
		on := m.mavenProfileChecked(prof)
		switch {
		case on && !prof.ActiveByDefault:
			out = append(out, prof.ID)
		case !on && prof.ActiveByDefault:
			out = append(out, "!"+prof.ID)
		}
	}
	return out
}

// mavenProfilesListForProject returns the choices for one project root, as
// mavenGoalConfig filters model.profiles before creating a run configuration.
func (m *Model) mavenProfilesListForProject(project *mavenProject) []string {
	out := []string{}
	if m.maven.model.Maven == nil || project == nil {
		return out
	}
	for _, prof := range m.maven.model.Maven.Profiles {
		if !sameOptionalPath(prof.ProjectRoot, project.ProjectRoot) {
			continue
		}
		on := m.mavenProfileChecked(prof)
		switch {
		case on && !prof.ActiveByDefault:
			out = append(out, prof.ID)
		case !on && prof.ActiveByDefault:
			out = append(out, "!"+prof.ID)
		}
	}
	return out
}

func sameOptionalPath(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return filepath.Clean(*a) == filepath.Clean(*b)
}

func (m *Model) mavenProfileChecked(p mavenProfile) bool {
	if v, ok := m.maven.profiles[mavenProfileKey(p)]; ok {
		return v
	}
	return p.ActiveByDefault
}

// SetMavenProfile is a profile checkbox: the choice syncs the project again,
// since the language server resolves it with the profiles of the last sync.
func (m *Model) SetMavenProfile(p mavenProfile, on bool) {
	m.loadBuildToolSettings(m.Root())
	if m.maven.profiles == nil {
		m.maven.profiles = map[string]bool{}
	}
	m.maven.profiles[mavenProfileKey(p)] = on
	m.saveBuildToolSettings()
	m.SyncMaven()
}

// SyncMaven is Load Maven Changes: build_sync with the current offline and
// profile choices, streaming its progress into the banner.
func (m *Model) SyncMaven() {
	if m.maven.syncing || (m.mgr == nil && m.invokeHook == nil) || m.ctx == nil || m.Root() == "" {
		return
	}
	m.maven.gen++
	req := m.nextRequest()
	ch := m.nextChannel()
	m.maven.syncing = true
	m.maven.syncTool = "maven"
	m.maven.syncMsg = "Syncing Maven projects…"
	m.maven.syncFrac = nil
	m.maven.request = req
	m.maven.channel = ch
	m.maven.err = ""
	m.invalidate()
	args := map[string]any{
		"tool":       "maven",
		"buildDir":   nil,
		"offline":    m.maven.offline,
		"profiles":   m.mavenProfilesList(),
		"request":    req,
		"onProgress": map[string]any{"$channel": ch},
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, "build_sync", args)
		var model buildModel
		if err == nil {
			err = json.Unmarshal(raw, &model)
		}
		m.post(func() {
			if m.maven.channel != ch {
				return
			}
			m.maven.syncing = false
			m.maven.syncTool = ""
			m.maven.syncMsg = ""
			m.maven.syncFrac = nil
			if err != nil {
				m.maven.err = "Could not load the Maven changes: " + err.Error()
				m.invalidate()
				return
			}
			m.maven.loading = false
			m.applyBuildToolModel(model)
			m.syncInitialBuildModels()
			m.invalidate()
		})
	}()
}

func (m *Model) refreshBuildToolRunConfigs() {
	if m.run.trusted && m.ctx != nil {
		m.loadRunConfigs()
	}
}

// SyncGradle is the Gradle tool window's Reload All Gradle Projects. The
// backend uses the same build_sync operation; each build directory is synced
// with the tool's toolbar switches.
func (m *Model) SyncGradle() {
	var dirs []string
	for _, build := range m.maven.model.Gradle {
		dirs = append(dirs, build.Dir)
	}
	if len(dirs) == 0 {
		return
	}
	m.SyncGradleBuilds(dirs)
}

// SyncGradleBuild reloads one linked build from the Gradle section's context
// menu. An empty directory addresses the opened project's root build.
func (m *Model) SyncGradleBuild(buildDir string) {
	m.SyncGradleBuilds([]string{buildDir})
}

// SyncGradleBuilds re-reads the task model through build_sync. The backend
// uses buildDir=null for the opened root build and a relative directory for an
// independently linked one.
func (m *Model) SyncGradleBuilds(dirs []string) {
	if m.maven.syncing || (m.mgr == nil && m.invokeHook == nil) || m.ctx == nil || m.Root() == "" || len(dirs) == 0 {
		return
	}
	unique := make([]string, 0, len(dirs))
	seen := map[string]bool{}
	for _, dir := range dirs {
		if !seen[dir] {
			seen[dir] = true
			unique = append(unique, dir)
		}
	}
	m.maven.gen++
	req := m.nextRequest()
	ch := m.nextChannel()
	m.maven.syncing = true
	m.maven.syncTool = "gradle"
	m.maven.syncMsg = "Syncing Gradle projects…"
	m.maven.syncFrac = nil
	m.maven.request = req
	m.maven.channel = ch
	m.maven.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		var first error
		var model buildModel
		for _, dir := range unique {
			var buildDir any
			if dir != "" {
				buildDir = dir
			}
			raw, err := m.invoke(ctx, "build_sync", map[string]any{
				"tool":       "gradle",
				"buildDir":   buildDir,
				"offline":    m.maven.gradleOffline,
				"profiles":   []string{},
				"request":    req,
				"onProgress": map[string]any{"$channel": ch},
			})
			if err != nil {
				first = err
				break
			}
			if err := json.Unmarshal(raw, &model); err != nil {
				first = err
				break
			}
		}
		m.post(func() {
			if m.maven.channel != ch {
				return
			}
			m.maven.syncing = false
			m.maven.syncTool = ""
			m.maven.syncMsg = ""
			m.maven.syncFrac = nil
			if first != nil {
				m.maven.err = "Could not reload the Gradle projects: " + first.Error()
				m.invalidate()
				return
			}
			m.maven.loading = false
			m.applyBuildToolModel(model)
			m.syncInitialBuildModels()
			m.invalidate()
		})
	}()
}

// CancelBuildToolReload stops the running Maven or Gradle sync, matching the
// tool windows' Stop Reloading actions.
func (m *Model) CancelBuildToolReload(tool string) {
	if !m.maven.syncing || m.maven.request == 0 {
		return
	}
	if tool != "" && m.maven.syncTool != "" && m.maven.syncTool != tool {
		return
	}
	request := m.maven.request
	m.maven.gen++
	m.maven.syncing = false
	m.maven.syncTool = ""
	m.maven.syncMsg = ""
	m.maven.syncFrac = nil
	m.maven.request = 0
	m.maven.channel = 0
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

// ReloadMaven and ReloadGradle are the two tool windows' Reload All actions.
func (m *Model) ReloadMaven()  { m.SyncMaven() }
func (m *Model) ReloadGradle() { m.SyncGradle() }

// applyBuildToolModel installs a model returned by build_model/build_sync and
// keeps the profiles and expansion state the two tool windows own.
func (m *Model) applyBuildToolModel(model buildModel) {
	m.maven.model = model
	m.mavenInitTree()
	m.refreshReadiness()
}

// AnalyzeBuildDependencies opens the native Dependency Analyzer for the active
// tree selection, after OpenIDEA's maven.analyzeDependencies and
// gradle.analyzeDependencies actions.
func (m *Model) AnalyzeBuildDependencies() {
	m.OpenBuildDependencyAnalyzer(nil)
}

// acceptMavenEvent is one SyncProgress value of the sync channel.
func (m *Model) acceptMavenEvent(data json.RawMessage) {
	var ev struct {
		Message  string   `json:"message"`
		Fraction *float32 `json:"fraction"`
	}
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	if ev.Message != "" {
		m.maven.syncMsg = ev.Message
	}
	m.maven.syncFrac = ev.Fraction
	m.invalidate()
}

// ToggleMavenOffline flips the sync's offline switch, as the toolbar's toggle.
func (m *Model) ToggleMavenOffline() {
	m.loadBuildToolSettings(m.Root())
	m.maven.offline = !m.maven.offline
	m.saveBuildToolSettings()
	m.invalidate()
}

// ToggleMavenSkipTests flips whether the test phase runs with tests skipped.
func (m *Model) ToggleMavenSkipTests() {
	m.loadBuildToolSettings(m.Root())
	m.maven.skipTests = !m.maven.skipTests
	m.saveBuildToolSettings()
	m.invalidate()
}

// ToggleGradleOffline and ToggleGradleSkipTests are the Gradle window's own
// switches, matching OpenIDEA's independent Gradle settings.
func (m *Model) ToggleGradleOffline() {
	m.loadBuildToolSettings(m.Root())
	m.maven.gradleOffline = !m.maven.gradleOffline
	m.saveBuildToolSettings()
	m.invalidate()
}

func (m *Model) ToggleGradleSkipTests() {
	m.loadBuildToolSettings(m.Root())
	m.maven.gradleSkipTests = !m.maven.gradleSkipTests
	m.saveBuildToolSettings()
	m.invalidate()
}

const buildToolsSettingsKey = "__goidea.buildTools"

type buildToolProjectSettings struct {
	Maven struct {
		Offline   bool            `json:"offline,omitempty"`
		SkipTests bool            `json:"skipTests,omitempty"`
		Profiles  map[string]bool `json:"profiles,omitempty"`
	} `json:"maven,omitempty"`
	Gradle struct {
		Offline   bool `json:"offline,omitempty"`
		SkipTests bool `json:"skipTests,omitempty"`
	} `json:"gradle,omitempty"`
}

type buildToolsSettings struct {
	Projects map[string]buildToolProjectSettings `json:"projects,omitempty"`
}

func buildToolsProjectKey(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root
}

// loadBuildToolSettings loads OpenIDEA's per-project toolbar choices. Its
// localStorage equivalent is native local state, so it lives beside plugin
// preferences rather than in the backend's Java settings.
func (m *Model) loadBuildToolSettings(root string) {
	key := buildToolsProjectKey(root)
	if m.maven.settingsRoot == key {
		return
	}
	m.maven.settingsRoot = key
	m.maven.offline = false
	m.maven.skipTests = false
	m.maven.gradleOffline = false
	m.maven.gradleSkipTests = false
	m.maven.profiles = nil
	if key == "" || len(m.prefs.Plugins[buildToolsSettingsKey]) == 0 {
		return
	}
	var settings buildToolsSettings
	if json.Unmarshal(m.prefs.Plugins[buildToolsSettingsKey], &settings) != nil {
		return
	}
	project, ok := settings.Projects[key]
	if !ok {
		return
	}
	m.maven.offline = project.Maven.Offline
	m.maven.skipTests = project.Maven.SkipTests
	m.maven.gradleOffline = project.Gradle.Offline
	m.maven.gradleSkipTests = project.Gradle.SkipTests
	if len(project.Maven.Profiles) > 0 {
		m.maven.profiles = map[string]bool{}
		for profile, checked := range project.Maven.Profiles {
			m.maven.profiles[profile] = checked
		}
	}
}

func (m *Model) saveBuildToolSettings() {
	key := buildToolsProjectKey(m.Root())
	if key == "" {
		return
	}
	m.loadBuildToolSettings(m.Root())
	var settings buildToolsSettings
	if len(m.prefs.Plugins[buildToolsSettingsKey]) > 0 {
		_ = json.Unmarshal(m.prefs.Plugins[buildToolsSettingsKey], &settings)
	}
	if settings.Projects == nil {
		settings.Projects = map[string]buildToolProjectSettings{}
	}
	project := settings.Projects[key]
	project.Maven.Offline = m.maven.offline
	project.Maven.SkipTests = m.maven.skipTests
	if len(m.maven.profiles) > 0 {
		project.Maven.Profiles = map[string]bool{}
		for profile, checked := range m.maven.profiles {
			project.Maven.Profiles[profile] = checked
		}
	}
	project.Gradle.Offline = m.maven.gradleOffline
	project.Gradle.SkipTests = m.maven.gradleSkipTests
	settings.Projects[key] = project
	raw, err := json.Marshal(settings)
	if err != nil {
		m.status = "Saving build tool settings failed: " + err.Error()
		return
	}
	if m.prefs.Plugins == nil {
		m.prefs.Plugins = map[string]json.RawMessage{}
	}
	m.prefs.Plugins[buildToolsSettingsKey] = raw
	_ = m.savePreferences()
}

// ExpandMavenAll opens every node of the tree.
func (m *Model) ExpandMavenAll() {
	if m.maven.expanded == nil {
		m.maven.expanded = map[string]bool{}
	}
	var walk func(rows []mavenRow)
	rows := m.mavenRootRows()
	walk = func(rows []mavenRow) {
		for _, r := range rows {
			if r.hasKids {
				m.maven.expanded[r.key] = true
				walk(m.mavenChildRows(r))
			}
		}
	}
	walk(rows)
	m.invalidate()
}

// CollapseMavenAll closes Maven nodes without changing the Gradle tree.
func (m *Model) CollapseMavenAll() {
	for key := range m.maven.expanded {
		if strings.HasPrefix(key, "mvn:") {
			delete(m.maven.expanded, key)
		}
	}
	m.invalidate()
}

// RunMavenSelection runs the selected phase, goal or configuration.
func (m *Model) RunMavenSelection() {
	sel := m.maven.sel
	switch sel.kind {
	case "runConfig":
		if sel.config != nil {
			m.startRunWith(sel.config.Raw(), sel.config.Name)
		}
	case "phase", "goal":
		if sel.project == nil {
			return
		}
		m.runMavenGoal(sel.project, []string{sel.goal})
	}
}

// RunMavenGoal runs the typed goals on a project, as Execute Maven Goal does.
func (m *Model) RunMavenGoal(projectDir string, goals []string) {
	for i := range m.maven.model.MavenProjects() {
		p := &m.maven.model.MavenProjects()[i]
		if p.Dir == projectDir {
			m.runMavenGoal(p, goals)
			return
		}
	}
}

// runMavenGoal starts a maven configuration for the goals, like
// mavenGoalConfig: the project's launcher, its directory, the settings' flags.
func (m *Model) runMavenGoal(p *mavenProject, goals []string) {
	if p == nil || len(goals) == 0 {
		return
	}
	cfg, name := m.mavenGoalRunConfig(p, goals)
	raw, err := json.Marshal(cfg)
	if err != nil {
		m.status = "Could not start the Maven goal: " + err.Error()
		m.invalidate()
		return
	}
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
	m.status = "Running " + strings.Join(goals, " ")
	m.startRunWith(raw, name)
}

// mavenGoalRunConfig is mavenGoalConfig: the launcher (the project's wrapper
// or mvn), the goals with the settings' flags, the project's directory and the
// name IDEA gives the configuration.
func (m *Model) mavenGoalRunConfig(p *mavenProject, goals []string) (openidea.RunConfig, string) {
	command, args := m.mavenLaunch(p, goals)
	has := func(flag string) bool {
		for _, a := range args {
			if a == flag || strings.HasPrefix(a, flag+"=") {
				return true
			}
		}
		return false
	}
	if m.maven.offline && !has("-o") && !has("--offline") {
		args = insertMavenExecutionFlags(args, "-o")
	}
	if m.maven.skipTests && !has("-DskipTests") {
		args = insertMavenExecutionFlags(args, "-DskipTests")
	}
	label := p.DisplayName
	if label == "" {
		label = p.ArtifactID
	}
	namedGoals := make([]string, 0, len(goals))
	for _, goal := range goals {
		if !strings.HasPrefix(goal, "-") {
			namedGoals = append(namedGoals, goal)
		}
	}
	name := label + " [" + strings.Join(namedGoals, ",") + "]"
	var cwd *string
	if p.Dir != "" {
		dir := p.Dir
		cwd = &dir
	}
	settingsEnv := map[string]string{}
	if jre := strings.TrimSpace(
		nativeSettingsString(m, settingsPageMaven, "languages.java.maven.runnerJre"),
	); jre != "" {
		settingsEnv["JAVA_HOME"] = expandMavenHome(jre)
	}
	// A whole RunConfig, not a sparse map: the backend's deserializer has no
	// defaults for isTest and debugAdapter, so a partial object is refused
	// with "missing field `isTest`".
	return openidea.RunConfig{
		ID:           "maven:goal:" + p.Pom + ":" + strings.Join(goals, " "),
		Name:         name,
		Kind:         "maven",
		Command:      command,
		Args:         args,
		Cwd:          cwd,
		Env:          settingsEnv,
		Detected:     false,
		IsTest:       mavenTestGoals(goals, args),
		DebugAdapter: nil,
		EnvFiles:     []string{},
	}, name
}

func (m *Model) mavenLaunch(p *mavenProject, goals []string) (string, []string) {
	if p == nil {
		return "mvn", append([]string(nil), goals...)
	}
	m.nativeSettingsMu.Lock()
	settings := m.nativeSettings.pages[settingsPageMaven]
	m.nativeSettingsMu.Unlock()
	settingString := func(key, fallback string) string {
		return nativeAppliedString(settings, key, fallback)
	}
	settingBool := func(key string, fallback bool) bool {
		return nativeAppliedBool(settings, key, fallback)
	}
	command := "mvn"
	if mvnw.MatchString(p.Command) {
		command = p.Command
	}
	if detected := strings.TrimSpace(p.Command); detected != "" && !mvnw.MatchString(detected) {
		if path, err := exec.LookPath(detected); err == nil {
			command = path
		} else {
			command = detected
		}
	}
	if custom := strings.TrimSpace(
		settingString("languages.java.maven.home", ""),
	); custom != "" {
		command = mavenExecutable(custom)
	}
	if settingBool("languages.java.maven.daemon", false) {
		if !mavenRunnerJreSupportsDaemon(
			settingString("languages.java.maven.runnerJre", ""),
		) {
			m.status = "Maven Daemon needs Java 17 or newer; building with Maven"
		} else if daemon := m.mavenDaemonExecutable(); daemon != "" {
			command = daemon
		} else {
			m.status = "Maven Daemon is enabled but mvnd could not be resolved; building with Maven"
		}
	}
	args := []string{}
	appendSettingArg := func(flag, key string) {
		if value := strings.TrimSpace(
			settingString(key, ""),
		); value != "" {
			args = append(args, flag, expandMavenHome(value))
		}
	}
	appendSettingArg("-s", "languages.java.maven.userSettings")
	appendSettingArg("-t", "languages.java.maven.toolchains")
	if repository := strings.TrimSpace(
		settingString("languages.java.maven.localRepository", ""),
	); repository != "" {
		args = append(args, "-Dmaven.repo.local="+expandMavenHome(repository))
	}
	if threads := normalizedMavenThreads(
		settingString("languages.java.maven.threads", ""),
	); threads != "" {
		args = append(args, "-T", threads)
	}
	profiles := mavenActiveProfiles(
		settingString("languages.java.maven.activeProfiles", ""),
		m.mavenProfilesListForProject(p),
	)
	if len(profiles) > 0 {
		args = append(args, "-P", strings.Join(profiles, ","))
	}
	return command, append(args, goals...)
}

func mavenExecutable(home string) string {
	home = expandMavenHome(home)
	if home == "" {
		return "mvn"
	}
	if info, err := os.Stat(home); err == nil && !info.IsDir() {
		return home
	}
	endsWithSeparator := strings.HasSuffix(home, string(filepath.Separator))
	if !endsWithSeparator && strings.ContainsRune(home, filepath.Separator) {
		if _, err := os.Stat(home); err != nil {
			return home
		}
	}
	if !endsWithSeparator && !strings.ContainsRune(home, filepath.Separator) {
		if path, err := exec.LookPath(home); err == nil {
			return path
		}
	}
	if endsWithSeparator {
		home = strings.TrimRight(home, string(filepath.Separator))
	}
	if name := filepath.Base(home); name == "mvn" || name == "mvn.cmd" ||
		name == "mvn.bat" {
		if _, err := os.Stat(home); err == nil {
			return home
		}
	}
	name := "mvn"
	if runtime.GOOS == "windows" {
		name = "mvn.cmd"
	}
	for _, candidate := range []string{
		filepath.Join(home, "bin", name),
		filepath.Join(home, name),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return home
}

func (m *Model) mavenDaemonExecutable() string {
	home := strings.TrimSpace(nativeSettingsString(
		m, settingsPageMaven, "languages.java.maven.mvndHome",
	))
	if home == "" {
		home = strings.TrimSpace(os.Getenv("MVND_HOME"))
	}
	if home == "" {
		if path, err := exec.LookPath("mvnd"); err == nil {
			return path
		}
		return ""
	}
	home = expandMavenHome(home)
	if info, err := os.Stat(home); err == nil && !info.IsDir() {
		return home
	}
	name := "mvnd"
	if runtime.GOOS == "windows" {
		name = "mvnd.cmd"
	}
	for _, candidate := range []string{
		filepath.Join(home, "bin", name),
		filepath.Join(home, name),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func mavenRunnerJreSupportsDaemon(path string) bool {
	path = expandMavenHome(path)
	if path == "" {
		return true
	}
	candidates := []string{filepath.Join(path, "bin", "java")}
	if runtime.GOOS == "windows" {
		candidates = append(
			[]string{filepath.Join(path, "bin", "java.exe")},
			candidates...,
		)
	}
	java := ""
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			java = candidate
			break
		}
	}
	if java == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, java, "-version").CombinedOutput()
	if err != nil {
		return false
	}
	return javaRuntimeAtLeast17(string(out))
}

func javaRuntimeAtLeast17(output string) bool {
	fields := strings.Fields(output)
	for i, field := range fields {
		if field == "version" && i+1 < len(fields) {
			version := strings.Trim(fields[i+1], `"`)
			parts := strings.Split(version, ".")
			major := 0
			if len(parts) > 0 {
				major, _ = strconv.Atoi(parts[0])
			}
			if major == 1 && len(parts) > 1 {
				major, _ = strconv.Atoi(parts[1])
			}
			return major >= 17
		}
	}
	return false
}

func expandMavenHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func normalizedMavenThreads(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	number := strings.TrimSuffix(value, "C")
	parts := strings.Split(number, ".")
	if len(parts) > 2 {
		return ""
	}
	for _, part := range parts {
		if part == "" {
			return ""
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return ""
			}
		}
	}
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil || parsed <= 0 {
		return ""
	}
	return value
}

func mavenActiveProfiles(settings string, choices []string) []string {
	combined := append(strings.Split(settings, ","), choices...)
	profiles := make([]string, 0, len(combined))
	for _, item := range combined {
		item = strings.TrimSpace(item)
		if item == "" || slices.Contains(profiles, item) {
			continue
		}
		profiles = append(profiles, item)
	}
	excluded := map[string]bool{}
	for _, profile := range profiles {
		profile = strings.TrimSpace(profile)
		if strings.HasPrefix(profile, "!") || strings.HasPrefix(profile, "-") {
			excluded[strings.TrimLeft(profile, "!-")] = true
		}
	}
	out := profiles[:0]
	for _, profile := range profiles {
		if !strings.HasPrefix(profile, "!") &&
			!strings.HasPrefix(profile, "-") &&
			!excluded[profile] {
			out = append(out, profile)
		}
	}
	return out
}

func insertMavenExecutionFlags(args []string, flags ...string) []string {
	prefix := 0
	for prefix < len(args) {
		flag := args[prefix]
		switch {
		case flag == "-P" || flag == "--activate-profiles":
			prefix = min(prefix+2, len(args))
		case flag == "-s" || flag == "--settings" ||
			flag == "-t" || flag == "--toolchains" ||
			flag == "-T" || flag == "--threads" ||
			false:
			prefix = min(prefix+2, len(args))
		case strings.HasPrefix(flag, "-s=") ||
			strings.HasPrefix(flag, "--settings=") ||
			strings.HasPrefix(flag, "-t=") ||
			strings.HasPrefix(flag, "--toolchains=") ||
			strings.HasPrefix(flag, "-T") ||
			strings.HasPrefix(flag, "--threads=") ||
			strings.HasPrefix(flag, "-P") ||
			strings.HasPrefix(flag, "--activate-profiles=") ||
			strings.HasPrefix(flag, "-Dmaven.repo.local=") ||
			strings.HasPrefix(flag, "-D"):
			prefix++
		default:
			goto insert
		}
	}
insert:
	out := make([]string, 0, len(args)+len(flags))
	out = append(out, args[:prefix]...)
	out = append(out, flags...)
	return append(out, args[prefix:]...)
}

// --- tree -----------------------------------------------------------------

// mavenRow is one visible tree row: the node's key, its depth and how it draws.
type mavenRow struct {
	key     string
	depth   int
	kind    string // profiles, profile, project, lifecycle, phase, plugins, plugin, goal, dependencies, dependency, runConfigs, runConfig, gradle
	label   string
	detail  string
	project *mavenProject
	profile *mavenProfile
	goal    string
	config  *openidea.RunConfig
	dep     *mavenDependency
	direct  *mavenDependency
	hasKids bool
	checked bool
	omitted bool
}

// mavenRootRows is the tree's top level: the profiles groups and the
// non-aggregated projects, as mavenRoots builds it.
func (m *Model) mavenRootRows() []mavenRow {
	var rows []mavenRow
	if mm := m.maven.model.Maven; mm != nil {
		groups := map[string]bool{}
		for i := range mm.Profiles {
			prof := &mm.Profiles[i]
			root := ""
			if prof.ProjectRoot != nil {
				root = *prof.ProjectRoot
			}
			if groups[root] {
				continue
			}
			groups[root] = true
			label := "Profiles"
			if root != "" {
				label += " (" + pathBase(root) + ")"
			}
			rows = append(rows, mavenRow{key: "mvn:profiles" + root, kind: "profiles", label: label,
				hasKids: true, depth: 0})
		}
		for i := range mm.Projects {
			p := &mm.Projects[i]
			if p.Aggregated {
				continue
			}
			rows = append(rows, m.mavenProjectRow(p, 0))
		}
	}
	return rows
}

func (m *Model) mavenProjectRow(p *mavenProject, depth int) mavenRow {
	label := p.DisplayName
	if label == "" {
		label = p.ArtifactID
	}
	row := mavenRow{key: mavenProjectKey(*p), kind: "project", label: label, depth: depth,
		project: p, hasKids: true}
	if p.Version != "" {
		row.detail = p.Version
	}
	return row
}

// mavenChildRows lists a node's children, mirroring mavenChildren.
func (m *Model) mavenChildRows(r mavenRow) []mavenRow {
	switch r.kind {
	case "profiles":
		var out []mavenRow
		mm := m.maven.model.Maven
		if mm == nil {
			return nil
		}
		for i := range mm.Profiles {
			prof := &mm.Profiles[i]
			root := ""
			if prof.ProjectRoot != nil {
				root = *prof.ProjectRoot
			}
			groupRoot := strings.TrimPrefix(r.key, "mvn:profiles")
			if root != groupRoot {
				continue
			}
			out = append(out, mavenRow{key: "mvn:profile:" + mavenProfileKey(*prof), kind: "profile",
				label: prof.ID, depth: r.depth + 1, profile: prof, checked: m.mavenProfileChecked(*prof)})
		}
		return out
	case "project":
		p := r.project
		out := []mavenRow{
			{key: r.key + ":lifecycle", kind: "lifecycle", label: "Lifecycle", depth: r.depth + 1, project: p, hasKids: true},
			{key: r.key + ":plugins", kind: "plugins", label: "Plugins", depth: r.depth + 1, project: p, hasKids: len(p.Plugins) > 0},
		}
		if len(p.Dependencies) > 0 {
			out = append(out, mavenRow{key: r.key + ":deps", kind: "dependencies", label: "Dependencies",
				depth: r.depth + 1, project: p, hasKids: true})
		}
		if configs := m.mavenConfigsOf(*p); len(configs) > 0 {
			out = append(out, mavenRow{key: r.key + ":runs", kind: "runConfigs", label: "Run Configurations",
				depth: r.depth + 1, project: p, hasKids: true})
		}
		for _, pom := range p.Modules {
			if child := m.mavenProjectByPom(pom); child != nil {
				out = append(out, m.mavenProjectRow(child, r.depth+1))
			}
		}
		return out
	case "lifecycle":
		var out []mavenRow
		for _, phase := range standardLifecycle {
			out = append(out, mavenRow{key: r.key + ":" + phase, kind: "phase", label: phase,
				depth: r.depth + 1, project: r.project, goal: phase})
		}
		return out
	case "plugins":
		var out []mavenRow
		for i := range r.project.Plugins {
			pl := &r.project.Plugins[i]
			label := mavenPluginLabel(*pl)
			out = append(out, mavenRow{key: r.key + ":" + pl.GroupID + ":" + pl.ArtifactID, kind: "plugin",
				label: label, depth: r.depth + 1, project: r.project, hasKids: len(pl.Goals) > 0,
				detail: pluginVersion(*pl)})
		}
		return out
	case "plugin":
		var out []mavenRow
		pl := mavenPluginByKey(r.project, r.key)
		if pl == nil {
			return nil
		}
		for _, goal := range pl.Goals {
			out = append(out, mavenRow{key: r.key + ":" + goal, kind: "goal",
				label: pl.Prefix + ":" + goal, detail: "", depth: r.depth + 1,
				project: r.project, goal: pl.Prefix + ":" + goal})
		}
		return out
	case "dependencies":
		return mavenDependencyRows(r.key, r.project.Dependencies, r.depth+1, nil)
	case "dependency":
		if r.dep == nil {
			return nil
		}
		return mavenDependencyRows(r.key, r.dep.Children, r.depth+1, r.direct)
	case "runConfigs":
		var out []mavenRow
		for _, cfg := range m.mavenConfigsOf(*r.project) {
			out = append(out, mavenRow{key: r.key + ":" + cfg.ID, kind: "runConfig", label: cfg.Name,
				depth: r.depth + 1, project: r.project, config: cfg})
		}
		return out
	}
	return nil
}

func mavenDependencyRows(
	parent string,
	deps []mavenDependency,
	depth int,
	direct *mavenDependency,
) []mavenRow {
	var out []mavenRow
	for i := range deps {
		d := &deps[i]
		if d.Omitted != nil {
			continue
		}
		label := d.GroupID + ":" + d.ArtifactID
		if d.Version != "" {
			label += ":" + d.Version
		}
		detail := ""
		if d.Scope != "" && d.Scope != "compile" {
			detail = "(" + d.Scope
			if d.Optional {
				detail += ", optional"
			}
			detail += ")"
		} else if d.Optional {
			detail = "(optional)"
		}
		root := direct
		if root == nil {
			root = d
		}
		out = append(out, mavenRow{key: fmt.Sprintf("%s/%d", parent, i), kind: "dependency",
			label: label, detail: detail, depth: depth, dep: d, direct: root,
			hasKids: len(d.Children) > 0})
	}
	return out
}

// --- panel ----------------------------------------------------------------

// mavenPanel draws the toolbar, the banner and the tree.
func (m *Model) mavenPanel(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).Children(func() {
		m.mavenToolbar(c, p)
		m.mavenBanner(c, p)
		if m.maven.err != "" {
			m.buildToolErrorBanner(c, p, "maven:modelError", m.maven.err, func() {
				m.loadBuildModel()
			})
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(0, 6).Children(func() {
				m.mavenTreeRows(c, p, m.mavenRootRows())
			})
		})
	})
}

// mavenToolbar is OpenIDEA's icon row: reload, generate sources, run, execute
// goal, offline, skip tests, expand and collapse.
func (m *Model) mavenToolbar(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(32).Padding(0, 6).Gap(2).AlignItems(ui.Center).Children(func() {
		if m.maven.syncing {
			m.mavenAction(c, p, "maven:reload", "run/stop", "Stop", func() { m.CancelBuildToolReload("maven") })
		} else if m.maven.loading {
			m.mavenActionDisabled(c, p, "maven:reload", "general/refresh", "Loading Maven Projects", false)
		} else {
			m.mavenAction(c, p, "maven:reload", "general/refresh", "Reload All Maven Projects", func() { m.SyncMaven() })
		}
		m.mavenAction(c, p, "maven:sources", "nodes/generatedSource", "Generate Sources", func() {
			m.GenerateMavenSources("")
		})
		m.mavenAction(c, p, "maven:add", "general/add", "Add Maven Projects", func() { m.AddMavenProjects() })
		m.mavenSeparator(c, p)
		runnable := m.maven.sel.kind == "phase" || m.maven.sel.kind == "goal" || m.maven.sel.kind == "runConfig"
		if runnable {
			m.mavenAction(c, p, "maven:run", "run/run", "Run Maven Build", func() { m.RunMavenSelection() })
		} else {
			m.mavenActionDisabled(c, p, "maven:run", "run/run", "Run Maven Build", false)
		}
		m.mavenAction(c, p, "maven:goal", "general/runAnything", "Execute Maven Goal", func() {
			m.OpenBuildToolExecute("maven")
		})
		m.mavenToggle(c, p, "maven:offline", "build/toggleOfflineMode", "Work Offline", m.maven.offline,
			func() { m.ToggleMavenOffline() })
		m.mavenAction(c, p, "maven:analyze", "build/dependencyAnalyzer", "Analyze Dependencies", func() {
			m.AnalyzeBuildDependencies()
		})
		m.mavenToggle(c, p, "maven:skipTests", "run/showIgnored", "Skip Tests", m.maven.skipTests,
			func() { m.ToggleMavenSkipTests() })
		m.mavenSeparator(c, p)
		m.mavenAction(c, p, "maven:expand", "general/expandAll", "Expand All", func() { m.ExpandMavenAll() })
		m.mavenAction(c, p, "maven:collapse", "general/collapseAll", "Collapse All", func() { m.CollapseMavenAll() })
		m.mavenAction(c, p, "maven:settings", "general/settings", "Maven Settings…", func() {
			m.OpenSettingsPage("languages.java")
		})
	})
}

func (m *Model) mavenSeparator(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Width(1).Height(16).Shrink(0).Margin(0, 4).Background(p.Border)
}

// mavenAction is one of OpenIDEA's small IconButtons: 20px, a 16px glyph in
// fg-muted that brightens on hover.
func (m *Model) mavenAction(c *ui.Context, p *ui.Theme, key, icon, tip string, run func()) {
	oi := oiPaletteOf(p)
	b := ui.ButtonBase(c.Key(key)).Size(20, 20).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	tone := oi.textMuted
	if b.Hovered() {
		tone = oi.textStrong
	}
	b.Tooltip(tip)
	b.Children(func() { oiIconEl(c, icon, 16, oi, tone) })
	if b.Clicked() {
		run()
	}
}

// mavenActionDisabled draws an action this build does not carry yet: it stays
// visible for the toolbar's shape but does nothing, as OpenIDEA disables it.
func (m *Model) mavenActionDisabled(c *ui.Context, p *ui.Theme, key, icon, tip string, always bool) {
	_ = always
	oi := oiPaletteOf(p)
	b := ui.ButtonBase(c.Key(key)).Size(20, 20).Radius(4).Justify(ui.Center).AlignItems(ui.Center)
	b.Tooltip(tip)
	b.Children(func() { oiIconEl(c, icon, 16, oi, oi.textMuted.Alpha(0.4)) })
}

func (m *Model) mavenToggle(c *ui.Context, p *ui.Theme, key, icon, tip string, on bool, run func()) {
	oi := oiPaletteOf(p)
	b := ui.ButtonBase(c.Key(key)).Size(20, 20).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	tone := oi.textMuted
	if on {
		b.Background(oi.selectedInactive)
		tone = oi.textStrong
	} else if b.Hovered() {
		tone = oi.textStrong
	}
	b.Tooltip(tip)
	b.Children(func() { oiIconEl(c, icon, 16, oi, tone) })
	if b.Clicked() {
		run()
	}
}

// mavenBanner shows the sync's progress, the changed-build-files notice with
// Load Maven Changes, or the last sync error.
func (m *Model) mavenBanner(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	mm := m.maven.model.Maven
	switch {
	case m.maven.syncing:
		ui.Row(c).MinHeight(28).Padding(2, 10).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, m.maven.syncMsg).FontSize(12).TextColor(oi.textSecondary).SingleLine().Grow(1).Ellipsis("…")
			if f := m.maven.syncFrac; f != nil {
				ui.Text(c, fmt.Sprintf("%d%%", int(*f*100))).FontSize(12).TextColor(oi.textSubtle)
			}
		})
	case mm != nil && mm.Stale:
		ui.Row(c).MinHeight(28).Padding(2, 10).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			oiIconEl(c, "maven/mavenLoadChanges", 16, oi, oi.textMuted)
			ui.Text(c, "Build files changed since the last reload").FontSize(12).TextColor(oi.textSecondary).
				SingleLine().Grow(1).Ellipsis("…")
			m.bannerButton(c, p, "maven:load", "Load Maven Changes", func() { m.SyncMaven() })
		})
	case mm != nil && mm.SyncErr != nil && *mm.SyncErr != "":
		ui.Row(c).MinHeight(28).Padding(2, 10).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			oiIconEl(c, "status/warning", 16, oi, oi.warning)
			ui.Text(c, strings.SplitN(*mm.SyncErr, "\n", 2)[0]).FontSize(12).TextColor(oi.warning).
				SingleLine().Grow(1).Ellipsis("…")
			m.bannerButton(c, p, "maven:retry", "Refresh", func() { m.SyncMaven() })
		})
	}
}

// bannerButton is the banner's ghost Button in its small size.
func (m *Model) bannerButton(c *ui.Context, p *ui.Theme, key, label string, run func()) {
	oi := oiPaletteOf(p)
	b := ui.ButtonBase(c.Key(key)).Height(20).Padding(0, 8).Radius(4).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.hoverTint)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(oi.textStrong).SingleLine()
	})
	b.OnClick(run)
}

// mavenTreeRows draws rows and, for expanded ones, their children.
func (m *Model) mavenTreeRows(c *ui.Context, p *ui.Theme, rows []mavenRow) {
	for _, r := range rows {
		r := r
		m.mavenRow(c, p, r)
		if r.hasKids && m.maven.expanded[r.key] {
			m.mavenTreeRows(c, p, m.mavenChildRows(r))
		}
	}
}

func (m *Model) mavenRow(c *ui.Context, p *ui.Theme, r mavenRow) {
	oi := oiPaletteOf(p)
	row := ui.Row(c.Key(r.key)).
		Height(24).PaddingStart(4 + float32(r.depth)*m.treeIndentWidth()).PaddingEnd(8).
		Gap(4).Radius(5).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if m.maven.selKey == r.key {
		row.Background(oi.selectedInactive)
	} else if row.Hovered() {
		row.Background(oi.hoverTint)
	}
	row.Children(func() {
		if r.kind == "profile" {
			on := r.checked
			if ui.Checkbox(c, &on, r.label).Changed() && r.profile != nil {
				m.SetMavenProfile(*r.profile, on)
			}
			return
		}
		chevron := ui.Row(c).Size(16, 16).Shrink(0).Justify(ui.Center).AlignItems(ui.Center)
		if r.hasKids {
			glyph := "general/chevronRight"
			if m.maven.expanded[r.key] {
				glyph = "general/chevronDown"
			}
			chevron.Children(func() { oiIconEl(c, glyph, 16, oi, oi.textMuted) })
		}
		if name := mavenRowIcon(r.kind); name != "" {
			oiIconEl(c, name, 16, oi, oi.textMuted).MarginEnd(2)
		}
		label := r.label
		tone := oi.text
		if m.maven.selKey == r.key {
			tone = oi.textStrong
		}
		t := ui.Text(c, label).FontSize(13).TextColor(tone).SingleLine().Ellipsis("…")
		if r.kind == "phase" && m.maven.skipTests && label == "test" {
			t.TextColor(oi.textMuted).Strikethrough()
		}
		if r.detail != "" {
			ui.Text(c, r.detail).FontSize(12).TextColor(oi.textSubtle).SingleLine().Ellipsis("…")
		}
	})
	if r.hasKids {
		row.OnClick(func() {
			if m.maven.expanded == nil {
				m.maven.expanded = map[string]bool{}
			}
			if r.kind == "project" && r.project != nil {
				m.selectMavenProject(r.project)
			}
			m.maven.expanded[r.key] = !m.maven.expanded[r.key]
			m.invalidate()
		})
	}
	if r.kind == "phase" || r.kind == "goal" || r.kind == "runConfig" {
		row.OnClick(func() {
			m.maven.sel = mavenSelection{kind: r.kind, project: r.project, goal: r.goal, config: r.config}
			m.maven.selKey = r.key
			m.invalidate()
		})
		if row.DoubleClicked() {
			m.maven.sel = mavenSelection{kind: r.kind, project: r.project, goal: r.goal, config: r.config}
			m.maven.selKey = r.key
			m.RunMavenSelection()
		}
	}
	switch r.kind {
	case "project", "phase", "goal", "runConfig", "dependencies", "dependency":
		row.ContextMenu(func(menu *ui.Menu) {
			if r.kind == "project" && r.project != nil {
				m.selectMavenProject(r.project)
			}
			m.selectMavenBuildRow(r)
			m.mavenRowContextMenu(menu, r)
		})
	}
}

// --- helpers --------------------------------------------------------------

// MavenProjects lists the build model's Maven modules.
func (b *buildModel) MavenProjects() []mavenProject {
	if b.Maven == nil {
		return nil
	}
	return b.Maven.MavenProjects()
}

// MavenProjects lists the reactor's modules in a stable order.
func (m *mavenModel) MavenProjects() []mavenProject {
	if m == nil || len(m.Projects) == 0 {
		return nil
	}
	return m.Projects
}

func mavenProjectKey(p mavenProject) string { return "mvn:" + p.Pom }

// mavenTestGoals reports whether the goals run tests, as MAVEN_TEST_GOALS does
// upstream: test, verify, integration-test, install, deploy or a surefire goal.
func mavenTestGoals(goals, args []string) bool {
	for _, a := range args {
		if a == "-DskipTests" {
			return false
		}
	}
	for _, g := range goals {
		switch g {
		case "test", "verify", "integration-test", "install", "deploy",
			"surefire:test", "failsafe:integration-test":
			return true
		}
	}
	return false
}

func mavenProfileKey(p mavenProfile) string {
	root := ""
	if p.ProjectRoot != nil {
		root = *p.ProjectRoot
	}
	return root + "\x00" + p.ID
}

func mavenPluginLabel(p mavenPlugin) string {
	if p.Prefix != "" {
		return p.Prefix
	}
	return p.GroupID + ":" + p.ArtifactID
}

func pluginVersion(p mavenPlugin) string {
	if p.Version != nil {
		return *p.Version
	}
	return ""
}

func (m *Model) mavenProjectByPom(pom string) *mavenProject {
	if m.maven.model.Maven == nil {
		return nil
	}
	for i := range m.maven.model.Maven.Projects {
		if m.maven.model.Maven.Projects[i].Pom == pom {
			return &m.maven.model.Maven.Projects[i]
		}
	}
	return nil
}

func (m *Model) selectMavenProject(project *mavenProject) {
	if project == nil || project.Pom == "" {
		return
	}
	m.maven.currentPOM = project.Pom
	m.maven.activeTool = "maven"
	m.maven.sel = mavenSelection{kind: "project", project: project}
	m.maven.selKey = mavenProjectKey(*project)
	m.invalidate()
}

func mavenPluginByKey(p *mavenProject, key string) *mavenPlugin {
	if p == nil {
		return nil
	}
	for i := range p.Plugins {
		pl := &p.Plugins[i]
		if strings.HasSuffix(key, ":"+pl.GroupID+":"+pl.ArtifactID) {
			return pl
		}
	}
	return nil
}

// mavenConfigsOf lists the maven run configurations working in a project's
// directory, as mavenRunConfigs does.
func (m *Model) mavenConfigsOf(p mavenProject) []*openidea.RunConfig {
	var out []*openidea.RunConfig
	for i := range m.run.configs {
		cfg := &m.run.configs[i]
		if cfg.Kind != "maven" || !m.mavenConfigBelongsTo(cfg, p) {
			continue
		}
		out = append(out, cfg)
	}
	return out
}

func (m *Model) mavenRootProject() *mavenProject {
	pom := m.maven.currentPOM
	if m.maven.sel.project != nil && m.maven.sel.project.Pom != "" {
		pom = m.maven.sel.project.Pom
	}
	if pom != "" {
		if project := m.mavenProjectByPom(pom); project != nil {
			return project
		}
	}
	if m.maven.model.Maven == nil {
		return nil
	}
	for i := range m.maven.model.Maven.Projects {
		p := &m.maven.model.Maven.Projects[i]
		if !p.Aggregated {
			return p
		}
	}
	return nil
}

func pathBase(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// mavenRowIcon is the row's expui drawing, as nodeIcon picks it upstream.
func mavenRowIcon(kind string) string {
	switch kind {
	case "project":
		return "maven/mavenProject"
	case "module":
		return "maven/mavenModule"
	case "lifecycle", "profiles", "plugins", "runConfigs", "dependencies", "tasks":
		return "nodes/folder"
	case "phase":
		return "maven/mavenGoal"
	case "plugin":
		return "maven/mavenPlugin"
	case "goal", "runConfig":
		return "maven/mavenPluginGoal"
	case "dependency":
		return "nodes/libraryFolder"
	}
	return ""
}

// SmokeMaven loads the build model for the open project and reports its size,
// for the real-server acceptance run.
func (m *Model) SmokeMaven(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	m.post(func() { m.ShowMaven() })
	deadline := time.Now().Add(60 * time.Second)
	for {
		var loading bool
		var errText string
		var n, deps, gradle, profiles, plugins int
		var visible bool
		m.post(func() {
			loading, errText = m.maven.loading, m.maven.err
			visible = m.maven.visible
			n = len(m.maven.model.MavenProjects())
			gradle = len(m.maven.model.Gradle)
			if mm := m.maven.model.Maven; mm != nil {
				profiles = len(mm.Profiles)
			}
			for _, p := range m.maven.model.MavenProjects() {
				deps += len(p.Dependencies)
				plugins += len(p.Plugins)
			}
		})
		if errText != "" {
			return out, fmt.Errorf("%s", errText)
		}
		if !loading {
			out["modules"], out["dependencies"] = n, deps
			out["gradle"] = gradle
			out["profiles"], out["plugins"] = profiles, plugins
			out["visible"] = visible
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("Reading the build model timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
