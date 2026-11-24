package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"mayoi/internal/openidea"
)

func TestUpdateApplicationSpringBootPublishesResolvedOutput(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	published := make(chan struct{}, 1)
	buildConfig := mustJSON(t, map[string]any{
		"id": "build:boot", "name": "Build Boot", "kind": "shell",
		"command": "mvn", "args": []string{"-q", "-DskipTests", "compile"},
	})
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		mu.Lock()
		calls = append(calls, command)
		mu.Unlock()
		switch command {
		case "java_resolve_launch":
			if input.(map[string]any)["policy"] != "noBuild" {
				t.Fatalf("resources policy = %v", input.(map[string]any)["policy"])
			}
			return json.Marshal(openidea.JavaLaunchInfo{
				Build: strPtr("succeeded"), ClassPaths: []string{"/p/app/target/classes"},
				PathListSeparator: ":", JavaExec: strPtr("/jdk/bin/java"),
			})
		case "run_application_build":
			return buildConfig, nil
		case "run_start":
			code := int32(0)
			m.post(func() {
				m.acceptExit(openidea.RunExitedEvent{Run: 91, Code: &code})
			})
			return json.Marshal(openidea.RunInfo{
				ID: 91, ConfigID: "build:boot", Name: "Build Boot",
			})
		case "run_application_publish":
			args := input.(map[string]any)
			paths := args["classPaths"].([]string)
			if len(paths) != 1 || paths[0] != "/p/app/target/classes" ||
				args["mainClass"] != "demo.Application" {
				t.Fatalf("publish args = %#v", args)
			}
			published <- struct{}{}
			return json.RawMessage(`null`), nil
		}
		return json.RawMessage(`null`), nil
	}
	m.setRoot("/p")
	module := "app"
	boot := map[string]any{
		"id": "boot", "name": "Boot", "kind": "spring-boot", "command": "java",
		"args":   []string{"-cp", "${classpath}", "demo.Application", "--spring.profiles.active=dev"},
		"module": module,
	}
	raw := mustJSON(t, boot)
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = []openidea.RunConfig{*cfg}
	m.run.lastID = cfg.ID
	m.run.lastRaw = raw
	m.run.running = true
	if !m.UpdateApplication("resources") {
		t.Fatalf("Spring Boot update did not start: %s", m.run.err)
	}
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		var runErr string
		main.do(func() { runErr = m.run.err })
		t.Fatalf("Spring Boot update did not publish: %s", runErr)
	}
	mu.Lock()
	got := strings.Join(calls, ",")
	mu.Unlock()
	want := "java_resolve_launch,run_application_build,run_start,run_application_publish"
	if got != want {
		t.Fatalf("update calls = %s, want %s", got, want)
	}
	waitForSerialMainThread(t, main, 2*time.Second, func() bool {
		return !m.applicationUpdateBusy(cfg.ID)
	})
}

func TestUpdateApplicationSpringBootWithoutHotSwapStillPublishes(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	buildConfig := mustJSON(t, map[string]any{
		"id": "build:boot", "name": "Build Boot", "kind": "shell", "command": "true",
	})
	published := make(chan struct{}, 1)
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "java_resolve_launch":
			return json.Marshal(openidea.JavaLaunchInfo{
				Build: strPtr("succeeded"), ClassPaths: []string{"/classes"},
				PathListSeparator: ":", JavaExec: strPtr("/java"),
			})
		case "run_application_build":
			return buildConfig, nil
		case "run_start":
			code := int32(0)
			m.post(func() {
				m.acceptExit(openidea.RunExitedEvent{Run: 95, Code: &code})
			})
			return json.Marshal(openidea.RunInfo{ID: 95, ConfigID: "build:boot", Name: "Build Boot"})
		case "run_application_publish":
			published <- struct{}{}
			return json.RawMessage(`null`), nil
		}
		return json.RawMessage(`null`), nil
	}
	m.setRoot("/p")
	raw := mustJSON(t, map[string]any{
		"id": "boot", "name": "Boot", "kind": "spring-boot", "command": "java",
		"args": []string{"-cp", "${classpath}", "demo.Application"}, "module": "app",
	})
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = []openidea.RunConfig{*cfg}
	if !m.startApplicationUpdate(applicationUpdateSession{
		config: *cfg, debugging: true,
	}, "classes") {
		t.Fatal("Spring Boot update did not start")
	}
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("Spring Boot update did not publish its classes")
	}
	waitForSerialMainThread(t, main, 2*time.Second, func() bool {
		return !m.applicationUpdateBusy(cfg.ID) && len(m.notify.items) > 0
	})
	main.do(func() {
		last := m.notify.items[len(m.notify.items)-1]
		if last.title != "Resources updated" || !strings.Contains(last.body, "Restart the application") {
			t.Fatalf("unavailable HotSwap notification = %+v", last)
		}
	})
}

