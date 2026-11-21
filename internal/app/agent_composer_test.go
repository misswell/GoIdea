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

func TestAgentCompletionAtAndApply(t *testing.T) {
	text := "Fix @src/App"
	completion, ok := agentCompletionAt(text, len([]rune(text)))
	if !ok || completion.Kind != "file" || completion.Query != "src/App" ||
		completion.From != 4 || completion.To != len([]rune(text)) {
		t.Fatalf("completion = %+v, %v", completion, ok)
	}
	if _, ok := agentCompletionAt("mail@example.com", 16); ok {
		t.Fatal("an address was treated as an @ reference")
	}
	if _, ok := agentCompletionAt("text @file", 3); ok {
		t.Fatal("completion began after the caret")
	}

	next, caret := agentApplyCompletion(text, completion, "@src/App.java")
	if next != "Fix @src/App.java " || caret != len([]rune(next)) {
		t.Fatalf("applied = %q caret %d", next, caret)
	}
	next, caret = agentApplyCompletion("Fix @src\nnext", agentCompletionRef{
		Kind: "file", Query: "src", From: 4, To: 8,
	}, "@src/App.java")
	if next != "Fix @src/App.java \nnext" ||
		caret != len([]rune("Fix @src/App.java ")) {
		t.Fatalf("line-break completion = %q caret %d", next, caret)
	}
	next, _ = agentApplyCompletion("Fix @s rest", agentCompletionRef{
		Kind: "file", Query: "s", From: 4, To: 6,
	}, "@src/App.java")
	if next != "Fix @src/App.java rest" {
		t.Fatalf("existing-space completion = %q", next)
	}
	next, caret = agentApplyCompletionGlue("Fix @src", agentCompletionRef{
		Kind: "file", Query: "src", From: 4, To: 8,
	}, "@src/", false)
	if next != "Fix @src/" || caret != len([]rune(next)) {
		t.Fatalf("folder descent = %q caret %d", next, caret)
	}
}

func TestAgentPathQueryMatchesTypedPaths(t *testing.T) {
	for _, query := range []string{".", "..", "./", "../", "./src", "../src", "/src"} {
		if !agentPathQuery(query) {
			t.Fatalf("%q was not treated as a typed path", query)
		}
	}
	for _, query := range []string{"", "src", "a/../b", ".../src", ".hidden"} {
		if agentPathQuery(query) {
			t.Fatalf("%q was treated as a typed path", query)
		}
	}
}

func TestAgentCommandCompletionRanksAndSections(t *testing.T) {
	commands := []agentCommand{
		{Name: "triage", Description: "Sort issues", Source: "skill"},
		{Name: "trip", Description: "Plan a trip"},
		{Name: "plugin:trip", Description: "Plugin command"},
		{Name: "unrelated"},
	}
	items := agentMatchCommands(commands, "tri")
	var labels []string
	for _, item := range items {
		if !item.Section {
			labels = append(labels, item.Label)
		}
	}
	want := []string{"/trip", "/plugin:trip", "/triage"}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("commands = %v, want %v", labels, want)
	}
	if len(items) < 3 || !items[0].Section || items[0].Label != "Commands" ||
		items[len(items)-1].Section || items[len(items)-2].Label != "Skills" {
		t.Fatalf("command sections = %+v", items)
	}
	if !agentLettersInOrder("配置任务", "配置") {
		t.Fatal("ordered matching did not handle Unicode")
	}
}

