package app

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

const (
	tabKindAgentSession = "agentSession"
	tabKindAgentHome    = "agentHome"
)

func agentSessionPath(taskID string) string {
	return "agent:" + url.PathEscape(taskID)
}

func (m *Model) agentTaskTab(taskID string) *Tab {
	for _, t := range m.allTabs() {
		if t.kind == tabKindAgentSession && t.agentTask == taskID {
			return t
		}
	}
	return nil
}

func (m *Model) agentTaskTabs() []*Tab {
	var tabs []*Tab
	for _, t := range m.allTabs() {
		if t.kind == tabKindAgentSession {
			tabs = append(tabs, t)
		}
	}
	return tabs
}

func (m *Model) agentHomeTab() *Tab {
	for _, t := range m.allTabs() {
		if t.kind == tabKindAgentHome {
			return t
		}
	}
	return nil
}

func (m *Model) agentNewTaskVisible() bool {
	return m.agents.newOpen || m.agentHomeTab() != nil
}

func (m *Model) agentNewTaskFocused() bool {
	t := m.tab()
	return m.agents.newOpen || t != nil && t.kind == tabKindAgentHome
}

func (m *Model) activeAgentTaskTab() *Tab {
	t := m.tab()
	if t == nil || t.kind != tabKindAgentSession {
		return nil
	}
	return t
}

func (m *Model) newAgentPageTab(kind, path, title, taskID string) *Tab {
	doc := docbridge.Virtual(path, "")
	t := &Tab{
		doc: doc,
		state: &editor.State{
			Buffer: doc.Buffer(), LineHeight: 1.65,
			Bindings: intellijEditorBindings(), ReadOnly: true,
		},
		kind:      kind,
		pageTitle: title,
		agentTask: taskID,
	}
	t.state.Font = ui.Font{Family: oiFontUI, Size: m.prefs.fontSize()}
	t.synced = doc.Buffer().Version()
	t.seen = t.synced
	m.noteTabUse(t)
	return t
}

// openAgentTask follows the Agents-wide placement preference.
func (m *Model) openAgentTask(taskID string) {
	if _, ok := m.agents.tasks[taskID]; !ok {
		return
	}
	if m.agentSessionLocation() == "toolWindow" {
		m.OpenAgentsWindow()
		m.openAgentConversation(taskID)
		return
	}
	m.openAgentTaskInEditor(taskID)
}

func (m *Model) openAgentTaskInEditor(taskID string) {
	task, ok := m.agents.tasks[taskID]
	if !ok {
		return
	}
	m.agents.visible = true
	m.agents.newOpen = false
	m.switchAgentComposer(taskID)
	m.agents.conversation = true
	m.agents.selectedSubagents = nil
	delete(m.agents.unread, taskID)
	m.agents.actionErr = ""
	m.loadAgentEntries(taskID, false)
	m.refreshAgentChanges(taskID, false)
	if t := m.agentTaskTab(taskID); t != nil {
		t.pageTitle = task.Title
		m.activateAgentTab(t)
		m.showingWelcome = false
		m.invalidate()
		return
	}
	t := m.newAgentPageTab(tabKindAgentSession, agentSessionPath(taskID),
		task.Title, taskID)
	if m.split != nil {
		m.focusGroup(groupFocused)
	}
	m.insertTab(t)
	m.active = m.tabIndex(t)
	m.showingWelcome = false
	m.revealEditor()
	m.applyActive()
	m.saveSession()
	m.invalidate()
}

func (m *Model) openAgentHomeInEditor() {
	m.agents.visible = true
	m.connectAgents(false)
	m.stashAgentComposer()
	m.agents.newOpen = false
	m.agents.conversation = false
	m.agents.selected = ""
	m.agents.selectedSubagents = nil
	m.agents.composer = ""
	m.agents.composerFiles = nil
	m.agents.composerImages = nil
	if t := m.agentHomeTab(); t != nil {
		m.activateAgentTab(t)
		m.showingWelcome = false
		m.invalidate()
		return
	}
	t := m.newAgentPageTab(tabKindAgentHome, "agent:new", "New Agent Task", "")
	if m.split != nil {
		m.focusGroup(groupFocused)
	}
	m.insertTab(t)
	m.active = m.tabIndex(t)
	m.showingWelcome = false
	m.revealEditor()
	m.applyActive()
	m.saveSession()
	m.invalidate()
}

func (m *Model) activateAgentTab(t *Tab) {
	if t == nil {
		return
	}
	if g, ok := m.groupOf(t); ok && g == groupSplit {
		m.focusGroup(groupSplit)
	}
	if index := m.tabIndex(t); index >= 0 {
		m.activate(index)
	}
}

