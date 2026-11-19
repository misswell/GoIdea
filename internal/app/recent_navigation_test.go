package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestRecentFilesShowsPreviousAndEditedFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "Main.java")
	previous := filepath.Join(root, "pom.xml")
	doc := docbridge.Virtual(path, "class Main {}")
	m := &Model{tabs: []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}}}
	m.setRoot(root)
	m.markRecent(previous)
	m.markRecent(path)
	m.tab().state.Insert("// edit\n")

	if !m.RunCommand("navigate.recentFiles") {
		t.Fatal("missing Recent Files action")
	}
	p := m.recentNav.popup
	if len(p.files) != 2 || p.files[p.index] != previous {
		t.Fatalf("expected previous file selected, got %+v", p)
	}
	m.RunCommand("navigate.recentFiles")
	if !p.onlyChanged || len(p.files) != 1 || p.files[0] != path {
		t.Fatalf("edited toggle did not filter files: %+v", p)
	}
	p.query = "main"
	m.refreshRecentNavigation(true)
	if len(p.files) != 1 || p.files[0] != path {
		t.Fatalf("file filter = %v", p.files)
	}
	m.resetProjectState(root)
	if m.recentNav.popup != nil || len(m.recentNav.paths) != 0 {
		t.Fatal("old recent navigation leaked to another project")
	}
}

func TestRecentLocationsTracksUTF16AndChangedSnippets(t *testing.T) {
	m := gotoLineTestModel("a😀b\n"+strings.Repeat("\n", 10)+"return thing;", 5)
	m.setRoot(t.TempDir())
	now := time.Now()
	m.observeRecentLocation(now, false)
	m.observeRecentLocation(now.Add(recentPlaceSettle), false)
	if places := m.recentNav.places; len(places) != 1 || places[0].pos != (editor.Pos{Line: 0, Col: 3}) {
		t.Fatalf("initial positions = %+v", places)
	}

	tab := m.tab()
	tab.state.GoTo(editor.Pos{Line: 11, Col: len("return thing;")})
	tab.state.Insert(" // changed")
	tab.state.GoTo(editor.Pos{Line: 11})
	m.observeRecentLocation(now.Add(time.Second), true)
	tab.seen = tab.doc.Buffer().Version()
	tab.state.GoTo(editor.Pos{Line: 11, Col: 2})
	m.observeRecentLocation(now.Add(2*time.Second), true)
	m.RunCommand("navigate.recentLocations")
	m.RunCommand("navigate.recentLocations")
	p := m.recentNav.popup
	p.query = "return thing"
	m.refreshRecentNavigation(true)
	if len(p.places) != 1 || !p.places[0].changed || p.places[0].pos.Line != 11 {
		t.Fatalf("changed filter = %+v", p.places)
	}
	m.removeRecentPlace()
	if len(p.places) != 0 || len(m.recentNav.places) != 1 {
		t.Fatalf("dismissed wrong locations: %+v", m.recentNav.places)
	}
}

func TestRecentNavigationAndGotoLineRenderOffscreen(t *testing.T) {
	m := gotoLineTestModel("first\nsecond", 0)
	m.setRoot(t.TempDir())
	m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	m.markRecent(m.tab().doc.Path())
	tt := ui.NewTester(m.View, 1440, 900)
	for _, check := range []struct {
		command, text string
		close         func()
	}{
		{"navigate.recentFiles", "Show edited only", m.closeRecentNavigation},
		{"navigate.recentLocations", "Show changed only", m.closeRecentNavigation},
		{"navigate.gotoLine", "Current position 1:1", m.closeGotoLine},
	} {
		m.RunCommand(check.command)
		tt.Frame()
		if !tt.HasText(check.text) {
			t.Errorf("%s did not render %q", check.command, check.text)
		}
		check.close()
		tt.Frame()
	}
}

func TestRecentNavigationKeyboardAndStatusEntry(t *testing.T) {
	m := gotoLineTestModel("first\nsecond\nthird", 0)
	m.setRoot(t.TempDir())
	m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	m.markRecent(filepath.Join(m.Root(), "pom.xml"))
	m.markRecent(m.tab().doc.Path())
	tt := ui.NewTester(m.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("Ln 1, Col 1"); err != nil {
		t.Fatal(err)
	}
	tt.Type("2:3")
	if m.gotoLine == nil || m.gotoLine.query != "2:3" {
		t.Fatal("typing did not replace the selected position")
	}
	tt.Key(0, ui.KeyEnter)
	if m.gotoLine != nil || m.tab().state.CursorPos() != (editor.Pos{Line: 1, Col: 2}) {
		t.Fatal("Enter did not navigate and dismiss")
	}
	tt.Key(ui.Cmd, ui.KeyE)
	if !tt.HasText("Show edited only") {
		t.Fatal("Cmd+E did not open Recent Files")
	}
	tt.Key(ui.Cmd, ui.KeyE)
	if !tt.HasText("Recently Edited Files") {
		t.Fatal("pressing Cmd+E again did not toggle edited files")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	if !tt.HasText("Show changed only") {
		t.Fatal("Cmd+Shift+E did not switch to Recent Locations")
	}
	tt.Key(0, ui.KeyEscape)
	if m.recentNav.popup != nil {
		t.Fatal("Escape did not dismiss Recent Locations")
	}
}