func TestAgentFileCompletionRPCAndWorktreePaths(t *testing.T) {
	type call struct {
		command string
		args    map[string]any
	}
	calls := make(chan call, 2)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args, _ := input.(map[string]any)
			calls <- call{command: command, args: args}
			switch command {
			case "find_files":
				return json.RawMessage(`[{"path":"/project/src/App.java","relPath":"src/App.java","score":20}]`), nil
			case "find_dirs":
				return json.RawMessage(`[{"path":"/project/src","relPath":"src/","score":10}]`), nil
			default:
				t.Fatalf("command = %q", command)
				return nil, nil
			}
		},
	}
	items, err := m.agentIndexedCompletions(context.Background(), "/worktrees/task", "app")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case got := <-calls:
			if got.args["query"] != "app" || got.args["limit"] != agentCompletionLimit {
				t.Fatalf("%s arguments = %#v", got.command, got.args)
			}
		default:
			t.Fatalf("only %d index calls were made", i)
		}
	}
	if len(items) != 2 || items[0].Path != "/worktrees/task/src/App.java" ||
		items[1].Path != "/worktrees/task/src" || !items[1].Dir {
		t.Fatalf("worktree completions = %+v", items)
	}

	pathCalls := make(chan call, 1)
	m.invokeHook = func(_ context.Context, command string, input any) (json.RawMessage, error) {
		args, _ := input.(map[string]any)
		pathCalls <- call{command: command, args: args}
		return json.RawMessage(`[
			{"name":"Lib.java","dir":false},
			{"name":"Other.java","dir":false}
		]`), nil
	}
	items, err = m.agentPathCompletions(context.Background(), "/worktrees/task", "../lib")
	if err != nil {
		t.Fatal(err)
	}
	got := <-pathCalls
	if got.command != "path_completions" ||
		got.args["from"] != filepath.Join("/worktrees/task", "_") ||
		got.args["prefix"] != "../lib" {
		t.Fatalf("path-completion call = %+v", got)
	}
	if len(items) != 1 || items[0].Rel != "../Lib.java" ||
		items[0].Path != filepath.Clean("/worktrees/Lib.java") {
		t.Fatalf("path completions = %+v", items)
	}
}

func TestAgentComposerCompletionKeysAndMentionPrompt(t *testing.T) {
	m := &Model{agents: AgentsPane{
		composer: "@s",
		completion: agentCompletionState{
			Open: true, Target: "task", Kind: "file", From: 0, To: 2,
			Items: []agentCompletionItem{
				{Section: true, Label: "Files"},
				{Kind: "file", Label: "@src/", Path: "/project/src", Rel: "src/", Dir: true},
				{Kind: "command", Label: "/status", Value: "status"},
			},
		},
	}}
	if !m.HandleAgentComposerInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyTab}) {
		t.Fatal("Tab was not handled")
	}
	if m.agents.composer != "@src/" || !m.agents.completion.Open ||
		m.agents.completionCaret != len([]rune("@src/")) {
		t.Fatalf("folder completion state = %+v", m.agents)
	}
	if len(m.agents.composerFiles) != 0 {
		t.Fatalf("mention became an attachment: %v", m.agents.composerFiles)
	}
	files := agentPromptFiles(m.agents.composer, nil, "/project")
	if !reflect.DeepEqual(files, []string{"/project/src"}) {
		t.Fatalf("mention prompt files = %v", files)
	}
	m.agents.composer = "@src"
	m.agents.completion = agentCompletionState{
		Open: true, Target: "task", Kind: "file", From: 0, To: 4,
		Items: []agentCompletionItem{{Kind: "file", Label: "@src", Path: "/project/src", Rel: "src"}},
	}
	if !m.HandleAgentComposerInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}) {
		t.Fatal("Enter was not handled")
	}
	if m.agents.composer != "@src " || m.agents.completion.Open {
		t.Fatalf("file completion state = %+v", m.agents)
	}

	m.agents.completion.Open = true
	if !m.HandleAgentComposerInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEscape}) {
		t.Fatal("Escape was not handled")
	}
	if m.agents.completion.Open || len(m.agents.completion.Items) != 0 {
		t.Fatalf("Escape left completion open: %+v", m.agents.completion)
	}
}

