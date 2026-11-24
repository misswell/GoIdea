package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	goideaPreferencesStorage = "goidea.preferences"
	goideaTemplatesStorage   = "goidea.templates"
)

// paritySettingsWrite is one global-layer setting import. Value is JSON text,
// matching settings_set's contract.
type paritySettingsWrite struct {
	Key   string
	Value string
	Label string
	Shown string
}

type paritySettingsPlan struct {
	Source  string
	Writes  []paritySettingsWrite
	Rows    []string
	Skipped []string
	Invalid []string
}

type parityDialogChoice struct {
	Label string
	Path  string
}

type parityDialogState struct {
	Kind    string
	Title   string
	Body    string
	Source  string
	Plan    *paritySettingsPlan
	Choices []parityDialogChoice
}

// ExportSettings opens the native save picker, or writes directly when an
// automation supplies a path. The bundle keeps backend settings together with
// GoIDEA's preferences and templates, as OpenIDEA's Manage IDE Settings does.
func (m *Model) ExportSettings(arg any) {
	path, _ := arg.(string)
	if strings.TrimSpace(path) == "" {
		path = filepath.Join(m.goideaDataDir(), "goidea-settings.json")
		saved, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      m.win,
			Title:       "Export Settings",
			DefaultPath: path,
			Filters: []mygo.FileFilter{{
				Name:       "GoIDEA settings",
				Extensions: []string{"json"},
			}},
		})
		if err != nil {
			m.paritySettingsError("Cannot export settings", err)
			return
		}
		path = saved
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	m.exportSettingsTo(path)
}

func (m *Model) exportSettingsTo(path string) {
	storage, err := m.localSettingsStorage()
	if err != nil {
		m.paritySettingsError("Cannot export settings", err)
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		bundle := map[string]any{
			"format":   "openidea-settings",
			"version":  1,
			"settings": "",
			"storage":  storage,
		}
		raw, marshalErr := json.MarshalIndent(bundle, "", "  ")
		if marshalErr == nil {
			marshalErr = writeFileAtomic(path, append(raw, '\n'))
		}
		if marshalErr != nil {
			m.paritySettingsError("Cannot export settings", marshalErr)
			return
		}
		m.status = "Settings exported to " + path
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "settings_export", map[string]any{
			"path": path, "storage": storage,
		})
		m.post(func() {
			if err != nil {
				m.paritySettingsError("Cannot export settings", err)
				return
			}
			m.status = "Settings exported to " + path
			m.invalidate()
		})
	}()
}

func (m *Model) localSettingsStorage() (map[string]string, error) {
	prefs, err := json.Marshal(m.prefs)
	if err != nil {
		return nil, err
	}
	storage := map[string]string{goideaPreferencesStorage: string(prefs)}
	if raw, readErr := os.ReadFile(m.templateRestorePath()); readErr == nil {
		storage[goideaTemplatesStorage] = string(raw)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	return storage, nil
}

// ImportSettings imports a JSON settings bundle, while a .zip enters the
// JetBrains summary path exactly as OpenIDEA does.
func (m *Model) ImportSettings(arg any) {
	path, _ := arg.(string)
	if strings.TrimSpace(path) == "" {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: m.win,
			Title:  "Import Settings",
			Filters: []mygo.FileFilter{{
				Name:       "GoIDEA or JetBrains settings",
				Extensions: []string{"json", "zip"},
			}},
		})
		if err != nil {
			m.paritySettingsError("Cannot import settings", err)
			return
		}
		if len(paths) > 0 {
			path = paths[0]
		}
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		m.loadJetBrainsSettings(path)
		return
	}
	m.importSettingsPath(path)
}

