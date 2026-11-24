package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func projectNewTemplateIndex(t *testing.T, id string) int {
	t.Helper()
	for i := range projectNewTemplates {
		if projectNewTemplates[i].id == id {
			return i
		}
	}
	t.Fatalf("missing file template %q", id)
	return -1
}

func TestProjectNewTemplateSearchAndOpenState(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.OpenProjectNew(root)
	if m.projectNew == nil || !m.projectNew.open || m.treeOp != nil {
		t.Fatalf("New popup state = %+v, %+v", m.projectNew, m.treeOp)
	}
	if len(m.projectNew.matches) != len(projectNewEntries()) {
		t.Fatalf("empty search matched %d entries, want %d",
			len(m.projectNew.matches), len(projectNewEntries()))
	}
	if o := overlays.byID["project.newPopup"]; o == nil || o.Modal || !o.Open(m) {
		t.Fatalf("project.newPopup overlay = %+v", o)
	}
	if o := overlays.byID["templates.newFile"]; o == nil || o.Open(m) {
		t.Fatalf("templates.newFile should stay closed until a kind is chosen: %+v", o)
	}

	m.projectNew.query = "docker"
	m.refreshProjectNewMatches()
	if len(m.projectNew.matches) == 0 ||
		projectNewTemplates[m.projectNew.matches[0].entry.templateIndex].id != "dockerfile" {
		t.Fatalf("docker search = %v", m.projectNew.matches)
	}

	m.projectNew.query = "not-a-real-template-zzz"
	m.refreshProjectNewMatches()
	if len(m.projectNew.matches) != 0 {
		t.Fatalf("unrelated search matched %v", m.projectNew.matches)
	}

	m.projectNew.query = ""
	m.refreshProjectNewMatches()
	java := projectNewTemplateIndex(t, "java")
	m.SelectProjectNewTemplate(java)
	if m.projectNew != nil || m.treeOp == nil || m.treeOp.projectTemplate == nil ||
		m.treeOp.projectTemplate.id != "java" {
		t.Fatalf("selected template state = %+v, %+v", m.projectNew, m.treeOp)
	}
	if o := overlays.byID["templates.newFile"]; o == nil || !o.Modal || !o.Open(m) {
		t.Fatalf("templates.newFile overlay = %+v", o)
	}
	m.treeOp.name = "customer-record"
	m.refreshProjectNewTarget(m.treeOp)
	if got := filepath.Base(m.treeOp.projectTarget); got != "CustomerRecord.java" {
		t.Fatalf("selected Java target = %q", got)
	}
	m.CloseTreeOp()
	if m.projectNew != nil || m.treeOp != nil {
		t.Fatalf("close left chooser or prompt open: %+v, %+v", m.projectNew, m.treeOp)
	}
}

func TestProjectNewPopupRendersOpenIDEAFilterAndActions(t *testing.T) {
	root := t.TempDir()
	m := &Model{}
	m.OpenProjectNewAt(root, ui.Rect{X: 24, Y: 80, W: 320, H: 24})
	tester := ui.NewTester(func(c *ui.Context) {
		m.projectNewDialog(c, c.Theme())
	}, 900, 640)
	tester.Frame()
	for _, want := range []string{
		"File", "Directory", "Scratch File", "Python File", "Stylesheet", "HTTP Request", "From Template…",
	} {
		if !tester.HasText(want) {
			t.Errorf("New popup lacks %q: %q", want, tester.Texts())
		}
	}

	m.projectNew.query = "py"
	m.refreshProjectNewMatches()
	if got := projectNewMatchLabels(m.projectNew.matches); len(got) != 1 || got[0] != "Python File" {
		t.Fatalf("py filter = %v", got)
	}
	m.projectNew.query = "css"
	m.refreshProjectNewMatches()
	wantCSS := []string{"PHP Class", "Java Class", "Kotlin Class/File", "Stylesheet"}
	if got := projectNewMatchLabels(m.projectNew.matches); !slices.Equal(got, wantCSS) {
		t.Fatalf("css filter = %v", got)
	}
	m.projectNew.query = "http"
	m.refreshProjectNewMatches()
	if got := projectNewMatchLabels(m.projectNew.matches); len(got) != 1 || got[0] != "HTTP Request" {
		t.Fatalf("http filter = %v", got)
	}
	m.projectNew.query = "zzz"
	m.refreshProjectNewMatches()
	tester.Frame()
	if got := projectNewMatchLabels(m.projectNew.matches); len(got) != 0 {
		t.Fatalf("zzz filter = %v", got)
	}
	if !tester.HasText("Nothing found") {
		t.Fatalf("empty typed popup = %q", tester.Texts())
	}

	m.projectNew.query = ""
	m.refreshProjectNewMatches()
	var directory projectNewMatch
	for _, match := range m.projectNew.matches {
		if match.entry.action == "directory" {
			directory = match
			break
		}
	}
	if directory.entry.label != "Directory" {
		t.Fatalf("missing Directory row: %+v", m.projectNew.matches)
	}
	m.chooseProjectNewMatch(directory)
	if m.projectNew != nil || m.treeOp == nil || m.treeOp.kind != treeOpNewDir ||
		m.treeOp.path != root {
		t.Fatalf("Directory action = popup %+v, treeOp %+v", m.projectNew, m.treeOp)
	}
	m.CloseTreeOp()
	m.OpenProjectNew(root)
	var scratch projectNewMatch
	for _, match := range m.projectNew.matches {
		if match.entry.action == "scratch" {
			scratch = match
			break
		}
	}
	if scratch.entry.label != "Scratch File" {
		t.Fatalf("missing Scratch File row: %+v", projectNewMatchLabels(m.projectNew.matches))
	}
	m.chooseProjectNewMatch(scratch)
	if m.projectNew != nil || m.scratch == nil || m.scratch.languages == nil {
		t.Fatalf("Scratch action did not open the chooser: popup %+v chooser %+v", m.projectNew, m.scratch)
	}
}

