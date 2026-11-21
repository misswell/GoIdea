package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// JavaSettings are Settings › Languages & Frameworks › Java as the OpenIDEA
// backend stores them. Project settings win when a project is open; otherwise
// the page edits the global scope.
type JavaSettings struct {
	JDK           string `json:"jdk"`
	LanguageLevel string `json:"languageLevel"`
	JDTLSJdk      string `json:"jdtlsJdk"`
	JDTLSShared   bool   `json:"jdtlsShared"`
	JDTLSVmArgs   string `json:"jdtlsVmArgs"`
}

// javaSettingsState keeps the backend snapshot and its staged copy for the
// settings dialog. Loading and saving are asynchronous so dialogs never block
// the UI thread on the backend.
type javaSettingsState struct {
	loading bool
	loaded  bool
	saving  bool
	scope   string
	applied JavaSettings
	draft   JavaSettings
	sdks    []openidea.Sdk
	err     string
	loadGen uint64
	saveGen uint64
}

var javaLanguageLevels = []string{
	"5", "6", "7", "8", "9", "10", "11", "12", "13",
	"14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25",
}

// LoadJavaSettings reads the current project scope, or the global scope when
// no project is open, and lists installed JDKs for the pickers.
func (m *Model) LoadJavaSettings() {
	state := &m.javaSettings
	scope := "global"
	if m.Root() != "" {
		scope = "project"
	}
	state.loading = true
	state.loaded = false
	state.err = ""
	state.scope = scope
	state.loadGen++
	generation := state.loadGen
	root := m.Root()
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.loaded = true
		state.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		command := "settings_get"
		if scope == "global" {
			command = "settings_get_global"
		}
		raw, err := m.invoke(ctx, command, nil)
		var settings JavaSettings
		if err == nil {
			var envelope struct {
				Languages struct {
					Java JavaSettings `json:"java"`
				} `json:"languages"`
			}
			err = json.Unmarshal(raw, &envelope)
			settings = envelope.Languages.Java
		}
		var sdks []openidea.Sdk
		if err == nil && root != "" {
			if rawSDKs, sdkErr := m.invoke(ctx, "sdk_detect", map[string]any{"kind": "jdk"}); sdkErr == nil {
				_ = json.Unmarshal(rawSDKs, &sdks)
			}
		}
		m.post(func() {
			if state.loadGen != generation || state.scope != scope ||
				(scope == "project" && m.Root() != root) {
				return
			}
			state.loading = false
			state.loaded = err == nil
			if err != nil {
				state.err = "Could not read Java settings: " + err.Error()
				m.invalidate()
				return
			}
			state.applied = settings
			state.draft = settings
			state.sdks = sdks
			m.invalidate()
		})
	}()
}

func modelContext(m *Model) context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func sameJavaSettings(a, b JavaSettings) bool {
	return a.JDK == b.JDK &&
		a.LanguageLevel == b.LanguageLevel &&
		a.JDTLSJdk == b.JDTLSJdk &&
		a.JDTLSShared == b.JDTLSShared &&
		a.JDTLSVmArgs == b.JDTLSVmArgs
}

type javaSettingWrite struct {
	key   string
	value string
}

// pendingJavaSettingWrites lists only changed fields. String values are sent
// as their JSON spelling, matching settings_set's value contract.
func pendingJavaSettingWrites(applied, draft JavaSettings) []javaSettingWrite {
	var writes []javaSettingWrite
	addString := func(key, before, after string) {
		if before == after {
			return
		}
		raw, _ := json.Marshal(after)
		writes = append(writes, javaSettingWrite{key: key, value: string(raw)})
	}
	addString("languages.java.jdk", applied.JDK, draft.JDK)
	addString("languages.java.languageLevel", applied.LanguageLevel, draft.LanguageLevel)
	if applied.JDTLSShared != draft.JDTLSShared {
		raw, _ := json.Marshal(draft.JDTLSShared)
		writes = append(writes, javaSettingWrite{key: "languages.java.jdtlsShared", value: string(raw)})
	}
	addString("languages.java.jdtlsJdk", applied.JDTLSJdk, draft.JDTLSJdk)
	addString("languages.java.jdtlsVmArgs", applied.JDTLSVmArgs, draft.JDTLSVmArgs)
	return writes
}