func (m *Model) importSettingsPath(path string) {
	if m.mgr == nil && m.invokeHook == nil {
		if err := m.importLocalSettingsBundle(path); err != nil {
			m.paritySettingsError("Cannot import settings", err)
			return
		}
		m.status = "Settings imported from " + path
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "settings_import", map[string]any{"path": path})
		var result struct {
			Storage map[string]string `json:"storage"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		m.post(func() {
			if err == nil {
				err = m.applyImportedStorage(result.Storage)
			}
			if err != nil {
				m.paritySettingsError("Cannot import settings", err)
				return
			}
			m.applyPreferences()
			m.status = "Settings imported from " + path + "; restart to apply every integration"
			m.invalidate()
		})
	}()
}

func (m *Model) importLocalSettingsBundle(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var bundle struct {
		Format  string            `json:"format"`
		Version int               `json:"version"`
		Storage map[string]string `json:"storage"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return err
	}
	if bundle.Format != "openidea-settings" || bundle.Version > 1 {
		return errors.New("not a GoIDEA settings file, or one from a newer version")
	}
	if err := m.applyImportedStorage(bundle.Storage); err != nil {
		return err
	}
	m.applyPreferences()
	return nil
}

func (m *Model) applyImportedStorage(storage map[string]string) error {
	if raw := strings.TrimSpace(storage[goideaPreferencesStorage]); raw != "" {
		var imported Preferences
		if err := json.Unmarshal([]byte(raw), &imported); err != nil {
			return fmt.Errorf("GoIDEA preferences: %w", err)
		}
		m.prefs = imported
		m.savePreferences()
	}
	if raw := strings.TrimSpace(storage[goideaTemplatesStorage]); raw != "" {
		var prefs TemplatePrefs
		if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
			return fmt.Errorf("templates: %w", err)
		}
		if err := m.saveTemplates(prefs); err != nil {
			return err
		}
	}
	return nil
}

// ImportJetBrainsSettings lists the installed JetBrains settings directories,
// or imports a supplied folder/archive path directly.
func (m *Model) ImportJetBrainsSettings(arg any) {
	if path, ok := arg.(string); ok && strings.TrimSpace(path) != "" {
		m.loadJetBrainsSettings(path)
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.chooseJetBrainsSettingsFolder()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "settings_list_jetbrains_ides", nil)
		var choices []struct {
			IDE  string `json:"ide"`
			Path string `json:"path"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &choices)
		}
		m.post(func() {
			if err != nil || len(choices) == 0 {
				m.chooseJetBrainsSettingsFolder()
				return
			}
			if len(choices) == 1 {
				m.loadJetBrainsSettings(choices[0].Path)
				return
			}
			dialog := m.openParityDialog()
			dialog.parity.Kind = "jetbrains-ides"
			dialog.parity.Title = "Import Settings from JetBrains IDE"
			dialog.parity.Body = "Choose the IDE whose editor settings you want to import."
			for _, choice := range choices {
				dialog.parity.Choices = append(dialog.parity.Choices, parityDialogChoice{
					Label: choice.IDE, Path: choice.Path,
				})
			}
			m.invalidate()
		})
	}()
}

func (m *Model) chooseJetBrainsSettingsFolder() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:    m.win,
		Title:     "Import Settings from JetBrains IDE",
		Directory: true,
		Message:   "Choose an exported settings.zip or a JetBrains IDE settings folder.",
	})
	if err != nil {
		m.paritySettingsError("Cannot import settings", err)
		return
	}
	if len(paths) > 0 {
		m.loadJetBrainsSettings(paths[0])
	}
}

func (m *Model) loadJetBrainsSettings(path string) {
	if m.mgr == nil && m.invokeHook == nil {
		m.paritySettingsError("Cannot import settings", errors.New("the GoIDEA backend is not connected"))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "settings_read_jetbrains", map[string]any{"path": path})
		var bundle struct {
			Entries []string          `json:"entries"`
			Files   map[string]string `json:"files"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &bundle)
		}
		plan := planJetBrainsSettings(path, bundle.Entries, bundle.Files)
		m.post(func() {
			if err != nil {
				m.paritySettingsError("Cannot import settings", err)
				return
			}
			if len(plan.Writes) == 0 {
				m.status = "No JetBrains settings that GoIDEA can import were found"
				m.invalidate()
				return
			}
			dialog := m.openParityDialog()
			dialog.parity.Kind = "jetbrains-import"
			dialog.parity.Title = "Import JetBrains Settings"
			dialog.parity.Body = "These settings will be imported:"
			dialog.parity.Source = path
			dialog.parity.Plan = &plan
			m.invalidate()
		})
	}()
}

