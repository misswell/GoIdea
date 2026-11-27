package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func chooserFixture(t *testing.T) *Model {
	t.Helper()
	raw := mustJSON(t, []map[string]any{
		{"id": "maven:test", "name": "test", "kind": "maven", "command": "mvn", "args": []string{"test"}, "detected": true},
		{"id": "user:app", "name": "App", "kind": "application", "command": "java",
			"args": []string{"-cp", "${classpath}", "com.example.App"}, "module": "", "detected": true},
	})
	configs, err := openidea.DecodeRunConfigs(raw)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.configs = configs
	m.run.loaded = true
	m.run.trusted = true
	return m
}

func TestRunChooserFiltersAndSelects(t *testing.T) {
	m := chooserFixture(t)
	m.OpenRunChooser("select")
	if m.chooser == nil || m.chooser.mode != "select" {
		t.Fatalf("the chooser opened in select mode")
	}
	if len(m.chooserRows()) != 2 {
		t.Fatalf("two rows without a query, got %d", len(m.chooserRows()))
	}
	m.chooser.query = "app"
	if len(m.chooserRows()) != 1 || m.chooserRows()[0].ID != "user:app" {
		t.Fatalf("the query filters by name and kind: %d", len(m.chooserRows()))
	}
	m.chooseActivate("select")
	if m.chooser != nil {
		t.Fatalf("selecting closes the chooser")
	}
	if m.run.sel < 0 || m.run.configs[m.run.sel].ID != "user:app" {
		t.Fatalf("the selection moved to the chosen configuration")
	}
}

func TestRunChooserRunsTheChosenConfiguration(t *testing.T) {
	m := chooserFixture(t)
	started := 0
	m.invokeHook = func(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
		if cmd == "run_start" {
			started++
			return json.Marshal(openidea.RunInfo{ID: 1, Name: "test", Running: true})
		}
		if cmd == "java_resolve_launch" {
			return json.Marshal(openidea.JavaLaunchInfo{Build: strPtr("succeeded"), PathListSeparator: ":"})
		}
		return json.Marshal(nil)
	}
	m.OpenRunChooser("run")
	m.chooser.query = "test"
	m.chooseActivate("run")
	if m.chooser != nil {
		t.Fatalf("running closes the chooser")
	}
	for i := 0; i < 100 && started == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if started != 1 {
		t.Fatalf("the chosen configuration starts, got %d starts", started)
	}
}

func TestRunChooserRefusesDebugOnlyConfiguration(t *testing.T) {
	m := chooserFixture(t)
	remote := openidea.RunConfig{
		ID: "remote", Name: "Remote JVM", Kind: "remote-jvm",
		Params: map[string]any{"host": "localhost", "port": "5005"},
	}
	m.run.configs = append(m.run.configs, remote)
	started := false
	m.invokeHook = func(_ context.Context, cmd string, _ any) (json.RawMessage, error) {
		if cmd == "run_start" {
			started = true
		}
		return json.Marshal(nil)
	}
	m.OpenRunChooser("run")
	m.chooser.query = "remote"
	m.chooseActivate("run")
	if started || m.run.running || m.run.awaiting || m.run.busy {
		t.Fatal("Run launched a configuration that only supports Debug")
	}
	if m.chooser != nil {
		t.Fatal("the invalid Run left the chooser open")
	}
	if !strings.Contains(m.status, "Only Debug is available") {
		t.Fatalf("the refused launch has no explanation: status=%q", m.status)
	}
}

func TestRunLauncherRefusesDebugOnlyConfiguration(t *testing.T) {
	m := chooserFixture(t)
	remote := openidea.RunConfig{
		ID: "remote", Name: "Remote JVM", Kind: "remote-jvm",
		Params: map[string]any{"host": "localhost", "port": "5005"},
	}
	started := false
	m.invokeHook = func(_ context.Context, cmd string, _ any) (json.RawMessage, error) {
		if cmd == "run_start" {
			started = true
		}
		return json.Marshal(nil)
	}
	if m.startRunWith(remote.Raw(), remote.Name) {
		t.Fatal("the central Run launcher accepted a debug-only configuration")
	}
	if started || m.run.running || m.run.awaiting || m.run.busy {
		t.Fatal("Run launched the remote debug configuration")
	}
}

