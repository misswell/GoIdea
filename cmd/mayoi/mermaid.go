package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	mermaidpkg "mayoi/internal/mermaid"
)

// isMermaid reports whether a document is a Mermaid diagram, which
// previews as the diagram it draws.
func isMermaid(d *document) bool {
	ext := filepath.Ext(d.path)
	return d.path != "" && (strings.EqualFold(ext, ".mmd") || strings.EqualFold(ext, ".mermaid"))
}

type mermaidStyle = mermaidpkg.Style
type mermaidDiagram = mermaidpkg.Diagram
type mermaidLabel = mermaidpkg.Label
type mermaidTile = mermaidpkg.Tile

const mermaidTileSide = 1024

// mermaidStyle is the style of diagrams in the theme the window is in,
// or on its way to.
func (a *app) mermaidStyle() mermaidStyle {
	p := a.pal
	if a.fade.to != nil {
		p = a.fade.to
	}
	flat := func(c ui.Color) ui.Color { return c.Over(p.editor) }
	st := mermaidStyle{
		BG: p.editor, FG: flat(p.text), Muted: flat(p.muted), Line: flat(p.muted),
		Surface: flat(p.accent.Alpha(0.1)), SurfaceAlt: flat(p.accent.Alpha(0.2)),
		Border: flat(p.accent.Alpha(0.6)), Accent: flat(p.accent), Danger: flat(p.danger),
		Mono: a.settings.FontFamily, MonoWeight: a.settings.FontWeight,
	}
	if p.mono {
		st.Sans = a.settings.FontFamily
	}
	return st
}

func renderMermaid(source string, style mermaidStyle) *mermaidDiagram {
	return mermaidpkg.Render(source, style)
}

func hexColor(color ui.Color) string {
	return mermaidpkg.HexColor(color)
}
