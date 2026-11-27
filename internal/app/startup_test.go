package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestInProjectToleratesSeparatorsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "src", "App.java")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("class App {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		root string
		path string
		want bool
	}{
		{"inside", root, file, true},
		{"trailing separator", root + string(filepath.Separator), file, true},
		{"double slash", root + "//", filepath.Join(root, "//", "src", "App.java"), true},
		{"dot segments", root, filepath.Join(root, "src", "..", "src", "App.java"), true},
		{"root itself", root, root, false},
		{"sibling", root, filepath.Join(filepath.Dir(root), "App.java"), false},
		{"prefix trick", root, root + "-other/App.java", false},
		{"empty root", "", file, false},
	}
	for _, c := range cases {
		if got := inProject(c.root, c.path); got != c.want {
			t.Errorf("%s: inProject(%q, %q) = %v, want %v", c.name, c.root, c.path, got, c.want)
		}
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err == nil {
		if !inProject(link, file) {
			t.Errorf("symlinked root: inProject(%q, %q) = false, want true", link, file)
		}
		if !inProject(root, filepath.Join(link, "src", "App.java")) {
			t.Errorf("symlinked file: inProject(%q, %q) = false, want true", root, filepath.Join(link, "src", "App.java"))
		}
	}
}

func TestProjectRestoreDoesNotFlashWelcome(t *testing.T) {
	project := t.TempDir()
	session := Session{Root: project}
	if !startupWillOpenProject(session, Preferences{}, false) {
		t.Fatal("the default startup did not schedule the remembered project")
	}
	reopenOff := false
	if startupWillOpenProject(session,
		Preferences{System: SystemPrefs{ReopenLastProject: &reopenOff}}, false) {
		t.Fatal("Reopen projects on startup off still scheduled a project")
	}
	if startupWillOpenProject(session, Preferences{}, true) {
		t.Fatal("a fresh session scheduled the remembered project")
	}

	m := &Model{startupPending: true}
	tester := ui.NewTester(func(c *ui.Context) { m.View(c) }, 900, 640)
	tester.Frame()
	if !tester.HasText("Opening project…") {
		t.Fatalf("startup surface is missing: %v", tester.Texts())
	}
	if tester.HasText("Welcome to GoIDEA") {
		t.Fatalf("project restore flashed Welcome: %v", tester.Texts())
	}

	m.startupPending = false
	tester.Frame()
	if !tester.HasText("Welcome to GoIDEA") {
		t.Fatalf("Welcome did not return when no project was pending: %v", tester.Texts())
	}
}
