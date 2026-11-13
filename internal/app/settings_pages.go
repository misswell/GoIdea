package app

// The pages the settings dialog shows. Each one draws only the options this
// build really implements, with OpenIDEA's labels and groups: the theme and
// its Sync with OS switch, the editor font size, the distraction free text
// width, the keymap this build carries, and the Java toolchain.

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

// settingsPageBody draws the selected node: a group folder's description and
// links, or the page's section.
func (m *Model) settingsPageBody(c *ui.Context, p *ui.Theme, oi oiColors, node settingsNode) {
	if node.page == nil {
		m.settingsGroupPage(c, oi, node)
		return
	}
	switch node.page.section {
	case "appearance":
		m.settingsAppearanceBody(c, oi)
	case "editorAppearance":
		m.settingsEditorAppearanceBody(c, oi)
	case "general":
		m.settingsGeneralBody(c, oi)
	case "breadcrumbs":
		m.settingsBreadcrumbsBody(c, oi)
	case "editorTabs":
		m.settingsEditorTabsBody(c, oi)
	case "codeFolding":
		m.settingsCodeFoldingBody(c)
	case "editorFont":
		m.settingsFontBody(c, oi)
	case "keymap":
		m.settingsKeymapBody(c, oi)
	case "java":
		m.javaToolsSection(c, p)
	}
}

// settingsGroupPage is a group folder: the description above the links to the
// pages in it, indented as GROUP_DESCRIPTIONS and groupLinks are.
func (m *Model) settingsGroupPage(c *ui.Context, oi oiColors, node settingsNode) {
	if description := settingsGroupDescriptions[node.title]; description != "" {
		ui.Row(c).MaxWidth(720).Padding(0, 0, 14, 0).Children(func() {
			ui.Text(c, description).FontSize(13).TextColor(oi.text)
		})
	}
	ui.Column(c).Gap(2).PaddingStart(32).Children(func() {
		for _, child := range node.children {
			key := child.key
			link := ui.Row(c.Key("settings:link:" + key)).Height(20).AlignItems(ui.Center)
			link.Children(func() {
				ui.Text(c, child.title).FontSize(12).TextColor(oi.link).SingleLine()
			})
			if link.Clicked() {
				m.settingsSelectPage(key)
			}
		}
	})
}

// settingsFormLabel is a settings row's label, in the column the rows share.
func settingsFormLabel(c *ui.Context, oi oiColors, label string, width float32) {
	ui.Text(c, label).FontSize(12).TextColor(oi.text).Width(width).
		SingleLine().Ellipsis("…")
}

// settingsAppearanceBody is Appearance's Theme row: the theme select and the
// Sync with OS switch, which follows the system's appearance when it is on.
func (m *Model) settingsAppearanceBody(c *ui.Context, oi oiColors) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Theme:", 220)
		shown := "Light"
		if m.settingsDraft.theme == "dark" {
			shown = "Dark"
		}
		sync := m.settingsDraft.theme == ""
		sel := ui.Select(c, &shown, []string{"Light", "Dark"}).Disabled(sync)
		if sel.Changed() {
			m.settingsSetTheme(strings.ToLower(shown))
		}
		switchOn := sync
		chk := ui.Checkbox(c, &switchOn, "Sync with OS")
		if chk.Changed() {
			if switchOn {
				m.settingsSetTheme("")
			} else {
				m.settingsSetTheme("light")
			}
		}
	})
}

// settingsFontBody is Editor › Font's Size row, in pixels.
func (m *Model) settingsFontBody(c *ui.Context, oi oiColors) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Size:", 220)
		size := float64(m.settingsDraft.fontSize)
		in := ui.NumberInput(c, &size, minFontSize, maxFontSize, 1).Width(100)
		if in.Changed() {
			m.settingsSetFont(float32(math.Round(size)))
		}
		ui.Text(c, "px").FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
}

// settingsGeneralBody is Editor › General: the Soft Wraps section with its
// file patterns, then Virtual Space, in general.tsx's order. The Markdown
// Images, Mouse Control, Scrolling and Save Files sections name features this
// build does not carry, so they are not drawn.
func (m *Model) settingsGeneralBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Soft Wraps")
	wrap := m.settingsDraft.softWrap
	if ui.Checkbox(c, &wrap, "Soft-wrap files").Changed() {
		m.settingsSetSoftWrap(wrap)
	}
	ui.Row(c).MinHeight(28).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "File patterns", 110)
		in := ui.TextInput(c, &m.settingsDraft.softWrapFiles).Grow(1).
			Disabled(!m.settingsDraft.softWrap)
		if in.Changed() {
			m.settingsPreview()
		}
	})
	ui.Text(c, "Use ; to separate patterns. * matches every file.").
		FontSize(10.5).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Virtual Space")
	end := m.settingsDraft.scrollPastEnd
	if ui.Checkbox(c, &end, "Show virtual space at file bottom").Changed() {
		m.settingsSetScrollPastEnd(end)
	}
}

