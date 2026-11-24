package app

// applyDebuggerRuntimeSettings copies the debugger pages loaded by the
// settings sheet into the native runtime. Settings_set remains authoritative
// for the backend, while these fields let editor contributions react without
// waiting for another RPC on every hover or stop.
func (m *Model) applyDebuggerRuntimeSettings(pages map[string]nativeSettingsPageState) bool {
	before := m.prefs.Debugger
	prefs := before

	if state, ok := pages[settingsPageDebugger]; ok && state.loaded {
		prefs.NoShowOnBreakpoint = !nativePageBool(state, "debugger.showOnBreakpoint", true)
		prefs.FocusAppOnBreakpoint = nativePageBool(state, "debugger.focusAppOnBreakpoint", false)
		prefs.HideOnTermination = nativePageBool(state, "debugger.hideOnTermination", false)
		prefs.BreakpointsAnyFile = nativePageBool(state, "debugger.breakpointsAnyFile", false)
		prefs.NoRunToCursorInlay = !nativePageBool(state, "debugger.runToCursorInlay", true)
	}
	if state, ok := pages[settingsPageDebuggerData]; ok && state.loaded {
		prefs.NoInlineValues = !nativePageBool(state, "debugger.inlineValues", true)
		prefs.SortValues = nativePageBool(state, "debugger.sortValues", false)
		prefs.NoValueTooltip = !nativePageBool(state, "debugger.valueTooltip", true)
		prefs.NoValueTooltipOnSelection =
			!nativePageBool(state, "debugger.valueTooltipOnSelection", true)
		delay := nativePageInt(state, "debugger.valueTooltipDelay", 700)
		prefs.ValueTooltipDelay = &delay
	}
	if state, ok := pages[settingsPageDebuggerStepping]; ok && state.loaded {
		prefs.NoSkipLibraryCode = !nativePageBool(state, "debugger.skipLibraryCode", true)
	}
	if state, ok := pages[settingsPagePythonDebugger]; ok && state.loaded {
		prefs.NoPythonAttachSubprocess =
			!nativePageBool(state, "debugger.python.attachSubprocess", true)
		prefs.PythonGevent = nativePageBool(state, "debugger.python.gevent", false)
	}

	if debuggerPrefsEqual(prefs, before) {
		return false
	}
	m.prefs.Debugger = prefs
	m.savePreferences()
	m.refreshDebuggerRuntime()
	return true
}

func (m *Model) refreshDebuggerRuntime() {
	for _, tab := range m.allTabs() {
		m.cancelDebugSelectionHover(tab)
	}
	m.refreshDecorations()
	m.invalidate()
}

func nativePageBool(state nativeSettingsPageState, key string, fallback bool) bool {
	value, ok := state.draft[key]
	if !ok {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func nativePageInt(state nativeSettingsPageState, key string, fallback int) int {
	value, ok := state.draft[key]
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
