package app

// IntelliJ theme conversion follows OpenIDEA's intellijTheme.ts. The backend
// reads a jar, zip, installed plugin folder or loose .theme.json and returns
// the theme JSON and optional .icls scheme. This layer maps the same UI and
// editor keys into OpenIDEA theme tokens and writes the same plugin package
// shape used by Install from Disk.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"mayoi/internal/openidea"
)

const (
	intellijPartForeground = "FOREGROUND"
	intellijPartBackground = "BACKGROUND"
	intellijPartEffect     = "EFFECT_COLOR"
	intellijPartStripe     = "ERROR_STRIPE_COLOR"
)

type intellijThemeFile struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Kind   string            `json:"kind"`
	Tokens map[string]string `json:"tokens"`
}

type intellijSchemeFile struct {
	Name   string            `json:"name"`
	Tokens map[string]string `json:"tokens"`
}

type intellijConvertedTheme struct {
	Theme  intellijThemeFile   `json:"theme"`
	Scheme *intellijSchemeFile `json:"scheme"`
}

type intellijThemePackageInfo struct {
	ID          string
	Name        string
	Version     string
	Description string
	Authors     []string
}

type intellijThemePackageFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

type intellijThemePackage struct {
	ID    string
	Name  string
	Files []intellijThemePackageFile
}

type intellijAttributeTarget struct {
	Part  string
	Token string
}

type intellijICLSAttribute struct {
	Name  string
	Empty bool
	Parts map[string]string
}

