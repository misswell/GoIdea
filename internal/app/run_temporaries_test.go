package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func temporaryRunConfig(t *testing.T, id, command string, args ...string) openidea.RunConfig {
	t.Helper()
	cfg, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": id, "name": id, "kind": "shell", "command": command, "args": args,
	}))
	if err != nil {
		t.Fatal(err)
	}
	return *cfg
}

func TestRunTemporariesAreProjectScopedPersistedAndCapped(t *testing.T) {
	m := &Model{
		postHook: func(fn func()) { fn() },
		ctx:      context.Background(),
	}
	m.root = "/p"
	m.cfg.SettingsPath = t.TempDir() + "/settings.json"
	m.run.loaded = true
	m.run.trusted = true
	for i := 1; i <= 6; i++ {
		m.adoptRunTemporary(temporaryRunConfig(t, "tmp:"+string(rune('0'+i)), "echo", string(rune('0'+i))))
	}
	if len(m.run.temporaries) != maxRunTemporaries {
		t.Fatalf("temporary count = %d", len(m.run.temporaries))
	}
	want := []string{"tmp:2", "tmp:3", "tmp:4", "tmp:5", "tmp:6"}
	for i, id := range want {
		if m.run.temporaries[i].ID != id {
			t.Fatalf("temporary order = %+v", m.run.temporaries)
		}
	}

	stored := LoadPreferences(m.cfg.SettingsPath)
	project := stored.Run.Projects["/p"]
	if project == nil || len(project.Temporary) != 5 || project.Selected != "tmp:6" {
		t.Fatalf("stored run project = %+v", project)
	}

	m.setRoot("/other")
	if temporaries, selected := m.storedRunTemporaries(); len(temporaries) != 0 || selected != "" {
		t.Fatalf("another project inherited temporaries: %+v selected=%q", temporaries, selected)
	}
}

func TestRunTemporaryAdoptionReusesSavedApplications(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.loaded = true
	m.run.trusted = true
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "user:app", "name": "Saved Main", "kind": "application",
		"command": "java", "args": []string{"-cp", "${classpath}", "demo.Main"},
		"module": "app",
	}}))
	generated, _ := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "context:file:src/Main.java", "name": "Main", "kind": "application",
		"command": "java", "args": []string{"-cp", "${classpath}", "demo.Main"},
		"module": "app",
	}))
	adopted := m.adoptRunTemporary(*generated)
	if adopted == nil || adopted.ID != "user:app" {
		t.Fatalf("adopted = %+v", adopted)
	}
	if len(m.run.temporaries) != 0 || m.run.current() == nil || m.run.current().ID != "user:app" {
		t.Fatalf("saved application was not reused: %+v", m.run)
	}
}

func TestRunTemporaryEvictionKeepsBusyConfigurations(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.loaded = true
	m.run.trusted = true
	m.adoptRunTemporary(temporaryRunConfig(t, "tmp:busy", "echo", "busy"))
	m.run.running = true
	m.run.lastID = "tmp:busy"
	for i := 1; i <= 5; i++ {
		m.adoptRunTemporary(temporaryRunConfig(t, "tmp:"+string(rune('a'+i)), "echo", string(rune('a'+i))))
	}
	ids := make([]string, len(m.run.temporaries))
	for i, cfg := range m.run.temporaries {
		ids[i] = cfg.ID
	}
	if len(ids) != 5 || ids[0] != "tmp:busy" || ids[4] != "tmp:f" {
		t.Fatalf("busy-aware eviction = %v", ids)
	}
}

