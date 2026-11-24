package app

import (
	"context"
	"time"
)

// runRuntimeSettings is the current project's applied Run/Debug page. It is
// replaced whenever the project changes, while RunPrefs remains a migration
// fallback for settings files created before these values were project-scoped.
type runRuntimeSettings struct {
	root               string
	loaded             bool
	confirmRerun       bool
	confirmTerminate   bool
	showOnStart        bool
	terminationTimeout time.Duration
}

// applyRunRuntimeSettings copies the applied Run/Debug page into the native
// runtime for the project that page belongs to.
func (m *Model) applyRunRuntimeSettings(pages map[string]nativeSettingsPageState) bool {
	state, ok := pages[settingsPageRunDebug]
	if !ok || !state.loaded {
		return false
	}
	if state.scope == "project" && state.root != m.Root() {
		return false
	}

	next := runRuntimeSettings{
		root:             state.root,
		loaded:           true,
		confirmRerun:     nativeAppliedBool(state, "run.confirmRerun", true),
		confirmTerminate: nativeAppliedBool(state, "run.confirmTerminate", true),
		showOnStart:      nativeAppliedBool(state, "run.showOnStart", true),
		terminationTimeout: time.Duration(max(0, min(
			nativeAppliedInt(state, "run.terminationTimeoutMs", 3000),
			600_000,
		))) * time.Millisecond,
	}
	if state.scope == "" && next.root == "" {
		next.root = m.Root()
	}
	if m.run.runSettings == next {
		return false
	}
	m.run.runSettings = next
	return true
}

func nativeAppliedBool(state nativeSettingsPageState, key string, fallback bool) bool {
	value, ok := state.applied[key]
	if !ok {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func nativeAppliedInt(state nativeSettingsPageState, key string, fallback int) int {
	value, ok := state.applied[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return fallback
	}
}

// setRunRuntimeBool applies a "Do not ask again" answer immediately and
// persists it at the current settings scope. Project values no longer leak
// into the legacy global preferences.
func (m *Model) setRunRuntimeBool(key string, value bool) {
	root := m.Root()
	runtime := m.run.runSettings
	if !runtime.loaded || runtime.root != root {
		runtime = runRuntimeSettings{
			root:               root,
			loaded:             true,
			confirmRerun:       m.confirmRerunEnabled(),
			confirmTerminate:   m.confirmTerminateEnabled(),
			showOnStart:        m.showRunOnStart(),
			terminationTimeout: m.runTerminationTimeout(),
		}
	}
	switch key {
	case "run.confirmRerun":
		runtime.confirmRerun = value
	case "run.confirmTerminate":
		runtime.confirmTerminate = value
	default:
		return
	}
	m.run.runSettings = runtime

	state := m.nativeSettings.pages[settingsPageRunDebug]
	if state.applied == nil {
		state.applied = map[string]any{}
	}
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	state.applied[key] = value
	state.draft[key] = value
	state.loaded = true
	state.root = root
	if root == "" {
		state.scope = "global"
	} else {
		state.scope = "project"
	}
	if m.nativeSettings.pages == nil {
		m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	}
	m.nativeSettings.pages[settingsPageRunDebug] = state

	if root == "" {
		if m.prefs.Run == nil {
			m.prefs.Run = &RunPrefs{}
		}
		confirmation := value
		switch key {
		case "run.confirmRerun":
			m.prefs.Run.ConfirmRerun = &confirmation
		case "run.confirmTerminate":
			m.prefs.Run.ConfirmTerminate = &confirmation
		}
		m.savePreferences()
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		return
	}
	go func(project, setting string) {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		err := m.invokeSettingsSet(ctx, "project", setting, value)
		if err == nil {
			return
		}
		m.post(func() {
			if m.Root() != project {
				return
			}
			m.settingsErr = "Could not save " + setting + ": " + err.Error()
			m.invalidate()
		})
	}(root, key)
}
