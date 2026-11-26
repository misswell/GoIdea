package app

import (
	"testing"

	"github.com/egoist/mygo"
)

// walkMenu calls visit for every entry, submenus included.
func walkMenu(entries []menuEntry, visit func(menuEntry)) {
	for _, e := range entries {
		visit(e)
		walkMenu(e.sub, visit)
	}
}

func TestMainMenuCommandsResolve(t *testing.T) {
	for _, groups := range [][]menuEntry{mainMenuGroups(), gearGroup(&Model{})} {
		walkMenu(groups, func(e menuEntry) {
			if e.sep || e.command == "" {
				return
			}
			if _, ok := Command(e.command); !ok {
				t.Errorf("menu item %q runs unknown command %q", e.label, e.command)
			}
		})
	}
}

func TestMainMenuAcceleratorsFollowTheSelectedPreset(t *testing.T) {
	m := &Model{}
	m.prefs.KeymapPreset = "intellij-macos"
	if got := m.MainMenu().ItemByID("menu.file.save").Accelerator; got != "Cmd+S" {
		t.Fatalf("IntelliJ macOS Save accelerator = %q, want Cmd+S", got)
	}
	m.prefs.KeymapPreset = "intellij-windows"
	if got := m.MainMenu().ItemByID("menu.file.save").Accelerator; got != "Ctrl+S" {
		t.Fatalf("IntelliJ Windows Save accelerator = %q, want Ctrl+S", got)
	}
}

func TestNativeMenuAcceleratorsSkipBindingsAMenuCannotRepresent(t *testing.T) {
	m := &Model{}
	m.prefs.KeymapPreset = "intellij-macos"
	cases := []struct {
		name    string
		command string
		want    string
	}{
		{name: "single stroke", command: "bookmarks.toggle", want: "F3"},
		{name: "chord", command: "navigate.searchEverywhere", want: ""},
		{name: "pointer", command: "editor.addOrRemoveCaret", want: ""},
	}
	for _, tc := range cases {
		got := menuAccelerator(m, menuEntry{command: tc.command})
		if got != tc.want {
			t.Errorf("%s accelerator = %q, want %q", tc.name, got, tc.want)
		}
	}

	m.KeymapSetOverrides(map[string][]string{"file.save": nil})
	if got := m.MainMenu().ItemByID("menu.file.save").Accelerator; got != "" {
		t.Fatalf("removed Save binding left native accelerator %q", got)
	}
}

func TestMainMenuFollowsOpenIDEAOrder(t *testing.T) {
	want := []string{"File", "Edit", "View", "Navigate", "Code", "Refactor", "Build", "Run", "Git", "Tools", "Window", "Help"}
	groups := mainMenuGroups()
	if len(groups) != len(want) {
		t.Fatalf("menu has %d titles, want %d", len(groups), len(want))
	}
	for i, label := range want {
		if groups[i].label != label {
			t.Errorf("menu %d = %q, want %q", i, groups[i].label, label)
		}
	}
}

func TestViewMenuFollowsOpenIDEAStructure(t *testing.T) {
	var view menuEntry
	for _, group := range mainMenuGroups() {
		if group.label == "View" {
			view = group
			break
		}
	}
	if view.label == "" {
		t.Fatal("main menu has no View menu")
	}
	var labels []string
	for _, item := range view.sub {
		if item.sep {
			labels = append(labels, "")
		} else {
			labels = append(labels, item.label)
		}
	}
	want := []string{
		"Tool Windows",
		"Appearance",
		"Active Editor",
		"",
		"Quick Definition",
		"Quick Type Definition",
		"",
		"Recent Locations",
		"Recent Files",
		"Recent Changes",
		"",
		"Compare with Clipboard",
		"",
		"Quick Switch Scheme…",
		"",
		"Reset Font Size in All Editors",
		"",
		"Notifications",
	}
	if len(labels) != len(want) {
		t.Fatalf("View menu = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("View[%d] = %q, want %q (all: %v)", i, labels[i], want[i], labels)
		}
	}
	if got := view.sub[4].command; got != "code.quickDefinition" {
		t.Fatalf("Quick Definition command = %q", got)
	}
	if got := view.sub[5].command; got != "code.quickTypeDefinition" {
		t.Fatalf("Quick Type Definition command = %q", got)
	}
	if got := view.sub[13].command; got != "view.quickSwitchScheme" {
		t.Fatalf("Quick Switch Scheme command = %q", got)
	}

	appearance := view.sub[1].sub
	appearanceLabels := make([]string, 0, len(appearance))
	for _, item := range appearance {
		if item.sep {
			appearanceLabels = append(appearanceLabels, "")
		} else {
			appearanceLabels = append(appearanceLabels, item.label)
		}
	}
	wantAppearance := []string{
		"Enter Distraction Free Mode",
		"Toggle Full Screen",
		"Compact Mode",
		"",
		"Zoom In IDE",
		"Zoom Out IDE",
		"Reset IDE Zoom",
		"",
		"Toggle Light/Dark Theme",
		"Customize Main Toolbar…",
		"",
		"Toolbar",
		"Tool Window Bars",
		"Navigation Bar",
		"Status Bar",
		"Status Bar Widgets",
	}
	if len(appearanceLabels) != len(wantAppearance) {
		t.Fatalf("Appearance menu = %v, want %v", appearanceLabels, wantAppearance)
	}
	for i := range wantAppearance {
		if appearanceLabels[i] != wantAppearance[i] {
			t.Fatalf("Appearance[%d] = %q, want %q (all: %v)",
				i, appearanceLabels[i], wantAppearance[i], appearanceLabels)
		}
	}
}

