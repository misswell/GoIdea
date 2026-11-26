package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	editorSemanticHighlightingKey = "editor.semanticHighlighting"
	editorRainbowBracketsKey      = "editor.rainbowBrackets"
	editorColorSchemeKey          = "editor.colorScheme"
	editorCustomSchemesKey        = "editor.customSchemes"
	editorColorOverridesKey       = "editor.colorOverrides"
	themeDefaultColorScheme       = "theme-default"
)

// EditorSchemePrefs is GoIDEA's local copy of the editor color-scheme
// settings. The backend remains the shared source used by OpenIDEA and other
// GoIDEA windows; this copy keeps the current process responsive and survives
// restarts even while the server is unavailable.
type EditorSchemePrefs struct {
	SemanticHighlighting bool                         `json:"semanticHighlighting,omitempty"`
	RainbowBrackets      bool                         `json:"rainbowBrackets,omitempty"`
	ColorScheme          string                       `json:"colorScheme,omitempty"`
	CustomSchemes        map[string]CustomColorScheme `json:"customSchemes,omitempty"`
	ColorOverrides       map[string]ColorAttributeSet `json:"colorOverrides,omitempty"`
}

// CustomColorScheme is the subset of an IntelliJ .icls scheme that GoIDEA can
// store and round-trip. MyGo themes edit token categories and editor surfaces;
// attributes outside those categories are retained only for later rendering
// and export.
type CustomColorScheme struct {
	Name       string                     `json:"name"`
	Parent     string                     `json:"parent"`
	Attributes map[string]ColorAttributes `json:"attributes,omitempty"`
}

// ColorAttributes is one imported IntelliJ text-attribute override.
type ColorAttributes struct {
	Foreground  string `json:"foreground,omitempty"`
	Background  string `json:"background,omitempty"`
	Bold        *bool  `json:"bold,omitempty"`
	Italic      *bool  `json:"italic,omitempty"`
	Effect      string `json:"effect,omitempty"`
	EffectColor string `json:"effectColor,omitempty"`
}

// ColorAttributeSet groups overrides by scheme id.
type ColorAttributeSet map[string]ColorAttributes

type backendEditorSchemeSettings struct {
	Editor struct {
		SemanticHighlighting *bool                        `json:"semanticHighlighting"`
		RainbowBrackets      *bool                        `json:"rainbowBrackets"`
		ColorScheme          *string                      `json:"colorScheme"`
		CustomSchemes        map[string]CustomColorScheme `json:"customSchemes"`
		ColorOverrides       map[string]ColorAttributeSet `json:"colorOverrides"`
	} `json:"editor"`
}

type parsedColorScheme struct {
	Scheme  CustomColorScheme
	Applied int
	Skipped int
}

type iclsColorTarget struct {
	Attribute string
	Field     string
}

var iclsColorTargets = map[string]iclsColorTarget{
	"CARET_COLOR":                    {"caret", "foreground"},
	"CARET_ROW_COLOR":                {"caretRow", "background"},
	"SELECTION_BACKGROUND":           {"selection", "background"},
	"SELECTION_FOREGROUND":           {"selectionForeground", "foreground"},
	"INACTIVE_SELECTION_BACKGROUND":  {"selectionInactive", "background"},
	"GUTTER_BACKGROUND":              {"gutter", "background"},
	"LINE_NUMBERS_COLOR":             {"lineNumber", "foreground"},
	"LINE_NUMBER_ON_CARET_ROW_COLOR": {"lineNumberCaret", "foreground"},
	"WHITESPACES":                    {"whitespaces", "foreground"},
	"INDENT_GUIDE":                   {"indentGuide", "foreground"},
	"RIGHT_MARGIN_COLOR":             {"rightMargin", "foreground"},
	"METHOD_SEPARATORS_COLOR":        {"methodSeparator", "foreground"},
	"CONSOLE_BACKGROUND_KEY":         {"consoleBackground", "background"},
}

