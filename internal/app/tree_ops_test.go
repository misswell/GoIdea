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
