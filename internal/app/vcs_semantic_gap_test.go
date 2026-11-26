package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func semanticGapTab(t *testing.T, path, text string, caret int) *Tab {
	t.Helper()
	doc := docbridge.Virtual(path, text)
	state := &editor.State{Buffer: doc.Buffer(), LineHeight: 1.4}
	state.Select(caret, caret)
	return &Tab{doc: doc, state: state}
}

func semanticGapModel(t *testing.T, tab *Tab, hook func(context.Context, string, any) (json.RawMessage, error)) *Model {
	t.Helper()
	root, err := filepath.EvalSymlinks(filepath.Dir(tab.doc.Path()))
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx: context.Background(), tabs: []*Tab{tab}, active: 0,
		postHook: func(fn func()) { fn() }, invokeHook: hook,
	}
	m.setRoot(root)
	m.vcs.current = root
	m.vcs.repositories = []vcsRepository{{Root: root, Head: semanticString("main")}}
	return m
}

func semanticString(value string) *string { return &value }

func semanticJSONString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func waitSemanticGap(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for VCS behavior")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestShowDiffInNewWindowUsesDistinctAuxiliarySurface(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "A.java")
	if err := os.WriteFile(path, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := semanticGapTab(t, path, "new\n", 0)
	m := semanticGapModel(t, tab, func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "vcs_file_at" {
			t.Fatalf("unexpected RPC %s", command)
		}
		return json.RawMessage(`"old\n"`), nil
	})
	m.ShowDiff(path)
	waitSemanticGap(t, func() bool { return m.diff != nil && !m.diff.loading })
	if !m.RunActionWithArg("vcs.showDiffInNewWindow", path) {
		t.Fatal("vcs.showDiffInNewWindow is unavailable")
	}
	if m.diff == nil || m.diff.surface != diffSurfaceWindow {
		t.Fatalf("new-window diff surface = %+v", m.diff)
	}
}

func TestShelfDiffNewTabCreatesDedicatedDiffTab(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "A.java")
	if err := os.WriteFile(path, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := semanticGapTab(t, path, "new\n", 0)
	second := filepath.Join(root, "B.java")
	if err := os.WriteFile(second, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := semanticGapModel(t, tab, func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command != "vcs_shelf_diff" {
			t.Fatalf("unexpected RPC %s", command)
		}
		return json.RawMessage(`[{"path":` + semanticJSONString(path) +
			`,"before":"old\n","after":"new\n"},{"path":` + semanticJSONString(second) +
			`,"before":"left\n","after":"right\n"}]`), nil
	})
	m.vcsMore.shelves = []shelvedChangeList{{ID: "shelf-1", Name: "Work"}}
	// No path opens the whole shelf chain, as OpenIDEA's tree command does
	// when a shelved-change node rather than one file was selected.
	m.RunActionWithArg("vcs.shelf.showDiffNewTab", map[string]any{"id": "shelf-1"})
	waitSemanticGap(t, func() bool { return m.diff != nil && m.diff.tab != nil })
	if m.diff.surface != diffSurfaceNewTab || m.tab() != m.diff.tab ||
		m.tab().kind != tabKindVCSDiff {
		t.Fatalf("new-tab shelf diff = diff %+v tab %+v", m.diff, m.tab())
	}
	if m.diff.leftText != "old\n" || m.diff.rightText != "new\n" {
		t.Fatalf("first shelf file = %q -> %q", m.diff.leftText, m.diff.rightText)
	}
	m.StepDiffFile(1)
	if m.diff.fileIndex != 1 || m.diff.leftText != "left\n" ||
		m.diff.rightText != "right\n" || m.diff.path != realPath(second) {
		t.Fatalf("second shelf file lost its stored text: %+v", m.diff)
	}
}