var intellijUIKeys = map[string][]string{
	"--bg-app":              {"Panel.background"},
	"--bg-frame":            {"MainWindow.background", "MainToolbar.background", "Panel.background"},
	"--bg-island":           {"ToolWindow.background", "Panel.background"},
	"--bg-elev":             {"ToolWindow.background", "SidePanel.background"},
	"--bg-popup":            {"Popup.background", "PopupMenu.background"},
	"--bg-floating-toolbar": {"Toolbar.Floating.background"},
	"--bg-backdrop":         {"Popup.background"},
	"--bg-hover":            {"List.hoverBackground", "Tree.hoverBackground"},
	"--bg-selected":         {"List.selectionBackground", "Tree.selectionBackground"},
	"--bg-selected-strong":  {"List.selectionBackground"},
	"--bg-tree-selected":    {"Tree.selectionBackground", "List.selectionBackground"},
	"--bg-tree-selected-inactive": {
		"Tree.selectionInactiveBackground",
		"List.selectionInactiveBackground",
	},
	"--bg-input":                {"TextField.background"},
	"--bg-field":                {"TextField.background"},
	"--bg-dialog":               {"Dialog.background", "Panel.background"},
	"--border":                  {"Borders.color"},
	"--border-popup":            {"Popup.borderColor", "Popup.Border.color"},
	"--border-strong":           {"Component.borderColor", "Borders.ContrastBorderColor"},
	"--border-window":           {"Borders.ContrastBorderColor", "Component.borderColor"},
	"--fg":                      {"Label.foreground"},
	"--fg-emphasis":             {"Label.foreground"},
	"--fg-strong":               {"Label.selectedForeground", "Label.foreground"},
	"--fg-secondary":            {"Label.infoForeground", "Component.infoForeground"},
	"--fg-muted":                {"Label.infoForeground", "Component.infoForeground"},
	"--fg-subtle":               {"Label.disabledForeground"},
	"--fg-faint":                {"Label.disabledForeground"},
	"--accent":                  {"Button.default.startBackground", "Button.default.background", "Component.focusColor"},
	"--fg-on-accent":            {"Button.default.foreground"},
	"--fg-button-primary":       {"Button.default.foreground"},
	"--focus-ring-soft":         {"Component.focusColor"},
	"--link":                    {"Link.activeForeground", "Hyperlink.linkColor"},
	"--link-hover":              {"Link.hoverForeground", "Link.activeForeground"},
	"--danger":                  {"Component.errorFocusColor"},
	"--warning":                 {"Component.warningFocusColor"},
	"--bg-tooltip":              {"ToolTip.background"},
	"--fg-tooltip":              {"ToolTip.foreground"},
	"--border-tooltip":          {"ToolTip.borderColor"},
	"--bg-button":               {"Button.startBackground", "Button.background"},
	"--border-button":           {"Button.startBorderColor", "Button.borderColor"},
	"--bg-checkbox":             {"Checkbox.Background.Default"},
	"--border-checkbox":         {"Checkbox.Border.Default"},
	"--bg-tab-selected":         {"EditorTabs.underlinedTabBackground", "EditorTabs.selectedBackground"},
	"--border-tab-selected":     {"EditorTabs.underlinedBorderColor", "EditorTabs.underlineColor"},
	"--bg-tab-hover":            {"EditorTabs.hoverBackground"},
	"--border-editor-tabs":      {"EditorTabs.borderColor"},
	"--bg-toolbar-hover":        {"ActionButton.hoverBackground"},
	"--bg-toolbar-pressed":      {"ActionButton.pressedBackground"},
	"--bg-stripe-hover":         {"ToolWindow.Button.hoverBackground"},
	"--bg-stripe-selected":      {"ToolWindow.Button.selectedBackground"},
	"--scrollbar-thumb":         {"ScrollBar.Mac.thumbColor", "ScrollBar.thumbColor"},
	"--scrollbar-thumb-hover":   {"ScrollBar.Mac.hoverThumbColor", "ScrollBar.hoverThumbColor"},
	"--bg-progress-track":       {"ProgressBar.trackColor"},
	"--bg-counter":              {"Counter.background"},
	"--fg-counter":              {"Counter.foreground"},
	"--bg-search-match":         {"SearchMatch.startBackground", "SearchMatch.endBackground"},
	"--fg-completion-match":     {"CompletionPopup.matchForeground"},
	"--bg-toggle-on":            {"ToggleButton.onBackground"},
	"--fg-toggle-on":            {"ToggleButton.onForeground"},
	"--bg-toggle-off":           {"ToggleButton.offBackground"},
	"--fg-toggle-off":           {"ToggleButton.offForeground"},
	"--border-toggle":           {"ToggleButton.borderColor"},
	"--bg-toggle-knob":          {"ToggleButton.buttonColor"},
	"--bg-feedback-info":        {"Banner.infoBackground"},
	"--border-feedback-info":    {"Banner.infoBorderColor"},
	"--bg-feedback-error":       {"Banner.errorBackground"},
	"--border-feedback-error":   {"Banner.errorBorderColor"},
	"--bg-feedback-warning":     {"Banner.warningBackground"},
	"--border-feedback-warning": {"Banner.warningBorderColor"},
	"--bg-feedback-success":     {"Banner.successBackground"},
	"--border-feedback-success": {"Banner.successBorderColor"},
}

var intellijICLSColors = map[string][]string{
	"CARET_COLOR":                                      {"--caret"},
	"CARET_ROW_COLOR":                                  {"--bg-current-line"},
	"SELECTION_BACKGROUND":                             {"--bg-editor-selection"},
	"SELECTION_FOREGROUND":                             {"--fg-editor-selection"},
	"LINE_NUMBERS_COLOR":                               {"--fg-line-number"},
	"LINE_NUMBER_ON_CARET_ROW_COLOR":                   {"--fg-line-number-current"},
	"EDITOR_GUTTER_BACKGROUND":                         {"--bg-editor-gutter"},
	"INDENT_GUIDE":                                     {"--border-indent-guide"},
	"SELECTED_INDENT_GUIDE":                            {"--border-indent-guide-active"},
	"RIGHT_MARGIN_COLOR":                               {"--border-right-margin"},
	"WHITESPACES":                                      {"--fg-whitespace"},
	"METHOD_SEPARATORS_COLOR":                          {"--border-method-separator"},
	"FILESTATUS_MODIFIED":                              {"--vcs-modified"},
	"FILESTATUS_ADDED":                                 {"--vcs-added"},
	"FILESTATUS_DELETED":                               {"--vcs-deleted"},
	"FILESTATUS_UNKNOWN":                               {"--vcs-untracked"},
	"FILESTATUS_IDEA_FILESTATUS_IGNORED":               {"--vcs-ignored"},
	"FILESTATUS_IDEA_FILESTATUS_MERGED_WITH_CONFLICTS": {"--vcs-conflict"},
	"MODIFIED_LINES_COLOR":                             {"--gutter-modified"},
	"ADDED_LINES_COLOR":                                {"--gutter-added"},
	"DELETED_LINES_COLOR":                              {"--gutter-deleted"},
	"CONSOLE_BACKGROUND_KEY":                           {"--bg-console"},
}

