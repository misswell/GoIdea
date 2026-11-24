package app

// The keymap registry after OpenIDEA's platform/keymap: shortcuts live apart
// from actions, every default comes from the six pinned OpenIDEA TypeScript
// presets, and the user's overrides replace a binding or remove it with an
// empty list.

//go:generate python3 ../../scripts/generate-keymap-presets.py

import (
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type keymapActionDefinition struct {
	ID       string
	Bindings []string
}

type keymapPresetDefinition struct {
	ID      string
	Title   string
	Actions []keymapActionDefinition
}

type keymapPresetRuntime struct {
	ID       string
	Title    string
	Actions  []keymapActionDefinition
	Bindings map[string][]string
	// CanonicalBindings resolves OpenIDEA ids and GoIDEA alias ids to one
	// effective default.
	CanonicalBindings map[string][]string
}

type keymapInputStroke struct {
	Mods ui.Modifiers
	Key  ui.Key
	Name string
}

type keymapDispatchCandidate struct {
	ActionID string
	Binding  string
	Strokes  []keymapInputStroke
}

type keymapDispatchState struct {
	signature  string
	candidates []keymapDispatchCandidate
	strokes    []keymapInputStroke
	pending    []string
	pendingAt  time.Time
}

var (
	keymapPresetsByID = map[string]keymapPresetRuntime{}
	keymapPresetOrder = make([]keymapPresetRuntime, 0, len(keymapPresetDefinitions))
)

func keymapPresetDefinitionsByOS() string {
	if runtime.GOOS == "darwin" {
		return "intellij-macos"
	}
	return "intellij-windows"
}

func keymapPresetForID(id string) keymapPresetRuntime {
	if preset, ok := keymapPresetsByID[id]; ok {
		return preset
	}
	return keymapPresetsByID[keymapPresetDefinitionsByOS()]
}

func keymapUserBindingsForID(overrides map[string][]string, id string) ([]string, bool) {
	if bindings, ok := overrides[id]; ok {
		return normalizeKeymapBindings(bindings), true
	}
	canonical := canonicalActionID(id)
	if bindings, ok := overrides[canonical]; ok {
		return normalizeKeymapBindings(bindings), true
	}
	return nil, false
}

func normalizeKeymapBindings(bindings []string) []string {
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if normalized := keymapNormalize(binding); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

// keymapNormalize makes "CmdOrCtrl+Shift+A" and "cmd+shift+a" the same key.
func keymapNormalize(binding string) string {
	binding = strings.TrimSpace(binding)
	if binding == "" {
		return ""
	}
	strokes := strings.Fields(binding)
	for i, stroke := range strokes {
		parts := strings.Split(stroke, "+")
		if len(parts) == 1 {
			strokes[i] = keymapKeyDisplay(parts[0])
			continue
		}
		key := keymapKeyDisplay(parts[len(parts)-1])
		mods := map[string]bool{}
		for _, part := range parts[:len(parts)-1] {
			switch strings.ToLower(strings.TrimSpace(part)) {
			case "cmdorctrl", "cmd", "meta":
				mods["Cmd"] = true
			case "alt", "option":
				mods["Alt"] = true
			case "ctrl", "control":
				mods["Ctrl"] = true
			case "shift":
				mods["Shift"] = true
			}
		}
		ordered := make([]string, 0, 5)
		for _, modifier := range []string{"Ctrl", "Alt", "Shift", "Cmd"} {
			if mods[modifier] {
				ordered = append(ordered, modifier)
			}
		}
		ordered = append(ordered, key)
		strokes[i] = strings.Join(ordered, "+")
	}
	return strings.Join(strokes, " ")
}

func keymapKeyDisplay(key string) string {
	key = strings.TrimSpace(key)
	switch strings.ToLower(key) {
	case "esc", "escape":
		return "Escape"
	case "return", "enter":
		return "Enter"
	case "space":
		return "Space"
	case "tab":
		return "Tab"
	case "backspace":
		return "Backspace"
	case "delete":
		return "Delete"
	case "up", "down", "left", "right":
		return strings.ToUpper(key[:1]) + key[1:]
	case "numpadadd":
		return "NumpadAdd"
	case "numpadsubtract":
		return "NumpadSubtract"
	case "numpadmultiply":
		return "NumpadMultiply"
	case "numpaddivide":
		return "NumpadDivide"
	default:
		if len(key) == 1 {
			return strings.ToUpper(key)
		}
		return key
	}
}

// keymapUserBindings is the stored override map; a nil map means defaults.
func (m *Model) keymapUserBindings() map[string][]string {
	return m.prefs.Keymap
}

// cloneKeymap copies the override map and every binding slice so editing the
// Settings draft cannot mutate the preferences behind the dialog.
func cloneKeymap(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string][]string, len(source))
	for id, bindings := range source {
		out[id] = append([]string(nil), bindings...)
	}
	return out
}

