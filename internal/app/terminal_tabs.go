package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// termTabMenu mirrors OpenIDEA's terminal tab context menu.
func (m *Model) termTabMenu(menu *ui.Menu, group, tab int, anchor ui.Rect) {
	if group < 0 || group >= len(m.term.groups) {
		return
	}
	tabs := m.term.groups[group].tabs
	if tab < 0 || tab >= len(tabs) {
		return
	}
	session := &tabs[tab]
	others := len(tabs) > 1

	if menu.Item("Rename Session…").Chosen() {
		m.OpenTermRename(group, tab, anchor)
	}
	menu.Separator()
	if menu.Item("Close Tab").Chosen() {
		m.CloseTermTab(group, tab)
	}
	if menu.Item("Close Other Tabs").Disabled(!others).Chosen() {
		m.CloseOtherTermTabs(group, tab)
	}
	menu.Separator()
	if menu.Item("Move Left").Disabled(tab == 0).Chosen() {
		m.MoveTermTab(group, tab, -1)
	}
	if menu.Item("Move Right").Disabled(tab == len(tabs)-1).Chosen() {
		m.MoveTermTab(group, tab, 1)
	}
	menu.Separator()
	if menu.Item("Split Right").Chosen() {
		m.splitTerminalAction(fmt.Sprint(session.channel), "right", false)
	}
	if menu.Item("Split Down").Chosen() {
		m.splitTerminalAction(fmt.Sprint(session.channel), "down", false)
	}
	if menu.Item("Split and Move Right").Disabled(!others).Chosen() {
		m.MoveTermToSplit(session, "right")
	}
	if menu.Item("Split and Move Down").Disabled(!others).Chosen() {
		m.MoveTermToSplit(session, "down")
	}
	if m.term.split != "" && menu.Item("Unsplit").Chosen() {
		m.UnsplitTerm()
	}
}

// OpenTermRename asks for a custom tab name. The backend can later change the
// shell title without replacing this user choice.
func (m *Model) OpenTermRename(group, tab int, anchor ui.Rect) bool {
	if group < 0 || group >= len(m.term.groups) {
		return false
	}
	tabs := m.term.groups[group].tabs
	if tab < 0 || tab >= len(tabs) {
		return false
	}
	text := tabs[tab].customName
	if text == "" {
		text = tabs[tab].shell
	}
	m.term.rename = &termRenameState{
		group: group, tab: tab, text: text, anchor: anchor,
	}
	m.invalidate()
	return true
}

func (m *Model) dismissTermRename() {
	m.term.rename = nil
	m.invalidate()
}

// ConfirmTermRename applies the open Rename Session prompt.
func (m *Model) ConfirmTermRename() bool {
	state := m.term.rename
	if state == nil {
		return false
	}
	return m.RenameTermSession(state.group, state.tab, state.text)
}

// RenameTermSession sets a terminal tab's custom name.
func (m *Model) RenameTermSession(group, tab int, name string) bool {
	if group < 0 || group >= len(m.term.groups) {
		return false
	}
	tabs := m.term.groups[group].tabs
	if tab < 0 || tab >= len(tabs) {
		return false
	}
	tabs[tab].customName = strings.TrimSpace(name)
	m.term.groups[group].tabs = tabs
	m.term.rename = nil
	m.invalidate()
	return true
}

func (m *Model) termRenameDialog(c *ui.Context, p *ui.Theme) {
	state := m.term.rename
	if state == nil {
		return
	}
	if state.group < 0 || state.group >= len(m.term.groups) ||
		state.tab < 0 || state.tab >= len(m.term.groups[state.group].tabs) {
		m.term.rename = nil
		return
	}
	oi := oiPaletteOf(p)
	width, height := c.Size()
	w := max(float32(280), min(float32(380), width-16))
	h := float32(118)
	x := state.anchor.X
	y := state.anchor.Y + state.anchor.H + 4
	if y+h > height-8 {
		y = state.anchor.Y - h - 4
	}
	x = max(float32(8), min(x, width-w-8))
	y = max(float32(8), min(y, height-h-8))

	ui.Overlay(c, func() {
		box := ui.Column(c.Key("term-rename:popup")).Role(ui.RolePopup).
			Label("Rename Session").Focusable().AutoFocus().Bind(&state.focus).
			Absolute().Left(x).Top(y).Width(w).Height(h).Padding(12).Gap(10).
			Background(oi.popup).Border(1, oi.borderStrong).Radius(7).
			Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.3))
		box.Children(func() {
			ui.Text(c, "Rename Session").FontSize(13).Bold().
				TextColor(oi.textStrong).SingleLine()
			input := ui.TextInput(c.Key("term-rename:input"), &state.text).
				Grow(1).AutoFocus().Placeholder("Terminal tab name")
			if input.Submitted() {
				m.ConfirmTermRename()
			}
			ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.End).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "Cancel").Clicked() {
					m.dismissTermRename()
				}
				if ui.Button(c, "Rename").Clicked() {
					m.ConfirmTermRename()
				}
			})
		})
		switch {
		case box.PressedOutside() || box.OverlayShortcut(0, ui.KeyEscape):
			m.dismissTermRename()
		}
	})
}
