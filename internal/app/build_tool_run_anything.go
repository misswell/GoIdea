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
	tool := buildToolForLine(line)
	args := buildToolLineArgs(line)
	if tool == "" || len(args) == 0 {
		return false
	}
	switch tool {
	case "maven":
		project := m.maven.sel.project
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
		build, project := m.currentGradleProject()
		if build == nil || project == nil {
			return false
		}
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
	return false
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
	query = strings.TrimSpace(query)
	tool := buildToolForLine(query)
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
		if project := m.mavenRootProject(); project != nil {
			for _, plugin := range project.Plugins {
				for _, goal := range plugin.Goals {
					label := plugin.Prefix + ":" + goal
					candidates = append(candidates, completion{label: label, detail: plugin.ArtifactID})
				}
			}
		}
	case "gradle":
		if build, _ := m.currentGradleProject(); build != nil {
			for _, project := range build.Projects {
				for _, task := range project.Tasks {
					label := task.Name
					if project.Path != "" && project.Path != ":" {
						label = project.Path + ":" + task.Name
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
