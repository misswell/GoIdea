package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSessionRecentsMigrateTheOldList(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "alpha"), filepath.Join(dir, "beta")
	for _, d := range []string{a, b} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := sessionRecents(Session{RecentProjects: []string{a, b}})
	if len(got) != 2 || got[0].Path != a || got[0].Name != "alpha" {
		t.Fatalf("migrated = %+v", got)
	}
	if got[0].OpenedAt <= got[1].OpenedAt {
		t.Fatalf("the order was not kept: %+v", got)
	}
	// The richer list wins once it is there.
	got = sessionRecents(Session{RecentProjects: []string{a},
		Recents: []RecentProject{{Path: b, Name: "Bee", Branch: "main"}}})
	if len(got) != 1 || got[0].Path != b || got[0].Branch != "main" {
		t.Fatalf("richer list ignored: %+v", got)
	}
}

func TestFuzzySubsequenceScoresNamesAndPaths(t *testing.T) {
	idx, score, ok := fuzzySubsequence("shp", "shop")
	if !ok || score <= 0 || len(idx) != 3 {
		t.Fatalf("subsequence: %v %d %v", idx, score, ok)
	}
	if _, _, ok := fuzzySubsequence("zzz", "shop"); ok {
		t.Fatal("a missing query matched")
	}
	if _, _, ok := fuzzySubsequence("SHOP", "shop"); !ok {
		t.Fatal("the match is case sensitive")
	}
	m := &Model{recents: []RecentProject{
		{Path: "/tmp/things", Name: "notes"},
		{Path: "/work/shop", Name: "shop", OpenedAt: 1},
		{Path: "/work/shop-app", Name: "shop-app", OpenedAt: 2},
	}}
	m.welcomeQuery = "shop"
	got := m.filteredRecents()
	if len(got) != 2 {
		t.Fatalf("filtered = %+v", got)
	}
	if got[0].project.Name != "shop-app" {
		t.Fatalf("a whole-name match did not rank first: %+v", got[0].project)
	}
	if len(got[0].nameIdx) == 0 {
		t.Fatalf("no highlight indices: %+v", got[0])
	}
}

func TestWelcomeSelectionClamps(t *testing.T) {
	m := &Model{recents: []RecentProject{{Path: "/a", Name: "a"}, {Path: "/b", Name: "b"}}}
	m.WelcomeSelect(-5)
	if m.welcomeSel != 0 {
		t.Fatalf("selection = %d", m.welcomeSel)
	}
	m.WelcomeSelect(9)
	if m.welcomeSel != 1 {
		t.Fatalf("selection = %d", m.welcomeSel)
	}
	m.WelcomeClearQuery()
	if m.welcomeSel != 0 || m.welcomeQuery != "" {
		t.Fatalf("clear left %d %q", m.welcomeSel, m.welcomeQuery)
	}
}

func TestRecentGroupsPersistAndMoveProjects(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &Model{cfg: Config{SessionPath: filepath.Join(dir, "session.json")}}
	m.rememberProject(a)
	m.CreateRecentGroup("Work")
	if !m.groupExists("Work") {
		t.Fatal("the group was not created")
	}
	m.MoveRecentToGroup(a, "Work")
	if m.welcomeGroupCount("Work") != 1 || m.recents[0].Group != "Work" {
		t.Fatalf("move: %+v", m.recents)
	}
	m.ToggleRecentGroup("Work")
	if !m.recentGroups[0].Collapsed {
		t.Fatal("collapse was not stored")
	}
	var s Session
	deadline := time.Now().Add(3 * time.Second)
	for {
		s = LoadSession(m.cfg.SessionPath)
		if len(s.RecentGroups) == 1 && s.RecentGroups[0].Collapsed || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(s.Recents) != 1 || s.Recents[0].Group != "Work" || len(s.RecentGroups) != 1 || !s.RecentGroups[0].Collapsed {
		t.Fatalf("session = %+v %+v", s.Recents, s.RecentGroups)
	}
	m.RenameRecentGroup("Work", "Job")
	if m.recents[0].Group != "Job" {
		t.Fatalf("rename did not move the project: %+v", m.recents[0])
	}
	m.DeleteRecentGroup("Job")
	if m.groupExists("Job") || m.recents[0].Group != "" {
		t.Fatalf("delete left %+v", m.recents[0])
	}
	// Let the sequenced writer drain, so the temp folder is quiet before the
	// test's cleanup removes it.
	m.saveSessionNow()
	_ = os.Remove(m.cfg.SessionPath)
}

func TestRememberProjectKeepsGroupAndBranch(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &Model{recents: []RecentProject{{Path: a, Name: "a", Group: "Work", Branch: "main"}}}
	m.rememberProject(a)
	if len(m.recents) != 1 || m.recents[0].Group != "Work" || m.recents[0].Branch != "main" || m.recents[0].OpenedAt == 0 {
		t.Fatalf("remember lost %+v", m.recents[0])
	}
}

func TestWelcomeRendersAGroupAndItsProjects(t *testing.T) {
	m := &Model{recents: []RecentProject{{Path: "/work/shop", Name: "shop"}}}
	m.CreateRecentGroup("Work")
	m.MoveRecentToGroup("/work/shop", "Work")
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1440, 900)
	tt.Frame()
	if !tt.HasText("Work") {
		t.Fatalf("the group did not draw: %q", tt.Texts())
	}
	if !tt.HasText("shop") {
		t.Fatalf("the grouped project did not draw: %q", tt.Texts())
	}
}

