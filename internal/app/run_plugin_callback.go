package app

// run.pluginStart owns one completion callback for the currently active run.
// RunPane has one primary session, so generation, project root and config id
// are enough to reject callbacks from an older launch without adding another
// global session registry.
func (m *Model) beginRunPlugin(
	configID string,
	onExit func(code *int32, stopped bool),
) (generation uint64) {
	m.run.pluginGeneration++
	m.run.pluginExit = onExit
	m.run.pluginStopped = false
	m.run.pluginRoot = m.Root()
	m.run.pluginConfigID = configID
	return m.run.pluginGeneration
}

func (m *Model) finishRunPlugin(code *int32) {
	m.finishRunPluginGeneration(0, code)
}

func (m *Model) finishRunPluginGeneration(generation uint64, code *int32) {
	if generation != 0 && generation != m.run.pluginGeneration {
		return
	}
	done := m.run.pluginExit
	if done == nil {
		return
	}
	if m.Root() != m.run.pluginRoot {
		m.cancelRunPlugin()
		return
	}
	stopped := m.run.pluginStopped
	m.run.pluginExit = nil
	m.run.pluginStopped = false
	m.run.pluginRoot = ""
	m.run.pluginConfigID = ""
	done(code, stopped)
}

func (m *Model) failRunPlugin() {
	m.failRunPluginGeneration(0)
}

func (m *Model) failRunPluginGeneration(generation uint64) {
	m.finishRunPluginGeneration(generation, nil)
}

func (m *Model) markRunPluginStopped() {
	if m.run.pluginExit != nil {
		m.run.pluginStopped = true
	}
}

func (m *Model) cancelRunPlugin() {
	m.cancelRunPluginGeneration(0)
}

func (m *Model) cancelRunPluginGeneration(generation uint64) {
	if generation != 0 && generation != m.run.pluginGeneration {
		return
	}
	m.run.pluginExit = nil
	m.run.pluginStopped = false
	m.run.pluginRoot = ""
	m.run.pluginConfigID = ""
}
