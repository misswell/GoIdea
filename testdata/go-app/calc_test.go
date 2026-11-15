package goapp

import "testing"

func TestAddOk(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("Add(2, 3) = %d, want 5", Add(2, 3))
	}
}

func TestAddNegative(t *testing.T) {
	if Add(-1, -2) != -3 {
		t.Fatalf("Add(-1, -2) = %d, want -3", Add(-1, -2))
	}
}

func TestFailsOnPurpose(t *testing.T) {
	// The rerun-failed regression derives a -run filter from this test's
	// failure and reruns only it.
	if Add(1, 1) != 3 {
		t.Fatalf("expected the fixture failure")
	}
}
