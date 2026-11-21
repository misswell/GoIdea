package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// AgentWorkspacePrefs is the placement of agent conversations and the task-list
// organization OpenIDEA keeps in local storage.
type AgentWorkspacePrefs struct {
	SessionLocation      string                   `json:"sessionLocation,omitempty"`
	Organize             map[string]AgentOrganize `json:"organize,omitempty"`
	ChangesGroupByCommit *bool                    `json:"changesGroupByCommit,omitempty"`
}

type AgentOrganizeGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AgentOrganize struct {
	Groups  []AgentOrganizeGroup `json:"groups,omitempty"`
	GroupOf map[string]string    `json:"groupOf,omitempty"`
	Pinned  []string             `json:"pinned,omitempty"`
}

func (w AgentWorkspacePrefs) normalized() AgentWorkspacePrefs {
	if w.SessionLocation != "toolWindow" {
		w.SessionLocation = "editor"
	}
	w.Organize = normalizeAgentOrganizeMap(w.Organize)
	return w
}

func (w AgentWorkspacePrefs) changesGroupByCommit() bool {
	return w.ChangesGroupByCommit == nil || *w.ChangesGroupByCommit
}

func normalizeAgentOrganizeMap(values map[string]AgentOrganize) map[string]AgentOrganize {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]AgentOrganize, len(values))
	for root, value := range values {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		value = value.normalized()
		if len(value.Groups) == 0 && len(value.GroupOf) == 0 && len(value.Pinned) == 0 {
			continue
		}
		out[layoutKey(root)] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (o AgentOrganize) normalized() AgentOrganize {
	groups := make([]AgentOrganizeGroup, 0, len(o.Groups))
	ids := make(map[string]bool, len(o.Groups))
	names := make(map[string]bool, len(o.Groups))
	for _, group := range o.Groups {
		group.ID = strings.TrimSpace(group.ID)
		group.Name = strings.TrimSpace(group.Name)
		if group.ID == "" || group.Name == "" || ids[group.ID] || names[group.Name] {
			continue
		}
		ids[group.ID] = true
		names[group.Name] = true
		groups = append(groups, group)
	}
	groupOf := make(map[string]string, len(o.GroupOf))
	for task, group := range o.GroupOf {
		if task != "" && ids[group] {
			groupOf[task] = group
		}
	}
	pinned := make([]string, 0, len(o.Pinned))
	seen := make(map[string]bool, len(o.Pinned))
	for _, task := range o.Pinned {
		if task != "" && !seen[task] {
			seen[task] = true
			pinned = append(pinned, task)
		}
	}
	return AgentOrganize{Groups: groups, GroupOf: groupOf, Pinned: pinned}
}

func (o AgentOrganize) clone() AgentOrganize {
	out := AgentOrganize{
		Groups:  append([]AgentOrganizeGroup(nil), o.Groups...),
		GroupOf: make(map[string]string, len(o.GroupOf)),
		Pinned:  append([]string(nil), o.Pinned...),
	}
	for task, group := range o.GroupOf {
		out.GroupOf[task] = group
	}
	return out
}

func (m *Model) agentSessionLocation() string {
	return m.prefs.AgentWorkspace.normalized().SessionLocation
}

func (m *Model) setAgentSessionLocation(location string) {
	if location != "toolWindow" {
		location = "editor"
	}
	if m.agentSessionLocation() == location {
		return
	}
	taskID := ""
	home := false
	if t := m.tab(); t != nil {
		switch t.kind {
		case tabKindAgentSession:
			taskID = t.agentTask
		case tabKindAgentHome:
			home = true
		}
	}
	if taskID == "" {
		taskID = m.agents.selected
	}
	if taskID == "" {
		taskID = m.mostRecentAgentTask()
	}
	m.prefs.AgentWorkspace.SessionLocation = location
	m.savePreferences()
	for _, taskTab := range m.agentTaskTabs() {
		m.removeTabNow(taskTab)
	}
	m.removeAgentHomeTab()
	m.closeAgentConversation()
	if location == "toolWindow" {
		m.agents.visible = true
		m.DockShow("agents")
		m.connectAgents(false)
		if home {
			m.agents.newOpen = true
			m.agents.newErr = ""
		} else if taskID != "" {
			m.agents.newOpen = false
			m.openAgentConversation(taskID)
		} else {
			m.agents.newOpen = true
			m.agents.newErr = ""
		}
		m.invalidate()
		return
	}
	m.agents.newOpen = false
	if home {
		m.openAgentHomeInEditor()
	} else if taskID != "" {
		m.openAgentTaskInEditor(taskID)
	} else {
		m.openAgentHomeInEditor()
	}
	m.invalidate()
}

func (m *Model) useAgentEditor() {
	m.setAgentSessionLocation("editor")
}

func (m *Model) useAgentToolWindow() {
	m.setAgentSessionLocation("toolWindow")
}

func (m *Model) openTaskInEditorLocation(task string) {
	m.prefs.AgentWorkspace.SessionLocation = "editor"
	m.savePreferences()
	m.openAgentTaskInEditor(task)
}

func (m *Model) dockAgentTask(task string) {
	if task == "" {
		task = m.agents.selected
	}
	if task == "" {
		return
	}
	m.prefs.AgentWorkspace.SessionLocation = "toolWindow"
	m.savePreferences()
	m.closeAgentTaskEditor(task)
	m.OpenAgentsWindow()
	m.openAgentConversation(task)
}

func (m *Model) undockAgentTask() {
	m.setAgentSessionLocation("editor")
}

func (m *Model) mostRecentAgentTask() string {
	var selected agentTask
	for _, task := range m.agents.tasks {
		if task.Status == "archived" {
			continue
		}
		if selected.ID == "" || task.UpdatedMS > selected.UpdatedMS ||
			(task.UpdatedMS == selected.UpdatedMS && task.CreatedMS > selected.CreatedMS) {
			selected = task
		}
	}
	return selected.ID
}

func (m *Model) agentOrganize() AgentOrganize {
	root := m.Root()
	if root == "" {
		return AgentOrganize{GroupOf: map[string]string{}}
	}
	return m.prefs.AgentWorkspace.Organize[layoutKey(root)].normalized()
}

func (m *Model) stashAgentComposer() {
	a := &m.agents
	if a.selected == "" {
		return
	}
	draft := agentComposerDraft{
		Text: a.composer, Files: append([]string(nil), a.composerFiles...),
		Images: append([]agentImage(nil), a.composerImages...), Caret: a.composerCaret,
		HistoryIndex: a.historyIndex, HistoryStash: a.historyStash,
		Completion: a.completion, CompletionPos: a.completionCaret,
		CompletionSeat: a.completionCaretSet,
	}
	if a.composerDrafts == nil {
		a.composerDrafts = make(map[string]agentComposerDraft)
	}
	if strings.TrimSpace(draft.Text) == "" && len(draft.Files) == 0 &&
		len(draft.Images) == 0 && !draft.Completion.Open && !draft.Completion.Busy {
		delete(a.composerDrafts, a.selected)
		return
	}
	a.composerDrafts[a.selected] = draft
}

func (m *Model) resumeAgentComposer(taskID string) {
	a := &m.agents
	draft, ok := a.composerDrafts[taskID]
	if !ok {
		a.composer = ""
		a.composerFiles = nil
		a.composerImages = nil
		a.composerCaret = 0
		a.historyIndex = -1
		a.historyStash = ""
		a.completion = agentCompletionState{}
		a.completionCaret = 0
		a.completionCaretSet = false
		return
	}
	a.composer = draft.Text
	a.composerFiles = append([]string(nil), draft.Files...)
	a.composerImages = append([]agentImage(nil), draft.Images...)
	a.composerCaret = draft.Caret
	a.historyIndex = draft.HistoryIndex
	a.historyStash = draft.HistoryStash
	a.completion = draft.Completion
	a.completionCaret = draft.CompletionPos
	a.completionCaretSet = draft.CompletionSeat
}

func (m *Model) switchAgentComposer(taskID string) {
	a := &m.agents
	if a.selected != taskID {
		m.stashAgentComposer()
	}
	a.selected = taskID
	m.resumeAgentComposer(taskID)
}

func (m *Model) clearAgentComposerDraft(taskID string) {
	if taskID == "" {
		return
	}
	delete(m.agents.composerDrafts, taskID)
	if m.agents.selected == taskID {
		m.agents.composer = ""
		m.agents.composerFiles = nil
		m.agents.composerImages = nil
		m.agents.composerCaret = 0
		m.agents.historyIndex = -1
		m.agents.historyStash = ""
		m.agents.completion = agentCompletionState{}
		m.agents.completionCaret = 0
		m.agents.completionCaretSet = false
	}
}

func (m *Model) saveAgentOrganize(value AgentOrganize) {
	root := m.Root()
	if root == "" {
		return
	}
	value = value.normalized()
	if m.prefs.AgentWorkspace.Organize == nil {
		m.prefs.AgentWorkspace.Organize = make(map[string]AgentOrganize)
	}
	m.prefs.AgentWorkspace.Organize[layoutKey(root)] = value
	m.savePreferences()
	m.invalidate()
}

// pruneAgentOrganize drops ids the backend no longer reports while keeping
// empty user groups, where a new task can be moved before it exists.
func (m *Model) pruneAgentOrganize() {
	if !m.agents.loaded || len(m.prefs.AgentWorkspace.Organize) == 0 {
		return
	}
	root := m.Root()
	key := layoutKey(root)
	current, ok := m.prefs.AgentWorkspace.Organize[key]
	if !ok {
		return
	}
	knownTasks := make(map[string]bool, len(m.agents.tasks))
	for id := range m.agents.tasks {
		knownTasks[id] = true
	}
	knownGroups := make(map[string]bool, len(current.Groups))
	for _, group := range current.Groups {
		knownGroups[group.ID] = true
	}
	groupOf := make(map[string]string, len(current.GroupOf))
	for task, group := range current.GroupOf {
		if knownTasks[task] && knownGroups[group] {
			groupOf[task] = group
		}
	}
	pinned := make([]string, 0, len(current.Pinned))
	for _, task := range current.Pinned {
		if knownTasks[task] {
			pinned = append(pinned, task)
		}
	}
	current.GroupOf = groupOf
	current.Pinned = pinned
	if current.Groups == nil {
		current.Groups = []AgentOrganizeGroup{}
	}
	m.prefs.AgentWorkspace.Organize[key] = current
}

func (m *Model) forgetAgentOrganizeTask(taskID string) {
	root := m.Root()
	if root == "" {
		return
	}
	key := layoutKey(root)
	organize, ok := m.prefs.AgentWorkspace.Organize[key]
	if !ok {
		return
	}
	delete(organize.GroupOf, taskID)
	organize.Pinned = removeAgentID(organize.Pinned, taskID)
	m.prefs.AgentWorkspace.Organize[key] = organize
}

func agentGroupNameError(value string, groups []AgentOrganizeGroup, current string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Enter a group name"
	}
	for _, group := range groups {
		if group.Name == value && group.Name != current {
			return fmt.Sprintf("A group named %q already exists", value)
		}
	}
	return ""
}

