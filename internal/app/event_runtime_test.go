package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

type runtimeEventBackend struct {
	info openidea.DocInfo
}

func (b *runtimeEventBackend) DocOpen(context.Context, string, string) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *runtimeEventBackend) DocEdit(context.Context, string, uint32, uint32, position.ChangeSet) (uint32, error) {
	return 0, nil
}

func (b *runtimeEventBackend) DocSnapshot(context.Context, string, uint32) (openidea.DocInfo, error) {
	return b.info, nil
}

func (b *runtimeEventBackend) DocSave(context.Context, string, uint32) error {
	return nil
}

func (b *runtimeEventBackend) DocClose(context.Context, string, uint32) error {
	return nil
}

func eventRuntimeTestModel(root string) *Model {
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
	}
	m.setRoot(root)
	return m
}

func taskByRuntimeID(t *testing.T, m *Model, id string) BackgroundTask {
	t.Helper()
	for _, task := range m.tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %q not found in %+v", id, m.tasks)
	return BackgroundTask{}
}

func runtimeDocument(t *testing.T, path, text string) *docbridge.Document {
	t.Helper()
	doc, err := docbridge.Open(context.Background(), &runtimeEventBackend{info: openidea.DocInfo{
		ID: 41, Path: path, Text: text, Version: 3,
	}}, Window, path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func runtimeTab(doc *docbridge.Document) *Tab {
	return &Tab{
		doc: doc,
		state: &editor.State{
			Buffer: doc.Buffer(),
		},
		synced: doc.Buffer().Version(),
		seen:   doc.Buffer().Version(),
	}
}

func TestRuntimeDBAndTrustEventsStayOnTheirProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	other := filepath.Join(t.TempDir(), "other")
	var callsMu sync.Mutex
	var calls []string
	m := eventRuntimeTestModel(root)
	m.db = &dbView{open: true}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		callsMu.Lock()
		calls = append(calls, command)
		callsMu.Unlock()
		switch command {
		case "db_sources":
			return json.RawMessage(`[{"id":"pg","name":"Postgres","driver":"postgres"}]`), nil
		case "db_connected":
			return json.RawMessage(`[]`), nil
		case "project_trust_list":
			return json.RawMessage(`{"projects":[],"locations":[],"restricted":[]}`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}

	if !m.acceptRuntimeEvent("dbSourcesChanged", json.RawMessage(`{"type":"dbSourcesChanged"}`)) {
		t.Fatal("dbSourcesChanged was not routed")
	}
	waitForRuntimeTest(t, func() bool { return !m.db.loading })
	if len(m.db.sources) != 1 || m.db.sources[0].ID != "pg" {
		t.Fatalf("database sources were not refreshed: %+v", m.db)
	}

	m.run.trusted = false
	m.run.configs = []openidea.RunConfig{{ID: "stale"}}
	m.acceptRuntimeEvent("projectTrustChanged", json.RawMessage(`{"type":"projectTrustChanged","root":"`+other+`","trusted":true}`))
	if m.run.trusted || len(m.run.configs) != 1 {
		t.Fatal("trust for a stale project changed the open project")
	}

	m.acceptRuntimeEvent("projectTrustChanged", json.RawMessage(`{"type":"projectTrustChanged","root":"`+root+`","trusted":true}`))
	if !m.run.trusted || m.trustAsk {
		t.Fatalf("trust grant was not applied: %+v", m.run)
	}
	m.acceptRuntimeEvent("projectTrustChanged", json.RawMessage(`{"type":"projectTrustChanged","root":"`+root+`","trusted":false}`))
	if m.run.trusted || m.run.loaded || len(m.run.configs) != 0 {
		t.Fatalf("trust revoke kept stale run state: %+v", m.run)
	}

	callsMu.Lock()
	defer callsMu.Unlock()
	if !containsRuntimeCall(calls, "db_sources") {
		t.Fatalf("runtime calls = %v", calls)
	}
}

func TestRuntimeBackendTaskCancelUsesNumericTaskIDAndDropsStaleRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	var callsMu sync.Mutex
	var calls []string
	m := eventRuntimeTestModel(root)
	m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		callsMu.Lock()
		defer callsMu.Unlock()
		calls = append(calls, command)
		if command == "cancel_task" {
			payload := args.(map[string]any)
			if got, ok := payload["id"].(uint32); !ok || got != 4294967295 {
				t.Errorf("cancel_task id = %#v, want uint32 4294967295", payload["id"])
			}
		}
		return nil, nil
	}
	m.acceptBackendTask(openidea.TaskEvent{
		ID: 4294967295, Title: "Build", Cancellable: true,
	})
	task := taskByRuntimeID(t, m, backendTaskID(4294967295))
	if task.Cancel == nil || task.Uncancellable != "" {
		t.Fatalf("cancellable task has no cancel action: %+v", task)
	}
	m.cancelTask(task.ID)
	task = taskByRuntimeID(t, m, backendTaskID(4294967295))
	if !task.Cancelling || task.Message != "Cancelling…" {
		t.Fatalf("cancelled task did not show Cancelling: %+v", task)
	}
	waitForRuntimeTest(t, func() bool {
		callsMu.Lock()
		defer callsMu.Unlock()
		return containsRuntimeCall(calls, "cancel_task")
	})

	oldRoot := m.Root()
	nextRoot := filepath.Join(t.TempDir(), "next")
	var pending []func()
	m.postHook = func(fn func()) { pending = append(pending, fn) }
	m.acceptBackendTask(openidea.TaskEvent{ID: 7, Title: "Late", Cancellable: true})
	if len(pending) != 1 {
		t.Fatalf("queued task callbacks = %d, want 1", len(pending))
	}
	m.tasks = nil
	if oldRoot == nextRoot {
		t.Fatal("fixture roots unexpectedly match")
	}
	m.setRoot(nextRoot)
	pending[0]()
	if len(m.tasks) != 0 || len(m.eventRuntime.backendTasks) != 0 {
		t.Fatalf("late task from the previous root was shown: %+v", m.tasks)
	}
}

