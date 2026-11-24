package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestRunAgentStartBuildsUpstreamConfigAndRejectsLiveName(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	started := make(chan openidea.RunConfig, 1)
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "run_start" {
				return nil, nil
			}
			args := input.(map[string]any)
			raw, _ := json.Marshal(args["config"])
			cfg, err := openidea.DecodeRunConfig(raw)
			if err != nil {
				t.Fatal(err)
			}
			started <- *cfg
			return json.Marshal(openidea.RunInfo{
				ID: 41, ConfigID: cfg.ID, Name: cfg.Name, Running: true,
			})
		},
	}
	m.setRoot(t.TempDir())
	m.run.trusted = true
	request := map[string]any{
		"name": "Reviewer", "command": "printf 'hello\\n'", "cwd": "app",
		"env": map[string]string{"TOKEN": "value"},
	}
	if !m.RunActionWithArg("run.agentStart", request) {
		t.Fatalf("run.agentStart was unavailable: %s", m.status)
	}
	select {
	case cfg := <-started:
		if cfg.ID != "mcp-agent:Reviewer" || cfg.Name != "Reviewer" ||
			cfg.Kind != "shell" || cfg.Command != "printf 'hello\\n'" ||
			cfg.Cwd == nil || *cfg.Cwd != "app" || cfg.Env["TOKEN"] != "value" {
			t.Fatalf("agent config = %+v", cfg)
		}
		if cfg.Params["mcp.agentRun"] != "Reviewer" ||
			cfg.Params["mcp.agentShell"] != "/bin/zsh" ||
			cfg.Target == nil || *cfg.Target != "" || !cfg.AllowParallel {
			t.Fatalf("agent params = %#v target=%v parallel=%v",
				cfg.Params, cfg.Target, cfg.AllowParallel)
		}
	case <-time.After(time.Second):
		t.Fatal("agent command did not reach run_start")
	}

	waitForSerialMainThread(t, main, time.Second, func() bool {
		run := m.agentRunByName("Reviewer")
		return run != nil && run.running && run.runID == 41
	})
	var rejected bool
	main.do(func() {
		rejected = !m.StartAgentRun(request)
	})
	if !rejected {
		t.Fatal("a live same-name agent command started twice")
	}
	var status string
	main.do(func() { status = m.status })
	if !strings.Contains(status, "still running") {
		t.Fatalf("same-name rejection status = %q", status)
	}

	main.do(func() {
		code := int32(0)
		m.acceptExit(openidea.RunExitedEvent{Run: 41, Code: &code})
	})
	waitForSerialMainThread(t, main, time.Second, func() bool {
		run := m.agentRunByName("Reviewer")
		return run != nil && !run.live() && run.exitCode != nil
	})
	request["command"] = "printf 'second\\n'"
	var restarted bool
	main.do(func() {
		restarted = m.StartAgentRun(request)
		status = m.status
	})
	if !restarted {
		t.Fatalf("finished agent tab was not reused: %s", status)
	}
	select {
	case cfg := <-started:
		if cfg.ID != "mcp-agent:Reviewer" || cfg.Command != "printf 'second\\n'" {
			t.Fatalf("reused agent config = %+v", cfg)
		}
	case <-time.After(time.Second):
		t.Fatal("reused agent tab did not reach run_start")
	}
	main.do(func() {
		code := int32(0)
		m.acceptExit(openidea.RunExitedEvent{Run: 41, Code: &code})
	})
}

func TestRunAgentStartIsHiddenAndProjectOnly(t *testing.T) {
	action, ok := actionGet("run.agentStart")
	if !ok {
		t.Fatal("run.agentStart is not registered")
	}
	if !hiddenActionIDs["run.agentStart"] {
		t.Fatal("run.agentStart must be hidden")
	}
	if action.When(&Model{}) {
		t.Fatal("run.agentStart must require an open project")
	}
	m := &Model{}
	m.setRoot(t.TempDir())
	if !action.When(m) {
		t.Fatal("run.agentStart is unavailable in an open project")
	}
}
