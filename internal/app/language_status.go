package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const languageStatusRowHeight float32 = 30

var phpLanguageLevels = []string{
	"",
	"8.5",
	"8.4",
	"8.3",
	"8.2",
	"8.1",
	"8.0",
	"7.4",
	"7.3",
	"7.2",
	"7.1",
	"7.0",
	"5.6",
}

var pythonProjectMarkers = []string{
	"pyproject.toml",
	"setup.py",
	"setup.cfg",
	"requirements.txt",
	"Pipfile",
	"pyrightconfig.json",
	"ty.toml",
	"uv.lock",
	"poetry.lock",
	"SConstruct",
	"SConscript",
	".python-version",
}

type pythonRuntimeStatus struct {
	root      string
	near      string
	key       string
	gen       uint64
	checking  bool
	checked   bool
	hasFiles  bool
	filesErr  string
	active    bool
	loading   bool
	loaded    bool
	effective *string
	sdks      []openidea.Sdk
	sdkErr    string
}

type languageStatusPopup struct {
	kind   string
	active int
	focus  ui.Handle
}

type languageStatusState struct {
	python   pythonRuntimeStatus
	popup    languageStatusPopup
	writeGen uint64
}

type languageStatusChoice struct {
	kind     string
	label    string
	detail   string
	value    string
	checked  bool
	disabled bool
}

func init() {
	for _, action := range []*Action{
		{
			ID: "python.interpreterPopup", Title: "Python Interpreter",
			Category: "Settings", Keywords: "python interpreter sdk",
			When: func(m *Model) bool { return m.Root() != "" },
			Run:  func(m *Model) { m.openLanguageStatusPopup("python") },
		},
		{
			ID: "python.selectInterpreter", Title: "Select Python Interpreter",
			Category: "Settings", Description: "Select the interpreter for the current project",
			RunWithArg: func(m *Model, arg any) {
				path, _ := arg.(string)
				m.selectPythonInterpreter(path)
			},
		},
		{
			ID: "python.addInterpreter", Title: "Add New Interpreter…",
			Category: "Settings", Keywords: "python interpreter sdk create venv",
			When: func(m *Model) bool { return m.Root() != "" },
			Run:  func(m *Model) { m.OpenSettingsPage(settingsPagePythonInterpret) },
		},
		{
			ID: "python.interpreterSettings", Title: "Interpreter Settings…",
			Category: "Settings", Keywords: "python interpreter sdk settings",
			When: func(m *Model) bool { return m.Root() != "" },
			Run:  func(m *Model) { m.OpenSettingsPage(settingsPagePythonInterpret) },
		},
		{
			ID: "php.languageLevelPopup", Title: "PHP Language Level",
			Category: "Settings", Keywords: "php version language level",
			When: func(m *Model) bool { return m.Root() != "" },
			Run:  func(m *Model) { m.openLanguageStatusPopup("php") },
		},
		{
			ID: "php.selectLanguageLevel", Title: "Select PHP Language Level",
			Category: "Settings", Description: "Select the PHP language level for the current project",
			RunWithArg: func(m *Model, arg any) {
				level, _ := arg.(string)
				m.selectPHPLanguageLevel(level)
			},
		},
	} {
		_ = registerAction(action)
	}

	registerStatusItem(&StatusItem{
		ID: "python.interpreter", Title: "Python Interpreter", Side: "right", Order: 90,
		Visible: func(m *Model) bool { return m.pythonInterpreterWidgetVisible() },
		Render:  func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) { m.renderPythonInterpreterStatus(c, p, oi) },
	})

	registerOverlay(&Overlay{
		ID: "languages.statusPopup", Layer: overlayLayerPopup, Priority: 91,
		Open: func(m *Model) bool { return m.languageSDKs.status.popup.kind != "" },
		Render: func(m *Model, c *ui.Context, p *ui.Theme) {
			m.renderLanguageStatusPopup(c, p)
		},
	})
}

func languageTabPath(m *Model, language string) string {
	if m == nil || m.tab() == nil {
		return ""
	}
	tab := m.tab()
	if tab.doc == nil {
		return ""
	}
	path := tab.doc.Path()
	tabLanguage := tab.doc.Info().Language
	if language := m.fileTypeLanguageFor(path); language != nil && language.Name != "" {
		tabLanguage = language.Name
	}
	if !strings.EqualFold(tabLanguage, language) {
		return ""
	}
	return path
}

