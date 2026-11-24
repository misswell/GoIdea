package app

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Macros follow OpenIDEA's Edit › Macros feature. Recording captures the
// actions the user invokes plus text and editing keys typed in a text editor.
// The last recording and every named macro persist in the user's settings.

const macroPlayActionPrefix = "macros.play."

type MacroStep struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
	Arg  any    `json:"arg,omitempty"`
	Text string `json:"text,omitempty"`
	Key  string `json:"key,omitempty"`
}

type Macro struct {
	Name  string      `json:"name"`
	Steps []MacroStep `json:"steps"`
}

type MacroPrefs struct {
	Last  []MacroStep `json:"last,omitempty"`
	Saved []Macro     `json:"saved,omitempty"`
}

type macroState struct {
	loaded          bool
	recording       []MacroStep
	recordingActive bool
	playing         bool
	last            []MacroStep
	saved           []Macro

	editOpen      bool
	editSelection int
	nameOpen      bool
	nameDraft     string
	renameOpen    bool
	renameDraft   string
	renameErr     string
	deleteOpen    bool
	playOpen      bool
	playSelection int
}

func init() {
	registerStatusItem(&StatusItem{
		ID:    "macros.recording",
		Side:  "left",
		Order: 4,
		Visible: func(m *Model) bool {
			m.ensureMacros()
			return m.macros.recordingActive
		},
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			tk := oiTokensFor(m, p)
			button := ui.ButtonBase(c.Key("status:macro-recording")).
				Height(tk.StatusBarHeight-tk.SpaceL).
				Padding(0, tk.SpaceM).Radius(tk.RadiusS).Gap(tk.SpaceS).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if button.Hovered() {
				button.Background(oi.toolbarHover)
			}
			button.Tooltip("Stop Macro Recording")
			button.Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(oi.danger)
				ui.Text(c, "Recording macro…").FontSize(tk.FontSizeM).
					TextColor(oi.textMuted).SingleLine()
				oiIconEl(c, "run/stop_stroke", 14, oi, oi.textMuted)
			})
			if button.Clicked() {
				m.stopMacroRecording()
			}
		},
	})
}

func (m *Model) ensureMacros() {
	if m.macros.loaded {
		return
	}
	m.macros.loaded = true
	m.macros.last = normalizeMacroSteps(m.prefs.Macros.Last)
	m.macros.saved = normalizeSavedMacros(m.prefs.Macros.Saved)
	if len(m.macros.last) == 0 {
		m.macros.last = nil
	}
}

func normalizeSavedMacros(saved []Macro) []Macro {
	out := make([]Macro, 0, len(saved))
	for _, macro := range saved {
		name := strings.TrimSpace(macro.Name)
		if name == "" {
			continue
		}
		out = append(out, Macro{Name: name, Steps: normalizeMacroSteps(macro.Steps)})
	}
	return out
}

func normalizeMacroSteps(steps []MacroStep) []MacroStep {
	out := make([]MacroStep, 0, len(steps))
	for _, step := range steps {
		switch step.Kind {
		case "action":
			if step.ID == "" {
				continue
			}
			if step.Arg != nil {
				arg, ok := storableMacroArg(step.Arg)
				if !ok {
					continue
				}
				step.Arg = arg
			}
		case "text", "key":
			if step.Kind == "text" && step.Text == "" ||
				step.Kind == "key" && step.Key == "" {
				continue
			}
		default:
			continue
		}
		out = append(out, step)
	}
	return out
}

func cloneMacroSteps(steps []MacroStep) []MacroStep {
	return normalizeMacroSteps(steps)
}

func (m *Model) persistMacros() {
	m.prefs.Macros.Last = cloneMacroSteps(m.macros.last)
	m.prefs.Macros.Saved = make([]Macro, len(m.macros.saved))
	for i, macro := range m.macros.saved {
		m.prefs.Macros.Saved[i] = Macro{
			Name:  macro.Name,
			Steps: cloneMacroSteps(macro.Steps),
		}
	}
	m.savePreferences()
}

func (m *Model) startMacroRecording() {
	m.ensureMacros()
	if m.macros.playing {
		return
	}
	m.macros.recording = nil
	m.macros.recordingActive = true
	m.invalidate()
}

