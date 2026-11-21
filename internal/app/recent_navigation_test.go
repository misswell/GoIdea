package app

import (
	"context"
	"path/filepath"
	"slices"
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

func TestRecentFilesDeleteForgetsTheSelectedFile(t *testing.T) {
	root := t.TempDir()
	previous := filepath.Join(root, "pom.xml")
	current := filepath.Join(root, "src", "Main.java")
	doc := docbridge.Virtual(current, "class Main {}")
	m := &Model{tabs: []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}}}
	m.setRoot(root)
	m.markRecent(previous)
	m.markRecent(current)

	m.RunCommand("navigate.recentFiles")
	p := m.recentNav.popup
	if p == nil {
		t.Fatalf("initial Recent Files = %+v", p)
	}
	p.index = slices.Index(p.files, previous)
	if p.index < 0 {
		t.Fatalf("previous file missing from Recent Files: %v", p.files)
	}
	m.removeSelectedRecentNavigation()
	if slices.Contains(m.recentNav.paths, previous) {
		t.Fatalf("deleted file remained in recent history: %v", m.recentNav.paths)
	}
	if p.index != 0 || len(p.files) != 1 || p.files[0] != current {
		t.Fatalf("deleted row was not removed: %+v", p)
	}
}

func TestSwitcherListsOnlyOpenFilesInActivationOrder(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "src", "First.java")
	second := filepath.Join(root, "src", "Second.java")
	closed := filepath.Join(root, "src", "Closed.java")
	firstDoc := docbridge.Virtual(first, "class First {}")
	secondDoc := docbridge.Virtual(second, "class Second {}")
	m := &Model{tabs: []*Tab{
		{doc: firstDoc, state: &editor.State{Buffer: firstDoc.Buffer()}},
		{doc: secondDoc, state: &editor.State{Buffer: secondDoc.Buffer()}},
	}}
	m.setRoot(root)
	m.recentNav.paths = []string{second, closed, first}

	got := m.openRecentSwitcherPaths()
	want := []string{second, first}
	if !slices.Equal(got, want) {
		t.Fatalf("switcher paths = %v, want %v", got, want)
	}
}

func TestSwitcherListsAgentEditorsAndFiltersTheirTitles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "src", "First.java")
	second := filepath.Join(root, "src", "Second.java")
	firstDoc := docbridge.Virtual(first, "class First {}")
	secondDoc := docbridge.Virtual(second, "class Second {}")
	m := &Model{}
	m.setRoot(root)
	m.agents.tasks = map[string]agentTask{
		"a": agentTestTask("a", "Fix the parser", "running", 1),
	}
	firstTab := &Tab{doc: firstDoc, state: &editor.State{Buffer: firstDoc.Buffer()}}
	secondTab := &Tab{doc: secondDoc, state: &editor.State{Buffer: secondDoc.Buffer()}, used: 2}
	agentTab := m.newAgentPageTab(tabKindAgentSession, agentSessionPath("a"), "Old title", "a")
	agentTab.used = 3
	m.tabs = []*Tab{firstTab, agentTab, secondTab}
	m.recentNav.paths = []string{first}

	items := m.recentSwitcherItems("")
	got := make([]string, len(items))
	for i, item := range items {
		got[i] = item.title
	}
	want := []string{"Fix the parser", "Second.java", "First.java"}
	if !slices.Equal(got, want) {
		t.Fatalf("switcher items = %v, want %v", got, want)
	}
	if !items[0].nonFile || items[0].tab != agentTab || items[0].path != agentSessionPath("a") {
		t.Fatalf("agent row = %+v", items[0])
	}
	if got := m.recentSwitcherItems("parser"); len(got) != 1 || got[0].tab != agentTab {
		t.Fatalf("agent title filter = %+v", got)
	}
}

