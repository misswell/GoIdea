package app

// toolWindows lists the bottom tool windows in the order OpenIDEA registers
// them. The shortcuts activate one of these and the Hide All action walks the
// list, as toolwindows/store.ts does for its slots.
var toolWindows = []string{"run", "debug", "build", "term", "log", "problems"}

// toolVisible reports whether a bottom tool window is showing.
func (m *Model) toolVisible(name string) bool {
	switch name {
	case "run":
		return m.run.visible
	case "debug":
		return m.debug.visible
	case "build":
		return m.build.visible
	case "term":
		return m.term.visible
	case "log":
		return m.log.visible
	case "problems":
		return m.problems.visible
	}
	return false
}

// hideTool closes a bottom tool window without the loading side effects of its
// toggle action. The last-used mark stays, as OpenIDEA's lastUsed survives a
// hide for F12.
func (m *Model) hideTool(name string) {
	switch name {
	case "run":
		m.run.visible = false
	case "debug":
		m.debug.visible = false
	case "build":
		m.build.visible = false
	case "term":
		m.term.visible = false
	case "log":
		m.log.visible = false
	case "problems":
		m.problems.visible = false
	}
}

// occupyBottom makes name the bottom slot's window and closes the others: an
// OpenIDEA slot shows one tool window at a time, so opening a window takes the
// area from the one that held it. The closed window keeps its contents, as a
// tab does: the terminal's shell and a run's output are still there.
func (m *Model) occupyBottom(name string) {
	for _, other := range toolWindows {
		if other != name {
			m.hideTool(other)
		}
	}
	m.tool = name
}

// visibleTool is the window the bottom slot shows, "" when the slot is closed.
func (m *Model) visibleTool() string {
	for _, name := range toolWindows {
		if m.toolVisible(name) {
			return name
		}
	}
	return ""
}

// activateTool opens the named bottom tool window and remembers it as the one
// the shortcuts act on, as OpenIDEA's showToolWindow does.
func (m *Model) activateTool(name string) {
	if m.toolVisible(name) {
		m.tool = name
		m.invalidate()
		return
	}
	switch name {
	case "run":
		m.ShowRun()
	case "debug":
		m.ShowDebug()
	case "build":
		m.ShowBuild()
	case "term":
		m.ShowTerm()
	case "log":
		m.ShowLog()
	case "problems":
		m.ShowProblems()
	}
}

// HideActiveToolWindow is OpenIDEA's Shift+Escape: the tool window the
// shortcuts last showed goes away and the editor keeps the keyboard.
func (m *Model) HideActiveToolWindow() {
	if m.tool == "" || !m.toolVisible(m.tool) {
		return
	}
	m.hideTool(m.tool)
	m.invalidate()
}

// HideAllToolWindows is OpenIDEA's Cmd+Shift+F12: it hides every bottom tool
// window, and a second press brings back exactly what it hid.
func (m *Model) HideAllToolWindows() {
	any := false
	for _, name := range toolWindows {
		if m.toolVisible(name) {
			any = true
			break
		}
	}
	if any {
		m.toolStash = make(map[string]bool, len(toolWindows))
		for _, name := range toolWindows {
			m.toolStash[name] = m.toolVisible(name)
			m.hideTool(name)
		}
	} else if m.toolStash != nil {
		stash := m.toolStash
		m.toolStash = nil
		for _, name := range toolWindows {
			if stash[name] {
				m.activateTool(name)
			}
		}
	}
	m.invalidate()
}

// JumpToLastToolWindow is OpenIDEA's F12: the bottom tool window used last
// comes back; without one, the project tree's window does.
func (m *Model) JumpToLastToolWindow() {
	if m.tool == "" {
		m.ShowProject()
		return
	}
	m.activateTool(m.tool)
}