func TestUpdateApplicationPreventsOverlapAndUsesTomcatSync(t *testing.T) {
	t.Run("overlap", func(t *testing.T) {
		buildStarted := make(chan struct{}, 1)
		releaseBuild := make(chan struct{})
		var mu sync.Mutex
		buildCalls := 0
		buildConfig := mustJSON(t, map[string]any{
			"id": "build:boot", "name": "Build Boot", "kind": "shell", "command": "true",
		})
		main := &serialMainThread{}
		m := &Model{
			ctx:      context.Background(),
			postHook: main.post,
		}
		m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "java_resolve_launch":
				return json.Marshal(openidea.JavaLaunchInfo{
					Build: strPtr("succeeded"), ClassPaths: []string{"/classes"},
					PathListSeparator: ":", JavaExec: strPtr("/java"),
				})
			case "run_application_build":
				mu.Lock()
				buildCalls++
				mu.Unlock()
				select {
				case buildStarted <- struct{}{}:
				default:
				}
				<-releaseBuild
				return buildConfig, nil
			case "run_start":
				code := int32(0)
				m.post(func() {
					m.acceptExit(openidea.RunExitedEvent{Run: 92, Code: &code})
				})
				return json.Marshal(openidea.RunInfo{ID: 92, ConfigID: "build:boot", Name: "Build Boot"})
			}
			return json.RawMessage(`null`), nil
		}
		m.setRoot("/p")
		raw := mustJSON(t, map[string]any{
			"id": "boot", "name": "Boot", "kind": "spring-boot", "command": "java",
			"args": []string{"-cp", "${classpath}", "demo.Application"}, "module": "app",
		})
		cfg, _ := openidea.DecodeRunConfig(raw)
		m.run.configs = []openidea.RunConfig{*cfg}
		m.run.lastRaw = raw
		m.run.running = true
		if !m.UpdateApplication("resources") {
			t.Fatal("first update did not start")
		}
		select {
		case <-buildStarted:
		case <-time.After(time.Second):
			t.Fatal("first update did not reach its build")
		}
		var overlapped bool
		main.do(func() {
			overlapped = m.UpdateApplication("resources")
		})
		if overlapped {
			t.Fatal("a second update overlapped the first")
		}
		close(releaseBuild)
		waitForDynamicAction(t, 2*time.Second, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return buildCalls == 1 && !m.applicationUpdateBusy("boot")
		})
	})

	t.Run("Tomcat sync", func(t *testing.T) {
		var mu sync.Mutex
		var calls []string
		main := &serialMainThread{}
		m := &Model{
			ctx:      context.Background(),
			postHook: main.post,
			invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
				mu.Lock()
				calls = append(calls, command)
				mu.Unlock()
				switch command {
				case "run_tomcat_artifacts":
					return mustJSON(t, []map[string]any{{
						"id": "maven:app/pom.xml", "name": "app",
						"module": "app", "tool": "maven",
					}}), nil
				case "java_resolve_launch":
					args := input.(map[string]any)
					if args["module"] != "app" || args["policy"] != "make" {
						t.Fatalf("Tomcat resolve args = %#v", args)
					}
					return json.Marshal(openidea.JavaLaunchInfo{
						Build: strPtr("succeeded"), ClassPaths: []string{"app/target/classes"},
						PathListSeparator: ":", JavaExec: strPtr("/jdk/bin/java"),
					})
				case "run_tomcat_sync":
					args := input.(map[string]any)
					if args["module"] != "app" || args["resourcesOnly"] != true {
						t.Fatalf("Tomcat sync args = %#v", args)
					}
					return json.RawMessage(`true`), nil
				}
				return json.RawMessage(`null`), nil
			},
		}
		m.setRoot("/p")
		raw := mustJSON(t, map[string]any{
			"id": "tomcat", "name": "Tomcat", "kind": "tomcat",
			"command": "java", "args": []string{},
			"params": map[string]any{"deployment": "artifact"},
		})
		cfg, _ := openidea.DecodeRunConfig(raw)
		m.run.configs = []openidea.RunConfig{*cfg}
		m.run.lastRaw = raw
		m.run.running = true
		if !m.UpdateApplication("resources") {
			t.Fatal("Tomcat update did not start")
		}
		waitForDynamicAction(t, 2*time.Second, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(calls) == 3 && !m.applicationUpdateBusy("tomcat")
		})
		mu.Lock()
		got := strings.Join(calls, ",")
		mu.Unlock()
		if got != "run_tomcat_artifacts,java_resolve_launch,run_tomcat_sync" {
			t.Fatalf("Tomcat sync calls = %s", got)
		}
	})
}

