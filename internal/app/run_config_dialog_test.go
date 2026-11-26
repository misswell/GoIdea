package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
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
			items := append([]map[string]any(nil), h.saved[len(h.saved)-1]...)
			items = append(items, mvn)
			return json.Marshal(items)
		}
		return json.Marshal([]map[string]any{mvn})
	case "run_templates", "plugin_run_targets":
		return json.RawMessage(`[]`), nil
	case "settings_get":
		return json.RawMessage(`{"runTargets":[],"run":{}}`), nil
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
	if _, ok := tt.Find("Run/Debug Configurations"); !ok {
		t.Fatalf("the Edit Configurations sheet was not drawn: %v", tt.Texts())
	}
}

func TestRunConfigDialogCancelDoesNotMutateSavedConfigurations(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "shell:run", "name": "run", "kind": "shell", "command": "echo",
		"args": []string{"hello"}, "detected": false,
		"beforeLaunch": []map[string]any{{"kind": "build"}},
	}}))
	m.OpenRunConfigs()
	draft := m.runCfgDlg.items[0]
	draft.Name = "changed"
	draft.Command = "printf"
	draft.Local = true
	draft.BeforeLaunch = append(draft.BeforeLaunch, openidea.BeforeLaunchTask{Kind: "command", Target: "make"})
	m.closeRunConfigs()

	saved := m.run.configs[0]
	if saved.Name != "run" || saved.Command != "echo" || saved.Local ||
		len(saved.BeforeLaunch) != 1 || saved.BeforeLaunch[0].Kind != "build" {
		t.Fatalf("Cancel leaked draft edits into the saved configurations: %+v", saved)
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

func TestRunConfigDialogKeepsUntouchedDetectedConfigurationDetected(t *testing.T) {
	hook := &runCfgHook{}
	m := &Model{invokeHook: hook.hook, postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn",
		"args": []string{"test"}, "detected": true,
	}}))
	m.OpenRunConfigs()
	d := m.runCfgDlg
	d.applyForm()
	if !d.items[0].Detected {
		t.Fatalf("applying the untouched form created a user override: %+v", d.items[0])
	}
	m.ApplyRunConfigs()
	for _, call := range hook.calls {
		if call == "run_save_configs" {
			t.Fatalf("an unchanged detected configuration reached run_save_configs: %v", hook.calls)
		}
	}
}