var intellijICLSAttributes = map[string][]intellijAttributeTarget{
	"TEXT": {
		{intellijPartForeground, "--fg-editor"},
		{intellijPartBackground, "--bg-editor"},
	},
	"DIFF_MODIFIED": {
		{intellijPartBackground, "--diff-modified"},
		{intellijPartStripe, "--diff-modified-stripe"},
	},
	"DIFF_INSERTED": {
		{intellijPartBackground, "--diff-inserted"},
		{intellijPartStripe, "--diff-inserted-stripe"},
	},
	"DIFF_DELETED": {
		{intellijPartBackground, "--diff-deleted"},
		{intellijPartStripe, "--diff-deleted-stripe"},
	},
	"DIFF_CONFLICT":                 {{intellijPartStripe, "--diff-conflict-stripe"}},
	"DEFAULT_KEYWORD":               {{intellijPartForeground, "--syn-keyword"}},
	"DEFAULT_STRING":                {{intellijPartForeground, "--syn-string"}},
	"DEFAULT_NUMBER":                {{intellijPartForeground, "--syn-number"}},
	"DEFAULT_LINE_COMMENT":          {{intellijPartForeground, "--syn-comment"}},
	"DEFAULT_DOC_COMMENT":           {{intellijPartForeground, "--syn-doc-comment"}},
	"DEFAULT_DOC_COMMENT_TAG":       {{intellijPartForeground, "--syn-doc-comment-tag"}},
	"DEFAULT_DOC_COMMENT_TAG_VALUE": {{intellijPartForeground, "--syn-doc-comment-tag-value"}},
	"DEFAULT_DOC_MARKUP": {
		{intellijPartForeground, "--syn-doc-markup"},
		{intellijPartBackground, "--syn-doc-markup-bg"},
	},
	"DEFAULT_FUNCTION_DECLARATION": {{intellijPartForeground, "--syn-function"}},
	"DEFAULT_FUNCTION_CALL":        {{intellijPartForeground, "--syn-function-call"}},
	"DEFAULT_CLASS_NAME":           {{intellijPartForeground, "--syn-type"}},
	"DEFAULT_CONSTANT":             {{intellijPartForeground, "--syn-constant"}},
	"DEFAULT_INSTANCE_FIELD":       {{intellijPartForeground, "--syn-property"}},
	"DEFAULT_IDENTIFIER":           {{intellijPartForeground, "--syn-variable"}},
	"DEFAULT_LOCAL_VARIABLE":       {{intellijPartForeground, "--syn-variable"}},
	"DEFAULT_PARAMETER":            {{intellijPartForeground, "--syn-param"}},
	"DEFAULT_GLOBAL_VARIABLE":      {{intellijPartForeground, "--syn-global"}},
	"DEFAULT_PREDEFINED_SYMBOL":    {{intellijPartForeground, "--syn-builtin"}},
	"DEFAULT_METADATA": {
		{intellijPartForeground, "--syn-annotation"},
		{intellijPartForeground, "--syn-decorator"},
	},
	"DEFAULT_OPERATION_SIGN":        {{intellijPartForeground, "--syn-punct"}},
	"DEFAULT_VALID_STRING_ESCAPE":   {{intellijPartForeground, "--syn-escape"}},
	"DEFAULT_TAG":                   {{intellijPartForeground, "--syn-tag"}},
	"DEFAULT_ATTRIBUTE":             {{intellijPartForeground, "--syn-attribute"}},
	"TODO_DEFAULT_ATTRIBUTES":       {{intellijPartForeground, "--syn-todo"}},
	"BAD_CHARACTER":                 {{intellijPartForeground, "--syn-invalid"}},
	"RAINBOW_COLOR1":                {{intellijPartForeground, "--syn-rainbow-1"}},
	"RAINBOW_COLOR2":                {{intellijPartForeground, "--syn-rainbow-2"}},
	"RAINBOW_COLOR3":                {{intellijPartForeground, "--syn-rainbow-3"}},
	"RAINBOW_COLOR4":                {{intellijPartForeground, "--syn-rainbow-4"}},
	"RAINBOW_COLOR5":                {{intellijPartForeground, "--syn-rainbow-5"}},
	"ERRORS_ATTRIBUTES":             {{intellijPartEffect, "--wave-error"}},
	"WARNING_ATTRIBUTES":            {{intellijPartEffect, "--wave-warning"}},
	"NOT_USED_ELEMENT_ATTRIBUTES":   {{intellijPartForeground, "--fg-unused"}},
	"MATCHED_BRACE_ATTRIBUTES":      {{intellijPartBackground, "--bg-matched-brace"}},
	"SEARCH_RESULT_ATTRIBUTES":      {{intellijPartBackground, "--bg-search-result"}},
	"TEXT_SEARCH_RESULT_ATTRIBUTES": {{intellijPartBackground, "--bg-text-search-result"}},
	"IDENTIFIER_UNDER_CARET_ATTRIBUTES": {
		{intellijPartBackground, "--bg-identifier-under-caret"},
	},
	"WRITE_IDENTIFIER_UNDER_CARET_ATTRIBUTES": {
		{intellijPartBackground, "--bg-identifier-under-caret-write"},
	},
	"FOLDED_TEXT_ATTRIBUTES": {
		{intellijPartForeground, "--fg-fold-placeholder"},
		{intellijPartBackground, "--bg-fold-placeholder"},
	},
	"INLINE_PARAMETER_HINT": {
		{intellijPartForeground, "--fg-inlay"},
		{intellijPartBackground, "--bg-inlay"},
	},
	"BREAKPOINT_ATTRIBUTES":       {{intellijPartBackground, "--bg-breakpoint-line"}},
	"DEBUGGER_INLINED_VALUES":     {{intellijPartForeground, "--syn-inline-value"}},
	"PY.SELF_PARAMETER":           {{intellijPartForeground, "--syn-self"}},
	"PY.KEYWORD_ARGUMENT":         {{intellijPartForeground, "--syn-keyword-arg"}},
	"PY.DOC_COMMENT":              {{intellijPartForeground, "--syn-docstring"}},
	"PY.DOC_COMMENT_TAG":          {{intellijPartForeground, "--syn-python-docstring-tag"}},
	"OPENIDEA_pyDocstringKeyword": {{intellijPartForeground, "--syn-python-docstring-keyword"}},
	"PY.BUILTIN_NAME":             {{intellijPartForeground, "--syn-predefined"}},
	"MARKDOWN_HEADER_LEVEL_1":     {{intellijPartForeground, "--syn-markup-heading"}},
	"MARKDOWN_CODE_SPAN":          {{intellijPartForeground, "--syn-markup-code"}},
	"MARKDOWN_LINK_TEXT":          {{intellijPartForeground, "--syn-link"}},
	"CONSOLE_BLACK_OUTPUT": {
		{intellijPartForeground, "--ansi-black"},
		{intellijPartForeground, "--terminal-ansi-black"},
	},
	"CONSOLE_RED_OUTPUT": {
		{intellijPartForeground, "--ansi-red"},
		{intellijPartForeground, "--terminal-ansi-red"},
	},
	"CONSOLE_GREEN_OUTPUT": {
		{intellijPartForeground, "--ansi-green"},
		{intellijPartForeground, "--terminal-ansi-green"},
	},
	"CONSOLE_YELLOW_OUTPUT": {
		{intellijPartForeground, "--ansi-yellow"},
		{intellijPartForeground, "--terminal-ansi-yellow"},
	},
	"CONSOLE_BLUE_OUTPUT": {
		{intellijPartForeground, "--ansi-blue"},
		{intellijPartForeground, "--terminal-ansi-blue"},
	},
	"CONSOLE_MAGENTA_OUTPUT": {
		{intellijPartForeground, "--ansi-magenta"},
		{intellijPartForeground, "--terminal-ansi-magenta"},
	},
	"CONSOLE_CYAN_OUTPUT": {
		{intellijPartForeground, "--ansi-cyan"},
		{intellijPartForeground, "--terminal-ansi-cyan"},
	},
	"CONSOLE_GRAY_OUTPUT": {
		{intellijPartForeground, "--ansi-white"},
		{intellijPartForeground, "--terminal-ansi-white"},
	},
	"CONSOLE_DARKGRAY_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-black"},
		{intellijPartForeground, "--terminal-ansi-bright-black"},
	},
	"CONSOLE_RED_BRIGHT_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-red"},
		{intellijPartForeground, "--terminal-ansi-bright-red"},
	},
	"CONSOLE_GREEN_BRIGHT_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-green"},
		{intellijPartForeground, "--terminal-ansi-bright-green"},
	},
	"CONSOLE_YELLOW_BRIGHT_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-yellow"},
		{intellijPartForeground, "--terminal-ansi-bright-yellow"},
	},
	"CONSOLE_BLUE_BRIGHT_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-blue"},
		{intellijPartForeground, "--terminal-ansi-bright-blue"},
	},
	"CONSOLE_MAGENTA_BRIGHT_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-magenta"},
		{intellijPartForeground, "--terminal-ansi-bright-magenta"},
	},
	"CONSOLE_CYAN_BRIGHT_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-cyan"},
		{intellijPartForeground, "--terminal-ansi-bright-cyan"},
	},
	"CONSOLE_WHITE_OUTPUT": {
		{intellijPartForeground, "--ansi-bright-white"},
		{intellijPartForeground, "--terminal-ansi-bright-white"},
	},
}

