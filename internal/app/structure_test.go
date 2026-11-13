package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func outlineItem(name, kind string, line uint32, kids ...openidea.OutlineItem) openidea.OutlineItem {
	r := openidea.OutlineRange{Start: openidea.OutlinePos{Line: line}, End: openidea.OutlinePos{Line: line}}
	return openidea.OutlineItem{Name: name, Kind: kind, Range: r, SelectionRange: r, Children: kids}
}

func structureOutline() []openidea.OutlineItem {
	return []openidea.OutlineItem{
		outlineItem("Greeter", "class", 0,
			outlineItem("hello", "method", 2),
			outlineItem("name", "field", 5),
		),
		outlineItem("Runner", "interface", 9,
			outlineItem("run", "method", 10),
		),
	}
}

func rowNames(rows []StructureRow) []string {
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
	}
	return names
}

func TestStructureRowsWalkTheOutlineInOrder(t *testing.T) {
	rows := structureRows(structureOutline(), "", nil, 0, "")
	got := rowNames(rows)
	want := []string{"Greeter", "hello", "name", "Runner", "run"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
	if rows[1].Depth != 1 || rows[1].Line != 2 || rows[1].Kind != "method" {
		t.Fatalf("child row = %+v", rows[1])
	}
}

func TestStructureFilterKeepsMatchingAncestors(t *testing.T) {
	rows := structureRows(structureOutline(), "RUN", nil, 0, "")
	got := rowNames(rows)
	if len(got) != 2 || got[0] != "Runner" || got[1] != "run" {
		t.Fatalf("rows = %v, want the Runner container and run", got)
	}
	if rows[0].Container || rows[1].Container {
		t.Fatalf("a matching symbol was marked as a container: %+v", rows)
	}

	rows = structureRows(structureOutline(), "hello", nil, 0, "")
	got = rowNames(rows)
	if len(got) != 2 || got[0] != "Greeter" || got[1] != "hello" {
		t.Fatalf("rows = %v, want Greeter and hello", got)
	}
	if !rows[0].Container {
		t.Fatal("Greeter matched nothing but was not marked a container")
	}
}

func TestStructureCollapseHidesTheDescendants(t *testing.T) {
	collapsed := map[string]bool{"/Greeter@0": true}
	rows := structureRows(structureOutline(), "", collapsed, 0, "")
	got := rowNames(rows)
	if len(got) != 3 || got[0] != "Greeter" || got[1] != "Runner" || got[2] != "run" {
		t.Fatalf("rows = %v, want Greeter collapsed, Runner and run", got)
	}
	if !rows[0].Collapsed {
		t.Fatal("Greeter was not reported collapsed")
	}

	// A filter expands a collapsed symbol so the match stays visible.
	rows = structureRows(structureOutline(), "hello", collapsed, 0, "")
	if len(rows) != 2 {
		t.Fatalf("filtered rows = %v, want Greeter and hello", rowNames(rows))
	}
}

func TestToggleStructureClosesBackToTheTree(t *testing.T) {
	m := &Model{}
	m.ToggleStructure()
	if !m.structure.visible || m.sideMode != "structure" {
		t.Fatalf("first Cmd+7: visible=%v side=%q", m.structure.visible, m.sideMode)
	}
	m.ToggleStructure()
	if m.structure.visible || m.sideMode != "" {
		t.Fatalf("second Cmd+7: visible=%v side=%q", m.structure.visible, m.sideMode)
	}
}
