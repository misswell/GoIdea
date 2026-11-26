package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
	runtimeDefs := nativeRuntimeSettingsDefinitions()
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
		var globalRoot, projectRoot map[string]any
		if globalErr == nil {
			_ = json.Unmarshal(globalRaw, &globalRoot)
		}
		if projectErr == nil {
			_ = json.Unmarshal(projectRaw, &projectRoot)
		}
		for _, def := range runtimeDefs {
			current := m.nativeSettings.pages[def.page.id]
			if current.loading || (current.loaded && m.nativeSettingsDirty(def.page.id)) {
				continue
			}
			if globalErr != nil {
				current.loaded = false
				current.err = "Could not read " + def.consumer + ": " + globalErr.Error()
				m.nativeSettings.pages[def.page.id] = current
				continue
			}
			rootValues := globalRoot
			if root != "" {
				if projectErr != nil {
					current.loaded = false
					current.err = "Could not read " + def.consumer + ": " + projectErr.Error()
					m.nativeSettings.pages[def.page.id] = current
					continue
				}
				rootValues = mergeNativeSettingRoots(globalRoot, projectRoot)
			}
			current.session = true
			current.loading = false
			current.loaded = true
			current.scope = "mixed"
			current.root = root
			current.loadGen++
			current.err = ""
			values := nativeSettingsPageValuesFromRuntimeRoot(rootValues, def.sections, "global")
			current.applied = nativeSettingsCloneValues(values)
			current.draft = nativeSettingsCloneValues(values)
			m.nativeSettings.pages[def.page.id] = current
		}
		if editorSchemeErr == nil {
			m.mergeBackendEditorSchemeSettings(editorScheme)
		}
		m.applyRunRuntimeSettings(m.nativeSettings.pages)
		m.applyEditorSchemeNativePages(m.nativeSettings.pages)
		m.applyNativePreferenceRuntimeSettings(runtimeDefs)
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
	page, key = nativeRuntimeReading(page, key)
	if state, ok := m.nativeSettings.pages[page]; ok && state.loaded {
		if value, exists := state.applied[key]; exists && value != nil {
			return value
		}
	}
	if def, ok := nativeSettingsDefinition(page); ok {
		for _, section := range def.sections {
			for _, field := range section.fields {
				if field.key == key {
					return nativeSettingsDefault(field)
				}
			}
		}
	}
	for _, def := range nativeRuntimeSettingsDefinitions() {
		if def.page.id != page {
			continue
		}
		for _, section := range def.sections {
			for _, field := range section.fields {
				if field.key == key {
					return nativeSettingsDefault(field)
				}
			}
		}
	}
	return nil
}

// nativeRuntimeReading aliases a runtime catalog page back to the visible
// settings page that owns the same key. Existing consumers keep their page
// IDs while startup can load the key straight from the full settings root.
func nativeRuntimeReading(page, key string) (string, string) {
	if def, ok := nativeSettingsDefinition(page); ok {
		for _, section := range def.sections {
			for _, field := range section.fields {
				if field.key == key {
					return page, key
				}
			}
		}
	}
	for _, def := range nativeRuntimeSettingsDefinitions() {
		for _, section := range def.sections {
			for _, field := range section.fields {
				if field.key == key {
					return def.page.id, key
				}
			}
		}
	}
	return page, key
}

func mergeNativeSettingRoots(global, project map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range global {
		out[key] = cloneNativeJSONValue(value)
	}
	for key, value := range project {
		if child, ok := value.(map[string]any); ok {
			existing, _ := out[key].(map[string]any)
			out[key] = mergeNativeSettingRoots(existing, child)
			continue
		}
		out[key] = cloneNativeJSONValue(value)
	}
	return out
}

func nativeSettingsPageValuesFromRuntimeRoot(
	root map[string]any,
	sections []nativeSettingsSection,
	scope string,
) map[string]any {
	def := nativeSettingsPageSpec{
		defaultScope: scope,
		sections:     sections,
	}
	return nativeSettingsPageValuesFromRoots(
		map[string]map[string]any{scope: root},
		def,
	)
}

func nativeSettingsRuntimeValues(
	pages map[string]nativeSettingsPageState,
	defs []nativeRuntimeSettingsPage,
) map[string]any {
	values := map[string]any{}
	for _, def := range defs {
		state, ok := pages[def.page.id]
		if !ok || !state.loaded {
			continue
		}
		for key, value := range state.applied {
			values[key] = value
		}
	}
	// A visible page is loaded lazily and receives the applied value after a
	// successful backend write. Overlay those values so runtime behavior
	// changes immediately instead of waiting for the next startup reload.
	runtimeKeys := map[string]bool{}
	runtimePages := map[string]bool{}
	for _, def := range defs {
		runtimePages[def.page.id] = true
		for _, section := range def.sections {
			for _, field := range section.fields {
				runtimeKeys[field.key] = true
			}
		}
	}
	for page, state := range pages {
		if !state.loaded || runtimePages[page] {
			continue
		}
		for key, value := range state.applied {
			if runtimeKeys[key] && value != nil {
				values[key] = value
			}
		}
	}
	return values
}