var iclsAttributeTargets = map[string][]string{
	"TEXT":                          {"text"},
	"DEFAULT_TEXT":                  {"text"},
	"DEFAULT_KEYWORD":               {"keyword"},
	"DEFAULT_IDENTIFIER":            {"identifier"},
	"DEFAULT_LOCAL_VARIABLE":        {"identifier"},
	"DEFAULT_PARAMETER":             {"parameter"},
	"DEFAULT_GLOBAL_VARIABLE":       {"globalVariable"},
	"DEFAULT_CONSTANT":              {"constant"},
	"DEFAULT_INSTANCE_FIELD":        {"field"},
	"DEFAULT_STATIC_FIELD":          {"constant"},
	"DEFAULT_FUNCTION_DECLARATION":  {"functionDeclaration"},
	"DEFAULT_FUNCTION_CALL":         {"functionCall"},
	"DEFAULT_CLASS_NAME":            {"className"},
	"DEFAULT_INTERFACE_NAME":        {"className"},
	"DEFAULT_NUMBER":                {"number"},
	"DEFAULT_STRING":                {"string"},
	"DEFAULT_VALID_STRING_ESCAPE":   {"escape"},
	"DEFAULT_INVALID_STRING_ESCAPE": {"invalid"},
	"DEFAULT_LINE_COMMENT":          {"lineComment"},
	"DEFAULT_BLOCK_COMMENT":         {"lineComment"},
	"DEFAULT_DOC_COMMENT":           {"docComment"},
	"DEFAULT_DOC_COMMENT_TAG":       {"docCommentTag"},
	"DEFAULT_DOC_COMMENT_TAG_VALUE": {"docCommentTagValue"},
	"DEFAULT_DOC_MARKUP":            {"docCommentMarkup"},
	"DEFAULT_METADATA":              {"metadata"},
	"DEFAULT_OPERATION_SIGN":        {"operator"},
	"DEFAULT_PARENTHS":              {"operator"},
	"DEFAULT_BRACES":                {"operator"},
	"DEFAULT_BRACKETS":              {"operator"},
	"DEFAULT_PREDEFINED_SYMBOL":     {"predefined"},
	"DEFAULT_MARKUP_TAG":            {"markupTag"},
	"DEFAULT_MARKUP_ATTRIBUTE":      {"markupAttribute"},
	"BAD_CHARACTER":                 {"invalid"},
	"TODO_DEFAULT_ATTRIBUTES":       {"todo"},
	"ERRORS_ATTRIBUTES":             {"error"},
	"WARNING_ATTRIBUTES":            {"warning"},
	"WEAK_WARNING_ATTRIBUTES":       {"weakWarning"},
	"NOT_USED_ELEMENT_ATTRIBUTES":   {"unused"},
	"JAVA_KEYWORD":                  {"java.keyword"},
	"JAVA_NUMBER":                   {"java.number"},
	"JAVA_STRING":                   {"java.string"},
	"JAVA_LINE_COMMENT":             {"java.lineComment"},
	"JAVA_BLOCK_COMMENT":            {"java.lineComment"},
	"JAVA_DOC_COMMENT":              {"java.docComment"},
	"JAVA_CLASS_NAME":               {"java.className"},
	"JAVA_LOCAL_VARIABLE":           {"java.identifier"},
	"JAVA_PARAMETER":                {"java.parameter"},
	"JAVA_INSTANCE_FIELD":           {"java.field"},
	"JAVA_STATIC_FIELD":             {"java.constant"},
	"JAVA_FUNCTION_DECLARATION":     {"java.functionDeclaration"},
	"JAVA_FUNCTION_CALL":            {"java.functionCall"},
	"JAVA_OPERATION_SIGN":           {"java.operator"},
	"JAVA_DOC_TAG":                  {"java.docCommentTag"},
	"PY.KEYWORD":                    {"pyKeyword"},
	"PY.NUMBER":                     {"pyNumber"},
	"PY.STRING":                     {"pyString"},
	"PY.LINE_COMMENT":               {"pyLineComment"},
	"PY.DOC_COMMENT":                {"pyDocstring"},
	"PY.FUNC_DEFINITION":            {"pyFunctionDeclaration"},
	"PY.PARAMETER":                  {"pyParameter"},
	"PY.OPERATION_SIGN":             {"pyOperationSign"},
	"REGEXP":                        {"regexp"},
}

var iclsExportKeys = map[string]string{
	"text": "TEXT", "keyword": "DEFAULT_KEYWORD", "identifier": "DEFAULT_IDENTIFIER",
	"parameter": "DEFAULT_PARAMETER", "globalVariable": "DEFAULT_GLOBAL_VARIABLE",
	"constant": "DEFAULT_CONSTANT", "field": "DEFAULT_INSTANCE_FIELD",
	"functionDeclaration": "DEFAULT_FUNCTION_DECLARATION", "functionCall": "DEFAULT_FUNCTION_CALL",
	"className": "DEFAULT_CLASS_NAME", "number": "DEFAULT_NUMBER", "string": "DEFAULT_STRING",
	"escape": "DEFAULT_VALID_STRING_ESCAPE", "invalid": "DEFAULT_INVALID_STRING_ESCAPE",
	"lineComment": "DEFAULT_LINE_COMMENT", "docComment": "DEFAULT_DOC_COMMENT",
	"docCommentTag": "DEFAULT_DOC_COMMENT_TAG", "docCommentTagValue": "DEFAULT_DOC_COMMENT_TAG_VALUE",
	"docCommentMarkup": "DEFAULT_DOC_MARKUP", "metadata": "DEFAULT_METADATA",
	"operator": "DEFAULT_OPERATION_SIGN", "todo": "TODO_DEFAULT_ATTRIBUTES",
	"error": "ERRORS_ATTRIBUTES", "warning": "WARNING_ATTRIBUTES",
	"weakWarning": "WEAK_WARNING_ATTRIBUTES", "unused": "NOT_USED_ELEMENT_ATTRIBUTES",
	"java.keyword": "JAVA_KEYWORD", "java.number": "JAVA_NUMBER", "java.string": "JAVA_STRING",
	"java.lineComment": "JAVA_LINE_COMMENT", "java.docComment": "JAVA_DOC_COMMENT",
	"java.className": "JAVA_CLASS_NAME", "java.identifier": "JAVA_LOCAL_VARIABLE",
	"java.parameter": "JAVA_PARAMETER", "java.field": "JAVA_INSTANCE_FIELD",
	"java.constant": "JAVA_STATIC_FIELD", "java.functionDeclaration": "JAVA_FUNCTION_DECLARATION",
	"java.functionCall": "JAVA_FUNCTION_CALL", "java.operator": "JAVA_OPERATION_SIGN",
	"java.docCommentTag": "JAVA_DOC_TAG",
	"pyKeyword":          "PY.KEYWORD", "pyNumber": "PY.NUMBER", "pyString": "PY.STRING",
	"pyLineComment": "PY.LINE_COMMENT", "pyDocstring": "PY.DOC_COMMENT",
	"pyFunctionDeclaration": "PY.FUNC_DEFINITION", "pyParameter": "PY.PARAMETER",
	"pyOperationSign": "PY.OPERATION_SIGN",
	"predefined":      "DEFAULT_PREDEFINED_SYMBOL",
	"markupTag":       "DEFAULT_MARKUP_TAG",
	"markupAttribute": "DEFAULT_MARKUP_ATTRIBUTE",
	"regexp":          "REGEXP",
}

