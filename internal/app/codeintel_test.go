package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"goidea/internal/openidea"
)

func TestHunkMarks(t *testing.T) {
	hunks := []openidea.LineHunk{
		{Kind: "added", Start: 2, Lines: 2, OldStart: 2, OldLines: 0},
		{Kind: "modified", Start: 7, Lines: 1, OldStart: 6, OldLines: 1},
		{Kind: "deleted", Start: 10, Lines: 0, OldStart: 9, OldLines: 2},
	}
	marks := hunkMarks(hunks)
	added, modified, deleted := gutterAddedColor, gutterModifiedColor, gutterDeletedColor
	want := []struct {
		line  int
		color ui.Color
	}{
		{2, added}, {3, added}, {7, modified}, {10, deleted},
	}
	if len(marks) != len(want) {
		t.Fatalf("marks = %+v, want %d entries", marks, len(want))
	}
	for i, w := range want {
		if marks[i].Line != w.line {
			t.Errorf("mark %d line = %d, want %d", i, marks[i].Line, w.line)
		}
		if marks[i].Color != w.color {
			t.Errorf("mark %d color = %+v, want %+v", i, marks[i].Color, w.color)
		}
	}
}

func TestOutlinePath(t *testing.T) {
	item := func(name string, start, end uint32, children ...openidea.OutlineItem) openidea.OutlineItem {
		return openidea.OutlineItem{
			Name: name,
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: start},
				End:   openidea.OutlinePos{Line: end},
			},
			Children: children,
		}
	}
	items := []openidea.OutlineItem{
		item("Hello", 2, 20,
			item("main", 4, 8),
			item("helper", 10, 14),
		),
	}
	names := func(chain []openidea.OutlineItem) []string {
		out := make([]string, len(chain))
		for i, it := range chain {
			out[i] = it.Name
		}
		return out
	}
	if got := names(outlinePath(items, 5)); len(got) != 2 || got[0] != "Hello" || got[1] != "main" {
		t.Errorf("path at 5 = %v, want [Hello main]", got)
	}
	if got := names(outlinePath(items, 12)); len(got) != 2 || got[1] != "helper" {
		t.Errorf("path at 12 = %v, want [Hello helper]", got)
	}
	if got := names(outlinePath(items, 30)); len(got) != 0 {
		t.Errorf("path outside the outline = %v, want none", got)
	}
}
