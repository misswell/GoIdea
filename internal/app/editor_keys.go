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
		{Mods: cmd, Key: ui.KeyF, Command: editor.Find},
		{Mods: cmd, Key: ui.KeyR, Command: editor.FindReplace},
		{Mods: cmd, Key: ui.KeyG, Command: editor.FindNext},
		{Mods: cmd | sh, Key: ui.KeyG, Command: editor.FindPrevious},
	}
}

// runEditorCommand runs a command of the active editor: the Code menu's line
// actions and the editor's own keys both come here, so they share one
// implementation and one undo history.
func (m *Model) runEditorCommand(cmd editor.Command) bool {
	t := m.tab()
	if t == nil || t.state == nil {
		return false
	}
	if t.state.Run(cmd) {
		m.invalidate()
		return true
	}
	return false
}

// activeEditorAvailable gates an action that needs an open code editor.
func (m *Model) activeEditorAvailable() bool {
	t := m.tab()
	return t != nil && t.state != nil
}

// activeEditorEditable gates Cut and Paste. Tests that build a tab without a
// document still exercise the editor state, while production tabs use the
// document's read-only flag.
func (m *Model) activeEditorEditable() bool {
	t := m.tab()
	return t != nil && t.state != nil && !t.state.ReadOnly &&
		(t.doc == nil || !t.doc.ReadOnly())
}

func (m *Model) activeEditorCanUndo() bool {
	t := m.tab()
	return t != nil && t.state != nil && t.state.Buffer != nil && t.state.Buffer.CanUndo()
}

func (m *Model) activeEditorCanRedo() bool {
	t := m.tab()
	return t != nil && t.state != nil && t.state.Buffer != nil && t.state.Buffer.CanRedo()
}

func (m *Model) activeEditorSoftWrapped() bool {
	t := m.tab()
	return t != nil && t.state != nil && t.state.SoftWrap
}

func (m *Model) activeEditorRenderedDocs() bool {
	t := m.tab()
	if t == nil {
		return false
	}
	if t.renderedDocsOverride != nil {
		return *t.renderedDocsOverride
	}
	return m.prefs.Appearance.RenderDocComments
}

// toggleActiveEditorSoftWrap changes the active editor's effective wrapping
// for this tab, leaving the configured file patterns alone.
func (m *Model) toggleActiveEditorSoftWrap() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	t.state.SoftWrap = !t.state.SoftWrap
	m.invalidate()
}

// toggleActiveEditorRenderedDocs overrides the global setting for this tab,
// as View › Active Editor does in OpenIDEA.
func (m *Model) toggleActiveEditorRenderedDocs() {
	t := m.tab()
	if t == nil || t.state == nil {
		return
	}
	on := !m.activeEditorRenderedDocs()
	t.renderedDocsOverride = &on
	state := "off"
	if on {
		state = "on"
		if !m.showRenderedDocumentationComment(t) {
			m.status = "Render documentation comments on · no doc comment at caret"
			m.invalidate()
			return
		}
	} else if m.documentation.popup != nil {
		m.closeDocumentationPopup()
	}
	m.status = "Render documentation comments " + state
	m.invalidate()
}