func (m *Model) applyJetBrainsSettings(dialog *workspaceDialogState) {
	if dialog == nil || dialog.parity.Plan == nil || dialog.busy {
		return
	}
	plan := *dialog.parity.Plan
	dialog.busy = true
	dialog.error = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		for _, write := range plan.Writes {
			if _, err := m.invoke(ctx, "settings_set", map[string]any{
				"scope": "global", "key": write.Key, "value": write.Value,
			}); err != nil {
				m.post(func() {
					if m.workspaceDialog != dialog {
						return
					}
					dialog.busy = false
					dialog.error = "Cannot save " + write.Key + ": " + err.Error()
					m.invalidate()
				})
				return
			}
		}
		m.post(func() {
			if m.workspaceDialog != dialog {
				return
			}
			dialog.busy = false
			m.applyJetBrainsLocalSettings(plan.Writes)
			m.workspaceDialog = nil
			m.applyPreferences()
			m.status = "JetBrains settings imported from " + plan.Source
			m.invalidate()
		})
	}()
}

func (m *Model) applyJetBrainsLocalSettings(writes []paritySettingsWrite) {
	changed := false
	for _, write := range writes {
		var value any
		if json.Unmarshal([]byte(write.Value), &value) != nil {
			continue
		}
		switch write.Key {
		case "editor.fontFamily":
			if text, ok := value.(string); ok {
				m.prefs.Font.Family = text
				changed = true
			}
		case "editor.fontSize":
			if number, ok := value.(float64); ok && number > 0 {
				m.prefs.FontSize = float32(number)
				changed = true
			}
		case "editor.lineHeight":
			if number, ok := value.(float64); ok && number > 0 {
				m.prefs.Font.LineHeight = float32(number)
				changed = true
			}
		case "editor.ligatures":
			if on, ok := value.(bool); ok {
				m.prefs.Font.Ligatures = on
				changed = true
			}
		case "files.ensureFinalNewline":
			if on, ok := value.(bool); ok {
				m.prefs.SaveFiles.EnsureFinalNewline = on
				changed = true
			}
		case "files.trimTrailingWhitespace":
			if on, ok := value.(bool); ok {
				m.prefs.SaveFiles.TrimTrailing = on
				changed = true
			}
		case "editor.general.stripTrailingScope":
			if scope, ok := value.(string); ok {
				m.prefs.SaveFiles.TrimAllLines = scope == "all"
				changed = true
			}
		case "editor.showWhitespace":
			if on, ok := value.(bool); ok {
				m.prefs.Editor.ShowWhitespace = on
				changed = true
			}
		case "editor.softWrap":
			if on, ok := value.(bool); ok {
				m.prefs.Editor.SoftWrap = on
				changed = true
			}
		case "general.confirmExit":
			if on, ok := value.(bool); ok {
				m.prefs.System.ConfirmExit = boolRef(on)
				changed = true
			}
		}
	}
	if changed {
		m.savePreferences()
	}
}

func (m *Model) paritySettingsError(title string, err error) {
	m.status = title + ": " + err.Error()
	m.banner = m.status
	m.bannerPath = ""
	m.invalidate()
}

