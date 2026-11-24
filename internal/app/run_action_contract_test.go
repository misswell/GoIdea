package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestRunActionArgumentsReachTheNativeFlows(t *testing.T) {
	m := &Model{ctx: context.Background()}
	m.setRoot("/project")
	m.run.loaded = true
	m.run.trusted = true
	m.run.configs = []openidea.RunConfig{
		{ID: "first", Name: "First", Kind: "Application"},
		{ID: "second", Name: "Second", Kind: "Application"},
	}

	if !m.RunActionWithArg("run.chooseConfig", "select") ||
		m.chooser == nil || m.chooser.mode != "select" {
		t.Fatal("run.chooseConfig did not open the select-only widget chooser")
	}
	m.closeRunChooser()
	if !m.RunAction("run.chooseConfig") || m.chooser == nil || m.chooser.mode != "run" {
		t.Fatal("run.chooseConfig without an argument did not open the run chooser")
	}
	m.closeRunChooser()

	if !m.RunActionWithArg("run.editConfigs", "second") || m.runCfgDlg == nil ||
		m.runCfgDlg.sel != 1 || m.runCfgDlg.items[1].ID != "second" {
		t.Fatal("run.editConfigs did not select the requested configuration")
	}

	stopped := make(chan uint32, 1)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command == "run_stop" {
			id, _ := args.(map[string]any)["id"].(uint32)
			stopped <- id
		}
		return nil, nil
	}
	if !m.RunActionWithArg("run.stopProcess", float64(77)) {
		t.Fatal("run.stopProcess rejected a numeric process id")
	}
	select {
	case id := <-stopped:
		if id != 77 {
			t.Fatalf("run_stop id = %d, want 77", id)
		}
	case <-time.After(time.Second):
		t.Fatal("run.stopProcess did not call run_stop")
	}
}

func TestRunConfigurationOnExitSurvivesWrappedArguments(t *testing.T) {
	cfg := openidea.RunConfig{ID: "app", Name: "App", Kind: "Application"}
	arg := map[string]any{
		"config": cfg,
		"onExit": func(*int32, bool) {},
	}
	got, temporary, ok := runConfigFromActionArg(arg)
	if !ok || temporary || got.ID != cfg.ID {
		t.Fatalf("wrapped run config = %+v temporary=%v ok=%v", got, temporary, ok)
	}
}

func TestDebugConfigurationActionUsesTheExplicitRunConfig(t *testing.T) {
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
		},
	}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "debug_set_breakpoints":
			return json.RawMessage(`null`), nil
		case "cancel_request":
			return json.RawMessage(`null`), nil
		}
		return nil, nil
	}
	cfg := openidea.RunConfig{
		ID: "explicit", Name: "Explicit", Kind: "application", Command: "java",
		Args: []string{"-cp", "${classpath}", "example.Explicit"},
	}
	if !m.RunActionWithArg("run.debugConfig", cfg) {
		t.Fatal("run.debugConfig did not accept a RunConfig argument")
	}
	if m.debug.lastID != cfg.ID || !m.debug.starting {
		t.Fatalf("debug route = id %q starting=%v err=%q", m.debug.lastID, m.debug.starting, m.debug.err)
	}
	m.StopDebug()
}
