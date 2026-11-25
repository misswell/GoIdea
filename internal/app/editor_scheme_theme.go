package app

import (
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
	for attribute, kind := range editorSchemeTokenKinds {
		if value, ok := attributes[attribute]; ok {
			if color, ok := parsedPreviewColor(value.Foreground); ok {
				setEditorTokenColor(theme, kind, color)
			}
			if value.Bold != nil {
				setEditorTokenBold(theme, kind, *value.Bold)
			}
			if value.Italic != nil {
				setEditorTokenItalic(theme, kind, *value.Italic)
			}
			continue
		}
		if value := visual.tokens[attribute]; value != "" {
			setEditorTokenColor(theme, kind, ui.Hex(value))
		}
	}
	if custom, ok := prefs.CustomSchemes[schemeID]; ok {
		for attribute, value := range custom.Attributes {
			applyEditorSchemeAttribute(theme, attribute, value)
		}
	}
	for attribute, value := range prefs.ColorOverrides[schemeID] {
		applyEditorSchemeAttribute(theme, attribute, value)
	}
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
	"keyword":               editor.TokenKeyword,
	"type":                  editor.TokenType,
	"className":             editor.TokenType,
	"functionDeclaration":   editor.TokenFunction,
	"functionCall":          editor.TokenFunction,
	"string":                editor.TokenString,
	"number":                editor.TokenNumber,
	"lineComment":           editor.TokenComment,
	"docComment":            editor.TokenComment,
	"operator":              editor.TokenOperator,
	"identifier":            editor.TokenVariable,
	"parameter":             editor.TokenVariable,
	"globalVariable":        editor.TokenVariable,
	"pyKeyword":             editor.TokenKeyword,
	"pyNumber":              editor.TokenNumber,
	"pyString":              editor.TokenString,
	"pyLineComment":         editor.TokenComment,
	"pyDocstring":           editor.TokenComment,
	"pyFunctionDeclaration": editor.TokenFunction,
	"pyParameter":           editor.TokenVariable,
	"pyOperationSign":       editor.TokenOperator,
	"property":              editor.TokenProperty,
	"constant":              editor.TokenConstant,
	"escape":                editor.TokenEscape,
	"tag":                   editor.TokenTag,
	"attribute":             editor.TokenAttribute,
	"todo":                  editor.TokenMeta,
	"invalid":               editor.TokenInvalid,
}

func applyEditorSchemeAttribute(theme *editor.Theme, attribute string, value ColorAttributes) {
	kind, ok := editorSchemeTokenKinds[attribute]
	if !ok {
		return
	}
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

func setEditorTokenColor(theme *editor.Theme, kind editor.TokenKind, color ui.Color) {
	theme.Tokens[kind].Color = color
}

func setEditorTokenBold(theme *editor.Theme, kind editor.TokenKind, bold bool) {
	theme.Tokens[kind].Bold = bold
}

func setEditorTokenItalic(theme *editor.Theme, kind editor.TokenKind, italic bool) {
	theme.Tokens[kind].Italic = italic
}
