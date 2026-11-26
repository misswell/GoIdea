package app

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestProfilerSupportMatchesOpenIDEA(t *testing.T) {
	cases := []struct {
		config openidea.RunConfig
		want   bool
	}{
		{openidea.RunConfig{Command: "/usr/bin/node", Args: []string{"app.js"}}, true},
		{openidea.RunConfig{Command: "pnpm", Args: []string{"test"}}, true},
		{openidea.RunConfig{Command: "java", Kind: "java"}, true},
		{openidea.RunConfig{Command: "python3.13", Kind: "python"}, true},
		{openidea.RunConfig{Command: "uv", Args: []string{"run", "main.py"}}, true},
		{openidea.RunConfig{Command: "uv", Args: []string{"sync"}}, false},
		{openidea.RunConfig{Command: "cargo", Args: []string{"+nightly", "run"}},
			runtime.GOOS == "linux"},
		{openidea.RunConfig{Command: "hello", Kind: "native"}, runtime.GOOS == "linux"},
		{openidea.RunConfig{Command: "ruby", Args: []string{"main.rb"}}, false},
	}
	for _, tc := range cases {
		if got := profilerSupports(tc.config); got != tc.want {
			t.Fatalf("profilerSupports(%+v) = %v, want %v", tc.config, got, tc.want)
		}
	}
	if !allocationProfilerSupports(openidea.RunConfig{Command: "/opt/jdk/bin/java"}) {
		t.Fatal("JVM allocations are not supported")
	}
	if allocationProfilerSupports(openidea.RunConfig{Command: "node"}) {
		t.Fatal("Node.js allocations were offered")
	}
}

func TestProfilerFlameCallTreeAndFormattingMatchOpenIDEA(t *testing.T) {
	profile := testProfile(openidea.ProfileCPU, "ms", 100)
	profile.Nodes = []openidea.ProfileNode{
		{Frame: 0, Total: profileFloat(100), SelfWeight: profileFloat(0), Children: []int{1, 2}},
		{Frame: 1, Total: profileFloat(60), SelfWeight: profileFloat(10), Children: []int{3}},
		{Frame: 2, Total: profileFloat(40), SelfWeight: profileFloat(40)},
		{Frame: 3, Total: profileFloat(50), SelfWeight: profileFloat(50)},
	}
	rects, depth := profileFlameLayout(profile, 0, 0)
	if depth != 3 {
		t.Fatalf("flame depth = %d, want 3", depth)
	}
	byNode := map[int]profileFlameRect{}
	for _, rect := range rects {
		byNode[rect.node] = rect
	}
	for node, want := range map[int]profileFlameRect{
		0: {node: 0, depth: 0, x: 0, width: 1},
		1: {node: 1, depth: 1, x: 0, width: 0.6},
		2: {node: 2, depth: 1, x: 0.6, width: 0.4},
		3: {node: 3, depth: 2, x: 0, width: 0.5},
	} {
		if byNode[node] != want {
			t.Fatalf("flame node %d = %+v, want %+v", node, byNode[node], want)
		}
	}

	rows := profileTreeRows(profile, map[int]bool{1: true})
	wantRows := []profileTreeRow{{node: 1, depth: 0}, {node: 3, depth: 1}, {node: 2, depth: 0}}
	if fmt.Sprint(rows) != fmt.Sprint(wantRows) {
		t.Fatalf("call tree rows = %+v, want %+v", rows, wantRows)
	}
	if expanded := hottestProfilePath(profile, 30); !expanded[1] || !expanded[3] {
		t.Fatalf("hottest path = %v", expanded)
	}

	if got := formatProfileWeight(10000, "ms"); got != "10.0 s" {
		t.Fatalf("10,000 ms = %q", got)
	}
	if got := profileSummary(testProfile(openidea.ProfileLiveObjects, "samples", 1)); got !=
		"test · 1 live sample (≈ 512 KB)" {
		t.Fatalf("single live-object summary = %q", got)
	}
}