func TestUpdateApplicationTomcatClassesFallbackAndResourcesSkipsHotSwap(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    string
		updates []string
	}{
		{name: "classes fall back to redeploy", mode: "classes", updates: []string{"redeploy"}},
		{name: "resources skip HotSwap", mode: "resources"},
	} {
		t.Run(test.name, func(t *testing.T) {
			main := &serialMainThread{}
			m := &Model{
				ctx:      context.Background(),
				postHook: main.post,
			}
			m.setRoot("/p")
			raw := mustJSON(t, map[string]any{
				"id": "tomcat", "name": "Tomcat", "kind": "tomcat", "args": []string{},
				"params": map[string]any{"deployment": "artifact"},
			})
			cfg, err := openidea.DecodeRunConfig(raw)
			if err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var updates []string
			m.invokeHook = func(ctx context.Context, command string, input any) (json.RawMessage, error) {
				if command == "run_tomcat_update" {
					mu.Lock()
					updates = append(updates, input.(map[string]any)["mode"].(string))
					mu.Unlock()
					return json.RawMessage(`null`), nil
				}
				if command == "run_tomcat_artifacts" {
					return mustJSON(t, []map[string]any{{
						"id": "maven:app", "name": "app", "module": "app", "tool": "maven",
					}}), nil
				}
				if command == "java_resolve_launch" {
					return json.Marshal(openidea.JavaLaunchInfo{
						Build: strPtr("succeeded"), ClassPaths: []string{"app/target/classes"},
						PathListSeparator: ":", JavaExec: strPtr("/java"),
					})
				}
				if command == "run_tomcat_sync" {
					return json.RawMessage(`true`), nil
				}
				return json.RawMessage(`null`), nil
			}
			if err := m.updateTomcatApplication(context.Background(), cfg, raw, true, test.mode); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			got := append([]string(nil), updates...)
			mu.Unlock()
			if strings.Join(got, ",") != strings.Join(test.updates, ",") {
				t.Fatalf("Tomcat updates = %#v, want %#v", got, test.updates)
			}
			if test.mode == "resources" && m.debug.err != "" {
				t.Fatalf("resources invoked HotSwap: %s", m.debug.err)
			}
		})
	}
}

