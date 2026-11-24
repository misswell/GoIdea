package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func strPtr(s string) *string { return &s }

func TestDebugUsesBackendWhileJavaBusy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Model{ctx: ctx, ready: &Readiness{Backend: true, Project: true, JavaDetail: "busy"}}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	finished := make(chan string, 1)
	m.postHook = func(fn func()) {
		fn()
		if !m.debug.starting {
			finished <- m.debug.err
		}
	}
	m.invokeHook = func(_ context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd != "java_resolve_launch" || args.(map[string]any)["module"] != "web" {
			return nil, fmt.Errorf("unexpected Java preparation request %s: %v", cmd, args)
		}
		return nil, errors.New("backend launch result")
	}
	cfg, err := openidea.DecodeRunConfig(runConfigJSON(t, map[string]any{
		"id": "app", "name": "BootstrapApplication", "kind": "application",
		"command": "java", "args": []string{"-cp", "${classpath}", "example.BootstrapApplication"}, "module": "web",
	}))
	if err != nil {
		t.Fatal(err)
	}
	m.DebugRunConfig(cfg)
	select {
	case got := <-finished:
		if got != "backend launch result" {
			t.Fatalf("debug preparation error = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("Debug bypassed backend Java preparation: %s", m.debug.err)
	}
}

func TestDebugStopCancelsJavaPreparation(t *testing.T) {
	m := &Model{ctx: context.Background(), postHook: func(fn func()) { fn() }}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	waiting := make(chan uint32, 1)
	cancelled := make(chan uint32, 1)
	m.invokeHook = func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		params := args.(map[string]any)
		switch cmd {
		case "java_resolve_launch":
			waiting <- params["request"].(uint32)
			<-ctx.Done()
			return nil, ctx.Err()
		case "cancel_request":
			cancelled <- params["id"].(uint32)
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected Java preparation request %s", cmd)
		}
	}
	m.DebugRunConfig(&openidea.RunConfig{
		Name: "Application", Kind: "application", Command: "java", Module: strPtr("web"),
		Args: []string{"-cp", "${classpath}", "example.Application"},
	})
	t.Cleanup(m.StopDebug)
	var request uint32
	select {
	case request = <-waiting:
	case <-time.After(time.Second):
		t.Fatal("debug preparation did not reach the backend")
	}
	m.StopDebug()
	select {
	case got := <-cancelled:
		if request == 0 || got != request {
			t.Fatalf("cancelled request = %d, want %d", got, request)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the backend preparation request")
	}
}

func TestDebugStopRetiresAnActiveSession(t *testing.T) {
	ctx := context.Background()
	stopped := make(chan uint32, 1)
	cleared := make(chan struct{}, 1)
	m := &Model{ctx: ctx}
	m.postHook = func(fn func()) {
		fn()
		if !m.debug.stopping && m.debug.session == 0 {
			select {
			case cleared <- struct{}{}:
			default:
			}
		}
	}
	m.invokeHook = func(_ context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd != "debug_stop" {
			return nil, fmt.Errorf("unexpected debug request %s", cmd)
		}
		stopped <- args.(map[string]any)["session"].(uint32)
		return nil, nil
	}
	m.debug.starting = true
	m.debug.session = 11

	m.StopDebug()
	if m.debug.starting || !m.debug.stopping {
		t.Fatalf("Stop state = starting %v, stopping %v", m.debug.starting, m.debug.stopping)
	}
	select {
	case session := <-stopped:
		if session != 11 {
			t.Fatalf("debug_stop session = %d, want 11", session)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not call debug_stop")
	}
	select {
	case <-cleared:
	case <-time.After(time.Second):
		t.Fatal("Stop did not clear the active session")
	}

	m.acceptDebugEvent(11, json.RawMessage(`{"kind":"started"}`))
	if m.debug.session != 0 || !m.debug.ended {
		t.Fatalf("a retired event revived the session: %+v", m.debug)
	}
}

func TestDebugTerminationClearsStartingState(t *testing.T) {
	for _, kind := range []string{"terminated", "exited"} {
		t.Run(kind, func(t *testing.T) {
			m := &Model{ctx: context.Background()}
			m.debug.starting = true
			m.acceptDebugEvent(17, json.RawMessage(fmt.Sprintf(`{"kind":%q}`, kind)))
			if m.debug.starting || m.debug.session != 0 || !m.debug.ended || m.debug.retiredSession != 17 {
				t.Fatalf("early %s left launch state active: %+v", kind, m.debug)
			}
		})
	}
}

func TestJavaDebugProjectBusy(t *testing.T) {
	server := func(status string, languages ...string) lspServerState {
		return lspServerState{Status: status, Languages: languages}
	}
	if !javaDebugProjectBusy([]lspServerState{server("busy", "java")}) {
		t.Fatal("a busy Java server must be waited for")
	}
	if !javaDebugProjectBusy([]lspServerState{server("starting", "java")}) {
		t.Fatal("a starting Java server must be waited for")
	}
	if javaDebugProjectBusy([]lspServerState{server("ready", "java")}) {
		t.Fatal("a ready Java server is not an import wait state")
	}
	if javaDebugProjectBusy([]lspServerState{server("crashed", "java")}) {
		t.Fatal("a failed Java server must keep the backend's precise error")
	}
	if javaDebugProjectBusy([]lspServerState{server("busy", "yaml")}) {
		t.Fatal("a non-Java server cannot hold Java Debug back")
	}
}

func TestJavaDebugPreparationRetriesTransientCancellation(t *testing.T) {
	root := t.TempDir()
	calls := 0
	var requests []uint32
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "lsp_servers":
			return json.RawMessage(`[]`), nil
		case "java_resolve_launch":
			calls++
			requests = append(requests, input.(map[string]any)["request"].(uint32))
			if calls == 1 {
				return nil, &openidea.Error{Code: -32603, Kind: "cancelled", Message: "cancelled"}
			}
			return json.Marshal(openidea.JavaLaunchInfo{
				Build:             strPtr("succeeded"),
				ProjectRoot:       &root,
				ClassPaths:        []string{filepath.Join(root, "target", "classes")},
				ModulePaths:       []string{},
				PathListSeparator: ":",
				JavaExec:          strPtr("/jdk/bin/java"),
			})
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}
	rc := &openidea.RunConfig{
		Name: "Application", Kind: "application", Command: "java", Module: strPtr(""),
		Args: []string{"-cp", "${classpath}", "example.Application"},
	}
	prepared, err := m.prepareJavaDebugLaunch(context.Background(), rc, root)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(requests) != 2 || requests[0] == requests[1] {
		t.Fatalf("preparation calls = %d, request ids = %v", calls, requests)
	}
	if prepared == nil || prepared.Parsed.MainClass != "example.Application" {
		t.Fatalf("prepared launch = %+v", prepared)
	}
}

func TestJavaDebugPreparationStopsWhenCallerCancels(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { fn() },
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "java_resolve_launch" {
			return nil, fmt.Errorf("unexpected command %s", command)
		}
		calls++
		cancel()
		return nil, &openidea.Error{Code: -32603, Kind: "cancelled", Message: "cancelled"}
	}
	rc := &openidea.RunConfig{
		Name: "Application", Kind: "application", Command: "java", Module: strPtr(""),
		Args: []string{"-cp", "${classpath}", "example.Application"},
	}
	started := time.Now()
	_, err := m.prepareJavaDebugLaunch(ctx, rc, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("preparation error = %v, want context cancellation", err)
	}
	if calls != 1 {
		t.Fatalf("preparation calls = %d, want 1", calls)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("caller cancellation took %s", elapsed)
	}
}

