package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func TestProjectWindowWorkspaceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	for _, root := range []string{first, second} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	want := ProjectWindowWorkspace{
		Active: "window-2",
		Windows: []ProjectWindowRecord{
			{ID: "main", Root: first, Group: 4},
			{ID: "welcome"},
			{ID: "window-2", Root: second, Group: 7},
		},
	}
	path := ProjectWindowWorkspacePath(filepath.Join(dir, "session.json"))
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got := LoadProjectWindowWorkspace(path)
	if got.Version != 1 {
		t.Fatalf("version = %d, want 1", got.Version)
	}
	if !reflect.DeepEqual(got.Windows, want.Windows) {
		t.Fatalf("windows = %+v, want %+v", got.Windows, want.Windows)
	}
	if got.Active != want.Active {
		t.Fatalf("active = %q, want %q", got.Active, want.Active)
	}
}

func TestProjectWindowWorkspaceFiltersAndRepairsIdentity(t *testing.T) {
	root := t.TempDir()
	got := normalizeProjectWindowWorkspace(ProjectWindowWorkspace{
		Active: "missing",
		Windows: []ProjectWindowRecord{
			{ID: "", Root: root},
			{ID: "window-2", Root: filepath.Join(root, ".", "child", ".."), Group: 3},
			{ID: "main", Root: root},
			{ID: "window-2", Root: root, Group: 9},
		},
	})
	want := []ProjectWindowRecord{
		{ID: "main", Root: root},
		{ID: "window-2", Root: root, Group: 3},
	}
	if !reflect.DeepEqual(got.Windows, want) {
		t.Fatalf("windows = %+v, want %+v", got.Windows, want)
	}
	if got.Active != "main" {
		t.Fatalf("active = %q, want main", got.Active)
	}

	got = normalizeProjectWindowWorkspace(ProjectWindowWorkspace{
		Active:  "welcome",
		Windows: []ProjectWindowRecord{{ID: "welcome"}},
	})
	if len(got.Windows) != 1 || got.Windows[0].Root != "" || got.Active != "welcome" {
		t.Fatalf("empty Welcome window = %+v active %q", got.Windows, got.Active)
	}
}

func TestProjectWindowWorkspaceIgnoresMissingOrDamagedFile(t *testing.T) {
	dir := t.TempDir()
	if got := LoadProjectWindowWorkspace(filepath.Join(dir, "missing.json")); !reflect.DeepEqual(got, ProjectWindowWorkspace{}) {
		t.Fatalf("missing file = %+v", got)
	}
	path := filepath.Join(dir, "windows.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadProjectWindowWorkspace(path); !reflect.DeepEqual(got, ProjectWindowWorkspace{}) {
		t.Fatalf("damaged file = %+v", got)
	}
}

func TestProjectWindowWorkspacePreservesGroups(t *testing.T) {
	got := normalizeProjectWindowWorkspace(ProjectWindowWorkspace{
		Active: "b",
		Windows: []ProjectWindowRecord{
			{ID: "main", Root: "/main"},
			{ID: "a", Root: "/a", Group: 12},
			{ID: "b", Root: "/b", Group: 12},
			{ID: "c", Root: "/c", Group: 13},
		},
	})
	if got.Active != "b" {
		t.Fatalf("active = %q, want b", got.Active)
	}
	wantGroups := map[string]int{"main": 0, "a": 12, "b": 12, "c": 13}
	for _, window := range got.Windows {
		if window.Group != wantGroups[window.ID] {
			t.Fatalf("group for %q = %d, want %d", window.ID, window.Group, wantGroups[window.ID])
		}
	}
}

func rollbackTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	root := t.TempDir()
	main := docbridge.Virtual(filepath.Join(root, "Main.java"), "class Main {}\n")
	split := docbridge.Virtual(filepath.Join(root, "Greeter.java"), "class Greeter {}\n")
	m := &Model{
		cfg:      Config{SessionPath: filepath.Join(t.TempDir(), "session.json")},
		postHook: func(fn func()) { fn() },
		tabs: []*Tab{{
			doc:   main,
			state: &editor.State{Buffer: main.Buffer()},
		}},
		active:    0,
		splitDown: true,
		split: &splitGroup{
			tabs: []*Tab{{
				doc:   split,
				state: &editor.State{Buffer: split.Buffer()},
			}},
			active: 0,
		},
		recents: []RecentProject{
			{Path: root, Name: "project", Group: "Work", Branch: "main"},
			{Path: filepath.Join(filepath.Dir(root), "other"), Name: "other"},
		},
	}
	m.setRoot(root)
	return m, root
}

func TestRollbackSnapshotCapturesTheLifecycleState(t *testing.T) {
	m, root := rollbackTestModel(t)
	got := m.rollbackSnapshot()
	if got.root != realPath(root) {
		t.Fatalf("root = %q, want %q", got.root, realPath(root))
	}
	if got.tabs != 1 || got.active != realPath(filepath.Join(root, "Main.java")) {
		t.Fatalf("focused group = tabs %d active %q", got.tabs, got.active)
	}
	if !got.split || got.splitActive != 0 ||
		!got.splitDown || got.splitDoc != realPath(filepath.Join(root, "Greeter.java")) {
		t.Fatalf("split = present %v down %v active %d document %q",
			got.split, got.splitDown, got.splitActive, got.splitDoc)
	}
	if len(got.recents) != 2 || !sameStringOrder(got.recentPaths, []string{
		root,
		filepath.Join(filepath.Dir(root), "other"),
	}) {
		t.Fatalf("recents = %+v paths %v", got.recents, got.recentPaths)
	}
}

func TestRollbackComparisonPreservesIdentityAndOrder(t *testing.T) {
	m, root := rollbackTestModel(t)
	before := m.rollbackSnapshot()
	if !sameProjectRoot(before.root, realPath(m.Root())) {
		t.Fatal("the same project root did not compare equal")
	}
	if !sameRecentProjects(before.recents, m.recents) {
		t.Fatal("the unchanged recents did not compare equal")
	}
	if !sameStringOrder(before.recentPaths, recentPaths(m.recents)) {
		t.Fatal("the unchanged recent paths did not compare equal")
	}

	reordered := append([]RecentProject(nil), m.recents...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	if sameRecentProjects(before.recents, reordered) ||
		sameStringOrder(before.recentPaths, recentPaths(reordered)) {
		t.Fatal("a reordered recent list compared equal")
	}
	changed := append([]RecentProject(nil), m.recents...)
	changed[0].Group = "Personal"
	if sameRecentProjects(before.recents, changed) {
		t.Fatal("changed recent metadata compared equal")
	}
	if sameProjectRoot(root, filepath.Join(root, "nested")) {
		t.Fatal("a different project root compared equal")
	}
	if !sameProjectRoot(root+string(filepath.Separator), root) {
		t.Fatal("a clean path for the same project compared different")
	}
}

func TestRollbackSnapshotPreservesAnEmptyWelcomeWindow(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	got := m.rollbackSnapshot()
	if got.root != "" || got.tabs != 0 || got.active != "" || got.split || got.document != "" {
		t.Fatalf("empty Welcome snapshot = %+v", got)
	}
	if len(got.recents) != 0 || len(got.recentPaths) != 0 {
		t.Fatalf("empty Welcome recents = %+v paths %v", got.recents, got.recentPaths)
	}
}