func TestSwitcherNavigationWrapsAcrossFilesAndTools(t *testing.T) {
	files := []recentSwitcherItem{{path: "a"}, {path: "b"}, {path: "c"}}
	tools := []*toolWindowSpec{{id: "one"}, {id: "two"}}
	tests := []struct {
		name        string
		onTools     bool
		index, step int
		wantTools   bool
		wantIndex   int
	}{
		{"stay in files", false, 1, 0, false, 1},
		{"files to tools", false, 1, 2, true, 0},
		{"next tool", true, 0, 1, true, 1},
		{"tools wrap forward to files", true, 1, 1, false, 0},
		{"files wrap back to tools", false, 0, -1, true, 1},
		{"tools wrap back to files", true, 0, -1, false, 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &recentNavPopup{
				switcher: true, switcherItems: files, tools: tools,
				onTools: test.onTools, index: test.index,
			}
			m := &Model{recentNav: recentNavigation{popup: p}}
			m.moveSwitcherNavigation(test.step)
			if p.onTools != test.wantTools || p.index != test.wantIndex {
				t.Fatalf("selection = tools:%v index:%d, want tools:%v index:%d",
					p.onTools, p.index, test.wantTools, test.wantIndex)
			}
		})
	}
}

func TestSwitcherActivatesAnAgentHomeTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Main.java")
	doc := docbridge.Virtual(path, "class Main {}")
	fileTab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{tabs: []*Tab{fileTab}, active: 0}
	m.setRoot(filepath.Dir(path))
	home := m.newAgentPageTab(tabKindAgentHome, "agent:new", "New Agent Task", "")
	m.tabs = append(m.tabs, home)
	m.recentNav.popup = &recentNavPopup{
		switcher: true,
		switcherItems: []recentSwitcherItem{
			{path: path},
			{path: "agent:new", title: "New Agent Task", tab: home, nonFile: true},
		},
		index: 1,
	}

	m.chooseRecentNavigation(false)

	if m.recentNav.popup != nil || m.tab() != home {
		t.Fatalf("Switcher activation = active:%+v popup:%+v", m.tab(), m.recentNav.popup)
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

func TestRecentFilesSingleClickOpens(t *testing.T) {
	m := gotoLineTestModel("class Main {}", 0)
	m.setRoot(t.TempDir())
	m.markRecent(filepath.Join(m.Root(), "pom.xml"))
	m.markRecent(m.tab().doc.Path())
	tt := ui.NewTester(m.View, 1440, 900)
	m.RunCommand("navigate.recentFiles")
	tt.Frame()
	if err := tt.Click("test.java"); err != nil {
		t.Fatal(err)
	}
	if m.recentNav.popup != nil {
		t.Fatal("single click selected the recent file instead of opening it")
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

func TestRecentFilesKeyboardRangeAndModifierClicks(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "one.java"),
		filepath.Join(root, "two.java"),
		filepath.Join(root, "three.java"),
	}
	current := docbridge.Virtual(filepath.Join(root, "current.java"), "class Current {}")
	m := &Model{
		tabs: []*Tab{{doc: current, state: &editor.State{Buffer: current.Buffer()}}},
	}
	m.setRoot(root)
	m.recentNav.paths = append([]string(nil), paths...)

	tt := ui.NewTester(m.View, 1440, 900)
	m.RunCommand("navigate.recentFiles")
	tt.Frame()
	p := m.recentNav.popup
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Shift, ui.KeyUp)
	if p.index != 0 || !slices.Equal(p.selectedRecentFiles(), paths[:2]) {
		t.Fatalf("keyboard range = %+v, selected %v", p.fileSelection, p.selectedRecentFiles())
	}

	tt.Frame()
	if err := tt.ClickWith(ui.Cmd, "three.java"); err != nil {
		t.Fatal(err)
	}
	if p.index != 2 || !slices.Equal(p.selectedRecentFiles(), paths) {
		t.Fatalf("Cmd-click add = %+v, selected %v", p.fileSelection, p.selectedRecentFiles())
	}
	tt.Frame()
	if err := tt.ClickWith(ui.Cmd, "two.java"); err != nil {
		t.Fatal(err)
	}
	if p.index != 2 || !slices.Equal(p.selectedRecentFiles(), []string{paths[0], paths[2]}) {
		t.Fatalf("Cmd-click remove = %+v, selected %v", p.fileSelection, p.selectedRecentFiles())
	}
	if m.recentNav.popup == nil {
		t.Fatal("modifier click opened instead of changing the selection")
	}
}

