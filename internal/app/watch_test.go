package app

import (
	"testing"
	"time"
)

func TestStampChanged(t *testing.T) {
	base := time.Unix(100, 0)
	later := time.Unix(101, 0)
	cases := []struct {
		name             string
		before, now      time.Time
		beforeSize, size int64
		want             bool
	}{
		{"unchanged", base, base, 10, 10, false},
		{"newer mtime", base, later, 10, 10, true},
		{"new size", base, base, 10, 11, true},
		{"unreadable before", time.Time{}, later, 0, 5, false},
		{"unreadable now", base, time.Time{}, 10, 0, false},
	}
	for _, c := range cases {
		if got := stampChanged(c.before, c.beforeSize, c.now, c.size); got != c.want {
			t.Errorf("%s: stampChanged = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWatchFilesPacesChecks(t *testing.T) {
	m := &Model{}
	now := time.Unix(1000, 0)
	m.watchFiles(now)
	if !m.watched.Equal(now) {
		t.Fatalf("watched = %v, want %v", m.watched, now)
	}
	m.watchFiles(now.Add(watchInterval / 2))
	if !m.watched.Equal(now) {
		t.Fatalf("a check ran inside the interval: watched = %v", m.watched)
	}
	m.watchFiles(now.Add(watchInterval))
	if !m.watched.Equal(now.Add(watchInterval)) {
		t.Fatalf("watched = %v after the interval", m.watched)
	}
}