func TestRecentRowsPutGroupsFirstAndSearchStaysFlat(t *testing.T) {
	m := &Model{
		recents: []RecentProject{
			{Path: "/work/a", Name: "a", OpenedAt: 3},
			{Path: "/work/b", Name: "b", OpenedAt: 2, Group: "Work"},
			{Path: "/work/c", Name: "c", OpenedAt: 1},
		},
		recentGroups: []RecentGroup{{Name: "Work"}},
	}
	rows := m.recentRows()
	if len(rows) != 4 || rows[0].group == nil || rows[0].group.Name != "Work" ||
		rows[1].match == nil || rows[1].match.project.Path != "/work/b" ||
		rows[2].match == nil || rows[2].match.project.Path != "/work/a" ||
		rows[3].match == nil || rows[3].match.project.Path != "/work/c" {
		t.Fatalf("grouped rows = %+v", rows)
	}
	m.welcomeQuery = "a"
	rows = m.recentRows()
	if len(rows) != 1 || rows[0].group != nil || rows[0].match.project.Path != "/work/a" {
		t.Fatalf("query rows = %+v", rows)
	}
}

func TestRecentRetentionOnlyCapsUngroupedProjects(t *testing.T) {
	base := t.TempDir()
	projects := make([]RecentProject, 0, maxRecentProjects+4)
	for i := 0; i < maxRecentProjects+2; i++ {
		path := filepath.Join(base, "open"+string(rune('a'+i)))
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		projects = append(projects, RecentProject{Path: path, Name: path})
	}
	for i := 0; i < 2; i++ {
		path := filepath.Join(base, "grouped"+string(rune('a'+i)))
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		projects = append(projects, RecentProject{Path: path, Name: path, Group: "Keep"})
	}
	m := &Model{}
	m.setRecentGroupsAndProjects([]RecentGroup{{Name: "Keep"}}, projects)
	ungrouped, grouped := 0, 0
	for _, project := range m.recents {
		if project.Group == "" {
			ungrouped++
		} else {
			grouped++
		}
	}
	if ungrouped != maxRecentProjects || grouped != 2 {
		t.Fatalf("retained = ungrouped %d grouped %d, want %d and 2", ungrouped, grouped, maxRecentProjects)
	}
}

func TestRecentGroupsNormalizeNamesAndMembership(t *testing.T) {
	m := &Model{}
	m.setRecentGroupsAndProjects(
		[]RecentGroup{{Name: " Work "}, {Name: "Work"}, {Name: " "}},
		[]RecentProject{
			{Path: "/work/a", Name: "a", Group: " Work "},
			{Path: "/work/b", Name: "b", Group: "Missing"},
		},
	)
	if len(m.recentGroups) != 1 || m.recentGroups[0].Name != "Work" {
		t.Fatalf("groups = %+v", m.recentGroups)
	}
	if len(m.recents) != 2 {
		t.Fatalf("recents = %+v", m.recents)
	}
	if m.recents[0].Group != "Work" || m.recents[1].Group != "" {
		t.Fatalf("memberships = %+v", m.recents)
	}
	if got := m.groupNameError("Work", ""); got != "A group named 'Work' already exists" {
		t.Fatalf("duplicate error = %q", got)
	}
	if got := m.groupNameError("Work", "Work"); got != "" {
		t.Fatalf("unchanged rename error = %q", got)
	}
}

func TestOpenRecentInNewWindowUsesNativeHost(t *testing.T) {
	host := &fakeWorkspaceHost{windows: 1, openProjectOK: true}
	m := &Model{workspaceHost: host}
	m.OpenRecentInNewWindow("/projects/local")
	if !reflect.DeepEqual(host.openProject, []string{"/projects/local"}) {
		t.Fatalf("local window = %q", host.openProject)
	}
	host.openProject = nil
	// URL-escaped text is not a usable remote target and must not create a
	// window with an unnormalized path.
	m.OpenRecentInNewWindow("ssh://host")
	if len(host.openProject) != 0 {
		t.Fatalf("invalid remote opened a window: %q", host.openProject)
	}
	host.openProject = nil
	m.OpenRecentInNewWindow("ssh://host/srv/app")
	if !reflect.DeepEqual(host.openProject, []string{"ssh://host/srv/app"}) {
		t.Fatalf("remote window = %q", host.openProject)
	}
}

func TestWelcomeUIFontWritesIDEAppearance(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	m := &Model{
		cfg:   Config{SettingsPath: settingsPath},
		prefs: Preferences{FontSize: 20},
	}
	m.SetWelcomeUIFontSize(15)
	if m.prefs.FontSize != 20 {
		t.Fatalf("editor font changed to %v", m.prefs.FontSize)
	}
	if m.prefs.Appearance.UIFontSize != 15 || !m.prefs.Appearance.UseCustomFont {
		t.Fatalf("UI appearance = %+v", m.prefs.Appearance)
	}
	stored := LoadPreferences(settingsPath)
	if stored.Appearance.UIFontSize != 15 || !stored.Appearance.UseCustomFont {
		t.Fatalf("stored UI appearance = %+v", stored.Appearance)
	}
	m.SetWelcomeUIFontSize(99)
	if m.prefs.Appearance.UIFontSize != 18 {
		t.Fatalf("size above range = %d", m.prefs.Appearance.UIFontSize)
	}
	m.SetWelcomeUIFontSize(1)
	if m.prefs.Appearance.UIFontSize != 10 {
		t.Fatalf("size below range = %d", m.prefs.Appearance.UIFontSize)
	}
}

func TestWelcomeGearMatchesSharedGearOrder(t *testing.T) {
	got := (&Model{}).welcomeGearEntries()
	want := gearGroup(&Model{})
	if len(got) != len(want) {
		t.Fatalf("Welcome gear has %d items, shared gear has %d", len(got), len(want))
	}
	for i := range want {
		if got[i].label != want[i].label || got[i].sep != want[i].sep || got[i].command != want[i].command {
			t.Fatalf("Welcome gear[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
