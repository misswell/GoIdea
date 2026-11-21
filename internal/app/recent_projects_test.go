package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecentProjectsPopupExcludesCurrentAndFilters(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(t.TempDir(), "alpha")
	beta := filepath.Join(t.TempDir(), "beta")
	for _, path := range []string{alpha, beta} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := &Model{}
	m.setRoot(root)
	m.recents = normalizeRecentProjects([]RecentProject{
		{Path: alpha, Name: "Alpha", OpenedAt: time.Now().Add(-time.Hour).UnixMilli()},
		{Path: beta, Name: "Beta", OpenedAt: time.Now().UnixMilli()},
		{Path: root, Name: "Current", OpenedAt: time.Now().Add(-2 * time.Hour).UnixMilli()},
	})
	m.showRecentProjectsPopup()
	if got := len(m.recentProjectsMatches()); got != 2 {
		t.Fatalf("recent popup has %d rows, want the current project hidden and 2 visible", got)
	}
	m.recentProjectsPopup.query = "bet"
	matches := m.recentProjectsMatches()
	if len(matches) != 1 || matches[0].project.Path != beta {
		t.Fatalf("query matches = %+v, want Beta", matches)
	}
	m.stepRecentProjects(-1)
	if m.recentProjectsPopup.index != 0 {
		t.Fatalf("wrapped selection = %d, want 0", m.recentProjectsPopup.index)
	}
}

func TestRecentProjectsPopupUsesOpenIDEAOverlayID(t *testing.T) {
	m := &Model{}
	m.showRecentProjectsPopup()
	overlay := overlays.byID["welcome.recentProjects"]
	if overlay == nil || !overlay.Open(m) {
		t.Fatal("File › Open Recent is not registered as welcome.recentProjects")
	}
	m.closeRecentProjectsPopup()
	if overlay.Open(m) {
		t.Fatal("the recent-projects overlay stayed open after closing")
	}
}
