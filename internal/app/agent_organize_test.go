package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAgentTaskMenuSelectsAnUnselectedTask(t *testing.T) {
	m := &Model{agents: AgentsPane{
		tasks: map[string]agentTask{
			"a": agentTestTask("a", "Task A", "ready", 1),
			"b": agentTestTask("b", "Task B", "ready", 2),
		},
		selectedTasks:   []string{"a"},
		selectionAnchor: "a",
	}}

	m.prepareAgentTaskMenu("b")
	if !reflect.DeepEqual(m.agents.selectedTasks, []string{"b"}) ||
		m.agents.selectionAnchor != "b" {
		t.Fatalf("selection = %v, anchor = %q", m.agents.selectedTasks, m.agents.selectionAnchor)
	}

	m.agents.selectedTasks = []string{"a", "b"}
	m.prepareAgentTaskMenu("b")
	if !reflect.DeepEqual(m.agents.selectedTasks, []string{"a", "b"}) {
		t.Fatalf("existing multi-selection changed: %v", m.agents.selectedTasks)
	}
}

func TestAgentRemoveTaskGroupRequiresConfirmation(t *testing.T) {
	m := &Model{agents: AgentsPane{}}
	m.setRoot("/repo")
	m.prefs.AgentWorkspace.Organize = map[string]AgentOrganize{
		layoutKey("/repo"): {
			Groups: []AgentOrganizeGroup{{ID: "group", Name: "Review"}},
			GroupOf: map[string]string{
				"a": "group",
			},
		},
	}

	m.openRemoveAgentTaskGroup("group")
	if !m.agents.removeGroupOpen || m.agents.removeGroupID != "group" {
		t.Fatalf("confirmation state = %+v", m.agents)
	}
	if got := m.agentOrganize().Groups; len(got) != 1 || got[0].Name != "Review" {
		t.Fatalf("group changed before confirmation: %+v", got)
	}

	m.agents.removeGroupOpen = false
	m.agents.removeGroupID = ""
	if len(m.agentOrganize().Groups) != 1 {
		t.Fatal("cancelling removed the group")
	}

	m.openRemoveAgentTaskGroup("group")
	m.confirmRemoveAgentTaskGroup()
	organize := m.agentOrganize()
	if len(organize.Groups) != 0 || len(organize.GroupOf) != 0 ||
		m.agents.removeGroupOpen || m.agents.removeGroupID != "" {
		t.Fatalf("confirmed result = %+v, state = %+v", organize, m.agents)
	}
}

func TestAgentArchiveManyIsSequentialAndAggregatesErrors(t *testing.T) {
	tests := []struct {
		name         string
		archived     bool
		fail         map[string]bool
		wantCommands []string
		wantErrParts []string
		wantEmptyErr bool
	}{
		{
			name:     "all succeed",
			archived: true,
			wantCommands: []string{
				"agent_archive:a",
				"agent_archive:b",
				"agent_archive:c",
			},
			wantEmptyErr: true,
		},
		{
			name:     "partial failure",
			archived: true,
			fail: map[string]bool{
				"b": true,
			},
			wantCommands: []string{
				"agent_archive:a",
				"agent_archive:b",
				"agent_archive:c",
			},
			wantErrParts: []string{"Task B: backend refused"},
		},
		{
			name:     "all fail unarchiving",
			archived: false,
			fail: map[string]bool{
				"a": true,
				"b": true,
				"c": true,
			},
			wantCommands: []string{
				"agent_unarchive:a",
				"agent_unarchive:b",
			},
			wantErrParts: []string{"Task A: backend refused", "Task B: backend refused"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := make(chan recordedAgentCall, 4)
			postDone := make(chan struct{}, 1)
			m := &Model{
				ctx: context.Background(),
				postHook: func(fn func()) {
					fn()
					postDone <- struct{}{}
				},
				invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
					args := input.(map[string]any)
					taskID := args["task"].(string)
					calls <- recordedAgentCall{command: command, args: args}
					if test.fail[taskID] {
						return nil, errors.New("backend refused")
					}
					return json.RawMessage(`null`), nil
				},
			}
			m.setRoot("/repo")
			status := "ready"
			if !test.archived {
				status = "archived"
			}
			m.agents = AgentsPane{
				generation: 1,
				tasks: map[string]agentTask{
					"a": agentTestTask("a", "Task A", status, 1),
					"b": agentTestTask("b", "Task B", status, 2),
					"c": agentTestTask("c", "Task C", "running", 3),
				},
			}
			if test.archived {
				m.agents.tasks["c"] = agentTestTask("c", "Task C", "ready", 3)
			}

			m.setAgentArchivedMany([]string{"a", "b", "c", "a"}, test.archived)
			select {
			case <-postDone:
			case <-time.After(5 * time.Second):
				t.Fatal("archive batch did not finish")
			}

			var got []string
		drain:
			for {
				select {
				case call := <-calls:
					got = append(got, call.command+":"+call.args["task"].(string))
				default:
					break drain
				}
			}
			if !reflect.DeepEqual(got, test.wantCommands) {
				t.Fatalf("calls = %v, want %v", got, test.wantCommands)
			}
			if test.wantEmptyErr && m.agents.actionErr != "" {
				t.Fatalf("unexpected error: %q", m.agents.actionErr)
			}
			for _, part := range test.wantErrParts {
				if !stringsContains(m.agents.actionErr, part) {
					t.Fatalf("error %q lacks %q", m.agents.actionErr, part)
				}
			}
		})
	}
}

func TestAgentPruneOrganizeWaitsForLoadAndDropsStaleTasks(t *testing.T) {
	key := layoutKey("/repo")
	original := AgentOrganize{
		Groups:  []AgentOrganizeGroup{{ID: "group", Name: "Review"}},
		GroupOf: map[string]string{"gone": "group"},
		Pinned:  []string{"gone"},
	}
	m := &Model{agents: AgentsPane{loaded: false}}
	m.setRoot("/repo")
	m.prefs.AgentWorkspace.Organize = map[string]AgentOrganize{key: original}

	m.pruneAgentOrganize()
	if got := m.prefs.AgentWorkspace.Organize[key]; !reflect.DeepEqual(got, original) {
		t.Fatalf("organize changed before the task list loaded: %+v", got)
	}

	m.agents.loaded = true
	m.agents.tasks = map[string]agentTask{}
	m.pruneAgentOrganize()
	got := m.prefs.AgentWorkspace.Organize[key]
	if !reflect.DeepEqual(got.Groups, original.Groups) ||
		len(got.GroupOf) != 0 || len(got.Pinned) != 0 {
		t.Fatalf("empty task list result = %+v", got)
	}
}

func stringsContains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
