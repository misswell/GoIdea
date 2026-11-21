package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func setTabPath(t *Tab, path string) {
	t.doc = docbridge.Virtual(path, "")
	t.state = &editor.State{Buffer: t.doc.Buffer()}
}

func TestWindowCompatibilityActionsSelectCloseAndReorderTabs(t *testing.T) {
	a := &Tab{}
	b := &Tab{}
	c := &Tab{}
	setTabPath(a, "/p/a.go")
	setTabPath(b, "/p/b.go")
	setTabPath(c, "/p/c.go")
	m := New(Config{SettingsPath: filepath.Join(t.TempDir(), "settings.json")})
	t.Cleanup(m.off)
	m.tabs, m.active = []*Tab{a, b, c}, 1

	if !m.RunActionWithArg("window.selectTab", "/p/c.go") || m.active != 2 {
		t.Fatalf("select window tab: active=%d", m.active)
	}
	if !m.RunActionWithArg("window.reorderTabs", []any{"/p/c.go", "/p/a.go", "/p/b.go"}) {
		t.Fatal("window.reorderTabs did not run")
	}
	wantOrder(t, m.tabs, c, a, b)
	if m.active != 0 {
		t.Fatalf("reordering lost the active tab: %d", m.active)
	}

	m.active = 1
	if !m.RunAction("window.nextTab") || m.active != 2 {
		t.Fatalf("next window tab: active=%d", m.active)
	}
	if !m.RunAction("window.previousTab") || m.active != 1 {
		t.Fatalf("previous window tab: active=%d", m.active)
	}

	if !m.RunActionWithArg("window.closeTabsToLeft", nil) || len(m.tabs) != 2 || m.tabs[0] != a {
		t.Fatalf("close tabs to the left: %v", m.tabs)
	}
	if !m.RunAction("toolwindow.focusEditor") {
		t.Fatal("toolwindow.focusEditor did not run in an editor context")
	}
}

func TestLightEditOpenProjectTargetsTheActiveFilesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "src", "Standalone.java")
	tab := &Tab{}
	setTabPath(tab, path)
	m := &Model{tabs: []*Tab{tab}, active: 0}
	if !m.ActionEnabled("lightEdit.openProject") {
		t.Fatal("LightEdit Open Project is disabled for a standalone file")
	}
	if got, want := filepath.Dir(tab.doc.Path()), filepath.Dir(path); got != want {
		t.Fatalf("LightEdit project target = %q, want %q", got, want)
	}

	m.root = path
	if m.ActionEnabled("lightEdit.openProject") {
		t.Fatal("LightEdit Open Project stayed enabled inside a project")
	}
}
