package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMavenPanelDrawsModulesAndDependencies(t *testing.T) {
	m := &Model{}
	m.maven = mavenPane{visible: true, model: buildModel{Maven: &mavenModel{Projects: []mavenProject{
		{Pom: "server/pom.xml", Dir: "server", GroupID: "demo", ArtifactID: "server", Version: "1.0",
			Packaging: "jar",
			Dependencies: []mavenDependency{{GroupID: "org.example", ArtifactID: "lib",
				Version: "2.0", Scope: "test"}}},
		{Pom: "api/pom.xml", Dir: "api", GroupID: "demo", ArtifactID: "api", Version: "1.0", Packaging: "jar"},
	}}}}
	m.DockShow("maven")
	m.mavenInitTree()
	// The module's Dependencies node is open, so its rows are drawn.
	m.maven.expanded["mvn:server/pom.xml:deps"] = true
	tt := workbenchTester(t, m)
	for _, text := range []string{"server", "api", "Lifecycle", "Dependencies", "org.example:lib:2.0", "(test)"} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not in the Maven window: %q", text, tt.Texts())
		}
	}
}

func TestMavenProfilesDiffFromTheDefaults(t *testing.T) {
	empty := &Model{maven: mavenPane{model: buildModel{Maven: &mavenModel{}}}}
	raw, err := json.Marshal(empty.mavenProfilesList())
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("an empty profile list encoded as %s, want []", raw)
	}
	root := ""
	m := &Model{}
	m.maven = mavenPane{model: buildModel{Maven: &mavenModel{
		Profiles: []mavenProfile{
			{ProjectRoot: &root, ID: "prod", ActiveByDefault: true},
			{ProjectRoot: &root, ID: "it", ActiveByDefault: false},
		},
	}}}
	if got := m.mavenProfilesList(); len(got) != 0 {
		t.Fatalf("defaults sync with %v, want nothing", got)
	}
	m.maven.profiles = map[string]bool{}
	m.SetMavenProfile(mavenProfile{ProjectRoot: &root, ID: "it"}, true)
	m.mavenInitTree()
	if !m.mavenProfileChecked(mavenProfile{ProjectRoot: &root, ID: "it"}) {
		t.Fatalf("a returned build model overwrote the profile choice with its default: %#v", m.maven.profiles)
	}
	// SyncMaven needs a backend; the arguments it would send are the contract.
	m.maven.profiles[mavenProfileKey(mavenProfile{ProjectRoot: &root, ID: "prod"})] = false
	m.maven.profiles[mavenProfileKey(mavenProfile{ProjectRoot: &root, ID: "it"})] = true
	got := m.mavenProfilesList()
	if len(got) != 2 || got[0] != "!prod" || got[1] != "it" {
		t.Fatalf("profiles list = %v, want [!prod it]", got)
	}
}

func TestMavenTreeFollowsTheReactor(t *testing.T) {
	root := ""
	m := &Model{}
	m.maven = mavenPane{model: buildModel{Maven: &mavenModel{
		Projects: []mavenProject{
			{Pom: "pom.xml", ArtifactID: "root", Aggregated: false, Modules: []string{"core/pom.xml"}},
			{Pom: "core/pom.xml", Dir: "core", ArtifactID: "core", Aggregated: true},
		},
		Profiles: []mavenProfile{{ProjectRoot: &root, ID: "dev", ActiveByDefault: true}},
	}}}
	m.mavenInitTree()
	rows := m.mavenRootRows()
	var labels []string
	for _, r := range rows {
		labels = append(labels, r.label)
	}
	if len(rows) != 2 || labels[0] != "Profiles" || labels[1] != "root" {
		t.Fatalf("roots = %v, want Profiles and root (an aggregated module is not a root)", labels)
	}
	kids := m.mavenChildRows(rows[1])
	var kinds []string
	for _, k := range kids {
		kinds = append(kinds, k.kind)
	}
	if len(kinds) < 3 || kinds[0] != "lifecycle" || kinds[1] != "plugins" {
		t.Fatalf("project children = %v, want lifecycle, plugins, …", kinds)
	}
	if kinds[len(kinds)-1] != "project" {
		t.Fatalf("the module is not under its aggregator: %v", kinds)
	}
}

