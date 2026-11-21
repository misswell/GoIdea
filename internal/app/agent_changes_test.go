package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type recordedAgentCall struct {
	command string
	args    map[string]any
}

func waitRecordedAgentCall(t *testing.T, calls <-chan recordedAgentCall, want string) recordedAgentCall {
	t.Helper()
	select {
	case call := <-calls:
		if call.command != want {
			t.Fatalf("command = %q, want %q", call.command, want)
		}
		return call
	case <-time.After(5 * time.Second):
		t.Fatalf("%s was not called", want)
		return recordedAgentCall{}
	}
}

func waitAgentState(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s did not settle", what)
}

func TestAgentChangesUseFlatAndGroupedCommands(t *testing.T) {
	calls := make(chan recordedAgentCall, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- recordedAgentCall{command: command, args: args}
			switch command {
			case "agent_changes":
				return json.RawMessage(`[{"path":"/repo/local.go","relPath":"local.go","kind":"modified","added":2,"removed":1}]`), nil
			case "agent_grouped_changes":
				return json.RawMessage(`{
					"uncommitted":[{"path":"/repo/worktree.go","relPath":"worktree.go","kind":"modified","added":3,"removed":2}],
					"commits":[{"id":"abcdef123456","shortId":"abcdef1","summary":"Fix it","message":"Fix it",
						"files":[{"path":"/repo/worktree.go","relPath":"worktree.go","kind":"modified","added":3,"removed":2}]}]
				}`), nil
			default:
				t.Fatalf("unexpected command %q", command)
				return nil, nil
			}
		},
		agents: AgentsPane{
			generation: 2,
			tasks: map[string]agentTask{
				"local":    {ID: "local", Isolation: "local", Cwd: "/repo"},
				"worktree": {ID: "worktree", Isolation: "worktree", Cwd: "/repo/worktrees/task"},
			},
		},
	}
	m.setRoot("/repo")
	grouped := false
	m.prefs.AgentWorkspace.ChangesGroupByCommit = &grouped

	m.refreshAgentChanges("local", true)
	call := waitRecordedAgentCall(t, calls, "agent_changes")
	if call.args["task"] != "local" {
		t.Fatalf("flat arguments = %#v", call.args)
	}
	waitAgentState(t, "flat changes", func() bool {
		return m.agents.changesLoaded["local"] && !m.agents.changesLoading["local"]
	})
	if len(m.agents.changesByTask["local"]) != 1 ||
		m.agents.changesByTask["local"][0].RelPath != "local.go" {
		t.Fatalf("flat changes = %+v", m.agents.changesByTask["local"])
	}

	grouped = true
	m.prefs.AgentWorkspace.ChangesGroupByCommit = &grouped
	m.refreshAgentChanges("worktree", true)
	call = waitRecordedAgentCall(t, calls, "agent_grouped_changes")
	if call.args["task"] != "worktree" {
		t.Fatalf("grouped arguments = %#v", call.args)
	}
	waitAgentState(t, "grouped changes", func() bool {
		return m.agents.changesLoaded["worktree"] && !m.agents.changesLoading["worktree"]
	})
	review := m.agents.groupedChanges["worktree"]
	if len(review.Uncommitted) != 1 || len(review.Commits) != 1 ||
		review.Commits[0].ID != "abcdef123456" {
		t.Fatalf("grouped changes = %+v", review)
	}

	grouped = false
	m.prefs.AgentWorkspace.ChangesGroupByCommit = &grouped
	m.refreshAgentChanges("worktree", true)
	waitRecordedAgentCall(t, calls, "agent_changes")
	waitAgentState(t, "flat worktree changes", func() bool {
		return len(m.agents.changesByTask["worktree"]) == 1 &&
			len(m.agents.groupedChanges["worktree"].Commits) == 0
	})
}

func TestAgentChangesPreferencePersists(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: settingsPath}}
	m.setRoot("/repo")
	grouped := false

	m.setAgentChangesGrouped(grouped)

	if p := m.prefs.AgentWorkspace.ChangesGroupByCommit; p == nil || *p {
		t.Fatalf("in-memory grouped preference = %v", p)
	}
	if stored := LoadPreferences(settingsPath); stored.AgentWorkspace.changesGroupByCommit() {
		t.Fatal("group-by-commit preference was not stored")
	}
}

