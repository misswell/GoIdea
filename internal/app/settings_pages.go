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
	case "todo":
		m.settingsTodoBody(c, oi)
	case "notifications":
		m.settingsNotificationsBody(c, oi)
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

// settingsFormLabelDim is a label whose control this state disables, as
// OpenIDEA dims Show tabs in when Tab placement is left, right or none.
func settingsFormLabelDim(c *ui.Context, oi oiColors, label string, width float32) {
	ui.Text(c, label).FontSize(12).TextColor(oi.textMuted).Width(width).
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
// file patterns, then Virtual Space and Save Files, in general.tsx's order.
// The Markdown Images, Mouse Control and Scrolling sections name features this
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
	ui.Row(c).Height(10)
	settingsSectionTitle(c, oi, "Save Files")
	trim := m.settingsDraft.trimTrailing
	if ui.Checkbox(c, &trim, "Remove trailing spaces on:").Changed() {
		m.settingsSetTrimTrailing(trim)
	}
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		shown := saveTrimLabel(m.settingsDraft.trimAllLines)
		sel := ui.Select(c, &shown, []string{"Modified lines", "All lines"}).
			Disabled(!m.settingsDraft.trimTrailing)
		if sel.Changed() {
			m.settingsSetTrimAllLines(shown == "All lines")
		}
	})
	blank := m.settingsDraft.dropTrailingBlankLines
	if ui.Checkbox(c, &blank, "Remove trailing blank lines at the end of saved files").Changed() {
		m.settingsSetDropTrailingBlankLines(blank)
	}
	final := m.settingsDraft.ensureFinalNewline
	if ui.Checkbox(c, &final, "Ensure every saved file ends with a line break").Changed() {
		m.settingsSetEnsureFinalNewline(final)
	}
}

// saveTrimLabel names the Remove trailing spaces choice as its select shows
// it.
func saveTrimLabel(all bool) string {
	if all {
		return "All lines"
	}
	return "Modified lines"
}

// settingsSectionTitle is a page section's heading, as the dialog draws the
// titles above groups of rows.
func settingsSectionTitle(c *ui.Context, oi oiColors, title string) {
	ui.Text(c, title).FontSize(12).TextColor(oi.textStrong).Padding(0, 0, 6, 0).SingleLine()
}

// settingsEditorAppearanceBody is Editor › General › Appearance: the caret,
// gutter and whitespace switches, then the distraction free text width, in the
// order general.tsx lists them.
func (m *Model) settingsEditorAppearanceBody(c *ui.Context, oi oiColors) {
	blockCaret := m.settingsDraft.blockCaret
	if ui.Checkbox(c, &blockCaret, "Use block caret").Changed() {
		m.settingsSetBlockCaret(blockCaret)
	}
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
// placement, its overflow and what a tab shows, then the tab order, the
// preview tab, the closing limit and the tab a close activates, in
// general.tsx's order.
func (m *Model) settingsEditorTabsBody(c *ui.Context, oi oiColors) {
	settingsSectionTitle(c, oi, "Appearance")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "Tab placement", 220)
		shown := tabPlacementLabel(m.settingsDraft.tabPlacement)
		sel := ui.Select(c, &shown, []string{"Top", "Bottom", "Left", "Right", "None"})
		if sel.Changed() {
			m.settingsSetTabPlacement(tabPlacementValue(shown))
		}
	})
	// A strip on the left or right is one scrolling column, as it is in
	// OpenIDEA, so Show tabs in does not apply to it.
	overflowOff := m.settingsDraft.tabPlacement == "left" ||
		m.settingsDraft.tabPlacement == "right" || m.settingsDraft.tabPlacement == "none"
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		if overflowOff {
			settingsFormLabelDim(c, oi, "Show tabs in:", 220)
		} else {
			settingsFormLabel(c, oi, "Show tabs in:", 220)
		}
	})
	oneRow := !m.settingsDraft.multipleRows
	ui.RadioGroup(c, func() {
		ui.Radio(c, &oneRow, true, "One row, and if tabs don't fit:")
		ui.Radio(c, &oneRow, false, "Multiple rows")
	}).PaddingStart(20).Disabled(overflowOff)
	if oneRow != !m.settingsDraft.multipleRows {
		m.settingsSetMultipleRows(!oneRow)
	}
	squeeze := m.settingsDraft.squeezeTabs
	ui.RadioGroup(c, func() {
		ui.Radio(c, &squeeze, false, "Scroll the tabs panel")
		ui.Radio(c, &squeeze, true, "Squeeze tabs")
	}).PaddingStart(48).Disabled(overflowOff || !oneRow)
	if squeeze != m.settingsDraft.squeezeTabs {
		m.settingsSetSqueezeTabs(squeeze)
	}
	pinnedRow := m.settingsDraft.pinnedSeparateRow
	if ui.Checkbox(c, &pinnedRow, "Show pinned tabs in a separate row").Changed() {
		m.settingsSetPinnedSeparateRow(pinnedRow)
	}
	fileIcon := m.settingsDraft.showFileIcon
	if ui.Checkbox(c, &fileIcon, "Show file icon").Changed() {
		m.settingsSetShowFileIcon(fileIcon)
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
	settingsSectionTitle(c, oi, "Opening Policy")
	previewTab := !m.settingsDraft.noPreviewTab
	if ui.Checkbox(c, &previewTab, "Enable preview tab").Changed() {
		m.settingsSetNoPreviewTab(!previewTab)
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
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabel(c, oi, "When the current tab is closed, activate", 220)
	})
	activate := m.settingsDraft.activateOnClose
	ui.RadioGroup(c, func() {
		ui.Radio(c, &activate, "left", "The tab on the left")
		ui.Radio(c, &activate, "right", "The tab on the right")
		ui.Radio(c, &activate, "mru", "Most recently opened tab")
	}).PaddingStart(20)
	if activate != m.settingsDraft.activateOnClose {
		m.settingsSetActivateOnClose(activate)
	}
	ui.Text(c, "Tabs with unsaved changes are never closed automatically.").
		FontSize(10.5).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
}