func TestAgentComposerHistoryAndSendIntent(t *testing.T) {
	m := &Model{agents: AgentsPane{
		conversation: true,
		selected:     "task",
		entries: map[string][]agentEntry{
			"task": {
				{Type: "user", Text: "old prompt"},
				{Type: "agent", Text: "answer"},
				{Type: "user", Text: "new prompt"},
			},
		},
		historyIndex: -1,
	}}
	if !m.HandleAgentComposerInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp}) {
		t.Fatal("Up did not recall history")
	}
	if m.agents.composer != "new prompt" {
		t.Fatalf("first recall = %q", m.agents.composer)
	}
	m.agents.composerCaret = len([]rune(m.agents.composer))
	m.HandleAgentComposerInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp})
	if m.agents.composer != "old prompt" {
		t.Fatalf("second recall = %q", m.agents.composer)
	}
	m.agents.composerCaret = len([]rune(m.agents.composer))
	m.HandleAgentComposerInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyDown})
	if m.agents.composer != "new prompt" {
		t.Fatalf("down recall = %q", m.agents.composer)
	}
	m.agents.composerCaret = len([]rune(m.agents.composer))
	m.HandleAgentComposerInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyDown})
	if m.agents.composer != "" {
		t.Fatalf("recall did not restore the empty draft: %q", m.agents.composer)
	}

	enter := AgentPrefs{}
	if now, ok := agentSendIntent(0, enter); !ok || now {
		t.Fatalf("Enter intent = %v, %v", now, ok)
	}
	if now, ok := agentSendIntent(ui.Cmd, enter); !ok || !now {
		t.Fatalf("Cmd+Enter intent = %v, %v", now, ok)
	}
	if _, ok := agentSendIntent(ui.Shift, enter); ok {
		t.Fatal("Shift+Enter sent with Enter-to-send")
	}
	mod := AgentPrefs{SendWith: "modEnter", WhileRunning: "steer"}
	if now, ok := agentSendIntent(ui.Cmd, mod); !ok || !now {
		t.Fatalf("steer intent = %v, %v", now, ok)
	}
	if now, ok := agentSendIntent(ui.Cmd|ui.Shift, mod); !ok || now {
		t.Fatalf("alternate steer intent = %v, %v", now, ok)
	}
}

func TestAgentComposerDraftsAreIsolatedPerTask(t *testing.T) {
	m := &Model{agents: AgentsPane{
		selected:       "a",
		taskBusy:       map[string]bool{},
		composerDrafts: map[string]agentComposerDraft{},
	}}
	m.agents.composer = "draft for a"
	m.agents.composerFiles = []string{"/repo/A.java"}
	m.agents.composerCaret = 5
	m.agents.historyIndex = 3

	m.switchAgentComposer("b")
	if m.agents.composer != "" || len(m.agents.composerFiles) != 0 {
		t.Fatalf("task b inherited task a's draft: %+v", m.agents)
	}
	m.agents.composer = "draft for b"
	m.agents.composerImages = []agentImage{{MimeType: "image/png", Data: "AA=="}}

	m.switchAgentComposer("a")
	if m.agents.composer != "draft for a" ||
		!reflect.DeepEqual(m.agents.composerFiles, []string{"/repo/A.java"}) ||
		m.agents.composerCaret != 5 || m.agents.historyIndex != 3 {
		t.Fatalf("restored task a draft = %+v", m.agents)
	}
	m.switchAgentComposer("b")
	if m.agents.composer != "draft for b" || len(m.agents.composerImages) != 1 {
		t.Fatalf("restored task b draft = %+v", m.agents)
	}
}

func TestAgentSteerRequiresRunningCapabilitiesAndImageSupport(t *testing.T) {
	running := agentTask{
		Status:       "running",
		Capabilities: agentCapabilities{Steer: true, Images: true},
	}
	if !agentCanSteer(running, nil) || !agentCanSteer(running, []agentImage{{MimeType: "image/png"}}) {
		t.Fatal("a capable running task cannot steer text and images")
	}
	running.Capabilities.Images = false
	if agentCanSteer(running, []agentImage{{MimeType: "image/png"}}) {
		t.Fatal("an image steer ignored the image capability")
	}
	if !agentCanSteer(running, nil) {
		t.Fatal("a missing image capability disabled a text steer")
	}
	running.Status = "ready"
	if agentCanSteer(running, nil) {
		t.Fatal("a ready task accepted a steer")
	}
	running.Status = "running"
	running.Capabilities.Steer = false
	if agentCanSteer(running, nil) {
		t.Fatal("an unsupported provider accepted a steer")
	}
}

func TestAgentComposerShowsQueueAndStopWhileRunning(t *testing.T) {
	task := agentTestTask("a", "Running task", "running", 1)
	task.Cwd = "/repo"
	m := &Model{agents: AgentsPane{
		selected:      "a",
		conversation:  true,
		composer:      "next change",
		tasks:         map[string]agentTask{"a": task},
		entries:       map[string][]agentEntry{"a": {}},
		entriesLoaded: map[string]bool{"a": true},
		entriesBusy:   map[string]bool{},
		entriesErr:    map[string]string{},
		taskBusy:      map[string]bool{},
		timelineOpen:  map[string]bool{},
		unread:        map[string]bool{},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentConversationView(c, c.Theme(), oiPaletteOf(c.Theme()), true)
	}, 900, 700)
	if !tester.HasText("Queue") || !tester.HasText("Stop") {
		t.Fatalf("running composer controls = %q", tester.Texts())
	}
	task.Status = "ready"
	m.agents.tasks["a"] = task
	tester = ui.NewTester(func(c *ui.Context) {
		m.agentConversationView(c, c.Theme(), oiPaletteOf(c.Theme()), true)
	}, 900, 700)
	if !tester.HasText("Send") || tester.HasText("Queue") || tester.HasText("Stop") {
		t.Fatalf("ready composer controls = %q", tester.Texts())
	}
}

