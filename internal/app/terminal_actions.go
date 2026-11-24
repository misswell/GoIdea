package app

import (
	"encoding/json"
	"strconv"
	"strings"
)

// terminalCompatibilityActions are the OpenIDEA terminal operations whose
// arguments cannot be represented by a plain Run(*Model) command.
func init() {
	for _, action := range terminalCompatibilityActionList() {
		_ = registerAction(action)
	}
}

func terminalCompatibilityActionList() []*Action {
	action := func(id, title string, run func(*Model, any)) *Action {
		return &Action{
			ID: id, Title: title, Category: "Terminal",
			Keywords:   "terminal shell pty split session command",
			RunWithArg: run,
		}
	}
	return []*Action{
		action("terminal.newPredefinedSession", "New Predefined Session", func(m *Model, arg any) {
			program := terminalActionString(arg)
			if program != "" {
				m.OpenTerminalCommand(program, nil, "", "")
			}
		}),
		action("terminal.openAt", "Open in Terminal", func(m *Model, arg any) {
			dir := terminalActionString(arg)
			if dir == "" {
				dir = m.Root()
			}
			m.OpenTerminalAt(dir)
		}),
		action("terminal.pluginSession", "Open Plugin Terminal Session", func(m *Model, arg any) {
			request := terminalActionObject(arg)
			program, _ := request["program"].(string)
			args := terminalActionStrings(request["args"])
			name, _ := request["name"].(string)
			cwd, _ := request["cwd"].(string)
			if program != "" {
				m.OpenTerminalCommand(program, args, name, cwd)
			}
		}),
		action("terminal.runCommand", "Run Command in Terminal", func(m *Model, arg any) {
			request := terminalActionObject(arg)
			program, _ := request["program"].(string)
			args := terminalActionStrings(request["args"])
			name, _ := request["name"].(string)
			cwd, _ := request["cwd"].(string)
			if program != "" {
				m.OpenTerminalCommand(program, args, name, cwd)
			}
		}),
		action("terminal.splitRight", "Split Right", func(m *Model, arg any) {
			m.splitTerminalAction(arg, "right", false)
		}),
		action("terminal.splitDown", "Split Down", func(m *Model, arg any) {
			m.splitTerminalAction(arg, "down", false)
		}),
		action("terminal.splitAndMoveRight", "Split and Move Right", func(m *Model, arg any) {
			m.splitTerminalAction(arg, "right", true)
		}),
		action("terminal.splitAndMoveDown", "Split and Move Down", func(m *Model, arg any) {
			m.splitTerminalAction(arg, "down", true)
		}),
		action("terminal.sampleTui", "Stream Sample TUI", func(m *Model, arg any) {
			m.RunTerminalSampleTUI(arg)
		}),
	}
}

func terminalActionString(arg any) string {
	switch value := arg.(type) {
	case string:
		return strings.TrimSpace(value)
	case json.RawMessage:
		var text string
		if json.Unmarshal(value, &text) == nil {
			return strings.TrimSpace(text)
		}
	case []byte:
		var text string
		if json.Unmarshal(value, &text) == nil {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func terminalActionObject(arg any) map[string]any {
	switch value := arg.(type) {
	case map[string]any:
		return value
	case json.RawMessage:
		var object map[string]any
		if json.Unmarshal(value, &object) == nil {
			return object
		}
	case []byte:
		var object map[string]any
		if json.Unmarshal(value, &object) == nil {
			return object
		}
	}
	return map[string]any{}
}

func terminalActionStrings(value any) []string {
	var out []string
	switch values := value.(type) {
	case []string:
		out = append(out, values...)
	case []any:
		for _, item := range values {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
	}
	return out
}

// OpenTerminalCommand is the backend of terminal.runCommand and plugin
// sessions: a terminal tab running this exact program rather than a shell.
func (m *Model) OpenTerminalCommand(program string, args []string, name, cwd string) {
	root := m.Root()
	if root == "" || program == "" {
		return
	}
	if cwd == "" {
		cwd = root
	}
	if !m.term.visible {
		m.term.visible = true
		m.occupyBottom("term")
	}
	m.spawnTermProcess(root, cwd, m.term.active, program, args, name)
}

func (m *Model) splitTerminalAction(arg any, axis string, move bool) {
	session := m.termSessionForAction(arg)
	if move {
		m.MoveTermToSplit(session, axis)
		return
	}
	if session == nil {
		m.SplitTerm(axis)
		return
	}
	if !m.term.visible {
		m.term.visible = true
		m.occupyBottom("term")
	}
	root := m.Root()
	if root == "" {
		return
	}
	if len(m.term.groups) < 2 {
		m.term.split = axis
		cwd := session.cwd
		if cwd == "" {
			cwd = root
		}
		m.spawnTermSession(root, cwd, 1)
		return
	}
	m.term.split = axis
	m.invalidate()
}

func (m *Model) termSessionForAction(arg any) *termSession {
	id := terminalActionString(arg)
	if id == "" {
		return m.term.current()
	}
	number, _ := strconv.ParseUint(id, 10, 32)
	for gi := range m.term.groups {
		for ti := range m.term.groups[gi].tabs {
			session := &m.term.groups[gi].tabs[ti]
			if uint64(session.id) == number || uint64(session.channel) == number {
				return session
			}
		}
	}
	return nil
}

// MoveTermToSplit is Split and Move: the selected PTY tab moves beside its
// current group. OpenIDEA only permits it when another tab stays behind.
func (m *Model) MoveTermToSplit(session *termSession, axis string) {
	if session == nil || len(m.term.groups) == 0 || m.term.split != "" {
		return
	}
	sourceGroup, sourceIndex := -1, -1
	for gi := range m.term.groups {
		for ti := range m.term.groups[gi].tabs {
			if &m.term.groups[gi].tabs[ti] == session {
				sourceGroup, sourceIndex = gi, ti
			}
		}
	}
	if sourceGroup != 0 || sourceIndex < 0 ||
		len(m.term.groups[sourceGroup].tabs) < 2 {
		return
	}
	moved := m.term.groups[sourceGroup].tabs[sourceIndex]
	m.term.remove(sourceGroup, sourceIndex)
	group := m.term.group(1)
	group.tabs = append(group.tabs, moved)
	group.active = len(group.tabs) - 1
	m.term.split = axis
	m.term.active = 1
	m.term.visible = true
	m.occupyBottom("term")
	m.invalidate()
}
