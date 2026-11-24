package app

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type keymapSmokeStroke struct {
	mods ui.Modifiers
	key  ui.Key
}

// SmokeKeymaps exercises the six pinned presets through their real settings
// and shortcut paths. It restores the window's keymap, tabs, project and
// settings state before returning.
func (m *Model) SmokeKeymaps() map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		wasTrustAsk, wasTrustFolder := m.trustAsk, m.trustFolder
		wasPendingTrust := m.run.pendingTrust
		wasPendingCoverage := m.run.pendingCoverage
		wasPendingContext := m.run.pendingContext
		wasPendingContextCoverage := m.run.pendingContextCoverage
		wasPendingContextProfile := m.run.pendingContextProfile
		wasPendingDebugContext := m.run.pendingDebugContext
		wasProfilerPending := m.profiler.pending
		if m.trustAsk {
			m.clearTrustPrompt()
		}
		var startOverlays []string
		for _, overlay := range m.openOverlays() {
			startOverlays = append(startOverlays, overlay.ID)
		}
		out["debugOverlaysAtStart"] = startOverlays
		wasPrefs := m.prefs
		wasTabs := m.tabs
		wasActive := m.active
		wasSplit := m.split
		m.rootMu.RLock()
		wasRoot := m.root
		m.rootMu.RUnlock()
		wasZen := m.zen
		wasStatus := m.status
		wasSettingsOpen := m.settingsOpen
		wasSettingsPage := m.settingsPage
		wasDraft := m.settingsDraft
		defer func() {
			m.prefs = wasPrefs
			m.tabs = wasTabs
			m.active = wasActive
			m.split = wasSplit
			m.rootMu.Lock()
			m.root = wasRoot
			m.rootMu.Unlock()
			m.zen = wasZen
			m.status = wasStatus
			m.settingsOpen = wasSettingsOpen
			m.settingsPage = wasSettingsPage
			m.settingsDraft = wasDraft
			m.trustAsk, m.trustFolder = wasTrustAsk, wasTrustFolder
			m.run.pendingTrust = wasPendingTrust
			m.run.pendingCoverage = wasPendingCoverage
			m.run.pendingContext = wasPendingContext
			m.run.pendingContextCoverage = wasPendingContextCoverage
			m.run.pendingContextProfile = wasPendingContextProfile
			m.run.pendingDebugContext = wasPendingDebugContext
			m.profiler.pending = wasProfilerPending
			m.applyKeymapPreferences()
			m.savePreferences()
		}()

		ids := make([]string, 0, len(keymapPresetOrder))
		titles := make([]string, 0, len(keymapPresetOrder))
		for _, preset := range keymapPresetOrder {
			ids = append(ids, preset.ID)
			titles = append(titles, preset.Title)
		}
		out["ids"] = ids
		out["titles"] = titles
		out["default"] = keymapPresetDefinitionsByOS()

		settingsPath := m.settingsWritePath()
		setAll := true
		persisted := true
		for _, preset := range keymapPresetOrder {
			m.SetKeymapPresetPref(preset.ID)
			if m.keymapPresetID() != preset.ID {
				setAll = false
			}
			if settingsPath == "" || LoadPreferences(settingsPath).KeymapPreset != preset.ID {
				persisted = false
			}
		}
		out["setAll"] = setAll
		out["persisted"] = persisted

		eclipseMac := keymapPresetsByID["eclipse-macos"]
		intellijMac := keymapPresetsByID["intellij-macos"]
		out["eclipseInherited"] = reflect.DeepEqual(
			eclipseMac.CanonicalBindings["file.save"],
			intellijMac.CanonicalBindings["file.save"],
		)
		out["eclipseRemoved"] = len(eclipseMac.CanonicalBindings["code.duplicateLine"]) == 0 &&
			len(eclipseMac.CanonicalBindings["vcs.commit"]) == 0

		vscodeWindows := keymapPresetsByID["vscode-windows"]
		out["vscodeConverted"] = reflect.DeepEqual(
			vscodeWindows.CanonicalBindings["code.addCaretNextOccurrence"],
			[]string{"Ctrl+D"},
		)
		mixed := false
		for _, bindings := range vscodeWindows.CanonicalBindings {
			for _, binding := range bindings {
				for _, stroke := range strings.Fields(binding) {
					parts := strings.Split(stroke, "+")
					if slicesContain(parts, "Cmd") && slicesContain(parts, "Ctrl") {
						mixed = true
					}
				}
			}
		}
		out["noMixedModifiers"] = !mixed

		m.rootMu.Lock()
		m.root = "/goidea-keymap-smoke"
		m.rootMu.Unlock()
		dispatch := func(preset, text string, start, end int, strokes ...keymapSmokeStroke) *Tab {
			tab := &Tab{
				state:         &editor.State{Buffer: editor.NewBuffer(text)},
				editorFocused: true,
			}
			tab.state.Select(start, end)
			m.prefs.KeymapPreset = preset
			m.tabs = []*Tab{tab}
			m.active = 0
			m.split = nil
			m.applyKeymapPreferences()
			tester := ui.NewTester(func(c *ui.Context) {
				m.shortcuts(c)
				editor.View(c, tab.state).Fill().AutoFocus()
			}, 900, 420)
			tester.Frame()
			for i, stroke := range strokes {
				tester.Key(stroke.mods, stroke.key)
				tester.Frame()
				if preset == "vscode-macos" && len(strokes) == 2 {
					out["debugChordPendingAfterStroke"+strconv.Itoa(i+1)] = append([]string(nil), m.keymapDispatch.pending...)
					out["debugChordZenAfterStroke"+strconv.Itoa(i+1)] = m.zen
				}
			}
			tester.Frame()
			return tab
		}

		out["editorDelete"] = map[string]string{
			"intellijMac": dispatch(
				"intellij-macos", "a\nb\nc\n", 2, 2,
				keymapSmokeStroke{ui.Cmd, ui.KeyBackspace},
			).state.Buffer.String(),
			"intellijWindows": dispatch(
				"intellij-windows", "a\nb\nc\n", 2, 2,
				keymapSmokeStroke{ui.Ctrl, ui.KeyY},
			).state.Buffer.String(),
			"eclipseMac": dispatch(
				"eclipse-macos", "a\nb\nc\n", 2, 2,
				keymapSmokeStroke{ui.Cmd, ui.KeyD},
			).state.Buffer.String(),
		}
		out["duplicateLine"] = dispatch(
			"intellij-macos", "a\nb\nc\n", 2, 2,
			keymapSmokeStroke{ui.Cmd, ui.KeyD},
		).state.Buffer.String()
		selectNext := dispatch(
			"vscode-macos", "foo foo\n", 0, 3,
			keymapSmokeStroke{ui.Cmd, ui.KeyD},
		)
		out["selectNextOccurrence"] = len(selectNext.state.Selections()) == 2

		column := dispatch(
			"intellij-macos", "abc\n", 0, 0,
			keymapSmokeStroke{ui.Cmd | ui.Shift, ui.Key8},
		)
		out["columnSelection"] = column.columnMode
		out["debugColumnActionEnabled"] = m.ActionEnabled("editor.toggleColumnSelection")
		out["debugColumnOverlays"] = func() []string {
			var ids []string
			for _, overlay := range m.openOverlays() {
				ids = append(ids, overlay.ID)
			}
			return ids
		}()
		out["debugChordZenBefore"] = m.zen
		out["debugChordBindings"] = keymapBindingsIn("vscode-macos", m.prefs.Keymap, "editor.toggleZen")
		dispatch(
			"vscode-macos", "abc\n", 0, 0,
			keymapSmokeStroke{ui.Cmd, ui.KeyK},
			keymapSmokeStroke{0, ui.KeyZ},
		)
		out["chord"] = m.zen
		out["debugChordPending"] = m.keymapDispatch.pending
		out["chordParses"] = func() bool {
			strokes, ok := keymapInputStrokes("Cmd+K Cmd+I")
			return ok && len(strokes) == 2
		}()

		nativeMenus := map[string]string{}
		m.prefs.KeymapPreset = "intellij-macos"
		nativeMenus["intellijMac"] = menuAccelerator(m, menuEntry{command: "file.save"})
		m.prefs.KeymapPreset = "intellij-windows"
		nativeMenus["intellijWindows"] = menuAccelerator(m, menuEntry{command: "file.save"})
		out["nativeMenus"] = nativeMenus

		var gear []string
		for _, entry := range gearGroup(m) {
			if entry.label == "Keymap" {
				gear = menuEntryLabelsForSmoke(entry.sub)
				break
			}
		}
		out["gear"] = reflect.DeepEqual(gear, titles)
		var quick []string
		for _, entry := range quickSwitchMenuEntries(m) {
			if entry.label == "Keymap" {
				quick = menuEntryLabelsForSmoke(entry.sub)
				break
			}
		}
		out["quickSwitch"] = reflect.DeepEqual(quick, titles)

		m.OpenSettingsPage(settingsPageKeymap)
		settingsTester := ui.NewTester(func(c *ui.Context) {
			m.settingsDialog(c, c.Theme())
		}, 1200, 900)
		settingsTester.Frame()
		settingsVisible := settingsTester.HasText("Keymap:") &&
			settingsTester.HasText("Restore Default Keymap")
		selected := keymapPresetForID(m.settingsDraft.keymapPreset).Title
		settingsChoices := []string(nil)
		settingsClickError := ""
		if err := settingsTester.Click(selected); err == nil {
			texts := settingsTester.Texts()
			if len(texts) >= len(titles) {
				settingsChoices = texts[len(texts)-len(titles):]
			}
		} else {
			settingsClickError = err.Error()
		}
		out["debugSettingsClickError"] = settingsClickError
		out["debugSettingsPage"] = m.settingsPage
		out["debugSettingsVisible"] = settingsVisible
		out["debugSettingsTexts"] = settingsTester.Texts()
		out["debugSettingsChoices"] = settingsChoices
		out["settings"] = settingsVisible && reflect.DeepEqual(settingsChoices, titles)
	})
	return out
}

func menuEntryLabelsForSmoke(entries []menuEntry) []string {
	labels := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.sep {
			labels = append(labels, entry.label)
		}
	}
	return labels
}