func TestRecentFilesEnterOpensSelectionInListOrder(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "one.java"),
		filepath.Join(root, "two.java"),
	}
	tabs := make([]*Tab, 0, len(paths))
	for _, path := range paths {
		doc := docbridge.Virtual(path, "class File {}")
		tabs = append(tabs, &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}})
	}
	m := &Model{tabs: tabs, active: 1}
	m.setRoot(root)
	m.recentNav.paths = append([]string(nil), paths...)

	tt := ui.NewTester(m.View, 1440, 900)
	m.RunCommand("navigate.recentFiles")
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Shift, ui.KeyUp)
	tt.Key(0, ui.KeyEnter)

	if m.recentNav.popup != nil {
		t.Fatal("Enter did not dismiss Recent Files")
	}
	if tabPath(m.tab()) != paths[0] {
		t.Fatalf("lead tab = %q, want %q", tabPath(m.tab()), paths[0])
	}
}

func TestRecentFilesShiftEnterOpensInSideGroup(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "one.java")
	second := filepath.Join(root, "two.java")
	firstDoc := docbridge.Virtual(first, "class One {}")
	secondDoc := docbridge.Virtual(second, "class Two {}")
	m := &Model{
		tabs: []*Tab{
			{doc: firstDoc, state: &editor.State{Buffer: firstDoc.Buffer()}},
			{doc: secondDoc, state: &editor.State{Buffer: secondDoc.Buffer()}},
		},
		active: 1,
	}
	m.setRoot(root)
	m.recentNav.paths = []string{first, second}

	tt := ui.NewTester(m.View, 1440, 900)
	m.RunCommand("navigate.recentFiles")
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Shift, ui.KeyEnter)

	if m.split == nil || len(m.split.tabs) != 1 || tabPath(m.split.tabs[0]) != first {
		t.Fatalf("side group = %+v", m.split)
	}
	if tabPath(m.tab()) != second || !m.splitFirst || m.splitDown {
		t.Fatalf("side open focused tab=%q first=%v down=%v", tabPath(m.tab()), m.splitFirst, m.splitDown)
	}
}

