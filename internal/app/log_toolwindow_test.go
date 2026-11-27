package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestGitToolWindowUsesSingleBottomWindow(t *testing.T) {
	spec := dockSpec("log")
	if spec == nil {
		t.Fatal("Git tool window is not registered")
	}
	if spec.title != "Git" {
		t.Fatalf("Git tool window title = %q, want Git", spec.title)
	}
	if spec.anchor != dockBottom {
		t.Fatalf("Git tool window anchor = %s, want bottom", dockSlotNames[spec.anchor])
	}
}

func TestGitToolWindowTabsSelectTheirNativeContent(t *testing.T) {
	root := "/project"
	m := &Model{}
	m.setRoot(root)
	m.vcs = VCSPane{
		loaded:       true,
		current:      root,
		repositories: []vcsRepository{{Root: root, Head: str("main")}},
		content:      map[string]vcsContentTab{root: vcsContentChanges},
		stashes:      &vcsStashPane{loaded: true},
		worktrees: map[string]*vcsWorktreePane{
			root: {loaded: true},
		},
	}
	m.log = LogPane{
		visible: true,
		loaded:  true,
		repo:    root,
		commits: []openidea.Commit{{
			ID:      "0123456789abcdef",
			ShortID: "0123456",
			Summary: "baseline",
		}},
	}

	tester := ui.NewTester(func(c *ui.Context) {
		m.logPanel(c, c.Theme())
	}, 640, 520)
	tester.Frame()
	for _, label := range []string{"Local Changes", "Stash", "Log", "Console", "Worktrees"} {
		if !tester.HasText(label) {
			t.Fatalf("Git tool window is missing tab %q: %v", label, tester.Texts())
		}
	}
	if !tester.HasText("No local changes") {
		t.Fatalf("Local Changes content is missing: %v", tester.Texts())
	}

	assertGitTabContent(t, tester, m, "Stash", vcsContentStash, "No stashes")
	assertGitTabContent(t, tester, m, "Log", vcsContentLog, "baseline")
	assertGitTabContent(t, tester, m, "Console", vcsContentConsole,
		"Git commands run by the IDE appear here.")
	assertGitTabContent(t, tester, m, "Worktrees", vcsContentWorktrees, "No worktrees")
	assertGitTabContent(t, tester, m, "Local Changes", vcsContentChanges, "No local changes")
}

func TestShowLogSelectsTheGitWindowLogTab(t *testing.T) {
	root := "/project"
	m := &Model{}
	m.setRoot(root)
	m.vcs = VCSPane{
		loaded:       true,
		current:      root,
		repositories: []vcsRepository{{Root: root, Head: str("main")}},
		content:      map[string]vcsContentTab{root: vcsContentChanges},
	}

	m.ShowLog()
	if got := m.currentVCSContent(); got != vcsContentLog {
		t.Fatalf("ShowLog selected Git tab %v, want Log", got)
	}
	if !m.log.visible {
		t.Fatal("ShowLog did not show the Git tool window")
	}
}

func TestGitConsoleMirrorsTypedOperationOutputAndFollowsItsLimit(t *testing.T) {
	m := &Model{vcsOps: VCSOperationPane{channel: 17}}
	m.logGitConsole(gitConsoleCommand, "Fetch\n\nReceiving objects")
	m.logGitConsole(gitConsoleInfo, "Fetch completed")
	if !m.acceptVCSOperationOutput(17, []byte(`"From github.com:example/repo\n"`)) {
		t.Fatal("Git operation output was not accepted")
	}
	if m.acceptVCSOperationOutput(18, []byte(`"wrong channel"`)) {
		t.Fatal("output from another channel was accepted")
	}
	if len(m.log.console) != 5 {
		t.Fatalf("Git console lines = %d, want 5", len(m.log.console))
	}
	want := []struct {
		kind gitConsoleKind
		text string
	}{
		{gitConsoleCommand, "Fetch"},
		{gitConsoleCommand, ""},
		{gitConsoleCommand, "Receiving objects"},
		{gitConsoleInfo, "Fetch completed"},
		{gitConsoleOutput, "From github.com:example/repo"},
	}
	for i, item := range want {
		if m.log.console[i].kind != item.kind || m.log.console[i].text != item.text {
			t.Fatalf("Git console line %d = %+v, want kind=%q text=%q",
				i, m.log.console[i], item.kind, item.text)
		}
	}

	m = &Model{}
	for i := 0; i < maxGitConsoleLines+5; i++ {
		m.logGitConsole(gitConsoleInfo, fmt.Sprintf("line %d", i))
	}
	if len(m.log.console) != maxGitConsoleLines {
		t.Fatalf("bounded Git console lines = %d, want %d", len(m.log.console), maxGitConsoleLines)
	}
	if got := m.log.console[0].text; got != "line 5" {
		t.Fatalf("first bounded Git console line = %q, want line 5", got)
	}
}

func TestGitConsoleRendersCommandOutputErrorsAndClears(t *testing.T) {
	root := "/project"
	m := &Model{}
	m.setRoot(root)
	m.vcs = VCSPane{
		loaded:       true,
		current:      root,
		repositories: []vcsRepository{{Root: root, Head: str("main")}},
		content:      map[string]vcsContentTab{root: vcsContentConsole},
	}
	m.log.visible = true
	m.log.repo = root
	m.logGitConsole(gitConsoleCommand, "Fetch")
	m.logGitConsole(gitConsoleOutput, "Receiving objects")
	m.logGitConsole(gitConsoleError, "fatal: authentication failed")

	tester := ui.NewTester(func(c *ui.Context) {
		m.logPanel(c, c.Theme())
	}, 640, 520)
	tester.Frame()
	for _, text := range []string{"Fetch", "Receiving objects", "fatal: authentication failed"} {
		if !tester.HasText(text) {
			t.Fatalf("Git console is missing %q: %v", text, tester.Texts())
		}
	}
	if err := tester.Click("Clear All"); err != nil {
		t.Fatalf("click Clear All: %v", err)
	}
	tester.Frame()
	if len(m.log.console) != 0 {
		t.Fatalf("Clear All left %d Git console lines", len(m.log.console))
	}
	if !tester.HasText("Git commands run by the IDE appear here.") {
		t.Fatalf("empty Git console is missing its hint: %v", tester.Texts())
	}
}

func assertGitTabContent(
	t *testing.T,
	tester *ui.Tester,
	m *Model,
	label string,
	tab vcsContentTab,
	want string,
) {
	t.Helper()
	if err := tester.Click(label); err != nil {
		t.Fatalf("click %s: %v", label, err)
	}
	tester.Frame()
	if !tester.HasText(want) {
		t.Fatalf("%s content is missing %q: %v", label, want, tester.Texts())
	}
	if got := m.gitToolWindowTab(); got != tab {
		t.Fatalf("%s selected Git tab %v, want %v", label, got, tab)
	}
}