func TestShowHistoryForSelectionUsesActualSelectedLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "A.java")
	text := "zero\nTWO\nTHREE\nfour\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := semanticGapTab(t, path, text, 0)
	tab.state.SetSelections(editor.Selection{
		Anchor: len("zero\n"), Head: len("zero\nTWO\nTHREE\n"),
	})
	var arguments map[string]any
	m := semanticGapModel(t, tab, func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "vcs_block_history" {
			t.Fatalf("unexpected RPC %s", command)
		}
		arguments = args.(map[string]any)
		return json.RawMessage(`{
			"headLines":[1,2],
			"revisions":[{
				"id":"abc12345","shortId":"abc12345","author":"Ada","timeMs":1,
				"summary":"Change two","path":"A.java",
				"before":"one\ntwo\nthree\n","after":"one\nTWO\nthree\n",
				"beforeLine":1,"afterLine":1
			}]
		}`), nil
	})
	m.RunAction("vcs.showHistoryForSelection")
	waitSemanticGap(t, func() bool {
		return !m.vcsMore.selectionHistory.loading && m.diff != nil
	})
	if got, want := arguments["lines"], [2]int{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selection lines = %#v, want %#v", got, want)
	}
	if realPath(arguments["path"].(string)) != realPath(path) || arguments["text"] != text {
		t.Fatalf("selection history arguments = %#v", arguments)
	}
	if m.vcsMore.selectionHistory.selected != 0 {
		t.Fatalf("selection history selected row = %d, want first revision",
			m.vcsMore.selectionHistory.selected)
	}
}

func TestCompareWithRevisionChoosesFileHistoryThenCurrentFile(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "A.java")
	if err := os.WriteFile(path, []byte("worktree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := semanticGapTab(t, path, "worktree\n", 0)
	var calls []recordedBackendCall
	m := semanticGapModel(t, tab, func(_ context.Context, command string, args any) (json.RawMessage, error) {
		calls = append(calls, recordedBackendCall{command: command, args: args})
		switch command {
		case "vcs_log":
			return json.RawMessage(`[{
				"id":"abcdef123456","shortId":"abcdef12","author":"Ada",
				"timeMs":1,"summary":"Earlier"
			}]`), nil
		case "vcs_file_at":
			return json.RawMessage(`"revision\n"`), nil
		default:
			t.Fatalf("unexpected RPC %s", command)
			return nil, nil
		}
	})
	m.RunActionWithArg("vcs.compareWithRevision", path)
	waitSemanticGap(t, func() bool {
		return !m.vcsMore.loading && len(m.vcsMore.items) == 1
	})
	query := calls[0].args.(map[string]any)["query"].(map[string]any)
	if query["path"] != realPath(path) || query["follow"] != true {
		t.Fatalf("file history query = %#v", query)
	}
	m.chooseFileRevision()
	waitSemanticGap(t, func() bool {
		return m.diff != nil && !m.diff.loading
	})
	if m.diff.leftText != "revision\n" || m.diff.rightText != "worktree\n" ||
		m.diff.path != path {
		t.Fatalf("revision diff = %+v", m.diff)
	}
}

func TestCompareWithBranchStillPromptsForARevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "A.java")
	tab := semanticGapTab(t, path, "current\n", 0)
	m := semanticGapModel(t, tab, func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		t.Fatalf("branch compare unexpectedly called %s before a revision was chosen", command)
		return nil, nil
	})

	m.RunActionWithArg("vcs.compareWithBranch", path)
	if !m.vcsMore.open || m.vcsMore.action != "compare.revision" ||
		m.vcsMore.title != "Compare with Branch or Revision" {
		t.Fatalf("branch compare prompt = %+v", m.vcsMore)
	}
}