func TestDebugSelectedRefusesUnsupportedAdapters(t *testing.T) {
	m := chooserFixture(t)
	// A Kotlin adapter is not shipped: refused, not silently launched.
	raw := mustJSON(t, map[string]any{
		"id": "user:kt", "name": "Kt", "kind": "application", "command": "java",
		"args": []string{"-cp", "${classpath}", "a.MainKt"}, "debugAdapter": "kotlin-debug-adapter",
	})
	kt, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.DebugRunConfig(kt)
	if m.debug.err == "" || !strings.Contains(m.debug.err, "kotlin-debug-adapter") {
		t.Fatalf("an unshipped adapter is reported, got %q", m.debug.err)
	}
}

func TestRunChooserDebugsTheCurrentFileRow(t *testing.T) {
	m := chooserFixture(t)
	m.OpenRunChooser("debug")
	if m.chooser.mode != "debug" {
		t.Fatalf("debug mode")
	}
	// The refusal path proves the current-file row reaches DebugContext.
	m.tab() // nothing open: DebugContext reports it
	m.chooseCurrentFile()
	if m.chooser != nil {
		t.Fatalf("the chooser closed")
	}
	if m.debug.err != "Open a source file first" {
		t.Fatalf("the current-file row debugs the context, got %q", m.debug.err)
	}
	if !m.run.currentFile || m.run.inline != nil {
		t.Fatalf("the current-file debug row selects the current-file sentinel: %+v", m.run)
	}
}

func TestRunChooserRunsTheCurrentFileRow(t *testing.T) {
	m := chooserFixture(t)
	m.OpenRunChooser("run")
	m.chooseCurrentFile()
	if m.chooser != nil {
		t.Fatalf("the chooser closed")
	}
	if m.run.err != "Open a source file first" {
		t.Fatalf("the current-file row runs the context, got %q", m.run.err)
	}
	if !m.run.currentFile || m.run.inline != nil {
		t.Fatalf("the current-file row selects the current-file sentinel: %+v", m.run)
	}
}

