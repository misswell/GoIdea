package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestQuickDefinitionUTF16IdentifierAndSnippet(t *testing.T) {
	source := "before\nclass Type {\n}\nafter\n"
	location := openidea.Location{
		Path: "/tmp/Example.java",
		Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 1, Col: 6},
			End:   openidea.OutlinePos{Line: 3, Col: 0},
		},
	}
	target := quickDefinitionTargetFromSource(location, quickDefinitionSource{
		title: "Example.java", language: "java", uri: location.Path, text: source,
	}, "java")
	if target.name != "Type" {
		t.Fatalf("target name = %q", target.name)
	}
	if target.firstLine != 0 || !strings.Contains(target.text, "class Type") ||
		!strings.Contains(target.text, "after") {
		t.Fatalf("target span = %d %q", target.firstLine, target.text)
	}
}

func TestQuickDefinitionLoadsAndRendersMultipleTargets(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "First.java")
	secondPath := filepath.Join(root, "Second.java")
	if err := os.WriteFile(firstPath, []byte("before\nclass First {}\nafter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("interface Second {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := gotoLineTestModel("First value;", 7)
	m.setRoot(root)
	m.ctx = context.Background()
	done := make(chan struct{})
	m.postHook = func(fn func()) {
		fn()
		close(done)
	}
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		raw, err := json.Marshal([]openidea.Location{
			{
				Path: firstPath,
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 1, Col: 6},
					End:   openidea.OutlinePos{Line: 2},
				},
			},
			{
				Path: secondPath,
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 0, Col: 10},
					End:   openidea.OutlinePos{Line: 0},
				},
			},
			{
				Path: firstPath,
				Range: openidea.OutlineRange{
					Start: openidea.OutlinePos{Line: 1, Col: 6},
					End:   openidea.OutlinePos{Line: 2},
				},
			},
		})
		return raw, err
	}
	m.QuickDefinition("definition")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Quick Definition did not finish")
	}
	if m.quickDefinition == nil || len(m.quickDefinition.targets) != 2 {
		t.Fatalf("Quick Definition state = %+v", m.quickDefinition)
	}
	if m.quickDefinition.targets[0].name != "First" ||
		m.quickDefinition.targets[1].name != "Second" {
		t.Fatalf("target names = %+v", m.quickDefinition.targets)
	}

	tester := ui.NewTester(func(c *ui.Context) {
		m.quickDefinitionPopup(c, c.Theme())
	}, 1100, 700)
	tester.Frame()
	text := strings.Join(tester.Texts(), "\n")
	if !strings.Contains(text, "Definition") || !strings.Contains(text, "First.java") ||
		!strings.Contains(text, "class First") || !strings.Contains(text, "1/2") {
		t.Fatalf("Quick Definition popup text = %q", text)
	}
	m.stepQuickDefinition(1)
	tester.Frame()
	text = strings.Join(tester.Texts(), "\n")
	if !strings.Contains(text, "Second.java") || !strings.Contains(text, "interface Second") ||
		!strings.Contains(text, "2/2") {
		t.Fatalf("next Quick Definition target = %q", text)
	}
}

func TestQuickDefinitionMissingTargetShowsOpenIDEAMessage(t *testing.T) {
	m := gotoLineTestModel("value", 0)
	m.setRoot(t.TempDir())
	m.ctx = context.Background()
	done := make(chan struct{})
	m.postHook = func(fn func()) {
		fn()
		close(done)
	}
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage("[]"), nil
	}
	m.QuickDefinition("typeDefinition")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Quick Type Definition did not finish")
	}
	if m.quickDefinition != nil || m.status != "No type definition found" {
		t.Fatalf("missing target state = %+v, status %q", m.quickDefinition, m.status)
	}
}
