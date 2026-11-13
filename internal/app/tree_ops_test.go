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
	if got := menuItem(t, items, "新建"); len(got.Items) != 2 ||
		got.Items[0].Label != "文件…" || got.Items[1].Label != "目录…" {
		t.Fatalf("New submenu = %+v", got.Items)
	}
	if menuItem(t, items, "剪切").Enabled {
		t.Fatal("the project root can be cut")
	}
	if menuItem(t, items, "粘贴").Enabled {
		t.Fatal("paste is enabled with an empty clipboard")
	}
	if menuItem(t, items, "重命名…").Enabled || menuItem(t, items, "删除…").Enabled {
		t.Fatal("the project root can be renamed or deleted")
	}
	if menuItem(t, items, "跳到源码").Enabled {
		t.Fatal("a folder can jump to source")
	}
	if !menuItem(t, items, "在终端中打开").Enabled {
		t.Fatal("a folder cannot open a terminal")
	}

	file := treeTarget{path: "/p/A.java"}
	items = treeMenuItems(file, true)
	if !menuItem(t, items, "剪切").Enabled || !menuItem(t, items, "粘贴").Enabled {
		t.Fatal("a file or a filled clipboard is disabled")
	}
	if !menuItem(t, items, "跳到源码").Enabled {
		t.Fatal("a file cannot jump to source")
	}
	if menuItem(t, items, "在终端中打开").Enabled {
		t.Fatal("a file opens a terminal")
	}
}

func TestTreeMenuLabelsFlattenSubmenus(t *testing.T) {
	labels := treeMenuLabels(treeMenuItems(treeTarget{path: "/p", dir: true, root: true}, false))
	joined := strings.Join(labels, "|")
	for _, want := range []string{"新建/文件…/目录…", "复制路径/引用/绝对路径/文件名/从内容根开始的路径", "删除…"} {
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
	if !menuItem(t, items, "提交…").Enabled {
		t.Fatal("a folder cannot commit")
	}
	ignore := menuItem(t, items, "添加到 .gitignore")
	for _, dest := range []string{".gitignore", ".git/info/exclude"} {
		if menuItem(t, ignore.Items, dest).Enabled {
			t.Fatalf("the project root can be added to %s", dest)
		}
	}
	if menuItem(t, items, "使用 Git Blame 注解").Enabled {
		t.Fatal("a folder can be annotated")
	}
	if menuItem(t, items, "添加").Enabled {
		t.Fatal("a folder can be added to VCS")
	}
	if !menuItem(t, items, "分支…").Enabled {
		t.Fatal("the branches popup is disabled")
	}

	file := treeTarget{path: "/p/src/A.java", git: true, kind: "modified", rollback: true}
	items = menuItem(t, treeMenuItems(file, false), "Git").Items
	if !menuItem(t, items, "提交文件…").Enabled || strings.Contains(strings.Join(treeMenuLabels(items), "|"), "提交…") {
		t.Fatal("a file commits itself alone")
	}
	if menuItem(t, items, "添加").Enabled {
		t.Fatal("a modified file can be added to VCS")
	}
	if !menuItem(t, items, "使用 Git Blame 注解").Enabled || !menuItem(t, items, "显示差异").Enabled {
		t.Fatal("a file cannot be annotated or diffed")
	}
	if !menuItem(t, items, "回滚…").Enabled {
		t.Fatal("a working-tree change cannot be rolled back")
	}
	untracked := treeTarget{path: "/p/src/New.java", git: true, kind: "untracked"}
	items = menuItem(t, treeMenuItems(untracked, false), "Git").Items
	if !menuItem(t, items, "添加").Enabled {
		t.Fatal("an unversioned file cannot be added")
	}
	if menuItem(t, items, "回滚…").Enabled {
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