func projectNewMatchLabels(matches []projectNewMatch) []string {
	labels := make([]string, 0, len(matches))
	for _, match := range matches {
		labels = append(labels, match.entry.label)
	}
	return labels
}

func TestProjectNewGenericKeywordsDoNotMatchEveryTemplate(t *testing.T) {
	m := &Model{}
	m.OpenProjectNewAt(t.TempDir(), ui.Rect{})
	for _, query := range []string{"new", "create", "template", "file"} {
		m.projectNew.query = query
		m.refreshProjectNewMatches()
		for _, match := range m.projectNew.matches {
			if _, _, ok := fuzzySubsequence(query, match.entry.label); !ok {
				t.Fatalf("%q matched %q only through a generic keyword", query, match.entry.label)
			}
		}
	}
}

func TestProjectNewPopupSeparatesTemplatesFromActions(t *testing.T) {
	m := &Model{}
	m.OpenProjectNewAt(t.TempDir(), ui.Rect{})
	m.projectNew.query = ""
	m.refreshProjectNewMatches()
	if len(m.projectNew.matches) < 4 {
		t.Fatalf("New entries = %v", projectNewMatchLabels(m.projectNew.matches))
	}
	if m.projectNew.matches[3].entry.action != "template" || !m.projectNew.matches[3].entry.separator {
		t.Fatalf("template separator row = %+v", m.projectNew.matches[3])
	}
	last := m.projectNew.matches[len(m.projectNew.matches)-1]
	if last.entry.action != "fromTemplate" || !last.entry.separator {
		t.Fatalf("From Template separator row = %+v", last)
	}
}

func TestProjectNewPopupUsesCenteredFallbackAnchor(t *testing.T) {
	m := &Model{}
	m.OpenProjectNewAt(t.TempDir(), ui.Rect{})
	tester := ui.NewTester(func(c *ui.Context) {
		m.projectNewDialog(c, c.Theme())
	}, 900, 640)
	tester.Frame()
	popup, ok := tester.Find("New")
	if !ok {
		t.Fatalf("New popup role label is missing: %q", tester.Texts())
	}
	if popup.X != 310 || popup.Y != 128 || popup.W != 280 {
		t.Fatalf("fallback popup bounds = %+v, want x=310 y=128 w=280", popup)
	}
}

func TestProjectNewFileNamePlanning(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		kind      int
		directory string
		input     string
		wantDir   string
		wantName  string
		wantFile  string
	}{
		{
			name: "Go typed extension and snake case", template: "go", kind: 0,
			directory: "/project/pkg", input: "HTTP-Server.go",
			wantDir: "/project/pkg", wantName: "http_server", wantFile: "http_server.go",
		},
		{
			name: "TypeScript typed extension", template: "typescript", kind: 0,
			directory: "/project/src", input: "feature-name.ts",
			wantDir: "/project/src", wantName: "feature-name", wantFile: "feature-name.ts",
		},
		{
			name: "Dotted JVM package", template: "java", kind: 0,
			directory: "/project/src/main/java", input: "com.acme.HelloWorld.java",
			wantDir: "/project/src/main/java/com/acme", wantName: "HelloWorld", wantFile: "HelloWorld.java",
		},
		{
			name: "Kind extension override", template: "stylesheet", kind: 1,
			directory: "/project", input: "theme.scss",
			wantDir: "/project", wantName: "theme", wantFile: "theme.scss",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := projectNewTemplateIndex(t, tt.template)
			plan, err := projectNewPlanFile(&projectNewTemplates[index], tt.kind, tt.directory, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if plan.directory != tt.wantDir || plan.name != tt.wantName || plan.fileName != tt.wantFile {
				t.Fatalf("plan = %+v", plan)
			}
			if want := filepath.Join(tt.wantDir, tt.wantFile); plan.path != want {
				t.Fatalf("path = %q, want %q", plan.path, want)
			}
		})
	}

	if _, err := projectNewPlanFile(&projectNewTemplates[projectNewTemplateIndex(t, "java")], 0,
		"/project", "../Escape"); err == nil {
		t.Fatal("parent traversal in a template name was accepted")
	}
}

