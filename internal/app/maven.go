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
	"regexp"
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
	Optional   bool              `json:"optional"`
	Omitted    *string           `json:"omitted"`
	Children   []mavenDependency `json:"children"`
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

type gradleModel struct {
	Dir      string            `json:"dir"`
	Projects []json.RawMessage `json:"projects"`
	Error    string            `json:"error"`
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
	loading bool

	// syncing is Load Maven Changes in flight: its message and fraction.
	syncing  bool
	syncMsg  string
	syncFrac *float32
	channel  uint32

	// profiles are the user's choices by profile key; a choice that differs
	// from the POM's activeByDefault is sent to the sync.
	profiles map[string]bool
	// offline and skipTests are the toolbar's two toggles.
	offline   bool
	skipTests bool

	err          string
	model        buildModel
	expanded     map[string]bool
	expandedInit bool
	sel          mavenSelection
	selKey       string
	gen          uint64
}

// ShowMaven opens the Maven window and loads the build model.
func (m *Model) ShowMaven() {
	m.maven.visible = true
	m.DockShow("maven")
	m.loadBuildModel()
}

// CloseMaven hides it without side effects.
func (m *Model) CloseMaven() { m.maven.visible = false; m.DockHide("maven") }

// ToggleMaven is the Window menu and palette action.
func (m *Model) ToggleMaven() { m.maven.visible = !m.maven.visible; m.DockToggle("maven") }

// loadBuildModel asks the backend for the reactor and Gradle builds.
func (m *Model) loadBuildModel() {
	if m.maven.loading || m.ctx == nil || m.mgr == nil || m.Root() == "" {
		return
	}
	m.maven.loading = true
	m.maven.err = ""
	m.maven.gen++
	gen := m.maven.gen
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "build_model", nil)
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
				m.maven.model = model
				m.mavenInitTree()
				m.refreshReadiness()
			}
			m.invalidate()
		})
	}()
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
			m.maven.profiles[mavenProfileKey(prof)] = prof.ActiveByDefault
		}
	}
}

// mavenProfilesList is the sync's -P list: the choices that differ from the
// defaults, deactivated ones prefixed with "!".
func (m *Model) mavenProfilesList() []string {
	var out []string
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

func (m *Model) mavenProfileChecked(p mavenProfile) bool {
	if v, ok := m.maven.profiles[mavenProfileKey(p)]; ok {
		return v
	}
	return p.ActiveByDefault
}

// SetMavenProfile is a profile checkbox: the choice syncs the project again,
// since the language server resolves it with the profiles of the last sync.
func (m *Model) SetMavenProfile(p mavenProfile, on bool) {
	if m.maven.profiles == nil {
		m.maven.profiles = map[string]bool{}
	}
	m.maven.profiles[mavenProfileKey(p)] = on
	m.SyncMaven()
}

// SyncMaven is Load Maven Changes: build_sync with the current offline and
// profile choices, streaming its progress into the banner.
func (m *Model) SyncMaven() {
	if m.maven.syncing || m.mgr == nil || m.ctx == nil || m.Root() == "" {
		return
	}
	m.maven.gen++
	req := m.nextRequest()
	ch := m.nextChannel()
	m.maven.syncing = true
	m.maven.syncMsg = "Syncing Maven projects…"
	m.maven.syncFrac = nil
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
		_, err := m.mgr.Invoke(ctx, Window, "build_sync", args)
		m.post(func() {
			if m.maven.channel != ch {
				return
			}
			m.maven.syncing = false
			m.maven.syncMsg = ""
			m.maven.syncFrac = nil
			if err != nil {
				m.maven.err = "Could not load the Maven changes: " + err.Error()
				m.invalidate()
				return
			}
			m.maven.loading = false
			m.loadBuildModel()
		})
	}()
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
	m.maven.offline = !m.maven.offline
	m.invalidate()
}