func TestRuntimeIndexTaskIsUncancellableAndUpdatesReadiness(t *testing.T) {
	m := eventRuntimeTestModel(filepath.Join(t.TempDir(), "project"))
	m.acceptIndexState(openidea.IndexStateEvent{Indexing: true, Files: 12, Symbols: 34})
	task := taskByRuntimeID(t, m, backendIndexingID())
	if task.Cancel != nil || task.Uncancellable == "" ||
		task.Message != "12 files · 34 symbols" || task.Fraction != -1 {
		t.Fatalf("indexing task = %+v", task)
	}
	m.acceptIndexState(openidea.IndexStateEvent{Indexing: false})
	if m.eventRuntime.indexingVisible || m.eventRuntime.indexing.Indexing ||
		m.eventRuntime.indexing.Files != 0 || m.eventRuntime.indexing.Symbols != 0 {
		t.Fatalf("indexing completion was not recorded: %+v", m.eventRuntime)
	}
}

func TestRuntimeLSPRefreshInvalidatesOnlySupportedConsumers(t *testing.T) {
	m := eventRuntimeTestModel(filepath.Join(t.TempDir(), "project"))
	doc := runtimeDocument(t, m.Root()+"/Main.java", "class Main {}\n")
	tab := runtimeTab(doc)
	tab.semanticLoaded = true
	tab.semanticGeneration = 4
	tab.inlay.loaded = true
	tab.inlay.count = 2
	tab.inlay.hints = []editor.Inlay{{Offset: 0, Text: "hint"}}
	m.tabs = []*Tab{tab}
	m.active = 0

	m.acceptLSPRefresh(openidea.LSPRefreshEvent{Kind: "semanticTokens"})
	if tab.semanticLoaded || tab.semanticGeneration != 5 || tab.state.TextBackgrounds != nil {
		t.Fatalf("semantic refresh did not invalidate tokens: %+v", tab)
	}
	m.acceptLSPRefresh(openidea.LSPRefreshEvent{Kind: "inlayHints"})
	if tab.inlay.loaded || tab.inlay.count != 0 || len(tab.inlay.hints) != 0 {
		t.Fatalf("inlay refresh did not invalidate hints: %+v", tab.inlay)
	}
	m.acceptLSPRefresh(openidea.LSPRefreshEvent{Kind: "codeLens"})
	if !m.eventRuntime.unsupported["codeLens"] {
		t.Fatal("codeLens refresh was silently claimed")
	}
}

