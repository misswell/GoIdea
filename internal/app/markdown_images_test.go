package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestMarkdownImageNamesTargetsAndRelativeLinks(t *testing.T) {
	now := time.Date(2026, time.November, 22, 9, 8, 7, 0, time.Local)
	if got := markdownImageName("", "image-{timestamp}", now); got != "image-20261122-090807.png" {
		t.Fatalf("clipboard image name = %q", got)
	}
	if got := markdownImageName(`C:\shots\from source.png`, "ignored", now); got != "from source.png" {
		t.Fatalf("source image name = %q", got)
	}
	root := t.TempDir()
	markdown := filepath.Join(root, "docs", "guide.md")
	target, err := markdownImageTarget(markdown, "../images", "shot image.png")
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join(root, "docs", "../images", "shot image.png") {
		t.Fatalf("target = %q", target)
	}
	link, err := markdownImageLink(markdown, target)
	if err != nil || link != "![](../images/shot%20image.png)" {
		t.Fatalf("link = %q, %v", link, err)
	}
	if _, err := markdownImageTarget(markdown, filepath.Join(root, "absolute"), "x.png"); err == nil {
		t.Fatal("an absolute image folder was accepted")
	}
}

func TestMarkdownImagesInsertAtTheSelectionAndKeepSourceNames(t *testing.T) {
	dir := t.TempDir()
	markdown := filepath.Join(dir, "notes", "guide.md")
	if err := os.MkdirAll(filepath.Dir(markdown), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := &fileActionBackend{info: openidea.DocInfo{
		ID: 51, Path: markdown, Text: "# Guide\n\n", Version: 1,
		Encoding: "UTF-8", LineEnding: "LF", URI: "file://" + markdown,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := docbridge.NewRegistry(backend, Window)
	doc, err := registry.Open(ctx, markdown)
	if err != nil {
		t.Fatal(err)
	}
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	tab.state.Select(len(tab.state.Buffer.String()), len(tab.state.Buffer.String()))
	m := &Model{
		ctx: ctx, reg: registry,
		cfg:  Config{SettingsPath: filepath.Join(dir, "settings.json")},
		tabs: []*Tab{tab}, active: 0,
		prefs: Preferences{General: GeneralPrefs{MarkdownImageFolder: "assets"}},
	}
	if !m.insertMarkdownImageSources(tab, []markdownImageSource{{name: "screen shot.png", data: []byte("png")}}) {
		t.Fatal("the Markdown image was not inserted")
	}
	if got := doc.Buffer().String(); !strings.Contains(got, "![](assets/screen%20shot.png)") {
		t.Fatalf("inserted Markdown = %q", got)
	}
	image := filepath.Join(dir, "notes", "assets", "screen shot.png")
	if got, err := os.ReadFile(image); err != nil || string(got) != "png" {
		t.Fatalf("saved image = %q, %v", got, err)
	}

	// A same-named drop gets OpenIDEA's numeric suffix instead of replacing the
	// image that is already on disk.
	source := filepath.Join(dir, "screen shot.png")
	if err := os.WriteFile(source, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.dropMarkdownImages(tab, []string{source}) {
		t.Fatal("the dropped image was not inserted")
	}
	if got := doc.Buffer().String(); !strings.Contains(got, "![](assets/screen%20shot-1.png)") {
		t.Fatalf("dropped Markdown = %q", got)
	}
}
