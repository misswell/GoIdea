package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func hierarchyTestModel() *Model {
	m := &Model{}
	m.hierarchy = hierarchyPane{
		mode: "call", direction: "incoming",
		rootName: "main", rootPath: "/p/App.java", rootLine: 4, rootCol: 20,
		children: map[string][]hierarchyNode{},
		expanded: map[string]bool{}, loading: map[string]bool{},
		failed: map[string]string{}, gen: 1,
	}
	m.rightTool = "hierarchy"
	return m
}

func hierarchyItemAt(name string, line, col int) hierarchyItem {
	pos := openidea.OutlinePos{Line: uint32(line), Col: uint32(col)}
	return hierarchyItem{
		Name: name, Kind: "method", Path: "/p/App.java",
		SelectionRange: openidea.OutlineRange{Start: pos, End: pos},
	}
}

func TestWordAtCaret(t *testing.T) {
	text := "public static void main(String[] args) {"
	at := len("public static void mai")
	if got := wordAtCaret(text, at); got != "main" {
		t.Fatalf("word = %q, want main", got)
	}
	if got := wordAtCaret(text, 0); got != "public" {
		t.Fatalf("word at 0 = %q, want public", got)
	}
	if got := wordAtCaret(text, len(text)); got != "" {
		t.Fatalf("word at end = %q, want empty", got)
	}
}

func TestHierarchyNodesMarkRecursionAndIds(t *testing.T) {
	m := hierarchyTestModel()
	root := hierarchyKey(m.hierarchy.rootPath, m.hierarchy.rootLine, m.hierarchy.rootCol, m.hierarchy.rootName)
	m.hierarchy.children[hierarchyRoot] = []hierarchyNode{
		{id: hierarchyRoot + "/" + root, parent: hierarchyRoot,
			item: hierarchyItemAt("main", m.hierarchy.rootLine, m.hierarchy.rootCol)},
	}
	m.hierarchy.expanded[hierarchyRoot+"/"+root] = true
	// The callee's own level repeats main, which is an ancestor.
	items := []hierarchyItem{
		hierarchyItemAt("main", m.hierarchy.rootLine, m.hierarchy.rootCol),
		hierarchyItemAt("run", 9, 3),
	}
	nodes := m.hierarchyNodes(hierarchyRoot+"/"+root, m.hierarchy.rootPath, items)
	if len(nodes) != 2 {
		t.Fatalf("nodes = %+v, want two", nodes)
	}
	if !nodes[0].recursion {
		t.Fatalf("repeated main is not marked recursive: %+v", nodes[0])
	}
	if nodes[1].recursion || nodes[1].depth != 1 {
		t.Fatalf("run = %+v, want a depth 1 child", nodes[1])
	}
	if len(m.hierarchy.children[nodes[0].id]) != 0 || m.hierarchy.children[nodes[0].id] == nil {
		t.Fatalf("recursion leaf = %v, want an empty level", m.hierarchy.children[nodes[0].id])
	}
}

func TestHierarchyRowsFollowExpansion(t *testing.T) {
	m := hierarchyTestModel()
	parent := hierarchyItemAt("caller", 1, 0)
	m.hierarchy.children[hierarchyRoot] = []hierarchyNode{
		{id: hierarchyRoot + "/a", parent: hierarchyRoot, item: parent},
	}
	parentID := hierarchyRoot + "/a"
	m.hierarchy.children[parentID] = []hierarchyNode{
		{id: parentID + "/b", parent: parentID, item: hierarchyItemAt("callee", 2, 0), depth: 1},
	}
	if rows := m.hierarchyRows(); len(rows) != 1 {
		t.Fatalf("collapsed rows = %+v, want the parent only", rows)
	}
	m.hierarchy.expanded[parentID] = true
	rows := m.hierarchyRows()
	if len(rows) != 2 || rows[1].item.Name != "callee" {
		t.Fatalf("expanded rows = %+v", rows)
	}
}

func TestHierarchyLabels(t *testing.T) {
	m := hierarchyTestModel()
	if got := m.hierarchyDirectionLabel("incoming"); got != "Callers" {
		t.Fatalf("incoming = %q, want Callers", got)
	}
	if got := m.hierarchyDirectionLabel("outgoing"); got != "Callees" {
		t.Fatalf("outgoing = %q, want Callees", got)
	}
	if got := m.hierarchyEmptyLabel(); got != "— no callers found" {
		t.Fatalf("empty = %q", got)
	}
	m.hierarchy.mode = "type"
	if got := m.hierarchyDirectionLabel("incoming"); got != "Supertypes" {
		t.Fatalf("type incoming = %q, want Supertypes", got)
	}
}

func TestLayoutKeepsTheRightArea(t *testing.T) {
	l := normalizeLayout(Layout{Right: "hierarchy", RightHidden: true, RightWidth: 320})
	if l.Right != "hierarchy" || !l.RightHidden || l.RightWidth != 320 {
		t.Fatalf("layout = %+v", l)
	}
	if got := normalizeLayout(Layout{Right: "unknown"}); got.Right != "" {
		t.Fatalf("unknown right window kept: %+v", got)
	}
	if got := normalizeLayout(Layout{Right: "hierarchy", RightWidth: 10}); got.RightWidth != 0 {
		t.Fatalf("out of range width kept: %+v", got)
	}
}