// tabPlacementLabel is the select's label for a stored placement.
func tabPlacementLabel(place string) string {
	switch place {
	case "bottom":
		return "Bottom"
	case "left":
		return "Left"
	case "right":
		return "Right"
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
	case "Left":
		return "left"
	case "Right":
		return "right"
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
	conflicts := m.KeymapConflicts()
	conflicted := map[string]bool{}
	for _, c := range conflicts {
		for _, id := range c.Actions {
			conflicted[id] = true
		}
	}
	if len(conflicts) > 0 {
		ui.Row(c).Padding(0, 0, 6, 0).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 个快捷键冲突；同一按键被两个 Action 使用。", len(conflicts))).
				FontSize(10.5).TextColor(oi.danger).SingleLine().Ellipsis("…")
		})
	}
	reset := ui.Button(c, "恢复默认键位").OnClick(func() {
		m.KeymapSetOverrides(nil)
		m.saveKeymapPrefs()
	})
	_ = reset
	for _, cmd := range commands() {
		bindings := m.KeymapBindings(cmd.id)
		if len(bindings) == 0 && cmd.shortcut == "" {
			continue
		}
		ui.Row(c.Key("keymap:"+cmd.id)).Height(24).Padding(0, 6).Gap(8).
			AlignItems(ui.Center).Children(func() {
			tone := oi.text
			if conflicted[cmd.id] {
				tone = oi.danger
			}
			ui.Text(c, cmd.title).FontSize(12).TextColor(tone).
				SingleLine().Ellipsis("…").Grow(1)
			label := strings.Join(bindings, " / ")
			if label == "" {
				label = "未绑定"
			}
			ui.Text(c, label).FontSize(10.5).TextColor(oi.textMuted).SingleLine()
			if _, custom := m.keymapUserBindings()[cmd.id]; custom {
				x := ui.ButtonBase(c.Key("keymap:clear:"+cmd.id)).Size(18, 18).Radius(4).
					Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if x.Hovered() {
					x.Background(oi.stripeHover)
				}
				x.Tooltip("Remove shortcut")
				x.Children(func() {
					ui.Text(c, "×").FontSize(12).TextColor(oi.textMuted).SingleLine()
				})
				x.OnClick(func() {
					overrides := map[string][]string{}
					for id, b := range m.keymapUserBindings() {
						overrides[id] = b
					}
					overrides[cmd.id] = []string{}
					m.KeymapSetOverrides(overrides)
					m.saveKeymapPrefs()
				})
			}
		})
	}
}

// saveKeymapPrefs writes the keymap overrides, when there is a settings file.
func (m *Model) saveKeymapPrefs() {
	if path := m.settingsWritePath(); path != "" {
		if err := m.prefs.Save(path); err != nil {
			m.notifyError("keymap:save", "Couldn't save the keymap", notifySuffix(err.Error()))
		}
	}
	m.invalidate()
}

// todoColorLabels are the colour names the TODO page's select offers, in the
// order OpenIDEA's TodoSettingsPage lists them.
var todoColorLabels = []string{"Blue", "Red", "Yellow", "Green", "Violet", "Grey"}

// todoColorLabel is the select's label for a stored tone.
func todoColorLabel(tone string) string {
	switch tone {
	case "danger":
		return "Red"
	case "warning":
		return "Yellow"
	case "success":
		return "Green"
	case "violet":
		return "Violet"
	case "muted":
		return "Grey"
	}
	return "Blue"
}

// todoColorValue is the stored tone for a select's label.
func todoColorValue(label string) string {
	switch label {
	case "Red":
		return "danger"
	case "Yellow":
		return "warning"
	case "Green":
		return "success"
	case "Violet":
		return "violet"
	case "Grey":
		return "muted"
	}
	return "info"
}

