package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestRunDynamicActionsRegisterOpenIDEAIDs(t *testing.T) {
	for _, test := range []struct {
		id       string
		category string
		hidden   bool
	}{
		{"go.generate.file", "Go", false},
		{"go.generate.package", "Go", false},
		{"php.composerInstall", "PHP", false},
		{"run.modifyContext", "Run", false},
		{"run.sampleOutput", "Run", true},
	} {
		action, ok := actionGet(test.id)
		if !ok {
			t.Fatalf("%s is not registered", test.id)
		}
		if action.Category != test.category || hiddenActionIDs[test.id] != test.hidden {
			t.Fatalf("%s = category %q hidden %v", test.id, action.Category, hiddenActionIDs[test.id])
		}
	}

	const externalID = "dynamic-registry-test"
	t.Cleanup(func() { unregisterAction("externalTools." + externalID) })
	m := &Model{prefs: Preferences{ExternalTools: []ExternalToolPrefs{{
		ID: externalID, Name: "Dynamic Tool", Enabled: true, Program: "tool",
	}}}}
	m.root = "/project"
	m.syncExternalToolActions()
	if !m.ActionEnabled("externalTools." + externalID) {
		t.Fatal("configured external tool was not registered as a dynamic action")
	}
}

func TestGoGenerateConfigMatchesOpenIDEA(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "gen.go")
	file, err := goGenerateConfig(path, "file", " stringer ")
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != "go:generate:file:"+path+":^stringer$" ||
		file.Name != "go generate gen.go" ||
		file.Command != "go" ||
		!sameStringSlice(file.Args, []string{"generate", "-run", "^stringer$", "gen.go"}) ||
		file.Cwd == nil || *file.Cwd != filepath.Dir(path) ||
		file.Params["useTags"] != "true" {
		t.Fatalf("file generation config = %+v", file)
	}
	pkg, err := goGenerateConfig(path, "package", "")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "go generate ." ||
		!sameStringSlice(pkg.Args, []string{"generate", "."}) {
		t.Fatalf("package generation config = %+v", pkg)
	}
}

func TestGoGenerateAndExternalToolStartThroughBackend(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := writeTestFile(path, "package main\n"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	t.Run("go generate", func(t *testing.T) {
		var mu sync.Mutex
		m := &Model{
			ctx:      ctx,
			postHook: func(fn func()) { mu.Lock(); defer mu.Unlock(); fn() },
		}
		m.root = root
		m.run.trusted = true
		started := make(chan openidea.RunConfig, 1)
		m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "run_start" {
				return nil, errors.New("unexpected RPC " + command)
			}
			args := input.(map[string]any)
			var cfg openidea.RunConfig
			if err := json.Unmarshal(args["config"].(json.RawMessage), &cfg); err != nil {
				return nil, err
			}
			started <- cfg
			return json.RawMessage(`{"id":51,"running":true,"name":"go generate ."}`), nil
		}

		m.RunGoGenerate("package", map[string]any{"path": path})
		select {
		case cfg := <-started:
			if cfg.Kind != "go" || !sameStringSlice(cfg.Args, []string{"generate", "."}) {
				t.Fatalf("backend Go Generate config = %+v", cfg)
			}
		case <-ctx.Done():
			t.Fatal("Go Generate did not reach run_start")
		}
	})

	t.Run("external tool", func(t *testing.T) {
		var mu sync.Mutex
		m := &Model{
			ctx:      ctx,
			postHook: func(fn func()) { mu.Lock(); defer mu.Unlock(); fn() },
			prefs: Preferences{ExternalTools: []ExternalToolPrefs{{
				ID: "lint", Name: "Lint", Enabled: true,
				Program: "lint", Arguments: `"$ProjectFileDir$"`,
				WorkingDirectory: "$ProjectFileDir$",
			}}},
		}
		m.root = root
		m.run.trusted = true
		started := make(chan openidea.RunConfig, 1)
		m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "run_start" {
				return nil, errors.New("unexpected RPC " + command)
			}
			args := input.(map[string]any)
			var cfg openidea.RunConfig
			if err := json.Unmarshal(args["config"].(json.RawMessage), &cfg); err != nil {
				return nil, err
			}
			started <- cfg
			return json.RawMessage(`{"id":52,"running":true,"name":"Lint"}`), nil
		}

		m.RunExternalTool("lint")
		select {
		case cfg := <-started:
			if cfg.Kind != "external" || cfg.Command != "lint" ||
				len(cfg.Args) != 1 || cfg.Args[0] != root {
				t.Fatalf("backend external tool config = %+v", cfg)
			}
		case <-ctx.Done():
			t.Fatal("external tool did not reach run_start")
		}
	})
}

