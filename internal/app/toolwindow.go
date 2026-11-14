package app

// toolWindows lists the bottom tool windows in the order OpenIDEA registers
// them. The shortcuts activate one of these and the Hide All action walks the
// list, as toolwindows/store.ts does for its slots.
var toolWindows = []string{"run", "debug", "build", "term", "log", "problems", "todo", "find", "usages", "services"}

// toolVisible reports whether a tool window is showing. Registered windows
// answer from the dock, which is the authority on their placement.
func (m *Model) toolVisible(name string) bool {
	if dockSpec(name) != nil {
		return m.dockWindowShown(name)
	}
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
	case "todo":
		return m.todo.visible
	case "find":
		return m.search.visible
	case "usages":
		return m.usages.visible
	case "services":
		return m.services.visible
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
	case "todo":
		m.todo.visible = false
	case "find":
		m.search.visible = false
	case "usages":
		m.usages.visible = false
	case "services":
		m.services.visible = false
	}
	if m.maximized && m.visibleTool() == "" {
		m.maximized = false
	}
	m.rememberLayout()
}

// occupyBottom makes name the window its area shows and closes the others in
// that area: an OpenIDEA area shows one tool window at a time, so opening a
// window takes the area from the one that held it. The other area keeps its
// window. The closed window keeps its contents, as a tab does: the terminal's
// shell and a run's output are still there.
func (m *Model) occupyBottom(name string) {
	if dockSpec(name) != nil {
		m.DockShow(name)
		return
	}
	area := m.toolArea(name)
	release := m.holdLayout()
	target := dockBottom
	if area == "split" {
		target = dockBottomSplit
	}
	m.dockEvictArea(target, name)
	for _, other := range toolWindows {
		if other != name && m.toolArea(other) == area {
			m.hideTool(other)
		}
	}
	release()
	m.tool = name
	m.rememberLayout()
}

// isToolWindow reports whether name is one of the bottom tool windows.
func isToolWindow(name string) bool {
	for _, n := range toolWindows {
		if n == name {
			return true
		}
	}
	return false
}

// holdLayout collapses the writes one action makes into one: Hide All and a
// restored layout move several windows, and only the state they end in is
// worth storing. The release function restores the mark it replaced, so holds
// nest.
func (m *Model) holdLayout() func() {
	held := m.layoutHold
	m.layoutHold = true
	return func() { m.layoutHold = held }
}

// currentLayout is the workbench state a project keeps between runs, the
// fields OpenIDEA's layout record stores: the left area's window, the bottom
// windows and their area, the bottom height and the last-used window.
func (m *Model) currentLayout() Layout {
	l := Layout{
		Side:         m.sideMode,
		LeftTool:     m.leftSplit,
		LeftHidden:   m.leftHidden,
		LeftRatio:    float64(m.leftShare()),
		LeftWidth:    int(m.leftWidth + 0.5),
		Tool:         m.tool,
		BottomHeight: m.bottomHeight,
		Right:        m.rightTool,
		RightHidden:  m.rightHidden || m.rightTool == "",
		RightWidth:   int(m.rightWidth + 0.5),
		RightSplit:   m.rightSplitTool,
		RightRatio:   float64(m.rightRatio),
		Dock:         m.currentDockLayout(),
	}
	for _, name := range toolWindows {
		if dockSpec(name) != nil {
			s, ok := m.dockSlotOf(name)
			if !ok {
				continue
			}
			l.Bottom = append(l.Bottom, name)
			if s == dockBottomSplit {
				if l.Split == nil {
					l.Split = map[string]bool{}
				}
				l.Split[name] = true
			}
			continue
		}
		if m.toolVisible(name) {
			l.Bottom = append(l.Bottom, name)
		}
		if m.splitNames[name] {
			if l.Split == nil {
				l.Split = map[string]bool{}
			}
			l.Split[name] = true
		}
	}
	return l
}

// layoutFor is the stored layout of a project, the default one when it has
// none.
func (m *Model) layoutFor(root string) Layout {
	return normalizeLayout(m.layouts[layoutKey(root)])
}

// rememberLayout stores the workbench layout under the open project, as
// OpenIDEA keeps one per project root, and writes the session. Without a
// project or a session file there is nowhere to keep it.
func (m *Model) rememberLayout() {
	if m.layoutHold {
		return
	}
	root := m.Root()
	if root == "" || m.cfg.SessionPath == "" {
		return
	}
	if m.layouts == nil {
		m.layouts = map[string]Layout{}
	}
	m.layouts[layoutKey(root)] = m.currentLayout()
	m.saveSession()
}