var (
	intellijHexColorPattern = regexp.MustCompile(`(?i)^#?([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)
	intellijICLSHexPattern  = regexp.MustCompile(`^[0-9a-f]{1,8}$`)
	intellijSemverPattern   = regexp.MustCompile(`^\s*v?(\d{1,9})(?:\.(\d{1,9}))?(?:\.(\d{1,9}))?`)
	intellijSchemeTag       = regexp.MustCompile(`<scheme\b[^>]*>`)
	intellijColorsSection   = regexp.MustCompile(`<colors>([\s\S]*?)</colors>`)
	intellijAttrsSection    = regexp.MustCompile(`<attributes>([\s\S]*?)</attributes>`)
	intellijSelfClosingTag  = regexp.MustCompile(`<option\b[^>]*\/>`)
	intellijAttributeOption = regexp.MustCompile(
		`<option name="([^"]+)"[^>]*>\s*(<value\s*\/>|<value>([\s\S]*?)</value>)\s*</option>`,
	)
)

func flattenIntellijUI(value map[string]any, prefix string, out map[string]string) {
	for key, raw := range value {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		switch item := raw.(type) {
		case string:
			out[path] = item
		case map[string]any:
			flattenIntellijUI(item, path, out)
		}
	}
}

func resolveIntellijColor(value string, named map[string]string, depth int) string {
	if match := intellijHexColorPattern.FindStringSubmatch(strings.TrimSpace(value)); match != nil {
		return "#" + strings.ToLower(match[1])
	}
	reference, ok := named[value]
	if !ok || depth >= 8 {
		return ""
	}
	return resolveIntellijColor(reference, named, depth+1)
}

