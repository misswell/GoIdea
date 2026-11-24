package app

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestMavenDebugLaunchMatchesPinnedJDWPHooks(t *testing.T) {
	tests := []struct {
		name   string
		config openidea.RunConfig
		port   int
		check  func(t *testing.T, config openidea.RunConfig)
	}{
		{
			name: "Spring Boot",
			config: openidea.RunConfig{
				Kind: "maven", IsTest: false,
				Args: []string{"spring-boot:run"},
			},
			check: func(t *testing.T, config openidea.RunConfig) {
				t.Helper()
				value := debugPropertyValue(config.Args, "spring-boot.run.jvmArguments")
				if !strings.Contains(value, debugJDWPAgent) {
					t.Fatalf("Spring Boot JVM arguments = %q", value)
				}
			},
		},
		{
			name: "Surefire and Failsafe",
			config: openidea.RunConfig{
				Kind: "maven", IsTest: true,
				Args: []string{"verify"},
			},
			port: 5011,
			check: func(t *testing.T, config openidea.RunConfig) {
				t.Helper()
				for _, key := range []string{"maven.surefire.debug", "maven.failsafe.debug"} {
					value := debugPropertyValue(config.Args, key)
					if !strings.Contains(value, "address=127.0.0.1:5011") {
						t.Fatalf("%s = %q", key, value)
					}
				}
			},
		},
		{
			name: "in-process plugin",
			config: openidea.RunConfig{
				Kind: "maven",
				Args: []string{"tomcat7:run"},
			},
			check: func(t *testing.T, config openidea.RunConfig) {
				t.Helper()
				if !strings.Contains(config.Env["MAVEN_OPTS"], debugJDWPAgent) {
					t.Fatalf("MAVEN_OPTS = %q", config.Env["MAVEN_OPTS"])
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &Model{}
			config, err := m.debugMavenLaunch(test.config, test.port)
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, config)
		})
	}
}

