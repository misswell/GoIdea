package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// intellijEditorBindings are the editor keys of OpenIDEA's IntelliJ macOS
// keymap (src/platform/keymaps/intellij-macos.ts) where it parts from MyGo's
// Zed defaults: IntelliJ works on lines where Zed adds carets, so Cmd+D
// duplicates, Cmd+Backspace deletes the line and Alt+Shift+Up/Down moves it,
// and it joins and moves statements on its own keys.
func intellijEditorBindings() []editor.Binding {
	cmd, alt, ctrl, sh := ui.Cmd, ui.Alt, ui.Ctrl, ui.Shift
	return []editor.Binding{
		{Mods: cmd, Key: ui.KeySlash, Command: editor.ToggleComment},
		{Mods: cmd | alt, Key: ui.KeySlash, Command: editor.ToggleBlockComment},
		{Mods: ctrl | sh, Key: ui.KeySlash, Command: editor.ToggleBlockComment},
		{Mods: cmd, Key: ui.KeyD, Command: editor.DuplicateSelection},
		{Mods: cmd, Key: ui.KeyBackspace, Command: editor.DeleteLine},
		{Mods: alt | sh, Key: ui.KeyUp, Command: editor.MoveLinesUp},
		{Mods: alt | sh, Key: ui.KeyDown, Command: editor.MoveLinesDown},
		{Mods: cmd | sh, Key: ui.KeyUp, Command: editor.MoveLinesUp},
		{Mods: cmd | sh, Key: ui.KeyDown, Command: editor.MoveLinesDown},
		{Mods: ctrl | sh, Key: ui.KeyJ, Command: editor.JoinLines},
	}
}

// runEditorCommand runs a command of the active editor: the Code menu's line
// actions and the editor's own keys both come here, so they share one
// implementation and one undo history.
func (m *Model) runEditorCommand(cmd editor.Command) {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	if t.state.Run(cmd) {
		m.invalidate()
	}
}