// ToggleMavenSkipTests flips whether the test phase runs with tests skipped.
func (m *Model) ToggleMavenSkipTests() {
	m.maven.skipTests = !m.maven.skipTests
	m.invalidate()
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

// CollapseMavenAll closes every node.
func (m *Model) CollapseMavenAll() {
	m.maven.expanded = map[string]bool{}
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
	command := "mvn"
	if mvnw.MatchString(p.Command) {
		command = p.Command
	}
	args := append([]string(nil), goals...)
	has := func(flag string) bool {
		for _, a := range args {
			if a == flag || strings.HasPrefix(a, flag+"=") {
				return true
			}
		}
		return false
	}
	if m.maven.offline && !has("-o") && !has("--offline") {
		args = append(args, "-o")
	}
	if m.maven.skipTests && !has("-DskipTests") {
		args = append(args, "-DskipTests")
	}
	if list := m.mavenProfilesList(); len(list) > 0 {
		args = append(args, "-P"+strings.Join(list, ","))
	}
	label := p.DisplayName
	if label == "" {
		label = p.ArtifactID
	}
	name := label + " [" + strings.Join(goals, ",") + "]"
	var cwd *string
	if p.Dir != "" {
		dir := p.Dir
		cwd = &dir
	}
	cfg := map[string]any{
		"id":       "maven:goal:" + p.Pom + ":" + strings.Join(goals, " "),
		"name":     name,
		"kind":     "maven",
		"command":  command,
		"args":     args,
		"cwd":      cwd,
		"env":      map[string]string{},
		"detected": false,
	}
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
	for i := range m.maven.model.Gradle {
		g := &m.maven.model.Gradle[i]
		label := g.Dir
		if label == "" {
			label = "Gradle build"
		}
		rows = append(rows, mavenRow{key: "gradle:" + g.Dir, kind: "gradle", label: label + " (Gradle)"})
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
		return mavenDependencyRows(r.key, r.project.Dependencies, r.depth+1)
	case "dependency":
		if r.dep == nil {
			return nil
		}
		return mavenDependencyRows(r.key, r.dep.Children, r.depth+1)
	case "runConfigs":
		var out []mavenRow
		for i := range m.run.configs {
			cfg := &m.run.configs[i]
			out = append(out, mavenRow{key: r.key + ":" + cfg.ID, kind: "runConfig", label: cfg.Name,
				depth: r.depth + 1, project: r.project, config: cfg})
		}
		return out
	}
	return nil
}

func mavenDependencyRows(parent string, deps []mavenDependency, depth int) []mavenRow {
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
		out = append(out, mavenRow{key: fmt.Sprintf("%s/%d", parent, i), kind: "dependency",
			label: label, detail: detail, depth: depth, dep: d, hasKids: len(d.Children) > 0})
	}
	return out
}

// --- panel ----------------------------------------------------------------

// mavenPanel draws the toolbar, the banner and the tree.
func (m *Model) mavenPanel(c *ui.Context, p *ui.Theme) {
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).Children(func() {
		m.mavenToolbar(c, p)
		m.mavenBanner(c, p)
		if m.maven.err != "" {
			ui.Text(c, m.maven.err).FontSize(11).TextColor(p.Danger).Padding(6, 10).
				SingleLine().Ellipsis("…")
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(4, 4).Children(func() {
				m.mavenTreeRows(c, p, m.mavenRootRows())
			})
		})
	})
}

