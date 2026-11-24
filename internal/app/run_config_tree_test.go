package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func runTreeTestConfig(t *testing.T, id, name, kind, folder string) *openidea.RunConfig {
	t.Helper()
	obj := map[string]any{
		"id": id, "name": name, "kind": kind, "command": name, "args": []string{},
	}
	if folder != "" {
		obj["folder"] = folder
	}
	cfg, err := openidea.DecodeRunConfig(mustJSON(t, obj))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRunConfigTreeGroupsFoldersAndLooseConfigurations(t *testing.T) {
	folder := "server"
	d := &runConfigDlg{
		items: []*openidea.RunConfig{
			runTreeTestConfig(t, "shell:top", "top", "shell", ""),
			runTreeTestConfig(t, "maven:build", "build", "maven", folder),
			runTreeTestConfig(t, "shell:serve", "serve", "shell", folder),
			runTreeTestConfig(t, "maven:test", "test", "maven", ""),
		},
		extraFolders:    []runConfigFolder{{Kind: "shell", Name: "empty"}},
		collapsedGroups: map[string]bool{},
		collapsedFolder: map[string]bool{},
	}
	rows := runConfigTreeRows(d)
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.Key)
	}
	want := []string{
		"group:shell",
		"folder:shell:server",
		"config:shell:serve",
		"folder:shell:empty",
		"config:shell:top",
		"group:maven",
		"folder:maven:server",
		"config:maven:build",
		"config:maven:test",
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
	if rows[2].Depth != 2 || rows[4].Depth != 1 {
		t.Fatalf("folder and loose configuration depths differ: %+v", rows)
	}
}

func TestRunConfigTreeCollapsesGroupsAndFolders(t *testing.T) {
	d := &runConfigDlg{
		items:           []*openidea.RunConfig{runTreeTestConfig(t, "maven:build", "build", "maven", "server")},
		collapsedGroups: map[string]bool{"group:maven": true},
		collapsedFolder: map[string]bool{},
	}
	if rows := runConfigTreeRows(d); len(rows) != 1 || rows[0].Kind != runConfigGroupRow {
		t.Fatalf("collapsed group rows = %+v", rows)
	}
	d.collapsedGroups["group:maven"] = false
	d.collapsedFolder["folder:maven:server"] = true
	rows := runConfigTreeRows(d)
	if len(rows) != 2 || rows[1].Kind != runConfigFolderRow {
		t.Fatalf("collapsed folder rows = %+v", rows)
	}
}

func TestRunConfigFolderLifecycleMovesAndKeepsConfigurations(t *testing.T) {
	cfg := runTreeTestConfig(t, "shell:run", "run", "shell", "")
	d := &runConfigDlg{
		open:            true,
		items:           []*openidea.RunConfig{cfg},
		sel:             0,
		collapsedGroups: map[string]bool{},
		collapsedFolder: map[string]bool{},
	}
	m := &Model{runCfgDlg: d}
	d.loadForm()
	m.NewRunConfigFolder()
	cfg = d.items[0]
	if len(d.extraFolders) != 1 || cfg.Folder == nil || *cfg.Folder != "New Folder" {
		t.Fatalf("new folder did not receive the selected configuration: %+v", d.extraFolders)
	}
	if d.folderKind != "shell" || d.folderName != "New Folder" {
		t.Fatalf("new folder was not selected: %q %q", d.folderKind, d.folderName)
	}
	m.RenameRunConfigFolder("Backend")
	if cfg.Folder == nil || *cfg.Folder != "Backend" || d.extraFolders[0].Name != "Backend" {
		t.Fatalf("folder rename did not move its members: %+v", cfg.Folder)
	}
	m.RemoveRunConfigFolder()
	if cfg.Folder != nil || len(d.extraFolders) != 0 || d.folderKind != "" {
		t.Fatalf("folder removal deleted or stranded its configuration: %+v", cfg.Folder)
	}
	if len(d.items) != 1 || d.items[0].ID != cfg.ID {
		t.Fatalf("folder removal changed the configuration list: %+v", d.items)
	}
}

func TestRunConfigMoveAndDropStayWithinKindAndFolder(t *testing.T) {
	build := "Build"
	first := runTreeTestConfig(t, "maven:a", "A", "maven", build)
	second := runTreeTestConfig(t, "maven:b", "B", "maven", build)
	loose := runTreeTestConfig(t, "maven:c", "C", "maven", "")
	other := runTreeTestConfig(t, "shell:d", "D", "shell", build)
	d := &runConfigDlg{
		items:           []*openidea.RunConfig{first, second, loose, other},
		sel:             1,
		collapsedGroups: map[string]bool{},
		collapsedFolder: map[string]bool{},
	}
	m := &Model{runCfgDlg: d}
	d.loadForm()
	m.MoveRunConfig(-1)
	if d.items[0].ID != second.ID || d.items[1].ID != first.ID {
		t.Fatalf("same-folder move did not swap siblings: %s, %s", d.items[0].ID, d.items[1].ID)
	}
	d.sel = 2
	d.loadForm()
	m.MoveRunConfig(-1)
	if d.items[2].ID != loose.ID {
		t.Fatalf("a loose configuration crossed folder boundaries: %s", d.items[2].ID)
	}
}

