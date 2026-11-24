package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestBuildToolCollapseKeepsTheOtherTreeExpanded(t *testing.T) {
	m := &Model{}
	m.maven.expanded = map[string]bool{
		"mvn:pom.xml":           true,
		"mvn:pom.xml:lifecycle": true,
		"gradle::":              true,
		"gradle:::tasks":        true,
	}

	m.CollapseMavenAll()
	if m.maven.expanded["mvn:pom.xml"] || m.maven.expanded["mvn:pom.xml:lifecycle"] {
		t.Fatalf("Maven collapse left Maven rows expanded: %v", m.maven.expanded)
	}
	if !m.maven.expanded["gradle::"] || !m.maven.expanded["gradle:::tasks"] {
		t.Fatalf("Maven collapse changed the Gradle tree: %v", m.maven.expanded)
	}

	m.maven.expanded["mvn:pom.xml"] = true
	m.CollapseGradleAll()
	if m.maven.expanded["gradle::"] || m.maven.expanded["gradle:::tasks"] {
		t.Fatalf("Gradle collapse left Gradle rows expanded: %v", m.maven.expanded)
	}
	if !m.maven.expanded["mvn:pom.xml"] {
		t.Fatalf("Gradle collapse changed the Maven tree: %v", m.maven.expanded)
	}
}

func TestMavenRunConfigurationsMatchKindProjectRootAndWorkingDirectory(t *testing.T) {
	m := &Model{}
	m.setRoot("/work/workspace")
	projectRoot := "/work/linked"
	project := mavenProject{ProjectRoot: &projectRoot, Pom: "pom.xml", Dir: "service"}
	m.run.configs = []openidea.RunConfig{
		{ID: "maven-absolute", Kind: "maven", Cwd: strPtr("/work/linked/service")},
		{ID: "maven-relative", Kind: "maven", Cwd: strPtr("service")},
		{ID: "gradle", Kind: "gradle", Cwd: strPtr("/work/linked/service")},
		{ID: "nested", Kind: "maven", Cwd: strPtr("/work/linked/service/sub")},
	}

	configs := m.mavenConfigsOf(project)
	if len(configs) != 2 || configs[0].ID != "maven-absolute" || configs[1].ID != "maven-relative" {
		t.Fatalf("Maven configs = %+v, want only the two configs in the module directory", configs)
	}
}

