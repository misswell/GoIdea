package main

import (
	"bytes"
	"encoding/xml"
	"image/png"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

func testMermaidStyle() mermaidStyle {
	p := darkPalette()
	return mermaidStyle{
		BG: p.editor, FG: p.text, Muted: p.muted, Line: p.muted,
		Surface: p.surface, SurfaceAlt: p.surfaceHover,
		Border: p.border, Accent: p.accent, Danger: p.danger,
		Mono: "monospace", MonoWeight: 400,
	}
}

const mermaidMarkdown = "# Diagrams\n\n```mermaid\nflowchart LR\n  A[Write **Markdown**] --> B{Has a diagram?}\n  B -- yes --> C[Draw it]\n  B -- no --> D[Show text]\n  C --> E((Done))\n```\n\n```mermaid\nsequenceDiagram\n  participant E as Editor\n  participant P as Pele\n  E->>P: render(source)\n  P-->>E: SVG\n  Note over E,P: labels drawn by Mayoi\n```\n\n```mermaid\nsequenceDiagram\n  A->>B hi\n```\n"

// TestMermaidPreview: a Markdown preview draws its blocks of Mermaid as
// diagrams, says why one cannot be drawn, shows one's source on asking,
// and shows them all as source with previewMermaid off.
func TestMermaidPreview(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("diagrams.md")
	os.WriteFile(path, []byte(mermaidMarkdown), 0o644)
	a.openFile(path, true)
	a.run("view.togglePreview")
	d := a.activeEditor()
	hs.until("diagrams", func() bool { return d.preview != nil && d.preview.loaded })
	hs.settle()
	m := d.preview.markdown
	var diagrams []markdownBlock
	for _, b := range m.blocks {
		if b.diagram != nil {
			diagrams = append(diagrams, b)
		}
	}
	if len(diagrams) != 3 || diagrams[0].diagram.Err != "" || diagrams[1].diagram.Err != "" || diagrams[2].diagram.Err == "" {
		t.Fatalf("diagrams %+v", diagrams)
	}
	hs.shot("mermaid-markdown")
	if !hs.tt.HasText(locales.MermaidCannotDraw(diagrams[2].diagram.Err)) {
		t.Error("no error under the diagram that cannot be drawn")
	}
	if hs.tt.HasText("flowchart LR") {
		t.Error("the source of a diagram shows")
	}
	m.sources = map[int]bool{diagrams[0].line: true}
	hs.settle()
	if !hs.tt.HasText("flowchart LR") {
		t.Error("the source asked for does not show")
	}

	a.settings.PreviewMermaid = false
	hs.until("source", func() bool {
		for _, b := range d.preview.markdown.blocks {
			if b.diagram != nil {
				return false
			}
		}
		return true
	})
}

// TestMermaidFilePreview: a .mmd file previews as its diagram, drawn
// again as it is edited.
func TestMermaidFilePreview(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	path := hs.path("flow.mmd")
	os.WriteFile(path, []byte("stateDiagram-v2\n  [*] --> Idle\n  Idle --> Busy: start\n  Busy --> Idle: done\n  Busy --> [*]\n"), 0o644)
	a.openFile(path, true)
	a.run("view.togglePreview")
	d := a.activeEditor()
	hs.until("diagram", func() bool { return d.preview != nil && d.preview.diagram != nil })
	hs.settle()
	hs.shot("mermaid-file")
	first := d.preview.diagram
	d.buf.Replace(d.buf.Len(), d.buf.Len(), "  Idle --> Off: stop\n")
	hs.until("drawn again", func() bool { return d.preview.diagram != first })
	if d.preview.err != "" {
		t.Fatal(d.preview.err)
	}

	// It copies as an image, and exports as PNG and SVG.
	hs.until("drawn", func() bool { return d.preview.diagram != nil && !d.preview.drawing && d.preview.wait == nil })
	a.run("mermaid.copyImage")
	if !bytes.HasPrefix(hs.h.image, []byte("\x89PNG")) {
		t.Fatalf("copied %d bytes, not a PNG", len(hs.h.image))
	}
	for _, ext := range []string{"png", "svg"} {
		hs.h.saveTo = hs.path("out." + ext)
		a.run("mermaid.export" + strings.ToUpper(ext))
		if !slices.Contains(hs.h.asked, "export flow."+ext) {
			t.Errorf("asked %q", hs.h.asked)
		}
		if b, err := os.ReadFile(hs.h.saveTo); err != nil || len(b) == 0 {
			t.Errorf("exported %s: %v", ext, err)
		}
	}
}

// TestMermaidExport: a diagram copied or exported is drawn as it shows,
// on the theme's background, its SVG whole, with its labels as text.
func TestMermaidExport(t *testing.T) {
	st := testMermaidStyle()
	d := renderMermaid("flowchart LR\nA[Start] --> B{Ok?}\nB -- yes --> C[Done]\nstyle C fill:#fff", st)
	if d.Err != "" {
		t.Fatal(d.Err)
	}
	b, err := mermaidPNG(d)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	w, h := int(math.Ceil(float64(d.W+2*mermaidExportPad))), int(math.Ceil(float64(d.H+2*mermaidExportPad)))
	if got := img.Bounds().Size(); got.X != w*mermaidExportScale || got.Y != h*mermaidExportScale {
		t.Fatalf("PNG %v, want %dx%d", got, w*mermaidExportScale, h*mermaidExportScale)
	}
	if r, g, bl, _ := img.At(1, 1).RGBA(); uint8(r>>8) != st.BG.R || uint8(g>>8) != st.BG.G || uint8(bl>>8) != st.BG.B {
		t.Errorf("corner %v, want the background %v", img.At(1, 1), st.BG)
	}
	svg := string(mermaidExportSVG(d))
	for _, want := range []string{">Start</text>", ">Done</text>", `fill="` + hexColor(st.BG) + `"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG has no %s", want)
		}
	}
	if strings.Contains(svg, "var(") {
		t.Error("SVG keeps CSS variables")
	}
	// Done's label reads on its white fill in the dark theme.
	if !strings.Contains(svg, `fill="`+hexColor(st.BG)+`">Done</text>`) {
		t.Errorf("Done drawn in another color than the background: %s", svg)
	}
	if _, err := ui.ParseSVG([]byte(svg)); err != nil {
		t.Errorf("SVG does not parse: %v", err)
	}
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("SVG is not XML: %v", err)
		}
	}
}

// TestMermaidOverlay: a diagram of a Markdown preview opens over the
// window from its button or a double click, zooms there, closes with
// Escape, and closes as its block stops drawing a diagram.
func TestMermaidOverlay(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("diagrams.md")
	os.WriteFile(path, []byte(mermaidMarkdown), 0o644)
	a.openFile(path, true)
	a.run("view.togglePreview")
	d := a.activeEditor()
	hs.until("diagrams", func() bool { return d.preview != nil && d.preview.loaded })
	hs.settle()
	box, ok := hs.tt.Find("mermaid")
	if !ok {
		t.Fatal("no diagram shows")
	}
	x, y := box.X+box.W/2, box.Y+box.H/2
	hs.tt.Move(x, y)
	hs.settle()
	if err := hs.tt.Click(locales.OpenDiagram()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	o := a.diagram
	if o == nil || !hs.tt.HasText(locales.CloseDiagram()) {
		t.Fatal("the diagram did not open over the window")
	}
	hs.shot("mermaid-overlay")
	if !hs.tt.HasText(locales.ExportDiagram()) {
		t.Error("the overlay has no export button")
	}
	if err := hs.tt.Click(locales.CopyAsImage()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !bytes.HasPrefix(hs.h.image, []byte("\x89PNG")) {
		t.Fatalf("copied %d bytes, not a PNG", len(hs.h.image))
	}
	if o.view.zoom != 0 {
		t.Fatalf("opened at zoom %v, not fitted", o.view.zoom)
	}
	a.run("image.zoomIn")
	if o.view.zoom == 0 {
		t.Error("zooming in left the diagram fitted")
	}
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.diagram != nil {
		t.Fatal("Escape left the diagram open")
	}

	hs.tt.ClickAt(x, y)
	hs.tt.ClickAt(x, y)
	hs.settle()
	if a.diagram == nil {
		t.Fatal("a double click did not open the diagram")
	}
	a.settings.PreviewMermaid = false
	hs.until("closed", func() bool { return a.diagram == nil })
}
