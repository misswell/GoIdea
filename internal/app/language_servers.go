package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	languageToolFilterAll       = "all"
	languageToolFilterMissing   = "missing"
	languageToolFilterAvailable = "available"
)

// languageServerConfig is one languageServers.<name> entry. Pointer and
// nil-backed fields keep an absent setting distinct from an explicit false,
// empty list or empty object so Apply can remove only what was removed.
type languageServerConfig struct {
	Enabled               *bool             `json:"enabled,omitempty"`
	Command               *string           `json:"command,omitempty"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	InitializationOptions json.RawMessage   `json:"initializationOptions,omitempty"`
	Settings              json.RawMessage   `json:"settings,omitempty"`
	Languages             []string          `json:"languages,omitempty"`
	RootMarkers           []string          `json:"rootMarkers,omitempty"`
}

type languageToolEditorState struct {
	Path                  string
	Command               string
	Args                  string
	Env                   string
	InitializationOptions string
	Settings              string
	Languages             string
	RootMarkers           string
	Errors                map[string]string
}

type languageServerScopeState struct {
	loaded    bool
	applied   map[string]languageServerConfig
	draft     map[string]languageServerConfig
	pathBase  map[string]string
	pathDraft map[string]string
	toolsRaw  map[string]json.RawMessage
	editors   map[string]*languageToolEditorState
}

type languageToolSettingsState struct {
	loading        bool
	loaded         bool
	saving         bool
	scope          string
	scopeChosen    bool
	session        bool
	loadGen        uint64
	err            string
	applied        map[string]languageServerConfig
	draft          map[string]languageServerConfig
	pathBase       map[string]string
	pathDraft      map[string]string
	toolsRaw       map[string]json.RawMessage
	scopes         map[string]*languageServerScopeState
	languages      []openidea.LanguageInfo
	editors        map[string]*languageToolEditorState
	addOpen        bool
	addCommand     string
	addName        string
	addLanguages   string
	addErr         string
	logOpen        bool
	closeAfterSave bool
}

type languageSettingWrite struct {
	key   string
	value any
}

type languageServerScopeSave struct {
	scope  string
	writes []languageSettingWrite
}

func languageToolKindLabel(kind string) string {
	switch kind {
	case "languageServer":
		return "Language server"
	case "formatter":
		return "Formatter"
	case "debugger":
		return "Debugger"
	case "plugin":
		return "Plugin"
	case "cli":
		return "Command-line tool"
	}
	return "Tool"
}

func isCustomLanguageServer(entry languageServerConfig) bool {
	return len(entry.Languages) > 0
}

// languageToolView is the backend's tool list after the dialog's staged
// additions and removals, matching OpenIDEA's `all` list.
func (m *Model) languageToolView() []javaTool {
	settings := &m.languageToolSettings
	view := make([]javaTool, 0, len(m.languageTools)+len(settings.draft))
	seen := make(map[string]bool, len(m.languageTools))
	for _, tool := range m.languageTools {
		if isCustomLanguageServer(settings.applied[tool.Name]) &&
			!isCustomLanguageServer(settings.draft[tool.Name]) {
			continue
		}
		view = append(view, tool)
		seen[tool.Name] = true
	}
	for name, entry := range settings.draft {
		if !isCustomLanguageServer(entry) || seen[name] {
			continue
		}
		command := ""
		if entry.Command != nil {
			command = *entry.Command
		}
		view = append(view, javaTool{
			Name: name, Display: name, Kind: "languageServer", Command: command,
			Languages: append([]string(nil), entry.Languages...), Status: "missing",
			PendingCreate: true,
		})
	}
	return view
}

// filterLanguageTools applies OpenIDEA's query and All / Missing / Available
// segment to individual tools.
func filterLanguageTools(tools []javaTool, query, filter string) []javaTool {
	q := strings.ToLower(strings.TrimSpace(query))
	ready := make([]javaTool, 0, len(tools))
	missing := make([]javaTool, 0, len(tools))
	for _, tool := range tools {
		if filter == languageToolFilterMissing && tool.Status != "missing" {
			continue
		}
		if filter == languageToolFilterAvailable && tool.Status == "missing" {
			continue
		}
		if q != "" {
			fields := []string{tool.Name, tool.Display, tool.Command}
			fields = append(fields, tool.Languages...)
			match := false
			for _, field := range fields {
				if strings.Contains(strings.ToLower(field), q) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		if tool.Status == "missing" {
			missing = append(missing, tool)
		} else {
			ready = append(ready, tool)
		}
	}
	return append(ready, missing...)
}

func languageToolsMissingCount(tools []javaTool) int {
	count := 0
	for _, tool := range tools {
		if tool.Status == "missing" {
			count++
		}
	}
	return count
}

// ensureLanguageToolSelection points the page at a tool still visible after a
// reload or filter change.
func (m *Model) ensureLanguageToolSelection() {
	shown := filterLanguageTools(m.languageToolView(), m.languageToolFind, m.normalizedLanguageToolFilter())
	if len(shown) == 0 {
		m.languageToolPick = ""
		return
	}
	for _, tool := range shown {
		if tool.Name == m.languageToolPick {
			return
		}
	}
	m.languageToolPick = shown[0].Name
}

func (m *Model) normalizedLanguageToolFilter() string {
	switch m.languageToolFilter {
	case languageToolFilterMissing, languageToolFilterAvailable:
		return m.languageToolFilter
	default:
		return languageToolFilterAll
	}
}

func (m *Model) languageToolBackendAvailable() bool {
	return m.ctx != nil && (m.mgr != nil || m.invokeHook != nil)
}

func (m *Model) languageToolList(ctx context.Context) ([]openidea.ToolInfo, error) {
	raw, err := m.invoke(ctx, "tools_list", nil)
	if err != nil {
		return nil, err
	}
	var list []openidea.ToolInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (m *Model) installLanguageToolBackend(
	ctx context.Context,
	name string,
	request, channel uint32,
) (openidea.ToolInfo, error) {
	raw, err := m.invoke(ctx, "tools_install", map[string]any{
		"name": name, "request": request,
		"onEvent": map[string]any{"$channel": channel},
	})
	if err != nil {
		return openidea.ToolInfo{}, err
	}
	var info openidea.ToolInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return openidea.ToolInfo{}, err
	}
	return info, nil
}

func (m *Model) uninstallLanguageToolBackend(
	ctx context.Context,
	name string,
) (openidea.ToolInfo, error) {
	raw, err := m.invoke(ctx, "tools_uninstall", map[string]any{"name": name})
	if err != nil {
		return openidea.ToolInfo{}, err
	}
	var info openidea.ToolInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return openidea.ToolInfo{}, err
	}
	return info, nil
}

// InstallLanguageTool installs a missing tool, or removes one the backend
// manages, using the same streamed progress and cancellation as the Java page.
func (m *Model) InstallLanguageTool(name string, uninstall bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		m.OpenSettingsPage(settingsPageLanguageServers)
		return
	}
	if m.toolsBusy != "" {
		return
	}
	if m.Root() == "" || !m.languageToolBackendAvailable() {
		m.OpenSettingsPage(settingsPageLanguageServers)
		m.languageToolPick = name
		m.status = "Open a project before installing language tools"
		m.invalidate()
		return
	}
	m.toolsErr = ""
	m.toolsLog = nil
	m.toolsProgress = ""
	m.toolsAbort.Store(false)
	m.languageToolPick = name
	m.toolsBusy = "Reading the tool status…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancel()
		list, err := m.languageToolList(ctx)
		if err != nil {
			m.finishLanguageToolInstall("Could not read the tool list: "+err.Error(), "")
			return
		}
		rows := languageToolRows(list)
		var tool javaTool
		found := false
		for _, row := range rows {
			if row.Name == name {
				tool, found = row, true
				break
			}
		}
		if !found {
			m.finishLanguageToolInstall("The backend does not know the tool "+name+".", "")
			return
		}
		if uninstall {
			m.post(func() { m.toolsBusy = "Removing " + tool.Display })
			info, err := m.uninstallLanguageToolBackend(ctx, name)
			if err != nil {
				m.finishLanguageToolInstall("Removing "+tool.Display+" failed: "+err.Error(), "")
				return
			}
			m.post(func() {
				m.toolsLog = append(m.toolsLog, "Removed "+tool.Display)
				m.toolsProgress = info.Status
			})
		} else {
			if !tool.CanInstall {
				reason := tool.Reason
				if reason == "" {
					reason = "The backend cannot install this tool automatically."
				}
				m.finishLanguageToolInstall(reason, name)
				return
			}
			ch := m.nextChannel()
			req := m.nextRequest()
			m.post(func() {
				m.toolsChannel = ch
				m.toolsRequest = req
				m.toolsBusy = "Installing " + tool.Display
			})
			var installErr error
			var installed openidea.ToolInfo
			for attempt := 0; attempt < 2; attempt++ {
				installed, installErr = m.installLanguageToolBackend(ctx, name, req, ch)
				if installErr == nil || m.toolsAbort.Load() || ctx.Err() != nil {
					break
				}
				select {
				case <-time.After(2 * time.Second):
				case <-ctx.Done():
				}
			}
			if installErr != nil {
				if m.toolsAbort.Load() {
					m.finishLanguageToolInstall("The tool installation was cancelled", name)
				} else {
					m.finishLanguageToolInstall(tool.Display+" installation failed: "+installErr.Error(), name)
				}
				return
			}
			m.post(func() {
				message := "Installed " + tool.Display
				if installed.Version != nil && *installed.Version != "" {
					message += " " + *installed.Version
				}
				m.toolsLog = append(m.toolsLog, message)
			})
			if _, err := m.invoke(ctx, "lsp_restart_named", map[string]any{"name": name}); err != nil {
				m.finishLanguageToolInstall("Installed, but restarting the language server failed: "+err.Error(), name)
				return
			}
		}
		reloaded, err := m.languageToolList(ctx)
		if err != nil {
			m.finishLanguageToolInstall("The tool changed, but reading the status again failed: "+err.Error(), name)
			return
		}
		m.post(func() {
			m.languageTools = languageToolRows(reloaded)
			m.javaTools = javaToolRowsFrom(m.languageTools)
			m.toolsLoaded = true
			m.ensureLanguageToolSelection()
		})
		if !uninstall {
			for _, row := range languageToolRows(reloaded) {
				if row.Name == name && row.Status == "missing" {
					m.finishLanguageToolInstall(tool.Display+" is still not installed after the installer finished.", name)
					return
				}
			}
		}
		if m.reg != nil {
			if err := m.reg.ReopenAll(ctx); err != nil {
				m.finishLanguageToolInstall("The tool changed, but reopening files failed: "+err.Error(), name)
				return
			}
		}
		action := "installed"
		if uninstall {
			action = "removed"
		}
		m.finishLanguageToolInstall("", name, tool.Display+" "+action)
	}()
}

func (m *Model) finishLanguageToolInstall(errText, selected string, success ...string) {
	m.post(func() {
		m.toolsBusy = ""
		m.toolsChannel = 0
		m.toolsRequest = 0
		m.toolsProgress = ""
		if selected != "" {
			m.languageToolPick = selected
		}
		if errText != "" {
			m.toolsErr = errText
			m.status = errText
			m.OpenSettingsPage(settingsPageLanguageServers)
			m.invalidate()
			return
		}
		if len(success) > 0 {
			m.status = success[0]
		}
		m.ensureLanguageToolSelection()
		m.invalidate()
	})
}

// PromptInstallLanguageTool is the hidden tools.promptInstall action. Like
// OpenIDEA, it offers Install and Language Servers actions instead of starting
// an installer immediately.
func (m *Model) PromptInstallLanguageTool(name string, sticky, formatter bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if !m.toolsLoaded {
		m.loadLanguageToolsForPrompt("install:"+name, func() {
			m.PromptInstallLanguageTool(name, sticky, formatter)
		})
		return
	}
	var tool *javaTool
	for i := range m.languageTools {
		if m.languageTools[i].Name == name {
			tool = &m.languageTools[i]
			break
		}
	}
	if tool == nil || tool.Status != "missing" || !tool.CanInstall {
		return
	}
	alternatives := append([]string(nil), tool.Alternatives...)
	if len(alternatives) == 0 {
		alternatives = []string{name}
	}
	if position := slices.Index(alternatives, name); position > 0 {
		for _, alternative := range alternatives[:position] {
			if m.languageToolOffered[alternative] {
				return
			}
		}
	}
	formatter = formatter || tool.Kind == "formatter"
	if !formatter && m.languageToolServed(tool) {
		return
	}
	if m.languageToolOffered == nil {
		m.languageToolOffered = make(map[string]bool)
	}
	m.languageToolOffered[name] = true
	m.languageToolPick = name

	display := tool.Display
	if display == "" {
		display = tool.Name
	}
	languages := strings.Join(tool.Languages, ", ")
	if languages == "" {
		languages = "these files"
	}
	install := tool.Install
	if install == "" {
		install = "the configured installer"
	}
	body := fmt.Sprintf("Code insight for %s needs %s. Install it via %s?", languages, tool.Command, install)
	if formatter {
		body = fmt.Sprintf("Reformat Code needs %s. Install it via %s?", display, install)
	}
	key := "lsp-missing-" + name
	actions := []notifyAction{
		{label: "Install", run: func(m *Model) { m.InstallLanguageTool(name, false) }},
		{label: "Language Servers…", run: func(m *Model) {
			m.OpenSettingsPage(settingsPageLanguageServers)
		}},
	}
	if sticky {
		m.notifySticky(key, "warning", display+" is not installed", body, false, actions...)
	} else {
		m.notifyWarning(key, display+" is not installed", body, actions...)
	}
}

// languageToolServed reports whether another installed tool of the same kind
// already covers these files, as typescript-language-server does for vtsls.
func (m *Model) languageToolServed(tool *javaTool) bool {
	for _, other := range m.languageTools {
		if other.Name == tool.Name || other.Kind != tool.Kind || other.Status == "missing" {
			continue
		}
		for _, language := range other.Languages {
			if slices.Contains(tool.Languages, language) {
				return true
			}
		}
	}
	return false
}

func (m *Model) loadLanguageToolsForPrompt(key string, done func()) {
	if m.toolsLoaded {
		done()
		return
	}
	if m.mgr == nil || m.languageToolLoading[key] {
		return
	}
	if m.languageToolLoading == nil {
		m.languageToolLoading = make(map[string]bool)
	}
	m.languageToolLoading[key] = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		list, err := m.mgr.ToolsList(ctx, Window)
		m.post(func() {
			delete(m.languageToolLoading, key)
			if err != nil {
				return
			}
			m.languageTools = languageToolRows(list)
			m.javaTools = javaToolRowsFrom(m.languageTools)
			m.toolsLoaded = true
			m.ensureLanguageToolSelection()
			done()
		})
	}()
}

// promptMissingLanguageTool is the languageServer listener: it loads the tool
// registry once, then offers the missing server through tools.promptInstall.
func (m *Model) promptMissingLanguageTool(name string) {
	name = strings.TrimSpace(name)
	if name == "" || m.languageToolPrompted[name] {
		return
	}
	if m.languageToolPrompted == nil {
		m.languageToolPrompted = make(map[string]bool)
	}
	m.languageToolPrompted[name] = true
	m.PromptInstallLanguageTool(name, true, false)
}

// acceptLanguageServerWarning replaces the generic warning with the managed
// tool install offer, keyed by server and tool as OpenIDEA does.
func (m *Model) acceptLanguageServerWarning(e openidea.LanguageServerWarningEvent) {
	key := strings.TrimSpace(e.Name) + "\x00" + strings.TrimSpace(e.InstallTool)
	if e.Name == "" || e.InstallTool == "" || m.languageToolWarned[key] {
		return
	}
	if m.languageToolWarned == nil {
		m.languageToolWarned = make(map[string]bool)
	}
	m.languageToolWarned[key] = true
	if !m.toolsLoaded {
		m.loadLanguageToolsForPrompt("warning:"+key, func() {
			m.postLanguageServerWarning(e)
		})
		return
	}
	m.postLanguageServerWarning(e)
}

func (m *Model) postLanguageServerWarning(e openidea.LanguageServerWarningEvent) {
	actions := []notifyAction{{
		label: "Language Servers…",
		run:   func(m *Model) { m.OpenSettingsPage(settingsPageLanguageServers) },
	}}
	for _, tool := range m.languageTools {
		if tool.Name == e.InstallTool && tool.CanInstall {
			actions = append([]notifyAction{{
				label: "Install",
				run:   func(m *Model) { m.InstallLanguageTool(e.InstallTool, false) },
			}}, actions...)
			break
		}
	}
	m.notifySticky("lsp-warning-"+e.Name+"-"+e.InstallTool, "warning", e.Name, e.Message, false, actions...)
}

func cloneLanguageServerConfig(entry languageServerConfig) languageServerConfig {
	out := entry
	if entry.Enabled != nil {
		value := *entry.Enabled
		out.Enabled = &value
	}
	if entry.Command != nil {
		value := *entry.Command
		out.Command = &value
	}
	out.Args = cloneLanguageToolList(entry.Args)
	if entry.Env != nil {
		out.Env = make(map[string]string, len(entry.Env))
		for key, value := range entry.Env {
			out.Env[key] = value
		}
	}
	if entry.InitializationOptions != nil {
		out.InitializationOptions = append(json.RawMessage{}, entry.InitializationOptions...)
	}
	if entry.Settings != nil {
		out.Settings = append(json.RawMessage{}, entry.Settings...)
	}
	out.Languages = cloneLanguageToolList(entry.Languages)
	out.RootMarkers = cloneLanguageToolList(entry.RootMarkers)
	return out
}

func cloneLanguageToolList(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func cloneLanguageServerMap(in map[string]languageServerConfig) map[string]languageServerConfig {
	out := make(map[string]languageServerConfig, len(in))
	for name, entry := range in {
		out[name] = cloneLanguageServerConfig(entry)
	}
	return out
}

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

// loadLanguageServerSettings starts one Settings-dialog session. Navigating
// away and back keeps each scope's staged edits, while opening Settings again
// reads fresh values from the backend.
func (m *Model) loadLanguageServerSettings() {
	state := &m.languageToolSettings
	if state.session && (state.loading || state.loaded) {
		return
	}
	state.session = true
	if m.settingsDraft.newProjects {
		state.scope = "global"
		state.scopeChosen = true
	} else if !state.scopeChosen || (state.scope == "project" && m.Root() == "") {
		state.scope = "global"
		if m.Root() != "" {
			state.scope = "project"
		}
		state.scopeChosen = true
	}
	m.loadLanguageServerScope(state.scope)
}

// SetLanguageServerScope switches between the global layer and this
// project's overrides. Staged values are retained independently per scope.
func (m *Model) SetLanguageServerScope(scope string) {
	state := &m.languageToolSettings
	if scope != "global" && scope != "project" {
		return
	}
	if m.settingsDraft.newProjects || scope == "project" && m.Root() == "" {
		scope = "global"
	}
	if scope == state.scope || state.loading || state.saving {
		return
	}
	state.scopeChosen = true
	m.captureLanguageServerScope()
	state.scope = scope
	if cached := state.scopes[scope]; cached != nil && cached.loaded {
		m.restoreLanguageServerScope(cached)
		state.loading = false
		state.err = ""
		m.ensureLanguageToolSelection()
		m.invalidate()
		return
	}
	m.loadLanguageServerScope(scope)
}

func (m *Model) loadLanguageServerScope(scope string) {
	state := &m.languageToolSettings
	state.scope = scope
	state.loading = true
	state.loaded = false
	state.err = ""
	state.loadGen++
	generation := state.loadGen
	clearLanguageServerScope(state)
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.loaded = true
		state.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	root := m.Root()
	command := "settings_get"
	if scope == "global" {
		command = "settings_get_global"
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, command, nil)
		if err != nil {
			m.post(func() {
				if state.loadGen != generation || state.scope != scope {
					return
				}
				state.loading = false
				state.loaded = true
				state.err = "Could not read language server settings: " + err.Error()
				m.invalidate()
			})
			return
		}
		var settings struct {
			LanguageServers map[string]languageServerConfig `json:"languageServers"`
			Tools           map[string]json.RawMessage      `json:"tools"`
		}
		if err := json.Unmarshal(raw, &settings); err != nil {
			m.post(func() {
				if state.loadGen != generation || state.scope != scope {
					return
				}
				state.loading = false
				state.loaded = true
				state.err = "Could not decode language server settings: " + err.Error()
				m.invalidate()
			})
			return
		}
		paths := make(map[string]string)
		for toolCommand, rawTool := range settings.Tools {
			var tool struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(rawTool, &tool) == nil && tool.Path != "" {
				paths[toolCommand] = tool.Path
			}
		}
		languages := []openidea.LanguageInfo(nil)
		if rawLanguages, languageErr := m.invoke(ctx, "languages", nil); languageErr == nil {
			_ = json.Unmarshal(rawLanguages, &languages)
		}
		cached := &languageServerScopeState{
			loaded:   true,
			applied:  cloneLanguageServerMap(settings.LanguageServers),
			draft:    cloneLanguageServerMap(settings.LanguageServers),
			pathBase: cloneStringMap(paths), pathDraft: cloneStringMap(paths),
			toolsRaw: cloneRawMap(settings.Tools),
			editors:  map[string]*languageToolEditorState{},
		}
		for name, entry := range cached.draft {
			cached.editors[name] = languageToolEditorFrom(entry)
		}
		m.post(func() {
			if state.loadGen != generation || state.scope != scope ||
				(scope == "project" && m.Root() != root) {
				return
			}
			if state.scopes == nil {
				state.scopes = map[string]*languageServerScopeState{}
			}
			state.scopes[scope] = cached
			m.restoreLanguageServerScope(cached)
			state.loading = false
			state.loaded = true
			state.languages = languages
			m.ensureLanguageToolSelection()
			m.invalidate()
		})
	}()
}

func clearLanguageServerScope(state *languageToolSettingsState) {
	state.applied = map[string]languageServerConfig{}
	state.draft = map[string]languageServerConfig{}
	state.pathBase = map[string]string{}
	state.pathDraft = map[string]string{}
	state.toolsRaw = map[string]json.RawMessage{}
	state.editors = map[string]*languageToolEditorState{}
}

func (m *Model) captureLanguageServerScope() {
	state := &m.languageToolSettings
	if state.scope == "" || !state.loaded {
		return
	}
	if state.scopes == nil {
		state.scopes = map[string]*languageServerScopeState{}
	}
	state.scopes[state.scope] = &languageServerScopeState{
		loaded:  true,
		applied: state.applied, draft: state.draft,
		pathBase: state.pathBase, pathDraft: state.pathDraft,
		toolsRaw: state.toolsRaw, editors: state.editors,
	}
}

func (m *Model) restoreLanguageServerScope(cached *languageServerScopeState) {
	state := &m.languageToolSettings
	if cached == nil || !cached.loaded {
		clearLanguageServerScope(state)
		state.loaded = false
		return
	}
	state.applied = cached.applied
	state.draft = cached.draft
	state.pathBase = cached.pathBase
	state.pathDraft = cached.pathDraft
	state.toolsRaw = cached.toolsRaw
	state.editors = cached.editors
	state.loaded = true
	if state.editors == nil {
		state.editors = map[string]*languageToolEditorState{}
	}
}

func languageToolEditorFrom(entry languageServerConfig) *languageToolEditorState {
	command := ""
	if entry.Command != nil {
		command = *entry.Command
	}
	return &languageToolEditorState{
		Command: command, Args: formatLanguageToolArgs(entry.Args),
		Env:                   formatLanguageToolEnv(entry.Env),
		InitializationOptions: formatLanguageToolJSON(entry.InitializationOptions),
		Settings:              formatLanguageToolJSON(entry.Settings),
		Languages:             formatLanguageToolList(entry.Languages),
		RootMarkers:           formatLanguageToolList(entry.RootMarkers),
		Errors:                map[string]string{},
	}
}

func (m *Model) languageToolEditor(name string) *languageToolEditorState {
	state := &m.languageToolSettings
	if state.editors == nil {
		state.editors = make(map[string]*languageToolEditorState)
	}
	editor := state.editors[name]
	if editor == nil {
		editor = languageToolEditorFrom(state.draft[name])
		for _, tool := range m.languageTools {
			if tool.Name == name {
				editor.Path = state.pathDraft[tool.Command]
				break
			}
		}
		state.editors[name] = editor
	}
	if editor.Errors == nil {
		editor.Errors = map[string]string{}
	}
	return editor
}

func (m *Model) resetLanguageServerDrafts() {
	state := &m.languageToolSettings
	state.addOpen = false
	state.addErr = ""
	state.draft = cloneLanguageServerMap(state.applied)
	state.pathDraft = cloneStringMap(state.pathBase)
	state.editors = make(map[string]*languageToolEditorState)
	for name, entry := range state.draft {
		state.editors[name] = languageToolEditorFrom(entry)
	}
	state.err = ""
	m.ensureLanguageToolSelection()
	m.invalidate()
}

// resetLanguageServerSettingsSession discards every scope and invalidates any
// in-flight reads when Settings closes.
func (m *Model) resetLanguageServerSettingsSession() {
	state := &m.languageToolSettings
	state.loadGen++
	state.session = false
	state.scopeChosen = false
	state.scope = ""
	state.loading = false
	state.loaded = false
	state.saving = false
	state.err = ""
	state.closeAfterSave = false
	state.scopes = nil
	clearLanguageServerScope(state)
	m.resetLanguageServerDrafts()
}

func boolRef(value bool) *bool { return &value }

func stringRef(value string) *string { return &value }

func languageServerConfigEmpty(entry languageServerConfig) bool {
	return entry.Enabled == nil && entry.Command == nil && entry.Args == nil &&
		entry.Env == nil && entry.InitializationOptions == nil &&
		entry.Settings == nil && entry.Languages == nil && entry.RootMarkers == nil
}

func (m *Model) stageLanguageServer(name string, change func(*languageServerConfig)) {
	state := &m.languageToolSettings
	if state.draft == nil {
		return
	}
	entry := state.draft[name]
	change(&entry)
	if languageServerConfigEmpty(entry) {
		delete(state.draft, name)
	} else {
		state.draft[name] = entry
	}
	m.invalidate()
}

func (m *Model) stageLanguageToolEnabled(name string, on bool) {
	m.stageLanguageServer(name, func(entry *languageServerConfig) {
		if on {
			if m.languageToolSettings.scope == "project" {
				entry.Enabled = boolRef(true)
			} else {
				entry.Enabled = nil
			}
			return
		}
		entry.Enabled = boolRef(false)
	})
}

func (m *Model) stageLanguageToolCommand(name, text string) {
	editor := m.languageToolEditor(name)
	editor.Command = text
	m.stageLanguageServer(name, func(entry *languageServerConfig) {
		if strings.TrimSpace(text) == "" {
			entry.Command = nil
		} else {
			entry.Command = stringRef(strings.TrimSpace(text))
		}
	})
}

func (m *Model) stageLanguageToolPath(name, command, text string) {
	state := &m.languageToolSettings
	m.languageToolEditor(name).Path = text
	if state.pathDraft == nil {
		state.pathDraft = map[string]string{}
	}
	if strings.TrimSpace(text) == "" {
		delete(state.pathDraft, command)
	} else {
		state.pathDraft[command] = strings.TrimSpace(text)
	}
	m.invalidate()
}

func (m *Model) stageLanguageToolArgs(name, text string) {
	editor := m.languageToolEditor(name)
	editor.Args = text
	args, err := parseLanguageToolArgs(text)
	if err != nil {
		editor.Errors["args"] = err.Error()
		m.invalidate()
		return
	}
	delete(editor.Errors, "args")
	m.stageLanguageServer(name, func(entry *languageServerConfig) { entry.Args = args })
}

func (m *Model) stageLanguageToolEnv(name, text string) {
	editor := m.languageToolEditor(name)
	editor.Env = text
	env, err := parseLanguageToolEnv(text)
	if err != nil {
		editor.Errors["env"] = err.Error()
		m.invalidate()
		return
	}
	delete(editor.Errors, "env")
	m.stageLanguageServer(name, func(entry *languageServerConfig) { entry.Env = env })
}

func (m *Model) stageLanguageToolJSON(name, field, text string) {
	editor := m.languageToolEditor(name)
	if field == "settings" {
		editor.Settings = text
	} else {
		editor.InitializationOptions = text
	}
	value, err := parseLanguageToolJSONObject(text)
	if err != nil {
		editor.Errors[field] = err.Error()
		m.invalidate()
		return
	}
	delete(editor.Errors, field)
	m.stageLanguageServer(name, func(entry *languageServerConfig) {
		if field == "settings" {
			entry.Settings = value
		} else {
			entry.InitializationOptions = value
		}
	})
}

func (m *Model) stageLanguageToolList(name, field, text string) {
	editor := m.languageToolEditor(name)
	if field == "languages" {
		editor.Languages = text
	} else {
		editor.RootMarkers = text
	}
	values := parseLanguageToolList(text)
	delete(editor.Errors, field)
	m.stageLanguageServer(name, func(entry *languageServerConfig) {
		if field == "languages" {
			entry.Languages = values
		} else {
			entry.RootMarkers = values
		}
	})
}

func languageServerScopeDirty(
	applied, draft map[string]languageServerConfig,
	pathBase, pathDraft map[string]string,
) bool {
	if len(applied) != len(draft) || len(pathBase) != len(pathDraft) {
		return true
	}
	for name, before := range applied {
		after, ok := draft[name]
		if !ok || !sameLanguageServerConfig(before, after) {
			return true
		}
	}
	for command, path := range pathBase {
		if pathDraft[command] != path {
			return true
		}
	}
	return false
}

func languageToolSettingsDirty(state *languageToolSettingsState) bool {
	if state.loaded && languageServerScopeDirty(
		state.applied, state.draft, state.pathBase, state.pathDraft,
	) {
		return true
	}
	for scope, cached := range state.scopes {
		if scope == state.scope || cached == nil || !cached.loaded {
			continue
		}
		if languageServerScopeDirty(
			cached.applied, cached.draft, cached.pathBase, cached.pathDraft,
		) {
			return true
		}
	}
	return false
}

func sameLanguageServerConfig(a, b languageServerConfig) bool {
	return sameBoolPointer(a.Enabled, b.Enabled) && sameStringPointer(a.Command, b.Command) &&
		sameStringSlice(a.Args, b.Args) && sameStringMap(a.Env, b.Env) &&
		sameJSON(a.InitializationOptions, b.InitializationOptions) &&
		sameJSON(a.Settings, b.Settings) && sameStringSlice(a.Languages, b.Languages) &&
		sameStringSlice(a.RootMarkers, b.RootMarkers)
}

func sameBoolPointer(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameStringPointer(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameStringSlice(a, b []string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return slices.Equal(a, b)
}

func sameStringMap(a, b map[string]string) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			return false
		}
	}
	return true
}

func sameJSON(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == 0 && len(b) == 0
	}
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return string(a) == string(b)
	}
	ac, _ := json.Marshal(av)
	bc, _ := json.Marshal(bv)
	return string(ac) == string(bc)
}

func (m *Model) removeLanguageServer(name string) {
	entry := m.languageToolSettings.draft[name]
	if !isCustomLanguageServer(entry) {
		return
	}
	delete(m.languageToolSettings.draft, name)
	delete(m.languageToolSettings.editors, name)
	m.ensureLanguageToolSelection()
	m.invalidate()
}

func (m *Model) restoreLanguageServerDefaults(name string) {
	entry := m.languageToolSettings.draft[name]
	command := entry.Command
	if command == nil {
		for _, tool := range m.languageTools {
			if tool.Name == name {
				if tool.Command != "" {
					delete(m.languageToolSettings.pathDraft, tool.Command)
				}
				break
			}
		}
	}
	delete(m.languageToolSettings.draft, name)
	delete(m.languageToolSettings.editors, name)
	m.invalidate()
}

func (m *Model) openLanguageServerAddDialog() {
	state := &m.languageToolSettings
	state.addOpen = true
	state.addCommand = ""
	state.addName = ""
	state.addLanguages = ""
	state.addErr = ""
	m.invalidate()
}

func (m *Model) closeLanguageServerAddDialog() {
	m.languageToolSettings.addOpen = false
	m.languageToolSettings.addErr = ""
	m.invalidate()
}

func (m *Model) addLanguageServer() {
	state := &m.languageToolSettings
	args, err := parseLanguageToolArgs(state.addCommand)
	if err != nil {
		state.addErr = err.Error()
		m.invalidate()
		return
	}
	if len(args) == 0 {
		state.addErr = "Enter the command that starts the server."
		m.invalidate()
		return
	}
	command := strings.TrimSpace(args[0])
	name := strings.TrimSpace(state.addName)
	if name == "" {
		name = languageServerNameFromCommand(command)
	}
	if !validLanguageServerName(name) {
		state.addErr = "Name: letters, digits, - _ + only."
		m.invalidate()
		return
	}
	if _, exists := state.draft[name]; exists {
		state.addErr = name + " already exists."
		m.invalidate()
		return
	}
	languages := parseLanguageToolList(state.addLanguages)
	if len(languages) == 0 {
		state.addErr = "Choose the languages it serves."
		m.invalidate()
		return
	}
	entry := languageServerConfig{Command: stringRef(command), Languages: languages}
	if len(args) > 1 {
		entry.Args = append([]string(nil), args[1:]...)
	}
	state.draft[name] = entry
	if state.editors == nil {
		state.editors = make(map[string]*languageToolEditorState)
	}
	state.editors[name] = languageToolEditorFrom(entry)
	state.addOpen = false
	state.addErr = ""
	m.languageToolPick = name
	m.languageToolFilter = languageToolFilterAll
	m.languageToolFind = ""
	m.invalidate()
}

func languageServerNameFromCommand(command string) string {
	base := filepath.Base(strings.TrimSpace(command))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var out strings.Builder
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '+' || r == '-' {
			out.WriteRune(r)
		} else {
			out.WriteByte('-')
		}
	}
	return out.String()
}

func validLanguageServerName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '+' || r == '-') {
			return false
		}
	}
	return true
}

// BrowseLanguageToolExecutable opens the native file picker for a built-in
// tool's path override, a custom server's command, or the Add dialog.
func (m *Model) BrowseLanguageToolExecutable(name string, newServer bool) {
	parent := m.win
	title := "Select the server executable"
	if name != "" {
		title = "Select " + name
	}
	start := m.Root()
	if start == "" {
		start = "."
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: title, ButtonLabel: "Choose", DefaultPath: start,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			m.applyLanguageToolBrowsedPath(name, paths[0], newServer)
		})
	}()
}

func (m *Model) applyLanguageToolBrowsedPath(name, path string, newServer bool) {
	if newServer {
		m.languageToolSettings.addCommand = quoteLanguageToolArg(path)
		m.invalidate()
		return
	}
	if isCustomLanguageServer(m.languageToolSettings.draft[name]) {
		m.stageLanguageToolCommand(name, path)
		return
	}
	var tool *javaTool
	for i := range m.languageTools {
		if m.languageTools[i].Name == name {
			tool = &m.languageTools[i]
			break
		}
	}
	if tool == nil {
		return
	}
	m.stageLanguageToolPath(name, tool.Command, path)
}

// parseLanguageToolArgs uses the shell-like quoting OpenIDEA's arguments field
// accepts: whitespace separates, quotes group and backslash escapes.
func parseLanguageToolArgs(text string) ([]string, error) {
	out := make([]string, 0)
	current := strings.Builder{}
	started := false
	var quote rune
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			switch {
			case r == quote:
				quote = 0
			case r == '\\' && quote == '"' && i+1 < len(runes):
				i++
				current.WriteRune(runes[i])
			default:
				current.WriteRune(r)
			}
			continue
		}
		switch {
		case r == '"' || r == '\'':
			quote = r
			started = true
		case r == '\\' && i+1 < len(runes):
			i++
			current.WriteRune(runes[i])
			started = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if started {
				out = append(out, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("Unclosed %c quote", quote)
	}
	if started {
		out = append(out, current.String())
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func quoteLanguageToolArg(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n'\"\\") {
		return value
	}
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return `"` + replacer.Replace(value) + `"`
}

func formatLanguageToolArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, quoteLanguageToolArg(arg))
	}
	return strings.Join(quoted, " ")
}

func parseLanguageToolEnv(text string) (map[string]string, error) {
	env := make(map[string]string)
	for _, raw := range strings.FieldsFunc(text, func(r rune) bool { return r == ';' || r == '\n' }) {
		pair := strings.TrimSpace(raw)
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !ok || !validEnvironmentName(key) {
			return nil, fmt.Errorf("Expected NAME=value: %s", pair)
		}
		env[key] = strings.TrimSpace(value)
	}
	if len(env) == 0 {
		return nil, nil
	}
	return env, nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') {
			return false
		}
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

func formatLanguageToolEnv(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+env[key])
	}
	return strings.Join(pairs, "; ")
}

func parseLanguageToolJSONObject(text string) (json.RawMessage, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("Expected a JSON object")
	}
	if path := jsonNullPath(value, ""); path != "" {
		if path == "top-level" {
			return nil, fmt.Errorf("null is not supported")
		}
		return nil, fmt.Errorf("null is not supported (at %s)", path)
	}
	raw, err := json.Marshal(value)
	return json.RawMessage(raw), err
}

func jsonNullPath(value any, path string) string {
	if value == nil {
		if path == "" {
			return "top-level"
		}
		return path
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if hit := jsonNullPath(child, next); hit != "" {
				return hit
			}
		}
	case []any:
		for i, child := range typed {
			next := fmt.Sprintf("[%d]", i)
			if path != "" {
				next = fmt.Sprintf("%s[%d]", path, i)
			}
			if hit := jsonNullPath(child, next); hit != "" {
				return hit
			}
		}
	}
	return ""
}

func formatLanguageToolJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(formatted)
}

func parseLanguageToolList(text string) []string {
	seen := map[string]bool{}
	var values []string
	for _, value := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	return values
}

func formatLanguageToolList(values []string) string { return strings.Join(values, ", ") }

func languageServerSettingWrites(applied, draft map[string]languageServerConfig) []languageSettingWrite {
	names := map[string]bool{}
	for name := range applied {
		names[name] = true
	}
	for name := range draft {
		names[name] = true
	}
	sortedNames := make([]string, 0, len(names))
	for name := range names {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)
	var writes []languageSettingWrite
	for _, name := range sortedNames {
		before, hadBefore := applied[name]
		after, hasAfter := draft[name]
		key := "languageServers." + name
		switch {
		case !hasAfter:
			if hadBefore {
				writes = append(writes, languageSettingWrite{key: key, value: nil})
			}
		case !hadBefore:
			writes = append(writes, languageSettingWrite{key: key, value: after})
		case sameLanguageServerConfig(before, after):
			continue
		default:
			writes = appendLanguageServerFieldWrites(writes, key, before, after)
		}
	}
	return writes
}

func appendLanguageServerFieldWrites(
	writes []languageSettingWrite,
	base string,
	before, after languageServerConfig,
) []languageSettingWrite {
	field := func(name string, changed bool, value any) []languageSettingWrite {
		if changed {
			return append(writes, languageSettingWrite{key: base + "." + name, value: value})
		}
		return writes
	}
	if !sameBoolPointer(before.Enabled, after.Enabled) {
		if after.Enabled == nil {
			writes = field("enabled", true, nil)
		} else {
			writes = field("enabled", true, *after.Enabled)
		}
	}
	if !sameStringPointer(before.Command, after.Command) {
		if after.Command == nil {
			writes = field("command", true, nil)
		} else {
			writes = field("command", true, *after.Command)
		}
	}
	if !sameStringSlice(before.Args, after.Args) {
		var args any
		if after.Args != nil {
			args = after.Args
		}
		writes = field("args", true, args)
	}
	if !sameStringMap(before.Env, after.Env) {
		var env any
		if after.Env != nil {
			env = after.Env
		}
		writes = field("env", true, env)
	}
	if !sameJSON(before.InitializationOptions, after.InitializationOptions) {
		writes = field("initializationOptions", true, after.InitializationOptions)
	}
	if !sameJSON(before.Settings, after.Settings) {
		writes = field("settings", true, after.Settings)
	}
	if !sameStringSlice(before.Languages, after.Languages) {
		var languages any
		if after.Languages != nil {
			languages = after.Languages
		}
		writes = field("languages", true, languages)
	}
	if !sameStringSlice(before.RootMarkers, after.RootMarkers) {
		var rootMarkers any
		if after.RootMarkers != nil {
			rootMarkers = after.RootMarkers
		}
		writes = field("rootMarkers", true, rootMarkers)
	}
	return writes
}

func languageToolPathWrites(
	base, draft map[string]string,
	toolsRaw map[string]json.RawMessage,
) []languageSettingWrite {
	changed := make(map[string]bool)
	for command, path := range base {
		if draft[command] != path {
			changed[command] = true
		}
	}
	for command, path := range draft {
		if base[command] != path {
			changed[command] = true
		}
	}
	if len(changed) == 0 {
		return nil
	}
	dotted := false
	for command := range changed {
		if strings.Contains(command, ".") {
			dotted = true
			break
		}
	}
	if !dotted {
		commands := make([]string, 0, len(changed))
		for command := range changed {
			commands = append(commands, command)
		}
		sort.Strings(commands)
		writes := make([]languageSettingWrite, 0, len(commands))
		for _, command := range commands {
			var value any
			if path := draft[command]; path != "" {
				value = path
			}
			writes = append(writes, languageSettingWrite{key: "tools." + command + ".path", value: value})
		}
		return writes
	}

	nextTools := cloneRawMap(toolsRaw)
	for command := range changed {
		var tool map[string]json.RawMessage
		if raw := nextTools[command]; len(raw) > 0 {
			_ = json.Unmarshal(raw, &tool)
		}
		if tool == nil {
			tool = map[string]json.RawMessage{}
		}
		if path := strings.TrimSpace(draft[command]); path != "" {
			encodedPath, _ := json.Marshal(path)
			tool["path"] = encodedPath
		} else {
			delete(tool, "path")
		}
		if len(tool) == 0 {
			delete(nextTools, command)
			continue
		}
		encodedTool, _ := json.Marshal(tool)
		nextTools[command] = encodedTool
	}
	return []languageSettingWrite{{key: "tools", value: nextTools}}
}

func languageToolRawAfterPaths(
	toolsRaw map[string]json.RawMessage,
	pathBase, pathDraft map[string]string,
) map[string]json.RawMessage {
	out := cloneRawMap(toolsRaw)
	commands := make(map[string]bool, len(pathBase)+len(pathDraft))
	for command := range pathBase {
		commands[command] = true
	}
	for command := range pathDraft {
		commands[command] = true
	}
	for command := range commands {
		var tool map[string]json.RawMessage
		if raw := out[command]; len(raw) > 0 {
			_ = json.Unmarshal(raw, &tool)
		}
		if tool == nil {
			tool = map[string]json.RawMessage{}
		}
		if path := strings.TrimSpace(pathDraft[command]); path != "" {
			encodedPath, _ := json.Marshal(path)
			tool["path"] = encodedPath
		} else {
			delete(tool, "path")
		}
		if len(tool) == 0 {
			delete(out, command)
			continue
		}
		encodedTool, _ := json.Marshal(tool)
		out[command] = encodedTool
	}
	return out
}

func languageServerEditorsError(editors map[string]*languageToolEditorState) string {
	names := make([]string, 0, len(editors))
	for name := range editors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		editor := editors[name]
		if editor == nil || len(editor.Errors) == 0 {
			continue
		}
		fields := make([]string, 0, len(editor.Errors))
		for field := range editor.Errors {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		return editor.Errors[fields[0]]
	}
	return ""
}

func (m *Model) languageServerSettingsError() string {
	state := &m.languageToolSettings
	if state.err != "" {
		return state.err
	}
	if state.loaded {
		if errText := languageServerEditorsError(state.editors); errText != "" {
			return errText
		}
	}
	scopes := make([]string, 0, len(state.scopes))
	for scope := range state.scopes {
		if scope != state.scope {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		cached := state.scopes[scope]
		if cached == nil || !cached.loaded {
			continue
		}
		if errText := languageServerEditorsError(cached.editors); errText != "" {
			return errText
		}
	}
	return ""
}

func (m *Model) pendingLanguageServerSaves() []languageServerScopeSave {
	state := &m.languageToolSettings
	m.captureLanguageServerScope()
	scopes := make([]string, 0, len(state.scopes))
	for scope := range state.scopes {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	saves := make([]languageServerScopeSave, 0, len(scopes))
	for _, scope := range scopes {
		cached := state.scopes[scope]
		if cached == nil || !cached.loaded || !languageServerScopeDirty(
			cached.applied, cached.draft, cached.pathBase, cached.pathDraft,
		) {
			continue
		}
		writes := languageServerSettingWrites(cached.applied, cached.draft)
		writes = append(writes, languageToolPathWrites(
			cached.pathBase, cached.pathDraft, cached.toolsRaw,
		)...)
		if len(writes) > 0 {
			saves = append(saves, languageServerScopeSave{scope: scope, writes: writes})
		}
	}
	return saves
}

func (m *Model) commitLanguageServerScope(scope string) {
	state := &m.languageToolSettings
	cached := state.scopes[scope]
	if cached == nil || !cached.loaded {
		return
	}
	pathBase := cached.pathBase
	cached.applied = cloneLanguageServerMap(cached.draft)
	cached.pathBase = cloneStringMap(cached.pathDraft)
	cached.toolsRaw = languageToolRawAfterPaths(
		cached.toolsRaw, pathBase, cached.pathDraft,
	)
	if state.scope == scope {
		m.restoreLanguageServerScope(cached)
	}
}

// saveLanguageServerSettings writes changed keys from every scope staged in
// the dialog. JSON null removes a server, field or path override.
func (m *Model) saveLanguageServerSettings(closeAfter bool) {
	state := &m.languageToolSettings
	if state.saving {
		return
	}
	m.captureLanguageServerScope()
	if !languageToolSettingsDirty(state) {
		if closeAfter {
			m.closeSettings()
		}
		return
	}
	if errText := m.languageServerSettingsError(); errText != "" {
		m.settingsErr = errText
		m.invalidate()
		return
	}
	saves := m.pendingLanguageServerSaves()
	if len(saves) == 0 {
		if closeAfter {
			m.closeSettings()
		}
		return
	}
	state.saving = true
	state.closeAfterSave = closeAfter
	state.err = ""
	m.settingsErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		for _, save := range saves {
			for _, write := range save.writes {
				value, err := json.Marshal(write.value)
				if err != nil {
					m.finishLanguageServerSave(save.scope, write.key, err)
					return
				}
				_, err = m.invoke(ctx, "settings_set", map[string]any{
					"scope": save.scope, "key": write.key, "value": string(value),
				})
				if err != nil {
					m.finishLanguageServerSave(save.scope, write.key, err)
					return
				}
			}
		}
		var reloaded []javaTool
		if m.mgr != nil {
			if list, err := m.mgr.ToolsList(ctx, Window); err == nil {
				reloaded = languageToolRows(list)
			}
		}
		m.post(func() {
			state.saving = false
			state.err = ""
			for _, save := range saves {
				m.commitLanguageServerScope(save.scope)
			}
			state.closeAfterSave = false
			m.settingsErr = ""
			if reloaded != nil {
				m.languageTools = reloaded
				m.javaTools = javaToolRowsFrom(reloaded)
				m.toolsLoaded = true
				m.ensureLanguageToolSelection()
			}
			m.status = "Language server settings updated"
			if closeAfter {
				m.closeSettings()
				return
			}
			m.invalidate()
		})
	}()
}

func (m *Model) finishLanguageServerSave(scope, key string, err error) {
	m.post(func() {
		state := &m.languageToolSettings
		state.saving = false
		state.closeAfterSave = false
		state.err = "Could not save " + scope + " " + key + ": " + err.Error()
		m.settingsErr = state.err
		m.invalidate()
	})
}

func languageToolStatusLabel(status string) string {
	switch status {
	case "installed":
		return "Installed"
	case "onPath":
		return "On PATH"
	case "custom":
		return "Set in Settings"
	case "missing":
		return "Missing"
	}
	return status
}

func (m *Model) languageToolNames(ids []string) []string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name := id
		for _, language := range m.languageToolSettings.languages {
			if language.ID == id {
				name = language.Name
				break
			}
		}
		names = append(names, name)
	}
	return names
}

// settingsLanguageServersBody is Settings › Languages & Frameworks ›
// Language Servers. It mirrors OpenIDEA's tool-first list and stages settings
// in the surrounding Settings dialog.
func (m *Model) settingsLanguageServersBody(c *ui.Context, p *ui.Theme, oi oiColors) {
	state := &m.languageToolSettings
	ui.Text(c, "Language servers and debug adapters used by the IDE. Missing tools can be installed into the IDE's own tools folder; nothing is downloaded until you click Install.").
		FontSize(12).TextColor(oi.textMuted).MaxWidth(760)
	if m.Root() != "" && !m.settingsDraft.newProjects {
		ui.Row(c).Height(8)
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			for _, choice := range []struct {
				value string
				label string
			}{
				{"global", "Global"},
				{"project", "This project"},
			} {
				choice := choice
				button := ui.Button(c, choice.label).
					Disabled(state.loading || state.saving)
				if state.scope == choice.value {
					button.Background(oi.selected)
				}
				button.OnClick(func() { m.SetLanguageServerScope(choice.value) })
			}
		})
	}
	note := "Saved to the global settings: used by new projects and by projects without their own value."
	if state.scope == "project" && m.Root() != "" {
		note = "Saved to this project's GoIDEA settings; overrides the global value."
	}
	ui.Text(c, note).FontSize(11).TextColor(oi.textSubtle).MaxWidth(760)
	if state.err != "" {
		ui.Text(c, state.err).FontSize(12).TextColor(p.Danger).MaxWidth(760)
	}
	if state.loading && !state.loaded {
		ui.Row(c).Height(10)
		ui.Text(c, "Reading language server settings…").FontSize(12).TextColor(oi.textMuted)
	}

	ui.Row(c).Height(10)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "+").Width(28).Tooltip("Add Language Server…").OnClick(m.openLanguageServerAddDialog)
		selectedCustom := isCustomLanguageServer(state.draft[m.languageToolPick])
		ui.Button(c, "−").Width(28).Tooltip("Remove Language Server").Disabled(!selectedCustom).
			OnClick(func() { m.removeLanguageServer(m.languageToolPick) })
		label := "Reload"
		if m.toolsLoading {
			label = "Checking…"
		}
		ui.Button(c, label).Disabled(m.toolsLoading || m.toolsBusy != "").
			OnClick(func() { m.LoadJavaTools() })
		ui.SearchField(c, &m.languageToolFind).Grow(1).Placeholder("Filter by tool or language")
	})
	ui.Row(c).Height(8)
	all := m.languageToolView()
	missingCount := languageToolsMissingCount(all)
	ui.Row(c).Gap(4).Children(func() {
		for _, choice := range []struct {
			value string
			label string
		}{
			{languageToolFilterAll, "All"},
			{languageToolFilterMissing, fmt.Sprintf("Missing (%d)", missingCount)},
			{languageToolFilterAvailable, "Available"},
		} {
			choice := choice
			on := m.normalizedLanguageToolFilter() == choice.value
			b := ui.Button(c, choice.label)
			if on {
				b.Background(oi.selected)
			}
			b.OnClick(func() {
				m.languageToolFilter = choice.value
				m.ensureLanguageToolSelection()
				m.invalidate()
			})
		}
	})
	ui.Row(c).Height(10)

	shown := filterLanguageTools(all, m.languageToolFind, m.normalizedLanguageToolFilter())
	m.ensureLanguageToolSelection()
	ready := make([]javaTool, 0, len(shown))
	missing := make([]javaTool, 0, len(shown))
	for _, tool := range shown {
		if tool.Status == "missing" {
			missing = append(missing, tool)
		} else {
			ready = append(ready, tool)
		}
	}
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(350).Shrink(0).Gap(3).Children(func() {
			if len(shown) == 0 {
				hint := "No tools match."
				if m.toolsLoading || !m.toolsLoaded {
					hint = "Checking installed tools…"
				}
				ui.Text(c, hint).FontSize(12).TextColor(oi.textMuted)
			}
			m.languageToolGroup(c, oi, "Available", ready)
			m.languageToolGroup(c, oi, "Missing", missing)
		})
		ui.Column(c).Grow(1).MinWidth(340).Gap(8).Children(func() {
			var selected *javaTool
			for i := range shown {
				if shown[i].Name == m.languageToolPick {
					selected = &shown[i]
					break
				}
			}
			if selected == nil {
				title := "No tool selected"
				if !m.toolsLoaded && !state.loaded {
					title = "Loading…"
				}
				ui.Text(c, title).FontSize(12).TextColor(oi.textMuted)
				return
			}
			m.languageToolDetails(c, p, oi, *selected)
		})
	})

	if m.toolsErr != "" {
		ui.Row(c).Height(8)
		ui.Text(c, m.toolsErr).FontSize(12).TextColor(p.Danger).MaxWidth(760)
	}
	if m.toolsBusy != "" {
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			text := m.toolsBusy
			if m.toolsProgress != "" {
				text += " · " + m.toolsProgress
			}
			ui.Text(c, text).FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			ui.Spacer(c)
			if m.toolsRequest != 0 {
				ui.Button(c, "Cancel").OnClick(m.CancelToolInstall)
			}
		})
	}
	if len(m.toolsLog) > 0 {
		label := "Show install log"
		if state.logOpen {
			label = "Hide install log"
		}
		ui.Button(c, label).OnClick(func() {
			state.logOpen = !state.logOpen
			m.invalidate()
		})
		if state.logOpen {
			log := strings.Join(m.toolsLog, "\n")
			ui.TextArea(c, &log).ReadOnly(true).Height(120).Font("monospace").
				FontSize(11).TextColor(oi.textMuted)
		}
	}
	m.languageServerAddDialog(c, p, oi)
}

func (m *Model) languageToolGroup(c *ui.Context, oi oiColors, title string, tools []javaTool) {
	if len(tools) == 0 {
		return
	}
	ui.Row(c).MinHeight(28).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(12).Bold().TextColor(oi.textStrong).SingleLine()
		ui.Text(c, fmt.Sprintf("%d", len(tools))).FontSize(11).TextColor(oi.textSubtle)
	})
	for _, tool := range tools {
		tool := tool
		entry := m.languageToolSettings.draft[tool.Name]
		off := entry.Enabled != nil && !*entry.Enabled
		row := ui.Row(c.Key("language-servers:tool:"+tool.Name)).MinHeight(46).
			Radius(4).Padding(4, 8).Gap(8).AlignItems(ui.Center)
		if tool.Name == m.languageToolPick {
			row.Background(oi.selected)
		} else if row.Hovered() {
			row.Background(oi.hoverTint)
		}
		row.Children(func() {
			badge := languageToolStatusLabel(tool.Status)
			color := oi.success
			if tool.PendingCreate {
				badge = "New"
				color = oi.accent
			} else if tool.Status == "missing" {
				color = oi.warning
			}
			ui.Text(c, badge).FontSize(10).TextColor(color).Width(52).SingleLine()
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, tool.Display).FontSize(12).TextColor(oi.text).
					SingleLine().Ellipsis("…")
				details := make([]string, 0, 2)
				if tool.Kind != "languageServer" {
					details = append(details, languageToolKindLabel(tool.Kind))
				}
				if len(tool.Languages) > 0 {
					details = append(details, strings.Join(m.languageToolNames(tool.Languages), ", "))
				}
				if off {
					details = append(details, "Disabled")
				}
				ui.Text(c, strings.Join(details, " · ")).FontSize(11).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…")
			})
		})
		if row.Clicked() {
			m.languageToolPick = tool.Name
			m.invalidate()
		}
	}
}

func (m *Model) languageToolDetails(c *ui.Context, p *ui.Theme, oi oiColors, tool javaTool) {
	state := &m.languageToolSettings
	entry := state.draft[tool.Name]
	custom := isCustomLanguageServer(entry)
	settingsSectionTitle(c, oi, tool.Display)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		badge := languageToolStatusLabel(tool.Status)
		color := oi.success
		if tool.PendingCreate {
			badge = "New"
			color = oi.accent
		} else if tool.Status == "missing" {
			color = oi.warning
		}
		ui.Text(c, badge).FontSize(11).TextColor(color).SingleLine()
		ui.Text(c, languageToolKindLabel(tool.Kind)).FontSize(11).TextColor(oi.textMuted).SingleLine()
		if tool.Version != "" {
			ui.Text(c, "· "+tool.Version).FontSize(11).TextColor(oi.textSubtle).SingleLine()
		}
		ui.Spacer(c)
		if tool.CanInstall {
			label := "Install"
			if tool.Status == "installed" {
				label = "Reinstall"
			}
			ui.Button(c, label).Disabled(m.toolsBusy != "").Tooltip(func() string {
				if tool.Install == "" {
					return ""
				}
				return "Installs into the IDE's own tools folder via " + tool.Install
			}()).OnClick(func() { m.InstallLanguageTool(tool.Name, false) })
		}
		if tool.CanRemove {
			ui.Button(c, "Uninstall").Disabled(m.toolsBusy != "").
				OnClick(func() { m.InstallLanguageTool(tool.Name, true) })
		}
	})

	if tool.CustomError != "" {
		ui.Text(c, tool.CustomError).FontSize(12).TextColor(p.Danger).MaxWidth(680)
	} else if tool.PendingCreate {
		ui.Text(c, "Checked when the settings are applied.").FontSize(11).TextColor(oi.textMuted).
			SingleLine().Ellipsis("…")
	} else if tool.Path != "" {
		line := "Using " + tool.Path
		if tool.Version != "" {
			line += " (" + tool.Version + ")"
		}
		ui.Text(c, line).FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	} else if tool.Hint != "" {
		ui.Text(c, tool.Hint).FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	} else if tool.Status == "missing" {
		ui.Text(c, "Not found on PATH.").FontSize(11).TextColor(oi.textMuted).SingleLine()
	}
	if tool.Reason != "" && !tool.CanInstall {
		ui.Text(c, tool.Reason).FontSize(12).TextColor(p.Danger).MaxWidth(680)
	}
	if tool.Homepage != "" {
		ui.Text(c, tool.Homepage).FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	}

	groups := m.languageToolAlternativeGroups(tool)
	if len(groups) == 0 {
		enabled := entry.Enabled == nil || *entry.Enabled
		languages := strings.Join(m.languageToolNames(tool.Languages), ", ")
		label := "Use " + tool.Display
		if languages != "" {
			label += " for " + languages + " files"
		}
		if ui.Checkbox(c, &enabled, label).Changed() {
			m.stageLanguageToolEnabled(tool.Name, enabled)
		}
	} else {
		for _, names := range groups {
			m.languageToolAlternativesRow(c, oi, names)
		}
	}

	settingsSectionTitle(c, oi, "Executable")
	editor := m.languageToolEditor(tool.Name)
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		label := "Path"
		if custom {
			label = "Command"
		}
		settingsFormLabel(c, oi, label, 150)
		var input ui.Element
		if custom {
			input = ui.TextInput(c, &editor.Command).Grow(1).Font("monospace")
			if input.Changed() {
				m.stageLanguageToolCommand(tool.Name, editor.Command)
			}
		} else {
			input = ui.TextInput(c, &editor.Path).Grow(1).Font("monospace")
			if input.Changed() {
				m.stageLanguageToolPath(tool.Name, tool.Command, editor.Path)
			}
		}
		if custom {
			input.Placeholder("Executable name or path")
		} else {
			input.Placeholder("Auto-detect")
		}
		ui.Button(c, "Browse…").OnClick(func() {
			m.BrowseLanguageToolExecutable(tool.Name, false)
		})
	})
	if custom {
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, "Languages", 150)
			in := ui.TextInput(c, &editor.Languages).Grow(1).Placeholder("java, kotlin")
			if in.Changed() {
				m.stageLanguageToolList(tool.Name, "languages", editor.Languages)
			}
		})
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, "Root markers", 150)
			in := ui.TextInput(c, &editor.RootMarkers).Grow(1).Placeholder("pyproject.toml, .git")
			if in.Changed() {
				m.stageLanguageToolList(tool.Name, "rootMarkers", editor.RootMarkers)
			}
		})
		ui.Text(c, "Files marking the workspace root.").FontSize(11).
			TextColor(oi.textMuted).PaddingStart(158).SingleLine().Ellipsis("…")
	}

	settingsSectionTitle(c, oi, "Startup")
	ui.Text(c, "Changing these restarts the server.").FontSize(11).TextColor(oi.textMuted).
		SingleLine().Ellipsis("…")
	m.languageToolTextRow(c, oi, "Arguments", tool.Name, "args", &editor.Args,
		formatLanguageToolArgs(tool.DefaultArgs), false)
	m.languageToolTextRow(c, oi, "Environment variables", tool.Name, "env", &editor.Env,
		"NAME=value; OTHER=value", false)
	placeholder := `{ "key": "value" }`
	if tool.DefaultInit != "" {
		var value any
		if json.Unmarshal([]byte(tool.DefaultInit), &value) == nil {
			placeholder = formatLanguageToolJSON(json.RawMessage(tool.DefaultInit))
		}
	}
	m.languageToolTextRow(c, oi, "Initialization options", tool.Name, "initializationOptions",
		&editor.InitializationOptions, placeholder, true)

	settingsSectionTitle(c, oi, "Workspace Configuration")
	ui.Text(c, "Sent to the running server; no restart needed.").FontSize(11).TextColor(oi.textMuted).
		SingleLine().Ellipsis("…")
	m.languageToolTextRow(c, oi, "Settings", tool.Name, "settings", &editor.Settings,
		`{ "section": { "key": "value" } }`, true)

	if !custom && (!languageServerConfigEmpty(entry) || editor.Path != state.pathBase[tool.Command]) {
		ui.Button(c, "Restore Defaults").OnClick(func() { m.restoreLanguageServerDefaults(tool.Name) })
	}
}

func (m *Model) languageToolTextRow(
	c *ui.Context,
	oi oiColors,
	label string,
	name, field string,
	value *string,
	placeholder string,
	multiline bool,
) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label, 150)
		if multiline {
			in := ui.TextArea(c, value).Grow(1).Height(86).Font("monospace").
				Placeholder(placeholder)
			if in.Changed() {
				m.stageLanguageToolJSON(name, field, *value)
			}
			return
		}
		in := ui.TextInput(c, value).Grow(1).Placeholder(placeholder)
		if in.Changed() {
			if field == "env" {
				m.stageLanguageToolEnv(name, *value)
			} else {
				m.stageLanguageToolArgs(name, *value)
			}
		}
	})
	editor := m.languageToolEditor(name)
	if errText := editor.Errors[field]; errText != "" {
		ui.Text(c, errText).FontSize(11).TextColor(oi.danger).PaddingStart(158).MaxWidth(600)
	}
}

func (m *Model) languageToolAlternativeGroups(tool javaTool) [][]string {
	groups := make(map[string][]string)
	for _, candidate := range m.languageToolView() {
		if len(candidate.Alternatives) < 2 || !slices.Contains(candidate.Alternatives, tool.Name) {
			continue
		}
		names := append([]string(nil), candidate.Alternatives...)
		groups[strings.Join(names, "\x00")] = names
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, groups[key])
	}
	return out
}

func (m *Model) languageToolAlternativesRow(c *ui.Context, oi oiColors, names []string) {
	all := m.languageToolView()
	byName := make(map[string]javaTool, len(all))
	for _, tool := range all {
		byName[tool.Name] = tool
	}
	enabled := make([]string, 0, len(names))
	labels := []string{"Automatic"}
	for _, name := range names {
		tool := byName[name]
		display := tool.Display
		if display == "" {
			display = name
		}
		labels = append(labels, display)
		entry := m.languageToolSettings.draft[name]
		if entry.Enabled == nil || *entry.Enabled {
			enabled = append(enabled, name)
		}
	}
	labels = append(labels, "None")
	shown := "Automatic"
	switch {
	case len(enabled) == 0:
		shown = "None"
	case len(enabled) == len(names):
		shown = "Automatic"
	case len(enabled) == 1:
		if tool := byName[enabled[0]]; tool.Display != "" {
			shown = tool.Display
		} else {
			shown = enabled[0]
		}
	default:
		shown = "Custom combination"
		labels = append([]string{"Custom combination"}, labels...)
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Language server", 150)
		sel := ui.Select(c, &shown, labels).Width(260)
		if sel.Changed() {
			for _, name := range names {
				on := shown == "Automatic" || shown == byName[name].Display || shown == name
				if shown == "None" || shown == "Custom combination" {
					on = false
				}
				m.stageLanguageToolEnabled(name, on)
			}
		}
	})
	ui.Text(c, "Only one of these runs at a time. Automatic uses the first one available.").
		FontSize(11).TextColor(oi.textMuted).PaddingStart(158).SingleLine().Ellipsis("…")
}

func languageToolSelectionHas(text, id string) bool {
	return slices.Contains(parseLanguageToolList(text), id)
}

func toggleLanguageToolSelection(text, id string, on bool) string {
	values := parseLanguageToolList(text)
	if on && !slices.Contains(values, id) {
		values = append(values, id)
	}
	if !on {
		values = slices.DeleteFunc(values, func(value string) bool { return value == id })
	}
	return formatLanguageToolList(values)
}

func (m *Model) languageServerAddProblem() string {
	state := &m.languageToolSettings
	args, err := parseLanguageToolArgs(state.addCommand)
	if err != nil {
		return err.Error()
	}
	if len(args) == 0 {
		return "Enter the command that starts the server."
	}
	name := strings.TrimSpace(state.addName)
	if name == "" {
		name = languageServerNameFromCommand(args[0])
	}
	if !validLanguageServerName(name) {
		return "Name: letters, digits, - _ + only."
	}
	for _, tool := range m.languageToolView() {
		if tool.Name == name {
			return name + " already exists."
		}
	}
	if len(parseLanguageToolList(state.addLanguages)) == 0 {
		return "Choose the languages it serves."
	}
	return ""
}

func (m *Model) languageServerAddDialog(c *ui.Context, p *ui.Theme, oi oiColors) {
	state := &m.languageToolSettings
	if !state.addOpen {
		return
	}
	open := state.addOpen
	nativeModal(c.Key("language-servers:add"), &open, func() {
		ui.Column(c).Width(520).Gap(10).Children(func() {
			ui.Text(c, "New Language Server").FontSize(15).Bold().TextColor(p.Text).SingleLine()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				settingsFormLabel(c, oi, "Command", 110)
				in := ui.TextInput(c, &state.addCommand).Grow(1).Font("monospace").AutoFocus().
					Placeholder("pylyzer --server")
				if in.Changed() {
					state.addErr = ""
					m.invalidate()
				}
				ui.Button(c, "Browse…").OnClick(func() {
					m.BrowseLanguageToolExecutable("", true)
				})
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				settingsFormLabel(c, oi, "Name", 110)
				in := ui.TextInput(c, &state.addName).Grow(1).Font("monospace").
					Placeholder("Automatic name")
				if in.Changed() {
					state.addErr = ""
					m.invalidate()
				}
			})
			ui.Text(c, "Languages").FontSize(12).TextColor(oi.text).SingleLine()
			if len(state.languages) == 0 {
				ui.Text(c, "No language choices are available.").FontSize(12).
					TextColor(oi.textMuted).SingleLine()
			} else {
				ui.Column(c).Gap(2).PaddingStart(12).Children(func() {
					for _, language := range state.languages {
						language := language
						on := languageToolSelectionHas(state.addLanguages, language.ID)
						label := language.Name
						if label == "" {
							label = language.ID
						}
						if ui.Checkbox(c, &on, label).Changed() {
							state.addLanguages = toggleLanguageToolSelection(state.addLanguages, language.ID, on)
							state.addErr = ""
							m.invalidate()
						}
					}
				})
			}
			problem := state.addErr
			if problem == "" {
				problem = m.languageServerAddProblem()
			}
			color := oi.textMuted
			if problem != "" {
				color = p.Danger
			}
			ui.Text(c, problem).FontSize(11).TextColor(color).MaxWidth(480)
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(m.closeLanguageServerAddDialog)
				ui.Button(c, "OK").Disabled(m.languageServerAddProblem() != "").
					OnClick(m.addLanguageServer)
			})
		})
	})
	if !open && state.addOpen {
		state.addOpen = false
		state.addErr = ""
		m.invalidate()
	}
}

func parityToolCommands() []appCommand {
	return []appCommand{
		{"tools.languageServers", "Language Servers…", "Tools", "", "lsp language server debugger adapter install", func(m *Model) {
			m.OpenSettingsPage(settingsPageLanguageServers)
		}},
		{"tools.install", "Install Language Server / Debugger", "Tools", "", "install language server debugger adapter tool", func(m *Model) {
			m.OpenSettingsPage(settingsPageLanguageServers)
		}},
		{"tools.promptInstall", "Install Missing Tool", "Tools", "", "prompt install missing tool", func(m *Model) {
			m.OpenSettingsPage(settingsPageLanguageServers)
		}},
	}
}

func parityToolActionState(a *Action) {
	switch a.ID {
	case "tools.install":
		a.RunWithArg = func(m *Model, arg any) {
			name, _ := arg.(string)
			name = strings.TrimSpace(name)
			if name != "" {
				if m.toolsLoaded {
					var found *javaTool
					for i := range m.languageTools {
						if m.languageTools[i].Name == name {
							found = &m.languageTools[i]
							break
						}
					}
					if found == nil || !found.CanInstall {
						m.languageToolPick = name
						m.OpenSettingsPage(settingsPageLanguageServers)
						return
					}
				}
				m.InstallLanguageTool(name, false)
				return
			}
			m.OpenSettingsPage(settingsPageLanguageServers)
		}
	case "tools.promptInstall":
		a.RunWithArg = func(m *Model, arg any) {
			name := ""
			sticky, formatter := true, false
			switch value := arg.(type) {
			case string:
				name = value
			case map[string]any:
				name, _ = value["name"].(string)
				if v, ok := value["sticky"].(bool); ok {
					sticky = v
				}
				formatter, _ = value["formatter"].(bool)
			}
			m.PromptInstallLanguageTool(name, sticky, formatter)
		}
	}
}
