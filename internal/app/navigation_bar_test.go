package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func navigationBarTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(src, "Main.java")
	if err := os.WriteFile(file, []byte("class Main {\n  void run() {}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tab := &Tab{doc: docbridge.Virtual(file, "class Main {\n  void run() {}\n}\n")}
	tab.state = &editor.State{Buffer: tab.doc.Buffer()}
	tab.outline = []openidea.OutlineItem{{
		Name: "Main",
		Kind: "class",
		Children: []openidea.OutlineItem{{
			Name: "run", Kind: "method",
		}},
	}}
	m := &Model{
		tabs:   []*Tab{tab},
		active: 0,
		mgr:    openidea.NewManager(openidea.LaunchConfig{}),
	}
	m.setRoot(root)
	return m, root
}

func TestNavigationDirectoryChildrenSkipsHiddenAndOrdersFoldersFirst(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z.txt", ".git", "aDir", "bDir"} {
		path := filepath.Join(root, name)
		if name == "aDir" || name == "bDir" {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	children, err := navigationDirectoryChildren(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, child := range children {
		got = append(got, child.Label)
	}
	want := []string{"aDir", "bDir", "z.txt"}
	if len(got) != len(want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children = %v, want %v", got, want)
		}
	}
}

func TestNavigationBarKeyboardTraversalAndChildList(t *testing.T) {
	m, _ := navigationBarTestModel(t)
	m.ShowNavigationBar()
	if !m.navigationOpen || len(m.navigation.crumbs) < 3 {
		t.Fatalf("navigation bar did not open with its segment snapshot: %+v", m.navigation)
	}
	file := navigationFileIndex(m.navigation.crumbs)
	if m.navigation.index != file {
		t.Fatalf("initial segment = %d, want file segment %d", m.navigation.index, file)
	}

	tt := ui.NewTester(m.View, 900, 700)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()

	tt.Key(0, ui.KeyLeft)
	if m.navigation.index != file-1 {
		t.Fatalf("left moved to %d, want %d", m.navigation.index, file-1)
	}
	for range file + 4 {
		tt.Key(0, ui.KeyLeft)
	}
	if m.navigation.index != 0 {
		t.Fatalf("left did not clamp at the first segment: %d", m.navigation.index)
	}

	tt.Key(0, ui.KeyDown)
	if !m.navigation.listOpen || len(m.navigation.children) == 0 ||
		m.navigation.children[m.navigation.sel].Label != "src" {
		t.Fatalf("root child list = %+v", m.navigation)
	}
	tt.Key(0, ui.KeyUp)
	if m.navigation.sel != 0 {
		t.Fatalf("up did not clamp at the first child: %d", m.navigation.sel)
	}

	tt.Key(0, ui.KeyEnter)
	if !m.navigation.listOpen || m.navigation.index != 1 ||
		len(m.navigation.crumbs) != 2 || m.navigation.crumbs[1].Label != "src" {
		t.Fatalf("folder selection = %+v", m.navigation)
	}
	tt.Key(0, ui.KeyEscape)
	if !m.navigationOpen || m.navigation.listOpen {
		t.Fatalf("first Escape should close only the child list: %+v", m.navigation)
	}
	tt.Key(0, ui.KeyEscape)
	if m.navigationOpen {
		t.Fatal("second Escape did not close the navigation bar")
	}
}

func TestStepNavigationSegmentClamps(t *testing.T) {
	if got := stepNavigationSegment(0, 3, -1); got != 0 {
		t.Fatalf("step from the start = %d", got)
	}
	if got := stepNavigationSegment(1, 3, 1); got != 2 {
		t.Fatalf("step in the middle = %d", got)
	}
	if got := stepNavigationSegment(2, 3, 1); got != 2 {
		t.Fatalf("step from the end = %d", got)
	}
}