func TestUpdateApplicationTomcatBuildAndRedeployFallback(t *testing.T) {
	var mu sync.Mutex
	var modes []string
	buildConfig := mustJSON(t, map[string]any{
		"id": "tomcat-build:tomcat", "name": "Build Tomcat", "kind": "shell",
		"command": "mvn", "args": []string{"package"},
	})
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "run_tomcat_artifacts":
			return json.RawMessage(`[]`), nil
		case "run_tomcat_build":
			return buildConfig, nil
		case "run_start":
			info, _ := json.Marshal(openidea.RunInfo{ID: 94, ConfigID: "tomcat-build:tomcat", Name: "Build Tomcat"})
			code := int32(0)
			m.post(func() { m.acceptExit(openidea.RunExitedEvent{Run: 94, Code: &code}) })
			return info, nil
		case "run_tomcat_update":
			if input.(map[string]any)["mode"] != "redeploy" {
				t.Fatalf("Tomcat update args = %#v", input)
			}
			mu.Lock()
			modes = append(modes, "redeploy")
			mu.Unlock()
			return json.RawMessage(`null`), nil
		}
		return json.RawMessage(`null`), nil
	}
	m.setRoot("/p")
	raw := mustJSON(t, map[string]any{
		"id": "tomcat", "name": "Tomcat", "kind": "tomcat",
		"command": "java", "args": []string{},
		"params": map[string]any{"deployment": "artifact"},
	})
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = []openidea.RunConfig{*cfg}
	m.run.lastID = cfg.ID
	m.run.lastRaw = raw
	m.run.running = true
	if !m.UpdateApplication("redeploy") {
		t.Fatal("Tomcat redeploy did not start")
	}
	waitForDynamicAction(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(modes) == 1 && !m.applicationUpdateBusy(cfg.ID)
	})
}

func TestApplicationUpdateSessionsIncludeAllLiveRunsAndDebugSessions(t *testing.T) {
	m := &Model{}
	m.setRoot("/p")
	bootRaw := mustJSON(t, map[string]any{
		"id": "boot", "name": "Boot", "kind": "spring-boot",
		"args": []string{"-cp", "${classpath}", "demo.Application"},
	})
	boot, _ := openidea.DecodeRunConfig(bootRaw)
	tomcatRaw := mustJSON(t, map[string]any{
		"id": "tomcat", "name": "Tomcat", "kind": "tomcat", "args": []string{},
		"params": map[string]any{"deployment": "artifact"},
	})
	tomcat, _ := openidea.DecodeRunConfig(tomcatRaw)
	serviceRaw := mustJSON(t, map[string]any{
		"id": "service", "name": "Service", "kind": "tomcat", "args": []string{},
		"params": map[string]any{"deployment": "artifact"},
	})
	service, _ := openidea.DecodeRunConfig(serviceRaw)
	debugRaw := mustJSON(t, map[string]any{
		"id": "debug", "name": "Debug", "kind": "tomcat", "args": []string{},
		"params": map[string]any{"deployment": "artifact"},
	})
	debug, _ := openidea.DecodeRunConfig(debugRaw)
	m.run.configs = []openidea.RunConfig{*boot, *tomcat, *service, *debug}
	m.run.lastID, m.run.lastRaw, m.run.running = "boot", bootRaw, true
	m.run.runID = 11
	m.run.children = []*runChild{{config: *tomcat, running: true, runID: 21}}
	m.services.children = []*runChild{{config: *service, running: true, runID: 22}}
	m.debug.sessions = map[uint32]*debugSessionState{
		31: {session: 31, lastID: "boot", lastRaw: bootRaw, paused: false},
		32: {session: 32, lastID: "debug", lastRaw: debugRaw, paused: true},
	}
	m.debug.sessionOrder = []uint32{31, 32}

	sessions := m.applicationUpdateSessions()
	if len(sessions) != 4 {
		t.Fatalf("update sessions = %#v", sessions)
	}
	byID := map[string]applicationUpdateSession{}
	for _, session := range sessions {
		byID[session.config.ID] = session
	}
	if !byID["boot"].debugging || byID["boot"].runID == 0 || byID["boot"].debugSessionID != 31 {
		t.Fatalf("primary Boot was not marked as debugged: %+v", byID["boot"])
	}
	if byID["tomcat"].runID != 21 || !byID["tomcat"].runChild ||
		byID["service"].runID != 22 || !byID["service"].runChild {
		t.Fatalf("run children lost their identity: %#v", sessions)
	}
	if byID["debug"].debugSessionID != 32 {
		t.Fatalf("debug session lost its identity: %+v", byID["debug"])
	}
}