// mavenToolbar is OpenIDEA's icon row: reload, generate sources, run, execute
// goal, offline, skip tests, expand and collapse.
func (m *Model) mavenToolbar(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Height(30).Padding(0, 6).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		if m.maven.loading || m.maven.syncing {
			m.mavenAction(c, p, "maven:reload", iconMavenStop, "Stop", func() {})
		} else {
			m.mavenAction(c, p, "maven:reload", iconMavenRefresh, "Reload Maven Projects", func() { m.loadBuildModel() })
		}
		m.mavenAction(c, p, "maven:sources", iconMavenSources, "Generate Sources", func() {
			if proj := m.mavenRootProject(); proj != nil {
				m.runMavenGoal(proj, []string{"generate-sources"})
			}
		})
		m.mavenActionDisabled(c, p, "maven:add", iconMavenPlus, "Add Maven Projects", true)
		m.mavenSeparator(c, p)
		runnable := m.maven.sel.kind == "phase" || m.maven.sel.kind == "goal" || m.maven.sel.kind == "runConfig"
		if runnable {
			m.mavenAction(c, p, "maven:run", iconMavenRun, "Run Maven Build", func() { m.RunMavenSelection() })
		} else {
			m.mavenActionDisabled(c, p, "maven:run", iconMavenRun, "Run Maven Build", false)
		}
		m.mavenAction(c, p, "maven:goal", iconMavenGoal, "Execute Maven Goal", func() {
			m.OpenRunAnything()
			if m.runAnything != nil {
				m.runAnything.query = "mvn "
			}
			m.invalidate()
		})
		m.mavenToggle(c, p, "maven:offline", iconMavenOffline, "Work Offline", m.maven.offline,
			func() { m.ToggleMavenOffline() })
		m.mavenActionDisabled(c, p, "maven:analyze", iconMavenAnalyze, "Analyze Dependencies", true)
		m.mavenToggle(c, p, "maven:skipTests", iconMavenSkip, "Skip Tests", m.maven.skipTests,
			func() { m.ToggleMavenSkipTests() })
		m.mavenSeparator(c, p)
		m.mavenAction(c, p, "maven:expand", iconMavenExpand, "Expand All", func() { m.ExpandMavenAll() })
		m.mavenAction(c, p, "maven:collapse", iconMavenCollapse, "Collapse All", func() { m.CollapseMavenAll() })
		m.mavenActionDisabled(c, p, "maven:settings", iconWelcomeGear, "Maven Settings…", true)
	})
}

func (m *Model) mavenSeparator(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Width(1).Height(16).Shrink(0).Background(p.Border)
}

func (m *Model) mavenAction(c *ui.Context, p *ui.Theme, key string, icon *ui.SVG, tip string, run func()) {
	b := ui.ButtonBase(c.Key(key)).Size(22, 22).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(p.SurfaceHover)
	}
	b.Tooltip(tip)
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(p.TextMuted) })
	if b.Clicked() {
		run()
	}
}

// mavenActionDisabled draws an action this build does not carry yet: it stays
// visible for the toolbar's shape but does nothing, as OpenIDEA disables it.
func (m *Model) mavenActionDisabled(c *ui.Context, p *ui.Theme, key string, icon *ui.SVG, tip string, always bool) {
	_ = always
	b := ui.ButtonBase(c.Key(key)).Size(22, 22).Radius(4).Justify(ui.Center).AlignItems(ui.Center)
	b.Tooltip(tip)
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(p.TextMuted.Alpha(80)) })
}

func (m *Model) mavenToggle(c *ui.Context, p *ui.Theme, key string, icon *ui.SVG, tip string, on bool, run func()) {
	b := ui.ButtonBase(c.Key(key)).Size(22, 22).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if on {
		b.Background(p.SurfaceHover)
	} else if b.Hovered() {
		b.Background(p.SurfaceHover)
	}
	b.Tooltip(tip)
	tone := p.TextMuted
	if on {
		tone = p.Accent
	}
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(tone) })
	if b.Clicked() {
		run()
	}
}

