package app

// Registrations for the tool windows that moved onto the dock. Each spec wires
// the window's existing business state to the dock's slots, so moving a window
// between areas opens its content there rather than moving an empty island.

import "github.com/egoist/mygo/ui"

func init() {
	registerToolWindow(&toolWindowSpec{
		id: "problems", title: "Problems", anchor: dockBottom, order: 60,
		open:  func(m *Model) { m.problems.visible = true },
		close: func(m *Model) { m.problems.visible = false },
		shown: func(m *Model) bool { return m.problems.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.problemsPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "todo", title: "TODO", anchor: dockBottom, order: 70,
		open: func(m *Model) {
			m.todo.visible = true
			if m.ctx == nil || m.mgr == nil {
				return
			}
			if m.Root() != "" {
				m.RefreshVCS()
			}
			if !m.todo.running && !m.todo.scannedOnce {
				m.RunTodo()
			}
		},
		close: func(m *Model) { m.todo.visible = false },
		shown: func(m *Model) bool { return m.todo.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.todoPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "services", title: "Services", anchor: dockBottom, order: 80,
		open:  func(m *Model) { m.services.visible = true },
		close: func(m *Model) { m.services.visible = false },
		shown: func(m *Model) bool { return m.services.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.servicesPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "notifications", title: "Notifications", anchor: dockRight, order: 10, stripe: true,
		open: func(m *Model) { m.markNotificationsRead() },
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.notificationsView(c, p, oi)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "hierarchy", title: "Hierarchy", anchor: dockRight, order: 40, stripe: true,
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.hierarchyRightView(c, p, oi)
		},
	})
}