func TestRunTemporariesReloadFromSettingsAndApplyToolchain(t *testing.T) {
	storedOne := temporaryRunConfig(t, "tmp:one", "echo", "one")
	storedCollision := temporaryRunConfig(t, "saved:one", "echo", "collision")
	applied := make(chan int, 1)
	m := &Model{
		postHook: func(fn func()) { fn() },
		ctx:      context.Background(),
	}
	m.root = "/p"
	m.cfg.SettingsPath = t.TempDir() + "/settings.json"
	m.prefs.Run = &RunPrefs{Projects: map[string]*RunProjectPrefs{
		"/p": {
			Temporary: []openidea.RunConfig{storedOne, storedCollision},
			Selected:  "tmp:one",
		},
	}}
	m.invokeHook = func(_ context.Context, cmd string, args any) (json.RawMessage, error) {
		switch cmd {
		case "run_configs":
			return mustJSON(t, []map[string]any{{
				"id": "saved:one", "name": "saved", "kind": "shell",
				"command": "echo", "args": []string{"saved"}, "detected": true,
			}}), nil
		case "run_apply_settings":
			configs := args.(map[string]any)["configs"].([]*openidea.RunConfig)
			applied <- len(configs)
			return json.Marshal(configs)
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.loadRunConfigs()
	for i := 0; i < 100 && (!m.run.loaded || m.run.inline == nil); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case count := <-applied:
		if count != 1 {
			t.Fatalf("run_apply_settings received %d configurations", count)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stored temporary configuration did not reapply toolchain settings")
	}
	if len(m.run.temporaries) != 1 || m.run.temporaries[0].ID != "tmp:one" {
		t.Fatalf("loaded temporaries = %+v", m.run.temporaries)
	}
	if m.run.selectedID() != "tmp:one" {
		t.Fatalf("selected configuration = %q", m.run.selectedID())
	}
}

func TestRunTemporaryPersistenceStripsDerivedToolchainValues(t *testing.T) {
	m := &Model{
		postHook: func(fn func()) { fn() },
		ctx:      context.Background(),
	}
	m.root = "/p"
	m.cfg.SettingsPath = t.TempDir() + "/settings.json"
	invoked := make(chan struct{}, 1)
	m.invokeHook = func(_ context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd != "run_without_derived" {
			return nil, fmt.Errorf("unexpected command %q", cmd)
		}
		configs := args.(map[string]any)["configs"].([]*openidea.RunConfig)
		kept := make([]openidea.RunConfig, 0, len(configs))
		for _, cfg := range configs {
			clone := *cfg
			clone.Command = "mvn"
			clone.Args = []string{"test"}
			kept = append(kept, clone)
		}
		invoked <- struct{}{}
		return json.Marshal(kept)
	}

	cfg, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "tmp:maven", "name": "test", "kind": "maven",
		"command": "/Applications/IDEA/jbr/bin/java",
		"args":    []string{"-Xmx2g", "test"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m.run.temporaries = runConfigPointers([]openidea.RunConfig{*cfg})
	m.run.inline = m.run.temporaries[0]
	m.persistRunTemporaries()
	select {
	case <-invoked:
	case <-time.After(2 * time.Second):
		t.Fatal("temporary persistence did not ask the backend to strip derived settings")
	}
	for i := 0; i < 100; i++ {
		project := LoadPreferences(m.cfg.SettingsPath).Run.Projects["/p"]
		if project != nil && len(project.Temporary) == 1 &&
			project.Temporary[0].Command == "mvn" &&
			len(project.Temporary[0].Args) == 1 &&
			project.Temporary[0].Args[0] == "test" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("derived toolchain values stayed in the persisted temporary configuration")
}

func TestRunTemporaryContextsDropWhenSourceIsRemoved(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	a := temporaryRunConfig(t, "context:file:src/a.go", "go", "run", "src/a.go")
	aBuild := temporaryRunConfig(t, "context:build:src/a.go#1", "go", "build", "src/a.go")
	b := temporaryRunConfig(t, "context:file:src/b.go", "go", "run", "src/b.go")
	m.run.temporaries = runConfigPointers([]openidea.RunConfig{aBuild, a, b})
	m.run.inline = m.run.temporaries[1]
	m.run.pendingInline = m.run.temporaries[0]
	m.dropStaleRunTemporaries([]string{"/p/src/a.go"})
	if len(m.run.temporaries) != 1 || m.run.temporaries[0].ID != "context:file:src/b.go" {
		t.Fatalf("stale context configurations = %+v", m.run.temporaries)
	}
	if m.run.inline != nil || m.run.pendingInline != nil {
		t.Fatalf("stale selected context stayed selected: %+v", m.run)
	}
	if m.run.selectedID() != "" {
		t.Fatalf("stale context fell back to an unrelated configuration: %q", m.run.selectedID())
	}
}

func TestRemovingSelectedTemporaryClearsTheSelection(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.cfg.SettingsPath = t.TempDir() + "/settings.json"
	saved := temporaryRunConfig(t, "saved", "echo", "saved")
	current := temporaryRunConfig(t, "tmp:current", "echo", "current")
	m.run.configs = []openidea.RunConfig{saved}
	m.run.sel = 0
	m.run.temporaries = runConfigPointers([]openidea.RunConfig{current})
	m.run.inline = m.run.temporaries[0]
	m.persistRunSelection()

	m.removeRunTemporary(current.ID)
	if m.run.selectedID() != "" {
		t.Fatalf("deleting the selected temporary selected %q", m.run.selectedID())
	}
	project := LoadPreferences(m.cfg.SettingsPath).Run.Projects["/p"]
	if project == nil || project.Selected != "" {
		t.Fatalf("deleted temporary selection was persisted: %+v", project)
	}
}
