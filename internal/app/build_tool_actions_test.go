package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestOpenIDEABuildToolActionsAreRegistered(t *testing.T) {
	for _, id := range []string{
		"maven.addProjects",
		"maven.importExternal",
		"maven.unlinkProjects",
		"maven.executeGoal",
		"maven.generateSources",
		"gradle.linkProjects",
		"gradle.unlinkProjects",
		"gradle.executeTask",
	} {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA build-tool action %q", id)
		}
	}
}

func TestBuildToolActionsFollowOpenIDEAProjectAndModelGates(t *testing.T) {
	m := &Model{}
	for _, id := range []string{
		"maven.addProjects", "maven.importExternal", "maven.unlinkProjects",
		"maven.executeGoal", "maven.generateSources", "gradle.linkProjects",
		"gradle.unlinkProjects", "gradle.executeTask",
	} {
		if m.ActionEnabled(id) {
			t.Errorf("%s is enabled without an open project", id)
		}
	}
	m.setRoot("/project")
	for _, id := range []string{"maven.addProjects", "maven.importExternal", "gradle.linkProjects"} {
		if !m.ActionEnabled(id) {
			t.Errorf("%s is disabled in an ordinary project", id)
		}
	}
	m.maven.model = buildModel{
		Maven:  &mavenModel{Projects: []mavenProject{{Pom: "pom.xml"}}},
		Gradle: []gradleModel{{Projects: []gradleProject{{Path: ":"}}}},
	}
	for _, id := range []string{
		"maven.unlinkProjects", "maven.executeGoal", "maven.generateSources",
		"gradle.unlinkProjects", "gradle.executeTask",
	} {
		if !m.ActionEnabled(id) {
			t.Errorf("%s is disabled with its build model", id)
		}
	}
	m.workspace = &openidea.Workspace{}
	for _, id := range []string{
		"maven.addProjects", "maven.importExternal", "maven.unlinkProjects",
		"gradle.linkProjects", "gradle.unlinkProjects",
	} {
		if m.ActionEnabled(id) {
			t.Errorf("%s is enabled in a workspace", id)
		}
	}
}

func TestBuildToolUnlinkDialogsUseOpenIDEACopy(t *testing.T) {
	gradle := buildToolUnlinkOptions("gradle", false, nil)
	if gradle.Title != "Unlink Gradle project?" ||
		gradle.Message != "Remove this build from Gradle? Files remain in the Project view." ||
		gradle.DefaultButton != 1 || gradle.CancelButton != 1 {
		t.Fatalf("Gradle unlink dialog = %+v", gradle)
	}
	maven := buildToolUnlinkOptions("maven", false, nil)
	if maven.Title != "Unlink Maven projects?" ||
		maven.Message != "Files remain in the Project view." {
		t.Fatalf("Maven unlink dialog = %+v", maven)
	}
	aggregated := buildToolUnlinkOptions("maven", true, nil)
	if aggregated.Message != "The Java language server stops importing this module and its submodules. Files remain in the Project view." {
		t.Fatalf("aggregated Maven unlink dialog = %+v", aggregated)
	}
}

func TestClassifyMavenPOMsUsesContentRootAndAbsoluteExternalPaths(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "modules", "api", "pom.xml")
	outside := filepath.Join(filepath.Dir(root), "shared", "pom.xml")
	notPOM := filepath.Join(root, "build.xml")
	link, rejected := classifyMavenPOMs(root, []string{inside, outside, notPOM, inside})
	want := []string{"modules/api/pom.xml", filepath.ToSlash(filepath.Clean(outside))}
	if !reflect.DeepEqual(link, want) {
		t.Fatalf("linked POMs = %v, want %v", link, want)
	}
	if !reflect.DeepEqual(rejected, []string{notPOM}) {
		t.Fatalf("rejected paths = %v, want %s", rejected, notPOM)
	}
}

func TestMavenRejectedImportMessageFollowsOpenIDEA(t *testing.T) {
	rejected := []string{"a.xml", "b.xml", "c.xml", "d.xml"}
	got := buildToolRejectedBody(rejected)
	want := "These files are not named pom.xml, so they were not added: a.xml, b.xml, c.xml (+1 more)"
	if got != want {
		t.Fatalf("rejected body = %q, want %q", got, want)
	}
}