func decodeBackendEditorSchemeSettings(raw json.RawMessage) (backendEditorSchemeSettings, error) {
	var out backendEditorSchemeSettings
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return out, nil
	}
	err := json.Unmarshal(raw, &out)
	return out, err
}

func (m *Model) mergeBackendEditorSchemeSettings(settings backendEditorSchemeSettings) {
	next := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	changed := false
	if settings.Editor.SemanticHighlighting != nil &&
		next.SemanticHighlighting != *settings.Editor.SemanticHighlighting {
		next.SemanticHighlighting = *settings.Editor.SemanticHighlighting
		changed = true
	}
	if settings.Editor.RainbowBrackets != nil &&
		next.RainbowBrackets != *settings.Editor.RainbowBrackets {
		next.RainbowBrackets = *settings.Editor.RainbowBrackets
		changed = true
	}
	if settings.Editor.ColorScheme != nil &&
		next.ColorScheme != normalizeColorSchemeID(*settings.Editor.ColorScheme) {
		next.ColorScheme = normalizeColorSchemeID(*settings.Editor.ColorScheme)
		changed = true
	}
	if settings.Editor.CustomSchemes != nil {
		customs := sanitizeCustomSchemes(settings.Editor.CustomSchemes)
		if !reflect.DeepEqual(next.CustomSchemes, customs) {
			next.CustomSchemes = customs
			changed = true
		}
	}
	if settings.Editor.ColorOverrides != nil {
		overrides := sanitizeColorOverrides(settings.Editor.ColorOverrides)
		if !reflect.DeepEqual(next.ColorOverrides, overrides) {
			next.ColorOverrides = overrides
			changed = true
		}
	}
	if changed {
		if err := m.setEditorSchemePreferences(next); err != nil {
			m.editorSchemeError("Cannot save the color scheme settings", err)
		}
	}
}

func normalizeColorSchemeID(id string) string {
	switch strings.TrimSpace(id) {
	case "":
		return themeDefaultColorScheme
	case "openidea-dark":
		return "dark"
	case "openidea-light":
		return "light"
	case "high-contrast":
		return "dark-hc"
	default:
		return strings.TrimSpace(id)
	}
}

func (m *Model) applyEditorSchemeNativePages(pages map[string]nativeSettingsPageState) {
	next := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	changed := false
	for page, state := range pages {
		if page != settingsPageColorScheme && page != settingsPageColorSchemeGen &&
			page != settingsPageColorSchemeLang && page != settingsPageColorSchemeDebug &&
			page != settingsPageColorSchemeCons {
			continue
		}
		if state.draft == nil {
			continue
		}
		if value, ok := state.draft[editorSemanticHighlightingKey].(bool); ok &&
			next.SemanticHighlighting != value {
			next.SemanticHighlighting = value
			changed = true
		}
		if value, ok := state.draft[editorRainbowBracketsKey].(bool); ok &&
			next.RainbowBrackets != value {
			next.RainbowBrackets = value
			changed = true
		}
		if value, ok := state.draft[editorColorSchemeKey].(string); ok &&
			value != "" && next.ColorScheme != value {
			next.ColorScheme = value
			changed = true
		}
	}
	if changed {
		if err := m.setEditorSchemePreferences(next); err != nil {
			m.editorSchemeError("Cannot save the color scheme settings", err)
		}
	}
}

func cloneEditorSchemePrefs(value EditorSchemePrefs) EditorSchemePrefs {
	out := value
	out.CustomSchemes = make(map[string]CustomColorScheme, len(value.CustomSchemes))
	for id, scheme := range value.CustomSchemes {
		out.CustomSchemes[id] = cloneCustomColorScheme(scheme)
	}
	out.ColorOverrides = make(map[string]ColorAttributeSet, len(value.ColorOverrides))
	for scheme, attributes := range value.ColorOverrides {
		next := make(ColorAttributeSet, len(attributes))
		for id, attribute := range attributes {
			next[id] = cloneColorAttributes(attribute)
		}
		out.ColorOverrides[scheme] = next
	}
	return out
}

func cloneCustomColorScheme(value CustomColorScheme) CustomColorScheme {
	out := value
	out.Attributes = make(map[string]ColorAttributes, len(value.Attributes))
	for id, attribute := range value.Attributes {
		out.Attributes[id] = cloneColorAttributes(attribute)
	}
	return out
}

func cloneColorAttributes(value ColorAttributes) ColorAttributes {
	out := value
	if value.Bold != nil {
		on := *value.Bold
		out.Bold = &on
	}
	if value.Italic != nil {
		on := *value.Italic
		out.Italic = &on
	}
	return out
}

func sanitizeCustomSchemes(values map[string]CustomColorScheme) map[string]CustomColorScheme {
	out := make(map[string]CustomColorScheme, len(values))
	for id, scheme := range values {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		scheme.Name = strings.TrimSpace(scheme.Name)
		if scheme.Name == "" {
			scheme.Name = id
		}
		scheme.Parent = normalizeSchemeParent(scheme.Parent, scheme.Attributes)
		out[id] = cloneCustomColorScheme(scheme)
	}
	return out
}

