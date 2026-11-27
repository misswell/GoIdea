package app

import (
	"sort"
	"strings"

	"github.com/egoist/mygo"
)

// QuickSwitchScheme is View › Quick Switch Scheme: the color scheme, keymap,
// view mode and theme pickers OpenIDEA shows in one native context menu.
func (m *Model) QuickSwitchScheme() {
	if m.win == nil {
		return
	}
	menu := mygo.NewMenu(menuItems(m, quickSwitchMenuEntries(m)))
	menu.PopupAt(m.win, 240, 42)
}

func quickSwitchMenuEntries(m *Model) []menuEntry {
	styleEntries := []menuEntry{
		actionChecked("Islands Light", "editor.colorScheme.light", func(mm *Model) bool {
			return normalizeColorSchemeID(mm.prefs.EditorScheme.ColorScheme) == "light"
		}),
		actionChecked("Islands Dark", "editor.colorScheme.dark", func(mm *Model) bool {
			return normalizeColorSchemeID(mm.prefs.EditorScheme.ColorScheme) == "dark"
		}),
		actionChecked("High Contrast", "editor.colorScheme.dark-hc", func(mm *Model) bool {
			return normalizeColorSchemeID(mm.prefs.EditorScheme.ColorScheme) == "dark-hc"
		}),
		actionChecked("Solarized Light", "editor.colorScheme.solarized-light", func(mm *Model) bool {
			return normalizeColorSchemeID(mm.prefs.EditorScheme.ColorScheme) == "solarized-light"
		}),
		actionChecked("Solarized Dark", "editor.colorScheme.solarized-dark", func(mm *Model) bool {
			return normalizeColorSchemeID(mm.prefs.EditorScheme.ColorScheme) == "solarized-dark"
		}),
		actionChecked("Theme default", "editor.colorScheme.theme-default", func(mm *Model) bool {
			return normalizeColorSchemeID(mm.prefs.EditorScheme.ColorScheme) == themeDefaultColorScheme
		}),
	}
	customIDs := make([]string, 0, len(m.prefs.EditorScheme.CustomSchemes))
	for id := range m.prefs.EditorScheme.CustomSchemes {
		customIDs = append(customIDs, id)
	}
	sort.Strings(customIDs)
	for _, id := range customIDs {
		id := id
		scheme := m.prefs.EditorScheme.CustomSchemes[id]
		label := strings.TrimSpace(scheme.Name)
		if label == "" {
			label = id
		}
		styleEntries = append(styleEntries, actionChecked(label, "editor.colorScheme."+id, func(mm *Model) bool {
			return normalizeColorSchemeID(mm.prefs.EditorScheme.ColorScheme) == id
		}))
	}
	return []menuEntry{
		submenu("Editor Color Scheme", styleEntries...),
		submenu("Keymap",
			keymapPresetMenuEntries()...,
		),
		submenu("View Mode",
			menuEntry{label: zenTitle(m), command: "editor.toggleZen"},
			action("Toggle Full Screen", "window.toggleFullScreen", "CmdOrCtrl+Ctrl+F"),
		),
		submenu("Theme",
			actionChecked("Light", "app.themeLight", func(mm *Model) bool {
				return mm.prefs.Theme == "light"
			}),
			actionChecked("Dark", "app.themeDark", func(mm *Model) bool {
				return mm.prefs.Theme == "dark"
			}),
			actionChecked("Use System Settings", "app.themeSystem", func(mm *Model) bool {
				return mm.prefs.Theme == "" || mm.prefs.Theme == "system"
			}),
		),
	}
}
