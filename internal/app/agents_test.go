package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func agentTestTask(id, title, status string, updated float64) agentTask {
	return agentTask{ID: id, Title: title, Status: status, UpdatedMS: updated}
}

func TestAgentTaskGroupsOrderFilterAndAttention(t *testing.T) {
	m := &Model{agents: AgentsPane{
		tasks: map[string]agentTask{
			"ready":     agentTestTask("ready", "Ready task", "ready", 4),
			"running":   agentTestTask("running", "Running task", "running", 2),
			"starting":  agentTestTask("starting", "Starting task", "starting", 8),
			"input":     agentTestTask("input", "Input task", "needsInput", 1),
			"failed":    agentTestTask("failed", "Failed task", "failed", 3),
			"limited":   agentTestTask("limited", "Limited task", "limited", 7),
			"archived":  agentTestTask("archived", "Archived task", "archived", 6),
			"unmatched": agentTestTask("unmatched", "Other task", "ready", 5),
		},
		filter: "task",
	}}
	groups := m.taskGroups()
	var labels []string
	for _, group := range groups {
		labels = append(labels, group.Label)
	}
	want := []string{"Needs Input", "Failed", "Usage Limit", "Running", "Ready to Review", "Archived"}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("group order = %v, want %v", labels, want)
	}
	if len(groups[3].Tasks) != 2 || groups[3].Tasks[0].ID != "starting" ||
		groups[3].Tasks[1].ID != "running" {
		t.Fatalf("running tasks = %+v", groups[3].Tasks)
	}
	if len(groups[4].Tasks) != 2 || groups[4].Tasks[0].ID != "unmatched" ||
		groups[4].Tasks[1].ID != "ready" {
		t.Fatalf("ready tasks = %+v", groups[4].Tasks)
	}

	m.agents.filter = "input"
	groups = m.taskGroups()
	if len(groups) != 1 || groups[0].Tasks[0].ID != "input" {
		t.Fatalf("filtered groups = %+v", groups)
	}
	if !agentNeedsAttention("ready") || !agentNeedsAttention("needsInput") ||
		agentNeedsAttention("archived") || agentNeedsAttention("running") {
		t.Fatal("attention statuses do not match OpenIDEA")
	}
}

func TestAgentEventsRouteTaskEntryAppendAndRemove(t *testing.T) {
	m := &Model{agents: AgentsPane{
		channel: 7,
		tasks: map[string]agentTask{
			"a": agentTestTask("a", "Task A", "running", 1),
		},
		unread:        map[string]bool{},
		selected:      "a",
		entries:       map[string][]agentEntry{"a": {{Type: "agent", Text: "hello"}}},
		entriesLoaded: map[string]bool{"a": true},
	}}
	m.acceptAgentChunk(7, json.RawMessage(`{"type":"task","task":{"id":"a","title":"Task A","status":"needsInput","updatedMs":2}}`))
	if got := m.agents.tasks["a"].Status; got != "needsInput" {
		t.Fatalf("task status = %q", got)
	}
	if m.agents.unread["a"] {
		t.Fatal("the selected task became unread")
	}
	m.acceptAgentChunk(7, json.RawMessage(`{"type":"append","task":"a","subagent":null,"index":0,"from":5,"text":" world"}`))
	if got := m.agents.entries["a"][0].Text; got != "hello world" {
		t.Fatalf("streamed text = %q", got)
	}
	m.acceptAgentChunk(7, json.RawMessage(`{"type":"entry","task":"a","subagent":null,"index":0,"entry":{"type":"agent","text":"replacement"}}`))
	if got := m.agents.entries["a"][0].Text; got != "replacement" {
		t.Fatalf("replaced entry = %q", got)
	}
	m.acceptAgentChunk(7, json.RawMessage(`{"type":"removed","id":"a"}`))
	if len(m.agents.tasks) != 0 || len(m.agents.entries) != 0 || m.agents.selected != "" ||
		m.agents.conversation {
		t.Fatalf("removed task state = %+v", m.agents)
	}
}