func sanitizeColorOverrides(values map[string]ColorAttributeSet) map[string]ColorAttributeSet {
	out := make(map[string]ColorAttributeSet, len(values))
	for scheme, attributes := range values {
		if strings.TrimSpace(scheme) == "" || len(attributes) == 0 {
			continue
		}
		next := make(ColorAttributeSet, len(attributes))
		for id, attribute := range attributes {
			if strings.TrimSpace(id) != "" {
				next[id] = cloneColorAttributes(attribute)
			}
		}
		if len(next) > 0 {
			out[scheme] = next
		}
	}
	return out
}

func (m *Model) setEditorSchemePreferences(next EditorSchemePrefs) error {
	m.prefs.EditorScheme = cloneEditorSchemePrefs(next)
	rememberEditorColorSchemeRegistry(m.prefs.EditorScheme)
	rememberEditorColorSchemeActions(m.prefs.EditorScheme)
	m.syncNativeEditorSchemeState(m.prefs.EditorScheme)
	m.applyEditorColorSchemeTheme()
	m.refreshEditorSchemeBackgrounds()
	m.invalidate()
	return m.savePreferences()
}

func (m *Model) syncNativeEditorSchemeState(prefs EditorSchemePrefs) {
	for _, page := range []string{
		settingsPageColorScheme, settingsPageColorSchemeGen,
		settingsPageColorSchemeLang, settingsPageColorSchemeDebug,
		settingsPageColorSchemeCons,
	} {
		state := m.nativeSettings.pages[page]
		if !state.loaded {
			continue
		}
		if state.applied == nil {
			state.applied = map[string]any{}
		}
		if state.draft == nil {
			state.draft = map[string]any{}
		}
		state.applied[editorColorSchemeKey] = prefs.ColorScheme
		state.draft[editorColorSchemeKey] = prefs.ColorScheme
		if page == settingsPageColorSchemeLang {
			state.applied[editorSemanticHighlightingKey] = prefs.SemanticHighlighting
			state.draft[editorSemanticHighlightingKey] = prefs.SemanticHighlighting
			state.applied[editorRainbowBracketsKey] = prefs.RainbowBrackets
			state.draft[editorRainbowBracketsKey] = prefs.RainbowBrackets
		}
		m.nativeSettings.pages[page] = state
	}
}

func (m *Model) editorSchemeAvailable() bool {
	return m != nil && (m.mgr != nil || m.invokeHook != nil)
}

func (m *Model) toggleSemanticHighlighting() {
	m.toggleEditorSchemeSwitch(editorSemanticHighlightingKey,
		!m.prefs.EditorScheme.SemanticHighlighting)
}

func (m *Model) toggleRainbowBrackets() {
	m.toggleEditorSchemeSwitch(editorRainbowBracketsKey,
		!m.prefs.EditorScheme.RainbowBrackets)
}

func (m *Model) toggleEditorSchemeSwitch(key string, on bool) {
	if m.editorSchemeBusy {
		return
	}
	next := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	label := "Semantic highlighting"
	if key == editorRainbowBracketsKey {
		next.RainbowBrackets = on
		label = "Rainbow brackets"
	} else {
		next.SemanticHighlighting = on
	}
	state := "off"
	if on {
		state = "on"
	}
	m.persistEditorScheme(next,
		[]settingsBackendWrite{{scope: "global", key: key, value: on}},
		label+" "+state)
}

func (m *Model) persistEditorScheme(next EditorSchemePrefs, writes []settingsBackendWrite, status string) {
	if !m.editorSchemeAvailable() {
		m.editorSchemeBusy = false
		if err := m.setEditorSchemePreferences(next); err != nil {
			m.editorSchemeError("Cannot save the color scheme settings", err)
			return
		}
		if status != "" {
			m.status = status
		}
		m.invalidate()
		return
	}
	m.editorSchemeBusy = true
	m.status = "Changing color scheme…"
	m.invalidate()
	previous := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		var applied []settingsBackendWrite
		for _, write := range writes {
			if err := m.invokeSettingsSet(ctx, write.scope, write.key, write.value); err != nil {
				rollbackErr := m.rollbackEditorSchemeWrites(previous, applied)
				m.post(func() {
					m.editorSchemeBusy = false
					m.editorSchemeError("Cannot change the color scheme setting",
						errors.Join(err, rollbackErr))
				})
				return
			}
			applied = append(applied, write)
		}
		m.post(func() {
			m.editorSchemeBusy = false
			if err := m.setEditorSchemePreferences(next); err != nil {
				m.editorSchemeError("Cannot save the color scheme settings", err)
				return
			}
			if status != "" {
				m.status = status
			}
			m.invalidate()
		})
	}()
}

func (m *Model) rollbackEditorSchemeWrites(
	previous EditorSchemePrefs,
	applied []settingsBackendWrite,
) error {
	if len(applied) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(modelContext(m), 10*time.Second)
	defer cancel()
	var rollbackErr error
	for index := len(applied) - 1; index >= 0; index-- {
		write := applied[index]
		value, ok := editorSchemePreviousValue(previous, write.key)
		if !ok {
			continue
		}
		if err := m.invokeSettingsSet(ctx, write.scope, write.key, value); err != nil {
			rollbackErr = errors.Join(rollbackErr,
				fmt.Errorf("rollback %s: %w", write.key, err))
		}
	}
	return rollbackErr
}