func TestBuildSyncUsesTheRealToolbarArguments(t *testing.T) {
	t.Run("maven", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		profileRoot := "/work/demo"
		defaultProfile := mavenProfile{
			ProjectRoot: &profileRoot, ID: "prod", ActiveByDefault: true,
		}
		selectedProfile := mavenProfile{
			ProjectRoot: &profileRoot, ID: "it", ActiveByDefault: false,
		}
		model := buildModel{
			Maven: &mavenModel{
				Profiles: []mavenProfile{defaultProfile, selectedProfile},
				SyncedAt: floatPointer(1),
			},
			Gradle: []gradleModel{{Dir: "", SyncedAt: floatPointer(1)}},
		}
		raw, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		var callsMu sync.Mutex
		calls := 0
		m := &Model{
			ctx: ctx,
			invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
				if command != "build_sync" {
					return nil, fmt.Errorf("unexpected RPC %s", command)
				}
				callsMu.Lock()
				calls++
				callsMu.Unlock()
				payload, _ = args.(map[string]any)
				return raw, nil
			},
			postHook: func(fn func()) { fn() },
		}
		m.setRoot("/work/demo")
		m.maven.model.Maven = &mavenModel{
			Profiles: []mavenProfile{defaultProfile, selectedProfile},
		}
		m.maven.offline = true
		m.maven.profiles = map[string]bool{
			mavenProfileKey(defaultProfile):  false,
			mavenProfileKey(selectedProfile): true,
		}

		m.SyncMaven()
		waitBuildToolTest(t, 2*time.Second, func() bool {
			callsMu.Lock()
			defer callsMu.Unlock()
			return calls == 1 && !m.maven.syncing
		})
		if payload["tool"] != "maven" || payload["offline"] != true || payload["buildDir"] != nil {
			t.Fatalf("Maven build_sync payload = %#v", payload)
		}
		if got := payload["profiles"]; !payloadStringsEqual(got, []string{"!prod", "it"}) {
			t.Fatalf("profiles = %#v, want [!prod it]", got)
		}
		if _, ok := payload["request"].(uint32); !ok {
			t.Fatalf("request = %#v", payload["request"])
		}
		progress, ok := payload["onProgress"].(map[string]any)
		if !ok || progress["$channel"] == nil {
			t.Fatalf("onProgress = %#v", payload["onProgress"])
		}
		if m.maven.model.Maven == nil || m.maven.model.Maven.SyncedAt == nil {
			t.Fatal("the build_sync model was not applied")
		}
	})

	t.Run("gradle independent builds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		model := buildModel{
			Maven: &mavenModel{SyncedAt: floatPointer(1)},
			Gradle: []gradleModel{
				{Dir: "", Projects: []gradleProject{{Path: ":", Name: "root"}}, SyncedAt: floatPointer(1)},
				{Dir: "tools", Projects: []gradleProject{{Path: ":", Name: "tools"}}, SyncedAt: floatPointer(1)},
			},
		}
		raw, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		var payloads []map[string]any
		var callsMu sync.Mutex
		m := &Model{
			ctx: ctx,
			invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
				if command != "build_sync" {
					return nil, fmt.Errorf("unexpected RPC %s", command)
				}
				callsMu.Lock()
				payloads = append(payloads, args.(map[string]any))
				callsMu.Unlock()
				return raw, nil
			},
			postHook: func(fn func()) { fn() },
		}
		m.setRoot("/work/demo")
		m.maven.gradleOffline = true

		m.SyncGradleBuilds([]string{"", "tools", "tools"})
		waitBuildToolTest(t, 2*time.Second, func() bool {
			callsMu.Lock()
			defer callsMu.Unlock()
			return len(payloads) == 2 && !m.maven.syncing
		})
		callsMu.Lock()
		defer callsMu.Unlock()
		if payloads[0]["buildDir"] != nil || payloads[1]["buildDir"] != "tools" {
			t.Fatalf("Gradle buildDir payloads = %#v, %#v", payloads[0]["buildDir"], payloads[1]["buildDir"])
		}
		for _, payload := range payloads {
			if payload["tool"] != "gradle" || payload["offline"] != true {
				t.Fatalf("Gradle build_sync payload = %#v", payload)
			}
			if got := payload["profiles"]; !slices.Equal(got.([]string), []string{}) {
				t.Fatalf("Gradle profiles = %#v", got)
			}
		}
	})
}