func TestAgentToolDiffCompletionRefreshesChangesOnce(t *testing.T) {
	calls := make(chan recordedAgentCall, 3)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- recordedAgentCall{command: command, args: args}
			if command != "agent_changes" {
				t.Fatalf("unexpected command %q", command)
			}
			return json.RawMessage(`[]`), nil
		},
		agents: AgentsPane{
			channel:       5,
			generation:    1,
			tasks:         map[string]agentTask{"a": {ID: "a", Isolation: "local", Cwd: "/repo"}},
			entries:       map[string][]agentEntry{"a\x00": {{Type: "agent", Text: "working"}}},
			entriesLoaded: map[string]bool{"a\x00": true},
		},
	}
	m.setRoot("/repo")
	event := json.RawMessage(`{"type":"entry","task":"a","subagent":null,"index":0,
		"entry":{"type":"tool","status":"completed","diffs":[{"path":"/repo/a.go","added":1,"removed":0}]}}`)
	m.acceptAgentChunk(5, event)
	m.acceptAgentChunk(5, event)
	call := waitRecordedAgentCall(t, calls, "agent_changes")
	if call.args["task"] != "a" {
		t.Fatalf("changes arguments = %#v", call.args)
	}
	waitAgentState(t, "scheduled changes", func() bool {
		return m.agents.changesLoaded["a"]
	})
	select {
	case extra := <-calls:
		t.Fatalf("duplicate refresh = %#v", extra)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAgentDiffTextsUseTaskRepositoryAndMissingSides(t *testing.T) {
	newFixture := func(t *testing.T) (*Model, agentTask, string, *[]recordedAgentCall) {
		t.Helper()
		cwd := t.TempDir()
		path := filepath.Join(cwd, "new.go")
		if err := os.WriteFile(path, []byte("working\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		calls := make([]recordedAgentCall, 0)
		m := &Model{ctx: context.Background()}
		m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls = append(calls, recordedAgentCall{command: command, args: args})
			switch command {
			case "vcs_open_external_log":
				return json.RawMessage(`null`), nil
			case "vcs_file_at":
				value, _ := json.Marshal(fmt.Sprintf("%s@%s", args["rev"], args["path"]))
				return value, nil
			case "agent_base_text":
				return json.RawMessage(`"base\n"`), nil
			default:
				return nil, fmt.Errorf("unexpected command %q", command)
			}
		}
		task := agentTask{
			ID: "task", Isolation: "worktree", Cwd: cwd,
		}
		return m, task, path, &calls
	}

	t.Run("flat uses base text", func(t *testing.T) {
		m, task, path, calls := newFixture(t)
		task.Isolation = "local"
		change := agentChange{Path: path, RelPath: "new.go", Kind: "modified"}
		left, right, err := m.agentDiffTexts(task, agentDiffTarget{Change: change},
			"new.go", "new.go", path, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if left != "base\n" || right != "working\n" {
			t.Fatalf("flat texts = %q, %q", left, right)
		}
		if len(*calls) != 1 || (*calls)[0].command != "agent_base_text" ||
			(*calls)[0].args["path"] != path {
			t.Fatalf("flat calls = %+v", *calls)
		}
	})

	t.Run("grouped registers worktree once and reads absolute path", func(t *testing.T) {
		m, task, path, calls := newFixture(t)
		change := agentChange{Path: path, RelPath: "new.go", Kind: "modified"}
		left, right, err := m.agentDiffTexts(task, agentDiffTarget{Change: change, Grouped: true},
			"new.go", "new.go", path, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(left, "HEAD@") || right != "working\n" {
			t.Fatalf("grouped texts = %q, %q", left, right)
		}
		want := []string{"vcs_open_external_log", "vcs_file_at"}
		for index, command := range want {
			if (*calls)[index].command != command {
				t.Fatalf("grouped calls = %+v", *calls)
			}
		}
		if (*calls)[0].args["path"] != task.Cwd || (*calls)[1].args["path"] != path ||
			(*calls)[1].args["rev"] != "HEAD" {
			t.Fatalf("grouped arguments = %+v", *calls)
		}
	})

	t.Run("commit added has no parent side", func(t *testing.T) {
		m, task, path, calls := newFixture(t)
		parent := "parent"
		commit := &agentCommit{ID: "commit", Parent: &parent}
		change := agentChange{Path: path, RelPath: "new.go", Kind: "added"}
		left, right, err := m.agentDiffTexts(task,
			agentDiffTarget{Change: change, Commit: commit},
			"new.go", "new.go", path, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if left != "" || !strings.Contains(right, "commit@") {
			t.Fatalf("added commit texts = %q, %q", left, right)
		}
		if len(*calls) != 2 || (*calls)[1].args["rev"] != "commit" ||
			(*calls)[1].args["path"] != path {
			t.Fatalf("added commit calls = %+v", *calls)
		}
	})

	t.Run("commit deleted has no commit side", func(t *testing.T) {
		m, task, path, calls := newFixture(t)
		parent := "parent"
		commit := &agentCommit{ID: "commit", Parent: &parent}
		change := agentChange{Path: path, RelPath: "new.go", Kind: "deleted"}
		left, right, err := m.agentDiffTexts(task,
			agentDiffTarget{Change: change, Commit: commit},
			"new.go", "new.go", path, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(left, "parent@") || right != "" {
			t.Fatalf("deleted commit texts = %q, %q", left, right)
		}
		if len(*calls) != 2 || (*calls)[1].args["rev"] != "parent" {
			t.Fatalf("deleted commit calls = %+v", *calls)
		}
	})

	t.Run("rename reads old path on the left", func(t *testing.T) {
		m, task, path, calls := newFixture(t)
		parent := "parent"
		commit := &agentCommit{ID: "commit", Parent: &parent}
		oldPath := "old.go"
		change := agentChange{
			Path: path, RelPath: "new.go", Kind: "renamed", OldPath: &oldPath,
		}
		left, right, err := m.agentDiffTexts(task,
			agentDiffTarget{Change: change, Commit: commit},
			"new.go", oldPath, path, "", false)
		if err != nil {
			t.Fatal(err)
		}
		oldAbsolute := filepath.Join(task.Cwd, oldPath)
		if !strings.Contains(left, "parent@"+oldAbsolute) ||
			!strings.Contains(right, "commit@"+path) {
			t.Fatalf("renamed texts = %q, %q", left, right)
		}
		if len(*calls) != 3 || (*calls)[1].args["path"] != oldAbsolute ||
			(*calls)[2].args["path"] != path {
			t.Fatalf("renamed calls = %+v", *calls)
		}
	})
}

func TestAgentChangeDiffBuildsAFileChain(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.go")
	secondPath := filepath.Join(root, "second.go")
	if err := os.WriteFile(firstPath, []byte("first working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second working\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "agent_base_text" {
				return nil, fmt.Errorf("unexpected command %q", command)
			}
			return json.RawMessage(`"base\n"`), nil
		},
		agents: AgentsPane{
			generation: 1,
			tasks: map[string]agentTask{
				"task": {ID: "task", Isolation: "local", Cwd: root, Branch: stringPointer("agent/feature")},
			},
			changesByTask: map[string][]agentChange{
				"task": {
					{Path: firstPath, RelPath: "first.go", Kind: "modified"},
					{Path: secondPath, RelPath: "second.go", Kind: "modified"},
				},
			},
		},
	}
	m.setRoot(root)
	grouped := false
	m.prefs.AgentWorkspace.ChangesGroupByCommit = &grouped

	m.showAgentChangeDiff("task", m.agents.changesByTask["task"][0], nil)
	waitAgentState(t, "first diff", func() bool {
		return m.diff != nil && !m.diff.loading
	})
	if m.diff.agentIndex != 0 || len(m.diff.agentChain) != 2 ||
		m.diff.leftLabel != "Base" || m.diff.rightLabel != "feature" {
		t.Fatalf("first chain = %+v", m.diff)
	}
	if got := []string{m.diff.agentChain[0].Change.RelPath, m.diff.agentChain[1].Change.RelPath}; !reflect.DeepEqual(got, []string{"first.go", "second.go"}) {
		t.Fatalf("chain = %v", got)
	}

	d := m.diff
	m.loadAgentDiff("task", d.agentChain, 1)
	waitAgentState(t, "second diff", func() bool {
		return m.diff != d && m.diff != nil && !m.diff.loading
	})
	if m.diff.agentIndex != 1 || m.diff.path != secondPath ||
		len(m.diff.agentChain) != 2 {
		t.Fatalf("second chain = %+v", m.diff)
	}
}

func TestAgentChangesDialogRendersCommitsAndFiles(t *testing.T) {
	m := &Model{agents: AgentsPane{
		changesOpen: true,
		changesTask: "task",
		tasks: map[string]agentTask{
			"task": {ID: "task", Title: "Align changes", Isolation: "worktree"},
		},
		groupedChanges: map[string]agentChanges{
			"task": {
				Uncommitted: []agentChange{{
					Path: "/repo/worktree/a.go", RelPath: "a.go", Kind: "modified", Added: 2, Removed: 1,
				}},
				Commits: []agentCommit{{
					ID: "abcdef123456", ShortID: "abcdef1", Summary: "Fix it", Message: "Fix it",
					Files: []agentChange{{
						Path: "/repo/worktree/b.go", RelPath: "b.go", Kind: "added", Added: 4,
					}},
				}},
			},
		},
		changesLoaded: map[string]bool{"task": true},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentsChangesDialog(c, c.Theme())
	}, 900, 700)
	tester.Frame()
	for _, want := range []string{
		"Changes · Align changes", "Uncommitted", "Committed", "abcdef1 Fix it", "Flat List",
	} {
		if !tester.HasText(want) {
			t.Fatalf("Changes dialog lacks %q: %q", want, tester.Texts())
		}
	}
}

func TestAgentChangeOutcomesRefreshTheMatchingView(t *testing.T) {
	tests := []struct {
		name       string
		isolation  string
		grouped    bool
		command    string
		wantView   string
		wantReload string
	}{
		{name: "accept grouped", isolation: "worktree", grouped: true, command: "agent_accept_changes", wantReload: "agent_grouped_changes"},
		{name: "rollback grouped", isolation: "worktree", grouped: true, command: "agent_rollback_changes", wantView: "group", wantReload: "agent_grouped_changes"},
		{name: "rollback flat", isolation: "local", grouped: false, command: "agent_rollback_changes", wantView: "sinceStart", wantReload: "agent_changes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := make(chan recordedAgentCall, 3)
			m := &Model{
				ctx:      context.Background(),
				postHook: func(fn func()) { fn() },
				invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
					args, _ := input.(map[string]any)
					calls <- recordedAgentCall{command: command, args: args}
					switch command {
					case "agent_grouped_changes":
						return json.RawMessage(`{"uncommitted":[],"commits":[]}`), nil
					case "agent_changes":
						return json.RawMessage(`[]`), nil
					default:
						return json.RawMessage(`null`), nil
					}
				},
				agents: AgentsPane{
					generation: 1,
					tasks: map[string]agentTask{
						"task": {ID: "task", Isolation: test.isolation, Cwd: "/repo/worktree"},
					},
					changesOpen: true,
					changesTask: "task",
					changesByTask: map[string][]agentChange{
						"task": {{Path: "/repo/worktree/a.go", RelPath: "a.go", Kind: "modified"}},
					},
					groupedChanges: map[string]agentChanges{
						"task": {Uncommitted: []agentChange{{
							Path: "/repo/worktree/a.go", RelPath: "a.go", Kind: "modified",
						}}},
					},
					changesLoaded: map[string]bool{"task": true},
				},
			}
			m.setRoot("/repo")
			m.prefs.AgentWorkspace.ChangesGroupByCommit = &test.grouped

			if test.command == "agent_accept_changes" {
				m.acceptAgentChanges()
			} else {
				m.rollbackAgentChanges()
			}
			action := waitRecordedAgentCall(t, calls, test.command)
			if action.args["task"] != "task" || action.args["paths"] != nil {
				t.Fatalf("action arguments = %#v", action.args)
			}
			if got := action.args["view"]; test.wantView == "" && got != nil ||
				test.wantView != "" && got != test.wantView {
				t.Fatalf("rollback view = %#v, want %q", got, test.wantView)
			}
			waitRecordedAgentCall(t, calls, test.wantReload)
			waitAgentState(t, "outcome refresh", func() bool {
				return !m.agents.changesBusy && !m.agents.changesLoading["task"]
			})
		})
	}
}