// TestMavenGoalConfigCarriesEveryRunConfigField guards the backend's
// deserializer: a partial object is refused with "missing field `isTest`",
// which is what a phase run showed before the config was whole.
func TestMavenGoalConfigCarriesEveryRunConfigField(t *testing.T) {
	m := &Model{}
	p := &mavenProject{DisplayName: "tg-tenant", ArtifactID: "tg-tenant", Pom: "pom.xml", Command: "mvn"}
	cfg, name := m.mavenGoalRunConfig(p, []string{"test"})
	if name != "tg-tenant [test]" {
		t.Fatalf("name = %q", name)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"id"`, `"name"`, `"kind"`, `"command"`, `"args"`, `"cwd"`,
		`"env"`, `"detected"`, `"isTest"`, `"debugAdapter"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("%s is missing from the configuration: %s", field, raw)
		}
	}
	if !cfg.IsTest {
		t.Fatal("a test goal is not marked as a test")
	}
	if cfg, _ := m.mavenGoalRunConfig(p, []string{"validate"}); cfg.IsTest {
		t.Fatal("validate is marked as a test")
	}
}

func TestMavenSettingsDriveGoalCommandArgumentsAndEnvironment(t *testing.T) {
	home := t.TempDir()
	maven := filepath.Join(home, "bin", "mvn")
	if err := os.MkdirAll(filepath.Dir(maven), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(maven, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	jre := filepath.Join(t.TempDir(), "jdk-21")
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageMaven: {
			loaded: true,
			applied: map[string]any{
				"languages.java.maven.home":            home,
				"languages.java.maven.runnerJre":       jre,
				"languages.java.maven.userSettings":    "~/.m2/settings.xml",
				"languages.java.maven.toolchains":      "/tmp/toolchains.xml",
				"languages.java.maven.localRepository": "/tmp/repository",
				"languages.java.maven.activeProfiles":  "dev, !prod,dev",
				"languages.java.maven.threads":         "2.5C",
			},
			draft: map[string]any{
				"languages.java.maven.runnerJre": jre,
			},
		},
	}
	p := &mavenProject{DisplayName: "app", Pom: "pom.xml", Command: "./mvnw"}
	cfg, _ := m.mavenGoalRunConfig(p, []string{"test"})
	if cfg.Command != maven {
		t.Fatalf("Maven command = %q, want %q", cfg.Command, maven)
	}
	want := []string{
		"-s", expandMavenHome("~/.m2/settings.xml"),
		"-t", "/tmp/toolchains.xml",
		"-Dmaven.repo.local=/tmp/repository",
		"-T", "2.5C",
		"-P", "dev",
		"test",
	}
	if !reflect.DeepEqual(cfg.Args, want) {
		t.Fatalf("Maven args = %#v, want %#v", cfg.Args, want)
	}
	if cfg.Env["JAVA_HOME"] != jre {
		t.Fatalf("JAVA_HOME = %q, want %q", cfg.Env["JAVA_HOME"], jre)
	}
}

func TestBuildToolSettingsPersistPerProject(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()

	m := &Model{
		cfg:   Config{SettingsPath: settingsPath},
		prefs: LoadPreferences(settingsPath),
	}
	m.setRoot(firstRoot)
	m.loadBuildModel()
	firstProfile := mavenProfile{ProjectRoot: strPtr(firstRoot), ID: "integration"}
	m.maven.model.Maven = &mavenModel{Profiles: []mavenProfile{firstProfile}}
	m.maven.profiles = map[string]bool{}
	m.SetMavenProfile(firstProfile, true)
	m.ToggleMavenOffline()
	m.ToggleMavenSkipTests()
	m.ToggleGradleOffline()

	restored := &Model{
		cfg:   Config{SettingsPath: settingsPath},
		prefs: LoadPreferences(settingsPath),
	}
	restored.setRoot(firstRoot)
	restored.loadBuildModel()
	if !restored.maven.offline || !restored.maven.skipTests || !restored.maven.gradleOffline {
		t.Fatalf("first project's build-tool settings were not restored: %+v", restored.maven)
	}
	if restored.maven.gradleSkipTests {
		t.Fatal("an unspecified Gradle skip-tests choice did not default to false")
	}
	if !restored.mavenProfileChecked(firstProfile) {
		t.Fatal("first project's Maven profile choice was not restored")
	}

	restored.setRoot(secondRoot)
	restored.loadBuildModel()
	if restored.maven.offline || restored.maven.skipTests ||
		restored.maven.gradleOffline || restored.maven.gradleSkipTests {
		t.Fatalf("second project inherited another project's settings: %+v", restored.maven)
	}
	restored.ToggleGradleSkipTests()

	restored.setRoot(firstRoot)
	restored.loadBuildModel()
	if !restored.maven.offline || !restored.maven.skipTests ||
		!restored.maven.gradleOffline || restored.maven.gradleSkipTests {
		t.Fatalf("switching back lost the first project's settings: %+v", restored.maven)
	}

	stored := LoadPreferences(settingsPath)
	var settings buildToolsSettings
	if err := json.Unmarshal(stored.Plugins[buildToolsSettingsKey], &settings); err != nil {
		t.Fatalf("stored build-tool settings: %v", err)
	}
	if got := settings.Projects[buildToolsProjectKey(firstRoot)]; !got.Maven.Offline ||
		!got.Maven.SkipTests || !got.Maven.Profiles[mavenProfileKey(firstProfile)] ||
		!got.Gradle.Offline || got.Gradle.SkipTests {
		t.Fatalf("stored first-project settings = %+v", got)
	}
	if got := settings.Projects[buildToolsProjectKey(secondRoot)]; got.Maven.Offline ||
		got.Maven.SkipTests || got.Gradle.Offline || !got.Gradle.SkipTests {
		t.Fatalf("stored second-project settings = %+v", got)
	}
}

func TestMavenExecuteGoalUsesCurrentProjectProfilesAndNames(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	first := mavenProject{
		ProjectRoot: strPtr(firstRoot), Pom: "first/pom.xml", Dir: "first",
		ArtifactID: "first", DisplayName: "first",
	}
	second := mavenProject{
		ProjectRoot: strPtr(secondRoot), Pom: "second/pom.xml", Dir: "second",
		ArtifactID: "second", DisplayName: "second",
	}
	firstProfile := mavenProfile{ProjectRoot: first.ProjectRoot, ID: "first-profile"}
	secondProfile := mavenProfile{ProjectRoot: second.ProjectRoot, ID: "second-profile"}
	m := &Model{}
	m.maven.model.Maven = &mavenModel{
		Projects: []mavenProject{first, second},
		Profiles: []mavenProfile{firstProfile, secondProfile},
	}
	m.maven.profiles = map[string]bool{
		mavenProfileKey(firstProfile):  true,
		mavenProfileKey(secondProfile): true,
	}
	m.selectMavenProject(m.mavenProjectByPom(second.Pom))

	project := m.mavenRootProject()
	if project == nil || project.Pom != second.Pom {
		t.Fatalf("current Maven project = %+v, want %s", project, second.Pom)
	}
	if m.maven.sel.kind != "project" || m.maven.sel.project == nil ||
		m.maven.sel.project.Pom != second.Pom {
		t.Fatalf("Maven selection = %+v, want the selected project", m.maven.sel)
	}
	cfg, name := m.mavenGoalRunConfig(project, []string{"-DskipTests=false", "verify"})
	if name != "second [verify]" {
		t.Fatalf("Maven goal name = %q, want %q", name, "second [verify]")
	}
	if !reflect.DeepEqual(cfg.Args, []string{"-P", "second-profile", "-DskipTests=false", "verify"}) {
		t.Fatalf("Maven goal args = %#v, want only the current project's profile", cfg.Args)
	}
}

func TestMavenSettingsRejectInvalidThreadsAndExcludedProfile(t *testing.T) {
	if got := normalizedMavenThreads("four"); got != "" {
		t.Fatalf("invalid thread count = %q", got)
	}
	if got := mavenActiveProfiles("dev,!prod", []string{"prod", "qa"}); !reflect.DeepEqual(got, []string{"dev", "qa"}) {
		t.Fatalf("Maven profiles = %#v, want dev and qa", got)
	}
}

func TestMavenDaemonFallsBackForOldRunnerJRE(t *testing.T) {
	jre := t.TempDir()
	if err := os.MkdirAll(filepath.Join(jre, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(jre, "bin", "java")
	if err := os.WriteFile(java, []byte(
		"#!/bin/sh\nprintf 'openjdk version \"11.0.2\" 2019-01-15\\n' >&2\n",
	), 0o755); err != nil {
		t.Fatal(err)
	}
	if !javaRuntimeAtLeast17(`openjdk version "17.0.10" 2024-01-16`) {
		t.Fatal("Java 17 was not recognized")
	}
	if javaRuntimeAtLeast17(`openjdk version "11.0.2" 2019-01-15`) {
		t.Fatal("Java 11 was accepted for Maven Daemon")
	}
	if mavenRunnerJreSupportsDaemon(jre) {
		t.Fatal("the Java 11 runner was accepted for Maven Daemon")
	}
}
