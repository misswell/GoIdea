package app

import (
	"path/filepath"
	"testing"
)

func TestCloseProjectReturnsToWelcome(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.setRoot(filepath.Join(t.TempDir(), "proj"))
	m.showingWelcome = false
	m.CloseProject()
	if m.Root() != "" {
		t.Fatalf("the project stayed open as %q", m.Root())
	}
	if !m.showingWelcome || m.status != "Open a Java project" {
		t.Fatalf("welcome %v status %q", m.showingWelcome, m.status)
	}
	if m.busy {
		t.Fatal("the window is still busy after closing the project")
	}
}

func TestCloseProjectFromWelcomeStaysThere(t *testing.T) {
	m := &Model{postHook: func(fn func()) { fn() }}
	m.showingWelcome = true
	m.CloseProject()
	if m.Root() != "" || !m.showingWelcome {
		t.Fatalf("root %q welcome %v", m.Root(), m.showingWelcome)
	}
	found := false
	for _, cmd := range builtinCommands() {
		if cmd.id == "project.close" {
			found = true
		}
	}
	if !found {
		t.Fatal("the command registry lacks project.close")
	}
}
