package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProjectNameValidation(t *testing.T) {
	for _, bad := range []string{"", "  ", "a/b", `a\b`, "a:b", "a*b", "?", "<", ">", "|", ".", ".."} {
		if projectNameError(bad) == "" {
			t.Fatalf("%q was accepted", bad)
		}
	}
	for _, ok := range []string{"shop", "my-app", "my_app.v2", "项目"} {
		if err := projectNameError(ok); err != "" {
			t.Fatalf("%q refused: %s", ok, err)
		}
	}
}

func TestNewProjectPathJoinsLocationAndName(t *testing.T) {
	m := &Model{}
	m.NPForTest("shop", "/tmp/work")
	if got := m.NewProjectPath(); got != filepath.Join("/tmp/work", "shop") {
		t.Fatalf("path = %q", got)
	}
	m.NPForTest("bad/name", "/tmp/work")
	if got := m.NewProjectPath(); got != "" {
		t.Fatalf("an invalid name still made %q", got)
	}
}

func TestCreateProjectFolderCreatesReusesAndRefuses(t *testing.T) {
	m := &Model{}
	ctx := context.Background()
	root := t.TempDir()

	fresh := filepath.Join(root, "fresh")
	if err := m.createProjectFolder(ctx, fresh, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	if st, err := os.Stat(fresh); err != nil || !st.IsDir() {
		t.Fatalf("the folder is not there: %v", err)
	}

	// An empty folder is reused.
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.createProjectFolder(ctx, empty, false); err != nil {
		t.Fatalf("reuse: %v", err)
	}

	// A non-empty one is refused.
	full := filepath.Join(root, "full")
	if err := os.Mkdir(full, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.createProjectFolder(ctx, full, false); err == nil {
		t.Fatal("a non-empty folder was accepted")
	}
}

func TestCreateProjectFolderRunsGitInit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	m := &Model{}
	dir := filepath.Join(t.TempDir(), "repo")
	if err := m.createProjectFolder(context.Background(), dir, true); err != nil {
		t.Fatalf("create with git: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("no Git repository: %v", err)
	}
	// Without the switch there is none.
	plain := filepath.Join(t.TempDir(), "plain")
	if err := m.createProjectFolder(context.Background(), plain, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(plain, ".git")); err == nil {
		t.Fatal("Git was initialized without the switch")
	}
}

// NPForTest stages the dialog's name and location without opening a window.
func (m *Model) NPForTest(name, location string) {
	m.newProject.name = name
	m.newProject.location = location
}
