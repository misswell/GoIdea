package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// runAgentTabs draws OpenIDEA's agent Run sessions and the archive at the end
// of the strip. The archive is newest first and only contains finished agent
// commands, never a live process or another kind of Run session.
func (m *Model) runAgentTabs(c *ui.Context, p *ui.Theme) {
	visible := m.visibleAgentRuns()
	archive := m.archivedAgentRuns()
	if len(visible) == 0 && len(archive) == 0 {
		return
	}
	ui.ScrollHorizontal(c).Shrink(0).Children(func() {
		ui.Row(c).Height(30).Padding(0, 6).Gap(2).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			if m.run.channel != 0 || m.run.lastName != "" || m.run.running || m.run.awaiting {
				m.runMainTab(c, p)
			}
			for _, run := range visible {
				m.runAgentTab(c, p, run)
			}
			if len(archive) > 0 {
				m.runAgentArchive(c, p, archive)
			}
		})
	})
}

func (m *Model) runMainTab(c *ui.Context, p *ui.Theme) {
	active := m.run.agentActive == 0
	label := m.run.lastName
	if label == "" {
		label = "Run"
	}
	row := ui.Row(c.Key("run-main-tab")).Height(26).Padding(0, 8).
		Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if active {
		row.Background(p.SurfaceHover).BorderWidth(1, 1, 1, 1).BorderColor(p.Border)
	} else if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	row.OnClick(func() {
		m.run.agentActive = 0
		m.run.activeChild = 0
		m.invalidate()
	})
	row.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
		if m.run.running || m.run.awaiting {
			ui.Text(c, "●").FontSize(9).TextColor(p.Success)
		}
	})
}

func (m *Model) runAgentTab(c *ui.Context, p *ui.Theme, run *agentRunSession) {
	active := run.id == m.run.agentActive
	row := ui.Row(c.Key(fmt.Sprintf("run-agent-tab:%d", run.id))).
		Height(26).Padding(0, 8).Gap(6).Radius(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if active {
		row.Background(p.SurfaceHover).BorderWidth(1, 1, 1, 1).BorderColor(p.Border)
	} else if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	row.OnClick(func() { m.selectAgentRun(run.id) })
	row.Children(func() {
		ui.Text(c, run.request.Name).FontSize(12).TextColor(p.Text).
			SingleLine().Ellipsis("…")
		switch {
		case run.live():
			ui.Text(c, "●").FontSize(9).TextColor(p.Success)
		case run.exitCode != nil && *run.exitCode == 0 && run.err == "":
			ui.Text(c, "✓").FontSize(10).TextColor(p.Success)
		case run.err != "" || run.exitCode != nil:
			ui.Text(c, "!").FontSize(10).Bold().TextColor(p.Danger)
		}
		closeButton := ui.ButtonBase(c.Key(fmt.Sprintf("run-agent-tab-close:%d", run.id))).
			Size(18, 18).Justify(ui.Center).AlignItems(ui.Center).
			Radius(4).Cursor(ui.CursorPointer)
		if closeButton.Hovered() {
			closeButton.Background(p.Surface)
		}
		closeButton.Tooltip("Close")
		closeButton.Children(func() {
			ui.Text(c, "×").FontSize(12).TextColor(p.TextMuted)
		})
		if closeButton.Clicked() {
			m.requestCloseAgentRun(run.id)
		}
	})
}

func (m *Model) runAgentArchive(c *ui.Context, p *ui.Theme, archive []*agentRunSession) {
	button := ui.ButtonBase(c.Key("run-agent-archive")).
		Height(26).Padding(0, 8).Gap(4).Radius(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(p.SurfaceHover)
	}
	button.Tooltip("Archived agent tabs")
	button.Children(func() {
		ui.Text(c, fmt.Sprintf("Archived (%d)", len(archive))).
			FontSize(12).TextColor(p.TextMuted).SingleLine()
	})
	button.Menu(func(menu *ui.Menu) {
		for _, run := range archive {
			item := menu.Item(run.request.Name)
			item.Checked(run.id == m.run.agentActive)
			if item.Chosen() {
				m.selectAgentRun(run.id)
			}
		}
		menu.Separator()
		if menu.Item("Close All Archived Tabs").Chosen() {
			m.closeAgentArchive()
		}
	})
}

func (m *Model) runAgentHeader(c *ui.Context, p *ui.Theme, run *agentRunSession) {
	ui.Row(c).Height(30).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "Run").FontSize(12).Bold().TextColor(p.Text)
		ui.Text(c, "Agent · "+run.request.Name).FontSize(12).TextColor(p.Text).
			SingleLine().Ellipsis("…").Grow(1)
		if run.live() {
			ui.Button(c, "Stop").OnClick(func() { m.stopActiveAgentRun() })
		} else {
			ui.Button(c, "Rerun").OnClick(func() { m.rerunActiveAgentRun() })
		}
		ui.Button(c, "Clear").OnClick(func() { m.clearActiveAgentRun() })
		ui.Button(c, "Close").OnClick(func() { m.requestCloseAgentRun(run.id) })
		ui.Button(c, "Collapse").OnClick(func() { m.ShowRun() })
	})
}
