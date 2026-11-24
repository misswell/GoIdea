package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// parityFileFindWindowActions completes OpenIDEA's File, Find-field and Window
// action surface. They register after actions_builtin.go so the normal action
// lifecycle also gives them keymap, palette, menu and automation entry points.
func parityFileFindWindowActions() []*Action {
	return []*Action{
		{
			ID: "file.exportSettings", Title: "Export Settings…", Category: "File",
			Keywords: "backup settings keymap templates manage ide settings",
			RunWithArg: func(m *Model, arg any) {
				m.ExportSettings(arg)
			},
		},
		{
			ID: "file.importSettings", Title: "Import Settings…", Category: "File",
			Keywords: "restore settings keymap templates manage ide settings",
			RunWithArg: func(m *Model, arg any) {
				m.ImportSettings(arg)
			},
		},
		{
			ID: "file.importJetBrainsSettings", Title: "Import Settings from JetBrains IDE…",
			Category: "File",
			Keywords: "intellij pycharm webstorm android studio compact migrate manage ide settings",
			RunWithArg: func(m *Model, arg any) {
				m.ImportJetBrainsSettings(arg)
			},
		},
		{
			ID: "file.newProjectsSettings", Title: "Settings for New Projects…", Category: "File",
			Keywords: "new projects setup default settings default project global settings jdk",
			RunWithArg: func(m *Model, arg any) {
				page, _ := arg.(string)
				m.OpenNewProjectsSettings(page)
			},
		},
		{
			ID: "file.openInNewWindow", Title: "Open Project in New Window", Category: "File",
			Keywords: "window project folder open",
			RunWithArg: func(m *Model, arg any) {
				m.OpenProjectInNewWindow(arg)
			},
		},
		{
			ID: "file.renameProject", Title: "Rename Project…", Category: "File",
			Keywords: "project name display folder",
			RunWithArg: func(m *Model, arg any) {
				m.RenameProject(arg)
			},
			When: func(m *Model) bool { return m.Root() != "" && !m.showingWelcome },
		},
		{
			ID: "file.togglePowerSave", Title: "Power Save Mode", Category: "File",
			Keywords: "battery performance completion documentation usage highlights",
			Run: func(m *Model) {
				m.TogglePowerSave()
			},
			Toggled: func(m *Model) bool { return m.prefs.General.PowerSaveMode },
		},
		{
			ID: "find.field.newLine", Title: "New Line in Search Field", Category: "Find",
			Keywords: "find search multi-line newline",
			Run:      func(m *Model) { m.AddNewLineToFindField() },
			When: func(m *Model) bool {
				return m.Root() != "" || m.toolVisible("find")
			},
		},
		{
			ID: "find.field.recentSearches", Title: "Recent Searches", Category: "Find",
			Keywords: "find search history recent",
			Run:      func(m *Model) { m.ToggleRecentSearchFields() },
			When: func(m *Model) bool {
				return m.Root() != "" || m.toolVisible("find")
			},
		},
		{
			ID: "find.openInWindow", Title: "Open Results in Find Window", Category: "Find",
			Keywords: "find results window project search",
			RunWithArg: func(m *Model, arg any) {
				m.OpenFindResultsInWindow(arg)
			},
			When: func(m *Model) bool { return m.activeEditorAvailable() },
		},
		{
			ID: "find.scrollToResults", Title: "Scroll to Results During Typing", Category: "Find",
			Keywords: "find results scroll follow type",
			Run:      func(m *Model) { m.ToggleFindScrollToResults() },
			Toggled: func(m *Model) bool {
				return m.FindScrollToResults()
			},
			When: func(m *Model) bool {
				return m.Root() != "" || m.toolVisible("find")
			},
		},
		{
			ID: "window.detachTab", Title: "Move Tab to New Window", Category: "Window",
			Keywords: "window group tab detach split move",
			RunWithArg: func(m *Model, arg any) {
				m.DetachWindowTab(arg)
			},
			When: func(m *Model) bool { return m.tab() != nil },
		},
		{
			ID: "window.mergeAllWindows", Title: "Merge All Project Windows", Category: "Window",
			Keywords: "window group tab merge combine",
			Run:      func(m *Model) { m.MergeAllWindowGroups() },
			When:     func(m *Model) bool { return m.tab() != nil },
		},
		{
			ID: "window.new", Title: "New Window", Category: "Window",
			Keywords: "new window project welcome",
			Run:      func(m *Model) { m.NewSingleWindow() },
		},
		{
			ID: "window.next", Title: "Next Window", Category: "Window",
			Keywords: "window group cycle next",
			Run:      func(m *Model) { m.CycleWindowGroup(false) },
			When: func(m *Model) bool {
				return m.split != nil || m.ProjectWindowCount() > 1
			},
		},
		{
			ID: "window.previous", Title: "Previous Window", Category: "Window",
			Keywords: "window group cycle previous",
			Run:      func(m *Model) { m.CycleWindowGroup(true) },
			When: func(m *Model) bool {
				return m.split != nil || m.ProjectWindowCount() > 1
			},
		},
	}
}

