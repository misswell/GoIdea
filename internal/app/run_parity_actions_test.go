package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
