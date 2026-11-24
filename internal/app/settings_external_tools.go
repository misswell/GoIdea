package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
)

type externalToolMacro struct {
	name        string
	description string
}

var externalToolMacros = []externalToolMacro{
	{"$FilePath$", "Absolute path of the current file"},
	{"$FileDir$", "Directory of the current file"},
	{"$FileName$", "Name of the current file"},
	{"$FileNameWithoutExtension$", "File name without extension"},
	{"$FileExt$", "Extension of the current file"},
	{"$FileRelativePath$", "Path of the current file relative to the project"},
	{"$FileDirRelativeToProjectRoot$", "Directory of the current file relative to the project"},
	{"$ProjectFileDir$", "Project root directory"},
	{"$ProjectName$", "Project name"},
	{"$LineNumber$", "Caret line number (1-based)"},
	{"$ColumnNumber$", "Caret column number (1-based)"},
	{"$SelectedText$", "Text selected in the editor"},
	{"$Prompt$", "Asks for a value when the tool runs"},
}

func defaultExternalTool(name string) ExternalToolPrefs {
	return ExternalToolPrefs{
		ID:               adapter.UID(),
		Name:             name,
		Group:            "External Tools",
		Enabled:          true,
		WorkingDirectory: "$ProjectFileDir$",
		SynchronizeAfter: true,
		OpenConsole:      true,
	}
}

func (m *Model) ensureExternalToolSelection() {
	for _, tool := range m.settingsDraft.externalTools {
		if tool.ID == m.settingsExternalTool {
			return
		}
	}
	if len(m.settingsDraft.externalTools) == 0 {
		m.settingsExternalTool = ""
		return
	}
	m.settingsExternalTool = m.settingsDraft.externalTools[0].ID
}

func (m *Model) selectedExternalTool() int {
	for i := range m.settingsDraft.externalTools {
		if m.settingsDraft.externalTools[i].ID == m.settingsExternalTool {
			return i
		}
	}
	return -1
}

func (m *Model) addExternalTool(copyCurrent bool) {
	tools := m.settingsDraft.externalTools
	var tool ExternalToolPrefs
	if copyCurrent {
		if index := m.selectedExternalTool(); index >= 0 {
			tool = tools[index]
			tool.ID = adapter.UID()
			tool.Name = strings.TrimSpace(tool.Name + " (copy)")
		}
	}
	if tool.ID == "" {
		tool = defaultExternalTool(fmt.Sprintf("Tool %d", len(tools)+1))
	}
	m.settingsDraft.externalTools = append(tools, tool)
	m.settingsExternalTool = tool.ID
	m.invalidate()
}

func (m *Model) removeExternalTool() {
	index := m.selectedExternalTool()
	if index < 0 {
		return
	}
	tools := m.settingsDraft.externalTools
	m.settingsDraft.externalTools = append(tools[:index], tools[index+1:]...)
	if len(m.settingsDraft.externalTools) == 0 {
		m.settingsExternalTool = ""
	} else {
		next := min(index, len(m.settingsDraft.externalTools)-1)
		m.settingsExternalTool = m.settingsDraft.externalTools[next].ID
	}
	m.invalidate()
}

func (m *Model) moveExternalTool(delta int) {
	index := m.selectedExternalTool()
	next := index + delta
	if index < 0 || next < 0 || next >= len(m.settingsDraft.externalTools) {
		return
	}
	tools := m.settingsDraft.externalTools
	tools[index], tools[next] = tools[next], tools[index]
	m.invalidate()
}

func externalToolField(
	c *ui.Context,
	oi oiColors,
	label string,
	value *string,
	placeholder string,
	macros bool,
) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, label+":", 170)
		input := ui.TextInput(c, value).Grow(1)
		if placeholder != "" {
			input.Placeholder(placeholder)
		}
		if !macros {
			return
		}
		menuButton := ui.Button(c, "Insert Macro…")
		menuButton.Menu(func(menu *ui.Menu) {
			for _, macro := range externalToolMacros {
				macro := macro
				if menu.Item(macro.name + " — " + macro.description).Chosen() {
					if strings.TrimSpace(*value) != "" && !strings.HasSuffix(*value, " ") {
						*value += " "
					}
					*value += macro.name
				}
			}
		})
	})
}