func init() {
	for _, a := range parityFileFindWindowActions() {
		if _, exists := actionGet(a.ID); exists {
			continue
		}
		a.Builtin = true
		builtinShortcuts[a.ID] = ""
		builtinCategories[a.ID] = a.Category
		_ = registerAction(a)
	}
}

// OpenProjectInNewWindow creates an independent native project window when a
// window host is attached. The standalone fallback keeps the guarded
// single-window behavior for tests and alternate front ends.
func (m *Model) OpenProjectInNewWindow(arg any) {
	path := projectPathArg(arg)
	var files []string
	if values, ok := arg.(map[string]any); ok {
		if raw, exists := values["files"]; exists {
			files = stringPaths(raw)
		} else if raw, exists := values["paths"]; exists {
			files = stringPaths(raw)
		}
	}
	if strings.TrimSpace(path) == "" {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    m.win,
			Title:     "Open Project in New Window",
			Directory: true,
		})
		if err != nil {
			m.paritySettingsError("Cannot open project", err)
			return
		}
		if len(paths) > 0 {
			path = paths[0]
		}
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	path = realPath(path)
	for i, file := range files {
		files[i] = realPath(file)
	}
	if m.openProjectInNativeWindow(path, files) {
		return
	}
	if m.workspaceHost != nil {
		m.status = "Could not open another project window"
		m.invalidate()
		return
	}
	m.askOpenInSingleWindow(path, nil)
}

// NewSingleWindow is retained for callers of the former one-window action.
// With a native-window host it creates a second window. The standalone
// fallback leaves the current project through the normal unsaved-changes flow
// and shows Welcome.
func (m *Model) NewSingleWindow() {
	if m.openEmptyNativeWindow() {
		return
	}
	if m.workspaceHost != nil {
		m.status = "Could not open another project window"
		m.invalidate()
		return
	}
	if m.Root() == "" {
		m.showWelcome()
		m.status = "This front end opens one native window"
		m.invalidate()
		return
	}
	m.status = "This front end opens one native window; closing this project shows Welcome"
	m.CloseProject()
	m.invalidate()
}

// RenameProject opens OpenIDEA's project prompt, or applies an exact name when
// automation supplies one. Empty input restores the folder name.
func (m *Model) RenameProject(arg any) {
	if m.Root() == "" || m.showingWelcome {
		return
	}
	if name, ok := arg.(string); ok {
		m.renameProjectWithDialog(nil, name)
		return
	}
	dialog := m.openParityDialog()
	dialog.parity.Kind = "project-rename"
	dialog.parity.Title = "Rename Project"
	dialog.parity.Body = "Leave empty to use the folder name (" +
		filepath.Base(m.Root()) + ")."
	dialog.name = strings.TrimSpace(m.projectName)
	if dialog.name == "" {
		dialog.name = filepath.Base(m.Root())
	}
	m.invalidate()
}

func (m *Model) submitProjectRename(dialog *workspaceDialogState) {
	if dialog == nil || dialog.busy {
		return
	}
	m.renameProjectWithDialog(dialog, dialog.name)
}