func TestBuildModelAutoSyncsTheVisibleToolFirst(t *testing.T) {
	t.Run("maven", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		var calls []string
		var callsMu sync.Mutex
		m := &Model{
			ctx: ctx,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				callsMu.Lock()
				calls = append(calls, command)
				callsMu.Unlock()
				switch command {
				case "build_model":
					return mustBuildToolJSON(t, buildModel{
						Maven: &mavenModel{Projects: []mavenProject{{Pom: "pom.xml"}}},
					}), nil
				case "build_sync":
					return mustBuildToolJSON(t, buildModel{
						Maven: &mavenModel{SyncedAt: floatPointer(1)},
					}), nil
				default:
					return nil, fmt.Errorf("unexpected RPC %s", command)
				}
			},
			postHook: func(fn func()) { fn() },
		}
		m.setRoot("/work/demo")
		m.maven.visible = true
		m.maven.activeTool = "maven"
		m.loadBuildModel()
		waitBuildToolTest(t, 2*time.Second, func() bool {
			callsMu.Lock()
			defer callsMu.Unlock()
			return len(calls) == 2 && !m.maven.loading && !m.maven.syncing
		})
		callsMu.Lock()
		defer callsMu.Unlock()
		if !slices.Equal(calls, []string{"build_model", "build_sync"}) {
			t.Fatalf("RPC calls = %v", calls)
		}
	})

	t.Run("gradle", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		var calls []string
		var callsMu sync.Mutex
		m := &Model{
			ctx: ctx,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				callsMu.Lock()
				calls = append(calls, command)
				callsMu.Unlock()
				switch command {
				case "build_model":
					return mustBuildToolJSON(t, buildModel{
						Maven: &mavenModel{SyncedAt: floatPointer(1)},
						Gradle: []gradleModel{{
							Dir: "", Projects: []gradleProject{{Path: ":", Name: "demo"}},
						}},
					}), nil
				case "build_sync":
					return mustBuildToolJSON(t, buildModel{
						Maven: &mavenModel{SyncedAt: floatPointer(1)},
						Gradle: []gradleModel{{
							Dir: "", Projects: []gradleProject{{Path: ":", Name: "demo"}},
							SyncedAt: floatPointer(1),
						}},
					}), nil
				default:
					return nil, fmt.Errorf("unexpected RPC %s", command)
				}
			},
			postHook: func(fn func()) { fn() },
		}
		m.setRoot("/work/demo")
		m.maven.gradleVisible = true
		m.maven.activeTool = "gradle"
		m.loadBuildModel()
		waitBuildToolTest(t, 2*time.Second, func() bool {
			callsMu.Lock()
			defer callsMu.Unlock()
			return len(calls) == 2 && !m.maven.loading && !m.maven.syncing
		})
		callsMu.Lock()
		defer callsMu.Unlock()
		if !slices.Equal(calls, []string{"build_model", "build_sync"}) {
			t.Fatalf("RPC calls = %v", calls)
		}
	})
}

func TestDependencyAnalyzerUsesGradleDependencyRPC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	requested := "1.0"
	selected := "1.1"
	response := mustBuildToolJSON(t, []gradleDependency{{
		Group: "org.example", Name: "lib", Requested: &requested, Selected: &selected,
		Children: []gradleDependency{{Group: "org.example", Name: "child", Selected: &selected}},
	}})
	var payload map[string]any
	var callsMu sync.Mutex
	m := &Model{
		ctx: ctx,
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "gradle_dependencies" {
				return nil, fmt.Errorf("unexpected RPC %s", command)
			}
			callsMu.Lock()
			payload = args.(map[string]any)
			callsMu.Unlock()
			return response, nil
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/work/demo")
	m.maven.model.Gradle = []gradleModel{{
		Dir: "tools", SyncedAt: floatPointer(1),
		Projects: []gradleProject{{Path: ":app", Name: "app", Dir: "app"}},
	}}
	m.maven.gradleOffline = true

	m.OpenBuildDependencyAnalyzer(map[string]any{"buildDir": "tools", "gradlePath": ":app"})
	waitBuildToolTest(t, 2*time.Second, func() bool {
		callsMu.Lock()
		defer callsMu.Unlock()
		return payload != nil && !m.maven.analyzer.loading
	})
	callsMu.Lock()
	if payload["gradlePath"] != ":app" ||
		payload["configuration"] != "runtimeClasspath" ||
		payload["offline"] != true ||
		payload["buildDir"] != "tools" {
		t.Fatalf("gradle_dependencies payload = %#v", payload)
	}
	if _, ok := payload["request"].(uint32); !ok {
		t.Fatalf("request = %#v", payload["request"])
	}
	callsMu.Unlock()
	deps := m.maven.analyzer.deps
	if len(deps) != 1 || deps[0].Requested == nil || *deps[0].Requested != "1.0" ||
		deps[0].Version != "1.1" || len(deps[0].Children) != 1 {
		t.Fatalf("converted Gradle dependencies = %+v", deps)
	}
}

