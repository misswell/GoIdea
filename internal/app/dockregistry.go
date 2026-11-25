package app

// Registrations for the tool windows that moved onto the dock. Each spec wires
// the window's existing business state to the dock's slots, so moving a window
// between areas opens its content there rather than moving an empty island.

import "github.com/egoist/mygo/ui"

func init() {
	registerToolWindow(&toolWindowSpec{
		id: "agents", title: "Agents", icon: "toolwindows/agents", anchor: dockRight, order: 0, stripe: true,
		badge: func(m *Model) int { return agentUnreadCount(&m.agents) },
		open: func(m *Model) {
			m.agents.visible = true
			m.connectAgents(false)
		},
		close:  func(m *Model) { m.agents.visible = false },
		shown:  func(m *Model) bool { return m.agents.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) { m.agentsRightView(c, p, oi) },
	})
	registerToolWindow(&toolWindowSpec{
		id: documentationWindow, title: "Documentation", icon: "status/info", anchor: dockRightSplit, order: 90, stripe: true,
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) { m.documentationPanel(c, p) },
	})
	registerToolWindow(&toolWindowSpec{
		id: "coverage", title: "Coverage", icon: "toolwindows/coverage", anchor: dockRight, order: 30,
		open:   func(m *Model) { m.coverage.open = true },
		close:  func(m *Model) { m.coverage.open = false },
		shown:  func(m *Model) bool { return m.coverage.open },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) { m.coveragePanel(c, p) },
	})
	registerToolWindow(&toolWindowSpec{
		id: "problems", title: "Problems", icon: "toolwindows/problems", anchor: dockBottom, order: 30, stripe: true,
		badge: func(m *Model) int { return m.buildErrorBadge() },
		open:  func(m *Model) { m.problems.visible = true },
		close: func(m *Model) { m.problems.visible = false },
		shown: func(m *Model) bool { return m.problems.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.problemsPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "todo", title: "TODO", icon: "toolwindows/todo", anchor: dockBottom, order: 40, stripe: true,
		badge: func(m *Model) int { return m.todoCount() },
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
		id: "services", title: "Services", icon: "toolwindows/services", anchor: dockBottom, order: 8,
		badge: func(m *Model) int { return m.runningServiceCount() },
		open:  func(m *Model) { m.services.visible = true; m.loadBuildModel() },
		close: func(m *Model) { m.services.visible = false },
		shown: func(m *Model) bool { return m.services.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.servicesPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "graphql", title: "GraphQL", icon: "toolwindows/graphql", anchor: dockBottom, order: 4,
		open: func(m *Model) {
			m.ensureGraphQL()
			m.ensureGraphQLDiscovered(nil)
		},
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.graphqlPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "run", title: "Run", icon: "toolwindows/run", anchor: dockBottom, order: 10,
		badge: func(m *Model) int { return m.runningRunCount() },
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
		id: "debug", title: "Debug", icon: "toolwindows/debug", anchor: dockBottom, order: 20,
		open:  func(m *Model) { m.debug.visible = true },
		close: func(m *Model) { m.debug.visible = false },
		shown: func(m *Model) bool { return m.debug.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.debugPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "profiler", title: "Profiler", icon: "toolwindows/profiler", anchor: dockBottom, order: 25,
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.profilerPanel(c, p, oi)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "logcat", title: "Logcat", icon: "toolwindows/logcat", anchor: dockBottom, order: 26,
		open: func(m *Model) {
			m.ensureLogcat()
			if !m.logcat.devicesLoaded && !m.logcat.devicesLoading {
				m.RefreshLogcatDevices()
			}
		},
		close: func(m *Model) { m.StopLogcat() },
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.logcatPanel(c, p, oi)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "build", title: "Build", icon: "toolwindows/build", anchor: dockBottom, order: 30,
		badge: func(m *Model) int { return m.buildErrorBadge() },
		open:  func(m *Model) { m.build.visible = true },
		close: func(m *Model) { m.build.visible = false },
		shown: func(m *Model) bool { return m.build.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.buildPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "term", title: "Terminal", icon: "toolwindows/terminal", anchor: dockBottom, order: 10, stripe: true,
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
		id: "log", title: "Git", icon: "toolwindows/vcs", anchor: dockBottom, order: 5, stripe: true,
		open: func(m *Model) {
			m.setGitToolWindowTab(vcsContentLog)
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
		id: "maven", title: "Maven", icon: "toolwindows/maven", anchor: dockRight, order: 10, stripe: true,
		open:   func(m *Model) { m.maven.visible = true; m.loadBuildModel() },
		close:  func(m *Model) { m.maven.visible = false },
		shown:  func(m *Model) bool { return m.maven.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) { m.mavenPanel(c, p) },
	})
	registerToolWindow(&toolWindowSpec{
		id: "gradle", title: "Gradle", icon: "toolwindows/gradle", anchor: dockRight, order: 11, stripe: true,
		open: func(m *Model) {
			m.maven.gradleVisible = true
			m.maven.activeTool = "gradle"
			m.loadBuildModel()
		},
		close:  func(m *Model) { m.maven.gradleVisible = false },
		shown:  func(m *Model) bool { return m.maven.gradleVisible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) { m.gradlePanel(c, p) },
	})
	registerToolWindow(&toolWindowSpec{
		id: "find", title: "Find", icon: "toolwindows/find", anchor: dockBottom, order: 5,
		open:  func(m *Model) { m.search.visible = true },
		close: func(m *Model) { m.search.visible = false },
		shown: func(m *Model) bool { return m.search.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.searchPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "usages", title: "Find Usages", icon: "actions/findEntireFile", anchor: dockBottom, order: 35,
		open:  func(m *Model) { m.usages.visible = true },
		close: func(m *Model) { m.usages.visible = false },
		shown: func(m *Model) bool { return m.usages.visible },
		render: func(m *Model, c *ui.Context, p *ui.Theme, _ oiColors) {
			m.usagesPanel(c, p)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "structure", title: "Structure", icon: "toolwindows/structure", anchor: dockLeftSplit, order: 20, stripe: true,
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
		id: "bookmarks", title: "Bookmarks", icon: "toolwindows/bookmarks", anchor: dockLeftSplit, order: 30, stripe: true,
		open: func(m *Model) {
			m.structure.visible = false
			m.leftHidden = false
		},
		renderGrow: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors, grow float32) {
			m.leftSplitIsland(c, p, oi, grow)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "notifications", title: "Notifications", icon: "toolwindows/notifications", anchor: dockRight, order: 10, stripe: true,
		badge: func(m *Model) int { return m.unreadNotifications() },
		open:  func(m *Model) { m.markNotificationsRead() },
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.notificationsView(c, p, oi)
		},
	})
	registerToolWindow(&toolWindowSpec{
		id: "hierarchy", title: "Hierarchy", icon: "toolwindows/hierarchy", anchor: dockRight, order: 40, stripe: true,
		render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			m.hierarchyRightView(c, p, oi)
		},
	})
}