func TestRunConfigDialogApplyPreservesAnUnchangedTemporaryConfiguration(t *testing.T) {
	hook := &runCfgHook{}
	m := &Model{invokeHook: hook.hook, postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	temp, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "context:temp", "name": "current.go", "kind": "go",
		"command": "go", "args": []string{"run", "current.go"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m.run.inline = temp
	m.OpenRunConfigs()
	d := m.runCfgDlg
	m.AddRunConfig()
	d.items[d.sel].Command = "echo"
	d.items[d.sel].Name = "Persistent"
	m.ApplyRunConfigs()
	for i := 0; i < 100 && len(hook.saved) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if len(hook.saved) != 1 {
		t.Fatalf("Apply persisted the new configuration: %v, errors: %v / %q", hook.saved, d.errs, d.saveErr)
	}
	for _, saved := range hook.saved[0] {
		if saved["id"] == temp.ID {
			t.Fatalf("an unchanged temporary configuration was saved: %v", saved)
		}
	}
	if !d.temporary[temp.ID] || m.runTemporaryByID(temp.ID) == nil {
		t.Fatalf("the untouched temporary configuration stayed temporary: temp=%+v saved=%v",
			d.temporary, hook.saved)
	}
}

func TestRunConfigDialogApplyPromotesAnEditedTemporaryConfiguration(t *testing.T) {
	hook := &runCfgHook{}
	m := &Model{invokeHook: hook.hook, postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	temp, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "context:temp", "name": "current.go", "kind": "go",
		"command": "go", "args": []string{"run", "current.go"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m.run.inline = temp
	m.OpenRunConfigs()
	d := m.runCfgDlg
	d.name = "Saved current.go"
	m.ApplyRunConfigs()
	for i := 0; i < 100 && len(hook.saved) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if len(hook.saved) != 1 || len(hook.saved[0]) != 1 || hook.saved[0][0]["id"] != temp.ID {
		t.Fatalf("the edited temporary configuration was not saved: %v, errors: %v / %q", hook.saved, d.errs, d.saveErr)
	}
	if d.temporary[temp.ID] || m.runTemporaryByID(temp.ID) != nil {
		t.Fatalf("the saved configuration is no longer temporary")
	}
}

func TestRunConfigDialogAppliesBuildToolForm(t *testing.T) {
	hook := &runCfgHook{}
	m := &Model{invokeHook: hook.hook, postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "gradle:assemble", "name": "assemble", "kind": "gradle",
		"command": "./gradlew", "args": []string{"assemble"}, "detected": true,
		"params": map[string]string{"tasks": "assemble", "wrapper": "true"},
	}}))
	m.OpenRunConfigs()
	d := m.runCfgDlg
	d.typeParams["tasks"] = "clean test"
	d.typeParams["arguments"] = "-q"
	d.typeParams["projectDir"] = "service"
	d.typeParams["wrapper"] = "false"
	d.typeParams["vmOptions"] = "-Xmx1g"
	d.typeParams["activeProfiles"] = "dev"
	d.applyForm()
	cfg := d.items[0]
	if cfg.Detected || cfg.Command != "gradle" ||
		strings.Join(cfg.Args, "\x00") != "clean\x00test\x00-q" {
		t.Fatalf("build tool command was not rebuilt: %+v", cfg)
	}
	if cfg.Cwd == nil || *cfg.Cwd != "service" ||
		cfg.Params["vmOptions"] != "-Xmx1g" ||
		cfg.Params["activeProfiles"] != "dev" {
		t.Fatalf("build tool params were not retained: %+v", cfg)
	}
}

func TestRunConfigTemplateStaysCleanUntilItsFieldsChange(t *testing.T) {
	template, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "template:maven", "name": "Maven", "kind": "maven",
		"command": "mvn", "args": []string{"test"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	d := &runConfigDlg{
		templateMode:      true,
		templateKind:      "maven",
		templates:         []*openidea.RunConfig{template},
		templatesBaseline: runTemplatesBaseline([]*openidea.RunConfig{template}),
	}
	d.loadTemplateForm()
	d.applyTemplateForm()
	if d.templatesDirty {
		t.Fatal("an untouched template was marked dirty")
	}
	d.typeParams["tasks"] = "verify"
	d.applyTemplateForm()
	if !d.templatesDirty {
		t.Fatal("an edited template was not marked dirty")
	}
	d.typeParams["tasks"] = "test"
	d.applyTemplateForm()
	if d.templatesDirty {
		t.Fatal("changing a template back to its baseline stayed dirty")
	}
}

func TestBeforeLaunchTaskEditingPreservesOrder(t *testing.T) {
	cfg := &openidea.RunConfig{BeforeLaunch: []openidea.BeforeLaunchTask{
		{Kind: "build"},
		{Kind: "command", Target: "make"},
		{Kind: "toolWindow", Target: "run"},
	}}
	moveBeforeLaunchTask(cfg, 2, -1)
	if cfg.BeforeLaunch[1].Kind != "toolWindow" || cfg.BeforeLaunch[2].Target != "make" {
		t.Fatalf("move did not preserve the edited order: %+v", cfg.BeforeLaunch)
	}
	removeBeforeLaunchTask(cfg, 1)
	if len(cfg.BeforeLaunch) != 2 || cfg.BeforeLaunch[1].Kind != "command" {
		t.Fatalf("remove changed the remaining tasks: %+v", cfg.BeforeLaunch)
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

func TestRerunFailedUsesTheSelectedChildTab(t *testing.T) {
	main := &serialMainThread{}
	m := &Model{
		ctx:      context.Background(),
		postHook: main.post,
	}
	m.setRoot(t.TempDir())
	m.run.trusted, m.run.loaded = true, true
	base := openidea.RunConfig{
		ID: "maven:selected", Name: "Selected tests", Kind: "maven",
		Command: "mvn", Args: []string{"test"}, IsTest: true,
	}
	other := openidea.RunConfig{
		ID: "maven:other", Name: "Other tests", Kind: "maven",
		Command: "mvn", Args: []string{"test"}, IsTest: true,
	}
	results := adapter.NewTestResults()
	results.Apply([]adapter.TestEvent{{
		Kind: "result", Path: []string{"demo.SelectedTest", "passes"}, Status: adapter.TestPassed,
	}, {
		Kind: "result", Path: []string{"demo.SelectedTest", "fails"}, Status: adapter.TestFailed,
	}})
	exit := int32(1)
	selected := &runChild{
		channel: 21, runID: 41, name: base.Name, config: base,
		finished: true, exitCode: &exit, started: time.Now(), ended: time.Now(),
		tests: results,
	}
	otherResults := adapter.NewTestResults()
	otherResults.Apply([]adapter.TestEvent{{
		Kind: "result", Path: []string{"demo.OtherTest", "fails"}, Status: adapter.TestFailed,
	}})
	otherChild := &runChild{
		channel: 22, runID: 42, name: other.Name, config: other,
		finished: true, exitCode: &exit, started: time.Now(), ended: time.Now(),
		tests: otherResults,
	}
	m.run.children = []*runChild{selected, otherChild}
	m.run.activeChild = selected.channel
	m.run.agentActive = 0

	var launched openidea.RunConfig
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "run_start" {
			return json.RawMessage(`null`), nil
		}
		raw, _ := json.Marshal(args.(map[string]any)["config"])
		if err := json.Unmarshal(raw, &launched); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"id":43,"running":true,"name":"Selected tests"} `), nil
	}

	m.RerunFailedTestsFor("run")
	waitForSerialMainThread(t, main, time.Second, func() bool {
		return launched.ID != ""
	})
	if !sameStringSlice(launched.Args, []string{
		"test", "-Dtest=demo.SelectedTest#fails",
	}) {
		t.Fatalf("selected child rerun args = %q", launched.Args)
	}
}