func TestProfilerSnapshotLimitAndCloseSelectionMatchOpenIDEA(t *testing.T) {
	m := &Model{}
	for index := 0; index < maxProfileSnapshots+2; index++ {
		m.addProfileSnapshot(fmt.Sprintf("App %d", index),
			testProfile(openidea.ProfileCPU, "ms", float64(index+1)), "")
	}
	if len(m.profiler.snapshots) != maxProfileSnapshots {
		t.Fatalf("snapshots = %d", len(m.profiler.snapshots))
	}
	first := m.profiler.snapshots[0]
	m.CloseProfileSnapshot(first.id)
	if m.profiler.activeID != m.profiler.snapshots[0].id {
		t.Fatalf("after closing active snapshot selected %q, want %q",
			m.profiler.activeID, m.profiler.snapshots[0].id)
	}
}

func TestProfilerRegistersAllOpenIDEAEntryPoints(t *testing.T) {
	for _, id := range []string{
		"view.profiler",
		"run.profile",
		"run.profileAllocations",
		"run.profileLiveObjects",
		"run.profileContext",
	} {
		if _, ok := Command(id); !ok {
			t.Fatalf("%s is not registered", id)
		}
	}
	if spec := dockSpec("profiler"); spec == nil || spec.title != "Profiler" ||
		spec.anchor != dockBottom || spec.order != 25 {
		t.Fatalf("profiler tool window = %+v", spec)
	}
}

func TestProfilerRunsCollectsAndShowsASnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	main := make(chan func(), 32)
	m := profilerTestModel(ctx, main)
	cfg := openidea.RunConfig{
		ID: "node-app", Name: "App", Kind: "shell", Command: "node", Args: []string{"app.js"},
	}
	m.run.configs = []openidea.RunConfig{cfg}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		args, _ := input.(map[string]any)
		switch command {
		case "profiler_prepare":
			if args["mode"] != string(openidea.ProfileCPU) {
				return nil, fmt.Errorf("prepare mode = %v", args["mode"])
			}
			return json.RawMessage(`{
				"status":"ready",
				"profiler":"node --cpu-prof",
				"launch":{"id":"node-app","name":"App","kind":"shell","command":"node","args":["app.js"]},
				"spec":{"kind":"cpuProfiles","dir":"/tmp/profile","root":"/tmp/project"}
			}`), nil
		case "run_start":
			m.post(func() {
				code := int32(0)
				m.acceptExit(openidea.RunExitedEvent{Run: 42, Code: &code})
			})
			return json.RawMessage(`{"id":42,"name":"Profile 'App'","running":true}`), nil
		case "profiler_collect":
			spec := args["spec"].(json.RawMessage)
			if !strings.Contains(string(spec), `"cpuProfiles"`) {
				return nil, fmt.Errorf("profile spec changed: %s", spec)
			}
			return json.RawMessage(`{
				"profiler":"node --cpu-prof","unit":"ms","mode":"cpu","total":100,
				"frames":[{"name":"all","file":null,"line":null,"kind":"code"},
					{"name":"run","file":"/tmp/project/app.js","line":9,"kind":"code"}],
				"nodes":[{"frame":0,"total":100,"selfWeight":0,"children":[1]},
					{"frame":1,"total":100,"selfWeight":100,"children":[]}],
				"methods":[{"frame":1,"selfWeight":100,"total":100}]
			}`), nil
		default:
			return nil, fmt.Errorf("unexpected RPC %q", command)
		}
	}

	m.RunWithProfiler(openidea.ProfileCPU)
	drainProfilerMain(t, ctx, main, func() bool { return len(m.profiler.snapshots) == 1 })

	if snapshot := m.activeProfileSnapshot(); snapshot == nil || snapshot.name != "App" ||
		snapshot.profile.Total == nil || *snapshot.profile.Total != 100 {
		t.Fatalf("collected snapshot = %+v", snapshot)
	}
	if !m.dockWindowShown("profiler") {
		t.Fatal("the profiler window was not shown after collection")
	}
	if m.run.running || m.run.awaiting {
		t.Fatalf("profiled run still active: running=%v awaiting=%v", m.run.running, m.run.awaiting)
	}
	if m.profiler.err != "" {
		t.Fatalf("profiler error = %q", m.profiler.err)
	}
}