func TestRunChooserShowsAndSavesTheTemporaryConfiguration(t *testing.T) {
	m := chooserFixture(t)
	temp, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "context:temp", "name": "current.go", "kind": "go",
		"command": "go", "args": []string{"run", "current.go"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m.run.inline = temp
	m.OpenRunChooser("run")
	rows := m.chooserRows()
	if len(rows) != 3 || rows[2].ID != temp.ID {
		t.Fatalf("the temporary configuration is a chooser row: %+v", rows)
	}
	m.editTemporaryRunConfig()
	if m.chooser != nil || m.runCfgDlg == nil ||
		m.runCfgDlg.selected() == nil || m.runCfgDlg.selected().ID != temp.ID {
		t.Fatalf("Save Temporary opens the temporary configuration in the editor")
	}
}

func TestRunChooserPinnedAndRecentRowsFollowOpenIDEA(t *testing.T) {
	raw := mustJSON(t, []map[string]any{
		{"id": "build", "name": "build", "kind": "npm", "command": "npm", "args": []string{"run", "build"}},
		{"id": "serve", "name": "serve", "kind": "npm", "command": "npm", "args": []string{"run", "serve"}},
		{"id": "App", "name": "App", "kind": "application", "command": "java", "args": []string{"App"}},
	})
	configs, err := openidea.DecodeRunConfigs(raw)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.configs = configs
	m.run.loaded = true
	m.prefs.Run = &RunPrefs{
		Pinned: []string{"serve"},
		Recent: []string{"App", "serve", "build"},
	}
	m.OpenRunChooser("run")

	sections := m.chooserSections()
	if len(sections) != 2 ||
		sections[0].label != "Pinned" || sections[0].configs[0].ID != "serve" ||
		sections[1].label != "Recent" || sections[1].configs[0].ID != "App" ||
		sections[1].configs[1].ID != "build" {
		t.Fatalf("pinned/recent/type rows = %+v", sections)
	}

	m.chooser.query = "s"
	sections = m.chooserSections()
	if len(sections) != 1 || sections[0].label != "Pinned" ||
		len(sections[0].configs) != 1 || sections[0].configs[0].ID != "serve" {
		t.Fatalf("search did not keep the pinned match on top: %+v", sections)
	}
	items := m.chooserItems()
	if len(items) != 2 || items[0].kind != runChooserConfig || items[1].kind != runChooserEdit {
		t.Fatalf("keyboard rows = %+v", items)
	}
}

func TestRunChooserPinOrderPersists(t *testing.T) {
	m := chooserFixture(t)
	m.cfg.SettingsPath = t.TempDir() + "/settings.json"
	m.togglePinnedRunConfig("user:app")
	m.togglePinnedRunConfig("maven:test")
	project := m.prefs.Run.Projects["/p"]
	if project == nil || len(project.Pinned) != 2 ||
		project.Pinned[0] != "user:app" || project.Pinned[1] != "maven:test" {
		t.Fatalf("pin order = %+v", project)
	}
	if m.prefs.Run.Pinned != nil || m.prefs.Run.Recent != nil {
		t.Fatalf("project state was written to the legacy global fields: %+v", m.prefs.Run)
	}
	stored := LoadPreferences(m.cfg.SettingsPath)
	storedProject := stored.Run.Projects["/p"]
	if storedProject == nil || len(storedProject.Pinned) != 2 ||
		storedProject.Pinned[0] != "user:app" || storedProject.Pinned[1] != "maven:test" {
		t.Fatalf("stored pins = %+v", stored.Run)
	}
	m.togglePinnedRunConfig("user:app")
	if got := storedProject.Pinned; len(got) != 2 {
		t.Fatalf("the loaded snapshot should be independent: %v", got)
	}
	if got := project.Pinned; len(got) != 1 || got[0] != "maven:test" {
		t.Fatalf("unpin order = %v", got)
	}
}

func TestRunChooserPinnedAndRecentAreProjectScoped(t *testing.T) {
	m := chooserFixture(t)
	m.cfg.SettingsPath = t.TempDir() + "/settings.json"
	m.togglePinnedRunConfig("user:app")
	m.recordRecentRunConfig("user:app")

	m.setRoot("/other")
	if got := m.runProjectPrefsState(); len(got.Pinned) != 0 || len(got.Recent) != 0 {
		t.Fatalf("another project inherited chooser state: %+v", got)
	}
	m.togglePinnedRunConfig("maven:test")
	m.recordRecentRunConfig("maven:test")

	m.setRoot("/p")
	project := m.runProjectPrefsState()
	if len(project.Pinned) != 1 || project.Pinned[0] != "user:app" ||
		len(project.Recent) != 1 || project.Recent[0] != "user:app" {
		t.Fatalf("/p chooser state = %+v", project)
	}
}

func TestRunChooserRecentIsNewestFirstUniqueAndCapped(t *testing.T) {
	m := chooserFixture(t)
	for _, id := range []string{"one", "two", "one", "three", "four", "five", "six"} {
		m.recordRecentRunConfig(id)
	}
	got := m.runProjectPrefsState().Recent
	want := []string{"six", "five", "four", "three", "one"}
	if len(got) != len(want) {
		t.Fatalf("recent = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("recent = %v, want %v", got, want)
		}
	}
	m.recordRecentRunConfig("six")
	got = m.runProjectPrefsState().Recent
	if got[0] != "six" || len(got) != 5 {
		t.Fatalf("repeating the newest recent changed order or cap: %v", got)
	}
}

func TestRunChooserDeletePersistsDetectedSuppression(t *testing.T) {
	saved := make(chan []string, 1)
	m := chooserFixture(t)
	m.invokeHook = func(_ context.Context, cmd string, args any) (json.RawMessage, error) {
		switch cmd {
		case "run_save_configs":
			payload := args.(map[string]any)
			removed, _ := payload["removedDetectedIds"].([]string)
			saved <- append([]string(nil), removed...)
			return json.RawMessage(`null`), nil
		case "run_configs":
			return json.RawMessage(`[]`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.OpenRunChooser("run")
	m.chooser.sel = m.chooserItemIndex(runChooserConfig, "maven:test")
	m.deleteChosenRunConfig()

	select {
	case removed := <-saved:
		if len(removed) != 1 || removed[0] != "maven:test" {
			t.Fatalf("removed detected ids = %v", removed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("deleting the detected configuration did not persist suppression")
	}
	for _, cfg := range m.run.configs {
		if cfg.ID == "maven:test" {
			t.Fatal("the deleted detected configuration is still in the chooser")
		}
	}
}

func TestRunChooserDeleteTemporaryDoesNotPersist(t *testing.T) {
	m := chooserFixture(t)
	temp, err := openidea.DecodeRunConfig(mustJSON(t, map[string]any{
		"id": "context:temp", "name": "current.go", "kind": "go",
		"command": "go", "args": []string{"run", "current.go"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	m.run.inline = temp
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		t.Fatal("deleting a temporary configuration reached the backend")
		return nil, nil
	}
	m.OpenRunChooser("run")
	m.chooser.sel = m.chooserItemIndex(runChooserConfig, temp.ID)
	m.deleteChosenRunConfig()
	if m.run.inline != nil {
		t.Fatal("the temporary configuration was not removed")
	}
	for _, cfg := range m.run.configs {
		if cfg.ID == temp.ID {
			t.Fatal("the temporary configuration stayed in the chooser")
		}
	}
}

func TestRunChooserF4EditsTheChosenConfiguration(t *testing.T) {
	m := chooserFixture(t)
	m.OpenRunChooser("run")
	m.chooser.sel = m.chooserItemIndex(runChooserConfig, "user:app")
	m.editChosenRunConfig()
	if m.chooser != nil || m.runCfgDlg == nil || m.runCfgDlg.selectedID() != "user:app" {
		t.Fatalf("F4 opened the wrong configuration: %+v", m.runCfgDlg)
	}
}

func TestRunChooserNumericKeysCountOnlyConfigurations(t *testing.T) {
	m := chooserFixture(t)
	m.OpenRunChooser("run")
	m.chooser.sel = m.chooserItemIndex(runChooserEdit, "")
	m.chooseNth(1, "select")
	if m.run.current() == nil || m.run.current().ID != "maven:test" {
		t.Fatalf("number 1 did not choose the first configuration")
	}
}

func TestRunChooserShiftDigitSwitchesRunToDebug(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }, ctx: context.Background()}
	m.root = "/p"
	m.run.loaded = true
	m.run.trusted = true
	m.run.configs, _ = openidea.DecodeRunConfigs(mustJSON(t, []map[string]any{{
		"id": "shell:run", "name": "run", "kind": "shell",
		"command": "echo", "args": []string{"hello"},
	}}))
	m.OpenRunChooser("run")
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()
	if !tt.HasText("Hold Shift to Debug") {
		t.Fatalf("the chooser was not drawn: %v", tt.Texts())
	}
	m.chooser.search.Focus()
	tt.Frame()
	tt.Frame()
	tt.Key(ui.Shift, ui.Key1)
	tt.Frame()
	if m.chooser != nil {
		t.Fatal("Shift+1 did not activate the first configuration")
	}
	if !strings.Contains(m.debug.err, "cannot debug Shell") {
		t.Fatalf("Shift+1 did not switch Run to Debug: %q", m.debug.err)
	}
}

func TestRunChooserFindsRunAndBackgroundDebugSessions(t *testing.T) {
	m := chooserFixture(t)
	m.run.running = true
	m.run.lastID = "maven:test"
	if got := m.chooserConfigSession("maven:test"); !got.running || got.debug {
		t.Fatalf("Run session = %+v", got)
	}
	m.run.running = false
	m.debug.sessionOrder = []uint32{42}
	m.debug.sessions = map[uint32]*debugSessionState{
		42: {session: 42, lastID: "user:app", status: "Paused"},
	}
	if got := m.chooserConfigSession("user:app"); !got.running || !got.debug || got.debugTab != 42 {
		t.Fatalf("Debug session = %+v", got)
	}
}

func TestRunChooserActiveRunActionsTargetTheChosenConfiguration(t *testing.T) {
	t.Run("rerun", func(t *testing.T) {
		m := chooserFixture(t)
		m.run.running = true
		m.run.lastID = "maven:test"
		m.run.sel = 1
		m.OpenRunChooser("run")
		session := m.chooserConfigSession("maven:test")
		m.activateChooserConfigSession("maven:test", session, true)

		if m.chooser != nil {
			t.Fatal("the chooser did not close")
		}
		if cfg := m.run.current(); cfg == nil || cfg.ID != "maven:test" {
			t.Fatalf("Rerun targeted %+v", cfg)
		}
		if m.rerunAsk == nil || m.rerunAsk.name != "test" {
			t.Fatalf("Rerun did not ask to restart the selected run: %+v", m.rerunAsk)
		}
	})

	t.Run("stop", func(t *testing.T) {
		m := chooserFixture(t)
		m.run.running = true
		m.run.lastID = "maven:test"
		m.run.runID = 7
		stopped := make(chan uint32, 1)
		m.invokeHook = func(_ context.Context, cmd string, args any) (json.RawMessage, error) {
			if cmd == "run_stop" {
				payload := args.(map[string]any)
				stopped <- payload["id"].(uint32)
			}
			return json.RawMessage(`null`), nil
		}
		m.OpenRunChooser("run")
		session := m.chooserConfigSession("maven:test")
		m.activateChooserConfigSession("maven:test", session, false)

		select {
		case id := <-stopped:
			if id != 7 {
				t.Fatalf("Stop targeted run %d", id)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Stop did not stop the selected run")
		}
		if cfg := m.run.current(); cfg == nil || cfg.ID != "maven:test" {
			t.Fatalf("Stop targeted %+v", cfg)
		}
	})
}
