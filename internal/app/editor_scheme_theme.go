package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type editorSchemeVisual struct {
	dark              bool
	text              ui.Color
	background        ui.Color
	lineNumber        ui.Color
	activeLineNumber  ui.Color
	selection         ui.Color
	inactiveSelection ui.Color
	caret             ui.Color
	tokens            map[string]string
}

var editorSchemeVisuals = map[string]editorSchemeVisual{
	"dark": {
		dark: true, text: ui.Hex("#bcbec4"), background: ui.Hex("#191a1c"),
		lineNumber: ui.Hex("#4b5059"), activeLineNumber: ui.Hex("#a1a3ab"),
		selection: ui.Hex("#214283"), inactiveSelection: ui.Hex("#1d3460"),
		caret: ui.Hex("#ced0d6"),
		tokens: map[string]string{
			"keyword": "#cf8e6d", "string": "#6aab73", "type": "#bcbec4",
			"functionDeclaration": "#56a8f5", "functionCall": "#bcbec4",
			"lineComment": "#7a7e85", "number": "#2aacb8", "operator": "#bcbec4",
			"identifier": "#bcbec4", "parameter": "#bcbec4", "property": "#c77dbb",
			"constant": "#c77dbb", "escape": "#cf8e6d", "tag": "#d5b778",
			"attribute": "#bcbec4", "todo": "#8bb33d", "invalid": "#f75464",
		},
	},
	"light": {
		text: ui.Hex("#080808"), background: ui.Hex("#ffffff"),
		lineNumber: ui.Hex("#aeb3c2"), activeLineNumber: ui.Hex("#767a8a"),
		selection: ui.Hex("#a6d2ff"), inactiveSelection: ui.Hex("#c9e3ff"),
		caret: ui.Hex("#000000"),
		tokens: map[string]string{
			"keyword": "#0033b3", "string": "#067d17", "type": "#000000",
			"functionDeclaration": "#00627a", "functionCall": "#080808",
			"lineComment": "#8c8c8c", "number": "#1750eb", "operator": "#080808",
			"identifier": "#080808", "parameter": "#080808", "property": "#871094",
			"constant": "#871094", "escape": "#0037a6", "tag": "#0033b3",
			"attribute": "#174ad4", "todo": "#008dde", "invalid": "#f50000",
		},
	},
	"dark-hc": {
		dark: true, text: ui.Hex("#e3e4e8"), background: ui.Hex("#000000"),
		lineNumber: ui.Hex("#8b8e95"), activeLineNumber: ui.Hex("#e3e4e8"),
		selection: ui.Hex("#2a5bb8"), inactiveSelection: ui.Hex("#1f3f7a"),
		caret: ui.Hex("#ffffff"),
		tokens: map[string]string{
			"keyword": "#e8a885", "string": "#8ccc94", "type": "#e3e4e8",
			"functionDeclaration": "#7cc0ff", "functionCall": "#e3e4e8",
			"lineComment": "#a5a9b0", "number": "#4fd0dc", "operator": "#e3e4e8",
			"identifier": "#e3e4e8", "parameter": "#e3e4e8", "property": "#e0a0d6",
			"constant": "#e0a0d6", "escape": "#e8a885", "tag": "#ecd39a",
			"attribute": "#e3e4e8", "todo": "#aad35a", "invalid": "#ff8a95",
		},
	},
	"solarized-light": {
		text: ui.Hex("#657b83"), background: ui.Hex("#fdf6e3"),
		lineNumber: ui.Hex("#93a1a1"), activeLineNumber: ui.Hex("#586e75"),
		selection: ui.Hex("#eee8d5"), inactiveSelection: ui.Hex("#eee8d5"),
		caret: ui.Hex("#268bd2"),
		tokens: map[string]string{
			"keyword": "#859900", "string": "#2aa198", "type": "#b58900",
			"functionDeclaration": "#268bd2", "functionCall": "#268bd2",
			"lineComment": "#93a1a1", "number": "#2aa198", "operator": "#657b83",
			"identifier": "#268bd2", "parameter": "#657b83", "property": "#268bd2",
			"constant": "#2aa198", "escape": "#dc322f", "tag": "#268bd2",
			"attribute": "#b58900", "todo": "#d33682", "invalid": "#dc322f",
		},
	},
	"solarized-dark": {
		dark: true, text: ui.Hex("#839496"), background: ui.Hex("#002b36"),
		lineNumber: ui.Hex("#586e75"), activeLineNumber: ui.Hex("#93a1a1"),
		selection: ui.Hex("#073642"), inactiveSelection: ui.Hex("#073642"),
		caret: ui.Hex("#268bd2"),
		tokens: map[string]string{
			"keyword": "#859900", "string": "#2aa198", "type": "#b58900",
			"functionDeclaration": "#268bd2", "functionCall": "#268bd2",
			"lineComment": "#586e75", "number": "#2aa198", "operator": "#839496",
			"identifier": "#268bd2", "parameter": "#839496", "property": "#268bd2",
			"constant": "#2aa198", "escape": "#dc322f", "tag": "#268bd2",
			"attribute": "#b58900", "todo": "#d33682", "invalid": "#dc322f",
		},
	},
}

