package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestProjectPopupMatchesOpenIDEAEntryGroupsAndSelection(t *testing.T) {
	current := filepath.Join(string(filepath.Separator), "Projects", "g3-applicationmarket")
	second := filepath.Join(string(filepath.Separator), "Projects", "tg-tenant-second")
	third := filepath.Join(string(filepath.Separator), "Projects", "cloudpivot")
	host := &fakeWorkspaceHost{openProjects: []WorkspaceWindow{
		{ID: "current", Title: "g3-applicationmarket", Root: current, Branch: "feature/fastjson-2.0.64", Selected: true},
		{ID: "second", Title: "tg-tenant-second", Root: second, Branch: "feature/cwh/cmp"},
		{ID: "third", Title: "cloudpivot", Root: third, Branch: "from-pre/261116"},
	}}
	m := &Model{
		workspaceHost: host,
		recents: []RecentProject{
			{Path: current, Name: "g3-applicationmarket", Group: "Work"},
			{Path: filepath.Join(string(filepath.Separator), "Projects", "tg-sync-fork"), Name: "tg-sync-fork"},
			{Path: filepath.Join(string(filepath.Separator), "Projects", "g3-org-fork"), Name: "g3-org-fork"},
		},
	}
	m.root = current
	entries := projectPopupEntriesFor(m, "")
	if got, want := len(entries), 4+3+2; got != want {
		t.Fatalf("entries = %d, want %d: %+v", got, want, entries)
	}
	if entries[0].label != "New Project…" || entries[1].label != "Open…" ||
		entries[2].label != "Clone Repository…" || entries[3].label != "Remote Development…" {
		t.Fatalf("action rows = %+v", entries[:4])
	}
	if !entries[4].firstOpen || entries[4].project.Name != "g3-applicationmarket" ||
		entries[5].project.Name != "tg-tenant-second" || entries[6].project.Name != "cloudpivot" {
		t.Fatalf("open project rows = %+v", entries[4:7])
	}
	if !entries[7].firstRecent || entries[7].project.Name != "tg-sync-fork" ||
		entries[8].project.Name != "g3-org-fork" {
		t.Fatalf("recent project rows = %+v", entries[7:])
	}

	m.projectPopup = &projectPopup{anchor: ui.Rect{X: 20, Y: 50, W: 300, H: 30}}
	m.projectPopup.active = 5
	m.chooseProjectPopup()
	if host.focusedProject != second {
		t.Fatalf("focused project = %q, want %q", host.focusedProject, second)
	}
	if m.projectPopup != nil {
		t.Fatal("opening an already open project left the popup up")
	}
}

func TestProjectPopupTypeToFilterAndNoMatch(t *testing.T) {
	openPath := filepath.Join(string(filepath.Separator), "Projects", "tg-tenant-second")
	host := &fakeWorkspaceHost{openProjects: []WorkspaceWindow{{
		ID: "tenant", Title: "tg-tenant-second", Root: openPath,
	}}}
	m := &Model{
		workspaceHost: host,
		recents: []RecentProject{
			{Path: filepath.Join(string(filepath.Separator), "Projects", "g3-org-fork"), Name: "g3-org-fork"},
			{Path: filepath.Join(string(filepath.Separator), "Projects", "tg-sync-fork"), Name: "tg-sync-fork"},
		},
	}
	m.projectPopup = &projectPopup{anchor: ui.Rect{X: 20, Y: 50, W: 300, H: 30}}
	m.typeProjectPopup("tenant")
	entries := projectPopupEntriesFor(m, m.projectPopup.query)
	if len(entries) != 1 || !entries[0].open ||
		entries[0].project.Path != openPath || len(m.projectPopup.query) != len("tenant") {
		t.Fatalf("filtered entries = %+v, query=%q", entries, m.projectPopup.query)
	}
	m.typeProjectPopup("zzz")
	if entries := projectPopupEntriesFor(m, m.projectPopup.query); len(entries) != 0 {
		t.Fatalf("no-match entries = %+v", entries)
	}
	m.eraseProjectPopup()
	if m.projectPopup.query != "tenantzz" {
		t.Fatalf("backspace left query %q", m.projectPopup.query)
	}
}