func (m *Model) stopMacroRecording() {
	m.ensureMacros()
	if !m.macros.recordingActive {
		return
	}
	steps := cloneMacroSteps(m.macros.recording)
	m.macros.recording = nil
	m.macros.recordingActive = false
	if len(steps) > 0 {
		m.macros.last = cloneMacroSteps(steps)
	}
	m.persistMacros()
	if len(steps) == 0 {
		m.notifyInfo("macro.recording", "Nothing was recorded", "")
		m.invalidate()
		return
	}
	m.macros.nameOpen = true
	m.macros.nameDraft = ""
	m.invalidate()
}

func (m *Model) recordMacroStep(step MacroStep) {
	m.ensureMacros()
	if !m.macros.recordingActive || m.macros.playing {
		return
	}
	normalized := normalizeMacroSteps([]MacroStep{step})
	if len(normalized) == 0 {
		return
	}
	step = normalized[0]
	if step.Kind == "text" && len(m.macros.recording) > 0 {
		last := &m.macros.recording[len(m.macros.recording)-1]
		if last.Kind == "text" {
			last.Text += step.Text
			return
		}
	}
	m.macros.recording = append(m.macros.recording, step)
}

func (m *Model) recordMacroEditorInput(ev ui.InputEvent) {
	m.ensureMacros()
	if !m.macros.recordingActive || m.macros.playing {
		return
	}
	switch ev.Kind {
	case ui.InputText:
		if ev.Text != "" {
			m.recordMacroStep(MacroStep{Kind: "text", Text: ev.Text})
		}
	case ui.InputKeyDown:
		if key, ok := macroEditorKeyName(ev); ok {
			m.recordMacroStep(MacroStep{Kind: "key", Key: key})
		}
	}
}

func (m *Model) recordMacroAction(id string, arg any) {
	m.ensureMacros()
	if !m.macros.recordingActive || m.macros.playing ||
		strings.HasPrefix(id, "macro.") || strings.HasPrefix(id, "macros.") {
		return
	}
	step := MacroStep{Kind: "action", ID: canonicalActionID(id)}
	if !isNilMacroArg(arg) {
		stored, ok := storableMacroArg(arg)
		if !ok {
			return
		}
		step.Arg = stored
	}
	m.recordMacroStep(step)
}

func isNilMacroArg(arg any) bool {
	if arg == nil {
		return true
	}
	value := reflect.ValueOf(arg)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// storableMacroArg mirrors OpenIDEA's storableArg: primitive values, string
// arrays and flat objects of the same values can be saved in a macro.
func storableMacroArg(arg any) (any, bool) {
	if isNilMacroArg(arg) {
		return nil, false
	}
	value := reflect.ValueOf(arg)
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		return value.String(), true
	case reflect.Bool:
		return value.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(value.Uint()), true
	case reflect.Float32, reflect.Float64:
		number := value.Float()
		if math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, false
		}
		return number, true
	case reflect.Slice, reflect.Array:
		items := make([]string, value.Len())
		for i := range value.Len() {
			item := value.Index(i)
			for item.Kind() == reflect.Interface || item.Kind() == reflect.Pointer {
				if item.IsNil() {
					return nil, false
				}
				item = item.Elem()
			}
			if item.Kind() != reflect.String {
				return nil, false
			}
			items[i] = item.String()
		}
		return items, true
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			item, ok := storableMacroObjectValue(iter.Value())
			if !ok {
				return nil, false
			}
			out[iter.Key().String()] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func storableMacroObjectValue(value reflect.Value) (any, bool) {
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		return value.String(), true
	case reflect.Bool:
		return value.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(value.Uint()), true
	case reflect.Float32, reflect.Float64:
		number := value.Float()
		return number, !math.IsInf(number, 0) && !math.IsNaN(number)
	case reflect.Slice, reflect.Array:
		items := make([]string, value.Len())
		for i := range value.Len() {
			item := value.Index(i)
			for item.Kind() == reflect.Interface || item.Kind() == reflect.Pointer {
				if item.IsNil() {
					return nil, false
				}
				item = item.Elem()
			}
			if item.Kind() != reflect.String {
				return nil, false
			}
			items[i] = item.String()
		}
		return items, true
	default:
		return nil, false
	}
}

