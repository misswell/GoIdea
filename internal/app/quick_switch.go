package app

import "github.com/egoist/mygo"

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
	return []menuEntry{
		submenu("Editor Color Scheme",
			menuEntry{
				label: "Theme default",
				check: func(*Model) bool { return true },
				run: func(mm *Model) {
					mm.status = "Editor color scheme: Theme default"
					mm.invalidate()
				},
			},
		),
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
