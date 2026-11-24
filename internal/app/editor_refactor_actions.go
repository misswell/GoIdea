package app

import (
	"regexp"
	"strings"
)

var (
	refactorExtractPrefix = regexp.MustCompile(`(?i)^(extract|introduce)( (in)?to)?( an?)? `)
	refactorTypeTitle     = regexp.MustCompile(
		`(?i)^(extract|introduce)( (in)?to)?( an?)? (type\b|typedef|interface|struct|enum|trait|superclass|module)`,
	)
	refactorOrder = map[string]int{
		"rename":           0,
		"changeSignature":  1,
		"move":             2,
		"safeDelete":       3,
		"extractMethod":    4,
		"extractVariable":  5,
		"extractConstant":  6,
		"extractField":     7,
		"extractParameter": 8,
		"inline":           9,
		"pullMembersUp":    10,
		"pushMembersDown":  11,
	}
)

// classifyRefactorAction maps a language server's code action to one of
// OpenIDEA's refactoring ids (third_party/openidea/src/editor/refactor/catalog.ts).
func classifyRefactorAction(action intentAction) string {
	kind := action.kind
	if !refactorKindIs(kind, "refactor") {
		return ""
	}
	title := action.title
	generic := kind == "refactor" || refactorKindIs(kind, "refactor.rewrite")
	if refactorKindIs(kind, "refactor.extract") ||
		generic && refactorExtractPrefix.MatchString(title) {
		sub := strings.TrimPrefix(kind, "refactor.extract")
		match := func(pattern string) bool {
			ok, _ := regexp.MatchString(pattern, sub)
			if ok {
				return true
			}
			ok, _ = regexp.MatchString(pattern, title)
			return ok
		}
		if refactorTypeTitle.MatchString(title) {
			return ""
		}
		switch {
		case match(`(?i)function|method`):
			return "extractMethod"
		case match(`(?i)field|member|property`):
			return "extractField"
		case match(`(?i)parameter|argument`):
			return "extractParameter"
		case regexp.MustCompile(`(?i)enclosing scope`).MatchString(title):
			return "extractVariable"
		case match(`(?i)constant|static`):
			return "extractConstant"
		case match(`(?i)variable|local|\blet\b`):
			return "extractVariable"
		}
		return ""
	}
	switch {
	case refactorKindIs(kind, "refactor.inline") ||
		generic && regexp.MustCompile(`(?i)^inline\b`).MatchString(title):
		return "inline"
	case refactorKindIs(kind, "refactor.move") ||
		generic && regexp.MustCompile(`(?i)^move\b`).MatchString(title):
		return "move"
	case regexp.MustCompile(`(?i)signature`).MatchString(kind) ||
		regexp.MustCompile(`(?i)\bchange signature\b|\breorder (parameters|arguments)\b`).MatchString(title):
		return "changeSignature"
	}
	return ""
}

func refactorKindIs(kind, prefix string) bool {
	return kind == prefix || strings.HasPrefix(kind, prefix+".")
}

func (m *Model) ShowRefactorThis() {
	items := make([]intentAction, 0, 2)
	if m.editorIdentifierAtCaret() {
		items = append(items, intentAction{
			title: "Rename…", kind: "refactor", priority: refactorOrder["rename"],
			run: func(m *Model) {
				if !m.RunCommand("refactor.rename") {
					m.status = "There is no renamable symbol under the caret"
					m.invalidate()
				}
			},
		})
		if m.activeEditorEditable() {
			items = append(items, intentAction{
				title: "Safe Delete…", kind: "refactor", priority: refactorOrder["safeDelete"],
				run: func(m *Model) { m.SafeDeleteActive() },
			})
		}
	}
	m.showIntentionsPane(intentionPaneOptions{
		title:       "Refactor This",
		emptyText:   "Nothing here",
		only:        []string{"refactor"},
		clientItems: items,
		filter: func(action intentAction) bool {
			return classifyRefactorAction(action) != ""
		},
		priority: func(action intentAction) int {
			if action.run != nil {
				return action.priority
			}
			if order, ok := refactorOrder[classifyRefactorAction(action)]; ok {
				return order
			}
			return len(refactorOrder) + 1
		},
	})
}

func (m *Model) showRefactor(id string) {
	m.showIntentionsPane(intentionPaneOptions{
		title:     refactorTitle(id),
		emptyText: refactorEmptyText(id, m.hasEditorSelection()),
		only:      []string{"refactor"},
		filter: func(action intentAction) bool {
			return classifyRefactorAction(action) == id
		},
		autoApplySingle: true,
	})
}

func (m *Model) hasEditorSelection() bool {
	t := m.tab()
	return t != nil && t.state != nil && !t.state.Selection().Empty()
}

func (m *Model) editorIdentifierAtCaret() bool {
	t := m.tab()
	if t == nil || t.state == nil || t.state.Buffer == nil {
		return false
	}
	snapshot := t.state.Buffer.Snapshot()
	start, end := editorWordRange(snapshot.String(), t.state.Cursor())
	return start < end
}

func refactorTitle(id string) string {
	switch id {
	case "changeSignature":
		return "Change Signature"
	case "move":
		return "Move"
	case "extractMethod":
		return "Extract Method"
	case "extractVariable":
		return "Introduce Variable"
	case "extractConstant":
		return "Introduce Constant"
	case "extractField":
		return "Introduce Field"
	case "extractParameter":
		return "Introduce Parameter"
	case "inline":
		return "Inline"
	}
	return "Refactor"
}

func refactorEmptyText(id string, selection bool) string {
	switch id {
	case "changeSignature":
		return "Cannot perform refactoring. Caret should be positioned at the name of the function or method to be refactored"
	case "move":
		return "Cannot perform refactoring. Caret should be positioned at the name of the element to be refactored"
	case "extractMethod":
		if selection {
			return "Cannot perform refactoring. Selected block should represent a set of statements or an expression"
		}
		return "Cannot perform refactoring. Select the code to extract"
	case "extractVariable", "extractConstant":
		if selection {
			return "Cannot perform refactoring. Selected block should represent an expression"
		}
		return "Cannot perform refactoring. Caret should be inside an expression"
	case "extractField":
		if selection {
			return "Cannot perform refactoring. Selected block should represent an expression"
		}
		return "Cannot perform refactoring. Caret should be inside an expression in a class"
	case "extractParameter":
		if selection {
			return "Cannot perform refactoring. Selected block should represent an expression"
		}
		return "Cannot perform refactoring. Caret should be inside an expression in a function"
	case "inline":
		return "Cannot perform refactoring. Caret should be positioned at the name of the element to be refactored"
	}
	return "Nothing here"
}
