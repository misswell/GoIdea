package app

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func TestEditorPathFlavours(t *testing.T) {
	got := editorPathFlavours("/work/demo", "/work/demo/src/Main.java", 7)
	want := []editorPathFlavour{
		{"Absolute Path", "/work/demo/src/Main.java"},
		{"File Name", "Main.java"},
		{"Path From Content Root", "src/Main.java"},
		{"Path with line number", "src/Main.java:7"},
	}
	if len(got) != len(want) {
		t.Fatalf("flavours = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("flavour %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestEditorPathFlavoursOutsideRootKeepAbsolute(t *testing.T) {
	got := editorPathFlavours("/work/demo", "/elsewhere/Lib.java", 3)
	if got[2].value != "/elsewhere/Lib.java" {
		t.Errorf("Path From Content Root = %q, want the absolute path", got[2].value)
	}
	if got[3].value != "/elsewhere/Lib.java:3" {
		t.Errorf("Path with line number = %q, want the absolute path and line", got[3].value)
	}
}

func TestTabPathFlavoursCopyInsideAndOutsideTheProject(t *testing.T) {
	got := tabPathFlavours("/work/demo", "/work/demo/src/Main.java")
	want := []editorPathFlavour{
		{"Absolute Path", "/work/demo/src/Main.java"},
		{"File Name", "Main.java"},
		{"Path From Content Root", "src/Main.java"},
	}
	if len(got) != len(want) {
		t.Fatalf("flavours = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("flavour %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// A file outside the project has no content root, so the menu offers only
	// the absolute path and the file name.
	for _, root := range []string{"/work/demo", ""} {
		outside := tabPathFlavours(root, "/elsewhere/Lib.java")
		if len(outside) != 2 {
			t.Fatalf("root %q: flavours = %+v, want absolute and name", root, outside)
		}
		if outside[0].value != "/elsewhere/Lib.java" || outside[1].value != "Lib.java" {
			t.Errorf("root %q: flavours = %+v", root, outside)
		}
	}
}

func TestEditorContextMenuOffersCompareEntryPoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "src", "Main.java")
	doc := docbridge.Virtual(path, "class Main {}\n")
	tab := &Tab{doc: doc, state: &editor.State{
		Buffer: doc.Buffer(),
		Label:  "Main.java",
	}}
	m := &Model{tabs: []*Tab{tab}, active: 0}
	tab.state.ContextMenu = func(menu *ui.Menu) {
		m.editorMenu(menu, tab)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, tab.state).Fill()
	}, 800, 400)
	tester.Frame()
	if err := tester.RightClick("Main.java"); err != nil {
		t.Fatal(err)
	}
	menu := tester.Menu()
	for _, label := range []string{"Compare With…", "Compare with Clipboard"} {
		found := false
		for _, item := range menu {
			if item == label {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("editor context menu %v misses %q", menu, label)
		}
	}
}

func TestHTMLEditorContextMenuOffersBrowserPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web", "index.html")
	doc := docbridge.Virtual(path, "<html></html>\n")
	tab := &Tab{doc: doc, state: &editor.State{
		Buffer: doc.Buffer(),
		Label:  "index.html",
	}}
	m := &Model{tabs: []*Tab{tab}, active: 0}
	tab.state.ContextMenu = func(menu *ui.Menu) {
		m.editorMenu(menu, tab)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		editor.View(c, tab.state).Fill()
	}, 800, 400)
	tester.Frame()
	if err := tester.RightClick("index.html"); err != nil {
		t.Fatal(err)
	}
	menu := tester.Menu()
	if !slices.Contains(menu, "Browser") {
		t.Fatalf("HTML editor context menu %v misses Browser", menu)
	}
}

func TestMainMenuAndTabOpenInOfferBrowserPreview(t *testing.T) {
	found := false
	walkMenu(mainMenuGroups(), func(entry menuEntry) {
		if entry.label == "Built-in Preview" && entry.command == "web.togglePreview" {
			found = true
		}
	})
	if !found {
		t.Fatal("main menu is missing Built-in Preview")
	}

	path := filepath.Join(t.TempDir(), "index.html")
	m := &Model{}
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Button(c, "Open In").Menu(func(menu *ui.Menu) {
			m.tabOpenInMenu(menu, path)
		})
	}, 360, 120)
	tester.Frame()
	if err := tester.Click("Open In"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("Open In", "Browser", "Built-in Preview"); err != nil {
		t.Fatalf("tab Open In›Browser lacks Built-in Preview: %v", err)
	}
	if !m.remote.webPreviews[path] {
		t.Fatal("tab Open In›Browser did not toggle Built-in Preview")
	}
}