func intellijICLSColor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !intellijICLSHexPattern.MatchString(value) {
		return ""
	}
	if len(value) <= 6 {
		return "#" + strings.Repeat("0", 6-len(value)) + value
	}
	return "#" + strings.Repeat("0", 8-len(value)) + value
}

func intellijXMLAttribute(tag, name string) string {
	expression := regexp.MustCompile(regexp.QuoteMeta(name) + `="([^"]*)"`)
	match := expression.FindStringSubmatch(tag)
	if match == nil {
		return ""
	}
	return match[1]
}

func setIntellijToken(tokens map[string]string, names []string, color string) {
	if color == "" {
		return
	}
	for _, name := range names {
		if _, exists := tokens[name]; !exists {
			tokens[name] = color
		}
	}
}

func convertIntellijICLS(xml string) intellijSchemeFile {
	tokens := map[string]string{}
	schemeTag := ""
	if match := intellijSchemeTag.FindString(xml); match != "" {
		schemeTag = match
	}
	name := intellijXMLAttribute(schemeTag, "name")
	if name == "" {
		name = "Imported"
	}

	colors := ""
	if match := intellijColorsSection.FindStringSubmatch(xml); match != nil {
		colors = match[1]
	}
	for _, tag := range intellijSelfClosingTag.FindAllString(colors, -1) {
		key := intellijXMLAttribute(tag, "name")
		value := intellijXMLAttribute(tag, "value")
		if names := intellijICLSColors[key]; key != "" && names != nil {
			setIntellijToken(tokens, names, intellijICLSColor(value))
		}
	}

	attributes := ""
	if match := intellijAttrsSection.FindStringSubmatch(xml); match != nil {
		attributes = match[1]
	}
	var (
		parsed         []intellijICLSAttribute
		attributeIndex = map[string]int{}
	)
	for _, indices := range intellijAttributeOption.FindAllStringSubmatchIndex(attributes, -1) {
		attributeName := attributes[indices[2]:indices[3]]
		valueTag := attributes[indices[4]:indices[5]]
		parts := map[string]string{}
		empty := strings.HasPrefix(valueTag, "<value />") || strings.HasPrefix(valueTag, "<value/>")
		if !empty {
			for _, tag := range intellijSelfClosingTag.FindAllString(attributes[indices[6]:indices[7]], -1) {
				part := intellijXMLAttribute(tag, "name")
				if part != "" {
					parts[part] = intellijXMLAttribute(tag, "value")
				}
			}
		}
		next := intellijICLSAttribute{Name: attributeName, Empty: empty, Parts: parts}
		if index, exists := attributeIndex[attributeName]; exists {
			next.Empty = next.Empty || parsed[index].Empty
			parsed[index] = next
		} else {
			attributeIndex[attributeName] = len(parsed)
			parsed = append(parsed, next)
		}
	}

	textForeground := ""
	for _, attribute := range parsed {
		if attribute.Name == "TEXT" {
			textForeground = intellijICLSColor(attribute.Parts["FOREGROUND"])
			break
		}
	}
	for _, attribute := range parsed {
		targets := intellijICLSAttributes[attribute.Name]
		if targets == nil {
			continue
		}
		if attribute.Empty {
			for _, target := range targets {
				if target.Part == intellijPartForeground {
					setIntellijToken(tokens, []string{target.Token}, textForeground)
				}
			}
			continue
		}
		for _, target := range targets {
			setIntellijToken(tokens, []string{target.Token}, intellijICLSColor(attribute.Parts[target.Part]))
		}
		if attribute.Name == "DEFAULT_LINE_COMMENT" {
			if fontType, ok := attribute.Parts["FONT_TYPE"]; ok {
				number, _ := strconv.Atoi(fontType)
				if number&2 != 0 {
					tokens["--syn-comment-style"] = "italic"
				} else {
					tokens["--syn-comment-style"] = "normal"
				}
			}
		}
	}
	if color := tokens["--wave-error"]; color != "" {
		tokens["--editor-error-decoration"] = "underline wavy " + color
	}
	if color := tokens["--bg-editor"]; color != "" {
		if _, exists := tokens["--bg-editor-gutter"]; !exists {
			tokens["--bg-editor-gutter"] = color
		}
	}
	return intellijSchemeFile{Name: name, Tokens: tokens}
}