func TestImportMavenModuleAcceptsAPOMOrDirectory(t *testing.T) {
	root := t.TempDir()
	pom := filepath.Join(root, "pom.xml")
	if err := os.WriteFile(pom, []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, picked := range []string{root, pom} {
		got, ok := mavenPOMForImport(picked)
		if !ok || got != pom {
			t.Fatalf("mavenPOMForImport(%q) = %q, %v, want %q", picked, got, ok, pom)
		}
	}
	if _, ok := mavenPOMForImport(t.TempDir()); ok {
		t.Fatal("a directory without pom.xml was accepted")
	}
}

func TestNextMavenLinkSettingsMirrorsOpenIDEALinkage(t *testing.T) {
	projects := []mavenProject{
		{Pom: "pom.xml"},
		{Pom: "modules/api/pom.xml"},
	}
	unlinked, linked := nextMavenLinkSettings(
		[]string{"modules/api/pom.xml"},
		nil,
		projects,
		[]string{"modules/api/pom.xml", "../shared/pom.xml"},
		true,
	)
	if len(unlinked) != 0 {
		t.Fatalf("linked module stayed unlinked: %v", unlinked)
	}
	if !reflect.DeepEqual(linked, []string{"../shared/pom.xml"}) {
		t.Fatalf("linked external POMs = %v", linked)
	}

	unlinked, linked = nextMavenLinkSettings(
		nil,
		[]string{"../shared/pom.xml"},
		projects,
		[]string{"pom.xml", "../shared/pom.xml"},
		false,
	)
	if !reflect.DeepEqual(linked, []string{}) {
		t.Fatalf("unlinked external POMs = %v", linked)
	}
	if !reflect.DeepEqual(unlinked, []string{"pom.xml"}) {
		t.Fatalf("discovered POM was not unlinked: %v", unlinked)
	}

	unlinked, linked = nextMavenLinkSettings(
		[]string{"pom.xml"},
		nil,
		projects,
		[]string{"pom.xml"},
		false,
	)
	if !reflect.DeepEqual(unlinked, []string{"pom.xml"}) || len(linked) != 0 {
		t.Fatalf("idempotent unlink = %v, %v", unlinked, linked)
	}
}

func TestBuildToolArgumentsAcceptOpenIDEAShapes(t *testing.T) {
	cases := []any{
		"pom.xml",
		map[string]any{"pom": "pom.xml"},
		[]byte(`{"pom":"pom.xml"}`),
	}
	for _, arg := range cases {
		if got := buildToolArgString(arg, "pom"); got != "pom.xml" {
			t.Fatalf("buildToolArgString(%#v) = %q", arg, got)
		}
	}
}

func TestOpenIDEABuildToolCommandsReachNativeState(t *testing.T) {
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.maven.model = buildModel{
		Maven: &mavenModel{Projects: []mavenProject{{
			Pom:        "pom.xml",
			ArtifactID: "demo",
			Dependencies: []mavenDependency{{
				GroupID: "org.example", ArtifactID: "lib",
			}},
		}}},
		Gradle: []gradleModel{{
			Dir: "",
			Projects: []gradleProject{{
				Path: ":", Name: "demo",
				Tasks: []gradleTask{{Name: "test", Group: "verification"}},
			}},
		}, {
			Dir: "tools",
			Projects: []gradleProject{{
				Path: ":", Name: "tools",
			}, {
				Path: ":app", Name: "app",
				Tasks: []gradleTask{{Name: "jar", Group: "build"}},
			}},
		}},
	}
	m.mavenInitTree()
	m.maven.expanded = map[string]bool{
		"mvn:pom.xml":      true,
		"mvn:pom.xml:deps": true,
		"gradle::":         true,
		"gradle:::tasks":   true,
	}

	if !m.RunCommand("maven.collapseAll") || m.maven.expanded["mvn:pom.xml"] ||
		!m.maven.expanded["gradle::"] {
		t.Fatal("maven.collapseAll did not affect only the Maven tree")
	}
	if !m.RunCommand("gradle.collapseAll") || m.maven.expanded["gradle::"] {
		t.Fatal("gradle.collapseAll did not affect the Gradle tree")
	}
	if !m.RunCommand("maven.expandAll") || !m.maven.expanded["mvn:pom.xml"] {
		t.Fatal("maven.expandAll did not expand the Maven tree")
	}
	if !m.RunCommand("gradle.expandAll") {
		t.Fatal("gradle.expandAll is unavailable")
	}
	gradleExpanded := false
	for key := range m.maven.expanded {
		if strings.HasPrefix(key, "gradle:") {
			gradleExpanded = true
			break
		}
	}
	if !gradleExpanded {
		t.Fatal("gradle.expandAll did not expand the Gradle tree")
	}

	for _, tc := range []struct {
		command string
		value   func() bool
		before  bool
	}{
		{"maven.toggleOffline", func() bool { return m.maven.offline }, m.maven.offline},
		{"maven.toggleSkipTests", func() bool { return m.maven.skipTests }, m.maven.skipTests},
		{"gradle.toggleOffline", func() bool { return m.maven.gradleOffline }, m.maven.gradleOffline},
		{"gradle.toggleSkipTests", func() bool { return m.maven.gradleSkipTests }, m.maven.gradleSkipTests},
	} {
		if !m.RunCommand(tc.command) || tc.value() == tc.before || !m.ActionToggled(tc.command) {
			t.Errorf("%s did not toggle native build state", tc.command)
		}
	}
	m.runAnything = nil
	if !m.RunActionWithArg("gradle.executeTask", map[string]any{
		"projectKey": "gradle:tools::app",
	}) || m.maven.gradleSel.project == nil || m.maven.gradleSel.project.Path != ":app" {
		t.Fatal("gradle.executeTask ignored projectKey")
	}
	if m.runAnything == nil || len(m.runAnything.rows) == 0 {
		t.Fatal("gradle.executeTask did not open Run Anything for the selected project")
	}
	for _, id := range []string{
		"maven.cancelReload", "maven.expandAll", "maven.collapseAll",
		"gradle.cancelReload", "gradle.expandAll", "gradle.collapseAll",
	} {
		if !hiddenActionIDs[id] {
			t.Errorf("%s is not hidden from the command palette", id)
		}
	}

	m.maven.syncing = true
	m.maven.syncTool = "maven"
	m.maven.request = 77
	cancelled := make(chan uint32, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "cancel_request" {
			cancelled <- args.(map[string]any)["id"].(uint32)
		}
		return nil, nil
	}
	if !m.RunCommand("maven.cancelReload") {
		t.Fatal("maven.cancelReload is unavailable")
	}
	if m.maven.syncing || m.maven.request != 0 {
		t.Fatal("maven.cancelReload kept a live sync state")
	}
	select {
	case id := <-cancelled:
		if id != 77 {
			t.Fatalf("cancel_request id = %d, want 77", id)
		}
	case <-time.After(time.Second):
		t.Fatal("maven.cancelReload did not send cancel_request")
	}
	m.maven.syncing = true
	m.maven.syncTool = "gradle"
	m.maven.request = 88
	if !m.RunCommand("maven.cancelReload") || !m.maven.syncing || m.maven.request != 88 {
		t.Fatal("maven.cancelReload canceled a Gradle sync")
	}
	if !m.RunCommand("gradle.cancelReload") {
		t.Fatal("gradle.cancelReload is unavailable")
	}
	if m.maven.syncing || m.maven.request != 0 || m.maven.syncTool != "" {
		t.Fatal("gradle.cancelReload kept a live sync state")
	}
	select {
	case id := <-cancelled:
		if id != 88 {
			t.Fatalf("Gradle cancel_request id = %d, want 88", id)
		}
	case <-time.After(time.Second):
		t.Fatal("gradle.cancelReload did not send cancel_request")
	}

	for _, tc := range []struct {
		command string
		want    string
		window  string
	}{
		{"maven.executeGoal", "mvn ", "maven"},
		{"gradle.executeTask", "gradle ", "gradle"},
	} {
		m.runAnything = nil
		if !m.RunCommand(tc.command) {
			t.Fatalf("%s is unavailable", tc.command)
		}
		if m.runAnything == nil || m.runAnything.query != tc.want || m.maven.activeTool != tc.window ||
			m.runAnything.buildTool != tc.window || m.runAnything.project == "" {
			t.Fatalf("%s opened query=%#v tool=%q", tc.command, m.runAnything, m.maven.activeTool)
		}
		if len(m.runAnything.rows) == 0 {
			t.Fatalf("%s did not refresh Run Anything rows", tc.command)
		}
	}

	if !m.RunCommand("maven.settings") || !m.settingsOpen || m.settingsPage != settingsPageMaven {
		t.Fatal("maven.settings did not open the Maven settings page")
	}

	if !m.RunCommand("maven.analyzeDependencies") {
		t.Fatal("maven.analyzeDependencies is unavailable")
	}
	if m.maven.analyzer == nil || !m.maven.analyzer.open {
		t.Fatal("maven.analyzeDependencies did not open analyzer state")
	}
	m.maven.analyzer = nil
	m.maven.model.Maven.Projects = append(m.maven.model.Maven.Projects, mavenProject{
		Pom: "app/pom.xml", ArtifactID: "app",
	})
	if !m.RunActionWithArg("maven.analyzeDependencies", map[string]any{"pom": "app/pom.xml"}) ||
		m.maven.analyzer == nil || m.maven.analyzer.module != "mvn:app/pom.xml" {
		t.Fatal("maven.analyzeDependencies ignored its POM argument")
	}
	m.maven.analyzer = nil
	if !m.RunActionWithArg("gradle.analyzeDependencies", map[string]any{
		"buildDir": "tools", "gradlePath": ":app",
	}) || m.maven.analyzer == nil || m.maven.analyzer.module != "gradle:tools\x00:app" {
		t.Fatal("gradle.analyzeDependencies ignored its build/project argument")
	}
}

func TestBuildToolExecutePopupRendersTheSelectedProject(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.run.loaded = true
	m.maven.model = buildModel{Maven: &mavenModel{Projects: []mavenProject{
		{Pom: "pom.xml", DisplayName: "demo", Dir: "."},
		{Pom: "api/pom.xml", DisplayName: "api", Dir: "api"},
	}}}
	m.OpenBuildToolExecute("maven")
	if m.runAnything == nil || m.runAnything.buildTool != "maven" || m.runAnything.project != "pom.xml" {
		t.Fatalf("Maven Execute popup = %+v", m.runAnything)
	}
	tt := runAnythingTester(t, m)
	for _, text := range []string{"Project:", "demo — ."} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from Execute Maven Goal: %q", text, tt.Texts())
		}
	}
}
