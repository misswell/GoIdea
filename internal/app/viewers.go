package app

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type markdownMode string

const (
	markdownEditorOnly    markdownMode = "editor"
	markdownEditorPreview markdownMode = "split"
	markdownPreviewOnly   markdownMode = "preview"
)

func init() {
	for _, action := range []*Action{
		{
			ID:       "markdown.editorOnly",
			Title:    "Editor Only",
			Category: "Markdown",
			Keywords: "markdown preview layout editor",
			When:     func(m *Model) bool { return activeMarkdownTab(m) != nil },
			Toggled: func(m *Model) bool {
				t := activeMarkdownTab(m)
				return t != nil && m.markdownModeFor(t) == markdownEditorOnly
			},
			RunWithArg: func(m *Model, arg any) {
				m.SetMarkdownMode(arg, markdownEditorOnly)
			},
		},
		{
			ID:       "markdown.editorAndPreview",
			Title:    "Editor and Preview",
			Category: "Markdown",
			Keywords: "markdown preview split layout editor",
			When:     func(m *Model) bool { return activeMarkdownTab(m) != nil },
			Toggled: func(m *Model) bool {
				t := activeMarkdownTab(m)
				return t != nil && m.markdownModeFor(t) == markdownEditorPreview
			},
			RunWithArg: func(m *Model, arg any) {
				m.SetMarkdownMode(arg, markdownEditorPreview)
			},
		},
		{
			ID:       "markdown.previewOnly",
			Title:    "Preview Only",
			Category: "Markdown",
			Keywords: "markdown rendered reader layout",
			When:     func(m *Model) bool { return activeMarkdownTab(m) != nil },
			Toggled: func(m *Model) bool {
				t := activeMarkdownTab(m)
				return t != nil && m.markdownModeFor(t) == markdownPreviewOnly
			},
			RunWithArg: func(m *Model, arg any) {
				m.SetMarkdownMode(arg, markdownPreviewOnly)
			},
		},
	} {
		_ = registerAction(action)
	}
	for _, action := range []*Action{
		{
			ID:       "image.zoomIn",
			Title:    "Zoom In",
			Category: "Image",
			Keywords: "image zoom larger viewer",
			When:     func(m *Model) bool { return activeImageViewer(m) != nil },
			RunWithArg: func(m *Model, arg any) {
				m.zoomImage(m.imageViewerForArg(arg), 1)
			},
		},
		{
			ID:       "image.zoomOut",
			Title:    "Zoom Out",
			Category: "Image",
			Keywords: "image zoom smaller viewer",
			When:     func(m *Model) bool { return activeImageViewer(m) != nil },
			RunWithArg: func(m *Model, arg any) {
				m.zoomImage(m.imageViewerForArg(arg), -1)
			},
		},
		{
			ID:       "image.actualSize",
			Title:    "Actual Size",
			Category: "Image",
			Keywords: "image zoom 100 percent viewer",
			When:     func(m *Model) bool { return activeImageViewer(m) != nil },
			RunWithArg: func(m *Model, arg any) {
				m.actualSizeImage(m.imageViewerForArg(arg))
			},
		},
		{
			ID:       "image.fitToWindow",
			Title:    "Fit Zoom to Window",
			Category: "Image",
			Keywords: "image fit window viewer",
			When:     func(m *Model) bool { return activeImageViewer(m) != nil },
			RunWithArg: func(m *Model, arg any) {
				m.fitImage(m.imageViewerForArg(arg))
			},
		},
		{
			ID:       "image.toggleChessboard",
			Title:    "Show Chessboard",
			Category: "Image",
			Keywords: "image transparency background checkerboard",
			When:     func(m *Model) bool { return activeImageViewer(m) != nil },
			Toggled: func(m *Model) bool {
				t := activeImageViewer(m)
				return t != nil && t.viewer != nil && t.viewer.chessboard
			},
			RunWithArg: func(m *Model, arg any) {
				t := m.imageViewerForArg(arg)
				if t != nil && t.viewer != nil {
					t.viewer.chessboard = !t.viewer.chessboard
					m.invalidate()
				}
			},
		},
	} {
		_ = registerAction(action)
	}
}

func activeMarkdownTab(m *Model) *Tab {
	if m == nil {
		return nil
	}
	t := m.tab()
	if t == nil || t.doc == nil || !isMarkdownPath(t.doc.Path()) {
		return nil
	}
	return t
}

func (m *Model) markdownModeFor(t *Tab) markdownMode {
	if t == nil {
		return markdownEditorOnly
	}
	if t.markdownMode != "" {
		return t.markdownMode
	}
	if m == nil {
		return markdownEditorPreview
	}
	return m.prefs.General.markdownMode()
}