// sameKeymaps compares two override maps after key normalization. A missing
// entry, a nil binding slice and an empty binding slice all mean unbound.
func sameKeymaps(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for id, left := range a {
		right, ok := b[id]
		if !ok || !sameKeyBindings(left, right) {
			return false
		}
	}
	return true
}

func sameKeyBindings(a, b []string) bool {
	left := make([]string, 0, len(a))
	for _, binding := range a {
		if normalized := keymapNormalize(binding); normalized != "" {
			left = append(left, normalized)
		}
	}
	right := make([]string, 0, len(b))
	for _, binding := range b {
		if normalized := keymapNormalize(binding); normalized != "" {
			right = append(right, normalized)
		}
	}
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// keymapPresetID returns the stored preset or this operating system's default.
func (m *Model) keymapPresetID() string {
	if _, ok := keymapPresetsByID[m.prefs.KeymapPreset]; ok {
		return m.prefs.KeymapPreset
	}
	return keymapPresetDefinitionsByOS()
}

func (m *Model) keymapPresetTitle() string {
	return keymapPresetForID(m.keymapPresetID()).Title
}

// KeymapBindings resolves an action's bindings in the selected preset: a user
// override when it has an entry, the preset default otherwise.
func (m *Model) KeymapBindings(id string) []string {
	return keymapBindingsIn(m.keymapPresetID(), m.keymapUserBindings(), id)
}

func (m *Model) keymapHasBinding(id, binding string) bool {
	binding = keymapNormalize(binding)
	if binding == "" {
		return false
	}
	for _, current := range m.KeymapBindings(id) {
		if current == binding {
			return true
		}
	}
	return false
}

// keymapBindingsIn resolves bindings against an override map, including the
// staged map shown in Settings before Apply.
func keymapBindingsIn(presetID string, overrides map[string][]string, id string) []string {
	if bindings, ok := keymapUserBindingsForID(overrides, id); ok {
		return bindings
	}
	preset := keymapPresetForID(presetID)
	canonical := canonicalActionID(id)
	if bindings, ok := preset.CanonicalBindings[canonical]; ok {
		return append([]string(nil), bindings...)
	}
	return nil
}

// keymapShortcutLabel is the first binding a row shows, formatted for the UI.
func keymapShortcutLabel(id string) string {
	return keymapLabelOf(keymapBindingsIn(keymapPresetDefinitionsByOS(), nil, id))
}

func keymapLabelOf(bindings []string) string {
	if len(bindings) == 0 {
		return ""
	}
	return keymapDisplay(bindings[0])
}

// keymapDisplay writes a binding the way the palette and settings name it
// today; the native menu labels use the same spelling.
func keymapDisplay(binding string) string { return binding }

const keymapChordTimeout = 2 * time.Second

// SetKeymapPresetPref stores one of OpenIDEA's six presets and applies it to
// open editors immediately.
func (m *Model) SetKeymapPresetPref(id string) {
	if _, ok := keymapPresetsByID[id]; !ok {
		return
	}
	if m.keymapPresetID() == id && m.prefs.KeymapPreset != "" {
		return
	}
	m.prefs.KeymapPreset = id
	m.applyKeymapPreferences()
	m.savePreferences()
	m.status = "Keymap: " + keymapPresetForID(id).Title
	m.windowMenuChanged()
	m.invalidate()
}

// applyKeymapPreferences rebuilds every open code editor's application
// bindings from the selected preset while leaving its intrinsic editing
// defaults below them.
func (m *Model) applyKeymapPreferences() {
	m.keymapDispatch = keymapDispatchState{}
	apply := func(tabs []*Tab) {
		for _, tab := range tabs {
			if tab != nil && tab.state != nil {
				tab.state.Bindings = m.editorBindings()
			}
		}
	}
	apply(m.tabs)
	if m.split != nil {
		apply(m.split.tabs)
	}
}

// dispatchKeymapShortcut runs the selected preset's action bindings before
// the native context-specific keys. It implements the same context-first
// tie-break as OpenIDEA and keeps one pending stroke for two-key chords.
func (m *Model) dispatchKeymapShortcut(c *ui.Context) bool {
	if m.modalOverlayOpen() || (m.overlayOpen() && m.documentation.popup == nil) {
		m.keymapDispatch.pending = nil
		return false
	}
	now := c.Now()
	if len(m.keymapDispatch.pending) > 0 &&
		now.Sub(m.keymapDispatch.pendingAt) > keymapChordTimeout {
		m.keymapDispatch.pending = nil
	}
	m.ensureKeymapDispatch()
	var pressed keymapInputStroke
	found := false
	if len(m.keymapDispatch.pending) > 0 {
		for _, stroke := range m.keymapPendingStrokes() {
			if c.Shortcut(stroke.Mods, stroke.Key) {
				pressed = stroke
				found = true
				break
			}
		}
	}
	if !found {
		for _, stroke := range m.keymapDispatch.strokes {
			if c.Shortcut(stroke.Mods, stroke.Key) {
				pressed = stroke
				found = true
				break
			}
		}
	}
	if !found {
		return false
	}

	chord := len(m.keymapDispatch.pending) > 0
	sequence := pressed.Name
	if chord {
		sequence = strings.Join(append(m.keymapDispatch.pending, pressed.Name), " ")
	}
	exact := make([]keymapDispatchCandidate, 0, 1)
	prefix := false
	for _, candidate := range m.keymapDispatch.candidates {
		switch {
		case candidate.Binding == sequence:
			exact = append(exact, candidate)
		case strings.HasPrefix(candidate.Binding, sequence+" "):
			prefix = true
		}
	}
	if len(exact) > 0 && m.runKeymapCandidates(exact) {
		m.keymapDispatch.pending = nil
		return true
	}
	if !chord && prefix {
		m.keymapDispatch.pending = []string{pressed.Name}
		m.keymapDispatch.pendingAt = now
		m.registerKeymapPendingStrokes(c)
		return true
	}
	if chord {
		// A second stroke that completes no chord is swallowed, as in IDEA.
		m.keymapDispatch.pending = nil
		return true
	}
	return false
}

func (m *Model) runKeymapCandidates(candidates []keymapDispatchCandidate) bool {
	winner, ok := m.keymapWinningCandidate(candidates)
	return ok && m.RunCommand(winner.ActionID)
}

// keymapWinningCandidate follows OpenIDEA's tie-break: later source bindings
// win, while a runnable action whose KeyWhen matches the current context beats
// a context-free action with the same binding.
func (m *Model) keymapWinningCandidate(
	candidates []keymapDispatchCandidate,
) (keymapDispatchCandidate, bool) {
	fallback := make([]keymapDispatchCandidate, 0, len(candidates))
	for i := len(candidates) - 1; i >= 0; i-- {
		candidate := candidates[i]
		action, ok := actionGet(candidate.ActionID)
		if !ok || !action.usable(m) {
			continue
		}
		if action.KeyWhen == nil {
			fallback = append(fallback, candidate)
			continue
		}
		if action.KeyWhen(m) {
			return candidate, true
		}
	}
	if len(fallback) > 0 {
		return fallback[0], true
	}
	return keymapDispatchCandidate{}, false
}

func (m *Model) ensureKeymapDispatch() {
	presetID := m.keymapPresetID()
	signature := keymapOverridesSignature(presetID, m.keymapUserBindings())
	if m.keymapDispatch.signature == signature {
		return
	}
	m.keymapDispatch = keymapDispatchState{signature: signature}
	seenStrokes := map[uint64]bool{}
	for _, action := range m.keymapEffectiveActions() {
		for _, binding := range action.Bindings {
			strokes, ok := keymapInputStrokes(binding)
			if !ok {
				continue
			}
			m.keymapDispatch.candidates = append(m.keymapDispatch.candidates, keymapDispatchCandidate{
				ActionID: action.ID,
				Binding:  keymapNormalize(binding),
				Strokes:  strokes,
			})
			// A bare single key belongs to focused text controls. Only a
			// modified first stroke is claimed globally; the second stroke of
			// a chord is registered just after that chord starts.
			stroke := strokes[0]
			if stroke.Mods == 0 {
				continue
			}
			key := uint64(stroke.Mods)<<32 | uint64(stroke.Key)
			if seenStrokes[key] {
				continue
			}
			seenStrokes[key] = true
			m.keymapDispatch.strokes = append(m.keymapDispatch.strokes, stroke)
		}
	}
}

// registerKeymapPendingStrokes claims the possible second strokes after a
// chord starts. This frame's registration is what routes the next key to the
// window before the focused editor can consume an unmodified letter.
func (m *Model) registerKeymapPendingStrokes(c *ui.Context) {
	for _, stroke := range m.keymapPendingStrokes() {
		c.Shortcut(stroke.Mods, stroke.Key)
	}
}

func (m *Model) keymapPendingStrokes() []keymapInputStroke {
	if len(m.keymapDispatch.pending) == 0 {
		return nil
	}
	sequence := strings.Join(m.keymapDispatch.pending, " ")
	var out []keymapInputStroke
	seen := map[uint64]bool{}
	for _, candidate := range m.keymapDispatch.candidates {
		index := len(m.keymapDispatch.pending)
		if len(candidate.Strokes) <= index ||
			!strings.HasPrefix(candidate.Binding, sequence+" ") {
			continue
		}
		stroke := candidate.Strokes[index]
		key := uint64(stroke.Mods)<<32 | uint64(stroke.Key)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, stroke)
	}
	return out
}

