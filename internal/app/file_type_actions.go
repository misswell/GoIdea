package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// registerFileTypeActions mirrors OpenIDEA's hidden File actions. The visible
// entry points are the Project view and editor-tab menus.
func registerFileTypeActions() {
	_ = registerAction(&Action{
		ID:       "file.overrideFileType",
		Title:    "Override File Type",
		Category: "File",
		Builtin:  true,
		RunWithArg: func(m *Model, arg any) {
			path, language := fileTypeOverrideArgument(arg)
			if path != "" && language != "" {
				m.SetFileTypeOverride(path, language)
			}
		},
	})
	_ = registerAction(&Action{
		ID:       "file.revertFileTypeOverride",
		Title:    "Revert File Type Override",
		Category: "File",
		Builtin:  true,
		RunWithArg: func(m *Model, arg any) {
			path, _ := fileTypeOverrideArgument(arg)
			if path != "" {
				m.SetFileTypeOverride(path, "")
			}
		},
	})
}

func fileTypeOverrideArgument(arg any) (path, language string) {
	switch value := arg.(type) {
	case string:
		return value, ""
	case map[string]any:
		path, _ := value["path"].(string)
		language, _ := value["language"].(string)
		return path, language
	case map[string]string:
		return value["path"], value["language"]
	}
	return "", ""
}

// fileTypeOverrideMenu appends OpenIDEA's Override File Type language submenu
// and, when the selected file is overridden, Revert File Type Override.
func (m *Model) fileTypeOverrideMenu(menu *ui.Menu, path string) {
	if path == "" || strings.Contains(path, "://") {
		return
	}
	languages := m.fileTypeMenuLanguages()
	if len(languages) == 0 {
		return
	}
	current := m.currentFileTypeOverride(path)
	menu.Submenu("Override File Type", func(sub *ui.Menu) {
		for _, language := range languages {
			language := language
			if sub.Item(language.Name).Checked(language.ID == current).Chosen() {
				m.SetFileTypeOverride(path, language.ID)
			}
		}
	})
	if current != "" && menu.Item("Revert File Type Override").Chosen() {
		m.SetFileTypeOverride(path, "")
	}
}

func (m *Model) fileTypeMenuLanguages() []openIDEALanguageInfo {
	if len(m.fileTypesSettings.languages) > 0 {
		return append([]openIDEALanguageInfo(nil), m.fileTypesSettings.languages...)
	}
	local := editor.Languages()
	out := make([]openIDEALanguageInfo, 0, len(local))
	for _, language := range local {
		if language == nil || language.Name == "" {
			continue
		}
		out = append(out, openIDEALanguageInfo{
			ID:   mygoLanguageID(language.Name),
			Name: language.Name,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (m *Model) currentFileTypeOverride(path string) string {
	key := fileTypeOverrideKey(path, m.Root())
	if m.fileTypesSettings.runtimeLoaded {
		if id := m.fileTypesSettings.runtime.FileTypeOverrides[key]; id != "" {
			return id
		}
	}
	if m.fileTypesSettings.loaded || m.fileTypesSettings.loading {
		return m.fileTypesSettings.draft.FileTypeOverrides[key]
	}
	return ""
}

// SetFileTypeOverride writes the project override for path, or the global
// override when no project is open. An empty language removes the override.
func (m *Model) SetFileTypeOverride(path, language string) {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "://") {
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.status = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	scope := m.settingsBackendScope()
	root := m.Root()
	key := fileTypeOverrideKey(path, root)
	if key == "" {
		return
	}
	m.status = "Updating file type override…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		overrides, err := m.readFileTypeOverrides(ctx, scope)
		if err == nil {
			if language == "" {
				delete(overrides, key)
			} else {
				overrides[key] = language
			}
			err = m.invokeSettingsSet(ctx, scope, "files.fileTypeOverrides", overrides)
		}
		m.post(func() {
			if scope != m.settingsBackendScope() || scope == "project" && m.Root() != root {
				return
			}
			if err != nil {
				m.status = ""
				m.fail(path, fmt.Errorf("change file type: %w", err))
				return
			}
			state := &m.fileTypesSettings
			state.runtime.FileTypeOverrides = cloneStringMap(overrides)
			state.runtimeLoaded = true
			if !m.fileTypesDirty() {
				state.applied.FileTypeOverrides = cloneStringMap(overrides)
				state.draft.FileTypeOverrides = cloneStringMap(overrides)
			}
			m.applyFileTypeRuntime()
			if language == "" {
				m.status = "File type override reverted"
			} else {
				m.status = "File type overridden"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) readFileTypeOverrides(ctx context.Context, scope string) (map[string]string, error) {
	raw, err := m.invokeSettingsScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	settings, err := decodeFileTypesSettings(raw)
	if err != nil {
		return nil, err
	}
	return cloneStringMap(settings.FileTypeOverrides), nil
}
