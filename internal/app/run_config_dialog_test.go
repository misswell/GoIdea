package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// runCfgHook is the injectable RPC client for the Edit Configurations flow:
// run_save_configs persists, run_configs answers the reload.
type runCfgHook struct {
	saved [][]map[string]any
	calls []string
}

func (h *runCfgHook) hook(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
	h.calls = append(h.calls, cmd)
	switch cmd {
	case "run_save_configs":
		payload := args.(map[string]any)
		configs, _ := payload["configs"].([]map[string]any)
		h.saved = append(h.saved, configs)
		return json.Marshal(nil)
	case "run_configs":
		mvn := map[string]any{
			"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn",
			"args": []string{"test"}, "detected": true,
			"unmodelled": "kept",
		}
		if len(h.saved) > 0 {
			// The backend merges user configs over detected ones.
			return json.Marshal([]map[string]any{h.saved[len(h.saved)-1][0], mvn})
		}
		return json.Marshal([]map[string]any{mvn})
	}
	return json.Marshal(nil)
}

func TestRunConfigDialogDrawsAsASheet(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn",
		"args": []string{"test"}, "detected": true,
	}}))
	m.OpenRunConfigs()
	if m.runCfgDlg == nil || !m.runCfgDlg.open {
		t.Fatal("Edit Configurations opened without its sheet flag")
	}
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()
	// The sheet used to be created with open=false, so the modal closed it in
	// the same frame and Run › Edit Configurations… showed nothing.
	if _, ok := tt.Find("运行/调试配置"); !ok {
		t.Fatalf("the Edit Configurations sheet was not drawn: %v", tt.Texts())
	}
}

func TestRunConfigDialogSavesAUserCopy(t *testing.T) {
	hook := &runCfgHook{}
	m := &Model{invokeHook: hook.hook, postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn",
		"args": []string{"test"}, "detected": true, "unmodelled": "kept",
	}}))
	m.OpenRunConfigs()
	if m.runCfgDlg == nil || len(m.runCfgDlg.items) != 1 {
		t.Fatalf("the dialog opened with the project's configurations")
	}
	// Editing the detected configuration makes a user copy with its id.
	d := m.runCfgDlg
	d.name = "test (edited)"
	m.SaveRunConfigs()
	// The save goroutine posts inline; wait for it to land.
	for i := 0; i < 100 && len(hook.saved) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if len(hook.saved) != 1 {
		t.Fatalf("one run_save_configs expected, got %d", len(hook.saved))
	}
	saved := hook.saved[0][0]
	if saved["detected"] != false || saved["name"] != "test (edited)" {
		t.Fatalf("the edit became a user config: %v", saved)
	}
	if saved["unmodelled"] != "kept" {
		t.Fatalf("a field GoIDEA does not model survives: %v", saved["unmodelled"])
	}
	if m.runCfgDlg != nil {
		t.Fatalf("a successful save closes the dialog and reloads")
	}
	if m.run.loading {
		t.Fatalf("the merged list is reloaded")
	}
}

func TestRunConfigDialogValidatesBeforeSaving(t *testing.T) {
	hook := &runCfgHook{}
	m := &Model{invokeHook: hook.hook, postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn",
		"args": []string{"test"}, "detected": true,
	}}))
	m.OpenRunConfigs()
	d := m.runCfgDlg
	d.name = ""
	m.SaveRunConfigs()
	if len(hook.saved) != 0 {
		t.Fatalf("an invalid edit must not reach run_save_configs")
	}
	if d.errs["name"] == "" {
		t.Fatalf("the field error is shown")
	}
	if m.runCfgDlg == nil {
		t.Fatalf("the dialog stays open")
	}
}

func TestRerunAsksWhileRunningAndRemembersDoNotAsk(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	raw := runConfigJSON(t, map[string]any{
		"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn", "args": []string{"test"},
	})
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.run.configs = []openidea.RunConfig{*cfg}
	m.run.sel = 0

	// Nothing started yet: a rerun has no target and does nothing.
	m.Rerun()
	if m.rerunAsk != nil {
		t.Fatalf("no target, no confirm")
	}

	// The pane started the configuration; rerunning while it runs asks.
	m.run.lastID = "maven:test"
	m.run.runID = 7
	m.run.running = true
	m.Rerun()
	if m.rerunAsk == nil || m.rerunAsk.name != "test" {
		t.Fatalf("a running configuration asks before it is stopped")
	}

	// Stop and Rerun with "Do not ask again" persists the choice and stops.
	m.rerunAsk.dontAsk = true
	invoked := 0
	m.invokeHook = func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd == "run_stop" {
			invoked++
		}
		return json.Marshal(nil)
	}
	m.ConfirmRerun()
	if m.rerunAsk != nil {
		t.Fatalf("the answer closes the dialog")
	}
	for i := 0; i < 100 && invoked == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if invoked != 1 {
		t.Fatalf("Stop and Rerun stops the process first, got %d stops", invoked)
	}
	if m.confirmRerunEnabled() {
		t.Fatalf("'Do not ask again' is remembered")
	}

	// With the choice stored, a rerun stops and restarts without asking.
	m.run.running = true
	m.run.exitCode = nil
	m.Rerun()
	if m.rerunAsk != nil {
		t.Fatalf("the stored choice skips the confirm")
	}
}
