package app

const (
	defaultMarkdownPreviewFontSize = 14.0
	minMarkdownPreviewFontSize     = 8.0
	maxMarkdownPreviewFontSize     = 72.0
)

func (m *Model) markdownPreviewFontSize() float64 {
	size := m.nativeRuntimeNumber(settingsPageMarkdown, "languages.markdown.previewFontSize")
	if size < minMarkdownPreviewFontSize || size > maxMarkdownPreviewFontSize {
		return defaultMarkdownPreviewFontSize
	}
	return size
}
