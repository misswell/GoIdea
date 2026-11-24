package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestGradleBuildModelDecodesProjectsAndTasks(t *testing.T) {
	raw := []byte(`{
		"maven": null,
		"gradle": [{
			"dir": "",
			"command": "/work/demo/gradlew",
			"javaVersion": 21,
			"stale": true,
			"syncError": "JAVA_HOME is missing",
			"projects": [{
				"path": ":",
				"name": "demo",
				"dir": "",
				"buildFile": "build.gradle.kts",
				"tasks": [
					{"name": "test", "group": "verification", "description": "Runs tests"},
					{"name": "assemble", "group": "build", "description": "Builds archives"}
				]
			}, {
				"path": ":app",
				"name": "app",
				"dir": "app",
				"tasks": [{"name": "jar", "group": "build", "description": null}]
			}]
		}]
	}`)
	var model buildModel
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Gradle) != 1 {
		t.Fatalf("Gradle models = %d", len(model.Gradle))
	}
	build := model.Gradle[0]
	if build.Command != "/work/demo/gradlew" || !build.Stale ||
		build.syncErrorText() != "JAVA_HOME is missing" ||
		len(build.Projects) != 2 || len(build.Projects[0].Tasks) != 2 {
		t.Fatalf("decoded Gradle model = %+v", build)
	}
}

func TestGradlePanelDrawsProjectsGroupsTasksAndRunConfigurations(t *testing.T) {
	m := &Model{}
	m.setRoot("/work/demo")
	m.run.configs = []openidea.RunConfig{{
		ID: "gradle:test", Name: "Run checks", Kind: "gradle", Command: "/work/demo/gradlew",
		Args: []string{"check"}, Cwd: strPtr("/work/demo"),
	}}
	m.maven.model.Gradle = []gradleModel{{
		Dir: "", Command: "/work/demo/gradlew",
		Projects: []gradleProject{
			{
				Path: ":", Name: "demo", Dir: "",
				Tasks: []gradleTask{
					{Name: "test", Group: "verification", Description: "Runs tests"},
					{Name: "assemble", Group: "build", Description: "Builds archives"},
				},
			},
			{
				Path: ":app", Name: "app", Dir: "app",
				Tasks: []gradleTask{{Name: "jar", Group: "build", Description: "Builds the JAR"}},
			},
		},
	}}
	m.ShowGradle()
	m.ExpandGradleAll()

	tester := workbenchTester(t, m)
	for _, text := range []string{
		"demo", "Tasks", "Run Configurations", "Run checks",
		"build", "verification", "assemble", "test", "app", "jar",
	} {
		if !tester.HasText(text) {
			t.Fatalf("the Gradle window lacks %q: %q", text, tester.Texts())
		}
	}
	if !m.dockWindowShown("gradle") || m.dockWindowShown("maven") {
		t.Fatalf("Gradle dock state: gradle=%t maven=%t",
			m.dockWindowShown("gradle"), m.dockWindowShown("maven"))
	}
}

func TestGradleTaskConfigFollowsOpenIDEAOptions(t *testing.T) {
	build := &gradleModel{
		Dir: "service", Command: "/work/demo/service/gradlew",
		Projects: []gradleProject{{
			Path: ":app", Name: "app", Dir: "app",
			Tasks: []gradleTask{{Name: "test"}, {Name: "jar"}},
		}},
	}
	project := &build.Projects[0]
	config := gradleTaskConfig(build, project, []string{"test", "jar"}, true, true)
	if config.Name != "app [test,jar]" {
		t.Fatalf("name = %q", config.Name)
	}
	if got := strings.Join(config.Args, " "); got != ":app:test :app:jar --offline -x test" {
		t.Fatalf("args = %q", got)
	}
	if config.IsTest {
		t.Fatal("Skip Tests produced a test configuration")
	}
	if config.Command != "/work/demo/service/gradlew" || config.Cwd == nil || *config.Cwd != "service" {
		t.Fatalf("launcher = %+v", config)
	}

	config = gradleTaskConfig(build, project, []string{"test"}, false, false)
	if !config.IsTest || strings.Join(config.Args, " ") != ":app:test" {
		t.Fatalf("test configuration = %+v", config)
	}
	if !gradleExcludesTest([]string{"-x", "test"}) ||
		!gradleExcludesTest([]string{"--exclude-task=test"}) ||
		gradleExcludesTest([]string{"-x", "check"}) {
		t.Fatal("Gradle test exclusion detection diverged from OpenIDEA")
	}
}