func TestJavaTargetForMavenLayout(t *testing.T) {
	root := "/p"
	text := "package demo;\n\npublic class Hello {\n"
	target, ok := javaTargetFor(root, "/p/src/main/java/demo/Hello.java", text)
	if !ok {
		t.Fatal("main class not found")
	}
	if target.module != "" || target.test {
		t.Fatalf("module = %q, test = %v", target.module, target.test)
	}
	if target.mainClass != "demo.Hello" {
		t.Fatalf("mainClass = %q", target.mainClass)
	}
}

func TestJavaTargetForModuleAndTest(t *testing.T) {
	main, ok := javaTargetFor("/p", "/p/app/src/main/java/demo/App.java", "package demo;\nclass App {}\n")
	if !ok || main.module != "app" || main.test {
		t.Fatalf("main = %+v, ok = %v", main, ok)
	}
	test, ok := javaTargetFor("/p", "/p/app/src/test/java/demo/AppTest.java", "package demo;\nclass AppTest {}\n")
	if !ok || test.module != "app" || !test.test {
		t.Fatalf("test = %+v, ok = %v", test, ok)
	}
}

func TestJavaTargetRejectsNonJava(t *testing.T) {
	if _, ok := javaTargetFor("/p", "/p/README.md", "hello"); ok {
		t.Fatal("a markdown file is not a Java target")
	}
}

