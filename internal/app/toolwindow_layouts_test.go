package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNamedToolWindowLayoutsSaveRestoreRenameDeleteAndPersist(t *testing.T) {
	m, root := layoutModel(t)
	m.ShowRun()
	m.MoveActiveToolWindowToSplit()
	m.ShowProblems()
	m.bottomHeight = bottomDefaultHeight + bottomStep

	if !m.RunAction("layout.save") || m.layoutDlg == nil {
		t.Fatal("layout.save did not open Save Current Layout as…")
	}
	m.layoutDlg.name = "Debugging"
	m.SaveCurrentToolWindowLayout()
	if len(m.namedLayouts) != 1 || m.activeNamedLayout != "Debugging" {
		t.Fatalf("saved layouts = %+v active=%q", m.namedLayouts, m.activeNamedLayout)
	}

	m.ResetToolWindowLayout()
	if m.activeNamedLayout != "" {
		t.Fatal("Restore Default Layout kept a named layout active")
	}
	if !m.RunActionWithArg("layout.restore", "Debugging") {
		t.Fatal("layout.restore by name did not run")
	}
	if m.splitTool() != "run" || m.primaryTool() != "problems" ||
		m.bottomHeight != bottomDefaultHeight+bottomStep || m.activeNamedLayout != "Debugging" {
		t.Fatalf("restored layout: split=%q primary=%q height=%d active=%q",
			m.splitTool(), m.primaryTool(), m.bottomHeight, m.activeNamedLayout)
	}

	// Save Current Layout as… replaces a same-named entry in place.
	m.bottomHeight = bottomDefaultHeight + 2*bottomStep
	m.OpenSaveCurrentLayout()
	m.layoutDlg.name = "Debugging"
	m.SaveCurrentToolWindowLayout()
	if len(m.namedLayouts) != 1 ||
		m.namedLayouts[0].Layout.BottomHeight != bottomDefaultHeight+2*bottomStep {
		t.Fatalf("same-name save did not replace in place: %+v", m.namedLayouts)
	}

	m.OpenSaveCurrentLayout()
	m.layoutDlg.name = "Review"
	m.SaveCurrentToolWindowLayout()
	m.OpenRenameToolWindowLayout()
	m.ChooseToolWindowLayout(0)
	m.layoutDlg.name = "Review"
	m.RenameToolWindowLayout()
	if m.layoutDlg == nil || m.layoutDlg.err != "A layout with this name exists" {
		t.Fatalf("collision validation = %+v", m.layoutDlg)
	}
	m.layoutDlg.name = "Reviewing"
	m.RenameToolWindowLayout()
	if len(m.namedLayouts) != 2 || m.namedLayouts[0].Name != "Reviewing" ||
		m.activeNamedLayout != "Review" {
		t.Fatalf("renamed layouts = %+v active=%q", m.namedLayouts, m.activeNamedLayout)
	}

	m.OpenRestoreToolWindowLayout("Reviewing")
	m.OpenDeleteToolWindowLayout()
	m.ChooseToolWindowLayout(0)
	if m.layoutDlg == nil || !m.layoutDlg.confirm {
		t.Fatal("delete did not ask for confirmation")
	}
	m.DeleteToolWindowLayout()
	if len(m.namedLayouts) != 1 || m.namedLayouts[0].Name != "Review" ||
		m.activeNamedLayout != "" {
		t.Fatalf("deleted layouts = %+v active=%q", m.namedLayouts, m.activeNamedLayout)
	}

	m.saveSessionNow()
	s := LoadSession(m.cfg.SessionPath)
	if len(s.NamedLayouts) != 1 || s.NamedLayouts[0].Name != "Review" ||
		s.ActiveNamedLayout != "" {
		t.Fatalf("stored named layouts = %+v active=%q", s.NamedLayouts, s.ActiveNamedLayout)
	}
	if m.Root() != root {
		t.Fatalf("layout test project = %q, want %q", m.Root(), root)
	}
}

func TestNamedToolWindowLayoutDialogRendersAndRestores(t *testing.T) {
	m, _ := layoutModel(t)
	m.namedLayouts = []SavedToolWindowLayout{
		{Name: "Debugging", Layout: Layout{Bottom: []string{"debug"}, Tool: "debug"}},
		{Name: "Review", Layout: Layout{Side: "vcs"}},
	}
	m.activeNamedLayout = "Review"
	m.OpenRestoreToolWindowLayout(nil)
	tester := ui.NewTester(func(c *ui.Context) {
		m.toolWindowLayoutDialog(c, c.Theme())
	}, 700, 500)
	tester.Frame()
	for _, text := range []string{"Restore Layout", "Debugging", "Review", "Restore", "Cancel"} {
		if !tester.HasText(text) {
			t.Fatalf("layout chooser is missing %q: %v", text, tester.Texts())
		}
	}
	if err := tester.Click("Debugging"); err != nil {
		t.Fatal(err)
	}
	if m.layoutDlg != nil || m.activeNamedLayout != "Debugging" ||
		m.primaryTool() != "debug" {
		t.Fatalf("chooser restore: dialog=%+v active=%q primary=%q",
			m.layoutDlg, m.activeNamedLayout, m.primaryTool())
	}
}

func TestNamedLayoutPersistenceIsSharedAcrossProjects(t *testing.T) {
	m, _ := layoutModel(t)
	m.ShowRun()
	m.OpenSaveCurrentLayout()
	m.layoutDlg.name = "Run"
	m.SaveCurrentToolWindowLayout()
	m.saveSessionNow()

	s := LoadSession(m.cfg.SessionPath)
	other, _ := layoutModel(t)
	other.namedLayouts = normalizeSavedToolWindowLayouts(s.NamedLayouts)
	other.activeNamedLayout = normalizeActiveNamedLayout(s.ActiveNamedLayout, other.namedLayouts)
	if !other.RestoreToolWindowLayout("Run") || !other.toolVisible("run") {
		t.Fatalf("named layout was not shared with another project: %+v", other.namedLayouts)
	}
}

func TestLayoutRestoreDefaultUsesOpenIDEAIDAndShortcut(t *testing.T) {
	m, _ := layoutModel(t)
	for _, id := range []string{
		"layout.save",
		"layout.restore",
		"layout.restoreDefault",
		"layout.rename",
		"layout.delete",
	} {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA action %q", id)
		}
	}
	if got := canonicalActionID("view.resetLayout"); got != "layout.restoreDefault" {
		t.Fatalf("view.resetLayout alias = %q", got)
	}
	if got := keymapShortcutLabel("layout.restoreDefault"); got != "Shift+F12" {
		t.Fatalf("layout.restoreDefault shortcut = %q", got)
	}
	if !m.RunAction("layout.restoreDefault") {
		t.Fatal("layout.restoreDefault did not run")
	}
}