func macroEditorKeyName(ev ui.InputEvent) (string, bool) {
	key, ok := macroEditorKey(ev.Key)
	if !ok {
		return "", false
	}
	parts := make([]string, 0, 5)
	if ev.Mods&ui.Super != 0 {
		parts = append(parts, "Meta")
	}
	if ev.Mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if ev.Mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if ev.Mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	parts = append(parts, key)
	name := strings.Join(parts, "+")
	if _, ok := macroEditorCommand(name); !ok {
		return "", false
	}
	return name, true
}

func macroEditorKey(key ui.Key) (string, bool) {
	switch key {
	case ui.KeyEnter:
		return "Enter", true
	case ui.KeyBackspace:
		return "Backspace", true
	case ui.KeyDelete:
		return "Delete", true
	case ui.KeyTab:
		return "Tab", true
	case ui.KeyLeft:
		return "ArrowLeft", true
	case ui.KeyRight:
		return "ArrowRight", true
	case ui.KeyUp:
		return "ArrowUp", true
	case ui.KeyDown:
		return "ArrowDown", true
	case ui.KeyHome:
		return "Home", true
	case ui.KeyEnd:
		return "End", true
	case ui.KeyPageUp:
		return "PageUp", true
	case ui.KeyPageDown:
		return "PageDown", true
	default:
		return "", false
	}
}

func macroEditorCommand(key string) (editor.Command, bool) {
	command, ok := macroEditorCommands[key]
	return command, ok
}

var macroEditorCommands = map[string]editor.Command{
	"Enter":                 editor.Newline,
	"Backspace":             editor.Backspace,
	"Delete":                editor.Delete,
	"Alt+Backspace":         editor.DeleteWordLeft,
	"Alt+Delete":            editor.DeleteWordRight,
	"Tab":                   editor.Tab,
	"Shift+Tab":             editor.Backtab,
	"ArrowLeft":             editor.MoveLeft,
	"ArrowRight":            editor.MoveRight,
	"ArrowUp":               editor.MoveUp,
	"ArrowDown":             editor.MoveDown,
	"Shift+ArrowLeft":       editor.SelectLeft,
	"Shift+ArrowRight":      editor.SelectRight,
	"Shift+ArrowUp":         editor.SelectUp,
	"Shift+ArrowDown":       editor.SelectDown,
	"Alt+ArrowLeft":         editor.MoveWordLeft,
	"Alt+ArrowRight":        editor.MoveWordRight,
	"Alt+Shift+ArrowLeft":   editor.SelectWordLeft,
	"Alt+Shift+ArrowRight":  editor.SelectWordRight,
	"Home":                  editor.MoveLineStart,
	"End":                   editor.MoveLineEnd,
	"Shift+Home":            editor.SelectLineStart,
	"Shift+End":             editor.SelectLineEnd,
	"PageUp":                editor.MovePageUp,
	"PageDown":              editor.MovePageDown,
	"Shift+PageUp":          editor.SelectPageUp,
	"Shift+PageDown":        editor.SelectPageDown,
	"Meta+ArrowLeft":        editor.MoveLineStart,
	"Meta+ArrowRight":       editor.MoveLineEnd,
	"Meta+Shift+ArrowLeft":  editor.SelectLineStart,
	"Meta+Shift+ArrowRight": editor.SelectLineEnd,
	"Meta+ArrowUp":          editor.MoveDocStart,
	"Meta+ArrowDown":        editor.MoveDocEnd,
	"Ctrl+Home":             editor.MoveDocStart,
	"Ctrl+End":              editor.MoveDocEnd,
	"Ctrl+Shift+Home":       editor.SelectDocStart,
	"Ctrl+Shift+End":        editor.SelectDocEnd,
	"Ctrl+ArrowLeft":        editor.MoveWordLeft,
	"Ctrl+ArrowRight":       editor.MoveWordRight,
}

func (m *Model) macroActionTitle(id string) string {
	if action, ok := actionGet(id); ok && action.Title != "" {
		return action.Title
	}
	return id
}

func macroDescribeStep(step MacroStep, title func(string) string) string {
	switch step.Kind {
	case "action":
		if step.Arg == nil {
			return title(step.ID)
		}
		return title(step.ID) + " (" + macroDescribeArg(step.Arg) + ")"
	case "text":
		return `Type "` + strings.ReplaceAll(step.Text, "\n", "↵") + `"`
	case "key":
		return "Press " + step.Key
	default:
		return ""
	}
}