func nativeSettingBool(values map[string]any, key string, fallback bool) bool {
	if value, ok := values[key].(bool); ok {
		return value
	}
	return fallback
}

func nativeSettingString(values map[string]any, key, fallback string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return fallback
}

func nativeSettingNumber(values map[string]any, key string, fallback float64) float64 {
	switch value := values[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		number, _ := value.Float64()
		return number
	default:
		return fallback
	}
}

func nativeSettingStringList(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok {
		return nil
	}
	return nativeSettingsStringList(value)
}

func nativeSettingJSONText(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "{}" || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

func nativeOptionalBool(value bool) *bool { return &value }

// applyNativePreferenceRuntimeSettings maps the keys already consumed by the
// native editor and agents into the local preference model. Settings that
// still have no native consumer deliberately are not added here.
func (m *Model) applyNativePreferenceRuntimeSettings(
	defs []nativeRuntimeSettingsPage,
) bool {
	values := nativeSettingsRuntimeValues(m.nativeSettings.pages, defs)
	if len(values) == 0 {
		return false
	}
	before := m.prefs

	agents := m.agentPrefs()
	agents.NewTaskApprovals = nativeSettingString(values, "agents.newTaskApprovals", agents.NewTaskApprovals)
	agents.SendWith = nativeSettingString(values, "agents.sendWith", agents.SendWith)
	agents.WhileRunning = nativeSettingString(values, "agents.whileRunning", agents.WhileRunning)
	agents.ToolCalls = nativeSettingString(values, "agents.toolCalls", agents.ToolCalls)
	agents.Thinking = nativeSettingString(values, "agents.thinking", agents.Thinking)
	agents.NoFoldTurns = !nativeSettingBool(values, "agents.foldTurns", !agents.NoFoldTurns)
	agents.NoSmoothText = !nativeSettingBool(values, "agents.smoothText", !agents.NoSmoothText)
	agents.ExpandToolOutput = nativeSettingBool(values, "agents.expandToolOutput", agents.ExpandToolOutput)
	agents.NoContextUsage = !nativeSettingBool(values, "agents.showContextUsage", !agents.NoContextUsage)
	agents.NoWorkTime = !nativeSettingBool(values, "agents.showWorkTime", !agents.NoWorkTime)
	agents.NoUsageLimits = !nativeSettingBool(values, "agents.showUsageLimits", !agents.NoUsageLimits)
	agents.ShowCost = nativeSettingBool(values, "agents.showCost", agents.ShowCost)
	agents.BackendDefaults = nativeSettingJSONText(values, "agents.defaults")
	agents.BackendMCPServers = nativeSettingJSONText(values, "agents.mcpServers")
	agents = agents.normalized()
	m.prefs.Agents = &agents

	security := m.agentSecurityPrefs()
	if _, ok := values["agentSecurity.allowedCommands"]; ok {
		security.AllowedCommands = nativeSettingStringList(values, "agentSecurity.allowedCommands")
	}
	if _, ok := values["agentSecurity.blockedCommands"]; ok {
		security.BlockedCommands = nativeSettingStringList(values, "agentSecurity.blockedCommands")
	}
	security = security.normalized()
	m.prefs.AgentSecurity = &security

	if theme := strings.TrimSpace(nativeSettingString(values, "appearance.theme", "")); theme != "" {
		m.prefs.Theme = theme
	}
	m.prefs.Accent = nativeSettingString(values, "appearance.accent", m.prefs.Accent)
	m.prefs.Appearance.IDEZoom = int(nativeSettingNumber(
		values, "appearance.zoom", float64(m.ideZoomPercent()),
	))
	m.prefs.Appearance.UseCustomFont = nativeSettingBool(
		values, "appearance.useCustomFont", m.prefs.Appearance.UseCustomFont,
	)
	m.prefs.Appearance.UIFontFamily = nativeSettingString(
		values, "appearance.uiFontFamily", m.prefs.Appearance.UIFontFamily,
	)
	m.prefs.Appearance.UIFontFallback = nativeSettingString(
		values, "appearance.uiFontFallback", m.prefs.Appearance.UIFontFallback,
	)
	m.prefs.Appearance.UIFontSize = int(nativeSettingNumber(
		values, "appearance.uiFontSize", float64(m.prefs.uiFontSize()),
	))
	m.prefs.Appearance.DifferentToolWindowBackground = nativeSettingBool(
		values, "appearance.differentToolWindowBackground",
		m.prefs.Appearance.DifferentToolWindowBackground,
	)
	m.prefs.Appearance.SmallTreeIndents = nativeSettingBool(
		values, "appearance.smallTreeIndents", m.prefs.Appearance.SmallTreeIndents,
	)
	m.prefs.Appearance.FullPathInTitle = nativeSettingBool(
		values, "appearance.fullPathInTitle", m.prefs.Appearance.FullPathInTitle,
	)
	m.prefs.Appearance.ShowToolWindowBadges = nativeOptionalBool(nativeSettingBool(
		values, "appearance.showToolWindowBadges", m.prefs.toolWindowBadgesVisible(),
	))
	m.prefs.Appearance.HideMainToolbar = !nativeSettingBool(
		values, "appearance.showMainToolbar", !m.prefs.Appearance.HideMainToolbar,
	)
	m.prefs.Appearance.HideToolWindowBars = !nativeSettingBool(
		values, "appearance.showToolWindowBars", !m.prefs.Appearance.HideToolWindowBars,
	)
	m.prefs.Appearance.ShowToolWindowNames = nativeSettingBool(
		values, "appearance.showToolWindowNames", m.prefs.Appearance.ShowToolWindowNames,
	)
	m.prefs.Appearance.WidescreenToolWindows = nativeSettingBool(
		values, "appearance.widescreenToolWindows", m.prefs.Appearance.WidescreenToolWindows,
	)
	m.prefs.Appearance.HideStatusBar = !nativeSettingBool(
		values, "appearance.showStatusBar", !m.prefs.Appearance.HideStatusBar,
	)
	m.prefs.Appearance.NavigationBar = nativeSettingString(
		values, "appearance.navigationBar", m.prefs.Appearance.NavigationBar,
	)
	if widgets, ok := values["appearance.statusBarWidgets"].(map[string]any); ok {
		m.prefs.Appearance.StatusBarWidgets = map[string]bool{}
		for id, value := range widgets {
			if visible, ok := value.(bool); ok {
				m.prefs.Appearance.StatusBarWidgets[id] = visible
			}
		}
	}

	fontFamily := nativeSettingString(values, "editor.fontFamily", m.prefs.Font.Family)
	if fontFamily == oiFontMono {
		fontFamily = ""
	}
	m.prefs.Font.Family = fontFamily
	m.prefs.FontSize = float32(nativeSettingNumber(
		values, "editor.fontSize", float64(m.prefs.fontSize()),
	))
	m.prefs.Font.LineHeight = float32(nativeSettingNumber(
		values, "editor.lineHeight", float64(m.prefs.Font.LineHeight),
	))
	m.prefs.Font.Ligatures = nativeSettingBool(values, "editor.ligatures", m.prefs.Font.Ligatures)
	m.prefs.Font.Weight = int(nativeSettingNumber(
		values, "editor.fontWeight", float64(m.prefs.Font.Weight),
	))
	m.prefs.Font.Fallback = nativeSettingString(
		values, "editor.fallbackFont", m.prefs.Font.Fallback,
	)

	m.prefs.Editor.SoftWrap = nativeSettingBool(values, "editor.softWrap", m.prefs.Editor.SoftWrap)
	m.prefs.Editor.ShowWhitespace = nativeSettingBool(
		values, "editor.showWhitespace", m.prefs.Editor.ShowWhitespace,
	)
	m.prefs.Editor.ScrollPastEnd = nativeSettingBool(
		values, "editor.general.scrollPastEnd", m.prefs.Editor.ScrollPastEnd,
	)
	m.prefs.Editor.SoftWrapFiles = strings.Join(
		nativeSettingStringList(values, "editor.general.softWrapFiles"), ";",
	)
	m.prefs.General.WheelZoom = nativeSettingBool(
		values, "editor.general.wheelZoom", m.prefs.General.WheelZoom,
	)
	m.prefs.General.NoDragAndDrop = !nativeSettingBool(
		values, "editor.general.dragAndDrop", m.prefs.General.dragAndDrop(),
	)
	m.prefs.General.CaretScroll = nativeSettingString(
		values, "editor.general.caretScroll", m.prefs.General.CaretScroll,
	)
	m.prefs.General.NoSoftWrapIndent = !nativeSettingBool(
		values, "editor.general.softWrapIndent", m.prefs.General.softWrapIndent(),
	)
	m.prefs.General.ReformatOnSave = nativeSettingBool(
		values, "editor.formatOnSave", m.prefs.General.ReformatOnSave,
	)
	m.prefs.General.MarkdownImageFolder = nativeSettingString(
		values, "editor.general.markdownImageFolder", m.prefs.General.MarkdownImageFolder,
	)
	m.prefs.General.MarkdownImageNamePattern = nativeSettingString(
		values, "editor.general.markdownImageNamePattern", m.prefs.General.MarkdownImageNamePattern,
	)
	m.prefs.General.PowerSaveMode = nativeSettingBool(
		values, "general.powerSaveMode", m.prefs.General.PowerSaveMode,
	)
	if _, ok := values["general.autoCheckUpdates"]; ok {
		updates := UpdatePrefs{}
		if m.prefs.Updates != nil {
			updates = *m.prefs.Updates
		}
		updates.NoAutoCheck = !nativeSettingBool(
			values, "general.autoCheckUpdates", !updates.NoAutoCheck,
		)
		updates.NoWhatsNew = !nativeSettingBool(
			values, "general.showWhatsNewAfterUpdate", !updates.NoWhatsNew,
		)
		updates.Source = nativeSettingString(values, "general.updateSource", updates.Source)
		m.prefs.Updates = &updates
	}

	m.prefs.Breadcrumbs.Hidden = !nativeSettingBool(
		values, "editor.breadcrumbs.show", !m.prefs.Breadcrumbs.Hidden,
	)
	m.prefs.Breadcrumbs.Placement = nativeSettingString(
		values, "editor.breadcrumbs.placement", m.prefs.Breadcrumbs.Placement,
	)
	m.prefs.Breadcrumbs.DisabledLanguages = nativeSettingStringList(
		values, "editor.breadcrumbs.disabledLanguages",
	)

	m.prefs.SaveFiles.TrimTrailing = nativeSettingBool(
		values, "files.trimTrailingWhitespace", m.prefs.SaveFiles.TrimTrailing,
	)
	m.prefs.SaveFiles.TrimAllLines =
		nativeSettingString(values, "editor.general.stripTrailingScope", "modified") == "all"
	m.prefs.SaveFiles.KeepTrailingOnCaretLine = nativeSettingBool(
		values, "editor.general.keepTrailingOnCaretLine",
		m.prefs.General.keepTrailingOnCaretLine(),
	)
	m.prefs.SaveFiles.DropTrailingBlankLines = nativeSettingBool(
		values, "editor.general.removeTrailingBlankLines",
		m.prefs.SaveFiles.DropTrailingBlankLines,
	)
	m.prefs.SaveFiles.EnsureFinalNewline = nativeSettingBool(
		values, "files.ensureFinalNewline", m.prefs.SaveFiles.EnsureFinalNewline,
	)
	autoSave := nativeSettingBool(values, "files.autoSave", m.prefs.System.autoSave())
	syncOnFocus := nativeSettingBool(values, "files.syncOnFocus", m.prefs.System.syncOnFocus())
	saveOnIdle := nativeSettingBool(values, "general.saveOnIdle", m.prefs.System.SaveOnIdle)
	idleSeconds := int(nativeSettingNumber(
		values, "general.idleSeconds", float64(m.prefs.System.idleSeconds()),
	))
	m.prefs.System.AutoSave = &autoSave
	m.prefs.System.SyncOnFocus = &syncOnFocus
	m.prefs.System.SaveOnIdle = saveOnIdle
	m.prefs.System.IdleSeconds = idleSeconds
	m.prefs.System.CloseWithoutWelcome = nativeSettingBool(
		values, "general.closeWithoutWelcome", m.prefs.System.CloseWithoutWelcome,
	)
	m.prefs.System.CloseTabsWithWindow = nativeSettingBool(
		values, "general.closeTabsWithWindow", m.prefs.System.CloseTabsWithWindow,
	)
	m.prefs.System.DefaultProjectDirectory = nativeSettingString(
		values, "general.defaultProjectDirectory", m.prefs.System.DefaultProjectDirectory,
	)
	m.prefs.System.OpenProjectIn = nativeSettingString(
		values, "general.openProjectIn", m.prefs.System.openProjectIn(),
	)

	switch theme := strings.TrimSpace(m.prefs.Theme); theme {
	case "light", "dark", "system", "":
	default:
		if scheme, ok := m.prefs.EditorScheme.CustomSchemes[theme]; ok &&
			strings.EqualFold(scheme.Parent, "dark") {
			m.prefs.Theme = "dark"
		} else {
			m.prefs.Theme = "light"
		}
	}
	m.prefs.Tabs = nativeTabPrefs(values, m.prefs.Tabs)
	m.prefs.Inlay = nativeInlayPrefs(values, m.prefs.Inlay)

	confirmExit := nativeSettingBool(
		values, "general.confirmExit", m.prefs.System.confirmExit(),
	)
	m.prefs.System.ConfirmExit = &confirmExit
	m.backendFormatOnSave = m.prefs.General.ReformatOnSave
	m.backendFormatOnSaveKnown = true

	if reflect.DeepEqual(before, m.prefs) {
		return false
	}
	if err := m.savePreferences(); err != nil {
		m.settingsErr = err.Error()
	}
	m.applyPreferences()
	return true
}

func nativeTabPrefs(values map[string]any, fallback TabPrefs) TabPrefs {
	prefs := fallback
	prefs.Placement = nativeSettingString(values, "editor.tabs.placement", prefs.Placement)
	if prefs.Placement == "top" {
		prefs.Placement = ""
	}
	prefs.PinnedSeparateRow = nativeSettingBool(
		values, "editor.tabs.pinnedSeparateRow", prefs.PinnedSeparateRow,
	)
	prefs.HideExtension = !nativeSettingBool(
		values, "editor.tabs.showExtension", !prefs.HideExtension,
	)
	prefs.NoDirectoryForNonUnique = !nativeSettingBool(
		values, "editor.tabs.showDirectoryForNonUnique", !prefs.NoDirectoryForNonUnique,
	)
	prefs.MarkModified = nativeSettingBool(values, "editor.tabs.markModified", prefs.MarkModified)
	prefs.HideFullPathOnHover = !nativeSettingBool(
		values, "editor.tabs.showFullPathOnHover", !prefs.HideFullPathOnHover,
	)
	closePosition := nativeSettingString(values, "editor.tabs.closeButtonPosition", "")
	switch closePosition {
	case "none":
		prefs.NoCloseButton = true
		prefs.CloseButtonLeft = false
	case "left":
		prefs.NoCloseButton = false
		prefs.CloseButtonLeft = true
	case "right":
		prefs.NoCloseButton = false
		prefs.CloseButtonLeft = false
	default:
		prefs.NoCloseButton = !nativeSettingBool(
			values, "editor.tabs.showCloseButton", !prefs.NoCloseButton,
		)
	}
	prefs.SortAlphabetically = nativeSettingBool(
		values, "editor.tabs.sortAlphabetically", prefs.SortAlphabetically,
	)
	prefs.OpenAtEnd = nativeSettingBool(values, "editor.tabs.openAtEnd", prefs.OpenAtEnd)
	prefs.TabLimit = int(nativeSettingNumber(values, "editor.tabs.tabLimit", float64(prefs.limit())))
	prefs.ActivateOnClose = nativeSettingString(
		values, "editor.tabs.activateOnClose", prefs.activation(),
	)
	if prefs.ActivateOnClose == "left" {
		prefs.ActivateOnClose = ""
	}
	prefs.ClosingPolicy = nativeSettingString(
		values, "editor.tabs.closingPolicy", prefs.closingPolicy(),
	)
	if prefs.ClosingPolicy == "lru" {
		prefs.ClosingPolicy = ""
	}
	prefs.NoPreviewTab = !nativeSettingBool(
		values, "editor.tabs.previewTab", !prefs.NoPreviewTab,
	)
	prefs.MultipleRows = nativeSettingBool(values, "editor.tabs.multipleRows", prefs.MultipleRows)
	prefs.HideFileIcon = !nativeSettingBool(
		values, "editor.tabs.showFileIcon", !prefs.HideFileIcon,
	)
	return prefs
}

func nativeInlayPrefs(values map[string]any, fallback InlayPrefs) InlayPrefs {
	prefs := fallback
	prefs.NoCodeVisionUsages = !nativeSettingBool(
		values, "editor.inlay.codeVisionUsages", !prefs.NoCodeVisionUsages,
	)
	prefs.NoCodeVisionAuthor = !nativeSettingBool(
		values, "editor.inlay.codeVisionAuthor", !prefs.NoCodeVisionAuthor,
	)
	prefs.HideCodeVisionNoUses = !nativeSettingBool(
		values, "editor.inlay.codeVisionNoUsages", !prefs.HideCodeVisionNoUses,
	)
	prefs.UsagesPosition = nativeSettingString(
		values, "editor.inlay.codeVisionUsagesPosition", prefs.UsagesPosition,
	)
	prefs.AuthorPosition = nativeSettingString(
		values, "editor.inlay.codeVisionAuthorPosition", prefs.AuthorPosition,
	)
	return prefs
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