// saveJavaSettingsThenLanguageServers saves changed Java fields first, then
// hands control to the existing language-server saver. Chaining prevents an
// OK that changes both pages from closing the dialog before Java is written.
func (m *Model) saveJavaSettingsThenLanguageServers(closeAfter bool) {
	state := &m.javaSettings
	if state.saving {
		return
	}
	if !state.loaded {
		if closeAfter {
			m.saveLanguageServerSettings(closeAfter)
		} else {
			m.saveLanguageServerSettings(false)
		}
		return
	}
	writes := pendingJavaSettingWrites(state.applied, state.draft)
	if len(writes) == 0 {
		m.saveLanguageServerSettings(closeAfter)
		return
	}
	state.saving = true
	state.err = ""
	state.saveGen++
	generation := state.saveGen
	scope := state.scope
	m.settingsErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		for _, write := range writes {
			if _, err := m.invoke(ctx, "settings_set", map[string]any{
				"scope": scope,
				"key":   write.key,
				"value": write.value,
			}); err != nil {
				m.post(func() {
					if state.saveGen != generation {
						return
					}
					state.saving = false
					state.err = "Could not save " + write.key + ": " + err.Error()
					m.settingsErr = state.err
					m.invalidate()
				})
				return
			}
		}
		m.post(func() {
			if state.saveGen != generation {
				return
			}
			state.saving = false
			state.applied = state.draft
			state.err = ""
			m.settingsErr = ""
			m.status = "Java settings updated"
			m.LoadJavaTools()
			m.saveLanguageServerSettings(closeAfter)
		})
	}()
}

// settingsJavaBody draws the same Project JDK and Language Server choices as
// OpenIDEA's Java page, followed by the installed-tool diagnostics.
func (m *Model) settingsJavaBody(c *ui.Context, p *ui.Theme, oi oiColors) {
	state := &m.javaSettings
	scope := "Project"
	if state.scope == "global" || (state.scope == "" && m.Root() == "") {
		scope = "Global"
	}
	if state.loading {
		ui.Text(c, "Loading Java settings…").FontSize(12).
			TextColor(oi.textMuted).SingleLine()
	} else if state.err != "" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, state.err).FontSize(12).TextColor(oi.danger).
				SingleLine().Ellipsis("…").Grow(1)
			ui.Button(c, "Retry").OnClick(m.LoadJavaSettings)
		})
	}

	settingsSectionTitle(c, oi, scope+" JDK")
	m.javaJdkRow(c, oi, "SDK", &state.draft.JDK, "Auto")
	ui.Text(c, "Compiles and runs the project, and also runs jdtls when its version is sufficient.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Language level", 220)
		level := state.draft.LanguageLevel
		shown := "SDK default"
		if level != "" {
			shown = level
		}
		choices := append([]string{"SDK default"}, javaLanguageLevels...)
		if ui.Select(c, &shown, choices).Width(180).Changed() {
			if shown == "SDK default" {
				state.draft.LanguageLevel = ""
			} else {
				state.draft.LanguageLevel = shown
			}
			m.invalidate()
		}
	})
	ui.Text(c, "Java source and bytecode compliance when no build descriptor overrides it.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")

	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Language Server")
	shared := state.draft.JDTLSShared
	if ui.Checkbox(c, &shared, "Share JDT LS across builds").Changed() {
		state.draft.JDTLSShared = shared
		m.invalidate()
	}
	ui.Text(c, "Uses one JDT LS process for linked builds in this project window.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	m.javaJdkRow(c, oi, "JDK", &state.draft.JDTLSJdk, "Auto")
	ui.Text(c, "Runs only jdtls. Auto uses a suitable installed JDK or a managed runtime.").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "JVM options", 220)
		ui.TextInput(c, &state.draft.JDTLSVmArgs).Grow(1).
			Placeholder("-Xmx4g")
	})
	ui.Text(c, "Passed to the JVM running jdtls (java.jdt.ls.vmargs).").
		FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")

	ui.Row(c).Height(12)
	m.javaToolsSection(c, p)
}