// settingsSectionTitle is a page section's heading, as the dialog draws the
// titles above groups of rows.
func settingsSectionTitle(c *ui.Context, oi oiColors, title string) {
	ui.Text(c, title).FontSize(12).TextColor(oi.textStrong).Padding(0, 0, 6, 0).SingleLine()
}

// settingsEditorAppearanceBody is Editor › General › Appearance: the gutter
// and whitespace switches, then the distraction free text width, in the order
// general.tsx lists them.
func (m *Model) settingsEditorAppearanceBody(c *ui.Context, oi oiColors) {
	lineNumbers := m.settingsDraft.lineNumbers
	if ui.Checkbox(c, &lineNumbers, "Show line numbers").Changed() {
		m.settingsSetLineNumbers(lineNumbers)
	}
	showWhitespace := m.settingsDraft.showWhitespace
	if ui.Checkbox(c, &showWhitespace, "Show whitespaces").Changed() {
		m.settingsSetShowWhitespace(showWhitespace)
	}
	indentGuides := m.settingsDraft.indentGuides
	if ui.Checkbox(c, &indentGuides, "Show indent guides").Changed() {
		m.settingsSetIndentGuides(indentGuides)
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		settingsFormLabel(c, oi, "Distraction free mode text width:", 260)
		width := float64(m.settingsDraft.zenWidth)
		in := ui.NumberInput(c, &width, minZenWidth, maxZenWidth, 1).Width(100)
		if in.Changed() {
			m.settingsSetZenWidth(int(math.Round(width)))
		}
		ui.Text(c, "chars").FontSize(12).TextColor(oi.textMuted).SingleLine()
	})
}

// settingsBreadcrumbsBody is Editor › General › Breadcrumbs: the Show
// breadcrumbs switch with its Top / Bottom placement radios, as general.tsx
// draws them. The languages chooser below it lists language settings this
// build does not carry, so it is not drawn.
func (m *Model) settingsBreadcrumbsBody(c *ui.Context, oi oiColors) {
	show := m.settingsDraft.showBreadcrumbs
	if ui.Checkbox(c, &show, "Show breadcrumbs:").Changed() {
		m.settingsSetShowBreadcrumbs(show)
	}
	place := m.settingsDraft.breadcrumbPlacement
	ui.RadioGroup(c, func() {
		ui.Radio(c, &place, "top", "Top")
		ui.Radio(c, &place, "bottom", "Bottom")
	}).PaddingStart(20).Disabled(!m.settingsDraft.showBreadcrumbs)
	if place != m.settingsDraft.breadcrumbPlacement {
		m.settingsSetBreadcrumbPlacement(place)
	}
}

