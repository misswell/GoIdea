package app

import (
	"regexp"
	"strings"
)

var (
	generateActionTitle = regexp.MustCompile(
		`(?i)^(generate|implement|override|declare missing methods\b|add (test for\b|methods to .+ implement\b|missing|derive|impl|getter|setter|constructor)|create (constructor|getter|setter))`,
	)
	implementActionTitle = regexp.MustCompile(
		`(?i)\bimplement\b|\b(add|generate) (missing|unimplemented|all missing)\b|missing (members|methods|trait items)`,
	)
	overrideActionTitle = regexp.MustCompile(`(?i)\boverride\b`)
)

// ShowGenerate is code.generate. It runs the one generated member or opens
// OpenIDEA's native chooser when the servers offer several.
func (m *Model) ShowGenerate() {
	m.showIntentionsPane(intentionPaneOptions{
		title:     "Generate",
		emptyText: "Nothing to generate here",
		filter: func(action intentAction) bool {
			return action.kind == "source.addTest" ||
				generateActionTitle.MatchString(action.title)
		},
		autoApplySingle: true,
	})
}

// ShowImplementMethods is code.implementMethods.
func (m *Model) ShowImplementMethods() {
	m.showIntentionsPane(intentionPaneOptions{
		title:     "Implement Members",
		emptyText: "Nothing to implement",
		filter: func(action intentAction) bool {
			return implementActionTitle.MatchString(action.title)
		},
		autoApplySingle: true,
	})
}

// ShowOverrideMethods is code.overrideMethods.
func (m *Model) ShowOverrideMethods() {
	m.showIntentionsPane(intentionPaneOptions{
		title:     "Override Members",
		emptyText: "Nothing to override",
		filter: func(action intentAction) bool {
			return overrideActionTitle.MatchString(action.title)
		},
		autoApplySingle: true,
	})
}

func (m *Model) activeEditorSupportsGenerate() bool {
	t := m.tab()
	if t == nil || t.doc == nil {
		return false
	}
	switch strings.ToLower(t.doc.Info().Language) {
	case "java", "kotlin", "scala", "groovy", "rust", "go",
		"typescript", "typescriptreact", "javascript", "javascriptreact",
		"c", "cpp", "objective-c", "objective-cpp", "csharp", "swift",
		"dart", "php":
		return true
	}
	return false
}
