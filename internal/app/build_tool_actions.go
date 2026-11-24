package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// OpenIDEA keeps project linkage and Execute Goal/Task in build-tool actions
// rather than in the palette's static command list. These adapters expose the
// same ids and arguments over GoIDEA's native Maven/Gradle window.
func init() {
	for _, action := range buildToolCompatibilityActions() {
		_ = registerAction(action)
	}
}

func buildToolCompatibilityActions() []*Action {
	action := func(id, title string, run func(*Model, any)) *Action {
		return &Action{
			ID: id, Title: title, Category: "Build",
			Keywords:   "maven gradle build project task goal link unlink execute",
			RunWithArg: run,
		}
	}
	return []*Action{
		action("maven.addProjects", "Add Maven Projects", func(m *Model, _ any) {
			m.AddMavenProjects()
		}),
		action("maven.importExternal", "Import Module from Existing Sources", func(m *Model, _ any) {
			m.ImportMavenModule()
		}),
		action("maven.unlinkProjects", "Unlink Maven Projects", func(m *Model, arg any) {
			m.UnlinkMavenProjects(buildToolArgString(arg, "pom"))
		}),
		action("maven.executeGoal", "Execute Maven Goal…", func(m *Model, _ any) {
			m.OpenBuildToolExecute("maven")
		}),
		action("maven.generateSources", "Generate Sources and Update Folders", func(m *Model, arg any) {
			m.GenerateMavenSources(buildToolArgString(arg, "pom"))
		}),
		action("gradle.linkProjects", "Link Gradle Projects", func(m *Model, _ any) {
			m.LinkGradleProjects()
		}),
		action("gradle.unlinkProjects", "Unlink Gradle Project", func(m *Model, arg any) {
			m.UnlinkGradleProject(buildToolArgString(arg, "buildDir"))
		}),
		action("gradle.executeTask", "Execute Gradle Task…", func(m *Model, arg any) {
			m.selectGradleProjectKey(buildToolArgString(arg, "projectKey"))
			m.OpenBuildToolExecute("gradle")
		}),
	}
}

func (m *Model) selectGradleProjectKey(key string) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		for j := range build.Projects {
			project := &build.Projects[j]
			if gradleProjectKey(build, project) != key {
				continue
			}
			m.maven.activeTool = "gradle"
			m.maven.gradleSel = gradleSelection{
				kind: "gradleProject", build: build, project: project,
			}
			m.maven.gradleSelKey = key
			return true
		}
	}
	return false
}

func buildToolArgString(arg any, key string) string {
	value, _ := buildToolArgValue(arg, key)
	return value
}

func buildToolArgValue(arg any, key string) (string, bool) {
	switch value := arg.(type) {
	case string:
		return strings.TrimSpace(value), true
	case map[string]any:
		text, ok := value[key].(string)
		return strings.TrimSpace(text), ok
	case []byte:
		var object map[string]any
		if json.Unmarshal(value, &object) == nil {
			text, ok := object[key].(string)
			return strings.TrimSpace(text), ok
		}
	case json.RawMessage:
		var object map[string]any
		if json.Unmarshal(value, &object) == nil {
			text, ok := object[key].(string)
			return strings.TrimSpace(text), ok
		}
	}
	return "", false
}

// OpenBuildToolExecute is Execute Maven Goal / Execute Gradle Task: the same
// Run Anything popup, prefilled with the selected build tool.
func (m *Model) OpenBuildToolExecute(tool string) {
	if strings.TrimSpace(tool) == "gradle" {
		m.ShowGradle()
	} else {
		m.ShowMaven()
	}
	m.OpenRunAnything()
	if m.runAnything == nil {
		return
	}
	command := strings.TrimSpace(tool)
	if command == "maven" {
		command = "mvn"
	}
	m.runAnything.query = command + " "
	m.refreshRunAnything()
	m.invalidate()
}

// GenerateMavenSources runs generate-sources for one POM when the action names
// it, or for every non-aggregated Maven project, then reloads on any success.
// This is upstream store.ts generateSources, including its sequential run
// behavior and cancellation semantics.
func (m *Model) GenerateMavenSources(pom string) {
	projects := make([]mavenProject, 0, len(m.maven.model.MavenProjects()))
	if strings.TrimSpace(pom) != "" {
		if project := m.mavenProjectByPom(strings.TrimSpace(pom)); project != nil {
			projects = append(projects, *project)
		}
	} else {
		for _, project := range m.maven.model.MavenProjects() {
			if !project.Aggregated {
				projects = append(projects, project)
			}
		}
	}
	if len(projects) == 0 {
		m.status = "No Maven project is available"
		m.invalidate()
		return
	}
	m.runMavenGenerateSources(projects, 0, false)
}

