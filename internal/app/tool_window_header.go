package app

// Shared tool-window chrome after OpenIDEA's HeaderRow: a 40 DIP title row,
// optional content tabs, fading corner actions, double-click maximize and an
// Options menu with Move To, Maximize / Restore and Hide.

import "github.com/egoist/mygo/ui"

func (m *Model) toolWindowHeader(
	c *ui.Context,
	p *ui.Theme,
	oi oiColors,
	id string,
	content func(),
) {
	spec := dockSpec(id)
	if spec == nil {
		return
	}
	header := ui.Row(c).Key("toolwindow:header:"+id).Height(oiHeaderHeight).
		Shrink(0).Padding(0, 6, 0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border)
	if header.DoubleClicked() {
		m.toggleToolWindowMaximized()
	}
	header.Children(func() {
		ui.Text(c, spec.title).FontSize(13).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…")
		if content != nil {
			ui.Row(c).Grow(1).MinWidth(0).Margin(6, 0, 0, 0).
				AlignItems(ui.Center).Children(content)
		} else {
			ui.Spacer(c)
		}
		actions := ui.Row(c).Shrink(0).Gap(2).AlignItems(ui.Center)
		if header.Hovered() || header.FocusWithin() {
			actions.Opacity(1)
		} else {
			actions.Opacity(0.52)
		}
		actions.Children(func() {
			more := ui.ButtonBase(c.Key("toolwindow:options:"+id)).Size(22, 22).
				Radius(4).Justify(ui.Center).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Tooltip("Options").Label("Options")
			if more.Hovered() || more.Focused() {
				more.Background(oi.toolbarHover)
			}
			more.Children(func() { oiIconEl(c, "general/moreVertical", 16, oi, oi.icon) })
			more.Menu(func(menu *ui.Menu) { m.toolWindowOptionsMenu(menu, id) })

			hideTip := "Hide " + spec.title + " Tool Window"
			hide := ui.ButtonBase(c.Key("toolwindow:hide:"+id)).Size(22, 22).
				Radius(4).Justify(ui.Center).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Tooltip(hideTip).Label(hideTip)
			if hide.Hovered() || hide.Focused() {
				hide.Background(oi.toolbarHover)
			}
			hide.Children(func() { oiIconEl(c, "general/hide", 16, oi, oi.icon) })
			if hide.Clicked() {
				m.DockHide(id)
			}
		})
	})
}

func (m *Model) toggleToolWindowMaximized() {
	m.maximized = !m.maximized
	m.rememberLayout()
	m.invalidate()
}

func (m *Model) toolWindowOptionsMenu(menu *ui.Menu, id string) {
	spec := dockSpec(id)
	if spec == nil {
		return
	}
	for _, slot := range []dockSlot{dockLeft, dockLeftSplit, dockRight, dockRightSplit, dockBottom, dockBottomSplit} {
		target := slot
		label := "Move To " + dockSlotLabel(target)
		if from, ok := m.dockSlotOf(id); ok && from == target {
			label += " (Current)"
		}
		if menu.Item(label).Chosen() {
			m.DockMove(id, target)
			return
		}
	}
	menu.Separator()
	maximize := "Maximize"
	if m.maximized {
		maximize = "Restore Size"
	}
	if menu.Item(maximize).Chosen() {
		m.toggleToolWindowMaximized()
	}
	if menu.Item("Hide").Chosen() {
		m.DockHide(id)
	}
}