func TestApplicationUpdateMenuUsesHeaderAndDisablesUnsupportedModes(t *testing.T) {
	remote := "remote"
	session := applicationUpdateSession{
		config: openidea.RunConfig{
			Kind: "tomcat", Target: &remote,
			Params: map[string]any{"deployment": "war"},
		},
	}
	items := applicationUpdateMenuItems(&Model{}, session, applicationUpdateChoices(session))
	if len(items) != 6 || items[0].Label != "Update Application" || !items[0].Disabled {
		t.Fatalf("update menu header = %#v", items)
	}
	if items[1].Type != mygo.MenuItemSeparator {
		t.Fatalf("second item is not a separator: %#v", items[1])
	}
	if !items[2].Disabled || !items[3].Disabled {
		t.Fatalf("WAR resources/classes should be disabled: %#v", items)
	}
}

func TestUpdateApplicationsOnDeactivationUpdatesEveryEligibleRun(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	m.setRoot("/p")
	firstRaw := mustJSON(t, map[string]any{
		"id": "first", "name": "First", "kind": "tomcat", "args": []string{},
		"params": map[string]any{"deployment": "artifact", "onDeactivation": "resources"},
	})
	secondRaw := mustJSON(t, map[string]any{
		"id": "second", "name": "Second", "kind": "tomcat", "args": []string{},
		"params": map[string]any{"deployment": "artifact", "onDeactivation": "resources"},
	})
	first, _ := openidea.DecodeRunConfig(firstRaw)
	second, _ := openidea.DecodeRunConfig(secondRaw)
	m.run.configs = []openidea.RunConfig{*first, *second}
	m.run.children = []*runChild{
		{config: *first, running: true, runID: 71},
		{config: *second, running: true, runID: 72},
	}
	var mu sync.Mutex
	synced := map[string]bool{}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "run_tomcat_artifacts":
			return mustJSON(t, []map[string]any{{
				"id": "maven:app", "name": "app", "module": "app", "tool": "maven",
			}}), nil
		case "java_resolve_launch":
			return json.Marshal(openidea.JavaLaunchInfo{
				Build: strPtr("succeeded"), ClassPaths: []string{"app/target/classes"},
				PathListSeparator: ":", JavaExec: strPtr("/jdk/bin/java"),
			})
		case "run_tomcat_sync":
			args := input.(map[string]any)
			cfg, _ := openidea.DecodeRunConfig(args["config"].(json.RawMessage))
			mu.Lock()
			synced[cfg.ID] = true
			mu.Unlock()
			return json.RawMessage(`true`), nil
		}
		return json.RawMessage(`null`), nil
	}
	m.updateApplicationsOnDeactivation()
	waitForDynamicAction(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(synced) == 2 && !m.applicationUpdateBusy("first") &&
			!m.applicationUpdateBusy("second")
	})
}

func TestApplicationRestartRestartsIndependentRunChildren(t *testing.T) {
	for _, test := range []struct {
		name    string
		service bool
		oldID   uint32
		newID   uint32
	}{
		{name: "Run child", oldID: 601, newID: 611},
		{name: "Services child", service: true, oldID: 602, newID: 612},
	} {
		t.Run(test.name, func(t *testing.T) {
			main := &serialMainThread{}
			m := &Model{
				ctx:      context.Background(),
				postHook: main.post,
			}
			root := t.TempDir()
			m.setRoot(root)
			raw := mustJSON(t, map[string]any{
				"id": "service", "name": "Service", "kind": "shell", "command": "true",
			})
			cfg, err := openidea.DecodeRunConfig(raw)
			if err != nil {
				t.Fatal(err)
			}
			old := &runChild{
				runID: test.oldID, name: cfg.Name, config: *cfg,
				service: test.service, root: root, running: true,
				done: make(chan error, 1),
			}
			if test.service {
				m.services.children = []*runChild{old}
			} else {
				m.run.children = []*runChild{old}
			}
			m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
				switch command {
				case "run_stop":
					if id := input.(map[string]any)["id"].(uint32); id != test.oldID {
						t.Fatalf("stopped run %d, want %d", id, test.oldID)
					}
					main.do(func() { m.finishRunChild(old, nil, "") })
					return json.RawMessage(`null`), nil
				case "run_start":
					return json.Marshal(openidea.RunInfo{
						ID: test.newID, ConfigID: cfg.ID, Name: cfg.Name, Running: true,
					})
				}
				return json.RawMessage(`null`), nil
			}
			if err := m.restartApplication(root, applicationUpdateSession{
				config: *cfg, runID: test.oldID, runChild: true,
			}); err != nil {
				t.Fatal(err)
			}
			main.do(func() {
				if !old.finished {
					t.Fatal("old child was not retired")
				}
				started := m.childByRun(test.newID)
				if started == nil || !started.running || started.service != test.service {
					t.Fatalf("new child = %+v", started)
				}
			})
		})
	}
}

