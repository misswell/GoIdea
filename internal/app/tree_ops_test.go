package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func menuItem(t *testing.T, items []treeMenuItem, label string) treeMenuItem {
	t.Helper()
	for _, it := range items {
		if it.Label == label {
			return it
		}
	}
	t.Fatalf("menu has no %q: %v", label, treeMenuLabels(items))
	return treeMenuItem{}
}

func TestTreeMenuFollowsTheSelection(t *testing.T) {
	root := treeTarget{path: "/p", dir: true, root: true}
	items := treeMenuItems(root, false)
	if got := menuItem(t, items, "New"); len(got.Items) != 2 ||
		got.Items[0].Label != "File…" || got.Items[1].Label != "Directory…" {
		t.Fatalf("New submenu = %+v", got.Items)
	}
	if menuItem(t, items, "Cut").Enabled {
		t.Fatal("the project root can be cut")
	}
	if menuItem(t, items, "Paste").Enabled {
		t.Fatal("paste is enabled with an empty clipboard")
	}
	if menuItem(t, items, "Rename…").Enabled || menuItem(t, items, "Delete…").Enabled {
		t.Fatal("the project root can be renamed or deleted")
	}
	if menuItem(t, items, "Jump to Source").Enabled {
		t.Fatal("a folder can jump to source")
	}
	if !menuItem(t, items, "Open in Terminal").Enabled {
		t.Fatal("a folder cannot open a terminal")
	}

	file := treeTarget{path: "/p/A.java"}
	items = treeMenuItems(file, true)
	if !menuItem(t, items, "Cut").Enabled || !menuItem(t, items, "Paste").Enabled {
		t.Fatal("a file or a filled clipboard is disabled")
	}
	if !menuItem(t, items, "Jump to Source").Enabled {
		t.Fatal("a file cannot jump to source")
	}
	if menuItem(t, items, "Open in Terminal").Enabled {
		t.Fatal("a file opens a terminal")
	}
}

func TestTreeMenuLabelsFlattenSubmenus(t *testing.T) {
	labels := treeMenuLabels(treeMenuItems(treeTarget{path: "/p", dir: true, root: true}, false))
	joined := strings.Join(labels, "|")
	for _, want := range []string{"New/File…/Directory…", "Copy Path/Reference/Absolute Path/File Name/Path From Content Root", "Delete…"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("labels %q miss %q", joined, want)
		}
	}
}

func TestTreeGitSubmenuNeedsARepository(t *testing.T) {
	labels := treeMenuLabels(treeMenuItems(treeTarget{path: "/p", dir: true, root: true}, false))
	if strings.Contains(strings.Join(labels, "|"), "Git") {
		t.Fatalf("the Git submenu shows without a Git repository: %v", labels)
	}
}

func TestTreeGitSubmenuFollowsTheSelection(t *testing.T) {
	root := treeTarget{path: "/p", dir: true, root: true, git: true}
	items := menuItem(t, treeMenuItems(root, false), "Git").Items
	if !menuItem(t, items, "Commit…").Enabled {
		t.Fatal("a folder cannot commit")
	}
	ignore := menuItem(t, items, "Add to .gitignore")
	for _, dest := range []string{".gitignore", ".git/info/exclude"} {
		if menuItem(t, ignore.Items, dest).Enabled {
			t.Fatalf("the project root can be added to %s", dest)
		}
	}
	if menuItem(t, items, "Annotate with Git Blame").Enabled {
		t.Fatal("a folder can be annotated")
	}
	if menuItem(t, items, "Add").Enabled {
		t.Fatal("a folder can be added to VCS")
	}
	if !menuItem(t, items, "Branches…").Enabled {
		t.Fatal("the branches popup is disabled")
	}

	file := treeTarget{path: "/p/src/A.java", git: true, kind: "modified", rollback: true}
	items = menuItem(t, treeMenuItems(file, false), "Git").Items
	if !menuItem(t, items, "Commit File…").Enabled || strings.Contains(strings.Join(treeMenuLabels(items), "|"), "Commit…") {
		t.Fatal("a file commits itself alone")
	}
	if menuItem(t, items, "Add").Enabled {
		t.Fatal("a modified file can be added to VCS")
	}
	if !menuItem(t, items, "Annotate with Git Blame").Enabled || !menuItem(t, items, "Show Diff").Enabled {
		t.Fatal("a file cannot be annotated or diffed")
	}
	if !menuItem(t, items, "Rollback…").Enabled {
		t.Fatal("a working-tree change cannot be rolled back")
	}
	untracked := treeTarget{path: "/p/src/New.java", git: true, kind: "untracked"}
	items = menuItem(t, treeMenuItems(untracked, false), "Git").Items
	if !menuItem(t, items, "Add").Enabled {
		t.Fatal("an unversioned file cannot be added")
	}
	if menuItem(t, items, "Rollback…").Enabled {
		t.Fatal("an unversioned file has no working-tree change to roll back")
	}
}

func TestUnderTreePathKeepsSiblingsApart(t *testing.T) {
	base := filepath.Join("/p", "src")
	if !underTreePath(base, base) {
		t.Fatal("a path is not under itself")
	}
	if !underTreePath(filepath.Join(base, "A.java"), base) {
		t.Fatal("a file inside the folder is not under it")
	}
	if underTreePath(filepath.Join("/p", "src2"), base) {
		t.Fatal("a sibling folder counts as inside")
	}
}