func TestAgentSendNowClearsTextFilesAndImages(t *testing.T) {
	calls := make(chan map[string]any, 1)
	posted := make(chan func(), 1)
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { posted <- fn },
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			if command != "agent_prompt" {
				t.Fatalf("command = %q", command)
			}
			calls <- input.(map[string]any)
			return json.RawMessage(`null`), nil
		},
		agents: AgentsPane{
			selected:      "a",
			composer:      "send now",
			composerFiles: []string{"/repo/App.java"},
			composerImages: []agentImage{{
				MimeType: "image/png", Data: "AA==",
			}},
			composerDrafts: map[string]agentComposerDraft{
				"a": {Text: "send now"},
			},
			tasks: map[string]agentTask{
				"a": func() agentTask {
					task := agentTestTask("a", "Task", "running", 1)
					task.Cwd = "/repo"
					task.Capabilities = agentCapabilities{Steer: true, Images: true}
					return task
				}(),
			},
			taskBusy: map[string]bool{},
		},
	}
	m.setRoot("/repo")
	m.submitAgentPromptNow()
	select {
	case args := <-calls:
		prompt, _ := args["prompt"].(map[string]any)
		if args["task"] != "a" || args["now"] != true ||
			prompt["text"] != "send now" || len(prompt["files"].([]string)) != 1 ||
			len(prompt["images"].([]agentImage)) != 1 {
			t.Fatalf("send-now request = %#v", args)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent_prompt was not called")
	}
	select {
	case apply := <-posted:
		apply()
	case <-time.After(5 * time.Second):
		t.Fatal("send-now reply was not posted")
	}
	if m.agents.composer != "" || len(m.agents.composerFiles) != 0 ||
		len(m.agents.composerImages) != 0 || m.agents.composerDrafts["a"].Text != "" {
		t.Fatalf("send-now kept the submitted draft: %+v", m.agents)
	}
}

func TestNewAgentTaskDefaultsToLocalIsolation(t *testing.T) {
	m := &Model{agents: AgentsPane{
		taskBusy: map[string]bool{},
		tasks:    map[string]agentTask{},
	}}
	m.NewAgentTaskOpen()
	if m.agents.newIsolation != "local" {
		t.Fatalf("default isolation = %q", m.agents.newIsolation)
	}
}

func TestAgentPathReferencesBecomeMentionsAndOutsideAttachments(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "src", "App.java")
	outside := filepath.Join(filepath.Dir(root), "Outside.java")
	text, files, added := agentAddPathReferences(
		"Keep this", nil, []string{inside, outside, inside}, root, true)
	if added != 2 || len(files) != 1 || filepath.Clean(files[0]) != filepath.Clean(outside) {
		t.Fatalf("references = %q %v added %d", text, files, added)
	}
	if !strings.Contains(text, "Keep this @src/App.java ") {
		t.Fatalf("inside path did not become a mention: %q", text)
	}
	got := agentPromptFiles(text, files, filepath.Join(root, "worktree"))
	if !containsCleanPath(got, filepath.Join(root, "worktree", "src", "App.java")) ||
		!containsCleanPath(got, outside) {
		t.Fatalf("prompt files = %v", got)
	}
}

func TestAgentCompletionPopupRendersRealItems(t *testing.T) {
	m := &Model{agents: AgentsPane{completion: agentCompletionState{
		Open: true,
		Items: []agentCompletionItem{
			{Section: true, Label: "Commands"},
			{Kind: "command", Label: "/build", Detail: "Build the project"},
			{Kind: "file", Label: "@src/App.java", Detail: "src/App.java"},
		},
	}}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentCompletionsView(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 480, 240)
	tester.Frame()
	for _, text := range []string{"Commands", "/build", "Build the project", "@src/App.java"} {
		if !tester.HasText(text) {
			t.Fatalf("completion popup lacks %q: %q", text, tester.Texts())
		}
	}
}

func containsCleanPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}