func (m *Model) applyEditorColorSchemeTheme() {
	for _, tab := range m.allTabs() {
		if tab == nil || tab.state == nil {
			continue
		}
		tab.state.Theme = m.editorColorSchemeTheme()
	}
}

func (m *Model) editorColorSchemeTheme() *editor.Theme {
	if m == nil {
		return nil
	}
	prefs := m.prefs.EditorScheme
	schemeID := normalizeColorSchemeID(prefs.ColorScheme)
	if schemeID == themeDefaultColorScheme {
		return nil
	}
	baseID, attributes := editorSchemeBaseAndAttributes(prefs, schemeID)
	visual, ok := editorSchemeVisuals[baseID]
	if !ok {
		return nil
	}
	theme := editor.DefaultTheme(openideaTheme(visual.dark))
	theme.Background = visual.background
	theme.Text = visual.text
	theme.LineNumber = visual.lineNumber
	theme.ActiveLineNumber = visual.activeLineNumber
	theme.Selection = visual.selection
	theme.InactiveSelection = visual.inactiveSelection
	theme.Cursor = visual.caret
	applyEditorSchemeVisualAttributes(theme, visual)
	effective := make(map[string]ColorAttributes, len(attributes))
	if custom, ok := prefs.CustomSchemes[schemeID]; ok {
		for attribute, value := range custom.Attributes {
			effective[attribute] = cloneColorAttributes(value)
		}
	}
	for attribute, value := range prefs.ColorOverrides[schemeID] {
		current := effective[attribute]
		current.Foreground = firstNonEmpty(current.Foreground, value.Foreground)
		current.Background = firstNonEmpty(current.Background, value.Background)
		current.EffectColor = firstNonEmpty(current.EffectColor, value.EffectColor)
		if value.Bold != nil {
			current.Bold = value.Bold
		}
		if value.Italic != nil {
			current.Italic = value.Italic
		}
		current.Effect = firstNonEmpty(current.Effect, value.Effect)
		effective[attribute] = current
	}
	applyEditorSchemeAttributes(theme, effective)
	return theme
}

func editorSchemeBaseAndAttributes(prefs EditorSchemePrefs, id string) (string, map[string]ColorAttributes) {
	if custom, ok := prefs.CustomSchemes[id]; ok {
		parent := normalizeSchemeParent(custom.Parent, custom.Attributes)
		if _, ok := editorSchemeVisuals[parent]; !ok {
			parent = "light"
		}
		return parent, custom.Attributes
	}
	return id, nil
}

var editorSchemeTokenKinds = map[string]editor.TokenKind{
	"keyword":             editor.TokenKeyword,
	"type":                editor.TokenType,
	"className":           editor.TokenType,
	"functionDeclaration": editor.TokenFunction,
	"functionCall":        editor.TokenFunction,
	"string":              editor.TokenString,
	"number":              editor.TokenNumber,
	"lineComment":         editor.TokenComment,
	"docComment":          editor.TokenComment,
	"docCommentTag":       editor.TokenComment,
	"docCommentTagValue":  editor.TokenComment,
	"docCommentMarkup":    editor.TokenComment,
	"operator":            editor.TokenOperator,
	"identifier":          editor.TokenVariable,
	"parameter":           editor.TokenVariable,
	"globalVariable":      editor.TokenVariable,
	"field":               editor.TokenProperty,
	"property":            editor.TokenProperty,
	"constant":            editor.TokenConstant,
	"predefined":          editor.TokenConstant,
	"escape":              editor.TokenEscape,
	"tag":                 editor.TokenTag,
	"markupTag":           editor.TokenTag,
	"attribute":           editor.TokenAttribute,
	"markupAttribute":     editor.TokenAttribute,
	"regexp":              editor.TokenRegexp,
	"todo":                editor.TokenMeta,
	"metadata":            editor.TokenMeta,
	"invalid":             editor.TokenInvalid,
}