func keymapOverridesSignature(presetID string, overrides map[string][]string) string {
	if len(overrides) == 0 {
		return presetID
	}
	ids := make([]string, 0, len(overrides))
	for id := range overrides {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var builder strings.Builder
	builder.WriteString(presetID)
	for _, id := range ids {
		builder.WriteByte(0)
		builder.WriteString(id)
		builder.WriteByte('=')
		builder.WriteString(strings.Join(normalizeKeymapBindings(overrides[id]), "\x1f"))
	}
	return builder.String()
}

// keymapEffectiveActions keeps preset source order, substitutes the user's
// bindings for matching actions, and appends overrides for actions the preset
// does not name.
func (m *Model) keymapEffectiveActions() []keymapActionDefinition {
	preset := keymapPresetForID(m.keymapPresetID())
	actions := make([]keymapActionDefinition, 0, len(preset.Actions)+len(m.prefs.Keymap))
	seen := make(map[string]bool, len(preset.Actions))
	for _, action := range preset.Actions {
		seen[canonicalActionID(action.ID)] = true
		actions = append(actions, keymapActionDefinition{
			ID:       action.ID,
			Bindings: keymapBindingsIn(m.keymapPresetID(), m.prefs.Keymap, action.ID),
		})
	}
	ids := make([]string, 0, len(m.prefs.Keymap))
	for id := range m.prefs.Keymap {
		if !seen[canonicalActionID(id)] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		actions = append(actions, keymapActionDefinition{
			ID:       id,
			Bindings: keymapBindingsIn(m.keymapPresetID(), m.prefs.Keymap, id),
		})
	}
	return actions
}

// keymapInputStrokes parses the keyboard forms MyGo can deliver. Mouse,
// swipe, bare double-modifier gestures and numpad-only keys remain visible in
// Settings but are not claimed by the generic keyboard dispatcher.
func keymapInputStrokes(binding string) ([]keymapInputStroke, bool) {
	strokes := strings.Fields(keymapNormalize(binding))
	if len(strokes) == 0 || len(strokes) > 2 {
		return nil, false
	}
	out := make([]keymapInputStroke, 0, len(strokes))
	for _, stroke := range strokes {
		parsed, ok := keymapInputStrokeOf(stroke)
		if !ok {
			return nil, false
		}
		out = append(out, parsed)
	}
	return out, true
}

func keymapInputStrokeOf(stroke string) (keymapInputStroke, bool) {
	parts := strings.Split(stroke, "+")
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "Ctrl":
			mods |= ui.Ctrl
		case "Alt":
			mods |= ui.Alt
		case "Shift":
			mods |= ui.Shift
		case "Cmd":
			mods |= ui.Super
		default:
			return keymapInputStroke{}, false
		}
	}
	name := parts[len(parts)-1]
	key, ok := keymapUIKey(name)
	if !ok {
		return keymapInputStroke{}, false
	}
	return keymapInputStroke{Mods: mods, Key: key, Name: stroke}, true
}