func macroDescribeArg(arg any) string {
	switch value := arg.(type) {
	case []string:
		return strings.Join(value, ", ")
	case map[string]any:
		if path, ok := value["path"].(string); ok {
			return path
		}
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		return fmt.Sprint(arg)
	}
	return string(raw)
}

func (m *Model) playMacroSteps(steps []MacroStep, name string) bool {
	m.ensureMacros()
	if m.macros.playing {
		return false
	}
	m.macros.playing = true
	defer func() {
		m.macros.playing = false
		m.invalidate()
	}()
	for _, step := range steps {
		var ok bool
		switch step.Kind {
		case "action":
			ok = m.RunActionWithArg(step.ID, step.Arg)
		case "text":
			tab := m.tab()
			ok = tab != nil && tab.state != nil && !tab.state.ReadOnly
			if ok {
				tab.state.Insert(step.Text)
			}
		case "key":
			command, known := macroEditorCommand(step.Key)
			tab := m.tab()
			ok = known && tab != nil && tab.state != nil && tab.state.Run(command)
		}
		if !ok {
			title := "Macro playback stopped"
			if name != "" {
				title = "Macro '" + name + "' stopped"
			}
			m.notifyWarning("macro.playback", title, "A recorded step could not run here.")
			return false
		}
		m.invalidate()
	}
	return true
}

func (m *Model) playLastMacro() {
	m.ensureMacros()
	if len(m.macros.last) == 0 || m.macros.recordingActive || m.macros.playing {
		return
	}
	m.playMacroSteps(cloneMacroSteps(m.macros.last), "")
}

func (m *Model) playSavedMacro(name string) bool {
	m.ensureMacros()
	if m.macros.recordingActive || m.macros.playing {
		return false
	}
	for _, macro := range m.macros.saved {
		if macro.Name == name {
			return m.playMacroSteps(cloneMacroSteps(macro.Steps), macro.Name)
		}
	}
	return false
}

func (m *Model) openPlaySavedMacros() {
	m.ensureMacros()
	if m.macros.recordingActive || m.macros.playing || len(m.macros.saved) == 0 {
		return
	}
	m.macros.playOpen = true
	m.macros.playSelection = clampIndex(m.macros.playSelection, len(m.macros.saved))
	m.invalidate()
}

func (m *Model) finishMacroNaming(save bool) {
	name := strings.TrimSpace(m.macros.nameDraft)
	m.macros.nameOpen = false
	m.macros.nameDraft = ""
	if save && name != "" {
		m.saveMacro(name, m.macros.last)
		m.status = "Saved macro '" + name + "'"
	}
	m.invalidate()
}

func (m *Model) saveMacro(name string, steps []MacroStep) {
	m.ensureMacros()
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	entry := Macro{Name: name, Steps: cloneMacroSteps(steps)}
	found := false
	for i := range m.macros.saved {
		if m.macros.saved[i].Name == name {
			m.macros.saved[i] = entry
			found = true
			break
		}
	}
	if !found {
		m.macros.saved = append(m.macros.saved, entry)
	}
	m.persistMacros()
	m.syncSavedMacroActions()
}

func (m *Model) openEditMacros() {
	m.ensureMacros()
	if m.macros.recordingActive || len(m.macros.saved) == 0 {
		return
	}
	m.macros.editOpen = true
	m.macros.editSelection = clampIndex(m.macros.editSelection, len(m.macros.saved))
	m.invalidate()
}

func (m *Model) selectedMacro() *Macro {
	m.ensureMacros()
	if len(m.macros.saved) == 0 {
		return nil
	}
	m.macros.editSelection = clampIndex(m.macros.editSelection, len(m.macros.saved))
	return &m.macros.saved[m.macros.editSelection]
}

func (m *Model) openRenameMacro() {
	macro := m.selectedMacro()
	if macro == nil {
		return
	}
	m.macros.renameOpen = true
	m.macros.renameDraft = macro.Name
	m.macros.renameErr = ""
	m.invalidate()
}

