package app

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestTabGitMenuIncludesAdvancedRevisionAndWorktreeActions(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "app")
	path := filepath.Join(repository, "src", "Main.java")
	m := &Model{}
	m.setRoot(root)
	m.vcs.repositories = []vcsRepository{{Root: repository, Head: str("main")}}
	m.vcs.current = repository
	m.vcs.files = []openidea.FileStatus{{
		Path: path, RelPath: "src/Main.java", Unstaged: str("modified"),
	}}

	tester := ui.NewTester(func(c *ui.Context) {
		ui.Button(c, "Git").Menu(func(menu *ui.Menu) { m.tabGitMenu(menu, path) })
	}, 360, 120)
	tester.Frame()
	if err := tester.Click("Git"); err != nil {
		t.Fatal(err)
	}
	menu := tester.Menu()
	for _, label := range []string{
		"Compare with Branch or Revision…",
		"Compare with Revision…",
		"Show History for Selection",
		"Show Current Revision",
		"Checkout Tag or Revision…",
		"Worktrees…",
		"New Worktree…",
		"Manage Remotes…",
		"Resolve Conflicts…",
		"Pull…",
		"Merge…",
		"Rebase…",
		"New Tag…",
		"Stash Changes…",
		"Reset HEAD…",
	} {
		if !slices.Contains(menu, label) {
			t.Errorf("tab Git menu %v misses %q", menu, label)
		}
	}
}

func TestMainGitMenuIncludesAdvancedOperations(t *testing.T) {
	labels := map[string]bool{}
	walkMenu(mainMenuGroups(), func(entry menuEntry) {
		if entry.label != "" {
			labels[entry.label] = true
		}
	})
	for _, label := range []string{
		"Compare with Branch or Revision…",
		"Checkout Tag or Revision…",
		"Worktrees…",
		"New Worktree…",
		"Manage Remotes…",
		"Resolve Conflicts…",
	} {
		if !labels[label] {
			t.Errorf("main menu misses %q", label)
		}
	}
}

func TestVCSToolbarMoreMenuIncludesAdvancedOperations(t *testing.T) {
	m := &Model{}
	m.setRoot("/project")
	m.vcs = VCSPane{
		loaded:       true,
		current:      "/project",
		repositories: []vcsRepository{{Root: "/project", Head: str("main")}},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsToolWindow(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 520, 420)
	tester.Frame()
	if err := tester.Click("More…"); err != nil {
		t.Fatal(err)
	}
	menu := tester.Menu()
	for _, label := range []string{
		"Fetch", "Merge…", "Rebase…", "New Branch…", "New Tag…",
		"Checkout Tag or Revision…", "Compare with Branch or Revision…",
		"Worktrees…", "New Worktree…", "Manage Remotes…",
		"Stash Changes…", "Unstash Changes…", "Reset HEAD…", "Resolve Conflicts…",
	} {
		if !slices.Contains(menu, label) {
			t.Errorf("Git toolbar More menu %v misses %q", menu, label)
		}
	}
}
