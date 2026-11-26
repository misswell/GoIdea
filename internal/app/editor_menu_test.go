package app

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func TestCopyEditorRichTextProvidesHTMLAndPlainText(t *testing.T) {
	doc := docbridge.Virtual("/Rich.java", `if (x < y) { "ok"; }`)
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(4, 9)
	m := &Model{tabs: []*Tab{{doc: doc, state: state}}, active: 0}
	var written transfer.Data
	original := clipboardWriteData
	originalHTML := clipboardWriteHTML
	t.Cleanup(func() {
		clipboardWriteData = original
		clipboardWriteHTML = originalHTML
	})
	clipboardWriteData = func(data transfer.Data) error {
		written = data
		return nil
	}
	clipboardWriteHTML = func(markup string) {
		t.Fatalf("HTML fallback used after a successful multi-format write: %q", markup)
	}
	if !m.copyEditorRichText(m.tab()) {
		t.Fatal("copyEditorRichText rejected a selection")
	}
	html, err := written.Read(transfer.HTML)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := written.Read(transfer.Text)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "x < y" {
		t.Fatalf("plain text = %q", plain)
	}
	if !strings.Contains(string(html), "x &lt; y") ||
		!strings.HasPrefix(string(html), "<pre") {
		t.Fatalf("HTML = %q", html)
	}
}

func TestCopyEditorRichTextFallsBackToHTML(t *testing.T) {
	doc := docbridge.Virtual("/Rich.java", "value")
	state := &editor.State{Buffer: doc.Buffer()}
	state.Select(0, len("value"))
	m := &Model{tabs: []*Tab{{doc: doc, state: state}}, active: 0}
	var fallback string
	original := clipboardWriteData
	originalHTML := clipboardWriteHTML
	t.Cleanup(func() {
		clipboardWriteData = original
		clipboardWriteHTML = originalHTML
	})
	clipboardWriteData = func(transfer.Data) error { return errors.New("no multi-format clipboard") }
	clipboardWriteHTML = func(markup string) { fallback = markup }
	if !m.copyEditorRichText(m.tab()) {
		t.Fatal("copyEditorRichText rejected a selection")
	}
	if !strings.Contains(fallback, "value") {
		t.Fatalf("fallback HTML = %q", fallback)
	}
}

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
