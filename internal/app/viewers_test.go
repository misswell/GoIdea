package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

func markdownTestModel(t *testing.T, text string) (*Model, *Tab) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "README.md")
	doc := docbridge.Virtual(path, text)
	tab := &Tab{doc: doc, state: &editor.State{Buffer: doc.Buffer()}}
	m := &Model{}
	m.setRoot(filepath.Dir(path))
	m.tabs = []*Tab{tab}
	m.active = 0
	return m, tab
}

func TestMarkdownNativeLayoutActionsRenderOneDocument(t *testing.T) {
	for _, id := range []string{"markdown.editorOnly", "markdown.editorAndPreview", "markdown.previewOnly"} {
		if _, ok := actionGet(id); !ok {
			t.Fatalf("%s is not registered", id)
		}
	}
	m, tab := markdownTestModel(t, "# Release\n\nA **native** preview.\n\n```go\nprintln(\"hello\")\n```\n")
	if !m.RunAction("markdown.editorAndPreview") || tab.markdownMode != markdownEditorPreview {
		t.Fatalf("split mode = %q", tab.markdownMode)
	}
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1200, 760)
	tt.Frame()
	for _, text := range []string{"Release", "A native preview.", `println("hello")`} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from the native split preview: %q", text, tt.Texts())
		}
	}
	if !m.RunAction("markdown.previewOnly") || tab.markdownMode != markdownPreviewOnly {
		t.Fatalf("preview-only mode = %q", tab.markdownMode)
	}
	if !m.RunAction("markdown.editorOnly") || tab.markdownMode != markdownEditorOnly {
		t.Fatalf("editor-only mode = %q", tab.markdownMode)
	}
	if live := markdownPreviewBlocks(tab); len(live) < 3 {
		t.Fatalf("Markdown source did not produce native blocks: %+v", live)
	}
}

func TestDjangoContentDetectsTagsAndOffersNativeFileTypeChoices(t *testing.T) {
	if !isDjangoContent(`<h1>{% block title %}Home{% endblock %}</h1>`) {
		t.Fatal("Django block tags were not detected")
	}
	if isDjangoContent("{{ ordinary_handlebars }}") {
		t.Fatal("plain handlebars-like braces were treated as a Django tag")
	}
	path := filepath.Join(t.TempDir(), "index.html")
	doc := docbridge.Virtual(path, `<h1>{% block title %}Home{% endblock %}</h1>`)
	state := &editor.State{Buffer: doc.Buffer()}
	setDone := make(chan struct{}, 1)
	m := &Model{
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "settings_get":
				return json.RawMessage(`{"files":{"fileTypeOverrides":{}}}`), nil
			case "settings_set":
				args := input.(map[string]any)
				var overrides map[string]string
				if err := json.Unmarshal([]byte(args["value"].(string)), &overrides); err != nil {
					t.Fatalf("decode overrides: %v", err)
				}
				if overrides["index.html"] != "django-html" {
					t.Fatalf("overrides = %#v", overrides)
				}
				return json.RawMessage(`{}`), nil
			default:
				t.Fatalf("command = %q", command)
			}
			return nil, nil
		},
		postHook: func(fn func()) {
			fn()
			setDone <- struct{}{}
		},
	}
	root := filepath.Dir(path)
	m.setRoot(root)
	m.tabs = []*Tab{{doc: doc, state: state}}
	m.active = 0
	// Rendering the workbench also primes Python discovery. Mark that unrelated
	// runtime state as already checked so this test only exercises the Django
	// banner and its file-type override RPC.
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPagePythonInterpret: {loaded: true, scope: "global", root: root},
	}
	m.languageSDKs.status.python = pythonRuntimeStatus{
		root:    root,
		key:     strings.Join([]string{languageSDKPathKey(root), "", ""}, "\x00"),
		gen:     1,
		checked: true,
	}
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1000, 700)
	tt.Frame()
	for _, text := range []string{
		"This file has Django template tags", "Keep as HTML", "Use Django Template",
	} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from the banner: %q", text, tt.Texts())
		}
	}
	for len(setDone) > 0 {
		<-setDone
	}
	m.SetFileTypeOverride(path, "django-html")
	<-setDone
	if m.currentFileTypeOverride(path) != "django-html" {
		t.Fatal("the Django file type choice was not applied")
	}
}