// planJetBrainsSettings ports the browser-side mapping in
// features/app/jetbrainsSettings.tsx. Only options the native settings model
// can apply are listed; unsupported categories stay visible as skipped.
func planJetBrainsSettings(path string, entries []string, files map[string]string) paritySettingsPlan {
	plan := paritySettingsPlan{Source: path}
	add := func(key string, value any, label, shown string) {
		raw, err := json.Marshal(value)
		if err != nil {
			return
		}
		plan.Writes = append(plan.Writes, paritySettingsWrite{
			Key: key, Value: string(raw), Label: label, Shown: shown,
		})
		plan.Rows = append(plan.Rows, label+": "+shown)
	}
	onOff := func(on bool) string {
		if on {
			return "On"
		}
		return "Off"
	}
	number := func(value string) (float64, bool) {
		parsed, err := strconv.ParseFloat(value, 64)
		return parsed, err == nil && parsed > 0
	}

	font := jetBrainsOptions(files["options/editor-font.xml"], "DefaultFont")
	if family := strings.TrimSpace(font["FONT_FAMILY"]); family != "" {
		add("editor.fontFamily", family, "Font", family)
	}
	size, sizeOK := number(font["FONT_SIZE"])
	if sizeOK {
		add("editor.fontSize", size, "Size", strconv.FormatFloat(size, 'f', -1, 64))
	}
	if spacing, ok := number(font["LINE_SPACING"]); ok && sizeOK {
		lineHeight := int(size*spacing + 0.5)
		add("editor.lineHeight", lineHeight, "Line height", strconv.Itoa(lineHeight))
	}
	if value, ok := font["USE_LIGATURES"]; ok {
		on := value == "true"
		add("editor.ligatures", on, "Enable ligatures", onOff(on))
	}

	editor := jetBrainsOptions(files["options/editor.xml"], "EditorSettings")
	switch editor["STRIP_TRAILING_SPACES"] {
	case "Changed", "Whole":
		add("files.trimTrailingWhitespace", true, "Remove trailing spaces", "On")
		scope := "modified"
		shown := "Modified lines"
		if editor["STRIP_TRAILING_SPACES"] == "Whole" {
			scope, shown = "all", "All lines"
		}
		add("editor.general.stripTrailingScope", scope, "Remove trailing spaces on", shown)
	case "None":
		add("files.trimTrailingWhitespace", false, "Remove trailing spaces", "Off")
	}
	if value, ok := editor["IS_ENSURE_NEWLINE_AT_EOF"]; ok {
		on := value == "true"
		add("files.ensureFinalNewline", on, "Ensure final line break", onOff(on))
	}
	if value, ok := editor["IS_WHITESPACES_SHOWN"]; ok {
		on := value == "true"
		add("editor.showWhitespace", on, "Show whitespaces", onOff(on))
	}
	if value, ok := editor["USE_SOFT_WRAPS"]; ok {
		on := strings.Contains(value, "MAIN_EDITOR")
		add("editor.softWrap", on, "Soft-wrap files", onOff(on))
	}

	general := jetBrainsOptions(files["options/ide.general.xml"], "GeneralSettings")
	if value, ok := general["confirmExit"]; ok {
		on := value == "true"
		add("general.confirmExit", on, "Confirm before exiting the IDE", onOff(on))
	}

	has := func(prefix string) bool {
		for _, entry := range entries {
			if strings.HasPrefix(entry, prefix) {
				return true
			}
		}
		return false
	}
	if has("keymaps/") {
		plan.Skipped = append(plan.Skipped, "Keymap")
	}
	if has("codestyles/") {
		plan.Skipped = append(plan.Skipped, "Code Style")
	}
	if has("templates/") {
		plan.Skipped = append(plan.Skipped, "Live templates")
	}
	if has("inspection/") {
		plan.Skipped = append(plan.Skipped, "Inspections")
	}
	if has("installed.txt") {
		plan.Skipped = append(plan.Skipped, "Plugins")
	}
	if has("colors/") {
		plan.Skipped = append(plan.Skipped, "Color schemes")
	}
	return plan
}