func TestRuntimeVCSChangedRefreshesAndDropsStaleDecorations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	m := eventRuntimeTestModel(root)
	doc := runtimeDocument(t, root+"/Main.java", "class Main {}\n")
	tab := runtimeTab(doc)
	tab.gitMarks = []editor.GutterMark{{Line: 3}}
	tab.gitPath = doc.Path()
	tab.inlay.inlineBlameLoaded = true
	tab.inlay.inlineBlameHasLine = true
	tab.inlay.inlineBlameGeneration = 2
	m.tabs = []*Tab{tab}
	m.active = 0
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "vcs_roots":
			return json.RawMessage(`[]`), nil
		case "vcs_status":
			return json.RawMessage(`[]`), nil
		case "vcs_branch_info":
			return json.RawMessage(`{"head":"main","ahead":0,"behind":0}`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}

	m.acceptRuntimeEvent("vcsChanged", json.RawMessage(`{"type":"vcsChanged","root":"`+filepath.Join(t.TempDir(), "other")+`"}`))
	if len(tab.gitMarks) == 0 {
		t.Fatal("stale VCS event cleared the open project")
	}
	m.acceptVCSChanged(openidea.VCSChangedEvent{Root: root})
	if len(tab.gitMarks) != 0 || tab.gitPath != "" || tab.inlay.inlineBlameLoaded ||
		tab.inlay.inlineBlameHasLine || tab.inlay.inlineBlameGeneration != 3 {
		t.Fatalf("VCS refresh left stale decorations: %+v / %+v", tab, tab.inlay)
	}
}

func TestRuntimeNotifyAndProjectRootsUseNativeState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	var callsMu sync.Mutex
	var calls []string
	m := eventRuntimeTestModel(root)
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		callsMu.Lock()
		calls = append(calls, command)
		callsMu.Unlock()
		switch command {
		case "build_model":
			return json.RawMessage(`{"maven":{"projects":[]},"gradle":[]}`), nil
		case "settings_get":
			return json.RawMessage(`{}`), nil
		case "settings_get_global":
			return json.RawMessage(`{}`), nil
		case "languages":
			return json.RawMessage(`[]`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}
	m.acceptNotify(openidea.NotifyEvent{Level: "warning", Title: "Index", Body: runtimeStringPtr("Paused")})
	if len(m.notify.items) != 1 ||
		m.notify.items[0].level != "warning" ||
		m.notify.items[0].title != "Index" ||
		m.notify.items[0].body != "Paused" {
		t.Fatalf("native notification = %+v", m.notify.items)
	}

	m.acceptRuntimeEvent("projectRootsChanged", json.RawMessage(`{"type":"projectRootsChanged","root":"`+root+`"}`))
	waitForRuntimeTest(t, func() bool {
		callsMu.Lock()
		defer callsMu.Unlock()
		return containsRuntimeCall(calls, "build_model")
	})
	if !m.projectStructure.loading && !m.projectStructure.loaded {
		t.Fatal("project roots did not refresh Project Structure")
	}
}

func TestBackendNotificationRecoveryUsesNativeSettingsAndJDKRoutes(t *testing.T) {
	actions, body := backendNotificationRecovery("cannot start /opt/mvn/bin/mvn: os error 2")
	if !strings.HasPrefix(body, "Maven could not be started.") || len(actions) != 1 ||
		actions[0].label != "Open Maven Settings" {
		t.Fatalf("Maven recovery = %q, %+v", body, actions)
	}
	if actions[0].run == nil {
		t.Fatal("Maven recovery has no action")
	}

	actions, body = backendNotificationRecovery(
		"Can't use Java 17 and Gradle 9.0 to import Gradle project. jdtls needs JDK 21 or newer.",
	)
	if !strings.HasPrefix(body, "Choose a JDK compatible") || len(actions) != 2 ||
		actions[0].label != "Download JDK 21" ||
		actions[1].label != "Choose Gradle JVM" {
		t.Fatalf("Gradle/JDK recovery = %q, %+v", body, actions)
	}

	actions, body = backendNotificationRecovery(
		"tsserver stopped because it reached its memory limit",
	)
	if body != "tsserver stopped because it reached its memory limit" ||
		len(actions) != 1 || actions[0].label != "Language Servers…" {
		t.Fatalf("tsserver recovery = %q, %+v", body, actions)
	}
}