func TestApplicationRestartStopsPrimaryRunAndDebugBeforeRestartingDebug(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	root := t.TempDir()
	m.setRoot(root)
	m.run.trusted = true
	raw := mustJSON(t, map[string]any{
		"id": "app", "name": "Application", "kind": "application", "command": "java",
		"args": []string{"-cp", "${classpath}", "demo.Application"}, "module": "app",
	})
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = []openidea.RunConfig{*cfg}
	m.run.lastID, m.run.lastRaw = cfg.ID, raw
	m.run.runID, m.run.channel = 701, 71
	m.run.running, m.run.awaiting = true, true
	m.debug.active, m.debug.session = 702, 702
	m.debug.sessions = map[uint32]*debugSessionState{
		702: {session: 702, lastID: cfg.ID, lastRaw: raw, tabName: cfg.Name},
	}
	m.debug.sessionOrder = []uint32{702}
	preparationStarted := make(chan struct{}, 1)
	releasePreparation := make(chan struct{})
	var mu sync.Mutex
	var stops []string
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "run_stop":
			mu.Lock()
			stops = append(stops, "run")
			mu.Unlock()
			main.do(func() {
				code := int32(0)
				m.acceptExit(openidea.RunExitedEvent{Run: input.(map[string]any)["id"].(uint32), Code: &code})
			})
			return json.RawMessage(`null`), nil
		case "debug_stop":
			mu.Lock()
			stops = append(stops, "debug")
			mu.Unlock()
			return json.RawMessage(`null`), nil
		case "java_resolve_launch":
			preparationStarted <- struct{}{}
			<-releasePreparation
			return nil, errors.New("test debug preparation stopped")
		}
		return json.RawMessage(`null`), nil
	}
	if err := m.restartApplication(root, applicationUpdateSession{
		config: *cfg, debugging: true, runID: m.run.runID, debugSessionID: 702,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-preparationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("restart did not start a debug session")
	}
	main.do(func() {
		if !m.debug.starting {
			t.Fatal("restarted debug session did not enter preparation")
		}
	})
	close(releasePreparation)
	waitForSerialMainThread(t, main, 2*time.Second, func() bool { return !m.debug.starting })
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(stops, ",") != "debug,run" && strings.Join(stops, ",") != "run,debug" {
		t.Fatalf("restart stops = %#v", stops)
	}
}