func (m *Model) macroRenameError() string {
	macro := m.selectedMacro()
	if macro == nil || !m.macros.renameOpen {
		return ""
	}
	name := strings.TrimSpace(m.macros.renameDraft)
	if name == "" {
		return "Enter a macro name"
	}
	for _, saved := range m.macros.saved {
		if saved.Name == name && saved.Name != macro.Name {
			return "A macro with this name exists"
		}
	}
	return ""
}

func (m *Model) commitRenameMacro() {
	macro := m.selectedMacro()
	if macro == nil || m.macroRenameError() != "" {
		return
	}
	oldName := macro.Name
	newName := strings.TrimSpace(m.macros.renameDraft)
	if newName == oldName {
		m.macros.renameOpen = false
		m.invalidate()
		return
	}
	macro.Name = newName
	m.macros.renameOpen = false
	m.macros.renameDraft = ""
	m.persistMacros()
	m.syncSavedMacroActions()
	m.status = "Renamed macro '" + oldName + "'"
	m.invalidate()
}

func (m *Model) openDeleteMacro() {
	if m.selectedMacro() == nil {
		return
	}
	m.macros.deleteOpen = true
	m.invalidate()
}

func (m *Model) deleteSelectedMacro() {
	macro := m.selectedMacro()
	if macro == nil {
		m.macros.deleteOpen = false
		return
	}
	name := macro.Name
	m.macros.saved = append(m.macros.saved[:m.macros.editSelection], m.macros.saved[m.macros.editSelection+1:]...)
	m.macros.deleteOpen = false
	m.macros.editSelection = clampIndex(m.macros.editSelection, len(m.macros.saved))
	if len(m.macros.saved) == 0 {
		m.macros.editOpen = false
	}
	m.persistMacros()
	m.syncSavedMacroActions()
	m.status = "Deleted macro '" + name + "'"
	m.invalidate()
}

func clampIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	return max(0, min(index, count-1))
}

func (m *Model) syncSavedMacroActions() {
	m.ensureMacros()
	current := make(map[string]bool, len(m.macros.saved))
	for _, macro := range m.macros.saved {
		current[macroPlayActionPrefix+macro.Name] = true
	}
	for _, action := range actionList() {
		if strings.HasPrefix(action.ID, macroPlayActionPrefix) && !current[action.ID] {
			unregisterAction(action.ID)
		}
	}
	for _, macro := range m.macros.saved {
		name := macro.Name
		id := macroPlayActionPrefix + name
		if _, exists := actionGet(id); exists {
			continue
		}
		_ = registerAction(&Action{
			ID:       id,
			Title:    name,
			Category: "Macros",
			Keywords: "macro play",
			When: func(m *Model) bool {
				m.ensureMacros()
				if m.macros.recordingActive || m.macros.playing {
					return false
				}
				for _, saved := range m.macros.saved {
					if saved.Name == name {
						return true
					}
				}
				return false
			},
			Run: func(m *Model) { m.playSavedMacro(name) },
		})
	}
}

func macroActionState(action *Action) {
	if action == nil {
		return
	}
	switch action.ID {
	case "macro.startRecording":
		action.When = func(m *Model) bool {
			m.ensureMacros()
			return !m.macros.recordingActive && !m.macros.playing
		}
	case "macro.stopRecording":
		action.When = func(m *Model) bool {
			m.ensureMacros()
			return m.macros.recordingActive
		}
	case "macro.playBack":
		action.When = func(m *Model) bool {
			m.ensureMacros()
			return !m.macros.recordingActive && !m.macros.playing && len(m.macros.last) > 0
		}
	case "macro.playSaved":
		action.When = func(m *Model) bool {
			m.ensureMacros()
			return !m.macros.recordingActive && !m.macros.playing && len(m.macros.saved) > 0
		}
	case "macro.edit":
		action.When = func(m *Model) bool {
			m.ensureMacros()
			return !m.macros.recordingActive && !m.macros.playing && len(m.macros.saved) > 0
		}
	}
}

func macroMenuEntries(m *Model) []menuEntry {
	m.ensureMacros()
	entries := make([]menuEntry, 0, 6)
	if m.macros.recordingActive {
		entries = append(entries, action("Stop Macro Recording", "macro.stopRecording", ""))
	} else {
		entries = append(entries,
			action("Start Macro Recording", "macro.startRecording", ""),
			rule(),
		)
		if len(m.macros.last) > 0 {
			entries = append(entries, action("Play Back Last Macro", "macro.playBack", ""))
		}
		if len(m.macros.saved) > 0 {
			entries = append(entries,
				action("Play Saved Macros…", "macro.playSaved", ""),
				action("Edit Macros…", "macro.edit", ""),
			)
		}
	}
	return entries
}

