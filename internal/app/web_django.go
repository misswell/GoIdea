package app

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
)

const djangoScanLimit = 64 * 1024

var djangoTemplateTag = regexp.MustCompile(
	`\{%-?\s*(?:extends|block|load|url|static|csrf_token|include|if|for|with|comment|trans|translate|blocktrans|blocktranslate)\b[^%\n]*%\}`,
)

func isDjangoContent(text string) bool {
	if len(text) > djangoScanLimit {
		text = text[:djangoScanLimit]
	}
	return djangoTemplateTag.MatchString(text)
}

func djangoBannerVisible(m *Model, t *Tab) bool {
	if m == nil || t == nil || t.doc == nil || !isHTMLPath(t.doc.Path()) {
		return false
	}
	language := strings.ToLower(strings.TrimSpace(t.doc.Info().Language))
	if language == "django-html" || language == "angular template" ||
		m.currentFileTypeOverride(t.doc.Path()) != "" {
		return false
	}
	return isDjangoContent(t.doc.Text())
}

func (m *Model) djangoTemplateBanner(c *ui.Context, p *ui.Theme, t *Tab) {
	if !djangoBannerVisible(m, t) {
		return
	}
	path := t.doc.Path()
	ui.Row(c.Key("web:django:"+strings.ToLower(filepath.ToSlash(path)))).
		MinHeight(36).Shrink(0).Padding(7, 10).Gap(8).AlignItems(ui.Center).
		Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Text(c, "This file has Django template tags").
				FontSize(12).TextColor(p.Text).Grow(1).SingleLine()
			ui.Button(c, "Keep as HTML").Size(94, 24).OnClick(func() {
				m.SetFileTypeOverride(path, "html")
			})
			ui.Button(c, "Use Django Template").Size(132, 24).OnClick(func() {
				m.SetFileTypeOverride(path, "django-html")
			})
		})
}