func TestMarkdownActionsFollowOpenIDEAArguments(t *testing.T) {
	m, tab := markdownTestModel(t, "# Title\n")
	otherPath := filepath.Join(filepath.Dir(tab.doc.Path()), "notes.md")
	other := &Tab{doc: docbridge.Virtual(otherPath, "# Notes\n")}
	m.tabs = append(m.tabs, other)
	if !m.RunActionWithArg("markdown.previewOnly", otherPath) {
		t.Fatal("the Markdown action did not accept its path argument")
	}
	if other.markdownMode != markdownPreviewOnly || m.markdownModeFor(tab) != markdownPreviewOnly {
		t.Fatalf("modes = %q %q", tab.markdownMode, other.markdownMode)
	}
	if _, err := os.Stat(otherPath); !os.IsNotExist(err) {
		t.Fatalf("virtual Markdown fixture unexpectedly touched disk: %v", err)
	}
}

func TestImageViewerAndActionsUseTheNativeTab(t *testing.T) {
	for _, name := range []string{"picture.png", "photo.JPG", "anim.gif", "image.webp", "image.bmp", "vector.svg"} {
		if !imageViewerPath(name) {
			t.Fatalf("%s should use the native image viewer", name)
		}
	}
	for _, name := range []string{"icon.ico", "photo.avif"} {
		if imageViewerPath(name) {
			t.Fatalf("%s has no native decoder and should not use the image viewer", name)
		}
	}
	root := t.TempDir()
	path := filepath.Join(root, "pixel.png")
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 240, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	m.setRoot(root)
	m.openFile(path)
	tab := m.tab()
	if tab == nil || tab.kind != tabKindImageViewer || tab.viewer == nil ||
		tab.viewer.width != 2 || tab.viewer.height != 3 {
		t.Fatalf("image tab = %+v", tab)
	}
	for _, id := range []string{
		"image.zoomIn", "image.zoomOut", "image.actualSize",
		"image.fitToWindow", "image.toggleChessboard",
	} {
		if _, ok := actionGet(id); !ok {
			t.Fatalf("%s is not registered", id)
		}
	}
	if !m.RunAction("image.zoomIn") || tab.viewer.zoom <= 0 {
		t.Fatalf("zoom in = %v", tab.viewer.zoom)
	}
	if !m.RunAction("image.actualSize") || tab.viewer.zoom != 1 {
		t.Fatalf("actual size = %v", tab.viewer.zoom)
	}
	if !m.RunAction("image.fitToWindow") || tab.viewer.zoom != 0 {
		t.Fatalf("fit = %v", tab.viewer.zoom)
	}
	if !m.RunAction("image.toggleChessboard") || !tab.viewer.chessboard {
		t.Fatal("chessboard did not toggle")
	}
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1000, 700)
	tt.Frame()
	for _, text := range []string{"pixel.png", "2 × 3", "Fit", "Chessboard"} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from the image viewer: %q", text, tt.Texts())
		}
	}
}

func TestImageViewerChessboardPaintsAlternatingSquares(t *testing.T) {
	light := ui.RGB(255, 255, 255)
	dark := ui.RGB(0, 0, 0)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(24, 24).Draw(func(p *ui.Painter, r ui.Rect) {
			paintImageViewerChessboard(p, r, light, dark)
		})
	}, 24, 24)
	tt.Frame()
	img := tt.Image()
	if got := img.RGBAAt(6, 6); got != (color.RGBA{A: 255}) {
		t.Fatalf("first square = %#v, want black", got)
	}
	if got := img.RGBAAt(18, 6); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("second square = %#v, want white", got)
	}
}

func TestBinaryViewerUsesVirtualizedNativeHexRows(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "memory.bin")
	data := []byte{0, 1, 2, 0x41, 0x42, 0xff}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !isBinaryViewerData(data) {
		t.Fatal("binary data was not detected")
	}
	m := &Model{}
	m.setRoot(root)
	m.openFile(path)
	tab := m.tab()
	if tab == nil || tab.kind != tabKindHexViewer || tab.viewer == nil {
		t.Fatalf("hex tab = %+v", tab)
	}
	offset, hexText, ascii := hexViewerRow(tab.viewer.hex, 0)
	if offset != "00000000" || hexText != "00 01 02 41 42 FF" || ascii != "...AB." {
		t.Fatalf("hex row = %q %q %q", offset, hexText, ascii)
	}
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 1000, 700)
	tt.Frame()
	for _, text := range []string{"memory.bin", "Read-only hex", "00000000", "00 01 02 41 42 FF"} {
		if !tt.HasText(text) {
			t.Fatalf("%q is missing from the hex viewer: %q", text, tt.Texts())
		}
	}
}