func keymapUIKey(name string) (ui.Key, bool) {
	if len(name) == 1 {
		switch {
		case name[0] >= 'A' && name[0] <= 'Z':
			return ui.KeyA + ui.Key(name[0]-'A'), true
		case name[0] >= '0' && name[0] <= '9':
			return ui.Key0 + ui.Key(name[0]-'0'), true
		}
	}
	if len(name) == 2 && name[0] == 'F' &&
		name[1] >= '1' && name[1] <= '9' {
		return ui.KeyF1 + ui.Key(name[1]-'1'), true
	}
	if len(name) == 3 && name[0] == 'F' && name[1] == '1' &&
		name[2] >= '0' && name[2] <= '2' {
		return ui.KeyF1 + ui.Key(9+name[2]-'0'), true
	}
	keys := map[string]ui.Key{
		"Escape":    ui.KeyEscape,
		"Enter":     ui.KeyEnter,
		"Space":     ui.KeySpace,
		"Tab":       ui.KeyTab,
		"Backspace": ui.KeyBackspace,
		"Delete":    ui.KeyDelete,
		"Insert":    ui.KeyInsert,
		"Home":      ui.KeyHome,
		"End":       ui.KeyEnd,
		"PageUp":    ui.KeyPageUp,
		"PageDown":  ui.KeyPageDown,
		"Left":      ui.KeyLeft,
		"Right":     ui.KeyRight,
		"Up":        ui.KeyUp,
		"Down":      ui.KeyDown,
		"-":         ui.KeyMinus,
		"=":         ui.KeyEqual,
		",":         ui.KeyComma,
		".":         ui.KeyPeriod,
		"/":         ui.KeySlash,
		";":         ui.KeySemicolon,
		"'":         ui.KeyQuote,
		"[":         ui.KeyBracketLeft,
		"]":         ui.KeyBracketRight,
		"\\":        ui.KeyBackslash,
		"`":         ui.KeyBackquote,
	}
	key, ok := keys[name]
	return key, ok
}