func TestProjectPopupRendersNativeSectionsAndTitleWidgetOpensIt(t *testing.T) {
	m := &Model{
		recents: []RecentProject{{
			Path: filepath.Join(string(filepath.Separator), "Projects", "g3-org-fork"),
			Name: "g3-org-fork", Branch: "stable",
		}},
	}
	m.root = filepath.Join(string(filepath.Separator), "Projects", "tg-tenant-fork")
	m.projectName = "tg-tenant-fork"
	if overlays.byID["workbench.projectPopup"] == nil {
		t.Fatal("the title-bar project popup is not registered as a native overlay")
	}

	widget := ui.NewTester(func(c *ui.Context) {
		m.projectWidget(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 320, 80)
	widget.Frame()
	if err := widget.Click("Projects"); err != nil {
		t.Fatal(err)
	}
	widget.Frame()
	if m.projectPopup == nil || m.projectPopup.anchor.W <= 0 {
		t.Fatalf("project widget did not open the anchored popup: %+v", m.projectPopup)
	}

	m.projectPopup = &projectPopup{anchor: m.projectPopup.anchor}
	tt := ui.NewTester(func(c *ui.Context) { m.projectPopupDialog(c, c.Theme()) }, 1000, 700)
	tt.Frame()
	for _, want := range []string{
		"New Project…", "Open…", "Clone Repository…", "Remote Development…",
		"Open Projects", "Recent Projects", "tg-tenant-fork", "g3-org-fork",
	} {
		if !tt.HasText(want) {
			t.Fatalf("project popup is missing %q: %s", want, strings.Join(tt.Texts(), "\n"))
		}
	}
	if err := tt.Click("tg-tenant-fork"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.projectPopup != nil {
		t.Fatal("clicking the current open project did not close the popup")
	}
}

func TestProjectPopupTypingAndEnterOpenTheFilteredProject(t *testing.T) {
	host := &fakeWorkspaceHost{openProjects: []WorkspaceWindow{{
		ID: "cloud", Title: "cloudpivot",
		Root: filepath.Join(string(filepath.Separator), "Projects", "cloudpivot"),
	}}}
	m := &Model{workspaceHost: host}
	m.root = filepath.Join(string(filepath.Separator), "Projects", "tg-tenant-fork")
	m.projectName = "tg-tenant-fork"
	m.projectPopup = &projectPopup{anchor: ui.Rect{X: 20, Y: 50, W: 300, H: 30}}
	tt := ui.NewTester(func(c *ui.Context) { m.projectPopupDialog(c, c.Theme()) }, 1000, 700)
	tt.Frame()
	tt.Type("cloud")
	tt.Frame()
	if m.projectPopup == nil || m.projectPopup.query != "cloud" {
		t.Fatalf("typed query = %+v", m.projectPopup)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if m.projectPopup != nil || host.focusedProject != filepath.Join(string(filepath.Separator), "Projects", "cloudpivot") {
		t.Fatalf("filtered Enter popup=%+v focused=%q", m.projectPopup, host.focusedProject)
	}
}

func TestProjectPopupRemovesOnlyRecentProjects(t *testing.T) {
	m := &Model{
		recents: []RecentProject{{
			Path: filepath.Join(string(filepath.Separator), "Projects", "recent"),
			Name: "recent",
		}},
	}
	m.root = filepath.Join(string(filepath.Separator), "Projects", "current")
	m.projectName = "current"
	m.projectPopup = &projectPopup{anchor: ui.Rect{X: 20, Y: 50, W: 300, H: 30}}
	entries := projectPopupEntriesFor(m, "")
	m.projectPopup.active = len(entries) - 1
	m.removeSelectedProjectPopupRecent()
	want := filepath.Join(string(filepath.Separator), "Projects", "recent")
	if !m.welcomeRemoveOpen || m.welcomeRemovePath != want {
		t.Fatalf("remove confirmation = open:%v path:%q", m.welcomeRemoveOpen, m.welcomeRemovePath)
	}
}
