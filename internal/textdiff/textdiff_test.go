package textdiff

import (
	"strings"
	"testing"
)

func kinds(rows []Row) string {
	var b strings.Builder
	for _, r := range rows {
		switch r.Kind {
		case Hunk:
			b.WriteByte('H')
		case Add:
			b.WriteByte('+')
		case Delete:
			b.WriteByte('-')
		default:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func TestCompareIdentical(t *testing.T) {
	if rows := Compare("a\nb\n", "a\nb", 3); rows != nil {
		t.Fatalf("identical texts produced %v", rows)
	}
}

func TestCompareReplacedLine(t *testing.T) {
	rows := Compare("a\nb\nc\n", "a\nB\nc\n", 3)
	if got := kinds(rows); got != "H -+ " {
		t.Fatalf("kinds = %q in %v", got, rows)
	}
	if rows[0].Text != "@@ -1,3 +1,3 @@" {
		t.Fatalf("header = %q", rows[0].Text)
	}
	if rows[2].Kind != Delete || rows[2].Old != 2 || rows[2].Text != "b" {
		t.Fatalf("delete row = %+v", rows[2])
	}
	if rows[3].Kind != Add || rows[3].New != 2 || rows[3].Text != "B" {
		t.Fatalf("add row = %+v", rows[3])
	}
}

func TestCompareInsertionAtStart(t *testing.T) {
	rows := Compare("", "a\nb\n", 3)
	if got := kinds(rows); got != "H++" {
		t.Fatalf("kinds = %q in %v", got, rows)
	}
	if rows[0].Text != "@@ -0,0 +1,2 @@" {
		t.Fatalf("header = %q", rows[0].Text)
	}
}

func TestCompareInsertionInMiddle(t *testing.T) {
	rows := Compare("a\nb\n", "a\nx\nb\n", 0)
	if got := kinds(rows); got != "H+" {
		t.Fatalf("kinds = %q in %v", got, rows)
	}
	if rows[0].Text != "@@ -1,0 +2 @@" {
		t.Fatalf("header = %q", rows[0].Text)
	}
	if rows[1].Text != "x" || rows[1].New != 2 {
		t.Fatalf("insert row = %+v", rows[1])
	}
}

func TestCompareTrimsDistantContext(t *testing.T) {
	old := "1\n2\n3\n4\n5\n6\n7\n8\n9\n"
	want := "1\n2\n3\n4\nFIVE\n6\n7\n8\n9\n"
	rows := Compare(old, want, 1)
	if got := kinds(rows); got != "H -+ " {
		t.Fatalf("kinds = %q in %v", got, rows)
	}
	if rows[0].Text != "@@ -4,3 +4,3 @@" {
		t.Fatalf("header = %q", rows[0].Text)
	}
}

func TestCompareTwoHunks(t *testing.T) {
	old := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	want := "1\nTWO\n3\n4\n5\n6\n7\n8\n9\nTEN\n"
	rows := Compare(old, want, 0)
	if got := kinds(rows); got != "H-+H-+" {
		t.Fatalf("kinds = %q in %v", got, rows)
	}
	if rows[0].Text != "@@ -2 +2 @@" || rows[3].Text != "@@ -10 +10 @@" {
		t.Fatalf("headers = %q, %q", rows[0].Text, rows[3].Text)
	}
}

func TestCompareDeletion(t *testing.T) {
	rows := Compare("a\nb\nc\n", "a\nc\n", 0)
	if got := kinds(rows); got != "H-" {
		t.Fatalf("kinds = %q in %v", got, rows)
	}
	if rows[0].Text != "@@ -2 +1,0 @@" {
		t.Fatalf("header = %q", rows[0].Text)
	}
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 1},
		{"a\nb\n", 2},
		{"a\n\n", 2},
	}
	for _, c := range cases {
		if got := len(SplitLines(c.text)); got != c.want {
			t.Fatalf("SplitLines(%q) has %d lines, want %d", c.text, got, c.want)
		}
	}
}
