package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var clipboardWriteText = mygo.Clipboard.WriteText

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
	if m.springMenuAvailable(t) {
		menu.Separator()
		m.springEditorMenu(menu, t)
	}
	menu.Separator()
	menu.Submenu("Copy / Paste Special", func(menu *ui.Menu) {
		if menu.Item("Copy Path/Reference…").Shortcut(ui.Cmd|ui.Shift, ui.KeyC).Chosen() {
			m.OpenCopyPathReferencePopup()
		}
		if menu.Item("Paste from History…").Shortcut(ui.Cmd|ui.Shift, ui.KeyV).Chosen() {
			m.OpenPasteFromHistory()
		}
	})
	if menu.Item("Column Selection Mode").Checked(m.columnSelectionEnabled()).Chosen() {
		m.ToggleColumnSelection()
	}
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
	if isWebPath(path) {
		menu.Submenu("Browser", func(sub *ui.Menu) {
			if sub.Item("Default Browser").Chosen() {
				m.OpenWebInBrowser(path)
			}
			if isHTMLPath(path) &&
				sub.Item("Built-in Preview").Checked(m.remote.webPreviews[path]).Chosen() {
				m.ToggleWebPreview(path)
			}
		})
	}
	if isMarkdownPath(path) {
		mode := m.markdownModeFor(t)
		menu.Submenu("Markdown", func(sub *ui.Menu) {
			if sub.Item("Editor Only").Checked(mode == markdownEditorOnly).Chosen() {
				m.SetMarkdownMode(path, markdownEditorOnly)
			}
			if sub.Item("Editor and Preview").Checked(mode == markdownEditorPreview).Chosen() {
				m.SetMarkdownMode(path, markdownEditorPreview)
			}
			if sub.Item("Preview Only").Checked(mode == markdownPreviewOnly).Chosen() {
				m.SetMarkdownMode(path, markdownPreviewOnly)
			}
		})
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
	if menu.Item("Compare With…").Chosen() {
		m.PickCompareFile(t)
	}
	if menu.Item("Compare with Clipboard").Chosen() {
		m.CompareTabWithClipboard(t)
	}
	menu.Separator()
	if menu.Item("Reformat Code").Shortcut(ui.Cmd|ui.Alt, ui.KeyL).Chosen() {
		m.FormatActive()
	}
	if strings.HasSuffix(strings.ToLower(path), ".go") {
		menu.Submenu("Go Generate", func(sub *ui.Menu) {
			if sub.Item("Go Generate File").Chosen() {
				m.RunCommandWithArg("go.generate.file", map[string]any{"path": path})
			}
			if sub.Item("Go Generate Package").Chosen() {
				m.RunCommandWithArg("go.generate.package", map[string]any{"path": path})
			}
		})
	}
	if _, err := contextRunConfigFor(m.Root(), path, t.doc.Text(), m.run.configs); err == nil {
		if menu.Item("Modify Run Configuration…").Chosen() {
			m.RunCommandWithArg("run.modifyContext", map[string]any{
				"path": path, "content": t.doc.Text(),
			})
		}
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
	clipboardWriteText(text)
	m.status = "Copied " + text
	m.invalidate()
}