// javaJdkRow is the native JDK picker: installed SDKs plus a path field for a
// JDK detection did not find and a folder button for explicit selection.
func (m *Model) javaJdkRow(c *ui.Context, oi oiColors, label string, value *string, emptyLabel string) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label, 220)
		choices, labels := m.javaJDKChoices(*value, emptyLabel)
		shown := labels[0]
		for i, choice := range choices {
			if choice == *value {
				shown = labels[i]
				break
			}
		}
		if ui.Select(c, &shown, labels).Grow(1).Changed() {
			for i, choiceLabel := range labels {
				if shown == choiceLabel {
					*value = choices[i]
					m.invalidate()
					break
				}
			}
		}
		ui.Button(c, "Browse…").OnClick(func() {
			m.browseJavaJDK(value)
		})
	})
	ui.Row(c).MinHeight(28).PaddingStart(228).Children(func() {
		ui.TextInput(c, value).Grow(1).Placeholder(filepath.Join("path", "to", "jdk"))
	})
}

func (m *Model) javaJDKChoices(current, emptyLabel string) ([]string, []string) {
	choices := []string{""}
	labels := []string{emptyLabel}
	seen := map[string]bool{"": true}
	for _, sdk := range m.javaSettings.sdks {
		path := strings.TrimSpace(sdk.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		choices = append(choices, path)
		labels = append(labels, javaSDKLabel(sdk))
	}
	current = strings.TrimSpace(current)
	if current != "" && !seen[current] {
		choices = append(choices, current)
		labels = append(labels, "Current · "+current)
	}
	return choices, labels
}

func javaSDKLabel(sdk openidea.Sdk) string {
	name := strings.TrimSpace(sdk.Name)
	if name == "" {
		name = filepath.Base(strings.TrimSpace(sdk.Path))
	}
	version := ""
	if sdk.Version != nil {
		version = strings.TrimSpace(*sdk.Version)
	}
	if version != "" && version != name {
		name += " (" + version + ")"
	}
	return name + " · " + strings.TrimSpace(sdk.Path)
}

// browseJavaJDK chooses a JDK home without blocking the main thread.
func (m *Model) browseJavaJDK(target *string) {
	parent := m.win
	current := strings.TrimSpace(*target)
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       "Select JDK Home Directory",
			ButtonLabel: "Choose",
			DefaultPath: current,
			Directory:   true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			*target = filepath.Clean(paths[0])
			m.invalidate()
		})
	}()
}

// javaSettingsDirty reports whether the staged Java page differs from the
// last backend snapshot.
func (m *Model) javaSettingsDirty() bool {
	state := &m.javaSettings
	return state.loaded && !sameJavaSettings(state.applied, state.draft)
}

// resetJavaSettingsDraft restores the stage to defaults, matching Reset on the
// other Settings pages. Applying it clears the backend's Java values.
func (m *Model) resetJavaSettingsDraft() {
	m.javaSettings.draft = JavaSettings{}
}

// resetJavaSettingsSession drops the backend snapshot on settings Cancel.
func (m *Model) resetJavaSettingsSession() {
	generation := m.javaSettings.loadGen
	saveGeneration := m.javaSettings.saveGen
	m.javaSettings = javaSettingsState{
		loadGen: generation,
		saveGen: saveGeneration,
	}
}
