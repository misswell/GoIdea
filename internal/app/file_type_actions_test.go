package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func TestFileTypeOverrideActionsAndProjectWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "notes.txt")
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	m.postHook = func(fn func()) { fn() }
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "settings_get":
			return json.RawMessage(`{"files":{"fileTypeOverrides":{"src/existing.txt":"go"}}}`), nil
		case "settings_set":
			request := args.(map[string]any)
			mu.Lock()
			writes = append(writes, capturedSettingsWrite{
				scope: request["scope"].(string),
				key:   request["key"].(string),
				value: request["value"].(string),
			})
			mu.Unlock()
			return json.RawMessage(`null`), nil
		default:
			return json.RawMessage(`null`), nil
		}
	}

	action, ok := actionGet("file.overrideFileType")
	if !ok || action.RunWithArg == nil {
		t.Fatal("file.overrideFileType is not registered")
	}
	if argPath, argLanguage := fileTypeOverrideArgument(map[string]any{"path": path, "language": "markdown"}); argPath != path || argLanguage != "markdown" {
		t.Fatalf("action argument = %q, %q", argPath, argLanguage)
	}
	if !m.RunActionWithArg("file.overrideFileType", map[string]any{"path": path, "language": "markdown"}) {
		t.Fatal("file.overrideFileType could not run")
	}
	waitSettingsBackendTest(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(writes) == 1
	})
	if m.status != "File type overridden" {
		t.Fatalf("override status = %q", m.status)
	}

	mu.Lock()
	if len(writes) != 1 {
		t.Fatalf("settings writes = %+v", writes)
	}
	write := writes[0]
	mu.Unlock()
	if write.scope != "project" || write.key != "files.fileTypeOverrides" {
		t.Fatalf("override write = %+v", write)
	}
	var overrides map[string]string
	if err := json.Unmarshal([]byte(write.value), &overrides); err != nil {
		t.Fatal(err)
	}
	if overrides["src/existing.txt"] != "go" || overrides["src/notes.txt"] != "markdown" {
		t.Fatalf("stored overrides = %+v", overrides)
	}
	if got := m.currentFileTypeOverride(path); got != "markdown" {
		t.Fatalf("runtime override = %q", got)
	}

	revert, ok := actionGet("file.revertFileTypeOverride")
	if !ok || revert.RunWithArg == nil {
		t.Fatal("file.revertFileTypeOverride is not registered")
	}
	if !m.RunActionWithArg("file.revertFileTypeOverride", map[string]string{"path": path}) {
		t.Fatal("file.revertFileTypeOverride could not run")
	}
	waitSettingsBackendTest(t, func() bool { return m.status == "File type override reverted" })
	if got := m.currentFileTypeOverride(path); got != "" {
		t.Fatalf("override survived revert: %q", got)
	}
}

func TestFileTypeMenuUsesBackendLanguagesAndCurrentOverride(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.setRoot(root)
	m.fileTypesSettings.languages = []openIDEALanguageInfo{
		{ID: "go", Name: "Go"},
		{ID: "markdown", Name: "Markdown"},
	}
	m.fileTypesSettings.runtimeLoaded = true
	m.fileTypesSettings.runtime.FileTypeOverrides = map[string]string{"notes.txt": "markdown"}
	if got := m.fileTypeMenuLanguages(); len(got) != 2 || got[1].ID != "markdown" {
		t.Fatalf("language menu = %+v", got)
	}
	if got := m.currentFileTypeOverride(filepath.Join(root, "notes.txt")); got != "markdown" {
		t.Fatalf("current override = %q", got)
	}
}

func TestFileTypeOverrideMenuRendersInProjectAndEditorContexts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := docbridge.Virtual(path, "note\n")
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer(), Label: "notes.txt"}}
	m := &Model{active: -1}
	m.setRoot(root)
	rootNode := &treeNode{path: root, name: filepath.Base(root), dir: true, expanded: true}
	fileNode := &treeNode{path: path, name: "notes.txt", depth: 1}
	m.tree = &Tree{
		root:    rootNode,
		rows:    []*treeNode{rootNode, fileNode},
		open:    map[string]bool{root: true},
		sels:    map[string]bool{path: true},
		sel:     path,
		anchor:  path,
		options: defaultTreeOptions(),
	}
	m.fileTypesSettings.languages = []openIDEALanguageInfo{
		{ID: "go", Name: "Go"},
		{ID: "markdown", Name: "Markdown"},
	}
	m.fileTypesSettings.runtimeLoaded = true
	m.fileTypesSettings.runtime.FileTypeOverrides = map[string]string{"notes.txt": "markdown"}

	workbench := workbenchTester(t, m)
	workbench.Frame()
	if err := workbench.RightClick("notes.txt"); err != nil {
		t.Fatal(err)
	}
	assertMenuLabels(t, workbench.Menu(), "Override File Type", "Revert File Type Override")
	workbench.CloseMenu()

	m.tabs = []*Tab{tab}
	m.active = 0
	tabTester := ui.NewTester(func(c *ui.Context) {
		ui.Button(c, "Tab").Menu(func(menu *ui.Menu) { m.tabMenu(menu, tab) })
	}, 800, 400)
	tabTester.Frame()
	if err := tabTester.Click("Tab"); err != nil {
		t.Fatal(err)
	}
	assertMenuLabels(t, tabTester.Menu(), "Override File Type", "Revert File Type Override")
}

func TestNewTabUsesTheLoadedExactPathFileTypeOverride(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	m := &Model{}
	m.setRoot(root)
	m.fileTypesSettings.runtimeLoaded = true
	m.fileTypesSettings.runtime.FileTypeOverrides = map[string]string{"notes.txt": "markdown"}

	tab := m.newTab(docbridge.Virtual(path, "# heading\n"))
	if language := tab.state.Buffer.Language(); language == nil || language.Name != "Markdown" {
		t.Fatalf("new tab language = %#v", language)
	}
}