// applyLayout restores what the project was left with. Windows come back
// through the ordinary show actions, so a terminal that was open still has its
// shell and a run window still its output, exactly as a shortcut press would
// leave them.
func (m *Model) applyLayout(root string) {
	l := m.layoutFor(root)
	release := m.holdLayout()
	defer release()

	// Reset the dock and the legacy mirrors it feeds.
	m.structure.visible = false
	m.leftSplit = ""
	m.problems.visible = false
	m.todo.visible = false
	m.services.visible = false
	m.hierarchy = hierarchyPane{}
	m.dock = dockState{}
	m.rightTool, m.rightSplitTool = "", ""
	m.rightHidden = true
	m.leftRatio = float32(l.LeftRatio)
	m.leftDragging = false
	m.leftWidth = float32(l.LeftWidth)
	m.leftWidthDragging = false
	m.leftDragCollapse = false
	m.bottomDragging = false
	m.bottomDragCollapse = false
	m.rightWidth = float32(l.RightWidth)
	m.rightWidthDragging = false
	m.rightRatio = float32(l.RightRatio)
	m.rightDragging = false
	m.bottomHeight = l.BottomHeight
	m.splitNames = nil
	m.tool = ""
	m.toolStash = nil
	m.toolStashMax = false
	m.maximized = false

	// The slots to restore: the versioned record when it is there, the old
	// fields otherwise. Both end in the same map.
	slots := map[string]string{}
	if l.Dock != nil && l.Dock.Version >= dockLayoutVersion {
		for k, v := range l.Dock.Slots {
			if v != "" {
				slots[k] = v
			}
		}
		m.dock.active = l.Dock.Active
		m.dock.last = append([]string(nil), l.Dock.Last...)
		m.maximized = l.Dock.Max && len(slots) > 0
		for k, v := range l.Dock.Hidden {
			if s, ok := dockSlotOfName(k); ok && v != "" {
				m.dock.hidden[s] = v
				m.dock.hasHidden = true
			}
		}
		for _, id := range l.Dock.Pinned {
			if dockSpec(id) == nil {
				continue
			}
			if m.dock.pinned == nil {
				m.dock.pinned = map[string]bool{}
			}
			m.dock.pinned[id] = true
		}
		for id, mode := range l.Dock.ViewModes {
			if dockSpec(id) == nil || mode != "dockUnpinned" {
				continue
			}
			if m.dock.viewModes == nil {
				m.dock.viewModes = map[string]string{}
			}
			m.dock.viewModes[id] = mode
		}
	} else {
		// Sessions from before the dock: split the old lists the way the
		// old workbench read them.
		if l.Side != "" {
			slots["left"] = l.Side
		}
		if l.LeftTool != "" {
			slots["leftSplit"] = l.LeftTool
		}
		primary := false
		for _, name := range l.Bottom {
			if l.Split[name] {
				slots["bottomSplit"] = name
				continue
			}
			if !primary {
				slots["bottom"] = name
				primary = true
			}
		}
		if l.Right != "" {
			slots["right"] = l.Right
		}
		if l.RightSplit != "" {
			slots["rightSplit"] = l.RightSplit
		}
		m.dock.last = append([]string(nil), l.Bottom...)
		m.dock.active = l.Tool
	}

	switch slots["left"] {
	case "vcs":
		m.sideMode = "vcs"
	default:
		m.sideMode = ""
	}
	m.leftHidden = l.LeftHidden

	// Legacy windows need their area known before they open, or occupyBottom
	// would put a split window back in the primary area.
	for name, id := range slots {
		if name == "bottomSplit" && id != "" && dockSpec(id) == nil {
			if m.splitNames == nil {
				m.splitNames = map[string]bool{}
			}
			m.splitNames[id] = true
		}
	}

	// Windows that are not registered yet restore through their old openers;
	// registered ones open their content and take the recorded slot.
	for name, id := range slots {
		if id == "" {
			continue
		}
		slot, ok := dockSlotOfName(name)
		if !ok {
			continue
		}
		if dockSpec(id) == nil {
			switch {
			case isToolWindow(id):
				m.activateTool(id)
			case slot == dockLeftSplit && id == "structure":
				m.ShowStructure()
			case slot == dockLeftSplit && id == "bookmarks":
				m.ShowBookmarksWindow()
			}
			continue
		}
		if slot == dockLeft {
			// The left primary area keeps its own field (Project / Commit).
			continue
		}
		m.dockOpenContent(id)
		m.dock.slots[slot] = id
		m.dock.last = append(m.dock.last, id)
	}

	m.dockSyncMirrors()
	if isToolWindow(l.Tool) && m.toolVisible(l.Tool) {
		m.tool = l.Tool
	} else if m.tool == "" {
		m.tool = m.primaryTool()
	}
	if m.dock.active == "" {
		m.dock.active = m.tool
	}
	m.invalidate()
}

