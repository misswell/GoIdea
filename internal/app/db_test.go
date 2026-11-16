package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func TestUTF16Len(t *testing.T) {
	cases := map[string]uint32{
		"":         0,
		"abc":      3,
		"你好":       2,
		"a😀b":      4,
		"SELECT 1": 8,
	}
	for in, want := range cases {
		if got := utf16Len(in); got != want {
			t.Fatalf("utf16Len(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDBOutcomeSummary(t *testing.T) {
	affected := 3.0
	if got := dbOutcomeSummary(openidea.DBOutcome{Result: &openidea.DBQueryResult{Affected: &affected, ElapsedMs: 1.25}}); got != "3 rows affected · 1.2 ms" {
		t.Fatalf("affected summary = %q", got)
	}
	rows := &openidea.DBQueryResult{Rows: make([][]*string, 2), HasMore: true, ElapsedMs: 0.5}
	if got := dbOutcomeSummary(openidea.DBOutcome{Result: rows}); got != "2 rows · 0.5 ms · more rows" {
		t.Fatalf("rows summary = %q", got)
	}
	if got := dbOutcomeSummary(openidea.DBOutcome{Error: "boom"}); got != "" {
		t.Fatalf("error summary = %q, want empty", got)
	}
}

func TestDBStatusText(t *testing.T) {
	v := &dbView{}
	if got := dbStatusText(v); got != "No data source" {
		t.Fatalf("empty status = %q", got)
	}
	v.source = "a"
	if got := dbStatusText(v); got != "Not connected" {
		t.Fatalf("disconnected status = %q", got)
	}
	v.connected = map[string]bool{"a": true}
	if got := dbStatusText(v); got != "Connected" {
		t.Fatalf("connected status = %q", got)
	}
	v.running = true
	if got := dbStatusText(v); got != "Running…" {
		t.Fatalf("running status = %q", got)
	}
}

func TestCloseDatabaseDropsSheet(t *testing.T) {
	m := &Model{db: &dbView{open: true}}
	m.closeDatabase()
	if m.db.open {
		t.Fatalf("database sheet stayed open")
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("SELECT *\n  FROM t\n WHERE id > 1"); got != "SELECT * FROM t WHERE id > 1" {
		t.Fatalf("oneLine = %q", got)
	}
}