func TestGradleRunConfigurationUsesTheInnermostLinkedBuild(t *testing.T) {
	m := &Model{}
	m.setRoot("/work/demo")
	m.maven.model.Gradle = []gradleModel{
		{Dir: "", Projects: []gradleProject{{Path: ":", Name: "demo", Dir: ""}}},
		{Dir: "tools", Projects: []gradleProject{{Path: ":", Name: "tools", Dir: ""}}},
	}
	rootConfig := &openidea.RunConfig{ID: "root", Kind: "gradle", Cwd: strPtr("/work/demo/app")}
	nestedConfig := &openidea.RunConfig{ID: "nested", Kind: "gradle", Cwd: strPtr("tools/checks")}
	if owner := m.gradleConfigOwner(rootConfig); owner == nil || owner.Dir != "" {
		t.Fatalf("root configuration owner = %+v", owner)
	}
	if owner := m.gradleConfigOwner(nestedConfig); owner == nil || owner.Dir != "tools" {
		t.Fatalf("nested configuration owner = %+v", owner)
	}
}

func TestBuildToolLinesUseTheOpenIDEABuildLaunchers(t *testing.T) {
	for line, want := range map[string]string{
		"mvn test":             "maven",
		"./mvnw -o verify":     "maven",
		"gradle build":         "gradle",
		"./gradlew :app:test":  "gradle",
		"go test ./...":        "",
		"echo gradle whatever": "",
	} {
		if got := buildToolForLine(line); got != want {
			t.Errorf("buildToolForLine(%q) = %q, want %q", line, got, want)
		}
	}

	hook := &runChainHook{}
	m := newRunChainModel(t, hook)
	m.maven.model.Maven = &mavenModel{Projects: []mavenProject{{
		Pom: "pom.xml", Dir: "", ArtifactID: "demo", DisplayName: "demo", Command: "mvn",
	}}}
	m.maven.model.Gradle = []gradleModel{{
		Dir: "", Command: "./gradlew",
		Projects: []gradleProject{{
			Path: ":", Name: "demo", Dir: "",
			Tasks: []gradleTask{{Name: "test"}, {Name: "build"}},
		}},
	}}
	if !m.executeBuildToolLine("mvn test") {
		t.Fatal("Maven line did not start a build configuration")
	}
	waitRunChain(t, 2*time.Second, func() bool {
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == 1
	})
	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		return !m.run.busy && !m.run.awaiting && !m.run.running
	})
	hook.mu.Lock()
	if !strings.HasPrefix(hook.started[0], "maven:goal:") {
		t.Fatalf("Maven line started %q", hook.started[0])
	}
	hook.started = nil
	hook.mu.Unlock()

	m.maven.offline = true
	m.maven.skipTests = true
	m.maven.gradleOffline = true
	m.maven.gradleSkipTests = true
	if !m.executeBuildToolLine("./gradlew test") {
		t.Fatal("Gradle line did not start a build configuration")
	}
	waitRunChain(t, 2*time.Second, func() bool {
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == 1 && len(hook.configs) == 2
	})
	hook.mu.Lock()
	if !strings.HasPrefix(hook.started[0], "gradle:task:") {
		t.Fatalf("Gradle line started %q", hook.started[0])
	}
	if len(hook.configs) != 2 {
		t.Fatalf("Gradle configurations = %+v", hook.configs)
	}
	args := hook.configs[1].Args
	if !slices.Contains(args, "--offline") || !gradleExcludesTest(args) {
		t.Fatalf("Gradle line ignored its own offline/skip-tests switches: %v", args)
	}
	if slices.Contains(args, "-o") || slices.Contains(args, "-DskipTests") {
		t.Fatalf("Gradle line inherited Maven switches: %v", args)
	}
	hook.mu.Unlock()
}
