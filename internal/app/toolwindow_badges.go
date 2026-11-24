package app

// Tool-window badges share OpenIDEA's sources: live Run sessions, running
// Services, failed Build diagnostics, project TODO items, unread Agents,
// unread notifications and working-tree changes.

func (m *Model) buildErrorBadge() int {
	if m.build.success == nil || *m.build.success {
		return 0
	}
	return int(m.build.errors)
}

func (m *Model) runningRunCount() int {
	count := 0
	for _, child := range m.run.children {
		if !child.service && child.live() {
			count++
		}
	}
	if count == 0 && m.run.running {
		return 1
	}
	return count
}

func (m *Model) todoBadge() int { return m.todoCount() }

func (m *Model) vcsBadge() int { return len(m.vcs.files) }

func (m *Model) toolWindowBadge(id string) int {
	spec := dockSpec(id)
	if spec == nil || spec.badge == nil || !m.prefs.toolWindowBadgesVisible() {
		return 0
	}
	return max(0, spec.badge(m))
}
