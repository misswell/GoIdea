package app

import "testing"

func TestCountUsesTheSingularForOne(t *testing.T) {
	if got := count(0, "item", "items"); got != "0 items" {
		t.Fatalf("count(0) = %q", got)
	}
	if got := count(1, "item", "items"); got != "1 item" {
		t.Fatalf("count(1) = %q", got)
	}
	if got := count(2, "item", "items"); got != "2 items" {
		t.Fatalf("count(2) = %q", got)
	}
}