func pythonStatusNearFile(m *Model) string {
	active := languageTabPath(m, "python")
	if active == "" {
		return ""
	}
	active = filepath.ToSlash(active)
	dir := filepath.ToSlash(filepath.Dir(active))
	var roots []string
	if m.workspace != nil {
		roots = append(roots, m.workspace.Projects...)
	}
	if root := m.Root(); root != "" {
		roots = append(roots, root)
	}
	sort.SliceStable(roots, func(i, j int) bool {
		return len(filepath.ToSlash(roots[i])) > len(filepath.ToSlash(roots[j]))
	})
	for _, root := range roots {
		root = strings.TrimSuffix(filepath.ToSlash(root), "/")
		if dir == root || strings.HasPrefix(dir, root+"/") {
			return dir
		}
	}
	return ""
}

func (m *Model) pythonInterpreterWidgetVisible() bool {
	if m.Root() == "" || (m.run.loaded && !m.run.trusted) {
		return false
	}
	state := m.ensurePythonRuntimeStatus()
	if state.root != m.Root() {
		return false
	}
	return state.active || state.hasFiles || state.effective != nil ||
		strings.TrimSpace(nativeSettingsString(m, settingsPagePythonInterpret, "languages.python.interpreter")) != ""
}

func (m *Model) ensurePythonRuntimeStatus() pythonRuntimeStatus {
	root := m.Root()
	if root == "" {
		return pythonRuntimeStatus{}
	}
	active := languageTabPath(m, "python")
	near := pythonStatusNearFile(m)
	configured := strings.TrimSpace(nativeSettingsString(
		m, settingsPagePythonInterpret, "languages.python.interpreter",
	))
	key := strings.Join([]string{languageSDKPathKey(root), languageSDKPathKey(near), configured}, "\x00")
	state := m.languageSDKs.status.python
	if state.key != key {
		gen := state.gen + 1
		state = pythonRuntimeStatus{
			root: root, near: near, key: key, gen: gen,
			active: active != "", hasFiles: active != "",
		}
		m.languageSDKs.status.python = state
	}
	state.active = active != ""
	state.near = near
	m.languageSDKs.status.python = state

	if !m.nativeSettingsLoaded(settingsPagePythonInterpret) {
		m.loadNativeSettingsPage(settingsPagePythonInterpret)
	}

	needDiscovery := state.effective != nil || state.loaded || configured != "" ||
		active != "" || state.hasFiles
	if !needDiscovery && !state.checked && !state.checking && m.canRunLanguageStatusRPC() {
		m.probePythonProject(state)
		state = m.languageSDKs.status.python
	}
	if (state.hasFiles || active != "" || configured != "") &&
		!state.loaded && !state.loading && m.canRunLanguageStatusRPC() {
		m.loadPythonRuntimeStatus(state)
		state = m.languageSDKs.status.python
	}
	return state
}

func (m *Model) nativeSettingsLoaded(page string) bool {
	state, ok := m.nativeSettings.pages[page]
	return ok && (state.loaded || state.loading)
}

func (m *Model) canRunLanguageStatusRPC() bool {
	return m.invokeHook != nil || m.mgr != nil && m.Phase() == "ready"
}

func (m *Model) probePythonProject(state pythonRuntimeStatus) {
	if state.root == "" || state.checking {
		return
	}
	state.checking = true
	m.languageSDKs.status.python = state
	root, key, generation := state.root, state.key, state.gen
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		queries := append([]string{".py", "requirements"}, pythonProjectMarkers...)
		var found bool
		var firstErr error
		for _, query := range queries {
			raw, err := m.invoke(ctx, "find_files", map[string]any{
				"query": query, "limit": 1,
			})
			if err != nil {
				if firstErr == nil && openidea.ErrorKind(err) != "cancelled" {
					firstErr = err
				}
				continue
			}
			var files []openidea.FileMatch
			if err := json.Unmarshal(raw, &files); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if len(files) > 0 {
				found = true
				break
			}
		}
		m.post(func() {
			current := m.languageSDKs.status.python
			if current.root != root || current.key != key || current.gen != generation || m.Root() != root {
				return
			}
			current.checking = false
			current.checked = true
			current.hasFiles = found
			if firstErr != nil && !found {
				current.filesErr = firstErr.Error()
			}
			m.languageSDKs.status.python = current
			m.invalidate()
		})
	}()
}

