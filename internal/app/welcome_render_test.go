package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// welcomeTester renders the Welcome screen offscreen at the window's default
// size, so the port is checked without opening a window.
func welcomeTester(t *testing.T, m *Model, dark bool) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		m.welcome(c, c.Theme())
	}, 1280, 820)
	tt.SetDark(dark)
	tt.SetScale(1)
	tt.Frame()
	if out := os.Getenv("WELCOME_SHOT"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	return tt
}

func TestWelcomeLandingRenders(t *testing.T) {
	m := &Model{}
	tt := welcomeTester(t, m, true)
	for _, text := range []string{
		"OpenIDEA", "An open-source IDE inspired by IntelliJ IDEA",
		"Projects", "Customize", "Plugins",
		"Welcome to OpenIDEA",
		"Open a folder from disk or clone a repository from version control.",
		"New Project", "Clone Repository", "Remote Development",
		"Open a folder", openAccelerator(),
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not on the landing: %q", text, tt.Texts())
		}
	}
	if r, ok := tt.Find("Projects"); !ok || r.X >= 240 {
		t.Fatalf("Projects is not in the 240px rail: %+v, %v", r, ok)
	}
}

func TestWelcomeRecentProjectsRenders(t *testing.T) {
	dir := t.TempDir()
	m := &Model{
		recents:         []string{dir},
		welcomeBranches: map[string]string{dir: "main"},
	}
	tt := welcomeTester(t, m, true)
	for _, text := range []string{
		"New Project", "Open", "Clone Repository",
		"main", shortenHome(dir),
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not on the projects page: %q", text, tt.Texts())
		}
	}
	if r, ok := tt.Find(filepath.Base(dir)); !ok || r.X < 240 {
		t.Fatalf("the recent project is not right of the rail: %+v, %v", r, ok)
	}
	m.welcomeQuery = "no-such-project"
	tt.Frame()
	if !tt.HasText("No matching projects") {
		t.Fatalf("a query matching nothing shows no empty state: %q", tt.Texts())
	}
}

func TestWelcomeCustomizeAndPluginsRender(t *testing.T) {
	m := &Model{welcomePage: "customize"}
	tt := welcomeTester(t, m, false)
	for _, text := range []string{
		"Color theme", "Theme", "Sync with OS",
		"Accessibility", "Editor font", "All settings…",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not on the customize page: %q", text, tt.Texts())
		}
	}
	m.welcomePage = "plugins"
	tt.Frame()
	for _, text := range []string{
		"Available", "Installed", "Preview",
		"No plugins installed.", "No plugin selected",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not on the plugins page: %q", text, tt.Texts())
		}
	}
}