// KeymapSetOverrides replaces the user's bindings, as Settings › Keymap writes
// them; an empty list for an action removes its shortcut.
func (m *Model) KeymapSetOverrides(overrides map[string][]string) {
	if len(overrides) == 0 {
		m.prefs.Keymap = nil
		m.keymapDispatch = keymapDispatchState{}
		m.windowMenuChanged()
		return
	}
	m.prefs.Keymap = make(map[string][]string, len(overrides))
	for id, bindings := range overrides {
		norm := make([]string, 0, len(bindings))
		for _, b := range bindings {
			if n := keymapNormalize(b); n != "" {
				norm = append(norm, n)
			}
		}
		m.prefs.Keymap[id] = norm
	}
	m.keymapDispatch = keymapDispatchState{}
	m.windowMenuChanged()
}

// keymapConflict is two actions fighting over one binding.
type keymapConflict struct {
	Binding string
	Actions []string
}

// KeymapConflicts lists bindings two usable actions share, as OpenIDEA's
// Keymap settings warns. Actions whose KeyWhen never overlaps are not
// separated yet; the check errs on the side of warning.
func (m *Model) KeymapConflicts() []keymapConflict {
	return keymapConflictsIn(m.keymapPresetID(), m.keymapUserBindings())
}

// keymapConflictsIn is KeymapConflicts over an explicit override map so the
// dialog can warn about a staged binding before Apply.
func keymapConflictsIn(presetID string, overrides map[string][]string) []keymapConflict {
	byBinding := map[string][]string{}
	for _, a := range actionList() {
		for _, b := range keymapBindingsIn(presetID, overrides, a.ID) {
			byBinding[b] = append(byBinding[b], a.ID)
		}
	}
	var out []keymapConflict
	for binding, ids := range byBinding {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		out = append(out, keymapConflict{Binding: binding, Actions: ids})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Binding < out[j].Binding })
	return out
}

func init() {
	for _, definition := range keymapPresetDefinitions {
		preset := keymapPresetRuntime{
			ID:                definition.ID,
			Title:             definition.Title,
			Actions:           make([]keymapActionDefinition, 0, len(definition.Actions)),
			Bindings:          make(map[string][]string, len(definition.Actions)),
			CanonicalBindings: make(map[string][]string, len(definition.Actions)),
		}
		for _, action := range definition.Actions {
			bindings := normalizeKeymapBindings(action.Bindings)
			preset.Actions = append(preset.Actions, keymapActionDefinition{
				ID: action.ID, Bindings: bindings,
			})
			preset.Bindings[action.ID] = bindings
			canonical := canonicalActionID(action.ID)
			preset.CanonicalBindings[canonical] = append(
				preset.CanonicalBindings[canonical], bindings...,
			)
		}
		keymapPresetsByID[preset.ID] = preset
		keymapPresetOrder = append(keymapPresetOrder, preset)
	}
}
