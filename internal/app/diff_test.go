package app

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
	"mayoi/internal/textdiff"
)

func TestShowDiffNeedsAProject(t *testing.T) {
	m := &Model{}
	m.ShowDiff("/tmp/x")
	if m.diff != nil {
		t.Fatalf("compare view opened without a project")
	}
}

func TestCloseDiffDropsView(t *testing.T) {
	m := &Model{diff: &diffView{open: true}}
	m.closeDiff()
	if m.diff != nil {
		t.Fatalf("compare view stayed open")
	}
}

func TestDiffErrorTextNamesMissingGit(t *testing.T) {
	err := &openidea.Error{Kind: "unavailable", Message: "not a git repository"}
	if got := diffErrorText(err); got != "The current project is not a Git repository" {
		t.Fatalf("diffErrorText = %q", got)
	}
	if got := diffErrorText(textdiffError{}); !strings.HasPrefix(got, "Could not read the diff: ") {
		t.Fatalf("diffErrorText = %q", got)
	}
}

type textdiffError struct{}

func (textdiffError) Error() string { return "boom" }

func TestDiffRowText(t *testing.T) {
	row := textdiff.Row{Kind: textdiff.Add, Old: 4, New: 5, Text: "x"}
	if got := diffRowText(row); got != "+ 4:5 x" {
		t.Fatalf("diffRowText = %q", got)
	}
}

func TestDiffRowsCountsBothSides(t *testing.T) {
	rows, added, gone := diffRows("a\nb\n", "a\nc\n")
	if len(rows) == 0 || added != 1 || gone != 1 {
		t.Fatalf("diffRows = %d rows, +%d -%d", len(rows), added, gone)
	}
	rows, added, gone = diffRows("same\n", "same\n")
	if len(rows) != 0 || added != 0 || gone != 0 {
		t.Fatalf("equal texts = %d rows, +%d -%d", len(rows), added, gone)
	}
}

func TestDiffTitleNamesEachCompare(t *testing.T) {
	cases := []struct {
		d    diffView
		want string
	}{
		{diffView{path: "/p/Main.java"}, "Main.java · Compared with HEAD"},
		{diffView{path: "/p/Main.java", other: "/p/Util.java"}, "Main.java ↔ Util.java"},
		{diffView{path: "/p/Main.java", inline: true, label: "Clipboard"}, "Main.java ↔ Clipboard"},
		{diffView{path: "/p/Main.java", loading: true}, "Main.java · Comparing…"},
	}
	for _, c := range cases {
		if got := diffTitle(&c.d); got != c.want {
			t.Fatalf("diffTitle(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestCompareWithoutATabOpensNothing(t *testing.T) {
	m := &Model{}
	m.CompareTabWithClipboard(nil)
	m.PickCompareFile(nil)
	m.ShowTextCompare("", "a", "Clipboard", "b")
	if m.diff != nil {
		t.Fatalf("compare view opened without a tab")
	}
}