func TestGradleDebugLaunchUsesPinnedScripts(t *testing.T) {
	var mu sync.Mutex
	var calls []struct {
		command string
		args    map[string]any
	}
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (stdjson.RawMessage, error) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, struct {
				command string
				args    map[string]any
			}{command, input.(map[string]any)})
			switch command {
			case "run_gradle_init_script":
				if !strings.Contains(input.(map[string]any)["text"].(string), "task.jvmArgs") {
					return nil, fmt.Errorf("boot script does not add JDWP")
				}
				return stdjson.RawMessage(`"/tmp/goidea-boot.gradle"`), nil
			case "run_gradle_jvm_args_script":
				return stdjson.RawMessage(`"/tmp/goidea-listener.gradle"`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	boot, err := m.debugGradleLaunch(context.Background(), openidea.RunConfig{
		Kind: "gradle",
		Args: []string{":app:bootRun"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasInitScript(boot.Args, "/tmp/goidea-boot.gradle") ||
		!containsString(boot.Args, debugGradleRunFlag) ||
		!containsString(boot.Args, debugGradleListenerFlag) {
		t.Fatalf("Gradle Boot debug arguments = %#v", boot.Args)
	}
	tests, err := m.debugGradleLaunch(context.Background(), openidea.RunConfig{
		Kind: "gradle",
		Args: []string{"test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tests.IsTest || !containsString(tests.Args, debugGradleRunFlag) ||
		!hasInitScript(tests.Args, "/tmp/goidea-listener.gradle") {
		t.Fatalf("Gradle test debug arguments = %#v", tests.Args)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("Gradle setup calls = %+v", calls)
	}
}

func TestBuildToolDebugIgnoresOldRunAndDeduplicatesPort(t *testing.T) {
	root := t.TempDir()
	configs := make(chan openidea.DebugLaunchConfig, 2)
	posted := make(chan struct{}, 16)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (stdjson.RawMessage, error) {
			switch command {
			case "java_debug_server":
				return stdjson.RawMessage(`4711`), nil
			case "debug_start":
				configs <- input.(map[string]any)["config"].(openidea.DebugLaunchConfig)
				return stdjson.RawMessage(`71`), nil
			case "debug_capabilities":
				return stdjson.RawMessage(`{}`), nil
			case "debug_set_function_breakpoints", "debug_set_data_breakpoints":
				return stdjson.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	m.run.trusted = true
	m.run.channel = 9
	m.debug.buildToolDebug = &debugBuildToolState{
		root: root, name: "Maven app", channel: 9, queued: map[int]bool{},
	}
	m.run.channel = 10
	m.acceptBuildToolDebugOutput("Listening for transport dt_socket at address: 5012")
	if len(m.debug.buildToolDebug.queued) != 0 {
		t.Fatal("JDWP output from an old Run channel reached the debugger")
	}
	m.run.channel = 9
	line := "Listening for transport dt_socket at address: 127.0.0.1:5012"
	m.acceptBuildToolDebugOutput(line)
	m.acceptBuildToolDebugOutput(line)
	if !m.debug.buildToolDebug.queued[5012] {
		t.Fatal("JDWP port was not queued")
	}
	select {
	case launch := <-configs:
		var extra map[string]any
		if launch.ExtraJSON == nil || stdjson.Unmarshal([]byte(*launch.ExtraJSON), &extra) != nil {
			t.Fatalf("attach launch has no valid extraJson: %+v", launch)
		}
		if extra["port"] != float64(5012) {
			t.Fatalf("JDWP attach port = %#v", extra["port"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("JDWP attach did not call debug_start")
	}
	deadline := time.After(2 * time.Second)
	for m.debug.starting {
		select {
		case <-posted:
		case <-deadline:
			t.Fatal("build-tool JDWP attach did not finish")
		}
	}
}

func TestBuildToolDebugUsesPinnedMavenTestPortOnForkBanner(t *testing.T) {
	root := t.TempDir()
	started := make(chan openidea.DebugLaunchConfig, 1)
	posted := make(chan struct{}, 16)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			posted <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (stdjson.RawMessage, error) {
			switch command {
			case "java_debug_server":
				return stdjson.RawMessage(`4711`), nil
			case "debug_start":
				started <- input.(map[string]any)["config"].(openidea.DebugLaunchConfig)
				return stdjson.RawMessage(`72`), nil
			case "debug_capabilities", "debug_set_function_breakpoints", "debug_set_data_breakpoints":
				return stdjson.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	m.setRoot(root)
	m.run.trusted = true
	m.run.channel = 3
	m.debug.buildToolDebug = &debugBuildToolState{
		root: root, name: "Tests", channel: 3,
		pinnedPort: 5099, queued: map[int]bool{},
	}
	m.acceptBuildToolDebugOutput("[INFO] T E S T S")
	select {
	case launch := <-started:
		var extra map[string]any
		if launch.ExtraJSON == nil || stdjson.Unmarshal([]byte(*launch.ExtraJSON), &extra) != nil {
			t.Fatal("pinned Maven attach has no extraJson")
		}
		if extra["port"] != float64(5099) {
			t.Fatalf("pinned Maven attach port = %#v", extra["port"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pinned Maven test port was not attached on the fork banner")
	}
}

func TestBuildToolDebugStateClearsOnRunExit(t *testing.T) {
	m := &Model{}
	m.run.awaiting = true
	m.run.runID = 71
	m.run.channel = 9
	m.debug.buildToolDebug = &debugBuildToolState{
		name: "Maven app", channel: 9, queued: map[int]bool{},
	}
	code := int32(0)

	m.acceptExit(openidea.RunExitedEvent{Run: 71, Code: &code})
	if m.debug.buildToolDebug != nil {
		t.Fatalf("build-tool debug state survived run exit: %+v", m.debug.buildToolDebug)
	}
}

func TestBuildToolDebugStateClearsWhenRunStops(t *testing.T) {
	stopped := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, _ any) (stdjson.RawMessage, error) {
			if command != "run_stop" {
				return nil, fmt.Errorf("unexpected command %s", command)
			}
			stopped <- struct{}{}
			return stdjson.RawMessage(`null`), nil
		},
	}
	m.run.running = true
	m.run.runID = 72
	m.run.channel = 10
	m.debug.buildToolDebug = &debugBuildToolState{
		name: "Gradle app", channel: 10, queued: map[int]bool{},
	}

	m.StopRun()
	if m.debug.buildToolDebug != nil {
		t.Fatalf("build-tool debug state survived run stop: %+v", m.debug.buildToolDebug)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("StopRun did not call run_stop")
	}
}