func TestMenuSubmenusCarryEntries(t *testing.T) {
	walkMenu(mainMenuGroups(), func(e menuEntry) {
		if e.label != "" && len(e.sub) > 0 && e.command != "" {
			t.Errorf("submenu %q also runs a command", e.label)
		}
	})
}

func TestOpenPluginsShowsPluginsPage(t *testing.T) {
	m := &Model{}
	m.openPlugins()
	if !m.showingWelcome || m.welcomePage != "plugins" {
		t.Fatalf("openPlugins = welcome %v page %q, want Welcome's Plugins page", m.showingWelcome, m.welcomePage)
	}
}

func TestToggleThemeAlternates(t *testing.T) {
	m := &Model{}
	m.ToggleTheme()
	if m.prefs.Theme != "light" {
		t.Fatalf("first toggle = %q, want light", m.prefs.Theme)
	}
	m.ToggleTheme()
	if m.prefs.Theme != "dark" {
		t.Fatalf("second toggle = %q, want dark", m.prefs.Theme)
	}
}

func TestTitleBarShowsVersionControlFallback(t *testing.T) {
	m := &Model{}
	m.setRoot(t.TempDir())
	tt := workbenchTester(t, m)
	if !tt.HasText("Version Control") {
		t.Fatalf("title bar lacks the Version Control fallback: %q", tt.Texts())
	}
}

func TestRunCommandRunsKnownAndRejectsUnknown(t *testing.T) {
	m := &Model{}
	if m.RunCommand("no.such.command") {
		t.Fatalf("unknown command reported as run")
	}
	if !m.RunCommand("plugins.settings") {
		t.Fatalf("plugins.settings is not in the registry")
	}
	if m.welcomePage != "plugins" {
		t.Fatalf("plugins.settings opened page %q", m.welcomePage)
	}
}

func TestOpenRecentNativeSubmenuCarriesPathsAndPlaceholder(t *testing.T) {
	m := &Model{recents: []RecentProject{
		{Path: "/work/a", Name: "Alpha"},
		{Path: "/work/b", Name: "Beta"},
	}}
	menu := m.MainMenu()
	file := menu.Items()[1]
	if file.Label != "File" {
		t.Fatalf("second top-level menu = %q", file.Label)
	}
	var openRecent *mygo.MenuItem
	for _, item := range file.Submenu {
		if item.Label == "Open Recent" {
			openRecent = item
			break
		}
	}
	if openRecent == nil || len(openRecent.Submenu) < 4 {
		t.Fatalf("Open Recent = %+v", openRecent)
	}
	first := openRecent.Submenu[0]
	if first.Label != "Alpha" || first.ID != "menu.file.openRecentProject" || first.Click == nil {
		t.Fatalf("first recent = %+v", first)
	}

	empty := (&Model{}).MainMenu().Items()[1].Submenu
	for _, item := range empty {
		if item.Label != "Open Recent" {
			continue
		}
		if len(item.Submenu) == 0 || item.Submenu[0].Label != "No Recent Projects" || !item.Submenu[0].Disabled {
			t.Fatalf("empty Open Recent = %+v", item.Submenu)
		}
		return
	}
	t.Fatal("empty menu lacks Open Recent")
}