// mavenBanner shows the sync's progress, the changed-build-files notice with
// Load Maven Changes, or the last sync error.
func (m *Model) mavenBanner(c *ui.Context, p *ui.Theme) {
	mm := m.maven.model.Maven
	switch {
	case m.maven.syncing:
		ui.Row(c).Height(30).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.maven.syncMsg).FontSize(11).TextColor(p.TextMuted).SingleLine().Grow(1).Ellipsis("…")
			if f := m.maven.syncFrac; f != nil {
				ui.Text(c, fmt.Sprintf("%d%%", int(*f*100))).FontSize(10.5).TextColor(p.TextMuted)
			}
		})
	case mm != nil && mm.Stale:
		ui.Row(c).Height(34).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconMavenLoad).Size(16, 16).TextColor(p.TextMuted)
			ui.Text(c, "Build files changed since the last reload").FontSize(11).TextColor(p.Text).
				SingleLine().Grow(1).Ellipsis("…")
			b := ui.ButtonBase(c.Key("maven:load")).Height(22).Padding(0, 8).Radius(4).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(p.SurfaceHover)
			}
			b.Children(func() {
				ui.Text(c, "Load Maven Changes").FontSize(11).TextColor(p.Accent).SingleLine()
			})
			b.OnClick(func() { m.SyncMaven() })
		})
	case mm != nil && mm.SyncErr != nil && *mm.SyncErr != "":
		ui.Row(c).Height(30).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, strings.SplitN(*mm.SyncErr, "\n", 2)[0]).FontSize(11).TextColor(p.Danger).
				SingleLine().Ellipsis("…")
		})
	}
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
	row := ui.Row(c.Key(r.key)).
		Height(22).PaddingStart(6 + float32(r.depth)*14).PaddingEnd(6).Gap(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if m.maven.selKey == r.key {
		row.Background(p.SurfaceHover)
	} else if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	row.Children(func() {
		if r.kind == "profile" {
			on := r.checked
			if ui.Checkbox(c, &on, r.label).Changed() && r.profile != nil {
				m.SetMavenProfile(*r.profile, on)
			}
			return
		}
		if r.hasKids {
			glyph := "▸"
			if m.maven.expanded[r.key] {
				glyph = "▾"
			}
			ui.Text(c, glyph).FontSize(9.5).TextColor(p.TextMuted)
		} else {
			ui.Row(c).Width(10)
		}
		if ic := mavenRowIcon(r.kind); ic != nil {
			ui.Icon(c, ic).Size(14, 14).TextColor(mavenRowIconTone(r.kind, p))
		}
		label := r.label
		tone := p.Text
		if r.kind == "phase" && m.maven.skipTests && label == "test" {
			tone = p.TextMuted
		}
		ui.Text(c, label).FontSize(11.5).TextColor(tone).SingleLine().Ellipsis("…")
		if r.detail != "" {
			ui.Text(c, r.detail).FontSize(10).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
	})
	if r.hasKids {
		row.OnClick(func() {
			if m.maven.expanded == nil {
				m.maven.expanded = map[string]bool{}
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
		if cfg.Kind != "maven" {
			continue
		}
		cwd := ""
		if cfg.Cwd != nil {
			cwd = strings.TrimPrefix(*cfg.Cwd, "./")
		}
		if strings.Trim(cwd, "/") == strings.Trim(p.Dir, "/") || (cwd == "" && p.Dir == "") {
			out = append(out, cfg)
		}
	}
	return out
}

func (m *Model) mavenRootProject() *mavenProject {
	for i := range m.maven.model.MavenProjects() {
		p := &m.maven.model.MavenProjects()[i]
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

func mavenRowIcon(kind string) *ui.SVG {
	switch kind {
	case "project", "module":
		return iconMavenProject
	case "lifecycle", "phase":
		return iconMavenRun
	case "plugins", "plugin", "goal":
		return iconWelcomePlugin
	case "profiles", "profile":
		return iconStructure
	case "dependencies", "dependency":
		return iconMavenDependency
	case "runConfigs", "runConfig":
		return iconMavenRun
	}
	return nil
}

func mavenRowIconTone(kind string, p *ui.Theme) ui.Color {
	switch kind {
	case "project":
		return p.Accent
	case "goal", "phase":
		return p.TextMuted
	}
	return p.TextMuted
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
