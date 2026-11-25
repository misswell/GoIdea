package app

import (
	"fmt"
	"strings"
	"sync"
)

const editorColorSchemeActionPrefix = "editor.colorScheme."

var (
	editorColorSchemeRegistryMu sync.RWMutex
	editorColorSchemeRegistry   = map[string]editorSchemeActionReference{}
)

type editorSchemeActionReference struct {
	id    string
	label string
}

// editorSchemeActionGet resolves color-scheme actions that depend on the
// settings model, as OpenIDEA's editor.colorScheme.${name} family does.
func editorSchemeActionGet(id string) (*Action, bool) {
	editorColorSchemeRegistryMu.RLock()
	reference, remembered := editorColorSchemeRegistry[id]
	editorColorSchemeRegistryMu.RUnlock()
	if remembered {
		return newEditorColorSchemeAction(id, reference.id), true
	}
	name, ok := editorColorSchemeActionName(id)
	if !ok {
		return nil, false
	}
	schemeID, _, ok := lookupEditorColorScheme(EditorSchemePrefs{}, name)
	if !ok {
		return nil, false
	}
	return newEditorColorSchemeAction(id, schemeID), true
}

func rememberEditorColorSchemeRegistry(prefs EditorSchemePrefs) {
	editorColorSchemeRegistryMu.Lock()
	defer editorColorSchemeRegistryMu.Unlock()
	for id, scheme := range prefs.CustomSchemes {
		label := strings.TrimSpace(scheme.Name)
		if label == "" {
			label = id
		}
		for _, name := range []string{id, label, editorColorSchemeSlug(id), editorColorSchemeSlug(label)} {
			if name == "" {
				continue
			}
			editorColorSchemeRegistry[editorColorSchemeActionPrefix+name] = editorSchemeActionReference{id, label}
		}
	}
}

func editorSchemeActionGetForModel(m *Model, id string) (*Action, bool) {
	if m == nil {
		return nil, false
	}
	name, ok := editorColorSchemeActionName(id)
	if !ok {
		return nil, false
	}
	rememberEditorColorSchemeActions(m.prefs.EditorScheme)
	schemeID, _, ok := lookupEditorColorScheme(m.prefs.EditorScheme, name)
	if !ok {
		return nil, false
	}
	return newEditorColorSchemeAction(id, schemeID), true
}

func editorColorSchemeActionName(id string) (string, bool) {
	name, ok := strings.CutPrefix(id, editorColorSchemeActionPrefix)
	if !ok {
		return "", false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	switch name {
	case "import", "export", "delete",
		"general", "language-defaults", "debugger", "console-colors":
		return "", false
	}
	return name, true
}

func rememberEditorColorSchemeActions(prefs EditorSchemePrefs) {
	editorColorSchemeRegistryMu.Lock()
	defer editorColorSchemeRegistryMu.Unlock()
	remember := func(id, label string) {
		for _, name := range []string{id, label, editorColorSchemeSlug(id), editorColorSchemeSlug(label)} {
			if name == "" {
				continue
			}
			editorColorSchemeRegistry[editorColorSchemeActionPrefix+name] = editorSchemeActionReference{id, label}
		}
	}
	for _, choice := range knownColorSchemeChoices() {
		remember(choice.value, choice.label)
	}
	for id, scheme := range prefs.CustomSchemes {
		name := strings.TrimSpace(scheme.Name)
		if name == "" {
			name = id
		}
		remember(id, name)
	}
}

func newEditorColorSchemeAction(id, schemeID string) *Action {
	title := "Color Scheme"
	if schemeID != "" {
		title = "Color Scheme: " + editorColorSchemeActionLabelFromRegistry(schemeID)
	}
	return &Action{
		ID:       id,
		Title:    title,
		Category: "Editor",
		Keywords: "color scheme editor syntax highlighting theme",
		Toggled: func(m *Model) bool {
			return m != nil && schemeID != "" &&
				normalizeColorSchemeID(m.prefs.EditorScheme.ColorScheme) == schemeID
		},
		RunWithArg: func(m *Model, arg any) {
			m.applyEditorColorSchemeAction(schemeID, arg)
		},
	}
}

func editorColorSchemeActionLabelFromRegistry(id string) string {
	editorColorSchemeRegistryMu.RLock()
	label := ""
	for _, reference := range editorColorSchemeRegistry {
		if reference.id == id {
			label = strings.TrimSpace(reference.label)
			break
		}
	}
	editorColorSchemeRegistryMu.RUnlock()
	if label != "" {
		return label
	}
	if label, ok := colorSchemeDisplayName(EditorSchemePrefs{}, id); ok {
		return label
	}
	return id
}

func editorColorSchemeActionLabel(prefs EditorSchemePrefs, id string) string {
	if label, ok := colorSchemeDisplayName(prefs, id); ok {
		return label
	}
	if id == themeDefaultColorScheme {
		return "Theme default"
	}
	return id
}

func (m *Model) applyEditorColorSchemeAction(fallbackID string, arg any) {
	if m == nil || m.editorSchemeBusy {
		return
	}
	requested := fallbackID
	if value, ok := arg.(string); ok && strings.TrimSpace(value) != "" {
		requested = value
	}
	schemeID, name, ok := lookupEditorColorScheme(m.prefs.EditorScheme, requested)
	if !ok {
		m.editorSchemeError("Cannot apply the color scheme",
			fmt.Errorf("color scheme %q was not found", strings.TrimSpace(requested)))
		return
	}
	next := cloneEditorSchemePrefs(m.prefs.EditorScheme)
	next.ColorScheme = schemeID
	m.persistEditorScheme(next, []settingsBackendWrite{{
		scope: "global", key: editorColorSchemeKey, value: schemeID,
	}}, "Editor color scheme: "+name)
}

func lookupEditorColorScheme(prefs EditorSchemePrefs, value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	normalized := normalizeColorSchemeID(value)
	for _, choice := range knownColorSchemeChoices() {
		if editorColorSchemeNameMatches(value, normalized, choice.value, choice.label) {
			return choice.value, choice.label, true
		}
	}
	for id, scheme := range prefs.CustomSchemes {
		if editorColorSchemeNameMatches(value, normalized, id, scheme.Name) {
			name := strings.TrimSpace(scheme.Name)
			if name == "" {
				name = id
			}
			return id, name, true
		}
	}
	return "", "", false
}

func editorColorSchemeNameMatches(value, normalized, id, label string) bool {
	value = strings.TrimSpace(value)
	normalized = strings.TrimSpace(normalized)
	id = strings.TrimSpace(id)
	label = strings.TrimSpace(label)
	return strings.EqualFold(value, id) ||
		strings.EqualFold(normalized, id) ||
		strings.EqualFold(value, label) ||
		editorColorSchemeSlug(value) == editorColorSchemeSlug(id) ||
		editorColorSchemeSlug(value) == editorColorSchemeSlug(label)
}

func editorColorSchemeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "#", "sharp")
	value = strings.ReplaceAll(value, "_", "-")
	return strings.Join(strings.Fields(value), "-")
}