func editorSchemePreviousValue(prefs EditorSchemePrefs, key string) (any, bool) {
	switch key {
	case editorSemanticHighlightingKey:
		return prefs.SemanticHighlighting, true
	case editorRainbowBracketsKey:
		return prefs.RainbowBrackets, true
	case editorColorSchemeKey:
		return prefs.ColorScheme, true
	case editorCustomSchemesKey:
		return prefs.CustomSchemes, true
	case editorColorOverridesKey:
		return prefs.ColorOverrides, true
	default:
		return nil, false
	}
}

func (m *Model) editorSchemeError(title string, err error) {
	if err == nil {
		return
	}
	m.status = title + ": " + err.Error()
	m.banner = m.status
	m.bannerPath = ""
	m.invalidate()
}

func parseICLS(text, fallbackName string) (parsedColorScheme, error) {
	fallbackName = strings.TrimSpace(fallbackName)
	if fallbackName == "" {
		fallbackName = "Imported scheme"
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var (
		stack   []string
		section string
		scheme  = CustomColorScheme{Name: fallbackName, Parent: "dark",
			Attributes: map[string]ColorAttributes{}}
		foundScheme  bool
		outerOption  int
		attrNames    []string
		attrOverride ColorAttributes
		effectType   string
		applied      int
		skipped      int
	)
	parent := func() string {
		if len(stack) == 0 {
			return ""
		}
		return stack[len(stack)-1]
	}
	merge := func(names []string, value ColorAttributes) {
		for _, name := range names {
			current := scheme.Attributes[name]
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
			scheme.Attributes[name] = current
		}
	}
	resetAttribute := func() {
		outerOption = 0
		attrNames = nil
		attrOverride = ColorAttributes{}
		effectType = ""
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return parsedColorScheme{}, fmt.Errorf("not valid XML: %w", err)
		}
		switch item := token.(type) {
		case xml.StartElement:
			parentName := parent()
			stack = append(stack, item.Name.Local)
			switch item.Name.Local {
			case "scheme":
				foundScheme = true
				if name := strings.TrimSpace(attrValue(item.Attr, "name")); name != "" {
					scheme.Name = name
				}
				scheme.Parent = schemeParentFromName(attrValue(item.Attr, "parent_scheme"))
			case "colors", "attributes":
				if parentName == "scheme" {
					section = item.Name.Local
				}
			case "option":
				optionName := attrValue(item.Attr, "name")
				switch {
				case section == "colors" && parentName == "colors":
					target, ok := iclsColorTargets[optionName]
					color := normalizedICLSCColor(attrValue(item.Attr, "value"))
					if ok && color != "" {
						value := ColorAttributes{}
						if target.Field == "background" {
							value.Background = color
						} else {
							value.Foreground = color
						}
						merge([]string{target.Attribute}, value)
						applied++
					} else {
						skipped++
					}
				case section == "attributes" && parentName == "attributes":
					outerOption = len(stack)
					attrNames = resolveICLSTargets(optionName)
					attrOverride = ColorAttributes{}
					if len(attrNames) == 0 {
						skipped++
						resetAttribute()
					} else {
						if iclsTargetsUseTokenStyle(attrNames) {
							plain := false
							attrOverride.Bold = &plain
							attrOverride.Italic = &plain
						}
						if strings.HasPrefix(optionName, "PY.") {
							attrOverride.Effect = "none"
						}
					}
				case section == "attributes" && parentName == "value" && outerOption > 0:
					raw := attrValue(item.Attr, "value")
					switch optionName {
					case "FOREGROUND":
						if color := normalizedICLSCColor(raw); color != "" {
							attrOverride.Foreground = color
							applied++
						} else {
							skipped++
						}
					case "BACKGROUND":
						if color := normalizedICLSCColor(raw); color != "" {
							attrOverride.Background = color
							applied++
						} else {
							skipped++
						}
					case "EFFECT_COLOR":
						if color := normalizedICLSCColor(raw); color != "" {
							attrOverride.EffectColor = color
							applied++
						} else {
							skipped++
						}
					case "FONT_TYPE":
						switch raw {
						case "0", "1", "2", "3":
							number, _ := strconv.Atoi(raw)
							bold := number&1 != 0
							italic := number&2 != 0
							attrOverride.Bold = &bold
							attrOverride.Italic = &italic
							applied++
						default:
							skipped++
						}
					case "EFFECT_TYPE":
						switch raw {
						case "1", "4", "5":
							effectType = "underline"
						case "2":
							effectType = "wave"
						case "3":
							effectType = "strikeout"
						default:
							skipped++
						}
					}
				}
			}
		case xml.EndElement:
			if item.Name.Local == "option" && outerOption > 0 &&
				len(stack) == outerOption && section == "attributes" {
				if effectType != "" {
					if attrOverride.EffectColor != "" {
						attrOverride.Effect = effectType
						applied++
					} else {
						skipped++
					}
				}
				merge(attrNames, attrOverride)
				resetAttribute()
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if item.Name.Local == "colors" || item.Name.Local == "attributes" {
				section = ""
			}
		}
	}
	if !foundScheme {
		return parsedColorScheme{}, errors.New("no IntelliJ <scheme> element found")
	}
	scheme.Attributes = sanitizeColorAttributes(scheme.Attributes)
	scheme.Parent = normalizeSchemeParent(scheme.Parent, scheme.Attributes)
	return parsedColorScheme{Scheme: scheme, Applied: applied, Skipped: skipped}, nil
}

func attrValue(attributes []xml.Attr, name string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func firstNonEmpty(current, next string) string {
	if next != "" {
		return next
	}
	return current
}

func normalizedICLSCColor(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
	if len(value) == 0 || len(value) > 6 {
		return ""
	}
	decodeValue := value
	if len(decodeValue)%2 != 0 {
		decodeValue = "0" + decodeValue
	}
	if _, err := hex.DecodeString(decodeValue); err != nil {
		return ""
	}
	value = strings.ToLower(value)
	if len(value) < 6 {
		value = strings.Repeat("0", 6-len(value)) + value
	}
	return "#" + value
}

func resolveICLSTargets(name string) []string {
	name = strings.TrimSpace(name)
	if targets, ok := iclsAttributeTargets[name]; ok {
		return append([]string(nil), targets...)
	}
	if id, ok := strings.CutPrefix(name, "GOIDEA_"); ok {
		return []string{id}
	}
	return nil
}

func iclsTargetsUseTokenStyle(ids []string) bool {
	for _, id := range ids {
		if _, ok := editorSchemeAttributeKind(id); ok {
			return true
		}
	}
	return false
}

func sanitizeColorAttributes(values map[string]ColorAttributes) map[string]ColorAttributes {
	out := make(map[string]ColorAttributes, len(values))
	for id, value := range values {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		value.Foreground = normalizedICLSCColor(value.Foreground)
		value.Background = normalizedICLSCColor(value.Background)
		value.EffectColor = normalizedICLSCColor(value.EffectColor)
		if value.Effect != "none" && value.Effect != "underline" &&
			value.Effect != "wave" && value.Effect != "strikeout" {
			value.Effect = ""
		}
		if value.Foreground != "" || value.Background != "" || value.EffectColor != "" ||
			value.Bold != nil || value.Italic != nil || value.Effect != "" {
			out[id] = cloneColorAttributes(value)
		}
	}
	return out
}

func schemeParentFromName(name string) string {
	if strings.Contains(strings.ToLower(name), "darcula") ||
		strings.Contains(strings.ToLower(name), "dark") {
		return "dark"
	}
	if strings.Contains(strings.ToLower(name), "default") ||
		strings.Contains(strings.ToLower(name), "light") {
		return "light"
	}
	return ""
}

func normalizeSchemeParent(parent string, attributes map[string]ColorAttributes) string {
	parent = strings.ToLower(strings.TrimSpace(parent))
	if parent == "dark" || parent == "light" {
		return parent
	}
	if text := attributes["text"].Background; text != "" {
		value := strings.TrimPrefix(text, "#")
		if len(value) == 6 {
			r, _ := strconv.ParseInt(value[0:2], 16, 64)
			g, _ := strconv.ParseInt(value[2:4], 16, 64)
			b, _ := strconv.ParseInt(value[4:6], 16, 64)
			if float64(r)*0.299+float64(g)*0.587+float64(b)*0.114 < 128 {
				return "dark"
			}
			return "light"
		}
	}
	if resolved := schemeParentFromName(parent); resolved != "" {
		return resolved
	}
	return "light"
}

func (m *Model) ImportColorScheme(arg any) {
	if m.editorSchemeBusy {
		return
	}
	path, _ := arg.(string)
	snapshot := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	m.editorSchemeBusy = true
	go func() {
		if strings.TrimSpace(path) == "" {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: m.win,
				Title:  "Import Color Scheme",
				Filters: []mygo.FileFilter{{
					Name:       "IntelliJ Color Scheme",
					Extensions: []string{"icls", "xml"},
				}},
			})
			if err != nil {
				m.post(func() {
					m.editorSchemeBusy = false
					m.editorSchemeError("Cannot import the color scheme", err)
				})
				return
			}
			if len(paths) == 0 {
				m.post(func() { m.editorSchemeBusy = false; m.invalidate() })
				return
			}
			path = paths[0]
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			m.post(func() {
				m.editorSchemeBusy = false
				m.editorSchemeError("Cannot import the color scheme", err)
			})
			return
		}
		fallback := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		parsed, err := parseICLS(string(raw), fallback)
		if err != nil {
			m.post(func() {
				m.editorSchemeBusy = false
				m.editorSchemeError("Invalid .icls color scheme", err)
			})
			return
		}
		duplicateID, duplicateBuiltin := findColorSchemeName(snapshot, parsed.Scheme.Name)
		overwrite := false
		if duplicateID != "" || duplicateBuiltin {
			buttons := []string{"Keep Both", "Cancel"}
			if duplicateID != "" {
				buttons = []string{"Overwrite", "Keep Both", "Cancel"}
			}
			response, dialogErr := mygo.Dialog.Message(mygo.MessageOptions{
				Parent: m.win, Type: mygo.MessageQuestion, Title: "Import Color Scheme",
				Message: "A scheme named '" + parsed.Scheme.Name + "' already exists.",
				Buttons: buttons, DefaultButton: len(buttons) - 2, CancelButton: len(buttons) - 1,
			})
			if dialogErr != nil {
				m.post(func() {
					m.editorSchemeBusy = false
					m.editorSchemeError("Cannot import the color scheme", dialogErr)
				})
				return
			}
			if duplicateID == "" {
				if response.Button != 0 {
					m.post(func() { m.editorSchemeBusy = false; m.invalidate() })
					return
				}
			} else {
				switch response.Button {
				case 0:
					overwrite = true
				case 1:
				default:
					m.post(func() { m.editorSchemeBusy = false; m.invalidate() })
					return
				}
			}
		}
		m.post(func() {
			next, err := applyImportedColorScheme(snapshot, parsed.Scheme, overwrite, newColorSchemeID())
			if err != nil {
				m.editorSchemeBusy = false
				m.editorSchemeError("Cannot import the color scheme", err)
				return
			}
			m.persistEditorScheme(next, colorSchemeSettingsWrites(next, true),
				fmt.Sprintf("Applied %d values and skipped %d in %s", parsed.Applied, parsed.Skipped, parsed.Scheme.Name))
		})
	}()
}

