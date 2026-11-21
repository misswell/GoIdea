package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// editorPathFlavour is one row of OpenIDEA's Copy Path/Reference chooser.
type editorPathFlavour struct {
	label string
	value string
}

// editorPathFlavours lists the paths OpenIDEA copies from the caret: the
// absolute path, the file name, the path from the content root and that path
// with the caret's line.
func editorPathFlavours(root, path string, line int) []editorPathFlavour {
	rel := projectRelative(root, path)
	return []editorPathFlavour{
		{"Absolute Path", path},
		{"File Name", filepath.Base(path)},
		{"Path From Content Root", rel},
		{"Path with line number", fmt.Sprintf("%s:%d", rel, line)},
	}
}

// tabPathFlavours are the paths OpenIDEA's tab menu copies: the absolute
// path, the file name and, inside the project, the path from the content
// root.
func tabPathFlavours(root, path string) []editorPathFlavour {
	flavours := []editorPathFlavour{
		{"Absolute Path", path},
		{"File Name", filepath.Base(path)},
	}
	if rel := projectRelative(root, path); rel != path {
		flavours = append(flavours, editorPathFlavour{"Path From Content Root", rel})
	}
	return flavours
}

// projectRelative is path inside root, or path itself when it lies outside.
func projectRelative(root, path string) string {
	if root == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}

// editorMenu adds OpenIDEA's editor context-menu actions to MyGo's editing
// commands, which stay at the top of the menu.
func (m *Model) editorMenu(menu *ui.Menu, t *Tab) {
	path := t.doc.Path()
	if path == "" {
		return
	}
	if menu.Item("Show Context Actions").Shortcut(ui.Alt, ui.KeyEnter).Chosen() {
		m.ShowIntentions()
	}
	menu.Separator()
	menu.Submenu("Copy / Paste Special", func(menu *ui.Menu) {
		if menu.Item("Copy Path/Reference…").Shortcut(ui.Cmd|ui.Shift, ui.KeyC).Chosen() {
			m.OpenCopyPathReferencePopup()
		}
	})
	menu.Separator()
	menu.Submenu("Go To", func(menu *ui.Menu) {
		if menu.Item("Find Usages").Shortcut(ui.Alt, ui.KeyF7).Chosen() {
			m.FindUsages()
		}
		if menu.Item("Show Usages").Shortcut(ui.Cmd|ui.Alt, ui.KeyF7).Chosen() {
			m.ShowUsagesPopup()
		}
	})
	if menu.Item("Configure Code Vision…").Chosen() {
		m.OpenSettingsPage(settingsPageInlayHints)
	}
	menu.Separator()
	menu.Submenu("Local History", func(menu *ui.Menu) {
		if menu.Item("Show History").Chosen() {
			m.ShowHistory(path)
		}
		if menu.Item("Put Label…").Chosen() {
			m.OpenHistoryLabel()
		}
	})
	menu.Separator()
	if menu.Item("Reformat Code").Shortcut(ui.Cmd|ui.Alt, ui.KeyL).Chosen() {
		m.FormatActive()
	}
	if m.fixESLintProblemsAvailable(t) &&
		menu.Item("Fix ESLint Problems").Chosen() {
		m.FixESLintProblems()
	}
	if menu.Item("Rename…").Shortcut(ui.Shift, ui.KeyF6).Chosen() {
		m.RenameActive()
	}
	if menu.Item("Safe Delete…").Shortcut(ui.Cmd, ui.KeyDelete).Chosen() {
		m.SafeDeleteActive()
	}
	if menu.Item("Send to Agent").Chosen() {
		m.SendEditorToAgent()
	}
	m.compileFileMenu(menu, path)
}

func (m *Model) canCompileFile(path string) bool {
	return m.Root() != "" && strings.EqualFold(filepath.Ext(path), ".java")
}

func (m *Model) compileFileMenu(menu *ui.Menu, path string) {
	if m.canCompileFile(path) && menu.Item("Compile File").Chosen() {
		m.CompileFilePath(path)
	}
}

// copyEditorText puts text on the clipboard and reports it in the status bar,
// as OpenIDEA shows its "Copied …" hint.
func (m *Model) copyEditorText(text string) {
	mygo.Clipboard.WriteText(text)
	m.status = "Copied " + text
	m.invalidate()
}
