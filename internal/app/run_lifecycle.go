package app

import (
	"context"
	"time"
)

const maxRetiredRunChildIDs = 4096

// requestRunningWorkTermination applies run.confirmTerminate before an action
// that leaves the current project or quits the window. The callback owns the
// action so cancelling the prompt leaves every live process untouched.
func (m *Model) requestRunningWorkTermination(proceed, cancel func()) {
	if proceed == nil {
		return
	}
	count := m.runningWorkCount()
	if count == 0 || !m.confirmTerminateEnabled() {
		proceed()
		return
	}
	m.requestRunTerminationCount(count, proceed, cancel)
}

func (m *Model) requestProjectLeave(proceed func()) {
	m.requestRunningWorkTermination(proceed, nil)
}

// retireRunChild closes the lifecycle gap around closeSessionTab and project
// switches. Late output and exits that still name the removed channel or run
// id are consumed without reviving a child or entering a new child's pending
// exit map.
func (m *Model) retireRunChild(child *runChild) {
	if child == nil {
		return
	}
	m.retireRunChildChannel(child.channel)
	m.retireRunChildRun(child.runID)
}

func (m *Model) retireRunChildChannel(channel uint32) {
	if channel == 0 {
		return
	}
	if m.run.retiredChildChannels == nil {
		m.run.retiredChildChannels = map[uint32]bool{}
	}
	if m.run.retiredChildChannels[channel] {
		return
	}
	m.run.retiredChildChannels[channel] = true
	m.run.retiredChildChannelOrder = append(m.run.retiredChildChannelOrder, channel)
	for len(m.run.retiredChildChannelOrder) > maxRetiredRunChildIDs {
		old := m.run.retiredChildChannelOrder[0]
		m.run.retiredChildChannelOrder = m.run.retiredChildChannelOrder[1:]
		delete(m.run.retiredChildChannels, old)
	}
}

func (m *Model) retireRunChildRun(runID uint32) {
	if runID == 0 {
		return
	}
	if m.run.retiredChildRuns == nil {
		m.run.retiredChildRuns = map[uint32]bool{}
	}
	if m.run.retiredChildRuns[runID] {
		return
	}
	m.run.retiredChildRuns[runID] = true
	m.run.retiredChildRunOrder = append(m.run.retiredChildRunOrder, runID)
	for len(m.run.retiredChildRunOrder) > maxRetiredRunChildIDs {
		old := m.run.retiredChildRunOrder[0]
		m.run.retiredChildRunOrder = m.run.retiredChildRunOrder[1:]
		delete(m.run.retiredChildRuns, old)
	}
}

func (m *Model) runChildChannelRetired(channel uint32) bool {
	return channel != 0 && m.run.retiredChildChannels[channel]
}

func (m *Model) runChildRunRetired(runID uint32) bool {
	return runID != 0 && m.run.retiredChildRuns[runID]
}

func (m *Model) runChildOwned(child *runChild) bool {
	if child == nil || child.root != m.Root() {
		return false
	}
	if child.service {
		return m.services.contains(child)
	}
	for _, candidate := range m.run.children {
		if candidate == child {
			return true
		}
	}
	return false
}

// resetRunLifecycle stops every process whose tab belongs to the departing
// project before the Run pane is replaced. The bounded retired ids survive the
// reset so callbacks already in flight cannot attach to the next project.
func (m *Model) resetRunLifecycle() {
	if m.run.retiredChildChannels == nil {
		m.run.retiredChildChannels = map[uint32]bool{}
	}
	if m.run.retiredChildRuns == nil {
		m.run.retiredChildRuns = map[uint32]bool{}
	}

	if m.run.testPlugin != nil {
		m.run.testPlugin.stop()
	}
	if m.run.sampleOutputCancel != nil {
		m.run.sampleOutputCancel()
	}
	m.markRunPluginStopped()
	m.stopPrimaryRunForProjectReset()

	for _, child := range append([]*runChild(nil), m.run.children...) {
		if child == nil {
			continue
		}
		m.retireRunChild(child)
		if child.live() {
			m.stopRunChild(child)
		}
	}
	for _, run := range append([]*agentRunSession(nil), m.run.agentRuns...) {
		if run == nil {
			continue
		}
		m.retireRunChildChannel(run.channel)
		m.retireRunChildRun(run.runID)
		m.closeAgentRun(run.id)
	}

	m.run = RunPane{
		retiredChildChannels:     m.run.retiredChildChannels,
		retiredChildChannelOrder: m.run.retiredChildChannelOrder,
		retiredChildRuns:         m.run.retiredChildRuns,
		retiredChildRunOrder:     m.run.retiredChildRunOrder,
	}
	m.chooser = nil
	m.rerunAsk = nil
	m.run.terminateAsk = nil
}

func (m *Model) stopPrimaryRunForProjectReset() {
	runID := m.run.runID
	if runID == 0 || m.mgr == nil && m.invokeHook == nil {
		return
	}
	go func(id uint32) {
		ctx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "run_stop", map[string]any{"id": id})
	}(runID)
}
