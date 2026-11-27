package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"mayoi/internal/docbridge"

	"github.com/egoist/mygo/ui/editor"
)

func TestProjectStructureActionRoutesSectionsThroughTheNativeSheet(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	path := filepath.Join(root, "pom.xml")
	doc := docbridge.Virtual(path, "<project/>")
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{
		ctx:    context.Background(),
		tabs:   []*Tab{tab},
		active: 0,
		postHook: func(fn func()) {
			fn()
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			switch command {
			case "settings_get":
				return json.RawMessage(`{
					"projectStructure": {
						"modules": {".": {"jdk":"","languageLevel":"","compilerOutput":""}}
					},
					"languages": {"java": {"jdk":"","languageLevel":"","jdtlsJdk":""}}
				}`), nil
			case "languages", "project_structure_detect":
				return json.RawMessage(`[]`), nil
			default:
				return json.RawMessage(`null`), nil
			}
		},
	}
	m.setRoot(root)
	if !m.RunAction("file.projectStructure") {
		t.Fatal("file.projectStructure is not registered")
	}
	if !m.projectStructure.open ||
		m.projectStructure.section != projectStructureProject ||
		!m.projectStructure.loading {
		t.Fatalf("Project Structure did not open its native sheet: %+v", m.projectStructure)
	}
	if !m.RunActionWithArg("file.projectStructure", projectStructureModules) {
		t.Fatal("Project Structure rejected its section argument")
	}
	if m.projectStructure.section != projectStructureModules {
		t.Fatalf("Project Structure section = %q, want modules", m.projectStructure.section)
	}
	if !m.RunActionWithArg("file.projectStructure", "unknown") ||
		m.projectStructure.section != projectStructureProject {
		t.Fatalf("unknown Project Structure section = %q, want project", m.projectStructure.section)
	}
	m.CloseProjectStructure()
	if m.projectStructure.open {
		t.Fatal("Project Structure did not close")
	}
}