func (m *Model) runMavenGenerateSources(
	projects []mavenProject,
	next int,
	anySucceeded bool,
) {
	if next >= len(projects) {
		if anySucceeded {
			m.SyncMaven()
		}
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

	project := &projects[next]
	config, name := m.mavenGoalRunConfig(project, []string{"generate-sources"})
	raw, err := json.Marshal(config)
	if err != nil {
		m.status = "Could not start Maven Generate Sources: " + err.Error()
		m.invalidate()
		return
	}
	if len(projects) > 1 {
		m.status = fmt.Sprintf(
			"Running generate-sources for %s (%d/%d)",
			mavenProjectName(*project),
			next+1,
			len(projects),
		)
	} else {
		m.status = "Running generate-sources"
	}
	m.beginRunPlugin(config.ID, func(code *int32, stopped bool) {
		if stopped || code == nil {
			return
		}
		m.runMavenGenerateSources(projects, next+1, anySucceeded || *code == 0)
	})
	if !m.startRunWith(raw, name) {
		m.cancelRunPlugin()
		m.status = "Maven Generate Sources did not start"
		m.invalidate()
	}
}

func mavenProjectName(project mavenProject) string {
	if project.DisplayName != "" {
		return project.DisplayName
	}
	if project.ArtifactID != "" {
		return project.ArtifactID
	}
	return project.Pom
}

// AddMavenProjects links one or more pom.xml files. Paths inside the project
// are stored relative to its root; external POMs stay absolute.
func (m *Model) AddMavenProjects() {
	if m.Root() == "" || m.showingWelcome {
		return
	}
	parent := m.win
	start := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Add Maven Projects", ButtonLabel: "Add",
			DefaultPath: start, Multiple: true,
			Filters: []mygo.FileFilter{{Name: "Maven POM", Extensions: []string{"xml"}}},
		})
		if err != nil {
			m.setBuildToolActionError("Could not choose Maven projects", err)
			return
		}
		if len(paths) == 0 {
			return
		}
		m.post(func() { m.linkMavenProjects(paths, true) })
	}()
}

// ImportMavenModule is Project Structure's Import Module from Existing
// Sources: choose a directory (or pom.xml), then link its POM.
func (m *Model) ImportMavenModule() {
	if m.Root() == "" || m.showingWelcome {
		return
	}
	parent := m.win
	start := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Import Module from Existing Sources",
			ButtonLabel: "Import", DefaultPath: start, Directory: true,
		})
		if err != nil {
			m.setBuildToolActionError("Could not choose a module", err)
			return
		}
		if len(paths) == 0 {
			return
		}
		m.post(func() {
			pom, ok := mavenPOMForImport(paths[0])
			if !ok {
				m.status = "The selected folder does not contain pom.xml"
				m.invalidate()
				return
			}
			m.linkMavenProjects([]string{pom}, true)
		})
	}()
}

// UnlinkMavenProjects removes a module from the Maven model without touching
// its files. The tree calls this with {pom}; the palette uses the selection.
func (m *Model) UnlinkMavenProjects(pom string) {
	if pom == "" && m.maven.sel.project != nil {
		pom = m.maven.sel.project.Pom
	}
	if pom == "" {
		if project := m.mavenRootProject(); project != nil {
			pom = project.Pom
		}
	}
	if pom == "" {
		return
	}
	m.linkMavenProjects([]string{pom}, false)
}

func (m *Model) linkMavenProjects(paths []string, linked bool) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "settings_get", nil)
		if err != nil {
			m.setBuildToolActionError("Could not read Maven settings", err)
			return
		}
		var settings backendBuildToolSettings
		if err := json.Unmarshal(raw, &settings); err != nil {
			m.setBuildToolActionError("Could not read Maven settings", err)
			return
		}
		poms, rejected := classifyMavenPOMs(root, paths)
		warning := ""
		if len(rejected) > 0 && len(poms) > 0 {
			warning = fmt.Sprintf(
				"%d selected file(s) were not pom.xml or were not absolute",
				len(rejected),
			)
		}
		if len(poms) == 0 {
			m.status = "No pom.xml files were selected"
			m.invalidate()
			return
		}
		nextUnlinked, nextLinked := nextMavenLinkSettings(
			settings.Languages.Java.Maven.UnlinkedProjects,
			settings.Languages.Java.Maven.LinkedProjects,
			m.maven.model.MavenProjects(),
			poms,
			linked,
		)
		if !slices.Equal(settings.Languages.Java.Maven.UnlinkedProjects, nextUnlinked) {
			if err := m.setProjectSetting(ctx, "languages.java.maven.unlinkedProjects", nextUnlinked); err != nil {
				m.setBuildToolActionError("Could not unlink the Maven project", err)
				return
			}
		}
		if !slices.Equal(settings.Languages.Java.Maven.LinkedProjects, nextLinked) {
			if err := m.setProjectSetting(ctx, "languages.java.maven.linkedProjects", nextLinked); err != nil {
				m.setBuildToolActionError("Could not link the Maven project", err)
				return
			}
		}
		m.post(func() {
			if warning != "" {
				m.status = warning
			}
			m.maven.loading = false
			m.loadBuildModel()
		})
	}()
}