func TestProfilerLiveObjectsCollectsBeforeStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	main := make(chan func(), 32)
	m := profilerTestModel(ctx, main)
	m.run.configs = []openidea.RunConfig{{
		ID: "jvm-app", Name: "JVM App", Kind: "shell", Command: "java", Args: []string{"Main"},
	}}
	var mu sync.Mutex
	var calls []string
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		mu.Lock()
		calls = append(calls, command)
		mu.Unlock()
		switch command {
		case "profiler_prepare":
			return json.RawMessage(`{
				"status":"ready","profiler":"async-profiler",
				"launch":{"id":"jvm-app","name":"JVM App","kind":"shell","command":"java","args":["Main"]},
				"spec":{"kind":"folded","profiler":"async-profiler","dir":"/tmp/profile",
					"ext":"collapsed","base":"/tmp/project","jvmRoot":"/tmp/project","mode":"liveObjects"}
			}`), nil
		case "run_start":
			return json.RawMessage(`{"id":43,"name":"Profile 'JVM App'","running":true}`), nil
		case "profiler_gc_before_stop":
			return json.RawMessage(`1`), nil
		case "run_stop":
			m.post(func() {
				code := int32(143)
				m.acceptExit(openidea.RunExitedEvent{Run: 43, Code: &code})
			})
			return json.RawMessage(`null`), nil
		case "profiler_collect":
			return json.RawMessage(`{
				"profiler":"async-profiler","unit":"samples","mode":"liveObjects","total":1,
				"frames":[{"name":"all","file":null,"line":null,"kind":"code"},
					{"name":"com.example.Main","file":"/tmp/project/Main.java","line":3,"kind":"code"}],
				"nodes":[{"frame":0,"total":1,"selfWeight":0,"children":[1]},
					{"frame":1,"total":1,"selfWeight":1,"children":[]}],
				"methods":[{"frame":1,"selfWeight":1,"total":1}]
			}`), nil
		default:
			return nil, fmt.Errorf("unexpected RPC %q", command)
		}
	}

	m.RunWithProfiler(openidea.ProfileLiveObjects)
	drainProfilerMain(t, ctx, main, func() bool {
		return m.run.runID == 43 && m.run.running
	})
	m.StopRun()
	drainProfilerMain(t, ctx, main, func() bool { return len(m.profiler.snapshots) == 1 })

	snapshot := m.activeProfileSnapshot()
	if snapshot == nil || snapshot.profile.Mode != openidea.ProfileLiveObjects || snapshot.notice != "" {
		t.Fatalf("live-object snapshot = %+v", snapshot)
	}
	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	var profileCalls []string
	for _, call := range gotCalls {
		switch call {
		case "profiler_prepare", "run_start", "profiler_gc_before_stop", "run_stop", "profiler_collect":
			profileCalls = append(profileCalls, call)
		}
	}
	want := []string{"profiler_prepare", "run_start", "profiler_gc_before_stop", "run_stop", "profiler_collect"}
	if fmt.Sprint(profileCalls) != fmt.Sprint(want) {
		t.Fatalf("profiler RPC order = %v, want %v (all calls: %v)", profileCalls, want, gotCalls)
	}
}

