package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestAgentRunLimitsArchiveOldestFinishedTabsAndNeverLiveOnes(t *testing.T) {
	main := &serialMainThread{}
	var invokeMu sync.Mutex
	nextID := uint32(40)
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		run:      RunPane{trusted: true},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "run_start" {
				return json.RawMessage(`null`), nil
			}
			invokeMu.Lock()
			nextID++
			id := nextID
			invokeMu.Unlock()
			args := input.(map[string]any)
			rawConfig, _ := json.Marshal(args["config"])
			cfg, err := openidea.DecodeRunConfig(rawConfig)
			if err != nil {
				t.Fatal(err)
			}
			return json.Marshal(openidea.RunInfo{
				ID: id, ConfigID: cfg.ID, Name: cfg.Name, Running: true,
			})
		},
	}
	m.setRoot(t.TempDir())
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageAgentMCPServer: {
			loaded: true,
			scope:  "global",
			applied: map[string]any{
				"mcpServer.agentTabLimit":     float64(1),
				"mcpServer.agentArchiveLimit": float64(1),
			},
			draft: map[string]any{
				"mcpServer.agentTabLimit":     float64(1),
				"mcpServer.agentArchiveLimit": float64(1),
			},
		},
	}

	start := func(name string) uint32 {
		t.Helper()
		var started bool
		main.do(func() {
			started = m.StartAgentRun(map[string]any{
				"name": name, "command": "printf " + name + "\n", "cwd": ".",
			})
		})
		if !started {
			t.Fatalf("%s did not start: %s", name, m.status)
		}
		waitForSerialMainThread(t, main, time.Second, func() bool {
			run := m.agentRunByName(name)
			return run != nil && run.runID != 0 && run.running
		})
		var id uint32
		main.do(func() { id = m.agentRunByName(name).runID })
		return id
	}
	finish := func(id uint32) {
		t.Helper()
		main.do(func() {
			code := int32(0)
			m.acceptExit(openidea.RunExitedEvent{Run: id, Code: &code})
		})
		waitForSerialMainThread(t, main, time.Second, func() bool {
			for _, run := range m.run.agentRuns {
				if run.runID == id {
					return !run.live()
				}
			}
			return false
		})
	}

	finish(start("alpha"))
	finish(start("beta"))
	finish(start("gamma"))

	main.do(func() {
		visible := m.visibleAgentRuns()
		archive := m.archivedAgentRuns()
		if len(m.run.agentRuns) != 2 ||
			len(visible) != 1 || visible[0].request.Name != "gamma" ||
			len(archive) != 1 || archive[0].request.Name != "beta" {
			t.Fatalf("sessions=%v visible=%v archive=%v",
				agentRunNames(m.run.agentRuns),
				agentRunNames(visible),
				agentRunNames(archive),
			)
		}
	})

	liveID := start("delta")
	main.do(func() {
		if archive := agentRunNames(m.archivedAgentRuns()); len(archive) != 1 || archive[0] != "beta" {
			t.Fatalf("finished archive = %v", archive)
		}
		for _, run := range m.archivedAgentRuns() {
			if run.request.Name == "delta" {
				t.Fatal("a live agent tab entered the archive")
			}
		}
		visible := agentRunNames(m.visibleAgentRuns())
		if len(visible) != 2 || visible[0] != "gamma" || visible[1] != "delta" {
			t.Fatalf("visible while live = %v", visible)
		}
	})

	tester := ui.NewTester(func(c *ui.Context) { m.runAgentTabs(c, c.Theme()) }, 700, 80)
	text := strings.Join(tester.Texts(), "\n")
	if !strings.Contains(text, "delta") || !strings.Contains(text, "Archived (1)") {
		t.Fatalf("native agent tabs = %q", tester.Texts())
	}
	finish(liveID)
}

func TestAgentRunSettingPageLoadsAndStagesGlobalLimits(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "settings_get_global" {
				t.Fatalf("command = %q, want settings_get_global", command)
			}
			return json.RawMessage(`{"mcpServer":{"agentTabLimit":4,"agentArchiveLimit":12}}`), nil
		},
	}
	m.setRoot(t.TempDir())
	main.do(func() { m.loadNativeSettingsPage(settingsPageAgentMCPServer) })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return m.nativeSettings.pages[settingsPageAgentMCPServer].loaded
	})
	main.do(func() {
		state := m.nativeSettings.pages[settingsPageAgentMCPServer]
		if state.scope != "global" ||
			nativeSettingsNumberValue(m, settingsPageAgentMCPServer, "mcpServer.agentTabLimit") != 4 ||
			nativeSettingsNumberValue(m, settingsPageAgentMCPServer, "mcpServer.agentArchiveLimit") != 12 {
			t.Fatalf("loaded limits = %+v", state)
		}
		m.setNativeSettingsValue(settingsPageAgentMCPServer, "mcpServer.agentTabLimit", float64(2))
		m.setNativeSettingsValue(settingsPageAgentMCPServer, "mcpServer.agentArchiveLimit", float64(7))
		writes := m.pendingNativeSettingsWrites()
		if len(writes) != 2 {
			t.Fatalf("writes = %+v", writes)
		}
		for _, write := range writes {
			if write.scope != "global" {
				t.Fatalf("write scope = %q for %s", write.scope, write.key)
			}
		}
	})
}

