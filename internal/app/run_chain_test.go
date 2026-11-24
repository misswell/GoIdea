package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

type runChainHook struct {
	mu       sync.Mutex
	model    *Model
	started  []string
	configs  []openidea.RunConfig
	builds   []string
	runIn    chan string
	buildIn  chan string
	buildOut chan openidea.BuildResult
	buildEnd chan struct{}
	stops    []uint32
	nextID   atomic.Uint32
	exitCode int32
	hold     bool
}

func (h *runChainHook) hook(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
	switch cmd {
	case "run_start":
		payload := args.(map[string]any)
		raw, _ := json.Marshal(payload["config"])
		var cfg struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(raw, &cfg)
		id := h.nextID.Add(1)
		h.mu.Lock()
		h.started = append(h.started, cfg.ID)
		if decoded, err := openidea.DecodeRunConfig(raw); err == nil && decoded != nil {
			h.configs = append(h.configs, *decoded)
		}
		hold := h.hold
		code := h.exitCode
		h.mu.Unlock()
		if h.runIn != nil {
			h.runIn <- cfg.ID
		}
		if !hold {
			value := code
			h.model.post(func() {
				h.model.acceptExit(openidea.RunExitedEvent{Run: id, Code: &value})
			})
		}
		return json.Marshal(openidea.RunInfo{
			ID: id, ConfigID: cfg.ID, Name: cfg.Name, Running: true,
		})
	case "run_stop":
		id := uint32(args.(map[string]any)["id"].(uint32))
		h.mu.Lock()
		h.stops = append(h.stops, id)
		h.mu.Unlock()
		code := int32(0)
		h.model.post(func() {
			h.model.acceptExit(openidea.RunExitedEvent{Run: id, Code: &code})
		})
		return json.Marshal(nil)
	case "build_run":
		payload := args.(map[string]any)
		kind, _ := payload["kind"].(string)
		h.mu.Lock()
		h.builds = append(h.builds, kind)
		h.mu.Unlock()
		if h.buildIn != nil {
			h.buildIn <- kind
		}
		result := openidea.BuildResult{Title: "Build", Success: true}
		if h.buildOut != nil {
			select {
			case result = <-h.buildOut:
			case <-h.buildEnd:
				result = openidea.BuildResult{Title: "Build"}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return json.Marshal(result)
	case "cancel_request":
		if h.buildEnd != nil {
			select {
			case h.buildEnd <- struct{}{}:
			default:
			}
		}
		return json.Marshal(nil)
	}
	return json.Marshal(nil)
}

func newRunChainModel(t *testing.T, hook *runChainHook, configs ...map[string]any) *Model {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var main sync.Mutex
	m := &Model{
		ctx:        ctx,
		invokeHook: hook.hook,
		// mygo.RunOnMain serializes callbacks. Keep the headless model's
		// Compound workers on the same contract instead of running several
		// main-thread mutations at once.
		postHook: func(fn func()) {
			main.Lock()
			defer main.Unlock()
			fn()
		},
	}
	m.root = "/p"
	m.run.trusted = true
	m.run.loaded = true
	decoded, err := openidea.DecodeRunConfigs(mustJSON(t, configs))
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = decoded
	hook.mu.Lock()
	hook.model = m
	hook.mu.Unlock()
	return m
}

func waitRunChain(t *testing.T, timeout time.Duration, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the run chain")
}

func TestCompoundStartsEveryMemberAndNeverCallsRunStartForItself(t *testing.T) {
	hook := &runChainHook{}
	m := newRunChainModel(t, hook,
		map[string]any{"id": "one", "name": "One", "kind": "shell", "command": "echo"},
		map[string]any{"id": "two", "name": "Two", "kind": "shell", "command": "echo"},
		map[string]any{
			"id": "all", "name": "All", "kind": "compound",
			"compound": []string{"one", "two"},
		},
	)
	all := m.run.configs[2]
	if !m.startRunWith(all.Raw(), all.Name) {
		t.Fatal("Compound did not start")
	}
	waitRunChain(t, 2*time.Second, func() bool {
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == 2
	})
	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		return len(m.run.children) == 2 && !m.run.awaiting && !m.run.running
	})
	hook.mu.Lock()
	started := strings.Join(hook.started, ",")
	hook.mu.Unlock()
	if strings.Contains(started, "all") || !strings.Contains(started, "one") || !strings.Contains(started, "two") {
		t.Fatalf("run_start configurations = %s", started)
	}
	if len(m.run.children) != 2 || m.run.children[0].name == "" || m.run.children[1].name == "" {
		t.Fatalf("Compound sessions = %+v", m.run.children)
	}
}