func TestApplicationRestartStopsEvenWhenDebugSessionIsNotSelected(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	root := t.TempDir()
	m.setRoot(root)
	m.run.trusted = true
	raw := mustJSON(t, map[string]any{
		"id": "app", "name": "Application", "kind": "application", "command": "java",
		"args": []string{"-cp", "${classpath}", "demo.Application"}, "module": "app",
	})
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.debug.active, m.debug.session = 801, 801
	m.debug.sessions = map[uint32]*debugSessionState{
		801: {session: 801, lastID: "first", tabName: "First"},
		802: {session: 802, lastID: cfg.ID, lastRaw: raw, tabName: cfg.Name},
	}
	m.debug.sessionOrder = []uint32{801, 802}
	stopped := make(chan uint32, 1)
	preparationStarted := make(chan struct{}, 1)
	releasePreparation := make(chan struct{})
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "debug_stop":
			stopped <- input.(map[string]any)["session"].(uint32)
			return json.RawMessage(`null`), nil
		case "java_resolve_launch":
			preparationStarted <- struct{}{}
			<-releasePreparation
			return nil, errors.New("test debug preparation stopped")
		}
		return json.RawMessage(`null`), nil
	}
	if err := m.restartApplication(root, applicationUpdateSession{
		config: *cfg, debugging: true, debugSessionID: 802,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-stopped:
		if id != 802 {
			t.Fatalf("stopped debug session %d, want 802", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restart did not stop the selected debug session")
	}
	select {
	case <-preparationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("restart did not start the selected debug session")
	}
	close(releasePreparation)
	waitForSerialMainThread(t, main, 2*time.Second, func() bool { return !m.debug.starting })
}

func TestApplicationRestartRejectsMissingSessionsAndCancelledRoot(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	root := t.TempDir()
	m.setRoot(root)
	cfg := openidea.RunConfig{ID: "app", Name: "Application", Kind: "application"}
	for _, test := range []struct {
		name    string
		session applicationUpdateSession
		want    string
	}{
		{
			name:    "missing run child",
			session: applicationUpdateSession{config: cfg, runID: 901, runChild: true},
			want:    "run session is no longer available",
		},
		{
			name:    "missing debug session",
			session: applicationUpdateSession{config: cfg, debugSessionID: 902},
			want:    "debug session is no longer available",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := m.restartApplication(root, test.session)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("restart error = %v, want %q", err, test.want)
			}
		})
	}

	otherRoot := t.TempDir()
	m.setRoot(otherRoot)
	started := time.Now()
	if m.waitApplicationUpdate(root, func() bool { return false }) {
		t.Fatal("wait completed after the project root changed")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancelled restart waited %s", elapsed)
	}
}

func TestUpdateApplicationPoliciesChooserAndVisibility(t *testing.T) {
	remote := "remote"
	for _, test := range []struct {
		name string
		cfg  openidea.RunConfig
		mode string
		want bool
	}{
		{"Spring resources", openidea.RunConfig{Kind: "spring-boot"}, "resources", true},
		{"Spring redeploy", openidea.RunConfig{Kind: "spring-boot"}, "redeploy", false},
		{"Tomcat redeploy", openidea.RunConfig{Kind: "tomcat"}, "redeploy", true},
		{"remote application", openidea.RunConfig{Kind: "spring-boot", Target: &remote}, "restart", false},
		{"Tomcat WAR classes", openidea.RunConfig{
			Kind: "tomcat", Params: map[string]any{"deployment": "war"},
		}, "classes", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := applicationUpdateAllowed(&test.cfg, test.mode); got != test.want {
				t.Fatalf("allowed = %v, want %v", got, test.want)
			}
		})
	}

	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	m.setRoot("/p")
	raw := mustJSON(t, map[string]any{
		"id": "boot", "name": "Boot", "kind": "spring-boot", "command": "java",
		"args":   []string{"-cp", "${classpath}", "demo.Application"},
		"params": map[string]any{"onUpdate": "ask"},
	})
	cfg, _ := openidea.DecodeRunConfig(raw)
	m.run.configs = []openidea.RunConfig{*cfg}
	m.run.lastRaw = raw
	m.run.running = true
	if !m.ActionEnabled("run.updateApplication") {
		t.Fatal("Update Application is disabled for a live Spring Boot run")
	}
	m.UpdateApplication(nil)
	if len(m.notify.items) == 0 {
		t.Fatal("ask policy did not open the update chooser")
	}
	last := m.notify.items[len(m.notify.items)-1]
	if last.title != "Update Application" || len(last.actions) != 3 {
		t.Fatalf("application chooser = %+v", last)
	}
	for _, action := range last.actions {
		if strings.Contains(strings.ToLower(action.label), "redeploy") {
			t.Fatal("Spring Boot chooser offered Redeploy")
		}
	}

	m.run.running = false
	if m.ActionEnabled("run.updateApplication") {
		t.Fatal("Update Application stayed enabled after the session ended")
	}
}
