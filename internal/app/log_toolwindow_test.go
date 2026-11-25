package app

import (
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