func (m *Model) loadPythonRuntimeStatus(state pythonRuntimeStatus) {
	if state.root == "" || state.loading {
		return
	}
	state.loading = true
	state.sdkErr = ""
	m.languageSDKs.status.python = state
	root, near, generation := state.root, state.near, state.gen
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		var effective *string
		var effectiveErr error
		args := map[string]any{}
		if near != "" {
			args["near"] = near
		}
		raw, effectiveErr := m.invoke(ctx, "sdk_python_effective", args)
		if effectiveErr == nil {
			effectiveErr = json.Unmarshal(raw, &effective)
		}
		raw, sdkErr := m.invoke(ctx, "sdk_detect", map[string]any{"kind": openidea.SdkPython})
		var sdks []openidea.Sdk
		if sdkErr == nil {
			sdkErr = json.Unmarshal(raw, &sdks)
		}
		m.post(func() {
			current := m.languageSDKs.status.python
			if current.root != root || current.gen != generation || m.Root() != root {
				return
			}
			current.loading = false
			current.loaded = effectiveErr == nil
			if effectiveErr == nil {
				current.effective = effective
			}
			if sdkErr == nil {
				current.sdks = append([]openidea.Sdk(nil), sdks...)
				m.rememberLanguageSDKs(sdks)
			} else {
				current.sdkErr = sdkErr.Error()
			}
			m.languageSDKs.status.python = current
			m.invalidate()
		})
	}()
}

func pythonStatusLabel(state pythonRuntimeStatus, configured string) string {
	if configured != "" && state.effective == nil && state.loaded {
		return "Invalid interpreter"
	}
	if state.effective == nil || strings.TrimSpace(*state.effective) == "" {
		return "No interpreter"
	}
	for _, sdk := range state.sdks {
		if samePythonInterpreterPath(sdk.Path, *state.effective, state.root) {
			if strings.TrimSpace(sdk.Name) != "" {
				return sdk.Name
			}
			break
		}
	}
	return filepath.Base(*state.effective)
}

func (m *Model) renderPythonInterpreterStatus(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
) {
	state := m.ensurePythonRuntimeStatus()
	if state.root != m.Root() {
		return
	}
	configured := strings.TrimSpace(nativeSettingsString(
		m, settingsPagePythonInterpret, "languages.python.interpreter",
	))
	label := pythonStatusLabel(state, configured)
	invalid := configured != "" && state.loaded && state.effective == nil
	tooltip := "Python Interpreter"
	if state.effective != nil {
		tooltip = *state.effective
	} else if invalid {
		tooltip = "The configured interpreter does not exist"
	}
	tk := oiTokensFor(m, p)
	button := ui.ButtonBase(c.Key("status:python:interpreter")).
		Height(tk.StatusBarHeight-tk.SpaceL).Padding(0, tk.SpaceM).
		Radius(tk.RadiusS).Gap(tk.SpaceS).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Tooltip(tooltip)
	button.Children(func() {
		if invalid || state.effective == nil {
			oiIconEl(c, "status/warning", 13, oi, oi.warning)
		}
		ui.Text(c, label).FontSize(tk.FontSizeM).TextColor(oi.textMuted).SingleLine()
	})
	button.Menu(func(menu *ui.Menu) { m.pythonInterpreterMenu(menu) })
}

func (m *Model) pythonInterpreterMenu(menu *ui.Menu) {
	state := m.ensurePythonRuntimeStatus()
	choices := m.pythonInterpreterChoices(state)
	names := map[string]int{}
	for _, choice := range choices {
		names[choice.label]++
	}
	for _, choice := range choices {
		label := choice.label
		if names[label] > 1 && choice.detail != "" {
			label += " — " + choice.detail
		}
		item := menu.Item(label)
		item.Checked(choice.checked)
		if item.Chosen() {
			m.selectPythonInterpreter(choice.value)
		}
	}
	menu.Separator()
	if menu.Item("Add New Interpreter…").Chosen() {
		m.OpenSettingsPage(settingsPagePythonInterpret)
	}
	if menu.Item("Interpreter Settings…").Chosen() {
		m.OpenSettingsPage(settingsPagePythonInterpret)
	}
}

