package app

import (
	"fmt"
	"strings"

	"mayoi/internal/openidea"
)

func (m *Model) showMavenDependencyDocumentation(
	project *mavenProject,
	dependency mavenDependency,
	direct *mavenDependency,
	isDirect bool,
) {
	lines := mavenDependencyDocumentationLines(project, dependency, direct, isDirect)
	m.showDependencyDocumentation(dependencyDocumentationTitle(dependency), lines...)
}

func mavenDependencyDocumentationLines(
	project *mavenProject,
	dependency mavenDependency,
	direct *mavenDependency,
	isDirect bool,
) []string {
	var lines []string
	if project != nil {
		source := strings.TrimSpace(project.Pom)
		if source != "" {
			lines = append(lines, "**Declared in** `"+source+"`")
		}
		name := strings.TrimSpace(project.DisplayName)
		if name == "" {
			name = strings.TrimSpace(project.ArtifactID)
		}
		if name != "" {
			lines = append(lines, "**Module** "+name)
		}
	}
	if direct != nil && !isDirect && mavenDependencyCoordinates(*direct) != "" {
		lines = append(lines, "**Direct dependency** `"+mavenDependencyCoordinates(*direct)+"`")
	}
	lines = append(lines, dependencyDocumentationLines(dependency)...)
	return lines
}

func (m *Model) showAnalyzerDependencyDocumentation(usage dependencyUsage) {
	module, moduleOK := m.analyzerCurrentModule()
	lines := analyzerDependencyDocumentationLines(module, moduleOK, usage)
	m.showDependencyDocumentation(dependencyDocumentationTitle(usage.dependency), lines...)
}

func analyzerDependencyDocumentationLines(
	module analyzerModule,
	moduleOK bool,
	usage dependencyUsage,
) []string {
	lines := dependencyDocumentationLines(usage.dependency)
	if moduleOK {
		if module.label != "" {
			lines = append(lines, "**Usage location** `"+module.label+"`")
		}
		switch module.tool {
		case "maven":
			if module.pom != "" {
				lines = append(lines, "**Source** `"+module.pom+"`")
			}
		case "gradle":
			if module.buildDir != "" {
				lines = append(lines, "**Build directory** `"+module.buildDir+"`")
			}
			if module.path != "" {
				lines = append(lines, "**Gradle path** `"+module.path+"`")
			}
		}
	}
	if len(usage.path) > 0 {
		path := make([]string, 0, len(usage.path))
		for index, dependency := range usage.path {
			coordinates := mavenDependencyCoordinates(dependency)
			if coordinates == "" {
				coordinates = dependency.ArtifactID
			}
			path = append(path, fmt.Sprintf("%d. `%s`", index+1, coordinates))
		}
		lines = append(lines, "**Resolution path**\n"+strings.Join(path, "\n"))
	}
	return lines
}

func dependencyDocumentationTitle(dependency mavenDependency) string {
	if coordinates := mavenDependencyCoordinates(dependency); coordinates != "" {
		return coordinates
	}
	if dependency.ArtifactID != "" {
		return dependency.ArtifactID
	}
	return "Dependency"
}

func dependencyDocumentationLines(dependency mavenDependency) []string {
	var lines []string
	coordinates := mavenDependencyCoordinates(dependency)
	if coordinates != "" {
		lines = append(lines, "**Coordinates** `"+coordinates+"`")
	}
	if dependency.Version != "" {
		lines = append(lines, "**Version** `"+dependency.Version+"`")
	}
	if dependency.Scope != "" {
		lines = append(lines, "**Scope** `"+dependency.Scope+"`")
	}
	if dependency.Type != "" {
		lines = append(lines, "**Type** `"+dependency.Type+"`")
	}
	if dependency.Classifier != nil && strings.TrimSpace(*dependency.Classifier) != "" {
		lines = append(lines, "**Classifier** `"+strings.TrimSpace(*dependency.Classifier)+"`")
	}
	if dependency.Optional {
		lines = append(lines, "**Optional** yes")
	}
	lines = append(lines, "**Resolution** "+dependencyUsageStatus(dependency))
	return lines
}

func (m *Model) showDependencyDocumentation(title string, lines ...string) {
	var text strings.Builder
	text.WriteString("### " + title)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		text.WriteString("\n\n")
		text.WriteString(line)
	}
	if m.documentation.popup != nil {
		m.closeDocumentationPopup()
	}
	entry := documentationEntry{blocks: documentationBlocks(openidea.Markup{Kind: "markdown", Value: text.String()})}
	m.documentation.window.push(entry)
	m.DockShow(documentationWindow)
	m.invalidate()
}