func (m *Model) renameProjectWithDialog(dialog *workspaceDialogState, name string) {
	if m.mgr == nil && m.invokeHook == nil {
		if dialog != nil {
			dialog.error = "The GoIDEA backend is not connected"
			m.invalidate()
			return
		}
		m.status = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	if dialog != nil {
		dialog.busy = true
		dialog.error = ""
	}
	m.status = "Renaming project…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "project_rename", map[string]any{
			"name": strings.TrimSpace(name),
		})
		var info openidea.ProjectInfo
		if err == nil {
			info, err = openidea.DecodeProjectInfo(raw)
		}
		m.post(func() {
			if dialog != nil && m.workspaceDialog != dialog {
				return
			}
			if err != nil {
				if dialog != nil {
					dialog.busy = false
					dialog.error = "Cannot rename project: " + err.Error()
				} else {
					m.paritySettingsError("Cannot rename project", err)
				}
				m.invalidate()
				return
			}
			m.setProjectInfo(info)
			m.saveSession()
			m.workspaceDialog = nil
			m.status = "Project renamed to " + m.projectLabel()
			if m.win != nil {
				m.applyActive()
			}
			m.invalidate()
		})
	}()
}

// OpenFindResultsInWindow transfers the selected text (or an explicit query)
// into the project Find window and runs it.
func (m *Model) OpenFindResultsInWindow(arg any) {
	query := ""
	switch value := arg.(type) {
	case string:
		query = value
	case map[string]any:
		query, _ = value["query"].(string)
	}
	if strings.TrimSpace(query) == "" {
		query = m.activeSelection()
	}
	if strings.TrimSpace(query) == "" {
		query = m.search.query
	}
	if strings.TrimSpace(query) == "" {
		m.status = "Select text or type a query before opening results in Find"
		m.invalidate()
		return
	}
	m.search.query = query
	m.search.replaceMode = false
	m.search.historyOpen = false
	m.ShowSearch()
	m.RunSearch()
}

// TogglePowerSave changes OpenIDEA's global general.powerSaveMode and mirrors
// it locally so the native editor can stop its expensive surfaces immediately.
func (m *Model) TogglePowerSave() {
	on := !m.prefs.General.PowerSaveMode
	if m.mgr == nil && m.invokeHook == nil {
		m.setPowerSaveMode(on)
		return
	}
	raw, err := json.Marshal(on)
	if err != nil {
		m.paritySettingsError("Cannot change Power Save Mode", err)
		return
	}
	m.status = "Changing Power Save Mode…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "settings_set", map[string]any{
			"scope": "global", "key": "general.powerSaveMode", "value": string(raw),
		})
		m.post(func() {
			if err != nil {
				m.paritySettingsError("Cannot change Power Save Mode", err)
				return
			}
			m.setPowerSaveMode(on)
		})
	}()
}

func (m *Model) setPowerSaveMode(on bool) {
	m.prefs.General.PowerSaveMode = on
	if on {
		m.documentation.popup = nil
		m.usagePopup = nil
	}
	for _, t := range m.allTabs() {
		if t == nil || t.state == nil {
			continue
		}
		t := t
		if on {
			t.state.OnCompletion = nil
			t.state.OnHover = nil
			m.clearInlineCompletionState(t)
		} else {
			if t.state.OnCompletion == nil {
				t.state.OnCompletion = func(req editor.CompletionRequest) {
					m.requestCompletion(t, req)
				}
			}
			if t.state.OnHover == nil {
				t.state.OnHover = func(req editor.HoverRequest) {
					m.requestHover(t, req)
				}
			}
		}
	}
	m.invalidateCodeVision()
	m.savePreferences()
	if on {
		m.status = "Power Save Mode on: completion, documentation and usage highlights are off"
	} else {
		m.status = "Power Save Mode off"
	}
	m.invalidate()
}

// PowerSaveMode is shared by the editor surfaces that can be stopped directly.
func (m *Model) PowerSaveMode() bool {
	return m != nil && m.prefs.General.PowerSaveMode
}
