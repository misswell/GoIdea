package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// runChildTabs draws OpenIDEA's Run tool-window content tabs. A normal single
// run keeps its one title tab. Compound and before-launch sessions each keep
// their own console, test tree and lifecycle.
func (m *Model) runChildTabs(c *ui.Context, p *ui.Theme) {
	if len(m.run.children) == 0 {
		return
	}
	ui.ScrollHorizontal(c).Shrink(0).Children(func() {
		ui.Row(c).Height(30).Padding(0, 6).Gap(2).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			if m.run.channel != 0 || m.run.lastName != "" {
				m.runChildTab(c, p, 0, m.run.lastName, m.run.running || m.run.awaiting, m.run.err != "")
			}
			for _, child := range m.run.children {
				child := child
				mark := ""
				switch {
				case child.err != "":
					mark = "  !"
				case child.starting:
					mark = "  …"
				case child.running:
					mark = "  ●"
				case child.exitCode != nil && *child.exitCode == 0:
					mark = "  ✓"
				case child.finished:
					mark = "  ×"
				}
				m.runChildTab(c, p, child.channel, child.name+mark, child.running || child.starting, child.err != "")
			}
		})
	})
}

func (m *Model) runChildTab(
	c *ui.Context,
	p *ui.Theme,
	channel uint32,
	label string,
	live bool,
	failed bool,
) {
	if label == "" {
		label = "Run"
	}
	active := m.run.activeChild == channel
	row := ui.Row(c.Key(fmt.Sprintf("run-tab:%d", channel))).Height(26).Padding(0, 8).Gap(6).
		Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if active {
		row.Background(p.SurfaceHover).BorderWidth(1, 1, 1, 1).BorderColor(p.Border)
	} else if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	row.OnClick(func() { m.selectRunChild(channel) })
	row.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
		if live {
			ui.Text(c, "●").FontSize(9).TextColor(p.Success)
		} else if failed {
			ui.Text(c, "!").FontSize(10).Bold().TextColor(p.Danger)
		}
		if channel != 0 {
			closeButton := ui.ButtonBase(c.Key(fmt.Sprintf("run-tab-close:%d", channel))).
				Size(18, 18).Justify(ui.Center).AlignItems(ui.Center).Radius(4).
				Cursor(ui.CursorPointer)
			if closeButton.Hovered() {
				closeButton.Background(p.Surface)
			}
			closeButton.Tooltip("Close")
			closeButton.Children(func() { ui.Text(c, "×").FontSize(12).TextColor(p.TextMuted) })
			if closeButton.Clicked() {
				m.requestCloseRunChild(channel)
			}
		}
	})
}
