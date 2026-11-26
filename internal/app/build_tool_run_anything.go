package app

// Run Anything's build-tool provider. A line beginning with mvn/mvnw or
// gradle/gradlew starts the same typed configuration as OpenIDEA, so the
// backend still applies the build-tool launcher, environment and test
// reporting instead of treating the line as a generic shell command.

import (
	"encoding/json"
	"sort"
	"strings"

	"mayoi/internal/adapter"
)

func buildToolForLine(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return ""
	}
	switch strings.ToLower(fields[0]) {
	case "mvn", "mvn.cmd", "./mvn", "./mvn.cmd",
		"mvnw", "mvnw.cmd", "./mvnw", "./mvnw.cmd":
		return "maven"
	case "gradle", "gradle.bat", "./gradle", "./gradle.bat",
		"gradlew", "gradlew.bat", "./gradlew", "./gradlew.bat":
		return "gradle"
	}
	return ""
}

func buildToolLineArgs(line string) []string {
	line = strings.TrimSpace(line)
	if at := strings.IndexAny(line, " \t\r\n"); at >= 0 {
		line = strings.TrimSpace(line[at:])
	} else {
		return nil
	}
	return adapter.ParseArgs(line)
}

func (m *Model) executeBuildToolLine(line string) bool {
	return m.executeBuildToolLineFor(line, "")
}

func (m *Model) executeBuildToolLineFor(line, selectedProject string) bool {
	tool := buildToolForLine(line)
	args := buildToolLineArgs(line)
	if tool == "" || len(args) == 0 {
		return false
	}
	switch tool {
	case "maven":
		project := m.mavenProjectByPom(selectedProject)
		if project == nil {
			project = m.maven.sel.project
		}
		if project == nil {
			project = m.mavenRootProject()
		}
		if project == nil {
			return false
		}
		config, name := m.mavenGoalRunConfig(project, args)
		raw, err := json.Marshal(config)
		if err != nil {
			m.setBuildToolActionError("Could not start the Maven goal", err)
			return true
		}
		m.maven.activeTool = "maven"
		m.status = "Running " + strings.Join(args, " ")
		if m.startRunWith(raw, name) {
			m.adoptRunTemporary(config)
		}
		return true
	case "gradle":
		if selectedProject != "" {
			if build, project := gradleProjectByKey(m.maven.model.Gradle, selectedProject); build != nil && project != nil {
				return m.executeGradleBuildToolLine(build, project, args)
			}
		}
		build, project := m.currentGradleProject()
		if build == nil || project == nil {
			return false
		}
		return m.executeGradleBuildToolLine(build, project, args)
	}
	return false
}

func (m *Model) executeGradleBuildToolLine(build *gradleModel, project *gradleProject, args []string) bool {
	config := gradleTaskConfig(
		build,
		project,
		args,
		m.maven.gradleOffline,
		m.maven.gradleSkipTests,
	)
	raw, err := json.Marshal(config)
	if err != nil {
		m.setBuildToolActionError("Could not start the Gradle task", err)
		return true
	}
	m.maven.activeTool = "gradle"
	m.startGradleConfig(raw, config.Name)
	m.adoptRunTemporary(config)
	return true
}

type buildToolProjectChoice struct {
	value  string
	label  string
	detail string
}

func (m *Model) buildToolRunProjects(tool string) []buildToolProjectChoice {
	var projects []buildToolProjectChoice
	switch tool {
	case "maven":
		for _, project := range m.maven.model.MavenProjects() {
			label := mavenProjectName(project)
			if label == "" {
				label = project.Pom
			}
			detail := project.Dir
			if detail == "" {
				detail = "."
			}
			projects = append(projects, buildToolProjectChoice{
				value:  project.Pom,
				label:  label + " — " + detail,
				detail: detail,
			})
		}
	case "gradle":
		for i := range m.maven.model.Gradle {
			build := &m.maven.model.Gradle[i]
			for j := range build.Projects {
				project := &build.Projects[j]
				label := project.Name
				if label == "" {
					label = project.Path
				}
				detail := strings.TrimSpace((build.Dir + " " + project.Path))
				if detail == "" {
					detail = "."
				}
				projects = append(projects, buildToolProjectChoice{
					value:  gradleProjectKey(build, project),
					label:  label + " — " + detail,
					detail: detail,
				})
			}
		}
	}
	return projects
}

func (m *Model) defaultBuildToolRunProject(tool string) string {
	switch tool {
	case "maven":
		if project := m.maven.sel.project; project != nil {
			return project.Pom
		}
		if project := m.mavenRootProject(); project != nil {
			return project.Pom
		}
	case "gradle":
		if m.maven.gradleSelKey != "" {
			return m.maven.gradleSelKey
		}
		if build, project := m.currentGradleProject(); build != nil && project != nil {
			return gradleProjectKey(build, project)
		}
	}
	return ""
}

