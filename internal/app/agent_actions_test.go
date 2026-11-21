package app

import (
	"reflect"
	"testing"
)

func TestAgentCompatibilityActionsAreRegistered(t *testing.T) {
	ids := []string{
		"agents.retryConversation",
		"agents.useToolWindow",
		"agents.useEditor",
		"agents.openTaskInEditor",
		"agents.openTask",
		"agents.dockTask",
		"agents.undockTask",
		"agents.renameTask",
		"agents.forkTask",
		"agents.archiveTasks",
		"agents.unarchiveTasks",
		"agents.deleteTasks",
		"agents.pinTasks",
		"agents.unpinTasks",
		"agents.newTaskGroup",
		"agents.renameTaskGroup",
		"agents.removeTaskGroup",
		"agents.copyReplyMarkdown",
		"agents.jumpToLatest",
		"agents.retractMessage",
		"agents.quoteReply",
		"agents.regenerateReply",
		"agents.forkReply",
	}
	for _, id := range ids {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA agent action %q", id)
		}
	}
}

func TestAgentActionTargetsTheVisibleConversation(t *testing.T) {
	m := &Model{}
	m.agents.tasks = map[string]agentTask{
		"recent": {ID: "recent", Title: "Recent", UpdatedMS: 2},
		"open":   {ID: "open", Title: "Open", UpdatedMS: 1},
	}
	m.agents.selected = "open"
	m.agents.selectedTasks = []string{"open"}
	m.agents.entries = map[string][]agentEntry{
		agentEntriesKey("open", ""): {
			{Type: "user", Text: "question"},
			{Type: "agent", Text: "answer"},
		},
	}
	if got := m.agentActionTaskID(); got != "open" {
		t.Fatalf("target task = %q", got)
	}
	if got := m.agentActionTaskIDs(); !reflect.DeepEqual(got, []string{"open"}) {
		t.Fatalf("target tasks = %v", got)
	}
	taskID, reply, ok := m.agentActionLatestReply()
	if !ok || taskID != "open" || reply.Text != "answer" {
		t.Fatalf("latest reply = %q %+v %v", taskID, reply, ok)
	}
}

func TestAgentActionsAcceptOpenIDEAArguments(t *testing.T) {
	m := &Model{}
	m.agents.tasks = map[string]agentTask{
		"one": {ID: "one", Title: "One", UpdatedMS: 2},
		"two": {ID: "two", Title: "Two", UpdatedMS: 1},
	}
	m.agents.selected = "one"
	m.agents.selectedTasks = []string{"one"}
	m.agents.entries = map[string][]agentEntry{
		agentEntriesKey("two", ""): {
			{Type: "user", Text: "first"},
			{Type: "agent", Text: "first reply"},
			{Type: "user", Text: "second"},
			{Type: "agent", Text: "second reply"},
		},
	}

	if got := m.agentActionTaskIDForArg("two"); got != "two" {
		t.Fatalf("task argument = %q", got)
	}
	if got := m.agentActionTaskIDForArg("missing"); got != "" {
		t.Fatalf("an invalid task argument selected %q", got)
	}
	if got := m.agentActionTaskIDsForArg([]any{"two", "missing", "one"}); !reflect.DeepEqual(got, []string{"two", "one"}) {
		t.Fatalf("task arguments = %v", got)
	}

	taskID, entries, target, ok := m.agentReplyTargetForArg(map[string]any{
		"task": "two",
		"turn": float64(0),
	})
	if !ok || taskID != "two" || len(entries) != 4 ||
		target.turn.Number != 0 || target.reply.Text != "first reply" {
		t.Fatalf("reply target = %q %d %+v %v", taskID, len(entries), target, ok)
	}
	if got := agentReplyTextArg(map[string]any{"text": "quoted"}); got != "quoted" {
		t.Fatalf("reply text = %q", got)
	}
}
