package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDependencyDocumentationShowsCoordinatesAndResolution(t *testing.T) {
	classifier := "sources"
	managed := "2.0"
	dependency := mavenDependency{
		GroupID: "org.example", ArtifactID: "library", Version: "1.4",
		Scope: "runtime", Type: "jar", Classifier: &classifier, Managed: &managed,
	}
	project := &mavenProject{Pom: "/project/pom.xml", DisplayName: "Application"}
	lines := mavenDependencyDocumentationLines(project, dependency, nil, true)
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"/project/pom.xml", "Application", "org.example:library:sources:1.4",
		"`1.4`", "`runtime`", "`jar`", "`sources`", "version managed from 2.0",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dependency documentation misses %q:\n%s", want, text)
		}
	}
}

func TestAnalyzerDependencyDocumentationShowsUsagePath(t *testing.T) {
	direct := mavenDependency{GroupID: "org.example", ArtifactID: "direct", Version: "1.0"}
	transitive := mavenDependency{GroupID: "org.example", ArtifactID: "leaf", Version: "2.0"}
	module := analyzerModule{
		tool: "maven", label: "Application", pom: "/project/pom.xml",
	}
	lines := analyzerDependencyDocumentationLines(module, true, dependencyUsage{
		path: []mavenDependency{direct, transitive}, dependency: transitive,
	})
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"Application", "/project/pom.xml", "Resolution path",
		"org.example:direct:1.0", "org.example:leaf:2.0",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("analyzer documentation misses %q:\n%s", want, text)
		}
	}
}

func TestDependencyContextMenusExposeQuickDocumentation(t *testing.T) {
	m := &Model{}
	project := &mavenProject{Pom: "pom.xml"}
	dependency := mavenDependency{GroupID: "org.example", ArtifactID: "library", Version: "1.0"}
	labels := buildToolMenuLabels(t, func(menu *ui.Menu) {
		m.mavenRowContextMenu(menu, mavenRow{
			kind: "dependency", project: project, dep: &dependency, direct: &dependency,
		})
	})
	assertMenuLabels(t, labels, "Quick Documentation", "Jump to Source", "Copy Coordinates")

	labels = buildToolMenuLabels(t, func(menu *ui.Menu) {
		m.analyzerUsageMenu(menu, dependencyUsage{dependency: dependency})
	})
	assertMenuLabels(t, labels, "Quick Documentation", "Copy Coordinates")
}
