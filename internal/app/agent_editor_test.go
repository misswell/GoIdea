package app

import (
	"path/filepath"
	"reflect"
	"testing"
)

func agentEditorModel(t *testing.T, location string) *Model {
	t.Helper()
	m := &Model{
		prefs: Preferences{
			AgentWorkspace: AgentWorkspacePrefs{SessionLocation: location},
		},
		agents: AgentsPane{
			tasks: map[string]agentTask{
				"a": agentTestTask("a", "Task A", "running", 2),
			},
			entries:       map[string][]agentEntry{"a": {{Type: "agent", Text: "done"}}},
			entriesLoaded: map[string]bool{"a": true},
			entriesBusy:   map[string]bool{},
			entriesErr:    map[string]string{},
			taskBusy:      map[string]bool{},
		},
	}
	m.setRoot(t.TempDir())
	return m
}

func TestAgentTaskDefaultsToEditorAndReusesItsTab(t *testing.T) {
	m := agentEditorModel(t, "")
	m.openAgentTask("a")
	if len(m.tabs) != 1 || m.tabs[0].kind != tabKindAgentSession ||
		m.tabs[0].agentTask != "a" || m.tab() != m.tabs[0] {
		t.Fatalf("editor task tabs = %+v", m.tabs)
	}
	if got := m.agentEditorTitle(m.tabs[0]); got != "Task A" {
		t.Fatalf("title = %q", got)
	}

	task := m.agents.tasks["a"]
	task.Title = "Renamed task"
	m.agents.tasks["a"] = task
	m.updateAgentTaskTab(task)
	m.openAgentTask("a")
	if len(m.tabs) != 1 || m.tabs[0].pageTitle != "Renamed task" ||
		m.agentEditorTitle(m.tabs[0]) != "Renamed task" {
		t.Fatalf("reused task tab = %+v", m.tabs)
	}
}

func TestAgentTaskToolWindowDoesNotCreateEditorTabs(t *testing.T) {
	m := agentEditorModel(t, "toolWindow")
	m.openAgentTask("a")
	if len(m.tabs) != 0 {
		t.Fatalf("tool-window task created editor tabs: %+v", m.tabs)
	}
	if !m.agents.visible || !m.agents.conversation || m.agents.selected != "a" {
		t.Fatalf("tool-window task state = %+v", m.agents)
	}
}

func TestAgentSessionLocationPreservesComposerDraft(t *testing.T) {
	m := agentEditorModel(t, "editor")
	m.agents.composerDrafts = map[string]agentComposerDraft{}
	m.agents.selected = "a"
	m.agents.composer = "keep this draft"
	m.agents.composerFiles = []string{filepath.Join(m.Root(), "App.java")}
	m.agents.composerCaret = 4
	m.agents.historyIndex = 2

	m.setAgentSessionLocation("toolWindow")
	if len(m.tabs) != 0 || !m.agents.newOpen && !m.agents.conversation {
		t.Fatalf("tool-window switch = tabs %+v agents %+v", m.tabs, m.agents)
	}
	draft, ok := m.agents.composerDrafts["a"]
	if !ok || draft.Text != "keep this draft" || draft.Caret != 4 ||
		!reflect.DeepEqual(draft.Files, m.agents.composerFiles) {
		t.Fatalf("stashed draft = %+v, %v", draft, ok)
	}

	m.setAgentSessionLocation("editor")
	if len(m.tabs) != 1 || m.tabs[0].agentTask != "a" {
		t.Fatalf("editor switch tabs = %+v", m.tabs)
	}
	if m.agents.composer != "keep this draft" || m.agents.composerCaret != 4 ||
		!reflect.DeepEqual(m.agents.composerFiles, draft.Files) {
		t.Fatalf("restored composer = %+v", m.agents)
	}
}

func TestAgentHomeOpensOnceAndCloses(t *testing.T) {
	m := agentEditorModel(t, "editor")
	m.openAgentHomeInEditor()
	m.openAgentHomeInEditor()
	if len(m.tabs) != 1 || m.tabs[0].kind != tabKindAgentHome ||
		m.agentHomeTab() != m.tabs[0] {
		t.Fatalf("home tabs = %+v", m.tabs)
	}
	m.removeAgentHomeTab()
	if len(m.tabs) != 0 || m.agentHomeTab() != nil {
		t.Fatalf("closed home tabs = %+v", m.tabs)
	}
}

func TestAgentTaskRemovalClosesTabAndRestoreDropsMissingTask(t *testing.T) {
	m := agentEditorModel(t, "editor")
	m.openAgentTask("a")
	delete(m.agents.tasks, "a")
	m.syncAgentTaskTabs()
	if len(m.tabs) != 0 {
		t.Fatalf("missing task kept tabs: %+v", m.tabs)
	}
}

func TestAgentSessionSnapshotKeepsPagesAndFocusedTask(t *testing.T) {
	m := agentEditorModel(t, "editor")
	m.openAgentTask("a")
	m.openAgentHomeInEditor()
	snapshot, ok := m.sessionSnapshot()
	if !ok {
		t.Fatal("agent-only workbench was not saved")
	}
	if !reflect.DeepEqual(snapshot.AgentTasks, []string{"a"}) ||
		!snapshot.AgentHome || snapshot.FocusAgentNew != true ||
		snapshot.FocusAgent != "" {
		t.Fatalf("agent session = %+v", snapshot)
	}

	m.activateAgentTab(m.agentTaskTab("a"))
	snapshot, ok = m.sessionSnapshot()
	if !ok || snapshot.FocusAgent != "a" || snapshot.FocusAgentNew {
		t.Fatalf("focused agent session = %+v, %v", snapshot, ok)
	}
}

func TestSaveActiveIgnoresAgentVirtualDocuments(t *testing.T) {
	m := agentEditorModel(t, "editor")
	m.openAgentTask("a")
	m.SaveActive()
	if m.tabs[0].saving || m.status == "Saving agent:a…" {
		t.Fatalf("agent tab entered the file save path: saving=%v status=%q",
			m.tabs[0].saving, m.status)
	}
	if err := saveAll(t.Context(), m.tabs, m.prefs.SaveFiles); err != nil {
		t.Fatalf("saveAll on an agent tab = %v", err)
	}
}

func TestAgentSampleConversationsOpenLongTabsWithoutBackend(t *testing.T) {
	m := agentEditorModel(t, "editor")
	m.OpenAgentSampleConversations(2, 3)
	if len(m.tabs) != 2 {
		t.Fatalf("sample tabs = %+v", m.tabs)
	}
	for _, id := range []string{"sample-1", "sample-2"} {
		if _, ok := m.agents.tasks[id]; !ok {
			t.Fatalf("sample task %s is missing", id)
		}
		if got := len(m.agents.entries[id]); got != 3*13 {
			t.Fatalf("sample task %s has %d entries", id, got)
		}
		if !m.agents.entriesLoaded[id] {
			t.Fatalf("sample task %s is not marked loaded", id)
		}
	}
	if _, visible := Command("agents.openSampleConversations"); !visible {
		t.Fatal("the hidden sample action is not callable by id")
	}
	for _, command := range commands() {
		if command.id == "agents.openSampleConversations" {
			t.Fatal("the hidden sample action appeared in the command palette")
		}
	}
}