func TestProjectNewPackageInference(t *testing.T) {
	root := t.TempDir()

	javaDir := filepath.Join(root, "app", "src", "main", "java", "com", "acme")
	if err := os.MkdirAll(javaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := projectNewInferJVMPackage(javaDir, root); got != "com.acme" {
		t.Fatalf("JVM package = %q", got)
	}

	goDir := filepath.Join(root, "pkg", "server")
	if err := os.MkdirAll(goDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goDir, "server.go"), []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := projectNewInferGoPackage(goDir, root); got != "api" {
		t.Fatalf("Go package = %q", got)
	}
	cmdDir := filepath.Join(root, "cmd", "tool")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := projectNewInferGoPackage(cmdDir, root); got != "main" {
		t.Fatalf("Go command package = %q", got)
	}

	composer := `{"autoload":{"psr-4":{"App\\":"src/"}}}`
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(composer), 0o644); err != nil {
		t.Fatal(err)
	}
	phpDir := filepath.Join(root, "src", "Feature", "Http")
	if err := os.MkdirAll(phpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := projectNewInferPHPNamespace(phpDir, root); got != `App\Feature\Http` {
		t.Fatalf("PHP namespace = %q", got)
	}
}

func TestFileTemplateIncludesAndCaret(t *testing.T) {
	body := "func before() {}\n#parse(\"Go Test Function\")"
	text, caret := renderFileTemplate(body, projectNewVars{name: "Widget", now: time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)})
	marker := "func before() {}\nfunc TestWidget(t *testing.T) {\n\t"
	wantCaret := strings.Index(text, marker) + len(marker)
	if wantCaret < len(marker) || caret != wantCaret {
		t.Fatalf("caret = %d, want %d in %q", caret, wantCaret, text)
	}
	if strings.Contains(text, "$END$") || strings.Contains(text, "#parse") {
		t.Fatalf("template directives remained in %q", text)
	}
	line, col := projectNewCaretPosition(text, caret)
	if line != 2 || col != 1 {
		t.Fatalf("caret position = %d:%d", line, col)
	}
}

func TestFileTemplateVariablesAndEmptyPackage(t *testing.T) {
	body := "package $PACKAGE$;\n\nclass $NAME$ {\n    $END$\n}\n"
	text, _ := renderFileTemplate(body, projectNewVars{name: "Widget"})
	want := "class Widget {\n    \n}\n"
	if text != want {
		t.Fatalf("empty package render = %q, want %q", text, want)
	}
	text, _ = renderFileTemplate(body, projectNewVars{name: "Widget", pkg: "com.acme"})
	if !strings.Contains(text, "package com.acme;") || !strings.Contains(text, "class Widget") {
		t.Fatalf("package render = %q", text)
	}
}

func TestFileTemplateCreateUsesBytesRPC(t *testing.T) {
	root := t.TempDir()
	template := projectNewTemplate{
		id: "test", name: "Test", nameStyle: "asis",
		kinds: []projectNewKind{{id: "file", label: "File", body: "BEGIN $NAME$\n$END$END"}},
	}
	var command string
	var args map[string]any
	finished := make(chan struct{})
	m := &Model{
		ctx: context.Background(),
		postHook: func(fn func()) {
			fn()
			close(finished)
		},
		invokeHook: func(_ context.Context, cmd string, input any) (json.RawMessage, error) {
			command = cmd
			args, _ = input.(map[string]any)
			return nil, errors.New("stop")
		},
	}
	m.setRoot(root)
	m.projectName = "Demo"
	m.treeOp = &treeOpState{
		kind: treeOpNewFile, path: root, dir: true, name: "notes",
		open: true, projectTemplate: &template,
	}
	m.ConfirmTreeOp()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("template creation did not finish")
	}
	if command != "fs_create_file_bytes" {
		t.Fatalf("command = %q", command)
	}
	if got := args["path"]; got != filepath.Join(root, "notes") {
		t.Fatalf("path argument = %#v", got)
	}
	values, ok := args["data"].([]any)
	if !ok {
		t.Fatalf("data argument = %#v", args["data"])
	}
	data := make([]byte, len(values))
	for i, value := range values {
		n, ok := value.(int)
		if !ok {
			t.Fatalf("data byte %d = %#v", i, value)
		}
		data[i] = byte(n)
	}
	if got, want := string(data), "BEGIN notes\nEND"; got != want {
		t.Fatalf("template data = %q, want %q", got, want)
	}
	if m.treeOp == nil || m.treeOp.err != "The operation failed: stop" {
		t.Fatalf("template error state = %+v", m.treeOp)
	}
}
