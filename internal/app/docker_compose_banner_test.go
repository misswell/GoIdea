package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func TestDockerComposeAutoDetectionBannerOffersBothChoices(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "stack.yml")
	const text = "services:\n  web:\n    image: nginx\n"
	tab := &Tab{doc: docbridge.Virtual(path, text)}
	tab.state = &editor.State{
		Buffer: tab.doc.Buffer(), LineHeight: editorLineHeight(13),
	}
	tab.state.Font.Size = 13
	m := &Model{tabs: []*Tab{tab}, active: 0}
	m.setRoot(root)
	tt := workbenchTester(t, m)

	for _, want := range []string{
		"This file was auto-detected as a Docker Compose configuration based on its content",
		"Not a Compose file",
		"Keep as Docker Compose",
	} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("Compose banner is missing %q: %v", want, tt.Texts())
		}
	}
	if err := tt.Click("Keep as Docker Compose"); err != nil {
		t.Fatal(err)
	}
	if got := m.services.docker.composeOverrides[filepath.Clean(path)]; got != "kept" {
		t.Fatalf("saved Compose override = %q", got)
	}
	if dockerComposeAutoDetected(path, text, m.services.docker.composeOverrides) {
		t.Fatal("the dismissed auto-detection banner still qualifies")
	}
}

func TestDockerComposeAutoDetectionSkipsNamedAndConfirmedFiles(t *testing.T) {
	const text = "services:\n  web:\n    image: nginx\n"
	if dockerComposeAutoDetected("/project/compose.yml", text, nil) {
		t.Fatal("a named Compose file must not show the automatic-detection banner")
	}
	for _, status := range []string{"kept", "excluded"} {
		if dockerComposeAutoDetected("/project/stack.yml", text, map[string]string{"/project/stack.yml": status}) {
			t.Fatalf("%s file still shows the automatic-detection banner", status)
		}
	}
	if dockerComposeAutoDetected("/project/stack.yaml", "services: invalid\n", nil) {
		t.Fatal("ordinary YAML must not show the automatic-detection banner")
	}
}
