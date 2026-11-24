package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestExternalToolMacrosUseOpenIDEANamesAndProjectPaths(t *testing.T) {
	ctx := externalMacroContext{
		projectRoot:  "/work/demo",
		projectName:  "demo",
		filePath:     "/work/demo/src/App.java",
		line:         12,
		column:       7,
		selectedText: "selected",
	}
	input := "$FilePath$|$FileDir$|$FileName$|$FileNameWithoutExtension$|$FileExt$|" +
		"$FileRelativePath$|$FileDirRelativeToProjectRoot$|$ProjectFileDir$|" +
		"$ProjectName$|$LineNumber$|$ColumnNumber$|$SelectedText$|$Prompt$|$Unknown$"
	got := expandExternalMacros(input, ctx, "answer")
	want := "/work/demo/src/App.java|/work/demo/src|App.java|App|java|" +
		"src/App.java|src|/work/demo|demo|12|7|selected|answer|$Unknown$"
	if got != want {
		t.Fatalf("expanded macros = %q\nwant %q", got, want)
	}
}

func TestExpandExternalToolBuildsACommandConfiguration(t *testing.T) {
	m := &Model{ctx: context.Background(), postHook: func(fn func()) { fn() }}
	m.root = "/work/demo"
	tool := ExternalToolPrefs{
		ID: "lint", Name: "Lint", Program: "$ProjectFileDir$/bin/lint",
		Arguments:        `--root "$ProjectFileDir$" --name "$ProjectName$"`,
		WorkingDirectory: "$ProjectFileDir$",
	}
	cfg, err := m.expandExternalTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ID != "external:lint" || cfg.Name != "Lint" || cfg.Kind != "external" ||
		cfg.Command != "/work/demo/bin/lint" {
		t.Fatalf("external tool identity = %+v", cfg)
	}
	if len(cfg.Args) != 4 || cfg.Args[1] != "/work/demo" || cfg.Args[3] != "demo" {
		t.Fatalf("external tool arguments = %q", cfg.Args)
	}
	if cfg.Cwd == nil || *cfg.Cwd != "/work/demo" {
		t.Fatalf("external tool cwd = %v", cfg.Cwd)
	}
}

func TestExternalToolsSettingsPageRendersTheNativeEditor(t *testing.T) {
	m := &Model{
		prefs: Preferences{ExternalTools: []ExternalToolPrefs{{
			ID: "lint", Name: "Lint", Group: "Quality", Enabled: true,
			Program: "golangci-lint", Arguments: `run "./..."`,
			WorkingDirectory: "$ProjectFileDir$",
		}}},
	}
	m.settingsDraft = m.settingsDraftFromPrefs()
	m.settingsExternalTool = "lint"
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsExternalToolsBody(c, oiPaletteOf(c.Theme()))
	}, 900, 620)
	tester.Frame()
	for _, text := range []string{
		"External Tools", "Lint", "Tool Settings", "Program", "Arguments",
		"Working directory", "Synchronize files after execution",
		"Open console for tool output",
	} {
		if !tester.HasText(text) {
			t.Fatalf("External Tools page lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestExternalToolSettingsAreStagedAndSaved(t *testing.T) {
	path := t.TempDir() + "/settings.json"
	m := &Model{cfg: Config{SettingsPath: path}}
	m.OpenSettingsPage(settingsPageExternalTools)
	m.addExternalTool(false)
	if len(m.settingsDraft.externalTools) != 1 || m.prefs.ExternalTools != nil {
		t.Fatalf("the new tool was not staged: draft=%+v prefs=%+v",
			m.settingsDraft.externalTools, m.prefs.ExternalTools)
	}
	m.settingsDraft.externalTools[0].Name = "Format"
	m.settingsDraft.externalTools[0].Program = "gofmt"
	m.applySettings()
	stored := LoadPreferences(path).ExternalTools
	if len(stored) != 1 || stored[0].Name != "Format" || stored[0].Program != "gofmt" ||
		!stored[0].Enabled {
		t.Fatalf("stored external tools = %+v", stored)
	}
}

func TestExternalToolsResetRestoresSavedToolsAndSelection(t *testing.T) {
	saved := []ExternalToolPrefs{{
		ID: "saved", Name: "Saved", Enabled: true, Program: "saved-tool",
	}}
	m := &Model{prefs: Preferences{ExternalTools: saved}}
	m.settingsDraft = m.settingsDraftFromPrefs()
	m.settingsPage = settingsPageExternalTools
	m.settingsDraft.externalTools = nil
	m.settingsExternalTool = ""

	m.settingsResetPage()

	if !sameExternalTools(m.settingsDraft.externalTools, saved) {
		t.Fatalf("Reset did not restore saved external tools: %+v", m.settingsDraft.externalTools)
	}
	if m.settingsExternalTool != "saved" {
		t.Fatalf("Reset selected %q, want the saved tool", m.settingsExternalTool)
	}
}

func TestExternalToolActionsFollowEnabledSettings(t *testing.T) {
	const id = "test-external-action"
	t.Cleanup(func() {
		unregisterAction("externalTools." + id)
	})
	m := &Model{prefs: Preferences{ExternalTools: []ExternalToolPrefs{{
		ID: id, Name: "Lint", Group: "Quality", Enabled: true, Program: "lint",
	}}}}
	m.syncExternalToolActions()
	found := false
	for _, action := range actionList() {
		if action.ID == "externalTools."+id {
			found = action.Title == "Lint" && action.Category == "Quality"
		}
	}
	if !found {
		t.Fatal("enabled external tool did not enter Find Action")
	}

	m.prefs.ExternalTools[0].Enabled = false
	m.syncExternalToolActions()
	for _, action := range actionList() {
		if action.ID == "externalTools."+id {
			t.Fatal("disabled external tool remained in Find Action")
		}
	}
}

func TestExternalToolPromptCancelNeverStartsAProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stateMu sync.Mutex
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { stateMu.Lock(); defer stateMu.Unlock(); fn() },
		prefs: Preferences{ExternalTools: []ExternalToolPrefs{{
			ID: "prompt", Name: "Prompted", Enabled: true, Program: "echo",
			Arguments: "$Prompt$",
		}}},
	}
	m.root = "/work/demo"
	var starts int
	m.invokeHook = func(_ context.Context, cmd string, _ any) (json.RawMessage, error) {
		if cmd == "run_start" {
			stateMu.Lock()
			starts++
			stateMu.Unlock()
		}
		return json.RawMessage(`{}`), nil
	}

	m.RunExternalTool("prompt")
	deadline := time.Now().Add(2 * time.Second)
	for {
		var waiting bool
		m.post(func() { waiting = m.externalToolPrompt != nil })
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the external tool prompt did not open")
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.finishExternalToolPrompt(false)

	deadline = time.Now().Add(2 * time.Second)
	for {
		var notified bool
		m.post(func() { notified = len(m.notify.items) > 0 })
		if notified {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancelling the prompt did not finish the tool action")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stateMu.Lock()
	gotStarts := starts
	stateMu.Unlock()
	if gotStarts != 0 {
		t.Fatalf("cancelling $Prompt$ started %d processes", gotStarts)
	}
}

func TestExternalToolSynchronizationStopsWithTheApplication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Model{ctx: ctx, postHook: func(fn func()) { fn() }}
	m.run.busy = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.waitForExternalToolThenRefresh()
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("external tool synchronization did not stop after cancellation")
	}
}