// SetMarkdownMode switches one open Markdown document between the native
// editor, split and rendered preview layouts. The document remains the same
// tab and buffer in every mode, so edits and unsaved text are never copied.
func (m *Model) SetMarkdownMode(arg any, mode markdownMode) {
	if m == nil {
		return
	}
	target := activeMarkdownTab(m)
	if value, ok := arg.(string); ok && strings.TrimSpace(value) != "" {
		path := realPath(strings.TrimSpace(value))
		if at := m.tabForPath(path); at >= 0 {
			if tab := m.tabs[at]; tab != nil && tab.doc != nil && isMarkdownPath(path) {
				target = tab
			}
		}
	}
	if target == nil {
		return
	}
	if mode != markdownEditorOnly && mode != markdownEditorPreview && mode != markdownPreviewOnly {
		return
	}
	target.markdownMode = mode
	m.prefs.General.MarkdownMode = string(mode)
	m.savePreferences()
	switch mode {
	case markdownPreviewOnly:
		m.status = "Showing Markdown preview"
	case markdownEditorPreview:
		m.status = "Showing Markdown editor and preview"
	default:
		m.status = "Showing Markdown editor"
	}
	m.invalidate()
}

func markdownPreviewBlocks(t *Tab) []documentationBlock {
	if t == nil || t.doc == nil {
		return nil
	}
	return documentationBlocks(openidea.Markup{Kind: "markdown", Value: t.doc.Text()})
}

// markdownPreviewPage renders the source as native MyGo rich text. It never
// decodes raw HTML, and links retain the same safe http/https and local-file
// routing as documentation previews.
func (m *Model) markdownPreviewPage(c *ui.Context, p *ui.Theme, t *Tab, key string) {
	codeTheme := editor.DefaultTheme(p)
	linkColor := oiPaletteOf(p).link
	ui.Column(c.Key(key)).Grow(1).MinHeight(0).MinWidth(0).
		Background(p.Background).Children(func() {
		ui.Scroll(c.Key(key + ":scroll")).Grow(1).MinHeight(0).MinWidth(0).
			Children(func() {
				ui.Column(c).MinWidth(0).Padding(20, 28).Gap(10).Selectable().
					Children(func() {
						for _, block := range markdownPreviewBlocks(t) {
							m.drawMarkdownPreviewBlock(c, p, codeTheme, linkColor, t, block)
						}
					})
			})
	})
}

func (m *Model) drawMarkdownPreviewBlock(
	c *ui.Context,
	p *ui.Theme,
	codeTheme *editor.Theme,
	linkColor ui.Color,
	t *Tab,
	block documentationBlock,
) {
	if block.rule {
		ui.Row(c).Height(1).Background(p.Border)
		return
	}
	fontSize := float32(m.markdownPreviewFontSize())
	para := ui.RichText(c).MinWidth(0).MarginStart(float32(block.indent) * 12).FontSize(fontSize).Wrap()
	if block.heading {
		para.Bold().FontSize(fontSize + 4)
	}
	if block.code {
		para.Padding(8, 10).Background(p.Surface).Radius(5).Font(oiFontMono)
	}
	para.Children(func() {
		for _, run := range block.runs {
			if run.text == "" {
				continue
			}
			var span ui.Element
			if run.href != "" {
				span = ui.Link(c, run.text, "").TextColor(linkColor)
				href := run.href
				span.OnClick(func() { m.followMarkdownLink(t, href) })
			} else {
				span = ui.Text(c, run.text)
			}
			if run.code {
				span.Font(oiFontMono)
				if run.token != editor.TokenText && run.href == "" {
					span.TextColor(codeTheme.TokenColor(run.token))
				}
			}
			if run.bold {
				span.Bold()
			}
			if run.italic {
				span.Italic()
			}
			if run.strike {
				span.Strikethrough()
			}
		}
	})
}

func (m *Model) followMarkdownLink(source *Tab, href string) {
	if source == nil || source.doc == nil {
		return
	}
	if web, err := url.Parse(href); err == nil &&
		(web.Scheme == "http" || web.Scheme == "https") && web.Hostname() != "" {
		if err := mygo.Shell.OpenPath(href); err != nil {
			m.notifyError("markdown-link", "Cannot open the link", err.Error())
		}
		return
	}
	target, ok := documentationLink(href)
	if !ok {
		if fragment, cut := strings.CutPrefix(href, "#"); cut && fragment != "" {
			m.status = "Section links open in the Markdown source"
			m.invalidate()
		}
		return
	}
	if !target.virtual && !filepath.IsAbs(target.path) {
		target.path = filepath.Join(filepath.Dir(source.doc.Path()), target.path)
	}
	m.openDocumentationSource(&documentationView{}, target)
}
