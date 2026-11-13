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
// hide for F12. Closing the slot's last window also gives the workbench back.
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
	if m.maximized && m.visibleTool() == "" {
		m.maximized = false
	}
}

// occupyBottom makes name the window its area shows and closes the others in
// that area: an OpenIDEA area shows one tool window at a time, so opening a
// window takes the area from the one that held it. The other area keeps its
// window. The closed window keeps its contents, as a tab does: the terminal's
// shell and a run's output are still there.
func (m *Model) occupyBottom(name string) {
	area := m.toolArea(name)
	for _, other := range toolWindows {
		if other != name && m.toolArea(other) == area {
			m.hideTool(other)
		}
	}
	m.tool = name
}

// toolArea is the bottom side area a window sits in, "primary" or "split".
func (m *Model) toolArea(name string) string {
	if m.splitNames[name] {
		return "split"
	}
	return "primary"
}

// primaryTool is the window the bottom primary area shows, "" when it is empty.
func (m *Model) primaryTool() string {
	for _, name := range toolWindows {
		if m.toolArea(name) == "primary" && m.toolVisible(name) {
			return name
		}
	}
	return ""
}

// splitTool is the window the bottom split area shows, "" when it is empty.
func (m *Model) splitTool() string {
	for _, name := range toolWindows {
		if m.toolArea(name) == "split" && m.toolVisible(name) {
			return name
		}
	}
	return ""
}

// MoveActiveToolWindowToSplit moves the window the shortcuts last showed into
// the bottom split area, as OpenIDEA's Move to › Bottom Right does; the primary
// area keeps its window. MoveActiveToolWindowToBottom moves it back.
func (m *Model) MoveActiveToolWindowToSplit() { m.moveActiveTool(true) }

// MoveActiveToolWindowToBottom moves the active window to the primary area.
func (m *Model) MoveActiveToolWindowToBottom() { m.moveActiveTool(false) }

func (m *Model) moveActiveTool(split bool) {
	if m.tool == "" || !m.toolVisible(m.tool) {
		return
	}
	if m.splitNames[m.tool] == split {
		return
	}
	if split {
		if m.splitNames == nil {
			m.splitNames = map[string]bool{}
		}
		m.splitNames[m.tool] = true
	} else {
		delete(m.splitNames, m.tool)
	}
	m.occupyBottom(m.tool)
	m.invalidate()
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
// window, and a second press brings back exactly what it hid, maximized again
// if it was.
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
		m.toolStashMax = m.maximized
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
		m.maximized = m.toolStashMax
	}
	m.invalidate()
}

// bottomDefaultHeight is the bottom slot's height before a resize, OpenIDEA's
// DEFAULT_SIZES.bottom. bottomStep and bottomMin match RESIZE_STEP and the
// smallest permitted size.
const (
	bottomDefaultHeight = 280
	bottomStep          = 24
	bottomMin           = 80
)

// bottomSize is the height the bottom slot draws at.
func (m *Model) bottomSize() int {
	if m.bottomHeight < bottomMin {
		return bottomDefaultHeight
	}
	return m.bottomHeight
}

// ToggleMaximizeToolWindow is OpenIDEA's Cmd+Shift+': the bottom slot takes the
// workbench and a second press gives it back. With the slot closed, the window
// the shortcuts last opened comes back rather than nothing happening.
func (m *Model) ToggleMaximizeToolWindow() {
	if m.visibleTool() == "" {
		if m.tool == "" {
			return
		}
		m.activateTool(m.tool)
		if m.visibleTool() == "" {
			return
		}
	}
	m.maximized = !m.maximized
	m.invalidate()
}

// ResizeBottomToolWindow is OpenIDEA's Resize Tool Window: it moves the bottom
// slot's inner edge one step. The arrows along the edge change nothing, as
// resizeDelta returns 0 for them on a bottom window, and a maximized window
// keeps the workbench. frameHeight caps the slot the way innerHeight does.
func (m *Model) ResizeBottomToolWindow(dir string) {
	if m.visibleTool() == "" || m.maximized {
		return
	}
	delta := 0
	switch dir {
	case "up":
		delta = bottomStep
	case "down":
		delta = -bottomStep
	default:
		return
	}
	h := m.bottomSize() + delta
	if h < bottomMin {
		return
	}
	if room := int(m.frameHeight) - 160; room > bottomMin && h > room {
		h = room
	}
	if h == m.bottomHeight {
		return
	}
	m.bottomHeight = h
	m.invalidate()
}

// revealEditor brings the editor back over a maximized tool window, as
// OpenIDEA's navigation restores the maximized side.
func (m *Model) revealEditor() {
	if m.maximized {
		m.maximized = false
		m.invalidate()
	}
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