func TestComposerInstallAppliesSettingsThenStartsBackend(t *testing.T) {
	root := t.TempDir()
	file, err := composerInstallConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if file.Command != "composer" || !sameStringSlice(file.Args, []string{"install"}) ||
		file.Kind != "composer" || file.Cwd == nil || *file.Cwd != root {
		t.Fatalf("Composer install config = %+v", file)
	}
	source := filepath.Join(root, "composer.json")
	if err := writeTestFile(source, "{}\n"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var mu sync.Mutex
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { mu.Lock(); defer mu.Unlock(); fn() },
	}
	m.root = root
	m.run.trusted = true
	var applied, started openidea.RunConfig
	startCh := make(chan struct{}, 1)
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "settings_get":
			return json.RawMessage(`{"runTargets":[],"run":{}}`), nil
		case "sdk_php_composer_install":
			args := input.(map[string]any)
			if args["path"] != source {
				t.Fatalf("Composer lookup path = %v", args["path"])
			}
			return json.RawMessage(`{"root":"` + root + `","reason":"missing"}`), nil
		case "run_apply_settings":
			args := input.(map[string]any)
			configs := args["configs"].([]*openidea.RunConfig)
			applied = *configs[0]
			applied.Env = map[string]string{"PATH": "/selected/php/bin"}
			raw, err := json.Marshal([]openidea.RunConfig{applied})
			return raw, err
		case "run_start":
			args := input.(map[string]any)
			if err := json.Unmarshal(args["config"].(json.RawMessage), &started); err != nil {
				return nil, err
			}
			startCh <- struct{}{}
			return json.RawMessage(`{"id":61,"running":true,"name":"composer install"}`), nil
		default:
			return nil, errors.New("unexpected RPC " + command)
		}
	}

	m.ComposerInstall(map[string]any{"path": source})
	select {
	case <-startCh:
		if started.Kind != "composer" || started.Env["PATH"] != "/selected/php/bin" ||
			started.Cwd == nil || *started.Cwd != root {
			t.Fatalf("launched Composer config = %+v (applied %+v)", started, applied)
		}
	case <-ctx.Done():
		t.Fatal("Composer Install did not reach run_start")
	}
}

func TestDynamicRunBackendFailureIsVisible(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := writeTestFile(path, "package main\n"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var mu sync.Mutex
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			mu.Lock()
			defer mu.Unlock()
			fn()
		},
	}
	m.root = root
	m.run.trusted = true
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "run_start" {
			return nil, errors.New("unexpected RPC " + command)
		}
		return nil, errors.New("toolchain unavailable")
	}
	m.RunGoGenerate("file", map[string]any{"path": path})
	for {
		var runErr string
		m.post(func() { runErr = m.run.err })
		if strings.Contains(runErr, "toolchain unavailable") {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("run failure stayed invisible: %q", runErr)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestModifyRunContextOpensSelectedTemporaryConfiguration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "run.sh")
	doc := writableSourceDocument(path, "#!/bin/sh\nprintf ok\n")
	m := &Model{
		ctx:    context.Background(),
		tabs:   []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}},
		active: 0,
	}
	m.setRoot(root)
	m.run.loaded = true
	if !m.RunCommandWithArg("run.modifyContext", map[string]any{
		"path": path, "content": doc.Text(),
	}) {
		t.Fatal("run.modifyContext was not registered")
	}
	if m.runCfgDlg == nil || len(m.run.temporaries) != 1 {
		t.Fatalf("context editor = %+v, temporaries = %+v", m.runCfgDlg, m.run.temporaries)
	}
	selected := m.runCfgDlg.selected()
	if selected == nil || selected.Kind != "shell" ||
		selected.ID != m.run.temporaries[0].ID ||
		!sameStringSlice(selected.Args, []string{"run.sh"}) {
		t.Fatalf("selected context configuration = %+v", selected)
	}
}

func TestGoGenerateProjectMenuOnlyShowsForGoFiles(t *testing.T) {
	goLabels := treeMenuLabels(treeMenuItems(treeTarget{path: "/project/main.go"}, false))
	if !containsString(goLabels, "Go Generate/Go Generate File/Go Generate Package") {
		t.Fatalf("Go file menu = %v", goLabels)
	}
	textLabels := treeMenuLabels(treeMenuItems(treeTarget{path: "/project/readme.txt"}, false))
	if containsString(textLabels, "Go Generate/Go Generate File/Go Generate Package") {
		t.Fatalf("non-Go file menu contains Go Generate: %v", textLabels)
	}
}

func TestSampleOutputStreamsAndStopIsVisible(t *testing.T) {
	var mu sync.Mutex
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { mu.Lock(); defer mu.Unlock(); fn() },
	}
	m.StreamSampleOutput(map[string]any{"lines": float64(6), "perFrame": 3})
	waitForDynamicAction(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return !m.run.sampleOutputBusy
	})
	mu.Lock()
	if len(m.run.output) != 6 || !strings.Contains(m.status, "finished") {
		t.Fatalf("sample output = %d lines, status %q", len(m.run.output), m.status)
	}
	mu.Unlock()

	m.StreamSampleOutput(map[string]any{"lines": 1000, "perFrame": 1})
	m.StopRun()
	waitForDynamicAction(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return !m.run.sampleOutputBusy && m.status == "Sample output stopped"
	})
}

func waitForDynamicAction(t *testing.T, timeout time.Duration, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for dynamic run action")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func writeTestFile(path, text string) error {
	return osWriteFile(path, []byte(text))
}