func inferIntellijSchemeDark(xml string) (bool, bool) {
	schemeTag := ""
	if match := intellijSchemeTag.FindString(xml); match != "" {
		schemeTag = match
	}
	parent := intellijXMLAttribute(schemeTag, "parent_scheme")
	if parent == "" {
		return false, false
	}
	dark := regexp.MustCompile(`(?i)darcula|dark`).MatchString(parent)
	return dark, true
}

func slugIntellijTheme(name string) string {
	value := strings.ToLower(name)
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 48 {
		value = value[:48]
	}
	if value == "" {
		return "imported"
	}
	return value
}

func convertIntellijTheme(themeJSON string, editorScheme *string, id *string) (intellijConvertedTheme, error) {
	var parsed any
	if err := json.NewDecoder(strings.NewReader(themeJSON)).Decode(&parsed); err != nil {
		return intellijConvertedTheme{}, fmt.Errorf("invalid IntelliJ theme JSON: %w", err)
	}
	source, ok := parsed.(map[string]any)
	if !ok {
		return intellijConvertedTheme{}, fmt.Errorf("Not an IntelliJ theme")
	}

	name := "Imported Theme"
	if value, ok := source["name"].(string); ok {
		name = value
	}
	named := map[string]string{}
	if colors, ok := source["colors"].(map[string]any); ok {
		for key, value := range colors {
			if color, ok := value.(string); ok {
				named[key] = color
			}
		}
	}
	ui := map[string]string{}
	if values, ok := source["ui"].(map[string]any); ok {
		flattenIntellijUI(values, "", ui)
	}
	lookup := func(key string) string {
		value, ok := ui[key]
		if !ok {
			last := key
			if at := strings.LastIndex(key, "."); at >= 0 {
				last = key[at+1:]
			}
			value, ok = ui["*."+last]
		}
		if !ok {
			return ""
		}
		return resolveIntellijColor(value, named, 0)
	}

	tokens := map[string]string{}
	for token, keys := range intellijUIKeys {
		for _, key := range keys {
			if color := lookup(key); color != "" {
				tokens[token] = color
				break
			}
		}
	}

	var scheme *intellijSchemeFile
	if editorScheme != nil {
		converted := convertIntellijICLS(*editorScheme)
		scheme = &converted
	}
	dark := true
	if value, ok := source["dark"].(bool); ok {
		dark = value
	} else if editorScheme != nil {
		if parentDark, known := inferIntellijSchemeDark(*editorScheme); known {
			dark = parentDark
		}
	}
	if scheme != nil {
		if editorBackground := scheme.Tokens["--bg-editor"]; editorBackground != "" {
			if _, exists := tokens["--bg-island"]; !exists {
				tokens["--bg-island"] = editorBackground
			}
		}
	}

	themeID := slugIntellijTheme(name)
	if id != nil {
		themeID = *id
	}
	kind := "light"
	if dark {
		kind = "dark"
	}
	return intellijConvertedTheme{
		Theme: intellijThemeFile{
			ID: themeID, Name: name, Kind: kind, Tokens: tokens,
		},
		Scheme: scheme,
	}, nil
}