func (m *Model) openNewAgentTaskGroup(tasks []string) {
	a := &m.agents
	a.groupOpen = true
	a.groupMode = "new"
	a.groupID = ""
	a.groupName = ""
	a.groupTasks = uniqueAgentIDs(tasks)
	a.groupErr = ""
	m.invalidate()
}

func (m *Model) openRenameAgentTaskGroup(groupID string) {
	organize := m.agentOrganize()
	for _, group := range organize.Groups {
		if group.ID != groupID {
			continue
		}
		a := &m.agents
		a.groupOpen = true
		a.groupMode = "rename"
		a.groupID = groupID
		a.groupName = group.Name
		a.groupTasks = nil
		a.groupErr = ""
		m.invalidate()
		return
	}
}

func (m *Model) submitAgentTaskGroup() {
	a := &m.agents
	organize := m.agentOrganize()
	current := ""
	if a.groupMode == "rename" {
		for _, group := range organize.Groups {
			if group.ID == a.groupID {
				current = group.Name
				break
			}
		}
		if current == "" {
			a.groupOpen = false
			return
		}
	}
	name := strings.TrimSpace(a.groupName)
	if message := agentGroupNameError(name, organize.Groups, current); message != "" {
		a.groupErr = message
		m.invalidate()
		return
	}
	if a.groupMode == "rename" {
		for index := range organize.Groups {
			if organize.Groups[index].ID == a.groupID {
				organize.Groups[index].Name = name
				break
			}
		}
	} else {
		id := fmt.Sprintf("group-%d", time.Now().UnixNano())
		organize.Groups = append(organize.Groups, AgentOrganizeGroup{ID: id, Name: name})
		if organize.GroupOf == nil {
			organize.GroupOf = make(map[string]string)
		}
		for _, task := range a.groupTasks {
			if _, ok := m.agents.tasks[task]; ok {
				organize.GroupOf[task] = id
				organize.Pinned = removeAgentID(organize.Pinned, task)
			}
		}
	}
	m.saveAgentOrganize(organize)
	a.groupOpen = false
	a.groupErr = ""
}