func findColorSchemeName(prefs EditorSchemePrefs, name string) (customID string, builtin bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for id, scheme := range prefs.CustomSchemes {
		if strings.ToLower(strings.TrimSpace(scheme.Name)) == name {
			return id, false
		}
	}
	for _, choice := range knownColorSchemeChoices() {
		if strings.ToLower(strings.TrimSpace(choice.label)) == name {
			return "", true
		}
	}
	return "", false
}

func applyImportedColorScheme(
	prefs EditorSchemePrefs,
	scheme CustomColorScheme,
	overwrite bool,
	newID string,
) (EditorSchemePrefs, error) {
	if strings.TrimSpace(scheme.Name) == "" {
		return prefs, errors.New("the imported scheme has no name")
	}
	next := cloneEditorSchemePrefs(prefs)
	if next.CustomSchemes == nil {
		next.CustomSchemes = map[string]CustomColorScheme{}
	}
	existingID, _ := findColorSchemeName(next, scheme.Name)
	id := existingID
	if !overwrite || existingID == "" {
		name := scheme.Name
		for suffix := 2; schemeNameExists(next, name); suffix++ {
			name = fmt.Sprintf("%s (%d)", scheme.Name, suffix)
		}
		scheme.Name = name
		id = newID
	}
	if id == "" {
		id = newColorSchemeID()
	}
	if existingID != "" && overwrite {
		delete(next.ColorOverrides, id)
	}
	next.CustomSchemes[id] = cloneCustomColorScheme(scheme)
	next.ColorScheme = id
	return next, nil
}