func TestDependencyAnalyzerStopCancelsTheGradleRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	requestSeen := make(chan uint32, 1)
	cancelSeen := make(chan uint32, 1)
	m := &Model{
		ctx: ctx,
		invokeHook: func(ctx context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "gradle_dependencies":
				requestSeen <- args.(map[string]any)["request"].(uint32)
				<-ctx.Done()
				return nil, ctx.Err()
			case "cancel_request":
				cancelSeen <- args.(map[string]any)["id"].(uint32)
				return json.RawMessage(`{}`), nil
			default:
				return nil, fmt.Errorf("unexpected RPC %s", command)
			}
		},
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/work/demo")
	m.maven.model.Gradle = []gradleModel{{
		Dir: "", SyncedAt: floatPointer(1),
		Projects: []gradleProject{{Path: ":", Name: "demo"}},
	}}
	m.OpenBuildDependencyAnalyzer(map[string]any{"buildDir": "", "gradlePath": ":"})

	var request uint32
	select {
	case request = <-requestSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("gradle_dependencies did not start")
	}
	m.CloseBuildDependencyAnalyzer()
	select {
	case cancelled := <-cancelSeen:
		if cancelled != request {
			t.Fatalf("cancel_request id = %d, want %d", cancelled, request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the analyzer did not cancel its Gradle request")
	}
}

func TestMavenDependencyNavigationAndExclusionRPCArguments(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var calls []struct {
		command string
		payload map[string]any
	}
	m := &Model{
		ctx: ctx,
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			calls = append(calls, struct {
				command string
				payload map[string]any
			}{command: command, payload: args.(map[string]any)})
			switch command {
			case "maven_dependency_declaration":
				return json.RawMessage(`{
					"path": "/work/demo/pom.xml",
					"range": {"start":{"line":8,"col":4},"end":{"line":8,"col":20}}
				}`), nil
			case "maven_exclude_dependency":
				return json.RawMessage(`{}`), nil
			default:
				return nil, fmt.Errorf("unexpected RPC %s", command)
			}
		},
	}
	direct := mavenDependency{GroupID: "org.example", ArtifactID: "direct"}
	exclusion := mavenDependency{GroupID: "org.example", ArtifactID: "transitive"}

	location, err := m.mavenDependencyDeclaration(ctx, "pom.xml", direct)
	if err != nil {
		t.Fatal(err)
	}
	if location.Path != "/work/demo/pom.xml" || location.Range.Start.Line != 8 {
		t.Fatalf("declaration = %+v", location)
	}
	if err := m.excludeMavenDependency(ctx, "pom.xml", direct, exclusion); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("RPC calls = %+v", calls)
	}
	if calls[0].command != "maven_dependency_declaration" ||
		calls[0].payload["pom"] != "pom.xml" {
		t.Fatalf("declaration payload = %+v", calls[0])
	}
	directPayload := calls[1].payload["dependency"].(map[string]any)
	exclusionPayload := calls[1].payload["exclusion"].(map[string]any)
	if directPayload["groupId"] != "org.example" || directPayload["artifactId"] != "direct" ||
		exclusionPayload["groupId"] != "org.example" || exclusionPayload["artifactId"] != "transitive" {
		t.Fatalf("exclusion payload = %+v", calls[1].payload)
	}
}