func TestAgentOutOfOrderEntryReloadsConversation(t *testing.T) {
	requests := make(chan map[string]any, 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "agent_entries" {
				t.Fatalf("command = %q", command)
			}
			requests <- input.(map[string]any)
			return json.RawMessage(`[{"type":"agent","text":"reloaded"}]`), nil
		},
		agents: AgentsPane{
			channel:       9,
			selected:      "a",
			tasks:         map[string]agentTask{"a": agentTestTask("a", "Task", "running", 1)},
			entries:       map[string][]agentEntry{"a": {{Type: "agent", Text: "old"}}},
			entriesLoaded: map[string]bool{"a": true},
			entriesBusy:   map[string]bool{},
		},
	}
	m.acceptAgentChunk(9, json.RawMessage(`{"type":"entry","task":"a","subagent":null,"index":4,"entry":{"type":"agent","text":"gap"}}`))
	select {
	case args := <-requests:
		if args["task"] != "a" || args["subagent"] != nil {
			t.Fatalf("entries arguments = %#v", args)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("out-of-order entry did not reload the conversation")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.agents.entries["a"]) == 1 && m.agents.entries["a"][0].Text == "reloaded" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("reloaded entries = %+v", m.agents.entries["a"])
}

func TestAgentProjectGenerationDropsOldEvents(t *testing.T) {
	m := &Model{agents: AgentsPane{
		channel: 3,
		tasks:   map[string]agentTask{"old": agentTestTask("old", "Old", "running", 1)},
	}}
	m.resetAgents()
	m.acceptAgentChunk(3, json.RawMessage(`{"type":"task","task":{"id":"new","title":"New","status":"running"}}`))
	if len(m.agents.tasks) != 0 || m.agents.channel != 0 {
		t.Fatalf("old project event changed state: %+v", m.agents)
	}
}