func schemeNameExists(prefs EditorSchemePrefs, name string) bool {
	if _, builtin := findColorSchemeName(prefs, name); builtin {
		return true
	}
	for _, scheme := range prefs.CustomSchemes {
		if strings.EqualFold(strings.TrimSpace(scheme.Name), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func newColorSchemeID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fmt.Sprintf("custom-%d", time.Now().UnixNano())
	}
	return "custom-" + hex.EncodeToString(id[:])
}

func colorSchemeSettingsWrites(prefs EditorSchemePrefs, includeSelection bool) []settingsBackendWrite {
	writes := []settingsBackendWrite{
		{scope: "global", key: editorCustomSchemesKey, value: prefs.CustomSchemes},
		{scope: "global", key: editorColorOverridesKey, value: prefs.ColorOverrides},
	}
	if includeSelection {
		writes = append(writes, settingsBackendWrite{
			scope: "global", key: editorColorSchemeKey, value: prefs.ColorScheme,
		})
	}
	return writes
}

func (m *Model) ExportColorScheme(arg any) {
	path, _ := arg.(string)
	prefs := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	id := prefs.ColorScheme
	if id == "" {
		id = themeDefaultColorScheme
	}
	name, _ := colorSchemeDisplayName(prefs, id)
	if strings.TrimSpace(path) == "" {
		saved, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      m.win,
			Title:       "Export Color Scheme",
			DefaultPath: colorSchemeFileName(name) + ".icls",
			Filters: []mygo.FileFilter{{
				Name:       "IntelliJ Color Scheme",
				Extensions: []string{"icls"},
			}},
		})
		if err != nil {
			m.editorSchemeError("Cannot export the color scheme", err)
			return
		}
		path = saved
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	raw, err := exportICLSWithBase(id, prefs, m.theme)
	if err != nil {
		m.editorSchemeError("Cannot export the color scheme", err)
		return
	}
	if err := writeFileAtomic(path, []byte(raw)); err != nil {
		m.editorSchemeError("Cannot export the color scheme", err)
		return
	}
	m.status = "Color scheme exported to " + path
	m.invalidate()
}

func colorSchemeDisplayName(prefs EditorSchemePrefs, id string) (string, bool) {
	if scheme, ok := prefs.CustomSchemes[id]; ok {
		return scheme.Name, true
	}
	for _, choice := range knownColorSchemeChoices() {
		if choice.value == id {
			return choice.label, true
		}
	}
	if id == themeDefaultColorScheme {
		return "Theme default", true
	}
	return id, false
}

func colorSchemeFileName(name string) string {
	replacer := strings.NewReplacer("/", "_", `\`, "_", ":", "_", "*", "_",
		"?", "_", `"`, "_", "<", "_", ">", "_", "|", "_")
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" {
		return "color-scheme"
	}
	return name
}

func exportICLS(id string, prefs EditorSchemePrefs) (string, error) {
	return exportICLSWithBase(id, prefs, openideaTheme(false))
}

