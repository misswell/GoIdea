package app

// SmokeDock moves a tool window through the workbench's six slots on the
// running app and reports what the dock did, for the base e2e: each move's
// slot, that the content stayed open, Hide All and its restore, Pin protection
// and the headers the moved windows actually render.

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// SmokeDock drives the dock and renders the workbench offscreen.
func (m *Model) SmokeDock() map[string]any {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		m.ShowTodo()
		moved := map[string]string{}
		content, duplicates := true, 0
		for _, s := range []dockSlot{dockLeft, dockLeftSplit, dockBottom, dockBottomSplit,
			dockRight, dockRightSplit} {
			m.DockMove("todo", s)
			moved[dockSlotNames[s]] = m.dockOccupant(s)
			if !m.todo.visible {
				content = false
			}
			n := 0
			for _, id := range m.dock.slots {
				if id == "todo" {
					n++
				}
			}
			if n != 1 {
				duplicates++
			}
		}
		out["moves"] = moved
		out["content"] = content
		out["duplicates"] = duplicates

		m.DockMove("todo", dockBottom)
		m.HideAllToolWindows()
		out["hidden"] = !m.dockWindowShown("todo")
		m.HideAllToolWindows()
		out["restored"] = m.dockWindowShown("todo") && m.todo.visible

		// Pin: Problems keeps the right slot while TODO moves beside it.
		m.ShowProblems()
		m.DockMove("problems", dockRight)
		m.SetToolWindowPinned("problems", true)
		m.ShowTodo()
		m.DockMove("todo", dockRight)
		out["pinnedRight"] = m.dock.slots[dockRight] == "problems"
		out["pinnedSplit"] = m.dock.slots[dockRightSplit] == "todo"

		tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, smokeShotWidth, smokeShotHeight)
		tt.SetDark(true)
		tt.SetScale(1)
		tt.Frame()
		out["problemsRendered"] = tt.HasText("问题")
		out["todoRendered"] = tt.HasText("TODO")

		m.SetToolWindowPinned("problems", false)
		m.DockHide("problems")
		m.DockHide("todo")
	})
	return out
}
