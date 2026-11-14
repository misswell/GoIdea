package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRememberProjectPutsTheFolderFirst(t *testing.T) {
	base := t.TempDir()
	a, b := filepath.Join(base, "a"), filepath.Join(base, "b")
	for _, dir := range []string{a, b} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := &Model{}
	m.rememberProject(a)
	m.rememberProject(b)
	m.rememberProject(a)
	if len(m.recents) != 2 || m.recents[0].Path != a || m.recents[1].Path != b {
		t.Fatalf("recents = %v, want %s then %s", m.recents, a, b)
	}
	m.rememberProject(filepath.Join(base, "gone"))
	if len(m.recents) != 2 {
		t.Fatalf("recents = %v, want the missing folder dropped", m.recents)
	}
}

func TestRecentsKeepTheCap(t *testing.T) {
	base := t.TempDir()
	var dirs []string
	for i := 0; i < maxRecentProjects+3; i++ {
		dir := filepath.Join(base, string(rune('a'+i)))
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, dir)
	}
	m := &Model{}
	for _, dir := range dirs {
		m.rememberProject(dir)
	}
	if len(m.recents) != maxRecentProjects {
		t.Fatalf("recents = %d, want %d", len(m.recents), maxRecentProjects)
	}
	if m.recents[0].Path != dirs[len(dirs)-1] {
		t.Fatalf("recents[0] = %s, want %s", m.recents[0].Path, dirs[len(dirs)-1])
	}
}

func TestFilteredRecentsSearchesThePath(t *testing.T) {
	m := &Model{recents: []RecentProject{{Path: "/work/shop"}, {Path: "/work/notes"}, {Path: "/tmp/shop"}}}
	m.welcomeQuery = "SHOP"
	got := m.filteredRecents()
	if len(got) != 2 || got[0].project.Path != "/work/shop" || got[1].project.Path != "/tmp/shop" {
		t.Fatalf("filtered = %v, want the two shop folders", got)
	}
	m.welcomeQuery = "  "
	if len(m.filteredRecents()) != 3 {
		t.Fatalf("blank query = %v, want every folder", m.filteredRecents())
	}
}

func TestOpenideaTokens(t *testing.T) {
	dark, light := openideaTheme(true), openideaTheme(false)
	if !dark.Dark || light.Dark {
		t.Fatalf("dark.Dark = %v, light.Dark = %v", dark.Dark, light.Dark)
	}
	if dark.Background == light.Background || dark.Text == light.Text {
		t.Fatalf("the two themes look the same: dark=%+v light=%+v", dark, light)
	}
	if dark.Accent != light.Accent {
		t.Fatalf("accents differ: %v and %v", dark.Accent, light.Accent)
	}
	if (Preferences{Theme: "dark"}).themePreference().Background != dark.Background {
		t.Fatal("the dark preference did not pick the OpenIDEA dark theme")
	}
	if (Preferences{Theme: "light"}).themePreference().Background != light.Background {
		t.Fatal("the light preference did not pick the OpenIDEA light theme")
	}
}

func TestProjectInitials(t *testing.T) {
	cases := map[string]string{
		"shop":      "S",
		"_demo":     "D",
		"MyProject": "MP",
		"py-demo":   "PD",
		"idea":      "I",
		"":          "?",
	}
	for name, want := range cases {
		if got := projectInitials(name); got != want {
			t.Fatalf("projectInitials(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCloneDirName(t *testing.T) {
	cases := map[string]string{
		"https://github.com/user/repo.git":   "repo",
		"git@github.com:user/repo.git":       "repo",
		"https://github.com/user/repo/":      "repo",
		"https://github.com/user/repo?x=1#y": "repo",
		"https://example.com/a b/c d":        "c-d",
		"https://example.com/.hidden":        "hidden",
		"user/repo":                          "repo",
	}
	for url, want := range cases {
		if got := cloneDirName(url); got != want {
			t.Fatalf("cloneDirName(%q) = %q, want %q", url, got, want)
		}
	}
}
