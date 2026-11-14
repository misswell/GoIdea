package app

import "testing"

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
	if len(v.results) != 1 {
		t.Fatal("closing Output removed a result")
	}
}
