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
		id: "run", title: "Run", anchor: dockBottom, order: 10,
		open: func(m *Model) {
			m.run.visible = true
			if m.run.trusted && !m.run.loaded && m.ctx != nil && m.mgr != nil {
				m.loadRunConfigs()
			}
		},
		close:  func(m *Model) { m.run.visible = false },
		shown:  func(m *Model) bool { return m.run.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) { m.runPanel(c, p) },
	})
	registerToolWindow(&toolWindowSpec{
		id: "debug", title: "Debug", anchor: dockBottom, order: 20,
		open:  func(m *Model) { m.debug.visible = true },
		close: func(m *Model) { m.debug.visible = false },
		shown: func(m *Model) bool { return m.debug.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.debugPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "build", title: "Build", anchor: dockBottom, order: 30,
		open:  func(m *Model) { m.build.visible = true },
		close: func(m *Model) { m.build.visible = false },
		shown: func(m *Model) bool { return m.build.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.buildPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "term", title: "Terminal", anchor: dockBottom, order: 40,
		open: func(m *Model) {
			m.term.visible = true
			if m.ctx == nil || m.mgr == nil {
				return
			}
			if s := m.term.current(); s == nil || (s.id == 0 && !s.starting && !s.ended) {
				m.SpawnTerm()
			}
		},
		close:  func(m *Model) { m.term.visible = false },
		shown:  func(m *Model) bool { return m.term.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) { m.termPanel(c, p) },
	})
	registerToolWindow(&toolWindowSpec{
		id: "log", title: "Git Log", anchor: dockBottom, order: 50,
		open: func(m *Model) {
			m.log.visible = true
			if m.Root() != "" && m.ctx != nil && m.mgr != nil {
				m.RefreshLog()
			}
		},
		close:  func(m *Model) { m.log.visible = false },
		shown:  func(m *Model) bool { return m.log.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) { m.logPanel(c, p) },
	})
	registerToolWindow(&toolWindowSpec{
		id: "find", title: "Find", anchor: dockBottom, order: 90,
		open:  func(m *Model) { m.search.visible = true },
		close: func(m *Model) { m.search.visible = false },
		shown: func(m *Model) bool { return m.search.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.searchPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "usages", title: "Find Usages", anchor: dockBottom, order: 100,
		open:  func(m *Model) { m.usages.visible = true },
		close: func(m *Model) { m.usages.visible = false },
		shown: func(m *Model) bool { return m.usages.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.usagesPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "structure", title: "Structure", anchor: dockLeftSplit, order: 20, stripe: true,
		open: func(m *Model) {
			m.structure.visible = true
			m.leftHidden = false
			m.refreshStructure()
		},
		close: func(m *Model) { m.structure.visible = false },
		shown: func(m *Model) bool { return m.structure.visible },
		renderGrow: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors, grow float32) {
			m.leftSplitIsland(c, p, oi, grow)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "bookmarks", title: "Bookmarks", anchor: dockLeftSplit, order: 30, stripe: true,
		open: func(m *Model) {
			m.structure.visible = false
			m.leftHidden = false
		},
		renderGrow: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors, grow float32) {
			m.leftSplitIsland(c, p, oi, grow)
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
