package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func outlineAt(name, kind string, line uint32, startLine, endLine uint32, children ...openidea.OutlineItem) openidea.OutlineItem {
	return openidea.OutlineItem{
		Name:           name,
		Kind:           kind,
		Range:          openidea.OutlineRange{Start: openidea.OutlinePos{Line: startLine}, End: openidea.OutlinePos{Line: endLine}},
		SelectionRange: openidea.OutlineRange{Start: openidea.OutlinePos{Line: line, Col: 4}, End: openidea.OutlinePos{Line: line, Col: uint32(4 + len(name))}},
		Children:       children,
	}
}

func TestVisibleCodeVisionTargetsFiltersSortsAndBounds(t *testing.T) {
	items := []openidea.OutlineItem{
		outlineAt("Outside", "class", 30, 30, 35),
		{
			Name: "Demo", Kind: "class",
			Range:          openidea.OutlineRange{Start: openidea.OutlinePos{Line: 10}, End: openidea.OutlinePos{Line: 24}},
			SelectionRange: openidea.OutlineRange{Start: openidea.OutlinePos{Line: 10, Col: 6}, End: openidea.OutlinePos{Line: 10, Col: 10}},
			Children: []openidea.OutlineItem{
				outlineAt("run", "method", 14, 14, 16),
				outlineAt("<lambda>", "function", 15, 15, 15),
				outlineAt("map() callback", "function", 16, 16, 16),
				outlineAt("count", "field", 17, 17, 17),
			},
		},
	}
	got := visibleCodeVisionTargets(items, 12, 18)
	if len(got) != 2 || got[0].name != "Demo" || got[1].name != "run" {
		t.Fatalf("targets = %+v, want Demo and run", got)
	}
	if got[0].start != 10 || got[0].end != 24 || got[1].position.Line != 14 {
		t.Fatalf("target ranges = %+v", got)
	}

	var many []openidea.OutlineItem
	for line := uint32(0); line < 60; line++ {
		many = append(many, outlineAt("D"+string(rune('A'+line)), "function", line, line, line))
	}
	if got := visibleCodeVisionTargets(many, 0, 59); len(got) != 40 {
		t.Fatalf("bounded targets = %d, want OpenIDEA's limit of 40", len(got))
	}
}

func TestCodeVisionLabelsAndAuthorSummary(t *testing.T) {
	labels := []string{codeVisionUsage(0), codeVisionUsage(1), codeVisionUsage(3)}
	want := []string{"no usages", "1 usage", "3 usages"}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("usage label %d = %q, want %q", i, labels[i], want[i])
		}
	}
	when := time.Date(2024, time.January, 2, 12, 0, 0, 0, time.UTC).UnixMilli()
	blame := []openidea.BlameLine{
		{Commit: "aaaaaaaa", Author: "Ann", TimeMs: 100},
		{Commit: "bbbbbbbb", Author: "Thomas", TimeMs: float64(when)},
		{Commit: "cccccccc", Author: "Bob", TimeMs: 200},
		{Commit: "00000000", Author: "Not Committed Yet", TimeMs: 999999},
	}
	datePrefs := DateFormatPrefs{OverrideSystem: true, Pattern: "MMM d"}
	if got := codeVisionAuthor(blame, 0, 3, datePrefs, dateRegion{}); got != "Thomas +2 Jan 2" {
		t.Fatalf("author summary = %q", got)
	}
}

func TestCodeVisionInlaysRespectPositionsAndNoUsages(t *testing.T) {
	doc := docbridge.Virtual("/demo.java", "😀class Demo {\n}\n")
	tab := &Tab{doc: doc}
	target := codeVisionTarget{
		name:     "Demo",
		position: openidea.OutlinePos{Line: 0, Col: 8},
		start:    0,
		end:      0,
	}
	when := time.Date(2024, time.January, 2, 12, 0, 0, 0, time.UTC).UnixMilli()
	counts := map[string]int{codeVisionTargetKey(target): 2}
	blame := []openidea.BlameLine{{Commit: "aaaaaaaa", Author: "Ann", TimeMs: float64(when)}}
	datePrefs := DateFormatPrefs{OverrideSystem: true, Pattern: "MMM d"}

	got := codeVisionInlays(tab, []codeVisionTarget{target}, counts, blame, InlayPrefs{}, datePrefs, dateRegion{})
	if len(got) != 1 || got[0].Offset != len("😀class ") || got[0].Text != "Ann Jan 2 · 2 usages  " {
		t.Fatalf("default inlay = %+v", got)
	}

	right := InlayPrefs{UsagesPosition: "right", AuthorPosition: "right"}
	got = codeVisionInlays(tab, []codeVisionTarget{target}, counts, blame, right, datePrefs, dateRegion{})
	if len(got) != 1 || got[0].Offset != len("😀class Demo {") ||
		got[0].Text != "  Ann Jan 2 · 2 usages  " {
		t.Fatalf("right inlay = %+v", got)
	}

	split := InlayPrefs{UsagesPosition: "right"}
	got = codeVisionInlays(tab, []codeVisionTarget{target}, counts, blame, split, datePrefs, dateRegion{})
	if len(got) != 2 || got[0].Text != "Ann Jan 2  " ||
		got[1].Offset != len("😀class Demo {") || got[1].Text != "  2 usages  " {
		t.Fatalf("split inlays = %+v", got)
	}

	noUsage := map[string]int{codeVisionTargetKey(target): 0}
	got = codeVisionInlays(tab, []codeVisionTarget{target}, noUsage, blame,
		InlayPrefs{NoCodeVisionAuthor: true}, datePrefs, dateRegion{})
	if len(got) != 1 || got[0].Text != "no usages  " {
		t.Fatalf("no usages inlay = %+v", got)
	}
	got = codeVisionInlays(tab, []codeVisionTarget{target}, noUsage, blame,
		InlayPrefs{NoCodeVisionAuthor: true, HideCodeVisionNoUses: true}, datePrefs, dateRegion{})
	if len(got) != 0 {
		t.Fatalf("hidden no-usages inlay = %+v", got)
	}
}

func TestInlayAndCodeVisionResultsMergeByOffset(t *testing.T) {
	state := &inlayState{
		hints: []editor.Inlay{{Offset: 20, Text: "hint"}},
		vision: []editor.Inlay{
			{Offset: 30, Text: "2 usages"},
			{Offset: 10, Text: "Ann"},
		},
		debug: []editor.Inlay{{Offset: 25, Text: "count: 42"}},
	}
	got := mergedEditorInlays(state)
	if len(got) != 4 || got[0].Text != "Ann" || got[1].Text != "hint" ||
		got[2].Text != "count: 42" || got[3].Text != "2 usages" {
		t.Fatalf("merged inlays = %+v", got)
	}
}