func TestBeforeLaunchFailureNeverStartsTheMainConfiguration(t *testing.T) {
	hook := &runChainHook{exitCode: 1}
	m := newRunChainModel(t, hook,
		map[string]any{
			"id": "app", "name": "App", "kind": "shell", "command": "echo",
			"beforeLaunch": []map[string]string{{"kind": "command", "target": "false"}},
		},
	)
	m.run.sel = 0
	app := m.run.configs[0]
	if !m.startRunWith(app.Raw(), app.Name) {
		t.Fatal("the launch did not enter before-launch preparation")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(m.run.err, "was not started") {
		m.post(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(m.run.err, "was not started") {
		hook.mu.Lock()
		started := append([]string(nil), hook.started...)
		hook.mu.Unlock()
		childCode, childErr := "nil", ""
		if len(m.run.children) > 0 && m.run.children[0].exitCode != nil {
			childCode = fmt.Sprintf("%d", *m.run.children[0].exitCode)
		}
		if len(m.run.children) > 0 {
			childErr = m.run.children[0].err
		}
		t.Fatalf("before-launch failure did not settle: preparing=%t busy=%t awaiting=%t err=%q children=%d started=%v childCode=%s childErr=%q",
			m.run.preparingGroup, m.run.busy, m.run.awaiting, m.run.err, len(m.run.children),
			started, childCode, childErr)
	}
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.started) != 1 || hook.started[0] != "before-launch:app:false" {
		t.Fatalf("failed before-launch tasks should be the only run_start: %v", hook.started)
	}
	if !strings.Contains(m.run.err, "Run 'false' failed") {
		t.Fatalf("the failure did not name the task: %s", m.run.err)
	}
}

func TestBeforeLaunchBuildRunsToCompletionBeforeTheMainProcess(t *testing.T) {
	hook := &runChainHook{}
	m := newRunChainModel(t, hook,
		map[string]any{
			"id": "app", "name": "App", "kind": "shell", "command": "echo",
			"beforeLaunch": []map[string]string{{"kind": "build"}},
		},
	)
	app := m.run.configs[0]
	if !m.startRunWith(app.Raw(), app.Name) {
		t.Fatal("build-before-launch did not start")
	}
	waitRunChain(t, 3*time.Second, func() bool {
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.builds) == 1 && len(hook.started) >= 1
	})
	hook.mu.Lock()
	builds := append([]string(nil), hook.builds...)
	started := append([]string(nil), hook.started...)
	hook.mu.Unlock()
	if len(builds) != 1 || builds[0] != "build" {
		t.Fatalf("build_run = %v", builds)
	}
	if len(started) == 0 || started[0] != "app" {
		t.Fatalf("the main configuration did not start after the build: %v", started)
	}
}

func TestStopCompoundStopsEveryLiveSession(t *testing.T) {
	hook := &runChainHook{hold: true}
	m := newRunChainModel(t, hook,
		map[string]any{"id": "one", "name": "One", "kind": "shell", "command": "echo"},
		map[string]any{"id": "two", "name": "Two", "kind": "shell", "command": "echo"},
		map[string]any{
			"id": "all", "name": "All", "kind": "compound",
			"compound": []string{"one", "two"},
		},
	)
	m.run.sel = 2
	all := m.run.configs[2]
	if !m.startRunWith(all.Raw(), all.Name) {
		t.Fatal("Compound did not start")
	}
	waitRunChain(t, 2*time.Second, func() bool {
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == 2
	})
	waitRunChain(t, 2*time.Second, func() bool {
		ids := make([]uint32, 0, 2)
		m.post(func() {
			for _, child := range m.run.children {
				ids = append(ids, child.runID)
			}
		})
		return len(ids) == 2 && ids[0] != 0 && ids[1] != 0
	})
	m.StopRun()
	waitRunChain(t, 2*time.Second, func() bool {
		m.post(func() {})
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.stops) == 2 && !m.run.awaiting && !m.run.running
	})
}

func TestPrepareTestReportingAddsFrameworkFlags(t *testing.T) {
	goConfig := &openidea.RunConfig{Kind: "go", IsTest: true, Args: []string{"test", "./..."}}
	raw, _ := json.Marshal(goConfig)
	got, _ := prepareTestReporting(raw, goConfig)
	var cfg openidea.RunConfig
	if err := json.Unmarshal(got, &cfg); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(cfg.Args) != "[test -json ./...]" {
		t.Fatalf("Go test arguments = %v", cfg.Args)
	}
	pytest := &openidea.RunConfig{Kind: "pytest", IsTest: true, Args: []string{"tests"}}
	pytestRaw, _ := json.Marshal(pytest)
	got, _ = prepareTestReporting(pytestRaw, pytest)
	if err := json.Unmarshal(got, &pytest); err != nil {
		t.Fatal(err)
	}
	if len(pytest.Args) != 2 || pytest.Args[1] != "-v" {
		t.Fatalf("pytest arguments = %v", pytest.Args)
	}
}

func TestPrepareTestReportingLeavesOrdinaryRunUnchanged(t *testing.T) {
	raw := json.RawMessage(`{"kind":"go","args":["run","."]}`)
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, dotnetReports := prepareTestReporting(raw, cfg)
	if string(got) != string(raw) {
		t.Fatalf("ordinary run changed from %s to %s", raw, got)
	}
	if dotnetReports != nil {
		t.Fatal("ordinary run unexpectedly configured .NET test reports")
	}
}