func TestAgentRunSettingPageRendersLimitsAndGlobalScope(t *testing.T) {
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageAgentMCPServer: {
			loaded: true,
			scope:  "global",
			applied: map[string]any{
				"mcpServer.agentTabLimit":     float64(4),
				"mcpServer.agentArchiveLimit": float64(12),
			},
			draft: map[string]any{
				"mcpServer.agentTabLimit":     float64(4),
				"mcpServer.agentArchiveLimit": float64(12),
			},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsNativeBody(c, oiPaletteOf(c.Theme()), settingsPageAgentMCPServer)
	}, 900, 500)
	tester.Frame()
	for _, text := range []string{
		"Finished agent tabs to show",
		"Archived agent tabs to keep",
		"Stored in Application.",
	} {
		if !tester.HasText(text) {
			t.Fatalf("IDE MCP Server page lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestAgentRunLimitsApplyTrimsArchive(t *testing.T) {
	main := &serialMainThread{}
	var invokeMu sync.Mutex
	var writes []settingsBackendWrite
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Fatalf("command = %q, want settings_set", command)
			}
			args := input.(map[string]any)
			invokeMu.Lock()
			writes = append(writes, settingsBackendWrite{
				scope: args["scope"].(string),
				key:   args["key"].(string),
				value: args["value"],
			})
			invokeMu.Unlock()
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(t.TempDir())
	m.backendFormatOnSaveKnown = true
	finished := time.Now().Add(-time.Minute)
	m.run.agentRuns = []*agentRunSession{
		{id: 1, request: agentRunRequest{Name: "alpha"}, ended: finished},
		{id: 2, request: agentRunRequest{Name: "beta"}, ended: finished.Add(time.Second)},
		{id: 3, request: agentRunRequest{Name: "gamma"}, ended: finished.Add(2 * time.Second)},
	}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageAgentMCPServer: {
			loaded: true,
			scope:  "global",
			applied: map[string]any{
				"mcpServer.agentTabLimit":     float64(0),
				"mcpServer.agentArchiveLimit": float64(999),
			},
			draft: map[string]any{
				"mcpServer.agentTabLimit":     float64(1),
				"mcpServer.agentArchiveLimit": float64(1),
			},
		},
	}

	main.do(func() { m.saveBackendCatalogThenJava(false) })
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return !m.nativeSettings.saving &&
			!m.settingsDirtyPage(settingsPageAgentMCPServer)
	})

	main.do(func() {
		tabs, archive := m.agentRunLimits()
		if tabs != 1 || archive != 1 {
			t.Fatalf("applied limits = %d, %d", tabs, archive)
		}
		if all := agentRunNames(m.run.agentRuns); strings.Join(all, ",") != "beta,gamma" {
			t.Fatalf("sessions after Apply = %v", all)
		}
		if visible := agentRunNames(m.visibleAgentRuns()); strings.Join(visible, ",") != "gamma" {
			t.Fatalf("visible after Apply = %v", visible)
		}
		if archived := agentRunNames(m.archivedAgentRuns()); strings.Join(archived, ",") != "beta" {
			t.Fatalf("archive after Apply = %v", archived)
		}
	})
	invokeMu.Lock()
	defer invokeMu.Unlock()
	if len(writes) != 2 {
		t.Fatalf("settings writes = %+v", writes)
	}
	values := map[string]any{}
	for _, write := range writes {
		if write.scope != "global" {
			t.Fatalf("write scope = %q for %s", write.scope, write.key)
		}
		values[write.key] = write.value
	}
	if values["mcpServer.agentTabLimit"] != "1" ||
		values["mcpServer.agentArchiveLimit"] != "1" {
		t.Fatalf("settings values = %+v", values)
	}
}

func agentRunNames(runs []*agentRunSession) []string {
	names := make([]string, 0, len(runs))
	for _, run := range runs {
		if run != nil {
			names = append(names, run.request.Name)
		}
	}
	return names
}
