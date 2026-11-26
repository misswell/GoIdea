package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func TestRunImportActionAddsConfigurationsToTheDialogDraft(t *testing.T) {
	done := make(chan struct{})
	var once sync.Once
	m := &Model{
		ctx:  context.Background(),
		root: "/project",
		run: RunPane{
			trusted: true,
			loaded:  true,
			configs: []openidea.RunConfig{{
				ID: "maven:test", Name: "All tests", Kind: "maven",
				Command: "mvn", Args: []string{"test"}, Detected: true,
			}},
		},
	}
	m.postHook = func(fn func()) {
		fn()
		once.Do(func() { close(done) })
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "run_import_sources":
			return json.RawMessage(`["vscode"]`), nil
		case "run_configs":
			return json.RawMessage(`[{
				"id":"maven:test","name":"All tests","kind":"maven",
				"command":"mvn","args":["test"],"detected":true
			}]`), nil
		case "run_import_configs":
			request := args.(map[string]any)
			if request["source"] != openidea.RunImportVSCode {
				t.Fatalf("import source = %#v", request["source"])
			}
			return json.RawMessage(`{
				"source":"vscode",
				"file":".vscode/launch.json",
				"imported":[{
					"config":{
						"id":"user:server",
						"name":"Server",
						"kind":"go",
						"command":"go",
						"args":["run","."],
						"detected":false
					},
					"ignored":["preLaunchTask"]
				}],
				"skipped":[{
					"name":"Native",
					"reason":{"kind":"unsupportedType","type":"cppdbg"}
				}]
			}`), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}

	if !m.RunAction("run.importVscodeLaunch") {
		t.Fatal("run.importVscodeLaunch did not run")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run configuration import did not finish")
	}
	d := m.runCfgDlg
	if d == nil || !d.open || len(d.items) != 2 {
		t.Fatalf("run configuration dialog = %+v; status=%q loading=%v notifications=%+v",
			d, m.status, m.run.loading, m.notify.items)
	}
	if d.items[d.sel].Name != "Server" {
		t.Fatalf("selected imported configuration = %+v", d.items[d.sel])
	}
	if d.importFile != ".vscode/launch.json" || len(d.importNotice) != 3 {
		t.Fatalf("import notice = %#v", d.importNotice)
	}
	if !strings.Contains(strings.Join(d.importNotice, "\n"), "cppdbg") ||
		!strings.Contains(strings.Join(d.importNotice, "\n"), "preLaunchTask") {
		t.Fatalf("import notice = %#v", d.importNotice)
	}
}

func TestCreateRunConfigurationSavesAndOpensTheUserCopy(t *testing.T) {
	done := make(chan struct{})
	var once sync.Once
	var saved any
	m := &Model{ctx: context.Background(), root: "/project"}
	m.postHook = func(fn func()) {
		fn()
		once.Do(func() { close(done) })
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "run_configs":
			return json.RawMessage(`[{
				"id":"maven:test","name":"All tests","kind":"maven",
				"command":"mvn","args":["test"],"detected":true
			}]`), nil
		case "run_save_configs":
			saved = args
			return json.RawMessage(`null`), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}

	if !m.RunActionWithArg("run.createConfig", map[string]any{
		"id": "user:app", "name": "App", "kind": "go", "command": "go",
		"args": []string{"run", "."}, "detected": true,
	}) {
		t.Fatal("run.createConfig did not run")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run.createConfig did not finish")
	}
	if !m.run.loaded || m.runCfgDlg == nil || m.runCfgDlg.items[m.runCfgDlg.sel].ID != "user:app" {
		t.Fatalf("created configuration was not opened: %+v", m.runCfgDlg)
	}
	request := saved.(map[string]any)
	configs := request["configs"].([]map[string]any)
	if len(configs) != 1 || configs[0]["id"] != "user:app" ||
		configs[0]["detected"] != false {
		t.Fatalf("saved configurations = %#v", configs)
	}
}

