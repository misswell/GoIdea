package app

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestDBResultTabsCycleCloseAndCloseOthers(t *testing.T) {
	m := &Model{db: &dbView{}}
	v := m.db
	v.addDBResult("two", nil, "")
	v.addDBResult("one", nil, "boom")
	if len(v.results) != 2 || v.results[0].title != "one" || v.results[1].title != "two" {
		t.Fatalf("results = %+v", v.results)
	}
	if v.active != 0 {
		t.Fatalf("the new result is not selected: %d", v.active)
	}
	m.selectDBResult(1)
	if v.active != 1 || v.err != "" {
		t.Fatalf("selecting the older tab: active=%d err=%q", v.active, v.err)
	}
	m.selectDBResult(0)
	if v.err != "boom" {
		t.Fatalf("the error did not follow its tab: %q", v.err)
	}
	m.StepDBResult(1) // to the older result
	if v.active != 1 {
		t.Fatalf("Next Result did not reach the older tab: %d", v.active)
	}
	m.StepDBResult(1) // to Output
	if v.active != -1 || len(v.outcomes) != 0 {
		t.Fatalf("Next Result did not reach Output: %d", v.active)
	}
	m.StepDBResult(1)
	if v.active != 0 {
		t.Fatalf("Next Result did not wrap to the newest result: %d", v.active)
	}
	m.CloseDBResult(0)
	if len(v.results) != 1 || v.results[0].title != "two" || v.active != 0 {
		t.Fatalf("close: results=%+v active=%d", v.results, v.active)
	}
	v.addDBResult("three", nil, "")
	m.CloseOtherDBResults(0)
	if len(v.results) != 1 || v.results[0].title != "three" {
		t.Fatalf("close others: %+v", v.results)
	}
	m.CloseDBResult(-1) // Output is not closable

	// A tables reply for the source the sheet left must not touch the new one.
	v.source = "b"
	v.schema = "b.schema"
	v.tables = []openidea.DBTable{{Name: "kept"}}
	m.loadDBTables("a", "a.schema")
	if len(v.tables) != 1 || v.tables[0].Name != "kept" || v.schema != "b.schema" {
		t.Fatalf("stale tables reply changed the sheet: %+v %q", v.tables, v.schema)
	}
	if len(v.results) != 1 {
		t.Fatal("closing Output removed a result")
	}
}

func TestDBTableRange(t *testing.T) {
	if got := dbTableRange(nil); got != "没有行" {
		t.Fatalf("nil sheet = %q", got)
	}
	s := &dbTableSheet{offset: 100}
	if got := dbTableRange(s); got != "没有行" {
		t.Fatalf("empty page = %q", got)
	}
	s.page.Result.Rows = [][]*string{nil, nil, nil}
	if got := dbTableRange(s); got != "101–103 行" {
		t.Fatalf("range = %q", got)
	}
}

func TestDBPlanRows(t *testing.T) {
	p := func(i uint32) *uint32 { return &i }
	nodes := []openidea.DBPlanNode{
		{Operation: "root"},
		{Parent: p(0), Operation: "child-a"},
		{Parent: p(0), Operation: "child-b"},
		{Parent: p(1), Operation: "grandchild"},
		{Operation: "root-2"},
	}
	rows := dbPlanRows(nodes)
	want := []dbPlanRow{{0, 0}, {1, 1}, {3, 2}, {2, 1}, {4, 0}}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestDBHistoryLabel(t *testing.T) {
	if got := dbHistoryLabel(openidea.DBHistoryEntry{Text: "  SELECT 1;\nSELECT 2;"}); got != "SELECT 1;" {
		t.Fatalf("label = %q", got)
	}
	long := strings.Repeat("x", 120)
	if got := dbHistoryLabel(openidea.DBHistoryEntry{Text: long}); got != long[:90]+"…" {
		t.Fatalf("long label = %q", got)
	}
}