// settingsEditorTabsBody is Editor › General › Editor Tabs: the strip's
// placement and what a tab shows, then the tab order and the closing limit, in
// general.tsx's order. "Show tabs in", the file icon, the preview tab and the
// closing-policy radios name behaviour this build does not carry, so they are
// not drawn.
func (m *Model) settingsEditorTabsBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Appearance")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Tab placement", 220)
		shown := tabPlacementLabel(m.settingsDraft.tabPlacement)
		sel := ui.Select(c, &shown, []string{"Top", "Bottom", "None"})
		if sel.Changed() {
			m.settingsSetTabPlacement(tabPlacementValue(shown))
		}
	})
	pinnedRow := m.settingsDraft.pinnedSeparateRow
	if ui.Checkbox(c, &pinnedRow, "Show pinned tabs in a separate row").Changed() {
		m.settingsSetPinnedSeparateRow(pinnedRow)
	}
	extension := m.settingsDraft.showExtension
	if ui.Checkbox(c, &extension, "Show file extension").Changed() {
		m.settingsSetShowExtension(extension)
	}
	directory := m.settingsDraft.showDirectoryForNonUnique
	if ui.Checkbox(c, &directory, "Show directory for non-unique file names").Changed() {
		m.settingsSetShowDirectoryForNonUnique(directory)
	}
	modified := m.settingsDraft.markModified
	if ui.Checkbox(c, &modified, "Mark modified (*)").Changed() {
		m.settingsSetMarkModified(modified)
	}
	fullPath := m.settingsDraft.showFullPathOnHover
	if ui.Checkbox(c, &fullPath, "Show full path on mouse hover").Changed() {
		m.settingsSetShowFullPathOnHover(fullPath)
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Close button position:", 220)
		shown := closePositionLabel(m.settingsDraft.closePosition)
		sel := ui.Select(c, &shown, []string{"Right", "Left", "None"})
		if sel.Changed() {
			m.settingsSetClosePosition(closePositionValue(shown))
		}
	})
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Tab Order")
	sortTabs := m.settingsDraft.sortAlphabetically
	if ui.Checkbox(c, &sortTabs, "Sort tabs alphabetically").Changed() {
		m.settingsSetSortTabs(sortTabs)
	}
	atEnd := m.settingsDraft.openAtEnd
	if ui.Checkbox(c, &atEnd, "Open new tabs at the end").Changed() {
		m.settingsSetOpenAtEnd(atEnd)
	}
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Closing Policy")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Tab limit", 220)
		limit := float64(m.settingsDraft.tabLimit)
		in := ui.NumberInput(c, &limit, 1, maxTabLimit, 1).Width(100)
		if in.Changed() {
			m.settingsSetTabLimit(int(math.Round(limit)))
		}
	})
	ui.Text(c, "Tabs with unsaved changes are never closed automatically.").
		FontSize(10.5).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
}

// tabPlacementLabel is the select's label for a stored placement.
func tabPlacementLabel(place string) string {
	switch place {
	case "bottom":
		return "Bottom"
	case "none":
		return "None"
	}
	return "Top"
}

// tabPlacementValue is the staged placement for a select's label.
func tabPlacementValue(label string) string {
	switch label {
	case "Bottom":
		return "bottom"
	case "None":
		return "none"
	}
	return "top"
}

// closePositionLabel is the select's label for a stored close position.
func closePositionLabel(place string) string {
	switch place {
	case "left":
		return "Left"
	case "none":
		return "None"
	}
	return "Right"
}

// closePositionValue is the staged close position for a select's label.
func closePositionValue(label string) string {
	switch label {
	case "Left":
		return "left"
	case "None":
		return "none"
	}
	return "right"
}

// settingsCodeFoldingBody is Code Folding's Show code folding outline switch.
// The editor folds from the gutter only, so the Fold by default types are not
// drawn.
func (m *Model) settingsCodeFoldingBody(c *ui.Context) {
	folding := m.settingsDraft.showFolding
	if ui.Checkbox(c, &folding, "Show code folding outline").Changed() {
		m.settingsSetShowFolding(folding)
	}
}

// settingsKeymapBody is the Keymap page: the map in use and the actions this
// build carries with their keys.
func (m *Model) settingsKeymapBody(c *ui.Context, oi oiColors) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Keymap:").FontSize(12).TextColor(oi.text).SingleLine()
		ui.Text(c, keymapPresetTitle).FontSize(12).TextColor(oi.textStrong).SingleLine()
	})
	ui.Row(c).Padding(0, 0, 6, 0).Children(func() {
		ui.Text(c, "The actions this build carries, with the keys of the IntelliJ macOS map.").
			FontSize(10.5).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	})
	for _, cmd := range commands() {
		if cmd.shortcut == "" {
			continue
		}
		ui.Row(c.Key("keymap:"+cmd.id)).Height(24).Padding(0, 6).Gap(8).
			AlignItems(ui.Center).Children(func() {
			ui.Text(c, cmd.title).FontSize(12).TextColor(oi.text).
				SingleLine().Ellipsis("…").Grow(1)
			ui.Text(c, cmd.shortcut).FontSize(10.5).TextColor(oi.textMuted).SingleLine()
		})
	}
}

// settingsPageSummary is the settings tree as text, for the offscreen smoke
// run: each visible row, indented, with the selected and staged marks.
func (m *Model) settingsPageSummary() []string {
	var out []string
	for _, row := range m.settingsVisibleRows() {
		mark := " "
		if row.node.key == m.settingsPage {
			mark = "*"
		}
		staged := " "
		if m.settingsDirtyPage(row.node.key) {
			staged = "!"
		}
		out = append(out, fmt.Sprintf("%s%s%s %s", mark, staged,
			strings.Repeat("  ", row.depth), row.node.title))
	}
	return out
}