func TestMavenAndGradleContextMenusExposeNativeActions(t *testing.T) {
	t.Run("Maven project", func(t *testing.T) {
		m := &Model{}
		project := &mavenProject{Pom: "pom.xml"}
		labels := buildToolMenuLabels(t, func(menu *ui.Menu) {
			m.mavenRowContextMenu(menu, mavenRow{
				kind: "project", project: project, key: "mvn:pom.xml",
			})
		})
		assertMenuLabels(t, labels,
			"Unlink Maven Project",
			"Open pom.xml",
			"Reload All Maven Projects",
			"Generate Sources and Update Folders",
			"Execute Maven Goal…",
			"Analyze Dependencies…",
			"Add Dependency…",
		)
	})

	t.Run("Maven dependencies", func(t *testing.T) {
		m := &Model{}
		project := &mavenProject{Pom: "pom.xml"}
		direct := mavenDependency{GroupID: "org.example", ArtifactID: "direct"}
		transitive := mavenDependency{GroupID: "org.example", ArtifactID: "transitive"}
		labels := buildToolMenuLabels(t, func(menu *ui.Menu) {
			m.mavenRowContextMenu(menu, mavenRow{
				kind: "dependency", project: project, dep: &direct, direct: &direct,
			})
		})
		assertMenuLabels(t, labels, "Jump to Source", "Analyze Dependencies…", "Add Dependency…", "Copy Coordinates")
		if slices.Contains(labels, "Exclude Dependency") {
			t.Fatalf("a direct dependency offered Exclude: %v", labels)
		}

		labels = buildToolMenuLabels(t, func(menu *ui.Menu) {
			m.mavenRowContextMenu(menu, mavenRow{
				kind: "dependency", project: project, dep: &transitive, direct: &direct,
			})
		})
		assertMenuLabels(t, labels, "Exclude Dependency", "Analyze Dependencies…", "Add Dependency…", "Copy Coordinates")
		if slices.Contains(labels, "Jump to Source") {
			t.Fatalf("a transitive dependency offered Jump to Source: %v", labels)
		}
	})

	t.Run("Gradle project and task", func(t *testing.T) {
		m := &Model{}
		buildFile := "build.gradle.kts"
		build := &gradleModel{
			Dir: "tools", Command: "gradle",
			Projects: []gradleProject{{
				Path: ":", Name: "tools", BuildFile: &buildFile,
				Tasks: []gradleTask{{Name: "test", Group: "verification"}},
			}},
		}
		project := &build.Projects[0]
		labels := buildToolMenuLabels(t, func(menu *ui.Menu) {
			m.gradleRowContextMenu(menu, gradleRow{
				kind: "gradleProject", build: build, project: project,
			})
		})
		assertMenuLabels(t, labels,
			"Open build.gradle.kts",
			"Unlink Gradle Project",
			"Reload Gradle Project",
			"Execute Gradle Task…",
			"Analyze Dependencies…",
		)

		task := &project.Tasks[0]
		labels = buildToolMenuLabels(t, func(menu *ui.Menu) {
			m.gradleRowContextMenu(menu, gradleRow{
				kind: "task", build: build, project: project, task: task,
			})
		})
		assertMenuLabels(t, labels, "Run 'tools [test]'", "Debug 'tools [test]'", "Create Run Configuration…")
	})
}

