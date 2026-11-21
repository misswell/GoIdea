package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestNavigationBarPlacementAndStatusWidgetPersistence(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "NavigationOnlyFolder", "Main.java")
	tab := &Tab{doc: docbridge.Virtual(path, "class Main {}")}
	tab.state = &editor.State{Buffer: tab.doc.Buffer()}
	m := &Model{
		cfg:    Config{SettingsPath: settings},
		tabs:   []*Tab{tab},
		active: 0,
	}
	m.setRoot(root)
	tt := workbenchTester(t, m)

	if !tt.HasText("NavigationOnlyFolder") {
		t.Fatalf("default status-bar navigation is missing: %q", tt.Texts())
	}
	if !m.RunActionWithArg("view.toggleStatusBarWidget", "status.file") {
		t.Fatal("the Navigation Bar status widget action did not run")
	}
	tt.Frame()
	if tt.HasText("NavigationOnlyFolder") {
		t.Fatalf("the hidden Navigation Bar widget still rendered: %q", tt.Texts())
	}

	if !m.RunCommand("view.navigationBarTop") ||
		m.navigationBarPosition() != "top" ||
		!m.ActionToggled("view.navigationBarTop") {
		t.Fatalf("top placement = %q, toggled=%v", m.navigationBarPosition(), m.ActionToggled("view.navigationBarTop"))
	}
	tt.Frame()
	if !tt.HasText("NavigationOnlyFolder") {
		t.Fatalf("top navigation did not render independently of the status widget: %q", tt.Texts())
	}

	stored := LoadPreferences(settings)
	if stored.Appearance.NavigationBar != "top" ||
		stored.Appearance.StatusBarWidgets["status.file"] {
		t.Fatalf("stored appearance = %+v", stored.Appearance)
	}
	if !m.RunCommand("view.navigationBarHidden") ||
		!m.ActionToggled("view.navigationBarHidden") {
		t.Fatal("Don't Show did not become the selected Navigation Bar placement")
	}
	tt.Frame()
	if tt.HasText("NavigationOnlyFolder") {
		t.Fatalf("the hidden Navigation Bar still rendered: %q", tt.Texts())
	}
}

