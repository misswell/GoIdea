package app

// The keymap registry after OpenIDEA's platform/keymap: shortcuts live apart
// from actions, defaults follow the macOS IntelliJ map the shipped commands
// name, and the user's overrides replace a binding or remove it with an empty
// list. Dispatch still runs through the editor bindings and the shortcut
// chain; this registry is what menus, the palette and settings read, and what
// conflict detection compares.

import (
	"sort"
	"strings"
)

// keymapDefaults is the built-in binding of each action, normalized.
var keymapDefaults = map[string][]string{}

// keymapNormalize makes "CmdOrCtrl+Shift+A" and "cmd+shift+a" the same key.
func keymapNormalize(binding string) string {
	binding = strings.TrimSpace(binding)
	if binding == "" {
		return ""
	}
	parts := strings.Split(binding, "+")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		switch strings.ToLower(p) {
		case "cmdorctrl", "cmd", "meta":
			p = "Cmd"
		case "alt", "option":
			p = "Alt"
		case "ctrl", "control":
			p = "Ctrl"
		case "shift":
			p = "Shift"
		case "esc", "escape":
			p = "Escape"
		case "return", "enter":
			p = "Enter"
		case "space":
			p = "Space"
		case "tab":
			p = "Tab"
		case "backspace":
			p = "Backspace"
		case "delete":
			p = "Delete"
		case "up", "down", "left", "right":
			p = strings.ToUpper(p[:1]) + p[1:]
		default:
			if len(p) == 1 {
				p = strings.ToUpper(p)
			}
		}
		parts[i] = p
	}
	return strings.Join(parts, "+")
}

// keymapUserBindings is the stored override map; a nil map means defaults.
func (m *Model) keymapUserBindings() map[string][]string {
	return m.prefs.Keymap
}

// KeymapBindings resolves an action's bindings: a user override when it has
// an entry (an empty list removes the binding), the default otherwise.
func (m *Model) KeymapBindings(id string) []string {
	if custom, ok := m.keymapUserBindings()[id]; ok {
		out := make([]string, 0, len(custom))
		for _, b := range custom {
			if n := keymapNormalize(b); n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	return append([]string(nil), keymapDefaults[id]...)
}

// keymapShortcutLabel is the first binding a row shows, formatted for the UI.
func keymapShortcutLabel(id string) string {
	return keymapLabelOf(keymapDefaults[id])
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

// KeymapSetOverrides replaces the user's bindings, as Settings › Keymap writes
// them; an empty list for an action removes its shortcut.
func (m *Model) KeymapSetOverrides(overrides map[string][]string) {
	if len(overrides) == 0 {
		m.prefs.Keymap = nil
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
	byBinding := map[string][]string{}
	for _, a := range actionList() {
		for _, b := range m.KeymapBindings(a.ID) {
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
	// The shipped command list names its macOS keys; they become the default
	// keymap, and only registered actions are listed.
	for id, shortcut := range builtinShortcuts {
		if strings.TrimSpace(shortcut) == "" {
			continue
		}
		for _, part := range strings.Split(shortcut, " ") {
			if n := keymapNormalize(part); n != "" {
				keymapDefaults[id] = append(keymapDefaults[id], n)
			}
		}
	}
}