func (m *Model) pythonInterpreterChoices(state pythonRuntimeStatus) []languageStatusChoice {
	root := m.Root()
	current := ""
	if state.effective != nil {
		current = *state.effective
	}
	if current == "" {
		current = strings.TrimSpace(nativeSettingsString(
			m, settingsPagePythonInterpret, "languages.python.interpreter",
		))
	}
	hidden := map[string]bool{}
	for _, path := range m.pythonInterpreterHidden() {
		hidden[pythonInterpreterPathKey(path, root)] = true
	}
	seen := map[string]bool{}
	var choices []languageStatusChoice
	add := func(sdk openidea.Sdk) {
		if strings.TrimSpace(sdk.Path) == "" {
			return
		}
		key := pythonInterpreterPathKey(sdk.Path, root)
		if seen[key] || hidden[key] {
			return
		}
		seen[key] = true
		label := strings.TrimSpace(sdk.Name)
		if label == "" {
			label = filepath.Base(sdk.Path)
		}
		choices = append(choices, languageStatusChoice{
			kind: "python", label: label, detail: sdk.Path,
			value: sdk.Path, checked: samePythonInterpreterPath(sdk.Path, current, root),
		})
	}
	for _, sdk := range state.sdks {
		add(sdk)
	}
	for _, sdk := range m.languageSDKList(openidea.SdkPython) {
		add(sdk)
	}
	for _, path := range m.pythonInterpreterAdded() {
		add(openidea.Sdk{Path: path, Name: path, Source: "custom"})
	}
	if current != "" && !seen[pythonInterpreterPathKey(current, root)] {
		add(openidea.Sdk{Path: current, Name: current, Source: "custom"})
	}
	sort.SliceStable(choices, func(i, j int) bool {
		return strings.ToLower(choices[i].label) < strings.ToLower(choices[j].label)
	})
	return choices
}

func (m *Model) selectPythonInterpreter(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || m.Root() == "" {
		return false
	}
	m.setLanguageStatusProjectSetting(
		settingsPagePythonInterpret,
		"languages.python.interpreter",
		path,
		"python-status",
		"Cannot select Python interpreter",
	)
	return true
}

func (m *Model) setLanguageStatusProjectSetting(
	page, key string,
	value any,
	notificationKey, failureTitle string,
) {
	if m.Root() == "" || key == "" {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		m.notifyError(notificationKey, failureTitle, err.Error())
		return
	}
	previous := nativeSettingsValue(m, page, key)
	m.languageSDKs.status.writeGen++
	generation := m.languageSDKs.status.writeGen
	root := m.Root()
	m.setNativeSettingsValue(page, key, value)
	if m.invokeHook == nil && m.mgr == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Second)
		defer cancel()
		_, invokeErr := m.invoke(ctx, "settings_set", map[string]any{
			"scope": "project", "key": key, "value": string(raw),
		})
		m.post(func() {
			if m.Root() != root {
				return
			}
			if invokeErr != nil {
				current := m.languageSDKs.status.writeGen
				if current == generation &&
					languageStatusValueEqual(nativeSettingsValue(m, page, key), value) {
					m.setNativeSettingsValue(page, key, previous)
				}
				m.notifyError(notificationKey, failureTitle, invokeErr.Error())
				return
			}
			state := m.nativeSettings.pages[page]
			if state.applied == nil {
				state.applied = map[string]any{}
			}
			state.applied[key] = cloneLanguageStatusValue(value)
			m.nativeSettings.pages[page] = state
			m.invalidate()
		})
	}()
}

func cloneLanguageStatusValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, entry := range typed {
			out[key] = entry
		}
		return out
	default:
		return value
	}
}