func TestRunContextActionArgumentsUseTheNamedFileAndLine(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal/calc/calc_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package calc\nfunc TestAdd(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "settings_get":
				return json.RawMessage(`{"runTargets":[],"run":{}}`), nil
			case "run_start":
				cfg := args.(map[string]any)["config"].(json.RawMessage)
				var launched openidea.RunConfig
				if err := json.Unmarshal(cfg, &launched); err != nil {
					return nil, err
				}
				if launched.ID == "" || !sameStringSlice(launched.Args[len(launched.Args)-2:], []string{"-run", "^TestAdd$"}) {
					t.Fatalf("launched context config = %+v", launched)
				}
				return json.RawMessage(`{"id":51,"running":true,"name":"TestAdd"}`), nil
			default:
				return nil, fmt.Errorf("unexpected RPC %s", command)
			}
		},
	}
	m.setRoot(root)
	m.run.trusted, m.run.loaded = true, true
	m.run.configs = []openidea.RunConfig{{
		ID: "go:root", Name: "Go Tests", Kind: "go", Command: "go",
		Args: []string{"test", "./..."}, IsTest: true,
	}}
	if !m.RunActionWithArg("run.runContext", map[string]any{
		"path": path, "line": float64(1), "content": "package calc\nfunc TestAdd(t *testing.T) {}\n",
	}) {
		t.Fatal("run.runContext did not accept the active-file argument")
	}
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.run.runID == 51 && m.run.running
	})
}

func TestRunStopAndRerunTargetTheSelectedChildTab(t *testing.T) {
	main := &serialMainThread{}
	stopped := make(chan uint32, 1)
	m := &Model{ctx: context.Background(), postHook: main.post}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "run_stop" {
			return nil, fmt.Errorf("unexpected RPC %s", command)
		}
		stopped <- args.(map[string]any)["id"].(uint32)
		return json.RawMessage(`null`), nil
	}
	m.tool = "run"
	m.splitNames = map[string]bool{"run": true}
	primary := openidea.RunConfig{ID: "primary", Name: "Primary", Kind: "application"}
	selected := openidea.RunConfig{ID: "child", Name: "Selected", Kind: "shell"}
	child := &runChild{
		channel: 12, runID: 44, name: selected.Name, config: selected,
		groupID: "compound", running: true, started: time.Now(),
	}
	m.run = RunPane{
		visible: true, trusted: true, loaded: true,
		configs: []openidea.RunConfig{primary, selected}, sel: 0,
		running: true, runID: 7, channel: 11, lastName: primary.Name,
		children: []*runChild{child}, activeChild: child.channel,
	}
	if !m.RunActionWithArg("run.stop", "widget") {
		t.Fatal("run.stop did not accept the widget route")
	}
	select {
	case id := <-stopped:
		if id != 44 {
			t.Fatalf("run_stop id = %d, want the selected child 44", id)
		}
	case <-time.After(time.Second):
		t.Fatal("run.stop did not stop the selected child tab")
	}

	failed := adapter.NewTestResults()
	failed.Apply([]adapter.TestEvent{{
		Kind: "result", Path: []string{"demo.CartTest", "fails"}, Status: adapter.TestFailed,
	}})
	child.finished = true
	child.running = false
	child.tests = failed
	child.config = openidea.RunConfig{
		ID: "maven:test", Name: "Tests", Kind: "maven", Command: "mvn",
		Args: []string{"test"}, IsTest: true,
	}
	m.run.running, m.run.awaiting = false, false
	m.run.children[0].config = child.config
	m.run.configs = append(m.run.configs, child.config)
	if !m.RunAction("run.rerunFailed") {
		t.Fatal("run.rerunFailed was unavailable")
	}
	if len(m.notify.items) != 0 {
		t.Fatalf("selected failed tab was not used: %+v", m.notify.items)
	}
}