func (m *Model) currentGradleProject() (*gradleModel, *gradleProject) {
	if m.maven.gradleSel.build != nil && m.maven.gradleSel.project != nil {
		for i := range m.maven.model.Gradle {
			build := &m.maven.model.Gradle[i]
			if build.Dir != m.maven.gradleSel.build.Dir {
				continue
			}
			for j := range build.Projects {
				if build.Projects[j].Path == m.maven.gradleSel.project.Path {
					return build, &build.Projects[j]
				}
			}
		}
	}
	for i := range m.maven.model.Gradle {
		build := &m.maven.model.Gradle[i]
		if project := gradleRootProject(build); project != nil {
			return build, project
		}
	}
	return nil, nil
}

var mavenCompletionPhases = []string{
	"pre-clean", "clean", "post-clean",
	"validate", "initialize", "generate-sources", "process-sources",
	"generate-resources", "process-resources", "compile", "process-classes",
	"generate-test-sources", "process-test-sources", "generate-test-resources",
	"process-test-resources", "test-compile", "process-test-classes", "test",
	"prepare-package", "package", "pre-integration-test", "integration-test",
	"post-integration-test", "verify", "install", "deploy",
	"pre-site", "site", "post-site", "site-deploy",
}

// buildToolCompletionRows supplies OpenIDEA's Build Tools provider in Run
// Anything. It completes the current word while preserving the command line
// before it and caps the provider at the popup's group limit.
func (m *Model) buildToolCompletionRows(query string) []runAnythingRow {
	tool := buildToolForLine(query)
	project := ""
	if tool != "" {
		project = m.defaultBuildToolRunProject(tool)
	}
	return m.buildToolCompletionRowsFor(query, tool, project)
}

func (m *Model) buildToolCompletionRowsFor(query, fixedTool, projectKey string) []runAnythingRow {
	query = strings.TrimSpace(query)
	tool := buildToolForLine(query)
	if fixedTool != "" {
		tool = fixedTool
	}
	if tool == "" {
		return nil
	}
	firstEnd := strings.IndexAny(query, " \t")
	if firstEnd < 0 {
		firstEnd = len(query)
	}
	head := query[:firstEnd]
	rest := strings.TrimLeft(query[firstEnd:], " \t")
	wordAt := strings.LastIndexAny(rest, " \t") + 1
	prefix := rest[wordAt:]
	if strings.HasPrefix(prefix, "-") {
		return nil
	}
	base := head
	if rest != "" {
		base += " " + rest[:wordAt]
	}
	if wordAt == 0 && rest != "" && base == head {
		base += " "
	}
	if base != "" && !strings.HasSuffix(base, " ") {
		base += " "
	}
	typed := map[string]bool{}
	for _, arg := range buildToolLineArgs(head + " " + rest[:wordAt]) {
		typed[arg] = true
	}
	type completion struct {
		label  string
		detail string
	}
	var candidates []completion
	switch tool {
	case "maven":
		for _, phase := range mavenCompletionPhases {
			candidates = append(candidates, completion{label: phase, detail: "phase"})
		}
		project := m.mavenProjectByPom(projectKey)
		if project == nil {
			project = m.mavenRootProject()
		}
		if project != nil {
			for _, plugin := range project.Plugins {
				for _, goal := range plugin.Goals {
					label := plugin.Prefix + ":" + goal
					candidates = append(candidates, completion{label: label, detail: plugin.ArtifactID})
				}
			}
		}
	case "gradle":
		build, _ := gradleProjectByKey(m.maven.model.Gradle, projectKey)
		if build == nil {
			build, _ = m.currentGradleProject()
		}
		if build != nil {
			for _, project := range build.Projects {
				for _, task := range project.Tasks {
					label := task.Name
					if project.Path != "" && project.Path != ":" {
						label = project.Path + ":" + label
					}
					detail := task.Group
					if detail == "" {
						detail = "other"
					}
					candidates = append(candidates, completion{label: label, detail: detail})
				}
			}
		}
	}
	lowerPrefix := strings.ToLower(prefix)
	seen := map[string]bool{}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if typed[candidate.label] || seen[candidate.label] ||
			(lowerPrefix != "" && !strings.HasPrefix(strings.ToLower(candidate.label), lowerPrefix)) {
			continue
		}
		seen[candidate.label] = true
		filtered = append(filtered, candidate)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if len(filtered[i].label) != len(filtered[j].label) {
			return len(filtered[i].label) < len(filtered[j].label)
		}
		return filtered[i].label < filtered[j].label
	})
	section := "Maven Goals"
	if tool == "gradle" {
		section = "Gradle Tasks"
	}
	rows := []runAnythingRow{{section: section}}
	for _, candidate := range filtered {
		if len(rows)-1 == runAnythingGroupLimit {
			break
		}
		line := base + candidate.label
		rows = append(rows, runAnythingRow{
			isItem: true, label: line, detail: candidate.detail,
			line: line, config: -1,
		})
	}
	if len(rows) == 1 {
		return nil
	}
	return rows
}

func gradleProjectByKey(builds []gradleModel, key string) (*gradleModel, *gradleProject) {
	for i := range builds {
		build := &builds[i]
		for j := range build.Projects {
			project := &build.Projects[j]
			if gradleProjectKey(build, project) == key {
				return build, project
			}
		}
	}
	return nil, nil
}