// settingsExternalToolsBody is Tools › External Tools: the ordered table,
// enable switches and the selected tool's single-command configuration.
func (m *Model) settingsExternalToolsBody(c *ui.Context, oi oiColors) {
	m.ensureExternalToolSelection()
	tools := &m.settingsDraft.externalTools
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "＋").Tooltip("Add").OnClick(func() { m.addExternalTool(false) })
		remove := ui.Button(c, "−")
		remove.Tooltip("Remove")
		remove.Disabled(m.selectedExternalTool() < 0)
		remove.OnClick(m.removeExternalTool)
		copyButton := ui.Button(c, "Copy")
		copyButton.Disabled(m.selectedExternalTool() < 0)
		copyButton.OnClick(func() { m.addExternalTool(true) })
		up := ui.Button(c, "Up")
		up.Disabled(m.selectedExternalTool() <= 0)
		up.OnClick(func() { m.moveExternalTool(-1) })
		down := ui.Button(c, "Down")
		index := m.selectedExternalTool()
		down.Disabled(index < 0 || index >= len(*tools)-1)
		down.OnClick(func() { m.moveExternalTool(1) })
	})

	ui.Row(c).MinHeight(24).Gap(8).AlignItems(ui.Center).PaddingStart(4).Children(func() {
		settingsFormLabelDim(c, oi, "", 18)
		settingsFormLabelDim(c, oi, "Name", 220)
		settingsFormLabelDim(c, oi, "Group", 180)
	})
	ui.Scroll(c).Height(150).BorderWidth(1, 1, 1, 1).BorderColor(oi.border).Children(func() {
		ui.Column(c).Children(func() {
			if len(*tools) == 0 {
				ui.Text(c, "No external tools. Click + to add one.").FontSize(12).
					TextColor(oi.textMuted).Padding(8, 8)
				return
			}
			for i := range *tools {
				i := i
				tool := &(*tools)[i]
				row := ui.Row(c.Key("settings:external-tool:"+tool.ID)).MinHeight(26).
					Gap(8).AlignItems(ui.Center).Padding(0, 4)
				if tool.ID == m.settingsExternalTool {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.hover)
				}
				row.OnClick(func() {
					m.settingsExternalTool = tool.ID
					m.invalidate()
				})
				row.Children(func() {
					enabled := tool.Enabled
					if ui.Checkbox(c, &enabled, "").Changed() {
						tool.Enabled = enabled
					}
					ui.Text(c, tool.Name).FontSize(12).TextColor(oi.text).Width(220).SingleLine().Ellipsis("…")
					ui.Text(c, tool.Group).FontSize(12).TextColor(oi.textMuted).Width(180).SingleLine().Ellipsis("…")
				})
			}
		})
	})

	index := m.selectedExternalTool()
	if index < 0 {
		ui.Text(c, "Select a tool to edit its settings.").FontSize(12).
			TextColor(oi.textMuted).Padding(12, 0)
		return
	}
	tool := &(*tools)[index]
	ui.Row(c).Height(12)
	settingsSectionTitle(c, oi, "Tool Settings")
	externalToolField(c, oi, "Name", &tool.Name, "", false)
	externalToolField(c, oi, "Group", &tool.Group, "External Tools", false)
	externalToolField(c, oi, "Description", &tool.Description, "", false)
	externalToolField(c, oi, "Program", &tool.Program, "e.g. /usr/bin/env", true)
	externalToolField(c, oi, "Arguments", &tool.Arguments, "", true)
	externalToolField(c, oi, "Working directory", &tool.WorkingDirectory, "$ProjectFileDir$", true)
	ui.Row(c).MinHeight(28).AlignItems(ui.Center).PaddingStart(178).Children(func() {
		synchronize := tool.SynchronizeAfter
		if ui.Checkbox(c, &synchronize, "Synchronize files after execution").Changed() {
			tool.SynchronizeAfter = synchronize
		}
	})
	ui.Row(c).MinHeight(28).AlignItems(ui.Center).PaddingStart(178).Children(func() {
		openConsole := tool.OpenConsole
		if ui.Checkbox(c, &openConsole, "Open console for tool output").Changed() {
			tool.OpenConsole = openConsole
		}
	})
	ui.Text(c, "Enabled tools appear in Find Action under External Tools and can get shortcuts in Keymap. Output is shown in the Run tool window.").
		FontSize(12).TextColor(oi.textMuted).Padding(12, 182)
}
