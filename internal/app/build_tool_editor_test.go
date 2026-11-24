package app

import "testing"

func TestBuildToolEditorActionsUseTheOpenModule(t *testing.T) {
	buildFile := "build.gradle.kts"
	model := &Model{maven: mavenPane{model: buildModel{
		Maven: &mavenModel{Projects: []mavenProject{{
			Pom: "/p/app/pom.xml",
		}}},
		Gradle: []gradleModel{{
			Dir: "",
			Projects: []gradleProject{{
				Path:      ":app",
				Dir:       "app",
				BuildFile: &buildFile,
			}},
		}},
	}}}
	model.setRoot("/p")

	action, ok := model.buildToolEditorAction("/p/app/pom.xml")
	if !ok || action.ID != "maven.analyzeDependencies" {
		t.Fatalf("Maven editor action = %+v, %v", action, ok)
	}
	arg, ok := action.Arg.(map[string]any)
	if !ok || arg["pom"] != "/p/app/pom.xml" {
		t.Fatalf("Maven action argument = %#v", action.Arg)
	}

	action, ok = model.buildToolEditorAction("/p/app/build.gradle.kts")
	if !ok || action.ID != "gradle.analyzeDependencies" {
		t.Fatalf("Gradle editor action = %+v, %v", action, ok)
	}
	arg, ok = action.Arg.(map[string]any)
	if !ok || arg["path"] != "/p/app/build.gradle.kts" {
		t.Fatalf("Gradle action argument = %#v", action.Arg)
	}

	if _, ok := model.buildToolEditorAction("/p/README.md"); ok {
		t.Fatal("an unrelated editor contributed a build-tool action")
	}
}

func TestBuildToolEditorActionsStayWithKnownProjects(t *testing.T) {
	model := &Model{}
	if _, ok := model.buildToolEditorAction("/p/pom.xml"); ok {
		t.Fatal("an unloaded Maven project contributed Analyze Dependencies")
	}
	if _, ok := model.buildToolEditorAction("/p/build.gradle"); ok {
		t.Fatal("an unloaded Gradle project contributed Analyze Dependencies")
	}
}
