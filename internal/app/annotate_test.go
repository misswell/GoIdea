package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func blame(commit, author string, ms float64, summary string) openidea.BlameLine {
	return openidea.BlameLine{Commit: commit, Author: author, TimeMs: ms, Summary: summary}
}

func TestAnnotateRowsGroupConsecutiveBlame(t *testing.T) {
	lines := []openidea.BlameLine{
		blame("aaa", "Alice", 1000, "First"),
		blame("aaa", "Alice", 1000, "First"),
		blame("bbb", "Bob", 2000, "Second"),
		blame("aaa", "Alice", 1000, "First"),
	}
	rows := annotateRows(lines)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].From != 1 || rows[0].To != 2 || rows[0].Commit != "aaa" {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].From != 3 || rows[1].To != 3 || rows[1].Commit != "bbb" {
		t.Fatalf("second row = %+v", rows[1])
	}
	if rows[2].From != 4 || rows[2].To != 4 || rows[2].Commit != "aaa" {
		t.Fatalf("third row = %+v", rows[2])
	}
}

func TestAnnotateRowsKeepSameCommitApartForDifferentAuthors(t *testing.T) {
	rows := annotateRows([]openidea.BlameLine{
		blame("aaa", "Alice", 1000, "First"),
		blame("aaa", "Bob", 1000, "First"),
	})
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
}

func TestAnnotateRowsEmpty(t *testing.T) {
	if rows := annotateRows(nil); len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

func TestAnnotateLinesRanges(t *testing.T) {
	if got := annotateLines(annotateRow{From: 12, To: 12}); got != "12" {
		t.Fatalf("single line = %q", got)
	}
	if got := annotateLines(annotateRow{From: 12, To: 18}); got != "12–18" {
		t.Fatalf("range = %q", got)
	}
}

func TestAnnotateTimeDateAndUncommitted(t *testing.T) {
	if got := annotateTime(0); got != "" {
		t.Fatalf("uncommitted date = %q, want empty", got)
	}
	got := annotateTime(1_700_000_000_000)
	if len(got) != 10 || got[4] != '-' || got[7] != '-' {
		t.Fatalf("date = %q, want yyyy-mm-dd", got)
	}
}