func TestPushUpToHerePassesSelectedCommitAndRepository(t *testing.T) {
	first, second := "/project/app", "/project/admin"
	m := &Model{
		ctx:                      context.Background(),
		postHook:                 func(fn func()) { fn() },
		refreshAfterVCSWriteHook: func() {},
		log: LogPane{
			visible: true, selected: 0,
			commitRepos: []string{second},
			commits: []openidea.Commit{{
				ID: "222222222222", ShortID: "22222222", Summary: "Boundary",
			}},
		},
	}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{
		{Root: first, Head: semanticString("main")},
		{Root: second, Head: semanticString("feature")},
	}
	var request map[string]any
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "vcs_branch_info":
			request = args.(map[string]any)
			return json.RawMessage(`{
				"head":"feature","headCommit":"222222222222","upstream":"origin/release","ahead":1
			}`), nil
		case "vcs_remotes":
			return json.RawMessage(`[
				{"name":"upstream","url":"git@example:team/app.git"},
				{"name":"origin","url":"git@example:mirror/app.git"}
			]`), nil
		case "vcs_remote":
			request = args.(map[string]any)
			return json.RawMessage(`null`), nil
		default:
			t.Fatalf("unexpected RPC %s", command)
			return nil, nil
		}
	}
	options, err := m.revisionPushOptions(
		second, "feature", "222222222222", semanticString("upstream/release"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	m.vcsOps = VCSOperationPane{
		kind: vcsOpPush, open: true, busy: true, repo: second,
		pushRevision: "222222222222", pushRepository: second,
	}
	m.runVCSRemoteAt(vcsRepositoryArgFor(second), vcsOpPush, "Push All up to Here", options)
	waitSemanticGap(t, func() bool {
		return !m.vcsOps.busy
	})
	if m.vcsOps.err != "" {
		t.Fatalf("push failed: %s", m.vcsOps.err)
	}
	if request["repo"] != second {
		t.Fatalf("push repository = %#v, want %s", request["repo"], second)
	}
	op := request["op"].(map[string]any)
	if op["op"] != "pushBranch" || op["branch"] != "feature" ||
		op["revision"] != "222222222222" ||
		op["remote"] != "upstream" || op["target"] != "release" {
		t.Fatalf("pushBranch options = %#v", op)
	}
}

func TestPushUpToHereRejectsAStaleRepositorySelection(t *testing.T) {
	first, second := "/project/app", "/project/admin"
	m := &Model{
		ctx:                      context.Background(),
		postHook:                 func(fn func()) { fn() },
		refreshAfterVCSWriteHook: func() {},
		log: LogPane{
			visible: true, selected: 0,
			commitRepos: []string{second},
			commits: []openidea.Commit{{
				ID: "222222222222", ShortID: "22222222", Summary: "Boundary",
			}},
		},
	}
	m.setRoot("/project")
	m.vcs.current = first
	m.vcs.repositories = []vcsRepository{{Root: first, Head: semanticString("main")}}
	called := false
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		called = true
		t.Fatalf("stale push called %s", command)
		return nil, nil
	}
	m.OpenGitPushAtSelectedCommit()
	if m.vcsOps.err == "" || called {
		t.Fatalf("stale selected repository = %+v", m.vcsOps)
	}
}

func TestPushUpToHereDisallowsForceToProtectedBranch(t *testing.T) {
	repo := "/project/app"
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.git.protectedBranches": "main",
	})
	m.refreshAfterVCSWriteHook = func() {}
	m.log = LogPane{
		visible: true, selected: 0,
		commitRepos: []string{repo},
		commits: []openidea.Commit{{
			ID: "222222222222", ShortID: "22222222", Summary: "Boundary",
		}},
	}
	m.vcs.current = repo
	m.vcs.repositories = []vcsRepository{{Root: repo, Head: semanticString("feature")}}
	m.vcsOps = VCSOperationPane{
		open: true, kind: vcsOpPush, repo: repo,
		pushRevision: "222222222222", pushRepository: repo, force: true,
	}
	m.invokeHook = func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		if command == "vcs_branch_info" {
			return json.RawMessage(`{"head":"main","upstream":"origin/main"}`), nil
		}
		if command != "vcs_remotes" {
			t.Fatalf("protected push called %s", command)
		}
		return json.RawMessage(`[{"name":"origin","url":"git@example:team/app.git"}]`), nil
	}
	m.submitVCSOperation()
	waitSemanticGap(t, func() bool { return !m.vcsOps.busy })
	if m.vcsOps.err != "Force push is not allowed to protected branches" {
		t.Fatalf("protected push error = %q", m.vcsOps.err)
	}
}

func TestNextChangeNavigatesActiveEditorWithoutDiff(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "A.java")
	text := "same\nchanged\nsame\nlater\n"
	tab := semanticGapTab(t, path, text, 0)
	var commands []string
	m := semanticGapModel(t, tab, func(_ context.Context, command string, args any) (json.RawMessage, error) {
		commands = append(commands, command)
		switch command {
		case "vcs_file_at":
			return json.RawMessage(`"same\nold\nsame\nold\n"`), nil
		case "vcs_diff_hunks":
			input := args.(map[string]any)
			if input["oldText"] != "same\nold\nsame\nold\n" || input["newText"] != text {
				t.Fatalf("hunk texts = %#v", input)
			}
			return json.RawMessage(`[
				{"start":1,"lines":1,"kind":"modified"},
				{"start":3,"lines":1,"kind":"modified"}
			]`), nil
		default:
			t.Fatalf("unexpected RPC %s", command)
			return nil, nil
		}
	})
	m.RunAction("vcs.nextChange")
	waitSemanticGap(t, func() bool {
		return tab.state.Buffer.Snapshot().LineOf(tab.state.Cursor()) == 1
	})
	if m.diff != nil || !reflect.DeepEqual(commands, []string{"vcs_file_at", "vcs_diff_hunks"}) {
		t.Fatalf("editor change navigation = diff %+v commands %v", m.diff, commands)
	}
}

