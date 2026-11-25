package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// loadNativeRuntimeSettings reads the scopes used by native runtime consumers.
// Settings pages normally load lazily when opened. Runtime pages also need
// their stored values at startup and after a project switch.
func (m *Model) loadNativeRuntimeSettings(ctx context.Context) error {
	setProxyRuntime(m)
	if m.mgr == nil && m.invokeHook == nil {
		return errors.New("the GoIDEA backend is not connected")
	}
	root := m.Root()
	var globalRaw, projectRaw json.RawMessage
	var globalErr, projectErr error
	globalRaw, globalErr = m.invoke(ctx, "settings_get_global", nil)
	if root != "" {
		projectRaw, projectErr = m.invoke(ctx, "settings_get", nil)
	}
	var editorScheme backendEditorSchemeSettings
	var editorSchemeErr error
	if globalErr == nil {
		editorScheme, editorSchemeErr = decodeBackendEditorSchemeSettings(globalRaw)
	}

	defs := nativeSettingsDefinitions()
	m.post(func() {
		if root != m.Root() {
			return
		}
		if m.nativeSettings.pages == nil {
			m.nativeSettings.pages = map[string]nativeSettingsPageState{}
		}
		for _, def := range defs {
			scope := nativeSettingsPageScope(m, def)
			scopes := nativeSettingsPageScopes(m, def)
			roots := map[string]map[string]any{}
			var loadErr error
			for _, requestedScope := range scopes {
				raw, scopeErr := globalRaw, globalErr
				if requestedScope == "project" {
					if root == "" {
						continue
					}
					raw, scopeErr = projectRaw, projectErr
				}
				if scopeErr != nil {
					loadErr = scopeErr
					break
				}
				var parsed map[string]any
				if err := json.Unmarshal(raw, &parsed); err != nil {
					loadErr = err
					break
				}
				roots[requestedScope] = parsed
			}
			current := m.nativeSettings.pages[def.page.id]
			// A project-scoped page from another root must reload even when its
			// old project had an unapplied draft. Within the same root an
			// in-flight or dirty page still wins, so typing is never replaced.
			sameRoot := current.root == root
			if sameRoot && (current.loading || current.loaded && m.nativeSettingsDirty(def.page.id)) {
				continue
			}
			current.session = true
			current.loading = false
			current.scope = scope
			current.root = root
			current.loadGen++
			if loadErr != nil {
				current.loaded = false
				current.err = "Could not read " + def.page.title + ": " + loadErr.Error()
				m.nativeSettings.pages[def.page.id] = current
				continue
			}
			values := nativeSettingsPageValuesFromRoots(roots, def)
			current.loaded = true
			current.err = ""
			current.applied = nativeSettingsCloneValues(values)
			current.draft = nativeSettingsCloneValues(values)
			m.nativeSettings.pages[def.page.id] = current
		}
		if editorSchemeErr == nil {
			m.mergeBackendEditorSchemeSettings(editorScheme)
		}
		m.applyRunRuntimeSettings(m.nativeSettings.pages)
		m.invalidate()
	})
	return errors.Join(globalErr, projectErr, editorSchemeErr)
}

// nativeRuntimeValue reads a loaded page value and otherwise uses the same
// default the Settings page shows.
func (m *Model) nativeRuntimeValue(page, key string) any {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	return m.nativeRuntimeValueLocked(page, key)
}

func (m *Model) nativeRuntimeValueLocked(page, key string) any {
	if state, ok := m.nativeSettings.pages[page]; ok && state.loaded {
		if value, exists := state.applied[key]; exists && value != nil {
			return value
		}
	}
	def, ok := nativeSettingsDefinition(page)
	if !ok {
		return nil
	}
	for _, section := range def.sections {
		for _, field := range section.fields {
			if field.key == key {
				return nativeSettingsDefault(field)
			}
		}
	}
	return nil
}

func (m *Model) proxySnapshot() nativeProxySettings {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	state := m.nativeSettings.pages[settingsPageHTTPProxy]
	return proxySnapshotFromState(state)
}