// todoToneOf is a pattern tone in the palette the page draws with.
func todoToneOf(tone string, oi oiColors) ui.Color {
	switch tone {
	case "danger":
		return oi.danger
	case "warning":
		return oi.warning
	case "success":
		return oi.success
	case "violet":
		return todoViolet
	case "muted":
		return oi.textMuted
	}
	return oi.accent
}

// settingsTodoBody is Editor › TODO: the matching switches and the keyword
// patterns, with their enable switches, colours and actions, as
// TodoSettingsPage.tsx draws them.
func (m *Model) settingsTodoBody(c *ui.Context, oi oiColors) {
	caseOn := m.settingsDraft.todoCaseSensitive
	if ui.Checkbox(c, &caseOn, "Case sensitive").Changed() {
		m.settingsSetTodoCase(caseOn)
	}
	comments := m.settingsDraft.todoCommentsOnly
	if ui.Checkbox(c, &comments, "Only in comments").Changed() {
		m.settingsSetTodoComments(comments)
	}
	ui.Row(c).Height(10)
	ui.Row(c).MinHeight(28).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Patterns").FontSize(12).TextColor(oi.textStrong).SingleLine().Grow(1)
		ui.Button(c, "Add Pattern").OnClick(m.settingsAddTodoPattern)
	})
	ui.Row(c).MinHeight(24).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		settingsFormLabelDim(c, oi, "", 18)
		settingsFormLabelDim(c, oi, "Keyword", 180)
		settingsFormLabelDim(c, oi, "", 14)
		settingsFormLabelDim(c, oi, "Color", 96)
	})
	for i := range m.settingsDraft.todoPatterns {
		ui.Row(c.Key(fmt.Sprintf("settings:todo:%d", i))).MinHeight(28).Gap(8).
			AlignItems(ui.Center).PaddingStart(20).Children(func() {
			enabled := m.settingsDraft.todoPatterns[i].Enabled
			if ui.Checkbox(c, &enabled, "").Changed() {
				m.settingsSetTodoEnabled(i, enabled)
			}
			keyword := m.settingsDraft.todoPatterns[i].Keyword
			in := ui.TextInput(c, &keyword).Width(180)
			if in.Changed() {
				m.settingsSetTodoKeyword(i, keyword)
			}
			ui.Row(c).Size(14, 14).Radius(3).
				Background(todoToneOf(m.settingsDraft.todoPatterns[i].Color, oi))
			shown := todoColorLabel(m.settingsDraft.todoPatterns[i].Color)
			sel := ui.Select(c, &shown, todoColorLabels)
			if sel.Changed() {
				m.settingsSetTodoColor(i, todoColorValue(shown))
			}
			ui.Button(c, "Remove").OnClick(func() { m.settingsRemoveTodoPattern(i) })
		})
	}
	ui.Row(c).Height(10)
	ui.Button(c, "Restore Defaults").OnClick(m.settingsRestoreTodoPatterns)
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

// settingsNotificationsBody is Appearance & Behavior › Notifications: the
// global balloon switch and each group's popup choice and log switch, as
// notificationPrefs() reads them.
func (m *Model) settingsNotificationsBody(c *ui.Context, oi oiColors) {
	balloons := m.settingsDraft.notifyBalloons
	if ui.Checkbox(c, &balloons, "Display balloon notifications").Changed() {
		m.settingsDraft.notifyBalloons = balloons
	}
	sound := m.settingsDraft.notifySound
	if ui.Checkbox(c, &sound, "Play sound").Changed() {
		m.settingsDraft.notifySound = sound
	}
	choices := []struct{ id, label string }{
		{"balloon", "Balloon"},
		{"sticky", "Sticky"},
		{"none", "No popup"},
	}
	for _, g := range notificationGroupTitles {
		g := g
		prefs := m.settingsDraft.notifyGroups[g.id]
		ui.Row(c).MaxWidth(720).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, g.title).FontSize(12).TextColor(oi.text).SingleLine()
			ui.Spacer(c)
			for _, opt := range choices {
				opt := opt
				on := prefs.Display == opt.id || (opt.id == "balloon" && prefs.Display == "")
				b := ui.ButtonBase(c.Key("settings:notify:"+g.id+":"+opt.id)).
					Height(20).Padding(0, 7).Radius(4).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if on {
					b.Background(oi.stripeSelected)
				} else if b.Hovered() {
					b.Background(oi.stripeHover)
				}
				b.Children(func() {
					tone := oi.textMuted
					if on {
						tone = oi.text
					}
					ui.Text(c, opt.label).FontSize(11).TextColor(tone).SingleLine()
				})
				b.OnClick(func() {
					p := m.settingsDraft.notifyGroups[g.id]
					p.Display = opt.id
					if p.Display == "balloon" {
						p.Display = ""
					}
					m.settingsDraft.notifyGroups[g.id] = p
					m.invalidate()
				})
			}
			log := !prefs.NoLog
			if ui.Checkbox(c, &log, "Log").Changed() {
				p := m.settingsDraft.notifyGroups[g.id]
				p.NoLog = !log
				m.settingsDraft.notifyGroups[g.id] = p
			}
		})
	}
}