func tomlBasicString(value string) string {
	var body strings.Builder
	for _, character := range value {
		switch {
		case character == '"' || character == '\\':
			body.WriteByte('\\')
			body.WriteRune(character)
		case character < 0x20 || character == 0x7f:
			fmt.Fprintf(&body, `\u%04x`, character)
		case character >= 0xd800 && character <= 0xdfff:
			body.WriteRune('\ufffd')
		default:
			body.WriteRune(character)
		}
	}
	return `"` + body.String() + `"`
}

func intellijPluginManifest(info intellijThemePackageInfo, themes []intellijConvertedTheme, comment string) string {
	var lines []string
	if comment != "" {
		lines = append(lines, "# "+regexp.MustCompile(`[\r\n]+`).ReplaceAllString(comment, " "))
	}
	lines = append(lines,
		"id = "+tomlBasicString(info.ID),
		"name = "+tomlBasicString(info.Name),
		"version = "+tomlBasicString(info.Version),
		"description = "+tomlBasicString(info.Description),
		"authors = ["+joinTomlStrings(info.Authors)+"]",
		`engines.openidea = "^1.0"`,
	)
	for _, converted := range themes {
		lines = append(lines,
			"",
			"[[contributes.themes]]",
			"path = "+tomlBasicString("themes/"+converted.Theme.ID+".theme.json"),
		)
		if converted.Scheme != nil {
			lines = append(lines,
				"editor-scheme = "+tomlBasicString("themes/"+converted.Theme.ID+".scheme.json"),
			)
		}
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func joinTomlStrings(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, tomlBasicString(value))
	}
	return strings.Join(quoted, ", ")
}

