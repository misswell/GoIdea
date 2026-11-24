package app

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestRunningWorkCountIncludesEveryLiveSessionKind(t *testing.T) {
	m := &Model{}
	m.run.runID = 1
	m.run.running = true
	m.run.children = []*runChild{{
		channel: 2, runID: 2, root: "/project", running: true,
	}}
	m.run.agentRuns = []*agentRunSession{{
		id: 3, channel: 3, runID: 4, awaiting: true,
	}}
	m.debug.session = 5
	m.debug.active = 5
	m.debug.sessions = map[uint32]*debugSessionState{
		5: {session: 5},
	}
	m.services.children = []*runChild{{
		channel: 6, runID: 6, root: "/project", service: true, running: true,
	}}
	m.services.starting = map[string]bool{"spring": true}
	m.term.groups = []termGroup{{tabs: []termSession{{id: 7}}}}

	if got := m.runningWorkCount(); got != 7 {
		t.Fatalf("running work = %d, want 7", got)
	}
}

func TestRunningWorkCountCountsIndependentDebugTabs(t *testing.T) {
	m := &Model{}
	m.debug.active = 5
	m.debug.session = 5
	m.debug.sessions = map[uint32]*debugSessionState{
		5: {session: 5},
		6: {session: 6},
	}

	if got := m.runningWorkCount(); got != 2 {
		t.Fatalf("running debug work = %d, want 2", got)
	}
}

func TestRunningWorkCountIgnoresAFinishedRun(t *testing.T) {
	m := &Model{}
	m.run.runID = 1
	m.run.exitCode = new(int32)
	if got := m.runningWorkCount(); got != 0 {
		t.Fatalf("finished run work = %d, want 0", got)
	}
}

func TestRunRuntimeSettingsFollowTheCurrentProjectRoot(t *testing.T) {
	m := &Model{}
	m.setRoot("/one")
	one := map[string]any{
		"run.confirmRerun":         false,
		"run.confirmTerminate":     false,
		"run.showOnStart":          false,
		"run.terminationTimeoutMs": float64(250),
	}
	if !m.applyRunRuntimeSettings(map[string]nativeSettingsPageState{
		settingsPageRunDebug: {
			loaded: true, scope: "project", root: "/one",
			applied: nativeSettingsCloneValues(one),
		},
	}) {
		t.Fatal("project one Run settings were not applied")
	}
	if m.confirmRerunEnabled() || m.confirmTerminateEnabled() || m.showRunOnStart() ||
		m.runTerminationTimeout() != 250*time.Millisecond {
		t.Fatal("project one Run settings were not active")
	}

	m.setRoot("/two")
	if !m.confirmRerunEnabled() || !m.confirmTerminateEnabled() || !m.showRunOnStart() ||
		m.runTerminationTimeout() != 3*time.Second {
		t.Fatal("project one Run settings leaked into project two before its page loaded")
	}
	two := map[string]any{
		"run.confirmRerun":         true,
		"run.confirmTerminate":     true,
		"run.showOnStart":          true,
		"run.terminationTimeoutMs": float64(9000),
	}
	m.applyRunRuntimeSettings(map[string]nativeSettingsPageState{
		settingsPageRunDebug: {
			loaded: true, scope: "project", root: "/two",
			applied: nativeSettingsCloneValues(two),
		},
	})
	if m.runTerminationTimeout() != 9*time.Second {
		t.Fatalf("project two termination timeout = %v", m.runTerminationTimeout())
	}
}