func (m *Model) openRemoveAgentTaskGroup(groupID string) {
	for _, group := range m.agentOrganize().Groups {
		if group.ID != groupID {
			continue
		}
		m.agents.removeGroupOpen = true
		m.agents.removeGroupID = groupID
		m.invalidate()
		return
	}
}

func (m *Model) confirmRemoveAgentTaskGroup() {
	groupID := m.agents.removeGroupID
	m.agents.removeGroupOpen = false
	m.agents.removeGroupID = ""
	if groupID != "" {
		m.removeAgentTaskGroup(groupID)
	}
	m.invalidate()
}

func (m *Model) removeAgentTaskGroup(groupID string) {
	organize := m.agentOrganize()
	groups := organize.Groups[:0]
	found := false
	for _, group := range organize.Groups {
		if group.ID == groupID {
			found = true
			continue
		}
		groups = append(groups, group)
	}
	if !found {
		return
	}
	organize.Groups = groups
	for task, group := range organize.GroupOf {
		if group == groupID {
			delete(organize.GroupOf, task)
		}
	}
	m.saveAgentOrganize(organize)
}

func (m *Model) pinAgentTasks(tasks []string, pinned bool) {
	organize := m.agentOrganize()
	for _, task := range uniqueAgentIDs(tasks) {
		if _, ok := m.agents.tasks[task]; !ok {
			continue
		}
		organize.Pinned = removeAgentID(organize.Pinned, task)
		if pinned {
			organize.Pinned = append(organize.Pinned, task)
		}
	}
	m.saveAgentOrganize(organize)
}

