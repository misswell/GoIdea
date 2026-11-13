package app

import "testing"

// walkMenu calls visit for every entry, submenus included.
func walkMenu(entries []menuEntry, visit func(menuEntry)) {
	for _, e := range entries {
		visit(e)
		walkMenu(e.sub, visit)
	}
}

func TestMainMenuCommandsResolve(t *testing.T) {
	for _, groups := range [][]menuEntry{mainMenuGroups(), gearGroup()} {
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

func TestMainMenuFollowsOpenIDEAOrder(t *testing.T) {
	want := []string{"File", "Edit", "View", "Navigate", "Code", "Refactor", "Build", "Run", "Git", "Tools", "Window"}
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
	if !m.RunCommand("app.plugins") {
		t.Fatalf("app.plugins is not in the registry")
	}
	if m.welcomePage != "plugins" {
		t.Fatalf("app.plugins opened page %q", m.welcomePage)
	}
}