func TestStatusBarContextMenuTogglesWidgets(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: settings}}
	tt := workbenchTester(t, m)
	if err := tt.RightClick("GoIDEA backend not connected"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	for _, want := range []string{"Navigation Bar", "Backend Status", "Line:Column Number"} {
		found := false
		for _, item := range menu {
			if item == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("status widget menu %v lacks %q", menu, want)
		}
	}
	if err := tt.ChooseMenuItem("Backend Status"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("GoIDEA backend not connected") {
		t.Fatalf("the hidden backend widget still rendered: %q", tt.Texts())
	}
	if visible, ok := m.prefs.Appearance.StatusBarWidgets["status.backend"]; !ok || visible {
		t.Fatalf("stored backend widget override = %v, present=%v", visible, ok)
	}
	if m.toggleStatusBarWidget("missing") {
		t.Fatal("an unknown status widget was toggled")
	}
}

func TestCompactModeChangesChromeAndKeepsCompactHeaderWithToolbarOff(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	root := t.TempDir()
	m := &Model{cfg: Config{SettingsPath: settings}}
	m.prefs.MainToolbar = &toolbarLayout{Right: []string{toolbarActionPrefix + "view.services"}}
	m.setRoot(root)
	tt := workbenchTester(t, m)
	normal, ok := tt.Find("GoIDEA backend not connected")
	if !ok {
		t.Fatalf("backend status is missing: %q", tt.Texts())
	}
	if !m.RunCommand("view.toggleCompactMode") || !m.ActionToggled("view.toggleCompactMode") {
		t.Fatal("Compact Mode did not turn on")
	}
	tt.Frame()
	compact, ok := tt.Find("GoIDEA backend not connected")
	if !ok || compact.Y <= normal.Y {
		t.Fatalf("compact status bar did not move down: normal=%+v compact=%+v present=%v", normal, compact, ok)
	}

	if !m.RunCommand("view.toggleToolbar") {
		t.Fatal("Toolbar did not turn off")
	}
	tt.Frame()
	if !tt.HasText(filepath.Base(root)) {
		t.Fatalf("the compact title bar did not keep the document title: %q", tt.Texts())
	}
	if tt.HasText("Services") {
		t.Fatalf("the toolbar widgets survived Toolbar off: %q", tt.Texts())
	}
	stored := LoadPreferences(settings)
	if !stored.Appearance.CompactMode || !stored.Appearance.HideMainToolbar {
		t.Fatalf("stored compact/toolbar = %+v", stored.Appearance)
	}
}

func TestBackgroundTasksOpenEmptyStopAllAndToolWindowCloseContent(t *testing.T) {
	m := &Model{}
	m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	tt := ui.NewTester(func(c *ui.Context) {
		m.workbench(c, c.Theme())
		m.renderOverlays(c, c.Theme())
	}, 1200, 800)
	tt.SetDark(true)
	tt.SetScale(1)
	tt.Frame()

	if !m.RunAction("view.backgroundTasks") || !m.tasksOpen {
		t.Fatal("Background Tasks did not open without a running task")
	}
	tt.Frame()
	if !tt.HasText("No background tasks") {
		t.Fatalf("empty Background Tasks popup is missing: %q", tt.Texts())
	}
	m.closeBackgroundTasks()
	tt.Frame()

	cancelled := 0
	stopped := 0
	m.tasks = []BackgroundTask{{
		ID: "single", Title: "Build", Fraction: -1,
		Cancel: func() { stopped++ },
	}}
	if !m.RunAction("workbench.stopBackgroundProcesses") || stopped != 1 || m.tasksOpen {
		t.Fatalf("one background process was not stopped directly: stopped=%d open=%v", stopped, m.tasksOpen)
	}

	m.tasks = []BackgroundTask{
		{ID: "one", Title: "Download one", Fraction: 0.5, Cancel: func() { cancelled++ }},
		{ID: "two", Title: "Download two", Fraction: -1, Cancel: func() { cancelled++ }},
	}
	if !m.RunAction("workbench.stopBackgroundProcesses") ||
		!m.tasksOpen || !m.tasksStopMode {
		t.Fatalf("Stop Background Processes did not open the chooser: open=%v stop=%v", m.tasksOpen, m.tasksStopMode)
	}
	tt.Frame()
	if err := tt.Click("Stop All"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cancelled != 2 || !m.tasks[0].Cancelling || !m.tasks[1].Cancelling {
		t.Fatalf("Stop All cancelled=%d tasks=%+v", cancelled, m.tasks)
	}

	m.tasksOpen = false
	m.tasksStopMode = false
	m.term.visible = true
	m.tool = "term"
	m.dock.slots[dockBottom] = "term"
	m.dock.active = "term"
	m.term.groups = []termGroup{{tabs: []termSession{{shell: "zsh"}, {shell: "bash"}}, active: 0}}
	if !m.RunAction("toolwindow.closeContent") {
		t.Fatal("Close Tool Window Tab did not run in a terminal")
	}
	if m.term.total() != 1 || !m.term.visible {
		t.Fatalf("closing one terminal content tab: total=%d visible=%v", m.term.total(), m.term.visible)
	}
	if !m.RunAction("toolwindow.closeContent") {
		t.Fatal("Close Tool Window Tab did not run for the last terminal tab")
	}
	if m.term.total() != 0 || m.term.visible || m.dockWindowShown("term") {
		t.Fatalf("closing the last terminal content tab: total=%d visible=%v dock=%v",
			m.term.total(), m.term.visible, m.dockWindowShown("term"))
	}
}

func TestOpenIDEAShellShortcuts(t *testing.T) {
	m := &Model{}
	checks := map[string][]string{
		"navigate.navigationBar":            {"Cmd+Up", "Alt+Home"},
		"workbench.stopBackgroundProcesses": {"Cmd+Shift+F2"},
		"toolwindow.closeContent":           {"Cmd+W", "Ctrl+Shift+F4"},
	}
	for id, want := range checks {
		got := m.KeymapBindings(id)
		if len(got) != len(want) {
			t.Fatalf("%s bindings = %v, want %v", id, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s bindings = %v, want %v", id, got, want)
			}
		}
	}
}