func semverIntellijTheme(version *string) string {
	value := ""
	if version != nil {
		value = *version
	}
	match := intellijSemverPattern.FindStringSubmatch(value)
	if match == nil {
		return "1.0.0"
	}
	major, _ := strconv.Atoi(match[1])
	minor, patch := 0, 0
	if match[2] != "" {
		minor, _ = strconv.Atoi(match[2])
	}
	if match[3] != "" {
		patch, _ = strconv.Atoi(match[3])
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

func indentedJSON(value any) string {
	var output strings.Builder
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
	return output.String()
}

func intellijThemePackageFromSource(source openidea.IntellijThemeSource) (intellijThemePackage, error) {
	ids := map[string]bool{}
	themes := make([]intellijConvertedTheme, 0, len(source.Themes))
	for _, item := range source.Themes {
		converted, err := convertIntellijTheme(item.Theme, item.EditorScheme, nil)
		if err != nil {
			return intellijThemePackage{}, fmt.Errorf("%s: %w", item.ID, err)
		}
		base := converted.Theme.ID
		id := base
		for number := 2; ids[id]; number++ {
			prefix := base
			if len(prefix) > 44 {
				prefix = prefix[:44]
			}
			id = fmt.Sprintf("%s-%d", prefix, number)
		}
		ids[id] = true
		converted.Theme.ID = id
		themes = append(themes, converted)
	}
	if len(themes) == 0 {
		return intellijThemePackage{}, fmt.Errorf("the plugin has no themes")
	}

	pluginName := source.Name
	pluginID := source.Name
	if source.PluginID != nil {
		pluginID = *source.PluginID
	}
	info := intellijThemePackageInfo{
		ID:          "intellij." + slugIntellijTheme(pluginID),
		Name:        pluginName,
		Version:     semverIntellijTheme(source.Version),
		Description: pluginName + ", imported from an IntelliJ theme plugin.",
	}
	if source.Vendor != nil && *source.Vendor != "" {
		info.Authors = []string{*source.Vendor}
	}

	files := []intellijThemePackageFile{{
		Path: "plugin.toml",
		Text: intellijPluginManifest(info, themes, ""),
	}}
	for _, converted := range themes {
		files = append(files,
			intellijThemePackageFile{
				Path: "themes/" + converted.Theme.ID + ".theme.json",
				Text: indentedJSON(converted.Theme),
			},
		)
		if converted.Scheme != nil {
			files = append(files, intellijThemePackageFile{
				Path: "themes/" + converted.Theme.ID + ".scheme.json",
				Text: indentedJSON(converted.Scheme),
			})
		}
	}
	return intellijThemePackage{ID: info.ID, Name: info.Name, Files: files}, nil
}