var editorSchemeLanguageTokenKinds = map[string]editor.TokenKind{
	"java.keyword":             editor.TokenKeyword,
	"java.number":              editor.TokenNumber,
	"java.string":              editor.TokenString,
	"java.lineComment":         editor.TokenComment,
	"java.docComment":          editor.TokenComment,
	"java.docCommentTag":       editor.TokenComment,
	"java.docCommentTagValue":  editor.TokenComment,
	"java.docCommentMarkup":    editor.TokenComment,
	"java.className":           editor.TokenType,
	"java.identifier":          editor.TokenVariable,
	"java.parameter":           editor.TokenVariable,
	"java.field":               editor.TokenProperty,
	"java.constant":            editor.TokenConstant,
	"java.functionDeclaration": editor.TokenFunction,
	"java.functionCall":        editor.TokenFunction,
	"java.operator":            editor.TokenOperator,
	"pyKeyword":                editor.TokenKeyword,
	"pyNumber":                 editor.TokenNumber,
	"pyString":                 editor.TokenString,
	"pyLineComment":            editor.TokenComment,
	"pyDocstring":              editor.TokenComment,
	"pyFunctionDeclaration":    editor.TokenFunction,
	"pyParameter":              editor.TokenVariable,
	"pyOperationSign":          editor.TokenOperator,
}

var editorSchemeAttributePriority = map[string]int{
	"type":                0,
	"className":           1,
	"functionDeclaration": 0,
	"functionCall":        1,
	"lineComment":         0,
	"docComment":          1,
	"docCommentTag":       2,
	"docCommentTagValue":  3,
	"docCommentMarkup":    4,
	"identifier":          0,
	"globalVariable":      1,
	"parameter":           2,
	"field":               1,
	"property":            0,
	"constant":            0,
	"predefined":          1,
}

var editorSchemeExportTokenAttributes = []struct {
	attribute string
	kind      editor.TokenKind
}{
	{"keyword", editor.TokenKeyword},
	{"className", editor.TokenType},
	{"functionDeclaration", editor.TokenFunction},
	{"identifier", editor.TokenVariable},
	{"field", editor.TokenProperty},
	{"constant", editor.TokenConstant},
	{"number", editor.TokenNumber},
	{"string", editor.TokenString},
	{"escape", editor.TokenEscape},
	{"regexp", editor.TokenRegexp},
	{"lineComment", editor.TokenComment},
	{"operator", editor.TokenOperator},
	{"markupTag", editor.TokenTag},
	{"markupAttribute", editor.TokenAttribute},
	{"metadata", editor.TokenMeta},
	{"invalid", editor.TokenInvalid},
}

func editorSchemeExportAttributes(
	prefs EditorSchemePrefs,
	id string,
	base *ui.Theme,
) map[string]ColorAttributes {
	attributes := map[string]ColorAttributes{}
	if custom, ok := prefs.CustomSchemes[id]; ok {
		for attribute, value := range custom.Attributes {
			attributes[attribute] = cloneColorAttributes(value)
		}
	}
	for attribute, value := range prefs.ColorOverrides[id] {
		current := attributes[attribute]
		current.Foreground = firstNonEmpty(current.Foreground, value.Foreground)
		current.Background = firstNonEmpty(current.Background, value.Background)
		current.EffectColor = firstNonEmpty(current.EffectColor, value.EffectColor)
		if value.Bold != nil {
			current.Bold = value.Bold
		}
		if value.Italic != nil {
			current.Italic = value.Italic
		}
		current.Effect = firstNonEmpty(current.Effect, value.Effect)
		attributes[attribute] = current
	}
	exportPrefs := prefs
	exportPrefs.ColorScheme = id
	theme := (&Model{prefs: Preferences{EditorScheme: exportPrefs}}).editorColorSchemeTheme()
	if theme == nil {
		if base == nil {
			base = openideaTheme(false)
		}
		theme = editor.DefaultTheme(base)
	}
	fillTextAttributes := func(attribute, foreground string, background string) {
		value := attributes[attribute]
		value.Foreground = firstNonEmpty(value.Foreground, foreground)
		value.Background = firstNonEmpty(value.Background, background)
		attributes[attribute] = value
	}
	fillTextAttributes("text",
		colorAsHex(theme.Text), colorAsHex(theme.Background))
	for _, target := range editorSchemeExportTokenAttributes {
		value := attributes[target.attribute]
		style := theme.Tokens[target.kind]
		value.Foreground = firstNonEmpty(value.Foreground, colorAsHex(theme.TokenColor(target.kind)))
		if value.Bold == nil {
			bold := style.Bold
			value.Bold = &bold
		}
		if value.Italic == nil {
			italic := style.Italic
			value.Italic = &italic
		}
		attributes[target.attribute] = value
	}
	fillSurface := func(attribute string, color ui.Color) {
		value := attributes[attribute]
		value.Foreground = firstNonEmpty(value.Foreground, colorAsHex(color))
		attributes[attribute] = value
	}
	fillSurface("caret", theme.Cursor)
	fillSurface("lineNumber", theme.LineNumber)
	fillSurface("lineNumberCaret", theme.ActiveLineNumber)
	fillSurface("whitespaces", theme.Whitespace)
	fillSurface("indentGuide", theme.IndentGuide)
	fillSurface("rightMargin", theme.WrapGuide)
	fillSurfaceBackground := func(attribute string, color ui.Color) {
		value := attributes[attribute]
		value.Background = firstNonEmpty(value.Background, colorAsHex(color))
		attributes[attribute] = value
	}
	fillSurfaceBackground("caretRow", theme.ActiveLine)
	fillSurfaceBackground("selection", theme.Selection)
	fillSurfaceBackground("selectionInactive", theme.InactiveSelection)
	return attributes
}

