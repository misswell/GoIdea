package app

// Build-tool editor context actions. OpenIDEA contributes Maven and Gradle
// dependency analysis from a POM or Gradle build script, passing that file as
// the action argument so the analyzer opens the matching module.

import "github.com/egoist/mygo/ui"

type buildToolEditorAction struct {
	ID  string
	Arg any
}

func (m *Model) buildToolEditorAction(path string) (buildToolEditorAction, bool) {
	if isPomPath(path) {
		project := m.mavenProjectByPom(path)
		if project == nil {
			return buildToolEditorAction{}, false
		}
		return buildToolEditorAction{
			ID:  "maven.analyzeDependencies",
			Arg: map[string]any{"pom": project.Pom},
		}, true
	}
	if !isGradleBuildScriptPath(path) {
		return buildToolEditorAction{}, false
	}
	project, _ := m.gradleProjectForBuildFile(path)
	if project == nil {
		return buildToolEditorAction{}, false
	}
	return buildToolEditorAction{
		ID:  "gradle.analyzeDependencies",
		Arg: map[string]any{"path": path},
	}, true
}

func (m *Model) buildToolEditorMenu(menu *ui.Menu, t *Tab) bool {
	if t == nil || t.doc == nil {
		return false
	}
	action, ok := m.buildToolEditorAction(t.doc.Path())
	if !ok {
		return false
	}
	menu.Separator()
	if menu.Item("Analyze Dependencies…").Chosen() {
		m.RunActionWithArg(action.ID, action.Arg)
	}
	return true
}