func proxySnapshotFromState(state nativeSettingsPageState) nativeProxySettings {
	port := nativeAppliedInt(state, "proxy.port", 0)
	if port < 1 || port > 65535 {
		if strings.EqualFold(
			nativeAppliedString(state, "proxy.kind", ""),
			"socks",
		) {
			port = 1080
		} else {
			port = 80
		}
	}
	return nativeProxySettings{
		mode:         nativeAppliedString(state, "proxy.mode", "auto"),
		kind:         nativeAppliedString(state, "proxy.kind", "http"),
		host:         nativeAppliedString(state, "proxy.host", ""),
		port:         port,
		noProxy:      nativeAppliedString(state, "proxy.noProxy", ""),
		authenticate: nativeAppliedBool(state, "proxy.authenticate", false),
		login:        nativeAppliedString(state, "proxy.login", ""),
		password:     nativeAppliedString(state, "proxy.password", ""),
	}
}

func nativeAppliedString(state nativeSettingsPageState, key, fallback string) string {
	if value, ok := state.applied[key].(string); ok {
		return value
	}
	return fallback
}

func (m *Model) proxyPageLoaded() bool {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	state, ok := m.nativeSettings.pages[settingsPageHTTPProxy]
	return ok && state.loaded
}

func (m *Model) nativeRuntimeBool(page, key string) bool {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	return m.nativeRuntimeBoolLocked(page, key)
}

func (m *Model) nativeRuntimeBoolLocked(page, key string) bool {
	value, _ := m.nativeRuntimeValueLocked(page, key).(bool)
	return value
}

func (m *Model) nativeRuntimeString(page, key string) string {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	return m.nativeRuntimeStringLocked(page, key)
}

func (m *Model) nativeRuntimeStringLocked(page, key string) string {
	value, _ := m.nativeRuntimeValueLocked(page, key).(string)
	return value
}

func (m *Model) nativeRuntimeNumber(page, key string) float64 {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	return m.nativeRuntimeNumberLocked(page, key)
}

func (m *Model) nativeRuntimeNumberLocked(page, key string) float64 {
	switch value := m.nativeRuntimeValueLocked(page, key).(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		number, _ := value.Float64()
		return number
	default:
		return 0
	}
}

func (m *Model) nativeRuntimeStringList(page, key string) []string {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	return nativeSettingsStringList(m.nativeRuntimeValueLocked(page, key))
}

func (m *Model) nativeRuntimeStringMap(page, key string) map[string]string {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	out := map[string]string{}
	switch values := m.nativeRuntimeValueLocked(page, key).(type) {
	case map[string]string:
		for name, value := range values {
			out[name] = value
		}
	case map[string]any:
		for name, value := range values {
			if text, ok := value.(string); ok {
				out[name] = text
			}
		}
	}
	return out
}

func (m *Model) setNativeSettingsValue(page, key string, value any) {
	m.nativeSettingsMu.Lock()
	state := m.nativeSettings.pages[page]
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	state.draft[key] = value
	m.nativeSettings.pages[page] = state
	if key == "languages.python.interpreter" {
		for _, other := range []string{settingsPagePythonDeps, settingsPagePythonInterpret} {
			if other == page {
				continue
			}
			otherState := m.nativeSettings.pages[other]
			if !otherState.loaded {
				continue
			}
			if otherState.draft == nil {
				otherState.draft = map[string]any{}
			}
			otherState.draft[key] = value
			m.nativeSettings.pages[other] = otherState
		}
	}
	m.nativeSettingsMu.Unlock()
	m.invalidate()
}

func nativeSettingsLanguageDisabled(m *Model, page, key, language string) bool {
	m.nativeSettingsMu.Lock()
	defer m.nativeSettingsMu.Unlock()
	language = strings.ToLower(strings.TrimSpace(language))
	for _, value := range nativeSettingsStringList(m.nativeRuntimeValueLocked(page, key)) {
		if strings.EqualFold(strings.TrimSpace(value), language) {
			return true
		}
	}
	return false
}