func (m *Model) removeAgentHomeTab() {
	if t := m.agentHomeTab(); t != nil {
		m.removeTabNow(t)
	}
}

func (m *Model) closeAgentTaskEditor(taskID string) {
	if t := m.agentTaskTab(taskID); t != nil {
		m.removeTabNow(t)
	}
}

func (m *Model) updateAgentTaskTab(task agentTask) {
	if t := m.agentTaskTab(task.ID); t != nil {
		t.pageTitle = task.Title
		if m.tab() == t {
			m.applyActive()
		}
	}
}

// syncAgentTaskTabs closes restored tabs the backend no longer reports and
// refreshes titles for the tabs it does.
func (m *Model) syncAgentTaskTabs() {
	for _, t := range m.agentTaskTabs() {
		task, ok := m.agents.tasks[t.agentTask]
		if !ok {
			m.removeTabNow(t)
			continue
		}
		t.pageTitle = task.Title
	}
}

func (m *Model) agentSessionPage(c *ui.Context, p *ui.Theme, oi oiColors, t *Tab) {
	task, ok := m.agents.tasks[t.agentTask]
	if !ok {
		m.removeTabNow(t)
		return
	}
	t.pageTitle = task.Title
	if m.agents.selected != task.ID {
		m.switchAgentComposer(task.ID)
		m.agents.selectedSubagents = nil
		m.loadAgentEntries(task.ID, false)
	}
	m.agents.conversation = true
	key := "agent-session:" + task.ID
	if m.split != nil && m.splitGroupContains(t) {
		key += ":split"
	}
	ui.Column(c.Key(key)).Grow(1).MinHeight(0).MinWidth(0).
		Background(oi.island).Children(func() {
		m.agentConversationView(c, p, oi, true)
	})
}

func (m *Model) splitGroupContains(t *Tab) bool {
	if m.split == nil {
		return false
	}
	for _, candidate := range m.split.tabs {
		if candidate == t {
			return true
		}
	}
	return false
}

func (m *Model) agentHomePage(c *ui.Context, p *ui.Theme, oi oiColors) {
	m.agents.conversation = false
	ui.Column(c.Key("agent-home")).Grow(1).MinHeight(0).MinWidth(0).
		Background(oi.island).Children(func() {
		m.agentsNewTaskEditor(c, p, oi)
	})
}