func (m *Model) moveAgentTasksToGroup(tasks []string, groupID string) {
	organize := m.agentOrganize()
	if groupID != "" {
		known := false
		for _, group := range organize.Groups {
			if group.ID == groupID {
				known = true
				break
			}
		}
		if !known {
			return
		}
	}
	if organize.GroupOf == nil {
		organize.GroupOf = make(map[string]string)
	}
	for _, task := range uniqueAgentIDs(tasks) {
		if _, ok := m.agents.tasks[task]; !ok {
			continue
		}
		if groupID == "" {
			delete(organize.GroupOf, task)
		} else {
			organize.GroupOf[task] = groupID
		}
		organize.Pinned = removeAgentID(organize.Pinned, task)
	}
	m.saveAgentOrganize(organize)
}

func uniqueAgentIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func removeAgentID(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func (m *Model) agentSelectTask(taskID string, mods ui.Modifiers) {
	a := &m.agents
	if taskID == "" {
		a.selectedTasks = nil
		a.selectionAnchor = ""
		return
	}
	order := m.agentVisibleTaskIDs()
	index := -1
	for i, id := range order {
		if id == taskID {
			index = i
			break
		}
	}
	if index < 0 {
		order = append(order, taskID)
		index = len(order) - 1
	}
	switch {
	case mods&ui.Shift != 0 && a.selectionAnchor != "":
		anchor := -1
		for i, id := range order {
			if id == a.selectionAnchor {
				anchor = i
				break
			}
		}
		if anchor < 0 {
			anchor = index
		}
		from, to := min(anchor, index), max(anchor, index)
		a.selectedTasks = append([]string(nil), order[from:to+1]...)
	case mods&(ui.Cmd|ui.Ctrl) != 0:
		if slicesContains(a.selectedTasks, taskID) {
			a.selectedTasks = removeAgentID(a.selectedTasks, taskID)
		} else {
			a.selectedTasks = append(a.selectedTasks, taskID)
		}
		a.selectionAnchor = taskID
	default:
		a.selectedTasks = []string{taskID}
		a.selectionAnchor = taskID
	}
	m.invalidate()
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (m *Model) agentSelectedTasks() []string {
	out := make([]string, 0, len(m.agents.selectedTasks))
	for _, id := range m.agents.selectedTasks {
		if _, ok := m.agents.tasks[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

func (m *Model) agentContextTasks(taskID string) []string {
	selected := m.agentSelectedTasks()
	if slicesContains(selected, taskID) {
		return selected
	}
	if _, ok := m.agents.tasks[taskID]; ok {
		return []string{taskID}
	}
	return nil
}

func (m *Model) agentVisibleTaskIDs() []string {
	groups := m.taskGroups()
	total := 0
	for _, group := range groups {
		total += len(group.Tasks)
	}
	out := make([]string, 0, total)
	for _, group := range groups {
		for _, task := range group.Tasks {
			out = append(out, task.ID)
		}
	}
	return out
}

func (m *Model) agentTaskHeaderMenu(menu *ui.Menu) {
	location := m.agentSessionLocation()
	if menu.Item("Chat in Tool Window").Checked(location == "toolWindow").Chosen() {
		m.useAgentToolWindow()
	}
	if menu.Item("Chat in Editor").Checked(location == "editor").Chosen() {
		m.useAgentEditor()
	}
	menu.Separator()
	if menu.Item("New Group…").Chosen() {
		m.openNewAgentTaskGroup(nil)
	}
}

func (m *Model) agentTaskMenu(menu *ui.Menu, taskID string) {
	tasks := m.agentContextTasks(taskID)
	if len(tasks) == 0 {
		return
	}
	organize := m.agentOrganize()
	pinned := make(map[string]bool, len(organize.Pinned))
	for _, id := range organize.Pinned {
		pinned[id] = true
	}
	if len(tasks) == 1 {
		if task, ok := m.agents.tasks[tasks[0]]; ok {
			if menu.Item("Open in Editor").Chosen() {
				m.openTaskInEditorLocation(task.ID)
			}
			if menu.Item("Open in Tool Window").Chosen() {
				m.dockAgentTask(task.ID)
			}
			menu.Separator()
			if menu.Item("Rename…").Chosen() {
				m.openAgentRename(task.ID)
			}
			if menu.Item("Terminal").Chosen() {
				dir := task.Cwd
				if dir == "" {
					dir = m.Root()
				}
				m.OpenTerminalAt(dir)
			}
			if task.Capabilities.Fork && task.Status != "archived" {
				if menu.Item("Fork").Disabled(agentTaskBusy(task.Status)).Chosen() {
					m.forkAgentTask(task.ID)
				}
			}
			menu.Separator()
		}
	}
	var toPin, toUnpin []string
	for _, id := range tasks {
		if pinned[id] {
			toUnpin = append(toUnpin, id)
		} else {
			toPin = append(toPin, id)
		}
	}
	if len(toPin) > 0 {
		label := "Pin"
		if len(toPin) > 1 {
			label = fmt.Sprintf("Pin (%d)", len(toPin))
		}
		if menu.Item(label).Chosen() {
			m.pinAgentTasks(toPin, true)
		}
	}
	if len(toUnpin) > 0 {
		label := "Unpin"
		if len(toUnpin) > 1 {
			label = fmt.Sprintf("Unpin (%d)", len(toUnpin))
		}
		if menu.Item(label).Chosen() {
			m.pinAgentTasks(toUnpin, false)
		}
	}
	menu.Submenu("Move to Group", func(sub *ui.Menu) {
		for _, group := range organize.Groups {
			group := group
			all := true
			for _, id := range tasks {
				if organize.GroupOf[id] != group.ID {
					all = false
					break
				}
			}
			if sub.Item(group.Name).Checked(all).Chosen() {
				m.moveAgentTasksToGroup(tasks, group.ID)
			}
		}
		if len(organize.Groups) > 0 {
			sub.Separator()
		}
		if sub.Item("New Group…").Chosen() {
			m.openNewAgentTaskGroup(tasks)
		}
		inGroup := false
		for _, id := range tasks {
			if organize.GroupOf[id] != "" {
				inGroup = true
				break
			}
		}
		if inGroup && sub.Item("Remove from Group").Chosen() {
			m.moveAgentTasksToGroup(tasks, "")
		}
	})
	menu.Separator()
	var archived, archivable []string
	for _, id := range tasks {
		task, ok := m.agents.tasks[id]
		if !ok {
			continue
		}
		if task.Status == "archived" {
			archived = append(archived, id)
		} else if !agentTaskBusy(task.Status) {
			archivable = append(archivable, id)
		}
	}
	if len(archived) > 0 {
		label := "Unarchive"
		if len(archived) > 1 {
			label = fmt.Sprintf("Unarchive (%d)", len(archived))
		}
		if menu.Item(label).Chosen() {
			m.setAgentArchivedMany(archived, false)
		}
	}
	label := "Archive"
	if len(tasks) > 1 {
		label = fmt.Sprintf("Archive (%d)", len(archivable))
	}
	if menu.Item(label).Disabled(len(archivable) == 0).Chosen() {
		m.setAgentArchivedMany(archivable, true)
	}
	label = "Delete…"
	if len(tasks) > 1 {
		label = fmt.Sprintf("Delete (%d)…", len(tasks))
	}
	if menu.Item(label).Chosen() {
		m.openAgentDeleteTasks(tasks)
	}
}

func (m *Model) prepareAgentTaskMenu(taskID string) {
	if !slicesContains(m.agents.selectedTasks, taskID) {
		m.agentSelectTask(taskID, 0)
	}
}

func (m *Model) setAgentArchivedMany(tasks []string, archived bool) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	type archiveTarget struct {
		id    string
		title string
	}
	targets := make([]archiveTarget, 0, len(tasks))
	for _, id := range uniqueAgentIDs(tasks) {
		task, ok := m.agents.tasks[id]
		if !ok || (archived && agentTaskBusy(task.Status)) ||
			(!archived && task.Status != "archived") {
			continue
		}
		targets = append(targets, archiveTarget{id: id, title: task.Title})
	}
	if len(targets) == 0 {
		return
	}
	a := &m.agents
	command := "agent_archive"
	action := "archive"
	if !archived {
		command = "agent_unarchive"
		action = "unarchive"
	}
	root, gen := m.Root(), a.generation
	a.actionErr = ""
	m.invalidate()
	go func() {
		errs := make([]string, 0)
		for _, target := range targets {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			_, err := m.invoke(ctx, command, map[string]any{"task": target.id})
			cancel()
			if err != nil {
				errs = append(errs, target.title+": "+err.Error())
			}
		}
		m.post(func() {
			if a.generation != gen || m.Root() != root {
				return
			}
			if len(errs) > 0 {
				a.actionErr = fmt.Sprintf("Cannot %s agent tasks:\n%s", action, strings.Join(errs, "\n"))
			}
			m.invalidate()
		})
	}()
}

func (m *Model) agentTaskDrag(row ui.Element, taskID string) {
	a := &m.agents
	dx, dy, pressed := row.Dragged()
	if pressed {
		lx, ly, _ := row.PointerPosition()
		at := ui.Point{X: row.Bounds().X + lx, Y: row.Bounds().Y + ly}
		if a.taskDragID != taskID {
			if a.taskDragID == "" {
				a.taskDragStart = at
			}
		}
		if a.taskDragID == "" {
			a.taskDragAt = at
			if dx*dx+dy*dy > 16 {
				a.taskDragID = taskID
				a.taskDragTasks = m.agentContextTasks(taskID)
				if len(a.taskDragTasks) == 0 {
					a.taskDragTasks = []string{taskID}
				}
			}
			return
		}
		if a.taskDragID == taskID {
			a.taskDragAt = at
		}
		return
	}
	if a.taskDragID != taskID {
		return
	}
	target := a.taskDropTarget
	taskID = a.taskDragID
	tasks := append([]string(nil), a.taskDragTasks...)
	a.taskDragID = ""
	a.taskDragTasks = nil
	a.taskDragStart = ui.Point{}
	a.taskDragAt = ui.Point{}
	a.taskDropTarget = ""
	if target == "" {
		m.invalidate()
		return
	}
	if len(tasks) == 0 {
		tasks = []string{taskID}
	}
	switch {
	case target == "pinned":
		m.pinAgentTasks(tasks, true)
	case strings.HasPrefix(target, "group:"):
		m.moveAgentTasksToGroup(tasks, strings.TrimPrefix(target, "group:"))
	}
}

func (m *Model) agentGroupDropTarget(row ui.Element, target string) {
	if row.Hovered() && m.agents.taskDragID != "" {
		m.agents.taskDropTarget = target
	}
}

func sortAgentTasksByAttention(tasks []agentTask) {
	rank := func(status string) int {
		switch agentTaskGroupFor(status) {
		case "needsInput":
			return 0
		case "failed":
			return 1
		case "limited":
			return 2
		case "running":
			return 3
		case "ready":
			return 4
		default:
			return 5
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		left, right := tasks[i], tasks[j]
		if rank(left.Status) != rank(right.Status) {
			return rank(left.Status) < rank(right.Status)
		}
		if left.UpdatedMS != right.UpdatedMS {
			return left.UpdatedMS > right.UpdatedMS
		}
		return strings.ToLower(left.Title) < strings.ToLower(right.Title)
	})
}