func TestAgentCreatePromptAndPermissionRPCArguments(t *testing.T) {
	type call struct {
		command string
		args    map[string]any
	}
	calls := make(chan call, 6)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- call{command: command, args: args}
			switch command {
			case "agent_create":
				return json.RawMessage(`{"id":"created","title":"Created","provider":"codex","status":"ready"}`), nil
			case "agent_entries":
				return json.RawMessage(`[]`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot("/project")
	m.agents = AgentsPane{
		generation: 4,
		providers: []agentProvider{{
			ID: "codex", Name: "Codex", Installed: true,
			Config: []agentConfigOption{
				{
					ID: "model", Name: "Model", Category: stringPointer("model"),
					Current: stringPointer("standard"),
					Values:  []agentConfigValue{{Value: "standard", Name: "Standard"}},
				},
				{
					ID: "mode", Name: "Mode", Category: stringPointer("mode"),
					Current: stringPointer("plan"),
					Values: []agentConfigValue{
						{Value: "plan", Name: "Plan"},
						{Value: "full-access", Name: "Full access"},
					},
				},
			},
		}},
		newOpen:      true,
		newProvider:  "codex",
		newPrompt:    "Fix the test",
		newFiles:     []string{"/project/src/Fix.java"},
		newIsolation: "worktree",
		tasks:        map[string]agentTask{},
		taskBusy:     map[string]bool{},
		entries:      map[string][]agentEntry{},
	}
	m.prepareNewAgentConfig()
	waitCall := func(want string) call {
		deadline := time.After(5 * time.Second)
		for {
			select {
			case got := <-calls:
				if got.command == "agent_entries" ||
					got.command == "agent_changes" ||
					got.command == "agent_grouped_changes" {
					continue
				}
				if got.command != want {
					t.Fatalf("command = %q, want %q", got.command, want)
				}
				return got
			case <-deadline:
				t.Fatalf("%s was not called", want)
			}
		}
	}
	m.createAgentTask()
	got := waitCall("agent_create")
	for {
		select {
		case next := <-calls:
			if next.command != "agent_entries" {
				continue
			}
			goto entriesLoaded
		case <-time.After(5 * time.Second):
			t.Fatal("created conversation was not loaded")
		}
	}

entriesLoaded:
	request := got.args["request"].(map[string]any)
	prompt := request["prompt"].(map[string]any)
	if prompt["text"] != "Fix the test" || request["provider"] != "codex" ||
		request["isolation"] != "worktree" || request["base"] != nil {
		t.Fatalf("create request = %#v", request)
	}
	if got := prompt["files"].([]string); !reflect.DeepEqual(got, []string{"/project/src/Fix.java"}) {
		t.Fatalf("create files = %v", got)
	}
	config := request["config"].([]agentConfigChoice)
	if !reflect.DeepEqual(config, []agentConfigChoice{
		{ID: "model", Value: "standard"},
		{ID: "mode", Value: "full-access"},
	}) {
		t.Fatalf("create config = %+v", config)
	}

	m.agents.selected = "created"
	m.agents.tasks["created"] = agentTestTask("created", "Created", "ready", 1)
	m.agents.composer = "Continue"
	m.agents.composerFiles = []string{"/project/src/Continue.java"}
	m.submitAgentPrompt()
	got = waitCall("agent_prompt")
	if got.args["task"] != "created" || got.args["now"] != false {
		t.Fatalf("prompt call = %#v", got)
	}
	prompt = got.args["prompt"].(map[string]any)
	if prompt["text"] != "Continue" {
		t.Fatalf("prompt = %#v", prompt)
	}
	if got := prompt["files"].([]string); !reflect.DeepEqual(got, []string{"/project/src/Continue.java"}) {
		t.Fatalf("prompt files = %v", got)
	}

	m.respondAgentPermission("created", 12, "allow")
	got = waitCall("agent_respond")
	if got.args["task"] != "created" || got.args["permission"] != uint32(12) ||
		got.args["option"] != "allow" {
		t.Fatalf("permission call = %#v", got)
	}
}

func TestAgentsWindowRendersGroupsTimelineAndStatus(t *testing.T) {
	m := &Model{
		prefs: Preferences{AgentWorkspace: AgentWorkspacePrefs{
			SessionLocation: "toolWindow",
		}},
		agents: AgentsPane{
			visible: true,
			loaded:  true,
			tasks: map[string]agentTask{
				"a": agentTestTask("a", "Needs a decision", "needsInput", 200),
				"b": agentTestTask("b", "Running tests", "running", 100),
			},
			unread: map[string]bool{"b": true},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentsRightView(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 420, 640)
	tester.Frame()
	for _, text := range []string{"Agents", "Needs Input  1", "Needs a decision", "Running  1", "Running tests"} {
		if !tester.HasText(text) {
			t.Fatalf("Agents window lacks %q: %q", text, tester.Texts())
		}
	}
	input, running, unread, target := agentStatusSummary(&m.agents)
	if input != 1 || running != 1 || unread != 1 || target != "a" {
		t.Fatalf("status summary = %d, %d, %d, %q", input, running, unread, target)
	}

	m.agents.entries = map[string][]agentEntry{}
	m.agents.entriesLoaded = map[string]bool{}
	m.agents.entriesBusy = map[string]bool{}
	m.agents.entriesErr = map[string]string{}
	m.openAgentConversation("a")
	m.agents.entries["a"] = []agentEntry{{Type: "agent", Text: "Choose one of the options"}}
	m.agents.entriesLoaded["a"] = true
	tester = ui.NewTester(func(c *ui.Context) {
		m.agentsRightView(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 420, 640)
	tester.Frame()
	if !tester.HasText("Choose one of the options") ||
		!tester.HasText("Enter queues for the next turn · ⌘Enter sends now") {
		t.Fatalf("conversation did not render: %q", tester.Texts())
	}
}

func TestAgentTaskSummaryRendersConfigQueueAndBackground(t *testing.T) {
	model := "Balanced"
	mode := "Plan"
	m := &Model{agents: AgentsPane{
		taskBusy: map[string]bool{},
		tasks:    map[string]agentTask{},
	}}
	task := agentTestTask("a", "Task controls", "running", 1)
	task.Config = []agentConfigOption{
		{
			ID: "model", Name: "Model", Category: stringPointer("model"), Current: &model,
			Values: []agentConfigValue{{Value: "Balanced", Name: "Balanced"}},
		},
		{
			ID: "mode", Name: "Mode", Category: stringPointer("mode"), Current: &mode,
			Values: []agentConfigValue{{Value: "Plan", Name: "Plan"}},
		},
	}
	task.Queue = []agentQueued{{
		ID: "q1", Prompt: agentPrompt{Text: "Queued prompt"},
	}}
	task.Background = []agentBackground{{
		ID: "bg1", Kind: "local_bash", Description: "Background test",
	}}
	task.Capabilities.BackgroundTasks = true
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentTaskSummary(c, c.Theme(), oiPaletteOf(c.Theme()), task, nil)
	}, 760, 620)
	for _, text := range []string{
		"Balanced", "Plan", "Queued messages", "Queued prompt",
		"Send now", "Edit", "Remove", "Running in the background",
		"Background test", "Stop",
	} {
		if !tester.HasText(text) {
			t.Fatalf("task controls lack %q: %q", text, tester.Texts())
		}
	}
}

func TestAgentQueueBackgroundAndConfigRPC(t *testing.T) {
	type call struct {
		command string
		args    map[string]any
	}
	calls := make(chan call, 8)
	actionsDone := make(chan struct{}, 8)
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			actionsDone <- struct{}{}
		},
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- call{command: command, args: args}
			if command == "agent_unqueue" {
				return json.RawMessage(`{"id":"q2","prompt":{"text":"Queued prompt","files":["/project/src/B.java"],"images":[]}}`), nil
			}
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot("/project")
	task := agentTestTask("a", "Task actions", "running", 1)
	model := "Standard"
	task.Config = []agentConfigOption{{
		ID: "model", Name: "Model", Category: stringPointer("model"), Current: &model,
		Values: []agentConfigValue{
			{Value: "Standard", Name: "Standard"},
			{Value: "Fast", Name: "Fast"},
		},
	}}
	task.Queue = []agentQueued{
		{ID: "q1", Prompt: agentPrompt{Text: "First"}},
		{ID: "q2", Prompt: agentPrompt{Text: "Second"}},
	}
	task.Background = []agentBackground{{ID: "bg1", Kind: "local_bash", Description: "Build"}}
	m.agents = AgentsPane{
		generation: 1,
		selected:   "a",
		composer:   "Keep this draft",
		tasks:      map[string]agentTask{"a": task},
		taskBusy:   map[string]bool{},
	}
	waitCall := func(command string) call {
		t.Helper()
		select {
		case got := <-calls:
			if got.command != command {
				t.Fatalf("command = %q, want %q", got.command, command)
			}
			return got
		case <-time.After(5 * time.Second):
			t.Fatalf("%s was not called", command)
			return call{}
		}
	}
	waitIdle := func() {
		t.Helper()
		select {
		case <-actionsDone:
			return
		case <-time.After(5 * time.Second):
			t.Fatal("agent action did not finish")
		}
	}

	m.sendAgentQueuedNow("a", "q1")
	got := waitCall("agent_send_queued_now")
	waitIdle()
	if got.args["task"] != "a" || got.args["queued"] != "q1" {
		t.Fatalf("send-now arguments = %#v", got.args)
	}

	m.editAgentQueued("a", "q2")
	got = waitCall("agent_unqueue")
	waitIdle()
	if got.args["task"] != "a" || got.args["queued"] != "q2" {
		t.Fatalf("unqueue arguments = %#v", got.args)
	}
	if !strings.Contains(m.agents.composer, "Keep this draft") ||
		!strings.Contains(m.agents.composer, "Queued prompt") {
		t.Fatalf("editing the queue lost text: %q", m.agents.composer)
	}
	if !reflect.DeepEqual(m.agents.composerFiles, []string{"/project/src/B.java"}) {
		t.Fatalf("restored queue files = %v", m.agents.composerFiles)
	}
	if queue := m.agents.tasks["a"].Queue; len(queue) != 0 {
		t.Fatalf("removed queued message remains: %+v", queue)
	}

	m.stopAgentBackground("a", "bg1")
	got = waitCall("agent_stop_background")
	waitIdle()
	if got.args["task"] != "a" || got.args["background"] != "bg1" {
		t.Fatalf("stop-background arguments = %#v", got.args)
	}
	if background := m.agents.tasks["a"].Background; len(background) != 0 {
		t.Fatalf("stopped background task remains: %+v", background)
	}

	m.setAgentConfig("a", "model", "Fast")
	got = waitCall("agent_set_config")
	waitIdle()
	if got.args["task"] != "a" || got.args["option"] != "model" || got.args["value"] != "Fast" {
		t.Fatalf("set-config arguments = %#v", got.args)
	}
	updated := m.agents.tasks["a"]
	if updated.Model == nil || *updated.Model != "Fast" ||
		updated.Config[0].Current == nil || *updated.Config[0].Current != "Fast" {
		t.Fatalf("updated config = %+v", updated)
	}
}

func TestAgentsToolWindowRendersOnTheRight(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	m.agents = AgentsPane{
		loaded: true,
		tasks: map[string]agentTask{
			"a": agentTestTask("a", "Right-side task", "ready", 1),
		},
	}
	m.OpenAgentsWindow()
	tester := workbenchTester(t, m)
	if !tester.HasText("Agents") || !tester.HasText("Right-side task") {
		t.Fatalf("Agents is not in the workbench: %q", tester.Texts())
	}
	if rect, ok := tester.Find("Agents"); !ok || rect.X < 1000 {
		t.Fatalf("Agents is not docked on the right: %+v, %v", rect, ok)
	}
}

func TestAppendAgentDeltaUsesUTF16Offsets(t *testing.T) {
	if got := appendAgentDelta("A😀", 3, "B"); got != "A😀B" {
		t.Fatalf("append after astral rune = %q", got)
	}
	if got := appendAgentDelta("hello", 2, "llo world"); got != "hello world" {
		t.Fatalf("overlapping stream = %q", got)
	}
	if got := appendAgentDelta("short", 10, "ignored"); got != "short" {
		t.Fatalf("future offset = %q", got)
	}
	if !strings.HasPrefix(agentEntryLabel(agentEntry{Type: "message", Incoming: true}), "Request") {
		t.Fatal("incoming parent message label is wrong")
	}
}

func TestAgentFindCountsEveryVisiblePartOfConversation(t *testing.T) {
	entries := []agentEntry{
		{
			Type:   "agent",
			Title:  "Needle title",
			Text:   "First needle and another NEEDLE",
			Output: stringPointer("tool needle output"),
			Items:  []agentPlanItem{{Content: "check needle", Status: "completed"}},
		},
		{Type: "user", Text: "No match"},
	}
	find := agentFindExpression("needle")
	if got := agentFindMatchCount(find, entries); got != 5 {
		t.Fatalf("match count = %d, want 5", got)
	}
	p := ui.Theme{Selection: ui.RGB(30, 70, 90)}
	spans := agentFindSpans("needle and NEEDLE", find, &p)
	highlighted := 0
	for _, span := range spans {
		if span.Background == p.Selection {
			highlighted++
		}
	}
	if highlighted != 2 {
		t.Fatalf("highlighted spans = %d, want 2", highlighted)
	}
	if agentFindExpression(strings.Repeat(" ", 4)) != nil {
		t.Fatal("a whitespace-only query has matches")
	}
}

func TestAgentFindBarTypesStepsAndCloses(t *testing.T) {
	m := &Model{agents: AgentsPane{
		selected:      "a",
		conversation:  true,
		findOpen:      true,
		findText:      "needle",
		entries:       map[string][]agentEntry{"a": {{Type: "agent", Text: "needle first needle second"}}},
		entriesLoaded: map[string]bool{"a": true},
		entriesBusy:   map[string]bool{},
		entriesErr:    map[string]string{},
		taskBusy:      map[string]bool{},
		tasks: map[string]agentTask{
			"a": agentTestTask("a", "Find task", "ready", 1),
		},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentsConversation(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 620, 640)
	if !tester.HasText("1 of 2") {
		t.Fatalf("initial find status is missing: %q", tester.Texts())
	}
	if err := tester.Click("Next"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("2 of 2") {
		t.Fatalf("next did not move to the second match: %q", tester.Texts())
	}
	if err := tester.Click("Done"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if m.agents.findOpen || m.agents.findText != "" || m.agents.findMatch != 0 {
		t.Fatalf("closed find state = open %v, text %q, match %d",
			m.agents.findOpen, m.agents.findText, m.agents.findMatch)
	}
}

func TestAgentNewTaskFilesStayReferencesAndCanBeRemoved(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "src", "App.java")
	outside := filepath.Join(filepath.Dir(root), "Outside.java")
	m := &Model{agents: AgentsPane{newPrompt: "Keep this prompt"}}
	m.setRoot(root)
	m.SendPathsToAgent([]string{inside, inside, outside})
	if !m.agentNewTaskVisible() {
		t.Fatal("Send to Agent did not open New Agent Task")
	}
	if m.agents.newPrompt != "Keep this prompt @src/App.java " {
		t.Fatalf("Send to Agent prompt = %q", m.agents.newPrompt)
	}
	if len(m.agents.newFiles) != 0 {
		t.Fatalf("inside files became attachments: %v", m.agents.newFiles)
	}
	if got := m.AgentTaskFilePath(inside); got != filepath.Join("src", "App.java") {
		t.Fatalf("compact path = %q", got)
	}
}

func TestAgentNewTaskDialogShowsAndRemovesFiles(t *testing.T) {
	root := t.TempDir()
	m := &Model{agents: AgentsPane{
		newOpen:      true,
		newProvider:  "codex",
		newIsolation: "worktree",
		newFiles:     []string{filepath.Join(root, "src", "App.java")},
		providers: []agentProvider{{
			ID: "codex", Name: "Codex", Installed: true,
			Config: []agentConfigOption{{
				ID: "model", Name: "Model", Category: stringPointer("model"),
				Current: stringPointer("standard"),
				Values: []agentConfigValue{
					{Value: "standard", Name: "Standard"},
					{Value: "fast", Name: "Fast"},
				},
			}},
		}},
	}}
	m.setRoot(root)
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentsNewTaskDialog(c, c.Theme())
	}, 760, 760)
	tester.Frame()
	for _, text := range []string{
		"Add Files…", "Add Folder…", "src/App.java", "Remove",
		"Model", "Standard",
	} {
		if !tester.HasText(text) {
			t.Fatalf("New Agent Task lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestAgentExternalSessionsClosedDuringLoadDoesNotStayBusy(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	posts := make(chan func(), 2)
	var calls int
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			posts <- fn
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "agent_external_sessions" {
				t.Fatalf("command = %q", command)
			}
			calls++
			if calls == 1 {
				firstStarted <- struct{}{}
				<-releaseFirst
			}
			return json.RawMessage(`[{"provider":"codex","sessionId":"s1","title":"External work"}]`), nil
		},
		agents: AgentsPane{
			generation:   1,
			externalOpen: true,
			external:     []agentExternalSession{},
			tasks:        map[string]agentTask{},
			entries:      map[string][]agentEntry{},
			entriesBusy:  map[string]bool{},
			entriesErr:   map[string]string{},
			taskBusy:     map[string]bool{},
		},
	}
	m.setRoot("/project")
	m.loadAgentExternalSessions()
	select {
	case <-firstStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("external session load did not start")
	}
	m.agents.externalOpen = false
	close(releaseFirst)
	var applyFirst func()
	select {
	case applyFirst = <-posts:
	case <-time.After(5 * time.Second):
		t.Fatal("closed external session result was not posted")
	}
	applyFirst()
	if m.agents.externalLoading {
		t.Fatal("closing the dialog left external session loading stuck")
	}

	m.agents.externalOpen = true
	m.loadAgentExternalSessions()
	var applySecond func()
	select {
	case applySecond = <-posts:
	case <-time.After(5 * time.Second):
		t.Fatal("the load retry was blocked after closing the dialog")
	}
	applySecond()
	if m.agents.externalLoading {
		t.Fatal("completed external session retry is still loading")
	}
	if len(m.agents.external) != 1 || m.agents.external[0].SessionID != "s1" {
		t.Fatalf("external sessions after retry = %+v", m.agents.external)
	}
}

func TestAgentExternalSessionsDialogListsImportAction(t *testing.T) {
	cwd := "/work/project"
	m := &Model{agents: AgentsPane{
		externalOpen:   true,
		externalLoaded: true,
		providers: []agentProvider{
			{ID: "claude", Name: "Claude Code", Installed: true},
		},
		external: []agentExternalSession{{
			Provider:  "claude",
			SessionID: "session-1",
			Title:     "Fix the failing test",
			Cwd:       &cwd,
		}},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentsExternalSessionsDialog(c, c.Theme())
	}, 760, 620)
	tester.Frame()
	for _, text := range []string{
		"Agent sessions",
		"Sessions started in a terminal or another editor",
		"Fix the failing test",
		"Claude Code · /work/project",
		"Import",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Agent sessions lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestAgentExternalSessionsLoadAndImportRPC(t *testing.T) {
	calls := make(chan struct {
		command string
		args    map[string]any
	}, 4)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- struct {
				command string
				args    map[string]any
			}{command: command, args: args}
			switch command {
			case "agent_external_sessions":
				return json.RawMessage(`[{"provider":"codex","sessionId":"s1","title":"External work"}]`), nil
			case "agent_import_session":
				return json.RawMessage(`{"id":"imported","title":"External work","provider":"codex","status":"ready"}`), nil
			case "agent_entries":
				return json.RawMessage(`[]`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
		agents: AgentsPane{
			generation:   3,
			externalOpen: true,
			tasks:        map[string]agentTask{},
			entries:      map[string][]agentEntry{},
			entriesBusy:  map[string]bool{},
			entriesErr:   map[string]string{},
			taskBusy:     map[string]bool{},
		},
	}
	m.setRoot("/project")
	m.loadAgentExternalSessions()
	requireCall := func(command string) map[string]any {
		t.Helper()
		select {
		case call := <-calls:
			if call.command != command {
				t.Fatalf("command = %q, want %q", call.command, command)
			}
			return call.args
		case <-time.After(5 * time.Second):
			t.Fatalf("%s was not called", command)
			return nil
		}
	}
	requireCall("agent_external_sessions")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(m.agents.external) == 0 {
		time.Sleep(time.Millisecond)
	}
	if len(m.agents.external) != 1 || m.agents.external[0].Title != "External work" {
		t.Fatalf("external sessions = %+v", m.agents.external)
	}
	session := m.agents.external[0]
	m.importAgentExternalSession(session)
	args := requireCall("agent_import_session")
	if args["provider"] != "codex" || args["sessionId"] != "s1" {
		t.Fatalf("import arguments = %#v", args)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if task, ok := m.agents.tasks["imported"]; ok && task.ID == "imported" &&
			!m.agents.externalOpen && m.agents.externalBusy == "" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("imported task was not opened: %+v", m.agents)
}

func stringPointer(value string) *string {
	return &value
}
