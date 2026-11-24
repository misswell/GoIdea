package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

func init() {
	hiddenActionIDs["run.agentStart"] = true
	_ = registerAction(&Action{
		ID:       "run.agentStart",
		Title:    "Run Agent Command",
		Category: "Run",
		When:     func(m *Model) bool { return m.Root() != "" },
		RunWithArg: func(m *Model, arg any) {
			m.StartAgentRun(arg)
		},
	})
}

type agentRunRequest struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Cwd     string            `json:"cwd"`
	Env     map[string]string `json:"env"`
}

// StartAgentRun starts one command in the agent tab named by the request. A
// finished prior command is reused by starting the same stable config id
// again. A live same-name command is refused, as OpenIDEA's startAgentRun
// does. Other agent names may run concurrently, as the backend allows.
func (m *Model) StartAgentRun(arg any) bool {
	return m.startAgentRun(arg, nil)
}

func (m *Model) startAgentRun(arg any, done func(error)) bool {
	finish := func(err error) {
		if done != nil {
			done(err)
		}
	}
	request, err := decodeAgentRunRequest(arg)
	if err != nil {
		m.status = "Cannot start the agent command: " + err.Error()
		m.invalidate()
		finish(err)
		return false
	}
	if m.Root() == "" {
		err := fmt.Errorf("open a project before running an agent command")
		m.status = "Cannot start the agent command: " + err.Error()
		m.invalidate()
		finish(err)
		return false
	}
	if m.ctx == nil {
		err := fmt.Errorf("the run backend is not connected")
		m.status = "The run backend is not connected"
		m.invalidate()
		finish(err)
		return false
	}
	if !m.run.trusted {
		err := fmt.Errorf("the project is not trusted: accept the prompt before running")
		m.status = "The project is not trusted: accept the prompt before running"
		m.invalidate()
		finish(err)
		return false
	}
	m.agentRunSettingsPage()
	existing := m.agentRunByName(request.Name)
	if existing != nil && existing.live() {
		err := fmt.Errorf("%s is still running", request.Name)
		m.status = err.Error()
		if m.run.agentActive != existing.id {
			m.selectAgentRun(existing.id)
		}
		m.invalidate()
		finish(err)
		return false
	}

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "sh"
	}
	cwd := request.Cwd
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	emptyTarget := ""
	config := map[string]any{
		"id":            "mcp-agent:" + request.Name,
		"name":          request.Name,
		"kind":          "shell",
		"command":       request.Command,
		"args":          []string{},
		"cwd":           cwd,
		"env":           request.Env,
		"detected":      false,
		"isTest":        false,
		"allowParallel": true,
		"params": map[string]any{
			"mcp.agentRun":   request.Name,
			"mcp.agentShell": shell,
		},
		"target": emptyTarget,
	}
	raw, err := json.Marshal(config)
	if err != nil {
		m.status = "Cannot prepare the agent command: " + err.Error()
		m.invalidate()
		finish(err)
		return false
	}

	run := existing
	if run == nil {
		run = m.newAgentRunSession(request)
	} else {
		run.request = request
	}
	m.beginAgentRun(run)
	root, channel := m.Root(), run.channel
	m.run.visible = true
	m.occupyBottom("run")
	m.status = "Starting agent command " + request.Name + "…"
	m.invalidate()
	base := modelContext(m)

	args := map[string]any{
		"config":   json.RawMessage(raw),
		"onOutput": map[string]any{"$channel": channel},
		"request":  nil,
	}
	go func() {
		ctx, cancel := context.WithTimeout(base, 5*time.Minute)
		defer cancel()
		rawInfo, err := m.invoke(ctx, "run_start", args)
		var info openidea.RunInfo
		if err == nil {
			err = json.Unmarshal(rawInfo, &info)
		}
		m.post(func() {
			finished := false
			finishOnce := func(err error) {
				if finished {
					return
				}
				finished = true
				finish(err)
			}
			current := m.agentRunByID(run.id)
			if current == nil {
				if err == nil && info.ID != 0 {
					go func(id uint32) {
						ctx, cancel := context.WithTimeout(base, 15*time.Second)
						defer cancel()
						_, _ = m.invoke(ctx, "run_stop", map[string]any{"id": id})
					}(info.ID)
				}
				if err != nil {
					finishOnce(err)
				} else {
					finishOnce(fmt.Errorf("the agent command tab closed before it started"))
				}
				return
			}
			if m.Root() != root {
				if err == nil && info.ID != 0 {
					go func(id uint32) {
						ctx, cancel := context.WithTimeout(base, 15*time.Second)
						defer cancel()
						_, _ = m.invoke(ctx, "run_stop", map[string]any{"id": id})
					}(info.ID)
				}
				m.closeAgentRun(run.id)
				if err != nil {
					finishOnce(err)
				} else {
					finishOnce(fmt.Errorf("the project changed before the agent command started"))
				}
				return
			}
			if err != nil {
				current.awaiting = false
				current.running = false
				current.ended = time.Now()
				current.err = "Starting failed: " + err.Error()
				m.trimAgentRuns()
				m.invalidate()
				finishOnce(err)
				return
			}
			current.runID = info.ID
			current.pid = info.PID
			if code, exited := m.run.agentPendingExit[info.ID]; exited {
				delete(m.run.agentPendingExit, info.ID)
				m.finishAgentRun(current, code)
				finishOnce(nil)
				return
			}
			if !info.Running {
				m.finishAgentRun(current, nil)
				finishOnce(nil)
				return
			}
			current.running = true
			current.awaiting = false
			m.status = "Running agent command " + request.Name
			m.invalidate()
			finishOnce(nil)
		})
	}()
	return true
}

func decodeAgentRunRequest(arg any) (agentRunRequest, error) {
	raw, err := json.Marshal(arg)
	if err != nil {
		return agentRunRequest{}, err
	}
	var request agentRunRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return agentRunRequest{}, err
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Command = strings.TrimSpace(request.Command)
	if request.Name == "" {
		return agentRunRequest{}, fmt.Errorf("the agent name is missing")
	}
	if request.Command == "" {
		return agentRunRequest{}, fmt.Errorf("the command is missing")
	}
	if request.Env == nil {
		request.Env = map[string]string{}
	}
	return request, nil
}
