package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

type runTerminateAsk struct {
	open      bool
	name      string
	count     int
	terminate func()
	cancel    func()
	dontAsk   bool
}

// requestRunTermination guards closing a live Run or Debug tab with
// run.confirmTerminate. The callback owns the actual stop and tab removal.
func (m *Model) requestRunTermination(name string, terminate func()) {
	m.requestRunTerminationCountWithName(name, 1, terminate, nil)
}

func (m *Model) requestRunTerminationCount(count int, terminate, cancel func()) {
	m.requestRunTerminationCountWithName("", count, terminate, cancel)
}

func (m *Model) requestRunTerminationCountWithName(
	name string,
	count int,
	terminate func(),
	cancel func(),
) {
	if terminate == nil {
		return
	}
	if m.run.terminateAsk != nil {
		return
	}
	if !m.confirmTerminateEnabled() {
		terminate()
		return
	}
	m.run.terminateAsk = &runTerminateAsk{
		open: true, name: name, count: max(count, 1), terminate: terminate, cancel: cancel,
	}
	m.invalidate()
}

func (m *Model) requestCloseRunChild(channel uint32) {
	child := m.childByChannel(channel)
	if child == nil {
		return
	}
	if !child.live() {
		m.closeRunChild(channel)
		m.hideRunWhenEmpty()
		return
	}
	m.requestRunTermination(child.name, func() {
		m.closeRunChild(channel)
		m.hideRunWhenEmpty()
	})
}

func (m *Model) requestCloseAgentRun(id uint64) {
	run := m.agentRunByID(id)
	if run == nil {
		return
	}
	if !run.live() {
		m.closeAgentRun(id)
		m.hideRunWhenEmpty()
		return
	}
	m.requestRunTermination(run.request.Name, func() {
		m.closeAgentRun(id)
		m.hideRunWhenEmpty()
	})
}

func (m *Model) requestCloseDebugSessionTab(id uint32) {
	name := "Debug"
	live := false
	if id == 0 && m.debug.starting {
		live = true
		if m.debug.tabName != "" {
			name = m.debug.tabName
		} else if m.debug.config != "" {
			name = m.debug.config
		}
	} else {
		for _, summary := range m.debug.debugSessionSummaries() {
			if summary.id != id {
				continue
			}
			name, live = summary.name, summary.live
			break
		}
	}
	if !live {
		m.closeDebugSessionTab(id)
		return
	}
	m.requestRunTermination(name, func() { m.closeDebugSessionTab(id) })
}

// ConfirmRunTerminate answers the close guard with Terminate.
func (m *Model) ConfirmRunTerminate() {
	ask := m.run.terminateAsk
	if ask == nil {
		return
	}
	m.run.terminateAsk = nil
	if ask.dontAsk {
		m.setRunRuntimeBool("run.confirmTerminate", false)
	}
	m.invalidate()
	ask.terminate()
}

// CancelRunTerminate leaves the process and its tab alone.
func (m *Model) CancelRunTerminate() {
	ask := m.run.terminateAsk
	m.run.terminateAsk = nil
	m.invalidate()
	if ask != nil && ask.cancel != nil {
		ask.cancel()
	}
}

func (m *Model) runTerminateConfirmDialog(c *ui.Context, p *ui.Theme) {
	ask := m.run.terminateAsk
	if ask == nil {
		return
	}
	open := ask.open
	modal := ui.Modal(c, &open, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			title := "A Process Is Running"
			body := "Do you want to terminate the running process?"
			if ask.name != "" {
				title = fmt.Sprintf("Process '%s' Is Running", ask.name)
				body = fmt.Sprintf("Do you want to terminate the process '%s'?", ask.name)
			}
			if ask.count > 1 {
				title = fmt.Sprintf("%d Processes Are Running", ask.count)
				body = "Do you want to terminate all running processes?"
			}
			ui.Text(c, title).
				FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, body).
				FontSize(12).TextColor(p.Text)
			dontAsk := ask.dontAsk
			if ui.Checkbox(c, &dontAsk, "Do not ask again").Changed() {
				ask.dontAsk = dontAsk
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Terminate").OnClick(m.ConfirmRunTerminate)
				ui.Button(c, "Cancel").OnClick(m.CancelRunTerminate)
			})
		})
	})
	if modal.Dismissed() || !open {
		m.CancelRunTerminate()
	}
}