func TestJavaPackage(t *testing.T) {
	cases := map[string]string{
		"package demo.app;\n":        "demo.app",
		"// x\npackage a ;\n":        "a",
		"class NoPackage {}":         "",
		"  package   spaced.name;  ": "spaced.name",
	}
	for text, want := range cases {
		if got := javaPackage(text); got != want {
			t.Errorf("javaPackage(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestJavaProjectName(t *testing.T) {
	dir := t.TempDir()
	pom := `<project><modelVersion>4.0.0</modelVersion>
  <parent><groupId>x</groupId><artifactId>parent-id</artifactId></parent>
  <artifactId>child-id</artifactId>
</project>`
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := javaProjectName(dir); got != "child-id" {
		t.Fatalf("artifactId = %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".project"), []byte("<projectDescription><name>eclipse-name</name></projectDescription>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := javaProjectName(dir); got != "eclipse-name" {
		t.Fatalf(".project name = %q", got)
	}
	empty := t.TempDir()
	if got := javaProjectName(empty); got != filepath.Base(empty) {
		t.Fatalf("folder name = %q", got)
	}
}

func TestJoinJavaArgs(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"-Xmx1g", "--add-opens", "java.base/java.lang=ALL-UNNAMED"}, "-Xmx1g --add-opens java.base/java.lang=ALL-UNNAMED"},
		{[]string{"a b", "", `c"d`, `e\f`}, `"a b" "" "c\"d" e\f`},
		{[]string{`p"q\r`}, `"p\"q\\r"`},
	}
	for _, c := range cases {
		if got := adapter.JoinJavaArgs(c.in); got != c.want {
			t.Errorf("JoinJavaArgs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJavaDebugLaunch(t *testing.T) {
	m := &Model{invokeHook: func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd == "java_debug_server" {
			return json.Marshal(5005)
		}
		return nil, errors.New("unexpected invoke: " + cmd)
	}}
	exec := "/jdk/bin/java"
	rc := &openidea.RunConfig{
		Name: "Hello", Kind: "application", Command: "java", Module: strPtr(""),
		Args: []string{"-cp", "${classpath}", "demo.Hello"},
	}
	prepared := &adapter.PreparedJavaLaunch{
		Module:   "",
		Parsed:   adapter.ParseJavaArgs(rc.Args),
		Command:  exec,
		Resolved: []string{"-cp", "/p/target/classes", "demo.Hello"},
		Info: openidea.JavaLaunchInfo{
			ClassPaths:        []string{"/p/target/classes"},
			ModulePaths:       nil,
			PathListSeparator: ":",
			JavaExec:          &exec,
			ProjectRoot:       strPtr("/p"),
		},
	}
	launch, err := m.javaDebugLaunch(context.Background(), rc, "/p", prepared, nil)
	if err != nil {
		t.Fatal(err)
	}
	if launch.Adapter != "java-debug" || launch.Request != "launch" || launch.Cwd != "/p" {
		t.Fatalf("launch = %+v", launch)
	}
	if launch.ExtraJSON == nil {
		t.Fatal("extraJson is empty")
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(*launch.ExtraJSON), &extra); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"debugServer":        float64(5005),
		"mainClass":          "demo.Hello",
		"classPaths":         []any{"/p/target/classes"},
		"modulePaths":        []any{},
		"javaExec":           "/jdk/bin/java",
		"vmArgs":             debugAttachListener,
		"args":               "",
		"shortenCommandLine": "none",
		"console":            "internalConsole",
		"stopOnEntry":        false,
	}
	for k, v := range want {
		if !reflect.DeepEqual(extra[k], v) {
			t.Errorf("extraJson[%q] = %#v, want %#v", k, extra[k], v)
		}
	}
	if extra["projectName"] != "p" {
		t.Errorf("projectName = %v", extra["projectName"])
	}
}

func TestJavaDebugLaunchKeepsExplicitAttachOptions(t *testing.T) {
	for _, option := range []string{
		"-XX:+StartAttachListener",
		"-XX:-StartAttachListener",
		"-XX:+DisableAttachMechanism",
	} {
		if got := debugAttachListenerArgs(nil, option); len(got) != 0 {
			t.Errorf("debugAttachListenerArgs(%q) = %v, want no override", option, got)
		}
	}
}

func TestToggledLines(t *testing.T) {
	lines, found := toggledLines(nil, 4)
	if found || !reflect.DeepEqual(lines, []int{4}) {
		t.Fatalf("add: %v, %v", lines, found)
	}
	lines, found = toggledLines([]int{2, 4, 7}, 4)
	if !found || !reflect.DeepEqual(lines, []int{2, 7}) {
		t.Fatalf("remove: %v, %v", lines, found)
	}
	lines, _ = toggledLines([]int{2, 7}, 4)
	if !reflect.DeepEqual(lines, []int{2, 4, 7}) {
		t.Fatalf("sorted insert: %v", lines)
	}
}

func TestBreakpointLinesCopiesAndSorts(t *testing.T) {
	p := DebugPane{bps: map[string][]int{"/p/A.java": {9, 3, 6}}}
	got := p.breakpointLines("/p/A.java")
	if !reflect.DeepEqual(got, []int{3, 6, 9}) {
		t.Fatalf("lines = %v", got)
	}
	got[0] = 100
	if p.bps["/p/A.java"][0] != 9 {
		t.Fatal("breakpointLines aliases the stored list")
	}
	if lines := p.breakpointLines("/p/Other.java"); lines != nil {
		t.Fatalf("unknown file = %v", lines)
	}
}

func TestAddWatchTrimsAndDedupes(t *testing.T) {
	m := &Model{}
	if !m.addWatch("  count  ") {
		t.Fatalf("watch was rejected")
	}
	if !m.addWatch("count") {
		t.Fatalf("repeat watch was rejected")
	}
	if len(m.debug.watches) != 1 || m.debug.watches[0].Expr != "count" {
		t.Fatalf("watches = %+v", m.debug.watches)
	}
	if m.addWatch("   ") {
		t.Fatalf("blank watch was accepted")
	}
}

func TestAddWatchCapsThePanel(t *testing.T) {
	m := &Model{}
	for i := 0; i < maxWatches; i++ {
		if !m.addWatch(fmt.Sprintf("v%d", i)) {
			t.Fatalf("watch %d was rejected", i)
		}
	}
	if m.addWatch("one-too-many") {
		t.Fatalf("watch past the cap was accepted")
	}
	if m.debug.err == "" {
		t.Fatalf("cap did not explain itself")
	}
}

func TestRemoveWatch(t *testing.T) {
	m := &Model{debug: DebugPane{watches: []debugWatch{{Expr: "a"}, {Expr: "b"}}}}
	m.RemoveWatch(0)
	if len(m.debug.watches) != 1 || m.debug.watches[0].Expr != "b" {
		t.Fatalf("watches = %+v", m.debug.watches)
	}
	m.RemoveWatch(9)
	if len(m.debug.watches) != 1 {
		t.Fatalf("out-of-range remove changed the panel")
	}
}

func TestSubmitWatchNeedsPause(t *testing.T) {
	m := &Model{debug: DebugPane{evalText: "count"}}
	m.SubmitWatch()
	if len(m.debug.watches) != 0 || !strings.Contains(m.debug.err, "Pause") {
		t.Fatalf("watches = %+v, err = %q", m.debug.watches, m.debug.err)
	}
}