func TestRunConfigDropRejectsDifferentKindAndAcceptsSameKind(t *testing.T) {
	cfg := runTreeTestConfig(t, "maven:a", "A", "maven", "")
	other := runTreeTestConfig(t, "shell:b", "B", "shell", "")
	d := &runConfigDlg{items: []*openidea.RunConfig{cfg, other}, sel: 0}
	if d.moveConfigToFolder(cfg.ID, "shell", "Wrong") {
		t.Fatal("a configuration moved under a different type group")
	}
	if cfg.Folder != nil {
		t.Fatalf("rejected drop changed the folder: %+v", cfg.Folder)
	}
	if !d.moveConfigToFolder(cfg.ID, "maven", "Server") || cfg.Folder == nil || *cfg.Folder != "Server" {
		t.Fatalf("same-kind drop did not move the configuration: %+v", cfg.Folder)
	}
}

type launchOrderHook struct {
	mu     sync.Mutex
	calls  []string
	saved  bool
	config json.RawMessage
}

func (h *launchOrderHook) invoke(_ context.Context, cmd string, args any) (json.RawMessage, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch cmd {
	case "run_templates", "plugin_run_targets":
		return json.RawMessage(`[]`), nil
	case "settings_get":
		return json.RawMessage(`{"runTargets":[],"run":{}}`), nil
	case "run_save_configs":
		h.calls = append(h.calls, "save")
		h.saved = true
		payload := args.(map[string]any)
		configs := payload["configs"].([]map[string]any)
		raw, _ := json.Marshal(configs)
		h.config = raw
		return json.RawMessage(`null`), nil
	case "run_configs":
		h.calls = append(h.calls, "reload")
		if h.config == nil {
			return json.RawMessage(`[]`), nil
		}
		return h.config, nil
	case "run_start":
		h.calls = append(h.calls, "start")
		if !h.saved {
			return nil, context.Canceled
		}
		return json.RawMessage(`{"id":7,"pid":70,"running":true}`), nil
	default:
		return json.RawMessage(`null`), nil
	}
}

func (h *launchOrderHook) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.calls...)
}

func TestRunFooterSavesBeforeLaunchingSelectedConfiguration(t *testing.T) {
	hook := &launchOrderHook{}
	m := &Model{
		ctx:        context.Background(),
		invokeHook: hook.invoke,
		postHook:   func(fn func()) { fn() },
	}
	m.root = "/p"
	m.run.trusted = true
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "shell:run", "name": "run", "kind": "shell", "command": "echo",
		"args": []string{"hello"}, "detected": false,
	}}))
	m.OpenRunConfigs()
	m.runCfgDlg.name = "run edited"
	m.RunSelectedRunConfig()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		calls := hook.snapshot()
		if len(calls) >= 2 && calls[len(calls)-1] == "start" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls := hook.snapshot()
	saveAt, startAt := -1, -1
	for index, call := range calls {
		switch call {
		case "save":
			saveAt = index
		case "start":
			startAt = index
		}
	}
	if saveAt < 0 || startAt < 0 || saveAt > startAt {
		t.Fatalf("Run footer did not save before run_start: %v", calls)
	}
	if m.runCfgDlg != nil {
		t.Fatal("Run footer left the dialog open")
	}
}

func TestRunFooterDoesNotLaunchAnInvalidConfiguration(t *testing.T) {
	hook := &launchOrderHook{}
	m := &Model{
		ctx:        context.Background(),
		invokeHook: hook.invoke,
		postHook:   func(fn func()) { fn() },
	}
	m.root = "/p"
	m.run.trusted = true
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "shell:run", "name": "run", "kind": "shell", "command": "echo",
		"args": []string{}, "detected": false,
	}}))
	m.OpenRunConfigs()
	m.runCfgDlg.name = ""
	m.RunSelectedRunConfig()
	if calls := hook.snapshot(); len(calls) != 0 {
		t.Fatalf("invalid configuration reached the backend: %v", calls)
	}
	if m.runCfgDlg == nil || m.runCfgDlg.launchAfter != "" {
		t.Fatal("invalid Run footer left a pending launch")
	}
}