// ResetToolWindowLayout is OpenIDEA's Restore Default Layout: every bottom
// window comes back to the primary area. A window that was in the split area
// hides, because the primary area keeps the window it showed, and sizes and
// other windows stay as they are.
func (m *Model) ResetToolWindowLayout() {
	primary := m.primaryTool()
	release := m.holdLayout()
	for _, name := range toolWindows {
		if dockSpec(name) != nil {
			if s, ok := m.dockSlotOf(name); ok {
				if s == dockBottomSplit {
					m.DockHide(name)
				} else if s != dockSpec(name).anchor {
					m.DockMove(name, dockSpec(name).anchor)
				}
			}
			continue
		}
		if m.splitNames[name] {
			m.hideTool(name)
		}
	}
	release()
	m.splitNames = nil
	if primary != "" && m.toolVisible(primary) {
		m.tool = primary
	}
	m.rememberLayout()
	m.invalidate()
}

// toolArea is the bottom side area a window sits in, "primary" or "split".
func (m *Model) toolArea(name string) string {
	if dockSpec(name) != nil {
		if s, ok := m.dockSlotOf(name); ok && s == dockBottomSplit {
			return "split"
		}
		return "primary"
	}
	if m.splitNames[name] {
		return "split"
	}
	return "primary"
}

// primaryTool is the window the bottom primary area shows, "" when it is empty.
func (m *Model) primaryTool() string {
	for _, name := range toolWindows {
		if dockSpec(name) != nil {
			if s, ok := m.dockSlotOf(name); ok && s == dockBottom {
				return name
			}
			continue
		}
		if m.toolArea(name) == "primary" && m.toolVisible(name) {
			return name
		}
	}
	return ""
}