func TestInsertMavenDependencyPreservesXMLAndRejectsDirectDuplicates(t *testing.T) {
	input := mavenDependencyInput{
		group: "org.example", artifact: "new-lib", version: "2.0",
		scope: "test", optional: true,
	}
	tests := []struct {
		name string
		text string
	}{
		{
			name: "existing dependencies",
			text: `<project>
    <dependencies>
        <dependency>
            <groupId>org.example</groupId>
            <artifactId>old-lib</artifactId>
        </dependency>
    </dependencies>
</project>
`,
		},
		{
			name: "no dependencies",
			text: `<project>
    <modelVersion>4.0.0</modelVersion>
</project>
`,
		},
		{
			name: "one line project",
			text: `<project><modelVersion>4.0.0</modelVersion></project>`,
		},
		{
			name: "one line dependencies",
			text: `<project><dependencies></dependencies></project>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			at, inserted, err := insertMavenDependency(tc.text, input)
			if err != nil {
				t.Fatal(err)
			}
			patched := tc.text[:at] + inserted + tc.text[at:]
			if err := consumeXML(patched); err != nil {
				t.Fatalf("inserted POM is not valid XML: %v\n%s", err, patched)
			}
			for _, text := range []string{
				"<groupId>org.example</groupId>",
				"<artifactId>new-lib</artifactId>",
				"<version>2.0</version>",
				"<scope>test</scope>",
				"<optional>true</optional>",
			} {
				if !strings.Contains(patched, text) {
					t.Fatalf("inserted dependency lacks %s:\n%s", text, patched)
				}
			}
		})
	}

	duplicatePOM := `<project>
    <dependencies>
        <dependency><groupId>org.example</groupId><artifactId>new-lib</artifactId></dependency>
    </dependencies>
</project>`
	if _, _, err := insertMavenDependency(duplicatePOM, input); err == nil ||
		!strings.Contains(err.Error(), "already declared") {
		t.Fatalf("direct duplicate error = %v", err)
	}

	nonRootDeclarations := `<project>
    <dependencyManagement><dependencies>
        <dependency><groupId>org.example</groupId><artifactId>new-lib</artifactId><version>1.0</version></dependency>
    </dependencies></dependencyManagement>
    <build><plugins><plugin><dependencies>
        <dependency><groupId>org.example</groupId><artifactId>new-lib</artifactId></dependency>
    </dependencies></plugin></plugins></build>
</project>`
	if mavenDependencyAlreadyDeclared(nonRootDeclarations, "org.example", "new-lib") {
		t.Fatal("dependencyManagement or plugin dependencies were treated as a project dependency duplicate")
	}
}

func TestBuildToolErrorsOfferRefresh(t *testing.T) {
	t.Run("Maven", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		mavenError := "Maven sync failed"
		var callsMu sync.Mutex
		calls := 0
		m := &Model{
			ctx: ctx,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command != "build_sync" {
					return nil, fmt.Errorf("unexpected RPC %s", command)
				}
				callsMu.Lock()
				calls++
				callsMu.Unlock()
				return mustBuildToolJSON(t, buildModel{
					Maven: &mavenModel{SyncedAt: floatPointer(1)},
				}), nil
			},
			postHook: func(fn func()) { fn() },
		}
		m.setRoot("/work/demo")
		m.maven.model.Maven = &mavenModel{SyncErr: &mavenError}
		m.maven.visible = true
		m.DockShow("maven")
		tester := workbenchTester(t, m)
		if !tester.HasText("Maven sync failed") || !tester.HasText("Refresh") {
			t.Fatalf("Maven error refresh is not visible: %v", tester.Texts())
		}
		if err := tester.Click("Refresh"); err != nil {
			t.Fatal(err)
		}
		waitBuildToolTest(t, 2*time.Second, func() bool {
			callsMu.Lock()
			defer callsMu.Unlock()
			return calls == 1 && !m.maven.syncing
		})
	})

	t.Run("Gradle", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		buildError := "Gradle sync failed"
		var callsMu sync.Mutex
		calls := 0
		m := &Model{
			ctx: ctx,
			invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command != "build_sync" {
					return nil, fmt.Errorf("unexpected RPC %s", command)
				}
				callsMu.Lock()
				calls++
				callsMu.Unlock()
				return mustBuildToolJSON(t, buildModel{
					Gradle: []gradleModel{{Dir: "tools", SyncedAt: floatPointer(1)}},
				}), nil
			},
			postHook: func(fn func()) { fn() },
		}
		m.setRoot("/work/demo")
		m.maven.model.Gradle = []gradleModel{{Dir: "tools", SyncError: &buildError}}
		m.maven.gradleVisible = true
		m.DockShow("gradle")
		tester := workbenchTester(t, m)
		if !tester.HasText("Gradle sync failed") || !tester.HasText("Refresh") {
			t.Fatalf("Gradle error refresh is not visible: %v", tester.Texts())
		}
		if err := tester.Click("Refresh"); err != nil {
			t.Fatal(err)
		}
		waitBuildToolTest(t, 2*time.Second, func() bool {
			callsMu.Lock()
			defer callsMu.Unlock()
			return calls == 1 && !m.maven.syncing
		})
	})
}

func buildToolMenuLabels(t *testing.T, build func(*ui.Menu)) []string {
	t.Helper()
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Button(c, "Build tools").Menu(build)
	}, 520, 220)
	tester.Frame()
	if err := tester.Click("Build tools"); err != nil {
		t.Fatal(err)
	}
	return tester.Menu()
}

func assertMenuLabels(t *testing.T, got []string, want ...string) {
	t.Helper()
	for _, label := range want {
		if !slices.Contains(got, label) {
			t.Errorf("menu %v misses %q", got, label)
		}
	}
}

func mustBuildToolJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func waitBuildToolTest(t *testing.T, timeout time.Duration, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for a Maven/Gradle build-tool operation")
}

func floatPointer(value float64) *float64 {
	return &value
}

func payloadStringsEqual(value any, want []string) bool {
	got, ok := value.([]string)
	return ok && slices.Equal(got, want)
}

func consumeXML(text string) error {
	decoder := xml.NewDecoder(strings.NewReader(text))
	for {
		_, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
