package app

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/egoist/mygo"
)

// OpenIDEA registers these task and reply operations dynamically rather than
// as palette rows. GoIDEA keeps the same ids invokable so keymaps, plugins and
// automation written for OpenIDEA reach the native equivalent.
func init() {
	for _, action := range agentCompatibilityActions() {
		_ = registerAction(action)
	}
}

func agentCompatibilityActions() []*Action {
	action := func(id, title string, run func(*Model, any)) *Action {
		return &Action{ID: id, Title: title, Category: "Agents", RunWithArg: run}
	}
	return []*Action{
		action("agents.retryConversation", "Retry Loading Conversation", func(m *Model, arg any) {
			taskID := m.agentActionTaskID()
			subagent := ""
			if request := agentArgObject(arg); request != nil {
				if value, ok := request["task"].(string); ok {
					taskID = value
				}
				if value, ok := request["subagent"].(string); ok {
					subagent = value
				}
			}
			if taskID != "" {
				m.loadAgentConversation(taskID, subagent, true)
			}
		}),
		action("agents.useToolWindow", "Chat in Tool Window", func(m *Model, _ any) {
			m.useAgentToolWindow()
		}),
		action("agents.useEditor", "Chat in Editor", func(m *Model, _ any) {
			m.useAgentEditor()
		}),
		action("agents.openTaskInEditor", "Open Agent Task in Editor", func(m *Model, arg any) {
			if taskID := m.agentActionTaskIDForArg(arg); taskID != "" {
				m.openTaskInEditorLocation(taskID)
			}
		}),
		action("agents.openTask", "Open Agent Task", func(m *Model, arg any) {
			if taskID := m.agentActionTaskIDForArg(arg); taskID != "" {
				m.openAgentTask(taskID)
			}
		}),
		action("agents.dockTask", "Open Agent Task in Tool Window", func(m *Model, arg any) {
			if taskID := m.agentActionTaskIDForArg(arg); taskID != "" {
				m.dockAgentTask(taskID)
			}
		}),
		action("agents.undockTask", "Open Agent Task in Editor", func(m *Model, _ any) {
			m.undockAgentTask()
		}),
		action("agents.renameTask", "Rename Agent Task", func(m *Model, arg any) {
			if taskID := m.agentActionTaskIDForArg(arg); taskID != "" {
				m.openAgentRename(taskID)
			}
		}),
		action("agents.forkTask", "Fork Agent Task", func(m *Model, arg any) {
			if taskID := m.agentActionTaskIDForArg(arg); taskID != "" {
				m.forkAgentTask(taskID)
			}
		}),
		action("agents.archiveTasks", "Archive Agent Tasks", func(m *Model, arg any) {
			m.setAgentArchivedMany(m.agentActionTaskIDsForArg(arg), true)
		}),
		action("agents.unarchiveTasks", "Unarchive Agent Tasks", func(m *Model, arg any) {
			m.setAgentArchivedMany(m.agentActionTaskIDsForArg(arg), false)
		}),
		action("agents.deleteTasks", "Delete Agent Tasks", func(m *Model, arg any) {
			m.openAgentDeleteTasks(m.agentActionTaskIDsForArg(arg))
		}),
		action("agents.pinTasks", "Pin Agent Tasks", func(m *Model, arg any) {
			m.pinAgentTasks(m.agentActionTaskIDsForArg(arg), true)
		}),
		action("agents.unpinTasks", "Unpin Agent Tasks", func(m *Model, arg any) {
			m.pinAgentTasks(m.agentActionTaskIDsForArg(arg), false)
		}),
		action("agents.newTaskGroup", "New Agent Task Group", func(m *Model, arg any) {
			m.openNewAgentTaskGroup(m.agentActionTaskIDsForArg(arg))
		}),
		action("agents.renameTaskGroup", "Rename Agent Task Group", func(m *Model, arg any) {
			groupID := agentArgString(arg)
			if groupID == "" {
				groupID = m.agents.groupID
			}
			if groupID != "" {
				m.openRenameAgentTaskGroup(groupID)
			}
		}),
		action("agents.removeTaskGroup", "Remove Agent Task Group", func(m *Model, arg any) {
			groupID := agentArgString(arg)
			if groupID == "" {
				groupID = m.agents.groupID
			}
			if groupID != "" {
				m.openRemoveAgentTaskGroup(groupID)
			}
		}),
		action("agents.copyReplyMarkdown", "Copy Markdown", func(m *Model, arg any) {
			if text := agentReplyTextArg(arg); text != "" {
				mygo.Clipboard.WriteText(text)
			} else if _, reply, ok := m.agentActionLatestReply(); ok {
				mygo.Clipboard.WriteText(reply.Text)
			}
		}),
		action("agents.jumpToLatest", "Jump to Latest", func(m *Model, _ any) {
			m.jumpAgentToLatest()
		}),
		action("agents.retractMessage", "Retract Message", func(m *Model, arg any) {
			taskID, entries, target, ok := m.agentReplyTargetForArg(arg)
			if !ok {
				return
			}
			m.retractAgentTurn(taskID, entries, target.turn.PromptIndex)
		}),
		action("agents.quoteReply", "Quote Reply", func(m *Model, arg any) {
			text := agentReplyTextArg(arg)
			if text == "" {
				if _, reply, ok := m.agentActionLatestReply(); ok {
					text = reply.Text
				}
			}
			if text != "" {
				m.quoteAgentText(text)
			}
		}),
		action("agents.regenerateReply", "Regenerate Response", func(m *Model, arg any) {
			taskID, entries, target, ok := m.agentReplyTargetForArg(arg)
			if !ok {
				return
			}
			m.regenerateAgentTurn(taskID, entries, target.turn.PromptIndex)
		}),
		action("agents.forkReply", "Fork Conversation from Here", func(m *Model, arg any) {
			taskID, _, target, ok := m.agentReplyTargetForArg(arg)
			if !ok {
				return
			}
			turns := target.turn.Number + 1
			m.forkAgentTaskFrom(taskID, &turns)
		}),
	}
}