func colorAsHex(color ui.Color) string {
	if color.A == 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", color.R, color.G, color.B)
}

func applyEditorSchemeVisualAttributes(theme *editor.Theme, visual editorSchemeVisual) {
	attributes := make(map[string]ColorAttributes, len(visual.tokens))
	for attribute, value := range visual.tokens {
		attributes[attribute] = ColorAttributes{Foreground: value}
	}
	applyEditorSchemeAttributes(theme, attributes)
}

func applyEditorSchemeAttributes(theme *editor.Theme, attributes map[string]ColorAttributes) {
	selected := map[editor.TokenKind]string{}
	for attribute := range attributes {
		kind, ok := editorSchemeTokenKinds[attribute]
		if !ok {
			continue
		}
		current, exists := selected[kind]
		if !exists ||
			editorSchemeAttributePriority[attribute] < editorSchemeAttributePriority[current] ||
			editorSchemeAttributePriority[attribute] == editorSchemeAttributePriority[current] &&
				attribute < current {
			selected[kind] = attribute
		}
	}
	for kind, attribute := range selected {
		value := attributes[attribute]
		if color, ok := parsedPreviewColor(value.Foreground); ok {
			setEditorTokenColor(theme, kind, color)
		}
		if value.Bold != nil {
			setEditorTokenBold(theme, kind, *value.Bold)
		}
		if value.Italic != nil {
			setEditorTokenItalic(theme, kind, *value.Italic)
		}
	}
	if text, ok := attributes["text"]; ok {
		if color, ok := parsedPreviewColor(text.Foreground); ok {
			theme.Text = color
		}
		if color, ok := parsedPreviewColor(text.Background); ok {
			theme.Background = color
		}
	}
	setSurfaceColor := func(attribute string, target *ui.Color) {
		value := attributes[attribute]
		if color, ok := parsedPreviewColor(firstNonEmpty(value.Foreground, value.Background)); ok {
			*target = color
		}
	}
	setSurfaceColor("caret", &theme.Cursor)
	setSurfaceColor("caretRow", &theme.ActiveLine)
	setSurfaceColor("selection", &theme.Selection)
	setSurfaceColor("selectionInactive", &theme.InactiveSelection)
	setSurfaceColor("lineNumber", &theme.LineNumber)
	setSurfaceColor("lineNumberCaret", &theme.ActiveLineNumber)
	setSurfaceColor("whitespaces", &theme.Whitespace)
	setSurfaceColor("indentGuide", &theme.IndentGuide)
	setSurfaceColor("rightMargin", &theme.WrapGuide)
}

func editorSchemeAttributeKind(attribute string) (editor.TokenKind, bool) {
	if kind, ok := editorSchemeTokenKinds[attribute]; ok {
		return kind, true
	}
	kind, ok := editorSchemeLanguageTokenKinds[attribute]
	return kind, ok
}

func setEditorTokenColor(theme *editor.Theme, kind editor.TokenKind, color ui.Color) {
	theme.Tokens[kind].Color = color
}

func setEditorTokenBold(theme *editor.Theme, kind editor.TokenKind, bold bool) {
	theme.Tokens[kind].Bold = bold
}

func setEditorTokenItalic(theme *editor.Theme, kind editor.TokenKind, italic bool) {
	theme.Tokens[kind].Italic = italic
}
