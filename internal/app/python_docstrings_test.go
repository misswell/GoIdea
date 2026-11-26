package app

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func pythonDocstringTestModel(format string) *Model {
	return &Model{nativeSettings: nativeSettingsCatalogState{pages: map[string]nativeSettingsPageState{
		settingsPagePythonDocstrings: {
			loaded: true,
			applied: map[string]any{
				"editor.docstringFormat": format,
			},
		},
	}}}
}

func pythonDocstringStyledText(t *testing.T, m *Model, value string) string {
	t.Helper()
	var text strings.Builder
	for _, block := range m.documentationBlocksForLanguage(
		documentationMarkupForTest(value), "python",
	) {
		for _, run := range block.runs {
			text.WriteString(run.text)
		}
	}
	return text.String()
}

func documentationMarkupForTest(value string) openidea.Markup {
	return openidea.Markup{Kind: "markdown", Value: value}
}

func TestPythonDocstringMarkupFormats(t *testing.T) {
	tests := []struct {
		format, input string
		wants         []string
		rejects       []string
	}{
		{
			format: "rest",
			input:  "Load things.\n\n:param x: name to load\n:returns: loaded value\n.. note:: cached\n",
			wants:  []string{"**param**", "*x*", "**returns**", "**note::**"},
		},
		{
			format: "epytext",
			input:  "Load things.\n\n@param x: name to load\n@return: loaded value\n",
			wants:  []string{"**param**", "*x*", "**return**"},
		},
		{
			format: "google",
			input:  "Load things.\n\nArgs:\n    x (int): first\n    y: second\n\nReturns:\n    Nothing.\n",
			wants:  []string{"**Args**:", "*x*", "*y*", "**Returns**:"},
		},
		{
			format: "numpy",
			input:  "Load things.\n\nParameters\n----------\nx : int\n    name\n\nReturns\n-------\nstr\n    loaded\n",
			wants:  []string{"**Parameters**", "*x*", "**Returns**"},
		},
		{
			format:  "plain",
			input:   ":param x: name\nArgs:\n    x: name\n",
			rejects: []string{"**param**", "*x*", "**Args**"},
		},
	}
	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			m := pythonDocstringTestModel(test.format)
			value := pythonDocstringMarkdown(test.input, test.format)
			for _, want := range test.wants {
				if !strings.Contains(value, want) {
					t.Fatalf("%s markup %q lacks %q", test.format, value, want)
				}
			}
			for _, reject := range test.rejects {
				if strings.Contains(value, reject) {
					t.Fatalf("%s markup %q contains %q", test.format, value, reject)
				}
			}
			if text := pythonDocstringStyledText(t, m, test.input); strings.TrimSpace(text) == "" {
				t.Fatal("marked docstring rendered no documentation")
			}
		})
	}
}

func TestPythonDocstringFormatSkipsNonPythonDocs(t *testing.T) {
	m := pythonDocstringTestModel("google")
	markup := documentationMarkupForTest("Args:\n    x: first\n")
	blocks := m.documentationBlocksForLanguage(markup, "java")
	if len(blocks) == 0 {
		t.Fatal("non-Python documentation did not render")
	}
	for _, run := range blocks[0].runs {
		if strings.Contains(run.text, "**") || strings.Contains(run.text, "*x*") {
			t.Fatalf("Python docstring format changed Java documentation: %+v", run)
		}
	}
}
