package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestRunTerminateSettingCanBypassThePrompt(t *testing.T) {
	off := false
	m := &Model{prefs: Preferences{Run: &RunPrefs{ConfirmTerminate: &off}}}
	called := false
	m.requestRunTermination("app", func() { called = true })
	if !called || m.run.terminateAsk != nil {
		t.Fatalf("termination bypass = called %v, prompt %#v", called, m.run.terminateAsk)
	}
}

func TestRunTerminatePromptCanPersistDontAskAndTerminate(t *testing.T) {
	m := &Model{cfg: Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")}}
	called := false
	m.requestRunTermination("server", func() { called = true })
	if m.run.terminateAsk == nil {
		t.Fatal("live process close did not ask")
	}
	m.run.terminateAsk.dontAsk = true
	m.ConfirmRunTerminate()
	if !called {
		t.Fatal("Terminate did not invoke the close action")
	}
	loaded := LoadPreferences(m.cfg.SettingsPath)
	if loaded.Run == nil || loaded.Run.ConfirmTerminate == nil ||
		*loaded.Run.ConfirmTerminate {
		t.Fatalf("persisted Run preferences = %#v", loaded.Run)
	}
}

func TestRequestCloseDebugSessionGuardOnlyPromptsForLiveSessions(t *testing.T) {
	off := false
	m := &Model{prefs: Preferences{Run: &RunPrefs{ConfirmTerminate: &off}}}
	m.debug.sessions = map[uint32]*debugSessionState{
		1: {session: 1, ended: true, status: "Finished", tabName: "server"},
	}
	m.debug.sessionOrder = []uint32{1}
	m.debug.active = 1
	m.debug.session = 1
	m.debug.tabName = "server"
	m.requestCloseDebugSessionTab(1)
	if m.run.terminateAsk != nil {
		t.Fatal("a finished debug session prompted")
	}

	m.prefs.Run.ConfirmTerminate = nil
	m.debug.session = 1
	m.debug.ended = false
	m.debug.tabName, m.debug.config = "server", "server"
	m.requestCloseDebugSessionTab(1)
	if m.run.terminateAsk == nil || m.run.terminateAsk.name != "server" {
		t.Fatalf("terminate prompt = %#v", m.run.terminateAsk)
	}
}

func TestConfirmTerminateStopsAndClosesARunChild(t *testing.T) {
	stopped := make(chan uint32, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "run_stop" {
				t.Fatalf("close used %q, want run_stop", command)
			}
			stopped <- input.(map[string]any)["id"].(uint32)
			return json.RawMessage(`null`), nil
		},
	}
	child := &runChild{channel: 9, runID: 41, name: "server", running: true}
	m.run.children = []*runChild{child}
	m.run.activeChild = child.channel

	m.requestCloseRunChild(child.channel)
	if m.run.terminateAsk == nil || m.run.terminateAsk.name != "server" {
		t.Fatalf("Run child close prompt = %#v", m.run.terminateAsk)
	}
	m.ConfirmRunTerminate()
	if m.childByChannel(child.channel) != nil || m.run.activeChild != 0 {
		t.Fatalf("confirmed Run child remained open: %+v", m.run.children)
	}
	select {
	case id := <-stopped:
		if id != 41 {
			t.Fatalf("run_stop id = %d, want 41", id)
		}
	case <-time.After(time.Second):
		t.Fatal("confirmed Run child did not stop")
	}
}

func TestCancelTerminateLeavesARunChildRunning(t *testing.T) {
	called := make(chan struct{}, 1)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(context.Context, string, any) (json.RawMessage, error) {
			called <- struct{}{}
			return json.RawMessage(`null`), nil
		},
	}
	child := &runChild{channel: 12, runID: 44, name: "worker", running: true}
	m.run.children = []*runChild{child}
	m.requestCloseRunChild(child.channel)
	m.CancelRunTerminate()
	if m.childByChannel(child.channel) != child || m.run.terminateAsk != nil {
		t.Fatal("Cancel closed or kept the Run child prompt")
	}
	select {
	case <-called:
		t.Fatal("Cancel stopped the Run child")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestConfirmTerminateStopsAndClosesAnAgentRun(t *testing.T) {
	type backendCall struct {
		command string
		id      uint32
	}
	calls := make(chan backendCall, 2)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			call := backendCall{command: command}
			if args, ok := input.(map[string]any); ok {
				call.id, _ = args["id"].(uint32)
			}
			calls <- call
			return json.RawMessage(`null`), nil
		},
	}
	run := &agentRunSession{
		id: 3, request: agentRunRequest{Name: "review"}, runID: 52, running: true,
	}
	m.run.agentRuns = []*agentRunSession{run}
	m.run.agentActive = run.id

	m.requestCloseAgentRun(run.id)
	if m.run.terminateAsk == nil || m.run.terminateAsk.name != "review" {
		t.Fatalf("Agent Run close prompt = %#v", m.run.terminateAsk)
	}
	m.ConfirmRunTerminate()
	if m.agentRunByID(run.id) != nil || m.run.agentActive != 0 {
		t.Fatalf("confirmed Agent Run remained open: %+v", m.run.agentRuns)
	}
	got := map[string]uint32{}
	for range 2 {
		select {
		case call := <-calls:
			got[call.command] = call.id
		case <-time.After(time.Second):
			t.Fatalf("confirmed Agent Run backend calls = %v", got)
		}
	}
	if len(got) != 2 || got["run_stop"] != 52 {
		t.Fatalf("confirmed Agent Run backend calls = %v", got)
	}
	if _, ok := got["mcp_agent_run_closed"]; !ok {
		t.Fatalf("confirmed Agent Run did not notify MCP: %v", got)
	}
}