type agentReplyTarget struct {
	turn  agentTurnPosition
	reply agentEntry
}

func (m *Model) agentActionTaskIDForArg(arg any) string {
	if arg == nil {
		return m.agentActionTaskID()
	}
	taskID := agentArgString(arg)
	if _, ok := m.agents.tasks[taskID]; ok {
		return taskID
	}
	return ""
}

func (m *Model) agentActionTaskIDsForArg(arg any) []string {
	if arg == nil {
		return m.agentActionTaskIDs()
	}
	var tasks []string
	switch value := arg.(type) {
	case string:
		tasks = []string{value}
	case []string:
		tasks = value
	case []any:
		for _, item := range value {
			if taskID, ok := item.(string); ok {
				tasks = append(tasks, taskID)
			}
		}
	case json.RawMessage:
		_ = json.Unmarshal(value, &tasks)
	}
	valid := tasks[:0]
	for _, taskID := range uniqueAgentIDs(tasks) {
		if _, ok := m.agents.tasks[taskID]; ok {
			valid = append(valid, taskID)
		}
	}
	return valid
}

func (m *Model) agentReplyTargetForArg(arg any) (string, []agentEntry, agentReplyTarget, bool) {
	target := agentReplyTarget{}
	request := agentArgObject(arg)
	taskID := m.agentActionTaskID()
	if request != nil {
		if value, ok := request["task"].(string); ok {
			taskID = value
		}
	}
	if taskID == "" {
		return "", nil, target, false
	}
	subagent := ""
	if taskID == m.agents.selected {
		subagent = m.agents.currentSubagent()
	}
	entries := m.agents.entries[agentEntriesKey(taskID, subagent)]
	if len(entries) == 0 && subagent != "" {
		entries = m.agents.entries[agentEntriesKey(taskID, "")]
	}
	if len(entries) == 0 {
		return taskID, nil, target, false
	}
	wantedTurn, hasTurn := agentArgTurn(request)
	for _, turn := range agentTurnPositions(entries) {
		if turn.LastReplyIndex < 0 || (hasTurn && turn.Number != wantedTurn) {
			continue
		}
		target.turn = turn
		target.reply = entries[turn.LastReplyIndex]
		if hasTurn {
			return taskID, entries, target, true
		}
	}
	return taskID, entries, target, target.turn.LastReplyIndex >= 0
}

func agentArgObject(arg any) map[string]any {
	switch value := arg.(type) {
	case map[string]any:
		return value
	case json.RawMessage:
		var object map[string]any
		if json.Unmarshal(value, &object) == nil {
			return object
		}
	}
	return nil
}

func agentArgString(arg any) string {
	switch value := arg.(type) {
	case string:
		return value
	case map[string]any:
		text, _ := value["task"].(string)
		if text == "" {
			text, _ = value["group"].(string)
		}
		return text
	case json.RawMessage:
		var text string
		_ = json.Unmarshal(value, &text)
		return text
	}
	return ""
}

func agentArgTurn(request map[string]any) (uint32, bool) {
	if request == nil {
		return 0, false
	}
	switch value := request["turn"].(type) {
	case float64:
		if value >= 0 && value <= math.MaxUint32 && value == math.Trunc(value) {
			return uint32(value), true
		}
	case int:
		if value >= 0 {
			return uint32(value), true
		}
	case uint32:
		return value, true
	case json.Number:
		if number, err := value.Int64(); err == nil && number >= 0 && number <= math.MaxUint32 {
			return uint32(number), true
		}
	}
	return 0, false
}

func agentReplyTextArg(arg any) string {
	if text, ok := arg.(string); ok {
		return text
	}
	if request := agentArgObject(arg); request != nil {
		text, _ := request["text"].(string)
		return text
	}
	return ""
}

func (m *Model) agentActionTaskID() string {
	if _, ok := m.agents.tasks[m.agents.selected]; ok {
		return m.agents.selected
	}
	if tab := m.activeAgentTaskTab(); tab != nil {
		if _, ok := m.agents.tasks[tab.agentTask]; ok {
			return tab.agentTask
		}
	}
	if tasks := m.agentSelectedTasks(); len(tasks) > 0 {
		return tasks[0]
	}
	return m.mostRecentAgentTask()
}

func (m *Model) agentActionTaskIDs() []string {
	if tasks := m.agentSelectedTasks(); len(tasks) > 0 {
		return tasks
	}
	if taskID := m.agentActionTaskID(); taskID != "" {
		return []string{taskID}
	}
	return nil
}

func (m *Model) agentActionConversation() (string, []agentEntry, bool) {
	taskID := m.agentActionTaskID()
	if taskID == "" {
		return "", nil, false
	}
	entries := m.agents.entries[agentEntriesKey(taskID, m.agents.currentSubagent())]
	if len(entries) == 0 {
		return taskID, nil, false
	}
	return taskID, entries, true
}

func (m *Model) agentActionLatestReply() (string, agentEntry, bool) {
	taskID, entries, ok := m.agentActionConversation()
	if !ok {
		return "", agentEntry{}, false
	}
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].Type == "agent" && strings.TrimSpace(entries[index].Text) != "" {
			return taskID, entries[index], true
		}
	}
	return taskID, agentEntry{}, false
}

func (m *Model) jumpAgentToLatest() {
	m.agents.conversationScroll.Y = math.MaxFloat32
	m.invalidate()
}