func languageStatusValueEqual(a, b any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func (m *Model) selectPHPLanguageLevel(level string) bool {
	valid := false
	for _, candidate := range phpLanguageLevels {
		if candidate == level {
			valid = true
			break
		}
	}
	if !valid || m.Root() == "" {
		return false
	}
	status := m.languageSDKs.php
	if status.levels.Member != nil && strings.TrimSpace(*status.levels.Member) != "" {
		return false
	}
	m.setLanguageStatusProjectSetting(
		settingsPagePHP,
		"languages.php.languageLevel",
		level,
		"php-language-level",
		"Cannot select PHP language level",
	)
	m.ensurePHPRuntimeStatus(phpStatusNearPath(m), true)
	return true
}

func (m *Model) phpLanguageLevelMenu(menu *ui.Menu) {
	status := m.languageSDKs.php
	chosen := ""
	if status.levels.Effective != nil && status.levels.Effective.Source == "settings" {
		chosen = phpShortVersion(status.levels.Effective)
	}
	member := status.levels.Member != nil && strings.TrimSpace(*status.levels.Member) != ""
	if member {
		menu.Item("Set in the settings of the linked project " +
			filepath.Base(*status.levels.Member)).Disabled(true)
		menu.Separator()
	}
	for _, level := range phpLanguageLevels {
		label := level
		if level == "" {
			label = "Auto"
			if auto := phpShortVersion(status.levels.Auto); auto != "" && !member {
				label += " (" + auto + ")"
			}
		}
		item := menu.Item(label).Checked(level == chosen).Disabled(member)
		if item.Chosen() {
			m.selectPHPLanguageLevel(level)
		}
	}
}

func (m *Model) phpStatusWidgetVisible() bool {
	return m.Root() != "" && languageTabPath(m, "php") != ""
}

func (m *Model) openLanguageStatusPopup(kind string) {
	root := m.Root()
	if root == "" {
		return
	}
	switch kind {
	case "python":
		if !m.pythonInterpreterWidgetVisible() {
			m.OpenSettingsPage(settingsPagePythonInterpret)
			return
		}
	case "php":
		if !m.phpStatusWidgetVisible() {
			m.OpenSettingsPage(settingsPagePHP)
			return
		}
	default:
		return
	}
	state := m.languageSDKs.status.popup
	if state.kind == kind {
		m.languageSDKs.status.popup = languageStatusPopup{}
		m.invalidate()
		return
	}
	rows := m.languageStatusPopupRows(kind)
	active := 0
	for i, row := range rows {
		if row.checked && !row.disabled {
			active = i
			break
		}
	}
	m.languageSDKs.status.popup = languageStatusPopup{
		kind: kind, active: active, focus: state.focus,
	}
	m.invalidate()
}

func (m *Model) languageStatusPopupRows(kind string) []languageStatusChoice {
	switch kind {
	case "python":
		state := m.ensurePythonRuntimeStatus()
		rows := m.pythonInterpreterChoices(state)
		rows = append(rows,
			languageStatusChoice{kind: "addPython", label: "Add New Interpreter…"},
			languageStatusChoice{kind: "openPython", label: "Interpreter Settings…"},
		)
		return rows
	case "php":
		status := m.languageSDKs.php
		chosen := ""
		if status.levels.Effective != nil && status.levels.Effective.Source == "settings" {
			chosen = phpShortVersion(status.levels.Effective)
		}
		member := status.levels.Member != nil && strings.TrimSpace(*status.levels.Member) != ""
		rows := make([]languageStatusChoice, 0, len(phpLanguageLevels))
		for _, level := range phpLanguageLevels {
			label := level
			detail := ""
			if level == "" {
				label = "Auto"
				detail = phpShortVersion(status.levels.Auto)
			}
			rows = append(rows, languageStatusChoice{
				kind: "php", label: label, detail: detail, value: level,
				checked: level == chosen, disabled: member,
			})
		}
		return rows
	default:
		return nil
	}
}

func (m *Model) stepLanguageStatusPopup(delta int) {
	popup := m.languageSDKs.status.popup
	rows := m.languageStatusPopupRows(popup.kind)
	if len(rows) == 0 {
		return
	}
	popup.active = (popup.active + delta + len(rows)) % len(rows)
	m.languageSDKs.status.popup = popup
	m.invalidate()
}

func (m *Model) chooseLanguageStatusPopup() {
	popup := m.languageSDKs.status.popup
	rows := m.languageStatusPopupRows(popup.kind)
	if popup.active < 0 || popup.active >= len(rows) {
		return
	}
	m.chooseLanguageStatusRow(popup, rows[popup.active])
}

func (m *Model) chooseLanguageStatusRow(popup languageStatusPopup, row languageStatusChoice) {
	if m.languageSDKs.status.popup.kind != popup.kind || row.disabled {
		return
	}
	switch row.kind {
	case "python":
		if !m.selectPythonInterpreter(row.value) {
			return
		}
	case "php":
		if !m.selectPHPLanguageLevel(row.value) {
			return
		}
	case "addPython", "openPython":
		m.OpenSettingsPage(settingsPagePythonInterpret)
	default:
		return
	}
	m.languageSDKs.status.popup = languageStatusPopup{}
	m.invalidate()
}

func (m *Model) renderLanguageStatusPopup(c *ui.Context, p *ui.Theme) {
	popup := &m.languageSDKs.status.popup
	if popup.kind == "" {
		return
	}
	root := m.Root()
	if root == "" ||
		popup.kind == "python" && !m.pythonInterpreterWidgetVisible() ||
		popup.kind == "php" && !m.phpStatusWidgetVisible() {
		*popup = languageStatusPopup{}
		return
	}
	rows := m.languageStatusPopupRows(popup.kind)
	if len(rows) == 0 {
		*popup = languageStatusPopup{}
		return
	}
	oi := oiPaletteOf(p)
	width, height := c.Size()
	w := float32(330)
	rowCount := len(rows)
	member := popup.kind == "php" && m.languageSDKs.php.levels.Member != nil &&
		strings.TrimSpace(*m.languageSDKs.php.levels.Member) != ""
	if member {
		rowCount++
	}
	h := min(float32(44+rowCount*int(languageStatusRowHeight)+30), height-16)
	x := max(float32(8), width-w-12)
	y := max(float32(8), height-oiStatusBarHeight-h-6)
	title := "PHP Language Level"
	if popup.kind == "python" {
		title = "Python Interpreter"
	}
	ui.Overlay(c, func() {
		box := ui.Column(c.Key("language-status:popup")).Role(ui.RolePopup).
			Label(title).Focusable().AutoFocus().Bind(&popup.focus).
			Absolute().Left(x).Top(y).Width(w).Height(h).MinWidth(0).Clip().
			Background(oi.popup).Border(1, oi.borderStrong).Radius(7).
			Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.3))
		box.Children(func() {
			ui.Row(c).Height(38).Shrink(0).Padding(0, 12).AlignItems(ui.Center).
				Children(func() {
					ui.Text(c, title).FontSize(12).Bold().
						TextColor(oi.textStrong).SingleLine()
				})
			ui.Row(c).Height(1).Shrink(0).Background(oi.border)
			if member {
				ui.Row(c).Height(languageStatusRowHeight).Shrink(0).Padding(0, 12).
					AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Set in the settings of the linked project "+
						filepath.Base(*m.languageSDKs.php.levels.Member)).
						FontSize(11).TextColor(oi.textMuted).SingleLine()
				})
			}
			ui.Scroll(c.Key("language-status:rows")).Grow(1).MinHeight(0).
				MinWidth(0).Children(func() {
				ui.Column(c).Padding(4, 0).Children(func() {
					for i, row := range rows {
						m.renderLanguageStatusRow(c, oi, popup, i, row)
					}
				})
			})
			ui.Row(c).Height(30).Shrink(0).Padding(0, 10).
				AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).
				BorderColor(oi.border).Children(func() {
				ui.Text(c, "↑/↓ select · Enter apply · Esc close").
					FontSize(10).TextColor(oi.textMuted).SingleLine()
			})
		})
		switch {
		case box.PressedOutside() || box.OverlayShortcut(0, ui.KeyEscape):
			m.languageSDKs.status.popup = languageStatusPopup{}
			m.invalidate()
		case box.OverlayShortcut(0, ui.KeyUp):
			m.stepLanguageStatusPopup(-1)
		case box.OverlayShortcut(0, ui.KeyDown):
			m.stepLanguageStatusPopup(1)
		case box.OverlayShortcut(0, ui.KeyEnter):
			m.chooseLanguageStatusPopup()
		}
	})
}

func (m *Model) renderLanguageStatusRow(
	c *ui.Context,
	oi oiColors,
	popup *languageStatusPopup,
	index int,
	row languageStatusChoice,
) {
	button := ui.ButtonBase(c.Key(fmt.Sprintf("language-status:%s:%d", popup.kind, index))).
		Height(languageStatusRowHeight).Padding(0, 9).Radius(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if index == popup.active {
		button.Background(oi.selected)
	} else if button.Hovered() {
		button.Background(oi.toolbarHover)
	}
	button.Disabled(row.disabled)
	button.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			if row.checked {
				oiIconEl(c, "status/check", 13, oi, oi.accent)
			} else {
				ui.Box(c).Width(13).Height(13)
			}
			ui.Text(c, row.label).Grow(1).FontSize(12).
				TextColor(oi.textStrong).SingleLine().Ellipsis("…")
			if row.detail != "" {
				ui.Text(c, row.detail).FontSize(10).
					TextColor(oi.textMuted).SingleLine().Ellipsis("…")
			}
		})
	})
	button.OnClick(func() {
		popup.active = index
		m.chooseLanguageStatusRow(*popup, row)
	})
}