type backendBuildToolSettings struct {
	Languages struct {
		Java struct {
			Maven struct {
				UnlinkedProjects []string `json:"unlinkedProjects"`
				LinkedProjects   []string `json:"linkedProjects"`
			} `json:"maven"`
			Gradle struct {
				LinkedProjects []string `json:"linkedProjects"`
			} `json:"gradle"`
		} `json:"java"`
	} `json:"languages"`
}

func (m *Model) setProjectSetting(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = m.invoke(ctx, "settings_set", map[string]any{
		"scope": "project", "key": key, "value": string(raw),
	})
	return err
}

func nextMavenLinkSettings(
	unlinkedBefore, linkedBefore []string,
	projects []mavenProject,
	poms []string,
	linked bool,
) ([]string, []string) {
	unlinked := append([]string(nil), unlinkedBefore...)
	extra := append([]string(nil), linkedBefore...)
	known := make(map[string]bool, len(projects))
	for _, project := range projects {
		known[project.Pom] = true
	}
	for _, pom := range poms {
		if linked {
			if containsString(unlinked, pom) {
				unlinked = removeString(unlinked, pom)
			}
			if !known[pom] && !containsString(extra, pom) {
				extra = append(extra, pom)
			}
			continue
		}
		if containsString(extra, pom) {
			extra = removeString(extra, pom)
		} else if !containsString(unlinked, pom) {
			unlinked = append(unlinked, pom)
		}
	}
	return unlinked, extra
}

func classifyMavenPOMs(root string, picked []string) (link, rejected []string) {
	base, err := filepath.Abs(root)
	if err != nil {
		return nil, append([]string(nil), picked...)
	}
	base = filepath.Clean(base)
	seen := map[string]bool{}
	for _, raw := range picked {
		path, err := filepath.Abs(raw)
		if err != nil || !samePOMName(filepath.Base(path)) {
			rejected = append(rejected, raw)
			continue
		}
		path = filepath.Clean(path)
		stored := filepath.ToSlash(path)
		if rel, err := filepath.Rel(base, path); err == nil &&
			rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			stored = filepath.ToSlash(rel)
		} else if runtime.GOOS == "windows" {
			stored = strings.TrimPrefix(filepath.Clean(path), `\\?\`)
		}
		key := stored
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			link = append(link, stored)
		}
	}
	return link, rejected
}

func mavenPOMForImport(path string) (string, bool) {
	path = filepath.Clean(path)
	if samePOMName(filepath.Base(path)) {
		info, err := os.Stat(path)
		return path, err == nil && !info.IsDir()
	}
	pom := filepath.Join(path, "pom.xml")
	info, err := os.Stat(pom)
	return pom, err == nil && !info.IsDir()
}

func samePOMName(name string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(name, "pom.xml")
	}
	return name == "pom.xml"
}

// LinkGradleProjects links independent build directories through the backend
// operation OpenIDEA uses, then draws the returned model.
func (m *Model) LinkGradleProjects() {
	if m.Root() == "" || m.showingWelcome {
		return
	}
	parent, start := m.win, m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Link Gradle Projects", ButtonLabel: "Link",
			DefaultPath: start, Multiple: true,
			Filters: []mygo.FileFilter{{
				Name: "Gradle build files", Extensions: []string{"gradle", "kts"},
			}},
		})
		if err != nil {
			m.setBuildToolActionError("Could not choose Gradle projects", err)
			return
		}
		if len(paths) > 0 {
			m.post(func() { m.updateGradleLinks(paths, true) })
		}
	}()
}

func (m *Model) UnlinkGradleProject(buildDir string) {
	if buildDir == "" {
		for _, build := range m.maven.model.Gradle {
			if build.Dir != "" && strings.Contains(m.maven.selKey, build.Dir) {
				buildDir = build.Dir
				break
			}
		}
	}
	if buildDir == "" && len(m.maven.model.Gradle) == 1 {
		buildDir = m.maven.model.Gradle[0].Dir
	}
	if buildDir != "" {
		m.updateGradleLinks([]string{buildDir}, false)
	}
}

func (m *Model) updateGradleLinks(paths []string, linked bool) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) || len(paths) == 0 {
		return
	}
	command := "gradle_unlink_projects"
	if linked {
		command = "gradle_link_projects"
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		raw, err := m.invoke(ctx, command, map[string]any{"paths": paths})
		if err != nil {
			m.setBuildToolActionError("Could not update Gradle projects", err)
			return
		}
		var model buildModel
		if err := json.Unmarshal(raw, &model); err != nil {
			m.setBuildToolActionError("Could not read Gradle projects", err)
			return
		}
		m.post(func() {
			m.maven.model = model
			m.mavenInitTree()
			m.invalidate()
		})
	}()
}

func (m *Model) setBuildToolActionError(action string, err error) {
	m.post(func() {
		m.status = action + ": " + err.Error()
		m.invalidate()
	})
}

func removeString(items []string, unwanted string) []string {
	out := items[:0]
	for _, item := range items {
		if item != unwanted {
			out = append(out, item)
		}
	}
	return out
}
