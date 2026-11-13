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
	line := t.state.CursorPos().Line + 1
	if menu.Item("Show Context Actions").Shortcut(ui.Alt, ui.KeyEnter).Chosen() {
		m.ShowIntentions()
	}
	menu.Separator()
	menu.Submenu("Copy / Paste Special", func(menu *ui.Menu) {
		menu.Submenu("Copy Path/Reference…", func(menu *ui.Menu) {
			for _, f := range editorPathFlavours(m.Root(), path, line) {
				if menu.Item(f.label).Chosen() {
					m.copyEditorText(f.value)
				}
			}
		})
	})
	menu.Separator()
	menu.Submenu("Go To", func(menu *ui.Menu) {
		if menu.Item("Find Usages").Shortcut(ui.Alt, ui.KeyF7).Chosen() {
			m.FindUsages()
		}
	})
	menu.Separator()
	if menu.Item("Reformat Code").Shortcut(ui.Cmd|ui.Alt, ui.KeyL).Chosen() {
		m.FormatActive()
	}
	if menu.Item("Rename…").Shortcut(ui.Shift, ui.KeyF6).Chosen() {
		m.RenameActive()
	}
}

// copyEditorText puts text on the clipboard and reports it in the status bar,
// as OpenIDEA shows its "Copied …" hint.
func (m *Model) copyEditorText(text string) {
	mygo.Clipboard.WriteText(text)
	m.status = "已复制 " + text
	m.invalidate()
}