func TestPreviousChangeNavigatesActiveEditorWithoutDiff(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "A.java")
	text := "same\nchanged\nsame\nlater\n"
	tab := semanticGapTab(t, path, text, len(text))
	m := semanticGapModel(t, tab, func(_ context.Context, command string, _ any) (json.RawMessage, error) {
		switch command {
		case "vcs_file_at":
			return json.RawMessage(`"same\nold\nsame\nold\n"`), nil
		case "vcs_diff_hunks":
			return json.RawMessage(`[
				{"start":1,"lines":1,"kind":"modified"},
				{"start":3,"lines":1,"kind":"modified"}
			]`), nil
		default:
			t.Fatalf("unexpected RPC %s", command)
			return nil, nil
		}
	})
	m.RunAction("vcs.prevChange")
	waitSemanticGap(t, func() bool {
		return tab.state.Buffer.Snapshot().LineOf(tab.state.Cursor()) == 3
	})
	if m.diff != nil {
		t.Fatalf("previous-change navigation opened a diff: %+v", m.diff)
	}
}

func TestRecentChangesCollapsesEventsAndNavigatesToChosenEntry(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "A.java")
	if err := os.WriteFile(path, []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var historyPath string
	m := &Model{
		ctx:                      context.Background(),
		postHook:                 func(fn func()) { fn() },
		refreshAfterVCSWriteHook: func() {},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "history_list_dir":
				if args.(map[string]any)["path"] == root {
					historyPath = root
					return json.RawMessage(`{"entries":[
						{"id":"new","path":` + semanticJSONString(path) + `,"timestampMs":30,"label":"Saved","kind":"snapshot"},
						{"id":"old","path":` + semanticJSONString(path) + `,"timestampMs":20,"label":"Saved","kind":"snapshot"},
						{"id":"label","path":` + semanticJSONString(root) + `,"timestampMs":25,"label":"Before refactor","kind":"label"}
					]}`), nil
				}
				historyPath, _ = args.(map[string]any)["path"].(string)
				return json.RawMessage(`null`), nil
			default:
				historyPath, _ = args.(map[string]any)["path"].(string)
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot(root)
	m.vcs.repositories = []vcsRepository{{Root: root}}
	m.OpenRecentChanges()
	waitSemanticGap(t, func() bool {
		return !m.recentHistory.loading && len(m.recentHistory.entries) > 0
	})
	if len(m.recentHistory.entries) != 3 {
		t.Fatalf("collapsed recent changes = %+v", m.recentHistory.entries)
	}
	m.recentHistory.selected = 0
	m.chooseRecentHistory()
	waitSemanticGap(t, func() bool {
		return historyPath == realPath(path)
	})
	if m.recentHistory.open || m.history == nil || m.history.path != realPath(path) || historyPath != realPath(path) {
		t.Logf("calls historyPath=%q last command data", historyPath)
		t.Fatalf("recent history navigation = state %+v history %+v path %q",
			m.recentHistory, m.history, historyPath)
	}
	if strings.Contains(m.history.err, "Cannot read") {
		t.Fatalf("history navigation error = %s", m.history.err)
	}

	m.recentHistory = recentChangesState{
		open: true, entries: m.recentHistory.entries,
	}
	m.recentHistory.open = false
	m.recentHistory.entries = []recentHistoryChange{
		{entry: historyEntry{ID: "new", Path: path, Kind: "snapshot"}},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.recentHistory.open = true
		m.renderOverlays(c, c.Theme())
	}, 900, 600)
	tt.SetDark(true)
	tt.Frame()
	if !tt.HasText("Recent Changes") || !tt.HasText(filepath.Base(path)) {
		t.Fatalf("recent changes popup is missing: %v", tt.Texts())
	}
	plain := collapseRecentHistory([]historyEntry{
		{ID: "newer", Path: filepath.Join(root, "B.java"), Ms: 30, Label: "Saved", Kind: "snapshot"},
		{ID: "older", Path: filepath.Join(root, "B.java"), Ms: 20, Label: "Saved", Kind: "snapshot"},
	}, 40)
	if len(plain) != 1 || plain[0].ID != "newer" {
		t.Fatalf("same-file event collapse = %+v", plain)
	}
}
