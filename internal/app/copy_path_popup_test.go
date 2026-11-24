package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCopyPathPopupRowsMatchOpenIDEAFlavours(t *testing.T) {
	rows := copyPathPopupRows(
		"/work/demo",
		"/work/demo/src/main/java/com/acme/Main.java",
		7,
	)
	want := []copyPathPopupRow{
		{id: "editor.copyPath", label: "Absolute Path", value: "/work/demo/src/main/java/com/acme/Main.java"},
		{id: "project.copyFileName", label: "File Name", value: "Main.java"},
		{id: "project.copyRelativePath", label: "Path From Content Root", value: "src/main/java/com/acme/Main.java"},
		{id: "project.copySourceRootPath", label: "Path From Source Root", value: "com/acme/Main.java"},
		{id: "editor.copyPathWithLineNumber", label: "Path with line number", value: "src/main/java/com/acme/Main.java:7"},
		{id: "editor.copyReference", label: "Reference", reference: true},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestCopyPathPopupOmitsUnknownSourceRoot(t *testing.T) {
	rows := copyPathPopupRows("/work/demo", "/elsewhere/Main.java", 3)
	for _, row := range rows {
		if row.id == "project.copySourceRootPath" {
			t.Fatalf("loose file received a source-root row: %+v", rows)
		}
	}
	if got := rows[len(rows)-2].value; got != "/elsewhere/Main.java:3" {
		t.Fatalf("loose path with line = %q", got)
	}
}

func TestCopyPathReferenceIgnoresStaleResolution(t *testing.T) {
	m := &Model{}
	state := &copyPathPopupState{
		reference:    copyReferencePreview{loading: true},
		referenceGen: 2,
	}
	m.copyPathPopup = state

	m.finishCopyPathReference(state, 1, editorReferenceResult{text: "stale"})
	if !state.reference.loading || state.reference.value != "" {
		t.Fatalf("stale result changed the preview: %+v", state.reference)
	}

	m.finishCopyPathReference(state, 2, editorReferenceResult{text: "com.acme.Main"})
	if state.reference.loading || state.reference.value != "com.acme.Main" {
		t.Fatalf("current result was not applied: %+v", state.reference)
	}
}

func TestCopyPathReferenceActionAndShortcutAreRegistered(t *testing.T) {
	if got := builtinShortcuts["editor.copyPathReference"]; got != "Cmd+Shift+C" {
		t.Fatalf("Copy Path/Reference shortcut = %q", got)
	}
	found := false
	for _, action := range actionList() {
		if action.ID == "editor.copyPathWithLineNumber" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Copy Path with Line Number is missing from the command registry")
	}
}

func TestCopyPathReferencePopupChoosesNativeRowOffscreen(t *testing.T) {
	originalClipboard := clipboardWriteText
	t.Cleanup(func() { clipboardWriteText = originalClipboard })
	copied := ""
	clipboardWriteText = func(text string) { copied = text }

	m := gotoLineTestModel("class Main {}", 0)
	m.setRoot(filepath.Dir(m.tab().doc.Path()))
	m.postHook = func(fn func()) { fn() }
	if !m.RunCommand("editor.copyPathReference") {
		t.Fatal("Copy Path/Reference action did not run")
	}

	tester := ui.NewTester(func(c *ui.Context) {
		m.copyPathReferencePopup(c, c.Theme())
	}, 760, 520)
	tester.Frame()
	for _, text := range []string{"Copy Path/Reference…", "Absolute Path", "File Name", "Reference"} {
		if !tester.HasText(text) {
			t.Fatalf("Copy Path popup lacks %q: %q", text, tester.Texts())
		}
	}
	if err := tester.Click("File Name"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if m.copyPathPopup != nil {
		t.Fatal("choosing a path row did not dismiss the popup")
	}
	if m.status != "Copied "+filepath.Base(m.tab().doc.Path()) {
		t.Fatalf("copy status = %q", m.status)
	}
	if copied != filepath.Base(m.tab().doc.Path()) {
		t.Fatalf("clipboard = %q", copied)
	}
}
