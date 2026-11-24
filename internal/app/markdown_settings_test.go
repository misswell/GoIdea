package app

import "testing"

func markdownPreviewSettingsModel(size any) *Model {
	values := map[string]any{}
	if size != nil {
		values["languages.markdown.previewFontSize"] = size
	}
	m := &Model{}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageMarkdown: {loaded: true, applied: values, draft: values},
	}
	return m
}

func TestMarkdownPreviewFontSizeUsesDefaultAndLoadedValue(t *testing.T) {
	if got := (&Model{}).markdownPreviewFontSize(); got != defaultMarkdownPreviewFontSize {
		t.Fatalf("unloaded preview size = %v, want %v", got, defaultMarkdownPreviewFontSize)
	}
	if got := markdownPreviewSettingsModel(float64(20)).markdownPreviewFontSize(); got != 20 {
		t.Fatalf("loaded preview size = %v, want 20", got)
	}
}

func TestMarkdownPreviewFontSizeFallsBackOutsideItsRange(t *testing.T) {
	for _, size := range []float64{0, 7.9, 72.1, -1} {
		if got := markdownPreviewSettingsModel(size).markdownPreviewFontSize(); got != defaultMarkdownPreviewFontSize {
			t.Fatalf("preview size %v = %v, want %v", size, got, defaultMarkdownPreviewFontSize)
		}
	}
}