func TestRecentFilesShiftEnterCopiesTheOnlyFocusedTab(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one.java")
	backend := &fileActionBackend{info: openidea.DocInfo{
		ID: 17, Path: path, Text: "class One {}", Version: 1,
		Encoding: "UTF-8", LineEnding: "LF", URI: "file://" + path,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg := docbridge.NewRegistry(backend, Window)
	doc, err := reg.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{ctx: ctx, reg: reg}
	original := m.newTab(doc)
	m.tabs = []*Tab{original}
	m.setRoot(root)
	m.recentNav.paths = []string{path}

	tt := ui.NewTester(m.View, 1440, 900)
	m.RunCommand("navigate.recentFiles")
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyEnter)

	if len(m.split.tabs) != 1 {
		t.Fatalf("side group = %+v", m.split)
	}
	copy := m.tab()
	if copy == original || copy.doc != doc || copy.state == original.state {
		t.Fatalf("copied tab = %+v, original = %+v", copy, original)
	}
	if len(m.tabs) != 1 || m.tabs[0] != copy || m.split.tabs[0] != original {
		t.Fatalf("focused=%+v side=%+v", m.tabs, m.split.tabs)
	}

	m.focusRecentLead(path, true)
	if len(m.allTabs()) != 2 || len(m.tabs) != 1 || len(m.split.tabs) != 1 || m.tab().doc != doc {
		t.Fatalf("repeated side focus added a tab: focused=%+v side=%+v", m.tabs, m.split.tabs)
	}
}

func TestRecentFilesNoMatchEnterCarriesQueryToGoToFile(t *testing.T) {
	doc := docbridge.Virtual(filepath.Join(t.TempDir(), "one.java"), "class One {}")
	m := &Model{tabs: []*Tab{{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx = ctx
	m.mgr = openidea.NewManager(openidea.LaunchConfig{})
	m.postHook = func(fn func()) { fn() }
	m.setRoot(filepath.Dir(doc.Path()))
	m.markRecent(doc.Path())
	tt := ui.NewTester(m.View, 1440, 900)
	m.RunCommand("navigate.recentFiles")
	p := m.recentNav.popup
	p.query = "NoMatchingRecentFile"
	m.refreshRecentNavigation(true)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)

	if m.recentNav.popup != nil || m.se == nil || m.se.tab != seTabFiles {
		t.Fatalf("Go to File handoff = popup:%v se:%+v", m.recentNav.popup, m.se)
	}
	if m.se.query != "NoMatchingRecentFile" {
		t.Fatalf("Go to File query = %q", m.se.query)
	}
}

func TestSwitcherOpensSelectionWhenControlIsReleased(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "one.java")
	second := filepath.Join(root, "two.java")
	firstDoc := docbridge.Virtual(first, "class One {}")
	secondDoc := docbridge.Virtual(second, "class Two {}")
	m := &Model{
		tabs: []*Tab{
			{doc: firstDoc, state: &editor.State{Buffer: firstDoc.Buffer()}},
			{doc: secondDoc, state: &editor.State{Buffer: secondDoc.Buffer()}},
		},
		active: 1,
	}
	m.setRoot(root)
	m.recentNav.paths = []string{second, first}
	m.CycleSwitcher(1)
	tt := ui.NewTester(m.View, 1440, 900)
	tt.HoldModifiers(ui.Ctrl)
	tt.Frame()
	if m.recentNav.popup == nil || !m.recentNav.popup.switcherSaw {
		t.Fatal("Switcher did not stay open while Ctrl was held")
	}
	tt.HoldModifiers(0)
	tt.Frame()

	if m.recentNav.popup != nil {
		t.Fatal("Switcher stayed open after Ctrl was released")
	}
	if tabPath(m.tab()) != first {
		t.Fatalf("Ctrl release opened %q, want %q", tabPath(m.tab()), first)
	}
}

func TestSwitcherHasNoSpeedSearchAndAcceptsNativeKeys(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "one.java")
	second := filepath.Join(root, "two.java")
	firstDoc := docbridge.Virtual(first, "class One {}")
	secondDoc := docbridge.Virtual(second, "class Two {}")
	m := &Model{
		tabs: []*Tab{
			{doc: firstDoc, state: &editor.State{Buffer: firstDoc.Buffer()}},
			{doc: secondDoc, state: &editor.State{Buffer: secondDoc.Buffer()}},
		},
		active: 0,
	}
	m.setRoot(root)
	m.recentNav.paths = []string{second, first}
	m.CycleSwitcher(1)

	tt := ui.NewTester(m.View, 1440, 900)
	tt.Frame()
	if tt.HasText("Switch to a recent file") {
		t.Fatalf("Switcher rendered a speed-search field: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)

	if m.recentNav.popup != nil || tabPath(m.tab()) != second {
		t.Fatalf("native Switcher Enter = tab:%q popup:%+v", tabPath(m.tab()), m.recentNav.popup)
	}
}

func TestRecentNavigationPageStepsMatchOpenIDEA(t *testing.T) {
	p := &recentNavPopup{files: make([]string, 12)}
	m := &Model{recentNav: recentNavigation{popup: p}}
	m.moveRecentNavigation(1, true, false)
	if p.index != 10 {
		t.Fatalf("Recent Files PageDown selected %d, want 10", p.index)
	}
	p = &recentNavPopup{locations: true, places: make([]recentPlace, 6)}
	m.recentNav.popup = p
	m.moveRecentNavigation(1, true, false)
	if p.index != 4 {
		t.Fatalf("Recent Locations PageDown selected %d, want 4", p.index)
	}
}