// splitTool is the window the bottom split area shows, "" when it is empty.
func (m *Model) splitTool() string {
	for _, name := range toolWindows {
		if dockSpec(name) != nil {
			if s, ok := m.dockSlotOf(name); ok && s == dockBottomSplit {
				return name
			}
			continue
		}
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
	if dockSpec(m.tool) != nil {
		target := dockBottom
		if split {
			target = dockBottomSplit
		}
		m.DockMove(m.tool, target)
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
	case "todo":
		m.ShowTodo()
	case "find":
		m.ShowSearch()
	case "usages":
		m.showUsages()
	case "services":
		m.ShowServices()
	}
	m.rememberLayout()
}

// MoveToolToRight docks a bottom window into the right area, as OpenIDEA's
// tool window menu moves a window between areas. Problems and TODO render in
// either area; the others keep their home.
func (m *Model) MoveToolToRight() {
	if dockSpec(m.tool) != nil && m.toolVisible(m.tool) {
		m.DockMove(m.tool, dockRight)
		return
	}
	if top := m.dockOccupant(dockRight); top != "" {
		m.DockMove(top, dockBottom)
		return
	}
	name := m.tool
	if name != "problems" && name != "todo" {
		if m.rightTool == "problems" || m.rightTool == "todo" {
			name = m.rightTool
		} else {
			m.status = "该工具窗口不能移到右侧"
			m.invalidate()
			return
		}
	}
	m.hideTool(name)
	m.rightTool = name
	m.rightHidden = false
	m.rememberLayout()
	m.invalidate()
}

// MoveToolToLeft docks the active window into the left area, the left split
// with MoveToolToLeftSplit, and the right split or bottom split with their own
// commands, as OpenIDEA's Move to › Left Top / Left Bottom / Right Bottom /
// Bottom Right do.
func (m *Model) MoveToolToLeft() { m.moveToolToSlot(dockLeft) }

// MoveToolToLeftSplit docks the active window under the left area's first one.
func (m *Model) MoveToolToLeftSplit() { m.moveToolToSlot(dockLeftSplit) }

// MoveToolToRightSplit docks the active window into the right area's bottom
// island, below the window already there.
func (m *Model) MoveToolToRightSplit() { m.moveToolToSlot(dockRightSplit) }

// MoveToolToBottomSplit docks the active window into the bottom area's right
// island, beside the window already there.
func (m *Model) MoveToolToBottomSplit() { m.moveToolToSlot(dockBottomSplit) }

func (m *Model) moveToolToSlot(target dockSlot) {
	if !isToolWindow(m.tool) || !m.toolVisible(m.tool) {
		return
	}
	if dockSpec(m.tool) == nil {
		m.status = "该工具窗口不能移动"
		m.invalidate()
		return
	}
	m.DockMove(m.tool, target)
}

// MoveToolToBottom docks a right-area window back into the bottom area.
func (m *Model) MoveToolToBottom() {
	if id := m.dockOccupant(dockRight); id != "" {
		m.DockMove(id, dockBottom)
		return
	}
	if id := m.dockOccupant(dockRightSplit); id != "" {
		m.DockMove(id, dockBottomSplit)
		return
	}
	switch m.rightTool {
	case "problems":
		m.rightHidden = true
		m.rememberLayout()
		m.ShowProblems()
	case "todo":
		m.rightHidden = true
		m.rememberLayout()
		m.ShowTodo()
	default:
		m.status = "该工具窗口不能移到底部"
		m.invalidate()
	}
}

// HideActiveToolWindow is OpenIDEA's Shift+Escape: the tool window the
// shortcuts last showed goes away and the editor keeps the keyboard.
func (m *Model) HideActiveToolWindow() {
	if m.tool == "" || !m.toolVisible(m.tool) {
		if dockSpec(m.rightTool) != nil && m.dockWindowShown(m.rightTool) {
			m.DockHide(m.rightTool)
			return
		}
		if m.rightShown() {
			if m.rightTool == "notifications" {
				m.CloseNotifications()
			} else {
				m.CloseHierarchyWindow()
			}
		}
		return
	}
	if dockSpec(m.tool) != nil {
		m.DockHide(m.tool)
		return
	}
	m.hideTool(m.tool)
	m.invalidate()
}

// HideAllToolWindows is OpenIDEA's Cmd+Shift+F12: it hides every bottom tool
// window, and a second press brings back exactly what it hid, maximized again
// if it was.
func (m *Model) HideAllToolWindows() {
	dockAny := false
	for s := dockSlot(0); s < dockSlotCount; s++ {
		if m.dock.slots[s] != "" {
			dockAny = true
		}
	}
	any := m.rightShown()
	for _, name := range toolWindows {
		if m.toolVisible(name) {
			any = true
			break
		}
	}
	if any {
		m.toolStash = make(map[string]bool, len(toolWindows))
		m.toolStashMax = m.maximized
		m.rightStash = m.rightShown()
		release := m.holdLayout()
		if dockAny {
			m.dockHideAll()
		}
		for _, name := range toolWindows {
			if dockSpec(name) != nil {
				continue
			}
			m.toolStash[name] = m.toolVisible(name)
			m.hideTool(name)
		}
		if m.rightStash {
			m.rightHidden = true
		}
		release()
	} else if m.toolStash != nil || m.rightStash || m.dock.hasHidden {
		stash := m.toolStash
		m.toolStash = nil
		release := m.holdLayout()
		for _, name := range toolWindows {
			if dockSpec(name) != nil {
				continue
			}
			if stash[name] {
				m.activateTool(name)
			}
		}
		if m.dock.hasHidden {
			m.dockRestoreAll()
		}
		if m.rightStash {
			m.rightHidden = false
			m.rightStash = false
		}
		release()
		m.maximized = m.toolStashMax
	} else {
		return
	}
	m.rememberLayout()
	m.invalidate()
}

// hideBottomWindows closes every bottom tool window, which a drag of the gap
// above the bottom area past half its smallest height does. Unlike Hide All it
// keeps no stash: the drag is how the side goes away, not a toggle.
func (m *Model) hideBottomWindows() {
	release := m.holdLayout()
	for _, name := range toolWindows {
		m.hideTool(name)
	}
	release()
	m.rememberLayout()
	m.invalidate()
}

// bottomDefaultHeight is the bottom slot's height before a resize, OpenIDEA's
// DEFAULT_SIZES.bottom. bottomStep and bottomMin match RESIZE_STEP and the
// smallest permitted size; maxBottomHeight is the tallest a stored layout can
// ask for, OpenIDEA's size sanitizer bound.
const (
	bottomDefaultHeight = 280
	bottomStep          = 24
	bottomMin           = 80
	maxBottomHeight     = 4000
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
	m.rememberLayout()
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
