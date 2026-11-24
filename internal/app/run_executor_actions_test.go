package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestWithExecutorCoverageAndProfilerDispatch(t *testing.T) {
	t.Run("coverage", func(t *testing.T) {
		started := make(chan openidea.RunConfig, 1)
		m := &Model{
			ctx:      context.Background(),
			postHook: func(fn func()) { fn() },
		}
		config := map[string]any{
			"id": "app", "name": "App", "kind": "shell", "command": "go",
			"args": []string{"run", "."},
		}
		m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "coverage_prepare":
				rawConfig := input.(map[string]any)["config"].(json.RawMessage)
				return json.Marshal(map[string]any{
					"status": "ready", "launch": json.RawMessage(rawConfig),
					"report": map[string]any{"kind": "go"},
				})
			case "run_start":
				raw, _ := json.Marshal(input.(map[string]any)["config"])
				cfg, err := openidea.DecodeRunConfig(raw)
				if err != nil {
					t.Fatal(err)
				}
				started <- *cfg
				return json.Marshal(openidea.RunInfo{
					ID: 31, ConfigID: cfg.ID, Name: cfg.Name, Running: true,
				})
			}
			return json.RawMessage(`null`), nil
		}
		m.setRoot(t.TempDir())
		m.run.trusted = true
		if !m.RunActionWithArg("run.withExecutor", map[string]any{
			"executor": "coverage", "config": config,
		}) {
			t.Fatal("coverage executor action was unavailable")
		}
		select {
		case cfg := <-started:
			if cfg.ID != "app" {
				t.Fatalf("coverage launched %q", cfg.ID)
			}
		case <-time.After(time.Second):
			t.Fatal("coverage executor did not reach run_start")
		}
	})

	for _, test := range []struct {
		executor string
		mode     openidea.ProfileMode
	}{
		{"profiler", openidea.ProfileCPU},
		{"profilerAllocations", openidea.ProfileAllocations},
		{"profilerLiveObjects", openidea.ProfileLiveObjects},
	} {
		t.Run(test.executor, func(t *testing.T) {
			started := make(chan struct{}, 1)
			m := &Model{
				ctx:      context.Background(),
				postHook: func(fn func()) { fn() },
			}
			config := map[string]any{
				"id": "app", "name": "App", "kind": "shell", "command": "go",
				"args": []string{"run", "."},
			}
			m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
				switch command {
				case "profiler_prepare":
					args := input.(map[string]any)
					if args["mode"] != string(test.mode) {
						t.Fatalf("profiler mode = %v, want %s", args["mode"], test.mode)
					}
					return json.Marshal(map[string]any{
						"status": "ready",
						"launch": map[string]any{
							"id": "profile:app", "name": "Profile App", "kind": "shell",
							"command": "go", "args": []string{"run", "."},
						},
						"spec": map[string]any{"mode": test.mode},
					})
				case "run_start":
					started <- struct{}{}
					return json.Marshal(openidea.RunInfo{
						ID: 32, ConfigID: "profile:app", Name: "Profile App", Running: true,
					})
				}
				return json.RawMessage(`null`), nil
			}
			m.setRoot(t.TempDir())
			m.run.trusted = true
			if !m.RunActionWithArg("run.withExecutor", map[string]any{
				"executor": test.executor, "config": config,
			}) {
				t.Fatal("profiler executor action was unavailable")
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("profiler executor did not reach run_start")
			}
		})
	}
}

func TestWithExecutorContextAndInvalidExecutor(t *testing.T) {
	started := make(chan struct{}, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "coverage_prepare":
			return json.Marshal(map[string]any{
				"status": "ready",
				"launch": input.(map[string]any)["config"],
				"report": map[string]any{"kind": "go"},
			})
		case "run_start":
			raw, _ := json.Marshal(input.(map[string]any)["config"])
			var config struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(raw, &config)
			if config.ID == "" {
				t.Fatalf("context executor launched no config: %s", raw)
			}
			started <- struct{}{}
			return json.Marshal(openidea.RunInfo{
				ID: 33, ConfigID: config.ID, Name: "Context", Running: true,
			})
		}
		return json.RawMessage(`null`), nil
	}
	root := t.TempDir()
	m.setRoot(root)
	m.run.trusted = true
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rawGo := mustJSON(t, map[string]any{
		"id": "go:run", "name": "go run .", "kind": "go", "command": "go",
		"args": []string{"run", "."}, "params": map[string]any{"goKind": "run"},
	})
	cfg, err := openidea.DecodeRunConfig(rawGo)
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = []openidea.RunConfig{*cfg}
	if !m.RunActionWithArg("run.withExecutor", map[string]any{
		"executor": "coverage",
		"context": map[string]any{
			"path": path, "content": "package main\nfunc main() {}\n",
		},
	}) {
		t.Fatalf("context executor was unavailable: %s", m.status)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("context executor did not reach run_start")
	}

	called := false
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		called = true
		return nil, fmt.Errorf("unexpected %s", command)
	}
	if m.RunWithExecutor(map[string]any{"executor": "unknown"}) {
		t.Fatal("an unknown executor was accepted")
	}
	if called {
		t.Fatal("an unknown executor reached the backend")
	}
}

func TestWithExecutorPresentNullContextUsesActiveEditor(t *testing.T) {
	started := make(chan string, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "coverage_prepare":
			return json.Marshal(map[string]any{
				"status": "ready",
				"launch": input.(map[string]any)["config"],
				"report": map[string]any{"kind": "go"},
			})
		case "run_start":
			raw, _ := json.Marshal(input.(map[string]any)["config"])
			var config struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(raw, &config)
			started <- config.ID
			return json.Marshal(openidea.RunInfo{
				ID: 34, ConfigID: config.ID, Name: "Active Editor", Running: true,
			})
		}
		return json.RawMessage(`null`), nil
	}
	root := t.TempDir()
	m.setRoot(root)
	m.run.trusted = true
	path := filepath.Join(root, "main.go")
	doc := writableSourceDocument(path, "package main\nfunc main() {}\n")
	m.tabs = []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}}
	rawGo := mustJSON(t, map[string]any{
		"id": "go:run", "name": "go run .", "kind": "go", "command": "go",
		"args": []string{"run", "."}, "params": map[string]any{"goKind": "run"},
	})
	cfg, err := openidea.DecodeRunConfig(rawGo)
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = []openidea.RunConfig{*cfg}
	if !m.RunWithExecutor(map[string]any{
		"executor": "coverage",
		"context":  nil,
	}) {
		t.Fatalf("present-null context executor was unavailable: %s", m.status)
	}
	select {
	case id := <-started:
		if id == "" {
			t.Fatal("present-null context launched no configuration")
		}
	case <-time.After(time.Second):
		t.Fatal("present-null context did not use the active editor")
	}
}

func TestWithExecutorIsHiddenProjectOnly(t *testing.T) {
	action, ok := actionGet("run.withExecutor")
	if !ok || !hiddenActionIDs["run.withExecutor"] {
		t.Fatal("run.withExecutor is not a hidden registered action")
	}
	if action.When(&Model{}) {
		t.Fatal("run.withExecutor must require an open project")
	}
}