func exportICLSWithBase(id string, prefs EditorSchemePrefs, base *ui.Theme) (string, error) {
	name, found := colorSchemeDisplayName(prefs, id)
	if !found && id != themeDefaultColorScheme {
		return "", fmt.Errorf("color scheme %q was not found", id)
	}
	parent := "dark"
	attributes := editorSchemeExportAttributes(prefs, id, base)
	if custom, ok := prefs.CustomSchemes[id]; ok {
		parent = normalizeSchemeParent(custom.Parent, custom.Attributes)
	}
	var output strings.Builder
	output.WriteString(`<scheme name="`)
	if err := xml.EscapeText(&output, []byte(name)); err != nil {
		return "", err
	}
	output.WriteString(`" version="142" parent_scheme="`)
	if parent == "dark" {
		output.WriteString("Darcula")
	} else {
		output.WriteString("Default")
	}
	output.WriteString("\">\n  <colors>\n")
	var colorKeys []string
	for key := range attributes {
		if target, ok := exportedColorTarget(key); ok {
			colorKeys = append(colorKeys, target+":"+key)
		}
	}
	sort.Strings(colorKeys)
	for _, pair := range colorKeys {
		parts := strings.SplitN(pair, ":", 2)
		value := attributes[parts[1]]
		color := value.Background
		field := "background"
		switch parts[1] {
		case "caret", "selectionForeground", "lineNumber", "lineNumberCaret",
			"whitespaces", "indentGuide", "rightMargin", "methodSeparator":
			color, field = value.Foreground, "foreground"
		default:
			_ = field
		}
		if color != "" {
			output.WriteString(`    <option name="` + parts[0] + `" value="`)
			output.WriteString(strings.TrimPrefix(color, "#"))
			output.WriteString("\" />\n")
		}
	}
	output.WriteString("  </colors>\n  <attributes>\n")
	var ids []string
	for key := range attributes {
		ids = append(ids, key)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, ok := exportedColorTarget(id); ok {
			continue
		}
		value := attributes[id]
		var options strings.Builder
		appendColorOption := func(name, color string) {
			if normalizedICLSCColor(color) == "" {
				return
			}
			options.WriteString(`<option name="` + name + `" value="` +
				strings.TrimPrefix(normalizedICLSCColor(color), "#") + `" />`)
		}
		appendColorOption("FOREGROUND", value.Foreground)
		appendColorOption("BACKGROUND", value.Background)
		appendColorOption("EFFECT_COLOR", value.EffectColor)
		fontType := 0
		if value.Bold != nil && *value.Bold {
			fontType |= 1
		}
		if value.Italic != nil && *value.Italic {
			fontType |= 2
		}
		options.WriteString(`<option name="FONT_TYPE" value="` +
			strconv.Itoa(fontType) + `" />`)
		if effect := effectCode(value.Effect); effect != "" {
			options.WriteString(`<option name="EFFECT_TYPE" value="` + effect + `" />`)
		}
		if options.Len() == 0 {
			continue
		}
		key := iclsExportKeys[id]
		if key == "" {
			key = "GOIDEA_" + id
		}
		output.WriteString(`    <option name="` + xmlEscape(key) + `"><value>`)
		output.WriteString(options.String())
		output.WriteString("</value></option>\n")
	}
	output.WriteString("  </attributes>\n</scheme>\n")
	return output.String(), nil
}

func exportedColorTarget(id string) (string, bool) {
	for key, target := range iclsColorTargets {
		if target.Attribute == id {
			return key, true
		}
	}
	return "", false
}

func effectCode(effect string) string {
	switch effect {
	case "underline":
		return "1"
	case "wave":
		return "2"
	case "strikeout":
		return "3"
	}
	return ""
}

func xmlEscape(value string) string {
	var out strings.Builder
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

func (m *Model) DeleteColorScheme(arg any) {
	if m.editorSchemeBusy {
		return
	}
	prefs := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	id, _ := arg.(string)
	if strings.TrimSpace(id) == "" {
		id = prefs.ColorScheme
	}
	selected, ok := prefs.CustomSchemes[id]
	if !ok {
		m.status = "Only imported color schemes can be deleted"
		m.invalidate()
		return
	}
	m.editorSchemeBusy = true
	go func() {
		response, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: m.win, Type: mygo.MessageWarning, Title: "Delete Color Scheme",
			Message: "Delete color scheme '" + selected.Name + "'?",
			Buttons: []string{"Delete", "Cancel"}, DefaultButton: 0, CancelButton: 1,
		})
		if err != nil {
			m.post(func() {
				m.editorSchemeBusy = false
				m.editorSchemeError("Cannot delete the color scheme", err)
			})
			return
		}
		if response.Button != 0 {
			m.post(func() { m.editorSchemeBusy = false; m.invalidate() })
			return
		}
		m.post(func() {
			next := deleteEditorColorScheme(prefs, id)
			m.persistEditorScheme(next, colorSchemeSettingsWrites(next, true),
				"Deleted color scheme "+selected.Name)
		})
	}()
}

func deleteEditorColorScheme(prefs EditorSchemePrefs, id string) EditorSchemePrefs {
	next := cloneEditorSchemePrefs(prefs)
	delete(next.CustomSchemes, id)
	delete(next.ColorOverrides, id)
	if next.ColorScheme == id {
		next.ColorScheme = themeDefaultColorScheme
	}
	return next
}

func knownColorSchemeChoices() []nativeSettingsChoice {
	return []nativeSettingsChoice{
		nativeChoice(themeDefaultColorScheme, "Theme default"),
		nativeChoice("light", "Islands Light"),
		nativeChoice("dark", "Islands Dark"),
		nativeChoice("dark-hc", "High Contrast"),
		nativeChoice("solarized-light", "Solarized Light"),
		nativeChoice("solarized-dark", "Solarized Dark"),
	}
}

func (m *Model) colorSchemeChoices(base []nativeSettingsChoice) []nativeSettingsChoice {
	choices := append([]nativeSettingsChoice(nil), base...)
	seen := map[string]bool{}
	for _, choice := range choices {
		seen[choice.value] = true
	}
	ids := make([]string, 0, len(m.prefs.EditorScheme.CustomSchemes))
	for id := range m.prefs.EditorScheme.CustomSchemes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		choices = append(choices, nativeChoice(id, m.prefs.EditorScheme.CustomSchemes[id].Name))
	}
	return choices
}