func (m *Model) macroEditDialog(c *ui.Context) {
	m.ensureMacros()
	if !m.macros.editOpen {
		return
	}
	p := c.Theme()
	oi := oiPaletteOf(p)
	open := true
	dialog := ui.Modal(c.Key("macros:edit"), &open, func() {
		ui.Column(c).Width(620).Height(360).Gap(12).Children(func() {
			ui.Text(c, "Edit Macros").FontSize(14).Bold().TextColor(oi.textStrong).SingleLine()
			ui.Row(c).Grow(1).MinHeight(0).Gap(12).Children(func() {
				ui.Column(c).Width(220).Shrink(0).MinHeight(0).Gap(8).Children(func() {
					ui.Box(c).Grow(1).MinHeight(0).Border(1, oi.border).Radius(4).Children(func() {
						ui.Scroll(c.Key("macros:list")).Grow(1).Children(func() {
							ui.Column(c).Padding(4).Gap(1).Children(func() {
								if len(m.macros.saved) == 0 {
									ui.Text(c, "No saved macros").FontSize(12).
										TextColor(oi.textMuted).Padding(6, 8)
								}
								for i, macro := range m.macros.saved {
									i, macro := i, macro
									row := ui.Row(c.Key(fmt.Sprintf("macros:item:%d", i))).
										Height(24).Padding(0, 8).AlignItems(ui.Center).
										Cursor(ui.CursorPointer)
									switch {
									case i == m.macros.editSelection:
										row.Background(oi.selected)
									case row.Hovered():
										row.Background(oi.hoverTint)
									}
									row.OnClick(func() {
										m.macros.editSelection = i
										m.invalidate()
									})
									row.Children(func() {
										ui.Text(c, macro.Name).FontSize(12).TextColor(oi.text).
											SingleLine().Ellipsis("…")
									})
								}
							})
						})
					})
					ui.Row(c).Gap(6).Children(func() {
						rename := ui.Button(c, "Rename…")
						rename.Disabled(m.selectedMacro() == nil)
						rename.OnClick(m.openRenameMacro)
						remove := ui.Button(c, "Delete")
						remove.Disabled(m.selectedMacro() == nil)
						remove.OnClick(m.openDeleteMacro)
					})
				})
				ui.Box(c).Grow(1).MinWidth(0).MinHeight(0).
					Border(1, oi.border).Radius(4).Children(func() {
					ui.Scroll(c.Key("macros:steps")).Grow(1).Children(func() {
						ui.Column(c).Padding(8, 10).Gap(4).Children(func() {
							macro := m.selectedMacro()
							if macro == nil {
								return
							}
							for i, step := range macro.Steps {
								ui.Text(c, fmt.Sprintf("%d. %s", i+1,
									macroDescribeStep(step, m.macroActionTitle))).
									FontSize(12).TextColor(oi.text)
							}
						})
					})
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, "Close").Clicked() {
					m.macros.editOpen = false
					m.invalidate()
				}
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.macros.editOpen = false
		m.macros.editSelection = clampIndex(m.macros.editSelection, len(m.macros.saved))
		m.invalidate()
	}
}

func (m *Model) macroNameDialog(c *ui.Context) {
	if !m.macros.nameOpen {
		return
	}
	p := c.Theme()
	oi := oiPaletteOf(p)
	open := true
	dialog := ui.Modal(c.Key("macros:name"), &open, func() {
		ui.Column(c).Width(460).Padding(18).Gap(10).
			Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
			ui.Text(c, "Enter Macro Name").FontSize(14).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Text(c, "Name (leave empty to keep it only as the last macro):").
				FontSize(12).TextColor(oi.textMuted)
			input := ui.TextInput(c.Key("macros:name-input"), &m.macros.nameDraft).
				Width(420).AutoFocus()
			input.OnSubmit(func() { m.finishMacroNaming(true) })
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					m.finishMacroNaming(false)
				}
				if ui.PrimaryButton(c, "OK").Clicked() {
					m.finishMacroNaming(true)
				}
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.finishMacroNaming(false)
	}
}

func (m *Model) macroPlayDialog(c *ui.Context) {
	if !m.macros.playOpen {
		return
	}
	p := c.Theme()
	oi := oiPaletteOf(p)
	open := true
	dialog := ui.Modal(c.Key("macros:play"), &open, func() {
		ui.Column(c).Width(430).Gap(10).Padding(18).
			Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
			ui.Text(c, "Play Saved Macro").FontSize(14).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Box(c).Height(220).Border(1, oi.border).Radius(4).Children(func() {
				ui.Scroll(c.Key("macros:play-list")).Grow(1).Children(func() {
					ui.Column(c).Padding(4).Gap(1).Children(func() {
						for i, macro := range m.macros.saved {
							i, macro := i, macro
							row := ui.Row(c.Key(fmt.Sprintf("macros:play-item:%d", i))).
								Height(24).Padding(0, 8).AlignItems(ui.Center).
								Cursor(ui.CursorPointer)
							switch {
							case i == m.macros.playSelection:
								row.Background(oi.selected)
							case row.Hovered():
								row.Background(oi.hoverTint)
							}
							row.OnClick(func() {
								m.macros.playSelection = i
								m.invalidate()
							})
							row.Children(func() {
								ui.Text(c, macro.Name).FontSize(12).
									TextColor(oi.text).SingleLine().Ellipsis("…")
							})
						}
					})
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					m.macros.playOpen = false
					m.invalidate()
				}
				play := ui.PrimaryButton(c, "Play")
				play.Disabled(len(m.macros.saved) == 0)
				if play.Clicked() {
					index := clampIndex(m.macros.playSelection, len(m.macros.saved))
					name := m.macros.saved[index].Name
					m.macros.playOpen = false
					m.playSavedMacro(name)
				}
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.macros.playOpen = false
		m.invalidate()
	}
}

func (m *Model) macroRenameDialog(c *ui.Context) {
	if !m.macros.renameOpen {
		return
	}
	p := c.Theme()
	oi := oiPaletteOf(p)
	open := true
	dialog := ui.Modal(c.Key("macros:rename"), &open, func() {
		ui.Column(c).Width(420).Padding(18).Gap(10).
			Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
			ui.Text(c, "Rename Macro").FontSize(14).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Text(c, "New name:").FontSize(12).TextColor(oi.textMuted).SingleLine()
			input := ui.TextInput(c.Key("macros:rename-input"), &m.macros.renameDraft).
				Width(380).AutoFocus()
			input.OnChange(func() {
				m.macros.renameErr = ""
				m.invalidate()
			})
			input.OnSubmit(func() {
				if m.macroRenameError() == "" {
					m.commitRenameMacro()
				}
			})
			if validation := m.macroRenameError(); validation != "" {
				ui.Text(c, validation).FontSize(12).TextColor(oi.danger).SingleLine()
			}
			if m.macros.renameErr != "" {
				ui.Text(c, m.macros.renameErr).FontSize(12).TextColor(oi.danger).SingleLine()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					m.macros.renameOpen = false
					m.invalidate()
				}
				rename := ui.Button(c, "Rename")
				rename.Disabled(m.macroRenameError() != "")
				rename.OnClick(m.commitRenameMacro)
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.macros.renameOpen = false
		m.invalidate()
	}
}

func (m *Model) macroDeleteDialog(c *ui.Context) {
	if !m.macros.deleteOpen {
		return
	}
	macro := m.selectedMacro()
	if macro == nil {
		m.macros.deleteOpen = false
		return
	}
	p := c.Theme()
	oi := oiPaletteOf(p)
	open := true
	dialog := ui.Modal(c.Key("macros:delete"), &open, func() {
		ui.Column(c).Width(420).Padding(18).Gap(12).
			Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
			ui.Text(c, "Delete Macro").FontSize(14).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Text(c, "Delete the macro '"+macro.Name+"'?").
				FontSize(12).TextColor(oi.text)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					m.macros.deleteOpen = false
					m.invalidate()
				}
				if ui.Button(c, "Delete").Clicked() {
					m.deleteSelectedMacro()
				}
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.macros.deleteOpen = false
		m.invalidate()
	}
}