func TestDoNotAskPersistsRunSettingInTheProjectScope(t *testing.T) {
	saved := make(chan map[string]any, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "settings_set" {
				saved <- input.(map[string]any)
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot("/project")
	m.setRunRuntimeBool("run.confirmTerminate", false)
	if m.confirmTerminateEnabled() {
		t.Fatal("Do not ask again did not change the current project's runtime")
	}
	if m.prefs.Run != nil {
		t.Fatalf("project Do not ask again wrote the legacy global: %+v", m.prefs.Run)
	}

	select {
	case write := <-saved:
		if write["scope"] != "project" || write["key"] != "run.confirmTerminate" ||
			write["value"] != "false" {
			t.Fatalf("settings_set = %+v", write)
		}
	case <-time.After(time.Second):
		t.Fatal("Do not ask again did not persist the project setting")
	}
}

func TestProjectLeaveAndQuitProcessGuardsRunWithDirtyDocuments(t *testing.T) {
	m := &Model{
		postHook: func(fn func()) { fn() },
		tabs: []*Tab{{
			kind: tabKindNotebook, pageDirty: true,
		}},
	}
	m.setRoot("/old")
	m.run.runID = 10
	m.run.running = true

	prevented := 0
	m.requestQuit(func() { prevented++ })
	if prevented != 1 || m.run.terminateAsk == nil {
		t.Fatalf("quit with dirty text bypassed the process guard: prevented=%d ask=%+v",
			prevented, m.run.terminateAsk)
	}
	m.CancelRunTerminate()
	if m.quitting || m.run.terminateAsk != nil {
		t.Fatal("cancelling the quit process guard left the quit flow active")
	}

	m.confirmSwitchDirect("/new", nil)
	if m.run.terminateAsk == nil || m.Root() != "/old" {
		t.Fatalf("project switch process guard = %+v root=%q", m.run.terminateAsk, m.Root())
	}
	m.CancelRunTerminate()
	if m.Root() != "/old" {
		t.Fatalf("cancelling the switch changed the project to %q", m.Root())
	}

	m.CloseProject()
	if m.run.terminateAsk == nil || m.Root() != "/old" {
		t.Fatalf("Close Project process guard = %+v root=%q", m.run.terminateAsk, m.Root())
	}
	m.CancelRunTerminate()
	if m.Root() != "/old" {
		t.Fatalf("cancelling Close Project changed the project to %q", m.Root())
	}
}

func TestProjectLeaveSkipsTheProcessPromptWhenDisabled(t *testing.T) {
	m := &Model{
		postHook: func(fn func()) { fn() },
	}
	m.setRoot("/project")
	m.run.runID = 12
	m.run.running = true
	m.setRunRuntimeBool("run.confirmTerminate", false)

	m.CloseProject()
	if m.run.terminateAsk != nil {
		t.Fatalf("disabled confirmTerminate still prompted: %+v", m.run.terminateAsk)
	}
	if m.Root() != "" || !m.showingWelcome {
		t.Fatalf("Close Project did not finish: root=%q welcome=%v", m.Root(), m.showingWelcome)
	}
}

func TestRemoteDisconnectProcessGuardPrecedesDirtyDocuments(t *testing.T) {
	m := &Model{
		postHook: func(fn func()) { fn() },
		tabs: []*Tab{{
			kind: tabKindNotebook, pageDirty: true,
		}},
	}
	m.setRoot("/srv/app")
	m.remote.disconnectOpen = true
	m.remote.session = &openidea.RemoteSession{
		Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH, Config: "dev-box"},
		Root:   "/srv/app",
		Host:   "dev@example.test:22",
	}
	m.run.runID = 14
	m.run.running = true

	m.DisconnectRemoteProject()
	if m.run.terminateAsk == nil {
		t.Fatal("remote disconnect bypassed the running-process guard")
	}
	if m.remote.disconnectOpen {
		t.Fatal("remote disconnect dialog stayed open behind the process guard")
	}
	if m.Root() != "/srv/app" || m.remote.session == nil {
		t.Fatalf("process guard changed the remote project: root=%q session=%v",
			m.Root(), m.remote.session)
	}

	m.CancelRunTerminate()
	if m.Root() != "/srv/app" || m.remote.session == nil || m.remote.busy {
		t.Fatalf("cancelling remote disconnect changed state: root=%q session=%v busy=%v",
			m.Root(), m.remote.session, m.remote.busy)
	}
}

func TestResetRunStopsPrimaryChildrenAndAgentRuns(t *testing.T) {
	stopped := make(chan uint32, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "run_stop" {
				stopped <- input.(map[string]any)["id"].(uint32)
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot("/project")
	m.run.runID = 11
	m.run.running = true
	m.run.children = []*runChild{
		{channel: 21, runID: 12, root: "/project", running: true},
		{channel: 22, runID: 13, root: "/project", running: true},
	}
	m.run.agentRuns = []*agentRunSession{{
		id: 3, channel: 31, runID: 14, running: true,
		request: agentRunRequest{Name: "review"},
	}}
	m.run.agentActive = 3
	m.chooser = &runChooser{open: true, mode: "run", query: "old"}

	m.resetRun()

	got := map[uint32]bool{}
	deadline := time.After(time.Second)
	for len(got) < 4 {
		select {
		case id := <-stopped:
			got[id] = true
		case <-deadline:
			t.Fatalf("reset stopped run ids = %v, want 11, 12, 13, 14", got)
		}
	}
	for _, id := range []uint32{11, 12, 13, 14} {
		if !got[id] {
			t.Fatalf("reset stopped run ids = %v, missing %d", got, id)
		}
	}
	if m.run.runID != 0 || len(m.run.children) != 0 ||
		len(m.run.agentRuns) != 0 || m.chooser != nil {
		t.Fatalf("reset Run state = %+v chooser=%+v", m.run, m.chooser)
	}
	for _, channel := range []uint32{21, 22, 31} {
		if !m.runChildChannelRetired(channel) {
			t.Fatalf("channel %d was not retired across reset", channel)
		}
	}
}

func TestRetiredChildrenRejectLateOutputExitsAndRegistration(t *testing.T) {
	stopped := make(chan uint32, 2)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "run_stop" {
				stopped <- input.(map[string]any)["id"].(uint32)
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot("/new")
	m.retireRunChildChannel(41)
	m.retireRunChildRun(42)
	m.run.children = []*runChild{{
		channel: 43, root: "/new", starting: true,
	}}
	m.run.pendingChildExits = map[uint32]*int32{}

	if !m.acceptChildChunk(41, json.RawMessage(`{"text":"late"}`)) {
		t.Fatal("late output from a retired child was not consumed")
	}
	if !m.acceptChildExit(42, nil) {
		t.Fatal("late exit from a retired child was not consumed")
	}
	if len(m.run.pendingChildExits) != 0 {
		t.Fatalf("late exit entered the pending map: %+v", m.run.pendingChildExits)
	}

	stale := &runChild{channel: 44, root: "/old", starting: true}
	m.registerRunChild(stale, runInfoForTest(45), nil)
	late := &runChild{channel: 41, root: "/new", starting: true}
	m.registerRunChild(late, runInfoForTest(46), nil)

	got := []uint32{}
	deadline := time.After(time.Second)
	for len(got) < 2 {
		select {
		case id := <-stopped:
			got = append(got, id)
		case <-deadline:
			t.Fatalf("late registrations stopped ids = %v, want 45 and 46", got)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []uint32{45, 46}) {
		t.Fatalf("late registrations stopped ids = %v", got)
	}
}

func TestResetRunClearsTheChooserQueryAndSelection(t *testing.T) {
	m := &Model{}
	m.chooser = &runChooser{
		open: true, mode: "debug", query: "server", lastQuery: "server", sel: 4,
	}
	m.resetRun()
	if m.chooser != nil {
		t.Fatalf("project transition kept chooser state: %+v", m.chooser)
	}
}

func TestDebugTabClosePromptsOnceThenStopsTheSession(t *testing.T) {
	stopped := make(chan uint32, 2)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command == "debug_stop" {
				stopped <- input.(map[string]any)["session"].(uint32)
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.debug.active = 51
	m.debug.session = 51
	m.debug.tabName = "server"
	m.debug.config = "server"
	m.debug.sessions = map[uint32]*debugSessionState{
		51: {session: 51, tabName: "server", lastID: "server"},
	}
	m.debug.sessionOrder = []uint32{51}

	m.requestCloseDebugSessionTab(51)
	if m.run.terminateAsk == nil || m.run.terminateAsk.name != "server" {
		t.Fatalf("Debug tab close prompt = %+v", m.run.terminateAsk)
	}
	m.ConfirmRunTerminate()
	if m.run.terminateAsk != nil {
		t.Fatal("confirming Debug tab close opened a second prompt")
	}
	select {
	case id := <-stopped:
		if id != 51 {
			t.Fatalf("debug_stop session = %d, want 51", id)
		}
	case <-time.After(time.Second):
		t.Fatal("confirmed Debug tab close did not stop the session")
	}
	select {
	case id := <-stopped:
		t.Fatalf("Debug tab close stopped the session twice: %d", id)
	case <-time.After(50 * time.Millisecond):
	}
}

func runInfoForTest(id uint32) openidea.RunInfo {
	return openidea.RunInfo{ID: id, Running: true}
}