type jetBrainsXMLOption struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type jetBrainsXMLComponent struct {
	Name    string               `xml:"name,attr"`
	Options []jetBrainsXMLOption `xml:"option"`
}

func jetBrainsOptions(raw, component string) map[string]string {
	out := map[string]string{}
	var root jetBrainsXMLComponent
	if xml.Unmarshal([]byte(raw), &root) != nil || root.Name != component {
		return out
	}
	for _, option := range root.Options {
		if option.Name != "" {
			out[option.Name] = option.Value
		}
	}
	return out
}

func (m *Model) openParityDialog() *workspaceDialogState {
	dialog := &workspaceDialogState{open: true}
	m.workspaceDialog = dialog
	return dialog
}

func (m *Model) drawParityWorkspaceDialog(c *ui.Context, p *ui.Theme, state *workspaceDialogState) {
	oi := oiPaletteOf(p)
	parity := &state.parity
	ui.Column(c).Width(600).MaxHeight(620).Padding(18, 20, 16, 20).Gap(10).
		Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
		ui.Text(c, parity.Title).FontSize(14).Bold().TextColor(oi.textStrong).SingleLine()
		switch parity.Kind {
		case "project-rename":
			ui.Text(c, "New project name:").FontSize(12).TextColor(oi.textMuted).SingleLine()
			input := ui.TextInput(c, &state.name).AutoFocus()
			input.OnShortcut(0, ui.KeyEnter, func() { m.submitProjectRename(state) })
			ui.Text(c, parity.Body).FontSize(11).TextColor(oi.textSubtle)
			if state.error != "" {
				ui.Text(c, state.error).FontSize(11).TextColor(oi.danger)
			}
			ui.Spacer(c)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").Disabled(state.busy).OnClick(func() {
					if !state.busy {
						m.workspaceDialog = nil
						m.invalidate()
					}
				})
				ui.PrimaryButton(c, "Rename").Disabled(state.busy).OnClick(func() {
					m.submitProjectRename(state)
				})
			})
		case "jetbrains-ides":
			ui.Text(c, parity.Body).FontSize(12).TextColor(oi.textMuted)
			for _, choice := range parity.Choices {
				choice := choice
				row := ui.Row(c.Key("settings:jetbrains:"+choice.Path)).Height(36).
					Padding(0, 8).AlignItems(ui.Center)
				if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				row.Children(func() {
					ui.Text(c, choice.Label).FontSize(12).TextColor(oi.textStrong).
						SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					m.workspaceDialog = nil
					m.loadJetBrainsSettings(choice.Path)
				}
			}
			ui.Spacer(c)
			ui.Row(c).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					m.workspaceDialog = nil
					m.invalidate()
				})
			})
		case "jetbrains-import":
			ui.Text(c, parity.Body).FontSize(12).TextColor(oi.textMuted)
			ui.Scroll(c).Grow(1).MinHeight(100).MaxHeight(360).Children(func() {
				ui.Column(c).Gap(3).Children(func() {
					if parity.Plan != nil {
						for _, row := range parity.Plan.Rows {
							ui.Text(c, row).FontSize(12).TextColor(oi.text).MaxWidth(540)
						}
						if len(parity.Plan.Skipped) > 0 {
							ui.Text(c, "Not imported: "+strings.Join(parity.Plan.Skipped, ", ")).
								FontSize(11).TextColor(oi.textSubtle).MaxWidth(540)
						}
					}
				})
			})
			if state.error != "" {
				ui.Text(c, state.error).FontSize(11).TextColor(oi.danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").Disabled(state.busy).OnClick(func() {
					if !state.busy {
						m.workspaceDialog = nil
						m.invalidate()
					}
				})
				ui.PrimaryButton(c, "Import").Disabled(state.busy).OnClick(func() {
					m.applyJetBrainsSettings(state)
				})
			})
		}
	})
}