func TestRuntimeSettingsChangedCoalescesAndDropsStaleTimers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	m := eventRuntimeTestModel(root)
	var callsMu sync.Mutex
	var calls []string
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		callsMu.Lock()
		calls = append(calls, command)
		callsMu.Unlock()
		if command == "languages" {
			return json.RawMessage(`[]`), nil
		}
		return json.RawMessage(`{}`), nil
	}
	var scheduledMu sync.Mutex
	var scheduled []func()
	oldSchedule := settingsEventSchedule
	settingsEventSchedule = func(_ time.Duration, fn func()) func() {
		scheduledMu.Lock()
		defer scheduledMu.Unlock()
		scheduled = append(scheduled, fn)
		return func() {}
	}
	t.Cleanup(func() { settingsEventSchedule = oldSchedule })

	m.acceptSettingsChanged(openidea.SettingsChangedEvent{Key: "editor.fontSize"})
	m.acceptSettingsChanged(openidea.SettingsChangedEvent{Key: "editor.inlay.codeVisionUsages"})
	scheduledMu.Lock()
	if len(scheduled) != 2 {
		t.Fatalf("settings timers = %d, want 2", len(scheduled))
	}
	first, second := scheduled[0], scheduled[1]
	scheduledMu.Unlock()
	first()
	callsMu.Lock()
	if containsRuntimeCall(calls, "settings_get_global") {
		callsMu.Unlock()
		t.Fatal("a superseded settings event still reloaded runtime settings")
	}
	callsMu.Unlock()

	second()
	waitForRuntimeTest(t, func() bool {
		callsMu.Lock()
		defer callsMu.Unlock()
		return containsRuntimeCall(calls, "settings_get_global")
	})

	m.setRoot(filepath.Join(t.TempDir(), "next"))
	before := len(calls)
	second()
	time.Sleep(10 * time.Millisecond)
	if len(calls) != before {
		t.Fatal("settings timer changed a stale project")
	}
}

func TestRuntimeAskpassQueuesAndDropsWithoutClaimingAnAnswerRPC(t *testing.T) {
	m := eventRuntimeTestModel(filepath.Join(t.TempDir(), "project"))
	event := openidea.AskpassEvent{
		ID: 4294967297, Prompt: "Password:", Kind: "secret", Remember: true,
	}
	m.acceptAskpass(event)
	m.acceptAskpass(event)
	if len(m.eventRuntime.askpass) != 1 {
		t.Fatalf("duplicate askpass prompt was queued: %+v", m.eventRuntime.askpass)
	}
	got, ok := m.currentAskpass()
	if !ok || got != (askpassPrompt{
		id: event.ID, prompt: event.Prompt, kind: event.Kind, remember: event.Remember,
	}) {
		t.Fatalf("askpass prompt = %+v, %v", got, ok)
	}
	m.dropAskpass(event.ID)
	if len(m.eventRuntime.askpass) != 0 {
		t.Fatalf("askpassDone did not remove the prompt: %+v", m.eventRuntime.askpass)
	}
	if strings.Contains(strings.ToLower(m.askpassDialogText()), "submit answer") {
		t.Fatal("askpass UI claims an answer route the stdio backend does not expose")
	}
}

func (m *Model) askpassDialogText() string {
	m.acceptAskpass(openidea.AskpassEvent{ID: 9, Prompt: "Passphrase:", Kind: "secret"})
	var texts []string
	state := &m.eventRuntime
	for _, prompt := range state.askpass {
		texts = append(texts, prompt.prompt)
	}
	texts = append(texts, "This backend connection exposes credential prompts but does not expose the desktop-only answer command.")
	return strings.Join(texts, " ")
}

func TestRuntimeExcludedDirsRefreshOnlyLoadedTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(root, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := eventRuntimeTestModel(root)
	m.tree = NewTree(root, m.post, m.invalidate)
	m.acceptExcludedDirsChanged(openidea.ExcludedDirsChangedEvent{
		Dirs: []string{filepath.Join(root, "target"), filepath.Join(t.TempDir(), "outside")},
	})
	target := filepath.Join(root, "target")
	if !m.tree.isDir(target) || !excludedDir(filepath.Base(target)) {
		t.Fatal("loaded excluded directory is missing from the project tree")
	}
}

func runtimeStringPtr(value string) *string { return &value }

func containsRuntimeCall(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func waitForRuntimeTest(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("runtime condition did not settle")
}