func TestProfilerMissingToolAndUnsupportedAreActionable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reply     string
		wantLevel string
		wantTitle string
	}{
		{
			name: "missing",
			reply: `{"status":"missingTool","tool":"py-spy","message":"Install py-spy.",
				"install":{"command":"python3","args":["-m","pip","install","py-spy"],"cwd":null},
				"homepage":"https://github.com/benfred/py-spy"}`,
			wantLevel: "warning",
			wantTitle: "py-spy is not installed",
		},
		{
			name:      "unsupported",
			reply:     `{"status":"unsupported","reason":"Profiling Bun programs is not supported."}`,
			wantLevel: "info",
			wantTitle: "Cannot profile 'App'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			main := make(chan func(), 16)
			m := profilerTestModel(ctx, main)
			m.run.configs = []openidea.RunConfig{{
				ID: "app", Name: "App", Kind: "python", Command: "python3", Args: []string{"main.py"},
			}}
			m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
				if command != "profiler_prepare" {
					return nil, fmt.Errorf("unexpected RPC %q", command)
				}
				return json.RawMessage(tc.reply), nil
			}
			m.RunWithProfiler(openidea.ProfileCPU)
			drainProfilerMain(t, ctx, main, func() bool {
				return !m.profiler.preparing && len(m.notify.items) > 0
			})
			if len(m.notify.items) != 1 {
				t.Fatalf("notifications = %+v", m.notify.items)
			}
			notification := m.notify.items[0]
			if notification.level != tc.wantLevel || notification.title != tc.wantTitle {
				t.Fatalf("notification = %+v", notification)
			}
			if tc.name == "missing" {
				if !notification.sticky || len(notification.actions) != 2 {
					t.Fatalf("missing-tool notification = %+v", notification)
				}
			}
		})
	}
}

func TestProfilerViewRendersNativeFlameGraph(t *testing.T) {
	m := &Model{}
	m.addProfileSnapshot("App", testProfile(openidea.ProfileAllocations, "bytes", 1024), "Read before GC.")
	m.profiler.query = "run"
	tester := ui.NewTester(func(c *ui.Context) {
		theme := openideaTheme(false)
		c.SetTheme(theme)
		m.profilerPanel(c, theme, openideaColors(false))
	}, 1000, 520)
	for _, text := range []string{
		"App · Allocations",
		"Flame Graph",
		"Call Tree",
		"Method List",
		"1.0 KB allocated",
		"all (1.0 KB)",
		"Read before GC.",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Profiler view is missing %q: %v", text, tester.Texts())
		}
	}
}

func profilerTestModel(ctx context.Context, main chan func()) *Model {
	m := &Model{ctx: ctx}
	m.setRoot("/tmp/project")
	m.postHook = func(fn func()) { main <- fn }
	m.run.trusted = true
	m.run.loaded = true
	m.run.sel = 0
	return m
}

func drainProfilerMain(t *testing.T, ctx context.Context, main <-chan func(), done func() bool) {
	t.Helper()
	for {
		if done() {
			return
		}
		select {
		case fn := <-main:
			fn()
		case <-ctx.Done():
			t.Fatalf("timed out waiting for the profiler: %v", ctx.Err())
		}
	}
}

func testProfile(mode openidea.ProfileMode, unit string, total float64) openidea.Profile {
	line := 2
	file := "/tmp/project/Main.java"
	return openidea.Profile{
		Profiler: "test",
		Unit:     unit,
		Mode:     mode,
		Total:    profileFloat(total),
		Frames: []openidea.ProfileFrame{
			{Name: "Root", Kind: "code"},
			{Name: "com.example.Main.run", File: &file, Line: &line, Kind: "code"},
		},
		Nodes: []openidea.ProfileNode{
			{Frame: 0, Total: profileFloat(total), SelfWeight: profileFloat(0), Children: []int{1}},
			{Frame: 1, Total: profileFloat(total), SelfWeight: profileFloat(total)},
		},
		Methods: []openidea.MethodStat{{
			Frame: 1, SelfWeight: profileFloat(total), Total: profileFloat(total),
		}},
	}
}

func profileFloat(value float64) *float64 {
	return &value
}