func (m *Model) agentsNewTaskEditor(c *ui.Context, p *ui.Theme, oi oiColors) {
	a := &m.agents
	ui.Row(c).Height(oiTokensFor(m, p).HeaderHeight).Shrink(0).Padding(0, 12).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, "New Agent Task").FontSize(13).TextColor(oi.textStrong).
					SingleLine()
				ui.Text(c, "Delegate a task to a coding agent").FontSize(11).
					TextColor(oi.textMuted).SingleLine()
			})
			ui.Button(c, "Chat in Tool Window").OnClick(m.useAgentToolWindow)
		})
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).Justify(ui.Center).Children(func() {
			ui.Column(c).Width(760).MaxWidthPercent(100).Padding(22, 28, 32, 28).
				Gap(10).Children(func() {
				ui.Text(c, "Provider").FontSize(12).TextColor(p.TextMuted)
				if a.providersLoading && len(a.providers) == 0 {
					ui.Text(c, "Loading agents…").FontSize(12).TextColor(p.TextMuted)
				} else if a.providersErr != "" {
					ui.Row(c).Gap(6).Children(func() {
						ui.Text(c, a.providersErr).Grow(1).FontSize(12).TextColor(p.Danger)
						ui.Button(c, "Retry").OnClick(func() { m.refreshAgentProviders(true) })
					})
				} else if len(a.providers) == 0 {
					ui.Text(c, "No agent providers are configured").FontSize(12).
						TextColor(p.TextMuted)
				} else {
					labels := make([]string, 0, len(a.providers))
					labelToID := make(map[string]string, len(a.providers))
					selected := ""
					for _, provider := range a.providers {
						label := provider.Name
						if !provider.Installed {
							label += " (not installed)"
						}
						labels = append(labels, label)
						labelToID[label] = provider.ID
						if provider.ID == a.newProvider {
							selected = label
						}
					}
					if selected == "" {
						selected = labels[0]
					}
					if choice := ui.Select(c, &selected, labels).Grow(1); choice.Changed() {
						a.newProvider = labelToID[selected]
						a.newConfig = nil
						m.prepareNewAgentConfig()
						m.resetNewAgentCommands()
					}
				}
				m.prepareNewAgentConfig()
				for _, option := range m.newAgentConfigOptions() {
					option := option
					if len(option.Values) == 0 {
						continue
					}
					labels := make([]string, 0, len(option.Values))
					values := make(map[string]string, len(option.Values))
					current := ""
					for _, value := range option.Values {
						label := value.Name
						if label == "" {
							label = value.Value
						}
						labels = append(labels, label)
						values[label] = value.Value
						if a.newConfig[option.ID] == value.Value {
							current = label
						}
					}
					if current == "" && len(labels) > 0 {
						current = labels[0]
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, option.Name).Width(120).FontSize(12).
							TextColor(p.TextMuted).SingleLine().Ellipsis("…")
						if choice := ui.Select(c, &current, labels).Grow(1); choice.Changed() {
							a.newConfig[option.ID] = values[current]
							m.prepareNewAgentConfig()
						}
					})
				}
				m.RefreshNewAgentCommands(false)
				if a.newCommandsErr != "" {
					ui.Text(c, a.newCommandsErr).FontSize(11).TextColor(p.Danger).
						SingleLine().Ellipsis("…")
				}
				ui.Text(c, "Where to work").FontSize(12).TextColor(p.TextMuted)
				locations := []string{"Local checkout", "New worktree"}
				shown := "Local checkout"
				if a.newIsolation == "worktree" {
					shown = "New worktree"
				}
				if choice := ui.Select(c, &shown, locations).Grow(1); choice.Changed() {
					if shown == "Local checkout" {
						a.newIsolation = "local"
					} else {
						a.newIsolation = "worktree"
					}
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Attachments").FontSize(12).TextColor(p.TextMuted)
					ui.Spacer(c)
					ui.Button(c, "Paste Image").Disabled(a.newFileBusy).OnClick(
						m.AddAgentTaskImageFromClipboard)
					ui.Button(c, "Add Files…").Disabled(a.newFileBusy).OnClick(func() {
						m.AddAgentTaskAttachments(false)
					})
					ui.Button(c, "Add Folder…").Disabled(a.newFileBusy).OnClick(func() {
						m.AddAgentTaskAttachments(true)
					})
				})
				for _, path := range a.newFiles {
					path := path
					ui.Row(c.Key("agent-home-file:"+path)).MinHeight(24).
						Padding(0, 5).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, m.AgentTaskFilePath(path)).Grow(1).MinWidth(0).
							FontSize(11).TextColor(p.Text).SingleLine().Ellipsis("…").
							Tooltip(path)
						ui.Button(c, "Remove").OnClick(func() { m.RemoveAgentTaskFile(path) })
					})
				}
				m.agentImageStrip(c, oi, a.newImages, m.RemoveAgentTaskImage)
				ui.Text(c, "Task").FontSize(12).TextColor(p.TextMuted)
				area := ui.TextArea(c.Key("agent-home-prompt"), &a.newPrompt).
					Height(210).AutoFocus().HandleInput(m.HandleAgentComposerInput)
				if dropped := area.DroppedFiles(); len(dropped) > 0 {
					m.addAgentDroppedPaths(true, dropped)
				}
				if area.FileDragOver() {
					area.BorderWidth(1).BorderColor(p.Accent)
				}
				if a.completionCaretSet {
					caret := a.completionCaret
					area.SetTextSelection(caret, caret)
					a.completionCaretSet = false
				}
				_, caret := area.TextSelection()
				m.SyncAgentCompletion("new", a.newPrompt, caret, m.Root(), a.newCommands)
				m.agentCompletionsView(c, p, oi)
				if a.newErr != "" {
					ui.Text(c, a.newErr).FontSize(12).TextColor(p.Danger)
				}
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "Create").Disabled(a.newBusy || a.newProvider == "" ||
						(strings.TrimSpace(a.newPrompt) == "" && len(a.newFiles) == 0 &&
							len(a.newImages) == 0)).OnClick(m.createAgentTask)
				})
			})
		})
	})
}

func (m *Model) agentEditorTitle(t *Tab) string {
	if t == nil {
		return ""
	}
	switch t.kind {
	case tabKindAgentSession:
		if task, ok := m.agents.tasks[t.agentTask]; ok && task.Title != "" {
			return task.Title
		}
		if t.pageTitle != "" {
			return t.pageTitle
		}
		return "Agent Task"
	case tabKindAgentHome:
		return "New Agent Task"
	default:
		return ""
	}
}

func (m *Model) agentTabStatus(t *Tab) string {
	if t == nil || t.kind != tabKindAgentSession {
		return ""
	}
	task, ok := m.agents.tasks[t.agentTask]
	if !ok {
		return ""
	}
	return fmt.Sprint(agentStatusLabel(task.Status))
}
